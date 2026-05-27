# Nerd Font Auto-Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install and configure a canonical Nerd Font (MesloLGS NF) across macOS, Linux/WSL, and Windows so that gsl's `powerline` style and p10k render glyphs correctly — driven entirely by gsl-packaged helper scripts that `install.sh` orchestrates AFTER the gsl skill and binary are installed, and PROVE the installed font covers the exact Private-Use-Area codepoints gsl emits.

**Architecture:** The font installers ship WITH the gsl skill, alongside `src/gsl/scripts/check-deps.sh`: `install_nerd_font_macos.sh`, `install_nerd_font_linux.sh`, `install_nerd_font_windows.ps1`, and the glyph-coverage checker `check-font-glyphs.sh` (+ its Go helper `cmd/glyphcheck`). A single OS-dispatch block in `install.sh` runs them AFTER the gsl build (after L414) so both the freshly-linked skill files and the `~/opt/bin/gsl` binary exist when fonts are configured and validated. macOS uses a pinned ryanoasis/nerd-fonts release plus iTerm2 **Dynamic Profiles** (no live-plist patching). Linux/WSL installs into `~/.local/share/fonts` + `fc-cache`. On Windows the Ubuntu-Mono logic is EXTRACTED out of `setup-apps.ps1` into the gsl-packaged `install_nerd_font_windows.ps1`; `setup-apps.ps1` keeps its other phases and simply delegates to the extracted script. This makes system font configuration gsl-compatible on every OS. The gsl `SKILL.md` documents the font setup so the skill is self-describing.

**Tech Stack:** ryanoasis/nerd-fonts pinned release (`v3.4.0`), iTerm2 Dynamic Profiles, fontconfig, PowerShell GDI font activation, a Go-based glyph-coverage checker (`golang.org/x/image/font/sfnt`), install.sh OS dispatch

---

Closes #29.

## Canonical Font Choice

**Recommended: `MesloLGS NF` from the pinned ryanoasis/nerd-fonts `Meslo.zip` release asset.**

Justification:
- MesloLGS NF is the _official_ recommended font for Powerlevel10k (`p10k`) — the repo already uses p10k (`opt/profiles/.p10k.zsh`, `opt/profiles/.zshrc`), so this is the zero-friction choice.
- The Meslo archive in the pinned nerd-fonts release contains all `MesloLGS NF` faces (Regular, Bold, Italic, BoldItalic) and covers every codepoint gsl emits (see Glyph-coverage validation below).
- Pinning the release (not the Homebrew `--cask`, which only tracks latest) gives reproducible installs across all three OSes from one `NERD_FONTS_VERSION` constant — see the pinning decision (Open Question 1, resolved).
- Alternatives (`CaskaydiaCove NF`, `FiraCode NF`) would also work but require a separate p10k font config change; MesloLGS NF avoids that extra step.

### Shared constants (used verbatim by every OS script)

```sh
NERD_FONTS_VERSION="v3.4.0"                                 # pinned ryanoasis/nerd-fonts release tag
NERD_FONTS_ASSET="Meslo.zip"                                # archive containing the MesloLGS NF faces
NERD_FONTS_URL_BASE="https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}"
FONT_FAMILY="MesloLGS NF"                                   # family name the terminal config references
FONT_SIZE=13
```

PowerShell equivalents (same values) live at the top of `install_nerd_font_windows.ps1`.

---

## Requirements / Preflight

Every OS script MUST run a preflight check FIRST and fail fast (`exit 1`) with a clear, copy-pasteable remediation message when a base requirement is missing. This is in scope for THIS round (no longer deferred).

| OS | Required tools (checked via `command -v` / `Get-Command`) | Failure message |
|----|-----------------------------------------------------------|-----------------|
| macOS | `brew`, `python3`, `curl`, `unzip` | `"ERROR: Homebrew not found. Install it from https://brew.sh/ then re-run."` (per missing tool) |
| Linux/WSL | `curl`, `unzip`, `fc-cache` (fontconfig) | `"ERROR: fontconfig not found. Install it: sudo apt-get install -y fontconfig unzip curl"` |
| Windows | PowerShell 5.1+, `Invoke-WebRequest`, write access to `%LOCALAPPDATA%\Microsoft\Windows\Fonts` | `"ERROR: cannot write to user font dir; run from a normal (non-elevated) PowerShell."` |

The orchestration block in `install.sh` only dispatches the script matching `uname -s`; each script still self-guards so it is safe to run by hand.

---

## macOS Tasks (Primary Scope)

### Task 1 — Create src/gsl/scripts/install_nerd_font_macos.sh (gsl-packaged)

