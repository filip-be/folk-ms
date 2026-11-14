import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  featuredOccurrence,
  findByPermalinkSegment,
  formatWhen,
  hasTranslation,
  httpsUrl,
  isCancelled,
  isMultiDay,
  nextOccurrence,
  occurrences,
  parseSnapshot,
  pickVariant,
  splitUpcomingPast,
  t,
  type Snapshot,
} from "../src/index.ts";

const fixtures = join(import.meta.dir, "..", "..", "fixtures");
const raw = (kind: string, name: string): unknown => JSON.parse(readFileSync(join(fixtures, kind, name), "utf8"));
const load = (name: string): Snapshot => parseSnapshot(raw("valid", name));
const TZ = "Europe/Warsaw";

describe("fixtures", () => {
  for (const f of readdirSync(join(fixtures, "valid"))) {
    test(`parses valid/${f}`, () => {
      expect(() => load(f)).not.toThrow();
    });
  }
  test("rejects schemaVersion 1 with a clear message", () => {
    expect(() => parseSnapshot(raw("invalid", "wrong-schema-version.json"))).toThrow(
      "unsupported schemaVersion 1; this reader reads version 2 only",
    );
  });
  test("rejects an event without occurrences", () => {
    expect(() => parseSnapshot(raw("invalid", "no-occurrences.json"))).toThrow("has no occurrences");
  });
  test("rejects a non-object and a missing events list", () => {
    expect(() => parseSnapshot(null)).toThrow("not an object");
    expect(() => parseSnapshot({ schemaVersion: 2, organization: {} })).toThrow("missing organization or events");
  });
});

describe("helpers", () => {
  const s = load("full.json");
  const byslug = (slug: string) => s.events.find((e) => e.slug === slug)!;
  const now = new Date("2026-11-07T12:00:00Z");

  test("language fallback", () => {
    const fest = byslug("festiwal-zimowy");
    expect(t(fest.title, "en")).toBe("Festiwal zimowy");
    expect(hasTranslation(fest.title, "en")).toBe(false);
    expect(t(byslug("bal-listopadowy").title, "en")).toBe("November bal");
    expect(t(undefined, "pl")).toBe("");
  });

  test("upcoming/past split keeps a cancelled upcoming Event upcoming", () => {
    const { upcoming, past } = splitUpcomingPast(s.events, now);
    expect(upcoming.map((e) => e.slug)).toEqual(["bal-listopadowy", "jam-czwartkowy", "warsztaty-online", "koncert-adwentowy", "festiwal-zimowy"]);
    expect(past).toEqual([]);
    const later = splitUpcomingPast(s.events, new Date("2027-02-01T00:00:00Z"));
    expect(later.upcoming).toEqual([]);
    expect(later.past.map((e) => e.slug)).toEqual(["festiwal-zimowy", "jam-czwartkowy", "koncert-adwentowy", "warsztaty-online", "bal-listopadowy"]);
  });

  test("a repeating Event: occurrence listing, next and featured", () => {
    const jam = byslug("jam-czwartkowy");
    expect(occurrences(jam).length).toBe(4);
    expect(occurrences(jam, { from: new Date("2026-10-01T00:00:00Z"), includeCancelled: false }).length).toBe(2);
    expect(nextOccurrence(jam, now)?.start).toBe("2026-11-26T19:00:00+01:00");
    expect(featuredOccurrence(jam, now).start).toBe("2026-11-26T19:00:00+01:00");
    expect(new Set(jam.occurrences.map((o) => o.facebookEventUrl)).size).toBe(4);
    expect(isCancelled(jam)).toBe(false);
  });

  test("a cancelled Event features its cancelled Occurrence", () => {
    const concert = byslug("koncert-adwentowy");
    expect(isCancelled(concert)).toBe(true);
    expect(nextOccurrence(concert, now)).toBeUndefined();
    expect(featuredOccurrence(concert, now).cancelled).toBe(true);
  });

  test("location and online live on the Occurrence", () => {
    const bal = byslug("bal-listopadowy").occurrences[0]!;
    expect(bal.location?.name).toBe("Dom Kultury Zamek");
    expect(bal.online).toBeUndefined();
    const online = byslug("warsztaty-online").occurrences[0]!;
    expect(online.online).toBe(true);
    expect(online.location).toBeUndefined();
  });

  test("price may be text, a link or both", () => {
    expect(byslug("bal-listopadowy").price).toEqual({ text: { pl: "40 zł, ulgowy 30 zł", en: "40 PLN, reduced 30 PLN" }, url: "https://tickets.example/bal" });
    expect(byslug("festiwal-zimowy").price).toEqual({ url: "https://tickets.example/festiwal" });
    expect(byslug("jam-czwartkowy").price?.url).toBeUndefined();
  });

  test("multi-day is derived from dates", () => {
    expect(isMultiDay(byslug("festiwal-zimowy").occurrences[0]!, TZ)).toBe(true);
    // A bal from 19:00 to 01:00 is one evening, not two days.
    expect(isMultiDay(byslug("bal-listopadowy").occurrences[0]!, TZ)).toBe(false);
    expect(isMultiDay({ start: "2026-11-21T19:00:00+01:00", end: "2026-11-22T10:00:00+01:00", cancelled: false, facebookEventUrl: "" }, TZ)).toBe(true);
    // Midnight in UTC is not midnight in Warsaw.
    expect(isMultiDay({ start: "2026-11-21T22:30:00Z", end: "2026-11-21T23:30:00Z", cancelled: false, facebookEventUrl: "" }, TZ)).toBe(false);
    expect(formatWhen(byslug("festiwal-zimowy").occurrences[0]!, "en", TZ)).toMatch(/^15.*17 January 2027$/);
    expect(formatWhen(byslug("bal-listopadowy").occurrences[0]!, "en", TZ)).toContain("19:00");
  });

  test("httpsUrl lets only https through", () => {
    expect(httpsUrl("https://maps.example/zamek")).toBe("https://maps.example/zamek");
    expect(httpsUrl("http://maps.example/zamek")).toBeUndefined();
    expect(httpsUrl("javascript:alert(1)")).toBeUndefined();
    expect(httpsUrl(" JavaScript:alert(1)//https://x")).toBeUndefined();
    expect(httpsUrl("//evil.example/")).toBeUndefined();
    expect(httpsUrl(undefined)).toBeUndefined();
  });

  test("permalink lookup ignores slug", () => {
    expect(findByPermalinkSegment(s, "abcdefgh-old-slug")?.slug).toBe("bal-listopadowy");
    expect(findByPermalinkSegment(s, "zzzzzzzz")).toBeUndefined();
  });

  test("variant picking", () => {
    const cover = byslug("bal-listopadowy").cover;
    expect(pickVariant(cover, 600)?.width).toBe(960);
    expect(pickVariant(cover, 2000)?.width).toBe(960);
    expect(pickVariant(undefined, 1)).toBeUndefined();
  });
});
