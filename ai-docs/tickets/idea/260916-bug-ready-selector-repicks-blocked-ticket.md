---
title: "Stop ready selection from re-picking explicitly blocked tickets"
related:
  260914-chore-ws-pi-root-manifest-runtime-deps: reproduced with its owner-action Blocked note
  260916-feat-pi-agent-gutter-active-time-placement: advanceable ticket starved by the selection
---

# Stop ready selection from re-picking explicitly blocked tickets

## Background

A live `ws:lead-run` invocation selected `260914-chore-ws-pi-root-manifest-runtime-deps` even though that ready ticket carries `## Blocked (2026-09-16)` stating that both phases are implemented, no agent can advance the remaining owner-run smoke, and lead-run must not re-pick it. The commit that added the note likewise says it exists so the ready-queue selector stops selecting the ticket.

`tickets.query` correctly reported no `dispatch_blocked` value because this is an owner-action `## Blocked` note rather than a `blocked-by:` prerequisite. The selector nevertheless returned the ticket as its sole selection, starving the advanceable `260916-feat-pi-agent-gutter-active-time-placement` ticket behind it.

## Constraints

- Preserve `blocked-by:` prerequisite handling; an explicit dated `## Blocked` note is a separate queue-state signal.
- A blocked ready ticket must remain visible in all-blocked reporting rather than disappearing from queue diagnostics.
- Do not require a worker dispatch merely to rediscover a block already recorded in the ticket.

## Phases

### Phase 1: Respect active Blocked notes during ready selection

Update ready selection so a ticket whose current `## Blocked` note says lead-run cannot advance it is not returned while another ready ticket is advanceable. When every remaining ready ticket is blocked, return the selector's blocked outcome with those tickets and their recorded reasons.

Verification:

- Reproduce the observed mixed queue: one ready ticket with an owner-action `## Blocked` note and one advanceable ready ticket; selection must return the advanceable ticket.
- Cover an all-blocked queue and assert that each blocked ticket and reason remains reportable.
- Preserve selection and `dispatch_blocked` behavior for prerequisite blocks and for ready tickets without an active Blocked note.
