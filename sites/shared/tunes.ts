// The tunes() page module: the Tune Collection on a Site.
//   /<lang>/tunes               the catalogue: search, Tune Type / Key / Origin /
//                               Instrument filters and Group by, all in the query
//                               string (?q=…&type=a,b&tonic=E&mode=minor&origin=x
//                               &instrument=y&fit=nottricky&group=key), so a view
//                               can be shared and Back/Forward work; each row
//                               shows its Incipit (drawn when it scrolls into
//                               view) and plays it; ticked Tunes print together.
//   /<lang>/tunes/<id>-<slug>   a Tune: sheet, playback, details, Recordings,
//                               Attachments, Sets, Share and Print.
//   /<lang>/sets/<id>-<slug>    a Set: its Tunes in order, with sheets, play, print.
// Snapshot text is escaped (esc); the one exception is a Tune's description,
// which the CMS renders to safe HTML (PLAN decision 12). abcjs (ADR 0013) loads
// only on these pages; the samples are served by the Site under /sounds/.

import { httpsUrl, t, type Lang } from "../../contract/client/src/index.ts";
import {
  attachmentVariant,
  bi,
  fetchTune,
  fetchTunes,
  filterTunes,
  groupTunes,
  instruments,
  keyLabel,
  MODES,
  nameTranslation,
  recordingPlayerUrl,
  recordingThumbnail,
  setPath,
  setTitle,
  setTunes,
  tuneOriginLabel,
  tuneOriginPath,
  tunePath,
  tuneType,
  withTempo,
  type Attachment,
  type CatalogueTune,
  type Mode,
  type Recording,
  type TempoBeat,
  type TuneBase,
  type TuneFile,
  type TuneFilter,
  type TuneGroup,
  type TuneGroupBy,
  type TuneSet,
  type TunesSnapshot,
} from "../../contract/client/src/tunes.ts";
import { INSTRUMENTS, loadAbcjs, SOUNDFONT_CREDIT, synthOptions, type Instrument, type MidiBuffer } from "../../notation/abcjs.ts";
import { esc, LoadError, type Module, type Page } from "./site.ts";

const L = {
  pl: {
    title: "Melodie",
    intro: "Melodie, które gramy na sesjach. Wyszukaj melodię po nazwie albo wybierz rodzaj, tonację, pochodzenie lub instrument.",
    search: "Szukaj w nazwach",
    types: "Rodzaj",
    tonic: "Tonika",
    anyTonic: "Każda",
    mode: "Skala",
    anyMode: "Każda",
    origin: "Pochodzenie",
    anyOrigin: "Wszystkie",
    instrument: "Instrument",
    anyInstrument: "Każdy",
    fit: "Melodia",
    fitSuits: "pasuje do niego",
    fitNotTricky: "nie jest na nim trudna",
    group: "Grupuj",
    groupNone: "bez grupowania",
    groupKey: "według tonacji",
    groupType: "według rodzaju",
    groupOrigin: "według pochodzenia",
    clear: "Wyczyść filtry",
    other: "Inne",
    none: "Brak melodii dla wybranych filtrów.",
    empty: "Wkrótce pojawią się tu melodie.",
    count: (n: number) => `${n} ${n === 1 ? "melodia" : [2, 3, 4].includes(n % 10) && ![12, 13, 14].includes(n % 100) ? "melodie" : "melodii"}`,
    pick: (name: string) => `Zaznacz do druku: ${name}`,
    print: "Drukuj / zapisz jako PDF",
    printPicked: (n: number) => `Drukuj / zapisz jako PDF (${n})`,
    printHint: "Zaznacz melodie, aby je wydrukować.",
    printing: "Przygotowuję wydruk…",
    printFailed: "Nie udało się przygotować wydruku.",
    sound: "Brzmienie",
    sounds: { piano: "fortepian", flute: "flet", fiddle: "skrzypce", accordion: "akordeon" } as Record<Instrument, string>,
    play: "Odtwórz",
    stop: "Zatrzymaj",
    playIncipit: (name: string) => `Odtwórz początek: ${name}`,
    playTune: (name: string) => `Odtwórz: ${name}`,
    playRecording: "Odtwórz nagranie",
    listen: "Posłuchaj",
    noAudio: "Ta przeglądarka nie odtwarza dźwięku.",
    incipit: (name: string) => `Początek melodii ${name}`,
    sheet: (name: string) => `Nuty: ${name}`,
    tempo: "Tempo",
    slower: "Wolniej",
    faster: "Szybciej",
    tempoValue: (pct: number, bpm: string) => `${pct}% (${bpm})`,
    facts: { type: "Rodzaj", key: "Tonacja", meter: "Metrum", tempo: "Tempo odtwarzania", origin: "Pochodzenie", composer: "Kompozytor", suits: "Pasuje do", tricky: "Trudna na" },
    alsoKnown: "Inne nazwy",
    noTranscription: "Ta melodia nie ma jeszcze zapisu nutowego.",
    recordings: "Nagrania",
    watchOn: { youtube: "Obejrzyj na YouTube", vimeo: "Obejrzyj na Vimeo" },
    attachments: "Załączniki",
    openPdf: "Otwórz PDF",
    sets: "Zestawy",
    inSets: "W zestawach",
    share: "Udostępnij",
    copied: "Skopiowano link.",
    copyFailed: "Nie udało się skopiować linku.",
    back: "Wszystkie melodie",
    setTitle: "Zestaw",
    tunesInSet: "Melodie w zestawie",
    notFound: "Nie znaleziono melodii.",
    setNotFound: "Nie znaleziono zestawu.",
    load: "Nie udało się wczytać melodii.",
    credit: "Dźwięki instrumentów: FluidR3 GM, Frank Wen, CC BY 3.0",
  },
  en: {
    title: "Tunes",
    intro: "The tunes we play at our sessions. Search by name, or choose a tune type, key, origin or instrument.",
    search: "Search names",
    types: "Tune type",
    tonic: "Tonic",
    anyTonic: "Any",
    mode: "Mode",
    anyMode: "Any",
    origin: "Origin",
    anyOrigin: "All",
    instrument: "Instrument",
    anyInstrument: "Any",
    fit: "Tune",
    fitSuits: "suits it",
    fitNotTricky: "is not tricky on it",
    group: "Group",
    groupNone: "no grouping",
    groupKey: "by key",
    groupType: "by tune type",
    groupOrigin: "by origin",
    clear: "Clear filters",
    other: "Other",
    none: "No tunes match the filters.",
    empty: "Tunes are coming soon.",
    count: (n: number) => `${n} ${n === 1 ? "tune" : "tunes"}`,
    pick: (name: string) => `Select for printing: ${name}`,
    print: "Print / Save as PDF",
    printPicked: (n: number) => `Print / Save as PDF (${n})`,
    printHint: "Tick tunes to print them.",
    printing: "Preparing the print…",
    printFailed: "Could not prepare the print.",
    sound: "Sound",
    sounds: { piano: "piano", flute: "flute", fiddle: "fiddle", accordion: "accordion" } as Record<Instrument, string>,
    play: "Play",
    stop: "Stop",
    playIncipit: (name: string) => `Play the opening: ${name}`,
    playTune: (name: string) => `Play: ${name}`,
    playRecording: "Play recording",
    listen: "Listen",
    noAudio: "This browser cannot play sound.",
    incipit: (name: string) => `Opening bars of ${name}`,
    sheet: (name: string) => `Sheet music: ${name}`,
    tempo: "Tempo",
    slower: "Slower",
    faster: "Faster",
    tempoValue: (pct: number, bpm: string) => `${pct}% (${bpm})`,
    facts: { type: "Tune type", key: "Key", meter: "Time signature", tempo: "Playback tempo", origin: "Origin", composer: "Composer", suits: "Suits", tricky: "Tricky on" },
    alsoKnown: "Also known as",
    noTranscription: "This tune has no transcription yet.",
    recordings: "Recordings",
    watchOn: { youtube: "Watch on YouTube", vimeo: "Watch on Vimeo" },
    attachments: "Attachments",
    openPdf: "Open PDF",
    sets: "Sets",
    inSets: "In sets",
    share: "Share",
    copied: "Link copied.",
    copyFailed: "Could not copy the link.",
    back: "All tunes",
    setTitle: "Set",
    tunesInSet: "Tunes in the set",
    notFound: "Tune not found.",
    setNotFound: "Set not found.",
    load: "Could not load the tunes.",
    credit: SOUNDFONT_CREDIT,
  },
};

