// Package mode decides whether gss is running in classic mode (a normal
// checkout) or feature-worker mode (cwd is inside a registered worker
// worktree). Every classic cobra leaf (push, pr, sync, status, …) routes
// its mode check through IsInWorker so the behaviour is identical
// everywhere (design.md → "Plain gss (unchanged)"; resolution #9). This is
// the canonical version of the gate prototyped inline in PR-23/25.
package mode

import (
	"os"
	"strings"

	"github.com/wenlock/dotfiles/gss/internal/registry"
)

// IsInWorker reports whether cwd is at or under a registered worker's
// worktree, returning the canonical worker_ref (feature/user/purpose
// [-suffix]) when so. An empty cwd or empty registry yields ("", false).
func IsInWorker(cwd string, reg registry.Registry) (string, bool) {
	if cwd == "" {
		return "", false
	}
	for _, f := range reg.Features {
		for _, w := range f.Workers {
			if w.Worktree == "" {
				continue
			}
			if cwd == w.Worktree || strings.HasPrefix(cwd, w.Worktree+string(os.PathSeparator)) {
				ref := f.Name + "/" + w.User + "/" + w.Purpose
				if w.Suffix != "" {
					ref += "-" + w.Suffix
				}
				return ref, true
			}
		}
	}
	return "", false
}
