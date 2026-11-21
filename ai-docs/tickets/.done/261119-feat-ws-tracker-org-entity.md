---
title: tracker 조직(법인) 객체와 조직 대상 참조·배정
related:
  261119-feat-ws-tracker-org-chart-editor: prerequisite for Phase 2 (landed); the org management UI lives in that editor
  261119-feat-ws-tracker-cc-assign-picker-modal: its Phase 2 depends on this ticket (org badges, org CC and org assign targets)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cd3083477f16957d
sage-review-completeness-reviewed: cd3083477f16957d
completed: 2026-11-21
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
  (`agents-plugin-tool/internal/wstracker/profile.go#L14-L18`; no `wt/`
  package exists); the org-chart editor and the picker modal show it as row
  text and search it (`tracker-forms.js` around 791, `tracker-picker.js`
  around 230 for search and 392-395 for the row text).

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
    personally, or also covered by another registration, stay. On a deleted
    org's registration only, "대상 바꾸기…" opens the picker; the chosen new
    target's registration and the old registration's removal go in one
    atomic batch. A live org's registration offers "−" alone. Either way a
    deleted org's registration leaves that org's "업무 N건".
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
14. **Org scope is the head's whole subtree** ((1447), mockup v2 legend:
    "헤드 이하 (리드 연결) 전원이 그 조직", "인원 = 헤드 포함 하위 전체").
    Org CC (Decision 5) reaches the head's whole subtree, nested orgs
    included, the same people an org assign rule rooted at the head with
    `levels: 0` reaches. The nearest org of Decision 1 is the per-member
    display label and the parent a deleted org's members fall to; it does not
    narrow the org's scope.
15. **Vacant and deleted orgs add nobody** ((1448)). A registration on a
    vacant org adds nobody until a head is set; an org assign rule keeps its
    last root and does not extend while the org is vacant, and re-roots when
    a head is set (Decision 6, under the consent rule of Decision 22). A
    deleted org's registrations never add anyone again
    ("새 멤버 배정·참조 없음", mockup v2). `item.cc_org` on a vacant or
    removed org, and an `item.assign` target whose `org` is vacant or removed
    or whose `root` is not that org's current head, are refused. Rejected:
    accepting a registration on a vacant org that stays idle (pickers already
    show vacant orgs inactive, Decision 9).
16. **Removing an org assign rule keeps people another rule covers**
    ((1449)). A person holds one open record per item, so a person covered by
    two rules holds the record of only one. To make Decision 11 true, the
    fold re-extends the item's live rules right after any event that ends an
    org-carrying rule (`item.unassign {rule}`, `{root}` or bare, and
    `item.return`; (1455)), so a person the ended rule held but another live
    rule covers gets that rule's record in the same step.
    Plain rules keep today's fold. An old binary reads the org rule as a
    plain rule and re-extends only at its next `roles.*`, `member.join` or
    `item.assign` (T1). Rejected: leaving the gap until the next unrelated
    refresh (the mockup promises "직접 배정한 사람은 유지" and Decision 11
    "also covered by another registration, stay").
17. **Concurrent org events and org names** ((1450)). Two admins can each
    pass the causal check while making one member head of two orgs, or while
    setting and removing the same org. Org events join the label-list
    fold-order pass (`verdictsFrom`, `labelClash` in `replay.go`): each is
    judged against the org list in fold order and the later conflicting one
    is flagged and not folded, identically in incremental and full replay. A
    `roles.org_set` folded after a concurrent `roles.org_remove` of the same
    org is therefore refused as a set on a removed id. The pass also applies
    `member.remove` to its org list, so a `roles.org_set` naming a head that
    folds after a concurrent removal of that member is flagged there (its
    causal check already passed). Live org names are
    unique, as label names are; a deleted org's name may be reused by a new
    org. Rejected: allowing duplicate live names (pickers and chips identify
    an org by name).
18. **Org ids** ((1454)). The editor mints a new org's id client-side in the
    mockup's `o-<random>` shape; `roles.org_set` creating an org refuses an
    id any org, live or removed, already holds.
