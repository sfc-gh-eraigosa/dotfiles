#!/usr/bin/env bash
# Test driver for sync-mcp.sh. Mirrors sync-plugins_test.sh style: a hermetic
# temp PATH of fake claude/gemini CLIs (real `yq` still resolves so the manifest
# is parsed for real), plus fixture manifests via SYNC_MCP_MANIFEST.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SYNC="${SCRIPT_DIR}/sync-mcp.sh"

PASS=0
FAIL=0

assert_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (missing: $needle)"; FAIL=$((FAIL+1))
    fi
}

assert_not_contains() {
    local haystack="$1" needle="$2" desc="$3"
    if printf '%s' "$haystack" | grep -qF -- "$needle"; then
        echo "FAIL: $desc (unexpectedly present: $needle)"; FAIL=$((FAIL+1))
    else
        echo "PASS: $desc"; PASS=$((PASS+1))
    fi
}

assert_eq() {
    local got="$1" want="$2" desc="$3"
    if [ "$got" = "$want" ]; then
        echo "PASS: $desc"; PASS=$((PASS+1))
    else
        echo "FAIL: $desc (got '$got' want '$want')"; FAIL=$((FAIL+1))
    fi
}

# --- Dry-run plans the right add commands (CLI-agnostic, real manifest) ---------
OUT="$(bash "$SYNC" --dry-run 2>&1)"
assert_contains "$OUT" "DRY-RUN: claude mcp add --scope user notebooklm -- npx -y notebooklm-mcp@2.0.0" "plans the Claude add at user scope with the pinned version"
assert_contains "$OUT" "DRY-RUN: gemini mcp add --scope user --transport stdio notebooklm npx -y notebooklm-mcp@2.0.0" "plans the Gemini add at user scope, stdio, pinned version"
assert_contains "$OUT" "DRY-RUN: post_install: npx --yes playwright install chromium" "dry-run shows the notebooklm playwright post_install command"
assert_not_contains "$OUT" "@latest" "never registers an unpinned @latest version"

CLAUDE_ADDS="$(printf '%s' "$OUT" | grep -c 'DRY-RUN: claude mcp add ')"
assert_eq "$CLAUDE_ADDS" "1" "plans exactly one Claude add (one enabled server with a claude block)"

# --- enabled:false is parked (fixture manifest) ---------------------------------
FIX="$(mktemp -d)"
cat > "$FIX/parked.yaml" <<'EOF'
servers:
  - name: liveone
    enabled: true
    transport: stdio
    command: npx
    args: ["-y", "liveone-mcp@1.0.0"]
    claude: {}
  - name: parkedone
    enabled: false
    transport: stdio
    command: npx
    args: ["-y", "parkedone-mcp@1.0.0"]
    claude: {}
EOF
PARKOUT="$(SYNC_MCP_MANIFEST="$FIX/parked.yaml" bash "$SYNC" --dry-run 2>&1)"
assert_contains "$PARKOUT" "DRY-RUN: claude mcp add --scope user liveone" "registers the enabled server"
assert_not_contains "$PARKOUT" "parkedone" "skips the parked (enabled:false) server"

# --- http transport is refused, never registered -------------------------------
cat > "$FIX/http.yaml" <<'EOF'
servers:
  - name: httpsrv
    enabled: true
    transport: http
    command: npx
    args: ["-y", "httpsrv-mcp@1.0.0"]
    claude: {}
EOF
HTTPOUT="$(SYNC_MCP_MANIFEST="$FIX/http.yaml" bash "$SYNC" --dry-run 2>&1)"
assert_contains "$HTTPOUT" "unsupported transport 'http'" "warns on a non-stdio transport"
assert_not_contains "$HTTPOUT" "mcp add --scope user httpsrv" "never registers an http-transport server"

# --- only the requested tool block is registered -------------------------------
cat > "$FIX/claudeonly.yaml" <<'EOF'
servers:
  - name: conly
    enabled: true
    transport: stdio
    command: npx
    args: ["-y", "conly-mcp@1.0.0"]
    claude: {}
EOF
CONLY="$(SYNC_MCP_MANIFEST="$FIX/claudeonly.yaml" bash "$SYNC" --dry-run 2>&1)"
assert_contains "$CONLY" "DRY-RUN: claude mcp add --scope user conly" "registers Claude when only a claude block is present"
assert_not_contains "$CONLY" "gemini mcp add --scope user --transport stdio conly" "does not register Gemini when no gemini block is present"
rm -rf "$FIX"

