# Path to your oh-my-zsh installation.
export ZSH=/home/wenlock/git/oh-my-zsh

# Set name of the theme to load.
# Look in ~/.oh-my-zsh/themes/
# Optionally, if you set this to "random", it'll load a random theme each
# time that oh-my-zsh is loaded.
ZSH_THEME="agnoster"
export PROMPT_ON_NEWLINE=true

# Uncomment the following line to use case-sensitive completion.
# CASE_SENSITIVE="true"

# Uncomment the following line to disable bi-weekly auto-update checks.
# DISABLE_AUTO_UPDATE="true"

# Uncomment the following line to change how often to auto-update (in days).
# export UPDATE_ZSH_DAYS=13

# Uncomment the following line to disable colors in ls.
# DISABLE_LS_COLORS="true"

# Uncomment the following line to disable auto-setting terminal title.
# DISABLE_AUTO_TITLE="true"

# Uncomment the following line to enable command auto-correction.
ENABLE_CORRECTION="true"

# Uncomment the following line to display red dots whilst waiting for completion.
COMPLETION_WAITING_DOTS="true"

# Uncomment the following line if you want to disable marking untracked files
# under VCS as dirty. This makes repository status check for large repositories
# much, much faster.
# DISABLE_UNTRACKED_FILES_DIRTY="true"

# Uncomment the following line if you want to change the command execution time
# stamp shown in the history command output.
# The optional three formats: "mm/dd/yyyy"|"dd.mm.yyyy"|"yyyy-mm-dd"
# HIST_STAMPS="mm/dd/yyyy"

# Would you like to use another custom folder than /home/wenlock/.oh-my-zsh/custom?
# ZSH_CUSTOM=/path/to/new-custom-folder

# Which plugins would you like to load? (plugins can be found in ~/.oh-my-zsh/plugins/*)
# Custom plugins may be added to ~/.oh-my-zsh/custom/plugins/
# Example format: plugins=(rails git textmate ruby lighthouse)
# Add wisely, as too many plugins slow down shell startup.
plugins=(git)

source /home/wenlock/.oh-my-zsh/oh-my-zsh.sh

# User configuration

# export PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games"
if [ -d /home/wenlock/.cabal/bin ] ; then
  export PATH="/home/wenlock/.cabal/bin:/home/wenlock/.local/bin:/home/wenlock/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/home/wenlock/go/bin:/usr/local/go/bin:/home/wenlock/go/bin:/usr/local/go/bin"
fi
# export MANPATH="/usr/local/man:"

# You may need to manually set your language environment
# export LANG=en_US.UTF-8

# Preferred editor for local and remote sessions
# if [[ -n 98.244.29.51 41902 10.0.0.4 22 ]]; then
#   export EDITOR='vim'
# else
#   export EDITOR='mvim'
# fi

# Compilation flags
# export ARCHFLAGS="-arch x86_64"

# ssh
# export SSH_KEY_PATH="~/.ssh/dsa_id"

if [ ! -f "/home/wenlock/.default_user" ] ; then
  printf wenlock > "/home/wenlock/.default_user"
fi
if [ "wenlock" = "" ] ; then
  DEFAULT_USER=
fi

