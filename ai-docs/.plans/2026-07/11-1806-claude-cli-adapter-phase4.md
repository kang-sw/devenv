# Plan: 260620-feat-ws-dashboard-agent-client-activity-sources — Phase 4: Claude CLI stream-json duplex adapter

> Research pass 2026-07-11: replaced the prior survey/escalation output. All
> three escalation blockers are now resolved with direct evidence captured this
> pass by driving the installed `claude` 2.1.207 binary through real stream-json
> sessions (multi-turn keep-alive, a `PreToolUse` hook deny, a client→CLI
> `control_request` interrupt, and `--resume` across clean exit / hard kill /
> cwd change). Escalations: None.

## Relevant Ticket Contract

- Add a Claude provider that drives the `claude` CLI directly in headless
  `--input-format stream-json --output-format stream-json` mode (no Agent SDK
  dependency); build a dashboard-owned duplex bridge and map the CLI's NDJSON
  events into the Phase 1 `AgentClientProvider` subset, projecting into the same
  Activity model as Codex/OpenCode (ticket Phase 4, `#L932-L953`).
- Approval/permission interception goes through CLI **hooks** (`PreToolUse`),
  not `--dangerously-skip-permissions`; the dangerously-bypass mode maps to
  `--permission-mode bypassPermissions` only on explicit per-session human
  opt-in (`#L941-L946`, Decisions `#L100-L116`).
- Process lifecycle is kill-and-respawn-via-`--resume` for idle sessions,
  rendered opaquely as the same ongoing conversation (`#L947-L953`, Decisions
  `#L117-L133`).
- Enforce the ws/wsflow plugin-presence spawn precondition via `claude plugin
  list` (already confirmed a no-session surface); refuse to spawn with install
  guidance if neither `ws` nor `wsflow` is installed/enabled (`#L955-L961`).
- Implement only the opinionated slice the dashboard contract needs; do not
  attempt full CLI feature coverage (Decisions `#L144-L153`).
- Browser-identity rule (Phase 1, unchanged): only the dashboard-owned
  `activityId` crosses the browser boundary. Provider `session_id`,
  `transcript_path`, `cwd`, model/tool names, process ids, and raw event JSON
  stay daemon-private (`crates/core/src/agent_client_provider.rs#L14-L22`).
- Verification boundary (`#L973-L980`): fixture projection tests captured from
  the spike (not handwritten from unofficial docs), bounded-degradation tests
  (missing binary/auth, hook-misconfiguration, incompatible version), a
  spawn-refusal test when the plugin gate fails, and route tests matching the
  Codex adapter's privacy/identity constraints.
- Cross-Harness Feature Matrix (`#L462-L475`): Claude is Passthrough for
  resume / create / send-receive (documented event subset) / permission
  interception (PreToolUse hooks) / context-usage display (`result` token
  counts); Unavailable for compact / steer / goal; Hack (out of Phase 4) for
  rewind / fork; skills is split Passthrough(plugin)+Overlay(fs scan).

## Out of Scope

- Phase 1 contract, Phase 2 Codex adapter, Phase 3 OpenCode adapter, Phase 5
  Activity UI — read as reuse precedent only.
- The SDK's `control_request`-based `can_use_tool` permission callback path (see
  Finding A4): the design uses `PreToolUse` hooks, not this path. `control_request`
  itself is real and confirmed (used for `interrupt`), but the CLI→client
  permission-prompt direction only activates with the SDK's
  `--permission-prompt-tool` MCP registration, which this phase does not adopt.
- Hack-tier rewind/fork (transcript-file truncation) — separate idea ticket with
  owner risk sign-off, per the Feature Matrix.
- Manual compaction (`/compact`), `steer`, and `goal` — Unavailable for Claude;
  the Claude adapter reports these capabilities as `false`.
- Redesigning the Phase 1 `AgentClientProvider` DTO shapes or the async trait
  decision (settled in Phase 2). Phase 4 implements the same async trait.
- Loose/project `SKILL.md` filesystem-scan half of `activity.session.skills`
  (Overlay tier) — land with the gated skills control, not the core adapter.

## Codebase Findings

### A. Live stream-json spike — RESOLVED (real sessions captured this pass)

Environment: `claude` 2.1.207. Invocation used:
`claude -p --input-format stream-json --output-format stream-json --verbose`
(add `--include-partial-messages` for token-level `stream_event` deltas).
`-p`/`--print` is mandatory; stream-json output requires `--verbose`. User input
is one NDJSON object per line on stdin; output is NDJSON on stdout.