const other = (lang: Lang): Lang => (lang === "pl" ? "en" : "pl");
const SOUNDS = Object.keys(INSTRUMENTS) as Instrument[];
const SOUND_FONT_URL = "/sounds/";
const LICENSE_URL = "https://creativecommons.org/licenses/by/3.0/";

// --- Routes ----------------------------------------------------------------

export type TuneRoute = { name: "tunes" } | { name: "tune"; id: string; slug: string } | { name: "set"; id: string; slug: string };

/** The path of the catalogue. */
export const tunesPath = (lang: Lang) => `/${lang}/tunes`;

/** The Tune Collection route of a path in `lang`, or undefined when it is not one. */
export function matchTunes(pathname: string, lang: Lang): TuneRoute | undefined {
  let parts: string[];
  try {
    parts = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  } catch {
    return undefined;
  }
  if (parts[0] !== lang) return undefined;
  if (parts[1] === "tunes" && parts.length === 2) return { name: "tunes" };
  if ((parts[1] === "tunes" || parts[1] === "sets") && parts.length === 3) {
    const m = /^([a-z2-7]{8})(?:-(.*))?$/.exec(parts[2]!);
    if (m) return { name: parts[1] === "tunes" ? "tune" : "set", id: m[1]!, slug: m[2] ?? "" };
  }
  return undefined;
}

// --- Catalogue state (query string) ----------------------------------------

export interface TuneState {
  query: string;
  typeIds: string[];
  tonic?: string;
  mode?: Mode;
  originId?: string;
  instrumentId?: string;
  /** Only with instrumentId. */
  fit: "suits" | "notTricky";
  group?: TuneGroupBy;
}

/** Reads the state from a query string; unknown ids and values are dropped. */
export function readState(s: TunesSnapshot, search: URLSearchParams): TuneState {
  const types = new Set(s.types.map((x) => x.id));
  const tonics = new Set(s.tunes.map((x) => x.key.tonic));
  const mode = search.get("mode") ?? "";
  const origin = search.get("origin") ?? "";
  const instrument = search.get("instrument") ?? "";
  const known = s.instruments.some((i) => i.id === instrument);
  const group = search.get("group");
  const tonic = search.get("tonic") ?? "";
  return {
    query: (search.get("q") ?? "").slice(0, 100),
    typeIds: [...new Set((search.get("type") ?? "").split(",").filter((id) => types.has(id)))],
    tonic: tonics.has(tonic) ? tonic : undefined,
    mode: (MODES as readonly string[]).includes(mode) ? (mode as Mode) : undefined,
    originId: tuneOriginPath(s, origin) ? origin : undefined,
    instrumentId: known ? instrument : undefined,
    fit: known && search.get("fit") === "nottricky" ? "notTricky" : "suits",
    group: group === "key" || group === "type" || group === "origin" ? group : undefined,
  };
}

/** Writes the state as a query string ("" when nothing is chosen). */
export function writeState(st: TuneState): string {
  const q = new URLSearchParams();
  if (st.query.trim()) q.set("q", st.query);
  if (st.typeIds.length) q.set("type", st.typeIds.join(","));
  if (st.tonic) q.set("tonic", st.tonic);
  if (st.mode) q.set("mode", st.mode);
  if (st.originId) q.set("origin", st.originId);
  if (st.instrumentId) {
    q.set("instrument", st.instrumentId);
    if (st.fit === "notTricky") q.set("fit", "nottricky");
  }
  if (st.group) q.set("group", st.group);
  const s = q.toString();
  return s ? `?${s}` : "";
}

/** The reader's filter for a state. */
export function toFilter(st: TuneState): TuneFilter {
  return {
    query: st.query,
    typeIds: st.typeIds,
    tonic: st.tonic,
    mode: st.mode,
    originId: st.originId,
    instrument: st.instrumentId ? { id: st.instrumentId, match: st.fit } : undefined,
  };
}

const isFiltered = (st: TuneState) => !!(st.query || st.typeIds.length || st.tonic || st.mode || st.originId || st.instrumentId || st.group);

// --- Small renderers ---------------------------------------------------------

const option = (value: string, label: string, selected: boolean) =>
  `<option value="${esc(value)}"${selected ? " selected" : ""}>${esc(label)}</option>`;

const BEAT_SIGNS: Partial<Record<TempoBeat, string>> = { "1/4": "♩", "3/8": "♩.", "1/8": "♪" };

/** A Playback Tempo as text: "♩. = 110". */
export function tempoText(tempo: number, beat: TempoBeat): string {
  return `${BEAT_SIGNS[beat] ?? beat} = ${Math.round(tempo)}`;
}

