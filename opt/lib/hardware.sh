#!/bin/bash

# hardware.sh - Hardware detection for dotfiles

is_jetson() {
  if [ -f /proc/device-tree/model ] && grep -q "NVIDIA Jetson" /proc/device-tree/model; then
    return 0
  fi
  if [ -f /etc/nv_tegra_release ]; then
    return 0
  fi
  return 1
}

is_arm64() {
  if [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ]; then
    return 0
  fi
  return 1
}

is_linux() {
  [ "$(uname -s)" = "Linux" ]
}

is_darwin() {
  [ "$(uname -s)" = "Darwin" ]
}

# --- NVIDIA DGX / discrete-GPU detection -----------------------------------
#
# These deliberately do NOT overlap with is_jetson(). A Jetson runs L4T
# (tegrastats, /etc/nv_tegra_release, jtop); a DGX Spark runs DGX OS on
# stock Ubuntu with the standard NVML driver stack. Tooling that assumes
# one will not work on the other — jetson-stats is the canonical example:
# it hard-depends on tegrastats and is unusable on a Spark.

# True on any NVIDIA DGX system (Spark, Station, or a DGX server).
is_dgx() {
  [ -f /etc/dgx-release ]
}

# True specifically on a DGX Spark (GB10). Checks the DGX release table
# first, then falls back to DMI for a system where /etc/dgx-release is
# absent or trimmed.
is_dgx_spark() {
  if [ -f /etc/dgx-release ] && grep -qi 'DGX_NAME="\?DGX Spark' /etc/dgx-release 2>/dev/null; then
    return 0
  fi
  if [ -r /sys/class/dmi/id/product_family ] &&
     grep -qi 'DGX Spark' /sys/class/dmi/id/product_family 2>/dev/null; then
    return 0
  fi
  return 1
}

# True when a working NVML-managed NVIDIA GPU is present. `nvidia-smi` on
# PATH is not sufficient evidence — the binary ships with driver packages
# and still exits non-zero when no device is bound, so actually run it.
has_nvidia_gpu() {
  command -v nvidia-smi >/dev/null 2>&1 || return 1
  nvidia-smi -L >/dev/null 2>&1
}

# True when the GPU shares system memory with the CPU rather than owning a
# discrete framebuffer (GB10 / Grace-Blackwell coherent memory, reported by
# NVML as "Addressing Mode: ATS").
#
# This is the single most important fact for GPU monitoring on a Spark:
# every AGGREGATE GPU-memory gauge reads N/A, because there is no separate
# framebuffer to measure. Per-process GPU memory still works. Tools whose
# headline feature is a VRAM bar (nvtop, nvitop, most Grafana GPU panels)
# render a blank or misleading panel here — track memory with free/btop
# against system RAM instead.
has_unified_gpu_memory() {
  has_nvidia_gpu || return 1
  nvidia-smi -q 2>/dev/null | grep -qi 'Addressing Mode.*: *ATS'
}
