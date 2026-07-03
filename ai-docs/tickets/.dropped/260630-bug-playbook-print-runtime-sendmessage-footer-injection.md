---
title: "playbook_print runtime appends hardcoded SendMessage footer to all playbooks"
dropped: 2026-06-30
---

# playbook_print runtime appends hardcoded SendMessage footer to all playbooks

## Background

Dogfood surprise (2026-06-30, policy-sweep Track 1 dogfood run).

`ws/playbook.print` appends a hardcoded continuity tip footer to every rendered
playbook, regardless of file content:

> "use `SendMessage(to: <agentId>)` to send follow-up messages to the same agent
> rather than spawning a new one"

This footer is injected at the runtime layer, not from the rsrc file body. Ticket
`260619-research-ws-delegate-continuity-host-neutral-fallback` fixed the file-body
tip in `lead-implement`, but the runtime-injected footer was not addressed and
continues to instruct SendMessage as the canonical continuity method for all
playbooks.

## Impact

- Any agent reading a rendered playbook sees the old SendMessage-specific tip even
  when the playbook's file body contains host-neutral guidance, creating a
  contradiction.
- New playbooks (e.g. `fresh-reader-audit`) inherit the SendMessage-centric footer
  by default regardless of their intended scope.
- On harnesses where `SendMessage` is not available (default Claude Code), agents
  following the footer tip dead-end silently.

## Direction

Replace the hardcoded runtime footer with either:
- A host-neutral tip consistent with the 260619 file-body fix
- Or remove the footer and let each playbook's file body own the continuity guidance

Coordinate with epic `260605-epic-ws-playbook-factory-pivot` (adapter-boundary work).


## Resolution (2026-06-30)

False alarm. The runtime footer is harness-aware (playbookTerminologyTable); the claude entry correctly uses SendMessage which IS available in Claude Code. No fix needed.
