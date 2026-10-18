package flags

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

var allOn = Links{Enabled: true, Repo: true, DirGit: true, AI: true, Time: true,
	Sections: map[string]bool{"host": true, "dirgit": true, "repo": true, "ai": true, "time": true, "video": true}}

func TestResolve_AllTrueByDefault(t *testing.T) {
	got := Resolve(context.Background(), func(string) (bool, error) { return true, nil })
	if !reflect.DeepEqual(got, allOn) {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_FalseFlagWins(t *testing.T) {
	look := func(k string) (bool, error) { return k != KeyTime, nil }
	got := Resolve(context.Background(), look)
	if got.Time || !got.Enabled || !got.Repo || !got.DirGit || !got.AI {
		t.Errorf("got %+v", got)
	}
}

func TestResolve_ErrorIsFailOpen(t *testing.T) {
	look := func(string) (bool, error) { return false, errors.New("unknown source") }
	if got := Resolve(context.Background(), look); !reflect.DeepEqual(got, allOn) {
		t.Errorf("errors must fail open: %+v", got)
	}
}

func TestResolve_SlowLookupIsFailOpenAndBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	look := func(string) (bool, error) { time.Sleep(200 * time.Millisecond); return false, nil }
	start := time.Now()
	got := Resolve(ctx, look)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond || !reflect.DeepEqual(got, allOn) {
		t.Errorf("slow lookup: took %v, got %+v", elapsed, got)
	}
}

func TestResolve_NilLookupIsAllTrue(t *testing.T) {
	if got := Resolve(context.Background(), nil); !reflect.DeepEqual(got, allOn) {
		t.Errorf("nil lookup must fail open: %+v", got)
	}
}

func TestResolve_AsksEveryKeyOnce(t *testing.T) {
	seen := make(chan string, 20)
	Resolve(context.Background(), func(k string) (bool, error) { seen <- k; return true, nil })
	close(seen)
	got := map[string]int{}
	for k := range seen {
		got[k]++
	}
	for _, k := range []string{KeyEnabled, KeyRepo, KeyDirGit, KeyAI, KeyTime, KeySectionHost, KeySectionDirGit, KeySectionRepo, KeySectionAI, KeySectionTime, KeySectionVideo} {
		if got[k] != 1 {
			t.Errorf("key %s looked up %d times, want 1", k, got[k])
		}
	}
}

func TestGFFLookup_UnregisteredSourceFallsBackToDotfilesDir(t *testing.T) {
	// No source is registered in a fresh HOME; with DOTFILES_DIR unset the
	// lookup must report an error (which Resolve treats as "on"), never panic.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	look := GFFLookup(func(string) string { return "" })
	if _, err := look(KeyEnabled); err == nil {
		t.Error("unregistered namespace with no DOTFILES_DIR: want an error")
	}
}

func TestResolve_SectionFlags(t *testing.T) {
	tests := []struct {
		name string
		look Lookup
		off  []string
	}{
		{"video off", func(k string) (bool, error) { return k != KeySectionVideo, nil }, []string{"video"}},
		{"host+time off", func(k string) (bool, error) { return k != KeySectionHost && k != KeySectionTime, nil }, []string{"host", "time"}},
		{"errors fail open", func(string) (bool, error) { return false, errors.New("x") }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(context.Background(), tt.look).SectionOff()
			if len(got) != len(tt.off) {
				t.Fatalf("SectionOff = %v, want %v", got, tt.off)
			}
			for _, o := range tt.off {
				if !got[o] {
					t.Errorf("%s should be off: %v", o, got)
				}
			}
		})
	}
}
