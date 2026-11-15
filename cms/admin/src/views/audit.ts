import { api, type AuditEntry } from "../api.ts";
import { html, showError } from "../ui.ts";
import { View } from "../view.ts";
import { L, locale } from "../i18n.ts";

const tf = new Intl.DateTimeFormat(locale, {
  timeZone: "Europe/Warsaw", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", second: "2-digit",
});

const row = (e: AuditEntry) => html`<tr><td>${tf.format(new Date(e.time))}</td><td>${e.actorEmail ?? e.actor}</td><td><code>${e.action}</code></td>
  <td>${e.org ?? ""}</td><td class="small">${e.target ? html`<code>${e.target}</code> ` : ""}${e.details ? JSON.stringify(e.details) : ""}</td></tr>`;

/** The Audit Log, newest first, 50 lines at a time; optionally one day. */
class AuditView extends View {
  async load(): Promise<void> {
    const day = new URLSearchParams(location.search).get("day") ?? "";
    const [days, page] = await Promise.all([api.auditDays(), api.auditPage(day)]);
    days.reverse();
    this.set(html`
      <div class="toolbar"><h1>${L("Audyt", "Audit log")}</h1>
        <label>${L("Dzień (UTC)", "Day (UTC)")} <select id="day"><option value="">${L("Wszystkie", "All days")}</option>${days.map(
          (d) => html`<option ${d === day ? "selected" : ""}>${d}</option>`,
        )}</select></label></div>
      ${page.entries.length
        ? html`<div class="table-wrap"><table class="list audit"><thead><tr><th>${L("Czas", "Time")}</th><th>${L("Kto", "Who")}</th><th>${L("Akcja", "Action")}</th><th>${L("Organizacja", "Organization")}</th><th>${L("Szczegóły", "Details")}</th></tr></thead><tbody>
          ${page.entries.map(row)}</tbody></table></div>
          <div class="actions audit-more"><button type="button" class="secondary" id="more" ${page.next ? "" : "hidden"}>${L("Wczytaj więcej", "Load more")}</button></div>`
        : html`<div class="card empty"><p>${L("Brak wpisów.", "No entries.")}</p></div>`}`);
    let next = page.next;
    this.querySelector("#day")?.addEventListener("change", (e) => {
      const v = (e.target as HTMLSelectElement).value;
      history.replaceState(null, "", v ? `?day=${v}` : location.pathname);
      void this.load();
    });
    this.querySelector<HTMLButtonElement>("#more")?.addEventListener("click", async (e) => {
      const btn = e.currentTarget as HTMLButtonElement;
      btn.disabled = true;
      try {
        const more = await api.auditPage(day, next);
        this.$("table.audit tbody").insertAdjacentHTML("beforeend", more.entries.map((x) => row(x).value).join(""));
        next = more.next;
        btn.hidden = !next;
      } catch (err) {
        showError(err);
      } finally {
        btn.disabled = false;
      }
    });
  }
}

customElements.define("folk-audit", AuditView);