# --- yq-absent precondition -> exit 1 ------------------------------------------
NOYQ="$(mktemp -d)"
for _t in bash sh env grep awk sed tr cat head cut dirname basename readlink mktemp; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$NOYQ/$_t" 2>/dev/null
done
NOYQ_OUT="$(PATH="$NOYQ" bash "$SYNC" --dry-run 2>&1 || true)"
NOYQ_RC=0; PATH="$NOYQ" bash "$SYNC" --dry-run >/dev/null 2>&1 || NOYQ_RC=$?
rm -rf "$NOYQ"
assert_eq "$NOYQ_RC" "1" "exits 1 when yq is absent"
assert_contains "$NOYQ_OUT" "install_yq.sh" "points at install_yq.sh when yq is absent"

# --- Behavioral: hermetic fake CLIs (real yq parses the real manifest) ----------
# The fakes read stdin on add — if anything ever attached an interactive stdin the
# read would block and this test would hang, so completing IS the hang-guard.
FAKE_BIN="$(mktemp -d)"
cat > "$FAKE_BIN/claude" <<'EOF'
#!/usr/bin/env bash
IFS= read -r _ </dev/stdin 2>/dev/null || true   # must hit EOF, never block
if [ "$1 $2" = "mcp add" ]; then
    echo "RECORDED claude $*" >>"$REC"
    # Simulate a server that already exists: add stays rc=0 but prints the notice.
    if [ "${FAKE_DUP:-0}" = "1" ]; then
        echo "MCP server already exists in user config" ; exit 0
    fi
    echo "Added stdio MCP server to user config"
elif [ "$1 $2" = "mcp remove" ]; then
    echo "RECORDED claude $*" >>"$REC"
fi
exit 0
EOF
cat > "$FAKE_BIN/gemini" <<'EOF'
#!/usr/bin/env bash
IFS= read -r _ </dev/stdin 2>/dev/null || true
if [ "$1 $2" = "mcp add" ]; then
    echo "RECORDED gemini $*" >>"$REC"
    if [ "${FAKE_DUP:-0}" = "1" ]; then
        echo 'MCP server "notebooklm" is already configured, updated in user settings.' ; exit 0
    fi
    echo "MCP server added to user settings"
elif [ "$1 $2" = "mcp remove" ]; then
    echo "RECORDED gemini $*" >>"$REC"
fi
exit 0
EOF
chmod +x "$FAKE_BIN/claude" "$FAKE_BIN/gemini"

# First run: fresh registration. Records the add commands; emits no dup/skip notice.
REC1="$(mktemp)"
RUN1="$(REC="$REC1" PATH="$FAKE_BIN:$PATH" bash "$SYNC" 2>&1)"
assert_contains "$RUN1" "+ claude mcp add --scope user notebooklm -- npx -y notebooklm-mcp@2.0.0" "invokes the real Claude add on a fresh host"
assert_contains "$(cat "$REC1")" "RECORDED claude mcp add --scope user notebooklm -- npx -y notebooklm-mcp@2.0.0" "passes scope+pinned args through to claude"
assert_contains "$(cat "$REC1")" "RECORDED gemini mcp add --scope user --transport stdio notebooklm npx -y notebooklm-mcp@2.0.0" "passes scope+transport+pinned args through to gemini"
assert_not_contains "$(cat "$REC1")" "mcp remove" "ensure-only: never emits a remove"
rm -f "$REC1"

# Idempotent re-run: both CLIs report the server already exists -> quiet skip, no WARNING.
REC2="$(mktemp)"
RUN2="$(REC="$REC2" FAKE_DUP=1 PATH="$FAKE_BIN:$PATH" bash "$SYNC" 2>&1)"
assert_contains "$RUN2" "(notebooklm already registered for Claude)" "tolerates Claude 'already exists' as a quiet skip"
assert_contains "$RUN2" "(notebooklm already registered for Gemini)" "tolerates Gemini 'already configured' as a quiet skip"
assert_not_contains "$RUN2" "sync-mcp: WARNING" "no sync-mcp WARNING on an idempotent re-run"
rm -f "$REC2"

