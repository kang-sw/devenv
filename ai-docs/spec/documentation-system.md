---
title: Documentation System
summary: Project memory, conventions, specs, tickets, mental models, reference tracing, and documentation authoring workflows for ws.
---

# Documentation System

The ws documentation system gives workflow agents a durable project memory and a
set of structured document systems for behavior, work tracking, operational
knowledge, and cross-reference discovery. It is designed for host-neutral access
through MCP discovery tools and convention documents.

## Project Memory Index {#260505-project-memory-index}

`ai-docs/_index.md` is the project memory and active inventory document. It
records the repository purpose, plugin topology, read-before-edit references,
implemented runtime surfaces, prompt and skill inventory, current spec list,
active ticket list, ticket queue, and compact session notes.

The index is intentionally bounded: completed and dropped ticket history lives
in the ticket archive directories and Git history, while the index keeps the
current queue and context a future session should not have to re-derive.

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

## Spec Document System {#260505-spec-document-system}

Specs describe caller-visible project behavior. They live under
`ai-docs/spec/`, use English content, carry stable `{#YYMMDD-slug}` anchors, and
avoid implementation detail that would drift under behavior-preserving refactors.

Planned spec behavior uses `🚧` markers: a planned new feature is a heading such
as `## 🚧 Feature Name {#YYMMDD-slug}`, and a planned change to an existing
feature uses a `> [!note] Planned 🚧` callout. Entries without `🚧` are treated
as implemented and must be verified before committing.

`ws/spec_stem.generate` creates collision-free anchor stems. `ws/spec_index.verify`
checks the spec corpus for duplicate anchors. `ws/specs.list`,
`ws/specs.find`, and `ws/specs.status` provide read-only spec discovery by
metadata, anchors, ticket references, marker context, query matches, and exact
stem.

## Ticket Document System {#260505-ticket-document-system}

Tickets track workflow work under `ai-docs/tickets/` with directory-based
status. Active status directories are `ready/`, `todo/`, and `idea/`: `idea/`
captures rough ideas before triage, `todo/` holds accepted backlog with
recoverable ticket intent, and `ready/` holds spec-gated implementation work.
Completed or dropped work moves to `.done/` or `.dropped/`. `## Ticket Queue`
lists `ready/` work only.

Ticket stems are stable and are referenced by stem rather than path. Actionable
tickets use phase sections with `### Result` blocks that freeze completed phase
content. Ticket frontmatter can connect work to specs, removed specs, parent
tickets, plans, skeletons, related mental models, and completion metadata.

Epic tickets are documented as lightweight milestone boards. Epic bodies keep
scope, non-scope, child-ticket boards, cross-child invariant decisions, and
done/drop/defer criteria; detailed discussion and implementation phases move
into child tickets. Epics remain decomposition artifacts exempt from ready spec
gating. {#260508-lightweight-epic-ticket-conventions}

`ws/tickets.list`, `ws/tickets.find`, and `ws/tickets.status` provide structured
ticket discovery across active and archived statuses, including phase/result
state, snippets, relationships, spec links, plans, skeletons, and status
metadata.

## Mental-Model Document System {#260505-mental-model-document-system}

Mental-model documents capture modification-relevant operational knowledge for
agents changing the workflow system. They live under `ai-docs/mental-model/`,
carry source and relationship metadata, and may be flat files or hierarchical
domain directories.

Domain documents may include `## Domain Rules`, which are persistent
user-authored prescriptions scoped to that domain. When a sub-domain document is
loaded, ancestor index documents are loaded first so inherited domain rules are
visible before work begins.

`ws/mental_models.list`, `ws/mental_models.find`, and
`ws/mental_models.status` expose domain, path, description, sources, spec
references, snippets, and hierarchy hints without requiring callers to scan the
tree manually.

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
implemented or `🚧` entries, verifies duplicate anchors, performs accuracy
checks, and commits the spec update.

`lead-update-spec` audits commit ranges for caller-visible behavior changes. It
adds missing implemented entries, strips `🚧` markers when implementation has
landed, handles removed spec stems, verifies duplicate anchors, and commits all
spec changes together.

`lead-write-ticket` creates or updates tickets. It applies the spec gate when a
non-`epic`, non-`research` ticket enters `ready/`, reads ticket conventions,
updates queue entries for `ready/` work, preserves stable ticket stems, and
commits ticket changes. Creating or promoting accepted backlog into `todo/`
preserves intent without requiring immediate spec linkage; optional `todo/`
`spec:` links are recovery hints and promotion candidates.

## Documentation Reconstruction Workflows {#260505-documentation-reconstruction-workflows}

`lead-forge-spec` rebuilds specs from current evidence. It archives stale specs
under `ai-docs/.old/spec/` after explicit user confirmation, surveys source,
tickets, archived specs, and commits, asks the user to confirm domains and
behavior classifications, writes anchor-keyed entries, verifies duplicate
anchors, and associates planned stems with active tickets when required.

`lead-forge-mental-model` rebuilds mental-model documents from current evidence.
It surveys operational domains, asks the user to confirm the domain set, writes
modification-focused domain files, verifies them, and commits the reconstructed
mental-model corpus.
