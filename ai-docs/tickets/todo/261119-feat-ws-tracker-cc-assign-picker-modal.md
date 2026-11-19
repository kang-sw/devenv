---
title: tracker 참조·배정 체크박스 모달 (조직도 트리 선택)
related:
  261119-feat-ws-tracker-org-chart-editor: sibling; shares the org-tree look; no ordering for Phase 1
---

# Tracker CC and assign: shared checkbox picker modal over the org tree

## Background

The owner (2026-11-19 10:13) asked that CC and assignment stop committing on a
single click and become a checkbox modal, so several people can be picked at
once, and that the picker follow the org chart so a whole team can be chosen.

Today:
- CC picker: one click commits `item.cc` immediately
  (`agents-plugin-tool/internal/wsdashboard/assets/tracker-menu.js` around 526,
  `ccPickList` / `m-cc`); CC events are emitted as `item.cc` / `item.uncc
  {item, member}` (`tracker-forms.js` around 719). `item.cc` takes one member
  (`wstracker/check.go` around 1097-1118).
- Assign picker: already multi-select (`asgState().sel`, flat `asgOpts` list);
  `asgEvents` sends one `item.assign` per completion class. Targets are member
  ids or rules `{root, include_root, levels}`; rules are dynamic (re-extended
  after every applied `roles.*`, `member.join` or `item.assign`,
  `wstracker/state.go` around 1700-1703).
- Atomic multi-event write: `POST /tracker/stores/<id>/dispatch {"events":[...]}`
  (`tracker_routes.go` around 911 → `Tracker.Dispatch`, all-or-nothing, 409
  with `{index,type,reason}` on refusal).

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
   their own; clicking a chip undoes that choice.
6. **Personal and team targets are separate** (owner, (1391) note). Picking a
   lead's "본인" targets that person only; "이하 전체" is a team target.
7. **Apply semantics without org objects (Phase 1).**
   - CC: one `item.cc` / `item.uncc {item, member}` per changed person; a
     "이하 전체" group expands to its current members at apply time (snapshot),
     because CC has no rule form today.
   - Assign: persons as member-id targets; "이하 전체" = rule
     `{root: <lead>, include_root: true}`; one `item.assign` per completion
     class as today. Assigned people are locked in the modal (unassign stays on
     the assignee card); people outside the viewer's reach show "대기" (the
     existing pending/accept flow). The topic (주제) choice stays single (radio).
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

## Constraints

- Applicable manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/performance.md`, `ai-docs/manuals/release-compatibility.md`.
- Every new user-visible string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/`.
- No local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  the owner's Windows PC (antivirus). Go tests run on remote w5090 only (no
  Node there). Run only touched JS test files locally with node.
- Do not start, stop, rebuild or swap the shared ws daemon; LOOK captures use an
  asset-level fixture page loading the real tracker assets with fixture state.
- Temp files only under `D:/Temp/<purpose>/`.
- Phase 2 is blocked until the org-entity ticket (org object events, org CC
  events, org assign target) has landed on develop.

## Phases

### Phase 1: Checkbox modal over the org tree

Implement Decisions 1-7, 9 and 10 for both CC and assign, replacing the
one-click CC popover and the flat assign list, with the apply going through the
atomic dispatch route. Show a refusal (409) inline in the modal with the
refused event's reason and keep the selection.

Verification:
- Pure-function tests (node) for: diffing checked state against current CC
  into cc/uncc events; group snapshot expansion; covered computation with
  nested groups; assign event building (person targets, team rule, completion
  classes, locked assigned people).
- Existing dashboard Go tests pass on w5090.
- LOOK: fixture captures of both modals compared side by side with the
  approved mockup frames A and B (without org badges, which are Phase 2).

### Phase 2: Org targets

After the org-entity ticket lands: Decision 8 — org badges on head rows, org
group labels and chips, org-targeted CC and assign events as that ticket
defines, vacant orgs as disabled rows.

Verification:
- Node tests for org-target event building and vacant-org handling.
- LOOK: fixture captures compared side by side with the full v2 mockup.
