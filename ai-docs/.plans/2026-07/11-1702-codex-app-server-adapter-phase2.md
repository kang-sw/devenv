# Plan: 260620-feat-ws-dashboard-agent-client-activity-sources — Phase 2: Codex app-server read/write adapter

> Research pass 2026-07-11: replaced the prior survey/escalation output. Both
> escalation reasons (live event-sequencing, JSON-RPC-over-stdio precedent) are
> now resolved with direct evidence captured this pass; the third open question
> (Codex no-session plugin listing) is answered confirmed. Escalations: None.

## Relevant Ticket Contract

- The daemon spawns `codex app-server` (default `--listen stdio://`; `--stdio`
  is shorthand — both confirmed in `codex app-server --help`) as a child
  process per the spawn-authority Decision, speaks its JSON-RPC stdio protocol,
  maps Codex lifecycle concepts into the Phase 1 `AgentClientProvider` contract,
  and projects native Codex activity into `ActivityItem` rows and
  `TranscriptBlock` backfill (ticket Phase 2, `#L782-L807`).
- In-scope Codex RPCs (schema fixture-verified against `codex-cli 0.144.1`, and
  the core read/write path live-verified this pass — see Codebase Findings):
  `thread/start`, `thread/resume`, `thread/fork`, `turn/start`, `turn/steer`
  (gated on `expectedTurnId`), `turn/interrupt`, `thread/compact/start` (empty
  `{}` ack; real result via async `thread/compacted`), `skills/list`
  (`{cwds?, forceReload?}` → `{data: SkillsListEntry[]}`), and
  `thread/goal/set`/`get`/`clear`. `thread/rollback` exists but is deprecated
  for removal — do not design new functionality around it.
- Unknown event types must degrade without breaking the whole Activity feed;
  provider adapters must degrade unknown/malformed events into bounded
  status/diagnostic transcript blocks, never leak raw JSON/paths/session
  ids/command output (ticket Constraints `#L535-L539`).
- Before spawning, enforce the ws/wsflow plugin-presence precondition and refuse
  to spawn with install guidance if neither `ws` nor `wsflow` is
  installed/enabled for the target project. **This phase's own fixture check is
  now done: Codex DOES expose a no-session plugin-listing CLI surface — see
  Codebase Findings; the config-file fallback the ticket allowed for is not
  needed.**
- Server-scoped routing: any new provider-process registry/route must key by
  `serverId` the same way `terminal.rs`/`servers.rs` do, not by `workRootId`
  alone (ticket Constraints `#L550-L561`).
- Browser identity rule (Phase 1, unchanged): only `activityId` crosses the
  browser boundary; provider thread/turn/session ids, raw event ids, process
  ids, and cache/transcript/session-file paths stay daemon-private
  (`crates/core/src/agent_client_provider.rs#L49-L60`, `#L79-L84`).
- Verification boundary (ticket `#L827-L837`): fixture projection tests for
  representative Codex thread/turn/item sequences (now capturable from the live
  spike this pass), route tests proving browser payloads omit provider session
  ids and raw paths, a test proving spawn is refused with install guidance when
  the plugin-presence check fails, and a WSL smoke path against the installed
  `codex app-server --stdio` binary runnable under the loopback-only no-auth
  debug profile without owner pairing.

## Out of Scope

- OpenCode ACP adapter (Phase 3, blocked pending install) and Claude CLI
  stream-json adapter (Phase 4) — separate phases and fixture spikes.
- Activity UI/UX lift and server-scoped Activity source-selection wiring
  (Phase 5) and the interactive chat UI (split to
  `260711-feat-ws-dashboard-agent-activity-chat-ui`).
- Overlay-tier capabilities (goal/loop auto-looping, subagent-as-nested-session
  modeling) — land in a later dedicated phase, not Phase 2.
- Hack-tier capabilities — all Codex matrix cells are Passthrough or Unavailable;
  none apply.
- Redesigning the Phase 1 `AgentClientProvider` **DTO shapes** — Phase 2
  implements against them as given. (Method async-ness is a Phase-2 decision the
  Phase 1 Result explicitly deferred; see Codebase Findings.)
- Realtime/audio (`thread/realtime/*`), review mode, memory, background
  terminals, and the other RPCs in the schema not named in the ticket's in-scope
  list.

## Codebase Findings

### A. Live event-sequencing — RESOLVED (real turn captured this pass)

