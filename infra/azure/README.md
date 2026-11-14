# infra/azure

`main.bicep` declares everything folk-ms runs on; see [Platform](../../docs/platform/README.md) for why.

Deployed by `.github/workflows/deploy-infra.yml` into one resource group. Parameters come from the workflow:

| Parameter | Source |
| --- | --- |
| `siteIds`, `siteOrigins` | `config/sites.json` (computed in the workflow) |
| `superAdminEmail`, `googleClientSecret`, `metaAppSecret`, `sessionKey` | GitHub environment secrets |
| `googleClientId`, `metaAppId`, `cmsBaseUrl` | GitHub environment variables |
| `cmsIdentityName` | Default `<prefix>-cms-id` (`folkms-cms-id`) |
| `monthlyBudget`, `budgetStart` | Defaults 10 (billing currency) and `2026-11-01` |

CORS on the blob service allows `siteOrigins` and, in a second rule, the default origin of every Static Web App (the generated `https://<host>.azurestaticapps.net`, see output `staticSites`), so a Site works before it has a custom domain.

`cmsBaseUrl` may be empty: the CMS then uses `https://<functionApp>.azurewebsites.net`. Outputs (`functionApp`, `cmsUrl`, `blobEndpoint`, `storageAccount`, `keyVault`, `keyVaultUrl`, `tokenVault`, `tokenVaultUrl`, `cmsIdentityClientId`, `staticSites`) feed the GitHub variables in the [first deployment](../../README.md#first-deployment).

Check locally (needs the Azure CLI): `az bicep build --file infra/azure/main.bicep --stdout > /dev/null`.

Generate `SESSION_KEY` once: `openssl rand -base64 32`. Changing it signs everyone out.

## CMS identity and Key Vault

See [ADR 0010](../../docs/architecture/adr/0010-autosync-keeps-a-page-token-in-key-vault.md).

- The CMS runs as the user-assigned identity `cmsIdentityName`. `main.bicep` does not create it: the first-deployment runbook does, because the deploy identity's role-assignment condition names it. The identity is used for the Functions host storage, the deployment package, Key Vault references and, through `AZURE_CLIENT_ID`, by the CMS's own blob and Key Vault calls.
- The deploy identity has `Contributor` and a `Role Based Access Control Administrator` assignment that lets it assign only `Storage Blob Data Owner`, `Key Vault Secrets User` and `Key Vault Secrets Officer`, and only to that identity. A new role in `main.bicep` therefore needs the condition changed first.
- Two vaults, both with Azure RBAC, soft delete for 7 days and no purge protection (the CMS never reuses a secret name, and purges deleted tokens). The platform vault (`keyVault`) holds `META_APP_SECRET` and `SESSION_KEY` as the secrets `meta-app-secret` and `session-key`; the app settings hold `@Microsoft.KeyVault(...)` references to them, and the CMS identity has only `Key Vault Secrets User` there. The token vault (`tokenVault`) holds one secret per Organization with Autosync on; the CMS finds it in `KEYVAULT_URL` and has `Key Vault Secrets Officer` there.
- Both vaults' `AuditEvent` logs and the blob service's `StorageWrite` and `StorageDelete` logs go to a Log Analytics workspace (`PerGB2018`, 30 days, 1 GB daily cap). A scheduled-query alert, evaluated every 15 minutes over the last hour, emails `superAdminEmail` when either vault is used by an identity other than the CMS's, the `cms` container is written by one, or page tokens are read more than 100 times in an hour. An Activity Log alert emails on role-assignment, diagnostic-setting, alert-rule and vault changes, and a budget emails at 50% and 100% of `monthlyBudget`.
- A vault name stays reserved while the vault is in soft delete. After deleting the resource group, purge both vaults (`az keyvault purge -n <keyVault>`, `az keyvault purge -n <tokenVault>`) before deploying into a resource group with the same id again.

Moving from the system-assigned identity of earlier versions: after the first deployment of this version, an `Owner` deletes the storage account's role assignment shown as *Identity not found*; the deploy identity cannot.

## Cost

Key Vault (a few thousand operations a month), the Log Analytics workspace (a few MB a month, inside the free allowance) and the alert rule (about $0.50 a month at a 15-minute frequency) bring the platform to roughly $2 a month.
