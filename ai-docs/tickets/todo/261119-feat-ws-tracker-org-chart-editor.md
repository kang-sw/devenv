---
title: tracker 팀 카드 조직도 보기와 관리자용 조직도 편집 창
---

# Tracker Team card org-chart view and admin org-chart editor

## Background

The tracker already models an org: every member has at most one lead
(`roles.set_lead`, a forest; cycles refused), admins are set by admins only
(`add_admin` / `remove_admin`, last-admin guard), and assignment scope,
superior-settable status, and the workload view all follow the lead tree.

The dashboard surfaces this poorly. The Team card (`cardTeam`,
`agents-plugin-tool/internal/wsdashboard/assets/tracker-wiki.js:577-619`)
already nests members under their lead, but with no leads set it reads as a
flat member list, and the only way to set a lead is the generic
"Admin: roles event" dialog (`tracker-forms.js:288-331`, submit at `657-666`)
where an admin picks `set_lead` from a type select. The owner (2026-11-19,
board topic `tracker-org`) did not find it and asked for a real org-chart
view plus a separate admin editor.

Approved mockups (owner (1386)=ok, 2026-11-19):

- `ai-docs/assets/mockups/261119/tracker-org/view.html` — Team card in
  org-chart style, admin and non-admin viewers side by side.
- `ai-docs/assets/mockups/261119/tracker-org/editor.html` — admin org-chart
  editor modal.

Both use fictitious people; they are the visual contract for this ticket.

## Decisions

1. **Team card becomes an org-chart view** (owner (1385)). An indented tree
   with elbow connector lines, two-line rows (name and chips / org label ·
   id), per-subtree collapse with a subtree member count that reads
   "N명 접힘" when collapsed, the "나" marker, admin chip, and the existing
   per-member "상세" expansion (keys, profile edit) unchanged. Layout and
   styling follow `view.html`.
   Rejected: a top-down boxed chart — it overflows the ~380 px side panel at
   three levels (mockup note).
2. **Admin-only "조직도 편집" button** right after the "멤버 N명" header chip
   (owner (1385), position circled by the owner). Non-admins see no button and
   a "조직도는 관리자만 편집할 수 있습니다" hint, as in `view.html` panel B.
3. **"미배치 (리드 없음)" group** = live members with no lead and no
   followers, shown in a separate dashed group under the tree. A root with
   followers stays a tree root. This is a UI-only rule; the data model has no
   "unplaced" state (owner (1386)=ok, announced default).
4. **Separate admin editor modal** (owner (1385)), layout per `editor.html`:
   left the editable tree (current state plus staged changes), right the
   staged-changes list with per-change undo and "모두 되돌리기", footer
   "취소" / "저장 (N건)". Closing with staged changes asks before discarding.
5. **Drag-and-drop reparenting is in scope** (owner 2026-11-19 09:53): drag a
   member onto another to make that one its lead; drop on the "최상위" zone to
   clear the lead. Allowed targets highlight blue; a target inside the dragged
   member's own subtree highlights red with the cycle refusal hint and the
   drop is refused. A per-row "리드 변경" select is the keyboard/click
   fallback, with the member's own subordinates disabled as "(하위 · 순환)".
   Technical: native HTML5 drag-and-drop over a drag library — the dashboard
   ships dependency-free assets, and the mockup's handler shape already works.