19. **Projection of what each registration added** ((1451), extends
    Decision 7). The payload also carries, per item, each CC's sources
    (personal and the org registrations that added it) and, per org
    registration, the people it added (an org assign rule's people are its
    records, already tagged with the rule id). The "N명 ▸" list, the
    "only through it" removal preview of Decision 11 and the deleted org's
    item list read these fields. The preview counts a person as "only
    through it" by coverage, not by record ownership: someone another live
    rule also covers stays under Decision 16 even when the removed rule
    holds their record, so the preview leaves them out. The preview uses the
    same test as Decision 16's re-extension: a person stays only if another
    rule that is live now (not pending, not ended) covers them and has not
    excluded them.
20. **Authority and order of removal and retarget batches** ((1452)). The "−"
    and "대상 바꾸기…" controls on an org assign registration, and its
    "정리 필요" cleanup, are offered only to whom the existing checks allow
    (`item.assign` is creator-only, `item.unassign {rule}` is the rule
    author's); org CC controls are open to every member like `item.cc`. In a
    retarget batch the old registration's removal comes before the new
    registration, because `checkTargets` refuses a rule the author's live
    rule already covers.
21. **`item.unassign {root}` matches the root at assign time** ((1453)). A
    re-rooted org rule keeps its assign-time root (the root its id is built
    from) beside its current root; `item.unassign {root}` and
    `{rule, root}` match the assign-time root, so old and new binaries judge
    them the same.
22. **Consent stays with the registration** (owner (1446)=a, 2026-11-20).
    An org assign rule's consent is judged as for any rule against the head
    at assign time: the request a rule rooted outside its author's reach
    sends (`addRule`, (491)) goes to that head, and `item.accept`,
    `item.decline` and `item.return` of the rule stay with that head (the
    rule's assign-time root, Decision 21), as an old binary sees them. A
    pending org rule extends to nobody until accepted. Once live (forced
    because the author reaches the head, or accepted) it re-roots and extends
    under each new head without a new request. Old and new binaries judge
    every consent event the same. Rejected: re-rooting outside the author's
    reach sends the rule back to pending with a request to the new head (an
    old binary refuses the new head's `item.accept` that the new binary
    accepts); re-rooting only while the author reaches the new head (an org
    assign from outside the org would stop following at the first such head
    change).

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
- Convention: ai-docs/manuals/performance.md (declared for agents-plugin-tool/)
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

## Prior Decisions

- 261119-feat-ws-tracker-cc-assign-picker-modal (2026-11-19, Decisions): "CC: one `item.cc` / `item.uncc {item, member}` per changed person; a \"이하 전체\" group expands to its current members at apply time (snapshot), because CC has no rule form today." — bearing: constrains
- 3573e749 (2026-11-11, commit): "One event per member (`{item, member}`) rather than a list field on item.edit: ... a per-member register is what makes a concurrent add and remove of one member resolve by winner()" — bearing: constrains
- 261108-feat-ws-tracker-data-structure (2026-11-08, commit 46731727): "Rules: new-form rule id `<event>~<root>` so one event can carry several rules; the older `mode: all` keeps the event id so old records and `ruleLevels` are unchanged." — bearing: supports
- 261119-feat-ws-tracker-org-chart-editor (2026-11-20, commit 1f13d8ce): "Extract pure orgTree(members, leadsOverride) helper in tracker.js ... so cardTeam, Phase 2 org editor, and sibling picker modal share one derivation" — bearing: supports
- 29c43cfb (2026-11-19, commit): "Checkpoint serialization of org state is unverified; Phase 1 must keep it at T2 or below or stop and report." — bearing: constrains (answered by (1434): checkpoints are memory-only, see Constraints)
- b5847c83 (2026-11-20, commit): "Removal semantics come from the parent lead's message (owner chose provenance for CC; assign uses the existing item.unassign {rule})." — bearing: supports
- ed704413 (2026-11-20, commit): "Phase 2 keeps its block on 261119-feat-ws-tracker-org-entity (declared in related: as a Phase 2-only prerequisite, no blocked-by key so Phase 1 ..." — bearing: supports
- 261110-feat-ws-tracker-superior-settable-status-and-new-statuses (2026-11-10, Decisions): "The assigner stays a separate concept ... org superiors additionally get the wrap-up statuses and the review authority." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, state.go, replay.go, payload.go, view.go, checkpoint_state.go; agents-plugin-tool/internal/wsdashboard/assets/tracker-forms.js, tracker-picker.js, tracker-render.js; agents-plugin-tool/internal/wsi18n/tables/ |
| scope.surface | cross-module | new event types in wstracker consumed by wsdashboard assets and the dashboard state payload; new locale keys in wsi18n |
| scope.new_public_symbol | yes | new exported event-type constants and payload structs beside TypeSetLead and TypeItemCC in agents-plugin-tool/internal/wstracker/payload.go#L33-L42, #L175-L176; new org view fields in view.go |
| scope.new_type_contract | yes | roles.org_set, roles.org_remove, item.cc_org, item.uncc_org event payloads and an org field on the item.assign target |
| scope.test_surface | new-files | existing agents-plugin-tool/internal/wstracker/cc_test.go, team_test.go, incremental_regression_test.go and wsdashboard/tracker_org_editor.test.js cover neighbours; the ticket names no org test file, so org fold, tombstone and checkpoint-parity tests are new |
| complexity.reuse_points | confirmed | needAdmin (check.go#L130-L135), extendRule and refreshLive (state.go#L728-L752), applyCC register (state.go#L1757-L1776), label and member tombstone checks (check.go#L20-L31, #L185-L195), orgTree helper (tracker.js#L268) |
| complexity.side_effect_risk | high | refreshLive runs after every roles.*, member.join and item.assign (state.go#L1700-L1703), so org re-rooting and CC provenance touch every fold and the checkpoint deep copy |
| risk.correctness | high | nearest-org resolution, re-rooting an existing rule at a changed head, CC source tracking with exclusions, and incremental-versus-full-fold parity must all agree |
| risk.fit | moderate | UI must match two approved mockups (v2 wins) inside the existing org-chart editor without splitting tracker-render.js |
| risk.test | moderate | Go fold and checkpoint parity are testable narrowly; drag-and-drop and LOOK captures need an asset-level fixture page |
| risk.security_or_contract | high | four new T1 wire surfaces and a hybrid assign target must stay readable by an old binary (flagged unknown types, plain root rule); org events are admin-only |

## Phases

### Phase 1: Org object, org events and org targets in the tracker store

Implement Decisions 1, 2, 3, 5, 6, 7, 14-19, 21 and 22 in `wstracker` (check, fold,
state, checkpoint copy) and expose org state, org registrations and what
each registration added in the dashboard's tracker state payload.

Verification (Go tests):
- `roles.org_set` / `roles.org_remove`: admin-only, one org per head, vacant
  head, rename, delete; membership derived from the tree with nearest-org
  resolution for nested orgs; the org moving with its head on `set_lead`;
  removing a head member leaves the org vacant; a new org's id held by a
  live or removed org is refused; a duplicate live name is refused and a
  deleted org's name is reusable.
- Concurrency (Decision 17): two concurrent `roles.org_set` making one
  member head of two orgs, and a concurrent `roles.org_set` /
  `roles.org_remove` of one org, flag the later one in fold order, the same
  in incremental and full replay; two concurrent non-conflicting
  `roles.org_set` of one org both fold, the later in fold order winning as
  label edits do; a `roles.org_set` naming a head concurrent with that
  member's `member.remove` leaves the removed member heading no org in
  either fold order (folded first, the removal vacates the seat; folded
  after it, the `roles.org_set` is refused for naming a removed member).
- Scope (Decision 14): an org CC on a parent org reaches the members of a
  nested child org.
- Vacant orgs (Decision 15): `item.cc_org` and an org assign target on a
  vacant org are refused, as is an org target whose `root` is not the
  org's current head; an org rule whose org becomes vacant keeps its last
  root and does not extend, and re-roots when a head is set.
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
  {rule}` ends the rule's records. A person it held who is covered by
  another live rule holds that rule's record in the same step, likewise
  when the org rule ends by `item.unassign {root}`, a bare `item.unassign`
  or `item.return` (Decision 16). `item.unassign {root}` matches the
  assign-time root after a re-root (Decision 21).
- Removal on dead orgs (Decisions 3 and 11): `item.uncc_org` and
  `item.unassign {rule}` on a registration of a removed or vacant org are
  accepted, unlike new registrations, and remove only what Decision 5 and
  Decision 16 say.
- Consent (Decision 22): an org rule rooted outside its author's reach
  stays pending with its request on the assign-time head through a head
  change, extends to nobody while pending, and that head's `item.accept`
  makes it live under the org's current head; a live rule re-roots under a
  new head outside the author's reach without a new request; `item.return`
  of an accepted org rule is the assign-time head's; an old-shape reader
  judges each of these consent events the same; a pending org rule accepted
  after its org became vacant or was removed goes live and adds nobody.
- Projection (Decisions 7 and 19): the state payload carries orgs (id, name,
  head, removed mark), each item's org CC registrations and org assign rules
  (org id, rule id), each CC's sources and each registration's added
  people.
- Checkpoints: incremental replay equals a full fold across org events, org
  CC and org assign (the existing incremental regression pattern).
- Compatibility: an old-shape reader sees an org assign target as a plain
  root rule; unknown org event types are flagged, not folded; an `item.uncc`
  of a member the old reader does not see as CC'd is a no-op.

### Result (67e9125) - 2026-11-20

- Added first-class org state and tombstones, admin-only org lifecycle events,
  fold-order conflict handling (including joined and removed members), nearest
  org and whole-subtree resolution, and checkpoint-safe copies.
- Added dynamic org CC registrations with personal/per-org provenance,
  exclusions and registration additions, plus org assignment rules that retain
  their assign-time identity and consent while following the current head.
  Ending an org rule immediately re-extends other live rules.
- Projected orgs, org CC registrations and sources, registration additions and
  org assignment identity through the tracker view and dashboard projection.
  No Phase 2 editor or item-window UI was started, and no locale strings were
  added.
- Compatibility remains T1: the four new event types are additive constants
  (`payload.go` lines 34-35 and 183-184), while an old binary's default event
  check flags unknown types before fold (`check.go` line 1580). The hybrid
  assign target still carries the legacy `root` shape (`org_test.go` lines
  9-19), so an old reader treats it as an ordinary root rule and changes no org
  state.
- Lite review found one Critical joined-head replay bug and one Important
  nested-org test gap. Both were fixed in `67e9125`; no Minor findings remain.
- Verification passed: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 ./internal/wstracker` and `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test -count=1 -run 'Tracker|Asset'
  ./internal/wsdashboard` (one package passed, zero failed, for each command).

#### Edition (fd021c0) - 2026-11-20

- Kept old/new assign verdicts aligned after an org head change by judging an
  existing org rule's overlap from its assign-time root, while extension alone
  follows the current head. A concurrently folded head change now also sets the
  rule's current root immediately without moving its id or consent request.
- Bare `item.unassign` now closes all of the actor's rules and records before a
  single re-extension of surviving rules. Root-qualified unassign and
  `item.return` re-extension paths are covered explicitly, including the
  removed-creator admin stand-in case.
- Personal `item.uncc` exclusions now belong to the item and apply to every
  current and future org registration until a personal `item.cc` clears them.
  Checkpoints own the new exclusion map.
- Added `RuleView.assignRoot` beside the current `root` (`view.go` lines
  262-276 and 910-915), allowing Phase 2 to form root-qualified unassign
  payloads without parsing rule ids. This view-only additive field is T0 for
  field-tolerant old readers; the existing org event and target behavior stays
  T1.
- Indexed active org CC registrations by item during view projection and
  cached each org's member scope once per roles refresh event; both indexes are
  in-memory only and add no file I/O or persisted state.
- Closed the remaining Phase 1 test gaps: both concurrent member-removal
  orders, incremental/full conflict parity, joined newcomers, nearest-parent
  fallback after child-org removal, vacant registration cleanup, old-reader
  `item.uncc` no-op behavior, and a real org-ignoring target check/fold path.
- Verification passed: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 ./internal/wstracker`; `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test -count=1 -run 'Tracker|Asset'
  ./internal/wsdashboard`; `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 -run 'TestTrackerProjectionSummaryContract|TestTrackerProjectionDetailVersions'
  ./cmd/ws-mcp`; native `go vet ./internal/wstracker ./cmd/ws-mcp`; and the
  same vet command with `GOOS=windows`.

#### Edition (b3f393b) - 2026-11-20

- Extended assign-time-root compatibility to legacy `item.assign mode:"all"`,
  so an org rule re-rooted under a new head does not make the new binary reject
  an assign the old binary accepts.
- Root-qualified `item.unassign` now ends every matching rule and record before
  one re-extension, preventing records from being created and ended by the
  same event when multiple rules share an assign-time root.
- Added a fold-owned per-item org CC registration index. Personal `item.cc` and
  `item.uncc` now touch only that item's registrations, view projection reuses
  the index, and checkpoints derive an owned index from copied canonical state
  while preserving a zero-value state's nil shape.
- Verification passed: `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 ./internal/wstracker`; `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local go test -count=1 -run 'Tracker|Asset'
  ./internal/wsdashboard`; `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 -run 'TestTrackerProjectionSummaryContract|TestTrackerProjectionDetailVersions'
  ./cmd/ws-mcp`; native `go vet ./internal/wstracker`; and the same vet command
  with `GOOS=windows`.

### Phase 2: Org management in the org-chart editor and org entries on items

Implement Decisions 4, 8-13, 18 (client-side id minting) and 20 in the
org-chart editor, the Team card (org badge on heads, nearest org shown for
members) and the item window's assignee and CC cards, with every save going
through the atomic dispatch route.

Verification:
- Node tests for staged-change building and the Decision 12 event order,
  including head release before take-up, the inserted vacate event for a
  swap, and a refused drop on an existing head.
- Node tests for the Decision 11 removal and retarget batches, including
  the removal-before-registration order and the author gating of org assign
  controls (Decision 20).
- Node tests for the item-window derivations of Decision 10: the "N명 ▸"
  people of a registration, the deleted-org count in the card title and the
  "정리 필요" marker.
- Dashboard Go tests (`-run 'Tracker|Asset'` on `./internal/wsdashboard`)
  pass.
- LOOK: fixture captures compared side by side with the approved mockups
  `ai-docs/assets/mockups/261119/tracker-org-entity/editor.html` and
  `ai-docs/assets/mockups/261120/tracker-org-entity/editor-v2.html`
  (`editor-v2.png`), covering frames ① and ②.

### Result (89123e5) - 2026-11-21

- Added pure org entity helpers (`orgIndex`, `orgMintId`, `orgItemRefs`), org
  staging/undo, case-insensitive org name uniqueness, and Decision 12 batch
  ordering (`add_admin` -> `org_remove` -> head releases -> topological
  `org_set` with head-swap vacate and name-swap cycle breaking -> `set_lead` ->
  `remove_admin` self last) in `tracker.js`.
- Wired the org-chart editor modal (`tracker-forms.js`, `tracker-mount.js`) with
  draggable org badges on tree heads, one-org-per-head refusal banner, orgs list
  with inline rename and head dropdown locking, staged-delete item-reference
  warnings, vacant-org warnings, saved deleted-org tombstones with expandable
  referencing item lists, and folded unreferenced tombstones.
- Updated `cardTeam` (`tracker-wiki.js`) to display org badges on heads and
  nearest enclosing org on non-head members, and extended `targetsText` and
  `describe` (`tracker.js`) for `roles.org_set`, `roles.org_remove`,
  `item.cc_org`, `item.uncc_org`, and org assign/unassign events.
- Added item-window org entries (`tracker-window.js`) for Assignees and CC cards
  (active org groups, deleted-org amber warning cards, `정리 필요` chip gated by
  Decision 20, coverage-based removal previews under Decision 19, item-wide
  personal `item.uncc` exclusion tooltip), atomic retarget picker mode
  (`tracker-picker.js`, including 1-person org heads and surviving-rule
  `lockedP`), org styles (`tracker.css`), all 11 locale tables under
  `internal/wsi18n/tables/` (including Decision 13 affiliation/org disambiguation),
  and regenerated `ai-docs/ref/tracker-mockup/i18n-en.js`.
- Kept `internal/wsdashboard/daemon.go` untouched and did not split
  `tracker-render.js`; every save goes through the existing atomic dispatch
  route. Compatibility stays T1 (no new wire surfaces beyond Phase 1).
- Lite review (`03febff7-01-org-entity-phase2.md`) reported 0 Critical, 3
  Important (pending record deduplication in `itemOrgAssignInfo`, dead/vacant
  org non-coverage in `ruleCoversMember`, and surviving-rule `lockedP` in
  retarget assign mode), and 2 Minor (IME `keyCode === 229` guard on org rename
  and helper coverage tests). All 3 Important and both Minor items were fixed in
  `89123e5`.
- LOOK verification: 7 headless Chromium captures under
  `ai-docs/assets/captures/261120/org-entity/` (`frame1-editor.png`,
  `frame1-editor-orgs.png`, `frame1-editor-light.png`,
  `frame2-item-window.png`, `frame2-item-window-cc.png`,
  `frame3-retarget-picker.png`, `team-card.png`) inspected and verified matching
  `ai-docs/assets/mockups/261120/tracker-org-entity/editor-v2.png` frames ① and
  ② (with polish commit `787be76` fixing tombstone date slicing and undo button
  styling).
- Verification passed: Node tests (`32/32` across
  `tracker_org_editor.test.js`, `tracker_org_entity.test.js`,
  `tracker_picker.test.js`, `tracker_org_tree.test.js`, `tracker_draft.test.js`),
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -timeout 20m -run 'Tracker|Asset' ./internal/wsdashboard`,
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 ./internal/wstracker`,
  and all tracker/org `wsi18n` checks (`TestEveryUsedKeyIsInTheEnglishTable`,
  `TestOtherTablesHoldOnlyEnglishKeysAndNames`, `TestOtherTablesHoldEveryEnglishKey`,
  `TestPlaceholdersMatchEnglish`, `TestPluralKeysComeInPairs`,
  `TestTablesAreValidJSON`; only `TestEveryEnglishKeyIsUsed` fails on 4
  pre-existing unrelated `wb.*` keys already present on `develop` `415f1f033`).

#### Edition (b42f0ba) - 2026-11-21

- Aligned `lockedG` overlap checks in `tracker-picker.js` with
  `wstracker/check.go`: `pmRuleCovers` now uses each surviving live rule's
  assign-time root (`assignRoot || root`), `pmRuleCovered` checks `!y.ended`,
  and `openPicker` keeps surviving live rules of deleted or vacant orgs in
  `otherRules` (`pmRetargetLockedPeople` left unchanged).
- Replaced `.toLowerCase()` org-name comparisons in `tracker.js`
  (`orgEdNameClash`, `orgEdPreflightAndBuild` `nameSeen` and `nameOwner`) with
  `orgNameFoldKey`, matching Go's `strings.EqualFold` at least as strictly
  (including Greek `σ`/`ς`/`Σ` and German `ß`/`ẞ`).
- Added regression tests in `tracker_org_entity.test.js` for re-headed
  `assignRoot` locks, deleted/vacant org live rule locks, and Greek `σ`/`ς`
  uniqueness checks.
- Verification passed: `node --test internal/wsdashboard/tracker_*.test.js`
  (`53/53` passed) and `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test
  -count=1 -timeout 20m -run 'Tracker|Asset' ./internal/wsdashboard` (`ok
  14.987s`).
