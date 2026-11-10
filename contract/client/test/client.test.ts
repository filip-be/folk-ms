import { describe, expect, test } from "bun:test";
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import {
  findByPermalinkSegment,
  nextOccurrence,
  occurrences,
  parseSnapshot,
  pickVariant,
  splitUpcomingPast,
  t,
  venue,
  type Snapshot,
} from "../src/index.ts";

const fixtures = join(import.meta.dir, "..", "..", "fixtures");
const load = (name: string): Snapshot => parseSnapshot(JSON.parse(readFileSync(join(fixtures, "valid", name), "utf8")));

describe("fixtures", () => {
  for (const f of readdirSync(join(fixtures, "valid"))) {
    test(`parses valid/${f}`, () => {
      expect(() => load(f)).not.toThrow();
    });
  }
  test("rejects wrong schema version", () => {
    const bad = JSON.parse(readFileSync(join(fixtures, "invalid", "wrong-schema-version.json"), "utf8"));
    expect(() => parseSnapshot(bad)).toThrow();
  });
});

describe("helpers", () => {
  const s = load("full.json");
  const now = new Date("2026-11-07T12:00:00Z");

  test("language fallback", () => {
    const fest = s.events[1]!;
    expect(t(fest.title, "en")).toBe("Festiwal zimowy");
    expect(t(s.events[0]!.title, "en")).toBe("November bal");
    expect(t(undefined, "pl")).toBe("");
  });

  test("upcoming/past split", () => {
    const { upcoming, past } = splitUpcomingPast(s.events, now);
    expect(upcoming.map((e) => e.slug)).toEqual(["bal-listopadowy", "jam-czwartkowy", "festiwal-zimowy"]);
    expect(past).toEqual([]);
    const later = splitUpcomingPast(s.events, new Date("2027-02-01T00:00:00Z"));
    expect(later.upcoming).toEqual([]);
    expect(later.past.map((e) => e.slug)).toEqual(["festiwal-zimowy", "jam-czwartkowy", "bal-listopadowy"]);
  });

  test("occurrence listing skips cancelled when asked", () => {
    const jam = s.events[2]!;
    expect(occurrences(jam).length).toBe(4);
    expect(occurrences(jam, { from: new Date("2026-10-01T00:00:00Z"), includeCancelled: false }).length).toBe(2);
    expect(nextOccurrence(jam, now)?.start).toBe("2026-11-26T19:00:00+01:00");
  });

  test("frozen venue for past occurrence", () => {
    const jam = s.events[2]!;
    expect(venue(s, jam.occurrences[0]!)?.name).toBe("Dom Kultury (stara sala)");
    expect(venue(s, jam.occurrences[2]!)?.name).toBe("Dom Kultury Zamek");
  });

  test("permalink lookup ignores slug", () => {
    expect(findByPermalinkSegment(s, "abcdefgh-old-slug")?.slug).toBe("bal-listopadowy");
    expect(findByPermalinkSegment(s, "zzzzzzzz")).toBeUndefined();
  });

  test("variant picking", () => {
    expect(pickVariant(s.events[0]!.cover, 600)?.width).toBe(960);
    expect(pickVariant(s.events[0]!.cover, 2000)?.width).toBe(960);
    expect(pickVariant(undefined, 1)).toBeUndefined();
  });
});

describe("facebook events", () => {
  test("an Occurrence carries its Facebook Event link", () => {
    const s = parseSnapshot(JSON.parse(readFileSync(join(fixtures, "valid", "full.json"), "utf8")));
    expect(s.events[0]!.occurrences[0]!.facebookEventUrl).toBe("https://www.facebook.com/events/1004759319341552/");
    expect(s.events[1]!.occurrences[0]!.facebookEventUrl).toBeUndefined();
  });
});
