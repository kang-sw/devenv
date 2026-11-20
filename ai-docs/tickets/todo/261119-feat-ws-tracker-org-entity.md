---
title: tracker 조직(법인) 객체와 조직 대상 참조·배정
related:
  261119-feat-ws-tracker-org-chart-editor: prerequisite for Phase 2 (landed); the org management UI lives in that editor
  261119-feat-ws-tracker-cc-assign-picker-modal: its Phase 2 depends on this ticket (org badges, org CC and org assign targets)
---

# Tracker org entity and org-targeted CC and assign

## Background

The owner (2026-11-19 10:31, 10:37) asked that team aliases follow the org
chart and that the org itself be a separate, first-class object, so a lead can
move to another position without the org being lost, and so CC and assign can
target "that org" rather than a lead person or a member snapshot.

Today:
- The org chart is the `roles.set_lead` forest; roles events are admin-only
  through `needAdmin` (`agents-plugin-tool/internal/wstracker/check.go`
  around 130-135).
- Assign rules `{root, include_root, levels}` are re-extended after every
  applied `roles.*`, `member.join` or `item.assign` (`state.go` around
  672-750, 1700-1703). Expansion is additive and never revokes. Rule ids are
  `<event>~<root>`; they are also used by `item.unassign {rule}`
  (`check.go` around 1181), in assignment record ids `<rule-id>><member>`
  (`state.go` around 210) and in the dashboard view.
- `item.unassign {rule}` ends the rule's live records; unassigning one member
  adds them to the rule's `excluded` set (`state.go` around 752-768,
  1470-1495).
- CC is per member only: registers `cc:<item>:<member>` (`state.go` around
  1757-1776); `item.cc` requires `member` (`check.go` around 1108). A CC has
  no provenance; `item.uncc` deletes it, and removing a member who is not
  CC'd is a no-op (`check.go` around 1097-1115).
- Removed members and labels are tombstones: they keep their name and new
  references to them are rejected (`check.go` around 28, 190, 239).
  `member.remove` also drops the member's admin role (`state.go` around
  1646-1656).
- An unknown event type is flagged by `check` and not folded (`check.go`
  around 1471-1472, `replay.go` around 301-306). On a known type, unknown
  payload keys are ignored. An `item.assign` target without a string `root`
  is a `targetShape` rejection of the whole event (`check.go` around 398-402).
- Tracker checkpoints are memory-only: `incremental.go` keeps them in the
  fold memo and `checkpoint_state.go` deep-copies the state. No version reads
  or writes them across a binary boundary.
- `member.profile.org` is an unrelated free-text profile field
  (`wt/profile.go`); the org-chart editor and the picker modal show it as row
  text and search it (`tracker-forms.js` around 791, `tracker-picker.js`
  around 392).

Approved mockups:
- `ai-docs/assets/mockups/261119/tracker-org-entity/editor.html` (owner check
  (1392)=ok, 2026-11-19 10:53; root class `.tkoo`), the org management part
  of the org-chart editor.
- `ai-docs/assets/mockups/261120/tracker-org-entity/editor-v2.html` and
  `editor-v2.png` (owner check (1444)=ok, 2026-11-20 20:24). It extends the
  first mockup with tombstoned orgs: deleted-org rows with an item count and
  an expandable item list in the editor (frame ①), and the item window's
  assignee and CC cards with live-org, deleted-org and personal entries
  (frame ②). Where the two differ, v2 wins.

## Decisions

1. **Org = first-class object** (owner (1391)=a): stable id, name, at most one
   head member. A member heads at most one org. The head's subtree in the
   `set_lead` forest belongs to the org automatically; with nested orgs a
   member's org is the nearest one above (or at) them. A vacant head seat is
   allowed (the org has no members until a head is set). The org moves with
   its head when the head's position changes; giving the org to someone else
   is an explicit admin action. Removing a member who heads an org leaves
   that org vacant in the same fold step, as `member.remove` already drops
   the member's admin role ((1445)). Rejected: an alias string on the lead
   (lost when the lead moves); orgs as a membership list (duplicates the
   tree); refusing a member removal while they head an org (it would block
   the removal on an org setting the remover may not see).
2. **Org events, admin-only, T1.** `roles.org_set {org, name, head}` creates
   or updates an org (`head: ""` = vacant) and `roles.org_remove {org}`
   deletes it. Both go through `needAdmin`. `roles.org_set` naming a head who
   already heads another org is refused (release first). An old binary flags
   both as unknown types and does not fold them: it shows no orgs and changes
   no data (T1).
