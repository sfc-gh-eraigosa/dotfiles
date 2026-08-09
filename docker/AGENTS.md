# docker/ — build images for the dotfiles CI test image

All the Dockerfiles live here so the guidance below is discovered exactly when
you edit them (progressive loading — the root `AGENTS.md` only carries a
one-line pointer). Build them via the `Makefile`, never `docker build` by hand:

| File | What it is | Built by |
|------|------------|----------|
| `Dockerfile` | The CI **test image**: the repo installed on top of the base. Two-phase (see below). | `make build` (`-f docker/Dockerfile`, context = repo root) |
| `Dockerfile.base` | The **base image** (`ghcr.io/…/dotfiles-base`): ubuntu + apt tooling + docker/dind. Published from **main only** by `.github/workflows/docker-base.yml`. | `make build-base` |
| `helloworld/` | Tiny image used by the Build job's "Docker-in-Docker (Legacy Check)" step. | that CI step |
| `busybox-py/` | Minimal example/fixture image. | — |

The build **context stays the repo root** (`docker build … .`), so every `COPY`
in these Dockerfiles is root-relative — moving the files here changed nothing
about what they copy.

## The three-tier layering — where a new install step belongs (CI performance)

The **Build and Integration Test** job (the most expensive CI job) is kept fast
by a three-tier split. Put a new dependency, tool, package, or config step in
the RIGHT tier or you silently regress build time (or, worse, correctness):

1. **OS-level foundation → `Dockerfile.base`** (published from main only, rebuilt
   weekly). Truly foundational, rarely-changing things: base apt packages,
   docker/dind, the shell itself. Costly to validate — see the base-change note.
2. **App-level external install dependency → `install.sh` `--phase deps`** (the
   **cached** deps layer of `Dockerfile`). The rule of thumb: **if it's an
   external install — an apt/brew package, a downloaded tool/CLI, or a language
   runtime/toolchain (goenv/pyenv/rbenv/nvm) — it's a `deps` step.** Gate it with
   a `gff` flag and add that key to **`_IP_DEPS_FLAGS`** in `install.sh`. Kept in
   the app (not the base) so a PR that changes a deps installer still busts that
   layer and is **validated per PR**; unchanged, it's a cache hit and skipped.
3. **Config / skill / symlink / repo-content step → `install.sh` `--phase
   config`** (the per-commit layer). **If it consumes repo content — links a
   dotfile, syncs a skill/plugin, builds an sdk binary, runs a project script —
   it does NOT belong in deps.** Add its key to **`_IP_CONFIG_FLAGS`**.

**Keep the two lists in `install.sh` in sync with new sections.** Omitting a
deps key makes the step re-run every commit (slow); omitting a config key bakes
it into the cached deps layer so later edits stop taking effect per commit (a
correctness bug). `--phase all` (a normal `./install.sh`) applies no overrides,
so real machines are unaffected.

**Changing `Dockerfile.base` in a PR? CI detects it automatically.** Because the
base publishes from main only, the Build job checks the PR's changed files and
**builds the base locally** (instead of pulling the stale published one) when a
PR touches `docker/Dockerfile.base` or `opt/scripts/docker/dockerd-entrypoint.sh`,
so your base change is validated in that PR. Deps/config changes need no special
handling — their app layers re-validate on their own.

## Caching

`make build` uses buildx; CI sets `BUILD_CACHE="--cache-from=type=gha
--cache-to=type=gha,mode=max"` so the deps layer persists across runs. Locally
`BUILD_CACHE` is empty (plain buildx; cache mounts still apply). See the
`Makefile` `build` / `build-base` targets and `.github/workflows/docker-*.yml`.
