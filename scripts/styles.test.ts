// Enforces the CSS rule in AGENTS.md: colour, spacing, type, radius, shadow
// and stacking values live in custom properties (tokens) only; rules use var().
import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const root = join(import.meta.dir, "..");
const sites = (JSON.parse(readFileSync(join(root, "config", "sites.json"), "utf8")) as { organizations: { site: { id: string } }[] })
  .organizations.map((o) => `sites/${o.site.id}/src`);
const dirs = ["cms/admin/src", ...sites];

// Properties whose values must come from tokens.
const TOKENIZED = /^(margin(-[a-z]+)*|padding(-[a-z]+)*|(row-|column-)?gap|font|font-size|font-family|border(-[a-z]+)*-radius|box-shadow|z-index)$/;
// What may remain once var(...), calc arithmetic and unitless numbers are removed.
const KEYWORDS = new Set(["0", "auto", "none", "inherit", "initial", "unset", "/"]);
const COLOR = /#[0-9a-f]{3,8}\b|\b(rgba?|hsla?|hwb|lab|lch|oklab|oklch)\(/i;

function violations(css: string): string[] {
  const found: string[] = [];
  // @font-face declares a face (its font-family is the name being defined), not a rule.
  const text = css.replace(/\/\*[\s\S]*?\*\//g, "").replace(/@font-face\s*\{[^}]*\}/g, "");
  for (const [, prop, raw] of text.matchAll(/([-\w]+)\s*:\s*([^;{}]+)(?=[;}])/g)) {
    const value = raw!.trim();
    if (prop!.startsWith("--")) continue; // a token definition
    if (COLOR.test(value)) found.push(`${prop}: ${value} (colour literal)`);
    if (!TOKENIZED.test(prop!)) continue;
    if (prop === "z-index" && !/^(var\(--[\w-]+\)|auto)$/.test(value)) found.push(`${prop}: ${value}`);
    const rest = value.replace(/var\(--[\w-]+\)/g, " ").replace(/calc\(|[()*+-]|\b\d+(\.\d+)?\b(?![%\w])/g, " ");
    if (rest.split(/\s+/).some((t) => t && !KEYWORDS.has(t))) found.push(`${prop}: ${value}`);
  }
  return found;
}

describe("stylesheets use tokens", () => {
  for (const dir of dirs) {
    for (const f of readdirSync(join(root, dir)).filter((n) => n.endsWith(".css"))) {
      test(`${dir}/${f}`, () => {
        expect(violations(readFileSync(join(root, dir, f), "utf8"))).toEqual([]);
      });
    }
  }
  test("the check catches raw values", () => {
    expect(violations(".a { color: #fff; margin: 0 1rem; z-index: 3; font: 14px sans-serif }").length).toBe(4);
    expect(violations(":root { --c: #fff; --s: 1rem } .a { color: var(--c); margin: 0 auto var(--s); font: inherit }")).toEqual([]);
    expect(violations('@font-face { font-family: "X"; src: url(x.woff2) } .a { font-family: "X" }').length).toBe(1);
  });
});
