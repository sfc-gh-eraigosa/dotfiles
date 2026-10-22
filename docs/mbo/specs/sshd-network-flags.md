# sshd + WSL networking as gff flags — spec

- **Slug:** sshd-network-flags
- **Date:** 2026-10-18
- **Status:** Draft
- **Relates to:** PR #409 (WSL distro resolution, portproxy firewall, `docs/sshd-setup.md`
  networking comparison) · prior objective `sshd-setup` (`./sshd-setup.md`)

## 1. Goal

A fresh machine that turns on sshd ends up reachable by fleet over the LAN. It uses the WSL
networking design that fits the machine, and the choice is recorded once as a gff flag. A
machine that already has sshd or a WSL networking design set up gets that setup **recorded**
as the same flags, never changed. After this, gff is the single place these settings live:
the `~/.sshd.env` marker is migrated away, and `install.sh` requires a working gff.

## 2. Use cases

**U1 — fresh Windows+WSL machine enables sshd (the case that matters most).**
*Actor:* operator. *Trigger:* the first `install.sh`, then `sshd-setup enable` (decided order).
*Flow:*
1. The first `install.sh` builds gff. sshd stays off, so nothing about networking is asked.
2. `sshd-setup enable` sets `install.network.sshd=true` and, on WSL, runs detection (F4).
   Enabling sshd **is** the fresh-enablement moment.
3. With nothing detected, it recommends a mode, asks, and records the answer with `gff set` (F5).
4. It prints the next step: re-run `install.sh`, whose apply step (F6) makes the Windows-side
   changes.

*Accept:* the flag holds the answer. After the next `install.sh`, `ssh <host>` from another LAN
machine works for `nat`. For `bridged`/`mirrored`, it works once the operator restarts WSL.

