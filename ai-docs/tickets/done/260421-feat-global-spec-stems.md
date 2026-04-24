---
title: "Global Unique Spec Stems — Design Pivot"
related:
  260420-feat-spec-driven-workflow: supersedes-tooling
plans:
  phase-1: 2026-04/260421-1259.global-spec-stems-p1-4.survey
  phase-2: 2026-04/260421-1259.global-spec-stems-p1-4.survey
  phase-3: 2026-04/260421-1259.global-spec-stems-p1-4.survey
  phase-4: 2026-04/260421-1259.global-spec-stems-p1-4.survey
started: 2026-04-21
completed: 2026-04-24
---

# Global Unique Spec Stems — Design Pivot

## Background

The spec-stem system implemented in `260420` uses a structured `file:parent.child` path format. This format couples stem references to two pieces of context the referencing agent must carry: the spec file path, and the heading hierarchy. In practice, this is unnecessary burden — any agent that has read the relevant spec section already has the stem in context, and grep-based lookup requires knowing the file path upfront.

This ticket pivots to globally unique stems that eliminate that coupling entirely.

## Decisions

**Stem format: `YYMMDD-slug`**
Date prefix ensures uniqueness without requiring timestamp precision. The slug is a short, descriptive identifier authored by the agent at stem-creation time. `generate-spec-stem` validates uniqueness and handles collision (appending `-2`, `-3`, etc.). No file path, no hierarchy encoding — the stem is a standalone opaque identifier.

**Stems placeable anywhere in a spec document, not just headings**
A spec commitment is not always heading-level. A paragraph, a bullet, or a note can represent a named concept. `{#260421-discussion-loop}` can appear on a heading or inline in body text.

**`generate-spec-stem <slug> [<slug>...]` — new command**
Greps `ai-docs/spec/` for all existing `{#YYMMDD-*}` anchors to check for collisions. Outputs valid stems with today's date prefix. If a collision is found (same date + slug), appends `-2`, `-3`, etc. Multiple slugs can be generated in one call.

**`list-spec-stems [<file>]`— grep-based rewrite with markdown context**
Greps spec file(s) for `{#YYMMDD-*}` anchors. When a file is specified, uses the markdown heading stack at each anchor's position to display stems in hierarchical context. Heading-level stems are displayed as section roots; body-level stems are displayed under their nearest parent heading.

Example output (`list-spec-stems -v ai-docs/spec/skills.md`):
```
260421-workflow-skills      ## Workflow Skills
  260421-discussion         ### Discussion
    260421-discussion-loop  ### Discussion Loop
  260421-spec-authoring     ### Spec Authoring
```

**`spec-build-index` drops `stems:` generation**
The `stems:` frontmatter block is no longer needed — stems are findable via grep across the spec directory. `spec-build-index` retains `features:` generation (human-readable heading list for quick overview) and strips `{#slug}` anchors from display text as before.

**All convention infrastructure carries over unchanged**
Ticket `spec:` field, commit `## Spec` section, spec-updater grep protocol — all remain valid. Only the stem format changes: references now use `260421-discussion-loop` instead of `skills:workflow-skills.discussion.discussion-loop`.

## Transition from `260420` Implementation

The phases 1–5 implementation provides infrastructure that partially carries over:

| Component | Status |
|---|---|
| `spec-build-index` features: generation + anchor strip | Keep as-is |
| `spec-build-index` stems: generation | Remove |
| `list-spec-stems` | Rewrite (frontmatter → grep + markdown parsing) |
| `generate-spec-stem` | New |
| `write-spec/SKILL.md` judge: spec-impact gate | Keep as-is |
| `write-spec/SKILL.md` stem authoring instructions | Update: use `generate-spec-stem` |
| `spec-conventions.md` stem format | Update: `YYMMDD-slug` format |
| `discuss/SKILL.md` unconditional write-spec | Keep as-is |
| `write-ticket/SKILL.md` spec-stem prompting | Keep as-is |
| `ticket-conventions.md` spec: field | Keep as-is |
| `CLAUDE.template.md` ## Spec section + v0023 | Keep as-is |
| `spec-updater.md` grep protocol | Keep as-is |

The main work is in the tooling layer (commands) and convention documents. The workflow plumbing is intact.

## Phases

### Phase 1: `generate-spec-stem` command

New Python command (`claude/bin/generate-spec-stem`) following the `list-mental-model` / `list-spec-stems` pattern.

- Accepts one or more slug arguments.
- Greps `ai-docs/spec/` recursively for `{#YYMMDD-*}` to collect existing stems.
- For each input slug: compute `YYMMDD-slug`. If collision, try `YYMMDD-slug-2`, `-3`, etc.
- Output: one valid stem per line (no prefix, ready to paste as `{#...}`).

