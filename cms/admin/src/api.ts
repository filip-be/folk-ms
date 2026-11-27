// Typed client for the CMS JSON API (same origin, cookie session).

import { inLang } from "./i18n.ts";

export type Lang = "pl" | "en";
export interface Text {
  pl: string;
  en?: string;
}
export type Source = "facebook" | "manual";

export interface ImageVariant {
  width: number;
  height: number;
  path: string;
}
export interface Image {
  id: string;
  alt?: Text;
  variants: ImageVariant[];
  instagram?: ImageVariant;
}
export interface Location {
  name: string;
  address?: string;
  mapUrl?: string;
}
/** One date of an Event; facebookId is the Facebook Event of that date. */
export interface Occurrence {
  facebookId?: string;
  start: string;
  end: string;
  cancelled?: boolean;
}
/** What Sync last read from Facebook (written only by Sync). */
export interface FacebookPart {
  pageId: string;
  eventId: string;
  name: string;
  description?: string;
  location?: Location;
  online?: boolean;
  cancelled?: boolean;
  ticketUrl?: string;
  coverId?: string;
  cover?: Image;
  occurrences: Occurrence[];
  updatedTime: string;
  syncedAt: string;
}
export interface TextOverride {
  value: string;
  against: string;
}
export interface Overrides {
  name?: TextOverride;
  description?: TextOverride;
  location?: { value?: Location; against?: Location };
  cover?: { value?: Image; against: string };
  ticketUrl?: TextOverride;
}
export type OverrideField = keyof Overrides;
export interface Extras {
  nameEn?: string;
  descriptionEn?: string;
  summary?: Text;
  price?: Text;
  socialImage?: Image;
  writtenAgainst?: { name: string; description: string };
}
export interface ManualPart {
  name: Text;
  description?: Text;
  location?: Location;
  online?: boolean;
  cover?: Image;
  ticketUrl?: string;
  facebookEventId: string;
  occurrences: Occurrence[];
}
export interface Event {
  id: string;
  orgId: string;
  shortId: string;
  source: Source;
  facebook?: FacebookPart;
  manual?: ManualPart;
  overrides?: Overrides;
  extras?: Extras;
  hidden?: boolean;
  syncConflict?: boolean;
  deletedAt?: string;
  binReason?: "gone" | "removed";
  createdAt: string;
  updatedAt: string;
}
/** The values the Site shows. */
export interface Effective {
  name: Text;
  description: Text;
  summary: Text;
  price: Text;
  location?: Location;
  online: boolean;
  cancelled: boolean;
  ticketUrl?: string;
  cover?: Image;
  socialImage?: Image;
  occurrences: Occurrence[];
}
/** What the Event list marks. */
export interface Flags {
  hidden?: boolean;
  cancelled?: boolean;
  manual?: boolean;
  overrideChanged?: OverrideField[];
  textOutOfDate?: boolean;
  formerPage?: boolean;
  syncConflict?: boolean;
  binned?: boolean;
}
export interface FieldError {
  field: string;
  /** Both languages; shown with inLang. */
  message: Text;
}
export interface EventView {
  event: Event;
  etag: string;
  effective: Effective;
  flags: Flags;
  permalink: Record<Lang, string>;
  facebookUrl?: string;
}
export interface EventSummary {
  id: string;
  shortId: string;
  slug: string;
  source: Source;
  name: Text;
  flags: Flags;
  firstStart?: string;
  nextStart?: string;
  lastEnd?: string;
  occurrences: number;
  deletedAt?: string;
  binReason?: "gone" | "removed";
  updatedAt: string;
  hasCover: boolean;
  facebookUrl?: string;
}
/** Sets, confirms (keep) or clears (follow) one Override of a synced Event. */
export interface OverrideInput {
  field: OverrideField;
  follow?: boolean;
  keep?: boolean;
  text?: string;
  location?: Location;
  cover?: Image;
}
export interface ExtrasInput {
  nameEn: string;
  descriptionEn: string;
  summary: Text;
  price: Text;
  socialImage?: Image;
}
/** One date as entered: local date and times; endDate empty means the same day (or the next, past midnight). */
export interface OccurrenceInput {
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
  cancelled: boolean;
}
export interface ManualInput {
  name: Text;
  description: Text;
  location?: Location;
  online: boolean;
  cover?: Image;
  ticketUrl: string;
  facebookUrl: string;
  occurrences: OccurrenceInput[];
  summary: Text;
  price: Text;
  socialImage?: Image;
}
export interface SyncResult {
  listed: number;
  added: number;
  updated: number;
  binned: number;
  conflicts: number;
  errors?: string[];
}
export interface SyncStatus {
  firstSyncAt?: string;
  lastRunAt?: string;
  /** A user id, or "autosync". */
  lastRunBy?: string;
  lastRunByEmail?: string;
  lastSuccessAt?: string;
  lastError?: string;
  lastResult?: SyncResult;
  autosync: boolean;
  running: boolean;
  /** When the timer next syncs; only while Autosync is on and the connection needs no reconnect. */
  nextSyncAt?: string;
}
export interface OrgInfo {
  id: string;
  name: Text;
  siteUrl: string;
  /** The Organization keeps a Tune Collection (the admin shows Tunes). */
  tunes?: boolean;
}
export interface Me {
  id: string;
  email: string;
  name: string;
  superAdmin: boolean;
  orgs: OrgInfo[];
  /** Base URL of the public store (images, snapshots). */
  publicUrl: string;
  /** The sign-in providers offered now. */
  signInMethods: SignInMethod[];
}
export type SignInMethod = "google" | "facebook";
export interface PageInfo {
  id: string;
  name: string;
  igUsername?: string;
}
export interface AutosyncStatus {
  byEmail: string;
  at: string;
  dataAccessExpiresAt?: string;
  reconnectSoon: boolean;
  needsReconnect: boolean;
  /** When the timer next syncs; absent while the connection needs a reconnect. */
  nextSyncAt?: string;
}
export interface ConnectionStatus {
  connected: boolean;
  pageName?: string;
  pageId?: string;
  igUsername?: string;
  createdByEmail?: string;
  createdAt?: string;
  pendingPages?: PageInfo[];
  /** Present while Autosync is on. */
  autosync?: AutosyncStatus;
  /** Autosync ended because the person who turned it on lost access (decision 19). */
  autosyncLost?: { reason: string; byEmail: string; at: string };
  /** The session may do guarded actions now (Recent sign-in). */
  recentSignIn: boolean;
}
export type Platform = "facebook" | "instagram";
export interface PostDraft {
  platform: Platform;
  caption: string;
}
export interface SocialPost {
  id: string;
  platform: Platform;
  status: "pending" | "posted" | "failed";
  url?: string;
  caption: string;
  error?: string;
  attemptedBy: string;
  attemptedAt: string;
}
export interface PostsView {
  /** Hidden Events and Events in the Bin cannot be posted. */
  postable: boolean;
  captions: Record<Lang, string>;
  posts: SocialPost[];
  images: Partial<Record<Platform, string>>;
  connection: ConnectionStatus;
  canInstagram: boolean;
  /** Autosync is on: posting uses the stored token after a Recent sign-in. */
  storedToken: boolean;
  postsToday: number;
  dailyLimit: number;
}
export interface PlatformResult {
  platform: Platform;
  status: string;
  reason?: string;
}
export interface PostOutcome {
  refused?: string;
  results?: PlatformResult[];
}

