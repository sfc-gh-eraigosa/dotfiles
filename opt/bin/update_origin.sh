#!/usr/bin/env bash

set -e

current_remote=$(git remote -v|grep origin | grep push| awk '{print $2}')
if [ -z "$1" ]; then
   echo "Usage: $(basename $0) https://github.com/owner/name"
   exit 1
fi
if [ ! "${current_remote}" = "$1" ]; then
    echo "setting origin to $1"
    git remote remove origin
    git remote add origin $1
else
    echo "remote is already set to $1"
fi