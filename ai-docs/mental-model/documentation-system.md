---
domain: documentation-system
description: "Project memory, conventions, specs, tickets, mental models, and wsdoc discovery contracts."
sources:
  - ai-docs/
  - agents-plugin-tool/internal/wsdoc/
related:
  workflow-skills: "planning skills enforce spec and ticket conventions before implementation."
  mcp-runtime: "MCP and CLI adapters render wsdoc discovery results; wsdoc owns matching and metadata."
---

# Documentation System

## Entry Points

- `agents-plugin-tool/internal/wsdoc/` powers read-only MCP discovery for infra docs, specs, tickets, mental models, references, conventions, and project tree.
- `agents-plugin-tool/internal/wsdoc/conventions/*.md` is the bundled source for `ws/convention.read`. {#260505-documentation-convention-access}

## Module Contracts

- Spec identity is body-anchor based. Tools scan `{#YYMMDD-slug}` anchors under `ai-docs/spec/`, not frontmatter. {#260505-spec-document-system}
- `ai-docs/.old/` is the Git-tracked project archive for inactive reference material; dot-prefix keeps it out of default listings. {#260511-project-old-archive}
- Reconstructed old spec inputs live under `ai-docs/.old/spec/`, not `ai-docs/ref/old-spec/`. {#260511-project-old-archive}
- `ws/spec_index.verify` currently verifies duplicate anchors; it does not rebuild indexes or validate stale behavior.
- Ticket status is directory state: `ready/`, `todo/`, and `idea/` are active; `.done` and `.dropped` are invisible unless discovery calls opt in. {#260505-ticket-document-system}
- Ticket Result hashes identify the commit that made the completed phase reviewable on its current branch; merge commits are only required when the phase was already merged before the ticket update. {#260505-ticket-document-system}
- Ticket phase plan text freezes after the first Result, but later implementation tweaks append `#### Edition` entries under the Result area; existing Result and Edition text remains frozen. {#260513-ticket-result-editions}
- Ready-ticket convention keeps spec addressing mandatory through `spec:`, `spec-remove:`, or `## Spec Impact`; `lead-write-ticket` invokes `lead-write-spec` only for contract-first planned spec entries before finalizing the queue entry. {#260505-documentation-authoring-workflows}
- Epic tickets are lightweight milestone boards for scope, child-ticket decomposition, cross-child decisions, and completion criteria; child tickets carry implementation detail and complete phase units. `lead-proceed` executes one selected phase without changing ticket decomposition. {#260508-lightweight-epic-ticket-conventions} {#260505-proceed-routing-pipeline}
- Mental-model hierarchy is path-derived; subdomain callers must load parent `index.md` before child docs. {#260505-mental-model-document-system}
- The root `ai-docs/mental-model.md` may carry a compact project reading map for task/topic routing; it does not own behavior, status, queue, or source-derived claims. {#260505-mental-model-document-system}
- `### Mental Model Notes` is an optional workflow-internal commit subsection under `## AI Context`; `mental-model-updater` treats those notes as primary intent and uses diffs for verification and fallback. {#260518-mental-model-update-context-annotation}
- Infra and convention docs are embedded in the Go runtime; retired legacy copies do not affect `ws/infra.read` or `ws/convention.read`.
- Broad `specs.find` and `mental_models.find` query matching is token-scored in shared wsdoc helpers, not per-discovery substring checks; exact selectors (`spec_stem`, `ticket_stem`, `domain`) still filter before query scoring. {#260519-tolerant-documentation-lookup-query-evidence}
- Query evidence is body-line-only: metadata can raise a document score, but it must not create synthetic line evidence. This prevents text output from pointing callers at non-existent line zero hits. {#260519-tolerant-documentation-lookup-query-evidence}
- `ai-docs/WORKFLOW.md` is bootstrap-installed explanatory documentation for plugin-less maintenance; wsdoc parsers and MCP tools do not treat it as convention, spec, ticket, or runtime input. {#260506-bootstrap-workflow-guide}

## Coupling

- `references.trace` composes ticket, spec, and mental-model discovery. Changes to any parser change trace completeness. {#260505-documentation-reference-tracing}
- A project reading map points to specs, mental-model docs, references, or lookup guidance; those target documents remain the owners for behavioral and implementation facts.
- Ticket/spec linking has two directions: ticket `spec:` frontmatter and spec body/frontmatter ticket refs. Both matter to trace output.
- Mental-model/spec linking is one-way from mental-model text to spec stems; when a spec stem is renamed, mental-model references must change in the same commit.
- `ProjectTree` still has compatibility behavior around old `features:` frontmatter; active spec truth is body anchors and markers.

## Extension Points & Change Recipes

- **Add a convention**: add Markdown under `internal/wsdoc/conventions/`, then update any compatibility copy that remains authoritative for Claude fallbacks.
- **Add a ticket status**: update status normalization, rank, scan defaults, project tree rendering, conventions, Git move detection, MCP schemas, prompts, and skills.
- **Add doc discovery tools**: update MCP dispatch, tool schema, parameter validation, tests, and docs; reuse shared query matching/evidence helpers when the tool accepts broad human `query` text. {#260505-documentation-authoring-workflows}

## Common Mistakes

- Adding spec anchors manually without checking for duplicates.
- Promoting non-epic, non-research work into `ready/` without a confirmed spec stem, `spec-remove:`, or `## Spec Impact`.
- Using full YAML features in frontmatter; the parser is deliberately minimal.
- Changing workflow semantics in the downstream workflow guide instead of the canonical plugin/runtime, bundled conventions, or bootstrap templates.
- Loading mental-model child docs without ancestors and missing inherited Domain Rules.
- Moving current feature inventory or implementation status from `_index.md` into the project reading map instead of specs, tickets, source, or tests.
- Replacing tolerant broad-query scoring with exact phrase matching; multi-word user questions should find candidates by shared terms while exact selectors remain exact filters.

## Technical Debt

- `markerContext` treats `planned` and legacy `wip` prose broadly, which can surface false-positive marker metadata.
- Project-tree spec feature stats are compatibility output, not the active spec index.
