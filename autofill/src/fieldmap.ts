// The field map (autofill/fieldmap.json) and the locator engine. Locators
// find Facebook's form fields by ARIA role, accessible name or visible
// label text in Polish or English, never by class names. The engine works
// on a small Finder interface so tests can run it without a DOM.

export interface Locator {
  /** A <label> whose first line of text is one of these; its control. */
  label?: string[];
  /** An element whose aria-label is one of these. */
  ariaLabel?: string[];
  /** A button whose visible text is one of these. */
  text?: string[];
  /** Restricts ariaLabel to a tag name (lowercase). */
  tag?: string;
  /** A structural CSS selector (no class names), as a last resort. */
  selector?: string;
  /** An ARIA role (used for the form root). */
  role?: string;
}

export type FieldKind = "text" | "date" | "time" | "choice" | "place" | "file";

export interface FieldSpec {
  kind: FieldKind;
  locators: Locator[];
  /** Controls to click first when the field is hidden behind a button. */
  reveal?: Locator[];
  /** place: how to choose an in-person event before typing the address. */
  mode?: { label: string[]; option: string[] };
  /** place: patterns of the option that keeps the typed text as it is. */
  freeText?: string[];
}

export interface FormSpec {
  name: string;
  detect: Locator[];
  root: Locator[];
  fields: Record<string, FieldSpec>;
}

export interface FieldMap {
  version: number;
  checked?: string;
  created: string;
  forms: FormSpec[];
  dateFormats: string[];
  timeFormats: string[];
}

/** The fields Autofill knows, in the order it fills them. */
export const FIELD_ORDER = [
  "cover",
  "name",
  "startDate",
  "startTime",
  "timezone",
  "endDate",
  "endTime",
  "location",
  "description",
  "ticketUrl",
] as const;
export type FieldName = (typeof FIELD_ORDER)[number];

/** What the engine needs from a document or element. */
export interface Finder<E> {
  /** Every <label> with its first line of text and its control. */
  labels(): { text: string; control: E | undefined }[];
  /** Elements with an aria-label, optionally of a tag. */
  ariaLabelled(tag?: string): { label: string; el: E }[];
  /** Buttons (role=button or <button>) with their visible text. */
  buttons(): { text: string; el: E }[];
  /** Elements matching a structural selector. */
  select(selector: string): E[];
  /** Elements with an ARIA role. */
  role(role: string): E[];
}

const norm = (s: string) => s.replace(/\s+/g, " ").trim().toLowerCase();
const oneOf = (names: string[] | undefined, s: string) => !!names?.some((n) => norm(n) === norm(s));

/** Finds the first element a locator matches, or undefined. */
export function locate<E>(f: Finder<E>, loc: Locator): E | undefined {
  if (loc.label) {
    const hit = f.labels().find((l) => oneOf(loc.label, l.text) && l.control !== undefined);
    if (hit) return hit.control;
  }
  if (loc.ariaLabel) {
    const hit = f.ariaLabelled(loc.tag).find((x) => oneOf(loc.ariaLabel, x.label));
    if (hit) return hit.el;
  }
  if (loc.text) {
    const hit = f.buttons().find((b) => oneOf(loc.text, b.text));
    if (hit) return hit.el;
  }
  if (loc.role) {
    const hit = f.role(loc.role)[0];
    if (hit) return hit;
  }
  if (loc.selector) {
    const hit = f.select(loc.selector)[0];
    if (hit) return hit;
  }
  return undefined;
}

/** Tries locators in order. */
export function locateAny<E>(f: Finder<E>, locs: Locator[] | undefined): E | undefined {
  for (const l of locs ?? []) {
    const el = locate(f, l);
    if (el !== undefined) return el;
  }
  return undefined;
}

/** Picks the form spec whose detect locator matches. */
export function detectForm<E>(f: Finder<E>, map: FieldMap): FormSpec | undefined {
  return map.forms.find((form) => locateAny(f, form.detect) !== undefined);
}

/** Checks the shape of a field map received from the CMS. */
export function parseFieldMap(v: unknown): FieldMap {
  const m = v as FieldMap;
  if (!m || typeof m !== "object" || typeof m.created !== "string" || !Array.isArray(m.forms) || !Array.isArray(m.dateFormats) || !Array.isArray(m.timeFormats)) {
    throw new Error("invalid field map");
  }
  for (const form of m.forms) {
    if (typeof form.name !== "string" || !Array.isArray(form.detect) || typeof form.fields !== "object") throw new Error("invalid field map form");
    for (const [name, spec] of Object.entries(form.fields)) {
      if (!(FIELD_ORDER as readonly string[]).includes(name)) throw new Error(`unknown field ${name}`);
      if (!Array.isArray(spec.locators) || !spec.locators.length) throw new Error(`field ${name} has no locators`);
      for (const l of [...spec.locators, ...(spec.reveal ?? [])]) {
        if (l.selector && /\.[A-Za-z_-]|\[class/.test(l.selector)) throw new Error(`field ${name}: class selectors are not allowed`);
      }
    }
  }
  new RegExp(m.created); // throws on a bad pattern
  return m;
}

/** Extracts the numeric id of a created Facebook Event from a path. */
export function createdEventId(map: FieldMap, pathname: string): string | undefined {
  return new RegExp(map.created).exec(pathname)?.[1];
}

/** The canonical address of a Facebook Event, as the CMS stores it. */
export function eventUrl(id: string): string {
  return `https://www.facebook.com/events/${id}/`;
}
