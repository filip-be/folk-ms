// Reader for the folk-ms Site snapshot (contract/events.schema.json).
// Hand-written types; keep them in step with the schema (enforced by the
// fixture tests in contract/client/test and cms/internal/schema).

export type Lang = "pl" | "en";

/** Polish is always present; English is optional and falls back to Polish. */
export interface LocalizedText {
  pl: string;
  en?: string;
}

export interface Organization {
  id: string;
  name: LocalizedText;
  siteId: string;
  baseUrl: string;
  timezone: string;
}

/** Where an Occurrence happens: free text from Facebook or the Moderator. */
export interface Location {
  name: string;
  address?: string;
  /** Any http(s) address; Sites link it only through httpsUrl. */
  mapUrl?: string;
}

/** Free-text price and/or a tickets link; at least one is present. */
export interface Price {
  text?: LocalizedText;
  url?: string;
}

export interface ImageVariant {
  width: number;
  height: number;
  url: string;
}

export interface Image {
  alt?: LocalizedText;
  variants: ImageVariant[];
}

export interface Occurrence {
  /** RFC 3339 with offset, in the Organization's timezone. */
  start: string;
  end: string;
  cancelled: boolean;
  /** Present (and true) only for an online Occurrence. */
  online?: boolean;
  location?: Location;
  /** The Facebook Event page of this Occurrence (https://www.facebook.com/events/<id>/). */
  facebookEventUrl: string;
}

/**
 * One Facebook Event of the Organization (or a Manual Event). A repeating
 * Facebook Event is one Event with several Occurrences. There is no kind:
 * multi-day is derived from an Occurrence's dates (isMultiDay).
 */
export interface Event {
  id: string;
  shortId: string;
  slug: string;
  permalink: Record<Lang, string>;
  title: LocalizedText;
  summary?: LocalizedText;
  description?: LocalizedText;
  price?: Price;
  cover?: Image;
  /** Sorted by start; at least one. */
  occurrences: Occurrence[];
  updatedAt: string;
}

export interface Snapshot {
  schemaVersion: 2;
  generatedAt: string;
  organization: Organization;
  events: Event[];
}

export const SCHEMA_VERSION = 2;

/** Fetches and sanity-checks a snapshot (pub/sites/<site>/events.json). */
export async function fetchSnapshot(url: string, init?: RequestInit): Promise<Snapshot> {
  const res = await fetch(url, { ...init, headers: { Accept: "application/json", ...(init?.headers ?? {}) } });
  if (!res.ok) throw new Error(`snapshot: HTTP ${res.status}`);
  return parseSnapshot(await res.json());
}

/**
 * Checks the shape the reader relies on. Throws on any schemaVersion other
 * than 2: a version 1 snapshot (venues, recurrence, kind) is not readable by
 * this reader, which means the CMS and the Site were deployed out of step.
 */
export function parseSnapshot(data: unknown): Snapshot {
  if (typeof data !== "object" || data === null) throw new Error("snapshot: not an object");
  const s = data as { schemaVersion?: unknown; organization?: unknown; events?: unknown };
  if (s.schemaVersion !== SCHEMA_VERSION) {
    throw new Error(
      `snapshot: unsupported schemaVersion ${String(s.schemaVersion)}; this reader reads version ${SCHEMA_VERSION} only (deploy the Site and the CMS together)`,
    );
  }
  if (typeof s.organization !== "object" || s.organization === null || !Array.isArray(s.events)) {
    throw new Error("snapshot: missing organization or events");
  }
  for (const e of s.events as Partial<Event>[]) {
    if (!Array.isArray(e?.occurrences) || e.occurrences.length === 0) {
      throw new Error(`snapshot: event ${String(e?.id)} has no occurrences`);
    }
  }
  return data as Snapshot;
}

/** Translation fallback: English where present, otherwise Polish. */
export function t(text: LocalizedText | undefined, lang: Lang): string {
  if (!text) return "";
  return (lang === "en" && text.en) || text.pl;
}

/** True when the text has a real translation in `lang` (no fallback used). */
export function hasTranslation(text: LocalizedText | undefined, lang: Lang): boolean {
  return !!text && (lang === "pl" ? !!text.pl : !!text.en);
}

/**
 * The address if it is an absolute https URL, else undefined. Sites make a
 * link only from what this returns (snapshot text from Facebook is untrusted).
 */
export function httpsUrl(url: string | undefined): string | undefined {
  if (!url) return undefined;
  try {
    return new URL(url).protocol === "https:" ? url : undefined;
  } catch {
    return undefined;
  }
}

export interface OccurrenceOptions {
  /** Only Occurrences ending at or after this instant. */
  from?: Date;
  /** Only Occurrences starting before this instant. */
  to?: Date;
  includeCancelled?: boolean;
}