/** A file size: "118 KB", "1,2 MB". */
export function sizeText(bytes: number, lang: Lang): string {
  const nf = new Intl.NumberFormat(lang === "pl" ? "pl-PL" : "en-GB", { maximumFractionDigits: 1 });
  if (bytes < 1024 * 1024) return `${nf.format(Math.max(1, Math.round(bytes / 1024)))} KB`;
  return `${nf.format(bytes / 1024 / 1024)} MB`;
}

const MODE_NAMES: Record<Lang, Record<Mode, string>> = {
  pl: { major: "dur", minor: "moll", dorian: "dorycka", mixolydian: "miksolidyjska", lydian: "lidyjska", phrygian: "frygijska", locrian: "lokrycka" },
  en: { major: "major", minor: "minor", dorian: "dorian", mixolydian: "mixolydian", lydian: "lydian", phrygian: "phrygian", locrian: "locrian" },
};

const typeLabel = (s: TunesSnapshot, tune: TuneBase, lang: Lang) => bi(tuneType(s, tune)?.label, lang);

/** The details shown with a Tune in a list: Tune Type · Key · Time Signature · Origin. */
function metaLine(s: TunesSnapshot, tune: TuneBase, lang: Lang): string {
  return [typeLabel(s, tune, lang), keyLabel(tune.key, lang), tune.meter, tuneOriginLabel(s, tune.originId, lang)].filter(Boolean).map(esc).join(" · ");
}

/** The translation of the name and the Other Names, for a list row. */
function namesLine(tune: TuneBase, lang: Lang): string {
  const tr = nameTranslation(tune.name, lang);
  return [tr ? `(${tr})` : "", ...tune.otherNames.map((n) => n.name)].filter(Boolean).map(esc).join(" · ");
}

const heading = (level: number) => `h${Math.min(6, Math.max(2, level))}`;

function soundSelect(lang: Lang, current: Instrument): string {
  return `<label class="sound">${esc(L[lang].sound)} <select data-control="sound">${SOUNDS.map((x) => option(x, L[lang].sounds[x], x === current)).join("")}</select></label>`;
}

function credit(lang: Lang): string {
  return `<p class="credit"><a href="${LICENSE_URL}" rel="license noopener">${esc(L[lang].credit)}</a></p>`;
}

/** How a Tune without a Transcription is heard: its first Recording. */
function recordingAction(r: Recording | undefined, tuneId: string, lang: Lang): string {
  if (!r) return "";
  if (recordingPlayerUrl(r)) return `<button type="button" class="play" data-play-recording="${esc(tuneId)}">${esc(L[lang].playRecording)}</button>`;
  const url = httpsUrl(r.url);
  if (!url) return "";
  const host = r.kind === "link" ? r.host : new URL(url).hostname;
  return `<a class="listen" href="${esc(url)}" target="_blank" rel="noopener">${esc(L[lang].listen)} (${esc(host)})</a>`;
}

// --- Catalogue ---------------------------------------------------------------

/** The filter and grouping controls; only Tune Types, Keys, Origins and Instruments in use are offered. */
export function renderControls(s: TunesSnapshot, st: TuneState, lang: Lang): string {
  const l = L[lang];
  const used = <T>(f: (t: CatalogueTune) => T) => new Set(s.tunes.map(f));
  const typeIds = used((x) => x.typeId);
  const types = s.types.filter((x) => typeIds.has(x.id));
  const tonicsUsed = used((x) => x.key.tonic);
  const tonics = ["C", "C#", "Db", "D", "D#", "Eb", "E", "F", "F#", "Gb", "G", "G#", "Ab", "A", "A#", "Bb", "B"].filter((x) => tonicsUsed.has(x));
  const modesUsed = used((x) => x.key.mode);
  const modes = MODES.filter((m) => modesUsed.has(m));
  const originIds = used((x) => x.originId ?? "");
  const origins = s.origins.flatMap((c) => {
    const regions = c.regions.filter((r) => originIds.has(r.id));
    return originIds.has(c.id) || regions.length
      ? [option(c.id, bi(c.label, lang), st.originId === c.id), ...regions.map((r) => option(r.id, `– ${bi(r.label, lang)}`, st.originId === r.id))]
      : [];
  });
  const instIds = new Set(s.tunes.flatMap((x) => [...x.suits, ...x.tricky]));
  const insts = s.instruments.filter((i) => instIds.has(i.id));
  const tonicName = (x: string) => keyLabel({ tonic: x, mode: "major" }, lang).replace(/-dur$| major$/, "");
  const modeName = (m: Mode) => MODE_NAMES[lang][m];
  return `${types.length ? `<fieldset class="chips"><legend>${esc(l.types)}</legend>${types
    .map((x) => `<button type="button" class="chip" data-type="${esc(x.id)}" aria-pressed="${st.typeIds.includes(x.id)}">${esc(bi(x.label, lang))}</button>`)
    .join("")}</fieldset>` : ""}
    <div class="selects">
      <label>${esc(l.search)} <input type="search" data-control="query" value="${esc(st.query)}" maxlength="100" autocomplete="off"></label>
      <label>${esc(l.tonic)} <select data-control="tonic">${option("", l.anyTonic, !st.tonic)}${tonics.map((x) => option(x, tonicName(x), st.tonic === x)).join("")}</select></label>
      <label>${esc(l.mode)} <select data-control="mode">${option("", l.anyMode, !st.mode)}${modes.map((m) => option(m, modeName(m), st.mode === m)).join("")}</select></label>
      ${origins.length ? `<label>${esc(l.origin)} <select data-control="origin">${option("", l.anyOrigin, !st.originId)}${origins.join("")}</select></label>` : ""}
      ${insts.length ? `<label>${esc(l.instrument)} <select data-control="instrument">${option("", l.anyInstrument, !st.instrumentId)}${insts
        .map((i) => option(i.id, bi(i.label, lang), st.instrumentId === i.id))
        .join("")}</select></label>
      <label>${esc(l.fit)} <select data-control="fit"${st.instrumentId ? "" : " disabled"}>${option("suits", l.fitSuits, st.fit === "suits")}${option("nottricky", l.fitNotTricky, st.fit === "notTricky")}</select></label>` : ""}
      <label>${esc(l.group)} <select data-control="group">${option("", l.groupNone, !st.group)}${option("key", l.groupKey, st.group === "key")}${option("type", l.groupType, st.group === "type")}${
        origins.length ? option("origin", l.groupOrigin, st.group === "origin") : ""
      }</select></label>
      <button type="button" class="secondary" data-control="clear"${isFiltered(st) ? "" : " disabled"}>${esc(l.clear)}</button>
    </div>`;
}

