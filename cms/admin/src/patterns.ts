// Patterns shared by the Event and connection screens and shown on
// /app/design: flag badges, the connection banners, the switch and the
// Autosync warning. Styled in app.css ("Badges", "Messages", "Switch").

import type { ConnectionStatus, Flags, OverrideField } from "./api.ts";
import { icon } from "./icons.ts";
import { L } from "./i18n.ts";
import { fmtDate, html, raw, type Raw } from "./ui.ts";

export const FIELD_LABEL: Record<OverrideField, string> = {
  name: L("nazwa", "name"),
  description: L("opis", "description"),
  location: L("miejsce", "location"),
  cover: L("zdjęcie główne", "cover image"),
  ticketUrl: L("link do biletów", "ticket link"),
};

/** The flags of an Event as badges; `binReason` explains an Event in the Bin. */
export function flagBadges(f: Flags, binReason?: string): Raw {
  const b: Raw[] = [];
  if (f.binned)
    b.push(
      html`<span class="badge binned">${binReason === "gone" ? L("w koszu: usunięte z Facebooka", "in the Bin: gone from Facebook") : binReason === "removed" ? L("w koszu: usunięte przez moderatora", "in the Bin: removed by a Moderator") : L("w koszu", "in the Bin")}</span>`,
    );
  if (f.hidden) b.push(html`<span class="badge">${L("ukryte", "hidden")}</span>`);
  if (f.cancelled) b.push(html`<span class="badge failed">${L("odwołane", "cancelled")}</span>`);
  if (f.manual) b.push(html`<span class="badge info">${L("dodane ręcznie", "manual")}</span>`);
  if (f.overrideChanged?.length)
    b.push(html`<span class="badge warn">${L("zmienione na Facebooku", "changed on Facebook")}: ${f.overrideChanged.map((x) => FIELD_LABEL[x]).join(", ")}</span>`);
  if (f.textOutOfDate) b.push(html`<span class="badge warn">${L("tekst angielski lub skrót może być nieaktualny", "English or Summary may be out of date")}</span>`);
  if (f.formerPage) b.push(html`<span class="badge">${L("z poprzedniej strony", "from a former page")}</span>`);
  if (f.syncConflict) b.push(html`<span class="badge warn">${L("jest też na połączonej stronie", "also on the connected page")}</span>`);
  return b.length ? html`<span class="badges">${b}</span>` : raw("");
}

/** Banners about the Social Account Connection and Autosync, for the Event list and the connection screen. */
export function connectionBanners(c: ConnectionStatus, org: string, onConnectionScreen = false): Raw {
  const link = (text: string) => (onConnectionScreen ? "" : html` <a href="/app/orgs/${org}/social">${text}</a>`);
  const out: Raw[] = [];
  if (!c.connected) {
    out.push(
      html`<div class="banner info"><p>${L("Organizacja nie ma połączonej strony na Facebooku, więc synchronizacja i posty nie są możliwe.", "The organization has no connected Facebook page, so Sync and Social Posts are not possible.")}${link(L("Połącz stronę", "Connect the Facebook page"))}</p></div>`,
    );
  }
  const as = c.autosync;
  if (as?.needsReconnect) out.push(reconnectBanner(link(L("Przejdź do połączenia", "Go to the connection"))));
  else if (as?.reconnectSoon) out.push(reconnectBeforeBanner(as.dataAccessExpiresAt, link(L("Przejdź do połączenia", "Go to the connection"))));
  if (c.autosyncLost) out.push(autosyncLostBanner(c.autosyncLost.byEmail, c.autosyncLost.at, link(L("Przejdź do połączenia", "Go to the connection"))));
  return html`${out}`;
}

export function reconnectBanner(extra: Raw | string = ""): Raw {
  return html`<div class="banner danger" role="status"><p><strong>${L("Potrzebne ponowne połączenie", "Reconnect needed")}</strong><br>
    ${L(
      "Facebook odrzucił zapisany token strony, więc Autosync nie działa. Włącz Autosync ponownie (nowe logowanie do Meta).",
      "Facebook refused the stored page token, so Autosync is not running. Turn Autosync on again (a new Meta sign-in).",
    )}${extra}</p></div>`;
}

