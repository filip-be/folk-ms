// Builds Autofill's artefacts from one codebase (see build.ts and
// cms/admin/build.ts):
// - a Manifest V3 WebExtension for desktop Chrome/Edge and Firefox, built
//   for one CMS origin;
// - the userscript the CMS serves at /app/autofill.user.js, with the CMS
//   origin filled in by the CMS when it serves it;
// - the harness of the dev-only practice page.
import { copyFileSync, mkdirSync, rmSync } from "node:fs";
import { join } from "node:path";

export const VERSION = "0.1.0";
/** Replaced by the CMS with its origin when it serves the userscript. */
export const ORIGIN_PLACEHOLDER = "__FOLKMS_CMS_ORIGIN__";

const dir = import.meta.dir;

async function bundle(entry: string, define: Record<string, string>): Promise<string> {
  const out = join(dir, "dist", ".tmp");
  mkdirSync(out, { recursive: true });
  const name = `${Math.random().toString(36).slice(2)}.js`;
  const r = await Bun.build({ entrypoints: [join(dir, "src", entry)], outdir: out, naming: name, minify: false, target: "browser", sourcemap: "none", define });
  if (!r.success) throw new Error("autofill build failed: " + JSON.stringify(r.logs));
  // Wrapped so nothing leaks into the page's or the sandbox's globals.
  const code = await Bun.file(join(out, name)).text();
  rmSync(join(out, name), { force: true });
  return `(() => {\n${code}\n})();\n`;
}

const defines = (channel: string, origin: string, practice: boolean) => ({
  __AF_CHANNEL__: JSON.stringify(channel),
  __AF_VERSION__: JSON.stringify(VERSION),
  __AF_CMS_ORIGIN__: JSON.stringify(origin),
  __AF_PRACTICE__: String(practice),
});

/** A match pattern for the CMS origin (ports are not part of match patterns). */
function originPattern(origin: string): string {
  const u = new URL(origin);
  return `${u.protocol}//${u.hostname}/app/*`;
}

export async function buildExtension(browserName: "chrome" | "firefox", cmsOrigin: string, practice = false): Promise<string> {
  const out = join(dir, "dist", `extension-${browserName}`);
  mkdirSync(out, { recursive: true });
  await Bun.write(join(out, "autofill.js"), await bundle("main.ts", defines("extension", cmsOrigin, practice)));
  const matches = [originPattern(cmsOrigin), "https://www.facebook.com/events/*", "https://m.facebook.com/events/*"];
  if (practice) matches.push(`${new URL(cmsOrigin).protocol}//${new URL(cmsOrigin).hostname}/dev/facebook/*`);
  const manifest: Record<string, unknown> = {
    manifest_version: 3,
    name: "folk-ms Autofill",
    version: VERSION,
    description: "Types a folk-ms Facebook Event Handoff into Facebook's create-event form. It never presses Create.",
    permissions: ["storage"],
    content_scripts: [{ matches, js: ["autofill.js"], run_at: "document_idle" }],
  };
  if (browserName === "firefox") {
    // Desktop only: no gecko_android (Firefox for Android uses the userscript).
    manifest.browser_specific_settings = {
      gecko: { id: "autofill@folk-ms", strict_min_version: "128.0", data_collection_permissions: { required: ["none"] } },
    };
  }
  await Bun.write(join(out, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  return out;
}

export async function buildUserscript(outFile: string): Promise<void> {
  const o = ORIGIN_PLACEHOLDER;
  const header = [
    "// ==UserScript==",
    "// @name         folk-ms Autofill",
    "// @namespace    https://github.com/filip-be/folk-ms",
    `// @version      ${VERSION}`,
    "// @description  Types a folk-ms Facebook Event Handoff into Facebook's create-event form. It never presses Create.",
    `// @match        ${o}/app/*`,
    "// @match        https://www.facebook.com/*",
    "// @match        https://m.facebook.com/*",
    "// @grant        GM.getValue",
    "// @grant        GM.setValue",
    "// @grant        GM.deleteValue",
    "// @grant        GM_getValue",
    "// @grant        GM_setValue",
    "// @grant        GM_deleteValue",
    "// @run-at       document-idle",
    "// @noframes",
    `// @downloadURL  ${o}/app/autofill.user.js`,
    `// @updateURL    ${o}/app/autofill.user.js`,
    "// ==/UserScript==",
    "",
  ].join("\n");
  await Bun.write(outFile, header + (await bundle("main.ts", defines("userscript", o, false))));
}

/** The practice page's harness: the fill engine without storage. */
export async function buildHarness(outFile: string): Promise<void> {
  await Bun.write(outFile, await bundle("harness.ts", {}));
}

export function copyFieldMap(to: string): void {
  copyFileSync(join(dir, "fieldmap.json"), to);
}
