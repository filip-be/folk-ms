import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { createdEventId, detectForm, eventUrl, locate, locateAny, parseFieldMap, type FieldMap, type Finder } from "../src/fieldmap.ts";

const raw = JSON.parse(readFileSync(join(import.meta.dir, "..", "fieldmap.json"), "utf8"));

interface FakePage {
  labels?: [string, string | undefined][];
  aria?: [string, string, string?][];
  buttons?: [string, string][];
  selectors?: Record<string, string[]>;
  roles?: Record<string, string[]>;
}

/** A fake page: elements are strings. */
function page(p: FakePage): Finder<string> {
  return {
    labels: () => (p.labels ?? []).map(([text, control]) => ({ text, control })),
    ariaLabelled: (tag) => (p.aria ?? []).filter(([, , t]) => !tag || t === tag).map(([label, el]) => ({ label, el })),
    buttons: () => (p.buttons ?? []).map(([text, el]) => ({ text, el })),
    select: (s) => p.selectors?.[s] ?? [],
    role: (r) => p.roles?.[r] ?? [],
  };
}

describe("field map", () => {
  test("the shipped field map is valid", () => {
    const m = parseFieldMap(raw);
    expect(m.forms.map((f) => f.name)).toEqual(["desktop", "mobile"]);
  });

  test("class selectors, unknown fields and bad patterns are refused", () => {
    const bad = (mut: (m: FieldMap) => void) => {
      const m = structuredClone(raw) as FieldMap;
      mut(m);
      return () => parseFieldMap(m);
    };
    expect(bad((m) => (m.forms[0]!.fields.name!.locators = [{ selector: "div.x1abc input" }]))).toThrow(/class/);
    expect(bad((m) => (m.forms[0]!.fields.name!.locators = [{ selector: "[class*=foo]" }]))).toThrow(/class/);
    expect(bad((m) => ((m.forms[0]!.fields as Record<string, unknown>).bogus = { kind: "text", locators: [{ label: ["x"] }] }))).toThrow(/unknown field/);
    expect(bad((m) => (m.forms[0]!.fields.name!.locators = []))).toThrow(/no locators/);
    expect(bad((m) => (m.created = "("))).toThrow();
    expect(() => parseFieldMap(null)).toThrow();
  });
});

describe("locators", () => {
  test("label text matches in Polish or English, ignoring case and spacing", () => {
    const f = page({ labels: [["  nazwa   WYDARZENIA ", "pl-name"], ["Start date", "start"]] });
    expect(locate(f, { label: ["Event name", "Nazwa wydarzenia"] })).toBe("pl-name");
    expect(locate(f, { label: ["Data rozpoczęcia", "Start date"] })).toBe("start");
    expect(locate(f, { label: ["End date"] })).toBeUndefined();
  });

  test("a label without a control does not match", () => {
    expect(locate(page({ labels: [["Event name", undefined]] }), { label: ["Event name"] })).toBeUndefined();
  });

  test("aria-label respects the tag; buttons, roles and selectors", () => {
    const f = page({
      aria: [
        ["Event name", "div-name", "div"],
        ["Event name", "input-name", "input"],
      ],
      buttons: [["Add tickets", "btn"]],
      roles: { dialog: ["dlg"] },
      selectors: { "input[type=file]": ["file"] },
    });
    expect(locate(f, { ariaLabel: ["Event name"], tag: "input" })).toBe("input-name");
    expect(locate(f, { text: ["Dodaj bilety", "Add tickets"] })).toBe("btn");
    expect(locate(f, { role: "dialog" })).toBe("dlg");
    expect(locate(f, { selector: "input[type=file]" })).toBe("file");
  });

  test("locators are tried in order", () => {
    const locs = [{ label: ["What are the details?"] }, { selector: "textarea" }];
    expect(locateAny(page({ labels: [["What are the details?", "rich"]], selectors: { textarea: ["ta"] } }), locs)).toBe("rich");
    expect(locateAny(page({ selectors: { textarea: ["ta"] } }), locs)).toBe("ta");
    expect(locateAny(page({}), undefined)).toBeUndefined();
  });

  test("the form is detected from the field map", () => {
    const m = parseFieldMap(raw);
    expect(detectForm(page({ labels: [["Nazwa wydarzenia", "n"]] }), m)?.name).toBe("desktop");
    expect(detectForm(page({ aria: [["Event name", "n", "input"]] }), m)?.name).toBe("mobile");
    expect(detectForm(page({}), m)).toBeUndefined();
  });
});

describe("Facebook's form as seen on 2026-11-10", () => {
  // Visible labels of the desktop create form, with end date and tickets revealed.
  const seen = {
    en: ["Event name", "Start date", "Start time", "Time zone", "End date", "End time", "Add location", "What are the details?", "Ticket URL"],
    pl: ["Nazwa wydarzenia", "Data rozpoczęcia", "Czas rozpoczęcia", "Strefa czasowa", "Data zakończenia", "Godzina zakończenia", "Dodaj lokalizację", "Jakie są szczegółowe informacje na temat tego wydarzenia?", "Adres URL strony z biletami"],
  };
  for (const [lang, labels] of Object.entries(seen)) {
    test(`every desktop field is found in the ${lang} UI`, () => {
      const m = parseFieldMap(raw);
      const f = page({ labels: labels.map((l) => [l, l] as [string, string]), selectors: { "input[type=file]": ["file"] } });
      const form = detectForm(f, m)!;
      expect(form.name).toBe("desktop");
      for (const [name, spec] of Object.entries(form.fields)) expect([name, locateAny(f, spec.locators)]).not.toEqual([name, undefined]);
    });
  }
  test("the location question and its in-person option, in both languages", () => {
    const loc = parseFieldMap(raw).forms[0]!.fields.location!;
    expect(loc.mode!.label).toContain("Czy wydarzenie jest offline czy online?");
    expect(loc.mode!.option).toContain("Offline");
    expect(loc.freeText!.some((p) => new RegExp(p, "i").test("Użyj tylko „Zamek”"))).toBe(true);
  });
});

describe("link capture", () => {
  const m = parseFieldMap(raw);
  test("the created event's id comes from its path", () => {
    expect(createdEventId(m, "/events/1234567890123/")).toBe("1234567890123");
    expect(createdEventId(m, "/events/1234567890123")).toBe("1234567890123");
    expect(createdEventId(m, "/events/create/")).toBeUndefined();
    expect(createdEventId(m, "/events/123/")).toBeUndefined();
    expect(createdEventId(m, "/groups/1/events/1234567890/")).toBeUndefined();
  });
  test("the address is the one the CMS stores", () => {
    expect(eventUrl("1234567890")).toBe("https://www.facebook.com/events/1234567890/");
  });
});
