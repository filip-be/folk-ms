import {
  api,
  ApiError,
  type Content,
  type EventView,
  type Exception,
  type Image,
  type Kind,
  type Occurrence,
  type Text,
  type Venue,
} from "../api.ts";
import { badge, busy, fmtDateTime, html, markProblems, navigate, setFieldError, showError, t, toast, WEEKDAYS } from "../ui.ts";
import { View } from "../view.ts";
import { inLang, L, lang } from "../i18n.ts";

const NTH = [
  [1, L("pierwszy", "first")],
  [2, L("drugi", "second")],
  [3, L("trzeci", "third")],
  [4, L("czwarty", "fourth")],
  [-1, L("ostatni", "last")],
] as const;

function emptyContent(): Content {
  return {
    slug: "",
    title: { pl: "" },
    schedule: { kind: "single", startDate: "", startTime: "19:00", endTime: "23:00" },
  };
}

class EditorView extends View {
  private view?: EventView;
  private content: Content = emptyContent();
  private venues: Venue[] = [];
  private exceptions = new Map<string, Exception>();
  private occurrences: Occurrence[] = [];
  private dirty = false;
  private previewTimer = 0;
  private editLang: "pl" | "en" = "pl";

  private get eventId(): string | undefined {
    return this.params.id;
  }

  async load(): Promise<void> {
    const [venues, view] = await Promise.all([api.venues(this.org), this.eventId ? api.event(this.org, this.eventId) : Promise.resolve(undefined)]);
    this.venues = venues;
    this.setView(view);
    this.render();
    this.bind();
    if (!view) this.schedulePreview();
  }

  private setView(view?: EventView): void {
    this.view = view;
    this.content = view ? structuredClone(view.event.draft) : emptyContent();
    this.exceptions = new Map((this.content.schedule.exceptions ?? []).map((e) => [e.date, { ...e }]));
    this.occurrences = view?.occurrences ?? [];
    this.dirty = false;
  }

  private publicUrl(path?: string): string {
    return path ? `${this.me.publicUrl.replace(/\/$/, "")}/${path}` : "";
  }

  /** A Polish and an English field; `path` is the field's path in validation problems. */
  private textFields(name: string, label: string, value: Text | undefined, kind: "input" | "short" | "long", max: number, path = name): ReturnType<typeof html> {
    return html`${(["pl", "en"] as const).map((l) => {
      const id = `f-${name}-${l}`;
      const v = value?.[l] ?? "";
      // The Polish title is required to publish; novalidate lets a draft be saved without it.
      const required = l === "pl" && name === "title";
      const field =
        kind === "input"
          ? html`<input id="${id}" name="${name}.${l}" data-field="${path}.${l}" value="${v}" maxlength="${max}" ${required ? "required" : ""}>`
          : html`<textarea id="${id}" name="${name}.${l}" data-field="${path}.${l}" rows="${kind === "short" ? 2 : 8}" maxlength="${max}">${v}</textarea>`;
      return html`<div class="field lang-${l}"><label for="${id}">${label}${l === "en" ? html` <span class="muted">${L("(EN, opcjonalnie)", "(EN, optional)")}</span>` : ""}</label>${field}</div>`;
    })}`;
  }

  private imageBlock(kind: "cover" | "social", img: Image | undefined): ReturnType<typeof html> {
    const label = kind === "cover" ? L("Zdjęcie główne (poziome)", "Cover image (landscape)") : L("Obraz na Instagram (kwadrat lub 4:5, opcjonalny)", "Instagram image (square or 4:5, optional)");
    const preview = img?.variants[0];
    return html`<div class="image-block" data-kind="${kind}">
      <h3>${label}</h3>
      ${img
        ? html`<div class="previews">
            ${preview ? html`<figure><img src="${this.publicUrl(preview.path)}" alt="" width="${preview.width}" height="${preview.height}"><figcaption>${L("Strona", "Site")}</figcaption></figure>` : ""}
            ${img.instagram ? html`<figure><img class="ig" src="${this.publicUrl(img.instagram.path)}" alt=""><figcaption>${L("Kadr Instagram", "Instagram crop")} ${img.instagram.width}×${img.instagram.height}</figcaption></figure>` : ""}
          </div>
          ${kind === "cover" ? this.textFields(`alt-${kind}`, L("Tekst alternatywny", "Alternative text"), img.alt, "input", 200) : ""}
          <button type="button" class="link danger" data-remove-image="${kind}">${L("Usuń obraz", "Remove image")}</button>`
        : html`<p class="muted small">${L("Brak obrazu.", "No image.")}</p>`}
      <label class="button">${L("Wybierz plik…", "Choose file…")}<input type="file" accept="image/jpeg,image/png,image/webp" data-upload="${kind}" hidden></label>
      <p class="muted small">${L("JPEG, PNG lub WebP, do 15 MB. HEIC (iPhone) nie jest obsługiwany – zapisz zdjęcie jako JPEG.", "JPEG, PNG or WebP, up to 15 MB. HEIC (iPhone) is not supported – save the photo as JPEG.")}</p>
    </div>`;
  }

