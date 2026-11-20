---
title: tracker 팀 카드 조직도 보기와 관리자용 조직도 편집 창
sage-review-design: completed
sage-review-completeness: completed
related:
  261111-feat-ws-tracker-store-settings-and-labels: the roles dialog and Team card store settings this ticket relabels and extends
  261108-feat-ws-tracker-data-structure: the layer split and removal effects behind set_lead and the admin events
  261110-epic-ws-tracker-team-feedback: workload visibility follows the org tree
sage-review-design-reviewed: 10a1cceda2f6fe00
sage-review-completeness-reviewed: 10a1cceda2f6fe00
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
   "unplaced" state (owner (1386)=ok, announced default). Technical: the
   editor tree and its collapse state follow `editor.html`; collapse state is
   in-memory per open view and not persisted — no file I/O on an interactive
   path (`performance.md`).
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
   toggle state the server would refuse: the last remaining admin (final
   admin set, below) is disabled with the tooltip
   "마지막 관리자는 해제할 수 없습니다" (owner (1385): "관리자 지정은
   관리자가"). Technical: the lock counts the final admin set (current admins
   plus staged `add_admin` minus staged `remove_admin`), not staged removals
   alone — `add_admin` is sent first (Decision 7) and the server judges
   the guard at each point of the batch (`tracker.go:254-258`), so a sole admin
   who stages another member's `add_admin` may also remove themselves. Final
   set over removals-only — it never disables a state the server accepts.
7. **Save is one atomic dispatch.** "저장" sends all staged changes as one
   `POST /tracker/stores/<id>/dispatch {"events":[...]}`
   (`wsdashboard/tracker_routes.go:911` → `Tracker.Dispatch`,
   `wstracker/tracker.go:91-118`): all-or-nothing, and a refusal returns 409
   with `{"error": ..., "refusal": {index, type, reason}}` (the triple is nested under `refusal`: `agents-plugin-tool/internal/wsdashboard/tracker_routes.go#L911-L935`, `TrackerRefusal` at `#L421-L426`), which the editor maps back to the staged
   change and highlights in red. Technical: the shared `commit()` helper in
   `tracker-forms.js` only toasts the error, so the editor makes its own
   dispatch call (the adapter already passes `refusal` through) and keeps an
   index-to-staged-change map for the ordered batch. A cleared lead is
   `roles.set_lead` with `leads: []` (as the existing roles dialog sends), a
   set lead is `leads: [<id>]`. Event order, chosen by the editor, never the
   user: `add_admin` first; then `set_lead` in ascending depth of the final
   tree; `remove_admin` last, with the saving admin's own removal very last.
   Each event is validated against the state including the earlier events of
   the batch (`tracker.go:160-195`), and this order never trips an
   intermediate cycle or loss of the author's admin right when the final
   state is valid.
   Rejected: one request per change — a partial save leaves the org
   half-moved.
8. **Save-time pre-flight check** (owner (1387)=a, 2026-11-19). Before the
   single dispatch, the editor re-applies the staged changes to the store's
   current view and checks all of them at once: it drops no-op changes (a
   lead already equal; an `add_admin` for a member who is already an admin or
   a `remove_admin` for one who is not, which the server would refuse), and
   highlights every other invalid change (cycle, removed
   member, last admin) instead of the 409's first only. A stale base — another
   admin changed a staged member's lead since the editor opened, as the
   view the card holds shows it (base X->Z, staged X->Y; the editor snapshots
   the base leads when it opens) — blocks the save and highlights the row with "다른 관리자가 이미
   변경함", so one admin never overwrites a move the view already shows unseen. This is not a
   concurrency guard: the server already re-checks every event under the store
   write lock after a reload (`wstrackerhost/host.go` `DispatchWith`, then
   `Tracker.validate`, `tracker.go:110-118, 155-195`), and server refusals
   (409 `{index, type, reason}` under `refusal`) still map back to the staged
   change (Decision 7).
   Rejected: a post-sync check that flags a dropped change — the fold-time
   silent cycle drop (`state.go:1618-1624`) happens only for causally
   concurrent `set_lead` events from another clone not yet synced at dispatch
   time, which neither a client check nor the 409 can see; out of scope
   (Constraints).
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
- Test environment (amended 2026-11-20, work moved to the macOS dev
  machine): Go tests run locally and narrowly with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test -count=1 -run '<pattern>' ./internal/<pkg>`
  from `agents-plugin-tool/`; never the whole `./internal/wsdashboard` or
  `./internal/mcp` package. Node tests run only for touched dashboard test
  files. No Electron, no real `agy` binary.
- Do not start, stop, or replace the shared daemon. LOOK captures use an
  asset-level fixture page in headless Chromium (dashboard assets plus a
  fixture tracker view), not a live daemon. If a faithful capture is
  impossible that way, stop and report to the lead.
- Temp files only under a per-purpose directory in the system temp dir.
- Out of scope (owner (1387)=a): a post-sync check that compares folded leads
  with the saved intent and flags a change the fold silently dropped for an
  unsynced cross-clone `set_lead`.
- Every new `js.tk.*` key makes `TestTrackerMockupI18nIsCurrent`
  (`wsdashboard/tracker_mockup_i18n_test.go`) fail against
  `ai-docs/ref/tracker-mockup/i18n-en.js`; regenerate it with
  `WS_REGEN_TRACKER_MOCKUP_I18N=1 go test -run TestTrackerMockupI18nIsCurrent ./internal/wsdashboard`. The same `ai-docs/ref/tracker-mockup/` directory (`index.html`,
  `fixtures.js`, `engine.js`, `mock-adapter.js`, which loads the `tracker-*.js`
  fragments from `file://`) is the asset-level fixture page for the LOOK
  captures.
