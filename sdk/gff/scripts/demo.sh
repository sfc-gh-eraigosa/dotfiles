#!/usr/bin/env bash
# demo.sh — gff end-to-end walkthrough: "flags for a fresh repo in two minutes".
# Narrated, re-runnable, and fully sandboxed: every step runs against a scratch
# HOME (GFF_DEMO_HOME) so it never touches your real ~/.config/gff.
# Steps follow docs/mbo/plans/gff.md §7.3 exactly.
#
# Optional: GFF_DEMO_TAG=v0.1.0 switches the step-6 finale to the true
# zero-install form (`go run <module>@<tag>`, needs network + module proxy);
# the default uses the module-local `go run .` stand-in, same code path.
set -euo pipefail

REPO_ROOT=$(cd -- "$(dirname "$0")/../../.." && pwd -P)
MODULE=github.com/sfc-gh-eraigosa/dotfiles/sdk/gff
NS=com.example.gffdemo

GFF_DEMO_HOME=${GFF_DEMO_HOME:-$(mktemp -d "${TMPDIR:-/tmp}/gff-demo.XXXXXX")}
DHOME="${GFF_DEMO_HOME}/home"
BIN="${GFF_DEMO_HOME}/bin"
mkdir -p "${DHOME}" "${BIN}"
printf '[user]\n\tname = gff-demo\n\temail = demo@example.com\n' > "${GFF_DEMO_HOME}/gitconfig"

step() { printf '\n══ STEP %s ═════════════════════════════════════════════\n   %s\n\n' "$1" "$2"; }
show() { printf '$ %s\n' "$*"; }
# G: run a command inside the scratch world (never the real HOME). The Go
# caches stay real so step 6's `go run` doesn't re-download the world into
# the scratch HOME.
GO_ENV_KEEP=(GOMODCACHE="$(go env GOMODCACHE)" GOPATH="$(go env GOPATH)" GOCACHE="$(go env GOCACHE)")
G() { env HOME="${DHOME}" XDG_DATA_HOME="${DHOME}/.local/share" \
      GIT_CONFIG_GLOBAL="${GFF_DEMO_HOME}/gitconfig" GIT_CONFIG_NOSYSTEM=1 \
      "${GO_ENV_KEEP[@]}" "$@"; }

printf 'gff end-to-end demo — %s\nscratch world: %s (your real HOME is untouched)\n' \
  "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "${GFF_DEMO_HOME}"

show "go build -o \${GFF_DEMO_HOME}/bin/gff  (from ${MODULE})"
( cd "${REPO_ROOT}/sdk/gff" && go build -o "${BIN}/gff" . )
GFF="${BIN}/gff"

step 1 "Scaffold a demo repo; author a flag file: 1 bool + 1 radio + 1 checkbox (typed values)"
DEMO_REPO="${GFF_DEMO_HOME}/gffdemo"
mkdir -p "${DEMO_REPO}/.gff"
( cd "${DEMO_REPO}" && G git init -q . \
  && G git remote add origin "https://example.com/gffdemo.git" ) # url derives ${NS}
cat > "${DEMO_REPO}/.gff/features.yaml" <<YAML
namespace: ${NS}
sets:
  - area: demo
    features:
      - path: demo.ui.dashboard
        description: Render the dashboard pane
        boolDefault: true
      - path: demo.pkg.manager
        description: Package manager selection (radio)
        choiceDefault:
          mode: CHOICE_MODE_SINGLE
          options:
            - {id: auto, description: Auto-detect, stringValue: auto, selected: true}
            - {id: apt,  description: Debian/Ubuntu apt, stringValue: apt}
            - {id: brew, description: Homebrew, stringValue: brew}
      - path: demo.shell.plugins
        description: Optional shell plugins (checkbox)
        choiceDefault:
          mode: CHOICE_MODE_MULTI
          options:
            - {id: fzf,      description: Fuzzy finder, stringValue: fzf, selected: true}
            - {id: starship, description: Prompt,       stringValue: starship, selected: true}
            - {id: zoxide,   description: Smart cd,     stringValue: zoxide}
YAML
sed -n '1,8p' "${DEMO_REPO}/.gff/features.yaml"
echo "  … (radio + checkbox options with typed stringValue payloads)"

step 2 "lint → install → list — note the LAYER (winning-layer provenance) column"
show "gff lint"
( cd "${DEMO_REPO}" && G "${GFF}" lint ) && echo "lint: clean (exit 0)"
show "gff install"
( cd "${DEMO_REPO}" && G "${GFF}" install )
show "gff list   (from \$HOME — a foreign CWD — via the registered snapshot)"
( cd "${DHOME}" && G "${GFF}" list --source "${NS}" )

