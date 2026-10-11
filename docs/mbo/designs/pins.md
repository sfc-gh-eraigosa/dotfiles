# pins — version pin management for install.sh tools — design

- **Slug:** pins
- **Date:** 2026-10-10
- **Status:** Approved (shape agreed in session 2026-10-10)
- **Relates to:** spec [`../specs/pins.md`](../specs/pins.md) · plan [`../plans/pins.md`](../plans/pins.md)

## 1. Problem

`install.sh` installs tools in three different modes, and nothing documents which tool is in which mode:

| Mode | Behaviour | Tools today |
| :-- | :-- | :-- |
| **Pinned, converging** | the pin is compared with the installed version; bumping the pin upgrades every host on its next install | Go (`.go-version`), uv, glow, Rust (min floor), herdr plugins (`ai/herdr/plugins.tsv`), Actions SHAs (Dependabot) |
| **Unpinned / always latest** | upgrades on every run | apt common-core, goenv/pyenv/rbenv (`git pull`), herdr (`latest.json`), Claude Code, agy, oh-my-zsh, sdk tools (built from HEAD) |
| **Frozen at first install** ⚠️ | `command -v X && exit 0`, so a pin only ever reaches *new* hosts and "latest" means "latest the day this host was set up" | sops, yq, gitleaks, kubectl/helm/kind, snowflake-cli, fnm, nvm, Brewfile (`--no-upgrade`) |

Pins are also scattered: each one is a hardcoded `${X_VERSION:-…}` default in its own script. A
few are never reviewed at all: keyd `v2.6.0`, Nerd Fonts `v3.4.0`, vault `1.21.4`. Live evidence of drift:
`.go-version` is `1.26.1` while `sdk/{fleet,gsl,wlink}/go.mod` ask for `1.26.3`, and that mismatch is why the
Security Scan is red.

## 2. Goals

1. **One manifest** lists every managed tool with its track mode and pin.
2. **Every pinned installer converges**: bumping a pin upgrades existing hosts, not only new ones.
3. **Both modes, switchable**: a tool can be pinned or follow latest, per row in the
   manifest, and a host can override everything through gff.
4. **See what's falling behind**: one command compares each pin with the latest upstream release.
5. **Bump with one command** and review it as a normal PR.
6. **Fleet-wide**: `fleet status` shows per-host version drift, and fleet can roll out pin
   changes without a full interactive `install.sh`.

## 3. Shape (decisions)

- **D1: the manifest is `opt/profiles/versions.tsv`.** It sits next to `opt/profiles/packages.tsv`, the
  existing precedent for a whitespace-separated tool manifest. Columns:
  `tool track version upstream installer probe…` (probe is the rest of the line). `version` may be
  `@<repo-path>` to point at a file that holds the version (Go keeps `.go-version`, which goenv
  itself reads).
- **D2: shell reads it through `opt/lib/pins.sh`.** `pins_want <tool> <fallback>` resolves the version in this order:
  per-tool env override (`X_VERSION`), then `GFF_PINS_TRACK` (`manifest` | `latest` | `pin`), then the manifest row, then the
  hardcoded fallback. It fails open, so a missing lib or manifest gives today's defaults.
- **D3: the CLI is `fleet pins`, a subcommand of fleet, not a new module.** fleet is already built on every host by
  install.sh and already owns SSH fan-out, the status table and the TUI. One binary means no new alias and
  no new module checklist. (Chosen by the user over a standalone `sdk/pins` module or a shell script.)
- **D4: drift rides the existing status probe.** `fleet status` appends a third section to its single SSH
  command (`fleet pins status --installed --json`), so drift costs no extra round trip. Hosts running an older
  fleet answer without the section, and the column shows `-`.
- **D5: fleet rollout is a plan file, not new update code.** `opt/etc/fleet/pins.yaml` is a fleet.yaml
  plan (sync → rebuild fleet → `fleet pins apply`), run with
  `fleet update --file opt/etc/fleet/pins.yaml <hosts>`. Every managed installer writes to `~/opt/bin`
  without sudo, so it is safe non-interactively. A full `fleet update` still converges too, because install.sh calls the same
  installers.
- **D6: the weekly report is an issue, not a PR.** PRs opened with Actions' `GITHUB_TOKEN` do not trigger
  workflows, so required checks would never report and the Mergify queue would stall
  (see [`docs/mergify.md`](../../mergify.md)). The workflow upserts one tracking issue
  listing tools that are behind. A human runs `fleet pins bump` and lands it through gss.
- **D7: report-only rows.** `installer = -` marks tools whose installer needs root or a desktop (keyd,
  Nerd Fonts, vault). `status` and `bump` cover them; `apply` lists them as "run install.sh" and does not run them.

## 4. Out of scope (v1)

- OS package managers (apt, brew): they have their own upgrade semantics.
- Self-updating CLIs (claude, agy) and the git-pull managers (goenv/pyenv/rbenv/omz).
- `docker/Dockerfile.base` compose pin: a Dockerfile can't read the manifest. That's a follow-up.
- Automatic bump PRs: these need a GitHub App token (D6).

## 5. Risks

- **GitHub API rate limit** (60/h unauthenticated): `status` uses `GITHUB_TOKEN`, or `gh auth token`
  when available, and makes one call per row (~13).
- **Version strings are messy** (`v` prefix, `go1.26.1`, `yq (https://…) version v4.44.6`): there's a single
  extractor (first `\d+\.\d+(\.\d+)?`), table-tested against real outputs.
- **A latest-track installer resolves the network on every run.** Today that's only herdr, and it already does this.
