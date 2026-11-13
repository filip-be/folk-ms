import { api, ApiError, type Platform, type PostDraft, type PostOutcome, type PostsView, type SocialPost } from "../api.ts";
import { reconnectBanner } from "../patterns.ts";
import { busy, fmtDateTime, html, recentSignInBanner, showError, toast } from "../ui.ts";
import { View } from "../view.ts";
import { L } from "../i18n.ts";

const PLATFORMS: Platform[] = ["facebook", "instagram"];
const NAME: Record<Platform, string> = { facebook: "Facebook", instagram: "Instagram" };
/** "on Facebook"; Polish uses the locative: "na Facebooku", "na Instagramie". */
const ON: Record<Platform, string> = {
  facebook: L("na Facebooku", "on Facebook"),
  instagram: L("na Instagramie", "on Instagram"),
};
const STATUS: Record<string, string> = { pending: L("wynik nieznany", "outcome unknown"), posted: L("opublikowano", "posted"), failed: L("błąd", "failed") };

/** Why a Post Request sent nothing to Meta (the callback's `refused`). */
function refusedMessage(reason: string, pageName: string): string {
  switch (reason) {
    case "cancelled":
      return L("Anulowano logowanie do Meta. Nic nie zostało opublikowane.", "The Meta sign-in was cancelled. Nothing was posted.");
    case "expired":
      return L("Prośba o publikację wygasła. Spróbuj ponownie.", "The posting request expired. Try again.");
    case "login":
      return L("Logowanie do Meta nie powiodło się. Nic nie zostało opublikowane.", "Signing in with Meta failed. Nothing was posted.");
    case "page":
      return L(
        `Twoje konto Meta nie ma uprawnień do publikowania na stronie „${pageName}”. Nic nie zostało opublikowane.`,
        `Your Meta account has no role on the Facebook page “${pageName}” that allows posting. Nothing was posted.`,
      );
    case "invalid":
      return L("Wydarzenie lub połączenie zmieniło się w międzyczasie. Nic nie zostało opublikowane.", "The event or the connection changed meanwhile. Nothing was posted.");
    case "limit":
      return L("Dzienny limit postów organizacji został wykorzystany. Nic nie zostało opublikowane.", "The organization's daily post limit is used up. Nothing was posted.");
    case "token":
      return L(
        "Facebook odrzucił zapisany token strony. Nic nie zostało opublikowane. Włącz Autosync ponownie na ekranie połączenia.",
        "Facebook refused the stored page token. Nothing was posted. Turn Autosync on again on the connection screen.",
      );
    default:
      return L("Publikacja nie powiodła się. Sprawdź historię przed kolejną próbą.", "Posting failed. Check the history before trying again.");
  }
}

/** What happened on one platform (the callback's `<platform>` parameter). */
function platformMessage(pl: Platform, result: string, igUsername: string): [string, "ok" | "error"] {
  switch (result) {
    case "posted":
      return [L(`Opublikowano ${ON[pl]}`, `Posted ${ON[pl]}`), "ok"];
    case "failed":
      return [L(`Publikacja ${ON[pl]} nie powiodła się – zobacz historię.`, `Posting ${ON[pl]} failed – see the history.`), "error"];
    case "instagram":
      return [
        L(`Twoje konto Meta nie ma dostępu do konta Instagram @${igUsername}. Nie opublikowano ${ON[pl]}.`, `Your Meta account has no access to the Instagram account @${igUsername}. Not posted ${ON[pl]}.`),
        "error",
      ];
    case "quota":
      return [L(`Wyczerpano dzienny limit publikacji na Instagramie. Nie opublikowano ${ON[pl]}.`, `The daily Instagram publishing limit is used up. Not posted ${ON[pl]}.`), "error"];
    case "limit":
      return [L(`Dzienny limit postów organizacji został wykorzystany. Nie opublikowano ${ON[pl]}.`, `The organization's daily post limit is used up. Not posted ${ON[pl]}.`), "error"];
    case "token":
      return [L(`Facebook odrzucił zapisany token strony. Nie opublikowano ${ON[pl]}.`, `Facebook refused the stored page token. Not posted ${ON[pl]}.`), "error"];
    case "confirm":
      return [
        L(`W międzyczasie pojawił się inny post ${ON[pl]}. Nie opublikowano – sprawdź historię i spróbuj ponownie.`, `Another post appeared ${ON[pl]} meanwhile. Not posted – check the history and try again.`),
        "error",
      ];
    default:
      return [L(`Nie opublikowano ${ON[pl]}.`, `Not posted ${ON[pl]}.`), "error"];
  }
}

class PostView extends View {
  private data!: PostsView;

