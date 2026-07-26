package gff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const typedChoicesYAML = `namespace: com.example.typed
sets:
  - area: tune
    features:
      - path: tune.retry.count
        description: Retry count
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: three, description: Three, intValue: 3, selected: true}
            - {id: five, description: Five, intValue: 5}
      - path: tune.retry.backoff
        description: Backoff factor
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: slow, description: Slow, floatValue: 1.5, selected: true}
            - {id: fast, description: Fast, floatValue: 0.5}
      - path: tune.retry.jitter
        description: Jitter on/off payloads
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: on-read, description: Jitter reads, boolValue: true, selected: true}
            - {id: on-write, description: Jitter writes, boolValue: false, selected: true}
      - path: tune.retry.flag
        description: A plain bool
        boolDefault: true
`

// typedRoot writes the typed fixture into a WithRoot layout (user snapshot layer).
func typedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	snapDir := filepath.Join(root, "user-snapshots")
	require.NoError(t, os.MkdirAll(snapDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(snapDir, "com.example.typed.yaml"), []byte(typedChoicesYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work"), 0o755))
	return root
}

func TestTypedValueAccessors(t *testing.T) {
	root := typedRoot(t)

	ints, err := IntValues("tune.retry.count", WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []int64{3}, ints)

	floats, err := FloatValues("tune.retry.backoff", WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []float64{1.5}, floats)

	bools, err := BoolValues("tune.retry.jitter", WithRoot(root))
	require.NoError(t, err)
	assert.Equal(t, []bool{true, false}, bools)
}

func TestTypedAccessorWrongTypeNamesActual(t *testing.T) {
	root := typedRoot(t)

	_, err := FloatValues("tune.retry.count", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"int"`, "error must name the actual type")

	_, err = IntValues("tune.retry.backoff", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"float"`)

	_, err = StringValues("tune.retry.jitter", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"bool"`)

	_, err = BoolValues("tune.retry.count", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"int"`)
}

func TestAccessorTypeMismatches(t *testing.T) {
	root := typedRoot(t)

	_, err := Bool("tune.retry.count", WithRoot(root))
	require.Error(t, err, "Bool on a choice flag errors")

	_, err = Selected("tune.retry.flag", WithRoot(root))
	require.Error(t, err, "Selected on a bool flag errors")

	_, err = Selected("tune.zz.zz", WithRoot(root))
	assert.ErrorIs(t, err, ErrUnknownKey)

	_, err = IsSelected("tune.retry.flag", "x", WithRoot(root))
	require.Error(t, err, "IsSelected on a bool flag errors")

	_, err = IsSelected("tune.retry.count", "nope", WithRoot(root))
	assert.ErrorIs(t, err, ErrUnknownOption)

	sel, err := IsSelected("tune.retry.count", "five", WithRoot(root))
	require.NoError(t, err)
	assert.False(t, sel, "non-default option is not selected")

	_, err = IntValues("tune.zz.zz", WithRoot(root))
	assert.ErrorIs(t, err, ErrUnknownKey)
}

// TestDefaultPathsBranch exercises buildResolver's paths.Default() arm with a
// hermetic HOME so no real user state is read or written.
func TestDefaultPathsBranch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")

	_, err := Bool("no.such.key")
	assert.ErrorIs(t, err, ErrUnknownKey)
}

// TestWithSourceLocalPath resolves a repo by local path from an unrelated root.
func TestWithSourceLocalPath(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work"), 0o755))

	repo := filepath.Join(root, "elsewhere", "repo")
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".gff"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".gff", "features.yaml"), []byte(typedChoicesYAML), 0o644))

	v, err := Bool("tune.retry.flag", WithRoot(root), WithSource(repo))
	require.NoError(t, err)
	assert.True(t, v)

	_, err = Bool("tune.retry.flag", WithRoot(root), WithSource(filepath.Join(root, "not-a-repo")))
	assert.ErrorIs(t, err, ErrUnknownSource)
}

// TestWithSourceRegisteredName resolves via a registry snapshot fixture.
func TestWithSourceRegisteredName(t *testing.T) {
	root := typedRoot(t)
	reg := "sources:\n  - namespace: com.example.typed\n    url: https://example.com/typed\n    commit: abc1234\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "sources.yaml"), []byte(reg), 0o644))

	v, err := Bool("tune.retry.flag", WithRoot(root), WithSource("com.example.typed"))
	require.NoError(t, err)
	assert.True(t, v)

	_, err = Bool("tune.retry.flag", WithRoot(root), WithSource("com.example.unknown"))
	assert.ErrorIs(t, err, ErrUnknownSource)
}

const mixedTypedYAML = `namespace: com.example.mixed
sets:
  - area: tune
    features:
      - path: tune.mix.multi
        description: Heterogeneous payloads (lint would flag; resolver tolerates)
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: f-one, description: A float, floatValue: 1.5, selected: true}
            - {id: s-two, description: A string, stringValue: two, selected: true}
      - path: tune.mix.untyped
        description: Options with no typed payload
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: plain, description: No payload, selected: true}
`

func TestTypedAccessorHeterogeneousLoopMismatch(t *testing.T) {
	root := t.TempDir()
	snapDir := filepath.Join(root, "user-snapshots")
	require.NoError(t, os.MkdirAll(snapDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(snapDir, "com.example.mixed.yaml"), []byte(mixedTypedYAML), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "work"), 0o755))

	// First option is float, second is string: FloatValues passes the
	// first-option type check but fails inside the loop on option 2.
	_, err := FloatValues("tune.mix.multi", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no float value")

	// Untyped options: actualValueType "none" passes the first-option type
	// check, then each accessor errors in the loop naming the payload-less option.
	_, err = IntValues("tune.mix.untyped", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no int value")

	_, err = StringValues("tune.mix.untyped", WithRoot(root))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no string value")
}
