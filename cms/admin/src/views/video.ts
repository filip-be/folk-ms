// One Video of the Video Collection: add or edit. The link is parsed by the
// CMS (link-preview) as soon as it is entered, so the Moderator sees which
// video, start time and thumbnail will be saved.

import { api, ApiError, type LinkPreview, type Video, type VideoCollection, type VideoInput } from "../api.ts";
import { L } from "../i18n.ts";
import { busy, html, navigate, showError, t, toast, type Raw } from "../ui.ts";
import { View } from "../view.ts";
import { enLabel, plLabel, textField } from "./eventParts.ts";

const PROVIDER = { youtube: "YouTube", vimeo: "Vimeo" } as const;

function inputOf(v?: Video): VideoInput {
  return {
    link: v?.linkUrl ?? "",
    start: v?.start ?? "",
    title: { pl: v?.title.pl ?? "", en: v?.title.en ?? "" },
    description: { pl: v?.description?.pl ?? "", en: v?.description?.en ?? "" },
    credit: v?.credit ?? "",
    originId: v?.originId ?? "",
    formIds: [...(v?.formIds ?? [])],
  };
}

class VideoView extends View {
  private c!: VideoCollection;
  private v?: Video;
  private input: VideoInput = inputOf();
  private preview?: LinkPreview;
  private previewFor = "";
  private dirty = false;

  async load(): Promise<void> {
    this.c = await api.videos(this.org);
    if (this.params.id) {
      this.v = this.c.videos.find((x) => x.id === this.params.id);
      if (!this.v) {
        this.set(html`<div class="card"><h1>${L("Nie znaleziono", "Not found")}</h1><p><a href="/app/orgs/${this.org}/videos">${L("Wróć do listy filmów", "Back to the video list")}</a></p></div>`);
        return;
      }
    }
    this.input = inputOf(this.v);
    if (this.v) {
      this.preview = { provider: this.v.link.provider, videoId: this.v.link.id, hash: this.v.link.hash, url: this.v.url, linkUrl: this.v.linkUrl, thumbnailUrl: this.v.thumbnailUrl, start: this.v.start };
      this.previewFor = this.input.link;
    }
    this.render();
    this.bind();
  }

  private previewBlock(): Raw {
    const p = this.preview;
    if (!p) return html`<p class="muted small">${L("Wklej link do filmu na YouTube lub Vimeo; CMS pokaże podgląd.", "Paste a link to a YouTube or Vimeo video; the CMS shows a preview.")}</p>`;
    return html`<figure class="video-preview">
      <img src="${p.thumbnailUrl}" alt="" width="240" height="135">
      <figcaption>${PROVIDER[p.provider]} · <a href="${p.url}" target="_blank" rel="noopener">${p.linkUrl}</a>${p.start ? html` · ${L("od", "from")} ${p.start}` : ""}</figcaption>
    </figure>`;
  }

  private originSelect(): Raw {
    const sel = this.input.originId;
    const opt = (id: string, label: string) => html`<option value="${id}" ${sel === id ? "selected" : ""}>${label}</option>`;
    return html`<div class="field"><label for="v-origin">${L("Pochodzenie", "Origin")}</label>
      <select id="v-origin" name="originId" data-field="originId">
        <option value="">${L("— brak —", "— none —")}</option>
        ${this.c.origins.map((co) =>
          co.regions.length
            ? html`<optgroup label="${t(co.label)}">${opt(co.id, L(`${t(co.label)} (cały kraj)`, `${t(co.label)} (whole country)`))}${co.regions.map((r) => opt(r.id, `${t(r.label)}, ${t(co.label)}`))}</optgroup>`
            : opt(co.id, t(co.label)),
        )}
      </select>
      ${this.c.origins.length ? "" : html`<p class="muted small">${L("Lista jest pusta. ", "The list is empty. ")}<a href="/app/orgs/${this.org}/videos?view=origins">${L("Dodaj kraje", "Add countries")}</a></p>`}
    </div>`;
  }

