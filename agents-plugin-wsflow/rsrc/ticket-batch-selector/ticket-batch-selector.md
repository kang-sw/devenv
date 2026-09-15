---
kind: render
delegates: false
role: delegate
tier: medium
variables:
  - RoleModel
---
# Ticket Batch Selector

Select the set of `ready/` tickets a lead-run parallel batch can execute at
once. Return the batch, the candidates you excluded with the reason, or one
terminal result.

1. Call `{{.McpNamespace}}/git.status(format: "json")`. When `impl_ticket` is
   present, stop: batch mode presumes a clean goal branch, and the in-progress
   serial ticket finishes first.
2. List `ready/` with `{{.McpNamespace}}/tickets.query(statuses: ["ready"],
   format: "json")` and point-resolve each candidate with
   `{{.McpNamespace}}/tickets.query(ticket_stem: "<stem>", format: "json")`.
   Exclude a candidate carrying a `## Blocked (...)` note or a
   `dispatch_blocked` field.
3. Read each remaining candidate's frontmatter and body and keep only mutually
   independent tickets. Two tickets are parallel-safe when neither
   functionally depends on the other's output. A `blocked-by:` edge between
   them, a `related:` or `parent:` hint naming the other as a prerequisite, or
   body prose that needs the other's feature is a dependency: exclude the
   dependent one with the reason; it runs after its prerequisite lands. A
   shared `parent:` epic or a bare `related:` edge is not by itself a
   dependency, and file-scope overlap is not an exclusion; a real conflict
   surfaces at the lead's serialized merge.
4. With no candidate left, return `ready/ empty` when the queue was empty and
   `every remaining ticket blocked` when every candidate was excluded as
   blocked.

## Output

```
batch: <ticket path, one per line> | ready/ empty | every remaining ticket blocked | stop: <reason>
excluded: <ticket path: reason, one per line> | none
omitted: <what you did not evaluate and why> | none
```
