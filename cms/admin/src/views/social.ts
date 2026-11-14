// The connection screen: the Social Account Connection (page, who connected
// it), the Autosync switch with its warning (ADR 0010), replacing the page
// (decision 17) and disconnecting (T11).

import { api, ApiError, type ConnectionStatus, type PageInfo } from "../api.ts";
import { L } from "../i18n.ts";
import { autosyncWarning, connectionBanners, removeAppInstruction, switchControl } from "../patterns.ts";
import { busy, fmtDate, fmtDateTime, html, recentSignInBanner, showError, showRecentSignIn, toast } from "../ui.ts";
import { View } from "../view.ts";

/** What the Meta callback of turning Autosync on reported (`?autosync=`). */
function autosyncOutcome(reason: string, pageName: string): [string, "ok" | "error"] {
  switch (reason) {
    case "on":
      return [L("Autosync włączony. CMS będzie synchronizować wydarzenia co godzinę.", "Autosync is on. The CMS will sync the events every hour."), "ok"];
    case "cancelled":
      return [L("Anulowano logowanie do Meta. Autosync nie został włączony.", "The Meta sign-in was cancelled. Autosync was not turned on."), "error"];
    case "page":
      return [L(`Twoje konto Meta nie ma roli na stronie „${pageName}”. Autosync nie został włączony.`, `Your Meta account has no role on the Facebook page “${pageName}”. Autosync was not turned on.`), "error"];
    case "recent_sign_in":
      return [L("Logowanie do CMS było zbyt dawno. Zaloguj się ponownie i włącz Autosync jeszcze raz.", "Your CMS sign-in is too old. Sign in again, then turn Autosync on once more."), "error"];
    default:
      return [L("Nie udało się włączyć Autosync. Spróbuj ponownie.", "Autosync could not be turned on. Try again."), "error"];
  }
}

class SocialView extends View {
  private st!: ConnectionStatus;
  /** Set after Autosync was turned off or the page disconnected: who should remove the app on Facebook (T11). */
  private removedBy?: string;

  async load(): Promise<void> {
    this.st = await api.social(this.org);
    const q = new URLSearchParams(location.search);
    const err = q.get("error");
    if (err) toast(err === "cancelled" ? L("Anulowano logowanie do Meta.", "The Meta sign-in was cancelled.") : L("Nie udało się połączyć ze stroną.", "Connecting the Facebook page failed."), "error");
    const as = q.get("autosync");
    if (as) {
      const [msg, kind] = autosyncOutcome(as, this.st.pageName ?? "");
      toast(msg, kind);
    }
    if (err || as) history.replaceState(null, "", location.pathname);
    this.render();
    if (as === "recent_sign_in") showRecentSignIn();
    this.on("#connect", "click", (_e, el) =>
      void busy(el as HTMLButtonElement, async () => {
        try {
          const { url } = await api.connect(this.org);
          location.href = url;
        } catch (err) {
          showError(err);
        }
      }),
    );
    this.on("[data-page]", "click", (_e, el) => void this.select(el.dataset.page!, el as HTMLButtonElement));
    this.on("#autosync", "change", (_e, el) => void this.toggleAutosync(el as HTMLInputElement));
    this.on("#renew", "click", (_e, el) => void busy(el as HTMLButtonElement, () => this.startAutosync()));
    this.on("#disconnect", "click", (_e, el) => void this.disconnect(el as HTMLButtonElement));
  }

  /** Chooses a page; a different page than the connected one is confirmed naming both (decision 17). */
  private async select(pageId: string, btn: HTMLButtonElement): Promise<void> {
    const page = this.st.pendingPages?.find((p) => p.id === pageId);
    let confirmed = false;
    if (this.st.connected && this.st.pageId !== pageId) {
      if (!confirm(this.replaceText(page))) return;
      confirmed = true;
    }
    await busy(btn, async () => {
      for (;;) {
        try {
          const before = this.st.autosync?.byEmail;
          this.st = await api.selectPage(this.org, pageId, confirmed);
          if (before && !this.st.autosync) this.removedBy = before;
          toast(L("Połączono stronę", "Facebook page connected"));
          this.render();
          return;
        } catch (err) {
          if (err instanceof ApiError && err.code === "needs_confirmation" && !confirmed) {
            if (!confirm(this.replaceText(page))) return;
            confirmed = true;
            continue;
          }
          showError(err);
          return;
        }
      }
    });
  }

