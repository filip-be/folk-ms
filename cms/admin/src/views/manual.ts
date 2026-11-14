// The Manual Event screen (decision 20): a co-hosted Facebook Event another
// page created, which Sync cannot read, entered by hand. Create and edit.

import { api, ApiError, type EventView, type ManualInput, type OccurrenceInput } from "../api.ts";
import { icon } from "../icons.ts";
import { L } from "../i18n.ts";
import { flagBadges } from "../patterns.ts";
import { busy, html, navigate, showError, t, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";
import { binnedBanner, enLabel, imageBlock, localDate, localTime, nextDay, permalinks, plLabel, readOnly, sharedActions, textField, upload } from "./eventParts.ts";

const blankOccurrence = (prev?: OccurrenceInput): OccurrenceInput => ({
  startDate: "",
  startTime: prev?.startTime ?? "19:00",
  endDate: "",
  endTime: prev?.endTime ?? "23:00",
  cancelled: false,
});

function emptyInput(): ManualInput {
  return {
    name: { pl: "", en: "" },
    description: { pl: "", en: "" },
    online: false,
    ticketUrl: "",
    facebookUrl: "",
    occurrences: [blankOccurrence()],
    summary: { pl: "", en: "" },
    price: { pl: "", en: "" },
  };
}

/** A stored Occurrence as the form shows it; an end on the next day before the start time needs no end date. */
function occurrenceInput(start: string, end: string, cancelled?: boolean): OccurrenceInput {
  const sd = localDate(start);
  const st = localTime(start);
  if (end === start) return { startDate: sd, startTime: st, endDate: "", endTime: "", cancelled: !!cancelled };
  const ed = localDate(end);
  const et = localTime(end);
  const implicit = ed === sd || (ed === nextDay(sd) && et <= st);
  return { startDate: sd, startTime: st, endDate: implicit ? "" : ed, endTime: et, cancelled: !!cancelled };
}

function inputOf(v: EventView): ManualInput {
  const m = v.event.manual!;
  const x = v.event.extras ?? {};
  return {
    name: { pl: m.name.pl, en: m.name.en ?? "" },
    description: { pl: m.description?.pl ?? "", en: m.description?.en ?? "" },
    location: m.location,
    online: !!m.online,
    cover: m.cover,
    ticketUrl: m.ticketUrl ?? "",
    facebookUrl: v.facebookUrl ?? "",
    occurrences: m.occurrences.map((o) => occurrenceInput(o.start, o.end, o.cancelled)),
    summary: { pl: x.summary?.pl ?? "", en: x.summary?.en ?? "" },
    price: { pl: x.price?.pl ?? "", en: x.price?.en ?? "" },
    socialImage: x.socialImage,
  };
}

export class ManualView extends View {
  /** Set by folk-event when it has already loaded a Manual Event. */
  preloaded?: EventView;
  private v?: EventView;
  private input: ManualInput = emptyInput();
  private dirty = false;

  async load(): Promise<void> {
    const v = this.preloaded ?? (this.params.id ? await api.event(this.org, this.params.id) : undefined);
    this.setView(v);
    this.render();
    this.bind();
  }

  private setView(v?: EventView): void {
    this.v = v;
    this.input = v ? inputOf(v) : emptyInput();
    this.dirty = false;
  }

  private occurrenceRow(o: OccurrenceInput, i: number, only: boolean): Raw {
    const id = (k: string) => `oc-${i}-${k}`;
    return html`<li class="occurrence-row" data-index="${i}">
      <div class="field"><label for="${id("sd")}">${L("Data", "Date")}</label>
        <input type="date" id="${id("sd")}" data-oc="startDate" data-field="occurrences[${i}].start" value="${o.startDate}" required></div>
      <div class="field"><label for="${id("st")}">${L("Początek", "Start")}</label>
        <input type="time" id="${id("st")}" data-oc="startTime" value="${o.startTime}" required></div>
      <div class="field"><label for="${id("ed")}">${L("Data końca", "End date")} <span class="muted">${L("(opcjonalnie)", "(optional)")}</span></label>
        <input type="date" id="${id("ed")}" data-oc="endDate" value="${o.endDate}"></div>
      <div class="field"><label for="${id("et")}">${L("Koniec", "End")}</label>
        <input type="time" id="${id("et")}" data-oc="endTime" data-field="occurrences[${i}].end" value="${o.endTime}"></div>
      <label class="inline" for="${id("c")}"><input type="checkbox" id="${id("c")}" data-oc="cancelled" ${o.cancelled ? "checked" : ""}> ${L("odwołany", "cancelled")}</label>
      <button type="button" class="link danger" data-remove-occurrence="${i}" ${only ? "disabled" : ""}
        aria-label="${L(`Usuń termin ${i + 1}`, `Remove date ${i + 1}`)}">${L("Usuń", "Remove")}</button>
    </li>`;
  }

  private render(): void {
    const c = this.input;
    const v = this.v;
    const binned = !!v?.flags.binned;
    const loc = c.location ?? { name: "", address: "", mapUrl: "" };
    this.set(html`
      <form id="manual" class="editor">
        <a href="/app/orgs/${this.org}/events" class="muted small">← ${L("Wydarzenia", "Events")}</a>
        <div class="toolbar sticky">
          <div><h1>${v ? t(v.effective.name) || L("(bez nazwy)", "(no name)") : L("Współorganizowane wydarzenie", "Co-hosted event")}</h1>
            ${v ? flagBadges(v.flags, v.event.binReason) : ""}</div>
          <div class="actions">
            ${binned ? "" : html`<button type="submit" class="primary" id="save">${v ? L("Zapisz", "Save") : L("Dodaj wydarzenie", "Add event")}</button>`}
            ${v ? sharedActions(this.me, this.org, v) : ""}
            ${v && !binned ? html`<button type="button" class="danger" id="remove">${L("Przenieś do kosza", "Move to the Bin")}</button>` : ""}
          </div>
        </div>
        ${v ? binnedBanner(this.me, v) : ""}
        <div class="banner info"><p>${L(
          "Wydarzenie dodane ręcznie: utworzyła je na Facebooku inna strona (np. współorganizowane), więc synchronizacja nie może go odczytać ani zaktualizować. Gdy coś zmieni się na Facebooku, popraw to tutaj.",
          "A manual event: another page created it on Facebook (for example one you co-host), so Sync can neither read nor update it. When something changes on Facebook, change it here too.",
        )}</p></div>
        ${v?.flags.syncConflict
          ? html`<div class="banner warn" role="status"><p>${L(
              "Synchronizacja znalazła to wydarzenie na połączonej stronie. Pozostaje dodane ręcznie; jeśli wolisz, by aktualizowała je synchronizacja, przenieś je do kosza i zsynchronizuj.",
              "Sync found this Facebook Event on the connected page. It stays manual; to have Sync keep it up to date instead, move it to the Bin and sync.",
            )}</p></div>`
          : ""}
        <div id="problems"></div>
        ${v ? permalinks(v) : ""}
        <p class="required-note">${L("wymagane", "required")}</p>

        <section class="card" aria-labelledby="m-fb-title">
          <h2 id="m-fb-title">${L("Wydarzenie na Facebooku", "Facebook Event")}</h2>
          ${textField({
            id: "m-facebookUrl",
            label: L("Link do wydarzenia na Facebooku", "Facebook Event link"),
            value: c.facebookUrl,
            path: "facebookUrl",
            kind: "url",
            required: true,
            readonly: !!v,
            hint: v
              ? L("Linku nie można zmienić. Aby wskazać inne wydarzenie, przenieś to do kosza i dodaj nowe.", "The link cannot change. To point to another event, move this one to the Bin and add a new one.")
              : L("Np. https://www.facebook.com/events/123456789/. CMS nigdy go nie otwiera; strona pokazuje go przy każdym terminie.", "E.g. https://www.facebook.com/events/123456789/. The CMS never opens it; the Site links to it from each date."),
          })}
        </section>

        <section class="card" aria-labelledby="m-text-title">
          <h2 id="m-text-title">${L("Nazwa i opis", "Name and description")}</h2>
          <div class="pair">
            ${textField({ id: "m-name-pl", label: plLabel(L("Nazwa", "Name")), value: c.name.pl, path: "name.pl", required: true, max: 200 })}
            ${textField({ id: "m-name-en", label: enLabel(L("Nazwa", "Name")), value: c.name.en, path: "name.en", max: 200 })}
          </div>
          <div class="pair">
            ${textField({ id: "m-description-pl", label: plLabel(L("Opis", "Description")), value: c.description.pl, path: "description.pl", kind: "long", max: 10000 })}
            ${textField({ id: "m-description-en", label: enLabel(L("Opis", "Description")), value: c.description.en, path: "description.en", kind: "long", max: 10000 })}
          </div>
        </section>

        <section class="card" aria-labelledby="m-dates-title">
          <h2 id="m-dates-title">${L("Terminy", "Dates")}</h2>
          <fieldset data-field="occurrences"><legend class="required">${L("Terminy, każdy osobno", "Dates, one by one")}</legend>
            <ol class="occurrence-list">${c.occurrences.map((o, i) => this.occurrenceRow(o, i, c.occurrences.length === 1))}</ol>
          </fieldset>
          <p class="muted small">${L(
            "Godzina końca wcześniejsza niż początek oznacza zakończenie następnego dnia. Datę końca podaj tylko dla terminu trwającego kilka dni.",
            "An end time earlier than the start time means it ends the next day. Give an end date only for a date that lasts several days.",
          )}</p>
          <button type="button" id="add-occurrence">${icon("plus")}${L("Dodaj termin", "Add a date")}</button>
        </section>

        <section class="card" aria-labelledby="m-place-title">
          <h2 id="m-place-title">${L("Miejsce i bilety", "Location and tickets")}</h2>
          <fieldset data-field="location"><legend class="sr-only">${L("Miejsce", "Location")}</legend>
            <div class="grid">
              ${textField({ id: "m-loc-name", label: L("Nazwa miejsca", "Place name"), value: loc.name, path: "location.name", max: 200 })}
              ${textField({ id: "m-loc-address", label: L("Adres", "Address"), value: loc.address, path: "location.address", max: 500 })}
              ${textField({ id: "m-loc-map", label: L("Link do mapy", "Map link"), value: loc.mapUrl, path: "location.mapUrl", kind: "url" })}
            </div>
          </fieldset>
          <label class="inline" for="m-online"><input type="checkbox" id="m-online" name="online" ${c.online ? "checked" : ""}> ${L("Wydarzenie online", "Online event")}</label>
          ${textField({ id: "m-ticketUrl", label: L("Link do biletów lub zapisów", "Tickets or registration link"), value: c.ticketUrl, path: "ticketUrl", kind: "url" })}
        </section>

        <section class="card images" aria-labelledby="m-img-title">
          <h2 id="m-img-title">${L("Obrazy", "Images")}</h2>
          ${imageBlock(this.me, "cover", c.cover, L("Zdjęcie główne (poziome)", "Cover image (landscape)"), binned)}
          ${c.cover
            ? html`<div class="pair">
                ${textField({ id: "m-alt-pl", label: plLabel(L("Tekst alternatywny", "Alternative text")), value: c.cover.alt?.pl, path: "cover.alt.pl", max: 200 })}
                ${textField({ id: "m-alt-en", label: enLabel(L("Tekst alternatywny", "Alternative text")), value: c.cover.alt?.en, path: "cover.alt.en", max: 200 })}
              </div>`
            : ""}
          ${imageBlock(this.me, "social", c.socialImage, L("Obraz na Instagram (kwadrat lub 4:5, opcjonalny)", "Instagram image (square or 4:5, optional)"), binned)}
        </section>

        <section class="card" aria-labelledby="m-extra-title">
          <h2 id="m-extra-title">${L("Skrót i cena", "Summary and price")}</h2>
          <div class="pair">
            ${textField({ id: "m-summary-pl", label: plLabel(L("Skrót", "Summary")), value: c.summary.pl, path: "summary.pl", kind: "short", max: 500 })}
            ${textField({ id: "m-summary-en", label: enLabel(L("Skrót", "Summary")), value: c.summary.en, path: "summary.en", kind: "short", max: 500 })}
            </div>
            <div class="pair">
            ${textField({ id: "m-price-pl", label: plLabel(L("Cena / wstęp", "Price / admission")), value: c.price.pl, path: "price.pl", max: 500 })}
            ${textField({ id: "m-price-en", label: enLabel(L("Cena / wstęp", "Price / admission")), value: c.price.en, path: "price.en", max: 500 })}
          </div>
        </section>
      </form>`);
    if (binned) readOnly(this);
  }

  /** Reads the form into this.input. */
  private collect(): ManualInput {
    const f = this.querySelector<HTMLFormElement>("#manual");
    if (!f) return this.input;
    const val = (id: string) => (f.querySelector<HTMLInputElement>(`#${id}`)?.value ?? "").trim();
    const c = this.input;
    c.facebookUrl = val("m-facebookUrl");
    c.name = { pl: val("m-name-pl"), en: val("m-name-en") };
    c.description = { pl: val("m-description-pl"), en: val("m-description-en") };
    const loc = { name: val("m-loc-name"), address: val("m-loc-address"), mapUrl: val("m-loc-map") };
    c.location = loc.name || loc.address || loc.mapUrl ? loc : undefined;
    c.online = f.querySelector<HTMLInputElement>("#m-online")!.checked;
    c.ticketUrl = val("m-ticketUrl");
    if (c.cover && f.querySelector("#m-alt-pl")) c.cover = { ...c.cover, alt: { pl: val("m-alt-pl"), en: val("m-alt-en") } };
    c.summary = { pl: val("m-summary-pl"), en: val("m-summary-en") };
    c.price = { pl: val("m-price-pl"), en: val("m-price-en") };
    c.occurrences = [...f.querySelectorAll<HTMLElement>(".occurrence-row")].map((row) => {
      const g = (k: string) => row.querySelector<HTMLInputElement>(`[data-oc=${k}]`)!;
      return { startDate: g("startDate").value, startTime: g("startTime").value, endDate: g("endDate").value, endTime: g("endTime").value, cancelled: g("cancelled").checked };
    });
    return c;
  }

  private async save(btn: HTMLButtonElement | null): Promise<void> {
    const c = this.collect();
    const problems = this.querySelector<HTMLElement>("#problems");
    await busy(btn, async () => {
      try {
        if (!this.v) {
          const v = await api.createManual(this.org, c);
          this.dirty = false;
          toast(L("Dodano wydarzenie – strona zaktualizuje się w ciągu minuty", "Event added – the Site updates within a minute"));
          navigate(`/app/orgs/${this.org}/events/${v.event.id}`);
          return;
        }
        const v = await api.updateManual(this.org, this.v.event.id, this.v.etag, c);
        this.setView(v);
        this.render();
        toast(L("Zapisano – strona zaktualizuje się w ciągu minuty", "Saved – the Site updates within a minute"));
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          toast(L("Ktoś inny zmienił to wydarzenie. Odśwież stronę, aby zobaczyć zmiany (Twoje zmiany nie zostały zapisane).", "Someone else changed this event. Refresh the page to see the changes (your changes were not saved)."), "error");
          return;
        }
        showError(err, problems, this.querySelector("#manual"));
      }
    });
  }

  private async action(name: "hide" | "show" | "delete" | "restore", control: HTMLInputElement | HTMLButtonElement): Promise<void> {
    if (name === "delete" && !confirm(L("Przenieść wydarzenie do kosza? Zniknie ze strony. Posty na Facebooku i Instagramie pozostaną.", "Move the event to the Bin? It leaves the Site. Facebook and Instagram posts stay."))) return;
    control.disabled = true;
    try {
      const v = await api.eventAction(this.org, this.v!.event.id, name);
      if (name === "delete") {
        this.dirty = false;
        toast(L("Przeniesiono do kosza", "Moved to the Bin"));
        navigate(`/app/orgs/${this.org}/events`);
        return;
      }
      // Keep unsaved edits: only the version and the flags change.
      const edits = this.collect();
      const dirty = this.dirty;
      this.setView(v);
      if (dirty && name !== "restore") {
        this.input = edits;
        this.dirty = true;
      }
      this.render();
      toast({ hide: L("Ukryto – strona zaktualizuje się w ciągu minuty", "Hidden – the Site updates within a minute"), show: L("Pokazano na stronie", "Shown on the Site"), restore: L("Przywrócono z kosza", "Restored from the Bin") }[name]);
    } catch (err) {
      if (control instanceof HTMLInputElement) control.checked = !control.checked;
      control.disabled = false;
      showError(err);
    }
  }

  private bind(): void {
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void this.save(this.querySelector<HTMLButtonElement>("#save"));
    });
    this.on("#add-occurrence", "click", () => {
      const c = this.collect();
      c.occurrences.push(blankOccurrence(c.occurrences.at(-1)));
      this.dirty = true;
      this.render();
      this.querySelector<HTMLInputElement>(`#oc-${c.occurrences.length - 1}-sd`)?.focus();
    });
    this.on("[data-remove-occurrence]", "click", (_e, el) => {
      const c = this.collect();
      const i = Number(el.dataset.removeOccurrence);
      c.occurrences.splice(i, 1);
      this.dirty = true;
      this.render();
      this.querySelector<HTMLInputElement>(`#oc-${Math.min(i, c.occurrences.length - 1)}-sd`)?.focus();
    });
    this.on("[data-upload]", "change", (_e, el) => {
      const kind = el.dataset.upload as "cover" | "social";
      void upload(this.org, kind, el as HTMLInputElement).then((img) => {
        if (!img) return;
        this.collect();
        if (kind === "cover") this.input.cover = { ...img, alt: this.input.cover?.alt };
        else this.input.socialImage = img;
        this.dirty = true;
        this.render();
        toast(L("Wgrano obraz – zapisz zmiany", "Image uploaded – save the changes"));
      });
    });
    this.on("[data-remove-image]", "click", (_e, el) => {
      this.collect();
      if (el.dataset.removeImage === "cover") this.input.cover = undefined;
      else this.input.socialImage = undefined;
      this.dirty = true;
      this.render();
    });
    this.on("#hidden", "change", (_e, el) => void this.action((el as HTMLInputElement).checked ? "hide" : "show", el as HTMLInputElement));
    this.on("#remove", "click", (_e, el) => void this.action("delete", el as HTMLButtonElement));
    this.on("[data-action=restore]", "click", (_e, el) => void this.action("restore", el as HTMLButtonElement));
    this.addEventListener("input", (e) => {
      const target = e.target as HTMLInputElement;
      if (target.type !== "file" && target.getAttribute("role") !== "switch") this.dirty = true;
    });
    addEventListener("beforeunload", this.guard);
  }

  private guard = (e: BeforeUnloadEvent) => {
    if (this.dirty && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
  }
}

customElements.define("folk-manual", ManualView);
