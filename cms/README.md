# cms

The folk-ms CMS: one Go module, one binary. How it fits together is in [Platform](../docs/platform/README.md); the capabilities it implements are under [docs/](../docs/README.md).

| Path | What |
| --- | --- |
| `cmd/cms/` | Composition root (adapter choice from the environment) |
| `internal/domain/` | Pure domain: Events (synced and Manual, Overrides, the Bin), Sync, access, Social Posts and the connection, snapshot, Caption |
| `internal/app/` | Use cases with per-Organization authorization |
| `internal/http/` | Routes, sessions, CSRF, headers, static SPA |
| `internal/ports/`, `internal/adapters/` | Ports and their adapters |
| `internal/schema/` | Validator for `contract/events.schema.json` |
| `admin/` | Admin SPA source (`src/`) and the embedded build (`dist/`, git-ignored) |
| `deploy/` | Azure Functions package (`host.json`, the `http`, `purge` and `sync` function folders, `package.sh`) |

## Build and test

```bash
go test ./...
```

```bash
go build -tags dev -o cms-dev ./cmd/cms
```

The `dev` tag adds the local sign-in stub; production builds leave it out. Build the SPA first (`bun admin/build.ts`) or the binary serves a "not built" notice under `/app/`. `scripts/dev.sh` at the repository root does both and runs it.

## Configuration

| Variable | Meaning |
| --- | --- |
| `FOLKMS_ENV` | Required: `local` (filesystem in `DATA_DIR`, default `.data`) or `azure` |
| `BASE_URL` | Public origin of the CMS |
| `SITES_CONFIG` | Path of `sites.json` (default: next to the binary, or `../config/sites.json`) |
| `SUPER_ADMIN_EMAIL` | The Super Admin's Google email |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | Google OAuth client (without them a `dev` build uses the stub) |
| `META_APP_ID`, `META_APP_SECRET`, `META_GRAPH_VERSION` | Meta app, for Facebook sign-in, Sync and Social Posts. Without `META_APP_ID` only a `dev` build runs, with the Meta recorder: posts are recorded instead of sent, and Sync reads canned Facebook Events |
| `SESSION_KEY` | Base64 key of 32+ bytes for cookies (azure only, a Key Vault reference there; local: generated in `DATA_DIR`) |
| `KEYVAULT_URL` | Key Vault for Autosync's Facebook page tokens, read with the managed identity. Required in `azure` mode; when unset locally, the file store in `SECRETS_DIR` is used |
| `SECRETS_DIR` | Directory of the file secret store (default `DATA_DIR/secrets`; created `0700`, files `0600`). Keep it out of backups that leave the machine |
| `AZURE_CLIENT_ID` | Client id of the CMS's user-assigned managed identity, for blob and Key Vault tokens (replaces `STORAGE_IDENTITY_CLIENT_ID`) |
| `STORAGE_BLOB_URL` | Blob endpoint; with it, local runs use blob storage too |
| `STORAGE_ACCOUNT_KEY`, `STORAGE_ACCOUNT_NAME` | SharedKey for Azurite; without it the managed identity is used (`IDENTITY_ENDPOINT`, `IDENTITY_HEADER`, set by the Functions host) |
| `STORAGE_PUBLIC_URL` | Public URL of the `pub` container (default `<blob>/pub`) |
| `STORAGE_CREATE_CONTAINERS` | `1` creates `cms` and `pub` at start (Azurite) |
| `PORT` / `FUNCTIONS_CUSTOMHANDLER_PORT` | Listen port; the latter means "running under the Functions host" |
| `HOST` | Listen address (default: every interface; `scripts/dev.sh` uses `127.0.0.1`) |

## Timer routes

The Functions host calls the timer functions as HTTP requests to the binary; both routes are public, take no input and are rate-bounded:

| Function | Schedule (UTC) | Request | What |
| --- | --- | --- | --- |
| `purge` | `0 30 3 * * *` | `POST /purge` | Purges the Bin and expired Post Requests ([Event management](../docs/event-management/README.md#the-bin)) |
| `sync` | `0 7 * * * *` | `POST /sync` | Autosync of every Organization that turned it on ([Sync](../docs/sync/README.md)) |

The Super Admin can run the same by hand with `POST /api/admin/purge` and `POST /api/admin/autosync`, and rebuild every snapshot with `POST /api/admin/rebuild`.

## Dependencies

Standard library and `golang.org/x/image` only. After changing `go.mod`: `go mod tidy`. Dependabot keeps the modules current. See [ADR 0003](../docs/architecture/adr/0003-stdlib-only-go.md) and [ADR 0007](../docs/architecture/adr/0007-latest-toolchains-and-dependabot-cooldown.md).
