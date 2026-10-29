package cmd

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/homedir"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// planGlyph prefixes the header's plan badge, the way localGlyph prefixes the
// host badge: the glyph, not colour, says which question the text answers.
const planGlyph = "📦"

// planBadge names what `u` would run on a host: the plan's entry (repo and
// run step, +N when the plan runs more than the headline shows) and the
// commit it would deliver. Empty when the plan runs nothing. width bounds it
// by truncation, like the host badge.
func (m tuiModel) planBadge(width int) string {
	if !m.hasEntry {
		return ""
	}
	label := printable(m.entry.Run)
	if m.entry.Repo.Name != "" {
		label = m.entry.Repo.Name + ":" + label
	}
	if m.entry.Others > 0 {
		label += " +" + strconv.Itoa(m.entry.Others)
	}
	badge := th.title.Render(planGlyph + " " + label)
	if m.entrySHA != "" {
		badge += th.dim.Render(" (" + m.entrySHA + ")")
	}
	return trunc(badge, width)
}

// printable makes plan text safe for one header line. Validation only bars
// NUL and newline from `run:`, so a CR, tab or escape sequence would otherwise
// reach the terminal — moving the cursor or breaking the width math the
// panel border depends on.
func printable(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return ' '
		}
		return r
	}, s)
}

// entryRef is the remote-tracking ref `u` delivers for the entry repo: the
// sync step fast-forwards to origin/<branch>, with --update-ref already
// folded into Branches[0] by resolveTUIPlan. "default" is the remote's
// default branch; no branch at all is "" — the sync refuses such a repo, so
// there is no commit it would deliver.
func entryRef(r updplan.Repo) string {
	if len(r.Branches) == 0 {
		return ""
	}
	if r.Branches[0] == "default" {
		return "origin/HEAD"
	}
	return "origin/" + r.Branches[0]
}

// revParse resolves ref in the checkout at dir, or "" when dir is not that
// checkout or the ref does not resolve.
type revParse func(dir, ref string) string

// resolveEntrySHA is the short commit `u` would deliver for the entry repo —
// what the host lands on after the sync, not what this machine happens to
// have checked out. It is resolved in this machine's clone at the plan's own
// path, never in --repo: --repo may be a different repo entirely (the
// dotfiles default under a playground plan), and a gss worktree of the entry
// repo shares its refs with that clone, so the answer is the same there.
// Anything that cannot be resolved is "" — no (sha), never a guess.
//
// It is read once, like base.Head(): both answer "what is the fleet being
// brought to", which a local update does not move.
func resolveEntrySHA(e updplan.Entry, ok bool, home string, rp revParse) string {
	if !ok || e.Repo.Path == "" || home == "" {
		return ""
	}
	ref := entryRef(e.Repo)
	if ref == "" {
		return ""
	}
	return short(rp(homedir.Expand(e.Repo.Path, home), ref))
}

// gitRevParseTimeout bounds the one git call the header may make before the
// dashboard opens: a stale network mount must cost a missing (sha), not a
// blank screen.
const gitRevParseTimeout = 2 * time.Second

// gitRevParse is the real revParse. It refuses a dir that is not itself the
// top of a work tree: `git -C` walks up, so a plain directory inside some
// other repo (an aborted clone under a git-managed $HOME) would otherwise
// answer with the PARENT repo's commit.
func gitRevParse(dir, ref string) string {
	ctx, cancel := context.WithTimeout(context.Background(), gitRevParseTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel", "--verify", "--quiet", ref+"^{commit}").Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 || !sameDir(lines[0], dir) {
		return ""
	}
	return lines[1]
}

// sameDir compares two paths after resolving symlinks, so a checkout reached
// through a linked parent still counts as itself.
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}
