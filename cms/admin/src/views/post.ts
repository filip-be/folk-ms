import { api, ApiError, type Platform, type PostsView, type SocialPost } from "../api.ts";
import { busy, fmtDateTime, html, showError, t, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

const NAME: Record<Platform, string> = { facebook: "Facebook", instagram: "Instagram" };
/** "on Facebook"; Polish uses the locative: "na Facebooku", "na Instagramie". */
const ON: Record<Platform, string> = {
  facebook: L("na Facebooku", "on Facebook"),
  instagram: L("na Instagramie", "on Instagram"),
};
const STATUS: Record<string, string> = { pending: L("wynik nieznany", "outcome unknown"), posted: L("opublikowano", "posted"), failed: L("błąd", "failed") };

class PostView extends View {
  private data!: PostsView;

  async load(): Promise<void> {
    this.data = await api.posts(this.org, this.params.id!);
    this.render();
    this.on("[data-post]", "click", (_e, el) => void this.post(el.dataset.post as Platform, el as HTMLButtonElement));
    this.on("[data-caption-lang]", "click", (_e, el) => {
      const pl = el.dataset.platform as Platform;
      const ta = this.$<HTMLTextAreaElement>(`#caption-${pl}`);
      if (ta.value.trim() && !confirm(L("Zastąpić obecną treść wygenerowaną?", "Replace the current text with the generated one?"))) return;
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
    const back = html`<a href="/app/orgs/${this.org}/events/${this.params.id}" class="muted small">${L("← Wróć do wydarzenia", "← Back to the event")}</a>`;
    if (!d.published) {
      this.set(html`${back}<div class="card"><h1>${L("Ogłoś w mediach", "Announce on social media")}</h1><p>${L("Ogłaszać można tylko opublikowane wydarzenia, bo post zawiera link do strony wydarzenia.", "Only published events can be announced, because the post links to the event's page.")}</p></div>`);
      return;
    }
    const conn = d.connection;
    this.set(html`${back}
      <h1>${L("Ogłoś w mediach", "Announce on social media")}</h1>
      ${!conn.connected
        ? html`<div class="banner warn">${L("Organizacja nie ma połączonego konta.", "The organization has no connected account.")} <a href="/app/orgs/${this.org}/social">${L("Połącz Facebook i Instagram", "Connect Facebook and Instagram")}</a></div>`
        : html`<p class="muted">${L("Strona", "Page")}: <strong>${conn.pageName}</strong>${conn.igUsername ? html` · Instagram: <strong>@${conn.igUsername}</strong>` : ""}</p>`}
      ${conn.needsReconnect ? html`<div class="banner warn">${t(conn.reconnectNote)}</div>` : ""}
      <div class="post-grid">${(["facebook", "instagram"] as Platform[]).map((pl) => {
        const hist = this.history(pl);
        const last = hist[0];
        const img = d.images[pl];
        const disabled = !conn.connected || !img || (pl === "instagram" && !d.canInstagram);
        return html`<section class="card post-card">
          <h2>${NAME[pl]}</h2>
          ${img ? html`<img class="post-image ${pl}" src="${img}" alt="${L("Obraz posta", "Post image")}">` : html`<p class="muted">${L("Brak obrazu – dodaj zdjęcie główne i opublikuj wydarzenie ponownie.", "No image – add a cover image and publish the event again.")}</p>`}
          <div class="field"><label for="caption-${pl}">${L("Treść posta", "Post text")}</label>
            <textarea id="caption-${pl}" data-platform="${pl}" rows="10">${last?.caption ?? d.captions.pl}</textarea>
            <div class="row"><span class="muted small" id="count-${pl}"></span>
              <span><button type="button" class="link" data-platform="${pl}" data-caption-lang="pl">${L("wygeneruj PL", "generate PL")}</button>
              <button type="button" class="link" data-platform="${pl}" data-caption-lang="en">${L("wygeneruj EN", "generate EN")}</button></span></div>
          </div>
          ${pl === "instagram" ? html`<p class="muted small">${L("Linki w opisie na Instagramie nie są klikalne.", "Links in an Instagram caption are not clickable.")}</p>` : ""}
          <button type="button" class="primary" data-post="${pl}" ${disabled ? "disabled" : ""}>${L(`Opublikuj ${ON[pl]}`, `Post ${ON[pl]}`)}</button>
          ${hist.length
            ? html`<h3>${L("Historia", "History")}</h3><ul class="history">${hist.map(
                (p) => html`<li><span class="badge ${p.status}">${STATUS[p.status]}</span> ${fmtDateTime(p.attemptedAt)}
                  ${p.url ? html` · <a href="${p.url}" target="_blank" rel="noopener">${L("zobacz post", "view post")}</a>` : ""}
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
          toast(
            res.status === "posted" ? L(`Opublikowano ${ON[pl]}`, `Posted ${ON[pl]}`) : L(`Nie udało się: ${res.error ?? ""}`, `Failed: ${res.error ?? ""}`),
            res.status === "posted" ? "ok" : "error",
          );
          break;
        } catch (err) {
          if (err instanceof ApiError && err.code === "needs_confirmation" && !confirmRetry) {
            const last = this.history(pl)[0];
            const msg =
              last?.status === "pending"
                ? L(
                    `Poprzednia próba publikacji ${ON[pl]} ma nieznany wynik. Sprawdź ${ON[pl]}, czy post się nie pojawił. Opublikować ponownie?`,
                    `The previous attempt to post ${ON[pl]} has an unknown outcome. Check ${ON[pl]} that the post did not appear. Post again?`,
                  )
                : last?.status === "posted"
                  ? L(`To wydarzenie zostało już opublikowane ${ON[pl]}. Opublikować kolejny post?`, `This event was already posted ${ON[pl]}. Post again?`)
                  : L(
                      `Poprzednia próba się nie udała. Upewnij się, że post nie pojawił się ${ON[pl]}. Spróbować ponownie?`,
                      `The previous attempt failed. Make sure the post did not appear ${ON[pl]}. Try again?`,
                    );
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
