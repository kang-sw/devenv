---
domain: documentation-system
description: "Project memory, conventions, specs, tickets, mental models, and wsdoc discovery contracts."
sources:
  - ai-docs/
  - agents-plugin-tool/internal/wsdoc/
related:
  workflow-skills: "planning skills enforce spec and ticket conventions before implementation."
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
- Epic tickets are lightweight milestone boards for scope, child-ticket decomposition, cross-child decisions, and completion criteria; child tickets carry implementation detail and phases. {#260508-lightweight-epic-ticket-conventions}
- Mental-model hierarchy is path-derived; subdomain callers must load parent `index.md` before child docs. {#260505-mental-model-document-system}
- Infra and convention docs are embedded in the Go runtime; retired legacy copies do not affect `ws/infra.read` or `ws/convention.read`.
- `ai-docs/WORKFLOW.md` is bootstrap-installed explanatory documentation for plugin-less maintenance; wsdoc parsers and MCP tools do not treat it as convention, spec, ticket, or runtime input. {#260506-bootstrap-workflow-guide}

## Coupling

- `references.trace` composes ticket, spec, and mental-model discovery. Changes to any parser change trace completeness. {#260505-documentation-reference-tracing}
- Ticket/spec linking has two directions: ticket `spec:` frontmatter and spec body/frontmatter ticket refs. Both matter to trace output.
- Mental-model/spec linking is one-way from mental-model text to spec stems; when a spec stem is renamed, mental-model references must change in the same commit.
- `ProjectTree` still has compatibility behavior around old `features:` frontmatter; active spec truth is body anchors and markers.

## Extension Points & Change Recipes

- **Add a convention**: add Markdown under `internal/wsdoc/conventions/`, then update any compatibility copy that remains authoritative for Claude fallbacks.
- **Add a ticket status**: update status normalization, rank, scan defaults, project tree rendering, conventions, Git move detection, MCP schemas, prompts, and skills.
- **Add doc discovery tools**: update MCP dispatch, tool schema, parameter validation, tests, and docs. {#260505-documentation-authoring-workflows}

## Common Mistakes

- Adding spec anchors manually without checking for duplicates.
- Promoting non-epic, non-research work into `ready/` without matching spec entry/stem linkage.
- Using full YAML features in frontmatter; the parser is deliberately minimal.
- Changing workflow semantics in the downstream workflow guide instead of the canonical plugin/runtime, bundled conventions, or bootstrap templates.
- Loading mental-model child docs without ancestors and missing inherited Domain Rules.

## Technical Debt

- `markerContext` treats `planned` and legacy `wip` prose broadly, which can surface false-positive marker metadata.
- Project-tree spec feature stats are compatibility output, not the active spec index.
