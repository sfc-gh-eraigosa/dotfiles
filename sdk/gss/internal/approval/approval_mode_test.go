package approval_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/approval"
)

// TestIssue_PrivateDirMode: the approval token lives in a 0700 directory
// (security-scan-green, G301); the token file itself was already 0600.
func TestIssue_PrivateDirMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gss", "approval.token")
	v := approval.NewVerifier(path, gitAtHead())
	if err := v.Issue(t.Context(), "."); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	fi, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("token dir mode = %o; want 0700", got)
	}
}
