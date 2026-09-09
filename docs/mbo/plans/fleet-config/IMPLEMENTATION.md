# fleet-config — implementation procedure

## Where

Worktree `~/git/worktrees/fleet-config`, branch `feature/fleet-config`, module
`sdk/fleet`. Built with local commits only; published as a single push on the
operator's explicit green light, as [#254](https://github.com/sfc-gh-eraigosa/dotfiles/pull/254).

**Status: complete.** All 12 tasks and 4 gates are done; this file is kept as the
record of how the build ran, not as pending instructions.

## Procedure per task

1. Read the task in [`../fleet-config.md`](../fleet-config.md).
2. Write the failing test **first**; run it; confirm it fails for the stated reason.
3. Write the minimal implementation.
4. `go test ./<pkg>/` then `go test -race ./...`; `gofmt -l .`; `go vet ./...`.
5. Commit locally with a conventional-commit subject.
6. Update TRACKING.md with the SHA + pasted output, then check the TODO box.

## Gates

- `go test -race ./...` green, `gofmt` clean, `go vet` clean before any commit.
- New packages ≥ 90% coverage.
- No private key is ever read, transmitted, or written — by any task.
- No verb moves config in two directions.

## Verification builds

Build to a scratch path (`go build -o <scratch>/fleet main.go`) rather than
`~/opt/bin/fleet`. Install to `~/opt/bin/fleet` via `sdk/fleet/build.sh` **only** when
the operator asks to verify the CLI, since that replaces the binary they are using.

## §8 Kickoff prompt (session bridge)

> Continue the `fleet-config` objective in the worktree `~/git/worktrees/fleet-config`
> on branch `feature/fleet-config`. Read `docs/mbo/plans/fleet-config/TODO.md` and start
> at the first unchecked box; the task detail is in `docs/mbo/plans/fleet-config.md` and
> the requirements in `docs/mbo/specs/fleet-config.md`. TDD strictly: failing test first,
> observed failure, minimal implementation, `go test -race ./...`, local commit, then
> update TRACKING.md with the SHA and pasted output. Do not push — the operator gives an
> explicit green light for a single jumbo push at the end.
