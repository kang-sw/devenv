# Tracker unhide: backend part of Phase 1

Ticket: `ai-docs/tickets/todo/261121-feat-ws-tracker-unhide.md` (read it
fully; its Decisions are binding).

## Scope (this assignment)

Backend only, in `agents-plugin-tool/internal/wstracker/` plus the
dashboard's server-side tracker plumbing if an unhide action needs a route or
view field to reach the browser:

- `item.unhide` and `comment.unhide` payload types next to the hide types
  (`payload.go`).
- Checks mirroring the hide permissions (`check.go` hide checks): item creator,
  or another member when the creator is gone; comment author only. Reject
  unhiding a not-hidden target, and reject unhiding a child whose parent is
  still hidden ("unhide the parent first").
- Folds: `item.unhide` writes `false` to the `it:<id>:hidden` register;
  `comment.unhide` clears the comment's hidden state.
- View: whatever the dashboard needs to list hidden items and comments (for
  the "show hidden" toggle) and to know whether the viewer may unhide each
  (a `canUnhide`-style field, matching how `canHide` is surfaced today).
- Tests next to the existing hide tests.
- The T1 compatibility check named in the ticket: from source (with
  file:line citations) or a test, show what a pre-change binary does with a
  log containing `*.unhide` events and whether a rejected event disturbs the
  fold of later events. If it does, stop and report - do not settle T1.

Out of scope: browser JS/CSS/HTML, the toggle UI, i18n strings for the UI
(a separate agy worker does the UI on this same branch afterwards). Do not
move the ticket status or write a `### Result` section.

## Verify

`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wstracker/...`
from `agents-plugin-tool`, plus narrow `-run` filters for any dashboard
tracker handlers you touch (never the whole `./internal/wsdashboard` or
`./internal/mcp` package). `go vet` on touched packages.

## Report

Commit on the current branch. Report: commit sha, test commands and results,
the T1 verdict with citations, and the exact view/route fields the UI worker
should consume.