  private render(): void {
    const c = this.content;
    const s = c.schedule;
    const r = s.recurrence ?? { freq: "weekly", weekday: 4, endDate: "" };
    const v = this.view;
    const status = v?.status;
    const binned = status === "binned";
    const activeVenues = this.venues.filter((x) => !x.archived || x.id === c.venueId);
    this.set(html`
      <form id="editor" class="editor" data-kind="${s.kind}" data-freq="${r.freq}" data-lang="${this.editLang}" novalidate>
        <div class="toolbar sticky">
          <div>
            <a href="/app/orgs/${this.org}/events" class="muted small">← ${L("Wydarzenia", "Events")}</a>
            <h1>${t(c.title) || (v ? L("(bez tytułu)", "(untitled)") : L("Nowe wydarzenie", "New event"))}</h1>
            ${v ? html`<div>${badge(v.status, v.event.published && v.event.draftRevision !== v.event.publishedRevision ? L("niepublikowane zmiany", "unpublished changes") : "")}
              ${v.event.publishedAt ? html`<span class="muted small">${L("opublikowano", "published")} ${fmtDateTime(v.event.publishedAt)}</span>` : ""}</div>` : ""}
          </div>
          <div class="actions">
            ${binned
              ? this.me.superAdmin ? html`<button type="button" class="primary" data-action="restore">${L("Przywróć z kosza", "Restore from the bin")}</button>` : ""
              : html`
                <button type="submit" class="${v ? "secondary" : "primary"}" id="save">${L("Zapisz szkic", "Save draft")}</button>
                ${v ? html`<button type="button" class="primary publish" data-action="publish">${status !== "published" ? L("Opublikuj", "Publish") : v!.event.draftRevision !== v!.event.publishedRevision ? L("Opublikuj zmiany", "Publish changes") : L("Opublikuj ponownie", "Publish again")}</button>` : ""}
                ${status === "published" ? html`<a class="button" href="/app/orgs/${this.org}/events/${v!.event.id}/post">${L("Ogłoś w mediach", "Announce on social media")}</a>
                  <a class="button" href="/app/orgs/${this.org}/events/${v!.event.id}/facebook">${L("Wydarzenie na Facebooku", "Facebook event")}</a>` : ""}
                ${v ? html`<details class="more"><summary class="button">${L("Więcej", "More")}</summary><div class="menu">
                  ${status === "published" ? html`<button type="button" data-action="unpublish">${L("Wycofaj ze strony", "Unpublish")}</button>` : ""}
                  ${v ? html`<button type="button" data-action="duplicate">${L("Duplikuj", "Duplicate")}</button>
                    <button type="button" class="danger" data-action="delete">${L("Przenieś do kosza", "Move to the bin")}</button>` : ""}
                </div></details>` : ""}`}
          </div>
        </div>
        ${binned ? html`<div class="banner warn">${L("Wydarzenie jest w koszu i można je tylko przeglądać.", "The event is in the bin and can only be viewed.")}${this.me.superAdmin ? L(" Przywróć je, aby edytować.", " Restore it to edit it.") : ""}</div>` : ""}
        <div id="problems"></div>
        ${v ? html`<p class="permalink muted small">${L("Adres na stronie", "Address on the site")}: <a href="${v.permalink[lang]}" target="_blank" rel="noopener">${v.permalink[lang]}</a></p>` : ""}

        <section class="card">
          <div class="section-head"><h2>${L("Treść", "Content")}</h2>
            <div class="lang-tabs" role="tablist">
              <button type="button" role="tab" data-lang="pl" aria-selected="${this.editLang === "pl"}">Polski</button>
              <button type="button" role="tab" data-lang="en" aria-selected="${this.editLang === "en"}">English</button>
            </div>
          </div>
          <p class="required-note">${L("wymagane do publikacji", "required to publish")}</p>
          <p class="muted small lang-en">${L("Puste pola angielskie są na stronie zastępowane polskimi.", "Empty English fields are replaced with Polish ones on the site.")}</p>
          ${this.textFields("title", L("Tytuł", "Title"), c.title, "input", 200)}
          ${this.textFields("summary", L("Krótki opis", "Summary"), c.summary, "short", 500)}
          ${this.textFields("description", L("Opis", "Description"), c.description, "long", 10000)}
          <div class="field"><label for="f-slug">${L("Końcówka adresu", "Address ending")}</label>
            <input id="f-slug" name="slug" data-field="slug" value="${c.slug}" pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="80" placeholder="${L("tworzona z tytułu", "made from the title")}">
            <p class="muted small">${L("Można zmieniać – stare linki nadal działają dzięki stałemu identyfikatorowi", "It can change – old links keep working thanks to the fixed identifier")} ${v ? html`<code>${v.event.shortId}</code>` : ""}.</p></div>
        </section>

        <section class="card schedule">
          <h2>${L(L("Termin", "Date"), "Date")}</h2>
          <fieldset class="kinds" data-field="schedule.kind"><legend class="sr-only">${L("Rodzaj", "Type")}</legend>
            ${(
              [
                ["single", L("Jednorazowe", "Single")],
                ["multiDay", L("Wielodniowe", "Multi-day")],
                ["recurring", L("Cykliczne", "Recurring")],
              ] as [Kind, string][]
            ).map(([k, label]) => html`<label class="chip"><input type="radio" name="kind" value="${k}" ${s.kind === k ? "checked" : ""}> ${label}</label>`)}
          </fieldset>
          <div class="grid">
            <div class="field"><label for="f-startDate">${L("Data rozpoczęcia", "Start date")}</label><input type="date" id="f-startDate" name="startDate" data-field="schedule.startDate" value="${s.startDate}" required></div>
            <div class="field"><label for="f-startTime">${L("Początek", "Start")}</label><input type="time" id="f-startTime" name="startTime" data-field="schedule.startTime" value="${s.startTime}" required></div>
            <div class="field k-multiDay"><label for="f-endDate">${L("Data końca", "End date")}</label><input type="date" id="f-endDate" name="endDate" data-field="schedule.endDate" value="${s.endDate ?? ""}" required></div>
            <div class="field"><label for="f-endTime">${L("Koniec", "End")}</label><input type="time" id="f-endTime" name="endTime" data-field="schedule.endTime" value="${s.endTime}" required></div>
          </div>
          <p class="muted small k-single">${L("Godzina końca wcześniejsza niż początek oznacza zakończenie następnego dnia.", "An end time earlier than the start time means it ends the next day.")}</p>
          <div class="grid k-recurring">
            <div class="field"><label for="f-freq">${L("Powtarzanie", "Repeats")}</label><select id="f-freq" name="freq" data-field="schedule.recurrence.freq schedule.recurrence" required>
              <option value="weekly" ${r.freq === "weekly" ? "selected" : ""}>${L("co tydzień", "weekly")}</option>
              <option value="biweekly" ${r.freq === "biweekly" ? "selected" : ""}>${L("co dwa tygodnie", "every two weeks")}</option>
              <option value="monthly" ${r.freq === "monthly" ? "selected" : ""}>${L("co miesiąc", "monthly")}</option></select></div>
            <div class="field nth"><label for="f-nth">${L("Który", "Which")}</label><select id="f-nth" name="nth" data-field="schedule.recurrence.nth" required>
              ${NTH.map(([n, label]) => html`<option value="${n}" ${r.nth === n ? "selected" : ""}>${label}</option>`)}</select></div>
            <div class="field"><label for="f-weekday">${L("Dzień tygodnia", "Weekday")}</label><select id="f-weekday" name="weekday" data-field="schedule.recurrence.weekday" required>
              ${WEEKDAYS.map((d, i) => html`<option value="${i}" ${r.weekday === i ? "selected" : ""}>${d}</option>`)}</select></div>
            <div class="field"><label for="f-until">${L("Do dnia (najwyżej 24 miesiące)", "Until (at most 24 months)")}</label><input type="date" id="f-until" name="until" data-field="schedule.recurrence.endDate" value="${r.endDate}" required></div>
          </div>
          <div id="occurrences">${this.occurrencesHtml()}</div>
        </section>

        <section class="card">
          <h2>${L("Miejsce i cena", "Venue and price")}</h2>
          <div class="field"><label for="f-venue">${L("Miejsce", "Venue")}</label>
            <select id="f-venue" name="venueId" data-field="venueId" required><option value="">${L("— wybierz —", "— choose —")}</option>
              ${activeVenues.map((x) => html`<option value="${x.id}" ${c.venueId === x.id ? "selected" : ""}>${x.name} – ${x.address}${x.archived ? L(" (zarchiwizowane)", " (archived)") : ""}</option>`)}
            </select>
            <p class="muted small"><button type="button" class="link" data-new-venue>${L("+ Nowe miejsce", "+ New venue")}</button>
              · <a href="/app/orgs/${this.org}/venues">${L("Zarządzaj miejscami", "Manage venues")}</a></p></div>
          <div class="new-venue" hidden>
            <h3>${L("Nowe miejsce", "New venue")}</h3>
            <div class="grid">
              <div class="field"><label for="nv-name">${L("Nazwa", "Name")}</label><input id="nv-name" data-nv="name" maxlength="200" required></div>
              <div class="field"><label for="nv-address">${L("Adres", "Address")}</label><input id="nv-address" data-nv="address" maxlength="500" required></div>
              <div class="field"><label for="nv-map">${L("Link do mapy (opcjonalnie)", "Map link (optional)")}</label><input id="nv-map" data-nv="mapUrl" type="url" placeholder="https://"></div>
            </div>
            <div class="actions"><button type="button" class="primary" data-nv-add>${L("Dodaj i wybierz", "Add and select")}</button>
            <button type="button" data-nv-cancel>${L("Anuluj", "Cancel")}</button></div>
          </div>
          ${this.textFields("price", L("Cena / wstęp", "Price / admission"), c.price?.text, "input", 500, "price.text")}
          <div class="field"><label for="f-priceUrl">${L("Link do biletów lub zapisów", "Tickets or registration link")}</label><input type="url" id="f-priceUrl" name="priceUrl" data-field="price.url" value="${c.price?.url ?? ""}" placeholder="https://"></div>
        </section>

        <section class="card images">
          <h2>${L("Obrazy", "Images")}</h2>
          ${this.imageBlock("cover", c.cover)}
          ${this.imageBlock("social", c.socialImage)}
        </section>
      </form>`);
    // What blocks publishing; an event in the bin is restored before it can be published.
    if (v && !binned) markProblems(this.$("#editor"), v.problems, this.$("#problems"));
    if (binned) this.readOnly();
  }

