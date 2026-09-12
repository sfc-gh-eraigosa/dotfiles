#!/usr/bin/env bash
# Test driver for opt/scripts/system/install_rust.sh
#
# What must hold:
#   * rustup-init is verified against the SHA-256 published beside it, and a
#     missing, malformed, or mismatched checksum is REFUSED (fail-closed)
#     before rustup-init ever runs;
#   * rustup-init runs with --no-modify-path (our rc files are symlinks into
#     the repo; rustup's append would land in the working tree), the minimal
#     profile, and the requested toolchain;
#   * a host with rustup and a new-enough rustc is a cheap no-op with no
#     download (fleet update re-runs install.sh on every host);
#   * a rustc older than RUST_MIN_VERSION, or RUST_UPDATE=1, runs
#     `rustup update` instead of reinstalling.
#
# Network is never touched: RUSTUP_INIT_BASE points curl at a file:// fixture
# and rustup-init / rustup / rustc are stubs.
#
# Run: bash opt/scripts/system/install_rust_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../../.." && pwd)"
# shellcheck source=../../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

SCRIPT="${SELF_DIR}/install_rust.sh"

assert_file_exists "${SCRIPT}" "install_rust.sh exists"
assert_exit_code 0 "parses with bash -n" bash -n "${SCRIPT}"
set +e

# --- source-level: the trust model and the rc-file rule ----------------------
assert_grep "downloads rustup-init from the official dist server" 'static\.rust-lang\.org/rustup/dist' "${SCRIPT}"
assert_grep "verifies SHA-256 of the download" 'checksum mismatch' "${SCRIPT}"
assert_grep_negative "never pipes a remote script into a shell" 'curl[^|]*\|[[:space:]]*(ba)?sh' "${SCRIPT}"
assert_grep "never lets rustup edit rc files" '--no-modify-path' "${SCRIPT}"
assert_grep "probes rustc through a pinned toolchain, not the cwd-following proxy" 'RUSTUP_TOOLCHAIN="\$\{RUST_TOOLCHAIN\}" "\$\{CARGO_HOME\}/bin/rustc"' "${SCRIPT}"
assert_grep "maps the 64-bit Pi / Nano / Spark (aarch64)" 'aarch64-unknown-linux-gnu' "${SCRIPT}"
assert_grep "maps 32-bit ARM Pis" 'armv7-unknown-linux-gnueabihf' "${SCRIPT}"
assert_grep "maps Apple silicon" 'aarch64-apple-darwin' "${SCRIPT}"

# --- functional fixtures ------------------------------------------------------
TMP="$(mktemp -d "${TMPDIR:-/tmp}/install_rust_test.XXXXXX")"
trap 'rm -rf "${TMP}"' EXIT
TRIPLE="test-unknown-linux-gnu"
DIST="${TMP}/dist"
mkdir -p "${DIST}/${TRIPLE}"
LOG="${TMP}/calls.log"
export LOG

# mkrustc <version>: lays down a rustc stub that behaves like rustup's proxy —
# it refuses to run unless a toolchain is selected (RUSTUP_TOOLCHAIN), which
# is what a stray rust-toolchain.toml in the caller's cwd does to the real one.
MK="${TMP}/mkrustc"
cat > "${MK}" <<'SH'
#!/bin/sh
mkdir -p "${CARGO_HOME}/bin"
{
  echo '#!/bin/sh'
  echo '[ -n "${RUSTUP_TOOLCHAIN:-}" ] || { echo "error: toolchain not installed (proxy needs RUSTUP_TOOLCHAIN)" >&2; exit 1; }'
  echo "echo \"rustc $1 (fixture)\""
} > "${CARGO_HOME}/bin/rustc"
chmod +x "${CARGO_HOME}/bin/rustc"
SH
chmod +x "${MK}"; export MK
# A fake rustup-init: logs its args and env, then lays down rustup + rustc
# stubs the way the real one populates $CARGO_HOME/bin. The rustup stub logs
# every call and, on `update`/`default`, re-lays rustc at
# RUSTUP_UPDATED_VERSION (default 1.98.1) — so a test can make an update
# succeed or leave rustc stale.
cat > "${DIST}/${TRIPLE}/rustup-init" <<'SH'
#!/bin/sh
echo "rustup-init $* SKIP_PATH_CHECK=${RUSTUP_INIT_SKIP_PATH_CHECK:-}" >> "${LOG}"
mkdir -p "${CARGO_HOME}/bin"
{
  echo '#!/bin/sh'
  echo "echo \"rustup \$*\" >> \"${LOG}\""
  echo 'case "$1" in update|default) "${MK}" "${RUSTUP_UPDATED_VERSION:-1.98.1}" ;; esac'
} > "${CARGO_HOME}/bin/rustup"
chmod +x "${CARGO_HOME}/bin/rustup"
"${MK}" "${FAKE_RUSTC_VERSION:-1.98.1}"
SH
sha_of() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum < "$1" | awk '{ print $1 }'
    else shasum -a 256 < "$1" | awk '{ print $1 }'; fi
}
GOOD_SHA="$(sha_of "${DIST}/${TRIPLE}/rustup-init")"
# The real .sha256 is "<hash> *./rustup-init".
printf '%s *./rustup-init\n' "${GOOD_SHA}" > "${DIST}/${TRIPLE}/rustup-init.sha256"

