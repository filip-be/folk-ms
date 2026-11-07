# infra/azure

`main.bicep` declares everything folk-ms runs on; see [Platform](../../docs/platform/README.md) for why.

Deployed by `.github/workflows/deploy-infra.yml` into one resource group. Parameters come from the workflow:

| Parameter | Source |
| --- | --- |
| `siteIds`, `siteOrigins` | `config/sites.json` (computed in the workflow) |
| `superAdminEmail`, `googleClientSecret`, `metaAppSecret`, `tokenEncKey`, `sessionKey` | GitHub environment secrets |
| `googleClientId`, `metaAppId`, `cmsBaseUrl` | GitHub environment variables |

Check locally (needs the Azure CLI): `az bicep build --file infra/azure/main.bicep --stdout > /dev/null`.

Generate keys once: `openssl rand -base64 32` for `TOKEN_ENC_KEY` and `SESSION_KEY`. Changing `TOKEN_ENC_KEY` makes stored Meta tokens unreadable: every Organization must reconnect.
