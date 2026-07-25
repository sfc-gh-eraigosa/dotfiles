# gff evidence tree (plan SS7.6)

One folder per spec feature F1-F11, plus e2e/ (harness runs) and demo/ (VD-1).
Each capture: gate command re-run with tee, dated 3-line header, append-only,
committed with the work. A feature without captured evidence is not done (SS7.5).

| Folder | Feature | Owning leaf |
| :-- | :-- | :-- |
| F01-keys .. F08-write-path, F11-gorun, e2e | F1-F8, F11 + harness | p1-engine |
| F09-gating | F9 | p2-instrument |
| F10-tui | F10 | p3-tui |
| demo | VD-1 | vd-demo |
