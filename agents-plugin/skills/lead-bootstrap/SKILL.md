---
name: lead-bootstrap
description: Bootstrap or upgrade a downstream project to AGENTS.md-based ws workflow context while preserving Claude compatibility.
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
- Commit each logical unit separately following the repository commit rules.
- Claude plugin artifacts are out of support for this skill; do not edit `claude-plugin/`.

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

## On: fresh

1. Copy template to `AGENTS.md`, stripping template-internal migration blocks.
2. Leave placeholder markers in project-specific sections.
3. Create `ai-docs/` structure per the template setup block.
4. Copy `WORKFLOW.md` to `ai-docs/WORKFLOW.md`.
5. Add `ai-docs/**/*.local.md` to `.gitignore` if not present.
6. Set `<!-- Template Version: vNNNN -->` to the latest version from the template.
7. Write `CLAUDE.md` with body `@AGENTS.md`.
8. Commit scaffolding.
9. Suggest `ws:lead-forge-spec` and `ws:lead-forge-mental-model` if baselines are absent.

## On: upgrade

1. Parse current version from `AGENTS.md`.
2. Walk migration checklist items where version > current, in order.
3. Apply each item only when its condition is met.
4. Update `AGENTS.md` template-managed sections from `AGENTS.template.md`.
5. Ensure `ai-docs/WORKFLOW.md` exists from `WORKFLOW.md`; if a project-local guide already exists, preserve project additions and only merge missing bootstrap semantics.
6. Preserve or merge project-specific sections.
7. Ensure `CLAUDE.md` body is `@AGENTS.md`.
8. Update the template version tag.
9. Commit.

## On: adopt

1. Audit v0001 through latest against current project state.
2. Apply only missing migration items.
3. Add the latest template version tag to `AGENTS.md`.
4. Proceed to the upgrade handler.

## On: claude-migrate

1. Create `AGENTS.md` from existing `CLAUDE.md` content.
2. Preserve project-specific sections and all existing template-version evidence.
3. Apply the AGENTS migration checklist in order.
4. Replace `CLAUDE.md` body with `@AGENTS.md`.
5. Preserve no separate Claude-only section unless explicitly requested.
6. Commit.

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

## Templates

### CLAUDE.md compatibility shim

```markdown
@AGENTS.md
```

## Doctrine

Bootstrap optimizes for **idempotent downstream migration**: preserve
project-specific content, apply only missing versioned migrations, and keep
Claude compatibility as a shim over host-neutral `AGENTS.md`. When ambiguous,
preserve idempotency and downstream content.
