# Design system

The visual language of the admin SPA: colour scales, the Figtree type scale, a 4px spacing grid, components, and an app shell with a collapsible sidebar that becomes a drawer on phones. Moderators use the CMS on desktops and phones alike, so every pattern is defined for both. Sites keep their own designs and do not use it.

## Credits

The design system is based on the work of **Felipe Cardona**: his colour, typography and "Add new user" admin shots on Dribbble — <https://dribbble.com/felipecardona/>. The colour scales, the type scale with its four weights, the spacing, the monochrome controls and the collapsible left-hand menu with its tree of sub-items follow those designs; the tokens, the dark theme, the mobile drawer and the components are folk-ms's adaptation.

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
| `--action` | black | neutral-100 | Primary buttons, checked controls, active tab |
| `--accent` | primary-600 | primary-300 | Links, focus rings |
| `--brand` | secondary-500 | secondary-400 | The folk-ms mark |
| `--ok`, `--warn`, `--danger`, `--info` (+ `-soft`) | 700/600 on 50 | 300 on 900 | Badges, banners, toasts |

Controls are monochrome, as in the reference: the primary button is near-black (white in dark mode); blue is kept for links and focus, so it always means "you can go there".

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
- **Narrow screens (below 768px).** A top bar with a menu button and the brand replaces the sidebar, which becomes an off-canvas drawer over a scrim. Opening it moves focus into it and makes the page inert; Escape, the close button, the scrim or following a link closes it and returns focus to the menu button. The rail is never used on phones.
- **Phones (640px and below).** Headlines drop to `--text-xl`, inputs to 16px (which stops iOS from zooming into a focused field), controls grow to 44px, the sticky editor toolbar scrolls away, and tables hide their third column. The shell pads for the notch and home indicator with the `--safe-*` tokens (`viewport-fit=cover`), and `theme-color` tints the browser chrome to match.

### Components

Focused fields get one 2px accent border (`--shadow-focus` thickens the 1px border), never a second ring. Selects draw their chevron with CSS gradients (the CSP allows no `data:` images); with a mouse, browsers that support customizable selects (`appearance: base-select`) also get a styled option list, while touch screens keep the native picker.

Buttons (default, `.primary`, `.danger`, `.link`, `.icon-button`), fields (`label` + `input`/`select`/`textarea`, `.chip`, `label.inline`), `.tabs`, segmented `.lang-tabs`, `table.list`, `.badge` (neutral, `info`, ok/warn/danger by status), `.banner`, `.problems`, `.card`, `.toolbar`, `details.more` menus and `.toast`. Each is shown on `/app/design`.

## Where it lives

| What | Path |
| --- | --- |
| Tokens and the font faces | [`cms/admin/src/tokens.css`](../../cms/admin/src/tokens.css) |
| Rules, the shell layout and breakpoints | [`cms/admin/src/app.css`](../../cms/admin/src/app.css) |
| Shell markup and behaviour | [`cms/admin/src/shell.ts`](../../cms/admin/src/shell.ts) |
| Icons | [`cms/admin/src/icons.ts`](../../cms/admin/src/icons.ts) |
| Live reference (`/app/design`) | [`cms/admin/src/views/design.ts`](../../cms/admin/src/views/design.ts) |
| Figtree and its licence | [`cms/admin/src/fonts/`](../../cms/admin/src/fonts/) |
| Token check | [`scripts/styles.test.ts`](../../scripts/styles.test.ts) |

## Change together

| Rule | Files | What breaks |
| --- | --- | --- |
| The drawer breakpoint is 767px in both places | `app.css` (`max-width: 767px`), `shell.ts` (`narrow`) | Resizing across it leaves the page inert or the drawer open |
| `theme-color` matches `--sidebar` | `index.html`, `tokens.css` | The phone's browser chrome no longer matches the page |
| Fonts are served from `/app/fonts/` | `tokens.css` (`@font-face`), `build.ts` (copies `src/fonts/`, keeps the URLs external), `index.html` (preload) | Text falls back to the system font |

## Read next

- [Access](../access/README.md): the admin UI language and who sees the Administration group
- [Platform](../platform/README.md): how the admin SPA is built and served
- [ADR 0003](../architecture/adr/0003-stdlib-only-go.md): why native CSS without new build dependencies
