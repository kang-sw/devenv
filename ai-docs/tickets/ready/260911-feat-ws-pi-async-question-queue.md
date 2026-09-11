---
title: "Redesign lead-raised ws-ask as a fork-less async question queue with a sequential prose-modal tier"
related:
  260908-research-ws-pi-ws-ask-removal: the cost/decision anchor this resolves — the fork-cost concern is answered by redesign (fork-less), not removal; that ticket records the settled direction and points here
  260904-feat-ws-pi-side-thread-fork-question-surface: the mechanism this redesigns for the lead-raised path (registry, widget, /thread, /answer, injection are reused); this reverses its "no fork-less quick-answer path / discussions dominate" decision on dogfood evidence and extends its §7 entry_id anchoring to the answer return path. Its fork-raised path (§1 "Entry A meets Entry B") is explicitly OUT of scope and untouched
  260903-feat-human-relay-interactive-gate: the "minimize how much, and how often, the work touches the user's hands" philosophy the pre-authored options follow
  260906-workset-ws-pi-dogfood-ux: the dogfood UX board this owner-friction fix belongs to
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: f8bcdc92885f04e9
sage-review-completeness-reviewed: f8bcdc92885f04e9
---

# Redesign lead-raised ws-ask as a fork-less async question queue with a sequential prose-modal tier

## Background

`ws-ask` / `ws-resolve` (`agents-plugin-pi/src/ask.ts`, landed by `260904`)
register a **lead-raised** owner question and, on `/answer`, spawn a fresh
**discussion fork** at the lead's tip to run an overlay chat. Owner dogfood
(`260908`) found that fork rarely reuses the prefix cache and causes abrupt cost
spikes; both tools are currently hidden from the active tool surface (`ac998f77`,
`a8cf1183`, `5f366eff`) as a reversible mitigation, and `260908` left permanent
removal undecided.

The settled resolution (discuss, 2026-09-11) is **redesign, not removal**, and
the scope is **lead-raised questions only**. The cost driver is exactly one
thing: the discussion fork spawned when a lead-raised question is opened. That
fork was always waste — for a lead-raised question the main lead *is* the owner's
conversation partner, so a "discussion" simply continues in the main channel.
Making the lead-raised path fork-less kills the cost with no capability loss.

**Out of scope — the fork-raised path is untouched.** A question raised by an
*already-live* delegated agent via `ws-report-to-lead(kind: "question")` (a real
spawned task fork or execute worker) is answered by `/answer` **attaching an
overlay to that live agent** — it never spawns a new fork on answer, so it never
had the cost problem, and it is closer to a continuation of that agent's
discussion. Its overlay-chat direct-comm surface stays exactly as is; this ticket
neither retires it nor routes through it.

The canonical use is the lead laying out a batch of non-blocking owner decisions
as an async queue — e.g. rendering a `lead-ticket` Open Decision Queue as a modal
the owner answers when ready.

## Decisions

Settled in discuss (2026-09-11); D-tags map to the confirmed decisions.

- **D5/D6 — resolve `260908` by redesign.** Making the lead-raised path fork-less
  answers the cost concern, so removal is moot. `260908` is updated to record
  this and point here; it is not deleted or promoted. Pi-adapter-local; **no
  `epic/refound` ready-gate** — independent of that epic. Rejected: deleting
  `ws-ask` and its question/thread state.

- **Scope — lead-raised only (revises the earlier "unified" framing).** The new
  prose modal is an **added tier for lead-raised questions**, not a replacement
  of the fork-raised overlay chat. The `260904` overlay-chat component is **kept**
  for the fork-raised direct-comm surface; no unified modal, no fork-raised
  routing change.

