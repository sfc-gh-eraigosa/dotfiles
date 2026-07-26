# gff — git fast features

Layered feature flags persisted in git. A repo declares its flags in a tracked
flag file (`.gff/features.yaml` or `.github/gff/features.yaml`); a 5-layer
resolver (system snapshot → user snapshot → live repo file → system override →
user override) computes effective values with winning-layer attribution; shell
scripts consume flags via `gff export --format shell` env vars plus the
fail-open `gff_on` helper in `opt/lib/gff.sh`.

- **Design / spec / plan:** `docs/mbo/{designs,specs,plans}/gff.md` (issue #180)
- **Schema:** protobuf (`proto/gff/v1/features.proto`), committed codegen under
  `gen/gff/v1/` — regenerate with `make gff-proto` (raw protoc, no buf)

## Quickstart

```sh
# 1. Install (from the repo root; builds + installs to ${HOME}/opt/bin/gff)
make gff-install

# 2. Declare flags in your repo — .gff/features.yaml (or .github/gff/features.yaml)
cat > .gff/features.yaml <<'YAML'
namespace: com.github.you.yourrepo
sets:
  - area: install
    features:
      - path: install.ai.claude
        description: Claude CLI setup
        boolDefault: true
      - path: install.pkg.manager
        description: Package manager selection
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt,  description: Debian/Ubuntu apt, stringValue: apt}
YAML

# 3. Validate, register, inspect
gff lint            # exit 0 = clean
gff install         # registers the repo + snapshots it for cross-repo use
gff list            # PATH TYPE VALUE LAYER DESCRIPTION

# 4. Read and write flags
gff get install.ai.claude          # -> true
gff set install.ai.claude false    # writes ONLY ~/.config/gff/config.yaml (0600)
gff list --json                    # machine-readable, winning layer included
gff unset install.ai.claude        # back to the default

# 5. Gate shell steps (fail-open: unset/garbage/missing-binary => run)
eval "$(gff export --format shell)"
. opt/lib/gff.sh
gff_on install.ai.claude && ./setup-claude.sh || gff_skip_msg install.ai.claude
```

## Make targets (repo root)

| Target | What it does |
| :-- | :-- |
| `make gff-build` | compile only |
| `make gff-test` | unit suite + the coverage bars (total ≥90, resolve ≥95, schema ≥90) |
| `make gff-install` | build + install to `${HOME}/opt/bin/gff` (ldflags-stamped) |
| `make gff-e2e` | binary-level e2e harness (25 IH/IA scenarios) |
| `make gff-proto` / `gff-proto-check` | regenerate proto output / assert committed output is clean |

## Verbs (`gff help`, `gff <verb> --help`)

```
gff get <key>              print effective value (exit 2 unknown key)
gff enabled <key>          exit 0 on / 1 off / 2 unknown-or-not-a-bool
gff selected <key> <id>    exit 0 selected / 1 not / 2 unknown key or option id
gff set <key> <value>      write the user override (~/.config/gff/config.yaml, 0600)
gff unset <key>            remove the user override for the key
gff list [--json]          table or JSON of every resolved flag + winning layer
gff lint [path]            lint a flag file (default: the discovered repo file)
gff export --format shell|dotenv|json|yaml [-o <file>]
gff install                register the CWD repo + snapshot into the user layer
gff version                version block
```

All read verbs accept `--source <namespace|path>` to scope resolution to one
registered source or a local repo path instead of CWD discovery. Exit code 2
always means a usage/definition error (unknown key/option/source, wrong type) —
shell callers treat ≥2 as fail-open.

## Layers & provenance — reading the LAYER column

Every value gff prints was won by exactly one of five layers; `gff list`'s
LAYER column names it. Resolution order (later wins):

| # | Layer | Backing file | When it wins |
| :-- | :-- | :-- | :-- |
| 1 | `system-snapshot` | `/opt/conf/gff/<ns>.yaml` (admin-provisioned) | fleet-wide defaults, nothing else defines the key |
| 2 | `user-snapshot` | `${XDG_DATA_HOME:-~/.local/share}/gff/snapshots/<ns>.yaml` (written by `gff install`) | resolving from OUTSIDE the repo — incl. `--source <namespace>` — via the installed snapshot |
| 3 | `repo-live` | the repo's tracked `.gff/features.yaml` / `.github/gff/features.yaml` | you are standing inside the repo (or used `--source <path>`), so the live tracked file supplies the definition |
| 4 | `system-override` | `/var/opt/conf/gff/config.yaml` | an admin flipped the key machine-wide |
| 5 | `user-override` | `~/.config/gff/config.yaml` (0600 — the ONLY file `set` writes) | you ran `gff set <key> <value>` |

Layers 1–3 are *definition* layers (they carry the flag's schema + default);
4–5 are sparse *override* maps (just `key: value` lines). So the column flips
are meaningful, not noise:

```sh
cd myrepo && gff list          # repo-live      — the tracked file's defaults
gff set install.ai.claude false
gff list                       # user-override  — your set is winning
gff unset install.ai.claude
gff list                       # repo-live      — default restored
cd / && gff list               # user-snapshot  — resolved via the installed snapshot
gff list --source com.example.demo   # user-snapshot from anywhere
```

Machine state lives in exactly three files: the registry
(`~/.config/gff/sources.yaml`: namespace/url/commit per `gff install`), the
snapshots dir (byte-identical copies of each repo's flag file — flags keep
resolving after a clone moves or disappears), and your override file. Delete
the override entry (`gff unset`) and the layer visibly reverts — that
round-trip is the quickest health-check that the whole chain works.

## Use cases

- **Gate installer steps per machine** — the dotfiles repo enumerates every
  `install.sh` component as an `install.*` flag; `gff set install.windows.wispr-flow false`
  skips that step on the next run, everywhere the override file lives.
- **Radio/checkbox choices with typed payloads** — `install.pkg.manager` as a
  `single` choice (`auto`/`apt`/`brew`), consumed from Go via
  `gff.StringValues("install.pkg.manager")` or from shell via `GFF_INSTALL_PKG_MANAGER`.
- **Cross-repo flags** — `gff install` snapshots a repo's flags under its
  reverse-DNS namespace; `gff get --source <namespace> <key>` works from any CWD,
  even after the clone moves or disappears.
- **Zero-install consumers** — CI or fresh machines with only a Go toolchain:
  `eval "$(go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> export --format shell --source <namespace>)"`.
- **Go SDK** — `import "github.com/sfc-gh-eraigosa/dotfiles/sdk/gff/pkg/gff"`:
  `Bool`, `Selected`, `IsSelected`, `IntValues`/`FloatValues`/`StringValues`/`BoolValues`.

## Versioning & releases

`VERSION` in this directory is the source of truth the build stamps into the
binary (`build.sh` ldflags). Releases are automated repo-wide:

1. Merge a conventional-commit PR touching `sdk/gff/**` — `sdk-auto-bump.yml`
   derives the semver bump and commits the new `sdk/gff/VERSION` on `main`.
2. `tag-sdk-modules.yml` then cuts the annotated tag `sdk/gff/v<VERSION>`.
3. Consumers pin it: `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@sdk/gff/v<VERSION>`.

So the default is the committed `VERSION`, and published tags always mirror it —
no manual tagging.

## Troubleshooting

| Symptom | Cause / fix |
| :-- | :-- |
| `gff get` exits 2 `unknown flag key` | Key not defined in any layer for this CWD. Check `gff list`, or scope with `--source <namespace>`; multi-namespace ambiguity wants the qualified `<namespace>:<key>` form. |
| `unknown source` (exit 2) on `--source` | The name isn't registered (`gff install` in that repo first) or the path isn't inside a git repository. |
| `gff lint` fails naming the file + line | The flag file is malformed YAML or violates a lint rule (3-segment kebab keys, no negative names, homogeneous choice value types, exactly one default-selected id in `single` mode, every feature needs a default). |
| `set` refused, file untouched | Validation happens before writing: unknown option id (exit 2) or two ids on a `single`-mode choice (exit 1) never modify the override. |
| Changes don't take effect in a running shell | `gff set` writes the override file; re-run `eval "$(gff export --format shell)"` (or your installer) to refresh the exported env. |
| `install.sh` step ran even though its flag is false | The gate is fail-open by design: only the exact lowercase string `false` skips; `FALSE`, `0`, unset, or a broken gff all run the step. Verify with `gff get <key>` and re-export. |
| `make gff-proto` fails: `protoc not found` | protoc is needed only to REGENERATE (`apt-get install protobuf-compiler` / `brew install protobuf`); normal builds use the committed `gen/` output. |
| Registry corrupt (`sources.yaml`) | `install` names the bad file; named `--source` lookups degrade to exit 2. Fix or delete `~/.config/gff/sources.yaml` and re-run `gff install` in the source repos. |

gff writes ONLY to `~/.config/gff/` and the user snapshot dir under
`${XDG_DATA_HOME:-$HOME/.local/share}/gff/snapshots/` — never repo or system files.
