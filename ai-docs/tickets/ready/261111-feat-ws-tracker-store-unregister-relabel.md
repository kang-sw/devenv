---
title: "Tracker: unregister a store from this machine (deleting its local clone), hide it, and change its label"
parent: 261107-epic-ws-personal-common-store
related:
  261111-refactor-ws-minimize-recurring-fs-touches: "overlaps wstrackerhost/watch.go (watch interval and registry re-read); not a prerequisite, interactions are checked at merge"
  261110-bug-ws-tracker-store-lock-network-split: "would split the per-store mu that Decision 5's in-flight check guard reads; not a prerequisite, whichever lands second adapts"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 904116e347c56364
sage-review-design-reviewed: 904116e347c56364
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
   (`Count > 0`), while `WikiList` reports any wiki with `Unpushed` set
   (a wiki branch is a separate native-git ref that `Pending` does not
   count), or while a check or sync is in flight. (154) ok.
   - The check runs after unregister holds the locks of Decision 6 and has
     reloaded the ledger (`Ledger.Reload`), so events a sibling process just
     appended to the shared clone are counted.
   - The refusal names the state. When only a sync can clear it, it says to
     sync first. When `PendingReport` shows `IDTaken`, `KeyNotRegistered`,
     or `ForeignAdvance`, no sync clears it, so the refusal names that state
     and its existing remedy (for example `MoveToFreshClone` for
     `ForeignAdvance`, or an admin adding the key for `KeyNotRegistered`).
     Such a store still needs manual cleanup; removing it is out of scope.
   - Rejected: a force flag that discards unpushed events. (150) b chose
     refusal as the guard, and a discard path is a separate data-loss
     decision nobody asked for.
6. **Cross-process safety.** (155) ok.
   - Unregister takes the store's `write.lock` and also wsledger's own file
     lock `<dir>/lock` (the one `Ledger.Sync`, `Fetch`, `Reload`, and the
     idle push take), so no sibling process's dispatch, fetch, or push runs
     during the removal. When either lock is held elsewhere, unregister
     refuses as Decision 5 does for an in-flight sync rather than waiting.
     It then removes the registry entry under the root lock.
   - It then renames `<root>/<id>` to a hidden `<root>/.removed-<id>-<unix>`
     and deletes that tree.
   - Nothing recreates a removed store directory. `writeJSON` creates only
     a missing root, never a missing store directory, so a sibling's late
     `host-sync.json` write after a failed sync fails instead of leaving a
     stub. `MarkSeen` holds `seenMu` and re-checks the entry under it, and
     unregister takes `seenMu` around the rename, so a late `ui-seen.json`
     write cannot land in a recreated directory. A stub left behind would
     make `Register` refuse with "already exists".
   - Every `Host` re-checks the registry entry before reusing a cached open
     store. The check compares the registration's identity, not only the
     entry's presence: `StoreEntry` gains a per-registration nonce written
     by `Register`. When the entry is gone or its nonce differs (an
     unregister followed by a re-register under the same id, whose fresh
     clone has a new clone id), the Host closes the stale ledger, stops its
     idle push timer, and either answers "no registered store" or reopens
     the new registration.
   - The daemon's watch and poll skip a missing entry.
   - A leftover `.removed-*` directory is ignored, and is swept on the next
     unregister or Host start.
   - Rejected: deleting the directory in place. `Host` keeps every store it
     opened open until `Close` (doc.go), so a sibling process's idle push or
     write would recreate files inside a half-deleted directory.
7. **Relabel is machine-local.** It rewrites only the registry entry's
   `label`, under the root lock, and never writes a ledger event. An empty
   label clears it back to the id. The label is trimmed; one holding a
   control character (such as a newline) or longer than 80 characters is
   refused. The dashboard re-renders through the existing tracker-changed
   path. A relabel or hide sent through MCP or another process reaches an
   open dashboard only through `StartWatch`, whose stamp today hashes only
   journals and refs, so the watch stamp also covers each entry's label and
   hidden flag. The registry is already read every tick, so this adds no
   file access. (156) ok.
   - Rejected: a store-wide name written to the ledger. The label is
     already a machine-local registry field (`StoreEntry.Label`), and a
     shared name would add a new event type to the store protocol.
8. **A store with no remote** (`Remote == ""`) holds its only copy locally.
   It may be unregistered only behind an explicit confirmation that types the
   store id. In the dashboard dialog this is a text field; on
   `tk:unregister` it is a `confirm="<id>"` attribute. The confirmation
   text states that the data is lost permanently. The unpushed guard
   (Decision 5) does not apply to such a store. On a store with a remote,
   a `confirm` attribute is ignored: the unpushed guard governs it.
   (157) b.
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
- The store's support-agent conversation runs with the clone as its working
  directory (`mcp/tracker_support.go` matches it by `canonicalDir(e.Dir)`).
  Unregister ends that conversation and drops its card, so no agent keeps
  running in a deleted directory.
- The adjacent idea `261108-bug-tracker-reregister-second-member-same-machine-key`
  stays out of scope: re-registering under a different member id from the
  same machine key is not made to work here.
