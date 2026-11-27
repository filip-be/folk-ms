// Builds one Site: bun sites/build.ts <site-id>
// SNAPSHOT_URL and VIDEOS_URL set where the Site reads its snapshots, events.json
// and videos.json (both default to the local CMS). Output: sites/<site-id>/dist,
// deployed to its Static Web App.
import { cpSync, copyFileSync, mkdirSync, readFileSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";

const site = process.argv[2];
if (!site || !/^[a-z0-9-]+$/.test(site)) {
  console.error("usage: bun sites/build.ts <site-id>");
  process.exit(2);
}
const dir = join(import.meta.dir, site);
const out = join(dir, "dist");
mkdirSync(out, { recursive: true });
// Chunk names carry a content hash: drop the previous build's.
rmSync(join(out, "chunks"), { recursive: true, force: true });
const snapshotUrl = process.env.SNAPSHOT_URL ?? `http://localhost:8080/pub/sites/${site}/events.json`;
const videosUrl = process.env.VIDEOS_URL ?? `http://localhost:8080/pub/sites/${site}/videos.json`;
const result = await Bun.build({
  entrypoints: [join(dir, "src", "main.ts")],
  outdir: out,
  // Splitting puts lazily imported code (abcjs, ADR 0012) in chunks/, loaded
  // only by the pages that import() it; index.html still loads main.js.
  splitting: true,
  naming: { entry: "main.js", chunk: "chunks/[name]-[hash].[ext]" },
  minify: true,
  target: "browser",
  define: { __SNAPSHOT_URL__: JSON.stringify(snapshotUrl), __VIDEOS_URL__: JSON.stringify(videosUrl) },
});
if (!result.success) {
  console.error(result.logs);
  process.exit(1);
}
for (const f of readdirSync(join(dir, "src"))) {
  if (f.endsWith(".css")) {
    // Bun inlines @imports and lowers CSS nesting.
    const css = await Bun.build({ entrypoints: [join(dir, "src", f)], outdir: out, minify: true });
    if (!css.success) {
      console.error(css.logs);
      process.exit(1);
    }
  } else if (!f.endsWith(".ts")) copyFileSync(join(dir, "src", f), join(out, f));
}
copyFileSync(join(dir, "staticwebapp.config.json"), join(out, "staticwebapp.config.json"));
// A Site with the Tune Collection ("tunes" in config/sites.json) serves the
// instrument samples from its own origin under /sounds/ (ADR 0012).
const config = JSON.parse(readFileSync(join(import.meta.dir, "..", "config", "sites.json"), "utf8")) as {
  organizations: { site: { id: string; tunes?: unknown } }[];
};
const usesTunes = config.organizations.some((o) => o.site.id === site && o.site.tunes);
rmSync(join(out, "sounds"), { recursive: true, force: true });
if (usesTunes) cpSync(join(import.meta.dir, "..", "vendor", "soundfonts", "FluidR3_GM"), join(out, "sounds"), { recursive: true, filter: (src) => !/(SHA256SUMS|CREDITS)$/.test(src) });
console.log(`built ${site} → ${out} (snapshots: ${snapshotUrl}, ${videosUrl})`);
