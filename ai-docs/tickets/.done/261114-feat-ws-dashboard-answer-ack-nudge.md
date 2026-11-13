---
title: "Dashboard answer-ack nudge: one follow-up turn naming answered board items still open after the agent's turn, and note-only sent marks that clear at turn settle"
related:
  261111-bug-ws-whiteboard-title-patch-reask: "built the re-ask rule (ReaskedAt / ReaskTimes) that drops sent marks older than a re-ask; this ticket reuses it for the nudge candidates"
  261113-feat-ws-dashboard-browser-pane: "also edits wsinbox (Entry fields, Merge); this ticket deliberately adds no wsinbox.Entry field, so the two do not collide"
  261113-feat-ws-compaction-before-queued-followup: "holds queued follow-ups up to 5 s at a turn end; the nudge's nothing-held gate must skip that window, not fire into it"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 817154cceffbcadb
sage-review-completeness-reviewed: 817154cceffbcadb
completed: 2026-11-14
---

# Dashboard answer-ack nudge: one follow-up turn naming answered board items still open after the agent's turn, and note-only sent marks that clear at turn settle

## Background

This is the owner's idea, from MAIN's board, topic `answer-ack-nudge`
(2026-11-13). The owner answers decision items on the dashboard board, and
the agent then works. When the agent's turn settles, some answers from that
batch may still have their items open: the agent never recorded them. In that
case the dashboard injects one more turn that names those IDs and asks
whether the agent recorded them.

The owner and MAIN settled three points: (558), (559), and (561). See
Decisions 1-3. The owner's note on (561) is quoted there verbatim.

### What exists today (develop, read 2026-11-14)

- **Sent marks are server-derived, not client state.**
  - `sentMarks` (`wsdashboard/pageview_inbox.go:172`) builds each board
    item's mark from the session inbox's drained records and held entries.
    For each item it takes the newest chip.
  - It drops a chip sent before the item's latest re-ask
    (`pageview_inbox.go:175-185`, fed by `PageContent.Reasked`,
    `page.go:61-64`).
  - `buildSessionView` calls it (`pageview.go:553`).
  - The convlive template carries the marks as hidden
    `.sentdata i[data-id|data-answer|data-note|data-state]`
    (`assets/dashboard.tmpl:244`, the `r-convlive` define).
  - The client's `applySent` (`assets/dashboard.js#L2322-L2352`) only mirrors
    that list onto the board. It recomputes from scratch each time: it adds
    `sent-sent`/`sent-held` and a `sentmark` reading `sent · note` for a
    note-only chip.
  - The open-question pill counts a `sent-sent` item as not open
    (`wswhiteboard/assets/pill.js:32-36`). So does the tab label
    (`dashboard.js#L5352-L5366`, fed by the pill's count, and `tlHeldAsks`
    at `dashboard.js#L5385-L5391`, which reads `.sentdata`).
  - Every tab therefore shows the same marks. Drained records live on disk,
    the newest `wsinbox.DrainedCap` = 50 (`wsinbox/inbox.go:96-98`). A
    daemon restart keeps them.
