# sites

One static Site per Organization, deployed to its own Azure Static Web App. How Sites get their data is in [Site delivery](../docs/site-delivery/README.md).

| Path | What |
| --- | --- |
| `shared/router.ts` | Routes every Site supports; tested against `config/sites.json` |
| `shared/site.ts` | `run()`: a Site's entry passes the page modules and header links it wants |
| `shared/render.ts` | `events()`: the Events pages, a plain starting renderer built on `contract/client`; it escapes all snapshot text and links only `https` addresses (`render.test.ts`) |
| `shared/videos.ts` | `videos()`: the `/<lang>/videos` page of the [Video Collection](../docs/video-collection/README.md) (`videos.test.ts`); only Bal Folk Poznań uses it |
| `<site>/src/` | The Site's entry, HTML and styles; designers own these |
| `<site>/staticwebapp.config.json` | `navigationFallback` and headers |
| `build.ts`, `serve.ts` | `bun sites/build.ts <site>` (`SNAPSHOT_URL` and `VIDEOS_URL` set the snapshots), `bun sites/serve.ts <site> <port>` |

A new Site needs an entry in `config/sites.json`, a folder here (copy an existing one), and a rerun of `deploy-infra`.
