---
title: "Audit remote-only latent bugs masked by the server-local fallback pattern"
related:
  260714-bug-linked-terminal-ws-relay-502: origin instance uncovered while fixing this ticket
---

# Audit remote-only latent bugs masked by the server-local fallback pattern

## Background

Linked/remote dashboard servers are meant to be first-tier: from the frontend's
point of view a remote server's resources must behave seamlessly like the local
server's. In practice a class of latent, **remote-only** defects can hide behind
two related habits in the frontend:

1. The `?? "server-local"` (and similar `?? LOCAL_DASHBOARD_SERVER_ROUTE`)
   fallback, used when comparing or defaulting a route. When a `serverRoute`
   is accidentally lost (undefined), the fallback silently substitutes
   `"server-local"`. For a local root this **coincidentally still matches**
   (`root.resourcePath.serverId === "server-local"`), so the bug is invisible;
   for a non-local root the comparison fails and behavior breaks.
2. Client-side `serverRoute` stitching that only *some* fetchers perform. The
   daemon never sends `serverRoute` on the wire; the frontend stitches it in
   after each fetch. Any code path that replaces a session/resource object with
   a raw daemon response **without re-stitching** drops the route.

Because linked-server terminal usage was dogfooded for the first time in this
session, an instance of this class had never been exercised before and surfaced
immediately once a linked ("wsl-daemon") server's terminals were opened.

## Confirmed origin instance (fixed under 260714)

`resizeTerminal` (`ws-dashboard/frontend/src/terminals.ts`) returned the raw
daemon response without stitching `serverRoute`, unlike its siblings
`createTerminal` and `listTerminals`. `forwardTerminalResize`
(`frontend/src/App.tsx`) then replaced a pane's whole `session` object with that
serverRoute-less value. The render filter

```
(pane.session.serverRoute ?? "server-local") === root.resourcePath.serverId
```

subsequently evaluated to `"server-local"`, which matched local roots but not a
`wsl-daemon` root, so linked terminal panes were filtered out of the active
root's editor groups and unmounted one-by-one — collapsing the workbench to an
empty Dockview `dv-watermark`. A resize fires on pane mount (xterm fit), so the
failure was deterministic on any linked-server terminal open. The fix stitches
`serverRoute` inside `resizeTerminal`, matching the other two fetchers.

Runtime confirmation (this session): terminals were delivered correctly and
stably by the relay (`/resources` and `.../terminals` both stable, 5 sessions,
`resourcePath.serverId = wsl-daemon`); the backend was fully exonerated; the
defect was purely the frontend session-replacement clobber. The `setTerminalPanes`
site labeled `resize` was observed mutating existing pane fields with no map-size
change, exactly when panes dropped out.

## Proposed audit sweep

Find and eliminate the remaining members of this class before they surface in
dogfooding:

- **Session/resource-replacement paths.** Audit every place that assigns or
  replaces a session/resource object from a raw daemon response (not just
  terminals: activity, agent-chat, git, and any other per-root sub-fetch).
  Confirm each either spreads the existing object or consumes an
  already-stitched response. The terminal audit found only `resizeTerminal`
  vulnerable among six terminal paths; repeat that audit for the other
  subsystems.
- **`serverRoute` stitching parity across fetchers.** Enumerate all fetchers
  that return objects carrying a `serverRoute`; ensure each stitches it
  consistently (`x.serverRoute ?? serverRoute ?? LOCAL_DASHBOARD_SERVER_ROUTE`).
  The asymmetry (some stitch, one did not) is the root shape.
- **`?? "server-local"` / `?? LOCAL_*` comparison and key-construction sites.**
  Grep for every fallback of this shape used in an equality comparison or a
  pane/state-key construction. For each, ask: "if the left operand were
  undefined, would this coincidentally pass for a local route while breaking
  for a non-local route?" Those sites are latent remote-only bugs even if not
  yet reachable.
- **Consider a structural hardening.** Evaluate whether `serverRoute` should
  live on the pane/resource wrapper (outside the mutable session object) so
  replacement can never clobber it, versus keeping per-fetcher stitching.
  Weigh blast radius; the origin fix chose the minimal per-fetcher stitch.

## Goal

Remote/linked servers behave identically to local from the frontend's
perspective. No behavior should depend on a route defaulting to `"server-local"`
being coincidentally correct.
