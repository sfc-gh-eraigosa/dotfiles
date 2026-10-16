# verdicts.json — the shape every triage subagent returns

One JSON array, one object per issue. `groom-apply.sh` and `groom-report.sh` read exactly these
fields; keep the names.

```json
[
  {
    "number": 123,
    "title": "gss: feature checkpoint cannot create a PR …",
    "verdict": "addressed | partially | open | obsolete",
    "evidence": "1-3 sentences citing paths / PR numbers / SHAs that were actually checked",
    "priority": 3,
    "priority_rationale": "one sentence against the §3 rubric",
    "needs_refactor": false,
    "refactor_notes": ["stale src/gss path", "leaf B merged in #305 — drop it from the checklist"],
    "suggested_action": "close-with-evidence | relabel-only | edit-body | edit-body-and-relabel | none",
    "close_comment": "required when suggested_action is close-with-evidence; cites the evidence",
    "new_body": "required when suggested_action is edit-body*: the refreshed body (markdown). The original is folded beneath it automatically."
  }
]
```

Rules the apply script enforces:

- `priority` must be 1–5; the issue ends with exactly that one `priority:N` label (a label that
  is already right is reported as `already priority:N` and not re-sent).
- `none` means "verified, nothing to change": the priority label is still reconciled, nothing
  else happens, and the report lists the issue as confirmed.
- `close-with-evidence` is honoured only for `addressed` (reason *completed*) and `obsolete`
  (reason *not planned*), and only with a non-empty `close_comment`.
- `edit-body*` needs `new_body`; the current body is appended inside `<details>` so nothing the
  author wrote is lost.

Subagent brief (copy, fill the angle brackets):

```
Groom these issues read-only: <numbers>. For each: `gh issue view N --json number,title,body,
labels,createdAt,comments`; verify against <index.md rows / TRACKING ledgers / code paths /
merged-prs.json>; decide the verdict and a 1-5 priority using this rubric: <§3 table>; say
whether the body needs refactoring and give 2-6 concrete notes. Cite evidence you actually
checked. Return ONLY the JSON array above.
```
