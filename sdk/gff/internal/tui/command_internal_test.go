package tui

import (
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolItem(path string) resolve.Resolved {
	return resolve.Resolved{Feature: &gffv1.Feature{Path: path, Default: &gffv1.Feature_BoolDefault{BoolDefault: true}}}
}

func choiceItem(path string, mode gffv1.ChoiceMode, ids ...string) resolve.Resolved {
	cd := &gffv1.ChoiceDefault{Mode: mode}
	for _, id := range ids {
		cd.Options = append(cd.Options, &gffv1.ChoiceOption{Id: id})
	}
	return resolve.Resolved{Feature: &gffv1.Feature{Path: path, Default: &gffv1.Feature_ChoiceDefault{ChoiceDefault: cd}}}
}

func TestParseValueBool(t *testing.T) {
	v, err := parseValue(boolItem("a.b"), "false")
	require.NoError(t, err)
	assert.False(t, v.GetBoolValue())
	_, err = parseValue(boolItem("a.b"), "yes")
	require.EqualError(t, err, `value for a.b must be true or false, got "yes"`)
}

func TestParseValueChoice(t *testing.T) {
	single := choiceItem("p.m", gffv1.ChoiceMode_CHOICE_MODE_SINGLE, "auto", "apt")
	multi := choiceItem("p.n", gffv1.ChoiceMode_CHOICE_MODE_MULTI, "a", "b", "c")
	v, err := parseValue(single, "apt")
	require.NoError(t, err)
	assert.Equal(t, []string{"apt"}, v.GetChoiceValue().GetSelected())
	v, err = parseValue(multi, "a,c")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "c"}, v.GetChoiceValue().GetSelected())
	_, err = parseValue(single, "auto,apt")
	require.EqualError(t, err, "p.m is a single-choice flag: give exactly one id")
	_, err = parseValue(single, "brew")
	require.EqualError(t, err, `unknown option "brew" for p.m`)
	_, err = parseValue(multi, "a,a")
	require.EqualError(t, err, `duplicate option "a" for p.n`)
}

func TestFindKeyScopedAndQualified(t *testing.T) {
	m := &Model{items: []resolve.Resolved{
		boolItem("x.y.z").WithNamespace("one"),
		boolItem("x.y.z").WithNamespace("two"),
		boolItem("only.here").WithNamespace("two"),
	}, scopeNS: "two"}
	idx, err := m.findKey("only.here")
	require.NoError(t, err)
	assert.Equal(t, 2, idx)
	idx, err = m.findKey("x.y.z")
	require.NoError(t, err, "ambiguous bare path resolves to the breadcrumb namespace")
	assert.Equal(t, 1, idx)
	idx, err = m.findKey("one:x.y.z")
	require.NoError(t, err)
	assert.Equal(t, 0, idx)
	_, err = m.findKey("nope")
	require.EqualError(t, err, "unknown key: nope")
	m.scopeNS = ""
	_, err = m.findKey("x.y.z")
	require.EqualError(t, err, `ambiguous key "x.y.z": qualify it as <namespace>:x.y.z`)
}
