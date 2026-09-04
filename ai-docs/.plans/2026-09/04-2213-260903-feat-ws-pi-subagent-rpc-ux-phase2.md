# Plan: 260903-feat-ws-pi-subagent-rpc-ux — Phase 2: child->lead report channel + path-only transcript

## Relevant Ticket Contract

- Add a child-side `ws-report-to-lead(message)` tool relayed over the parent's
  `RpcClient` event stream into a per-agent bounded buffer. Add
  `ws-agent-transcript(agent_id) -> { transcript_path }` (the Pi session JSONL
  path; no content marshalling).
- `ws-agent-wait` must carry `reason: idle|report` and, on wake, drain **all**
  pending reports for the woken agent in FIFO order (D-D) — a waking lead sees
  the full queue, not one at a time — regardless of whether the wake was
  triggered by idle-settle or by a report arriving.
- Buffer is bounded: default cap the most recent 32 reports per agent;
  on overflow the oldest report is dropped with a marker so the lead knows
  truncation happened.
- Idle harvest stays edge/consume (already landed in Phase 1): once
  `ws-agent-wait` returns a child as idle, that idle is consumed so a later
  wait on an array still listing it does not busy-return it.
- `ws-report-to-lead` is the **only** child-side tool this ticket adds; it
  must be in the worker (`full-worker`) `--tools` allowlist (D-B's depth
  policy — literal-name inclusion, same precedent as `explore`).
- `ws-agent-transcript` is a lead-side introspection tool (same family as
  `ws-agent-list`/`ws-agent-stop`), not a driving/spawn tool — it must NOT be
  reachable from a worker's `--tools`.
- Golden rule: ws-mcp Go source (`agents-plugin-tool/`) is never modified;
  adapter-local only.
- Verification boundary (from the ticket phase text): a live run showing a
  child `ws-report-to-lead` mid-run waking a lead `ws-agent-wait` with
  `reason: report`, multiple queued reports draining FIFO in one wake, a
  report buffering when no wait is pending, and `ws-agent-transcript`
  returning a greppable session path. Depends on Phase 1 (already landed on
  this branch, commit `8abefa9b`).

## Out of Scope

- Idle-timeout auto-reap — deferred post-Phase-2 per the ticket's "Remaining
  open questions" section (still open after this phase).
- In-process `AgentSession` variant — deferred, not part of this phase.
- Any change to `explore`'s tool group (`recon`) or its one-shot machinery
  (`exploreLeaf`/`spawnPiProcess`/`AgentEventLineBuffer`/`handleAgentEvent`) —
  `ws-report-to-lead` is added only to `full-worker`, per the ticket's
  explicit "the only child-side tool ADDED by this ticket."
- Editing `ai-docs/spec/pi-adapter-runtime.md` — the ticket's Spec Impact
  section says the new `260904` report-channel/transcript anchors are written
  via `lead-write-spec` at proceed, not as a code-implementation step. This is
  a code plan.
- Any ws-mcp (Go, `agents-plugin-tool/`) change.
- Validating `model_effort`/model-catalog behavior — untouched by this phase
  (Phase 1 concern).

## Codebase Findings

### Current Phase-1 engine (extend, do not rewrite)

- `agents-plugin-pi/src/spawner.ts#L531-556` — `RpcAgentRecord`: `agentId`,
  `client?`, `sessionPath`, `systemPromptPath`, `modelBase?`, `modelEffort?`,
  `wsToolNames`, `streaming`, `idlePending`, `waiters: Array<() => void>`,
  `lastText?`, `unsubscribe?`. New fields needed: `pendingReports: string[]`
  and `reportsDropped: number`.
