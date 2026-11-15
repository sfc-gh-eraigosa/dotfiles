package providers

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/lpass"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/vault/op"
)

func TestInteractiveWiresStdio(t *testing.T) {
	var out bytes.Buffer
	if err := Interactive(context.Background(), strings.NewReader("hi"), &out, io.Discard, "cat"); err != nil || out.String() != "hi" {
		t.Fatalf("out=%q err=%v", out.String(), err)
	}
}

// The vendor sign-in child must receive the caller's *os.File itself: exec
// then hands the child the fd, instead of copying through a goroutine+pipe
// that keeps fleet blocked on a read after the child exits.
func TestOpenWiresTheSharedRunnerAndPassesTheFileThrough(t *testing.T) {
	t.Setenv("LPASS_USER", "me@example.com")
	f, err := os.Create(filepath.Join(t.TempDir(), "tty"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, p := range []string{"lpass", "op"} {
		s, err := Open(vault.Config{Provider: p}, SystemExec)
		if err != nil {
			t.Fatal(err)
		}
		var got io.Reader
		rec := func(_ context.Context, in io.Reader, _, _ io.Writer, _ string, _ ...string) error {
			got = in
			return nil
		}
		switch st := s.(type) {
		case *lpass.Store:
			if st.Interactive == nil {
				t.Fatal("lpass: no interactive runner wired")
			}
			st.Interactive = rec
		case *op.Store:
			if st.Interactive == nil {
				t.Fatal("op: no interactive runner wired")
			}
			st.Interactive = rec
		}
		if err := s.SignIn(context.Background(), f, io.Discard, io.Discard); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if g, ok := got.(*os.File); !ok || g != f {
			t.Fatalf("%s: runner got %T, want the *os.File", p, got)
		}
	}
}
