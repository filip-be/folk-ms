import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  abcKey,
  abcTempoLine,
  allNames,
  attachmentVariant,
  bi,
  compareTunes,
  EMBED_HOSTS,
  fetchTune,
  fetchTunes,
  filterTunes,
  findSet,
  findTune,
  groupTunes,
  hasLang,
  idFromSegment,
  instruments,
  keyLabel,
  matchesQuery,
  nameTranslation,
  OTHER_GROUP,
  parseTune,
  parseTunes,
  permalink,
  recordingPlayerUrl,
  recordingThumbnail,
  setPath,
  setTitle,
  setTunes,
  sortKey,
  sortTunes,
  tuneFileUrl,
  tuneOriginLabel,
  tuneOriginPath,
  tunePath,
  tunesUrl,
  tuneType,
  withTempo,
  type ImageAttachment,
  type TunesSnapshot,
} from "../src/tunes.ts";

const fixtures = join(import.meta.dir, "..", "..", "fixtures");
const raw = (dir: string, kind: string, name: string): unknown => JSON.parse(readFileSync(join(fixtures, dir, kind, name), "utf8"));
const load = (): TunesSnapshot => parseTunes(raw("tunes", "valid", "full.json"));
const loadTune = () => parseTune(raw("tune", "valid", "full.json"));
const ids = (ts: { id: string }[]) => ts.map((t) => t.id).join(",");

describe("tune fixtures", () => {
  for (const f of readdirSync(join(fixtures, "tunes", "valid"))) {
    test(`parses tunes/valid/${f}`, () => expect(() => parseTunes(raw("tunes", "valid", f))).not.toThrow());
  }
  for (const f of readdirSync(join(fixtures, "tune", "valid"))) {
    test(`parses tune/valid/${f}`, () => expect(() => parseTune(raw("tune", "valid", f))).not.toThrow());
  }
  test("refuses another schemaVersion", () => {
    expect(() => parseTunes(raw("tunes", "invalid", "wrong-schema-version.json"))).toThrow("unsupported schemaVersion 2; this reader reads version 1 only");
    expect(() => parseTune(raw("tune", "invalid", "wrong-schema-version.json"))).toThrow("unsupported schemaVersion 2");
  });
  test("refuses broken shapes", () => {
    expect(() => parseTunes(null)).toThrow("not an object");
    expect(() => parseTunes(raw("tunes", "invalid", "missing-sets.json"))).toThrow("missing organization");
    expect(() => parseTune(raw("tune", "invalid", "missing-recordings.json"))).toThrow("has no recordings");
    expect(() => parseTunes({ ...load(), tunes: [{ id: "x" }] })).toThrow("tune x has no name");
  });
  test("the catalogue agrees with the Tune's file", () => {
    const s = load();
    const f = loadTune();
    const c = s.tunes.find((t) => t.id === f.id)!;
    expect(c.firstRecording).toEqual(f.recordings[0]!);
    expect(c.incipit).toBe(f.incipit!);
    expect(c.hasTranscription).toBe(true);
    expect(f.sets.map((x) => x.id)).toEqual(s.sets.filter((x) => x.tuneIds.includes(f.id)).map((x) => x.id));
  });
});

