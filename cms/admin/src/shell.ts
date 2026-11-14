// App shell: the collapsible sidebar and, on narrow screens, the top bar and
// the off-canvas drawer. Layout and states are styled in app.css ("App shell");
// the pattern is described in docs/design-system/README.md.

import { api, type Me } from "./api.ts";
import { icon, type IconName } from "./icons.ts";
import { L, lang, setLang } from "./i18n.ts";
import { html, navigate, raw, t } from "./ui.ts";

const NAV_KEY = "folkms.nav"; // "collapsed" or "expanded" (wide screens only)
const ADMIN_KEY = "folkms.nav.admin"; // "closed" when the Administration group is folded
const THEME_KEY = "folkms.theme"; // "light" or "dark"; absent follows the system
// Keep in step with the breakpoint of the drawer in app.css.
const narrow = matchMedia("(max-width: 767px)");

function stored(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function store(key: string, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage unavailable */
  }
}

type Theme = "system" | "light" | "dark";
const THEMES: Theme[] = ["system", "light", "dark"];
const THEME_ICON: Record<Theme, IconName> = { system: "monitor", light: "sun", dark: "moon" };
const themeLabel = (th: Theme) =>
  ({ system: L("Motyw: systemowy", "Theme: system"), light: L("Motyw: jasny", "Theme: light"), dark: L("Motyw: ciemny", "Theme: dark") })[th];

function currentTheme(): Theme {
  const v = stored(THEME_KEY);
  return v === "light" || v === "dark" ? v : "system";
}

/** Sets html[data-theme], on which tokens.css switches color-scheme, and
    points the theme-color metas of index.html at the chosen scheme. */
export function applyTheme(th: Theme = currentTheme()): void {
  const root = document.documentElement;
  if (th === "system") delete root.dataset.theme;
  else root.dataset.theme = th;
  document.querySelectorAll<HTMLMetaElement>("meta[name=theme-color][data-scheme]").forEach((m) => {
    m.media = th === "system" ? `(prefers-color-scheme: ${m.dataset.scheme})` : m.dataset.scheme === th ? "all" : "not all";
  });
}
applyTheme();

function themeButton(): ReturnType<typeof html> {
  const th = currentTheme();
  return html`<button type="button" class="nav-item" id="theme-toggle" title="${themeLabel(th)}">${icon(THEME_ICON[th])}<span class="label">${themeLabel(th)}</span></button>`;
}

/** Switches to the other language; its label is in that language. */
export function langSwitch(withIcon = false) {
  const other = lang === "pl" ? "en" : "pl";
  const label = other === "en" ? "English" : "Polski";
  return html`<button type="button" class="${withIcon ? "nav-item" : "link"} lang-switch" data-set-lang="${other}" lang="${other}"
    title="${other === "en" ? "Switch to English" : "Przełącz na polski"}">${withIcon ? html`${icon("globe")}<span class="label">${label}</span>` : label}</button>`;
}

export function bindLangSwitch(): void {
  document.querySelector<HTMLElement>("[data-set-lang]")?.addEventListener("click", (e) => {
    setLang((e.currentTarget as HTMLElement).dataset.setLang as "pl" | "en");
  });
}

function initials(me: Me): string {
  const words = (me.name || me.email).split(/[\s@.]+/).filter(Boolean);
  return ((words[0]?.[0] ?? "") + (words[1]?.[0] ?? "")).toUpperCase() || "?";
}

