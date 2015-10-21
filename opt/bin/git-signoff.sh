#!/bin/bash
# http://stackoverflow.com/questions/13043357/git-sign-off-previous-commits
# Example:
# git-signoff.sh d385ecac44e44dcd71e863ced7ffbfac34643a44 --glob="refs/heads/oneview_plugin*"

test $# -gt 0 || {
echo "Usage: $0 [commit-range]" >&2
exit 1
}
AUTHOR="$(git var -l | sed -n 's/GIT_AUTHOR_IDENT=\(.*>\).*/\1/p')"
SOB="Signed-off-by: $AUTHOR"
git filter-branch --msg-filter \
    'msg="$(cat)" &&
    echo "$msg" &&
    case "$msg" in
        *"'"$SOB"'"*)
            ;;
        *)
            printf "\n%s\n" "'"$SOB"'"
            ;;
    esac' \
    "$@"

