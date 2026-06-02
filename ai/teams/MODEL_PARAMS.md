# Model Parameter Convention

Each agent file declares model selection for **four AI coding tools** and **Ollama** (local).
The header block looks like this:

```
# Model:
#   claude:      <claude-code model slug>       # effort hint: auto|think|think-hard
#   gemini:      <gemini-cli model slug>        # think_budget: 0 (fast) | 1024 | 8192 (deep)
#   antigravity: <anti-gravity cli model slug>  # effort: low | medium | high | max
#   ollama:      <ollama model tag>             # (local fallback, always present)
```

## Effort / Thinking Tier Guide

| Tier | Claude Code | Gemini CLI | Anti-Gravity | Ollama |
|------|-------------|------------|--------------|--------|
| **Fast** — automation, CI, rote tasks | `claude-haiku-4-5` (auto) | `gemini-2.0-flash` (think_budget: 0) | `gpt-4.1-mini` (effort: low) | `qwen2.5-coder:1.5b` |
| **Standard** — day-to-day coding | `claude-sonnet-4-5` (auto) | `gemini-2.5-flash` (think_budget: 0) | `gpt-4.1` (effort: medium) | `qwen2.5-coder:7b` |
| **Think** — complex problems, code review | `claude-sonnet-4-5` (think) | `gemini-2.5-flash` (think_budget: 1024) | `gpt-4.1` (effort: high) | `qwen2.5:7b` |
| **Deep Think** — architecture, high-stakes | `claude-opus-4-5` (think-hard) | `gemini-2.5-pro` (think_budget: 8192) | `o3` (effort: max) | `gemma3:27b` |

## Context-Strategy Values
- `aggressive` — minimal context, fastest throughput (monitoring, trivial rewrites)
- `standard`   — balanced (most coding tasks)
- `deep`        — full context, multi-file reasoning (architecture, design reviews)
