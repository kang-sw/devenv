---
title: "Dashboard rail: repair session and branch/worktree drag ordering"
related:
  261113-feat-ws-dashboard-session-rail-tree: "completed rail interaction and persistence contract; preserve its validated session tree and context menu"
  261113-feat-ws-dashboard-iframe-shell: "completed; rail pointer handling runs in the shell document"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 87ca94d4f3aee8fb
sage-review-completeness-reviewed: 87ca94d4f3aee8fb
completed: 2026-11-14
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

## Contract settlement and current evidence

Owner (648), option a, confirmed through MAIN on 2026-11-14: freely reorder all
worktree groups in the same workspace, including the root. Without a saved
order, preserve existing root-first sorting. The owner also suggested a small
root icon: `648=a^루트에는 아이콘 하나 정도만 추가로 표시해주면 어떨까 싶긴 하네요.`
MAIN relayed explicit owner answers on 2026-11-14: (654) `ok`, (655) `ok`,
(656) `a`, and (657) `a`. These settle the additive T0 storage/request contract,
unlisted-group tail, and full-sequence splice recorded below. Existing API/store
semantics remain valid; no breaking change is authorized.

The owner also explicitly added new-session append-at-end behavior in (656):
`656=a^그리고 겸사겸사 지금 새 세션 추가하면 가장 앞에 추가되게 되어 있는데, 이것도 가장 뒤에 새 세션 추가되는 식으로 로직 변경해주세요.`
This covers new sessions as well as new groups, preserving existing grouping,
parent/child validity, and saved manual order; it does not establish a new
child-placement policy.

Earlier grounding found no branch-press admission or persisted group-order
operation. The former policy blocker is settled by (648), but this ticket must
be re-grounded and pass its configured Sage gates before ready promotion. MAIN
remains the owner-facing channel for any remaining policy or required technical
acknowledgment; writer handoff is not a ready stamp.

Runtime reproduction was not part of ticket preparation. The reported session
failure and the index-error hypothesis are not independently reproduced facts.

## Decisions

- **Scope authority: owner (616).** Fix both session-row and branch/worktree-row
  dragging, not session rows alone. Workspace-row dragging remains intact.
- **Group ordering authority: owner (648), option a.** Freely reorder all
  worktree groups within the same workspace, including the root group, and
  persist the chosen sequence. Without a saved sequence, retain existing
  root-first/path sorting. Reject retaining a fixed root position or allowing
  cross-workspace group moves. Incorporate a small root icon into the existing
  appearance; no separate visual redesign or approved mockup was supplied.
- **Compatibility tier: T0 additive.** Add group ordering without changing the
  meaning or validity of existing layout fields and operations. Preserve
  unknown optional fields on read-modify-write; prove compatibility with old
  layout fixtures. Reject schema replacement, reinterpreting existing
  operations, and a T2/minor bump.
- **(654), owner confirmed: additive storage.** Add optional per-workspace
  `worktree_order`, an ordered array of cleaned worktree paths in schema 1,
  preserving unknown fields. Rejected: a separate store or reusing session
  `order`, which introduces lifecycle coupling or changes existing meaning.
- **(655), owner confirmed: additive request.** Add `wtmove` with `workspace`,
  `path`, and exactly one displayed `before` or `after` reference in the same
  workspace. Source/reference must be distinct displayed groups, including
  empty groups. Reject malformed, unknown and cross-workspace addresses without
  mutation. Preserve existing operation meanings, validation and response shape.
  Rejected: overloading `move`/`wsmove`, and no-reference append. End placement
  uses `after` the last other visible group.
- **(656), option a, owner confirmed: newly visible group tail.** With a saved
  sequence, render saved visible groups first and append unlisted visible
  groups using the existing root-first/path fallback. Without a saved sequence,
  retain the existing whole-list fallback. Rejected: forcing a new root first
  or blending unlisted groups into the saved sequence by path.
- **(656), explicit owner addition: new sessions append at the end.** Newly
  admitted ordinary top-level live sessions appear after existing sessions in
  their own group and applicable star section, rather than at the front, both
  without saved order and when merging with saved manual order. Preserve saved
  relative order, group membership, starred-unit precedence, parent/child
  ordering and validity, orphan handling, and ended-row ordering. This changes
  default admission placement, not revival behavior, child-placement policy or
  the meanings/defaults of existing move/attach/detach operations.
  Source evidence, not runtime reproduction: live defaults sort registration
  newest-first, and saved merging independently ranks unlisted ordinary rows
  before saved ones; both paths need coverage. See
  agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L520-L543 and
  agents-plugin-tool/internal/wsdashboard/rail_layout.go#L533-L615.
