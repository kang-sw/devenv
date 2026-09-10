# AGENTS.md - [PROJECT_NAME]

## Project Memory

Read at every session start, before other action:

1. **Preamble** - repo identity, project map/topology, and canonical flows live in this file's `## Project Orientation` section below; read the `repo` note layer at `ai-docs/ws-notes/` (one file per key) for volatile session context, `ai-docs/manuals/` for procedures, and the ticket status directories for current work. Keep only context a session must not re-derive.
2. **Project arc** - run `git log --oneline --graph -50`.
3. **Binding anchor** - when the task touches a topic declared under `## Workflow` -> `### Binding Anchor`, read the declared anchor before answering or editing.

## Response Discipline

- **Evidence before claims.** Run verification and read output before stating success.
- **No performative agreement.** Restate the requirement, verify, then act or push back.
- **Actions over words.** Prefer "Fixed. [what changed]" or the diff. Skip filler.

## Code Standards

<!-- Project-wide code quality rules. -->

1. **Simplicity.** Write the simplest complete implementation that satisfies the spec.
2. **Surgical changes.** Change only what the task requires; follow existing style.
3. **Responsibility check.** Keep module roles clean; split when responsibility drifts.
4. **Testability.** Prefer explicit dependencies, minimal hidden state, pure logic over side effects.
5. **[Project-specific rule].** [Description.]

## Workflow

### Approval Protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, boilerplate, single-module refactors.
- **Ask first:** new components/protocols, architecture changes, cross-module interfaces, observable behavior changes.
- **Always ask:** deleting functionality, changing protocol/API semantics, modifying persistence schema.

### Implementation Conventions

<!-- Optional. Rules that are longer than one line or apply to some paths
     only live in `ai-docs/manuals/` and are declared here. A worker reads
     the manual of every row whose `paths` match a file the ticket names or
     the change touches, before editing those paths; review treats a change
     that contradicts a cited manual as a finding. Delete this section when
     the project has no such rules; an absent section means nothing is read. -->

| paths | manual |
|-------|--------|
| `[glob, comma-separated]` | `ai-docs/manuals/[name].md` |

### Binding Anchor

<!-- Optional. Names one ticket a lead must read before answering or editing
     when the task touches one of the topics. Both keys are required; a
     section with one key, or no section, declares no anchor. -->

```text
anchor: ai-docs/tickets/<status>/<stem>.md
topics: [comma-separated topics]
```

### Commit Rules

Auto-create one commit per logical unit. Include `## AI Context` explaining why the approach was chosen.

```text
<type>(<scope>): <summary>

<what changed - brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional - ticket-driven only
- <ticket-stem>[: <optional-label>]
  > Forward: <future-phase finding>
```

### Context Window Discipline

- Source code is ground truth; load only docs relevant to the task.
- Update drifted docs on contact.

## Architecture Rules

<!-- Project-wide invariants the AI must never violate. -->

1. **[Rule name].** [Rule description.]
2. **[Rule name].** [Rule description.]

<!-- Optional for GUI/TUI projects:
1. **Headless-testable architecture.** Domain logic and state live in framework-agnostic layers testable without a display. UI layers stay thin: no branching logic, state ownership, or domain knowledge.
-->

## Project Orientation

<!-- Every-session orientation an AI session needs without re-deriving it each
     time: repo identity, project map/topology, and canonical flows. Keep
     compact; route procedures and path-scoped rules to `ai-docs/manuals/`. -->

- **Repo identity.** [Project-specific summary: what this repo is, its scope boundaries.]
- **Project map / topology.** [Project-specific: key directories/packages and their roles.]
- **Canonical flows.** [Project-specific: named workflows, entry points, or pipelines, if any.]

## Project Knowledge

