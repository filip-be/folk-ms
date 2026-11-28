// Reader for the folk-ms Tune Collection snapshots: the catalogue
// pub/sites/<site>/tunes.json (contract/tunes.schema.json) and one Tune's file
// pub/sites/<site>/tunes/<id>.json (contract/tune.schema.json). Hand-written
// types; keep them in step with the schemas (enforced by the fixture tests in
// contract/client/test and cms/internal/schema).

import { type ImageVariant, type Lang, type Organization } from "./index.ts";
import { fold } from "./videos.ts";

export { fold };

/** Bilingual text where either language may be written first; at least one is present. */
export interface Bi {
  pl?: string;
  en?: string;
}

/** A list entry (Instrument, region): 8 characters a–z, 2–7, and a label. */
export interface TuneLabeled {
  id: string;
  label: Bi;
}

export type TempoBeat = "1/1" | "1/2" | "1/4" | "3/8" | "1/8" | "1/16" | "1/32";

/** A Tune Type (reel, jig, …) with the Time Signature and Playback Tempo it proposes. */
export interface TuneType extends TuneLabeled {
  meter: string;
  tempo: number;
  tempoBeat: TempoBeat;
}

/** A country with its regions, in list order. */
export interface TuneCountry extends TuneLabeled {
  regions: TuneLabeled[];
}

/** A Tune's name or an Other Name as musicians know it, with an optional translation. */
export interface TuneName {
  name: string;
  translation?: Bi;
}

export type Mode = "major" | "minor" | "dorian" | "mixolydian" | "lydian" | "phrygian" | "locrian";

/** The Modes in the order the admin lists them. */
export const MODES: readonly Mode[] = ["major", "minor", "dorian", "mixolydian", "lydian", "phrygian", "locrian"];

export interface Key {
  /** A–G with an optional # or b. */
  tonic: string;
  mode: Mode;
}

/** A YouTube or Vimeo video, played on the Site. */
export interface VideoRecording {
  kind: "video";
  /** Canonical watch address, with the start. */
  url: string;
  provider: "youtube" | "vimeo";
  /** Player address on www.youtube-nocookie.com or player.vimeo.com. */
  embedUrl: string;
  /** On i.ytimg.com or i.vimeocdn.com. */
  thumbnail: string;
  /** Start time in seconds. */
  start?: number;
  /** Who plays it. */
  label?: string;
}

/** A page elsewhere (SoundCloud, Bandcamp, …), linked to. */
export interface LinkRecording {
  kind: "link";
  url: string;
  /** The page's host without www. */
  host: string;
  label?: string;
}

export type Recording = VideoRecording | LinkRecording;

/** Absolute Permalink of each language version. */
export type Permalinks = Record<Lang, string>;

/** What the catalogue and a Tune's file both carry. */
export interface TuneBase {
  id: string;
  slug: string;
  permalink: Permalinks;
  name: TuneName;
  otherNames: TuneName[];
  typeId: string;
  key: Key;
  /** Time Signature, n/d. */
  meter: string;
  /** Playback Tempo: tempoBeat notes a minute. */
  tempo: number;
  tempoBeat: TempoBeat;
  /** A country or region id; absent without an Origin. */
  originId?: string;
  composer?: string;
  /** Instrument ids it suits. */
  suits: string[];
  /** Instrument ids it is tricky on. */
  tricky: string[];
  /** The opening bars as ABC; absent without a Transcription. */
  incipit?: string;
}

export interface CatalogueTune extends TuneBase {
  hasTranscription: boolean;
  firstRecording?: Recording;
  /** Content hash of the Tune's own file. */
  rev: string;
}

export interface TuneSet {
  id: string;
  slug: string;
  permalink: Permalinks;
  /** Absent: the title is its Tunes' names (see setTitle). */
  name?: Bi;
  /** Its visible Tunes, in order (2–10). */
  tuneIds: string[];
}

/** The catalogue: tunes.json. */
export interface TunesSnapshot {
  schemaVersion: 1;
  organization: Organization;
  types: TuneType[];
  instruments: TuneLabeled[];
  origins: TuneCountry[];
  /** Not Hidden, sorted by name (see compareTunes). */
  tunes: CatalogueTune[];
  sets: TuneSet[];
  updatedAt?: string;
}

