---
title: "Tracker: a superior may set a set of wrap-up statuses on a subordinate's assignment, and new statuses dropped / in-review"
related:
  261110-feat-ws-dashboard-tracker-quick-add-fields: "its Decision 8 keeps the status chip to the creator alone because a superior cannot set a subordinate's status today; this ticket may relax that"
  261110-feat-ws-tracker-idea-status-and-assignment-invariant: "defines the current status set (idea, todo, doing, blocked, done) and the assignment invariant this extends"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 12b9c9a12eee6812
sage-review-completeness-reviewed: 12b9c9a12eee6812
completed: 2026-11-11
---

# Tracker: superior-settable statuses and new statuses

## Background

User, 2026-11-10 whiteboard, note on topic "tracker 빠른 추가 필드" after
(609): "todo 티켓 수준으로, 상위에서 지정할 수 있는 상태 세트를 몇 개 두는 걸로
하죠. 뭐 blocked랑 done으로는 상위에서 지정할 수 있다던지(마무리 취급).
그리고 해당 티켓에 새 가능한 상태 추가하는 것도요. done과 비슷한 취급의
dropped, 그리고 in-review상태 정도? 이건 TBD로 구현 시점에 추가 논의"

Today `item.status` changes only the author's own assignment record
(`agents-plugin-tool/internal/wstracker/check.go`, `TypeItemStatus` ->
`needRec(it, a)`), subordinates included, and the settable set is idea,
todo, doing, blocked (done goes through `item.close`). Status ranking and
roll-up live in `internal/wstracker/view.go` (`rank`, item-level status
derivation) and the query filter set in `query.go` (`QueryStatuses`).

## Decisions

1. **A superior may set wrap-up statuses on a subordinate's assignment
   (user).** "Superior" means any org-tree ancestor of the assignee (user,
   (242) b), and the superior-settable subset is blocked, done, and dropped
   (user, (243) a), treated as the superior's wrap-up actions.
   A superior's write applies to a live record, the creator-fallback record
   included, and not to a pending one; dropped is a status, not an end, so
   it does not trigger the fallback. A superior-set blocked is left by the
   assignee under the existing status rules or by any org superior
   (technical, review).
   Rejected: the assigner as the superior ((242) a); the direct superior
   only ((242) c); blocked and done only, or every status ((243)).
2. **A superior's done is final (technical, (244); wording (278)).** When
   the assignee's org superior records done, it is done directly, with no
   review step: the actor is the confirmer.
   Rejected: passing through review and self-confirming.
3. **The assigner stays a separate concept (user, (277) a).** The assigner
   remains the record of who gave the assignment, possibly a peer; org
   superiors additionally get the wrap-up statuses and the review authority.
   Rejected: reterming the assigner as the org superior, so that only
   superiors assign, which removes peer and cross-team pending assignments.
4. **`dropped` is a terminal assignment state outside item roll-up (user,
   (245) a).** When the remaining assignments are all done the item is done;
   when all are dropped the item shows dropped; a mix of done and dropped
   rolls up to done (technical, review). A dropped assignment reopens
   as done does. `tracker.query` and the dashboard filter gain
   `status:dropped`, and dropped is excluded from `open`. `status:awaiting`
   stays accepted as an alias of `status:in-review`, so a downstream
   playbook copy or saved query that still sends it does not fail
   (technical, review).
   The assignee drops their own assignment directly only when its
   completion is `assignee`; under `completion: assigner`, only the
   assigner or an org superior drops it (technical, (285)).
   Rejected: identical to done in roll-up with a different label; letting
   the assignee drop past the review gate, which would turn drop into an
   unreviewed way out of a superior's assignment ((285)).
