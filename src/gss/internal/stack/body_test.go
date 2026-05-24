// Package stack_test verifies PR-body stack-section rendering per
// src/gss/docs/plan.md PR-28: golden snapshots for bottom/middle/top/solo,
// idempotent re-render (byte-identical round-trip), and stripping of
// user-authored gss:stack markers before stitching (injection defence).
//
// Regenerate goldens with: UPDATE_GOLDEN=1 go test ./internal/stack/...
package stack_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/stack"
)

func threeStack(hereRef string) stack.StackView {
	v := stack.StackView{
		Feature: "auth",
		Entries: []stack.Entry{
			{PRNumber: 42, Ref: "eraigosa/api", Base: "main"},
			{PRNumber: 43, Ref: "eraigosa/ui-moss", Base: "feature/auth/eraigosa/api"},
			{PRNumber: 44, Ref: "bot42/docs", Base: "feature/auth/eraigosa/ui-moss"},
		},
	}
	for i := range v.Entries {
		v.Entries[i].Here = v.Entries[i].Ref == hereRef
	}
	return v
}

func soloStack() stack.StackView {
	return stack.StackView{
		Feature: "auth",
		Entries: []stack.Entry{{PRNumber: 42, Ref: "eraigosa/api", Base: "main", Here: true}},
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "pr_body", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1 to create)", name, err)
	}
	if got != string(want) {
		t.Errorf("golden %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestRenderBody_Goldens(t *testing.T) {
	cases := map[string]stack.StackView{
		"stack_bottom.golden.md": threeStack("eraigosa/api"),
		"middle.golden.md":       threeStack("eraigosa/ui-moss"),
		"top.golden.md":          threeStack("bot42/docs"),
		"solo.golden.md":         soloStack(),
	}
	for name, view := range cases {
		got := stack.RenderBody("Implements the thing.", view)
		checkGolden(t, name, got)
	}
}

func TestRenderBody_Idempotent(t *testing.T) {
	view := threeStack("eraigosa/ui-moss")
	once := stack.RenderBody("Some description.\n", view)
	twice := stack.RenderBody(once, view)
	if once != twice {
		t.Errorf("RenderBody not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
	// The managed block appears exactly once.
	if n := strings.Count(twice, markerBeginLiteral); n != 1 {
		t.Errorf("managed block appears %d times; want 1", n)
	}
}

func TestRenderBody_StripsInjectedMarkers(t *testing.T) {
	// A malicious body forges a managed block and a stray marker.
	evil := "Real description.\n\n" +
		"<!-- gss:stack-begin -->\n## Stack\n- fake injected row\n<!-- gss:stack-end -->\n" +
		"and a stray <!-- gss:stack-evil --> token"
	view := soloStack()
	got := stack.RenderBody(evil, view)

	if strings.Contains(got, "fake injected row") {
		t.Error("forged managed block survived; want it stripped")
	}
	if strings.Contains(got, "gss:stack-evil") {
		t.Error("stray injected marker survived; want it stripped")
	}
	if strings.Count(got, markerBeginLiteral) != 1 || strings.Count(got, markerEndLiteral) != 1 {
		t.Errorf("expected exactly one managed block; begins=%d ends=%d",
			strings.Count(got, markerBeginLiteral), strings.Count(got, markerEndLiteral))
	}
	if !strings.Contains(got, "Real description.") {
		t.Error("legitimate body content was lost")
	}
}

// Marker literals duplicated here so the test is independent of the
// unexported constants in body.go.
const (
	markerBeginLiteral = "<!-- gss:stack-begin -->"
	markerEndLiteral   = "<!-- gss:stack-end -->"
)