/** One catalogue row: tick box, name and details, the Incipit and its play button. */
export function renderRow(s: TunesSnapshot, tune: CatalogueTune, lang: Lang, level: number, picked: boolean): string {
  const l = L[lang];
  const h = heading(level);
  const names = namesLine(tune, lang);
  const play = tune.incipit
    ? `<button type="button" class="play" data-play-incipit="${esc(tune.id)}" aria-label="${esc(l.playIncipit(tune.name.name))}">${esc(l.play)}</button>`
    : recordingAction(tune.firstRecording, tune.id, lang);
  return `<li class="tune-row" data-tune="${esc(tune.id)}">
    <input type="checkbox" class="pick" data-pick="${esc(tune.id)}" aria-label="${esc(l.pick(tune.name.name))}"${picked ? " checked" : ""}>
    <div class="tune-info">
      <${h}><a href="${esc(tunePath(lang, tune.id, tune.slug))}">${esc(tune.name.name)}</a></${h}>
      ${names ? `<p class="names">${names}</p>` : ""}
      <p class="meta">${metaLine(s, tune, lang)}</p>
    </div>
    ${tune.incipit ? `<div class="incipit" data-incipit="${esc(tune.id)}" role="img" aria-label="${esc(l.incipit(tune.name.name))}"></div>` : `<div class="incipit"></div>`}
    <div class="tune-play" data-player="${esc(tune.id)}">${play}</div>
  </li>`;
}

const rows = (s: TunesSnapshot, ts: CatalogueTune[], lang: Lang, level: number, picked: ReadonlySet<string>) =>
  ts.length ? `<ul class="tune-list">${ts.map((x) => renderRow(s, x, lang, level, picked.has(x.id))).join("")}</ul>` : "";

function groupLabel(g: TuneGroup, lang: Lang): string {
  if (g.key) return keyLabel(g.key, lang);
  return g.label ? bi(g.label, lang) : L[lang].other;
}

/** The Tunes matching the state: a list, or groups. */
export function renderResults(s: TunesSnapshot, st: TuneState, lang: Lang, picked: ReadonlySet<string> = new Set()): string {
  const l = L[lang];
  if (!s.tunes.length) return `<p>${esc(l.empty)}</p>`;
  const tunes = filterTunes(s, s.tunes, toFilter(st));
  if (!tunes.length) return `<p>${esc(l.none)}</p>`;
  if (!st.group) return rows(s, tunes, lang, 3, picked);
  return groupTunes(s, tunes, st.group)
    .map(
      (g) => `<section class="tune-group">
      <h3>${esc(groupLabel(g, lang))}</h3>
      ${rows(s, g.tunes, lang, g.subgroups?.length ? 5 : 4, picked)}
      ${(g.subgroups ?? []).map((sg) => `<section class="tune-subgroup"><h4>${esc(groupLabel(sg, lang))}</h4>${rows(s, sg.tunes, lang, 5, picked)}</section>`).join("")}
    </section>`,
    )
    .join("");
}

export const countText = (s: TunesSnapshot, st: TuneState, lang: Lang) => L[lang].count(filterTunes(s, s.tunes, toFilter(st)).length);

function printButton(lang: Lang, n: number): string {
  return `<button type="button" class="primary" data-control="print"${n ? "" : " disabled"}>${esc(n ? L[lang].printPicked(n) : L[lang].print)}</button>`;
}

/** The whole catalogue page. */
export function renderCatalogue(s: TunesSnapshot, st: TuneState, lang: Lang, picked: ReadonlySet<string> = new Set(), sound: Instrument = "fiddle"): string {
  const l = L[lang];
  const sets = s.sets.filter((x) => setTunes(s, x).length);
  return `<section class="tunes-page">
    <h2>${esc(l.title)}</h2>
    <p class="intro">${esc(l.intro)}</p>
    ${s.tunes.length ? `<form class="tune-filters" role="search" data-filters>${renderControls(s, st, lang)}</form>
    <div class="tune-toolbar">
      <p class="count" role="status" data-count>${esc(countText(s, st, lang))}</p>
      ${soundSelect(lang, sound)}
      ${printButton(lang, picked.size)}
      <p class="hint" data-status role="status">${picked.size ? "" : esc(l.printHint)}</p>
    </div>` : ""}
    <div class="tune-results" data-results>${renderResults(s, st, lang, picked)}</div>
    ${sets.length ? `<section class="tune-sets"><h3>${esc(l.sets)}</h3><ul>${sets
      .map((x) => `<li><a href="${esc(setPath(lang, x.id, x.slug))}">${esc(setTitle(s, x, lang))}</a></li>`)
      .join("")}</ul></section>` : ""}
    ${s.tunes.some((x) => x.incipit) ? credit(lang) : ""}
  </section>`;
}

// --- A Tune's page -----------------------------------------------------------

function fact(label: string, value: string): string {
  return value ? `<div><dt>${esc(label)}</dt><dd>${esc(value)}</dd></div>` : "";
}

/** The key facts (also printed): name, Tune Type, Key, Time Signature, Composer, Origin. */
export function printFacts(s: TunesSnapshot, tune: TuneBase, lang: Lang): string {
  const f = L[lang].facts;
  return [
    [f.type, typeLabel(s, tune, lang)],
    [f.key, keyLabel(tune.key, lang)],
    [f.meter, tune.meter],
    [f.composer, tune.composer ?? ""],
    [f.origin, tuneOriginLabel(s, tune.originId, lang)],
  ]
    .filter(([, v]) => v)
    .map(([k, v]) => `${esc(k)}: ${esc(v)}`)
    .join(" · ");
}

function renderRecording(r: Recording, i: number, name: string, lang: Lang): string {
  const l = L[lang];
  const url = httpsUrl(r.url);
  if (r.kind === "video") {
    const thumb = recordingThumbnail(r);
    const label = r.label ? `<p class="label">${esc(r.label)}</p>` : "";
    const watch = url ? `<a href="${esc(url)}" target="_blank" rel="noopener">${esc(l.watchOn[r.provider] ?? r.url)}</a>` : "";
    const player = recordingPlayerUrl(r)
      ? `<button type="button" class="play-video" data-recording="${i}" aria-label="${esc(`${l.play}: ${r.label || name}`)}">${thumb ? `<img src="${esc(thumb)}" alt="" width="480" height="360" loading="lazy">` : ""}<span class="play-icon" aria-hidden="true"></span></button>`
      : thumb
        ? `<img src="${esc(thumb)}" alt="" width="480" height="360" loading="lazy">`
        : "";
    return `<li class="recording video"><div class="player">${player}</div>${label}${watch ? `<p class="watch">${watch}</p>` : ""}</li>`;
  }
  if (!url) return "";
  return `<li class="recording link"><a href="${esc(url)}" target="_blank" rel="noopener">${esc(r.label || r.host)}</a> <span class="host">(${esc(r.host)})</span></li>`;
}

