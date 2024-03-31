#!/bin/zsh
# Path to your oh-my-zsh installation.


# Pull the latest repos
if [ -f "${HOME}/.gitrepos" ] ; then
  cd "${HOME}"
  [ -d "${HOME}/.git" ] && \
    git pull origin $(git branch | grep '*'|awk '{print $2}')
  "${HOME}/.gitrepos"
fi

#. ~/.profile
if [ -f ~/.bash_aliases ]; then
    . ~/.bash_aliases
fi


# echo 'POWERLEVEL9K_DISABLE_CONFIGURATION_WIZARD=true'
# To customize prompt, run `p10k configure` or edit ~/.p10k.zsh.
[[ ! -f ~/.p10k.zsh ]] || source ~/.p10k.zsh

DEFAULT_USER=docker
export Z_HOME=$HOME
if [ ! -d $Z_HOME ] ; then
    export Z_HOME=/home/eraigosa
fi
if [ ! -d $Z_HOME ] ; then
    export Z_HOME=/home/wenlock
fi
if [ ! -d $Z_HOME ] ; then
    export Z_HOME=/Users/eraigosa
fi
if [ ! -d $Z_HOME ] ; then
    export Z_HOME=/home/codespace
fi

if [ -d "${Z_HOME}/.oh-my-zsh" ] ; then
  export ZSH="${Z_HOME}/.oh-my-zsh"
else
  export ZSH="${Z_HOME}/git/oh-my-zsh"
fi

if [ -f ~/opt/themes/agnoster.zsh-theme ] ; then
cp ~/opt/themes/agnoster.zsh-theme $ZSH/themes/agnoster.zsh-theme
fi

# Set name of the theme to load.
# Look in ~/.oh-my-zsh/themes/
# Optionally, if you set this to "random", it'll load a random theme each
# time that oh-my-zsh is loaded.
ZSH_THEME="agnoster"

# Uncomment the following line to use case-sensitive completion.
# CASE_SENSITIVE="true"

# Uncomment the following line to use hyphen-insensitive completion. Case
# sensitive completion must be off. _ and - will be interchangeable.
# HYPHEN_INSENSITIVE="true"

# Uncomment the following line to disable bi-weekly auto-update checks.
# DISABLE_AUTO_UPDATE="true"

# Uncomment the following line to change how often to auto-update (in days).
# export UPDATE_ZSH_DAYS=13

# Uncomment the following line to disable colors in ls.
# DISABLE_LS_COLORS="true"

# Uncomment the following line to disable auto-setting terminal title.
# DISABLE_AUTO_TITLE="true"

# Uncomment the following line to enable command auto-correction.
# ENABLE_CORRECTION="true"

# Uncomment the following line to display red dots whilst waiting for completion.
# COMPLETION_WAITING_DOTS="true"

# Uncomment the following line if you want to disable marking untracked files
# under VCS as dirty. This makes repository status check for large repositories
# much, much faster.
# DISABLE_UNTRACKED_FILES_DIRTY="true"

# Uncomment the following line if you want to change the command execution time
# stamp shown in the history command output.
# The optional three formats: "mm/dd/yyyy"|"dd.mm.yyyy"|"yyyy-mm-dd"
# HIST_STAMPS="mm/dd/yyyy"

# Would you like to use another custom folder than $ZSH/custom?
# ZSH_CUSTOM=/path/to/new-custom-folder

# Which plugins would you like to load? (plugins can be found in ~/.oh-my-zsh/plugins/*)
# Custom plugins may be added to ~/.oh-my-zsh/custom/plugins/
# Example format: plugins=(rails git textmate ruby lighthouse)
# Add wisely, as too many plugins slow down shell startup.
# plugins=(git)
plugins=(git docker golang zsh-completions kubectl)


if [ ! -d "${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions" ] ; then
  git clone https://github.com/zsh-users/zsh-completions ${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions
fi

# User configuration

# export PATH="$PATH:$HOME/opt/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$HOME/go/bin"
export PATH="$PATH:$HOME/bin:$HOME/opt/bin"
# export MANPATH="/usr/local/man:$MANPATH"
fpath+=${ZSH_CUSTOM:-${ZSH:-~/.oh-my-zsh}/custom}/plugins/zsh-completions/src

source $ZSH/oh-my-zsh.sh

# You may need to manually set your language environment
# export LANG=en_US.UTF-8

# Preferred editor for local and remote sessions
# if [[ -n $SSH_CONNECTION ]]; then
#   export EDITOR='vim'
# else
#   export EDITOR='mvim'
# fi

# Compilation flags
# export ARCHFLAGS="-arch x86_64"

# ssh
# export SSH_KEY_PATH="~/.ssh/dsa_id"

# Set personal aliases, overriding those provided by oh-my-zsh libs,
# plugins, and themes. Aliases can be placed here, though oh-my-zsh
# users are encouraged to define aliases within the ZSH_CUSTOM folder.
# For a full list of active aliases, run `alias`.
#
# Example aliases
# alias zshconfig="mate ~/.zshrc"
# alias ohmyzsh="mate ~/.oh-my-zsh"
if [ -f ~/.zsh/zsh-autosuggestions/zsh-autosuggestions.zsh ] ; then
  .  ~/.zsh/zsh-autosuggestions/zsh-autosuggestions.zsh
  export ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE=fg=5
fi

if [ -f ~/.zsh/dircolors-solarized/dircolors.ansi-universal ] ; then
  eval $(dircolors ~/.zsh/dircolors-solarized/dircolors.ansi-dark)
fi


export PATH="$PATH:$HOME/.rvm/bin" # Add RVM to PATH for scripting

# setup any missing brew packages from the $HOME/Brewfile
if [ "$(uname -s)" = "Darwin" ]; then
    _cwd="$(pwd)"
    brew bundle check || brew bundle &
fi

if command -v nodenv &> /dev/null; then
  eval "$(nodenv init -)"
fi
source  $HOME/git/powerlevel10k/powerlevel10k.zsh-theme

#THIS MUST BE AT THE END OF THE FILE FOR SDKMAN TO WORK!!!
if [ -d "${HOME}/.sdkman" ] ; then
  export SDKMAN_DIR="${HOME}/.sdkman"
  [[ -s "${HOME}/.sdkman/bin/sdkman-init.sh" ]] && source "${HOME}/.sdkman/bin/sdkman-init.sh"
fi
set -o vi
unset GIT_URL
test -f ${HOME}/.rbenv/shims/gh && rm -f ${HOME}/.rbenv/shims/gh

#
# node in path
PATH=$PATH:$HOME/.nodenv/shims

#
# GITHUB_TOKEN setup from stored credential
if [[ ! -f .no_github_token ]] ; then
  export GITHUB_TOKEN=${GITHUB_TOKEN:-$( printf "protocol=https\\nhost=github.com\\npath=github\\n" | git credential fill | awk -F'=' '/password=/{print $2}')}
fi

test -n "$(alias ruby)" && unalias ruby

#
# we're in codespaces, setup some custom things
# change to workspace if defined
if [ -d "${CODESPACE_VSCODE_FOLDER}" ]; then
  cd "${CODESPACE_VSCODE_FOLDER}"
  git config pull.rebase true
  if [ -n "${MY_GITHUB_TOKEN}" ] ; then
    export GITHUB_TOKEN="${MY_GITHUB_TOKEN}"
  fi
fi

# source any secrets
if [ -f ~/.secrets.env ] ; then
  source ~/.secrets.env
fi

# autoload -Uz compinit
# compinit
