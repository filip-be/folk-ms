// The Tune Collection of an Organization (only those that opted in): the
// Tunes (search and filters), the Sets, the lists Tunes are described with
// (Tune Types, Instruments, Origins) and the import of ABC files. Every
// change names the revision of the item it changes and gets the whole
// collection back; a conflict means that item changed meanwhile.

import { api, ApiError, type Bi, type ImportCandidate, type TuneCollection, type TuneCountry, type TuneInput, type TuneLabeled, type TuneListKind, type TuneSet, type TuneType } from "../api.ts";
import { icon } from "../icons.ts";
import { inLang, L, lang } from "../i18n.ts";
import { bindSortable, grip, moveButtons } from "../patterns.ts";
import { busy, html, markProblems, navigate, raw, showError, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";
import { biBoth, biText, fold, keyText, nameText, tuneOriginText, tuneUsesText, typeText } from "./tuneParts.ts";

const TABS = [
  ["tunes", L("Melodie", "Tunes")],
  ["sets", L("Zestawy", "Sets")],
  ["lists", L("Listy", "Lists")],
  ["import", L("Import ABC", "ABC import")],
] as const;
type Tab = (typeof TABS)[number][0];
type HiddenFilter = "all" | "visible" | "hidden";

/** The largest ABC file the import reads (domain.MaxImportBytes). */
const MAX_IMPORT_BYTES = 1 << 20;
const MIN_SET = 2;
const MAX_SET = 10;
const MAX_LABEL = 60;

/** A Set being written in the Sets tab's form: new (no id) or one being edited. */
interface SetDraft {
  id?: string;
  rev: number;
  tuneIds: string[];
}

/** Problems of an import candidate that the table resolves itself (a Tune Type select; its Time Signature). */
const RESOLVED = new Set(["typeId", "meter"]);

class TunesView extends View {
  private tab: Tab = "tunes";
  private c!: TuneCollection;
  private q = "";
  private type = "";
  private vis: HiddenFilter = "all";
  /** The list entry being edited ("" when none). */
  private renaming = "";
  private draft: SetDraft = { rev: 0, tuneIds: [] };
  private candidates?: ImportCandidate[];

  async load(): Promise<void> {
    const q = new URLSearchParams(location.search);
    const v = q.get("view");
    this.tab = TABS.some(([k]) => k === v) ? (v as Tab) : "tunes";
    this.q = q.get("q") ?? "";
    this.type = q.get("type") ?? "";
    const h = q.get("hidden");
    this.vis = h === "visible" || h === "hidden" ? h : "all";
    this.c = await api.tunes(this.org);
    this.render();
    this.bind();
  }

  // ── Changes ───────────────────────────────────────────────────────────

  /**
   * Runs a change and redraws. A conflict reloads the collection (the item
   * changed meanwhile). Problems go next to the fields of `form`, or into a toast.
   */
  private async change(btn: HTMLButtonElement | null, fn: () => Promise<TuneCollection>, done?: string, form?: HTMLFormElement | null, refocus?: () => void): Promise<boolean> {
    let ok = false;
    await busy(btn, async () => {
      try {
        this.c = await fn();
        this.renaming = "";
        this.render();
        if (done) toast(done);
        ok = true;
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          this.c = await api.tunes(this.org);
          this.renaming = "";
          this.render();
          toast(L("Ktoś inny zmienił ten wpis w międzyczasie. Wczytano aktualną wersję – sprawdź ją i spróbuj ponownie.", "Someone else changed this entry meanwhile. The current version is loaded – check it and try again."), "error");
          return;
        }
        if (!form && err instanceof ApiError && err.problems.length) toast(err.problems.map((p) => inLang(p.message)).join("\n"), "error");
        else showError(err, form?.querySelector<HTMLElement>(".problems-slot"), form);
      }
    });
    if (ok) refocus?.();
    return ok;
  }

  private focusFirst(...selectors: string[]): void {
    for (const sel of selectors) {
      const el = this.querySelector<HTMLElement>(sel);
      if (el && !(el as HTMLButtonElement).disabled) {
        el.focus();
        return;
      }
    }
  }

  // ── Tunes tab ─────────────────────────────────────────────────────────

  private filtered() {
    const q = fold(this.q.trim());
    return this.c.tunes.filter((tu) => {
      if (this.type && tu.typeId !== this.type) return false;
      if (this.vis === "hidden" && !tu.hidden) return false;
      if (this.vis === "visible" && tu.hidden) return false;
      if (!q) return true;
      return [tu.name, ...tu.otherNames].some((n) => [n.name, n.translation?.pl, n.translation?.en].some((s) => s && fold(s).includes(q)));
    });
  }

  private tuneRows(): Raw {
    const list = this.filtered();
    if (!this.c.tunes.length) return html`<div class="card empty"><p>${L("Nie ma jeszcze melodii.", "There are no tunes yet.")}</p></div>`;
    if (!list.length) return html`<div class="card empty"><p>${L("Żadna melodia nie pasuje do wyszukiwania.", "No tune matches the search.")}</p></div>`;
    return html`<p class="muted small" aria-live="polite">${L(`Melodie: ${list.length} z ${this.c.tunes.length}`, `Tunes: ${list.length} of ${this.c.tunes.length}`)}</p>
      <div class="table-wrap"><table class="list tunes">
        <thead><tr><th>${L("Nazwa", "Name")}</th><th>${L("Rodzaj", "Tune Type")}</th><th>${L("Tonacja", "Key")}</th><th>${L("Metrum", "Time Signature")}</th></tr></thead>
        <tbody>${list.map((tu) => {
          const others = tu.otherNames.map(nameText).join(" · ");
          const badges: Raw[] = [];
          if (tu.hidden) badges.push(html`<span class="badge">${L("ukryta", "hidden")}</span>`);
          if (!tu.hasTranscription) badges.push(html`<span class="badge">${L("bez zapisu ABC", "no ABC")}</span>`);
          if (tu.setIds.length) badges.push(html`<span class="badge info">${L(`w zestawach: ${tu.setIds.length}`, `in ${tu.setIds.length} set${tu.setIds.length === 1 ? "" : "s"}`)}</span>`);
          return html`<tr data-id="${tu.id}" tabindex="0" class="${tu.hidden ? "hidden-row" : ""}">
            <td><a href="/app/orgs/${this.org}/tunes/${tu.id}">${nameText(tu.name)}</a>
              ${others ? html`<div class="muted small">${others}</div>` : ""}
              ${badges.length ? html`<span class="badges">${badges}</span>` : ""}</td>
            <td>${typeText(this.c, tu.typeId)}</td>
            <td>${keyText(tu.key)}</td>
            <td>${tu.meter}</td>
          </tr>`;
        })}</tbody></table></div>`;
  }

  private tunesTab(): Raw {
    return html`
      <form class="filters" role="search" id="tune-filters">
        <div class="field grow"><label for="tf-q">${L("Szukaj", "Search")}</label>
          <input id="tf-q" type="search" value="${this.q}" placeholder="${L("nazwa, inna nazwa lub tłumaczenie", "name, other name or translation")}"></div>
        <div class="field"><label for="tf-type">${L("Rodzaj melodii", "Tune Type")}</label>
          <select id="tf-type"><option value="">${L("Wszystkie", "All")}</option>${this.c.types.map((ty) => html`<option value="${ty.id}" ${ty.id === this.type ? "selected" : ""}>${biText(ty.label)}</option>`)}</select></div>
        <div class="field"><label for="tf-hidden">${L("Widoczność", "Visibility")}</label>
          <select id="tf-hidden">
            <option value="all">${L("Wszystkie", "All")}</option>
            <option value="visible" ${this.vis === "visible" ? "selected" : ""}>${L("Na stronie", "On the Site")}</option>
            <option value="hidden" ${this.vis === "hidden" ? "selected" : ""}>${L("Ukryte", "Hidden")}</option>
          </select></div>
      </form>
      <div id="tune-rows">${this.tuneRows()}</div>`;
  }

  /** Keeps the filters in the address, so Back returns to the same list. */
  private applyFilters(): void {
    this.q = this.querySelector<HTMLInputElement>("#tf-q")?.value ?? "";
    this.type = this.querySelector<HTMLSelectElement>("#tf-type")?.value ?? "";
    this.vis = (this.querySelector<HTMLSelectElement>("#tf-hidden")?.value ?? "all") as HiddenFilter;
    const p = new URLSearchParams();
    if (this.q) p.set("q", this.q);
    if (this.type) p.set("type", this.type);
    if (this.vis !== "all") p.set("hidden", this.vis);
    history.replaceState(null, "", location.pathname + (p.size ? `?${p}` : ""));
    this.$("#tune-rows").innerHTML = this.tuneRows().value;
  }

  // ── Sets tab ──────────────────────────────────────────────────────────

  private tuneName(id: string): string {
    const tu = this.c.tunes.find((x) => x.id === id);
    return tu ? nameText(tu.name) : L("(usunięta)", "(removed)");
  }

  private draftTunes(): Raw {
    const ids = this.draft.tuneIds;
    const left = this.c.tunes.filter((tu) => !ids.includes(tu.id));
    const full = ids.length >= MAX_SET;
    return html`
      ${ids.length
        ? html`<ol class="sortable set-tunes" data-list="set-tunes" aria-label="${L("Melodie zestawu", "Tunes of the set")}">${ids.map((id, i) => {
            const tu = this.c.tunes.find((x) => x.id === id);
            return html`<li data-id="${id}" draggable="true">${grip()}<span class="order">${i + 1}.</span>
              <div class="row-main"><strong>${this.tuneName(id)}</strong>${tu ? html` <span class="muted small">${typeText(this.c, tu.typeId)} · ${keyText(tu.key)}</span>` : ""}
                ${tu?.hidden ? html`<span class="badges"><span class="badge">${L("ukryta", "hidden")}</span></span>` : ""}</div>
              <span class="row-actions">
                <button type="button" class="link danger" data-set-drop="${id}" aria-label="${L(`Usuń z zestawu: ${this.tuneName(id)}`, `Remove from the set: ${this.tuneName(id)}`)}">${L("Usuń z zestawu", "Remove from set")}</button>
                ${moveButtons("set-tunes", id, i, ids.length, this.tuneName(id))}
              </span></li>`;
          })}</ol>`
        : html`<p class="muted small">${L("Jeszcze bez melodii.", "No tunes yet.")}</p>`}
      <div class="inline-form set-pick">
        <div class="field"><label for="set-pick">${L("Dodaj melodię", "Add a tune")}</label>
          <select id="set-pick" ${full || !left.length ? "disabled" : ""}>${left.map((tu) => html`<option value="${tu.id}">${nameText(tu.name)} – ${typeText(this.c, tu.typeId)}${tu.hidden ? L(" (ukryta)", " (hidden)") : ""}</option>`)}</select></div>
        <button type="button" id="set-add" ${full || !left.length ? "disabled" : ""}>${icon("plus")}${L("Dodaj do zestawu", "Add to set")}</button>
      </div>
      <p class="muted small" id="set-count">${full ? L(`Zestaw ma już ${MAX_SET} melodii (najwięcej).`, `The set already has ${MAX_SET} tunes (the most).`) : L(`Od ${MIN_SET} do ${MAX_SET} melodii, w kolejności grania.`, `${MIN_SET} to ${MAX_SET} tunes, in playing order.`)}</p>`;
  }

  private setForm(): Raw {
    const s = this.draft.id ? this.c.sets.find((x) => x.id === this.draft.id) : undefined;
    return html`<section class="card" aria-labelledby="set-form-title">
      <h2 id="set-form-title">${s ? L(`Edycja zestawu: ${biText(s.title)}`, `Edit set: ${biText(s.title)}`) : L("Nowy zestaw", "New set")}</h2>
      <form id="set-form">
        <div class="problems-slot"></div>
        <p class="required-note">${L("wymagane", "required")}</p>
        <div class="pair">
          <div class="field"><label for="set-name-pl">${L("Nazwa", "Name")} <span class="muted">${L("(PL, opcjonalnie)", "(PL, optional)")}</span></label>
            <input id="set-name-pl" name="name.pl" value="${s?.name.pl ?? ""}" maxlength="200" aria-describedby="set-name-hint"></div>
          <div class="field"><label for="set-name-en">${L("Nazwa", "Name")} <span class="muted">${L("(EN, opcjonalnie)", "(EN, optional)")}</span></label>
            <input id="set-name-en" name="name.en" value="${s?.name.en ?? ""}" maxlength="200" aria-describedby="set-name-hint"></div>
        </div>
        <p class="muted small" id="set-name-hint">${L("Bez nazwy strona pokazuje nazwy melodii („Melodia A / Melodia B”).", "Without a name the Site shows the tunes' names (“Tune A / Tune B”).")}</p>
        <fieldset data-field="tuneIds"><legend class="required">${L("Melodie", "Tunes")}</legend>
          <div id="set-tunes">${this.draftTunes()}</div>
        </fieldset>
        <div class="actions">
          <button type="submit" class="primary">${s ? L("Zapisz zestaw", "Save set") : html`${icon("plus")}${L("Utwórz zestaw", "Create set")}`}</button>
          ${s ? html`<button type="button" id="set-cancel">${L("Anuluj", "Cancel")}</button>` : ""}
        </div>
      </form>
    </section>`;
  }

  private setRow(s: TuneSet): Raw {
    const status = s.hidden
      ? html`<span class="badge">${L("ukryty", "hidden")}</span>`
      : s.published
        ? html`<span class="badge published">${L("na stronie", "on the Site")}</span>`
        : html`<span class="badge warn">${L("nieopublikowany: mniej niż 2 widoczne melodie", "not published: fewer than 2 visible tunes")}</span>`;
    const title = biText(s.title);
    return html`<li data-id="${s.id}" class="${s.hidden ? "hidden-row" : ""}">
      <div class="row-main"><strong>${title}</strong>
        <div class="muted small">${s.tuneIds.map((id) => this.tuneName(id)).join(" → ")}</div>
        <span class="badges">${status}</span></div>
      <span class="row-actions">
        ${s.published && s.permalink ? html`<a class="button" href="${s.permalink[lang]}" target="_blank" rel="noopener">${icon("external")}${L("Zobacz na stronie", "View on Site")}</a>` : ""}
        <button type="button" data-set-edit="${s.id}">${L("Edytuj", "Edit")}</button>
        <button type="button" data-set-hide="${s.id}" data-hidden="${s.hidden ? "1" : ""}">${s.hidden ? L("Pokaż na stronie", "Show on the Site") : L("Ukryj", "Hide")}</button>
        <button type="button" class="link danger" data-set-remove="${s.id}" aria-label="${L(`Usuń zestaw: ${title}`, `Remove the set: ${title}`)}">${L("Usuń", "Remove")}</button>
      </span>
    </li>`;
  }

  private setsTab(): Raw {
    const ss = this.c.sets;
    return html`
      <p class="muted small">${L(
        "Zestaw to kilka melodii (2–10) granych jedna po drugiej. Strona pokazuje zestaw, gdy nie jest ukryty i ma co najmniej 2 widoczne melodie.",
        "A Set is a few Tunes (2–10) played back to back. The Site shows a set when it is not hidden and has at least 2 visible tunes.",
      )}</p>
      ${this.setForm()}
      ${ss.length
        ? html`<ul class="item-list" aria-label="${L("Zestawy", "Sets")}">${ss.map((s) => this.setRow(s))}</ul>`
        : html`<div class="card empty"><p>${L("Nie ma jeszcze zestawów.", "There are no sets yet.")}</p></div>`}`;
  }

  private redrawDraft(): void {
    this.$("#set-tunes").innerHTML = this.draftTunes().value;
  }

  // ── Lists tab ─────────────────────────────────────────────────────────

  /** The label in both languages; a region's form also marks a problem with its country (parentId). */
  private labelFields(id: string, label?: Bi, parent = ""): Raw {
    return html`<div class="field"><label for="${id}-pl">${L("Nazwa (PL)", "Name (PL)")}</label><input id="${id}-pl" name="label.pl" ${parent ? raw('data-field="label.pl parentId"') : ""} value="${label?.pl ?? ""}" maxlength="${MAX_LABEL}" aria-describedby="labels-hint"></div>
      <div class="field"><label for="${id}-en">${L("Nazwa (EN)", "Name (EN)")}</label><input id="${id}-en" name="label.en" value="${label?.en ?? ""}" maxlength="${MAX_LABEL}" aria-describedby="labels-hint"></div>`;
  }

  private typeFields(id: string, ty?: TuneType): Raw {
    return html`<div class="field narrow-field"><label for="${id}-meter">${L("Metrum", "Time Signature")}</label><input id="${id}-meter" name="meter" value="${ty?.meter ?? ""}" required placeholder="4/4" maxlength="7"></div>
      <div class="field narrow-field"><label for="${id}-tempo">${L("Tempo", "Tempo")}</label><input id="${id}-tempo" name="tempo" type="number" min="20" max="400" value="${ty?.tempo ?? ""}" required></div>`;
  }

  private addForm(kind: TuneListKind, id: string, legend: string, parent = ""): Raw {
    return html`<form class="inline-form label-form add" data-add="${id}" data-kind="${kind}" data-parent="${parent}" aria-label="${legend}">
      <div class="problems-slot"></div>
      ${this.labelFields(id, undefined, parent)}
      ${kind === "types" ? this.typeFields(id) : ""}
      <button type="submit" class="primary">${icon("plus")}${L("Dodaj", "Add")}</button>
    </form>`;
  }

  private labelCell(kind: TuneListKind, item: TuneLabeled | TuneType): Raw {
    const ty = kind === "types" ? (item as TuneType) : undefined;
    if (this.renaming !== item.id) {
      return html`<div class="row-main"><strong>${biBoth(item.label)}</strong>
        ${ty ? html` <span class="muted small">${ty.meter} · ♩ ${ty.tempo}</span>` : ""}
        <span class="muted small">${tuneUsesText(item.uses)}</span></div>`;
    }
    const id = `rename-${item.id}`;
    return html`<form class="inline-form label-form row-main" data-rename="${item.id}" data-kind="${kind}" data-rev="${item.rev}">
      <div class="problems-slot"></div>
      ${this.labelFields(id, item.label)}
      ${ty ? this.typeFields(id, ty) : ""}
      <button type="submit" class="primary">${L("Zapisz", "Save")}</button>
      <button type="button" data-cancel-rename>${L("Anuluj", "Cancel")}</button>
    </form>`;
  }

  private itemActions(kind: TuneListKind, item: TuneLabeled, i: number, n: number, blocked: string): Raw {
    const name = biText(item.label);
    return html`<span class="row-actions">
      ${this.renaming === item.id ? "" : html`<button type="button" data-rename-start="${item.id}">${L("Edytuj", "Edit")}</button>`}
      <button type="button" class="link danger" data-remove="${item.id}" data-kind="${kind}" ${blocked ? html`disabled title="${blocked}"` : ""}
        aria-label="${L(`Usuń: ${name}`, `Remove: ${name}`)}">${L("Usuń", "Remove")}</button>
      ${moveButtons(kind, item.id, i, n, name)}
    </span>`;
  }

  private inUse = (n: number) => (n ? L(`Używane ${tuneUsesText(n)}`, `Used ${tuneUsesText(n)}`) : "");

  private simpleList(kind: "types" | "instruments", items: TuneLabeled[], label: string): Raw {
    const drag = this.renaming ? "false" : "true";
    return items.length
      ? html`<ol class="sortable labels" data-list="${kind}" aria-label="${label}">${items.map(
          (it, i) => html`<li data-id="${it.id}" draggable="${drag}">${grip()}${this.labelCell(kind, it)}${this.itemActions(kind, it, i, items.length, this.inUse(it.uses))}</li>`,
        )}</ol>`
      : html`<div class="card empty"><p>${L("Lista jest pusta.", "The list is empty.")}</p></div>`;
  }

  private countryRow(co: TuneCountry, i: number, n: number): Raw {
    const drag = this.renaming ? "false" : "true";
    const blocked = co.regions.length ? L("Najpierw usuń regiony", "Remove its regions first") : this.inUse(co.uses);
    return html`<li data-id="${co.id}" draggable="${drag}">
      <div class="country">${grip()}${this.labelCell("origins", co)}${this.itemActions("origins", co, i, n, blocked)}</div>
      ${co.regions.length
        ? html`<ol class="sortable labels regions" data-list="origins" aria-label="${L(`Regiony: ${biText(co.label)}`, `Regions: ${biText(co.label)}`)}">${co.regions.map(
            (r, j) => html`<li data-id="${r.id}" draggable="${drag}">${grip()}${this.labelCell("origins", r)}${this.itemActions("origins", r, j, co.regions.length, this.inUse(r.uses))}</li>`,
          )}</ol>`
        : ""}
      <details class="add-region"><summary class="button">${icon("plus")}${L("Dodaj region", "Add a region")}</summary>
        ${this.addForm("origins", `new-region-${co.id}`, L(`Nowy region: ${biText(co.label)}`, `New region: ${biText(co.label)}`), co.id)}</details>
    </li>`;
  }

  private listsTab(): Raw {
    const os = this.c.origins;
    return html`
      <p class="muted small" id="labels-hint">${L(
        "Nazwy wpisz po polsku, po angielsku albo w obu językach – wystarczy jeden; gdy brakuje drugiego, strona pokaże ten, który jest. Wpisu używanego przez melodie nie można usunąć.",
        "Write each name in Polish, English or both – one is enough; where one is missing the Site shows the other. An entry that tunes use cannot be removed.",
      )}</p>
      <section class="list-section" aria-labelledby="types-title">
        <h2 id="types-title">${L("Rodzaje melodii", "Tune Types")}</h2>
        <p class="muted small">${L("Każdy rodzaj proponuje metrum i tempo odtwarzania (w uderzeniach na minutę).", "Each type proposes a Time Signature and a Playback Tempo (beats a minute).")}</p>
        <div class="card">${this.addForm("types", "new-type", L("Nowy rodzaj melodii", "New tune type"))}</div>
        ${this.simpleList("types", this.c.types, L("Rodzaje melodii", "Tune Types"))}
      </section>
      <section class="list-section" aria-labelledby="instruments-title">
        <h2 id="instruments-title">${L("Instrumenty", "Instruments")}</h2>
        <div class="card">${this.addForm("instruments", "new-instrument", L("Nowy instrument", "New instrument"))}</div>
        ${this.simpleList("instruments", this.c.instruments, L("Instrumenty", "Instruments"))}
      </section>
      <section class="list-section" aria-labelledby="origins-title">
        <h2 id="origins-title">${L("Pochodzenie", "Origins")}</h2>
        <p class="muted small">${L("Kraje i (opcjonalnie) ich regiony. Melodia ma najwyżej jedno pochodzenie; kraj obejmuje swoje regiony.", "Countries and (optionally) their regions. A tune has at most one origin; a country includes its regions.")}</p>
        <div class="card">${this.addForm("origins", "new-country", L("Nowy kraj", "New country"))}</div>
        ${os.length
          ? html`<ol class="sortable labels countries" data-list="origins" aria-label="${L("Kraje", "Countries")}">${os.map((co, i) => this.countryRow(co, i, os.length))}</ol>`
          : html`<div class="card empty"><p>${L("Nie ma jeszcze krajów.", "There are no countries yet.")}</p></div>`}
      </section>`;
  }

  // ── Import tab ────────────────────────────────────────────────────────

  private importTab(): Raw {
    return html`
      <section class="card" aria-labelledby="import-title">
        <h2 id="import-title">${L("Plik ABC", "ABC file")}</h2>
        <form id="import-form">
          <div class="problems-slot"></div>
          <div class="field"><label for="import-text">${L("Zapis ABC (jedna lub wiele melodii, każda od X:)", "ABC (one or more tunes, each starting with X:)")}</label>
            <textarea id="import-text" class="abc-text" name="abc" data-field="abc" rows="10" required spellcheck="false" aria-describedby="import-hint"></textarea>
            <p class="muted small" id="import-hint">${L(
              "Wklej zapis albo wybierz plik .abc (do 1 MB). Każda melodia zostanie dodana jako ukryta – pokażesz ją na stronie po sprawdzeniu.",
              "Paste the text or choose an .abc file (up to 1 MB). Each tune is added hidden – show it on the Site once checked.",
            )}</p></div>
          <div class="actions">
            <button type="submit" class="primary">${L("Pokaż podgląd", "Preview")}</button>
            <label class="button" for="import-file">${L("Wybierz plik .abc…", "Choose .abc file…")}<input type="file" id="import-file" accept=".abc,text/plain,text/vnd.abc" hidden></label>
          </div>
        </form>
      </section>
      <div id="import-result">${this.importResult()}</div>`;
  }

  private typeOptions(selected?: string): Raw {
    return html`<option value="">${L("— wybierz —", "— choose —")}</option>${this.c.types.map((ty) => html`<option value="${ty.id}" ${ty.id === selected ? "selected" : ""}>${biText(ty.label)} (${ty.meter})</option>`)}`;
  }

  /** Problems the Moderator cannot fix in the table: the candidate cannot be created. */
  private blocking(cand: ImportCandidate) {
    return (cand.problems ?? []).filter((p) => !RESOLVED.has(p.field));
  }

  private importResult(): Raw {
    const cs = this.candidates;
    if (!cs) return raw("");
    if (!cs.length) return html`<div class="card empty"><p>${L("Nie znaleziono melodii.", "No tunes found.")}</p></div>`;
    return html`<section class="card" aria-labelledby="import-list-title">
      <h2 id="import-list-title">${L(`Znalezione melodie: ${cs.length}`, `Tunes found: ${cs.length}`)}</h2>
      <form id="import-create">
        <div class="problems-slot"></div>
        <p class="muted small">${L(
          "Zaznacz melodie do dodania. Gdy rodzaju (R:) nie ma na liście, wybierz go; brakujące metrum zostanie wzięte z rodzaju. Melodii bez tytułu, tonacji albo z błędnym zapisem nie można dodać – popraw plik i pokaż podgląd ponownie.",
          "Tick the tunes to add. Where the type (R:) is not on the list, choose one; a missing time signature is taken from the type. Tunes without a title or key, or with broken ABC, cannot be added – fix the file and preview again.",
        )}</p>
        <div class="table-wrap" data-field="tunes"><table class="list import">
          <thead><tr><th><span class="sr-only">${L("Dodaj", "Add")}</span></th><th>${L("Nazwa", "Name")}</th><th>${L("Rodzaj melodii", "Tune Type")}</th><th>${L("Tonacja i metrum", "Key and time signature")}</th><th>${L("Pochodzenie i kompozytor", "Origin and composer")}</th></tr></thead>
          <tbody>${cs.map((cand) => {
            const blocking = this.blocking(cand);
            const id = `cand-${cand.index}`;
            const others = cand.otherNames.map(nameText).join(" · ");
            const ty = this.c.types.find((x) => x.id === cand.typeId);
            return html`<tr data-cand="${cand.index}" class="${blocking.length ? "blocked" : ""}">
              <td><input type="checkbox" id="${id}" ${blocking.length ? "disabled" : "checked"}></td>
              <td><label for="${id}" id="${id}-name"><span data-cf="name" aria-label="${L("Nazwa", "Name")}">${cand.name.name || L("(bez tytułu)", "(no title)")}</span></label>
                ${others ? html`<div class="muted small">${others}</div>` : ""}
                ${cand.duplicate?.length ? html`<span class="badges"><span class="badge warn">${L("ta sama nazwa: ", "same name as: ")}${cand.duplicate.join(", ")}</span></span>` : ""}
                ${blocking.length ? html`<ul class="candidate-problems">${blocking.map((p) => html`<li>${icon("alert")}<span>${inLang(p.message)}</span></li>`)}</ul>` : ""}
                <details class="abc-source"><summary>${L("Zapis ABC", "ABC")}</summary><pre data-cf="abc" aria-label="ABC">${cand.abc}</pre></details></td>
              <td><label class="sr-only" for="${id}-type">${L(`Rodzaj melodii: ${cand.name.name}`, `Tune Type: ${cand.name.name}`)}</label>
                <select id="${id}-type" data-cf="typeId" ${blocking.length ? "disabled" : ""}>${this.typeOptions(cand.typeId)}</select>
                ${cand.rhythm && !ty ? html`<div class="muted small">R: ${cand.rhythm} – ${L("nie ma na liście", "not on the list")}</div>` : ""}</td>
              <td><span data-cf="key" aria-label="${L("Tonacja", "Key")}">${keyText(cand.key) || "–"}</span><br>
                <span data-cf="meter" class="cand-meter" aria-label="${L("Metrum", "Time Signature")}">${this.candidateMeter(cand, cand.typeId)}</span></td>
              <td><span data-cf="originId" aria-label="${L("Pochodzenie", "Origin")}">${cand.originId ? tuneOriginText(this.c, cand.originId) : cand.origin ? html`<span class="muted">„${cand.origin}” – ${L("nie ma na liście", "not on the list")}</span>` : "–"}</span>
                ${cand.composer ? html`<div class="muted small" data-cf="composer" aria-label="${L("Kompozytor", "Composer")}">${cand.composer}</div>` : ""}</td>
            </tr>`;
          })}</tbody></table></div>
        <div class="actions form-actions">
          <button type="submit" class="primary" id="import-go">${L("Dodaj zaznaczone (ukryte)", "Add the ticked tunes (hidden)")}</button>
          <span class="muted small" id="import-count" aria-live="polite"></span>
        </div>
      </form>
    </section>`;
  }

  private candidateMeter(cand: ImportCandidate, typeId?: string): string {
    if (cand.meter) return cand.meter;
    const ty = this.c.types.find((x) => x.id === typeId);
    return ty ? L(`${ty.meter} (z rodzaju)`, `${ty.meter} (from the type)`) : "–";
  }

  private countTicked(): void {
    const n = this.querySelectorAll("#import-create tbody input[type=checkbox]:checked").length;
    const out = this.querySelector("#import-count");
    if (out) out.textContent = L(`Zaznaczone: ${n}`, `Ticked: ${n}`);
    const go = this.querySelector<HTMLButtonElement>("#import-go");
    if (go) go.disabled = n === 0;
  }

  private async previewImport(btn: HTMLButtonElement | null): Promise<void> {
    const form = this.$<HTMLFormElement>("#import-form");
    const text = this.$<HTMLTextAreaElement>("#import-text").value;
    await busy(btn, async () => {
      try {
        const r = await api.importPreview(this.org, text);
        this.candidates = r.candidates;
        markProblems(form, [], form.querySelector(".problems-slot"));
        this.$("#import-result").innerHTML = this.importResult().value;
        this.countTicked();
        this.querySelector<HTMLElement>("#import-list-title")?.scrollIntoView({ block: "start", behavior: "smooth" });
      } catch (err) {
        showError(err, form.querySelector<HTMLElement>(".problems-slot"), form);
      }
    });
  }

  private async createImported(form: HTMLFormElement): Promise<void> {
    const rows = [...form.querySelectorAll<HTMLTableRowElement>("tbody tr[data-cand]")];
    const tunes: TuneInput[] = [];
    rows.forEach((tr) => {
      tr.querySelectorAll<HTMLElement>("[data-cf]").forEach((el) => el.removeAttribute("data-field"));
      if (!tr.querySelector<HTMLInputElement>("input[type=checkbox]")!.checked) return;
      const cand = this.candidates!.find((x) => x.index === Number(tr.dataset.cand))!;
      const typeId = tr.querySelector<HTMLSelectElement>("select[data-cf=typeId]")!.value;
      const ty = this.c.types.find((x) => x.id === typeId);
      const j = tunes.length;
      // The problem paths of this candidate as the CMS names them (tunes.<j>.<field>).
      const paths: Record<string, string[]> = {
        name: ["name.name", "name.translation.pl", "name.translation.en", "otherNames", ...Array.from({ length: 10 }, (_, k) => [`otherNames.${k}.name`, `otherNames.${k}.translation.pl`, `otherNames.${k}.translation.en`]).flat()],
        typeId: ["typeId"],
        key: ["key"],
        meter: ["meter", "tempo"],
        originId: ["originId"],
        composer: ["composer"],
        abc: ["abc", "incipitBars", "recordings", "description", "description.pl", "description.en", "suits", "tricky"],
      };
      tr.querySelectorAll<HTMLElement>("[data-cf]").forEach((el) => el.setAttribute("data-field", (paths[el.dataset.cf!] ?? []).map((p) => `tunes.${j}.${p}`).join(" ")));
      tunes.push({
        name: cand.name,
        otherNames: cand.otherNames,
        typeId,
        key: cand.key ?? { tonic: "", mode: "" },
        meter: cand.meter || ty?.meter || "",
        tempo: 0,
        originId: cand.originId ?? "",
        composer: cand.composer ?? "",
        suits: [],
        tricky: [],
        abc: cand.abc,
        incipitBars: 0,
        recordings: [],
        description: {},
      });
    });
    await busy(form.querySelector<HTMLButtonElement>("#import-go"), async () => {
      try {
        this.c = await api.importTunes(this.org, tunes);
        toast(L(`Dodano melodie: ${tunes.length} (ukryte). Sprawdź je i pokaż na stronie.`, `Added ${tunes.length} tune${tunes.length === 1 ? "" : "s"} (hidden). Check them and show them on the Site.`));
        navigate(`/app/orgs/${this.org}/tunes?hidden=hidden`);
      } catch (err) {
        showError(err, form.querySelector<HTMLElement>(".problems-slot"), form);
      }
    });
  }

  private async readImportFile(input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    if (file.size > MAX_IMPORT_BYTES) {
      toast(L("Plik jest za duży (najwyżej 1 MB).", "The file is too large (at most 1 MB)."), "error");
      return;
    }
    this.$<HTMLTextAreaElement>("#import-text").value = await file.text();
    await this.previewImport(this.querySelector<HTMLButtonElement>("#import-form button[type=submit]"));
  }

  // ── Screen ────────────────────────────────────────────────────────────

  private render(): void {
    const body = this.tab === "sets" ? this.setsTab() : this.tab === "lists" ? this.listsTab() : this.tab === "import" ? this.importTab() : this.tunesTab();
    this.set(html`
      <div class="toolbar">
        <h1>${L("Melodie", "Tunes")}</h1>
        <a class="button" href="/app/orgs/${this.org}/tunes/new">${icon("plus")}${L("Dodaj melodię", "Add tune")}</a>
      </div>
      <nav class="tabs" aria-label="${L("Widok", "View")}">
        ${TABS.map(([k, label]) => html`<a href="/app/orgs/${this.org}/tunes?view=${k}" class="${k === this.tab ? "active" : ""}" ${k === this.tab ? raw('aria-current="page"') : ""}>${label}</a>`)}
      </nav>
      ${body}`);
    if (this.tab === "import") this.countTicked();
  }

  private bind(): void {
    // Tunes
    this.on("#tune-filters", "input", () => this.applyFilters());
    this.on("tr[data-id]", "click", (e, tr) => {
      if ((e.target as HTMLElement).closest("a")) return;
      navigate(`/app/orgs/${this.org}/tunes/${tr.dataset.id}`);
    });
    this.on("tr[data-id]", "keydown", (e, tr) => {
      if ((e as KeyboardEvent).key === "Enter" && e.target === tr) navigate(`/app/orgs/${this.org}/tunes/${tr.dataset.id}`);
    });

    // Moves: list entries on the CMS, a Set's Tunes in the form.
    bindSortable(this, (list, id, to, button) => {
      const sel = (dir: string) => `button[data-move="${id}"][data-list="${list}"][data-dir="${dir}"]`;
      const dir = button?.dataset.dir ?? "";
      const refocus = () => button && this.focusFirst(sel(dir), sel(dir === "up" ? "down" : "up"));
      if (list === "set-tunes") {
        const ids = this.draft.tuneIds.filter((x) => x !== id);
        ids.splice(to, 0, id);
        this.draft.tuneIds = ids;
        this.redrawDraft();
        refocus();
        return;
      }
      void this.change(button ?? null, () => api.moveTuneListItem(this.org, list as TuneListKind, id, to), undefined, null, refocus);
    });

    // Sets
    this.on("#set-add", "click", () => {
      const id = this.querySelector<HTMLSelectElement>("#set-pick")?.value;
      if (!id || this.draft.tuneIds.includes(id) || this.draft.tuneIds.length >= MAX_SET) return;
      this.draft.tuneIds.push(id);
      this.redrawDraft();
      this.focusFirst("#set-pick", "#set-form button[type=submit]");
    });
    this.on("button[data-set-drop]", "click", (_e, el) => {
      this.draft.tuneIds = this.draft.tuneIds.filter((x) => x !== el.dataset.setDrop);
      this.redrawDraft();
      this.focusFirst("#set-pick");
    });
    this.on("button[data-set-edit]", "click", (_e, el) => {
      const s = this.c.sets.find((x) => x.id === el.dataset.setEdit);
      if (!s) return;
      this.draft = { id: s.id, rev: s.rev, tuneIds: [...s.tuneIds] };
      this.render();
      this.$("#set-form-title").scrollIntoView({ block: "start", behavior: "smooth" });
      this.querySelector<HTMLInputElement>("#set-name-pl")?.focus({ preventScroll: true });
    });
    this.on("#set-cancel", "click", () => {
      const id = this.draft.id;
      this.draft = { rev: 0, tuneIds: [] };
      this.render();
      this.focusFirst(`button[data-set-edit="${id}"]`);
    });
    this.on("button[data-set-hide]", "click", (_e, el) => {
      const id = el.dataset.setHide!;
      const hide = !el.dataset.hidden;
      void this.change(el as HTMLButtonElement, () => api.setAction(this.org, id, hide ? "hide" : "show"),
        hide ? L("Ukryto zestaw", "Set hidden") : L("Zestaw jest widoczny", "Set shown"), null, () => this.focusFirst(`button[data-set-hide="${id}"]`));
    });
    this.on("button[data-set-remove]", "click", (_e, el) => {
      const s = this.c.sets.find((x) => x.id === el.dataset.setRemove);
      if (!s || !confirm(L(`Usunąć zestaw „${biText(s.title)}”? Melodie zostają w kolekcji.`, `Remove the set “${biText(s.title)}”? Its tunes stay in the collection.`))) return;
      if (this.draft.id === s.id) this.draft = { rev: 0, tuneIds: [] };
      void this.change(el as HTMLButtonElement, () => api.removeSet(this.org, s.id, s.rev), L("Usunięto zestaw", "Set removed"));
    });

    // Lists
    this.on("button[data-rename-start]", "click", (_e, el) => {
      this.renaming = el.dataset.renameStart!;
      this.render();
      this.querySelector<HTMLInputElement>(`#rename-${this.renaming}-pl`)?.focus();
    });
    this.on("button[data-cancel-rename]", "click", () => {
      const id = this.renaming;
      this.renaming = "";
      this.render();
      this.focusFirst(`button[data-rename-start="${id}"]`);
    });
    this.on("button[data-remove]", "click", (_e, el) => {
      const kind = el.dataset.kind as TuneListKind;
      const id = el.dataset.remove!;
      if (!confirm(L("Usunąć z listy?", "Remove from the list?"))) return;
      void this.change(el as HTMLButtonElement, () => api.removeTuneListItem(this.org, kind, id), L("Usunięto", "Removed"));
    });

    // Import
    this.on("#import-file", "change", (_e, el) => void this.readImportFile(el as HTMLInputElement));
    this.on("#import-create", "change", (e) => {
      const sel = (e.target as HTMLElement).closest<HTMLSelectElement>("select[data-cf=typeId]");
      if (sel) {
        const tr = sel.closest<HTMLElement>("tr")!;
        const cand = this.candidates?.find((x) => x.index === Number(tr.dataset.cand));
        const out = tr.querySelector(".cand-meter");
        if (cand && out) out.textContent = this.candidateMeter(cand, sel.value);
      }
      this.countTicked();
    });

    this.addEventListener("submit", (e) => {
      const form = e.target as HTMLFormElement;
      e.preventDefault();
      const btn = form.querySelector<HTMLButtonElement>("button[type=submit]");
      if (form.id === "tune-filters") return;
      if (form.id === "import-form") return void this.previewImport(btn);
      if (form.id === "import-create") return void this.createImported(form);
      const val = (name: string) => form.querySelector<HTMLInputElement>(`[name='${name}']`)?.value.trim() ?? "";
      if (form.id === "set-form") {
        const name = { pl: val("name.pl"), en: val("name.en") };
        const d = this.draft;
        const save = d.id ? () => api.updateSet(this.org, d.id!, d.rev, name, d.tuneIds) : () => api.addSet(this.org, name, d.tuneIds);
        const editing = d.id;
        void this.change(btn, save, editing ? L("Zapisano zestaw", "Set saved") : L("Utworzono zestaw", "Set created"), form).then((ok) => {
          if (!ok) return;
          this.draft = { rev: 0, tuneIds: [] };
          this.render();
          if (editing) this.focusFirst(`button[data-set-edit="${editing}"]`);
          else this.focusFirst("#set-name-pl");
        });
        return;
      }
      const kind = form.dataset.kind as TuneListKind;
      const label = { pl: val("label.pl"), en: val("label.en") };
      const meter = val("meter");
      const tempo = Number(val("tempo")) || 0;
      if (form.dataset.rename) {
        const id = form.dataset.rename;
        const rev = Number(form.dataset.rev);
        const save =
          kind === "types"
            ? () => api.updateTuneType(this.org, id, rev, label, meter, tempo)
            : kind === "instruments"
              ? () => api.updateInstrument(this.org, id, rev, label)
              : () => api.renameTuneOrigin(this.org, id, rev, label);
        void this.change(btn, save, L("Zapisano", "Saved"), form, () => this.focusFirst(`button[data-rename-start="${id}"]`));
        return;
      }
      const add =
        kind === "types"
          ? () => api.addTuneType(this.org, label, meter, tempo)
          : kind === "instruments"
            ? () => api.addInstrument(this.org, label)
            : () => api.addTuneOrigin(this.org, label, form.dataset.parent ?? "");
      void this.change(btn, add, L("Dodano", "Added"), form).then((ok) => {
        if (ok) this.querySelector<HTMLInputElement>(`#${CSS.escape(form.dataset.add!)}-pl`)?.focus();
      });
    });
  }
}

customElements.define("folk-tunes", TunesView);
