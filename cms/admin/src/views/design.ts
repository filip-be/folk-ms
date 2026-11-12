// Living reference of the design system (Super Admin only): palette, type
// scale, spacing and components, all drawn from tokens.css and app.css.
// The written reference is docs/design-system/README.md.

import { L } from "../i18n.ts";
import { icon, ICON_NAMES } from "../icons.ts";
import { html } from "../ui.ts";
import { View } from "../view.ts";

const STEPS = ["25", "50", "100", "200", "300", "400", "500", "600", "700", "800", "900"];
const SCALES: [string, string[]][] = [
  ["neutral", ["white", ...["100", "200", "300", "400", "500", "600", "700", "800", "850", "900", "950"].map((s) => `neutral-${s}`), "black"]],
  ...["primary", "secondary", "error", "success", "warning"].map((n) => [n, STEPS.map((s) => `${n}-${s}`)] as [string, string[]]),
];
const TYPE = [
  ["headline-xl", "Headline XL", "64/72"],
  ["headline-l", "Headline L", "56/64"],
  ["headline-m", "Headline M", "48/56"],
  ["headline-s", "Headline S", "40/48"],
  ["headline-xs", "Headline XS", "32/40"],
  ["text-xl", "XL", "24/32"],
  ["text-l", "L", "20/24"],
  ["text-m", "M", "16/24"],
  ["text-s", "S", "14/20"],
  ["text-xs", "XS", "12/16"],
];
const WEIGHTS = [
  ["regular", "Regular"],
  ["medium", "Medium"],
  ["semibold", "Semibold"],
  ["bold", "Bold"],
];
const SPACES = [2, 4, 6, 8, 10, 12, 16, 20, 24, 32, 40, 48, 64];

class DesignView extends View {
  async load(): Promise<void> {
    this.set(html`
      <div class="toolbar"><h1>${L("System projektowy", "Design system")}</h1></div>
      <p class="muted">${L("Na podstawie projektu ", "Based on the design by ")}<a href="https://dribbble.com/felipecardona/" target="_blank" rel="noopener">Felipe Cardona</a>.
        ${L("Opis: docs/design-system/README.md.", "Reference: docs/design-system/README.md.")}</p>

      <section class="card">
        <h2>${L("Kolory", "Colors")}</h2>
        ${SCALES.map(
          ([name, tokens]) => html`<h3>${name[0]!.toUpperCase() + name.slice(1)}</h3>
            <ul class="swatches">${tokens.map((tk) => html`<li><span class="swatch" data-token="--${tk}"></span><span class="small">${tk}</span><code class="hex"></code></li>`)}</ul>`,
        )}
      </section>

      <section class="card">
        <h2>${L("Typografia", "Typography")}</h2>
        <p class="type-sample">Figtree — Aa Ąą Ćć Ęę Łł Ńń Óó Śś Źź Żż 0123456789</p>
        <div class="table-wrap"><table class="type-scale">
          <tbody>${TYPE.map(
            ([tk, name, metrics]) => html`<tr><th scope="row"><span class="small">${name}</span><span class="muted small">${metrics}</span></th>
              ${WEIGHTS.map(([w, wn]) => html`<td><span class="ts" data-size="${tk}" data-weight="${w}">${wn}</span></td>`)}</tr>`,
          )}</tbody>
        </table></div>
      </section>

      <section class="card">
        <h2>${L("Odstępy", "Spacing")}</h2>
        <ul class="spaces">${SPACES.map((n) => html`<li><span class="space-bar" data-space="--space-${n}"></span><code>--space-${n}</code></li>`)}</ul>
      </section>

      <section class="card">
        <h2>${L("Komponenty", "Components")}</h2>
        <h3>${L("Przyciski", "Buttons")}</h3>
        <div class="actions">
          <button class="primary">${L("Zapisz", "Save")}</button>
          <button>${L("Anuluj", "Cancel")}</button>
          <button class="danger">${L("Usuń", "Delete")}</button>
          <button class="link">${L("Link", "Link")}</button>
          <button disabled>${L("Wyłączony", "Disabled")}</button>
          <button class="icon-button" aria-label="${L("Dodaj", "Add")}">${icon("plus")}</button>
        </div>
        <h3>${L("Pola", "Fields")}</h3>
        <div class="grid">
          <div class="field"><label>${L("Nazwa", "Name")}<input placeholder="${L("Wpisz nazwę", "Enter a name")}"></label></div>
          <div class="field"><label>${L("Rola", "Role")}<select><option>${L("Wybierz rolę", "Select role")}</option></select></label></div>
          <div class="field"><label>${L("Opis", "Description")}<textarea rows="1"></textarea></label></div>
        </div>
        <label class="chip"><input type="checkbox" checked> ${L("Zaznaczony", "Checked")}</label>
        <label class="chip"><input type="checkbox"> ${L("Pusty", "Unchecked")}</label>
        <h3>${L("Plakietki", "Badges")}</h3>
        <p class="actions"><span class="badge">neutral</span><span class="badge info">info</span><span class="badge published">ok</span>
          <span class="badge warn">warn</span><span class="badge failed">danger</span></p>
        <h3>${L("Komunikaty", "Messages")}</h3>
        <div class="banner warn">${L("Baner ostrzeżenia.", "A warning banner.")}</div>
        <h3>${L("Ikony", "Icons")}</h3>
        <ul class="icons">${ICON_NAMES.map((n) => html`<li>${icon(n)}<span class="small muted">${n}</span></li>`)}</ul>
      </section>
    `);
    // CSP forbids style attributes in markup; the CSSOM is allowed.
    const root = getComputedStyle(document.documentElement);
    this.querySelectorAll<HTMLElement>("[data-token]").forEach((el) => {
      el.style.setProperty("--swatch", `var(${el.dataset.token})`);
      el.parentElement!.querySelector(".hex")!.textContent = root.getPropertyValue(el.dataset.token!).trim();
    });
    this.querySelectorAll<HTMLElement>(".ts").forEach((el) => {
      el.style.setProperty("--ts-size", `var(--${el.dataset.size})`);
      el.style.setProperty("--ts-leading", `var(--leading-${el.dataset.size})`);
      el.style.setProperty("--ts-weight", `var(--weight-${el.dataset.weight})`);
    });
    this.querySelectorAll<HTMLElement>("[data-space]").forEach((el) => el.style.setProperty("--bar", `var(${el.dataset.space})`));
  }
}

customElements.define("folk-design", DesignView);
