import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { EMBED_HOSTS, parseTune, parseTunes, THUMBNAIL_HOSTS, type TunesSnapshot } from "../../contract/client/src/tunes.ts";
import { cspSources, storageOrigin, swaConfig } from "../swa-config.ts";
import { esc } from "./site.ts";
import {
  matchTunes,
  printFacts,
  readState,
  renderCatalogue,
  renderControls,
  renderPrintTune,
  renderResults,
  renderRow,
  renderSet,
  renderTune,
  sizeText,
  tempoText,
  tunes,
  writeState,
  type TuneState,
} from "./tunes.ts";

const root = join(import.meta.dir, "..", "..");
const fixtures = join(root, "contract", "fixtures");
const catalogueJson = readFileSync(join(fixtures, "tunes", "valid", "full.json"), "utf8");
const tuneJson = readFileSync(join(fixtures, "tune", "valid", "full.json"), "utf8");
const minimalJson = readFileSync(join(fixtures, "tune", "valid", "minimal.json"), "utf8");
const load = (): TunesSnapshot => parseTunes(JSON.parse(catalogueJson));
const butterfly = () => parseTune(JSON.parse(tuneJson));
const state = (q: string): TuneState => readState(load(), new URLSearchParams(q));
const names = (html: string) => [...html.matchAll(/<h[3-5]><a href="[^"]*">([^<]*)<\/a>/g)].map((m) => m[1]!.replaceAll("&#39;", "'"));

/** Replaces fetch for one test: answers by the path's last segment. */
async function withFetch<T>(fn: (urls: string[]) => Promise<T>): Promise<T> {
  const real = globalThis.fetch;
  const urls: string[] = [];
  const tuneFile = (id: string) => (id === "tunbutfl" ? tuneJson : minimalJson.replaceAll("tundrows", id));
  globalThis.fetch = (async (input: string) => {
    urls.push(String(input));
    const path = new URL(String(input)).pathname;
    const m = /tunes\/([a-z2-7]{8})\.json$/.exec(path);
    const body = path.endsWith("/tunes.json") ? catalogueJson : m ? tuneFile(m[1]!) : "{}";
    return new Response(body, { headers: { "Content-Type": "application/json" } });
  }) as unknown as typeof fetch;
  const realHistory = globalThis.history;
  (globalThis as { history?: unknown }).history = { replaceState: (_: unknown, __: string, url: string) => urls.push(`replace:${url}`) };
  try {
    return await fn(urls);
  } finally {
    globalThis.fetch = real;
    (globalThis as { history?: unknown }).history = realHistory;
  }
}

describe("routes", () => {
  test("catalogue, Tune and Set", () => {
    expect(matchTunes("/pl/tunes", "pl")).toEqual({ name: "tunes" });
    expect(matchTunes("/en/tunes/tunbutfl-the-butterfly", "en")).toEqual({ name: "tune", id: "tunbutfl", slug: "the-butterfly" });
    expect(matchTunes("/en/tunes/tunbutfl", "en")).toEqual({ name: "tune", id: "tunbutfl", slug: "" });
    expect(matchTunes("/pl/sets/setreelz-x", "pl")).toEqual({ name: "set", id: "setreelz", slug: "x" });
    for (const p of ["/pl/", "/pl/events", "/en/tunes", "/pl/tunes/NOTANID", "/pl/sets", "/pl/tunes/tunbutfl/x", "/pl/tunes/%E0%A4%A"]) {
      expect(matchTunes(p, "pl")).toBeUndefined();
    }
  });

  // The Permalink templates in config/sites.json and the Site's routes must agree.
  test("config/sites.json Permalink templates resolve", () => {
    const config = JSON.parse(readFileSync(join(root, "config", "sites.json"), "utf8")) as {
      organizations: { site: { id: string; tunes?: { tunePermalinkTemplate: string; setPermalinkTemplate: string } } }[];
    };
    const sites = config.organizations.filter((o) => o.site.tunes);
    expect(sites.length).toBeGreaterThan(0);
    for (const o of sites) {
      for (const lang of ["pl", "en"] as const) {
        const fill = (tpl: string) => tpl.replace("{lang}", lang).replace("{shortId}", "abcdefg2").replace("{slug}", "the-kesh");
        expect(matchTunes(fill(o.site.tunes!.tunePermalinkTemplate), lang)).toEqual({ name: "tune", id: "abcdefg2", slug: "the-kesh" });
        expect(matchTunes(fill(o.site.tunes!.setPermalinkTemplate), lang)).toEqual({ name: "set", id: "abcdefg2", slug: "the-kesh" });
      }
    }
  });
});

describe("query-string state", () => {
  test("round trip; unknown ids and values dropped", () => {
    const q = "q=kesh&type=typejigg,nosuchid,typejigg&tonic=F%23&mode=minor&origin=regkerry&instrument=instwhis&fit=nottricky&group=key";
    const st = state(q);
    expect(st).toEqual({ query: "kesh", typeIds: ["typejigg"], tonic: "F#", mode: "minor", originId: "regkerry", instrumentId: "instwhis", fit: "notTricky", group: "key" });
    expect(writeState(st)).toBe("?q=kesh&type=typejigg&tonic=F%23&mode=minor&origin=regkerry&instrument=instwhis&fit=nottricky&group=key");
    expect(state(writeState(st).slice(1))).toEqual(st);
    // A tonic no Tune has, an unknown Mode, Origin, Instrument or grouping; fit without an Instrument.
    expect(state("tonic=C&mode=ionian&origin=x&instrument=nosuch&fit=nottricky&group=evil")).toEqual({
      query: "", typeIds: [], tonic: undefined, mode: undefined, originId: undefined, instrumentId: undefined, fit: "suits", group: undefined,
    });
    expect(writeState(state(""))).toBe("");
  });
});

describe("catalogue", () => {
  const s = load();

  test("controls offer only what Tunes use, and reflect the state", () => {
    const html = renderControls(s, state("type=typereel&tonic=E&origin=regclare&instrument=instfidd"), "pl");
    expect(html).toContain(`data-type="typereel" aria-pressed="true">reel</button>`);
    expect(html).toContain(`data-type="typeslip" aria-pressed="false">slip jig</button>`); // English label as fallback
    expect(html).toContain(`<option value="F#">Fis</option>`);
    expect(html).toContain(`<option value="Bb">B</option>`);
    expect(html).toContain(`<option value="E" selected>E</option>`);
    expect(html).toContain(`<option value="mixolydian">miksolidyjska</option>`);
    expect(html).not.toContain(`value="lydian"`);
    expect(html).toContain(`<option value="regclare" selected>– Clare</option>`);
    expect(html).toContain(`<option value="countrpl">Polska</option>`);
    expect(html).toContain(`<select data-control="fit">`);
    expect(renderControls(s, state(""), "en")).toContain(`<select data-control="fit" disabled>`);
  });

  test("rows in catalogue order, filtered", () => {
    expect(names(renderResults(s, state(""), "en"))).toEqual(s.tunes.map((x) => x.name.name));
    expect(names(renderResults(s, state("mode=minor&type=typereel,typeslip"), "en"))).toEqual(["The Butterfly", "Cooley's"]);
    expect(names(renderResults(s, state("origin=countrie&instrument=instwhis&fit=nottricky"), "en"))).toEqual(["The Butterfly", "Na Ceannabháin Bhána", "Cooley's", "The Kesh"]);
    expect(names(renderResults(s, state("q=feileacan"), "pl"))).toEqual(["The Butterfly"]);
    expect(renderResults(s, state("q=nothing-like-this"), "en")).toContain("No tunes match the filters.");
  });

  test("groups by Key, Tune Type and Origin", () => {
    const heads = (q: string, lang: "pl" | "en") => [...renderResults(s, state(q), lang).matchAll(/class="tune-(?:sub)?group">\s*<h[34]>([^<]*)</g)].map((m) => m[1]).join("|");
    expect(heads("group=key", "pl")).toBe("D-dur|e-moll|E dorycka|fis-moll|G-dur|A miksolidyjska|B-dur");
    expect(heads("group=type", "en")).toBe("reel|jig|slip jig|polka|waltz|march");
    expect(heads("group=origin", "en")).toBe("Ireland|Clare|Kerry|France|Brittany|Poland|Other");
  });

  test("a row: tick box, link, details, Incipit and play; else the first Recording", () => {
    const row = (id: string, picked = false) => renderRow(s, s.tunes.find((x) => x.id === id)!, "en", 3, picked);
    const kesh = row("tunkessh", true);
    expect(kesh).toContain(`<input type="checkbox" class="pick" data-pick="tunkessh" aria-label="Select for printing: The Kesh" checked>`);
    expect(kesh).toContain(`<h3><a href="/en/tunes/tunkessh-the-kesh">The Kesh</a></h3>`);
    expect(kesh).toContain(`<p class="names">The Kesh Jig · Kesh Mountain</p>`);
    expect(kesh).toContain(`<p class="meta">jig · G major · 6/8 · Ireland</p>`);
    expect(kesh).toContain(`data-incipit="tunkessh"`);
    expect(kesh).toContain(`data-play-incipit="tunkessh"`);
    // No Transcription: a link Recording, shown with its host; a Tune with nothing has no action.
    expect(row("tundrows")).toContain(`<a class="listen" href="https://soundcloud.com/someone/drowsy-maggie" target="_blank" rel="noopener">Listen (soundcloud.com)</a>`);
    expect(row("tundrows")).not.toContain("data-incipit=");
    expect(row("tunbrest")).not.toContain(`class="play"`);
    const video = { ...s.tunes.find((x) => x.id === "tunbrest")!, firstRecording: s.tunes[0]!.firstRecording };
    expect(renderRow(s, video, "en", 3, false)).toContain(`data-play-recording="tunbrest"`);
    // Translations only in the page's language.
    expect(renderRow(s, s.tunes[0]!, "pl", 3, false)).toContain(`<p class="names">(Motyl) · An Féileacán</p>`);
    expect(renderRow(s, s.tunes[0]!, "en", 3, false)).toContain(`<p class="names">An Féileacán</p>`);
  });

  test("the page: count, print button, Sets and the samples' credit", () => {
    const html = renderCatalogue(s, state(""), "pl", new Set(["tunkessh", "tuncooly"]));
    expect(html).toContain(`<p class="count" role="status" data-count>8 melodii</p>`);
    expect(html).toContain(`data-control="print">Drukuj / zapisz jako PDF (2)</button>`);
    expect(html).toContain(`<a href="/pl/sets/setjigsl-dzigi-na-poczatek">Dżigi na początek</a>`);
    expect(html).toContain(`<a href="/pl/sets/setreelz-cooley-s-drowsy-maggie">Cooley&#39;s / Drowsy Maggie</a>`);
    expect(html).toContain("FluidR3 GM");
    expect(renderCatalogue(s, state(""), "en")).toContain(`data-control="print" disabled>Print / Save as PDF</button>`);
    const empty = parseTunes(JSON.parse(readFileSync(join(fixtures, "tunes", "valid", "empty.json"), "utf8")));
    const none = renderCatalogue(empty, readState(empty, new URLSearchParams()), "en");
    expect(none).toContain("Tunes are coming soon.");
    expect(none).not.toContain("<form");
    expect(none).not.toContain("FluidR3");
  });
});

describe("a Tune's page", () => {
  const s = load();

  test("details, sheet and playback, description, Recordings, Attachments, Sets", () => {
    const html = renderTune(s, butterfly(), "en");
    expect(html).toContain(`<h2>The Butterfly</h2>`); // the Polish translation is not shown in English
    expect(html).toContain(`<li>An Féileacán <span class="translation">(The Butterfly)</span></li>`);
    for (const [k, v] of [["Tune type", "slip jig"], ["Key", "E minor"], ["Time signature", "9/8"], ["Playback tempo", "♩. = 110"], ["Origin", "Ireland"], ["Composer", "Tommy Potts"], ["Suits", "whistle, flute"], ["Tricky on", "harfa"]]) {
      expect(html).toContain(`<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`);
    }
    expect(html).toContain(`data-sheet="tunbutfl"`);
    expect(html).toContain(`data-play-tune="tunbutfl"`);
    expect(html).toContain(`<select data-control="sound"><option value="piano">piano</option><option value="flute">flute</option><option value="fiddle" selected>fiddle</option><option value="accordion">accordion</option></select>`);
    expect(html).toContain(`data-tempo="-10"`);
    expect(html).toContain("FluidR3 GM");
    // The CMS's HTML, as it is.
    expect(html).toContain(`<div class="description"><p>A slip jig by <em>Tommy Potts</em>; see <a href="https://thesession.org/tunes/10" rel="nofollow noopener ugc">The Session</a>.</p>`);
    // Recordings: click-to-load players, and a link showing its host.
    expect(html).toContain(`data-recording="0" aria-label="Play: Kevin Burke"><img src="https://i.ytimg.com/vi/aBcDeFgHiJk/hqdefault.jpg"`);
    expect(html).toContain(`<a href="https://vimeo.com/123456789/abcdef1234" target="_blank" rel="noopener">Watch on Vimeo</a>`);
    expect(html).toContain(`<a href="https://example.bandcamp.com/track/the-butterfly" target="_blank" rel="noopener">Sesja w Pubie</a> <span class="host">(example.bandcamp.com)</span>`);
    expect(html).not.toContain("<iframe");
    // Attachments: an image with srcset and its caption as alt; a PDF frame and its card.
    expect(html).toContain(`src="https://storage.example/pub/media/celtycki-wroclaw/tunes/tunbutfl/a1b2c3-960.jpg" srcset="https://storage.example/pub/media/celtycki-wroclaw/tunes/tunbutfl/a1b2c3-480.jpg 480w, `);
    expect(html).toContain(`alt="Skan starego zapisu"`);
    expect(html).toContain(`data-pdf="https://storage.example/pub/media/celtycki-wroclaw/tunes/tunbutfl/d4e5f6.pdf" data-pdf-title="Typeset score"`);
    expect(html).toContain(`<span class="pdf-open">Open PDF</span> <span class="pdf-name">The Butterfly.pdf</span> <span class="pdf-size">118 KB</span>`);
    expect(html).toContain(`<a href="/en/sets/setjigsl-dzigi-na-poczatek">Jigs to start</a>`);
    expect(html).toContain(`data-control="share"`);
    expect(renderTune(s, butterfly(), "pl")).toContain(`<h2>The Butterfly <span class="translation">(Motyl)</span></h2>`);
  });

  test("without a Transcription: no sheet, player or credit", () => {
    const html = renderTune(s, parseTune(JSON.parse(minimalJson)), "pl");
    expect(html).toContain("Ta melodia nie ma jeszcze zapisu nutowego.");
    expect(html).not.toContain("data-sheet");
    expect(html).not.toContain("FluidR3");
    expect(html).not.toContain("Tempo odtwarzania");
  });

  test("the print layout: name and key facts, without Recordings and Attachments", () => {
    const html = renderPrintTune(s, butterfly(), true, "en");
    expect(html).toContain("<h2>The Butterfly</h2>");
    expect(printFacts(s, butterfly(), "en")).toBe("Tune type: slip jig · Key: E minor · Time signature: 9/8 · Composer: Tommy Potts · Origin: Ireland");
    expect(html).toContain(`data-print-sheet="tunbutfl"`);
    expect(html).not.toContain("youtube");
    expect(html).not.toContain("storage.example");
  });

  test("untrusted text is escaped; links only https, media only http(s)", () => {
    const payload = `"><img src=x onerror=alert(1)>`;
    const tune = butterfly();
    tune.name = { name: payload, translation: { pl: payload, en: payload } };
    tune.otherNames = [{ name: payload }];
    tune.composer = payload;
    tune.recordings = [
      { kind: "link", url: "javascript:alert(1)", host: payload },
      { kind: "video", url: "javascript:alert(1)", provider: "youtube", embedUrl: "https://evil.example/x", thumbnail: "https://evil.example/t.jpg", label: payload },
    ];
    tune.attachments = [{ id: "a", kind: "pdf", name: payload, size: 1, caption: { pl: payload }, url: "javascript:alert(1)" }];
    const html = renderTune(s, tune, "pl") + renderPrintTune(s, tune, true, "pl");
    expect(html).not.toContain("<img src=x");
    expect(html).not.toContain("javascript:");
    expect(html).not.toContain("evil.example");
    expect(html).toContain(esc(payload));
  });
});

describe("a Set's page", () => {
  test("its Tunes in order, with sheets where they have Transcriptions", () => {
    const s = load();
    const set = s.sets.find((x) => x.id === "setreelz")!;
    const cooley = { ...parseTune(JSON.parse(minimalJson)), id: "tuncooly", transcription: { format: "abc" as const, text: "X:1\nK:Em\nEBBA|" } };
    const html = renderSet(s, set, [cooley, parseTune(JSON.parse(minimalJson))], "en");
    expect(html).toContain("<h2>Cooley&#39;s / Drowsy Maggie</h2>");
    expect(names(html)).toEqual(["Cooley's", "Drowsy Maggie"]);
    expect(html).toContain(`data-sheet="tuncooly"`);
    expect(html).toContain(`data-play-tune="tuncooly"`);
    expect(html).toContain("This tune has no transcription yet.");
    expect(html).toContain("(soundcloud.com)");
    expect(html).toContain(`data-control="print"`);
  });
});

describe("the module", () => {
  const base = "https://store.example/pub/sites/celtycki-wroclaw/";

  test("owns the Tune Collection paths only; alternates keep the state", async () => {
    await withFetch(async (urls) => {
      const m = tunes(base);
      expect(await m.render(new URL("https://site.example/pl/events"), "pl")).toBeUndefined();
      expect(await m.render(new URL("https://site.example/pl/"), "pl")).toBeUndefined();
      expect(urls).toEqual([]); // nothing fetched for other pages
      const page = (await m.render(new URL("https://site.example/pl/tunes?group=type&type=typereel"), "pl"))!;
      expect(page.title).toBe("Melodie – Celtycki Wrocław");
      expect(page.alternate).toBe("/en/tunes?type=typereel&group=type");
      expect(urls).toEqual([`${base}tunes.json`]);
    });
  });

  test("a Tune's page reads its file with the rev and fixes a wrong slug", async () => {
    await withFetch(async (urls) => {
      const m = tunes(base);
      const page = (await m.render(new URL("https://site.example/en/tunes/tunbutfl-old-name"), "en"))!;
      expect(page.title).toBe("The Butterfly – Celtycki Wrocław");
      expect(page.alternate).toBe("/pl/tunes/tunbutfl-the-butterfly");
      expect(urls).toContain(`${base}tunes/tunbutfl.json?rev=0123456789abcdef`);
      expect(urls).toContain("replace:/en/tunes/tunbutfl-the-butterfly");
      const missing = (await m.render(new URL("https://site.example/en/tunes/zzzzzzzz"), "en"))!;
      expect(missing.body).toContain("Tune not found.");
    });
  });

  test("a Set's page reads its Tunes' files and fixes a wrong slug", async () => {
    await withFetch(async (urls) => {
      const m = tunes(base);
      const page = (await m.render(new URL("https://site.example/pl/sets/setjigsl"), "pl"))!;
      expect(page.title).toBe("Dżigi na początek – Celtycki Wrocław");
      expect(urls).toContain(`${base}tunes/tunkessh.json?rev=9f8e7d6c5b4a3921`);
      expect(urls).toContain("replace:/pl/sets/setjigsl-dzigi-na-poczatek");
      expect(names(page.body)).toEqual(["The Kesh", "The Butterfly"]);
    });
  });
});

describe("formatting", () => {
  test("tempo and sizes", () => {
    expect(tempoText(99.6, "1/4")).toBe("♩ = 100");
    expect(tempoText(80, "1/16")).toBe("1/16 = 80");
    expect(sizeText(120334, "en")).toBe("118 KB");
    expect(sizeText(1_300_000, "pl")).toBe("1,2 MB");
  });
});

// The Wrocław Site loads players from exactly the embed hosts, PDFs from the
// public storage origin (filled in at build from SNAPSHOT_URL), thumbnails
// from the thumbnail hosts, and scripts only from itself (docs/tune-collection,
// Change together).
describe("Celtycki Wrocław's built CSP", () => {
  const text = readFileSync(join(root, "sites", "celtycki-wroclaw", "staticwebapp.config.json"), "utf8");
  for (const snapshotUrl of ["https://folkpub.blob.core.windows.net/pub/sites/celtycki-wroclaw/events.json", "http://localhost:8080/pub/sites/celtycki-wroclaw/events.json"]) {
    test(snapshotUrl, () => {
      const storage = storageOrigin(snapshotUrl);
      const cfg = JSON.parse(swaConfig(text, snapshotUrl)) as { globalHeaders: Record<string, string>; navigationFallback: { exclude: string[] } };
      const csp = cfg.globalHeaders["Content-Security-Policy"]!;
      expect(csp).not.toContain("{{");
      expect(cspSources(csp, "frame-src")).toEqual([...EMBED_HOSTS.map((h) => `https://${h}`), storage]);
      expect(cspSources(csp, "img-src")).toEqual(["'self'", "data:", ...THUMBNAIL_HOSTS.map((h) => `https://${h}`), storage]);
      expect(cspSources(csp, "connect-src")).toEqual(["'self'", storage]);
      expect(cspSources(csp, "script-src")).toEqual(["'self'"]);
      expect(cspSources(csp, "style-src")).toEqual(["'self'", "'unsafe-inline'"]);
      expect(cfg.navigationFallback.exclude.filter((p) => p === "/chunks/*" || p === "/sounds/*").length).toBe(2);
    });
  }
});
