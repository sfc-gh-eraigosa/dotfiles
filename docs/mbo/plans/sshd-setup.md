# sshd-setup — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use
> checkbox (`- [ ]`) syntax for tracking.

- **Slug:** sshd-setup
- **Date:** 2026-07-16
- **Status:** Draft
- **Relates to:** spec `../specs/sshd-setup.md` · issue #169 · PR #170

**Goal:** An on-demand `sshd-setup` bash tool (+ `setup-sshd.ps1` for Windows-native) that
installs/starts the OS-native sshd, opens the firewall, and seeds `authorized_keys` from the
GitHub public keys of the account derived from git credentials.

**Architecture:** One bash dispatcher in `opt/bin/` (function-per-unit, every external command
mockable via PATH shadowing, mirroring `pkg-install`), one admin PowerShell script for the
Windows OpenSSH Server capability, docs, and a `*_test.sh` driver auto-discovered by
`make shell-test`.

**Tech Stack:** bash + `ai/_test_helpers.sh` · PowerShell 5+ · `pkg-install` · GitHub `.keys`
endpoint · shellcheck.

## Global Constraints

- `set -euo pipefail` in the bash tool; shellcheck-clean (`make lint-shell`).
- Nothing runs unless invoked (no rc/install.sh hook); `status` and `--dry-run` never mutate.
- Native-first: if an sshd is already active, never install/start a second one.
- Key seeding is additive + deduped; an EMPTY GitHub keys response aborts with exit 1.
- Idempotent: second `enable` run performs zero new mutations.
- GitHub account precedence (exact order): `gh api user --jq .login` → owner segment of
  `git remote get-url origin` → `git config github.user` → error exit 1.

## 1. Summary & verdict

Restores and future-proofs SSH access lost when a Windows update removed the OpenSSH Server
capability. Two entry points (bash / ps1), shared behavior contract, validated on the
Windows+WSL host that motivated it. No open design questions; spec approved 2026-07-16.

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `opt/bin/sshd-setup` | bash tool: `status\|enable\|keys`, `--dry-run` | spec §3, F1–F9 |
| `opt/bin/sshd-setup_test.sh` | mock-command test driver (auto-discovered) | spec §5, §6 |
| `opt/Desktop/Apps/scripts/setup-sshd.ps1` | Windows-native capability/service/firewall/keys (+`-WslPortProxy`) | spec F10 |
| `docs/sshd-setup.md` | per-OS how-to + Windows+WSL walkthrough + rollback | spec §1, §9 |
| `docs/mbo/index.md` | register objective row | mbo policy |

No changes to `install.sh`, `Makefile`, or `scripts/test.sh` — discovery of
`opt/bin/*_test.sh` and shellcheck coverage are already wired.

## 3. Interface contracts

CLI: `sshd-setup [--dry-run] {status|enable|keys}` — exit 0 success / 1 failure; `status`
exit 0 even when sshd absent (it reports, mutating nothing).

Functions (all in `opt/bin/sshd-setup`; unit = mock boundary):

```bash
detect_platform        # -> echoes linux|macos|wsl   (uname -s + grep -qi microsoft /proc/version)
sshd_running           # -> 0 if active (systemctl is-active ssh|sshd, pgrep -x sshd, macOS systemsetup -getremotelogin)
sshd_installed         # -> 0 if binary present (command -v sshd || -x /usr/sbin/sshd; macOS always 0)
install_sshd           # pkg-install openssh-server (linux/wsl); macOS no-op
enable_service         # sudo systemctl enable --now ssh||sshd ; macOS: sudo systemsetup -setremotelogin on
configure_firewall     # ufw active -> sudo ufw allow 22/tcp ; firewalld running -> add-port; else note+skip
derive_github_user     # precedence per Global Constraints; echoes login
seed_keys              # curl -fsS ${SSHD_SETUP_KEYS_URL:-https://github.com/<user>.keys}; refuse empty; merge+dedupe ~/.ssh/authorized_keys; chmod 700/600
write_sshd_env         # ensure ~/.sshd.env contains SSHD_LOGIN=true
wsl_handoff            # print admin-PowerShell command for setup-sshd.ps1 (+ portproxy hint)
run                    # $DRY_RUN=1 -> echo "DRY: $*" ; else "$@"   (wraps every mutation)
```