- **(657), option a, owner confirmed: full-sequence splice.** Apply a visible
  group move to the full saved sequence, retaining absent entries. For saved
  `[A,X,B,C]` with `X` absent, moving `C` before `B` yields `[A,X,C,B]`.
  Rejected: visible-block replacement yielding `[A,C,B,X]`, which shifts absent
  entries relative to unmoved entries. Retention is not a promise of an
  unqualified original return position after deliberate later moves.
- **Technical, auto-confirmed from fact-populator gap: stable identity.**
  Use the workspace's cleaned path plus the group's cleaned worktree path,
  consistent with `.rws[data-ws]` and `.rwt[data-g]`; root uses its existing path,
  not a magic sentinel. Validate against the full displayed worktree inventory,
  including groups without sessions. Preserve saved entries for temporarily
  absent groups, skipping them at render time, so discovery loss is not a
  destructive cleanup. The retention and movement interaction follows the
  explicitly confirmed (654) and (657), not an inferred cleanup policy.
  Rejected: branch labels or session IDs, which are
  mutable or absent for empty groups; a session-derived-only inventory also
  omits displayed empty groups. This is autonomous technical settlement, not
  owner approval of the storage or request contract. Evidence:
  agents-plugin-tool/internal/wsdashboard/rail_layout.go#L437-L469;
  agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl#L354.
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

- Compatibility target is the owner-approved T0 additive dashboard repair.
  No T2/minor bump, deletion, schema replacement, or silent change to existing
  route/op semantics is authorized. Implement only the confirmed additive
  group storage/request representation in (654)-(655); do not
  reinterpret a session `move` or workspace `wsmove` as a worktree-group
  operation.
- Diagnose the reproduced failure before choosing a repair; do not encode the
  owner's index-error hypothesis as established source truth.
- No approved mockup is part of (616) or (648). Preserve existing rail
  appearance and drag/menu affordances apart from the suggested small root icon.
