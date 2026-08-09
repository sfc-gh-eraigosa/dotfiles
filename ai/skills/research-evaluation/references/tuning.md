# Tuning the rubric (gff weights)

The rubric's per-dimension weights are **[gff](../../../sdk/gff/AGENTS.md) feature flags**,
not hard-coded in the skill — so weightings are configurable on demand and *others can
dissent without editing anything the skill ships*. One size does not fit all; your lived
experience and priorities are meant to override these defaults.

## The flags

Nine weight flags + a decision mode + two requirement toggles, under `research-rubric.*`
(defined in `.github/gff/features.yaml`; env-export form `RESEARCH_RUBRIC_*`):

| Flag | Dimension | Public default |
| :-- | :-- | :-- |
| `research-rubric.weight.value` | (a) Value | high |
| `research-rubric.weight.setup-licensing` | (b) Setup & licensing | medium |
| `research-rubric.weight.adversarial` | (c) Adversarial | high |
| `research-rubric.weight.security` | (d) Security & safety | critical |
| `research-rubric.weight.stability` | (e) Stability | medium |
| `research-rubric.weight.quality-support` | (f) Quality & support | medium |
| `research-rubric.weight.demo` | (g) Demo | low |
| `research-rubric.weight.borrowable` | (h) Borrowable (build-vs-adopt) | high |
| `research-rubric.weight.business` | (i) Business outcomes | medium |
| `research-rubric.decision.mode` | verdict posture | balanced |
| `research-rubric.require.adversarial` | gate: case-against mandatory | true |
| `research-rubric.require.docker-demo` | gate: docker-or-skip demos | true |

Weight tiers are `none · low · medium · high · critical` → **0 · 1 · 2 · 3 · 4**. The
skill multiplies each dimension's assessment by its tier value, so `none` drops a
dimension entirely and `critical` makes it decisive.

## Reading them (what the skill does)

```bash
gff get research-rubric.weight.security          # -> critical
gff list --source com.github.sfc-gh-eraigosa.dotfiles   # all flags + winning layer
```

The skill reads the effective tiers at evaluation time and states the active weighting in
its output, so a reader can see which lens produced the verdict.

## Tuning on demand (the override layer)

Overrides live in the gff **user layer** (`~/.config/gff/config.yaml`) — they win over the
repo defaults and travel with you, no fork required:

```bash
gff set research-rubric.weight.business high      # I care more about ROI than the default
gff set research-rubric.weight.demo none          # I don't gate on demos
gff set research-rubric.decision.mode strict      # raise the adoption bar
gff tui                                            # browse/toggle interactively w/ provenance
gff unset research-rubric.weight.business          # restore the default
```

Share a weighting by committing a repo-layer override in your own project, or hand someone
your `~/.config/gff/config.yaml` snippet. Dissenting rubrics are a feature: capture *why*
alongside the override so the next person understands the lens, not just the numbers.