  private formsField(): Raw {
    const fs = this.c.forms;
    return html`<fieldset data-field="formIds"><legend>${L("Formy", "Forms")} <span class="muted">${L("(dowolnie wiele)", "(any number)")}</span></legend>
      ${fs.length
        ? fs.map((f) => html`<label class="chip"><input type="checkbox" name="formIds" value="${f.id}" ${this.input.formIds.includes(f.id) ? "checked" : ""}> ${t(f.label)}</label>`)
        : html`<p class="muted small">${L("Lista jest pusta. ", "The list is empty. ")}<a href="/app/orgs/${this.org}/videos?view=forms">${L("Dodaj formy", "Add forms")}</a></p>`}
    </fieldset>`;
  }

  private render(): void {
    const c = this.input;
    const v = this.v;
    this.set(html`
      <form id="video" class="editor">
        <a href="/app/orgs/${this.org}/videos" class="muted small">← ${L("Wideopomoc", "Video help")}</a>
        <div class="toolbar sticky">
          <div><h1>${v ? t(v.title) : L("Nowy film", "New video")}</h1>
            ${v?.hidden ? html`<span class="badges"><span class="badge">${L("ukryty", "hidden")}</span></span>` : ""}</div>
          <div class="actions">
            <button type="submit" class="primary" id="save">${v ? L("Zapisz", "Save") : L("Dodaj film", "Add video")}</button>
            ${v ? html`<button type="button" class="danger" id="delete">${L("Usuń film", "Delete video")}</button>` : ""}
          </div>
        </div>
        <div id="problems"></div>
        <p class="required-note">${L("wymagane", "required")}</p>

        <section class="card" aria-labelledby="v-link-title">
          <h2 id="v-link-title">${L("Film", "Video")}</h2>
          <div class="grid">
            ${textField({ id: "v-link", label: L("Link do filmu (YouTube lub Vimeo)", "Video link (YouTube or Vimeo)"), value: c.link, path: "link", required: true })}
            ${textField({ id: "v-start", label: html`${L("Początek", "Start at")} <span class="muted">${L("(m:ss, opcjonalnie)", "(m:ss, optional)")}</span>`, value: c.start, path: "start",
              hint: L("Pusty: od początku, albo od czasu zapisanego w linku (t=).", "Empty: from the beginning, or from the time in the link (t=).") })}
          </div>
          <div id="v-preview" aria-live="polite">${this.previewBlock()}</div>
        </section>

        <section class="card" aria-labelledby="v-text-title">
          <h2 id="v-text-title">${L("Tytuł i opis", "Title and description")}</h2>
          <div class="pair">
            ${textField({ id: "v-title-pl", label: plLabel(L("Tytuł", "Title")), value: c.title.pl, path: "title.pl", required: true, max: 200 })}
            ${textField({ id: "v-title-en", label: enLabel(L("Tytuł", "Title")), value: c.title.en, path: "title.en", max: 200 })}
          </div>
          <div class="pair">
            ${textField({ id: "v-description-pl", label: plLabel(L("Opis", "Description")), value: c.description.pl, path: "description.pl", kind: "short", max: 2000 })}
            ${textField({ id: "v-description-en", label: enLabel(L("Opis", "Description")), value: c.description.en, path: "description.en", kind: "short", max: 2000 })}
          </div>
          ${textField({ id: "v-credit", label: html`${L("Autorzy / źródło", "Credit")} <span class="muted">${L("(opcjonalnie)", "(optional)")}</span>`, value: c.credit, path: "credit", max: 200,
            hint: L("Np. kto tańczy lub kto nagrał film.", "E.g. who dances or who recorded the video.") })}
        </section>

        <section class="card" aria-labelledby="v-kind-title">
          <h2 id="v-kind-title">${L("Pochodzenie i forma", "Origin and form")}</h2>
          ${this.originSelect()}
          ${this.formsField()}
        </section>
      </form>`);
  }

