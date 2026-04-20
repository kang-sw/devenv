---
title: "Spec-Driven Workflow: Spec-Stem System and Mandatory Gate"
plans:
  all-phases: 2026-04/21-0837.spec-driven-workflow
---

# Spec-Driven Workflow: Spec-Stem System and Mandatory Gate

## Background

Currently specs are optional outputs of ticket work — `/write-ticket` prompts for a spec update only when public behavior changes. This treats the spec as secondary to the ticket.

The correct model is spec-first: the spec is the authoritative blueprint of the public surface, and tickets are ephemeral work units that reference and implement spec commitments. The ticket describes the "path to reach" a spec feature, not the feature itself.

This ticket implements the structural changes needed to realize that model:
1. A spec-stem system for stable, canonical identifiers on spec headings.
2. A mandatory `/write-spec` gate after `/discuss`, with a judge inside write-spec deciding relevance.
3. Convention updates (tickets, commits, spec-updater) that flow from the spec-as-authority model.

## Decisions

**Spec → Ticket direction, not Ticket → Spec:**
Tickets reference spec-stems. Spec items do not reference tickets. This keeps spec independent of task management and enables multiple tickets to address the same spec feature.

**Auto-derived stems, not manually assigned:**
Stems are derived from file path + heading hierarchy. No authoring overhead. Agents use `list-stems` to query canonical stems before writing tickets or commits. Manual assignment was rejected: it adds cognitive overhead with no payoff given that heading renames are rare in practice.

**Stem format `path/to/file:heading.subheading`:**
`:` separates file path from heading hierarchy. `.` separates heading levels within the hierarchy. `/` within the file path component follows directory structure. `index.md` files are treated mechanically (`dir/index:heading`, no special-casing). This guarantees uniqueness across subdirectory-expanded spec structures.

**Judge in write-spec, not discuss:**
`/write-spec` always fires after `/discuss`. The `judge: spec-impact` inside write-spec's on-invoke decides whether spec work is needed. If not, write-spec exits immediately and suggests write-ticket. Rejected: judge in discuss — responsibility co-location is better, and unconditional routing eliminates the risk of the check being skipped.

**🚧 = construction marker only, no ticket link:**
Spec items no longer embed `[ticket-stem/pN]` references. Implementation traceability is grep-based: tickets and commits reference spec-stems. `spec-updater` verifies 🚧 items by grepping commit history for the spec-stem.

**Mental model stays post-facto and internal:**
Mental model documents describe current internal state only. No 🚧 markers. They capture the "how it works now" for implementers; spec captures the "what it should do" for callers.

## Phases

### Phase 1: Spec-stem tooling

Extend `spec-build-index` to generate a `stems:` field in spec frontmatter alongside the existing `features:` field. Add a `list-stems` bin command that reads this frontmatter and outputs canonical stem → heading-text pairs.

**Stem derivation rules:**
- Normalize heading text: lowercase, spaces → hyphens, strip non-alphanumeric characters (including 🚧, parentheses)
- Format: `<file-stem-path>:<h2-slug>[.<h3-slug>[...]]`
- File stem path: relative to `ai-docs/spec/`, without `.md` extension, `/`-separated
- All `##` and deeper headings get stems
- `index.md` in a subdirectory: `dir/index:heading` (no special-casing)

Example frontmatter output:
```yaml
stems:
  skills:workflow-skills: "## Workflow Skills"
  skills:workflow-skills.discussion: "### Discussion"
  skills:workflow-skills.discussion.discussion-loop: "### Discussion Loop"
```

**Acceptance criteria:**
- `spec-build-index <file>` regenerates both `features:` and `stems:` without losing existing frontmatter fields
- `list-stems <spec-file>` outputs the stems map
- Headings containing 🚧 and special characters produce valid, non-empty slug stems

### Phase 2: write-spec — mandatory gate and spec format change

Add `judge: spec-impact` as step 0 in write-spec's on-invoke:

```
0. judge: spec-impact
   - Evaluate: does this work introduce or modify behavior a caller can observe?
   - no  → output "No public behavior affected." Suggest /write-ticket. Exit.
   - yes → proceed with existing on-invoke flow
```

Remove ticket references from spec item format. Spec items carry only:
- `🚧` heading prefix for unimplemented features (no `[ticket-stem/pN]`)
- Body content describing caller-visible behavior

Update the spec-format template in write-spec to reflect this.

Depends on: Phase 1 (agents can call `list-stems` when writing new spec sections).

**Acceptance criteria:**
- write-spec invoked on an internal-only change exits with "No public behavior affected" and suggests write-ticket
- New spec sections written by write-spec contain no embedded ticket references
- `spec-build-index` runs successfully after each write

### Phase 3: /discuss — always suggest write-spec

Update /discuss on-user-signals-done:
- Remove conditional "did this involve public behavior?" prompt
- Always suggest `/write-spec` as the next step — write-spec's own judge handles relevance

Update the Workflow Context section in discuss to reflect the canonical chain: `discuss → write-spec → write-ticket`.

**Acceptance criteria:**
- /discuss always offers write-spec as the next step regardless of topic type

### Phase 4: Ticket and commit spec-stem convention

Add optional `spec:` field to ticket frontmatter template — present when the ticket implements one or more spec features:

```yaml
spec:
  - skills:workflow-skills.discussion.discussion-loop
  - skills:workflow-skills.spec-authoring
```

Add optional `## Spec` section to commit convention in CLAUDE.md:

```markdown
## Spec
- <spec-stem>   # one per affected spec feature; omit section if none
```

Update write-ticket to prompt for spec-stem references when the ticket implements a spec feature (after phase content is written).

Also add rename convention to commit rules: when a spec heading is renamed, the commit message must include `renamed-spec: <old-stem> → <new-stem>`. This is the only traceability mechanism for rename events (no alias machinery needed since renames are rare).

Depends on: Phase 1 (canonical stems available via list-stems).

**Acceptance criteria:**
- Ticket frontmatter template includes optional `spec:` field
- Commit convention in CLAUDE.md includes optional `## Spec` section
- write-ticket prompts for spec-stem references at the appropriate step

### Phase 5: spec-updater protocol

Update spec-updater's 🚧 verification protocol:

- Old: strip 🚧 when the linked `[ticket-stem/pN]` phase is complete
- New: for each 🚧 heading, derive its spec-stem; grep commit history for that stem; if found in a merged commit, prompt to remove 🚧

Stop flagging bare 🚧 markers as anomalies — bare 🚧 (no ticket link) is now the standard format.

Depends on: Phase 1 (spec-stems must exist), Phase 2 (🚧 markers have no ticket links).

**Acceptance criteria:**
- spec-updater identifies implemented features via commit history grep without requiring ticket links
- spec-updater does not flag bare 🚧 markers as anomalies

### Phase 6: Canonical flow docs and spec migration

Update `ai-docs/_index.md` canonical flows:

```
Full ceremony: /discuss → /write-spec → /write-ticket → /write-skeleton → /delegate-implement
```

(write-spec's judge handles the no-op case; the canonical chain is uniform regardless of topic type.)

Update enter-session briefing template to reflect write-spec as the mandatory second step after discuss.

Migrate existing spec files: remove embedded `[ticket-stem/pN]` references from all spec headings and body text. Confirm all 🚧 markers are bare after migration.

Depends on: Phases 2–5 complete.

**Acceptance criteria:**
- `_index.md` canonical flow reflects uniform `discuss → write-spec` chain
- enter-session briefing mentions write-spec as the post-discuss step
- No `[ticket-stem/pN]` references remain in any spec file under `ai-docs/spec/`
