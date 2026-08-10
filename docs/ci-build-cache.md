# CI build cache — operations runbook

How the Docker layer cache in **Docker Image CI** (`.github/workflows/docker-image.yml`)
is authenticated, how to verify it, and what to do when it breaks.

**TL;DR — there is no secret to configure.** The cache authenticates with
credentials GitHub mints automatically for every job. Nothing to create, store,
rotate, or renew. If you came here looking for "where do I set
`ACTIONS_RUNTIME_TOKEN`", the answer is: **you don't** — you expose it, and the
workflow already does. Read [§1](#1-how-it-works) then
[§4](#4-when-ci-fails-hard-on-the-cache).

---

## 1. How it works

PR #217 split `docker/Dockerfile` into a cached **deps** layer and a per-commit
**config** layer, and asked buildx to persist those layers between runs with
`--cache-to=type=gha`. That backend needs two environment values:

| Variable | Meaning | Where it comes from |
|---|---|---|
| `ACTIONS_RUNTIME_TOKEN` | Auth token for the Actions cache service | Minted **per job** by GitHub. Ephemeral — expires with the job. |
| `ACTIONS_RESULTS_URL` | Endpoint of the **v2** cache service | Set by the runner. |

> The older `ACTIONS_CACHE_URL` addressed the **v1** cache service, which GitHub
> shut down on **2025-04-15**. Only the URL variable changed in that migration —
> the token variable has always been `ACTIONS_RUNTIME_TOKEN`.

**The catch that broke #217:** GitHub injects these into
`docker/build-push-action` automatically, but **not** into a plain
`run: docker buildx build` step — which is what `make build` is. Docker's own
docs say so: *"If you invoke the `docker buildx` command manually from an inline
step, then the variables must be manually exposed."*

So the workflow exposes them explicitly:

```yaml
- name: Expose GitHub Actions runtime credentials (for the type=gha build cache)
  uses: crazy-max/ghaction-github-runtime@<sha>  # v4.0.0
```

That action re-exports every `ACTIONS_*` variable the runner holds into
`$GITHUB_ENV` (it is a wildcard, not a fixed list), after which `make build`
inherits them.

### Why this failure was invisible

With the credentials **absent**, buildx does not error — it skips the backend
entirely. Verified on the #217 post-merge run
([31332375184](https://github.com/sfc-gh-eraigosa/dotfiles/actions/runs/31332375184)):
zero `exporting cache` / `importing cache` stages, no warning, no 401, and the
job still went **green** in 7m33s while rebuilding everything from scratch.

That is the whole reason the preflight in §2 exists: a dead cache costs minutes
per run and reports nothing.

---

## 2. The guardrails (why CI fails hard)

Two checks, in order:

1. **Preflight — `Assert the build cache backend is live`.** Runs *before* the
   build and fails the job if `ACTIONS_RUNTIME_TOKEN` or `ACTIONS_RESULTS_URL`
   is empty. This is the credentials-absent case — the only mode that degrades
   silently.
2. **Exit code.** buildx defaults `ignore-error=false` for `cache-to`, so once
   the credentials are present, a cache *export* failure (bad token, service
   outage, 401) already fails the step on its own. We deliberately do **not**
   pass `ignore-error=true`.

Together these cover both modes. We intentionally do **not** grep the build
output for `exporting cache`: that string varies with `--progress` mode, TTY
presence, and BuildKit version, so it is a false-failure generator.

---

## 3. Verify the cache is working

**From a run's logs** — a warm build shows import and export stages:

```
importing cache manifest from gha:...
=> CACHED [stage-0 6/10] RUN ... install.sh --phase deps
exporting cache to GitHub Actions Cache
```

The `CACHED` on the `--phase deps` step is the signal that matters: a cold build
runs that step for ~2 minutes, a warm one skips it.

**List the stored entries:**

```bash
gh api repos/sfc-gh-eraigosa/dotfiles/actions/caches --jq '.actions_caches[] | "\(.key)\t\(.size_in_bytes)"'
```

**Reproduce locally** (no GHA cache; uses the local buildx cache instead):

```bash
make build                       # BUILD_CACHE is empty by default
```

---

## 4. When CI fails hard on the cache

### `BUILD CACHE BROKEN: missing runtime env: ACTIONS_RUNTIME_TOKEN`

The preflight fired. In order of likelihood:

1. **The expose step was removed or reordered.** It must sit *after*
   `Set up Docker Buildx` and *before* the preflight. Restore it.
2. **The pinned action failed to run** (e.g. a bad SHA after a Dependabot bump).
   Check the step's own log.
3. **GitHub renamed the variables again** (as in the v1→v2 migration). Confirm
   against <https://docs.docker.com/build/cache/backends/gha/>, then update both
   the preflight and the table in §1.

There is **no secret to regenerate** — see §5 if you believe otherwise.

### The build succeeds but is slow, and `--phase deps` never shows `CACHED`

Expected in these cases, none of which are faults:

- **First run on a new branch.** Nothing to import yet.
- **The PR touches `opt/`, `sdk/`, `install.sh`, or `Makefile`.** These are the
  deps layer's `COPY` inputs, so the layer correctly busts and re-validates.
  This is by design (see `docker/AGENTS.md`).
- **Cache evicted.** GitHub keeps **10 GB per repository**, evicting entries
  untouched for **>7 days**, oldest-by-last-access first once over the cap.
- **Branch scoping.** A run reads caches from its own branch, the default
  branch, and (for PRs) its base branch — never from sibling or child branches.
  A long-lived branch whose base has moved on will miss until it lands.

---

## 5. Why there is no Actions secret here

A PAT-backed **registry** cache (`type=registry` against GHCR) was considered and
**rejected**. It persists across branches and dodges the 10 GB cap, but it
requires a personal access token stored as an Actions secret — which expires,
silently disabling the cache until someone notices, and needs a documented
rotation ritual plus an expiry probe in CI.

The `type=gha` credentials are minted per job and cannot expire in storage
because they are never stored. That removes the rotation problem outright rather
than automating around it.

**If a future change does introduce a PAT-backed cache**, it must ship with an
expiry preflight that fails hard, e.g.:

```yaml
- name: Preflight — registry cache auth
  run: |
    code=$(curl -s -o /dev/null -w '%{http_code}' \
      -H "Authorization: Bearer ${REGISTRY_PAT}" \
      "https://ghcr.io/v2/${OWNER}/dotfiles-base/tags/list")
    if [ "$code" = "401" ]; then
      echo "::error::Cache PAT expired. Regenerate and run: gh secret set REGISTRY_PAT"
      exit 1
    fi
```

Do not add a cache credential without that gate.

---

## 6. Related

- [`docker/AGENTS.md`](../docker/AGENTS.md) — the three-tier layering rule that
  decides whether a step belongs in the base image, `--phase deps`, or
  `--phase config`. **Read before adding any install step.**
- [`.github/workflows/docker-image.yml`](../.github/workflows/docker-image.yml) —
  the workflow itself.
- Docker's backend reference — <https://docs.docker.com/build/cache/backends/gha/>
