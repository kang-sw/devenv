---
title: Documentation System
summary: Project memory, conventions, specs, tickets, mental models, reference tracing, and documentation authoring workflows for ws.
---

# Documentation System

The ws documentation system gives workflow agents a durable project memory and a
set of structured document systems for behavior, work tracking, operational
knowledge, and cross-reference discovery. It is designed for host-neutral access
through MCP discovery tools and convention documents.

## Project Memory {#260505-project-memory-index}

Project memory — the repository purpose, plugin topology, procedure references,
runtime surfaces, inventories, and session notes a future session should not
re-derive — is distributed across purpose-specific homes rather than one
hand-maintained file:

- **Every-session orientation** (repo identity, plugin topology, canonical
  flows, documentation-system routing) lives in the `AGENTS.md` body, which the
  host injects every session.
- **Session notes and volatile state** live in the note layers, injected into
  the workflow manual rather than file-read; tracked cross-clone notes use the
  `repo` layer.
- **Procedures** live in `manuals/`, ambient-injected and indexed by the
  generated `# Manuals` list.
- **Derivable inventories** (ticket and spec tables, status/focus) are generated
  (`project_tree`), not hand-copied.

`ai-docs/_index.md` — formerly the single project-memory-and-inventory document —
is retired by a versioned `lead-bootstrap` migration that moves each region to
its home and deletes the file.

**Transitional coexistence.** A project on the current runtime that has not yet
run the dissolution migration keeps a live `ai-docs/_index.md`, and that is a
supported configuration: the new ambient injections (`# Notes`, `# Manuals`) and
the generated tables coexist additively with it, and every workflow step that
reads or maintains `_index.md` does so only while the file exists, degrading
cleanly once it is gone. A migrated project carries no `_index.md`; a
fresh-bootstrapped project never creates one. Both reach the same
`AGENTS.md`-anchored shape.

## Project Old Archive {#260511-project-old-archive}

`ai-docs/.old/` is the tracked project archive for files that are no longer
active workflow state but are kept as possible future reference. It is a Git
tracked archive, not a local cache, and its dot-prefixed path keeps archived
material out of default file listings and search results.

Old reconstructed spec inputs live under `ai-docs/.old/spec/`. Bootstrap
migrations move legacy archive paths such as `ai-docs/ref/old-spec`,
`ai-docs/old-spec`, and project-level `ai-docs/old` material into
`ai-docs/.old/` when upgrading downstream projects.

## Documentation Convention Access {#260505-documentation-convention-access}

The runtime exposes canonical convention documents through
`ws/convention.read`. Callers request bundled conventions by bare name or
filename, such as `ticket-conventions`, `spec-conventions`, or
`mental-model-conventions`.

Workflow skills read these conventions before editing the matching document
system. Shared skill text uses this MCP surface instead of hard-coded
repository-local convention paths.

## Ticket-System Concept Grounding {#260723-ticket-system-concept-grounding}

Ticket-system concepts — what each status directory means, the (non-behavioral)
distinction between type prefixes, the sage-review rationale and posture
semantics, the purpose of spec addressing, the phase model, and the
epic-vs-workset distinction — are explained once in a "Ticket System Concepts"
section of the `lead-workflow-manual` bundle, surfaced to a lead session through
`ws/workflow_manual` at bootstrap (the session-once grounding call). Because the
manual is loaded once per session, convention and playbook documents carry only
the mechanical rules, hard invariants, and a pointer to the concept section; they
do not re-gloss concept meaning per invocation.

The grounding doc obeys a strict layering separation:

- **Concepts live in the manual.** Prose meaning is authored once in the bundle,
  dual-maintained across `agents-plugin/rsrc/` and `agents-plugin-wsflow/rsrc/`.
- **Guardrails live in enforcement.** Any invariant a tool can mechanically check
  stays in `ticket.verify` and the mutation tools; the concept doc must not soften
  a hard guardrail into descriptive prose. An invariant enforced only by prose
  (e.g. phase numbers never renumbered, Result/Edition text frozen once written,
  worksets never change `parent:`) stays verbatim in the convention doc, not moved
  into the concept section.
