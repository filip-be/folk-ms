// The protocol between the admin SPA and Autofill, shared by both. Every
// message is a DOM CustomEvent on `document` whose detail is a JSON string
// (strings cross the boundary between a page and a content script or
// userscript in every browser; objects do not, in Firefox).

/** Version of the protocol; the SPA ignores an Autofill that speaks another. */
export const PROTOCOL = 1;

/** Event names. "page:" events are dispatched by the SPA, "af:" by Autofill. */
export const EV = {
  /** SPA → Autofill: is anyone there? Autofill answers with af:hello. */
  ping: "folkms-autofill:page:ping",
  /** Autofill → SPA: announces itself (on load and in answer to ping). */
  hello: "folkms-autofill:af:hello",
  /** SPA → Autofill: keep this handoff for Facebook (detail: Handoff). */
  handoff: "folkms-autofill:page:handoff",
  /** Autofill → SPA: the handoff was stored (detail: {id, ok, error?}). */
  stored: "folkms-autofill:af:stored",
  /** SPA → Autofill: deliver results and reports now. */
  poll: "folkms-autofill:page:poll",
  /** Autofill → SPA: a Facebook Event was created (detail: FillResult). */
  result: "folkms-autofill:af:result",
  /** Autofill → SPA: queued failure reports (detail: Report[]). */
  reports: "folkms-autofill:af:reports",
  /** SPA → Autofill: these results/reports were handled (detail: {ids}). */
  ack: "folkms-autofill:page:ack",
} as const;

/** The attribute on <html> marking that an Autofill runs in this page. */
export const MARKER = "data-folkms-autofill";

export interface Hello {
  protocol: number;
  version: string;
  /** "extension" or "userscript". */
  channel: string;
}

/** The fields of a Facebook Event Handoff, as the Moderator edited them. */
export interface HandoffFields {
  name: string;
  /** Local to the Organization: YYYY-MM-DD and HH:MM. */
  startDate: string;
  startTime: string;
  endDate: string;
  endTime: string;
  /** IANA time zone of the Organization. */
  timezone: string;
  location: string;
  description: string;
  ticketUrl?: string;
}

export interface Handoff {
  /** Random id of this handoff. */
  id: string;
  org: string;
  eventId: string;
  occurrenceKey: string;
  /** The handoff token the SPA saves the link with. */
  token: string;
  pageName: string;
  fields: HandoffFields;
  /** The Cover Image, base64 JPEG, or omitted. */
  cover?: { name: string; type: string; data: string };
  /** The field map (autofill/fieldmap.json) embedded in the admin build. */
  fieldMap: unknown;
  /** Where "Back to the CMS" leads. */
  returnUrl: string;
  /** The UI language of the CMS, for the status panel. */
  lang: "pl" | "en";
}

export type FieldStatus = "filled" | "needs-you" | "failed";

/** What Autofill did on Facebook, delivered back to the CMS. */
export interface FillResult {
  /** Same as the handoff id. */
  id: string;
  org: string;
  eventId: string;
  occurrenceKey: string;
  token: string;
  /** The new Facebook Event's address, once the Moderator pressed Create. */
  url?: string;
  fields: Record<string, FieldStatus>;
  at: string;
}

/** A failure report for the Audit Log. */
export interface Report {
  id: string;
  org: string;
  eventId: string;
  version: string;
  channel: string;
  browser: string;
  uiLanguage: string;
  fields: Record<string, FieldStatus>;
  errors: string[];
  filledAt: string;
}

/** How long a stored handoff waits for Facebook. */
export const HANDOFF_TTL_MS = 15 * 60 * 1000;
/** How long an undelivered result is kept. */
export const RESULT_TTL_MS = 24 * 60 * 60 * 1000;

/** Sends a protocol event with a JSON detail. */
export function send(name: string, detail: unknown, target: EventTarget = document): void {
  target.dispatchEvent(new CustomEvent(name, { detail: JSON.stringify(detail) }));
}

/** Reads a protocol event's detail, or undefined when it is not valid JSON. */
export function read<T>(e: Event): T | undefined {
  const d = (e as CustomEvent).detail;
  if (typeof d !== "string") return undefined;
  try {
    return JSON.parse(d) as T;
  } catch {
    return undefined;
  }
}