describe("loading", () => {
  test("addresses under a snapshot base URL", () => {
    expect(tunesUrl("https://s.example/pub/sites/celtycki-wroclaw")).toBe("https://s.example/pub/sites/celtycki-wroclaw/tunes.json");
    expect(tuneFileUrl("https://s.example/pub/sites/x/", "tunbutfl")).toBe("https://s.example/pub/sites/x/tunes/tunbutfl.json");
    expect(tuneFileUrl("https://s.example/pub/sites/x/", "tunbutfl", "0123456789abcdef")).toBe("https://s.example/pub/sites/x/tunes/tunbutfl.json?rev=0123456789abcdef");
  });
  test("fetches the catalogue and one Tune", async () => {
    const realFetch = globalThis.fetch;
    const seen: string[] = [];
    globalThis.fetch = (async (url: string) => {
      seen.push(url);
      if (url.endsWith("/tunes.json")) return new Response(JSON.stringify(load()));
      if (url.includes("/tunes/tunbutfl.json")) return new Response(JSON.stringify(loadTune()));
      return new Response("", { status: 404 });
    }) as typeof fetch;
    try {
      const s = await fetchTunes("https://s.example/pub/sites/x");
      const t = await fetchTune("https://s.example/pub/sites/x", "tunbutfl", { rev: s.tunes[0]!.rev });
      expect(t.name.name).toBe("The Butterfly");
      expect(seen).toEqual(["https://s.example/pub/sites/x/tunes.json", "https://s.example/pub/sites/x/tunes/tunbutfl.json?rev=0123456789abcdef"]);
      const err = await fetchTune("https://s.example/pub/sites/x", "nopenope").then(() => undefined, (e: Error) => e.message);
      expect(err).toBe("tune: HTTP 404");
    } finally {
      globalThis.fetch = realFetch;
    }
  });
});

describe("languages", () => {
  test("either language may be missing; the other is shown", () => {
    expect(bi({ pl: "walc", en: "waltz" }, "en")).toBe("waltz");
    expect(bi({ en: "slip jig" }, "pl")).toBe("slip jig");
    expect(bi({ pl: "harfa" }, "en")).toBe("harfa");
    expect(bi(undefined, "pl")).toBe("");
    expect(hasLang({ en: "x" }, "pl")).toBe(false);
    expect(hasLang({ en: "x" }, "en")).toBe(true);
  });
  test("a name's translation is shown only in its own language", () => {
    const [butterfly] = load().tunes;
    expect(nameTranslation(butterfly!.name, "pl")).toBe("Motyl");
    expect(nameTranslation(butterfly!.name, "en")).toBe("");
    expect(nameTranslation(butterfly!.otherNames[0]!, "en")).toBe("The Butterfly");
    expect(nameTranslation({ name: "Polka", translation: { pl: "polka" } }, "pl")).toBe("");
  });
});

describe("lists", () => {
  const s = load();
  test("Tune Type, Instruments and Origin", () => {
    const [butterfly] = s.tunes;
    expect(bi(tuneType(s, butterfly!)!.label, "pl")).toBe("slip jig");
    expect(instruments(s, butterfly!.suits).map((i) => bi(i.label, "en"))).toEqual(["whistle", "flute"]);
    expect(tuneOriginLabel(s, "regkerry", "pl")).toBe("Kerry, Irlandia");
    expect(tuneOriginLabel(s, "countrie", "en")).toBe("Ireland");
    expect(tuneOriginPath(s, "nopenope")).toBeUndefined();
  });
  test("Keys", () => {
    expect(keyLabel({ tonic: "E", mode: "minor" }, "en")).toBe("E minor");
    expect(keyLabel({ tonic: "F#", mode: "minor" }, "en")).toBe("F♯ minor");
    expect(keyLabel({ tonic: "Bb", mode: "major" }, "en")).toBe("B♭ major");
    expect(keyLabel({ tonic: "E", mode: "minor" }, "pl")).toBe("e-moll");
    expect(keyLabel({ tonic: "G", mode: "major" }, "pl")).toBe("G-dur");
    expect(keyLabel({ tonic: "B", mode: "minor" }, "pl")).toBe("h-moll");
    expect(keyLabel({ tonic: "Bb", mode: "major" }, "pl")).toBe("B-dur");
    expect(keyLabel({ tonic: "F#", mode: "minor" }, "pl")).toBe("fis-moll");
    expect(keyLabel({ tonic: "Eb", mode: "major" }, "pl")).toBe("Es-dur");
    expect(keyLabel({ tonic: "A", mode: "dorian" }, "pl")).toBe("A dorycka");
    expect(abcKey({ tonic: "A", mode: "dorian" })).toBe("Ador");
    expect(abcKey({ tonic: "D", mode: "mixolydian" })).toBe("Dmix");
    expect(abcKey({ tonic: "E", mode: "minor" })).toBe("Em");
    expect(abcKey({ tonic: "G", mode: "major" })).toBe("G");
  });
});

