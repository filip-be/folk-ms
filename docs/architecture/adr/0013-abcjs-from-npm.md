# 0013. abcjs from npm, as the one runtime npm dependency

- Status: accepted
- Date: 2026-11-28
- Supersedes the vendoring of abcjs in [ADR 0012](0012-abcjs-vendored-for-notation.md); a second, named exception to [ADR 0003](0003-stdlib-only-go.md) (zero runtime npm dependencies); updated as [ADR 0007](0007-latest-toolchains-and-dependabot-cooldown.md) describes

## Context

ADR 0012 took abcjs into the repository as a vendored copy of its prebuilt `dist/abcjs-basic-min.js`, with checksums, so that ADR 0003's "no runtime npm package" held and Dependabot did not see it. That left abcjs as the one dependency that ADR 0007's reasoning does not cover: its bug and security fixes reach us only when a maintainer notices a release and repeats the vendoring by hand, and its API is typed by declarations we wrote and keep in step ourselves.

The maintainers trust the abcjs author (Paul Rosen, with Gregory Dyke) and accept abcjs as a runtime npm dependency. The package has no dependencies of its own, so the lockfile gains one entry. It ships the same prebuilt file the repository vendored (byte-identical for 6.7.3) and its own TypeScript types (`types/index.d.ts`).

## Decision

- **Installed, not vendored.** `abcjs` is the one entry under `dependencies` in the root `package.json`, at an exact version, locked with integrity in `package-lock.json` and installed with `npm ci --ignore-scripts`. `vendor/abcjs/` is gone. The dev-only `typescript` keeps its caret range: for a tool that ships nothing, the lockfile alone is enough. For code that ships to visitors, the exact version makes `package.json` say what the lockfile installs, and a Dependabot PR changes both.
- **Updated by Dependabot with the 7-day cooldown** (ADR 0007), like every other npm package. An upgrade is a reviewed PR: read the changelog and check the Tune screens and pages before merging.
- **Lazy, one chunk.** `notation/abcjs.ts`, imported by the admin and the Sites, `import()`s the package's prebuilt `abcjs/dist/abcjs-basic-min.js`, not the package root (which would bundle the CommonJS sources, about 15 KB larger). Bun's code splitting makes it its own chunk (`chunks/abcjs-basic-min-<hash>.js` next to `app.js` / `main.js`), which only Tune screens and pages download. Nothing else imports the package except for types.
- **Typed by the package.** The wrapper re-exports the package's types; `notation/abcjs-dist.d.ts` only maps the dist file to them.
- **Enforced in CI.** `scripts/vendor.test.ts` checks that abcjs is the only runtime dependency, at an exact version that the lockfile pins with integrity and with no dependencies of its own, and that only the wrapper imports it, through the dist file.
- **Samples unchanged.** ADR 0012's decision on samples stands as written: the FluidR3 GM samples stay vendored in `vendor/soundfonts/FluidR3_GM/` with checksums, served from the Site's own origin and, piano only, from the admin's.

## Alternatives

- **Keep the vendored copy (ADR 0012):** no runtime entry in the lockfile, but fixes wait for someone to repeat the vendoring, and the hand-written types drift from the API.
- **`import("abcjs")` from the package root:** the same API, but Bun bundles the CommonJS sources instead of the minified release file, and the chunk grows.
- **A caret range, like `typescript`:** `npm ci` installs the locked version either way, but `package.json` would no longer name the code that ships.

## Consequences

- abcjs releases reach the repository as Dependabot PRs after a week, security updates at once. What ships is still the release's own minified file, at a version the lockfile pins with integrity.
- `node_modules` is now needed to build, not only to type-check: CI runs `npm ci --ignore-scripts` before the admin and Site builds (`--omit=dev` in the deploy workflows, which install abcjs alone), and developers run it once after cloning and after each upgrade.
- A change to `package.json`, `package-lock.json` or `notation/` deploys the CMS (whose binary embeds the admin) and the Sites.
- The CSP consequences of ADR 0012 (`style-src 'unsafe-inline'`, scripts `'self'`) are unchanged.
