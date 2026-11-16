package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestReadBodyFrom(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "body.md")
	if err := os.WriteFile(good, []byte("## What\n\nnew\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.md")
	if err := os.WriteFile(empty, []byte(" \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &cobra.Command{}

	if got, err := readBodyFrom(c, ""); err != nil || got != "" {
		t.Errorf("unset: got %q, %v; want empty, nil", got, err)
	}
	if got, err := readBodyFrom(c, good); err != nil || !strings.Contains(got, "new") {
		t.Errorf("file: got %q, %v", got, err)
	}
	if _, err := readBodyFrom(c, empty); err == nil {
		t.Error("an empty file must be refused, not blank the PR body")
	}
	if _, err := readBodyFrom(c, filepath.Join(dir, "missing.md")); err == nil {
		t.Error("a missing file must be an error")
	}
	c.SetIn(strings.NewReader("from stdin\n"))
	if got, err := readBodyFrom(c, "-"); err != nil || got != "from stdin\n" {
		t.Errorf("stdin: got %q, %v", got, err)
	}
}

func TestCheckpointBodyFlagsRegistered(t *testing.T) {
	for _, name := range []string{"body-from", "refresh-body"} {
		if featureCheckpointCmd.Flags().Lookup(name) == nil {
			t.Errorf("checkpoint has no --%s flag", name)
		}
	}
}
