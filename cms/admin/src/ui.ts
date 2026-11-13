// Small rendering helpers: escaped HTML templates, formatting, toasts.

import { ApiError, type FieldError, type Text } from "./api.ts";
import { icon } from "./icons.ts";
import { inLang, L, locale } from "./i18n.ts";

const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };

export function esc(v: unknown): string {
  return String(v ?? "").replace(/[&<>"']/g, (c) => ESC[c]!);
}

/** Raw HTML that html`` must not escape. */
export class Raw {
  constructor(readonly value: string) {}
  toString(): string {
    return this.value;
  }
}

export const raw = (s: string) => new Raw(s);

/** Tagged template that escapes interpolations (arrays are joined). */
export function html(strings: TemplateStringsArray, ...values: unknown[]): Raw {
  let out = strings[0] ?? "";
  values.forEach((v, i) => {
    out += render(v) + (strings[i + 1] ?? "");
  });
  return new Raw(out);
}

function render(v: unknown): string {
  if (v instanceof Raw) return v.value;
  if (Array.isArray(v)) return v.map(render).join("");
  if (v === false || v === null || v === undefined) return "";
  return esc(v);
}

const dtf = new Intl.DateTimeFormat(locale, {
  timeZone: "Europe/Warsaw",
  weekday: "short",
  day: "numeric",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});
const df = new Intl.DateTimeFormat(locale, { timeZone: "Europe/Warsaw", day: "numeric", month: "short", year: "numeric" });

export const fmtDateTime = (s?: string) => (s ? dtf.format(new Date(s)) : "–");
export const fmtDate = (s?: string) => (s ? df.format(new Date(s)) : "–");
const tf = new Intl.DateTimeFormat(locale, { timeZone: "Europe/Warsaw", hour: "2-digit", minute: "2-digit" });
export const fmtTime = (s?: string) => (s ? tf.format(new Date(s)) : "–");
/** The address of a Facebook Event (as the CMS stores it). */
export const facebookEventUrl = (id: string) => `https://www.facebook.com/events/${encodeURIComponent(id)}/`;
/** Bilingual text in the UI language, falling back to Polish. */
export const t = (x?: Text) => inLang(x);

export function toast(message: string, kind: "ok" | "error" = "ok"): void {
  const el = document.createElement("div");
  el.className = `toast ${kind}`;
  el.setAttribute("role", kind === "error" ? "alert" : "status");
  el.textContent = message;
  document.body.append(el);
  setTimeout(() => el.remove(), kind === "error" ? 7000 : 3500);
}

/**
 * Shows an error from the API. Validation problems are marked on their fields
 * inside `form` (see markProblems) and, given a `summary` element, also listed
 * there; `form` defaults to the summary's form.
 */
export function showError(err: unknown, summary?: HTMLElement | null, form?: ParentNode | null): void {
  if (err instanceof ApiError) {
    if (err.status === 401) {
      location.href = "/auth/login?return=" + encodeURIComponent(location.pathname);
      return;
    }
    if (err.code === "recent_sign_in") {
      showRecentSignIn();
      return;
    }
    const root = form ?? summary?.closest("form");
    if (err.problems.length && (root || summary)) {
      markProblems(root ?? document, err.problems, summary);
      (summary ?? root?.querySelector<HTMLElement>(".field-error"))?.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
    toast(err.message, "error");
    return;
  }
  console.error(err);
  toast(L("Wystąpił błąd. Spróbuj ponownie.", "Something went wrong. Try again."), "error");
}

// ── Recent sign-in ────────────────────────────────────────────────────
// Posting with the stored token, turning Autosync on or off and changing or
// removing the connection need a sign-in in the last 15 minutes (decision 13).

/** Where "Sign in again" goes: the sign-in, then back to this screen. */
export const signInAgainUrl = () => "/auth/login?return=" + encodeURIComponent(location.pathname);

/** The banner explaining a Recent sign-in, with "Sign in again". */
export function recentSignInBanner(proactive = false): Raw {
  return html`<div class="banner warn recent-sign-in" role="${proactive ? "status" : "alert"}">
    <p><strong>${L("Potrzebne ponowne logowanie", "Sign in again to continue")}</strong><br>
      ${proactive
        ? L(
            "Publikowanie z zapisanym tokenem strony oraz włączanie i wyłączanie Autosync, zmiana i odłączenie strony wymagają logowania do CMS w ciągu ostatnich 15 minut.",
            "Posting with the stored page token, turning Autosync on or off, and changing or disconnecting the Facebook page need a sign-in to the CMS within the last 15 minutes.",
          )
        : L(
            "Ta operacja wymaga logowania do CMS w ciągu ostatnich 15 minut. Zaloguj się ponownie i spróbuj jeszcze raz.",
            "This action needs a sign-in to the CMS within the last 15 minutes. Sign in again, then try once more.",
          )}</p>
    <a class="button" href="${signInAgainUrl()}">${L("Zaloguj się ponownie", "Sign in again")}</a>
  </div>`;
}

/** Shows the Recent sign-in banner at the top of the current screen. */
export function showRecentSignIn(): void {
  const view = document.getElementById("view");
  if (!view) {
    location.href = signInAgainUrl();
    return;
  }
  view.querySelector(".recent-sign-in")?.remove();
  view.insertAdjacentHTML("afterbegin", recentSignInBanner().value);
  view.querySelector(".recent-sign-in")?.scrollIntoView({ block: "nearest", behavior: "smooth" });
}

// ── Field validation ──────────────────────────────────────────────────────
// A problem's field path (e.g. "schedule.startDate") is matched to a control
// by data-field (a space-separated list of paths) or by name. A path with no
// control of its own falls back to its Polish variant ("title" → "title.pl").
// A data-field on a fieldset marks the whole group (e.g. checkboxes).

const CONTROLS = "input, select, textarea";
let errorSeq = 0;

function findField(root: ParentNode, path: string): HTMLElement | null {
  for (const p of [path, `${path}.pl`]) {
    const q = CSS.escape(p);
    const el = root.querySelector<HTMLElement>(`[data-field~="${q}"], [name="${q}"]:not([type=hidden])`);
    if (el) return el;
  }
  return null;
}

/** The label text of a control or group, for the error summary. */
function fieldLabel(el: HTMLElement): string {
  const label = el.matches("fieldset")
    ? el.querySelector("legend")
    : (el.id && el.ownerDocument.querySelector(`label[for="${CSS.escape(el.id)}"]`)) || el.closest("label");
  if (!label) return el.getAttribute("aria-label") ?? "";
  const copy = label.cloneNode(true) as HTMLElement;
  copy.querySelectorAll(CONTROLS).forEach((c) => c.remove());
  return copy.textContent!.replace(/\s+/g, " ").trim();
}

const capital = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

/** Shows `message` right below a control (or at the end of a group) and marks it invalid. */
export function setFieldError(el: HTMLElement, message: string): void {
  clearFieldError(el);
  const id = `field-error-${++errorSeq}`;
  const p = document.createElement("p");
  p.className = "field-error";
  p.id = id;
  p.innerHTML = html`${icon("alert")}<span>${capital(message)}</span>`.value;
  if (el.matches("fieldset")) el.append(p);
  else (el.closest("label") ?? el).after(p);
  el.setAttribute("aria-invalid", "true");
  el.setAttribute("aria-describedby", [el.getAttribute("aria-describedby"), id].filter(Boolean).join(" "));
}

export function clearFieldError(el: HTMLElement): void {
  if (!el.hasAttribute("aria-invalid")) return;
  el.removeAttribute("aria-invalid");
  const ids = (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);
  for (const id of ids.filter((i) => i.startsWith("field-error-"))) document.getElementById(id)?.remove();
  const rest = ids.filter((i) => !i.startsWith("field-error-"));
  if (rest.length) el.setAttribute("aria-describedby", rest.join(" "));
  else el.removeAttribute("aria-describedby");
}

/**
 * Marks every problem on its field inside `root` (replacing earlier marks)
 * and, given a `summary` element, lists them all there: each with a link to
 * its field, so a problem in a hidden or distant field is still reachable.
 */
export function markProblems(root: ParentNode, problems: FieldError[], summary?: Element | null, heading = L("Do poprawy:", "To fix:")): void {
  root.querySelectorAll<HTMLElement>("[aria-invalid=true]").forEach(clearFieldError);
  const byField = new Map<HTMLElement, string[]>();
  const items = problems.map((p) => {
    const el = findField(root, p.field);
    const message = inLang(p.message);
    if (el) byField.set(el, [...(byField.get(el) ?? []), message]);
    const label = el && fieldLabel(el);
    return label
      ? html`<li><button type="button" class="link" data-goto-field="${p.field}">${label}</button>: ${message}</li>`
      : html`<li>${capital(message)}</li>`;
  });
  byField.forEach((messages, el) => setFieldError(el, messages.join("; ")));
  if (summary) {
    summary.innerHTML = problems.length ? html`<div class="problems" role="alert"><strong>${heading}</strong><ul>${items}</ul></div>`.value : "";
  }
}

/** Focuses the field of a problem; `folk:reveal` lets a view show it first (e.g. switch the language tab). */
function gotoField(path: string, from: Element): void {
  const root = from.closest("form") ?? from.closest("main") ?? document;
  const el = findField(root, path);
  if (!el) return;
  el.dispatchEvent(new CustomEvent("folk:reveal", { bubbles: true }));
  const target = el.matches("fieldset") ? el.querySelector<HTMLElement>(CONTROLS) : el;
  target?.focus();
  el.scrollIntoView({ block: "center", behavior: "smooth" });
}

/** The message for a control that fails the browser's checks (required, type, pattern). */
function validityMessage(c: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement): string {
  const v = c.validity;
  if (v.valueMissing) return c.type === "checkbox" ? L("zaznacz to pole", "check this box") : c instanceof HTMLSelectElement ? L("wybierz wartość", "choose a value") : L("to pole jest wymagane", "this field is required");
  if (v.typeMismatch && c.type === "email") return L("podaj poprawny adres e-mail", "enter a valid email address");
  if (v.typeMismatch && c.type === "url") return L("podaj adres zaczynający się od https://", "enter an address starting with https://");
  if (v.tooLong) return L(`najwyżej ${(c as HTMLInputElement).maxLength} znaków`, `at most ${(c as HTMLInputElement).maxLength} characters`);
  return L("niepoprawna wartość", "the value is not valid");
}

// Document-wide behaviour, so every form gets it without wiring:
// - the browser's own checks (on submit of a form without novalidate) show
//   the design system's inline message instead of the browser's bubble, in
//   the admin UI language, and the first invalid field is focused;
// - editing a field clears its error;
// - a link in an error summary focuses its field.
let focusPending = false;
document.addEventListener(
  "invalid",
  (e) => {
    const c = e.target as HTMLInputElement;
    e.preventDefault();
    setFieldError(c, validityMessage(c));
    if (focusPending) return;
    focusPending = true;
    queueMicrotask(() => {
      focusPending = false;
      c.form?.querySelector<HTMLElement>("[aria-invalid=true]")?.focus();
    });
  },
  true,
);
for (const type of ["input", "change"]) {
  document.addEventListener(type, (e) => {
    const c = e.target as HTMLElement;
    if (!c.matches?.(CONTROLS)) return;
    clearFieldError(c);
    const group = c.closest<HTMLElement>("fieldset[aria-invalid=true]");
    if (group) clearFieldError(group);
  });
}
document.addEventListener("click", (e) => {
  const link = (e.target as Element).closest?.<HTMLElement>("[data-goto-field]");
  if (link) gotoField(link.dataset.gotoField!, link);
});

export function navigate(path: string): void {
  history.pushState(null, "", path);
  dispatchEvent(new PopStateEvent("popstate"));
}

export async function busy<T>(button: HTMLButtonElement | null, fn: () => Promise<T>): Promise<T | undefined> {
  if (button) button.disabled = true;
  try {
    return await fn();
  } finally {
    if (button) button.disabled = false;
  }
}
