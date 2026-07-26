// Package gff is the public runtime SDK for git-persisted feature flags.
//
// It is a thin wrapper over internal/{paths,resolve,registry,gitx} that exposes
// only Go-exportable types so external importers can call it without importing
// any internal package (which would cause a compile error outside this module).
//
// # WithRoot layer derivation
//
// WithRoot(dir) derives every layer path under dir using fixed sub-paths:
//
//	<dir>/system-snapshots/   SystemSnapshotDir  (read-only, admin-provisioned)
//	<dir>/user-snapshots/     UserSnapshotDir    (written by gff install)
//	<dir>/system-config.yaml  SystemOverride     (read-only, admin-provisioned)
//	<dir>/user-config.yaml    UserOverride       (written by gff set)
//	<dir>/sources.yaml        RegistryFile       (written by gff install)
//	<dir>/work/               WorkDir            (used for git-repo discovery)
//
// Tests call WithRoot(t.TempDir()) and write fixture files at these sub-paths;
// the production binary uses the Default() paths (~/.config/gff/…, etc.).
package gff

import (
	"errors"
	"fmt"
	"path/filepath"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/registry"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
)

// ── Options ──────────────────────────────────────────────────────────────────

// config holds the resolved SDK configuration after applying Option functions.
type config struct {
	paths  paths.Paths
	source string
	hasP   bool // true when WithRoot has been applied
}

// Option is a functional option for the SDK accessor functions.
type Option func(*config)

// WithRoot derives every layer path under dir (see package doc for the mapping).
// Use it in tests: WithRoot(t.TempDir()). Production callers typically omit it
// and let the SDK use Default() paths.
func WithRoot(dir string) Option {
	return func(c *config) {
		c.paths = paths.Paths{
			SystemSnapshotDir: filepath.Join(dir, "system-snapshots"),
			UserSnapshotDir:   filepath.Join(dir, "user-snapshots"),
			SystemOverride:    filepath.Join(dir, "system-config.yaml"),
			UserOverride:      filepath.Join(dir, "user-config.yaml"),
			RegistryFile:      filepath.Join(dir, "sources.yaml"),
			WorkDir:           filepath.Join(dir, "work"),
		}
		c.hasP = true
	}
}

// WithSource scopes resolution to a registered source name or local repo path
// (same semantics as the --source CLI flag).
func WithSource(nameOrPath string) Option {
	return func(c *config) { c.source = nameOrPath }
}

// ── resolver construction ─────────────────────────────────────────────────────

func buildResolver(opts []Option) (*resolve.Resolver, error) {
	c := &config{}
	for _, o := range opts {
		o(c)
	}
	if !c.hasP {
		p, err := paths.Default()
		if err != nil {
			return nil, fmt.Errorf("gff: %w", err)
		}
		c.paths = p
	}
	r := resolve.New(c.paths, gitx.ExecRunner{}, c.source)
	r.S = &registry.Registry{P: c.paths}
	return r, nil
}

// ── Bool ─────────────────────────────────────────────────────────────────────

// Bool returns the effective boolean value of key across all 5 layers.
// Returns an error for unknown keys or if key is a choice flag.
func Bool(key string, opts ...Option) (bool, error) {
	r, err := buildResolver(opts)
	if err != nil {
		return false, err
	}
	res, err := r.Resolve(key)
	if err != nil {
		return false, err
	}
	bv, ok := res.Value.GetKind().(*gffv1.Value_BoolValue)
	if !ok {
		return false, fmt.Errorf("gff.Bool: key %q is not a bool flag", key)
	}
	return bv.BoolValue, nil
}

// ── Selected ─────────────────────────────────────────────────────────────────

// Selected returns the effective selected option ids for a choice flag.
// Returns an error for unknown keys or if key is a bool flag.
func Selected(key string, opts ...Option) ([]string, error) {
	r, err := buildResolver(opts)
	if err != nil {
		return nil, err
	}
	res, err := r.Resolve(key)
	if err != nil {
		return nil, err
	}
	cv, ok := res.Value.GetKind().(*gffv1.Value_ChoiceValue)
	if !ok {
		return nil, fmt.Errorf("gff.Selected: key %q is not a choice flag", key)
	}
	return cv.ChoiceValue.GetSelected(), nil
}

// ── IsSelected ───────────────────────────────────────────────────────────────

// IsSelected reports whether optionID is currently selected for a choice flag.
// Returns an error for unknown keys, unknown option ids, or bool flags.
func IsSelected(key, optionID string, opts ...Option) (bool, error) {
	r, err := buildResolver(opts)
	if err != nil {
		return false, err
	}
	res, err := r.Resolve(key)
	if err != nil {
		return false, err
	}

	cd := res.Feature.GetChoiceDefault()
	if cd == nil {
		return false, fmt.Errorf("gff.IsSelected: key %q is not a choice flag", key)
	}

	// Validate the option id against the definition.
	validIDs := make(map[string]bool, len(cd.GetOptions()))
	for _, opt := range cd.GetOptions() {
		validIDs[opt.GetId()] = true
	}
	if !validIDs[optionID] {
		return false, fmt.Errorf("gff.IsSelected: key %q: %w: option id %q not defined",
			key, resolve.ErrUnknownOption, optionID)
	}

	cv, ok := res.Value.GetKind().(*gffv1.Value_ChoiceValue)
	if !ok {
		return false, nil
	}
	for _, id := range cv.ChoiceValue.GetSelected() {
		if id == optionID {
			return true, nil
		}
	}
	return false, nil
}

// ── typed value accessors ─────────────────────────────────────────────────────

