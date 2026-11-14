// The videos() page module: /<lang>/videos, the Organization's Video
// Collection with filters by Form, Origin and title, and grouping by Form
// or Origin. The state lives in the query string (?form=a,b&origin=x&q=…&group=form),
// so a filtered view can be shared and Back/Forward works. A card shows the
// thumbnail; the player loads only when the visitor asks for it.

import { t, type Lang, type LocalizedText } from "../../contract/client/src/index.ts";
import {
  fetchVideos,
  filterVideos,
  formatStart,
  groupVideos,
  originLabel,
  originPath,
  playerUrl,
  usedForms,
  usedOrigins,
  type GroupBy,
  type Video,
  type VideoGroup,
  type VideoSnapshot,
} from "../../contract/client/src/videos.ts";
import { esc, LoadError, type Module, type Page } from "./site.ts";

const L = {
  pl: {
    title: "Wideopomoc",
    forms: "Forma",
    country: "Kraj",
    region: "Region",
    anyCountry: "Wszystkie kraje",
    wholeCountry: "Cały kraj",
    search: "Szukaj w tytułach",
    group: "Grupuj",
    groupNone: "bez grupowania",
    groupForm: "według formy",
    groupOrigin: "według pochodzenia",
    clear: "Wyczyść filtry",
    other: "Inne",
    none: "Brak filmów dla wybranych filtrów.",
    empty: "Wkrótce pojawią się tu filmy.",
    play: "Odtwórz",
    from: "od",
    watchOn: { youtube: "Obejrzyj na YouTube", vimeo: "Obejrzyj na Vimeo" },
    count: (n: number) => `${n} ${n === 1 ? "film" : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? "filmy" : "filmów"}`,
    load: "Nie udało się wczytać filmów.",
  },
  en: {
    title: "Video help",
    forms: "Form",
    country: "Country",
    region: "Region",
    anyCountry: "All countries",
    wholeCountry: "Whole country",
    search: "Search titles",
    group: "Group",
    groupNone: "no grouping",
    groupForm: "by form",
    groupOrigin: "by origin",
    clear: "Clear filters",
    other: "Other",
    none: "No videos match the filters.",
    empty: "Videos are coming soon.",
    play: "Play",
    from: "from",
    watchOn: { youtube: "Watch on YouTube", vimeo: "Watch on Vimeo" },
    count: (n: number) => `${n} ${n === 1 ? "video" : "videos"}`,
    load: "Could not load the videos.",
  },
};

/** The page's state, as in the query string. */
export interface VideoState {
  formIds: string[];
  originId?: string;
  query: string;
  group?: GroupBy;
}

/** The path of the videos page. */
export const videosPath = (lang: Lang) => `/${lang}/videos`;

/** Reads the state from a query string; unknown ids are dropped. */
export function readState(s: VideoSnapshot, search: URLSearchParams): VideoState {
  const known = new Set(s.forms.map((f) => f.id));
  const formIds = (search.get("form") ?? "").split(",").filter((id) => known.has(id));
  const origin = search.get("origin") ?? "";
  const group = search.get("group");
  return {
    formIds: [...new Set(formIds)],
    originId: originPath(s, origin) ? origin : undefined,
    query: (search.get("q") ?? "").slice(0, 100),
    group: group === "form" || group === "origin" ? group : undefined,
  };
}

/** Writes the state as a query string ("" when nothing is chosen). */
export function writeState(st: VideoState): string {
  const q = new URLSearchParams();
  if (st.formIds.length) q.set("form", st.formIds.join(","));
  if (st.originId) q.set("origin", st.originId);
  if (st.query.trim()) q.set("q", st.query);
  if (st.group) q.set("group", st.group);
  const s = q.toString();
  return s ? `?${s}` : "";
}

const option = (value: string, label: string, selected: boolean) =>
  `<option value="${esc(value)}"${selected ? " selected" : ""}>${esc(label)}</option>`;

/** The filter and grouping controls. */
export function renderControls(s: VideoSnapshot, st: VideoState, lang: Lang): string {
  const l = L[lang];
  const forms = usedForms(s);
  const origins = usedOrigins(s);
  const path = originPath(s, st.originId);
  const country = path && origins.find((c) => c.id === path.country.id);
  const chips = forms.length
    ? `<fieldset class="chips"><legend>${esc(l.forms)}</legend>${forms
        .map((f) => `<button type="button" class="chip" data-form="${esc(f.id)}" aria-pressed="${st.formIds.includes(f.id)}">${esc(t(f.label, lang))}</button>`)
        .join("")}</fieldset>`
    : "";
  const countries = origins.length
    ? `<label>${esc(l.country)} <select data-control="country">${option("", l.anyCountry, !path)}${origins
        .map((c) => option(c.id, t(c.label, lang), path?.country.id === c.id))
        .join("")}</select></label>`
    : "";
  const regions = country?.regions.length
    ? `<label>${esc(l.region)} <select data-control="region">${option("", l.wholeCountry, !path?.region)}${country.regions
        .map((r) => option(r.id, t(r.label, lang), path?.region?.id === r.id))
        .join("")}</select></label>`
    : "";
  return `${chips}
    <div class="selects">
      ${countries}${regions}
      <label>${esc(l.search)} <input type="search" data-control="query" value="${esc(st.query)}" maxlength="100" autocomplete="off"></label>
      <label>${esc(l.group)} <select data-control="group">${option("", l.groupNone, !st.group)}${forms.length ? option("form", l.groupForm, st.group === "form") : ""}${
        origins.length ? option("origin", l.groupOrigin, st.group === "origin") : ""
      }</select></label>
      <button type="button" data-control="clear"${st.formIds.length || st.originId || st.query || st.group ? "" : " disabled"}>${esc(l.clear)}</button>
    </div>`;
}