const mediaUrl = (u: string | undefined) => (u && /^https?:\/\//i.test(u) ? u : undefined);

function renderAttachment(a: Attachment, lang: Lang): string {
  const caption = bi(a.caption, lang);
  if (a.kind === "image") {
    const variants = (a.variants ?? []).filter((v) => mediaUrl(v.url));
    const v = attachmentVariant({ ...a, variants }, 960);
    if (!v) return "";
    const srcset = variants.map((x) => `${esc(x.url)} ${esc(x.width)}w`).join(", ");
    return `<figure class="attachment image"><img src="${esc(v.url)}" srcset="${srcset}" sizes="(min-width: 960px) 960px, 100vw" width="${esc(v.width)}" height="${esc(v.height)}" alt="${esc(caption)}" loading="lazy">${
      caption ? `<figcaption>${esc(caption)}</figcaption>` : ""
    }</figure>`;
  }
  const url = mediaUrl(a.url);
  if (!url) return "";
  return `<figure class="attachment pdf">
      <div class="pdf-frame" data-pdf="${esc(url)}" data-pdf-title="${esc(caption || a.name)}"></div>
      <a class="pdf-card" href="${esc(url)}" target="_blank" rel="noopener"><span class="pdf-open">${esc(L[lang].openPdf)}</span> <span class="pdf-name">${esc(a.name)}</span> <span class="pdf-size">${esc(sizeText(a.size, lang))}</span></a>
      ${caption ? `<figcaption>${esc(caption)}</figcaption>` : ""}
    </figure>`;
}

/** The playback controls: play, sound and (on a Tune's page) tempo. */
function playback(lang: Lang, tune: TuneBase, sound: Instrument, withTempoControl: boolean): string {
  const l = L[lang];
  return `<div class="playback">
      <button type="button" class="play primary" data-play-tune="${esc(tune.id)}" aria-label="${esc(l.playTune(tune.name.name))}">${esc(l.play)}</button>
      ${soundSelect(lang, sound)}
      ${withTempoControl ? `<span class="tempo">${esc(l.tempo)}
        <button type="button" class="secondary" data-tempo="-10">${esc(l.slower)}</button>
        <output data-tempo-value>${esc(l.tempoValue(100, tempoText(tune.tempo, tune.tempoBeat)))}</output>
        <button type="button" class="secondary" data-tempo="10">${esc(l.faster)}</button></span>` : ""}
    </div>`;
}

/** A Tune's page. */
export function renderTune(s: TunesSnapshot, tune: TuneFile, lang: Lang, sound: Instrument = "fiddle"): string {
  const l = L[lang];
  const f = l.facts;
  const tr = nameTranslation(tune.name, lang);
  const others = tune.otherNames.map((n) => {
    const x = nameTranslation(n, lang);
    return `<li>${esc(n.name)}${x ? ` <span class="translation">(${esc(x)})</span>` : ""}</li>`;
  });
  const inst = (ids: string[]) => instruments(s, ids).map((i) => bi(i.label, lang)).join(", ");
  const description = bi(tune.descriptionHtml, lang);
  const recordings = tune.recordings.map((r, i) => renderRecording(r, i, tune.name.name, lang)).filter(Boolean);
  const attachments = tune.attachments.map((a) => renderAttachment(a, lang)).filter(Boolean);
  const sets = tune.sets.filter((x) => setTunes(s, x).length);
  return `<article class="tune-page" data-tune-page="${esc(tune.id)}">
    <h2>${esc(tune.name.name)}${tr ? ` <span class="translation">(${esc(tr)})</span>` : ""}</h2>
    ${others.length ? `<div class="other-names"><h3>${esc(l.alsoKnown)}</h3><ul>${others.join("")}</ul></div>` : ""}
    <dl class="facts">
      ${fact(f.type, typeLabel(s, tune, lang))}
      ${fact(f.key, keyLabel(tune.key, lang))}
      ${fact(f.meter, tune.meter)}
      ${tune.transcription ? fact(f.tempo, tempoText(tune.tempo, tune.tempoBeat)) : ""}
      ${fact(f.origin, tuneOriginLabel(s, tune.originId, lang))}
      ${fact(f.composer, tune.composer ?? "")}
      ${fact(f.suits, inst(tune.suits))}
      ${fact(f.tricky, inst(tune.tricky))}
    </dl>
    <div class="tune-actions">
      <button type="button" class="secondary" data-control="share">${esc(l.share)}</button>
      <button type="button" class="secondary" data-control="print">${esc(l.print)}</button>
      <p class="status" data-status role="status"></p>
    </div>
    ${tune.transcription ? `<section class="sheet-section">
      <div class="sheet" data-sheet="${esc(tune.id)}" role="img" aria-label="${esc(l.sheet(tune.name.name))}"></div>
      ${playback(lang, tune, sound, true)}
      ${credit(lang)}
    </section>` : `<p class="no-transcription">${esc(l.noTranscription)}</p>`}
    ${description ? `<div class="description">${description}</div>` : ""}
    ${recordings.length ? `<section class="recordings"><h3>${esc(l.recordings)}</h3><ul>${recordings.join("")}</ul></section>` : ""}
    ${attachments.length ? `<section class="attachments"><h3>${esc(l.attachments)}</h3>${attachments.join("")}</section>` : ""}
    ${sets.length ? `<section class="tune-sets"><h3>${esc(l.inSets)}</h3><ul>${sets
      .map((x) => `<li><a href="${esc(setPath(lang, x.id, x.slug))}">${esc(setTitle(s, x, lang))}</a></li>`)
      .join("")}</ul></section>` : ""}
    <p class="back"><a href="${esc(tunesPath(lang))}">${esc(l.back)}</a></p>
  </article>`;
}

// --- A Set's page ------------------------------------------------------------

/** A Set's page: its Tunes in order, each with its sheet and play button. */
export function renderSet(s: TunesSnapshot, set: TuneSet, files: TuneFile[], lang: Lang, sound: Instrument = "fiddle"): string {
  const l = L[lang];
  const tunes = setTunes(s, set);
  return `<article class="set-page">
    <h2>${esc(setTitle(s, set, lang))}</h2>
    <div class="tune-actions">
      ${soundSelect(lang, sound)}
      <button type="button" class="secondary" data-control="print">${esc(l.print)}</button>
      <p class="status" data-status role="status"></p>
    </div>
    <ol class="set-tunes">${tunes
      .map((tune) => {
        const file = files.find((x) => x.id === tune.id);
        return `<li class="set-tune">
        <h3><a href="${esc(tunePath(lang, tune.id, tune.slug))}">${esc(tune.name.name)}</a></h3>
        <p class="meta">${metaLine(s, tune, lang)}</p>
        ${file?.transcription
          ? `<div class="sheet" data-sheet="${esc(tune.id)}" role="img" aria-label="${esc(l.sheet(tune.name.name))}"></div>
        <div class="playback"><button type="button" class="play" data-play-tune="${esc(tune.id)}" aria-label="${esc(l.playTune(tune.name.name))}">${esc(l.play)}</button></div>`
          : `<p class="no-transcription">${esc(l.noTranscription)}</p><div class="tune-play" data-player="${esc(tune.id)}">${recordingAction(tune.firstRecording, tune.id, lang)}</div>`}
      </li>`;
      })
      .join("")}</ol>
    ${files.some((x) => x.transcription) ? credit(lang) : ""}
    <p class="back"><a href="${esc(tunesPath(lang))}">${esc(l.back)}</a></p>
  </article>`;
}

// --- Print layout --------------------------------------------------------------

/** One Tune in the print layout: name, key facts, then its sheet (drawn by mount). */
export function renderPrintTune(s: TunesSnapshot, tune: TuneBase, hasSheet: boolean, lang: Lang): string {
  return `<section class="print-tune">
    <h2>${esc(tune.name.name)}</h2>
    <p class="print-facts">${printFacts(s, tune, lang)}</p>
    ${hasSheet ? `<div class="print-sheet" data-print-sheet="${esc(tune.id)}"></div>` : `<p>${esc(L[lang].noTranscription)}</p>`}
  </section>`;
}

// --- Module ------------------------------------------------------------------

export interface TunesOptions {
  /** Where the sound select starts. */
  sound?: Instrument;
}

/** The Tune Collection pages from the snapshots under base (…/pub/sites/<site>/). */
export function tunes(base: string, opts: TunesOptions = {}): Module {
  let catalogue: Promise<TunesSnapshot> | undefined;
  const files = new Map<string, Promise<TuneFile>>();
  const ctx: Ctx = {
    sound: opts.sound ?? "fiddle",
    picked: new Set(),
    tuneFile(tune) {
      const k = `${tune.id}@${tune.rev}`;
      let p = files.get(k);
      if (!p) {
        p = fetchTune(base, tune.id, { rev: tune.rev });
        p.catch(() => files.delete(k));
        files.set(k, p);
      }
      return p;
    },
  };
  return {
    async render(url: URL, lang: Lang): Promise<Page | undefined> {
      player.stop();
      const route = matchTunes(url.pathname.replace(/(.)\/+$/, "$1"), lang);
      if (!route) return undefined;
      let s: TunesSnapshot;
      try {
        catalogue ??= fetchTunes(base);
        catalogue.catch(() => (catalogue = undefined));
        s = await catalogue;
      } catch {
        throw new LoadError(L[lang].load);
      }
      const siteName = t(s.organization.name, lang);
      const l = L[lang];
      if (route.name === "tunes") {
        const st = readState(s, url.searchParams);
        return {
          siteName,
          title: `${l.title} – ${siteName}`,
          body: renderCatalogue(s, st, lang, ctx.picked, ctx.sound),
          alternate: tunesPath(other(lang)) + writeState(st),
          mount: (main) => mountCatalogue(main, s, lang, ctx),
        };
      }
      if (route.name === "tune") {
        const tune = s.tunes.find((x) => x.id === route.id);
        if (!tune) return { siteName, title: `${l.notFound} – ${siteName}`, body: `<p>${esc(l.notFound)}</p>` };
        let file: TuneFile;
        try {
          file = await ctx.tuneFile(tune);
        } catch {
          throw new LoadError(l.load);
        }
        // The slug is cosmetic: fix the address bar if it changed.
        const want = tunePath(lang, tune.id, tune.slug);
        if (url.pathname !== want) history.replaceState(null, "", want);
        return {
          siteName,
          title: `${tune.name.name} – ${siteName}`,
          body: renderTune(s, file, lang, ctx.sound),
          alternate: tunePath(other(lang), tune.id, tune.slug),
          mount: (main) => mountTune(main, s, file, lang, ctx),
        };
      }
      const set = s.sets.find((x) => x.id === route.id);
      const members = set ? setTunes(s, set) : [];
      if (!set || !members.length) return { siteName, title: `${l.setNotFound} – ${siteName}`, body: `<p>${esc(l.setNotFound)}</p>` };
      let tuneFiles: TuneFile[];
      try {
        tuneFiles = await Promise.all(members.map((x) => ctx.tuneFile(x)));
      } catch {
        throw new LoadError(l.load);
      }
      const want = setPath(lang, set.id, set.slug);
      if (url.pathname !== want) history.replaceState(null, "", want);
      return {
        siteName,
        title: `${setTitle(s, set, lang)} – ${siteName}`,
        body: renderSet(s, set, tuneFiles, lang, ctx.sound),
        alternate: setPath(other(lang), set.id, set.slug),
        mount: (main) => mountSet(main, s, set, tuneFiles, lang, ctx),
      };
    },
  };
}

interface Ctx {
  sound: Instrument;
  /** Tunes ticked for printing: kept while the visitor stays on the Site, never stored or shared. */
  picked: Set<string>;
  tuneFile(tune: CatalogueTune): Promise<TuneFile>;
}

// --- Browser wiring (not covered by the string tests) --------------------------

/** One tune plays at a time; a button shows Play or Stop. */
class Player {
  private synth?: MidiBuffer;
  private button?: HTMLButtonElement;
  private label = "";

  /** Starts abc on button (stopping whatever plays), or stops it when it is the one playing. */
  async toggle(button: HTMLButtonElement, abc: string, sound: Instrument, lang: Lang, status?: HTMLElement | null): Promise<void> {
    if (this.button === button) return this.stop();
    this.stop();
    this.button = button;
    this.label = button.textContent ?? "";
    button.textContent = L[lang].stop;
    button.setAttribute("aria-pressed", "true");
    try {
      const abcjs = await loadAbcjs();
      if (!abcjs.synth.supportsAudio()) {
        if (status) status.textContent = L[lang].noAudio;
        return this.stop();
      }
      // An audio context made in the click itself is allowed to start.
      if (!abcjs.synth.activeAudioContext()) abcjs.synth.registerAudioContext();
      const [visualObj] = abcjs.renderAbc("*", abc);
      const synth = new abcjs.synth.CreateSynth();
      await synth.init({ visualObj, options: synthOptions(SOUND_FONT_URL, sound, { onEnded: () => this.button === button && this.stop() }) });
      await synth.prime();
      if (this.button !== button) return;
      this.synth = synth;
      synth.start();
    } catch {
      if (this.button === button) {
        if (status) status.textContent = L[lang].noAudio;
        this.stop();
      }
    }
  }

  stop(): void {
    this.synth?.stop();
    this.synth = undefined;
    if (this.button) {
      this.button.textContent = this.label;
      this.button.removeAttribute("aria-pressed");
    }
    this.button = undefined;
  }

  /** True when button is the one playing. */
  playing(button: HTMLElement): boolean {
    return this.button === button;
  }
}

const player = new Player();

/** Draws ABC into el, once. */
async function draw(el: HTMLElement, abc: string): Promise<void> {
  if (el.dataset.drawn) return;
  el.dataset.drawn = "1";
  const abcjs = await loadAbcjs();
  abcjs.renderAbc(el, abc, { responsive: "resize", add_classes: true, paddingleft: 0, paddingright: 0 });
}

/** Draws each Incipit when it nears the viewport. */
function observeIncipits(root: HTMLElement, s: TunesSnapshot): void {
  const els = [...root.querySelectorAll<HTMLElement>("[data-incipit]")];
  const drawOne = (el: HTMLElement) => {
    const tune = s.tunes.find((x) => x.id === el.dataset.incipit);
    // The row already names the Tune: no title above its opening bars.
    if (tune?.incipit) void draw(el, tune.incipit.replace(/^T:.*(\r?\n|$)/gm, ""));
  };
  if (!("IntersectionObserver" in window)) return els.forEach(drawOne);
  const io = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (!e.isIntersecting) continue;
        io.unobserve(e.target);
        drawOne(e.target as HTMLElement);
      }
    },
    { rootMargin: "200px" },
  );
  els.forEach((el) => io.observe(el));
}

