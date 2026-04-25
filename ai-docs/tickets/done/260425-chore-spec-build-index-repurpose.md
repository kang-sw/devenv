---
title: "Repurpose ws-spec-build-index: strip frontmatter redundancy and add spec health checks"
spec:
  - 260421-ws-spec-build-index-tool
  - 260425-write-ticket-spec-gate-promotion
  - 260425-discuss-promotion-handler-order
related-mental-model:
  - spec-system
completed: 2026-04-25
---

# Repurpose ws-spec-build-index: strip frontmatter redundancy and add spec health checks

## Background

`ws-spec-build-index` currently writes a `features:` frontmatter block derived from document
headings, and removes the legacy `stems:` block. The `features:` block is a derivative copy —
`ws-list-spec-stems` live-parses heading anchors directly, so the frontmatter entry adds visual
noise and a false "which is authoritative?" signal. The spec-system mental model already mandates
body-grep as the canonical lookup path; the frontmatter field never had any readers.

The opportunity: repurpose the tool as a spec health pipeline. "Build" semantics (VS-style:
compile + verify) justify adding validation steps alongside the cleanup. Running no-arg always
produces a repo-wide health report — the tool becomes the single enforcer agents call after
every spec write.

Out of scope (deferred, separate discussion):
- `🚧`/ticket coherence check via build-index (gate vs. lint model TBD)
- Mandating build-index at end of `write-ticket`

## Decisions

- **No file-arg mode.** Cross-file checks (duplicate stems, anchor integrity) require repo-wide
  scan regardless of which file was edited. Single-file mode cannot do the full job — removing it
  forces correct behavior and simplifies the interface.
- **strip vs. write.** The tool transitions from writer to cleaner: remove `features:` and `stems:`
  blocks if present; silently no-op on already-clean files.
- **warn, not fail.** Validation output uses `[fix]` for auto-applied changes and `[warn]` for
  items requiring human judgment. The tool always exits 0; it is a reporter, not a gate.

## Phases

### Phase 0: Fix spec-ticket circular dependency in conventions and write-ticket

Background: `spec-conventions.md` says "Defer spec entry until the ticket is promoted to `todo/`,"
while `write-ticket`'s `judge: spec-gate` blocks ticket creation without a pre-existing spec entry.
For brand-new behaviors this is a deadlock. Resolution: remove the ordering gate from the convention
(the core constraint "🚧 requires todo/-or-higher ticket" is sufficient), and extend spec-gate to
also enforce on `idea/` → `todo/` promotion (currently uncovered).

Goals:
- `spec-conventions.md`: remove the sentence "Defer spec entry until the ticket is promoted to
  `todo/`." The remaining rule ("🚧 entries require a `todo/`-or-higher ticket") is declarative
  and sufficient.
- `write-ticket/SKILL.md` `judge: spec-gate`: change trigger from "CREATE path only" to
  "any action that results in `todo/`-or-higher status" — covers both direct `todo/` creation
  and `idea/` → `todo/` promotion moves. `idea/` creation remains ungated.
- `discuss/SKILL.md` "On: Ticket Status Transition" promotion handler: reverse step order from
  "git mv → write-spec" to "write-spec → git mv". Spec must exist before the promotion move so
  that the new spec-gate can enforce correctly.

Depends on: nothing. Must precede Phase 5 (convention doc updates).

Success: an agent can write a `🚧` spec entry before the ticket exists; promoting an `idea/`
ticket to `todo/` without a spec entry is blocked at the move step; direct `todo/` creation
without a spec entry remains blocked; the discuss promotion handler runs write-spec before git mv.

### Result (a89abc2) - 2026-04-25

Implemented all three changes in a single commit. spec-conventions.md: removed "Defer spec entry"
sentence. write-ticket spec-gate: extended trigger to cover idea/→todo/ promotion. discuss
promotion handler: reversed to write-spec before git mv. No deviations from plan.

### Phase 1: Strip features: from spec files and update tool core

Goals:
- Remove the `features:` field from all spec files under `ai-docs/spec/` (one-time migration).
- Update `ws-spec-build-index` to remove `features:` blocks (extending the existing `stems:`
  removal pattern) instead of writing them.
- Remove file-argument mode entirely; the tool always scans all spec files under `ai-docs/spec/`.
- Output format: `[fix] features: removed — <filename>` per cleaned file; silent on already-clean files.

