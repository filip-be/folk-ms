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
  lastSuccessAt?: string;
  lastError?: string;
  lastResult?: SyncResult;
  autosync: boolean;
  running: boolean;
}
export interface OrgInfo {
  id: string;
  name: Text;
  siteUrl: string;
}
export interface Me {
  id: string;
  email: string;
  name: string;
  superAdmin: boolean;
  orgs: OrgInfo[];
  /** Base URL of the public store (images, snapshots). */
  publicUrl: string;
}
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
export interface AuditEntry {
  time: string;
  actor: string;
  actorEmail?: string;
  action: string;
  org?: string;
  target?: string;
  details?: Record<string, unknown>;
}

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

export const api = {
  me: () => request<Me>("GET", "/api/me"),
  logout: () => request<void>("POST", "/auth/logout"),

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

  users: () => request<User[]>("GET", "/api/users"),
  setUserOrgs: (id: string, orgs: string[]) => request<User>("PUT", `/api/users/${id}/orgs`, { orgs }),
  revokeUser: (id: string) => request<User>("POST", `/api/users/${id}/revoke`),
  invitations: () => request<Invitation[]>("GET", "/api/invitations"),
  invite: (email: string, orgs: string[]) => request<Invitation>("POST", "/api/invitations", { email, orgs }),
  revokeInvitation: (id: string) => request<Invitation>("POST", `/api/invitations/${id}/revoke`),
  auditDays: () => request<string[]>("GET", "/api/audit"),
  audit: (day: string) => request<AuditEntry[]>("GET", `/api/audit/${day}`),
};
