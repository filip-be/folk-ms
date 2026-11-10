// The status panel Autofill shows on Facebook: each field as filled, needs
// you or failed, with copy buttons, the "act as the Page" reminder and,
// once the Moderator pressed Create, the way back to the CMS. It lives in a
// closed shadow root so Facebook's styles and scripts leave it alone.

import type { FieldName } from "./fieldmap.ts";
import type { FieldStatus, Handoff } from "./protocol.ts";
import CSS from "./panel.css" with { type: "text" };

type Lang = "pl" | "en";
const T = {
  title: { pl: "folk-ms Autofill", en: "folk-ms Autofill" },
  filling: { pl: "Wypełniam formularz…", en: "Filling the form…" },
  focus: { pl: "Kliknij gdziekolwiek na tej stronie, aby kontynuować.", en: "Click anywhere on this page to continue." },
  waiting: { pl: "Czekam na formularz Facebooka…", en: "Waiting for Facebook's form…" },
  noForm: { pl: "Nie znaleziono formularza. Wypełnij go ręcznie przyciskami „Kopiuj”.", en: "The form was not found. Fill it by hand with the Copy buttons." },
  done: { pl: "Sprawdź formularz i kliknij „Utwórz wydarzenie”. Autofill nigdy nie tworzy wydarzenia sam.", en: "Review the form and press “Create event”. Autofill never creates the event itself." },
  asPage: { pl: "Upewnij się, że działasz jako strona", en: "Make sure you are acting as the Page" },
  hostWarn: { pl: "Gospodarzem nie wygląda na stronę", en: "The host does not look like the Page" },
  tz: { pl: "Strefa czasowa przeglądarki różni się od strefy organizacji", en: "Your browser's time zone differs from the organization's" },
  created: { pl: "Utworzono wydarzenie na Facebooku:", en: "Facebook event created:" },
  back: { pl: "Wróć do CMS", en: "Back to the CMS" },
  copy: { pl: "Kopiuj", en: "Copy" },
  copied: { pl: "Skopiowano", en: "Copied" },
  close: { pl: "Zamknij", en: "Close" },
  filled: { pl: "wypełnione", en: "filled" },
  "needs-you": { pl: "uzupełnij ręcznie", en: "needs you" },
  failed: { pl: "nie udało się", en: "failed" },
} satisfies Record<string, Record<Lang, string>>;

const FIELD_LABEL: Record<FieldName, Record<Lang, string>> = {
  cover: { pl: "Zdjęcie", en: "Cover photo" },
  name: { pl: "Nazwa", en: "Name" },
  startDate: { pl: "Data rozpoczęcia", en: "Start date" },
  startTime: { pl: "Godzina rozpoczęcia", en: "Start time" },
  timezone: { pl: "Strefa czasowa", en: "Time zone" },
  endDate: { pl: "Data zakończenia", en: "End date" },
  endTime: { pl: "Godzina zakończenia", en: "End time" },
  location: { pl: "Miejsce", en: "Location" },
  description: { pl: "Szczegóły", en: "Details" },
  ticketUrl: { pl: "Link do biletów", en: "Ticket URL" },
};


export interface PanelState {
  message: string;
  fields: Partial<Record<FieldName, FieldStatus>>;
  warnings: string[];
  createdUrl?: string;
}

export class Panel {
  private readonly host = document.createElement("div");
  private readonly root: ShadowRoot;
  private readonly lang: Lang;
  private state: PanelState = { message: "", fields: {}, warnings: [] };

  constructor(
    private readonly h: Handoff,
    private readonly values: Partial<Record<FieldName, string>>,
    private readonly onBack: () => void,
  ) {
    this.lang = h.lang === "pl" ? "pl" : "en";
    this.root = this.host.attachShadow({ mode: "closed" });
    document.documentElement.append(this.host);
    this.root.addEventListener("click", (e) => void this.click(e));
  }

  t(k: keyof typeof T): string {
    return T[k][this.lang];
  }

  update(s: Partial<PanelState>): void {
    this.state = { ...this.state, ...s };
    this.render();
  }

  private render(): void {
    const s = this.state;
    const esc = (v: string) => v.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
    const rows = (Object.keys(s.fields) as FieldName[]).map((f) => {
      const st = s.fields[f]!;
      const copy = st !== "filled" && f !== "cover" && this.values[f] ? `<button data-copy="${f}">${this.t("copy")}</button>` : "";
      return `<li><span class="n">${esc(FIELD_LABEL[f][this.lang])}</span><span class="s ${st}">${esc(this.t(st))}</span>${copy}</li>`;
    });
    this.root.innerHTML = `<style>${CSS}</style><div class="p" role="status">
      <h2>${this.t("title")}<button class="x" data-close aria-label="${this.t("close")}">×</button></h2>
      <p class="m">${esc(this.t("asPage"))}: <strong>${esc(this.h.pageName)}</strong></p>
      ${s.warnings.map((w) => `<p class="m w">${esc(w)}</p>`).join("")}
      <p class="m">${esc(s.message)}</p>
      ${rows.length ? `<ul>${rows.join("")}</ul>` : ""}
      ${s.createdUrl ? `<p class="m">${this.t("created")}<br><a href="${esc(s.createdUrl)}" target="_blank" rel="noopener">${esc(s.createdUrl)}</a></p>
        <p class="m"><button class="primary" data-back>${this.t("back")}</button></p>` : ""}
    </div>`;
  }

  private async click(e: Event): Promise<void> {
    const el = (e.target as HTMLElement).closest<HTMLElement>("button");
    if (!el) return;
    if (el.dataset.close !== undefined) {
      this.host.remove();
      return;
    }
    if (el.dataset.back !== undefined) {
      this.onBack();
      return;
    }
    const f = el.dataset.copy as FieldName | undefined;
    if (f && this.values[f]) {
      await navigator.clipboard.writeText(this.values[f]!);
      el.textContent = this.t("copied");
    }
  }
}
