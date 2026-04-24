---
domain: doc-tooling
description: "Mental-model authoring toolchain: forge-mental-model construction skill and mental-model-updater agent, their contracts, coupling, and the commit-stamp checkpoint mechanism."
sources:
  - claude/skills/forge-mental-model/
  - claude/agents/
related:
  spec-system: "forge-mental-model calls list-spec-stems to embed spec stems when ai-docs/spec/ is present. A stem format change breaks the embedding step."
---

# Doc Tooling

Two tools maintain `ai-docs/mental-model/`: `forge-mental-model` (from-scratch
construction) and `mental-model-updater` (incremental update after code changes).
They are independent — neither calls the other.

## Entry Points

- `claude/skills/forge-mental-model/SKILL.md` — from-scratch construction workflow; entry point when no mental-model baseline exists or a full rebuild is needed.
- `claude/agents/mental-model-updater.md` — incremental updater agent; entry point for post-implementation mental-model maintenance.

## Module Contracts

- `forge-mental-model` guarantees: no domain file is written before the domain survey and verifier steps complete for that domain. The cold-start → per-domain → wrap-up sequence is not skippable.
- `forge-mental-model` guarantees: domain list must receive explicit user confirmation before any `TaskCreate` call or file write. Proceeding without confirmation violates the skill's invariant.
- `forge-mental-model` has `disable-model-invocation: true`. It cannot be dispatched programmatically from another skill. Only slash-command invocation (`/forge-mental-model`) works.
- `mental-model-updater` uses `git log --grep="mental-model-updated" -1` to locate the last checkpoint. Any commit that touches `ai-docs/mental-model/` without `(mental-model-updated)` in the message body is invisible to the updater's base-commit search. The updater falls back to the caller-provided base commit when no stamp is found.

## Coupling

- `forge-mental-model` ↔ `TaskCreate` / `TaskList` / `TaskUpdate`: forge-mental-model registers one task per domain using the name prefix `forge-mental-model-<domain>`. Resume detection at invocation reads tasks by this prefix. Clearing or renaming these tasks destroys cross-compact resume state silently.
- `forge-mental-model` → `list-spec-stems`: when `ai-docs/spec/` is present, forge-mental-model calls `list-spec-stems` (no args) to discover all spec stems, then embeds matching stems in the domain draft. This step is skipped when no spec exists. A change to the `{#YYMMDD-slug}` stem format breaks the embedding step.
- `forge-spec` ↔ `forge-mental-model`: independent tools. forge-spec produces spec content; forge-mental-model consumes stems from that content via list-spec-stems. forge-spec does not invoke forge-mental-model and vice versa. Bootstrap suggests forge-spec first when both are absent.

## Extension Points & Change Recipes

- **Build mental-model from scratch**: invoke `/forge-mental-model`. If no `(mental-model-updated)` checkpoint exists in git history, pass the repository's initial commit as the base commit to the updater after forge-mental-model completes.
- **Update mental-model after code changes**: dispatch the `mental-model-updater` agent with the base commit. The agent reads the last `(mental-model-updated)` checkpoint automatically.
- **Add a new domain during a forge run**: confirm the domain in the user confirmation step. `TaskCreate` will add it to the task list. Do not create domain files outside the per-domain handler — the verifier step would be skipped.

## Common Mistakes

- Clearing `forge-mental-model-*` tasks between sessions — destroys resume state. The next invocation restarts from cold-start, re-surveys, and may overwrite existing domain files.
- Committing `ai-docs/mental-model/` changes without `(mental-model-updated)` in the message body — the mental-model-updater's checkpoint search skips that commit. The base for the next update run shifts earlier than intended, causing re-assessment of already-covered changes.
- Trying to dispatch `forge-mental-model` from another skill — `disable-model-invocation: true` prevents model-initiated invocation. Use slash-command only.
- Running forge-mental-model before forge-spec when both are absent — mental-model is built without spec stem cross-references. forge-spec should run first when spec coverage is desired.