This script lives alongside `check-deps.sh` so it ships with the gsl skill. It (a) preflights, (b) downloads the pinned Meslo release and installs the faces to `~/Library/Fonts`, and (c) writes an iTerm2 **Dynamic Profile** plus best-effort Terminal.app config. Idempotent and safe to re-run.

**File:** `src/gsl/scripts/install_nerd_font_macos.sh`

```bash
#!/usr/bin/env bash
# install_nerd_font_macos.sh — gsl-packaged macOS Nerd Font installer.
# 1. Preflight base requirements (brew, python3, curl, unzip).
# 2. Install the pinned MesloLGS NF faces into ~/Library/Fonts.
# 3. Write an iTerm2 Dynamic Profile (loads live, survives quit) and
#    best-effort Terminal.app config.
# Safe to re-run (idempotent).
set -euo pipefail

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_ASSET="Meslo.zip"
NERD_FONTS_URL_BASE="https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}"
FONT_FAMILY="MesloLGS NF"
FONT_SIZE=13

# ── Preflight ────────────────────────────────────────────────────────────────
missing=0
for tool in brew python3 curl unzip; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "ERROR: required tool '$tool' not found."
    case "$tool" in
      brew) echo "  Install Homebrew from https://brew.sh/ then re-run." ;;
      *)    echo "  Install '$tool' (e.g. 'brew install $tool') then re-run." ;;
    esac
    missing=1
  fi
done
[ "$missing" -eq 0 ] || exit 1

# ── Install font faces from the pinned release ───────────────────────────────
font_dir="${HOME}/Library/Fonts"
mkdir -p "$font_dir"
if system_profiler SPFontsDataType 2>/dev/null | grep -qi "MesloLGS NF"; then
  echo "MesloLGS NF already present in system fonts; skipping download."
else
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "Downloading ${NERD_FONTS_ASSET} (${NERD_FONTS_VERSION})..."
  curl -fsSL -o "${tmp}/${NERD_FONTS_ASSET}" "${NERD_FONTS_URL_BASE}/${NERD_FONTS_ASSET}"
  unzip -o -q "${tmp}/${NERD_FONTS_ASSET}" 'MesloLGS NF *.ttf' -d "$tmp"
  cp "${tmp}"/MesloLGS\ NF\ *.ttf "$font_dir"/
  echo "Installed MesloLGS NF faces into ${font_dir}."
fi

# ── iTerm2 Dynamic Profile (loads live; NOT overwritten on quit) ─────────────
# Replaces the old com.googlecode.iterm2.plist patching that lost writes when
# iTerm2 was running. DynamicProfiles are read live by a running iTerm2.
dyn_dir="${HOME}/Library/Application Support/iTerm2/DynamicProfiles"
mkdir -p "$dyn_dir"
cat > "${dyn_dir}/gsl-nerd-font.json" <<JSON
{
  "Profiles": [
    {
      "Name": "gsl Nerd Font",
      "Guid": "gsl-nerd-font",
      "Normal Font": "${FONT_FAMILY} ${FONT_SIZE}",
      "Non Ascii Font": "${FONT_FAMILY} ${FONT_SIZE}",
      "Use Non-ASCII Font": true
    }
  ]
}
JSON
echo "Wrote iTerm2 Dynamic Profile: ${dyn_dir}/gsl-nerd-font.json"
echo "  In iTerm2: Preferences → Profiles → select 'gsl Nerd Font' (or set it as Default)."

# ── Terminal.app (best effort; weaker PUA fallback than iTerm2) ──────────────
# Terminal.app renders fewer Nerd Font PUA glyphs than iTerm2 (no broad PUA
# fallback). iTerm2 is the recommended terminal for full gsl rendering.
osascript <<OSASCRIPT 2>/dev/null && echo "Terminal.app default font set." || \
  echo "NOTE: Terminal.app font update skipped (Terminal not scriptable here)."
tell application "Terminal"
  set font name of settings set "Basic" to "${FONT_FAMILY}"
  set font size of settings set "Basic" to ${FONT_SIZE}
end tell
OSASCRIPT

echo ""
echo "Done. Restart iTerm2 (or open a new window/tab) to apply the new font."
echo "Verify: system_profiler SPFontsDataType | grep -i meslo"
```

- [ ] Create `src/gsl/scripts/install_nerd_font_macos.sh` with the content above
- [ ] `chmod +x src/gsl/scripts/install_nerd_font_macos.sh`
- [ ] Commit: `feat(gsl): add macOS Nerd Font installer (pinned release + iTerm2 Dynamic Profile)`

---

