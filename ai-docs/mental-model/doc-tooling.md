---
domain: doc-tooling
description: "Mental-model authoring toolchain (forge-mental-model, mental-model-updater, add-rule) and plugin doc quality agent (polish-writer) — contracts, coupling, forge authority, Domain Rules handling, and non-obvious maintenance requirements."
sources:
  - claude/skills/forge-mental-model/
  - claude/skills/add-rule/
  - claude/agents/
  - claude/infra/
  - .claude/skills/polish-plugin-docs/
related:
  spec-system: "forge-mental-model calls list-spec-stems to embed spec stems when ai-docs/spec/ is present. A stem format change breaks the embedding step."
---

# Doc Tooling

Three tools maintain `ai-docs/mental-model/`: `forge-mental-model` (from-scratch
construction), `mental-model-updater` (incremental update after code changes), and
`/add-rule` (rule authoring and routing). All three are independent — none calls another.

## Entry Points

- `claude/skills/forge-mental-model/SKILL.md` — from-scratch construction workflow; entry point when no mental-model baseline exists or a full rebuild is needed.
- `claude/agents/mental-model-updater.md` — incremental updater agent; entry point for post-implementation mental-model maintenance.
- `claude/skills/add-rule/SKILL.md` — rule authoring skill; entry point for classifying and routing new rules to `CLAUDE.md ## Architecture Rules` or a domain doc `## Domain Rules`.
- `.claude/skills/polish-plugin-docs/polish-writer.md` — no-file-read sub-agent for plugin doc simplification; entry point for the verbatim-embed pattern used when an agent cannot read files.

## Module Contracts

- `forge-mental-model` guarantees: no domain file is written before the domain survey and verifier steps complete for that domain. The cold-start → per-domain → wrap-up sequence is not skippable.
- `forge-mental-model` guarantees: domain list must receive explicit user confirmation before any `TaskCreate` call or file write. Proceeding without confirmation violates the skill's invariant.
- `forge-mental-model` has `disable-model-invocation: true`. It cannot be dispatched programmatically from another skill. Only slash-command invocation (`/forge-mental-model`) works.
- `mental-model-updater` uses `git log --grep="mental-model-updated" -1` to locate the last checkpoint. Any commit that touches `ai-docs/mental-model/` without `(mental-model-updated)` in the message body is invisible to the updater's base-commit search. The updater falls back to the caller-provided base commit when no stamp is found.
- `mental-model-updater` holds forge authority: it may create new domain docs and split flat docs into `<domain>/index.md` + child files. This authority activates only when the diff shows a matching code-structure change (a new module directory, or an existing module splitting into sub-directories). Restructuring on authorial judgment alone violates the contract.
- `mental-model-updater` guarantees: Domain Rules content is never modified autonomously. During doc splits it may promote rules upward (sub-domain → parent `index.md`) — position changes only, never content changes, and never downward movement.
- `mental-model-updater` guarantees: when a Domain Rule appears inconsistent with current code, it appends a `## Stale Rules` block to its output listing the rule verbatim and the observed inconsistency. It does not edit the rule. The user resolves via manual edit or `/add-rule`.
- `mental-model-updater` must load the parent `index.md` before editing any sub-domain doc (`<domain>/<sub>.md`) so inherited Domain Rules are visible before work begins.
- `/add-rule` guarantees: append-only — it never modifies existing rule content. One invocation writes to exactly one target file.
- `/add-rule` classifies the rule as cross-cutting or domain-scoped before routing. Cross-cutting rules go to `CLAUDE.md ## Architecture Rules`; domain-scoped rules go to the matching domain doc's `## Domain Rules` section. When classification is ambiguous, the skill stops and prompts the user — it never guesses.
- `polish-writer` has no file-read capability. The authoring reference (`ai-docs/ref/skill-authoring.md`) is embedded verbatim inside the agent document. The calling skill must supply the full file content and findings in the spawn prompt — the agent cannot retrieve any context itself.

## Coupling

- `forge-mental-model` ↔ `TaskCreate` / `TaskList` / `TaskUpdate`: forge-mental-model registers one task per domain using the name prefix `forge-mental-model-<domain>`. Resume detection at invocation reads tasks by this prefix. Clearing or renaming these tasks destroys cross-compact resume state silently.
- `forge-mental-model` → `list-spec-stems`: when `ai-docs/spec/` is present, forge-mental-model calls `list-spec-stems` (no args) to discover all spec stems, then embeds matching stems in the domain draft. This step is skipped when no spec exists. A change to the `{#YYMMDD-slug}` stem format breaks the embedding step.
- `forge-spec` ↔ `forge-mental-model`: independent tools. forge-spec produces spec content; forge-mental-model consumes stems from that content via list-spec-stems. forge-spec does not invoke forge-mental-model and vice versa. Bootstrap suggests forge-spec first when both are absent.
- `polish-writer` ↔ `ai-docs/ref/skill-authoring.md`: polish-writer embeds the full content of skill-authoring.md verbatim (no file-read capability). A change to skill-authoring.md requires a manual update to the embedded block in `.claude/skills/polish-plugin-docs/polish-writer.md`. The block is identified by the Doctrine comment in that file.

## Extension Points & Change Recipes

- **Build mental-model from scratch**: invoke `/forge-mental-model`. If no `(mental-model-updated)` checkpoint exists in git history, pass the repository's initial commit as the base commit to the updater after forge-mental-model completes.
- **Update mental-model after code changes**: dispatch the `mental-model-updater` agent with the base commit. The agent reads the last `(mental-model-updated)` checkpoint automatically.
- **Add a new domain during a forge run**: confirm the domain in the user confirmation step. `TaskCreate` will add it to the task list. Do not create domain files outside the per-domain handler — the verifier step would be skipped.
- **Add a rule that should persist across sessions**: invoke `/add-rule "<rule description>"`. The skill classifies and routes the rule; do not write directly to `## Architecture Rules` or `## Domain Rules` without classification — a misrouted rule hides cross-cutting invariants inside domain docs or dilutes Architecture Rules with domain trivia.

## Common Mistakes

- Clearing `forge-mental-model-*` tasks between sessions — destroys resume state. The next invocation restarts from cold-start, re-surveys, and may overwrite existing domain files.
- Committing `ai-docs/mental-model/` changes without `(mental-model-updated)` in the message body — the mental-model-updater's checkpoint search skips that commit. The base for the next update run shifts earlier than intended, causing re-assessment of already-covered changes.
- Trying to dispatch `forge-mental-model` from another skill — `disable-model-invocation: true` prevents model-initiated invocation. Use slash-command only.
- Running forge-mental-model before forge-spec when both are absent — mental-model is built without spec stem cross-references. forge-spec should run first when spec coverage is desired.
- Splitting or restructuring mental-model docs without a corresponding code-structure change in the diff — forge authority requires an observable code-structure trigger. Restructuring on judgment alone violates the updater's invariant.
- Editing `## Domain Rules` content directly via `mental-model-updater` — the updater only promotes rules upward and flags stale ones in output. Edits to rule content require manual user action or re-invocation of `/add-rule`.
- Writing a rule directly to `CLAUDE.md` or a domain doc without going through `/add-rule` — misrouting is silent and not caught at write time. Use `/add-rule` to trigger the classification step.
- Editing `ai-docs/ref/skill-authoring.md` without updating the verbatim copy embedded in `.claude/skills/polish-plugin-docs/polish-writer.md` — the agent silently applies stale authoring principles. The embedded block must stay in sync with the source file.
