# folk-ms

CMS for bal folk websites: it syncs each Organization's Facebook Events into bilingual Events, Moderators add English text and more, each Organization's static Site shows them, and Moderators announce them on Facebook and Instagram.

Documentation: [docs/README.md](docs/README.md). Glossary: [CONTEXT.md](CONTEXT.md).

The admin UI's [design system](docs/design-system/README.md) was originally defined by [Felipe Cardona](https://dribbble.com/felipecardona/).

## Repository

| Path | What |
| --- | --- |
| [`cms/`](cms/) | The CMS: one Go binary with the embedded admin SPA ([`cms/admin/`](cms/admin/)) |
| [`contract/`](contract/) | Snapshot JSON Schema, fixtures and the TypeScript reader Sites use |
| [`sites/`](sites/) | One static Site skeleton per Organization |
| [`config/sites.json`](config/sites.json) | The fixed list of Organizations and Sites |
| [`infra/azure/`](infra/azure/) | Bicep |
| [`docs/`](docs/) | Capability articles and [ADRs](docs/architecture/adr/) |

## Local development

Prerequisites, on `PATH` and kept up to date: Go (at least the `go` version in [`cms/go.mod`](cms/go.mod)), Bun, Git Bash (Windows) or any POSIX shell, Node.js/npm (only for the type-checker), Docker (only for Azurite tests). CI uses the latest Go and Bun; see [ADR 0007](docs/architecture/adr/0007-latest-toolchains-and-dependabot-cooldown.md).

On Windows:

```bash
winget install GoLang.Go Oven-sh.Bun
```

On Ubuntu (including WSL): Ubuntu's `golang` package is usually older than `cms/go.mod` requires, so install Go from go.dev and Bun with its installer. Rerun the Go command to update Go. Run `bun upgrade` to update Bun.

```bash
sudo apt install -y curl unzip
```

```bash
v=$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -1) && curl -fsSL "https://go.dev/dl/$v.linux-amd64.tar.gz" -o /tmp/go.tgz && sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tgz && rm /tmp/go.tgz
```

```bash
curl -fsSL https://bun.sh/install | bash
```

```bash
echo 'export PATH="/usr/local/go/bin:$HOME/.bun/bin:$PATH"' >> ~/.bashrc && exec bash
```

Then `go version` and `bun --version` should both work. For the type-checker, install Node.js LTS (e.g. via nvm); for Azurite, use Docker Desktop with WSL integration or Docker Engine.

Run the CMS at http://localhost:8080/app/ with filesystem storage in `.data/`, a dev sign-in page that accepts any email (`admin@example.org` is the Super Admin), Social Posts recorded instead of sent, and Sync reading canned Facebook Events from a fake Facebook page:

```bash
scripts/dev.sh
```