run_rust() {
    : > "${LOG}"
    env -u RUST_UPDATE -u RUST_TOOLCHAIN -u RUST_MIN_VERSION \
        CARGO_HOME="$1" RUST_HOST_TRIPLE="${TRIPLE}" RUSTUP_INIT_BASE="file://${DIST}" \
        "${@:2}" bash "${SCRIPT}" 2>&1
}

# 1. Fresh host: verified download, rustup-init with the load-bearing flags.
CH="${TMP}/cargo-fresh"
out="$(run_rust "${CH}")"; rc=$?
assert_eq "${rc}" "0" "fresh host exits 0"
assert_eq "$(grep -c '^rustup-init -y --no-modify-path --profile minimal --default-toolchain stable ' "${LOG}")" "1" \
    "rustup-init runs non-interactively with --no-modify-path, minimal profile, stable"
assert_eq "$(grep -c 'SKIP_PATH_CHECK=yes' "${LOG}")" "1" "a distro rustc in PATH does not stop rustup-init"
assert_eq "$(printf '%s' "${out}" | grep -c 'rustc 1.98.1 .*sha256 verified')" "1" "success names the version and the verification"

# 2. Re-run with rustup + a new-enough rustc: no-op, no download at all (the
#    fixture dist is moved away to prove nothing is fetched). The rustc stub
#    only answers with RUSTUP_TOOLCHAIN set, so this also proves the probe is
#    pinned to our toolchain rather than the caller's cwd.
mv "${DIST}" "${DIST}.away"
out="$(run_rust "${CH}")"; rc=$?
assert_eq "${rc}" "0" "already-installed exits 0"
assert_eq "$(printf '%s' "${out}" | grep -c 'Rust already installed: rustc 1.98.1')" "1" "already-installed is reported"
assert_eq "$(grep -c 'rustup' "${LOG}")" "0" "already-installed runs neither rustup-init nor rustup"
mv "${DIST}.away" "${DIST}"

# 3. rustc older than RUST_MIN_VERSION: `rustup update`, made the default,
#    then re-checked — not a reinstall.
CARGO_HOME="${CH}" "${MK}" 1.90.0
out="$(run_rust "${CH}")"; rc=$?
assert_eq "${rc}" "0" "old rustc exits 0 once the update converged"
assert_eq "$(grep -c '^rustup update stable --no-self-update$' "${LOG}")" "1" "old rustc triggers rustup update"
assert_eq "$(grep -c '^rustup default stable$' "${LOG}")" "1" "the updated channel is made the default (a pinned old default would otherwise stay)"
assert_eq "$(grep -c '^rustup-init' "${LOG}")" "0" "old rustc does not rerun rustup-init"
assert_eq "$(printf '%s' "${out}" | grep -c 'older than 1.96.0')" "1" "old rustc names the minimum"
assert_eq "$(printf '%s' "${out}" | grep -c 'Rust ready: rustc 1.98.1')" "1" "success reports the rustc actually installed after the update"

