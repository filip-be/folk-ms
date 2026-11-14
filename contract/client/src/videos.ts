// Reader for the folk-ms Video Collection snapshot (contract/videos.schema.json):
// pub/sites/<site>/videos.json. Hand-written types; keep them in step with the
// schema (enforced by the fixture tests in contract/client/test and
// cms/internal/schema).

import { t, type Lang, type LocalizedText, type Organization } from "./index.ts";

export interface Labeled {
  /** 8 characters a–z, 2–7. */
  id: string;
  label: LocalizedText;
}

/** A country with its regions, in list order. */
export interface Country extends Labeled {
  regions: Labeled[];
}

export type Provider = "youtube" | "vimeo";

export interface Video {
  id: string;
  title: LocalizedText;
  description?: LocalizedText;
  /** Plain text: who made or dances in the video. */
  credit?: string;
  /** A country or a region id; absent when the Video has no Origin. */
  originId?: string;
  /** Form ids in list order; may be empty. */
  formIds: string[];
  provider: Provider;
  videoId: string;
  /** Hash of an unlisted Vimeo video. */
  hash?: string;
  /** Start time in seconds. */
  start?: number;
  /** Canonical watch address (with the start). */
  url: string;
  /** Player address for an iframe: www.youtube-nocookie.com or player.vimeo.com. */
  embedUrl: string;
  /** https thumbnail on i.ytimg.com or i.vimeocdn.com. */
  thumbnail: string;
}

export interface VideoSnapshot {
  schemaVersion: 1;
  organization: Organization;
  forms: Labeled[];
  origins: Country[];
  /** Not Hidden, in the Moderators' order. */
  videos: Video[];
  updatedAt?: string;
}

export const VIDEOS_SCHEMA_VERSION = 1;

/** Fetches and sanity-checks a Video Collection snapshot. */
export async function fetchVideos(url: string, init?: RequestInit): Promise<VideoSnapshot> {
  const res = await fetch(url, { ...init, headers: { Accept: "application/json", ...(init?.headers ?? {}) } });
  if (!res.ok) throw new Error(`videos: HTTP ${res.status}`);
  return parseVideos(await res.json());
}

/** Checks the shape the reader relies on; refuses any other schemaVersion. */
export function parseVideos(data: unknown): VideoSnapshot {
  if (typeof data !== "object" || data === null) throw new Error("videos: not an object");
  const s = data as Partial<Record<keyof VideoSnapshot, unknown>>;
  if (s.schemaVersion !== VIDEOS_SCHEMA_VERSION) {
    throw new Error(
      `videos: unsupported schemaVersion ${String(s.schemaVersion)}; this reader reads version ${VIDEOS_SCHEMA_VERSION} only (deploy the Site and the CMS together)`,
    );
  }
  if (typeof s.organization !== "object" || s.organization === null || !Array.isArray(s.forms) || !Array.isArray(s.origins) || !Array.isArray(s.videos)) {
    throw new Error("videos: missing organization, forms, origins or videos");
  }
  for (const v of s.videos as Partial<Video>[]) {
    if (!Array.isArray(v?.formIds)) throw new Error(`videos: video ${String(v?.id)} has no formIds`);
  }
  for (const c of s.origins as Partial<Country>[]) {
    if (!Array.isArray(c?.regions)) throw new Error(`videos: country ${String(c?.id)} has no regions`);
  }
  return data as VideoSnapshot;
}

/** The thumbnail YouTube serves for a video id. */
export const youtubeThumbnail = (id: string) => `https://i.ytimg.com/vi/${encodeURIComponent(id)}/hqdefault.jpg`;

/** Where an Origin id sits: its country, and its region when it is one. */
export interface OriginPath {
  country: Country;
  region?: Labeled;
}

/** Resolves a country or region id; undefined for an unknown id. */
export function originPath(s: VideoSnapshot, id: string | undefined): OriginPath | undefined {
  if (!id) return undefined;
  for (const country of s.origins) {
    if (country.id === id) return { country };
    const region = country.regions.find((r) => r.id === id);
    if (region) return { country, region };
  }
  return undefined;
}

/** An Origin as text: "Region, Country" or "Country". */
export function originLabel(s: VideoSnapshot, id: string | undefined, lang: Lang): string {
  const p = originPath(s, id);
  if (!p) return "";
  return p.region ? `${t(p.region.label, lang)}, ${t(p.country.label, lang)}` : t(p.country.label, lang);
}

/**
 * Text folded for search: lowercase, without diacritics (ł and đ, which do
 * not decompose, included), so "Łańcuch" matches "lancuch".
 */