3. **Deleted orgs are tombstones** ((1441), following the member and label
   tombstones in Background). `roles.org_remove` keeps the org's id and name
   with a removed mark; it is never undeleted (`roles.org_set` on a removed
   id is refused; a new org gets a new id) and cannot be the target of a new
   org CC or org assign. Its head seat is released and its members fall to
   the nearest parent org. Registrations that point at it (Decisions 5 and
   6) stay on the item, labelled with the deleted org's name; they stop
   following the org, and nothing they already added is revoked. Rejected:
   deleting the org outright (its registrations would lose their label, and
   the items they float on could no longer be found).
4. **Personal and org targets are separate** (owner (1391) note). Targeting a
   lead personally stays personal; the org badge next to a lead in pickers is
   client-side sugar for the org target.
5. **Org CC = new event types with provenance, T1** ((1440)=a).
   `item.cc_org {item, org}` registers the org on the item and
   `item.uncc_org {item, org}` removes the registration. While registered,
   the org's current members are CC'd, and people who join the org later are
   CC'd too; people who leave keep the CC they already received. Each CC
   remembers its sources (personal `item.cc`, and each org registration that
   added it):
   - `item.uncc_org` removes the registration and the CCs whose only source
     was that registration; a person also CC'd personally or through another
     org registration stays.
   - `item.uncc {member}` on a person removes their CC whatever its sources
     and excludes them from the item's org registrations, so no registration
     re-adds them automatically. A later personal `item.cc` adds them back as
     a personal CC.
   An old binary flags the two new types and shows only the per-member CCs
   it knows; it folds an `item.uncc` of a member it does not see as CC'd as
   the existing no-op (T1). Rejected: org CC that never revokes when turned
   off (people stay after the box is unchecked); a member snapshot at apply
   time (not dynamic).
6. **Org assign = hybrid `item.assign` target, T1.**
   `{root: <current head>, org: <id>, include_root, levels}`. The rule id
   stays `<event>~<root at assign>` ((1435)) and the org id is stored on the
   rule as a separate field. The new binary follows the org: on later
   extensions it re-roots the same rule at the org's current head, so a head
   change never creates a second rule. An old binary reads the target as an
   ordinary rule on `root` and keeps extending from that member (T1).
   `item.unassign {rule}` and per-member unassign work on org rules as on any
   rule. Rejected: a bare `{org}` target (an old binary rejects the whole
   event as `targetShape`); a rule id keyed on the org id (an old binary
   would then judge the same log differently, e.g. refuse an
   `item.unassign {rule}` the new binary accepts).
7. **View projection** ((1436)). The dashboard tracker state payload exports
   orgs (id, name, head, removed mark) and each item's org registrations (org
   CC registrations, and org assign rules with their org id and rule id)
   separately from the per-member CC list and assignment records, so the UI
   can tell personal entries from org entries and address the removal events.
8. **Leads without an org.** "이하 전체" for a lead who heads no org keeps the
   existing team rule for assign and a member snapshot for CC (picker-modal
   ticket Decision 7).
9. **Editor UI (approved mockups, v2 wins).** Org badges on heads, dragged
   onto another member to reassign the head, plus a "헤드" select as the
   keyboard/click fallback. The badge drag is its own drag kind, distinct
   from the member-row drag that changes leads ((1437)). A drop on a member
   who already heads an org is refused (that member is disabled in the
   select). An org section lists orgs (새 조직 / 이름 바꾸기 / 삭제); a vacant
   org shows as an amber row and is inactive in pickers. Deleting an org
   makes its members fall to the nearest parent org. The staged list is
   shared with lead changes, with one `roles.org_set` per changed org. A
   staged (unsaved) deletion is struck through with undo; when items still
   point at that org, its row warns "업무 N건이 이 조직을 가리킵니다 — 저장하면
   "삭제된 조직"으로 남고 따라가기를 멈춥니다".
10. **Deleted-org cleanup** ((1442)=a). A saved deleted org shows as a dashed
    grey chip with a "삭제됨" tag, distinct from the struck-through staged
    deletion. In the editor's org section, each deleted org that items still
    point at gets its own dashed row with a clickable "업무 N건" that expands
    the item list under the row (item, assign or CC, status; clicking opens
    the item window). Deleted orgs no item points at fold into one
    "삭제된 조직 (N)" row and are not counted in the "조직 N개" total. In the
    item window, the assignee and CC cards show org registrations (live:
    with an expandable "N명 ▸" of the people it added; deleted: dashed grey
    with a "삭제된 조직" band), the card title counts deleted-org
    registrations, and the item carries a "정리 필요" marker.
11. **Removing or retargeting an org registration** ((1443)). "−" on an org
    registration, live or deleted, removes the registration together with the
    people who came in only through it: `item.unassign {rule}` for an org
    assign rule, `item.uncc_org` for an org CC (Decision 5). People added
    personally, or also covered by another registration, stay. "대상 바꾸기…"
    opens the picker; the chosen new target's registration and the old
    registration's removal go in one atomic batch. Either way the item leaves
    the deleted org's "업무 N건".
