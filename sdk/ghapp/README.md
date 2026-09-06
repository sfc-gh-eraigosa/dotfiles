# ghapp — GitHub App credential toolkit

> A fine-grained PAT expires and belongs to a person; a GitHub App's
> installation token lasts an hour and belongs to the App. `ghapp` creates
> the App (manifest flow, one browser round-trip), keeps its key safe, and
> mints tokens on demand.

**Status:** P0 scaffold — `version` only. Verbs land task by task per
[`docs/mbo/plans/gcfg.md`](../../docs/mbo/plans/gcfg.md) §4 P0.

```console
$ go run . version
ghapp vdev
  Commit:      none
  Dirty:       false
  Build Date:  unknown
  Description: ghapp — GitHub App credential toolkit: manifest-flow create, installation tokens
```

Build/install: `bash sdk/ghapp/build.sh` (stamps the version from the
`sdk/ghapp/vX.Y.Z` tag into `~/opt/bin/ghapp`).