- **The send path.**
  - `POST /s/<rid>/message` → `messageWrite` → `sendMessage`
    (`wsdashboard/inbox_routes.go:247`, `:344`) → `wsinbox.Send` under
    `d.inboxMu`.
  - On success, `inboxAnswered` (`inbox_routes.go:185-203`) runs the
    post-write wakes: `index.Reread`, `broker.wake`, `mailWake.Wake` (an
    idle mod session's board-inbox wait), and `agents.InboxChanged` (a
    hosted conversation's `kick`).
  - Each chip records its own send time, `Chip.SentAt`
    (`wsinbox/inbox.go:152-160`, set in `Send` at `:739-749`).
  - A hosted conversation drains the inbox when its next turn starts
    (`wsagent/runner.go:1790`, in `kick`, after the held check at `:1762`).
    The Claude mod drains it through `dashboard.inbox_drain` at its turn
    end, then submits the batch as the next turn
    (`agents-plugin/hooks/claude-mod/register.ts:1072`).
- **Turn settle is already a shared, persisted signal.**
  - Both adapters feed `wslive` `turn_start`/`turn_end` events:
    - the hosted runner at `wsagent/runner.go:1892`, `:2663`, `:2695`,
      `:2943`, and `wsagent/spawn.go:340`;
    - the mod through `dashboard_intake`, applied in
      `mcp/live_snapshot.go:465`.
  - `turn_end` appends a `RingTurn` entry with `DurationMs` to the session's
    conversation ring (`wslive/state.go:289-305`). The hosted runner writes
    its ring in `wsagent/record.go:423`.
  - Each ring record has a time, `RingRecord.At`, taken from its stamped
    file name (`wslive/ring.go#L118-L123`, parsed at `#L321-L333`).
  - The snapshot's `Lead.TurnStartedAt` is absent while the lead is idle
    (`wslive/snapshot.go:281-282`). `Activity.Phase` is `idle` between
    turns (`snapshot.go:263-265`, `:96`).
  - The daemon learns of changes in three ways:
    - `sessionChanged`, the index's `OnChange` (`daemon.go:419`, `:512`);
    - `agentChanged`, the runner's write hook (`daemon.go:535`);
    - a hosted ws-mcp's `daemonLiveSlot.Publish` and `RingAppended`
      (`live_hosted.go:128`, `:149`).
  - The client cannot see every settle. The convlive region may never show
    `idle` between two turns when the next one starts at once
    (`dashboard.js#L2218-L2226`, the interrupt-badge comment).
- **Board item model.**
  - `wswhiteboard.Item.Status` is one of `open`, `answered`, `assumed`,
    `withdrawn`, or `superseded` (`wswhiteboard/store.go:43-47`).
  - `wb:answer` sets `answered`, or `assumed` with `assumed="true"`
    (`store.go:902-914`). That is the agent's act of recording an answer.
  - Re-asking an item stamps `ReaskedAt` (`store.go:143-148`, `:1070`).
  - `FormatID` renders `(%02d)` (`store.go:319-321`).
  - The daemon reads a session's board only through the ws-mcp-side
    `DashboardSessionSource` (`mcp/dashboard_page.go:50-75`). For a
    non-agent session, `agentSessions` forwards to it; an agent conversation
    has no board (`wsdashboard/agent_sessions.go:68-80`).
- **A daemon-owned file in the session directory already exists.**
  `label.json` (`wsdashboard/label_route.go:22-32`). An older daemon leaves
  it alone, and the registry prunes it with the session directory.

## Decisions

1. **The nudge** ((558)=a, owner).
   - When the agent's turn settles, compare the answers the session was sent
     with the board items' status.
   - If any sent answer's item is still open, inject one nudge turn that
     names those IDs.
   - Send at most one nudge per sent-answer batch, so the nudge cannot loop.
   - Rejected: (b) the dashboard auto-records simple answers. "Answered"
     must mean that the agent actually handled the answer.
2. **Note-only answers are excluded** ((559)=ok, MAIN technical, owner ok).
   - A chip with a note and no value (`N=^note`) never makes an item a nudge
     candidate.
   - Topic notes (`[topic]^note`) are not items and are excluded too.
   - A chip with a value and a note does count.
3. **A note-only sent mark clears when the agent's turn settles** ((561),
   MAIN technical, after the owner's note on 559). The owner's note,
   verbatim:

   > 그러면 이건 지금 프론트엔드에 sent*note로 암전되게 되어 있는데, 에이전트
   > settle 시점에 note 상태도 풀리게 하면 좋겠습니다. 아니면 answered에 ack
   > 섹션을 따로 두기? 뭐가 의미가 있을지 저도 잘 모르겠네요.

   - Once a turn that started after the note was delivered has settled, the
     item's `sent · note` dim mark goes away.
   - The item returns to "waiting on owner": the pill and the tab label count
     it again.
   - The sent note stays in the item's history:
     - The delivered message's answer capsule in the conversation keeps it
       (`ConvEntry.Answers`).
     - The drained inbox record keeps it.
     - Nothing is deleted.
   - Rejected: a separate ack section under answered.
4. **"Turn settled" means the newest `RingTurn` ring entry, read the same way
   for hosted and mod sessions** (technical).
   - The settled turn's start is `At − DurationMs` of the newest ring record
     with kind `turn`.
   - A chip counts as handled by a settled turn when
     `0 < chip.SentAt < that start`. The chip was then in the inbox before
     the turn began, and both adapters drain the inbox before or as a turn
     starts.
   - A chip held during a turn has a `SentAt` after that turn's start. It
     falls to the next settle. This covers the mod's drain at turn end and a
     hosted conversation's drain in its next `kick`.
   - The same rule drives Decision 3's mark clearing and Decision 6's
     candidates. The page and the nudge therefore agree on what "settled"
     means.
   - Accepted limitation: a steered chip is injected in the middle of the
     turn it was sent in. It counts only at the turn after that one, so its
     note mark and its nudge wait one more settle.
   - Accepted limitation: a chip with `SentAt == 0`, written before chips
     carried a send time, keeps today's behaviour. It keeps its mark and is
     never a candidate.
   - A settled turn whose `DurationMs` is 0 or missing (for example a daemon
     restart between a mod's `turn_start` and `turn_end`) has no usable
     start: the start would equal `At` and count a chip sent mid-turn too
     early. Such a turn clears no mark and is no settle for Decision 6: the
     evaluation skips without advancing `through`, and the batch waits for
     the next turn that has a duration.
   - `RingRecord.At` is the ring file's write stamp (`ring.go`
     `nextRingStamp`), while `DurationMs` comes from the event times, so the
     computed start runs late by the intake and flush latency. The
     held-message gate and the 1 s coalescing absorb this; the helper carries
     a comment saying the start is approximate.
   - Rejected:
     - Client-side edge detection on `data-phase`. The page misses a settle
       that is followed at once by the next turn (`dashboard.js#L2218-L2226`),
       and it needs an open tab.
     - In-memory tracking of `TurnStartedAt` edges in the daemon. Coalesced
       snapshot reads can miss a short turn, and a restart loses the state.
5. **The nudge is evaluated in the daemon, not in the browser** (technical).
   - One component in `wsdashboard` evaluates a session after a short
     coalescing delay (about 1 s per session). Its triggers are:
     - `sessionChanged`;
     - `agentChanged`;
     - `daemonLiveSlot.Publish`, when the snapshot changed;
     - `daemonLiveSlot.RingAppended`.
   - The index's first `Refresh` after a daemon start fires `OnChange` for
     every session. That re-checks every session after a restart.
   - Gates, cheapest first:
     - The session is indexed, has a board, and is not an agent
       conversation.
     - `adapterPresent` (`pageview.go:493`): the same gate the message
       route uses (`inbox_routes.go:174`).
     - The lead is idle: `TurnStartedAt == 0`, phase empty or `idle`, and no
       permission wait.
     - A `RingTurn` entry exists.
     - The inbox holds no message entry (`wsinbox.MessageNames` is empty).
       Held input means another turn is coming, so the batch rolls into
       that turn's settle. This also covers held board messages during the
       queued-follow-up hold in
       `261113-feat-ws-compaction-before-queued-followup` (the mod's lever
       hold keeps them undrained; the hosted `kick()` gate leaves the inbox
       intact).
     - Accepted limitation: the gate does not see a hosted conversation's
       `rec.Queued` or `rec.Mail` (`runner.go:1762-1768`), pending input
       outside the inbox. In that edge the nudge is written during the 5 s
       hold. It is benign: the hosted `kick()` is gated and the mod's
       `boardHeld` keeps the wait from submitting, so the nudge queues behind
       the compaction rather than preempting it.
   - The evaluation is state-based and idempotent, so extra triggers cost
     only reads.
   - It works with no tab open. Multiple tabs cannot double-nudge because no
     browser takes part.
   - Rejected: a client-side guard. Two tabs would each nudge, and a closed
     tab would never nudge.
6. **Batch and once-per-batch guard: a daemon-owned watermark file**
   (technical).
   - The file is `sessions/<reply-id>/ack-check.json`, a sibling of
     `label.json`. It holds `{"through": <epoch ms>}`, and Decision 8 adds
     `"interruptedAt"`.
   - The batch of an evaluation is every value chip (`Answer != ""`) in the
     drained records with `through <= SentAt < settledTurnStart`, less the
     chips the re-ask rule drops.
   - If the batch is empty, nothing is written and nothing is read from the
     board.
   - Otherwise:
     - Read the board's open items (Decision 7).
     - Set `through = settledTurnStart`.
     - Then, if any batch item is still `open`, send the nudge (Decision
       9).
   - The batch is spent whether or not a nudge goes out. Each chip is
     therefore checked at exactly one settle, and a nudge never re-asks
     about an earlier nudge's IDs.
   - First evaluation (no `ack-check.json`, or a file without `through`,
     such as one Decision 8 created with only `interruptedAt`): seed
     `through = settledTurnStart`, write it, and send no nudge, even when the
     batch would be non-empty. Without the seed, the first daemon start with
     this feature would nudge every idle session about every open item
     answered in the last `DrainedCap` drained records, including answers
     from before the feature that the agent deliberately left open. A new
     session loses nothing: its items are asked in a turn and answered after
     it.
   - Lock scope (`261110-bug-ws-dashboard-daemon-lock-scope`): read the
     ring, the drained records, and the board outside `d.inboxMu`. Then,
     under it, re-check the held-message gate and re-read the watermark;
     write the watermark and send only if `through` is unchanged since the
     unlocked read. Two evaluations cannot both send.
   - The watermark is written before the send, by temp file and rename. A
     crash between the two loses a nudge; it never doubles one.
   - Candidate items: an item counts if it is `open` on the board and any
     value chip for it is in the batch.
     - A missing item does not count. Neither does an `answered`,
       `assumed`, `withdrawn`, or `superseded` one.
     - An `assumed` item reads as handled, as in the stale-topic nudge
       (`wswhiteboard/nudge.go`).
   - Accepted limitation: if the owner re-answers an item the agent already
     recorded, the item is not open, so the re-answer is never checked.
     (558) is defined on open items.
   - Rejected:
     - A new `wsinbox.Entry` marker field on the nudge entry, with the
       nudge record as the watermark. It changes the inbox entry shape that
       `Normalize`, `Merge`, and `MergeTarget` must carry, and it collides
       with the browser-pane ticket's `wsinbox.Entry` edits. A cancelled or
       interrupted nudge would also take the watermark with it and re-nudge
       at the next settle.
     - Recognising the nudge by its text. A user can type the same text.
     - A watermark file inside the inbox directory. The inbox waits and
       drains own that directory. The session directory already holds the
       daemon's own `label.json`.
7. **Board status reaches the daemon through a new optional source
   interface** (technical).
   - Add an interface to `wsdashboard/page.go`, beside `ContactSource`:
     `BoardItemsSource { BoardItems(reg Registration) (BoardItems, bool) }`.
     `BoardItems` is `{Open map[int]bool; Reasked map[int]int64}`.
   - Implement it on `mcp.DashboardSessionSource` from `record.Whiteboard`:
     - items with `Status == wswhiteboard.StatusOpen`;
     - `ReaskTimes()`.
   - `agentSessions` forwards it, as it forwards `ReadContacts`. An agent
     conversation answers with an empty value and true.
   - A source without the interface means no nudge.
   - Rejected: reading `PageContent` through `ReadPage`. That path renders
     the whole board, and the render belongs to the page.
8. **No nudge right after a dashboard interrupt** (technical).
   - The interrupt route (`inboxWrite`, route `interrupt`) records
     `interruptedAt = now` in `ack-check.json`, under the same `d.inboxMu`.
   - An evaluation whose settled turn started at or before `interruptedAt`
     skips without advancing `through`. The owner stopped the agent, so the
     batch waits for the next turn that ends normally.
   - Rejected: nudging anyway. That would start a turn the owner just
     stopped.
   - A terminal stop is covered by Decision 11.
9. **Injection and text** (technical).
   - The nudge is a plain inbox message: `wsinbox.Send` with one `Parts`
     entry, no chips, no topics, and no images. It goes through the same
     post-write wakes as a page send. Extract the success branch of
     `inboxAnswered` into a helper that both use.
   - It reaches the agent as an ordinary user turn on both adapters.
   - It carries no chips, so it adds no sent mark. Its `SentAt` lies after
     `through`, so it can never be a candidate. It is never a new answer
     batch.
   - Text, in English, sent to the agent only, so it needs no i18n key.
     IDs come from `FormatID` in ascending order, capped at 20 with `, ...`:

     ```text
     Dashboard answer check: the owner's answers to (558), (561) reached you, but those board items are still open. Record each answer you acted on with wb:answer (the owner's words as its body); if an item should stay open, say why. This check runs once per answer batch.
     ```

   - The text names only shipped grammar (`wb:answer`). It names nothing of
     this repository.
10. **Compatibility tier: T0** (expected tier confirmed against the code).
    Evidence:
    - No MCP tool or argument removal or rename. No `/mcp` hello change. No
      tracker event. No config or rsrc manifest field.
      - `dashboard.inbox_drain` stays as it is. `dashboard_intake` gains
        one optional `reason` field on its `turn_end` event (Decision 11):
        additive. The intake decoder does not reject unknown fields (no
        `DisallowUnknownFields` in `internal/mcp` outside the config and
        worktree-profile manifests), so an older daemon ignores a newer
        mod's `reason`, and a newer daemon treats an older mod's missing
        `reason` as a normal settle.
      - `RingEntry` gains one optional `omitempty` field (Decision 11). An
        older reader ignores it; entries without it read as before.
      - Decision 4 reads the existing `RingTurn` entry and its existing
        `DurationMs`.
    - Inbox store: no change in shape. The nudge is an ordinary entry with
      `Parts` only (`wsinbox/inbox.go:164-199`). Any reader version
      delivers it through `Entry.Text()` (`inbox.go:317-344`).
    - New `ack-check.json`: an additive daemon-owned file in the session
      directory, following the `label.json` precedent
      (`label_route.go:22-32`).
      - `entryNames` lists only `entryNamePattern` names in the inbox
        directory (`inbox.go:1184-1196`).
      - The session fingerprint hashes only the ring, inbox, and drained
        names (`agent_sessions.go:182-196`).
      - So no reader sees the file. An older daemon leaves it alone, and the
        registry prunes it with the session directory.
    - `BoardItemsSource` and the `sentMarks` signature are in-binary Go
      seams. Only the one daemon build compiles them
      (`release-compatibility.md`: hosted MCP tools run in the daemon
      binary). They reach no cross-version surface.
    - Decision 3 changes server-rendered `.sentdata` content only. The
      client's `applySent` and the pill are unchanged. That falls under
      dashboard assets and behaviour, which is T0.
11. **A terminal stop suppresses the nudge** ((571)=a, owner, 2026-11-13).
    - The Claude mod's `turn_end` intake event gains an optional `reason`
      string. The mod already knows it (`dashboard.ts:1465`, the `reason`
      argument of `turnCompleted`; its subagent branch reads
      `reason === 'answer'` at `:1474`) and today sends only
      `{kind: 'turn_end', at}` (`:1494`). It sends the reason it has on every
      main-thread turn end. The values are `answer` (a normal end),
      `aborted` (Esc in the terminal, or a dashboard-interrupt abort), and
      `error` (`dashboard.test.ts`; `compaction.ts:528` already treats
      `reason !== 'aborted'` as normal).
    - The reason travels on `wslive.Event` (`state.go:78`) as a new optional
      field, and `state.Apply` copies it onto the `RingTurn` entry it builds
      (`wslive/state.go:289-305`). `RingEntry` gains an optional,
      `omitempty` field for it (`wslive/ring.go:63`). The hosted runner's
      `wslive.Event{Kind: KindTurnEnd}` (`runner.go:2695`, `:2943`) stays
      reasonless. The `dashboard_intake` tool description names the new
      optional field.
    - Stop predicate: only `reason == "aborted"` is a terminal stop. An
      evaluation whose settled turn carries it skips without advancing
      `through`, exactly as a dashboard interrupt does (Decision 8).
      `answer`, `error`, any other value, and an absent reason (an older mod,
      the hosted runner) are normal settles. `error` counts as normal because
      the owner did not stop the agent; the once-per-batch rule bounds it.
    - Rejected: suppressing on any non-empty reason. The mod sends a reason
      on every turn, so no mod session would ever be nudged.
    - The hosted runner needs no change: a hosted conversation is stopped
      through the dashboard interrupt route, which Decision 8 covers.
    - Rejected: accepting the gap, so a nudge restarts the agent right after
      the owner stopped it from the terminal.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  `agents-plugin-tool/`). The nudge text is a string emitted to an agent.
  It must not name this repository's tickets, layout, or tooling.
