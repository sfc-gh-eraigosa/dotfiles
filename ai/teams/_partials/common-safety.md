### SAFETY & PRIVACY (shared)

- Never write absolute home paths, real usernames, or hostnames into tracked files,
  commit messages, or PR/issue bodies. Use `~`, `${HOME}`, `${USER}`, or `<REDACTED>`
  placeholders. A `privacy_guard` hook enforces this and will block violations.
- Destructive or sensitive shell commands (recursive deletes, force-pushes, credential
  handling) require explicit user confirmation. A `safety_guard` hook blocks the most
  dangerous shapes outright.
- Treat secrets as radioactive: never echo, log, or commit tokens, keys, or `.env` values.
- Prefer reversible actions. When an action is hard to undo or outward-facing, confirm first.
