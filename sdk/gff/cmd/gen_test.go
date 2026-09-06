package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installGenResolver wires the newResolver seam for gen tests and resets gen
// flag vars on cleanup.
func installGenResolver(t *testing.T, p paths.Paths) {
	t.Helper()
	orig := newResolver
	origSource := sourceFlag
	t.Cleanup(func() {
		newResolver = orig
		sourceFlag = origSource
		resetGenFlags()
	})
	resetGenFlags()
	newResolver = func() (*resolve.Resolver, error) {
		r := resolve.New(p, fakeTestRunner{}, sourceFlag)
		r.S = &registry.Registry{P: p}
		return r, nil
	}
}

// ── golden test ───────────────────────────────────────────────────────────────

// TestGenGolden verifies that `gff gen --pkg gffgen --out <tmp>` produces
// <tmp>/gffgen.go matching cmd/testdata/gen.golden byte-for-byte, and that
// the output compiles cleanly (go vet via a scratch module with a replace
// directive — no network).
func TestGenGolden(t *testing.T) {
	p := goldenWorld(t)
	installGenResolver(t, p)

	outDir := t.TempDir()
	_, err := runCmd(t, "gen", "--pkg", "gffgen", "--out", outDir)
	require.NoError(t, err)

	gotBytes, err := os.ReadFile(filepath.Join(outDir, "gffgen.go"))
	require.NoError(t, err, "gffgen.go must be written to --out dir")

	golden, err := os.ReadFile("testdata/gen.golden")
	require.NoError(t, err, "testdata/gen.golden must exist")

	assert.Equal(t, string(golden), string(gotBytes),
		"gen output must match golden file byte-for-byte")
}

// TestGenGoldenShape verifies structural properties of the generated file
// independently of the exact byte representation.
func TestGenGoldenShape(t *testing.T) {
	p := goldenWorld(t)
	installGenResolver(t, p)

	outDir := t.TempDir()
	_, err := runCmd(t, "gen", "--pkg", "gffgen", "--out", outDir)
	require.NoError(t, err)

	src, err := os.ReadFile(filepath.Join(outDir, "gffgen.go"))
	require.NoError(t, err)
	content := string(src)

	// Package declaration matches --pkg.
	assert.Contains(t, content, "package gffgen")

	// Nested var chain: var Install = struct{ Ai struct{ ... } ... }{...}
	assert.Contains(t, content, "var Install =")

	// BoolFlag type with Bool() method delegating by literal key.
	assert.Contains(t, content, "type BoolFlag struct")
	assert.Contains(t, content, "func (f BoolFlag) Bool() (bool, error)")
	assert.Contains(t, content, `"install.ai.claude"`)

	// ChoiceFlag type with choice accessors delegating by literal key.
	assert.Contains(t, content, "type ChoiceFlag struct")
	assert.Contains(t, content, "func (f ChoiceFlag) Selected() ([]string, error)")
	assert.Contains(t, content, "func (f ChoiceFlag) IsSelected(optionID string) (bool, error)")
	assert.Contains(t, content, `"install.pkg.manager"`)

	// Naming: segments Title-cased, dashes camel-cased.
	// install.windows.wispr-flow → Windows.WisprFlow
	assert.Contains(t, content, "Windows")
	assert.Contains(t, content, "WisprFlow")

	// Claude bool flag present (install.ai.claude → Ai.Claude).
	assert.Contains(t, content, "Claude")
}

// TestGenGoldenCompiles verifies the generated file vetted by go vet via a
// scratch module embedding the output. A replace directive points at the local
// sdk/gff module so no network fetch occurs.
func TestGenGoldenCompiles(t *testing.T) {
	// Skip if go is not on PATH (should not happen in CI).
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	p := goldenWorld(t)
	installGenResolver(t, p)

	outDir := t.TempDir()
	_, err := runCmd(t, "gen", "--pkg", "gffgen", "--out", outDir)
	require.NoError(t, err)

	gotBytes, err := os.ReadFile(filepath.Join(outDir, "gffgen.go"))
	require.NoError(t, err)

	// Build scratch module that imports the generated package.
	scratchDir := t.TempDir()
	gffgenDir := filepath.Join(scratchDir, "gffgen")
	require.NoError(t, os.MkdirAll(gffgenDir, 0o755))

	// Copy generated file into scratch/gffgen/.
	require.NoError(t, os.WriteFile(filepath.Join(gffgenDir, "gffgen.go"), gotBytes, 0o644))

	// Resolve the absolute path to sdk/gff for the replace directive.
	// Tests run with CWD = sdk/gff/cmd/, so ".." = sdk/gff/.
	sdkGffPath, err := filepath.Abs("..")
	require.NoError(t, err)
	// Confirm it's actually the sdk/gff module by checking go.mod exists.
	_, modErr := os.Stat(filepath.Join(sdkGffPath, "go.mod"))
	require.NoError(t, modErr, "could not locate sdk/gff go.mod at %s", sdkGffPath)

	// Write scratch module go.mod with replace directive.
	goMod := "module scratch.local/vetcheck\n\ngo 1.21\n\nrequire github.com/sfc-gh-eraigosa/dotfiles/sdk/gff v0.0.0\n\nreplace github.com/sfc-gh-eraigosa/dotfiles/sdk/gff => " + sdkGffPath + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "go.mod"), []byte(goMod), 0o644))

	// Write a minimal main.go that imports the generated package to force
	// compilation; go vet ./... will pick up both packages.
	mainSrc := `package main

import (
	_ "scratch.local/vetcheck/gffgen"
)

func main() {}
`
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "main.go"), []byte(mainSrc), 0o644))

	// Run go mod tidy then go vet ./... inside the scratch module.
	// GOFLAGS=-mod=mod lets tidy update the go.sum without a proxy fetch.
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = scratchDir
	tidyCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GONOSUMCHECK=*", "GONOSUMDB=*", "GOFLAGS=-mod=mod")
	tidyOut, tidyErr := tidyCmd.CombinedOutput()
	require.NoError(t, tidyErr, "go mod tidy failed:\n%s", string(tidyOut))

	vetCmd := exec.Command("go", "vet", "./...")
	vetCmd.Dir = scratchDir
	vetCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	vetOut, vetErr := vetCmd.CombinedOutput()
	assert.NoError(t, vetErr, "go vet on generated code failed:\n%s", string(vetOut))
}

