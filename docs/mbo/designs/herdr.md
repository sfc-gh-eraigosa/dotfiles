# herdr — research evaluation & install design

- **Slug:** `herdr`
- **Date:** 2026-09-01 (all maintenance numbers observed this date)
- **Status:** Proposed
- **Relates to:** issue [#260](https://github.com/sfc-gh-eraigosa/dotfiles/issues/260) · this PR (gss feature `herdr`, worker `install`)
- **Target:** <https://github.com/herdrdev/herdr> · <https://herdr.dev> · "the runtime your coding agents live on"
- **Verdict:** **adopt selectively** — install as a pinned-or-latest, checksummed, gff-gated tool with agent
  integrations; keep tmux + `tmux-mgr` as the orchestration layer.

Produced with the `research-evaluation` skill. Active rubric weighting (from `gff list research-rubric.*`):
security **critical**; value, adversarial, borrowable **high**; setup/licensing, stability, quality,
business **medium**; demo **low**. Gates: adversarial section required, docker-or-skip demo.

## 1. Problem / context

herdr is a Rust terminal workspace manager built for running many coding agents at once: mouse-first
panes/tabs/workspaces, a sidebar that shows each detected agent's state (`working` / `blocked` / `done` /
`idle`), per-repo git worktrees under `~/.herdr/worktrees`, a local socket API, and a CLI. It was already
present on the DGX Spark host (v0.8.2 in `~/.local/bin`, installed by the upstream `curl | sh` installer)
with nothing in the dotfiles tracking it. The ask: how should the dotfiles install it (script / nix / other),
can it be gff-gated, is the licence acceptable, what else needs checking, can the agent integrations be
configured too, and how do we keep up with releases across the fleet (including the Pi).

## 2. Nine-dimension dossier

### (a) Value — what it is worth to us

- Fills a real gap: cross-project **agent state visibility** (which Claude/agy pane is blocked on a prompt)
  that neither tmux nor `tmux-mgr` provides.
- Per-repo worktree management overlaps with `gss feature` workers; herdr's worktrees live in
  `~/.herdr/worktrees`, gss's in `~/.config/gss/worktrees`. They coexist — this doc was written from a
  herdr worktree and landed through a gss worker — but it is a second worktree convention to know about.
- Overlaps with tmux as a multiplexer. `sdk/tmux-mgr` hard-requires `tmux` on PATH, the `tmux` /
  `tmux-agent` skills orchestrate through it, and several profile aliases assume tmux. herdr therefore
  **complements** the stack rather than replacing anything.

### (b) Setup cost & licensing

- **Licence: Apache-2.0** — confirmed in both `LICENSE` and the `Cargo.toml` `license` field. Meets the
  "Apache/MIT or equivalent" bar. No commercial tier; `SPONSORS.md` and an enterprise contact e-mail only.
- **Install surfaces upstream:** `curl -fsSL https://herdr.dev/install.sh | sh`, `brew install herdr`,
  `mise use -g herdr`, a `flake.nix` (+ nixpkgs), a Windows PowerShell installer, and raw GitHub release
  assets (`herdr-linux-{x86_64,aarch64}`, `herdr-macos-{x86_64,aarch64}`, `herdr-windows-x86_64.zip`).
  Linux assets are **statically linked musl** binaries (verified with `file`/`ldd` on the Spark).
- **Our fit:** this repo has no mise and no nix surface (only a `~/.config/nix_managed` bail-out sentinel);
  the Brewfile is macOS-extras-only. The established pattern for binaries is a pinned release download to
  `~/opt/bin` (`install_sops.sh`, `install_yq.sh`, `install_k8s_tools.sh`). Setup cost: one script.
- The upstream installer verifies SHA-256 from `herdr.dev/latest.json` but cannot pin a version, defaults
  to `~/.local/bin`, and is a `curl | sh` pattern the repo avoids. We reuse its **manifest**, not the script.

### (c) Adversarial review — the case against

- **Pre-1.0 and five months old** (created 2026-03-27). Releases roughly every two weeks; breaking changes
  are to be expected. The client/server **protocol is versioned** (currently 20): hosts on different
  releases may not `herdr --remote` into each other.
- **Bus factor ≈ 1.** 30 contributors, but one author has ~1,180 of the commits; the next humans have 25.
  The two "bots" in the top five are the maintainer's automation.
- **Self-update fights any pin.** `herdr update` overwrites the binary it runs from; a stable/preview
  channel is a per-host setting. Without a fleet-wide convergence step, hosts drift.
- **Key collision with tmux.** herdr's default prefix is `ctrl+b`, the same as our untouched tmux default;
  `.tmux.conf` also binds `C-h/j/k/l` in the root table and claims the mouse. herdr **inside** tmux loses
  its prefix and its mouse. Run it as the outer layer or rebind `[keys]`. *Resolved (worker `theme`):* the
  managed `ai/herdr/config.toml` sets `prefix = "ctrl+a"`.
