// Package tmpl renders gss's markdown templates — FEATURE.md and WORKER.md
// (design.md → "FEATURE.md template", "WORKER.md template"). The embedded
// template files + loader land in PR-31; this file is the renderer Service,
// which sanitises user-supplied fields (description, goal) before
// substitution so ANSI escapes and control characters can't leak into a
// rendered file (pre-v1 hardening checklist).
//
// text/template (not html/template) is used because the output is markdown,
// not HTML; the explicit sanitise() is what guards untrusted fields.
package tmpl

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// FeatureData is the substitution set for a FEATURE.md render.
type FeatureData struct {
	Name        string
	Description string // user-supplied → sanitised
	BaseBranch  string
	StartedAt   string
}

// WorkerData is the substitution set for a WORKER.md render.
type WorkerData struct {
	Feature     string
	User        string
	Purpose     string
	Suffix      string
	Branch      string
	BaseBranch  string
	Description string // user-supplied → sanitised
	Goal        string // user-supplied → sanitised
}

// RenderFeature renders the feature template with d, sanitising the
// user-supplied Description first.
func RenderFeature(tmplText string, d FeatureData) (string, error) {
	d.Description = sanitize(d.Description)
	return render("feature", tmplText, d)
}

// RenderWorker renders the worker template with d, sanitising the
// user-supplied Description and Goal first.
func RenderWorker(tmplText string, d WorkerData) (string, error) {
	d.Description = sanitize(d.Description)
	d.Goal = sanitize(d.Goal)
	return render("worker", tmplText, d)
}

func render(name, text string, data any) (string, error) {
	t, err := template.New(name).Parse(text)
	if err != nil {
		return "", fmt.Errorf("tmpl: parse %s: %w", name, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("tmpl: execute %s: %w", name, err)
	}
	return b.String(), nil
}

// ansiCSIRe matches an ANSI CSI escape sequence (ESC [ … final byte).
var ansiCSIRe = regexp.MustCompile("\x1b\\[[\x00-\x3f]*[\x40-\x7e]")

// sanitize strips ANSI CSI sequences and C0/C1/DEL control characters from
// a user-supplied field, preserving ordinary text plus newlines and tabs
// (multi-line goals stay intact). This keeps a rendered FEATURE/WORKER.md
// free of terminal-control injection.
func sanitize(s string) string {
	s = ansiCSIRe.ReplaceAllString(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f: // C0 (except \n,\t handled above) + DEL
			continue
		case r >= 0x80 && r <= 0x9f: // C1
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