`SSHD_SETUP_KEYS_URL` env override exists solely so tests can point at a local fixture.

PowerShell: `setup-sshd.ps1 [-GithubUser <login>] [-WslPortProxy] [-WslPort 2222]`,
`#Requires -RunAsAdministrator`; same key precedence (param overrides detection).

## 4. TDD build order

### Task 1: scaffold + platform detection

**Files:** Create `opt/bin/sshd-setup`, `opt/bin/sshd-setup_test.sh`
**Interfaces produced:** `detect_platform`, `usage`, arg dispatch, `run`/`DRY_RUN`.

- [ ] **Step 1: failing test** — create `opt/bin/sshd-setup_test.sh`:

```bash
#!/usr/bin/env bash
# Test driver for opt/bin/sshd-setup. Mocks external commands via PATH
# shadowing (same pattern as pkg-install_test.sh). Run: bash opt/bin/sshd-setup_test.sh
set -u
SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"
SSHD_SETUP="${SELF_DIR}/sshd-setup"
TMPDIR_TEST="$(mktemp -d)"; trap 'rm -rf "$TMPDIR_TEST"' EXIT

assert_exit_code 0 "sshd-setup parses with bash -n" bash -n "$SSHD_SETUP"

# usage on no args -> exit 1, mentions subcommands
set +e; OUT=$(bash "$SSHD_SETUP" 2>&1); RC=$?; set -e
assert_eq "$RC" "1" "no args exits 1"
case "$OUT" in *"status|enable|keys"*) echo "PASS: usage lists subcommands"; PASS=$((PASS+1));;
  *) echo "FAIL: usage missing subcommand list (got: $OUT)"; FAIL=$((FAIL+1));; esac

# platform detection: wsl via fake /proc/version
echo "Linux version 6.6 (Microsoft@Microsoft.com)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "wsl" "detects wsl from microsoft /proc/version"
echo "Linux version 6.6 (gcc)" > "${TMPDIR_TEST}/proc_version"
OUT=$(SSHD_SETUP_PROC_VERSION="${TMPDIR_TEST}/proc_version" bash "$SSHD_SETUP" _detect 2>&1)
assert_eq "$OUT" "linux" "detects plain linux"
_test_report
```

- [ ] **Step 2:** `bash opt/bin/sshd-setup_test.sh` → FAIL (file missing).
- [ ] **Step 3: minimal implementation** — create `opt/bin/sshd-setup`:

```bash
#!/usr/bin/env bash
# sshd-setup — on-demand native sshd install/enable + GitHub-key seeding.
# Only acts when invoked; see docs/sshd-setup.md. Windows: setup-sshd.ps1.
set -euo pipefail

DRY_RUN=0
run() { if [ "$DRY_RUN" = 1 ]; then echo "DRY: $*"; else "$@"; fi; }

detect_platform() {
    case "$(uname -s)" in
        Darwin) echo macos; return ;;
        Linux)
            local pv="${SSHD_SETUP_PROC_VERSION:-/proc/version}"
            if [ -r "$pv" ] && grep -qi microsoft "$pv"; then echo wsl; else echo linux; fi ;;
        *) echo "sshd-setup: unsupported platform $(uname -s)" >&2; exit 1 ;;
    esac
}

usage() { echo "Usage: sshd-setup [--dry-run] {status|enable|keys}" >&2; exit 1; }

main() {
    [ $# -ge 1 ] || usage
    [ "$1" = "--dry-run" ] && { DRY_RUN=1; shift; }
    case "${1:-}" in
        _detect) detect_platform ;;   # test hook
        status|enable|keys) "cmd_$1" ;;
        *) usage ;;
    esac
}
main "$@"
```