Spawned `codex app-server --stdio`, drove a real `initialize` → `thread/start`
→ `turn/start` (trivial prompt "reply HELLO", `approvalPolicy: "never"`,
side-effect-free), captured the full notification stream, and terminated the
process cleanly (exit 0, no stray process left). **Framing is
newline-delimited JSON (one JSON-RPC object per line, `\n`-terminated) in both
directions — NOT LSP `Content-Length` framing.** Handshake is `initialize`
(request/response) followed by an `initialized` client notification (accepted
without error). Observed real ordering for one turn:

| # | direction | message | notes |
|---|---|---|---|
| 1 | C→S req | `initialize` | params `{clientInfo:{name,version}}` |
| 2 | S→C resp | (result) | `{userAgent, codexHome, platformOs, ...}` |
| 3 | S→C notif | `remoteControl/status/changed` | fires right after initialize |
| 4 | C→S notif | `initialized` | no id, no response |
| 5 | C→S req | `thread/start` | params `{cwd, approvalPolicy}` |
| 6 | S→C resp | (result) | `{thread:{id, sessionId, path, status:{type:"idle"}, historyMode, modelProvider, ...}}` — `thread.id` == `sessionId` |
| 7 | S→C notif | `thread/started` | duplicate thread object |
| 8 | S→C notif | `mcpServer/startupStatus/updated` ×N | one per configured MCP server (`codex_apps`, `wsflow`), `status:"starting"` — async, interleaves later |
| 9 | C→S req | `turn/start` | params `{threadId, input:[{type:"text",text}]}` |
| 10 | S→C resp | (result) | `{turn:{id, status:"inProgress", items:[], itemsView:"notLoaded"}}` |
| 11 | S→C notif | `thread/status/changed` | `{status:{type:"active"}}` |
| 12 | S→C notif | `turn/started` | `{threadId, turn}` |
| 13 | S→C notif | `mcpServer/startupStatus/updated` | `status:"ready"` (async, not ordered vs turn) |
| 14 | S→C notif | `item/started` (userMessage) | the client's own prompt echoed back as an item |
| 15 | S→C notif | `item/completed` (userMessage) | same item, terminal |
| 16 | S→C notif | `item/started` (agentMessage) | `{item:{type:"agentMessage", id, text:"", phase:"final_answer"}}` |
| 17 | S→C notif | `item/agentMessage/delta` ×N | `{threadId, turnId, itemId, delta}` incremental text |
| 18 | S→C notif | `item/completed` (agentMessage) | `{item:{..., text:"HELLO"}}` full final text |
| 19 | S→C notif | `thread/tokenUsage/updated` | `{threadId, turnId, tokenUsage:{total, last, modelContextWindow}}` — fires **before** `turn/completed` |
| 20 | S→C notif | `account/rateLimits/updated` | `{rateLimits:{primary, secondary, planType, ...}}` |
| 21 | S→C notif | `thread/status/changed` | `{status:{type:"idle"}}` |
| 22 | S→C notif | `turn/completed` | `{threadId, turn:{status:"completed", durationMs, items:[], itemsView:"notLoaded"}}` |

Projection-critical facts the schema alone did not reveal:

- **Items arrive only via the `item/*` notification stream, not in
  `turn/completed`.** `turn/completed.turn.items` is `[]` with
  `itemsView:"notLoaded"`. The adapter MUST accumulate items from the stream;
  it must NOT read them from the turn-completion payload.
- **Item lifecycle is `item/started` → type-specific `.../delta`* →
  `item/completed`.** Every notification carries `threadId`+`turnId`, and item
  events carry `item.id` / `itemId` for correlation. `item/completed` carries
  the fully-assembled item (e.g. `agentMessage.text` final), so a projector that
  cannot handle deltas can still fall back to `item/completed` snapshots.
- **The client's own prompt is echoed back as a `userMessage` item** (steps
  14-15). The projector must not double-render the browser's local send against
  this echo.
- **Token usage is its own `thread/tokenUsage/updated` notification** that
  precedes `turn/completed` (confirms the ticket; drives `activity.session.usage`
  read-only display, `modelContextWindow` = size).
- `ThreadItem.type` enum (schema): `userMessage`, `hookPrompt`, `agentMessage`,
  `plan`, `reasoning`, `commandExecution`, `fileChange`, `mcpToolCall`,
  `dynamicToolCall`, `collabAgentToolCall`, `subAgentActivity`, `webSearch`,
  `imageView`, `sleep`, `imageGeneration`, `enteredReviewMode`,
  `exitedReviewMode`, `contextCompaction`.