// resolveSelectedOptions returns the currently-selected ChoiceOption objects
// for a choice flag, or an error if key is not a choice flag or is unknown.
func resolveSelectedOptions(key string, opts []Option) ([]*gffv1.ChoiceOption, error) {
	r, err := buildResolver(opts)
	if err != nil {
		return nil, err
	}
	res, err := r.Resolve(key)
	if err != nil {
		return nil, err
	}
	cd := res.Feature.GetChoiceDefault()
	if cd == nil {
		return nil, fmt.Errorf("gff: key %q is not a choice flag", key)
	}

	cv, ok := res.Value.GetKind().(*gffv1.Value_ChoiceValue)
	if !ok {
		return nil, nil
	}

	// Index options by id for fast lookup.
	byID := make(map[string]*gffv1.ChoiceOption, len(cd.GetOptions()))
	for _, opt := range cd.GetOptions() {
		byID[opt.GetId()] = opt
	}

	var result []*gffv1.ChoiceOption
	for _, id := range cv.ChoiceValue.GetSelected() {
		if opt, ok := byID[id]; ok {
			result = append(result, opt)
		}
	}
	return result, nil
}

// actualValueType returns a human-readable type name for a ChoiceOption's value.
func actualValueType(opts []*gffv1.ChoiceOption) string {
	if len(opts) == 0 {
		return "none"
	}
	switch opts[0].GetValue().(type) {
	case *gffv1.ChoiceOption_IntValue:
		return "int"
	case *gffv1.ChoiceOption_FloatValue:
		return "float"
	case *gffv1.ChoiceOption_StringValue:
		return "string"
	case *gffv1.ChoiceOption_BoolValue:
		return "bool"
	default:
		return "none"
	}
}

// IntValues returns the int64 payload of each currently-selected option.
// Errors when the feature's value type is not int (naming the actual type).
func IntValues(key string, opts ...Option) ([]int64, error) {
	selected, err := resolveSelectedOptions(key, opts)
	if err != nil {
		return nil, err
	}
	// Check type of the first option (lint enforces homogeneity).
	if at := actualValueType(selected); at != "int" && at != "none" {
		return nil, fmt.Errorf("gff.IntValues: key %q has value type %q, not int", key, at)
	}
	out := make([]int64, 0, len(selected))
	for _, opt := range selected {
		iv, ok := opt.GetValue().(*gffv1.ChoiceOption_IntValue)
		if !ok {
			return nil, fmt.Errorf("gff.IntValues: key %q: option %q has no int value", key, opt.GetId())
		}
		out = append(out, iv.IntValue)
	}
	return out, nil
}

// FloatValues returns the float64 payload of each currently-selected option.
// Errors when the feature's value type is not float (naming the actual type).
func FloatValues(key string, opts ...Option) ([]float64, error) {
	selected, err := resolveSelectedOptions(key, opts)
	if err != nil {
		return nil, err
	}
	if at := actualValueType(selected); at != "float" && at != "none" {
		return nil, fmt.Errorf("gff.FloatValues: key %q has value type %q, not float", key, at)
	}
	out := make([]float64, 0, len(selected))
	for _, opt := range selected {
		fv, ok := opt.GetValue().(*gffv1.ChoiceOption_FloatValue)
		if !ok {
			return nil, fmt.Errorf("gff.FloatValues: key %q: option %q has no float value", key, opt.GetId())
		}
		out = append(out, fv.FloatValue)
	}
	return out, nil
}

// StringValues returns the string payload of each currently-selected option.
// Errors when the feature's value type is not string (naming the actual type).
func StringValues(key string, opts ...Option) ([]string, error) {
	selected, err := resolveSelectedOptions(key, opts)
	if err != nil {
		return nil, err
	}
	if at := actualValueType(selected); at != "string" && at != "none" {
		return nil, fmt.Errorf("gff.StringValues: key %q has value type %q, not string", key, at)
	}
	out := make([]string, 0, len(selected))
	for _, opt := range selected {
		sv, ok := opt.GetValue().(*gffv1.ChoiceOption_StringValue)
		if !ok {
			return nil, fmt.Errorf("gff.StringValues: key %q: option %q has no string value", key, opt.GetId())
		}
		out = append(out, sv.StringValue)
	}
	return out, nil
}

// BoolValues returns the bool payload of each currently-selected option.
// Errors when the feature's value type is not bool (naming the actual type).
func BoolValues(key string, opts ...Option) ([]bool, error) {
	selected, err := resolveSelectedOptions(key, opts)
	if err != nil {
		return nil, err
	}
	if at := actualValueType(selected); at != "bool" && at != "none" {
		return nil, fmt.Errorf("gff.BoolValues: key %q has value type %q, not bool", key, at)
	}
	out := make([]bool, 0, len(selected))
	for _, opt := range selected {
		bv, ok := opt.GetValue().(*gffv1.ChoiceOption_BoolValue)
		if !ok {
			return nil, fmt.Errorf("gff.BoolValues: key %q: option %q has no bool value", key, opt.GetId())
		}
		out = append(out, bv.BoolValue)
	}
	return out, nil
}

// Ensure resolve sentinels are re-exported for callers that want to errors.Is
// against them without importing internal/resolve directly.
var (
	// ErrUnknownKey is re-exported from internal/resolve.
	ErrUnknownKey = resolve.ErrUnknownKey
	// ErrUnknownOption is re-exported from internal/resolve.
	ErrUnknownOption = resolve.ErrUnknownOption
	// ErrUnknownSource is re-exported from internal/resolve.
	ErrUnknownSource = resolve.ErrUnknownSource
	// ErrWrongFlagType is re-exported from internal/resolve.
	ErrWrongFlagType = resolve.ErrWrongFlagType
)

// Verify that the sentinel vars are used (suppress "declared and not used").
var _ = errors.Is(ErrUnknownKey, ErrUnknownKey)
