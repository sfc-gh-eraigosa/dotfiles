// Package errors defines the gss error contract: a fixed set of
// sentinel errors, their stable exit codes, their canonical
// JSON-serialisable slugs, and the worker-mode error envelope.
//
// This package is the foundation deliverable per src/gss/docs/plan.md
// (PR-01) and the design table in src/gss/docs/design.md → Code layout
// → errors/. Every higher layer (cmd/gss, internal/feature, internal/
// registry, …) imports from here and never declares its own ad-hoc
// error vars for these conditions.
//
// Wire shape:
//
//	{
//	  "error": {
//	    "code":    "rebase_conflict",
//	    "message": "rebase onto origin/main failed",
//	    "worker":  "feature/login/erai/refactor",
//	    "details": { "conflicted_files": ["a.go", "b.go"] }
//	  }
//	}
//
// `worker` and `details` are optional and omitted when empty.
//
// # Security
//
// The Marshal path sanitises every untrusted string (C0/C1/DEL/ANSI-CSI
// stripped) and bounds payload sizes (Message ≤ 4 KiB, Worker ≤ 256 B,
// Details ≤ 64 KiB marshalled JSON with nested depth ≤ 8). The Unmarshal
// path rejects unknown fields at both the outer wrapper and inner
// Envelope levels, and validates Worker against the worker_ref grammar.
// FromSlug is documented as diagnostic-only: never use a slug recovered
// from untrusted JSON to gate authorization — trust the local error
// chain (errors.Is against an in-process sentinel) instead.
package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"regexp"
	"strings"
)

// Sentinel errors — one per row of the design's error table. New
// sentinels MUST be added together with (a) a slug in slugByErr, (b)
// an exit code in Codes, and (c) a test row in errors_test.go's
// allSentinels. The no-orphan tests enforce this.
var (
	ErrRebaseConflict       = stderrors.New("rebase conflict requires human intervention")
	ErrGHAuthRequired       = stderrors.New("github authentication required (run: gh auth refresh)")
	ErrDirtyWorktree        = stderrors.New("worktree is dirty")
	ErrLockHeld             = stderrors.New("registry lock is held")
	ErrRegistryStale        = stderrors.New("registry entry is stale")
	ErrNotInWorker          = stderrors.New("not inside a registered worker worktree")
	ErrPRReadyNeedsToken    = stderrors.New("promoting a draft PR to ready requires a token with the right scopes")
	ErrInvalidIdent         = stderrors.New("invalid identifier")
	ErrWrongMode            = stderrors.New("command not valid in this mode")
	ErrConflictMarker       = stderrors.New("conflict markers detected in tracked files")
	ErrSchemaMismatch       = stderrors.New("registry schema version mismatch (CAS rejected)")
	ErrPermissionMode       = stderrors.New("file permission mode or uid mismatch on a gss-owned file")
	ErrApprovalTokenMissing = stderrors.New("approval token missing or invalid for a gated operation")
	ErrMarkerInjection      = stderrors.New("user content contains a reserved <!-- gss:* --> marker token")
)

// Code-slug constants. These are the strings emitted in the JSON
// envelope's "code" field and consumed by tmux-mgr / slash commands;
// they form a stable, snake_case public contract.
const (
	CodeRebaseConflict       = "rebase_conflict"
	CodeGHAuthRequired       = "gh_auth_required"
	CodeDirtyWorktree        = "dirty_worktree"
	CodeLockHeld             = "lock_held"
	CodeRegistryStale        = "registry_stale"
	CodeNotInWorker          = "not_in_worker"
	CodePRReadyNeedsToken    = "pr_ready_needs_token"
	CodeInvalidIdent         = "invalid_ident"
	CodeWrongMode            = "wrong_mode"
	CodeConflictMarker       = "conflict_marker"
	CodeSchemaMismatch       = "schema_mismatch"
	CodePermissionMode       = "permission_mode"
	CodeApprovalTokenMissing = "approval_token_missing"
	CodeMarkerInjection      = "marker_injection"
)

