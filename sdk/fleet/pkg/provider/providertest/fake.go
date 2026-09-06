// Package providertest is the harness for anything that consumes a
// provider.Provider: fleet's own leaves, and a plugin author's tests.
//
// FakeProvider is a provider of an ARBITRARY kind — five columns, three
// levels, one action of each of the three kinds — so a consumer proves it
// works for a kind it cannot know, rather than for herdr's. It reaches its
// host only through Host.Exec, and it can be scripted into the failure modes
// a consumer must render as a row.
//
// The package depends on pkg/provider and the standard library only.
package providertest

import (
	"context"
	"fmt"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/pkg/provider"
)

// Kinds of the fake tree. Each has five columns, so a renderer that assumes
// a kind's shape fails against it.
const (
	KindCapability = "fake-capability"
	KindWidget     = "fake-widget"
	KindGadget     = "fake-gadget"
)

// ProbeArgv is what FakeProvider runs on the host. A test asserts the
// provider reached the runner seam with exactly this.
var ProbeArgv = []string{"sh", "-c", "fake-provider probe"}

// FakeProvider is the arbitrary-kind provider. Build it with NewFakeProvider.
type FakeProvider struct {
	absent     string
	probeError string
	hang       bool
}

// Option scripts a FakeProvider into one of the states a consumer must
// render.
type Option func(*FakeProvider)

// Absent makes Probe report provider.ErrAbsent with reason — still returning
// a Node, because an absent capability is a row with a reason, never an
// omission.
func Absent(reason string) Option { return func(f *FakeProvider) { f.absent = reason } }

// ProbeError makes Probe fail outright, the way a plugin that answered badly
// would.
func ProbeError(msg string) Option { return func(f *FakeProvider) { f.probeError = msg } }

// Hang makes every call block until its context is done, so a consumer can
// prove it bounds a provider rather than waiting on one.
func Hang() Option { return func(f *FakeProvider) { f.hang = true } }

// NewFakeProvider returns a FakeProvider scripted by opts.
func NewFakeProvider(opts ...Option) *FakeProvider {
	f := &FakeProvider{}
	for _, o := range opts {
		o(f)
	}
	return f
}

// Name identifies the provider in config and in the capability row.
func (f *FakeProvider) Name() string { return "fake" }

// Columns is positional against Node.Cells; an unknown kind renders IDs only.
func (f *FakeProvider) Columns(kind string) []string {
	switch kind {
	case KindCapability:
		return []string{"CAPABILITY", "VERSION", "STATE", "WIDGETS", "NOTE"}
	case KindWidget:
		return []string{"WIDGET", "STATE", "GADGETS", "OWNER", "NOTE"}
	case KindGadget:
		return []string{"GADGET", "STATE", "PORT", "OWNER", "NOTE"}
	default:
		return nil
	}
}

// Probe costs exactly one round trip through Host.Exec — the provider's only
// reach — and carries the alias and the widget list forward in Attrs.
func (f *FakeProvider) Probe(ctx context.Context, h provider.Host) (provider.Node, error) {
	if err := f.block(ctx); err != nil {
		return provider.Node{}, err
	}
	if f.absent != "" {
		return provider.Node{
			ID:     "fake",
			Kind:   KindCapability,
			Cells:  []string{"fake", "-", "absent", "-", f.absent},
			Detail: f.absent,
			Leaf:   true,
		}, fmt.Errorf("%w: %s", provider.ErrAbsent, f.absent)
	}
	if f.probeError != "" {
		return provider.Node{}, fmt.Errorf("providertest: %s", f.probeError)
	}
	res, err := h.Exec(ctx, "", ProbeArgv...)
	if err != nil {
		return provider.Node{}, fmt.Errorf("providertest: probe: %w", err)
	}
	widgets := lines(res.Stdout)
	return provider.Node{
		ID:     "fake",
		Kind:   KindCapability,
		Cells:  []string{"fake", "1.0", "ok", fmt.Sprint(len(widgets)), ""},
		Detail: fmt.Sprintf("%d widgets on %s", len(widgets), h.Alias()),
		Attrs:  map[string]string{"alias": h.Alias(), "widgets": strings.Join(widgets, ",")},
	}, nil
}

// Children lists the level at path. Depth 0 is the widgets, depth 1 the
// gadgets (leaves); anything else is provider.ErrNoSuchPath. Attrs arrive
// exactly as the parent left them and are handed down again.
func (f *FakeProvider) Children(ctx context.Context, h provider.Host, path []string, attrs map[string]string) ([]provider.Node, error) {
	if err := f.block(ctx); err != nil {
		return nil, err
	}
	switch len(path) {
	case 0:
		return f.widgets(ctx, h, attrs)
	case 1:
		return f.gadgets(h, path[0], attrs)
	default:
		return nil, fmt.Errorf("%w: %s", provider.ErrNoSuchPath, strings.Join(path, "/"))
	}
}

func (f *FakeProvider) widgets(ctx context.Context, h provider.Host, attrs map[string]string) ([]provider.Node, error) {
	names := lines(strings.ReplaceAll(attrs["widgets"], ",", "\n"))
	if len(names) == 0 {
		res, err := h.Exec(ctx, "", ProbeArgv...)
		if err != nil {
			return nil, fmt.Errorf("providertest: widgets: %w", err)
		}
		names = lines(res.Stdout)
	}
	out := make([]provider.Node, 0, len(names))
	for _, n := range names {
		out = append(out, provider.Node{
			ID:     n,
			Kind:   KindWidget,
			Cells:  []string{n, "running", "1", h.Alias(), ""},
			Detail: "widget " + n,
			Attrs:  map[string]string{"alias": h.Alias(), "parent": "fake", "widget": n},
		})
	}
	return out, nil
}

func (f *FakeProvider) gadgets(h provider.Host, widget string, attrs map[string]string) ([]provider.Node, error) {
	if attrs["widget"] != "" && attrs["widget"] != widget {
		return nil, fmt.Errorf("%w: %s", provider.ErrNoSuchPath, widget)
	}
	if !strings.HasPrefix(widget, "widget-") {
		return nil, fmt.Errorf("%w: %s", provider.ErrNoSuchPath, widget)
	}
	id := widget + "-gadget"
	return []provider.Node{{
		ID:     id,
		Kind:   KindGadget,
		Cells:  []string{id, "running", "8080", h.Alias(), ""},
		Detail: "gadget of " + widget,
		Leaf:   true,
		Attrs:  map[string]string{"alias": h.Alias(), "parent": widget},
		Actions: []provider.Action{
			{Key: "c", Label: "attach " + id, Handoff: &provider.Handoff{
				Kind: provider.HandoffLocal,
				Argv: []string{"fake-client", "--remote", h.Alias(), "--gadget", id},
			}},
			{Key: "l", Label: "logs of " + id, Stream: &provider.Stream{
				Command: "fake-provider logs " + id,
				Follow:  true,
			}},
			{Key: provider.TunnelKey, Label: "bridge " + id, Tunnel: &provider.Tunnel{
				RemotePort: 8080,
				Scheme:     "http",
			}},
			{Key: "x", Label: "shell into " + id, Unavailable: "the fake gadget has no shell",
				Handoff: &provider.Handoff{Kind: provider.HandoffRemote, Command: "fake-provider shell " + id}},
		},
	}}, nil
}

// block implements Hang: wait for the caller's context, never a timer of the
// provider's own.
func (f *FakeProvider) block(ctx context.Context) error {
	if !f.hang {
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
