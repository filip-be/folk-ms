// The Event list: views, flags, the sync status panel with "Sync now", and
// the connection banners.

import { api, ApiError, type ConnectionStatus, type EventSummary, type SyncResult, type SyncStatus } from "../api.ts";
import { connectionBanners, flagBadges } from "../patterns.ts";
import { busy, fmtDate, fmtDateTime, html, navigate, raw, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";
import { icon } from "../icons.ts";

const TABS = [
  ["upcoming", L("Nadchodzące", "Upcoming")],
  ["past", L("Minione", "Past")],
  ["hidden", L("Ukryte", "Hidden")],
  ["bin", L("Kosz", "Bin")],
] as const;
type Tab = (typeof TABS)[number][0];

/** What the Meta callback of "Sync now" reported (`?sync=`). */
function syncOutcome(reason: string, pageName: string): [string, "ok" | "error"] {
  switch (reason) {
    case "ok":
      return [L("Synchronizacja zakończona.", "Sync finished."), "ok"];
    case "cancelled":
      return [L("Anulowano logowanie do Meta. Synchronizacja nie została uruchomiona.", "The Meta sign-in was cancelled. Sync did not run."), "error"];
    case "page":
      return [
        L(`Twoje konto Meta nie ma roli na stronie „${pageName}”. Synchronizacja nie została uruchomiona.`, `Your Meta account has no role on the Facebook page “${pageName}”. Sync did not run.`),
        "error",
      ];
    case "too_soon":
      return [L("Synchronizacja była przed chwilą – spróbuj ponownie za minutę.", "A sync ran moments ago – try again in a minute."), "error"];
    case "running":
      return [L("Synchronizacja już trwa – odśwież za chwilę.", "A sync is already running – refresh in a moment."), "error"];
    default:
      return [L("Synchronizacja nie powiodła się. Szczegóły w panelu synchronizacji.", "Sync failed. See the sync panel for details."), "error"];
  }
}

function resultText(r: SyncResult): string {
  return L(
    `Na Facebooku: ${r.listed}; nowe: ${r.added}, zmienione: ${r.updated}, do kosza: ${r.binned}${r.conflicts ? `, zajęte przez wydarzenia dodane ręcznie: ${r.conflicts}` : ""}.`,
    `On Facebook: ${r.listed}; new: ${r.added}, changed: ${r.updated}, to the Bin: ${r.binned}${r.conflicts ? `, held by manual events: ${r.conflicts}` : ""}.`,
  );
}

class EventsView extends View {
  private tab: Tab = "upcoming";
  private list: EventSummary[] = [];
  private sync!: SyncStatus;
  private conn!: ConnectionStatus;

  async load(): Promise<void> {
    const q = new URLSearchParams(location.search);
    const v = q.get("view");
    this.tab = TABS.some(([k]) => k === v) ? (v as Tab) : "upcoming";
    [this.list, this.sync, this.conn] = await Promise.all([api.events(this.org, this.tab), api.syncStatus(this.org), api.social(this.org)]);
    this.render();
    const outcome = q.get("sync");
    if (outcome) {
      const [msg, kind] = syncOutcome(outcome, this.conn.pageName ?? "");
      toast(msg, kind);
      q.delete("sync");
      history.replaceState(null, "", location.pathname + (q.size ? `?${q}` : ""));
    }
    this.on("tr[data-id]", "click", (e, tr) => {
      if ((e.target as HTMLElement).closest("a")) return;
      navigate(`/app/orgs/${this.org}/events/${tr.dataset.id}`);
    });
    this.on("tr[data-id]", "keydown", (e, tr) => {
      if ((e as KeyboardEvent).key === "Enter" && e.target === tr) navigate(`/app/orgs/${this.org}/events/${tr.dataset.id}`);
    });
    this.on("#sync-now", "click", (_e, el) => void this.syncNow(el as HTMLButtonElement));
  }

  private async syncNow(btn: HTMLButtonElement): Promise<void> {
    await busy(btn, async () => {
      try {
        const res = await api.syncNow(this.org);
        if (res.url) {
          location.href = res.url; // the Meta sign-in; its callback runs the Sync and comes back here
          return;
        }
        toast(L("Synchronizacja zakończona. ", "Sync finished. ") + (res.result ? resultText(res.result) : ""));
        [this.list, this.sync] = await Promise.all([api.events(this.org, this.tab), api.syncStatus(this.org)]);
        this.render();
      } catch (err) {
        if (err instanceof ApiError && err.code === "too_soon") {
          toast(syncOutcome("too_soon", "")[0], "error");
          return;
        }
        if (err instanceof ApiError && err.code === "conflict") {
          toast(syncOutcome("running", "")[0], "error");
          return;
        }
        showError(err);
      }
    });
  }

  private when(e: EventSummary): ReturnType<typeof html> {
    switch (this.tab) {
      case "bin":
        return html`${L("w koszu od", "in the Bin since")} ${fmtDate(e.deletedAt)}`;
      case "upcoming":
        return html`${fmtDateTime(e.nextStart)}`;
      default:
        return html`${fmtDateTime(e.nextStart ?? e.firstStart)}`;
    }
  }

  private syncPanel(): ReturnType<typeof html> {
    const s = this.sync;
    const by = s.lastRunBy === "autosync" ? L("Autosync", "Autosync") : s.lastRunBy ? L("moderator", "a Moderator") : "";
    const failed = s.lastError && (!s.lastSuccessAt || (!!s.lastRunAt && Date.parse(s.lastRunAt) > Date.parse(s.lastSuccessAt)));
    return html`<section class="card sync-panel" aria-labelledby="sync-title">
      <div class="section-head">
        <h2 id="sync-title">${L("Synchronizacja z Facebookiem", "Sync with Facebook")}</h2>
        <span class="badges">
          ${s.running ? html`<span class="badge info">${L("trwa", "running")}</span>` : ""}
          <span class="badge ${s.autosync ? "posted" : ""}">${s.autosync ? L("Autosync włączony", "Autosync on") : L("Autosync wyłączony", "Autosync off")}</span>
        </span>
      </div>
      <dl class="facts">
        <dt>${L("Ostatnia synchronizacja", "Last sync")}</dt><dd>${s.lastRunAt ? html`${fmtDateTime(s.lastRunAt)}${by ? html` <span class="muted">(${by})</span>` : ""}` : L("jeszcze nie było", "none yet")}</dd>
        <dt>${L("Ostatnia udana", "Last success")}</dt><dd>${s.lastSuccessAt ? fmtDateTime(s.lastSuccessAt) : "–"}${s.lastResult && !failed ? html`<div class="muted small">${resultText(s.lastResult)}</div>` : ""}</dd>
        ${s.lastError ? html`<dt>${L("Ostatni błąd", "Last error")}</dt><dd class="${failed ? "error" : "muted"}">${s.lastError}</dd>` : ""}
      </dl>
      ${s.autosync
        ? html`<p class="muted small">${L("Autosync synchronizuje wydarzenia co godzinę.", "Autosync syncs the events every hour.")}</p>`
        : html`<p class="small">${L(
            "Autosync jest wyłączony: nic nie aktualizuje się samo. Po zmianach na Facebooku naciśnij „Synchronizuj teraz” (wymaga logowania do Meta).",
            "Autosync is off: nothing updates unattended. After changing events on Facebook, press “Sync now” (it needs a Meta sign-in).",
          )}</p>`}
      <button type="button" class="primary" id="sync-now" ${this.conn.connected && !s.running ? "" : "disabled"}>${icon("sync")}${s.autosync ? L("Synchronizuj teraz", "Sync now") : L("Zaloguj się do Meta i synchronizuj", "Sign in with Meta and sync")}</button>
    </section>`;
  }

  private render(): void {
    const list = this.list;
    this.set(html`
      <div class="toolbar">
        <h1>${L("Wydarzenia", "Events")}</h1>
        <a class="button" href="/app/orgs/${this.org}/events/new">${icon("plus")}${L("Dodaj współorganizowane wydarzenie", "Add co-hosted event")}</a>
      </div>
      ${connectionBanners(this.conn, this.org)}
      ${this.syncPanel()}
      <nav class="tabs" aria-label="${L("Widok", "View")}">
        ${TABS.map(([k, label]) => html`<a href="/app/orgs/${this.org}/events?view=${k}" class="${k === this.tab ? "active" : ""}" ${k === this.tab ? raw('aria-current="page"') : ""}>${label}</a>`)}
      </nav>
      ${list.length === 0
        ? html`<div class="card empty"><p>${this.tab === "bin" ? L("Kosz jest pusty.", "The Bin is empty.") : L("Brak wydarzeń w tym widoku.", "No events in this view.")}</p></div>`
        : html`<div class="table-wrap"><table class="list">
            <thead><tr><th>${L("Nazwa", "Name")}</th><th>${this.tab === "upcoming" ? L("Najbliższy termin", "Next date") : this.tab === "bin" ? L("W koszu", "In the Bin") : L("Termin", "Date")}</th><th>${L("Terminy", "Dates")}</th></tr></thead>
            <tbody>${list.map(
              (e) => html`<tr data-id="${e.id}" tabindex="0">
                <td><a href="/app/orgs/${this.org}/events/${e.id}">${t(e.name) || L("(bez nazwy)", "(no name)")}</a>
                  <div class="muted small">/${e.shortId}-${e.slug}</div>
                  ${flagBadges(e.flags, e.binReason)}</td>
                <td>${this.when(e)}</td>
                <td>${e.occurrences}</td>
              </tr>`,
            )}</tbody></table></div>`}
      ${this.tab === "bin" ? html`<p class="muted small">${L("Wydarzenia w koszu są usuwane na stałe po 30 dniach. Przywrócić może administrator.", "Events in the Bin are deleted permanently after 30 days. The administrator can restore them.")}</p>` : ""}
      ${this.tab === "hidden" ? html`<p class="muted small">${L("Ukrytych wydarzeń nie ma na stronie. Synchronizacja nigdy ich nie odkrywa.", "Hidden events are not on the Site. Sync never shows them again.")}</p>` : ""}
    `);
  }
}

customElements.define("folk-events", EventsView);