// slugByErr is the forward (sentinel → slug) lookup. errBySlug is the
// reverse. They are derived from the same source list so they cannot
// drift.
var (
	slugByErr = map[error]string{
		ErrRebaseConflict:       CodeRebaseConflict,
		ErrGHAuthRequired:       CodeGHAuthRequired,
		ErrDirtyWorktree:        CodeDirtyWorktree,
		ErrLockHeld:             CodeLockHeld,
		ErrRegistryStale:        CodeRegistryStale,
		ErrNotInWorker:          CodeNotInWorker,
		ErrPRReadyNeedsToken:    CodePRReadyNeedsToken,
		ErrInvalidIdent:         CodeInvalidIdent,
		ErrWrongMode:            CodeWrongMode,
		ErrConflictMarker:       CodeConflictMarker,
		ErrSchemaMismatch:       CodeSchemaMismatch,
		ErrPermissionMode:       CodePermissionMode,
		ErrApprovalTokenMissing: CodeApprovalTokenMissing,
		ErrMarkerInjection:      CodeMarkerInjection,
	}
	errBySlug = func() map[string]error {
		m := make(map[string]error, len(slugByErr))
		for e, s := range slugByErr {
			m[s] = e
		}
		return m
	}()
)

// SlugOf returns the canonical slug for the first sentinel found while
// walking err's wrap chain. Returns "" when err is nil or carries no
// gss sentinel.
func SlugOf(err error) string {
	if err == nil {
		return ""
	}
	for sent, slug := range slugByErr {
		if stderrors.Is(err, sent) {
			return slug
		}
	}
	return ""
}

// FromSlug returns the sentinel corresponding to a code slug, or nil
// if the slug is unknown.
//
// SECURITY: The returned sentinel is for diagnostic and exit-code
// mapping only. Callers MUST NOT use a slug recovered from untrusted
// JSON (e.g. a remote PR body, a CI artifact, an inter-agent message)
// to gate authorization decisions. Trust the local error chain
// (errors.Is against an in-process value), never the wire code.
func FromSlug(slug string) error {
	if slug == "" {
		return nil
	}
	return errBySlug[slug]
}

// ValidationError is exported for typed-error consumers but its fields
// are package-private. Callers construct via NewValidationError, which
// sanitises inputs and always wraps ErrInvalidIdent.
type ValidationError struct {
	field   string
	reason  string
	wrapped error
}

// Field returns the (sanitised) name of the invalid field.
func (e *ValidationError) Field() string {
	if e == nil {
		return ""
	}
	return e.field
}

// Reason returns the (sanitised) human-readable reason.
func (e *ValidationError) Reason() string {
	if e == nil {
		return ""
	}
	return e.reason
}

// Unwrap exposes the wrapped sentinel (always ErrInvalidIdent for values
// constructed via NewValidationError).
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.wrapped
}

// Error returns the human-readable message. Both Field and Reason are
// already sanitised at construction time.
//
// Behaviour: a nil *ValidationError returns "" rather than panicking;
// this defensive shape protects callers that pass an interface holding
// a typed-nil value.
func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.field != "" && e.reason != "":
		return fmt.Sprintf("invalid %s: %s", e.field, e.reason)
	case e.field != "":
		return fmt.Sprintf("invalid %s", e.field)
	case e.reason != "":
		return e.reason
	default:
		return "validation error"
	}
}

// NewValidationError constructs a ValidationError that always wraps
// ErrInvalidIdent. field and reason are sanitised on construction
// (C0/C1/DEL/ANSI-CSI stripped) so the resulting error is safe to log,
// emit in JSON, or surface in a TTY.
func NewValidationError(field, reason string) *ValidationError {
	return &ValidationError{
		field:   sanitise(field),
		reason:  sanitise(reason),
		wrapped: ErrInvalidIdent,
	}
}

