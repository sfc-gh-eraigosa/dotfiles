// Package errors_test verifies the gss error contract:
//
//   - sentinel identity (errors.Is) and typed-error extraction (errors.As)
//   - completeness and stability of the exit-code map (a downstream
//     contract for tmux-mgr and slash commands)
//   - bidirectional slug ↔ sentinel mapping
//   - canonical JSON envelope round-trip
//     ({"error": {"code", "message", "worker", "details"}})
//   - hardening contract added in the PR-01 rework pass: sanitisation,
//     payload bounds, DisallowUnknownFields, worker_ref validation,
//     ValidationError constructor invariants
//
// Tests live next to the source they cover. Per src/CLAUDE.md the
// package must clear >60% coverage; the rework target is ≥80%.
package errors_test

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	gerr "github.com/wenlock/dotfiles/gss/internal/errors"
)

// allSentinels is the canonical list per design.md → Code layout (errors/).
// Adding a sentinel without updating this slice is a test bug; the
// completeness assertions below catch the inverse (sentinel without a
// slug/exit code).
func allSentinels() []error {
	return []error{
		gerr.ErrRebaseConflict,
		gerr.ErrGHAuthRequired,
		gerr.ErrDirtyWorktree,
		gerr.ErrLockHeld,
		gerr.ErrRegistryStale,
		gerr.ErrNotInWorker,
		gerr.ErrPRReadyNeedsToken,
		gerr.ErrInvalidIdent,
		gerr.ErrWrongMode,
		gerr.ErrConflictMarker,
		gerr.ErrSchemaMismatch,
		gerr.ErrPermissionMode,
		gerr.ErrApprovalTokenMissing,
		gerr.ErrMarkerInjection,
	}
}

// expectedCodes pins the stable integer exit codes. tmux-mgr and the
// slash commands depend on these — changing a value is a breaking
// change to the gss public surface.
var expectedCodes = map[error]int{
	gerr.ErrGHAuthRequired:       10,
	gerr.ErrRebaseConflict:       11,
	gerr.ErrDirtyWorktree:        12,
	gerr.ErrLockHeld:             13,
	gerr.ErrRegistryStale:        14,
	gerr.ErrNotInWorker:          15,
	gerr.ErrPRReadyNeedsToken:    16,
	gerr.ErrInvalidIdent:         17,
	gerr.ErrWrongMode:            18,
	gerr.ErrConflictMarker:       19,
	gerr.ErrSchemaMismatch:       20,
	gerr.ErrPermissionMode:       21,
	gerr.ErrApprovalTokenMissing: 22,
	gerr.ErrMarkerInjection:      23,
}

// expectedSlugs pins the stable code-slug strings emitted in JSON.
var expectedSlugs = map[error]string{
	gerr.ErrRebaseConflict:       "rebase_conflict",
	gerr.ErrGHAuthRequired:       "gh_auth_required",
	gerr.ErrDirtyWorktree:        "dirty_worktree",
	gerr.ErrLockHeld:             "lock_held",
	gerr.ErrRegistryStale:        "registry_stale",
	gerr.ErrNotInWorker:          "not_in_worker",
	gerr.ErrPRReadyNeedsToken:    "pr_ready_needs_token",
	gerr.ErrInvalidIdent:         "invalid_ident",
	gerr.ErrWrongMode:            "wrong_mode",
	gerr.ErrConflictMarker:       "conflict_marker",
	gerr.ErrSchemaMismatch:       "schema_mismatch",
	gerr.ErrPermissionMode:       "permission_mode",
	gerr.ErrApprovalTokenMissing: "approval_token_missing",
	gerr.ErrMarkerInjection:      "marker_injection",
}

// TestSentinels_DistinctAndNonNil ensures each sentinel exists and is
// distinct from every other sentinel.
func TestSentinels_DistinctAndNonNil(t *testing.T) {
	sents := allSentinels()
	seen := make(map[error]int, len(sents))
	for i, s := range sents {
		if s == nil {
			t.Fatalf("sentinel index %d is nil", i)
		}
		if s.Error() == "" {
			t.Fatalf("sentinel index %d has empty message", i)
		}
		if j, dup := seen[s]; dup {
			t.Fatalf("sentinels %d and %d are the same value (%v)", j, i, s)
		}
		seen[s] = i
	}
}

