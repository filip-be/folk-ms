// Feature flags (Super Admin): every known flag, on or off, with who changed
// it last. A change needs a Recent sign-in and is written to the Audit Log.

import { api, type FeatureFlag } from "../api.ts";
import { busy, fmtDateTime, html, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

class FeaturesView extends View {
  private flags: FeatureFlag[] = [];

  async load(): Promise<void> {
    this.flags = await api.featureFlags();
    this.render();
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void this.toggle(e.target as HTMLFormElement);
    });
  }

  private render(): void {
    this.set(html`
      <h1>${L("Flagi funkcji", "Feature flags")}</h1>
      <p class="muted">${L(
        "Włączają i wyłączają części CMS dla wszystkich. Każda zmiana działa od razu i trafia do dziennika audytu.",
        "Turn parts of the CMS on or off for everyone. Each change takes effect at once and is written to the Audit log.",
      )}</p>
      ${this.flags.map(
        (f) => html`<section class="card" aria-labelledby="flag-${f.key}">
          <h2 id="flag-${f.key}">${t(f.name)}
            <span class="badge ${f.enabled ? "on" : ""}">${f.enabled ? L("włączona", "on") : L("wyłączona", "off")}</span></h2>
          <p>${t(f.description)}</p>
          <p class="muted small">${f.changedAt
            ? html`${L("Zmieniona", "Changed")} ${fmtDateTime(f.changedAt)}${f.changedByEmail ? html` ${L("przez", "by")} ${f.changedByEmail}` : ""}.`
            : html`${L("Domyślnie", "Default")}: ${f.default ? L("włączona", "on") : L("wyłączona", "off")}.`}</p>
          <form data-flag="${f.key}" data-enable="${f.enabled ? "false" : "true"}">
            <button type="submit" class="${f.enabled ? "" : "primary"}">${f.enabled ? L("Wyłącz", "Turn off") : L("Włącz", "Turn on")}</button>
          </form>
        </section>`,
      )}`);
  }

  private async toggle(f: HTMLFormElement): Promise<void> {
    const on = f.dataset.enable === "true";
    await busy(f.querySelector("button"), async () => {
      try {
        this.flags = await api.setFeatureFlag(f.dataset.flag!, on);
        toast(on ? L("Włączono", "Turned on") : L("Wyłączono", "Turned off"));
        this.render();
      } catch (err) {
        showError(err, null, f);
      }
    });
  }
}

customElements.define("folk-features", FeaturesView);
