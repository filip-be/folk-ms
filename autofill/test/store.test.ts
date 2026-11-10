import { describe, expect, test } from "bun:test";
import { HANDOFF_TTL_MS, RESULT_TTL_MS, type FillResult, type Handoff, type Report } from "../src/protocol.ts";
import { MemoryKV, Store } from "../src/store.ts";

const handoff = (id: string): Handoff => ({
  id,
  org: "org-a",
  eventId: "e1",
  occurrenceKey: "2026-11-21",
  token: "t",
  pageName: "Folk",
  fields: { name: "Bal", startDate: "2026-11-21", startTime: "19:00", endDate: "2026-11-22", endTime: "01:00", timezone: "Europe/Warsaw", location: "Zamek", description: "Opis" },
  fieldMap: {},
  returnUrl: "https://cms.example/app/",
  lang: "pl",
});

function clock(t0 = Date.parse("2026-11-10T12:00:00Z")) {
  const c = { t: t0, now: () => c.t };
  return c;
}

describe("pending handoff", () => {
  test("is used once", async () => {
    const s = new Store(new MemoryKV(), clock().now);
    await s.putHandoff(handoff("h1"));
    expect(await s.hasHandoff()).toBe(true);
    expect((await s.takeHandoff())?.id).toBe("h1");
    expect(await s.hasHandoff()).toBe(false);
    expect(await s.takeHandoff()).toBeUndefined();
  });

  test("expires after 15 minutes, and an expired one is dropped", async () => {
    const c = clock();
    const kv = new MemoryKV();
    const s = new Store(kv, c.now);
    await s.putHandoff(handoff("h1"));
    c.t += HANDOFF_TTL_MS - 1;
    expect(await s.hasHandoff()).toBe(true);
    c.t += 1;
    expect(await s.hasHandoff()).toBe(false);
    expect(await s.takeHandoff()).toBeUndefined();
    expect(kv.data.size).toBe(0);
  });

  test("only one is kept: a later handoff replaces the earlier", async () => {
    const s = new Store(new MemoryKV(), clock().now);
    await s.putHandoff(handoff("h1"));
    await s.putHandoff(handoff("h2"));
    expect((await s.takeHandoff())?.id).toBe("h2");
    expect(await s.takeHandoff()).toBeUndefined();
  });
});

describe("results and reports", () => {
  const result = (id: string, at: number): FillResult => ({
    id,
    org: "org-a",
    eventId: "e1",
    occurrenceKey: "k",
    token: "t",
    url: "https://www.facebook.com/events/123456/",
    fields: { name: "filled" },
    at: new Date(at).toISOString(),
  });
  const report = (id: string): Report => ({
    id,
    org: "org-a",
    eventId: "e1",
    version: "0.1.0",
    channel: "extension",
    browser: "Firefox 140",
    uiLanguage: "pl",
    fields: { location: "failed" },
    errors: [],
    filledAt: "",
  });

  test("a result replaces one with the same id and expires after a day", async () => {
    const c = clock();
    const s = new Store(new MemoryKV(), c.now);
    await s.addResult(result("h1", c.t));
    await s.addResult({ ...result("h1", c.t), url: "https://www.facebook.com/events/999999/" });
    expect((await s.results()).map((r) => r.url)).toEqual(["https://www.facebook.com/events/999999/"]);
    c.t += RESULT_TTL_MS;
    expect(await s.results()).toEqual([]);
  });

  test("ack drops only the delivered results and reports", async () => {
    const c = clock();
    const s = new Store(new MemoryKV(), c.now);
    await s.addResult(result("h1", c.t));
    await s.addResult(result("h2", c.t));
    await s.addReport(report("h1:report"));
    await s.addReport(report("h2:report"));
    await s.ack(["h1", "h2:report"]);
    expect((await s.results()).map((r) => r.id)).toEqual(["h2"]);
    expect((await s.reports()).map((r) => r.id)).toEqual(["h1:report"]);
  });

  test("queues are bounded", async () => {
    const s = new Store(new MemoryKV(), clock().now);
    for (let i = 0; i < 60; i++) await s.addReport(report(`r${i}`));
    const ids = (await s.reports()).map((r) => r.id);
    expect(ids.length).toBe(50);
    expect(ids[49]).toBe("r59");
  });
});