- Heavy Go build and test runs go through the remote runner, never locally.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261107-epic-ws-personal-common-store (2026-11-11, commit b2398a0c): "Cross-process deletion renames the directory before deleting it, and every Host re-checks the registry entry. Reason: each Host keeps opened stores open until Close." — bearing: supports
- 261108-feat-ws-tracker-caller (2026-11-08, Decisions): "Which stores a session shows ((407) a): all of the user's registered stores, in every session. Rejected: ... a per-project hide list (can be added later as one key)." — bearing: supports
- 261108-feat-ws-tracker-caller (2026-11-08, Result dcd9d44d8): "The registry is `stores.json` ({version, stores: [{id, label, remote, ...}]}); `keys` is a reserved store id; root follows $WS_CONFIG_HOME (default ~/.ws/tracker)." — bearing: constrains
- 715363ea (2026-11-11, commit): "the tracker store registry has an optional display label (wstrackerhost StoreEntry.Label, DisplayLabel(), view.go StoreInfo{Name, Label} in the store view JSON)" — bearing: supports
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Store watch: Host.StartWatch(interval, onChange) re-reads the registry each tick and stamps each store's journals ... and refs" — bearing: constrains
- 261110-feat-ws-dashboard-toast-framework-and-tab-label-alerts (2026-11-10, Result): "Tracker API: opts.onRender(u), api.unread() ({store, ready, entries[...]}), api.openNotice(entry), api.openNotify(store)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstrackerhost/host.go, registry.go, watch.go, poll.go; internal/mcp/tracker_tools.go; internal/wsdashboard/tracker_routes.go, assets/tracker.js; agents-plugin/rsrc/tracker/tracker.md plus wsflow and pi mirrors |
| scope.surface | cross-module | wstrackerhost.Host, StoreEntry, wsdashboard TrackerHost interface, MCP tk grammar, shipped tracker.md |
| scope.new_public_symbol | yes | Host.Unregister, Host.Relabel, Host.SetHidden; tk:unregister, tk:relabel, tk:hide, tk:unhide |
| scope.new_type_contract | yes | StoreEntry gains hidden json field; TrackerHost interface gains methods; new dashboard routes |
| scope.test_surface | existing | wstrackerhost/host_test.go, watch_test.go; mcp/tracker_tools_test.go, tracker_commands_test.go; wsdashboard/tracker_routes_test.go |
| complexity.reuse_points | confirmed | withRootLock registry.go#L130, withWriteLock attachments.go#L272, wsledger Ledger.Pending store.go#L1151, StartWatch registry re-read watch.go#L30 |
| complexity.side_effect_risk | high | deletes a store directory on disk while sibling processes may hold it open |
| risk.correctness | high | cross-process unregister race with cached open stores, idle push timers, and the in-flight check guard |
| risk.fit | moderate | touches host, poll, watch, MCP grammar, dashboard routes and card menu UI together |
| risk.test | moderate | multi-Host cross-process and remote-untouched scenarios need careful fixtures |
| risk.security_or_contract | high | irreversible local data deletion, typed-id confirm for no-remote stores, new shipped command grammar |

## Phases

### Phase 1: unregister, hide, and relabel

- `wstrackerhost`: `Host.Unregister(ctx, id)`, `Host.Relabel(id, label)`,
  and `Host.SetHidden(id, hidden)` per Decisions; `StoreEntry` gains a
  `hidden` field (omitted when false) and a per-registration nonce; the
  registry-entry identity re-check on cached stores; the second lock and
  the no-recreate rules of Decision 6; the label and hidden flag in the
  watch stamp; watch and poll skipping missing entries; sweeping
  `.removed-*` leftovers.
- MCP: `tk:unregister`, `tk:relabel`, `tk:hide`, and `tk:unhide` in the
  tracker grammar, with their manual lines next to `tk:register` in
  `agents-plugin/rsrc/tracker/tracker.md` and its mirrors in
  `agents-plugin-wsflow/` and `agents-plugin-pi/` (per
  `ai-docs/manuals/wsflow-mirroring.md`).
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
    store open sees "no registered store" and leaves no files behind; a
    second `Host` holding the store while unregister and re-register run
    elsewhere reopens the new registration cleanly; a sibling's late sync
    status write or `MarkSeen` after the rename leaves no directory behind;
    refusal with an unpushed wiki; refusal while `<dir>/lock` is held; a
    stuck store (`ForeignAdvance` or `KeyNotRegistered`) refused with its
    state named; relabel round-trip, clear, and an invalid label refused;
    hide and unhide round-trip, with MCP reads of a hidden store still
    answering; a watch test where a label or hidden change alone fires
    `onChange`.
  - MCP command tests for all four commands and their refusals, including
    `confirm` ignored on a store with a remote.
  - Dashboard route tests: a hidden store is omitted from the card strip
    list; an open card closes when its store is hidden or removed; the
    hidden list offers show and unregister; a removed store produces no
    error toast loop; the support chat card of a removed store is dropped.
  - Remote run:
    `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wstrackerhost/ ./internal/mcp/ ./internal/wsdashboard/`.
  - `node --check` on edited JS.
