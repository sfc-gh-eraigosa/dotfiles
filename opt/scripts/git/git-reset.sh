#!/bin/bash
branch_name="$(printf '%s' $(git branch -l 2>&1|grep '^*'|awk -F' ' '{print $2}'))"
if [ -z "${branch_name}" ]; then
  echo "ERROR no default branch found"
  exit 1
fi

if [ ! "$1" = "--force" ]; then
  echo "Current directory $(pwd) and repo $(git rev-parse --show-toplevel)"
  echo " The current branch will be reset and pulled from origin/${branch_name}"
  echo "  "
  echo -n "Would you like to reset the current branch (${branch_name})? [Y]:" && \
    read -r doreset
else
  doreset="Y"
fi


if [ "${doreset}" = "Y" ]; then
  git reset origin/$branch_name; \
  git add -A && git stash; \
  git pull origin $branch_name
else
  echo "We would have executed the commands"
  echo ""
  echo "git reset origin/${branch_name}"
  echo "git add -A && git stash"
  echo "git pull origin ${branch_name}"
  echo ""
  echo "However,Y was not selected, so git-reset was skipped"
fi

echo "git-reset done"
exit 0