- Relevant delta/streaming notifications (schema `ServerNotification`):
  `item/agentMessage/delta`, `item/reasoning/textDelta`,
  `item/reasoning/summaryTextDelta`, `item/reasoning/summaryPartAdded`,
  `item/plan/delta`, `item/commandExecution/outputDelta`,
  `item/commandExecution/terminalInteraction`, `item/fileChange/outputDelta`,
  `item/fileChange/patchUpdated`, `item/mcpToolCall/progress`,
  `turn/diff/updated`, `turn/plan/updated`, `thread/compacted`,
  `thread/goal/updated`, `thread/goal/cleared`.
- **Approvals are server-initiated REQUESTS (have an `id`), not notifications.**
  Schema `ServerRequest` includes `CommandExecutionRequestApprovalParams`,
  `FileChangeRequestApprovalParams`, `PermissionsRequestApprovalParams`,
  `ToolRequestUserInputParams`, plus legacy `ExecCommandApprovalParams` /
  `ApplyPatchApprovalParams`. The reader loop must route messages that have both
  `method` and `id` to an approval/response path and send a JSON-RPC response
  back, distinct from fire-and-forget notifications (`method`, no `id`) and
  responses to our own requests (`id`, `result`/`error`, no `method`). With
  `approvalPolicy:"never"` no approval request fired this pass; the three-way
  message classification is still mandatory for the interactive path.
- The `initialize` result and every `thread/*` payload include daemon-private
  data that must stay behind the boundary: `thread.id`/`sessionId`,
  `thread.path` (e.g. `/home/.../.codex/sessions/...`), `codexHome`,
  `installationId`, rate-limit details. Only projected/derived Activity content
  and the dashboard-owned `activityId` cross to the browser.

Live-capture fixture data for the projection tests lives at
`<scratchpad>/spike-log.txt` (this pass) — the executor should re-run one
capture into a repo fixture (the ticket wants captured, not handwritten,
fixtures; see Verification Plan) rather than hand-transcribing this table.

### B. JSON-RPC-over-stdio duplex — framing/correlation/concurrency strategy

No JSON-RPC-over-stdio precedent exists in this codebase (`grep` for
`jsonrpc`/`json-rpc` across `crates/daemon/src` + `crates/core/src` is empty).
`terminal.rs` is the closest subprocess precedent but is PTY-byte-stream shaped
(blocking `std::thread` reader + `tokio::sync::watch` wake), not framed RPC.
Available dependencies (from `Cargo.toml`/`Cargo.lock`) that fit the confirmed
NDJSON framing **with no new crate**:

- `tokio` is a direct daemon dep with features `fs, io-util, macros, net,
  process, rt-multi-thread, signal, time`. This gives `tokio::process::Command`
  (spawn with `Stdio::piped()` on stdin/stdout/stderr) and, via `io-util`,
  `tokio::io::{BufReader, AsyncBufReadExt, AsyncWriteExt}`.
  `AsyncBufReadExt::lines()`/`read_line` reads exactly the confirmed
  `\n`-delimited frames — this is the framing primitive, no codec crate needed.
- `serde_json` (direct dep) for per-line `from_str`/`to_string`. A JSON-RPC
  message enum (`Response{id,result|error}` / `Notification{method,params}` /
  `ServerRequest{id,method,params}`) with `#[serde(untagged)]` or explicit
  presence checks classifies each parsed line into the three cases above.
- `tokio::sync` is available (terminal.rs already uses `tokio::sync::watch`), so
  `oneshot` (per-request response correlation), `mpsc`/`broadcast`
  (notification fan-out to the projector), and `Mutex` are all usable.
- **Rejected:** `tokio-util` 0.7.18 is present but only *transitively* (via
  `tokio-tungstenite`), and its `codec` feature (`LinesCodec`/`Framed`) is not
  guaranteed enabled; using it means adding `tokio-util` as a direct dep with
  `features=["codec"]`. `AsyncBufReadExt::lines()` already covers NDJSON with
  zero dependency delta, so prefer it — do not add `tokio-util` or any
  `jsonrpc`/`jsonrpsee` crate. `serde_json::StreamDeserializer` is also rejected:
  it is a synchronous `io::Read` consumer that does not compose with the async
  child stdout the daemon needs.