step 3 "Gate a toy script with gff_on; flip the bool off; rerun shows the SKIP line; flip back"
TOY="${GFF_DEMO_HOME}/toy.sh"
cat > "${TOY}" <<TOY
#!/bin/sh
. "${REPO_ROOT}/opt/lib/gff.sh"
if gff_on demo.ui.dashboard; then echo "RUN  (dashboard step executed)"; else gff_skip_msg demo.ui.dashboard; fi
TOY
chmod +x "${TOY}"
show "eval \"\$(gff export --shell)\" && sh toy.sh    # default: on"
( cd "${DEMO_REPO}" && eval "$(G "${GFF}" export --shell)" && export GFF_DEMO_UI_DASHBOARD && sh "${TOY}" )
show "gff set demo.ui.dashboard false && re-eval && sh toy.sh    # now: SKIP"
( cd "${DEMO_REPO}" && G "${GFF}" set demo.ui.dashboard false \
  && eval "$(G "${GFF}" export --shell)" && export GFF_DEMO_UI_DASHBOARD && sh "${TOY}" )
show "gff unset demo.ui.dashboard && re-eval && sh toy.sh    # default restored"
( cd "${DEMO_REPO}" && G "${GFF}" unset demo.ui.dashboard \
  && eval "$(G "${GFF}" export --shell)" && export GFF_DEMO_UI_DASHBOARD && sh "${TOY}" )

step 4 "Export all four formats; eval the shell form in dash; parse the .env"
for fmt in shell dotenv json yaml; do
  printf -- '--- gff export --format %s ---\n' "${fmt}"
  ( cd "${DEMO_REPO}" && G "${GFF}" export --format "${fmt}" )
done
show "sh (dash) -c 'eval \"\$(gff export --format shell)\" …'   # POSIX-eval proof"
( cd "${DEMO_REPO}" && G sh -c "eval \"\$($GFF export --format shell)\"; echo \"dash sees: GFF_DEMO_PKG_MANAGER=\$GFF_DEMO_PKG_MANAGER\"" )
show "gff export --format dotenv -o .env && parse it line by line"
( cd "${DEMO_REPO}" && G "${GFF}" export --format dotenv -o "${GFF_DEMO_HOME}/.env" )
while IFS='=' read -r k v; do printf '  parsed: %s -> %s\n' "$k" "$v"; done < "${GFF_DEMO_HOME}/.env"

step 5 "The guardrail: a DIFFERENT repo url may not claim an already-registered namespace"
IMPOSTOR="${GFF_DEMO_HOME}/impostor"
mkdir -p "${IMPOSTOR}/.gff"
( cd "${IMPOSTOR}" && G git init -q . && G git remote add origin https://example.com/other/impostor.git )
sed "s|^namespace: .*|namespace: ${NS}|" "${DEMO_REPO}/.gff/features.yaml" > "${IMPOSTOR}/.gff/features.yaml"
show "cd impostor && gff install    # same namespace, different url => rejected"
if ( cd "${IMPOSTOR}" && G "${GFF}" install ); then
  echo "ERROR: impostor install unexpectedly succeeded" >&2; exit 1
else
  echo "rejected as expected (registry unchanged; the error names the existing url)"
fi

step 6 "Finale: an empty directory, no gff on PATH — zero-install via go run"
EMPTY="${GFF_DEMO_HOME}/empty"; mkdir -p "${EMPTY}"
if [ -n "${GFF_DEMO_TAG:-}" ]; then
  show "eval \"\$(go run ${MODULE}@${GFF_DEMO_TAG} export --format shell --source ${NS})\""
  ( cd "${EMPTY}" && eval "$(G go run "${MODULE}@${GFF_DEMO_TAG}" export --format shell --source "${NS}")" \
    && echo "zero-install (tagged) sees: GFF_DEMO_PKG_MANAGER=${GFF_DEMO_PKG_MANAGER:-}" )
else
  show "eval \"\$(go run . export --format shell --source ${NS})\"   # module-local stand-in for @<tag>"
  ( cd "${EMPTY}" && eval "$(cd "${REPO_ROOT}/sdk/gff" && G go run . export --format shell --source "${NS}")" \
    && echo "zero-install sees: GFF_DEMO_PKG_MANAGER=${GFF_DEMO_PKG_MANAGER:-}" )
fi

printf '\n✔ demo complete — scratch world %s (re-run me any time; each run is fresh)\n' "${GFF_DEMO_HOME}"
