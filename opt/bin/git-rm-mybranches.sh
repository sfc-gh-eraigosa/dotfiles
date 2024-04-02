#!/bin/bash

# set -x -v

# clean up the current users branches
set -e
export SEARCH_TERM="${SEARCH_TERM:-}"


# make sure before moving forward
if [ "$1" = "--force" ]; then
  cleanup="Y"
fi

if [ "${cleanup}" = "Y" ]; then
    SEARCH_TERM="${2}"
fi

if [ -n "${1}" ]; then
    SEARCH_TERM="${1}"
fi

if [ -z "${SEARCH_TERM}" ]; then
    echo "Usage: $o [--force] regex"
    exit 1
fi

if [ ! "${cleanup}" = "Y" ]; then
  echo "Current directory $(pwd) and repo $(git rev-parse --show-toplevel)"
  echo -n "Would you like to cleanup ($(printf '%s' $(git branch -l 2>&1 |grep "^\s\s${SEARCH_TERM}"|wc -l))) all branches? [Y]:" && \
    read -r cleanup
fi

# this depends on branches starting with the logged in users user name
for b in $(git branch -l 2>&1 |grep "^\s\s${SEARCH_TERM}"); do
  echo "Cleaning up branch $b";
  if [ "${cleanup}" = "Y" ]; then
    git branch -D $b
  else
    echo "Y was not selected, branch cleanup skipped"
  fi
done

echo "git-rm-mybranches done"
exit 0