**A1. Real single-turn event ordering** (safe prompt, no tools). Each `type` is
the top-level discriminator (Claude output is *typed events*, NOT JSON-RPC
responses/notifications like Codex):

| # | `type` | notes |
|---|--------|-------|
| 1 | `system` (`subtype:"init"`) | `session_id`, `tools[]`, `mcp_servers[]`, `model`, `permissionMode`, `slash_commands[]`, `plugins[]` (each `{name,path,source}`), `skills[]`, `capabilities[]` (`interrupt_receipt_v1`,`msg_lifecycle_v1`), `cwd`, `claude_code_version` |
| 2 | `rate_limit_event` | `rate_limit_info{status,resetsAt,...}` — protocol-control, no transcript effect |
| 3 | `assistant` | `message.content[]` = full Anthropic content blocks; here one `text` block. Carries `message.usage`, `session_id`, `parent_tool_use_id` |
| 4 | `result` (`subtype:"success"`) | turn terminal. `result` (final text), `session_id`, `num_turns`, `total_cost_usd`, `usage{input_tokens,output_tokens,cache_*}`, `modelUsage{<model>:{contextWindow,...}}`, `permission_denials[]`, `is_error` |

**A2. Multi-turn keep-alive duplex — CONFIRMED.** A single long-lived process
handled two turns over one open stdin (same `session_id` throughout); it exits
only when stdin closes. **Each turn re-emits its own `system/init` +
`system/status` at the start and a `result` at the end** — so a turn boundary is
a `result` event, and `system/init` is a per-turn metadata refresh, NOT a
session reset. With `--include-partial-messages`, each turn also streams
Anthropic-SSE-shaped `stream_event` frames: `message_start` →
`content_block_start` → `content_block_delta` (`delta.type:"text_delta"`, etc.)
→ `content_block_stop` → `message_delta` (stop_reason+usage) → `message_stop`.

**A3. Tool-call turn shape.** For a turn that calls a tool: `assistant` with a
`thinking` block (`{type:"thinking",thinking,signature}`), then `assistant` with
a `tool_use` block (`{type:"tool_use",id:"toolu_...",name:"Bash",input:{...}}`),
then a `user` event whose `message.content[]` holds a `tool_result`
(`{type:"tool_result",tool_use_id,content,is_error}`), then `assistant` text,
then `result`. **Claude does NOT echo the client's own sent user prompt back as
an output event by default** (unlike Codex's `userMessage` item echo) — so the
projector needs no prompt-echo suppression unless `--replay-user-messages` is
passed (do not pass it).

**A4. Permission interception in plain stream-json — tools auto-run; no
CLI→client `control_request`.** With `--permission-mode default` and no hook, a
`Bash pwd` request executed immediately (`tool_result is_error:false`,
`result.permission_denials:[]`) — the CLI did NOT emit a `control_request`
asking the client for permission. The `can_use_tool` control-callback direction
is an SDK-only mechanism (SDK registers `--permission-prompt-tool`); it is not on
this phase's path. **This resolves survey blocker 1**: the design must rely on
`PreToolUse` hooks (Finding D), not a CLI→client `control_request` permission
prompt.

**A5. `control_request`/`control_response` IS real (client→CLI).** Writing
`{"type":"control_request","request_id":"ctl-req-1","request":{"subtype":"interrupt"}}`
to stdin during an active turn produced
`{"type":"control_response","response":{"subtype":"success","request_id":"ctl-req-1","response":{"still_queued":[]}}}`
on stdout and cut the turn short (`result subtype:"error_during_execution"`).
So the bidirectional control shape is confirmed against the real binary and is
the correct mechanism for the provider `interrupt` method — the third-party doc
shape held for the interrupt subtype. (Only the *permission-prompt* use of
`control_request` is out of scope per A4.)

### B. `--resume` lifecycle — RESOLVED (blocker 2)

Session created with `--session-id <uuid>` (the flag is honored; lets the daemon
control the id). The transcript is written to
`~/.claude/projects/<mangled-cwd>/<session-id>.jsonl`, where `<mangled-cwd>` is
the spawn cwd path with separators replaced — i.e. **resume lookup is keyed by
cwd**, matching the official "scoped to current project directory" note.

- **Resume after clean exit, same cwd — WORKS.** `claude -p --resume <sid> ...`
  recalled a fact set in the prior turn; `session_id` preserved, `num_turns`
  resets per invocation.
- **Resume after a hard SIGKILL mid-turn, same cwd — WORKS and is
  non-destructive.** The `.jsonl` is appended incrementally as events occur
  (line count grew 14→17 across a killed mid-turn essay), and a subsequent
  `--resume` from the same cwd succeeded (`subtype:"success"`, recalled the
  prior codeword). A hard kill does not corrupt the session; the interrupted
  turn's partial output is persisted but the session cleanly accepts the next
  turn. **This validates the kill-and-respawn-via-`--resume` lifecycle.**
- **Resume from a DIFFERENT cwd — FAILS HARD.** `--resume <sid>` from a fresh
  directory produced `stderr: "No conversation found with session ID: <sid>"`,
  `result subtype:"error_during_execution"`, `errors:["No conversation
  found..."]`, exit 1. **Hard constraint**: the daemon MUST persist the original
  spawn cwd (the work-root path) in session state and always respawn/resume from
  exactly that cwd. A work-root path change between kill and resume breaks
  resume with no fallback.

**Lifecycle design consequences.** (1) Keep the in-memory `ClaudeProjector` and
session metadata in the daemon `ClaudeSession` struct across respawns — resume
does NOT replay prior turns to stdout, so accumulated transcript state must
survive the child process, not be re-read. (2) On idle-timeout kill, drop only
the child/transport (kill_on_drop), keep the session record with its
`session_id` + `cwd`; the next `send_prompt`/`resume_session` respawns
`claude -p --resume <session_id>` in the stored cwd. (3) Detect a killed turn:
`result subtype:"error_during_execution"` (interrupt) vs `"success"`.

### C. Plugin-presence gate — RESOLVED (`claude plugin list --json`)

`claude plugin list --json` is a no-session CLI surface returning a JSON **array**
of `{id, version, scope, enabled, installPath, installedAt, projectPath?}`. `id`
is `<name>@<marketplace>` (e.g. `ws@kang-sw-devenv`, confirmed `enabled:true`).
**The shape differs from Codex's `codex plugin list --json`** (Codex nests
`{installed:[{name,installed,enabled}]}`; Claude is a flat top-level array with
`id` and only `enabled` — presence in the array means installed). So the Claude
gate needs its OWN parser, NOT reuse of `evaluate_plugin_gate` from
`codex_app_server.rs`. Gate rule: parse the array, pass iff any entry has
`id.split('@').next() ∈ {"ws","wsflow"}` and `enabled == true`; otherwise refuse
with install guidance. `--json` is the machine-readable surface; the earlier
skill-listing spike's human-readable output is superseded here.

