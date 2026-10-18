# shellcheck shell=bash
# htop-fit — make `htop` open with the meter header filling the terminal and
# only the top $HTOP_PROC_ROWS processes (default 4) below it.
#
# Deployed by opt/scripts/system/install_htop.sh to ~/.config/htop/htop-fit.sh
# (gff install.tools.htop-fit) and sourced by ~/.zshrc and ~/.bashrc; edit the
# repo copy (opt/etc/htop/htop-fit.sh) and re-run the installer.
#
# htop has no setting to cap the process list, so htop() renders a throwaway
# copy of ~/.config/htop/htoprc sized to the current terminal height:
#   1. 4-row history graphs while they fit — left column CPU/Memory/Swap,
#      right column GPU (only when the GPU is readable, see below)/Network/
#      Disk/Load;
#   2. then the tallest per-core CPU graph group that still fits;
#   3. then Blank meters for any rows left over.
# htop is launched against the copy via $HTOPRC. Settings changed inside htop
# land in that copy and are discarded — run `command htop` to edit the real
# htoprc. Resizing the terminal while htop is open does not re-fit it.
#
# GPU: htop's GPU meter (and GPU% process column) read DRM fdinfo engine stats,
# which only some Linux kernel drivers expose. They are added automatically
# when such a driver is bound to a /sys/class/drm card; on WSL (/dev/dxg),
# macOS, and NVIDIA's proprietary driver the meter would sit at 0%, so it is
# left out. HTOP_FIT_GPU=1 forces it on, HTOP_FIT_GPU=0 off.
#
# Sourced by both bash and zsh: keep it to syntax both accept, and never put a
# bare glob in it (zsh aborts on a glob with no match).

command -v htop >/dev/null 2>&1 || return 0

# Succeeds when htop's GPU meter can read real utilization on this host.
_htop_fit_gpu() {
  case "${HTOP_FIT_GPU:-auto}" in
    1 | on | true) return 0 ;;
    0 | off | false) return 1 ;;
  esac
  [ "$(uname -s 2>/dev/null)" = Linux ] || return 1
  # The GPU meter and GPU_PERCENT column are new; an older htop drops them,
  # which would throw the row math off. 3.4 is the oldest verified.
  local ver
  ver=$(command htop --version 2>/dev/null | awk 'NR == 1 { print $2 }')
  case "$ver" in
    [0-2].* | 3.[0-3] | 3.[0-3].*) return 1 ;;
  esac
  local card drv
  find /sys/class/drm -maxdepth 1 -name 'card[0-9]*' 2>/dev/null |
    while IFS= read -r card; do
      drv=$(readlink "${card}/device/driver" 2>/dev/null) || continue
      case "${drv##*/}" in
        amdgpu | i915 | xe | nouveau | msm | panfrost | panthor | v3d | etnaviv | lima | asahi)
          echo yes
          break
          ;;
      esac
    done | grep -q yes
}

