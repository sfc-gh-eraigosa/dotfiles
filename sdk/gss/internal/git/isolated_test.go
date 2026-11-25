package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunIsolatedUsesOnlyItsEnvAndDir(t *testing.T) {
	t.Setenv("LEAK", "yes")
	bin := filepath.Join(t.TempDir(), "probe")
	script := "#!/bin/sh\n[ -z \"$LEAK\" ] || exit 3\n[ \"$ONLY\" = 1 ] || exit 4\n[ \"$(pwd -P)\" = / ] || exit 5\n[ \"$*\" = \"a b\" ] || exit 6\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunIsolated(bin, "/", []string{"ONLY=1", "PATH=/usr/bin:/bin"}, "a", "b"); err != nil {
		t.Fatalf("RunIsolated: %v", err)
	}
	if err := RunIsolated(bin, "/", []string{"PATH=/usr/bin:/bin"}, "a", "b"); err == nil {
		t.Fatal("RunIsolated passed without ONLY=1: the check is not running")
	}
}

func TestRunIsolatedNilEnvInheritsNothing(t *testing.T) {
	t.Setenv("LEAK", "yes")
	bin := filepath.Join(t.TempDir(), "probe")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n[ -z \"$LEAK\" ]\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunIsolated(bin, "/", nil); err != nil {
		t.Fatalf("nil env leaked gss's environment: %v", err)
	}
}
