import { describe, expect, test } from "bun:test";
import { dateCandidates, formatDate, parseYMD, showsDate, showsTime, timeCandidates } from "../src/dates.ts";

const d = parseYMD("2026-11-05")!;

describe("dates", () => {
  test("parse", () => {
    expect(d).toEqual({ y: 2026, m: 11, d: 5 });
    expect(parseYMD("5.11.2026")).toBeUndefined();
  });

  test("format in English and Polish", () => {
    expect(formatDate(d, "d MMM yyyy")).toBe("5 Nov 2026");
    expect(formatDate(d, "dd.MM.yyyy")).toBe("05.11.2026");
    expect(formatDate(d, "MMMM d, yyyy")).toBe("November 5, 2026");
    expect(formatDate(d, "d MMMM yyyy", "pl")).toBe("5 listopada 2026");
    expect(formatDate(d, "d MMM yyyy", "pl")).toBe("5 lis 2026");
  });

  test("candidates are unique and in pattern order", () => {
    expect(dateCandidates(d, ["d MMM yyyy", "dd/MM/yyyy"])).toEqual(["5 Nov 2026", "5 lis 2026", "05/11/2026"]);
  });

  test("recognises what Facebook shows", () => {
    expect(showsDate("Start date, selected: 5 Nov 2026", d)).toBe(true);
    expect(showsDate("05/11/2026", d)).toBe(true);
    expect(showsDate("2026-11-05", d)).toBe(true);
    expect(showsDate("5 listopada 2026", d)).toBe(true);
    expect(showsDate("Thursday, November 5, 2026", d)).toBe(true);
    expect(showsDate("11/05/2026", d)).toBe(false); // month read as the day
    expect(showsDate("5 Dec 2026", d)).toBe(false);
    expect(showsDate("5 Nov 2027", d)).toBe(false);
    expect(showsDate("", d)).toBe(false);
  });
});

describe("times", () => {
  test("candidates", () => {
    expect(timeCandidates("19:00", ["HH:mm", "h:mm a"])).toEqual(["19:00", "7:00 PM"]);
    expect(timeCandidates("00:30", ["h:mm a"])).toEqual(["12:30 AM"]);
    expect(timeCandidates("12:15", ["h:mm a"])).toEqual(["12:15 PM"]);
    expect(timeCandidates("7pm", ["HH:mm"])).toEqual([]);
  });

  test("recognises what Facebook shows", () => {
    expect(showsTime("19:00", "19:00")).toBe(true);
    expect(showsTime("7:00 PM", "19:00")).toBe(true);
    expect(showsTime("12:30 a.m.", "00:30")).toBe(true);
    expect(showsTime("12:15 pm", "12:15")).toBe(true);
    expect(showsTime("7:00 AM", "19:00")).toBe(false);
    expect(showsTime("", "19:00")).toBe(false);
  });
});
