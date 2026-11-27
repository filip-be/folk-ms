# sites

One static Site per Organization, deployed to its own Azure Static Web App. How Sites get their data is in [Site delivery](../docs/site-delivery/README.md).

| Path | What |
| --- | --- |
| `shared/router.ts` | Routes every Site supports; tested against `config/sites.json` |
| `shared/site.ts` | `run()`: a Site's entry passes the page modules and header links it wants |
| `shared/render.ts` | `events()`: the Events pages, a plain starting renderer built on `contract/client`; it escapes all snapshot text and links only `https` addresses (`render.test.ts`) |
| `shared/videos.ts` | `videos()`: the `/<lang>/videos` page of the [Video Collection](../docs/video-collection/README.md) (`videos.test.ts`); only Bal Folk Poznań uses it |
| `shared/tunes.ts` | `tunes()`: the [Tune Collection](../docs/tune-collection/README.md) pages (`tunes.test.ts`): `/<lang>/tunes` (search, Tune Type / Key / Origin / Instrument filters and Group by in the query string; each row's Incipit drawn when it scrolls into view, with a play button; ticked Tunes print together), `/<lang>/tunes/<id>-<slug>` (sheet, playback with sound and tempo, details, the CMS's description HTML, click-to-load Recordings, Attachments, Sets, Share, Print) and `/<lang>/sets/<id>-<slug>`. abcjs loads lazily from `chunks/`; samples are served from `/sounds/` with the FluidR3 credit. Only Celtycki Wrocław uses it |
| `swa-config.ts` | Fills `{{STORAGE_ORIGIN}}` in a Site's `staticwebapp.config.json` with the origin of `SNAPSHOT_URL` at build (the Wrocław CSP's `img-src`, `connect-src` and `frame-src` for PDFs); `tunes.test.ts` checks the built CSP |
| `<site>/src/` | The Site's entry, HTML and styles; designers own these |
| `<site>/staticwebapp.config.json` | `navigationFallback` and headers |
| `build.ts`, `serve.ts` | `bun sites/build.ts <site>` (`SNAPSHOT_URL` and `VIDEOS_URL` set the snapshots; `tunes.json` and `tunes/<id>.json` are read from `SNAPSHOT_URL`'s folder), `bun sites/serve.ts <site> <port>` (serves `dist/` with the config's `globalHeaders`, so the CSP applies locally) |

A new Site needs an entry in `config/sites.json`, a folder here (copy an existing one), and a rerun of `deploy-infra`.
