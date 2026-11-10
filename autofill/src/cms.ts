// Autofill on the CMS origin: announces itself to the admin SPA, keeps the
// handoff the SPA passes (only a page on the CMS origin can create one),
// and passes back results and failure reports when the SPA asks, when the
// tab becomes visible or gains focus. Autofill never calls the CMS API.

import { EV, PROTOCOL, read, send, type Handoff, type Hello } from "./protocol.ts";
import type { Store } from "./store.ts";

export function runOnCms(store: Store, cmsOrigin: string, hello: Omit<Hello, "protocol">): void {
  if (location.origin !== cmsOrigin) return;
  const announce = () => send(EV.hello, { protocol: PROTOCOL, ...hello } satisfies Hello);

  document.addEventListener(EV.ping, announce);
  document.addEventListener(EV.handoff, (e) => {
    const h = read<Handoff>(e);
    if (!h || typeof h.id !== "string" || !h.fields || typeof h.returnUrl !== "string" || !h.returnUrl.startsWith(cmsOrigin + "/")) {
      send(EV.stored, { id: h?.id ?? "", ok: false, error: "invalid handoff" });
      return;
    }
    store.putHandoff(h).then(
      () => send(EV.stored, { id: h.id, ok: true }),
      (err: unknown) => send(EV.stored, { id: h.id, ok: false, error: String(err) }),
    );
  });

  let delivering = false;
  const deliver = async () => {
    if (delivering) return;
    delivering = true;
    try {
      for (const r of await store.results()) send(EV.result, r);
      const reports = await store.reports();
      if (reports.length) send(EV.reports, reports);
    } finally {
      delivering = false;
    }
  };
  document.addEventListener(EV.poll, () => void deliver());
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") void deliver();
  });
  addEventListener("focus", () => void deliver());
  document.addEventListener(EV.ack, (e) => {
    const ids = read<{ ids: string[] }>(e)?.ids;
    if (Array.isArray(ids)) void store.ack(ids.filter((x) => typeof x === "string"));
  });

  announce();
}
