# CLAUDE.md — [PROJECT_NAME]

## Project Memory

Read in this order at every session start, before any other action:

1. **Preamble** — read `ai-docs/_index.md`. Project-level truth that no
   session should re-derive. Prune aggressively: if derivable from code
   or commit history, delete.
2. **Local** — read `ai-docs/_index.local.md` if it exists. .gitignored.
   Machine-bound context (paths, env vars, build config) and personal
   session notes.
3. **Project arc** — run `git log --oneline --graph -50`. Trajectory and
   topic clusters at a glance.
4. **Recent history** — run `git log -10`. Decision rationale via AI Context
   sections. Fades as history grows.

## Response Discipline

- **Evidence before claims.** Run verification commands and read output before
  stating success. Never use "should pass", "probably works", or "looks correct."
- **No performative agreement.** Never respond with "Great point!", "You're
  absolutely right!", or similar. Restate the technical requirement, verify
  against the codebase, then act (or push back with reasoning).
- **Actions over words.** "Fixed. [what changed]" or just show the diff.
  Skip gratitude expressions and filler.

## Code Standards

<!-- Principles governing code quality and style. -->

1. **Simplicity.** Write the simplest code that works. Implement fully when the spec is
   clear — judge scope by AI effort, not human-hours.
2. **Surgical changes.** Change only what the task requires. Follow existing style. Every
   changed line must trace to the request.
3. **Responsibility check.** As you implement, ask whether each change
   keeps the module's role clean. Split when responsibility drifts.
4. **Testability.** Prefer designs that are straightforward to test —
   explicit dependencies, minimal hidden state, pure logic over side effects.
5. **[Project-specific rule].** [Description.]

## Workflow

### Approval Protocol

- **Auto-proceed**: Bug fixes, pattern-following additions, test code, boilerplate,
  refactoring within a single module.
- **Ask first**: New component/protocol additions, architectural changes,
  cross-module interface changes, anything that changes observable behavior.
- **Always ask**: Deleting existing functionality, changing protocol/API semantics,
  modifying persistence schema.

### Commit Rules

Auto-create git commits, each covering one logical unit of change.
Include an **AI context** section in every commit message recording design decisions,
alternatives considered, and trade-offs — focus on _why_ this approach was chosen.

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem>[: <optional-label>]
  > Forward: <what future phases must know>

