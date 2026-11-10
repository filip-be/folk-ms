// The fill engine: types a Handoff into Facebook's create-event form. It
// never presses Create. Findings from the spike (ADR 0006):
// - typing through execCommand("insertText") is accepted by Facebook's
//   inputs; dates and times commit on blur, which needs the tab to have
//   focus;
// - a date in a format the UI locale cannot parse is silently reverted;
// - options of comboboxes are clicked on their innermost role=option.

import { dateCandidates, parseYMD, showsDate, showsTime, timeCandidates } from "./dates.ts";
import { detectForm, FIELD_ORDER, locateAny, type FieldMap, type FieldName, type FieldSpec, type Finder, type FormSpec } from "./fieldmap.ts";
import type { FieldStatus, Handoff } from "./protocol.ts";

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

const firstLine = (el: HTMLElement) => (el.innerText ?? el.textContent ?? "").split("\n").map((s) => s.trim()).find(Boolean) ?? "";

/** Rendered on the page (hidden controls may stay in the DOM). */
const visible = (el: Element) => el.getClientRects().length > 0;

/** A Finder over a DOM subtree. Labels, aria-labels and buttons count only when visible. */
export function domFinder(root: ParentNode): Finder<HTMLElement> {
  return {
    labels: () =>
      [...root.querySelectorAll<HTMLLabelElement>("label")].filter(visible).map((l) => ({
        text: firstLine(l),
        control:
          l.querySelector<HTMLElement>("input, textarea, select") ??
          (l.htmlFor ? (document.getElementById(l.htmlFor) ?? undefined) : undefined),
      })),
    ariaLabelled: (tag) =>
      [...root.querySelectorAll<HTMLElement>(tag ? `${tag}[aria-label]` : "[aria-label]")].filter(visible).map((el) => ({ label: el.getAttribute("aria-label") ?? "", el })),
    buttons: () => [...root.querySelectorAll<HTMLElement>('[role="button"], button')].filter(visible).map((el) => ({ text: firstLine(el), el })),
    select: (sel) => [...root.querySelectorAll<HTMLElement>(sel)],
    role: (r) => [...root.querySelectorAll<HTMLElement>(`[role="${r}"]`)],
  };
}

/** Waits until fn returns something, polling. */
export async function waitFor<T>(fn: () => T | undefined | null | false, timeoutMs: number, everyMs = 250): Promise<T | undefined> {
  const end = Date.now() + timeoutMs;
  for (;;) {
    const v = fn();
    if (v) return v;
    if (Date.now() > end) return undefined;
    await sleep(everyMs);
  }
}

/** Finds the form on the page: its spec and the element to search in. */
export function findForm(map: FieldMap): { form: FormSpec; root: HTMLElement } | undefined {
  for (const form of map.forms) {
    const roots: HTMLElement[] = [];
    for (const r of form.root) if (r.role) roots.push(...document.querySelectorAll<HTMLElement>(`[role="${r.role}"]`));
    if (!form.root.length) roots.push(document.body);
    for (const root of roots) {
      if (detectForm(domFinder(root), { ...map, forms: [form] })) return { form, root };
    }
  }
  return undefined;
}

/** Types text the way a person would, so the page's framework sees it. */
export function typeInto(el: HTMLElement, text: string, blur = true): void {
  const input = el as HTMLInputElement;
  el.scrollIntoView({ block: "center" });
  el.focus();
  input.select?.();
  let ok = false;
  // execCommand types wherever the caret is: only when el really has it.
  if (document.activeElement === el) {
    try {
      ok = document.execCommand("insertText", false, text);
    } catch {
      ok = false;
    }
  }
  if (!ok || input.value !== text) {
    const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value")?.set?.call(el, text);
    el.dispatchEvent(new Event("input", { bubbles: true }));
    el.dispatchEvent(new Event("change", { bubbles: true }));
  }
  if (blur) el.blur();
}

/** Visible, innermost options of open listboxes (anywhere: popups are portals). */
function options(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>('[role="option"]')].filter(
    (o) => !o.querySelector('[role="option"]') && o.getBoundingClientRect().height > 0,
  );
}

async function pickOption(match: (text: string) => boolean, timeoutMs = 3000): Promise<boolean> {
  const opt = await waitFor(() => options().find((o) => match((o.innerText ?? "").trim())), timeoutMs);
  if (!opt) return false;
  opt.click();
  await sleep(400);
  return true;
}

/** Scrolls an element into view, then clicks it: Facebook renders the popups
 * of controls scrolled out of its dialog with no size. */
async function clickInView(el: HTMLElement): Promise<void> {
  el.scrollIntoView({ block: "center" });
  await sleep(250);
  el.click();
}

/** Waits until the page has focus: Facebook commits dates on blur only then. */
export async function ensureFocus(onWait: () => void): Promise<void> {
  if (document.hasFocus()) return;
  onWait();
  await new Promise<void>((resolve) => {
    const done = () => {
      removeEventListener("focus", done);
      resolve();
    };
    addEventListener("focus", done);
  });
}

/** The text Facebook shows for a date control: its "…, selected: …" label, else its value. */
function shownDate(el: HTMLElement): string {
  const label = el.closest("label");
  const sel = label?.querySelector<HTMLElement>('[aria-label*="selected" i], [aria-label*="wybran" i]');
  return sel?.getAttribute("aria-label") ?? (el as HTMLInputElement).value ?? "";
}