- Convention: ai-docs/manuals/release-compatibility.md (declared for
  `agents-plugin-tool/internal/`). Decision 10 records the T0 evidence,
  including the one optional `turn_end` `reason` and the one optional
  `RingEntry` field of Decision 11. Any further field on `wsinbox.Entry`,
  `RingEntry`, or an intake event needs the tier restated.
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for
  `agents-plugin/hooks/`). The `turn_end` `reason` change in
  `agents-plugin/hooks/claude-mod/dashboard.ts` is mirrored to
  `agents-plugin-wsflow/hooks/claude-mod/` as that manual directs.
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  `agents-plugin-tool/internal/mcp/`). Phase 2 adds `BoardItems` to
  `mcp/dashboard_page.go`. Update the runbook's session-directory and inbox
  description (`ws-mcp.md` around the `sessions/<reply-id>/inbox/` entry):
  add `ack-check.json` and the daemon's answer check.
- Do not touch the `wsinbox` package. A parallel ticket edits
  `wsinbox.Entry` and `Merge`.
- Client assets (`applySent`, `pill.js`, the tab-label count) stay as they
  are. Decision 3 is carried entirely by the server-rendered `.sentdata`.
- The nudge never fires for a session without a live adapter, without a
  board, or for an agent conversation.
- An evaluation never blocks a page render or a stream. It runs on its own
  coalesced timer, and it stops with the daemon (`d.stop`).

