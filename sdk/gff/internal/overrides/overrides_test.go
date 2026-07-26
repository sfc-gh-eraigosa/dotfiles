package overrides

import (
	"os"
	"path/filepath"
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"gopkg.in/yaml.v3"
)

func TestWriteCreatesFile0600(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write a bool value
	v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "test.key", v); err != nil {
		t.Fatal(err)
	}

	// Verify file exists with 0600 permissions
	info, err := os.Stat(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("file mode %o, want %o", got, want)
	}

	// Verify content
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["test.key"]; !ok || v != true {
		t.Errorf("expected test.key: true, got %v", m)
	}
}

func TestWritePreservesOtherKeys(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write first key
	v1 := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "key1", v1); err != nil {
		t.Fatal(err)
	}

	// Write second key
	v2 := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}
	if err := Write(p, "key2", v2); err != nil {
		t.Fatal(err)
	}

	// Verify both keys exist
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["key1"]; !ok || v != true {
		t.Errorf("key1 missing or wrong value: %v", m)
	}
	if v, ok := m["key2"]; !ok || v != false {
		t.Errorf("key2 missing or wrong value: %v", m)
	}
}

func TestUnsetRemovesKey(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write a key
	v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "key1", v); err != nil {
		t.Fatal(err)
	}

	// Unset it
	if err := Unset(p, "key1"); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["key1"]; ok {
		t.Errorf("key1 should be gone, got %v", m)
	}
}

func TestUnsetKeepsOtherKeys(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write two keys
	v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "key1", v); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, "key2", v); err != nil {
		t.Fatal(err)
	}

	// Unset one
	if err := Unset(p, "key1"); err != nil {
		t.Fatal(err)
	}

	// Verify key2 remains
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["key2"]; !ok || v != true {
		t.Errorf("key2 should remain, got %v", m)
	}
}

func TestUnsetNonExistentKeyIsNoop(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Unset a key that was never written
	if err := Unset(p, "nonexistent"); err != nil {
		t.Fatalf("Unset nonexistent key should not error: %v", err)
	}

	// File should not exist
	_, err := os.Stat(p.UserOverride)
	if err == nil {
		t.Fatal("file should not exist after Unset on nonexistent key")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteFileAtomicCreatesParents(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "a", "b", "c", "file.yaml")

	data := []byte("test: value\n")
	if err := WriteFileAtomic(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify file and parents exist
	if content, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	} else if string(content) != "test: value\n" {
		t.Errorf("content mismatch: %s", string(content))
	}
}

func TestWriteFileAtomicRespectsPerm(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "file.yaml")

	data := []byte("test\n")
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("file mode %o, want %o", got, want)
	}
}

func TestWriteFileAtomicErrorOnUnwritableDir(t *testing.T) {
	tmpdir := t.TempDir()
	rodir := filepath.Join(tmpdir, "readonly")
	if err := os.Mkdir(rodir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rodir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(rodir, 0o755) // cleanup

	path := filepath.Join(rodir, "file.yaml")
	if err := WriteFileAtomic(path, []byte("test\n"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic should fail on readonly dir")
	}
}

func TestMarshalBoolValue(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write bool values (true and false)
	v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "key.bool.true", v); err != nil {
		t.Fatal(err)
	}
	v = &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}
	if err := Write(p, "key.bool.false", v); err != nil {
		t.Fatal(err)
	}

	// Verify format
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]any)
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if v, ok := m["key.bool.true"]; !ok || v != true {
		t.Errorf("bool true mismatch: %v", m)
	}
	if v, ok := m["key.bool.false"]; !ok || v != false {
		t.Errorf("bool false mismatch: %v", m)
	}
}

func TestMarshalChoiceSingleValue(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write single choice selection (stored as bare string)
	v := &gffv1.Value{
		Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: []string{"option1"}},
		},
	}
	if err := Write(p, "choice.single", v); err != nil {
		t.Fatal(err)
	}

	// Verify it's stored as a bare string
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !contains(content, "choice.single: option1") {
		t.Errorf("single choice not found in output:\n%s", content)
	}
}

func TestMarshalChoiceMultiValue(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write multi choice selection (stored as list)
	v := &gffv1.Value{
		Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: []string{"opt1", "opt2"}},
		},
	}
	if err := Write(p, "choice.multi", v); err != nil {
		t.Fatal(err)
	}

	// Verify it's stored as a list
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	// Should have the list format
	if !contains(content, "choice.multi:") {
		t.Errorf("choice.multi not found in output:\n%s", content)
	}
	// Both options should appear
	if !contains(content, "opt1") || !contains(content, "opt2") {
		t.Errorf("options not found in output:\n%s", content)
	}
}

