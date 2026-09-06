---
name: gss-tmux-mgr-rebuild-after-source-change
description: "After changing gss source, rebuild+reinstall via sdk/gss/build.sh — a stale installed gss silently breaks tmux-mgr's shell-outs even when all unit tests are green."
metadata: 
  scope: account
  node_type: memory
  type: project
  originSessionId: 5efd40da-ccb4-49e8-a327-6b9e289edc86
---

`tmux-mgr` shells out to the **installed** `gss` binary (`~/opt/bin/gss`), parsing `gss feature worker add --json` into `cmd/agent_gss.go`'s `workerAddResult` (keys `worker_ref/branch/worktree_path/base_branch`). The unit tests on both sides **mock the gss runner**, so they pass even when the deployed binary is older than the source.

**Why:** Found during the PR-54..57 end-to-end evaluation (2026-05-22): the installed `gss` predated PR-55, lacked `--json`/spawned_by flags, and `tmux-mgr feature add-agent` would die with `unknown flag: --json` despite green source + green unit suites. There is no runtime gss version/capability gate (logged as STATE.md carry-forward #16; potential follow-up: have tmux-mgr detect the old-gss case and tell the user to rebuild).

**How to apply:** Whenever gss source under `sdk/gss/**` changes, rebuild + reinstall with `sdk/gss/build.sh` before exercising the tmux-mgr↔gss flow. Regression guard: run `sdk/tmux-mgr/scripts/e2e-gss-integration.sh` (defaults to the PATH `gss`; drives the real `worker add --json` in a sandbox and asserts the JSON contract). Part of the [[gss-autonomous-backlog]] (Batch J, tmux-mgr refactor).
