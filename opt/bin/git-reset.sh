#!/bin/bash

upstream_remote=$(git remote -v|grep -v wenlock|grep '(push)'|awk '{print $1}')

if [ -z "${upstream_remote}" ]; then
  echo "ERROR no remote for upstream: $upstream_remote"
  exit 1
fi


ssh_remote=$(git remote -v | grep 'ssh://'|grep '(push)'|awk '{print $1}')
if [ -z "${ssh_remote}" ]; then
  echo "ERROR no remote for ssh: $ssh_remote"
  exit 1
fi

_BRANCH=master
if [ ! -z "$1" ]; then
  _BRANCH=$1
fi

echo "Reseting $upstream_remote to $_BRANCH ..." && \
git reset --hard ${upstream_remote}/$_BRANCH && \
echo "Pulling $upstream_remote $_BRANCH ..." && \
git pull ${upstream_remote} $_BRANCH && \
echo "Reseting $upstream_remote to $_BRANCH ..." && \
git reset --hard ${upstream_remote}/$_BRANCH && \
echo "Pushing updates $ssh_remote to $_BRANCH ..." && \
git push ${ssh_remote} $_BRANCH -f && \
git pull origin $_BRANCH && \
echo "Reseting $upstream_remote to $_BRANCH ..." && \
git reset --hard ${upstream_remote}/$_BRANCH
