# Plan: 260711-feat-ws-dashboard-agent-activity-chat-ui — Phase 3: Resume/fork and mid-turn submission queuing

## Relevant Ticket Contract

- "resume from here": in-place rewind via `activity.session.rewind`, replaces
  the current tab. "fork from here": new session via `activity.session.fork`,
  opens a new tab. Both gated by the Cross-Harness Feature Matrix; an
  Unavailable/Hack cell hides/disables its button rather than attempting it.
- **Owner decision, must re-check before wiring live**: "resume from here" may
  need to ship disabled/hidden everywhere in the first pass if no harness
  cleanly supports exact-point rewind, shipping only "fork from here" first.
- **Owner decision, isolation requirement**: "resume from here" must be its
  own isolated component/module behind its own feature flag/capability gate,
  separate from "fork from here" and the shared bubble/turn rendering —
  removable/disable-able by flipping one flag or deleting one module.
- Mid-turn queuing: an immediately-rendered pending user bubble with a
  "pending/queued" badge, cleared once delivered next batch (Codex
  `turn/steer` where available; queue-for-next-turn elsewhere); prompt-box
  up/down-arrow history traversal; a revert/되돌리기 control that pulls a
  still-pending bubble back into the editable prompt input and cancels its
  queued submission (pending bubble removed, nothing sent for it).
- Every chat bubble (user, agent-turn, tool-use) has a copy button (already
  shipped Phase 2 — not touched here).
