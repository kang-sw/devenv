---
title: "implementer prompt blocks required ticket Result checkpoint"
---

# implementer prompt blocks required ticket Result checkpoint

## Observation

During Phase 2 of `260730-chore-ws-dashboard-drop-sweep`, the rendered
implementer prompt required a ticket `### Result` update but prohibited reading
ticket files unless the generated plan authorized an escalation. The plan had
`## Escalations: None`, so the implementer correctly stopped after the source
checkpoint instead of assuming authorization.

## Direction

Make ticket-driven implementer prompts authorize the selected ticket's Result
read/write checkpoint explicitly, or render an unambiguous conditional that
does not conflict with the required result expectation. Add a regression test
for a ticket phase with a required Result update and no escalations.