export function fold(text: string): string {
  return text
    .normalize("NFD")
    .replace(/\p{M}/gu, "")
    .replace(/[łŁ]/g, "l")
    .replace(/[đĐ]/g, "d")
    .replace(/ß/g, "ss")
    .toLowerCase();
}

export interface VideoFilter {
  /** A Video matches when it has any of these Forms; empty or absent: any. */
  formIds?: string[];
  /** A country (which includes its regions) or a region. */
  originId?: string;
  /** Words matched in the Polish or English title, ignoring case and diacritics. */
  query?: string;
}

/** The Videos matching a filter, in their order. */
export function filterVideos(s: VideoSnapshot, f: VideoFilter): Video[] {
  const forms = new Set(f.formIds ?? []);
  const origin = originPath(s, f.originId);
  const allowed = origin && new Set(origin.region ? [origin.region.id] : [origin.country.id, ...origin.country.regions.map((r) => r.id)]);
  const words = fold(f.query ?? "")
    .split(/\s+/)
    .filter(Boolean);
  return s.videos.filter((v) => {
    if (forms.size && !v.formIds.some((id) => forms.has(id))) return false;
    if (f.originId && !(v.originId && allowed?.has(v.originId))) return false;
    if (words.length) {
      const title = fold(`${v.title.pl} ${v.title.en ?? ""}`);
      if (!words.every((w) => title.includes(w))) return false;
    }
    return true;
  });
}

export type GroupBy = "form" | "origin";

/** A group of Videos; `label` is absent for the final "Other" group. */
export interface VideoGroup {
  id: string;
  label?: LocalizedText;
  videos: Video[];
  /** Grouped by Origin: a country's regions, each with its Videos. */
  subgroups?: VideoGroup[];
}

/** The id of the "Other" group. */
export const OTHER = "other";

/**
 * Groups Videos (keeping their order inside each group); groups follow the
 * list order and empty ones are left out.
 * - By Form: a Video appears under each of its Forms; Videos without a Form
 *   go to a final Other group.
 * - By Origin: one group per country, with the Videos of the country itself
 *   and a subgroup per region; Videos without an Origin go to Other.
 */
export function groupVideos(s: VideoSnapshot, videos: Video[], by: GroupBy): VideoGroup[] {
  const out: VideoGroup[] = [];
  const other: Video[] = [];
  if (by === "form") {
    const known = new Set(s.forms.map((f) => f.id));
    for (const f of s.forms) {
      const vs = videos.filter((v) => v.formIds.includes(f.id));
      if (vs.length) out.push({ id: f.id, label: f.label, videos: vs });
    }
    other.push(...videos.filter((v) => !v.formIds.some((id) => known.has(id))));
  } else {
    for (const c of s.origins) {
      const own = videos.filter((v) => v.originId === c.id);
      const subgroups = c.regions
        .map((r) => ({ id: r.id, label: r.label, videos: videos.filter((v) => v.originId === r.id) }))
        .filter((g) => g.videos.length);
      if (own.length || subgroups.length) out.push({ id: c.id, label: c.label, videos: own, subgroups });
    }
    other.push(...videos.filter((v) => !originPath(s, v.originId)));
  }
  if (other.length) out.push({ id: OTHER, videos: other });
  return out;
}

/** The Forms that at least one of the Videos has, in list order. */
export function usedForms(s: VideoSnapshot, videos: Video[] = s.videos): Labeled[] {
  return s.forms.filter((f) => videos.some((v) => v.formIds.includes(f.id)));
}

/** The countries (and of each, the regions) that at least one of the Videos comes from, in list order. */
export function usedOrigins(s: VideoSnapshot, videos: Video[] = s.videos): Country[] {
  const ids = new Set(videos.map((v) => v.originId).filter(Boolean));
  return s.origins
    .map((c) => ({ ...c, regions: c.regions.filter((r) => ids.has(r.id)) }))
    .filter((c) => ids.has(c.id) || c.regions.length);
}

/** Formats a start time in seconds as m:ss or h:mm:ss. */
export function formatStart(seconds: number | undefined): string {
  if (!seconds || seconds <= 0) return "";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor(seconds / 60) % 60;
  const sec = String(seconds % 60).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${sec}` : `${m}:${sec}`;
}

/** Hosts a player may be loaded from (the Site's CSP frame-src). */
export const EMBED_HOSTS = ["www.youtube-nocookie.com", "player.vimeo.com"];

/**
 * The player address to load into an iframe, with autoplay: only an https
 * address on an embed host, else undefined.
 */
export function playerUrl(v: Video): string | undefined {
  try {
    const u = new URL(v.embedUrl);
    if (u.protocol !== "https:" || !EMBED_HOSTS.includes(u.hostname)) return undefined;
    u.searchParams.set("autoplay", "1");
    return u.toString();
  } catch {
    return undefined;
  }
}
