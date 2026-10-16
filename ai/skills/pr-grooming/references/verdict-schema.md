# verdicts.json — the shape every PR triage subagent returns

One JSON array, one object per PR. `pr-apply.sh` and `pr-report.sh` read exactly these fields.

```json
[
  {
    "number": 146,
    "title": "notebooklm-mcp: impl",
    "category": "mbo-draft | stale-draft | bugfix | dependabot | other  (the corrected one, not triage.tsv's guess)",
    "verdict": "stale | duplicate | refresh | finished | ready | rebase-only | leave",
    "evidence": "1-3 sentences citing paths / PR numbers / index rows / commands actually checked",
    "rebased": false,
    "ci_verified": false,
    "suggested_action": "close-stale | comment | ready | ready-for-merge | none",
    "close_comment": "required for close-stale: why it is stale/duplicate, the successor, where the tracker issue lives",
    "comment": "optional for comment / ready / ready-for-merge: the refresh proof or the decision being asked"
  }
]
```

Rules the apply script enforces:

- `close-stale` only for `stale` / `duplicate`, only with a non-empty `close_comment`; the
  branch is never deleted and the comment says the PR can be reopened.
- `ready` (`gh pr ready`) only for `refresh` and `finished`.
- `close_comment` and `comment` may be omitted or left `""` when unused — the script treats
  both the same; a non-empty `comment` on `none` is ignored (use the `comment` action).
- `ready-for-merge` only for `ready`, on a non-draft PR whose live rollup is passing, or with
  `ci_verified: true` set by whoever read the rollup after the rebase; a `DIRTY` merge state
  is refused (rebase first).
- `none` records "verified, nothing to change" (also the action for `rebase-only`, since the
  rebase itself is done by `pr-rebase.sh` before apply).

Subagent brief (copy, fill the angle brackets):

```
Groom these open PRs read-only: <numbers> (category <category>). Snapshot dir: <dir>
(prs.txt, triage.tsv, mbo-rows.txt). For each: check whether the PR still applies to
origin/main (paths it touches, index.md rows, merged PRs covering the same slug), its merge
state and CI, and — for bugfixes — whether the bug reproduces on origin/main. Decide the
verdict with this table: <§2 table>. Cite evidence you actually checked. Do not push, close,
or comment. Write ONLY the JSON array above to <dir>/verdicts-<category>.json.
```
