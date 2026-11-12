import { api, type ConnectionStatus } from "../api.ts";
import { busy, fmtDate, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

class SocialView extends View {
  private st!: ConnectionStatus;

  async load(): Promise<void> {
    this.st = await api.social(this.org);
    const q = new URLSearchParams(location.search);
    if (q.get("error")) toast(q.get("error") === "cancelled" ? L("Anulowano logowanie do Meta.", "The Meta sign-in was cancelled.") : L("Nie udało się połączyć ze stroną.", "Connecting the Facebook page failed."), "error");
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
          toast(L("Połączono stronę", "Facebook page connected"));
          this.render();
        } catch (err) {
          showError(err);
        }
      }),
    );
    this.on("#disconnect", "click", (_e, el) => {
      if (!confirm(L("Rozłączyć stronę? Publikowanie postów nie będzie możliwe do ponownego połączenia.", "Disconnect the Facebook page? Posting will not be possible until it is connected again."))) return;
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
      <h1>${L("Social media", "Social media")}</h1>
      ${st.pendingPages?.length
        ? html`<section class="card"><h2>${L("Wybierz stronę na Facebooku", "Choose a Facebook page")}</h2>
            <ul class="pages">${st.pendingPages.map(
              (p) => html`<li><strong>${p.name}</strong> ${p.igUsername ? html`<span class="muted">· Instagram @${p.igUsername}</span>` : html`<span class="muted">${L("· bez Instagrama", "· no Instagram")}</span>`}
                <button type="button" class="primary" data-page="${p.id}">${L("Wybierz", "Choose")}</button></li>`,
            )}</ul></section>`
        : ""}
      <section class="card">
        ${st.connected
          ? html`<h2>${L("Połączono", "Connected")}</h2>
            <dl class="facts"><dt>${L("Strona", "Facebook page")}</dt><dd>${st.pageName}</dd>
              <dt>Instagram</dt><dd>${st.igUsername ? `@${st.igUsername}` : L("brak połączonego konta", "no connected account")}</dd>
              <dt>${L("Połączył(a)", "Connected by")}</dt><dd>${st.createdByEmail} · ${fmtDate(st.createdAt)}</dd></dl>
            <button type="button" id="connect">${L("Zmień stronę", "Change the Facebook page")}</button>
            <button type="button" id="disconnect" class="danger">${L("Rozłącz", "Disconnect")}</button>`
          : html`<h2>${L("Brak połączenia", "Not connected")}</h2>
            <p>${L("Wybierz stronę organizacji na Facebooku (i powiązane konto Instagram), aby ogłaszać opublikowane wydarzenia.", "Choose the organization's Facebook page (and its linked Instagram account) to announce published events.")}</p>
            <button type="button" id="connect" class="primary">${L("Wybierz stronę przez Meta", "Choose the Facebook page with Meta")}</button>`}
        <p class="muted small">${L(
          "CMS zapamiętuje tylko, która strona i które konto Instagram należą do organizacji – nie przechowuje dostępu do nich. Każdy moderator, który publikuje post, loguje się wtedy do Meta. Potrzebuje uprawnień do publikowania na stronie i dostępu do jej konta Instagram, a administrator CMS musi dodać go jako testera aplikacji Meta.",
          "The CMS remembers only which Facebook page and Instagram account belong to the organization – it keeps no access to them. Every moderator who posts signs in with Meta at that moment. They need a role on the Facebook page that allows posting and access to its Instagram account, and the CMS administrator must add them as a tester of the Meta app.",
        )}</p>
      </section>`);
  }
}

customElements.define("folk-social", SocialView);