func TestMarshalChoiceEmptyValue(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write empty choice selection (stored as empty list)
	v := &gffv1.Value{
		Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: []string{}},
		},
	}
	if err := Write(p, "choice.empty", v); err != nil {
		t.Fatal(err)
	}

	// Verify file was written
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}
	// Just verify it doesn't error and file exists
	if len(data) == 0 {
		t.Errorf("empty choice file should not be empty")
	}
}

func TestWriteFileAtomicRenameSucceeds(t *testing.T) {
	tmpdir := t.TempDir()
	path := filepath.Join(tmpdir, "file.yaml")

	// Write initial file
	data1 := []byte("first\n")
	if err := WriteFileAtomic(path, data1, 0o644); err != nil {
		t.Fatal(err)
	}

	// Overwrite with rename (truncate-overwrite behavior)
	data2 := []byte("second\n")
	if err := WriteFileAtomic(path, data2, 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify final content
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "second\n" {
		t.Errorf("file should contain 'second', got %q", string(content))
	}
}

func TestWriteFileAtomicTempWriteError(t *testing.T) {
	tmpdir := t.TempDir()
	// Create a read-only directory to trigger write error
	rodir := filepath.Join(tmpdir, "readonly")
	if err := os.Mkdir(rodir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(rodir, 0o444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(rodir, 0o755)

	path := filepath.Join(rodir, "file.yaml")
	if err := WriteFileAtomic(path, []byte("test\n"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic should fail on readonly dir")
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestMarshalDeterministic(t *testing.T) {
	tmpdir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(tmpdir, "config.yaml")}

	// Write keys in non-alphabetical order
	keys := []string{"zzz", "aaa", "mmm"}
	for _, k := range keys {
		v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
		if err := Write(p, k, v); err != nil {
			t.Fatal(err)
		}
	}

	// Read and verify order is alphabetical
	data, err := os.ReadFile(p.UserOverride)
	if err != nil {
		t.Fatal(err)
	}

	// Simple check: keys should appear in alphabetical order in the YAML
	content := string(data)
	aaa_pos := len(content)
	mmm_pos := len(content)
	zzz_pos := len(content)

	for i := range content {
		if i < len(content)-3 && content[i:i+3] == "aaa" && aaa_pos == len(content) {
			aaa_pos = i
		}
		if i < len(content)-3 && content[i:i+3] == "mmm" && mmm_pos == len(content) {
			mmm_pos = i
		}
		if i < len(content)-3 && content[i:i+3] == "zzz" && zzz_pos == len(content) {
			zzz_pos = i
		}
	}

	if !(aaa_pos < mmm_pos && mmm_pos < zzz_pos) {
		t.Errorf("keys not in alphabetical order: aaa@%d, mmm@%d, zzz@%d", aaa_pos, mmm_pos, zzz_pos)
	}
}

func TestWriteFileAtomicUnwritableDir(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.MkdirAll(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	err := WriteFileAtomic(filepath.Join(locked, "x.yaml"), []byte("a: 1\n"), 0o600)
	if err == nil {
		t.Fatal("want error writing into 0500 dir")
	}
	entries, _ := os.ReadDir(locked)
	if len(entries) != 0 {
		t.Fatalf("no temp-file litter expected, got %v", entries)
	}
}

func TestWriteFileAtomicOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(target, []byte("old: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(target, []byte("new: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "new: true\n" {
		t.Fatalf("want truncate-and-overwrite, got %q err %v", got, err)
	}
}

func TestWriteMalformedExistingOverrides(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{UserOverride: filepath.Join(dir, "config.yaml")}
	if err := os.WriteFile(p.UserOverride, []byte("nested:\n  map: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: true}}
	if err := Write(p, "a.b.c", v); err == nil {
		t.Fatal("want error on malformed existing override file")
	}
}

func TestWriteFileAtomicMkdirFails(t *testing.T) {
	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(blocked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })
	err := WriteFileAtomic(filepath.Join(blocked, "sub", "x.yaml"), []byte("a: 1\n"), 0o600)
	if err == nil {
		t.Fatal("want mkdir error under 0500 parent")
	}
}

func TestWriteFileAtomicRenameOntoDirFails(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "iamadir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	err := WriteFileAtomic(target, []byte("a: 1\n"), 0o600)
	if err == nil {
		t.Fatal("want rename error onto existing directory")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() != "iamadir" {
			t.Fatalf("temp litter left behind: %s", e.Name())
		}
	}
}
