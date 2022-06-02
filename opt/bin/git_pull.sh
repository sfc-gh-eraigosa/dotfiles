#!/usr/bin/env bash

set -e

BRANCH="${BRANCH:-main}"

if [ -n "$1" ]; then
  BRANCH=$1
fi

git pull origin $BRANCH