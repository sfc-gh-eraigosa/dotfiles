#!/usr/bin/env bash
# Test driver for the herdr skill's bundled tools:
#   scripts/herdr-layout  save/restore/list/show/delete named tab layouts
#                         (host-local JSON under ~/.config/herdr/layouts) via
#                         the socket API's layout.export / layout.apply
#   scripts/herdr-prefs   host-local config.toml edits that take ownership
#                         away from install.sh (drops the managed marker)
#
# What must hold:
#   * the socket is never touched unless HERDR_SOCKET_PATH (or the default
#     socket) exists; a missing socket is a clear error, not a hang;
#   * save writes portable JSON: ephemeral pane_ids are stripped, the tree,
#     zoom state, and a saved_at stamp are kept;
#   * restore sends layout.apply with the saved root and the caller's options;
#   * a server error is surfaced with its code and exits non-zero;
#   * prefs set edits exactly one key in one section, preserves comments,
#     removes the "managed by dotfiles" marker (host takes ownership), and
#     reloads a running server best-effort;
#   * prefs reset re-renders the managed file via install_herdr.sh config.
#
# The herdr server is a fake: a one-request-per-connection newline-JSON unix
# socket server (python3) that records what it received.
#
# Run: bash ai/skills/herdr/scripts/herdr-skill_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../../.." && pwd)"
# shellcheck source=../../../_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

LAYOUT="${SELF_DIR}/herdr-layout"
PREFS="${SELF_DIR}/herdr-prefs"
SKILL="${SELF_DIR}/../SKILL.md"

assert_file_exists "${LAYOUT}" "herdr-layout exists"
assert_file_exists "${PREFS}" "herdr-prefs exists"
assert_file_exists "${SKILL}" "SKILL.md exists"
assert_exit_code 0 "herdr-layout parses" bash -n "${LAYOUT}"
assert_exit_code 0 "herdr-prefs parses" bash -n "${PREFS}"
set +e

TMP="$(mktemp -d "${TMPDIR:-/tmp}/herdr_skill_test.XXXXXX")"
trap 'rm -rf "${TMP}"; [ -n "${SRV_PID:-}" ] && kill "${SRV_PID}" 2>/dev/null' EXIT

# --- fake herdr socket server ------------------------------------------------
SOCK="${TMP}/herdr.sock"
LOG="${TMP}/requests.jsonl"
cat > "${TMP}/fake_server.py" <<'PY'
import socket, json, os, sys
path, log = sys.argv[1], sys.argv[2]
srv = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
srv.bind(path); srv.listen(8)
while True:
    conn, _ = srv.accept()
    buf = b""
    while not buf.endswith(b"\n"):
        c = conn.recv(65536)
        if not c: break
        buf += c
    req = json.loads(buf.decode() or "{}")
    with open(log, "a") as fh: fh.write(json.dumps(req) + "\n")
    m, p = req.get("method"), req.get("params", {})
    if m == "layout.export":
        if p.get("tab_id") == "w9:t9":
            res = {"id": req["id"], "error": {"code": "layout_not_found", "message": "layout target not found"}}
        else:
            res = {"id": req["id"], "result": {"type": "layout_export", "layout": {
                "workspace_id": "w1", "tab_id": "w1:t1", "zoomed": False, "focused_pane_id": "w1:p2",
                "root": {"type": "split", "direction": "right", "ratio": 0.6,
                         "first": {"type": "pane", "pane_id": "w1:p1", "cwd": "/repo", "label": "editor"},
                         "second": {"type": "pane", "pane_id": "w1:p2", "cwd": "/repo",
                                    "command": ["sh", "-c", "make test"]}}}}}
    elif m == "layout.apply":
        # Real 0.8.2 shape: the applied layout comes back with the NEW tab id.
        res = {"id": req["id"], "result": {"type": "layout_apply", "layout": {
            "workspace_id": p.get("workspace_id") or "w1", "tab_id": "w1:t2", "zoomed": False,
            "focused_pane_id": "w1:p3", "root": p["root"]}}}
    elif m == "server.reload_config":
        res = {"id": req["id"], "result": {"type": "config_reload", "status": "applied", "diagnostics": []}}
    else:
        res = {"id": req["id"], "error": {"code": "unknown_method", "message": m}}
    conn.sendall((json.dumps(res) + "\n").encode()); conn.close()
