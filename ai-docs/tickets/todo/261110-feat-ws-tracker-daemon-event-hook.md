---
title: "Tracker: the dashboard daemon runs a configured local command on tracker events"
parent: 261110-epic-ws-tracker-team-feedback
---

# Tracker: daemon event hook

## Background

From the team introduction feedback (user, 2026-11-10 whiteboard, topic
"tracker 팀 피드백"): connect the tracker to mail. Mail integration is
downstream work. The user's direction is for the daemon to expose
tracker-related hooks that a downstream setup or a client plugin can use;
per-PC desktop notifications (for example Windows toasts) can later use the
same hook. Today the daemon pushes tracker changes only to browser pages
over SSE, and nothing reaches an outside program.

## Decisions

1. **The hook is a configured local command that the daemon runs, with the
   event as JSON on stdin (lead technical decision, user ok on (615)).**
   Rejected: an HTTP webhook, or an event-stream endpoint for clients to
   subscribe to. Those need a server or an open port and fail offline. A
   local command works offline, a Windows toast is one PowerShell script, and
   a webhook can still be made from the script with curl.
2. **By default the hook receives only events that concern the local
   member, and a setting switches it to every store event (user, (616) c).**
   Events that concern the local member are: an assignment or request to me,
   and a comment or status change on an item I created or hold, plus a due
   date approaching on an item I hold. Rejected: local-member events only
   (a); every event only, leaving the filter to the script (b).

## Constraints

- To settle at promotion:
  - where the hook is configured (the dashboard config manifest or a
    per-machine file) and whether several hooks are allowed;
  - the event JSON shape and its versioning;
  - the "due approaching" threshold, and how a due event fires once rather
    than on every reload;
  - delivery: at-most-once versus catch-up after the daemon was down;
  - timeout, concurrency, and failure logging for the command;
  - how events are deduplicated across the daemon's tracker reloads, given
    the late-reload path counted in `/debug/pprof/streams`.
- The command runs with the daemon's environment and privileges. It is
  configured locally by the machine's own user, never taken from store
  content, because a store is shared and its content must not be able to
  choose what runs.
- The daemon process code is shipped surface:
  `ai-docs/manuals/shipped-surface-boundary.md` applies.

## Phases

### Phase 1: hook

Scope and verification are settled at promotion, after the Constraints'
open items.