export interface FieldOutcome {
  status: FieldStatus;
  error?: string;
}

/** Values the Handoff gives each field (undefined: nothing to fill). */
export function valuesOf(h: Handoff): Partial<Record<FieldName, string>> {
  const f = h.fields;
  return {
    cover: h.cover ? h.cover.name : undefined,
    name: f.name,
    startDate: f.startDate,
    startTime: f.startTime,
    timezone: f.timezone,
    endDate: f.endDate,
    endTime: f.endTime,
    location: f.location,
    description: f.description,
    ticketUrl: f.ticketUrl || undefined,
  };
}

async function locateField(f: Finder<HTMLElement>, spec: FieldSpec): Promise<HTMLElement | undefined> {
  let el = locateAny(f, spec.locators);
  if (!el && spec.reveal) {
    const btn = locateAny(f, spec.reveal);
    if (btn) {
      await clickInView(btn);
      el = await waitFor(() => locateAny(f, spec.locators), 2000);
    }
  }
  return el;
}

function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

export async function fillOne(root: HTMLElement, map: FieldMap, spec: FieldSpec, value: string, h: Handoff): Promise<FieldOutcome> {
  const f = domFinder(root);
  const el = await locateField(f, spec);
  if (spec.kind === "place" && spec.mode) {
    // Choose "In person" first: the address field appears only then.
    const same = (a: string, b: string) => a.trim().toLowerCase() === b.trim().toLowerCase();
    const modeEl = [...root.querySelectorAll<HTMLElement>("label")].find((l) => spec.mode!.label.some((t) => same(t, firstLine(l))));
    if (modeEl && !spec.mode.option.some((o) => modeEl.innerText.includes(o))) {
      await clickInView(modeEl);
      if (!(await pickOption((t) => spec.mode!.option.includes(t)))) return { status: "failed", error: "location: in-person option not found" };
    }
  }
  const target = el ?? (spec.kind === "place" ? await waitFor(() => locateAny(domFinder(root), spec.locators), 2000) : undefined);
  if (!target) return { status: "failed", error: "not found" };

  switch (spec.kind) {
    case "text": {
      typeInto(target, value);
      await sleep(150);
      const v = (target as HTMLInputElement).value;
      return v === value ? { status: "filled" } : { status: "failed", error: "value not kept" };
    }
    case "date": {
      const ymd = parseYMD(value);
      if (!ymd) return { status: "failed", error: "bad date" };
      for (const cand of dateCandidates(ymd, map.dateFormats)) {
        typeInto(target, cand);
        await sleep(400);
        if (showsDate(shownDate(target), ymd)) return { status: "filled" };
      }
      return { status: "failed", error: "no date format accepted" };
    }
    case "time": {
      for (const cand of timeCandidates(value, map.timeFormats)) {
        typeInto(target, cand);
        await sleep(300);
        if (showsTime((target as HTMLInputElement).value, value)) return { status: "filled" };
      }
      return { status: "failed", error: "no time format accepted" };
    }
    case "choice": {
      typeInto(target, value, false);
      const ok = await pickOption((t) => t.startsWith(value));
      target.blur();
      return ok ? { status: "filled" } : { status: "needs-you", error: "option not offered" };
    }
    case "place": {
      typeInto(target, value, false);
      const patterns = (spec.freeText ?? []).map((p) => new RegExp(p, "i"));
      const ok = await pickOption((t) => patterns.some((re) => re.test(t)));
      target.blur();
      return ok ? { status: "filled" } : { status: "needs-you", error: "free-text option not offered" };
    }
    case "file": {
      if (!h.cover) return { status: "needs-you" };
      const bytes = base64ToBytes(h.cover.data);
      const file = new File([bytes.buffer as ArrayBuffer], h.cover.name, { type: h.cover.type });
      const dt = new DataTransfer();
      dt.items.add(file);
      (target as HTMLInputElement).files = dt.files;
      target.dispatchEvent(new Event("input", { bubbles: true }));
      target.dispatchEvent(new Event("change", { bubbles: true }));
      await sleep(1500);
      return { status: "filled" };
    }
  }
}

export interface FillReport {
  form: string;
  fields: Partial<Record<FieldName, FieldStatus>>;
  errors: string[];
}

/** Fills every field the Handoff has a value for, in order. */
export async function fillAll(map: FieldMap, h: Handoff, found: { form: FormSpec; root: HTMLElement }, progress: (r: FillReport) => void): Promise<FillReport> {
  const values = valuesOf(h);
  const report: FillReport = { form: found.form.name, fields: {}, errors: [] };
  for (const name of FIELD_ORDER) {
    const value = values[name];
    if (value === undefined || value === "") continue;
    const spec = found.form.fields[name];
    if (!spec) {
      report.fields[name] = "needs-you";
      progress(report);
      continue;
    }
    try {
      const out = await fillOne(found.root, map, spec, value, h);
      report.fields[name] = out.status;
      if (out.error) report.errors.push(`${name}: ${out.error}`);
    } catch (err) {
      report.fields[name] = "failed";
      report.errors.push(`${name}: ${String(err).slice(0, 200)}`);
    }
    progress(report);
  }
  return report;
}