/** An Event's Occurrences in start order, filtered. */
export function occurrences(event: Event, opts: OccurrenceOptions = {}): Occurrence[] {
  const from = opts.from?.getTime() ?? -Infinity;
  const to = opts.to?.getTime() ?? Infinity;
  return event.occurrences
    .filter((o) => (opts.includeCancelled ?? true) || !o.cancelled)
    .filter((o) => Date.parse(o.end) >= from && Date.parse(o.start) < to)
    .sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
}

/** The next Occurrence that has not ended yet (cancelled ones skipped). */
export function nextOccurrence(event: Event, now: Date = new Date()): Occurrence | undefined {
  return occurrences(event, { from: now, includeCancelled: false })[0];
}

/**
 * The Occurrence to show for an Event in a list: the next one that has not
 * ended and is not cancelled; else the next cancelled one that has not ended
 * (the Event is shown as cancelled); else the last one.
 */
export function featuredOccurrence(event: Event, now: Date = new Date()): Occurrence {
  const ahead = occurrences(event, { from: now });
  const all = occurrences(event);
  return ahead.find((o) => !o.cancelled) ?? ahead[0] ?? all[all.length - 1]!;
}

/** True when every Occurrence of the Event is cancelled. */
export function isCancelled(event: Event): boolean {
  return event.occurrences.every((o) => o.cancelled);
}

export interface Split {
  /** Events with an Occurrence that has not ended, ordered by their featured Occurrence. */
  upcoming: Event[];
  /** Events whose Occurrences all ended, most recent first. */
  past: Event[];
}

/**
 * Splits Events into upcoming and past relative to `now`. An Event whose
 * remaining Occurrences are all cancelled stays upcoming, shown as cancelled.
 */
export function splitUpcomingPast(events: Event[], now: Date = new Date()): Split {
  const upcoming: { e: Event; at: number }[] = [];
  const past: { e: Event; at: number }[] = [];
  for (const e of events) {
    const ahead = occurrences(e, { from: now });
    if (ahead.length) {
      upcoming.push({ e, at: Date.parse(featuredOccurrence(e, now).start) });
    } else {
      past.push({ e, at: Math.max(...e.occurrences.map((o) => Date.parse(o.end))) });
    }
  }
  upcoming.sort((a, b) => a.at - b.at);
  past.sort((a, b) => b.at - a.at);
  return { upcoming: upcoming.map((x) => x.e), past: past.map((x) => x.e) };
}

/** Finds an Event by the `<shortId>-<slug>` path segment; the slug is cosmetic. */
export function findByPermalinkSegment(snapshot: Snapshot, segment: string): Event | undefined {
  const shortId = segment.split("-", 1)[0]?.toLowerCase();
  return snapshot.events.find((e) => e.shortId === shortId);
}

/** Chooses the smallest image variant at least `minWidth` wide (or the largest). */
export function pickVariant(image: Image | undefined, minWidth: number): ImageVariant | undefined {
  if (!image || image.variants.length === 0) return undefined;
  const sorted = [...image.variants].sort((a, b) => a.width - b.width);
  return sorted.find((v) => v.width >= minWidth) ?? sorted[sorted.length - 1];
}

/** Formats an Occurrence's start in the Organization's timezone. */
export function formatStart(o: Occurrence, lang: Lang, timeZone: string): string {
  return new Intl.DateTimeFormat(lang === "pl" ? "pl-PL" : "en-GB", {
    timeZone,
    weekday: "short",
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(o.start));
}

const DAY = new Map<string, Intl.DateTimeFormat>();
function dayKey(at: number, timeZone: string): string {
  let f = DAY.get(timeZone);
  if (!f) DAY.set(timeZone, (f = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" })));
  return f.format(at);
}

/** An end before this hour of a later day still belongs to the night before. */
const NIGHT_ENDS_HOURS = 6;

/**
 * True when the Occurrence spans more than one day in the Organization's
 * timezone. An end before 06:00 on the next day (a bal running past midnight)
 * does not make it multi-day.
 */
export function isMultiDay(o: Occurrence, timeZone: string): boolean {
  const end = Date.parse(o.end) - NIGHT_ENDS_HOURS * 3_600_000;
  return dayKey(end, timeZone) > dayKey(Date.parse(o.start), timeZone);
}

/**
 * Formats when an Occurrence happens: its start with the time, or for a
 * multi-day Occurrence its date range without times.
 */
export function formatWhen(o: Occurrence, lang: Lang, timeZone: string): string {
  if (!isMultiDay(o, timeZone)) return formatStart(o, lang, timeZone);
  return new Intl.DateTimeFormat(lang === "pl" ? "pl-PL" : "en-GB", { timeZone, day: "numeric", month: "long", year: "numeric" }).formatRange(
    new Date(o.start),
    new Date(o.end),
  );
}
