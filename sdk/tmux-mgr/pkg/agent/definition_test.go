package agent

import (
	"os"
	"path/filepath"
	"testing"
)

const captainFile = `# Persona: The Captain (Program Manager)
# Model: internlm2:1.8b
# Aliases: cap, captain, boss
# GPU-Layers: system-auto
# Context-Window: 4096
# Symbol: 👨‍✈️
# Color: #FFD700

You are **The Captain**, the visionary and primary coordinator of the AI Agent Team.
`

const researcherFile = `# Persona: The Researcher (Model Architect)
# Model: smollm:360m
# Aliases: researcher, research, model-arch
# Symbol: 🔍

You are the Model Architect.
`

func writeAgent(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestParseDefinitionFile(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "the_captain", captainFile)

	def, err := parseDefinitionFile(filepath.Join(dir, "the_captain.md"))
	if err != nil {
		t.Fatalf("parseDefinitionFile: %v", err)
	}
	if def.Name != "the_captain" {
		t.Errorf("Name = %q, want the_captain", def.Name)
	}
	if def.Persona != "The Captain (Program Manager)" {
		t.Errorf("Persona = %q", def.Persona)
	}
	if def.Model != "internlm2:1.8b" {
		t.Errorf("Model = %q", def.Model)
	}
	if def.Symbol != "👨‍✈️" {
		t.Errorf("Symbol = %q", def.Symbol)
	}
	wantAliases := []string{"cap", "captain", "boss"}
	if len(def.Aliases) != len(wantAliases) {
		t.Fatalf("Aliases = %v, want %v", def.Aliases, wantAliases)
	}
	for i, a := range wantAliases {
		if def.Aliases[i] != a {
			t.Errorf("Aliases[%d] = %q, want %q", i, def.Aliases[i], a)
		}
	}
}

func TestParseDefinitionFile_StopsAtBody(t *testing.T) {
	dir := t.TempDir()
	writeAgent(t, dir, "x", "# Persona: Test\n# Model: foo:1b\n\nFree-form body text with: a colon should NOT be parsed.\n# Symbol: 💥\n")

	def, err := parseDefinitionFile(filepath.Join(dir, "x.md"))
	if err != nil {
		t.Fatalf("parseDefinitionFile: %v", err)
	}
	if def.Persona != "Test" {
		t.Errorf("Persona = %q", def.Persona)
	}
	if def.Symbol != "" {
		t.Errorf("Symbol should not leak through body: got %q", def.Symbol)
	}
}

func TestLoadDefinition_RepoBeforeHome(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	originalWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", home)

	writeAgent(t, filepath.Join(cwd, ".ai", "agents"), "the_captain", "# Persona: Repo Captain\n# Model: foo:1b\n")
	writeAgent(t, filepath.Join(home, ".ai", "agents"), "the_captain", "# Persona: Home Captain\n# Model: foo:1b\n")

	def, err := LoadDefinition("the_captain")
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if def == nil {
		t.Fatal("LoadDefinition returned nil")
	}
	if def.Persona != "Repo Captain" {
		t.Errorf("expected repo-local to win, got Persona=%q", def.Persona)
	}
}

func TestLoadDefinition_HomeFallback(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	originalWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", home)

	writeAgent(t, filepath.Join(home, ".ai", "agents"), "the_researcher", researcherFile)

	def, err := LoadDefinition("the_researcher")
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	if def == nil || def.Persona != "The Researcher (Model Architect)" {
		t.Errorf("expected home fallback to load researcher, got %+v", def)
	}
}

func TestLoadDefinition_AliasMatch(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	originalWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", home)

	writeAgent(t, filepath.Join(home, ".ai", "agents"), "the_captain", captainFile)

	def, err := LoadDefinition("cap")
	if err != nil {
		t.Fatalf("LoadDefinition(cap): %v", err)
	}
	if def == nil || def.Name != "the_captain" {
		t.Errorf("alias 'cap' should resolve to the_captain, got %+v", def)
	}
}

func TestLoadDefinition_NoMatchReturnsNil(t *testing.T) {
	cwd := t.TempDir()
	home := t.TempDir()

	originalWD, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(originalWD) })
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Setenv("HOME", home)

	def, err := LoadDefinition("nope")
	if err != nil {
		t.Fatalf("expected no error for missing def, got %v", err)
	}
	if def != nil {
		t.Errorf("expected nil for missing def, got %+v", def)
	}
}
