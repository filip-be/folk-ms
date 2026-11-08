# folk-ms

CMS for bal folk websites: Moderators manage bilingual Events, Publish them to each Organization's static Site, and announce them on Facebook and Instagram.

Documentation: [docs/README.md](docs/README.md). Glossary: [CONTEXT.md](CONTEXT.md).

## Repository

| Path | What |
| --- | --- |
| [`cms/`](cms/) | The CMS: one Go binary with the embedded admin SPA ([`cms/admin/`](cms/admin/)) |
| [`contract/`](contract/) | Snapshot JSON Schema, fixtures and the TypeScript reader Sites use |
| [`sites/`](sites/) | One static Site skeleton per Organization |
| [`config/sites.json`](config/sites.json) | The fixed list of Organizations and Sites |
| [`infra/azure/`](infra/azure/) | Bicep |
| [`tools/`](tools/) | Pinned Go and Bun with checksums |
| [`docs/`](docs/) | Capability articles and [ADRs](docs/architecture/adr/) |

## Local development

Prerequisites: Git Bash (Windows) or any POSIX shell, `curl`, `unzip`, Node.js/npm (only for the type-checker), Docker (only for Azurite tests).

```bash
tools/fetch.sh
```

Downloads the pinned Go and Bun into `.tools/` and verifies their SHA-256. Put them on `PATH`:

```bash
export PATH="$PWD/.tools/go/bin:$PWD/.tools/bun:$PATH"
```

Run the CMS at http://localhost:8080/app/ with filesystem storage in `.data/`, a dev sign-in page that accepts any email (`admin@example.org` is the Super Admin), and Social Posts recorded instead of sent:

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
bun test contract sites
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

1. Create a resource group and an Entra app registration with a federated credential for this repository's `production` environment; grant it `Contributor` and `User Access Administrator` (for the role assignment) on the resource group.
2. In the GitHub `production` environment set variables `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, `GOOGLE_CLIENT_ID`, `META_APP_ID`, `CMS_BASE_URL` (e.g. `https://<app>.azurewebsites.net`), and secrets `SUPER_ADMIN_EMAIL`, `GOOGLE_CLIENT_SECRET`, `META_APP_SECRET`, `SESSION_KEY` (`openssl rand -base64 32`). `SUPER_ADMIN_EMAIL` is a secret because variables are not masked in logs.
3. Run `deploy-infra`, then set `CMS_FUNCTION_APP` and `STORAGE_PUBLIC_URL` (`https://<account>.blob.core.windows.net/pub`) from its outputs, then run `deploy-cms` and `deploy-site`.
4. Add each Site's custom domain to its Static Web App and its origin to `config/sites.json` (`origins` and `baseUrl`), then rerun `deploy-infra` so CORS allows it.

### Google OAuth client

In Google Cloud Console → APIs & Services: configure the consent screen (External, scopes `openid email profile`), then create an OAuth client of type *Web application* with the authorized redirect URI `<CMS_BASE_URL>/auth/callback`. Put its id and secret into `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`.

### Super Admin bootstrap

The Super Admin is whoever first signs in with the Google account whose email equals `SUPER_ADMIN_EMAIL`; that identity (Google `sub`) is then bound. Changing `SUPER_ADMIN_EMAIL` re-binds on the next sign-in with the new email. The Super Admin invites Moderators under *Moderatorzy* and sends them the CMS link; Invitations expire after 14 days.

### Meta app

1. At developers.facebook.com create a *Business* app, add *Facebook Login for Business* and the Instagram Graph API.
2. Valid OAuth redirect URI: `<CMS_BASE_URL>/auth/meta/callback`. Privacy policy URL: `<CMS_BASE_URL>/privacy`. Data deletion instructions URL: `<CMS_BASE_URL>/data-deletion`.
3. Keep the app in Standard Access. For every Moderator who will post Social Posts or connect a Page: *App roles → Add people → Tester*; they accept the invitation in their Facebook developer settings. Their Meta account needs a role on the Organization's Page that allows posting, and access to its Instagram account; the Page must be linked to an Instagram professional account for Instagram posts.
4. Put the app id and secret into `META_APP_ID` / `META_APP_SECRET`.

### Who can post Social Posts

The CMS stores no Meta token: a Moderator signs in with Meta every time they post, and the CMS posts with that sign-in only. So a Moderator can post only while they are a Tester on the Meta app and have a role on the Page (and access to its Instagram account). To stop someone posting, remove their CMS access, their Page role or their Tester role; nothing in the CMS needs reconnecting. Changing the Organization's Page or disconnecting it is done under *Media społecznościowe*, by any of its Moderators or the Super Admin.

Upgrading from a version that stored Page tokens: the next daily purge rewrites each connection without its token, and old blob versions expire within 90 days. Delete the `TOKEN_ENC_KEY` secret from the GitHub `production` environment. The old token stays valid at Meta until the person who connected removes the app under Facebook *Settings → Apps and websites*; they approve it again on their next post.

### Backup and restore

- Blob versioning keeps every previous version of every document for 90 days; soft delete keeps deleted blobs and containers for 30 days. To restore a document, find it in the Azure portal (container `cms`, prefix `data/`, *Show deleted* / *Versions*) and promote the version; then a Super Admin triggers `POST /api/admin/rebuild` (or any Publish) to rebuild the snapshots.
- Snapshots in `pub/sites/` are derived data and are rebuilt on every CMS start.
- Events purged from the Bin after 30 days are gone; the Audit Log (`cms/audit/<day>.log`) keeps their history, including Facebook and Instagram URLs.
- Offline copy: `az storage blob download-batch --auth-mode login -s cms -d ./backup --account-name <account>`.
