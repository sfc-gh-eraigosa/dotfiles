#!/bin/bash
# yq and other related tools
# from https://github.com/wildducktheories/y2j
# https://github.com/wildducktheories/y2j/blob/master/LICENSE
function y2j() {
    if test "$1" = "-d"; then
        shift 1
        j2y "$@"
    else
        script=$(echo -e "import sys, yaml, json\n\nfor doc in yaml.load_all(sys.stdin):\n\tjson.dump(doc, sys.stdout, indent=4)\n\tprint(\"\")\n")
        python -c "$script" | (
            if test $# -gt 0; then
                jq "$@"
            else
                cat
            fi
        )
    fi
}
export -f y2j
pip install pyyaml