- **Mechanical content stays in Go.** The template/checklist/sage constants remain
  the mechanical source of truth; the concept doc explains meaning, not structure.
- **Type prefixes are categorization guidance only.** `feat`/`bug`/`refactor`/
  `chore` are mechanically identical (all actionable-and-phased); the concept doc
  gives plain-word "which prefix fits" guidance and states the identity explicitly
  so no behavioral divergence is inferred.

## Reference Document Ownership {#260524-reference-document-ownership}

Reference documents under `ai-docs/ref/` are operational runbooks, stable
external notes, or link hubs. They do not own caller-visible behavior contracts,
current implementation inventory, or source-derived schemas. Specs own
observable behavior, mental models own non-obvious modification coupling, and
runtime/source discovery owns facts that can be read directly from code-backed
surfaces.

A reference may point to runtime-discoverable inventory such as MCP
`tools/list`, `runtime capabilities`, source registries, or generated metadata,
but it should not duplicate that inventory unless the duplicate is generated or
is itself a public artifact. When a reference contains operational commands, the
commands are examples or runbook steps rather than the behavioral source of
truth.

## Spec Document System {#260505-spec-document-system}

Specs describe caller-visible project behavior. They live under
`ai-docs/spec/`, use English content, carry stable `{#YYMMDD-slug}` anchors, and
avoid implementation detail that would drift under behavior-preserving refactors.

Every spec entry describes implemented behavior and must be verified before
committing. Planned-but-unbuilt behavior stays in the owning ticket's
`## Spec Impact`; a known-but-unscheduled gap with no ticket uses the
`> [!note] Implementation Gap · YYYY-MM-DD` callout.

`ws/spec_stem.generate` creates collision-free anchor stems. `ws/spec_index.verify`
checks the spec corpus for duplicate anchors. `ws/specs.list`,
`ws/specs.find`, and `ws/specs.status` provide read-only spec discovery by
metadata, anchors, ticket references, query matches, and exact
stem.

## Ticket Document System {#260505-ticket-document-system}

Tickets track workflow work under `ai-docs/tickets/` with directory-based
status. Active status directories are `ready/`, `todo/`, and `idea/`: `idea/`
captures rough ideas before triage, `todo/` holds accepted backlog with
recoverable ticket intent, and `ready/` holds spec-addressed implementation
work.
Completed or dropped work moves to `.done/` or `.dropped/`. Active attention is
discovered from the status directories via `tickets.list`/`project_tree`, not a
cached index section; only `ready/` entries are direct implementation targets.