12. **Save order** inside the one atomic dispatch batch: admin grants → org
    deletions and head releases → org creations and head take-ups → lead
    changes → admin removals, self last. Release before take-up keeps the
    one-org-per-head check satisfied at every step. A staged head swap
    between two orgs cannot be ordered that way, so the builder inserts one
    extra `roles.org_set` that vacates one side first, in the same batch
    ((1437)); the batch is atomic, so the intermediate state is never
    visible.
13. **Label collision** ((1439)). The free-text `member.profile.org` field is
    labelled "소속" in the UI and the new object "조직", with distinct
    labels in every locale.

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`.
- Compatibility: every new surface is T1 (Decisions 2, 5, 6). Checkpoints are
  memory-only (Background), so they add no cross-version risk ((1434));
  instead, every new piece of state (orgs, org registrations, CC sources,
  exclusions) must be deep-copied by `checkpoint_state.go`, and any new
  register type needs its `checkpointHeadValue` case, or incremental replay
  diverges from a full fold.
- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`. Tables are append-only; never
  re-sort one.
- Every new `js.tk.*` key makes `TestTrackerMockupI18nIsCurrent`
  (`wsdashboard/tracker_mockup_i18n_test.go`) fail against
  `ai-docs/ref/tracker-mockup/i18n-en.js`; regenerate it with
  `WS_REGEN_TRACKER_MOCKUP_I18N=1 go test -run TestTrackerMockupI18nIsCurrent ./internal/wsdashboard`.
- Test environment (amended 2026-11-20, work moved to the macOS dev machine,
  (1439)): Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/wsdashboard` or
  `./internal/mcp` package. Before handing back a phase, run
  `-run 'Tracker|Asset'` on `./internal/wsdashboard` plus the whole
  `./internal/wstracker` package. Node tests run only for touched dashboard
  test files. No Electron.
- Do not start, stop, rebuild or swap the shared ws daemon. LOOK captures use
  an asset-level fixture page in headless Chromium loading the real tracker
  assets with fixture state (`ai-docs/ref/tracker-mockup/`), not a live
  daemon. If a faithful capture is impossible that way, stop and report to
  the lead.
- Temp files only under a per-purpose directory in the system temp dir.
- Real org badges need horizontal padding; the mockup's `.tkoo .org` chip is
  cramped and is not the reference for that spacing.
- Do not split `tracker-render.js` in this ticket.

## Phases

### Phase 1: Org object, org events and org targets in the tracker store

Implement Decisions 1, 2, 3, 5, 6 and 7 in `wstracker` (check, fold, state,
checkpoint copy) and expose org state and org registrations in the
dashboard's tracker state payload.

Verification (Go tests):
- `roles.org_set` / `roles.org_remove`: admin-only, one org per head, vacant
  head, rename, delete; membership derived from the tree with nearest-org
  resolution for nested orgs; the org moving with its head on `set_lead`;
  removing a head member leaves the org vacant.
- Tombstones: a removed org keeps id and name, refuses `roles.org_set`,
  `item.cc_org` and org assign targets, releases its head, and its members
  resolve to the nearest parent org; its registrations stay and stop
  following; nothing is revoked.
- `item.cc_org`: newcomers to the org get CC'd after `roles.*` /
  `member.join`; leavers keep their CC; `item.uncc_org` removes only the
  CCs whose sole source was that registration; `item.uncc` of an org-added
  person excludes them from re-adds, and a later personal `item.cc` restores
  them.
- Org assign target: one rule id `<event>~<root at assign>` with the org
  field; re-rooting after a head change; additive expansion; `item.unassign
  {rule}` ends the rule's records.
- Checkpoints: incremental replay equals a full fold across org events, org
  CC and org assign (the existing incremental regression pattern).
- Compatibility: an old-shape reader sees an org assign target as a plain
  root rule; unknown org event types are flagged, not folded; an `item.uncc`
  of a member the old reader does not see as CC'd is a no-op.

### Phase 2: Org management in the org-chart editor and org entries on items

Implement Decisions 4 and 8-13 in the org-chart editor, the Team card (org
badge on heads, nearest org shown for members) and the item window's assignee
and CC cards, with every save going through the atomic dispatch route.

Verification:
- Node tests for staged-change building and the Decision 12 event order,
  including head release before take-up, the inserted vacate event for a
  swap, and a refused drop on an existing head.
- Node tests for the Decision 11 removal and retarget batches.
- Dashboard Go tests (`-run 'Tracker|Asset'` on `./internal/wsdashboard`)
  pass.
- LOOK: fixture captures compared side by side with the approved mockups
  `ai-docs/assets/mockups/261119/tracker-org-entity/editor.html` and
  `ai-docs/assets/mockups/261120/tracker-org-entity/editor-v2.html`
  (`editor-v2.png`), covering frames ① and ②.
