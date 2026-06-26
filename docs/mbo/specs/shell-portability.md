# Shell Portability Standard

**Status:** specifying → building · **Slug:** `shell-portability` · **Owner:** edward-raigosa

The contract every shell script and sourced profile fragment in this repo MUST follow so
that one checkout behaves identically across our three supported host families. This standard
is **normative**: CI (`make lint-shell` + the `shell-lint` workflow) enforces the mechanical
parts, and code review enforces the rest.

> **Why this exists.** `./install.sh` on macOS catastrophically failed when
> `eval "$(goenv init -)"` clobbered `PATH`, after which *every* coreutil
> (`tr`, `uname`, `git`, `mkdir`, `curl`, `bash`…) became `command not found`
> and the remainder of the install silently collapsed. The same checkout also
> surfaced a `read -A` (zsh-only) error from a drifted local copy. Both are
> classic cross-shell / cross-OS portability failures. This document captures
> the rules that prevent the whole class.

---

## 1. Target platform matrix

| Family | Login shell | Script interpreter | Coreutils | Notes |
| :-- | :-- | :-- | :-- | :-- |
| **WSL2 + Ubuntu** (20.04 LTS and newer) | bash (often zsh) | bash 5.x | GNU | Windows interop on `PATH`; `uname -r` ends in `-microsoft-*`/`-Microsoft`. |
| **macOS** (Apple Silicon & Intel) | **zsh** (default since Catalina) | **bash 3.2** system bash, or Homebrew bash 5 | **BSD** (+ optional `coreutils` keg) | The single biggest source of portability bugs — BSD tools and an ancient `/bin/bash`. |
| **Linux** — Raspberry Pi OS, Jetson **Nano**, generic | bash | bash 5.x | GNU | ARM; sometimes minimal images (no `curl`, no `git` until installed). |

A script is "portable" only if it works on **all three** without modification.

---

## 2. The rules

### 2.1 Shebang & shell declaration
- **Executable scripts:** first line MUST be `#!/usr/bin/env bash` (not `#!/bin/bash` — on
  macOS `/bin/bash` is the 3.2 relic; `env bash` picks up Homebrew bash 5 when present).
- **Sourced profile fragments** (`.bashrc`, `.zshrc`, `.goenv.sh`, `.docker.sh`, …) have **no
  shebang** because they are `source`d. They are read by **both bash and zsh**, so their code
  MUST be dual-shell safe (see §2.3). Add a `# shellcheck shell=bash` directive at the top so
  shellcheck lints them deterministically.

### 2.2 Two shell worlds — keep them straight
1. **Scripts** run under bash → bash features are fine **subject to the bash-3.2 caveat (§2.4)**.
2. **Sourced fragments** run under whatever the user's login shell is (bash *or* zsh) → restrict
   to constructs that mean the same thing in both, or guard by `$ZSH_VERSION` / `$BASH_VERSION`.

### 2.3 Banned cross-shell-isms
- **`read -A`** is zsh array-read; in bash it is an error (`read: -A: invalid option`). In a
  bash script use `read -a` (or `mapfile`/`readarray`, bash 4+). In a **sourced fragment** that
  both shells read, avoid array reads entirely or branch on `$ZSH_VERSION`.
- No `setopt`, `${(s:x:)var}`, `${~var}`, `=command`, or zsh glob qualifiers in bash scripts.
- Conversely, no bash-only `declare -A`, `${var,,}`, `mapfile` in sourced fragments unless
  guarded — zsh chokes on them.
- Prefer `[[ … ]]` (works in bash **and** zsh) over `[ … ]` only when you need its features;
  pure-POSIX `[ … ]` is always safe.

### 2.4 macOS bash is 3.2 (2007) — avoid bash-4+ features in `/bin/bash`-reachable code
Associative arrays (`declare -A`), `${var,,}` / `${var^^}` case conversion, `mapfile`/`readarray`,
and `|&` are bash-4+. They break under macOS system bash. Either (a) require `#!/usr/bin/env bash`
**and** ensure Homebrew bash is installed, or (b) write 3.2-compatible code. Document which path a
script takes.

### 2.5 BSD vs GNU coreutils (macOS ships BSD) — the high-frequency traps
| Tool | GNU (Linux/WSL) | BSD (macOS) | Portable approach |
| :-- | :-- | :-- | :-- |
| `sed -i` | `sed -i 's/…/…/' f` | `sed -i '' 's/…/…/' f` | Use a tmpfile + `mv`, or `perl -pi -e`. |
| `stat` mtime | `stat -c %Y f` | `stat -f %m f` | Try one, fall back to the other (see §4). |
| `date` | `date -d @123` | `date -r 123` / `date -v` | Branch on OS, or compute in shell. |
| `readlink -f` | yes | no (old BSD) | Ship a `realpath()` shell shim or use `cd … && pwd`. |
| `grep -P` (PCRE) | yes | no | Use `-E` (ERE) and rewrite the pattern. |
| `base64 -w0` | yes | no `-w` | `base64 | tr -d '\n'`. |
| `find -printf` | yes | no | `find … -exec` / `-print0 | while read`. |
| `timeout` | yes | not by default | Guard with `command -v timeout`. |

### 2.6 `eval "$(tool init -)"` — pass the shell explicitly, and guard `PATH`
Two distinct hazards, both real and both observed on macOS:

