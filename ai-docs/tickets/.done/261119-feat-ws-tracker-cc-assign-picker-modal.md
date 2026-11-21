---
title: tracker 참조·배정 체크박스 모달 (조직도 트리 선택)
related:
  261119-feat-ws-tracker-org-chart-editor: sibling; shares the org-tree helper and "미배치" rule (Decision 11); no ordering for Phase 1
  261119-feat-ws-tracker-org-entity: Phase 2 prerequisite only (org object, org CC events, org assign target); Phase 1 does not depend on it
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b7dd96a88b1e08f3
sage-review-completeness-reviewed: b7dd96a88b1e08f3
completed: 2026-11-22
---

# Tracker CC and assign: shared checkbox picker modal over the org tree

## Background

The owner (2026-11-19 10:13) asked that CC and assignment stop committing on a
single click and become a checkbox modal, so several people can be picked at
once, and that the picker follow the org chart so a whole team can be chosen.

Today:
- CC picker: one click commits `item.cc` immediately
  (`agents-plugin-tool/internal/wsdashboard/assets/tracker-menu.js` around 526,
  `ccPickList` / `m-cc`); `item.cc {item, member}` is emitted in
  `tracker-menu.js`, while `item.uncc {item, member}` is emitted in
  `tracker-forms.js` (`agents-plugin-tool/internal/wsdashboard/assets/tracker-menu.js#L526-L526`,
  `agents-plugin-tool/internal/wsdashboard/assets/tracker-forms.js#L719-L719`). `item.cc` takes one member
  (`wstracker/check.go` around 1097-1118).
- Assign picker: already multi-select (`asgState().sel`, flat `asgOpts` list);
  `asgEvents` sends one `item.assign` per completion class. Targets are member
  ids or rules `{root, include_root, levels}`; rules are dynamic (re-extended
  after every applied `roles.*`, `member.join` or `item.assign`,
  `wstracker/state.go` around 1700-1703).
- Atomic multi-event write: `POST /tracker/stores/<id>/dispatch {"events":[...]}`
  (`tracker_routes.go` around 911 → `Tracker.Dispatch`, all-or-nothing, 409
  with `{error, refusal: {index,type,reason}}` on refusal;
  `agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L911-L935`,
  `agents-plugin-tool/internal/wstracker/tracker.go#L92-L117`).

Approved mockup (owner check (1390)=ok, 2026-11-19 10:40, v2):
`ai-docs/assets/mockups/261119/tracker-picker-modal/picker.html` (root class
`.tkpm`; frame A = CC, interactive; frame B = assign differences).

## Decisions

1. **One shared modal for CC and assign.** Opened from the CC card "+" / item
   menu "참조에 추가…" and from the assign entry points. Rejected: keeping the
   one-click CC popover — the owner asked for multi-select.
2. **Checked = state after apply.** The modal opens with current CC'd people
   checked ("이미 참조"); newly checked shows "추가", unchecking a current one
   shows "제거" (strikethrough). Clicks send nothing; **적용** sends everything
   in one atomic dispatch batch. Cancel, Esc or outside click discards.
   The CC diff runs only over the live members the modal lists (all of
   them, whatever the search filter hides): a CC held by a removed member
   (still removable, `check.go` `item.uncc`) or by anyone not listed is left
   alone, never unCC'd implicitly. The keyboard model of today's popover
   carries over (arrows move, Enter toggles, Cmd/Ctrl+Enter applies, Esc
   closes).
3. **The list is the org tree** (no separate alias section). A lead row has two
   independent choices, "본인" (that person only) and "이하 전체" (the lead and
   everyone below). Non-lead members have one checkbox. A "미배치" group lists
   members with no lead and no followers (same rule as the org-chart ticket).
