// Admin UI language. The user's choice is kept in the folkms-lang cookie,
// which the CMS also reads for its server-rendered pages; without it the
// browser's languages decide: the first of Polish or English, else English.
// Changing the language reloads the page, so every string is read once.

import type { Lang, Text } from "./api.ts";

// The CMS reads the same cookie: cms/internal/http/lang.go (langCookie).
const COOKIE = "folkms-lang";
const isLang = (v: unknown): v is Lang => v === "pl" || v === "en";

function fromCookie(): Lang | undefined {
  const v = document.cookie
    .split(";")
    .map((c) => c.trim().split("="))
    .find(([k]) => k === COOKIE)?.[1];
  return isLang(v) ? v : undefined;
}

function fromBrowser(): Lang {
  const prefs = navigator.languages?.length ? navigator.languages : [navigator.language];
  for (const tag of prefs) {
    const primary = (tag ?? "").toLowerCase().split("-")[0];
    if (isLang(primary)) return primary;
  }
  return "en";
}

export const lang: Lang = fromCookie() ?? fromBrowser();

/** BCP 47 locale for dates and numbers. */
export const locale = lang === "pl" ? "pl-PL" : "en-GB";

/** Picks the UI string for the current language. */
export const L = (pl: string, en: string): string => (lang === "pl" ? pl : en);

/** Bilingual text in the UI language, falling back to Polish. */
export const inLang = (x?: Text | string): string =>
  typeof x === "string" ? x : (lang === "en" && x?.en) || x?.pl || "";

/** Stores the choice for a year and reloads the page in that language. */
export function setLang(l: Lang): void {
  const secure = location.protocol === "https:" ? "; Secure" : "";
  document.cookie = `${COOKIE}=${l}; Path=/; Max-Age=31536000; SameSite=Lax${secure}`;
  location.reload();
}

document.documentElement.lang = lang;
