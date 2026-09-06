# gcfg — GitHub settings as code

> Your repository's settings, declared in `.github/gcfg.yaml`, checked in CI,
> and changed only on purpose.

**The problem.** Repository settings live in a web UI that records nothing. Six
months later the wiki is on, `delete_branch_on_merge` is off, secret scanning
was never enabled on the newest repo, and nobody can say when any of it changed
or whether it was deliberate. A script per setting is a write with no way to
ask "is it still like that?"

**What it does about it.** One file says what the settings should be. `export`
writes that file from whatever is live, so an existing repo adopts it without
anyone typing YAML. `verify` fails a PR when the live repo disagrees, naming
the key. `apply` makes the change and then reads every setting back, because
GitHub answering 200 is not the same as a setting that changed.

```console
$ gcfg export --out .github/gcfg.yaml
wrote .github/gcfg.yaml (sfc-gh-eraigosa/dotfiles)

$ gcfg verify
sfc-gh-eraigosa/dotfiles: clean (2 families checked)

# someone turns the wiki off in the UI:
$ gcfg verify
sfc-gh-eraigosa/dotfiles: 1 drift (2 families checked)

general
  drift      features.wiki
             want true
             live false

$ gcfg apply --yes
update general.features.wiki: false → true
sfc-gh-eraigosa/dotfiles: clean (2 families checked)
```

## Verbs

| Verb | Does | Exit |
| :-- | :-- | :-- |
| `init [--from owner/repo] [--force]` | write a starter file, or copy another repo's | 0 · 2 |
| `export [--out path\|-] [--force] [--only …]` | write live state to the file | 0 · 2 |
| `lint [--json]` | check the file alone — no network, no credential | 0 · 2 |
| `schema [--out path]` | print the JSON Schema editors complete against | 0 |
| `verify [--only …] [--json\|--markdown]` | compare the file with the live repo | 0 · 1 · 2 |
| `plan [--only …] [--json]` | show what apply would change, write nothing | 0 · 1 · 2 |
| `apply [--yes] [--dry-run] [--only …]` | change it, then read it back | 0 · 1 · 2 |
| `auth status` | which credential gcfg would use, and where from | 0 · 2 |
| `version` | build metadata | 0 |

**Exit codes are the contract:** `0` clean · `1` something needs a human ·
`2` the file or the credential is the problem. That makes `gcfg verify` a CI
gate with no extra glue.

Common flags: `-R owner/repo` (default: this checkout's origin), `-f path`
(default `.github/gcfg.yaml`), `--auth env|gh|app|auto`, `--org`, `--no-color`.

## The file

```yaml
version: 1
ownership: declared        # declared | full, per file or per family
repo:
  general:
    description: "…"
    features: {issues: true, wiki: false}
    merge: {squash: true, delete_branch_on_merge: true}
  security:
    secret_scanning: true
    push_protection: true
```

- **Every key is optional.** A key the file does not mention is *unmanaged*:
  reported, never changed. That is what makes adoption incremental rather than
  all-or-nothing.
- **`ownership: full`** flips that for a family: anything live but undeclared
  becomes drift, and `apply` removes it.
- **Unknown keys are an error** naming the key and its line — a typo silently
  managing nothing is the failure mode this avoids.
- **Secrets by name only.** Actions secrets and webhooks are declared by
  presence; `lint` rejects any value shaped like a credential, because the file
  is committed.

`gcfg schema --out .github/gcfg.schema.json` publishes the JSON Schema, so an
editor completes the file and CI can fail on drift between schema and structs.

## Families

A *family* is one coherent group of settings that is read, diffed and applied
on its own. One family failing never stops the rest: a family the credential
cannot read is reported as `unreadable`, not raised as an error — unless no
family could be read at all, which exits 2 rather than claiming "no drift".

Today: `general` (description, homepage, topics, visibility, default branch,
feature tabs, merge settings) and `security` (secret scanning, push protection,
non-provider patterns, Dependabot alerts and security updates, private
vulnerability reporting). The rest of the families in the plan land one task at
a time.

## Credentials

Resolved in order: `GH_TOKEN`, `GITHUB_TOKEN`, a `gh` login (from `hosts.yml`
or, when gh keeps it in the system keyring, `gh auth token`), then a GitHub App
via [`sdk/ghapp`](../ghapp/README.md). `--auth env|gh|app` pins one source
instead of falling back. `gcfg auth status` says which one answers and never
prints the value.

Writes need repository **Administration** permission. Actions' built-in
`GITHUB_TOKEN` does not have it, so a workflow must bring a fine-grained PAT or
an App token.

## Gotchas

- Off a terminal, `apply` refuses without `--yes` and writes nothing.
- `apply` re-reads: a setting GitHub accepts and ignores is reported as
  `not_honoured` with the reason, not as drift that never clears.
  `non_provider_patterns` does exactly that without GitHub Secret Protection.
- The `org:` block only works in the organization's own `.github` repository;
  `lint` says so when it is anywhere else.
- Build/install: `bash sdk/gcfg/build.sh` stamps the version from the
  `sdk/gcfg/vX.Y.Z` tag into `~/opt/bin/gcfg`.

→ [Agent context](./AGENTS.md) · [plan](../../docs/mbo/plans/gcfg.md) ·
[spec](../../docs/mbo/specs/gcfg.md)
