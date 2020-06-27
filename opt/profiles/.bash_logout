# ~/.bash_logout: executed by bash(1) when login shell exits.

# when leaving the console clear the screen to increase privacy

if [ "$SHLVL" = 1 ]; then
    [ -x /usr/bin/clear_console ] && /usr/bin/clear_console -q
fi
if [ ! "$GIT_SAVE_OFF" = "true" ]; then
  cd ~
  git checkout master
  git commit -a -m "bash_logout commit $(date)"
  git push origin master
fi
