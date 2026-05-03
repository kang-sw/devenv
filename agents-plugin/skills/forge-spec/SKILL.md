---
name: forge-spec
description: Reconstruct specs from scratch by surveying the project, confirming behavioral domains, and guiding a user-confirmed authoring loop that produces complete anchor-keyed specs under ai-docs/spec/.
---

# Forge Spec

## Invariants

- Call `ws/convention.read` for `spec-conventions` before any spec write.
- Use `ws/subquery` with `deep_research: true` for every survey delegate call.
- Archive existing spec files only after explicit user confirmation.
- Confirm the final domain list with the user before registering domain work.
- Maintain a visible task list with one `forge-spec-<domain>` item per confirmed domain.
- No spec entry is written without user confirmation of caller-visible status and implemented/planned classification.
- Call `ws/spec_stem.generate` before every anchor insertion.
- Call `ws/spec_index.verify` after every spec file write or update.
- Dispatch all survey delegates for a phase in one response turn when the host supports parallel calls.
- Use `ws/subquery` only for the clerk-style ticket association step, and pass self-contained inline instructions.
- Commit generated spec and ticket association changes once per completed domain.
- Keep all AI-authored spec, ticket, and commit text in English.
- Do not read convention files from host-local plugin source paths.

## On: Invoke

1. Inspect the visible task list for items whose names begin with `forge-spec-`.
2. If an incomplete matching item exists, resume **On: Per Domain** with the first incomplete item.
3. If no matching item exists, start **On: Cold Start**.

## On: Cold Start

### 1. Archive gate

1. List files currently under `ai-docs/spec/`.
2. If `ai-docs/spec/` is empty or absent, skip to **2. Parallel codebase survey**.
3. Present the file list and state that these files will move to `ai-docs/ref/old-spec/YYMMDD/` as reference-only material.
4. Ask the user to confirm the archive move before proceeding.
5. Wait for explicit confirmation; do not proceed on ambiguity.
6. On confirmation, create `ai-docs/ref/old-spec/YYMMDD/` and move existing spec files there with version-control-aware file moves when available.

### 2. Parallel codebase survey

Call `ws/subquery` four times in the same response turn, each with `deep_research: true`:

1. Survey project directory and module structure; return module names, paths, and inferred purposes.
2. Survey all tickets under `ai-docs/tickets/`; group public-facing behaviors by apparent domain.
3. Survey archived specs under `ai-docs/ref/old-spec/`; list domains, heading topics, and planned versus implemented markers as reference candidates only.
4. Survey recent commit history; group user-visible features, API changes, CLI changes, and spec-related commits by behavioral area.

Wait for all four survey results before synthesizing.

### 3. Synthesize domain candidates

1. Cross-reference module structure, ticket domains, archived specs, and commit areas.
2. Produce one candidate domain per significant caller-visible surface.
3. For each candidate, note inferred source paths, archived spec files if any, and representative behaviors.

### 4. User domain confirmation

1. Present candidate domains as a numbered list.
2. Tell the user they may reorder, merge, split, rename, or drop entries.
3. Wait for the user's adjustments or confirmation.
4. Do not proceed until the user explicitly confirms the final domain list.

### 5. Register domain work

1. Create or update a visible task list with one item per confirmed domain in confirmed order.
2. Name each item `forge-spec-<domain>`.
3. Include the domain name, inferred source paths, and relevant archived spec files in each item description.
4. Mark the first domain `in_progress` and proceed to **On: Per Domain**.

## On: Per Domain

For each visible task-list item named `forge-spec-<domain>`, in confirmed order, skip completed items.

### 1. Mark in progress

1. Mark the current domain task `in_progress` in the visible task list.
2. Read the domain task description for source paths and archived spec references.

### 2. Parallel domain survey

Call `ws/subquery` four times in the same response turn, each with `deep_research: true`:

1. Survey the domain source paths; identify caller-visible behaviors and implementation status.
2. Survey tickets relevant to the domain keywords and source paths; list features with ticket status.
3. Survey archived specs relevant to the domain; list old features, planned markers, and current-source presence.
4. Survey commit history for the domain source paths; list behavior-changing commits and implementation signals.

Wait for all four survey results before synthesizing.

### 3. Synthesize behavior brief

1. Combine survey results into one bullet per distinct caller-visible behavior.
2. For each bullet, include behavior description, evidence sources, and candidate classification.
3. Mark uncertain caller-visible status or implementation status explicitly.

### 4. User classification loop

