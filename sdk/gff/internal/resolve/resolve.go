// Package resolve implements the 5-layer feature-flag resolver.
//
// Layers (lowest to highest priority):
//
//  1. SystemSnapshot  – every *.yaml|*.json file in P.SystemSnapshotDir
//  2. UserSnapshot    – every *.yaml|*.json file in P.UserSnapshotDir
//  3. RepoLive        – live feature file in the repo pointed to by P.WorkDir / Resolver.Source
//  4. SystemOverride  – sparse override file at P.SystemOverride
//  5. UserOverride    – sparse override file at P.UserOverride
package resolve

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/schema"
	"google.golang.org/protobuf/encoding/protojson"
)

// Layer identifies which resolution layer provided a feature's effective value.
type Layer int

const (
	LayerNone           Layer = iota // 0 – no value (feature absent)
	LayerSystemSnapshot              // 1 – system-wide snapshot directory
	LayerUserSnapshot                // 2 – per-user snapshot directory
	LayerRepoLive                    // 3 – live repo feature file
	LayerSystemOverride              // 4 – system-wide sparse override file
	LayerUserOverride                // 5 – per-user sparse override file
)

// String returns the canonical string name of the layer.
func (l Layer) String() string {
	switch l {
	case LayerNone:
		return "none"
	case LayerSystemSnapshot:
		return "system-snapshot"
	case LayerUserSnapshot:
		return "user-snapshot"
	case LayerRepoLive:
		return "repo-live"
	case LayerSystemOverride:
		return "system-override"
	case LayerUserOverride:
		return "user-override"
	default:
		return fmt.Sprintf("layer(%d)", int(l))
	}
}

// Sentinel errors returned by the resolver.
var (
	// ErrUnknownKey is returned when a flag key does not exist in any definition layer.
	ErrUnknownKey = errors.New("unknown flag key")
	// ErrUnknownSource is returned when Resolver.Source names a source that cannot be found.
	ErrUnknownSource = errors.New("unknown source")
	// ErrUnknownOption is returned when a choice override references an undefined option id,
	// or when a CHOICE_MODE_SINGLE flag receives more than one selected id.
	ErrUnknownOption = errors.New("unknown option id")
	// ErrWrongFlagType is returned when an override value type does not match the flag type
	// (e.g. a bool override on a choice flag, or vice versa).
	ErrWrongFlagType = errors.New("flag type not expressible by this verb")
)

// SourceLookup resolves a registered source name to a snapshot file path.
type SourceLookup interface {
	Snapshot(name string) (path string, ok bool)
}

// Resolved is the outcome of resolving one feature flag.
type Resolved struct {
	Feature   *gffv1.Feature
	Value     *gffv1.Value
	Layer     Layer
	namespace string // set internally; exposed via JSON()
}

// ResolvedJSON is the JSON-serialisable projection of a Resolved.
type ResolvedJSON struct {
	Namespace   string          `json:"namespace"`
	Path        string          `json:"path"`
	Description string          `json:"description"`
	Type        string          `json:"type"`    // "bool" | "choice"
	Layer       string          `json:"layer"`   // Layer.String()
	Value       json.RawMessage `json:"value"`   // protojson of gffv1.Value
	Feature     json.RawMessage `json:"feature"` // protojson of gffv1.Feature
}

// JSON converts r into a JSON-friendly struct. Value and Feature are encoded
// with protojson so proto oneofs are represented correctly.
// Namespace returns the owning source's reverse-DNS identity for this
// resolution (additive accessor; the field itself stays unexported).
func (r Resolved) Namespace() string { return r.namespace }

// WithValue returns a copy of r carrying a new effective value and winning
// layer while PRESERVING the unexported namespace. Callers that rebuild a
// Resolved after a write (e.g. the TUI's optimistic refresh) must use this —
// a bare struct literal silently drops the namespace and breaks any
// namespace-scoped filtering downstream.
func (r Resolved) WithValue(v *gffv1.Value, l Layer) Resolved {
	r.Value = v
	r.Layer = l
	return r
}

func (r Resolved) JSON() (ResolvedJSON, error) {
	valBytes, err := protojson.Marshal(r.Value)
	if err != nil {
		return ResolvedJSON{}, fmt.Errorf("resolve.JSON: marshal value: %w", err)
	}
	featBytes, err := protojson.Marshal(r.Feature)
	if err != nil {
		return ResolvedJSON{}, fmt.Errorf("resolve.JSON: marshal feature: %w", err)
	}

	typ := "bool"
	if r.Feature.GetChoiceDefault() != nil {
		typ = "choice"
	}

	return ResolvedJSON{
		Namespace:   r.namespace,
		Path:        r.Feature.GetPath(),
		Description: r.Feature.GetDescription(),
		Type:        typ,
		Layer:       r.Layer.String(),
		Value:       json.RawMessage(valBytes),
		Feature:     json.RawMessage(featBytes),
	}, nil
}

