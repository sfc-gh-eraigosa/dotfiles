// Package tmpl_test verifies the renderer per sdk/gss/docs/plan.md PR-30:
// FEATURE.md / WORKER.md substitution and ANSI/control-char stripping of
// user-supplied fields (the real embedded templates land in PR-31, so
// these tests use representative inline templates).
package tmpl_test

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gss/internal/tmpl"
)

func TestRenderFeature(t *testing.T) {
	got, err := tmpl.RenderFeature("# Feature: {{.Name}}\n\n{{.Description}}\n\nbase: {{.BaseBranch}}\n", tmpl.FeatureData{
		Name: "auth", Description: "Add login flow", BaseBranch: "main", StartedAt: "2026-05-21",
	})
	if err != nil {
		t.Fatalf("RenderFeature: %v", err)
	}
	want := "# Feature: auth\n\nAdd login flow\n\nbase: main\n"
	if got != want {
		t.Errorf("RenderFeature =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderWorker(t *testing.T) {
	got, err := tmpl.RenderWorker("# {{.Feature}}/{{.User}}/{{.Purpose}}\nbranch: {{.Branch}}\ngoal: {{.Goal}}\n", tmpl.WorkerData{
		Feature: "auth", User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", Goal: "ship endpoints",
	})
	if err != nil {
		t.Fatalf("RenderWorker: %v", err)
	}
	want := "# auth/erai/api\nbranch: feature/auth/erai/api\ngoal: ship endpoints\n"
	if got != want {
		t.Errorf("RenderWorker =\n%q\nwant\n%q", got, want)
	}
}

func TestRender_SanitizesControlChars(t *testing.T) {
	// Description carries an ANSI colour sequence + a bell control char.
	got, err := tmpl.RenderFeature("{{.Description}}", tmpl.FeatureData{
		Description: "ple\x07ase \x1b[31mred\x1b[0m text",
	})
	if err != nil {
		t.Fatalf("RenderFeature: %v", err)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("ANSI escape survived: %q", got)
	}
	if strings.ContainsRune(got, 0x07) {
		t.Errorf("bell control char survived: %q", got)
	}
	if got != "please red text" {
		t.Errorf("sanitised output = %q; want %q", got, "please red text")
	}
}

func TestRender_KeepsNewlinesAndTabs(t *testing.T) {
	got, err := tmpl.RenderWorker("{{.Goal}}", tmpl.WorkerData{Goal: "line1\n\tindented"})
	if err != nil {
		t.Fatalf("RenderWorker: %v", err)
	}
	if got != "line1\n\tindented" {
		t.Errorf("newlines/tabs not preserved: %q", got)
	}
}

func TestRender_ParseError(t *testing.T) {
	if _, err := tmpl.RenderFeature("{{.Unclosed", tmpl.FeatureData{}); err == nil {
		t.Error("malformed template: err = nil; want parse error")
	}
}