5. **`in-review` replaces awaiting (user, (246) a).** It is the "please
   review" state just before done. A close under `completion: assigner`
   enters it automatically, and the assignee may enter it voluntarily on any
   assignment. The assigner or any org superior of the assignee resolves it:
   done approves, and a rejection returns the assignment to doing. The
   assignee may leave a voluntary in-review with a status write, as from
   awaiting today, and a self-assigned assignee resolves their own
   in-review as its assigner (technical, review). Existing
   awaiting records fold as in-review, and it takes awaiting's place in the
   status rank (`view.go`), between idea and done.
   Rejected: keeping awaiting as the forced gate beside a separate voluntary
   in-review resolved by superiors.
6. **The completion default follows the relationship (user, (282) b).** An
   assignment given by an org superior of the assignee defaults to
   `completion: assigner` (a close goes through in-review); a self- or peer
   assignment defaults to closing directly. The assigner may override the
   default per assignment through the existing `completion` field. The
   relationship is read from the org tree in the assign event's causal past.
   The writers resolve the default and record it explicitly in the event
   (the MCP writer, `asgCompl`, and quick-add); the fold keeps today's rule
   that an absent `completion` means `assignee`, so old events keep their
   meaning. An assignment whose targets mix subordinates and peers is
   written as one event per relationship class; a rule or levels target
   takes the default from whether the assigner is an org superior of the
   rule root (technical, review).
   This reverses, for peer assignments, the default of
   `261109-feat-ws-tracker-review-stage-and-assignment-defaults` (board
   (20) b: assigning to someone else defaults to `assigner`), which the
   quick-add chip follows through `asgCompl`; both follow the new rule.
   Rejected: keeping the per-assignment choice with the default `assignee`
   ((282) a); keeping (20) b's "anyone else defaults to assigner"; requiring in-review for every superior's assignment with no
   override ((282) c).
7. **The quick-add status chip offers the wrap-up set (technical, (247);
   wording (278)).** When the creator is the assignee's org superior, the
   chip offers blocked, done, and dropped, relaxing
   `261110-feat-ws-dashboard-tracker-quick-add-fields` Decision 8, whose
   stated reason this ticket removes. With several assignees the chip
   offers the wrap-up set only when the creator is an org superior of
   every picked assignee, and it writes one status per assignee
   (technical, review).
   Rejected: keeping the chip to the creator's own assignment.
8. **Mirroring and one phase (announced, (248)).** The new statuses and
   rules are mirrored in the reference engine
   (`ai-docs/ref/tracker-mockup/engine.js`) and its golden cases,
   `tracker.update`, and the tracker playbook of all three packages, as the
   existing event types are ((210)).

## Constraints

- Korean UI strings live in `internal/wsi18n/tables/ko.json`; every table
  holds every English key.
- `261110-feat-ws-tracker-workload-balance-view` counts assignments per
  status; whichever of the two tickets lands second folds dropped and
  in-review into those counts, each with its own per-status count; both
  stay outside the load total, as awaiting does today.
- `261110-feat-ws-tracker-daemon-event-hook` carries assignment status
  strings. If it lands first, this ticket switches its values from
  awaiting to in-review and adds dropped; when the hook has not yet shipped
  in a release this needs no `v` bump, and otherwise it bumps `v` under
  that ticket's Decision 4.
- `261111-feat-ws-tracker-cc-field` touches the same view, query, engine,
  dashboard, and playbook surfaces; its notifications for status and
  completion changes cover in-review and dropped.
- Golden files: view JSON fields carry no `omitempty`; regenerate them from
  the reference engine after each merge, never merge them by hand.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/skill-authoring.md`, `ai-docs/manuals/wsflow-mirroring.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Event and field names, and the rank of dropped, are chosen in planning.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Default completion mode (user; board (20) b): assigning to someone else defaults to `assigner` (awaiting, then the assigner approves); assigning to oneself defaults to `assignee`." — bearing: contradiction-candidate
