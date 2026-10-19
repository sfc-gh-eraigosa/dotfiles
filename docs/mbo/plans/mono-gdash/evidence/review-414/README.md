# Evidence — code review of #414, fixes

Review of the merged #414 (mono decks, nav, zoom, theme). Ten findings; seven
fixed here, three not.

| # | Finding | Outcome | Test |
| :-- | :-- | :-- | :-- |
| 1 | double-esc swallowed the second esc; the key after it was dropped | fixed: both escapes forwarded; a non-nav key leaves nav mode and is handled as usual | `TestDoubleEscForwardsBoth`, `TestNavOtherKeyLeaves` |
| 2 | prefix in nav mode was eaten; the next key went to the program | fixed: the prefix leaves nav mode and starts a command | `TestNavPrefixRunsCommand` |
| 3 | `esc esc ctrl+b z` in one read dropped the prefix key; a trailing prefix left the router waiting | fixed: the key runs as a command; nav mode waits while the router is pending | `TestNavBurstKeepsPrefixKey` |
| 4 | gdash auto-zooms and hides htop when it is the first usable tool | not changed — that is the requested "gdash uses the entire middle" | — |
| 5 | `ctrl+b r` on a deck re-read the config file | fixed: re-checks tools against the running config | `TestDeckRestartRechecksWithoutReload` |
| 6 | deck detail lines overflowed the config screen | fixed: the list is budgeted in lines and scrolls to the selection | `TestDeckDetailFitsTheScreen` |
| 7 | config preview ignored `middle: single` | fixed: preview uses `Effective()` | `TestPreviewFollowsMiddleSingle` |
| 8 | `every`/`probe`/`cwd`/`fallbacks` on a deck pane were accepted and ignored | fixed: rejected with the field named | `TestDeckValidates` (4 cases) |
| 9 | configui painted Accent with OK; two copies of `scheme()` | fixed: one `theme.Chrome` used by both | `TestChrome` |
| 10 | palette re-resolved per paint; mono ignored `NO_COLOR` | fixed in mono (`theme.ChromeFor` once, honors `NO_COLOR`); gdash's per-segment `palette()` is a struct copy, left | `TestFor`, `TestChrome` |

Each new test was run against the unfixed code first and failed
(`TestNav*`, `TestDoubleEscForwardsBoth`, `TestDeckRestart…`,
`TestDeckDetailFits…`, `TestPreviewFollows…`), then passed.

Gates on this branch: `go test -race ./...` in sdk/mono, sdk/libs, sdk/gdash —
all ok; mono coverage 84.7%; golangci-lint 0 issues (all three); gosec clean;
govulncheck: no vulnerabilities called.

CLI check (finding 8):

```text
$ mono config set panes.mid.cwd /srv
Error: panes.mid.cwd: a deck ignores it — set it on each view (a probe also fits on a tool)
$ mono config set panes.htop.cwd /tmp
saved …/m.json
```