4. **Covered display.** Turning on "이하 전체" marks the subtree rows (the
   lead's own "본인" included) as covered — blue band and dimmed check, not
   clickable; individual checks underneath are preserved and return when the
   group is turned off. A lower lead's group toggle shows covered when a group
   above it is on.
5. **Selection band.** Selected targets show as chips under the search bar: a
   group as one chip, a person as a person chip; covered people get no chip of
   their own; clicking a chip undoes that choice. Exception (assign): picks
   under a pending ("대기") group are still sent on apply (Decision 7), so
   they keep their own chips — the band always matches what apply sends.
6. **Personal and team targets are separate** (owner, (1391) note). Picking a
   lead's "본인" targets that person only; "이하 전체" is a team target.
7. **Apply semantics without org objects (Phase 1).**
   - CC: one `item.cc` / `item.uncc {item, member}` per changed person; a
     "이하 전체" group expands to its current members at apply time (snapshot),
     because CC has no rule form today.
   - Assign: persons as member-id targets; "이하 전체" = rule
     `{root: <lead>, include_root: true}` with no `levels` (no levels = the
     whole subtree, `wstracker/state.go` `wholeSubtree`); one `item.assign`
     per completion class as today (`complClass` / `asgClass` / `asgEvents`
     in `tracker-menu.js`). Only uncovered top-level choices are sent: a
     person or lower group covered by an active group is kept in the modal
     but left out of the payload (`checkTargets` does not refuse the
     redundancy, so it would write duplicate records). Only a group that
     would be live (its root is the viewer or inside `VS.viewer.reach`)
     drops what it covers from the payload; a pending ("대기") group still
     shows the covered look but its covered picks are sent as their own
     targets, so a pick live by itself is not held back until the lead
     accepts. Assigned people are
     locked in the modal (unassign stays on the assignee card), and so is a
     lead's "이하 전체" that a live rule of the viewer already covers (today's
     `ruleCovered` / `off`; `checkTargets` refuses it as "already covered").
     People outside the viewer's reach (`VS.viewer.reach`, today's `pend`)
     show "대기" (the existing pending/accept flow).
   - Assign entry points: the assignee card "+" (`openAssign`), the item
     menu, and the new-item quick-add chip (`asgApply` draft branch →
     `newPicked`). For the quick-add draft and for a topic (주제), group
     toggles are hidden (people only, as today's `norule`); a topic keeps a
     single holder (radio).
   - The level-limited "My team" rules of today's popover (`rule(v, 1)`,
     `rule(v, 2)`) are retired: the approved mockup offers only "본인" and
     "이하 전체".
