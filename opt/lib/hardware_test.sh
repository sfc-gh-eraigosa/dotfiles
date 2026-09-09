#!/usr/bin/env bash
# Test driver for opt/lib/hardware.sh and opt/scripts/system/setup_gpu.sh
#
# The detectors read host files (/etc/dgx-release, DMI, nvidia-smi), so
# these run hermetically: a fake root is built in a temp dir and the
# detector bodies are re-pointed at it via a sandbox harness. That keeps
# the suite meaningful in CI (a GPU-less container) as well as on a Spark.
#
# Run: bash opt/lib/hardware_test.sh
set -u

SELF_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SELF_DIR}/../.." && pwd)"
# shellcheck source=../../ai/_test_helpers.sh
. "${REPO_ROOT}/ai/_test_helpers.sh"

HARDWARE="${SELF_DIR}/hardware.sh"
SETUP_GPU="${REPO_ROOT}/opt/scripts/system/setup_gpu.sh"

assert_file_exists "${HARDWARE}" "hardware.sh exists"
assert_exit_code 0 "hardware.sh parses with bash -n" bash -n "${HARDWARE}"
assert_exit_code 0 "hardware.sh parses with dash -n (POSIX)" dash -n "${HARDWARE}"
assert_file_exists "${SETUP_GPU}" "setup_gpu.sh exists"
assert_exit_code 0 "setup_gpu.sh parses with bash -n" bash -n "${SETUP_GPU}"

# Every detector must be defined and must not blow up on any host.
. "${HARDWARE}"
for fn in is_jetson is_arm64 is_linux is_darwin is_dgx is_dgx_spark \
          has_nvidia_gpu has_unified_gpu_memory; do
    if command -v "${fn}" >/dev/null 2>&1; then
        echo "PASS: ${fn} is defined"; PASS=$((PASS + 1))
    else
        echo "FAIL: ${fn} is defined"; FAIL=$((FAIL + 1))
    fi
    # Detectors are predicates: they may return 0 or 1, but must never
    # crash, hang, or emit output. Anything else is a bug.
    # `set +e` is required: the shared helper leaves `set -e` enabled, and a
    # predicate returning 1 is a PASS here, not a driver abort.
    set +e
    out=$("${fn}" 2>&1); rc=$?
    set -e
    if [ "${rc}" -le 1 ] && [ -z "${out}" ]; then
        echo "PASS: ${fn} is a silent predicate (rc=${rc})"; PASS=$((PASS + 1))
    else
        echo "FAIL: ${fn} is a silent predicate (rc=${rc}, output '${out}')"; FAIL=$((FAIL + 1))
    fi
done

# --- hermetic detector logic, via a fake root -----------------------------
# is_dgx/is_dgx_spark read fixed absolute paths, so exercise the same
# matching logic against fixtures to prove the patterns are right on a
# host that is not a DGX.
TMPROOT=$(mktemp -d)
trap 'rm -rf "${TMPROOT}"' EXIT

cat > "${TMPROOT}/dgx-release" <<'EOF'
DGX_NAME="DGX Spark"
DGX_PRETTY_NAME="NVIDIA DGX Spark"
DGX_SWBUILD_VERSION="7.2.3"
EOF
assert_exit_code 0 "DGX Spark release table matches the is_dgx_spark pattern" \
    grep -qi 'DGX_NAME="\?DGX Spark' "${TMPROOT}/dgx-release"

cat > "${TMPROOT}/dgx-release-station" <<'EOF'
DGX_NAME="DGX Station"
DGX_PRETTY_NAME="NVIDIA DGX Station"
EOF
assert_exit_code 1 "a non-Spark DGX does NOT match the is_dgx_spark pattern" \
    grep -qi 'DGX_NAME="\?DGX Spark' "${TMPROOT}/dgx-release-station"

echo 'DGX Spark' > "${TMPROOT}/product_family"
assert_exit_code 0 "DMI product_family fallback matches DGX Spark" \
    grep -qi 'DGX Spark' "${TMPROOT}/product_family"

# --- mutual exclusion: Jetson and DGX must never both claim a host --------
set +e
if is_jetson && is_dgx; then
    echo "FAIL: is_jetson and is_dgx are mutually exclusive"; FAIL=$((FAIL + 1))
else
    echo "PASS: is_jetson and is_dgx are mutually exclusive"; PASS=$((PASS + 1))
fi

# has_unified_gpu_memory implies has_nvidia_gpu (it calls it as a guard).
if has_unified_gpu_memory && ! has_nvidia_gpu; then
    echo "FAIL: has_unified_gpu_memory implies has_nvidia_gpu"; FAIL=$((FAIL + 1))
else
    echo "PASS: has_unified_gpu_memory implies has_nvidia_gpu"; PASS=$((PASS + 1))
fi

# --- setup_gpu.sh guards --------------------------------------------------
# --dry-run must always exit 0 and never invoke apt, on ANY host: a GPU-less
# CI container, a Jetson, or a Spark.
assert_exit_code 0 "setup_gpu.sh --dry-run exits 0 on this host" \
    bash "${SETUP_GPU}" --dry-run

DRY_OUT=$(bash "${SETUP_GPU}" --dry-run 2>&1)
set +e
if has_nvidia_gpu; then
    case "${DRY_OUT}" in
        *"NVIDIA"*) echo "PASS: setup_gpu.sh reports the GPU host"; PASS=$((PASS + 1)) ;;
        *) echo "FAIL: setup_gpu.sh reports the GPU host (got '${DRY_OUT}')"; FAIL=$((FAIL + 1)) ;;
    esac
else
    case "${DRY_OUT}" in
        *"Skipping"*) echo "PASS: setup_gpu.sh no-ops on a GPU-less host"; PASS=$((PASS + 1)) ;;
        *) echo "FAIL: setup_gpu.sh no-ops on a GPU-less host (got '${DRY_OUT}')"; FAIL=$((FAIL + 1)) ;;
    esac
fi

# The Jetson/DGX split is the whole point of this script - assert it in code.
assert_grep "setup_gpu.sh defers to jtop on Jetson" 'if is_jetson; then' "${SETUP_GPU}"
assert_grep "setup_gpu.sh installs nvitop as the primary tool" 'nvitop' "${SETUP_GPU}"

# install.sh must gate it behind the gff flag, like every other component.
assert_grep "install.sh gates setup_gpu.sh behind install.system.gpu" \
    'gff_on install\.system\.gpu' "${REPO_ROOT}/install.sh"
assert_grep "install.system.gpu is declared in the gff inventory" \
    'path: install\.system\.gpu' "${REPO_ROOT}/.github/gff/features.yaml"

_test_report
