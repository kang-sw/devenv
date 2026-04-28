# Doc System Orientation

This project maintains three documentation layers. Each has a distinct role;
understanding their relationships helps you navigate context and report
accurately.

## The Three Layers

**`ai-docs/spec/`** — External-facing contracts.
Each file documents caller-visible behavior for a domain. Features are identified
by `{#YYMMDD-slug}` anchors (spec stems). A 🚧 marker on an entry means the
feature is planned but not yet implemented. Stems are the shared key that
connects spec entries to tickets.

**`ai-docs/mental-model/`** — Current implementation understanding.
These docs describe how the codebase actually works: domain structure, extension
points, invariants, coupling. They are kept in sync with code by the
`ws:mental-model-updater` agent after merges. Read them to understand existing
behavior before implementing.

**`ai-docs/tickets/`** — Work units.
Each ticket tracks a bounded scope of work through `idea/ → todo/ → wip/ → done/`.
A ticket's `spec:` frontmatter field lists the spec stems it covers. Tickets in
`todo/` or later require at least one spec stem.

## How They Connect

```
ticket (spec: field)  →  spec stem {#YYMMDD-slug}  →  spec entry in ai-docs/spec/
                                                         (🚧 = not yet implemented)
mental-model docs  ←  updated after code lands  →  reflect current implementation
```

## Your Role

Read these docs for context. Do not modify them unless your brief explicitly
tasks you to do so — lifecycle management (stripping 🚧, updating mental models,
advancing ticket status) is the lead's responsibility.

If a doc appears stale or contradicts code you encounter, note it in your output.
Do not silently work around it.