# Missing-CLI graceful skip: yq present, no claude/gemini on a curated PATH.
NOCLI="$(mktemp -d)"
for _t in bash sh env grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp yq; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$NOCLI/$_t" 2>/dev/null
done
NOCLI_RC=0
NOCLI_OUT="$(PATH="$NOCLI" bash "$SYNC" 2>&1)" || NOCLI_RC=$?
rm -rf "$NOCLI"
assert_eq "$NOCLI_RC" "0" "exits 0 when the CLIs are absent (graceful skip)"
assert_contains "$NOCLI_OUT" "'claude' CLI not on PATH; skipping" "skips Claude when its CLI is absent"
assert_contains "$NOCLI_OUT" "'gemini' CLI not on PATH; skipping" "skips Gemini when its CLI is absent"

# --- Behavioral: timeout actually kills a SIGTERM-ignoring add (-k) -------------
# A Node CLI may ignore SIGTERM; with FAKE_HANG the fake ignores it and loops, so
# only the timeout's -k SIGKILL escalation can stop it. Outer `timeout 40` is the
# test's own backstop: a missing -k would hang and exit 124, failing loudly.
cat > "$FAKE_BIN/claude" <<'EOF'
#!/usr/bin/env bash
if [ "$1 $2" = "mcp add" ] && [ "${FAKE_HANG:-0}" = "1" ]; then
    trap '' TERM
    for _ in $(seq 1 120); do sleep 0.2; done
fi
exit 0
EOF
chmod +x "$FAKE_BIN/claude"
RUN3="$(FAKE_HANG=1 SYNC_MCP_TIMEOUT=2 SYNC_MCP_KILL_GRACE=2 \
        PATH="$FAKE_BIN:$PATH" timeout 40 bash "$SYNC" 2>&1)"
T3RC=$?
assert_eq "$T3RC" "0" "sync completes (no hang) when an add ignores SIGTERM"
assert_contains "$RUN3" "timed out after 2s" "escalates to SIGKILL on a SIGTERM-ignoring add"
rm -rf "$FAKE_BIN"

# --- Behavioral: add calls run under setsid (no controlling terminal) -----------
GUARD_BIN="$(mktemp -d)"
cat > "$GUARD_BIN/setsid" <<'EOF'
#!/usr/bin/env bash
echo "SETSID_USED" >&2
[ "$1" = "-w" ] && shift   # sync-mcp passes -w; then exec the rest
exec "$@"
EOF
cat > "$GUARD_BIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$GUARD_BIN/gemini" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$GUARD_BIN/setsid" "$GUARD_BIN/claude" "$GUARD_BIN/gemini"
GUARDOUT="$(PATH="$GUARD_BIN:$PATH" bash "$SYNC" 2>&1)"
rm -rf "$GUARD_BIN"
assert_contains "$GUARDOUT" "SETSID_USED" "runs mcp add under setsid to detach the controlling terminal"

# --- Behavioral: a /dev/tty-grabbing CLI must NOT hang the installer ------------
# End-to-end proof under a real pty (Linux-only; needs `script` + `setsid`).
if command -v script >/dev/null 2>&1 && command -v setsid >/dev/null 2>&1; then
    TTY_BIN="$(mktemp -d)"
    cat > "$TTY_BIN/claude" <<'EOF'
#!/usr/bin/env bash
if : <>/dev/tty 2>/dev/null; then
    sleep 600            # controlling terminal reachable -> mimic a hanging TUI
else
    echo "headless-ok"   # no controlling terminal -> setsid detached it
fi
exit 0
EOF
    cat > "$TTY_BIN/gemini" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
    chmod +x "$TTY_BIN/claude" "$TTY_BIN/gemini"
    TTYOUT="$(SYNC_MCP_TIMEOUT=3 SYNC_MCP_KILL_GRACE=2 PATH="$TTY_BIN:$PATH" \
              timeout 60 script -qec "bash '$SYNC'" /dev/null </dev/null 2>&1 | tr -d '\r')"
    TTYRC=$?
    rm -rf "$TTY_BIN"
    assert_eq "$TTYRC" "0" "sync completes under a pty when the CLI would grab /dev/tty"
    assert_contains "$TTYOUT" "headless-ok" "setsid detaches the controlling terminal so claude runs headless"
fi