- `agents-plugin-pi/src/spawner.ts#L606-614` — `applyRpcEvent(record, evt)`
  handles `agent_start`/`agent_settled` only; all other event types are
  ignored. This is the single seam to extend for report detection (its
  existing doc comment at `#L596-605` says exactly this: "exported so ...
  has direct unit coverage without a real `RpcClient` subprocess").
- `agents-plugin-pi/src/spawner.ts#L616-618` — `attachEventListener` casts the
  raw event to `{ type?: string }` before calling `applyRpcEvent`; the cast
  must widen to include `toolName`/`args` for the new branch.
- `agents-plugin-pi/src/spawner.ts#L655-687` — `spawnAgent`'s `RpcAgentRecord`
  literal needs the two new fields initialized (`pendingReports: []`,
  `reportsDropped: 0`).
- `agents-plugin-pi/src/spawner.ts#L772-774` — `firstIdlePendingAgentId`, a
  pure selection helper over `{id, record}[]`. Add a sibling
  `firstReportPendingAgentId` with the identical shape, checking
  `record.pendingReports.length > 0`.
- `agents-plugin-pi/src/spawner.ts#L776-866` — `WaitForAgentsResult` +
  `waitForAgents`. Three return sites need the new `reason`/`reports`/
  `reports_dropped` fields: the already-idle fast path (`#L821-826`), the
  race-winner path (`#L859-865`), and the timeout path (`#L859-861`, `{
  timed_out: true }`). A new report fast path must be inserted between the
  idle fast path and the `allDormant` guard (`#L828-838`) — a dormant record
  can still hold an undrained report from before it stopped, and that must
  be harvestable without hitting the "every listed id is dormant, nothing
  can ever settle" guard.
- `agents-plugin-pi/src/spawner.ts#L890-913` — `stopAgent`. Does not touch
  `waiters`/registry deletion (D-C: stays dormant/resumable). Must likewise
  NOT clear `pendingReports`/`reportsDropped` — an undrained report survives
  a stop, same as the session file does.
- `agents-plugin-pi/src/spawner.ts#L86-102` (`TOOL_GROUPS`/`resolveTools`) —
  `full-worker` already literal-lists `explore` (a pi-native custom tool, not
  a `ws__*` bridge name) precisely because `--tools` filters "built-in,
  extension, and custom tools alike" and would otherwise silently drop it
  (doc comment `#L72-84`). `ws-report-to-lead` needs the identical
  literal-name treatment in `full-worker` only (not `recon`/`read-only`).
- `agents-plugin-pi/src/spawner.ts#L947-1139` — `registerAgentTools`. Add two
  `pi.registerTool({...})` blocks (`ws-report-to-lead`, `ws-agent-transcript`)
  alongside the existing five; update `ws-agent-wait`'s description text
  (`#L1043-1046`) to mention `reason`/report-draining.

### Child->parent relay mechanism (confirmed by reading the installed package — this is the load-bearing finding)

The ticket's own text says "study how a child RpcClient's event stream
surfaces custom-tool calls to the parent." Traced end-to-end through
`agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent` (installed
`0.84.4`):

- `dist/core/agent-session.js#L360-386` (`AgentSession._handleAgentEvent`) —
  for **every** raw event off the low-level `Agent` (from `pi-agent-core`),
  the session first dispatches it to extension hooks
  (`this._emitExtensionEvent(event)`, `#L384`) and then, unconditionally,
  forwards the **same raw event** to its own subscriber list:
  `this._emit(event.type === "agent_end" ? {...} : event)` (`#L386`). This is
  a second, parallel fan-out — not a filtered subset of the extension-hook
  events.
- `dist/core/agent-session.js#L528-556` — `tool_execution_start`/
  `-update`/`-end` ARE among the events re-emitted to extension hooks
  (`pi.on("tool_execution_start", ...)`), confirming these fire per
  `docs/extensions.md#L656-659` ("`tool_execution_start` is emitted in
  assistant source order during the preflight phase" — i.e. as soon as the
  LLM dispatches the tool call, before the tool body executes). This is a
  separate fan-out from the one that matters for the relay (previous
  bullet); it establishes the ordering guarantee only.
- `dist/core/extensions/types.d.ts#L607-613` —
  `ToolExecutionStartEvent = { type: "tool_execution_start", toolCallId,
  toolName, args }`, where `args` is the raw JSON args object the LLM passed,
  keyed by the tool's declared parameter names (so for a tool with
  `parameters.properties.message`, `args` is `{ message: "<text>" }`).
- `dist/modes/rpc/rpc-mode.js#L263-267` — RPC mode's `rebindSession()` does
  `unsubscribe = session.subscribe((event) => { output(toJsonEvent(event));
  ... })`: it prints the **full** raw `AgentSessionEvent` stream (not a
  curated subset) as JSON lines on the child's stdout. `toJsonEvent`
  (`dist/modes/json-event.d.ts`) only special-cases `message_update`; every
  other event type (including `tool_execution_start`/`-end`) passes through
  unchanged.
- `dist/modes/rpc/rpc-client.js#L409-427` (`RpcClient.handleLine`) — on the
  **parent** side, any stdout JSON line that isn't a `{type:"response", id}`
  matching a pending request is treated as "an event" and forwarded to every
  `onEvent()` listener verbatim. This is the exact mechanism
  `attachEventListener`/`applyRpcEvent` already ride for `agent_start`/
  `agent_settled` (Phase 1) — no new wiring is needed, only a new branch in
  the existing `applyRpcEvent` switch.

