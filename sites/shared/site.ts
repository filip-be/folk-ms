// What every Site is made of: page modules (events(), videos()) that each
// own some routes, and run(), which a Site's entry calls with the modules
// and header links it wants. It renders the current address, keeps
// same-origin links in the page (path and query) and follows Back/Forward.

import { type Lang } from "../../contract/client/src/index.ts";
import { match } from "./router.ts";

// Everything in a snapshot is untrusted (much of it is Facebook text): every
// value goes through esc(), and an address becomes a link only through
// httpsUrl() (images: http or https, since the local CMS serves them over http).
const ESC: Record<string, string> = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
export const esc = (v: unknown) => String(v ?? "").replace(/[&<>"']/g, (c) => ESC[c]!);

/** The language of a path: its first segment, else Polish. */
export const langOf = (pathname: string): Lang => match(pathname).lang;

/** A rendered page. */
export interface Page {
  /** The Site's name for the header (from the snapshot). */
  siteName: string;
  /** HTML of <main>; every value in it escaped. */
  body: string;
  /** The document title; absent keeps the Site's own. */
  title?: string;
  /** The same page in the other language. */
  alternate?: string;
  /** Runs after the page is in the document (event handlers). */
  mount?(main: HTMLElement): void;
}

/** Thrown by a module whose data could not be loaded; its message is shown. */
export class LoadError extends Error {}

/** A page module: it renders the addresses it owns. */
export interface Module {
  /** The page for an address, or undefined when the module does not own it. */
  render(url: URL, lang: Lang): Promise<Page | undefined>;
  /** The "not found" page, from the first module that has one. */
  notFound?(lang: Lang): Promise<Page>;
}

export interface NavItem {
  href(lang: Lang): string;
  label: Record<Lang, string>;
}

export interface SiteOptions {
  modules: Module[];
  /** Header links; none: no navigation in the header. */
  nav?: NavItem[];
}

const SWITCH_LANG: Record<Lang, string> = { pl: "English", en: "Polski" };
const NOT_FOUND: Record<Lang, string> = { pl: "Nie znaleziono strony.", en: "Page not found." };

/** The header, with the language switch and the Site's navigation. */
export function header(page: Page, lang: Lang, nav: NavItem[] = [], pathname = ""): string {
  const other: Lang = lang === "pl" ? "en" : "pl";
  const links = nav.length
    ? `<nav aria-label="${lang === "pl" ? "Strona" : "Site"}"><ul>${nav
        .map((n) => {
          const href = n.href(lang);
          return `<li><a href="${esc(href)}"${pathname === href ? ` aria-current="page"` : ""}>${esc(n.label[lang])}</a></li>`;
        })
        .join("")}</ul></nav>`
    : "";
  return `<header><h1><a href="/${lang}/">${esc(page.siteName)}</a></h1>${links ? `\n      ${links}` : ""}
      ${page.alternate ? `<a class="lang" href="${esc(page.alternate)}" hreflang="${other}">${esc(SWITCH_LANG[lang])}</a>` : ""}</header>`;
}

/** Renders the current address into root and keeps it in step with navigation. */
export async function run(root: HTMLElement, opts: SiteOptions): Promise<void> {
  const siteTitle = document.title;
  const render = async () => {
    const url = new URL(location.href);
    const lang = langOf(url.pathname);
    document.documentElement.lang = lang;
    let page: Page | undefined;
    try {
      for (const m of opts.modules) {
        page = await m.render(url, lang);
        if (page) break;
      }
      if (!page) {
        const nf = opts.modules.find((m) => m.notFound);
        page = nf ? await nf.notFound!(lang) : { siteName: siteTitle, body: `<p>${esc(NOT_FOUND[lang])}</p>` };
      }
    } catch (err) {
      root.innerHTML = `<p>${esc(err instanceof LoadError ? err.message : NOT_FOUND[lang])}</p>`;
      return;
    }
    document.title = page.title ?? siteTitle;
    root.innerHTML = `${header(page, lang, opts.nav, url.pathname)}
      <main>${page.body}</main>`;
    page.mount?.(root.querySelector("main")!);
  };
  document.addEventListener("click", (e) => {
    const a = (e.target as HTMLElement).closest("a");
    const url = a && new URL(a.href, location.href);
    if (!a || !url || url.origin !== location.origin || a.target || a.hasAttribute("download") || e.ctrlKey || e.metaKey || e.shiftKey) return;
    e.preventDefault();
    history.pushState(null, "", url.pathname + url.search);
    void render();
    scrollTo(0, 0);
  });
  addEventListener("popstate", () => void render());
  await render();
}