(`cmd_status`/`cmd_enable`/`cmd_keys` arrive in Tasks 2/4/5 — until then dispatch fails,
which only the later tests exercise.)

- [ ] **Step 4:** rerun test → PASS. **Step 5:** `git add opt/bin/sshd-setup*` ·
  `git commit -m "feat(sshd-setup): scaffold with platform detection (TDD)"`

### Task 2: status — running/installed probes, read-only

**Interfaces produced:** `sshd_running`, `sshd_installed`, `cmd_status`.
**Consumes:** `detect_platform`.

- [ ] **Step 1: failing tests** (append to driver before `_test_report`):

```bash
# status with mocked systemctl "active" -> reports running, exit 0
mkdir -p "${TMPDIR_TEST}/mocks"
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0
exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
OUT=$(PATH="${TMPDIR_TEST}/mocks:$PATH" bash "$SSHD_SETUP" status 2>&1)
case "$OUT" in *"running"*) echo "PASS: status reports running"; PASS=$((PASS+1));;
  *) echo "FAIL: status missing 'running' (got: $OUT)"; FAIL=$((FAIL+1));; esac

# status with everything absent -> still exit 0, reports not installed
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 3
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
cat > "${TMPDIR_TEST}/mocks/pgrep" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${TMPDIR_TEST}/mocks/pgrep"
set +e
OUT=$(PATH="${TMPDIR_TEST}/mocks:$PATH" SSHD_SETUP_SSHD_PATH=/nonexistent bash "$SSHD_SETUP" status 2>&1); RC=$?
set -e
assert_eq "$RC" "0" "status exits 0 even when sshd absent"
case "$OUT" in *"not installed"*) echo "PASS: status reports not installed"; PASS=$((PASS+1));;
  *) echo "FAIL: status missing 'not installed' (got: $OUT)"; FAIL=$((FAIL+1));; esac
```

- [ ] **Step 2:** run → FAIL (`cmd_status` undefined).
- [ ] **Step 3: implement** (insert above `main`):

```bash
sshd_running() {
    if command -v systemctl >/dev/null 2>&1; then
        systemctl is-active --quiet ssh 2>/dev/null && return 0
        systemctl is-active --quiet sshd 2>/dev/null && return 0
    fi
    if [ "$(detect_platform)" = macos ]; then
        systemsetup -getremotelogin 2>/dev/null | grep -qi "on" && return 0
    fi
    pgrep -x sshd >/dev/null 2>&1
}

sshd_installed() {
    [ "$(detect_platform)" = macos ] && return 0
    command -v sshd >/dev/null 2>&1 && return 0
    [ -x "${SSHD_SETUP_SSHD_PATH:-/usr/sbin/sshd}" ]
}

cmd_status() {
    echo "platform: $(detect_platform)"
    if sshd_running; then echo "sshd: running"
    elif sshd_installed; then echo "sshd: installed, not running"
    else echo "sshd: not installed"; fi
    [ -f "${HOME}/.sshd.env" ] && echo "sshd.env: present" || echo "sshd.env: absent"
    [ -f "${HOME}/.ssh/authorized_keys" ] \
        && echo "authorized_keys: $(wc -l < "${HOME}/.ssh/authorized_keys" | tr -d ' ') line(s)" \
        || echo "authorized_keys: absent"
}
```

- [ ] **Step 4:** run → PASS. **Step 5:** commit `feat(sshd-setup): read-only status probes`.

### Task 3: GitHub account derivation

**Interfaces produced:** `derive_github_user` (echoes login; exit 1 with message when none).

- [ ] **Step 1: failing tests** — mock `gh` present/absent, `git` remote/config fallbacks:

