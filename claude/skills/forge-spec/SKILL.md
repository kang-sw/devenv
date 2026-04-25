---
name: forge-spec
description: >
  From-scratch spec reconstruction. Surveys the codebase, identifies
  behavioral domains, and guides a user-confirmed authoring loop that
  produces a complete, anchor-keyed spec under ai-docs/spec/.
disable-model-invocation: true
argument-hint: "[optional: starting domain name]"
---

# Forge Spec

Target: $ARGUMENTS

## Invariants

- Run `ws-print-infra spec-conventions.md` (Bash) before any spec write — conventions are canonical there.
- Every `Agent()` dispatch carries explicit `model = "sonnet"` — never inherited.
- Archive step (`git mv ai-docs/spec/*`) requires explicit user confirmation before executing.
- No spec entry is written without user confirmation of caller-visible status and implemented/planned classification.
- Run `ws-generate-spec-stem <descriptive-slug>` before every anchor insertion.
- Verify each spec file has at least one `##`-or-deeper heading before calling `ws-spec-build-index` — the tool prints a skip notice to stdout and returns without modifying the file.
- Run `ws-spec-build-index <file>` after every spec file write or update.
- Domain task names use the prefix `forge-spec-<domain>` (e.g., `forge-spec-auth`).
- All survey subagents for a phase are dispatched in a single response turn (parallel).

## On: invoke

1. Call `TaskList` and scan for tasks whose name begins with `forge-spec-`.
2. If matching tasks exist → skip to **On: per-domain** with the first task whose status is not `completed`.
3. If no matching tasks exist → proceed to **On: cold-start**.

## On: cold-start

### 1. Archive gate