export interface ImageAttachment {
  id: string;
  kind: "image";
  name: string;
  size: number;
  /** Also the alt text. */
  caption?: Bi;
  width: number;
  height: number;
  variants?: ImageVariant[];
}

export interface PdfAttachment {
  id: string;
  kind: "pdf";
  name: string;
  size: number;
  caption?: Bi;
  url: string;
}

export type Attachment = ImageAttachment | PdfAttachment;

export interface Transcription {
  format: "abc";
  text: string;
}

/** One Tune in full: tunes/<id>.json. */
export interface TuneFile extends TuneBase {
  schemaVersion: 1;
  transcription?: Transcription;
  /** Rendered by the CMS (safe HTML, its own tags only). */
  descriptionHtml?: Bi;
  recordings: Recording[];
  attachments: Attachment[];
  /** The published Sets it is in. */
  sets: TuneSet[];
}

export const TUNES_SCHEMA_VERSION = 1;

// --- Loading ---------------------------------------------------------------

const dir = (base: string) => (base.endsWith("/") ? base : `${base}/`);

/** The catalogue's address under a snapshot base URL (…/pub/sites/<site>/). */
export function tunesUrl(base: string): string {
  return `${dir(base)}tunes.json`;
}

/** A Tune file's address; `rev` (from the catalogue) is added to bust caches. */
export function tuneFileUrl(base: string, id: string, rev?: string): string {
  const u = `${dir(base)}tunes/${encodeURIComponent(id)}.json`;
  return rev ? `${u}?rev=${encodeURIComponent(rev)}` : u;
}

async function getJSON(url: string, what: string, init?: RequestInit): Promise<unknown> {
  const res = await fetch(url, { ...init, headers: { Accept: "application/json", ...(init?.headers ?? {}) } });
  if (!res.ok) throw new Error(`${what}: HTTP ${res.status}`);
  return res.json();
}

/** Fetches and sanity-checks the catalogue from a snapshot base URL. */
export async function fetchTunes(base: string, init?: RequestInit): Promise<TunesSnapshot> {
  return parseTunes(await getJSON(tunesUrl(base), "tunes", init));
}

/** Fetches and sanity-checks one Tune's file from a snapshot base URL. */
export async function fetchTune(base: string, id: string, opts: { rev?: string; init?: RequestInit } = {}): Promise<TuneFile> {
  return parseTune(await getJSON(tuneFileUrl(base, id, opts.rev), "tune", opts.init));
}

function checkVersion(what: string, v: unknown) {
  if (v !== TUNES_SCHEMA_VERSION) {
    throw new Error(
      `${what}: unsupported schemaVersion ${String(v)}; this reader reads version ${TUNES_SCHEMA_VERSION} only (deploy the Site and the CMS together)`,
    );
  }
}

function checkBase(what: string, t: Partial<TuneBase> | undefined) {
  if (typeof t?.name?.name !== "string" || typeof t.key !== "object" || t.key === null || typeof t.permalink !== "object") {
    throw new Error(`${what}: tune ${String(t?.id)} has no name, key or permalink`);
  }
  if (!Array.isArray(t.otherNames) || !Array.isArray(t.suits) || !Array.isArray(t.tricky)) {
    throw new Error(`${what}: tune ${String(t.id)} has no otherNames, suits or tricky`);
  }
}

function checkSets(what: string, sets: Partial<TuneSet>[]) {
  for (const s of sets) {
    if (!Array.isArray(s?.tuneIds)) throw new Error(`${what}: set ${String(s?.id)} has no tuneIds`);
  }
}

