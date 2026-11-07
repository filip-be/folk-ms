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
import { badge, busy, fmtDateTime, html, navigate, problemsHtml, showError, toast, WEEKDAYS } from "../ui.ts";
import { View } from "../view.ts";

const NTH = [
  [1, "pierwszy"],
  [2, "drugi"],
  [3, "trzeci"],
  [4, "czwarty"],
  [-1, "ostatni"],
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

  private textFields(name: string, label: string, value: Text | undefined, kind: "input" | "short" | "long", max: number): ReturnType<typeof html> {
    return html`${(["pl", "en"] as const).map((l) => {
      const id = `f-${name}-${l}`;
      const v = value?.[l] ?? "";
      const field =
        kind === "input"
          ? html`<input id="${id}" name="${name}.${l}" value="${v}" maxlength="${max}" ${l === "pl" && name === "title" ? "required" : ""}>`
          : html`<textarea id="${id}" name="${name}.${l}" rows="${kind === "short" ? 2 : 8}" maxlength="${max}">${v}</textarea>`;
      return html`<div class="field lang-${l}"><label for="${id}">${label} ${l === "en" ? html`<span class="muted">(EN, opcjonalnie)</span>` : ""}</label>${field}</div>`;
    })}`;
  }

  private imageBlock(kind: "cover" | "social", img: Image | undefined): ReturnType<typeof html> {
    const label = kind === "cover" ? "Zdjęcie główne (poziome)" : "Obraz na Instagram (kwadrat lub 4:5, opcjonalny)";
    const preview = img?.variants[0];
    return html`<div class="image-block" data-kind="${kind}">
      <h3>${label}</h3>
      ${img
        ? html`<div class="previews">
            ${preview ? html`<figure><img src="${this.publicUrl(preview.path)}" alt="" width="${preview.width}" height="${preview.height}"><figcaption>Strona</figcaption></figure>` : ""}
            ${img.instagram ? html`<figure><img class="ig" src="${this.publicUrl(img.instagram.path)}" alt=""><figcaption>Kadr Instagram ${img.instagram.width}×${img.instagram.height}</figcaption></figure>` : ""}
          </div>
          ${kind === "cover" ? this.textFields(`alt-${kind}`, "Tekst alternatywny", img.alt, "input", 200) : ""}
          <button type="button" class="link danger" data-remove-image="${kind}">Usuń obraz</button>`
        : html`<p class="muted small">Brak obrazu.</p>`}
      <label class="button">Wybierz plik…<input type="file" accept="image/jpeg,image/png,image/webp" data-upload="${kind}" hidden></label>
      <p class="muted small">JPEG, PNG lub WebP, do 15 MB. HEIC (iPhone) nie jest obsługiwany – zapisz zdjęcie jako JPEG.</p>
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
            <a href="/app/orgs/${this.org}/events" class="muted small">← Wydarzenia</a>
            <h1>${c.title.pl || (v ? "(bez tytułu)" : "Nowe wydarzenie")}</h1>
            ${v ? html`<div>${badge(v.status, v.event.published && v.event.draftRevision !== v.event.publishedRevision ? "niepublikowane zmiany" : "")}
              ${v.event.publishedAt ? html`<span class="muted small">opublikowano ${fmtDateTime(v.event.publishedAt)}</span>` : ""}</div>` : ""}
          </div>
          <div class="actions">
            ${binned
              ? this.me.superAdmin ? html`<button type="button" data-action="restore">Przywróć z kosza</button>` : ""
              : html`
                <button type="submit" class="primary" id="save">Zapisz szkic</button>
                ${v ? html`<button type="button" class="primary publish" data-action="publish">${status !== "published" ? "Opublikuj" : v!.event.draftRevision !== v!.event.publishedRevision ? "Opublikuj zmiany" : "Opublikuj ponownie"}</button>` : ""}
                ${status === "published" ? html`<a class="button" href="/app/orgs/${this.org}/events/${v!.event.id}/post">Ogłoś w mediach</a>` : ""}
                ${v ? html`<details class="more"><summary class="button">Więcej</summary><div class="menu">
                  ${status === "published" ? html`<button type="button" data-action="unpublish">Wycofaj ze strony</button>` : ""}
                  ${v ? html`<button type="button" data-action="duplicate">Duplikuj</button>
                    <button type="button" class="danger" data-action="delete">Przenieś do kosza</button>` : ""}
                </div></details>` : ""}`}
          </div>
        </div>
        <div id="problems">${v ? problemsHtml(v.problems) : ""}</div>
        ${v ? html`<p class="permalink muted small">Adres na stronie: <a href="${v.permalink.pl}" target="_blank" rel="noopener">${v.permalink.pl}</a></p>` : ""}

        <section class="card">
          <div class="section-head"><h2>Treść</h2>
            <div class="lang-tabs" role="tablist">
              <button type="button" role="tab" data-lang="pl" aria-selected="${this.editLang === "pl"}">Polski</button>
              <button type="button" role="tab" data-lang="en" aria-selected="${this.editLang === "en"}">English</button>
            </div>
          </div>
          <p class="muted small lang-en">Puste pola angielskie są na stronie zastępowane polskimi.</p>
          ${this.textFields("title", "Tytuł", c.title, "input", 200)}
          ${this.textFields("summary", "Krótki opis", c.summary, "short", 500)}
          ${this.textFields("description", "Opis", c.description, "long", 10000)}
          <div class="field"><label for="f-slug">Końcówka adresu</label>
            <input id="f-slug" name="slug" value="${c.slug}" pattern="[a-z0-9]+(-[a-z0-9]+)*" maxlength="80" placeholder="tworzona z tytułu">
            <p class="muted small">Można zmieniać – stare linki nadal działają dzięki stałemu identyfikatorowi ${v ? html`<code>${v.event.shortId}</code>` : ""}.</p></div>
        </section>

        <section class="card schedule">
          <h2>Termin</h2>
          <fieldset class="kinds"><legend class="sr-only">Rodzaj</legend>
            ${(
              [
                ["single", "Jednorazowe"],
                ["multiDay", "Wielodniowe"],
                ["recurring", "Cykliczne"],
              ] as [Kind, string][]
            ).map(([k, label]) => html`<label class="chip"><input type="radio" name="kind" value="${k}" ${s.kind === k ? "checked" : ""}> ${label}</label>`)}
          </fieldset>
          <div class="grid">
            <div class="field"><label for="f-startDate">Data rozpoczęcia</label><input type="date" id="f-startDate" name="startDate" value="${s.startDate}"></div>
            <div class="field"><label for="f-startTime">Początek</label><input type="time" id="f-startTime" name="startTime" value="${s.startTime}"></div>
            <div class="field k-multiDay"><label for="f-endDate">Data końca</label><input type="date" id="f-endDate" name="endDate" value="${s.endDate ?? ""}"></div>
            <div class="field"><label for="f-endTime">Koniec</label><input type="time" id="f-endTime" name="endTime" value="${s.endTime}"></div>
          </div>
          <p class="muted small k-single">Godzina końca wcześniejsza niż początek oznacza zakończenie następnego dnia.</p>
          <div class="grid k-recurring">
            <div class="field"><label for="f-freq">Powtarzanie</label><select id="f-freq" name="freq">
              <option value="weekly" ${r.freq === "weekly" ? "selected" : ""}>co tydzień</option>
              <option value="biweekly" ${r.freq === "biweekly" ? "selected" : ""}>co dwa tygodnie</option>
              <option value="monthly" ${r.freq === "monthly" ? "selected" : ""}>co miesiąc</option></select></div>
            <div class="field nth"><label for="f-nth">Który</label><select id="f-nth" name="nth">
              ${NTH.map(([n, label]) => html`<option value="${n}" ${r.nth === n ? "selected" : ""}>${label}</option>`)}</select></div>
            <div class="field"><label for="f-weekday">Dzień tygodnia</label><select id="f-weekday" name="weekday">
              ${WEEKDAYS.map((d, i) => html`<option value="${i}" ${r.weekday === i ? "selected" : ""}>${d}</option>`)}</select></div>
            <div class="field"><label for="f-until">Do dnia (najwyżej 24 miesiące)</label><input type="date" id="f-until" name="until" value="${r.endDate}"></div>
          </div>
          <div id="occurrences">${this.occurrencesHtml()}</div>
        </section>

        <section class="card">
          <h2>Miejsce i cena</h2>
          <div class="field"><label for="f-venue">Miejsce</label>
            <select id="f-venue" name="venueId"><option value="">— wybierz —</option>
              ${activeVenues.map((x) => html`<option value="${x.id}" ${c.venueId === x.id ? "selected" : ""}>${x.name} – ${x.address}${x.archived ? " (zarchiwizowane)" : ""}</option>`)}
            </select>
            <p class="muted small"><a href="/app/orgs/${this.org}/venues">Zarządzaj miejscami</a></p></div>
          ${this.textFields("price", "Cena / wstęp", c.price?.text, "input", 500)}
          <div class="field"><label for="f-priceUrl">Link do biletów lub zapisów</label><input type="url" id="f-priceUrl" name="priceUrl" value="${c.price?.url ?? ""}" placeholder="https://"></div>
        </section>

        <section class="card images">
          <h2>Obrazy</h2>
          ${this.imageBlock("cover", c.cover)}
          ${this.imageBlock("social", c.socialImage)}
        </section>
      </form>`);
  }

  private occurrencesHtml(): ReturnType<typeof html> {
    const occ = this.occurrences;
    if (!occ.length) return html`<p class="muted small">Uzupełnij termin, aby zobaczyć daty.</p>`;
    const recurring = this.content.schedule.kind === "recurring";
    const now = Date.now();
    return html`<h3>${recurring ? `Terminy (${occ.length})` : "Termin"}</h3>
      <ol class="occurrences">${occ.map((o) => {
        const ex = this.exceptions.get(o.key) ?? { date: o.key };
        const past = Date.parse(o.end) < now;
        return html`<li class="${o.cancelled ? "cancelled" : ""} ${past ? "past" : ""}" data-key="${o.key}">
          <span class="when">${fmtDateTime(o.start)} – ${fmtDateTime(o.end).split(", ").pop()}</span>
          ${recurring
            ? html`<label class="inline"><input type="checkbox" data-ex="cancelled" ${ex.cancelled ? "checked" : ""}> odwołane</label>
              <details ${ex.startTime || ex.endTime || ex.note?.pl ? "open" : ""}><summary>zmień</summary>
                <label class="inline">od <input type="time" data-ex="startTime" value="${ex.startTime ?? ""}"></label>
                <label class="inline">do <input type="time" data-ex="endTime" value="${ex.endTime ?? ""}"></label>
                <label class="inline">uwaga <input data-ex="note" value="${ex.note?.pl ?? ""}" maxlength="200" placeholder="np. zmiana sali"></label>
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
            box.insertAdjacentHTML("afterbegin", html`<ul class="hint">${res.problems.map((p) => html`<li>${p.message}</li>`)}</ul>`.value);
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
        toast("Utworzono szkic");
        navigate(`/app/orgs/${this.org}/events/${v.event.id}`);
        return undefined;
      }
      this.setView(v);
      this.render();
      toast("Zapisano szkic");
      return v;
    } catch (err) {
      if (err instanceof ApiError && err.code === "conflict") {
        toast("Ktoś inny zmienił to wydarzenie. Odśwież stronę, aby zobaczyć zmiany (Twoje zmiany nie zostały zapisane).", "error");
        return undefined;
      }
      showError(err, problems as HTMLElement);
      return undefined;
    }
  }

  private async action(name: "publish" | "unpublish" | "delete" | "restore" | "duplicate", btn: HTMLButtonElement): Promise<void> {
    const confirmText: Record<string, string> = {
      unpublish: "Wycofać wydarzenie ze strony? Pozostanie w CMS jako szkic.",
      delete: "Przenieść wydarzenie do kosza? Opublikowane zostanie wycofane ze strony. Posty na Facebooku i Instagramie pozostaną.",
    };
    if (confirmText[name] && !confirm(confirmText[name])) return;
    await busy(btn, async () => {
      if (name === "publish" && (this.dirty || !this.view)) {
        if (!(await this.save())) return;
      }
      try {
        const v = await api.eventAction(this.org, this.view!.event.id, name);
        if (name === "duplicate") {
          toast("Utworzono kopię – uzupełnij daty");
          navigate(`/app/orgs/${this.org}/events/${v.event.id}`);
          return;
        }
        this.setView(v);
        this.render();
        toast(
          { publish: "Opublikowano – strona zaktualizuje się w ciągu minuty", unpublish: "Wycofano ze strony", delete: "Przeniesiono do kosza", restore: "Przywrócono jako szkic" }[name] ??
            "Gotowe",
        );
      } catch (err) {
        showError(err, this.querySelector("#problems") as HTMLElement);
      }
    });
  }

  private async upload(kind: "cover" | "social", input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    if (!file) return;
    if (/\.(heic|heif)$/i.test(file.name) || /hei[cf]/i.test(file.type)) {
      toast("Format HEIC nie jest obsługiwany – zapisz zdjęcie jako JPEG.", "error");
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
      toast("Wgrano obraz – zapisz szkic");
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
      this.editLang = el.dataset.lang as "pl" | "en";
      this.$("#editor").dataset.lang = this.editLang;
      this.querySelectorAll("[role=tab]").forEach((t) => t.setAttribute("aria-selected", String((t as HTMLElement).dataset.lang === this.editLang)));
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
      if (t.dataset.ex || t.type === "file") return;
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

  private guard = (e: BeforeUnloadEvent) => {
    if (this.dirty && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
  }
}

// Listeners are delegated on the element itself, so re-renders keep them.
customElements.define("folk-editor", EditorView);
