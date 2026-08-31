# DGX Spark support in dotfiles — design

- **Slug:** `spark-gpu-support`
- **Date:** 2026-08-31
- **Status:** Draft
- **Relates to:** issue TBD / PR TBD (branch `design/fleet-config-pull`)
- **Author(s):** Edward Raigosa

## 1. Problem / context

The lab DGX Spark (`<host>`) is an **NVIDIA DGX Spark** (GB10 Grace-Blackwell) running DGX OS 7.2.3
on Ubuntu 24.04, kernel `6.17.0-1031-nvidia`, driver 580.173.02 / CUDA 13.0. The repo
had **no support for it at all**, and three defects made the environment quietly worse
than a stock install. All were verified on the machine, not inferred.

### 1.1 `jtop` does not and cannot apply

`jtop` (`jetson-stats`) is an **L4T** tool: it hard-depends on `tegrastats`,
`/etc/nv_tegra_release`, and the `nvpmodel`/`jetson_clocks` surface. A Spark has none
of them — it is stock Ubuntu with the standard NVML stack. `install.sh` gating
`setup_jtop.sh` behind `is_jetson()` was therefore *correct*; the gap was that no
branch existed for the non-Jetson GPU case.

| | Jetson (Orin) | DGX Spark |
| :-- | :-- | :-- |
| OS | JetPack / L4T | DGX OS on Ubuntu 24.04 |
| Release marker | `/etc/nv_tegra_release` | `/etc/dgx-release` |
| Telemetry | `tegrastats` | NVML (`nvidia-smi`) |
| Monitor | `jtop` | *(nothing, before this work)* |

### 1.2 `~/.profile` clobbered PATH — CUDA toolchain unreachable

`opt/profiles/.profile` did `. /etc/environment`. That file is a **pam_env(8)
key=value table, not a shell script**; sourcing it executed its `PATH="..."` line as
an assignment, discarding everything `/etc/profile.d/*.sh` had just set up —
including NVIDIA's `nv_paths.sh`, which is what puts `/usr/local/cuda/bin` on PATH.

Bisected:

```text
after /etc/profile only:              CUDA present
after /etc/profile then ~/.profile:   CUDA MISSING
```

Effect: `nvcc`, `ncu`, `cuda-gdb`, and `compute-sanitizer` were absent from **every
login shell**. `nsys` worked only by luck — it is independently symlinked into
`/usr/local/bin` via `/etc/alternatives`. `~/.profile` is a symlink into this repo,
so this was ours. It affects any host where a vendor ships a `profile.d` PATH
drop-in, not just Spark.

### 1.3 zsh login shells never ran `/etc/profile.d` at all

