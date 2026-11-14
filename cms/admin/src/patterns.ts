// Patterns shared by the Event and connection screens and shown on
// /app/design: flag badges, the connection banners, the switch and the
// Autosync warning. Styled in app.css ("Badges", "Messages", "Switch").

import type { ConnectionStatus, Flags, OverrideField } from "./api.ts";
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
