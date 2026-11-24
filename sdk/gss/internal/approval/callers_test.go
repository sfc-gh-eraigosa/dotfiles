package approval

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every gated verb builds its Verifier with NewVerifier, which wires in the
// switch. A struct literal outside this package would leave Required nil and
// demand a token even with the switch off.
func TestCallersUseNewVerifier(t *testing.T) {
	root := filepath.Join("..", "..")
	literal := regexp.MustCompile(`approval\.Verifier\s*\{`)
	calls := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if literal.Match(b) {
			t.Errorf("%s builds approval.Verifier directly; use approval.NewVerifier", p)
		}
		calls += strings.Count(string(b), "approval.NewVerifier(")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// push, pr and the feature service (pr --ready, merged, restack).
	if calls < 3 {
		t.Fatalf("approval.NewVerifier called %d times, want at least 3 (push, pr, feature service)", calls)
	}
}
