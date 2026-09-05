# T6 human click check — 2026-09-05

Observer: repo owner (via Claude), in a herdr pane inside gnome-terminal (VTE 0.76), Claude Code status line rendered by `~/opt/bin/gsl` built from this branch (T5 state).

Gesture: Ctrl+click on the underlined field in the status bar.

| Field | Expected target | Result |
| :-- | :-- | :-- |
| `dirgit` directory name | `file://<worktree>` (file manager) | opened correctly |
| `dirgit` branch / `repo` label | `https://github.com/sfc-gh-eraigosa/dotfiles/tree/worktree/gsl` | opened correctly |
| `repo` glyph | `https://github.com/sfc-gh-eraigosa/dotfiles` | opened correctly |
| `ai` model name / context % | `https://claude.ai/settings/usage` | opened correctly |
| `time` | `https://time.is/Los_Angeles` | opened correctly |
| `repo` PR badge (agy_defaults worktree) | `https://github.com/sfc-gh-eraigosa/dotfiles/pull/269` | opened correctly |

User's answer to the check prompt: "All targets open correctly".
Chain proven end to end: gsl → Claude Code status line → herdr (ghostty-vt) → gnome-terminal.
