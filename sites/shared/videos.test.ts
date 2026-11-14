import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parseSnapshot } from "../../contract/client/src/index.ts";
import { parseVideos, type VideoSnapshot } from "../../contract/client/src/videos.ts";
import { events, renderList } from "./render.ts";
import { esc, header, type Page } from "./site.ts";
import { readState, renderControls, renderResults, renderVideo, renderVideosPage, videos, writeState, type VideoState } from "./videos.ts";

const contract = join(import.meta.dir, "..", "..", "contract", "fixtures");
const fullVideos = readFileSync(join(contract, "videos", "valid", "full.json"), "utf8");
const fullEvents = readFileSync(join(contract, "valid", "full.json"), "utf8");
const load = (): VideoSnapshot => parseVideos(JSON.parse(fullVideos));
const state = (q: string): VideoState => readState(load(), new URLSearchParams(q));
const cards = (html: string) => [...html.matchAll(/<h[2-6]>([^<]*)<\/h[2-6]>\s*(?:<p class="meta">|<\/article>|\s)/g)].length;

/** Replaces fetch for one test: every request answers body. */
async function withFetch<T>(body: string, fn: () => Promise<T>): Promise<T> {
  const real = globalThis.fetch;
  globalThis.fetch = (async () => new Response(body, { headers: { "Content-Type": "application/json" } })) as unknown as typeof fetch;
  try {
    return await fn();
  } finally {
    globalThis.fetch = real;
  }
}

describe("query-string state", () => {
  test("round trip; unknown ids and values dropped", () => {
    const st = state("form=formpara,nosuchid,formpara&origin=regberry&q=walc&group=origin");
    expect(st).toEqual({ formIds: ["formpara"], originId: "regberry", query: "walc", group: "origin" });
    expect(writeState(st)).toBe("?form=formpara&origin=regberry&q=walc&group=origin");
    expect(state("origin=nosuchid&group=evil")).toEqual({ formIds: [], originId: undefined, query: "", group: undefined });
    expect(writeState(state(""))).toBe("");
  });
});