1. List files currently under `ai-docs/spec/`. If the directory is empty or absent, skip to step 2.
2. Present the file list to the user and state that these files will be moved to `ai-docs/ref/old-spec/YYMMDD/` (today's date) and used as reference only — not as a base to extend. Ask the user to confirm before proceeding.
3. Wait for explicit user confirmation. Do not proceed on ambiguity.
4. On confirmation, execute:
   ```bash
   YYMMDD=$(date +%y%m%d)
   mkdir -p ai-docs/ref/old-spec/$YYMMDD
   git mv ai-docs/spec/* ai-docs/ref/old-spec/$YYMMDD/
   ```

### 2. Parallel codebase survey

Dispatch all four survey subagents in a single response turn:

```
Agent(
  name = "survey-structure",
  description = "Survey directory and module structure",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey the project's directory and module structure.

    Steps:
    1. Run `ls -R` or `find . -type f -name '*.md' -o -name '*.py' -o -name '*.ts'`
       on the working directory to enumerate the source tree layout.
    2. Identify top-level modules, packages, or service boundaries.
    3. Return a structured summary: module names, paths, brief description
       of purpose derived from file names and directory layout.

    Format your response as a markdown bullet list grouped by module/area.
  """
)

Agent(
  name = "survey-tickets",
  description = "Survey all tickets for behavioral signals",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey all tickets under ai-docs/tickets/ (all statuses: idea/, todo/,
    wip/, done/, dropped/).

    Steps:
    1. Glob ai-docs/tickets/**/*.md and read each file.
    2. Extract: ticket title, status directory, and any behavior or feature
       described as public-facing or user-visible.
    3. Group by apparent behavioral domain (infer from ticket title and
       content).

    Return a grouped list: domain → behaviors/features mentioned in tickets.
  """
)

Agent(
  name = "survey-old-spec",
  description = "Survey archived spec files for domain candidates",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey the archived spec files in ai-docs/ref/old-spec/ (most recent
    YYMMDD subdirectory).

    Steps:
    1. Glob ai-docs/ref/old-spec/**/*.md and read each file.
    2. For each file: extract the title, summary, and all ## headings.
    3. Note which features are marked 🚧 (planned) vs unmarked (implemented).

    Return: a flat list of domain names found, with their heading topics.
    These are reference candidates only — do not treat them as authoritative.
  """
)

Agent(
  name = "survey-commits",
  description = "Survey recent commit history for behavioral signals",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey recent commit history for behavioral signals.

    Steps:
    1. Run `git log --oneline -100`.
    2. Identify commits that mention user-visible features, API changes,
       CLI changes, or spec updates (look for feat:, fix:, spec: prefixes
       and commit bodies referencing spec-stems).
    3. Group commit messages by apparent behavioral area.

    Return: a grouped list of behavioral areas → representative commit
    messages. Omit chore/docs/refactor commits unless they reference
    spec-stems.
  """
)
```

Wait for all four subagents to return.

### 3. Synthesize domain candidates

Combine the four survey returns:

1. Cross-reference module structure with ticket domains and commit areas.
2. Produce a candidate domain list — one domain per significant caller-visible surface.
3. For each candidate: note the spec files that covered it (if any) and representative behaviors.

### 4. User domain confirmation

Present the candidate domains to the user in a numbered list. Tell the user they may reorder, merge, split, rename, or drop entries before proceeding.

Wait for user response. Apply any adjustments. Do not proceed until the user explicitly confirms the final list.

### 5. Lock the task list

Call `TaskCreate` once — one task per confirmed domain, in confirmed order:

```
TaskCreate(
  name = "forge-spec-<domain>",
  description = """
    Spec authoring for domain: <domain>
    Source paths: <inferred module paths for this domain>
    Old spec files: <archived spec files that covered this domain, or none>
  """
)
```

Proceed immediately to **On: per-domain** with the first domain.

## On: per-domain

For each domain task in order, skipping tasks with status `completed`:

### 1. Mark in-progress

Call `TaskUpdate` to set the domain task status to `in_progress`.

### 2. Parallel domain survey

Dispatch all four survey subagents in a single response turn:

```
Agent(
  name = "domain-survey-code",
  description = "Survey source code for domain: <domain>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey source code for the <domain> domain.

    Module paths: <paths from task description>

    Steps:
    1. Read all source files under the listed paths.
    2. Identify caller-visible behaviors: public functions, CLI commands,
       HTTP endpoints, config options, output formats, events, or any
       interface a consumer can observe.
    3. For each behavior: note whether it appears fully implemented or
       partially implemented (stubs, TODOs, feature flags).

    Return: bullet list of caller-visible behaviors with implementation
    status (implemented / partial / none visible).
  """
)

Agent(
  name = "domain-survey-tickets",
  description = "Survey tickets relevant to domain: <domain>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Find tickets relevant to the <domain> domain.

    Steps:
    1. Glob ai-docs/tickets/**/*.md.
    2. Filter to tickets whose title or body mentions <domain> keywords
       or the module paths: <paths>.
    3. For each match: extract the feature or behavior described and
       its ticket status (todo/wip/done/dropped).

    Return: list of features → ticket status. Wip/todo items are
    candidates for 🚧 planned markers.
  """
)

Agent(
  name = "domain-survey-old-spec",
  description = "Survey archived spec for domain: <domain>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey the archived spec files for the <domain> domain.

    Archived location: ai-docs/ref/old-spec/ (most recent YYMMDD subdirectory)
    Old spec files for this domain: <files from task description, or scan all>

    Steps:
    1. Read the relevant archived spec files.
    2. For each ## heading: note the feature name and 🚧 status.
    3. Flag any behaviors in the old spec not visible in current source
       (potential regressions or planned features that were never implemented).

    Return: feature list from old spec with 🚧 status and a note on
    current-source presence.
  """
)

Agent(
  name = "domain-survey-commits",
  description = "Survey commit history for domain: <domain>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    Survey commit history for the <domain> domain.

    Module paths: <paths from task description>

    Steps:
    1. Run `git log --oneline -- <paths>`.
    2. Identify commits that added or changed caller-visible behavior
       (feat:, fix:, spec: prefixes or spec-stem references in body).
    3. For each significant commit: note the behavior changed and whether
       it appears implemented (feat/fix merged) or still in-flight.

    Return: chronological list of behavioral changes, newest first.
  """
)
```

Wait for all four subagents to return.

### 3. Synthesize behavior brief

Combine the four returns into a behavior brief:

- One bullet per distinct caller-visible behavior.
- Each bullet: behavior description, evidence source (code / ticket / old-spec / commit), and candidate classification (implemented / planned).
- Flag any item where the classification is uncertain.

### 4. User classification loop

Present the behavior brief to the user. For each item, establish:

1. **Caller-visible or internal-only?** — Internal behaviors are excluded from spec per `spec-conventions.md`. Ask on every ambiguous item.
2. **Implemented or planned?** — Implemented → plain `{#slug}`. Planned → `🚧 {#slug}`.

Ask on every ambiguous item. Do not classify without confirmation.

Collect the confirmed list before writing anything.

### 5. Write spec entries

1. Determine the target spec file path. Apply `judge: directory-vs-flat`.
2. Run `ws-print-infra spec-conventions.md` (Bash) before writing — read the output before proceeding.
3. For each confirmed behavior:
   a. Run `ws-generate-spec-stem <descriptive-slug>` to obtain `{#YYMMDD-slug}`.
   b. Write the spec entry using the `spec-format` template from `spec-conventions.md`.
   c. Place `🚧` prefix on the `##` heading if planned; omit if implemented.
4. After writing the file, verify it contains at least one `##` heading. If not, add a placeholder section and note it to the user.
5. Run `ws-spec-build-index <spec-file>`.
6. Apply `judge: directory-vs-flat` — if the written file warrants a directory split, note it as a split candidate for a follow-up `/write-spec` invocation. Do not perform the split inline.

### 6. Associate stems with tickets

1. From the step 2 survey output, collect all tickets in `wip/` or `todo/` status relevant to this domain. If none, skip to step 7.
2. Dispatch one `clerk` agent (model override: sonnet) covering all collected tickets in a single prompt:

```
Agent(
  name = "clerk-ticket-association",
  description = "Associate spec stems with wip/todo tickets for domain: <domain>",
  subagent_type = "ws:clerk",
  model = "sonnet",
  prompt = """
    Associate spec stems with tickets and check convention compliance.

    Run first:
    ```bash
    ws-print-infra ticket-conventions.md
    ```

    Spec stems generated for this domain:
    <list: {#YYMMDD-slug} — feature name, one per line>

    Tickets to update (wip/todo only):
    <list: ai-docs/tickets/<status>/<stem>.md — one-line description>

    For each ticket:
    1. Read the ticket file.
    2. Add or update the `spec:` frontmatter field with the stems relevant to
       this ticket. Merge with any existing `spec:` entries — never overwrite.
    3. Check the ticket body against loaded conventions. Fix any issues in place.
    4. Do not commit — the caller handles all git operations.
  """
)
```

3. Review the `## Clerk report`. Resolve any open questions with the user before committing.
4. Commit all domain changes in one commit: spec file + ticket association updates.

### 7. Complete domain

1. Call `TaskUpdate` to set the domain task status to `completed`.
2. If more domain tasks remain, continue with the next incomplete task from step 1 of **On: per-domain**.
3. When all domain tasks are `completed`, proceed to **On: wrap-up**.

## On: wrap-up

### 1. Final index pass

Run `ws-spec-build-index` on every spec file created during this session (idempotent safety pass):

```bash
ws-spec-build-index ai-docs/spec/<file1>.md ai-docs/spec/<file2>.md ...
```

### 2. Summary report

Emit to the user:

```
## Forge Spec — Complete

Domains covered: <N>
Spec files created: <list of paths>
Total stems generated: <count>
  Implemented: <count>
  🚧 Planned: <count>
```

### 3. Suggested next steps

- Spawn `ws:spec-updater` agent to strip `🚧` markers from any planned features whose implementation has since landed in commit history.
- Review `🚧` entries with open tickets — confirm each has an active wip/todo ticket or drop the marker.
- Run `/write-spec` for any domain surfaces discovered after wrap-up.

## Judgments

### judge: directory-vs-flat

| Decision | When |
|----------|------|
| Flat file `ai-docs/spec/<area>.md` | Single, self-contained surface — none of the split conditions below apply |
| Directory `ai-docs/spec/<area>/index.md` | Any one split condition is met: (1) a section has its own 🚧 markers with a distinct ticket lifecycle; (2) more than one `[!note] Constraints` block is present; (3) a section has a distinct audience from the parent doc |

When uncertain, start flat. Re-evaluate after writing — if a split condition fires, note the file for a follow-up `/write-spec` invocation.

## Templates

### Agent() spawn

```
Agent(
  name = "<role>",
  description = "<one-line purpose>",
  subagent_type = "general-purpose",
  model = "sonnet",
  prompt = """
    <instructions>
  """
)
```

All four survey agents per phase share a single response turn. Wait for all before synthesizing.

### ws-spec-build-index call

```bash
# Verify ## heading exists first:
grep -q '^##' <spec-file.md> || echo "WARNING: no ## heading in <spec-file.md>"
ws-spec-build-index <spec-file.md>
```

### Task registration

```
TaskCreate(
  name = "forge-spec-<domain>",
  description = """
    Spec authoring for domain: <domain>
    Source paths: <comma-separated module paths>
    Old spec files: <comma-separated archived spec paths, or none>
  """
)
```

## Doctrine

Forge-spec optimizes for **confirmed spec entries per domain** — every entry
in the produced spec reflects an explicit user decision on caller-visibility
and implementation status. When a rule is ambiguous, apply whichever
interpretation more reliably requires explicit user confirmation before any
spec content is written.
