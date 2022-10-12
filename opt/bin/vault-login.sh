#!/usr/bin/env bash

set -e
echo "# Login to vault"

echo -n "# Please profile a DUO token number:"
read DUO_PASS
echo ""

if [[ -z "${OKTA_USER}" ]]; then
    echo -n "# Please provide a OKTA USERNAME :"
    read OKTA_USER
    echo ""
fi

echo "# Please type your OKTA password next."
echo -n "# "
_token=$(vault login -token-only -method=okta username="${OKTA_USER}" passcode="${DUO_PASS}")

VAULT_TOKEN=${_token} vault token lookup > /dev/null || (
    echo "failed to login"
    return 1
)
echo "# Run the command to setup vault: "
echo "export VAULT_TOKEN=${_token}"