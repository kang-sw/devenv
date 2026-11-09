---
title: "Tracker search: a query grammar parsed once in Go, served to the dashboard through /tracker/search, covering item fields and comment text"
related:
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "changes the statuses a status: filter names"
  261110-feat-ws-tracker-topic-ownership-and-kind-merge: "owned by another session; merges task/todo, which a kind: filter names"
---

# Tracker search: query grammar

## Background

User, 2026-11-09/10 whiteboard ((541), (542)): the Items panel search box
should filter better. Today it is a client-side substring match on title,
body, project, ID, and alias (`agents-plugin-tool/internal/wsdashboard/assets/tracker.js:800-810`,
`matchItem`), combined with the Today filter. Status, assignee, due, and
priority cannot be searched, nor can comments.

## Decisions

1. **One query parser, in Go; the dashboard asks the daemon (user ack,
   (541)).** A new route `/tracker/search?q=` returns the matching IDs for
   the current store, and the panel filters by them; the client-side
   `matchItem` substring logic is replaced. Typing is debounced. Rejected: a
   second parser in JS, which would drift from Go so that `tracker.query`
   and the screen disagree.
2. **Scope: item fields, with field filters for status, assignee, due, and
   priority, plus comment and thread text, in the current store (user,
   (542) b).** Rejected: item fields only ((542) a); searching across every
   registered store ((542) c), which needs its own result and permission UI.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/ws-mcp.md` before editing `internal/mcp/` (if the
  `tracker.query` change in the queue lands).
- Status names come from the tracker fold; the idea status and the retired
  review stage arrive with
  `261110-feat-ws-tracker-idea-status-and-assignment-invariant`.

## Phases

### Phase 1: grammar, route, and panel

The Go parser and matcher, the route, the panel switch-over with debounce,
and the `tracker.query` change settled in the queue. Verification: `go test
./...` under `agents-plugin-tool` with a table of queries (each field
filter, quoted phrases, comment matches), and a headless-browser check of
the panel.

## Open Decision Queue
Last ID: (567)

(565) [open] [technical] The grammar is `key:value` tokens plus free text, every token AND-ed: `status:doing`, `assignee:maya`, `kind:task`, `project:x`, `due:<2026-11-20` (also `>`, `<=`, `>=`, a bare date for equality), `priority:>=3`, `"quoted phrase"`; bare words match title, body, project, ID, alias, and comment text. Over a structured filter form - the familiar issue-tracker convention, typed in the box that exists.
(566) [open] [policy] `tracker.query` gains a `q` payload field parsed by the same grammar, and `text` keeps its substring meaning.
- Context: (541)'s reason for one parser was that agents and the screen should agree.
- Alternative a: `text` itself is parsed by the grammar, a semantic change to an existing API field (a colon in a search would start parsing as a filter).
- Alternative b: a new `q` field, additive; `text` unchanged.
- Alternative c: dashboard only; `tracker.query` unchanged.
> b: agents get the same results as the screen without changing what `text` means today.
(567) [open] [announced] The 2026-11-10 tickets land in `todo/`, standalone with `related:` links - user, 2026-11-10: "다 todo로요".