- **Integration clobbering.** `install_antigravity_skills.sh` re-renders `~/.gemini/config/hooks.json`
  from the repo template on every run, deleting herdr's `herdr` hook entry. The Claude `SessionStart` hook
  survives the forced-settings merge only because that merge replaces `hooks.PreToolUse` /
  `hooks.DirectoryAdded` and preserves other keys — a fragile coincidence to be aware of.
- 87 open bug-labelled issues; heavy churn in agent-detection heuristics (screen-scraping titles/spinners).
- A second **worktree convention** (`~/.herdr/worktrees`) next to gss's.

### (d) Security & safety

- **No telemetry found:** zero code hits for `telemetry` / `analytics` in the Rust sources; the socket API
  and client/server logs are local (`~/.config/herdr/*.sock`, `*.log`). Network egress: release manifest +
  GitHub assets for updates, plus the optional `--remote` bridge.
- **Supply chain:** release assets carry a published SHA-256 manifest but **no GitHub artifact attestation**
  (`gh attestation verify` → 404) and no Sigstore signatures. Vendored dependencies
  (`vendor/libghostty-vt`, `vendor/portable-pty`, with patch sets) widen the surface; `Cargo.lock` is
  ~64 KB.
- **What it executes/touches:** spawns shells and agent processes in panes; the integrations write hook
  scripts into `~/.claude/hooks`, `~/.gemini/config/hooks`, etc. and register them in each agent's hook
  config. The hook (`herdr-agent-state.sh`) exits immediately unless `HERDR_ENV=1`, so it is inert outside
  herdr panes.
- Mitigation adopted: every download verified against the manifest checksum, **fail-closed** when no
  checksum is published for the exact version+target.

### (e) Stability

- Blast radius is bounded: a user-space binary in `~/opt/bin` plus hook scripts. Nothing system-wide, no
  services, no PATH changes beyond what the profiles already do.
- The known destabiliser is the tmux key collision above, and version drift between hosts for `--remote`.
- Since 0.8.x: "Stable direct installs, self-updates, and remote helper downloads now require and verify the
  SHA-256 digest published for each GitHub release asset" — upstream is moving in the right direction.

### (f) Quality & support (observed 2026-09-01)

| Signal | Value |
| :-- | :-- |
| Stars / forks | 34,474 / 2,525 |
| Contributors | 30 (one dominant author) |
| Open issues | 279 (87 labelled bug); 470 closed since 2026-08-01 |
| Releases | v0.7.0 (06-15) → v0.7.5 (07-21) → v0.8.0 (08-03) → v0.8.2 (08-19); preview builds most days |
| Last push | 2026-09-01 |
| Docs | herdr.dev/docs (quick start, config reference, agents, socket API), `AGENTS.md`, `agent-guide.md`, `llms.txt`, a bundled `herdr` skill |

Very active, well documented, responsive — and young.

### (g) Demo — docker-or-skip

Not run (weight **low**, and the tool is already in daily use on this host, which is stronger first-hand
evidence than a sandbox). A sandboxed plan for the record:

```bash
docker run --rm -it -e HERDR_INSTALL_DIR=/tmp/bin ubuntu:24.04 bash -c '
  apt-get update -qq && apt-get install -y -qq curl jq ca-certificates >/dev/null
  curl -fsSL https://herdr.dev/latest.json -o /tmp/latest.json
  url=$(jq -r ".assets[\"linux-x86_64\"]" /tmp/latest.json); sha=$(jq -r ".sha256[\"linux-x86_64\"]" /tmp/latest.json)
  curl -fsSL "$url" -o /tmp/herdr && echo "$sha  /tmp/herdr" | sha256sum -c && chmod +x /tmp/herdr
  /tmp/herdr --version && /tmp/herdr --default-config | head -40'
```

Success criteria: checksum verifies, `--version` matches the manifest, default config prints. Teardown is
implicit (`--rm`). No credentials involved.

### (h) Borrowable features — build vs adopt

| Gap herdr fills | Value | Build it ourselves? | Worth it? |
| :-- | :-- | :-- | :-- |
| Agent state sidebar (working/blocked/done across panes) | high | tmux status-line + `gsl` already render per-pane state for Claude; extending to a cross-session roll-up is a `tmux-mgr` feature | partially — `gsl` covers the single pane; the roll-up is the piece we lack |
| Mouse-first pane/tab UI | medium | no — that is the product | no |
| Per-repo worktree manager | low (gss has it) | already have `gss feature` | no |
| Local socket API to drive panes | medium | `tmux-mgr` already wraps `tmux send-keys`/`capture-pane` | no |
| Native session restore for Claude/agy | medium | would mean re-implementing the hook + restore plumbing per agent | no |

Conclusion: the one borrowable idea is the **cross-session agent-state roll-up** (a `tmux-mgr`/`gsl`
follow-up); everything else is cheaper to adopt than to rebuild. Verdict stays *adopt selectively*.

### (i) Business outcomes

- **Time saved:** medium — fewer "is that agent waiting on me?" round trips when several agents run.
- **Cost savings:** low — no paid tier replaced.
- **Revenue potential:** none directly.

## 3. Options considered (install shape)