Ubuntu 24.04's `/etc/zsh/zprofile` on this box is **comment-only** — it never sources
`/etc/profile`. So for zsh (the repo's default shell) *no* `profile.d` drop-in ever
ran. Even with §1.2 fixed, bash had CUDA on PATH and zsh did not: a silent,
shell-dependent environment split.

### 1.4 Two incidental findings

- The login PATH carried **20 duplicated entries** (re-entrant logins compound
  prepends). Not fatal, but it slows every lookup and makes ordering bugs unreadable.
- `make shell-test` discovery covered `ai opt/scripts opt/bin opt/profiles` but **not
  `opt/lib`**, so `gff_test.sh` and `winsetup_test.sh` had never run in CI. Both
  still pass (20/20 each) — the gap was latent, not yet a regression.

## 2. Goals & non-goals

### Goals

1. A login shell on a Spark reaches the full CUDA toolchain, identically under bash and zsh.
2. The repo can *tell* a Spark from a Jetson, and provision each correctly.
3. GPU monitoring exists on the Spark, with the unified-memory caveat made explicit.
4. Every fix is covered by a test that runs in CI, not only on this host.

### Non-goals

- Fleet-wide GPU telemetry (DCGM + Prometheus). Deferred — see §8.
- Replacing `nvidia-smi`/NVML with anything custom.
- Touching the Jetson path. `setup_jtop.sh` is unchanged.

## 3. The unified-memory constraint (the load-bearing fact)

GB10 reports `Addressing Mode: ATS` — CPU and GPU share one 121 GiB pool. **There is
no framebuffer**, so every *aggregate* GPU-memory gauge reads N/A:

```text
nvidia-smi   FB Memory Usage -> Total: N/A  Reserved: N/A  Used: N/A
nvtop        MEM[N/A]
gpustat      ?? / ?? MB
nvitop       N/A / N/A
```

Per-process GPU memory **does** work (`Xorg 149MiB`, `gnome-shell 131MiB`, …), as do
utilization, temperature, power, and clocks.

The practical rule: **per-process attribution → `nvidia-smi`; memory pressure →
system RAM.** Any tool whose headline feature is a VRAM bar is structurally
misleading here. This is a property of the hardware, not a driver bug, and no tool
can fix it — NVML simply does not expose a unified figure.

## 4. Options considered — the monitoring tool

Evaluated on the live machine, not from documentation.

| Tool | GPU util | GPU mem | System RAM | Per-proc | Verdict |
| :-- | :-- | :-- | :-- | :-- | :-- |
| **`nvitop` 1.3.2** | ✅ | N/A | ✅ **RAM + swap** | ✅ | **Chosen** |
| `nvtop` 3.0.2 | ✅ best graph | ❌ dead bar | per-proc only | ✅ | Secondary |
| `gpustat` 1.1.1 | ✅ | `?? / ??` | ❌ | ✅ | Scripting only |
| `btop` 1.3.0 | ❌ none | ❌ | ✅ | ❌ | System half only |
| `jtop` | — | — | — | — | **Cannot run** |

**Decision: `nvitop` is the closest thing to a `jtop` equivalent.** jtop's value was
one screen showing GPU + CPU + memory + power; `nvitop` is the only candidate that
does that here, and crucially it shows **system RAM and swap** — which on unified
memory *is* the GPU memory constraint. `nvtop` keeps the better utilization history
graph, so both are installed.

Note `btop` 1.3.0 (Ubuntu 24.04's version) has **no** GPU panel; GPU support landed
in btop 1.4. It is installed for the system-memory half only.

**Honest gap:** nothing shows a *correct unified* memory figure, because NVML exposes
none. Closing that means deriving it ourselves — §8.

## 5. Decision

| Change | File | What it does |
| :-- | :-- | :-- |
| Parse, don't source | `opt/profiles/.profile` | Reads `/etc/environment` as a key=value table; **never** lets it overwrite PATH. Falls back to its PATH only when PATH is genuinely empty. |
| PATH dedupe | `opt/profiles/.profile` | First-occurrence-wins, order preserved; drops empty fields (implicit-CWD footgun). Makes re-sourcing idempotent. |
| Source `/etc/profile` | `opt/profiles/.zprofile` | zsh now follows bash's order: `/etc/profile` (+ `profile.d`) then `~/.profile`. |
| 4 new detectors | `opt/lib/hardware.sh` | `is_dgx`, `is_dgx_spark`, `has_nvidia_gpu`, `has_unified_gpu_memory`. |
| GPU provisioning | `opt/scripts/system/setup_gpu.sh` | Installs `nvitop`/`nvtop`/`gpustat`/`btop`; self-guards on Jetson and GPU-less hosts; `--dry-run`. |
| Wiring | `install.sh`, `.github/gff/features.yaml` | New `install.system.gpu` flag, sibling to `install.system.jetson`. |
| Test discovery | `Makefile` | `shell-test` now also finds `opt/lib/*_test.sh`. |

Detection uses `/etc/dgx-release` (`DGX_NAME="DGX Spark"`) with a DMI
`product_family` fallback. `is_jetson` and `is_dgx` are **mutually exclusive by
construction** and asserted as such in the tests.

## 6. Risks & blast radius

| Risk | Severity | Mitigation |
| :-- | :-- | :-- |
| `.profile` is on every login on every host — a bug here locks everyone out | **High** | POSIX-only, parses under dash/bash/zsh; 21 test cases; the parser fails *safe* (unparseable lines are skipped, never guessed) |
| Dropping `. /etc/environment` loses a var someone relied on | Low | Non-PATH vars are still exported, now more correctly (quotes stripped, a leading `export` prefix handled) |
| `.zprofile` double-sources `/etc/profile` where the system zprofile already did | Low | Idempotent; the dedupe collapses repeats. `/etc/profile` only pulls `bash.bashrc` when `$BASH` is set, so it stays inert under zsh |
| `setup_gpu.sh` runs `apt-get` on an unexpected host | Low | Guarded by `has_nvidia_gpu` (actually runs `nvidia-smi -L`), skips Jetson, gff-flaggable, `--dry-run` |
| Widening `shell-test` discovery breaks CI | None | Both newly-discovered drivers verified passing first (20/20, 20/20) |

## 7. Rollback

Each change is independent. `git revert` the commit; `~/.profile` and `~/.zprofile`
are symlinks into the repo, so a revert takes effect on next login with no
re-install. `apt-get remove nvitop nvtop gpustat btop` undoes the provisioning.
Setting `install.system.gpu=false` disables the block without a code change.

## 8. Evidence expectations

Captured during this work:

- **PATH bisect** isolating `~/.profile` as the clobber point — before/after.
- **Both shells verified**: bash and zsh login shells, 51 PATH entries each, 0
  duplicates, all five CUDA tools resolving.
- **Live detector run** on the Spark: `is_jetson` false, `is_dgx_spark` true,
  `has_unified_gpu_memory` true.
- **Tool comparison** run on GB10 hardware (the §4 table) — including pty captures
  of the two TUIs.
- **Gates green**: `lint-portability` Tier 1 = 0 / Tier 2 = 0; `lint-shell` exit 0;
  `shell-test` 35 drivers, 0 failed (53 new assertions across two new drivers).

Still to prove, and the natural start of the next objective:

- A **fresh `install.sh` run** on a Spark exercising the `install.system.gpu` block
  from a clean state (only `--dry-run` and an already-provisioned run are proven).
- Behavior on a **second, non-Spark GPU host** — the `is_dgx`-false,
  `has_nvidia_gpu`-true path is currently untested on real hardware.
- Whether a **derived unified-memory figure** (§4's honest gap) is worth building.
