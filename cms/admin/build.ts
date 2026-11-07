// Builds the admin SPA into dist/ (embedded into the CMS binary by embed.go).
import { copyFileSync, mkdirSync } from "node:fs";
import { join } from "node:path";

const dir = import.meta.dir;
const out = join(dir, "dist");
mkdirSync(out, { recursive: true });
const result = await Bun.build({
  entrypoints: [join(dir, "src", "main.ts")],
  outdir: out,
  naming: "app.js",
  minify: true,
  target: "browser",
  sourcemap: "none",
});
if (!result.success) {
  console.error(result.logs);
  process.exit(1);
}
copyFileSync(join(dir, "src", "index.html"), join(out, "index.html"));
copyFileSync(join(dir, "src", "app.css"), join(out, "app.css"));
console.log("admin built to", out);
