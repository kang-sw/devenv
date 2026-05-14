---
name: lead-bootstrap
description: Bootstrap or upgrade a downstream project to AGENTS.md-based wsflow workflow context while preserving Claude compatibility.
---

# Bootstrap

Mode: user request

## Invariants

- Template sources are `AGENTS.template.md` and `WORKFLOW.md` in this skill directory; read both before any action.
- Canonical downstream workflow context is `AGENTS.md`.
- `CLAUDE.md` is a compatibility shim whose body is `@AGENTS.md`.
- Never overwrite project-specific sections: Architecture Rules, custom Code Standards entries, custom Project Knowledge entries.
- Merge surgically; flag unresolved conflicts inline with `<!-- CONFLICT: ... -->`.
- Every migration item is idempotent; re-running on an already-migrated project produces no changes.
- Template version history is package-local; apply only entries listed in this skill's `AGENTS.template.md`.
- Index health checks are advisory; first pass reads `_index.md` only.
- Index cleanup writes only `ai-docs/_index.md`; semantic migration routes through owning workflow skills.
- Commit each logical unit separately following the repository commit rules.
- Retired Claude plugin artifacts are out of support for this skill; do not reintroduce `claude-plugin/`.

## On: invoke

1. Read `AGENTS.template.md` and `WORKFLOW.md`.
2. Read downstream `AGENTS.md` if it exists.
3. Read downstream `CLAUDE.md` if it exists.
4. Detect mode:
   - **fresh** - neither root file exists.
   - **upgrade** - `AGENTS.md` has `<!-- Template Version: vNNNN -->`.
   - **adopt** - `AGENTS.md` exists without a version tag.
   - **claude-migrate** - `CLAUDE.md` exists and `AGENTS.md` does not.
5. Execute the matching handler.
6. Run the index health check when `ai-docs/_index.md` exists.

## On: fresh

1. Copy template to `AGENTS.md`, stripping template-internal migration blocks.
2. Leave placeholder markers in project-specific sections.
3. Create `ai-docs/` structure per the template setup block.
4. Copy `WORKFLOW.md` to `ai-docs/WORKFLOW.md`.
5. Add `ai-docs/**/*.local.md` and `ai-docs/.deps/` to `.gitignore` if absent.
6. Create `ai-docs/.old/` as a tracked project archive when project archive material exists.
7. Set `<!-- Template Version: vNNNN -->` to the latest version from the template.
8. Write `CLAUDE.md` with body `@AGENTS.md`.
9. Commit scaffolding.
10. Suggest `wsflow:lead-forge-spec` and `wsflow:lead-forge-mental-model` if baselines are absent.

## On: upgrade

1. Parse current version from `AGENTS.md`.
2. Walk this package's migration checklist items where version > current, in order.
3. Apply each item only when its condition is met.
4. Update `AGENTS.md` template-managed sections from `AGENTS.template.md`.
5. Ensure `ai-docs/WORKFLOW.md` exists from `WORKFLOW.md`; if a project-local guide already exists, preserve project additions and only merge missing bootstrap semantics.
6. Preserve or merge project-specific sections.
7. Ensure `CLAUDE.md` body is `@AGENTS.md`.
8. Update the template version tag.
9. Commit.

## On: adopt

1. Audit the current project against this package's baseline checklist.
2. Apply only missing baseline items.
3. Add the latest template version tag to `AGENTS.md`.
4. Proceed to the upgrade handler.

## On: claude-migrate

1. Create `AGENTS.md` from existing `CLAUDE.md` content.
2. Preserve project-specific sections and all existing template-version evidence.
3. Apply this package's baseline checklist.
4. Replace `CLAUDE.md` body with `@AGENTS.md`.
5. Preserve no separate Claude-only section unless explicitly requested.
6. Commit.

## On: index health check

1. Read `ai-docs/_index.md`.
2. Apply **judge: index-scope-drift** as a cheap first pass.
3. Do not read the full spec or mental-model corpus for this pass.
4. Do not move semantic content into specs, mental models, tickets, or refs.
5. Emit a concise health note when drift candidates exist.
6. Route user-approved second passes by the table below.

| Finding | Route |
|---------|-------|
| Source-derived detail | Compact to source pointers; use `wsflow:lead-discuss` if meaning is unclear |
| Behavior coverage | `wsflow:lead-forge-spec` or `wsflow:lead-write-spec` |
| Modification knowledge | `wsflow:lead-forge-mental-model` |
| Static reference material | Compact to `ai-docs/ref/` or API-doc pointers |
| Queue or ticket ordering | `wsflow:lead-write-ticket` |
| Work history | Compact to Git history, ticket archives, or roadmap pointers |
| Duplicated doc map | Compact to start-here pointers |
| Ambiguous project direction | `wsflow:lead-discuss` |

## On: user approves index cleanup

1. Re-read `ai-docs/_index.md`.
2. Preserve the memory-policy comment.
3. Keep summary, stack, workspace, build/test commands, read-before-edit pointers, active inventory, `ready/` queue, and compact notes.
4. Compact deep sections into links only when a clear owning document already exists.
5. Leave unique project direction, active priorities, and unresolved operational caveats in place.
6. Do not author or semantically update specs, mental models, tickets, or refs.
7. Report each compacted section with its replacement path or retained-note reason.

## Judgments

### judge: section-merge

| Decision | When |
|----------|------|
| Replace | Project section is identical to the old template or only trivially reformatted |
| Merge | Project section has meaningful additions that do not conflict with the new template |
| Conflict marker | Project section and template give incompatible instructions |

### judge: migration-condition

| Decision | When |
|----------|------|
| Skip | No matching files or no project state exists for the condition |
| Apply subset | Some matching files need the migration and others already satisfy it |
| Apply all | The condition is met across the affected project state |

### judge: index-scope-drift

Cheap `_index.md` scan only; report candidates, not confirmed defects.

| Candidate | Signal |
|-----------|--------|
| Source-derived detail | Deep source tree, file-by-file roles, type listings, or implementation inventory |
| Behavior inventory | Long "what works" lists, player-visible behavior descriptions, or feature semantics |
| Modification knowledge | Data flows, lifecycle narratives, extension recipes, common mistakes, audit/logging rules |
| Static reference material | Dependency API notes, archived design excerpts, or long external-reference summaries |
| Work history | Done/dropped tickets, completed milestones, or stale session chronology |
| Duplicated doc map | Long spec, mental-model, module, or ticket indexes beyond start-here pointers |

## Templates

### CLAUDE.md compatibility shim

```markdown
@AGENTS.md
```

## Doctrine

Bootstrap optimizes for **idempotent downstream baseline alignment**: preserve
project-specific content, apply only this package's versioned migrations, and
keep Claude compatibility as a shim over host-neutral `AGENTS.md`. When
ambiguous, preserve idempotency and downstream content.
