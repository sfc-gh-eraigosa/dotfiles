# Evidence: F9 fail-open shell gating

- **Owning leaf:** p2-instrument (plan SS7.6 rule 4)
- **Expected proofs (plan SS7.4):** gff_test.sh bash+dash; IH-7, IA-7; P2-T5 real-install evidence

Captures land here per plan SS7.6: gate command output via tee, 3-line dated header,
append-only (never overwrite), committed with the code that produced it.

_No captures yet._