6. **Admin designation inside the editor**: a per-row "관리자 지정 / 해제"
   toggle staging `add_admin` / `remove_admin`. Admin writes stay admin-only
   (already enforced, `wstracker/tracker.go:239-261`); the UI must not offer a
   toggle state the server would refuse: the last remaining admin (counting
   staged removals) is disabled with the tooltip
   "마지막 관리자는 해제할 수 없습니다" (owner (1385): "관리자 지정은
   관리자가").
7. **Save is one atomic dispatch.** "저장" sends all staged changes as one
   `POST /tracker/stores/<id>/dispatch {"events":[...]}`
   (`wsdashboard/tracker_routes.go:911` → `Tracker.Dispatch`,
   `wstracker/tracker.go:91-118`): all-or-nothing, and a refusal returns 409
   with `{index, type, reason}`, which the editor maps back to the staged
   change and highlights in red. Event order, chosen by the editor, never the
   user: `add_admin` first; then `set_lead` in ascending depth of the final
   tree; `remove_admin` last, with the saving admin's own removal very last.
   Each event is validated against the state including the earlier events of
   the batch (`tracker.go:160-195`), and this order never trips an
   intermediate cycle or loss of the author's admin right when the final
   state is valid.
   Rejected: one request per change — a partial save leaves the org
   half-moved.
8. **Re-validate on save against the latest state** (owner (1386)=ok,
   announced default). Before dispatching, re-apply the staged changes to the
   store's current view; if another admin's write made any of them invalid
   (cycle, removed member, last admin, lead already equal), block the save
   and highlight the affected changes. Reason: a concurrent `set_lead` that
   closes a cycle is silently ignored at fold (`wstracker/state.go:1618-1624`),
   not refused, so a 409 cannot be relied on.
9. **Key add/revoke and member removal stay in the generic roles dialog**
   (owner (1386)=ok, announced default), whose button is relabelled
   "기타 역할 이벤트…". `set_lead`, `add_admin`, and `remove_admin` remain
   available there too; the editor is the primary path.

## Constraints

- Compatibility tier T0: UI only. No new event type, payload field, route, or
  store shape; the editor emits existing `roles.set_lead`, `roles.add_admin`,
  and `roles.remove_admin` events through the existing dispatch route.
- Every new or changed string goes into all locale tables under
  `agents-plugin-tool/internal/wsi18n/tables/*.json` with the `js.tk.team.*`
  (or a new `js.tk.org.*`) prefix; Korean wording follows the mockups.
- Manuals that apply to the touched paths: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/performance.md`
  (`agents-plugin-tool/` rows of AGENTS.md Implementation Conventions). The
  editor adds no file I/O on interactive paths; it works from the view the
  card already holds.
- This PC (AhnLab V3): no local `go test`, Go tools, gofmt, ws/ws-mcp exes,
  launcher, or Electron. Go tests run on w5090 via
  `D:/sys_lock.sh remote w5090 run ...` (Linux, no Node). Local Node is
  allowed for touched dashboard test files only, never the full
  desktop-electron suite.
- Do not start, stop, or replace the shared daemon: the owner keeps it on the
  track/agent-v2 build today. LOOK captures use an asset-level fixture page in
  a browser (dashboard assets plus a fixture tracker view), not a live daemon.
  If a faithful capture is impossible that way, stop and report to the lead.
- Temp files only under `D:/Temp/<purpose>/`.

## Phases

### Phase 1: Org-chart Team card

Restyle `cardTeam` per Decisions 1-3 and 9 and `view.html`: tree with
connectors, two-line rows, subtree collapse and counts, the "미배치" group,
the admin-only "조직도 편집" button (wired to a no-op or a placeholder until
Phase 2), the non-admin hint, and the generic dialog button relabelled
"기타 역할 이벤트…". Existing chips, avatar relation tints, problem flags
(cycles, unreachable members), and the per-member "상세" expansion keep
working.

Verification:
- Go: `wsdashboard` and `wsi18n` packages green on w5090 (locale key parity
  included); update `tracker_settings_test.go` where it asserts the card or
  dialog text.
- LOOK: capture the card for an admin viewer and a non-admin viewer on a
  fixture with a 3-level tree, one unplaced member, and one collapsed
  subtree; compare side by side with `view.html` panels A and B. Report any
  deliberate deviation.

### Phase 2: Admin org-chart editor

Build the editor modal per Decisions 4-8 and `editor.html`, opened from the
Phase 1 button: editable tree with drag-and-drop and the "리드 변경" select
fallback, cycle refusal while dragging and in the select, the admin toggle
with the last-admin lock, the staged-changes list with undo, discard
confirmation, ordered single-batch save, 409 mapping to the refused change,
and the re-validate-on-save check.

Verification:
- Unit-level checks (Node test for the touched asset, or Go-side asset test,
  whichever the dashboard already uses for tracker JS) for: batch ordering
  (including a lead swap where a subordinate becomes its former lead's lead,
  and self-removal of admin last), cycle detection for drag and select,
  last-admin lock with staged removals, and re-validation blocking after a
  concurrent change.
- Go: `wsdashboard` and `wsi18n` green on w5090.
- LOOK: capture the editor with three staged changes, a refused drag target,
  and the locked last-admin toggle; compare side by side with `editor.html`.