export interface User {
  id: string;
  email: string;
  name?: string;
  orgs: string[] | null;
  revoked?: boolean;
  lastLoginAt: string;
  superAdmin: boolean;
}
export interface Invitation {
  id: string;
  email: string;
  orgs: string[];
  status: string;
  effectiveStatus: string;
  createdAt: string;
  expiresAt: string;
}
/** A Feature Flag as the Super Admin sees it. */
export interface FeatureFlag {
  key: string;
  name: Text;
  description: Text;
  default: boolean;
  enabled: boolean;
  changedAt?: string;
  changedByEmail?: string;
}
export interface AuditEntry {
  time: string;
  actor: string;
  actorEmail?: string;
  action: string;
  org?: string;
  target?: string;
  details?: Record<string, unknown>;
}

/** Up to 50 Audit Log lines, newest first; next continues it. */
export interface AuditPage {
  entries: AuditEntry[];
  next?: string;
}

// ── Video Collection (wideopomoc) ─────────────────────────────────────
export type VideoProvider = "youtube" | "vimeo";
export interface VideoLink {
  provider: VideoProvider;
  id: string;
  hash?: string;
  startSeconds?: number;
}
export interface Video {
  id: string;
  title: Text;
  description?: Text;
  credit?: string;
  /** A country or region id; absent: no Origin. */
  originId?: string;
  formIds: string[];
  link: VideoLink;
  hidden?: boolean;
  createdAt: string;
  updatedAt: string;
  /** Canonical address with the start; linkUrl without it. */
  url: string;
  linkUrl: string;
  /** m:ss */
  start?: string;
  thumbnailUrl: string;
}
/** A Form or region with the number of Videos that use it. */
export interface Labeled {
  id: string;
  label: Text;
  uses: number;
}
/** A Form with its optional description (absent when there is none). */
export interface Form extends Labeled {
  description?: Text;
}
export interface Country extends Labeled {
  regions: Labeled[];
}
/** The whole collection with its version; every change sends `etag` back. */
export interface VideoCollection {
  etag: string;
  forms: Form[];
  origins: Country[];
  videos: Video[];
}
export interface VideoInput {
  link: string;
  start: string;
  title: Text;
  description: Text;
  credit: string;
  originId: string;
  formIds: string[];
}
export interface LinkPreview {
  provider: VideoProvider;
  videoId: string;
  hash?: string;
  startSeconds?: number;
  start?: string;
  url: string;
  linkUrl: string;
  thumbnailUrl: string;
}