Suggested approach:
- Reuse the existing `_replace_or_insert_block(lines, "features", [])` pattern already present
  in `remove_stems()`.
- `main()` discovers all `*.md` files under `ai-docs/spec/` via `Path.glob`; `sys.argv` check is
  removed.
- Run the updated tool once to migrate all current spec files as part of this phase commit.

Success: all spec files have no `features:` block; tool accepts no arguments; calling with an
argument exits with a usage error.

### Result (fa21a79) - 2026-04-25

Rewrote tool: removed write path (extract_features, build_tree_text, update_frontmatter,
_indent_block), added remove_features() mirroring remove_stems() pattern. main() now
glob-discovers ai-docs/spec/**/*.md and rejects argv args. Ran migration: 6 spec files cleaned.
Idempotency and arg-rejection verified. No deviations.

### Phase 2: Add cross-file duplicate stem detection

Goals:
- After cleaning, build an in-memory stem registry across all spec files.
- Detect duplicate `{#YYMMDD-slug}` anchors appearing in more than one file (or twice in one file).
- Emit `[warn] duplicate stem <stem> — found in <file1>, <file2>`.

Suggested approach:
- Reuse the `{#[\w-]+}` regex already implicit in `ws-list-spec-stems`; extract all anchors from
  body text (skip fenced blocks, same guard as existing heading extractor).
- Collect into `dict[stem, list[filepath]]`; warn on any list with length > 1.

Success: running the tool on a corpus with a manually introduced duplicate stem emits the expected
warning; clean corpus emits nothing.

### Result (a3654ad) - 2026-04-25

Implemented two-pass design: cleanup loop runs first (process_file unchanged), then a second read
pass builds stem_registry via collect_stems(). STEM_RE tightened to \d{6}-[\w-]+ to exclude
template placeholder examples ({#YYMMDD-slug}) found in spec-system.md. Fenced-block exclusion
applied. Clean corpus emits nothing; duplicate detection verified with synthetic registry test.

### Phase 3: Add Implementation Gap staleness warning

Goals:
- Detect `> [!note] Implementation Gap · YYYY-MM-DD` callouts older than 90 days.
- Emit `[warn] Implementation Gap (<date>) in <file> — <N> days old`.

Suggested approach:
- Regex: `r'> \[!note\] Implementation Gap · (\d{4}-\d{2}-\d{2})'`
- Compare each matched date against `datetime.date.today()`; warn when delta > 90 days.
- Fenced-block exclusion: apply same guard used in heading extraction.

Success: a gap callout dated more than 90 days ago triggers a warning; one dated within 90 days
does not.

### Result (61143ba) - 2026-04-25

Implemented check_implementation_gaps() scanning spec files for
`> [!note] Implementation Gap · YYYY-MM-DD` callouts; warns when age > 90 days.
Fenced-block exclusion applied. Verified: 2025-01-01 (479 days) fires, today (0 days) silent.

### Phase 4: Add cross-doc anchor integrity check

Goals:
- Detect `{#YYMMDD-slug}` references in `ai-docs/mental-model/` files that do not correspond to
  any anchor present in `ai-docs/spec/`.
- Emit `[warn] {#<stem>} in mental-model/<file> — no matching anchor in spec/`.

Suggested approach:
- Build the spec stem registry (from Phase 2) first.
- Scan all `*.md` files under `ai-docs/mental-model/` for `{#[\w-]+}` occurrences.
- Any match not in the registry → warn.

Success: a manually broken reference in a mental-model file triggers a warning; all current
valid references pass silently.

### Result (61143ba) - 2026-04-25

Implemented check_mental_model_refs() using the spec stem registry from Phase 2.
Scans ai-docs/mental-model/**/*.md for {#YYMMDD-slug} anchors not in the registry.
STEM_RE shared with Phase 2 excludes template placeholders. Verified: ghost stem fires,
valid stem silent. Combined with Phase 3 in one commit.

### Phase 5: Update convention docs

Goals:
- `spec-conventions.md` Frontmatter section: reframe "auto-generated by ws-spec-build-index —
  never edit manually / run after every write" to reflect new purpose (repo-wide health check,
  no longer a frontmatter generator). Remove template `features:` placeholder.
- `ai-docs/mental-model/spec-system.md`: update `ws-spec-build-index` contract and usage
  description to match the new interface and output format.

Success: no remaining reference to `ws-spec-build-index <file>` in any doc; tool description
accurately reflects strip + validate behavior.
