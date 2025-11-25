#!/bin/sh

VERSION="1.4.3"
os="$(uname -s)"; arch="$(uname -m)"
case "$os" in
    Darwin)  platform="darwin"; ext="pkg" ;;
    Linux)   platform="linux";  ext="bash" ;;
    *) echo "Unsupported OS: $os" >&2; exit 1 ;;
esac
case "$arch" in
    x86_64|amd64)   arch="x86_64" ;;
    arm64|aarch64)  arch="arm64"  ;;
    *) echo "Unsupported arch: $arch" >&2; exit 1 ;;
esac

PLATFORM="${platform}_${arch}"
BASE="https://sfc-repo.snowflakecomputing.com/snowsql/bootstrap/${VERSION}/${PLATFORM}"
SIG_FILE="snowsql-${VERSION}-${PLATFORM}.${ext}.sig"
BASH_FILE="snowsql-${VERSION}-${PLATFORM}.${ext}"


curl -fSLo "$SIG_FILE" "${BASE}/${SIG_FILE}"
curl -fSLo "$BASH_FILE" "${BASE}/${BASH_FILE}"

chmod +x "$SIG_FILE"
chmod +x "$BASH_FILE"

gpg --verify "$SIG_FILE" "$BASH_FILE" || exit 1
SNOWSQL_DEST=~/opt/bin SNOWSQL_LOGIN_SHELL=~/.profile bash "$BASH_FILE"





