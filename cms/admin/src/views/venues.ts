import { api, type Venue } from "../api.ts";
import { busy, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

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
    const id = (name: string) => `venue-${v?.id ?? "new"}-${name}`;
    return html`<form class="venue-form card" data-id="${v?.id ?? ""}">
      <p class="required-note">${L("wymagane", "required")}</p>
      <div class="grid">
        <div class="field"><label for="${id("name")}">${L("Nazwa", "Name")}</label><input id="${id("name")}" name="name" required maxlength="200" value="${v?.name ?? ""}"></div>
        <div class="field"><label for="${id("address")}">${L("Adres", "Address")}</label><input id="${id("address")}" name="address" required maxlength="500" value="${v?.address ?? ""}"></div>
        <div class="field"><label for="${id("mapUrl")}">${L("Link do mapy (opcjonalnie)", "Map link (optional)")}</label><input id="${id("mapUrl")}" name="mapUrl" type="url" value="${v?.mapUrl ?? ""}" placeholder="https://"></div>
      </div>
      ${v ? html`<p class="muted small">${L("Zmiana dotyczy od razu wszystkich nadchodzących terminów w tym miejscu. Terminy, które już się odbyły, zachowają poprzednie dane.", "The change applies at once to all upcoming dates at this venue. Dates that already happened keep the previous details.")}</p>` : ""}
      <div class="actions"><button class="primary" type="submit">${v ? L("Zapisz", "Save") : L("Dodaj miejsce", "Add venue")}</button>
      ${v ? html`<button type="button" data-cancel>${L("Anuluj", "Cancel")}</button>` : ""}</div>
    </form>`;
  }

  private render(): void {
    this.set(html`
      <h1>${L("Miejsca", "Venues")}</h1>
      <p class="muted">${L("Lista miejsc wydarzeń tej organizacji. Miejsc się nie usuwa – można je zarchiwizować.", "The event venues of this organization. Venues are never deleted – they can be archived.")}</p>
      <ul class="venues">${this.venues.map((v) =>
        this.editing === v.id
          ? html`<li>${this.form(v)}</li>`
          : html`<li class="card venue ${v.archived ? "archived" : ""}">
              <div><strong>${v.name}</strong>${v.archived ? html` <span class="badge">${L("zarchiwizowane", "archived")}</span>` : ""}
                <div>${v.address}</div>${v.mapUrl ? html`<a href="${v.mapUrl}" target="_blank" rel="noopener" class="small">${L("mapa", "map")}</a>` : ""}</div>
              <div class="actions"><button type="button" data-edit="${v.id}">${L("Edytuj", "Edit")}</button>
                <button type="button" data-archive="${v.id}" data-value="${v.archived ? "0" : "1"}">${v.archived ? L("Przywróć", "Restore") : L("Archiwizuj", "Archive")}</button></div>
            </li>`,
      )}</ul>
      <h2>${L("Nowe miejsce", "New venue")}</h2>
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
          toast(L("Zapisano miejsce", "Venue saved"));
        } else {
          await api.createVenue(this.org, data);
          toast(L("Dodano miejsce", "Venue added"));
        }
        this.editing = "";
        this.venues = await api.venues(this.org);
        this.render();
      } catch (err) {
        showError(err, null, f);
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