  /** An event in the bin is shown read-only: text can be selected and copied,
   *  other controls are disabled. The language tabs keep working. */
  private readOnly(): void {
    this.$("#editor")
      .querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | HTMLButtonElement>("section :is(input, select, textarea, button)")
      .forEach((el) => {
        if (el.getAttribute("role") === "tab") return;
        if ((el instanceof HTMLInputElement && !["checkbox", "radio", "file", "date", "time"].includes(el.type)) || el instanceof HTMLTextAreaElement) el.readOnly = true;
        else el.disabled = true;
      });
  }

  private occurrencesHtml(): ReturnType<typeof html> {
    const occ = this.occurrences;
    if (!occ.length) return html`<p class="muted small">${L("Uzupełnij termin, aby zobaczyć daty.", "Fill in the date to see the dates.")}</p>`;
    const recurring = this.content.schedule.kind === "recurring";
    const now = Date.now();
    return html`<h3>${recurring ? L(`Terminy (${occ.length})`, `Dates (${occ.length})`) : L("Termin", "Date")}</h3>
      <ol class="occurrences">${occ.map((o) => {
        const ex = this.exceptions.get(o.key) ?? { date: o.key };
        const past = Date.parse(o.end) < now;
        return html`<li class="${o.cancelled ? "cancelled" : ""} ${past ? "past" : ""}" data-key="${o.key}">
          <span class="when">${fmtDateTime(o.start)} – ${fmtDateTime(o.end).split(", ").pop()}</span>
          ${recurring
            ? html`<label class="inline"><input type="checkbox" data-ex="cancelled" ${ex.cancelled ? "checked" : ""}> ${L("odwołane", "cancelled")}</label>
              <details ${ex.startTime || ex.endTime || ex.note?.pl ? "open" : ""}><summary>${L("zmień", "change")}</summary>
                <label class="inline">${L("od", "from")} <input type="time" data-ex="startTime" value="${ex.startTime ?? ""}"></label>
                <label class="inline">${L("do", "to")} <input type="time" data-ex="endTime" value="${ex.endTime ?? ""}"></label>
                <label class="inline">${L("uwaga", "note")} <input data-ex="note" value="${ex.note?.pl ?? ""}" maxlength="200" placeholder="${L("np. zmiana sali", "e.g. different room")}"></label>
              </details>`
            : ""}
        </li>`;
      })}</ol>`;
  }