// Resolver resolves feature flags across all 5 layers.
type Resolver struct {
	P      paths.Paths
	R      gitx.Runner
	S      SourceLookup // nil => named --source lookups return ErrUnknownSource
	Source string       // "" = CWD discovery; local path or registered name
}

// New creates a Resolver with the given paths, runner, and source specifier.
// The S field (SourceLookup) defaults to nil; wire it via r.S after creation.
func New(p paths.Paths, r gitx.Runner, source string) *Resolver {
	return &Resolver{P: p, R: r, Source: source}
}

// defKey is the internal index key: (namespace, feature path).
type defKey struct {
	namespace string
	path      string
}

// definition is an indexed feature with its origin layer.
type definition struct {
	feature   *gffv1.Feature
	layer     Layer
	namespace string
}

// resolvedState is the full resolution result.
type resolvedState struct {
	defs    []definition   // all definitions, in definition-layer order (last wins)
	byKey   map[defKey]int // defKey → index in defs
	sysOvr  map[string]*gffv1.Value
	usrOvr  map[string]*gffv1.Value
	focusNS string // namespace unqualified keys bind to first (§3.2):
	// the CWD repo's flag file or the --source target

	// defHistory retains every definition layer's entry for a key (byKey/defs
	// keep only the winner). Feeds Explain's per-layer provenance story.
	defHistory map[defKey]map[Layer]definition
}

// load builds the resolved state from all layers.
func (r *Resolver) load() (*resolvedState, error) {
	st := &resolvedState{
		byKey:      make(map[defKey]int),
		defHistory: make(map[defKey]map[Layer]definition),
	}

	// ── definition layers ────────────────────────────────────────────────────

	// Layer 1: system snapshot dir.
	if err := r.loadSnapshotDir(r.P.SystemSnapshotDir, LayerSystemSnapshot, st); err != nil {
		return nil, err
	}

	// Layer 2: user snapshot dir.
	if err := r.loadSnapshotDir(r.P.UserSnapshotDir, LayerUserSnapshot, st); err != nil {
		return nil, err
	}

	// Layer 3: live repo file.
	livePath, liveLayer, err := r.resolveLivePath()
	if err != nil {
		return nil, err
	}
	if livePath != "" {
		ns, err := r.loadFile(livePath, liveLayer, st)
		if err != nil {
			return nil, err
		}
		// The live-slot file is the focus: unqualified keys bind to its
		// namespace before any cross-namespace ambiguity check (§3.2).
		st.focusNS = ns
	}

	// ── override layers ──────────────────────────────────────────────────────

	sysOvr, err := schema.LoadOverrides(r.P.SystemOverride)
	if err != nil {
		return nil, fmt.Errorf("resolve: loading system overrides: %w", err)
	}
	st.sysOvr = sysOvr

	usrOvr, err := schema.LoadOverrides(r.P.UserOverride)
	if err != nil {
		return nil, fmt.Errorf("resolve: loading user overrides: %w", err)
	}
	st.usrOvr = usrOvr

	return st, nil
}

// resolveLivePath determines the live repo file path based on Resolver.Source.
// resolveLivePath returns the definition file for the "live" slot plus the
// layer it truthfully belongs to: repo-live for CWD discovery and local
// paths, user-snapshot when a registered name resolves via its snapshot.
func (r *Resolver) resolveLivePath() (string, Layer, error) {
	src := r.Source

	// "" => CWD discovery.
	if src == "" {
		repoRoot, ok := gitx.RepoRoot(r.P.WorkDir)
		if !ok {
			return "", LayerNone, nil // not in a git repo; live layer absent
		}
		return gitx.SourcePath(r.R, repoRoot), LayerRepoLive, nil
	}

	// Local path: starts with "/", "./", "../", or stat says it exists as a dir.
	// A path that is not (inside) a git repository is an unknown source (plan
	// §7.2 IA-10: exit 2), never silently an empty world.
	if isLocalPath(src) {
		repoRoot, ok := gitx.RepoRoot(src)
		if !ok {
			return "", LayerNone, fmt.Errorf("resolve: source path %q is not a git repository: %w", src, ErrUnknownSource)
		}
		return gitx.SourcePath(r.R, repoRoot), LayerRepoLive, nil
	}

	// Registered name.
	if r.S == nil {
		return "", LayerNone, fmt.Errorf("resolve: source %q: %w", src, ErrUnknownSource)
	}
	snapPath, ok := r.S.Snapshot(src)
	if !ok {
		return "", LayerNone, fmt.Errorf("resolve: source %q: %w", src, ErrUnknownSource)
	}
	// The registered path IS a snapshot file — attribute it truthfully.
	return snapPath, LayerUserSnapshot, nil
}