Ticket stems are stable and are referenced by stem rather than path. Actionable
tickets use phase sections with `### Result` blocks that freeze completed phase
plan text. A Result hash identifies the commit that first made the phase
reviewable on its current branch; already-merged phase updates may use the merge
commit. Later implementation passes for the same completed phase append
`#### Edition (<short-hash>) - YYYY-MM-DD` entries under that phase's Result
area; existing Result and Edition entries remain frozen once written.
Result and Edition prose records behavioral deltas, deviations, verification
evidence, unresolved findings, and deferred follow-up findings without
restating the frozen phase plan or linked spec.
{#260513-ticket-result-editions} Ticket frontmatter can connect work to specs,
removed specs, parent tickets, plans, skeletons, related mental models, and
completion metadata. Workflow routing may implement unfinished phases one slice
at a time without renaming or splitting the ticket; ticket authoring remains
responsible for decomposition.

Epic tickets are documented as lightweight milestone boards. Epic bodies keep
scope, non-scope, child-ticket boards, cross-child invariant decisions, and
done/drop/defer criteria; detailed discussion and implementation phases move
into child tickets. Epics remain decomposition artifacts exempt from ready spec
gating. {#260508-lightweight-epic-ticket-conventions}

Workset tickets are documented as non-hierarchical operating-context boards.
Workset bodies keep the context, included ticket list, current focus, and exit
criteria for a session, goal, sprint, or temporary focus area. Included tickets
are listed by stem or path with status and role; planned-but-not-created items
go under `## Planned References` with provisional labels and creation
conditions, not status or path. Workset inclusion never changes `parent:`,
and worksets do not own decomposition, cross-child invariants, implementation
phases, or spec-ready behavior. If a grouping starts owning scope decomposition
or invariant decisions, it becomes epic-shaped instead. Worksets remain
coordination artifacts exempt from ready spec gating and normally stay in
`idea/` or `todo/`, not `ready/`.
{#260524-workset-ticket-conventions}

`ws/tickets.list`, `ws/tickets.find`, and `ws/tickets.status` provide structured
ticket discovery across active and archived statuses, including phase/result
state, snippets, relationships, spec links, plans, skeletons, and status
metadata.

## Mental-Model Document System {#260505-mental-model-document-system}

Mental-model documents capture modification-relevant operational knowledge for
agents changing the workflow system. They live under `ai-docs/mental-model/`,
carry source and relationship metadata, and may be flat files or hierarchical
domain directories.

The root `ai-docs/mental-model.md` may include a compact project reading map
that routes common task or discussion topics to relevant specs, mental-model
documents, stable references, or lookup guidance. The map is routing metadata,
not project truth: it must not duplicate feature descriptions, active ticket
focus, implementation status, source paraphrases, or behavioral claims owned by
specs.

Domain documents may include `## Domain Rules`, which are persistent
user-authored prescriptions scoped to that domain. When a sub-domain document is
loaded, ancestor index documents are loaded first so inherited domain rules are
visible before work begins.

`ws/mental_models.list`, `ws/mental_models.find`, and
`ws/mental_models.status` expose domain, path, description, sources, spec
references, snippets, and hierarchy hints without requiring callers to scan the
tree manually.

## Manuals Document System {#260807-manuals-document-system}

`ai-docs/manuals/` is a flat, ambient-injection doc tier for short,
path-addressable operational manuals. Each file's frontmatter carries exactly
one field, `summary:` — a one-line description of the manual's purpose.
Unlike mental-model documents, manuals carry no domain/sources/spec-refs
metadata and no applicability predicate: every manual under
`ai-docs/manuals/*.md` is announced unconditionally in `workflow_manual`
output (see Manuals Ambient Injection in the mcp-tools spec), rather than
being selected by relevance.

The ambient `# Manuals` block is always rendered for lead sessions as an
authoring anchor (header + a fixed authoring-guidance paragraph + the list, or
a `- (none yet)` placeholder when the tier is empty), not presence-gated like
`# Notes`; the block's behavioral contract lives in Manuals Ambient Injection
in the mcp-tools spec. The guidance also teaches the local/tracked split:
machine-local details (credentials, IPs, hostnames) go to a gitignored
`*.local.md` sibling rather than into a tracked manual.

A **tracked** manual with no `summary:` frontmatter line is still discovered
and announced — reported with an explicit no-summary marker, not silently
dropped — so an author notices and fills in the missing line rather than the
manual quietly vanishing from the ambient block. A
`*.local.md` manual is exempt: it is listed as a bare path line with no summary
and no no-summary marker, because the suffix already marks it machine-local and
a gitignored file must not be nagged to add frontmatter.

The always-on ambient `# Manuals` block in `workflow_manual` output is the
manuals discovery surface: unlike `specs.*`/`mental_models.*`, manuals have no
dedicated discovery MCP tools — every manual under `ai-docs/manuals/*.md` is
already surfaced unconditionally without a separate lookup call.

The manuals-vs-`ref` boundary is a per-file editorial decision made at
content-migration time, not a schema field: content that benefits from
ambient, always-surfaced discovery (short, frequently needed procedures)
belongs under `ai-docs/manuals/`; longer or rarely needed reference material
stays under `ai-docs/ref/`, reachable only by explicit lookup. The initial
migration of applicable `ai-docs/ref/`/`ai-docs/_index.md` procedure content
into this tier has landed; further content moves between the two tiers
follow the same per-file editorial decision as new docs are authored.

## Mental-Model Update Context Annotation {#260518-mental-model-update-context-annotation}

Implementers annotate commits with a `### Mental Model Notes` sub-section
under `## AI Context` when the implementation creates a non-obvious invariant,
ordering constraint, lifecycle assumption, or cross-module contract not directly
visible from the code. The annotation is optional; absence means no implicit
contracts were introduced, not a violation.

```markdown
## AI Context
- <decision rationale>

### Mental Model Notes
- <implicit contract or invariant not visible in code>
```

This is a workflow-internal convention: defined in the implementation playbook
(`ws/infra.read("impl-playbook")`), consumed by `mental-model-updater`. It does
not appear in AGENTS.md or project-wide commit conventions.

`mental-model-updater` reads `### Mental Model Notes` entries from commit bodies
as primary intent context before processing code diffs. Notes are extracted via
`ws/git.log` with `include_body: true`; diffs serve as secondary verification.
When no notes are present, the updater falls back to diff-only analysis.

> [!note] Constraints
> - `### Mental Model Notes` does not replace `## AI Context`; it is a
>   sub-section of it.
> - Updater fallback to diff-only analysis is required when notes are absent.

> [!note] Implementation Gap · 2026-05-19
> Missing behavior: `mental-model-updater` does not explicitly read
> generated implementation plan files from `ai-docs/.plans/`. This is accepted
> as best-effort for now because implementation plans already appear in the
> commit-range diff when workflow timing includes them; explicit plan parsing
> can be revisited if notes plus diff evidence miss important intent.

## Documentation Reference Tracing {#260505-documentation-reference-tracing}

`ws/references.trace` returns the documentation graph reachable from exactly one
ticket stem or spec stem. It connects tickets, specs, and mental-model documents
through frontmatter links, spec anchors, spec removal references, and
mental-model spec references.

The trace output helps workflow agents decide which documents to open or update
without loading the full documentation corpus into context.

## Documentation Authoring Workflows {#260505-documentation-authoring-workflows}

`lead-write-spec` creates or updates spec entries for caller-visible behavior.
It reads spec conventions, chooses a file layout, generates stems, writes
implemented entries, verifies duplicate anchors, performs accuracy checks, and
commits the spec update.

`lead-update-spec` audits commit ranges for caller-visible behavior changes. It
adds missing implemented entries, handles removed spec stems, verifies
duplicate anchors, and commits all spec changes together.

`lead-backfill-docs` is the retroactive entry point for both document kinds. It
resolves an audit window from commit markers, groups the window into coherent
behavior changes, then per group runs `lead-update-spec` inline and dispatches
`mental-model-updater`. It exists because the in-flow documentation pass is
reachable only by having gone through implementation, which ad-hoc work has not.

`lead-write-ticket` creates or updates tickets. It applies the spec-address gate
when a non-`epic`, non-`research`, non-`workset` ticket enters `ready/`, reads
ticket conventions, verifies existing stems or ticket-local `## Spec Impact`,
preserves stable ticket stems, and commits ticket changes. It never invokes
`lead-write-spec`; spec addressing runs through `spec:`, `spec-remove:`, or
`## Spec Impact`. Creating or promoting accepted backlog into `todo/` preserves
intent without requiring immediate spec linkage; optional `todo/` `spec:` links
are recovery hints and promotion candidates.

## Documentation Reconstruction Workflows {#260505-documentation-reconstruction-workflows}

`lead-forge-spec` rebuilds specs from current evidence. It archives stale specs
under `ai-docs/.old/spec/` after explicit user confirmation, surveys source,
tickets, archived specs, and commits, asks the user to confirm domains and
behavior classifications, writes anchor-keyed entries, verifies duplicate
anchors, and associates spec stems with active tickets when required.

`lead-forge-mental-model` rebuilds mental-model documents from current evidence.
It surveys operational domains, asks the user to confirm the domain set, writes
modification-focused domain files, verifies them, and commits the reconstructed
mental-model corpus.
