// Admin SPA entry: app shell, router, sign-in gate.

import { api, ApiError, type Me } from "./api.ts";
import { L, lang, setLang } from "./i18n.ts";
import { html, navigate, showError, t } from "./ui.ts";
import { View } from "./view.ts";
import "./views/events.ts";
import "./views/editor.ts";
import "./views/post.ts";
import "./views/facebookEvent.ts";
import { relayReports } from "./autofill.ts";
import "./views/venues.ts";
import "./views/social.ts";
import "./views/access.ts";
import "./views/audit.ts";

interface Route {
  re: RegExp;
  tag: string;
  names: string[];
  superAdmin?: boolean;
}

const routes: Route[] = [
  { re: /^\/app\/orgs\/([^/]+)\/events\/?$/, tag: "folk-events", names: ["org"] },
  { re: /^\/app\/orgs\/([^/]+)\/events\/new$/, tag: "folk-editor", names: ["org"] },
  { re: /^\/app\/orgs\/([^/]+)\/events\/([a-z0-9]+)\/post$/, tag: "folk-post", names: ["org", "id"] },
  { re: /^\/app\/orgs\/([^/]+)\/events\/([a-z0-9]+)\/facebook$/, tag: "folk-facebook-event", names: ["org", "id"] },
  { re: /^\/app\/orgs\/([^/]+)\/events\/([a-z0-9]+)$/, tag: "folk-editor", names: ["org", "id"] },
  { re: /^\/app\/orgs\/([^/]+)\/venues$/, tag: "folk-venues", names: ["org"] },
  { re: /^\/app\/orgs\/([^/]+)\/social$/, tag: "folk-social", names: ["org"] },
  { re: /^\/app\/access$/, tag: "folk-access", names: [], superAdmin: true },
  { re: /^\/app\/audit$/, tag: "folk-audit", names: [], superAdmin: true },
];

let me: Me;
const ORG_KEY = "folkms.org";

function currentOrg(): string {
  const m = location.pathname.match(/^\/app\/orgs\/([^/]+)/);
  if (m) return decodeURIComponent(m[1]!);
  try {
    const saved = localStorage.getItem(ORG_KEY);
    if (saved && me.orgs.some((o) => o.id === saved)) return saved;
  } catch {
    /* storage unavailable */
  }
  return me.orgs[0]?.id ?? "";
}

/** Switches to the other language; its label is in that language. */
function langSwitch() {
  const other = lang === "pl" ? "en" : "pl";
  return html`<button type="button" class="link lang-switch" data-set-lang="${other}" lang="${other}"
    title="${other === "en" ? "Switch to English" : "Przełącz na polski"}">${other === "en" ? "English" : "Polski"}</button>`;
}

function bindLangSwitch(): void {
  document.querySelector<HTMLElement>("[data-set-lang]")?.addEventListener("click", (e) => {
    setLang((e.currentTarget as HTMLElement).dataset.setLang as "pl" | "en");
  });
}

function shell(): void {
  const org = currentOrg();
  const section = location.pathname.split("/")[4] ?? location.pathname.split("/")[2] ?? "";
  const link = (href: string, label: string, key: string) =>
    html`<a href="${href}" class="${section === key ? "active" : ""}">${label}</a>`;
  document.getElementById("app")!.innerHTML = html`
    <header class="top">
      <a class="brand" href="/app/">folk-ms</a>
      ${me.orgs.length > 1
        ? html`<label class="org-switch"><span class="sr-only">${L("Organizacja", "Organization")}</span>
            <select id="org-select">${me.orgs.map((o) => html`<option value="${o.id}" ${o.id === org ? "selected" : ""}>${t(o.name)}</option>`)}</select></label>`
        : html`<span class="org-name">${t(me.orgs[0]?.name)}</span>`}
      <nav>
        ${link(`/app/orgs/${org}/events`, L("Wydarzenia", "Events"), "events")}
        ${link(`/app/orgs/${org}/venues`, L("Miejsca", "Venues"), "venues")}
        ${link(`/app/orgs/${org}/social`, L("Media społecznościowe", "Social media"), "social")}
        ${me.superAdmin ? html`${link("/app/access", L("Moderatorzy", "Moderators"), "access")}${link("/app/audit", L("Dziennik", "Audit log"), "audit")}` : ""}
      </nav>
      <span class="user" title="${me.email}">${me.name || me.email}</span>
      ${langSwitch()}
      <button id="logout" class="link">${L("Wyloguj", "Sign out")}</button>
    </header>
    <main id="view"></main>`.value;
  document.getElementById("org-select")?.addEventListener("change", (e) => {
    const id = (e.target as HTMLSelectElement).value;
    const rest = location.pathname.match(/^\/app\/orgs\/[^/]+\/(events|venues|social)/)?.[1] ?? "events";
    navigate(`/app/orgs/${id}/${rest}`);
  });
  bindLangSwitch();
  document.getElementById("logout")!.addEventListener("click", async () => {
    await api.logout().catch(() => undefined);
    location.href = "/app/";
  });
}