## Linux / WSL Tasks (In Scope)

### Task 2 — Create src/gsl/scripts/install_nerd_font_linux.sh (gsl-packaged)

Downloads the SAME pinned Meslo release, installs to `~/.local/share/fonts`, refreshes the font cache.

**File:** `src/gsl/scripts/install_nerd_font_linux.sh`

```bash
#!/usr/bin/env bash
# install_nerd_font_linux.sh — gsl-packaged Linux/WSL Nerd Font installer.
# Installs the pinned MesloLGS NF faces into ~/.local/share/fonts and runs
# fc-cache. Safe to re-run (idempotent).
set -euo pipefail

NERD_FONTS_VERSION="v3.4.0"
NERD_FONTS_ASSET="Meslo.zip"
NERD_FONTS_URL_BASE="https://github.com/ryanoasis/nerd-fonts/releases/download/${NERD_FONTS_VERSION}"

# ── Preflight ────────────────────────────────────────────────────────────────
missing=0
for tool in curl unzip fc-cache; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "ERROR: required tool '$tool' not found."
    echo "  Install it: sudo apt-get install -y fontconfig unzip curl"
    missing=1
  fi
done
[ "$missing" -eq 0 ] || exit 1

font_dir="${HOME}/.local/share/fonts/MesloLGS"
mkdir -p "$font_dir"
if fc-list 2>/dev/null | grep -qi "MesloLGS NF"; then
  echo "MesloLGS NF already installed; skipping download."
else
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  echo "Downloading ${NERD_FONTS_ASSET} (${NERD_FONTS_VERSION})..."
  curl -fsSL -o "${tmp}/${NERD_FONTS_ASSET}" "${NERD_FONTS_URL_BASE}/${NERD_FONTS_ASSET}"
  unzip -o -q "${tmp}/${NERD_FONTS_ASSET}" 'MesloLGS NF *.ttf' -d "$font_dir"
  fc-cache -f "$font_dir"
  echo "Installed MesloLGS NF into ${font_dir} and refreshed font cache."
fi

echo "Verify: fc-list | grep -i MesloLGS"
echo "WSL note: also install the font on the WINDOWS host for Windows Terminal"
echo "          to render it (see install_nerd_font_windows.ps1)."
```

- [ ] Create `src/gsl/scripts/install_nerd_font_linux.sh` with the content above
- [ ] `chmod +x src/gsl/scripts/install_nerd_font_linux.sh`
- [ ] Commit: `feat(gsl): add Linux/WSL Nerd Font installer (pinned release + fc-cache)`

---

## Windows Tasks (In Scope — extract from setup-apps.ps1)

### Task 3 — Extract the font logic out of setup-apps.ps1 into the gsl-packaged installer

`setup-apps.ps1` currently hardcodes `$TerminalFont = 'Ubuntu Mono'` (line 51), defines `Install-UbuntuMono` (lines 172–228, the `# ---- Fonts ----` section), calls it from the install flow at line 426 (`# [4/6] Font`), and consumes `$TerminalFont` in the Windows Terminal profile font object at line 294 (`font = [pscustomobject]@{ face = $TerminalFont; size = 12 }`).

The font logic moves OUT into the gsl-packaged script. `setup-apps.ps1` keeps all its other phases (WSL, Distros, GitHub link, Windows Terminal, winget apps) and simply delegates.

**New file:** `src/gsl/scripts/install_nerd_font_windows.ps1`

```powershell
<#
.SYNOPSIS
    gsl-packaged Windows Nerd Font installer. Installs the pinned MesloLGS NF
    faces from the ryanoasis/nerd-fonts release and activates them for the
    running session via GDI. Idempotent.
.OUTPUTS
    Returns the installed font family name ('MesloLGS NF') so callers (e.g.
    setup-apps.ps1) can use it for terminal profiles.
#>
[CmdletBinding()]
param()

$NerdFontsVersion = 'v3.4.0'
$NerdFontsAsset   = 'Meslo.zip'
$NerdFontsUrl     = "https://github.com/ryanoasis/nerd-fonts/releases/download/$NerdFontsVersion/$NerdFontsAsset"
$FontFamily       = 'MesloLGS NF'

# ── Preflight ────────────────────────────────────────────────────────────────
$fontDir = "$env:LOCALAPPDATA\Microsoft\Windows\Fonts"
$regKey  = 'HKCU:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts'
try { New-Item -ItemType Directory -Force -Path $fontDir -ErrorAction Stop | Out-Null }
catch {
    Write-Host "ERROR: cannot write to user font dir '$fontDir'; run from a normal (non-elevated) PowerShell." -ForegroundColor Red
    return $null
}
if (-not (Get-Command Invoke-WebRequest -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: Invoke-WebRequest unavailable; PowerShell 5.1+ required." -ForegroundColor Red
    return $null
}

Write-Host "`n=== gsl Nerd Font ($FontFamily, nerd-fonts $NerdFontsVersion) ===" -ForegroundColor Cyan

