# Evidence: F9 fail-open shell gating

- **Owning leaf:** p2-instrument (plan SS7.6 rule 4)
- **Expected proofs (plan SS7.4):** gff_test.sh bash+dash; IH-7, IA-7; P2-T5 real-install evidence

Captures land here per plan SS7.6: gate command output via tee, 3-line dated header,
append-only (never overwrite), committed with the code that produced it.

Captures:

- `p2-t1-lint-list.txt` — `gff lint` clean + 43 repo-live keys counted via `gff list --json`
- `p2-t2-gff-test-bash-dash.txt` — gff_test.sh 10/10 under bash AND dash (first green run)
- `p2-t2-gff-test-bash-dash-r2.txt` — re-run after shellcheck-directive comments (append-only rule)
- `p2-t3-install-sh-gates.txt` — `bash -n install.sh` clean, helper sourced before first gate, manual SKIP line
- `p2-t4-windows-passthrough.txt` — install_windows.sh syntax/shellcheck clean, WSLENV builder dedup proof, pwsh deferral note
