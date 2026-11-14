// The Event screen. A synced Event shows its Facebook values read-only beside
// each Override control, its dates (never overridable), and the fields the
// CMS adds; a Manual Event is handed to folk-manual (manual.ts).

import { api, ApiError, type EventView, type ExtrasInput, type Location, type OverrideField, type OverrideInput } from "../api.ts";
import { L } from "../i18n.ts";
import { FIELD_LABEL, flagBadges } from "../patterns.ts";
import { busy, facebookEventUrl, fmtDateTime, fmtTime, html, showError, t, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";
import { binnedBanner, enLabel, imageBlock, localDate, permalinks, publicUrl, readOnly, sharedActions, textField, upload } from "./eventParts.ts";
import type { ManualView } from "./manual.ts";

const FIELDS: OverrideField[] = ["name", "description", "location", "cover", "ticketUrl"];

function extrasOf(v: EventView): ExtrasInput {
  const x = v.event.extras ?? {};
  return {
    nameEn: x.nameEn ?? "",
    descriptionEn: x.descriptionEn ?? "",
    summary: { pl: x.summary?.pl ?? "", en: x.summary?.en ?? "" },
    price: { pl: x.price?.pl ?? "", en: x.price?.en ?? "" },
    socialImage: x.socialImage,
  };
}

const emptyLocation = (): Location => ({ name: "", address: "", mapUrl: "" });

export class EventScreen extends View {
  private v!: EventView;
  private extras!: ExtrasInput;
  private extrasDirty = false;
  /** Override rows open for editing, with what was typed so far. */
  private drafts = new Map<OverrideField, { text?: string; location?: Location }>();

  async load(): Promise<void> {
    const v = await api.event(this.org, this.params.id!);
    if (v.event.source === "manual") {
      const m = document.createElement("folk-manual") as ManualView;
      m.params = this.params;
      m.me = this.me;
      m.preloaded = v;
      this.replaceWith(m);
      return;
    }
    this.setView(v, true);
    this.render();
    this.bind();
  }

  private setView(v: EventView, resetExtras: boolean): void {
    this.v = v;
    if (resetExtras || !this.extrasDirty) {
      this.extras = extrasOf(v);
      this.extrasDirty = false;
    }
  }

  // ── Rendering ──────────────────────────────────────────────────────────

  private render(): void {
    const v = this.v;
    const fb = v.event.facebook!;
    const binned = !!v.flags.binned;
    const changed = v.flags.overrideChanged ?? [];
    this.set(html`
      <a href="/app/orgs/${this.org}/events" class="muted small">← ${L("Wydarzenia", "Events")}</a>
      <div class="toolbar">
        <div><h1>${t(v.effective.name) || L("(bez nazwy)", "(no name)")}</h1>${flagBadges(v.flags, v.event.binReason)}</div>
        <div class="actions">${sharedActions(this.me, this.org, v)}</div>
      </div>
      ${binnedBanner(this.me, v)}
      ${v.flags.formerPage
        ? html`<div class="banner info" role="status"><p>${L("To wydarzenie pochodzi ze strony na Facebooku, która nie jest już połączona. Synchronizacja go nie aktualizuje; ukryj je, jeśli nie jest potrzebne.", "This event comes from a Facebook page that is no longer connected. Sync does not update it; hide it if it is not wanted.")}</p></div>`
        : ""}
      ${changed.length
        ? html`<div class="banner warn" role="status"><p>${L("Facebook zmienił pola, które nadpisałeś:", "Facebook changed fields you overrode:")} <strong>${changed.map((f) => FIELD_LABEL[f]).join(", ")}</strong>.
            ${L("Zachowaj swoją wartość albo wróć do Facebooka.", "Keep your value or follow Facebook again.")}</p></div>`
        : ""}
      ${permalinks(v)}

      <section class="card" aria-labelledby="dates-title">
        <h2 id="dates-title">${L("Terminy", "Dates")}</h2>
        <p class="muted small">${L("Daty i odwołanie pochodzą z Facebooka i nie można ich nadpisać. Zmień je na Facebooku, a potem zsynchronizuj.", "Dates and cancellation come from Facebook and cannot be overridden. Change them on Facebook, then sync.")}</p>
        ${this.occurrences()}
        <p class="muted small">${fb.online ? html`<span class="badge info">${L("online", "online")}</span> ` : ""}${L("Odczytano z Facebooka", "Read from Facebook")} ${fmtDateTime(fb.syncedAt)}.</p>
      </section>

      <section class="card" aria-labelledby="fb-title">
        <h2 id="fb-title">${L("Z Facebooka", "From Facebook")}</h2>
        <p class="muted small">${L(
          "Strona pokazuje wartości z Facebooka. Nadpisanie zastępuje jedną z nich wartością z CMS; synchronizacja go nie cofa. Gdy Facebook później zmieni to pole, wydarzenie zostanie oznaczone.",
          "The Site shows the values from Facebook. An Override replaces one of them with a CMS value; Sync never undoes it. When Facebook later changes that field, the event is flagged.",
        )}</p>
        ${FIELDS.map((f) => this.overrideRow(f, changed.includes(f), binned))}
      </section>

      ${this.extrasForm()}
    `);
    if (binned) readOnly(this);
  }

  private occurrences(): Raw {
    const occ = this.v.effective.occurrences;
    if (!occ.length) return html`<p class="muted">${L("Brak terminów.", "No dates.")}</p>`;
    const now = Date.now();
    return html`<ol class="occurrences">${occ.map(
      (o) => html`<li class="${o.cancelled ? "cancelled" : ""} ${Date.parse(o.end) < now ? "past" : ""}">
        <span class="when">${fmtDateTime(o.start)}${o.end !== o.start ? html` – ${localDate(o.end) === localDate(o.start) ? fmtTime(o.end) : fmtDateTime(o.end)}` : ""}</span>
        ${o.cancelled ? html`<span class="badge failed">${L("odwołane", "cancelled")}</span>` : ""}
        ${o.facebookId ? html`<a href="${facebookEventUrl(o.facebookId)}" target="_blank" rel="noopener">${L("na Facebooku", "on Facebook")}</a>` : ""}
      </li>`,
    )}</ol>`;
  }

  /** One Facebook-sourced field: its Facebook value read-only beside the Override control. */
  private overrideRow(f: OverrideField, changed: boolean, binned: boolean): Raw {
    const fb = this.v.event.facebook!;
    const ov = this.v.event.overrides?.[f];
    const draft = this.drafts.get(f);
    const editing = !!ov || !!draft;
    const label = { name: L("Nazwa", "Name"), description: L("Opis", "Description"), location: L("Miejsce", "Location"), cover: L("Zdjęcie główne", "Cover image"), ticketUrl: L("Link do biletów", "Ticket link") }[f];
    const status = ov
      ? html`<span class="badge info">${L("nadpisane", "overridden")}</span>${changed ? html` <span class="badge warn">${L("zmienione na Facebooku", "changed on Facebook")}</span>` : ""}`
      : html`<span class="badge">${L("jak na Facebooku", "as on Facebook")}</span>`;

    let facebookSide: Raw;
    let cmsSide: Raw;
    switch (f) {
      case "name":
      case "description":
      case "ticketUrl": {
        const kind = f === "description" ? "long" : f === "ticketUrl" ? "url" : "input";
        const fbValue = (f === "name" ? fb.name : f === "description" ? fb.description : fb.ticketUrl) ?? "";
        const value = draft?.text ?? (ov as { value: string } | undefined)?.value ?? fbValue;
        facebookSide = textField({ id: `fb-${f}`, label: L("Na Facebooku", "On Facebook"), value: fbValue, path: `fb.${f}`, kind, readonly: true });
        cmsSide = editing
          ? textField({
              id: `ov-${f}`,
              label: L("W CMS (pokazywane na stronie)", "In the CMS (shown on the Site)"),
              value,
              path: f,
              kind,
              required: f === "name",
              max: f === "name" ? 200 : f === "description" ? 10000 : undefined,
              hint: changed && ov ? L(`Wcześniej na Facebooku: ${(ov as { against: string }).against || "(puste)"}`, `Before on Facebook: ${(ov as { against: string }).against || "(empty)"}`) : undefined,
            })
          : html`<p class="muted small override-same">${L("Strona pokazuje wartość z Facebooka.", "The Site shows the Facebook value.")}</p>`;
        break;
      }
      case "location": {
        const fl = fb.location ?? emptyLocation();
        const lo = this.v.event.overrides?.location;
        const cur = draft?.location ?? (lo ? (lo.value ?? emptyLocation()) : fl);
        const three = (prefix: string, l: Location, ro: boolean, path: string) => html`
          ${textField({ id: `${prefix}-loc-name`, label: L("Nazwa miejsca", "Place name"), value: l.name, path: `${path}.name`, readonly: ro, max: 200, required: !ro && !!(l.address || l.mapUrl) })}
          ${textField({ id: `${prefix}-loc-address`, label: L("Adres", "Address"), value: l.address, path: `${path}.address`, readonly: ro, max: 500 })}
          ${textField({ id: `${prefix}-loc-map`, label: L("Link do mapy", "Map link"), value: l.mapUrl, path: `${path}.mapUrl`, kind: "url", readonly: ro })}`;
        facebookSide = html`<fieldset><legend>${L("Na Facebooku", "On Facebook")}</legend>${three("fb", fl, true, "fb.location")}</fieldset>`;
        cmsSide = editing
          ? html`<fieldset data-field="location"><legend>${L("W CMS (pokazywane na stronie)", "In the CMS (shown on the Site)")}</legend>${three("ov", cur, false, "location")}</fieldset>`
          : html`<p class="muted small override-same">${L("Strona pokazuje wartość z Facebooka.", "The Site shows the Facebook value.")}</p>`;
        break;
      }
      case "cover": {
        const fc = fb.cover?.variants.at(-1);
        const oc = this.v.event.overrides?.cover?.value;
        facebookSide = html`<div class="field"><p class="label">${L("Na Facebooku", "On Facebook")}</p>${fc ? html`<img class="override-image" src="${publicUrl(this.me, fc.path)}" alt="">` : html`<p class="muted small">${L("Brak zdjęcia.", "No image.")}</p>`}</div>`;
        cmsSide = ov
          ? html`<div class="field"><p class="label">${L("W CMS (pokazywane na stronie)", "In the CMS (shown on the Site)")}</p>${oc?.variants.length ? html`<img class="override-image" src="${publicUrl(this.me, oc.variants.at(-1)!.path)}" alt="">` : html`<p class="muted small">${L("Bez zdjęcia.", "No image.")}</p>`}</div>`
          : html`<p class="muted small override-same">${L("Strona pokazuje zdjęcie z Facebooka.", "The Site shows the Facebook image.")}</p>`;
        break;
      }
    }

    const coverPicker = html`<label class="button" for="ov-cover-file">${ov ? L("Wybierz inne zdjęcie…", "Choose another image…") : L("Nadpisz: wybierz zdjęcie…", "Override: choose an image…")}<input type="file" id="ov-cover-file" accept="image/jpeg,image/png,image/webp" data-override-cover hidden ${binned ? "disabled" : ""}></label>`;
    const actions =
      f === "cover"
        ? html`${coverPicker}${ov ? html`<button type="button" data-follow>${L("Wróć do Facebooka", "Follow Facebook again")}</button>` : ""}${changed && ov ? html`<button type="button" class="secondary" data-keep>${L("Zachowaj moją wartość", "Keep my value")}</button>` : ""}`
        : editing
          ? html`<button type="submit" class="primary">${ov ? L("Zapisz", "Save") : L("Zapisz nadpisanie", "Save override")}</button>
              ${changed && ov && !draft ? html`<button type="button" class="secondary" data-keep>${L("Zachowaj moją wartość", "Keep my value")}</button>` : ""}
              ${ov ? html`<button type="button" data-follow>${L("Wróć do Facebooka", "Follow Facebook again")}</button>` : html`<button type="button" data-cancel>${L("Anuluj", "Cancel")}</button>`}`
          : html`<button type="button" data-start>${L("Nadpisz", "Override")}</button>`;
    return html`<form class="override ${changed ? "changed" : ""}" data-override="${f}" aria-labelledby="ov-${f}-title">
      <div class="override-head"><h3 id="ov-${f}-title">${label}</h3>${status}</div>
      <div class="override-grid"><div>${facebookSide}</div><div>${cmsSide}</div></div>
      <div class="override-problems"></div>
      <div class="actions">${actions}</div>
    </form>`;
  }

  private extrasForm(): Raw {
    const x = this.extras;
    const ev = this.v.event;
    const wa = ev.extras?.writtenAgainst;
    const fb = ev.facebook!;
    return html`<section class="card" aria-labelledby="extras-title"><form id="extras">
      <h2 id="extras-title">${L("Dodane w CMS", "Added in the CMS")}</h2>
      <p class="muted small">${L("Facebook nie ma tych pól. Polski tekst pochodzi z Facebooka; puste pola angielskie strona zastępuje polskimi.", "Facebook does not have these fields. The Polish text comes from Facebook; the Site shows Polish where English is empty.")}</p>
      ${this.v.flags.textOutOfDate
        ? html`<div class="banner warn" role="status"><p><strong>${L("Może być nieaktualne", "May be out of date")}</strong><br>
            ${L("Nazwa lub opis na Facebooku zmieniły się od czasu napisania tekstów angielskich i skrótu.", "The Facebook name or description changed since the English texts and the Summary were written.")}
            ${wa && wa.name !== fb.name ? html`<br>${L("Wcześniejsza nazwa", "Earlier name")}: <em>${wa.name}</em>` : ""}</p>
            <button type="button" id="dismiss-text">${L("Odrzuć oznaczenie", "Dismiss")}</button></div>`
        : ""}
      <div id="extras-problems"></div>
      ${textField({ id: "x-nameEn", label: enLabel(L("Nazwa", "Name")), value: x.nameEn, path: "nameEn", max: 200 })}
      ${textField({ id: "x-descriptionEn", label: enLabel(L("Opis", "Description")), value: x.descriptionEn, path: "descriptionEn", kind: "long", max: 10000 })}
      <div class="pair">
        ${textField({ id: "x-summary-pl", label: html`${L("Skrót", "Summary")} <span class="muted">(PL)</span>`, value: x.summary.pl, path: "summary.pl", kind: "short", max: 500 })}
        ${textField({ id: "x-summary-en", label: enLabel(L("Skrót", "Summary")), value: x.summary.en, path: "summary.en", kind: "short", max: 500 })}
        </div>
        <div class="pair">
        ${textField({ id: "x-price-pl", label: html`${L("Cena / wstęp", "Price / admission")} <span class="muted">(PL)</span>`, value: x.price.pl, path: "price.pl", max: 500 })}
        ${textField({ id: "x-price-en", label: enLabel(L("Cena / wstęp", "Price / admission")), value: x.price.en, path: "price.en", max: 500 })}
      </div>
      ${imageBlock(this.me, "social", x.socialImage, L("Obraz na Instagram (kwadrat lub 4:5, opcjonalny)", "Instagram image (square or 4:5, optional)"), !!this.v.flags.binned)}
      <div class="actions form-actions"><button type="submit" class="primary" id="save-extras" ${this.v.flags.binned ? "disabled" : ""}>${L("Zapisz", "Save")}</button>
        <span class="muted small">${L("Zmiany są na stronie od razu po zapisaniu.", "Changes are on the Site as soon as they are saved.")}</span></div>
    </form></section>`;
  }

  // ── Behaviour ──────────────────────────────────────────────────────────

  /** Reads the CMS fields form into this.extras. */
  private collectExtras(): void {
    const f = this.querySelector<HTMLFormElement>("#extras");
    if (!f) return;
    const val = (n: string) => ((f.elements.namedItem(n) as HTMLInputElement | null)?.value ?? "").trim();
    this.extras = {
      ...this.extras,
      nameEn: val("nameEn"),
      descriptionEn: val("descriptionEn"),
      summary: { pl: val("summary.pl"), en: val("summary.en") },
      price: { pl: val("price.pl"), en: val("price.en") },
    };
  }

  /** Reads an open Override row into this.drafts. */
  private collectDraft(form: HTMLFormElement): void {
    const f = form.dataset.override as OverrideField;
    if (!this.drafts.has(f) && !this.v.event.overrides?.[f]) return;
    const val = (id: string) => (form.querySelector<HTMLInputElement>(`#${id}`)?.value ?? "").trim();
    if (f === "location") this.drafts.set(f, { location: { name: val("ov-loc-name"), address: val("ov-loc-address"), mapUrl: val("ov-loc-map") } });
    else if (f !== "cover") this.drafts.set(f, { text: val(`ov-${f}`) });
  }

  /** Re-renders after an action, keeping unsaved edits elsewhere on the screen. */
  private rerender(v?: EventView): void {
    this.collectExtras();
    if (v) this.setView(v, false);
    const focus = document.activeElement?.id;
    this.render();
    if (focus) document.getElementById(focus)?.focus();
  }

  private async override(form: HTMLFormElement, in_: OverrideInput, btn: HTMLButtonElement | null): Promise<void> {
    await busy(btn, async () => {
      try {
        const v = await api.setOverride(this.org, this.v.event.id, in_);
        this.drafts.delete(in_.field);
        this.rerender(v);
        toast(in_.follow ? L("Strona znów pokazuje wartość z Facebooka", "The Site shows the Facebook value again") : in_.keep ? L("Zachowano Twoją wartość", "Your value is kept") : L("Zapisano nadpisanie", "Override saved"));
      } catch (err) {
        showError(err, form.querySelector<HTMLElement>(".override-problems"), form);
      }
    });
  }

  private async saveExtras(btn: HTMLButtonElement | null): Promise<void> {
    this.collectExtras();
    const form = this.$<HTMLFormElement>("#extras");
    await busy(btn, async () => {
      try {
        const v = await api.setExtras(this.org, this.v.event.id, this.v.etag, this.extras);
        this.extrasDirty = false;
        this.setView(v, true);
        this.rerender();
        toast(L("Zapisano – strona zaktualizuje się w ciągu minuty", "Saved – the Site updates within a minute"));
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          toast(L("Ktoś inny zmienił to wydarzenie. Odśwież stronę, aby zobaczyć zmiany (Twoje zmiany nie zostały zapisane).", "Someone else changed this event. Refresh the page to see the changes (your changes were not saved)."), "error");
          return;
        }
        showError(err, this.querySelector<HTMLElement>("#extras-problems"), form);
      }
    });
  }

  private async action(name: "hide" | "show" | "dismiss-text" | "restore", control: HTMLInputElement | HTMLButtonElement): Promise<void> {
    control.disabled = true;
    try {
      const v = await api.eventAction(this.org, this.v.event.id, name);
      this.rerender(v);
      toast(
        {
          hide: L("Ukryto – strona zaktualizuje się w ciągu minuty", "Hidden – the Site updates within a minute"),
          show: L("Pokazano na stronie", "Shown on the Site"),
          "dismiss-text": L("Odrzucono oznaczenie", "Flag dismissed"),
          restore: L("Przywrócono z kosza", "Restored from the Bin"),
        }[name],
      );
    } catch (err) {
      if (control instanceof HTMLInputElement) control.checked = !control.checked;
      control.disabled = false;
      showError(err);
    }
  }

  private bind(): void {
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      const form = e.target as HTMLFormElement;
      const submit = form.querySelector<HTMLButtonElement>("button[type=submit]");
      if (form.id === "extras") {
        void this.saveExtras(submit);
        return;
      }
      const f = form.dataset.override as OverrideField | undefined;
      if (!f) return;
      this.collectDraft(form);
      const d = this.drafts.get(f) ?? {};
      void this.override(form, f === "location" ? { field: f, location: d.location } : { field: f, text: d.text ?? "" }, submit);
    });
    this.on("[data-start]", "click", (_e, el) => {
      const form = el.closest<HTMLFormElement>("form")!;
      const f = form.dataset.override as OverrideField;
      const fb = this.v.event.facebook!;
      this.drafts.set(f, f === "location" ? { location: { ...emptyLocation(), ...fb.location } } : { text: (f === "name" ? fb.name : f === "description" ? fb.description : fb.ticketUrl) ?? "" });
      this.rerender();
      this.querySelector<HTMLElement>(`form[data-override="${f}"] :is(input, textarea):not([readonly])`)?.focus();
    });
    this.on("[data-cancel]", "click", (_e, el) => {
      this.drafts.delete(el.closest<HTMLFormElement>("form")!.dataset.override as OverrideField);
      this.rerender();
    });
    this.on("[data-follow]", "click", (_e, el) => {
      const form = el.closest<HTMLFormElement>("form")!;
      if (!confirm(L("Usunąć Twoją wartość i pokazywać wartość z Facebooka?", "Drop your value and show the Facebook value?"))) return;
      void this.override(form, { field: form.dataset.override as OverrideField, follow: true }, el as HTMLButtonElement);
    });
    this.on("[data-keep]", "click", (_e, el) => {
      const form = el.closest<HTMLFormElement>("form")!;
      void this.override(form, { field: form.dataset.override as OverrideField, keep: true }, el as HTMLButtonElement);
    });
    this.on("[data-override-cover]", "change", (_e, el) => {
      const form = el.closest<HTMLFormElement>("form")!;
      void upload(this.org, "cover", el as HTMLInputElement).then((img) => {
        if (img) void this.override(form, { field: "cover", cover: img }, null);
      });
    });
    this.on("[data-upload=social]", "change", (_e, el) => {
      void upload(this.org, "social", el as HTMLInputElement).then((img) => {
        if (!img) return;
        this.collectExtras();
        this.extras.socialImage = img;
        this.extrasDirty = true;
        this.rerender();
        toast(L("Wgrano obraz – zapisz zmiany", "Image uploaded – save the changes"));
      });
    });
    this.on("[data-remove-image=social]", "click", () => {
      this.collectExtras();
      this.extras.socialImage = undefined;
      this.extrasDirty = true;
      this.rerender();
    });
    this.on("#hidden", "change", (_e, el) => void this.action((el as HTMLInputElement).checked ? "hide" : "show", el as HTMLInputElement));
    this.on("#dismiss-text", "click", (_e, el) => void this.action("dismiss-text", el as HTMLButtonElement));
    this.on("[data-action=restore]", "click", (_e, el) => void this.action("restore", el as HTMLButtonElement));
    this.addEventListener("input", (e) => {
      const target = e.target as HTMLElement;
      if (target.closest("#extras") && (target as HTMLInputElement).type !== "file") this.extrasDirty = true;
      const form = target.closest<HTMLFormElement>("form[data-override]");
      if (form) this.collectDraft(form);
    });
    addEventListener("beforeunload", this.guard);
  }

  private guard = (e: BeforeUnloadEvent) => {
    if ((this.extrasDirty || this.drafts.size) && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
  }
}

customElements.define("folk-event", EventScreen);
