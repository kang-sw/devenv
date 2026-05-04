---
title: compress workflow documentation prose
related:
  260503-epic-agents-plugin-skill-porting: parent migration context for Agents plugin workflow docs
  260504-feat-agents-plugin-lead-skill-namespace: recent skill namespace cleanup before this style pass
parent: 260503-epic-agents-plugin-skill-porting
---

# compress workflow documentation prose

## Background

The Agents plugin skill docs, workflow references, project memory, and bootstrap
templates are accurate but too verbose. Long explanatory prose competes with
execution instructions and consumes session context. The current session added a
small documentation style rule to `lead-workflow` and `lead-skill-authoring`:
write compressed professional prose, prefer positive commands, use examples only
for repeated mistakes, and keep full grammar when compression could blur order,
ownership, or safety.

This ticket is a handoff for a next-session documentation optimization pass. It
should not change workflow semantics.

## Decisions

- Treat compression as an execution-accuracy tool, not a style game.
- Keep technical nouns exact; remove filler, duplicate rationale, and long setup.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- Keep safety, ordering, and ownership language explicit even if that costs words.
- Do not edit specs or mental-model documents on this branch.

## Phases

### Phase 1: Compress root and bootstrap context

Compress these files without changing semantics:

- `AGENTS.md`
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`
- `ai-docs/_index.md`

Focus:

- Shorten repeated migration-state prose.
- Keep Claude compatibility warnings intact.
- Keep setup and ticket-system rules explicit.
- Remove stale session notes or replace them with current branch state.

Acceptance criteria:

- The three files are materially shorter or more skimmable.
- No durable rule is removed without an equivalent shorter rule.
- `git diff --check` passes.

### Phase 2: Compress active Agents plugin skills

Apply the same style to active `agents-plugin/skills/lead-*` docs.

Priority order:

1. `lead-proceed`
2. `lead-implement`
3. `lead-sprint`
4. `lead-write-code`
5. `lead-edit`
6. Remaining `lead-*` skills by context cost

Focus:

- Keep handlers command-shaped.
- Move rationale to Doctrine.
- Cut duplicate invariants.
- Preserve exact MCP pseudo-call notation.
- Preserve reviewer adjudication and user-approval gates.

Acceptance criteria:

- No old `ws:<non-lead>` references are introduced.
- Repo and local plugin cache skill docs match for edited skills.
- `cd agents-plugin-tool && go test ./...` passes if embedded prompts or tooling
  are touched; otherwise `git diff --check` is enough.

### Phase 3: Audit prompts and infra docs

Compress prompt and infra docs that delegated agents read directly:

- `agents-plugin-tool/internal/wsprompt/infra/delegate-orientation.md`
- `agents-plugin-tool/internal/wsprompt/prompts/*.md`
- `ai-docs/ref/skill-authoring.md`

Focus:

- Keep worker output contracts exact.
- Keep ask-api requirement prominent.
- Keep `lead-*` boundary visible.
- Update `agents-plugin/runtime.json` prompt bundle hash if embedded prompts
  change.

Acceptance criteria:

- Prompt tests pass after any embedded prompt change.
- Delegate-facing prompts remain self-contained.

### Phase 4: Closeout

Review the diff for meaning changes, then commit the compression pass.

Acceptance criteria:

- Run `rg 'ws:(add-rule|bootstrap|discuss|edit|exit-session|forge-mental-model|forge-spec|implement|proceed|ship|skill-authoring|sprint|update-spec|workflow|write-code|write-skeleton|write-spec|write-ticket)\\b' agents-plugin agents-plugin-tool ai-docs/_index.md AGENTS.md`.
- Run `git diff --check`.
- Report files changed, verification run, and any deferred documents.