## Prior Decisions

- 261109-bug-ws-whiteboard-patch-cannot-change-item-class (2026-11-09, commit 57ba56f1): "sentMarks filters server-side; .sentdata, applySent and pill.js are untouched. A chip with no send time (pre-change records) keeps its mark." — bearing: supports
- 261106-feat-ws-dashboard-browser-interaction (2026-11-06, commit 62055a21): "Sent marks derive from disk: the newest chip per item across the drained records and then the held entries, so a held answer shows over a delivered one and a mark survives reload." — bearing: supports
- 548c3aac (2026-11-09, commit): "User report: items already sent from the dashboard without a wb:answer record stayed in the open count, which was distracting." — bearing: constrains
- 97c3ad72 (2026-11-08, commit): "The class is recomputed from scratch on every applySent run, like the marks, so it clears by itself when the lead records the answer: the inbox record leaves the convlive sent data and the board swap redraws the item answered." — bearing: supports
- 8d43e769 (2026-11-11, commit): "The inbox hard wake fires from the daemon's inboxAnswered (one success point for message/cancel/steer/interrupt) rather than inside wsinbox.Send/Steer/Interrupt" — bearing: supports
- 261110-bug-ws-dashboard-daemon-lock-scope (2026-11-10, commit 291bb82c): "agentChanged and inboxAnswered use the new Reread: the runner and the index entry are trusted, so they skip the registration read on the caller." — bearing: constrains
- 261107-feat-ws-dashboard-interrupt-badge (2026-11-08, Result 07674bfaf): "an interrupt drain aborts only a running main turn that started before the newest sent_at, setting the interrupted mark before $.turn.abort" — bearing: constrains
- 261112-feat-ws-dashboard-lead-wake-events (2026-11-13, commit fd75fe2f): "Gate wake separately and gate new status fields with task_counts so older status-capable daemons never receive unsupported count fields." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard (pageview_inbox.go, pageview.go, page.go, agent_sessions.go, inbox_routes.go, daemon.go, live_hosted.go, new ack_check.go), internal/mcp (dashboard_page.go, live_snapshot.go, server.go), internal/wslive (ring.go, state.go), agents-plugin/hooks/claude-mod/dashboard.ts with its wsflow mirror, ai-docs/manuals/ws-mcp.md |
| scope.surface | cross-module | new BoardItemsSource interface in wsdashboard implemented by mcp.DashboardSessionSource; optional turn_end reason on the dashboard_intake event and RingEntry |
| scope.new_public_symbol | yes | BoardItemsSource, BoardItems, an optional RingEntry reason field |
| scope.new_type_contract | yes | BoardItemsSource { BoardItems(reg Registration) (BoardItems, bool) } and BoardItems { Open map[int]bool; Reasked map[int]int64 }; sentMarks gains a settled-start argument |
| scope.test_surface | new-files | existing wsdashboard/pageview_inbox_test.go, check_client_test.go, and mcp/live_snapshot_test.go extend; the evaluator integration tests need a new test file (no ack_check*.go exists in wsdashboard) |
| complexity.reuse_points | confirmed | read: sentMarks re-ask rule (pageview_inbox.go#L172-L186), inboxAnswered wakes (inbox_routes.go#L185-L201), label.json precedent (label_route.go#L35-L36), ReaskTimes (wswhiteboard/store.go#L180) |
| complexity.side_effect_risk | high | the daemon injects an agent turn on its own timer, under d.inboxMu, with a watermark file; a mistake loops or spams agents |
| risk.correctness | high | settled-turn timing (At minus DurationMs vs chip SentAt), steer and held-input races, restart and concurrency once-per-batch guarantees |
| risk.fit | moderate | follows the label.json and inboxAnswered patterns, but adds a daemon-side evaluator with four trigger sites and a new source interface |
| risk.test | moderate | timers, concurrency (-race), and headless hosted or mod-engine checks; the mod change relies on the existing dashboard-engine test harness |
| risk.security_or_contract | moderate | T0 additive fields across versions (intake reason, RingEntry field) and agent-facing nudge text that must stay downstream-neutral |

## Phases

### Phase 1: Settled-turn rule and note-only marks that clear

Implements Decisions 3 and 4.

- Add a pure helper over `[]wslive.RingRecord`: the newest `RingTurn`
  record's start (`At − DurationMs`, in epoch ms) and whether one exists.
  A newest `RingTurn` record with `DurationMs == 0` reports no usable start
  (Decision 4), and a comment notes the start is approximate (`At` is the
  ring write stamp).
- Give `sentMarks` the settled start.
  - A drained note-only chip (`Answer == ""`) with
    `0 < SentAt < settledStart` clears its item's mark. When it is the
    item's newest chip, an older value chip's mark does not come back.
  - Held chips are always marked, as today.
  - `buildSessionView` passes the start computed from `in.Ring`.

Verification:

- `go test ./internal/wsdashboard/`. Add `sentMarks` cases to
  `pageview_inbox_test.go`:
  - a note-only chip before the settled start: no mark;
  - a note-only chip after the settled start: marked;
  - a held note-only chip: marked;
  - a value chip before the settled start: still marked;
  - a cleared note over an older value chip: no mark;
  - `SentAt == 0`: marked;
  - no ring turn: unchanged behaviour;
  - a newest ring turn with `DurationMs == 0`: unchanged behaviour.
- Add a session-page render test: the convlive region's `.sentdata` drops
  the cleared note chip once a `RingTurn` ring entry lands after it.
- Node harness: extend `TestCheckRowClientPaths`
  (`check_client_test.go`). An `applySent` run without the item's entry
  removes `sent-sent` and its `sentmark`. `TestTabLabelAsksAreThePillsItems`
  and `TestTabLabelCount` stay green.
- Headless check:
  - Run a dev daemon. Use a session with a board and a drained note-only
    chip.
  - Capture the board before and after appending a `RingTurn` ring entry
    that starts after the chip.
  - The `sent · note` mark disappears, and the open-question pill count
    rises by one.

### Result (bf4e1be18) - 2026-11-14

- Landed:
  - `newestTurn`, `turnStart`, and `settledTurnStart` in
    `wsdashboard/pageview_inbox.go`. A newest turn with no duration reports
    no start, with no fallback to an older turn. The start is commented as
    approximate.
  - `sentMarks(drained, held, reasked, settled)` clears a drained note-only
    chip with `0 < SentAt < settled`. A cleared newest chip leaves no older
    value mark behind.
  - `buildSessionView` passes `settledTurnStart(in.Ring)`.
- Tests (b8d1e55a7):
  - `TestSentMarksClearNoteOnlyChipsAtTurnSettle` covers every listed case,
    plus a held answer after a cleared note.
  - `TestSettledTurnStartReadsTheNewestTurn`.
  - `TestSessionPageDropsANoteOnlySentMarkOnceItsTurnSettles` covers the
    convlive `.sentdata` drop.
  - `TestCheckRowClientPaths` is extended so `applySent` without the entry
    removes `sent-sent` and the `sentmark`. `TestTabLabelAsksAreThePillsItems`
    and `TestTabLabelCount` stay green.
- Verification:
  - `go test ./internal/wsdashboard/ -count=1`: ok.
  - Headless check on an isolated dev daemon (scratch `WS_CACHE_HOME`, own
    port, shut down afterwards):
    - Server-side, `.sentdata` went from
      `<i data-id="01" data-answer="" data-note="just a note" data-state="sent">`
      to empty once a `turn` ring entry starting after the chip landed.
    - In live Chromium, `.sent-sent` and the `sent · note` sentmark
      disappeared without a reload.
    - The tab title's open count, which comes from the pill's count, went
      from (1) to (2).

### Phase 2: Daemon answer check and nudge

Implements Decisions 1, 2, and 5-9. Depends on Phase 1's settled-turn
helper.

- Add `BoardItemsSource` and `BoardItems` in `wsdashboard/page.go`.
  Implement them on `mcp.DashboardSessionSource` and forward them in
  `agentSessions`.
- Add the `ack-check.json` read and write (temp file and rename) in
  `wsdashboard`, beside the label file helpers.
- Add the evaluator, for example `wsdashboard/ack_check.go`:
  - per-session coalescing timers, poked from the four trigger sites;
  - the Decision 5 gates;
  - a pure batch function (drained records, re-ask map, `through`, settled
    start, open map → IDs);
  - the first-evaluation seed (Decision 6);
  - reads outside `d.inboxMu`, then the held re-check, the watermark
    re-check and write, and the send under it;
  - the shared post-write wake helper extracted from `inboxAnswered`.
- The interrupt route records `interruptedAt`.
- Terminal stop (Decision 11): the mod sends `reason` on `turn_end` (and its
  wsflow mirror); `wslive.Event` carries it and `state.Apply` keeps it on the
  `RingTurn` entry; the `dashboard_intake` description names the field; the
  evaluator skips a settled turn whose reason is `aborted` without advancing
  `through`.
- Keep the nudge text as one constant.

Verification:

- `go test ./internal/wsdashboard/ ./internal/mcp/`. Unit-test the pure
  batch function:
  - note-only chips excluded;
  - stale chips (re-ask) excluded;
  - the `through`/settled range bounds;
  - non-open and missing items excluded;
  - IDs ascending and capped.
- Integration tests on a test daemon with a fake source:
  - **Nudge once.** A value chip is drained before a ring turn, and its item
    is still open. One nudge entry is held. Its text names `(NN)`.
    `ack-check.json` advances. A second trigger sends nothing.
  - **No loop.** After the nudge turn settles with the item still open,
    there is no second nudge.
  - **Recorded.** The item is answered, so there is no nudge. The watermark
    advances.
  - **Note-only.** A note-only chip alone never nudges.
  - **Held input.** A held message skips the check. The batch is nudged at
    the next settle.
  - **Busy.** The lead is mid-turn or waiting on a permission, so the check
    skips.
  - **Dashboard interrupt.** No nudge, and `through` is unchanged.
  - **Terminal stop.** A `RingTurn` entry with reason `aborted`: no nudge,
    and `through` is unchanged. Entries with reason `answer`, with reason
    `error`, and without a reason each nudge as usual.
  - **First evaluation.** No `ack-check.json` and a non-empty batch: the
    file is seeded with `through = settledTurnStart` and no nudge is sent.
  - **No duration.** A settled `RingTurn` entry with `DurationMs == 0`: no
    nudge, and `through` is unchanged.
  - **No adapter, agent conversation.** No nudge.
  - **Restart.** A new daemon over the same directory, after a nudge, sends
    nothing more.
  - **Concurrency.** Two concurrent evaluations send exactly one nudge (run
    with `-race`).
- Intake test (`internal/mcp`): a `turn_end` event with `reason` stores it
  on the `RingTurn` entry; one without `reason` stores an entry identical
  to today's.
- Mod adapter: the claude-mod `turn_end` payload carries the reason (its
  existing test harness, if any; otherwise a string check), and the wsflow
  mirror matches.
- `mcp` test: `BoardItems` reports open items and re-ask times from a
  stored board, and false for an unknown session.
- Headless check:
  - Run a dev daemon with a hosted conversation, or the mod engine test
    double, that answers a turn without recording.
  - Send a value answer from the page.
  - The conversation shows the nudge as the next user turn, once. The item
    keeps its `sent` mark until the agent records it.

### Result (bf4e1be18) - 2026-11-14

- Landed:
  - `BoardItemsSource` and `BoardItems` in `wsdashboard/page.go`.
    `agentSessions.BoardItems` forwards them; an agent conversation gets an
    empty value and true.
  - `mcp.DashboardSessionSource.BoardItems` (7f7573afc) reads
    `record.Whiteboard` without a render.
  - `wsdashboard/ack_check.go`:
    - the `ack-check.json` read and write (temp file and rename), and the
      pure `ackBatch` / `ackNudgeIDs` / `ackNudgeText`;
    - `checkAnswers`, which runs the Decision 5 gates in order and adds the
      aborted, no-duration, and interrupt skips;
    - the first-evaluation seed;
    - `commitAnswerCheck`, which re-checks held messages and the whole record
      under `d.inboxMu`, then writes the record and sends;
    - `ackChecker`, the per-session coalescing timer (1 s), closed in
      `Daemon.Close`.
  - Triggers: `sessionChanged`, `agentChanged`, `daemonLiveSlot.Publish`
    (when changed), and `RingAppended`.
  - `inboxChanged` is extracted from `inboxAnswered`; the nudge uses it.
  - The interrupt route records `interruptedAt` through `recordInterrupt`.
  - Decision 11:
    - `wslive.Event.Reason` and `RingEntry.Reason` (`omitempty`), copied
      onto the `RingTurn` entry.
    - `dashboard_intake` describes `turn_end {reason?}`.
    - The Claude mod sends `reason` on the main-thread `turn_end`
      (fa0dde139), mirrored to wsflow.
- Decisions taken:
  - The answer check asks `agentSessions.readsBoardItems()` first, so a
    source without `BoardItemsSource` never gets a seed file written.
  - Before the board read, the batch is computed without the re-ask map.
    Re-ask only drops chips, so an empty result there means an empty batch.
  - The interrupt route takes the session directory as the inbox
    directory's parent, which keeps `inboxWrite`'s signature.
  - The commit compares the whole record, so an interrupt that lands
    between the unlocked read and the commit also aborts the commit.
- Tests (b8d1e55a7):
  - `ack_check_test.go` covers:
    - the pure functions: bounds, note-only, stale, missing and non-open
      items, sorting, the 20-ID cap, and the exact Decision 9 text;
    - one integration test for each Verification scenario: nudge once, no
      loop, recorded, note-only, held input, busy, dashboard interrupt
      through the real route, terminal stop with each reason, first
      evaluation, no duration, no adapter or agent conversation, restart,
      and concurrency under `-race`;
    - the trigger path.
  - `TestDashboardBoardItemsReadsOpenItemsAndReaskTimes` and
    `TestDashboardIntakeKeepsTheTurnEndReason` (mcp).
  - The mod `dashboard.test.ts` checks that the `turn_end` reason is
    present for `answer`, `aborted`, and `error`, and absent when empty.
- Verification:
  - `go test ./internal/wsdashboard/ -count=15 -race -run 'Ack|AnswerCheck|...'`: ok.
  - `go test ./internal/mcp/` (full package): ok.
  - `claude plugin test .` in `agents-plugin`: 821 pass.
  - `TestWsflowHooksMirrorUpToDate`: ok.
  - `python3 -m unittest discover agents-plugin-wsflow/tests`: ok.
  - Headless check on an isolated dev daemon:
    - The first check seeded `through` and sent nothing.
    - A drained value chip for open item (01), followed by a newer turn,
      gave exactly one held entry with the Decision 9 text naming (01).
    - `through` was set to that turn's start.
    - The page showed the held nudge, and the item kept its `sent` mark.
    - Later triggers (held nudge, drained nudge, the nudge turn settled with
      the item still open) sent nothing more.
  - Independent lite review: clean.

### Phase 3: Runbook

- In `ai-docs/manuals/ws-mcp.md`, describe:
  - `ack-check.json` in the session directory;
  - the answer check: when it runs, its gates, and that it sends once per
    batch;
  - the note-only mark clearing at turn settle.

Verification:

- The manual names `ack-check.json` and its `through` and `interruptedAt`
  fields, the first-evaluation seed, every Decision 5 gate, the
  `aborted`-reason and interrupt skips, the once-per-batch rule, and the
  note-only mark clearing at turn settle; each statement matches the code.
- `go test ./...` from `agents-plugin-tool/` stays green apart from the
  known darwin environment failures.

### Result (f2399e6e3) - 2026-11-14

- Landed: `ws-mcp.md` explains how the inbox drives the note-only mark
  clearing, and gives `sessions/<reply-id>/ack-check.json` its own entry.
  - The entry covers the `through` and `interruptedAt` fields, the batch
    rule, the triggers, every Decision 5 gate as a bullet, the aborted and
    interrupt skips, the first-evaluation seed, and once per batch.
  - It also covers the write-before-send rule and the lock re-check.
- Verification:
  - The independent lite review checked the runbook against the code and
    found it consistent.
  - `go test ./... -count=1 -timeout 45m` from `agents-plugin-tool/` passed
    every package except the known darwin environment failures:
    - `cmd/ws-mcp` TestHostedConnectionSeeingALANChangeReplacesTheDaemon;
    - `internal/wsagent` TestClaudeLeadLocalSettingsCanonicalWorktree.
