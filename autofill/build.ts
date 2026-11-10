// Builds the Autofill WebExtension for desktop Chrome/Edge and Firefox:
//   CMS_ORIGIN=https://cms.example bun autofill/build.ts
// Output: autofill/dist/extension-chrome and autofill/dist/extension-firefox,
// to zip and upload to the Chrome Web Store and addons.mozilla.org. The
// userscript is built with the admin SPA (cms/admin/build.ts).
// AUTOFILL_PRACTICE=1 also matches the CMS's dev-only practice page.
import { buildExtension } from "./lib.ts";

const origin = process.env.CMS_ORIGIN ?? "http://localhost:8080";
const u = new URL(origin);
if (u.origin !== origin) {
  console.error(`CMS_ORIGIN must be an origin like https://cms.example, got ${origin}`);
  process.exit(2);
}
const practice = process.env.AUTOFILL_PRACTICE === "1";
for (const b of ["chrome", "firefox"] as const) {
  console.log("built", await buildExtension(b, origin, practice));
}
