package hooklog

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func envOf(m map[string]string) Env { return func(k string) string { return m[k] } }

func TestPath(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"GSS_HOOK_LOG": "/x/log", "HOME": "/h"}, "/x/log"},
		{map[string]string{"XDG_STATE_HOME": "/s", "HOME": "/h"}, "/s/githooks/runs.jsonl"},
		{map[string]string{"HOME": "/h"}, "/h/.local/state/githooks/runs.jsonl"},
	}
	for _, c := range cases {
		if got := Path(envOf(c.env)); got != c.want {
			t.Errorf("Path(%v) = %q; want %q", c.env, got, c.want)
		}
	}
}

func TestBuild(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	env := envOf(map[string]string{"HOME": "/h"})
	cases := []struct {
		status, result string
		exit           int
		hasExit        bool
	}{
		{"0", Pass, 0, true},
		{"1", Refused, 1, true},
		{"chained", Chained, 0, false},
	}
	for _, c := range cases {
		e, err := Build(now, "box.lan", "/h/src/repo", "pre-push", c.status, env)
		if err != nil {
			t.Fatalf("Build(%s): %v", c.status, err)
		}
		if e.Result != c.result || (e.Exit != nil) != c.hasExit || (c.hasExit && *e.Exit != c.exit) {
			t.Errorf("Build(%s) = %+v", c.status, e)
		}
		if e.Time != "2026-01-02T03:04:05Z" || e.Host != "box" || e.Repo != "~/src/repo" || e.Hook != "pre-push" || e.Skip {
			t.Errorf("Build(%s) fields = %+v", c.status, e)
		}
	}
	if _, err := Build(now, "h", "/r", "pre-push", "nope", env); err == nil {
		t.Error("a non-numeric status must be an error")
	}
	if _, err := Build(now, "h", "/r", "", "0", env); err == nil {
		t.Error("an empty hook name must be an error")
	}
	e, _ := Build(now, "h", "/r", "pre-commit", "0", envOf(map[string]string{"PRIVACY_GUARD_SKIP": "1"}))
	if !e.Skip {
		t.Error("PRIVACY_GUARD_SKIP must be recorded as skip")
	}
}

func TestAppendWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "runs.jsonl")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, s := range []string{"0", "1", "chained"} {
		e, err := Build(now, "h", "/r", "commit-msg", s, envOf(nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := Append(path, e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var results []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		results = append(results, e.Result)
	}
	if got := strings.Join(results, ","); got != "pass,refused,chained" {
		t.Errorf("results = %s", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("log mode = %v; want 0600", st.Mode().Perm())
	}
}

func TestAppendTrimsPastMax(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	big := strings.Repeat(`{"pad":"`+strings.Repeat("x", 1000)+`"}`+"\n", MaxBytes/1000+10)
	if err := os.WriteFile(path, []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	e, _ := Build(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "h", "/r", "pre-push", "0", envOf(nil))
	if err := Append(path, e); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if len(raw) > MaxBytes {
		t.Errorf("log not trimmed: %d bytes", len(raw))
	}
	if !strings.HasSuffix(string(raw), "\"hook\":\"pre-push\",\"result\":\"pass\",\"exit\":0}\n") {
		t.Errorf("newest line lost by the trim: %q", string(raw[len(raw)-120:]))
	}
}

func TestNewestWithin(t *testing.T) {
	raw := []byte("aaaa\nbb\ncc\n")
	if got := string(newestWithin(raw, 6)); got != "bb\ncc\n" {
		t.Errorf("newestWithin(6) = %q", got)
	}
	if got := string(newestWithin(raw, 100)); got != string(raw) {
		t.Errorf("newestWithin(100) = %q", got)
	}
	// One line longer than the limit: nothing is kept, so the next append
	// does not trim again.
	if got := newestWithin([]byte(strings.Repeat("x", 50)+"\n"), 10); len(got) != 0 {
		t.Errorf("an over-long line was kept: %d bytes", len(got))
	}
}

// TestAppendConcurrentKeepsEveryLine: runs that append while the file
// crosses MaxBytes lose no line to a trim.
func TestAppendConcurrentKeepsEveryLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.jsonl")
	pad := strings.Repeat(`{"pad":"`+strings.Repeat("x", 1000)+`"}`+"\n", MaxBytes/1010)
	if err := os.WriteFile(path, []byte(pad), 0o600); err != nil {
		t.Fatal(err)
	}
	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e, _ := Build(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), "h", "/r", "hook-"+strconv.Itoa(i), "0", envOf(nil))
			if err := Append(path, e); err != nil {
				t.Errorf("Append %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	raw, _ := os.ReadFile(path)
	for i := 0; i < n; i++ {
		if !strings.Contains(string(raw), `"hook":"hook-`+strconv.Itoa(i)+`"`) {
			t.Errorf("line from run %d lost", i)
		}
	}
	if len(raw) > MaxBytes {
		t.Errorf("file not trimmed: %d bytes", len(raw))
	}
}