### D. `PreToolUse` hooks in headless stream-json — RESOLVED (blocker 3)

A `PreToolUse` hook configured for matcher `Bash` FIRED in headless stream-json
mode and successfully BLOCKED the tool. Confirmed contract:

- **Hook stdin** (JSON): `{session_id, transcript_path, cwd, prompt_id,
  permission_mode, effort, hook_event_name:"PreToolUse", tool_name, tool_input,
  tool_use_id}`. This gives the hook everything needed to identify the session
  and the exact tool invocation.
- **Hook stdout** (deny): `{"hookSpecificOutput":{"hookEventName":"PreToolUse",
  "permissionDecision":"deny","permissionDecisionReason":"<text>"}}` with exit
  0. (`allow`/`ask` are the symmetric decisions; exit code 2 also blocks.)
- **How the block surfaces in the CLI's stream-json output**: the `tool_use`
  runs into a `user` `tool_result` with `is_error:true` and `content` = the
  decision reason, and the final `result` carries
  `permission_denials:[{tool_name, tool_use_id, tool_input}]`. The assistant
  then continues (explains the block). So the dashboard can observe both the
  denial (via `result.permission_denials`) and the reason (via the tool_result).

**Design mechanism for the approval relay** (`260711-idea-dashboard-agent-facing-mcp-control-surface`):
the hook command is a dashboard-owned executable that reads the tool_input from
stdin, calls back to the daemon over the loopback control surface to obtain the
human's allow/deny/ask decision, and emits the corresponding
`hookSpecificOutput`. Correlation is by `session_id`+`tool_use_id` from the hook
stdin. **Inject the hook config via `--settings <json-or-file>` at spawn** (the
flag exists and layers settings) rather than writing into the user's project
`.claude/settings.json` — this keeps the dashboard's hook out of the user's
tree and avoids two divergent settings paths. The hook command path must be
absolute (dashboard-owned). This resolves survey blocker 3: hooks are the sound,
confirmed interception point in this mode.