export function reconnectBeforeBanner(expires: string | undefined, extra: Raw | string = ""): Raw {
  return html`<div class="banner warn" role="status"><p><strong>${L(`Połącz ponownie przed ${fmtDate(expires)}`, `Reconnect before ${fmtDate(expires)}`)}</strong><br>
    ${L(
      "Meta kończy dostęp zapisanego tokenu do danych strony 90 dni po logowaniu, które go wydało. Odnów Autosync (nowe logowanie do Meta), aby synchronizacja nie stanęła.",
      "Meta ends the stored token's access to the page's data 90 days after the sign-in that issued it. Renew Autosync (a new Meta sign-in) so that Sync does not stop.",
    )}${extra}</p></div>`;
}

export function autosyncLostBanner(byEmail: string, at: string, extra: Raw | string = ""): Raw {
  return html`<div class="banner warn" role="status"><p><strong>${L("Autosync wyłączony", "Autosync turned off")}</strong><br>
    ${L(
      `Autosync wyłączył się ${fmtDate(at)}, bo ${byEmail}, który go włączył, nie ma już dostępu do tej organizacji. Zapisany token usunięto. Inny moderator może włączyć Autosync ponownie; do tego czasu synchronizacja i posty wymagają logowania do Meta.`,
      `Autosync turned off on ${fmtDate(at)} because ${byEmail}, who turned it on, no longer has access to this organization. The stored token was deleted. Another Moderator can turn Autosync on again; until then Sync and posts need a Meta sign-in.`,
    )}${extra}</p></div>`;
}

/** A switch: a checkbox with role=switch that acts at once. */
export function switchControl(id: string, label: string, on: boolean, disabled = false, describedBy = ""): Raw {
  return html`<label class="switch" for="${id}"><input type="checkbox" role="switch" id="${id}" ${on ? "checked" : ""} ${disabled ? "disabled" : ""}
    ${describedBy ? raw(`aria-describedby="${describedBy}"`) : ""}><span class="track" aria-hidden="true"></span><span>${label}</span></label>`;
}

/** The warning beside the Autosync switch (ADR 0010): what is stored, what it allows, the 90 days. */
export function autosyncWarning(id = "autosync-warning"): Raw {
  return html`<div class="banner warn" id="${id}">
    <p><strong>${L("Co oznacza włączenie Autosync", "What turning Autosync on means")}</strong></p>
    <ul>
      <li>${L("CMS zapisuje token strony na Facebooku w Azure Key Vault. Nikt nie może go odczytać przez CMS.", "The CMS stores a Facebook page token in Azure Key Vault. Nobody can read it through the CMS.")}</li>
      <li>${L("Z tym tokenem CMS co godzinę odczytuje wydarzenia strony i publikuje posty bez logowania do Meta (po niedawnym logowaniu do CMS). Kto mógłby uruchomić kod jako CMS, mógłby publikować na stronie.", "With it the CMS reads the page's events every hour and posts Social Posts without a Meta sign-in (after a recent CMS sign-in). Whoever could run code as the CMS could post to the page.")}</li>
      <li>${L("Meta kończy dostęp tokenu do danych po 90 dniach: ktoś musi zalogować się do Meta ponownie co najmniej tak często.", "Meta ends the token's data access after 90 days: someone must sign in with Meta again at least that often.")}</li>
      <li>${L("Wyłączenie Autosync, zmiana lub odłączenie strony usuwa token.", "Turning Autosync off, or changing or disconnecting the page, deletes the token.")}</li>
    </ul>
  </div>`;
}

/** The T11 instruction: a page token cannot revoke itself. */
export function removeAppInstruction(byEmail?: string): string {
  return byEmail
    ? L(
        `${byEmail}, który włączył Autosync, powinien też usunąć aplikację na Facebooku: Ustawienia → Integracje biznesowe.`,
        `${byEmail}, who turned Autosync on, should also remove the app on Facebook: Settings → Business integrations.`,
      )
    : L(
        "Osoba, która włączyła Autosync, powinna też usunąć aplikację na Facebooku: Ustawienia → Integracje biznesowe.",
        "The person who turned Autosync on should also remove the app on Facebook: Settings → Business integrations.",
      );
}

