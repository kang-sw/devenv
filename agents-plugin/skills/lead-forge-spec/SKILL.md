---
name: lead-forge-spec
description: Reconstruct specs from scratch by surveying the project, confirming behavioral domains, and guiding a user-confirmed authoring loop that produces complete anchor-keyed specs under ai-docs/spec/.
---

# Forge Spec

Target: user request

## Invariants

- Call `ws/convention.read(name: "spec-conventions")` before any spec write - conventions are canonical there.
- All survey queries spawn a native broad-scope Explore-style subagent via the `explore` playbook (see `lead-workflow-manual`); ticket-association checks spawn a scoped Explore-style subagent.
- Archive step (`git mv ai-docs/spec/*`) requires explicit user confirmation before executing.
- No spec entry is written without user confirmation of caller-visible status and implemented/planned classification.
- Call `ws/spec_stem.generate(slug: "<descriptive-slug>")` before every anchor insertion.
- Call `ws/spec_index.verify()` after every spec file write or update.
- Domain task labels use the prefix `forge-spec-<domain>` (e.g., `forge-spec-auth`).
- All survey Explore-style subagents for a phase are spawned in a single response turn when the host can issue parallel spawns; collect all results before synthesizing.

## On: invoke

1. Inspect the current visible task list, if present, for labels beginning with `forge-spec-`.
2. If matching tasks exist -> skip to **On: per-domain** with the first task whose status is not `completed`.
3. If no matching tasks exist -> proceed to **On: cold-start**.

## On: cold-start

### 1. Archive gate

