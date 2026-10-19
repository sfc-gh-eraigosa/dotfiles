// Package probe decides which program each pane gets on this machine: the
// pane's command or its first usable fallback, where "usable" means it is on
// PATH and, for tools that only work with certain hardware, that the hardware
// is there (nvtop on a host it cannot read would be an empty pane).
package probe

import (
	"context"
	"fmt"
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
	Cmd    string // the command that will run first; "" = pane dropped
	Reason string // why Cmd was chosen, or why each candidate was refused
	// Views are the views that can run, in tool then view order (a plain pane
	// has one, named after the pane); Dropped are the rest, with reasons.
	Views   []ViewChoice
	Dropped []ViewChoice
}

// ViewChoice is the outcome for one view of a deck.
type ViewChoice struct {
	Tool, View string
	Zoom       bool   // the tool fills the middle when on screen
	Cmd        string // the command that will run ("" when dropped)
	Every, Cwd string
	Reason     string
}

// Usable reports whether the pane will run.
func (c Choice) Usable() bool { return c.Cmd != "" }

// Choose picks a command for every pane, in config order. A deck is decided
// per view, then per tool (its probe, run once), then per pane: a pane with
// no usable view is dropped (spec F8).
func Choose(ctx context.Context, env Env, panes []config.Pane) []Choice {
	out := make([]Choice, len(panes))
	for i, p := range panes {
		if p.IsDeck() {
			out[i] = chooseDeck(ctx, env, p)
			continue
		}
		c := choose(ctx, env, p)
		vc := ViewChoice{Tool: p.Name, View: p.Name, Cmd: c.Cmd, Every: p.Every, Cwd: p.Cwd, Reason: c.Reason}
		if c.Usable() {
			c.Views = []ViewChoice{vc}
		} else {
			c.Dropped = []ViewChoice{vc}
		}
		out[i] = c
	}
	return out
}

func chooseDeck(ctx context.Context, env Env, p config.Pane) Choice {
	ch := Choice{Pane: p}
	for _, t := range p.Tools {
		if t.Probe != "" {
			pctx, cancel := context.WithTimeout(ctx, Timeout)
			ok := env.Run(pctx, t.Probe)
			cancel()
			if !ok {
				for _, v := range t.Views {
					ch.Dropped = append(ch.Dropped, ViewChoice{Tool: t.Name, View: v.Name, Zoom: t.Zoom, Reason: "tool probe `" + t.Probe + "` failed"})
				}
				continue
			}
		}
		for _, v := range t.Views {
			c := choose(ctx, env, config.Pane{Name: v.Name, Cmd: v.Cmd, Fallbacks: v.Fallbacks, Probe: v.Probe})
			vc := ViewChoice{Tool: t.Name, View: v.Name, Zoom: t.Zoom, Cmd: c.Cmd, Every: v.Every, Cwd: v.Cwd, Reason: c.Reason}
			if c.Usable() {
				ch.Views = append(ch.Views, vc)
			} else {
				ch.Dropped = append(ch.Dropped, vc)
			}
		}
	}
	total := len(ch.Views) + len(ch.Dropped)
	if len(ch.Views) == 0 {
		var why []string
		for _, d := range ch.Dropped {
			why = append(why, d.Tool+"/"+d.View+": "+d.Reason)
		}
		ch.Reason = "no usable view (" + strings.Join(why, "; ") + ")"
		return ch
	}
	ch.Cmd = ch.Views[0].Cmd
	ch.Reason = fmt.Sprintf("%d of %d views usable", len(ch.Views), total)
	return ch
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