// isLocalPath returns true when src should be treated as a filesystem path
// rather than a registered source name.
func isLocalPath(src string) bool {
	if strings.HasPrefix(src, "/") ||
		strings.HasPrefix(src, "./") ||
		strings.HasPrefix(src, "../") {
		return true
	}
	// os.Stat check: if the path exists (e.g. a relative dir without ./ prefix).
	if _, err := os.Stat(src); err == nil {
		return true
	}
	return false
}

// loadSnapshotDir loads all *.yaml and *.json files from dir into st.
// If the directory does not exist, it is silently skipped.
func (r *Resolver) loadSnapshotDir(dir string, layer Layer, st *resolvedState) error {
	if _, err := os.Stat(dir); err != nil {
		return nil // absent dir is ok
	}

	var files []string
	for _, pat := range []string{"*.yaml", "*.yml", "*.json"} {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			return fmt.Errorf("resolve: glob %s: %w", dir, err)
		}
		files = append(files, matches...)
	}
	sort.Strings(files) // deterministic order

	for _, f := range files {
		if _, err := r.loadFile(f, layer, st); err != nil {
			return err
		}
	}
	return nil
}

// loadFile loads a single feature file into st, returning its namespace.
func (r *Resolver) loadFile(path string, layer Layer, st *resolvedState) (string, error) {
	ff, err := schema.LoadFeatureFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil // absent live file is ok
		}
		return "", fmt.Errorf("resolve: loading %s: %w", path, err)
	}

	ns := ff.GetNamespace()
	for _, set := range ff.GetSets() {
		for _, feat := range set.GetFeatures() {
			k := defKey{namespace: ns, path: feat.GetPath()}
			if st.defHistory[k] == nil {
				st.defHistory[k] = make(map[Layer]definition)
			}
			st.defHistory[k][layer] = definition{feature: feat, layer: layer, namespace: ns}
			idx, exists := st.byKey[k]
			if exists {
				// Replace: update layer and feature.
				st.defs[idx] = definition{feature: feat, layer: layer, namespace: ns}
			} else {
				st.byKey[k] = len(st.defs)
				st.defs = append(st.defs, definition{feature: feat, layer: layer, namespace: ns})
			}
		}
	}
	return ns, nil
}

// defaultValue computes the default value carried by a feature definition.
func defaultValue(feat *gffv1.Feature) *gffv1.Value {
	switch feat.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: feat.GetBoolDefault()}}
	case *gffv1.Feature_ChoiceDefault:
		cd := feat.GetChoiceDefault()
		var ids []string
		for _, opt := range cd.GetOptions() {
			if opt.GetSelected() {
				ids = append(ids, opt.GetId())
			}
		}
		return &gffv1.Value{Kind: &gffv1.Value_ChoiceValue{
			ChoiceValue: &gffv1.ChoiceSelection{Selected: ids},
		}}
	default:
		// Unknown default type — bool false fallback.
		return &gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}}
	}
}

// effectiveValue computes the effective value and layer for a definition,
// taking overrides into account.
func effectiveValue(def definition, sysOvr, usrOvr map[string]*gffv1.Value) (Resolved, error) {
	feat := def.feature
	defLayer := def.layer

	defaultVal := defaultValue(feat)
	effectiveVal := defaultVal
	effectiveLayer := defLayer

	// Apply system override if present.
	if ovrVal, ok := sysOvr[feat.GetPath()]; ok {
		validated, err := validateOverride(feat, ovrVal)
		if err != nil {
			return Resolved{}, err
		}
		effectiveVal = validated
		effectiveLayer = LayerSystemOverride
	}

	// Apply user override if present (wins over system override).
	if ovrVal, ok := usrOvr[feat.GetPath()]; ok {
		validated, err := validateOverride(feat, ovrVal)
		if err != nil {
			return Resolved{}, err
		}
		effectiveVal = validated
		effectiveLayer = LayerUserOverride
	}

	return Resolved{
		Feature:   feat,
		Value:     effectiveVal,
		Layer:     effectiveLayer,
		namespace: def.namespace,
	}, nil
}