  private replaceText(page?: PageInfo): string {
    const from = this.st.pageName ?? "";
    const to = page?.name ?? "";
    return [
      L(`Zastąpić połączoną stronę „${from}” stroną „${to}”?`, `Replace the connected Facebook page “${from}” with “${to}”?`),
      L(
        `Wydarzenia ze strony „${from}” zostaną na stronie internetowej bez zmian i nie będą już synchronizowane; ukryj te, których nie chcesz. Wydarzenia dodane ręcznie się nie zmienią.`,
        `Events synced from “${from}” stay on the Site as they are and are no longer synced; hide those you do not want. Manual events are not affected.`,
      ),
      this.st.autosync ? L("Autosync zostanie wyłączony, a zapisany token usunięty.", "Autosync is turned off and the stored token deleted.") : "",
    ]
      .filter(Boolean)
      .join("\n\n");
  }

  private async startAutosync(): Promise<void> {
    try {
      const res = await api.setAutosync(this.org, true);
      if (res.url) location.href = res.url; // the Meta sign-in that issues the token; its callback comes back here
    } catch (err) {
      showError(err);
    }
  }

  private async toggleAutosync(sw: HTMLInputElement): Promise<void> {
    const on = sw.checked;
    const question = on
      ? L(
          "Włączyć Autosync? CMS zapisze token strony na Facebooku w Azure Key Vault i będzie co godzinę synchronizować wydarzenia oraz publikować posty bez logowania do Meta. Teraz nastąpi logowanie do Meta.",
          "Turn Autosync on? The CMS will store a Facebook page token in Azure Key Vault, sync the events every hour and post Social Posts without a Meta sign-in. You will now sign in with Meta.",
        )
      : L(
          "Wyłączyć Autosync? Zapisany token zostanie usunięty; synchronizacja i posty będą znów wymagać logowania do Meta.",
          "Turn Autosync off? The stored token is deleted; Sync and posts will need a Meta sign-in again.",
        );
    if (!confirm(question)) {
      sw.checked = !on;
      return;
    }
    sw.disabled = true;
    if (on) {
      await this.startAutosync();
      sw.checked = false;
      sw.disabled = false;
      return;
    }
    try {
      const by = this.st.autosync?.byEmail;
      await api.setAutosync(this.org, false);
      this.st = await api.social(this.org);
      this.removedBy = by;
      toast(L("Autosync wyłączony, token usunięty", "Autosync is off and the token deleted"));
      this.render();
    } catch (err) {
      sw.checked = true;
      sw.disabled = false;
      showError(err);
    }
  }

  private async disconnect(btn: HTMLButtonElement): Promise<void> {
    const as = this.st.autosync;
    const text = [
      L("Odłączyć stronę? Synchronizacja i posty nie będą możliwe do ponownego połączenia. Wydarzenia zostaną na stronie internetowej.", "Disconnect the Facebook page? Sync and posting stop until a page is connected again. The events stay on the Site."),
      as ? L("Autosync zostanie wyłączony, a zapisany token usunięty.", "Autosync is turned off and the stored token deleted.") + " " + removeAppInstruction(as.byEmail) : "",
    ]
      .filter(Boolean)
      .join("\n\n");
    if (!confirm(text)) return;
    await busy(btn, async () => {
      try {
        await api.disconnect(this.org);
        this.st = await api.social(this.org);
        if (as) this.removedBy = as.byEmail;
        toast(L("Rozłączono", "Disconnected"));
        this.render();
      } catch (err) {
        showError(err);
      }
    });
  }

