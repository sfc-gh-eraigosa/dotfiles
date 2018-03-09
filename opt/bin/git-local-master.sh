#!/bin/bash

# lets update the current branch onto the local master for the purposes of hacking
# with the master branch to simulate the branch merged on master.
# We'll stash whatever sits on master that doesn't match the origin.


# lets make sure we're doing this on a branch other than master
branch_name="$(printf '%s' $(git branch -l 2>&1|grep '^*'|awk -F' ' '{print $2}'))"
if [ -z "${branch_name}" ] || [ "${branch_name}" = 'master' ] ; then
  echo "ERROR no branch found or the current branch is master, you need to use a branch for this command"
  exit 1
fi

if [ ! "$1" = "--force" ]; then
  echo "Current directory $(pwd) and repo $(git rev-parse --show-toplevel)"
  echo " The current branch ${branch_name} will update master"
  echo "  "
  echo -n "Are you ready to continue, use --force to skip this message, (${branch_name})? [Y]:" && \
    read -r dorit
else
  doit="Y"
fi


if [ "${doreset}" = "Y" ]; then
  git checkout master && \
    git reset origin master && \
    git add -A && \
    git stash && \
    git merge "${branch_name}" && \
    git checkout "${branch_name}"
else
  echo "We would have executed the commands"
  echo "git checkout master && \
    git reset origin master && \
    git add -A && \
    git stash && \
    git merge \"${branch_name}\" && \
    git checkout \"${branch_name}\""
  echo ""
  echo "However,Y was not selected, so git-git-local-master was skipped"
fi

echo "git-local-master done"
exit 0
