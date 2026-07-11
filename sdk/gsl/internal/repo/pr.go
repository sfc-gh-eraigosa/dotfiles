package repo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gsl/internal/gh"
)

// prWarnWriter is the sink for PR's diagnostic warnings. It defaults to
// os.Stderr and is a package-level var (rather than a parameter) so the
// warning can be exercised in tests without changing PR's signature, which
// would force edits to the out-of-package caller in internal/render. See the
// Finding #8 note: keeping the writer in-package preserves the existing
// callers while still surfacing a diagnostic that other code paths emit.
var prWarnWriter io.Writer = os.Stderr

// RepoInfo is the combined view of the current repository state that the
// render layer consumes. All fields are safe to read even when the
// corresponding source was unavailable (zero values = omit the field).
type RepoInfo struct {
	// Location describes root-vs-worktree and the worktree count.
	Location Location

	// FeatureName is the parent feature name from the gss registry, when
	// the current worktree / branch was found in the registry. Empty when
	// the registry had no match.
	FeatureName string

	// PRNumber is the pull-request number (> 0 when a PR was resolved).
	PRNumber int
	// PRState is the PR state string (e.g. "OPEN", "MERGED"). Empty when
	// PRNumber == 0.
	PRState string
}

// PR resolves the PR number and state for the given branch, using the
// gss registry as a fast, offline-first source and falling back to
// gh.PR when the registry is unavailable, outdated, or has no entry.
//
// Parameters:
//   - ctx: context for the gh.Runner call (only used when falling back).
//   - ghRunner: injected gh.Runner; used only on the fallback path.
//   - branch: the current git branch name.
//   - toplevel: the absolute path of the working-tree root (from git.Toplevel).
//   - registryPath: path to the gss registry JSON; pass DefaultRegistryPath()
//     in production, or a testdata path in tests.
//
// Return contract mirrors gh.PR: (nil, nil) means "no PR found". A non-nil
// error is returned only for unexpected internal failures.
//
// The returned *RepoInfo always has its Location zero-valued; callers are
// expected to populate it separately via Locate, then assign the PR fields.
// This keeps the two I/O sources (git + gh/registry) independently composable.
func PR(ctx context.Context, ghRunner gh.Runner, branch, toplevel, registryPath string) (*RepoInfo, error) {
	// --- registry-first path ---
	reg, err := LoadRegistry(registryPath)
	if err == nil {
		// Registry loaded successfully; try to find a match.
		if m, ok := Match(reg, toplevel, branch); ok {
			if m.HasPR {
				return &RepoInfo{
					FeatureName: m.Feature,
					PRNumber:    m.PRNumber,
					PRState:     m.PRState,
				}, nil
			}
			// Registry matched but no pr_url: fall through to gh,
			// preserving the feature name from the registry.
			info, err := ghFallback(ctx, ghRunner, branch)
			if err != nil {
				return nil, err
			}
			if info != nil {
				info.FeatureName = m.Feature
			} else {
				// No PR from gh either; return empty info with feature name.
				info = &RepoInfo{FeatureName: m.Feature}
			}
			return info, nil
		}
	} else if !errors.Is(err, ErrRegistryNotFound) && !errors.Is(err, ErrUnsupportedSchema) {
		// Unexpected error loading registry (unreadable or malformed JSON).
		// Unlike absent/bumped-schema (which are expected), surface a
		// diagnostic so the failure is not swallowed, then fall through to
		// the gh fallback as before (behavior unchanged beyond the warning).
		fmt.Fprintf(prWarnWriter, "gsl: failed to load registry %q: %v; falling back to gh\n", registryPath, err)
	}
	// Registry absent, schema bumped, unreadable, or no match → gh fallback.
	return ghFallback(ctx, ghRunner, branch)
}

// ghFallback calls gh.PR and wraps the result in a *RepoInfo.
// Returns nil when gh.PR returns nil (no PR found).
func ghFallback(ctx context.Context, ghRunner gh.Runner, branch string) (*RepoInfo, error) {
	prInfo, err := gh.PR(ctx, ghRunner, branch)
	if err != nil {
		return nil, err
	}
	if prInfo == nil {
		return nil, nil
	}
	return &RepoInfo{
		PRNumber: prInfo.Number,
		PRState:  prInfo.State,
	}, nil
}