# _htop_fit_render RC LINES PROC_ROWS NCPU GPU(0|1) — print the fitted htoprc.
_htop_fit_render() {
  awk -v lines="$2" -v procs="$3" -v ncpu="$4" -v gpu="$5" '
    function ceil(x) { return (x == int(x)) ? x : int(x) + 1 }
    # Rows one meter takes: bar/text = 1, graph = 4, LED = 3; CPU groups stack
    # ceil(cpus / per-row) sub-meters of that height.
    function mh(name, mode,   per, cpus, cols) {
      per = (mode == 3) ? 4 : (mode == 4) ? 3 : 1
      if (name !~ /CPUs[0-9]*$/) return per
      cpus = (name ~ /^Left/) ? ceil(ncpu / 2) : (name ~ /^Right/) ? int(ncpu / 2) : ncpu
      cols = name; sub(/.*CPUs/, "", cols); if (cols == "") cols = 1
      return ceil(cpus / cols) * per
    }
    function add(c, name, mode) {
      line[mi[c]] = line[mi[c]] (line[mi[c]] ~ /=$/ ? "" : " ") name
      line[di[c]] = line[di[c]] (line[di[c]] ~ /=$/ ? "" : " ") mode
    }
    BEGIN {
      graphs[0] = "CPU Memory Swap"
      graphs[1] = (gpu ? "GPU " : "") "NetworkIO DiskIO Load"
      split("1 2 4 8", per_row, " ")
      tabs = 1; fbar = 1; margin = 1
    }
    { sub(/=!$/, "=") }   # "!" marks an empty column
    /^screen_tabs=/       { tabs = substr($0, 13) + 0 }
    /^hide_function_bar=/ { fbar = (substr($0, 19) + 0 == 2) ? 0 : 1 }   # 1 = hidden only after ESC
    /^header_margin=/     { margin = substr($0, 15) + 0 }
    /^screen:Main=/ && gpu && !/GPU_PERCENT/ { sub(/ Command$/, " GPU_PERCENT Command") }
    { line[NR] = $0 }
    /^column_meters_[0-9]+=/      { c = $0; sub(/^column_meters_/, "", c); sub(/=.*/, "", c); mi[c] = NR }
    /^column_meter_modes_[0-9]+=/ { c = $0; sub(/^column_meter_modes_/, "", c); sub(/=.*/, "", c); di[c] = NR }
    END {
      # Rows outside the header: column titles + tab bar + function bar + margin.
      target = lines - (1 + (tabs ? 1 : 0) + fbar + (margin ? 2 : 0)) - procs
      for (c in mi) {
        if (!(c in di)) continue
        v = line[mi[c]]; sub(/^[^=]*=/, "", v); split(v, m, " ")
        v = line[di[c]]; sub(/^[^=]*=/, "", v); n = split(v, d, " ")
        h = 0; for (i = 1; i <= n; i++) h += mh(m[i], d[i])
        g = split(graphs[c], gl, " ")
        for (i = 1; i <= g && h + 4 <= target; i++) { add(c, gl[i], 3); h += 4 }
        side = (c == 0) ? "Left" : (c == 1) ? "Right" : ""
        best = ""; bh = 0
        if (side != "") for (k = 1; k <= 4; k++) {
          name = side "CPUs" (per_row[k] == 1 ? "" : per_row[k])
          ch = mh(name, 3); if (ch > bh && h + ch <= target) { best = name; bh = ch }
        }
        if (best != "") { add(c, best, 3); h += bh }
        for (; h < target; h++) add(c, "Blank", 2)
      }
      for (i = 1; i <= NR; i++) print (line[i] ~ /^column_meter(s|_modes)_[0-9]+=$/ ? line[i] "!" : line[i])
    }' "$1"
}

htop() {
  local rc="${XDG_CONFIG_HOME:-$HOME/.config}/htop/htoprc"
  if [ ! -t 1 ] || [ ! -r "$rc" ] || [ -n "${HTOPRC:-}" ]; then
    command htop "$@"
    return
  fi
  local lines ncpu gpu=0 tmp
  lines=${LINES:-$(tput lines 2>/dev/null || echo 24)}
  # Not nproc: it honors OMP_NUM_THREADS and the affinity mask, htop does not.
  ncpu=$(getconf _NPROCESSORS_ONLN 2>/dev/null || nproc --all 2>/dev/null || echo 1)
  _htop_fit_gpu && gpu=1
  tmp=$(mktemp "${TMPDIR:-/tmp}/htoprc.XXXXXX") || {
    command htop "$@"
    return
  }
  _htop_fit_render "$rc" "$lines" "${HTOP_PROC_ROWS:-4}" "$ncpu" "$gpu" >"$tmp"
  # Subshell + trap: the copy is removed even when htop dies on a signal
  # (terminal or ssh session closed).
  (
    trap 'rm -f "$tmp"' EXIT HUP INT TERM
    HTOPRC=$tmp command htop "$@"
  )
}
