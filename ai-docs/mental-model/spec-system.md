---
domain: spec-system
description: "Spec authoring tools, stem format, anchor protocol, frontmatter contracts, and feature removal protocol."
sources:
  - claude/agents/
  - claude/bin/
  - claude/infra/
  - claude/skills/forge-spec/
related:
  doc-tooling: "forge-mental-model calls ws-list-spec-stems (no args) to embed spec stems into domain drafts when ai-docs/spec/ is present. A stem format change breaks forge-mental-model's embedding step."
  mental-model-domain: "ws-spec-build-index reads ai-docs/mental-model/ to verify anchor integrity. Renaming or removing a spec stem without updating mental-model refs produces [warn] output on the next run."
---

# Spec System

Spec documents live under `ai-docs/spec/` and use a stem-and-anchor system for
stable feature identity. Three tools cooperate: `ws-generate-spec-stem`,
`ws-list-spec-stems`, and `ws-spec-build-index`.

## Entry Points

- `claude/infra/spec-conventions.md` — canonical stem format, anchor rules, frontmatter protocol.
- `claude/bin/ws-generate-spec-stem` — collision-safe stem minting.
- `claude/bin/ws-spec-build-index` — spec health pipeline; strips `features:` and `stems:` blocks; detects duplicate stems, stale Implementation Gap callouts (> 90 days), and broken mental-model anchor refs; no file arguments; scans `ai-docs/spec/` and `ai-docs/mental-model/` automatically.
- `claude/skills/forge-spec/SKILL.md` — from-scratch spec reconstruction workflow; entry point when rebuilding an entire spec.

## Module Contracts

- `ws-generate-spec-stem` guarantees: output stem is globally unique across all `{#YYMMDD-slug}` anchors
  currently present under `ai-docs/spec/`. Uniqueness holds within a single invocation
  (multiple slugs passed at once are deduplicated against each other too).
- `ws-spec-build-index` guarantees: after each run, both the `features:` and `stems:` frontmatter
  blocks are removed from every `*.md` file under `ai-docs/spec/`. Health checks run after
  cleanup: duplicate `{#YYMMDD-slug}` stems across spec files, Implementation Gap callouts older
  than 90 days, and `{#YYMMDD-slug}` references in `ai-docs/mental-model/` that have no matching
  anchor in `ai-docs/spec/`. Accepts no file arguments; always repo-wide. Output: `[fix]` for
  auto-applied cleanup, `[warn]` for health findings, `[error]` on read/write failure; silent
  when already clean and all checks pass. Always exits 0.
- `ws-list-spec-stems` (no file arg) guarantees: a flat list of every `{#YYMMDD-slug}` found by
  grepping `ai-docs/spec/`. Hierarchy and display labels require a file argument.
- `forge-spec` guarantees: no spec file is written and no archive `git mv` executes without explicit
  user confirmation. Resume detection runs at every invocation via `TaskList` — tasks prefixed
  `forge-spec-<domain>` are the persistence mechanism across compact boundaries.
- `forge-spec` has `disable-model-invocation: true`. It cannot be dispatched programmatically from
  another skill. Only slash-command invocation (`/forge-spec`) works.
- `spec-updater` identifies a spec-stem for git lookup as the bare slug extracted from the
  `{#slug}` anchor (e.g., `260421-feature-name`). It does not use a compound `file-path:slug` form.
  `git log --all --grep="<slug>"` is the lookup mechanism.
- `spec-updater` additionally scans all commits for lines matching `removed: <spec-stem>` in the
  commit message body. For each matched stem, it locates the corresponding implemented spec heading
  (no `🚧` prefix) and adds it to the `### Pending removal` report section. It never removes or
  modifies spec entries automatically — all removals require human confirmation.
- `spec-updater` supports Suggestion mode: when the call prompt begins with `Suggestion mode:`, the
  agent skips all file edits and emits `### Proposed strips` in the report instead of `### Stripped`.
  The caller (e.g., `ws:sprint` wrap-up) reads the proposals and applies strips directly. All other
  report sections (`### Pending removal`, `### Planned entry dropped`, `### Missing anchors`) are
  identical to normal mode.

## Coupling

- `ws-generate-spec-stem` ↔ `ws-list-spec-stems` ↔ `ws-spec-build-index`: all three share the
  `{#YYMMDD-slug}` regex as the stem format. A format change breaks all three.
- Anchors live in document body text (any line), not in frontmatter. Every tool
  that reads stems must grep document body — not parse frontmatter.
- `forge-spec` ↔ `TaskCreate` / `TaskList` / `TaskUpdate`: forge-spec registers one task per
  domain (`forge-spec-<domain>`) for cross-compact persistence. Clearing or renaming these tasks
  destroys resume state.
- `forge-spec` → `spec-updater`: forge-spec suggests invoking the `spec-updater` agent at
  wrap-up to strip `🚧` markers. The two tools are independent — forge-spec does not call
  spec-updater directly.
- `ws-spec-build-index` → `ai-docs/mental-model/`: the anchor integrity check reads every
  `*.md` under `ai-docs/mental-model/` and matches each `{#YYMMDD-slug}` ref against the
  spec registry. Renaming or deleting a spec stem without updating mental-model refs produces
  a `[warn]` on the next run. The dependency is one-way: mental-model docs reference spec
  stems; the spec corpus does not reference mental-model anchors.

## Extension Points & Change Recipes

- **Add a new stem to a spec**: run `ws-generate-spec-stem <descriptive-slug>` first,
  insert the returned `{#YYMMDD-slug}` on a heading line or any body line,
  then run `ws-spec-build-index` (no args) for cleanup and verification.
  The anchor can appear anywhere in the document.
- **Find all stems in the repo**: run `ws-list-spec-stems` (no args) — greps all files.
  Do not look in frontmatter; `stems:` blocks no longer exist.
- **Change the stem format**: update the `STEM_RE` constant in all three tools
  (`ws-generate-spec-stem`, `ws-list-spec-stems`, `ws-spec-build-index`) and update `spec-conventions.md`.
- **Rebuild spec from scratch**: invoke `/forge-spec`. It archives existing spec files,
  surveys the codebase via parallel Sonnet subagents, confirms domain list and per-behavior
  classification with the user, then writes spec entries domain by domain.
- **Remove a feature from the codebase**: include `removed: <spec-stem>` in the commit's
  `## Spec` section (one line per stem). The `spec-updater` agent detects this on its next
  run and adds the spec entry to the `### Pending removal` report. Remove the spec entry
  from the file manually after confirming the report. On the ticket side, declare
  `spec-remove: <stem>` in ticket frontmatter so the stem is recorded before the commit.

## Common Mistakes

- Adding `stems:` manually to spec frontmatter — `ws-spec-build-index` removes it silently
  on next run. Stems are anchors in document body text only.
- Inserting a `{#YYMMDD-slug}` without running `ws-generate-spec-stem` first — the stem
  may collide with an existing anchor in another file.
- Expecting `ws-list-spec-stems -v` (no file arg) to return display labels — flat mode cannot
  reconstruct heading context; `-v` emits a warning and flat output only.
- Searching for stems in the `stems:` frontmatter field of spec files — that field
  no longer exists. All stem lookup must grep document body for `{#\d{6}-[\w-]+}`.
- Deleting a spec entry directly without a `removed: <stem>` commit — skips the
  `spec-updater` detection step and leaves no audit trail. Always commit the removal
  signal first; let `spec-updater` surface it in the report before deleting.
