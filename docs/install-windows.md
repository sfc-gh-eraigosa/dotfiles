# `install.sh` on Windows/WSL — interactivity & customization flow

`install.sh` is interactive — never run it non-interactively or backgrounded. This doc explains
the Windows/WSL-specific flow; the one-line rule lives in the root `AGENTS.md`.

## Prompt timing is front-loaded; execution is back-loaded

On Windows/WSL the interactivity is **front-loaded** — sudo and the
"Windows Desktop Customization detected … [y/n/s]" prompt happen in the first minute — but the
Windows **execution** (the `opt/Desktop/*` deploy + PowerShell setup) runs at the **END** of the
script, after the gff bootstrap has exported `GFF_*`, so `install.windows.*` overrides apply with
no manual export (`gff set install.windows.wispr-flow false && ./install.sh` just works).

## What each answer means

- **`[y]`** — the **UAC prompt appears at the end of the run** — stay nearby, or the elevated
  batch reports itself skipped (loud warning + rerun hint; its log is
  `%USERPROFILE%\setup-elevated.log`).
- **`[s]`** — records the skip as a gff override (`install.windows.desktop-deploy=false`; undo
  with `gff unset install.windows.desktop-deploy`) — the legacy sentinel file is migrated
  automatically.
- **No TTY** (backgrounded, or piped through `tail`/redirection) — the ask phase prints guidance
  instead of prompting, and the run performs the file deploy but no customization.

## Deploy vs. customization

The `opt/Desktop/*` file deploy runs per-run regardless of the y/n answer — only the PowerShell
customization is answer-gated.