# --- Behavioral: macOS keg-only util-linux setsid is discovered via brew --------
KEG_ROOT="$(mktemp -d)"
mkdir -p "$KEG_ROOT/bin"
cat > "$KEG_ROOT/bin/setsid" <<'EOF'
#!/usr/bin/env bash
echo "KEG_SETSID_USED" >&2
[ "$1" = "-w" ] && shift
exec "$@"
EOF
chmod +x "$KEG_ROOT/bin/setsid"
KEGBIN="$(mktemp -d)"
for _t in bash sh env yq grep egrep awk sed tr cat head cut sort uniq dirname basename readlink mktemp; do
    _src="$(command -v "$_t" 2>/dev/null)" && ln -s "$_src" "$KEGBIN/$_t" 2>/dev/null
done
cat > "$KEGBIN/brew" <<EOF
#!/usr/bin/env bash
[ "\$1" = "--prefix" ] && [ "\$2" = "util-linux" ] && { printf '%s\n' "$KEG_ROOT"; exit 0; }
exit 0
EOF
cat > "$KEGBIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat > "$KEGBIN/gemini" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$KEGBIN/brew" "$KEGBIN/claude" "$KEGBIN/gemini"
KEGOUT="$(PATH="$KEGBIN" bash "$SYNC" 2>&1 || true)"
rm -rf "$KEGBIN" "$KEG_ROOT"
assert_contains "$KEGOUT" "KEG_SETSID_USED" "discovers keg-only util-linux setsid via 'brew --prefix' when setsid is off PATH (macOS)"

# --- post_install: dry-run shows command from fixture --------------------------
FIX="$(mktemp -d)"
cat > "$FIX/postinstall.yaml" <<'EOF'
servers:
  - name: withpostinstall
    enabled: true
    transport: stdio
    command: npx
    args: ["-y", "some-mcp@1.0.0"]
    post_install:
      - "echo POST_INSTALL_CMD"
    claude: {}
EOF
PIOUT_DRY="$(SYNC_MCP_MANIFEST="$FIX/postinstall.yaml" bash "$SYNC" --dry-run 2>&1)"
assert_contains "$PIOUT_DRY" "DRY-RUN: post_install: echo POST_INSTALL_CMD" "dry-run shows post_install command from fixture"

# --- post_install: command executes on live run ---------------------------------
cat > "$FIX/postinstall.yaml" <<'EOF'
servers:
  - name: withpostinstall
    enabled: true
    transport: stdio
    command: npx
    args: ["-y", "some-mcp@1.0.0"]
    post_install:
      - "echo POST_INSTALL_RAN"
    claude: {}
EOF
PIBIN="$(mktemp -d)"
cat > "$PIBIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$PIBIN/claude"
PIOUT_LIVE="$(SYNC_MCP_MANIFEST="$FIX/postinstall.yaml" PATH="$PIBIN:$PATH" bash "$SYNC" 2>&1)"
assert_contains "$PIOUT_LIVE" "POST_INSTALL_RAN" "post_install command executes on a live run"
rm -rf "$PIBIN"

# --- post_install: failure emits WARNING but exit 0 (non-fatal) ----------------
cat > "$FIX/postinstall_fail.yaml" <<'EOF'
servers:
  - name: failinstall
    enabled: true
    transport: stdio
    command: npx
    args: ["-y", "some-mcp@1.0.0"]
    post_install:
      - "exit 42"
    claude: {}
EOF
FAILBIN="$(mktemp -d)"
cat > "$FAILBIN/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$FAILBIN/claude"
FAILRC=0
FAILOUT="$(SYNC_MCP_MANIFEST="$FIX/postinstall_fail.yaml" PATH="$FAILBIN:$PATH" bash "$SYNC" 2>&1)" || FAILRC=$?
assert_eq "$FAILRC" "0" "post_install failure is non-fatal (exit 0)"
assert_contains "$FAILOUT" "WARNING" "post_install failure emits a WARNING"
rm -rf "$FAILBIN"

# --- post_install: absent field produces no post_install output ----------------
NOPI_DRY="$(SYNC_MCP_MANIFEST="$FIX/parked.yaml" bash "$SYNC" --dry-run 2>&1)"
assert_not_contains "$NOPI_DRY" "post_install" "no post_install output when field is absent"
rm -rf "$FIX"

echo "----"
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
