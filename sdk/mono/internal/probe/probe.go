// Package probe decides which program each pane gets on this machine: the
// pane's command or its first usable fallback, where "usable" means it is on
// PATH and, for tools that only work with certain hardware, that the hardware
// is there (nvtop on a host it cannot read would be an empty pane).
package probe

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/mono/internal/config"
)

// Timeout caps one probe command.
const Timeout = 3 * time.Second

// Env is what probing needs from the host. System is the real one; tests fake it.
type Env interface {
	LookPath(file string) (string, error)
	// Run runs a shell command line and reports whether it exited 0.
	Run(ctx context.Context, cmdline string) bool
	// DRMDrivers lists the kernel driver bound to each /sys/class/drm card.
	DRMDrivers() []string
}

// System probes the real host.
type System struct{}

func (System) LookPath(f string) (string, error) { return exec.LookPath(f) }

func (System) Run(ctx context.Context, cmdline string) bool {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdline)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	return cmd.Run() == nil
}

func (System) DRMDrivers() []string {
	cards, _ := filepath.Glob("/sys/class/drm/card[0-9]*")
	var out []string
	for _, c := range cards {
		if strings.Contains(filepath.Base(c), "-") { // card0-HDMI-A-1 connectors
			continue
		}
		if l, err := os.Readlink(filepath.Join(c, "device", "driver")); err == nil {
			out = append(out, filepath.Base(l))
		}
	}
	return out
}

// drmGPUDrivers are kernel drivers whose engine stats nvtop can read.
var drmGPUDrivers = map[string]bool{
	"amdgpu": true, "i915": true, "xe": true, "nouveau": true, "msm": true,
	"panfrost": true, "panthor": true, "v3d": true,
}

// gpuTools need a readable GPU: NVIDIA's driver tools, or a DRM driver above.
var gpuTools = map[string]string{"nvtop": "any", "nvitop": "nvidia", "gpustat": "nvidia"}

// Choice is the outcome for one pane.
type Choice struct {
	Pane   config.Pane
	Cmd    string // the command that will run; "" = pane dropped
	Reason string // why Cmd was chosen, or why each candidate was refused
}

// Usable reports whether the pane will run.
func (c Choice) Usable() bool { return c.Cmd != "" }

// Choose picks a command for every pane, in config order.
func Choose(ctx context.Context, env Env, panes []config.Pane) []Choice {
	out := make([]Choice, len(panes))
	for i, p := range panes {
		out[i] = choose(ctx, env, p)
	}
	return out
}

func choose(ctx context.Context, env Env, p config.Pane) Choice {
	var refused []string
	for _, cand := range append([]string{p.Cmd}, p.Fallbacks...) {
		ok, why := usable(ctx, env, p, cand)
		if ok {
			reason := "found " + Binary(cand)
			if len(refused) > 0 {
				reason += " (" + strings.Join(refused, "; ") + ")"
			}
			return Choice{Pane: p, Cmd: cand, Reason: reason}
		}
		refused = append(refused, why)
	}
	return Choice{Pane: p, Reason: strings.Join(refused, "; ")}
}

func usable(ctx context.Context, env Env, p config.Pane, cand string) (bool, string) {
	bin := Binary(cand)
	if bin == "" {
		return false, "empty command"
	}
	if _, err := env.LookPath(bin); err != nil {
		return false, bin + " not installed"
	}
	switch p.Probe {
	case "none":
		return true, ""
	case "":
		return builtin(ctx, env, bin)
	default:
		pctx, cancel := context.WithTimeout(ctx, Timeout)
		defer cancel()
		if env.Run(pctx, p.Probe) {
			return true, ""
		}
		return false, bin + ": probe `" + p.Probe + "` failed"
	}
}

func builtin(ctx context.Context, env Env, bin string) (bool, string) {
	need, ok := gpuTools[bin]
	if !ok {
		return true, ""
	}
	if hasNVIDIA(ctx, env) {
		return true, ""
	}
	if need == "any" {
		for _, d := range env.DRMDrivers() {
			if drmGPUDrivers[d] {
				return true, ""
			}
		}
	}
	return false, bin + ": no GPU it can read on this host"
}

func hasNVIDIA(ctx context.Context, env Env) bool {
	if _, err := env.LookPath("nvidia-smi"); err != nil {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	return env.Run(pctx, "nvidia-smi -L >/dev/null 2>&1")
}

// Binary is the program a command line starts: its first word, skipping
// leading VAR=value assignments.
func Binary(cmdline string) string {
	for _, f := range strings.Fields(cmdline) {
		if strings.Contains(f, "=") && !strings.HasPrefix(f, "=") && !strings.Contains(f, "/") {
			continue
		}
		return f
	}
	return ""
}
