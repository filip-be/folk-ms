# 0003. Minimal supply chain: stdlib-only Go, no npm runtime dependencies

- Status: accepted
- Date: 2026-11-07

## Context

The CMS holds Meta Page tokens and decides who may publish for an Organization. It is maintained by volunteers, so every dependency is an attack surface and an upgrade chore.

## Decision

- **Go:** standard library and `golang.org/x/*` only (today only `golang.org/x/image`, for WebP decoding and resampling), vendored, with `govulncheck` in CI. No Azure SDK: blob REST, SharedKey signing and managed-identity tokens are hand-written. No OIDC or JWT library: the Google adapter verifies RS256 ID tokens itself (`iss`, `aud`/`azp`, `exp`, `iat`, nonce, `email_verified`). The JSON Schema validator is our own and accepts only the keywords the schema uses.
- **TypeScript:** zero runtime npm dependencies. Bun (pinned, downloaded and SHA-256-verified by `tools/fetch.sh`) bundles, tests and serves. The only npm package is the dev-only type-checker `typescript`, at an exact version with lockfile integrity, installed with `npm ci --ignore-scripts`. TypeScript 7 ships its native compiler as platform packages under the `@typescript/` scope from the same publisher; that is accepted.
- **GitHub Actions:** only `actions/*`, pinned by commit SHA; Dependabot updates actions only. Azure sign-in uses the runner's preinstalled Azure CLI with the workflow's OIDC token (`.github/scripts/az-login.sh`) instead of a third-party action.
- **Exception:** deploying to Static Web Apps uses `@azure/static-web-apps-cli` at an exact version through `npx`, in `deploy-site.yml` only. It never ships to visitors; replace it if Static Web Apps gains a plain upload API.

## Alternatives

- Azure SDK for Go, `coreos/go-oidc`, `golang-jwt`: well maintained, but each brings a dependency tree.
- A front-end framework: faster to write screens, but a runtime dependency tree in the browser and in the build.

## Consequences

- Hand-written security code (ID-token verification, cookie signing, SharedKey) is covered by focused tests, including tampering and wrong-claim cases.
- Upgrades are deliberate: bump `tools/versions.env` and `tools/checksums.txt`, run `go get` and `go mod vendor`, or edit `package.json`.
