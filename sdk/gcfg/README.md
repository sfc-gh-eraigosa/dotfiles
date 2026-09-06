# gcfg — GitHub settings as code

> Repository settings drift in a web UI where nothing records who changed
> what. `gcfg` puts them in `.github/gcfg.yaml`, verifies that file against
> the live repo in CI, and applies the difference only when you say so.

**Status:** P1 scaffold — `version` only. Verbs land task by task per
[`docs/mbo/plans/gcfg.md`](../../docs/mbo/plans/gcfg.md) §4.

```console
$ go run . version
gcfg vdev
  Commit:      none
  Dirty:       false
  Build Date:  unknown
  Description: gcfg — GitHub settings as code: .github/gcfg.yaml exported, verified, applied
```

Planned surface (plan §3.3): `init · export · lint · schema · verify · plan ·
apply · tui · actions install|uninstall · auth · version`, with
`-R owner/repo`, `--auth env|gh|app|auto`, and exit codes 0 clean · 1 drift ·
2 usage/no credential.

Credentials come from `GH_TOKEN`, `GITHUB_TOKEN`, a `gh` login, or a GitHub
App via [`sdk/ghapp`](../ghapp/README.md).

Build/install: `bash sdk/gcfg/build.sh` (version from the `sdk/gcfg/vX.Y.Z`
tag into `~/opt/bin/gcfg`).