PY
python3 "${TMP}/fake_server.py" "${SOCK}" "${LOG}" &
SRV_PID=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do [ -S "${SOCK}" ] && break; sleep 0.1; done
[ -S "${SOCK}" ] || { echo "FAIL: fake server did not start"; FAIL=$((FAIL+1)); }

LAYOUTS="${TMP}/layouts"
run_layout() { HERDR_SOCKET_PATH="${SOCK}" HERDR_LAYOUT_DIR="${LAYOUTS}" bash "${LAYOUT}" "$@"; }

# 1. No socket -> clear error, non-zero, nothing written.
out="$(HERDR_SOCKET_PATH="${TMP}/missing.sock" HERDR_LAYOUT_DIR="${LAYOUTS}" bash "${LAYOUT}" save nope 2>&1)"; rc=$?
assert_eq "${rc}" "1" "layout save: missing socket exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'no herdr server socket')" "1" "layout save: missing socket is explained"
assert_eq "$(ls "${LAYOUTS}" 2>/dev/null | wc -l | tr -d ' ')" "0" "layout save: missing socket writes nothing"

# 2. save <name>: exports the active tab, strips pane_ids, keeps the tree.
out="$(run_layout save review 2>&1)"; rc=$?
assert_eq "${rc}" "0" "layout save: exits 0"
assert_file_exists "${LAYOUTS}/review.json" "layout save: writes <dir>/<name>.json"
assert_eq "$(tail -1 "${LOG}" | jq -r .method)" "layout.export" "layout save: calls layout.export"
assert_eq "$(jq -r '.root.type' "${LAYOUTS}/review.json")" "split" "layout save: keeps the split tree"
assert_eq "$(jq -r '.root.second.command | join(" ")' "${LAYOUTS}/review.json")" "sh -c make test" "layout save: keeps pane commands"
assert_eq "$(jq -r '.root.first.label' "${LAYOUTS}/review.json")" "editor" "layout save: keeps pane labels"
assert_eq "$(grep -c pane_id "${LAYOUTS}/review.json")" "0" "layout save: strips ephemeral pane_ids"
assert_eq "$(jq -r '.zoomed' "${LAYOUTS}/review.json")" "false" "layout save: records zoom state"
assert_eq "$(jq -r '.name' "${LAYOUTS}/review.json")" "review" "layout save: records the name"
assert_eq "$(jq -r '.saved_at | length > 0' "${LAYOUTS}/review.json")" "true" "layout save: records saved_at"
assert_eq "$(printf '%s' "${out}" | grep -c 'review.json')" "1" "layout save: prints the file path"

# 3. save --tab ID forwards the target; a server error is surfaced.
run_layout save other --tab w1:t1 >/dev/null 2>&1
assert_eq "$(tail -1 "${LOG}" | jq -r .params.tab_id)" "w1:t1" "layout save --tab: forwards tab_id"
out="$(run_layout save bad --tab w9:t9 2>&1)"; rc=$?
assert_eq "${rc}" "1" "layout save: server error exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'layout_not_found')" "1" "layout save: server error code is shown"
assert_eq "$([ -e "${LAYOUTS}/bad.json" ] && echo yes || echo no)" "no" "layout save: server error writes nothing"

# 4. Names are validated (they become file names).
out="$(run_layout save '../evil' 2>&1)"; rc=$?
assert_eq "${rc}" "1" "layout save: path-like name is rejected"