### E. Integration points (mirror the Phase 2 Codex structure exactly)

- `crates/core/src/agent_client_provider.rs#L240-L275` — the async
  `AgentClientProvider` trait Phase 4 implements (unchanged DTOs).
  `AgentClientCapabilities#L38-L47` → Claude sets `compact:false, steer:false,
  goal:false, rewind:false, fork:false, skills:true`.
- `crates/core/src/codex_projection.rs` — precedent shape for the pure,
  runtime-free `ClaudeProjector` (`BlockState`/`order`/`blocks` map, `bound_text`
  caps, `diagnostics`, `CODEX_RENDER_KIND_*` → `CLAUDE_RENDER_KIND_*`
  equivalents reusing the same open `TranscriptBlock::render_kind` vocabulary:
  `markdown`/`thinking`/`tool`/`fileChange`/`status`). Reuse the same
  degrade-to-bounded-diagnostic discipline. Register in `crates/core/src/lib.rs`
  next to `pub mod codex_projection;` (`#L3`, `#L26`).
- `crates/daemon/src/codex_app_server.rs` — precedent for the async transport +
  registry + gate + provider. The Claude transport reuses the SAME dependency
  set (`tokio::process::Command`, `AsyncBufReadExt::lines()` NDJSON framing,
  `serde_json`, `AsyncMutex<ChildStdin>`, `kill_on_drop`, bounded stderr drain,
  `mpsc` notification fan-out to a `spawn_projector_pump`) with NO new crate.
  Key differences from Codex to encode: (a) classify output by top-level `type`,
  not JSON-RPC id/method three-way; (b) a "turn" is: write one user-message line
  → drain events until the next `result` line; there is no per-request `oneshot`
  id correlation — correlation is turn-sequential; (c) `interrupt` writes a
  `control_request` interrupt line and awaits the matching `control_response`
  (by `request_id`); (d) the session record holds `session_id` + spawn `cwd` and
  survives child respawn (Finding B); (e) a Claude-specific `PluginGateRefusal`
  parser (Finding C).
- `crates/daemon/src/codex_routes.rs` + `router.rs#L179-L196`/`#L365-L382`
  (dual server-scoped + local route registration) + `servers.rs#L676-L720`/
  `#L1083-L1170` (`ServerScopedForwardOperation` + `LOCAL_SERVER_ID`
  short-circuit) — precedent for a `claude-sessions` route family. Add
  `pub claude_sessions: ClaudeProviderRegistry` to `AppState`
  (`router.rs#L77`), init `ClaudeProviderRegistry::default()` in
  `server.rs#L94`, and add `state.claude_sessions.remove_for_work_roots(...)` at
  the two cleanup call sites (`root_picker.rs#L368`, `resources.rs#L37`).
- `crates/daemon/src/work_root_activity.rs#L205-L216` — the `merge_activity_items`
  step-6 unified-feed merge; add a sibling `claude_activity_items(...)` merge so
  Claude sessions appear in `ActivityFeed.items` (never `agents`), exactly like
  Codex.
- `crates/daemon/src/codex_routes.rs#L34-L60`, `AppStateResolver` /
  `resolve_online_available_work_root` — the work-root→cwd resolver the Claude
  provider reuses to get the spawn/resume cwd (which Finding B makes
  load-bearing: it is also the resume key).

## Implementation Plan

1. **Pure Claude projection mapper** — new `crates/core/src/claude_projection.rs`,
   a `ClaudeProjector` following `CodexProjector`'s pure/runtime-free shape.
   Contract:
   - Ingest by top-level `type`. Authoritative content source is the `assistant`
     event's complete `message.content[]` blocks plus the `user` event's
     `tool_result`; `stream_event` deltas are an optional live-append path
     (`content_block_delta.delta.text`) that `assistant`/`result` overwrite
     authoritatively — same "completed snapshot wins" rule as the Codex
     projector.
   - Block mapping: content-block `text` → `markdown` "Assistant";
     `thinking` → `thinking` "Reasoning" (drop `signature`); `tool_use` → `tool`
     block titled by `name` with a bounded, path-free input summary (never the
     raw `input` payload); `user` `tool_result` → update the correlated tool
     block (by `tool_use_id`) with a bounded status (`completed`/`failed` from
     `is_error`) and bounded content; `result.usage`/`modelUsage` → read-only
     `ClaudeUsage {used_input_tokens, used_output_tokens, context_window}` (from
     `modelUsage.<model>.contextWindow`), not a transcript block; `system`
     (init/status) and `rate_limit_event` → recognized protocol-control,
     Ignored (no transcript block); unknown `type` or unknown content-block
     `type` → one bounded diagnostic status block, never raw JSON/paths/ids.
   - Turn state: `result` → turn terminal (record `subtype` — `success` vs
     `error_during_execution`); a fresh `assistant`/`stream_event message_start`
     after a `result` begins the next turn. No prompt-echo suppression needed
     (Finding A3).
   - Strip all daemon-private fields: `session_id`, `transcript_path`, `cwd`,
     `request_id`, `uuid`, model/tool internal ids, `signature`. The projector
     takes ids only for `tool_use_id` correlation and never emits them.
