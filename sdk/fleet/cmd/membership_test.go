package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	os.WriteFile(p, []byte("Host a\n"), 0o600)
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
	os.WriteFile(p, []byte(orig), 0o600)

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