describe("sorting", () => {
  test("ignores a leading article and diacritics", () => {
    expect(sortKey("The Butterfly")).toBe("butterfly");
    expect(sortKey("Na Ceannabháin Bhána")).toBe("ceannabhain bhana");
    expect(sortKey("An Féileacán")).toBe("feileacan");
    expect(sortKey("A Fig for a Kiss")).toBe("fig for a kiss");
    expect(sortKey("Ósemka")).toBe("osemka");
    expect(sortKey("The")).toBe("the");
    expect(sortKey("Annie's")).toBe("annie's");
  });
  test("the fixture's catalogue is in catalogue order", () => {
    const s = load();
    expect(ids(sortTunes([...s.tunes].reverse()))).toBe(ids(s.tunes));
    expect(ids(s.tunes)).toBe("tunbutfl,tunceann,tuncooly,tundrows,tunkessh,tunbrest,tunosemk,tunsibhe");
    const a = { ...s.tunes[0]!, id: "aaaaaaaa" };
    expect(compareTunes(a, s.tunes[0]!)).toBe(-1);
    expect(compareTunes(s.tunes[0]!, s.tunes[0]!)).toBe(0);
  });
});

describe("search and filters", () => {
  const s = load();
  const f = (x: Parameters<typeof filterTunes>[2]) => ids(filterTunes(s, s.tunes, x));

  test("search covers every name and translation, ignoring case and diacritics", () => {
    expect(allNames(s.tunes[0]!)).toEqual(["The Butterfly", "Motyl", "An Féileacán", "Motyl", "The Butterfly"]);
    expect(f({ query: "feileacan" })).toBe("tunbutfl");
    expect(f({ query: "MOTYL" })).toBe("tunbutfl");
    expect(f({ query: "white bog" })).toBe("tunceann");
    expect(f({ query: "ceannabháin" })).toBe("tunceann");
    expect(f({ query: "luttrell" })).toBe("tuncooly");
    expect(f({ query: "kesh mountain" })).toBe("tunkessh");
    expect(f({ query: "figure eight" })).toBe("tunosemk");
    expect(f({ query: "osemka" })).toBe("tunosemk");
    expect(f({ query: "  " })).toBe(ids(s.tunes));
    expect(matchesQuery(s.tunes[0]!, "butterfly motyl")).toBe(true);
    expect(matchesQuery(s.tunes[0]!, "butterfly reel")).toBe(false);
  });
  test("Tune Type: any of the chosen ones", () => {
    expect(f({ typeIds: ["typereel"] })).toBe("tuncooly,tundrows");
    expect(f({ typeIds: ["typewalc", "typejigg"] })).toBe("tunkessh,tunosemk,tunsibhe");
    expect(f({ typeIds: [] })).toBe(ids(s.tunes));
  });
  test("Key: tonic and/or mode", () => {
    expect(f({ tonic: "E" })).toBe("tunbutfl,tuncooly,tundrows");
    expect(f({ mode: "minor" })).toBe("tunbutfl,tuncooly,tunosemk");
    expect(f({ tonic: "E", mode: "dorian" })).toBe("tundrows");
    expect(f({ tonic: "F#" })).toBe("tunosemk");
  });
  test("Origin: a country includes its regions; a region only itself", () => {
    expect(f({ originId: "countrie" })).toBe("tunbutfl,tunceann,tuncooly,tundrows,tunkessh");
    expect(f({ originId: "regkerry" })).toBe("tunceann");
    expect(f({ originId: "countrfr" })).toBe("tunbrest");
    expect(f({ originId: "nopenope" })).toBe("");
  });
  test("Instrument: suits, or not tricky on", () => {
    expect(f({ instrument: { id: "instwhis", match: "suits" } })).toBe("tunbutfl,tunkessh,tunsibhe");
    expect(f({ instrument: { id: "instwhis", match: "notTricky" } })).toBe("tunbutfl,tunceann,tuncooly,tunkessh,tunbrest,tunosemk,tunsibhe");
    expect(f({ instrument: { id: "instharp", match: "notTricky" } })).not.toContain("tunbutfl");
  });
  test("filters combine", () => {
    expect(f({ typeIds: ["typereel"], originId: "countrie", tonic: "E", mode: "minor", query: "cooley" })).toBe("tuncooly");
    expect(f({ typeIds: ["typereel"], instrument: { id: "instwhis", match: "notTricky" } })).toBe("tuncooly");
  });
});

