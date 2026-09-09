package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// mustWriteFile writes a test fixture and fails the test if it cannot. An
// unchecked fixture write lets the assertions below run against a file that
// was never created, turning a setup failure into a confusing assertion error.
func mustWriteFile(t *testing.T, path string, data []byte, perm os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// Adopting a host that is ALREADY in ~/.ssh/config must not require re-typing
// its connection details — that was the whole friction. resolveAdd marks the
// existing block in place and keeps its directives.
func TestResolveAddAdoptsExistingHostWithoutHostname(t *testing.T) {
	cfg := "Host box\n    HostName 10.0.0.7\n    User ops\n"
	next, act, err := resolveAdd(cfg, sshconf.Host{Alias: "box"}, "#fleet")
	if err != nil {
		t.Fatalf("resolveAdd: %v", err)
	}
	if act != actionAdopted {
		t.Fatalf("expected actionAdopted, got %v", act)
	}
	hosts, _ := sshconf.Parse(next, "#fleet")
	if len(hosts) != 1 || !hosts[0].Fleet || hosts[0].HostName != "10.0.0.7" || hosts[0].User != "ops" {
		t.Fatalf("adoption lost fields or marker: %+v\n%s", hosts, next)
	}
}

// Creating a genuinely new host still needs a hostname — you can't write a
// usable Host block without one. The error must point the user at discovery.
func TestResolveAddCreateRequiresHostname(t *testing.T) {
	_, _, err := resolveAdd("", sshconf.Host{Alias: "ghost"}, "#fleet")
	if err == nil {
		t.Fatal("expected an error creating a new host with no hostname")
	}
	if !strings.Contains(err.Error(), "discover") {
		t.Fatalf("error should mention `discover`: %v", err)
	}
}

func TestResolveAddCreatesNewHostWithHostname(t *testing.T) {
	next, act, err := resolveAdd("", sshconf.Host{Alias: "web", HostName: "10.0.0.9"}, "#fleet")
	if err != nil {
		t.Fatalf("resolveAdd: %v", err)
	}
	if act != actionCreated {
		t.Fatalf("expected actionCreated, got %v", act)
	}
	hosts, _ := sshconf.Parse(next, "#fleet")
	if len(hosts) != 1 || !hosts[0].Fleet || hosts[0].HostName != "10.0.0.9" {
		t.Fatalf("create did not produce a marked host: %+v\n%s", hosts, next)
	}
}

func TestResolveAddAlreadyInFleetIsNoOp(t *testing.T) {
	cfg := "Host box  #fleet\n    HostName 10.0.0.7\n"
	next, act, err := resolveAdd(cfg, sshconf.Host{Alias: "box"}, "#fleet")
	if err != nil {
		t.Fatalf("resolveAdd: %v", err)
	}
	if act != actionAlready {
		t.Fatalf("expected actionAlready, got %v", act)
	}
	if next != cfg {
		t.Fatalf("no-op must not change the config:\n%s", next)
	}
}

func TestWriteConfigTakesBackupFirst(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	orig := "Host beta\n    HostName 10.0.0.2\n"
	if err := os.WriteFile(p, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeConfig(p, "Host beta\n    HostName 10.0.0.9\n"); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.bak-") {
			backups = append(backups, e.Name())
		}
	}
	if len(backups) != 1 {
		t.Fatalf("expected exactly 1 backup, got %v", backups)
	}
	got, _ := os.ReadFile(filepath.Join(dir, backups[0]))
	if string(got) != orig {
		t.Fatalf("backup does not hold the ORIGINAL content:\n%s", got)
	}
	now, _ := os.ReadFile(p)
	if !strings.Contains(string(now), "10.0.0.9") {
		t.Fatal("new content not written")
	}
}

func TestWriteConfigKeepsOwnerOnlyPermissions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	mustWriteFile(t, p, []byte("Host a\n"), 0o600)
	if err := writeConfig(p, "Host a\n    HostName 1\n"); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("ssh config must stay 0600, got %o", fi.Mode().Perm())
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	orig := "Host beta\n    HostName 10.0.0.2\n"
	mustWriteFile(t, p, []byte(orig), 0o600)

	var sb strings.Builder
	if err := applyConfig(&sb, p, "Host beta\n    HostName 10.0.0.9\n", true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != orig {
		t.Fatal("--dry-run modified the file")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("--dry-run created files: %v", entries)
	}
	if !strings.Contains(sb.String(), "10.0.0.9") {
		t.Fatalf("--dry-run must show the resulting content:\n%s", sb.String())
	}
}

func TestWriteConfigCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config")
	if err := writeConfig(p, "Host a\n"); err != nil {
		t.Fatalf("writeConfig on missing file: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("no backup should be made for a new file, got %v", entries)
	}
}
