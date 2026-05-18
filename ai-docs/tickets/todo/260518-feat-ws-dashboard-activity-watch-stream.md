---
title: ws dashboard Activity Feed watch stream
parent: 260518-epic-ws-dashboard-activity-console
related:
  260518-feat-ws-dashboard-activity-feed-api: feed item contract consumed by stream events
  260517-feat-ws-dashboard-workroot-activity-live-refresh: absorbed narrow SSE/filewatch follow-up
  260513-feat-async-exec-output-reader: future exec activity may share feed invalidation and fallback behavior
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
---

# ws dashboard Activity Feed watch stream

## Background

The current Activity pane uses a bounded polling hotfix while open: it fetches
recently modified named-agent rows and merges them into the initial full
projection. That unblocks dogfood but is not the durable dashboard model. The
Activity Console needs a workRoot-scoped watcher and read-only event stream so
the ribbon and selected transcript can update as local wsagent files change.

## Decisions

- Prefer SSE for read-only feed events. Use WebSocket only if an implementation
  phase records a concrete bidirectional need.
- Watch only selected or otherwise visible workRoots. Do not eagerly watch every
  remembered/opened root.
- Keep a bounded polling fallback for platforms or filesystems where watcher
  behavior is unavailable, lossy, or difficult to prove.
- Stream item invalidations or row updates, not raw file paths or backend-native
  event payloads.

## Constraints

- File watching must be cross-platform and handle atomic renames, nested
  `current/` files, missing directories, agent erasure, and recreated agent
  directories on macOS, Linux, and Windows.
- Watch events must be coalesced or debounced enough to avoid repeatedly
  rebuilding a monotonically growing full feed for a single backend write.
- Stream events must remain owner-authenticated and must not begin before
  route auth succeeds.

## Phases

### Phase 1: Add watcher abstraction and fallback mode

Introduce a daemon-side watcher abstraction for workRoot Activity sources. It
should observe the resolved agents directory and key child files such as
`agent.json`, `events.jsonl`, `output.md`, `current/state.json`,
`current/stdout`, and `current/stderr` without exposing those paths to browser
callers.

Verification should include deterministic unit tests for watch-target
selection, unavailable directory fallback, deletion/recreation handling, and
platform-independent event normalization. Native Windows evidence may be
recorded as a later implementation note when the watcher crate or OS behavior
requires host dogfood.

### Phase 2: Add authenticated feed event stream

Add a workRoot-scoped feed event stream that emits bounded events such as item
upsert, item removal, transcript update, snapshot invalidation, heartbeat, and
fallback-mode notification. The stream should be cursored or otherwise
reconnect-safe enough for the browser to refresh a snapshot after missed
events.

Verification should cover unauthenticated rejection, selected workRoot
subscription, heartbeat/fallback behavior, agent file change invalidation, and
private-field redaction.

### Phase 3: Replace the open-pane polling hotfix

Update frontend Activity state consumption so an open Activity Console uses the
feed stream when available and falls back to bounded polling only when the
daemon reports fallback mode or the stream is unavailable.

Verification should prove that newly registered or called named agents appear
in the ribbon without browser reload, call status transitions update while a
call runs or completes, stale root updates are ignored after switching
workRoots, and the old always-on interval path is removed or limited to
fallback mode.