1. **Pinned-or-latest release-binary script to `~/opt/bin` (chosen).** Same shape as `install_sops.sh` /
   `install_k8s_tools.sh`; adds manifest-checksum verification (none of the existing binary installers
   verify checksums — this closes that gap for herdr). Works unchanged on WSL, macOS, the Jetson Nano and
   the 64-bit Pi (both ARM fleet hosts report `aarch64`).
2. **Upstream `curl | sh`.** Rejected: no pin, `~/.local/bin`, pattern the repo's safety hook flags.
3. **Nix / mise.** Rejected: no such surface in this repo; adding a toolchain for one tool is not a fit.
4. **Brew row.** Not added: the script covers macOS; a brew copy would lag the latest-tracking policy and
   the script's "already installed" probe would then re-download anyway.
5. **Build from source on the Pi.** Rejected: both ARM hosts are 64-bit and upstream ships a static
   `aarch64` musl binary; a source build needs Rust + cmake + ninja for the vendored libghostty-vt for no
   gain. Only a 32-bit armv7 Pi would need a build, upstream has no armv7 target, and we have none.

## 4. Decision

- `opt/scripts/system/install_herdr.sh` — two modes:
  - default (`install.sh --phase deps`, flag `install.tools.herdr`): read `herdr.dev/latest.json`; resolve
    `HERDR_VERSION` (default `latest`); skip when `~/opt/bin/herdr --version` already matches; otherwise
    download the release asset, **verify SHA-256 from the manifest (fail-closed)**, atomically replace the
    binary, verify it runs, and warn if another `herdr` shadows ours in PATH. `armv7l/armv6l` → skip.
  - `integrations` (`install.sh --phase config`, flag `install.tools.herdr-integrations`): for each id in
    `HERDR_INTEGRATIONS` (default `claude antigravity-cli`) whose agent CLI exists, run
    `herdr integration install <id>` (idempotent upstream). Placed **after** `install_antigravity_skills.sh`
    so the hooks.json re-render is repaired on every run.
- Two gff flags in `.github/gff/features.yaml` (`tools` group, default on); `INSTALL_TOOLS_HERDR` in
  `_IP_DEPS_FLAGS`, `INSTALL_TOOLS_HERDR_INTEGRATIONS` in `_IP_CONFIG_FLAGS`.
- **Keeping up with releases:** the default is *latest from the manifest*, so every `install.sh` /
  `fleet update` converges all hosts on the same version. `HERDR_VERSION=x.y.z` pins a host when a release
  misbehaves (the manifest carries checksums for older releases, so pins are still verified).
- No `.tmux.conf` changes in this PR; the collision is documented and left to the user's choice of outer
  layer (herdr as the outer layer is the upstream-intended shape).

## 5. Risks & blast radius

See (c)–(e). Worst realistic case: a bad upstream release lands fleet-wide via "latest" — mitigation is
`HERDR_VERSION` (env or a gff-driven override later) plus `HERDR_FORCE=1` to roll back. Hook writes are
confined to agent config dirs and are inert outside herdr panes.

## 6. Rollback

`gff set install.tools.herdr false` and `gff set install.tools.herdr-integrations false`; remove
`~/opt/bin/herdr`; `herdr integration uninstall claude antigravity-cli` before removing the binary if the
hooks should go too. `~/.config/herdr` and `~/.herdr/worktrees` are user data and are left alone.

## 7. Evidence expectations

- `opt/scripts/system/install_herdr_test.sh` (no network): already-installed no-op, fail-closed on
  unknown release / missing checksum, integrations skip for absent CLIs, gff wiring + phase-list membership,
  ordering after `install_antigravity_skills.sh`.
- `make lint-shell` and `make lint-portability` clean.
- Real-machine evidence to capture at land time: one `install.sh` run on the Spark showing the
  "already installed" path and both integrations reporting `current`, and one `fleet update` bringing the
  Pi to the manifest version.

## 8. Follow-ups (not in this PR)

- A gff **choice** flag for the version policy (`latest` vs a pinned string) once a pin is actually needed.
- `tmux-mgr`/`gsl`: the cross-session agent-state roll-up from (h).
- ~~A herdr counterpart of the `tmux` skill.~~ Done (worker `theme`): `ai/skills/herdr` maps the tmux-mgr
  verbs onto herdr's CLI and bundles `herdr-layout` (named tab layouts via socket `layout.export` /
  `layout.apply`, host-local under `~/.config/herdr/layouts`) and `herdr-prefs` (host-local
  `config.toml` edits that drop the managed marker; `reset` returns to the baseline).
- ~~Track `~/.config/herdr/config.toml` under `opt/etc/` via the copy pattern.~~ Done (worker `theme`):
  `ai/herdr/config.toml` is rendered by `install_herdr.sh config` (gff `install.tools.herdr-config`). It
  turns on herdr's host light/dark following (DEC 2031, verified against Windows Terminal) with the
  Solarized pair, so the default dark catppuccin sidebar no longer lands on a Solarized Light profile, and
  moves the prefix to `ctrl+a`. Host-owned: rewritten only while the `managed by dotfiles` marker is present.
- Re-evaluate at herdr 1.0 or when signed/attested release assets appear.
