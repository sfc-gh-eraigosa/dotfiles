package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func runArgv(t *testing.T, in string) (string, int) {
	t.Helper()
	code := -1
	hooksExit = func(c int) { code = c }
	t.Cleanup(func() { hooksExit = os.Exit })
	var out bytes.Buffer
	hooksArgvCmd.SetIn(strings.NewReader(in))
	hooksArgvCmd.SetOut(&out)
	t.Cleanup(func() { hooksArgvCmd.SetIn(nil); hooksArgvCmd.SetOut(nil) })
	if err := hooksArgvCmd.RunE(hooksArgvCmd, nil); err != nil {
		t.Fatal(err)
	}
	return out.String(), code
}

func TestHooksArgv(t *testing.T) {
	if out, code := runArgv(t, "g\\\nss push; bash -c 'gff set k v'"); code != -1 || out != "gss push\nbash -c gff_set_k_v\ngff set k v\n" {
		t.Fatalf("got exit %d, %q", code, out)
	}
	if _, code := runArgv(t, `echo "open`); code != 2 {
		t.Fatalf("unparsable: exit %d, want 2", code)
	}
	argvFormat = "json"
	if _, code := runArgv(t, "gss push"); code != 2 {
		t.Fatalf("unknown format: exit %d, want 2", code)
	}
	argvFormat = "lines"
	if _, code := runArgv(t, strings.Repeat("x", maxArgvInput+1)); code != 2 {
		t.Fatalf("over the limit: exit %d, want 2", code)
	}
}