/** Replaces button with the Recording's player (click-to-load). */
function loadRecording(button: HTMLElement, r: Recording, title: string): void {
  const src = recordingPlayerUrl(r);
  if (!src) return;
  const frame = document.createElement("iframe");
  frame.src = src;
  frame.title = title;
  frame.allow = "autoplay; fullscreen; picture-in-picture; encrypted-media";
  frame.allowFullscreen = true;
  frame.referrerPolicy = "strict-origin-when-cross-origin";
  button.replaceWith(frame);
  frame.focus();
}

/** Shows the ticked or given Tunes as a print layout, opens the print dialog, then removes it. */
async function printTunes(main: HTMLElement, s: TunesSnapshot, items: { tune: TuneBase; abc?: string }[], lang: Lang): Promise<void> {
  const layout = document.createElement("div");
  layout.className = "print-layout";
  layout.innerHTML = items.map((x) => renderPrintTune(s, x.tune, !!x.abc, lang)).join("");
  main.querySelector(".print-layout")?.remove();
  main.append(layout);
  // The layout is shown on screen while the dialog is open: abcjs measures
  // text as it draws, which needs a rendered (not display: none) element.
  document.documentElement.classList.add("printing");
  const abcjs = await loadAbcjs();
  for (const x of items) {
    const el = layout.querySelector<HTMLElement>(`[data-print-sheet="${CSS.escape(x.tune.id)}"]`);
    if (el && x.abc) abcjs.renderAbc(el, x.abc, { responsive: "resize" });
  }
  const done = () => {
    document.documentElement.classList.remove("printing");
    layout.remove();
  };
  addEventListener("afterprint", done, { once: true });
  await new Promise((r) => requestAnimationFrame(() => r(null)));
  print();
}