- 261110-feat-ws-dashboard-tracker-quick-add-fields (2026-11-10, Result bf059c46): "For non-me assignees, the completion defaults to assigner, following `asgCompl`'s rule." — bearing: constrains
- 261110-feat-ws-tracker-idea-status-and-assignment-invariant (2026-11-10, commit 2dfd0f9e): "A status write that had not seen the fallback's start event folds onto the ended record (onEnded) rather than the new fallback" — bearing: constrains
- c536206a (2026-11-10, commit): "an empty status would rank differently in the engine's RANK sort (undefined -> NaN) and Go's rank map (missing -> 0), breaking golden parity for the my.assignments order" — bearing: constrains
- 261108-feat-ws-tracker-data-structure (2026-11-08, commit 4449ed91): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains
- 4771bc05 (2026-11-08, commit): "\"done\" is disabled while awaiting, and reads \"request done\" for an assigner-confirmed item, as in the menu." — bearing: constrains
- e11589f5 (2026-11-08, commit): "Accept keeps the assigner and completion mode, which is how (214) \"requester confirms\" falls out" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wstracker/check.go, state.go, view.go, query.go; ai-docs/ref/tracker-mockup/engine.js and wstracker/testdata/cases; internal/mcp/tracker_tools.go; internal/wsdashboard/assets/tracker.js; internal/wsi18n/tables/*.json; rsrc/tracker/tracker.md in agents-plugin, agents-plugin-wsflow, agents-plugin-pi |
| scope.surface | public-interface | tracker.update and tracker.query MCP tools, QueryStatuses in query.go#L37, the tracker event protocol status values, and the shipped tracker playbook |
| scope.new_public_symbol | yes | statuses dropped and in-review, status:dropped and status:in-review query values; event and field names chosen in planning |
| scope.new_type_contract | yes | item.status payload settable set in state.go#L13 widens; view JSON status values change awaiting to in-review and add dropped |
| scope.test_surface | existing | agents-plugin-tool/internal/wstracker/*_test.go, testdata/cases with gen-golden.js, query_test.go, internal/mcp/tracker_tools_test.go |
| complexity.reuse_points | confirmed | org tree in state.go#L412, completion field check in check.go#L489-L495, awaiting confirm flow in check.go#L728-L765, rank map view.go#L781, roll-up view.go#L388-L408 |
| complexity.side_effect_risk | high | existing awaiting records must fold as in-review and goldens regenerate; completion default for peer assignments changes |
| risk.correctness | high | fold, roll-up, and authorization changes across two engines that must stay in golden parity |
| risk.fit | moderate | extends the existing awaiting and completion mechanisms but touches Go, JS engine, MCP, dashboard, and three playbooks |
| risk.test | moderate | golden and reasons parity cover the domain; dashboard needs a browser check |
| risk.security_or_contract | high | new authorization rule lets org superiors write status on others' assignments; protocol status values change |

## Phases

### Phase 1: superior-settable statuses and new statuses

Scope:
- Decisions 1-7 in the Go domain (`check.go`, `state.go`, `view.go`,
  `query.go`), the reference engine with its golden cases,
  `tracker.update` and `tracker.query`, the playbook in all three packages,
  and the dashboard (status menus for superiors, the in-review resolve
  actions, the dropped filter, and the quick-add chip).

Verification:
- Tracker tests: an org superior (direct and indirect) sets blocked, done,
  and dropped on a subordinate's assignment, and a non-superior is
  rejected; a superior's done skips review; a close under
  `completion: assigner` enters in-review, which the assigner or any org
  superior approves to done or rejects to doing; the assignee enters
  in-review voluntarily; a superior's assignment defaults to
  `completion: assigner`, a self- or peer assignment closes directly, and an
  explicit `completion` overrides either default; an item whose remaining
  assignments are all dropped shows dropped, and dropped stays out of
  roll-up and `open`; the assignee drops their own assignment under
  `completion: assignee` and is rejected under `completion: assigner`; a
  dropped assignment reopens; an old awaiting record
  folds as in-review; a mix of done and dropped rolls up to done; a
  superior-set blocked is left by the assignee and by a superior; a
  superior's write on a pending record is rejected; the assignee leaves a
  voluntary in-review, and a self-assigned assignee resolves their own;
  a legacy assign event without `completion` keeps the assignee default
  under a superior; an assignment mixing a subordinate and a peer writes
  one event per class with the matching defaults.
- Golden cases from the reference engine agree with the Go domain.
- `status:dropped`, `status:in-review`, and the `status:awaiting` alias
  query tests; a browser check of the superior's status menu, the review
  actions, the dropped filter, and the quick-add chip with one and with
  several assignees.
- The three tracker playbook copies stay in parity, and `ko.json` holds the
  new UI keys.

### Result (3a04d584d) - 2026-11-11

Landed on `impl/develop/patio-album-job` in commits 6915ebd0f (domain,
engine, goldens), b00bad68a (MCP), 2c0482b0e (playbooks), a125cdae9
(dashboard and i18n), 8ee793e38 (mockup docs), 3a04d584d (event-hook
activity). Develop was merged in at 3dd3c0f6a and 75663bb95.

- Domain and engine: `in-review` replaces `awaiting` (old records fold as
  in-review) and `dropped` is new; `wrapped` (done or dropped) gates
  handoff, open, request, hide, notices, and reopen. An org superior of
  the assignee writes blocked, done, or dropped on the assignment through
  `item.status` with `assignee`; the assigner of a record drops it. Confirm
  and reject belong to the assigner or an org superior of the assignee,
  on an in-review record only. Item status rolls dropped out (all dropped
  reads dropped); counts leave dropped out; rank puts in-review and done
  after open work and dropped last. reasons.json gains 12 cases and a new
  `wrap-up-statuses` golden case; the engine and Go agree.
- Writers: MCP `tk:item.status` takes `assignee` and the full enum, the
  completion default follows Decision 6 by the writer's reach (split per
  class), and the query takes `status:in-review`, `status:dropped`, and
  `status:awaiting` as an alias. The dashboard has the superior's status
  menu, the review actions, the dropped filter, and the quick-add chip;
  every i18n table holds the new keys.
- Event hook: activity writes `in-review` for a close the assigner
  confirms, names the assignee of a status written on another member's
  assignment as `target`, and carries `dropped` under the `status` kind;
  due_soon skips dropped assignments. The hook had not shipped in a
  release (f931dda48 is on develop only), so `v` stays 1.

Verification: `ws-rgo test -count=1` on ./internal/wstracker/,
./internal/wstrackerhook/, ./internal/wstrackerhost/, ./internal/mcp/,
./internal/wsrsrc/, ./internal/wsdashboard/, ./internal/wsi18n/,
./cmd/ws-mcp/ all ok after the final merge; vet clean on wstracker,
wstrackerhook, mcp, and wsdashboard; `gen-golden.js` reports 269
reasons.json cases agreeing with the engine and leaves the tree clean. A
browser check passed 76/76 on the superior menu, review actions, dropped
filter, and quick-add chip.

Decisions taken in implementation:

- The assigner drops a record under either completion mode, which is
  wider than Decision 4's assigner-completion wording; the assigner made
  the assignment and dropping it is their wrap-up either way.
- The confirm list stays the assigner's own records; a superior confirms
  from the item.
- A dropped assignment raises a notice like done and blocked.
- An assignee may close from a voluntary in-review.
- A self-rooted rule counts as assigner-class for the completion default.
- The MCP writer reads the viewer's reach before the batch, so a
  `set_lead` in the same batch does not change that batch's defaults.

Left open:

- A split assignment whose targets all already hold the item is refused
  rather than skipped.
- The Today confirm chip opens the "assigned by me" list, which omits
  self records.
- Not browser-checked: kanban drag into in-review or dropped, team counts,
  non-English locales, and the fallback superior menu.
- `TestClientTableHoldsOnlyClientKeysWithEnglishFallback` depends on test
  order (preexisting).
