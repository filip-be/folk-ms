import { api, type Venue } from "../api.ts";
import { busy, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";

class VenuesView extends View {
  private venues: Venue[] = [];
  private editing = "";

  async load(): Promise<void> {
    this.venues = await api.venues(this.org);
    this.render();
    this.on("[data-edit]", "click", (_e, el) => {
      this.editing = el.dataset.edit!;
      this.render();
    });
    this.on("[data-cancel]", "click", () => {
      this.editing = "";
      this.render();
    });
    this.on("[data-archive]", "click", (_e, el) => void this.archive(el.dataset.archive!, el.dataset.value === "1", el as HTMLButtonElement));
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void this.save(e.target as HTMLFormElement);
    });
  }

  private form(v?: Venue): ReturnType<typeof html> {
    return html`<form class="venue-form card" data-id="${v?.id ?? ""}">
      <div class="grid">
        <div class="field"><label>Nazwa<input name="name" required maxlength="200" value="${v?.name ?? ""}"></label></div>
        <div class="field"><label>Adres<input name="address" required maxlength="500" value="${v?.address ?? ""}"></label></div>
        <div class="field"><label>Link do mapy (opcjonalnie)<input name="mapUrl" type="url" value="${v?.mapUrl ?? ""}" placeholder="https://"></label></div>
      </div>
      ${v ? html`<p class="muted small">Zmiana dotyczy od razu wszystkich nadchodzących terminów w tym miejscu. Terminy, które już się odbyły, zachowają poprzednie dane.</p>` : ""}
      <button class="primary" type="submit">${v ? "Zapisz" : "Dodaj miejsce"}</button>
      ${v ? html`<button type="button" data-cancel>Anuluj</button>` : ""}
    </form>`;
  }

  private render(): void {
    this.set(html`
      <h1>Miejsca</h1>
      <p class="muted">Lista miejsc wydarzeń tej organizacji. Miejsc się nie usuwa – można je zarchiwizować.</p>
      <ul class="venues">${this.venues.map((v) =>
        this.editing === v.id
          ? html`<li>${this.form(v)}</li>`
          : html`<li class="card venue ${v.archived ? "archived" : ""}">
              <div><strong>${v.name}</strong>${v.archived ? html` <span class="badge">zarchiwizowane</span>` : ""}
                <div>${v.address}</div>${v.mapUrl ? html`<a href="${v.mapUrl}" target="_blank" rel="noopener" class="small">mapa</a>` : ""}</div>
              <div class="actions"><button type="button" data-edit="${v.id}">Edytuj</button>
                <button type="button" data-archive="${v.id}" data-value="${v.archived ? "0" : "1"}">${v.archived ? "Przywróć" : "Archiwizuj"}</button></div>
            </li>`,
      )}</ul>
      <h2>Nowe miejsce</h2>
      ${this.form()}`);
  }

  private async save(f: HTMLFormElement): Promise<void> {
    const data = Object.fromEntries(new FormData(f)) as Record<string, string>;
    const id = f.dataset.id;
    await busy(f.querySelector("button[type=submit]"), async () => {
      try {
        if (id) {
          const cur = this.venues.find((v) => v.id === id)!;
          await api.updateVenue(this.org, id, cur.etag, data);
          toast("Zapisano miejsce");
        } else {
          await api.createVenue(this.org, data);
          toast("Dodano miejsce");
        }
        this.editing = "";
        this.venues = await api.venues(this.org);
        this.render();
      } catch (err) {
        showError(err);
      }
    });
  }

  private async archive(id: string, archived: boolean, btn: HTMLButtonElement): Promise<void> {
    await busy(btn, async () => {
      try {
        await api.archiveVenue(this.org, id, archived);
        this.venues = await api.venues(this.org);
        this.render();
      } catch (err) {
        showError(err);
      }
    });
  }
}

customElements.define("folk-venues", VenuesView);
