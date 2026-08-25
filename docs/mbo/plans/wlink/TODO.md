# wlink — execution cursor

- **Slug:** `wlink`
- **Playbook:** [`IMPLEMENTATION.md`](./IMPLEMENTATION.md) · **Ledger:** [`TRACKING.md`](./TRACKING.md)
- **Plan (source of truth):** [`../wlink.md`](../wlink.md) — every phase reference points there

> **How to use:** the **first unchecked box is the next action**. Tick a box only after you ran
> the command and read the output. After finishing a `###` phase: update `TRACKING.md`, commit
> with the plan's message, checkpoint.
>
> **Legend:** `SETUP` prep · `RED` write a failing test · `RUN-RED` run it, expect FAIL ·
> `GREEN` implement · `RUN-GREEN` run it, expect PASS · `VERIFY` extra gate ·
> `ALLOWLIST` `.gitignore` check · `EVIDENCE` capture into `evidence/<phase>/` · `DOCS` ·
> `COMMIT` · `LEDGER` update TRACKING.md · `CHECKPOINT` push/PR refresh.

## Preflight (once)

- [ ] `git rev-parse --abbrev-ref HEAD` → `feature/wsl-dns-lan/edward-raigosa/dns` (the build happens **in** PR #242, not after it)
- [ ] `go version` → toolchain present
- [ ] `grep -qi microsoft /proc/version` → on WSL (needed for P1 fixtures and P14)
- [ ] `fleet discover --json | head -3` → the contract P7 consumes is live
- [ ] `bash opt/scripts/system/wsl_dns_lan_test.sh | tail -2` → `PASS=54 FAIL=0` — **the oracle**; it must keep passing until P15 deletes it
- [ ] `mkdir -p docs/mbo/plans/wlink/evidence`

---

### Phase P0 — module skeleton + shared logging  (plan §4 P0)

- [ ] RED: `main_test.go` asserting `--version` prints a stamped version
- [ ] RUN-RED: `go test ./...` → expect **FAIL**
- [ ] GREEN: `go.mod`, `main.go`, `cmd/root.go`, `internal/version`, `build.sh` sourcing `sdk/version.sh`
- [ ] GREEN: wire `sdk/libs/log` — `applog.SetDefaultTool("wlink")` once in `cmd/root.go`
- [ ] RED: a test asserting `SetDefaultTool` runs before any command body
- [ ] RUN-GREEN: `go test ./...` → expect **PASS**
- [ ] VERIFY: `./build.sh && ./wlink --version` stamps a version
- [ ] VERIFY: `grep -rn "logrus\|lumberjack" sdk/wlink --include='*.go' | grep -v libs/log` → **empty** (no hand-rolled logger)
- [ ] ALLOWLIST: `git status --short -- sdk/wlink` → files listed, not ignored
- [ ] EVIDENCE → `evidence/p0/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** builds, version stamped, logging via `libs/log` only, tracked by git.

---

### Phase P1 — `winhost` + `Runner` seam  **(BLOCKING)**  (plan §4 P1)

- [ ] SETUP: capture real PowerShell output on a WSL host with Wi-Fi + WireGuard + Bluetooth into `internal/winhost/testdata/`
- [ ] RED: fixture-driven test parsing that output → `[]Interface`, incl. `IsTunnel` detection
- [ ] RUN-RED: `go test ./internal/winhost/...` → expect **FAIL**
- [ ] GREEN: `runner.go` (the interface), `powershell.go` (PATH lookup **plus** the absolute fallback — interop PATH entries can be missing on a healthy WSL), `query.go`
- [ ] RUN-GREEN: `go test ./internal/winhost/...` → expect **PASS**
- [ ] VERIFY: `go test -cover ./internal/winhost/...` → **≥60%**
- [ ] VERIFY: `grep -rn "powershell" sdk/wlink --include='*.go' | grep -v internal/winhost` → **empty** (interop stays behind the seam)
- [ ] EVIDENCE → `evidence/p1-winhost/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** real captures parse correctly; tunnel detection right; no interop outside `winhost`.

---

### Phase P2 — `linkstate.State` schema  **(BLOCKING)**  (plan §4 P2)

- [ ] RED: JSON round-trip + golden schema test
- [ ] RUN-RED: `go test ./internal/linkstate/...` → expect **FAIL**
- [ ] GREEN: `State`, `TunnelState` (`up|not-ready|down|unknown`), `PinState`, `Candidate`, `FleetSummary`, `DriftReport`
- [ ] RUN-GREEN: expect **PASS**
- [ ] DOCS: schema documented in `sdk/wlink/README.md` — it is a public contract `gsl` will consume
- [ ] EVIDENCE → `evidence/p2/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** schema frozen and documented. Downstream phases may now consume it.

---

### Phase P3 — native DNS (drops `dig`)  (plan §4 P3)

- [ ] RED: EC-8 against a local in-process DNS server — NXDOMAIN · no-response · SERVFAIL · NOERROR-no-data
- [ ] RUN-RED: `go test ./internal/probe/...` → expect **FAIL**
- [ ] GREEN: `dns.go` — query a specific server natively
- [ ] RUN-GREEN: expect **PASS**
- [ ] VERIFY: outcomes match the recorded `dig` behavior the shell script characterized
- [ ] VERIFY: `grep -rn '"dig"' sdk/wlink --include='*.go'` → **empty**
- [ ] EVIDENCE → `evidence/p3-probe/` (native vs recorded-`dig` comparison)
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** native resolver reproduces `dig` semantics; no `dig` dependency remains.

---

### Phase P4 — scoring + recursion guard  (plan §4 P4)

- [ ] RED: EC-1 — the default-gateway trap (a non-default-route interface resolves all fleet names; the gateway resolves none)
- [ ] RED: EC-2 — guard refuses a candidate that NXDOMAINs the public sentinel; `--allow-nonrecursive` overrides loudly
- [ ] RED: silent vs reachable-but-ignorant classification (a SERVFAIL is *reachable*, not silent)
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `score.go`, `guard.go`
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: ties resolve deterministically (first by enumeration order)
- [ ] EVIDENCE → `evidence/p4/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** EC-1 and EC-2 pass; classification distinguishes silent from ignorant.

---

### Phase P5 — `resolvconf` render + derived budget  (plan §4 P5)

- [ ] RED: EC-3 — golden `resolv.conf`; `wsl.conf` across **all five** INI shapes (key present / key absent / `[network]` absent / `[network]` last / empty file), other sections untouched
- [ ] RED: budget = `nameservers × timeout × 2 families + 1`
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `resolv.go`, `wslconf.go`
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: golden files byte-exact
- [ ] EVIDENCE → `evidence/p5/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** all five INI shapes correct; budget derived, not hardcoded.

---

### Phase P6 — snapshot, restore, drift  (plan §4 P6)

- [ ] RED: EC-4 — snapshot written **before** the first byte; round-trip restores `resolv.conf` (symlink target included) and `wsl.conf` byte-for-byte
- [ ] RED: snapshot-failure ⇒ **no write**, exit 0
- [ ] RED: re-running `pin` must **not** overwrite a good snapshot
- [ ] RED: EC-11 — drift detected after a hand-edit; **not** reported for a byte-identical file
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `snapshot.go` (atomic privileged writes)
- [ ] RUN-GREEN → expect **PASS**
- [ ] EVIDENCE → `evidence/p6-snapshot/` (the round-trip diff)
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** the undo path is proven byte-for-byte, and no write can happen without it.

---

### Phase P7 — `fleetsrc` + `/etc/hosts` exclusion  (plan §4 P7)

- [ ] RED: hosts from a stubbed `fleet discover --json`; ssh-config fallback when `fleet` is absent
- [ ] RED: EC-5 — a name in `/etc/hosts` is excluded **and announced**; score reflects only DNS-resolvable hosts
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `fleet.go`, `hostsfile.go`
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: no ssh-config **writes** anywhere (`fleet` owns that)
- [ ] EVIDENCE → `evidence/p7/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** one owner for `#fleet`; local hostname excluded from DNS probing.

---

### Phase P8 — `cmd/pin` + `cmd/unpin`  (plan §4 P8)

- [ ] RED: port every one of the 54 shell cases that exercises pin/unpin; cite each in plan §5
- [ ] RED: `--dry-run` writes nothing
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: wire P1–P7 per the plan §3 orchestration pseudocode
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: safe declines exit **0** (no winner · guard tripped · non-WSL · snapshot failed)
- [ ] EVIDENCE → `evidence/p8/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** all ported pin/unpin cases green; every safe decline exits 0.

---

### Phase P9 — `cmd/status` + `--json`  (plan §4 P9)

- [ ] RED: EC-9 — schema validates; exit 0 healthy / 1 degraded / 2 usage
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `status.go`
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: completes inside the status-line budget on a fixture run
- [ ] EVIDENCE → `evidence/p9/`
- [ ] COMMIT · LEDGER · CHECKPOINT

---

### Phase P10 — `cmd/verify`  (plan §4 P10)

- [ ] RED: EC-7 — pass (tunnel up) · pass (down, miss within budget) · fail (public unresolvable) · fail (miss over budget)
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `verify.go` — resolve through the **host resolver path**, never direct-to-server
- [ ] RUN-GREEN → expect **PASS**
- [ ] EVIDENCE → `evidence/p10/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** all four matrix outcomes correct.

---

### Phase P11 — `cmd/wait` + readiness  (plan §4 P11)

- [ ] RED: EC-6 — all-silent ⇒ `not-ready` (not `down`); some answer but none knows the fleet ⇒ **not** a tunnel problem; `wait --ready` 0 on ready, 1 on timeout
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `wait.go`
- [ ] RUN-GREEN → expect **PASS**
- [ ] EVIDENCE → `evidence/p11/`
- [ ] COMMIT · LEDGER · CHECKPOINT

---

### Phase P12 — `sshcfg` + `cmd/doctor`  (plan §4 P12)

- [ ] RED: EC-10 — flags `ServerAliveInterval 0` for the git host; silent when set; `--fix` idempotent and touches only its own block
- [ ] RUN-RED → expect **FAIL**
- [ ] GREEN: `keepalive.go`, `doctor.go` (+ snapshot-missing, `resolv.conf` drift, full-tunnel routing, absent fleet source)
- [ ] RUN-GREEN → expect **PASS**
- [ ] VERIFY: every finding explains itself in one line
- [ ] EVIDENCE → `evidence/p12/`
- [ ] COMMIT · LEDGER · CHECKPOINT

---

### Phase P13 — integration & rollout  (plan §4 P13, §6)

- [ ] `build.sh` sources `sdk/version.sh` ✓ · logs via `libs/log` ✓
- [ ] `sdk/wlink/AGENTS.md` + `ln -s AGENTS.md CLAUDE.md`
- [ ] `sdk/wlink/README.md` — absorbs `docs/wsl-dns.md`
- [ ] `.github/gff/features.yaml`: add `install.sdk.wlink`, **`boolDefault: false`**, description stating it is opt-in/fail-closed and why it differs from the other `install.sdk.*` flags
- [ ] `install.sh`: one block gated `gff_opt_in install.sdk.wlink` (**not** `gff_on` — an unset flag must mean *do not build*) that builds/installs the binary and runs the pin
- [ ] `install.sh`: remove the prototype's block (replaced, not migrated — neither flag reaches `main`)
- [ ] VERIFY flag behavior on a real `install.sh` run (plan §6):
  - [ ] `install.sdk.wlink=false` (default) → one SKIP line, nothing built, nothing pinned
  - [ ] `install.sdk.wlink=true` → binary built into `~/opt/bin/`, pin runs
  - [ ] `install.sdk.wlink=true` with the tunnel **down** → pin declines, exit 0, `install.sh` still succeeds
- [ ] Row in `sdk/AGENTS.md` **Modules** table
- [ ] Row in `sdk/README.md` "Pick your tool" **and** a full section in the house shape — **demo must be real captured output**
- [ ] `scripts/test.sh` coverage floor: `wlink) echo 60 ;;`
- [ ] `docs/AGENTS.md` row repointed at `sdk/wlink/README.md` (no redirect stub — `docs/wsl-dns.md` never lands on `main`)
- [ ] `opt/scripts/system/AGENTS.md` entry dropped, points at the module
- [ ] ALLOWLIST: `git status --short` for every new path
- [ ] VERIFY: `make lint-shell && make lint-portability && make shell-test`
- [ ] EVIDENCE → `evidence/p13/`
- [ ] COMMIT · LEDGER · CHECKPOINT

**Done when:** all 9 `sdk/AGENTS.md` checklist items done and exactly one implementation is live.

---

### Phase P14 — live acceptance  (plan §4 P14, §6 checklist)

- [ ] Tunnel **down** → `status` = `down`; `pin` declines, writes nothing, exit 0
- [ ] **During handshake** → `status` = `not-ready`; `wait --ready` returns 0 on completion → `evidence/e2e/handshake/`
- [ ] Tunnel **up** → `pin` selects the tunnel resolver, guard passes, snapshot written
- [ ] `verify` PASS with tunnel up → `evidence/e2e/tunnel-up/`
- [ ] `verify` PASS with tunnel down (miss within budget) → `evidence/e2e/tunnel-down/`
- [ ] `ssh <fleet-host>` connects immediately; public DNS still resolves
- [ ] `doctor` flags the missing `ServerAliveInterval`
- [ ] `unpin` restores both files byte-for-byte
- [ ] Timing ≤ baseline **20–21s → 4s** → `evidence/e2e/timing/`
- [ ] `demo/` — one capture of `status` before connect / mid-handshake / after
- [ ] LEDGER: tick every box in `TRACKING.md` §3
- [ ] `docs/mbo/index.md` state → `in-review`
- [ ] COMMIT · CHECKPOINT · promote the draft PR

**Done when:** the §3 stop condition in `TRACKING.md` is fully ticked with captured evidence.

---

### Phase P15 — retire the prototype  (plan §4 P15)

> **Gate:** do not start until plan §5's traceability table is complete, with **every one of the
> 54 shell cases citing a passing Go test**. Until then the prototype is the only proof this port
> is faithful — run it side by side when a Go result surprises you.

- [ ] VERIFY: plan §5 table complete — every EC rule and every shell case cites a passing test
- [ ] VERIFY: `go test ./...` green; coverage ≥60%
- [ ] DELETE: `opt/scripts/system/wsl_dns_lan.sh`
- [ ] DELETE: `opt/scripts/system/wsl_dns_lan_test.sh`
- [ ] DELETE: `docs/wsl-dns.md` (content now lives in `sdk/wlink/README.md`)
- [ ] DELETE: the `install.system.wsl-dns` entry in `.github/gff/features.yaml`
- [ ] VERIFY: `grep -rn 'wsl_dns_lan\|install.system.wsl-dns' . | grep -v docs/mbo/` → **empty**
- [ ] VERIFY: `make shell-test && make lint-shell && make lint-portability` still green
- [ ] VERIFY: `git diff --stat origin/main...HEAD -- opt/scripts/system/ docs/wsl-dns.md` → prototype absent from the PR's net diff
- [ ] EVIDENCE → `evidence/p15/`
- [ ] COMMIT · LEDGER · CHECKPOINT
- [ ] `docs/mbo/index.md` state → `in-review`; promote the draft PR

**Done when:** `main` will see only `wlink` — no prototype, no `docs/wsl-dns.md`, no
`install.system.wsl-dns`. Not archived: it never shipped, and git history holds it.
