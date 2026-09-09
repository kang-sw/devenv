# Bootstrap Template Changes

Drop-in text for `AGENTS.template.md` (both packages, identical apart from the
MCP namespace token), the new migration item, and the bootstrap playbook
edits. Section by section; unchanged sections are not repeated.

## `## Project Memory`

```markdown
Read at every session start, before other action:

1. **Preamble** - repo identity, project map/topology, and canonical flows live in this file's `## Project Orientation` section below; read the `repo` note layer at `ai-docs/ws-notes/` (one file per key) for volatile session context, `ai-docs/manuals/` for procedures, and the ticket status directories for current work. Keep only context a session must not re-derive.
2. **Project arc** - run `git log --oneline --graph -50`.
3. **Binding anchor** - when the task touches a topic declared under `## Workflow` -> `### Binding Anchor`, read the declared anchor before answering or editing.
```

## `## Workflow` additions

Both sections are optional. Insert after `### Approval Protocol`.

```markdown
### Implementation Conventions

<!-- Optional. Rules that are longer than one line or apply to some paths
     only live in `ai-docs/manuals/` and are declared here. A worker reads
     the manual of every row whose `paths` match a file the ticket names or
     the change touches, before editing those paths; review treats a change
     that contradicts a cited manual as a finding. Delete this section when
     the project has no such rules; an absent section means nothing is read. -->

| paths | manual |
|-------|--------|
| `[glob, comma-separated]` | `ai-docs/manuals/[name].md` |

### Binding Anchor

<!-- Optional. Names one ticket a lead must read before answering or editing
     when the task touches one of the topics. Both keys are required; a
     section with one key, or no section, declares no anchor. -->

```text
anchor: ai-docs/tickets/<status>/<stem>.md
topics: [comma-separated topics]
```
```

## `### Commit Rules`

Remove the `## Spec` block from the template and the sentence
`When a spec heading `{#slug}` changes, include `renamed-spec: ...``. The
resulting block:

```text
<type>(<scope>): <summary>

<what changed - brief>

## AI Context
- <decision rationale, rejected alternatives, user directives, etc.>

## Ticket Updates                          # optional - ticket-driven only
- <ticket-stem>[: <optional-label>]
  > Forward: <future-phase finding>
```

## `## Project Orientation` comment

```markdown
<!-- Every-session orientation an AI session needs without re-deriving it each
     time: repo identity, project map/topology, and canonical flows. Keep
     compact; route procedures and path-scoped rules to `ai-docs/manuals/`. -->
```

## Scaffold layout block

```text
ai-docs/
  manuals/           - procedures, how-to content, and path-scoped conventions (one file per topic, `summary:` frontmatter)
  ws-notes/          - git-tracked repo note layer (one file per key), written via ws/note.write(layer: "repo")
  .old/              - tracked project archive hidden from default listings
  ref/               - static reference material and non-derivable external facts
  WORKFLOW.md        - plugin-less maintenance guide
  tickets/<status>/  - idea/ todo/ ready/ .done/ .dropped/
```

The paragraph after it loses "Ticket and spec inventories are source-derivable"
in favor of: "Ticket inventory is the status directories; do not hand-maintain
a table for it."

## Inclusion test comment

```markdown
<!-- Inclusion test: keep a rule in this file only if it applies to every
     path and fits in one line. A rule that is longer, or applies to some
     paths only, goes in `ai-docs/manuals/<name>.md` and is declared under
     `## Workflow` -> `### Implementation Conventions` with the paths it
     covers. A rule a test can check becomes a test. A trap tied to one site
     becomes a code comment at that site. A fact about an external system
     goes in `ai-docs/ref/`. Context goes in `## Project Orientation` or the
     `repo` note layer; process goes in skills. -->
```

## Migration item

Appended to the checklist as the next version; the head tag moves with it.

```markdown
- v0048: Retire the spec and mental-model layers; tests are the behavioral
  contract and the ticket is the plan. Before moving anything, triage the
  content of `ai-docs/spec/`, `ai-docs/mental-model/`, and
  `ai-docs/mental-model.md` once, by judgment, into four classes: derivable
  from code (archive only); a prescriptive convention (move to
  `ai-docs/manuals/<name>.md` and declare it under `## Workflow` ->
  `### Implementation Conventions`, or inline in this file when it is one
  universal line); a site-specific trap (move to a code comment at that
  site); a non-derivable external fact (move to `ai-docs/ref/`). Then, if
  `ai-docs/spec/` exists, `git mv` it to `ai-docs/.old/spec` (merge into an
  existing archive rather than replacing it); likewise
  `ai-docs/mental-model/` to `ai-docs/.old/mental-model` and
  `ai-docs/mental-model.md` to `ai-docs/.old/mental-model.md`; create
  `ai-docs/.old/` first if absent. Delete nothing. Remove the `## Spec`
  block and the `renamed-spec:` sentence from `### Commit Rules`; add the
  binding-anchor step to `## Project Memory` and the optional
  `### Implementation Conventions` and `### Binding Anchor` sections under
  `## Workflow` if absent; replace any `## Project Knowledge` or
  `## Project Orientation` wording that routes detail to specs or mental
  models with `ai-docs/manuals/`; remove `spec/`, `mental-model/`, and
  `mental-model.md` from this template's scaffold layout above so fresh
  projects never create them. Rewrite the Inclusion test comment to the
  current wording. This is a one-time migration judgment call, not an
  automated reconciliation; do not build staleness-detection tooling for it.
```

## `lead-bootstrap` playbook

- `## On: fresh`, last step: replace the forge-skill suggestion with:
  "Suggest declaring `### Implementation Conventions` when the project has
  path-scoped rules, and creating the first ticket through the ticket
  skill."
- `## On: index health check` route table: drop the two rows that route to
  the forge skills; behavior and modification knowledge route to
  `ai-docs/manuals/` (prescriptive) or nowhere (derivable).
- Every handler step that names `spec/`, `mental-model/`, or
  `mental-model.md` as a scaffold target is removed; the archive rule above
  is the only place those names remain.