// ── Sortable list ──────────────────────────────────────────────────────
// An ordered list the Moderator reorders (Videos, Forms, Origins): each row
// has Move up / Move down buttons, the accessible way, and can also be
// dragged with a mouse. Markup: <ol class="sortable" data-list="…"> with
// <li data-id="…" draggable="true">; styled in app.css ("Sortable list").

/** The Move up / Move down buttons of row i of n; `name` names the row for screen readers. */
export function moveButtons(list: string, id: string, i: number, n: number, name: string): Raw {
  return html`<span class="move">
    <button type="button" class="icon-button" data-move="${id}" data-list="${list}" data-to="${i - 1}" data-dir="up" ${i === 0 ? "disabled" : ""}
      aria-label="${L(`Przesuń wyżej: ${name}`, `Move up: ${name}`)}" title="${L("Przesuń wyżej", "Move up")}">${icon("arrowUp")}</button>
    <button type="button" class="icon-button" data-move="${id}" data-list="${list}" data-to="${i + 1}" data-dir="down" ${i === n - 1 ? "disabled" : ""}
      aria-label="${L(`Przesuń niżej: ${name}`, `Move down: ${name}`)}" title="${L("Przesuń niżej", "Move down")}">${icon("arrowDown")}</button>
  </span>`;
}

/** The drag handle shown at the start of a row (decorative: the buttons are the accessible path). */
export const grip = (): Raw => html`<span class="grip" aria-hidden="true">${icon("grip")}</span>`;

/**
 * Wires the sortable lists inside root: the move buttons, and drag and drop
 * of rows within their own list. onMove gets the list, the row id and its
 * new index.
 */
export function bindSortable(root: HTMLElement, onMove: (list: string, id: string, to: number, button?: HTMLButtonElement) => void): void {
  root.addEventListener("click", (e) => {
    const b = (e.target as Element).closest<HTMLButtonElement>("button[data-move]");
    if (b && !b.disabled) onMove(b.dataset.list!, b.dataset.move!, Number(b.dataset.to), b);
  });
  let dragged: HTMLLIElement | null = null;
  const rowOf = (t: EventTarget | null): HTMLLIElement | null => {
    for (let el = t as Element | null; el && el !== root; el = el.parentElement) {
      if (dragged && el.parentElement === dragged.parentElement && el instanceof HTMLLIElement) return el;
    }
    return null;
  };
  const clear = () => root.querySelectorAll(".drop-before, .drop-after").forEach((el) => el.classList.remove("drop-before", "drop-after"));
  const after = (li: HTMLElement, e: DragEvent) => {
    const r = li.getBoundingClientRect();
    return e.clientY > r.top + r.height / 2;
  };
  root.addEventListener("dragstart", (e) => {
    // The innermost row: a region inside a country drags alone.
    const li = (e.target as Element).closest?.<HTMLLIElement>("ol.sortable > li[draggable=true]");
    if (!li || !root.contains(li)) return;
    dragged = li;
    li.classList.add("dragging");
    e.dataTransfer?.setData("text/plain", li.dataset.id ?? "");
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
  });
  root.addEventListener("dragover", (e) => {
    const li = rowOf(e.target);
    if (!li) return;
    e.preventDefault();
    clear();
    if (li !== dragged) li.classList.add(after(li, e) ? "drop-after" : "drop-before");
  });
  root.addEventListener("drop", (e) => {
    const li = rowOf(e.target);
    const from = dragged;
    clear();
    if (!li || !from || li === from) return;
    e.preventDefault();
    const rows = [...from.parentElement!.children];
    const i = rows.indexOf(from);
    const insert = rows.indexOf(li) + (after(li, e) ? 1 : 0);
    const to = insert > i ? insert - 1 : insert;
    if (to !== i) onMove(from.closest<HTMLElement>("ol.sortable")!.dataset.list!, from.dataset.id!, to);
  });
  root.addEventListener("dragend", () => {
    dragged?.classList.remove("dragging");
    dragged = null;
    clear();
  });
}
