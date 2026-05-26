package git

import (
	"bufio"
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"
)

// Info holds the parsed result of a git status query for a single directory.
// All counts are zero on a clean repository. If the repository has no upstream
// branch the Ahead/Behind fields are zero.
//
// Field naming mirrors the glyph semantics in opt/profiles/.p10k.zsh's
// my_git_formatter:
//
//	+N staged   (X ≠ '.' in porcelain v2 ordinary/renamed entries)
//	!N unstaged (Y ≠ '.' in porcelain v2 ordinary/renamed entries)
//	?N untracked (lines starting with '?')
//	⇡N ahead    (from # branch.ab header)
//	⇣N behind   (from # branch.ab header)
//
// Unmerged/conflict entries (lines starting with 'u') are counted in Conflicts.
// They are NOT added to Staged or Unstaged to avoid double-counting.
type Info struct {
	Branch   string // current branch name; "(detached)" when detached
	Detached bool   // true when HEAD is detached

	Staged    int // index changes (X ≠ '.')
	Unstaged  int // worktree changes (Y ≠ '.')
	Untracked int // untracked files ('?' lines)
	Conflicts int // unmerged entries ('u' lines)

	Ahead  int // commits ahead of upstream
	Behind int // commits behind upstream

	Stashes int // entries in the stash
}

const statusTimeout = 800 * time.Millisecond

// Status runs `git status --porcelain=v2 --branch` and `git stash list`
// inside dir (using the provided Runner) and returns a parsed Info struct.
//
// A context with an 800 ms deadline is layered on top of ctx so that slow
// git invocations do not stall the status-line render. If that deadline is
// reached the context error is surfaced to the caller so it can degrade
// gracefully.
//
// If the Runner returns an error for either call (e.g. not a git repo) the
// error is surfaced immediately; the caller is responsible for degradation.
func Status(ctx context.Context, r Runner, _ string) (Info, error) {
	tctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	out, err := r.Run(tctx, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return Info{}, err
	}

	info := parsePortcelainV2(out)

	stashOut, err := r.Run(tctx, "stash", "list")
	if err != nil {
		return Info{}, err
	}

	info.Stashes = countLines(stashOut)
	return info, nil
}

// parsePortcelainV2 parses the output of `git status --porcelain=v2 --branch`.
func parsePortcelainV2(out []byte) Info {
	var info Info
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 {
			continue
		}

		switch {
		case strings.HasPrefix(line, "# branch.head "):
			name := strings.TrimPrefix(line, "# branch.head ")
			info.Branch = name
			info.Detached = name == "(detached)"

		case strings.HasPrefix(line, "# branch.ab "):
			// Format: # branch.ab +<ahead> -<behind>
			rest := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(rest)
			if len(parts) == 2 {
				if v, err := strconv.Atoi(strings.TrimPrefix(parts[0], "+")); err == nil {
					info.Ahead = v
				}
				if v, err := strconv.Atoi(strings.TrimPrefix(parts[1], "-")); err == nil {
					info.Behind = v
				}
			}

		case strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 "):
			// Ordinary (1) and renamed/copied (2) entries.
			// Field layout: <type> <XY> ...
			// XY is at index 2-3 of the line (after "1 " or "2 ").
			if len(line) < 4 {
				continue
			}
			xy := line[2:4] // e.g. "M.", ".M", "MM"
			if xy[0] != '.' {
				info.Staged++
			}
			if xy[1] != '.' {
				info.Unstaged++
			}

		case strings.HasPrefix(line, "? "):
			info.Untracked++

		case strings.HasPrefix(line, "u "):
			// Unmerged / conflict entries. Counted separately.
			info.Conflicts++
		}
	}
	return info
}

// countLines counts non-empty lines in b.
func countLines(b []byte) int {
	if len(b) == 0 {
		return 0
	}
	count := 0
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		if sc.Text() != "" {
			count++
		}
	}
	return count
}
