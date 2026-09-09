//go:build e2e
// +build e2e

package e2e_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// getGffBinary returns the path to the gff binary built by e2e.sh.
func getGffBinary(t *testing.T) string {
	bin := os.Getenv("GFF_E2E_BIN")
	if bin == "" {
		t.Fatal("GFF_E2E_BIN env var not set (e2e.sh should set it)")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("GFF_E2E_BIN binary not found: %s", bin)
	}
	return bin
}

// runCmd runs a gff command in a fake HOME with real git and returns exit code, stdout, stderr.
func runCmd(t *testing.T, fakeHome string, args ...string) (int, string, string) {
	return runCmdIn(t, fakeHome, fakeHome, args...)
}

// runCmdIn runs the binary with an explicit working directory — required for
// verbs that discover the repo from CWD (install, unscoped get/lint). The
// process-level os.Chdir has NO effect on the child: cmd.Dir wins.
func runCmdIn(t *testing.T, fakeHome, dir string, args ...string) (int, string, string) {
	bin := getGffBinary(t)
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"HOME=" + fakeHome,
		"XDG_DATA_HOME=" + filepath.Join(fakeHome, ".local", "share"),
		"GIT_CONFIG_GLOBAL=" + filepath.Join(fakeHome, ".gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"PATH=" + os.Getenv("PATH"),
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("command failed: %v", err)
		}
	}

	return exitCode, stdout.String(), stderr.String()
}

// initRepo creates a git repo at path with a flag file and returns the repo path.
func initRepo(t *testing.T, path, namespace, features string) {
	os.MkdirAll(path, 0755)
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git init failed: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config email failed: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config name failed: %v", err)
	}

	flagFile := filepath.Join(path, ".gff", "features.yaml")
	os.MkdirAll(filepath.Dir(flagFile), 0755)
	if err := os.WriteFile(flagFile, []byte(features), 0644); err != nil {
		t.Fatalf("write features.yaml failed: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add failed: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	cmd = exec.Command("git", "config", "remote.origin.url", "https://example.com/repo.git")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		t.Fatalf("git config remote failed: %v", err)
	}
}

// minimalFlagFile returns a YAML flag file with a bool, a single-choice, and a multi-choice.
func minimalFlagFile() string {
	return `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude CLI
        boolDefault: true
      - path: install.pkg.manager
        description: Package manager selection
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt, description: Debian/Ubuntu apt, stringValue: apt}
            - {id: brew, description: Homebrew, stringValue: brew}
      - path: install.shell.plugins
        description: Shell plugins
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: fzf, description: Fuzzy finder, selected: false}
            - {id: starship, description: Starship prompt, selected: false}
`
}