  /** Reads the form into this.content. */
  private collect(): Content {
    const f = this.$<HTMLFormElement>("#editor");
    const val = (n: string) => ((f.elements.namedItem(n) as HTMLInputElement | null)?.value ?? "").trim();
    const text = (n: string): Text => {
      const t: Text = { pl: val(`${n}.pl`) };
      const en = val(`${n}.en`);
      if (en) t.en = en;
      return t;
    };
    const kind = (f.querySelector<HTMLInputElement>('input[name="kind"]:checked')?.value ?? "single") as Kind;
    const c = this.content;
    c.slug = val("slug");
    c.title = text("title");
    c.summary = text("summary");
    c.description = text("description");
    c.venueId = val("venueId") || undefined;
    c.price = { text: text("price"), url: val("priceUrl") || undefined };
    if (c.cover && f.elements.namedItem("alt-cover.pl")) c.cover.alt = text("alt-cover");
    const s = (c.schedule = { kind, startDate: val("startDate"), startTime: val("startTime"), endTime: val("endTime") } as Content["schedule"]);
    if (kind === "multiDay") s.endDate = val("endDate");
    if (kind === "recurring") {
      const freq = val("freq") as "weekly" | "biweekly" | "monthly";
      s.recurrence = { freq, weekday: Number(val("weekday")), endDate: val("until") };
      if (freq === "monthly") s.recurrence.nth = Number(val("nth"));
      const keys = new Set(this.occurrences.map((o) => o.key));
      s.exceptions = [...this.exceptions.values()].filter(
        (e) => keys.has(e.date) && (e.cancelled || e.startTime || e.endTime || e.note?.pl || e.venueId),
      );
    }
    return c;
  }

