### REPOSITORY CONVENTIONS (shared)

- **Portability:** resolve paths from `${HOME}`/`~` or a script's own location — never a
  hardcoded `${HOME}/git/dotfiles`. The repo may be cloned anywhere.
- **Git workflow:** the `gss` (git-safe-sync) skill is the canonical commit + push path;
  confirmation is mandatory before any `git add`/`commit`/`push`/`pr`. Stage files by
  explicit name — never `git add -A`/`git add .`.
- **.gitignore allowlist:** the root `.gitignore` starts with `*` (everything ignored);
  paths are tracked only via explicit `!`-rules. A new file outside an opted-in tree is
  invisible to git until you add a rule — verify with `git status` / `git check-ignore`.
- **Minimal alias surface:** prefer one canonical alias per workflow; don't proliferate
  variants.
- **Docs discoverability:** any new tool/doc directory gets a `GEMINI.md` plus a
  `CLAUDE.md -> GEMINI.md` symlink so both assistants can navigate it.
