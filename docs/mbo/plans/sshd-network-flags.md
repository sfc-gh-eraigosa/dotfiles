# sshd + WSL networking as gff flags — implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking; the resumable cursor is [`sshd-network-flags/TODO.md`](./sshd-network-flags/TODO.md).

- **Slug:** sshd-network-flags
- **Date:** 2026-10-18
- **Status:** Draft
- **Relates to:** spec [`../specs/sshd-network-flags.md`](../specs/sshd-network-flags.md) · PR #409 (branch `fix/wsl-distro-detect`, sequential, no breakout)

**Goal:** sshd on/off and the WSL networking design become gff flags. Existing setups are recorded rather than changed, a fresh machine is asked once, and `install.sh` refuses to run without a working gff.

**Architecture:** Two new POSIX helper libs (`opt/lib/sshdflags.sh`, `opt/lib/wslnet.sh`) on top of a shared `gff_cli` / `gff_user_set` / `gff_require` in `opt/lib/gff.sh`. `install.sh` calls them right after the gff bootstrap; `sshd-setup enable` calls `wslnet_sync`. A new PowerShell script `set-wsl-network.ps1` applies the mode from a new deferred `install_windows.sh --wsl-network` step, elevating only when `-Status` reports a change. `sdk/fleet` pre-answers the new prompt with `WSLNET_ANSWER`.

**Tech Stack:** POSIX sh (dash-safe libs), bash (`install.sh`, `sshd-setup`), PowerShell 5.1, Go (`sdk/fleet`), gff choice flags.

**Spec:** `docs/mbo/specs/sshd-network-flags.md`. Read it with this plan; F1–F6 and U1–U5 below refer to it.

## Global Constraints

- Flag keys: `install.network.sshd` (bool, `boolDefault: false`), `install.windows.wsl-network` (choice, `CHOICE_MODE_SINGLE`, options `off` (selected), `nat`, `bridged`, `mirrored`, each `stringValue` = its id). Remove `install.windows.portproxy`.
- "Set by this machine" = winning layer `user-override` or `system-override` in `gff list`.
- Pre-answer variable `WSLNET_ANSWER`, values `skip|off|nat|bridged|mirrored`; fleet default `skip`.
- Never run `wsl --shutdown`; never create a Hyper-V switch; back up `.wslconfig` before any write.
- Firewall rule names: `WSL-SSH-<port>` (Windows firewall, existing), `WSL-SSH-Mirrored` (Hyper-V firewall). Hyper-V VMCreatorId `{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}`.
- Bridged MAC: `02:` + the first 5 bytes of SHA-256(`$env:COMPUTERNAME`), lowercase hex, colon-separated.
- Shell: `opt/lib/*.sh` must pass under both `bash` and `sh` (dash); every `.sh` change passes `shellcheck`, `make lint-portability` (0 Tier 1/2), `make lint-shell`.
- Privacy: no host names, user names, IPs or MACs of real machines in committed files (the `privacy_guard` hook enforces it).
- `sdk/fleet` keeps its coverage floor (`scripts/test.sh` `coverage_min()`); `gofmt` + `golangci-lint` clean.
- Commits stage files by explicit name, end with the session's Co-Authored-By/Claude-Session trailer, and go out through `gss push` after the user confirms (repo rule).

## Review Focus