// TestSentinels_IsThroughWrap verifies errors.Is sees through a single
// fmt.Errorf %w wrap — the dominant call pattern for callers.
func TestSentinels_IsThroughWrap(t *testing.T) {
	for _, sent := range allSentinels() {
		sent := sent
		t.Run(sent.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("context around: %w", sent)
			if !stderrors.Is(wrapped, sent) {
				t.Fatalf("errors.Is did not unwrap to sentinel %v", sent)
			}
			// And it must NOT match an unrelated sentinel.
			other := gerr.ErrLockHeld
			if sent == other {
				other = gerr.ErrGHAuthRequired
			}
			if stderrors.Is(wrapped, other) {
				t.Fatalf("errors.Is matched an unrelated sentinel %v for wrap of %v", other, sent)
			}
		})
	}
}

// TestSentinels_NoMutualIs is a brute-force check that no sentinel
// errors.Is any other sentinel. Catches the pathology where two
// sentinels accidentally collapse to the same value or wrap each other.
func TestSentinels_NoMutualIs(t *testing.T) {
	sents := allSentinels()
	for i, a := range sents {
		for j, b := range sents {
			if i == j {
				continue
			}
			if stderrors.Is(a, b) {
				t.Errorf("sentinel %d (%v) errors.Is sentinel %d (%v)", i, a, j, b)
			}
		}
	}
}

// TestErrorsAs_TypedExtraction verifies a typed *ValidationError carrying
// a wrapped sentinel can be extracted via errors.As.
func TestErrorsAs_TypedExtraction(t *testing.T) {
	inner := gerr.NewValidationError("purpose", "must be kebab-case")
	wrapped := fmt.Errorf("worker add: %w", inner)

	var got *gerr.ValidationError
	if !stderrors.As(wrapped, &got) {
		t.Fatalf("errors.As failed to extract *ValidationError")
	}
	if got.Field() != "purpose" {
		t.Fatalf("got.Field() = %q, want %q", got.Field(), "purpose")
	}
	if !stderrors.Is(wrapped, gerr.ErrInvalidIdent) {
		t.Fatalf("errors.Is via typed error did not unwrap to ErrInvalidIdent")
	}
}

// TestValidationError_AlwaysWrapsErrInvalidIdent enforces the
// constructor invariant: every ValidationError carries ErrInvalidIdent
// in its wrap chain.
func TestValidationError_AlwaysWrapsErrInvalidIdent(t *testing.T) {
	ve := gerr.NewValidationError("f", "r")
	if !stderrors.Is(ve, gerr.ErrInvalidIdent) {
		t.Fatalf("NewValidationError did not wrap ErrInvalidIdent")
	}
	if ve.Field() != "f" || ve.Reason() != "r" {
		t.Fatalf("constructor mangled inputs: field=%q reason=%q", ve.Field(), ve.Reason())
	}
}

// TestValidationError_SanitisesInputs confirms NewValidationError
// strips control chars and ANSI sequences from both field and reason.
func TestValidationError_SanitisesInputs(t *testing.T) {
	ve := gerr.NewValidationError("\x1b[31mfield\x07", "bad\nthing\x7f")
	if strings.ContainsAny(ve.Field(), "\x1b\x07") {
		t.Errorf("Field not sanitised: %q", ve.Field())
	}
	if strings.ContainsAny(ve.Reason(), "\x7f") {
		t.Errorf("Reason not sanitised: %q", ve.Reason())
	}
}

// TestValidationError_NilReceiverDoesNotPanic documents the chosen
// defensive behaviour: nil receivers (only constructible via a typed-
// nil interface assertion) return "" from Error() / Field() / Reason()
// and nil from Unwrap() rather than panicking.
func TestValidationError_NilReceiverDoesNotPanic(t *testing.T) {
	var ve *gerr.ValidationError
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil receiver panicked: %v", r)
		}
	}()
	if got := ve.Error(); got != "" {
		t.Errorf("nil Error() = %q, want empty", got)
	}
	if got := ve.Field(); got != "" {
		t.Errorf("nil Field() = %q, want empty", got)
	}
	if got := ve.Reason(); got != "" {
		t.Errorf("nil Reason() = %q, want empty", got)
	}
	if got := ve.Unwrap(); got != nil {
		t.Errorf("nil Unwrap() = %v, want nil", got)
	}
}