  private render(): void {
    const st = this.st;
    const as = st.autosync;
    // Guarded actions need a Recent sign-in while a token is stored (decision 13).
    const guarded = !!as && !st.recentSignIn;
    this.set(html`
      <h1>${L("Facebook i Instagram", "Facebook and Instagram")}</h1>
      ${guarded ? recentSignInBanner(true) : ""}
      ${connectionBanners(st, this.org, true)}
      ${this.removedBy
        ? html`<div class="banner warn" role="status"><p><strong>${L("Usuń też aplikację na Facebooku", "Also remove the app on Facebook")}</strong><br>
            ${L("Zapisany token został usunięty z CMS, ale token strony nie może sam się unieważnić.", "The stored token was deleted from the CMS, but a page token cannot revoke itself.")} ${removeAppInstruction(this.removedBy)}</p></div>`
        : ""}
      ${st.pendingPages?.length
        ? html`<section class="card"><h2>${L("Wybierz stronę na Facebooku", "Choose a Facebook page")}</h2>
            <ul class="pages">${st.pendingPages.map(
              (p) => html`<li><span><strong>${p.name}</strong> <span class="muted small">(id ${p.id})</span>
                ${p.igUsername ? html`<span class="muted">· Instagram @${p.igUsername}</span>` : html`<span class="muted">${L("· bez Instagrama", "· no Instagram")}</span>`}
                ${st.connected && p.id === st.pageId ? html`<span class="badge posted">${L("połączona", "connected")}</span>` : ""}</span>
                <button type="button" class="secondary" data-page="${p.id}" ${guarded ? "disabled" : ""}>${L("Wybierz", "Choose")}</button></li>`,
            )}</ul></section>`
        : ""}
      <section class="card">
        ${st.connected
          ? html`<h2>${L("Połączona strona", "Connected page")}</h2>
            <dl class="facts"><dt>${L("Strona", "Facebook page")}</dt><dd>${st.pageName} <span class="muted small">(id ${st.pageId})</span></dd>
              <dt>Instagram</dt><dd>${st.igUsername ? `@${st.igUsername}` : L("brak połączonego konta", "no connected account")}</dd>
              <dt>${L("Połączył(a)", "Connected by")}</dt><dd>${st.createdByEmail} · ${fmtDateTime(st.createdAt)}</dd></dl>
            <div class="actions">
              <button type="button" id="connect" ${guarded ? "disabled" : ""}>${L("Zmień stronę", "Change the Facebook page")}</button>
              <button type="button" id="disconnect" class="danger" ${guarded ? "disabled" : ""}>${L("Rozłącz", "Disconnect")}</button>
            </div>
            ${as ? html`<p class="muted small">${L("Po rozłączeniu: ", "After disconnecting: ")}${removeAppInstruction(as.byEmail)}</p>` : ""}`
          : html`<h2>${L("Brak połączenia", "Not connected")}</h2>
            <p>${L("Wybierz stronę organizacji na Facebooku (i powiązane konto Instagram). CMS synchronizuje jej wydarzenia i ogłasza je w postach.", "Choose the organization's Facebook page (and its linked Instagram account). The CMS syncs its events and announces them in Social Posts.")}</p>
            <button type="button" id="connect" class="primary">${L("Wybierz stronę przez Meta", "Choose the Facebook page with Meta")}</button>`}
      </section>
      ${st.connected ? this.autosyncCard() : ""}`);
  }

  private autosyncCard(): ReturnType<typeof html> {
    const st = this.st;
    const as = st.autosync;
    const guarded = !!as && !st.recentSignIn;
    return html`<section class="card" aria-labelledby="autosync-title">
      <h2 id="autosync-title">Autosync</h2>
      ${switchControl("autosync", L("Synchronizuj co godzinę (Autosync)", "Sync every hour (Autosync)"), !!as, guarded, "autosync-warning")}
      ${as
        ? html`<dl class="facts">
            <dt>${L("Włączył(a)", "Turned on by")}</dt><dd>${as.byEmail} · ${fmtDateTime(as.at)}</dd>
            <dt>${L("Dostęp do danych do", "Data access until")}</dt><dd>${as.dataAccessExpiresAt ? fmtDate(as.dataAccessExpiresAt) : "–"}
              ${as.needsReconnect ? html` <span class="badge failed">${L("token odrzucony", "token refused")}</span>` : as.reconnectSoon ? html` <span class="badge warn">${L("wkrótce", "soon")}</span>` : ""}</dd>
          </dl>
          <p><button type="button" class="secondary" id="renew" ${guarded ? "disabled" : ""}>${L("Odnów: zaloguj się do Meta", "Renew: sign in with Meta")}</button></p>`
        : html`<p class="small">${L(
            "Autosync jest wyłączony: CMS nie przechowuje nic z Meta. „Synchronizuj teraz” i każdy post wymagają logowania do Meta.",
            "Autosync is off: the CMS stores nothing from Meta. “Sync now” and each Social Post need a Meta sign-in.",
          )}</p>`}
      ${autosyncWarning()}
    </section>`;
  }
}

customElements.define("folk-social", SocialView);
