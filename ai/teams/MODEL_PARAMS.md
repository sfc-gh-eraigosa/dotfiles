# Model Parameter Convention

Agent personas (`<team>/the_*.md`) do **not** name models. Each declares a single abstract
**`tier:`** in its YAML frontmatter:

```yaml
tier: deep-think   # fast | standard | think | deep-think
```

`opt/scripts/system/install_ai_teams.sh` resolves that tier through
[`model-map.yaml`](./model-map.yaml) — the **single source of truth** for which concrete
model + effort/temperature each tool uses. Update a slug once in `model-map.yaml` and every
persona on that tier picks it up on the next `sync-teams`. This is why the personas survive
model churn: there are no per-file model slugs to go stale.

## Tiers (resolved per tool in `model-map.yaml`)

| Tier | When | Claude | Gemini | Antigravity | Ollama |
|------|------|--------|--------|-------------|--------|
| **fast** | automation, CI, rote | `model` + `effort` | `model` + `temperature` | `model` + `effort` | `FROM` + `num_ctx` |
| **standard** | day-to-day coding | ⤷ | ⤷ | ⤷ | ⤷ |
| **think** | complex problems, review | ⤷ | ⤷ | ⤷ | ⤷ |
| **deep-think** | architecture, high-stakes | ⤷ | ⤷ | ⤷ | ⤷ |

The concrete values live in `model-map.yaml` (e.g. `deep-think.claude = {model: opus,
effort: high}`). `validate.sh` asserts every persona's `tier` exists there.

## Other resolved frontmatter fields

| Field | Claude | Gemini | Antigravity | Ollama |
|-------|--------|--------|-------------|--------|
| model | `model:` | `model:` | `model:` | `FROM` |
| effort/sampling | `effort:` | `temperature:` | `effort:` | — |
| context | (n/a) | (n/a) | (n/a) | `PARAMETER num_ctx` |

## Context-Strategy Values (informational hint in `context_strategy:`)
- `aggressive` — minimal context, fastest throughput (monitoring, trivial rewrites)
- `standard`   — balanced (most coding tasks)
- `deep`       — full context, multi-file reasoning (architecture, design reviews)