  private schedulePreview(): void {
    clearTimeout(this.previewTimer);
    this.previewTimer = setTimeout(async () => {
      try {
        const res = await api.preview(this.org, this.collect());
        this.occurrences = res.occurrences;
        const box = this.querySelector("#occurrences");
        if (box) {
          box.innerHTML = this.occurrencesHtml().value;
          if (res.problems.length && this.content.schedule.startDate) {
            box.insertAdjacentHTML("afterbegin", html`<ul class="hint">${res.problems.map((p) => html`<li>${inLang(p.message)}</li>`)}</ul>`.value);
          }
        }
      } catch (err) {
        showError(err);
      }
    }, 350) as unknown as number;
  }

  private async save(): Promise<EventView | undefined> {
    const c = this.collect();
    const problems = this.querySelector("#problems");
    try {
      const v = this.view ? await api.updateEvent(this.org, this.view.event.id, this.view.etag, c) : await api.createEvent(this.org, c);
      this.dirty = false;
      if (!this.view) {
        toast(L("Utworzono szkic", "Draft created"));
        navigate(`/app/orgs/${this.org}/events/${v.event.id}`);
        return undefined;
      }
      this.setView(v);
      this.render();
      toast(L("Zapisano szkic", "Draft saved"));
      return v;
    } catch (err) {
      if (err instanceof ApiError && err.code === "conflict") {
        toast(L("Ktoś inny zmienił to wydarzenie. Odśwież stronę, aby zobaczyć zmiany (Twoje zmiany nie zostały zapisane).", "Someone else changed this event. Refresh the page to see the changes (your changes were not saved)."), "error");
        return undefined;
      }
      showError(err, problems as HTMLElement);
      return undefined;
    }
  }

