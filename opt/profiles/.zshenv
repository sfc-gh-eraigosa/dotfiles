#!/bin/zsh
# ~/.zshenv — sourced for EVERY zsh invocation, and (crucially) BEFORE the
# system rc /etc/zsh/zshrc. It is the only user-owned startup file early enough
# to influence what that global rc does, so it is the right home for the toggle
# below.

# ---------------------------------------------------------------------------
# Skip the distro's global compinit.
#
# Why: Debian/Ubuntu's /etc/zsh/zshrc runs `compinit` itself (guarded by
# `skip_global_compinit`). Because the system rc is sourced before ~/.zshrc,
# that global compinit scans $fpath and, on WSL, chokes on a DANGLING symlink
# that Docker Desktop's integration leaves in a root-owned completions dir:
#
#     /usr/share/zsh/vendor-completions/_docker
#         -> /mnt/wsl/docker-desktop/cli-tools/.../_docker   (gone when the
#            docker-desktop mount is absent, e.g. Docker Desktop not running)
#
# compinit reads the first line of every `_*` file to find its #compdef tag and
# errors on the broken link:
#     compinit:<n>: no such file or directory: .../vendor-completions/_docker
#
# Our ~/.zshrc runs its own compinit later — after shadowing dangling
# completion symlinks (see the "compshadow" block there) — so the global
# compinit is redundant. Skipping it removes the error AND avoids a wasteful
# double compinit. /etc/zsh/zshrc documents this exact escape hatch.
#
# Setting the variable is harmless on systems whose global rc doesn't check it
# (it is simply unused there), so we set it unconditionally for portability.
# ---------------------------------------------------------------------------
skip_global_compinit=1