describe("videos page", () => {
  const s = load();

  test("controls reflect the state; only used Forms and Origins are offered", () => {
    const html = renderControls(s, state("form=formkolo&origin=regbreta&group=form"), "pl");
    expect(html).toContain(`data-form="formkolo" aria-pressed="true">Koło</button>`);
    expect(html).toContain(`data-form="formpara" aria-pressed="false">Para</button>`);
    expect(html).not.toContain("Włochy"); // a country no Video comes from
    expect(html).not.toContain("Polska</option>");
    expect(html).toContain(`<option value="countrfr" selected>Francja</option>`);
    expect(html).toContain(`<option value="regbreta" selected>Bretania</option>`);
    expect(html).not.toContain("Gaskonia"); // an unused region
    expect(html).toContain(`<option value="form" selected>według formy</option>`);
    // A country without regions offers no region select.
    expect(renderControls(s, state("origin=countrse"), "en")).not.toContain(`data-control="region"`);
  });

  test("list in the Moderators' order, filtered", () => {
    expect(cards(renderResults(s, state(""), "pl"))).toBe(8);
    const html = renderResults(s, state("origin=countrfr&form=formpara"), "en");
    expect(cards(html)).toBe(2);
    expect(html.indexOf("Mazurka – figures")).toBeGreaterThan(-1);
    expect(html.indexOf("Schottische")).toBeGreaterThan(html.indexOf("Mazurka – figures"));
    expect(renderResults(s, state("q=nothing-like-this"), "en")).toContain("No videos match the filters.");
  });

  test("groups by Origin with region headings and Other last", () => {
    const html = renderResults(s, state("group=origin"), "en");
    const heads = [...html.matchAll(/class="video-(?:sub)?group">\s*<h[34]>([^<]*)</g)].map((m) => m[1]).join("|");
    expect(heads).toBe("France|Brittany|Berry|Sweden|Other");
  });

  test("groups by Form: a Video under each of its Forms", () => {
    const html = renderResults(s, state("group=form"), "pl");
    expect(html.match(/<h4>Hanter dro<\/h4>/g)?.length).toBe(2);
    expect(html).not.toContain("<h3>Inne</h3>");
  });

  test("a card: thumbnail as the play button, details, a link to watch", () => {
    const v = s.videos.find((x) => x.id === "vidschot")!;
    const html = renderVideo(s, v, "pl", 3);
    expect(html).toContain(`<button type="button" class="play" data-play="vidschot" aria-label="Odtwórz: Szot (Schottische)"><img src="https://i.vimeocdn.com/video/987654321-d_640" alt=""`);
    expect(html).toContain(`<p class="meta">Berry, Francja · Para · od 1:15</p>`);
    expect(html).toContain(`<a href="https://vimeo.com/123456789/abcdef1234#t=75s" target="_blank" rel="noopener">Obejrzyj na Vimeo</a>`);
    expect(html).not.toContain("<iframe");
    // No player for an embed address on another host, and no link for a non-https one.
    const bad = renderVideo(s, { ...v, embedUrl: "https://evil.example/x", url: "javascript:alert(1)" }, "pl", 3);
    expect(bad).not.toContain("data-play");
    expect(bad).not.toContain("javascript:");
  });

  test("an empty collection says so, without controls", () => {
    const empty = parseVideos(JSON.parse(readFileSync(join(contract, "videos", "valid", "empty.json"), "utf8")));
    const html = renderVideosPage(empty, readState(empty, new URLSearchParams()), "en");
    expect(html).toContain("Videos are coming soon.");
    expect(html).not.toContain("<form");
  });

  test("untrusted text is escaped everywhere", () => {
    const payload = `"><img src=x onerror=alert(1)>`;
    const h = load();
    h.forms[0]!.label = { pl: payload };
    h.origins[0]!.label = { pl: payload };
    h.origins[0]!.regions[0]!.label = { pl: payload };
    for (const v of h.videos) {
      v.title = { pl: payload, en: payload };
      v.description = { pl: payload };
      v.credit = payload;
      v.thumbnail = `https://i.ytimg.com/x.jpg"><script>alert(1)</script>`;
    }
    for (const q of ["", "group=form", "group=origin", `q=${encodeURIComponent(payload)}`]) {
      const html = renderVideosPage(h, readState(h, new URLSearchParams(q)), "pl", { pl: payload, en: payload });
      expect(html).not.toContain("<img src=x");
      expect(html).not.toContain("<script>");
      expect(html).toContain(esc(payload));
    }
  });

  test("the module owns /<lang>/videos only; the alternate keeps the state", async () => {
    const m = videos("https://store.example/videos.json", { intro: { pl: "Wstęp.\n\nDrugi akapit.", en: "Intro." } });
    await withFetch(fullVideos, async () => {
      expect(await m.render(new URL("https://site.example/pl/events"), "pl")).toBeUndefined();
      const page = (await m.render(new URL("https://site.example/pl/videos?group=form&form=formpara"), "pl"))!;
      expect(page.alternate).toBe("/en/videos?form=formpara&group=form");
      expect(page.title).toBe("Wideopomoc – Bal Folk Poznań");
      expect(page.body).toContain("<p>Wstęp.</p><p>Drugi akapit.</p>");
      expect(page.body).toContain(`<p class="count" role="status" data-count>4 filmy</p>`);
    });
  });
});

describe("Sites compose modules", () => {
  const page: Page = { siteName: "Celtycki Wrocław", body: "", alternate: "/en/" };

  test("a header without navigation is the header Sites had before (Celtycki)", () => {
    expect(header(page, "pl")).toBe(`<header><h1><a href="/pl/">Celtycki Wrocław</a></h1>
      <a class="lang" href="/en/" hreflang="en">English</a></header>`);
  });

  test("a header with navigation marks the current page (Poznań)", () => {
    const html = header(page, "en", [
      { href: (l) => `/${l}/`, label: { pl: "Wydarzenia", en: "Events" } },
      { href: (l) => `/${l}/videos`, label: { pl: "Wideopomoc", en: "Video help" } },
    ], "/en/videos");
    expect(html).toContain(`<li><a href="/en/">Events</a></li><li><a href="/en/videos" aria-current="page">Video help</a></li>`);
  });

  test("events() renders what the Events pages rendered, and leaves other paths", async () => {
    const m = events("https://store.example/events.json");
    await withFetch(fullEvents, async () => {
      const now = new Date();
      const list = (await m.render(new URL("https://site.example/en/events/past"), "en"))!;
      expect(list.body).toBe(renderList(parseSnapshot(JSON.parse(fullEvents)), "en", true, now));
      expect(list.alternate).toBe("/pl/events/past");
      expect(list.siteName).toBe("Bal Folk Poznań");
      expect(await m.render(new URL("https://site.example/pl/videos"), "pl")).toBeUndefined();
      expect((await m.notFound!("pl")).body).toBe("<p>Nie znaleziono strony.</p>");
    });
  });
});
