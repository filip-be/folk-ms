import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { t } from "../src/index.ts";
import {
  filterVideos,
  fold,
  formatStart,
  groupVideos,
  originLabel,
  originPath,
  OTHER,
  parseVideos,
  playerUrl,
  usedForms,
  usedOrigins,
  youtubeThumbnail,
  type VideoSnapshot,
} from "../src/videos.ts";

const fixtures = join(import.meta.dir, "..", "..", "fixtures", "videos");
const raw = (kind: string, name: string): unknown => JSON.parse(readFileSync(join(fixtures, kind, name), "utf8"));
const load = (): VideoSnapshot => parseVideos(raw("valid", "full.json"));
const ids = (vs: { id: string }[]) => vs.map((v) => v.id).join(",");

describe("video fixtures", () => {
  for (const f of readdirSync(join(fixtures, "valid"))) {
    test(`parses valid/${f}`, () => {
      expect(() => parseVideos(raw("valid", f))).not.toThrow();
    });
  }
  test("refuses another schemaVersion", () => {
    expect(() => parseVideos(raw("invalid", "wrong-schema-version.json"))).toThrow("unsupported schemaVersion 2; this reader reads version 1 only");
  });
  test("refuses broken shapes", () => {
    expect(() => parseVideos(null)).toThrow("not an object");
    expect(() => parseVideos({ schemaVersion: 1, organization: {}, forms: [] })).toThrow("missing organization");
    expect(() => parseVideos(raw("invalid", "missing-form-ids.json"))).toThrow("has no formIds");
    expect(() => parseVideos(raw("invalid", "country-without-regions.json"))).toThrow("has no regions");
  });
});

describe("filters", () => {
  const s = load();

  test("Forms: any of the chosen ones", () => {
    expect(ids(filterVideos(s, { formIds: ["formlanc"] }))).toBe("vidandro,vidhanto,vidbourr");
    expect(ids(filterVideos(s, { formIds: ["formkolo", "formmixr"] }))).toBe("vidhanto,vidcircl");
    expect(filterVideos(s, { formIds: [] }).length).toBe(s.videos.length);
  });

  test("a country includes its regions; a region only itself", () => {
    expect(ids(filterVideos(s, { originId: "countrfr" }))).toBe("vidandro,vidhanto,vidmazur,vidschot,vidbourr");
    expect(ids(filterVideos(s, { originId: "regberry" }))).toBe("vidschot,vidbourr");
    expect(ids(filterVideos(s, { originId: "countrse" }))).toBe("vidpolsk");
    expect(filterVideos(s, { originId: "countrit" }).length).toBe(0);
    expect(filterVideos(s, { originId: "unknownx" }).length).toBe(0);
  });

  test("search ignores case and diacritics, in Polish and English titles", () => {
    expect(fold("Łańcuch ŻÓŁW")).toBe("lancuch zolw");
    expect(ids(filterVideos(s, { query: "WALCZYK" }))).toBe("vidwalcz");
    expect(ids(filterVideos(s, { query: "bourree" }))).toBe("vidbourr");
    expect(ids(filterVideos(s, { query: "waltz" }))).toBe("vidwalcz");
    expect(ids(filterVideos(s, { query: "an dro" }))).toBe("vidandro,vidhanto");
    expect(ids(filterVideos(s, { query: "  " }))).toBe(ids(s.videos));
  });

  test("filters combine", () => {
    expect(ids(filterVideos(s, { formIds: ["formpara"], originId: "countrfr", query: "szot" }))).toBe("vidschot");
  });
});

describe("groups", () => {
  const s = load();

  test("by Form: a Video under each of its Forms, empty Forms left out, no Form in Other", () => {
    const g = groupVideos(s, s.videos, "form");
    expect(g.map((x) => x.id).join(",")).toBe("formpara,formkolo,formlanc,formmixr");
    expect(ids(g.find((x) => x.id === "formkolo")!.videos)).toBe("vidhanto,vidcircl");
    const noForm = { ...s.videos[0]!, id: "vidnofrm", formIds: [] };
    const withOther = groupVideos(s, [...s.videos, noForm], "form");
    expect(withOther.at(-1)!.id).toBe(OTHER);
    expect(withOther.at(-1)!.label).toBeUndefined();
    expect(ids(withOther.at(-1)!.videos)).toBe("vidnofrm");
  });

  test("by Origin: countries in list order with region subgroups, no Origin in Other", () => {
    const g = groupVideos(s, s.videos, "origin");
    expect(g.map((x) => x.id).join(",")).toBe(`countrfr,countrse,${OTHER}`);
    const fr = g[0]!;
    expect(ids(fr.videos)).toBe("vidmazur");
    expect(fr.subgroups!.map((x) => x.id).join(",")).toBe("regbreta,regberry");
    expect(ids(fr.subgroups![1]!.videos)).toBe("vidschot,vidbourr");
    expect(ids(g.at(-1)!.videos)).toBe("vidcircl,vidwalcz");
  });

  test("grouping a filtered list keeps only its groups", () => {
    const g = groupVideos(s, filterVideos(s, { query: "polska" }), "origin");
    expect(g.map((x) => x.id).join(",")).toBe("countrse");
  });
});

describe("helpers", () => {
  const s = load();

  test("origins and labels with Translation fallback", () => {
    expect(originPath(s, "regberry")!.country.id).toBe("countrfr");
    expect(originLabel(s, "regbreta", "en")).toBe("Brittany, France");
    expect(originLabel(s, "regberry", "en")).toBe("Berry, France");
    expect(originLabel(s, undefined, "pl")).toBe("");
    expect(t(s.forms.find((f) => f.id === "formmixr")!.label, "en")).toBe("Mikser");
  });

  test("used Forms and Origins hide empty ones", () => {
    expect(usedForms(s).length).toBe(4);
    const used = usedOrigins(s);
    expect(used.map((c) => c.id).join(",")).toBe("countrfr,countrse");
    expect(used[0]!.regions.map((r) => r.id).join(",")).toBe("regbreta,regberry");
  });

  test("player address: https on an embed host only, with autoplay", () => {
    const yt = s.videos.find((v) => v.id === "vidhanto")!;
    expect(playerUrl(yt)).toBe("https://www.youtube-nocookie.com/embed/aBcDeFgHiJk?start=90&autoplay=1");
    const vm = s.videos.find((v) => v.id === "vidschot")!;
    expect(playerUrl(vm)).toBe("https://player.vimeo.com/video/123456789?h=abcdef1234&autoplay=1#t=75s");
    for (const bad of ["https://evil.example/embed/x", "http://www.youtube-nocookie.com/embed/x", "javascript:alert(1)", "not a url"]) {
      expect(playerUrl({ ...yt, embedUrl: bad })).toBeUndefined();
    }
  });

  test("start times and thumbnails", () => {
    expect(formatStart(undefined)).toBe("");
    expect(formatStart(75)).toBe("1:15");
    expect(formatStart(3723)).toBe("1:02:03");
    expect(youtubeThumbnail("dQw4w9WgXcQ")).toBe("https://i.ytimg.com/vi/dQw4w9WgXcQ/hqdefault.jpg");
  });
});