# Faces to extract from the Meslo archive (the four MesloLGS NF faces).
$faces = @(
    @{ File = 'MesloLGS NF Regular.ttf';     Reg = 'MesloLGS NF (TrueType)' }
    @{ File = 'MesloLGS NF Bold.ttf';        Reg = 'MesloLGS NF Bold (TrueType)' }
    @{ File = 'MesloLGS NF Italic.ttf';      Reg = 'MesloLGS NF Italic (TrueType)' }
    @{ File = 'MesloLGS NF Bold Italic.ttf'; Reg = 'MesloLGS NF Bold Italic (TrueType)' }
)

# Drop stale Ubuntu Mono registry entries whose files no longer exist (migration).
$existing = Get-ItemProperty $regKey -ErrorAction SilentlyContinue
if ($existing) {
    foreach ($p in $existing.PSObject.Properties) {
        if ($p.Name -match 'Ubuntu Mono' -and $p.Value -and -not (Test-Path $p.Value)) {
            Remove-ItemProperty -Path $regKey -Name $p.Name -ErrorAction SilentlyContinue
        }
    }
}

# Download + extract once if any face is missing.
$needDownload = $faces | Where-Object { -not (Test-Path (Join-Path $fontDir $_.File)) }
if ($needDownload) {
    $tmp = Join-Path $env:TEMP ("meslo-" + [Guid]::NewGuid())
    New-Item -ItemType Directory -Force -Path $tmp | Out-Null
    $zip = Join-Path $tmp $NerdFontsAsset
    Write-Host "Downloading $NerdFontsAsset..." -ForegroundColor Cyan
    Invoke-WebRequest -Uri $NerdFontsUrl -OutFile $zip -UseBasicParsing
    Expand-Archive -Path $zip -DestinationPath $tmp -Force
    foreach ($f in $faces) {
        $src = Get-ChildItem -Path $tmp -Filter $f.File -Recurse | Select-Object -First 1
        if ($src) { Copy-Item $src.FullName (Join-Path $fontDir $f.File) -Force }
    }
    Remove-Item $tmp -Recurse -Force -ErrorAction SilentlyContinue
}

$installed = @()
foreach ($f in $faces) {
    $dest = Join-Path $fontDir $f.File
    if (Test-Path $dest) {
        New-ItemProperty -Path $regKey -Name $f.Reg -Value $dest -PropertyType String -Force | Out-Null
        $installed += $dest
    }
}

# Activate for the running session via GDI + broadcast WM_FONTCHANGE
# (same pattern previously in Install-UbuntuMono, lines 217-226).
$sig = @'
[DllImport("gdi32.dll", CharSet=CharSet.Unicode)]
public static extern int AddFontResourceW(string lpFileName);
[DllImport("user32.dll", CharSet=CharSet.Auto)]
public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, IntPtr wParam, IntPtr lParam, uint fuFlags, uint uTimeout, out IntPtr lpdwResult);
'@
$api = Add-Type -MemberDefinition $sig -Name FontApi -Namespace Win32 -PassThru
foreach ($p in $installed) { [void]$api::AddFontResourceW($p) }
$res = [IntPtr]::Zero
[void]$api::SendMessageTimeout([IntPtr]0xffff, 0x001D, [IntPtr]::Zero, [IntPtr]::Zero, 2, 2000, [ref]$res)
Write-Host "Activated $FontFamily for this session (restart Terminal to pick it up)." -ForegroundColor Green
return $FontFamily
```

### Task 4 — Refactor setup-apps.ps1 to delegate to the extracted script

**File:** `opt/Desktop/Apps/scripts/setup-apps.ps1`

1. **Line 51** — change `$TerminalFont = 'Ubuntu Mono'` to `$TerminalFont = 'MesloLGS NF'`. (Consumed unchanged at line 294 in `Configure-Terminal`, so the Windows Terminal profiles pick up the new face automatically.)
2. **Lines 172–228** — DELETE the `Install-UbuntuMono` function body. Replace the `# ---- Fonts ----` section with a thin delegator that locates and dot-sources the gsl-packaged script. The dotfiles `opt` tree is exposed in WSL; from Windows the script is reached via the repo checkout. Add:

