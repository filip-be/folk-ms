// Small rendering helpers: escaped HTML templates, formatting, toasts.

import { ApiError, type FieldError, type Text } from "./api.ts";
import { inLang, L, lang, locale } from "./i18n.ts";

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
/** Bilingual text in the UI language, falling back to Polish. */
export const t = (x?: Text) => inLang(x);

export const WEEKDAYS =
  lang === "pl"
    ? ["niedziela", "poniedziałek", "wtorek", "środa", "czwartek", "piątek", "sobota"]
    : ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export function toast(message: string, kind: "ok" | "error" = "ok"): void {
  const el = document.createElement("div");
  el.className = `toast ${kind}`;
  el.setAttribute("role", kind === "error" ? "alert" : "status");
  el.textContent = message;
  document.body.append(el);
  setTimeout(() => el.remove(), kind === "error" ? 7000 : 3500);
}

/** Shows an error from the API (validation problems are listed). */
export function showError(err: unknown, target?: HTMLElement | null): void {
  if (err instanceof ApiError) {
    if (err.status === 401) {
      location.href = "/auth/login?return=" + encodeURIComponent(location.pathname);
      return;
    }
    if (target && err.problems.length) {
      target.innerHTML = problemsHtml(err.problems).value;
      target.scrollIntoView({ block: "nearest", behavior: "smooth" });
    }
    toast(err.message, "error");
    return;
  }
  console.error(err);
  toast(L("Wystąpił błąd. Spróbuj ponownie.", "Something went wrong. Try again."), "error");
}

export function problemsHtml(problems: FieldError[]): Raw {
  if (!problems.length) return html``;
  return html`<div class="problems" role="alert"><strong>${L("Do poprawy przed publikacją:", "To fix before publishing:")}</strong><ul>${problems.map(
    (p) => html`<li>${inLang(p.message)} <span class="muted">(${p.field})</span></li>`,
  )}</ul></div>`;
}

export const STATUS_LABEL: Record<string, string> = {
  draft: L("Szkic", "Draft"),
  published: L("Opublikowane", "Published"),
  binned: L("W koszu", "In the bin"),
};
export const KIND_LABEL: Record<string, string> = {
  single: L("Jednorazowe", "Single"),
  multiDay: L("Wielodniowe", "Multi-day"),
  recurring: L("Cykliczne", "Recurring"),
};

export function badge(status: string, extra = ""): Raw {
  return html`<span class="badge ${status}">${STATUS_LABEL[status] ?? status}</span>${extra ? html` <span class="badge warn">${extra}</span>` : ""}`;
}

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
