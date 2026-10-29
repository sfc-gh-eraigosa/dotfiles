package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/fleet/internal/updplan"
)

// planGlyph prefixes the header's plan badge, the way localGlyph prefixes the
// host badge: the glyph, not colour, says which question the text answers.
const planGlyph = "📦"

// planBadge names what `u` would run on a host: the plan's entry (repo and
// run step, +N when the plan runs more than the headline shows) and the
// commit this machine's checkout of that repo sits on. Empty when the plan
// runs nothing. width bounds it by truncation, like the host badge.
func (m tuiModel) planBadge(width int) string {
	e, ok := m.plan.Entry()
	if !ok {
		return ""
	}
	label := e.Run
	if e.Repo.Name != "" {
		label = e.Repo.Name + ":" + label
	}
	if e.Others > 0 {
		label += " +" + strconv.Itoa(e.Others)
	}
	badge := th.title.Render(planGlyph + " " + label)
	if m.entrySHA != "" {
		badge += th.dim.Render(" (" + m.entrySHA + ")")
	}
	return trunc(badge, width)
}

// detectEntrySHA is the one impure edge behind the badge's commit: the short
// HEAD of THIS machine's checkout of the plan's entry repo. It is what is on
// disk now — the plan's sync step fast-forwards before the entry runs — and
// "" whenever there is no such checkout to ask, which the badge renders as no
// commit rather than a guess.
func detectEntrySHA(p updplan.Plan) string {
	e, ok := p.Entry()
	if !ok || e.Repo.Path == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	out, err := exec.Command("git", "-C", expandLocalPath(e.Repo.Path, home), "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// expandLocalPath resolves a plan repo path, which Parse leaves absolute or
// "~/…", against this machine's home.
func expandLocalPath(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}
