---
title: tracker 조직(법인) 객체와 조직 대상 참조·배정
related:
  261119-feat-ws-tracker-org-chart-editor: prerequisite for Phase 2; the org management UI lives in that editor
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
  `<event>~<root>`.
- CC is per member only: registers `cc:<item>:<member>` (`state.go` around
  1757-1776); `item.cc` requires `member` (`check.go` around 1108).
- An unknown event type is flagged by `check` and not folded (`check.go`
  around 1471-1472, `replay.go` around 301-306). On a known type, unknown
  payload keys are ignored. An `item.assign` target without a string `root`
  is a `targetShape` rejection of the whole event (`check.go` around 398-402).
- `member.profile.org` is an unrelated free-text profile field
  (`wt/profile.go`).

Approved mockup (owner check (1392)=ok, 2026-11-19 10:53):
`ai-docs/assets/mockups/261119/tracker-org-entity/editor.html` (root class
`.tkoo`), the org management part of the org-chart editor.

## Decisions

1. **Org = first-class object** (owner (1391)=a): stable id, name, at most one
   head member. A member heads at most one org. The head's subtree in the
   `set_lead` forest belongs to the org automatically; with nested orgs a
   member's org is the nearest one above (or at) them. A vacant head seat is
   allowed (the org has no members until a head is set). The org moves with
   its head when the head's position changes; giving the org to someone else
   is an explicit admin action. Rejected: an alias string on the lead (lost
   when the lead moves); orgs as a membership list (duplicates the tree).
2. **Org events, admin-only, T1.** `roles.org_set {org, name, head}` creates
   or updates an org (`head: ""` = vacant) and `roles.org_remove {org}`
   deletes it. Both go through `needAdmin`. `roles.org_set` naming a head who
   already heads another org is refused (release first). An old binary flags
   both as unknown types and does not fold them: it shows no orgs and changes
   no data (T1).
3. **Personal and org targets are separate** (owner (1391) note). Targeting a
   lead personally stays personal; the org badge next to a lead in pickers is
   client-side sugar for the org target.
4. **Org CC = new event types, T1.** `item.cc_org {item, org}` /
   `item.uncc_org {item, org}`. Org CC is dynamic and additive, like assign
   rules: people who join the org later get CC'd; people who leave keep the
   CC they already received. An old binary flags the types and shows only the
   per-member CCs it knows (T1). Rejected: expanding to per-member `item.cc`
   at apply time (not dynamic).
5. **Org assign = hybrid `item.assign` target, T1.**
   `{root: <current head>, org: <id>, include_root, levels}`. The new binary
   follows the org (re-rooting at the org's current head on later
   extensions); an old binary reads it as an ordinary rule on `root` (T1).
   The rule id keys on the org id, so a head change does not create a second
   rule. Rejected: a bare `{org}` target (an old binary rejects the whole
   event as `targetShape`).
6. **Leads without an org.** "이하 전체" for a lead who heads no org keeps the
   existing team rule for assign and a member snapshot for CC (picker-modal
   ticket Decision 7).
7. **Editor UI (approved mockup).** Org badges on heads, dragged onto another
   member to reassign the head, plus a "헤드" select as the keyboard/click
   fallback. A drop on a member who already heads an org is refused; a swap
   needs a vacate step first. An org section lists orgs (새 조직 / 이름 바꾸기
   / 삭제); a vacant org shows as an amber row. Deleting an org makes its
   members fall to the nearest parent org. The staged list is shared with
   lead changes, with one `roles.org_set` per changed org.
8. **Save order** inside the one atomic dispatch batch: admin grants → org
   deletions and head releases → org creations and head take-ups → lead
   changes → admin removals, self last. Release before take-up keeps the
   one-org-per-head check satisfied at every step.
9. **Label collision.** The free-text `member.profile.org` field gets a UI
   label distinct from the new "조직" object, in every locale.

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`.
- Compatibility: every new surface is T1 (Decisions 2, 4, 5). Org state in
  checkpoints is unverified (`wt/checkpoint_state.go`): Phase 1 checks how an
  old binary reads and rewrites a checkpoint carrying org state, and must
  keep it at T2 or below with recovery from the append-only event log. If
  that needs a T3 boundary, stop and report.
- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`.
- No local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  the owner's Windows PC (antivirus). Go tests run on remote w5090 only (no
  Node there). Run only touched JS test files locally with node.
- Do not start, stop, rebuild or swap the shared ws daemon; LOOK captures use
  an asset-level fixture page loading the real tracker assets with fixture
  state.
- Temp files only under `D:/Temp/<purpose>/`.
- Phase 2 starts after `261119-feat-ws-tracker-org-chart-editor` has landed
  on develop.

## Phases

### Phase 1: Org object, org events and org targets in the tracker store

Implement Decisions 1, 2, 4 and 5 in `wstracker` (check, fold, state,
checkpoint) and expose org state to the dashboard's tracker state payload.

Verification (Go tests on w5090):
- `roles.org_set` / `roles.org_remove`: admin-only, one org per head, vacant
  head, rename, delete; membership derived from the tree with nearest-org
  resolution for nested orgs; the org moving with its head on `set_lead`.
- `item.cc_org`: newcomers to the org get CC'd after `roles.*` /
  `member.join`; leavers keep their CC; `item.uncc_org` stops further
  expansion.
- Org assign target: re-rooting after a head change, one rule per org id,
  additive expansion.
- Compatibility: an old-shape reader sees an org assign target as a plain
  root rule; unknown org event types are flagged, not folded; the checkpoint
  round-trip finding from Constraints.

### Phase 2: Org management in the org-chart editor

Implement Decisions 3, 7, 8 and 9 in the org-chart editor and Team card (org
badge on heads, nearest org shown for members), with the save going through
the atomic dispatch route.

Verification:
- Node tests for staged-change building and the Decision 8 event order,
  including head release before take-up and a refused drop on an existing
  head.
- Existing dashboard Go tests pass on w5090.
- LOOK: fixture captures compared side by side with the approved mockup
  `ai-docs/assets/mockups/261119/tracker-org-entity/editor.html`.