1. **`.wslconfig` with CRLF endings or a UTF-8 BOM** (it's often edited in Notepad). Expect correct parsing and preserved line endings. Owner: Task 4 (detect) and Task 6 (edit).
2. **`networkingMode` under `[experimental]`** (older WSL put it there). Detection must see it; the edit must move it into `[wsl2]` and remove the old line. Owners: Tasks 4 and 6.
3. **A checkout whose `features.yaml` lacks the key** (old branch): `gff set` / `gff list` fail. Expect a warning and no abort; the migration file is kept. Owners: Tasks 1 and 3.
4. **A Windows profile directory whose name contains a space** (`<users-dir>/<name with space>/.wslconfig`). Expect correct quoting everywhere. Owner: Task 4.
5. **Re-running `sshd-setup enable`** on a machine whose mode is already set. Expect no prompt and no change. Owner: Task 5.

---

## 2. File inventory

| Path | Purpose | Implements |
| :-- | :-- | :-- |
| `opt/lib/gff.sh` (+`gff_test.sh`) | add `gff_cli`, `gff_user_set`, `gff_require` | F1, "set by this machine" |
| `install.sh` (~672 bootstrap, ~1059 sshd block, ~1097 deferred, `_IP_CONFIG_FLAGS`) | call `gff_require`; call `sshd_migrate` + `wslnet_sync`; `gff_opt_in` sshd gate; new `--wsl-network` call | F1, F2, F3, F4/F5, F6 |
| `opt/lib/sshdflags.sh` (+`sshdflags_test.sh`) | `sshd_migrate` | F3 |
| `opt/scripts/network/sshd_run.sh` | drop the `~/.sshd.env` read | F2 |
| `opt/lib/wslnet.sh` (+`wslnet_test.sh`) | detect / recommend / ask / sync | F4, F5 |
| `opt/bin/sshd-setup` (+`sshd-setup_test.sh`) | flag instead of file; `wslnet_sync` on WSL | U1 |
| `opt/Desktop/Apps/scripts/set-wsl-network.ps1` | apply / status / config-only | F6 |
| `opt/bin/install_windows.sh` | `--wsl-network` mode | F6 trigger |
| `opt/scripts/system/wsl-network_test.sh` | static + live checks for the ps1 and the `--wsl-network` mode | F1/F2 static, F6 |
| `.github/gff/features.yaml` | flag changes (Global Constraints) | §3 |
| `sdk/fleet/cmd/{tui_cmds,answers_store,tui_keys,tui_view,update_output}.go` (+ tests) | `WSLNET_ANSWER` | U5 |
| `docs/sshd-setup.md`, `docs/install-windows.md`, `opt/Desktop/Apps/scripts/AGENTS.md`, `opt/scripts/network/AGENTS.md`, `sdk/fleet/README.md`, `docs/mbo/index.md` | docs + registration | §6 |
| `docs/mbo/plans/sshd-network-flags/evidence/<task>/` | gate output per task | §7 |

## 3. Interface contracts

```sh
# opt/lib/gff.sh (POSIX)
gff_cli ARGS...      # ( cd "${GFF_REPO_DIR:-.}" || :; "${GFF_BIN:-gff}" ARGS... ) — exit code passed through
gff_user_set KEY     # 0 iff `gff_cli list` row for KEY has layer (4th column) user-override|system-override
gff_require BASE_DIR # 0 = usable gff at ${GFF_BIN:-$HOME/opt/bin/gff}; 1 = printed ERROR. Builds via
                     # ${GFF_BUILD_CMD:-bash "$BASE_DIR/sdk/gff/build.sh"} when `go` is on PATH; "usable" = `"$GFF_BIN" version` exits 0

# opt/lib/sshdflags.sh (POSIX; needs gff.sh)
sshd_migrate         # always returns 0; file: ${SSHD_ENV_FILE:-$HOME/.sshd.env}; may export GFF_INSTALL_NETWORK_SSHD=true

# opt/lib/wslnet.sh (POSIX; needs gff.sh)
wslnet_is_wsl        # 0 on WSL; reads ${WSLNET_PROC_VERSION:-/proc/version}
wslnet_wslconfig     # echoes the .wslconfig path or nothing (see Task 4)
wslnet_detect        # echoes bridged|mirrored|nat or nothing
wslnet_recommend     # echoes bridged|nat
wslnet_ask REC       # echoes off|nat|bridged|mirrored|skip
wslnet_sync          # orchestrates F4→F5; always returns 0
# seams: WSLNET_WSLCONFIG, WSLNET_USERS_DIR (/mnt/c/Users), WSLNET_NETSH (netsh.exe),
#        WSLNET_PS (powershell.exe), WSLNET_K3S_BIN (/usr/local/bin/k3s), WSLNET_ANSWER
```

```powershell
# set-wsl-network.ps1
param([ValidateSet('off','nat','bridged','mirrored')][string]$Mode = 'off',
      [switch]$Status, [switch]$ConfigOnly, [string]$WslConfigPath = "$env:USERPROFILE\.wslconfig")
# -Status: read-only report; exit 0 = current for -Mode, 3 = applying -Mode would change something
# -ConfigOnly: only the .wslconfig step (no portproxy/firewall/task) — test seam, no elevation needed
```

```sh
# install_windows.sh "$BASE_DIR" --wsl-network
# mode from $GFF_INSTALL_WINDOWS_WSL_NETWORK (empty/off → exit 0). -Status rc 0 → "WSL network '<m>' already current";
# rc 3 → one elevated -Mode run; script dir = Desktop copy, else copied to %LOCALAPPDATA%\dotfiles\wsl-network\
```

## 4. TDD build order

Every task: RED test → run, see it fail → GREEN → run, see it pass → gates → commit → evidence
`tee`'d to `evidence/task-N/` (dated header) → TRACKING row → checkpoint.

### Task 1: gff helpers (`gff_cli`, `gff_user_set`, `gff_require`)

**Files:** Modify `opt/lib/gff.sh`; Test `opt/lib/gff_test.sh`.
**Interfaces:** Produces the three functions in §3. Consumed by Tasks 2–5.

- [ ] **Step 1: failing tests** in `gff_test.sh` (run under bash and sh), using a stub `GFF_BIN` script that prints canned `gff list` rows and records argv:
  - `gff_user_set install.network.sshd` → 0 when the row's layer is `user-override`; 0 for `system-override`; 1 for `repo-live`; 1 when the key has no row; 1 when the stub exits 1.
  - `gff_cli version` runs from `GFF_REPO_DIR` (the stub logs `$PWD`).
  - `gff_require`: (a) build ok + binary ok → 0; (b) no `go` (PATH without it) + working binary → 0, and output contains `WARNING: no 'go' on PATH; using existing gff`; (c) build fails + working binary → 0, output contains `WARNING: gff build failed; using existing gff`; (d) build fails + no binary → 1, output contains `ERROR: install.sh requires a working gff`; (e) no go + no binary → 1, same ERROR line plus `no 'go' on PATH`.
- [ ] **Step 2:** `bash opt/lib/gff_test.sh; sh opt/lib/gff_test.sh` → FAIL (functions undefined).
- [ ] **Step 3:** implement in `gff.sh`. `gff_user_set` parses with `awk -v k="$1" '$1==k {print $4}'`. Every message goes to stderr except the WARNING lines (stdout, matching the existing `WARNING:` style).
- [ ] **Step 4:** both shells PASS; `shellcheck opt/lib/gff.sh`.
- [ ] **Step 5:** commit `feat(gff): gff_cli, gff_user_set and gff_require helpers`.

### Task 2: `install.sh` requires a working gff (F1)

**Files:** Modify `install.sh` (~lines 670–687); Test `opt/lib/gff_test.sh` (already covers `gff_require`) + a new STATIC tier in `opt/scripts/system/wsl-network_test.sh` (created here).
**Interfaces:** Consumes `gff_require`.

- [ ] **Step 1: failing static checks** in a new `opt/scripts/system/wsl-network_test.sh`, using `ai/_test_helpers.sh`: `install.sh` contains `gff_require "${BASE_DIR}" || exit 1`; it no longer contains `gff build failed; all components will run`.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** replace the `if gff_bootstrap_ok=false; …` build with `gff_require "${BASE_DIR}" || exit 1`. Then `export GFF_BIN="${HOME}/opt/bin/gff" GFF_REPO_DIR="${BASE_DIR}"`, so every later helper finds the freshly built binary before `~/opt/bin` is on PATH. Keep the `set -a` export + `gff install` registration as they are, now unconditional. Update the comment block above (fail-open → required).
- [ ] **Step 4:** test PASS; `bash -n install.sh`; `make lint-portability`.
- [ ] **Step 5:** commit `feat(install): require a working gff instead of running every step`.

### Task 3: sshd becomes a flag + one-time migration (F2, F3)

**Files:** Create `opt/lib/sshdflags.sh`, `opt/lib/sshdflags_test.sh`; Modify `.github/gff/features.yaml` (`install.network.sshd` → `boolDefault: false`, description "Enables sshd on this host (set by sshd-setup enable); install.sh starts it when true."), `install.sh` (source the lib; call `sshd_migrate` right after the bootstrap's `apply_install_phase`; sshd block → `if gff_opt_in install.network.sshd; then "${BASE_DIR}/opt/scripts/network/sshd_run.sh"; else …`; add `INSTALL_NETWORK_SSHD` to `_IP_CONFIG_FLAGS`), `opt/scripts/network/sshd_run.sh` (delete the `~/.sshd.env` / `SSHD_LOGIN` check).
**Interfaces:** Consumes `gff_cli`, `gff_user_set`. Produces `sshd_migrate`.

- [ ] **Step 1: failing tests** (`sshdflags_test.sh`, bash + sh, a stub gff that records `set` calls and answers `list`), covering spec F3:
  - `SSHD_LOGIN=true`, key not set → stub got `set install.network.sshd true`; file gone; `GFF_INSTALL_NETWORK_SSHD=true` exported; output `Migrated ~/.sshd.env to gff: install.network.sshd=true (file removed).`
  - Key already `user-override` → no `set`; file gone; output `Removed stale ~/.sshd.env (install.network.sshd is already set on this machine).`
  - File without `SSHD_LOGIN=true` → no `set`; file gone; output `Removed ~/.sshd.env (it did not enable sshd).`
  - Stub `set` exits 1 → file kept; output `WARNING: could not record install.network.sshd in gff; kept ~/.sshd.env (retrying next run).`; returns 0.
  - No file → no output, no gff calls.
  - Second run after a migration → no-op.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** implement `sshd_migrate`; wire into `install.sh` and `features.yaml`; trim `sshd_run.sh`.
- [ ] **Step 4:** tests PASS (both shells); `(cd "$BASE_DIR" && gff lint)` clean; `gff list | grep install.network.sshd` shows `false`; static check added to `wsl-network_test.sh`: `install.sh` gates sshd with `gff_opt_in install.network.sshd`.
- [ ] **Step 5:** commit `feat(sshd): install.network.sshd is the sshd switch; migrate ~/.sshd.env once`.

### Task 4: WSL network flag + detect / recommend / ask (F4, F5)

**Files:** Create `opt/lib/wslnet.sh`, `opt/lib/wslnet_test.sh`; Modify `.github/gff/features.yaml` (add `install.windows.wsl-network` per Global Constraints, description "How this Windows host exposes its WSL sshd to the LAN: off | nat (portproxy) | bridged | mirrored. Recorded from the existing setup, or asked once when sshd is first enabled."; remove `install.windows.portproxy`; fix the `--- windows (N) ---` count comment), `install.sh` (source the lib; call `wslnet_sync` right after `sshd_migrate`).
**Interfaces:** Consumes `gff_cli`, `gff_user_set`, `gff_opt_in`. Produces the `wslnet_*` functions in §3.

- [ ] **Step 1: failing tests** (`wslnet_test.sh`, bash + sh, fake `.wslconfig` files, stub `netsh.exe` / `powershell.exe` / gff on PATH):
  - detect: `[wsl2]` `networkingMode=bridged` → `bridged`; `=mirrored` → `mirrored`; the key under `[experimental]` → same; CRLF + BOM file → same; bridged file + a portproxy stub showing a `0.0.0.0 22 <wsl-ip> 22` row → `bridged` (the file wins); no file + portproxy rule to port 22 → `nat`; no file + `powershell.exe` stub reporting the `RefreshWslPortProxy` task → `nat`; nothing → empty.
  - `wslnet_wslconfig` with no interop (no `powershell.exe`) and exactly one `"$WSLNET_USERS_DIR"/*/.wslconfig`, inside a directory whose name has a space → that path; two candidates → empty.
  - recommend: `WSLNET_K3S_BIN` executable + PS stub listing one Up wired adapter → `bridged`; k3s + Wi-Fi only → `nat`; no k3s → `nat`.
  - ask: `WSLNET_ANSWER=nat` → `nat`; `=skip` → `skip`; `=junk` with no TTY → `skip`; no answer + no TTY → `skip`.
  - sync: not WSL → no gff calls; key user-set → no calls besides `list`; detected → `set install.windows.wsl-network bridged` + output `Recorded existing WSL networking: install.windows.wsl-network=bridged (nothing changed).`; nothing detected + sshd off → no `set`, no prompt; nothing detected + `GFF_INSTALL_NETWORK_SSHD=true` + `WSLNET_ANSWER=nat` → `set … nat`; `WSLNET_ANSWER=skip` → no `set` + output `Choose later: gff set install.windows.wsl-network <off|nat|bridged|mirrored>`.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** implement. Strip `\r` and a leading BOM before parsing; the section-aware awk reads `networkingMode` from `[wsl2]` or `[experimental]`. The prompt prints the four modes with `(recommended)` beside REC, reads `/dev/tty` like `winsetup_ask`, and re-asks once on an invalid answer. The wired-adapter probe is `Get-NetAdapter -Physical | Where-Object { $_.Status -eq 'Up' -and $_.PhysicalMediaType -notmatch '802\.11|Wireless' }`.
- [ ] **Step 4:** tests PASS (both shells); `gff lint`; `gff list | grep wsl-network` → `off`; `git grep install.windows.portproxy` → only historical docs.
- [ ] **Step 5:** commit `feat(wsl): install.windows.wsl-network flag; record the existing setup, ask on fresh enablement`.

### Task 5: `sshd-setup` uses the flag (U1)

**Files:** Modify `opt/bin/sshd-setup`, `opt/bin/sshd-setup_test.sh`.
**Interfaces:** Consumes `gff_cli`, `gff_opt_in`, `wslnet_sync`. Locates the libs from the script's own path (`$(dirname "$0")/../lib/`), mirroring `install_windows.sh`. Sets `GFF_REPO_DIR` to the repo root from that path, and `GFF_BIN` to `${GFF_BIN:-$(command -v gff || echo "$HOME/opt/bin/gff")}`; "usable" = `"$GFF_BIN" version` exits 0.

- [ ] **Step 1: failing tests**:
  - `enable` with a stub gff → stub got `set install.network.sshd true`; no `~/.sshd.env` written.
  - `enable` with no usable gff → exit 1, output contains `sshd-setup: gff not found — run install.sh first`.
  - `enable` on WSL (`SSHD_SETUP_PROC_VERSION` = microsoft) with `WSLNET_ANSWER=nat` and nothing detected → stub got `set install.windows.wsl-network nat`, and output contains `Next: re-run install.sh to apply it on Windows.`
  - A second `enable` with the mode already `user-override` → no wsl-network `set`, no prompt (Review Focus 5).
  - `status` prints `sshd flag: true|false` instead of the `sshd.env:` line.
  - `--dry-run enable` → `DRY: gff set install.network.sshd true`, no gff calls.
  - Replace the old `SSHD_LOGIN` asserts.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** replace `write_sshd_env` with `record_sshd_flag` (`run gff_cli set install.network.sshd true`, then `export GFF_INSTALL_NETWORK_SSHD=true`). On WSL, call `wslnet_sync`, then print the next-step line in place of the old Windows-sshd handoff. Keep the handoff's `setup-sshd.ps1` mention as one line for Windows-native sshd.
- [ ] **Step 4:** tests PASS; `shellcheck`.
- [ ] **Step 5:** commit `feat(sshd-setup): enable records the gff flag and picks the WSL networking mode`.

### Task 6: `set-wsl-network.ps1` (F6 apply)

**Files:** Create `opt/Desktop/Apps/scripts/set-wsl-network.ps1`; extend `opt/scripts/system/wsl-network_test.sh`.
**Interfaces:** §3 PowerShell contract. Consumes `refresh-wsl-portproxy.ps1` (`-ListenPort`, `-RegisterTask`, `-UnregisterTask`) and `lib\wsl.ps1`.

- [ ] **Step 1: failing tests**:
  - STATIC: no `wsl --shutdown` / `wsl.exe --shutdown` command; `New-VMSwitch` appears only inside a string; `Copy-Item` to `.wslconfig.bak-` precedes every `Set-Content` of the config; the MAC is built from `$env:COMPUTERNAME` with SHA256.
  - LIVE (`powershell.exe`, temp files via `-ConfigOnly -WslConfigPath`):
    - `nat` on `[wsl2]\nmemory=192GB\nnetworkingMode=mirrored\n` → the networking key is removed, `memory` is kept, a `.bak-` file exists.
    - `bridged` on a file already `networkingMode=bridged` + `vmSwitch=Other` + a `macAddress` → byte-identical, no backup, `-Status` exit 0.
    - `mirrored` on an empty file → `[wsl2]` + `networkingMode=mirrored`.
    - `[experimental] networkingMode=mirrored` + `-Mode nat` → removed from `[experimental]`.
    - CRLF input keeps CRLF.
    - `-Status -Mode nat -ConfigOnly` on a current file → exit 0; on a file needing change → exit 3.
    - `bridged -ConfigOnly` when no switch exists → no change; output contains `New-VMSwitch -Name WSLBridge`.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** implement per spec F6. Elevation is the caller's job: `-Mode` without `-Status`/`-ConfigOnly` requires admin, so check `IsInRole(Administrator)` and exit 1 with a clear message when it's missing. Cleanup runs only when `networkingMode` before ≠ after. `nat` port = 2222 if `Get-Service sshd -ErrorAction SilentlyContinue` returns one, else 22. Print `restart WSL to apply: wsl --shutdown` when the file changed.
- [ ] **Step 4:** tests PASS; the PowerShell parser reports no errors.
- [ ] **Step 5:** commit `feat(windows): set-wsl-network.ps1 applies the recorded WSL networking mode`.

### Task 7: deferred `--wsl-network` step (F6 trigger)

**Files:** Modify `opt/bin/install_windows.sh` (new `--wsl-network)` case + `run_wsl_network_setup`), `install.sh` (after the `--deferred` block: `if [ "${GFF_INSTALL_WINDOWS_WSL_NETWORK:-off}" != off ] && [ -f …/install_windows.sh ]; then bash … --wsl-network; fi`); extend `wsl-network_test.sh`.
**Interfaces:** Consumes `set-wsl-network.ps1` (§3) and `find_powershell` (`opt/lib/winpowershell.sh`).

- [ ] **Step 1: failing tests** (stub `powershell.exe` on PATH logging argv and returning a scripted rc):
  - Mode `off` or unset → no powershell call.
  - `-Status` rc 0 → exactly one call (the `-Status`), output `WSL network 'nat' already current`.
  - rc 3 → a second call containing `-Verb RunAs` and `-Mode nat`.
  - Desktop copy absent → the script dir is copied under the stubbed `%LOCALAPPDATA%\dotfiles\wsl-network\` before the call.
  - STATIC: `install.sh` calls `--wsl-network` outside the `install.windows.desktop-deploy` gate.
- [ ] **Step 2:** run → FAIL.
- [ ] **Step 3:** implement. stdin is `</dev/null` for every `powershell.exe` call (existing rule). The elevated call is `Start-Process powershell -Verb RunAs -Wait -WorkingDirectory $env:SystemRoot -ArgumentList '-NoProfile','-ExecutionPolicy','Bypass','-File',"<ps1>",'-Mode','<m>'` (local path, never UNC).
- [ ] **Step 4:** tests PASS; `shellcheck`; `make lint-portability`.
- [ ] **Step 5:** commit `feat(install): deferred --wsl-network step, elevating only when something changes`.

### Task 8: fleet pre-answers `WSLNET_ANSWER` (U5)

**Files:** Modify `sdk/fleet/cmd/tui_cmds.go` (const `envWslnetAnswer = "WSLNET_ANSWER"`; `answers.wslnet string // "" (= skip) | skip | off | nat | bridged | mirrored`; `remembered`; `envPrefix` and the handoff env at ~line 689 always emit `WSLNET_ANSWER=<wslnet or "skip">`), `answers_store.go` (persist `wslnet`), `tui_keys.go` (cycle `"", "off", "nat", "bridged", "mirrored"`), `tui_view.go` (form row `(unset = skip, record nothing)` and the strip), `update_output.go` (local preamble: the env value, else `skip`); tests beside each.
**Interfaces:** Consumes the `WSLNET_ANSWER` contract (Global Constraints).

- [ ] **Step 1: failing tests**:
  - `envPrefix()` with empty answers → `export WSLNET_ANSWER=skip; `; with `wslnet: "nat"` and `windows: "y"` → both assignments.
  - The handoff env contains `WSLNET_ANSWER=skip` by default.
  - The store round-trips `wslnet`.
  - A key press cycles through the values.
  - The local preamble defaults to `skip` and honors the env.
  - Update existing tests that expected `""` from `envPrefix`.
- [ ] **Step 2:** `cd sdk/fleet && go test ./cmd/...` → FAIL.
- [ ] **Step 3:** implement.
- [ ] **Step 4:** `go test ./...` PASS; `./scripts/test.sh` coverage floor holds; `gofmt -l` empty; `make lint-go`.
- [ ] **Step 5:** commit `feat(fleet): pre-answer install.sh's WSL networking prompt (WSLNET_ANSWER, default skip)`.

### Task 9: docs, registration, end-to-end evidence

**Files:** `docs/sshd-setup.md` (enable/status/rollback now use the flags; the networking section names `install.windows.wsl-network`; the `~/.sshd.env` integration section → migration note), `docs/install-windows.md` (the new deferred step + its UAC), `opt/Desktop/Apps/scripts/AGENTS.md` (`set-wsl-network.ps1`), `opt/scripts/network/AGENTS.md` (`sshd_run.sh` no longer reads the file), `sdk/fleet/README.md` (the answer key), `docs/mbo/index.md` (row `sshd-network-flags`: spec, plan, PR #409, state `building` → `in-review`).

- [ ] **Step 1:** `make lint-markdown`, `make shell-test`, `make lint`, `make lint-portability` → all green; tee to `evidence/task-9/`.
- [ ] **Step 2:** human gate **H1** on the laptop: `install.sh` from `~/git/dotfiles` on this branch → output shows the migration line, `Recorded existing WSL networking: …=nat`, `already current`, no UAC prompt; `ls ~/.sshd.env` fails; `ssh <host>` from another LAN machine works. Capture the transcript to `evidence/e2e/h1.txt` with names redacted.
- [ ] **Step 3:** human gate **H2** on the k3s workstation: `install.sh` → `…=bridged`, `.wslconfig` unchanged (`sha256sum` before/after).
- [ ] **Step 4:** **H3**: a `fleet update` wave across both reports no stalled host.
- [ ] **Step 5:** commit `docs(sshd): flags replace ~/.sshd.env; WSL networking modes; register sshd-network-flags`; refresh the PR #409 description.

## 5. Verification mapping

| Spec rule | Test |
| :-- | :-- |
| F1 rows | `gff_test.sh` `gff_require` (a)–(e); `wsl-network_test.sh` STATIC bootstrap check; CI Build job |
| F2 | `wsl-network_test.sh` STATIC sshd gate; `sshd_run.sh` no longer reads the file (STATIC) |
| F3 rows | `sshdflags_test.sh` (6 cases) |
| F4 rows | `wslnet_test.sh` detect + `wslnet_wslconfig` cases |
| F5 rows | `wslnet_test.sh` recommend / ask / sync cases; `sshd-setup_test.sh` WSL enable |
| F6 rows | `wsl-network_test.sh` STATIC + LIVE ps1 cases; `--wsl-network` stub cases |
| U5 | fleet `cmd` tests (Task 8) |
| H1–H3 | Task 9 evidence |

## 6. Integration & rollout

- The tests are auto-discovered (`make shell-test` finds `*_test.sh`; `scripts/test.sh` runs `sdk/fleet`).
- New paths are under `opt/**` and `docs/**`, which the `.gitignore` allowlist already opts in. Verify each with `git status --short -- <path>`.
- Rollout order on a machine: the code lands → `install.sh` migrates and records (U2/U3) → nothing on Windows changes for the two existing machines.
- No breakout: one branch, nine sequential commits on PR #409.

## 7. Validation & evidence

`docs/mbo/plans/sshd-network-flags/evidence/task-<N>/` holds each task's gate output (dated
header, append-only). `evidence/e2e/` holds H1–H3. A task without captured evidence is not done.

> Produced via `superpowers:writing-plans`. Execute with `superpowers:executing-plans` /
> `subagent-driven-development`, TDD throughout. Update `../index.md` state as it moves.