**Conclusion for implementation:** the child-side `ws-report-to-lead`
tool's own `execute()` body does not need to reach the parent process by any
special means — it only needs to exist as a registered tool (so the child's
LLM can call it and `--tools` lets it through) and return a trivial ack. The
`tool_execution_start` event that Pi emits automatically when the LLM calls
it is already relayed to the parent's `RpcClient.onEvent()` by the existing
wire — the parent-side `applyRpcEvent` just needs to recognize
`evt.toolName === "ws-report-to-lead"` and read `evt.args.message`. Each
`RpcAgentRecord`'s listener (`attachEventListener`, one per spawned child) is
already 1:1 with that child's own `agentId`, so routing to the right
per-agent buffer is automatic — no correlation logic needed.

### Transcript path

- `agents-plugin-pi/src/spawner.ts#L657-658` — `spawnAgent` already computes
  `sessionPath = join(sessionDir, "session.jsonl")` and stores it on the
  record (`#L665`, `sessionPath` field) — this absolute path is exactly the
  Pi session JSONL the ticket wants returned. `ws-agent-transcript` needs no
  RPC round-trip and no new state: just `registry.get(agentId).sessionPath`.

### Test seams

- `agents-plugin-pi/test/spawner.test.ts#L351-361` (`freshRpcRecord`) — the
  single object-literal factory backing every `RpcAgentRecord` construction
  in the test file (confirmed: no other file in `test/` builds this type
  inline). Adding `pendingReports: [], reportsDropped: 0` to its defaults
  covers all existing call sites with a one-line change.
- `agents-plugin-pi/test/spawner.test.ts#L364-410` (`applyRpcEvent`/
  `firstIdlePendingAgentId` describe blocks) and `#L435-511`
  (`waitForAgents` describe block) — precedent for injecting synthetic event
  objects (`applyRpcEvent(record, { type: "agent_settled" })`) and racing
  `waitForAgents` against a manually-triggered `applyRpcEvent` call
  (`#L477-497`) with no real subprocess. The new report path is
  testable the identical way: `applyRpcEvent(record, { type:
  "tool_execution_start", toolName: "ws-report-to-lead", args: { message:
  "..." } })`.

## Implementation Plan

1. **`agents-plugin-pi/src/spawner.ts` — extend `RpcAgentRecord` and add
   report-buffer helpers** (near `applyRpcEvent`, `#L596-614`):
   - Add `pendingReports: string[]` and `reportsDropped: number` fields to
     `RpcAgentRecord` (`#L531-556`).
   - Add `export const REPORT_TO_LEAD_TOOL_NAME = "ws-report-to-lead";` as
     the single source of truth shared by the tool registration and the
     event-matching branch.
   - Add `export const REPORT_BUFFER_CAP = 32;`.
   - Add `export function enqueueReport(record, message: string): void` —
     `record.pendingReports.push(message)`; if length exceeds
     `REPORT_BUFFER_CAP`, `shift()` the oldest and increment
     `record.reportsDropped`; then `settleWaiters(record)` (reuses the
     existing generic `settleWaiters` at `#L322-326` — a report is a wake
     condition exactly like `agent_settled`, and draining an empty
     `waiters` array when nobody is currently waiting is already a no-op).
   - Add `export function drainReports(record): { reports: string[];
     reports_dropped: number }` — swaps `record.pendingReports` for `[]` and
     `record.reportsDropped` for `0`, returning the previous values. Pure,
     mirrors the edge/consume shape of the existing `idlePending` clear.
   - Extend `applyRpcEvent`'s signature to `evt: { type?: string; toolName?:
     string; args?: unknown }` and add one branch: `else if (evt.type ===
     "tool_execution_start" && evt.toolName === REPORT_TO_LEAD_TOOL_NAME)` —
     read `(evt.args as { message?: unknown } | undefined)?.message`; if a
     string, call `enqueueReport(record, message)`. Update the widened cast
     at the `attachEventListener` call site (`#L617`) to match.
   - Add `export function firstReportPendingAgentId(records: ReadonlyArray<{
     id: string; record: RpcAgentRecord }>): string | undefined` — identical
     shape to `firstIdlePendingAgentId` (`#L772-774`), checking
     `record.pendingReports.length > 0`.

