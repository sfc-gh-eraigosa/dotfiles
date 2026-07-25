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
- **Install:** `bash build.sh` → `${HOME}/opt/bin/gff`
- **Zero-install:** `go run github.com/sfc-gh-eraigosa/dotfiles/sdk/gff@<tag> <verb> …`

## Verbs

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
registered source or a local repo path instead of CWD discovery.

gff writes ONLY to `~/.config/gff/` and the user snapshot dir under
`${XDG_DATA_HOME:-$HOME/.local/share}/gff/snapshots/` — never repo or system files.
