# Nerd Font Auto-Install Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire a canonical Nerd Font into the dotfiles install pipeline so that gsl's `powerline` style and p10k render glyphs correctly in iTerm2 and Terminal.app on macOS, and provide a clear cross-platform follow-up path for Linux/WSL and Windows.

**Architecture:** On macOS, the font is declared as a `cask` entry in `opt/profiles/Brewfile` (with the `homebrew/cask-fonts` tap) so that `brew bundle` in `install.sh` pulls it automatically; iTerm2 and Terminal.app font settings are applied by a new script `opt/scripts/system/install_nerd_font_macos.sh` that is called from `install.sh` after the brew bundle step. On Linux/WSL, a separate companion script downloads the font archive and installs it to `~/.local/share/fonts` then runs `fc-cache`. On Windows, `setup-apps.ps1` is updated to replace Ubuntu Mono with the matching Nerd Font variant, using the same GDI activation pattern already present in `Install-UbuntuMono`.

**Tech Stack:** Homebrew Cask, macOS defaults/plist, install.sh, (later) fontconfig + PowerShell

---

Closes #29.

## Canonical Font Choice

**Recommended: `font-meslo-lg-nerd-font`**

Justification:
- MesloLGS NF is the _official_ recommended font for Powerlevel10k (`p10k`) — the repo already uses p10k (`opt/profiles/.p10k.zsh`, `opt/profiles/.zshrc` line 78), so this is the zero-friction choice.
- The Homebrew Cask name `font-meslo-lg-nerd-font` exists under `homebrew/cask-fonts` tap (as of Nerd Fonts v3+, merged under `homebrew/cask`) and installs all four faces (Regular, Bold, Italic, BoldItalic) covering `MesloLGS NF`.
- Covers every codepoint used in `src/gsl/internal/style/builtins.go` (U+E0A0–E0B3 Powerline separators, plus U+E000–U+F8FF, U+F0000+ Nerd Fonts PUA ranges for branch, folder, staged/unstaged/untracked, stash, ahead/behind, AI/rocket, MCP/plug, clock, circuit icons).
- Alternatives (`font-caskaydia-cove-nerd-font`, `font-fira-code-nerd-font`) would also work but require a separate p10k font config change; MesloLGS NF avoids that extra step.

---

## macOS Tasks (Primary Scope)

### Task 1 — Add homebrew/cask-fonts tap and font cask to Brewfile

**File:** `opt/profiles/Brewfile`

After the existing `tap 'hashicorp/tap'` block (around line 12), add:

```ruby
# Nerd Font for gsl powerline style and p10k glyphs
# MesloLGS NF is the p10k-recommended Nerd Font; covers all gsl PUA codepoints.
tap 'homebrew/cask-fonts'
cask 'font-meslo-lg-nerd-font'
```

> Note: As of Homebrew 4.x the `homebrew/cask-fonts` tap may already be merged into core casks. The `tap` line is safe to keep — if already tapped/merged, `brew tap` is a no-op.

- [ ] Open `opt/profiles/Brewfile`
- [ ] Insert the tap + cask lines after the last existing `tap` block
- [ ] Verify: `brew search font-meslo-lg-nerd-font` returns the cask (on a macOS dev box)
- [ ] Commit: `feat(fonts): add font-meslo-lg-nerd-font cask to Brewfile`

---

### Task 2 — Create opt/scripts/system/install_nerd_font_macos.sh

This script configures iTerm2 and Terminal.app to use MesloLGS NF. It is idempotent and safe to re-run.

**File:** `opt/scripts/system/install_nerd_font_macos.sh`

