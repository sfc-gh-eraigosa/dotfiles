// Package tmpl_test verifies the embedded templates + loaders per
// src/gss/docs/plan.md PR-31: the embed FS contains both templates and the
// renderer produces substituted output from them.
package tmpl_test

import (
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/tmpl"
)

func TestEmbeddedTemplatesPresent(t *testing.T) {
	ft, err := tmpl.FeatureTemplate()
	if err != nil {
		t.Fatalf("FeatureTemplate: %v", err)
	}
	for _, want := range []string{"# Feature: {{.Name}}", "## Workers", "## Decisions & notes"} {
		if !strings.Contains(ft, want) {
			t.Errorf("feature template missing %q", want)
		}
	}
	wt, err := tmpl.WorkerTemplate()
	if err != nil {
		t.Fatalf("WorkerTemplate: %v", err)
	}
	for _, want := range []string{"{{.Feature}}", "**Branch**", "## Open questions"} {
		if !strings.Contains(wt, want) {
			t.Errorf("worker template missing %q", want)
		}
	}
}

func TestRenderEmbeddedFeature(t *testing.T) {
	got, err := tmpl.RenderEmbeddedFeature(tmpl.FeatureData{
		Name: "auth", Description: "Add login", StartedAt: "2026-05-21", BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("RenderEmbeddedFeature: %v", err)
	}
	if !strings.Contains(got, "# Feature: auth") || !strings.Contains(got, "Add login") {
		t.Errorf("rendered feature missing substitutions:\n%s", got)
	}
}

func TestRenderEmbeddedWorker(t *testing.T) {
	got, err := tmpl.RenderEmbeddedWorker(tmpl.WorkerData{
		Feature: "auth", User: "erai", Purpose: "api", Suffix: "moss",
		Branch: "feature/auth/erai/api-moss", BaseBranch: "main", Goal: "ship it",
	})
	if err != nil {
		t.Fatalf("RenderEmbeddedWorker: %v", err)
	}
	for _, want := range []string{"# auth: api", "erai/api-moss", "feature/auth/erai/api-moss", "ship it"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered worker missing %q:\n%s", want, got)
		}
	}
}

func TestRenderEmbeddedWorker_GoalPlaceholder(t *testing.T) {
	got, err := tmpl.RenderEmbeddedWorker(tmpl.WorkerData{Feature: "f", User: "u", Purpose: "p", Branch: "b", BaseBranch: "main"})
	if err != nil {
		t.Fatalf("RenderEmbeddedWorker: %v", err)
	}
	if !strings.Contains(got, "<!-- describe this worker's goal -->") {
		t.Errorf("empty goal should render the placeholder:\n%s", got)
	}
}