- **D4 — rename to `ws-queue-question`.** A **contract change** (async /
  non-blocking / "answer when ready", Codex's queued-question shape), not
  cosmetic; the name `ask` drove the model to "ask now." `ws-resolve` is renamed
  consistently to signal "withdraw a queued question" (exact token an
  implementation detail). The **blocking** question path stays
  `ws-report-to-lead(kind: "question")`, untouched; the two contracts are now
  cleanly separated.

- **D1 — drop the lead-raised discussion-fork spawn (fork-less).** `/answer` on a
  lead-raised queued question no longer spawns a fork; the owner's prose answer is
  injected into the lead session on idle (the existing `followUp` custom-message
  path). Genuine multi-turn discussion continues in the main lead channel, or the
  lead explicitly calls `ws-fork` / `ws-execute` if it decides it needs a peer.
  Rejected: an opt-in "escalate to discussion fork" tier (non-goal until dogfood
  shows a gap).

- **D3 — the sequential prose-modal tier.** A new modal tier, distinct from
  `audit` (read-only viewer) and the fork-raised `interaction` (live overlay
  chat).
  - **Prose is the channel; the modal never parses.** The agent may embed
    enumerated options as `#1. #2. ...` in the question text; the owner answers in
    prose referencing them ("1번이요", "#1") or freeform. The modal stores the
    prose **verbatim** — no option parsing — and the ask-time options travel to
    the lead so the lead interprets. (Owners routinely answer with context beyond
    the question; a parser would flatten it.)
  - **Submission model (canonical = Enter through the sequence → final confirm):**
    - `Enter` = respond and advance to the next question; on the **last** question
      `Enter` raises the **final confirm modal** — the single submit gate, its
      cursor defaulting to the safe **"No"** so nothing submits by accident;
      confirming submits. `shift+Enter` / `ctrl+j` insert a newline within a prose
      answer (so `Enter` is free to advance, per the Slack/Discord convention).
    - **Blank questions stay pending on submit.** The final confirm submits only
      answered questions and shows an "N unanswered — left pending" count; it never
      submits an empty answer. This unifies the Enter path and the Esc path (both
      submit answered, leave blanks pending).
    - `Esc` = the **partial-submit / exit** entry point: with non-empty unsent
      input it asks whether to submit the completed parts (answered only), safe
      default preserving drafts; it is the only path to submit mid-sequence.
    - `tab` / `shift+tab` = move between questions; navigation **wraps around**
      (last → first) and **never submits**, so nothing "falls off the end" into a
      submit. No `ctrl+`-combo nav keys (terminal/ADE conflicts). `↑`/`↓` and
      `pgup`/`pgdn` stay **scroll** within the focused question.
  - **Never auto-pop** (carried from `260904` §5): widget + notify only; the owner
    opens the queue when ready — the point of the async reframe.
  - **Per-question drafts persist** on the thread record (already persisted), so
    closing and reopening resumes typed prose — answering may span multiple
    sittings. `Esc` with non-empty input offers to submit the completed parts
    first (above).
  - **Coverage indicator** (`Qn/N · answered`) so the owner knows what is left
    with wrap-around navigation.

- **D3 (return path) — anchor answers with ask-time context.** On completion the
  queue re-reports to the lead **per question, with each question's surrounding
  context**. Because the answer may arrive after the lead has lost that context
  (compaction), the ask-time snapshot — the offered `#1/#2` options and an
  ask-time anchor (short commit hash and/or the `260904` `entry_id`) — is captured
  **at queue time** and travels with the answer so the lead can recover where the
  question came from. Mirror of `260904` §7 applied to the return path.

- **Model-side withdrawal + concurrency contract.** The model can `ws-resolve`
  (withdraw) a still-pending queued question to cut clutter ("checked — not this
  after all"). Both the model's `ws-resolve` and the owner's modal run in the same
  adapter process, so the adapter serializes them; the UX policy:
  - pending & no modal open → **remove immediately** (widget count decrements).
  - open in the modal → **never yank an in-progress edit.** Show a non-destructive
    "the agent withdrew this" banner and apply the removal on modal close.
    **If the owner has already started answering (non-empty input), that content is
    still delivered to the lead** on submit/close — a withdrawal never discards the
    owner's typed prose. Only a question with no owner input is dropped on close.
  - already submitted → **no-op** (the answer is already injected).

## Constraints

- **Pi-track-local authorship (AGENTS.md clause 1).** Everything is
  `agents-plugin-pi/` (the adapter's own tools and TUI). No `agents-plugin-tool/`
  (ws-mcp Go) or shared `agents-plugin/skills/` change; not gated on
  `epic/refound`.
- **Cross-harness note (flag, not scope).** "Queue a question to the owner" is a
  pattern Codex also has; it is a candidate future ws-mcp harness-peer surface
  (then develop-authored per the harness-peer clause). Kept adapter-local here;
  revisit promotion only once the contract is stable.
- Reuse, do not rebuild: the persisted thread registry
  (`<lead session>.ws-threads.json`), the pending-question row in
  `agent-widget.ts`'s merged `belowEditor` live-agent panel (`260905` folded
  the former standalone `aboveEditor` "N pending" widget into it;
  `agents-plugin-pi/src/agent-widget.ts#L1-8`, `agents-plugin-pi/src/ask.ts#L847-859`),
  `/thread` / `/answer` / reopen shortcut, never-auto-pop, `MAX_CONTEXT_CHARS`
  warning, and idle-`followUp` injection carry over from `260904`.
- **Do not retire the overlay-chat component** — the fork-raised direct-comm
  surface keeps using it. This ticket adds a tier; it does not replace one.
- Lifting the current lead-raised tool-surface hide (`ac998f77` / `a8cf1183`) is
  part of landing the fork-less path, not a re-enable of the old fork behavior.
- **Rename fan-out is in scope.** D4 is a documented-contract change: the
  `ws-ask` / `ws-resolve` names live in `agents-plugin-pi/pi-lead-guide.md` and
  the `pi-adapter-runtime` spec (`ai-docs/spec/pi-adapter-runtime.md`), and the
  literal fans out across the adapter source (`index.ts`, `fork.ts`,
  `fork-context.ts`, `spawner.ts`, `agent-widget.ts`) and its tests. The rename
  must update the guide and the spec passage, not only the tool registration.
  Stale references in not-yet-landed `ready/` siblings
  (`260908-feat-ws-pi-subagent-audit-window-and-owner-steering`,
  `260909-feat-ws-pi-agent-count-panel-header`,
  `260909-feat-ws-pi-agent-row-model-and-usage`) are a coordination note for
  whoever lands them after this — this ticket does not edit those tickets.

## Prior Art

- `260904` `ask.ts` (registry, `/answer` / `/thread`, widget, injection), and its
  fork-raised overlay chat — the standalone `overlay-chat.ts` module no longer
  exists; it was merged into `agents-plugin-pi/src/conversation-view.ts`
  (`770d8fc3`, with its tests migrated off in `22def4f0`) — which stays as the
  fork-raised surface; `audit.ts` (the read-only viewer tier the new tier is
  distinct from). The `260904` Prior Art `questionnaire.ts` (a custom
  component from a tool `execute()`) is the building block for the sequential
  prose modal.
- `260904` §7 `entry_id` anchoring + post-compaction excerpt insertion — the
  inbound analog of the return-path anchoring.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/ask.ts, agents-plugin-pi/src/conversation-view.ts, plus the ws-ask/ws-resolve tool-name literal in agents-plugin-pi/src/index.ts, fork.ts, fork-context.ts, spawner.ts, agent-widget.ts, pi-lead-guide.md and their test files |
| scope.surface | public-interface | model-facing tool contract: registerWsTool's ASK_TOOL_NAME/RESOLVE_TOOL_NAME (agents-plugin-pi/src/ask.ts#L106-109), documented in agents-plugin-pi/pi-lead-guide.md and ai-docs/spec/pi-adapter-runtime.md#L1376-1383 |
| scope.new_public_symbol | yes | renamed tool name ws-queue-question (D4) replacing ws-ask; the ws-resolve replacement token is left as an implementation detail |
| scope.new_type_contract | yes | Phase 1's ask-time snapshot fields (offered options + short-hash/entry_id anchor) added to ThreadRecord (agents-plugin-pi/src/ask.ts#L221-259), and Phase 2's sequential prose-modal component are both new to the tree |
| scope.test_surface | existing | agents-plugin-pi/test/ask.test.ts, conversation-view.test.ts, agent-widget.test.ts, fork.test.ts, fork-lifecycle.integration.test.ts, fork-prefix.integration.test.ts, fork-review-regressions.test.ts already exercise ws-ask/ws-resolve, the registry and the widget |
| complexity.reuse_points | confirmed | persisted registry (agents-plugin-pi/src/ask.ts#L796-845 ThreadRegistryHandle), belowEditor widget row (agent-widget.ts#L1-8), followUp injection (ask.ts#L1109-1126 injectDiscussionSummary/sendToLead), entryId anchoring (ask.ts#L221-233, #L536-600) all read and present |
| complexity.side_effect_risk | moderate | the ws-ask/ws-resolve rename's literal fans out across 7+ source files, their tests, the documented spec ai-docs/spec/pi-adapter-runtime.md (Constraints puts the guide + spec update in scope), and stale references in not-yet-landed sibling tickets (260908-feat-ws-pi-subagent-audit-window-and-owner-steering, 260909-feat-ws-pi-agent-count-panel-header, 260909-feat-ws-pi-agent-row-model-and-usage, all status ready) |
| risk.correctness | moderate | the three-way withdrawal concurrency contract (pending / open-in-modal / already-submitted) and the new fork-less injection path are new branchy state layered on the existing dormant-fork-resume logic (ask.ts#L714-751 rehydrateForkRecord) |
| risk.fit | low | built entirely from already-present, already-read mechanisms per the ticket's own Constraints (persisted registry, merged widget row, followUp injection, entryId anchoring) |
| risk.test | moderate | Phase 2's own verification section defers TTY-only key-sequence behavior (Enter/Esc/tab) to a manual "260903-shape owner runbook", not fully agent-driven |
| risk.security_or_contract | moderate | D4 itself calls the ws-ask -> ws-queue-question rename "a contract change ... not cosmetic"; it changes a model-facing tool name documented in ai-docs/spec/pi-adapter-runtime.md, which Constraints names as in-scope to update |

## Phases

### Phase 1: Async contract — rename, fork-less lead-raised path, withdrawal

Rename `ws-ask` → `ws-queue-question` and `ws-resolve` consistently; make the
lead-raised contract async / non-blocking. Remove the discussion-fork spawn: a
lead-raised answer is injected into the lead session on idle (`followUp`).
Capture the ask-time snapshot (offered options + short-hash/`entry_id` anchor) on
the thread record and carry it on the return report. Implement withdrawal with
the concurrency contract above (immediate for pending; deferred-with-banner and
content-preserving for an open edit; no-op after submit). Lift the tool-surface
hide as part of landing the fork-less path. The fork-raised path and its overlay
are untouched. Headless (`--mode rpc`) baselines from `260904` §8 keep working.

Verification: opening a lead-raised queued question spawns **no** fork process
(`ws-agent-list` / `ps` shows none) and its answer appears as an injected lead
message on idle; the registry records the ask-time anchor and it appears on the
returned report; a withdrawal of a pending question removes it, a withdrawal of a
question the owner has already answered still delivers the owner's content, and a
withdrawal after submit is a no-op; the fork-raised `/answer` overlay path is
unchanged; `pi-lead-guide.md` and the `pi-adapter-runtime` spec passage reflect
the renamed tool(s); the adapter suite is green.

### Result (75f123c9) - 2026-09-11

Landed on `impl/track/pi-agent/argue-roman-dodgy` (4 commits, merged into
`track/pi-agent`): `e4967a56` (rename), `5ec41d18` (fork-less redesign, D3
anchor, withdrawal, tool-surface-hide lift), `ed10d065` (spec doc), `93cf8587`
+ `75f123c9` (review-round fixes).

**What landed, against each Decision:**
- **D4 (rename)**: `ws-ask`/`ws-resolve` → `ws-queue-question`/
  `ws-withdraw-question`. Chose `ws-withdraw-question` as the exact
  replacement token (explicitly left open by the ticket) — it reads
  correctly against the new async-queue framing. Fans out across
  `index.ts`/`fork.ts`/`fork-context.ts`/`spawner.ts`/`agent-widget.ts`,
  `pi-lead-guide.md`, `ai-docs/spec/pi-adapter-runtime.md`, and every test
  file that hardcoded the old literals; `ASK_TOOL_NAME`/`RESOLVE_TOOL_NAME`
  symbol names are unchanged (only their string values), so no caller had to
  be touched beyond the literal-string sites.
- **D1 (fork-less)**: `openThread` gained an early-dispatch guard —
  a `"lead-ask"` thread opens via a new `openLeadAskThread`, which never
  spawns a discussion fork. The answer is delivered back into the lead
  session through the pre-existing `sendToLead(pi, message, "followUp")`
  push/hold path (`deliverQueuedAnswer`), landing on the lead's next idle.
  **Decision**: the old discussion-fork-spawn branch inside
  `ensureRespondent` is kept in place rather than deleted — `openThread`'s
  new early return makes it unreachable for `lead-ask` (and it was already
  unreachable for `fork-raised`, which always registers with a
  `respondentAgentId` set, so it never took that branch either) — kept as
  inert insurance rather than risk a larger deletion diff for a phase that
  scopes the fork-raised path as unchanged; documented as such at the call
  site and in the spec.
- **D3 (return-path anchor)**: `captureAskCommitHash` (best-effort short git
  hash, never throws) + the existing `entryId`, combined via
  `buildAskAnchorLine`, captured at queue time on the thread record and
  carried into the delivered answer message, with a verbatim excerpt
  attached when the anchored entry has fallen off the live branch by
  delivery time.
- **Model-side withdrawal + concurrency contract**: `withdrawQueuedQuestion`
  (`ws-withdraw-question`) implements pending→immediate removal,
  open→deferred (`withdrawnPending`, never yanking an in-progress edit —
  delivered once submitted or closed with a non-empty draft),
  dormant/closed→no-op. The owner-side open/Esc decision (deliver the draft
  vs. finalize the withdrawal vs. revert to pending) was extracted into
  `resolveLeadAskEscapeAction`/`runLeadAskEscapeAction` (mirroring the
  existing `resolveDoneAction`/`runDoneAction` split) during review-round 1,
  so it has direct unit coverage rather than only living inside the
  TUI-only `onEscape` closure. `fork-raised` withdrawal keeps its
  unconditional-immediate-close, unchanged.
- **Tool-surface hide lift**: `addAskToolsIfLead` again appends the ask
  tools only for the true lead (`role === undefined`), a no-op for any
  other role; `computeForkToolSurface` stays an identity function.
  **Clarification** (non-obvious, worth recording): since a fork's
  `--tools` argv snapshot is taken from the lead's own active tools at
  fork-spawn time, this means a forked child's own tool list now shows
  `ws-queue-question`/`ws-withdraw-question` again too — **visible**, with
  identical metadata, but refused only at the **handler level**
  (`readSpawnRole(env) === "fork"` throws inside `execute()`), exactly
  mirroring how `ws-fork` itself is inherited-but-refused in a fork. This
  is a re-enable of the tool surface, not a re-enable of fork-spawning
  behavior, matching the ticket's Constraints framing.
- **Interim UI scope note**: `openLeadAskThread`'s single-question overlay
  (reusing `ConversationViewComponent` with an agent-less local
  `ConversationChannel`) is Phase-1-only scaffolding — Phase 2's sequential
  prose-modal tier supersedes it as the owner-facing surface; it is not a
  preview of Phase 2's UX.
- Headless (`--mode rpc`) baselines: unaffected — `registerAsk`'s
  `execute()` bodies don't branch on TUI mode beyond the pre-existing
  `notify`-only fallback, and headless was already fork-less pre-`260911`.

**Test-scenario changes in `fork-lifecycle.integration.test.ts`** (the one
file whose existing scenarios directly assumed the removed spawn
behavior): the "lead restart" scenario was rewritten to assert the
fork-less contract directly (no process spawn on `/answer`, no paid model
turn, thread opens with status `"open"` and no `respondentAgentId`/
`forkResume`) in place of the old discussion-fork-spawn assertions, and the
dependent forkResume-rehydration-across-generations loop was removed as
now-parasitic (it only ever drove the now-unreachable insurance code) —
`captureForkResume`/`rehydrateForkRecord` keep independent direct unit
coverage in `ask.test.ts` and `agent-telemetry-lifecycle.test.ts`, so this
removed no coverage (round-2 test-partition review independently confirmed
this). The tool-visibility assertion for a fork's inherited tool list was
rewritten from "excluded" to "visible but handler-refused", mirroring the
adjacent `ws-fork`/`nestedFork` pattern, to match the tool-surface-hide-lift
consequence above.

**Verification (per the ticket's own bar):**
- No fork spawns on opening a lead-raised question, and its answer arrives
  as an injected lead message on idle — `fork-lifecycle.integration.test.ts`'s
  rewritten "lead restart"/"noPrior" scenarios; `ask.test.ts`'s
  `deliverQueuedAnswer` suite for the injection shape itself.
- The registry records the ask-time anchor and it appears on the returned
  report — `ask.test.ts`'s `captureAskCommitHash`/`buildAskAnchorLine` and
  `deliverQueuedAnswer` describe blocks.
- Withdrawal semantics (pending/open/dormant-closed, for both `lead-ask`
  and unchanged `fork-raised`) — `ask.test.ts`'s `withdrawQueuedQuestion`
  describe block, plus the extracted `resolveLeadAskEscapeAction`/
  `runLeadAskEscapeAction` unit tests for the owner-side open/Esc decision,
  plus a real `saveThreadRegistryFile` → `hydrateThreadRegistry`
  persisted-restart round-trip for the open→pending/closed normalization.
- The fork-raised `/answer` overlay path is unchanged — confirmed by
  round-1 correctness review (no functional diff to `handleForkRaisedQuestion`,
  `ensureRespondent`'s live/rehydrate/spawn branches, `closeThreadOnDone`,
  `injectDiscussionSummary`, or the overlay-chat component beyond
  comment/doc renames).
- `pi-lead-guide.md` and `ai-docs/spec/pi-adapter-runtime.md` reflect the
  renamed tool(s) and the new fork-less contract (round-1 correctness
  review found no drift between the updated spec prose and the actual
  code).
- The adapter suite is green: `npm test` inside `agents-plugin-pi/` —
  1583 passed, 221 suites, 0 failed, 0 skipped.

**Review**: two independent rounds, partitioned correctness/test per the
route verdict. Round 1: correctness `non-clean: 1 important` (untested
`onEscape` withdrawal/draft decision) + 1 minor (`hydrateThreadRegistry`
setting a stray `withdrawnPending` field on `fork-raised` records); test
`non-clean: 2 important` (the same `onEscape` gap, independently found,
plus the `hydrateThreadRegistry` restart-normalization block untested via
an actual persisted round-trip). Both fixed in `93cf8587` (extraction +
new tests) and `75f123c9` (one follow-up test-precision fix). Round 2:
correctness `clean`; test `clean with 3 minor remaining` (2 pre-existing,
declined as optional; 1 addressed in `75f123c9`). No Critical/Important
findings remain.

**Not carried forward**: Phase 2 (sequential prose-modal tier) is not
started; this ticket stays open in `ready/` for it.

### Phase 2: Sequential prose-modal tier

Depends on Phase 1. Build the modal tier: `Enter` = respond-and-advance,
last-question `Enter` → final confirm (cursor default "No") submitting answered
questions and leaving blanks pending with a count; `shift+Enter` / `ctrl+j` =
newline; `Esc` = partial-submit/exit (answered only, drafts preserved);
`tab` / `shift+tab` = wrap-around question navigation that never submits;
`↑`/`↓` / `pgup`/`pgdn` = scroll within the focused question; a `Qn/N · answered`
coverage indicator; per-question draft persistence across close/reopen; the
withdrawal banner surfaced non-destructively in an open edit; never auto-pop.
Store answers verbatim (no option parsing). Wire completion to the Phase 1
per-question re-report with ask-time anchors.

Verification (agent-driven where possible; TTY-only items packaged as a
`260903`-shape owner runbook): the modal renders queued questions with their
options and a coverage indicator; `Enter` advances and a mid-sequence `Enter`
does not submit; last-question `Enter` raises the No-defaulted confirm and only
that confirm submits, leaving blanks pending with the shown count; `Esc` offers
to submit completed parts and preserves drafts on decline; wrap-around navigation
never submits; a withdrawal on an open, non-empty question shows the banner and
still delivers the owner's content; each answer re-reports to the lead with its
question context and, when the lead is compacted past the ask point, the
recovered options + ask-time anchor.

### Result (a085aa5a) - 2026-09-11

Landed on `impl/track/pi-agent/argue-roman-dodgy`: `6b24e141` (modal
component + queue helpers), `f7b520a0` (pure-helper + interaction tests),
`8bd39c18` (guide/spec docs), `134ddb7a` + `a085aa5a` (review-round-1
fixes and their tests).

**What landed:** `LeadAskQueueComponent` (`agents-plugin-pi/src/ask.ts`)
is the batch owner-facing surface superseding Phase 1's interim
`openLeadAskThread` (removed). It renders every queued `"lead-ask"`
question at once via `collectLeadAskQueue`, one focused pi-tui `Editor`
per question:
- `Enter` commits the focused answer and advances to the next question
  (no wrap); on the last question it raises a final confirm
  (`LeadAskQueueConfirmState`, cursor defaulting to "No") that submits
  only answered questions on "Yes", leaving blanks pending with a count
  (`buildQueueSubmitConfirmMessage`).
- `shift+Enter` / `ctrl+j` insert a newline — native `Editor` behavior,
  no custom handling needed.
- `Esc` is the partial-submit/exit entry: with nothing answered it closes
  immediately; with any answered question it raises the same-shaped
  confirm, defaulting to "No" (preserve drafts, no submit).
- `tab` / `shift+tab` (`QUEUE_SHIFT_TAB = "\x1b[Z"`) wrap-around navigate
  between questions and never submit.
- `↑`/`↓`/`pgup`/`pgdn` reach the focused `Editor`'s own native scroll —
  no custom handling.
- Coverage line via `buildQueueCoverageLine` (`"Qn/N · answered"`,
  `countQueueAnswered` counting non-empty-after-trim drafts).
- Per-question drafts persist on `ThreadRecord.draftAnswer`, restored
  into each `Editor` at construction and cleared by `deliverQueuedAnswer`.
- A withdrawn-pending question shows a non-destructive banner in place
  without discarding typed prose; `repaintActiveQueue()` (a module-scope
  `activeQueueRepaint` hook, mirroring `activeOverlay`) live-repaints an
  open queue the moment `withdrawQueuedQuestion` marks a thread
  `withdrawnPending`, so the owner sees it without having to close/reopen.
- Completion wires to the unchanged Phase 1 `deliverQueuedAnswer` /
  `resolveLeadAskEscapeAction` / `runLeadAskEscapeAction` chain via a new
  thin wrapper, `resolveLeadAskQueueEntryAction(mode, withdrawnPending,
  draft)`, that adds only the batch-submit "deliver if non-blank" branch
  and otherwise defers to the existing (Phase 1, still directly tested)
  function — so the owner-side open/Esc decision logic is not duplicated.
- Answers are stored and delivered verbatim; `#1./#2.` option text is
  never parsed, only carried through as ask-time context (Phase 1's
  anchor mechanism, unchanged).
- `openThread`'s `origin === "lead-ask"` branch now opens
  `openLeadAskQueue(pi, ctx, handle, thread.threadId)` (the whole queue,
  focused on the requested thread) instead of a single-question overlay.

**Decisions taken beyond the ticket's literal text:**
- Chose a manual height-budget truncation over a `ScrollView` for the
  question/context block (`renderInner`, using the existing
  `conversationOverlayHeight(tui)`): the ticket's own scope only asks the
  **answer Editor** to scroll, not the question text, so a full
  `ScrollView` rearchitecture there would be disproportionate. An
  overlong question is truncated with a visible "truncated — see
  `/thread <id>`" marker so the answer `Editor` and the `Esc` exit hint
  can never be pushed off a short viewport.
- Kept `resolveLeadAskEscapeAction`'s existing (Phase 1, tested)
  "already-trimmed input" contract unchanged rather than widening it to
  trim internally; `LeadAskQueueComponent.liveDrafts()` is the single
  choke point that trims every `Editor.getText()` once, so every
  downstream consumer (confirm counts, `handleEscape`, `finish`,
  `openLeadAskQueue`'s `onClose`) agrees on emptiness.
- `activeOverlay` and the new `activeQueueRepaint` hook are cleared
  together, guarded by the same `overlayToken` comparison, so a
  superseded queue instance can never clear a later one's repaint hook.

**Verification (per the ticket's own bar, deferring TTY-only key-sequence
behavior to a manual runbook):**
- Coverage, advance-on-Enter, last-question confirm, blanks-pending count,
  Esc partial-submit/preserve, wrap-around tab navigation never
  submitting, withdrawal-banner content-preservation, per-question draft
  persistence across construction, `initialFocusIndex` (including
  clamping out-of-bounds), and the whitespace-trim regression are all
  covered directly in `agents-plugin-pi/test/ask.test.ts`'s
  `LeadAskQueueComponent` describe block (34 tests) plus the
  `resolveLeadAskQueueEntryAction`/`collectLeadAskQueue`/coverage-helper
  describe blocks.
- Completion wiring to `deliverQueuedAnswer` is exercised through the
  existing Phase 1 `runLeadAskEscapeAction`/`deliverQueuedAnswer` tests,
  unchanged, plus the new queue-level `onClose` tests confirming each
  thread's draft reaches that chain.
- The adapter suite is green: `npm test` inside `agents-plugin-pi/` —
  1624 passed, 225 suites, 0 failed, 0 skipped (final run, after all
  review-round fixes).

**Review**: two rounds, partitioned correctness/test per the route
verdict. Round 1 — correctness: `non-clean: 1 critical` (a whitespace-only
draft on a withdrawn-pending question could be delivered as a real answer,
because the queue's draft map used raw untrimmed `Editor.getText()`
against `resolveLeadAskEscapeAction`'s already-trimmed contract) + 1
important (a very long question could push the answer `Editor` and the
`Esc` hint off a short viewport) + 3 minor. Test — `non-clean: 3
important` (untested stale-submit text-restore path, untested
ordinary-keystroke default routing, untested `initialFocusIndex`
clamping) + 3 minor (the fake test `Editor` didn't trim on submit,
understating its fidelity to the real contract; the single-question
queue path untested; `openLeadAskQueue`'s own notify branches untested).

Dispositions — Critical: **[fixed]** in `134ddb7a` (the `liveDrafts()`
trim choke point) with a regression test in `a085aa5a`; round 2
(Critical-scoped, fresh reviewer) confirmed **clean**, with every
consumer of a draft traced back through the single trim point and the
regression test confirmed to fail pre-fix / pass post-fix. Important
(viewport): **[fixed]** in `134ddb7a` (height-budget truncation) with
long/short-question coverage tests in `a085aa5a`. Test-partition's 3
important: **[fixed]**, tests added in `a085aa5a`. Test-partition's
fake-editor-trim and single-question minors: **[fixed]**, in `a085aa5a`.
Remaining minors, all **[won't fix]**: (a) batch-wide `"open"` marking on
open has no rollback if the overlay throws or is closed externally —
pre-existing pattern shared by every overlay in this file, and
`hydrateThreadRegistry` already heals a stuck `"open"` status on restart;
(b) `openLeadAskQueue`'s "no queued questions" notify branch is
unreachable dead code — the only caller always passes `thread.threadId`,
kept as a defensive fallback; (c) the withdrawal banner shows only for
the focused question — consistent with the banner's job of warning
before that specific answer is submitted, not broadcasting withdrawal
state queue-wide; (d) (test-partition) `openLeadAskQueue`'s own notify
branches remain untested — a pre-existing gap pattern in this file, not a
Phase 2 regression. No Critical/Important findings remain open.

This was the ticket's final phase; both phases are now landed.