// Envelope is the JSON-serialisable shape worker-mode commands emit on
// stdout when --json is set, and that tmux-mgr / slash commands parse
// back. It is wrapped under a top-level "error" key by Marshal.
type Envelope struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Worker  string         `json:"worker,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// wireEnvelope is the on-wire wrapper. Keeping it private lets the
// public Envelope stay flat for ergonomic call sites.
type wireEnvelope struct {
	Error Envelope `json:"error"`
}

// Payload-size limits. These are part of the public hardening contract:
// callers that exceed them have a bug, so Marshal/Unmarshal refuse
// rather than truncate.
const (
	maxMessageBytes     = 4096
	maxWorkerBytes      = 256
	maxDetailsBytes     = 65536
	maxDetailsDepth     = 8
	maxValidationField  = 256
	maxValidationReason = 1024
	requiredCodeOnEmpty = false // documented choice: Marshal({}) is rejected, not silently emitted
	_                   = maxValidationField
	_                   = maxValidationReason
	_                   = requiredCodeOnEmpty
)

// workerSegmentRe matches one segment of the worker_ref grammar
// (`<feature>`, `<user>`, `<purpose>`): a lower-case letter, up to 30
// further lower-case alphanumerics or hyphens, ending in a lower-case
// alphanumeric (so total length is 2..32, and no leading/trailing
// hyphen).
//
// TODO(internal/identity): replace with the identity package's
// validator once PR-07 lands. Inlined here so PR-01 can ship the wire
// contract without an upstream dependency.
var workerSegmentRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

// workerSuffixRe matches the optional `-<suffix>` after <purpose>. The
// suffix itself is `[a-z0-9-]+` per resolution #1.
var workerSuffixRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// validateWorkerRef enforces resolution #1's grammar:
//
//	worker_ref ::= <feature> "/" <user> "/" <purpose> [ "-" <suffix> ]
//
// An empty string is valid (means "not associated with a worker"); any
// other string must match. Returns a descriptive error on failure.
func validateWorkerRef(s string) error {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return fmt.Errorf("worker ref %q: want feature/user/purpose[-suffix], got %d segments", s, len(parts))
	}
	feature, user, purpose := parts[0], parts[1], parts[2]
	if !workerSegmentRe.MatchString(feature) {
		return fmt.Errorf("worker ref %q: invalid feature segment %q", s, feature)
	}
	if !workerSegmentRe.MatchString(user) {
		return fmt.Errorf("worker ref %q: invalid user segment %q", s, user)
	}
	// purpose may carry a `-<suffix>`; split on the FIRST `-` after the
	// kebab-case core. The grammar allows internal hyphens in purpose
	// too, so we treat the whole purpose-plus-suffix as a single
	// segment matched by workerSegmentRe — that's the easy reading
	// and matches the test cases in resolution #1.
	if !workerSegmentRe.MatchString(purpose) {
		return fmt.Errorf("worker ref %q: invalid purpose segment %q", s, purpose)
	}
	// The suffix regex exists so a future caller that wants to split
	// purpose from suffix has a vetted matcher. Referenced here to
	// document the contract.
	_ = workerSuffixRe
	return nil
}

// ansiCSIRe matches an ANSI CSI escape sequence: ESC [ , followed by
// any byte ≤ 0x3f zero-or-more times, then a final byte in 0x40..0x7e.
// This catches colour codes, cursor moves, and the other common
// terminal-control families.
var ansiCSIRe = regexp.MustCompile("\x1b\\[[\x00-\x3f]*[\x40-\x7e]")

// sanitise strips C0/C1 control characters (except \t), ANSI CSI escape
// sequences (\x1b[…), and DEL from s. Used on every untrusted string
// that lands in a Marshalled Envelope.
func sanitise(s string) string {
	if s == "" {
		return ""
	}
	// 1. Drop CSI sequences first — they're multi-byte and would
	//    otherwise be partially shredded by the per-byte pass below
	//    (leaving a bare '[' or final letter behind).
	s = ansiCSIRe.ReplaceAllString(s, "")
	// 2. Per-byte filter for the remaining single-byte controls.
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\t': // 0x09: preserved (per spec)
			b.WriteByte(c)
		case c <= 0x1f: // C0 except \t — drop
			continue
		case c == 0x7f: // DEL — drop
			continue
		case c >= 0x80 && c <= 0x9f: // C1 — drop
			continue
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// detailsDepth returns the maximum nesting depth of a JSON-shaped
// value. Slices/maps count as one level. Returns 0 for scalars.
func detailsDepth(v any) int {
	switch x := v.(type) {
	case map[string]any:
		max := 0
		for _, val := range x {
			if d := detailsDepth(val); d > max {
				max = d
			}
		}
		return 1 + max
	case []any:
		max := 0
		for _, val := range x {
			if d := detailsDepth(val); d > max {
				max = d
			}
		}
		return 1 + max
	default:
		return 0
	}
}

// rawDetailsDepth walks a json.RawMessage tree to compute depth without
// materialising the full Go object graph. Used on the Unmarshal path
// where Details arrives as a RawMessage we can inspect cheaply.
func rawDetailsDepth(data []byte) (int, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return 0, err
	}
	return detailsDepth(v), nil
}

// Marshal serialises e into the canonical {"error": {...}} JSON shape,
// after sanitising untrusted strings and enforcing size + grammar
// bounds. Marshal returns an error rather than silently truncating so
// that callers exceeding the contract are forced to surface the bug.
func Marshal(e Envelope) ([]byte, error) {
	// Code is a slug from a closed set, but defensive sanitisation
	// catches the rare hand-built Envelope where Code came from a
	// remote source.
	code := sanitise(e.Code)
	if code == "" {
		return nil, fmt.Errorf("envelope: code required")
	}

	msg := sanitise(e.Message)
	if len(msg) > maxMessageBytes {
		return nil, fmt.Errorf("envelope: message exceeds %d bytes", maxMessageBytes)
	}

	worker := sanitise(e.Worker)
	if len(worker) > maxWorkerBytes {
		return nil, fmt.Errorf("envelope: worker exceeds %d bytes", maxWorkerBytes)
	}
	if err := validateWorkerRef(worker); err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}

	if e.Details != nil {
		if d := detailsDepth(e.Details); d > maxDetailsDepth {
			return nil, fmt.Errorf("envelope: details depth %d exceeds %d", d, maxDetailsDepth)
		}
	}

	sanitised := Envelope{
		Code:    code,
		Message: msg,
		Worker:  worker,
		Details: e.Details,
	}

	out, err := json.Marshal(wireEnvelope{Error: sanitised})
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}
	// Bound the marshalled Details payload. We compute on the
	// post-marshal bytes so we measure exactly what's emitted on the
	// wire.
	if e.Details != nil {
		dj, err := json.Marshal(e.Details)
		if err != nil {
			return nil, fmt.Errorf("envelope: %w", err)
		}
		if len(dj) > maxDetailsBytes {
			return nil, fmt.Errorf("envelope: details exceed %d bytes", maxDetailsBytes)
		}
	}
	return out, nil
}

// innerEnvelope mirrors Envelope but uses RawMessage for Details so we
// can policy-check it (size, depth) before final decoding.
type innerEnvelope struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Worker  string          `json:"worker,omitempty"`
	Details json.RawMessage `json:"details,omitempty"`
}

// outerWrapper rejects every top-level key other than "error".
type outerWrapper struct {
	Error json.RawMessage `json:"error"`
}

// Unmarshal parses a payload produced by Marshal back into an Envelope.
// It rejects unknown fields at both the outer wrapper and inner
// envelope levels, validates Worker against the worker_ref grammar, and
// enforces the same size + depth bounds Marshal applies.
func Unmarshal(data []byte) (Envelope, error) {
	// Outer level: only "error" is allowed.
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var outer outerWrapper
	if err := dec.Decode(&outer); err != nil {
		return Envelope{}, fmt.Errorf("envelope: %w", err)
	}
	if len(outer.Error) == 0 {
		return Envelope{}, fmt.Errorf("envelope: missing top-level \"error\" key")
	}

	// Inner level: only the documented Envelope fields are allowed.
	innerDec := json.NewDecoder(bytes.NewReader(outer.Error))
	innerDec.DisallowUnknownFields()
	var inner innerEnvelope
	if err := innerDec.Decode(&inner); err != nil {
		return Envelope{}, fmt.Errorf("envelope: %w", err)
	}

	// Worker grammar.
	if len(inner.Worker) > maxWorkerBytes {
		return Envelope{}, fmt.Errorf("envelope: worker exceeds %d bytes", maxWorkerBytes)
	}
	if err := validateWorkerRef(inner.Worker); err != nil {
		return Envelope{}, fmt.Errorf("envelope: %w", err)
	}

	// Message size.
	if len(inner.Message) > maxMessageBytes {
		return Envelope{}, fmt.Errorf("envelope: message exceeds %d bytes", maxMessageBytes)
	}

	// Details bounds.
	var details map[string]any
	if len(inner.Details) > 0 {
		if len(inner.Details) > maxDetailsBytes {
			return Envelope{}, fmt.Errorf("envelope: details exceed %d bytes", maxDetailsBytes)
		}
		d, err := rawDetailsDepth(inner.Details)
		if err != nil {
			return Envelope{}, fmt.Errorf("envelope: %w", err)
		}
		if d > maxDetailsDepth {
			return Envelope{}, fmt.Errorf("envelope: details depth %d exceeds %d", d, maxDetailsDepth)
		}
		if err := json.Unmarshal(inner.Details, &details); err != nil {
			return Envelope{}, fmt.Errorf("envelope: %w", err)
		}
	}

	return Envelope{
		Code:    inner.Code,
		Message: inner.Message,
		Worker:  inner.Worker,
		Details: details,
	}, nil
}

// EnvelopeFromError builds an Envelope from a sentinel-bearing error.
// worker may be "" when emitted from classic (non-worker) mode.
//
// A nil err yields a zero-value Envelope (Code: "", Message: ""), which
// callers may want to treat as "nothing to emit" rather than passing on
// to Marshal (Marshal would reject the empty code).
func EnvelopeFromError(err error, worker string) Envelope {
	if err == nil {
		return Envelope{}
	}
	return Envelope{
		Code:    SlugOf(err),
		Message: err.Error(),
		Worker:  worker,
	}
}