  private async action(name: "publish" | "unpublish" | "delete" | "restore" | "duplicate", btn: HTMLButtonElement): Promise<void> {
    const confirmText: Record<string, string> = {
      unpublish: L("Wycofać wydarzenie ze strony? Pozostanie w CMS jako szkic.", "Unpublish the event from the site? It stays in the CMS as a draft."),
      delete: L("Przenieść wydarzenie do kosza? Opublikowane zostanie wycofane ze strony. Posty na Facebooku i Instagramie pozostaną.", "Move the event to the bin? If published, it is removed from the site. Facebook and Instagram posts stay."),
    };
    if (confirmText[name] && !confirm(confirmText[name])) return;
    await busy(btn, async () => {
      if (name === "publish" && (this.dirty || !this.view)) {
        if (!(await this.save())) return;
      }
      try {
        const v = await api.eventAction(this.org, this.view!.event.id, name);
        if (name === "duplicate") {
          toast(L("Utworzono kopię – uzupełnij daty", "Copy created – fill in the dates"));
          navigate(`/app/orgs/${this.org}/events/${v.event.id}`);
          return;
        }
        this.setView(v);
        this.render();
        toast(
          { publish: L("Opublikowano – strona zaktualizuje się w ciągu minuty", "Published – the site updates within a minute"), unpublish: L("Wycofano ze strony", "Unpublished from the site"), delete: L("Przeniesiono do kosza", "Moved to the bin"), restore: L("Przywrócono jako szkic", "Restored as a draft") }[name] ??
            L("Gotowe", "Done"),
        );
      } catch (err) {
        showError(err, this.querySelector("#problems") as HTMLElement);
      }
    });
  }

  /** Shows or hides the inline New venue panel; it is outside the event's content. */
  private toggleNewVenue(open: boolean): void {
    const panel = this.$<HTMLElement>(".new-venue");
    panel.hidden = !open;
    panel.querySelectorAll<HTMLInputElement>("[data-nv]").forEach((i) => (i.value = ""));
    if (open) panel.querySelector<HTMLInputElement>("[data-nv=name]")!.focus();
  }

  /** Creates a Venue from the inline panel and selects it for this event. */
  private async addVenue(btn: HTMLButtonElement): Promise<void> {
    const panel = this.$<HTMLElement>(".new-venue");
    const inputs = [...panel.querySelectorAll<HTMLInputElement>("[data-nv]")];
    const data = Object.fromEntries(inputs.map((i) => [i.dataset.nv!, i.value.trim()]));
    const missing = inputs.filter((i) => i.required && !i.value.trim());
    missing.forEach((i) => setFieldError(i, L("to pole jest wymagane", "this field is required")));
    if (missing.length) {
      missing[0]!.focus();
      return;
    }
    await busy(btn, async () => {
      try {
        const v = await api.createVenue(this.org, data);
        this.collect();
        this.venues = [...this.venues, v];
        this.content.venueId = v.id;
        this.dirty = true;
        this.render();
        this.schedulePreview();
        this.$<HTMLSelectElement>("#f-venue").focus();
        toast(L("Dodano miejsce i wybrano je dla wydarzenia", "Venue added and selected for the event"));
      } catch (err) {
        showError(err, null, panel);
      }
    });
  }

  private async upload(kind: "cover" | "social", input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    if (!file) return;
    if (/\.(heic|heif)$/i.test(file.name) || /hei[cf]/i.test(file.type)) {
      toast(L("Format HEIC nie jest obsługiwany – zapisz zdjęcie jako JPEG.", "HEIC is not supported – save the photo as JPEG."), "error");
      return;
    }
    this.collect();
    const block = input.closest(".image-block");
    block?.classList.add("uploading");
    try {
      const img = await api.uploadImage(this.org, kind, file);
      if (kind === "cover") this.content.cover = { ...img, alt: this.content.cover?.alt };
      else this.content.socialImage = img;
      this.dirty = true;
      this.render();
      toast(L("Wgrano obraz – zapisz szkic", "Image uploaded – save the draft"));
    } catch (err) {
      showError(err);
    } finally {
      block?.classList.remove("uploading");
    }
  }

