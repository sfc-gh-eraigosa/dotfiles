# fleet-tui evidence

Per-task captures of a **done-when gate at the moment it passed** — a record of
something observed once, which cannot be re-derived later.

## What does NOT belong here

Rendered TUI frames. They were committed as `demo/frames.txt` and
`logpane/frames.txt` and both were deleted, because a snapshot of a
deterministic test is not evidence:

- `TestDemoFrames` already asserts every frame's content and width, on every CI
  run. The test is the guarantee; a committed copy verifies nothing.
- It went stale the instant the UI changed — 1,700+ lines of diff across seven
  commits, which buried the real changes in review.
- Two copies drifted apart, so at least one was misrepresenting the UI.

Regenerate them any time, in colour:

```sh
cd sdk/fleet
FLEET_DEMO=1 go test ./cmd/ -run TestDemoFrames -v
```

The rule: capture what you cannot reproduce (a live run against real hardware,
a one-off measurement, a gate's output at a point in time). Do not capture what
a test reproduces on demand.