# 5. list / show / delete.
assert_eq "$(run_layout list 2>&1 | sort | tr '\n' ' ')" "other review " "layout list: names, one per line"
assert_eq "$(run_layout show review | jq -r .name)" "review" "layout show: prints the JSON"
out="$(run_layout show missing 2>&1)"; rc=$?
assert_eq "${rc}" "1" "layout show: unknown name exits 1"
run_layout delete other >/dev/null 2>&1
assert_eq "$([ -e "${LAYOUTS}/other.json" ] && echo yes || echo no)" "no" "layout delete: removes the file"

# 6. restore <name>: layout.apply with the saved root; options forwarded.
out="$(run_layout restore review 2>&1)"; rc=$?
assert_eq "${rc}" "0" "layout restore: exits 0"
assert_eq "$(tail -1 "${LOG}" | jq -r .method)" "layout.apply" "layout restore: calls layout.apply"
assert_eq "$(tail -1 "${LOG}" | jq -r .params.root.direction)" "right" "layout restore: sends the saved root"
assert_eq "$(tail -1 "${LOG}" | jq -r .params.tab_label)" "review" "layout restore: tab label defaults to the name"
assert_eq "$(tail -1 "${LOG}" | jq -r .params.focus)" "false" "layout restore: does not steal focus by default"
assert_eq "$(printf '%s' "${out}" | grep -c 'w1:t2')" "1" "layout restore: reports the new tab id"
run_layout restore review --workspace w2 --label dev --focus --replace-tab w1:t1 >/dev/null 2>&1
assert_eq "$(tail -1 "${LOG}" | jq -r '[.params.workspace_id, .params.tab_label, .params.focus, .params.tab_id] | join(" ")')" \
    "w2 dev true w1:t1" "layout restore: forwards --workspace/--label/--focus/--replace-tab"

# --- herdr-prefs --------------------------------------------------------------
CFG="${TMP}/cfg"; mkdir -p "${CFG}"
run_prefs() { HERDR_SOCKET_PATH="${SOCK}" HERDR_CONFIG_DIR="${CFG}" bash "${PREFS}" "$@"; }
# Seed the managed file exactly as install_herdr.sh config would.
HERDR_CONFIG_DIR="${CFG}" HERDR_INSTALL_DIR="${TMP}/nobin" bash "${REPO_ROOT}/opt/scripts/system/install_herdr.sh" config >/dev/null 2>&1
assert_grep "prefs fixture: seeded managed config" '^# managed by dotfiles' "${CFG}/config.toml"

# 7. status: reports managed vs host-owned and the live values.
out="$(run_prefs status 2>&1)"
assert_eq "$(printf '%s' "${out}" | grep -c 'ownership: managed')" "1" "prefs status: managed file is reported"
assert_eq "$(printf '%s' "${out}" | grep -c 'theme.name = solarized')" "1" "prefs status: shows theme.name"
assert_eq "$(printf '%s' "${out}" | grep -c 'keys.prefix = ctrl+a')" "1" "prefs status: shows keys.prefix"
assert_eq "$(printf '%s' "${out}" | grep -c 'host appearance: unknown')" "1" "prefs status: host appearance is unknown without a tty (never guessed)"

# 8. get / set on an existing key in an existing section.
assert_eq "$(run_prefs get theme.name)" "solarized" "prefs get: reads a string"
assert_eq "$(run_prefs get theme.auto_switch)" "true" "prefs get: reads a boolean"
out="$(run_prefs set theme.name nord 2>&1)"; rc=$?
assert_eq "${rc}" "0" "prefs set: exits 0"
assert_grep "prefs set: rewrites the key in place" '^name = "nord"$' "${CFG}/config.toml"
assert_eq "$(grep -c '^name = ' "${CFG}/config.toml")" "1" "prefs set: exactly one name key remains"
assert_grep "prefs set: other keys untouched" '^light_name = "solarized-light"$' "${CFG}/config.toml"
assert_grep "prefs set: comments preserved" '^# tmux-style prefix on ctrl\+a' "${CFG}/config.toml"
assert_grep_negative "prefs set: managed marker removed (host owns the file now)" 'managed by dotfiles' "${CFG}/config.toml"
assert_grep "prefs set: ownership handoff is recorded in the file" '^# host-owned' "${CFG}/config.toml"
assert_eq "$(tail -1 "${LOG}" | jq -r .method)" "server.reload_config" "prefs set: reloads the running server"
assert_eq "$(printf '%s' "${out}" | grep -c 'host-owned')" "1" "prefs set: tells the user install.sh will leave it alone"
assert_eq "$(run_prefs status 2>&1 | grep -c 'ownership: host-owned')" "1" "prefs status: host-owned after set"

