import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { href, match } from "./router.ts";

// The Permalink pattern in config/sites.json and the Sites' routes must agree,
// or Social Posts link to pages that do not exist (docs/site-delivery,
// "Change together").
const config = JSON.parse(readFileSync(join(import.meta.dir, "..", "..", "config", "sites.json"), "utf8")) as {
  organizations: { id: string; site: { id: string; baseUrl: string; permalinkTemplate: string } }[];
};

describe("permalinks from config/sites.json resolve to the event route", () => {
  for (const org of config.organizations) {
    for (const lang of ["pl", "en"] as const) {
      test(`${org.site.id} ${lang}`, () => {
        const path = org.site.permalinkTemplate.replace("{lang}", lang).replace("{shortId}", "abcdefg2").replace("{slug}", "bal-jesienny");
        const r = match(path);
        expect(r).toEqual({ name: "event", lang, shortId: "abcdefg2", slug: "bal-jesienny" });
        expect(href(r)).toBe(path);
      });
    }
  }
});

describe("routes", () => {
  test("home and lists", () => {
    expect(match("/")).toEqual({ name: "home", lang: "pl" });
    expect(match("/en/")).toEqual({ name: "home", lang: "en" });
    expect(match("/pl/events")).toEqual({ name: "events", lang: "pl", past: false });
    expect(match("/en/events/past")).toEqual({ name: "events", lang: "en", past: true });
  });
  test("slug is optional and cosmetic", () => {
    expect(match("/pl/events/abcdefg2")).toEqual({ name: "event", lang: "pl", shortId: "abcdefg2", slug: "" });
    expect(match("/pl/events/abcdefg2-old-name")).toEqual({ name: "event", lang: "pl", shortId: "abcdefg2", slug: "old-name" });
  });
  test("unknown paths", () => {
    expect(match("/de/events").name).toBe("notFound");
    expect(match("/pl/events/NOTANID").name).toBe("notFound");
    expect(match("/pl/whatever").name).toBe("notFound");
  });
});
