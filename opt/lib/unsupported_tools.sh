#!/bin/bash

# unsupported_tools.sh - List of tools to skip or alias on Jetson/ARM

# Only run this if we are on Jetson or ARM
. ~/opt/lib/hardware.sh

if is_arm64; then
  # These are x86_64 or platform-specific binaries in opt/bin
  UNSUPPORTED_TOOLS=(
    "toggle_browser.scpt"
    "enable-vmx.sh"
  )

  for tool in "${UNSUPPORTED_TOOLS[@]}"; do
    # Create an alias that warns the user
    alias "$tool"="echo 'Tool \"$tool\" is not supported on ARM64/Jetson hardware. Skipping.'"
  done
fi

if is_linux; then
  # AppleScript is definitely not working on Linux
  alias toggle_browser.scpt="echo 'AppleScript is not supported on Linux. Skipping.'"
  
  # Generic browser alias for Linux
  if command -v chromium-browser >/dev/null 2>&1; then
    alias browser='chromium-browser'
  elif command -v firefox >/dev/null 2>&1; then
    alias browser='firefox'
  fi
  
  # clip alias for Linux
  if command -v xclip >/dev/null 2>&1; then
    alias clip='xclip -selection clipboard'
  elif command -v xsel >/dev/null 2>&1; then
    alias clip='xsel --clipboard --input'
  else
    alias clip="echo 'No clipboard tool (xclip/xsel) found. Please install one.'"
  fi
fi