/** Checks the shape the reader relies on; refuses any other schemaVersion. */
export function parseTunes(data: unknown): TunesSnapshot {
  if (typeof data !== "object" || data === null) throw new Error("tunes: not an object");
  const s = data as Partial<Record<keyof TunesSnapshot, unknown>>;
  checkVersion("tunes", s.schemaVersion);
  if (
    typeof s.organization !== "object" ||
    s.organization === null ||
    ![s.types, s.instruments, s.origins, s.tunes, s.sets].every(Array.isArray)
  ) {
    throw new Error("tunes: missing organization, types, instruments, origins, tunes or sets");
  }
  for (const c of s.origins as Partial<TuneCountry>[]) {
    if (!Array.isArray(c?.regions)) throw new Error(`tunes: country ${String(c?.id)} has no regions`);
  }
  for (const t of s.tunes as Partial<CatalogueTune>[]) checkBase("tunes", t);
  checkSets("tunes", s.sets as Partial<TuneSet>[]);
  return data as TunesSnapshot;
}

/** Checks the shape of one Tune's file; refuses any other schemaVersion. */
export function parseTune(data: unknown): TuneFile {
  if (typeof data !== "object" || data === null) throw new Error("tune: not an object");
  const t = data as Partial<TuneFile>;
  checkVersion("tune", t.schemaVersion);
  checkBase("tune", t);
  if (![t.recordings, t.attachments, t.sets].every(Array.isArray)) {
    throw new Error(`tune: tune ${String(t.id)} has no recordings, attachments or sets`);
  }
  checkSets("tune", t.sets as Partial<TuneSet>[]);
  return data as TuneFile;
}

// --- Languages -------------------------------------------------------------

/** Language fallback: the text in `lang`, else the other language (either may be missing). */
export function bi(text: Bi | undefined, lang: Lang): string {
  if (!text) return "";
  const other: Lang = lang === "pl" ? "en" : "pl";
  return text[lang] || text[other] || "";
}

/** True when the text is written in `lang` (no fallback used). */
export function hasLang(text: Bi | undefined, lang: Lang): boolean {
  return !!text?.[lang];
}

/**
 * A name's translation in `lang` only (a Polish translation is not shown on
 * the English page); empty when there is none or it equals the name.
 */
export function nameTranslation(n: TuneName, lang: Lang): string {
  const tr = n.translation?.[lang] ?? "";
  return tr && fold(tr) !== fold(n.name) ? tr : "";
}

// --- Lists -----------------------------------------------------------------

export function tuneType(s: TunesSnapshot, t: TuneBase): TuneType | undefined {
  return s.types.find((x) => x.id === t.typeId);
}

/** Instruments by id, in list order (unknown ids are left out). */
export function instruments(s: TunesSnapshot, ids: string[]): TuneLabeled[] {
  return s.instruments.filter((i) => ids.includes(i.id));
}

/** Where an Origin id sits: its country, and its region when it is one. */
export interface TuneOriginPath {
  country: TuneCountry;
  region?: TuneLabeled;
}

/** Resolves a country or region id; undefined for an unknown id. */
export function tuneOriginPath(s: TunesSnapshot, id: string | undefined): TuneOriginPath | undefined {
  if (!id) return undefined;
  for (const country of s.origins) {
    if (country.id === id) return { country };
    const region = country.regions.find((r) => r.id === id);
    if (region) return { country, region };
  }
  return undefined;
}

/** An Origin as text: "Region, Country" or "Country". */
export function tuneOriginLabel(s: TunesSnapshot, id: string | undefined, lang: Lang): string {
  const p = tuneOriginPath(s, id);
  if (!p) return "";
  return p.region ? `${bi(p.region.label, lang)}, ${bi(p.country.label, lang)}` : bi(p.country.label, lang);
}

// --- Keys ------------------------------------------------------------------

const MODE_NAMES: Record<Lang, Record<Mode, string>> = {
  en: { major: "major", minor: "minor", dorian: "dorian", mixolydian: "mixolydian", lydian: "lydian", phrygian: "phrygian", locrian: "locrian" },
  pl: { major: "dur", minor: "moll", dorian: "dorycka", mixolydian: "miksolidyjska", lydian: "lidyjska", phrygian: "frygijska", locrian: "lokrycka" },
};