# 3b. The update ran but rustc is still too old: fail, do not claim ready.
CARGO_HOME="${CH}" "${MK}" 1.90.0
out="$(run_rust "${CH}" RUSTUP_UPDATED_VERSION=1.90.0)"; rc=$?
assert_eq "${rc}" "1" "a non-converging update exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'still older than 1.96.0')" "1" "a non-converging update says so"
assert_eq "$(printf '%s' "${out}" | grep -c 'Rust ready')" "0" "a non-converging update never claims ready"
CARGO_HOME="${CH}" "${MK}" 1.98.1

# 4. Version compare is numeric per field (1.100 is newer than 1.96).
CARGO_HOME="${CH}" "${MK}" 1.100.0
out="$(run_rust "${CH}")"; rc=$?
assert_eq "$(grep -c '^rustup update' "${LOG}")" "0" "1.100.0 counts as newer than 1.96.0"

# 5. RUST_UPDATE=1 updates even when new enough; RUST_TOOLCHAIN is honoured.
out="$(run_rust "${CH}" RUST_UPDATE=1 RUST_TOOLCHAIN=beta)"; rc=$?
assert_eq "$(grep -c '^rustup update beta --no-self-update$' "${LOG}")" "1" "RUST_UPDATE=1 updates the requested toolchain"
assert_eq "$(grep -c '^rustup default beta$' "${LOG}")" "1" "RUST_TOOLCHAIN becomes the default after its update"
CARGO_HOME="${CH}" "${MK}" 1.98.1

# 6. Checksum mismatch: refused, rustup-init never runs, nothing installed.
printf '%s *./rustup-init\n' "$(printf '0%.0s' $(seq 1 64))" > "${DIST}/${TRIPLE}/rustup-init.sha256"
CH2="${TMP}/cargo-bad"
out="$(run_rust "${CH2}")"; rc=$?
assert_eq "${rc}" "1" "checksum mismatch exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'checksum mismatch')" "1" "checksum mismatch is the stated reason"
assert_eq "$(grep -c '^rustup-init' "${LOG}")" "0" "a mismatched rustup-init never runs"
assert_eq "$([ -e "${CH2}/bin/rustc" ] && echo present || echo absent)" "absent" "nothing is installed on a mismatch"

# 7. Malformed checksum file: refused before the binary is even downloaded.
printf 'not-a-checksum\n' > "${DIST}/${TRIPLE}/rustup-init.sha256"
out="$(run_rust "${TMP}/cargo-malformed")"; rc=$?
assert_eq "${rc}" "1" "malformed checksum exits 1"
assert_eq "$(printf '%s' "${out}" | grep -c 'refusing to run an unverified rustup-init')" "1" "malformed checksum is refused"

# 8. No checksum published at all: refused.
rm -f "${DIST}/${TRIPLE}/rustup-init.sha256"
out="$(run_rust "${TMP}/cargo-nosum")"; rc=$?
assert_eq "${rc}" "1" "missing checksum exits 1"
assert_eq "$(grep -c '^rustup-init' "${LOG}")" "0" "rustup-init never runs without a checksum"

# --- gff wiring -------------------------------------------------------------
INSTALL_SH="${REPO_ROOT}/install.sh"
assert_grep "features.yaml declares install.runtime.rust" 'path: install\.runtime\.rust$' "${REPO_ROOT}/.github/gff/features.yaml"
assert_grep "install.sh gates install_rust.sh on install.runtime.rust" 'gff_on install\.runtime\.rust;' "${INSTALL_SH}"
assert_eq "$(grep -E '^_IP_DEPS_FLAGS=' "${INSTALL_SH}" | grep -c -w 'INSTALL_RUNTIME_RUST')" "1" \
    "INSTALL_RUNTIME_RUST is a deps-phase key (a toolchain is an external install)"

# --- profiles own the PATH entry rustup was told not to write -----------------
for f in .profile .zshrc; do
    assert_grep "opt/profiles/${f} puts ~/.cargo/bin on PATH (guarded)" \
        'if \[ -d "\$\{HOME\}/\.cargo/bin" \]' "${REPO_ROOT}/opt/profiles/${f}"
done

_test_report