```bash
# gh present wins
cat > "${TMPDIR_TEST}/mocks/gh" <<'EOF'
#!/usr/bin/env bash
echo "gh-user"
EOF
chmod +x "${TMPDIR_TEST}/mocks/gh"
OUT=$(PATH="${TMPDIR_TEST}/mocks:$PATH" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "gh-user" "gh api user wins precedence"

# gh absent -> origin owner (https)
rm -f "${TMPDIR_TEST}/mocks/gh"
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "remote get-url origin") echo "https://github.com/origin-owner/repo.git" ;;
  "config github.user") exit 1 ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
OUT=$(PATH="${TMPDIR_TEST}/mocks:/usr/bin:/bin" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "origin-owner" "falls back to origin owner (https URL)"

# ssh-style URL parses too
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "remote get-url origin") echo "git@github.com:ssh-owner/repo.git" ;;
  "config github.user") exit 1 ;;
esac
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
OUT=$(PATH="${TMPDIR_TEST}/mocks:/usr/bin:/bin" bash "$SSHD_SETUP" _github_user 2>&1)
assert_eq "$OUT" "ssh-owner" "falls back to origin owner (ssh URL)"

# nothing available -> exit 1
cat > "${TMPDIR_TEST}/mocks/git" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "${TMPDIR_TEST}/mocks/git"
set +e
PATH="${TMPDIR_TEST}/mocks:/usr/bin:/bin" bash "$SSHD_SETUP" _github_user >/dev/null 2>&1; RC=$?
set -e
assert_eq "$RC" "1" "no derivation source exits 1"
```

- [ ] **Step 2:** run → FAIL. **Step 3: implement:**

```bash
derive_github_user() {
    local u
    if command -v gh >/dev/null 2>&1; then
        u=$(gh api user --jq .login 2>/dev/null || true)
        [ -n "$u" ] && { echo "$u"; return; }
    fi
    u=$(git remote get-url origin 2>/dev/null \
        | sed -nE 's#^(git@[^:]+:|https?://[^/]+/)([^/]+)/.*#\2#p' || true)
    [ -n "$u" ] && { echo "$u"; return; }
    u=$(git config github.user 2>/dev/null || true)
    [ -n "$u" ] && { echo "$u"; return; }
    echo "sshd-setup: cannot derive GitHub user (need gh auth, an origin remote, or git config github.user)" >&2
    return 1
}
```

Add `_github_user) derive_github_user ;;` to the dispatch `case` (test hook, same as `_detect`).

- [ ] **Step 4:** run → PASS. **Step 5:** commit `feat(sshd-setup): GitHub account derivation with fallbacks`.

### Task 4: key seeding (refuse-empty, idempotent) + `cmd_keys`

**Consumes:** `derive_github_user`. **Produces:** `seed_keys`, `cmd_keys`.

- [ ] **Step 1: failing tests** — fixture file via `SSHD_SETUP_KEYS_URL` + mocked `curl`:

```bash
# curl mock serves the file named in SSHD_SETUP_KEYS_URL
cat > "${TMPDIR_TEST}/mocks/curl" <<'EOF'
#!/usr/bin/env bash
for a in "$@"; do :; done   # last arg = URL = fixture path
cat "$a"
EOF
chmod +x "${TMPDIR_TEST}/mocks/curl"
printf 'ssh-ed25519 AAAA-test-key-1\nssh-ed25519 AAAA-test-key-2\n' > "${TMPDIR_TEST}/keys_fixture"
FAKE_HOME="${TMPDIR_TEST}/home"; mkdir -p "$FAKE_HOME"
cat > "${TMPDIR_TEST}/mocks/gh" <<'EOF'
#!/usr/bin/env bash
echo "fixture-user"
EOF
chmod +x "${TMPDIR_TEST}/mocks/gh"

PATH="${TMPDIR_TEST}/mocks:$PATH" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "first keys run adds 2 lines"
# idempotent: run again, still 2
PATH="${TMPDIR_TEST}/mocks:$PATH" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "second keys run adds nothing"
# refuse-empty: empty fixture -> exit 1, file untouched
: > "${TMPDIR_TEST}/keys_fixture"
set +e
PATH="${TMPDIR_TEST}/mocks:$PATH" HOME="$FAKE_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture" bash "$SSHD_SETUP" keys >/dev/null 2>&1; RC=$?
set -e
assert_eq "$RC" "1" "empty keys response exits 1"
assert_eq "$(wc -l < "${FAKE_HOME}/.ssh/authorized_keys" | tr -d ' ')" "2" "authorized_keys untouched on empty response"
```