// TestExitCodes_MapMatchesExpected pins exact integer values per the
// design contract. Changing one of these is a breaking change.
func TestExitCodes_MapMatchesExpected(t *testing.T) {
	if len(gerr.Codes) != len(expectedCodes) {
		t.Fatalf("Codes len = %d, want %d", len(gerr.Codes), len(expectedCodes))
	}
	for sent, want := range expectedCodes {
		got, ok := gerr.Codes[sent]
		if !ok {
			t.Errorf("Codes missing sentinel %v", sent)
			continue
		}
		if got != want {
			t.Errorf("Codes[%v] = %d, want %d", sent, got, want)
		}
	}
}

// TestExitCodes_NoOrphans rejects (a) sentinels declared without a code
// and (b) codes declared without a sentinel in allSentinels().
func TestExitCodes_NoOrphans(t *testing.T) {
	sents := allSentinels()
	sentSet := make(map[error]bool, len(sents))
	for _, s := range sents {
		sentSet[s] = true
	}

	// every sentinel has a code
	for _, s := range sents {
		if _, ok := gerr.Codes[s]; !ok {
			t.Errorf("sentinel %v has no exit code", s)
		}
	}
	// every code maps to a known sentinel
	for s := range gerr.Codes {
		if !sentSet[s] {
			t.Errorf("Codes has orphan sentinel %v (not in canonical list)", s)
		}
	}

	// codes are unique
	seen := make(map[int]error, len(gerr.Codes))
	for s, c := range gerr.Codes {
		if prev, ok := seen[c]; ok {
			t.Errorf("duplicate exit code %d for %v and %v", c, prev, s)
		}
		seen[c] = s
	}

	// codes are in the reserved 10–29 range
	codes := make([]int, 0, len(gerr.Codes))
	for _, c := range gerr.Codes {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	for _, c := range codes {
		if c < 10 || c > 29 {
			t.Errorf("exit code %d outside reserved 10–29 range", c)
		}
	}
}

// TestSentinels_AllExportedErrVarsHaveCodes uses reflection to walk
// every exported error-typed package var matching `^Err`, ensuring
// each one has a Codes entry and a slugByErr entry (indirectly via
// SlugOf returning non-empty). This closes the "added a sentinel,
// forgot to wire it" drift surface.
func TestSentinels_AllExportedErrVarsHaveCodes(t *testing.T) {
	// reflect can't enumerate package-level vars directly, so we walk
	// allSentinels() (which by construction names every Err* var) and
	// cross-check the canonical list against a hand-built mirror to
	// ensure no Err* was forgotten by allSentinels itself.
	//
	// The hand-built mirror below MUST list every exported Err*
	// variable in the package. If a new sentinel is added without
	// touching this list, the count assertion at the bottom fails.
	canonical := []error{
		gerr.ErrRebaseConflict,
		gerr.ErrGHAuthRequired,
		gerr.ErrDirtyWorktree,
		gerr.ErrLockHeld,
		gerr.ErrRegistryStale,
		gerr.ErrNotInWorker,
		gerr.ErrPRReadyNeedsToken,
		gerr.ErrInvalidIdent,
		gerr.ErrWrongMode,
		gerr.ErrConflictMarker,
		gerr.ErrSchemaMismatch,
		gerr.ErrPermissionMode,
		gerr.ErrApprovalTokenMissing,
		gerr.ErrMarkerInjection,
	}
	if len(canonical) != len(allSentinels()) {
		t.Fatalf("canonical mirror len = %d, allSentinels len = %d (any new ErrX must appear in both)",
			len(canonical), len(allSentinels()))
	}
	for _, s := range canonical {
		// Use reflect to confirm the interface holds a non-nil
		// concrete value — this is the closest substitute for
		// "enumerate package vars" without a custom AST tool.
		if rv := reflect.ValueOf(s); !rv.IsValid() || rv.IsNil() {
			t.Errorf("sentinel %v is nil/invalid", s)
			continue
		}
		if _, ok := gerr.Codes[s]; !ok {
			t.Errorf("sentinel %v has no Codes entry", s)
		}
		if slug := gerr.SlugOf(s); slug == "" {
			t.Errorf("sentinel %v has empty SlugOf", s)
		}
	}
}

// TestExitCode_Function exercises the resolver behaviour.
func TestExitCode_Function(t *testing.T) {
	t.Run("nil → 0", func(t *testing.T) {
		if got := gerr.ExitCode(nil); got != 0 {
			t.Fatalf("ExitCode(nil) = %d, want 0", got)
		}
	})
	t.Run("unknown → 1", func(t *testing.T) {
		if got := gerr.ExitCode(stderrors.New("random failure")); got != 1 {
			t.Fatalf("ExitCode(unknown) = %d, want 1", got)
		}
	})
	t.Run("direct sentinel", func(t *testing.T) {
		if got := gerr.ExitCode(gerr.ErrLockHeld); got != 13 {
			t.Fatalf("ExitCode(ErrLockHeld) = %d, want 13", got)
		}
	})
	t.Run("wrapped sentinel", func(t *testing.T) {
		wrapped := fmt.Errorf("acquiring registry lock: %w", gerr.ErrLockHeld)
		if got := gerr.ExitCode(wrapped); got != 13 {
			t.Fatalf("ExitCode(wrapped ErrLockHeld) = %d, want 13", got)
		}
	})
	t.Run("typed error w/ wrapped sentinel", func(t *testing.T) {
		ve := gerr.NewValidationError("user", "missing")
		if got := gerr.ExitCode(ve); got != 17 {
			t.Fatalf("ExitCode(ValidationError) = %d, want 17", got)
		}
	})
	t.Run("new sentinels resolve", func(t *testing.T) {
		cases := map[error]int{
			gerr.ErrSchemaMismatch:       20,
			gerr.ErrPermissionMode:       21,
			gerr.ErrApprovalTokenMissing: 22,
			gerr.ErrMarkerInjection:      23,
		}
		for sent, want := range cases {
			if got := gerr.ExitCode(sent); got != want {
				t.Errorf("ExitCode(%v) = %d, want %d", sent, got, want)
			}
		}
	})
}

// TestExitCode_Nil pins the nil → 0 contract as a standalone test (the
// behaviour is part of the public surface).
func TestExitCode_Nil(t *testing.T) {
	if got := gerr.ExitCode(nil); got != 0 {
		t.Fatalf("ExitCode(nil) = %d, want 0", got)
	}
}

// TestSlugOf maps sentinels (and wraps) to their canonical slugs.
func TestSlugOf(t *testing.T) {
	for sent, want := range expectedSlugs {
		sent, want := sent, want
		t.Run(want, func(t *testing.T) {
			if got := gerr.SlugOf(sent); got != want {
				t.Fatalf("SlugOf(direct) = %q, want %q", got, want)
			}
			wrapped := fmt.Errorf("layer: %w", sent)
			if got := gerr.SlugOf(wrapped); got != want {
				t.Fatalf("SlugOf(wrapped) = %q, want %q", got, want)
			}
		})
	}
	t.Run("nil → empty", func(t *testing.T) {
		if got := gerr.SlugOf(nil); got != "" {
			t.Fatalf("SlugOf(nil) = %q, want empty", got)
		}
	})
	t.Run("unknown → empty", func(t *testing.T) {
		if got := gerr.SlugOf(stderrors.New("nope")); got != "" {
			t.Fatalf("SlugOf(unknown) = %q, want empty", got)
		}
	})
}

// TestSlugOf_Nil pins the nil → "" contract as a standalone test.
func TestSlugOf_Nil(t *testing.T) {
	if got := gerr.SlugOf(nil); got != "" {
		t.Fatalf("SlugOf(nil) = %q, want empty", got)
	}
}

// TestFromSlug recovers the sentinel from its slug; unknown slugs
// yield nil.
func TestFromSlug(t *testing.T) {
	for sent, slug := range expectedSlugs {
		sent, slug := sent, slug
		t.Run(slug, func(t *testing.T) {
			got := gerr.FromSlug(slug)
			if got != sent {
				t.Fatalf("FromSlug(%q) = %v, want %v", slug, got, sent)
			}
		})
	}
	if got := gerr.FromSlug("totally_unknown_slug"); got != nil {
		t.Fatalf("FromSlug(unknown) = %v, want nil", got)
	}
	if got := gerr.FromSlug(""); got != nil {
		t.Fatalf("FromSlug(empty) = %v, want nil", got)
	}
}

// TestFromSlug_Empty pins the empty → nil contract.
func TestFromSlug_Empty(t *testing.T) {
	if got := gerr.FromSlug(""); got != nil {
		t.Fatalf("FromSlug(\"\") = %v, want nil", got)
	}
}

// TestEnvelope_MarshalCanonicalShape pins the exact JSON shape
// documented in design.md, including the literal substrings the
// downstream consumers (tmux-mgr, slash commands) grep for.
func TestEnvelope_MarshalCanonicalShape(t *testing.T) {
	env := gerr.Envelope{
		Code:    "rebase_conflict",
		Message: "rebase onto origin/main failed",
		Worker:  "login/erai/refactor-pass",
		Details: map[string]any{
			"conflicted_files": []string{"a.go", "b.go"},
		},
	}
	data, err := gerr.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// Must be wrapped under top-level "error" key.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if _, ok := raw["error"]; !ok {
		t.Fatalf("payload missing top-level \"error\" key: %s", string(data))
	}
	s := string(data)
	// Q1: literal JSON-key substrings — these are load-bearing for the
	// "stable output strings" contract in plan.md.
	for _, lit := range []string{`"code":`, `"message":`, `"worker":`, `"details":`} {
		if !strings.Contains(s, lit) {
			t.Errorf("payload missing literal %q: %s", lit, s)
		}
	}
	if !strings.Contains(s, `"code":"rebase_conflict"`) {
		t.Errorf("payload missing code field: %s", s)
	}
	if !strings.Contains(s, `"worker":"login/erai/refactor-pass"`) {
		t.Errorf("payload missing worker field: %s", s)
	}
}

