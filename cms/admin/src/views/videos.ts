// The Video Collection (wideopomoc) of an Organization: the ordered Videos,
// and the Forms and Origins lists they are described with. Every change sends
// the version the screen holds (If-Match) and gets the whole collection back.

import { api, ApiError, type Country, type Labeled, type Text, type Video, type VideoCollection } from "../api.ts";
import { icon } from "../icons.ts";
import { inLang, L } from "../i18n.ts";
import { bindSortable, grip, moveButtons } from "../patterns.ts";
import { busy, html, raw, showError, t, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";

const TABS = [
  ["videos", L("Filmy", "Videos")],
  ["forms", L("Formy", "Forms")],
  ["origins", L("Pochodzenie", "Origins")],
] as const;
type Tab = (typeof TABS)[number][0];

/** "Used by N videos", in the UI language. */
export function usesText(n: number): string {
  if (!n) return L("nieużywana", "not used");
  const pl = n === 1 ? "filmie" : "filmach";
  return L(`w ${n} ${pl}`, `in ${n} video${n === 1 ? "" : "s"}`);
}

/** An Origin as text: "Region, Country" or "Country". */
export function originText(c: VideoCollection, id?: string): string {
  if (!id) return "";
  for (const co of c.origins) {
    if (co.id === id) return t(co.label);
    const r = co.regions.find((x) => x.id === id);
    if (r) return `${t(r.label)}, ${t(co.label)}`;
  }
  return "";
}

export const formsText = (c: VideoCollection, ids: string[]) =>
  ids
    .map((id) => c.forms.find((f) => f.id === id))
    .filter((f): f is Labeled => !!f)
    .map((f) => t(f.label))
    .join(", ");

const labelText = (l: Text) => (l.en ? `${l.pl} / ${l.en}` : l.pl);

class VideosView extends View {
  private tab: Tab = "videos";
  private c!: VideoCollection;
  /** The row being renamed ("" when none). */
  private renaming = "";

  async load(): Promise<void> {
    const v = new URLSearchParams(location.search).get("view");
    this.tab = TABS.some(([k]) => k === v) ? (v as Tab) : "videos";
    this.c = await api.videos(this.org);
    this.render();
    this.bind();
  }

  /**
   * Runs a change with the current version and redraws; a conflict reloads
   * the collection. Problems go next to the fields of `form`, or into a toast.
   */
  private async change(btn: HTMLButtonElement | null, fn: (etag: string) => Promise<VideoCollection>, done?: string, form?: HTMLFormElement | null, refocus?: () => void): Promise<boolean> {
    let ok = false;
    await busy(btn, async () => {
      try {
        this.c = await fn(this.c.etag);
        this.renaming = "";
        this.render();
        if (done) toast(done);
        ok = true;
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          this.c = await api.videos(this.org);
          this.renaming = "";
          this.render();
        }
        if (!form && err instanceof ApiError && err.problems.length) toast(err.problems.map((p) => inLang(p.message)).join("\n"), "error");
        else showError(err, form?.querySelector<HTMLElement>(".problems-slot"), form);
      }
    });
    if (ok) refocus?.();
    return ok;
  }

  /** Focuses the first of these that exists and is enabled. */
  private focusFirst(...selectors: string[]): void {
    for (const sel of selectors) {
      const el = this.querySelector<HTMLElement>(sel);
      if (el && !(el as HTMLButtonElement).disabled) {
        el.focus();
        return;
      }
    }
  }

  private videoRow(v: Video, i: number, n: number): Raw {
    const meta = [originText(this.c, v.originId), formsText(this.c, v.formIds), v.start ? L(`od ${v.start}`, `from ${v.start}`) : ""].filter(Boolean).join(" · ");
    return html`<li data-id="${v.id}" draggable="true" class="${v.hidden ? "hidden-row" : ""}">
      ${grip()}
      <img class="thumb" src="${v.thumbnailUrl}" alt="" width="120" height="68" loading="lazy">
      <div class="row-main">
        <a href="/app/orgs/${this.org}/videos/${v.id}">${t(v.title)}</a>
        ${meta ? html`<div class="muted small">${meta}</div>` : ""}
        ${v.hidden ? html`<span class="badges"><span class="badge">${L("ukryty", "hidden")}</span></span>` : ""}
      </div>
      <span class="row-actions">
        <button type="button" data-hide="${v.id}" data-hidden="${v.hidden ? "1" : ""}">${v.hidden ? L("Pokaż na stronie", "Show on the Site") : L("Ukryj", "Hide")}</button>
        <button type="button" class="link danger" data-delete="${v.id}" aria-label="${L(`Usuń: ${t(v.title)}`, `Delete: ${t(v.title)}`)}">${L("Usuń", "Delete")}</button>
        ${moveButtons("videos", v.id, i, n, t(v.title))}
      </span>
    </li>`;
  }

  private videosTab(): Raw {
    const vs = this.c.videos;
    return html`
      <p class="muted small">${L(
        "Strona pokazuje filmy w tej kolejności (ukryte pomija). Kolejność zmienisz przyciskami strzałek albo przeciągając wiersz.",
        "The Site shows the videos in this order (leaving out hidden ones). Change the order with the arrow buttons or by dragging a row.",
      )}</p>
      ${vs.length
        ? html`<ol class="sortable videos" data-list="videos" aria-label="${L("Filmy", "Videos")}">${vs.map((v, i) => this.videoRow(v, i, vs.length))}</ol>`
        : html`<div class="card empty"><p>${L("Nie ma jeszcze filmów.", "There are no videos yet.")}</p></div>`}`;
  }

  /** A label shown in a row, or the inline form renaming it. */
  private labelCell(kind: "forms/" | "origins/", item: Labeled): Raw {
    if (this.renaming !== item.id) {
      return html`<div class="row-main"><strong>${labelText(item.label)}</strong> <span class="muted small">${usesText(item.uses)}</span></div>`;
    }
    const id = `rename-${item.id}`;
    return html`<form class="inline-form label-form row-main" data-rename="${item.id}" data-kind="${kind}">
      <div class="problems-slot"></div>
      <div class="field"><label for="${id}-pl">${L("Nazwa (PL)", "Name (PL)")}</label><input id="${id}-pl" name="label.pl" value="${item.label.pl}" required maxlength="60"></div>
      <div class="field"><label for="${id}-en">${L("Nazwa (EN, opcjonalnie)", "Name (EN, optional)")}</label><input id="${id}-en" name="label.en" value="${item.label.en ?? ""}" maxlength="60"></div>
      <button type="submit" class="primary">${L("Zapisz", "Save")}</button>
      <button type="button" data-cancel-rename>${L("Anuluj", "Cancel")}</button>
    </form>`;
  }

  private itemActions(kind: "forms/" | "origins/", list: string, item: Labeled, i: number, n: number, blocked: string): Raw {
    return html`<span class="row-actions">
      ${this.renaming === item.id ? "" : html`<button type="button" data-rename-start="${item.id}">${L("Zmień nazwę", "Rename")}</button>`}
      <button type="button" class="link danger" data-remove="${item.id}" data-kind="${kind}" ${blocked ? html`disabled title="${blocked}"` : ""}
        aria-label="${L(`Usuń: ${item.label.pl}`, `Remove: ${item.label.pl}`)}">${L("Usuń", "Remove")}</button>
      ${moveButtons(list, item.id, i, n, item.label.pl)}
    </span>`;
  }

  private addForm(id: string, legend: string, parent = ""): Raw {
    return html`<form class="inline-form label-form add" data-add="${id}" data-parent="${parent}">
      <div class="problems-slot"></div>
      <div class="field"><label for="${id}-pl">${legend} (PL)</label><input id="${id}-pl" name="label.pl" required maxlength="60"></div>
      <div class="field"><label for="${id}-en">${legend} ${L("(EN, opcjonalnie)", "(EN, optional)")}</label><input id="${id}-en" name="label.en" maxlength="60"></div>
      <button type="submit" class="primary">${icon("plus")}${L("Dodaj", "Add")}</button>
    </form>`;
  }

  private formsTab(): Raw {
    const fs = this.c.forms;
    return html`
      <p class="muted small">${L(
        "Formy tańca (np. solo, para, łańcuch, koło). Film może mieć kilka form. Formy używanej przez filmy nie można usunąć.",
        "Dance forms (e.g. solo, pair, chain, circle). A video can have several. A form that videos use cannot be removed.",
      )}</p>
      <section class="card" aria-labelledby="add-form-title"><h2 id="add-form-title">${L("Nowa forma", "New form")}</h2>${this.addForm("new-form", L("Nazwa", "Name"))}</section>
      ${fs.length
        ? html`<ol class="sortable labels" data-list="forms" aria-label="${L("Formy", "Forms")}">${fs.map(
            (f, i) => html`<li data-id="${f.id}" draggable="${this.renaming ? "false" : "true"}">${grip()}${this.labelCell("forms/", f)}
              ${this.itemActions("forms/", "forms", f, i, fs.length, f.uses ? L(`Używana ${usesText(f.uses)}`, `Used ${usesText(f.uses)}`) : "")}</li>`,
          )}</ol>`
        : html`<div class="card empty"><p>${L("Nie ma jeszcze form.", "There are no forms yet.")}</p></div>`}`;
  }

  private countryRow(co: Country, i: number, n: number): Raw {
    const blocked = co.regions.length ? L("Najpierw usuń regiony", "Remove its regions first") : co.uses ? L(`Używany ${usesText(co.uses)}`, `Used ${usesText(co.uses)}`) : "";
    return html`<li data-id="${co.id}" draggable="${this.renaming ? "false" : "true"}">
      <div class="country">${grip()}${this.labelCell("origins/", co)}${this.itemActions("origins/", "origins", co, i, n, blocked)}</div>
      ${co.regions.length
        ? html`<ol class="sortable labels regions" data-list="origins" aria-label="${L(`Regiony: ${co.label.pl}`, `Regions: ${co.label.pl}`)}">${co.regions.map(
            (r, j) => html`<li data-id="${r.id}" draggable="${this.renaming ? "false" : "true"}">${grip()}${this.labelCell("origins/", r)}
              ${this.itemActions("origins/", "origins", r, j, co.regions.length, r.uses ? L(`Używany ${usesText(r.uses)}`, `Used ${usesText(r.uses)}`) : "")}</li>`,
          )}</ol>`
        : ""}
      <details class="add-region"><summary class="button">${icon("plus")}${L("Dodaj region", "Add a region")}</summary>
        ${this.addForm(`new-region-${co.id}`, L("Region", "Region"), co.id)}</details>
    </li>`;
  }

  private originsTab(): Raw {
    const os = this.c.origins;
    return html`
      <p class="muted small">${L(
        "Kraje i (opcjonalnie) ich regiony. Film ma najwyżej jedno pochodzenie: kraj albo region. Na stronie filtr kraju obejmuje jego regiony.",
        "Countries and (optionally) their regions. A video has at most one origin: a country or a region. On the Site, filtering by a country includes its regions.",
      )}</p>
      <section class="card" aria-labelledby="add-country-title"><h2 id="add-country-title">${L("Nowy kraj", "New country")}</h2>${this.addForm("new-country", L("Kraj", "Country"))}</section>
      ${os.length
        ? html`<ol class="sortable labels countries" data-list="origins" aria-label="${L("Kraje", "Countries")}">${os.map((co, i) => this.countryRow(co, i, os.length))}</ol>`
        : html`<div class="card empty"><p>${L("Nie ma jeszcze krajów.", "There are no countries yet.")}</p></div>`}`;
  }

  private render(): void {
    const body = this.tab === "forms" ? this.formsTab() : this.tab === "origins" ? this.originsTab() : this.videosTab();
    this.set(html`
      <div class="toolbar">
        <h1>${L("Wideopomoc", "Video help")}</h1>
        <a class="button" href="/app/orgs/${this.org}/videos/new">${icon("plus")}${L("Dodaj film", "Add video")}</a>
      </div>
      <nav class="tabs" aria-label="${L("Widok", "View")}">
        ${TABS.map(([k, label]) => html`<a href="/app/orgs/${this.org}/videos?view=${k}" class="${k === this.tab ? "active" : ""}" ${k === this.tab ? raw('aria-current="page"') : ""}>${label}</a>`)}
      </nav>
      ${body}`);
  }

  private bind(): void {
    bindSortable(this, (list, id, to, button) => {
      const kind = list === "forms" ? "forms/" : list === "origins" ? "origins/" : "";
      const sel = (dir: string) => `button[data-move="${id}"][data-list="${list}"][data-dir="${dir}"]`;
      const dir = button?.dataset.dir ?? "";
      void this.change(button ?? null, (etag) => api.moveItem(this.org, kind, id, etag, to), undefined, null,
        () => button && this.focusFirst(sel(dir), sel(dir === "up" ? "down" : "up")));
    });
    this.on("button[data-hide]", "click", (_e, el) => {
      const hide = !el.dataset.hidden;
      const id = el.dataset.hide!;
      void this.change(el as HTMLButtonElement, (etag) => api.videoAction(this.org, id, etag, hide ? "hide" : "show"),
        hide ? L("Ukryto – strona zaktualizuje się w ciągu minuty", "Hidden – the Site updates within a minute") : L("Pokazano na stronie", "Shown on the Site"),
        null, () => this.focusFirst(`button[data-hide="${id}"]`));
    });
    this.on("button[data-delete]", "click", (_e, el) => {
      const v = this.c.videos.find((x) => x.id === el.dataset.delete);
      if (!v || !confirm(L(`Usunąć film „${t(v.title)}”? Tego nie można cofnąć.`, `Delete the video “${t(v.title)}”? This cannot be undone.`))) return;
      void this.change(el as HTMLButtonElement, (etag) => api.deleteVideo(this.org, v.id, etag), L("Usunięto film", "Video deleted"));
    });
    this.on("button[data-rename-start]", "click", (_e, el) => {
      this.renaming = el.dataset.renameStart!;
      this.render();
      this.querySelector<HTMLInputElement>(`#rename-${this.renaming}-pl`)?.focus();
    });
    this.on("button[data-cancel-rename]", "click", () => {
      const id = this.renaming;
      this.renaming = "";
      this.render();
      this.querySelector<HTMLElement>(`button[data-rename-start="${id}"]`)?.focus();
    });
    this.on("button[data-remove]", "click", (_e, el) => {
      const kind = el.dataset.kind as "forms/" | "origins/";
      const id = el.dataset.remove!;
      if (!confirm(L("Usunąć z listy?", "Remove from the list?"))) return;
      void this.change(el as HTMLButtonElement, (etag) => api.removeItem(this.org, kind, id, etag), L("Usunięto", "Removed"));
    });
    this.addEventListener("submit", (e) => {
      const form = e.target as HTMLFormElement;
      e.preventDefault();
      const label = { pl: form.querySelector<HTMLInputElement>("[name='label.pl']")!.value.trim(), en: form.querySelector<HTMLInputElement>("[name='label.en']")!.value.trim() };
      const btn = form.querySelector<HTMLButtonElement>("button[type=submit]");
      if (form.dataset.rename) {
        const id = form.dataset.rename;
        void this.change(btn, (etag) => api.renameItem(this.org, form.dataset.kind as "forms/" | "origins/", id, etag, label), L("Zapisano", "Saved"), form,
          () => this.focusFirst(`button[data-rename-start="${id}"]`));
        return;
      }
      const parent = form.dataset.parent ?? "";
      const add = this.tab === "forms" ? (etag: string) => api.addForm(this.org, etag, label) : (etag: string) => api.addOrigin(this.org, etag, label, parent);
      void this.change(btn, add, L("Dodano", "Added"), form).then((ok) => {
        if (ok) this.querySelector<HTMLInputElement>(`#${CSS.escape(form.dataset.add!)}-pl`)?.focus();
      });
    });
  }
}

customElements.define("folk-videos", VideosView);