```bash
#!/bin/bash
# install_nerd_font_macos.sh
# Configures iTerm2 and Terminal.app to use MesloLGS NF (Nerd Font).
# Run after brew bundle has installed font-meslo-lg-nerd-font.
# Safe to re-run (idempotent via defaults write).

set -euo pipefail

FONT_NAME="MesloLGS NF"
FONT_SIZE=13

# ── Verify the font is actually installed ─────────────────────────────────────
if ! system_profiler SPFontsDataType 2>/dev/null | grep -qi "MesloLGS"; then
  echo "WARNING: MesloLGS NF not found in system fonts."
  echo "         Run: brew install --cask font-meslo-lg-nerd-font"
  echo "         Then re-run this script."
  exit 1
fi
echo "MesloLGS NF found in system fonts."

# ── iTerm2 ─────────────────────────────────────────────────────────────────────
# iTerm2 stores its prefs in ~/Library/Preferences/com.googlecode.iterm2.plist
# We set the font for the Default profile (Guid "").
# Uses plutil to write a typed plist value rather than raw defaults write.
ITERM_PLIST="${HOME}/Library/Preferences/com.googlecode.iterm2.plist"

if [ -f "$ITERM_PLIST" ]; then
  echo "Configuring iTerm2 Default profile font -> ${FONT_NAME} ${FONT_SIZE}..."

  # The Normal Font key encodes as: "<FamilyName> <Size>" in the legacy format.
  # For plist binary prefs, plutil is more reliable than defaults write.
  # We target BookmarkName="" (the Default profile) inside the "New Bookmarks" array.

  python3 - <<PYEOF
import plistlib, os, sys

path = os.path.expanduser("~/Library/Preferences/com.googlecode.iterm2.plist")
with open(path, "rb") as f:
    data = plistlib.load(f)

bookmarks = data.get("New Bookmarks", [])
changed = False
for bm in bookmarks:
    if bm.get("Bookmark Name") in ("Default", "") or bm.get("Name") in ("Default", ""):
        bm["Normal Font"] = "${FONT_NAME} ${FONT_SIZE}"
        bm["Non Ascii Font"] = "${FONT_NAME} ${FONT_SIZE}"
        bm["Use Non-ASCII Font"] = True
        changed = True

if not changed and bookmarks:
    # No "Default" named profile found; patch the first profile as fallback
    bookmarks[0]["Normal Font"] = "${FONT_NAME} ${FONT_SIZE}"
    bookmarks[0]["Non Ascii Font"] = "${FONT_NAME} ${FONT_SIZE}"
    bookmarks[0]["Use Non-ASCII Font"] = True
    changed = True

if changed:
    with open(path, "wb") as f:
        plistlib.dump(data, f)
    print("iTerm2 plist updated.")
else:
    print("WARNING: No iTerm2 profile found to update.")
PYEOF

  # Invalidate iTerm2's in-memory pref cache so changes take effect.
  # This is safe even if iTerm2 is not running.
  defaults read com.googlecode.iterm2 > /dev/null 2>&1 || true

else
  echo "iTerm2 plist not found at ${ITERM_PLIST}; skipping iTerm2 configuration."
  echo "  (Install iTerm2, launch it once, then re-run this script.)"
fi

# ── Terminal.app ────────────────────────────────────────────────────────────────
# Terminal.app uses com.apple.Terminal preferences.
# The "Font" key in each window settings dict is a serialized NSFont archived
# as NSData. The cleanest approach for scripting is osascript.
# NOTE: Terminal.app renders fewer Nerd Font PUA glyphs than iTerm2 because
#       it does not perform font fallback across the full Unicode PUA range.
#       iTerm2 is the recommended terminal for full gsl powerline rendering.

echo "Configuring Terminal.app default profile font -> ${FONT_NAME} ${FONT_SIZE}..."

osascript <<OSASCRIPT 2>/dev/null && echo "Terminal.app font set via AppleScript." || \
  echo "WARNING: Terminal.app font update via AppleScript failed (Terminal may not be running)."
tell application "Terminal"
  set default settings to settings set "Basic"
  set font name of default settings to "${FONT_NAME}"
  set font size of default settings to ${FONT_SIZE}
end tell
OSASCRIPT

echo ""
echo "Done. Restart iTerm2 and/or Terminal.app to pick up the new font."
echo "Verify with: system_profiler SPFontsDataType | grep -i meslo"
```

