import { api, type ConnectionStatus } from "../api.ts";
import { busy, fmtDate, html, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

class SocialView extends View {
  private st!: ConnectionStatus;

  async load(): Promise<void> {
    this.st = await api.social(this.org);
    const q = new URLSearchParams(location.search);
    if (q.get("error")) toast(q.get("error") === "cancelled" ? L("Anulowano logowanie do Facebooka.", "Facebook sign-in was cancelled.") : L("Nie udało się połączyć z Facebookiem.", "Connecting to Facebook failed."), "error");
    this.render();
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
    this.on("[data-page]", "click", (_e, el) =>
      void busy(el as HTMLButtonElement, async () => {
        try {
          this.st = await api.selectPage(this.org, el.dataset.page!);
          history.replaceState(null, "", location.pathname);
          toast(L("Połączono konto", "Account connected"));
          this.render();
        } catch (err) {
          showError(err);
        }
      }),
    );
    this.on("#disconnect", "click", (_e, el) => {
      if (!confirm(L("Rozłączyć konto? Publikowanie postów nie będzie możliwe do ponownego połączenia.", "Disconnect the account? Posting will not be possible until it is connected again."))) return;
      void busy(el as HTMLButtonElement, async () => {
        try {
          await api.disconnect(this.org);
          this.st = await api.social(this.org);
          toast(L("Rozłączono", "Disconnected"));
          this.render();
        } catch (err) {
          showError(err);
        }
      });
    });
  }

  private render(): void {
    const st = this.st;
    this.set(html`
      <h1>${L("Media społecznościowe", "Social media")}</h1>
      ${st.pendingPages?.length
        ? html`<section class="card"><h2>${L("Wybierz stronę na Facebooku", "Choose a Facebook Page")}</h2>
            <ul class="pages">${st.pendingPages.map(
              (p) => html`<li><strong>${p.name}</strong> ${p.igUsername ? html`<span class="muted">· Instagram @${p.igUsername}</span>` : html`<span class="muted">${L("· bez Instagrama", "· no Instagram")}</span>`}
                <button type="button" class="primary" data-page="${p.id}">${L("Wybierz", "Choose")}</button></li>`,
            )}</ul></section>`
        : ""}
      <section class="card">
        ${st.connected
          ? html`<h2>${L("Połączono", "Connected")}</h2>
            ${st.needsReconnect ? html`<div class="banner warn">${t(st.reconnectNote)}</div>` : ""}
            <dl class="facts"><dt>${L("Strona", "Page")}</dt><dd>${st.pageName}</dd>
              <dt>Instagram</dt><dd>${st.igUsername ? `@${st.igUsername}` : L("brak połączonego konta", "no connected account")}</dd>
              <dt>${L("Połączył(a)", "Connected by")}</dt><dd>${st.createdByEmail} · ${fmtDate(st.createdAt)}</dd></dl>
            <button type="button" id="connect">${L("Połącz ponownie", "Reconnect")}</button>
            <button type="button" id="disconnect" class="danger">${L("Rozłącz", "Disconnect")}</button>`
          : html`<h2>${L("Brak połączenia", "Not connected")}</h2>
            <p>${L("Połącz stronę organizacji na Facebooku (i powiązane konto Instagram), aby ogłaszać opublikowane wydarzenia.", "Connect the organization's Facebook Page (and its linked Instagram account) to announce published events.")}</p>
            <p class="muted small">${L("Musisz administrować tą stroną, a administrator CMS musi dodać Cię jako testera aplikacji Meta.", "You must administer this Page, and the CMS administrator must add you as a tester of the Meta app.")}</p>
            <button type="button" id="connect" class="primary">${L("Połącz z Facebookiem", "Connect with Facebook")}</button>`}
        <p class="muted small">${L("Połączenie należy do organizacji – każdy jej moderator może je rozłączyć lub odnowić.", "The connection belongs to the organization – any of its moderators can disconnect or renew it.")}</p>
      </section>`);
  }
}

customElements.define("folk-social", SocialView);
