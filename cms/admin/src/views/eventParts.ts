// Pieces shared by the synced Event screen (event.ts) and the Manual Event
// screen (manual.ts).

import { api, type EventView, type Image, type Me } from "../api.ts";
import { icon } from "../icons.ts";
import { L } from "../i18n.ts";
import { switchControl } from "../patterns.ts";
import { html, raw, showError, toast, type Raw } from "../ui.ts";

export const publicUrl = (me: Me, path?: string) => (path ? `${me.publicUrl.replace(/\/$/, "")}/${path}` : "");

/** An Event the Site shows can get Social Posts; Hidden ones and the Bin cannot. */
export const postable = (v: EventView) => !v.flags.binned && !v.flags.hidden && v.effective.occurrences.length > 0;

/** A text input or text area with its label; `path` is the field's path in validation problems. */
export function textField(o: {
  id: string;
  label: string | Raw;
  value?: string;
  path: string;
  kind?: "input" | "url" | "short" | "long";
  max?: number;
  required?: boolean;
  readonly?: boolean;
  hint?: string;
}): Raw {
  const kind = o.kind ?? "input";
  const attrs = raw(
    [o.required ? "required" : "", o.readonly ? "readonly" : "", o.max ? `maxlength="${o.max}"` : "", o.hint ? `aria-describedby="${o.id}-hint"` : ""].filter(Boolean).join(" "),
  );
  const control =
    kind === "short" || kind === "long"
      ? html`<textarea id="${o.id}" name="${o.path}" data-field="${o.path}" rows="${kind === "short" ? 3 : 8}" ${attrs}>${o.value ?? ""}</textarea>`
      : html`<input id="${o.id}" name="${o.path}" data-field="${o.path}" ${kind === "url" ? raw('type="url" placeholder="https://"') : ""} value="${o.value ?? ""}" ${attrs}>`;
  return html`<div class="field"><label for="${o.id}">${o.label}</label>${control}${o.hint ? html`<p class="muted small" id="${o.id}-hint">${o.hint}</p>` : ""}</div>`;
}

export const enLabel = (label: string) => html`${label} <span class="muted">${L("(EN, opcjonalnie)", "(EN, optional)")}</span>`;
export const plLabel = (label: string) => html`${label} <span class="muted">(PL)</span>`;

/** An image with its previews, a file picker and Remove. */
export function imageBlock(me: Me, kind: "cover" | "social", img: Image | undefined, label: string, disabled = false): Raw {
  const preview = img?.variants[img.variants.length - 1] ?? img?.variants[0];
  const id = `upload-${kind}`;
  return html`<div class="image-block" data-kind="${kind}">
    <h3>${label}</h3>
    ${img
      ? html`<div class="previews">
          ${preview ? html`<figure><img src="${publicUrl(me, preview.path)}" alt="" width="${preview.width}" height="${preview.height}"><figcaption>${L("Strona", "Site")}</figcaption></figure>` : ""}
          ${img.instagram ? html`<figure><img class="ig" src="${publicUrl(me, img.instagram.path)}" alt=""><figcaption>${L("Kadr Instagram", "Instagram crop")} ${img.instagram.width}×${img.instagram.height}</figcaption></figure>` : ""}
        </div>
        <p><button type="button" class="link danger" data-remove-image="${kind}" ${disabled ? "disabled" : ""}>${L("Usuń obraz", "Remove image")}</button></p>`
      : html`<p class="muted small">${L("Brak obrazu.", "No image.")}</p>`}
    <label class="button" for="${id}">${L("Wybierz plik…", "Choose file…")}<input type="file" id="${id}" accept="image/jpeg,image/png,image/webp" data-upload="${kind}" hidden ${disabled ? "disabled" : ""}></label>
    <p class="muted small">${L("JPEG, PNG lub WebP, do 15 MB. HEIC (iPhone) nie jest obsługiwany – zapisz zdjęcie jako JPEG.", "JPEG, PNG or WebP, up to 15 MB. HEIC (iPhone) is not supported – save the photo as JPEG.")}</p>
  </div>`;
}