/** The tonic as Polish writes it: B is H, Bb is B, sharps -is, flats -es. */
function polishTonic(tonic: string): string {
  const [letter = "", acc = ""] = [tonic[0], tonic.slice(1)];
  if (letter === "B") return acc === "b" ? "B" : acc === "#" ? "His" : "H";
  if (acc === "#") return `${letter}is`;
  if (acc === "b") return letter === "E" || letter === "A" ? `${letter}s` : `${letter}es`;
  return letter;
}

/** A Key as text: "E minor", "A dorian"; in Polish "e-moll", "G-dur", "A dorycka". */
export function keyLabel(k: Key, lang: Lang): string {
  if (lang === "en") return `${k.tonic.replace("#", "♯").replace(/b$/, "♭")} ${MODE_NAMES.en[k.mode]}`;
  const tonic = polishTonic(k.tonic);
  if (k.mode === "major") return `${tonic}-dur`;
  if (k.mode === "minor") return `${tonic.toLowerCase()}-moll`;
  return `${tonic} ${MODE_NAMES.pl[k.mode]}`;
}

/** A Key as ABC writes it: "G", "Em", "Ador", "Dmix". */
export function abcKey(k: Key): string {
  if (k.mode === "major") return k.tonic;
  if (k.mode === "minor") return `${k.tonic}m`;
  return k.tonic + k.mode.slice(0, 3);
}

const keyId = (k: Key) => `${k.tonic}-${k.mode}`;

function keyOrder(k: Key): number {
  const semis: Record<string, number> = { C: 0, D: 2, E: 4, F: 5, G: 7, A: 9, B: 11 };
  const acc = k.tonic[1] === "#" ? 1 : k.tonic[1] === "b" ? -1 : 0;
  const semi = ((semis[k.tonic[0] ?? "C"] ?? 0) + acc + 12) % 12;
  // Sharps before flats on the same pitch, then the Mode order.
  return semi * 100 + (acc === 1 ? 0 : acc === -1 ? 50 : 25) + MODES.indexOf(k.mode);
}

// --- Sorting and searching -------------------------------------------------

/** The sort key of a name: folded, without a leading The / An / A / Na. */
export function sortKey(name: string): string {
  const k = fold(name).trim();
  for (const a of ["the ", "an ", "a ", "na "]) {
    if (k.startsWith(a) && k.slice(a.length).trim()) return k.slice(a.length).trim();
  }
  return k;
}

/** Catalogue order: by name ignoring a leading article and diacritics, then by id. */
export function compareTunes(a: TuneBase, b: TuneBase): number {
  const x = sortKey(a.name.name);
  const y = sortKey(b.name.name);
  if (x !== y) return x < y ? -1 : 1;
  return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
}

/** A sorted copy. */
export function sortTunes<T extends TuneBase>(tunes: readonly T[]): T[] {
  return [...tunes].sort(compareTunes);
}

/** Every name a Tune is found by: its name, Other Names and their translations. */
export function allNames(t: TuneBase): string[] {
  const out: string[] = [];
  for (const n of [t.name, ...t.otherNames]) {
    out.push(n.name);
    if (n.translation?.pl) out.push(n.translation.pl);
    if (n.translation?.en) out.push(n.translation.en);
  }
  return out;
}

/** True when every word of the query is in one of the Tune's names (ignoring case and diacritics). */
export function matchesQuery(t: TuneBase, query: string): boolean {
  const words = fold(query).split(/\s+/).filter(Boolean);
  if (!words.length) return true;
  const names = fold(allNames(t).join("\n"));
  return words.every((w) => names.includes(w));
}

export interface TuneFilter {
  /** Words matched in all names and translations. */
  query?: string;
  /** Any of these Tune Types; empty or absent: any. */
  typeIds?: string[];
  /** Key: tonic and/or mode. */
  tonic?: string;
  mode?: Mode;
  /** A country (which includes its regions) or a region. */
  originId?: string;
  /** An Instrument: Tunes that suit it, or Tunes that are not tricky on it. */
  instrument?: { id: string; match: "suits" | "notTricky" };
}

