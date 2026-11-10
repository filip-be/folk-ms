// Autofill on Facebook's create-event page: takes the pending handoff,
// fills the form, shows the status panel and, once the Moderator pressed
// Create, keeps the new Facebook Event's address for the CMS. Facebook
// navigates to the new event client-side (history.replaceState), so the
// address is polled.

import { createdEventId, eventUrl, parseFieldMap, type FieldMap, type FieldName } from "./fieldmap.ts";
import { ensureFocus, fillAll, findForm, valuesOf, waitFor } from "./fill.ts";
import { Panel } from "./panel.ts";
import type { FieldStatus, Handoff, Report } from "./protocol.ts";
import type { Store } from "./store.ts";

export interface Build {
  version: string;
  channel: string;
  browser: string;
}

/** How long Autofill watches for Create after filling. */
const WATCH_MS = 2 * 60 * 60 * 1000;

export async function runOnFacebook(store: Store, build: Build): Promise<void> {
  // The SPA stores the handoff before it opens this tab; allow a moment.
  if (!(await store.hasHandoff())) {
    await new Promise((r) => setTimeout(r, 1500));
    if (!(await store.hasHandoff())) return;
  }
  const h = await store.takeHandoff();
  if (!h) return;
  const values = valuesOf(h);
  const panel = new Panel(h, values, () => back(h));
  const warnings: string[] = [];
  let map;
  try {
    map = parseFieldMap(h.fieldMap);
  } catch (err) {
    panel.update({ message: panel.t("noForm"), fields: allNeedYou(values) });
    await report(store, build, h, allFailed(values), [`field map: ${String(err)}`]);
    return;
  }
  panel.update({ message: panel.t("waiting") });
  const found = await waitFor(() => findForm(map), 20000, 300);
  if (!found) {
    const fields = allFailed(values);
    panel.update({ message: panel.t("noForm"), fields });
    await report(store, build, h, fields, ["form not found"]);
    void watch(store, map, h, fields, panel);
    return;
  }
  await ensureFocus(() => panel.update({ message: panel.t("focus") }));
  panel.update({ message: panel.t("filling") });
  const result = await fillAll(map, h, found, (r) => panel.update({ fields: { ...r.fields } }));

  const browserTZ = Intl.DateTimeFormat().resolvedOptions().timeZone;
  if (result.fields.timezone !== "filled" && browserTZ !== h.fields.timezone) warnings.push(`${panel.t("tz")} (${h.fields.timezone}).`);
  const hostOk = [...document.querySelectorAll("[aria-label]")].some((e) => (e.getAttribute("aria-label") ?? "").startsWith(h.pageName + ","));
  if (found.form.name === "desktop" && !hostOk) warnings.push(`${panel.t("hostWarn")} „${h.pageName}”.`);
  panel.update({ message: panel.t("done"), warnings, fields: result.fields });

  if (Object.values(result.fields).some((s) => s === "failed")) {
    await report(store, build, h, result.fields as Record<string, FieldStatus>, result.errors);
  }
  await watch(store, map, h, result.fields as Record<string, FieldStatus>, panel);
}

function allFailed(values: Partial<Record<FieldName, string>>): Record<string, FieldStatus> {
  return Object.fromEntries(Object.entries(values).filter(([, v]) => v).map(([k]) => [k, "failed" as FieldStatus]));
}
function allNeedYou(values: Partial<Record<FieldName, string>>): Partial<Record<FieldName, FieldStatus>> {
  return Object.fromEntries(Object.entries(values).filter(([, v]) => v).map(([k]) => [k, "needs-you" as FieldStatus]));
}

async function report(store: Store, build: Build, h: Handoff, fields: Record<string, FieldStatus>, errors: string[]): Promise<void> {
  const r: Report = {
    id: `${h.id}:report`,
    org: h.org,
    eventId: h.eventId,
    version: build.version,
    channel: build.channel,
    browser: build.browser,
    uiLanguage: document.documentElement.lang || navigator.language,
    fields,
    errors: errors.slice(0, 20),
    filledAt: new Date().toISOString(),
  };
  await store.addReport(r);
}

/** Polls the address until it is a created Facebook Event. */
async function watch(store: Store, map: FieldMap, h: Handoff, fields: Record<string, FieldStatus>, panel: Panel): Promise<void> {
  const id = await waitFor(() => createdEventId(map, location.pathname), WATCH_MS, 500);
  if (!id) return;
  const url = eventUrl(id);
  await store.addResult({ id: h.id, org: h.org, eventId: h.eventId, occurrenceKey: h.occurrenceKey, token: h.token, url, fields, at: new Date().toISOString() });
  panel.update({ createdUrl: url, message: "" });
}

function back(h: Handoff): void {
  // The SPA opened this tab: closing it returns to the CMS tab. When the
  // browser refuses, go to the CMS here.
  window.close();
  setTimeout(() => location.assign(h.returnUrl), 300);
}