- [ ] Create `opt/scripts/system/install_nerd_font_macos.sh` with the content above
- [ ] `chmod +x opt/scripts/system/install_nerd_font_macos.sh`
- [ ] Commit: `feat(fonts): add install_nerd_font_macos.sh for iTerm2 + Terminal.app`

---

### Task 3 — Wire install_nerd_font_macos.sh into install.sh

**File:** `install.sh`

After the existing `brew bundle` block (around lines 174–188), add a call to the new script:

```bash
  # Configure iTerm2 + Terminal.app to use MesloLGS NF (Nerd Font).
  # This runs after brew bundle so the font cask is already installed.
  if [ -f "${BASE_DIR}/opt/scripts/system/install_nerd_font_macos.sh" ]; then
    echo "Configuring Nerd Font in terminal emulators..."
    bash "${BASE_DIR}/opt/scripts/system/install_nerd_font_macos.sh" || \
      echo "WARNING: Nerd Font terminal configuration reported problems; continuing."
  fi
```

The insertion point in `install.sh` (exact context to match):
```bash
    # Setup Vault as a standalone binary to avoid Xcode versioning issues
    if [ -f "${BASE_DIR}/opt/scripts/network/vault-setup.sh" ]; then
      "${BASE_DIR}/opt/scripts/network/vault-setup.sh"
    fi
  else
    echo "WARNING: Homebrew not found. Please install it: https://brew.sh/"
  fi
fi
```

Insert **after** the closing `fi` of the `if [[ "$(uname -s)" == "Darwin" ]]` block (line ~188), immediately before the `# Install sops` comment. Alternatively, insert it **inside** the Darwin block, after vault-setup.sh but before the closing `fi` — keeping font config macOS-gated.

Preferred: place inside the Darwin+brew block, after the vault-setup call:
```bash
    # Configure iTerm2 + Terminal.app to use MesloLGS NF (installed by brew bundle above).
    if [ -f "${BASE_DIR}/opt/scripts/system/install_nerd_font_macos.sh" ]; then
      echo "Configuring Nerd Font in terminal emulators..."
      bash "${BASE_DIR}/opt/scripts/system/install_nerd_font_macos.sh" || \
        echo "WARNING: Nerd Font terminal configuration reported problems; continuing."
    fi
```

**Worktree safety note:** Never run `install.sh` from a `gss feature` worktree. This script creates absolute symlinks in `$HOME` (e.g. `~/.zshrc -> .../dotfiles/opt/profiles/zshrc`). Running it from a worktree would link your global config to a transient, task-specific path. Always switch to `~/git/dotfiles` (main branch) and run `./install.sh` from there.

- [ ] Edit `install.sh` to insert the font-configuration call inside the macOS brew block
- [ ] Verify the insertion is inside the `if [[ "$(uname -s)" == "Darwin" ]]; then` ... `fi` guard
- [ ] Commit: `feat(install): wire install_nerd_font_macos.sh into macOS install path`

---

### Task 4 — Verification step (proves glyphs render, not just that font installed)

Run this after a fresh `brew bundle` + `install.sh` run on macOS:

```bash
# Step 1: Confirm font files are present on disk
system_profiler SPFontsDataType | grep -i "MesloLGS"
# Expected: "MesloLGS NF" entries for Regular, Bold, Italic, BoldItalic

# Step 2: Confirm fc-list sees them (if fontconfig is available on macOS via brew)
fc-list | grep -i "MesloLGS" || echo "fc-list not available (ok on macOS)"

# Step 3: gsl render smoke test — must show powerline separators, NOT tofu boxes
# Open iTerm2, set font to MesloLGS NF 13 (done by install script), then:
cd ~/git/dotfiles
echo '{"cwd":"/tmp","session_id":"test"}' | gsl render
# Expected: colored segments with  (right-chevron), branch, folder glyphs
# Failure:  segments show □□□ or ??? instead of glyphs → font not active in terminal

# Step 4: Direct glyph probe — paste these in the terminal; all should render as icons
printf '\n'
# Expected: filled chevrons, folder, branch symbol — not tofu
```

