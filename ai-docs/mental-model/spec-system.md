---
domain: spec-system
description: "Spec authoring tools, stem format, anchor protocol, frontmatter contracts, and feature removal protocol."
sources:
  - claude/agents/
  - claude/bin/
  - claude/infra/
  - claude/skills/forge-spec/
---

# Spec System

Spec documents live under `ai-docs/spec/` and use a stem-and-anchor system for
stable feature identity. Three tools cooperate: `generate-spec-stem`,
`list-stems`, and `spec-build-index`.

## Entry Points

- `claude/infra/spec-conventions.md` — canonical stem format, anchor rules, frontmatter protocol.
- `claude/bin/generate-spec-stem` — collision-safe stem minting.
- `claude/bin/spec-build-index` — frontmatter regeneration; entry point for understanding what it writes and removes.
- `claude/skills/forge-spec/SKILL.md` — from-scratch spec reconstruction workflow; entry point when rebuilding an entire spec.

## Module Contracts

- `generate-spec-stem` guarantees: output stem is globally unique across all `{#YYMMDD-slug}` anchors
  currently present under `ai-docs/spec/`. Uniqueness holds within a single invocation
  (multiple slugs passed at once are deduplicated against each other too).
- `spec-build-index` guarantees: after each run, the `features:` frontmatter block reflects
  the current heading structure (headings inside ` ``` ` or `~~~` fenced blocks are excluded),
  and any `stems:` block is removed. It does not write stems anywhere.
- `list-stems` (no file arg) guarantees: a flat list of every `{#YYMMDD-slug}` found by
  grepping `ai-docs/spec/`. Hierarchy and display labels require a file argument.
- `forge-spec` guarantees: no spec file is written and no archive `git mv` executes without explicit
  user confirmation. Resume detection runs at every invocation via `TaskList` — tasks prefixed
  `forge-spec-<domain>` are the persistence mechanism across compact boundaries.
- `spec-updater` identifies a spec-stem for git lookup as the bare slug extracted from the
  `{#slug}` anchor (e.g., `260421-feature-name`). It does not use a compound `file-path:slug` form.
  `git log --all --grep="<slug>"` is the lookup mechanism.
- `spec-updater` additionally scans all commits for lines matching `removed: <spec-stem>` in the
  commit message body. For each matched stem, it locates the corresponding implemented spec heading
  (no `🚧` prefix) and adds it to the `### Pending removal` report section. It never removes or
  modifies spec entries automatically — all removals require human confirmation.

## Coupling

- `generate-spec-stem` ↔ `list-stems` ↔ `spec-build-index`: all three share the
  `{#YYMMDD-slug}` regex as the stem format. A format change breaks all three.
- Anchors live in document body text (any line), not in frontmatter. Every tool
  that reads stems must grep document body — not parse frontmatter.
- `forge-spec` ↔ `TaskCreate` / `TaskList` / `TaskUpdate`: forge-spec registers one task per
  domain (`forge-spec-<domain>`) for cross-compact persistence. Clearing or renaming these tasks
  destroys resume state.
- `forge-spec` → `spec-updater`: forge-spec suggests invoking the `spec-updater` agent at
  wrap-up to strip `🚧` markers. The two tools are independent — forge-spec does not call
  spec-updater directly.

## Extension Points & Change Recipes

- **Add a new stem to a spec**: run `generate-spec-stem <descriptive-slug>` first,
  insert the returned `{#YYMMDD-slug}` on a heading line or any body line,
  then run `spec-build-index <file>`. The anchor can appear anywhere in the document.
- **Find all stems in the repo**: run `list-stems` (no args) — greps all files.
  Do not look in frontmatter; `stems:` blocks no longer exist.
- **Change the stem format**: update the `STEM_RE` constant in all three tools
  (`generate-spec-stem`, `list-stems`, `spec-build-index`) and update `spec-conventions.md`.
- **Rebuild spec from scratch**: invoke `/forge-spec`. It archives existing spec files,
  surveys the codebase via parallel Sonnet subagents, confirms domain list and per-behavior
  classification with the user, then writes spec entries domain by domain.
- **Remove a feature from the codebase**: include `removed: <spec-stem>` in the commit's
  `## Spec` section (one line per stem). The `spec-updater` agent detects this on its next
  run and adds the spec entry to the `### Pending removal` report. Remove the spec entry
  from the file manually after confirming the report. On the ticket side, declare
  `spec-remove: <stem>` in ticket frontmatter so the stem is recorded before the commit.

## Common Mistakes

- Adding `stems:` manually to spec frontmatter — `spec-build-index` removes it silently
  on next run. Stems are anchors in document body text only.
- Inserting a `{#YYMMDD-slug}` without running `generate-spec-stem` first — the stem
  may collide with an existing anchor in another file.
- Expecting `list-stems -v` (no file arg) to return display labels — flat mode cannot
  reconstruct heading context; `-v` emits a warning and flat output only.
- Searching for stems in the `stems:` frontmatter field of spec files — that field
  no longer exists. All stem lookup must grep document body for `{#\d{6}-[\w-]+}`.
- Deleting a spec entry directly without a `removed: <stem>` commit — skips the
  `spec-updater` detection step and leaves no audit trail. Always commit the removal
  signal first; let `spec-updater` surface it in the report before deleting.