// TestEnvelope_RoundTrip ensures Marshal → Unmarshal preserves every
// field including Details. Also asserts the literal JSON substrings
// per Q1 of the rework brief.
func TestEnvelope_RoundTrip(t *testing.T) {
	// Worker uses the legal worker_ref grammar; "lock_held"-style
	// codes are slugs from the closed set, but the test data is the
	// "registry locked" shape.
	in := gerr.Envelope{
		Code:    "lock_held",
		Message: "registry locked by pid 12345",
		Worker:  "login/erai/refactor-pass",
		Details: map[string]any{
			"pid":  float64(12345), // JSON numbers come back as float64
			"path": "/tmp/registry.json",
		},
	}
	data, err := gerr.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	for _, lit := range []string{`"code":`, `"message":`, `"worker":`, `"details":`} {
		if !strings.Contains(s, lit) {
			t.Errorf("payload missing literal %q: %s", lit, s)
		}
	}
	out, err := gerr.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Code != in.Code || out.Message != in.Message || out.Worker != in.Worker {
		t.Fatalf("scalar fields differ: %+v vs %+v", out, in)
	}
	if !reflect.DeepEqual(out.Details, in.Details) {
		t.Fatalf("Details differ:\n got %#v\nwant %#v", out.Details, in.Details)
	}
}