// ── Tune Collection ──────────────────────────────────────────────────
// Every Tune, Set and list entry has its own revision (rev): a change sends
// the revision of the item it changes (If-Match) and is refused (conflict)
// only when that item changed meanwhile.

/** Bilingual text where either language may be written first (absent when empty). */
export interface Bi {
  pl?: string;
  en?: string;
}
/** A Tune's name or an Other Name as musicians know it, with an optional translation. */
export interface TuneName {
  name: string;
  translation?: Bi;
}
export interface TuneKey {
  tonic: string;
  mode: string;
}
export interface TuneLabeled {
  id: string;
  label: Bi;
  rev: number;
  uses: number;
}
/** A Tune Type with the Time Signature and Playback Tempo it proposes. */
export interface TuneType extends TuneLabeled {
  meter: string;
  tempo: number;
}
export interface TuneCountry extends TuneLabeled {
  regions: TuneLabeled[];
}
export interface TuneSummary {
  id: string;
  name: TuneName;
  otherNames: TuneName[];
  typeId: string;
  key: TuneKey;
  meter: string;
  /** The Playback Tempo in effect. */
  tempo: number;
  originId?: string;
  composer?: string;
  suits: string[];
  tricky: string[];
  hasTranscription: boolean;
  recordings: number;
  attachments: number;
  setIds: string[];
  hidden: boolean;
  rev: number;
  updatedAt: string;
}
export interface TuneSet {
  id: string;
  name: Bi;
  /** The name, else the Tunes' names. */
  title: Bi;
  tuneIds: string[];
  hidden: boolean;
  /** Shown on the Site: not Hidden, with at least two visible Tunes. */
  published: boolean;
  permalink?: Record<Lang, string>;
  rev: number;
  updatedAt: string;
}
export interface TuneCollection {
  types: TuneType[];
  instruments: TuneLabeled[];
  origins: TuneCountry[];
  /** Alphabetical, ignoring a leading article. */
  tunes: TuneSummary[];
  sets: TuneSet[];
  modes: string[];
}
export interface Recording {
  /** What the form sends back to keep the Recording. */
  link: string;
  label?: string;
  kind: "video" | "link";
  provider?: VideoProvider;
  videoId?: string;
  start?: string;
  url: string;
  embedUrl?: string;
  thumbnailUrl?: string;
  host?: string;
}
export interface TuneAttachment {
  id: string;
  kind: "image" | "pdf";
  name: string;
  size: number;
  caption: Bi;
  /** The PDF, or the largest image width. */
  url: string;
  width?: number;
  height?: number;
  variants?: { width: number; height: number; url: string }[];
  addedAt: string;
  addedBy: string;
}
/** A Set the Tune is in; emptiedOnRemove: removing the Tune removes the Set too. */
export interface TuneSetRef {
  id: string;
  title: Bi;
  tunes: number;
  hidden: boolean;
  emptiedOnRemove: boolean;
}
export interface AbcProposal {
  key?: TuneKey;
  meter?: string;
}
export interface TuneView {
  id: string;
  name: TuneName;
  otherNames: TuneName[];
  typeId: string;
  key: TuneKey;
  meter: string;
  /** The Tune's own Playback Tempo (0: the Tune Type's). */
  tempo: number;
  effectiveTempo: number;
  tempoBeat: string;
  originId?: string;
  composer?: string;
  suits: string[];
  tricky: string[];
  abc: string;
  incipitBars: number;
  incipit?: string;
  abcProposal?: AbcProposal;
  keyDiffers: boolean;
  meterDiffers: boolean;
  recordings: Recording[];
  description: Bi;
  attachments: TuneAttachment[];
  sets: TuneSetRef[];
  hidden: boolean;
  permalink?: Record<Lang, string>;
  rev: number;
  createdAt: string;
  updatedAt: string;
}
export interface RecordingInput {
  link: string;
  label: string;
}
export interface TuneInput {
  name: TuneName;
  otherNames: TuneName[];
  typeId: string;
  key: TuneKey;
  meter: string;
  /** 0: the Tune Type's. */
  tempo: number;
  originId: string;
  composer: string;
  suits: string[];
  tricky: string[];
  abc: string;
  incipitBars: number;
  recordings: RecordingInput[];
  description: Bi;
}
export interface AbcPreview {
  header: { titles?: string[]; rhythm?: string; origin?: string; composer?: string; key?: string; meter?: string; unit?: string };
  proposal: AbcProposal;
  bars: number;
  incipit?: string;
  /** The text has a K: line and notes. */
  valid: boolean;
}
export interface ImportCandidate {
  index: number;
  name: TuneName;
  otherNames: TuneName[];
  /** The Tune Type whose label matches R: ("" or absent when none does). */
  typeId?: string;
  rhythm?: string;
  key?: TuneKey;
  meter?: string;
  originId?: string;
  /** The O: text, matched or not. */
  origin?: string;
  composer?: string;
  abc: string;
  /** Names of Tunes (or earlier candidates) with the same name. */
  duplicate?: string[];
  problems?: FieldError[];
}
export type TuneListKind = "types" | "instruments" | "origins";

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly problems: FieldError[] = [],
  ) {
    super(message);
  }
}

