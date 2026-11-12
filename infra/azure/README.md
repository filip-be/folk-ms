# infra/azure

`main.bicep` declares everything folk-ms runs on; see [Platform](../../docs/platform/README.md) for why.

Deployed by `.github/workflows/deploy-infra.yml` into one resource group. Parameters come from the workflow:

| Parameter | Source |
| --- | --- |
| `siteIds`, `siteOrigins` | `config/sites.json` (computed in the workflow) |
| `superAdminEmail`, `googleClientSecret`, `metaAppSecret`, `sessionKey` | GitHub environment secrets |
| `googleClientId`, `metaAppId`, `cmsBaseUrl` | GitHub environment variables |

CORS on the blob service allows `siteOrigins` and, in a second rule, the default `https://<name>.azurestaticapps.net` origin of every Static Web App, so a Site works before it has a custom domain.

`cmsBaseUrl` may be empty: the CMS then uses `https://<functionApp>.azurewebsites.net`. Outputs (`functionApp`, `cmsUrl`, `blobEndpoint`, `storageAccount`, `staticSites`) feed the GitHub variables in the [first deployment](../../README.md#first-deployment).

Check locally (needs the Azure CLI): `az bicep build --file infra/azure/main.bicep --stdout > /dev/null`.

Generate `SESSION_KEY` once: `openssl rand -base64 32`. Changing it signs everyone out.