**(a) Pass the shell name — do not let the tool infer it.** `goenv init -` (and
`pyenv`/`rbenv`) infer the target shell from **`$SHELL`**, *not* the shell actually running the
script. On a host whose login shell is zsh (`$SHELL=…/zsh`), a **bash** script that runs bare
`eval "$(goenv init -)"` gets **zsh** code back — including a `while IFS=: read -rA … done <<<
"$PATH"` PATH-rebuild loop. Bash errors `read: -A: invalid option`, the loop's accumulator stays
empty, and the trailing `export PATH="$_NEW_PATH"` **wipes PATH** — after which every coreutil is
"command not found". Always name the shell:

```sh
eval "$(goenv init - bash)"        # in a bash script — forces bash-safe output
```

In a fragment sourced by *either* shell (`.goenv.sh`), detect the live shell and pass it
(`$ZSH_VERSION` → `zsh`, `$BASH_VERSION` → `bash`, empty → let the tool detect):

```sh
if [ -n "${ZSH_VERSION:-}" ]; then _sh=zsh; elif [ -n "${BASH_VERSION:-}" ]; then _sh=bash; else _sh=""; fi
eval "$(goenv init - "$_sh")"
```

**(b) Guard `PATH` anyway (belt-and-suspenders).** Even with (a), keep a restore guard so any
future tool/version that drops the system bin dirs cannot break the rest of the script:

```sh
__path_safe="$PATH"
eval "$(goenv init - bash)"
case ":${PATH}:" in
    *":/usr/bin:"*) : ;;                      # system PATH survived
    *) PATH="${PATH}:${__path_safe}" ;;       # init clobbered PATH; restore
esac
export PATH
unset __path_safe
```

This guard is itself POSIX (`case` + parameter expansion) so it is safe in bash **and** zsh.
Reference implementations: `install.sh` (goenv block) and `opt/profiles/.goenv.sh`.

### 2.7 PATH & command hygiene
- Never assume any binary exists — `command -v foo >/dev/null || { echo "need foo"; … }`.
- Use `command -v`, never `which` (not guaranteed installed; BSD/GNU differ).
- Keep `/usr/bin:/bin:/usr/sbin:/sbin` on `PATH`; prepend tool dirs, never replace.

### 2.8 Portability-safe defaults
- Quote every expansion (`"${var}"`); `set -euo pipefail` is encouraged for **scripts** (never
  in sourced fragments — it would change the user's interactive shell).
- Use `$HOME`/`~`, never hardcoded `/Users/...` or `/home/...` (see root `CLAUDE.md`).
- Detect WSL via `uname -r` matching `-[Mm]icrosoft`; detect macOS via `[ "$(uname -s)" = Darwin ]`.

---

## 3. Enforcement

| Layer | Mechanism | Scope |
| :-- | :-- | :-- |
| Config | `.shellcheckrc` (`external-sources=true`, SC1090/91 disabled) | repo-wide |
| Local/CI lint | `make lint-shell` → `shellcheck -x -S warning` | all `*.sh` + listed profile dotfiles |
| Dedicated CI | `.github/workflows/shell-lint.yml` | **every** `*.sh` on push/PR |
| Syntax gate | `bash -n` in the shell-test harness | per script |
| **Portability gate (enforcing)** | `make lint-portability` → `opt/scripts/system/shell-portability-scan.sh --strict` in `shell-lint.yml` — **fails CI** on Tier 1 (dash `/bin/sh` parse breakage) or Tier 2 (BSD/bash-3.2 hazard); Tier 3 informational. Opt out a reviewed line with `# portability-ok: <reason>`. | every `*.sh` + dash-sourced profile fragments |
| Review | This standard | everything shellcheck can't see (runtime `PATH`, BSD/GNU, bash-3.2) |

`make lint-shell` historically scanned only `install.sh` + `opt/scripts/**` + `ai/**` +
`opt/profiles/*`. The shell-lint workflow closes the coverage gap to include `sdk/**`, `src/**`,
and `opt/bin/**` (≈17 previously-unlinted scripts).

---

## 4. Worked example — the `stat` mtime fallback (already in `opt/profiles/.goenv.sh`)

```sh
if mtime=$(stat -f %m "${file}" 2>/dev/null); then   # BSD / macOS
    :
else
    mtime=$(stat -c %Y "${file}" 2>/dev/null || echo 0)  # GNU / Linux
fi
```

Try the BSD form, fall back to GNU, default to `0`. This is the canonical "probe, don't assume"
shape for any tool whose flags differ across coreutils.

---

## 5. Checklist for a new / changed shell script

- [ ] `#!/usr/bin/env bash` (script) **or** `# shellcheck shell=bash` (sourced fragment).
- [ ] `shellcheck -x -S warning` clean; `bash -n` clean.
- [ ] `make lint-portability` clean — the **enforcing** dash + macOS scan (`shell-portability-scan.sh --strict`); it fails CI on any Tier 1 (dash `/bin/sh` parse breakage) or Tier 2 (BSD/bash-3.2 hazard). Reviewed exceptions carry a trailing `# portability-ok: <reason>`.
- [ ] No `read -A` / zsh-isms in bash; no unguarded bash-4+ in `/bin/bash`-reachable or sourced code.
- [ ] Every coreutil flag verified on **both** BSD and GNU (or probed with a fallback).
- [ ] Any `eval "$(tool init)"` wrapped in the §2.6 `PATH` guard.
- [ ] No hardcoded home paths; binaries checked with `command -v` before use.
- [ ] Mentally run it on macOS-zsh, WSL-Ubuntu, and a Pi/Nano before claiming done.