## Spec                                    # optional — one per affected spec feature; omit if none
- <spec-stem>
```

When a spec heading's `{#slug}` changes, include `renamed-spec: <old-stem> → <new-stem>` in the commit message body.

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the current task. Update drifted docs on contact.

## Architecture Rules

<!-- Project-specific invariants the AI must never violate. -->

1. **[Rule name].** [Rule description.]
2. **[Rule name].** [Rule description.]

<!-- Optional — enable for projects with a GUI/TUI presentation layer:
1. **Headless-testable architecture.** All domain logic and state must live in
   framework-agnostic layers testable without a display. UI layers are thin
   adapters: no branching logic, no state ownership, no domain knowledge.
-->

## Project Knowledge

- Project state and cross-session context live in `ai-docs/`.
- Before creating or editing tickets, load `/write-ticket` for conventions.
- Reference tickets by **stem only** (e.g., `260115-feat-foo-bar`), never by
  full path — stems stay stable across status moves.
- Check `## Ticket Queue` in `ai-docs/_index.md` for the intended implementation order before starting a ticket.
- To check ticket completion or prior phase results, use `git log --grep=<ticket-stem>`
  and look for `## Ticket Updates` sections in matching commits.
- **Language:** All AI-authored artifacts — documents, plans, commit messages, ticket entries,
  and inline code comments — must be in English regardless of conversation language.
  Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.md          — single session-start read; project context
  _index.local.md    — local memory (free-form, .gitignored)
  mental-model.md    — overview file for overall mental model documents.
  mental-model/      — project map: contracts, coupling, architectural narrative
  spec/              — external-perspective specs (area/ directories for multi-section areas)
  ref/               — static reference material (external specs, protocol docs, design notes)
  tickets/<status>/  — idea/ todo/ .done/ .dropped/

_index.md should cover:
  - Project summary (what it is, who it's for, current milestone)
  - Tech stack (languages, frameworks, key libraries)
  - Workspace (top-level directories and their roles)
  - Conventions (tickets, dependency docs, naming rules)
  - Build/test commands and operational pitfalls
  - Session notes (cross-session intent only, 2-5 lines max, delete when stale)
  - Never reference .done/ or .dropped/ tickets — they live in git history

_index.md must start with this comment (do not remove after creation):

  <!-- Memory policy: prune aggressively as project advances. Completed
       work belongs in git history, not here. Keep only what an AI session
       needs to orient itself and pick up work. If it's derivable from
       code or git log, delete it from this file. --\>

Adapt structure to fit the project — these are guidelines, not a rigid schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce
     wrong results AND the rule applies everywhere in the codebase
     regardless of which area is being worked in, it belongs here.

     Domain-scoped rules — rules that only apply when working in a
     specific domain area — do NOT belong here. Use `/add-rule` to
     classify the rule and route domain-scoped ones to
     `ai-docs/mental-model/<domain>.md ## Domain Rules`.

     Everything else (context) goes in `_index.md`; everything else
     (process) goes in skills. -->

<!-- MIGRATION CHECKLIST
     This block is template-internal tooling. NEVER copy it into a
     project's CLAUDE.md — only the Template Version tag belongs there.
     Read the Template Version tag at the bottom of this file.
     Apply all items with version > current, in order. Then update the tag.
     Items marked [obsoleted by vNNNN] — skip entirely.
     Project-specific content (Architecture Rules, custom standards) must
     survive — merge surgically, flag conflicts rather than overwriting.

- v0001: If `ai-docs/_memory.md` exists, merge useful content into
         `ai-docs/_index.md` and delete `_memory.md`.
- v0002: [obsoleted]
- v0003: If tickets lack `plans:` frontmatter, add it (only phases with
         existing plan documents — no null placeholders). Audit phase
         content: discussion decisions stay in tickets, codebase-derived
         details belong in plans.
- v0004: If tickets have `plans:` entries with `null` values, remove them.
         Absence means "not yet created".
- v0005: If tickets lack `parent:` frontmatter for epic relationships, add
         it where applicable. Epic tickets use category `epic`.
- v0006: If plan paths use old format (`YYMM/DD-HHMM.<name>.md`), rename
         to `YYYY-MM/DD-hhmm.<name>.md` via `git mv`.
- v0007: [obsoleted]
- v0008: [obsoleted by v0014]
- v0009: If Commit Rules lack `## Ticket Updates`, add it. Ticket-driven
         commits must record the stem and forward-facing findings.
- v0010: If the `<!-- Inclusion test: ... --\>` comment block is missing
         above this checklist, add it. Do not remove after migration —
         permanent authoring guardrail.
- v0011: Template version tracking starts here. If the project has no
         `<!-- Template Version: ... --\>` tag, review v0001-v0010 against
         the current project state to determine which items still apply.
         After resolving, add the tag at the bottom of CLAUDE.md.
- v0012: [obsoleted by v0014]
- v0013: Add the memory-policy comment to the top of `ai-docs/_index.md`
         (see MIGRATION block for exact text). Do not remove after adding —
         permanent pruning guardrail. Remove any references to done/ or
         dropped/ tickets from `_index.md`.
- v0014: Replace the session-start `_index.md` / `_index.local.md` lines
         with the `## Project Memory` section (see template). Remove
         Session Start section — it is now part of Project Memory.
         Add `_index.local.md` to `.gitignore` if not already present.
- v0015: Move Project Summary, Tech Stack, and Workspace sections from
         CLAUDE.md to `ai-docs/_index.md`. Remove the `---` separator
         if it was used to divide behavioral/contextual sections.
         CLAUDE.md keeps only behavioral rules (Architecture Rules,
         Project Knowledge). Context lives in `_index.md`.
- v0016: If Project Knowledge lacks a ticket completion check rule, add:
         "To check ticket completion or prior phase results, use
         `git log --grep=<ticket-stem>` and look for `## Ticket Updates`
         sections in matching commits."
- v0017: If Project Knowledge items are plain paragraphs, convert to a
         bulleted list (`- ` prefix per item) for readability.
- v0018: If the project has a GUI/TUI presentation layer and Architecture
         Rules lacks a headless-testable rule, add:
         `**Headless-testable architecture.** All domain logic and state
         must live in framework-agnostic layers testable without a display.
         UI layers are thin adapters: no branching logic, no state
         ownership, no domain knowledge.`
- v0019: Replace any explicit per-file `.local.md` gitignore entries
         under `ai-docs/` with a single glob: `ai-docs/**/*.local.md`.
         Covers existing files like `_index.local.md` and session-scratch
         files like `_continue.local.md` without per-file maintenance.
- v0020: If `related:` entries use list format (`- stem  # comment`),
         convert to map format (`stem: comment`) across all tickets in
         `ai-docs/tickets/` (all statuses). Empty lists (`related: []`)
         may be removed or left as-is — the map script handles both.
         See the `ws-proj-tree` bin script for canonical parser.
- v0021: If `ai-docs/mental-model/overview.md` exists, promote it to the
         top-level index: `git mv ai-docs/mental-model/overview.md ai-docs/mental-model.md`.
         Then dispatch the `mental-model-updater` agent to add frontmatter (`domain`,
         `description`, `sources`, `related`) to all existing domain docs that lack it.
         Caller note: if no `(mental-model-updated)` checkpoint exists in git history,
         pass the repository's initial commit as the base commit to the agent.
         Commit all changes with `(mental-model-updated)` in the message body.
- v0022: If spec documents exist in a flat layout under `ai-docs/spec/`, reorganize
         areas with multiple sub-docs into directories: `ai-docs/spec/<area>/index.md`
         plus child files. Run `/write-spec` to rebuild the `features:` frontmatter
         via `ws-spec-build-index` for all spec files after reorganization.
- v0023: If Commit Rules lack a `## Spec` section, add it after `## Ticket Updates`.
         Format: `- <spec-stem>  # one per affected spec feature; omit section if none`.
         Also add rename convention: "When a spec heading's {#slug} changes, include
         `renamed-spec: <old-stem> → <new-stem>` in the commit message."
- v0024: If spec documents under `ai-docs/spec/` contain `[!note] Constraints` callouts,
         audit each one and reclassify:
         (a) Permanent behavioral invariants → move to body prose.
         (b) Known-but-unscheduled implementation gaps → convert to
             `[!note] Implementation Gap · <YYYY-MM-DD>` (use the date of migration).
         (c) Planned features with an existing `todo/`-or-higher ticket → convert to
             `### 🚧 <Feature Name>` heading.
         Remove the `[!note] Constraints` form after reclassifying all items in a file.
- v0025: If `ai-docs/_continue.local.md` exists, delete it. This file was written by
         the now-removed `exit-session` skill and has no remaining consumer.
- v0026: If `ai-docs/spec/` contains files but none contain a `{#YYMMDD-slug}` stem anchor
         (check: `grep -r '{#[0-9]\{6\}-' ai-docs/spec/ | head -1`), the spec predates
         stem-based management. Suggest running `/forge-spec` to rebuild with canonical
         stem-anchored entries. Output-only — do not modify spec files automatically.
- v0027: If `ai-docs/mental-model/` contains domain files but none embed a spec stem
         (check: `grep -r '{#[0-9]\{6\}-' ai-docs/mental-model/ | head -1`), the mental-model
         predates spec cross-references. Suggest running `/forge-mental-model` to rebuild
         with spec-stem embedding. Output-only — do not modify mental-model files automatically.
- v0028: Re-evaluate entries in `## Architecture Rules` (CLAUDE.md) and architectural
         conventions in `_index.md`. Rules that apply only in a specific domain area
         are domain-scoped, not cross-cutting. Reclassify them into
         `ai-docs/mental-model/<domain>.md ## Domain Rules` via `/add-rule`.
         Applied on the next `/bootstrap upgrade` run.
- v0029: If `ai-docs/tickets/wip/` exists: for each ticket inside, `git mv` it to
         `ai-docs/tickets/todo/`. Remove the now-empty `wip/` directory.
         Add a `## Ticket Queue` section to `ai-docs/_index.md` if absent
         (format: one line per `todo/` ticket — `` `stem` — purpose and dependency notes ``).
         Then open a `/discuss` session with the user to agree on implementation
         order for `todo/` tickets and populate the queue.
- v0030: Rename archive ticket directories and plan directory to dot-prefix form
         (hidden from default grep/glob). Run the following `git mv` commands if
         the plain-named directories still exist:
           `git mv ai-docs/tickets/done  ai-docs/tickets/.done`
           `git mv ai-docs/tickets/dropped  ai-docs/tickets/.dropped`
           `git mv ai-docs/plans  ai-docs/.plans`
         Update any references to `tickets/done/`, `tickets/dropped/`, or `ai-docs/plans/`
         in CLAUDE.md, `ai-docs/_index.md`, and infra/skill docs to use the dot-prefix form.
- v0031: Archive legacy `ai-docs/deps/` directory if it exists. This directory
         held manually-maintained API delta docs; it is superseded by the
         `ws-ask-api` / `ai-docs/.deps/` cache system.
         If `ai-docs/deps/` exists: `git mv ai-docs/deps ai-docs/ref/deps-old`.
         Add `ai-docs/.deps/` to `.gitignore` if not already present (the cache
         is machine-local and should not be committed in downstream projects).
-->

<!-- Template Version: v0031 -->