/** One Video: its thumbnail as the play button, title and details. */
export function renderVideo(s: VideoSnapshot, v: Video, lang: Lang, heading: number): string {
  const l = L[lang];
  const title = t(v.title, lang);
  const thumb = /^https:\/\//i.test(v.thumbnail) ? v.thumbnail : "";
  const forms = v.formIds.map((id) => s.forms.find((f) => f.id === id)).filter((f) => !!f);
  const meta = [originLabel(s, v.originId, lang), forms.map((f) => t(f.label, lang)).join(", "), v.start ? `${l.from} ${formatStart(v.start)}` : ""]
    .filter(Boolean)
    .map(esc)
    .join(" · ");
  const watch = /^https:\/\//i.test(v.url) ? `<a href="${esc(v.url)}" target="_blank" rel="noopener">${esc(l.watchOn[v.provider] ?? v.url)}</a>` : "";
  const h = `h${Math.min(6, Math.max(2, heading))}`;
  return `<article class="video-card">
    <div class="player">${
      playerUrl(v)
        ? `<button type="button" class="play" data-play="${esc(v.id)}" aria-label="${esc(`${l.play}: ${title}`)}">${thumb ? `<img src="${esc(thumb)}" alt="" width="480" height="360" loading="lazy">` : ""}<span class="play-icon" aria-hidden="true"></span></button>`
        : thumb
          ? `<img src="${esc(thumb)}" alt="" width="480" height="360" loading="lazy">`
          : ""
    }</div>
    <${h}>${esc(title)}</${h}>
    ${meta ? `<p class="meta">${meta}</p>` : ""}
    ${v.description ? `<p class="description">${esc(t(v.description, lang)).replace(/\n/g, "<br>")}</p>` : ""}
    ${v.credit ? `<p class="credit">${esc(v.credit)}</p>` : ""}
    ${watch ? `<p class="watch">${watch}</p>` : ""}
  </article>`;
}

const list = (s: VideoSnapshot, vs: Video[], lang: Lang, heading: number) =>
  vs.length ? `<div class="video-list">${vs.map((v) => renderVideo(s, v, lang, heading)).join("")}</div>` : "";

function groupLabel(g: VideoGroup, lang: Lang): string {
  return g.label ? t(g.label as LocalizedText, lang) : L[lang].other;
}

/** The Videos matching the state: a list, or groups. */
export function renderResults(s: VideoSnapshot, st: VideoState, lang: Lang): string {
  const l = L[lang];
  const videos = filterVideos(s, st);
  if (!s.videos.length) return `<p>${esc(l.empty)}</p>`;
  if (!videos.length) return `<p>${esc(l.none)}</p>`;
  if (!st.group) return list(s, videos, lang, 3);
  return (
    groupVideos(s, videos, st.group)
      .map(
        (g) => `<section class="video-group">
      <h3>${esc(groupLabel(g, lang))}</h3>
      ${list(s, g.videos, lang, g.subgroups?.length ? 5 : 4)}
      ${(g.subgroups ?? [])
        .map((sg) => `<section class="video-subgroup"><h4>${esc(groupLabel(sg, lang))}</h4>${list(s, sg.videos, lang, 5)}</section>`)
        .join("")}
    </section>`,
      )
      .join("")
  );
}

/** How many Videos match, for the live count. */
export const countText = (s: VideoSnapshot, st: VideoState, lang: Lang) => L[lang].count(filterVideos(s, st).length);

/** The whole page body. */
export function renderVideosPage(s: VideoSnapshot, st: VideoState, lang: Lang, intro?: Record<Lang, string>): string {
  const text = intro?.[lang];
  return `<section class="videos-page">
    <h2>${esc(L[lang].title)}</h2>
    ${text ? `<div class="intro">${text
      .split(/\n{2,}/)
      .map((p) => `<p>${esc(p)}</p>`)
      .join("")}</div>` : ""}
    ${s.videos.length ? `<form class="video-filters" role="search" data-filters>${renderControls(s, st, lang)}</form>
    <p class="count" role="status" data-count>${esc(countText(s, st, lang))}</p>` : ""}
    <div class="video-results" data-results>${renderResults(s, st, lang)}</div>
  </section>`;
}