Recommended concurrency shape (grounded in the above and mirroring
`TerminalRegistry`'s lifecycle shape, not its blocking-thread I/O):

- One `tokio::spawn`ed **reader task** owns the child's stdout
  `BufReader::lines()`. Per line: parse, then dispatch — response → resolve the
  matching `oneshot` from a `Mutex<HashMap<i64, oneshot::Sender<...>>>` pending
  map; notification → push to the session's notification channel consumed by the
  projector; server-request (approval) → route to the approval handler which
  writes a response line back.
- **Writes are serialized through the same stdin handle** behind an
  `AsyncMutex<ChildStdin>` (or an mpsc write-queue task). Request ids come from a
  monotonic `AtomicI64`; a request registers its `oneshot::Sender` in the
  pending map *before* writing, then awaits the receiver with a `tokio::time`
  timeout.
- One reader task drains stderr into a bounded diagnostic buffer (reuse the
  bounded-`VecDeque` cap idea from `terminal.rs`, e.g. a `MAX_*` const).
- **`turn/steer` `expectedTurnId` race:** track the current in-flight `turnId`
  in session state, updated from `turn/started` (set) and `turn/completed`/
  `turn/*` terminal notifications (clear), under the same lock that serializes
  writes. `steer` reads the tracked active turn id under that lock and rejects
  with a bounded provider error if it does not match — no separate race guard
  is needed because the write lock already serializes the check-and-send.

### C. Codex plugin-presence detection — RESOLVED (no-session CLI surface exists)

Codex exposes a no-session plugin-listing CLI analogous to `claude plugin
list`: `codex plugin list --json` returns
`{installed:[{pluginId, name, marketplaceName, version, installed, enabled,
source, ...}], available:[]}` with **no session and no running app-server
required** (ran directly this pass; returned `wsflow@kang-sw-devenv`
`installed:true, enabled:true`). Precondition check: run `codex plugin list
--json`, parse `installed[]`, pass iff any entry has `name` ∈ {`ws`, `wsflow`}
with `installed==true && enabled==true`; otherwise refuse to spawn and surface
install guidance. This matches the epic's Claude-parity approach and needs no
config-file fallback.

Secondary surfaces found (do NOT use as the primary check): `codex mcp list`
(shows the wsflow MCP server registration, human-formatted) and the app-server
`plugin/list` RPC (client-request method exists) — but the RPC requires a live
app-server, defeating a *pre-spawn* gate, so the CLI `plugin list --json` is the
correct pre-spawn surface.

### D. Integration points

- `crates/core/src/agent_client_provider.rs#L231-L266` — the
  `AgentClientProvider` trait (`initialize`/`list_sessions`/`create_session`/
  `resume_session`/`send_prompt`/`interrupt`/`backfill_transcript`), synchronous
  today. **DTO shapes are fixed (out of scope to change); method async-ness is
  the Phase-2 decision the Phase 1 Result deferred.** Recommended split
  (see Implementation Plan step 1): a pure, synchronous **projection** mapper in
  `ws-dashboard-core` (event → `TranscriptBlock`/`ActivityItem`, fixture-tested
  with no runtime), plus the async **transport/session** owned by the daemon
  crate (which has tokio). This keeps fixture tests runtime-free per the
  verification boundary and confines the async choice to the daemon.
- `crates/core/src/agent_client_provider.rs#L40-L60` —
  `AgentClientCapabilities`; Codex `initialize` maps to `compact:true`,
  `steer:true`, `goal:true`, `fork:true`, `skills:true`, and `rewind:true`
  *only if* the executor decides to surface the deprecated `thread/rollback`
  (ticket says do not design around it — recommend `rewind:false` for Codex so
  no UI is built on a sunsetting RPC).
- `crates/core/src/activity.rs` — additive open-string `kind`/`render_kind`
  vocabulary (`agent.codex`, `thinking`, etc.) already exists from Phase 1; this
  phase is a consumer. Do not grow the vocabulary unless a concrete Codex shape
  is genuinely unrepresentable (ticket `#L590-L594`).
- `crates/daemon/src/terminal.rs#L138-L195` — `TerminalRegistry`
  (`Arc<RwLock<HashMap<String, Arc<TerminalSession>>>>`, `MAX_TERMINAL_SESSIONS`
  cap, `remove_for_work_roots`). Reuse this *lifecycle/registry* shape for a new
  `CodexProviderRegistry` (session cap, keyed so `serverId` participates,
  `remove_for_work_roots`), but NOT its blocking-thread I/O — transport is async
  tokio tasks (Finding B).
- `crates/daemon/src/router.rs#L63-L76` — `AppState` holds sibling registries
  (`terminals: TerminalRegistry`, `work_root_activity: WorkRootActivityProjector`).
  A new Codex provider registry is added here the same way.
- `crates/daemon/src/servers.rs#L636-L664`, `#L981-L1025`,
  `forward_server_scoped_activity_events#L1431` — existing Activity routes and
  their server-scoped forwarding wrappers
  (`/api/dashboard/servers/{serverRoute}/...`). New write routes for
  `activity.session.create/start/send`/gated controls follow this exact
  server-scoped wrapper pattern; `LOCAL_SERVER_ID` short-circuits to local.
- `crates/daemon/src/work_root_activity.rs#L92-L138`,
  `#L446-L498` — `WorkRootActivityProjector::project*` currently builds
  `ActivityFeed` from named-agent registry records; Codex-projected `items` must
  merge into the same `ActivityFeed.items` (never `agents`), per the ticket's
  public-interface briefing.
- `crates/daemon/src/config.rs#L18-L72`, `router.rs#L78-L88` — the loopback-only
  no-auth debug profile (`--no-auth`, loopback-bind-enforced) the WSL smoke path
  runs under.
- `servers.rs#L2179` is the only existing `tokio::process::Command` use (one-shot
  `ssh`); no persistent duplex child-process manager exists — this phase adds the
  first.

## Implementation Plan

1. **Pure Codex projection mapper in `ws-dashboard-core`** (sync, fixture-tested,
   no runtime). New module (e.g. `crates/core/src/codex_projection.rs`) that
   consumes the classified Codex event stream (a deserialized notification/item
   enum) and produces ordered `TranscriptBlock`s and `ActivityItem`/
   `ActivityConsoleEvent` updates. Contract/decisions:
   - Consume `item/started` → `item/*/delta` → `item/completed` per `item.id`;
     accumulate streamed text but treat `item/completed` as the authoritative
     final snapshot.
   - Map item `type` → `render_kind`: `agentMessage`→assistant text,
     `reasoning`(+`item/reasoning/*Delta`)→`thinking`, `commandExecution`/
     `mcpToolCall`/`dynamicToolCall`→tool/command block,
     `fileChange`(+`turn/diff/updated`)→file-change block, `userMessage`→user
     block **but suppress double-render of the browser's own just-sent prompt**,
     `contextCompaction`/`plan`→status blocks.
   - `thread/tokenUsage/updated` → usage state (used=`total.totalTokens`/`last`,
     size=`modelContextWindow`); not a transcript block.
   - Unknown item `type` or unknown notification `method` → one bounded
     diagnostic/status block; never emit raw provider JSON, paths, ids, or
     command output outside the selected transcript.
   - Strip/omit all daemon-private ids/paths (Finding A last bullet) — the mapper
     takes provider ids as input for correlation but never places them in
     browser-facing output.
2. **Async Codex transport/session in the daemon** (new module, e.g.
   `crates/daemon/src/codex_app_server.rs`). Implements Finding B:
   `tokio::process::Command` spawn of `codex app-server --stdio` with piped
   stdio; reader task (`BufReader::lines()` + `serde_json` three-way message
   classification); serialized writes behind `AsyncMutex<ChildStdin>`; request/
   response correlation via `AtomicI64` ids + `Mutex<HashMap<i64,
   oneshot::Sender>>` pending map with `tokio::time` timeout; notification
   fan-out channel feeding the step-1 projector; stderr drain into a bounded
   diagnostic buffer; in-flight-turn tracking for the `turn/steer`
   `expectedTurnId` guard.
   - Handshake: `initialize {clientInfo}` → await result → send `initialized`
     notification, before any `thread/*` call.
   - `thread/start` params include `cwd` (the work root) and an approval policy;
     capture the returned `thread.id` as daemon-private session state keyed to
     the dashboard `activityId`.
   - Approval server-requests (`item/*/requestApproval`, `ToolRequestUserInput`,
     legacy `execCommandApproval`/`applyPatchApproval`) are routed to the
     approval path and answered with a JSON-RPC response; wire to the dashboard
     approval relay per `260711-idea-dashboard-agent-facing-mcp-control-surface`
     (or, for this phase's non-interactive tests, a policy that responds
     deterministically).
3. **`CodexProviderRegistry` in the daemon**, mirroring `TerminalRegistry`'s
   registry/lifecycle shape (session cap, `remove_for_work_roots`, `is_live()`),
   keyed so `serverId` participates in the key (not `workRootId` alone). Added to
   `AppState` in `router.rs` alongside `terminals`.
4. **Plugin-presence spawn precondition.** Before spawning (step 2), run `codex
   plugin list --json`, parse `installed[]`, and refuse to spawn (returning
   install guidance, not a silent degrade) unless a `ws`/`wsflow` entry is
   `installed && enabled` (Finding C). This is a pre-spawn gate — do not use the
   app-server `plugin/list` RPC for it.
5. **`AgentClientProvider` impl for Codex** bridging step-2 transport + step-1
   projection to the Phase 1 trait DTOs. Map `initialize` capabilities per
   Finding D (`compact/steer/goal/fork/skills` = true; recommend `rewind` =
   false). If the trait must become async to drive the transport without
   blocking, that is the deferred Phase-2 async decision — keep the DTO shapes
   unchanged; prefer async methods over `block_on`/`block_in_place` inside axum
   handlers.
6. **Write routes + Activity merge.** Add server-scoped write routes for
   `activity.session.create/start/send` and the Codex-gated
   `compact/steer/goal/skills/fork` controls under
   `/api/dashboard/servers/{serverRoute}/...` following the `servers.rs`
   wrapper + `LOCAL_SERVER_ID` short-circuit pattern. Merge Codex-projected rows
   into `WorkRootActivityProjector`'s `ActivityFeed.items` (never `agents`);
   emit existing `ActivityConsoleEvent` variants (item upsert/remove, transcript
   update, snapshot invalidation, mode change, heartbeat) — do not grow the
   event vocabulary.

Rejected shortcut paths: (a) adding a `jsonrpc`/`jsonrpsee`/`tokio-util` crate
when `AsyncBufReadExt::lines()` + `serde_json` already cover the confirmed NDJSON
framing; (b) reading Codex's config/session files for the plugin gate when
`codex plugin list --json` is a clean no-session CLI surface; (c) reading turn
items from `turn/completed` (they are not there) instead of accumulating the
`item/*` stream; (d) forcing Codex rows into the legacy `agents` projection.

## Verification Plan

- **Projection fixture tests (core, TDD-friendly, no runtime):** capture one
  real `codex app-server` turn into a repo fixture (re-run the spike; the
  scratchpad `spike-log.txt` and the Finding-A table are the reference shape,
  not the committed fixture) and assert the step-1 mapper produces the expected
  ordered `TranscriptBlock`s, suppresses the `userMessage` echo, folds
  `agentMessage` deltas into one assistant block, emits a `thinking` block for
  `reasoning` items, and degrades an injected unknown item type to a bounded
  diagnostic with no raw JSON/paths.
- **Privacy/route tests (daemon):** browser payloads for a Codex Activity feed
  omit `thread.id`/`sessionId`, `thread.path`, `codexHome`, and raw event ids;
  only `activityId` and projected content cross the boundary.
- **Plugin-gate test:** spawn is refused with install guidance when `codex
  plugin list --json` reports neither `ws` nor `wsflow` `installed && enabled`
  (inject a fake `plugin list` result / stub the command).
- **Transport unit tests:** three-way message classification (response vs
  notification vs server-request), request/response id correlation with timeout,
  and the `turn/steer` `expectedTurnId` mismatch rejection — drivable against a
  scripted in-process NDJSON peer, no real binary needed.
- **WSL smoke (manual, gated on binary present):** under the loopback no-auth
  debug profile, spawn `codex app-server --stdio`, drive a real turn plus one
  Codex-native `skills/list` and one `thread/compact/start` → `thread/compacted`
  round-trip, and confirm it appears in and is driveable from the Activity
  Console without owner pairing.

## Escalations

None. Both prior escalation reasons are resolved with direct evidence captured
this pass: (1) the live `item/*`/`turn/*` event sequence and NDJSON framing are
captured from a real turn (Finding A); (2) a concrete, dependency-free framing/
correlation/concurrency strategy grounded in the daemon's existing `tokio` +
`serde_json` deps is specified (Finding B). The Codex plugin-presence surface is
confirmed (`codex plugin list --json`, Finding C). One deliberate decision left
to the executor within guardrails: whether the `AgentClientProvider` trait
methods become async (recommended) — this preserves the Phase 1 DTO shapes and
only exercises the async choice the Phase 1 Result explicitly deferred to Phase 2.
