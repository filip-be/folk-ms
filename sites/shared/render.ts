// A minimal, unstyled renderer every Site skeleton starts from. Designers
// replace the markup and styles; the data access stays in contract/client.

import {
  featuredOccurrence,
  fetchSnapshot,
  formatWhen,
  httpsUrl,
  isCancelled,
  occurrences,
  pickVariant,
  splitUpcomingPast,
  t,
  type Event,
  type Image,
  type Lang,
  type Occurrence,
  type Snapshot,
} from "../../contract/client/src/index.ts";
import { href, match, type Route } from "./router.ts";

// Everything in a snapshot is untrusted (much of it is Facebook text): every
// value goes through esc(), and an address becomes a link only through
// httpsUrl() (images: http or https, since the local CMS serves them over http).
const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const esc = (v: unknown) => String(v ?? "").replace(/[&<>"']/g, (c) => ESC[c]!);

const L = {
  pl: { upcoming: "Nadchodzące wydarzenia", past: "Minione wydarzenia", none: "Brak wydarzeń.", more: "Szczegóły", dates: "Terminy", cancelled: "odwołane", online: "online", tickets: "Bilety / zapisy", map: "Mapa", facebook: "Wydarzenie na Facebooku", notFound: "Nie znaleziono strony.", switchLang: "English", showPast: "Minione", showUpcoming: "Nadchodzące" },
  en: { upcoming: "Upcoming events", past: "Past events", none: "No events.", more: "Details", dates: "Dates", cancelled: "cancelled", online: "online", tickets: "Tickets / registration", map: "Map", facebook: "Facebook event", notFound: "Page not found.", switchLang: "Polski", showPast: "Past", showUpcoming: "Upcoming" },
};

const link = (url: string | undefined, label: string) => {
  const u = httpsUrl(url);
  return u ? `<a href="${esc(u)}" rel="noopener">${esc(label)}</a>` : "";
};

function img(image: Image | undefined, minWidth: number, lang: Lang, lazy: boolean): string {
  const v = pickVariant(image, minWidth);
  if (!v || !/^https?:\/\//i.test(v.url)) return "";
  return `<img src="${esc(v.url)}" width="${esc(v.width)}" height="${esc(v.height)}" alt="${esc(t(image?.alt, lang))}"${lazy ? ` loading="lazy"` : ""}>`;
}

/** Where an Occurrence happens, as plain text: location name, or "online". */
function where(o: Occurrence, lang: Lang): string {
  return [o.location?.name, o.online ? L[lang].online : undefined].filter(Boolean).join(" · ");
}

/** One Event in a list. */
export function renderCard(s: Snapshot, e: Event, lang: Lang, now: Date): string {
  const o = featuredOccurrence(e, now);
  const path = href({ name: "event", lang, shortId: e.shortId, slug: e.slug });
  return `<article class="event-card${o.cancelled ? " cancelled" : ""}">
    ${img(e.cover, 480, lang, true)}
    <h3><a href="${esc(path)}">${esc(t(e.title, lang))}</a></h3>
    <p class="when"><span class="date">${esc(formatWhen(o, lang, s.organization.timezone))}</span>${o.cancelled ? ` – <strong class="status">${esc(L[lang].cancelled)}</strong>` : ""}</p>
    <p class="where">${esc(where(o, lang))}</p>
    ${e.summary ? `<p>${esc(t(e.summary, lang))}</p>` : ""}
  </article>`;
}

/** The list of upcoming or past Events. */
export function renderList(s: Snapshot, lang: Lang, past: boolean, now: Date): string {
  const split = splitUpcomingPast(s.events, now);
  const events = past ? split.past : split.upcoming;
  return `<h2>${esc(past ? L[lang].past : L[lang].upcoming)}</h2>
    <p><a href="${esc(href({ name: "events", lang, past: !past }))}">${esc(past ? L[lang].showUpcoming : L[lang].showPast)}</a></p>
    ${events.length ? `<div class="events">${events.map((e) => renderCard(s, e, lang, now)).join("")}</div>` : `<p>${esc(L[lang].none)}</p>`}`;
}

function occurrenceItem(s: Snapshot, o: Occurrence, lang: Lang, now: Date): string {
  const loc = o.location;
  const classes = [o.cancelled ? "cancelled" : "", Date.parse(o.end) < now.getTime() ? "past" : ""].filter(Boolean).join(" ");
  const parts = [
    `<span class="when">${esc(formatWhen(o, lang, s.organization.timezone))}</span>`,
    loc
      ? `<span class="where">${esc(loc.name)}${loc.address ? `, ${esc(loc.address)}` : ""}${httpsUrl(loc.mapUrl) ? ` (${link(loc.mapUrl, L[lang].map)})` : ""}</span>`
      : "",
    o.online ? `<span class="online">${esc(L[lang].online)}</span>` : "",
    link(o.facebookEventUrl, L[lang].facebook),
  ].filter(Boolean);
  return `<li${classes ? ` class="${classes}"` : ""}>${o.cancelled ? `<strong class="status">${esc(L[lang].cancelled)}</strong> ` : ""}${parts.join(" · ")}</li>`;
}

function price(e: Event, lang: Lang): string {
  if (!e.price) return "";
  const parts = [e.price.text ? esc(t(e.price.text, lang)) : "", link(e.price.url, L[lang].tickets)].filter(Boolean);
  return parts.length ? `<p class="price">${parts.join(" · ")}</p>` : "";
}

/** An Event's Permalink page: every Occurrence, past and cancelled ones included. */
export function renderEvent(s: Snapshot, e: Event, lang: Lang, now: Date): string {
  return `<article class="event${isCancelled(e) ? " cancelled" : ""}">
    <h2>${esc(t(e.title, lang))}</h2>
    ${img(e.cover, 960, lang, false)}
    <h3>${esc(L[lang].dates)}</h3>
    <ul class="dates">${occurrences(e)
      .map((o) => occurrenceItem(s, o, lang, now))
      .join("")}</ul>
    ${price(e, lang)}
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
        body = renderList(s, lang, route.name === "events" && route.past, now);
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
        body = renderEvent(s, e, lang, now);
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