- [ ] **Step 2:** run → FAIL. **Step 3: implement:**

```bash
seed_keys() {
    local user url keys line added=0
    user=$(derive_github_user)
    url="${SSHD_SETUP_KEYS_URL:-https://github.com/${user}.keys}"
    keys=$(curl -fsS "$url")
    if [ -z "$keys" ]; then
        echo "sshd-setup: no public keys returned for '$user' — refusing to continue" >&2
        return 1
    fi
    run mkdir -p "${HOME}/.ssh"
    run chmod 700 "${HOME}/.ssh"
    [ -f "${HOME}/.ssh/authorized_keys" ] || run touch "${HOME}/.ssh/authorized_keys"
    while IFS= read -r line; do
        [ -z "$line" ] && continue
        if ! grep -qxF "$line" "${HOME}/.ssh/authorized_keys"; then
            if [ "$DRY_RUN" = 1 ]; then echo "DRY: append key to authorized_keys"
            else printf '%s\n' "$line" >> "${HOME}/.ssh/authorized_keys"; fi
            added=$((added + 1))
        fi
    done <<< "$keys"
    run chmod 600 "${HOME}/.ssh/authorized_keys"
    echo "sshd-setup: seeded ${added} new key(s) from ${url} (user: ${user})"
}
cmd_keys() { seed_keys; }
```

- [ ] **Step 4:** run → PASS. **Step 5:** commit `feat(sshd-setup): idempotent GitHub key seeding with refuse-empty guard`.

### Task 5: enable — native-first install/service/firewall/sshd.env (+ dry-run) + WSL handoff

**Consumes:** everything above. **Produces:** `install_sshd`, `enable_service`,
`configure_firewall`, `write_sshd_env`, `wsl_handoff`, `cmd_enable`.

- [ ] **Step 1: failing tests:**

```bash
# native-first: sshd already running -> no pkg-install call
cat > "${TMPDIR_TEST}/mocks/systemctl" <<'EOF'
#!/usr/bin/env bash
[ "$1" = "is-active" ] && exit 0; exit 0
EOF
chmod +x "${TMPDIR_TEST}/mocks/systemctl"
cat > "${TMPDIR_TEST}/mocks/pkg-install" <<'EOF'
#!/usr/bin/env bash
echo "PKG-INSTALL-CALLED" >> "${PKG_LOG:?}"
EOF
chmod +x "${TMPDIR_TEST}/mocks/pkg-install"
printf 'ssh-ed25519 AAAA-k\n' > "${TMPDIR_TEST}/keys_fixture2"
PKG_LOG="${TMPDIR_TEST}/pkg.log"; : > "$PKG_LOG"
PATH="${TMPDIR_TEST}/mocks:$PATH" HOME="$FAKE_HOME" PKG_LOG="$PKG_LOG" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" enable >/dev/null 2>&1
assert_eq "$(cat "$PKG_LOG")" "" "enable skips pkg-install when sshd running"
assert_grep "sshd.env written with SSHD_LOGIN" "SSHD_LOGIN=true" "${FAKE_HOME}/.sshd.env"

# dry-run mutates nothing: fresh HOME stays empty
DRY_HOME="${TMPDIR_TEST}/dryhome"; mkdir -p "$DRY_HOME"
PATH="${TMPDIR_TEST}/mocks:$PATH" HOME="$DRY_HOME" \
  SSHD_SETUP_KEYS_URL="${TMPDIR_TEST}/keys_fixture2" bash "$SSHD_SETUP" --dry-run enable >/dev/null 2>&1
assert_exit_code 1 "dry-run wrote no sshd.env" test -f "${DRY_HOME}/.sshd.env"
```

