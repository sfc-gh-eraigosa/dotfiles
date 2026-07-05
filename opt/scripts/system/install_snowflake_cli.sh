#!/usr/bin/env bash
# ==============================================================================
# Snowflake CLI Setup — install `snow` via brew (macOS) or pipx (Linux)
# ==============================================================================
# Why this exists:
#   * .zshrc used to auto-install the CLI with `pip3 install` from daily
#     maintenance. On Debian Bookworm+ (PEP 668 externally-managed-environment)
#     the system pip refuses, printing an error at login once a day, forever.
#   * The supported, PEP 668-safe paths are the homebrew-core `snowflake-cli`
#     formula on macOS and `pipx install snowflake-cli` on Linux (pipx gives
#     the CLI its own venv, leaving the system Python untouched).
#   * The PyPI package was renamed snowflake-cli-labs -> snowflake-cli; this
#     script installs the current name.
#
# Safe to re-run: exits early when `snow` is already resolvable.
set -e

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
        if ! command -v brew >/dev/null 2>&1; then
            echo -e "${RED}install_snowflake_cli: Homebrew not found — install it first: https://brew.sh/${NC}"
            exit 1
        fi
        echo -e "${BLUE}Installing snowflake-cli via brew...${NC}"
        brew install snowflake-cli
        ;;
    Linux)
        # pipx is the PEP 668-safe installer for PyPI CLIs. Bootstrap it from
        # apt when missing (Debian/Ubuntu/RPi); other distros must provide
        # pipx themselves.
        if ! command -v pipx >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                echo -e "${BLUE}Installing pipx via apt...${NC}"
                sudo apt-get install -y -qq pipx
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
fi
