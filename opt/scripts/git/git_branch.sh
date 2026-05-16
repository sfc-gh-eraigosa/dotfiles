#!/usr/bin/env bash

set -e 
# create a branch in the current repo

BRANCH="$(whoami)/$(cat /dev/urandom | env LC_CTYPE=C tr -cd 'a-f0-9' | head -c 6)"

if [ -n "$1" ]; then
  BRANCH=$1
fi

git checkout -b $BRANCH