  async load(): Promise<void> {
    this.data = await api.posts(this.org, this.params.id!);
    this.render();
    this.showOutcome();
    this.on("#post", "click", (_e, el) => void this.post(el as HTMLButtonElement));
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

  /** Shows what the Meta callback reported, then drops it from the address. */
  private showOutcome(): void {
    const q = new URLSearchParams(location.search);
    const refused = q.get("refused") ?? undefined;
    const results = PLATFORMS.flatMap((pl) => {
      const r = q.get(pl);
      return r ? [{ platform: pl, status: r }] : [];
    });
    if (!refused && !results.length) return;
    this.toastOutcome({ refused, results });
    history.replaceState(null, "", location.pathname);
  }

  /** One toast for the whole outcome, so those of both platforms do not cover each other. */
  private toastOutcome(out: PostOutcome): void {
    const conn = this.data.connection;
    const msgs: [string, "ok" | "error"][] = out.refused ? [[refusedMessage(out.refused, conn.pageName ?? ""), "error"]] : [];
    for (const r of out.results ?? []) msgs.push(platformMessage(r.platform, r.reason || r.status, conn.igUsername ?? ""));
    if (!msgs.length) return;
    toast(msgs.map(([m]) => m).join("\n"), msgs.some(([, k]) => k === "error") ? "error" : "ok");
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

  private available(pl: Platform): boolean {
    const d = this.data;
    return d.connection.connected && !!d.images[pl] && (pl !== "instagram" || d.canInstagram);
  }

  private render(): void {
    const d = this.data;
    const back = html`<a href="/app/orgs/${this.org}/events/${this.params.id}" class="muted small">${L("← Wróć do wydarzenia", "← Back to the event")}</a>`;
    if (!d.postable) {
      this.set(html`${back}<div class="card"><h1>${L("Ogłoś w mediach", "Announce on social media")}</h1><p>${L(
        "Ukrytych wydarzeń, wydarzeń w koszu i wydarzeń bez terminów nie można ogłaszać, bo post zawiera link do strony wydarzenia, której nie ma na stronie internetowej.",
        "Hidden events, events in the Bin and events without dates cannot be announced: the post links to the event's page, which is not on the Site.",
      )}</p></div>`);
      return;
    }
    const conn = d.connection;
    // With the stored token, posting needs a Recent sign-in (decision 13).
    const guarded = d.storedToken && !conn.recentSignIn;
    const limitReached = d.postsToday >= d.dailyLimit;
    const canPost = PLATFORMS.some((pl) => this.available(pl)) && !guarded && !limitReached && !conn.autosync?.needsReconnect;
    this.set(html`${back}
      <h1>${L("Ogłoś w mediach", "Announce on social media")}</h1>
      ${guarded ? recentSignInBanner(true) : ""}
      ${d.storedToken && conn.autosync?.needsReconnect ? reconnectBanner(html` <a href="/app/orgs/${this.org}/social">${L("Przejdź do połączenia", "Go to the connection")}</a>`) : ""}
      <div id="post-problems"></div>
      ${!conn.connected
        ? html`<div class="banner warn">${L("Organizacja nie ma połączonej strony.", "The organization has no connected Facebook page.")} <a href="/app/orgs/${this.org}/social">${L("Połącz Facebook i Instagram", "Connect Facebook and Instagram")}</a></div>`
        : html`<p class="muted">${L("Strona", "Facebook page")}: <strong>${conn.pageName}</strong>${conn.igUsername ? html` · Instagram: <strong>@${conn.igUsername}</strong>` : ""}</p>`}
      <div class="post-grid">${PLATFORMS.map((pl) => {
        const hist = this.history(pl);
        const last = hist[0];
        const img = d.images[pl];
        const ok = this.available(pl);
        // Preselect a platform that can be posted and has no post yet.
        const checked = ok && !hist.some((p) => p.status === "posted");
        return html`<section class="card post-card">
          <h2><label><input type="checkbox" data-include="${pl}" ${checked ? "checked" : ""} ${ok ? "" : "disabled"}> ${NAME[pl]}</label></h2>
          ${img ? html`<img class="post-image ${pl}" src="${img}" alt="${L("Obraz posta", "Post image")}">` : html`<p class="muted">${L("Brak obrazu – dodaj zdjęcie główne (na Facebooku albo nadpisując je w CMS).", "No image – add a cover image (on Facebook, or override it in the CMS).")}</p>`}
          <div class="field"><label for="caption-${pl}">${L("Treść posta", "Post text")}</label>
            <textarea id="caption-${pl}" data-platform="${pl}" rows="10" ${ok ? "" : "readonly"}>${last?.caption ?? d.captions.pl}</textarea>
            <div class="row"><span class="muted small" id="count-${pl}"></span>
              <span><button type="button" class="link" data-platform="${pl}" data-caption-lang="pl" ${ok ? "" : "disabled"}>${L("wygeneruj PL", "generate PL")}</button>
              <button type="button" class="link" data-platform="${pl}" data-caption-lang="en" ${ok ? "" : "disabled"}>${L("wygeneruj EN", "generate EN")}</button></span></div>
          </div>
          ${pl === "instagram" ? html`<p class="muted small">${L("Linki w opisie na Instagramie nie są klikalne.", "Links in an Instagram caption are not clickable.")}</p>` : ""}
          ${hist.length
            ? html`<h3>${L("Historia", "History")}</h3><ul class="history">${hist.map(
                (p) => html`<li><span class="badge ${p.status}">${STATUS[p.status]}</span> ${fmtDateTime(p.attemptedAt)}
                  ${p.url ? html` · <a href="${p.url}" target="_blank" rel="noopener">${L("zobacz post", "view post")}</a>` : ""}
                  ${p.error ? html`<div class="error small">${p.error}</div>` : ""}</li>`,
              )}</ul>`
            : ""}
        </section>`;
      })}</div>
      <section class="card">
        <button type="button" id="post" class="primary" ${canPost ? "" : "disabled"}>${d.storedToken ? L("Opublikuj", "Post") : L("Zaloguj się do Meta i opublikuj", "Sign in with Meta and post")}</button>
        <p class="small ${limitReached ? "error" : "muted"}">${L(`Dziś organizacja opublikowała ${d.postsToday} z ${d.dailyLimit} dozwolonych postów.`, `Posts today: ${d.postsToday} of the organization's daily limit of ${d.dailyLimit}.`)}</p>
        <p class="muted small">${d.storedToken
          ? L(
              "Autosync jest włączony: CMS publikuje z zapisanym tokenem strony, bez logowania do Meta. Wymaga to logowania do CMS w ciągu ostatnich 15 minut.",
              "Autosync is on: the CMS posts with the stored page token, without a Meta sign-in. This needs a sign-in to the CMS within the last 15 minutes.",
            )
          : L(
              "Za każdym razem logujesz się do Meta (Facebook): CMS nie przechowuje dostępu do strony. Twoje konto Meta musi mieć uprawnienia do publikowania na stronie i dostęp do jej konta Instagram, a administrator CMS musi dodać Cię jako testera aplikacji Meta.",
              "You sign in with Meta (Facebook) every time: the CMS keeps no access to the Facebook page. Your Meta account needs a role on the Facebook page that allows posting and access to its Instagram account, and the CMS administrator must add you as a tester of the Meta app.",
            )}</p>
      </section>`);
    for (const pl of PLATFORMS) this.counter(pl);
  }

  private async post(btn: HTMLButtonElement): Promise<void> {
    const drafts: PostDraft[] = PLATFORMS.filter((pl) => this.$<HTMLInputElement>(`[data-include="${pl}"]`).checked).map((pl) => ({
      platform: pl,
      caption: this.$<HTMLTextAreaElement>(`#caption-${pl}`).value,
    }));
    if (!drafts.length) {
      toast(L("Zaznacz Facebook lub Instagram.", "Select Facebook or Instagram."), "error");
      return;
    }
    await busy(btn, async () => {
      let confirmRetry = false;
      for (;;) {
        try {
          const res = await api.post(this.org, this.params.id!, drafts, confirmRetry);
          if (res.url) {
            location.href = res.url; // the Meta sign-in; its callback posts and comes back here
            return;
          }
          // Autosync on: posted at once with the stored token.
          if (res.outcome) this.toastOutcome(res.outcome);
          this.data = await api.posts(this.org, this.params.id!);
          this.render();
          return;
        } catch (err) {
          if (err instanceof ApiError && err.code === "needs_confirmation" && !confirmRetry) {
            const msgs = drafts.flatMap(({ platform: pl }) => {
              const last = this.history(pl)[0];
              if (!last) return [];
              return [
                last.status === "pending"
                  ? L(`Poprzednia próba publikacji ${ON[pl]} ma nieznany wynik. Sprawdź ${ON[pl]}, czy post się nie pojawił.`, `The previous attempt to post ${ON[pl]} has an unknown outcome. Check ${ON[pl]} that the post did not appear.`)
                  : last.status === "posted"
                    ? L(`To wydarzenie zostało już opublikowane ${ON[pl]}.`, `This event was already posted ${ON[pl]}.`)
                    : L(`Poprzednia próba ${ON[pl]} się nie udała. Upewnij się, że post się nie pojawił.`, `The previous attempt ${ON[pl]} failed. Make sure the post did not appear.`),
              ];
            });
            if (!confirm([...msgs, L("Opublikować ponownie?", "Post again?")].join("\n\n"))) return;
            confirmRetry = true;
            continue;
          }
          showError(err, this.querySelector<HTMLElement>("#post-problems"));
          return;
        }
      }
    });
  }
}

customElements.define("folk-post", PostView);
