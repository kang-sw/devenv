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
3. **The grammar is `key:value` tokens plus free text, every token AND-ed
   ((565)):** `status:doing`, `assignee:maya`, `kind:task`, `project:x`,
   `due:<2026-11-20` (also `>`, `<=`, `>=`, and a bare date for equality),
   `priority:>=3`, and `"quoted phrase"`; bare words match title, body,
   project, ID, alias, and comment text. Rejected: a structured filter form
   (the `key:value` box is the familiar issue-tracker convention and is
   typed in the box that exists).
4. **`tracker.query` gains a `q` payload field parsed by the same grammar,
   and `text` keeps its substring meaning (user, (566) b),** so agents get
   the screen's results. Rejected: parsing `text` itself by the grammar
   ((566) a, a semantic change to an existing field: a colon in a search
   would start parsing as a filter); a dashboard-only change ((566) c).

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/ws-mcp.md` before editing `internal/mcp/` (Decision 4).
- Status names come from the tracker fold; the idea status and the retired
  review stage arrive with
  `261110-feat-ws-tracker-idea-status-and-assignment-invariant`.

## Phases

### Phase 1: grammar, route, and panel

The Go parser and matcher, the route, the panel switch-over with debounce,
and the `tracker.query` `q` field (Decision 4). Verification: `go test
./...` under `agents-plugin-tool` with a table of queries (each field
filter, quoted phrases, comment matches), and a headless-browser check of
the panel.
