# ghapp — GitHub App credential toolkit

> A fine-grained PAT expires and belongs to a person; a GitHub App's
> installation token lasts an hour and belongs to the App. `ghapp` creates
> the App (manifest flow, one browser round-trip), keeps its key safe, and
> mints tokens on demand. `gcfg` uses it; anything else in this repo can too.

**The problem.** Repo/org administration needs a credential with
`administration:write`. A classic PAT is all-or-nothing and lives in a shell
history; a fine-grained PAT is scoped but still a person's token that expires
on a calendar. Actions' `GITHUB_TOKEN` has no administration permission at all.

**What it does about it.** One `ghapp create` registers a private GitHub App
through GitHub's *manifest flow*: `ghapp` serves a one-page form on localhost,
your browser posts it to GitHub, GitHub redirects back with a code, `ghapp`
exchanges the code for the App id + private key and stores them under
`~/.config/ghapp/` (directory `0700`, `<slug>.pem` `0600`). From then on
`ghapp token --repo owner/repo` mints a one-hour installation token scoped to
that repository. No secret is ever printed except by `token`, whose stdout is
the token and nothing else.

**Reach for it when**

- a CLI needs admin-level GitHub access without a long-lived PAT (gcfg);
- a workflow should use `actions/create-github-app-token` and you need the
  `APP_ID` + private key to put in the repo secrets;
- you want to see exactly what an App can do on a repo (`doctor --repo`).

```console
$ ghapp create --name "gcfg (edward)"
waiting for GitHub to hand back the App (up to 10m0s)…
created App gcfg-edward (id 1234567)
  key:  ~/.config/ghapp/gcfg-edward.pem (0600)
  next: ghapp install --app gcfg-edward   # install it on your account/org, then `ghapp token --repo owner/repo`

$ ghapp install --no-browser
gcfg-edward is installed on:
  sfc-gh-eraigosa          id 87654321 User (all repositories)

$ ghapp doctor --repo sfc-gh-eraigosa/dotfiles
ok    store          ~/.config/ghapp 0700
ok    pem            ~/.config/ghapp/gcfg-edward.pem 0600 ok
ok    jwt            App gcfg-edward (id 1234567) https://github.com/apps/gcfg-edward
ok    installations  1 (sfc-gh-eraigosa=87654321)
ok    token          minted for installation 87654321, ghs_*** (expires 2026-09-06T06:31:00Z)
ok    repo           sfc-gh-eraigosa/dotfiles reachable; permissions: admin,maintain,pull,push,triage

$ GH_TOKEN=$(ghapp token --repo sfc-gh-eraigosa/dotfiles --permissions administration=write) gh api repos/sfc-gh-eraigosa/dotfiles -q .full_name
sfc-gh-eraigosa/dotfiles
```

(Transcript shape from the test stub; ids and dates are illustrative until the
live evidence in `docs/mbo/plans/gcfg/evidence/ghapp/` replaces them.)

## Verbs

| Verb | Does | Exit |
| :-- | :-- | :-- |
| `create [--name] [--manifest file] [--org] [--hook-url] [--port 8479] [--force] [--no-browser] [--timeout 10m]` | manifest flow → `<slug>.pem` + `apps.json` | 0 · 1 (expired code, opener) · 2 (no name) |
| `install [--no-browser]` | opens `github.com/apps/<slug>/installations/new`, then records `account → installation id` | 0 · 1 |
| `token --repo owner/repo \| --org name [--permissions k=v …]` | prints **only** the installation token; discovers + records the installation on first use | 0 · 1 · 2 (no target, bad flag, no app) |
| `status` | apps, key paths + modes, installations — no secrets | 0 · 2 (no app) |
| `doctor [--repo owner/repo]` | store/pem modes, App JWT (`GET /app`), installations, and with `--repo` a real token + `GET /repos/…` permissions | 0 · 1 (a check failed) · 2 (no app) |
| `version` | build metadata | 0 |

Common flags: `--config-dir` (default `~/.config/ghapp`, `$XDG_CONFIG_HOME`
honoured), `--app <slug>` when several Apps are stored.

## Create your own App from the same manifest

The default manifest asks for repository **Administration: write**,
**Metadata: read**, **Contents: read** — what `gcfg` needs. To make a
different App, write a manifest file and pass it:

```json
{
  "name": "my-admin-app",
  "url": "https://github.com/you/your-repo",
  "description": "what it is for",
  "public": false,
  "permissions": { "administration": "write", "metadata": "read" },
  "events": []
}
```

```sh
ghapp create --manifest my-app.json            # under your user
ghapp create --manifest my-app.json --org acme # under an organization you admin
```

Field names follow GitHub's manifest (`hook_url` becomes `hook_attributes`;
`permissions`/`events` become `default_permissions`/`default_events`). The
redirect URL is always `http://127.0.0.1:<port>/callback`; when the port is
busy the next nine are tried. The conversion code GitHub hands back is valid
for one hour — a stale one fails with a clear *expired* error.

## In GitHub Actions

You do not need the `ghapp` binary in a workflow. Put the App id and the PEM
contents in repository secrets and mint the token with
`actions/create-github-app-token`; `gcfg actions install` renders exactly that.

## Library

`pkg/ghapp`: `Create`, `App.Installations`, `App.Token` (cached until
expiry−2m), `App.Info`, `RepoAccess`, `FileStore`, `SignJWT`,
`ParsePrivateKey`. Contracts: [`docs/mbo/plans/gcfg.md`](../../docs/mbo/plans/gcfg.md) §3.2.

## Gotchas

- `Token` redacts itself under `%v`/`%s`/JSON — read `.Value` on purpose.
- `Load` refuses a PEM readable by group/other; `chmod 600` it.
- A second `create` with the same slug is refused without `--force`.
- Build/install: `bash sdk/ghapp/build.sh` (version from the
  `sdk/ghapp/vX.Y.Z` tag into `~/opt/bin/ghapp`).
