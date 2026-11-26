# fleet-herdr-machines — execution cursor

## Preflight (once)

- [x] worker `fleet-herdr-machines/<user>/sync-verb` created
- [x] herdr `machine list --json` shape re-checked (0.9.5)

### Task 1 — herdrmach  (plan Task 1)

- [x] RED: fixture + `herdrmach_test.go`
- [x] RUN-RED
- [x] GREEN: `herdrmach.go`
- [x] RUN-GREEN + coverage

### Task 2 — fleet herdr sync  (plan Task 2)

- [x] RED: `cmd/herdr_test.go`
- [x] RUN-RED
- [x] GREEN: `cmd/herdr.go`
- [x] RUN-GREEN

### Task 3 — S key onto herdrmach  (plan Task 3)

- [x] refactor `tui_cmds.go`; `go test ./...` green

### Task 4 — docs + land  (plan Task 4)

- [x] AGENTS.md / README.md / index.md
- [x] live `fleet herdr sync --dry-run` evidence
- [ ] COMMIT + CHECKPOINT (operator confirms)
