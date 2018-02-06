#!/bin/bash

# clean up the current users branches
set -e
export MY_USER="${MY_USER:-$(whoami)}"

# make sure before moving forward
if [ ! "$1" = "--force" ]; then
  echo "Current directory $(pwd) and repo $(git rev-parse --show-toplevel)"
  echo -n "Would you like to cleanup ($(printf '%s' $(git branch -l 2>&1 |grep "^\s\s${MY_USER}\/.*"|wc -l))) all branches? [Y]:" && \
    read -r cleanup
else
  cleanup="Y"
fi
# this depends on branches starting with the logged in users user name
for b in $(git branch -l 2>&1 |grep "^\s\s${MY_USER}\/.*"); do
  echo "Cleaning up branch $b";
  if [ "${cleanup}" = "Y" ]; then
    git branch -D $b
  else
    echo "Y was not selected, branch cleanup skipped"
  fi
done

echo "git-rm-mybranches done"
exit 0