- Candidate scope is `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
  and `assets/rail.js`, with existing rail script/layout tests and
  `scripts/rail-headless.mjs` (the harness is root-local, not under
  `agents-plugin-tool/`; `scripts/rail-headless.mjs#L1-L34`). `pageview_rail.go`
  and `rail_layout.go` also cover the confirmed default-session ordering and
  additive persisted-group ordering. Change `rail.go` only if reproduction
  proves a missing admission/markup path; do not broaden admission membership.
  Current `rdStart` admits only
  session `.drgrip` and workspace `.wsbar` presses, not branch `.wthd`
  presses (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L6860-L6876`).
  Worktree groups currently sort root-first, then by path; layout ordering
  changes sessions within those groups and workspace sections, not the group
  sequence (`agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L508-L515`,
  `agents-plugin-tool/internal/wsdashboard/rail_layout.go#L995-L1022`).
  The stored fields and seven validated ops contain no worktree-group order
  contract, so persisted branch/worktree reordering requires the owner-approved
  additive store/request expansion, not an assets-only bug fix
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

- 261113-feat-ws-dashboard-session-rail-tree (2026-11-14, commit b710d81c): "Rejected the reviewer's suggested \"settle every orphan in the group\": it would drop dormant links of orphans the user never touched, against Decision 7 (re-nest on revive)." — bearing: constrains
- 261113-feat-ws-dashboard-iframe-shell (2026-11-14, commit 0586a428): "in shell mode the rail lives in the shell document" — bearing: constrains
- 068e3715 (2026-11-14, commit): "dashboard.js: kept both the overview hold check and the new railGate.forget() from develop's session rail tree merge." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: The ticket names agents-plugin-tool/internal/wsdashboard/assets/dashboard.js, assets/rail.js, assets/dashboard.tmpl, rail_layout.go, rail_layout_route.go, pageview_rail.go, railscript_test.go, rail_layout_test.go and scripts/rail-headless.mjs. New-session append affects both base live sorting and saved-order merging; agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L520-L532; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L582-L615 |
| scope.surface | public-interface | VERIFIED: POST /rail/layout and rail-layout.json need additive group ordering while existing meanings remain valid; agents-plugin-tool/internal/wsdashboard/rail_layout_route.go#L15-L28; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L104-L119 |
| scope.new_public_symbol | yes | VERIFIED: Owner explicitly approved storage/op items (654)-(655); stable workspace-qualified path identity is an autonomous technical resolution. Current request and seven-op dispatcher have no group-order interface, agents-plugin-tool/internal/wsdashboard/rail_layout.go#L475-L487 and #L689-L729 |
| scope.new_type_contract | yes | VERIFIED: Persisted group sequence needs an additive contract distinct from session order/children/stars and workspace_order; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L16-L35 and #L104-L119 |
| scope.test_surface | existing | VERIFIED: Existing ordering expectations put missing rows before saved rows; agents-plugin-tool/internal/wsdashboard/rail_layout_test.go#L449-L495. Existing move, workspace, store and helper tests remain relevant; rail_layout_test.go#L179-L207, #L295-L315 and #L598-L651; railscript_test.go#L236-L285 under the same directory. Real-pointer cases exist in scripts/rail-headless.mjs#L562-L614 and #L1007-L1116; sequential admission needs added coverage in both plain-folder and worktree groups |
| complexity.reuse_points | confirmed | VERIFIED: Read sortRailRows, layoutScope.rank/seq and orderRows; agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L520-L543; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L582-L615 and #L1091-L1118. Existing WsRail.dropZone/dropNoop/moveRow/swapGate and rdDrop/rdSave remain reuse points; assets/rail.js#L275-L377 and #L459-L476; assets/dashboard.js#L6935-L6959 under the same package |
| complexity.side_effect_risk | high | INFERRED: New-session admission changes shared ordering even without a drag. rank/seq also feed orphan settlement and detach placement, so a blanket rank change risks preserved behavior; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L582-L615, #L660-L668 and #L819-L863. Pointer/swap/store interactions remain; assets/dashboard.js#L6760-L6963 and rail_layout.go#L382-L419 under the same package |
| risk.correctness | high | INFERRED: Append must cover unsaved and saved order while preserving star sections, child units, orphan ranks, ended order and old-op defaults. Changing timestamp direction alone leaves ordinary missing rows at rank -1 ahead of saved rows; agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L520-L543; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L582-L615, #L819-L863 and #L1091-L1118 |
| risk.fit | high | INFERRED: The settled scope combines drag repair, additive worktree_order/wtmove and new-session append in two independent ordering layers. Plain-folder and repository groups both use base sorting before layout ordering; agents-plugin-tool/internal/wsdashboard/pageview_rail.go#L386-L403 and #L495-L515; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L995-L1022. Keep changes scoped rather than rewriting the 9085-line dashboard.js or 1118-line rail_layout.go |
| risk.test | high | INFERRED: Existing synthetic ordering and real-pointer tests do not establish sequential admission through base sorting plus saved merging. Extend unsaved/saved, plain-folder/worktree, star-section and parent-unit cases across swaps/reload, retaining orphan/ended/old-op regressions; agents-plugin-tool/internal/wsdashboard/rail_layout_test.go#L425-L495; scripts/rail-headless.mjs#L562-L614 and #L1007-L1116. No tests or browser reproduction were run during this grounding |
| risk.security_or_contract | moderate | VERIFIED: MAIN approved additive T0; owner explicitly approved storage/op items (654)-(655) and placement policies (656)-(657); stable identity was autonomously settled, not presented as owner approval. Unknown-field preservation already exists and must remain; agents-plugin-tool/internal/wsdashboard/rail_layout.go#L104-L212; rail_layout_test.go#L598-L627 under the same directory. The existing route checks sameOrigin; rail_layout_route.go#L68-L79 under the same directory |

## Phases

### Phase 1: Repair rail drag ordering with persisted regression coverage

Reproduce the owner's session and branch/worktree failures using the real
pointer/drag path. Establish whether the failure is pointer admission, row-unit
placement, index adjustment, persistence, or a combination; do not replace an
unproven cause with a blanket reorder rewrite. Restore the approved behavior
within the existing session validity rules and the owner-approved group-order
contract and stable-identity decision. Implement confirmed (654)-(657), including
new-session append-at-end admission, without broadening existing grouping or
child-placement rules. Configured Sage review precedes implementation.

Verification:

- A lower session row moves upward and an upper row moves downward through the
  grip, including adjacent and non-adjacent placements next to expanded parents.
- Parents move with their children in correct order. Valid child moves across
  parents preserve the depth-one tree; invalid nesting remains refused.
- Sequential new ordinary top-level live sessions append in their own group
  and applicable star section with and without saved manual order, in both
  repository/worktree and plain-folder groups. Existing manually arranged
  sessions and parent units with children keep their relative order; successive
  newcomers append in admission order. Verify admission, rail swaps and reload.
  Preserve starred-unit precedence, child/orphan/ended ordering and old-operation
  regressions; do not add a child-placement or revival policy. Extend existing
  ordering expectations in `rail_layout_test.go` (including current #L449-L495).
- Branch/worktree rows, including the root group, can be freely reordered
  within their workspace as complete `.rwt` DOM subtrees, without moving
  unrelated workspace sections or losing sessions/children. Group-aware swap
  recovery, optimistic undo and generation protection must preserve the unit.
  Cross-workspace group moves remain refused. Verify the
  small root icon and preserve existing root-first/path fallback when no saved
  order exists.
- Old layout fixtures and existing layout operations remain valid. Group-only
  workspace state and temporarily absent group entries survive pruning; retained
  group entries count as non-empty for the existing import gate. Visible no-op
  moves must not incidentally relocate absent entries. Optional unknown fields
  survive read-modify-write, and saved group order survives
  subsequent session/tree/star/workspace changes and store reload. Verify root,
  empty groups, duplicate/malformed/unknown/cross-workspace addresses, saved
  missing-group retention, and the settled newly visible group policy. Trace
  (654) and (657) through group-only workspace pruning, import non-emptiness,
  absent-slot movement and no-op tests; preserve all existing operation meanings.
  On a first move, materialize the effective fallback sequence so unaffected
  visible relationships remain stable. Later moves combine retained saved
  entries with (656)'s tail before applying (657); render-only skipping must
  not mutate saved state. Exercise retention through every existing operation.
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

### Result (ebc2555e8) - 2026-11-14

- Reproduced the failures before choosing the repair. A dragged parent landing
  after an expanded parent was incorrectly redirected before its first child,
  then refused as too deep; this was not a general index-adjustment defect.
  Worktree headers did not admit a drag or hold swaps. New sessions sorted
  newest-first, while saved merging independently put unlisted ordinary rows
  ahead of saved rows. Baseline evidence: `d73e64281` and
  `/tmp/rail-drag-baseline-{first,fixture-check,preservation}.log`.
- `e480c257a` repairs parent-unit placement without changing the existing drop
  geometry, and adds schema-1 optional `worktree_order` plus workspace-qualified
  `wtmove`. Root and empty groups move as whole subtrees with optimistic undo,
  swap holding/refinding and generation protection. Saved absent paths survive
  pruning and every existing operation; group-only state blocks legacy import.
  Visible no-ops retain absent slots, and actual moves splice the full saved
  sequence after appending unlisted groups in fallback order. Unknown fields
  and old layout fixtures remain valid.
- `c5abd306f` makes base registration sorting oldest-first and gives only
  ordinary unlisted live rows a tail rank when merging saved order. Parent
  units, star sections, orphan inherited/head ranks, ended ordering, revival,
  and existing operation defaults remain covered. Newcomers append in their
  own group, without changing membership or child placement.
- Technical decisions: keep group-specific ordering helpers/tests in adjacent
  `rail_layout_worktrees.go` / `rail_layout_worktrees_test.go` rather than
  expanding the session-layout core further; target group headers by midpoint
  while moving their entire subtrees; use the existing repository icon as a
  small accessible `.wtroot` marker without changing rail indentation.
- Verification: focused Go layout/Node-backed rail tests passed; full
  `go test ./internal/wsdashboard -count=1 -timeout=10m` passed (129.644s),
  full output `/tmp/rail-admission-648/full.log`. Final source-only
  `go build -o /tmp/rail-drag-final-ws-mcp ./cmd/ws-mcp`, Node syntax checks
  for both assets and the harness, and `git diff --check` passed. Windows test
  binary cross-compilation passed; Windows execution was not performed.
- All 34 isolated Chromium scenarios passed, including adjacent/non-adjacent
  parent up/down, across-parent children, worktree root/empty up/down, refused
  requests and undo, stale generations, sequential admissions with/without
  manual order in plain/worktree groups, workspace up/down persistence, menus,
  navigation and emulated touch. Full output: `/tmp/rail-drag-core-full.log`.
  Root captures: `/tmp/rail-drag-core-shots/worktree-up-root-marker.png` and
  `/tmp/rail-drag-core-shots/worktree-down-root-marker.png`; the latter was
  inspected for the visible root marker and intact parent/child units. This
  ticket supplies no approved visual reference, so no reference comparison
  applies.
- Earlier failures were classified before correction: the fixture needed to
  await the actual 30-second registry poll; Go test authoring had an incorrect
  field access, an ended-display assumption, and intentional old ordering
  expectations. Tests were corrected against source and the approved contract,
  not weakened to accommodate broken behavior.
- Independent one-pass lite review was clean over `969c98158..ebc2555e8`:
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@9a21da5a/review-paths/cd06e270-01-rail-drag-lite.md`.
  No unresolved findings. Headed-browser, physical-touch and Windows-runtime
  verification were omitted; no live/shared daemon, launcher, release or push
  was used. Implementation branch is retained for lead-owned integration.
