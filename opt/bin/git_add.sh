#!/usr/bin/env bash

set -e 

git status | grep Untracked && \
git add -A