/** The Tunes matching a filter, in their order. */
export function filterTunes<T extends TuneBase>(s: TunesSnapshot, tunes: readonly T[], f: TuneFilter): T[] {
  const types = new Set(f.typeIds ?? []);
  const origin = tuneOriginPath(s, f.originId);
  const allowed = origin && new Set(origin.region ? [origin.region.id] : [origin.country.id, ...origin.country.regions.map((r) => r.id)]);
  return tunes.filter((t) => {
    if (types.size && !types.has(t.typeId)) return false;
    if (f.tonic && t.key.tonic !== f.tonic) return false;
    if (f.mode && t.key.mode !== f.mode) return false;
    if (f.originId && !(t.originId && allowed?.has(t.originId))) return false;
    if (f.instrument) {
      const { id, match } = f.instrument;
      if (match === "suits" ? !t.suits.includes(id) : t.tricky.includes(id)) return false;
    }
    return matchesQuery(t, f.query ?? "");
  });
}

// --- Grouping --------------------------------------------------------------

export type TuneGroupBy = "key" | "type" | "origin";

/** A group of Tunes; neither `label` nor `key` for the final "Other" group. */
export interface TuneGroup<T extends TuneBase = CatalogueTune> {
  id: string;
  /** Tune Type, country or region label. */
  label?: Bi;
  /** Grouped by Key: the Key (show it with keyLabel). */
  key?: Key;
  tunes: T[];
  /** Grouped by Origin: a country's regions, each with its Tunes. */
  subgroups?: TuneGroup<T>[];
}

/** The id of the "Other" group. */
export const OTHER_GROUP = "other";

/**
 * Groups Tunes, keeping their order inside each group; empty groups are left out.
 * - By Key: one group per Key, by tonic (C to B) then Mode.
 * - By Tune Type: in list order; an unknown type goes to Other.
 * - By Origin: one group per country with the Tunes of the country itself and
 *   a subgroup per region; Tunes without a (known) Origin go to Other.
 */
export function groupTunes<T extends TuneBase>(s: TunesSnapshot, tunes: readonly T[], by: TuneGroupBy): TuneGroup<T>[] {
  const out: TuneGroup<T>[] = [];
  const other: T[] = [];
  if (by === "key") {
    const groups = new Map<string, TuneGroup<T>>();
    for (const t of tunes) {
      const id = keyId(t.key);
      let g = groups.get(id);
      if (!g) groups.set(id, (g = { id, key: t.key, tunes: [] }));
      g.tunes.push(t);
    }
    out.push(...[...groups.values()].sort((a, b) => keyOrder(a.key!) - keyOrder(b.key!)));
  } else if (by === "type") {
    for (const ty of s.types) {
      const ts = tunes.filter((t) => t.typeId === ty.id);
      if (ts.length) out.push({ id: ty.id, label: ty.label, tunes: ts });
    }
    other.push(...tunes.filter((t) => !s.types.some((ty) => ty.id === t.typeId)));
  } else {
    for (const c of s.origins) {
      const own = tunes.filter((t) => t.originId === c.id);
      const subgroups = c.regions
        .map((r) => ({ id: r.id, label: r.label, tunes: tunes.filter((t) => t.originId === r.id) }))
        .filter((g) => g.tunes.length);
      if (own.length || subgroups.length) out.push({ id: c.id, label: c.label, tunes: own, subgroups });
    }
    other.push(...tunes.filter((t) => !tuneOriginPath(s, t.originId)));
  }
  if (other.length) out.push({ id: OTHER_GROUP, tunes: other });
  return out;
}

// --- Sets ------------------------------------------------------------------

/** A Set's Tunes from the catalogue, in order (ids not in the catalogue are left out). */
export function setTunes(s: TunesSnapshot, set: TuneSet): CatalogueTune[] {
  return set.tuneIds.flatMap((id) => s.tunes.filter((t) => t.id === id));
}

/** A Set's title: its name (with language fallback), else its Tunes' names joined with " / ". */
export function setTitle(s: TunesSnapshot, set: TuneSet, lang: Lang): string {
  return bi(set.name, lang) || setTunes(s, set).map((t) => t.name.name).join(" / ");
}

// --- Permalinks ------------------------------------------------------------