- Project state and cross-session context live in `ai-docs/`.
- Workflow shape and plugin-less maintenance guidance live in `ai-docs/WORKFLOW.md`; read it only if the `ws` or `wsflow` `workflow-manual` MCP tool is not in your toolbox. It is explanatory and does not override plugin runtime or MCP parser behavior.
- Before creating or editing tickets, follow the ticket conventions and the shape of existing tickets under `ai-docs/tickets/`.
- Reference tickets by stem only, never full path; stems survive status moves.
- To check ticket completion or prior phase results, use `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- Claude Code compatibility is `CLAUDE.md` containing `@AGENTS.md`.
- **Language:** AI-authored docs, plans, commits, tickets, and code comments are English. Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  manuals/           - procedures, how-to content, and path-scoped conventions (one file per topic, `summary:` frontmatter)
  ws-notes/          - git-tracked repo note layer (one file per key), written via wsflow/note.write(layer: "repo")
  .old/              - tracked project archive hidden from default listings
  ref/               - static reference material and non-derivable external facts
  WORKFLOW.md        - plugin-less maintenance guide
  tickets/<status>/  - idea/ todo/ ready/ .done/ .dropped/

CLAUDE.md compatibility shim:

  @AGENTS.md

Populate this template's `## Project Orientation` section directly with repo
identity, project map/topology, and canonical flows; do not create a separate
`_index.md` orientation document. Route procedures and path-scoped rules to
`ai-docs/manuals/`. Ticket inventory is the status directories; do not
hand-maintain a table for it. Volatile or tracked session context (open
threads, session notes) goes to the `repo` note layer via
`wsflow/note.write(layer: "repo", ...)`, one key per topic, pruned
qualitatively as it goes stale.

Adapt structure to the project; this is a starting point, not a schema.
-->

<!-- Inclusion test: keep a rule in this file only if it applies to every
     path and fits in one line. A rule that is longer, or applies to some
     paths only, goes in `ai-docs/manuals/<name>.md` and is declared under
     `## Workflow` -> `### Implementation Conventions` with the paths it
     covers. A rule a test can check becomes a test. A trap tied to one site
     becomes a code comment at that site. A fact about an external system
     goes in `ai-docs/ref/`. Context goes in this file's
     `## Project Orientation` section or the `repo` note layer; process goes
     in skills. -->

<!-- MIGRATION CHECKLIST
     Template-internal. NEVER copy into a project AGENTS.md; only the Template
     Version tag belongs there. Read the tag at the bottom, apply items with
     version > current in order, then update the tag. Skip obsoleted items.
     Preserve project-specific Architecture Rules and standards; merge
     surgically and mark conflicts instead of overwriting.

