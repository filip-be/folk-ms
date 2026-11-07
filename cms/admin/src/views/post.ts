import { api, ApiError, type Platform, type PostsView, type SocialPost } from "../api.ts";
import { busy, fmtDateTime, html, showError, toast } from "../ui.ts";
import { View } from "../view.ts";

const NAME: Record<Platform, string> = { facebook: "Facebook", instagram: "Instagram" };
/** Polish locative: "na Facebooku", "na Instagramie". */
const ON: Record<Platform, string> = { facebook: "na Facebooku", instagram: "na Instagramie" };
const STATUS: Record<string, string> = { pending: "wynik nieznany", posted: "opublikowano", failed: "błąd" };

class PostView extends View {
  private data!: PostsView;

  async load(): Promise<void> {
    this.data = await api.posts(this.org, this.params.id!);
    this.render();
    this.on("[data-post]", "click", (_e, el) => void this.post(el.dataset.post as Platform, el as HTMLButtonElement));
    this.on("[data-caption-lang]", "click", (_e, el) => {
      const pl = el.dataset.platform as Platform;
      const ta = this.$<HTMLTextAreaElement>(`#caption-${pl}`);
      if (ta.value.trim() && !confirm("Zastąpić obecną treść wygenerowaną?")) return;
      ta.value = this.data.captions[el.dataset.captionLang as "pl" | "en"];
      this.counter(pl);
    });
    this.addEventListener("input", (e) => {
      const ta = e.target as HTMLTextAreaElement;
      if (ta.dataset.platform) this.counter(ta.dataset.platform as Platform);
    });
  }

  private counter(pl: Platform): void {
    const n = [...this.$<HTMLTextAreaElement>(`#caption-${pl}`).value].length;
    const el = this.$(`#count-${pl}`);
    el.textContent = `${n} / 2200`;
    el.classList.toggle("over", n > 2200);
  }

  private history(pl: Platform): SocialPost[] {
    return this.data.posts.filter((p) => p.platform === pl).reverse();
  }

  private render(): void {
    const d = this.data;
    const back = html`<a href="/app/orgs/${this.org}/events/${this.params.id}" class="muted small">← Wróć do wydarzenia</a>`;
    if (!d.published) {
      this.set(html`${back}<div class="card"><h1>Ogłoś w mediach</h1><p>Ogłaszać można tylko opublikowane wydarzenia, bo post zawiera link do strony wydarzenia.</p></div>`);
      return;
    }
    const conn = d.connection;
    this.set(html`${back}
      <h1>Ogłoś w mediach</h1>
      ${!conn.connected
        ? html`<div class="banner warn">Organizacja nie ma połączonego konta. <a href="/app/orgs/${this.org}/social">Połącz Facebook i Instagram</a></div>`
        : html`<p class="muted">Strona: <strong>${conn.pageName}</strong>${conn.igUsername ? html` · Instagram: <strong>@${conn.igUsername}</strong>` : ""}</p>`}
      ${conn.needsReconnect ? html`<div class="banner warn">${conn.reconnectNote}</div>` : ""}
      <div class="post-grid">${(["facebook", "instagram"] as Platform[]).map((pl) => {
        const hist = this.history(pl);
        const last = hist[0];
        const img = d.images[pl];
        const disabled = !conn.connected || !img || (pl === "instagram" && !d.canInstagram);
        return html`<section class="card post-card">
          <h2>${NAME[pl]}</h2>
          ${img ? html`<img class="post-image ${pl}" src="${img}" alt="Obraz posta">` : html`<p class="muted">Brak obrazu – dodaj zdjęcie główne i opublikuj wydarzenie ponownie.</p>`}
          <div class="field"><label for="caption-${pl}">Treść posta</label>
            <textarea id="caption-${pl}" data-platform="${pl}" rows="10">${last?.caption ?? d.captions.pl}</textarea>
            <div class="row"><span class="muted small" id="count-${pl}"></span>
              <span><button type="button" class="link" data-platform="${pl}" data-caption-lang="pl">wygeneruj PL</button>
              <button type="button" class="link" data-platform="${pl}" data-caption-lang="en">wygeneruj EN</button></span></div>
          </div>
          ${pl === "instagram" ? html`<p class="muted small">Linki w opisie na Instagramie nie są klikalne.</p>` : ""}
          <button type="button" class="primary" data-post="${pl}" ${disabled ? "disabled" : ""}>Opublikuj ${ON[pl]}</button>
          ${hist.length
            ? html`<h3>Historia</h3><ul class="history">${hist.map(
                (p) => html`<li><span class="badge ${p.status}">${STATUS[p.status]}</span> ${fmtDateTime(p.attemptedAt)}
                  ${p.url ? html` · <a href="${p.url}" target="_blank" rel="noopener">zobacz post</a>` : ""}
                  ${p.error ? html`<div class="error small">${p.error}</div>` : ""}</li>`,
              )}</ul>`
            : ""}
        </section>`;
      })}</div>`);
    for (const pl of ["facebook", "instagram"] as Platform[]) this.counter(pl);
  }

  private async post(pl: Platform, btn: HTMLButtonElement): Promise<void> {
    const caption = this.$<HTMLTextAreaElement>(`#caption-${pl}`).value;
    await busy(btn, async () => {
      let confirmRetry = false;
      for (;;) {
        try {
          const res = await api.post(this.org, this.params.id!, pl, caption, confirmRetry);
          toast(res.status === "posted" ? `Opublikowano ${ON[pl]}` : `Nie udało się: ${res.error ?? ""}`, res.status === "posted" ? "ok" : "error");
          break;
        } catch (err) {
          if (err instanceof ApiError && err.code === "needs_confirmation" && !confirmRetry) {
            const last = this.history(pl)[0];
            const msg =
              last?.status === "pending"
                ? `Poprzednia próba publikacji ${ON[pl]} ma nieznany wynik. Sprawdź ${ON[pl]}, czy post się nie pojawił. Opublikować ponownie?`
                : last?.status === "posted"
                  ? `To wydarzenie zostało już opublikowane ${ON[pl]}. Opublikować kolejny post?`
                  : `Poprzednia próba się nie udała. Upewnij się, że post nie pojawił się ${ON[pl]}. Spróbować ponownie?`;
            if (!confirm(msg)) return;
            confirmRetry = true;
            continue;
          }
          showError(err);
          break;
        }
      }
      this.data = await api.posts(this.org, this.params.id!);
      this.render();
    });
  }
}

customElements.define("folk-post", PostView);
