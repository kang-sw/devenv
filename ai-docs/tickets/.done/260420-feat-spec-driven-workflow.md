---
title: "Spec-Driven Workflow: Spec-Stem System and Mandatory Gate"
plans:
  phase-1: 2026-04/21-1040.spec-driven-workflow-p1-5.survey
  phase-2: 2026-04/21-1040.spec-driven-workflow-p1-5.survey
  phase-3: 2026-04/21-1040.spec-driven-workflow-p1-5.survey
  phase-4: 2026-04/21-1040.spec-driven-workflow-p1-5.survey
  phase-5: 2026-04/21-1040.spec-driven-workflow-p1-5.survey
completed: 2026-04-24
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

**Explicit `{#slug}` anchors, not auto-derived stems:**
Each spec heading that represents a named feature carries an explicit `{#slug}` anchor. The heading display text is the description; the slug is the stable identity. Auto-derivation from heading text was rejected: long headings produce verbose, unreadable stems, and the display text / identity separation has real payoff (display can evolve without breaking stems).

**Stem format `path/to/file:slug`:**
`:` separates file path from slug. `/` within the file path follows directory structure. `index.md` files are treated mechanically (`dir/index:slug`). The slug is the `{#slug}` value — no normalization needed since it is already authored as a clean identifier. Heading hierarchy is encoded by dotting parent slug onto child slug: `path:parent.child`.

**Judge in write-spec, not discuss:**
`/write-spec` always fires after `/discuss`. The `judge: spec-impact` inside write-spec's on-invoke decides whether spec work is needed. If not, write-spec exits immediately and suggests write-ticket. Rejected: judge in discuss — responsibility co-location is better, and unconditional routing eliminates the risk of the check being skipped.

**🚧 = construction marker only, no ticket link:**
Spec items no longer embed `[ticket-stem/pN]` references. Implementation traceability is grep-based: tickets and commits reference spec-stems. `spec-updater` verifies 🚧 items by grepping commit history for the spec-stem.

**Mental model stays post-facto and internal:**
Mental model documents describe current internal state only — no 🚧 markers. They capture the "how it works now" for implementers; spec captures the "what it should do" for callers.

## Phases

### Phase 1: Spec-stem tooling

Extend `spec-build-index` to generate a `stems:` field in spec frontmatter alongside the existing `features:` field. Add a `list-spec-stems` bin command that reads this frontmatter and outputs canonical stems.

**Heading anchor convention:**

```markdown
## The Feature Display Name {#feature-slug}

### Sub-feature Long Title {#sub-slug}
```

- Headings with `{#slug}` are indexed; headings without are skipped (treated as organizational).
- The slug is the authored identifier — no normalization applied.
- Stem = `<file-stem-path>:<parent-slug>.<child-slug>` for nested headings, or `<file-stem-path>:<slug>` for top-level.
- File stem path: relative to `ai-docs/spec/`, without `.md` extension, `/`-separated. `index.md` → `dir/index`.

**Example frontmatter output:**

```yaml
stems:
  skills:workflow-skills: "## Workflow Skills"
  skills:workflow-skills.discussion: "### Discussion"
  skills:workflow-skills.discussion.discussion-loop: "### Discussion Loop"
```

**`list-spec-stems` command:**

- `list-spec-stems <spec-file>` — outputs stem keys only (for referencing in tickets/commits)
- `list-spec-stems -v <spec-file>` — outputs stem + heading display text

**Acceptance criteria:**
- `spec-build-index <file>` regenerates both `features:` and `stems:` without losing existing frontmatter fields
- `list-spec-stems ai-docs/spec/skills.md` outputs stem keys
- `list-spec-stems -v ai-docs/spec/skills.md` outputs stem + heading text pairs
- Headings without `{#slug}` are silently skipped (no error)

### Result (fe2fda5) - 2026-04-21

- `spec-build-index` extended: strips `{#slug}` anchors from `features:` display text; generates `stems:` YAML map keyed by canonical stem.
- `claude/bin/list-spec-stems` added: Python script following `list-mental-model` pattern; `-v` flag adds display text; computes spec-relative path from caller's working directory.
- Review fix: `list-spec-stems` was not robust to callers below repo root; `d7e3b61` adds CWD-independent path resolution.

### Phase 2: write-spec — mandatory gate and spec format change

Add `judge: spec-impact` as step 0 in write-spec's on-invoke:

```
0. judge: spec-impact
   - Evaluate: does this work introduce or modify behavior a caller can observe?
   - no  → output "No public behavior affected." Suggest /write-ticket. Exit.
   - yes → proceed with existing on-invoke flow
```

Update write-spec to:
- Require `{#slug}` anchors on all new feature headings.
- Stop embedding ticket references in spec items. Spec items carry only `🚧` prefix (no `[ticket-stem/pN]`).
- Run `spec-build-index` after every write to keep `stems:` current.

Update the spec-format template in write-spec to reflect this.

Depends on: Phase 1.

**Acceptance criteria:**
- write-spec invoked on an internal-only change exits with "No public behavior affected" and suggests write-ticket
- New spec sections written by write-spec include `{#slug}` anchors and no embedded ticket references
- `spec-build-index` runs successfully after each write and `stems:` is populated

### Result (fe2fda5) - 2026-04-21