- [ ] **Step 2:** run → FAIL. **Step 3: implement:**

```bash
install_sshd() {
    [ "$(detect_platform)" = macos ] && return 0
    run pkg-install openssh-server
}

enable_service() {
    if [ "$(detect_platform)" = macos ]; then
        run sudo systemsetup -setremotelogin on
    elif systemctl list-unit-files ssh.service >/dev/null 2>&1; then
        run sudo systemctl enable --now ssh
    else
        run sudo systemctl enable --now sshd
    fi
}

configure_firewall() {
    if command -v ufw >/dev/null 2>&1 && sudo ufw status 2>/dev/null | grep -qi "active"; then
        run sudo ufw allow 22/tcp
    elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
        run sudo firewall-cmd --permanent --add-service=ssh
        run sudo firewall-cmd --reload
    else
        echo "sshd-setup: no active firewall manager detected — skipping firewall step"
    fi
}

write_sshd_env() {
    if ! grep -qs "^SSHD_LOGIN=true" "${HOME}/.sshd.env" 2>/dev/null; then
        if [ "$DRY_RUN" = 1 ]; then echo "DRY: write SSHD_LOGIN=true to ~/.sshd.env"
        else echo "SSHD_LOGIN=true" >> "${HOME}/.sshd.env"; fi
    fi
}

wsl_handoff() {
    cat <<'EOF'
sshd-setup: WSL detected. The sshd above serves THIS distro. For the
Windows-native sshd (port 22, survives reboots) run in an ADMIN PowerShell:

  powershell -ExecutionPolicy Bypass -File <dotfiles>\opt\Desktop\Apps\scripts\setup-sshd.ps1

Add -WslPortProxy to also expose this WSL sshd to your LAN (default port 2222).
EOF
}

cmd_enable() {
    if sshd_running; then
        echo "sshd-setup: native sshd already running — skipping install/start"
    else
        sshd_installed || install_sshd
        enable_service
    fi
    configure_firewall
    seed_keys
    write_sshd_env
    [ "$(detect_platform)" = wsl ] && wsl_handoff
    echo "sshd-setup: done. Rollback: see docs/sshd-setup.md#rollback"
}
```

- [ ] **Step 4:** run → PASS; also `shellcheck opt/bin/sshd-setup` → clean.
- [ ] **Step 5:** commit `feat(sshd-setup): enable flow — native-first, firewall, sshd.env, WSL handoff`.

### Task 6: `setup-sshd.ps1` (Windows-native side)

**Files:** Create `opt/Desktop/Apps/scripts/setup-sshd.ps1`.

- [ ] **Step 1:** write the script (no PS test runner in repo; gate = PSScriptAnalyzer when
  available + the manual acceptance in §6):