describe("grouping", () => {
  const s = load();
  const shape = (gs: ReturnType<typeof groupTunes>) =>
    gs.map((g) => `${g.id}:${ids(g.tunes)}${g.subgroups?.length ? `[${g.subgroups.map((x) => `${x.id}:${ids(x.tunes)}`).join(" ")}]` : ""}`);

  test("by Key: tonic C to B, then Mode", () => {
    const gs = groupTunes(s, s.tunes, "key");
    expect(shape(gs)).toEqual(["D-major:tunsibhe", "E-minor:tunbutfl,tuncooly", "E-dorian:tundrows", "F#-minor:tunosemk", "G-major:tunkessh", "A-mixolydian:tunceann", "Bb-major:tunbrest"]);
    expect(keyLabel(gs[1]!.key!, "en")).toBe("E minor");
  });
  test("by Tune Type: list order, unknown types in Other", () => {
    const tunes = [...s.tunes, { ...s.tunes[0]!, id: "tunorphn", typeId: "typegone" }];
    expect(shape(groupTunes(s, tunes, "type"))).toEqual([
      "typereel:tuncooly,tundrows",
      "typejigg:tunkessh",
      "typeslip:tunbutfl",
      "typepolk:tunceann",
      "typewalc:tunosemk,tunsibhe",
      "typemarc:tunbrest",
      `${OTHER_GROUP}:tunorphn`,
    ]);
  });
  test("by Origin: countries with region subgroups, no Origin in Other", () => {
    expect(shape(groupTunes(s, s.tunes, "origin"))).toEqual([
      "countrie:tunbutfl,tundrows,tunkessh[regclare:tuncooly regkerry:tunceann]",
      "countrfr:[regbreta:tunbrest]",
      "countrpl:tunosemk",
      `${OTHER_GROUP}:tunsibhe`,
    ]);
  });
  test("empty groups are left out", () => {
    expect(groupTunes(s, [], "type")).toEqual([]);
  });
});

describe("Sets", () => {
  const s = load();
  test("a Set's title: its name with fallback, else its Tunes' names", () => {
    const [reels, jigs, waltzes] = s.sets;
    expect(setTitle(s, reels!, "pl")).toBe("Cooley's / Drowsy Maggie");
    expect(setTitle(s, jigs!, "pl")).toBe("Dżigi na początek");
    expect(setTitle(s, jigs!, "en")).toBe("Jigs to start");
    expect(setTitle(s, waltzes!, "pl")).toBe("Waltzes");
    expect(ids(setTunes(s, waltzes!))).toBe("tunsibhe,tunosemk");
  });
  test("Permalinks", () => {
    expect(permalink(s.tunes[0]!, "en")).toBe("https://celtycki-wroclaw.example/en/tunes/tunbutfl-the-butterfly");
    expect(tunePath("pl", "tunbutfl", "the-butterfly")).toBe("/pl/tunes/tunbutfl-the-butterfly");
    expect(setPath("en", "setreelz", "cooley-s-drowsy-maggie")).toBe("/en/sets/setreelz-cooley-s-drowsy-maggie");
    for (const t of s.tunes) {
      for (const lang of ["pl", "en"] as const) expect(t.permalink[lang]).toBe(s.organization.baseUrl + tunePath(lang, t.id, t.slug));
    }
    for (const x of s.sets) expect(x.permalink.pl).toBe(s.organization.baseUrl + setPath("pl", x.id, x.slug));
    expect(idFromSegment("TUNBUTFL-old-slug")).toBe("tunbutfl");
    expect(findTune(s, "tunbutfl-renamed")?.name.name).toBe("The Butterfly");
    expect(findSet(s, "setjigsl")?.id).toBe("setjigsl");
    expect(findTune(s, "nopenope-x")).toBeUndefined();
  });
});

