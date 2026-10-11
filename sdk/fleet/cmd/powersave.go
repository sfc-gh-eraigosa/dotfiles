package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Wi-Fi power save on THIS workstation is a cause of "unreachable" that the
// reach package models only from the other side. Its header describes a
// sleeping TARGET whose broadcast ARP a peer must chase; the mirror image is a
// sleeping WORKSTATION, whose NIC drops the broadcast ARP a cold peer sends to
// reach IT. A Raspberry Pi (brcmfmac) ships with power_save on, so a fleet run
// from one can report a perfectly healthy host as unreachable — the peer could
// not resolve the workstation's MAC to answer. We cannot see that from the
// failed probe alone, but we CAN read our own radio, and saying so turns a
// dead-end "unreachable" into the one-line fix.

// parsePowerSave reads `iw dev <d> get power_save`, whose body is a single
// "\tPower save: on" / "...: off" line. Only a line that IS that marker counts
// (trimmed, case-insensitive), so a warning that merely quotes the phrase, or
// any value other than on/off, is "don't know" (ok=false) and never raises a
// false alarm.
func parsePowerSave(out string) (on, ok bool) {
	for _, line := range strings.Split(out, "\n") {
		v, found := strings.CutPrefix(strings.ToLower(strings.TrimSpace(line)), "power save:")
		if !found {
			continue
		}
		switch strings.TrimSpace(v) {
		case "on":
			return true, true
		case "off":
			return false, true
		}
		return false, false
	}
	return false, false
}

// iwTimeout bounds one `iw` call. A wedged wireless driver can block the
// netlink request indefinitely, and this probe must never hang a command that
// has already finished its real work.
const iwTimeout = 2 * time.Second

// iwFallbacks are where Debian and Raspberry Pi OS install iw. Both are sbin
// dirs, absent from a normal user's PATH, so a bare-name exec never found it
// on the very platform (brcmfmac, power_save on by default) this targets.
var iwFallbacks = []string{"/usr/sbin/iw", "/sbin/iw"}

// findIW resolves the iw binary: PATH first, then the sbin fallbacks. "" means
// no iw, and the probe stays silent.
func findIW(lookPath func(string) (string, error), exists func(string) bool) string {
	if p, err := lookPath("iw"); err == nil {
		return p
	}
	for _, c := range iwFallbacks {
		if exists(c) {
			return c
		}
	}
	return ""
}

// radioProbe holds the impure edges of the power-save check, so the walk over
// interfaces is tested against a fake /sys tree and a fake iw.
type radioProbe struct {
	sysNet string // /sys/class/net
	iw     string // resolved iw path; "" = none
	run    func(ctx context.Context, iw, iface string) (string, error)
}

// powerSaveOn names a local wireless interface that is UP with power save ON,
// if any. Best-effort and SILENT on every platform that cannot answer:
// /sys/class/net is Linux-only (macOS reports nothing), a missing iw or a
// failing call is "don't know", and an interface that is not up is skipped —
// an idle dongle is not on the path to the fleet, so blaming it would send the
// operator to the wrong radio.
func (p radioProbe) powerSaveOn() (iface string, on bool) {
	if p.iw == "" {
		return "", false
	}
	entries, err := os.ReadDir(p.sysNet)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		name := e.Name()
		dir := filepath.Join(p.sysNet, name)
		if _, err := os.Stat(filepath.Join(dir, "wireless")); err != nil {
			continue // not a wireless interface
		}
		if st, err := os.ReadFile(filepath.Join(dir, "operstate")); err != nil || strings.TrimSpace(string(st)) != "up" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), iwTimeout)
		out, err := p.run(ctx, p.iw, name)
		cancel()
		if err != nil {
			continue
		}
		if ps, ok := parsePowerSave(out); ok && ps {
			return name, true
		}
	}
	return "", false
}

func runIW(ctx context.Context, iw, iface string) (string, error) {
	c := exec.CommandContext(ctx, iw, "dev", iface, "get", "power_save")
	c.WaitDelay = time.Second
	out, err := c.CombinedOutput()
	return string(out), err
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// localPowerSaveIface is the real probe, run at most once per process: both
// `fleet status` and the TUI's row tip ask, and the answer cannot change in a
// way that matters within one run. "" = nothing to report.
var localPowerSaveIface = sync.OnceValue(func() string {
	p := radioProbe{sysNet: "/sys/class/net", iw: findIW(exec.LookPath, fileExists), run: runIW}
	if iface, on := p.powerSaveOn(); on {
		return iface
	}
	return ""
})

// localPowerSaveNote is the impure entry point status wires in: it reads the
// radio and formats the advisory, or returns "" when there is nothing to say.
// Split from powerSaveNote so the wording is unit-tested without a radio.
func localPowerSaveNote() string {
	iface := localPowerSaveIface()
	return powerSaveNote(iface, iface != "")
}

// powerSaveNote is the pure advisory. Empty unless a named interface is on, so
// the common "no wireless / power save already off" case adds nothing.
func powerSaveNote(iface string, on bool) string {
	if !on || iface == "" {
		return ""
	}
	return fmt.Sprintf(
		"  NOTE: this workstation's Wi-Fi (%s) has power_save ON. A sleeping Wi-Fi\n"+
			"  NIC drops the broadcast ARP a cold peer must send, so peers can fail to\n"+
			"  reach THIS machine — which makes the fleet look unreachable from here.\n"+
			"  Disable it:\n"+
			"    sudo iw dev %s set power_save off                   # now\n"+
			"    sudo nmcli c m <conn> 802-11-wireless.powersave 2   # persist (or a\n"+
			"    /etc/NetworkManager/conf.d drop-in: [connection] / wifi.powersave = 2)\n",
		iface, iface)
}