/** Renders the shell into #app with an empty <main id="view">. */
export function renderShell(me: Me, org: string): void {
  const parts = location.pathname.split("/");
  const section = parts[2] === "orgs" ? (parts[4] ?? "") : (parts[2] ?? "");
  const collapsed = stored(NAV_KEY) === "collapsed";
  const item = (href: string, name: IconName, label: string, key: string) =>
    html`<li><a class="nav-item" href="${href}" title="${label}" ${section === key ? raw('aria-current="page"') : ""}>${icon(name)}<span class="label">${label}</span></a></li>`;
  const adminOpen = stored(ADMIN_KEY) !== "closed" || ["access", "audit", "design"].includes(section);
  const app = document.getElementById("app")!;
  app.className = "app";
  app.dataset.nav = collapsed ? "collapsed" : "expanded";
  delete app.dataset.drawer;
  app.innerHTML = html`
    <header class="topbar">
      <button type="button" class="icon-button" id="drawer-open" aria-controls="sidebar" aria-expanded="false"
        aria-label="${L("Otwórz menu", "Open menu")}">${icon("menu")}</button>
      <a class="brand" href="/app/"><span class="brand-mark" aria-hidden="true">f</span>folk-ms</a>
    </header>
    <aside class="sidebar" id="sidebar" aria-label="${L("Menu główne", "Main menu")}">
      <div class="sidebar-head">
        <a class="brand" href="/app/" title="folk-ms"><span class="brand-mark" aria-hidden="true">f</span><span class="label">folk-ms</span></a>
        <button type="button" class="icon-button collapse-toggle" id="nav-collapse" aria-controls="sidebar"
          aria-expanded="${collapsed ? "false" : "true"}" aria-label="${collapseLabel(collapsed)}"
          title="${collapseLabel(collapsed)}">${icon("sidebar")}</button>
        <button type="button" class="icon-button drawer-close" id="drawer-close"
          aria-label="${L("Zamknij menu", "Close menu")}">${icon("close")}</button>
      </div>
      ${me.orgs.length > 1
        ? html`<label class="org-switch"><span class="sr-only">${L("Organizacja", "Organization")}</span>
            <select id="org-select">${me.orgs.map((o) => html`<option value="${o.id}" ${o.id === org ? "selected" : ""}>${t(o.name)}</option>`)}</select></label>`
        : html`<p class="org-name" title="${t(me.orgs[0]?.name)}">${t(me.orgs[0]?.name)}</p>`}
      <nav class="side-nav" aria-label="${L("Sekcje", "Sections")}">
        <ul>
          ${item(`/app/orgs/${org}/events`, "calendar", L("Wydarzenia", "Events"), "events")}
          ${item(`/app/orgs/${org}/social`, "social", L("Social media", "Social media"), "social")}
          ${me.superAdmin
            ? html`<li><details class="nav-group" id="nav-admin" ${adminOpen ? "open" : ""}>
                <summary class="nav-item" title="${L("Administracja", "Administration")}">${icon("shield")}<span class="label">${L("Administracja", "Administration")}</span>${icon("chevronDown")}</summary>
                <ul class="nav-tree">
                  ${item("/app/access", "users", L("Moderatorzy", "Moderators"), "access")}
                  ${item("/app/audit", "log", L("Audyt", "Audit log"), "audit")}
                  ${item("/app/design", "palette", L("Design system", "Design system"), "design")}
                </ul>
              </details></li>`
            : ""}
        </ul>
      </nav>
      <div class="sidebar-foot">
        <div class="user-card" title="${me.email}">
          <span class="avatar" aria-hidden="true">${initials(me)}</span>
          <span class="label"><strong>${me.name || me.email}</strong>${me.name ? html`<span class="muted">${me.email}</span>` : ""}</span>
        </div>
        ${themeButton()}
        ${langSwitch(true)}
        ${me.superAdmin
          ? ""
          : html`<a class="nav-item" href="/auth/login?provider=facebook&return=${encodeURIComponent(location.pathname)}" title="${L("Połącz logowanie przez Facebooka z tym kontem", "Link Facebook sign-in to this account")}">${icon("link")}<span class="label">${L("Połącz z Facebookiem", "Link Facebook")}</span></a>`}
        <button type="button" class="nav-item" id="logout" title="${L("Wyloguj", "Sign out")}">${icon("logout")}<span class="label">${L("Wyloguj", "Sign out")}</span></button>
      </div>
    </aside>
    <div class="scrim" id="scrim"></div>
    <main id="view" tabindex="-1"></main>`.value;
  bind(app);
}

const collapseLabel = (collapsed: boolean) => (collapsed ? L("Rozwiń menu", "Expand menu") : L("Zwiń menu", "Collapse menu"));

function bind(app: HTMLElement): void {
  const sidebar = document.getElementById("sidebar")!;
  const openBtn = document.getElementById("drawer-open") as HTMLButtonElement;
  const collapseBtn = document.getElementById("nav-collapse") as HTMLButtonElement;
  const view = document.getElementById("view")!;
  const topbar = app.querySelector<HTMLElement>(".topbar")!;

  const setDrawer = (open: boolean) => {
    if (open) app.dataset.drawer = "open";
    else delete app.dataset.drawer;
    openBtn.setAttribute("aria-expanded", String(open));
    view.inert = open;
    topbar.inert = open;
    if (open) (sidebar.querySelector<HTMLElement>("[aria-current=page]") ?? sidebar.querySelector<HTMLElement>(".nav-item"))?.focus();
  };
  const close = () => {
    if (!app.dataset.drawer) return;
    setDrawer(false);
    openBtn.focus();
  };
  openBtn.addEventListener("click", () => setDrawer(true));
  document.getElementById("drawer-close")!.addEventListener("click", close);
  document.getElementById("scrim")!.addEventListener("click", close);
  sidebar.addEventListener("keydown", (e) => {
    if (e.key === "Escape") close();
  });
  narrow.onchange = () => setDrawer(false);

  collapseBtn.addEventListener("click", () => {
    const collapsed = app.dataset.nav !== "collapsed";
    app.dataset.nav = collapsed ? "collapsed" : "expanded";
    collapseBtn.setAttribute("aria-expanded", String(!collapsed));
    collapseBtn.title = collapseLabel(collapsed);
    collapseBtn.setAttribute("aria-label", collapseLabel(collapsed));
    store(NAV_KEY, collapsed ? "collapsed" : "expanded");
  });
  const group = document.getElementById("nav-admin") as HTMLDetailsElement | null;
  group?.addEventListener("toggle", () => store(ADMIN_KEY, group.open ? "open" : "closed"));

  document.getElementById("org-select")?.addEventListener("change", (e) => {
    const id = (e.target as HTMLSelectElement).value;
    const rest = location.pathname.match(/^\/app\/orgs\/[^/]+\/(events|social)/)?.[1] ?? "events";
    navigate(`/app/orgs/${id}/${rest}`);
  });
  document.getElementById("theme-toggle")!.addEventListener("click", (e) => {
    const next = THEMES[(THEMES.indexOf(currentTheme()) + 1) % THEMES.length]!;
    if (next === "system") {
      try {
        localStorage.removeItem(THEME_KEY);
      } catch {
        /* storage unavailable */
      }
    } else store(THEME_KEY, next);
    applyTheme(next);
    const btn = e.currentTarget as HTMLButtonElement;
    btn.innerHTML = html`${icon(THEME_ICON[next])}<span class="label">${themeLabel(next)}</span>`.value;
    btn.title = themeLabel(next);
  });
  bindLangSwitch();
  document.getElementById("logout")!.addEventListener("click", async () => {
    await api.logout().catch(() => undefined);
    location.href = "/app/";
  });
}
