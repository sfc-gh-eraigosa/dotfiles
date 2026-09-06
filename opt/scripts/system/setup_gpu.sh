#!/usr/bin/env bash
# setup_gpu.sh - Monitoring tools for NVIDIA DGX / discrete-GPU hosts.
#
# The non-Jetson sibling of setup_jtop.sh. Jetson runs L4T and gets
# jetson-stats/jtop; a DGX Spark runs DGX OS on stock Ubuntu with the
# standard NVML stack, where jtop cannot work at all (it hard-depends on
# tegrastats and /etc/nv_tegra_release, neither of which exists there).
#
# UNIFIED MEMORY IS THE HEADLINE CAVEAT. On GB10 the GPU shares system
# RAM (NVML "Addressing Mode: ATS"), so there is no framebuffer and every
# AGGREGATE GPU-memory gauge reads N/A:
#
#     nvidia-smi   FB Memory Usage -> Total: N/A  Used: N/A
#     nvtop        MEM[N/A]
#     gpustat      ?? / ?? MB
#
# Per-process GPU memory still works everywhere. The real memory ceiling
# is system RAM, so this script installs tools that show BOTH.
#
# Installed (all idempotent, apt only):
#   nvitop  - closest thing to jtop here: GPU + CPU + system MEM + SWP +
#             per-process on one screen. The primary recommendation.
#   nvtop   - best GPU utilization/history graph (its MEM bar is dead on
#             GB10; use it for compute, not memory).
#   gpustat - one-line snapshot, for scripts and status lines.
#   btop    - system CPU/RAM/swap. Ubuntu 24.04 ships 1.3.0, which has NO
#             GPU panel (that landed in btop 1.4) - it is here for the
#             system-memory half of the unified-memory picture.
#
# Run: bash opt/scripts/system/setup_gpu.sh [--dry-run]
set -eu

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
BASE_DIR="$(cd "${SELF_DIR}/../../.." && pwd)"
HARDWARE_LIB="${BASE_DIR}/opt/lib/hardware.sh"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

if [ ! -f "${HARDWARE_LIB}" ]; then
    echo "Error: Hardware library not found at ${HARDWARE_LIB}" >&2
    exit 1
fi
# shellcheck source=../../lib/hardware.sh
. "${HARDWARE_LIB}"

# Jetson is setup_jtop.sh's job. Bail so the two never fight over a host.
if is_jetson; then
    echo "Jetson hardware detected - jtop/jetson-stats is the right tool here."
    echo "Skipping GPU setup (see setup_jtop.sh)."
    exit 0
fi

if ! has_nvidia_gpu; then
    echo "No NVML-managed NVIDIA GPU detected. Skipping GPU monitoring setup."
    exit 0
fi

if is_dgx_spark; then
    echo "Detected NVIDIA DGX Spark."
elif is_dgx; then
    echo "Detected NVIDIA DGX system."
else
    echo "Detected an NVIDIA GPU host."
fi

if has_unified_gpu_memory; then
    echo "  Unified GPU memory (ATS): aggregate GPU-memory gauges will read N/A."
    echo "  Track memory pressure against SYSTEM RAM (nvitop / btop / free -h)."
fi

if ! command -v apt-get >/dev/null 2>&1; then
    echo "No apt-get on this host; install nvitop/nvtop/gpustat/btop manually." >&2
    exit 0
fi

# Only ask apt to do work when something is genuinely missing - keeps a
# re-run of install.sh fast and silent.
PKGS="nvitop nvtop gpustat btop"
MISSING=""
for pkg in ${PKGS}; do
    if ! dpkg -s "${pkg}" >/dev/null 2>&1; then
        MISSING="${MISSING} ${pkg}"
    fi
done

if [ -z "${MISSING# }" ]; then
    echo "All GPU monitoring tools already installed:${PKGS:+ }${PKGS}"
else
    echo "Installing:${MISSING}"
    if [ "${DRY_RUN}" -eq 1 ]; then
        echo "(--dry-run: not installing)"
    else
        # DEBIAN_FRONTEND on the sudo env is load-bearing: a debconf prompt
        # blocks forever without a tty (same trap as the Jetson block).
        # shellcheck disable=SC2086  # intentional word splitting of the package list
        sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ${MISSING} ||
            echo "WARNING: apt-get install failed; continuing." >&2
    fi
fi

echo
echo "GPU monitoring available:"
for t in nvitop nvtop gpustat btop nvidia-smi; do
    printf '  %-10s %s\n' "${t}" "$(command -v "${t}" 2>/dev/null || echo '(not installed)')"
done
echo
echo "  nvitop            interactive TUI (GPU + CPU + system RAM + swap)"
echo "  nvitop -1         one-shot snapshot"
echo "  nvidia-smi dmon   scrolling utilization/power/clocks, no TUI"
echo "  nvidia-smi pmon   per-process utilization"
