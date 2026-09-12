# Workflow Guide

This guide is copied to `ai-docs/WORKFLOW.md` by bootstrap so a
maintainer can preserve the project shape when plugin skills or MCP tools are not
available. It is an explanation and manual fallback only: editing this file does
not change MCP parser behavior, plugin/runtime semantics, ticket status logic,
or any other machine contract.

When this guide and installed plugin tooling disagree, treat the installed plugin,
runtime, and bundled conventions as canonical. Update the upstream bootstrap
template rather than relying on a project-local guide override.

## Authority Files

- `AGENTS.md` is the canonical root workflow context for AI agents and automation.
- `CLAUDE.md` exists only for Claude compatibility and should contain
  `@AGENTS.md` when the project has migrated to the host-neutral context.
- `ai-docs/WORKFLOW.md` is this pinned guide for plugin-less
  maintenance. Keep root context short; put durable project orientation in
  `AGENTS.md`'s `## Project Orientation` section, procedures in
  `ai-docs/manuals/`, volatile session context in the `repo` note layer, and
  workflow-system changes in upstream tooling.

## `ai-docs/` Layout

- `AGENTS.md`'s `## Project Orientation` section is the every-session
  orientation: repo identity, project map/topology, and canonical flows. Keep
  it compact; route deep detail to `ai-docs/manuals/`.
- `manuals/` stores procedures, how-to content, and path-scoped conventions,
  one file per topic with a `summary:` frontmatter line describing when it
  applies; a `*.local.md` sibling (gitignored) holds machine-local procedure
  content such as credentials, IPs, hostnames, or host-specific runbooks.
- `ws-notes/` is the git-tracked `repo` note layer, one file per key. It holds
  volatile or tracked session context; prune stale entries qualitatively as
  the project advances.
- `tickets/` stores work by status directory: `idea/`, `todo/`, `ready/`,
  `.done/`, and `.dropped/`.
- `ref/` stores static references and non-derivable external facts that are
  not active workflow state.
- `.old/` stores tracked project archive material kept only as possible future
  reference and hidden from default listings.
- `WORKFLOW.md` is this human-readable fallback guide.

## Tickets

- Reference tickets by stem, never by path; stems stay stable when tickets move
  between status directories.
- `idea/` is rough intake, `todo/` is accepted backlog, and `ready/` is the
  implementation-ready status for actionable tickets. Ordinary actionable
  `todo/` creation and editing are ungated; at `ready/` promotion populate
  facts, then run design and completeness review against that body.
- Epics settle design explicitly at `idea/` to `todo/`: check facts, then run
  design-only review. Material cross-child decision edits require explicit
  re-settlement before a child relies on them; ordinary edits do not auto-review.
  Epics and research stay in `idea/` or `todo/`; research remains ungated.
- Actionable tickets use `## Phases` with stable `### Phase N: <title>`
  headings. Research tickets may use freeform topic sections.
- After a phase has a `### Result` section, treat its plan text and existing
  result entries as frozen. Add later implementation tweaks as a
  `#### Edition` entry under that Result area.
- Move tickets with `git mv` when possible so history preserves status changes.

## Behavioral Contract

Tests are the behavioral contract. A behavior that matters has a test; a
change that alters behavior changes a test in the same change, and review
treats a behavior change without a test change as a finding. There is no
separate specification document to keep in step with the code, and nothing
checks whether a project's tests are strong enough to carry this role: that
is the project's own property, and the workflow assumes it rather than
enforcing it.

Prescriptive knowledge - preferred libraries, patterns, boundaries, domain
constraints - is a human decision that code cannot reconstruct, so it is
written down: one-line universal rules inline in `AGENTS.md`; longer or
path-scoped rules as one manual each under `ai-docs/manuals/`, declared in
`AGENTS.md` under `## Workflow` -> `### Implementation Conventions` (add that
section when it is absent) with the paths they cover. A rule a test can check
becomes a test; a trap tied to one site becomes a code comment at that site; a
fact about an external system goes in `ai-docs/ref/`. Descriptive knowledge -
what the code does and why - is reconstructed from the code, the tests, and
commit `## AI Context` bodies when needed, and is not maintained as a
document. Manuals carry no per-commit update obligation; drift is fixed on
contact and by review.

## Execution Model

The ticket is the plan. At actionable `ready/` promotion its stated facts are
checked against the code and written into it, then its plan passes independent
design and completeness review against that populated body. Execution
consumes those decisions instead of re-making them.

