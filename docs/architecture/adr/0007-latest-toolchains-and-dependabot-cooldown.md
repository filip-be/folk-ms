# 0007. Latest Go and Bun; dependencies through Dependabot with a cooldown

- Status: accepted
- Date: 2026-11-10
- Supersedes the pinned toolchains and the vendoring in [ADR 0003](0003-stdlib-only-go.md)

## Context

ADR 0003 pinned Go and Bun by version and SHA-256 in `tools/`, pinned `typescript` to an exact version, and vendored `golang.org/x/image`. Every bugfix or security release of any of them then waited for a maintainer to notice it and bump it by hand. For volunteers that means fixes arrive late or not at all, which is a bigger risk than the one the pins avoided.

Dependabot can update npm packages, Go modules and GitHub Actions, and its cooldown holds a new release back for some days. A malicious publish is usually found and pulled within that time. Dependabot cannot update the Go toolchain (the `go` and `toolchain` lines of `go.mod`, dependabot-core issue #13520) or the Bun runtime: its `bun` ecosystem covers packages in `bun.lock` only.

## Decision

- **Go and Bun:** CI installs the latest release of each on every run. Go comes from `actions/setup-go` with `go-version: stable` and `check-latest: true`. Bun comes from `.github/scripts/setup-bun.sh`, which downloads the latest GitHub release and checks it against the release's `SHASUMS256.txt`. The `go` line in `cms/go.mod` is a minimum, not a pin. Developers install both the usual way for their OS and keep them up to date.
- **Packages:** `typescript` (npm), `golang.org/x/image` (Go module, no longer vendored) and the GitHub Actions are updated by Dependabot with a 7-day cooldown. Security updates are not held back. `go.sum` and the Go checksum database verify every module download. CI checks that `go.mod` and `go.sum` are tidy.
- **CI tools run at latest:** `govulncheck` runs at `@latest`.
- ADR 0003's limits on what may be depended on are unchanged.

## Alternatives

- **Keep the pins and bump them by hand:** reproducible, but fixes depend on someone remembering to bump them.
- **Track Go and Bun through Docker image tags** (`FROM golang:X`, `FROM oven/bun:Y` in a version-only Dockerfile that Dependabot's `docker` ecosystem bumps): every update would get a reviewed PR and the cooldown. Rejected as an indirection kept only for its version numbers.
- **`oven-sh/setup-bun`:** a third-party action, which ADR 0003 excludes. The script does the same in a few lines.

## Consequences

- Toolchain fixes reach CI on the next run with no PR. A new Go or Bun release can break the build without any change in the repository; the fix is a code change, or briefly pinning the version in the workflow.
- Go and Bun releases get no cooldown. They come from the projects' own release channels (go.dev and the Bun GitHub releases) and not from a public registry, so the risk the cooldown guards against is smaller there.
- Builds of the same commit are not reproducible across toolchain releases.
- Upgrading `golang.org/x/image` is a Dependabot PR changing `go.mod` and `go.sum`, not a vendored tree.