# Set personal aliases, overriding those provided by oh-my-zsh libs,
# plugins, and themes. Aliases can be placed here, though oh-my-zsh
# users are encouraged to define aliases within the ZSH_CUSTOM folder.
# For a full list of active aliases, run -='cd -'
...=../..
....=../../..
.....=../../../..
......=../../../../..
1='cd -'
2='cd -2'
3='cd -3'
4='cd -4'
5='cd -5'
6='cd -6'
7='cd -7'
8='cd -8'
9='cd -9'
_=sudo
afind='ack -il'
alert='notify-send --urgency=low -i "$([ $? = 0 ] && echo terminal || echo error)" "$(history|tail -n1|sed -e '\''s/^\s*[0-9]\+\s*//;s/[;&|]\s*alert$//'\'')"'
d='dirs -v | head -10'
delkey='xdotool key Delete'
dockerup='bash ~/opt/bin/docker_up.sh'
egrep='egrep --color=auto'
f11='xdotool key F11'
f12='xdotool key F12'
fgrep='fgrep --color=auto'
forjssh='~/bin/forjssh.sh'
g=git
ga='git add'
gaa='git add --all'
gapa='git add --patch'
gb='git branch'
gba='git branch -a'
gbda='git branch --merged | command grep -vE "^(\*|\s*master\s*$)" | command xargs -n 1 git branch -d'
gbl='git blame -b -w'
gbnm='git branch --no-merged'
gbr='git branch --remote'
gbs='git bisect'
gbsb='git bisect bad'
gbsg='git bisect good'
gbsr='git bisect reset'
gbss='git bisect start'
gc='git commit -v'
'gc!'='git commit -v --amend'
gca='git commit -v -a'
'gca!'='git commit -v -a --amend'
gcam='git commit -a -m'
'gcan!'='git commit -v -a -s --no-edit --amend'
gcb='git checkout -b'
gcf='git config --list'
gcl='git clone --recursive'
gclean='git clean -fd'
gcm='git checkout master'
gcmsg='git commit -m'
gco='git checkout'
gcount='git shortlog -sn'
gcp='git cherry-pick'
gcs='git commit -S'
gd='git diff'
gdca='git diff --cached'
gdct='git describe --tags `git rev-list --tags --max-count=1`'
gdt='git diff-tree --no-commit-id --name-only -r'
gdw='git diff --word-diff'
gerrit-rebase='fgit-gerrit-rebase $@'
gertty='source gertty-env/bin/activate && gertty'
gf='git fetch'
gfa='git fetch --all --prune'
gfo='git fetch origin'
gg='git gui citool'
gga='git gui citool --amend'
ggpull='git pull origin $(git_current_branch)'
ggpur=ggu
ggpush='git push origin $(git_current_branch)'
ggsup='git branch --set-upstream-to=origin/$(git_current_branch)'
gignore='git update-index --assume-unchanged'
gignored='git ls-files -v | grep "^[[:lower:]]"'
git-addalias='fgit-addalias $@'
git-clean='git checkout$(git branch -v|grep "^\*"|awk '\''{printf $2}'\'');git reset --hard origin/$(git branch -v|grep "^\*"|awk '\''{printf $2}'\'');git clean -x -d -f'
git-clone=fgit-clone
git-config='fgit-config $@'
git-help=fgit-help
git-keys='fgit-keys $@'
git-login='fgit-login $@'
git-logout='ssh-add -d /home/wenlock/.ssh/gerrit_keys'
git-projects='fgit-projects $@'
git-proxy='fgit-proxy $@'
git-pull=fgit-pull
git-push='git push origin HEAD:refs/for/$(git branch -v|grep "^\*"|awk '\''{printf $2}'\'')'
git-refresh=fgit-remote-refresh
git-reset=fgit-reset
git-reset-all=fgit-reset-all
git-sshkill='kill -9 $(ps -ef|grep ssh-agent|grep -v grep | awk '\''{printf $2" " }'\'')'
git-ssl='fgit-ssl $@'
git-svn-dcommit-push='git svn dcommit && git push github master:svntrunk'
git-test='fgit-test $@'
git-testt='fgit-testt $@'
git-tunel='fgit-tunel $@'
git-tunelkill=fgit-tunelkill
gitsave='unset GIT_SAVE_OFF;echo "GIT_SAVE_OFF is unset, bash_logout will commit";'
gitsave_off='export GIT_SAVE_OFF=true;echo "GIT_SAVE_OFF is true, bash_logout will not commit";'
gk='\gitk --all --branches'
gke='\gitk --all $(git log -g --pretty=format:%h)'
gl='git pull'
glg='git log --stat --color'
glgg='git log --graph --color'
glgga='git log --graph --decorate --all'
glgm='git log --graph --max-count=10'
glgp='git log --stat --color -p'
glo='git log --oneline --decorate --color'
globurl='noglob urlglobber '
glog='git log --oneline --decorate --color --graph'
glol='git log --graph --pretty=format:'\''%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset'\'' --abbrev-commit'
glola='git log --graph --pretty=format:'\''%Cred%h%Creset -%C(yellow)%d%Creset %s %Cgreen(%cr) %C(bold blue)<%an>%Creset'\'' --abbrev-commit --all'
glp=_git_log_prettily
glum='git pull upstream master'
gm='git merge'
gmom='git merge origin/master'
gmt='git mergetool --no-prompt'
gmtvim='git mergetool --no-prompt --tool=vimdiff'
gmum='git merge upstream/master'
gp='git push'
gpd='git push --dry-run'
gpoat='git push origin --all && git push origin --tags'
gpristine='git reset --hard && git clean -dfx'
gpu='git push upstream'
gpv='git push -v'
gr='git remote'
gra='git remote add'
grb='git rebase'
grba='git rebase --abort'
grbc='git rebase --continue'
grbi='git rebase -i'
grbm='git rebase master'
grbs='git rebase --skip'
grep='grep --color=auto'
grh='git reset HEAD'
grhh='git reset HEAD --hard'
grmv='git remote rename'
grrm='git remote remove'
grset='git remote set-url'
grt='cd $(git rev-parse --show-toplevel || echo ".")'
gru='git reset --'
grup='git remote update'
grv='git remote -v'
gsb='git status -sb'
gsd='git svn dcommit'
gsi='git submodule init'
gsps='git show --pretty=short --show-signature'
gsr='git svn rebase'
gss='git status -s'
gst='git status'
gsta='git stash'
gstaa='git stash apply'
gstd='git stash drop'
gstl='git stash list'
gstp='git stash pop'
gsts='git stash show --text'
gsu='git submodule update'
gts='git tag -s'
gtv='git tag | sort -V'
gunignore='git update-index --no-assume-unchanged'
gunwip='git log -n 1 | grep -q -c "\-\-wip\-\-" && git reset HEAD~1'
gup='git pull --rebase'
gupv='git pull --rebase -v'
gvt='git verify-tag'
gwch='git whatchanged -p --abbrev-commit --pretty=medium'
gwip='git add -A; git rm $(git ls-files --deleted) 2> /dev/null; git commit -m "--wip--"'
history='fc -l 1'
irc=irssi
l='ls -CF'
la='ls -A'
ll='ls -alF'
ls='ls --color=auto'
lsa='ls -lah'
md='mkdir -p'
novassh='function nova_ssh { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ3.pem -l ubuntu $1;};nova_ssh'
novassh1='function nova_ssh1 { ssh-keygen -f ~/.ssh/known_hosts -R $1;ssh -i ~/.ssh/nova-USWest-AZ1.pem -l ubuntu $1;};nova_ssh1'
please=sudo
po=popd
pu=pushd
pulse=/usr/local/bin/nclauncher.pl
rd=rmdir
refresh-gitenv=refresh-gitenv
sshhost='cat ~/.ssh/config|grep "Host\s"|sed "s/Host /ssh /g"'
tb=/home/wenlock/git/forj-oss/maestro/tools/bin/test-box.sh
tunnelp='sudo openvpn --mktun --dev tun0'
vi='vim -u ~/.vimrc_active'
vim='vim -u ~/.vimrc_active'
vimg='vim -u ~/.vimrc_green'
vimw='vim -u ~/.vimrc_white'
which-command=whence
win='wine explorer -opengl'
win_nogl='LIBGL_ALWAYS_SOFTWARE=1 wine explorer'
wow='WINEDEBUG=-all wine "/home/wenlock/.wine/drive_c/Program Files (x86)/World of Warcraft/Wow.exe"'.
#
# Example aliases
# alias zshconfig="mate ~/.zshrc"
# alias ohmyzsh="mate ~/.oh-my-zsh"
if [ -f ~/.bash_aliases ]; then
  . ~/.bash_aliases
fi

if [ "0" = "1" ] && [ -f /home/wenlock/.local/bin/powerline ] ; then
  export POWERLINE_COMMAND=~/.local/bin/powerline
fi

# use this function to give a vim status line at the prompt
function vimprompt
{
  _VIMPROMPT=
  if [ -z "" ] ; then
    _VIMPROMPT=on
  fi
  if [ "" = "on" ] ; then
    if [[ -r /home/wenlock/.local/lib/python2.7/site-packages/powerline/bindings/zsh/powerline.zsh ]]; then
      source /home/wenlock/.local/lib/python2.7/site-packages/powerline/bindings/zsh/powerline.zsh
      echo "vim prompt is on"
    else
      echo "missing vim prompt, install with: pip install --user git+git://github.com/Lokaltog/powerline"
    fi
  elif [ "" = "off" ] ; then
    source /home/wenlock/.oh-my-zsh/oh-my-zsh.sh
    echo "vim prompt is off"
  else
    echo "vimprompt supports arguments on|off only."
  fi
}
