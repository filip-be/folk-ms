// A minimal, unstyled renderer every Site skeleton starts from. Designers
// replace the markup and styles; the data access stays in contract/client.

import {
  fetchSnapshot,
  formatStart,
  nextOccurrence,
  occurrences,
  pickVariant,
  splitUpcomingPast,
  t,
  venue,
  type Event,
  type Lang,
  type Snapshot,
} from "../../contract/client/src/index.ts";
import { href, match, type Route } from "./router.ts";

const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const esc = (v: unknown) => String(v ?? "").replace(/[&<>"']/g, (c) => ESC[c]!);

const L = {
  pl: { upcoming: "Nadchodzące wydarzenia", past: "Minione wydarzenia", none: "Brak wydarzeń.", more: "Szczegóły", dates: "Terminy", cancelled: "odwołane", tickets: "Bilety / zapisy", map: "Mapa", notFound: "Nie znaleziono strony.", switchLang: "English", showPast: "Minione", showUpcoming: "Nadchodzące" },
  en: { upcoming: "Upcoming events", past: "Past events", none: "No events.", more: "Details", dates: "Dates", cancelled: "cancelled", tickets: "Tickets / registration", map: "Map", notFound: "Page not found.", switchLang: "Polski", showPast: "Past", showUpcoming: "Upcoming" },
};

function card(s: Snapshot, e: Event, lang: Lang, now: Date): string {
  const next = nextOccurrence(e, now) ?? e.occurrences[e.occurrences.length - 1]!;
  const img = pickVariant(e.cover, 480);
  const path = href({ name: "event", lang, shortId: e.shortId, slug: e.slug });
  return `<article class="event-card">
    ${img ? `<img src="${esc(img.url)}" width="${img.width}" height="${img.height}" alt="${esc(t(e.cover?.alt, lang))}" loading="lazy">` : ""}
    <h3><a href="${esc(path)}">${esc(t(e.title, lang))}</a></h3>
    <p class="when">${esc(formatStart(next, lang, s.organization.timezone))}</p>
    <p class="where">${esc(venue(s, next)?.name ?? "")}</p>
    ${e.summary ? `<p>${esc(t(e.summary, lang))}</p>` : ""}
  </article>`;
}

function list(s: Snapshot, lang: Lang, past: boolean, now: Date): string {
  const split = splitUpcomingPast(s.events, now);
  const events = past ? split.past : split.upcoming;
  return `<h2>${esc(past ? L[lang].past : L[lang].upcoming)}</h2>
    <p><a href="${esc(href({ name: "events", lang, past: !past }))}">${esc(past ? L[lang].showUpcoming : L[lang].showPast)}</a></p>
    ${events.length ? `<div class="events">${events.map((e) => card(s, e, lang, now)).join("")}</div>` : `<p>${esc(L[lang].none)}</p>`}`;
}

function detail(s: Snapshot, e: Event, lang: Lang, now: Date): string {
  const img = pickVariant(e.cover, 960);
  const occ = occurrences(e, { from: e.kind === "recurring" ? now : undefined });
  return `<article class="event">
    <h2>${esc(t(e.title, lang))}</h2>
    ${img ? `<img src="${esc(img.url)}" width="${img.width}" height="${img.height}" alt="${esc(t(e.cover?.alt, lang))}">` : ""}
    <h3>${esc(L[lang].dates)}</h3>
    <ul class="dates">${occ
      .map((o) => {
        const v = venue(s, o);
        return `<li class="${o.cancelled ? "cancelled" : ""}">${esc(formatStart(o, lang, s.organization.timezone))}
          ${v ? ` · ${esc(v.name)}, ${esc(v.address)}${v.mapUrl ? ` (<a href="${esc(v.mapUrl)}" rel="noopener">${esc(L[lang].map)}</a>)` : ""}` : ""}
          ${o.cancelled ? ` – <strong>${esc(L[lang].cancelled)}</strong>` : ""}${o.note ? ` – ${esc(t(o.note, lang))}` : ""}</li>`;
      })
      .join("")}</ul>
    ${e.price ? `<p class="price">${esc(t(e.price.text, lang))}${e.price.url ? ` · <a href="${esc(e.price.url)}" rel="noopener">${esc(L[lang].tickets)}</a>` : ""}</p>` : ""}
    ${e.description ? `<div class="description">${esc(t(e.description, lang)).replace(/\n/g, "<br>")}</div>` : ""}
  </article>`;
}

/** Renders the current location into root. */
export async function start(root: HTMLElement, snapshotUrl: string): Promise<void> {
  let snapshot: Snapshot | undefined;
  const render = async () => {
    const route: Route = match(location.pathname);
    const lang = route.lang;
    document.documentElement.lang = lang;
    try {
      snapshot ??= await fetchSnapshot(snapshotUrl);
    } catch {
      root.innerHTML = `<p>${lang === "pl" ? "Nie udało się wczytać wydarzeń." : "Could not load events."}</p>`;
      return;
    }
    const s = snapshot;
    const now = new Date();
    const other: Lang = lang === "pl" ? "en" : "pl";
    let body: string;
    let canonical: string | undefined;
    switch (route.name) {
      case "home":
      case "events":
        body = list(s, lang, route.name === "events" && route.past, now);
        canonical = href(route.name === "home" ? { name: "home", lang: other } : { ...route, lang: other });
        break;
      case "event": {
        const e = s.events.find((x) => x.shortId === route.shortId);
        if (!e) {
          body = `<p>${esc(L[lang].notFound)}</p>`;
          break;
        }
        // The slug is cosmetic: fix the address bar if it changed.
        const want = href({ name: "event", lang, shortId: e.shortId, slug: e.slug });
        if (location.pathname !== want) history.replaceState(null, "", want);
        document.title = `${t(e.title, lang)} – ${t(s.organization.name, lang)}`;
        body = detail(s, e, lang, now);
        canonical = href({ name: "event", lang: other, shortId: e.shortId, slug: e.slug });
        break;
      }
      default:
        body = `<p>${esc(L[lang].notFound)}</p>`;
    }
    root.innerHTML = `<header><h1><a href="/${lang}/">${esc(t(s.organization.name, lang))}</a></h1>
      ${canonical ? `<a class="lang" href="${esc(canonical)}" hreflang="${other}">${esc(L[lang].switchLang)}</a>` : ""}</header>
      <main>${body}</main>`;
  };
  document.addEventListener("click", (e) => {
    const a = (e.target as HTMLElement).closest("a");
    const url = a && new URL(a.href, location.href);
    if (!a || !url || url.origin !== location.origin || a.target || e.ctrlKey || e.metaKey) return;
    e.preventDefault();
    history.pushState(null, "", url.pathname);
    void render();
    scrollTo(0, 0);
  });
  addEventListener("popstate", () => void render());
  await render();
}
