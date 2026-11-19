---
kind: print
---

# Tracker: event hooks

The dashboard daemon runs local commands on tracker events, such as a
desktop notification or a mail. Write or edit the hook file only when the
user asks. The file is `tracker-hooks.json` in the daemon's state
directory, `dashboard/` under the ws cache root (`$WS_CACHE_HOME` when
set), next to `endpoint.json`:

```json
{"hooks": [{"command": ["notify-tracker", "--quiet"], "scope": "mine"}],
 "due_soon_hours": 24}
```

- `command`: an argv run without a shell, in the state directory, with the
  daemon's environment. `scope`: `mine` (default: events that concern the
  store's local member, not their own actions) or `all` (every event of the
  store). `due_soon_hours` (default 24): how long before a due date the
  `due_soon` event fires. Each hook applies to every registered store.
- The daemon reloads the file when it changes; an invalid file runs no hook,
  and the daemon log says why.
- Each run gets one event as JSON on stdin. A hook's runs are serial and in
  order, each stopped after 30 seconds; a failure is logged, not retried.
  After downtime the daemon delivers what arrived meanwhile, up to 24 hours
  old.

The event:

- `v`: 1. Adding a field keeps it; changing a field's meaning bumps it.
- `id`: the tracker event's ID; `due:<item id>:<due date>` for `due_soon`.
  An event that assigns several members gives one event per member, same
  `id`.
- `kind`: `assigned` (also a claim or an approved take request, for its
  taker), `assign_pending` (a request or handoff awaiting acceptance),
  `take_request` (a member asks for the assignment `target` holds),
  `take_denied` (to the requester, `target`), `comment`, `status`,
  `closed`, `due_soon`. Ignore an unknown kind; new kinds come without a
  `v` bump.
- `ts`: RFC 3339.
- `store`: `{id, name}`; `item`: `{id, alias, title, kind}`.
- `actor`: `{id, name}` of the member who acted; null for `due_soon`.
- `target`: `{id, name}` of the member the event is aimed at (the assignee,
  the holder asked, or the requester denied), when there is one.
- `detail`: `status` (for `status` and `closed`: the status written, so
  `in-review` for a close that awaits the assigner's review and `dropped`
  for a drop), `comment` (its first 500 characters), `start` and `due`
  (RFC 3339, each present when the item has that date); absent fields are
  omitted.
- `url`: opens the item in this machine's dashboard.
