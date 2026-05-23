---
title: Persist dashboard open workRoots across daemon restarts
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260523-bug-ws-dashboard-dev-run-ctrl-c-shutdown: dogfood now encourages restarting the local server frequently
  260523-feat-ws-dashboard-linked-worktree-discovery: adjacent resource-discovery persistence concern
spec:
  - 260516-ws-web-dashboard-workroot-io-restore-model
related-mental-model:
  - ws-web-dashboard
---

# Persist dashboard open workRoots across daemon restarts

## Background

Dogfood feedback found that repeatedly starting and stopping
`ws-dashboard/dev.sh run` is inconvenient because the dashboard returns to an
empty live resource tree after every daemon restart. The current implementation
keeps `OpenedWorkRoots` as an in-memory map in daemon state; browser workbench
arrangement is also presentation state held in React state. That matches the
early substrate but makes normal development restarts lose useful context.

The first persistence step should restore the owner's recent/open workRoot list
after daemon restart. This is different from keeping live terminal PTYs alive
across daemon process death; terminal processes remain daemon-owned live
sessions and cannot survive that process boundary without a separate supervisor
design. If feasible, the dashboard should still remember enough terminal
presentation state to recreate previously open terminal tabs with their intended
workRoot and working-directory hint, while making clear that the old process
itself was not resumed.

## Decisions

- Phase 1 persists only opened workRoot paths. It does not persist auth
  sessions, live terminal processes, Activity acknowledgement state, or exact
  browser workbench arrangement.
- The persisted store is daemon-owned local state. Browser localStorage alone is
  insufficient because the canonical `/api/dashboard/resources` route should
  show remembered workRoots before the browser opens one manually.
- Restored paths re-enter normal discovery on every daemon start, so moved,
  offline, inaccessible, primary-root, and linked-worktree states stay honest.
- Terminal process survival is out of scope. A later phase may restore terminal
  tab placeholders or recreate shells in a captured PWD if the terminal model
  gains a safe PWD hint.

## Phases

### Phase 1: Persist opened workRoot paths

Implement a daemon-owned local JSON state file for opened workRoot paths. On
startup, the daemon seeds `OpenedWorkRoots` from that file before building the
router, and `GET /api/dashboard/resources` reports those remembered roots
through the existing discovery provider. Opening a new workRoot updates the same
store after the in-memory registration succeeds.

Keep the first persistence format minimal and bounded: versioned envelope,
deduplicated paths, deterministic ordering, and best-effort recovery from
missing or malformed state by returning an empty remembered set instead of
failing daemon startup. Tests should cover startup restore, open-time write,
malformed-state degradation, and that authentication/pairing remains unchanged.

### Phase 2: Terminal tab/PWD restore design

Deferred. Define a separate terminal restore contract before implementation.
The desired shape is restoring useful terminal tab context after daemon restart,
possibly workRoot plus PWD and a clear non-resumed placeholder or newly spawned
shell. This needs a safe way to capture PWD or a user-visible approximation
without pretending the old PTY process survived.
