---
summary: Prose drafts for the ws refoundation epic, written against the re-baselined skill-authoring manual before any child ticket runs; each child moves its draft into place
---

# Refoundation Prose Drafts

These files are the judgment-carrying texts the refoundation epic
(`260909-epic-ws-worker-interpreter-refoundation`) ships. They are written
first, by the lead-tier model, against `ai-docs/manuals/skill-authoring.md`,
so that the child tickets' workers do mechanical placement (frontmatter,
callers, Go, tests, manifests, wsflow mirror) rather than authoring.

Every draft that lands on a shipped surface is written under AGENTS.md
Architecture Rule 4: it names nothing a downstream project does not hold.
Template variables (`{{.McpNamespace}}`, `{{.SkillNamespace}}`,
`{{.ExploreAgent}}`, `{{.SpawnIdiom}}`) are the render-time substitution
forms; a draft placed as an inline `SKILL.md` gets the literal namespace
through the mirror generator instead.

| Draft | Lands as | Owning child |
|-------|----------|--------------|
| `lead-discuss.md` | `discuss` skill body | lead-surface-collapse, Phase 2 |
| `lead-ticket.md` | `ticket` skill body (name binding is that ticket's open question; the body is name-independent) | lead-surface-collapse, Phase 2 |
| `lead-review.md` | `review` skill body | lead-surface-collapse, Phase 2 |
| `lead-ship.md` | `ship` skill body | lead-surface-collapse, Phase 2 |
| `ticket-fact-populator.md` | `agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md` | route-resolve-implement-reads-ticket-facts, Phase 2 |
| `bootstrap-template.md` | `AGENTS.template.md` sections and the new migration item, both packages | bootstrap-refoundation-template-migration, Phase 1 |
| `workflow-guide-sections.md` | two new `WORKFLOW.md` sections, both packages and this repository's copy | bootstrap-refoundation-template-migration, Phase 1 |

Placement rules for the worker moving a draft:

- When a draft lands, delete it from this directory and drop its row from the
  table above; the placed file is the single home and git history holds the
  mapping.
- Move the text; do not rewrite it. A change the mechanical work forces (a
  tool name, a variable the renderer does not supply, a heading a parser
  needs) is made in place and named in the commit's `## AI Context`.
- Run the fresh-reader audit from the manual once per placed file, before it
  lands. The drafts have not had one.
- Where a draft leaves a bracketed `[design-review: ...]` marker, the child's
  design review settles it; the marker must not ship.
