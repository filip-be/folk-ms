import { api } from "../api.ts";
import { html } from "../ui.ts";
import { View } from "../view.ts";

const tf = new Intl.DateTimeFormat("pl-PL", { timeZone: "Europe/Warsaw", hour: "2-digit", minute: "2-digit", second: "2-digit" });

class AuditView extends View {
  async load(): Promise<void> {
    const days = (await api.auditDays()).reverse();
    const day = new URLSearchParams(location.search).get("day") ?? days[0] ?? "";
    const entries = day ? await api.audit(day) : [];
    this.set(html`
      <div class="toolbar"><h1>Dziennik zmian</h1>
        <label>Dzień (UTC) <select id="day">${days.map((d) => html`<option ${d === day ? "selected" : ""}>${d}</option>`)}</select></label></div>
      ${entries.length
        ? html`<table class="list audit"><thead><tr><th>Czas</th><th>Kto</th><th>Akcja</th><th>Organizacja</th><th>Szczegóły</th></tr></thead><tbody>
          ${entries.map(
            (e) => html`<tr><td>${tf.format(new Date(e.time))}</td><td>${e.actorEmail ?? e.actor}</td><td><code>${e.action}</code></td>
              <td>${e.org ?? ""}</td><td class="small">${e.target ? html`<code>${e.target}</code> ` : ""}${e.details ? JSON.stringify(e.details) : ""}</td></tr>`,
          )}</tbody></table>`
        : html`<div class="card empty"><p>Brak wpisów.</p></div>`}`);
    this.querySelector("#day")?.addEventListener("change", (e) => {
      history.replaceState(null, "", `?day=${(e.target as HTMLSelectElement).value}`);
      void this.load();
    });
  }
}

customElements.define("folk-audit", AuditView);
