---
title: "A requested compaction runs before the user's queued follow-up"
related:
  261112-bug-ws-claude-mod-dashboard-message-during-compaction-submits-as-follow-up: "built the compacting hold and the compactEnded release after continuation() that the mod side extends"
  261018-feat-claude-mod-mid-turn-steer-delivery: "steer vs queued split; its turn-end mail submit stays out of this hold"
  261020-feat-claude-mod-lever-continue-flag: "the resume prompt, CONTINUATION_WINDOW_MS, and turn-start cancel that fix the continue-order divergence (Decision 7)"
  261002-feat-pi-lead-ws-owned-compaction: "out-of-scope boundary: Pi's own ws-compact lever is not changed here"
  261018-research-claude-mod-mid-turn-steer: "background research for the steer/follow-up distinction"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 08521715e5e0c995
sage-review-completeness-reviewed: 08521715e5e0c995
completed: 2026-11-14
---

# A requested compaction runs before the user's queued follow-up

## Background

Investigation by the MAIN lead, 2026-11-13 (owner answers (512)=b, (513)=ok on
MAIN's board).

When a lead calls the compaction lever (`*__request_compaction`) and the user
has queued a follow-up (not a steer), the follow-up is delivered at turn end
before the compaction starts, so the follow-up turn runs first and the
compaction is pushed back.

- claude-mod: `agents-plugin/hooks/claude-mod/compaction.ts` waits
  `SETTLE_MS = 1000` (`:78`) and runs only when no main-thread turn is
  running, otherwise reschedules at the next turn end. Queued board messages
  are released at turn end by `turnCompleted` -> `releaseBoard`
  (`watcher.ts:1225`, `:1270`, `releaseBoard` at `:703`).
- Hosted (daemon): `wsagent/runner.go` `kick()` (`:1734`) delivers queued
  input right at turn end. `claude.go` parses the stream-json, and the fixture
  `agents-plugin-tool/internal/wsagent/testdata/claude-lead-compaction.jsonl`
  already contains a `*__request_compaction` call.
- In hosted mode (`register.ts:114`, `WS_HOSTED_SESSION*`) the mod's input
  delivery is disabled, so the mod change applies only to non-hosted
  sessions.

## Decisions

1. **Scope: hosted sessions and claude-mod sessions' dashboard-delivered
   (board) messages** ((512)=b, owner 2026-11-13). Pi sessions are out of
   scope ((512)=b did not include them; Pi's own `ws-compact` lever, from
   `261002-feat-pi-lead-ws-owned-compaction`, is not changed here).
2. **Queued follow-ups are held at most 5 s** ((513)=ok, owner 2026-11-13):
   after a successful lever request in the turn, queued follow-ups wait until
   the compaction starts or 5 s pass, whichever comes first.
3. **Hosted mechanism** (MAIN design, technical): the daemon detects a
   successful `*__request_compaction` tool_use / tool_result pair in the
   turn's stream-json, and `kick()` holds the queued follow-ups until the
   compaction starts or the 5 s cap. A compaction is its own turn, so `kick()`
   already waits through it. With `continue`, the lever's resume prompt is
   cancelled when another turn starts within `CONTINUATION_WINDOW_MS = 1500`
   (`compaction.ts:71`), so no duplicate turn results. Rejected: delivering
   the follow-up first (today's behaviour; the compaction the lead asked for
   is delayed by a whole turn).
   - **Hold scope: the whole `kick()`** (reviewer, technical). `kick()`
     (`runner.go:1734`) submits held inbox messages together with
     `rec.Mail`, `rec.Reports`, and `rec.Queued` in one turn
     (`runner.go:1739-1743`). While the hold is armed the whole `kick()` is
     gated, mail and child reports included: a mail-only turn would push the
     compaction back just the same. Decision 1's "hosted sessions" scope
     covers this. The asymmetry with the mod side is deliberate: there,
     mod-queued mail keeps its turn-end submit (see Constraints).
   - **Compaction-start signal** (reviewer, technical): `claude.go:827-828`
     maps a `status: compacting` system line to `Event{TurnStart}` plus a
     `wslive.KindCompact` / `wslive.CompactStart` live event. The hold clears
     at that event. The `TurnStart` sets the running turn, so `kick()`
     already returns early (`runner.go:1735`) until the compaction turn
     ends.
   - **Lever detection** (reviewer, technical): the mapper pairs the
     `tool_use` named `*__request_compaction` with its `tool_result` by
     `tool_use_id` (fixture `claude-lead-compaction.jsonl` lines 29-30). The
     call counts only if the result is not an error and the turn's `result`
     is not an interrupt. An interrupted turn ends with an `aborted*`
     terminal state, which `claudeResultError` maps to no error
     (`claude.go:954-958`). So the mapper, or the runner when it issued the
     interrupt itself, must tell that case apart: an interrupted lever turn
     does not arm the hold.
   - **Cap timer** (reviewer, technical): the 5 s cap is a named Go constant
     (e.g. `leverHoldCap = 5 * time.Second`). It is a timer the conversation
     loop selects on, the way `actionTimer` is (`runner.go:1397-1431`); its
     expiry clears the hold and calls `kick()`. The timer source is
     injectable, beside `Options.Now` (`runner.go:122-123`), so tests drive
     the cap without real time.
4. **Mod mechanism** (MAIN design, technical): in `watcher.ts`, while a
   lever request is pending, `turnCompleted` does not release held board
   messages; the existing `compactStarted` / `compactEnded` path releases
   them after `continuation()`. The same 5 s cap applies.
   - **Pending-lever predicate** (reviewer, technical): add a new optional
     watcher edge, set in `register.ts` beside `continuation`
     (`register.ts:1124-1125`), e.g. `leverPending?: () => boolean`, backed
     by `compaction.ts`. It is true only while the compact step is still
     owed: `request?.compact` holds (`compaction.ts:192`, `:557-558`) and no
     aborted turn parked the request. An aborted turn leaves the request
     pending without scheduling the step (`compaction.ts:505`, `:523`). A
     compaction that already landed (`compact` false) or an interrupted
     lever turn does not arm the hold. Note the order: `register.ts:1754`
     calls `watcher.turnCompleted()` before `compaction.turnComplete(...)`
     at `:1765`. The predicate is therefore read before this turn end
     schedules the step, and it must take the ending turn's abort into
     account itself (e.g. the edge closes over the `turn.complete` reason),
     not infer it from the step's schedule. The hold arms once per lever
     request, at the end of the turn that carried the call. A later turn
     end for the same still-owed request releases as today.
   - **Cap release safety** (reviewer, technical): `compactStarted` cancels
     the cap timer. When the cap fires, the release re-checks `compacting`
     and `edges.turnRunning()`, as `compactEnded` does
     (`watcher.ts:1301-1307`). If a turn started in the meantime (e.g.
     mod-queued mail submitted at the lever turn's end), the board messages
     stay held for that turn's end and are not submitted into a running
     compaction or turn. The cap wait is an `edges.sleep` raced outside the
     board chain, like the turn-end `BOARD_FIRST_WAIT_MS` race at
     `watcher.ts:1272-1278`. It is never awaited inside `boardChain`
     (`watcher.ts:437`; c4e68fc3e).
   - **Settle-gap wakes** (reviewer, technical): a board message that wakes
     the wait in the ~1 s `SETTLE_MS` gap (`compaction.ts:78`) between the
     lever turn's end and the compaction start would submit at once today:
     `boardBusy()` (`watcher.ts:482-484`) reads only `compacting` and
     `turnRunning()`. `boardBusy()` therefore also reads the armed hold, and
     a turn end that holds sets `boardHeld`. Such a message is held under the
     same 5 s cap.
5. **Compatibility tier: T0** (announced by MAIN): daemon-internal delivery
   timing and mod hook behavior; no tool, argument, store, protocol, or
   tracker event change.
6. **Cap anchor and constants** (reviewer, technical; refines Decision 2
   without changing it). On both sides, the 5 s is measured from the end of
   the turn that carried the successful lever call. Mod: a new named
   constant in `watcher.ts` (e.g. `LEVER_HOLD_MS = 5_000`).
   `BOARD_FIRST_WAIT_MS` (`watcher.ts:261`) is not reused: it bounds how
   long turn-end mail waits for the board drain, a different wait.
   `COMPACTION_CONTINUATION_WAIT_MS` (`:265`) stays the bound after a
   compaction ends. Hosted: the named Go constant from Decision 3.
7. **Order under `continue` diverges, deliberately** (reviewer, technical).
   - Mod: the resume prompt runs first and the held board message follows,
     because `compactEnded` waits on `continuation()` before releasing
     (`watcher.ts:1287-1308`, `compaction.ts:566`).
   - Hosted: the daemon has no view of the mod's continuation. The held
     follow-up is delivered when the compaction turn ends, and its turn
     start cancels the held resume (`compaction.ts:71`, 261020 Result).
   - Each side keeps its own order. Tests pin both.

## Constraints

- A steer (input that interrupts the running turn) is not held; only queued
  follow-ups are.
- Mailbox mail the mod queues and submits at turn end is out of scope and
  keeps today's turn-end submit (Decision 1 scopes the mod side to board
  messages; `261018-feat-claude-mod-mid-turn-steer-delivery` recorded "mail
  still queued at turn end is submitted as today"). The pending-lever hold
  in Decision 4 gates only the board release. On the hosted side, the hold
  gates all of `kick()` (Decision 3); the asymmetry is deliberate.
- An armed hold never outlasts its cap. Each side has exactly three release
  paths: compaction start, cap expiry, or a non-arming outcome (no lever
  call, a failed call, an interrupted lever turn, or a compaction that
  already landed). Turn-end delivery stays unchanged otherwise.
- Mirror the mod change into `agents-plugin-wsflow/` per
  `ai-docs/manuals/wsflow-mirroring.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261112-bug-ws-claude-mod-dashboard-message-during-compaction-submits-as-follow-up (2026-11-12, owner answer): "User answered (378)=a: a board message sent while the main conversation compacts is held like during a running turn and released after the compaction continuation, instead of submitting as a follow-up." — bearing: supports
- 00860d04 (2026-11-12, commit): "The release waits on a new compaction.continuation() promise rather than a fixed delay: it stays pending while the lever's awaited /compact owes a resume ... while the resume is held for CONTINUATION_WINDOW_MS" — bearing: supports
- 261030-bug-claude-mod-headless-compaction-and-captions (2026-10-30, Decisions): "Settled idle step; the lever never queues `/compact`. After a non-aborted main-thread `turn.complete`, the compact step is scheduled with `$.clock.after(1000, ...)` instead of `after(0)`." — bearing: constrains
- 261020-feat-claude-mod-lever-continue-flag (2026-10-20, Result 816801dbd): "A main-thread turn start cancels the held compaction resume (landed or vetoed)" — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Result 7fe04e8e6): "mail still queued at turn end is submitted as today" (turn-end submit of mod-queued mail stays a follow-up turn) — bearing: constrains
- 261112-feat-ws-dashboard-slash-commands (2026-11-12, c4e68fc3): "The run is not awaited inside the board chain. Awaiting it would block the turn-end flush, the compaction release, and the wait loop (mail included)" — bearing: constrains
- 17a94d08 (2026-11-13, commit): "Ending the turn at every result was rejected: ... flushing held messages at the result reopens the trailing-idle race in the common case." — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Result 2a4cfcf57): "delivery blocked while a request is pending" (lever core tests) — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/runner.go, agents-plugin-tool/internal/wsagent/claude.go, agents-plugin/hooks/claude-mod/watcher.ts, agents-plugin-wsflow/hooks/claude-mod/ mirror (byte-identical to ws today) |
| scope.surface | cross-module | Go daemon (wsagent) and the TS claude-mod change together; no exported symbol or tool surface changes (T0) |
| scope.new_public_symbol | no | none; daemon-internal delivery timing and a mod-internal hold |
| scope.new_type_contract | no | none declared; Decision 5 states no tool, argument, store, protocol, or tracker event change |
| scope.test_surface | existing | agents-plugin-tool/internal/wsagent/runner_test.go, lead_claude_test.go (TestClaudeLeadCompaction on claude-lead-compaction.jsonl), agents-plugin/hooks/claude-mod/watcher.test.ts and the wsflow mirror copy |
| complexity.reuse_points | confirmed | claude-lead-compaction.jsonl fixture, existing compacting/compactStarted/compactEnded hold in watcher.ts#L1283-L1312, BOARD_FIRST_WAIT_MS 5 s constant at watcher.ts#L261, and kick() turn-wait in runner.go#L1734 |
| complexity.side_effect_risk | moderate | kick() gates all queued delivery for every hosted backend; a hold that never clears would starve input, bounded by the 5 s cap |
| risk.correctness | high | timing hold across two implementations (Go daemon, TS mod) with a 5 s cap, steer-vs-follow-up distinction, and lever success detection from tool_use/tool_result pairing |
| risk.fit | moderate | mirrors the existing compaction-hold path in watcher.ts, but a new pending-lever state must not break the turn-end mail path or the continuation window |
| risk.test | moderate | needs time-driven tests in Go (fixture replay) and TS (engine clock) for hold and cap paths; the Go compaction fixture replay exists |
| risk.security_or_contract | low | Decision 5 declares T0 with no tool, store, protocol, or event change; only delivery timing moves |

## Phases

### Phase 1: Hold queued follow-ups behind a requested compaction

- Hosted: lever detection in the stream parser (`claude.go`) and the
  whole-`kick()` hold with its injectable cap timer in `runner.go`, per
  Decisions 3 and 6.
- Mod: the `leverPending` edge (`compaction.ts` -> `register.ts`) and the
  pending-lever hold, cap, and settle-gap hold in `watcher.ts`, per
  Decisions 4 and 6, mirrored to wsflow.

Verification:

- Hosted mapper (`lead_claude_test.go`). `TestClaudeLeadCompaction` (`:674`)
  replays only through `newClaudeMapper` (via `newLeadReplay`, `:118`). Beside
  it, add a mapper test that `claude-lead-compaction.jsonl` yields the
  lever-detected signal exactly once, for the turn carrying the
  `request_compaction` pair (lines 29-30). Also cover: a turn without the
  call yields none; an error `tool_result` yields none; an `aborted*` result
  for that turn yields none.
- Hosted runner, with the fake backend/process from `fake_test.go`
  (`fakeBackend`, `fakeProc`). Feed it events mapped from the fixture through
  `readLeadFixture` (`lead_claude_test.go:42`, the helper
  `lead_command_test.go` and `wake_test.go` already use). Use the injectable
  cap timer. Cases:
  - a follow-up queued during the lever turn is held until the compaction's
    `TurnStart` + `CompactStart`, then delivered when the compaction turn
    ends;
  - with no compaction start, it is released when the cap fires;
  - while the hold is armed, mail and child reports are held too;
  - a turn without a lever call, or an interrupted lever turn, delivers
    queued input at turn end as today.
- Mod (`watcher.test.ts`, engine clock). Cases:
  - `leverPending` true at turn end holds the board release;
  - `compactStarted` / `compactEnded` releases it after `continuation()`,
    and the cap timer does not fire after `compactStarted`;
  - the cap releases it when no compaction starts;
  - at cap expiry with a turn running, the messages stay held until that
    turn's end;
  - a board wake in the settle gap is held (`boardHeld` set);
  - an aborted lever turn, or `compact` already false, does not hold;
  - mod-queued mail still submits at turn end;
  - the cap wait does not block `boardChain`.
  Add a `compaction.ts` unit test for the predicate (true after a lever call,
  false after a landing or an aborted turn).
- Order under `continue` (Decision 7). A mod test shows the resume prompt's
  turn starting before the held board message. A hosted runner test shows
  the held follow-up delivered at the compaction turn's end.
- The claude-mod suites and the `agents-plugin-wsflow/` mirror suites pass,
  and the mirrored files stay byte-identical.

### Result (02baa619f) - 2026-11-14

Landed in 5fdf8d007 (mod) and 02baa619f (hosted).

- Hosted: `claudeMapper` keeps a lead's `*__request_compaction` tool_use ids
  and marks the turn when the paired tool_result is not an error; the
  ending `TurnEnd` (at the result, or at the idle state on a states harness)
  carries the new `Event.CompactRequested` unless the result had an
  `aborted*` terminal reason. The runner gates all of `kick()` while
  `leverC` is armed; the hold lifts at `CompactStart`, at the cap
  (`leverHoldCap = 5 * time.Second`, from the new injectable
  `Options.Timer`), at the next non-replay turn end, or at process exit. An
  interrupt the runner issued itself (`interruptAsked`) also keeps a lever
  turn from arming.
- Mod: `compaction.leverPending(reason)` (true for the turn that carried the
  call while the compact step is owed; false for an aborted turn, after
  that turn end, after a landing), wired in `register.ts` as the watcher's
  `leverPending` edge over `mainTurnEndReason`. `watcher.ts` adds
  `LEVER_HOLD_MS = 5_000` and a `leverHold` that `boardBusy()` reads; a
  holding turn end sets `boardHeld`, mail still submits, and the cap sleep
  is raced outside `boardChain` and re-checks `compacting`,
  `turnRunning()`, and the stop generation. Mirrored byte-identically to
  `agents-plugin-wsflow/`.

Verification:

- `go test -run 'TestClaudeLeadCompaction|TestLeverHold' ./internal/wsagent`:
  pass. The full `go test ./internal/wsagent/` fails only the known env-only
  TestClaudeLeadLocalSettingsCanonicalWorktree and TestPiLeadResolverAdapter.
  Disabling the `kick()` gate or the `interruptAsked` check makes the new
  runner tests fail.
- `claude plugin test agents-plugin`: 812 pass, 1 fail (engine.test.ts
  streaming delivery timed out under load; it passes run alone, 72/72).
  `claude plugin test agents-plugin-wsflow`: 813 pass. `claude plugin
  validate` passes on both. The mirrored directories are identical
  (`diff -r`).

Decisions:

- A repeat lever call clears the request's `ended` flag, so the turn
  carrying it holds again: "the turn that carried the call" applies per call.
- Every main-thread turn end lifts an earlier hold before deciding again, on
  both sides. This is the "non-arming outcome" release path, and it keeps a
  hold from outliving the turn after the lever turn.
- The lever is paired by a mapper map of tool_use ids, not
  `batchTracker.callName`, which sees only the current batch.
- The hosted runner tests drive the real mapper through the lead harness,
  using lines from `claude-lead-compaction.jsonl`, instead of the plain fake
  process.