- v0001: If `ai-docs/_memory.md` exists, merge useful content into `_index.md`, then delete it.
- v0002: [obsoleted]
- v0003: If tickets lack `plans:` frontmatter, add entries only for phases with existing plan docs. Keep discussion decisions in tickets; codebase-derived detail belongs in plans.
- v0004: Remove `plans:` entries with `null`; absence means "not yet created".
- v0005: Add `parent:` frontmatter for epic relationships where applicable. Epic tickets use category `epic`.
- v0006: Rename old plan paths `YYMM/DD-HHMM.<name>.md` to `YYYY-MM/DD-hhmm.<name>.md` with `git mv`.
- v0007: [obsoleted]
- v0008: [obsoleted by v0014]
- v0009: If Commit Rules lack `## Ticket Updates`, add it.
- v0010: If the Inclusion test comment above is missing, add it and keep it permanently.
- v0011: If no `<!-- Template Version: ... --\>` tag exists, review v0001-v0010, resolve applicable items, then add the tag to the managed context file.
- v0012: [obsoleted by v0014]
- v0013: Add the memory-policy comment to the top of `ai-docs/_index.md`; keep it permanently. Remove done/dropped ticket references.
- v0014: Replace session-start lines with `## Project Memory`; add `ai-docs/_index.local.md` to `.gitignore`.
- v0015: Move Project Summary, Tech Stack, and Workspace from CLAUDE.md to `_index.md`; keep CLAUDE.md behavioral.
- v0016: Add the ticket completion check rule to Project Knowledge if missing.
- v0017: Convert Project Knowledge paragraphs to bullets.
- v0018: For GUI/TUI projects, add the headless-testable Architecture Rule if missing.
- v0019: Replace per-file `ai-docs/*.local.md` ignores with `ai-docs/**/*.local.md`.
- v0020: Convert ticket `related:` list format to map format across all ticket statuses.
- v0021: [obsoleted by v0048]
- v0022: [obsoleted by v0048]
- v0023: [obsoleted by v0048]
- v0024: [obsoleted by v0048]
- v0025: Delete `ai-docs/_continue.local.md` if present; the removed exit-session consumer no longer reads it.
- v0026: [obsoleted by v0048]
- v0027: [obsoleted by v0048]
- v0028: [obsoleted by v0048]
- v0029: If `ai-docs/tickets/wip/` exists, `git mv` tickets to `todo/`, remove empty `wip/`, add `## Ticket Queue` if absent, then use `wsflow:lead-discuss` to agree order.
- v0030: Rename archive dirs to dot-prefix via `git mv`: `tickets/done` -> `.done`, `tickets/dropped` -> `.dropped`, `ai-docs/plans` -> `.plans`; update references.
- v0031: If `ai-docs/deps/` exists, archive it to `ai-docs/ref/deps-old`; local API documentation cache data belongs under `ai-docs/.deps/`.
- v0032: If `AGENTS.md` is absent and `CLAUDE.md` exists, create `AGENTS.md` from current `CLAUDE.md`.
- v0033: Replace `CLAUDE.md` body with `@AGENTS.md`.
- v0034: Treat `AGENTS.md` as the canonical managed template target.
- v0035: Create `ai-docs/tickets/ready/` if absent. Move existing non-`epic`, non-`research`, non-`workset` implementation-ready tickets from `todo/` to `ready/` with `git mv` when they have spec addressing; keep `epic`, `research`, `workset`, missing-spec-address, and uncertain tickets in `todo/`; recreate/keep an empty `todo/` directory when needed; treat `ready/` as the implementation queue and `## Ticket Queue` source; promote scoped `idea/` tickets to `todo/` through `wsflow:lead-discuss`.
- v0036: If `ai-docs/WORKFLOW.md` is absent, create it from the bootstrap workflow guide source. If `AGENTS.md` lacks the workflow-guide Project Knowledge bullet, add it without expanding root context into convention detail. The guide is explanatory only and does not override ws or wsflow runtime or MCP parser behavior.
- v0037: Add `ai-docs/.deps/` to `.gitignore` if absent; API documentation cache contents are runtime-managed local data, not project memory.
- v0038: Create `ai-docs/.old/` as the tracked project archive for files kept only as possible future reference and hidden from default listings. Move legacy spec archives with `git mv`: `ai-docs/ref/old-spec` or `ai-docs/old-spec` -> `ai-docs/.old/spec`; move `ai-docs/old` -> `ai-docs/.old/misc` when present and not already project-specific.
- v0039: If `ai-docs/_index.md` looks like an old all-in-one
  architecture digest, report an index health note and ask whether to clean up
  `_index.md`. Do not move semantic content into specs or mental models from
  bootstrap; the lead compacts `_index.md` only after user approval and only when an
  owning document already preserves the meaning.
- v0040: [obsoleted by v0048]
- v0041: Replace `_index.md ## Ticket Queue` with `## Ticket Focus`. If both
  sections exist, preserve `Ticket Focus` and remove `Ticket Queue`; if only
  `Ticket Queue` exists, move the entries already listed in that section into
  `Ticket Focus` preserving order, then remove `Ticket Queue`. Update managed
  AGENTS/WORKFLOW wording to refer to `Ticket Focus`. Preserve entry text
  during migration; do not add omitted tickets, infer readiness, normalize
  wording, reorder, or promote ticket status. If any migrated entry still lacks
  clear status or readiness wording, report that a follow-up `lead-ticket`
  focus cleanup is needed.