// TestGenEmptyWorld verifies that an empty feature set produces a valid,
// compilable Go file (no flags, but valid package structure).
func TestGenEmptyWorld(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	dir := t.TempDir()
	p := paths.Paths{
		SystemSnapshotDir: filepath.Join(dir, "sys-snap"),
		UserSnapshotDir:   filepath.Join(dir, "user-snap"),
		SystemOverride:    filepath.Join(dir, "sys-config.yaml"),
		UserOverride:      filepath.Join(dir, "user-config.yaml"),
		RegistryFile:      filepath.Join(dir, "sources.yaml"),
		WorkDir:           filepath.Join(dir, "workdir"),
	}
	require.NoError(t, os.MkdirAll(p.WorkDir, 0o755))
	installGenResolver(t, p)

	outDir := t.TempDir()
	_, err := runCmd(t, "gen", "--pkg", "mypkg", "--out", outDir)
	require.NoError(t, err, "gen on empty world must succeed")

	src, err := os.ReadFile(filepath.Join(outDir, "mypkg.go"))
	require.NoError(t, err)
	content := string(src)

	// Must declare the correct package.
	assert.Contains(t, content, "package mypkg")

	// Verify it compiles.
	// Tests run with CWD = sdk/gff/cmd/, so ".." = sdk/gff/.
	sdkGffPath, err := filepath.Abs("..")
	require.NoError(t, err)

	scratchDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644))

	pkgDir := filepath.Join(scratchDir, "mypkg")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkgDir, "mypkg.go"), src, 0o644))

	goMod := "module scratch.local/emptycheck\n\ngo 1.21\n\nrequire github.com/sfc-gh-eraigosa/dotfiles/sdk/gff v0.0.0\n\nreplace github.com/sfc-gh-eraigosa/dotfiles/sdk/gff => " + sdkGffPath + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "go.mod"), []byte(goMod), 0o644))

	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = scratchDir
	tidyCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GONOSUMCHECK=*", "GONOSUMDB=*")
	tidyOut, tidyErr := tidyCmd.CombinedOutput()
	require.NoError(t, tidyErr, "go mod tidy failed:\n%s", string(tidyOut))

	vetCmd := exec.Command("go", "vet", "./...")
	vetCmd.Dir = scratchDir
	vetCmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	vetOut, vetErr := vetCmd.CombinedOutput()
	assert.NoError(t, vetErr, "go vet on empty-world generated code failed:\n%s", string(vetOut))
}

// TestGenBadOutDir verifies that an unwritable output directory returns an error.
func TestGenBadOutDir(t *testing.T) {
	p := goldenWorld(t)
	installGenResolver(t, p)

	// Point --out at a non-existent deeply nested path with a read-only parent.
	roDir := t.TempDir()
	require.NoError(t, os.Chmod(roDir, 0o555))
	t.Cleanup(func() { _ = os.Chmod(roDir, 0o755) })

	badOut := filepath.Join(roDir, "nested", "output")
	_, err := runCmd(t, "gen", "--pkg", "gffgen", "--out", badOut)
	require.Error(t, err, "gen with unwritable --out must return an error")
}

// TestGenNaming verifies the Title-casing and dash→camelCase naming rules
// without relying on the exact golden file byte layout.
func TestGenNaming(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"wispr-flow", "WisprFlow"},
		{"claude", "Claude"},
		{"pkg", "Pkg"},
		{"manager", "Manager"},
		{"windows", "Windows"},
		{"ai", "Ai"},
	}
	for _, tc := range cases {
		got := segmentToTitle(tc.input)
		assert.Equal(t, tc.want, got, "segmentToTitle(%q)", tc.input)
	}
}

// ── update-golden helper ──────────────────────────────────────────────────────

// TestGenUpdateGolden regenerates the golden file when run with
// -update flag. Normally skipped.
func TestGenUpdateGolden(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "-update") {
		t.Skip("pass -update to regenerate golden files")
	}

	p := goldenWorld(t)
	installGenResolver(t, p)

	outDir := t.TempDir()
	var outBuf bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)
	cmd.SetArgs([]string{"gen", "--pkg", "gffgen", "--out", outDir})
	require.NoError(t, cmd.Execute())

	generated, err := os.ReadFile(filepath.Join(outDir, "gffgen.go"))
	require.NoError(t, err)

	require.NoError(t, os.WriteFile("testdata/gen.golden", generated, 0o644))
	t.Logf("golden updated (%d bytes)", len(generated))
}
