// Package video collects GPU utilisation and memory for the status line's
// "video" segment and caches it so the render hot path never waits on a probe.
//
// # Sources, first that works
//
//  1. NVIDIA: nvidia-smi --query-gpu=utilization.gpu,memory.used,memory.total
//     --format=csv,noheader,nounits. Only the FIRST GPU (first output line) is
//     reported; summing GPUs would hide a saturated card behind idle ones and
//     make "used/max" meaningless across mixed cards.
//  2. WSL (detected via /dev/dxg or "microsoft" in /proc/version): Windows
//     performance counters through powershell.exe. See wslScript for the
//     heuristics.
//  3. Linux DRM sysfs: /sys/class/drm/card*/device/{gpu_busy_percent,
//     mem_info_vram_used,mem_info_vram_total} (amdgpu, some Intel).
//
// Anything else (macOS, Jetson, no GPU) yields ErrNoSource and the segment is
// simply hidden.
package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ErrNoSource means no GPU data source worked on this host.
var ErrNoSource = errors.New("video: no GPU data source available")

// Stats is one GPU reading. TotalBytes may be 0 when the source reports only
// utilisation (some DRM drivers).
type Stats struct {
	Util       int    `json:"util"` // 0..100
	UsedBytes  uint64 `json:"used_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
	Source     string `json:"source"` // "nvidia" | "wsl" | "drm"
}

const mib = 1 << 20

func clampPct(v float64) int {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return int(v + 0.5)
}

// ParseNvidia parses nvidia-smi csv (noheader,nounits) output: "12, 2845, 16384"
// (MiB). Only the first GPU line is used.
func ParseNvidia(out []byte) (Stats, error) {
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	parts := strings.Split(line, ",")
	if len(parts) != 3 {
		return Stats{}, fmt.Errorf("video: unexpected nvidia-smi output %q", line)
	}
	var v [3]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil || f < 0 {
			// "[N/A]" on some boards/WSL drivers, or garbage.
			return Stats{}, fmt.Errorf("video: bad nvidia-smi field %q", p)
		}
		v[i] = f
	}
	return Stats{Util: clampPct(v[0]), UsedBytes: uint64(v[1] * mib), TotalBytes: uint64(v[2] * mib), Source: "nvidia"}, nil
}

// ParseWSL parses the single "util|usedBytes|maxBytes" line wslScript prints.
func ParseWSL(out []byte) (Stats, error) {
	// PowerShell may emit extra lines (progress, warnings); take the last
	// non-empty one.
	var line string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			line = l
		}
	}
	parts := strings.Split(line, "|")
	if len(parts) != 3 {
		return Stats{}, fmt.Errorf("video: unexpected powershell output %q", line)
	}
	u, err1 := strconv.ParseFloat(parts[0], 64)
	used, err2 := strconv.ParseUint(parts[1], 10, 64)
	tot, err3 := strconv.ParseUint(parts[2], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return Stats{}, fmt.Errorf("video: bad powershell output %q", line)
	}
	return Stats{Util: clampPct(u), UsedBytes: used, TotalBytes: tot, Source: "wsl"}, nil
}

// wslScript prints "util|usedBytes|maxBytes". Heuristics:
//   - util = sum of every 3D-engine utilisation counter (all processes), capped
//     at 100 (the per-process counters add up to the adapter's 3D load).
//   - used = sum of Dedicated Usage when > 0 (discrete GPU), else sum of Shared
//     Usage (integrated GPUs such as Intel Arc Graphics live in shared memory).
//   - max  = largest Win32_VideoController.AdapterRAM when dedicated is in use
//     (never below used; AdapterRAM is a uint32 that saturates near 4 GB),
//     else half of physical RAM, which is Windows' default shared-GPU-memory
//     limit.
//
// Only single quotes are used so the string survives WSL->Windows argument
// re-quoting.
const wslScript = `$ErrorActionPreference='SilentlyContinue';` +
	`$c=(Get-Counter -Counter '\GPU Engine(*engtype_3D)\Utilization Percentage','\GPU Adapter Memory(*)\Dedicated Usage','\GPU Adapter Memory(*)\Shared Usage').CounterSamples;` +
	`$u=($c|?{$_.Path -like '*engtype_3d*'}|Measure-Object CookedValue -Sum).Sum;` +
	`$d=($c|?{$_.Path -like '*dedicated usage'}|Measure-Object CookedValue -Sum).Sum;` +
	`$s=($c|?{$_.Path -like '*shared usage'}|Measure-Object CookedValue -Sum).Sum;` +
	`$r=(Get-CimInstance Win32_VideoController|Measure-Object AdapterRAM -Maximum).Maximum;` +
	`$t=(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory;` +
	`if($d -gt 0){$used=$d;$max=[Math]::Max($r,$d)}else{$used=$s;$max=[Math]::Floor($t/2)};` +
	`if($u -gt 100){$u=100};` +
	`@([int][Math]::Round($u),[long]$used,[long]$max) -join '|'`

// ReadDRM reads the first DRM card under root (normally /sys/class/drm) that
// exposes gpu_busy_percent.
func ReadDRM(root string) (Stats, error) {
	cards, _ := filepath.Glob(filepath.Join(root, "card[0-9]*", "device"))
	for _, dev := range cards {
		busy, ok := readNum(filepath.Join(dev, "gpu_busy_percent"))
		if !ok {
			continue
		}
		s := Stats{Util: clampPct(float64(busy)), Source: "drm"}
		if used, ok := readNum(filepath.Join(dev, "mem_info_vram_used")); ok {
			s.UsedBytes = used
		}
		if tot, ok := readNum(filepath.Join(dev, "mem_info_vram_total")); ok {
			s.TotalBytes = tot
		}
		return s, nil
	}
	return Stats{}, ErrNoSource
}

func readNum(path string) (uint64, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	n, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	return n, err == nil
}

// Collector gathers one reading. All fields are injectable for tests.
type Collector struct {
	Runner  Runner
	DRMRoot string      // default /sys/class/drm
	IsWSL   func() bool // default: /dev/dxg exists or /proc/version mentions microsoft
}

// Collect tries each source in order and returns the first that works.
func (c Collector) Collect(ctx context.Context) (Stats, error) {
	if c.Runner != nil {
		if out, err := c.Runner.Run(ctx, "nvidia-smi",
			"--query-gpu=utilization.gpu,memory.used,memory.total", "--format=csv,noheader,nounits"); err == nil {
			if s, perr := ParseNvidia(out); perr == nil {
				return s, nil
			}
		}
		isWSL := c.IsWSL
		if isWSL == nil {
			isWSL = DetectWSL
		}
		if isWSL() {
			if out, err := c.Runner.Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", wslScript); err == nil {
				if s, perr := ParseWSL(out); perr == nil {
					return s, nil
				}
			}
		}
	}
	root := c.DRMRoot
	if root == "" {
		root = "/sys/class/drm"
	}
	return ReadDRM(root)
}

// DetectWSL reports whether this is a WSL guest.
func DetectWSL() bool {
	if _, err := os.Stat("/dev/dxg"); err == nil {
		return true
	}
	b, err := os.ReadFile("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// HumanBytes formats a byte count as "512M", "2.8G", "16G": one decimal below
// 10G, none above, and a trailing ".0" dropped.
func HumanBytes(b uint64) string {
	const gib = 1 << 30
	if b < gib {
		return fmt.Sprintf("%dM", b/mib)
	}
	g := float64(b) / gib
	if g < 10 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", g), ".0") + "G"
	}
	return fmt.Sprintf("%.0fG", g)
}
