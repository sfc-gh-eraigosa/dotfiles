# 🧰 The SDK — small Go tools that make an agent-driven workflow safe

Seven Go modules that turn a laptop and a pile of machines into an environment
where **AI agents can do real work without you holding your breath.**

Each one is a single static binary, independently versioned, `go install`-able,
and useful on its own. Together they cover the loop: *let an agent work
(`tmux-mgr`) → let it commit without losing anything (`gss`) → see what's
happening (`gsl`) → roll it out to every machine you own (`fleet`).*

```bash
# Everything, wired into ~/opt/bin:
~/git/dotfiles/install.sh

# Or just the one you want:
go install github.com/sfc-gh-eraigosa/dotfiles/sdk/gss@latest
```

---

## Pick your tool

| Tool | Reach for it when… | One-liner |
| :--- | :--- | :--- |
| **[gss](#-gss--never-lose-work-to-git-again)** | An agent is about to touch git | Safe sync: automatic backups, approval gates, stacked feature worktrees |
| **[tmux-mgr](#-tmux-mgr--run-five-agents-at-once)** | One agent isn't fast enough | Parallel agents in isolated worktrees + tmux panes |
| **[gsl](#-gsl--know-what-your-agent-is-costing-you)** | You can't tell how much context is left | Powerline status line for Claude Code / Antigravity |
| **[fleet](#-fleet--is-every-machine-actually-updated)** | You have more machines than you can `ssh` into | Multi-host install-drift dashboard + wake ladder |
| **[gff](#-gff--flags-that-live-in-git)** | "Skip that step on this machine" | Layered feature flags with full provenance |
| **[wol](#-wol--turn-it-on-from-anywhere)** | The machine you need is powered off | Wake-on-LAN magic packets |
| **[libs](#-libs--the-shared-foundation)** | You're writing tool #8 | Shared logging: rotation, capture, XDG paths |

---

## 🛡️ `gss` — never lose work to git again

> **Git Safe Sync.** Safe pushes, automatic backups, and approval gates that
> autonomous agents cannot talk their way past.

**The problem.** Give an AI assistant push access and you have invented a new
way to lose a day. A rebase explodes mid-flight. A `git add -A` sweeps in an
unrelated half-finished file. An agent decides on its own that now is a good
time to force-push. Recovery means spelunking through the reflog, assuming you
notice at all.

**What it does about it.** Every push first writes a timestamped
`backup/gss-*` branch, so the "before" state is always one checkout away. Pushes
are gated on an approval token that a human has to mint — an agent can prepare a
push, but it cannot complete one. And when you want to parallelize a feature,
`gss feature` hands each worker its own worktree, branch, and stacked draft PR.

**Reach for it when:**

- An agent is about to commit or push on your behalf
- You want three workers on one feature without them stepping on each other
- You need to know which of your repos are dirty, without touching them
- A rebase just went wrong and you want the pre-rebase state back

```console
$ gss status
No changes detected in .

$ gss backup
Creating safety backup branch in .: backup/gss-20260823-120545
Backup branch created successfully.

$ gss feature start my-api --base main --description "REST API refactor"
Started feature "my-api" (base main)

$ gss feature worker add --feature my-api --purpose api --description "Core endpoints"
Added worker api to feature my-api
  Branch: feature/my-api/api   Base: main
```

**Gotchas.** `gss push` needs a fresh `~/.config/gss/approval.token` matched
against current `HEAD` — that gate is the point, not an obstacle to route
around. Inside a feature worktree, plain `gss push`/`pr`/`sync` are rejected on
purpose; use `gss feature checkpoint` instead. PR operations shell out to `gh`,
so it must be authenticated.

→ [Full docs](./gss/README.md) · [Agent context](./gss/AGENTS.md)

---

## 🪟 `tmux-mgr` — run five agents at once

> Spawn isolated AI agents into their own git worktrees and tmux panes, then
> collect their results.

**The problem.** Delegating to one assistant blocks your terminal. Delegating to
three corrupts your working tree, because all three are editing the same files.
And your carefully arranged four-pane layout evaporates the moment the terminal
dies.

**What it does about it.** `tmux-mgr agent start` provisions a fresh git
worktree *and* a tmux pane per agent, so parallel work is genuinely isolated —
no shared filesystem, no collisions. It detects whether you're in Claude Code or
Antigravity and launches the matching assistant. Layouts save and restore by
name. And any pane's contents can be captured as text, which is how you show an
agent what another process actually printed.

**Reach for it when:**

- Two independent chunks of work could proceed in parallel
- You want your daily multi-pane layout back with one command
- An agent needs to *see* output from a long-running process in another pane
- You're fanning out a migration across many files

```console
$ tmux-mgr agent list
No active agent sessions found.

$ tmux-mgr agent start refactor-auth \
    --task-description "Improve error handling in the auth module"

$ tmux-mgr agent list
SESSION ID          AGENT NAME     STATUS    START TIME
refactor-auth-...   refactor-auth  RUNNING   23 Aug 2026 14:30 UTC

$ tmux-mgr agent complete refactor-auth-...   # reads the worker's RESULT.md
```

**Gotchas.** tmux has to already be running. Anchor the orchestration root once
(`tmux-mgr pane anchor`) so splits target the right pane. Spawned agents are
expected to write their findings to `RESULT.md` — that file *is* the return
value, and `agent complete` reads it.

→ [Full docs](./tmux-mgr/README.md) · [Agent context](./tmux-mgr/AGENTS.md)

---

## 📊 `gsl` — know what your agent is costing you

> A powerline status line for Claude Code and Antigravity CLI: git state, model,
> context burn, and rate limits at a glance.

**The problem.** Mid-session you cannot answer the two questions that matter
most — *how much context is left* and *what branch am I actually on*. Both need
a detour that breaks your train of thought, and the second one is how edits end
up on the wrong branch.

**What it does about it.** Four segments render after every assistant turn:
working directory + git state, repo/PR context, the AI segment (model, context
window %, MCP count, 5h/7d rate-limit burn), and the clock. Each segment is
hard-capped at one second and drops itself rather than hanging your prompt — the
status line can never be the reason Claude feels slow.

**Reach for it when:**

- You want context-window headroom visible without asking
- You need branch and dirty-state confirmation before letting an agent edit
- You're on a terminal whose font can't render Nerd Font glyphs (there's an
  emoji and an ASCII fallback)

```console
$ gsl status
  dotfiles  main   12   2026-08-23 08:54:22 PDT

$ gsl preview --once          # single frame, full segments
$ gsl config style emoji      # no Nerd Font? no problem
```

**Gotchas.** The default `powerline` style needs a Nerd Font installed on the
*client* terminal — over SSH that means your laptop, not the remote host. Fall
back with `gsl config style emoji`. Config lives at `~/.config/gsl/config.json`;
the time segment defaults to `America/Los_Angeles`.

→ [Full docs](./gsl/README.md) · [Agent context](./gsl/AGENTS.md)

---

## 🚚 `fleet` — is every machine *actually* updated?

> Install-drift dashboard for every host you own, with an interactive TUI and a
> wake ladder for machines that are merely asleep.

**The problem.** "Did I run the installer on the Pi after that change?" You
genuinely do not know. A host can sit on the newest commit while still *running*
a config from three weeks ago, because pulling and installing are different
events. So you SSH around one box at a time, guessing — and half of them don't
answer because they're asleep, not broken.

**What it does about it.** Each host writes an install stamp (commit, branch,
timestamp) when the installer actually runs. `fleet status` reads all of them
and classifies real drift — including the tell-tale case where the checked-out
branch differs from the one last installed from. The TUI adds vim navigation,
regex search, multi-select, and concurrent updates. Unreachable hosts go through
a non-destructive wake ladder: retry → ARP prime → relay through a peer that
*is* reachable.

**Reach for it when:**

- You just shipped an installer change and need to know who has it
- You want to update a subset of hosts concurrently, watching the logs stream
- A Wi-Fi machine is unreachable and you suspect it's asleep, not down
- You need to audit or prune SSH keys across every host

```console
$ fleet status
baseline: origin/main c6fccf8

HOST      COMMIT    BRANCH   LAST RUN   STATUS
web-01    c6fccf8   main     3h ago     up-to-date
web-02    d31a5f9   main     4h ago     behind 2
pi-01     -         -        never      unknown

$ fleet wake pi-01
  retry        x   still unreachable
  local-prime  x   skipped: not on target subnet
  peer-relay   OK  via web-01

$ fleet tui        # navigate, search, select, update concurrently
```

**Gotchas.** Your `~/.ssh/config` *is* the inventory — hosts opt in with a
`#fleet` marker comment (`fleet discover --add-all` bulk-adopts). Hosts show
`unknown` until they've run an installer new enough to write the stamp, so a
fresh pull alone won't clear it. `update --force` resets a dirty clone, but
commits the existing state to a `fleet-reset/<timestamp>` branch first.

→ [Full docs](./fleet/README.md) · [Agent context](./fleet/AGENTS.md)

---

## 🚩 `gff` — flags that live in git

> **git fast features.** A layered feature-flag engine that gates installer
> steps and survives across machines, with provenance for every decision.

**The problem.** Some steps should not run on some machines — the Windows
desktop automation on a Mac, the GPU bits on a Raspberry Pi. That logic
metastasizes into a thicket of `if [ "$(uname)" = ... ]` branches and
environment variables nobody remembers setting, and there's no way to ask *why*
a step was skipped.

**What it does about it.** Flags are declared in `.gff/features.yaml` and
resolved through five layers — system snapshot, user snapshot, repo live, system
override, user override — with the winning layer shown in the output. So "why
did that step skip?" is one command, not an afternoon. Flags export to shell,
dotenv, JSON, or YAML for CI and fresh machines.

**Reach for it when:**

- An install step should be per-machine optional
- You want the same toggles on a new laptop without re-deciding them
- CI needs the flag state without cloning the repo
- You need to prove which layer set a value

```console
$ gff list
PATH                      TYPE   VALUE   LAYER          DESCRIPTION
install.ai.antigravity    bool   true    repo-live      Runs the Antigravity blocks.
install.ai.claude         bool   true    user-snapshot  Claude CLI setup
install.ai.claude         bool   true    repo-live      Runs the Claude Code blocks.

$ gff set install.ai.claude false
$ eval "$(gff export --format shell)"     # GFF_INSTALL_AI_CLAUDE=false
$ gff enabled install.ai.claude && echo run || echo skip
skip
```

**Gotchas.** The design is deliberately **fail-open**: only an exact lowercase
`false` skips a step, so a broken or missing gff runs everything rather than
silently disabling your install. Namespaces are reverse-DNS and exclusive — two
repos can't claim the same one.

→ [Full docs](./gff/README.md) · [Agent context](./gff/AGENTS.md)

---

## ⏻ `wol` — turn it on from anywhere

> Wake-on-LAN magic packets, with MAC formats that just work.

**The problem.** The machine you need is off, and you are not in the same
building. Hand-rolling a magic packet means getting 6 bytes of `0xFF` plus
sixteen MAC repetitions exactly right, on the correct broadcast address and
port.

**What it does about it.** One argument, any MAC format — colons, hyphens, dots,
or bare hex. Broadcast address and port are flags when your network needs
something other than the defaults.

**Reach for it when:**

- A lab or fleet host needs powering on before you can `ssh` to it
- `fleet wake` reports a host is genuinely down rather than asleep
- You're scripting "power on, wait, deploy"

```console
$ wol b4:2e:99:aa:79:8b
Successfully sent magic packet for b4:2e:99:aa:79:8b to 255.255.255.255:9

$ wol --bcast 192.168.1.255 B4-2E-99-AA-79-8B
```

**Gotchas.** UDP broadcast gets you no delivery confirmation — success means the
packet left, not that anything woke. Many networks need the *subnet* broadcast
(`192.168.1.255`) rather than the `255.255.255.255` default, and some hardware
wants port 7. WoL must also be enabled in the target's BIOS/NIC.

→ [Agent context](./wol/AGENTS.md)

---

## 🪵 `libs` — the shared foundation

> The logging every other tool here uses. Not a CLI — import it.

**The problem.** Tool number eight hand-rolls its own logger, picks its own log
location, and grows an unbounded file that eventually fills the disk on a
Raspberry Pi. Multiply by seven.

**What it does about it.** One import gives you structured JSON diagnostics with
lumberjack rotation, XDG-correct paths, `0600` permissions, and per-tool env
overrides (`$FLEET_LOG_LEVEL`, `$FLEET_LOG_FILE`). Separately, `Capture` records
raw subprocess output as plain text — because a captured install log's whole
value is being readable as-is.

**The rule that matters:** construction never fails. A logger that cannot open
its file discards instead, and a nil `*Capture` is safe to call. **Logging must
never introduce a failure mode into the thing it observes.**

```go
import applog "github.com/sfc-gh-eraigosa/dotfiles/sdk/libs/log"

applog.SetDefaultTool("mytool")            // once, at startup
applog.Default().WithField("host", h).Info("started")
```

**Gotchas.** Call `SetDefaultTool` before the first `Default()` — the singleton
is lazy and built once. Diagnostics go through `Default()`; captured bytes from
another process go through `NewCapture` and stay plain text.

→ [Agent context](./libs/AGENTS.md)

---

## Install & versioning

Every module is independent: its own `go.mod`, its own release tag, its own
`build.sh`.

```bash
# All of them, into ~/opt/bin (the normal path):
~/git/dotfiles/install.sh

# One of them, from source:
bash sdk/<tool>/build.sh

# One of them, from anywhere:
go install github.com/sfc-gh-eraigosa/dotfiles/sdk/<tool>@sdk/<tool>/vX.Y.Z
```

Versions are **tag-driven — there is no `VERSION` file.** `build.sh` derives the
version from `git describe --tags --match "sdk/<tool>/v*"`, so a clean release
build stamps `X.Y.Z` and a dev build stamps `X.Y.Z-<n>-g<sha>`. Release tags are
cut automatically by `.github/workflows/sdk-auto-bump.yml`.

## Adding a tool

New module? Follow the checklist in **[AGENTS.md](./AGENTS.md)** — it covers the
module path, the docs a tool ships, wiring into `install.sh`, and adding a
section to this page so it doesn't go stale.

---

*Part of [sfc-gh-eraigosa/dotfiles](../README.md) · Apache 2.0*