1. List files currently under `ai-docs/spec/`. If the directory is empty or absent, skip to step 2.
2. Present the file list to the user and state that these files will be moved to `ai-docs/.old/spec/YYMMDD/` (today's date) and used as reference only - not as a base to extend. Ask the user to confirm before proceeding.
3. Wait for explicit user confirmation. Do not proceed on ambiguity.
4. On confirmation, execute:
   ```text
   YYMMDD=$(date +%y%m%d)
   mkdir -p ai-docs/.old/spec/$YYMMDD
   git mv ai-docs/spec/* ai-docs/.old/spec/$YYMMDD/
   ```

### 2. Parallel codebase survey

Spawn all four Explore-style subagents with broad-tracing scope in a single response turn via the `explore` playbook (see `lead-workflow-manual`). Collect all results before synthesizing.

Query 1 — directory and module structure:

```text
Survey the project's directory and module structure.

Enumerate source files, identify top-level modules/packages/service boundaries,
and return module names, paths, and purpose inferred from names/layout.
Format: markdown bullets grouped by module/area.
```

Query 2 — ticket domain survey:

```text
Survey all tickets through `ws/tickets.list(include_done: true, include_dropped: true)`.

Extract title, status directory, and public-facing or user-visible behavior.
Group by inferred behavioral domain.
Return: domain -> behaviors/features mentioned in tickets.
```

Query 3 — archived spec survey:

```text
Survey the archived spec files in ai-docs/.old/spec/ (most recent YYMMDD subdirectory).

Glob ai-docs/.old/spec/**/*.md. Extract title, summary, `##` headings, and
`🚧` status. Return domain names and heading topics as reference candidates
only; do not treat archived specs as authoritative.
```

Query 4 — commit history behavioral signals:

```text
Survey recent commit history for behavioral signals.

Use `ws/git.log`. Identify user-visible features, API changes, CLI
changes, or spec updates (`feat:`, `fix:`, `spec:`, spec-stems in bodies).
Return behavioral areas -> representative commits. Omit chore/docs/refactor
unless they reference spec-stems.
```

Collect each subagent result before synthesizing.

### 3. Synthesize domain candidates

1. Cross-reference module structure, ticket domains, archived specs, and commits.
2. Produce one candidate domain per significant caller-visible surface.
3. For each candidate, list covering archived spec files and representative behaviors.

### 4. User domain confirmation

Present the candidate domains to the user in a numbered list. Tell the user they may reorder, merge, split, rename, or drop entries before proceeding.

Wait for user response. Apply any adjustments. Do not proceed until the user explicitly confirms the final list.

### 5. Lock the task list

Create or refresh the visible Markdown task list with one entry per confirmed domain, in confirmed order:

```markdown
- [ ] forge-spec-<domain> - Source paths: <paths>; old spec files: <paths or none>
```

Proceed immediately to **On: per-domain** with the first domain.

## On: per-domain

For each domain task in order, skipping tasks with status `completed`:

### 1. Mark in-progress

Update the visible task list entry for this domain to in-progress.

### 2. Parallel domain survey

Spawn all four Explore-style subagents with broad-tracing scope in a single response turn via the `explore` playbook (see `lead-workflow-manual`). Collect all results before synthesizing.

Query 1 — domain source code:

```text
Survey source code for the <domain> domain.
Module paths: <paths from task description>

Read listed source paths. Identify caller-visible public functions, CLI commands,
HTTP endpoints, config options, output formats, events, and observable interfaces.
Return behaviors with status: implemented / partial / none visible.
```

Query 2 — domain tickets:

```text
Find tickets relevant to the <domain> domain.
Module paths: <paths from task description>

Use `ws/tickets.find(query: "<domain>")`; filter by module paths when needed.
Return features -> ticket status. Only contract-first ready implementation items, plus epic/research/workset planned decomposition, investigation text, or operating context, are `🚧` candidates; todo items are ticket-intent evidence.
```

Query 3 — archived specs for this domain:

```text
Survey the archived spec files for the <domain> domain.
Archived location: ai-docs/.old/spec/ (most recent YYMMDD subdirectory)
Old spec files for this domain: <files from task description, or scan all>

Read relevant archived specs. For each `##` heading, note feature name,
`🚧` status, and whether current source shows it.
Return features with archived status and current-source presence.
```

Query 4 — domain commit history:

```text
Survey commit history for the <domain> domain.
Module paths: <paths from task description>

Use path-filtered native Git history until ws exposes path-history. Identify commits that added or changed
caller-visible behavior (`feat:`, `fix:`, `spec:`, spec-stems).
Return behavioral changes newest first, with implementation status when visible.
```

Collect each subagent result before synthesizing.

### 3. Synthesize behavior brief

Combine the four returns into one bullet per distinct caller-visible behavior:
description, evidence source (code / ticket / old-spec / commit), candidate
classification (implemented / planned), and uncertainty flags.

### 4. User classification loop

Present the behavior brief to the user. For each item, establish:

1. **Caller-visible or internal-only?** - Internal behaviors are excluded from spec per `spec-conventions.md`. Ask on every ambiguous item.
2. **Implemented or planned?** - Implemented -> plain `{#slug}`. Contract-first planned implementation behavior -> `## 🚧 Feature {#slug}` only when backed by a non-`epic`, non-`research`, non-`workset` `ready/` ticket. Epic, research, or workset tickets may back only planned decomposition, investigation text, or operating context. Other planned work stays in ticket `## Spec Impact` or the survey report.

Ask on every ambiguous item. Do not classify without confirmation. Collect the
confirmed list before writing anything.

### 5. Write spec entries

1. Determine the target spec file path. Apply `judge: directory-vs-flat`.
2. Call `ws/convention.read(name: "spec-conventions")` before writing - read the output before proceeding.
3. For each confirmed behavior:
   a. Call `ws/spec_stem.generate(slug: "<descriptive-slug>")` to obtain `{#YYMMDD-slug}`.
   b. Write the spec entry using the `spec-format` template from `spec-conventions.md`.
   c. Place `🚧` after the heading marker if planned; omit if implemented.
4. After writing the file, verify it contains at least one `##` heading. If not, add a placeholder section and note it to the user.
5. Call `ws/spec_index.verify()`.
6. Apply `judge: directory-vs-flat` - if the written file warrants a directory split, note it as a split candidate for a follow-up lead-write-spec procedure invocation. Do not perform the split inline.

### 6. Associate stems with tickets

1. From the step 2 survey output, collect all tickets in `ready/` status relevant to this domain. If none, commit the spec file changes through `ws/git.commit` and skip to step 7.
2. Spawn a scoped Explore-style subagent via the `explore` playbook (see `lead-workflow-manual`) for the ticket-association check:

```text
Associate spec stems with tickets and check convention compliance.

Run first:
  Call `ws/convention.read(name: "ticket-conventions")`

Spec stems generated for this domain:
<list: {#YYMMDD-slug} - feature name, one per line>

Tickets to update (ready only):
<list: ai-docs/tickets/<status>/<stem>.md - one-line description>

For each ticket:
1. Read the ticket.
2. Merge relevant stems into `spec:` frontmatter; never overwrite existing entries.
3. Check body against conventions and fix issues in place.
4. Do not commit; caller owns git.
```

3. Collect the subagent result; review the ticket-association report and resolve any open questions with the user before committing.
4. Commit all domain changes in one commit: spec file + ticket association updates.

### 7. Complete domain

1. Mark the domain task complete in the visible task list.
2. If more domain tasks remain, continue with the next incomplete task from step 1 of **On: per-domain**.
3. When all domain tasks are `completed`, proceed to **On: wrap-up**.

## On: wrap-up

### 1. Final index pass

Call `ws/spec_index.verify()` as an idempotent safety pass over all spec files.

### 2. Summary report

Emit to the user:

```
## Forge Spec - Complete

Domains covered: <N>
Spec files created: <list of paths>
Total stems generated: <count>
  Implemented: <count>
  🚧 Planned: <count>
```

### 3. Suggested next steps

- Spawn a scoped Explore-style subagent via the `explore` playbook (see `lead-workflow-manual`) with a spec-updater brief to strip `🚧` markers from any planned features whose implementation has since landed in commit history.
- Review `🚧` entries with open tickets - confirm implementation behavior has a non-`epic`, non-`research`, non-`workset` `ready/` ticket, or that epic/research/workset backing documents only planned decomposition, investigation text, or operating context; otherwise drop the marker.
- Run the lead-write-spec procedure via `ws/playbook.print(name: "lead-write-spec")` for any domain surfaces discovered after wrap-up.

## Judgments

### judge: directory-vs-flat

| Decision | When |
|----------|------|
| Flat file `ai-docs/spec/<area>.md` | Single, self-contained surface - none of the split conditions below apply |
| Directory `ai-docs/spec/<area>/index.md` | Any one split condition is met: (1) a section has its own `🚧` markers with a distinct ticket lifecycle; (2) more than one `[!note] Constraints` block is present; (3) a section has a distinct audience from the parent doc |

When uncertain, start flat. Re-evaluate after writing - if a split condition fires, note the file for a follow-up lead-write-spec procedure invocation.

## Templates

### ws/spec_index.verify call

```text
ws/spec_index.verify()
```

No file arguments. Scans `ai-docs/spec/**/*.md` for duplicate anchors. Run once after any spec write or update in this session.

### Task list entry

```markdown
- [ ] forge-spec-<domain> - Source paths: <comma-separated module paths>; old spec files: <comma-separated archived spec paths, or none>
```

## Doctrine

Forge-spec optimizes for **confirmed spec entries per domain** - every produced
entry reflects an explicit user decision on caller-visibility and implementation
status. When ambiguous, require confirmation before writing spec content.