- Verification boundary: frontend integration tests for gating logic per
  harness capability (including a test asserting "resume from here" stays
  disabled wherever the rewind cell isn't a clean Passthrough/Overlay match);
  browser-level acceptance evidence for a queued mid-turn submission landing
  in the next tool-call batch, and for revert/undo removing a pending bubble
  without sending it.
- Constraint (verbatim, ticket `## Constraints`): "This ticket does not
  re-litigate `260620`'s scope, tiering, or provider adapter design; it only
  consumes the interaction-API contract and capability tiering `260620`
  defines. Any capability gap discovered while designing this UI (e.g. an
  affordance with no backing method) is a `260620` change, not a workaround
  built here." Per-harness-gated affordances must reflect the Cross-Harness
  Feature Matrix at render time; do not show a control for an
  Unavailable-classified cell.

## Resolved Strategic Question 1: Does Any Harness Qualify for Live "resume from here"?

**No.** Per `ai-docs/mental-model/ws-dashboard-agent-harness.md` (fixture-verified
2026-07-11):
- Codex's only rewind primitive, `thread/rollback`, is Passthrough but
  **confirmed deprecated for removal**, and is turn-count-based (drops N
  turns from the end), not point-based — it does not revert file changes and
  is wrong if turns were forked/reordered. Not a clean Passthrough/Overlay
  match for point-based "resume from here".
- OpenCode's equivalent is unverified/unconfirmed (OpenCode not installed).
- Claude's only reachable rewind/fork path is a **Hack** (transcript-file
  truncation, unofficial) — Hack-tier cells require a dedicated ticket with
  experimental UI labeling and owner risk sign-off before backing any shipped
  method (per the mental model's Coupling section), which this phase is not.

Confirmed also against `ws-dashboard/crates/core/src/agent_client_provider.rs`
(`AgentClientCapabilities`): Phase 2 (Codex app-server adapter) and Phase 4
(Claude CLI adapter) both report only `skills: true` in `capabilities()` today
— **no adapter anywhere backs `rewind` in a shipped route**, independent of
tiering. There is no live capability to gate a "resume from here" button on
even if the tiering question were somehow resolved differently.

Per the ticket's own Decisions text, Phase 3's correct scope is: build the
"resume from here" module/flag scaffold in a **shipped-disabled** state for
every current harness, and ship "fork from here" live (backed by Codex's
confirmed-real `thread/fork`, Passthrough). Do **not** wire Codex's deprecated
`thread/rollback` into a working "resume from here" button.

## Resolved Strategic Question 2: ActivitySessionForkRequest Has No Cursor Field — Local Workaround or 260620 Change?

**Verified ownership.** `ws-dashboard/frontend/src/activitySessionApi.ts` was
added by commit `852cd0ad` ("feat(dashboard): draft activity session API
shapes, mirror new kinds in TS"), whose `## AI Context` states explicitly:
"Phase 1 of 260620-feat-ws-dashboard-agent-client-activity-sources; mirrors
the same commit's Rust-side `agent_client_provider` module ... so Rust/TS stay
hand-synchronized." **This file is 260620's contract, not 260711's own
draft.** `ActivitySessionForkRequest` (lines 144–148) carries only
`{ workRootId, activityId, serverRoute? }` — no cursor/turn-cut-point field.
Also verified: `260620-feat-ws-dashboard-agent-client-activity-sources`
(`ai-docs/tickets/ready/260620-feat-ws-dashboard-agent-client-activity-sources.md`)
has Phase 1 (the type-draft phase) and Phase 5 both already closed with a
`### Result`, and its remaining Phase 3 (OpenCode) is blocked on an install —
there is no open, editable 260620 phase to fold a field addition into even if
a local edit were appropriate.

**Ruling: this is a genuine `260620` capability-gap per the ticket's own
Constraints text (quoted above) — do not add a field to the shared
`ActivitySessionForkRequest` type in this phase.** Flag it via a new `idea/`
ticket (Implementation Plan step 3 below) rather than editing 260620's closed
phases or the shared type.

**But this does not force a degraded "fork always operates on the whole
transcript" implementation.** No real `260620` fork route/fetch call exists
yet — `activitySessionStub.ts` has no `stubForkActivitySession` at all today,
so **every current caller of a would-be fork is 100% local/synthetic**; there
is no wire boundary to violate by passing additional context alongside the
unchanged request shape. Concretely: `stubForkActivitySession` is a
Phase-3/711-owned function (lives in `activitySessionStub.ts`, not the shared
`activitySessionApi.ts`), so it may accept a **second, ordinary TypeScript
function parameter** carrying the transcript slice to seed the new session
with — this is plain call-site plumbing, not a change to
`ActivitySessionForkRequest`'s fields. This gives fully correct, real
per-bubble "fork from here" truncation in the Phase 3 demo (forking from an
earlier bubble genuinely produces a new session containing only that bubble's
prefix, not the whole conversation), while leaving the shared wire type
byte-for-byte as `260620` drafted it.

The new idea/ticket must note: once a real `260620` fork route/adapter
exists, the wire contract will need an equivalent field (e.g. `fromCursor` or
`turnId`) since a real HTTP/adapter boundary cannot carry an ad hoc second
function parameter the way this phase's local stub call can — that is the
genuine future 260620 change, not something to invent now.

## Out of Scope

- Wiring any harness's rewind primitive into a live, enabled "resume from
  here" action (blocked by Resolved Question 1 — scaffold only, shipped
  disabled).
- OpenCode adapter work of any kind (Phase 3 of `260620`, unimplemented,
  blocked on an OpenCode install).
- Editing `ws-dashboard/frontend/src/activitySessionApi.ts` or any other
  shared `260620`-owned contract file (Resolved Question 2 — flag via ticket
  instead).
- Real `260620` adapters / real `260624` history source — this phase
  continues to build against `activitySessionStub.ts`, extending it as
  needed (same pattern as Phase 1/2).
- Phase 4 (server-scoped integration) — no `serverId`-specific behavior work
  here beyond what already exists on `AgentChatSessionView.serverRoute`.
- Skill-invocation affordance (separate open question in the ticket's
  Decisions, not part of this phase).
- Any broader dashboard layout adjustment (flagged in ticket Decisions as "not
  yet detailed" and out of this phase's contract).
- Actually implementing a real Codex `turn/steer` HTTP call — the stub's
  `stubSteerActivitySession` is a same-shape placeholder a real adapter
  replaces wholesale later; do not attempt real duplex mid-turn delivery
  timing here.

## Codebase Findings

- `ws-dashboard/frontend/src/agentChatSessions.ts#L30-L50` —
  `AgentChatSessionView`/`AgentChatPaneState` currently carry **no capability
  field** and **no pending/queued-message state**. Both must be added
  additively here.
- `ws-dashboard/frontend/src/activitySessionApi.ts#L57-L66` and `#L96-L105` —
  **verified already present** (contrary to assuming they need drafting):
  `ActivitySessionSendRequest`/`Response` (`{ workRootId, activityId, text,
  serverRoute? }` → `{ accepted: boolean }`) and
  `ActivitySessionSteerRequest`/`Response` (same shape) are already drafted
  inert types. Phase 3 only needs stub-side functions matching these shapes
  (`stubSendActivitySession`, `stubSteerActivitySession`) — no new wire type
  drafting needed for send/steer.
- `ws-dashboard/frontend/src/activitySessionApi.ts#L144-L152` —
  `ActivitySessionForkRequest`/`Response` exist, inert, no cursor field — see
  Resolved Question 2 above for the ruling (do not edit; carry the cut-point
  as a second stub-function parameter instead).
- `ws-dashboard/frontend/src/activitySessionApi.ts#L130-L142` —
  `ActivitySessionRewindRequest` is explicitly `turnsToDrop: number` (coarse,
  matches Codex `thread/rollback`), confirming the type-level contract itself
  assumes coarse, non-point-based semantics — reinforces Resolved Question 1.
- `ws-dashboard/frontend/src/activitySessionStub.ts` — verified current
  state: has `stubCreateActivitySession`/`stubStartActivitySession`/
  `stubStartNewAgentChatSession`/`stubResumeAgentChatSession`/
  `stubActivityHistoryList`, and the Phase 2 streaming demo
  `stubBeginStreamingTurn` (fixed-interval, grows one canned
  `STUB_STREAM_TURN_ID`-tagged block via `onUpdate`, `stop()` only — **no
  `onComplete` callback, no per-harness capability table, no
  send/steer/fork functions**). All must be added here (see Implementation
  Plan).
- `ws-dashboard/frontend/src/agentChatBubbles.tsx#L213-L220` — `UserBubble`
  is the sole existing user-bubble render site, taking only `{ bubble }`, no
  per-bubble handler props. `ChatBubbleView`/`AgentChatTranscriptBubbles`
  (`#L262-L287`) currently take only `{ blocks, sourceKind }` — new optional
  props must thread through both without touching `AgentTurnBubble`/
  `ToolUseBubble`. `ChatBubble` (`#L35-L44`) has no `pending`/`turnId`-as-cursor
  concept for synthetic (not-yet-real) entries — pending bubbles are rendered
  via a separate, parallel path (see Implementation Plan step 6), not by
  feeding synthetic blocks through `groupTranscriptIntoBubbles`, to avoid
  distorting that function's pure "derive bubbles from real `TranscriptBlock[]`"
  contract.
- `ws-dashboard/frontend/src/App.tsx#L6850-L6991` (`AgentChatPaneBody`) —
  verified: **no prompt/send input box exists yet at all** for an active
  session. The existing `useEffect` at `#L6870-L6879` auto-starts the Phase 2
  canned demo (`stubBeginStreamingTurn` with `cursor: "stream-1"`,
  `STUB_STREAM_TURN_ID`) whenever `activeActivityId` changes — this is
  independent of any user action and must be left alone (Phase 2, shipped).
  Phase 3's send-triggered turns must use a **distinct cursor namespace**
  (e.g. `user-turn-{n}`) so a user-driven simulated turn's blocks never
  collide with/overwrite the Phase 2 canned demo entry in the same
  `streamingBlocks` record (both are keyed by `block.cursor`).
- `ws-dashboard/frontend/src/App.tsx#L6809-L6818` (`AgentChatPaneActions`) —
  current action surface: `onClose`, `onStartHarness`, `onResumeHistoryItem`,
  `onLoadHistory`, `isActivePane`. New actions needed: `onSendMessage`,
  `onForkFromBubble`, (scaffold-only, no-op or absent) `onResumeFromBubble`.
  Constructed at `App.tsx#L4023-L4032`; mirrors the same
  `Object.values(agentChatPanes)`/dispatch pattern already used for
  `onResumeHistoryItem`/`onStartHarness`. `startAgentChatHarness`
  (`App.tsx#L4906-L4943`) and `applyAgentChatSession`
  (`App.tsx#L4980-L4992`) are the concrete patterns to mirror for
  `sendAgentChatMessage`/`forkAgentChatFromBubble`: mark-pending →
  await stub call → apply-or-error, using `setAgentChatPanes` functional
  updates keyed by `pane.logicalKey`.
- `ws-dashboard/frontend/src/App.tsx#L7389-L7391` (terminal
  `keydownFallback`) — **negative finding, confirmed**: the only
  `ArrowUp`/`ArrowDown` handling anywhere in
  `ws-dashboard/frontend/src/` forwards raw ANSI escape bytes to the pty;
  history navigation there is delegated to the shell/readline inside the
  terminal. **No reusable in-browser history-array-plus-index precedent**
  exists to mirror for the chat prompt box — build a small, local
  `string[]` + cursor-index mechanism from scratch, scoped to the new prompt
  input component.
- `ws-dashboard/frontend/src/agentChatSessions.test.ts`,
  `agentChatBubbles.test.ts` — confirmed test convention: plain
  `assertEqual`/`assert` helper functions (no framework/mocking library),
  `agentChatBubbles.test.ts` also uses
  `renderToStaticMarkup(createElement(...))` from `react-dom/server` for
  component-shape assertions. Run via `tsc -p tsconfig.route-tests.json`
  then `node ./node_modules/.tmp/route-tests/<file>.test.js`. New test
  files must be added to `tsconfig.route-tests.json`'s `include` array
  (verified current list ends at `src/workbench/**/*.ts`, includes
  `activitySessionApi.ts`/`.test.ts`, `activitySessionStub.ts` — but
  **not** `activitySessionStub.test.ts`, which does not exist yet — add
  it if new stub logic warrants direct unit coverage) and register a new
  `npm` script in `ws-dashboard/frontend/package.json` (verified existing
  `test:agent-chat-tabs`/`test:agent-chat-bubbles`/`test:agent-chat-stream-merge`
  at lines 18–20) alongside them.
- `ws-dashboard/frontend/src/agentChatStreamMerge.ts` /
  `mergeStreamingTranscriptBlocks` (imported `App.tsx#L56`) — the existing
  mechanism that overlays in-flight `streamingBlocks` onto
  `session.transcript.blocks` before rendering; Phase 3's user-triggered
  simulated turns reuse this exact merge path (same `streamingBlocks` state
  variable, different cursor keys) rather than inventing a second overlay
  mechanism.
- `ws-dashboard/crates/core/src/agent_client_provider.rs#L38-L47` —
  confirmed authoritative shape: `AgentClientCapabilities { compact: bool,
  steer: bool, goal: bool, rewind: bool, fork: bool, skills: bool }`
  (`serde(rename_all = "camelCase")`). Confirmed (`#L239-L275`,
  `AgentClientProvider` trait): no adapter method for
  `compact`/`steer`/`goal`/`rewind`/`fork` exists on the trait itself yet
  either (only `initialize`/`list_sessions`/`create_session`/
  `resume_session`/`send_prompt`/`interrupt`/`backfill_transcript`) —
  reinforces that `rewind`/`fork` capability flags are purely
  forward-declared today, matching the mental model's "no adapter yet backs
  rewind/fork/compact/steer/goal in a shipped route" statement.

## Implementation Plan

1. **Capability model (new, small)**: add an `AgentChatCapabilities` type
   (`{ compact: boolean; steer: boolean; goal: boolean; rewind: boolean;
   fork: boolean; skills: boolean }`) to `agentChatSessions.ts`, field-order
   and naming mirroring `AgentClientCapabilities` in
   `agent_client_provider.rs#L38-L47` exactly. Add a `capabilities:
   AgentChatCapabilities` field to `AgentChatSessionView`. Populate via a new
   per-harness table in `activitySessionStub.ts`
   (`stubCapabilitiesForHarness(harness): AgentChatCapabilities`), applied in
   `stubStartActivitySession`/`stubResumeAgentChatSession`:
   - `codex`: `{ compact: true, steer: true, goal: true, rewind: false,
     fork: true, skills: true }` — matches the mental model's
     fixture-verified Codex tiering exactly (`thread/compact/start`,
     `turn/steer`, `thread/goal/*`, `thread/fork`, `skills/list` all
     Passthrough; `thread/rollback` excluded per Resolved Question 1).
   - `claude`: `{ compact: false, steer: false, goal: false, rewind: false,
     fork: false, skills: true }` — `skills: true` because Claude's skill
     listing is split Passthrough(plugin)/Overlay(loose `SKILL.md`), already
     a real dashboard-buildable surface per the mental model, independent of
     this phase's fork/rewind/steer scope; every other Claude cell here is
     Hack (fork/rewind) or Unavailable (compact) or has no native primitive
     (steer/goal) — all `false`.
   - `opencode`: `{ compact: false, steer: false, goal: false, rewind: false,
     fork: false, skills: false }` — unverified/unconfirmed column, default
     to the strictest reading (no cell claimed available) rather than
     guessing.
   **`rewind` stays `false` for every harness** — this is the load-bearing
   gate keeping "resume from here" disabled everywhere, per Resolved
   Question 1. No dev/test-only override for exercising other harnesses'
   hidden-fork path is needed: the gating test (step 8) asserts the
   hidden/shown behavior directly against hand-constructed
   `AgentChatCapabilities` values, not against the stub's harness table, so
   it already exercises every capability combination without a stub-side
   override flag.
2. **"fork from here" (shipped live)**: add
   `stubForkActivitySession(request: ActivitySessionForkRequest, cutBlocks:
   readonly TranscriptBlock[]): Promise<ActivitySessionForkResponse & {
   session: AgentChatSessionView }>` to `activitySessionStub.ts`. `request`
   uses the unchanged, `260620`-owned `ActivitySessionForkRequest` shape
   (`{ workRootId, activityId, serverRoute? }`); `cutBlocks` is a
   Phase-3-local, non-wire second parameter carrying the transcript slice
   from the session's start up through the clicked bubble's blocks
   (inclusive) — the caller computes this slice from the bubble's own
   `ChatBubble.blocks` and `session.transcript.blocks` (find the clicked
   bubble's last block's `cursor` in the full array, slice up to and
   including it). The stub allocates a new synthetic `activityId`
   (`stubActivityId`), builds a fresh `AgentChatSessionView` whose
   `transcript.blocks` is `[...cutBlocks, <synthetic "Forked from
   conversation" agent block>]`, and never mutates the original session.
   Add `onForkFromBubble(pane, bubbleId)` in `App.tsx` mirroring
   `startAgentChatHarness`'s mark-pending/await/apply-or-error pattern: opens
   a **new** agent chat pane/tab (reuse `createAgentChatPane`'s flow), calls
   `stubForkActivitySession`, attaches the resulting session to the new
   pane via `applyAgentChatSession`-equivalent. Add a "Fork from here"
   button in `agentChatBubbles.tsx`'s `UserBubble`, gated on
   `capabilities.fork`, rendered only when true (hidden, not disabled, for
   Unavailable cells per ticket Constraints).
3. **File the 260620 follow-up ticket (do this before/alongside step 2)**:
   create a new `idea/` ticket (e.g.
   `ai-docs/tickets/idea/<date>-idea-activity-fork-cursor-field.md` via
   `ws/tickets_create` or the bundled template) flagging: once a real
   `260620` `activity.session.fork` route/adapter exists,
   `ActivitySessionForkRequest` will need an equivalent
   `fromCursor`/`turnId` field, because a real HTTP/adapter boundary cannot
   carry the ad hoc second function parameter this phase's local stub call
   uses. Reference this plan and `260711` Phase 3 as the origin of the
   finding. This ticket is the correct outlet per the ticket's Constraints
   text ("Any capability gap discovered ... is a `260620` change") — do not
   skip it in favor of only a code comment.
4. **"resume from here" (scaffold only, shipped disabled)**: create an
   isolated new module `agentChatResumeFromHere.tsx` exporting a single
   `ResumeFromHereButton` component and a gate function
   `isResumeFromHereEnabled(capabilities: AgentChatCapabilities): boolean`
   that always returns `false` regardless of `capabilities.rewind` (document
   in a code comment that this is deliberate per Resolved Question 1, not a
   bug — flipping it later is exactly the isolation the ticket requires).
   Import and conditionally render it from `UserBubble` in
   `agentChatBubbles.tsx` behind that single gate — never inlined into the
   shared bubble path. Do not wire it to Codex's `thread/rollback` or any
   live `activity.session.rewind` call in this phase; no stub
   `stubRewindActivitySession` function is needed either, since the button
   never renders enabled.
5. **Base prompt/send input (prerequisite, required, not explicitly named in
   ticket phase text)**: add a text input + submit control to
   `AgentChatPaneBody` (`App.tsx#L6881-L6903`, active-session branch). Add
   `onSendMessage(pane: AgentChatPaneState, text: string): void` to
   `AgentChatPaneActions` (`App.tsx#L6809-L6818`), wired at the construction
   site (`App.tsx#L4023-L4032`) the same way as `onStartHarness`. Add a
   pure helper `appendUserTranscriptBlock(session: AgentChatSessionView,
   text: string): AgentChatSessionView` to `agentChatSessions.ts` (mirrors
   `stubTranscriptBlock`'s block shape: `role: "user"`, fresh unique
   `cursor`, `renderKind: "markdown"`) and a matching
   `applyAgentChatTranscriptAppend(logicalKey, text)` setter in `App.tsx`
   mirroring `applyAgentChatSession`'s `setAgentChatPanes` functional-update
   pattern.
6. **Turn-in-flight / batch-boundary stub mechanism (resolves escalated
   design question 1)**: extend `stubBeginStreamingTurn`'s options with an
   optional `onComplete?: () => void`, invoked exactly once when
   `index >= STUB_STREAM_LINES.length` triggers the existing
   `clearInterval(timer)` path (natural completion only — `stop()` external
   cancellation must not also fire `onComplete`). This single callback is
   the concrete, minimal "next tool-call batch boundary" signal for the
   whole phase: the stub deliberately collapses "batch boundary" and "turn
   completion" into one event, since the stub only ever simulates one
   linear, non-batched stream per turn — document this simplification in a
   comment next to the new option (a real Codex adapter's actual mid-turn
   `turn/steer` batch points are a finer-grained future replacement that
   this callback shape does not need to anticipate further).
   In `AgentChatPaneBody`, add two new pieces of per-pane state:
   `turnInFlight: Record<string, boolean>` and `pendingMessages:
   Record<string, ReadonlyArray<{ id: string; text: string }>>`, both keyed
   by `pane.paneId` (or lift into `AgentChatPaneState` itself if that keeps
   state ownership cleaner — implementer's call, no functional difference).
   `onSendMessage(pane, text)` behavior:
   - If `!turnInFlight[pane.paneId]`: call
     `applyAgentChatTranscriptAppend(pane.logicalKey, text)` (real block,
     step 5), set `turnInFlight[pane.paneId] = true`, and start a **new**
     `stubBeginStreamingTurn` call using a fresh unique cursor (e.g.
     `` `user-turn-${nextUserTurnSequence++}` ``, distinct from the
     existing Phase 2 `STUB_STREAM_TURN_ID`/`"stream-1"` demo so the two
     never collide in the same `streamingBlocks` record), with `onComplete`
     that: sets `turnInFlight[pane.paneId] = false`; then, if
     `pendingMessages[pane.paneId]` is non-empty, dequeues the front entry
     (FIFO — `shift`-equivalent on the readonly array) and re-invokes this
     same "not in flight" branch for that entry's `text` (this is what
     visibly clears its pending badge and starts its own new turn).
   - Else (`turnInFlight[pane.paneId] === true`): push `{ id:
     nextPendingId(), text }` onto `pendingMessages[pane.paneId]`. If
     `pane.session.capabilities.steer` is `true` (Codex only, per step 1's
     table), also call `stubSteerActivitySession({ workRootId, activityId,
     text })` (new trivial stub function returning `{ accepted: true }`,
     fire-and-forget) — this is the exact call site a real Codex `turn/steer`
     adapter replaces wholesale later; render the pending badge label as
     "steering…" for this path. Otherwise (non-Codex, or `steer: false`), no
     stub/network call fires at all — purely local FIFO queue; render the
     badge label as "queued for next turn". In both cases, actual delivery
     timing is still governed by the same local FIFO/`onComplete` above,
     since no real duplex exists yet — document this convergence in a code
     comment as an intentional stub-phase simplification, not a bug.
   - Render pending entries as a small, separate list of pending bubbles
     appended after `AgentChatTranscriptBubbles`' real bubbles (parallel
     rendering path, not fed through `groupTranscriptIntoBubbles`, per the
     Codebase Findings note above) — each with a "pending/queued" (or
     "steering…") badge and a revert button.
7. **Prompt-box history traversal**: in the new prompt input component
   (step 5), track a local `string[]` history array (append on every
   successful send — i.e. every call that reaches
   `applyAgentChatTranscriptAppend`, including ones delivered from the
   pending queue) plus a cursor index. On `ArrowUp`/`ArrowDown` while the
   input is focused and the caret is at the start of the field (standard
   REPL convention — do not intercept the arrow key when the caret is
   mid-text), move the index and set the input value to
   `history[index]`. New, self-contained mechanism — no existing component
   to extend (see negative finding above).
8. **Revert/되돌리기 control**: render a revert button next to each pending
   bubble (step 6). On click — or when up-arrow history traversal reaches a
   still-pending entry (i.e. the traversed history string still matches a
   live entry in `pendingMessages[pane.paneId]`) — pull its text back into
   the editable prompt input and remove it from
   `pendingMessages[pane.paneId]` by `id`. No stub/network call fires for a
   reverted entry going forward. Note (document in a code comment, not a
   blocking concern): if the reverted entry had already fired a
   `stubSteerActivitySession` call (Codex path), that call is a no-op stub
   `{accepted:true}` today with no real delivery effect — a real adapter's
   `turn/steer` may already be irrevocably in flight by the time a human
   clicks revert, which is a genuine future UX question for the live
   Codex integration, not something this phase's stub can or needs to model.
9. **Gating tests**: new `agentChatCapabilities.test.ts` (registered in
   `tsconfig.route-tests.json` and a new `test:agent-chat-capabilities` npm
   script) asserting, via `renderToStaticMarkup` against hand-constructed
   `AgentChatCapabilities` values (not the stub's harness table, so every
   combination is exercised): "fork from here" renders only when
   `capabilities.fork` is `true`; "resume from here" **never** renders
   (or renders disabled — pick one, consistently, and assert it) regardless
   of any capability combination passed in, including `{ rewind: true, ...
   everything else true }` — the explicit test the ticket's Verification
   boundary calls out.
10. **Browser-level acceptance evidence**: extend the existing
    `test:browser` Playwright flow (established Phase 1/2 pattern) with:
    sending a message while idle renders a real bubble and starts a
    streaming reply; sending a second message while the first is still
    streaming renders it as a pending/queued bubble; once the first turn's
    stub stream naturally completes, the pending bubble's badge clears and
    a new bubble/streaming reply begins for it (the concrete "queued
    mid-turn submission landing in the next tool-call batch" acceptance
    evidence); clicking revert on a still-pending bubble removes it with no
    corresponding new bubble ever appearing for it; up/down history
    traversal cycles through previously sent message texts in the prompt
    input.

## Verification Plan

- `npm run test:agent-chat-tabs` / `test:agent-chat-bubbles` (extended) —
  route-test coverage for capability gating and pending-bubble state
  transitions.
- New `test:agent-chat-capabilities` npm script (step 9), following the
  `tsc -p tsconfig.route-tests.json && node
  ./node_modules/.tmp/route-tests/<file>.test.js` convention; add the new
  test file (and any new non-test module it imports, e.g.
  `agentChatResumeFromHere.tsx`) to `tsconfig.route-tests.json`'s `include`.
- `npx tsc -b` / `npm run build` clean (established Phase 1/2 gate).
- `npm run test:browser` (Playwright + cargo daemon build) — add e2e steps
  per Implementation Plan step 10, matching the ticket's explicit
  browser-level verification boundary (queued-submission-lands-next-batch,
  revert-removes-pending-bubble).

## Escalations

None. Both design questions the prior survey escalated are resolved above:
- Stub turn-in-flight/batch-boundary mechanism: a single `onComplete`
  callback on the existing `stubBeginStreamingTurn`, paired with per-pane
  `turnInFlight`/`pendingMessages` state and a FIFO dequeue-on-complete —
  concrete, minimal, and genuinely browser-observable (Implementation Plan
  step 6).
- `ActivitySessionForkRequest`'s missing cursor field: confirmed as a
  genuine `260620`-owned contract gap (file ownership verified via `git log`
  on `activitySessionApi.ts`) — do not edit the shared type; file a new
  `idea/` ticket flagging the future wire-level need (step 3), and implement
  correct per-bubble fork truncation now via a second, non-wire parameter on
  the Phase-3-local `stubForkActivitySession` function (step 2), since no
  real wire boundary exists yet for this call.

Everything else in this plan (capability field addition, fork wiring, resume
scaffold-disabled, base prompt input, turn-in-flight state, history array,
revert control, test locations) is directly executable from this plan
without further research.
