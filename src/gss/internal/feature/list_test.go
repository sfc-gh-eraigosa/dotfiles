package feature_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/wenlock/dotfiles/gss/internal/feature"
	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// seedListService returns a Service over a temp registry pre-populated with
// a 2-worker stacked feature. List only needs the Store.
func seedListService(t *testing.T) *feature.Service {
	t.Helper()
	store := registry.NewStore(filepath.Join(t.TempDir(), "registry.json"))
	if err := store.Update(func(r *registry.Registry) error {
		*r = registry.Registry{
			SchemaVersion: 1,
			Features: []registry.Feature{{
				Name: "auth", DefaultBaseBranch: "main", Description: "login work",
				Workers: []registry.Worker{
					{User: "erai", Purpose: "api", Branch: "feature/auth/erai/api", BaseBranch: "main", Description: "endpoints", PRState: "draft"},
					{User: "erai", Purpose: "ui", Suffix: "moss", Branch: "feature/auth/erai/ui-moss", BaseBranch: "feature/auth/erai/api", Description: "wire ui"},
				},
			}},
		}
		return nil
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return &feature.Service{Store: store}
}

func TestList_FlatShowsDescription(t *testing.T) {
	out, err := seedListService(t).List(context.Background(), feature.ListOpts{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, want := range []string{"Feature: auth (base: main)", "auth/erai/api — endpoints [draft]", "auth/erai/ui-moss — wire ui"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

func TestList_TreeIndentsByDepth(t *testing.T) {
	out, err := seedListService(t).List(context.Background(), feature.ListOpts{Tree: true})
	if err != nil {
		t.Fatalf("List tree: %v", err)
	}
	// api is bottom (depth 0 → 2-space indent); ui-moss is depth 1 → 4-space.
	if !strings.Contains(out, "  - auth/erai/api") {
		t.Errorf("api should be at depth 0:\n%s", out)
	}
	if !strings.Contains(out, "    - auth/erai/ui-moss") {
		t.Errorf("ui-moss should be indented one level deeper:\n%s", out)
	}
}

func TestList_JSONSchema(t *testing.T) {
	out, err := seedListService(t).List(context.Background(), feature.ListOpts{JSON: true})
	if err != nil {
		t.Fatalf("List json: %v", err)
	}
	path := filepath.Join("testdata", "list", "feature_list.golden.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run UPDATE_GOLDEN=1)", err)
	}
	if out != string(want) {
		t.Errorf("json schema drift:\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestList_WithSessionsNotImplemented(t *testing.T) {
	if _, err := seedListService(t).List(context.Background(), feature.ListOpts{WithSessions: true}); err == nil {
		t.Error("--with sessions: want not-yet-implemented error")
	}
}

func TestList_UnknownFeature(t *testing.T) {
	if _, err := seedListService(t).List(context.Background(), feature.ListOpts{Feature: "ghost"}); err == nil {
		t.Error("unknown feature filter: want error")
	}
}

// TestSpawnedByInformationalOnly enforces resolution #8: no code path in
// internal/ branches on spawned_by.engine. We grep every non-test .go file
// under internal/ for an Engine comparison.
func TestSpawnedByInformationalOnly(t *testing.T) {
	cmp := regexp.MustCompile(`Engine\s*[=!]=`)
	caseEngine := regexp.MustCompile(`case\b.*\bEngine\b`)
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if cmp.Match(data) || caseEngine.Match(data) {
			t.Errorf("%s appears to branch on spawned_by engine (resolution #8: informational-only)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}
