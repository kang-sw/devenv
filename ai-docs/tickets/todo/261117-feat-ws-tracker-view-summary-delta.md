---
title: "Tracker view: summary first, deltas since a revision, incremental server view"
---

# Tracker view: summary first, deltas since a revision, incremental server view

## Background

The dashboard tracker view (`GET /tracker/stores/<store>/view`) sends the
whole store as one JSON document. Two fixes reduced its repeat cost:

- `41a1fa839` (tracker-view-refetch): the view is cached by data revision,
  answered with a weak ETag and 304 when unchanged. The owner measured a hit
  at 7 ms on 2026-11-17.
- `impl/develop/tracker-miss-path` (in flight on 2026-11-17): a miss whose
  ledger and git state did not change spawns no git and reloads nothing.

Both only remove waste when nothing changed. Every store change (one entry
added, one status changed) still costs O(N) in entries, three times:

| step | cost |
|---|---|
| server rebuilds the whole view | CPU and file reads |
| the whole view travels as JSON | 291 kB at the time of writing |
| the browser parses and re-renders all of it | render time |

The owner expects stores to reach tens of thousands of entries. At that size
each change would move several MB to over 10 MB, and with several sessions
writing, changes arrive every few seconds. The owner's PC runs AhnLab V3, and
target users run heavier security software, so file touches and large
responses are the cost to cut (clone note
`release-gate-daemon-file-io-20261117`).

## Decisions

- **Direction (owner (1307)=a, 2026-11-17 18:23).** The view is redesigned
  along three lines:
  1. **Summary first, detail on demand.** The first response carries only
     what the list needs (title, status, time). Bodies and details are
     fetched when the user expands an entry or it scrolls into view.
  2. **Deltas since a revision.** The browser sends the revision it holds;
     the server returns only the entries changed since then. The server keeps
     the per-revision change list in memory and does not reread files.
  3. **Incremental server view.** The server applies changed entries to its
     in-memory view instead of rebuilding from scratch, and runs no git while
     the store stamp is unchanged.
- **Sequencing (owner (1307)=a).** This work starts after the current
  daemon file-load work (2026-11-17 perf queue) settles.
- Mechanism choices (endpoint shape, revision and change-list retention,
  fallback to a full summary when the browser's revision is too old, the
  detail-fetch granularity) are not yet decided. Settle them through the Open
  Decision Queue at `ready/` promotion.

## Constraints

- Expected compatibility tier T0: new endpoints are added beside the current
  view; an old binary and an old page keep using the full view. Confirm the
  tier with source lines at promotion.
- Keep the 304/ETag behaviour from `41a1fa839`.
- Read the manuals in `AGENTS.md` `### Implementation Conventions` for the
  touched paths (`agents-plugin-tool/internal/` -> release-compatibility).
- No local `go test`, Go test executables, ws exes or launcher on the owner's
  PC; Go runs only on the w5090 remote (clone note
  `no-local-go-test-v3-20261116`).
- Verification must include a measured before/after on a large synthetic
  store: response bytes and server time per change, and browser render time.

## Phases

### Phase 1: Summary-first view with revision deltas

Settle the open mechanism decisions above, then implement the summary
response, the since-revision delta response, the in-memory incremental view,
and the browser's detail-on-demand fetch.