```powershell
# ---- Fonts -----------------------------------------------------------------
# Font install/activation now lives in the gsl skill so it stays in sync with
# the codepoints gsl renders. setup-apps.ps1 only delegates.
function Install-NerdFont {
    Write-Host "`n=== [4/6] Nerd Font (MesloLGS NF) ===" -ForegroundColor Cyan
    # Resolve the gsl-packaged installer relative to this repo checkout.
    # setup-apps.ps1 sits at opt/Desktop/Apps/scripts/; the script is at
    # src/gsl/scripts/ — four levels up, then into src/gsl/scripts.
    $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..\..\..')).Path
    $fontScript = Join-Path $repoRoot 'src\gsl\scripts\install_nerd_font_windows.ps1'
    if (-not (Test-Path $fontScript)) {
        Write-Host "Nerd Font installer not found at $fontScript -- skipping." -ForegroundColor Yellow
        return
    }
    $family = & $fontScript
    if ($family) { $script:TerminalFont = $family }
}
```

3. **Line 426** — change the call `Install-UbuntuMono` to `Install-NerdFont`.

- [ ] Create `src/gsl/scripts/install_nerd_font_windows.ps1` with the Task 3 content; `chmod +x` (or mark executable as repo convention allows)
- [ ] Edit `opt/Desktop/Apps/scripts/setup-apps.ps1`: line 51 font value, lines 172–228 function replacement, line 426 call rename
- [ ] Confirm `setup-apps.ps1`'s other phases (WSL, Distros, GitHub link, Windows Terminal, winget) are untouched
- [ ] Commit: `feat(gsl): extract Windows font install into gsl skill; setup-apps.ps1 delegates`

---

## Glyph-Coverage Validation (proves gsl's codepoints render)

This proves the INSTALLED font covers the exact PUA codepoints gsl emits — not merely that "a font is installed." The codepoints come from `src/gsl/internal/style/builtins.go` (the `powerlineStyle` glyph constants).

### The exact codepoints gsl uses

| Constant | Codepoint | Meaning |
|----------|-----------|---------|
| `nfSepRight` | U+E0B0 | Powerline right-filled separator |
| `nfSepRightThin` | U+E0B1 | Powerline right-thin sub-separator |
| `nfBranch` | U+E0A0 | Powerline branch symbol |
| `nfFolder` | U+F07B | Folder (dirgit) |
| `nfRepoRoot` | U+F408 | Repo-root indicator (main worktree) |
| `nfWorktree` | U+F126 | Worktree indicator (linked worktree) |
| `nfStaged` | U+F055 | Staged-changes (plus-circle) |
| `nfUnstaged` | U+F071 | Unstaged-changes (warning) |
| `nfUntracked` | U+F128 | Untracked (question-mark) |
| `nfStash` | U+F48E | Stash (archive) |
| `nfAhead` | U+F176 | Ahead-of-remote (up-arrow) |
| `nfBehind` | U+F175 | Behind-remote (down-arrow) |
| `nfAI` | U+F135 | AI (rocket) |
| `nfMCP` | U+F1E6 | MCP (plug) |
| `nfTime` | U+F017 | Clock |
| `nfContext` | U+F47E | Context-window (circuit) |
| `nfWTCount` | U+F1C0 | Worktree-count prefix (database) |

That is 17 codepoints: `U+E0A0 U+E0B0 U+E0B1 U+F017 U+F055 U+F071 U+F07B U+F126 U+F128 U+F135 U+F175 U+F176 U+F1C0 U+F1E6 U+F408 U+F47E U+F48E`.

### Task 5 — Add a Go-based glyph checker under src/gsl

A small Go program parses a `.ttf` cmap (via `golang.org/x/image/font/sfnt`) and reports whether each gsl codepoint has a glyph. Go is preferred over a Python fontTools script because the gsl module already builds in Go (module `github.com/wenlock/dotfiles/gsl`, go 1.26.3) and `install.sh` already has the toolchain installed at this point in the run. (`golang.org/x/image` is not yet a dependency, so `go get golang.org/x/image/font/sfnt` is part of this task; the new dependency must pass `src/gsl/scripts/check-deps.sh` — `golang.org/x/image` is BSD-licensed, permissive, so the license gate passes.)

**New file:** `src/gsl/cmd/glyphcheck/main.go`

```go
// Command glyphcheck verifies that a font file contains a glyph for every
// Private-Use-Area codepoint the gsl powerline style emits. Exit 0 = all
// covered; exit 1 = one or more missing (printed to stderr).
package main

import (
	"fmt"
	"os"

	"golang.org/x/image/font/sfnt"
)