- [ ] Run verification steps in iTerm2 after install
- [ ] Capture before/after screenshot (optional but recommended for PR)
- [ ] Commit: `docs(fonts): add verification steps to plan`

---

### Task 5 — Update .gitignore if new script path is not covered

The `install_nerd_font_macos.sh` lives under `opt/scripts/system/`, which is covered by the existing `!opt/**` rule in `.gitignore`. No `.gitignore` change is needed. Verify:

```bash
cd ~/git/dotfiles
git check-ignore -v opt/scripts/system/install_nerd_font_macos.sh
# Should output nothing (not ignored) — the file is tracked via !opt/**
git status --short -- opt/scripts/system/install_nerd_font_macos.sh
# Should show: A  opt/scripts/system/install_nerd_font_macos.sh (staged) or ?? (untracked but not ignored)
```

- [ ] Run `git check-ignore -v opt/scripts/system/install_nerd_font_macos.sh` (from main repo)
- [ ] If ignored: add `!opt/scripts/system/install_nerd_font_macos.sh` to `.gitignore`
- [ ] Commit any `.gitignore` fix as its own commit: `fix(gitignore): opt-in install_nerd_font_macos.sh`

---

## Cross-Platform Follow-Up (Plan Only — Implement Later)

These are scoped out of the initial PR. Each is a separate issue or sub-task.

### Follow-up A — Linux/WSL: install to ~/.local/share/fonts + fc-cache

**Trigger:** `install.sh` on Linux (non-macOS, non-Nix).

**Script to create:** `opt/scripts/system/install_nerd_font_linux.sh`

Approach:
1. Download the `MesloLGS NF` release zip from https://github.com/ryanoasis/nerd-fonts/releases (the `Meslo.zip` asset from the latest release, currently v3.x).
2. Unzip into `~/.local/share/fonts/MesloLGS/`.
3. Run `fc-cache -f ~/.local/share/fonts`.
4. Verify: `fc-list | grep -i "MesloLGS"`.

Hook into `install.sh`:
```bash
if [[ "$(uname -s)" == "Linux" ]] && [ -z "$NIX_MANAGED_FILE" ]; then
  if [ -f "${BASE_DIR}/opt/scripts/system/install_nerd_font_linux.sh" ]; then
    echo "Installing MesloLGS NF for Linux..."
    bash "${BASE_DIR}/opt/scripts/system/install_nerd_font_linux.sh" || \
      echo "WARNING: Linux Nerd Font install reported problems; continuing."
  fi
fi
```

WSL note: the font only needs to be installed on the **Windows host** for the terminal to render it. Installing to `~/.local/share/fonts` inside WSL affects WSL GUI apps only. For Windows Terminal (the usual WSL host), follow Follow-up B.

---

### Follow-up B — Windows: replace Ubuntu Mono with MesloLGS NF in setup-apps.ps1

**File:** `opt/Desktop/Apps/scripts/setup-apps.ps1`

Current state: `$TerminalFont = 'Ubuntu Mono'` (line 51) and the `Install-UbuntuMono` function (lines 172–228) download Ubuntu Mono from Google Fonts.

Changes needed:
1. Change `$TerminalFont = 'MesloLGS NF'` (line 51).
2. Rename `Install-UbuntuMono` to `Install-NerdFont` and replace the download URL and face list with the MesloLGS NF TTF files from https://github.com/ryanoasis/nerd-fonts/releases/latest (`MesloLGS-NF-Regular.ttf`, `MesloLGS-NF-Bold.ttf`, `MesloLGS-NF-Italic.ttf`, `MesloLGS-NF-BoldItalic.ttf`).
3. Update registry key names from `Ubuntu Mono` to `MesloLGS NF`.
4. The `Configure-Terminal` function already uses `$TerminalFont` in profile font objects, so updating step 1 propagates automatically to Windows Terminal profiles.