# 9. set: booleans/numbers unquoted, strings quoted; new key appended to its section; new section created.
run_prefs set theme.auto_switch false >/dev/null 2>&1
assert_grep "prefs set: boolean stays unquoted" '^auto_switch = false$' "${CFG}/config.toml"
run_prefs set ui.sidebar_width 30 >/dev/null 2>&1
assert_grep "prefs set: creates a missing section" '^\[ui\]$' "${CFG}/config.toml"
assert_grep "prefs set: number stays unquoted" '^sidebar_width = 30$' "${CFG}/config.toml"
run_prefs set keys.new_tab prefix+c >/dev/null 2>&1
assert_grep "prefs set: new key lands in its section" '^new_tab = "prefix\+c"$' "${CFG}/config.toml"
assert_eq "$(awk '/^\[keys\]/{f=1;next} /^\[/{f=0} f && /^new_tab/{print "in-keys"}' "${CFG}/config.toml")" "in-keys" \
    "prefs set: appended key is inside [keys], not at EOF"
out="$(run_prefs set nodots value 2>&1)"; rc=$?
assert_eq "${rc}" "1" "prefs set: key must be section.key"

# 10. reset: re-renders the managed file (install_herdr.sh config, forced), keeping a .bak.
out="$(HERDR_INSTALL_DIR="${TMP}/nobin" run_prefs reset 2>&1)"; rc=$?
assert_eq "${rc}" "0" "prefs reset: exits 0"
assert_grep "prefs reset: managed marker is back" '^# managed by dotfiles' "${CFG}/config.toml"
assert_grep "prefs reset: fleet theme is back" '^name = "solarized"$' "${CFG}/config.toml"
assert_grep "prefs reset: the host-owned file was backed up" '^name = "nord"$' "${CFG}/config.toml.bak"

# 11. reset through a symlinked skill folder (how sync-skills installs it) still
#     finds the installer in the physical checkout.
ln -s "${SELF_DIR}" "${TMP}/skill-link"
run_prefs set theme.name dracula >/dev/null 2>&1
out="$(HERDR_SOCKET_PATH="${SOCK}" HERDR_CONFIG_DIR="${CFG}" HERDR_INSTALL_DIR="${TMP}/nobin" bash "${TMP}/skill-link/herdr-prefs" reset 2>&1)"; rc=$?
assert_eq "${rc}" "0" "prefs reset via symlinked skill dir: exits 0"
assert_grep "prefs reset via symlinked skill dir: managed marker is back" '^# managed by dotfiles' "${CFG}/config.toml"

# --- SKILL.md shape -------------------------------------------------------------
assert_grep "SKILL.md: frontmatter name is herdr" '^name: herdr$' "${SKILL}"
assert_grep "SKILL.md: description starts with a trigger, not a workflow" '^description: Use when' "${SKILL}"
assert_grep "SKILL.md: gates on HERDR_ENV like the upstream skill" 'HERDR_ENV' "${SKILL}"
assert_grep "SKILL.md: documents layout save" 'herdr-layout save' "${SKILL}"
assert_grep "SKILL.md: documents prefs set" 'herdr-prefs set' "${SKILL}"
assert_grep "SKILL.md: warns that restore does not carry running processes" 'not preserve live' "${SKILL}"
assert_grep "SKILL.md: says layouts are host-local, not in the repo" 'never in the dotfiles repo' "${SKILL}"
assert_file_exists "${SELF_DIR}/../evals/evals.json" "evals corpus exists"

_test_report