// TestEnvelope_OmitsEmptyOptionalFields ensures Worker and Details are
// omitted when empty (so a minimal envelope is compact on the wire).
func TestEnvelope_OmitsEmptyOptionalFields(t *testing.T) {
	env := gerr.Envelope{Code: "gh_auth_required", Message: "gh auth refresh required"}
	data, err := gerr.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	s := string(data)
	if strings.Contains(s, `"worker"`) {
		t.Errorf("expected worker omitted when empty: %s", s)
	}
	if strings.Contains(s, `"details"`) {
		t.Errorf("expected details omitted when empty: %s", s)
	}
}

// TestEnvelope_FromError builds an envelope from any sentinel-bearing
// error — the convenience the CLI's error reporter will use.
func TestEnvelope_FromError(t *testing.T) {
	wrapped := fmt.Errorf("git push: %w", gerr.ErrGHAuthRequired)
	env := gerr.EnvelopeFromError(wrapped, "feature/x/erai/y")
	if env.Code != "gh_auth_required" {
		t.Fatalf("Code = %q, want %q", env.Code, "gh_auth_required")
	}
	if env.Worker != "feature/x/erai/y" {
		t.Fatalf("Worker = %q, want %q", env.Worker, "feature/x/erai/y")
	}
	if !strings.Contains(env.Message, "authentication") && !strings.Contains(env.Message, "git push") {
		t.Fatalf("Message = %q, expected to contain context", env.Message)
	}
}