// validateOverride ensures the override value is type-compatible with the feature
// and, for choice features, that all referenced option ids exist and that
// CHOICE_MODE_SINGLE features have at most one selected id.
func validateOverride(feat *gffv1.Feature, ovr *gffv1.Value) (*gffv1.Value, error) {
	key := feat.GetPath()

	switch feat.Default.(type) {
	case *gffv1.Feature_BoolDefault:
		// Bool flag: override must be a bool value.
		if _, isBool := ovr.Kind.(*gffv1.Value_BoolValue); !isBool {
			return nil, fmt.Errorf("resolve: key %q: %w: expected bool value", key, ErrWrongFlagType)
		}
		return ovr, nil

	case *gffv1.Feature_ChoiceDefault:
		// Choice flag: override must be a choice value.
		choiceVal, isChoice := ovr.Kind.(*gffv1.Value_ChoiceValue)
		if !isChoice {
			return nil, fmt.Errorf("resolve: key %q: %w: expected choice value", key, ErrWrongFlagType)
		}

		cd := feat.GetChoiceDefault()

		// Build valid option id set.
		validIDs := make(map[string]bool, len(cd.GetOptions()))
		for _, opt := range cd.GetOptions() {
			validIDs[opt.GetId()] = true
		}

		selected := choiceVal.ChoiceValue.GetSelected()

		// Validate each selected id exists.
		for _, id := range selected {
			if !validIDs[id] {
				return nil, fmt.Errorf("resolve: key %q: %w: option id %q not defined", key, ErrUnknownOption, id)
			}
		}

		// CHOICE_MODE_SINGLE: at most one id.
		if cd.GetMode() == gffv1.ChoiceMode_CHOICE_MODE_SINGLE && len(selected) > 1 {
			return nil, fmt.Errorf("resolve: key %q: %w: CHOICE_MODE_SINGLE allows at most one id, got %v", key, ErrUnknownOption, selected)
		}

		return ovr, nil

	default:
		return ovr, nil
	}
}

// All returns all resolved features across all definition layers, sorted by Feature.Path.
// Unknown-key override entries are ignored.
func (r *Resolver) All() ([]Resolved, error) {
	st, err := r.load()
	if err != nil {
		return nil, err
	}

	results := make([]Resolved, 0, len(st.defs))
	for _, def := range st.defs {
		res, err := effectiveValue(def, st.sysOvr, st.usrOvr)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}

	// Stable, fully-deterministic order: path, then namespace. The tie-break
	// matters when two sources define the same path — without it the sort is
	// unstable and the UI's row/group order flips between runs.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Feature.GetPath() != results[j].Feature.GetPath() {
			return results[i].Feature.GetPath() < results[j].Feature.GetPath()
		}
		return results[i].namespace < results[j].namespace
	})

	return results, nil
}

// Resolve returns the resolved value for a single key.
//
// Key forms:
//   - "namespace:key" — fully qualified (split on first colon)
//   - "key" — unqualified: resolves if exactly one namespace defines it; ambiguous => ErrUnknownKey
func (r *Resolver) Resolve(key string) (Resolved, error) {
	st, err := r.load()
	if err != nil {
		return Resolved{}, err
	}
	def, err := bindKey(st, key)
	if err != nil {
		return Resolved{}, err
	}
	return effectiveValue(def, st.sysOvr, st.usrOvr)
}

// bindKey resolves a user-supplied key to its winning definition per the §3.2
// binding rules (qualified form, focus-namespace-first, ambiguity => error).
// Shared by Resolve and Explain.
func bindKey(st *resolvedState, key string) (definition, error) {
	// Split on first colon.
	var ns, path string
	if idx := strings.IndexByte(key, ':'); idx >= 0 {
		ns = key[:idx]
		path = key[idx+1:]
	} else {
		path = key
	}

	if ns != "" {
		// Fully qualified.
		k := defKey{namespace: ns, path: path}
		idx, ok := st.byKey[k]
		if !ok {
			return definition{}, fmt.Errorf("resolve: %w: %s", ErrUnknownKey, key)
		}
		return st.defs[idx], nil
	}

	// Unqualified: the focus namespace (CWD repo / --source target) wins
	// outright when it defines the key (§3.2).
	if st.focusNS != "" {
		if idx, ok := st.byKey[defKey{namespace: st.focusNS, path: path}]; ok {
			return st.defs[idx], nil
		}
	}

	// Otherwise collect all definitions for this path across all namespaces.
	var candidates []definition
	for k, idx := range st.byKey {
		if k.path == path {
			candidates = append(candidates, st.defs[idx])
		}
	}

	switch len(candidates) {
	case 0:
		return definition{}, fmt.Errorf("resolve: %w: %s", ErrUnknownKey, key)
	case 1:
		return candidates[0], nil
	default:
		// Ambiguous: collect namespace names for a helpful error message.
		nsList := make([]string, 0, len(candidates))
		for _, c := range candidates {
			nsList = append(nsList, c.namespace)
		}
		sort.Strings(nsList)
		return definition{}, fmt.Errorf("resolve: %w: %q is defined in multiple namespaces (%s); qualify as <namespace>:%s",
			ErrUnknownKey, path, strings.Join(nsList, ", "), path)
	}
}