8. **Org objects (Phase 2).** After the org-entity ticket lands (first-class
   org object, admin-managed; owner (1391)=a), a lead who heads an org shows
   the org badge and "이하 전체" reads "<org> 전체 (N명)"; selecting it targets
   the org, not a member snapshot: CC via the new org CC events, assign via the
   org-carrying rule target, both defined by that ticket. Orgs with a vacant
   head seat list as a disabled row ("공석", "관리자가 헤드를 지정해야 선택
   가능"). The mockup legend line that expands an org CC into per-member events
   is superseded by (1391)=a for org targets; it still applies to leads without
   an org.
9. **Layout.** Both modals have a fixed height (640px, min 520px) with the list
   area guaranteed at least 260px; only the list scrolls (owner note on frame
   B, 10:31).
10. **Compatibility.** Phase 1 is T0 (existing event shapes and route only).
    Phase 2 inherits the tiers of the org-entity ticket (T1) and adds no event
    shape of its own.
11. **One shared org-tree helper** (technical, lead (1430), 2026-11-20). The
    tree and the "미배치" grouping come from one standalone pure client helper
    shared with 261119-feat-ws-tracker-org-chart-editor, which writes it
    outside the `wkAct`…`cardTeam` span `tracker_wiki_test.go` pins; this
    ticket reuses that helper. If it has not landed when this ticket is
    implemented, introduce it in the same standalone form; whichever of the
    two lands second switches to the first one's helper. Rejected: a picker-local tree builder — two copies of the
    "미배치" rule could drift between the Team card and the picker.

## Constraints

- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`.
- Test environment (amended 2026-11-20, work moved to the macOS dev
  machine): Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/wsdashboard` or
  `./internal/mcp` package. Node tests run only for touched dashboard test
  files. No Electron.
- Do not start, stop, rebuild or swap the shared ws daemon. LOOK captures use
  an asset-level fixture page in headless Chromium loading the real tracker
  assets with fixture state, not a live daemon. If a faithful capture is
  impossible that way, stop and report to the lead.
- Temp files only under a per-purpose directory in the system temp dir.
- Every new `js.tk.*` key makes `TestTrackerMockupI18nIsCurrent`
  (`wsdashboard/tracker_mockup_i18n_test.go`) fail against
  `ai-docs/ref/tracker-mockup/i18n-en.js`; regenerate it with
  `WS_REGEN_TRACKER_MOCKUP_I18N=1 go test -run TestTrackerMockupI18nIsCurrent ./internal/wsdashboard`.
  The same `ai-docs/ref/tracker-mockup/` directory (`index.html`,
  `fixtures.js`, `engine.js`, `mock-adapter.js`) is the asset-level fixture
  page for the LOOK captures.
- `TestTrackerItemWindowCC` (`wsdashboard/tracker_cc_test.go`) pins the
  one-click `case 'm-cc': return commit([['item.cc', ...]])` line this ticket
  replaces; update it to the modal's apply path.
- Phase 2 is blocked until the org-entity ticket (org object events, org CC
  events, org assign target) has landed on develop.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

## Prior Decisions

- 261119-feat-ws-tracker-org-chart-editor (2026-11-19, Decisions): "\"미배치 (리드 없음)\" group = live members with no lead and no followers, shown in a separate dashed group under the tree." — bearing: constrains
- 261119-feat-ws-tracker-org-entity (2026-11-19, Decisions): "Org CC = new event types with provenance, T1. `item.cc_org {item, org}` / `item.uncc_org {item, org}`. Org CC is dynamic and revokes on removal: `item.uncc_org` removes people who came only through that registration." — bearing: constrains
- 261119-feat-ws-tracker-org-entity (2026-11-19, Decisions): "Org assign = hybrid `item.assign` target, T1. `{root: <current head>, org: <id>, include_root, levels}`." — bearing: constrains
- 261111-feat-ws-tracker-cc-field (2026-11-11, Decisions): "Removing a CC is open to everyone too, symmetric with adding (technical, (279)): CC carries no duty" — bearing: constrains
- 261111-feat-ws-tracker-assignee-edit-and-body-merge (2026-11-11, Decisions): "`item.hide` and `item.assign` stay creator-only." — bearing: constrains
- fc071c6e (2026-11-17, commit): "Mechanically peel the snapshot tail into one ordered tracker fragment, with comments and bytes unchanged." — bearing: constrains
- 7fd706f9 (2026-11-17, commit): "Keep embedded and development readers on the assembled single-IIFE source and preserve its asset identity." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker-menu.js, tracker-forms.js, tracker.css; agents-plugin-tool/internal/wsi18n/tables/*.json; dashboard tests |
| scope.surface | internal | dashboard asset UI; Phase 1 emits existing item.cc, item.uncc, and item.assign event shapes through the existing route |
| scope.new_public_symbol | no | none in this ticket; org event and target contracts belong to 261119-feat-ws-tracker-org-entity |
| scope.new_type_contract | no | Phase 1 uses existing event types; Phase 2 consumes the org-entity ticket's event and target types |
| scope.test_surface | new-files | Phase 1 calls for pure-function Node tests of CC diff, tree coverage, and assignment building; no matching picker-modal test exists in HEAD:agents-plugin-tool/internal/wsdashboard |
| complexity.reuse_points | confirmed | asgState/asgEvents/asgOpts in tracker-menu.js#L297-L395 and the existing dispatch route in tracker_routes.go#L911-L937; sibling org-tree helper is not yet visible on impl/develop/why-shone-henna |
| complexity.side_effect_risk | moderate | one apply can add and remove CC for many members or assign multiple target classes in one batch |
| risk.correctness | high | checked-state diff, nested coverage, snapshot expansion, and assignment completion classes must agree with tracker validation |
| risk.fit | moderate | one modal must preserve the distinct CC and assign entry points, pending/locked states, and approved 640px layout |
| risk.test | moderate | pure logic is Node-testable, while modal interaction and 409 retention require fixture LOOK checks |
| risk.security_or_contract | moderate | CC is open to every member; item.assign requires the creator or creator-gone fallback (check.go#L849-L859); the modal must surface refusal without losing staged choices |

## Phases

### Phase 1: Checkbox modal over the org tree

Implement Decisions 1-7, 9, 10 and 11 for both CC and assign, replacing the
one-click CC popover and the flat assign list, with the apply going through the
atomic dispatch route. Show a refusal (409) inline in the modal with the
refused event's reason and keep the selection.

Verification:
- Pure-function tests (node) for: diffing checked state against current CC
  into cc/uncc events, leaving unlisted and removed members' CCs untouched;
  group snapshot expansion; covered computation with nested groups; assign
  event building (person targets, team rule without `levels`, completion
  classes, targets covered by a live group left out of the payload while
  those under a pending group are still sent, the assign chip band derived
  from the same pure function and equal to the payload targets, locked
  assigned people
  and already-covered groups, people-only for draft and topic).
- Go: touched `wsdashboard` tests (`TestTrackerItemWindowCC`,
  `TestTrackerMockupI18nIsCurrent`, tracker assign/CC tests,
  `tracker_quickadd_test.go`) and `wsi18n` locale parity green (narrow
  `-run` patterns); Node `tracker_draft.test.js` green.
- LOOK: fixture captures of both modals compared side by side with the
  approved mockup frames A and B (without org badges, which are Phase 2),
  plus one capture of a fixture 409 refusal shown inline with its reason
  and the selection kept.

### Result (8d9a77b) - 2026-11-20

Landed on `impl/develop/paper-puma-plus` (a152cef07..8d9a77b2a, merged with
develop in ffd2766a6).

- The modal UI and its pure logic live in a new `tracker-picker.js` fragment,
  registered after `tracker-menu.js` in every tracker fragment list; this
  keeps `tracker-menu.js` (now 897 lines) and `tracker-render.js` (1000) from
  growing. The old one-click CC step and its `js.tk.dlg.cc.*` strings are gone.
- Entry points: the CC card "+", the item menu's new "참조에 추가…" entry, the
  item menu's assign step, and the new-item draft's assign step (people only).
- Apply is one `AD.dispatch` batch; a refusal shows the refused event's reason
  inline and keeps the selection.
- Verification: the 7 tracker node test files pass (picker 7/7); `go test
  ./internal/wsi18n` and `./internal/wsdashboard -run 'Tracker|Asset'` pass.
  The Playwright fixture captures of CC (open, group-covered, add/remove
  diff, light theme, 409 refusal) and assign (item mode, topic mode) match
  mockup frames A and B, apart from the Phase 2 org badges and vacant-org row.
- Keyboard focus ring shows only during arrow-key navigation, not after a
  pointer toggle (8d9a77b2a).

### Phase 2: Org targets

After the org-entity ticket lands: Decision 8 — org badges on head rows, org
group labels and chips, org-targeted CC and assign events as that ticket
defines, vacant orgs as disabled rows.

Verification:
- Node tests for org-target event building and vacant-org handling.
- LOOK: fixture captures compared side by side with the full v2 mockup.

### Result (7d959cb) - 2026-11-21

- Enabled org targets in the ordinary CC and assign pickers (`tracker-picker.js`,
  `tracker.css`): heads of live orgs (including 1-person orgs) show the org
  badge, `<org> 전체 (N명)` group toggle, org-aware covered/pending tooltips,
  and org chips in the selection band; leads without an org keep Phase 1
  behavior (CC member snapshot, plain assign rule).
- CC picker (`pmCcInit`, `pmCcEval`, `pmCcEvents`): selecting an org emits
  `item.cc_org`, existing live org CC registrations open checked, and unchecking
  emits `item.uncc_org` without emitting personal `item.uncc` on only-through
  members (preserving personal CCs and respecting item exclusions per org-entity
  Decision 5).
- Assign picker (`pmAssignOrgInit`, `pmAssignLocks`, `pmAssignEval`,
  `pmAssignEvents`): org heads emit `{root, include_root: true, org}` rule
  targets (shared with `pmRetargetAssignEvents`); existing open org rules open
  checked and unchecking emits `item.unassign {item, rule, root}` before new
  `item.assign` events only for the rule author (`ru.by === viewer`, org-entity
  Decisions 11 & 20), locking other authors' rules and preserving `assignRoot`
  overlap locking until unchecked.
- Vacant orgs (`pmVacantOrgs`, `pmVacantRowHtml`): live orgs with no live head
  render as disabled rows (`공석`, `관리자가 헤드를 지정해야 선택 가능`),
  searchable by name/id and hidden in `norule` (draft/topic) and CC
  selected-only modes.
- Added 14 `js.tk.pm.*` keys across all 11 locale tables under
  `internal/wsi18n/tables/` and regenerated `ai-docs/ref/tracker-mockup/i18n-en.js`.
  No Go code under `internal/wstracker` was modified (T1 compatibility
  inherited, Decision 10). Updated the stale `## Prior Decisions` org-entity CC
  quote to match settled Decision 5.
- Verification:
  - `node --test agents-plugin-tool/internal/wsdashboard/tracker_picker.test.js agents-plugin-tool/internal/wsdashboard/tracker_org_entity.test.js agents-plugin-tool/internal/wsdashboard/tracker_draft.test.js`: 28/28 passed (picker 12/12).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 ./internal/wsi18n`: pass (2.078s).
  - `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run 'TestTrackerItemWindowCC|TestTrackerMockupI18nIsCurrent|TestTrackerAssignee|TestTrackerQuickAdd|Asset' ./internal/wsdashboard`: pass (10.876s). Note: broader `-run 'Tracker|Asset'` hits pre-existing `TestTrackerRelatedScope` failure (`ReferenceError: showHidden is not defined` in `listRelated` from `f49b634e4` on `develop`, untouched here).
  - Headless Chromium captures (dark + light, untracked under `ai-docs/assets/captures/261121/picker-org-targets/`: `cc-org-dark.png`, `cc-org-light.png`, `assign-org-dark.png`, `assign-org-light.png`, `assign-existing-uncheck-dark.png`, `assign-existing-uncheck-light.png`) verified side by side against `ai-docs/assets/mockups/261119/tracker-picker-modal/picker.html` (match, with live DOM dispatch assertions passing for `item.cc_org`, org `item.assign`, and `item.unassign`).

#### Edition (f192daf) - 2026-11-21

- Fixed CC picker per-source provenance (`pmCcInit`, `pmCcEval`, `pmCcRowHtml`, `pmToggleCcPerson`): members CC'd solely via org sources (including moved-out members and deleted/vacant orgs) are no longer auto-picked as personal, so unchecking an org emits only `item.uncc_org` without fabricating personal `item.cc` or destructive `item.uncc` across surviving org sources.
- Derived item-wide CC exclusions from applied `item.uncc` events (`pmCcExcluded`) so globally excluded members reached by a newly selected org do not render as covered or added, stay personally toggleable, and emit `item.cc` when re-picked.
- Updated plain-lead CC snapshots to emit `item.cc` for every listed subtree member lacking a personal source, including members currently CC'd only via an org.
- Matched server assign rule overlap and surviving-rule coverage (`pmRawChildren`, `pmReaches`, `pmRuleCovers`, `pmRuleCovered`, `pmLiveRuleCoversMember`, `pmAssignLocks`, `pmRetargetLockedPeople`) by traversing live members' `leads[]` from `assignRoot` even when the assign-time head was removed, and kept viewer-authored removable org rules unlocked with their removal chip when covered by a broader plain rule.
- Added `pmBuildApplyEvents(cur, vs)` so apply re-derives live org/head mappings, rules, and CC state against concurrent rehead, vacate, or delete instead of using open-time `headToOrg`, and guarded search-box Enter against stale `focusId` when filtered to vacant org rows.
- Expanded `tracker_picker.test.js` (19/19 pass) with pure and DOM click/keyboard tests for moved-member org CC provenance, global exclusion under a parent org, plain-lead snapshot over org-only members, removed-head assign overlap, author-owned org rules under a plain rule, live-state apply mutations, and vacant-row click/keyboard handling; updated `tracker_cc_test.go` apply pin. All other `tracker_*.test.js` (52/52) and `go test ./internal/wsdashboard -run 'TestTracker' -count=1` (77/77) pass.

#### Edition (f848f7a) - 2026-11-21

- Projected `ItemView.CCExcluded` (`json:"ccExcluded,omitempty"`, T0 transparent additive field: older dashboards ignore it, omitted when empty) in `wstracker/view.go` from live members in `it.ccExcluded` (`state_cc.go`) and read it directly in `pmCcExcluded` instead of replaying `VS.events`, so excluded members stay uncovered and re-addable even on unhydrated projected list rows (`events: []`); added `TestItemViewCCExcludedProjection` and excluded `items[].ccExcluded` in `golden_test.go`.
- Replaced live-apply state rebasing in `tracker-picker.js` (`pmSnapshot`, `pmBuildApplyEvents`, `pmApply`) with open-time snapshot staging plus targeted dependency stale checks (org id <-> head mapping for staged/removed orgs and plain lead groups, removed rule ids, and CC personal/org/excluded sources for batch-touched members only). On drift, apply sends nothing, keeps the modal open without mutating snapshot state, and displays `js.tk.pm.stale` across all 11 `wsi18n` locale tables and `ai-docs/ref/tracker-mockup/i18n-en.js`.
- Verified: `tracker_picker.test.js` (20/20), all other `tracker_*.test.js` (52/52), `go test ./internal/wsdashboard ./internal/wstracker -run 'TestTracker|TestItemViewCCExcludedProjection' -count=1` (77/77 wsdashboard, 1/1 wstracker, plus `TestPersonalUnccExcludesOrgReadds` and `TestGoldenParity`), and `go test ./internal/wsi18n -count=1`.

#### Edition (2c45021) - 2026-11-21

- Refined `pmBuildApplyEvents` stale checks: deleted-org CC retarget selecting an org already registered at open compares open vs live registration state (no longer rejected as stale when unchanged), staged plain-lead CC groups validate their open vs live subtree member sets (`openIdx.subtree` vs `liveIdx.subtree`), and assign mode compares an item-scoped state signature (`pmAssignStateSig`: current assignees + open rule ids/states on this item only, unaffected by other items).
- Optimized `ItemView.CCExcluded` projection in `wstracker/view.go` to iterate `it.ccExcluded` directly (skipping empty sets and sorting for determinism) rather than scanning every member per item. Compatibility note (T0 transparent): older dashboards ignore `ccExcluded`; when empty, the field may be omitted (`ItemView` JSON `omitempty`) or `null` (reflective summary path `cmd/ws-mcp/tracker_projection.go`), and `pmCcExcluded` treats `null`/missing as empty.
- Verified: `tracker_picker.test.js` (21/21), all other `tracker_*.test.js` (52/52), and `go test ./internal/wsdashboard ./internal/wstracker -run 'TestTracker|TestItemViewCCExcludedProjection|TestPersonalUnccExcludesOrgReadds|TestGoldenParity' -count=1` (77/77 wsdashboard, 3/3 wstracker).

#### Edition (2bb133e) - 2026-11-22

- Added every member of each staged plain-lead CC group's subtree to `touchedMembers` in `pmBuildApplyEvents` so concurrent CC source/exclusion changes on a subtree member (e.g., a peer unCCing a member who was personally CC'd at open) trigger `js.tk.pm.stale` and send nothing instead of leaving that member unCC'd/excluded.
- Verified `pmApply`'s dispatch-failure path for server-authoritative refusals without adding further client-side coverage logic: added DOM tests pinning 409 `refusal.reason` and non-409 `res.error` handling (modal stays open, `#tkpm-err` displays the server error, `PM.busy` clears and Apply re-enables, selection is preserved, and `wrote()` is not called).
- Verified: `tracker*.test.js` (74/74: `tracker_picker.test.js` 22/22, other `tracker_*.test.js` 52/52) and `go test ./internal/wsdashboard ./internal/wstracker -run 'TestTracker|TestItemViewCCExcludedProjection|TestPersonalUnccExcludesOrgReadds|TestGoldenParity' -count=1` (77/77 wsdashboard, 3/3 wstracker).
