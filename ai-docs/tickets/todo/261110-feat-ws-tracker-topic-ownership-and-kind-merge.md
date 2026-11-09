---
title: "Tracker: topics take an ownership assignment; task and todo merge into one kind"
related:
  261110-feat-ws-tracker-item-move-reparent: "idea; its kind constraints must follow this ticket's kinds"
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
   can be returned or handed off, and shows in the holder's "mine" while
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
   `todo` stays in `kinds` (`state.go:12`) and in the MCP create enum
   (`tracker_tools.go:220`) so old events replay unflagged and an agent that
   still sends `todo` succeeds; the query filter enum (`tracker_tools.go:1005`),
   the dashboard kind pickers (`tracker.js:1608`, `:3275`), and tool
   descriptions drop it. Rejected: view-time mapping only (every filter and
   check would need it); rejecting `todo` on input (flags old events on
   replay).
4. **One holder per topic (lead, assumed).** A topic assignment names exactly
   one member through `mode: "chosen"` with one person; subtree rules and
   multi-person assignment are refused on a topic with a reason in the
   flagged-reasons table. Rationale: "who looks after this" is one person;
   rule fan-out (`applyAssign` `state.go:1347`, `extendRule` `:638-651`,
   `refreshLive` `:654`) would create several holders. Rejected: allowing
   rules (no reading of ownership needs it, and it multiplies the rows the
   views must exclude).
5. **Topic records carry no status (lead, assumed).** On a topic,
   `item.status`, `item.close` of a record, and completion modes are refused
   ("topics have no status"); `itemStatus` keeps returning "" for a topic
   (`view.go:364-366`) and `counted()` keeps excluding it (`view.go:419-421`).
   Accept, decline, handoff, and return work unchanged (they already go
   through `needRec` / `openRec` / `pendingRec`), so a handoff still needs the
   receiver's accept and a return still follows its existing rule.
6. **Where a held topic shows (lead, assumed).** It shows in the holder's
   `my.assignments` ("mine") with the topic chip instead of a status; a
   pending handoff shows in the receiver's received list and badge like any
   pending assignment. It stays out of every status-based list and aggregate:
   `my.today.work`, `team.open`, the assignment-scope kanban (`tracker.js`
   `assignRecs` 1650-1659, 1678-1685, which has no kind filter today), due and
   gantt (already excluded at 1736, 1756). `my.created` keeps excluding topics.
7. **Auto-assign on create covers topics on both writers (consequence of
   1).** The dashboard's `selfAssigned` (`tracker.js:3310-3316`) drops its
   `kind !== 'topic'` condition (the review-column create `createKnew` stays
   unassigned, as today), and the MCP `writerDefaults`
   (`tracker_tools.go:710-760`) includes topics at line 744 (mockup decision
   66a). The holder of a topic can then create children under it through
   the existing live-assignment rule (`check.go:355`).
8. **Existing topics are not backfilled (lead, assumed).** Topics created
   before this change stay unheld; their creator can assign them later. No
   ledger write happens on upgrade.
9. **The mockup engine moves in lockstep.** The Go fold is pinned against
   golden files that `internal/wstracker/testdata/gen-golden.js` writes from
   the mockup engine (`internal/wstracker/golden_test.go:16`), so
   `ai-docs/ref/tracker-mockup/engine.js` (kind check :431, topic assign
   :473, status :1115, counted :1137, created :1281) and its README decision
   rows (522, 525, 795, 929, 1042, 1048 and the prose at 76-77, 264) change
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
  topics take no assignment are updated (and mirrored per
  `ai-docs/manuals/wsflow-mirroring.md` if the playbook text changes).
- Never touch the user's dashboard daemons on ports 29417 / 45743; manual
  checks use a dev daemon with isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME`.

## Phases

### Phase 1: Topic ownership and the task/todo merge

- `internal/wstracker`: `todo` -> `task` at the create fold; lift the
  check.go:438 refusal for a single chosen holder; refuse rules,
  multi-person, status, record close, and completion on topics with reasons;
  view lists per Decision 6.
- Mockup engine and README in lockstep (Decision 9); regenerate goldens with
  `gen-golden.js`; update `testdata/reasons.json` (the "assign topic" case
  at :54 flips; new refusal cases added).
- `internal/mcp`: create enum keeps `todo`, filter enum and descriptions drop
  it; `writerDefaults` self-assigns topics; tests at
  `tracker_tools_test.go:452`, `TestTrackerUpdateSelfAssignsCreates`
  (~755-790), and fixtures that create `todo` updated.
- Dashboard `tracker.js`: kind pickers without `todo`; `selfAssigned` covers
  topics; held topics in "mine" with the topic chip; assignment-scope
  kanban skips topic records; topic property card and assignees panel show
  the holder and the return / hand-off actions.
- Tests: Go tests for each refusal and for a held topic appearing in
  `my.assignments` but not `my.today.work`, `team.open`, or counts; a replay
  of a ledger holding `todo` creates yields `task` with no flagged event;
  dashboard tests where a harness exists for the touched paths.
- Verify in a headless browser against a dev daemon: create a topic (held by
  the creator, in "mine", not in kanban), hand it off and accept, return it;
  an old `todo` item shows as a task.