One worker executes one whole ticket: it routes, edits, verifies, runs
independent review, commits, records the phase result, and closes the
ticket. It reads the ticket, `AGENTS.md`, the declared conventions and cited
manuals, the tests, the code, and git history; it receives no summary of any
of them. The lead converses with the user, manages the ticket inventory,
spawns workers, and handles what they escalate; it edits no source.

The worker stops only for: a merge into a parent branch (user approval; the
veto point for everything the worker decided alone); an unresolved decision
the ticket does not settle; a ticket decision contradicted by code reality;
an irreversible action in the always-ask category of the `### Approval
Protocol` the project's `AGENTS.md` declares; a Critical (blocking) review
finding still open after the single fix round. Every other decision is
recorded in the commit's `## AI Context` and the ticket's
`### Result` and listed in the worker's terminal report for veto. The lead
resolves a contradicted decision itself when it can (design review over the
worker's proposed resolution), and hands a surviving Critical finding to a
stronger worker; the user sees low-reversibility decisions and exhausted lead
attempts.

Without the workflow tooling, one maintainer plays both roles and the same
model holds: before promoting a ticket to `ready/`, check its stated facts
against the code and have its plan reviewed by someone who did not write it;
then read the ticket and the declared manuals, work on a branch, keep the stop
list, and record decisions in the commit body and the ticket result.

## Index Health

`ai-docs/_index.md` is a legacy all-in-one memory file. Current projects route
every-session orientation into `AGENTS.md`'s `## Project Orientation` section,
procedures into `ai-docs/manuals/`, volatile or tracked session context into
the `repo` note layer, and ticket inventory into generated project-tree
output; they do not have an `_index.md`. This section only applies to a
project that still has one, until it runs the bootstrap migration item that
dissolves it.

When `ai-docs/_index.md` exists, bootstrap reports scope-drift candidates as
an advisory health note and asks whether to clean up now, defer cleanup, or
migrate to the current model. The first pass reads `_index.md` only and does
not move semantic content.

Common drift candidates:

- deep source trees, file-by-file roles, type listings, or implementation inventory;
- long behavior inventories that belong in the test suite or a linked
  "what works" doc;
- data-flow narratives, lifecycle descriptions, extension recipes, common
  mistakes, audit rules, or logging rules that belong in `ai-docs/manuals/` or
  in a code comment at the site they bite;
- dependency API notes, archived design excerpts, or external-reference summaries;
- done/dropped ticket history, completed milestones, or stale session chronology;
- stable task/topic reading maps mixed into `_index.md`;
- long duplicated module or ticket indexes.

When a maintainer approves cleanup, prefer the dissolution migration (move
`_index.md` content into `AGENTS.md`'s `## Project Orientation`, the `repo`
note layer, and `ai-docs/manuals/`, then delete `_index.md`) over a partial
in-place compaction. If a maintainer only wants a lighter compaction pass
instead of full dissolution:

1. Preserve the memory-policy comment.
2. Keep project summary, stack, top-level workspace, build/test commands,
   read-before-edit pointers, active inventory, and compact
   session notes.
3. Compact deep sections into links only when a clear owning document already
   exists.
4. Keep unique project direction, active priorities, and unresolved operational
   caveats in `_index.md`.
5. Do not author or semantically update tickets, manuals, or refs during index
   cleanup.
6. Compact source-derived detail to source pointers, static material to
   `ai-docs/ref/` or API-doc pointers, work history to Git or ticket archives,
   and duplicated maps to start-here pointers.
7. Route deeper semantic work through the owning workflow: procedures into
   `ai-docs/manuals/`, ticket readiness/status wording into the ticket body,
   and ambiguous direction to a discussion pass.

## Commit Traceability

- Every AI-authored commit should include `## AI Context` explaining why the
  approach was chosen and what alternatives or constraints mattered.
- Ticket-driven commits may include `## Ticket Updates` with forward-facing
  findings for future phases.

## Manual Fallback

When workflow skills, MCP tools, or Claude compatibility commands are unavailable:

1. Read `AGENTS.md`, this guide, and the relevant current docs.
2. Use existing nearby tickets and manuals as formatting examples.
3. Prefer conservative, append-only changes when parser behavior is uncertain.
4. Keep generated AI docs and commit messages in English unless a human-facing
   product string requires another language.
5. Verify with plain Git and shell commands, then re-run plugin verification
   tools when they become available.
