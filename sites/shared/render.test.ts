import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { parseSnapshot, type Event, type Snapshot } from "../../contract/client/src/index.ts";
import { esc, renderCard, renderEvent, renderList } from "./render.ts";

const full = readFileSync(join(import.meta.dir, "..", "..", "contract", "fixtures", "valid", "full.json"), "utf8");
const load = (): Snapshot => parseSnapshot(JSON.parse(full));
const now = new Date("2026-11-07T12:00:00Z");
const bySlug = (s: Snapshot, slug: string): Event => s.events.find((e) => e.slug === slug)!;

describe("Permalink page", () => {
  const s = load();

  test("a repeating Event lists every Occurrence, each with its Facebook Event", () => {
    const html = renderEvent(s, bySlug(s, "jam-czwartkowy"), "pl", now);
    expect(html.match(/<li/g)?.length).toBe(4);
    for (const o of bySlug(s, "jam-czwartkowy").occurrences) expect(html).toContain(`href="${o.facebookEventUrl}"`);
    expect(html.match(/Wydarzenie na Facebooku/g)?.length).toBe(4);
    expect(html.match(/<li class="cancelled past">/g)?.length).toBe(1);
    expect(html).toContain(`<strong class="status">odwołane</strong>`);
  });

  test("location with address and an https map link", () => {
    const html = renderEvent(s, bySlug(s, "bal-listopadowy"), "en", now);
    expect(html).toContain("Dom Kultury Zamek, ul. Św. Marcin 80/82, Poznań");
    expect(html).toContain(`<a href="https://maps.example/zamek" rel="noopener">Map</a>`);
    expect(html).toContain(`<a href="https://www.facebook.com/events/1004759319341552/" rel="noopener">Facebook event</a>`);
    expect(html).toContain(`<p class="price">40 PLN, reduced 30 PLN · <a href="https://tickets.example/bal" rel="noopener">Tickets / registration</a></p>`);
    expect(html).toContain("Join us for a bal with live music.<br>Workshops from 6 pm.");
  });

  test("an online Occurrence without a location", () => {
    const html = renderEvent(s, bySlug(s, "warsztaty-online"), "pl", now);
    expect(html).toContain(`<span class="online">online</span>`);
    expect(html).not.toContain(`class="where"`);
    expect(html).not.toContain(`class="price"`);
  });

  test("price as a tickets link only; English falls back to Polish", () => {
    const html = renderEvent(s, bySlug(s, "festiwal-zimowy"), "en", now);
    expect(html).toContain(`<p class="price"><a href="https://tickets.example/festiwal" rel="noopener">Tickets / registration</a></p>`);
    expect(html).toContain("<h2>Festiwal zimowy</h2>");
    expect(html).toContain("Trzy dni tańca.");
    // Multi-day: a date range, no time.
    expect(html).toMatch(/<span class="when">15[^<]*17 January 2027<\/span>/);
  });
});

describe("lists", () => {
  const s = load();

  test("a cancelled Event stays in the upcoming list, marked cancelled", () => {
    const html = renderList(s, "en", false, now);
    expect(html.match(/<article/g)?.length).toBe(5);
    expect(html).toContain(`<article class="event-card cancelled">`);
    const card = renderCard(s, bySlug(s, "koncert-adwentowy"), "en", now);
    expect(card).toContain(`<strong class="status">cancelled</strong>`);
  });

  test("card shows the next live Occurrence of a repeating Event, and online", () => {
    expect(renderCard(s, bySlug(s, "jam-czwartkowy"), "en", now)).toContain("26 November 2026");
    expect(renderCard(s, bySlug(s, "warsztaty-online"), "en", now)).toContain(`<p class="where">online</p>`);
  });
});

describe("untrusted text is rendered harmless", () => {
  const HOSTILE = [`<script>alert(1)</script>`, `"><img src=x onerror=alert(1)>`, `'><svg onload=alert(1)>`];

  function hostile(payload: string): Snapshot {
    const s = load();
    const e = bySlug(s, "bal-listopadowy");
    e.title = { pl: payload, en: payload };
    e.summary = { pl: payload };
    e.description = { pl: payload, en: payload };
    e.price = { text: { pl: payload }, url: "javascript:alert(1)" };
    e.cover!.alt = { pl: payload };
    e.cover!.variants = [{ width: 480, height: 270, url: `https://store.example/a.jpg"><script>alert(1)</script>` }];
    const o = e.occurrences[0]!;
    o.location = { name: payload, address: payload, mapUrl: "javascript:alert(1)" };
    o.facebookEventUrl = `javascript:alert(1)//https://www.facebook.com/events/1/`;
    s.organization.name = { pl: payload };
    return s;
  }

  // Every tag in the output is one the renderer writes, with only its own
  // attributes, each a well-formed double-quoted value; links are https or
  // site paths. So nothing from the payload became markup.
  const TAGS = new Set(["article", "h2", "h3", "img", "ul", "li", "span", "a", "strong", "p", "div", "br"]);
  const ATTRS = new Set(["class", "href", "rel", "src", "width", "height", "alt", "loading"]);
  const assertHarmless = (html: string) => {
    expect(html.replace(/<\/?[a-z0-9]+(?:\s[a-z]+="[^"<>]*")*\s*>/g, "")).not.toMatch(/[<>]/);
    for (const [, name, attrs] of html.matchAll(/<\/?([a-z0-9]+)([^>]*)>/g)) {
      expect(TAGS.has(name!)).toBe(true);
      for (const [, attr, value] of attrs!.matchAll(/\s([a-z]+)="([^"]*)"/g)) {
        expect(ATTRS.has(attr!)).toBe(true);
        if (attr === "href") expect(value).toMatch(/^(https:\/\/|\/)/);
      }
    }
    expect(html).not.toContain("javascript:");
  };

  for (const payload of HOSTILE) {
    for (const lang of ["pl", "en"] as const) {
      test(`${lang}: ${payload}`, () => {
        const s = hostile(payload);
        const e = bySlug(s, "bal-listopadowy");
        const html = renderEvent(s, e, lang, now) + renderList(s, lang, false, now) + renderCard(s, e, lang, now);
        assertHarmless(html);
        expect(html).toContain(esc(payload));
      });
    }
  }

  test("a javascript: or http map, tickets or Facebook address is not a link", () => {
    const s = hostile("x");
    const html = renderEvent(s, bySlug(s, "bal-listopadowy"), "pl", now);
    expect(html).not.toContain("Mapa");
    expect(html).not.toContain("Bilety");
    expect(html).not.toContain("Wydarzenie na Facebooku");
    const o = bySlug(s, "bal-listopadowy").occurrences[0]!;
    o.location!.mapUrl = "http://maps.example/zamek";
    expect(renderEvent(s, bySlug(s, "bal-listopadowy"), "pl", now)).not.toContain("Mapa");
  });

  test("the check catches markup that escaped escaping", () => {
    expect(() => assertHarmless(`<p><script>alert(1)</script></p>`)).toThrow();
    expect(() => assertHarmless(`<img alt=""><img src=x onerror=alert(1)>">`)).toThrow();
    expect(() => assertHarmless(`<a href="javascript:alert(1)">x</a>`)).toThrow();
    expect(() => assertHarmless(`<p onclick="x">y</p>`)).toThrow();
  });

  test("esc covers the HTML special characters", () => {
    expect(esc(`<a href="x" title='y'>&</a>`)).toBe("&lt;a href=&quot;x&quot; title=&#39;y&#39;&gt;&amp;&lt;/a&gt;");
  });
});