Build and serve a Site against the local CMS (http://localhost:5173/pl/):

```bash
bun sites/build.ts balfolk-poznan && bun sites/serve.ts balfolk-poznan 5173
```

Tests:

```bash
(cd cms && go test ./...)
```

```bash
bun test contract sites scripts
```

```bash
npm ci --ignore-scripts && npx tsc -b tsconfig.json
```

Blob adapter tests against Azurite (the digest is the one pinned in `.github/workflows/ci.yml`; the script also starts `dockerd` when no daemon is running, as in a Claude Code cloud session):

```bash
scripts/azurite.sh
```

```bash
(cd cms && AZURITE_BLOB_URL=http://127.0.0.1:10000/devstoreaccount1 go test ./internal/adapters/azblob/)
```

## Operations

### First deployment

The CMS address contains a hash that `main.bicep` derives from the resource group, so it is known only after the first `deploy-infra` run. The Google and Meta apps are therefore created first and get their redirect URIs afterwards.

1. Create a resource group and an Entra app registration (the deploy identity). On the app registration add a federated credential (*Certificates & secrets → Federated credentials → GitHub Actions deploying Azure resources*) with entity type *Environment* and environment `production`; its subject is `repo:filip-be/folk-ms:environment:production`. Grant the app registration `Contributor` on the resource group.
2. Create the CMS's managed identity and let the deploy identity assign roles to it, and to nothing else ([ADR 0010](docs/architecture/adr/0010-autosync-keeps-a-page-token-in-key-vault.md), T4). Someone with `Owner` on the resource group runs, in Azure Cloud Shell (Bash), with the resource group and the app registration's client id filled in:

   ```bash
   rg=<resource group>
   az identity create -g "$rg" -n folkms-cms-id
   cms=$(az identity show -g "$rg" -n folkms-cms-id --query principalId -o tsv)
   deploy=$(az ad sp show --id <AZURE_CLIENT_ID> --query id -o tsv)
   scope=$(az group show -n "$rg" --query id -o tsv)
   roles='b7e6dc6d-f1e8-4753-8033-0f276bb0955b, 4633458b-17de-408a-b874-0445c86b69e6, b86a8fe4-44ce-4948-aee5-eccb2c155cd7'
   condition="((!(ActionMatches{'Microsoft.Authorization/roleAssignments/write'})) OR (@Request[Microsoft.Authorization/roleAssignments:RoleDefinitionId] ForAnyOfAnyValues:GuidEquals {$roles} AND @Request[Microsoft.Authorization/roleAssignments:PrincipalId] ForAnyOfAnyValues:GuidEquals {$cms})) AND ((!(ActionMatches{'Microsoft.Authorization/roleAssignments/delete'})) OR (@Resource[Microsoft.Authorization/roleAssignments:RoleDefinitionId] ForAnyOfAnyValues:GuidEquals {$roles} AND @Resource[Microsoft.Authorization/roleAssignments:PrincipalId] ForAnyOfAnyValues:GuidEquals {$cms}))"
   az role assignment create --assignee-object-id "$deploy" --assignee-principal-type ServicePrincipal \
     --role "Role Based Access Control Administrator" --scope "$scope" \
     --condition "$condition" --condition-version 2.0 \
     --description "folk-ms deploy: only Storage Blob Data Owner, Key Vault Secrets User and Key Vault Secrets Officer, only to folkms-cms-id"
   ```

   The three role ids are `Storage Blob Data Owner`, `Key Vault Secrets User` (the platform vault) and `Key Vault Secrets Officer` (the token vault), the only roles `main.bicep` assigns. An existing assignment made with the two-role condition is replaced by running the command again after `az role assignment delete --assignee "$deploy" --role "Role Based Access Control Administrator" --scope "$scope"`. The identity's name must be `<prefix>-cms-id` (`main.bicep` parameter `cmsIdentityName`). If the deploy identity still holds `User Access Administrator` from an earlier setup, remove it: `az role assignment delete --assignee "$deploy" --role "User Access Administrator" --scope "$scope"`. No person gets a standing Key Vault data role; see [Key Vault and token incident response](#key-vault-and-token-incident-response) for the rare case someone must read a secret.
3. Create the [Google OAuth client](#google-oauth-client) and the [Meta app](#meta-app) without redirect URIs yet; note their ids and secrets.
4. In GitHub *Settings → Environments* create the environment `production` and protect it, because its secrets and its Azure sign-in are what an attacker with push access would want: under *Deployment branches and tags* allow only `main`, and under *Required reviewers* add the maintainer. Protect `main` too (*Settings → Rules*: require a pull request, block force pushes). Only the deploy jobs name the environment, and they run only Azure CLI on what the build jobs packaged; builds and tests run in jobs with no environment and no Azure sign-in. Set variables `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `GOOGLE_CLIENT_ID`, `META_APP_ID`, and secrets `SUPER_ADMIN_EMAIL`, `GOOGLE_CLIENT_SECRET`, `META_APP_SECRET`, `SESSION_KEY` (`openssl rand -base64 32`). `SUPER_ADMIN_EMAIL` is a secret because variables are not masked in logs. `deploy-infra` writes `META_APP_SECRET` and `SESSION_KEY` into Key Vault; the CMS's app settings hold only Key Vault references to them. Leave `CMS_BASE_URL` unset for now: the CMS then uses its default `*.azurewebsites.net` address.
5. Run `deploy-infra`. It prints no outputs; read them in the Azure portal (resource group → *Deployments* → `main` → *Outputs*) or with `az deployment group show -g <resource group> -n main --query properties.outputs`. Set the variables `CMS_FUNCTION_APP` (output `functionApp`), `CMS_BASE_URL` (output `cmsUrl`) and `STORAGE_PUBLIC_URL` (output `blobEndpoint` followed by `pub`, i.e. `https://<account>.blob.core.windows.net/pub`). Azure Monitor emails `SUPER_ADMIN_EMAIL` that it was added to an action group: that address receives the alerts. The budget (parameter `monthlyBudget`, default 10 in the billing currency) emails the same address at 50% and 100%.
6. Add the redirect URIs and the other `<CMS_BASE_URL>` addresses to the Google OAuth client and the Meta app as described in their sections below.
7. Run `deploy-cms` (its smoke test calls `CMS_BASE_URL`), then `deploy-site`. New role assignments can take a few minutes to apply; if the CMS reports that it cannot read `META_APP_SECRET` or `SESSION_KEY`, restart the Function app.
8. The Sites now work at their `*.azurestaticapps.net` addresses (output `staticSites`): CORS allows each Static Web App's default address without any configuration. Once a Site has a custom domain, add it to its Static Web App and its origin to `config/sites.json` (`origins` and `baseUrl`), then rerun `deploy-infra` so CORS allows it.

After that, pushes to `main` redeploy whatever changed: `infra/azure/**` runs `deploy-infra`, `cms/**` and `contract/**` run `deploy-cms`, `sites/**` and `contract/client/**` run `deploy-site`, and `config/sites.json` runs all three. Nothing needs to be installed locally to deploy.

Then the [Super Admin](#super-admin-bootstrap) signs in and invites Moderators, and a Moderator of each Organization [connects its Facebook page](#connecting-a-facebook-page-and-autosync) and runs the first Sync.

### Google OAuth client

In Google Cloud Console → APIs & Services: configure the consent screen (External, scopes `openid email profile`), then create an OAuth client of type *Web application* with the authorized redirect URI `<CMS_BASE_URL>/auth/callback`. Put its id and secret into `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`.

### Super Admin bootstrap

The Super Admin is whoever first signs in with the Google account whose email equals `SUPER_ADMIN_EMAIL`; that identity (Google `sub`) is then bound, and the Super Admin signs in with it only, never with Facebook. Changing `SUPER_ADMIN_EMAIL` re-binds on the next sign-in with the new email. The Super Admin invites Moderators under *Moderatorzy* and sends them the CMS link; Invitations expire after 14 days. A Moderator can sign in with Google or Facebook, and is the same user with either, as long as both accounts return the email they were invited with ([Access](docs/access/README.md#sign-in)).

### Meta app

1. At developers.facebook.com create a *Business* app, add *Facebook Login for Business* and the Instagram Graph API.
2. Valid OAuth redirect URIs: `<CMS_BASE_URL>/auth/meta/callback` (connecting a Facebook page, "Sync now", Autosync and posting) and `<CMS_BASE_URL>/auth/callback` (Facebook sign-in). Privacy policy URL: `<CMS_BASE_URL>/privacy`. Data deletion instructions URL: `<CMS_BASE_URL>/data-deletion`.
3. Permissions: the CMS asks for `pages_show_list`, `pages_read_engagement`, `pages_manage_posts`, `instagram_basic` and `instagram_content_publish` (and `email`, `public_profile` for Facebook sign-in); add them to the app if Meta asks. Standard Access is enough. `business_management` is no longer requested; an app set up for an earlier version may drop it.
4. Under *App settings → Advanced → Security*, switch on **Require App Secret**. Graph then refuses any call without `appsecret_proof`, so a stolen Facebook page token is useless without the app secret ([ADR 0010](docs/architecture/adr/0010-autosync-keeps-a-page-token-in-key-vault.md), T2). The CMS signs every call, so nothing else changes.
5. Keep the app in Standard Access. For every Moderator who will connect a Facebook page, run "Sync now", turn Autosync on, post Social Posts or sign in with Facebook: *App roles → Add people → Tester*; they accept the invitation in their Facebook developer settings. Their Meta account needs a role on the Organization's Facebook page that allows posting, and access to its Instagram account; the Facebook page must be linked to an Instagram professional account for Instagram posts.
6. Put the app id and secret into `META_APP_ID` / `META_APP_SECRET`.

### Connecting a Facebook page and Autosync

How Sync and Autosync work: [Sync](docs/sync/README.md) and [Social posting](docs/social-posting/README.md#autosync).

1. A Moderator of the Organization (a Tester with a role on the Facebook page) opens *Social media*, chooses the Facebook page with Meta, and picks the page from the list. The CMS stores the page's and the Instagram account's ids and names, no token.
2. On the Event list, *Sync now* imports the page's upcoming Facebook Events (not past ones). With Autosync off, every *Sync now* asks for a Meta sign-in; it runs at most once a minute.
3. Optionally, turn **Autosync** on under *Social media*: hourly Sync, and posting without a Meta sign-in each time. It stores a Facebook page token in Key Vault, so read the warning first. It needs a CMS sign-in in the last 15 minutes (the CMS asks you to sign in again otherwise) and a Meta sign-in by someone with a role on the page. Meta ends the token's data access 90 days later: from 14 days before, a "reconnect before" banner asks someone to turn Autosync on again, which issues a new token.
4. Turning Autosync off deletes the token at once; so do replacing or disconnecting the page, and the Super Admin removing the person who turned it on from the Organization. In each case the person who turned it on should also remove the app on Facebook under *Settings → Business integrations*, since a page token cannot revoke itself. A "needs reconnect" banner means Graph rejected the token (password changed, page role lost, app removed): turn Autosync on again.
5. Co-hosted Facebook Events another page created cannot be synced: add each with *Add co-hosted event* (a Manual Event) and keep it up to date by hand.

### Key Vault and token incident response

There are two Key Vaults ([ADR 0010](docs/architecture/adr/0010-autosync-keeps-a-page-token-in-key-vault.md)): the platform vault (output `keyVault`) holds the Meta app secret and `SESSION_KEY`, which the CMS identity `folkms-cms-id` can only read (`Key Vault Secrets User`); the token vault (output `tokenVault`) holds the Facebook page token of every Organization with Autosync on, which the CMS identity reads, writes, deletes and purges (`Key Vault Secrets Officer`). No person has a standing role on either. Both audit logs, and every write and delete in the storage account, go to the Log Analytics workspace `folkms-logs-<hash>`. Alerts email the Super Admin when:

- either vault is used in any way by an identity other than the CMS, or the `cms` container is written by one or with an account key or SAS;
- page tokens are read more than 100 times in an hour;
- a role assignment, a vault, the storage account's settings or keys, a diagnostic setting, an alert rule or the action group in the resource group changes (also after each `deploy-infra` run, which is expected);
- costs pass half or all of the monthly budget.

On an alert, see who did what in the workspace (*Logs*):

```kusto
AzureDiagnostics
| where ResourceProvider == "MICROSOFT.KEYVAULT"
| project TimeGenerated, Resource, OperationName, id_s, httpStatusCode_d, CallerIPAddress,
    caller = tostring(column_ifexists("identity_claim_oid_g", "")),
    user = tostring(column_ifexists("identity_claim_upn_s", ""))
| order by TimeGenerated desc
```

```kusto
StorageBlobLogs
| where ObjectKey has "/cms/"
| project TimeGenerated, OperationName, ObjectKey, RequesterObjectId, CallerIpAddress
| order by TimeGenerated desc
```

An object id resolves with `az ad sp show --id <id>` or `az ad user show --id <id>`. Remove any role assignment on a vault that is not the CMS identity's. A changed or deleted `cms` document can be restored from its previous blob version (kept 90 days) or from soft delete (30 days), the Audit Log included.

**One Organization's token may have leaked:**

1. A Moderator of the Organization or the Super Admin turns Autosync off in the CMS. That deletes and purges the secret.
2. The person who turned Autosync on (the connection screen names them) removes the app on Facebook under *Settings → Business integrations*. Only that kills the token at Meta: a page token cannot revoke itself.
3. Turn Autosync on again when it is safe; it stores a new token under a new secret name.

**A secret was not deleted:** the Audit Log has `autosync.secret_delete_failed` with the secret's name (the CMS log says `autosync secret not deleted` or `orphaned autosync secret`). The daily purge deletes any token secret no connection names once it is an hour old (`autosync.orphan_deleted`) and purges deleted ones. To do it at once, an `Owner` grants themselves `Key Vault Secrets Officer` on the token vault, runs `az keyvault secret delete --vault-name <tokenVault> -n <name>` and `az keyvault secret purge --vault-name <tokenVault> -n <name>`, and removes the assignment at once.

**Code execution in the CMS or the deploy pipeline is suspected:** do the above for every Organization with Autosync on, then rotate the Meta app secret (Meta app dashboard → *App settings → Basic → App secret → Reset*), put the new one into the `META_APP_SECRET` GitHub secret and run `deploy-infra`. With "Require App Secret" on, a token copied out is useless without the current app secret. Rotate `SESSION_KEY` the same way; that signs everyone out.

**Reading a secret by hand** should not be needed. If it is, an `Owner` grants themselves `Key Vault Secrets User` on the vault, reads, and removes the assignment at once; the alert fires, as it should.

### Who can post Social Posts

Either way, an Organization can attempt at most 10 Social Posts a day, and Hidden Events and Events in the Bin cannot be posted.

- **Autosync off:** the CMS stores no Meta token. A Moderator signs in with Meta every time they post, and the CMS posts with that sign-in only. So a Moderator can post only while they are a Tester on the Meta app and have a role on the Facebook page (and access to its Instagram account). To stop someone posting, remove their CMS access, their Facebook page role or their Tester role; nothing in the CMS needs reconnecting.
- **Autosync on:** any Moderator of the Organization, and the Super Admin, can post with the stored token after a CMS sign-in in the last 15 minutes; their own Meta roles do not matter. To stop someone posting, remove their CMS access to the Organization. To stop all posting with the token, turn Autosync off.

Changing the Organization's Facebook page or disconnecting it is done under *Social media*, by any of its Moderators or the Super Admin.

### Upgrade notes

**To Facebook Events as the source of Events** ([ADR 0009](docs/architecture/adr/0009-facebook-events-are-the-source-of-events.md)), one-off. Existing CMS Events, Venues and their index documents are discarded; Events come back from Facebook through Sync. Social Post records of the discarded Events go with them (the Audit Log keeps their URLs).

1. Deploy this version. The CMS and both Sites must deploy together (snapshot schema version 2); the change touches `contract/client/`, so the push runs both `deploy-cms` and `deploy-site`.
2. Delete the old documents. In Azure, an `Owner` grants themselves `Storage Blob Data Contributor` on the storage account for this and removes it afterwards; then:

   ```bash
   for p in events venues shortids posts; do
     az storage blob delete-batch --auth-mode login --account-name <account> -s cms --pattern "data/orgs/*/$p/*"
   done
   ```

   Locally, stop the CMS and run `rm -rf .data/cms/orgs/*/events .data/cms/orgs/*/venues .data/cms/orgs/*/shortids .data/cms/orgs/*/posts`. Blob versioning and soft delete keep copies for 90 and 30 days.
3. Restart the Function app (or the local CMS): it rebuilds every snapshot when it starts, now without the old Events.
4. In each Organization, connect the Facebook page if needed and press *Sync now*; add co-hosted events as Manual Events.
5. **Moderators: remove Autofill.** The Autofill browser extension and userscript are gone and do nothing any more. Remove the extension (Chrome or Edge: `chrome://extensions` or `edge://extensions`; Firefox: `about:addons`) and the userscript from Violentmonkey, Tampermonkey or the iOS userscript manager. Whoever published the unlisted extension listings removes them from the Chrome Web Store and addons.mozilla.org.

**From a version that stored Facebook page tokens** (before ADR 0005): the next daily purge rewrites each connection without its token, and old blob versions expire within 90 days. Delete the `TOKEN_ENC_KEY` secret from the GitHub `production` environment. The old token stays valid at Meta until the person who connected removes the app under Facebook *Settings → Apps and websites*; they approve it again on their next post.

### Backup and restore

- Blob versioning keeps every previous version of every document for 90 days; soft delete keeps deleted blobs and containers for 30 days. To restore a document, find it in the Azure portal (container `cms`, prefix `data/`, *Show deleted* / *Versions*) and promote the version; then restart the Function app, or have a Super Admin call `POST /api/admin/rebuild`, to rebuild the snapshots.
- Snapshots in `pub/sites/` are derived data and are rebuilt on every CMS start.
- Events purged from the Bin after 30 days are gone; the Audit Log (`cms/audit/<day>.log`) keeps their history, including Facebook and Instagram URLs.
- Offline copy: `az storage blob download-batch --auth-mode login -s cms -d ./backup --account-name <account>`.
- Key Vault is not backed up, by design. The Meta app secret and `SESSION_KEY` live in the GitHub `production` secrets and every `deploy-infra` run writes them again. A Facebook page token is replaced by turning Autosync on again. A deleted secret stays in soft delete for 7 days and is then purged; the CMS never reuses its name, so recovering it is not needed.
