# cms

The folk-ms CMS: one Go module, one binary. How it fits together is in [Platform](../docs/platform/README.md); the capabilities it implements are under [docs/](../docs/README.md).

| Path | What |
| --- | --- |
| `cmd/cms/` | Composition root (adapter choice from the environment) |
| `internal/domain/` | Pure domain: Events, Recurrence, Venues, access, Social Posts, snapshot, Caption |
| `internal/app/` | Use cases with per-Organization authorization |
| `internal/http/` | Routes, sessions, CSRF, headers, static SPA |
| `internal/ports/`, `internal/adapters/` | Ports and their adapters |
| `internal/schema/` | Validator for `contract/events.schema.json` |
| `admin/` | Admin SPA source (`src/`) and the embedded build (`dist/`, git-ignored) |
| `deploy/` | Azure Functions package (`host.json`, function folders, `package.sh`) |

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
| `FOLKMS_ENV` | Required: `local` (filesystem in `DATA_DIR`) or `azure` |
| `BASE_URL` | Public origin of the CMS |
| `SITES_CONFIG` | Path of `sites.json` (default: next to the binary, or `../config/sites.json`) |
| `SUPER_ADMIN_EMAIL` | The Super Admin's Google email |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | Google OAuth client (without them a `dev` build uses the stub) |
| `META_APP_ID`, `META_APP_SECRET`, `META_GRAPH_VERSION` | Meta app (without them, only a `dev` build runs, recording posts instead of sending them) |
| `SESSION_KEY` | Base64 key of 32+ bytes for cookies (azure only; local: generated) |
| `STORAGE_BLOB_URL` | Blob endpoint; with it, local runs use blob storage too |
| `STORAGE_ACCOUNT_KEY`, `STORAGE_ACCOUNT_NAME` | SharedKey for Azurite; without it the managed identity is used |
| `STORAGE_PUBLIC_URL` | Public URL of the `pub` container (default `<blob>/pub`) |
| `STORAGE_CREATE_CONTAINERS` | `1` creates `cms` and `pub` at start (Azurite) |
| `PORT` / `FUNCTIONS_CUSTOMHANDLER_PORT` | Listen port; the latter means "running under the Functions host" |

## Dependencies

Standard library and `golang.org/x/image` only. After changing `go.mod`: `go mod tidy`. Dependabot keeps the modules current. See [ADR 0003](../docs/architecture/adr/0003-stdlib-only-go.md) and [ADR 0007](../docs/architecture/adr/0007-latest-toolchains-and-dependabot-cooldown.md).