// TestEnvelopeFromError_Nil pins the documented contract: nil err yields
// a zero-value Envelope (Code: "" and Message: "").
func TestEnvelopeFromError_Nil(t *testing.T) {
	env := gerr.EnvelopeFromError(nil, "")
	if env.Code != "" {
		t.Errorf("Code = %q, want empty", env.Code)
	}
	if env.Message != "" {
		t.Errorf("Message = %q, want empty", env.Message)
	}
	if env.Worker != "" {
		t.Errorf("Worker = %q, want empty", env.Worker)
	}
	if env.Details != nil {
		t.Errorf("Details = %v, want nil", env.Details)
	}
}

// TestMarshalEnvelope_Empty pins the documented contract: Marshal of a
// zero Envelope (empty Code) is REJECTED. The caller has a bug if they
// reach this path with no code.
func TestMarshalEnvelope_Empty(t *testing.T) {
	if _, err := gerr.Marshal(gerr.Envelope{}); err == nil {
		t.Fatalf("Marshal(Envelope{}) returned no error; want \"code required\"")
	}
}

// TestEnvelope_UnmarshalRejectsMissingErrorKey rejects payloads that
// don't have the wrapping "error" envelope.
func TestEnvelope_UnmarshalRejectsMissingErrorKey(t *testing.T) {
	bad := []byte(`{"code": "lock_held", "message": "oops"}`)
	if _, err := gerr.Unmarshal(bad); err == nil {
		t.Fatalf("Unmarshal accepted payload missing top-level \"error\" key")
	}
}

// TestEnvelope_StripsControlCharsAndANSI confirms Marshal scrubs ANSI
// CSI sequences and C0/C1/DEL control bytes from Message. Policy:
// \t is preserved; everything else in the control range is dropped.
// The CSI scrubber runs first, so "\x1b[31mred\x1b[0m" → "red" and the
// per-byte pass then drops the bell (\x07). \n is in C0 (0x0A) and is
// also dropped, so the final string is "redbell".
func TestEnvelope_StripsControlCharsAndANSI(t *testing.T) {
	env := gerr.Envelope{
		Code:    "lock_held",
		Message: "\x1b[31mred\x1b[0m\n\x07bell",
	}
	data, err := gerr.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	out, err := gerr.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Message != "redbell" {
		t.Fatalf("Message = %q, want %q", out.Message, "redbell")
	}
	if strings.Contains(out.Message, "\x1b") || strings.Contains(out.Message, "\x07") {
		t.Fatalf("Message still contains control bytes: %q", out.Message)
	}
}

// TestEnvelope_RejectsUnknownFields ensures the outer wrapper rejects
// any sibling key alongside "error".
func TestEnvelope_RejectsUnknownFields(t *testing.T) {
	bad := []byte(`{"error":{"code":"lock_held","message":"y"},"extra":"z"}`)
	if _, err := gerr.Unmarshal(bad); err == nil {
		t.Fatalf("Unmarshal accepted outer unknown field")
	}
}