function route(): void {
  if (!me.orgs.length) {
    document.getElementById("app")!.innerHTML = html`<main class="card narrow"><h1>${L("Brak organizacji", "No organizations")}</h1>
      <p>${L("Twoje konto nie ma przypisanych organizacji. Skontaktuj się z administratorem.", "Your account has no organizations assigned. Contact the administrator.")}</p>${langSwitch()}</main>`.value;
    bindLangSwitch();
    return;
  }
  const path = location.pathname.replace(/\/+$/, "") || "/app";
  if (path === "/app" || path === "/app/orgs") {
    history.replaceState(null, "", `/app/orgs/${currentOrg()}/events`);
  }
  shell();
  const view = document.getElementById("view")!;
  for (const r of routes) {
    const m = location.pathname.match(r.re);
    if (!m) continue;
    if (r.superAdmin && !me.superAdmin) break;
    const params: Record<string, string> = {};
    r.names.forEach((n, i) => (params[n] = decodeURIComponent(m[i + 1]!)));
    if (params.org) {
      if (!me.orgs.some((o) => o.id === params.org)) break;
      try {
        localStorage.setItem(ORG_KEY, params.org);
      } catch {
        /* ignore */
      }
    }
    const el = document.createElement(r.tag) as View;
    el.params = params;
    el.me = me;
    view.replaceChildren(el);
    window.scrollTo(0, 0);
    return;
  }
  view.innerHTML = html`<div class="card"><h1>${L("Nie znaleziono", "Not found")}</h1><p><a href="/app/">${L("Wróć do listy wydarzeń", "Back to the event list")}</a></p></div>`.value;
}

function signInScreen(): void {
  const ret = encodeURIComponent(location.pathname + location.search);
  document.getElementById("app")!.innerHTML = html`
    <main class="card narrow signin">
      <h1>folk-ms</h1>
      <p>${L("Zarządzanie wydarzeniami organizacji folkowych.", "Event management for folk organizations.")}</p>
      <a class="button primary" href="/auth/login?return=${ret}">${L("Zaloguj się przez Google", "Sign in with Google")}</a>
      <p class="muted small">${L("Dostęp mają wyłącznie osoby zaproszone przez administratora.", "Only people invited by the administrator have access.")}</p>
      ${langSwitch()}
    </main>`.value;
  bindLangSwitch();
}

document.addEventListener("click", (e) => {
  const a = (e.target as HTMLElement).closest("a");
  if (!a || a.target || e.ctrlKey || e.metaKey || e.shiftKey || e.button !== 0) return;
  const href = a.getAttribute("href");
  if (href?.startsWith("/app/")) {
    e.preventDefault();
    navigate(href);
  }
});
addEventListener("popstate", route);

async function start(): Promise<void> {
  try {
    me = await api.me();
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) {
      signInScreen();
      return;
    }
    showError(err);
    return;
  }
  // Autofill keeps failure reports until a CMS page is open.
  relayReports((org) => me.orgs.some((o) => o.id === org));
  route();
}

void start();
