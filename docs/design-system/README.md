# Design system

The visual language of the admin SPA: colour scales, the Figtree type scale, a 4px spacing grid, components, and an app shell with a collapsible sidebar that becomes a drawer on phones. Moderators use the CMS on desktops and phones alike, so every pattern is defined for both. Sites keep their own designs and do not use it.

## Credits

The design system is based on the work of **Felipe Cardona**: his colour, typography and "Add new user" admin shots on Dribbble — <https://dribbble.com/felipecardona/>. The colour scales, the type scale with its four weights, the spacing and the collapsible left-hand menu with its tree of sub-items follow those designs; the tokens, the dark theme, the mobile drawer, the components and the colouring of buttons by role are folk-ms's adaptation.

The typeface is [Figtree](https://github.com/erikdkennedy/figtree) by Erik Kennedy, under the SIL Open Font License 1.1 ([`fonts/OFL.txt`](../../cms/admin/src/fonts/OFL.txt)).

## How it works

Everything is native CSS with custom properties (no Sass, [ADR 0003](../architecture/adr/0003-stdlib-only-go.md)), in two layers:

1. **Tokens** ([`tokens.css`](../../cms/admin/src/tokens.css)) define every colour, size, font, radius, shadow and z-index once. Rules use `var(--…)` only; `scripts/styles.test.ts` fails the build on a literal (see the CSS section of [AGENTS.md](../../AGENTS.md)).
2. **Rules** ([`app.css`](../../cms/admin/src/app.css)) style plain elements and a few classes (`.card`, `.toolbar`, `.badge`, …), so views write semantic HTML and get the design for free.

A Super Admin can see the whole system live at `/app/design` (Administration → Design system), drawn from the same tokens in light or dark mode.

### Colour

Six scales, each from 25 (lightest) to 900 (darkest): **neutral** (white, 100–950, black), **primary** (blue), **secondary** (violet), **error**, **success** and **warning**. Rules never use a scale step directly; they use semantic tokens written as `light-dark(light, dark)`, which follow `color-scheme`: the system's by default, or the Moderator's choice from the theme switch in the sidebar (System → Light → Dark, kept per browser in `localStorage`, `folkms.theme`, and applied as `html[data-theme]`).

Surfaces are layered one step apart so that each plane reads on its own: **page** at the back, then the **sidebar** and **cards**, then **fields**, which sit inset (darker) in dark mode.

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--page` | neutral-100 | black | Page background |
| `--sidebar`, `--surface` | white | neutral-950 | Sidebar and top bar; cards, menus, buttons |
| `--field` | white | black | Inputs, selects, text areas |
| `--bg` | neutral-100 | neutral-900 | Tinted areas inside a surface: table heads |
| `--surface-sunken` | neutral-200 | neutral-850 | Hover, active nav item, segmented controls |
| `--text` / `--text-secondary` / `--muted` | black / neutral-800 / neutral-700 | neutral-100 / 300 / 500 | Body, labels, secondary text |
| `--line` / `--line-subtle` | neutral-300 / 200 | neutral-800 / 850 | Borders, row dividers |
| `--action` (+ `-hover`, `-text`) | primary-600 | primary-300 | Primary buttons, checked controls, active tab |
| `--action-secondary` (+ `-hover`, `-text`, `-line`) | secondary-50, text 700 | secondary-900, text 200 | Secondary buttons (tonal) |
| `--danger-line`, `--danger-hover` | error-200 / 50 | error-700 / 900 | Danger buttons' border and hover |
| `--disabled-bg` / `--disabled-text` / `--disabled-line` | neutral-200 / 500 / 300 | neutral-900 / 600 / 800 | Disabled buttons, disabled and read-only fields |
| `--accent` | primary-600 | primary-300 | Links, focus rings |
| `--brand` | secondary-500 | secondary-400 | The folk-ms mark |
| `--ok`, `--warn`, `--danger`, `--info` (+ `-soft`) | 700/600 on 50 | 300 on 900 | Badges, banners, toasts |

Buttons take the colour of their role (see [Buttons](#buttons)): primary blue, secondary violet, neutral white, danger red. A disabled control loses its role's colour and turns grey.

### Typography

Figtree, self-hosted (the CMS's Content-Security-Policy allows fonts from `'self'` only) in a Latin and a Latin Extended subset for Polish. Sizes are paired with line heights, in rem so that they follow the user's font size:

| Token | px | Use |
| --- | --- | --- |
| `--headline-xl` … `--headline-xs` | 64/72, 56/64, 48/56, 40/48, 32/40 | `h1` uses XS; larger sizes are for marketing-style screens |
| `--text-xl` | 24/32 | `h1` on phones |
| `--text-l` | 20/24 | `h2`, the brand |
| `--text-m` | 16/24 | `h3`; body and inputs on phones |
| `--text-s` | 14/20 | Body, buttons, labels |
| `--text-xs` | 12/16 | Captions, badges, table heads |

Weights: `--weight-regular` 400, `--weight-medium` 500, `--weight-semibold` 600, `--weight-bold` 700. Headlines use `--tracking-headline` (−2%).

### Spacing, shape and depth

`--space-N` is N px at the default 16px root (written in rem): 2, 4, 6, 8, 10, 12, 16, 20, 24, 32, 40, 48, 64. Radii: 4, 6, 8 (buttons, inputs, nav items), 12 (cards, tables), 16 (the drawer), pill. Shadows: `xs` on controls and cards, `sm` on a selected segment, `md` on menus, `lg` on the drawer and toasts. Controls are `--control-height` (40px) tall, `--touch-target` (44px) on phones.

### App shell

The shell ([`shell.ts`](../../cms/admin/src/shell.ts)) is a sidebar beside the page: the brand and collapse button, the Organization switcher, the sections with line icons ([`icons.ts`](../../cms/admin/src/icons.ts)), a collapsible **Administration** group whose items hang off a tree line, and at the bottom the signed-in user, the theme switch, the language switch and Sign out.

- **Wide screens (768px and up).** The sidebar is sticky and full height. The collapse button folds it to a 72px icon rail; labels stay in the DOM for screen readers and show as tooltips. The choice is kept per browser (`localStorage`, `folkms.nav`), as is a folded Administration group (`folkms.nav.admin`).
- **Narrow screens (below 768px).** A top bar with a menu button and the brand replaces the sidebar, which becomes an off-canvas drawer over a scrim. Opening it moves focus to the current page's item (or the first item) and makes the page inert; Escape, the close button or the scrim closes it and returns focus to the menu button. The rail is never used on phones.
- **Navigation.** Every client-side navigation (a link, the Organization switcher, Back/Forward) re-renders the shell, which also closes the drawer, and then moves focus to the new page's `main#view` (`tabindex="-1"`), so keyboard and screen-reader users start at the new content rather than at the top of the document. The first load leaves focus alone.
- **Phones (640px and below).** Headlines drop to `--text-xl`, inputs to 16px (which stops iOS from zooming into a focused field), controls grow to 44px, the sticky editor toolbar scrolls away, and tables hide their third column. The shell pads for the notch and home indicator with the `--safe-*` tokens (`viewport-fit=cover`) at every width — the top bar, drawer and page on narrow screens, the sidebar and page on wide ones (a phone held sideways) — and `theme-color` tints the browser chrome to match.

### Components

Focused fields get one 2px accent border (`--shadow-focus` thickens the 1px border), never a second ring. Selects draw their chevron with CSS gradients (the CSP allows no `data:` images); with a mouse, browsers that support customizable selects (`appearance: base-select`) also get a styled option list, while touch screens keep the native picker.

Buttons (see below), fields (see [Forms](#forms); `.chip`, `label.inline`), `.tabs`, segmented `.lang-tabs`, `table.list` (always inside a `.table-wrap`, which draws the rounded box and scrolls a table wider than the page sideways), `.badge` (neutral, `info`, ok/warn/danger by status), `.banner`, `.problems`, `.card`, `.toolbar`, `details.more` menus and `.toast`. Each is shown on `/app/design`.

### Buttons

Every button has exactly one role, picked by what it does, not by how prominent it should look:

| Role | Class | Look | Use |
| --- | --- | --- | --- |
| Primary | `.primary` | Blue fill (`--action`) | The one main action of a screen or form: Publish, Save, Invite, Add venue. At most one per form or toolbar. |
| Secondary | `.secondary` | Violet tonal (`--action-secondary`) | A further positive action beside the primary one (Save draft next to Publish), or a repeated action in a list (Choose for each Facebook page). |
| Neutral | none | White, grey border | Cancel, Edit, More, navigation (`a.button` to another screen), file pickers. |
| Danger | `.danger` | Red text and border | Removes, revokes or disconnects. Inside a `details.more` menu it is red text only. |
| Link | `.link` (+ `.danger`) | Underlined text | Small inline actions: Copy, + New venue, Revoke. |
| Icon | `.icon-button` | Icon only, with `aria-label` | Toolbar and shell controls. |

**Disabled** (`disabled`; for an `a.button`, `aria-disabled="true"`; a `label.button` around a disabled file input) is the same for every role: grey fill `--disabled-bg`, grey text, no shadow, `cursor: not-allowed`. `busy()` in `ui.ts` disables a button while its request runs, so the look also says "working".

### Forms

- **Labels.** A field is `<div class="field"><label for=…>…</label><input id=…>…</div>`. Use the `for` pattern, not a label wrapped around the control: the required mark and the error message are placed relative to it.
- **Required.** Give the control `required`. A `.field` whose label precedes a required control shows a red asterisk after the label automatically; for a group (checkboxes in a `fieldset`) or any other label, add `class="required"` to the `legend` or `label`. A form with required fields shows a `<p class="required-note">` key ("required", or "required to publish" in the event editor, which is `novalidate` so a Draft saves without them while publishing enforces them on the server).
- **Validation errors** appear right below their field as `.field-error` (alert icon and message), with `aria-invalid` and `aria-describedby` on the control and a red border; editing the field clears it. `ui.ts` does this for both sources:
  - The browser's checks (`required`, `type=email|url`, `maxlength`) on a form without `novalidate`: the native bubble is replaced by the same inline message, in the admin UI language.
  - Problems from the API: `showError(err, summary, form)` calls `markProblems`, which matches each problem's `field` path to a control by `data-field` (space-separated paths) or `name`, falling back to the Polish variant (`title` → `title.pl`). A `data-field` on a `fieldset` marks the group. Given a `summary` element it also lists every problem, each linked to its field (a field in the other language tab is revealed via the `folk:reveal` event). A problem with no matching control is listed only in the summary, so add a `data-field` when a new problem path appears.
- **Disabled and read-only.** A field the Moderator cannot change is grey (`--disabled-bg`) and flat. Prefer `readonly` for text a Moderator may still want to copy; use `disabled` for selects, checkboxes, radios and buttons. A `label` or `.chip` around a disabled checkbox is greyed (a chip also gets a dashed border). A whole screen that cannot be edited (an Event in the bin) says so in a `.banner` and makes its controls read-only or disabled.

## Where it lives

| What | Path |
| --- | --- |
| Tokens and the font faces | [`cms/admin/src/tokens.css`](../../cms/admin/src/tokens.css) |
| Rules, the shell layout and breakpoints | [`cms/admin/src/app.css`](../../cms/admin/src/app.css) |
| Shell markup and behaviour | [`cms/admin/src/shell.ts`](../../cms/admin/src/shell.ts) |
| Icons | [`cms/admin/src/icons.ts`](../../cms/admin/src/icons.ts) |
| Validation (`markProblems`, `showError`, native checks) | [`cms/admin/src/ui.ts`](../../cms/admin/src/ui.ts) |
| Live reference (`/app/design`) | [`cms/admin/src/views/design.ts`](../../cms/admin/src/views/design.ts) |
| Figtree and its licence | [`cms/admin/src/fonts/`](../../cms/admin/src/fonts/) |
| Token check | [`scripts/styles.test.ts`](../../scripts/styles.test.ts) |

## Change together

| Rule | Files | What breaks |
| --- | --- | --- |
| The drawer breakpoint is 767px in both places | `app.css` (`max-width: 767px`), `shell.ts` (`narrow`) | Resizing across it leaves the page inert or the drawer open |
| `theme-color` matches `--sidebar` | `index.html`, `tokens.css` | The phone's browser chrome no longer matches the page |
| A validation problem's `field` path has a matching `data-field` or `name` | `cms/internal/domain/*.go` (`p.add`, `Problem`), the view's form | The problem is listed only in the summary, not next to its field |
| Fonts are served from `/app/fonts/` | `tokens.css` (`@font-face`), `build.ts` (copies `src/fonts/`, keeps the URLs external), `index.html` (preload) | Text falls back to the system font |

## Read next

- [Access](../access/README.md): the admin UI language and who sees the Administration group
- [Platform](../platform/README.md): how the admin SPA is built and served
- [ADR 0003](../architecture/adr/0003-stdlib-only-go.md): why native CSS without new build dependencies