```powershell
#Requires -RunAsAdministrator
<#
.SYNOPSIS
  On-demand Windows-native OpenSSH Server setup: capability, service,
  firewall, authorized_keys from GitHub. Companion to opt/bin/sshd-setup.
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File setup-sshd.ps1 -WslPortProxy
#>
param(
    [string]$GithubUser,
    [switch]$WslPortProxy,
    [int]$WslPort = 2222
)
$ErrorActionPreference = 'Stop'

function Get-GithubUser {
    if ($GithubUser) { return $GithubUser }
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if ($gh) { $u = (& gh api user --jq .login) 2>$null; if ($u) { return $u } }
    $origin = (& git remote get-url origin) 2>$null
    if ($origin -match '(?:[:/])([^/]+)/[^/]+?(?:\.git)?$') { return $Matches[1] }
    $cfg = (& git config github.user) 2>$null
    if ($cfg) { return $cfg }
    throw "Cannot derive GitHub user: pass -GithubUser <login>"
}

# 1. Capability (native-first: skip when present)
$cap = Get-WindowsCapability -Online -Name 'OpenSSH.Server*'
if ($cap.State -ne 'Installed') {
    Write-Host "Installing OpenSSH.Server capability..."
    Add-WindowsCapability -Online -Name $cap.Name | Out-Null
} else { Write-Host "OpenSSH.Server capability already installed." }

# 2. Service: auto-start + start
Set-Service -Name sshd -StartupType Automatic
if ((Get-Service sshd).Status -ne 'Running') { Start-Service sshd }
Write-Host "sshd service: $((Get-Service sshd).Status), startup=Automatic"

# 3. Firewall rule for 22 (idempotent)
if (-not (Get-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -ErrorAction SilentlyContinue)) {
    New-NetFirewallRule -Name 'OpenSSH-Server-In-TCP' -DisplayName 'OpenSSH Server (sshd)' `
        -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null
    Write-Host "Firewall rule created for TCP 22."
} else { Write-Host "Firewall rule for TCP 22 already present." }

# 4. Keys from GitHub (refuse-empty; seed user + administrators files)
$user = Get-GithubUser
$keys = (Invoke-RestMethod -Uri "https://github.com/$user.keys").Trim()
if (-not $keys) { throw "GitHub returned no public keys for '$user' — refusing to continue" }
$targets = @(
    (Join-Path $env:USERPROFILE '.ssh\authorized_keys'),
    (Join-Path $env:ProgramData 'ssh\administrators_authorized_keys')
)
foreach ($t in $targets) {
    $dir = Split-Path $t
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    $existing = if (Test-Path $t) { Get-Content $t } else { @() }
    $added = 0
    foreach ($k in ($keys -split "`n")) {
        $k = $k.Trim()
        if ($k -and ($existing -notcontains $k)) { Add-Content -Path $t -Value $k; $added++ }
    }
    Write-Host "Seeded $added new key(s) into $t (user: $user)"
}
# Lock down the administrators file per OpenSSH-on-Windows requirements
$admFile = Join-Path $env:ProgramData 'ssh\administrators_authorized_keys'
icacls $admFile /inheritance:r /grant 'Administrators:F' /grant 'SYSTEM:F' | Out-Null

