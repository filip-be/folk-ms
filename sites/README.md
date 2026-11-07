# sites

One static Site per Organization, deployed to its own Azure Static Web App. How Sites get their data is in [Site delivery](../docs/site-delivery/README.md).

| Path | What |
| --- | --- |
| `shared/router.ts` | Routes every Site supports; tested against `config/sites.json` |
| `shared/render.ts` | A plain starting renderer built on `contract/client` |
| `<site>/src/` | The Site's entry, HTML and styles; designers own these |
| `<site>/staticwebapp.config.json` | `navigationFallback` and headers |
| `build.ts`, `serve.ts` | `bun sites/build.ts <site>` (`SNAPSHOT_URL` sets the snapshot), `bun sites/serve.ts <site> <port>` |

A new Site needs an entry in `config/sites.json`, a folder here (copy an existing one), and a rerun of `deploy-infra`.
