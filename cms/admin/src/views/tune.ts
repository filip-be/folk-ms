// One Tune of the Tune Collection: add or edit. The ABC Transcription is
// drawn as it is typed (abcjs, ADR 0013) and can be played on the piano; the
// CMS reads it for the Key and Time Signature it proposes and for the
// Incipit. Attachments are added once the Tune exists. A save names the
// Tune's revision: if someone else saved the Tune meanwhile, the Moderator's
// entries stay in the form and the screen offers the current version.

import { api, ApiError, type AbcProposal, type Recording, type TuneAttachment, type TuneCollection, type TuneInput, type TuneName, type TuneView } from "../api.ts";
import { icon } from "../icons.ts";
import { inLang, L, lang } from "../i18n.ts";
import { bindSortable, grip, moveButtons, switchControl } from "../patterns.ts";
import { busy, clearFieldError, html, markProblems, raw, setFieldError, showError, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";
import { enLabel, plLabel, textField } from "./eventParts.ts";
import { biText, keyText, modeLabel, nameText, playButton, Sheet, sizeText, stopPlayback, tempoBeat, TONICS, tonicLabel } from "./tuneParts.ts";

const PROVIDER = { youtube: "YouTube", vimeo: "Vimeo" } as const;
const MAX_OTHER_NAMES = 10;
const MAX_RECORDINGS = 10;
const MAX_ATTACHMENTS = 10;
const MAX_INCIPIT_BARS = 8;
const DEFAULT_INCIPIT_BARS = 2;
/** Image types the CMS re-encodes, and PDF. */
const ATTACHMENT_TYPES = ["image/jpeg", "image/png", "image/webp", "application/pdf"];
/** How long typing pauses before the sheet is redrawn. */
const PREVIEW_DELAY = 400;

/** A Recording row of the form; key identifies it across reorders. */
interface RecordingRow {
  key: string;
  link: string;
  label: string;
  preview?: Recording;
  previewFor: string;
}

let rowSeq = 0;
const recordingRow = (r?: Recording): RecordingRow => ({ key: `r${++rowSeq}`, link: r?.link ?? "", label: r?.label ?? "", preview: r, previewFor: r?.link ?? "" });

class TuneEditor extends View {
  private c!: TuneCollection;
  private v?: TuneView;
  /** The revision the form was loaded at; a save sends it. */
  private rev = 0;
  private otherNames: TuneName[] = [];
  private recordings: RecordingRow[] = [];
  /** The Tune Type picked last, to tell whether the Time Signature still is its default. */
  private lastType = "";
  private proposal?: AbcProposal;
  private sheet?: Sheet;
  private incipit?: Sheet;
  private timer = 0;
  private seq = 0;
  private dirty = false;

  async load(): Promise<void> {
    const id = this.params.id;
    const [c, v] = await Promise.all([
      api.tunes(this.org),
      id ? api.tune(this.org, id).catch((err) => (err instanceof ApiError && err.status === 404 ? undefined : Promise.reject(err))) : undefined,
    ]);
    this.c = c;
    if (id && !v) {
      this.set(html`<div class="card"><h1>${L("Nie znaleziono", "Not found")}</h1><p><a href="/app/orgs/${this.org}/tunes">${L("Wróć do listy melodii", "Back to the tune list")}</a></p></div>`);
      return;
    }
    this.show(v);
    this.bind();
  }

  /** Fills the form from a Tune (or empty for a new one). */
  private show(v?: TuneView): void {
    this.v = v;
    this.rev = v?.rev ?? 0;
    this.otherNames = (v?.otherNames ?? []).map((n) => ({ name: n.name, translation: { ...n.translation } }));
    this.recordings = (v?.recordings ?? []).map(recordingRow);
    this.lastType = v?.typeId ?? "";
    this.proposal = v?.abcProposal;
    this.dirty = false;
    this.render();
    this.sheet = new Sheet(this.$("#abc-sheet"), this.querySelector<HTMLButtonElement>("#abc-play"));
    this.incipit = new Sheet(this.$("#incipit-sheet"), this.querySelector<HTMLButtonElement>("#incipit-play"));
    this.syncInstruments();
    this.updateNotice();
    void this.refreshAbc(true);
  }

  // ── Parts of the form ─────────────────────────────────────────────────

  private otherNameRows(): Raw {
    const ns = this.otherNames;
    return html`${ns.length
      ? html`<ol class="other-names">${ns.map(
          (n, i) => html`<li>
            ${textField({ id: `on-${i}-name`, label: L("Nazwa", "Name"), value: n.name, path: `otherNames.${i}.name`, max: 200, required: true })}
            ${textField({ id: `on-${i}-pl`, label: plLabel(L("Tłumaczenie", "Translation")), value: n.translation?.pl, path: `otherNames.${i}.translation.pl`, max: 200 })}
            ${textField({ id: `on-${i}-en`, label: enLabel(L("Tłumaczenie", "Translation")), value: n.translation?.en, path: `otherNames.${i}.translation.en`, max: 200 })}
            <button type="button" class="link danger" data-other-remove="${i}" aria-label="${L(`Usuń inną nazwę ${i + 1}`, `Remove other name ${i + 1}`)}">${L("Usuń", "Remove")}</button>
          </li>`,
        )}</ol>`
      : html`<p class="muted small">${L("Brak innych nazw.", "No other names.")}</p>`}
      <button type="button" id="add-other" ${ns.length >= MAX_OTHER_NAMES ? "disabled" : ""}>${icon("plus")}${L("Dodaj inną nazwę", "Add other name")}</button>`;
  }

  private recordingPreview(r: RecordingRow): Raw {
    const p = r.preview;
    if (!p) return html`<span class="muted small">${L("YouTube i Vimeo grają na stronie; inny link (https://) strona pokaże jako odnośnik.", "YouTube and Vimeo play on the Site; another link (https://) is shown as a link.")}</span>`;
    if (p.kind === "video")
      return html`<figure class="video-preview small-preview">${p.thumbnailUrl ? html`<img src="${p.thumbnailUrl}" alt="" width="160" height="90">` : ""}
        <figcaption>${p.provider ? PROVIDER[p.provider] : ""}${p.start ? html` · ${L("od", "from")} ${p.start}` : ""} · <a href="${p.url}" target="_blank" rel="noopener">${L("otwórz", "open")}</a></figcaption></figure>`;
    return html`<span class="small">${icon("link")} ${p.host} · <a href="${p.url}" target="_blank" rel="noopener">${L("otwórz", "open")}</a></span>`;
  }

  private recordingRows(): Raw {
    const rs = this.recordings;
    return html`${rs.length
      ? html`<ol class="sortable recordings" data-list="recordings" aria-label="${L("Nagrania", "Recordings")}">${rs.map((r, i) => {
          const name = r.label || r.link || L(`Nagranie ${i + 1}`, `Recording ${i + 1}`);
          return html`<li data-id="${r.key}" draggable="true">${grip()}
            <div class="row-main recording">
              <div class="pair">
                <div class="field"><label for="rec-${r.key}-link">${L("Link", "Link")}</label>
                  <input id="rec-${r.key}-link" name="recordings.${i}.link" data-field="recordings.${i}.link recordings.${i}.start" value="${r.link}" required placeholder="https://"></div>
                ${textField({ id: `rec-${r.key}-label`, label: html`${L("Opis", "Label")} <span class="muted">${L("(opcjonalnie, np. kto gra)", "(optional, e.g. who plays)")}</span>`, value: r.label, path: `recordings.${i}.label`, max: 200 })}
              </div>
              <div class="rec-preview" id="rec-${r.key}-preview" aria-live="polite">${this.recordingPreview(r)}</div>
            </div>
            <span class="row-actions">
              <button type="button" class="link danger" data-rec-remove="${r.key}" aria-label="${L(`Usuń nagranie: ${name}`, `Remove the recording: ${name}`)}">${L("Usuń", "Remove")}</button>
              ${moveButtons("recordings", r.key, i, rs.length, name)}
            </span>
          </li>`;
        })}</ol>`
      : html`<p class="muted small">${L("Brak nagrań.", "No recordings.")}</p>`}
      <button type="button" id="add-recording" ${rs.length >= MAX_RECORDINGS ? "disabled" : ""}>${icon("plus")}${L("Dodaj nagranie", "Add recording")}</button>`;
  }

  private instrumentGroup(field: "suits" | "tricky", legend: string, hint: string): Raw {
    const chosen = this.v?.[field] ?? [];
    return html`<fieldset data-field="${field}" aria-describedby="${field}-hint"><legend>${legend}</legend>
      <p class="muted small" id="${field}-hint">${hint}</p>
      ${this.c.instruments.length
        ? this.c.instruments.map((ins) => html`<label class="chip"><input type="checkbox" name="${field}" value="${ins.id}" ${chosen.includes(ins.id) ? "checked" : ""}> ${biText(ins.label)}</label>`)
        : html`<p class="muted small">${L("Lista jest pusta. ", "The list is empty. ")}<a href="/app/orgs/${this.org}/tunes?view=lists">${L("Dodaj instrumenty", "Add instruments")}</a></p>`}
    </fieldset>`;
  }

  private originSelect(): Raw {
    const sel = this.v?.originId ?? "";
    const opt = (id: string, label: string) => html`<option value="${id}" ${sel === id ? "selected" : ""}>${label}</option>`;
    return html`<div class="field"><label for="t-origin">${L("Pochodzenie", "Origin")}</label>
      <select id="t-origin" name="originId">
        <option value="">${L("— brak —", "— none —")}</option>
        ${this.c.origins.map((co) =>
          co.regions.length
            ? html`<optgroup label="${biText(co.label)}">${opt(co.id, L(`${biText(co.label)} (cały kraj)`, `${biText(co.label)} (whole country)`))}${co.regions.map((r) => opt(r.id, `${biText(r.label)}, ${biText(co.label)}`))}</optgroup>`
            : opt(co.id, biText(co.label)),
        )}
      </select></div>`;
  }

  private keyFields(): Raw {
    const k = this.v?.key ?? { tonic: "", mode: "" };
    const tonics = k.tonic && !TONICS.includes(k.tonic) ? [...TONICS, k.tonic] : TONICS;
    return html`<fieldset data-field="key" class="key-fields"><legend class="required">${L("Tonacja", "Key")}</legend>
      <div class="field"><label for="t-tonic">${L("Dźwięk", "Tonic")}</label>
        <select id="t-tonic" required><option value="">—</option>${tonics.map((x) => html`<option value="${x}" ${x === k.tonic ? "selected" : ""}>${tonicLabel(x)}</option>`)}</select></div>
      <div class="field"><label for="t-mode">${L("Skala", "Mode")}</label>
        <select id="t-mode" required><option value="">—</option>${this.c.modes.map((m) => html`<option value="${m}" ${m === k.mode ? "selected" : ""}>${modeLabel(m)}</option>`)}</select></div>
    </fieldset>`;
  }

  private tempoHint(typeId: string): string {
    const ty = this.c.types.find((x) => x.id === typeId);
    return ty
      ? L(`Uderzenia na minutę. Puste: tempo rodzaju melodii (${ty.tempo}).`, `Beats a minute. Empty: the Tune Type's tempo (${ty.tempo}).`)
      : L("Uderzenia na minutę. Puste: tempo rodzaju melodii.", "Beats a minute. Empty: the Tune Type's tempo.");
  }

  private siteActions(): Raw {
    const v = this.v;
    if (!v) return raw("");
    return html`${switchControl("t-hidden", L("Ukryta na stronie", "Hidden from the Site"), v.hidden)}
      ${v.permalink && !v.hidden ? html`<a class="button" href="${v.permalink[lang]}" target="_blank" rel="noopener">${icon("external")}${L("Zobacz na stronie", "View on Site")}</a>` : ""}`;
  }

  private setsNote(): Raw {
    const sets = this.v?.sets ?? [];
    if (!sets.length) return raw("");
    return html`<p class="muted small">${L("W zestawach: ", "In sets: ")}${sets.map((s, i) => html`${i ? ", " : ""}${biText(s.title)}${s.hidden ? L(" (ukryty)", " (hidden)") : ""}`)}</p>`;
  }

  private render(): void {
    const v = this.v;
    const typeId = v?.typeId ?? "";
    const ty = this.c.types.find((x) => x.id === typeId);
    this.set(html`
      <form id="tune" class="editor">
        <a href="/app/orgs/${this.org}/tunes" class="muted small">← ${L("Melodie", "Tunes")}</a>
        <div class="toolbar sticky">
          <div><h1>${v ? nameText(v.name) : L("Nowa melodia", "New tune")}</h1>
            <span class="badges" id="t-badges">${v?.hidden ? html`<span class="badge">${L("ukryta", "hidden")}</span>` : ""}</span></div>
          <div class="actions">
            <button type="submit" class="primary" id="save">${v ? L("Zapisz", "Save") : L("Dodaj melodię", "Add tune")}</button>
            <span class="actions" id="site-actions">${this.siteActions()}</span>
            ${v ? html`<button type="button" class="danger" id="remove">${L("Usuń melodię", "Remove tune")}</button>` : ""}
          </div>
        </div>
        <div id="conflict"></div>
        <div id="problems"></div>
        <p class="required-note">${L("wymagane", "required")}</p>
        <div id="sets-note">${this.setsNote()}</div>

        <section class="card" aria-labelledby="t-name-title">
          <h2 id="t-name-title">${L("Nazwa", "Name")}</h2>
          ${textField({ id: "t-name", label: L("Nazwa (tak, jak znają ją muzycy)", "Name (as musicians know it)"), value: v?.name.name, path: "name.name", required: true, max: 200 })}
          <div class="pair">
            ${textField({ id: "t-name-pl", label: plLabel(L("Tłumaczenie", "Translation")), value: v?.name.translation?.pl, path: "name.translation.pl", max: 200 })}
            ${textField({ id: "t-name-en", label: enLabel(L("Tłumaczenie", "Translation")), value: v?.name.translation?.en, path: "name.translation.en", max: 200 })}
          </div>
          <fieldset data-field="otherNames"><legend>${L("Inne nazwy", "Other names")} <span class="muted">${L("(np. irlandzka, regionalna)", "(e.g. Irish, regional)")}</span></legend>
            <div id="other-names">${this.otherNameRows()}</div>
          </fieldset>
        </section>

        <section class="card" aria-labelledby="t-music-title">
          <h2 id="t-music-title">${L("Melodia", "Tune")}</h2>
          <div class="grid">
            <div class="field"><label for="t-type">${L("Rodzaj melodii", "Tune Type")}</label>
              <select id="t-type" name="typeId" required aria-describedby="t-type-hint">
                <option value="">${L("— wybierz —", "— choose —")}</option>
                ${this.c.types.map((x) => html`<option value="${x.id}" ${x.id === typeId ? "selected" : ""}>${biText(x.label)} (${x.meter})</option>`)}
              </select>
              <p class="muted small" id="t-type-hint" aria-live="polite">${L("Rodzaj proponuje metrum i tempo.", "The type proposes a time signature and tempo.")}</p></div>
            <div class="field"><label for="t-meter">${L("Metrum", "Time Signature")}</label>
              <input id="t-meter" name="meter" value="${v?.meter ?? ""}" required placeholder="${ty?.meter ?? "4/4"}" maxlength="7">
              <div id="meter-propose"></div></div>
            <div class="field"><label for="t-tempo">${L("Tempo odtwarzania", "Playback Tempo")} <span class="muted">${L("(opcjonalnie)", "(optional)")}</span></label>
              <input id="t-tempo" name="tempo" type="number" min="20" max="400" inputmode="numeric" value="${v?.tempo || ""}" placeholder="${ty?.tempo ?? ""}" aria-describedby="t-tempo-hint">
              <p class="muted small" id="t-tempo-hint">${this.tempoHint(typeId)}</p></div>
          </div>
          ${this.keyFields()}
          <div class="grid">
            ${this.originSelect()}
            ${textField({ id: "t-composer", label: html`${L("Kompozytor", "Composer")} <span class="muted">${L("(opcjonalnie)", "(optional)")}</span>`, value: v?.composer, path: "composer", max: 200, hint: L("„trad.”, gdy melodia jest tradycyjna.", "“trad.” when the tune is traditional.") })}
          </div>
        </section>

        <section class="card" aria-labelledby="t-instruments-title">
          <h2 id="t-instruments-title">${L("Instrumenty", "Instruments")}</h2>
          ${this.instrumentGroup("suits", L("Pasuje do", "Suits"), L("Instrumenty, na których melodia dobrze leży.", "Instruments the tune sits well on."))}
          ${this.instrumentGroup("tricky", L("Trudna na", "Tricky on"), L("Trudniejsza do zagrania (nigdy zakazana). Instrument nie może być w obu grupach.", "Harder to play there (never forbidden). An instrument cannot be in both groups."))}
        </section>

        <section class="card" aria-labelledby="t-abc-title" id="t-abc-section">
          <h2 id="t-abc-title">${L("Zapis nutowy (ABC)", "Transcription (ABC)")}</h2>
          <div class="field"><label for="t-abc">ABC <span class="muted">${L("(opcjonalnie)", "(optional)")}</span></label>
            <textarea id="t-abc" class="abc-text" name="abc" data-field="abc" rows="12" spellcheck="false" aria-describedby="t-abc-hint">${v?.abc ?? ""}</textarea>
            <p class="muted small" id="t-abc-hint">${L(
              "Jedna melodia z nagłówkiem (X:, T:, M:, L:, K:) i nutami. Tonacja i metrum z ABC są tylko propozycją – na stronie liczą się pola powyżej.",
              "One tune with its header (X:, T:, M:, L:, K:) and notes. The Key and Time Signature in the ABC are only a proposal – the Site uses the fields above.",
            )}</p></div>
          <div id="abc-notice" aria-live="polite"></div>
          <div class="sheet-head"><h3>${L("Podgląd", "Preview")}</h3>${playButton("abc-play")}</div>
          <div class="abc-sheet" id="abc-sheet" aria-label="${L("Nuty", "Sheet music")}"></div>
          <div id="abc-status" class="muted small" aria-live="polite"></div>
          <div class="sheet-head"><h3>${L("Incipit", "Incipit")}</h3>${playButton("incipit-play")}</div>
          <div class="field narrow-field"><label for="t-incipit">${L("Liczba taktów", "Bars")}</label>
            <input id="t-incipit" name="incipitBars" type="number" min="1" max="${MAX_INCIPIT_BARS}" inputmode="numeric" value="${v?.incipitBars || DEFAULT_INCIPIT_BARS}" aria-describedby="t-incipit-hint">
            <p class="muted small" id="t-incipit-hint">${L("Początek melodii w katalogu: przedtakt i pierwsze takty (1–8).", "The opening the catalogue shows: the pickup and the first bars (1–8).")}</p></div>
          <div class="abc-sheet incipit" id="incipit-sheet" aria-label="Incipit"></div>
        </section>

        <section class="card" aria-labelledby="t-rec-title">
          <h2 id="t-rec-title">${L("Nagrania", "Recordings")}</h2>
          <fieldset data-field="recordings"><legend class="sr-only">${L("Nagrania", "Recordings")}</legend>
            <div id="recordings">${this.recordingRows()}</div>
          </fieldset>
        </section>

        <section class="card" aria-labelledby="t-desc-title">
          <h2 id="t-desc-title">${L("Opis", "Description")}</h2>
          <p class="muted small" id="t-desc-hint">${L(
            "Opcjonalnie, w dowolnym języku (strona pokaże drugi, gdy brakuje jednego). Formatowanie: **pogrubienie**, *kursywa*, [link](https://…), listy (- lub 1.), # nagłówek, > cytat.",
            "Optional, in either language (the Site shows the other where one is missing). Formatting: **bold**, *italic*, [link](https://…), lists (- or 1.), # heading, > quote.",
          )}</p>
          <div class="pair" data-field="description">
            ${(["pl", "en"] as const).map(
              (l) => html`<div>
                ${textField({ id: `t-desc-${l}`, label: html`${L("Opis", "Description")} <span class="muted">(${l.toUpperCase()})</span>`, value: v?.description[l], path: `description.${l}`, kind: "long", max: 10000 })}
                <button type="button" data-md-preview="${l}">${L("Podgląd", "Preview")}</button>
                <div class="md-preview" id="md-${l}" aria-live="polite" hidden></div>
              </div>`,
            )}
          </div>
        </section>
      </form>
      <div id="attachments">${this.attachmentsBlock()}</div>`);
  }

  // ── Attachments ───────────────────────────────────────────────────────

  private attachmentRow(a: TuneAttachment, i: number, n: number): Raw {
    const thumb = a.variants?.[0];
    return html`<li data-id="${a.id}" draggable="true">${grip()}
      ${a.kind === "image" && thumb
        ? html`<img class="thumb" src="${thumb.url}" alt="" width="${thumb.width}" height="${thumb.height}" loading="lazy">`
        : html`<span class="thumb file-thumb" aria-hidden="true">${icon("file")}<span>PDF</span></span>`}
      <div class="row-main">
        <a href="${a.url}" target="_blank" rel="noopener">${a.name}</a> <span class="muted small">${a.kind === "pdf" ? "PDF" : `${a.width}×${a.height}`} · ${sizeText(a.size)}</span>
        <div class="pair caption-pair">
          <div class="field"><label for="att-${a.id}-pl">${L("Podpis (PL)", "Caption (PL)")}</label><input id="att-${a.id}-pl" name="caption.pl" value="${a.caption.pl ?? ""}" maxlength="300"></div>
          <div class="field"><label for="att-${a.id}-en">${L("Podpis (EN)", "Caption (EN)")}</label><input id="att-${a.id}-en" name="caption.en" value="${a.caption.en ?? ""}" maxlength="300"></div>
        </div>
      </div>
      <span class="row-actions">
        <button type="button" data-att-caption="${a.id}">${L("Zapisz podpis", "Save caption")}</button>
        <button type="button" class="link danger" data-att-remove="${a.id}" aria-label="${L(`Usuń załącznik: ${a.name}`, `Remove the attachment: ${a.name}`)}">${L("Usuń", "Remove")}</button>
        ${moveButtons("attachments", a.id, i, n, a.name)}
      </span>
    </li>`;
  }

  private attachmentsBlock(): Raw {
    const v = this.v;
    if (!v)
      return html`<section class="card" aria-labelledby="t-att-title"><h2 id="t-att-title">${L("Załączniki", "Attachments")}</h2>
        <p class="muted">${L("Obrazy i pliki PDF dodasz po zapisaniu melodii.", "Add images and PDF files once the tune is saved.")}</p></section>`;
    const as = v.attachments;
    const full = as.length >= MAX_ATTACHMENTS;
    return html`<section class="card attachments-card" aria-labelledby="t-att-title" id="att-section">
      <h2 id="t-att-title">${L("Załączniki", "Attachments")}</h2>
      <p class="muted small">${L(
        "Obrazy (JPEG, PNG, WebP; dla obrazu podpis to też tekst alternatywny) i pliki PDF (do 15 MB), najwyżej 10. Zmiany załączników zapisują się od razu.",
        "Images (JPEG, PNG, WebP; for an image the caption is also its alt text) and PDF files (up to 15 MB), at most 10. Attachment changes are saved at once.",
      )}</p>
      ${as.length ? html`<ol class="sortable attachments" data-list="attachments" aria-label="${L("Załączniki", "Attachments")}">${as.map((a, i) => this.attachmentRow(a, i, as.length))}</ol>` : ""}
      <label class="button" for="att-file">${icon("plus")}${L("Dodaj plik…", "Add a file…")}<input type="file" id="att-file" name="file" data-field="file attachments" accept="${ATTACHMENT_TYPES.join(",")}" hidden ${full ? "disabled" : ""}></label>
      ${full ? html`<p class="muted small">${L("Melodia ma już 10 załączników.", "The tune already has 10 attachments.")}</p>` : ""}
    </section>`;
  }

  private redrawAttachments(v: TuneView, focus?: string): void {
    if (this.v) {
      this.v.attachments = v.attachments;
    }
    this.$("#attachments").innerHTML = this.attachmentsBlock().value;
    if (focus) this.querySelector<HTMLElement>(focus)?.focus();
  }

  private async attachmentChange(btn: HTMLButtonElement | null, fn: () => Promise<TuneView>, done: string, root?: HTMLElement | null, focus?: string): Promise<void> {
    await busy(btn, async () => {
      try {
        const v = await fn();
        this.redrawAttachments(v, focus);
        toast(done);
      } catch (err) {
        showError(err, null, root ?? this.querySelector("#att-section"));
      }
    });
  }

  private async upload(input: HTMLInputElement): Promise<void> {
    const file = input.files?.[0];
    input.value = "";
    if (!file || !this.v) return;
    if (!ATTACHMENT_TYPES.includes(file.type)) {
      // After this change event reaches the document, whose listener clears the field's error.
      await new Promise((r) => setTimeout(r));
      setFieldError(input, L("dodaj obraz (JPEG, PNG, WebP) albo plik PDF; HEIC (iPhone) zapisz najpierw jako JPEG", "add an image (JPEG, PNG, WebP) or a PDF file; save HEIC (iPhone) as JPEG first"));
      return;
    }
    const section = this.querySelector<HTMLElement>("#att-section");
    section?.classList.add("uploading");
    try {
      const v = await api.uploadAttachment(this.org, this.v.id, file);
      this.redrawAttachments(v, "#att-file");
      toast(L("Dodano załącznik", "Attachment added"));
    } catch (err) {
      showError(err, null, section);
    } finally {
      section?.classList.remove("uploading");
    }
  }

  // ── Reading the form ──────────────────────────────────────────────────

  private val(sel: string): string {
    return (this.querySelector<HTMLInputElement>(sel)?.value ?? "").trim();
  }

  private readOtherNames(): void {
    this.otherNames = this.otherNames.map((_, i) => ({ name: this.val(`#on-${i}-name`), translation: { pl: this.val(`#on-${i}-pl`), en: this.val(`#on-${i}-en`) } }));
  }

  private readRecordings(): void {
    for (const r of this.recordings) {
      r.link = this.val(`#rec-${r.key}-link`);
      r.label = this.val(`#rec-${r.key}-label`);
    }
  }

  private collect(): TuneInput {
    this.readOtherNames();
    this.readRecordings();
    const checked = (name: string) => [...this.querySelectorAll<HTMLInputElement>(`#tune input[name=${name}]:checked`)].map((i) => i.value);
    return {
      name: { name: this.val("#t-name"), translation: { pl: this.val("#t-name-pl"), en: this.val("#t-name-en") } },
      otherNames: this.otherNames,
      typeId: this.val("#t-type"),
      key: { tonic: this.val("#t-tonic"), mode: this.val("#t-mode") },
      meter: this.val("#t-meter"),
      tempo: Number(this.val("#t-tempo")) || 0,
      originId: this.val("#t-origin"),
      composer: this.val("#t-composer"),
      suits: checked("suits"),
      tricky: checked("tricky"),
      abc: this.querySelector<HTMLTextAreaElement>("#t-abc")?.value ?? "",
      incipitBars: Number(this.val("#t-incipit")) || DEFAULT_INCIPIT_BARS,
      recordings: this.recordings.map((r) => ({ link: r.link, label: r.label })),
      description: { pl: this.querySelector<HTMLTextAreaElement>("#t-desc-pl")?.value ?? "", en: this.querySelector<HTMLTextAreaElement>("#t-desc-en")?.value ?? "" },
    };
  }

  // ── Behaviour ─────────────────────────────────────────────────────────

  /** An instrument cannot both suit a Tune and be tricky on it: its box in the other group is disabled. */
  private syncInstruments(): void {
    for (const [a, b] of [["suits", "tricky"], ["tricky", "suits"]] as const) {
      this.querySelectorAll<HTMLInputElement>(`#tune input[name=${b}]`).forEach((box) => {
        const other = this.querySelector<HTMLInputElement>(`#tune input[name=${a}][value="${CSS.escape(box.value)}"]`);
        box.disabled = !!other?.checked;
        box.closest("label")!.title = box.disabled ? (b === "tricky" ? L("Zaznaczone jako „pasuje do”", "Ticked as “suits”") : L("Zaznaczone jako „trudna na”", "Ticked as “tricky on”")) : "";
      });
    }
  }

  /** Picking a Tune Type proposes its Time Signature (filled in while the field is empty or still the previous type's). */
  private typeChanged(): void {
    const ty = this.c.types.find((x) => x.id === this.val("#t-type"));
    const prev = this.c.types.find((x) => x.id === this.lastType);
    this.lastType = ty?.id ?? "";
    const meter = this.$<HTMLInputElement>("#t-meter");
    const tempo = this.$<HTMLInputElement>("#t-tempo");
    const hint = this.$("#t-type-hint");
    const propose = this.$("#meter-propose");
    propose.innerHTML = "";
    tempo.placeholder = ty ? String(ty.tempo) : "";
    this.$("#t-tempo-hint").textContent = this.tempoHint(ty?.id ?? "");
    if (!ty) return;
    meter.placeholder = ty.meter;
    if (!meter.value.trim() || meter.value.trim() === prev?.meter) {
      if (meter.value.trim() !== ty.meter) {
        meter.value = ty.meter;
        clearFieldError(meter);
        hint.textContent = L(`Metrum ustawione na ${ty.meter} (z rodzaju melodii).`, `Time Signature set to ${ty.meter} (from the Tune Type).`);
      }
    } else if (meter.value.trim() !== ty.meter) {
      propose.innerHTML = html`<button type="button" class="secondary small-button" data-use-meter="${ty.meter}">${L(`Użyj ${ty.meter} (z rodzaju)`, `Use ${ty.meter} (the type's)`)}</button>`.value;
    }
    this.updateNotice();
  }

  /** The Key and Time Signature the ABC proposes: offered when the form has none, a notice when they differ. */
  private updateNotice(): void {
    const box = this.querySelector<HTMLElement>("#abc-notice");
    if (!box) return;
    const p = this.proposal;
    const tonic = this.val("#t-tonic");
    const mode = this.val("#t-mode");
    const meter = this.val("#t-meter");
    const keyDiff = !!p?.key && (p.key.tonic !== tonic || p.key.mode !== mode);
    const meterDiff = !!p?.meter && p.meter !== meter;
    if (!keyDiff && !meterDiff) {
      box.innerHTML = "";
      return;
    }
    // Every differing field still empty: a proposal, not a difference.
    const empty = (!keyDiff || (!tonic && !mode)) && (!meterDiff || !meter);
    const parts: Raw[] = [];
    if (keyDiff) parts.push(tonic || mode ? html`${L("tonacja w formularzu", "the form's Key")} <strong>${keyText({ tonic, mode }) || "–"}</strong>, ${L("w ABC", "the ABC's")} <strong>${keyText(p!.key)}</strong>` : html`${L("tonacja", "Key")} <strong>${keyText(p!.key)}</strong>`);
    if (meterDiff) parts.push(meter ? html`${L("metrum w formularzu", "the form's Time Signature")} <strong>${meter}</strong>, ${L("w ABC", "the ABC's")} <strong>${p!.meter}</strong>` : html`${L("metrum", "Time Signature")} <strong>${p!.meter}</strong>`);
    box.innerHTML = html`<div class="banner ${empty ? "info" : "warn"} abc-notice">
      <p>${empty ? L("ABC proponuje: ", "The ABC proposes: ") : L("Różnica z zapisem ABC: ", "Differs from the ABC: ")}${parts.map((x, i) => html`${i ? "; " : ""}${x}`)}.
        ${empty ? "" : html`<span class="small">${L("Strona pokazuje wartości z formularza; możesz zapisać bez zmian.", "The Site shows the form's values; you may save as it is.")}</span>`}</p>
      <span class="actions">
        ${keyDiff ? html`<button type="button" class="secondary" data-use-key>${L(`Użyj tonacji ${keyText(p!.key)}`, `Use Key ${keyText(p!.key)}`)}</button>` : ""}
        ${meterDiff ? html`<button type="button" class="secondary" data-use-meter="${p!.meter}">${L(`Użyj metrum ${p!.meter}`, `Use Time Signature ${p!.meter}`)}</button>` : ""}
      </span>
    </div>`.value;
  }

  /** Playback follows the Tune's tempo, else its type's, counted in the beat of its Time Signature. */
  private playTempo(): { bpm: number; beat: string } {
    const ty = this.c.types.find((x) => x.id === this.val("#t-type"));
    const bpm = Number(this.val("#t-tempo")) || ty?.tempo || 120;
    return { bpm, beat: tempoBeat(this.val("#t-meter") || this.proposal?.meter || ty?.meter || "4/4") };
  }

  private scheduleAbc(): void {
    clearTimeout(this.timer);
    this.timer = window.setTimeout(() => void this.refreshAbc(), PREVIEW_DELAY);
  }

  /** Redraws the sheet, then asks the CMS for the proposal and the Incipit. */
  private async refreshAbc(first = false): Promise<void> {
    const abc = this.querySelector<HTMLTextAreaElement>("#t-abc")?.value ?? "";
    const bars = Number(this.val("#t-incipit")) || DEFAULT_INCIPIT_BARS;
    const seq = ++this.seq;
    const status = this.querySelector<HTMLElement>("#abc-status");
    const section = this.querySelector<HTMLElement>("#t-abc-section");
    if (!status || !section) return;
    const tempo = this.playTempo();
    await this.sheet?.render(abc, tempo);
    if (seq !== this.seq) return;
    const warnings = this.sheet?.warnings ?? [];
    status.textContent = warnings.length ? L(`Ostrzeżenia zapisu: ${warnings.slice(0, 3).join("; ")}`, `Notation warnings: ${warnings.slice(0, 3).join("; ")}`) : "";
    if (!abc.trim()) {
      this.proposal = undefined;
      await this.incipit?.render("");
      this.updateNotice();
      return;
    }
    if (first && this.v?.incipit !== undefined && this.v.abc === abc) {
      await this.incipit?.render(this.v.incipit ?? "", tempo);
      return;
    }
    try {
      const p = await api.abcPreview(this.org, abc, bars);
      if (seq !== this.seq) return;
      markProblems(section, []);
      this.proposal = p.proposal;
      this.updateNotice();
      if (!p.valid) status.textContent = L("Zapis potrzebuje linii K: i nut.", "The ABC needs a K: line and notes.");
      await this.incipit?.render(p.incipit ?? "", tempo);
    } catch (err) {
      if (seq !== this.seq) return;
      if (err instanceof ApiError && err.problems.length) markProblems(section, err.problems);
      else showError(err);
    }
  }

  /** Asks the CMS what a Recording's link will save as. */
  private async previewRecording(key: string): Promise<void> {
    const r = this.recordings.find((x) => x.key === key);
    const input = this.querySelector<HTMLInputElement>(`#rec-${key}-link`);
    if (!r || !input) return;
    const link = input.value.trim();
    if (link === r.previewFor) return;
    r.previewFor = link;
    const box = this.$(`#rec-${key}-preview`);
    r.preview = undefined;
    if (link) {
      try {
        r.preview = await api.recordingPreview(this.org, link);
        if (r.previewFor !== link) return;
        clearFieldError(input);
      } catch (err) {
        if (r.previewFor !== link) return;
        if (err instanceof ApiError && err.problems.length) setFieldError(input, err.problems.map((p) => inLang(p.message)).join("; "));
        else showError(err);
      }
    }
    box.innerHTML = this.recordingPreview(r).value;
  }

  private async previewMarkdown(l: "pl" | "en", btn: HTMLButtonElement): Promise<void> {
    const area = this.$<HTMLTextAreaElement>(`#t-desc-${l}`);
    const out = this.$(`#md-${l}`);
    await busy(btn, async () => {
      try {
        const r = await api.markdownPreview(this.org, area.value);
        // The CMS's renderer escapes every character and emits only its own tags (decision 12).
        out.innerHTML = r.html || html`<p class="muted">${L("(pusto)", "(empty)")}</p>`.value;
        out.hidden = false;
      } catch (err) {
        if (err instanceof ApiError && err.problems.length) setFieldError(area, err.problems.map((p) => inLang(p.message)).join("; "));
        else showError(err);
      }
    });
  }

  private conflictBanner(): void {
    this.$("#conflict").innerHTML = html`<div class="banner danger" role="alert">
      <p><strong>${L("Ktoś inny zmienił tę melodię", "Someone else changed this tune")}</strong><br>
        ${L(
          "Od otwarcia formularza ktoś zapisał tę melodię, więc Twoje zmiany nie zostały zapisane. Twoje wpisy są nadal w formularzu: skopiuj, co potrzebujesz, a potem wczytaj aktualną wersję i wprowadź zmiany ponownie.",
          "Someone saved this tune after you opened it, so your changes are not saved. Your entries are still in the form: copy what you need, then load the current version and make your changes again.",
        )}</p>
      <button type="button" id="reload">${L("Wczytaj aktualną wersję", "Load the current version")}</button>
    </div>`.value;
    this.$("#conflict").scrollIntoView({ block: "nearest", behavior: "smooth" });
  }

  private async save(btn: HTMLButtonElement | null): Promise<void> {
    const form = this.$<HTMLFormElement>("#tune");
    const input = this.collect();
    const problems = this.querySelector<HTMLElement>("#problems");
    await busy(btn, async () => {
      try {
        if (!this.v) {
          const v = await api.createTune(this.org, input);
          this.dirty = false;
          toast(L("Dodano melodię – strona zaktualizuje się w ciągu minuty", "Tune added – the Site updates within a minute"));
          // Replace /new, so Back does not return to an empty form; the new screen allows Attachments.
          history.replaceState(null, "", `/app/orgs/${this.org}/tunes/${v.id}`);
          dispatchEvent(new PopStateEvent("popstate"));
          return;
        }
        const v = await api.updateTune(this.org, this.v.id, this.rev, input);
        toast(L("Zapisano – strona zaktualizuje się w ciągu minuty", "Saved – the Site updates within a minute"));
        this.show(v);
        this.$<HTMLButtonElement>("#save").focus();
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          this.conflictBanner();
          return;
        }
        showError(err, problems, form);
      }
    });
  }

  private async reload(): Promise<void> {
    if (this.dirty && !confirm(L("Wczytać aktualną wersję? Twoje niezapisane zmiany przepadną.", "Load the current version? Your unsaved changes will be lost."))) return;
    try {
      this.show(await api.tune(this.org, this.v!.id));
      toast(L("Wczytano aktualną wersję", "The current version is loaded"));
    } catch (err) {
      showError(err);
    }
  }

  private async setHidden(box: HTMLInputElement): Promise<void> {
    const v = this.v!;
    const hide = box.checked;
    if (hide && v.sets.length && !confirm(L(
      `Melodia zostaje w zestawach (${v.sets.map((s) => biText(s.title)).join(", ")}), które strona pokaże bez niej. Zestaw z mniej niż 2 widocznymi melodiami nie jest publikowany. Ukryć?`,
      `The tune stays in its sets (${v.sets.map((s) => biText(s.title)).join(", ")}), which the Site shows without it. A set with fewer than 2 visible tunes is not published. Hide it?`,
    ))) {
      box.checked = false;
      return;
    }
    box.disabled = true;
    try {
      const r = await api.tuneAction(this.org, v.id, hide ? "hide" : "show");
      v.hidden = r.hidden;
      v.permalink = r.permalink;
      v.sets = r.sets;
      this.$("#site-actions").innerHTML = this.siteActions().value;
      this.$("#t-badges").innerHTML = v.hidden ? html`<span class="badge">${L("ukryta", "hidden")}</span>`.value : "";
      this.$<HTMLInputElement>("#t-hidden").focus();
      toast(hide ? L("Ukryto – strona zaktualizuje się w ciągu minuty", "Hidden – the Site updates within a minute") : L("Pokazano na stronie", "Shown on the Site"));
    } catch (err) {
      box.checked = !hide;
      box.disabled = false;
      showError(err);
    }
  }

  private async removeTune(btn: HTMLButtonElement): Promise<void> {
    const v = this.v!;
    const name = nameText(v.name);
    const sets = v.sets;
    const emptied = sets.filter((s) => s.emptiedOnRemove);
    const msg = sets.length
      ? L(
          `Usunąć melodię „${name}”? Jest w zestawach: ${sets.map((s) => `„${biText(s.title)}”`).join(", ")} – zostanie z nich wyjęta.${emptied.length ? ` Zestawy, w których jest jedyną melodią, zostaną usunięte: ${emptied.map((s) => `„${biText(s.title)}”`).join(", ")}.` : ""} Tego nie można cofnąć.`,
          `Remove the tune “${name}”? It is in the sets ${sets.map((s) => `“${biText(s.title)}”`).join(", ")} and will be taken out of them.${emptied.length ? ` Sets where it is the only tune will be removed: ${emptied.map((s) => `“${biText(s.title)}”`).join(", ")}.` : ""} This cannot be undone.`,
        )
      : L(`Usunąć melodię „${name}” i jej załączniki? Tego nie można cofnąć.`, `Remove the tune “${name}” and its attachments? This cannot be undone.`);
    if (!confirm(msg)) return;
    await busy(btn, async () => {
      try {
        await api.removeTune(this.org, v.id, this.rev, sets.length > 0);
        this.dirty = false;
        toast(L("Usunięto melodię", "Tune removed"));
        history.replaceState(null, "", `/app/orgs/${this.org}/tunes`);
        dispatchEvent(new PopStateEvent("popstate"));
      } catch (err) {
        if (err instanceof ApiError && err.code === "needs_confirmation") {
          // Added to a Set meanwhile: show its Sets and ask again.
          const cur = await api.tune(this.org, v.id);
          v.sets = cur.sets;
          this.$("#sets-note").innerHTML = this.setsNote().value;
          toast(L("Melodia trafiła w międzyczasie do zestawu. Sprawdź listę zestawów i spróbuj ponownie.", "The tune was added to a set meanwhile. Check its sets and try again."), "error");
          return;
        }
        if (err instanceof ApiError && err.code === "conflict") {
          this.conflictBanner();
          return;
        }
        showError(err);
      }
    });
  }

  private bind(): void {
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void this.save(this.querySelector<HTMLButtonElement>("#save"));
    });
    this.addEventListener("input", (e) => {
      const el = e.target as HTMLElement;
      if (el.closest("#attachments")) return;
      this.dirty = true;
      if (el.id === "t-abc" || el.id === "t-incipit") this.scheduleAbc();
      if (el.id === "t-meter") this.updateNotice();
    });
    this.addEventListener("change", (e) => {
      const el = e.target as HTMLElement;
      if (el.closest("#attachments") || el.id === "t-hidden") return;
      this.dirty = true;
      if (el.matches("input[name=suits], input[name=tricky]")) this.syncInstruments();
      if (el.id === "t-type") this.typeChanged();
      if (el.id === "t-tonic" || el.id === "t-mode") this.updateNotice();
      if (el.id === "t-tempo" || el.id === "t-meter") this.scheduleAbc();
      const rec = el.id.match(/^rec-(r\d+)-link$/);
      if (rec) void this.previewRecording(rec[1]!);
    });
    this.on("#t-hidden", "change", (_e, el) => void this.setHidden(el as HTMLInputElement));
    this.on("#remove", "click", (_e, el) => void this.removeTune(el as HTMLButtonElement));
    this.on("#reload", "click", () => void this.reload());

    this.on("[data-use-key]", "click", () => {
      const k = this.proposal?.key;
      if (!k) return;
      const tonic = this.$<HTMLSelectElement>("#t-tonic");
      if (![...tonic.options].some((o) => o.value === k.tonic)) tonic.add(new Option(tonicLabel(k.tonic), k.tonic));
      tonic.value = k.tonic;
      this.$<HTMLSelectElement>("#t-mode").value = k.mode;
      clearFieldError(this.$("fieldset[data-field=key]"));
      this.dirty = true;
      this.updateNotice();
      tonic.focus();
    });
    this.on("[data-use-meter]", "click", (_e, el) => {
      const meter = this.$<HTMLInputElement>("#t-meter");
      meter.value = el.dataset.useMeter!;
      clearFieldError(meter);
      this.$("#meter-propose").innerHTML = "";
      this.dirty = true;
      this.updateNotice();
      this.scheduleAbc();
      meter.focus();
    });

    this.on("#add-other", "click", () => {
      this.readOtherNames();
      this.otherNames.push({ name: "", translation: {} });
      this.$("#other-names").innerHTML = this.otherNameRows().value;
      this.querySelector<HTMLInputElement>(`#on-${this.otherNames.length - 1}-name`)?.focus();
    });
    this.on("button[data-other-remove]", "click", (_e, el) => {
      this.readOtherNames();
      this.otherNames.splice(Number(el.dataset.otherRemove), 1);
      this.dirty = true;
      this.$("#other-names").innerHTML = this.otherNameRows().value;
      this.$("#add-other").focus();
    });

    this.on("#add-recording", "click", () => {
      this.readRecordings();
      const r = recordingRow();
      this.recordings.push(r);
      this.$("#recordings").innerHTML = this.recordingRows().value;
      this.querySelector<HTMLInputElement>(`#rec-${r.key}-link`)?.focus();
    });
    this.on("button[data-rec-remove]", "click", (_e, el) => {
      this.readRecordings();
      this.recordings = this.recordings.filter((r) => r.key !== el.dataset.recRemove);
      this.dirty = true;
      this.$("#recordings").innerHTML = this.recordingRows().value;
      this.$("#add-recording").focus();
    });

    this.on("button[data-md-preview]", "click", (_e, el) => void this.previewMarkdown(el.dataset.mdPreview as "pl" | "en", el as HTMLButtonElement));

    // Recordings reorder in the form (saved with it); Attachments at once.
    bindSortable(this, (list, id, to, button) => {
      const sel = (dir: string) => `button[data-move="${id}"][data-list="${list}"][data-dir="${dir}"]`;
      const dir = button?.dataset.dir ?? "";
      const focus = () => {
        if (!button) return;
        const el = this.querySelector<HTMLButtonElement>(sel(dir));
        (el && !el.disabled ? el : this.querySelector<HTMLButtonElement>(sel(dir === "up" ? "down" : "up")))?.focus();
      };
      if (list === "recordings") {
        this.readRecordings();
        const r = this.recordings.find((x) => x.key === id)!;
        const rest = this.recordings.filter((x) => x !== r);
        rest.splice(to, 0, r);
        this.recordings = rest;
        this.dirty = true;
        this.$("#recordings").innerHTML = this.recordingRows().value;
        focus();
        return;
      }
      if (list === "attachments" && this.v) {
        const tuneId = this.v.id;
        void this.attachmentChange(button ?? null, () => api.moveAttachment(this.org, tuneId, id, to), L("Zmieniono kolejność", "Order changed")).then(focus);
      }
    });
    this.on("#att-file", "change", (_e, el) => void this.upload(el as HTMLInputElement));
    this.on("button[data-att-caption]", "click", (_e, el) => {
      const aid = el.dataset.attCaption!;
      const li = el.closest<HTMLElement>("li");
      const caption = { pl: this.val(`#att-${aid}-pl`), en: this.val(`#att-${aid}-en`) };
      void this.attachmentChange(el as HTMLButtonElement, () => api.setAttachmentCaption(this.org, this.v!.id, aid, caption), L("Zapisano podpis", "Caption saved"), li,
        `button[data-att-caption="${aid}"]`);
    });
    this.on("button[data-att-remove]", "click", (_e, el) => {
      const a = this.v?.attachments.find((x) => x.id === el.dataset.attRemove);
      if (!a || !confirm(L(`Usunąć załącznik „${a.name}”? Tego nie można cofnąć.`, `Remove the attachment “${a.name}”? This cannot be undone.`))) return;
      void this.attachmentChange(el as HTMLButtonElement, () => api.removeAttachment(this.org, this.v!.id, a.id), L("Usunięto załącznik", "Attachment removed"), null, "#att-file");
    });
    addEventListener("beforeunload", this.guard);
  }

  private guard = (e: BeforeUnloadEvent) => {
    if (this.dirty && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
    clearTimeout(this.timer);
    stopPlayback();
  }
}

customElements.define("folk-tune", TuneEditor);
