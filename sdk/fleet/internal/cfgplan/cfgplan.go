// Package cfgplan computes a ONE-WAY ssh-config transfer as a reviewable plan.
//
// It is pure: no filesystem, no network, no clock. Direction lives entirely in
// the caller's choice of which text is "local" and which is "remote" — there is
// deliberately no bidirectional operation here, because a combined transfer
// would have to resolve conflicts by policy rather than by an operator reading
// a diff, and would make one mistake's blast radius the union of both
// directions.
package cfgplan

import (
	"fmt"
	"sort"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/sshconf"
)

// ChangeKind is what applying this change would do to the destination.
type ChangeKind string

const (
	Add       ChangeKind = "add"
	Update    ChangeKind = "update"
	Unchanged ChangeKind = "unchanged"
	Skipped   ChangeKind = "skipped"
)

// FieldDelta is one modelled directive that moved.
type FieldDelta struct{ Name, From, To string }

// Change is one alias's outcome. Host is the resolved result to write; Fields
// is what to show the operator.
type Change struct {
	Alias  string
	Kind   ChangeKind
	Host   sshconf.Host
	Fields []FieldDelta
	Reason string
}

// Opts bounds the transfer.
type Opts struct {
	Marker string // fleet marker, default "#fleet"
	All    bool   // every concrete Host, not just marked ones
	Source string // recorded in the provenance comment
}

// Plan is the whole decision surface for one transfer.
type Plan struct {
	Source      string
	Changes     []Change
	Includes    int      // Include directives in the source text (cat does not follow them)
	NotImported []string // distinct unmodelled directive names found in the source
}

// Empty reports whether applying this plan would change nothing.
func (p Plan) Empty() bool {
	for _, c := range p.Changes {
		if c.Kind == Add || c.Kind == Update {
			return false
		}
	}
	return true
}

// modelledFields is the single ordered list both deltas and merge walk, so the
// two can never disagree about what a transfer carries.
func modelledFields(h sshconf.Host) []struct{ Name, Value string } {
	return []struct{ Name, Value string }{
		{"HostName", h.HostName},
		{"User", h.User},
		{"Port", h.Port},
		{"IdentityFile", h.Identity},
	}
}

// deltas lists the modelled fields that differ, in a stable order so a rendered
// diff never reshuffles between runs.
//
// An empty incoming value is skipped: omission is not an instruction to delete,
// so a source that simply does not set User can never blank the local one.
func deltas(from, to sshconf.Host) []FieldDelta {
	f, t := modelledFields(from), modelledFields(to)
	var out []FieldDelta
	for i := range t {
		if t[i].Value != "" && f[i].Value != t[i].Value {
			out = append(out, FieldDelta{Name: t[i].Name, From: f[i].Value, To: t[i].Value})
		}
	}
	return out
}

// merge applies the incoming host's non-empty modelled fields onto the local
// one, for the same reason deltas skips them.
func merge(local, incoming sshconf.Host) sshconf.Host {
	out := local
	if incoming.HostName != "" {
		out.HostName = incoming.HostName
	}
	if incoming.User != "" {
		out.User = incoming.User
	}
	if incoming.Port != "" {
		out.Port = incoming.Port
	}
	if incoming.Identity != "" {
		out.Identity = incoming.Identity
	}
	return out
}

// Build computes the transfer from remoteText into localText. Neither argument
// is modified; nothing is written anywhere.
func Build(localText, remoteText string, o Opts) (Plan, error) {
	if o.Marker == "" {
		o.Marker = "#fleet"
	}
	locals, err := sshconf.Parse(localText, o.Marker)
	if err != nil {
		return Plan{}, fmt.Errorf("cfgplan: local config: %w", err)
	}
	remotes, err := sshconf.Parse(remoteText, o.Marker)
	if err != nil {
		return Plan{}, fmt.Errorf("cfgplan: source config: %w", err)
	}

	byAlias := make(map[string]sshconf.Host, len(locals))
	for _, h := range locals {
		byAlias[h.Alias] = h
	}

	p := Plan{Source: o.Source}
	for _, r := range remotes {
		// The SOURCE decides what it is willing to share: an unmarked block is
		// its own business, not fleet inventory.
		if !r.Fleet && !o.All {
			continue
		}
		local, exists := byAlias[r.Alias]
		if !exists {
			p.Changes = append(p.Changes, Change{Alias: r.Alias, Kind: Add, Host: r})
			continue
		}
		d := deltas(local, r)
		if len(d) == 0 {
			p.Changes = append(p.Changes, Change{Alias: r.Alias, Kind: Unchanged, Host: local})
			continue
		}
		p.Changes = append(p.Changes, Change{
			Alias: r.Alias, Kind: Update, Host: merge(local, r), Fields: d,
		})
	}
	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].Alias < p.Changes[j].Alias })
	return p, nil
}
