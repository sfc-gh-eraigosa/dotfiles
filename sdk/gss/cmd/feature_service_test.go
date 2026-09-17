package cmd

import (
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/errors"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/feature"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/registry"
)

// seedEmptyRegistry points GSS_REGISTRY_DIR at a fresh, empty registry.json
// under root, so currentWorkerRef's IsInWorker lookup always misses — the
// "no matching row at all" half of every case below.
func seedEmptyRegistry(t *testing.T, root string) {
	t.Helper()
	regDir := filepath.Join(root, "gss-registry")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := registry.NewStore(filepath.Join(regDir, "registry.json")).Update(func(r *registry.Registry) error {
		*r = registry.Registry{SchemaVersion: 1}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GSS_REGISTRY_DIR", regDir)
}

// TestCurrentWorkerRef_OrphanedWorkerMDNamesRecovery pins the dotfiles#336
// fix: when cwd is a former worker root whose registry row is gone but its
// WORKER.md survives, the error must name the missing row and the recovery
// command instead of the bare, misleading "command not valid in this mode"
// (the same text used for the unrelated "classic verb inside a worker"
// case, which actively points at the wrong diagnosis).
func TestCurrentWorkerRef_OrphanedWorkerMDNamesRecovery(t *testing.T) {
	root := t.TempDir()
	seedEmptyRegistry(t, root)

	wt := filepath.Join(root, "wt", "auth", "erai", "api")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := feature.WorkerMetaPath(wt)
	if err := os.MkdirAll(filepath.Dir(meta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(meta, []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wt)

	_, err := currentWorkerRef()
	if !stderrors.Is(err, errors.ErrWrongMode) {
		t.Fatalf("err = %v; want it to still wrap ErrWrongMode (same exit code as before)", err)
	}
	if !strings.Contains(err.Error(), "no matching registry row") {
		t.Errorf("err = %v; want a self-describing message naming the missing row", err)
	}
	if !strings.Contains(err.Error(), "gss feature worker add") {
		t.Errorf("err = %v; want it to name the recovery command", err)
	}
}

// TestCurrentWorkerRef_PlainWrongMode confirms the ORIGINAL bare message
// still applies when cwd has no trace of ever being a worker at all — the
// improved message must fire only for the specific orphaned-row case, not
// for every ErrWrongMode.
func TestCurrentWorkerRef_PlainWrongMode(t *testing.T) {
	root := t.TempDir()
	seedEmptyRegistry(t, root)
	t.Chdir(root) // an ordinary directory, never a worker

	_, err := currentWorkerRef()
	if !stderrors.Is(err, errors.ErrWrongMode) {
		t.Fatalf("err = %v; want ErrWrongMode", err)
	}
	if strings.Contains(err.Error(), "gss feature worker add") {
		t.Errorf("err = %v; a plain non-worker directory must get the ORIGINAL bare message, not the orphaned-row one", err)
	}
}
