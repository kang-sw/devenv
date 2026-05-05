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
- `ws/spec_index.verify` currently verifies duplicate anchors; it does not rebuild indexes or validate stale behavior.
- Ticket status is directory state. `.done` and `.dropped` are invisible unless discovery calls opt in. {#260505-ticket-document-system}
- Mental-model hierarchy is path-derived; subdomain callers must load parent `index.md` before child docs. {#260505-mental-model-document-system}
- Convention docs are embedded in the Go runtime; editing only `claude-plugin/infra/*` leaves `ws/convention.read` stale.

## Coupling

- `references.trace` composes ticket, spec, and mental-model discovery. Changes to any parser change trace completeness. {#260505-documentation-reference-tracing}
- Ticket/spec linking has two directions: ticket `spec:` frontmatter and spec body/frontmatter ticket refs. Both matter to trace output.
- Mental-model/spec linking is one-way from mental-model text to spec stems; when a spec stem is renamed, mental-model references must change in the same commit.
- `ProjectTree` still has compatibility behavior around old `features:` frontmatter; active spec truth is body anchors and markers.

## Extension Points & Change Recipes

- **Add a convention**: add Markdown under `internal/wsdoc/conventions/`, then update any compatibility copy that remains authoritative for Claude fallbacks.
- **Add a ticket status**: update status normalization, rank, scan defaults, project tree rendering, conventions, and skills.
- **Add doc discovery tools**: update MCP dispatch, tool schema, parameter validation, tests, and docs. {#260505-documentation-authoring-workflows}

## Common Mistakes

- Adding spec anchors manually without checking for duplicates.
- Promoting non-epic, non-research todo work without matching spec entry/stem linkage.
- Using full YAML features in frontmatter; the parser is deliberately minimal.
- Loading mental-model child docs without ancestors and missing inherited Domain Rules.

## Technical Debt

- `markerContext` treats `planned` and `wip` prose broadly, which can surface false-positive marker metadata.
- Project-tree spec feature stats are compatibility output, not the active spec index.
