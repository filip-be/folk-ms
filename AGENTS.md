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

## Memory files

Repository memory lives in this `AGENTS.md`. Never create a `CLAUDE.md` anywhere in
this repository.
