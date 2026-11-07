---
name: document-capability
description: How to document a capability. Use when a change alters how a capability behaves, adds a capability or use case, moves or renames files named in a capability's "Change together" table, or changes behaviour that consumers sees. Also use when unsure where a piece of documentation belongs.
---

# Document a capability

The documentation of this repository is organized by capability. [docs/README.md](../../../docs/README.md) is the map: the building blocks and the capabilities that use them. This skill is the procedure for keeping that structure when you change the code.

## 1. Find the home

Every piece of documentation has one home. Decide by what it is about, not by which folder you happen to be editing:

| Content | Home |
| --- | --- |
| What a capability does, how its parts work together, why it works that way | The capability's folder under `docs/` |
| A use case or a sizeable part of a capability | A sub-article beside the capability's main article |
| A rule that binds files in different folders | The `## Change together` table of the capability that owns the producing side |
| Build, run, test, configure or deploy one component | That component's `README.md` |
| An instruction for agents working inside one folder | That folder's `AGENTS.md` |
| A rule for the whole repository | The root `AGENTS.md` |
| Infrastructure, pipelines, authentication, local development, observability plumbing | `docs/platform/` |
| A domain term | `CONTEXT.md`, in the section of its capability |
| A decision with alternatives and trade-offs | A new ADR in `docs/architecture/adr/` |

A new capability gets a folder and a row and a paragraph in `docs/README.md`.

Component READMEs keep only what is needed inside their folder. When you find yourself explaining a flow or a reason in a component README, move it to the capability article and link back to the folder instead.

## 2. Write the main article

The main article is `docs/<capability>/README.md`. Keep it light; detail goes into sub-articles beside it.

```markdown
# <Capability>

One paragraph: what it is, for whom, and which building blocks it uses.

## How it works

The flow, in prose and short lists. Link sub-articles for each use case.

## Where it lives

| What | Path |
| --- | --- |
| ... | [`src/...`](../../src/...) |

## Change together

| Rule | Files | What breaks |
| --- | --- | --- |

## Read next

- Sub-articles
- [CONTEXT.md](../../CONTEXT.md#<section>): the terms this capability uses
- ADRs
- Component READMEs for running and testing
```

`## Change together` is optional; leave it out when the capability has no cross-folder rule. Use the terms from `CONTEXT.md` in their meanings, and do not restate what an ECI under `.eci/` already says for consumers: link it.

### Diagrams

A diagram helps when a flow crosses several components, but only if a person and an agent can both read it at a glance:

- Use Mermaid, which GitHub renders and an agent reads as text. Never draw with aligned ASCII arrows: the alignment carries the meaning and breaks on the first edit.
- Draw one flow per diagram. When a flow has stages triggered by different events, give each stage its own diagram with a one-line lead-in.
- Keep it to a handful of nodes, and label each edge with what happens, in the terms of `CONTEXT.md`.
- The prose says everything the diagram shows, so nothing is lost when the diagram does not render.

## 3. Keep the change-together rules

A rule that binds files in different folders is written once, as one row of a `## Change together` table: the rule, the files it binds, and what breaks when only one side changes. When a rule spans two capabilities, it lives in the one that owns the producing side and the other links to it.

An agent editing a bound file has no reason to open the capability article, so decide for each rule whether it also needs a pointer:

1. **Enforce it in code first.** Prefer a shared type or constant, a contract test or a lint rule. When the build or the tests catch a mismatch, the row is enough: add no pointer.
2. **Point to it when it breaks silently.** When the rule cannot be enforced and breaking it fails without a build or test error, add a pointer in the same change. This is the one planned exception to step 4: the rule is known to be invisible from the code, so do not wait for an agent to break it.
3. **Keep the pointer small.** One line per rule in the `AGENTS.md` nearest the bound files, not one per file, for example:

```markdown
- `src/runName.ts` is bound to the event management: see [Event management, Change together](../../docs/event-management/README.md#change-together).
```

When a rule becomes enforced in code, remove its pointer. When you add, move or rename a bound file, update the row and its pointer in the same change. When a change touches a file named in a table, check the other files in its row before you finish.

## 4. Keep `AGENTS.md` short

Every line of an `AGENTS.md` is loaded into every agent session in its folder, so each line costs context and dilutes the others. `AGENTS.md` is the last resort, not a place to document:

- Add a line only when agents have struggled with the same issue more than once, and the documentation in its proper home did not prevent it. The only exception is a pointer to a silently breaking change-together rule (step 3).
- Write the smallest line that fixes the issue: a pointer to the home, not a copy of its content.
- Keep a line only while it is proven to improve the workflow. Remove it when the issue no longer occurs, or when it has not helped.
- Create a nested `AGENTS.md` only when such a line is needed and none exists. Never create a `CLAUDE.md`: one in a directory or above makes Claude Code ignore `AGENTS.md`.

When in doubt, leave `AGENTS.md` unchanged.

## 5. Link the glossary and the decisions

- Add or adjust terms in `CONTEXT.md`, in the section of the capability. Terms only: no implementation detail.
- A decision with real alternatives gets an ADR, following the organization's `adr-template` and `adr-process` ECIs. Take the next free number on `main`.
- The main article's `## Read next` links its `CONTEXT.md` section and its ADRs.

## Checklist

- [ ] The content sits in its one home; no flow or reason is repeated in a component README.
- [ ] The main article follows the template and stays short.
- [ ] Each diagram is Mermaid, shows one flow and stays small.
- [ ] Every cross-folder rule touched is in exactly one `## Change together` table, and is either enforced by the build or tests, or has a one-line pointer in the nearest `AGENTS.md`.
- [ ] Otherwise, `AGENTS.md` grew only for an issue agents have hit repeatedly, by the smallest possible line.
- [ ] `CONTEXT.md` and the ADRs are linked from `## Read next`.
- [ ] Every relative link resolves and every file ends with a newline.