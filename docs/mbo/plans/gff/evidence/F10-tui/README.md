# Evidence: F10 TUI

- **Owning leaf:** p3-tui (plan SS7.6 rule 4)
- **Expected proofs (plan SS7.4):** teatest goldens; post-P3 capture

Captures land here per plan SS7.6: gate command output via tee, 3-line dated header,
append-only (never overwrite), committed with the code that produced it.

- `P3-T1-teatest-cover.txt` — teatest suite green at 90.6% package coverage (unit proof).
- Post-P3 TUI capture (demo proof) — pending, VD-1 addendum.
