// Facebook Event Handoff: prepares a Facebook Event for the next Occurrence
// of a Published Event, for the Moderator to copy (or have Autofill type)
// into Facebook's own form, and keeps the links of the Facebook Events they
// created. The CMS never creates, changes or removes a Facebook Event.

import { api, type ChangedField, type FacebookEventState, type HandoffView, type Occurrence } from "../api.ts";
import { ack, detect, fetchBase64, fieldMap, handOff, newId, onResult, poll, type FillResult, type Hello } from "../autofill.ts";
import { L, lang } from "../i18n.ts";
import { busy, fmtDateTime, html, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";

const FACEBOOK_CREATE = "https://www.facebook.com/events/create/";
const MAX_NAME = 100;

const CHANGED: Record<ChangedField, string> = {
  title: L("tytuł", "title"),
  start: L("początek", "start"),
  end: L("koniec", "end"),
  venue: L("miejsce", "venue"),
  cancelled: L("odwołanie", "cancellation"),
};

/** A local date and time in the Organization's time zone, for people. */
function when(iso: string, tz: string): string {
  return new Intl.DateTimeFormat(lang === "pl" ? "pl-PL" : "en-GB", {
    timeZone: tz,
    weekday: "long",
    day: "numeric",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(iso));
}

class FacebookEventView extends View {
  private data!: HandoffView;
  private autofill?: Hello;
  private cover?: { data: string; type: string };
  private pending?: FillResult;
  private stopResults?: () => void;
  private pollTimer?: number;
  private tz = "Europe/Warsaw";

  get eventId(): string {
    return this.params.id!;
  }

  async load(): Promise<void> {
    this.data = await api.facebookEvent(this.org, this.eventId);
    this.tz = this.data.fields?.timezone ?? this.tz;
    this.autofill = await detect();
    this.render();
    this.bind();
    if (this.autofill) {
      this.stopResults = onResult((r) => this.onResult(r));
      poll();
      this.pollTimer = window.setInterval(poll, 3000);
    }
    if (this.autofill && this.data.coverUrl) this.cover = await fetchBase64(this.data.coverUrl).catch(() => undefined);
  }

  disconnectedCallback(): void {
    this.stopResults?.();
    if (this.pollTimer) clearInterval(this.pollTimer);
  }

  private onResult(r: FillResult): void {
    if (r.org !== this.org || r.eventId !== this.eventId || !r.url) return;
    if (this.data.links.some((s) => s.link.url === r.url)) {
      ack([r.id]); // already saved
      return;
    }
    this.pending = r;
    this.render();
  }

  private update(v: HandoffView, msg?: string): void {
    this.data = v;
    this.render();
    if (msg) toast(msg);
  }

  private occLabel(key: string, occ?: Occurrence): string {
    const o = occ ?? this.data.occurrences.find((x) => x.key === key);
    return o ? when(o.start, this.tz) : key;
  }

  private copyBtn(value: string): unknown {
    return html`<button type="button" class="link" data-copy="${value}">${L("Kopiuj", "Copy")}</button>`;
  }

  private linkCard(s: FacebookEventState): unknown {
    const l = s.link;
    const options = this.data.occurrences.filter((o) => !o.cancelled);
    return html`<li class="fb-link ${s.detached ? "detached" : ""}" data-link="${l.id}">
      <div><a href="${l.url}" target="_blank" rel="noopener">${l.url}</a>
        ${s.detached ? html` <span class="badge warn">${L("bez terminu", "detached")}</span>` : ""}
        ${s.outOfDate && !s.detached ? html` <span class="badge warn">${L("może być nieaktualne", "may be out of date")}</span>` : ""}</div>
      <div class="muted small">${s.detached ? L(`Termin ${l.occurrenceKey} już nie istnieje – link jest ukryty na stronie.`, `The date ${l.occurrenceKey} no longer exists – the link is hidden on the site.`) : this.occLabel(l.occurrenceKey, s.occurrence)}
        · ${L("zapisano", "recorded")} ${fmtDateTime(l.recordedAt)}</div>
      ${s.unpublished ? html`<p class="small">${L("Wydarzenie nie jest opublikowane: zmień lub usuń wydarzenie na Facebooku ręcznie.", "The event is not published: change or delete the Facebook event by hand.")}</p>` : ""}
      ${s.outOfDate && s.current && !s.detached
        ? html`<div class="banner warn small"><p>${L("Od przekazania zmieniło się", "Changed since the handoff")}: <strong>${s.changed.map((c) => CHANGED[c]).join(", ")}</strong>.
            ${L("Popraw wydarzenie na Facebooku ręcznie, potem kliknij „Zaktualizowano”.", "Edit the Facebook event by hand, then click “Updated”.")}</p>
            <dl class="facts">${s.changed.map((c) => {
              const cur = s.current!;
              const v =
                c === "title" ? cur.title : c === "start" ? when(cur.start, this.tz) : c === "end" ? when(cur.end, this.tz) : c === "venue" ? `${cur.venueName}, ${cur.venueAddress}` : cur.cancelled ? L("odwołane", "cancelled") : L("odbędzie się", "takes place");
              return html`<dt>${CHANGED[c]}</dt><dd>${v} ${this.copyBtn(v)}</dd>`;
            })}</dl>
            <button type="button" data-dismiss="${l.id}">${L("Zaktualizowano na Facebooku", "Updated on Facebook")}</button></div>`
        : ""}
      <div class="row small">
        <input type="url" value="${l.url}" data-url-of="${l.id}" aria-label="${L("Adres wydarzenia na Facebooku", "Facebook event address")}">
        ${this.data.published && options.length > 1 || s.detached
          ? html`<select data-key-of="${l.id}" aria-label="${L("Termin", "Date")}">${options.map((o) => html`<option value="${o.key}" ${o.key === l.occurrenceKey ? "selected" : ""}>${when(o.start, this.tz)}</option>`)}</select>`
          : ""}
        <button type="button" data-change="${l.id}">${s.detached ? L("Przypisz", "Re-attach") : L("Zmień", "Change")}</button>
        <button type="button" class="link danger" data-remove="${l.id}">${L("Usuń link", "Remove link")}</button>
      </div>
    </li>`;
  }

  private render(): void {
    const d = this.data;
    const back = html`<a href="/app/orgs/${this.org}/events/${this.eventId}" class="muted small">${L("← Wróć do wydarzenia", "← Back to the event")}</a>`;
    const head = html`${back}<h1>${L("Wydarzenie na Facebooku", "Facebook event")}</h1><p class="muted">${t(d.title)}</p>`;
    const detached = d.links.filter((s) => s.detached);
    const attached = d.links.filter((s) => !s.detached);
    const links = d.links.length
      ? html`<section class="card"><h2>${L("Zapisane wydarzenia na Facebooku", "Recorded Facebook events")}</h2>
          <ul class="fb-links">${[...detached, ...attached].map((s) => this.linkCard(s))}</ul></section>`
      : "";
    if (!d.published) {
      this.set(html`${head}<div class="card"><p>${L("Wydarzenie na Facebooku można przygotować tylko dla opublikowanego wydarzenia.", "A Facebook event can be prepared only for a published event.")}</p></div>${links}`);
      return;
    }
    const conn = d.connection;
    const f = d.fields;
    const pending = this.pending;
    this.set(html`${head}
      ${pending
        ? html`<div class="banner" role="status"><p>${L("Autofill przechwycił adres nowego wydarzenia na Facebooku:", "Autofill captured the address of the new Facebook event:")}
            <a href="${pending.url}" target="_blank" rel="noopener">${pending.url}</a></p>
            <button type="button" class="primary" data-confirm-result>${L("Zapisz link", "Save the link")}</button>
            <button type="button" data-reject-result>${L("Odrzuć", "Discard")}</button></div>`
        : ""}
      ${detached.length ? html`<div class="banner warn">${L("Niektóre linki straciły swój termin. Przypisz je do innego terminu albo usuń, zanim utworzysz nowe wydarzenie.", "Some links lost their date. Re-attach them to another date or remove them before creating a new event.")}</div>` : ""}
      ${links}
      ${!conn.connected
        ? html`<div class="banner warn">${L("Organizacja nie ma połączonej strony na Facebooku, która byłaby gospodarzem wydarzenia.", "The organization has no connected Facebook Page to host the event.")} <a href="/app/orgs/${this.org}/social">${L("Połącz stronę", "Connect a Page")}</a></div>`
        : !f
          ? html`<div class="card"><p>${L("To wydarzenie nie ma nadchodzącego, nieodwołanego terminu.", "This event has no upcoming, non-cancelled date.")}</p></div>`
          : html`<section class="card fb-handoff">
              <h2>${L("Nowe wydarzenie na Facebooku", "New Facebook event")} · ${when(f.start, this.tz)}</h2>
              ${d.nextLinked ? html`<p class="banner warn small">${L("Ten termin ma już zapisane wydarzenie na Facebooku.", "This date already has a recorded Facebook event.")}</p>` : ""}
              <p class="banner small">${L("Na Facebooku działaj jako strona", "On Facebook, act as the Page")} <strong>${conn.pageName}</strong>${L(" – przełącz profil, zanim utworzysz wydarzenie.", " – switch your profile before creating the event.")}</p>
              <div class="field"><label for="fb-name">${L("Nazwa", "Name")}</label>
                <input id="fb-name" maxlength="${MAX_NAME}" value="${f.name}">
                <div class="row"><span class="muted small" id="fb-name-count"></span><button type="button" class="link" data-copy-from="fb-name">${L("Kopiuj", "Copy")}</button></div></div>
              <dl class="facts">
                <dt>${L("Początek", "Start")}</dt><dd>${when(f.start, this.tz)} ${this.copyBtn(when(f.start, this.tz))}</dd>
                <dt>${L("Koniec", "End")}</dt><dd>${when(f.end, this.tz)} ${this.copyBtn(when(f.end, this.tz))}</dd>
                <dt>${L("Strefa czasowa", "Time zone")}</dt><dd>${f.timezone}</dd>
                <dt>${L("Miejsce", "Location")}</dt><dd>${f.location} ${this.copyBtn(f.location)}</dd>
                ${f.ticketUrl ? html`<dt>${L("Bilety", "Tickets")}</dt><dd>${f.ticketUrl} ${this.copyBtn(f.ticketUrl)}</dd>` : ""}
              </dl>
              <div class="field"><label for="fb-desc">${L("Szczegóły", "Details")}</label>
                <textarea id="fb-desc" rows="12">${f.description}</textarea>
                <div class="row"><span></span><button type="button" class="link" data-copy-from="fb-desc">${L("Kopiuj", "Copy")}</button></div></div>
              ${d.coverUrl
                ? html`<div class="field"><span class="label">${L("Zdjęcie", "Cover photo")}</span><img class="post-image facebook" src="${d.coverUrl}" alt="">
                    <a class="button" href="${d.coverUrl}" download="${d.coverName ?? "cover.jpg"}">${L("Pobierz zdjęcie", "Download the photo")}</a></div>`
                : html`<p class="muted small">${L("Wydarzenie nie ma zdjęcia głównego.", "The event has no cover image.")}</p>`}
              <div class="actions">
                ${this.autofill
                  ? html`<button type="button" class="primary" data-fill>${L("Wypełnij na Facebooku", "Fill on Facebook")}</button>
                      <a class="button" href="${FACEBOOK_CREATE}" target="_blank" rel="noopener">${L("Otwórz Facebooka", "Open Facebook")}</a>`
                  : html`<a class="button primary" href="${FACEBOOK_CREATE}" target="_blank" rel="noopener">${L("Otwórz Facebooka", "Open Facebook")}</a>`}
              </div>
              ${this.autofill
                ? html`<p class="muted small">${L("Autofill wpisze pola w formularz Facebooka. Sprawdź je i kliknij „Utwórz wydarzenie” – Autofill nigdy nie tworzy wydarzenia sam.", "Autofill types the fields into Facebook's form. Review them and press “Create event” – Autofill never creates the event itself.")}</p>`
                : html`<p class="muted small">${L("Możesz zainstalować Autofill, który wpisze pola za Ciebie:", "You can install Autofill to type the fields for you:")}
                    <a href="/app/autofill.user.js">${L("skrypt użytkownika", "userscript")}</a> ${L("(Violentmonkey, Tampermonkey, Userscripts) lub rozszerzenie ze sklepu – zobacz instrukcję w README.", "(Violentmonkey, Tampermonkey, Userscripts) or the browser extension from the store – see the README.")}</p>`}
            </section>`}
      <section class="card">
        <h2>${L("Zapisz link do wydarzenia na Facebooku", "Record a Facebook event link")}</h2>
        <p class="muted small">${L("Po utworzeniu wydarzenia wklej jego adres. Strona pokaże go przy terminie.", "After creating the event, paste its address. The site shows it next to the date.")}</p>
        <form class="inline-form" id="add-link">
          <input type="url" id="new-url" required placeholder="https://www.facebook.com/events/…" aria-label="${L("Adres wydarzenia na Facebooku", "Facebook event address")}">
          ${d.occurrences.length > 1
            ? html`<select id="new-key" aria-label="${L("Termin", "Date")}">${d.occurrences
                .filter((o) => !o.cancelled)
                .map((o) => html`<option value="${o.key}" ${o.key === f?.occurrenceKey ? "selected" : ""}>${when(o.start, this.tz)}</option>`)}</select>`
            : ""}
          <button type="submit" class="primary">${L("Zapisz link", "Save the link")}</button>
        </form>
      </section>`);
    this.count();
  }

  private count(): void {
    const n = this.querySelector<HTMLInputElement>("#fb-name");
    const c = this.querySelector("#fb-name-count");
    if (n && c) c.textContent = `${[...n.value].length} / ${MAX_NAME}`;
  }

  private bind(): void {
    this.addEventListener("input", (e) => {
      if ((e.target as HTMLElement).id === "fb-name") this.count();
    });
    this.on("[data-copy], [data-copy-from]", "click", (_e, el) => {
      const v = el.dataset.copy ?? this.querySelector<HTMLInputElement | HTMLTextAreaElement>(`#${el.dataset.copyFrom}`)?.value ?? "";
      navigator.clipboard.writeText(v).then(
        () => toast(L("Skopiowano", "Copied")),
        () => toast(L("Nie udało się skopiować", "Could not copy"), "error"),
      );
    });
    this.on("#add-link", "submit", (e) => {
      e.preventDefault();
      const url = this.$<HTMLInputElement>("#new-url").value;
      const key = this.querySelector<HTMLSelectElement>("#new-key")?.value ?? this.data.fields?.occurrenceKey;
      void this.save(this.querySelector<HTMLButtonElement>("#add-link button"), () =>
        api.addFacebookEvent(this.org, this.eventId, {
          url,
          occurrenceKey: key,
          token: key === this.data.fields?.occurrenceKey ? this.data.token : undefined,
        }),
      L("Zapisano link", "Link saved"));
    });
    this.on("[data-change]", "click", (_e, el) => {
      const id = el.dataset.change!;
      const url = this.querySelector<HTMLInputElement>(`[data-url-of="${id}"]`)?.value;
      const key = this.querySelector<HTMLSelectElement>(`[data-key-of="${id}"]`)?.value;
      void this.save(el as HTMLButtonElement, () => api.changeFacebookEvent(this.org, this.eventId, id, { url, occurrenceKey: key }), L("Zmieniono link", "Link changed"));
    });
    this.on("[data-remove]", "click", (_e, el) => {
      if (!confirm(L("Usunąć link? Wydarzenie na Facebooku pozostanie – usuń je na Facebooku, jeśli trzeba.", "Remove the link? The Facebook event stays – delete it on Facebook if needed."))) return;
      void this.save(el as HTMLButtonElement, () => api.removeFacebookEvent(this.org, this.eventId, el.dataset.remove!), L("Usunięto link", "Link removed"));
    });
    this.on("[data-dismiss]", "click", (_e, el) => {
      void this.save(el as HTMLButtonElement, () => api.dismissFacebookEvent(this.org, this.eventId, el.dataset.dismiss!), L("Oznaczono jako aktualne", "Marked as up to date"));
    });
    this.on("[data-confirm-result]", "click", (_e, el) => {
      const r = this.pending;
      if (!r) return;
      void this.save(
        el as HTMLButtonElement,
        async () => {
          const v = await api.addFacebookEvent(this.org, this.eventId, { url: r.url, occurrenceKey: r.occurrenceKey, token: r.token });
          ack([r.id]);
          this.pending = undefined;
          return v;
        },
        L("Zapisano link", "Link saved"),
      );
    });
    this.on("[data-reject-result]", "click", () => {
      if (this.pending) ack([this.pending.id]);
      this.pending = undefined;
      this.render();
    });
    this.on("[data-fill]", "click", (_e, el) => void this.fill(el as HTMLButtonElement));
  }

  private async save(btn: HTMLButtonElement | null, fn: () => Promise<HandoffView>, msg: string): Promise<void> {
    await busy(btn, async () => {
      try {
        this.update(await fn(), msg);
      } catch (err) {
        showError(err);
      }
    });
  }

  /** Hands the edited fields to Autofill, then opens Facebook. */
  private async fill(btn: HTMLButtonElement): Promise<void> {
    const d = this.data;
    const f = d.fields;
    if (!f || !d.token) return;
    const name = this.$<HTMLInputElement>("#fb-name").value.trim();
    const description = this.$<HTMLTextAreaElement>("#fb-desc").value;
    if (!name) {
      toast(L("Podaj nazwę wydarzenia.", "Enter the event name."), "error");
      return;
    }
    await busy(btn, async () => {
      const ok = await handOff({
        id: newId(),
        org: this.org,
        eventId: this.eventId,
        occurrenceKey: f.occurrenceKey,
        token: d.token!,
        pageName: d.connection.pageName ?? "",
        fields: {
          name,
          startDate: f.startDate,
          startTime: f.startTime,
          endDate: f.endDate,
          endTime: f.endTime,
          timezone: f.timezone,
          location: f.location,
          description,
          ticketUrl: f.ticketUrl,
        },
        cover: this.cover ? { name: d.coverName ?? "cover.jpg", type: this.cover.type, data: this.cover.data } : undefined,
        fieldMap,
        returnUrl: location.href,
        lang,
      });
      if (!ok) {
        toast(L("Autofill nie odpowiada. Użyj przycisków „Kopiuj” i „Otwórz Facebooka”.", "Autofill does not answer. Use the Copy buttons and “Open Facebook”."), "error");
        return;
      }
      window.open(FACEBOOK_CREATE, "_blank", "noopener");
    });
  }
}

customElements.define("folk-facebook-event", FacebookEventView);
