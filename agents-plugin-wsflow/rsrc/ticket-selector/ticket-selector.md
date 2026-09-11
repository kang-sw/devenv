---
kind: render
delegates: false
role: delegate
tier: small
variables:
  - RoleModel
---
# Ticket Selector

Select one executable ticket for a lead-run cycle. Return exactly one selected
ticket path, `ready/ empty`, `every remaining ticket blocked`, or a stop reason.

1. Call `{{.McpNamespace}}/git.status(format: "json")` before reading the
   ready queue.
2. When its `impl_ticket` field is present, never consider base-branch queue
   ordering:
   - `active` with status `ready`: read that ticket only. Select it unless it
     carries a `## Blocked (...)` note; a blocked owner stops.
   - `active` with status `idea` or `todo`: stop because the owner is not
     executable.
   - `missing` or `ambiguous`: call `{{.McpNamespace}}/git.status()` and stop
     with its nudge line verbatim.
3. Without `impl_ticket`, inspect `ready/`. Skip candidates carrying a
   `## Blocked (...)` note, then prefer an in-progress ticket (some phase has a
   `### Result`, at least one does not), one named as a prerequisite by another
   ready ticket's `related:` or `parent:`, then the oldest.
4. On a `goal/*` branch, preserve the distinct `ready/ empty` and `every
   remaining ticket blocked` terminal results. Do not select another ticket
   after any implementation-branch stop.

## Output

```
selection: <ticket path | ready/ empty | every remaining ticket blocked | stop: reason>
omitted: none
```