/** The Site path of a Tune: /{lang}/tunes/{id}-{slug}. */
export function tunePath(lang: Lang, id: string, slug: string): string {
  return `/${lang}/tunes/${encodeURIComponent(id)}-${encodeURIComponent(slug)}`;
}

/** The Site path of a Set: /{lang}/sets/{id}-{slug}. */
export function setPath(lang: Lang, id: string, slug: string): string {
  return `/${lang}/sets/${encodeURIComponent(id)}-${encodeURIComponent(slug)}`;
}

/** The absolute Permalink the snapshot carries for a Tune or Set. */
export function permalink(item: { permalink: Permalinks }, lang: Lang): string {
  return item.permalink[lang];
}

/** The id in a `<id>-<slug>` path segment (the slug is cosmetic and may change). */
export function idFromSegment(segment: string): string {
  return (segment.split("-", 1)[0] ?? "").toLowerCase();
}

/** Finds a Tune by its `<id>-<slug>` path segment. */
export function findTune(s: TunesSnapshot, segment: string): CatalogueTune | undefined {
  const id = idFromSegment(segment);
  return s.tunes.find((t) => t.id === id);
}

/** Finds a Set by its `<id>-<slug>` path segment. */
export function findSet(s: TunesSnapshot, segment: string): TuneSet | undefined {
  const id = idFromSegment(segment);
  return s.sets.find((t) => t.id === id);
}

// --- ABC playback tempo ----------------------------------------------------

/** The ABC tempo field for a Playback Tempo: "Q:3/8=110". */
export function abcTempoLine(tempo: number, beat: TempoBeat): string {
  return `Q:${beat}=${Math.round(tempo)}`;
}

/**
 * The ABC with its tempo set: the first tune's header Q: field is replaced,
 * or one is added before its K: line (the last header field), so playback
 * follows the Playback Tempo (or the visitor's choice).
 */
export function withTempo(abc: string, tempo: number, beat: TempoBeat): string {
  const lines = abc.split(/\r?\n/);
  const q = abcTempoLine(tempo, beat);
  const k = lines.findIndex((l) => /^K:/.test(l));
  const end = k < 0 ? lines.length : k;
  const existing = lines.slice(0, end).findIndex((l) => /^Q:/.test(l));
  if (existing >= 0) lines[existing] = q;
  else if (k >= 0) lines.splice(k, 0, q);
  else {
    const x = lines.findIndex((l) => /^X:/.test(l));
    lines.splice(x + 1, 0, q);
  }
  return lines.join("\n");
}

// --- Recordings and Attachments --------------------------------------------

/** Hosts a player may be loaded from (the Site's CSP frame-src). */
export const EMBED_HOSTS = ["www.youtube-nocookie.com", "player.vimeo.com"];

/** Hosts a Recording's thumbnail may be loaded from. */
export const THUMBNAIL_HOSTS = ["i.ytimg.com", "i.vimeocdn.com"];

/**
 * The player address to load into an iframe, with autoplay: only an https
 * address on an embed host, else undefined.
 */
export function recordingPlayerUrl(r: Recording): string | undefined {
  if (r.kind !== "video") return undefined;
  try {
    const u = new URL(r.embedUrl);
    if (u.protocol !== "https:" || !EMBED_HOSTS.includes(u.hostname)) return undefined;
    u.searchParams.set("autoplay", "1");
    return u.toString();
  } catch {
    return undefined;
  }
}

/** The thumbnail of a video Recording when it is https on a thumbnail host. */
export function recordingThumbnail(r: Recording): string | undefined {
  if (r.kind !== "video") return undefined;
  try {
    const u = new URL(r.thumbnail);
    return u.protocol === "https:" && THUMBNAIL_HOSTS.includes(u.hostname) ? r.thumbnail : undefined;
  } catch {
    return undefined;
  }
}

/** The smallest image variant at least `minWidth` wide (or the largest). */
export function attachmentVariant(a: ImageAttachment, minWidth: number): ImageVariant | undefined {
  const sorted = [...(a.variants ?? [])].sort((x, y) => x.width - y.width);
  return sorted.find((v) => v.width >= minWidth) ?? sorted[sorted.length - 1];
}
