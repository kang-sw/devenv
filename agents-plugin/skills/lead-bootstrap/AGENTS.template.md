# AGENTS.md - [PROJECT_NAME]

## Project Memory

Read at every session start, before other action:

1. **Preamble** - repo identity, project map/topology, and canonical flows live in this file's `## Project Orientation` section below; read the `repo` note layer at `ai-docs/ws-notes/` (one file per key) for volatile session context, `ai-docs/manuals/` for procedures, and generated ticket/spec inventories for current status. Keep only context a session must not re-derive.
2. **Project arc** - run `git log --oneline --graph -50`.

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

## Spec                                    # optional - omit when none
- <spec-stem>
```

When a spec heading `{#slug}` changes, include `renamed-spec: <old-stem> -> <new-stem>`.

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
     compact; route deep detail to specs, mental models, or manuals. -->

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
  mental-model.md    - overall mental-model index and optional project reading map
  mental-model/      - contracts, coupling, architecture narrative
  spec/              - external-perspective specs
  manuals/           - procedures and how-to content (one file per procedure, `summary:` frontmatter)
  ws-notes/          - git-tracked repo note layer (one file per key), written via ws/note.write(layer: "repo")
  .old/              - tracked project archive hidden from default listings
  ref/               - static reference material
  WORKFLOW.md        - plugin-less maintenance guide
  tickets/<status>/  - idea/ todo/ ready/ .done/ .dropped/

CLAUDE.md compatibility shim:

  @AGENTS.md

Populate this template's `## Project Orientation` section directly with repo
identity, project map/topology, and canonical flows; do not create a separate
`_index.md` orientation document. Route procedures and how-to content to
`ai-docs/manuals/`. Ticket and spec inventories are source-derivable; do not
hand-maintain a table for them. Volatile or tracked session context (open
threads, session notes) goes to the `repo` note layer via
`ws/note.write(layer: "repo", ...)`, one key per topic, pruned qualitatively as
it goes stale.

Adapt structure to the project; this is a starting point, not a schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules`.
     Context goes in this file's `## Project Orientation` section or the
     `repo` note layer; process goes in skills. -->

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
- v0021: If `ai-docs/mental-model/overview.md` exists, `git mv` it to `ai-docs/mental-model.md`; then run mental-model-updater to add required frontmatter to domain docs. If no `(mental-model-updated)` checkpoint exists, pass the initial commit as base. Commit with `(mental-model-updated)`.
- v0022: If flat `ai-docs/spec/` has multi-doc areas, reorganize to `ai-docs/spec/<area>/index.md` plus children; run the lead-write-spec procedure via `ws/playbook.print(name: "lead-write-spec")` to rebuild `features:` frontmatter.
- v0023: If Commit Rules lack `## Spec`, add it after `## Ticket Updates`; add `renamed-spec: <old-stem> -> <new-stem>`.
- v0024: Replace `[!note] Constraints` in specs: permanent invariants -> body prose; known unscheduled gaps -> `[!note] Implementation Gap · <YYYY-MM-DD>`.
- v0025: Delete `ai-docs/_continue.local.md` if present; the removed exit-session consumer no longer reads it.
- v0026: If specs exist but no `{#YYMMDD-slug}` anchor exists, suggest `ws:lead-forge-spec`; do not edit specs automatically.
- v0027: If mental-model docs exist but embed no spec stem, suggest `ws:lead-forge-mental-model`; do not edit mental models automatically.
- v0028: Reclassify domain-scoped rules from `## Architecture Rules` or `_index.md` into `ai-docs/mental-model/<domain>.md ## Domain Rules` via `ws:lead-add-rule`.
- v0029: If `ai-docs/tickets/wip/` exists, `git mv` tickets to `todo/`, remove empty `wip/`, add `## Ticket Queue` if absent, then use `ws:lead-discuss` to agree order.
- v0030: Rename archive dirs to dot-prefix via `git mv`: `tickets/done` -> `.done`, `tickets/dropped` -> `.dropped`, `ai-docs/plans` -> `.plans`; update references.
- v0031: If `ai-docs/deps/` exists, archive it to `ai-docs/ref/deps-old`; local API documentation cache data belongs under `ai-docs/.deps/`.
- v0032: If `AGENTS.md` is absent and `CLAUDE.md` exists, create `AGENTS.md` from current `CLAUDE.md`.
- v0033: Replace `CLAUDE.md` body with `@AGENTS.md`.
- v0034: Treat `AGENTS.md` as the canonical managed template target.
- v0035: Create `ai-docs/tickets/ready/` if absent. Move existing non-`epic`, non-`research`, non-`workset` implementation-ready tickets from `todo/` to `ready/` with `git mv` when they have spec addressing; keep `epic`, `research`, `workset`, missing-spec-address, and uncertain tickets in `todo/`; recreate/keep an empty `todo/` directory when needed; treat `ready/` as the implementation queue and `## Ticket Queue` source; promote scoped `idea/` tickets to `todo/` through `ws:lead-discuss`.
- v0036: If `ai-docs/WORKFLOW.md` is absent, create it from the bootstrap workflow guide source. If `AGENTS.md` lacks the workflow-guide Project Knowledge bullet, add it without expanding root context into convention detail. The guide is explanatory only and does not override ws runtime or MCP parser behavior.
- v0037: Add `ai-docs/.deps/` to `.gitignore` if absent; API documentation cache contents are runtime-managed local data, not project memory.
- v0038: Create `ai-docs/.old/` as the tracked project archive for files kept only as possible future reference and hidden from default listings. Move legacy spec archives with `git mv`: `ai-docs/ref/old-spec` or `ai-docs/old-spec` -> `ai-docs/.old/spec`; move `ai-docs/old` -> `ai-docs/.old/misc` when present and not already project-specific.
- v0039: If `ai-docs/_index.md` looks like an old all-in-one
  architecture digest, report an index health note and ask whether to clean up
  `_index.md`. Do not move semantic content into specs or mental models from
  bootstrap; the lead compacts `_index.md` only after user approval and only when an
  owning document already preserves the meaning.
- v0040: Treat stable task/topic -> document routing maps as candidates for
  `ai-docs/mental-model.md ## Project Reading Map` during later
  mental-model work. Bootstrap may report the drift, but must not move mixed
  status or feature inventory automatically.
- v0041: Replace `_index.md ## Ticket Queue` with `## Ticket Focus`. If both
  sections exist, preserve `Ticket Focus` and remove `Ticket Queue`; if only
  `Ticket Queue` exists, move the entries already listed in that section into
  `Ticket Focus` preserving order, then remove `Ticket Queue`. Update managed
  AGENTS/WORKFLOW wording to refer to `Ticket Focus`. Preserve entry text
  during migration; do not add omitted tickets, infer readiness, normalize
  wording, reorder, or promote ticket status. If any migrated entry still lacks
  clear status or readiness wording, report that a follow-up `lead-write-ticket`
  focus cleanup is needed.
- v0042: Replace step 4 in `## Project Memory` from `git log -10` to `git log --oneline -20` with description "recent commit stems".
- v0043: Remove step 4 (`git log --oneline -20`) from `## Project Memory`; it is a redundant subset of step 3 (`git log --oneline --graph -50`). Renumber former step 5 to step 4 when present.
- v0044: Remove the `Check '## Ticket Focus' in 'ai-docs/_index.md'` reader-instruction bullet from `## Project Knowledge` on upgrade; do not re-add it or any replacement section. Active-attention discovery is filesystem-backed (`tickets.list`/`project_tree` over the status directories) and each ticket's own body, not a cached index section.
- v0045: Retire spec planned markers. Remove every `🚧` from `ai-docs/spec/` in
  all three forms: `🚧 <Feature Name>` headings at any level (`#` through
  `######`), `> [!<keyword>] Planned 🚧` body callouts under any alphabetic
  callout keyword (`note`, `warning`, and the like), and `- 🚧 <name>` items in
  `features:` frontmatter, with or without a trailing `[<stem>/p<N>]` reference.
  Resolve each marker before removing it: when a live `idea/`, `todo/`, or
  `ready/` ticket references that spec, move the pending text into that ticket's
  `## Spec Impact`; otherwise verify the behavior against source and keep the
  text as an ordinary implemented entry when it shipped, or as
  `> [!note] Implementation Gap · <YYYY-MM-DD>` when it did not. For a
  `features:` item that shipped, strip the `🚧 ` prefix and any `[<stem>/p<N>]`
  reference instead of deleting the line. Preserve every `{#YYMMDD-slug}` anchor
  on the retained text, and update mental-model files that cross-reference a
  changed anchor in the same commit. Planned behavior no longer belongs in a
  spec; it lives in the owning ticket's `## Spec Impact`.
- v0046: Dissolve `ai-docs/_index.md` as the project memory store. If
  `ai-docs/_index.md` exists: migrate its repo-identity, project-map/topology,
  and canonical-flow content into this file's `## Project Orientation` section
  (create the section first if a prior migration or manual addition has not
  already added it); migrate its `## Session Notes` (or equivalent volatile
  history) into the `repo` note layer via `ws/note.write(layer: "repo", ...)`,
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
  the manuals convention; volatile local context to `ws/note.write(session_key,
  layer: "worktree", ...)` by default, or `layer: "clone"` only when the
  content is judged clone-wide (shared across worktrees of the same clone)
  rather than worktree-specific. Then delete `ai-docs/_index.local.md`, remove
  its `## Project Memory` read step (renumbering trailing steps), and remove
  the `_index.local.md` layout-tree entry from this template's MIGRATION
  scaffold comment above. Fresh bootstrap must never create
  `ai-docs/_index.local.md`. This is a one-time migration judgment call, not an
  automated reconciliation; do not build staleness-detection tooling for it.
-->

<!-- Template Version: v0047 -->