2. **Async Claude transport/session** — new `crates/daemon/src/claude_cli.rs`.
   Spawn `claude -p --input-format stream-json --output-format stream-json
   --verbose --session-id <uuid> --settings <hook-config>` (add
   `--include-partial-messages` when live deltas are wanted) with piped stdio,
   `current_dir(cwd)`, `kill_on_drop(true)`. Reader task: `BufReader::lines()`,
   classify each line by `type`, route `control_response` by `request_id` to a
   pending map, feed `system`/`assistant`/`user`/`stream_event`/`result`/
   `rate_limit_event` to the projector pump, track turn boundaries on `result`.
   Writer behind `AsyncMutex<ChildStdin>`: `send_prompt` writes a `{"type":"user",
   "message":{"role":"user","content":[{"type":"text","text":...}]}}` line and
   awaits the next `result`; `interrupt` writes a `control_request` interrupt and
   awaits its `control_response`. Bounded stderr drain (reuse
   `MAX_STDERR_LINES`). Resume/respawn: store `session_id` + `cwd`; when the
   child is absent (idle-killed) or dead, respawn with `--resume <session_id>`
   from the stored `cwd` (Finding B constraint), preserving the existing
   projector.
3. **`ClaudeProviderRegistry`** mirroring `CodexProviderRegistry`: keyed by
   `(server_id, activity_id)`, `MAX_CLAUDE_SESSIONS` cap, `is_live`, `remove`,
   `remove_for_work_roots`, `session_for`, `insert_session_for_tests`. Add to
   `AppState`, init in `server.rs`, wire both work-root cleanup sites.
4. **Plugin-presence spawn gate** — Claude-specific `evaluate_claude_plugin_gate`
   parsing the flat `claude plugin list --json` array (Finding C): pass iff some
   entry's `id` name-part ∈ {`ws`,`wsflow`} and `enabled`. Own `PluginGateRefusal`
   with `claude.plugin_gate` code + install guidance. Enforce in `initialize`
   and before any spawn in `create_session`, exactly as the Codex provider does.
5. **`PreToolUse` hook config injection + approval-relay seam** — at spawn, pass
   `--settings` pointing at a dashboard-generated settings object that registers a
   `PreToolUse` hook whose `command` is an absolute dashboard-owned executable
   (Finding D). The executable relays `{session_id, tool_use_id, tool_name,
   tool_input}` to the daemon's loopback control surface and emits the returned
   `hookSpecificOutput` decision. For this phase's non-interactive default path
   and tests, the hook may deny-by-default or allow a fixed safe set; the live
   interactive relay integrates with
   `260711-idea-dashboard-agent-facing-mcp-control-surface`. The dangerously-
   bypass opt-in maps to spawning with `--permission-mode bypassPermissions`
   instead of the hook config, only on explicit human opt-in. Reject the SDK
   `--permission-prompt-tool`/`control_request` permission path (Finding A4).
6. **`AgentClientProvider` impl (`ClaudeCliProvider`) + routes + feed merge.**
   Implement the trait bridging step-2 transport + step-1 projector; capabilities
   per Finding E (only `skills:true`). Add a `claude-sessions` route family
   (create/list/prompt/interrupt/transcript) with dual local + server-scoped
   registration mirroring `codex_routes.rs`/`servers.rs`/`router.rs`, keyed by
   `serverId`, `LOCAL_SERVER_ID` short-circuit, and a Claude-specific
   `provider_error_response` status map (reuse the Codex code→status mapping;
   `claude.plugin_gate`→424, `claude.unknown_session`→404, `claude.timeout`→504,
   etc.). Add `claude_activity_items(...)` and merge into
   `work_root_activity.rs`'s step-6 `merge_activity_items` (into `items`, never
   `agents`).