- `write-spec/SKILL.md`: `judge: spec-impact` added as step 0 in on-invoke; no-public-behavior path exits immediately and suggests write-ticket.
- `{#slug}` anchor requirement added to spec-format template; embedded ticket refs removed from template.
- `spec-conventions.md` updated: `## 🚧 Markers` section and template block reflect new bare-🚧 + `{#slug}` format (no `[ticket-stem/pN]`).

### Phase 3: /discuss — always suggest write-spec

Update /discuss on-user-signals-done:
- Remove the conditional "did this involve public behavior?" prompt.
- Always suggest `/write-spec` as the next step — write-spec's own judge handles relevance.

Update the Workflow Context section in discuss to reflect the canonical chain: `discuss → write-spec → write-ticket`.

**Acceptance criteria:**
- /discuss always offers write-spec as the next step regardless of topic type

### Result (fe2fda5) - 2026-04-21

- `discuss/SKILL.md`: on-user-signals-done now unconditionally suggests `/write-spec`; Workflow Context section updated to `discuss → write-spec → write-ticket`.

### Phase 4: Ticket and commit spec-stem convention

Add optional `spec:` field to ticket frontmatter template — present when the ticket implements one or more spec features:

```yaml
spec:
  - skills:workflow-skills.discussion.discussion-loop
  - skills:workflow-skills.spec-authoring
```

Add optional `## Spec` section to the commit convention in `claude/skills/bootstrap/CLAUDE.template.md` (the template downstream projects adopt via `/bootstrap`):

```markdown
## Spec
- <spec-stem>   # one per affected spec feature; omit section if none
```

Add rename convention to the same template: when a spec heading's `{#slug}` changes, the commit message must include `renamed-spec: <old-stem> → <new-stem>`. This is the sole traceability mechanism for slug renames.

Add a migration checklist item to `CLAUDE.template.md` so existing downstream projects pick up the convention on next `/bootstrap upgrade`:

```
- vNNNN: If Commit Rules lack a `## Spec` section, add it after `## Ticket Updates`.
         Format: `- <spec-stem>  # one per affected spec feature; omit section if none`.
         Also add rename convention: "When a spec heading's {#slug} changes, include
         `renamed-spec: <old-stem> → <new-stem>` in the commit message."
```

Update write-ticket to prompt for spec-stem references when the ticket implements a spec feature (use `list-spec-stems` to confirm canonical stems before writing).

Depends on: Phase 1.

**Acceptance criteria:**
- `CLAUDE.template.md` commit convention includes optional `## Spec` section and rename convention
- `CLAUDE.template.md` migration checklist includes a vNNNN item for existing downstream projects
- Ticket frontmatter template includes optional `spec:` field
- write-ticket prompts for spec-stems at the appropriate step

### Result (fe2fda5) - 2026-04-21

- `claude/skills/bootstrap/CLAUDE.template.md`: `## Spec` section added to commit convention; rename convention added; v0023 migration checklist item added.
- `claude/infra/ticket-conventions.md`: optional `spec:` field added to frontmatter template (after `related:`, before `parent:`).
- `claude/skills/write-ticket/SKILL.md`: prompts for spec-stem references when ticket implements a spec feature.
- Deviation: survey identified `claude/CLAUDE.md` as target; corrected to `claude/skills/bootstrap/CLAUDE.template.md` per user directive (downstream projects adopt via `/bootstrap`, not directly from the thinking-doctrine CLAUDE.md).

### Phase 5: spec-updater protocol

Update spec-updater's 🚧 verification protocol:

- Old: strip 🚧 when the linked `[ticket-stem/pN]` phase is complete.
- New: for each 🚧 heading, extract its spec-stem from `{#slug}`; grep commit history for that stem in `## Spec` sections; if found in a merged commit, prompt to remove 🚧.

Stop flagging bare 🚧 markers (no ticket link) as anomalies — bare 🚧 is now the standard format.

Depends on: Phase 1, Phase 2.

**Acceptance criteria:**
- spec-updater identifies implemented features via commit history grep without requiring ticket links
- spec-updater does not flag bare 🚧 markers as anomalies

### Result (fe2fda5) - 2026-04-21

- `claude/agents/spec-updater.md`: verification protocol rewritten to grep commit history for spec-stem in `## Spec` sections; `### Untracked 🚧` output category removed; bare 🚧 markers no longer flagged as anomalies.

### Phase 6: Canonical flow docs and spec migration

Update `ai-docs/_index.md` canonical flows:

```
Full ceremony: /discuss → /write-spec → /write-ticket → /write-skeleton → /delegate-implement
```

write-spec's judge handles the no-op case; the canonical chain is uniform regardless of topic type.

Update enter-session briefing template to reflect write-spec as the mandatory second step after discuss.

Migrate existing spec files: add `{#slug}` anchors to existing feature headings; remove embedded `[ticket-stem/pN]` references from all headings and body text; run `spec-build-index` on each file after migration.

Depends on: Phases 2–5 complete.

**Acceptance criteria:**
- `_index.md` canonical flow reflects the uniform `discuss → write-spec` chain
- enter-session briefing mentions write-spec as the post-discuss step
- All headings in `ai-docs/spec/` that represent named features carry `{#slug}` anchors
- No `[ticket-stem/pN]` references remain in any spec file
- `stems:` frontmatter is populated in all spec files after migration

### Result — cancelled 2026-04-24

Canonical flows already current; spec migration not required. Closed unimplemented.
