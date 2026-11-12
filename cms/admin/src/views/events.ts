import { api, type EventSummary } from "../api.ts";
import { badge, fmtDate, fmtDateTime, html, KIND_LABEL, navigate, raw, t } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

const TABS = [
  ["upcoming", L("Nadchodzące", "Upcoming")],
  ["drafts", L("Szkice", "Drafts")],
  ["past", L("Minione", "Past")],
  ["bin", L("Kosz", "Bin")],
] as const;

class EventsView extends View {
  async load(): Promise<void> {
    const tab = new URLSearchParams(location.search).get("view") ?? "upcoming";
    const list = await api.events(this.org, tab);
    const when = (e: EventSummary) =>
      tab === "bin"
        ? L(`usunięte ${fmtDate(e.deletedAt)}`, `deleted ${fmtDate(e.deletedAt)}`)
        : tab === "drafts"
          ? e.firstStart
            ? fmtDateTime(e.firstStart)
            : L("bez daty", "no date")
          : fmtDateTime(tab === "upcoming" ? e.nextStart : e.lastEnd);
    this.set(html`
      <div class="toolbar">
        <h1>${L("Wydarzenia", "Events")}</h1>
        <a class="button primary" href="/app/orgs/${this.org}/events/new">${L("+ Nowe wydarzenie", "+ New event")}</a>
      </div>
      <nav class="tabs" aria-label="${L("Widok", "View")}">
        ${TABS.map(([k, label]) => html`<a href="/app/orgs/${this.org}/events?view=${k}" class="${k === tab ? "active" : ""}" ${k === tab ? raw('aria-current="page"') : ""}>${label}</a>`)}
      </nav>
      ${list.length === 0
        ? html`<div class="card empty"><p>${tab === "bin" ? L("Kosz jest pusty.", "The bin is empty.") : L("Brak wydarzeń w tym widoku.", "No events in this view.")}</p></div>`
        : html`<table class="list">
            <thead><tr><th>${L("Tytuł", "Title")}</th><th>${L("Termin", "Date")}</th><th>${L("Rodzaj", "Type")}</th><th>${L("Stan", "Status")}</th></tr></thead>
            <tbody>${list.map(
              (e) => html`<tr data-id="${e.id}" tabindex="0">
                <td><a href="/app/orgs/${this.org}/events/${e.id}">${t(e.title) || L("(bez tytułu)", "(untitled)")}</a>
                  <div class="muted small">/${e.shortId}-${e.slug}</div></td>
                <td>${when(e)}${e.kind === "recurring" ? html`<div class="muted small">${L(`${e.occurrences} terminów`, `${e.occurrences} dates`)}</div>` : ""}</td>
                <td>${KIND_LABEL[e.kind] ?? "–"}</td>
                <td>${badge(e.status, e.unpublishedChanges ? L("niepublikowane zmiany", "unpublished changes") : "")}
                  ${e.facebookDetached || e.facebookOutOfDate
                    ? html` <a class="badge warn" href="/app/orgs/${this.org}/events/${e.id}/facebook">${e.facebookDetached ? L("Facebook: link bez terminu", "Facebook: detached link") : L("Facebook: do aktualizacji", "Facebook: out of date")}</a>`
                    : ""}</td>
              </tr>`,
            )}</tbody></table>`}
      ${tab === "bin" ? html`<p class="muted small">${L("Wydarzenia w koszu są usuwane na stałe po 30 dniach. Przywrócić może administrator.", "Events in the bin are deleted permanently after 30 days. The administrator can restore them.")}</p>` : ""}
    `);
    this.on("tr[data-id]", "click", (e, tr) => {
      if ((e.target as HTMLElement).closest("a")) return;
      navigate(`/app/orgs/${this.org}/events/${tr.dataset.id}`);
    });
  }
}

customElements.define("folk-events", EventsView);
