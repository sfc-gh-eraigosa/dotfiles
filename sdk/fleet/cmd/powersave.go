package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// "\tPower save: on" / "...: off" line. ok is false when neither marker
// appears, so a kernel that answers something unexpected is treated as "don't
// know" and never raises a false alarm.
func parsePowerSave(out string) (on, ok bool) {
	s := strings.ToLower(out)
	switch {
	case strings.Contains(s, "power save: on"):
		return true, true
	case strings.Contains(s, "power save: off"):
		return false, true
	default:
		return false, false
	}
}

// wifiPowerSaveOn names a local wireless interface whose power save is ON, if
// any. It is best-effort and SILENT on every platform that cannot answer:
// `/sys/class/net/<i>/wireless` is Linux-only, so macOS and a WSL box with no
// wireless NIC simply report nothing, and a missing `iw` degrades the same
// way. Detection only ever ADDS a hint — it never blocks, fails, or slows a
// command enough to matter (one `iw` call per wireless NIC, and most hosts
// have none or one).
func wifiPowerSaveOn() (iface string, on bool) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		name := e.Name()
		if _, err := os.Stat(filepath.Join("/sys/class/net", name, "wireless")); err != nil {
			continue // not a wireless interface
		}
		out, err := exec.Command("iw", "dev", name, "get", "power_save").CombinedOutput()
		if err != nil {
			continue
		}
		if ps, ok := parsePowerSave(string(out)); ok && ps {
			return name, true
		}
	}
	return "", false
}

// localPowerSaveNote is the impure entry point status wires in: it reads the
// radio and formats the advisory, or returns "" when there is nothing to say.
// Split from powerSaveNote so the wording is unit-tested without a radio.
func localPowerSaveNote() string { return powerSaveNote(wifiPowerSaveOn()) }

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
