import { api } from "../api.ts";
import { html } from "../ui.ts";
import { View } from "../view.ts";
import { L, locale } from "../i18n.ts";

const tf = new Intl.DateTimeFormat(locale, { timeZone: "Europe/Warsaw", hour: "2-digit", minute: "2-digit", second: "2-digit" });

class AuditView extends View {
  async load(): Promise<void> {
    const days = (await api.auditDays()).reverse();
    const day = new URLSearchParams(location.search).get("day") ?? days[0] ?? "";
    const entries = day ? await api.audit(day) : [];
    this.set(html`
      <div class="toolbar"><h1>${L("Audyt", "Audit log")}</h1>
        <label>${L("Dzień (UTC)", "Day (UTC)")} <select id="day">${days.map((d) => html`<option ${d === day ? "selected" : ""}>${d}</option>`)}</select></label></div>
      ${entries.length
        ? html`<div class="table-wrap"><table class="list audit"><thead><tr><th>${L("Czas", "Time")}</th><th>${L("Kto", "Who")}</th><th>${L("Akcja", "Action")}</th><th>${L("Organizacja", "Organization")}</th><th>${L("Szczegóły", "Details")}</th></tr></thead><tbody>
          ${entries.map(
            (e) => html`<tr><td>${tf.format(new Date(e.time))}</td><td>${e.actorEmail ?? e.actor}</td><td><code>${e.action}</code></td>
              <td>${e.org ?? ""}</td><td class="small">${e.target ? html`<code>${e.target}</code> ` : ""}${e.details ? JSON.stringify(e.details) : ""}</td></tr>`,
          )}</tbody></table></div>`
        : html`<div class="card empty"><p>${L("Brak wpisów.", "No entries.")}</p></div>`}`);
    this.querySelector("#day")?.addEventListener("change", (e) => {
      history.replaceState(null, "", `?day=${(e.target as HTMLSelectElement).value}`);
      void this.load();
    });
  }
}

customElements.define("folk-audit", AuditView);
