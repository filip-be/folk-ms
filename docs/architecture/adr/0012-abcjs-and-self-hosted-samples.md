# 0012. abcjs from npm for music notation, with self-hosted samples

- Status: accepted
- Date: 2026-11-28
- A second, named exception to [ADR 0003](0003-stdlib-only-go.md) (zero runtime npm dependencies); updated as [ADR 0007](0007-latest-toolchains-and-dependabot-cooldown.md) describes

## Context

The Tune Collection stores Transcriptions in ABC notation. The admin's Tune editor previews them while the Moderator types, and the Sites show the sheet music and play it. Drawing staff notation from ABC (beams, ties, repeats, first and second endings, grace notes, line breaks) and turning it into timed notes is a large, well-solved problem. abcjs (MIT, by Paul Rosen and Gregory Dyke) does both. Writing our own would take far more volunteer time than reviewing a dependency.

ADR 0003 allows no runtime npm package. The maintainers trust the abcjs author and accept abcjs as a named exception. The package has no dependencies of its own, so the lockfile gains one entry. It ships a prebuilt, minified `dist/abcjs-basic-min.js` and its own TypeScript types (`types/index.d.ts`).

abcjs is about 0.5 MB minified. Most admin screens and Site pages show no sheet music, so they must not download it.

abcjs's synth plays per-note MP3 samples, which it loads by default from a third-party host. A Site must not depend on, or tell, a third party each time a visitor presses play.

## Decision

- **Installed from npm.** `abcjs` is the one entry under `dependencies` in the root `package.json`, at an exact version, locked with integrity in `package-lock.json` and installed with `npm ci --ignore-scripts`. The dev-only `typescript` keeps its caret range: for a tool that ships nothing, the lockfile alone is enough. For code that ships to visitors, the exact version makes `package.json` say what the lockfile installs, and a Dependabot PR changes both.
- **Updated by Dependabot with the 7-day cooldown** (ADR 0007), like every other npm package. An upgrade is a reviewed PR: read the changelog and check the Tune screens and pages before merging.
- **Lazy, one chunk.** `notation/abcjs.ts`, imported by the admin and the Sites, `import()`s the package's prebuilt `abcjs/dist/abcjs-basic-min.js`, not the package root (which would bundle the CommonJS sources, about 15 KB larger). Bun's code splitting makes it its own chunk (`chunks/abcjs-basic-min-<hash>.js` next to `app.js` / `main.js`), which only Tune screens and pages download. Nothing else imports the package except for types.
- **Typed by the package.** The wrapper re-exports the package's types; `notation/abcjs-dist.d.ts` only maps the dist file to them.
- **Enforced in CI.** `scripts/vendor.test.ts` checks that abcjs is the only runtime dependency, at an exact version that the lockfile pins with integrity and with no dependencies of its own, and that only the wrapper imports it, through the dist file.
- **Samples are self-hosted.** `vendor/soundfonts/FluidR3_GM/` holds the FluidR3 GM soundfont (Frank Wen, CC BY 3.0) as packaged by gleitz/midi-js-soundfonts at a fixed commit, for piano, flute, fiddle (`violin`) and accordion, notes G3 to D7 only (melody; chords are off), with `SHA256SUMS` and `CREDITS`. `scripts/fetch-samples.ts` downloads them; the files are committed, and `scripts/vendor.test.ts` checks the checksums. Dependabot does not see them; an update repeats the fetch. `sites/build.ts` copies all four into a Site with the Tune Collection, served at `/sounds/`; `cms/admin/build.ts` copies piano only, embedded in the binary and served at `/app/sounds/`. Wherever samples play, the page shows the CC BY credit.

## Alternatives

- **abcjs vendored** (its prebuilt file committed with checksums, no npm entry): keeps ADR 0003 whole, but bug and security fixes wait for someone to notice a release and repeat the vendoring by hand, and hand-written types drift from the API.
- **`import("abcjs")` from the package root:** the same API, but Bun bundles the CommonJS sources instead of the minified release file, and the chunk grows.
- **A caret range, like `typescript`:** `npm ci` installs the locked version either way, but `package.json` would no longer name the code that ships.
- **Our own ABC renderer:** no third-party code, but months of work for notation that abcjs already draws well.
- **Server-side rendering to SVG in Go:** no JavaScript library, but no live preview while typing and no playback.
- **Samples from the default third-party host:** no files in the repository, but every play reaches another origin, and a Site breaks when it goes away.

## Consequences

- About 0.5 MB of minified third-party JavaScript ships to visitors of Tune pages and to Moderators on Tune screens. Other pages and screens are unchanged.
- abcjs releases reach the repository as Dependabot PRs after a week, security updates at once. What ships is the release's own minified file, at a version the lockfile pins with integrity. abcjs draws SVG from ABC text and does not fetch or run code; the Transcriptions it draws are written by Moderators.
- `node_modules` is needed to build, not only to type-check: CI runs `npm ci --ignore-scripts` before the admin and Site builds (`--omit=dev` in the deploy workflows, which install abcjs alone), and developers run it once after cloning and after each upgrade.
- A change to `package.json`, `package-lock.json` or `notation/` deploys the CMS (whose binary embeds the admin) and the Sites.
- abcjs inserts `<style>` elements and `style` attributes, so the admin and Site CSPs allow `style-src 'unsafe-inline'`; scripts stay `'self'`.
- The repository grows by about 4.7 MB of samples; the CMS binary by about 1.1 MB (piano) plus abcjs.