function wireSound(root: HTMLElement, ctx: Ctx): void {
  root.addEventListener("change", (e) => {
    const el = e.target as HTMLSelectElement;
    if (el.dataset.control !== "sound" || !(SOUNDS as string[]).includes(el.value)) return;
    ctx.sound = el.value as Instrument;
    player.stop();
    root.querySelectorAll<HTMLSelectElement>("select[data-control=sound]").forEach((x) => (x.value = ctx.sound));
  });
}

function mountCatalogue(main: HTMLElement, s: TunesSnapshot, lang: Lang, ctx: Ctx): void {
  const l = L[lang];
  const form = main.querySelector<HTMLFormElement>("[data-filters]");
  const results = main.querySelector<HTMLElement>("[data-results]")!;
  const status = main.querySelector<HTMLElement>("[data-status]");
  const state = () => readState(s, new URL(location.href).searchParams);
  void loadAbcjs();
  observeIncipits(results, s);
  wireSound(main, ctx);
  const refreshPrint = () => {
    const btn = main.querySelector<HTMLButtonElement>("[data-control=print]");
    if (btn) btn.outerHTML = printButton(lang, ctx.picked.size);
    if (status) status.textContent = ctx.picked.size ? "" : l.printHint;
  };
  const apply = (st: TuneState, push: boolean) => {
    const next = tunesPath(lang) + writeState(st);
    if (push) history.pushState(null, "", next);
    else history.replaceState(null, "", next);
    const alt = document.querySelector<HTMLAnchorElement>("header a.lang");
    if (alt) alt.href = tunesPath(other(lang)) + writeState(st);
    player.stop();
    results.innerHTML = renderResults(s, st, lang, ctx.picked);
    observeIncipits(results, s);
    const count = main.querySelector<HTMLElement>("[data-count]");
    if (count) count.textContent = countText(s, st, lang);
  };
  /** Redraws the controls, keeping focus on the control that had it. */
  const redraw = (st: TuneState) => {
    if (!form) return;
    const active = document.activeElement as HTMLElement | null;
    const key = active && form.contains(active) ? (active.dataset.type ? `[data-type="${CSS.escape(active.dataset.type)}"]` : active.dataset.control ? `[data-control="${active.dataset.control}"]` : "") : "";
    form.innerHTML = renderControls(s, st, lang);
    if (key) (form.querySelector<HTMLElement>(key) ?? form.querySelector<HTMLElement>("[data-control=query]"))?.focus();
  };
  form?.addEventListener("submit", (e) => e.preventDefault());
  form?.addEventListener("click", (e) => {
    const el = (e.target as HTMLElement).closest<HTMLElement>("button");
    if (!el) return;
    let st = state();
    if (el.dataset.type) {
      const id = el.dataset.type;
      st.typeIds = st.typeIds.includes(id) ? st.typeIds.filter((x) => x !== id) : [...st.typeIds, id];
    } else if (el.dataset.control === "clear") {
      st = readState(s, new URLSearchParams());
    } else return;
    apply(st, true);
    redraw(st);
  });
  form?.addEventListener("change", (e) => {
    const el = e.target as HTMLSelectElement;
    const st = state();
    const v = el.value;
    switch (el.dataset.control) {
      case "tonic":
        st.tonic = v || undefined;
        break;
      case "mode":
        st.mode = (MODES as readonly string[]).includes(v) ? (v as Mode) : undefined;
        break;
      case "origin":
        st.originId = v || undefined;
        break;
      case "instrument":
        st.instrumentId = v || undefined;
        if (!v) st.fit = "suits";
        break;
      case "fit":
        st.fit = v === "nottricky" ? "notTricky" : "suits";
        break;
      case "group":
        st.group = v === "key" || v === "type" || v === "origin" ? v : undefined;
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
    if (clear) clear.disabled = !isFiltered(st);
  });
  results.addEventListener("change", (e) => {
    const el = e.target as HTMLInputElement;
    const id = el.dataset.pick;
    if (!id) return;
    if (el.checked) ctx.picked.add(id);
    else ctx.picked.delete(id);
    refreshPrint();
  });
  results.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>("button");
    if (!btn) return;
    const incipit = btn.dataset.playIncipit;
    const rec = btn.dataset.playRecording;
    if (incipit) {
      const tune = s.tunes.find((x) => x.id === incipit);
      if (tune?.incipit) void player.toggle(btn, withTempo(tune.incipit, tune.tempo, tune.tempoBeat), ctx.sound, lang, status);
    } else if (rec) {
      const tune = s.tunes.find((x) => x.id === rec);
      if (tune?.firstRecording) loadRecording(btn, tune.firstRecording, tune.name.name);
    }
  });
  main.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>("button[data-control=print]");
    if (!btn || !ctx.picked.size) return;
    const picked = s.tunes.filter((x) => ctx.picked.has(x.id));
    if (status) status.textContent = l.printing;
    btn.disabled = true;
    Promise.all(picked.map((x) => (x.hasTranscription ? ctx.tuneFile(x) : undefined)))
      .then((fs) => {
        if (status) status.textContent = "";
        return printTunes(main, s, picked.map((tune, i) => ({ tune, abc: fs[i]?.transcription?.text })), lang);
      })
      .catch(() => {
        if (status) status.textContent = l.printFailed;
      })
      .finally(() => (btn.disabled = false));
  });
}

