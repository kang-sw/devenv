# AGENTS.md - [PROJECT_NAME]

## Project Memory

Read at every session start, before other action:

1. **Preamble** - repo identity, project map/topology, and canonical flows live in this file's `## Project Orientation` section below; read repo-tracked notes (`wsflow/note.search(layer: "repo")`) for volatile session context, `ai-docs/manuals/` for procedures, and generated ticket/spec inventories for current status. Keep only context a session must not re-derive.
2. **Local** - read `ai-docs/_index.local.md` if present; it is .gitignored clone context.
3. **Project arc** - run `git log --oneline --graph -50`.
4. **Recent history** - run `git log -10` for `## AI Context` rationale.

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
- Workflow shape and plugin-less maintenance guidance live in `ai-docs/WORKFLOW.md`; it is explanatory and does not override wsflow runtime or wsflow MCP parser behavior.
- Before creating or editing tickets, load the write-ticket workflow skill for conventions.
- Reference tickets by stem only, never full path; stems survive status moves.
- To check ticket completion or prior phase results, use `git log --grep=<ticket-stem>` and inspect `## Ticket Updates`.
- Claude Code compatibility is `CLAUDE.md` containing `@AGENTS.md`.
- **Language:** AI-authored docs, plans, commits, tickets, and code comments are English. Human-facing UI strings are exempt.

<!-- MIGRATION: Set up ai-docs/ for this project, then delete this block.

ai-docs/
  _index.local.md    - untracked clone-scoped memory, .gitignored
  mental-model.md    - overall mental-model index and optional project reading map
  mental-model/      - contracts, coupling, architecture narrative
  spec/              - external-perspective specs
  manuals/           - procedures and how-to content (one file per procedure, `summary:` frontmatter)
  ws-notes/          - git-tracked repo note layer (one file per key), written via wsflow/note.write(layer: "repo")
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
`wsflow/note.write(layer: "repo", ...)`, one key per topic, pruned
qualitatively as it goes stale.

Adapt structure to the project; this is a starting point, not a schema.
-->

<!-- Inclusion test: if breaking this rule makes a skill produce wrong results
     AND it applies everywhere, keep it here. Domain-scoped rules belong in
     `ai-docs/mental-model/<domain>.md ## Domain Rules` via `wsflow:lead-add-rule`.
     Context goes in this file's `## Project Orientation` section or the
     `repo` note layer; process goes in skills. -->

<!-- MIGRATION CHECKLIST
     Template-internal. NEVER copy into a project AGENTS.md; only the Template
     Version tag belongs there. Read the tag at the bottom, apply items with
     version > current in order, then update the tag.
     This template has package-local version history; apply only entries listed here.
     Preserve project-specific Architecture Rules and standards; merge
     surgically and mark conflicts instead of overwriting.

- v0001: Align `AGENTS.md` with the initial wsflow baseline: Project Memory,
  Response Discipline, Workflow, Architecture Rules, Project Knowledge, the
  inclusion-test comment, and the Template Version tag.
- v0001: Ensure `CLAUDE.md` contains `@AGENTS.md`.
- v0001: Ensure `ai-docs/WORKFLOW.md` exists from the bootstrap workflow guide.
- v0001: Ensure `ai-docs/` has `_index.md`, `mental-model.md`,
  `mental-model/`, `spec/`, `ref/`, `.old/`, and tickets status directories:
  `idea/`, `todo/`, `ready/`, `.done/`, `.dropped/`.
- v0001: Ensure `.gitignore` includes `ai-docs/**/*.local.md` and
  `ai-docs/.deps/`.
- v0002: If `ai-docs/_index.md` looks like an old all-in-one
  architecture digest, report an index health note and ask whether to clean up
  `_index.md`. Do not move semantic content into specs or mental models from
  bootstrap; the lead compacts `_index.md` only after user approval and only when an
  owning document already preserves the meaning.
- v0003: Treat stable task/topic -> document routing maps as candidates for
  `ai-docs/mental-model.md ## Project Reading Map` during later
  mental-model work. Bootstrap may report the drift, but must not move mixed
  status or feature inventory automatically.
- v0004: Replace `_index.md ## Ticket Queue` with `## Ticket Focus`. If both
  sections exist, preserve `Ticket Focus` and remove `Ticket Queue`; if only
  `Ticket Queue` exists, move the entries already listed in that section into
  `Ticket Focus` preserving order, then remove `Ticket Queue`. Update managed
  AGENTS/WORKFLOW wording to refer to `Ticket Focus`. Preserve entry text
  during migration; do not add omitted tickets, infer readiness, normalize
  wording, reorder, or promote ticket status. If any migrated entry still lacks
  clear status or readiness wording, report that a follow-up `lead-write-ticket`
  focus cleanup is needed.
- v0005: Remove the `Check '## Ticket Focus' in 'ai-docs/_index.md'` reader-instruction bullet from `## Project Knowledge` on upgrade; do not re-add it or any replacement section. Active-attention discovery is filesystem-backed (`tickets.list`/`project_tree` over the status directories) and each ticket's own body, not a cached index section.
- v0006: Retire spec planned markers. Remove every `🚧` from `ai-docs/spec/` in
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
- v0007: Dissolve `ai-docs/_index.md` as the project memory store. If
  `ai-docs/_index.md` exists: migrate its repo-identity, project-map/topology,
  and canonical-flow content into this file's `## Project Orientation` section
  (create the section first if a prior migration or manual addition has not
  already added it); migrate its `## Session Notes` (or equivalent volatile
  history) into the `repo` note layer via `wsflow/note.write(layer: "repo",
  ...)`, one key per topic, pruning entries that read as stale rather than
  copying them verbatim; drop remaining sections that are duplicate,
  derivable, or already homed elsewhere - procedure/how-to content to
  `ai-docs/manuals/` (only if not already covered by an existing manual),
  ticket/spec inventory tables (derivable via generated project-tree output),
  moment-in-time state such as branch-verification reminders, and any
  runtime/MCP/prompt/agent/skill surface description already duplicated in
  `ai-docs/manuals/` or the source tree. Then delete `ai-docs/_index.md` and
  remove the `_index.md`-reading step from `## Project Memory` (or
  equivalent). Update any project-memory pointer bullet elsewhere in
  `AGENTS.md` that still names `_index.md` to point at the new homes instead.
  This is a one-time migration judgment call, not an automated reconciliation;
  do not build staleness-detection tooling for it.
-->

<!-- Template Version: v0007 -->
