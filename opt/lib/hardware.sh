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