**Acceptance criteria:**
- `generate-spec-stem discussion-loop` outputs `260421-discussion-loop` (today's date)
- `generate-spec-stem a b c` outputs three stems in one call
- Collision on same date+slug appends `-2`

### Result (cab9b24) - 2026-04-21

`claude/bin/generate-spec-stem` created. Scans `ai-docs/spec/` via `Path(__file__).resolve().parents[2]` (CWD-independent). Collision handling appends `-2`, `-3`, etc. Multi-slug deduplication runs in-memory within a single call.

### Phase 2: `list-spec-stems` rewrite

Replace frontmatter-reading logic with grep-based + markdown parsing.

- Without file arg: grep all files under `ai-docs/spec/`, output flat list of stems found.
- With file arg: parse markdown, track heading stack, output stems in hierarchical context.
- `-v` flag: include heading/nearby text as display label.
- Heading-level stem: extracted from the heading line itself.
- Body-level stem: displayed indented under nearest parent heading, label from surrounding text (best-effort: first non-empty text after the anchor on the same line, or heading text of parent).

**Acceptance criteria:**
- `list-spec-stems ai-docs/spec/skills.md` outputs stems in document order with heading context
- `list-spec-stems -v ai-docs/spec/skills.md` adds display labels
- `list-spec-stems` (no arg) outputs all stems across spec directory, flat

### Result (cab9b24) - 2026-04-21

`claude/bin/list-spec-stems` fully rewritten. No-arg mode: flat grep scan across `ai-docs/spec/`. File-arg mode: markdown heading stack parser; H1 excluded; heading-level stems at depth-proportional indent; body-level stems one level deeper than current heading. `-v` flag: tab-separated stem + label; emits stderr warning in no-arg mode instead of silently ignoring.

### Phase 3: Tooling cleanup — `spec-build-index`

Remove `update_stems` / `build_stems_block` logic and the `stems:` frontmatter block update from `spec-build-index`. The `features:` update and anchor-stripping behavior remain unchanged.

**Acceptance criteria:**
- `spec-build-index ai-docs/spec/skills.md` updates `features:` only; no `stems:` block written or left behind
- Existing `stems:` blocks in any spec file are removed on next `spec-build-index` run (or left as inert — decide at implementation time)

### Result (cab9b24) - 2026-04-21

`stems:` generation fully removed from `spec-build-index` (`extract_stems`, `update_stems`, `build_stems_block` deleted). `_replace_or_insert_block` extended: when `new_field_lines` is empty, removes existing block rather than no-op. `remove_stems()` added; `process_file()` now actively clears stale `stems:` blocks on every run.

### Phase 4: Convention documents

Update `claude/infra/spec-conventions.md`:
- Stem format section: `{#YYMMDD-slug}` replaces `{#slug}`
- Placement: anywhere in document, not heading-only
- Add: use `generate-spec-stem` to obtain a valid stem before inserting

Update `claude/skills/write-spec/SKILL.md`:
- Stem authoring step: call `generate-spec-stem <descriptive-slug>` to obtain stem; insert result as `{#...}` at the appropriate point in the spec
- Remove: any reference to heading-only placement constraint

**Acceptance criteria:**
- `spec-conventions.md` stem format and placement conventions match new design
- `write-spec/SKILL.md` instructs use of `generate-spec-stem`

### Result (cab9b24) - 2026-04-21

`spec-conventions.md` updated: `{#YYMMDD-slug}` format throughout, anchors placeable anywhere (not heading-only), `generate-spec-stem` as the authoring tool, `stems:` frontmatter references removed. `write-spec/SKILL.md` updated: `judge: spec-impact` as step 0; step 3 split into 3b-3e covering generate stem → insert anchor → add/remove markers → run spec-build-index; body-line anchor example added to templates. Structural review found two issues post-implementation (spec-updater grep using compound stem, ticket-conventions.md example in old format); both fixed in `cab9b24`.

### Phase 5: Spec file migration (deferred — see Phase 6 of `260420`)

Existing spec files currently have either no stems or legacy `{#slug}` anchors (non-dated). Migration runs alongside the Phase 6 migration from `260420`:

- Replace any legacy `{#slug}` anchors with `{#YYMMDD-slug}` equivalents using `generate-spec-stem`.
- Remove `stems:` frontmatter blocks (if any remain after Phase 3).
- Run `spec-build-index` to refresh `features:`.

This phase is intentionally deferred: no spec file currently has `stems:` in production (phases 1–5 are on an unmerged branch), so there is no live migration urgency.

### Result — cancelled 2026-04-24

Migration not required for current usage. Closed unimplemented.
