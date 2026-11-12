// Living reference of the design system (Super Admin only): palette, type
// scale, spacing and components, all drawn from tokens.css and app.css.
// The written reference is docs/design-system/README.md.

import { L } from "../i18n.ts";
import { icon, ICON_NAMES } from "../icons.ts";
import { html, setFieldError } from "../ui.ts";
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
      <div class="toolbar"><h1>${L("Design system", "Design system")}</h1></div>
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
        <p class="muted small">${L("Główny: jedno główne działanie formularza. Drugorzędny: kolejne działanie obok głównego. Neutralny: anuluj, edytuj, nawigacja. Niebezpieczny: usuwa lub odbiera.", "Primary: the one main action of a form. Secondary: a further action beside it. Neutral: cancel, edit, navigation. Danger: removes or revokes.")}</p>
        <div class="actions">
          <button class="primary">${L("Opublikuj", "Publish")}</button>
          <button class="secondary">${L("Zapisz szkic", "Save draft")}</button>
          <button>${L("Anuluj", "Cancel")}</button>
          <button class="danger">${L("Usuń", "Delete")}</button>
          <button class="link">${L("Link", "Link")}</button>
          <button class="icon-button" aria-label="${L("Dodaj", "Add")}">${icon("plus")}</button>
        </div>
        <p class="muted small">${L("Wyłączone – wyglądają tak samo bez względu na rolę:", "Disabled – the same whatever the role:")}</p>
        <div class="actions">
          <button class="primary" disabled>${L("Opublikuj", "Publish")}</button>
          <button class="secondary" disabled>${L("Zapisz szkic", "Save draft")}</button>
          <button disabled>${L("Anuluj", "Cancel")}</button>
          <button class="danger" disabled>${L("Usuń", "Delete")}</button>
          <button class="link" disabled>${L("Link", "Link")}</button>
        </div>
        <h3>${L("Pola", "Fields")}</h3>
        <p class="required-note">${L("wymagane", "required")}</p>
        <div class="grid">
          <div class="field"><label for="ds-name">${L("Nazwa", "Name")}</label><input id="ds-name" required placeholder="${L("Wpisz nazwę", "Enter a name")}"></div>
          <div class="field"><label for="ds-role">${L("Rola", "Role")}</label><select id="ds-role"><option>${L("Wybierz rolę", "Select role")}</option></select></div>
          <div class="field"><label for="ds-desc">${L("Opis", "Description")}</label><textarea id="ds-desc" rows="1"></textarea></div>
          <div class="field"><label for="ds-invalid">${L("Z błędem", "With an error")}</label><input id="ds-invalid" required type="email" value="anna@"></div>
          <div class="field"><label for="ds-readonly">${L("Tylko do odczytu", "Read-only")}</label><input id="ds-readonly" readonly value="${L("można skopiować", "can be copied")}"></div>
          <div class="field"><label for="ds-disabled">${L("Wyłączone", "Disabled")}</label><select id="ds-disabled" disabled><option>${L("Niedostępne", "Unavailable")}</option></select></div>
        </div>
        <fieldset data-field="ds-group"><legend class="required">${L("Grupa", "Group")}</legend>
          <label class="chip"><input type="checkbox" checked> ${L("Zaznaczony", "Checked")}</label>
          <label class="chip"><input type="checkbox"> ${L("Pusty", "Unchecked")}</label>
          <label class="chip"><input type="checkbox" checked disabled> ${L("Wyłączony", "Disabled")}</label>
          <label class="chip"><input type="checkbox" disabled> ${L("Wyłączony", "Disabled")}</label>
        </fieldset>
        <label class="inline"><input type="checkbox" checked> ${L("Pole wyboru", "Checkbox")}</label>
        <label class="inline"><input type="checkbox" checked disabled> ${L("Wyłączone", "Disabled")}</label>
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
    setFieldError(this.$("#ds-invalid"), L("podaj poprawny adres e-mail", "enter a valid email address"));
    this.querySelectorAll<HTMLElement>("[data-space]").forEach((el) => el.style.setProperty("--bar", `var(${el.dataset.space})`));
  }
}

customElements.define("folk-design", DesignView);
