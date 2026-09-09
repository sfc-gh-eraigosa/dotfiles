# deps — dependency blast-radius proof (design §5)

`gss` is a `sdk/libs` consumer (`replace … => ../libs`) that does **not** import `libs/tui`.
Adding bubbletea + testify to `libs/go.mod` must not change what gss links.

| Build | File | Size (bytes) |
| :-- | :-- | --: |
| before — clean `main` @ `93e0d02`, plan command `go build -o /tmp/gss-before .` | `gss-size-before.txt` | 7675540 |
| after — lib worker (Task 7), plan command `go build -o /tmp/gss-after .` | `gss-size.txt` | 7675876 |
| before — normalized `go build -trimpath -buildvcs=false` | `gss-size.txt` | 7672586 |
| after — normalized `go build -trimpath -buildvcs=false` | `gss-size.txt` | 7672586 |

**Raw delta: +336 bytes. Explained, not a dependency change.** `diff <(go version -m gss-before) <(go version -m gss-after)`
shows an identical module list (every `dep` line, including `golang.org/x/sys v0.37.0`, is the same); the only
differences are the build stamps: the `main` build carries `vcs=git / vcs.revision / vcs.time / vcs.modified` lines
and the worker build carries none, and — without `-trimpath` — each binary embeds its own absolute source paths,
which are ~60 characters longer under the gss worker tree than under `~/git/dotfiles`.

**Normalized (trimpath, no VCS stamp): byte-identical**, sha256 `4194b1688cd62c6c7a18b75aea7082500715b8f2e8c745091d0fb969c336b6bb` for both.
Nothing in the new `libs/tui` packages, nor the two new `libs/go.mod` requirements, reaches a consumer that does not import them.
