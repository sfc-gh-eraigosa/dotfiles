package approval

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// The key gss asks gff for is the one the repo declares, off by default.
func TestFlagDeclared(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", ".github", "gff", "features.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Sets []struct {
			Features []struct {
				Path        string `yaml:"path"`
				BoolDefault *bool  `yaml:"boolDefault"`
			} `yaml:"features"`
		} `yaml:"sets"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	for _, s := range doc.Sets {
		for _, f := range s.Features {
			if f.Path == FlagKey {
				if f.BoolDefault == nil || *f.BoolDefault {
					t.Fatalf("%s: boolDefault must be false", FlagKey)
				}
				return
			}
		}
	}
	t.Fatalf("%s is not declared in .github/gff/features.yaml", FlagKey)
}
