---
title: "Tracker: topics take an ownership assignment; task and todo merge into one kind"
related:
  261110-feat-ws-tracker-item-move-reparent: "idea; its kind constraints must follow this ticket's kinds"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f4a2ce2a3ef89bee
sage-review-completeness-reviewed: f4a2ce2a3ef89bee
completed: 2026-11-10
---

# Tracker: topics take an ownership assignment; task and todo merge into one kind

## Background

User questions on the 2026-11-09 board, topic "트래커: 내가 만든 주제":

- A topic the user creates from the dashboard appears nowhere in "mine":
  topics refuse assignment (`agents-plugin-tool/internal/wstracker/check.go:438-440`,
  "topics have no assignment"; mockup README decision 529 auto-assigns only
  dashboard-created task/todo to the creator), and the "created by me and
  not assigned" list excludes topics (`internal/wstracker/view.go:853`,
  `it.Kind != "topic"`). The user would rather topics carry an assignment,
  "by default assigned to me, so I can release it".
- "What is the difference between task and todo?" Nothing in the code
  distinguishes them: the only kind branches test `== "topic"`
  (check.go:195, :344, :438; view.go:365, :420, :853); the MCP self-assign
  rule treats task and todo alike (`internal/mcp/tracker_tools.go:743-744`).

All paths below are under `agents-plugin-tool/` unless they start with
`ai-docs/`.

## Decisions

1. **Topics take an ownership-only assignment (board 153 b, user).** A topic
   can be held by a member: it shows who looks after the topic, with no
   status and no completion. It is auto-assigned to its creator on create,
   can be released or handed off (Decision 5a), and shows in the holder's "mine" while
   held. It is not counted in kanban, due, or progress aggregates. This
   reverses mockup decision 529's "topics are not assigned" and the
   check.go:438 refusal. Supersedes board 151 a (show created topics under
   "created by me"), which the user's 153 answer replaced.
2. **Task and todo merge into one kind; the `kind` field stays (board 154
   a, user).** The user: the split "only confuses"; keep the framework so a
   subcategory can be added later. Existing `todo` items display and filter
   as `task`.
3. **Normalize `todo` to `task` in the fold, keep `todo` valid on input
   (lead, assumed).** `kind` is stored only in `item.create` event payloads
   (`internal/wstracker/payload.go:64`, read at `state.go:985` on every
   replay), so no ledger migration is needed. The fold stores `task` for a
   `todo` create, so the MCP query filter (`internal/mcp/tracker_render.go:171`
   exact match), dashboard filters, and every kind check see one value.
   The dashboard event history maps a raw `todo` create payload to `task`
   too (`kindName(p.kind)` `tracker.js:419`, which falls back to the raw
   string at :308 once the `todo` key is dropped), as does any MCP renderer
   that echoes a raw create payload.
   `todo` stays in `kinds` (`state.go:12`) and in the MCP create enum
   (`tracker_tools.go:220`) so old events replay unflagged and an agent that
   still sends `todo` succeeds; the query filter enum (`tracker_tools.go:1005`),
   the dashboard kind pickers (`tracker.js:1608`, `:3275`), and tool
   descriptions drop it. Rejected: view-time mapping only (every filter and
   check would need it); rejecting `todo` on input (flags old events on
   replay).
4. **One holder per topic (lead, assumed).** A topic `item.assign` is
   accepted only when it names exactly one person, in either payload form:
   `targets: [<person id>]`, or `mode: "chosen"` with `people` of length one
   (the form `selfAssigned` `tracker.js:3313`, the MCP self-assign
   `tracker_tools.go:747-750`, and `buildItemAssign` `:322-355` for a
   people-only assign send). `mode: "all"`, a rule object, or more than one
   entry is refused on a topic with a reason in the flagged-reasons table. A
   topic `item.assign` is also refused while the topic has any live or
   pending record, a pending handoff record included ("unassign <holder>
   first"); `item.handoff` is a separate event and keeps its own check.
   The payload's `completion` field is accepted and ignored on a topic (see
   Decision 5), so the existing writers keep their payload shape: the
   dashboard assign dialog (`tracker.js:3125`, `asgCompl` :2999-3002),
   `selfAssigned` (:3313), and the MCP `writerDefaults` (`tracker_tools.go:722-735`)
   need no topic-specific completion handling. Rationale: "who looks after
   this" is one person; rule fan-out (`applyAssign` `state.go:1347`,
   `extendRule` `:638-651`, `refreshLive` `:654`) or a second assign
   (`check.go:470-475`, `assignPerson` `state.go:682` skips only people who
   already hold a record) would create several holders. Rejected: allowing
   rules (no reading of ownership needs it, and it multiplies the rows the
   views must exclude); refusing an explicit `completion` (every existing
   writer sends one, and `writerDefaults` would need the folded kind of an
   existing item id to omit it).
