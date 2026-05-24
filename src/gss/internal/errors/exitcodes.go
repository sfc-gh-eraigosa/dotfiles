package errors

import stderrors "errors"

// Exit code contract.
//
// These integers are a stable public surface: tmux-mgr's pane-close
// hook, slash commands, and the worker-mode JSON consumers all switch
// on them. Reserved range is 10–29 (one decade per the design's error
// table; the first decade carries the original PR-01 sentinels, the
// second carries the hardening rework set added in the PR-01 rework
// pass). 0 means success; 1 is the catch-all for unknown errors.
//
// Changing any value here is a breaking change to the gss CLI contract
// and must go through the rework protocol in plan.md.
const (
	ExitOK             = 0
	ExitUnknown        = 1
	ExitGHAuthRequired = 10
	ExitRebaseConflict = 11
	ExitDirtyWorktree  = 12
	ExitLockHeld       = 13
	ExitRegistryStale  = 14
	ExitNotInWorker    = 15
	ExitPRReadyNeeds   = 16
	ExitInvalidIdent   = 17
	ExitWrongMode      = 18
	ExitConflictMarker = 19

	// Rework-pass additions. The ordering matches the four new
	// sentinels declared in errors.go.
	ExitSchemaMismatch       = 20
	ExitPermissionMode       = 21
	ExitApprovalTokenMissing = 22
	ExitMarkerInjection      = 23
)

// Codes is the sentinel → exit-code lookup. It is exported so tests
// can audit completeness and so external integrators (linting,
// docs-gen) can introspect the contract.
var Codes = map[error]int{
	ErrGHAuthRequired:       ExitGHAuthRequired,
	ErrRebaseConflict:       ExitRebaseConflict,
	ErrDirtyWorktree:        ExitDirtyWorktree,
	ErrLockHeld:             ExitLockHeld,
	ErrRegistryStale:        ExitRegistryStale,
	ErrNotInWorker:          ExitNotInWorker,
	ErrPRReadyNeedsToken:    ExitPRReadyNeeds,
	ErrInvalidIdent:         ExitInvalidIdent,
	ErrWrongMode:            ExitWrongMode,
	ErrConflictMarker:       ExitConflictMarker,
	ErrSchemaMismatch:       ExitSchemaMismatch,
	ErrPermissionMode:       ExitPermissionMode,
	ErrApprovalTokenMissing: ExitApprovalTokenMissing,
	ErrMarkerInjection:      ExitMarkerInjection,
}

// ExitCode walks err's wrap chain looking for a known sentinel and
// returns its mapped code. Returns ExitOK for nil, ExitUnknown for an
// error that carries no gss sentinel.
//
// Callers use this at the top of main():
//
//	if err := run(); err != nil {
//	    fmt.Fprintln(os.Stderr, err)
//	    os.Exit(errors.ExitCode(err))
//	}
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	for sent, code := range Codes {
		if stderrors.Is(err, sent) {
			return code
		}
	}
	return ExitUnknown
}