Verification on Windows:
```powershell
# After running setup-apps.ps1
[void][System.Reflection.Assembly]::LoadWithPartialName("System.Drawing")
$fonts = (New-Object System.Drawing.Text.InstalledFontCollection).Families
$fonts | Where-Object { $_.Name -like "*MesloLGS*" }
# Expected: MesloLGS NF family entry
```

---

### Follow-up C — Retire/replace stale font docs

**Files to update:**
- `opt/docs/powerline-fonts.md` — replace entire content with a note pointing to `install.sh` and `brew bundle`, plus a brief note that the legacy `PowerlineSymbols.otf` approach is superseded by the full Nerd Font.
- `opt/docs/ubuntu_tools.md` — the "give your command prompt some bling" section (lines 181–199) references the old pip-based powerline + `PowerlineSymbols.otf`. Update it to reference the new `install_nerd_font_linux.sh` script.

---

### Follow-up D — gsl self-detecting missing glyphs and auto-falling back

**Scope:** Go code change in `src/gsl/` — out of scope for this issue but worth planning.

Current situation: if a Nerd Font is not installed, `gsl render` outputs Private-Use-Area codepoints that render as tofu. The user's current workaround is `gsl config style emoji` to switch to the emoji style (no PUA codepoints needed).

**Proposed approach:**
1. In `src/gsl/cmd/statusline.go` (the `runStatusLine` function), before rendering, check whether the resolved style is `powerline` (Glyphs: `nerdfont`).
2. Probe for font availability by running `system_profiler SPFontsDataType | grep -qi MesloLGS` on macOS, `fc-list | grep -qi MesloLGS` on Linux, or checking `%LOCALAPPDATA%\Microsoft\Windows\Fonts\MesloLGS*` on Windows.
3. If not found, downgrade the style to `emoji` and emit a one-time `stderr` warning: `gsl: MesloLGS NF not found; using emoji style. Run 'gsl config style powerline' after installing the font.`
4. Cache the probe result in `~/.config/gsl/font_probe_cache` (refresh after 24h) to avoid the `system_profiler` penalty on every prompt render.

This is a Go module change — open a separate issue for it. The install fix in this issue eliminates the root cause; the auto-fallback is a UX nicety for machines that still lack the font.

---

## Open Questions

1. **Homebrew Cask name stability:** `font-meslo-lg-nerd-font` is the current Homebrew Cask name for Nerd Fonts v3. If the cask is renamed in a future Nerd Fonts release, `brew bundle` will fail. Mitigation: pin a comment with the Nerd Fonts release version in the Brewfile.

2. **iTerm2 plist write race:** if iTerm2 is open when `install_nerd_font_macos.sh` runs, it may overwrite the plist on quit, undoing the font change. The script should warn and prompt the user to quit iTerm2 before running, or document a post-install step of restarting iTerm2.

3. **Terminal.app AppleScript reliability:** the `font name of default settings` AppleScript key may not exist on older macOS versions (<= Ventura). The script already has a `|| echo WARNING` fallback. A `plutil`-based plist edit for `com.apple.Terminal` is a more robust alternative but requires knowing the `Window Settings` dict key for the active profile.

4. **p10k wizard vs. pre-configured font:** if the user runs `p10k configure` after install, the wizard will re-ask about font and may overwrite `~/.p10k.zsh`. The plan does not attempt to suppress the wizard; the correct fix is to have `opt/profiles/.p10k.zsh` already reference `MesloLGS NF` (it is currently set up for a Nerd Font — verify by checking `POWERLEVEL9K_MODE` in that file).

5. **gsl render test in CI:** the verification step (Task 4) is manual. A follow-up could add a CI job on macOS runners that installs the font cask, runs `gsl render`, and checks that the output contains the U+E0B0 codepoint rather than a replacement character.
