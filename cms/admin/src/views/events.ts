import { api, type EventSummary } from "../api.ts";
import { badge, fmtDate, fmtDateTime, html, KIND_LABEL, navigate } from "../ui.ts";
import { View } from "../view.ts";

const TABS = [
  ["upcoming", "Nadchodzące"],
  ["drafts", "Szkice"],
  ["past", "Minione"],
  ["bin", "Kosz"],
] as const;

class EventsView extends View {
  async load(): Promise<void> {
    const tab = new URLSearchParams(location.search).get("view") ?? "upcoming";
    const list = await api.events(this.org, tab);
    const when = (e: EventSummary) =>
      tab === "bin"
        ? `usunięte ${fmtDate(e.deletedAt)}`
        : tab === "drafts"
          ? e.firstStart
            ? fmtDateTime(e.firstStart)
            : "bez daty"
          : fmtDateTime(tab === "upcoming" ? e.nextStart : e.lastEnd);
    this.set(html`
      <div class="toolbar">
        <h1>Wydarzenia</h1>
        <a class="button primary" href="/app/orgs/${this.org}/events/new">+ Nowe wydarzenie</a>
      </div>
      <nav class="tabs" aria-label="Widok">
        ${TABS.map(([k, label]) => html`<a href="/app/orgs/${this.org}/events?view=${k}" class="${k === tab ? "active" : ""}" ${k === tab ? 'aria-current="page"' : ""}>${label}</a>`)}
      </nav>
      ${list.length === 0
        ? html`<div class="card empty"><p>${tab === "bin" ? "Kosz jest pusty." : "Brak wydarzeń w tym widoku."}</p></div>`
        : html`<table class="list">
            <thead><tr><th>Tytuł</th><th>Termin</th><th>Rodzaj</th><th>Stan</th></tr></thead>
            <tbody>${list.map(
              (e) => html`<tr data-id="${e.id}" tabindex="0">
                <td><a href="/app/orgs/${this.org}/events/${e.id}">${e.title.pl || "(bez tytułu)"}</a>
                  <div class="muted small">/${e.shortId}-${e.slug}</div></td>
                <td>${when(e)}${e.kind === "recurring" ? html`<div class="muted small">${e.occurrences} terminów</div>` : ""}</td>
                <td>${KIND_LABEL[e.kind] ?? "–"}</td>
                <td>${badge(e.status, e.unpublishedChanges ? "niepublikowane zmiany" : "")}</td>
              </tr>`,
            )}</tbody></table>`}
      ${tab === "bin" ? html`<p class="muted small">Wydarzenia w koszu są usuwane na stałe po 30 dniach. Przywrócić może administrator.</p>` : ""}
    `);
    this.on("tr[data-id]", "click", (e, tr) => {
      if ((e.target as HTMLElement).closest("a")) return;
      navigate(`/app/orgs/${this.org}/events/${tr.dataset.id}`);
    });
  }
}

customElements.define("folk-events", EventsView);
