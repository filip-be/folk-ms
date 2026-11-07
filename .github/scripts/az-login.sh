#!/usr/bin/env bash
# Signs the Azure CLI in with the workflow's GitHub OIDC token (federated
# credential), so no Azure secret is stored. Needs `permissions: id-token: write`
# and AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID.
set -euo pipefail
token="$(curl -fsS -H "Authorization: bearer $ACTIONS_ID_TOKEN_REQUEST_TOKEN" \
  "$ACTIONS_ID_TOKEN_REQUEST_URL&audience=api://AzureADTokenExchange" | jq -r .value)"
echo "::add-mask::$token"
az login --service-principal -u "$AZURE_CLIENT_ID" -t "$AZURE_TENANT_ID" --federated-token "$token" --output none
az account set --subscription "$AZURE_SUBSCRIPTION_ID"