1. Present the behavior brief to the user before writing any spec content.
2. For every item, confirm whether it is caller-visible or internal-only.
3. Exclude internal-only behavior from spec output.
4. For every caller-visible item, confirm whether it is implemented or planned.
5. Treat implemented behavior as a plain `{#slug}` entry and planned behavior as a `🚧 {#slug}` entry.
6. Ask about every ambiguous item; do not classify by assumption.
7. Collect the confirmed list before writing anything.

### 5. Write spec entries

1. Determine the target spec file path with `judge: directory-vs-flat`.
2. Call MCP tool `ws/convention.read` with `{"name":"spec-conventions"}` and read the result.
3. For each confirmed caller-visible behavior, call MCP tool `ws/spec_stem.generate` with `{"slug":"<descriptive-slug>"}`.
4. Write each entry using the loaded spec conventions and `Templates / Spec Entry`.
5. Prefix the heading with `🚧` only for user-confirmed planned behavior.
6. Verify the written file contains at least one `##` heading; if not, add a placeholder section and report it.
7. Call MCP tool `ws/spec_index.verify`.
8. Apply `judge: directory-vs-flat` again and note any split candidate for a follow-up spec workflow; do not split inline.

### 6. Associate stems with tickets

1. Collect relevant `wip/` and `todo/` tickets from the domain survey output.
2. If no relevant active tickets exist, commit the domain spec changes and skip to **7. Complete domain**.
3. Call MCP tool `ws/subquery` for clerk-style ticket association with a self-contained inline prompt using `Templates / Ticket Association Prompt`.
4. Do not pass a prompt preset or `prompt_refs`; include all conventions and task instructions in the inline prompt.
5. Review the ticket association report.
6. Resolve any open questions with the user before committing ticket association changes.
7. Commit the spec file and ticket association changes together.

### 7. Complete domain

1. Mark the current domain task `completed` in the visible task list.
2. If more incomplete domain tasks remain, continue with the next one.
3. When all domain tasks are complete, proceed to **On: Wrap Up**.

## On: Wrap Up

1. Call MCP tool `ws/spec_index.verify` as an idempotent final pass.
2. Report the number of domains covered, spec files created, total stems generated, implemented entries, and planned entries.
3. Suggest running the configured spec-update workflow to strip any planned markers whose implementation has since landed.
4. Suggest reviewing planned entries with open tickets and running the write-spec workflow for surfaces discovered after wrap-up.

## Judgments

### judge: directory-vs-flat

| Decision | When |
|----------|------|
| Flat file `ai-docs/spec/<area>.md` | Single self-contained surface with no split condition |
| Directory `ai-docs/spec/<area>/index.md` | A section has its own planned-marker lifecycle, more than one Constraints callout is present, or a section has a distinct audience |

When uncertain, start flat. Re-evaluate after writing and record follow-up split candidates instead of splitting inline.

## Templates

### Spec Entry

```markdown
## <Feature Name> {#YYMMDD-feature-name}

Behavioral description of what users, callers, hosts, or tools observe.

> [!note] Implementation Gap · YYYY-MM-DD
> Known-but-unscheduled incomplete behavior. No ticket yet.

> [!note] Planned 🚧
> Planned behavior. Current behavior remains unchanged until implemented.
```

Use a `🚧` heading for a fully planned feature entry. Use the Implementation Gap callout for confirmed implemented behavior with a known incomplete edge. Use the Planned callout for planned changes to existing behavior.

### Visible Task Item

```text
forge-spec-<domain> — <status>
  Domain: <domain>
  Source paths: <comma-separated module paths>
  Old spec files: <comma-separated archived spec paths, or none>
```

### Ticket Association Prompt

```markdown
Associate spec stems with tickets and check convention compliance.

First call `ws/convention.read` with `{"name":"ticket-conventions"}` and follow the returned conventions.

Spec stems generated for this domain:
<list: {#YYMMDD-slug} — feature name, one per line>

Tickets to update, wip/todo only:
<list: ai-docs/tickets/<status>/<stem>.md — one-line description>

For each ticket:
1. Read the ticket file.
2. Add or update the `spec:` frontmatter field with relevant stems.
3. Merge with existing `spec:` entries; never overwrite existing stems.
4. Fix convention issues in place.
5. Do not commit; the caller handles git operations.

Return a `## Ticket association report` with files changed, stems associated, convention fixes, and open questions.
```

### Completion Report

```text
## Forge Spec — Complete

Domains covered: <N>
Spec files created: <list of paths>
Total stems generated: <count>
  Implemented: <count>
  🚧 Planned: <count>
```

## Doctrine

Forge-spec optimizes for confirmed spec entries per domain: every produced entry reflects an explicit user decision on caller visibility and implementation status before text lands in the spec corpus. When a rule is ambiguous, apply whichever interpretation more reliably requires explicit user confirmation before any spec content is written.
