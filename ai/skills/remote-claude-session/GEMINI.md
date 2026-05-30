# ai/skills/remote-claude-session/

Skill for starting a persistent `claude --remote-control` session on any remote
SSH host inside a specified git repository, via a named tmux session that survives
disconnects.

See `SKILL.md` for the full step-by-step workflow (validate alias → detect Claude
→ check existing session → start → verify → report attach command).

**Planned integration:** `tmux-mgr remote start <alias>` — see
`docs/designs/tmux-mgr-remote-command.md` for the design.
