---
title: "Dashboard rail: repair session and branch/worktree drag ordering"
related:
  261113-feat-ws-dashboard-session-rail-tree: "completed rail interaction and persistence contract; preserve its validated session tree and context menu"
  261113-feat-ws-dashboard-iframe-shell: "completed; rail pointer handling runs in the shell document"
---

# Dashboard rail: repair session and branch/worktree drag ordering

## Background

Owner (616), confirmed as `ok` and relayed by MAIN, reports that session rows
and branch/worktree rows cannot be dragged reliably; workspace rows already
work. Moving a lower row upward does not take. An index-adjustment error near
child rows is the owner's hypothesis, not a verified root cause.

The accepted correction covers session and branch/worktree rows, with
upward, downward and across-parent regression coverage where those moves are
valid under the existing session-tree rules. It preserves right-click behavior
and the working workspace drag path. This is a new regression ticket, not a
rewrite of the completed session-rail-tree phases.

## Blocked — branch/worktree persistence contract

Fact grounding confirms that the approved branch/worktree-row outcome cannot be
represented by the existing layout contract: groups sort root-first/path,
`rail-layout.json` stores only session/tree/star and workspace ordering, and
`POST /rail/layout` exposes no group-order operation. Branch presses also lack
an admission path. Ready promotion is held; the accepted scope remains both
session and branch/worktree rows.

Owner-facing settlement belongs to MAIN. The missing contract must cover group
identity, allowed destinations and root positioning, additive storage and
request representation, and compatibility treatment. No option is selected
here, and no new operation, schema field, root-position rule, or owner decision
ID is authorized by this ticket. After that settlement, update the contract,
re-ground the changed premises, and run the configured Sage gate before ready.

Runtime reproduction was not part of ticket preparation. The reported session
failure and the index-error hypothesis are not independently reproduced facts.

## Decisions

- **Scope authority: owner (616).** Fix both session-row and branch/worktree-row
  dragging, not session rows alone. Workspace-row dragging remains intact.
- **Preserve existing drop geometry.** A valid session parent has top 25% / middle
  50% / bottom 25% zones; other session targets split 50/50. The handoff's casual
  "thirds" wording does not supersede Decision 9 of the completed rail-tree
  ticket. Reject changing thresholds as a presumed bug fix.
- **Preserve established tree validity and menu behavior.** Keep depth one,
  same-workspace attach rules, top-level worktree grouping, stars, ended-row
  restrictions, and the existing right-click/long-press/keyboard context-menu
  paths. A parent is moved with its children as one unit, not flattened.
- **Prove persistence, not just an optimistic DOM move.** The chosen order must
  survive a rail swap and page reload. Include up, down and across-parent moves
  with children, checking only valid moves and retaining existing refusals.

## Constraints

- Compatibility target is T0 dashboard repair. No T2/minor bump, deletion,
  schema replacement, or silent change to existing route/op semantics is
  authorized. If branch/worktree persistence requires an additive store or
  request contract not already present, identify that exact gap and settle its
  compatibility and API authority before implementation; do not reinterpret a
  session `move` or workspace `wsmove` as a worktree-group operation.
- Diagnose the reproduced failure before choosing a repair; do not encode the
  owner's index-error hypothesis as established source truth.
- No new visual design or approved mockup is part of (616). Preserve the existing
  rail appearance and drag/menu affordances.
