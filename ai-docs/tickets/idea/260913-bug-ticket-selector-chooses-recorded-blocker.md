---
title: Ticket selector can choose a ready ticket with a current Blocked note
related:
  260908-feat-ws-pi-claude-delegate-tool: blocked-ticket example selected during dogfood
  260911-bug-ws-pi-question-queue-dogfood: advanceable ticket omitted by the incorrect selection
---

# Ticket selector can choose a ready ticket with a current Blocked note

## Background

During a Pi goal-run drain, `ticket-selector` first reported that every remaining ready ticket was blocked and correctly reproduced the owner-acceptance blocker on `260908-feat-ws-pi-claude-delegate-tool`. After an unrelated stale blocker was cleared from `260911-bug-ws-pi-question-queue-dogfood`, a fresh selector invocation chose the still-blocked Claude delegate ticket instead of the newly advanceable question-queue ticket. A correction prompt that explicitly required skipping current `## Blocked` sections then selected the question-queue ticket.

The selector therefore does not consistently apply recorded blockers when at least one ready ticket is advanceable. This can dispatch workers into known owner-only or track-routing stops and can prevent a goal-run queue from making progress.

## Phases

### Phase 1: Make ready selection consistently blocker-aware

Ensure the selector excludes ready tickets carrying a current dated `## Blocked` section when choosing ordinary queue work, while preserving direct-ticket invocation behavior. Return `every remaining ticket blocked` only when every remaining ready ticket is currently blocked, and include the recorded blocker inventory needed by the goal-run terminal.

Add regression coverage for a mixed ready queue containing both a blocked earlier ticket and an advanceable later ticket, plus the all-blocked case. Verify that clearing one ticket's stale blocker makes that ticket selectable without making other blocked tickets eligible.
