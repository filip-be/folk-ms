# Agent memory for folk-ms

## Adversarial review

The GitHub Copilot CLI (`copilot`) is not available to the user. When running the
`adversarial-review` skill (plan or code mode), do not call `copilot`, do not run
`copilot --version`, and do not stop to ask about the missing engine. Instead:

- Run the Reviewer as a subagent with the Agent tool: model `opus`, effort `high`.
- Hand it the Review brief (Reviewer instructions for the mode, Requirements, and the
  mode-specific content) as its prompt; it stays read-only and must not change the work.
- For later rounds, continue the same subagent with SendMessage so it keeps its memory,
  sending your response to each Finding by number.
- Everything else in the skill applies unchanged: Findings, the 3-round limit,
  `VERDICT: CLEAN` / `VERDICT: FINDINGS`, and the final report to the user.

## CSS

Applies to every stylesheet: `cms/admin/src/` and `sites/<id>/src/`. Native CSS, no Sass (ADR 0003: no new build dependencies).

- **Tokens.** Colour, spacing, font, font size, radius, shadow and z-index values are
  defined once as custom properties (`cms/admin/src/tokens.css`; elsewhere the `:root` or
  `:host` block at the top of the file); rules use `var(--…)`. Missing a value? Add a
  token, not a literal. `--space-N` is N × the file's unit (.1rem, or 1px where noted).
- **Nesting.** Group a component's children and states under it with `&`; Bun's CSS
  bundler lowers nesting at build. Never nest under a selector list (it becomes `:is()`
  and raises specificity).
- Enforced by `scripts/styles.test.ts` (`bun test scripts`, run in CI).
- Admin SPA breakpoints, `theme-color` and fonts are bound across files: see
  [Design system, Change together](docs/design-system/README.md#change-together).

## Admin UI

Views in `cms/admin/src/views/` follow [Design system, Buttons and Forms](docs/design-system/README.md#buttons):
one `.primary` per form (`.secondary`, neutral, `.danger` for the rest), `required` controls
with a `<label for>`, errors next to their fields via `showError(err, summary, form)`, and
inactive controls `disabled`/`readonly`, never only faded. Show a new pattern on `/app/design`
and check it in light and dark. A new validation problem path in `cms/internal/domain/` needs a
matching `data-field`: see [Design system, Change together](docs/design-system/README.md#change-together).

## Sync and Autosync

Rules that no test enforces; change both sides together (details in the
"Change together" tables of [Sync](docs/sync/README.md) and [Platform](docs/platform/README.md)):

- The `?sync=` and `?autosync=` callback codes in `cms/internal/http/auth.go` and the
  admin messages for them; `domain.Flags` and `domain.OverrideFields` and the admin
  Event screens.
- The `sync` timer schedule (`cms/deploy/functions/sync/function.json`) and
  `domain.AutosyncInterval`.
- The Key Vault alert threshold (`maxSecretGetsPerHour` in `infra/azure/main.bicep`) and
  how often the CMS reads page tokens (`SyncNowInterval`, `AutosyncInterval`, `DailyPostLimit`).

## Memory files

Repository memory lives in this `AGENTS.md`. Never create a `CLAUDE.md` anywhere in
this repository.
