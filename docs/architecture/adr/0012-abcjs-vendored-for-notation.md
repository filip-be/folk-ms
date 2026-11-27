# 0012. abcjs vendored for music notation and playback

- Status: accepted
- Date: 2026-11-27
- A narrow exception to [ADR 0003](0003-stdlib-only-go.md) (zero runtime npm dependencies); [ADR 0007](0007-latest-toolchains-and-dependabot-cooldown.md) does not apply to it

## Context

The Tune Collection stores Transcriptions in ABC notation. The admin's Tune editor previews them while the Moderator types, and the Sites show the sheet music and play it. Drawing staff notation from ABC (beams, ties, repeats, first and second endings, grace notes, line breaks) and turning it into timed notes is a large, well-solved problem. abcjs (MIT, by Paul Rosen and Gregory Dyke) does both and has no dependencies of its own. Writing our own would take far more volunteer time than reviewing a single file.

ADR 0003 allows no runtime npm package. Dependabot (ADR 0007) would track one through `package.json`, which is the channel ADR 0003 closes.

abcjs's synth plays per-note MP3 samples, which it loads by default from a third-party host. A Site must not depend on, or tell, a third party each time a visitor presses play.

## Decision

- **Vendored, not installed.** `vendor/abcjs/6.7.3/` holds the release's prebuilt, minified `dist/abcjs-basic-min.js`, unchanged, with its licence files and a `SHA256SUMS` (checked against the npm tarball's integrity when vendored). abcjs is not in `package.json` or `package-lock.json`, and Dependabot does not see it. `scripts/vendor.test.ts` (run in CI) checks the checksums and that no npm entry exists.
- **Typed by a small wrapper.** `vendor/abcjs/abcjs.ts` declares only the API we use and exports `loadAbcjs()`, which `import()`s the file. Bun bundles it into the admin and the Sites with code splitting, so abcjs is its own chunk (`chunks/` next to `app.js` / `main.js`) and only Tune screens and pages download it.
- **Upgrades are deliberate**, like the toolchains before ADR 0007: download the new release from the npm registry, check its integrity, replace the folder (the version is in the path), update `SHA256SUMS` and the wrapper's import, read the changelog, and check the Tune screens and pages.
- **Samples are self-hosted.** `vendor/soundfonts/FluidR3_GM/` holds the FluidR3 GM soundfont (Frank Wen, CC BY 3.0) as packaged by gleitz/midi-js-soundfonts at a fixed commit, for piano, flute, fiddle (`violin`) and accordion, notes G3 to D7 only (melody; chords are off), with `SHA256SUMS` and `CREDITS`. `scripts/fetch-samples.ts` downloads them; the files are committed. `sites/build.ts` copies all four into a Site with the Tune Collection, served at `/sounds/`; `cms/admin/build.ts` copies piano only, embedded in the binary and served at `/app/sounds/`. Wherever samples play, the page shows the CC BY credit.

## Alternatives

- **abcjs as an npm dependency:** updates through Dependabot with a cooldown, but a runtime dependency in the lockfile, which ADR 0003 excludes, and each release would ship without anyone reading it.
- **Our own ABC renderer:** no third-party code, but months of work for notation that abcjs already draws well.
- **Server-side rendering to SVG in Go:** no JavaScript library, but no live preview while typing and no playback.
- **Samples from the default third-party host:** no files in the repository, but every play reaches another origin, and a Site breaks when it goes away.

## Consequences

- About 0.5 MB of minified third-party JavaScript ships to visitors of Tune pages and to Moderators on Tune screens. Other pages and screens are unchanged.
- Security fixes in abcjs reach us only when someone upgrades it. abcjs draws SVG from ABC text and does not fetch or run code; the Transcriptions it draws are written by Moderators.
- abcjs inserts `<style>` elements and `style` attributes, so the admin and Site CSPs allow `style-src 'unsafe-inline'`; scripts stay `'self'`.
- The repository grows by about 4.7 MB of samples; the CMS binary by about 1.1 MB (piano) plus abcjs.
