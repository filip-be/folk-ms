// Builds the admin SPA into dist/ (embedded into the CMS binary by embed.go).
import { cpSync, copyFileSync, mkdirSync, readdirSync, rmSync } from "node:fs";
import { join } from "node:path";

const dir = import.meta.dir;
const out = join(dir, "dist");
mkdirSync(out, { recursive: true });
// Chunk names carry a content hash: drop the previous build's.
rmSync(join(out, "chunks"), { recursive: true, force: true });
const result = await Bun.build({
  entrypoints: [join(dir, "src", "main.ts")],
  outdir: out,
  // Splitting puts lazily imported code (abcjs, ADR 0012) in chunks/, loaded
  // only by the screens that import() it; index.html still loads app.js.
  splitting: true,
  naming: { entry: "app.js", chunk: "chunks/[name]-[hash].[ext]" },
  minify: true,
  target: "browser",
  sourcemap: "none",
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
// The Tune editor's preview plays piano only (ADR 0012): /app/sounds/, embedded
// in the binary with the rest of dist/.
rmSync(join(out, "sounds"), { recursive: true, force: true });
cpSync(join(dir, "..", "..", "vendor", "soundfonts", "FluidR3_GM", "acoustic_grand_piano-mp3"), join(out, "sounds", "acoustic_grand_piano-mp3"), { recursive: true });
console.log("admin built to", out);
