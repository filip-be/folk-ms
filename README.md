# folk-ms

CMS for bal folk websites: Moderators manage bilingual Events, Publish them to each Organization's static Site, and announce them on Facebook and Instagram.

Documentation: [docs/README.md](docs/README.md). Glossary: [CONTEXT.md](CONTEXT.md).

## Repository

| Path | What |
| --- | --- |
| [`cms/`](cms/) | The CMS: one Go binary with the embedded admin SPA ([`cms/admin/`](cms/admin/)) |
| [`contract/`](contract/) | Snapshot JSON Schema, fixtures and the TypeScript reader Sites use |
| [`sites/`](sites/) | One static Site skeleton per Organization |
| [`autofill/`](autofill/) | Autofill: the browser helper that types a Facebook Event Handoff into Facebook's form (WebExtension and userscript) |
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
bun test contract sites autofill scripts
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

The Super Admin is whoever first signs in with the Google account whose email equals `SUPER_ADMIN_EMAIL`; that identity (Google `sub`) is then bound, and the Super Admin signs in with it only, never with Facebook. Changing `SUPER_ADMIN_EMAIL` re-binds on the next sign-in with the new email. The Super Admin invites Moderators under *Moderatorzy* and sends them the CMS link; Invitations expire after 14 days. A Moderator can sign in with Google or Facebook, and is the same user with either, as long as both accounts return the email they were invited with ([Access](docs/access/README.md#sign-in)).

### Meta app

1. At developers.facebook.com create a *Business* app, add *Facebook Login for Business* and the Instagram Graph API.
2. Valid OAuth redirect URIs: `<CMS_BASE_URL>/auth/meta/callback` (posting) and `<CMS_BASE_URL>/auth/callback` (Facebook sign-in). Privacy policy URL: `<CMS_BASE_URL>/privacy`. Data deletion instructions URL: `<CMS_BASE_URL>/data-deletion`.
3. Keep the app in Standard Access. For every Moderator who will post Social Posts, connect a Facebook page or sign in with Facebook: *App roles → Add people → Tester*; they accept the invitation in their Facebook developer settings. Their Meta account needs a role on the Organization's Facebook page that allows posting, and access to its Instagram account; the Facebook page must be linked to an Instagram professional account for Instagram posts.
4. Put the app id and secret into `META_APP_ID` / `META_APP_SECRET`.

### Who can post Social Posts

The CMS stores no Meta token: a Moderator signs in with Meta every time they post, and the CMS posts with that sign-in only. So a Moderator can post only while they are a Tester on the Meta app and have a role on the Facebook page (and access to its Instagram account). To stop someone posting, remove their CMS access, their Facebook page role or their Tester role; nothing in the CMS needs reconnecting. Changing the Organization's Facebook page or disconnecting it is done under *Media społecznościowe*, by any of its Moderators or the Super Admin.

Upgrading from a version that stored Facebook page tokens: the next daily purge rewrites each connection without its token, and old blob versions expire within 90 days. Delete the `TOKEN_ENC_KEY` secret from the GitHub `production` environment. The old token stays valid at Meta until the person who connected removes the app under Facebook *Settings → Apps and websites*; they approve it again on their next post.

### Autofill

Autofill types a Facebook Event Handoff into Facebook's create-event form; the Moderator still presses Create. How it works: [Facebook events](docs/facebook-events/README.md).

**Terms risk, for Moderators.** Autofill acts inside your own Facebook session, which Meta's terms restrict for automated tools. It only types what you could type and never presses Create, but use it at your own discretion; the copy buttons on the Handoff always work without it. It is unofficial and will break when Facebook changes its form: then fill the fields the panel marks as *needs you* or *failed* with its copy buttons.

**Publishing the extension** (desktop Chrome/Edge and Firefox). Bump `VERSION` in `autofill/lib.ts`, then build for the production CMS:

```bash
CMS_ORIGIN=https://<cms host> bun autofill/build.ts
```

- Chrome Web Store: zip the contents of `autofill/dist/extension-chrome/` and upload it in the developer dashboard (listing visibility *Unlisted*; the extension works only with this CMS). Edge users install it from the Chrome Web Store.
- addons.mozilla.org: zip `autofill/dist/extension-firefox/` and submit it as an unlisted add-on; it is signed for desktop Firefox only (no Android).
- Privacy answers for both stores: no data is collected or sent; a handoff is kept in the browser for at most 15 minutes. Point both listings at `<CMS_BASE_URL>/privacy`.

**The userscript** is built with the admin SPA and served by the CMS at `<CMS_BASE_URL>/app/autofill.user.js` with its origin filled in; managers update it from there after every CMS deploy. Installing it:

- Desktop: Violentmonkey or Tampermonkey, then open the address above and confirm. Use the extension or the userscript, not both (if both are installed, the second stays idle).
- Firefox for Android: install Violentmonkey from addons.mozilla.org, open the address above, confirm.
- iOS Safari (best effort): install a userscript manager from the App Store (Userscripts, Stay or Macaque), enable it for `facebook.com` and the CMS host, and add the script from the address above. When facebook.com opens in the Facebook app instead, long-press "Open Facebook" and open it in Safari; when it still fails, use the copy buttons.

**Per-release checklist against real Facebook**, with a test Facebook page, on desktop Chrome, desktop Firefox, Firefox for Android (Violentmonkey) and iOS Safari (one manager), in English and Polish Facebook UI:

1. On the Handoff, "Fill on Facebook" appears (Autofill detected); with nothing installed the install hint shows.
2. Facebook opens the create form; switch to acting as the Facebook page. The panel appears and no field is filled twice.
3. Each field ends as *filled*, or as *needs you* where the field map knows it is manual (on m.facebook.com everything but name, description and cover). Check name, start and end date and time, time zone, location, description line breaks, cover photo and ticket link in the form itself.
4. Autofill did not press Create. Press it; the panel shows the new link and "Back to the CMS".
5. Back in the CMS the link is offered for confirmation; confirm it and check it on the Site.
6. Break a locator on purpose in a local field map: the panel shows *failed* with a copy button, and `autofill.reported` appears in the Audit Log after returning to the CMS.
7. Record what changed in `autofill/fieldmap.json` (`checked`) and, for platforms that newly work or stop working, in ADR 0006.

Fixing a broken field: adjust its locators in `autofill/fieldmap.json` (labels and roles only; class selectors are refused) and deploy the CMS. Only a change in the fill engine itself needs a store release.

### Backup and restore

- Blob versioning keeps every previous version of every document for 90 days; soft delete keeps deleted blobs and containers for 30 days. To restore a document, find it in the Azure portal (container `cms`, prefix `data/`, *Show deleted* / *Versions*) and promote the version; then a Super Admin triggers `POST /api/admin/rebuild` (or any Publish) to rebuild the snapshots.
- Snapshots in `pub/sites/` are derived data and are rebuilt on every CMS start.
- Events purged from the Bin after 30 days are gone; the Audit Log (`cms/audit/<day>.log`) keeps their history, including Facebook and Instagram URLs.
- Offline copy: `az storage blob download-batch --auth-mode login -s cms -d ./backup --account-name <account>`.
