// Package registry manages the gff source registry (sources.yaml) and user snapshots.
//
// The registry is keyed by reverse-DNS namespace (e.g. "com.github.example.repo").
// For each registered source it stores the URL and the most-recently-installed commit.
// Snapshots are verbatim byte copies of the source's features file stored at
// <UserSnapshotDir>/<namespace>.yaml; they are the "user-snapshot" layer for the
// 5-layer resolver.
//
// Registry also implements the resolve.SourceLookup seam (Snapshot method) so
// the resolver can look up registered sources without importing this package.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/gitx"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/paths"
	"google.golang.org/protobuf/encoding/protojson"
	"gopkg.in/yaml.v3"
)

// ErrNamespaceTaken is returned by Install when the namespace is already
// registered by a different URL. The error wraps the existing URL so callers
// can use errors.Is and read the message for human display.
var ErrNamespaceTaken = errors.New("namespace registered by a different url")

// Registry manages the gff source registry and user snapshots.
// P must point to the caller's well-known paths (RegistryFile, UserSnapshotDir).
type Registry struct {
	P paths.Paths
}

// Install registers a source repo under namespace, refreshing the entry if
// the same URL is already registered (commit and snapshot are updated), and
// writing a verbatim byte-copy of the repo's features file to the user
// snapshot dir.
//
// If namespace is already registered by a DIFFERENT url, ErrNamespaceTaken is
// returned and the registry file is not modified.
//
// repoRoot is the repo's root directory; the source file is discovered via
// gitx.SourcePath (probes .gff/features.yaml then .github/gff/features.yaml).
// ff is the parsed FeatureFile — provided so callers can pre-validate it;
// Install still copies the raw bytes from disk for the snapshot.
func (g *Registry) Install(repoRoot, namespace, url, commit string, ff *gffv1.FeatureFile) error {
	_ = ff // caller provides for pre-validation; raw bytes come from disk

	// Read existing registry (missing = empty, ok).
	reg, err := g.loadRegistry()
	if err != nil {
		return fmt.Errorf("registry.Install: load: %w", err)
	}

	// Find the existing entry for this namespace, if any.
	var existing *gffv1.Source
	for _, s := range reg.Sources {
		if s.Namespace == namespace {
			existing = s
			break
		}
	}

	if existing != nil && existing.Url != url {
		// Different URL: conflict.
		return fmt.Errorf("%w: %s (existing url: %s)", ErrNamespaceTaken, namespace, existing.Url)
	}

	// Locate and read the source features file (raw bytes for snapshot).
	srcPath := gitx.SourcePath(gitx.ExecRunner{}, repoRoot)
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("registry.Install: read source file %s: %w", srcPath, err)
	}

	// Write the snapshot (verbatim bytes) atomically.
	snapshotPath := filepath.Join(g.P.UserSnapshotDir, namespace+".yaml")
	if err := atomicWrite(snapshotPath, raw, 0o644); err != nil {
		return fmt.Errorf("registry.Install: write snapshot: %w", err)
	}

	// Update or append the registry entry.
	if existing != nil {
		existing.Commit = commit
	} else {
		reg.Sources = append(reg.Sources, &gffv1.Source{
			Namespace: namespace,
			Url:       url,
			Commit:    commit,
		})
	}

	// Marshal and write the registry atomically.
	regBytes, err := marshalSourceRegistry(reg)
	if err != nil {
		return fmt.Errorf("registry.Install: marshal: %w", err)
	}
	if err := atomicWrite(g.P.RegistryFile, regBytes, 0o644); err != nil {
		return fmt.Errorf("registry.Install: write registry: %w", err)
	}
	return nil
}

// Sources returns all registered sources. If the registry file does not exist,
// an empty slice and nil error are returned.
func (g *Registry) Sources() ([]*gffv1.Source, error) {
	reg, err := g.loadRegistry()
	if err != nil {
		return nil, fmt.Errorf("registry.Sources: %w", err)
	}
	return reg.Sources, nil
}

// Snapshot returns the path to the user-snapshot YAML for namespace and true
// when the namespace is registered. Returns ("", false) for unknown namespaces.
//
// Snapshot implements the resolve.SourceLookup seam without importing internal/resolve.
func (g *Registry) Snapshot(namespace string) (string, bool) {
	srcs, err := g.Sources()
	if err != nil {
		return "", false
	}
	for _, s := range srcs {
		if s.Namespace == namespace {
			return filepath.Join(g.P.UserSnapshotDir, namespace+".yaml"), true
		}
	}
	return "", false
}

// loadRegistry reads and parses sources.yaml. Missing file returns an empty
// SourceRegistry with nil error.
func (g *Registry) loadRegistry() (*gffv1.SourceRegistry, error) {
	data, err := os.ReadFile(g.P.RegistryFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &gffv1.SourceRegistry{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", g.P.RegistryFile, err)
	}

	// YAML → generic map → JSON → proto (same trick as schema.LoadFeatureFile).
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("yaml parse %s: %w", g.P.RegistryFile, err)
	}
	normalized := normalizeMapKeys(raw)
	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	reg := &gffv1.SourceRegistry{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(jsonBytes, reg); err != nil {
		return nil, fmt.Errorf("proto unmarshal: %w", err)
	}
	return reg, nil
}

// marshalSourceRegistry serializes a SourceRegistry as YAML (proto → protojson
// → generic map → YAML, the same round-trip as schema uses for FeatureFile).
func marshalSourceRegistry(reg *gffv1.SourceRegistry) ([]byte, error) {
	// proto → JSON bytes.
	jsonBytes, err := protojson.Marshal(reg)
	if err != nil {
		return nil, fmt.Errorf("protojson marshal: %w", err)
	}
	// JSON bytes → generic map (normalised intermediate).
	var m any
	if err := json.Unmarshal(jsonBytes, &m); err != nil {
		return nil, fmt.Errorf("json unmarshal for yaml re-encode: %w", err)
	}
	// generic map → YAML bytes.
	out, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("yaml marshal: %w", err)
	}
	return out, nil
}

// atomicWrite writes data to path using a temp file + rename so concurrent
// readers never see a partial write. The temp file is created in the same
// directory as path to ensure rename stays on the same filesystem.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Create temp in the same dir so os.Rename is atomic on POSIX.
	tmp, err := os.CreateTemp(dir, ".gff-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	// Clean up the temp file on any error path.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	success = true
	return nil
}

// normalizeMapKeys recursively converts map[any]any to map[string]any so that
// json.Marshal succeeds on the output of yaml.Unmarshal.
func normalizeMapKeys(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeMapKeys(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprintf("%v", k)] = normalizeMapKeys(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeMapKeys(val)
		}
		return out
	default:
		return v
	}
}
