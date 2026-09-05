# Plan: 260905-feat-ws-pi-live-agent-widget — Phase 1: Live-agent widget

## Relevant Ticket Contract

- One compact `belowEditor` widget (not a footer/header replacement) lists live
  agents and threads, one row each: `name · role (worker/execute/fork/thread) ·
  state (running/awaiting approval/awaiting owner) · elapsed`. `name` = alias >
  title > shortened uuid. Idle is not a row state (auto-park removes it); `explore`
  leaves are never rows. (`ai-docs/tickets/ready/260905-feat-ws-pi-live-agent-widget.md#L41-64`)
- Rows ordered `awaiting owner`, `awaiting approval`, `running` (elapsed desc within
  each state); cap 5 with a `+N more` tail; the two awaiting states are **never**
  folded into the tail — only `running` rows get trimmed. Widget disappears when
  nothing is live. (`#L51-56`)
- `setStatus` shows one segment (`ws: 3 agents · 1 question`, question part only
  while a thread is pending) — owns the footer's agent count; the sibling
  goal-loop yield status writes its own separate key, not a second count.
  (`#L56-61`, confirmed distinct from `GOAL_LOOP_YIELD_STATUS_KEY` at
  `src/goal-loop.ts:343`)
- The `260904` pending-question line merges into this widget — one panel, not two.
  (`#L65-68`)
- One row per child; a thread collapses onto its respondent: `buildAgentRows`
  dedupes by agent id — a `threadBound` record renders once, as role `thread`
  only when the thread's `origin === "lead-ask"`, else as its own role. State
  precedence: `awaiting owner` > `awaiting approval` > `running`. The `setStatus`
  count is the deduped row count. (`#L69-75`)
- Source of truth is the RPC registry + thread registry, no widget model. Re-render
  on registry transitions (spawn, settle, report, stop, thread open/close) and a
  10-second timer for elapsed, armed only while the widget has rows.
  `runStartedAt` — a new `RpcAgentRecord` field stamped by `promptAgent` at every
  prompt site (lead send, overlay line, nudge) — is the running-row clock; thread
  rows use `ThreadRecord.touchedAt`. (`#L76-83`)
- Lead only, TUI only: `ctx.mode !== "tui"` guard, `isChildProcess` guard.
  (`#L84-85`)
- Constraints: rows respect `visibleWidth(line) <= width` reusing
  `overlay-chat.ts`'s helpers (not duplicated); never flicker on every
  `text_delta` — re-render only on state transitions and the timer tick.
  (`#L87-92`)
- Phase 1 deliverable: `agents-plugin-pi/src/agent-widget.ts` with pure
  `buildAgentRows(records, threads, now)`, the `setWidget`/`setStatus` IO glue
  wired from `index.ts` on `session_start` (TUI lead only) subscribed to registry
  transitions, `refreshPendingWidget` in `ask.ts` replaced by the merged widget,
  and `runStartedAt` stamped in `promptAgent`. Test list given verbatim in
  `#L109-114`; live check in `#L114-117`.

## Out of Scope

- `ws-agent-list` tool output/shape (`src/spawner.ts:2272-2287`) — unchanged, own
  data source for the model, not this widget.
- Alias/park/registry-cap mechanics (`reserveAgentAlias`, `evictForCapacity`,
  automatic park in `attachEventListener`) — already implemented by the
  prerequisite ticket `260905-feat-ws-pi-agent-alias-park-and-registry-cap`;
  consumed as-is (`record.alias`, `record.title`, auto-park-to-dormant).
- Goal-loop yield status segment (`GOAL_LOOP_YIELD_STATUS_KEY`,
  `src/goal-loop.ts:343,417,452`) — separate key, already shipped by
  `260905-feat-ws-pi-push-only-child-reports` Phase 2; this widget must not
  touch it, only avoid double-counting in its own segment.
- The one-shot `explore` leaf registry (`AgentRegistry`/`AgentRecord`,
  `src/spawner.ts:393-464`) — a structurally different registry `buildAgentRows`
  never reads, so explore rows are excluded by type, not by a filter.
- Right-hand column layout and footer/header replacement — explicitly rejected
  in the ticket (`#L61-64`).
- Any spec content beyond the anchor/amendment named in "Spec Impact"
  (`#L94-99`) — no other spec sections touched.

## Codebase Findings