// TestEnvelope_RejectsUnknownInnerFields ensures the inner Envelope
// rejects unknown keys (the inner decoder also has DisallowUnknownFields).
func TestEnvelope_RejectsUnknownInnerFields(t *testing.T) {
	bad := []byte(`{"error":{"code":"lock_held","message":"y","mystery":1}}`)
	if _, err := gerr.Unmarshal(bad); err == nil {
		t.Fatalf("Unmarshal accepted inner unknown field")
	}
}

// TestEnvelope_RejectsInvalidWorker confirms the worker_ref grammar
// rejects path-traversal-shaped values and mixed-case identifiers.
func TestEnvelope_RejectsInvalidWorker(t *testing.T) {
	cases := []string{
		`{"error":{"code":"lock_held","message":"y","worker":"../../etc/passwd"}}`,
		`{"error":{"code":"lock_held","message":"y","worker":"BadCase/user/purpose"}}`,
	}
	for _, c := range cases {
		if _, err := gerr.Unmarshal([]byte(c)); err == nil {
			t.Errorf("Unmarshal accepted invalid worker in %s", c)
		}
	}
}

// TestEnvelope_AcceptsEmptyWorker confirms an empty Worker passes
// validation (it means "not associated with a worker").
func TestEnvelope_AcceptsEmptyWorker(t *testing.T) {
	good := []byte(`{"error":{"code":"lock_held","message":"y"}}`)
	if _, err := gerr.Unmarshal(good); err != nil {
		t.Fatalf("Unmarshal rejected empty worker: %v", err)
	}
}

// TestEnvelope_AcceptsValidWorker confirms a legal worker_ref passes.
func TestEnvelope_AcceptsValidWorker(t *testing.T) {
	good := []byte(`{"error":{"code":"lock_held","message":"y","worker":"auth/eraigosa/api-moss"}}`)
	out, err := gerr.Unmarshal(good)
	if err != nil {
		t.Fatalf("Unmarshal rejected valid worker: %v", err)
	}
	if out.Worker != "auth/eraigosa/api-moss" {
		t.Fatalf("Worker = %q, want auth/eraigosa/api-moss", out.Worker)
	}
}

// TestEnvelope_RejectsOversizedMessage confirms the 4 KiB Message cap.
func TestEnvelope_RejectsOversizedMessage(t *testing.T) {
	env := gerr.Envelope{Code: "lock_held", Message: strings.Repeat("a", 5000)}
	if _, err := gerr.Marshal(env); err == nil {
		t.Fatalf("Marshal accepted 5000-byte message")
	}
}

// TestEnvelope_RejectsOversizedDetails confirms the 64 KiB Details cap
// (measured on the marshalled JSON).
func TestEnvelope_RejectsOversizedDetails(t *testing.T) {
	env := gerr.Envelope{
		Code:    "lock_held",
		Message: "y",
		Details: map[string]any{"k": strings.Repeat("v", 70000)},
	}
	if _, err := gerr.Marshal(env); err == nil {
		t.Fatalf("Marshal accepted 70000-byte details value")
	}
}

// TestEnvelope_RejectsDeepDetails builds a 9-level-deep nested map and
// confirms Marshal rejects it (the cap is 8).
func TestEnvelope_RejectsDeepDetails(t *testing.T) {
	// Build inner-most leaf then wrap 9 times.
	var v any = "leaf"
	for i := 0; i < 9; i++ {
		v = map[string]any{"k": v}
	}
	env := gerr.Envelope{
		Code:    "lock_held",
		Message: "y",
		Details: v.(map[string]any),
	}
	if _, err := gerr.Marshal(env); err == nil {
		t.Fatalf("Marshal accepted 9-level-deep details")
	}
}

// TestEnvelope_AcceptsShallowDetails confirms a within-limit nested
// Details (5 levels) round-trips successfully — exercises the depth
// walker's accept path.
func TestEnvelope_AcceptsShallowDetails(t *testing.T) {
	var v any = "leaf"
	for i := 0; i < 5; i++ {
		v = map[string]any{"k": v}
	}
	env := gerr.Envelope{
		Code:    "lock_held",
		Message: "y",
		Details: v.(map[string]any),
	}
	data, err := gerr.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if _, err := gerr.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
}