2. **`agents-plugin-pi/src/spawner.ts` — thread `reason`/`reports` through
   `waitForAgents`** (`#L776-866`):
   - Extend `WaitForAgentsResult`: add `reason?: "idle" | "report"`,
     `reports: string[]`, `reports_dropped: number` (always present;
     `[]`/`0` when nothing was harvested, e.g. on timeout).
   - Factor a small helper `async function harvestWinner(record:
     RpcAgentRecord, agentId: string): Promise<WaitForAgentsResult>`: if
     `record.idlePending`, clear it, `drainReports(record)`, return `{
     agent_id: agentId, reason: "idle", last_message: await
     harvestLastMessage(record), ...drained, timed_out: false }`; else
     `drainReports(record)` and return `{ agent_id: agentId, reason:
     "report", ...drained, timed_out: false }` (no `last_message` — the
     agent hasn't settled). **Tie-break note (implementation-level, not a
     ticket ambiguity):** idle is checked before pendingReports in
     `harvestWinner`, so if both happen to be true at harvest time, the wake
     reports `reason: "idle"` (reports are still drained either way, per
     D-D — only the `reason` label differs). This mirrors the two fast-path
     checks below and keeps a single, deterministic ordering.
   - Replace the `alreadyIdle` fast-path body (`#L821-826`) with `if
     (alreadyIdle) return harvestWinner(registry.get(alreadyIdle)!,
     alreadyIdle);`.
   - Insert a report fast path immediately after (before the `allDormant`
     guard, `#L828`): `const alreadyReported =
     firstReportPendingAgentId(records); if (alreadyReported) return
     harvestWinner(registry.get(alreadyReported)!, alreadyReported);` — this
     ordering matters: a dormant record can carry an undrained report from
     before it stopped, and this must resolve before the `allDormant`
     hang-guard would otherwise (incorrectly) refuse the wait.
   - Replace the winner-harvest tail (`#L863-865`) with `return
     harvestWinner(registry.get(winnerId)!, winnerId);`.
   - Timeout branch (`#L859-861`) becomes `return { timed_out: true, reports:
     [], reports_dropped: 0 };`.
   - No change to the `allDormant` guard's condition itself (`#L833-838`) —
     only its position relative to the new report fast path.

3. **`agents-plugin-pi/src/spawner.ts` — tool-group + transcript accessor**:
   - `TOOL_GROUPS["full-worker"]` (`#L89`): append the literal
     `REPORT_TO_LEAD_TOOL_NAME` alongside `"explore"`. Update the doc
     comment above `TOOL_GROUPS` (`#L72-84`) to mention the new literal name
     using the same "pi-native custom tool, not a `ws__*` bridge name"
     justification already written for `explore`.
   - Add `export function getAgentTranscriptPath(registry: RpcAgentRegistry,
     agentId: string): { transcript_path: string }` — look up the record,
     throw `ws-pi-agent: unknown agentId "${agentId}"` (same message
     convention as `sendToAgent`/`stopAgent`) if missing, else return `{
     transcript_path: record.sessionPath }`. No RPC call, no async.

4. **`agents-plugin-pi/src/spawner.ts` — `registerAgentTools`
   (`#L947-1139`)**:
   - Register `ws-report-to-lead`: `parameters: { type: "object",
     properties: { message: { type: "string", description: "Status update
     or intermediate finding to surface to the lead immediately." } },
     required: ["message"] }`. `execute()` returns a trivial ack (e.g. `{
     content: [{ type: "text", text: "reported" }] }`) and touches no
     registry — per the Codebase Findings conclusion above, the relay is
     already handled by Pi's own `tool_execution_start` event forwarding
     into the parent's `applyRpcEvent`. Description text should tell the
     model this is for buffered async status updates, distinct from its
     final answer (harvested separately via `agent_settled` +
     `getLastAssistantText()`).
   - Register `ws-agent-transcript`: `parameters: { type: "object",
     properties: { agent_id: { type: "string", ... } }, required:
     ["agent_id"] }`. `execute()` calls `getAgentTranscriptPath(rpcRegistry,
     p.agent_id)` and returns its JSON-stringified result — same pattern as
     `ws-agent-stop`/`ws-agent-list`.
   - Update `ws-agent-wait`'s registered `description` (`#L1043-1046`) to
     mention it now also wakes on a child's `ws-report-to-lead` call
     (`reason: "report"`), draining all buffered reports for that agent in
     one response.
   - Update the module-level doc comment (`#L1-36`) and the
     `registerAgentTools` doc comment (`#L931-946`) — both currently say
     "five" tools / list five names; bump to seven and mention the
     report-buffer + transcript additions, following the same prose style
     already used for the Phase-1 rewrite notes.

