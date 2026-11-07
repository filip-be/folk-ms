import { api, type ConnectionStatus } from "../api.ts";
import { busy, fmtDate, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";

class SocialView extends View {
  private st!: ConnectionStatus;

  async load(): Promise<void> {
    this.st = await api.social(this.org);
    const q = new URLSearchParams(location.search);
    if (q.get("error")) toast(q.get("error") === "cancelled" ? "Anulowano logowanie do Facebooka." : "Nie udało się połączyć z Facebookiem.", "error");
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
          toast("Połączono konto");
          this.render();
        } catch (err) {
          showError(err);
        }
      }),
    );
    this.on("#disconnect", "click", (_e, el) => {
      if (!confirm("Rozłączyć konto? Publikowanie postów nie będzie możliwe do ponownego połączenia.")) return;
      void busy(el as HTMLButtonElement, async () => {
        try {
          await api.disconnect(this.org);
          this.st = await api.social(this.org);
          toast("Rozłączono");
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
      <h1>Media społecznościowe</h1>
      ${st.pendingPages?.length
        ? html`<section class="card"><h2>Wybierz stronę na Facebooku</h2>
            <ul class="pages">${st.pendingPages.map(
              (p) => html`<li><strong>${p.name}</strong> ${p.igUsername ? html`<span class="muted">· Instagram @${p.igUsername}</span>` : html`<span class="muted">· bez Instagrama</span>`}
                <button type="button" class="primary" data-page="${p.id}">Wybierz</button></li>`,
            )}</ul></section>`
        : ""}
      <section class="card">
        ${st.connected
          ? html`<h2>Połączono</h2>
            ${st.needsReconnect ? html`<div class="banner warn">${st.reconnectNote}</div>` : ""}
            <dl class="facts"><dt>Strona</dt><dd>${st.pageName}</dd>
              <dt>Instagram</dt><dd>${st.igUsername ? `@${st.igUsername}` : "brak połączonego konta"}</dd>
              <dt>Połączył(a)</dt><dd>${st.createdByEmail} · ${fmtDate(st.createdAt)}</dd></dl>
            <button type="button" id="connect">Połącz ponownie</button>
            <button type="button" id="disconnect" class="danger">Rozłącz</button>`
          : html`<h2>Brak połączenia</h2>
            <p>Połącz stronę organizacji na Facebooku (i powiązane konto Instagram), aby ogłaszać opublikowane wydarzenia.</p>
            <p class="muted small">Musisz administrować tą stroną, a administrator CMS musi dodać Cię jako testera aplikacji Meta.</p>
            <button type="button" id="connect" class="primary">Połącz z Facebookiem</button>`}
        <p class="muted small">Połączenie należy do organizacji – każdy jej moderator może je rozłączyć lub odnowić.</p>
      </section>`);
  }
}

customElements.define("folk-social", SocialView);