// codepoints are the exact runes from internal/style/builtins.go (powerlineStyle).
var codepoints = []rune{
	0xE0A0, 0xE0B0, 0xE0B1, 0xF017, 0xF055, 0xF071, 0xF07B, 0xF126,
	0xF128, 0xF135, 0xF175, 0xF176, 0xF1C0, 0xF1E6, 0xF408, 0xF47E, 0xF48E,
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: glyphcheck <font.ttf>")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	f, err := sfnt.Parse(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", os.Args[1], err)
		os.Exit(2)
	}
	var b sfnt.Buffer
	var missing []rune
	for _, r := range codepoints {
		g, err := f.GlyphIndex(&b, r)
		if err != nil || g == 0 {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		for _, r := range missing {
			fmt.Fprintf(os.Stderr, "MISSING U+%04X\n", r)
		}
		fmt.Fprintf(os.Stderr, "FAIL: %d/%d gsl codepoints missing in %s\n",
			len(missing), len(codepoints), os.Args[1])
		os.Exit(1)
	}
	fmt.Printf("OK: all %d gsl codepoints present in %s\n", len(codepoints), os.Args[1])
}
```

**New file:** `src/gsl/scripts/check-font-glyphs.sh` (the dev-loop / CI entry point, mirroring `check-deps.sh`)

```bash
#!/usr/bin/env bash
# check-font-glyphs.sh — prove the installed MesloLGS NF covers every codepoint
# gsl's powerline style emits. Locates the font per-OS, builds + runs the
# glyphcheck Go helper. Citable from CI.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MODULE_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Locate a MesloLGS NF Regular face per platform.
case "$(uname -s)" in
  Darwin) font="$(/usr/bin/find "${HOME}/Library/Fonts" /Library/Fonts \
            -iname 'MesloLGS NF Regular.ttf' 2>/dev/null | head -1)" ;;
  *)      font="$(fc-list 2>/dev/null | grep -i 'MesloLGS NF Regular' \
            | head -1 | cut -d: -f1)" ;;
esac
if [ -z "${font:-}" ] || [ ! -f "$font" ]; then
  echo "FAIL: MesloLGS NF Regular not found. Run the OS font installer first."
  exit 1
fi

echo "== glyph-coverage check against: $font =="
( cd "$MODULE_DIR" && go run ./cmd/glyphcheck "$font" )
```

**Companion test** (mirrors the `check-deps_test.sh` convention already in `src/gsl/scripts/`):

**New file:** `src/gsl/scripts/check-font-glyphs_test.sh` — asserts `glyphcheck` exits 0 against a fixture known to contain all 17 codepoints, and exits 1 against a fixture missing one (e.g. plain `Helvetica`/a stub TTF). Keep it network-free by checking out a tiny vendored fixture or skipping with a clear message when no font is available.

Exact dev-loop command + expected output:

```
$ bash src/gsl/scripts/check-font-glyphs.sh
== glyph-coverage check against: /Users/<you>/Library/Fonts/MesloLGS NF Regular.ttf ==
OK: all 17 gsl codepoints present in /Users/<you>/Library/Fonts/MesloLGS NF Regular.ttf
```

- [ ] `cd src/gsl && go get golang.org/x/image/font/sfnt` (adds the BSD-licensed dep)
- [ ] Create `src/gsl/cmd/glyphcheck/main.go`
- [ ] Create `src/gsl/scripts/check-font-glyphs.sh`; `chmod +x`
- [ ] Create `src/gsl/scripts/check-font-glyphs_test.sh`; `chmod +x`
- [ ] Run `bash src/gsl/scripts/check-deps.sh` to confirm the new dep passes the seam + license gate
- [ ] Run `bash src/gsl/scripts/check-font-glyphs.sh` and confirm the `OK: all 17` line
- [ ] Commit: `feat(gsl): add glyph-coverage checker proving font covers gsl codepoints`

---

## install.sh OS-Dispatch Orchestration (after the gsl build)

### Task 6 — Wire the gsl-packaged font scripts into install.sh AFTER the gsl build

The orchestration runs AFTER the gsl build block (`install.sh` lines 405–414, which runs `src/gsl/build.sh` and produces `~/opt/bin/gsl`). Placing it here guarantees the gsl skill files are already linked (sync-skills ran at L114) AND the `gsl` binary exists, so the font config + glyph validation can reference both. Insert immediately AFTER the gsl build block's closing `fi` (after line 414, before the `# install fnm` block).

**File:** `install.sh` — insert after line 414:

