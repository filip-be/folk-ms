import { describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, readdirSync, readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { buildExtension, buildUserscript, ORIGIN_PLACEHOLDER } from "../lib.ts";

describe("builds", () => {
  test("the Firefox extension is desktop only, for one CMS origin", async () => {
    const out = await buildExtension("firefox", "https://cms.example");
    const m = JSON.parse(readFileSync(join(out, "manifest.json"), "utf8"));
    expect(m.manifest_version).toBe(3);
    expect(m.permissions).toEqual(["storage"]);
    expect(m.browser_specific_settings.gecko_android).toBeUndefined();
    expect(m.content_scripts[0].matches).toEqual(["https://cms.example/app/*", "https://www.facebook.com/events/*", "https://m.facebook.com/events/*"]);
    const js = readFileSync(join(out, "autofill.js"), "utf8");
    expect(js).toContain('"https://cms.example"');
    expect(js).not.toContain(ORIGIN_PLACEHOLDER);
  });

  test("the userscript leaves the origin for the CMS to fill in and grants only GM storage", async () => {
    const file = join(mkdtempSync(join(tmpdir(), "af-")), "autofill.user.js");
    await buildUserscript(file);
    const s = readFileSync(file, "utf8");
    expect(s).toContain(`// @match        ${ORIGIN_PLACEHOLDER}/app/*`);
    const grants = [...s.matchAll(/^\/\/ @grant\s+(\S+)/gm)].map((m) => m[1]);
    expect(grants.sort()).toEqual(["GM.deleteValue", "GM.getValue", "GM.setValue", "GM_deleteValue", "GM_getValue", "GM_setValue"]);
    expect(s).not.toMatch(/addValueChangeListener/);
  });

  test("builds leave no temporary files", () => {
    const tmp = join(import.meta.dir, "..", "dist", ".tmp");
    expect(existsSync(tmp) ? readdirSync(tmp) : []).toEqual([]);
  });
});
