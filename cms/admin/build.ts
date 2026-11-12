// Builds the admin SPA into dist/ (embedded into the CMS binary by embed.go),
// with Autofill's field map embedded and the Autofill userscript beside it
// (served at /app/autofill.user.js). Also refreshes the dev-only practice
// page's harness in cms/cmd/cms/practice.
import { copyFileSync, mkdirSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { buildHarness, buildUserscript, copyFieldMap } from "../../autofill/lib.ts";

const dir = import.meta.dir;
const out = join(dir, "dist");
mkdirSync(out, { recursive: true });
const fieldMap = await Bun.file(join(dir, "..", "..", "autofill", "fieldmap.json")).text();
const result = await Bun.build({
  entrypoints: [join(dir, "src", "main.ts")],
  outdir: out,
  naming: "app.js",
  minify: true,
  target: "browser",
  sourcemap: "none",
  define: { __AUTOFILL_FIELDMAP__: JSON.stringify(JSON.parse(fieldMap)) },
});
if (!result.success) {
  console.error(result.logs);
  process.exit(1);
}
copyFileSync(join(dir, "src", "index.html"), join(out, "index.html"));
// app.css @imports tokens.css: Bun inlines it and lowers CSS nesting.
const css = await Bun.build({ entrypoints: [join(dir, "src", "app.css")], outdir: out, minify: true, external: ["/app/fonts/*"] });
if (!css.success) {
  console.error(css.logs);
  process.exit(1);
}
// Fonts are served as files (tokens.css names them by /app/fonts/ URL): Bun would
// inline a relative url() as a data: URL, which the CSP's font-src blocks.
mkdirSync(join(out, "fonts"), { recursive: true });
for (const f of readdirSync(join(dir, "src", "fonts"))) copyFileSync(join(dir, "src", "fonts", f), join(out, "fonts", f));
await buildUserscript(join(out, "autofill.user.js"));
const practice = join(dir, "..", "cmd", "cms", "practice");
await buildHarness(join(practice, "harness.js"));
copyFieldMap(join(practice, "fieldmap.json"));
console.log("admin built to", out);
