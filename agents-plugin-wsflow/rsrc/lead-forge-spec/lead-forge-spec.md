---
kind: print
delegates: true
---

# Forge Spec

Target: user request

## Invariants

- Call `{{.McpNamespace}}/convention.read(name: "spec-conventions")` before any spec write - conventions are canonical there.
- All survey queries spawn host-native broad-scope exploration workers directly with the listed query blocks as task prompts; ticket-association checks spawn a scoped exploration worker with a ticket-association prompt and cited evidence.
- Archive step (`git mv ai-docs/spec/*`) requires explicit user confirmation before executing.
- The once-per-run domain list requires explicit user confirmation before the todo list is locked.
- Ambiguous caller-visibility and implemented/planned calls are decided by best judgment and recorded with behavior name, chosen classification, and reason; every recorded item reaches the final summary, and those written to the spec also carry an inline `<!-- AMBIGUOUS: <reason> -->` marker.
- Call `{{.McpNamespace}}/spec_stem.generate(slug: "<descriptive-slug>")` before every anchor insertion.
- Call `{{.McpNamespace}}/spec_index.verify()` after every spec file write or update.
- Domain todo items use the key and title prefix `forge-spec-<domain>` (e.g., `forge-spec-auth`); the key is derivable from the domain name for status mutation.
- Domain todo item titles are resume state; read `Source paths:` and `old spec files:` from the exact title segments before each per-domain pass.
- All survey exploration workers for a phase are spawned in a single response turn when the host can issue parallel spawns; collect all results before synthesizing.

## On: invoke

1. Establish `<your lead key>`: call `{{.McpNamespace}}/workflow_manual(session_key: <your lead key>)` and execute the returned reference inline. No lead key yet? Call `{{.McpNamespace}}/workflow_manual(session_key: "obsidian-latch")` to bootstrap. After compaction, recover the key through `{{.SkillNamespace}}:lead-revive` first.
2. Call `{{.McpNamespace}}/todo.list(session_key: <your lead key>, mode: "full")` and scan rendered item titles for the `forge-spec-` prefix.
3. If matching todo items exist and at least one is not done (`- [x]`) -> skip to **On: per-domain** with the first not-done item.
4. If matching todo items exist and all are done (`- [x]`) -> proceed to **On: wrap-up**.
5. If no matching todo items exist -> proceed to **On: cold-start**.

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

Spawn all four host-native exploration workers with broad-tracing scope in a single response turn, using the query blocks below as task prompts. Require cited evidence, gaps, and follow-up needs. Collect all results before synthesizing.

Query 1 — directory and module structure:

```text
Survey the project's directory and module structure.

Enumerate source files, identify top-level modules/packages/service boundaries,
and return module names, paths, and purpose inferred from names/layout.
Format: markdown bullets grouped by module/area.
```

Query 2 — ticket domain survey:

```text
Survey all tickets through `{{.McpNamespace}}/tickets.query(include_done: true, include_dropped: true)`.

Extract title, status directory, and public-facing or user-visible behavior.
Group by inferred behavioral domain.
Return: domain -> behaviors/features mentioned in tickets.
```

Query 3 — archived spec survey:

```text
Survey the archived spec files in ai-docs/.old/spec/ (most recent YYMMDD subdirectory).

Glob ai-docs/.old/spec/**/*.md. Extract title, summary, and `##` headings.
Return domain names and heading topics as reference candidates
only; do not treat archived specs as authoritative.
```

Query 4 — commit history behavioral signals:

```text
Survey recent commit history for behavioral signals.

Use `{{.McpNamespace}}/git.log`. Identify user-visible features, API changes, CLI
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

### 5. Lock the todo list

Call `{{.McpNamespace}}/todo.clear(session_key: <your lead key>)`, then one `{{.McpNamespace}}/todo.append` per confirmed domain, in confirmed order.
The title schema is resume state: `Source paths:` stores comma-separated module paths; `old spec files:` stores comma-separated archived spec paths or `none`.

```text
{{.McpNamespace}}/todo.append(session_key: <your lead key>, key: "forge-spec-<domain>", title: "forge-spec-<domain> - Source paths: <paths>; old spec files: <paths or none>")
```

