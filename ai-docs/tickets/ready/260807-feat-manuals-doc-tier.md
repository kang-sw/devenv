---
title: manuals doc tier — first-tier operating-procedure category with ambient one-line injection
sage-review-design: completed
sage-review-completeness: completed
parent: 260807-epic-mechanical-project-memory
related:
  260716-feat-mental-model-openup-injection: shares-substrate — reuses the tier-agnostic frontmatter parser and one-line `summary:` schema, but NOT its selective (rule-based relevance) injection logic, which stays owned there
  260728-research-index-ticket-table-drift: related — the hand-maintained _index routing/procedure content this tier replaces with a generated ambient index
  260807-feat-note-memory-layers: sibling — the other workflow_manual injection surface under the same epic; both model on the scopeAnnouncement inject pattern
---

# manuals doc tier — first-tier operating-procedure category with ambient one-line injection

## Background

Per-repo operating procedures (how to cut a release, how to run the dogfood
loop, how the bootstrap migration handles legacy docs) currently live in two bad
homes: `ai-docs/ref/` (a catch-all for documents with no better place, never
injected) and inlined into `ai-docs/_index.md` (read only if an agent chooses to
read the file — behavioral, unreliable). Neither surface tells a session *which*
procedures apply to the work it is about to do; the agent has to already know a
procedure exists to go find it.

This ticket adds a first-tier document category, `ai-docs/manuals/`, peer to
`spec/` / `mental-model/` / `ticket`, whose one-line applicability index is
generated from each manual's frontmatter and **ambient-injected** into
`workflow_manual` output. Bodies are read on demand. This is the "always-resident
operating-procedure" home the epic's dissolution redistributes `_index.md`'s
procedure content into.

## Decisions

- **Ambient injection needs no applicability predicate.** The `# Manuals` block
  injects *every* manual's path plus its one-line summary; the agent self-selects
  which body to read. A missed one-line pointer is cheap to recover, and no rule
  has to be correct — this is the cheap-miss side of the epic's cost-of-miss cut.
  Rule-based "which manual is relevant to what I'm about to do" selection is the
  expensive-miss path, explicitly owned by the `260716` cluster and out of scope.
- **The schema is one line: `summary:`.** Because injection is ambient, a manual's
  frontmatter needs only a `summary:` string for the index line. No `sources:` /
  `applies-when:` applicability signal is required (those exist to *select*, which
  this tier does not do). Keeping the schema minimal is what keeps the tier cheap.
- **Shared frontmatter substrate, not shared injection.** The tier-agnostic
  frontmatter parser and the `summary:` field are shared infrastructure (mental-model
  already carries frontmatter; note-memory and this tier both inject via the
  `scopeAnnouncement` pattern). The injection *logic* and any telemetry are NOT
  shared with `260716` — this tier's injector is a flat "list all manuals" emit.
- **manuals vs ref is an injection boundary.** `ref/` stays the home for documents
  with no injection role (static references, historical Claude material). `manuals/`
  is for operating procedures that should ambient-inject. The split is decided
  per-file at migration time (Phase 2), not by a schema field.
- **Generation follows the `scopeAnnouncement` pattern.** A `computeManuals(root)
  string` walks `ai-docs/manuals/`, reads each frontmatter `summary:`, and emits a
  `# Manuals` block; it is injected in `workflow_manual.go` on the same
  fresh-with-root and continue branches as the existing scope/notes injections.
  Whether the walk lives behind `project_tree` or a standalone compute is an
  implementation choice, not a contract change.

## Spec Impact

No existing stem defines a `manuals/` document tier or its injection.

- **`spec/documentation-system.md`**: add `ai-docs/manuals/` as a first-tier
  document category (peer to spec / mental-model / ticket), its `summary:`
  frontmatter schema, and the ref-vs-manuals boundary. Caller-visible change: a
  new authored-document category with its own convention.
- **`spec/mcp-tools.md`** `workflow_manual` section: document the injected
  `# Manuals` block (path + one-line summary per manual, on the fresh-with-root
  and continue branches). Caller-visible change: `workflow_manual` output gains a
  Manuals index every session that engages the workflow.

## Phases

### Phase 1: manuals tier + ambient `# Manuals` injection

Deliver the category and its injection:

- Recognize `ai-docs/manuals/*.md` as a first-tier document tier with a
  `summary:` frontmatter field, reusing the tier-agnostic frontmatter parser
  rather than forking a manuals-only parser.
- A `computeManuals(root) string` that walks the directory, reads each `summary:`,
  and renders a `# Manuals` block (each line: manual path + summary), modeled on
  `scopeAnnouncement`.
- Inject the block into `workflow_manual` output on the fresh-with-root and
  continue branches, alongside the existing scope announcement.
- Discovery parity: whatever `specs.*` / `mental_models.*` list/find surface
  exists, give manuals an equivalent so a body is reachable by tool, not only by
  the injected path.

Verification: a manual added under `ai-docs/manuals/` with a `summary:` appears in
the next `workflow_manual` call's `# Manuals` block; a manual with no `summary:`
is reported (not silently dropped); the discovery surface returns the new manual.

### Phase 2: bootstrap migration of ref/ and inline _index procedures into manuals/

Move existing operating-procedure content into the new tier and teach the
migration path to keep doing so:

- Relocate procedure-shaped documents from `ai-docs/ref/` into `ai-docs/manuals/`,
  applying the manuals-vs-ref boundary per file. The concrete migration source is
  the hand-maintained `_index.md` "Read-Before-Editing" table, whose rows already
  pair a `ref/` document with a one-line applicability description — exactly the
  `path` + `summary:` a manual needs. Candidates there include
  `ref/skill-authoring.md`, `ref/wsflow-mirroring.md`, `ref/codex-integration.md`,
  `ref/ws-mcp.md`, `ref/windows-dogfood.md`, and `ref/ws-agent-runtime.md`; each
  row's description becomes the moved manual's `summary:`. Static or historical
  references with no applicability description (e.g. `ref/claude-home-legacy.md`)
  stay in `ref/`.
- Update the bootstrap skill `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`
  (and its wsflow mirror, per `ref/wsflow-mirroring.md`) so its ref-handling step
  routes new procedure/manual-shaped docs into `ai-docs/manuals/` rather than
  `ai-docs/ref/`.

Verification: every `_index.md` "Read-Before-Editing" row that names a procedure
doc resolves to a file under `ai-docs/manuals/` carrying a `summary:` equal to that
row's description, and appears in the injected `# Manuals` block; no procedure doc
meeting the manuals boundary remains under `ref/`; `lead-bootstrap` (and its
mirror) names `manuals/` as the destination for new procedure docs.

This phase depends on Phase 1 (the destination tier must exist). It is the
`manuals` leg of the epic's `_index.md` dissolution; it does not itself dissolve
`_index.md` (that is the epic's separate decomposition child), only drains the
procedure content that would otherwise block it.