- Candidate scope is `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
  and `assets/rail.js`, with existing rail script/layout tests and
  `scripts/rail-headless.mjs` (the harness is root-local, not under
  `agents-plugin-tool/`; `scripts/rail-headless.mjs#L1-L34`). Change `rail.go` or
  `pageview_rail.go` only if reproduction proves a missing handle/markup path;
  change `rail_layout.go` only after proving missing persistence support and
  settling any new cross-version contract. Current `rdStart` admits only
  session `.drgrip` and workspace `.wsbar` presses, not branch `.wthd`
  presses (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L6865-L6883`).
  Worktree groups currently sort root-first, then by path; layout ordering
  changes sessions within those groups and workspace sections, not the group
  sequence (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L508-L515`,
  `agents-plugin-tool/internal/wsdashboard/rail_layout.go#L995-L1022`).
  The stored fields and seven validated ops contain no worktree-group order
  contract, so persisted branch/worktree reordering requires an additive
  store/request contract decision, not an assets-only bug fix
  (`agents-plugin-tool/internal/wsdashboard/rail_layout.go#L16-L35`,
  `agents-plugin-tool/internal/wsdashboard/rail_layout.go#L104-L119`,
  `agents-plugin-tool/internal/wsdashboard/rail_layout_route.go#L15-L28`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Browser verification uses isolated temporary HOME/cache/config and a temporary
  server, following the current rail harness. Do not invoke `scripts/dev.py rerun`,
  installed/source development launchers, or the live shared daemon/snapshot.
  Source builds and isolated tests are permitted; live activation, release and
  push are outside this ticket's implementation authorization.

## Prior Decisions

- 261113-feat-ws-dashboard-session-rail-tree (2026-11-14, commit 066d31ea): "Both read dropZone for every place they involve, so the keyboard and the menu refuse with the same codes and in the same check order as the drag and the route" — bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-14, commit 0586a428): "in shell mode the rail lives in the shell document" — bearing: constrains
- 068e3715 (2026-11-14, commit): "dashboard.js: kept both the overview hold check and the new railGate.forget() from develop's session rail tree merge." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, assets/rail.js, railscript_test.go, rail_layout_test.go; scripts/rail-headless.mjs; persistence expansion requires a settled contract |
| scope.surface | public-interface | Persisted worktree-group ordering is absent from POST /rail/layout and rail-layout.json; rail_layout_route.go#L15-L28 and rail_layout.go#L104-L119 under agents-plugin-tool/internal/wsdashboard/ |
| scope.new_public_symbol | unknown | No group-order op or field has been authorized; naming the additive interface is a decision gap |
| scope.new_type_contract | yes | Required persisted worktree-group sequence cannot be represented by current session order/children/stars or workspace_order; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L16-L35 |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/railscript_test.go#L236-L285, rail_layout_test.go (TestRailLayoutMove, TestRailLayoutWorkspaceMove, TestRailLayoutStoreRoundTrip); scripts/rail-headless.mjs |
| complexity.reuse_points | confirmed | WsRail.dropZone, dropNoop, moveRow and swapGate read in agents-plugin-tool/internal/wsdashboard/assets/rail.js#L275-L344 and #L459-L476; rdDrop/rdSave reuse layoutSave in assets/dashboard.js#L6937-L6963 |
| complexity.side_effect_risk | high | Pointer admission, parent/child DOM units, swap holding and shared persistent ordering interact; assets/dashboard.js#L6770-L6963 under agents-plugin-tool/internal/wsdashboard/ |
| risk.correctness | high | Up/down placement must preserve depth-one validity, stars and child units across swaps and reload; agents-plugin-tool/internal/wsdashboard/assets/rail.js#L275-L344 |
| risk.fit | high | Requested branch-row drag has no admission or persistence path; existing root-first/path ordering must be reconciled without silently selecting new semantics |
| risk.test | high | Existing pure-helper and layout tests cannot establish real-pointer regressions; new bidirectional and reload browser cases remain unexecuted |
| risk.security_or_contract | moderate | Additive shared-store/request contract requires explicit approval; existing route is same-origin gated, agents-plugin-tool/internal/wsdashboard/rail_layout_route.go#L68-L79 |

## Phases

### Phase 1: Repair rail drag ordering with persisted regression coverage

Reproduce the owner's session and branch/worktree failures using the real
pointer/drag path. Establish whether the failure is pointer admission, row-unit
placement, index adjustment, persistence, or a combination; do not replace an
unproven cause with a blanket reorder rewrite. Restore the approved behavior
within the existing validity rules. Resolve any missing branch/worktree
persistence contract under Constraints before coding that expanded surface.

Verification:

- A lower session row moves upward and an upper row moves downward through the
  grip, including adjacent and non-adjacent placements next to expanded parents.
- Parents move with their children in correct order. Valid child moves across
  parents preserve the depth-one tree; invalid nesting remains refused.
- Branch/worktree rows can be dragged to their valid positions without moving
  unrelated workspace sections or losing their sessions/children.
- Assert immediate placement, rail-swap stability, and persisted/reloaded order
  for each repaired row kind, not only request generation or visual drop lines.
- Existing workspace up/down dragging still persists and reloads correctly.
- Right-click menus, row-body navigation, touch grip behavior and existing
  swap-during-press protection remain intact.
- Extend `railscript_test.go`, `rail_layout_test.go` and
  `scripts/rail-headless.mjs` where each layer needs coverage; use real-pointer
  headless cases for the user-visible regression. Run the focused Go/Node tests
  and the rail harness under isolated temporary runtime paths, recording actual
  failures separately from unavailable verification.
