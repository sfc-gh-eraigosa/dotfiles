# Machine-Local Overrides (`~/.zshrc.local`)

Some machines need shell behavior that differs from the shared dotfiles — a
different command launcher, host-specific `PATH` entries, work credentials, or
aliases that only make sense on one box. The `.zshrc.local` pattern lets you
do this without touching `~/git/dotfiles`.

## How It Works

`opt/profiles/.zshrc` sources `~/.zshrc.local` at the very end of startup:

```zsh
[ -f "$HOME/.zshrc.local" ] && source "$HOME/.zshrc.local"
```

Because it loads **after** everything else in `.zshrc`, any function, alias, or
variable defined in `.zshrc.local` wins over the shared defaults.

The file is **not created by `install.sh`** and is **not tracked in the repo**,
so it exists only on the machines where you create it.

## Creating the File

```bash
touch ~/.zshrc.local
```

Then edit it with your host-specific overrides. Reload your shell or run:

```bash
source ~/.zshrc.local
```

## Common Use Cases

### Override a shell function for a different launcher

Some environments wrap a CLI inside a platform-specific command (e.g.
`sf ai claude` on Snowflake-managed machines). Override the dotfiles wrapper
locally without changing the repo:

```zsh
# ~/.zshrc.local
claude() {
    if _claude_yolo_enabled; then
        sf ai claude -- --dangerously-skip-permissions "$@"
    else
        sf ai claude -- "$@"
    fi
}
```

### Add a host-specific PATH entry

```zsh
# ~/.zshrc.local
export PATH="/opt/custom-toolchain/bin:$PATH"
```

### Source work credentials

```zsh
# ~/.zshrc.local
[ -f ~/.work-secrets.env ] && source ~/.work-secrets.env
```

### Set a machine-specific editor

```zsh
# ~/.zshrc.local
export EDITOR="code --wait"
```

## Rules of Thumb

| Do | Don't |
|:---|:------|
| Put host-specific overrides here | Commit this file to the repo |
| Source sensitive credentials | Put secrets in dotfiles |
| Override shared functions | Duplicate large blocks of shared config |
| Keep it short and focused | Use it as a second `.zshrc` |

## Relationship to Shared Config

`~/.zshrc.local` is loaded **after** all dotfiles config, so load order is:

```
~/.zshrc  (symlink → dotfiles/opt/profiles/.zshrc)
  └─ sources opt/profiles/.zshrc content
       └─ sources ~/.config/claude/aliases.sh  (claude, claude-toggle)
       └─ ... other tools ...
       └─ sources ~/.zshrc.local  ← your overrides land here, last
```

Functions defined here shadow any earlier definition of the same name for the
duration of the shell session.