5. **`agents-plugin-pi/test/spawner.test.ts`**:
   - `freshRpcRecord` (`#L351-361`): add `pendingReports: [], reportsDropped:
     0` to the default literal — covers every existing construction site
     with one edit.
   - New `describe("applyRpcEvent: ws-report-to-lead")` block: a
     `tool_execution_start` event with a matching `toolName` and a
     string-valued `args.message` enqueues into `pendingReports` and settles
     any pending waiter; a `tool_execution_start` for a *different*
     `toolName` (e.g. `"bash"`) is ignored (no mutation); a missing/
     non-string `args.message` is ignored.
   - New `describe("enqueueReport / drainReports")` block (or fold into the
     above): pushing `REPORT_BUFFER_CAP + 1` messages keeps exactly
     `REPORT_BUFFER_CAP` entries (the most recent ones) and sets
     `reportsDropped` to `1`; `drainReports` returns the buffered messages
     in FIFO (push) order plus the dropped count, and resets both.
   - New `describe("firstReportPendingAgentId")` block mirroring
     `firstIdlePendingAgentId`'s two existing tests (`#L393-410`): undefined
     when none pending; first-in-order id when multiple have pending
     reports.
   - Extend the `waitForAgents` describe block (`#L435-511`):
     - a report enqueued on an agent with no live client is still harvested
       via the report fast path (dormant-but-reported case), returning
       `reason: "report"` and the buffered message(s), without hitting the
       "every listed agentId is dormant" rejection.
     - a report arriving while a `waitForAgents(...)` call is already
       pending (via `enqueueReport` after the wait starts, mirroring the
       existing agent_settled race test at `#L477-497`) resolves the wait
       with `reason: "report"` and the message.
     - multiple reports enqueued before any wait call are all drained in one
       response (`reports` has length > 1, FIFO order, `reports_dropped:
       0`).
     - an idle-settle and a pending report on the SAME agent at harvest time
       report `reason: "idle"` but still return the buffered report(s) in
       `reports` (exercises the tie-break decided in Implementation step 2).
   - New `describe("getAgentTranscriptPath")` block: known agent id returns
     `{ transcript_path: record.sessionPath }`; unknown id throws matching
     `/unknown agentId/` (same assertion style as the existing `sendToAgent`/
     `waitForAgents` unknown-id tests, e.g. `#L443`, `#L593`).

## Verification Plan

- `cd agents-plugin-pi && node --test test/` — unit suite green after the
  `spawner.test.ts` additions above (no `npm install` needed; Phase 1 already
  added the `@earendil-works/pi-coding-agent` runtime dependency).
- Live gate (per the ticket phase text, manual/local — no live provider
  credentials in this environment, same caveat Phase 1's Result section
  recorded): spawn a worker via `ws-agent-spawn`, have it call
  `ws-report-to-lead` mid-run and confirm a concurrent `ws-agent-wait`
  returns with `reason: "report"` and the message; queue multiple reports
  before calling `ws-agent-wait` once and confirm all drain together in
  FIFO order; confirm a report buffers correctly (no wait pending) and a
  later `ws-agent-wait` picks it up; call `ws-agent-transcript` and confirm
  the returned path is the actual on-disk session JSONL (greppable,
  non-empty once the child has run at least one turn).
- No automated test in this repo's `node --test` suite can exercise a real
  `pi --mode rpc` child process (no live model/API key in CI) — consistent
  with Phase 1's precedent; the live gate above is manual/local only.

## Escalations

- None. Confidence is high: the ticket's own explicit ask — "study how a
  child RpcClient's event stream surfaces custom-tool calls to the parent" —
  is answered by direct source evidence (Codebase Findings above), not
  assumption: `AgentSession._handleAgentEvent` forwards every raw Agent
  event (including `tool_execution_start`/`-end`) to the same subscriber
  list RPC mode prints to stdout, and `RpcClient.handleLine` forwards any
  non-response line to `onEvent()` listeners — so the existing
  `applyRpcEvent` seam from Phase 1 is sufficient with one new branch; no
  new transport, polling, or SDK feature is needed. The only judgment calls
  made (idle-before-report tie-break when both are true at harvest time;
  `ws-report-to-lead`'s `execute()` being a no-op ack since the relay
  happens via the event stream, not the tool's return value) are narrow,
  reversible implementation details within the ticket's settled design, not
  scope reductions — both are called out explicitly above rather than
  silently assumed.
