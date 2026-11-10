---
title: "Tracker search: a query grammar parsed once in Go, served to the dashboard through a per-store search route, covering item fields and comment text"
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "changes the statuses a status: filter names"
  261110-feat-ws-tracker-support-agent-ui-commands: "its open-search command takes this grammar; the later of the two updates the tracker.ui description"
  261110-feat-ws-tracker-topic-ownership-and-kind-merge: "done (bbb14e07c); merged task/todo, which a kind: filter names (the tracker.query kind enum is now task, topic: agents-plugin-tool/internal/mcp/tracker_tools.go#L1003-L1006)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 9057b416ee62b5c1
sage-review-design-reviewed: 9057b416ee62b5c1
---

# Tracker search: query grammar

## Background

User, 2026-11-09/10 whiteboard ((541), (542)): the Items panel search box
should filter better. Today it is a client-side substring match on title,
body, project, ID, and alias (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js:808-814`,
`matchItem`), combined with the Today filter. Status, assignee, due, and
priority cannot be searched, nor can comments.

## Decisions

1. **One query parser, in Go; the dashboard asks the daemon (user ack,
   (541)).** A new store op, `/tracker/stores/<id>/search?q=` (alongside
   the existing `trackerStoreOps`, `tracker_routes.go#L344-L357`), returns
   the matching IDs for that store, and the panel filters by them; the client-side
   `matchItem` substring logic is replaced. Typing is debounced. Rejected: a
   second parser in JS, which would drift from Go so that `tracker.query`
   and the screen disagree.
2. **Scope: item fields, with field filters for status, assignee, due, and
   priority, plus comment and thread text, in the current store (user,
   (542) b).** Rejected: item fields only ((542) a); searching across every
   registered store ((542) c), which needs its own result and permission UI.
3. **The grammar is `key:value` tokens plus free text, every token AND-ed
   ((565)):** `status:doing`, `assignee:maya`, `kind:task`, `project:x`,
   `due:<2026-11-20` (also `>`, `<=`, `>=`, and a bare date for equality),
   `priority:>=3`, and `"quoted phrase"`; bare words match title, body,
   project, ID, alias, and thread comment text (the item's thread, the
   same scope Decision 2 names). Rejected: a structured filter form
   (the `key:value` box is the familiar issue-tracker convention and is
   typed in the box that exists).
   Matching rules: keys and values are case-insensitive; `status:` takes
   the fold's item-level statuses plus tracker.query's `open` and `hidden`
   and means the item's derived status, also in the *mine* view where rows
   are assignment records; `assignee:` matches any live or pending
   assignee's name; `due:` and `priority:` comparisons never match an item
   without that field; dates are `YYYY-MM-DD`. A token whose key is not one
   of the listed keys (a pasted URL, `foo:bar`) is free text, never a
   refusal; a known key with a malformed value (`due:<tomorrow`) is a
   refusal naming the token, and the panel shows that message under the
   search box and leaves the list unfiltered.
4. **`tracker.query` gains a `q` payload field parsed by the same grammar,
   and `text` keeps its substring meaning (user, (566) b),** so agents get
   the screen's results. `q` AND-s with the payload's existing `kind`,
   `status`, `project`, and `text` filters. Rejected: parsing `text` itself by the grammar
   ((566) a, a semantic change to an existing field: a colon in a search
   would start parsing as a filter); a dashboard-only change ((566) c).
5. **The parser and matcher live in `internal/wstracker`,** where both
   `internal/mcp` (`tracker.query`, today's `trackerItemMatches` in
   `tracker_render.go#L145-L185`) and `internal/wsdashboard` can import
   them. Rejected: keeping the matcher in `internal/mcp`, which the
   dashboard daemon would have to import for one function.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/ws-mcp.md` before editing `internal/mcp/` (Decision 4).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Status names come from the tracker fold; the idea status and the retired
  review stage arrive with
  `261110-feat-ws-tracker-idea-status-and-assignment-invariant`, which owns
  the `trackerStatuses` list (`mcp/tracker_tools.go:1009`); validate
  `status:` against that list rather than a second copy. The two tickets
  run alongside and touch the same tracker.js filter code,
  `tracker_tools.go`, and `tracker.md` copies; neither depends on the other.
- `261110-feat-ws-tracker-support-agent-ui-commands` adds an "open the
  search box with a query" command whose query is this grammar. Whichever
  of the two lands second updates the `tracker.ui` tool description and the
  support prompt (`internal/wsagent/instructions/tracker-support.md`) to
  name the grammar and point at `tracker.query` `q`.
- The panel's ID set goes stale after a write or an onChange refresh: the
  panel re-runs the search when the store view changes, not only on typing.

## Prior Decisions

- e7944d14 (2026-11-10, commit): "The create enum keeps todo so an agent that still sends it succeeds (the store folds it to task); only the query filter enum drops it, since no folded item ever has kind todo." — bearing: constrains
- 261110-feat-ws-tracker-topic-ownership-and-kind-merge (2026-11-10, Decisions): "Task and todo merge into one kind; the `kind` field stays (board 154 a, user). ... Existing `todo` items display and filter as `task`." — bearing: constrains
- 261110-feat-ws-tracker-topic-ownership-and-kind-merge (2026-11-10, Decisions): "The dashboard's `ctxTree` (`tracker.js:875-897`) walks each matched row's parent chain and draws non-matching ancestors as greyed context rows" — bearing: constrains
- 78af82e6 (2026-11-08, commit): "Esc in an empty search box propagates (closes the window or drawer); Esc with text clears it first. IME composition is not interrupted." — bearing: constrains
- d39b84d6 (2026-11-08, commit): "Mechanism: the parser owns Decode and the grammar declares where it applies, so whiteboard and tracker share one implementation. Rejected: decoding at each caller (two copies...)" — bearing: supports
- 9eff2c86 (2026-11-08, commit): "Board citations like (354) were dropped from the shipped comments and one user-visible string ... was reworded, since the asset ships downstream." — bearing: constrains
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "search indexes are built per store and stay local, outside git. A local AI may search across the user's stores read-only, only to answer the user's own query." — bearing: supports

## Route Facts

| fact | allowed values | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker.js, agents-plugin-tool/internal/wsdashboard/tracker_routes.go, agents-plugin-tool/internal/mcp/tracker_tools.go, agents-plugin-tool/internal/mcp/tracker_render.go, plus a new Go parser file |
| scope.surface | public-interface | new dashboard HTTP route and a new tracker.query payload field q; the shipped tracker playbook documents the payload at agents-plugin/rsrc/tracker/tracker.md#L45-L56 |
| scope.new_public_symbol | yes | /tracker/stores/<id>/search store op and tracker.query payload.q |
| scope.new_type_contract | yes | trackerQuery gains q; the search route returns matching item IDs; the key:value grammar itself |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tracker_tools_test.go, agents-plugin-tool/internal/wsdashboard/tracker_routes_test.go |
| complexity.reuse_points | confirmed | trackerItemMatches in agents-plugin-tool/internal/mcp/tracker_render.go#L145-L185; wstracker.ItemView fields Status, Due, Priority, Assignments, Thread in agents-plugin-tool/internal/wstracker/view.go#L98-L133 |
| complexity.side_effect_risk | moderate | replaces the synchronous matchItem used by keepRec and keepItem with an async debounced daemon call that must combine with the Today filter and ctxTree |
| risk.correctness | moderate | tokenizer with quotes, comparison operators on dates and priority, assignee and comment-text matching across the fold |
| risk.fit | moderate | parser must be shared by internal/mcp and internal/wsdashboard; search follows the existing per-store /tracker/stores/id/op routes (Decision 1) |
| risk.test | moderate | Go table tests plus a headless-browser panel check of debounce and filter combination |
| risk.security_or_contract | moderate | adds a field to a shipped MCP tool payload and its refusal text, mirrored in three tracker playbook copies |

## Phases

### Phase 1: grammar, route, and panel

The Go parser and matcher, the route, the panel switch-over with debounce,
and the `tracker.query` `q` field (Decision 4) with its payload docs in
`agents-plugin/rsrc/tracker/tracker.md` (and the byte-identical wsflow and
Pi copies, via the rsrc after-edit checklist in
`ai-docs/manuals/wsflow-mirroring.md`). Verification: `go test
./...` under `agents-plugin-tool` with a table of queries (each field
filter, quoted phrases, comment matches, an unknown key as free text, a
malformed known key refused, `q` combined with `text` and `status`), and a
headless-browser check on an isolated daemon that: typing filters the list
after the debounce; the result combines with the Today filter; a matched
row's non-matching ancestors still show greyed through `ctxTree`; Esc
behaves as 78af82e6 states; a malformed query shows its message; and a
write that changes a match refreshes the list.

### Result (6d0736cb7) - 2026-11-10

Landed in 91bf22ca8 and 6d0736cb7.

- `internal/wstracker/query.go`: `ParseQuery`, `Query.Match`,
  `View.Search`, `QueryError`, and `QueryStatuses` / `QueryKinds` (the single
  status and kind lists; `mcp`'s `trackerStatuses` / `trackerKinds` alias
  them). Free text also matches the item's shown thread comments, ID in both
  forms, and alias.
- `tracker.query` takes `payload.q`, AND-ed with `kind`, `status`, `project`,
  and `text`; `text` keeps substring meaning; a malformed token refuses as
  `payload.q: search "<token>": <what the key takes>`.
- Dashboard: `GET /tracker/stores/<id>/search?q=` -> `{ids}` (400 with the
  message on a malformed query) through a new `TrackerHost.Search`,
  implemented in `cmd/ws-mcp` so `wsdashboard` stays free of the tracker
  packages (cosmetic deviation from Decision 5's wording). The panel calls
  `adapter.search`, debounced 250 ms; it re-sends at once when the store's
  view object changes; a malformed query shows the message under the box and
  leaves the list unfiltered. Adapters without `search` (the mockup) keep the
  substring match.
- Tracker playbook (ws, wsflow, pi copies) documents `q`; `ws-mcp.md` lists
  the route.

Decisions: a query without a status token keeps hidden items, and
`tracker.query` keeps them only for `status:hidden` (field or token);
`assignee:` matches part of a live or pending assignee's member id or name;
`project:` is exact (case-insensitive); `priority:` compares the level.

Verification: `go test ./... -count=1` under `agents-plugin-tool` on the
remote build server: all 29 packages ok; after the review fixes, `wsrsrc`,
`wsdashboard`, `mcp` ok. Tests added: `wstracker` query table (each field
filter, quoted phrases, comment matches, unknown key as free text, every
malformed known key), `TestTrackerQuerySearchField` (`q` with `status` and
`text`, mine scope, hidden), route and adapter tests, and the node-run
`TestTrackerSearchRequests`. Headless browser on an isolated daemon: 12/12
checks (debounce, comment match, ctxTree greyed ancestor, Today combination
and empty state, Esc clears and keeps focus, malformed message with an
unfiltered list, refresh after a write). Lite review: no Critical or
Important; minors 1 and 3 fixed in 6d0736cb7; minor 2 (`status:hidden`
lists ids the panel never draws as rows) left: the panel's hidden-row policy
is out of scope.

Follow-up for the support-UI ticket: it lands second, so it updates the
`tracker.ui` description and support prompt to name this grammar and
`tracker.query` `q`.