- v0042: Replace step 4 in `## Project Memory` from `git log -10` to `git log --oneline -20` with description "recent commit stems".
- v0043: Remove step 4 (`git log --oneline -20`) from `## Project Memory`; it is a redundant subset of step 3 (`git log --oneline --graph -50`). Renumber former step 5 to step 4 when present.
- v0044: Remove the `Check '## Ticket Focus' in 'ai-docs/_index.md'` reader-instruction bullet from `## Project Knowledge` on upgrade; do not re-add it or any replacement section. Active-attention discovery is filesystem-backed (`tickets.query`/`project_tree` over the status directories) and each ticket's own body, not a cached index section.
- v0045: [obsoleted by v0048]
- v0046: Dissolve `ai-docs/_index.md` as the project memory store. If
  `ai-docs/_index.md` exists: migrate its repo-identity, project-map/topology,
  and canonical-flow content into this file's `## Project Orientation` section
  (create the section first if a prior migration or manual addition has not
  already added it); migrate its `## Session Notes` (or equivalent volatile
  history) into the `repo` note layer via `wsflow/note.write(layer: "repo", ...)`,
  one key per topic, pruning entries that read as stale rather than copying
  them verbatim; drop remaining sections that are duplicate, derivable, or
  already homed elsewhere - procedure/how-to content to `ai-docs/manuals/`
  (only if not already covered by an existing manual), ticket/spec inventory
  tables (derivable via generated project-tree output), moment-in-time state
  such as branch-verification reminders, and any runtime/MCP/prompt/agent/skill
  surface description already duplicated in `ai-docs/manuals/` or the source
  tree. Then delete `ai-docs/_index.md` and remove the `_index.md`-reading step
  from `## Project Memory` (or equivalent). Update any project-memory pointer
  bullet elsewhere in `AGENTS.md` that still names `_index.md` to point at the
  new homes instead. This is a one-time migration judgment call, not an
  automated reconciliation; do not build staleness-detection tooling for it.
- v0047: Dissolve `ai-docs/_index.local.md` as the local project memory store.
  If `ai-docs/_index.local.md` exists: split its content by judgment -
  machine-local procedure content (credentials, IPs, hostnames, host-specific
  runbooks) to a new gitignored `ai-docs/manuals/*.local.md` sibling following
  the manuals convention; volatile local context to `wsflow/note.write(session_key,
  layer: "worktree", ...)` by default, or `layer: "clone"` only when the
  content is judged clone-wide (shared across worktrees of the same clone)
  rather than worktree-specific. Then delete `ai-docs/_index.local.md`, remove
  its `## Project Memory` read step (renumbering trailing steps), and remove
  the `_index.local.md` layout-tree entry from this template's MIGRATION
  scaffold comment above. Fresh bootstrap must never create
  `ai-docs/_index.local.md`. This is a one-time migration judgment call, not an
  automated reconciliation; do not build staleness-detection tooling for it.
- v0048: Retire the spec and mental-model document layers; tests are the
  behavioral contract and the ticket is the plan. Before moving anything,
  triage the content of `ai-docs/spec/`, `ai-docs/mental-model/`, and
  `ai-docs/mental-model.md` once, by judgment, into four classes: derivable
  from code (archive only); a prescriptive convention (move to
  `ai-docs/manuals/<name>.md` and declare it under `## Workflow` ->
  `### Implementation Conventions`, or inline in this file when it is one
  universal line); a site-specific trap (move to a code comment at that
  site); a non-derivable external fact (move to `ai-docs/ref/`). Then, if
  `ai-docs/spec/` exists, `git mv` it to `ai-docs/.old/spec` (merge into an
  existing archive rather than replacing it); likewise `ai-docs/mental-model/`
  to `ai-docs/.old/mental-model` and `ai-docs/mental-model.md` to
  `ai-docs/.old/mental-model.md`; create `ai-docs/.old/` first if absent.
  Delete nothing. Remove the `## Spec` block and the `renamed-spec:` sentence
  from `### Commit Rules`; rewrite `## Project Memory` step 1 so it names the
  ticket status directories instead of a spec inventory, and add the
  binding-anchor read step; add the optional `### Implementation Conventions`
  and `### Binding Anchor` sections under `## Workflow` when absent; replace
  any `## Project Orientation`, `## Project Knowledge`, or
  `## Architecture Rules` wording that routes detail to specs or mental models
  with `ai-docs/manuals/`; remove `spec/`, `mental-model/`, and
  `mental-model.md` from this template's scaffold layout above so fresh
  projects never create them; rewrite the Inclusion test comment to the
  current wording. In `ai-docs/WORKFLOW.md`, drop the sections teaching the
  retired layers along with their layout bullets and the spec-entry bullet
  under commit traceability, and merge in the current guide's
  `## Behavioral Contract` and `## Execution Model` sections. Promotion to
  `ready/` is gated by the ticket's own design review, not by spec
  addressing; read any earlier item's spec-address qualifier that way. This
  is a one-time migration judgment call, not an automated reconciliation; do
  not build staleness-detection tooling for it.
-->

<!-- Template Version: v0048 -->
