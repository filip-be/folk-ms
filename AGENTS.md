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

Applies to every stylesheet: `cms/admin/src/`, `sites/<id>/src/`, `autofill/src/panel.css`,
`cms/cmd/cms/practice/`. Native CSS, no Sass (ADR 0003: no new build dependencies).

- **Tokens.** Colour, spacing, font, font size, radius, shadow and z-index values are
  defined once as custom properties (`cms/admin/src/tokens.css`; elsewhere the `:root` or
  `:host` block at the top of the file); rules use `var(--…)`. Missing a value? Add a
  token, not a literal. `--space-N` is N × the file's unit (.1rem, or 1px where noted).
- **Nesting.** Group a component's children and states under it with `&`; Bun's CSS
  bundler lowers nesting at build. Never nest under a selector list (it becomes `:is()`
  and raises specificity). Keep `autofill/src/panel.css` flat: it ships unprocessed.
- Enforced by `scripts/styles.test.ts` (`bun test scripts`, run in CI).
- Admin SPA breakpoints, `theme-color` and fonts are bound across files: see
  [Design system, Change together](docs/design-system/README.md#change-together).

## Memory files

Repository memory lives in this `AGENTS.md`. Never create a `CLAUDE.md` anywhere in
this repository.