  private bind(): void {
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void busy(this.querySelector<HTMLButtonElement>("#save"), () => this.save());
    });
    this.on("[data-action]", "click", (_e, el) => {
      (el.closest("details") as HTMLDetailsElement | null)?.removeAttribute("open");
      void this.action(el.dataset.action as "publish", el as HTMLButtonElement);
    });
    this.on("[data-lang]", "click", (e, el) => {
      if (el.tagName !== "BUTTON") return;
      e.preventDefault();
      this.setLang(el.dataset.lang as "pl" | "en");
    });
    // A link in the problem summary to a field of the other language switches to it.
    this.addEventListener("folk:reveal", (e) => {
      const l = (e.target as HTMLElement).closest(".lang-pl, .lang-en");
      if (l) this.setLang(l.classList.contains("lang-en") ? "en" : "pl");
      if ((e.target as HTMLElement).closest(".new-venue")) this.$<HTMLElement>(".new-venue").hidden = false;
    });
    this.on("[data-new-venue]", "click", () => this.toggleNewVenue(true));
    this.on("[data-nv-cancel]", "click", () => this.toggleNewVenue(false));
    this.on("[data-nv-add]", "click", (_e, el) => void this.addVenue(el as HTMLButtonElement));
    this.on("[data-nv]", "keydown", (e) => {
      const k = e as KeyboardEvent;
      if (k.key === "Enter") {
        // Enter would submit the editor (Save draft); add the venue instead.
        e.preventDefault();
        void this.addVenue(this.$<HTMLButtonElement>("[data-nv-add]"));
      } else if (k.key === "Escape") this.toggleNewVenue(false);
    });
    this.on("[data-upload]", "change", (_e, el) => void this.upload(el.dataset.upload as "cover", el as HTMLInputElement));
    this.on("[data-remove-image]", "click", (_e, el) => {
      this.collect();
      if (el.dataset.removeImage === "cover") delete this.content.cover;
      else delete this.content.socialImage;
      this.dirty = true;
      this.render();
    });
    this.on("[data-ex]", "change", (_e, el) => {
      const key = el.closest<HTMLElement>("li[data-key]")!.dataset.key!;
      const ex = this.exceptions.get(key) ?? { date: key };
      const input = el as HTMLInputElement;
      switch (input.dataset.ex) {
        case "cancelled":
          ex.cancelled = input.checked || undefined;
          break;
        case "startTime":
          ex.startTime = input.value || undefined;
          break;
        case "endTime":
          ex.endTime = input.value || undefined;
          break;
        case "note":
          ex.note = input.value.trim() ? { pl: input.value.trim() } : undefined;
          break;
      }
      this.exceptions.set(key, ex);
      this.dirty = true;
      this.schedulePreview();
    });
    this.addEventListener("input", (e) => {
      const t = e.target as HTMLInputElement;
      if (t.dataset.ex || t.dataset.nv || t.type === "file") return;
      this.dirty = true;
      if (t.closest(".schedule") || t.name === "venueId") this.schedulePreview();
    });
    this.addEventListener("change", (e) => {
      const t = e.target as HTMLInputElement;
      if (t.name === "kind") {
        this.$("#editor").dataset.kind = t.value;
        this.content.schedule.kind = t.value as Kind;
      }
      if (t.name === "freq") this.$("#editor").dataset.freq = t.value;
    });
    addEventListener("beforeunload", this.guard);
  }

  private setLang(l: "pl" | "en"): void {
    this.editLang = l;
    this.$("#editor").dataset.lang = l;
    this.querySelectorAll("[role=tab]").forEach((t) => t.setAttribute("aria-selected", String((t as HTMLElement).dataset.lang === l)));
  }

  private guard = (e: BeforeUnloadEvent) => {
    if (this.dirty && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
  }
}

// Listeners are delegated on the element itself, so re-renders keep them.
customElements.define("folk-editor", EditorView);
