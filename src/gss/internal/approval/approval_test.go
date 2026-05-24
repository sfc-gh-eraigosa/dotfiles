// Package approval_test verifies the HEAD-bound approval token per
// src/gss/docs/plan.md PR-10: verify-then-consume, missing/mismatch
// rejection wrapping errors.ErrApprovalTokenMissing, and the
// --force-autonomous bypass. Semantics mirror the classic cmd/push.go
// handshake.
package approval_test

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/approval"
	"github.com/wenlock/dotfiles/gss/internal/errors"
	gitfake "github.com/wenlock/dotfiles/gss/internal/git/fake"
)

const headSHA = "abc123def4567890"

// gitAtHead returns a fake git Runner whose `rev-parse HEAD` yields headSHA.
func gitAtHead() *gitfake.Runner {
	return &gitfake.Runner{Default: gitfake.Response{Stdout: []byte(headSHA + "\n")}}
}

func tokenPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "approval.token")
}

func writeToken(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
}

func TestVerify_ForceAutonomousBypass(t *testing.T) {
	v := approval.NewVerifier(tokenPath(t), gitAtHead())
	if err := v.Verify(t.Context(), ".", true); err != nil {
		t.Errorf("force-autonomous Verify = %v; want nil bypass", err)
	}
}

func TestVerify_MissingToken(t *testing.T) {
	v := approval.NewVerifier(tokenPath(t), gitAtHead())
	err := v.Verify(t.Context(), ".", false)
	if err == nil {
		t.Fatal("missing token: err = nil; want rejection")
	}
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Errorf("err = %v; want wrapping ErrApprovalTokenMissing", err)
	}
	var ae *approval.Error
	if !stderrors.As(err, &ae) || ae.Reason != approval.ReasonMissing {
		t.Errorf("err = %v; want *approval.Error{ReasonMissing}", err)
	}
}

func TestVerify_MissingToken_HeadUnresolvable(t *testing.T) {
	// Guardrail robustness: a missing token must surface as ReasonMissing
	// even when HEAD cannot be resolved (e.g. cwd is not a git repo). The
	// token-existence check must short-circuit before headSHA so the
	// "you need approval" signal isn't masked by an unrelated git error.
	failHead := &gitfake.Runner{Default: gitfake.Response{
		Stderr: []byte("fatal: not a git repository"),
		Err:    stderrors.New("exit status 128"),
	}}
	v := approval.NewVerifier(tokenPath(t), failHead) // dir exists; token file does not

	err := v.Verify(t.Context(), ".", false)
	if err == nil {
		t.Fatal("missing token (HEAD unresolvable): err = nil; want rejection")
	}
	var ae *approval.Error
	if !stderrors.As(err, &ae) || ae.Reason != approval.ReasonMissing {
		t.Fatalf("err = %v; want *approval.Error{ReasonMissing} (token check must precede HEAD)", err)
	}
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Errorf("err = %v; want wrapping ErrApprovalTokenMissing", err)
	}
	// Prove the short-circuit: headSHA must not have been called.
	if failHead.CallCount() != 0 {
		t.Errorf("headSHA called %d times; want 0 (token check should short-circuit)", failHead.CallCount())
	}
}

func TestVerify_Mismatch(t *testing.T) {
	path := tokenPath(t)
	writeToken(t, path, "STALEsha\n")
	v := approval.NewVerifier(path, gitAtHead())

	err := v.Verify(t.Context(), ".", false)
	if err == nil {
		t.Fatal("mismatch: err = nil; want rejection")
	}
	if !stderrors.Is(err, errors.ErrApprovalTokenMissing) {
		t.Errorf("err = %v; want wrapping ErrApprovalTokenMissing", err)
	}
	var ae *approval.Error
	if !stderrors.As(err, &ae) || ae.Reason != approval.ReasonMismatch {
		t.Fatalf("err = %v; want *approval.Error{ReasonMismatch}", err)
	}
	if ae.Expected != headSHA {
		t.Errorf("Expected = %q; want %q", ae.Expected, headSHA)
	}
	// A rejected token must NOT be consumed.
	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("token consumed on mismatch; want it left in place: %v", statErr)
	}
}

func TestVerify_SuccessConsumesToken(t *testing.T) {
	path := tokenPath(t)
	writeToken(t, path, headSHA+"\n")
	v := approval.NewVerifier(path, gitAtHead())

	if err := v.Verify(t.Context(), ".", false); err != nil {
		t.Fatalf("Verify(valid): %v", err)
	}
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("valid token not consumed; stat err = %v (want not-exist)", statErr)
	}
}

func TestIssueThenVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "approval.token")
	v := approval.NewVerifier(path, gitAtHead())

	if err := v.Issue(t.Context(), "."); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if err := v.Verify(t.Context(), ".", false); err != nil {
		t.Errorf("Verify after Issue: %v; want nil", err)
	}
}
