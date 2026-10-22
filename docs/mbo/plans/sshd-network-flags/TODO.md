# sshd + WSL networking as gff flags — execution cursor

- **Slug:** sshd-network-flags
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../sshd-network-flags.md`](../sshd-network-flags.md) — every task/§ reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you ran
> the command and read the output. After finishing a `###` task: update `TRACKING.md`, commit
> with the plan's exact message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `DOCS` · `COMMIT` · `LEDGER` update TRACKING.md ·
> `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [ ] SETUP: `git branch --show-current` → `fix/wsl-distro-detect`
- [ ] SETUP: `gh pr view 409 --json state -q .state` → `OPEN`
- [ ] SETUP: `bash sdk/gff/build.sh && ~/opt/bin/gff version`
- [ ] SETUP: `command -v powershell.exe` (LIVE tiers) · `go version` (fleet)

---

### Task 1 — gff helpers  (plan Task 1)

- [x] RED: `gff_user_set` (5 cases), `gff_cli` cwd, `gff_require` (a)–(e) in `opt/lib/gff_test.sh`
- [x] RUN-RED: `bash opt/lib/gff_test.sh; sh opt/lib/gff_test.sh` → expect **FAIL**
- [x] GREEN: `gff_cli`, `gff_user_set`, `gff_require` in `opt/lib/gff.sh`
- [x] RUN-GREEN: both shells → expect **PASS**
- [x] VERIFY: `shellcheck opt/lib/gff.sh` · evidence → `evidence/task-1/`
- [x] COMMIT: `feat(gff): gff_cli, gff_user_set and gff_require helpers`
- [x] LEDGER + CHECKPOINT

**Done when:** both shells pass every `gff_require`/`gff_user_set` case.

### Task 2 — install.sh requires a working gff  (plan Task 2)

- [x] RED: create `opt/scripts/system/wsl-network_test.sh` STATIC: `gff_require "${BASE_DIR}" || exit 1` present; old fail-open line absent
- [x] RUN-RED: `bash opt/scripts/system/wsl-network_test.sh` → expect **FAIL**
- [x] GREEN: bootstrap → `gff_require`; export `GFF_BIN`/`GFF_REPO_DIR`; update the comment
- [x] RUN-GREEN: → expect **PASS**
- [x] VERIFY: `bash -n install.sh` · `make lint-portability` · ALLOWLIST `git status --short -- opt/scripts/system/wsl-network_test.sh`
- [x] COMMIT: `feat(install): require a working gff instead of running every step`
- [x] LEDGER + CHECKPOINT

**Done when:** STATIC passes and the portability gate is clean.

### Task 3 — sshd flag + one-time migration  (plan Task 3)

- [x] RED: `opt/lib/sshdflags_test.sh` (6 cases from the plan)
- [x] RUN-RED: `bash opt/lib/sshdflags_test.sh; sh opt/lib/sshdflags_test.sh` → expect **FAIL**
- [x] GREEN: `opt/lib/sshdflags.sh`; `features.yaml` sshd → `boolDefault: false`; `install.sh` wiring + `_IP_CONFIG_FLAGS`; trim `sshd_run.sh`
- [x] RUN-GREEN: both shells → expect **PASS**
- [x] VERIFY: `gff lint` · `gff list | grep install.network.sshd` → `false` · STATIC sshd gate in `wsl-network_test.sh` · shellcheck
- [x] COMMIT: `feat(sshd): install.network.sshd is the sshd switch; migrate ~/.sshd.env once`
- [x] LEDGER + CHECKPOINT

**Done when:** every F3 case passes in both shells; gff lint is clean.

### Task 4 — wsl-network flag + detect / recommend / ask  (plan Task 4)

- [x] RED: `opt/lib/wslnet_test.sh` (detect, wslconfig, recommend, ask, sync cases from the plan, incl. Review Focus 1, 2, 4)
- [x] RUN-RED: both shells → expect **FAIL**
- [x] GREEN: `opt/lib/wslnet.sh`; `features.yaml` add `install.windows.wsl-network`, remove `install.windows.portproxy`; `install.sh` calls `wslnet_sync`
- [x] RUN-GREEN: both shells → expect **PASS**
- [x] VERIFY: `gff lint` · `gff list | grep wsl-network` → `off` · shellcheck · portability
- [x] COMMIT: `feat(wsl): install.windows.wsl-network flag; record the existing setup, ask on fresh enablement`
- [x] LEDGER + CHECKPOINT

**Done when:** every F4/F5 case passes in both shells.

### Task 5 — sshd-setup uses the flag  (plan Task 5)

- [x] RED: `opt/bin/sshd-setup_test.sh` cases from the plan (incl. Review Focus 5); replace the `SSHD_LOGIN` asserts
- [x] RUN-RED: `bash opt/bin/sshd-setup_test.sh` → expect **FAIL**
- [x] GREEN: `record_sshd_flag`; `wslnet_sync` on WSL; the next-step line; `status` flag line
- [x] RUN-GREEN: → expect **PASS**
- [x] VERIFY: shellcheck
- [x] COMMIT: `feat(sshd-setup): enable records the gff flag and picks the WSL networking mode`
- [x] LEDGER + CHECKPOINT

**Done when:** all `sshd-setup_test.sh` cases pass.

### Task 6 — set-wsl-network.ps1  (plan Task 6)

- [x] RED: STATIC + LIVE cases in `wsl-network_test.sh` (incl. Review Focus 1, 2)
- [x] RUN-RED: `bash opt/scripts/system/wsl-network_test.sh` → expect **FAIL**
- [x] GREEN: `opt/Desktop/Apps/scripts/set-wsl-network.ps1`
- [x] RUN-GREEN: → expect **PASS**
- [x] VERIFY: PowerShell parser clean · ALLOWLIST the new ps1
- [x] COMMIT: `feat(windows): set-wsl-network.ps1 applies the recorded WSL networking mode`
- [x] LEDGER + CHECKPOINT

**Done when:** every STATIC and LIVE F6 case passes.

### Task 7 — deferred --wsl-network step  (plan Task 7)

- [x] RED: stub-powershell cases + STATIC call-site check in `wsl-network_test.sh`
- [x] RUN-RED: → expect **FAIL**
- [x] GREEN: `install_windows.sh --wsl-network` + `run_wsl_network_setup`; `install.sh` call site
- [x] RUN-GREEN: → expect **PASS**
- [x] VERIFY: shellcheck · `make lint-portability`
- [x] COMMIT: `feat(install): deferred --wsl-network step, elevating only when something changes`
- [x] LEDGER + CHECKPOINT

**Done when:** no elevation when current, exactly one elevated call otherwise.

### Task 8 — fleet WSLNET_ANSWER  (plan Task 8)

- [x] RED: fleet `cmd` tests (envPrefix, handoff env, store, keys, local preamble)
- [x] RUN-RED: `cd sdk/fleet && go test ./cmd/...` → expect **FAIL**
- [x] GREEN: the five files listed in the plan
- [x] RUN-GREEN: `go test ./...` → expect **PASS**
- [x] VERIFY: `./scripts/test.sh` coverage floor · `gofmt -l .` empty · `make lint-go`
- [x] COMMIT: `feat(fleet): pre-answer install.sh's WSL networking prompt (WSLNET_ANSWER, default skip)`
- [x] LEDGER + CHECKPOINT

**Done when:** fleet tests pass and the coverage floor holds.

### Task 9 — docs, registration, end-to-end  (plan Task 9)

- [x] DOCS: `docs/sshd-setup.md`, `docs/install-windows.md`, the two AGENTS.md files, `sdk/fleet/README.md`
- [x] DOCS: `docs/mbo/index.md` row → `in-review`
- [x] VERIFY: `make shell-test lint-shell lint-portability lint-markdown` (automated; `./scripts/test.sh` not run here)
- [ ] VERIFY (human H1): laptop `install.sh` transcript → `evidence/e2e/h1.txt` (redacted)
- [ ] VERIFY (human H2): k3s workstation `install.sh`, `.wslconfig` sha256 before/after
- [ ] VERIFY (human H3): `fleet update` wave, no stalled host
- [ ] COMMIT: `docs(sshd): flags replace ~/.sshd.env; WSL networking modes; register sshd-network-flags`
- [ ] LEDGER + CHECKPOINT (refresh the PR #409 description)

**Done when:** TRACKING §3 is fully ticked.
