package report

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/engine"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
)

// sample is a report with one of every kind, so a renderer change shows up
// in the goldens.
func sample() engine.Report {
	findings := []family.Finding{
		{Family: "general", Key: "merge.delete_branch_on_merge", Kind: family.Drift, Want: true, Live: false},
		{Family: "general", Key: "description", Kind: family.Drift, Want: "dotfiles and lab tooling", Live: "old text"},
		{Family: "labels", Key: "chore", Kind: family.Unmanaged, Live: "chore"},
		{Family: "security", Key: "non_provider_patterns", Kind: family.NotHonoured, Want: true, Live: false,
			Reason: "GitHub accepted the write but the setting stayed off — non-provider patterns need GitHub Secret Protection on this plan"},
		{Family: "actions", Key: "*", Kind: family.Unreadable,
			Reason: "GET /repos/o/r/actions/permissions: HTTP 403 Resource not accessible by personal access token (needs repo:Administration:write)"},
	}
	counts := map[family.Kind]int{}
	for _, f := range findings {
		counts[f.Kind]++
	}
	return engine.Report{Target: "sfc-gh-eraigosa/dotfiles", Families: 5, Findings: findings, Counts: counts}
}

func clean() engine.Report {
	return engine.Report{Target: "sfc-gh-eraigosa/dotfiles", Families: 5, Counts: map[family.Kind]int{}}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s drifted (regenerate with UPDATE_GOLDEN=1 go test ./internal/report/):\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestTTYGolden(t *testing.T) {
	var b bytes.Buffer
	if err := TTY(&b, sample(), Options{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	golden(t, "tty.golden", b.Bytes())
}

func TestTTYCleanGolden(t *testing.T) {
	var b bytes.Buffer
	if err := TTY(&b, clean(), Options{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	golden(t, "tty-clean.golden", b.Bytes())
}

func TestMarkdownGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, sample()); err != nil {
		t.Fatal(err)
	}
	golden(t, "markdown.golden", b.Bytes())
}

func TestMarkdownCleanGolden(t *testing.T) {
	var b bytes.Buffer
	if err := Markdown(&b, clean()); err != nil {
		t.Fatal(err)
	}
	golden(t, "markdown-clean.golden", b.Bytes())
}

func TestJSONGolden(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sample()); err != nil {
		t.Fatal(err)
	}
	golden(t, "json.golden", b.Bytes())
}

// The JSON shape is a contract (plan §3.3: "all --json outputs are stable,
// documented shapes"), so it is asserted structurally too.
func TestJSONShapeIsStable(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, sample()); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Target   string         `json:"target"`
		Clean    bool           `json:"clean"`
		Families int            `json:"families"`
		Counts   map[string]int `json:"counts"`
		Findings []struct {
			Family string `json:"family"`
			Key    string `json:"key"`
			Kind   string `json:"kind"`
			Want   any    `json:"want"`
			Live   any    `json:"live"`
			Reason string `json:"reason"`
		} `json:"findings"`
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b.String())
	}
	if got.Target != "sfc-gh-eraigosa/dotfiles" || got.Clean || got.Families != 5 {
		t.Errorf("head = %+v", got)
	}
	if len(got.Findings) != 5 {
		t.Fatalf("findings = %d", len(got.Findings))
	}
	if got.Findings[0].Kind != "drift" || got.Findings[2].Kind != "unmanaged" ||
		got.Findings[3].Kind != "not_honoured" || got.Findings[4].Kind != "unreadable" {
		t.Errorf("kinds must be names, not numbers: %+v", got.Findings)
	}
	if got.Counts["drift"] != 2 {
		t.Errorf("counts = %v", got.Counts)
	}
	if b.Bytes()[b.Len()-1] != '\n' {
		t.Error("JSON output should end with a newline")
	}
}

// Whatever a finding carries, a renderer must not print a credential.
func TestRenderersNeverEchoASecret(t *testing.T) {
	canary := strings.Join([]string{"ghs", "FIXTURE_TOKEN_DO_NOT_PRINT"}, "_")
	rep := engine.Report{Target: "o/r", Families: 1, Counts: map[family.Kind]int{family.Drift: 1},
		Findings: []family.Finding{{Family: "webhooks", Key: "url", Kind: family.Drift,
			Want: "https://example.com/h?t=" + canary, Live: "https://example.com/h"}}}
	for name, render := range map[string]func(*bytes.Buffer) error{
		"tty":      func(b *bytes.Buffer) error { return TTY(b, rep, Options{NoColor: true}) },
		"markdown": func(b *bytes.Buffer) error { return Markdown(b, rep) },
		"json":     func(b *bytes.Buffer) error { return JSON(b, rep) },
	} {
		var b bytes.Buffer
		if err := render(&b); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(b.String(), canary) {
			t.Errorf("%s echoed a secret-shaped value: %s", name, b.String())
		}
		if !strings.Contains(b.String(), "REDACTED") {
			t.Errorf("%s should say it redacted something:\n%s", name, b.String())
		}
	}
}

// Colour is on by default and off with --no-color; the golden is the
// no-color form, so this only checks that colour appears at all.
func TestTTYColour(t *testing.T) {
	var colored, plain bytes.Buffer
	if err := TTY(&colored, sample(), Options{}); err != nil {
		t.Fatal(err)
	}
	if err := TTY(&plain, sample(), Options{NoColor: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(colored.String(), "\x1b[") {
		t.Error("want ANSI colour by default")
	}
	if strings.Contains(plain.String(), "\x1b[") {
		t.Error("--no-color must emit no escapes")
	}
}

func TestMarkdownTableEscapesPipes(t *testing.T) {
	rep := engine.Report{Target: "o/r", Families: 1, Counts: map[family.Kind]int{family.Drift: 1},
		Findings: []family.Finding{{Family: "general", Key: "description", Kind: family.Drift, Want: "a | b", Live: "c"}}}
	var b bytes.Buffer
	if err := Markdown(&b, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `a \| b`) {
		t.Fatalf("a pipe in a value must not break the table:\n%s", b.String())
	}
}