Rejected shortcut paths: (a) adding an SDK dependency or a JSON-RPC/`tokio-util`
crate — plain NDJSON over `AsyncBufReadExt::lines()` + `serde_json` covers the
confirmed framing; (b) using a CLI→client `control_request can_use_tool`
permission path — it does not fire in plain stream-json (Finding A4); hooks are
the mechanism; (c) mutating the user's project `.claude/settings.json` for the
hook — inject via `--settings` instead; (d) reusing Codex's `evaluate_plugin_gate`
parser — Claude's `plugin list --json` shape differs (Finding C); (e) re-reading
the on-disk `~/.claude/projects/*.jsonl` transcript for backfill (leaks paths and
resume does not replay) — keep the in-memory projector alive across respawns; (f)
resuming from any cwd other than the original work-root cwd — it fails hard
(Finding B); (g) forcing Claude rows into the legacy `agents` projection.

## Verification Plan

- **Projection fixture tests (core, no runtime):** capture one real Claude
  stream-json turn (a text turn and a tool-call turn) into a repo fixture (e.g.
  `crates/core/tests/fixtures/claude-cli-turn.ndjson`) and assert the
  `ClaudeProjector` produces ordered `TranscriptBlock`s (assistant text,
  thinking, tool + tool_result correlation), reads usage/context-window from
  `result.modelUsage`, and degrades an injected unknown `type`/content-block to
  a bounded diagnostic with no raw JSON/paths/ids/`signature`. Reference shapes:
  Findings A1/A2/A3 (the scratchpad spike NDJSON under
  `scratchpad/spike/{sess1,sess2,sess3,hookproj}` is the capture source; re-run
  one clean capture into the committed fixture).
- **Privacy/route tests (daemon):** browser payloads for a Claude Activity feed
  omit `session_id`, `transcript_path`, `cwd`, model/tool ids, and raw event
  JSON; only `activityId` + projected content cross. Mirror
  `codex_app_server::browser_payloads_omit_provider_ids_and_paths`.
- **Plugin-gate test:** `evaluate_claude_plugin_gate` passes for a flat array
  containing `{"id":"ws@x","enabled":true}` and refuses (with install guidance)
  when neither `ws` nor `wsflow` is present or when the sole match is
  `enabled:false`. Spawn refusal returns 424.
- **Transport unit tests (scripted in-process NDJSON peer, no binary):**
  `type`-based line classification (`system`/`assistant`/`user`/`result`/
  `control_response`); a user-message write resolves on the next `result`;
  `interrupt` writes a `control_request` and resolves on the matching
  `control_response`; EOF fails outstanding awaits.
- **Hook-block test:** feed a projector/transport a captured hook-denied turn
  (Finding D) and assert the tool block renders a bounded "blocked" status and
  the feed does not leak the reason path; assert `result.permission_denials` is
  surfaced as a bounded diagnostic/permission state.
- **Resume-lifecycle test (manual, gated on real binary):** create a session with
  a known `--session-id`, kill the child, `resume_session`/`send_prompt` respawns
  `--resume` from the stored cwd and recalls prior context; assert a
  work-root-cwd mismatch surfaces a bounded provider error rather than a silent
  hang (Finding B).
- **WSL smoke (manual, `#[ignore]`, gated on installed+authenticated `claude`):**
  under the loopback no-auth debug profile, plugin gate → spawn → create session
  → a real safe turn (e.g. "reply PONG") appears in and is driveable from the
  Activity Console; a `PreToolUse`-denied Bash turn surfaces the denial; an
  `interrupt` cuts a long turn short. Child killed on drop, no stray process.

## Escalations

None. All three survey blockers are resolved with direct evidence captured this
pass against `claude` 2.1.207: (1) the `control_request`/`control_response`
shape is real (interrupt round-trip, Finding A5), and permission interception in
plain stream-json does NOT use a CLI→client `control_request` (Finding A4) — the
design correctly relies on `PreToolUse` hooks; (2) `--resume` works across clean
exit and hard SIGKILL from the same cwd and fails hard from a different cwd, so
the kill-and-respawn lifecycle is safe iff the daemon pins the work-root cwd
(Finding B); (3) `PreToolUse` hooks fire and can allow/deny/ask in headless
stream-json mode, with a confirmed stdin/stdout/exit contract and an observable
`permission_denials` surface (Finding D). The plugin gate surface
(`claude plugin list --json`, distinct shape from Codex) is confirmed
(Finding C). No blocker remains for a fresh executor.
