# Git Utilities (opt/scripts/git)

- `git_identity_doctor.sh`: Verifies git user.email against the authenticated GitHub account (run by install.sh and `make git-doctor`); `git_identity_doctor_test.sh` is its CI test driver.
- `git_add.sh`, `git_branch.sh`, `git_pull.sh`: Basic git operations.
- `git-local-master.sh`: Helper for managing local master branches.
- `git-rm-mybranches.sh`: Cleanup utility for personal branches.
- `git-signoff.sh`: Adds sign-off to commits.
- `update_origin.sh`: Updates the remote origin URL.
- `gh_issues.rb`: Script for managing GitHub issues.

Interactive git shortcuts (`git-reset`, `git-reset-all`, `git-clean`, `git-help`)
live in [`opt/profiles/.gitools.sh`](../../profiles/.gitools.sh) — the slim
replacement for the retired Gerrit-era `setup_git_alias.sh` / `~/.gitenv`
generator (removed 2026-08-08; git history has the old toolset).
