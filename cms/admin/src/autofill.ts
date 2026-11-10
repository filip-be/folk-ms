// The admin SPA's side of the Autofill protocol (autofill/src/protocol.ts).
// Autofill announces itself through DOM events; the SPA hands it a
// Facebook Event Handoff before opening Facebook, and later receives the
// new Facebook Event's address and failure reports, which it saves through
// the API. Autofill itself never calls the API.

import { api } from "./api.ts";
import { EV, MARKER, PROTOCOL, read, send, type FillResult, type Handoff, type Hello, type Report } from "../../../autofill/src/protocol.ts";

export type { FillResult, Handoff, Hello };

/** autofill/fieldmap.json, embedded by build.ts: fixes travel with a CMS deploy. */
declare const __AUTOFILL_FIELDMAP__: unknown;
export const fieldMap: unknown = __AUTOFILL_FIELDMAP__;

let hello: Hello | undefined;
document.addEventListener(EV.hello, (e) => {
  const h = read<Hello>(e);
  if (h && h.protocol === PROTOCOL) hello = h;
});

/** Asks whether an Autofill runs in this page. */
export async function detect(timeoutMs = 400): Promise<Hello | undefined> {
  if (hello) return hello;
  send(EV.ping, {});
  const end = Date.now() + timeoutMs;
  while (!hello && Date.now() < end) await new Promise((r) => setTimeout(r, 50));
  return hello ?? (document.documentElement.hasAttribute(MARKER) ? { protocol: PROTOCOL, version: "?", channel: "?" } : undefined);
}

/** Passes a handoff to Autofill and waits until it is stored. */
export function handOff(h: Handoff, timeoutMs = 3000): Promise<boolean> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => done(false), timeoutMs);
    const onStored = (e: Event) => {
      const r = read<{ id: string; ok: boolean }>(e);
      if (r?.id === h.id) done(!!r.ok);
    };
    const done = (ok: boolean) => {
      clearTimeout(timer);
      document.removeEventListener(EV.stored, onStored);
      resolve(ok);
    };
    document.addEventListener(EV.stored, onStored);
    send(EV.handoff, h);
  });
}

/** Listens for created Facebook Events; returns the unsubscribe function. */
export function onResult(fn: (r: FillResult) => void): () => void {
  const l = (e: Event) => {
    const r = read<FillResult>(e);
    if (r && typeof r.id === "string") fn(r);
  };
  document.addEventListener(EV.result, l);
  return () => document.removeEventListener(EV.result, l);
}

export const poll = () => send(EV.poll, {});
export const ack = (ids: string[]) => send(EV.ack, { ids });

/** Relays Autofill's failure reports to the Audit Log, from any CMS page. */
export function relayReports(canManage: (org: string) => boolean): void {
  document.addEventListener(EV.reports, (e) => {
    const list = read<Report[]>(e);
    if (!Array.isArray(list)) return;
    void (async () => {
      const done: string[] = [];
      for (const r of list) {
        if (!r || typeof r.id !== "string") continue;
        if (!canManage(r.org)) {
          done.push(r.id); // not ours to report: drop it
          continue;
        }
        try {
          await api.autofillReport(r.org, {
            eventId: r.eventId,
            version: r.version,
            channel: r.channel,
            browser: r.browser,
            uiLanguage: r.uiLanguage,
            fields: r.fields,
            errors: r.errors,
            filledAt: r.filledAt,
          });
          done.push(r.id);
        } catch {
          /* kept by Autofill for the next CMS page */
        }
      }
      if (done.length) ack(done);
    })();
  });
  poll();
}

/** A random id for a handoff. */
export function newId(): string {
  const b = new Uint8Array(8);
  crypto.getRandomValues(b);
  return [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
}

/** Base64 of a same-origin file (the Cover Image route). */
export async function fetchBase64(url: string): Promise<{ data: string; type: string } | undefined> {
  const res = await fetch(url, { credentials: "same-origin" });
  if (!res.ok) return undefined;
  const buf = new Uint8Array(await res.arrayBuffer());
  let bin = "";
  for (let i = 0; i < buf.length; i += 0x8000) bin += String.fromCharCode(...buf.subarray(i, i + 0x8000));
  return { data: btoa(bin), type: res.headers.get("Content-Type") ?? "image/jpeg" };
}
