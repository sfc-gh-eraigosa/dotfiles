package providertest

import (
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

var (
	stubOnce sync.Once
	stubPath string
	stubErr  error
)

// BuildStub compiles the scriptable stub process once per test binary and
// returns its path. Later calls return the same path without rebuilding.
//
// The stub is deliberately protocol-AGNOSTIC: leaf A owns no wire format, so
// the stub is a line-oriented process whose replies are canned by the caller
// (-reply) and whose failure modes are the ones a protocol consumer must
// survive — sleeping past a deadline, exiting at once, writing half a line,
// noise on stderr. Leaf B supplies the JSON; it needs no change here.
func BuildStub(t *testing.T) string {
	t.Helper()
	stubOnce.Do(func() {
		dir := t.TempDir()
		stubPath = filepath.Join(dir, "stubplugin")
		out, err := exec.Command("go", "build", "-o", stubPath,
			"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider/providertest/stubplugin").CombinedOutput()
		if err != nil {
			stubErr = err
			t.Logf("go build stubplugin: %s", out)
		}
	})
	if stubErr != nil {
		t.Fatalf("building the stub plugin: %v", stubErr)
	}
	return stubPath
}