# 5. Optional: portproxy so the WSL sshd is LAN-reachable
if ($WslPortProxy) {
    $wslIp = ((& wsl hostname -I) -split '\s+')[0]
    if (-not $wslIp) { throw "Could not determine WSL IP (is the distro running?)" }
    netsh interface portproxy delete v4tov4 listenport=$WslPort listenaddress=0.0.0.0 2>$null | Out-Null
    netsh interface portproxy add v4tov4 listenport=$WslPort listenaddress=0.0.0.0 `
        connectport=22 connectaddress=$wslIp | Out-Null
    if (-not (Get-NetFirewallRule -Name "WSL-SSH-$WslPort" -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -Name "WSL-SSH-$WslPort" -DisplayName "WSL sshd ($WslPort)" `
            -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort $WslPort | Out-Null
    }
    Write-Host "Portproxy: host :$WslPort -> WSL ${wslIp}:22 (rerun after WSL IP changes)"
}
Write-Host "Done. Rollback: docs/sshd-setup.md#rollback"
```

- [ ] **Step 2:** if `pwsh` available: `pwsh -NoProfile -Command "Invoke-ScriptAnalyzer -Path opt/Desktop/Apps/scripts/setup-sshd.ps1"` (else note: validated in §6 acceptance).
- [ ] **Step 3:** commit `feat(sshd-setup): Windows-native setup-sshd.ps1 with WSL portproxy option`.

### Task 7: docs

**Files:** Create `docs/sshd-setup.md`; Modify `docs/mbo/index.md` (state → building/in-review).

- [ ] **Step 1:** write `docs/sshd-setup.md` with: what it does (per OS table) · quickstart per
  OS (`sshd-setup enable` / admin `setup-sshd.ps1 [-WslPortProxy]`) · the Windows+WSL
  "both in one go" walkthrough · key-derivation precedence · `~/.sshd.env` integration note
  ("on-demand only; nothing at login — `opt/profiles/.bashrc:189` hook stays disabled") ·
  **Rollback** section (`Remove-WindowsCapability`, `Stop-Service`+`Set-Service -StartupType Disabled`,
  `Remove-NetFirewallRule`, `netsh interface portproxy delete`, `sudo apt-get remove openssh-server`,
  `sudo ufw delete allow 22/tcp`, `rm ~/.sshd.env`, hand-edit `authorized_keys`).
- [ ] **Step 2:** `make lint-markdown` → clean. **Step 3:** commit `docs(sshd-setup): per-OS usage + rollback`.

### Task 8: full gates

- [ ] `bash opt/bin/sshd-setup_test.sh` → all PASS · `make shell-test` picks the driver up ·
  `make lint-shell` clean. Commit anything outstanding.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1 wsl/linux detect | Task 1 `_detect` fixture tests |
| F2 native-first | Task 5 "enable skips pkg-install when sshd running" |
| F3 install-if-missing | Task 5 mocked `pkg-install` (call-count) |
| F4 service enable+start | Task 5 `enable_service` (mocked systemctl) + §6 acceptance steps 3/5 |
| F5 firewall active/skip | Task 5 `configure_firewall` (ufw mock) + skip-note branch |
| F6 precedence chain | Task 3 four fallback tests |
| F7 refuse-empty / idempotent | Task 4 empty-fixture + run-twice tests |
| F8 sshd.env | Task 5 `assert_grep SSHD_LOGIN=true` |
| F9 dry-run/status read-only | Task 2 status exit-0; Task 5 dry-run empty-HOME test |
| F10 Windows parity | Task 6 script + §6 manual acceptance (no CI runner) |

## 6. Integration & rollout

No wiring needed: `make shell-test` discovers `opt/bin/sshd-setup_test.sh`; `make lint-shell`
covers the new script. Docs linked from `docs/sshd-setup.md`.

**Manual acceptance on the motivating Windows+WSL host (the validation we designed):**

1. WSL: `opt/bin/sshd-setup status` → reports "not installed" (pre-state, read-only).
2. WSL: `sshd-setup --dry-run enable` → prints DRY lines; `~/.sshd.env` and apt state unchanged.
3. WSL: `sshd-setup enable` (sudo password at the prompt) → apt install, `ssh` service active,
   keys seeded from the derived account's `github.com/<login>.keys`, `~/.sshd.env` written.
4. WSL: `ssh -o BatchMode=yes localhost true` → exit 0 (key auth, real login).
5. Windows (admin PS): `setup-sshd.ps1 -WslPortProxy` → capability Installed, sshd Running/
   Automatic, firewall rules for 22 + 2222, `administrators_authorized_keys` seeded.
6. From WSL: `ssh <winuser>@<windows-host-ip> true` (port 22 → Windows) and, from another LAN
   machine, `ssh -p 2222 <user>@<windows-host-ip> true` (→ WSL) both succeed with key auth.
7. Reboot Windows; repeat step 6 for port 22 (survives); note the WSL side requires the distro
   to be running (documented limitation, out of scope).

### 6.1 Build leaves / DAG

Not broken out — single worker/PR build (5 files, sequential TDD; a parallel split would create
false leaves over one script file).

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
