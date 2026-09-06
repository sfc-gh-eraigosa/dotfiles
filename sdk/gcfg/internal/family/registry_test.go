package family_test

import (
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family"
	_ "github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family/general"
	_ "github.com/sfc-gh-eraigosa/dotfiles/sdk/gcfg/internal/family/security"
)

// Importing a family package must be all it takes to make it available —
// that is what lets cmd stay ignorant of which families a build has.
func TestFamiliesSelfRegister(t *testing.T) {
	var names []string
	for _, f := range family.All(family.ScopeRepo) {
		names = append(names, f.Name())
	}
	if strings.Join(names, ",") != "general,security" {
		t.Fatalf("registered repo families = %v", names)
	}
	got, err := family.Select(family.ScopeRepo, []string{"security"})
	if err != nil || len(got) != 1 || got[0].Name() != "security" {
		t.Fatalf("Select = %v %v", got, err)
	}
	if _, err := family.Select(family.ScopeRepo, []string{"labels"}); err == nil {
		t.Fatal("a family this build does not have must be an error, not a silent skip")
	}
	// Every registered family must declare the permission its writes need,
	// so `auth doctor` can name it when GitHub answers 403.
	for _, f := range family.All(family.ScopeRepo) {
		if !strings.Contains(f.Permission(), ":") {
			t.Errorf("%s permission = %q, want scope:Permission:level", f.Name(), f.Permission())
		}
	}
}