```bash
# Configure the Nerd Font (MesloLGS NF) used by gsl's powerline style.
# Runs AFTER the gsl build so both the gsl skill files (linked by sync-skills
# above) and the freshly-built ~/opt/bin/gsl exist. OS-dispatch to the
# gsl-packaged installers under src/gsl/scripts/.
GSL_FONT_SCRIPTS="${BASE_DIR}/src/gsl/scripts"
case "$(uname -s)" in
  Darwin)
    if [ -f "${GSL_FONT_SCRIPTS}/install_nerd_font_macos.sh" ]; then
      echo "Configuring Nerd Font (macOS)..."
      bash "${GSL_FONT_SCRIPTS}/install_nerd_font_macos.sh" || \
        echo "WARNING: macOS Nerd Font setup reported problems; continuing."
    fi
    ;;
  Linux)
    if [ ! -f "$NIX_MANAGED_FILE" ] && [ -f "${GSL_FONT_SCRIPTS}/install_nerd_font_linux.sh" ]; then
      echo "Configuring Nerd Font (Linux/WSL)..."
      bash "${GSL_FONT_SCRIPTS}/install_nerd_font_linux.sh" || \
        echo "WARNING: Linux Nerd Font setup reported problems; continuing."
    fi
    ;;
esac

# Prove the installed font covers every codepoint gsl renders (non-fatal).
if [ -f "${GSL_FONT_SCRIPTS}/check-font-glyphs.sh" ] && command -v go >/dev/null 2>&1; then
  echo "Validating gsl glyph coverage..."
  bash "${GSL_FONT_SCRIPTS}/check-font-glyphs.sh" || \
    echo "WARNING: glyph-coverage check failed; gsl powerline glyphs may not render."
fi
```

(Windows is provisioned by `setup-apps.ps1` → `Install-NerdFont`, not by this bash `install.sh`, so it has no branch here.)

**Worktree safety note (KEEP):** NEVER run `install.sh` from a `gss feature` worktree. The script creates absolute symlinks in `$HOME` (e.g. `~/.zshrc -> .../dotfiles/opt/profiles/zshrc`) and now also dispatches font installers. Running it from a worktree would link your global config to a transient, task-specific path. Always switch to `~/git/dotfiles` (main branch) and run `./install.sh` from there.

- [ ] Edit `install.sh` to insert the OS-dispatch block after line 414 (after the gsl build's closing `fi`)
- [ ] Verify placement: it must be AFTER the `# build and install gsl` block and BEFORE `# install fnm`
- [ ] Commit: `feat(install): orchestrate gsl Nerd Font setup after gsl build`

---

## End-to-End Verification

Run after a fresh `install.sh` on each platform.

**macOS (iTerm2):**
```bash
# 1. Font files present
system_profiler SPFontsDataType | grep -i "MesloLGS"          # expect 4 faces
# 2. Glyph coverage proven
bash ~/git/dotfiles/src/gsl/scripts/check-font-glyphs.sh       # expect: OK: all 17 ...
# 3. iTerm2 Dynamic Profile present + selected
ls "${HOME}/Library/Application Support/iTerm2/DynamicProfiles/gsl-nerd-font.json"
# 4. gsl render smoke test (must NOT show tofu boxes)
echo '{"cwd":"/tmp","session_id":"test"}' | gsl render
#    expect: colored powerline segments with chevron + branch + folder glyphs
```

**Linux/WSL:**
```bash
fc-list | grep -i "MesloLGS"
bash ~/git/dotfiles/src/gsl/scripts/check-font-glyphs.sh        # expect: OK: all 17 ...
echo '{"cwd":"/tmp","session_id":"test"}' | gsl render
```

**Windows (PowerShell):**
```powershell
[void][System.Reflection.Assembly]::LoadWithPartialName("System.Drawing")
(New-Object System.Drawing.Text.InstalledFontCollection).Families |
  Where-Object { $_.Name -like "*MesloLGS*" }                  # expect: MesloLGS NF
```

- [ ] Run platform verification; capture a before/after screenshot for the PR (optional)
- [ ] Commit: `docs(gsl): record Nerd Font end-to-end verification`

---

## Documentation

### Task 7 — Mention the font setup in gsl SKILL.md

So the skill documents its own font dependency.

**File:** `src/gsl/skill/SKILL.md`

Add to the **Install wiring** section a note that `install.sh` runs the gsl-packaged font installers (`src/gsl/scripts/install_nerd_font_{macos,linux,windows}.sh`) AFTER the gsl build, and that `src/gsl/scripts/check-font-glyphs.sh` proves the installed font covers the 17 powerline codepoints. Note the pinned `NERD_FONTS_VERSION` and that iTerm2 uses a Dynamic Profile (`gsl-nerd-font`).

- [ ] Edit `src/gsl/skill/SKILL.md` to add the font-setup note under Install wiring
- [ ] Commit: `docs(gsl): document Nerd Font auto-install in SKILL.md`

---

## .gitignore Note

All new files live under `src/gsl/...` (`src/gsl/scripts/`, `src/gsl/cmd/glyphcheck/`) and under `opt/...` (the edited `setup-apps.ps1`). Both trees are already opted in by the existing `!src/**` and `!opt/**` rules in `.gitignore`, so NO `.gitignore` change is needed. Verify (from the MAIN repo, never a worktree):

```bash
git check-ignore -v src/gsl/scripts/install_nerd_font_macos.sh   # expect: no output (tracked)
git status --short -- src/gsl/ install.sh opt/Desktop/Apps/scripts/setup-apps.ps1
```

- [ ] Run `git check-ignore -v` on each new path; confirm none are ignored
- [ ] If any path IS ignored, add a narrow `!`-rule (e.g. `!src/gsl/**`) — never `git add -f`

---

## Genuinely-Deferred Follow-Up (separate future issue)

### Follow-up — gsl self-detecting a missing Nerd Font and auto-falling back

**Scope:** A Go CODE change inside `src/gsl/` — explicitly out of scope for THIS issue because it is application logic, not a font install. Track as its own issue.

Current behavior: if a Nerd Font is absent, `gsl render` emits PUA codepoints that render as tofu; the manual workaround is `gsl config style emoji`.

Proposed approach (for the future issue):
1. In the gsl render path, when the resolved style's `Glyphs` is `nerdfont`, probe font availability through an `os/exec` seam (per `check-deps.sh`'s seam rule — route the subprocess via `internal/git`/`internal/mcp`/`internal/gh`-style seam, NOT a raw `os/exec` import in `internal/style`). Reuse the same `check-font-glyphs.sh` logic / `glyphcheck` helper conceptually.
2. If the font is missing, downgrade to `emoji` and emit a one-time stderr warning suggesting `gsl config style powerline` after installing the font.
3. Cache the probe (e.g. `~/.config/gsl/font_probe_cache`, refresh after 24h) to avoid a per-render `system_profiler`/`fc-list` penalty.