/** Uploads the chosen file; undefined when nothing was uploaded. */
export async function upload(org: string, kind: "cover" | "social", input: HTMLInputElement): Promise<Image | undefined> {
  const file = input.files?.[0];
  input.value = "";
  if (!file) return undefined;
  if (/\.(heic|heif)$/i.test(file.name) || /hei[cf]/i.test(file.type)) {
    toast(L("Format HEIC nie jest obsługiwany – zapisz zdjęcie jako JPEG.", "HEIC is not supported – save the photo as JPEG."), "error");
    return undefined;
  }
  const block = input.closest(".image-block");
  block?.classList.add("uploading");
  try {
    return await api.uploadImage(org, kind, file);
  } catch (err) {
    showError(err);
    return undefined;
  } finally {
    block?.classList.remove("uploading");
  }
}

/** The Permalink of an Event in both languages. */
export function permalinks(v: EventView): Raw {
  return html`<dl class="facts permalink">
    <dt>${L("Adres na stronie", "Address on the Site")}</dt>
    <dd><a href="${v.permalink.pl}" target="_blank" rel="noopener">${v.permalink.pl}</a><br><a href="${v.permalink.en}" target="_blank" rel="noopener">${v.permalink.en}</a></dd>
  </dl>`;
}

/** The toolbar actions both screens share: Hide, the Post dialog, the Facebook Event, Restore. */
export function sharedActions(me: Me, org: string, v: EventView): Raw {
  const binned = !!v.flags.binned;
  const canPost = postable(v);
  return html`
    ${switchControl("hidden", L("Ukryte na stronie", "Hidden from the Site"), !!v.event.hidden, binned)}
    ${canPost
      ? html`<a class="button" href="/app/orgs/${org}/events/${v.event.id}/post">${L("Ogłoś w mediach", "Announce on social media")}</a>`
      : html`<a class="button" aria-disabled="true" title="${L("Ukrytych wydarzeń i wydarzeń w koszu nie można ogłaszać", "Hidden events and events in the Bin cannot be announced")}">${L("Ogłoś w mediach", "Announce on social media")}</a>`}
    ${v.facebookUrl ? html`<a class="button" href="${v.facebookUrl}" target="_blank" rel="noopener">${icon("external")}${L("Wydarzenie na Facebooku", "Facebook Event")}</a>` : ""}
    ${binned && me.superAdmin ? html`<button type="button" class="primary" data-action="restore">${L("Przywróć z kosza", "Restore from the Bin")}</button>` : ""}`;
}

export function binnedBanner(me: Me, v: EventView): Raw {
  if (!v.flags.binned) return raw("");
  const why =
    v.event.binReason === "gone"
      ? L("Facebook potwierdził, że wydarzenie zostało usunięte.", "Facebook confirmed the Facebook Event is gone.")
      : L("Moderator usunął to wydarzenie.", "A Moderator removed this event.");
  return html`<div class="banner warn" role="status"><p>${L("Wydarzenie jest w koszu i można je tylko przeglądać.", "The event is in the Bin and can only be viewed.")} ${why}
    ${me.superAdmin ? L(" Przywróć je, aby edytować.", " Restore it to edit it.") : L(" Przywrócić może administrator w ciągu 30 dni.", " The administrator can restore it within 30 days.")}</p></div>`;
}

/** Makes a screen of an Event in the Bin read-only: text stays selectable, other controls are disabled. */
export function readOnly(root: Element): void {
  root.querySelectorAll<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement | HTMLButtonElement>("section :is(input, select, textarea, button)").forEach((el) => {
    if ((el instanceof HTMLInputElement && ["text", "url", "search", ""].includes(el.type)) || el instanceof HTMLTextAreaElement) el.readOnly = true;
    else el.disabled = true;
  });
}

/** The local date (YYYY-MM-DD) and time (HH:MM) of an instant, in the Organizations' time zone. */
const ymd = new Intl.DateTimeFormat("en-CA", { timeZone: "Europe/Warsaw", year: "numeric", month: "2-digit", day: "2-digit" });
const hm = new Intl.DateTimeFormat("en-GB", { timeZone: "Europe/Warsaw", hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
export const localDate = (s: string) => ymd.format(new Date(s));
export const localTime = (s: string) => hm.format(new Date(s));
export function nextDay(date: string): string {
  const d = new Date(`${date}T12:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);
  return d.toISOString().slice(0, 10);
}