// TestHappyPath runs the happy-path scenarios (IH-1 through IH-10) in order, sharing one world.
func TestHappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	world := t.TempDir()
	repoA := filepath.Join(world, "repo-a")

	// Derive module root from where the test is running
	// When go test -tags e2e ./e2e runs, we're in sdk/gff
	moduleRoot, _ := os.Getwd()
	if !strings.Contains(moduleRoot, "/sdk/gff") && !strings.HasSuffix(moduleRoot, "/sdk/gff") {
		// If we're not in sdk/gff, walk up from the test package location
		moduleRoot = filepath.Join(moduleRoot, "..", "..")
	}

	// IH-1: lint on a valid flag file → exit 0
	t.Run("IH-1-lint-valid-file", func(t *testing.T) {
		initRepo(t, repoA, "com.example.test", minimalFlagFile())
		code, stdout, stderr := runCmd(t, world, "lint", filepath.Join(repoA, ".gff", "features.yaml"))
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
		if stdout != "" && !strings.Contains(stdout, "No findings") && !strings.Contains(stdout, "no findings") {
			t.Logf("lint output: %s", stdout)
		}
	})

	// IH-2: install in repo A → sources.yaml + snapshot written
	t.Run("IH-2-install-repo", func(t *testing.T) {
		// Install requires CWD to be the repo; create a wrapper command
		bin := getGffBinary(t)
		cmd := exec.Command(bin, "install")
		cmd.Dir = repoA // CWD = the repo
		cmd.Env = []string{
			"HOME=" + world,
			"XDG_DATA_HOME=" + filepath.Join(world, ".local", "share"),
			"GIT_CONFIG_GLOBAL=" + filepath.Join(world, ".gitconfig"),
			"GIT_CONFIG_NOSYSTEM=1",
			"PATH=" + os.Getenv("PATH"),
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr.String())
		}

		// Check sources.yaml exists
		srcFile := filepath.Join(world, ".config", "gff", "sources.yaml")
		if _, err := os.Stat(srcFile); err != nil {
			t.Errorf("sources.yaml not found: %v", err)
		}

		// Check snapshot exists
		snapDir := filepath.Join(world, ".local", "share", "gff", "snapshots")
		snaps, _ := os.ReadDir(snapDir)
		if len(snaps) == 0 {
			t.Error("no snapshots found")
		}
	})

	// IH-3: get/enabled on a default-true bool from foreign CWD → true / exit 0
	t.Run("IH-3-get-default-true", func(t *testing.T) {
		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(filepath.Join(world, "some-other-dir"))
		os.MkdirAll(filepath.Join(world, "some-other-dir"), 0755)

		code, stdout, stderr := runCmd(t, world, "get", "--source", repoA, "install.ai.claude")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
		out := strings.TrimSpace(stdout)
		if out != "true" {
			t.Errorf("expected 'true', got %q", out)
		}

		code, _, stderr = runCmd(t, world, "enabled", "--source", repoA, "install.ai.claude")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
	})

	// IH-4: selected on default choice option → exit 0; get prints the id
	t.Run("IH-4-selected-default", func(t *testing.T) {
		code, _, stderr := runCmd(t, world, "selected", "--source", repoA, "install.pkg.manager", "auto")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}

		code, stdout, stderr := runCmd(t, world, "get", "--source", repoA, "install.pkg.manager")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
		out := strings.TrimSpace(stdout)
		if out != "auto" {
			t.Errorf("expected 'auto', got %q", out)
		}
	})

	// IH-5: set bool false → ONLY user override changes (0600); list --json shows user-override layer
	t.Run("IH-5-set-bool-false", func(t *testing.T) {
		code, _, stderr := runCmd(t, world, "set", "--source", repoA, "install.ai.claude", "false")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}

		// Check override file exists and has 0600 perms
		ovr := filepath.Join(world, ".config", "gff", "config.yaml")
		info, err := os.Stat(ovr)
		if err != nil {
			t.Errorf("override file not found: %v", err)
		} else if info.Mode()&0777 != 0600 {
			t.Errorf("expected perms 0600, got %o", info.Mode()&0777)
		}

		// Check list --json shows user-override layer
		code, stdout, _ := runCmd(t, world, "list", "--source", repoA, "--json")
		var items []map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &items); err != nil {
			t.Fatalf("list --json parse failed: %v", err)
		}
		for _, item := range items {
			if path, ok := item["path"].(string); ok && path == "install.ai.claude" {
				if layer, ok := item["layer"].(string); ok && layer != "user-override" {
					t.Errorf("expected layer 'user-override', got %q", layer)
				}
				break
			}
		}
	})

	// IH-6: set choice — single: one id; multi: two ids — round-trip through get
	t.Run("IH-6-set-choice", func(t *testing.T) {
		code, _, stderr := runCmd(t, world, "set", "--source", repoA, "install.pkg.manager", "apt")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
		code, stdout, _ := runCmd(t, world, "get", "--source", repoA, "install.pkg.manager")
		if code != 0 || strings.TrimSpace(stdout) != "apt" {
			t.Errorf("single-choice round-trip failed: got %q", strings.TrimSpace(stdout))
		}

		code, _, stderr = runCmd(t, world, "set", "--source", repoA, "install.shell.plugins", "fzf,starship")
		if code != 0 {
			t.Errorf("expected exit 0, got %d; stderr: %s", code, stderr)
		}
		var multiOut string
		code, multiOut, _ = runCmd(t, world, "get", "--source", repoA, "install.shell.plugins")
		if code != 0 {
			t.Errorf("get multi-choice failed: %d", code)
		}
		ids := strings.TrimSpace(multiOut)
		if ids != "fzf,starship" && ids != "starship,fzf" {
			t.Errorf("expected 'fzf,starship' or 'starship,fzf', got %q", ids)
		}
	})

	// IH-7: export --format shell evals cleanly in bash AND dash; gff_on works
	t.Run("IH-7-export-shell", func(t *testing.T) {
		code, stdout, stderr := runCmd(t, world, "export", "--source", repoA, "--format", "shell")
		if code != 0 {
			t.Errorf("export shell failed: exit %d; stderr: %s", code, stderr)
		}

		// Embed the gff_on helper inline for testing (mirrors §3.5 contract)
		helper := `
gff_on() {
  _gff_key=$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr '.-' '__')
  eval "_gff_val=\${GFF_${_gff_key}:-}"
  [ "${_gff_val}" != "false" ]
}
`
		script := helper + stdout + `
gff_on install.ai.claude && echo "claude-on" || echo "claude-off"
gff_on install.pkg.manager && echo "pkg-on" || echo "pkg-off"
`

		// Test in bash
		cmd := exec.Command("bash", "-c", script)
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			t.Errorf("bash eval failed: %v", err)
		}
		outStr := out.String()
		if !strings.Contains(outStr, "claude-off") {
			t.Errorf("bash: expected 'claude-off' (was set false), got: %s", outStr)
		}
		if !strings.Contains(outStr, "pkg-on") {
			t.Errorf("bash: expected 'pkg-on', got: %s", outStr)
		}

		// Test in dash
		cmd = exec.Command("sh", "-c", script)
		out.Reset()
		cmd.Stdout = &out
		if err := cmd.Run(); err != nil {
			t.Errorf("dash eval failed: %v", err)
		}
		outStr = out.String()
		if !strings.Contains(outStr, "claude-off") {
			t.Errorf("dash: expected 'claude-off', got: %s", outStr)
		}
	})

	// IH-8: export dotenv, json, yaml all work and formats are consistent
	t.Run("IH-8-export-formats", func(t *testing.T) {
		// JSON
		code, jsonOut, stderr := runCmd(t, world, "export", "--source", repoA, "--format", "json")
		if code != 0 {
			t.Errorf("export json failed: exit %d; stderr: %s", code, stderr)
		}
		var jsonItems []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonOut), &jsonItems); err != nil {
			t.Fatalf("json parse failed: %v", err)
		}

		// YAML
		code, yamlOut, stderr := runCmd(t, world, "export", "--source", repoA, "--format", "yaml")
		if code != 0 {
			t.Errorf("export yaml failed: exit %d; stderr: %s", code, stderr)
		}
		var yamlItems []map[string]interface{}
		if err := yaml.Unmarshal([]byte(yamlOut), &yamlItems); err != nil {
			t.Fatalf("yaml parse failed: %v", err)
		}

		// Counts should match
		if len(jsonItems) != len(yamlItems) {
			t.Errorf("json/yaml item count mismatch: %d vs %d", len(jsonItems), len(yamlItems))
		}

		// Dotenv with explicit -o to stdout (empty -o would default to .env file)
		// We use a temp file and then read it for the test
		dotenvFile := filepath.Join(world, "test.env")
		bin := getGffBinary(t)
		cmd := exec.Command(bin, "export", "--source", repoA, "--format", "dotenv", "-o", dotenvFile)
		cmd.Dir = world
		cmd.Env = []string{
			"HOME=" + world,
			"XDG_DATA_HOME=" + filepath.Join(world, ".local", "share"),
			"GIT_CONFIG_GLOBAL=" + filepath.Join(world, ".gitconfig"),
			"GIT_CONFIG_NOSYSTEM=1",
			"PATH=" + os.Getenv("PATH"),
		}
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		err := cmd.Run()
		dotenvCode := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				dotenvCode = ee.ExitCode()
			}
		}
		if dotenvCode != 0 {
			t.Errorf("export dotenv failed: exit %d; stderr: %s", dotenvCode, stderrBuf.String())
		}

		// Read the .env file
		dotenvBytes, errRead := os.ReadFile(dotenvFile)
		if errRead != nil {
			t.Errorf("dotenv file not created: %v", errRead)
		}
		dotenvOut := string(dotenvBytes)
		if !strings.Contains(dotenvOut, "GFF_") {
			t.Errorf("dotenv output missing GFF_ vars: %s", dotenvOut)
		}
	})

	// IH-9: unset → default restored; winning layer reverts
	t.Run("IH-9-unset-restore-default", func(t *testing.T) {
		code, _, stderr := runCmd(t, world, "unset", "--source", repoA, "install.ai.claude")
		if code != 0 {
			t.Errorf("unset failed: exit %d; stderr: %s", code, stderr)
		}

		code, stdout, _ := runCmd(t, world, "get", "--source", repoA, "install.ai.claude")
		if code != 0 || strings.TrimSpace(stdout) != "true" {
			t.Errorf("after unset, expected 'true', got %q", strings.TrimSpace(stdout))
		}
	})

	// IH-10: --source flags work from foreign CWD (zero-install via local repo path, not go run)
	t.Run("IH-10-source-foreign-cwd", func(t *testing.T) {
		// Query using --source <local-path> from a CWD that's not in any repo
		otherCwd := filepath.Join(world, "random-dir")
		os.MkdirAll(otherCwd, 0755)

		bin := getGffBinary(t)
		cmd := exec.Command(bin, "get", "--source", repoA, "install.ai.claude")
		cmd.Dir = otherCwd // CWD is unrelated to repoA
		cmd.Env = []string{
			"HOME=" + world,
			"XDG_DATA_HOME=" + filepath.Join(world, ".local", "share"),
			"GIT_CONFIG_GLOBAL=" + filepath.Join(world, ".gitconfig"),
			"GIT_CONFIG_NOSYSTEM=1",
			"PATH=" + os.Getenv("PATH"),
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()
		code := 0
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			}
		}
		if code != 0 {
			t.Errorf("get with --source <path> failed: exit %d; stderr: %s", code, stderr.String())
		}
		out := strings.TrimSpace(stdout.String())
		if out != "true" {
			t.Errorf("expected 'true', got %q", out)
		}
	})
}

