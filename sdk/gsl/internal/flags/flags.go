// Package flags resolves the gff feature flags gsl consults at render time.
//
// Every lookup is fail-open: an error, an unregistered source, a missing gff
// schema, or a slow answer all mean "on" — a flag can only ever turn a link
// family OFF. This mirrors install.sh's gff_on gate, which fails open for the
// same reason (a fresh machine must render exactly as if gff were absent).
//
// The package is the only place gsl touches the gff SDK, and it is called from
// cmd, never from render (render stays free of subprocess-shaped dependencies).
package flags

import (
	"context"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"
)

// Namespace is the dotfiles repo's gff namespace (.github/gff/features.yaml).
// `gff install` from the checkout registers it on a host; install.sh does that.
const Namespace = "com.github.sfc-gh-eraigosa.dotfiles"

// The link flags. Enabled is the master; the rest are link families.
const (
	KeyEnabled = "gsl.links.enabled"
	KeyRepo    = "gsl.links.repo"
	KeyDirGit  = "gsl.links.dirgit"
	KeyAI      = "gsl.links.ai"
	KeyTime    = "gsl.links.time"
)

// Per-section flags: gsl.section.<segment type>. A section can only be turned
// OFF by its flag; failure means on.
const (
	KeySectionHost   = "gsl.section.host"
	KeySectionDirGit = "gsl.section.dirgit"
	KeySectionRepo   = "gsl.section.repo"
	KeySectionAI     = "gsl.section.ai"
	KeySectionTime   = "gsl.section.time"
	KeySectionVideo  = "gsl.section.video"
)

// SectionKeys maps a segment type to its gff flag key.
var SectionKeys = map[string]string{
	"host":   KeySectionHost,
	"dirgit": KeySectionDirGit,
	"repo":   KeySectionRepo,
	"ai":     KeySectionAI,
	"time":   KeySectionTime,
	"video":  KeySectionVideo,
}

// Links is the resolved flag set. The zero value is NOT the default — use
// Resolve, which starts from all-true.
//
// Sections holds the gsl.section.* answers keyed by segment type; it is always
// fully populated by Resolve.
type Links struct {
	Enabled, Repo, DirGit, AI, Time bool
	Sections                        map[string]bool
}

// SectionOff returns the segment types whose section flag resolved to false.
func (l Links) SectionOff() map[string]bool {
	off := map[string]bool{}
	for t, on := range l.Sections {
		if !on {
			off[t] = true
		}
	}
	return off
}

// Lookup answers one flag key. It is the seam tests replace with a fake.
type Lookup func(key string) (bool, error)

// Resolve evaluates the link and section keys concurrently and returns when all have
// answered or ctx is done; keys that errored or never answered read true.
// The goroutines of a timed-out lookup are abandoned, not joined — the render
// must not wait on them.
func Resolve(ctx context.Context, look Lookup) Links {
	out := Links{Enabled: true, Repo: true, DirGit: true, AI: true, Time: true, Sections: map[string]bool{}}
	for t := range SectionKeys {
		out.Sections[t] = true
	}
	if look == nil {
		return out
	}
	type answer struct {
		key string
		val bool
	}
	keys := []string{KeyEnabled, KeyRepo, KeyDirGit, KeyAI, KeyTime}
	secType := map[string]string{}
	for t, k := range SectionKeys {
		keys = append(keys, k)
		secType[k] = t
	}
	ch := make(chan answer, len(keys))
	for _, k := range keys {
		go func(k string) {
			v, err := look(k)
			if err != nil {
				v = true
			}
			ch <- answer{k, v}
		}(k)
	}
	for range keys {
		select {
		case a := <-ch:
			switch a.key {
			case KeyEnabled:
				out.Enabled = a.val
			case KeyRepo:
				out.Repo = a.val
			case KeyDirGit:
				out.DirGit = a.val
			case KeyAI:
				out.AI = a.val
			case KeyTime:
				out.Time = a.val
			default:
				if t, ok := secType[a.key]; ok {
					out.Sections[t] = a.val
				}
			}
		case <-ctx.Done():
			return out
		}
	}
	return out
}

// GFFLookup resolves through the registered namespace first and, when that
// source is unknown on this host, through the checkout named by $DOTFILES_DIR
// (a path source). env is injected for tests; nil means os.Getenv.
func GFFLookup(env func(string) string) Lookup {
	if env == nil {
		env = os.Getenv
	}
	return func(key string) (bool, error) {
		v, err := gff.Bool(key, gff.WithSource(Namespace))
		if err == nil {
			return v, nil
		}
		if dir := env("DOTFILES_DIR"); dir != "" {
			return gff.Bool(key, gff.WithSource(dir))
		}
		return true, err
	}
}