**U2 — machine with the old `~/.sshd.env` marker (both of today's WSL machines).**
*Trigger:* `install.sh` on a machine with `SSHD_LOGIN=true` in `~/.sshd.env`.
*Flow:* migration (F3) sets `install.network.sshd=true` and deletes `~/.sshd.env`. The next run
does nothing new.
*Accept:* the flag is `true` from `user-override`, the file is gone, and sshd behavior is unchanged.

**U3 — machine with an existing WSL networking setup.**
*Trigger:* `install.sh` on a machine whose `.wslconfig` says `networkingMode=bridged` (the k3s
workstation), or that has the WSL portproxy or the `RefreshWslPortProxy` task (the laptop).
*Flow:* detection (F4) records `bridged` or `nat` with `gff set`. Nothing is changed and nothing
is asked.
*Accept:* the flag matches what is running. There is no prompt and no Windows-side change. A
recorded mode is re-applied idempotently and is never followed by cleanup.

**U4 — operator overrides.** An explicit `gff set` (any value, including `off` / `false`)
always wins. Migration and detection never touch a key this machine has set, and install
never asks about it again.

**U5 — unattended fleet update.** `fleet update` runs `install.sh` over `ssh -t`. A TTY exists,
but nobody is watching it.
- `sdk/fleet` sends `WSLNET_ANSWER` the way it already sends `WINSETUP_ANSWER`. The default is
  `skip`: ask nothing, record nothing, and print the `gff set` command.
- The operator can pre-answer a mode in `fleet tui`. The answer goes to the hosts of one wave;
  it is never saved to `answers.json` or carried to a later wave, because the mode is a per-host choice.
- Valid values are `skip|off|nat|bridged|mirrored`. Anything else falls through to the normal path.
- No TTY and no answer behaves as `skip`.

## 3. Architecture

| Component | Where | Does |
| :-- | :-- | :-- |
| Flag definitions | `.github/gff/features.yaml` | `install.network.sshd` becomes the sshd switch (bool, default **false**). New `install.windows.wsl-network` (choice, SINGLE: `off`* / `nat` / `bridged` / `mirrored`). Remove the dead `install.windows.portproxy`. |
| gff hard requirement (F1) | `install.sh` gff bootstrap (~line 672) | require a **working** gff (see F1) |
| "set by this machine?" helper | `opt/lib/gff.sh` | `gff_user_set <key>`: 0 when the winning layer in `gff list` is `user-override` or `system-override` |
| sshd migration (F3) | new `opt/lib/sshdflags.sh`, called by `install.sh` right after the gff bootstrap (gff is guaranteed there) | U2 |
| sshd service start (F2) | `install.sh` sshd block + `opt/scripts/network/sshd_run.sh` | the gate becomes `gff_opt_in install.network.sshd`; `sshd_run.sh` no longer reads `~/.sshd.env` |
| sshd-setup | `opt/bin/sshd-setup` | `enable` runs `gff set install.network.sshd true` and, on WSL, `wslnet` detect → recommend → ask (U1). With no usable gff it errors: "run install.sh first". `status` prints the flag instead of the file. |
| WSL network detect / recommend / ask (F4, F5) | new `opt/lib/wslnet.sh` | called by `sshd-setup enable`, and by `install.sh` right after the sshd migration (both after gff exists) |
| WSL network apply (F6) | new `opt/Desktop/Apps/scripts/set-wsl-network.ps1`, run by a new deferred step `install_windows.sh --wsl-network` | its own call site in `install.sh`, independent of the Windows-customization `y` answer and of `install.windows.desktop-deploy`. `-Status` is read-only. |
| fleet pre-answer | `sdk/fleet` (`cmd/tui_cmds.go` answer set + the update executor) | `WSLNET_ANSWER`, default `skip` (U5) |

Data flow for apply: gff (WSL user layer) → `install_windows.sh --wsl-network` reads the mode
→ `set-wsl-network.ps1 -Status` (not elevated) decides whether anything must change → only then
one elevated `set-wsl-network.ps1 -Mode <id>`. That elevation is its own UAC prompt, like
`run_security_hardening_setup`; the mode travels as `-Mode`. When the Desktop copy is absent
(desktop deploy off), the step first copies `set-wsl-network.ps1`, `refresh-wsl-portproxy.ps1`
and `lib\` to `%LOCALAPPDATA%\dotfiles\wsl-network\`. An elevated process cannot read the
repo's `\\wsl.localhost` path (see the note in `setup-apps.ps1`).

## 4. Behavior / features

**F1 — a working gff is required.** `install.sh` builds gff.
- If `go` is missing or the build fails, but an existing `~/opt/bin/gff` runs (`gff version`),
  it warns and uses that binary. Offline hosts and hosts without Go keep updating.
- With no usable gff, it stops (`exit 1`) with an error naming the cause.
- The old "all steps run" fallback is gone.
- Steps before the bootstrap keep reading flags from the early `gff export`, as today.

**F2 — sshd is a flag.** `install.network.sshd` (default `false`) is the sshd switch.
`install.sh` starts sshd only when it is `true`. The key joins `_IP_CONFIG_FLAGS`: it starts a
service on each run, which is not an install dependency.

**F3 — sshd migration (once).** If `~/.sshd.env` exists:
- It has `SSHD_LOGIN=true` and the key is **not** set by this machine → `gff set
  install.network.sshd true`, re-export `GFF_INSTALL_NETWORK_SSHD=true` for this run, then
  delete the file.
- The key is already set by this machine → delete the file. The explicit setting wins, and the
  marker is stale.
- The file has no `SSHD_LOGIN=true` → delete it. sshd was never on.
- `gff set` fails → keep the file, warn, and continue. The next run retries, so the migration
  is never half-done.

Print one line saying what was migrated.

**F4 — WSL network detection (record, never change).** Runs only on WSL, and only when
`install.windows.wsl-network` is not set by this machine. First match wins:
1. `%USERPROFILE%\.wslconfig` `[wsl2]` `networkingMode=bridged` → `bridged`; `=mirrored` → `mirrored`.
2. A portproxy v4tov4 rule whose connect port is 22, or a `RefreshWslPortProxy` scheduled task → `nat`.
3. Nothing matches → not detected (go to F5).

A detected mode is recorded with `gff set`, and one line is printed. When Windows interop is
unavailable, only the file check runs, against the one `/mnt/c/Users/*/.wslconfig` if exactly
one exists. Otherwise detection is skipped.

**F5 — recommend + ask (fresh enablement only).** Runs only when all of these hold:
- sshd is on (after F3 in `install.sh`, or just turned on by `sshd-setup enable`);
- the key is not set by this machine;
- F4 detected nothing.

Recommendation:
- k3s in this distro (a `k3s` or `k3s-agent` unit enabled, or `/usr/local/bin/k3s`) **and** an
  Up wired (non-wireless) Windows adapter → `bridged`;
- otherwise → `nat`.
- `mirrored` is offered but never recommended.

The prompt lists the four modes with the recommendation marked, and records the answer, `off`
included, so the machine is never asked again. Answer sources, in order: `WSLNET_ANSWER`
(`skip` means ask nothing), then `/dev/tty`. With no TTY, record nothing and print
`gff set install.windows.wsl-network <mode>`. An answer that isn't one of the modes re-asks
once, then counts as `skip`.

In `install.sh` this prompt sits right after the gff bootstrap. It isn't front-loaded, but it
only fires in that rare state, and fleet pre-answers it (U5).

**F6 — apply (deferred step at the end of the run; elevates only when needed).** Runs when the
mode is not `off`, on WSL with Windows interop. `set-wsl-network.ps1 -Mode <m>`:
- `off`: hands off. It changes nothing and cleans up nothing.
- `nat`: run `refresh-wsl-portproxy.ps1 -ListenPort <p> -RegisterTask`, which sets up the
  forward, the `LocalSubnet` firewall rule, and the task; the distro comes from `lib\wsl.ps1`.
  `<p>` is 22, or 2222 when a Windows `sshd` service exists.
- `bridged`:
  - If `.wslconfig` already has `networkingMode=bridged`, it is current. Keep its `vmSwitch`,
    `macAddress` and every other key exactly as they are; this protects the k3s workstation's
    DHCP address.
  - Otherwise, if a Hyper-V **External** switch named `WSLBridge` exists, write
    `networkingMode=bridged`, `vmSwitch=WSLBridge`, `dhcp=true`, and a stable locally
    administered `macAddress` derived from the computer name (`02:` + 5 bytes of
    SHA-256(COMPUTERNAME)).
  - Otherwise, print `New-VMSwitch -Name WSLBridge -NetAdapterName <wired adapter>
    -AllowManagementOS $true`, with the adapter filled in when exactly one Up wired adapter
    exists, and change nothing. **It never creates the switch.**
- `mirrored`: write `networkingMode=mirrored`, and ensure a Hyper-V firewall rule allowing
  inbound TCP 22 to WSL (`New-NetFirewallHyperVRule`, VMCreatorId
  `{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}`).
- **Cleanup runs only when this run changed the mode** in `.wslconfig`, i.e. a real switch the
  operator chose. A mode recorded by detection (F4) is never followed by cleanup.
  - Switching to `bridged` or `mirrored` removes `nat`'s leftovers: the `RefreshWslPortProxy`
    task, the portproxy rule to WSL, and the `WSL-SSH-<port>` firewall rule.
  - Switching to `nat` removes the Hyper-V rule and the `networkingMode`/`vmSwitch`/`macAddress`
    keys.
- `.wslconfig` edits:
  - Touch only the networking keys in `[wsl2]`; keep every other key and comment (e.g.
    `memory=192GB`).
  - Write a timestamped `.wslconfig.bak-<ts>` first. No-op when already current.
  - **Never run `wsl --shutdown`.** When the file changed, print "restart WSL to apply:
    `wsl --shutdown`".
- `-Status`: print the recorded mode, the `.wslconfig` networking keys, the portproxy, the
  task, and the firewall rules. It changes nothing and doesn't elevate; its exit code tells the
  caller whether `-Mode` would change anything.

## 5. Evaluation criteria (per feature)

| F | Fires when | Must not fire when | Edge | Pass |
| :-- | :-- | :-- | :-- | :-- |
| F1 | no usable gff | gff builds, or an existing binary runs | no `go` + existing binary; Docker `--phase deps` / `config` | exit 1 + message only with no usable gff; CI Build job still green |
| F2 | flag `true` | flag `false` / unset | Docker phases | `sshd_run.sh` runs only when on |
| F3 | `~/.sshd.env` present | file absent | key already set; no `SSHD_LOGIN`; `gff set` fails | flag + file state per F3; idempotent second run |
| F4 | sshd on, key not set, on WSL | sshd off (no probe); key set; not WSL | bridged + portproxy both present (the file wins); no interop | correct mode recorded and marked found; the deferred step reports a found mode and never changes it |
| F5 | sshd on, key not set, nothing detected | sshd off; key set; F4 matched | no TTY; `WSLNET_ANSWER` = `skip` / a mode / junk | recommendation per F5; answer recorded or guidance printed; a prompt `skip` is remembered (install.sh stops asking, `sshd-setup enable` asks) |
| F6 | deferred step, mode not `off` | `off`; already current (no UAC prompt) | bridged without a switch; an existing bridged config; no `.wslconfig`; Desktop copy absent | changes per F6; other keys preserved; no shutdown; no cleanup after a recorded mode |

## 6. Verification harness

- **Shell unit tests** (CI-safe, POSIX):
  - `opt/lib/sshdflags_test.sh` and `opt/lib/wslnet_test.sh`, with seams like `winsetup.sh`'s:
    a `*_GFF` stub, a fake `HOME`, a fake `.wslconfig`, stubbed `netsh.exe` / `powershell.exe`,
    and `WSLNET_ANSWER`. Cover every row of §5 F3–F5.
  - `opt/lib/gff_test.sh` gains `gff_user_set` cases (repo-live vs user-override).
- **`install.sh` F1:** run the bootstrap block with no `go`, a failing build stub, and with/without
  a working existing binary. Assert exit 1 + message only in the no-usable-gff case. The Docker
  Build CI job proves the happy path.
- **`sshd-setup_test.sh`:** updated for the flag (gff stub seam), replacing the `~/.sshd.env`
  assertions. `enable` without gff errors, and `enable` on WSL calls `wslnet`.
- **`install_windows.sh --wsl-network`:** a stubbed run proves no elevation when `-Status` says
  current, one elevated call otherwise, and the `%LOCALAPPDATA%` copy when the Desktop copy is absent.
- **fleet:** `sdk/fleet` unit tests for `WSLNET_ANSWER` in the pre-answer set (default `skip`,
  plus a tui override), keeping the module's ≥60 % coverage gate.
- **PowerShell:**
  - Static checks for `set-wsl-network.ps1`: no `wsl --shutdown`, a backup before every write,
    and no `New-VMSwitch` run as a command.
  - A live `-Status` run when `powershell.exe` exists.
  - Live `.wslconfig` edits against a temp file, through a `-WslConfigPath` test parameter:
    current-bridged is preserved, other keys survive, and the change-only cleanup holds.
- **Human-evidenced gates:**
  - **H1, laptop:** `install.sh` records `nat` + `sshd=true`, deletes `~/.sshd.env`, asks
    nothing, and raises no UAC prompt (already current). `ssh <host>` from another LAN machine
    still works.
  - **H2, k3s workstation:** detection records `bridged` and changes nothing; its `vmSwitch` and
    `macAddress` are untouched.
  - **H3:** a `fleet update` wave reaches both and stalls on nothing.
- **Lint:** `shellcheck`, `make lint-portability`, `make lint-shell`, and gff lint of `features.yaml`.

## 7. Prerequisites / dependencies

- PR #409's `lib\wsl.ps1` distro resolution and the `refresh-wsl-portproxy.ps1` firewall rule.
- gff choice flags and the `gff list` layer column (both present).
- fleet's existing pre-answer plumbing (`WINSETUP_ANSWER`).

## 8. Out of scope (and why)

- Creating the Hyper-V switch (decided: it risks dropping the machine's network).
- Windows-native sshd (`install.windows.sshd`, `setup-sshd.ps1`): the fleet uses the WSL sshd.
- Reaching sshd over a VPN: the firewall rule allows `LocalSubnet` only.
- Restarting WSL automatically: it would kill the running install.
- Recommending a change on a machine that already has a mode (decided).

## 9. Rollback

- **Code:** `git revert` the commits.
- **A machine:** `gff unset install.windows.wsl-network` / `install.network.sshd`. To undo the
  migration, `echo SSHD_LOGIN=true > ~/.sshd.env` before running a pre-change `install.sh`.
- **Windows state per mode:** the rollback blocks in `docs/sshd-setup.md`; `.wslconfig.bak-<ts>`
  restores the networking keys.

> Produced via `superpowers:brainstorming`. The matching plan goes in `../plans/sshd-network-flags.md`.
> Registered in `../index.md`.