async function request<T>(method: string, path: string, body?: unknown, headers: Record<string, string> = {}): Promise<T> {
  const init: RequestInit = { method, credentials: "same-origin", headers: { Accept: "application/json", ...headers } };
  if (body instanceof Blob) {
    init.body = body;
  } else if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)["Content-Type"] = "application/json";
  }
  const res = await fetch(path, init);
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    throw new ApiError(res.status, data?.error ?? "error", inLang(data?.message) || `HTTP ${res.status}`, data?.problems ?? []);
  }
  return data as T;
}

const o = (org: string) => `/api/orgs/${encodeURIComponent(org)}`;
const t = (org: string) => `${o(org)}/tunes`;
/** The item revision a Tune Collection change is made on. */
const ifMatch = (rev: number) => ({ "If-Match": String(rev) });

export const api = {
  me: () => request<Me>("GET", "/api/me"),
  logout: () => request<void>("POST", "/auth/logout"),
  /** Public: the sign-in providers offered, for the sign-in screen. */
  signInMethods: () => request<{ methods: SignInMethod[] }>("GET", "/auth/methods"),

  events: (org: string, view: string) => request<EventSummary[]>("GET", `${o(org)}/events?view=${view}`),
  event: (org: string, id: string) => request<EventView>("GET", `${o(org)}/events/${id}`),
  createManual: (org: string, m: ManualInput) => request<EventView>("POST", `${o(org)}/events`, m),
  updateManual: (org: string, id: string, etag: string, m: ManualInput) =>
    request<EventView>("PUT", `${o(org)}/events/${id}/manual`, m, { "If-Match": etag }),
  setExtras: (org: string, id: string, etag: string, x: ExtrasInput) =>
    request<EventView>("PUT", `${o(org)}/events/${id}/extras`, x, { "If-Match": etag }),
  setOverride: (org: string, id: string, in_: OverrideInput) => request<EventView>("POST", `${o(org)}/events/${id}/override`, in_),
  eventAction: (org: string, id: string, action: "hide" | "show" | "dismiss-text" | "delete" | "restore") =>
    request<EventView>("POST", `${o(org)}/events/${id}/${action}`),
  uploadImage: (org: string, kind: "cover" | "social", file: File) =>
    request<Image>("POST", `${o(org)}/images?kind=${kind}`, file, { "Content-Type": file.type || "application/octet-stream" }),

  posts: (org: string, id: string) => request<PostsView>("GET", `${o(org)}/events/${id}/posts`),
  /** With Autosync off the answer is the Meta sign-in to send the browser to (`url`); with it on, the `outcome`. */
  post: (org: string, id: string, posts: PostDraft[], confirm: boolean) =>
    request<{ url?: string; outcome?: PostOutcome }>("POST", `${o(org)}/events/${id}/posts`, { posts, confirm }),

  syncStatus: (org: string) => request<SyncStatus>("GET", `${o(org)}/sync`),
  /** "Sync now": the `result` with Autosync on, else the Meta sign-in (`url`) whose callback runs it. */
  syncNow: (org: string) => request<{ url?: string; result?: SyncResult }>("POST", `${o(org)}/sync`),

  social: (org: string) => request<ConnectionStatus>("GET", `${o(org)}/social`),
  connect: (org: string) => request<{ url: string }>("POST", `${o(org)}/social/connect`),
  selectPage: (org: string, pageId: string, confirm: boolean) => request<ConnectionStatus>("POST", `${o(org)}/social/select`, { pageId, confirm }),
  /** On answers the Meta sign-in that issues the token (`url`); off deletes the token. */
  setAutosync: (org: string, on: boolean) => request<{ url?: string; ok?: boolean }>("POST", `${o(org)}/social/autosync`, { on }),
  disconnect: (org: string) => request<{ ok: boolean }>("POST", `${o(org)}/social/disconnect`),

  videos: (org: string) => request<VideoCollection>("GET", `${o(org)}/videos`),
  linkPreview: (org: string, link: string, start: string) => request<LinkPreview>("POST", `${o(org)}/videos/link-preview`, { link, start }),
  addVideo: (org: string, etag: string, v: VideoInput) => request<VideoCollection>("POST", `${o(org)}/videos`, v, { "If-Match": etag }),
  updateVideo: (org: string, id: string, etag: string, v: VideoInput) =>
    request<VideoCollection>("PUT", `${o(org)}/videos/${id}`, v, { "If-Match": etag }),
  deleteVideo: (org: string, id: string, etag: string) => request<VideoCollection>("DELETE", `${o(org)}/videos/${id}`, undefined, { "If-Match": etag }),
  videoAction: (org: string, id: string, etag: string, action: "hide" | "show") =>
    request<VideoCollection>("POST", `${o(org)}/videos/${id}/${action}`, undefined, { "If-Match": etag }),
  /** kind: "" for a Video, "forms/" or "origins/" for a list item. */
  moveItem: (org: string, kind: "" | "forms/" | "origins/", id: string, etag: string, to: number) =>
    request<VideoCollection>("POST", `${o(org)}/videos/${kind}${id}/move`, { to }, { "If-Match": etag }),
  addForm: (org: string, etag: string, label: Text, description: Text) =>
    request<VideoCollection>("POST", `${o(org)}/videos/forms`, { label, description }, { "If-Match": etag }),
  /** Replaces a Form's label and description (empty clears it). */
  updateForm: (org: string, id: string, etag: string, label: Text, description: Text) =>
    request<VideoCollection>("PUT", `${o(org)}/videos/forms/${id}`, { label, description }, { "If-Match": etag }),
  addOrigin: (org: string, etag: string, label: Text, parentId = "") =>
    request<VideoCollection>("POST", `${o(org)}/videos/origins`, { label, parentId }, { "If-Match": etag }),
  renameOrigin: (org: string, id: string, etag: string, label: Text) =>
    request<VideoCollection>("PUT", `${o(org)}/videos/origins/${id}`, { label }, { "If-Match": etag }),
  removeItem: (org: string, kind: "forms/" | "origins/", id: string, etag: string) =>
    request<VideoCollection>("DELETE", `${o(org)}/videos/${kind}${id}`, undefined, { "If-Match": etag }),

  tunes: (org: string) => request<TuneCollection>("GET", t(org)),
  tune: (org: string, id: string) => request<TuneView>("GET", `${t(org)}/${id}`),
  createTune: (org: string, v: TuneInput) => request<TuneView>("POST", t(org), v),
  updateTune: (org: string, id: string, rev: number, v: TuneInput) => request<TuneView>("PUT", `${t(org)}/${id}`, v, ifMatch(rev)),
  /** confirm: also when the Tune is in Sets (else 409 needs_confirmation). */
  removeTune: (org: string, id: string, rev: number, confirm: boolean) =>
    request<TuneCollection>("DELETE", `${t(org)}/${id}${confirm ? "?confirm=true" : ""}`, undefined, ifMatch(rev)),
  tuneAction: (org: string, id: string, action: "hide" | "show") => request<TuneView>("POST", `${t(org)}/${id}/${action}`),
  uploadAttachment: (org: string, id: string, file: File) =>
    request<TuneView>("POST", `${t(org)}/${id}/attachments?name=${encodeURIComponent(file.name)}`, file, { "Content-Type": file.type || "application/octet-stream" }),
  setAttachmentCaption: (org: string, id: string, aid: string, caption: Bi) => request<TuneView>("PUT", `${t(org)}/${id}/attachments/${aid}`, { caption }),
  moveAttachment: (org: string, id: string, aid: string, to: number) => request<TuneView>("POST", `${t(org)}/${id}/attachments/${aid}/move`, { to }),
  removeAttachment: (org: string, id: string, aid: string) => request<TuneView>("DELETE", `${t(org)}/${id}/attachments/${aid}`),
  markdownPreview: (org: string, text: string) => request<{ html: string }>("POST", `${t(org)}/markdown-preview`, { text }),
  abcPreview: (org: string, abc: string, bars: number) => request<AbcPreview>("POST", `${t(org)}/abc-preview`, { abc, bars }),
  importPreview: (org: string, text: string) => request<{ candidates: ImportCandidate[] }>("POST", `${t(org)}/import-preview`, { text }),
  recordingPreview: (org: string, link: string) => request<Recording>("POST", `${t(org)}/link-preview`, { link }),
  /** Creates the chosen Tunes, all Hidden. */
  importTunes: (org: string, tunes: TuneInput[]) => request<TuneCollection>("POST", `${t(org)}/import`, { tunes }),
  addSet: (org: string, name: Bi, tuneIds: string[]) => request<TuneCollection>("POST", `${t(org)}/sets`, { name, tuneIds }),
  updateSet: (org: string, id: string, rev: number, name: Bi, tuneIds: string[]) =>
    request<TuneCollection>("PUT", `${t(org)}/sets/${id}`, { name, tuneIds }, ifMatch(rev)),
  removeSet: (org: string, id: string, rev: number) => request<TuneCollection>("DELETE", `${t(org)}/sets/${id}`, undefined, ifMatch(rev)),
  setAction: (org: string, id: string, action: "hide" | "show") => request<TuneCollection>("POST", `${t(org)}/sets/${id}/${action}`),
  addTuneType: (org: string, label: Bi, meter: string, tempo: number) => request<TuneCollection>("POST", `${t(org)}/types`, { label, meter, tempo }),
  updateTuneType: (org: string, id: string, rev: number, label: Bi, meter: string, tempo: number) =>
    request<TuneCollection>("PUT", `${t(org)}/types/${id}`, { label, meter, tempo }, ifMatch(rev)),
  addInstrument: (org: string, label: Bi) => request<TuneCollection>("POST", `${t(org)}/instruments`, { label }),
  updateInstrument: (org: string, id: string, rev: number, label: Bi) => request<TuneCollection>("PUT", `${t(org)}/instruments/${id}`, { label }, ifMatch(rev)),
  addTuneOrigin: (org: string, label: Bi, parentId = "") => request<TuneCollection>("POST", `${t(org)}/origins`, { label, parentId }),
  renameTuneOrigin: (org: string, id: string, rev: number, label: Bi) => request<TuneCollection>("PUT", `${t(org)}/origins/${id}`, { label }, ifMatch(rev)),
  removeTuneListItem: (org: string, kind: TuneListKind, id: string) => request<TuneCollection>("DELETE", `${t(org)}/${kind}/${id}`),
  moveTuneListItem: (org: string, kind: TuneListKind, id: string, to: number) => request<TuneCollection>("POST", `${t(org)}/${kind}/${id}/move`, { to }),

  users: () => request<User[]>("GET", "/api/users"),
  setUserOrgs: (id: string, orgs: string[]) => request<User>("PUT", `/api/users/${id}/orgs`, { orgs }),
  revokeUser: (id: string) => request<User>("POST", `/api/users/${id}/revoke`),
  invitations: () => request<Invitation[]>("GET", "/api/invitations"),
  invite: (email: string, orgs: string[]) => request<Invitation>("POST", "/api/invitations", { email, orgs }),
  revokeInvitation: (id: string) => request<Invitation>("POST", `/api/invitations/${id}/revoke`),
  auditDays: () => request<string[]>("GET", "/api/audit"),
  auditPage: (day = "", cursor = "") =>
    request<AuditPage>("GET", `/api/audit/entries?${new URLSearchParams({ day, cursor })}`),
  featureFlags: () => request<FeatureFlag[]>("GET", "/api/feature-flags"),
  setFeatureFlag: (key: string, enabled: boolean) =>
    request<FeatureFlag[]>("PUT", `/api/feature-flags/${encodeURIComponent(key)}`, { enabled }),
};
