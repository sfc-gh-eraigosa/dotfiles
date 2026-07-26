package resolve_test

// Explain: the per-layer provenance story behind one key — which of the five
// layers define or override it, what each contributes, and which one won.
// Additive extension to §3.3 (owner-approved on PR #187 review: the TUI
// detail view renders this breakdown).

import (
	"errors"
	"testing"

	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/internal/resolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const explainDefYAML = `
namespace: com.example.a
sets:
  - area: demo
    features:
      - {path: demo.ui.dash, description: Dashboard, boolDefault: true}
`

func TestExplainAllFiveLayers(t *testing.T) {
	r := newResolver(t, world{
		sysSnap:  explainDefYAML,
		userSnap: explainDefYAML,
		repo:     explainDefYAML,
		sysOvr:   "demo.ui.dash: false\n",
		usrOvr:   "demo.ui.dash: true\n",
	})

	res, layers, err := r.Explain("demo.ui.dash")
	require.NoError(t, err)
	require.Len(t, layers, 5, "one row per resolution layer, in §3.3 order")

	// Rows come in layer order.
	wantLayers := []resolve.Layer{
		resolve.LayerSystemSnapshot, resolve.LayerUserSnapshot,
		resolve.LayerRepoLive, resolve.LayerSystemOverride, resolve.LayerUserOverride,
	}
	for i, li := range layers {
		assert.Equal(t, wantLayers[i], li.Layer, "row %d layer", i)
		assert.True(t, li.Present, "row %d: every layer contributes in this world", i)
	}

	// Definition layers contribute the default; override layers their value.
	assert.True(t, layers[0].Value.GetBoolValue(), "sys-snap default true")
	assert.False(t, layers[3].Value.GetBoolValue(), "sys override false")
	assert.True(t, layers[4].Value.GetBoolValue(), "user override true")

	// Winner = user override, and it matches the effective resolution.
	assert.True(t, layers[4].Winner)
	assert.False(t, layers[3].Winner)
	assert.Equal(t, resolve.LayerUserOverride, res.Layer)
	assert.True(t, res.Value.GetBoolValue())
}

func TestExplainSparseWorld(t *testing.T) {
	r := newResolver(t, world{repo: explainDefYAML})

	res, layers, err := r.Explain("demo.ui.dash")
	require.NoError(t, err)
	require.Len(t, layers, 5)

	present := map[resolve.Layer]bool{}
	for _, li := range layers {
		present[li.Layer] = li.Present
		if li.Layer == resolve.LayerRepoLive {
			assert.True(t, li.Winner, "repo-live wins with no overrides")
			assert.True(t, li.Value.GetBoolValue())
		} else {
			assert.False(t, li.Winner)
			assert.False(t, li.Present, "layer %s must be absent", li.Layer)
		}
	}
	assert.True(t, present[resolve.LayerRepoLive])
	assert.Equal(t, resolve.LayerRepoLive, res.Layer)
}

func TestExplainUnknownKey(t *testing.T) {
	r := newResolver(t, world{repo: explainDefYAML})
	_, _, err := r.Explain("demo.ui.nope")
	require.Error(t, err)
	assert.True(t, errors.Is(err, resolve.ErrUnknownKey))
}

func TestExplainQualifiedKey(t *testing.T) {
	r := newResolver(t, world{repo: explainDefYAML, usrOvr: "demo.ui.dash: false\n"})
	res, layers, err := r.Explain("com.example.a:demo.ui.dash")
	require.NoError(t, err)
	require.Len(t, layers, 5)
	assert.Equal(t, resolve.LayerUserOverride, res.Layer)
	assert.False(t, res.Value.GetBoolValue())
}

func TestNamespaceAccessorAndWithValue(t *testing.T) {
	r := newResolver(t, world{repo: explainDefYAML})
	res, err := r.Resolve("demo.ui.dash")
	require.NoError(t, err)
	require.Equal(t, "com.example.a", res.Namespace(), "accessor exposes the owning namespace")

	// WithValue must carry the namespace through — a bare literal drops it
	// (the exact bug that made TUI rows vanish from namespace-scoped pages).
	updated := res.WithValue(
		&gffv1.Value{Kind: &gffv1.Value_BoolValue{BoolValue: false}},
		resolve.LayerUserOverride,
	)
	assert.Equal(t, "com.example.a", updated.Namespace(), "namespace preserved")
	assert.False(t, updated.Value.GetBoolValue())
	assert.Equal(t, resolve.LayerUserOverride, updated.Layer)
	assert.Same(t, res.Feature, updated.Feature, "definition pointer unchanged")
}
