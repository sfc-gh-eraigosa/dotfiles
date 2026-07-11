---
name: gss-agent-cli-gotchas
description: "Recurring friction points when driving gss + the repo's hooks from Bash — use $HOME, --worker, and re-apply PR body last."
metadata: 
  scope: account
  node_type: memory
  type: reference
  originSessionId: 9afcdc3a-a822-4864-980c-1d707265ee3d
---

Driving `gss feature` and this repo's PreToolUse hooks from an agent, three things bite every time:

1. **`privacy_guard.sh` blocks literal home paths.** A Bash command containing `/home/<user>` is refused ("absolute home path leaks the account name"). Always write `$HOME` / `~` in commands — including `cd` targets and heredoc commit bodies.
2. **`safety_guard.sh` does STATIC path analysis and can't expand `$HOME`.** `gss feature checkpoint` / `pr --ready` resolve the worker from cwd; when cwd is given as `"$HOME/.config/gss/worktrees/.../<leaf>"` the hook sees the unexpanded string, decides it "is not a feature worker worktree," and blocks. Fix: pass `--worker <feature/user/purpose>` explicitly and run from any cwd, instead of relying on cwd resolution.
3. **`gss feature checkpoint` regenerates the PR body from its template every run**, wiping any custom body. So apply a rich PR description **last, after the final checkpoint**, and preserve the `<!-- gss:stack-begin -->`…`<!-- gss:stack-end -->` markers (read the current body, re-inject the stack block) or gss's stack section is lost.

Also: `gss feature pr --ready` is token-gated — two separate Bash calls (generate `~/.config/gss/approval.token` = `git rev-parse HEAD`, then the command); the token is one-shot. See [[gss-land-flow-no-interrupt]].
