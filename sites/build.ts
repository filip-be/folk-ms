// Builds one Site: bun sites/build.ts <site-id>
// SNAPSHOT_URL sets where the Site reads its snapshot (defaults to the local
// CMS). Output: sites/<site-id>/dist, deployed to its Static Web App.
import { copyFileSync, mkdirSync, readdirSync } from "node:fs";
import { join } from "node:path";

const site = process.argv[2];
if (!site || !/^[a-z0-9-]+$/.test(site)) {
  console.error("usage: bun sites/build.ts <site-id>");
  process.exit(2);
}
const dir = join(import.meta.dir, site);
const out = join(dir, "dist");
mkdirSync(out, { recursive: true });
const snapshotUrl = process.env.SNAPSHOT_URL ?? `http://localhost:8080/pub/sites/${site}/events.json`;
const result = await Bun.build({
  entrypoints: [join(dir, "src", "main.ts")],
  outdir: out,
  naming: "main.js",
  minify: true,
  target: "browser",
  define: { __SNAPSHOT_URL__: JSON.stringify(snapshotUrl) },
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
console.log(`built ${site} → ${out} (snapshot: ${snapshotUrl})`);