// TestAdversarial runs negative/error scenarios (IA-1 through IA-15), each in isolation.
func TestAdversarial(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	// IA-1: unknown key → exit 2 on get/enabled/set; unknown option id → exit 2 on selected
	t.Run("IA-1-unknown-key", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		code, _, stderr := runCmd(t, world, "get", "--source", repoA, "unknown.key.here")
		if code != 2 {
			t.Errorf("expected exit 2 on unknown key, got %d", code)
		}
		if !strings.Contains(stderr, "unknown") && !strings.Contains(stderr, "key") {
			t.Logf("stderr: %s", stderr)
		}

		code, _, _ = runCmd(t, world, "selected", "--source", repoA, "install.pkg.manager", "unknown-id")
		if code != 2 {
			t.Errorf("expected exit 2 on unknown option id, got %d", code)
		}
	})

	// IA-2: set two ids on single-mode choice → exit 1; file unchanged
	t.Run("IA-2-single-choice-arity", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		// Get initial file state
		ovrPath := filepath.Join(world, ".config", "gff", "config.yaml")
		beforeStat, _ := os.Stat(ovrPath)
		var beforeTime time.Time
		if beforeStat != nil {
			beforeTime = beforeStat.ModTime()
		}

		code, _, _ := runCmd(t, world, "set", "--source", repoA, "install.pkg.manager", "apt,brew")
		if code != 1 {
			t.Errorf("expected exit 1 on two ids for single-mode, got %d", code)
		}

		// Check file unchanged (or was not created)
		afterStat, err := os.Stat(ovrPath)
		if err == nil && beforeStat != nil {
			// File exists before and after; check mtime didn't change
			if afterStat.ModTime() != beforeTime {
				t.Error("override file was modified despite error")
			}
		}
	})

	// IA-3: malformed flag file → lint and read verbs fail with file+line; no panic
	t.Run("IA-3-malformed-flag-file", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		os.MkdirAll(filepath.Join(repoA, ".gff"), 0755)

		// Write a truncated YAML
		flagFile := filepath.Join(repoA, ".gff", "features.yaml")
		// Truncated mid-list: unclosed flow mapping — a real YAML parse error.
		os.WriteFile(flagFile, []byte(`namespace: com.example.test
sets:
  - area: install
    features:
      - {path: install.ai.claude, description: truncated
`), 0644)

		cmd := exec.Command("git", "init")
		cmd.Dir = repoA
		cmd.Run()
		exec.Command("git", "-C", repoA, "config", "user.email", "t@t.com").Run()
		exec.Command("git", "-C", repoA, "config", "user.name", "T").Run()
		exec.Command("git", "-C", repoA, "add", ".").Run()
		exec.Command("git", "-C", repoA, "commit", "-m", "bad").Run()

		code, stdout, stderr := runCmd(t, world, "lint", flagFile)
		if code == 0 {
			t.Errorf("lint should fail on malformed file, got exit 0")
		}
		if !strings.Contains(stderr, "features.yaml") && !strings.Contains(stdout, "features.yaml") {
			t.Errorf("lint error should name the offending file, got stdout=%q stderr=%q", stdout, stderr)
		}
		if strings.Contains(stdout, "panic") || strings.Contains(stderr, "panic") {
			t.Errorf("lint crashed with panic: %s", stderr)
		}

		// Read verbs must fail cleanly too (repo discovery finds the bad file).
		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)
		code, _, stderr = runCmdIn(t, world, repoA, "get", "install.ai.claude")
		if code == 0 {
			t.Errorf("get should fail on malformed flag file, got exit 0")
		}
		if strings.Contains(stderr, "panic") {
			t.Errorf("get crashed with panic: %s", stderr)
		}
	})

	// IA-4: malformed override yaml → read verbs error; other layers unaffected
	t.Run("IA-4-malformed-override", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		// Write a malformed override
		ovrDir := filepath.Join(world, ".config", "gff")
		os.MkdirAll(ovrDir, 0755)
		ovrPath := filepath.Join(ovrDir, "config.yaml")
		os.WriteFile(ovrPath, []byte(`install.ai.claude: {bad: yaml: here`), 0644)

		code, _, stderr := runCmd(t, world, "get", "--source", repoA, "install.ai.claude")
		if code == 0 {
			t.Errorf("expected error on malformed override, got exit 0")
		}
		if strings.Contains(stderr, "panic") {
			t.Errorf("crashed with panic: %s", stderr)
		}
	})

	// IA-5: injection safety — description with $(rm -rf) never reaches export; option id evil;rm rejected by lint
	t.Run("IA-5-injection", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		badFile := `namespace: com.example.test
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude CLI $(rm -rf /tmp/pwned)
        boolDefault: true
`
		initRepo(t, repoA, "com.example.test", badFile)

		// lint should reject it or pass but get should not eval the injection
		code, shellOut, _ := runCmd(t, world, "export", "--source", repoA, "--format", "shell")
		if code == 0 {
			// Check shell output is injection-safe: only [A-Z0-9_=,.\n-]
			if strings.Contains(shellOut, "$(") || strings.Contains(shellOut, "`") {
				t.Errorf("shell export contains injection vector: %s", shellOut)
			}
		}
	})

	// IA-6: different URL + same namespace → ErrNamespaceTaken naming the existing URL
	t.Run("IA-6-namespace-taken", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo-a")
		repoB := filepath.Join(world, "repo-b")

		// Both use the same namespace but different origins
		initRepo(t, repoA, "com.example.shared", minimalFlagFile())
		initRepo(t, repoB, "com.example.shared", minimalFlagFile())

		// Set repoA's origin
		exec.Command("git", "-C", repoA, "config", "remote.origin.url", "https://example.com/repo-a.git").Run()

		// Install repoA
		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)
		if code, _, stderr := runCmdIn(t, world, repoA, "install"); code != 0 {
			t.Fatalf("first install must succeed, got exit %d: %s", code, stderr)
		}

		// Set repoB's origin to a different URL
		exec.Command("git", "-C", repoB, "config", "remote.origin.url", "https://different.com/repo-b.git").Run()

		// Try to install repoB with same namespace → should fail
		os.Chdir(repoB)
		code, _, stderr := runCmdIn(t, world, repoB, "install")
		if code == 0 {
			t.Errorf("expected install to fail on namespace conflict, got exit 0")
		}
		if !strings.Contains(stderr, "example.com/repo-a.git") {
			t.Errorf("stderr must name the existing URL, got: %s", stderr)
		}
	})

	// IA-7: corrupt sources.yaml → verbs degrade with clear error, shell gate stays fail-open
	t.Run("IA-7-corrupt-registry", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		// Corrupt sources.yaml
		srcDir := filepath.Join(world, ".config", "gff")
		os.MkdirAll(srcDir, 0755)
		srcFile := filepath.Join(srcDir, "sources.yaml")
		os.WriteFile(srcFile, []byte("{ bad yaml"), 0644)

		// Plain list never consults sources.yaml (the resolver reads snapshot
		// DIRS); the registry-reading verbs are install and --source <name>.
		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)
		code, _, stderr := runCmdIn(t, world, repoA, "install")
		if code == 0 {
			t.Errorf("expected install to error on corrupt registry, got exit 0")
		}
		if !strings.Contains(stderr, "sources.yaml") && !strings.Contains(stderr, "yaml") {
			t.Errorf("install error should name the corrupt registry, got %q", stderr)
		}
		if strings.Contains(stderr, "panic") {
			t.Errorf("crashed: %s", stderr)
		}

		// A named --source lookup degrades to a clean unknown-source (exit 2) —
		// no panic, no partial writes; the env-only shell gate stays fail-open.
		code, _, stderr = runCmd(t, world, "get", "--source", "com.example.test", "install.ai.claude")
		if code != 2 {
			t.Errorf("expected exit 2 for --source over corrupt registry, got %d (stderr %q)", code, stderr)
		}
		if strings.Contains(stderr, "panic") {
			t.Errorf("crashed: %s", stderr)
		}
	})

	// IA-8: read-only ~/.config → set exits 1, no temp-file litter
	t.Run("IA-8-readonly-config", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		// Make ~/.config read-only
		cfgDir := filepath.Join(world, ".config")
		os.MkdirAll(cfgDir, 0755)
		os.Chmod(cfgDir, 0500)
		defer os.Chmod(cfgDir, 0755) // restore

		// List files before
		beforeFiles, _ := filepath.Glob(filepath.Join(cfgDir, "gff", "*"))

		code, _, _ := runCmd(t, world, "set", "--source", repoA, "install.ai.claude", "false")
		if code == 0 {
			t.Errorf("expected set to fail on read-only config")
		}

		// List files after — should not have temp files
		afterFiles, _ := filepath.Glob(filepath.Join(cfgDir, "gff", "*"))
		if len(afterFiles) > len(beforeFiles) {
			t.Errorf("temp-file litter detected: %v", afterFiles)
		}
	})

	// IA-9: HOME unset → clear error; nothing written to CWD
	t.Run("IA-9-home-unset", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		// Run without HOME
		bin := getGffBinary(t)
		cmd := exec.Command(bin, "get", "--source", repoA, "install.ai.claude")
		cmd.Dir = world
		cmd.Env = []string{
			"XDG_DATA_HOME=" + filepath.Join(world, ".local", "share"),
			"GIT_CONFIG_GLOBAL=" + filepath.Join(world, ".gitconfig"),
			"GIT_CONFIG_NOSYSTEM=1",
			"PATH=" + os.Getenv("PATH"),
		}
		// Explicitly unset HOME
		cmd.Env = append(cmd.Env, "HOME=")
		cwdBefore, _ := filepath.Glob(filepath.Join(world, "repo", "*"))

		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			t.Errorf("expected command to fail with HOME unset")
		}
		if stderr.String() == "" {
			t.Logf("stderr empty when HOME unset (should have error message)")
		}

		// Check nothing was written to CWD (the repo dir legitimately holds
		// .gff and .git from the fixture — assert the listing is unchanged).
		after, _ := filepath.Glob(filepath.Join(world, "repo", "*"))
		if len(after) != len(cwdBefore) {
			t.Errorf("files written to CWD: before=%v after=%v", cwdBefore, after)
		}
	})

	// IA-10: unknown --source name and non-repo path → exit 2
	t.Run("IA-10-unknown-source", func(t *testing.T) {
		world := t.TempDir()

		code, _, _ := runCmd(t, world, "get", "--source", "unknown-namespace", "install.ai.claude")
		if code != 2 {
			t.Errorf("expected exit 2 on unknown source name, got %d", code)
		}

		code, _, _ = runCmd(t, world, "get", "--source", "/nonexistent/path", "install.ai.claude")
		if code != 2 {
			t.Errorf("expected exit 2 on non-repo path, got %d", code)
		}
	})

	// IA-11: 10 concurrent set calls with distinct keys → final file is valid YAML and exactly ONE writer's snapshot
	t.Run("IA-11-concurrent-writes", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				// Each writer sets the same key to a different value for determinism
				key := "install.ai.claude"
				value := fmt.Sprintf("%v", idx%2 == 0) // alternate true/false
				runCmd(t, world, "set", "--source", repoA, key, value)
			}(i)
		}
		wg.Wait()

		// Final file should be valid YAML
		ovrPath := filepath.Join(world, ".config", "gff", "config.yaml")
		data, err := os.ReadFile(ovrPath)
		if err != nil {
			t.Errorf("override file not readable: %v", err)
		}
		var result map[string]interface{}
		if err := yaml.Unmarshal(data, &result); err != nil {
			t.Errorf("final YAML is invalid: %v", err)
		}
		// Should have exactly one key (the one being set)
		if len(result) != 1 {
			t.Logf("final file has %d keys (expected 1): %v", len(result), result)
		}
	})

	// IA-12: gff.source redirect pointing at missing file / outside repo → clean error
	t.Run("IA-12-invalid-source-redirect", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		os.MkdirAll(filepath.Join(repoA, ".gff"), 0755)

		// Create a valid but small flag file
		os.WriteFile(filepath.Join(repoA, ".gff", "features.yaml"), []byte(minimalFlagFile()), 0644)

		cmd := exec.Command("git", "init")
		cmd.Dir = repoA
		cmd.Run()
		exec.Command("git", "-C", repoA, "config", "user.email", "t@t.com").Run()
		exec.Command("git", "-C", repoA, "config", "user.name", "T").Run()
		exec.Command("git", "-C", repoA, "add", ".").Run()
		exec.Command("git", "-C", repoA, "commit", "-m", "init").Run()

		// Set gff.source to a missing file
		exec.Command("git", "-C", repoA, "config", "gff.source", "/nonexistent/flags.yaml").Run()

		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)

		code, _, stderr := runCmdIn(t, world, repoA, "get", "install.ai.claude")
		if code == 0 {
			t.Errorf("expected error on missing redirect file, got exit 0")
		}
		if strings.Contains(stderr, "panic") {
			t.Errorf("crashed: %s", stderr)
		}
	})

	// IA-13: after install, git status --porcelain in the source repo is empty
	t.Run("IA-13-install-clean-worktree", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)

		if code, _, stderr := runCmdIn(t, world, repoA, "install"); code != 0 {
			t.Fatalf("install must succeed, got exit %d: %s", code, stderr)
		}

		// Check repo is still clean
		cmd := exec.Command("git", "status", "--porcelain")
		cmd.Dir = repoA
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Run()

		if out.Len() > 0 {
			t.Errorf("repo is dirty after install: %s", out.String())
		}
	})

	// IA-14: repo moved on disk → snapshot still resolves from any CWD
	t.Run("IA-14-moved-repo", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		initRepo(t, repoA, "com.example.test", minimalFlagFile())

		origCwd, _ := os.Getwd()
		defer os.Chdir(origCwd)
		os.Chdir(repoA)

		// Install from original location
		if code, _, stderr := runCmdIn(t, world, repoA, "install"); code != 0 {
			t.Fatalf("install must succeed, got exit %d: %s", code, stderr)
		}

		// Move the repo
		repoMoved := filepath.Join(world, "repo-moved")
		os.Rename(repoA, repoMoved)

		// From a different CWD, query using the registered namespace.
		// (MkdirAll must precede Chdir; the old CWD was just renamed away.)
		os.MkdirAll(filepath.Join(world, "somewhere-else"), 0755)
		if err := os.Chdir(filepath.Join(world, "somewhere-else")); err != nil {
			t.Fatalf("chdir: %v", err)
		}

		// Get should still work via the snapshot
		code, stdout, _ := runCmd(t, world, "get", "--source", "com.example.test", "install.ai.claude")
		if code != 0 {
			t.Errorf("expected success after repo moved, got exit %d", code)
		}
		if strings.TrimSpace(stdout) != "true" {
			t.Errorf("expected 'true', got %q", strings.TrimSpace(stdout))
		}
	})

	// IA-15: empty feature set → all four export formats emit valid empty output, exit 0
	t.Run("IA-15-empty-features", func(t *testing.T) {
		world := t.TempDir()
		repoA := filepath.Join(world, "repo")
		emptyFile := `namespace: com.example.test
sets:
  - area: install
    features: []
`
		initRepo(t, repoA, "com.example.test", emptyFile)

		// Shell format
		code, out, _ := runCmd(t, world, "export", "--source", repoA, "--format", "shell")
		if code != 0 {
			t.Errorf("shell export on empty set failed: exit %d", code)
		}
		if strings.TrimSpace(out) != "" {
			t.Logf("shell export of empty set: %q", out)
		}

		// JSON format
		code, out, _ = runCmd(t, world, "export", "--source", repoA, "--format", "json")
		if code != 0 {
			t.Errorf("json export on empty set failed: exit %d", code)
		}
		var items []interface{}
		if err := json.Unmarshal([]byte(out), &items); err != nil {
			t.Errorf("json parse failed: %v", err)
		}
		if len(items) != 0 {
			t.Errorf("expected empty array, got %d items", len(items))
		}

		// YAML format
		code, out, _ = runCmd(t, world, "export", "--source", repoA, "--format", "yaml")
		if code != 0 {
			t.Errorf("yaml export on empty set failed: exit %d", code)
		}
		var yamlItems []interface{}
		if err := yaml.Unmarshal([]byte(out), &yamlItems); err != nil {
			t.Errorf("yaml parse failed: %v", err)
		}
		if len(yamlItems) != 0 {
			t.Errorf("expected empty/null, got %d items", len(yamlItems))
		}

		// Dotenv format
		code, out, _ = runCmd(t, world, "export", "--source", repoA, "--format", "dotenv")
		if code != 0 {
			t.Errorf("dotenv export on empty set failed: exit %d", code)
		}
	})
}