5. **Topic records carry no status (lead, assumed).** On a topic,
   `item.status` and `item.close` of a record are refused ("topics have no
   status"), and a topic record stores no completion mode whatever the
   assign payload carried; `itemStatus` keeps returning "" for a topic
   (`view.go:364-366`) and `counted()` keeps excluding it (`view.go:419-421`).
   Accept, decline, handoff, and `item.return` work unchanged (they already
   go through `needRec` / `openRec` / `pendingRec`): a handoff still needs the
   receiver's accept, and `item.return` stays the holder's lead's action
   under its existing rule (`check.go:552-580`).
5a. **The holder releases a topic with `item.unassign` (lead, assumed).** The
   user's "I could release it" is `item.unassign` of the holder's own record.
   Today only the assigner may unassign (`check.go:600-610`), and a holder who
   accepted a handoff keeps the original assigner as `by` (`state.go:1077`).
   On a topic, the current holder may also unassign their own record, so the
   creator and a handoff receiver can both release it. Rejected: holder-facing
   `item.return` (lead-only, and a forced self-assign is not returnable);
   leaving release to the assigner only (a handoff receiver could only hand
   the topic off again, contrary to the ownership reading).
6. **Where a held topic shows (lead, assumed).** It shows in the holder's
   `my.assignments` ("mine") with the topic chip instead of a status; a
   pending handoff shows in the receiver's received list and badge like any
   pending assignment. It stays out of every status-based list and aggregate:
   `my.today.work`, `team.open`, the assignment-scope kanban (`tracker.js`
   `assignRecs` 1650-1659, 1678-1685, which has no kind filter today), due and
   gantt (already excluded at 1736, 1756), the team walk's `Pending` / `Done`
   counts (`view.go:917-930`), and the forced-assignment badge
   (`view.go:947`; a topic carries no work to start). `my.created` keeps
   excluding topics. The dashboard status controls (`canStatus` / `stBtn`
   `tracker.js:612`, `:621`) and the kanban drop path (`:3504`) offer no
   status change on a topic, which they would otherwise do once the topic
   holds a live record.
7. **Auto-assign on create covers topics on both writers (consequence of
   1).** The dashboard's `selfAssigned` (`tracker.js:3310-3316`) drops its
   `kind !== 'topic'` condition (the review-column create `createKnew` stays
   unassigned, as today), and the MCP `writerDefaults`
   (`tracker_tools.go:705-773`) includes topics at line 744 (mockup decision
   66a). The holder of a topic can then create children under it through
   the existing live-assignment rule (`check.go:362`, the parent `liveRec` check).
8. **Existing topics are not backfilled (lead, assumed).** Topics created
   before this change stay unheld; their creator can assign them later. No
   ledger write happens on upgrade.
9. **The mockup engine moves in lockstep.** The Go fold is pinned against
   golden files that `internal/wstracker/testdata/gen-golden.js` writes from
   the mockup engine (`internal/wstracker/golden_test.go:16`), so
   `ai-docs/ref/tracker-mockup/engine.js` (kind check :431, topic assign
   :473, status :1115, counted :1137, created :1281) and its README decision
   lines (522, 525, 795, 929, 1042 = decision 529, 1048 = decision 66 a, and the
   prose at lines 76-77, 264; all README line numbers, not decision IDs) change
   with the Go code, and the goldens are regenerated.

Answer to the user's 153 note ("when a task under a topic is assigned to
someone, is the parent topic shown with it?"): yes, today. The dashboard's
`ctxTree` (`tracker.js:875-897`) walks each matched row's parent chain and
draws non-matching ancestors as greyed context rows, so the assignee sees the
parent topic above the task in "mine". This ticket keeps that.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` before editing
  (`agents-plugin-tool/`) and `ai-docs/manuals/ws-mcp.md` before editing
  `internal/mcp/`.
- Old ledgers replay unchanged: no event may become flagged because of this
  change, and no migration write.
- i18n: drop `js.tk.kind.todo` from the dashboard UI; every new or changed
  user-facing string gets its English key and the key in every other table
  (`TestOtherTablesHoldEveryEnglishKey`). Existing topic strings that say
  "discussion only" (`js.tk.prop.topic`, ko "주제: 논의 전용") are reworded to
  mention ownership.
- MCP tool descriptions and the tracker playbook text that list kinds or say
  topics take no assignment are updated. The playbook has three copies, all
  edited together: `agents-plugin/rsrc/tracker/tracker.md` (kinds at :7,
  topic assignment at :120), `agents-plugin-wsflow/rsrc/tracker/tracker.md`,
  and `agents-plugin-pi/rsrc/tracker/tracker.md`.
- Never touch the user's dashboard daemons on ports 29417 / 45743; manual
  checks use a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)

## Prior Decisions

- a0c84960 (2026-11-10, commit): "User board answers 2026-11-09: (153) b topics take an ownership-only assignment; (154) a merge task/todo, keep the kind field. (151) a superseded by (153)." — bearing: supports
- 261109-feat-ws-tracker-review-stage-and-assignment-defaults (2026-11-09, Decisions): "Agents self-assign what they create (user; board (66) a): when an agent creates a task or todo through the MCP tracker tool and the same batch carries no item.assign for it, the MCP writer adds the self-assignment" — bearing: constrains
- e2bdce49 (2026-11-08, commit): "Self-assign also applies to the Subtasks '+' / inline subtask row. This supersedes (474)'s 'unassigned task', since (529) says any task or todo created from the dashboard; recorded as README row 529." — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Every flagged reason has a reasons.json case checked against the engine by the generator and against Go by TestFlaggedReasons, so the strings cannot drift between the two." — bearing: constrains
- 4449ed91 (2026-11-08, commit): "Verdicts fold only the event's thread-local causal past (roles, identity, the item's thread and comments, assigns as live-rule triggers) instead of the whole past; TestVerdictsEqualFullReplay pins that shortcut" — bearing: constrains
- e11589f5 (2026-11-08, commit): "Accept keeps the assigner and completion mode, which is how (214) 'requester confirms' falls out; v2 made acceptance a self-assignment." — bearing: constrains
- 166fd45f (2026-11-08, commit): "A fresh-reader audit of the playbook added the assignment lifecycle, kind meanings, lead tree, refusal end state, and register profile rule." — bearing: constrains
- 261107-epic-ws-personal-common-store (2026-11-07, Cross-Child Decisions): "Assignment (user, board (128) c, (129) a, (125) c): An assigner targets either everyone within N levels below them, or chosen people within that range." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wstracker/check.go, view.go, state.go, testdata/reasons.json, goldens; internal/mcp/tracker_tools.go, tracker_tools_test.go; internal/wsdashboard/assets/tracker.js; internal/wsi18n/tables/*.json; ai-docs/ref/tracker-mockup/engine.js, README.md; agents-plugin/rsrc/tracker/tracker.md and mirrors |
| scope.surface | cross-module | wstracker fold and checks, MCP tracker tool enums and writerDefaults, dashboard tracker.js, i18n tables, mockup engine, shipped tracker playbook |
| scope.new_public_symbol | no | none; new refusal reason strings only |
| scope.new_type_contract | no | kind field and payload shape unchanged per Decision 2; behavior of topic item.assign and the todo fold changes |
| scope.test_surface | existing | internal/wstracker/testdata/reasons.json, golden_test.go, internal/mcp/tracker_tools_test.go#L452 and TestTrackerUpdateSelfAssignsCreates L756, internal/wsi18n/keys_test.go TestOtherTablesHoldEveryEnglishKey |
| complexity.reuse_points | confirmed | needRec check.go:305, liveRec/pendingRec/openRec state.go:488-510, ctxTree tracker.js:875-897, gen-golden.js |
| complexity.side_effect_risk | high | fold change applies on every ledger replay and to Go and mockup goldens; old ledgers must replay unflagged |
| risk.correctness | high | create-fold normalization plus new topic assign refusals across Go fold, mockup engine, and view lists must stay in lockstep |
| risk.fit | moderate | reverses mockup decision 529 topic rule and touches several view lists and the kanban that lack kind filters |
| risk.test | moderate | Go goldens and reasons cover the fold, but dashboard paths rely on headless browser checks |
| risk.security_or_contract | moderate | ledger replay compatibility and MCP query filter enum drop todo for agent callers |

## Phases

### Phase 1: Topic ownership and the task/todo merge

- `internal/wstracker`: `todo` -> `task` at the create fold; lift the
  check.go:438 refusal for a single person in either form (Decision 4);
  refuse `mode: "all"`, rules, multi-person, an assign while any live or
  pending record exists (pending handoff included), status, and
  record close on topics with reasons; store no completion on topic records;
  let the holder unassign their own topic record (Decision 5a); view lists
  and counts per Decision 6.
- Mockup engine and README in lockstep (Decision 9); regenerate goldens with
  `gen-golden.js`; update `testdata/reasons.json` (the "assign topic" case
  at :54 flips; new refusal cases added).
- `internal/mcp`: create enum keeps `todo`, filter enum and descriptions drop
  it; `writerDefaults` self-assigns topics; tests at
  `tracker_tools_test.go:452`, `TestTrackerUpdateSelfAssignsCreates`
  (~755-790), and fixtures that create `todo` updated.
- Dashboard `tracker.js`: kind pickers without `todo`; `selfAssigned` covers
  topics; held topics in "mine" with the topic chip; assignment-scope
  kanban skips topic records; no status controls on topics (Decision 6
  sites); event history maps `todo` creates to `task`; topic property card
  and assignees panel show the holder, the holder's release (unassign) and
  hand-off actions, and no holder-facing return.
- Tests: Go tests and `reasons.json` cases for each refusal (`mode: "all"`,
  rule target, two targets, two `people`, second holder, an assign while the
  giver has unassigned but their handoff is still pending, status, record
  close), for both accepted single-person forms, for a topic assign with
  `completion` set being accepted and storing none, for a handoff receiver
  unassigning their own topic, and for a held topic appearing in
  `my.assignments` but not `my.today.work`, `team.open`, team counts, or the
  forced badge; a replay
  of a ledger holding `todo` creates yields `task` with no flagged event;
  no tracker.js unit harness exists in `internal/wsdashboard` (only Go route
  tests), so dashboard behavior is covered by the headless-browser step
  below.
- Verify in a headless browser against a dev daemon: create a topic (held by
  the creator, in "mine", not in kanban), hand it off and accept, the
  receiver releases it (unassign);
  an old `todo` item shows as a task.

### Result (381e94371) - 2026-11-10

Landed in c536206ac (Go fold, mockup engine and README, goldens,
reasons.json, `topic_test.go`), e7944d145 (MCP writerDefaults and kind
filter, tracker playbook in all three copies), c1ade9b13 (dashboard
`tracker.js`, i18n tables, regenerated mockup `i18n-en.js`), and 381e94371
(playbook bullet order fix from review).

- Fold: a `todo` create folds as `task`; `todo` stays valid on input and in
  the MCP create enum, the query kind filter is `task`/`topic`. A topic
  assign takes exactly one person (`targets: [id]` or `chosen` with one
  person), refused otherwise ("a topic has one holder: assign it to exactly
  one person") and while any live or pending record exists ("topic <alias>
  is already held by <ids>; unassign <first> first"); status and close are
  refused ("topics have no status"); a topic record stores completion `""`;
  the topic's live holder may unassign their own record.
- Views: a held topic is in `my.assignments` (and pending: received,
  today.pending, `p:` badge), out of `today.work`, the `a:` badge, and every
  `team` row list and count (open, pending, waiting, done, last).
- Dashboard: kind pickers task/topic; event history maps raw `todo` to task;
  `selfAssigned` covers topics; topic chip instead of status in lists; no
  status controls or kanban drop on topics; assignment kanban and due skip
  topic records; topic window shows the holder, Release
  (`item.unassign {assignee: self}`) and Hand off, no holder-facing return;
  the assign popover on a topic is single-pick, people only, offered to the
  creator only while unheld. `js.tk.kind.todo` removed; `js.tk.prop.topic`
  and `js.tk.st.topic.desc` reworded; six new keys in all 11 tables.

Verification: `go test ./... -count=1` in `agents-plugin-tool` (exit 0, all
packages ok, at c1ade9b13); `node testdata/gen-golden.js` reports
"reasons.json: 169 cases agree with the engine"; `go test ./internal/wsrsrc/...`
ok after 381e94371; `python3 -m unittest discover agents-plugin-wsflow/tests`
ok. Headless browser (two dev daemons on ports 39511/39512, isolated
`WS_CACHE_HOME`/`WS_CONFIG_HOME`, shared bare remote): an old `todo` item
shows as task; a created topic is held by its creator, in "mine" with the
topic chip, absent from the kanban; handed off and accepted by the second
member; the receiver released it; the creator reassigned it. Lite review:
clean, two minors (one fixed in 381e94371, one recorded below).

Decisions:
- Topic records keep the internal assignment status `todo` (an empty status
  ranks differently in the engine's and Go's `my.assignments` sort);
  completion is stored as `""` in both engines, so the Go JSON shape is
  unchanged.
- The team walk drops topic records from every list and count, not only
  Pending/Done (Decision 6: every status-based aggregate).
- The planned test "an assign while the giver has unassigned but their
  handoff is still pending" is unreachable: ending a record withdraws its
  pending handoff (295). reasons.json pins the assign refused while the
  holder's handoff is pending and while an initial assign is pending.
- Accepted limit: the one-holder rule is causal. Two concurrent assigns of
  one topic (the creator on two clones) are each checked against their own
  past and both apply, leaving two holders; views render a holder list, so
  nothing breaks, and either can be unassigned.
- Dashboard: the minus badge stays only for a creator whose topic someone
  else holds (Release covers the holder); the "mine" chip count and the
  page feed's forced/due entries skip held topics; new key
  `js.tk.ev.assign.topic` so a topic assign line carries no completion text;
  a topic still gets no dashboard subtask row (unchanged).
