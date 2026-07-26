package resolve

// Explain — the per-layer provenance story behind one key. Additive §3.3
// extension (owner-approved on the PR #187 review): the TUI detail view
// renders this breakdown; the frozen exported surface is unchanged.

import (
	gffv1 "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/gen/gff/v1"
)

// LayerInfo describes one resolution layer's contribution to a key.
type LayerInfo struct {
	Layer   Layer
	Present bool         // the layer defines (definition layers) or overrides (override layers) the key
	Value   *gffv1.Value // contributed value: that layer's default, or the override value
	Winner  bool         // this layer set the effective value
}

// Explain returns the winning resolution for key plus one LayerInfo per
// resolution layer, in §3.3 layer order. Key binding follows the same §3.2
// rules as Resolve (qualified, focus-namespace-first, ambiguity => error).
func (r *Resolver) Explain(key string) (Resolved, []LayerInfo, error) {
	st, err := r.load()
	if err != nil {
		return Resolved{}, nil, err
	}

	def, err := bindKey(st, key)
	if err != nil {
		return Resolved{}, nil, err
	}

	res, err := effectiveValue(def, st.sysOvr, st.usrOvr)
	if err != nil {
		return Resolved{}, nil, err
	}

	k := defKey{namespace: def.namespace, path: def.feature.GetPath()}
	infos := make([]LayerInfo, 0, 5)

	for _, l := range []Layer{LayerSystemSnapshot, LayerUserSnapshot, LayerRepoLive} {
		li := LayerInfo{Layer: l}
		if d, ok := st.defHistory[k][l]; ok {
			li.Present = true
			li.Value = defaultValue(d.feature)
		}
		infos = append(infos, li)
	}

	for _, ov := range []struct {
		l Layer
		m map[string]*gffv1.Value
	}{
		{LayerSystemOverride, st.sysOvr},
		{LayerUserOverride, st.usrOvr},
	} {
		li := LayerInfo{Layer: ov.l}
		if v, ok := ov.m[def.feature.GetPath()]; ok {
			li.Present = true
			li.Value = v
		}
		infos = append(infos, li)
	}

	for i := range infos {
		infos[i].Winner = infos[i].Layer == res.Layer
	}
	return res, infos, nil
}
