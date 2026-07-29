---
title: terminal creation failure is invisible in the UI
related:
  260725-feat-dashboard-nav-row-two-line-open-state: found-during
  260725-bug-dashboard-terminal-socket-path-length-unguarded: found-during
---

# terminal creation failure is invisible in the UI

## Background

`App.tsx`'s `createTerminalPane()` (~`App.tsx:5364`) calls `createTerminal(...)`
and swallows any rejection: `.catch(() => undefined)` (~`App.tsx:5398`). When
the daemon's `create_terminal` endpoint returns a non-2xx response (for
example the HTTP 400 `{"error":"terminal spawn failed"}` produced by the
macOS socket-path-length failure mode described in
`260725-bug-dashboard-terminal-socket-path-length-unguarded`), nothing
happens from the user's point of view: no new tab, no toast, no console
error, no state change at all. The user clicks "New terminal" and the click
appears to do nothing.

This silence is what made the socket-path bug hard to diagnose from the
frontend side in the first place — the failure was only found by reproducing
it directly against the daemon HTTP API.

The codebase already has an established local pattern for surfacing async
failures inline: a component-local `error` state rendered through the shared
`InlineNotice` component (e.g. `App.tsx:2869` /
`<InlineNotice tone="error" title="Refresh failed" detail={error} />` in the
server/resource-list region, and the git-status area's own `error` state
around `App.tsx:6715`/`6813`). There is no centralized toast/notification
system in this codebase — every existing error surface follows this
local-state-plus-`InlineNotice` shape.

## Phases

### Phase 1: Surface terminal-creation failure to the user

Route `createTerminalPane()`'s rejection path through the same
local-error-state + `InlineNotice` pattern used elsewhere (find the most
natural owning surface — e.g. the workbench toolbar/tab-strip area — rather
than inventing a new presentation mechanism). At minimum the user must see
that the action failed; include the server-provided error message when
available.
