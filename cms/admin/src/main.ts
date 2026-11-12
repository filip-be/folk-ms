// Admin SPA entry: app shell, router, sign-in gate.

import { api, ApiError, type Me } from "./api.ts";
import { L } from "./i18n.ts";
import { bindLangSwitch, langSwitch, renderShell } from "./shell.ts";
import { html, navigate, showError } from "./ui.ts";
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
import "./views/design.ts";

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
  { re: /^\/app\/design$/, tag: "folk-design", names: [], superAdmin: true },
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

/** `moveFocus`: after a client-side navigation the shell is re-rendered, so focus goes to the new view. */
function route(moveFocus = false): void {
  if (!me.orgs.length) {
    resetApp();
    document.getElementById("app")!.innerHTML = html`<main class="card narrow"><h1>${L("Brak organizacji", "No organizations")}</h1>
      <p>${L("Twoje konto nie ma przypisanych organizacji. Skontaktuj się z administratorem.", "Your account has no organizations assigned. Contact the administrator.")}</p>${langSwitch()}</main>`.value;
    bindLangSwitch();
    return;
  }
  const path = location.pathname.replace(/\/+$/, "") || "/app";
  if (path === "/app" || path === "/app/orgs") {
    history.replaceState(null, "", `/app/orgs/${currentOrg()}/events`);
  }
  renderShell(me, currentOrg());
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
    if (moveFocus) view.focus({ preventScroll: true });
    return;
  }
  view.innerHTML = html`<div class="card"><h1>${L("Nie znaleziono", "Not found")}</h1><p><a href="/app/">${L("Wróć do listy wydarzeń", "Back to the event list")}</a></p></div>`.value;
  if (moveFocus) view.focus({ preventScroll: true });
}

/** Drops the shell's layout for screens without it (sign-in, no organization). */
function resetApp(): void {
  const app = document.getElementById("app")!;
  app.className = "";
  delete app.dataset.nav;
  delete app.dataset.drawer;
}

function signInScreen(): void {
  resetApp();
  const ret = encodeURIComponent(location.pathname + location.search);
  document.getElementById("app")!.innerHTML = html`
    <main class="card narrow signin">
      <h1>folk-ms</h1>
      <p>${L("Zarządzanie wydarzeniami organizacji folkowych.", "Event management for folk organizations.")}</p>
      <div class="providers">
        <a class="button primary" href="/auth/login?provider=google&return=${ret}">${L("Zaloguj się przez Google", "Sign in with Google")}</a>
        <a class="button" href="/auth/login?provider=facebook&return=${ret}">${L("Zaloguj się przez Facebooka", "Sign in with Facebook")}</a>
      </div>
      <p class="muted small">${L("Logowanie przez Facebooka działa dla osób dodanych jako testerzy aplikacji Meta. Administrator loguje się tylko przez Google.", "Facebook sign-in works for people added as Testers of the Meta app. The administrator signs in with Google only.")}</p>
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
addEventListener("popstate", () => route(true));

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