describe("ABC tempo", () => {
  test("the Q: line counts the Tune's beat", () => {
    expect(abcTempoLine(110, "3/8")).toBe("Q:3/8=110");
    expect(abcTempoLine(90.4, "1/2")).toBe("Q:1/2=90");
  });
  test("adds Q: before K:, replaces one in the header, leaves the body alone", () => {
    const t = loadTune();
    const out = withTempo(t.transcription!.text, t.tempo, t.tempoBeat);
    expect(out.split("\n").slice(0, 7)).toEqual(["X:1", "T:The Butterfly", "R:slip jig", "M:9/8", "L:1/8", "Q:3/8=110", "K:Em"]);
    expect(withTempo("X:1\nQ:1/4=60\nK:G\nGABc|\nQ:1/4=200\n", 180, "1/4")).toBe("X:1\nQ:1/4=180\nK:G\nGABc|\nQ:1/4=200\n");
    expect(withTempo("X:1\r\nT:x\r\nK:D\r\nDEF|", 120, "1/4")).toBe("X:1\nT:x\nQ:1/4=120\nK:D\nDEF|");
    expect(withTempo("X:1\nT:no key", 100, "1/4")).toBe("X:1\nQ:1/4=100\nT:no key");
    expect(withTempo("abc|", 100, "1/4")).toBe("Q:1/4=100\nabc|");
  });
});

describe("Recordings and Attachments", () => {
  const t = loadTune();
  test("players load only from the embed hosts", () => {
    expect(EMBED_HOSTS).toEqual(["www.youtube-nocookie.com", "player.vimeo.com"]);
    expect(recordingPlayerUrl(t.recordings[0]!)).toBe("https://www.youtube-nocookie.com/embed/aBcDeFgHiJk?start=42&autoplay=1");
    expect(recordingPlayerUrl(t.recordings[1]!)).toBe("https://player.vimeo.com/video/123456789?h=abcdef1234&autoplay=1");
    expect(recordingPlayerUrl(t.recordings[2]!)).toBeUndefined();
    const video = t.recordings[0]!;
    if (video.kind !== "video") throw new Error("fixture");
    expect(recordingPlayerUrl({ ...video, embedUrl: "https://evil.example/embed/x" })).toBeUndefined();
    expect(recordingPlayerUrl({ ...video, embedUrl: "not a url" })).toBeUndefined();
    expect(recordingThumbnail(video)).toBe("https://i.ytimg.com/vi/aBcDeFgHiJk/hqdefault.jpg");
    expect(recordingThumbnail({ ...video, thumbnail: "http://i.ytimg.com/x.jpg" })).toBeUndefined();
  });
  test("image variants", () => {
    const img = t.attachments[0] as ImageAttachment;
    expect(attachmentVariant(img, 600)?.width).toBe(960);
    expect(attachmentVariant(img, 4000)?.width).toBe(1600);
    expect(attachmentVariant({ ...img, variants: undefined }, 100)).toBeUndefined();
    expect(t.attachments[1]!.kind).toBe("pdf");
  });
});