Proceed immediately to **On: per-domain** with the first domain.

## On: per-domain

For each domain task in order, skipping tasks whose status is done (`- [x]`):

### 1. Mark in-progress

Call `{{.McpNamespace}}/todo.check(session_key: <your lead key>, key: "forge-spec-<domain>", status: "wip")`.

### 2. Parallel domain survey

Spawn all four host-native exploration workers with broad-tracing scope in a single response turn, using the query blocks below as task prompts. Require cited evidence, gaps, and follow-up needs. Collect all results before synthesizing.

Query 1 — domain source code:

```text
Survey source code for the <domain> domain.
Module paths: <paths from the `Source paths:` segment of the todo item title>

Read listed source paths. Identify caller-visible public functions, CLI commands,
HTTP endpoints, config options, output formats, events, and observable interfaces.
Return behaviors with status: implemented / partial / none visible.
```

Query 2 — domain tickets:

```text
Find tickets relevant to the <domain> domain.
Module paths: <paths from the `Source paths:` segment of the todo item title>

Use `{{.McpNamespace}}/tickets.query(query: "<domain>")`; filter by module paths when needed.
Return features -> ticket status; todo items are ticket-intent evidence.
```

Query 3 — archived specs for this domain:

```text
Survey the archived spec files for the <domain> domain.
Archived location: ai-docs/.old/spec/ (most recent YYMMDD subdirectory)
Old spec files for this domain: <files from the `old spec files:` segment of the todo item title, or scan all when `none`>

Read relevant archived specs. For each `##` heading, note feature name
and whether current source shows it.
Return features with archived status and current-source presence.
```

Query 4 — domain commit history:

```text
Survey commit history for the <domain> domain.
Module paths: <paths from the `Source paths:` segment of the todo item title>

