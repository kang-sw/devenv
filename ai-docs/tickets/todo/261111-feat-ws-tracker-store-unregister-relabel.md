---
title: "Tracker: unregister a store from this machine (deleting its local clone), hide it, and change its label"
parent: 261107-epic-ws-personal-common-store
---

# Tracker: unregister, hide, and relabel a store on this machine

## Background

The user asked (2026-11-11, whiteboard) whether a tracker store can be
removed. It cannot. `wstrackerhost.Host` exposes `Register` and no inverse.
Neither the dashboard's tracker routes (`TrackerHost` in
`agents-plugin-tool/internal/wsdashboard/tracker_routes.go`) nor the MCP
tracker commands (`tk:register` in
`agents-plugin-tool/internal/mcp/tracker_tools.go`) can unregister a store,
delete its directory, or change its label. The label (`StoreEntry.Label` in
`wstrackerhost/registry.go`) is set only at registration.

Today a store can only be removed by hand: drop its entry from
`<root>/stores.json` and delete `<root>/<id>/`.

## Decisions

1. **Unregister removes the store from this machine only, deleting its
   local clone. The remote is kept, and a store with unpushed own events is
   refused. Relabel is included.** The user chose (150) b: "(150) b".
   - Rejected: unregister that keeps the local clone (150 a). It is
     reversible, but it leaves directories to clean up by hand, and the
     clone can be fetched again from the remote.
   - Rejected: no feature for now (150 c).

2. **Parent and placement.** The ticket is a child of
   `261107-epic-ws-personal-common-store`, which owns store registration
   (the join dialog came from `261108-feat-ws-tracker-drawer-ui` under it).
   Unregister is the inverse of registration. (151) ok.
3. **Unregister is local only.** It writes no event to the store and
   leaves this member's join and key in the remote. Re-registering under the
   same id and member therefore finishes without a new join: per
   `RemoteHeldError`, this machine's key is already that member. This
   follows from (150) b. (152) ok.
4. **Both surfaces.** MCP gets the commands `tk:unregister store="..."` and
   `tk:relabel store="..." label="..."`, siblings of `tk:register` in the
   same grammar. The dashboard gets routes and tracker card menu entries.
   (153) ok.
   - Rejected: dashboard only. The epic is agent-first, and `tk:register`
     already sets the precedent of a store-level command an agent may send.
5. **Unpushed guard.** For a store with a remote, unregister refuses while
   `wsledger.Ledger.Pending()` reports any own event not yet pushed
   (`Count > 0`), or while a check or sync is in flight. The refusal says to
   sync first. (154) ok.
   - Rejected: a force flag that discards unpushed events. (150) b chose
     refusal as the guard, and a discard path is a separate data-loss
     decision nobody asked for.
6. **Cross-process safety.** (155) ok.
   - Unregister takes the store's `write.lock`, then removes the registry
     entry under the root lock.
   - It then renames `<root>/<id>` to a hidden `<root>/.removed-<id>-<unix>`
     and deletes that tree.
   - Every `Host` re-checks the registry entry before reusing a cached open
     store. When the entry is gone, it closes the store's ledger, stops its
     idle push timer, and answers "no registered store".
   - The daemon's watch and poll skip a missing entry.
   - A leftover `.removed-*` directory is ignored, and is swept on the next
     unregister or Host start.
   - Rejected: deleting the directory in place. `Host` keeps every store it
     opened open until `Close` (doc.go), so a sibling process's idle push or
     write would recreate files inside a half-deleted directory.
7. **Relabel is machine-local.** It rewrites only the registry entry's
   `label`, under the root lock, and never writes a ledger event. An empty
   label clears it back to the id. The dashboard re-renders through the
   existing tracker-changed path. (156) ok.
   - Rejected: a store-wide name written to the ledger. The label is
     already a machine-local registry field (`StoreEntry.Label`), and a
     shared name would add a new event type to the store protocol.
8. **A store with no remote** (`Remote == ""`) holds its only copy locally.
   It may be unregistered only behind an explicit confirmation that types the
   store id. In the dashboard dialog this is a text field; on
   `tk:unregister` it is a `confirm="<id>"` attribute. The confirmation
   text states that the data is lost permanently. The unpushed guard
   (Decision 5) does not apply to such a store. (157) b.
   - Rejected: refusing to unregister a store with no remote (157 a). That
     brings back the manual cleanup this ticket removes.
   - Rejected: moving the directory aside instead of deleting it (157 c).

9. **Hide.** Hiding a store does not delete anything. (158) a.
   - It sets a machine-local `hidden` flag on the registry entry, under the
     root lock, and writes no ledger event.
   - The dashboard card strip omits a hidden store. An open card of a store
     that becomes hidden closes. The tracker card menu's register area lists
     hidden stores, each with a "show" action and the unregister action.
   - MCP gets `tk:hide store="..."` and `tk:unhide store="..."`. MCP reads
     and writes of a hidden store keep working, and poll and watch continue.
   - The user suggested this in a note on (157): "삭제를 하지 않는
     \"숨기기\" 옵션도 하나 두는 것도 좋을지도요 ..?"
   - Rejected: a dormant hide that also stops poll and watch and refuses MCP
     reads (158 b). It adds a second store state to every read path, while
     the poll cost of a few hidden stores is negligible.
   - Rejected: no hide in this ticket (158 c).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (the dashboard
  assets, the MCP command grammar, and its manual text ship downstream);
  `ai-docs/manuals/ws-mcp.md` for the MCP tool surface.
- Every tracker card or drawer state that names the removed store (an open
  card, the store picker, badges in `ui-seen.json`) must drop it without an
  error toast loop. `ui-seen.json` goes away with the directory.
- The adjacent idea `261108-bug-tracker-reregister-second-member-same-machine-key`
  stays out of scope: re-registering under a different member id from the
  same machine key is not made to work here.
- Heavy Go build and test runs go through the remote runner, never locally.

## Phases

### Phase 1: unregister, hide, and relabel

- `wstrackerhost`: `Host.Unregister(ctx, id)`, `Host.Relabel(id, label)`,
  and `Host.SetHidden(id, hidden)` per Decisions; `StoreEntry` gains a
  `hidden` field (omitted when false); the registry-entry re-check on cached stores; watch and
  poll skipping missing entries; sweeping `.removed-*` leftovers.
- MCP: `tk:unregister`, `tk:relabel`, `tk:hide`, and `tk:unhide` in the
  tracker grammar, with their manual lines next to `tk:register`.
- Dashboard: `TrackerHost` methods, routes, and tracker card menu entries
  (unregister behind a confirmation dialog that names the store and says
  the remote is kept, or the typed-id confirmation for a store with no
  remote; relabel as an inline edit or small dialog; hide, and the hidden
  list with show and unregister in the register area), using the existing
  `--wb-*` quiet styling.
- Verification:
  - `wstrackerhost` tests for: unregister of a synced remote store
    (registry entry and directory gone, remote untouched, re-register
    restores it); refusal with unpushed events; a store with no remote
    refused without the matching confirm and removed with it; a second `Host` holding the
    store open sees "no registered store" and leaves no files behind; and
    relabel round-trip and clear; hide and unhide round-trip, with MCP
    reads of a hidden store still answering.
  - MCP command tests for both commands and their refusals.
  - Dashboard route tests.
  - Remote run:
    `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wstrackerhost/ ./internal/mcp/ ./internal/wsdashboard/`.
  - `node --check` on edited JS.
