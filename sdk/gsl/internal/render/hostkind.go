package render

// hostkind.go — what kind of machine is this?
//
// The host segment picks its glyph from a coarse hardware/platform class so a
// status line glanced at across a fleet says "this is the Pi" or "this is the
// DGX" before the hostname is even read. Classification is split in two:
//
//   - probeHost reads the handful of files that answer the question (all
//     cheap, all optional, no subprocesses) into a hostProbe;
//   - classifyHost turns a hostProbe into a kind. It is PURE so every rule is
//     unit-testable without a Raspberry Pi on the desk.
//
// Precedence, most specific first: macOS (GOOS) → container → WSL → device
// tree (Pi / Jetson) → DMI (DGX) → generic Linux. A container wins over the
// hardware it runs on because "inside a container" is what the user needs to
// know about the shell they are typing into.

import (
	"os"
	"runtime"
	"strings"
)

// Host kinds. The string is also the config `kind` option value and the suffix
// of the per-kind icon key ("host_" + kind).
const (
	hostKindMacOS     = "macos"
	hostKindWSL       = "wsl"
	hostKindRPi       = "rpi"
	hostKindJetson    = "jetson"
	hostKindDGX       = "dgx"
	hostKindContainer = "container"
	hostKindLinux     = "linux"
)

// hostKinds lists every kind classifyHost can return, for option validation.
var hostKinds = []string{
	hostKindMacOS, hostKindWSL, hostKindRPi, hostKindJetson, hostKindDGX, hostKindContainer, hostKindLinux,
}

// validHostKind reports whether s is a known kind (exact, case-sensitive).
func validHostKind(s string) bool {
	for _, k := range hostKinds {
		if s == k {
			return true
		}
	}
	return false
}

// hostProbe is the raw evidence classifyHost decides on. Every field is
// optional: an unreadable source is simply left empty.
type hostProbe struct {
	// GOOS is runtime.GOOS ("darwin", "linux", …).
	GOOS string
	// ProcVersion is /proc/version (WSL kernels mention "microsoft").
	ProcVersion string
	// DeviceTreeModel is /proc/device-tree/model (ARM boards: Pi, Jetson).
	DeviceTreeModel string
	// DMIVendor is /sys/class/dmi/id/sys_vendor (x86/arm64 servers, e.g. "NVIDIA").
	DMIVendor string
	// DMIProduct is /sys/class/dmi/id/product_name (e.g. "DGX Spark").
	DMIProduct string
	// InContainer is true when /.dockerenv or /run/.containerenv exists.
	InContainer bool
}

// classifyHost maps a probe to a kind. Pure; see the precedence note above.
func classifyHost(p hostProbe) string {
	if p.GOOS == "darwin" {
		return hostKindMacOS
	}
	if p.InContainer {
		return hostKindContainer
	}
	if strings.Contains(strings.ToLower(p.ProcVersion), "microsoft") {
		return hostKindWSL
	}
	model := strings.ToLower(p.DeviceTreeModel)
	switch {
	case strings.Contains(model, "raspberry pi"):
		return hostKindRPi
	case strings.Contains(model, "jetson"):
		return hostKindJetson
	}
	dmi := strings.ToLower(p.DMIVendor + " " + p.DMIProduct)
	if strings.Contains(dmi, "dgx") || strings.HasPrefix(strings.ToLower(p.DMIVendor), "nvidia") {
		return hostKindDGX
	}
	return hostKindLinux
}

// probeHost gathers the evidence from the running machine. Each read is
// best-effort; a missing file leaves its field empty. No subprocesses.
func probeHost() hostProbe {
	return hostProbe{
		GOOS:            runtime.GOOS,
		ProcVersion:     readSmallFile("/proc/version"),
		DeviceTreeModel: readSmallFile("/proc/device-tree/model"),
		DMIVendor:       readSmallFile("/sys/class/dmi/id/sys_vendor"),
		DMIProduct:      readSmallFile("/sys/class/dmi/id/product_name"),
		InContainer:     fileExists("/.dockerenv") || fileExists("/run/.containerenv"),
	}
}

// readSmallFile returns the trimmed contents of path, or "" when unreadable.
// The sysfs/procfs files involved are a few bytes; the cap is a safety net.
func readSmallFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) > 512 {
		b = b[:512]
	}
	return strings.TrimSpace(strings.TrimRight(string(b), "\x00"))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
