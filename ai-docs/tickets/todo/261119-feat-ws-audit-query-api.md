---
title: "감사 조회·검색 API"
parent: 261119-epic-ws-conversation-audit
blocked-by: 261119-feat-ws-audit-sqlite-store#phase1
related:
  261119-feat-ws-audit-sqlite-store: "prerequisite (blocked-by #phase1): the store this API reads"
  261119-feat-ws-audit-capture-harness-adapters: "source of the captured events, call IDs and usage behind duration_ms and ctx_after"
  261119-feat-ws-dashboard-audit-view: "consumer: every byte the view shows comes from this API"
  261114-feat-ws-conversation-audit-event-store: "superseded source of (599), (681) and the read-API verification"
---

# Audit query and search API

## Background

Fourth child of `261119-epic-ws-conversation-audit` ((1331)=a, 2026-11-19).
It serves the audit store to the dashboard view. The contract below was
approved under the superseded `261114-feat-ws-conversation-audit-event-store`
(owner `594..609,611=ok`, recorded 2026-11-14; (600) was the backend group
acknowledgment) and is carried unchanged except where the storage moved to
SQLite.

## Decisions

1. **Read API contract ((599), carried).** All routes are under the existing
   session prefix, return JSON, and require the same auth as
   `/s/<id>/history`.
   - `GET /s/<id>/audit/events?before=<event id>&after=<event id>&n=<count>&kinds=<k1,k2>`
     pages events, oldest first within the page. `n` defaults to 100, max
     500. It returns `{events: [summary], before, after, end, cut}`, where
     `summary` is `{id, seq, at, kind, subkind, call_id, parent_id, agent_id,
     model_seen, title, in_bytes, out_bytes, attached: [event id]}`.
     `attached` lists the events tied to a call (its result, hooks,
     reminders, subagent events).
   - `GET /s/<id>/audit/event/<event id>` returns the full envelope (`in`,
     `out`, `meta`, `raw`, `redactions`, `seen`). A body past 8 MiB is served
     as a truncated head with `truncated: true` and a `range` follow-up
     (`?part=out&offset=&len=`).
   - `GET /s/<id>/audit/search?q=<text>&regex=0|1&case=0|1&kinds=&n=` searches
     the stored `in` and `out` of the whole conversation, newest first. It
     returns `{hits: [{event_id, kind, field, line_no, line, call_id,
     parent_id}], next}`.
   - `GET /s/<id>/audit/events?call=<call_id>` returns that call's summary
     plus its attached events, so a click on a ring history row (which holds
     the ring's call ID, (601)) finds its audit events.
   - Additive fields the approved mockup needs: a `tool_call` summary carries
     `duration_ms` (null while the call runs) and `ctx_after`, the context
     size of the request that follows the call (input + cache_read +
     cache_creation, from the next assistant message's usage). Parallel calls
     answered in one request each get the same shared value.
   - Rejected: an MCP tool surface as the first consumer surface. It may come
     later as an additive T0 tool. The UI is the consumer.
2. **UTC selection ((681), carried, T0).** `GET /s/<id>/audit/events` also
   accepts optional `since=<RFC3339Nano UTC>` and `until=<RFC3339Nano UTC>`.
   - Select `since <= at < until`. Omitted bounds are unbounded. Malformed
     bounds or `since > until` return HTTP 400. Equal bounds select an empty
     interval.
   - Apply UTC and kinds selection before the event-ID cursors and the `n`
     limit. Preserve oldest-first per-session sequence order, the response
     shape, defaults and maximum page size. `before`/`after` remain event IDs,
     never timestamps.
   - Follow returned ID cursors with unchanged filters until `end`.
     Equal-nanosecond events are distinguished by sequence/event IDs.
   - Timestamp filters select hidden-event ranges only. `?call=` and (601)
     remain the sole history-call join.
   - Rejected: loading the whole retained log and filtering client-side, or a
     separate ring-to-event-ID resolver.
3. **Search runs over the SQLite store.** (599)'s note that search is a
   streaming scan of JSONL segment files with no index is superseded by
   (1331)=a. The search contract in Decision 1 is unchanged. How it runs on
   the settled layout (scan or a full-text index, with regex still honoured)
   is a technical choice made at ready promotion.
4. **Compatibility tier T0.** New routes only. Existing routes keep their
   semantics.

## Constraints

- Implementation conventions (AGENTS.md): `agents-plugin-tool/` ->
  `ai-docs/manuals/shipped-surface-boundary.md`; `agents-plugin-tool/internal/`
  -> `ai-docs/manuals/release-compatibility.md`; `internal/mcp/` ->
  `ai-docs/manuals/ws-mcp.md` (only if an MCP surface is ever added).
- UTC bounds are selection filters only. Call identity and linkage remain
  event/call IDs ((599)/(601)); timestamps never guess joins.
- Reads never block capture or the store writer; the dashboard's interactive
  paths never wait on file I/O beyond the query itself (parent epic
  file-touch rule).

## Phases

### Phase 1: Events, event, call lookup and search routes

The route families of Decision 1, with paging, the kinds filter, attached
links, the cut, call lookup, search and the optional UTC bounds of
Decision 2.

Verification: route tests on a fixture store, including paging boundaries,
a truncated large body with its range follow-up, and a search hit's line
number and `call_id`. Cover inclusive-lower/exclusive-upper UTC bounds;
omitted, equal, invalid and reversed bounds; kinds and event-ID cursors
combined; and more than 500 equal-nanosecond events across pages without
gaps or duplicates. Confirm requests without UTC bounds keep their contract,
call lookup stays ID-based, and auth matches `/history`.
