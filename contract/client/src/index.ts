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

export interface Venue {
  name: string;
  address: string;
  mapUrl?: string;
}

export interface Price {
  text: LocalizedText;
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

export type Freq = "weekly" | "biweekly" | "monthly";

export interface Recurrence {
  freq: Freq;
  /** 0 = Sunday */
  weekday: number;
  /** Monthly only: Nth weekday of the month, -1 = last. */
  nth?: -1 | 1 | 2 | 3 | 4;
  startDate: string;
  endDate: string;
}

export interface Occurrence {
  /** RFC 3339 with offset, in the Organization's timezone. */
  start: string;
  end: string;
  cancelled: boolean;
  venueId: string;
  note?: LocalizedText;
}

export type EventKind = "single" | "multiDay" | "recurring";

export interface Event {
  id: string;
  shortId: string;
  slug: string;
  permalink: Record<Lang, string>;
  kind: EventKind;
  title: LocalizedText;
  summary?: LocalizedText;
  description?: LocalizedText;
  price?: Price;
  cover?: Image;
  recurrence?: Recurrence;
  occurrences: Occurrence[];
  publishedAt: string;
}

export interface Snapshot {
  schemaVersion: 1;
  generatedAt: string;
  organization: Organization;
  venues: Record<string, Venue>;
  events: Event[];
}

export const SCHEMA_VERSION = 1;

/** Fetches and sanity-checks a snapshot (pub/sites/<site>/events.json). */
export async function fetchSnapshot(url: string, init?: RequestInit): Promise<Snapshot> {
  const res = await fetch(url, { ...init, headers: { Accept: "application/json", ...(init?.headers ?? {}) } });
  if (!res.ok) throw new Error(`snapshot: HTTP ${res.status}`);
  return parseSnapshot(await res.json());
}

/** Checks the shape the reader relies on; throws on an unsupported schema version. */
export function parseSnapshot(data: unknown): Snapshot {
  if (typeof data !== "object" || data === null) throw new Error("snapshot: not an object");
  const s = data as Partial<Snapshot>;
  if (s.schemaVersion !== SCHEMA_VERSION) throw new Error(`snapshot: unsupported schemaVersion ${String(s.schemaVersion)}`);
  if (!s.organization || !Array.isArray(s.events) || typeof s.venues !== "object" || s.venues === null) {
    throw new Error("snapshot: missing organization, venues or events");
  }
  return s as Snapshot;
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

export function venue(snapshot: Snapshot, occurrence: Occurrence): Venue | undefined {
  return snapshot.venues[occurrence.venueId];
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

export interface Split {
  /** Events with an Occurrence that has not ended, ordered by that Occurrence. */
  upcoming: Event[];
  /** Events whose Occurrences all ended, most recent first. */
  past: Event[];
}

/** Splits Events into upcoming and past relative to `now`. */
export function splitUpcomingPast(events: Event[], now: Date = new Date()): Split {
  const upcoming: { e: Event; at: number }[] = [];
  const past: { e: Event; at: number }[] = [];
  for (const e of events) {
    const next = occurrences(e, { from: now, includeCancelled: false })[0];
    if (next) {
      upcoming.push({ e, at: Date.parse(next.start) });
    } else {
      const last = Math.max(...e.occurrences.map((o) => Date.parse(o.end)));
      // An Event whose only remaining Occurrences are cancelled counts as past.
      past.push({ e, at: last });
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