Use path-filtered native Git history until ws exposes path-history. Identify commits that added or changed
caller-visible behavior (`feat:`, `fix:`, `spec:`, spec-stems).
Return behavioral changes newest first, with implementation status when visible.
```

Collect each subagent result before synthesizing.

### 3. Synthesize behavior brief

Combine the four returns into one bullet per distinct caller-visible behavior:
description, evidence source (code / ticket / old-spec / commit), candidate
classification (implemented / planned), and uncertainty flags.

### 4. Classification pass

For each item in the behavior brief, decide autonomously using best judgment:

1. **Caller-visible or internal-only?** - Internal behaviors are excluded from spec per `spec-conventions.md`.
2. **Implemented or planned?** - Implemented -> plain `{#slug}`. Planned work is not written to the spec; it stays in the owning ticket's `## Spec Impact` or in the survey report.

When an item is genuinely ambiguous on either axis, classify it with the best-judgment call and record its behavior name, chosen classification, and a one-line reason in this run's ambiguity record. The wrap-up summary reports from that record, so it holds items excluded from the spec as internal-only or planned on the same terms as written ones. Do not stop to ask per item. Collect the classified list and the ambiguity record before writing anything.

### 5. Write spec entries

1. If step 4 left this domain with no behavior to write to the spec - every behavior classified planned, internal-only, or both - write no spec file: record the domain as producing no spec for the wrap-up summary and skip to step 6.
2. Determine the target spec file path. Apply `judge: directory-vs-flat`.
3. Call `{{.McpNamespace}}/convention.read(name: "spec-conventions")` before writing - read the output before proceeding.
4. For each behavior step 4 classified implemented - planned items are not written here:
   a. Call `{{.McpNamespace}}/spec_stem.generate(slug: "<descriptive-slug>")` to obtain `{#YYMMDD-slug}`.
   b. Write the spec entry using the `spec-format` template from `spec-conventions.md`.
   c. If step 4's classification pass flagged this item ambiguous, add `<!-- AMBIGUOUS: <reason> -->` on the line directly beneath the entry heading, and add the generated stem to that item's ambiguity record entry.
5. After writing the file, verify it contains at least one `##` heading. If not, add a placeholder section and note it to the user.
6. Call `{{.McpNamespace}}/spec_index.verify()`.
7. Apply `judge: directory-vs-flat` - if the written file warrants a directory split, note it as a split candidate for a follow-up lead-write-spec procedure invocation. Do not perform the split inline.

### 6. Associate stems with tickets

1. If step 5 wrote no spec file for this domain, there are no stems to associate and nothing to commit - skip to step 7 without spawning the worker below. Otherwise collect all tickets in `ready/` status relevant to this domain from the step 2 survey output; if none, commit the spec file through `{{.McpNamespace}}/git.commit` and skip to step 7.
2. Spawn a scoped exploration worker for the ticket-association check, using this block as the task prompt:

```text
Associate spec stems with tickets and check convention compliance.

Run first:
  Call `{{.McpNamespace}}/convention.read(name: "ticket-conventions")`

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
4. Commit all domain changes in one commit: the spec file step 5 wrote, plus ticket association updates.

### 7. Complete domain

1. Call `{{.McpNamespace}}/todo.check(session_key: <your lead key>, key: "forge-spec-<domain>", status: "done")`.
2. If more domain todo items remain, continue with the next incomplete one from step 1 of **On: per-domain**.
3. When every `forge-spec-` todo item is done, proceed to **On: wrap-up**.

## On: wrap-up

### 1. Final index pass

Call `{{.McpNamespace}}/spec_index.verify()` as an idempotent safety pass over all spec files.

### 2. Summary report

Read the ambiguity list from step 4's ambiguity record - it is the authoritative source, and items classified internal-only or planned live there and carry no marker. Name each one by its generated stem when step 5 wrote one, otherwise by its behavior name.

A resumed run holds no ambiguity record for domains classified in an earlier session. Reconstruct those entries from the `<!-- AMBIGUOUS: <reason> -->` markers under `ai-docs/spec/`, and label what the reconstruction cannot complete instead of emitting it as a finished count: append `(reconstructed from spec markers - items classified planned or internal-only carry no marker and are not recoverable)` to the ambiguity line, and `(not recorded in this session)` to the domains-with-no-spec-file line.

Emit to the user:

```
## Forge Spec - Complete

Domains covered: <N>
Spec files created: <list of paths>
Domains with no spec file written: <list, or none>
Total stems generated: <count>
Ambiguous classifications (auto-decided, review recommended): <count>
  <stem or behavior name> - <implemented|planned|internal-only> - <reason>
  ...
```

### 3. Chain into lead-forge-mental-model

Ask the user whether to run `lead-forge-mental-model` next, regardless of how
this `lead-forge-spec` run was reached (a direct standalone invocation,
`lead-bootstrap`'s fresh-install suggestion, or the index-health-check
routing table). On a yes answer, call
`{{.McpNamespace}}/playbook.read(name: "lead-forge-mental-model")` and
execute the returned procedure inline. On a no answer or no response,
continue to step 4 without invoking it.

### 4. Suggested next steps

- Run the lead-write-spec procedure via `{{.McpNamespace}}/playbook.read(name: "lead-write-spec")` for any domain surfaces discovered after wrap-up.

## Judgments

### judge: directory-vs-flat

| Decision | When |
|----------|------|
| Flat file `ai-docs/spec/<area>.md` | Single, self-contained surface - none of the split conditions below apply |
| Directory `ai-docs/spec/<area>/index.md` | Any one split condition is met: (1) more than one `[!note] Constraints` block is present; (2) a section has a distinct audience from the parent doc |

When uncertain, start flat. Re-evaluate after writing - if a split condition fires, note the file for a follow-up lead-write-spec procedure invocation.

## Templates

### {{.McpNamespace}}/spec_index.verify call

```text
{{.McpNamespace}}/spec_index.verify()
```

No file arguments. Scans `ai-docs/spec/**/*.md` for duplicate anchors. Run once after any spec write or update in this session.

### Todo entry

```text
{{.McpNamespace}}/todo.append(session_key: <your lead key>, key: "forge-spec-<domain>", title: "forge-spec-<domain> - Source paths: <comma-separated module paths>; old spec files: <comma-separated archived spec paths, or none>")
```

## Doctrine

Forge-spec optimizes for **low-friction throughput per domain** while keeping the
two high-leverage decisions - the archive gate and the once-per-run domain list -
explicitly user-confirmed. Per-item caller-visibility and implementation-status
classification is decided autonomously and surfaced for review afterward rather
than spending a confirmation turn on each item.