This issue eliminates the root cause (the font is now installed by `install.sh`); the auto-fallback is a UX nicety for machines that still lack it.

---

## Review feedback addressed

| # | Review comment (summary) | Resolved in |
|---|--------------------------|-------------|
| 1 | Make these gsl-packaged helper scripts; install.sh orchestrates them AFTER skills are installed; break font logic out of setup-apps.ps1 (keep its other functions, delegate); refactor system font config to be gsl-compatible; SKILL.md should mention it | **Architecture** (rewritten), **Tasks 1–4** (scripts live in `src/gsl/scripts/`), **Task 6** (install.sh OS-dispatch after the gsl build), **Task 7** (SKILL.md note) |
| 2 | Add base-requirement checks (e.g. brew on macOS); implement Linux/WSL + Windows in THIS round, not "later"; drop "(later)" from Tech Stack | **Requirements / Preflight** section + per-script preflight in **Tasks 1–4**; Linux/WSL (**Task 2**) and Windows (**Tasks 3–4**) are now in-scope; **Tech Stack** line no longer says "(later)"; only the gsl Go auto-fallback stays deferred (**Genuinely-Deferred Follow-Up**) |
| 3 | Validate during dev loops that the gsl characters are covered | **Glyph-Coverage Validation** + **Task 5**: lists the 17 real codepoints from `builtins.go`, adds `cmd/glyphcheck/main.go` + `check-font-glyphs.sh` (+ companion test), with the exact command and expected `OK: all 17` output; cited from CI and from **Task 6** |
| 4 | Use a pinned version of the files/cask | **Shared constants** + Canonical Font Choice: single pinned `NERD_FONTS_VERSION="v3.4.0"` used by all OS scripts, downloading the pinned `Meslo.zip` release asset (not the always-latest Homebrew cask); old "Open Question 1" resolved to: prefer pinned release download for reproducibility |
| 5 | iTerm2 plist race — find a workaround that works while running inside iTerm2; OK to print a restart message | **Task 1**: replaced live `com.googlecode.iterm2.plist` patching with an iTerm2 **Dynamic Profile** (`gsl-nerd-font.json`) that iTerm2 loads live and does not overwrite on quit; ends with a "restart iTerm2 / open a new window to apply" message; quit-first requirement dropped; Terminal.app guidance retained with its weaker-PUA-fallback note; old "Open Question 2" resolved |
