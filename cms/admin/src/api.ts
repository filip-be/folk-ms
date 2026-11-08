// Typed client for the CMS JSON API (same origin, cookie session).

import { inLang } from "./i18n.ts";

export type Lang = "pl" | "en";
export interface Text {
  pl: string;
  en?: string;
}
export type Kind = "single" | "multiDay" | "recurring";
export type Freq = "weekly" | "biweekly" | "monthly";
export type Status = "draft" | "published" | "binned";

export interface Recurrence {
  freq: Freq;
  weekday: number;
  nth?: number;
  endDate: string;
}
export interface Exception {
  date: string;
  cancelled?: boolean;
  startTime?: string;
  endTime?: string;
  venueId?: string;
  note?: Text;
}
export interface Schedule {
  kind: Kind;
  startDate: string;
  startTime: string;
  endDate?: string;
  endTime: string;
  recurrence?: Recurrence;
  exceptions?: Exception[];
}
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
export interface Content {
  slug: string;
  title: Text;
  summary?: Text;
  description?: Text;
  schedule: Schedule;
  venueId?: string;
  price?: { text?: Text; url?: string };
  cover?: Image;
  socialImage?: Image;
}
export interface Event {
  id: string;
  orgId: string;
  shortId: string;
  draft: Content;
  published?: Content;
  publishedAt?: string;
  deletedAt?: string;
  draftRevision: number;
  publishedRevision?: number;
  updatedAt: string;
}
export interface Occurrence {
  key: string;
  start: string;
  end: string;
  cancelled: boolean;
  venueId: string;
  note?: Text;
}
export interface FieldError {
  field: string;
  /** Both languages; shown with inLang. */
  message: Text;
}
export interface EventView {
  event: Event;
  etag: string;
  status: Status;
  occurrences: Occurrence[];
  problems: FieldError[];
  permalink: Record<Lang, string>;
}
export interface EventSummary {
  id: string;
  shortId: string;
  slug: string;
  title: Text;
  kind: Kind;
  status: Status;
  unpublishedChanges: boolean;
  firstStart?: string;
  nextStart?: string;
  lastEnd?: string;
  occurrences: number;
  deletedAt?: string;
  updatedAt: string;
  hasCover: boolean;
}
export interface Venue {
  id: string;
  name: string;
  address: string;
  mapUrl?: string;
  archived?: boolean;
  etag: string;
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
export interface ConnectionStatus {
  connected: boolean;
  pageName?: string;
  pageId?: string;
  igUsername?: string;
  createdByEmail?: string;
  createdAt?: string;
  pendingPages?: PageInfo[];
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
  published: boolean;
  captions: Record<Lang, string>;
  posts: SocialPost[];
  images: Partial<Record<Platform, string>>;
  connection: ConnectionStatus;
  canInstagram: boolean;
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
  createEvent: (org: string, c: Partial<Content>) => request<EventView>("POST", `${o(org)}/events`, c),
  updateEvent: (org: string, id: string, etag: string, c: Content) =>
    request<EventView>("PUT", `${o(org)}/events/${id}`, c, { "If-Match": etag }),
  eventAction: (org: string, id: string, action: "publish" | "unpublish" | "delete" | "restore" | "duplicate") =>
    request<EventView>("POST", `${o(org)}/events/${id}/${action}`),
  preview: (org: string, c: Content) =>
    request<{ occurrences: Occurrence[]; problems: FieldError[] }>("POST", `${o(org)}/occurrences/preview`, c),
  uploadImage: (org: string, kind: "cover" | "social", file: File) =>
    request<Image>("POST", `${o(org)}/images?kind=${kind}`, file, { "Content-Type": file.type || "application/octet-stream" }),

  posts: (org: string, id: string) => request<PostsView>("GET", `${o(org)}/events/${id}/posts`),
  /** Starts a Post Request; the answer is the Meta sign-in to send the browser to. */
  requestPost: (org: string, id: string, posts: PostDraft[], confirm: boolean) =>
    request<{ url: string }>("POST", `${o(org)}/events/${id}/posts`, { posts, confirm }),

  venues: (org: string) => request<Venue[]>("GET", `${o(org)}/venues`),
  createVenue: (org: string, v: Partial<Venue>) => request<Venue>("POST", `${o(org)}/venues`, v),
  updateVenue: (org: string, id: string, etag: string, v: Partial<Venue>) =>
    request<Venue>("PUT", `${o(org)}/venues/${id}`, { name: v.name, address: v.address, mapUrl: v.mapUrl }, { "If-Match": etag }),
  archiveVenue: (org: string, id: string, archived: boolean) => request<Venue>("POST", `${o(org)}/venues/${id}/archive`, { archived }),

  social: (org: string) => request<ConnectionStatus>("GET", `${o(org)}/social`),
  connect: (org: string) => request<{ url: string }>("POST", `${o(org)}/social/connect`),
  selectPage: (org: string, pageId: string) => request<ConnectionStatus>("POST", `${o(org)}/social/select`, { pageId }),
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