function wirePdfs(main: HTMLElement): void {
  // Only where the browser shows PDFs inline; the "Open PDF" card is there anyway.
  if (!navigator.pdfViewerEnabled) return;
  for (const el of main.querySelectorAll<HTMLElement>("[data-pdf]")) {
    const frame = document.createElement("iframe");
    frame.src = el.dataset.pdf!;
    frame.title = el.dataset.pdfTitle ?? "PDF";
    frame.loading = "lazy";
    frame.referrerPolicy = "strict-origin-when-cross-origin";
    el.append(frame);
  }
}

function mountTune(main: HTMLElement, s: TunesSnapshot, tune: TuneFile, lang: Lang, ctx: Ctx): void {
  const l = L[lang];
  const status = main.querySelector<HTMLElement>("[data-status]");
  const sheet = main.querySelector<HTMLElement>("[data-sheet]");
  const abc = tune.transcription?.text;
  let pct = 100;
  void loadAbcjs();
  if (sheet && abc) void draw(sheet, abc);
  wirePdfs(main);
  wireSound(main, ctx);
  main.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>("button");
    if (!btn) return;
    if (btn.dataset.playTune && abc) {
      void player.toggle(btn, withTempo(abc, (tune.tempo * pct) / 100, tune.tempoBeat), ctx.sound, lang, status);
    } else if (btn.dataset.tempo) {
      pct = Math.min(150, Math.max(50, pct + Number(btn.dataset.tempo)));
      const out = main.querySelector<HTMLOutputElement>("[data-tempo-value]");
      if (out) out.value = l.tempoValue(pct, tempoText((tune.tempo * pct) / 100, tune.tempoBeat));
      main.querySelectorAll<HTMLButtonElement>("[data-tempo]").forEach((b) => (b.disabled = (Number(b.dataset.tempo) < 0 ? pct <= 50 : pct >= 150)));
      // Restart at the new tempo when playing.
      const playing = main.querySelector<HTMLButtonElement>("[data-play-tune]");
      if (playing && player.playing(playing)) {
        player.stop();
        void player.toggle(playing, withTempo(abc!, (tune.tempo * pct) / 100, tune.tempoBeat), ctx.sound, lang, status);
      }
    } else if (btn.dataset.recording) {
      const r = tune.recordings[Number(btn.dataset.recording)];
      if (r) loadRecording(btn, r, r.label || tune.name.name);
    } else if (btn.dataset.control === "share") {
      void share(location.href, tune.name.name, lang, status);
    } else if (btn.dataset.control === "print") {
      player.stop();
      void printTunes(main, s, [{ tune, abc }], lang);
    }
  });
}

async function share(url: string, title: string, lang: Lang, status: HTMLElement | null): Promise<void> {
  if (navigator.share) {
    try {
      await navigator.share({ title, url });
    } catch {
      // Cancelled by the visitor.
    }
    return;
  }
  try {
    await navigator.clipboard.writeText(url);
    if (status) status.textContent = L[lang].copied;
  } catch {
    if (status) status.textContent = L[lang].copyFailed;
  }
}

function mountSet(main: HTMLElement, s: TunesSnapshot, set: TuneSet, files: TuneFile[], lang: Lang, ctx: Ctx): void {
  const status = main.querySelector<HTMLElement>("[data-status]");
  const abcOf = (id: string) => files.find((x) => x.id === id)?.transcription?.text;
  for (const el of main.querySelectorAll<HTMLElement>("[data-sheet]")) {
    const abc = abcOf(el.dataset.sheet!);
    if (abc) void draw(el, abc);
  }
  wireSound(main, ctx);
  main.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>("button");
    if (!btn) return;
    const id = btn.dataset.playTune;
    const tune = id ? s.tunes.find((x) => x.id === id) : undefined;
    const abc = id ? abcOf(id) : undefined;
    if (tune && abc) {
      void player.toggle(btn, withTempo(abc, tune.tempo, tune.tempoBeat), ctx.sound, lang, status);
    } else if (btn.dataset.playRecording) {
      const rt = s.tunes.find((x) => x.id === btn.dataset.playRecording);
      if (rt?.firstRecording) loadRecording(btn, rt.firstRecording, rt.name.name);
    } else if (btn.dataset.control === "print") {
      player.stop();
      void printTunes(main, s, setTunes(s, set).map((tune) => ({ tune, abc: abcOf(tune.id) })), lang);
    }
  });
}