- `src/spawner.ts:664-883` — `RpcAgentRecord` fields the widget reads:
  `alias`, `title`, `spawnRole` (`"worker" | "execute-worker" | "fork"`,
  `#L930`), `running`, `threadBound`, `pendingApproval`, `reportLog`. No
  `runStartedAt` field exists yet — must be added here (near `running`/
  `lastLeadPromptAt`, `#L718-763`).
- `src/spawner.ts:1316-1331` — `promptAgent` is the single funnel for every
  `client.prompt(...)` call (spawn's initial prompt `#L2104`, `sendToAgent`'s
  idle/dormant-resume branches `#L2196,2220`, `fork.ts:523`'s anti-bleed nudge).
  Stamp `record.runStartedAt = Date.now()` here, unconditionally (not gated by
  `opts?.isLeadPrompt`, unlike `lastLeadPromptAt` at `#L1327-1329` — the ticket
  wants elapsed to reset on a nudge too).
- **Risk signal / non-obvious constraint**: `sendToAgent`'s streaming branch
  (`src/spawner.ts:2200-2214`, the `interrupt ? steer() : followUp()` path) does
  **not** call `promptAgent` — it flips `record.running`/`terminalThisTurn`/
  `pendingFinal` inline and calls `live.steer`/`live.followUp` directly. Per the
  ticket's literal wording ("stamped by `promptAgent` at every prompt site"),
  `runStartedAt` is intentionally NOT touched by a steer/followUp join — the
  elapsed clock keeps ticking from the turn's original start. Implement as
  written; do not "fix" this into also touching `runStartedAt` from that branch.
- **Risk signal / non-obvious constraint**: a `fork-raised` thread's respondent
  is marked `threadBound` at *registration* time even while its process may be
  dormant (`src/ask.ts:690-739`, `handleForkRaisedQuestion`, sets
  `live.threadBound = true` only when `rpcRegistry.get(agentId)` is live, but
  the thread itself is `"pending"` regardless), and a reopened dormant
  discussion fork is rehydrated with `client: undefined` before its first send
  (`src/ask.ts:550-566`, `rehydrateForkRecord`). "Idle is not a row state"
  (ticket `#L47-49`) applies to plain non-`threadBound` workers that get
  auto-parked; a `threadBound` record must still render as an "awaiting owner"
  row even when dormant (`record.client === undefined`), since that row is the
  owner's action cue. `buildAgentRows`'s row-inclusion test must therefore be
  `record.threadBound || record.pendingApproval || record.client !== undefined`
  — not "has a live client".
- `src/spawner.ts:1836-1876` — `attachEventListener`: the concrete transition
  points a re-render must fire on — `agent_start` (streaming flips true),
  the async settle IIFE (after `probeAgentLiveness` and the automatic-park
  `stopAgent` call), and the `tool_execution_start` branches for
  `GATED_EXEC_TOOL_NAME` (sets `pendingApproval`) and `REPORT_TO_LEAD_TOOL_NAME`
  (report pushes). `applyRpcEvent` itself (`#L1703-1783`) stays pure per its own
  doc comment — do not add a widget call inside it; hook at the IO layer
  (`attachEventListener`) instead.
- `src/spawner.ts:2037-2111` (`spawnAgent`), `:2306-2359` (`stopAgent`),
  `:1380-1391` (`markAgentExited`), `:1429-1437` (`pushSpawnFailed`) — the
  remaining registry-transition points (spawn, stop, exit, spawn-failed).
- `src/spawner.ts:1123-1134` — `leadIdleRef`: exported mutable ref pattern
  (`{ current: (() => boolean) | undefined }`) filled by `index.ts`'s
  `session_start` and read internally by `spawner.ts`, used for the
  held-push decision. Reuse the identical shape for a new exported ref (e.g.
  `agentWidgetRefreshRef: { current: (() => void) | undefined }`) so
  `spawner.ts`'s transition points can trigger a re-render without importing
  `agent-widget.ts`/`ask.ts` (spawner.ts must stay the lower layer — mirrors
  `ask.ts`'s own "imports FROM spawner.ts, never the reverse" rule,
  `src/ask.ts:39-42`).
- `src/spawner.ts:1403-1417` — `startLivenessProbe`: the existing
  arm/disarm-a-timer-only-while-something-is-outstanding pattern
  (`setInterval` + `.unref()`, returns a stopper) to mirror for the widget's
  10-second elapsed-tick timer, armed only while `buildAgentRows(...).length > 0`.
- `src/ask.ts:87-88,337-341,664-667` — `PENDING_WIDGET_KEY`,
  `buildWidgetLines`, `refreshPendingWidget` (the `aboveEditor` widget this
  ticket folds in and Phase 1 explicitly replaces). Call sites to update:
  `src/ask.ts:665` (def, becomes the merged call), `:795` (`ws-ask` tool,
  TUI branch), `:832` (`ws-resolve`), `:891` (`detachForkRaisedThread`),
  `:956` (`injectDiscussionSummary`), `:1248` (`openThread`), plus
  `src/index.ts:358` (`session_start`, after orphan revival).
- `src/ask.ts:138-245` — `ThreadRecord` fields the widget needs:
  `threadId`, `status`, `origin` (`"lead-ask" | "fork-raised"`),
  `respondentAgentId`, `touchedAt`. `src/ask.ts:332-335` — `countPending`
  (exported pure helper) is directly reusable for the `setStatus` question
  count instead of re-deriving it.
- `src/overlay-chat.ts:216-237` — `visibleWidth` (exported), the required reuse
  target for the width bound; `src/overlay-chat.ts:245-263` — `wrapLine`
  available if a row needs truncation-to-width (a widget row is one line per
  the ticket's row shape, so truncate-with-ellipsis is more likely correct than
  wrap — executor's call, using `visibleWidth` either way).
- `src/index.ts:171-406` — `session_start`/`session_shutdown` shape: the
  `rpcRegistryRef`/`leadIdleRef`/`threadHandle` mutable-ref wiring pattern
  (`#L174-188`) to extend with the new refresh ref; `index.ts:249-261` shows
  the exact `ctx.mode === "tui" && isLeadOrFork(readSpawnRole(process.env))`
  guard already used for TUI-only registration (`registerPushMessageRenderers`)
  — reuse this same guard shape for arming the widget.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts:43-100`
  — confirms `ctx.ui.setWidget(key, content, {placement: "belowEditor"})` and
  `ctx.ui.setStatus(key, text | undefined)` are both live Pi APIs (not just
  `ask.ts`'s duck-typed `AskUiCtx`), and `setWidget` accepts either
  `string[] | undefined` or a `(tui, theme) => Component` factory per the
  ticket's own wording ("lines | factory").
- `src/fork.ts:592` — confirms a `ws-fork` task fork's `spawnAgent` call sets
  `spawnRole: "fork"` explicitly; `execute-gateway.ts` sets no `spawnRole`
  (falls through to `spawnAgent`'s own default `toolGroup === "execute-worker"
  ? "execute-worker" : "worker"`, `src/spawner.ts:2070`) — role-label mapping is
  `worker→"worker"`, `execute-worker→"execute"`, `fork→"fork"`, overridden to
  `"thread"` only for a `threadBound` record whose matching `ThreadRecord.origin
  === "lead-ask"`.
- Test convention: `test/<module>.test.ts`, run via `npm test` (`node --test`,
  `package.json:22`, no extra config — Node's native test runner discovers
  `test/*.test.ts` directly). Existing modules test exported pure helpers
  directly with duck-typed fake records/ctx (`test/ask.test.ts`,
  `test/spawner.test.ts`) with no live subprocess for the pure tier; follow the
  same shape for `test/agent-widget.test.ts`.

## Implementation Plan

1. `src/spawner.ts`: add `runStartedAt?: number` to `RpcAgentRecord`
   (near `running`/`lastLeadPromptAt`, `#L718-763`); stamp it unconditionally
   inside `promptAgent` (`#L1316-1331`).
2. `src/spawner.ts`: export a new mutable ref mirroring `leadIdleRef`
   (`#L1123-1134`), e.g. `agentWidgetRefreshRef: { current: (() => void) |
   undefined }`. Invoke it (wrapped in try/catch, best-effort — matches every
   other push call site's swallow-and-continue convention) from: the success
   tail of `spawnAgent` (`#L2037-2111`) and `pushSpawnFailed` (`#L1429-1437`);
   inside `attachEventListener`'s callback (`#L1836-1876`) after each state
   change (`agent_start`, the settle IIFE, the two `tool_execution_start`
   branches); `stopAgent` (`#L2306-2359`); `markAgentExited` (`#L1380-1391`).
3. New file `src/agent-widget.ts`:
   - `export interface AgentRow { name: string; role: "worker" | "execute" |
     "fork" | "thread"; state: "running" | "awaiting-approval" |
     "awaiting-owner"; elapsedMs: number; answerHint?: string }` (or equivalent;
     `answerHint` carries the `/answer <id>` text for `thread` rows per the
     ticket's test list, `#L112`).
   - `buildAgentRows(records: RpcAgentRegistry, threads: readonly ThreadRecord[],
     now: number): AgentRow[]` — pure, per the Codebase Findings row-inclusion
     rule (`threadBound || pendingApproval || client !== undefined`), state
     precedence (`threadBound` > `pendingApproval` > else `"running"`), role
     mapping with the `thread` override (match by
     `thread.respondentAgentId === agentId` and `thread.origin === "lead-ask"`),
     name precedence (`alias ?? title ?? agentId.slice(0, 8)`, mirrors
     `src/ask.ts:351`'s short-uuid convention), elapsed (`thread` rows:
     `now - Date.parse(thread.touchedAt)`; else `now - (record.runStartedAt ??
     now)`), sort (state rank, then elapsed desc), and the cap (keep every
     awaiting-state row, cap `running` rows so the total row count is 5 with a
     synthetic `+N more` trailing element/line — only trim `running`).
   - `buildWidgetLines(rows): string[] | undefined` — one line per row using
     `visibleWidth` (`overlay-chat.ts:216-237`) to bound/truncate to width;
     `undefined` when `rows.length === 0` (hides the widget).
   - `buildStatusSegment(rows, pendingCount): string | undefined` —
     `ws: N agents` (`N = rows.length`) plus ` · M question(s)` only when
     `pendingCount > 0`; `undefined` when `rows.length === 0` and
     `pendingCount === 0`.
4. `src/index.ts`: register a periodic timer mirroring `startLivenessProbe`
   (`#L1403-1417`), armed only while the last-built row set is non-empty, that
   recomputes rows and calls `ctx.ui.setWidget(WIDGET_KEY, buildWidgetLines(...),
   {placement: "belowEditor"})` + `ctx.ui.setStatus(STATUS_KEY,
   buildStatusSegment(...))`. In `session_start` (`#L239-406`), under the same
   `isLeadOrFork(readSpawnRole(process.env))` / `ctx.mode === "tui"` guard
   already used at `#L249,322,388`, set `agentWidgetRefreshRef.current` to a
   closure over `ctx`, `agentTools.rpcRegistry`, and `threadHandle.threads`;
   replace the `refreshPendingWidget(ctx, threadHandle)` call at `#L358` with
   this new refresh. In `session_shutdown` (`#L408-437`), clear
   `agentWidgetRefreshRef.current` and stop the timer, mirroring
   `leadIdleRef.current = undefined` there.
5. `src/ask.ts`: remove the standalone `PENDING_WIDGET_KEY`/`buildWidgetLines`/
   `refreshPendingWidget` widget machinery (`#L87-88,337-341,664-667`) and
   update every call site (`#L795,832,891,956,1248`) to call the merged
   refresh (via `agentWidgetRefreshRef` or an equivalent handle threaded
   through, matching however `index.ts` wires it in step 4) instead. Keep
   `countPending`/`buildThreadListLines` — still used by `/thread`.
6. `ai-docs/spec/pi-adapter-runtime.md`: add the new anchor for the live-agent
   widget (placement, row shape, states, hide-when-empty, `setStatus` segment)
   per the ticket's "Spec Impact" (`#L94-99`), and amend the "Owner surface (TUI
   lead)" bullet of `{#260905-pi-side-thread-owner-question-surface}`
   (`ai-docs/spec/pi-adapter-runtime.md#L835-837`) to replace the standalone
   `aboveEditor` pending-line description with "a row in the live-agent widget".
7. New `test/agent-widget.test.ts`: row shape and states, name precedence
   (alias > title > short uuid), ordering and the cap never folding an awaiting
   row, dedupe of a `threadBound` record with state precedence, hide-on-empty,
   width bound across 40/80/120, thread rows carrying the `/answer` hint, the
   footer segment with and without a pending question — verbatim from the
   ticket's test list (`#L109-114`), following the existing pure-helper,
   fake-record testing convention (`test/ask.test.ts`, `test/spawner.test.ts`).

## Verification Plan

- `npm test` from `agents-plugin-pi/` (runs `node --test`, discovers
  `test/*.test.ts` including the new file); `node --test test/agent-widget.test.ts`
  for a scoped run during development.
- Live check (owner runbook, not automatable offline): spawn two workers and a
  fork, open a thread, confirm the rows update without a lead turn and vanish
  when everything settles (`ai-docs/tickets/ready/260905-feat-ws-pi-live-agent-widget.md#L114-117`).

## Escalations

- None.