export interface VideosOptions {
  /** The Site's own introduction (plain text; a blank line starts a paragraph). */
  intro?: Record<Lang, string>;
}

/** The /<lang>/videos page from the Video Collection snapshot at url (read once). */
export function videos(url: string, opts: VideosOptions = {}): Module {
  let snapshot: VideoSnapshot | undefined;
  return {
    async render(at: URL, lang: Lang): Promise<Page | undefined> {
      if (at.pathname.replace(/\/+$/, "") !== videosPath(lang)) return undefined;
      let s: VideoSnapshot;
      try {
        s = snapshot ??= await fetchVideos(url);
      } catch {
        throw new LoadError(L[lang].load);
      }
      const st = readState(s, at.searchParams);
      const other: Lang = lang === "pl" ? "en" : "pl";
      const siteName = t(s.organization.name, lang);
      return {
        siteName,
        title: `${L[lang].title} – ${siteName}`,
        body: renderVideosPage(s, st, lang, opts.intro),
        alternate: videosPath(other) + writeState(st),
        mount: (main) => mount(main, s, lang),
      };
    },
  };
}

/** Wires the controls and the players of a rendered page. */
function mount(main: HTMLElement, s: VideoSnapshot, lang: Lang): void {
  const form = main.querySelector<HTMLFormElement>("[data-filters]");
  const results = main.querySelector<HTMLElement>("[data-results]")!;
  const state = () => readState(s, new URL(location.href).searchParams);
  const alternate = () => document.querySelector<HTMLAnchorElement>("header a.lang");
  const apply = (st: VideoState, push: boolean) => {
    const next = videosPath(lang) + writeState(st);
    if (push) history.pushState(null, "", next);
    else history.replaceState(null, "", next);
    const alt = alternate();
    if (alt) alt.href = videosPath(lang === "pl" ? "en" : "pl") + writeState(st);
    results.innerHTML = renderResults(s, st, lang);
    const count = main.querySelector<HTMLElement>("[data-count]");
    if (count) count.textContent = countText(s, st, lang);
  };
  /** Redraws the controls, keeping focus on the control that had it. */
  const redraw = (st: VideoState) => {
    if (!form) return;
    const active = document.activeElement as HTMLElement | null;
    const key = active && form.contains(active) ? (active.dataset.form ? `[data-form="${CSS.escape(active.dataset.form)}"]` : active.dataset.control ? `[data-control="${active.dataset.control}"]` : "") : "";
    form.innerHTML = renderControls(s, st, lang);
    if (key) (form.querySelector<HTMLElement>(key) ?? form.querySelector<HTMLElement>("[data-control=country]"))?.focus();
  };
  form?.addEventListener("submit", (e) => e.preventDefault());
  form?.addEventListener("click", (e) => {
    const el = (e.target as HTMLElement).closest<HTMLElement>("button");
    if (!el) return;
    const st = state();
    if (el.dataset.form) {
      const id = el.dataset.form;
      st.formIds = st.formIds.includes(id) ? st.formIds.filter((x) => x !== id) : [...st.formIds, id];
    } else if (el.dataset.control === "clear") {
      st.formIds = [];
      st.originId = undefined;
      st.query = "";
      st.group = undefined;
    } else return;
    apply(st, true);
    redraw(st);
  });
  form?.addEventListener("change", (e) => {
    const el = e.target as HTMLSelectElement;
    const st = state();
    switch (el.dataset.control) {
      case "country":
      case "region": {
        const country = form.querySelector<HTMLSelectElement>("[data-control=country]")?.value ?? "";
        const region = el.dataset.control === "region" ? el.value : "";
        st.originId = region || country || undefined;
        break;
      }
      case "group":
        st.group = el.value === "form" || el.value === "origin" ? el.value : undefined;
        break;
      default:
        return;
    }
    apply(st, true);
    redraw(st);
  });
  form?.addEventListener("input", (e) => {
    const el = e.target as HTMLInputElement;
    if (el.dataset.control !== "query") return;
    const st = state();
    st.query = el.value.slice(0, 100);
    apply(st, false);
    const clear = form.querySelector<HTMLButtonElement>("[data-control=clear]");
    if (clear) clear.disabled = !(st.formIds.length || st.originId || st.query || st.group);
  });
  // Click-to-load: the player (and its cookies) loads only on request.
  results.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>("button[data-play]");
    const v = btn && s.videos.find((x) => x.id === btn.dataset.play);
    const src = v && playerUrl(v);
    if (!btn || !v || !src) return;
    const frame = document.createElement("iframe");
    frame.src = src;
    frame.title = t(v.title, lang);
    frame.allow = "autoplay; fullscreen; picture-in-picture; encrypted-media";
    frame.allowFullscreen = true;
    frame.referrerPolicy = "strict-origin-when-cross-origin";
    btn.replaceWith(frame);
    frame.focus();
  });
}