  /** Reads the form into this.input. */
  private collect(): VideoInput {
    const f = this.querySelector<HTMLFormElement>("#video");
    if (!f) return this.input;
    const val = (id: string) => (f.querySelector<HTMLInputElement>(`#${id}`)?.value ?? "").trim();
    const c = this.input;
    c.link = val("v-link");
    c.start = val("v-start");
    c.title = { pl: val("v-title-pl"), en: val("v-title-en") };
    c.description = { pl: val("v-description-pl"), en: val("v-description-en") };
    c.credit = val("v-credit");
    c.originId = f.querySelector<HTMLSelectElement>("#v-origin")!.value;
    c.formIds = [...f.querySelectorAll<HTMLInputElement>("input[name=formIds]:checked")].map((i) => i.value);
    return c;
  }

  /** Asks the CMS what the link (and start) will save as. */
  private async refreshPreview(): Promise<void> {
    const c = this.collect();
    const key = `${c.link}\n${c.start}`;
    if (key === this.previewFor) return;
    this.previewFor = key;
    const box = this.$("#v-preview");
    if (!c.link) {
      this.preview = undefined;
      box.innerHTML = this.previewBlock().value;
      return;
    }
    try {
      const p = await api.linkPreview(this.org, c.link, c.start);
      if (this.previewFor !== key) return; // a newer entry is on its way
      this.preview = p;
      // Show the start found in the link, so the Moderator sees it.
      const start = this.querySelector<HTMLInputElement>("#v-start");
      if (start && !start.value && p.start) {
        start.value = p.start;
        this.previewFor = `${c.link}\n${p.start}`;
      }
      box.innerHTML = this.previewBlock().value;
    } catch (err) {
      if (this.previewFor !== key) return;
      this.preview = undefined;
      box.innerHTML = this.previewBlock().value;
      showError(err, null, this.querySelector("#video"));
    }
  }

  private async save(btn: HTMLButtonElement | null): Promise<void> {
    const form = this.querySelector<HTMLFormElement>("#video")!;
    const c = this.collect();
    const problems = this.querySelector<HTMLElement>("#problems");
    await busy(btn, async () => {
      try {
        this.c = this.v ? await api.updateVideo(this.org, this.v.id, this.c.etag, c) : await api.addVideo(this.org, this.c.etag, c);
        this.dirty = false;
        toast(this.v ? L("Zapisano – strona zaktualizuje się w ciągu minuty", "Saved – the Site updates within a minute") : L("Dodano film – strona zaktualizuje się w ciągu minuty", "Video added – the Site updates within a minute"));
        navigate(`/app/orgs/${this.org}/videos`);
      } catch (err) {
        if (err instanceof ApiError && err.code === "conflict") {
          toast(L("Ktoś inny zmienił wideopomoc. Odśwież stronę, aby zobaczyć zmiany (Twoje wpisy przepadną).", "Someone else changed the video help. Refresh to see their changes (your entries will be lost)."), "error");
          return;
        }
        showError(err, problems, form);
      }
    });
  }

  private async deleteVideo(btn: HTMLButtonElement): Promise<void> {
    const v = this.v!;
    if (!confirm(L(`Usunąć film „${t(v.title)}”? Tego nie można cofnąć.`, `Delete the video “${t(v.title)}”? This cannot be undone.`))) return;
    await busy(btn, async () => {
      try {
        await api.deleteVideo(this.org, v.id, this.c.etag);
        this.dirty = false;
        toast(L("Usunięto film", "Video deleted"));
        navigate(`/app/orgs/${this.org}/videos`);
      } catch (err) {
        showError(err);
      }
    });
  }

  private bind(): void {
    this.addEventListener("submit", (e) => {
      e.preventDefault();
      void this.save(this.querySelector<HTMLButtonElement>("#save"));
    });
    this.on("#delete", "click", (_e, el) => void this.deleteVideo(el as HTMLButtonElement));
    this.on("#v-link, #v-start", "change", () => void this.refreshPreview());
    this.addEventListener("input", () => (this.dirty = true));
    this.addEventListener("change", () => (this.dirty = true));
    addEventListener("beforeunload", this.guard);
  }

  private guard = (e: BeforeUnloadEvent) => {
    if (this.dirty && this.isConnected) e.preventDefault();
  };

  disconnectedCallback(): void {
    removeEventListener("beforeunload", this.guard);
  }
}

customElements.define("folk-video", VideoView);

