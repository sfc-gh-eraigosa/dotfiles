#!/usr/bin/env bash
# ==============================================================================
# Snowflake CLI Setup — install `snow` via brew (macOS) or pipx (Linux)
# ==============================================================================
# Why this exists:
#   * .zshrc used to auto-install the CLI with `pip3 install` from daily
#     maintenance. On Debian Bookworm+ (PEP 668 externally-managed-environment)
#     the system pip refuses, printing an error at login once a day, forever.
#   * The supported, PEP 668-safe paths are `uv tool install snowflake-cli` on
#     macOS (prebuilt wheels on both Mac architectures; uv is installed just
#     before this step by install_uv.sh / the Brewfile), the homebrew-core
#     `snowflake-cli` formula as the macOS fallback, and `pipx install
#     snowflake-cli` on Linux. Each gives the CLI its own venv, leaving the
#     system Python untouched. The brew formula is only the FALLBACK because on
#     an Intel Mac (no bottles since Sept 2026) it compiles cryptography and
#     pydantic-core, which drags in a from-source Rust AND LLVM build — hours.
#   * The PyPI package was renamed snowflake-cli-labs -> snowflake-cli; this
#     script installs the current name.
#
# Safe to re-run: exits early when `snow` is already resolvable.
set -e
SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# pipx installs into ~/.local/bin; brew into its own prefix (already on PATH on
# macOS). Put ~/.local/bin on PATH for the rest of this script so both the
# early-exit probe and the post-install verification below reflect reality —
# install.sh run from a NON-login shell (`fleet update` over ssh) does not get
# ~/.local/bin from ~/.profile, which is why a correctly-installed `snow`
# reported "not resolvable after install".
case ":${PATH}:" in
    *":${HOME}/.local/bin:"*) ;;
    *) PATH="${HOME}/.local/bin:${PATH}"; export PATH ;;
esac
# uv (install_uv.sh) lands in ~/opt/bin; make it resolvable when this script
# runs standalone from a shell that has not sourced ~/.profile.
case ":${PATH}:" in
    *":${HOME}/opt/bin:"*) ;;
    *) PATH="${HOME}/opt/bin:${PATH}"; export PATH ;;
esac

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

if command -v snow >/dev/null 2>&1; then
    echo -e "${GREEN}snowflake-cli already installed: $(snow --version 2>&1 | head -1)${NC}"
    exit 0
fi

case "$(uname -s)" in
    Darwin)
        # uv may be missing only because its download failed moments ago
        # (install.sh runs install_uv.sh right before this step); try once more
        # here rather than falling through to a from-source brew build.
        if ! command -v uv >/dev/null 2>&1 && [ -x "${SELF_DIR}/install_uv.sh" ]; then
            echo -e "${BLUE}uv not found; running install_uv.sh first...${NC}"
            "${SELF_DIR}/install_uv.sh" || true
            hash -r 2>/dev/null || true
        fi
        if command -v uv >/dev/null 2>&1; then
            # Pin the interpreter range: left alone, uv picks the NEWEST Python on
            # the host (3.15 here), for which pydantic-core has no wheel, so it
            # tried to compile it (and cryptography) with maturin and failed. The
            # Snowflake connector supports 3.10-3.13; inside that range every
            # dependency has a prebuilt wheel and uv prefers an interpreter that is
            # already installed (brew's python@3.10 here) over downloading one.
            echo -e "${BLUE}Installing snowflake-cli via uv tool (into ~/.local/bin)...${NC}"
            uv tool install --python '>=3.10,<3.14' snowflake-cli
        elif [ "$(uname -m)" = "arm64" ] && command -v brew >/dev/null 2>&1; then
            # Bottled on Apple silicon: a quick pour, so brew is a fine fallback.
            echo -e "${BLUE}uv unavailable; installing snowflake-cli via brew...${NC}"
            brew install snowflake-cli
        else
            # Intel: no bottles, and the formula builds cryptography + pydantic-core
            # and therefore Rust AND LLVM from source — hours. Refuse instead.
            echo -e "${RED}install_snowflake_cli: uv is unavailable and the brew formula would compile Rust + LLVM from source on this Intel Mac (hours).${NC}"
            echo -e "${RED}                       Fix uv first (opt/scripts/system/install_uv.sh), then re-run.${NC}"
            exit 1
        fi
        ;;
    Linux)
        # pipx is the PEP 668-safe installer for PyPI CLIs. Bootstrap it from
        # apt when missing (Debian/Ubuntu/RPi); other distros must provide
        # pipx themselves.
        if ! command -v pipx >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                echo -e "${BLUE}Installing pipx via apt...${NC}"
                # DEBIAN_FRONTEND=noninteractive is load-bearing: pipx pulls in
                # tzdata, whose debconf prompt blocks forever in non-interactive
                # contexts (this hung the Docker image build for hours once the
                # exec-bit fix let this script actually run there).
                sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends pipx
            else
                echo -e "${RED}install_snowflake_cli: pipx not found and no apt-get — install pipx manually, then re-run.${NC}"
                exit 1
            fi
        fi
        echo -e "${BLUE}Installing snowflake-cli via pipx...${NC}"
        pipx install snowflake-cli
        # pipx installs into ~/.local/bin; make sure future shells resolve it.
        pipx ensurepath >/dev/null 2>&1 || true
        ;;
    *)
        echo -e "${RED}install_snowflake_cli: unsupported OS $(uname -s)${NC}"
        exit 1
        ;;
esac

if command -v snow >/dev/null 2>&1 && snow --version >/dev/null 2>&1; then
    echo -e "${GREEN}snowflake-cli installed: $(snow --version 2>&1 | head -1)${NC}"
else
    echo -e "${RED}install_snowflake_cli: snow not resolvable after install — is ~/.local/bin on PATH?${NC}"
    echo -e "${RED}                       (open a new shell or run 'pipx ensurepath' and retry)${NC}"
    exit 1
fi