- Existing tests pin `cardTeam`'s neighbourhood (`tracker_settings_test.go`
  requires `h += settingsHtml(!!me.admin);`; `tracker_wiki_test.go` runs the
  span between `wkAct` and `cardTeam` under Node): put new helpers outside
  that span.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

## Prior Decisions

- 261111-feat-ws-tracker-store-settings-and-labels (2026-11-11, Result): "The roles-event dialog offers only the seven membership types, as readable names (`js.tk.rtype.*`), and shows only the selected type's fields (`rfld` wrappers with `data-rt`, toggled by `rtypeShow`)." — bearing: constrains
- 261111-feat-ws-tracker-store-settings-and-labels (2026-11-11, Result): "The Team card ends with a \"Store settings\" section (`settingsHtml` in `tracker.js`) ... each with an Edit button only for an admin" — bearing: constrains
- 261108-feat-ws-tracker-data-structure (2026-11-08, Decisions): "Layer split at the event boundary ((236), (242)). The protocol owns the identity events (`member.join`, `roles.add_admin`, `roles.remove_admin`, ...) and their verdicts. The domain owns `roles.set_lead`" — bearing: supports
- 261108-feat-ws-tracker-data-structure (2026-11-08, Decisions): "Removal effects ((287)) ... (209) \"a removed member must lead nobody\" is enforced through the protocol's veto hook" — bearing: constrains
- 261110-epic-ws-tracker-team-feedback (2026-11-10, Cross-Child Decisions): "Workload visibility follows the org tree: a lead sees their whole subtree (direct and indirect reports), and an admin sees everyone ((613) a)." — bearing: supports
- 76c9201c (2026-11-10, commit): "Self avatars now carry .me at every site (the team tree, team workload groups, menus previously rendered self in the default tint)" — bearing: constrains
- 25f1d159 (2026-11-17, commit): "Keep embedded and development readers on the assembled single-IIFE source and preserve its asset identity." (tracker-wiki.js is a peeled fragment of tracker.js) — bearing: constrains
- 261107-feat-ws-tracker-ledger-protocol (2026-11-07, Decisions): "Protocol-owned identity events. `member.join` (self-join), `roles.add_admin`, `roles.remove_admin`, `roles.add_key`, `roles.revoke_key`, `roles.remove_member`. `roles.set_lead` ... are domain events." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/tracker-wiki.js, tracker-forms.js, tracker.css, agents-plugin-tool/internal/wsi18n/tables/*.json (11 locale tables) |
| scope.surface | internal | dashboard asset UI only; emits existing roles.set_lead/add_admin/remove_admin via existing dispatch route; no exported symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; no new event type, payload field, route, or store shape (Constraints, T0) |
| scope.test_surface | new-files | existing agents-plugin-tool/internal/wsdashboard/tracker_settings_test.go and tracker_*.test.js cover the card and dialog; Phase 2 batch-ordering, cycle, last-admin and pre-flight checks need a new test file |
| complexity.reuse_points | confirmed | cardTeam (tracker-wiki.js#L577-L619), admin dialog (tracker-forms.js#L288-L331), trackerDispatch route (tracker_routes.go#L911) |
| complexity.side_effect_risk | moderate | editor stages and sends admin and lead writes as one atomic batch; server refusals and fold-time cycle drops are the guard |
| risk.correctness | high | event ordering, cycle and last-admin simulation, and the save-time pre-flight (Decision 8) must be reproduced client-side; the fold-time silent cycle drop (state.go#L1618-L1624) is out of scope |
| risk.fit | moderate | must match view.html and editor.html in a ~380 px side panel with native drag-and-drop; Team card source is a peeled tracker.js fragment |
| risk.test | moderate | drag-and-drop and LOOK capture need an asset-level fixture page; pure ordering and cycle logic is Node-testable |
| risk.security_or_contract | moderate | admin-only writes and last-admin guard are server-enforced (tracker.go#L239-L261); UI must not offer states the server refuses |

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
- Go: `wsdashboard` and `wsi18n` packages green (narrow `-run` patterns) (locale key parity
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
and the save-time pre-flight check (Decision 8).

Verification:
- Unit-level checks (Node test for the touched asset, or Go-side asset test,
  whichever the dashboard already uses for tracker JS) for: batch ordering
  (including a lead swap where a subordinate becomes its former lead's lead,
  and self-removal of admin last), cycle detection for drag and select,
  last-admin lock over the final admin set, mapping a 409 `refusal.index`
  back to the staged change, staged-change undo and discard confirmation, and
  the pre-flight check (no-op dropped, every invalid change highlighted, a
  stale base blocking the save) per Decision 8.
- Go: `wsdashboard` and `wsi18n` green (narrow `-run` patterns).
- LOOK: capture the editor with three staged changes, a refused drag target,
  and the locked last-admin toggle; compare side by side with `editor.html`.
