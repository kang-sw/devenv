# Plan: 260903-feat-ws-pi-goal-loop-compaction-hook — Phase 1: Goal-mode arming + agent_settled loop + terminal levers

## Relevant Ticket Contract

- Register the goal-entry **command** `/goal <goal>`: inject the announcement
  `"Goal settled: <goal>"` and set an active-goal marker.
- Arm the `agent_settled` handler **only while a goal is active**; a settle
  outside goal mode is an ordinary stop (no re-fire).
- On each armed re-fire, inject a reminder carrying the goal + the levers
  (ticket's own example: `"Goal yet running … <goal> … call /goal-achieved |
  /goal-blocked | /goal-compact-and-continue for a state transition."` — Phase
  1 drops the `/goal-compact-and-continue` mention since that lever doesn't
  exist yet).
- Two terminal levers end the run: `/goal-achieved <summary>` and
  `/goal-blocked <reason>`. Design section states the signal shape is
  **"explicit skill calls, zero prose parsing"** — state transitions happen
  only through model-invoked calls, never by parsing response text.
- Runaway backstop: **N consecutive re-fires with no tool call** force-stop
  the goal loop (default 10, config-tunable). "Force-stop" means stop
  re-injecting / disarm goal mode — Pi has no session-kill primitive that
  fits here (`ctx.shutdown()` exits the whole process, not what's wanted).
- Config knob (runaway threshold) is **adapter-owned data-file config**,
  built-in constant default overridden by a file read fresh per use — the
  `model-catalog.json` sibling precedent (ticket's "Compaction ownership"
  paragraph explicitly extends this pattern to "the Phase 1 runaway
  threshold"), never in ws-mcp.
- Settled cross-ticket fact (Background/"Remaining open questions", applies
  to the whole goal-loop mechanism this phase builds): the goal-loop runs on
  the **lead session only**; the `agent_settled` handler is a **no-op when
  the process is a spawned child** (RPC workers driven via `ws-agent-send`
  get no settle re-injection or compaction lever).
- Verification boundary (five points, from Phase 1 text): (a) goal entry arms
  the loop, (b) a settle re-injects the reminder, (c) `/goal-achieved` and
  `/goal-blocked` each terminate the loop, (d) a non-goal session settle does
  NOT re-fire, (e) the runaway backstop force-stops after the configured
  count — verified via a live `pi … --mode json` transcript. "Loop-guard /
  threshold logic unit-tested where seam-extractable."
- Non-goal (ticket-wide): no ws-mcp (Go) changes; the loop is adapter-local.
  No compaction lever in this phase (`/goal-compact-and-continue`,
  `getContextUsage().percent` surfacing, `session_before_compact` — all
  Phase 2).

## Out of Scope

- Phase 2: `/goal-compact-and-continue`, `getContextUsage().percent`
  surfacing in the reminder, the compression-safety-heuristic advisory, the
  compaction-advisory-point / context-window-override config knobs, and the
  `session_before_compact` companion. None of this is touched in Phase 1.
- Durable goal state across compaction / whether `/goal-blocked` writes a
  durable ticket record — explicitly deferred post-2026-09-03, not blocking
  Phase 1 (Phase 1 verification is single-session).
- Editing `ai-docs/spec/pi-adapter-runtime.md` anchors — per the ticket's own
  "Contract-first: yes … at proceed via `lead-write-spec`" and the sibling
  `260903-feat-ws-pi-subagent-rpc-ux` Phase 1 plan's identical precedent, spec
  anchor writing is a proceed-time step, not part of this code-implementation
  plan. (Confirmed live: `ai-docs/spec/pi-adapter-runtime.md` currently has no
  `🚧`/`260904` goal-loop anchors yet — only the closing Constraints
  blockquote at L359-365 lists the goal-loop as still deferred.)
- The delegation spawner's existing RPC/explore machinery
  (`agents-plugin-pi/src/spawner.ts`'s spawn/send/wait/list/stop tools) is
  untouched except for the one-line child-marker addition called out below.

## Codebase Findings

- `agents-plugin-pi/src/index.ts#L71-134` — the extension factory. Commands
  and tools (`ws-model-catalog-list`, `ws-discuss`) are registered directly
  in the factory body, **before** `pi.on("session_start", …)` — the file's
  own doc comment confirms "command/tool registration is declarative and not
  gated behind session_start; only subprocess spawning is." The goal-loop
  needs no subprocess, so it registers the same way: a `registerGoalLoop(pi,
  …)` call at factory top level, not inside `session_start`.
- `agents-plugin-pi/src/discuss.ts` (whole file) and `model-catalog.ts` (whole
  file) — the established pure-helper convention: pure, unit-tested builder
  functions with no `pi.*`/IO calls, imported by a thin IO call site. `bridge.ts`
  and `spawner.ts` instead mix pure exported helpers **and** the
  `pi.registerTool`/`registerCommand` IO glue in one file
  (`bridge.ts#L120-178` pure next to `startBridge` IO at `#L180-279`;
  `spawner.ts` throughout). The goal-loop's IO surface (1 command + 2 tools +
  2 event listeners) is closer in shape to spawner.ts than to discuss.ts's
  single call site, so a new `agents-plugin-pi/src/goal-loop.ts` should follow
  the bridge.ts/spawner.ts convention: pure state-machine functions plus one
  `registerGoalLoop(pi, opts)` IO function, all in one file.
- `agents-plugin-pi/src/model-catalog.ts#L45-57` (`readModelCatalog`) — exact
  pattern to copy for a new `readGoalLoopConfig(path)`: `readFileSync` +
  `JSON.parse`, both wrapped in `try/catch` returning `undefined` on any
  failure (missing file, bad JSON) — "never hard-fail," read fresh on every
  call (no caching).
- `agents-plugin-pi/model-catalog.json` (tracked in git, currently `3` bytes,
  content `{}`) and `package.json#L14-20`'s `files` whitelist (`"src/",
  "bin/", "rsrc/", "skills/", "runtime.json", "model-catalog.json"`) — the
  sibling-data-file precedent to copy for the new runaway-threshold config
  file: create `agents-plugin-pi/goal-loop-config.json` shipping as `{}`,
  tracked in git (no `.gitignore` entry excludes it — confirmed no
  `agents-plugin-pi/.gitignore` exists), and add it to `package.json`'s
  `files` array or it will be silently dropped from a published tarball.
- `agents-plugin-pi/src/spawner.ts#L1113-1301` (tool registration examples,
  e.g. `ws-report-to-lead` at `#L1249-1270`) — the exact
  `pi.registerTool({name, label, description, parameters: {type:"object",
  properties:{...}, required:[...]} as never, async execute(...) {...}})`
  shape to copy for `goal-achieved`/`goal-blocked`. `ws-report-to-lead`'s
  `execute()` is a plain string-in/string-out no-op-relay tool — closest
  existing precedent for a simple, non-bridged, model-invoked custom tool.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L559-562`
  — `AgentSettledEvent`: "Fired after an agent run has fully settled and no
  automatic retry, compaction, or queued continuation will run." Handler
  signature is `on(event: "agent_settled", handler: ExtensionHandler<AgentSettledEvent>)`
  → `(event, ctx: ExtensionContext) => void` (no result type — cannot
  block/modify, can only act as a side effect, e.g. `pi.sendUserMessage`).
- `types.d.ts#L906-946` — `pi.on("tool_call", handler)` fires **before** a
  tool executes, for every tool call in the session (built-in and custom);
  returning `undefined` is a pass-through (no `block`). This is the seam to
  detect "any tool call happened this cycle" for the runaway counter — it
  fires for ordinary tools (bash/edit/etc.) too, not just goal tools, which
  matches the ticket's "no tool call" wording (any tool call resets the
  streak, not just goal-lever calls).
- `types.d.ts#L980-983` / `examples/extensions/send-user-message.ts` —
  `pi.sendUserMessage(text)` "always triggers a turn," and is a top-level
  `ExtensionAPI` method (not `ctx.*`), exactly how `index.ts#L107`'s
  `/ws-discuss` handler and the intended `agent_settled` re-injection both
  call it — confirms the same primitive works for both the user-invoked `/goal`
  command and the event-driven reminder re-injection.
- **Risk signal — no built-in "is this process a spawned child" signal
  exists today**, yet the ticket settles that the `agent_settled` handler
  must no-op in a child. Traced both spawn call sites:
  - `spawner.ts#L604-617` (`buildRpcClientOptions`, used by `ws-agent-spawn`'s
    `RpcClient` construction at `#L746-747`/`#L805-806`) builds
    `RpcClientOptions` with no `env` field today.
  - `node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.js#L42-44`
    confirms `RpcClient.start()` does `spawn("node", [cliPath, …args], {…,
    env: {...process.env, ...this.options.env}})` — passing `env: {
    WS_PI_AGENT_CHILD: "1" }` in `RpcClientOptions` is merged over the
    inherited `process.env`, so this is a one-line, additive, low-risk fix.
  - `spawner.ts#L399-404` (`spawnPiProcess`, the one-shot `explore` leaf's
    `child_process.spawn` call) passes no `env` at all today, which means
    Node defaults to inheriting `process.env` verbatim — also needs the same
    explicit `env: { ...process.env, WS_PI_AGENT_CHILD: "1" }` override to
    carry the marker.
  - Each spawned child process loads this same extension fresh
    (`index.ts`'s module-level `let` state is per-process, so a child's
    goal-loop state starts inert regardless), but the ticket's guard is
    about defense-in-depth against a message that happens to start with
    `/goal …` reaching a child's input pipeline (e.g. a lead-authored
    `ws-agent-send` message) — `docs/extensions.md` documents extension
    commands as checked during ordinary input processing, not gated by
    `--mode`, so this is a real (if narrow) risk without the guard.
- `agents-plugin-pi/test/model-catalog.test.ts` (whole file) and
  `test/discuss.test.ts` (whole file) — direct templates for
  `test/goal-loop.test.ts`'s two halves: file-read edge cases
  (missing/malformed/empty/populated, via `mkdtempSync`) and pure
  string/state-machine assertions (no IO).

## Implementation Plan

1. **`agents-plugin-pi/src/spawner.ts`** — add
   `export const WS_PI_AGENT_CHILD_ENV = "WS_PI_AGENT_CHILD";` near
   `REPORT_TO_LEAD_TOOL_NAME` (`#L72`). In `buildRpcClientOptions`
   (`#L604-617`), add `env: { [WS_PI_AGENT_CHILD_ENV]: "1" }` to the returned
   object. In `spawnPiProcess` (`#L399-404`), add
   `env: { ...process.env, [WS_PI_AGENT_CHILD_ENV]: "1" }` to the `spawn(...)`
   options. This is the only touch to existing delegation machinery.
2. **`agents-plugin-pi/goal-loop-config.json`** — new file, content `{}`
   (matches `model-catalog.json`'s shipped-empty precedent).
3. **`agents-plugin-pi/package.json`** — add `"goal-loop-config.json"` to the
   `files` array (`#L14-20`) alongside `"model-catalog.json"`.
4. **`agents-plugin-pi/src/goal-loop.ts`** (new file) — pure state machine +
   config reader (unit-testable, no `pi.*` calls) plus the IO registration
   function, mirroring bridge.ts/spawner.ts's mixed-file convention:
   - `export interface GoalLoopConfig { runaway_threshold?: number }` and
     `export const DEFAULT_RUNAWAY_THRESHOLD = 10;`.
   - `export function readGoalLoopConfig(path: string): GoalLoopConfig | undefined`
     — copy `model-catalog.ts#L45-57`'s try/catch-returns-undefined shape
     exactly.
   - `export function resolveRunawayThreshold(config: GoalLoopConfig | undefined): number`
     — `config?.runaway_threshold` when it's a positive finite number, else
     `DEFAULT_RUNAWAY_THRESHOLD` (never hard-fail on a malformed value).
   - `export function buildGoalAnnouncement(goal: string): string` →
     `` `Goal settled: ${goal}` `` (verbatim, per ticket wording).
   - `export function buildGoalReminder(goal: string): string` — a short
     reminder naming the goal and both lever tool names (`goal-achieved`,
     `goal-blocked`) plus the "silence keeps repeating this reminder, N
     re-fires force-stops" caveat; exact wording is a presentation detail,
     not pinned by the ticket beyond "carrying the goal + levers."
   - `export interface GoalLoopState { active: boolean; goal?: string; noToolCallStreak: number; sawToolCallThisCycle: boolean }`
     and `export function initialGoalLoopState(): GoalLoopState`.
   - `export function armGoal(goal: string): GoalLoopState` (always returns a
     fresh active state — arming is idempotent/replaces any prior state).
   - `export function disarmGoal(): GoalLoopState` (returns
     `initialGoalLoopState()`).
   - `export function recordToolCall(state: GoalLoopState): GoalLoopState` —
     no-op (`return state`) when `!state.active`; otherwise sets
     `sawToolCallThisCycle: true`.
   - `export type SettleDecision = {action:"ignore"} | {action:"reinject"; reminder:string} | {action:"force-stop"; reason:string}`
     and `export function decideOnSettle(state: GoalLoopState, threshold: number): {next: GoalLoopState; decision: SettleDecision}`
     — pure reducer: `!active` → `ignore`/unchanged; else compute
     `streak = sawToolCallThisCycle ? 0 : noToolCallStreak + 1`; if
     `streak >= threshold` → `next = initialGoalLoopState()`,
     `decision = {action:"force-stop", reason: "<threshold> consecutive re-fires with no tool call"}`;
     else → `next = {...state, noToolCallStreak: streak, sawToolCallThisCycle: false}`,
     `decision = {action:"reinject", reminder: buildGoalReminder(state.goal!)}`.
   - `export function registerGoalLoop(pi: ExtensionAPI, opts: {goalLoopConfigPath: string}): void`
     — the IO glue:
     - `let state: GoalLoopState = initialGoalLoopState();`
     - `pi.registerCommand("goal", {description: "…", handler: async (args, ctx) => { if (!args.trim()) {ctx.ui.notify("Usage: /goal <goal>", "warning"); return;} if (!ctx.isIdle()) {ctx.ui.notify("Agent is busy — try again when idle.", "warning"); return;} state = armGoal(args.trim()); pi.sendUserMessage(buildGoalAnnouncement(args.trim())); }})`
       — mirrors `index.ts#L100-109`'s `/ws-discuss` idle guard exactly.
     - `pi.on("tool_call", () => { state = recordToolCall(state); });`
     - `pi.on("agent_settled", (_event, ctx) => { if (process.env[WS_PI_AGENT_CHILD_ENV]) return; const threshold = resolveRunawayThreshold(readGoalLoopConfig(opts.goalLoopConfigPath)); const {next, decision} = decideOnSettle(state, threshold); state = next; if (decision.action === "ignore") return; if (decision.action === "force-stop") {ctx.ui.notify(\`Goal loop force-stopped: ${decision.reason}\`, "warning"); return;} pi.sendUserMessage(decision.reminder); });`
       — import `WS_PI_AGENT_CHILD_ENV` from `./spawner.ts`.
     - `pi.registerTool({name: "goal-achieved", label: "goal-achieved", description: "…", parameters: {type:"object", properties:{summary:{type:"string", description:"…"}}, required:["summary"]} as never, async execute(_id, params) { const p = params as {summary:string}; state = disarmGoal(); return {content:[{type:"text", text: \`Goal achieved: ${p.summary}\`}]}; }})`
       — same shape for `goal-blocked` with a `reason` param.
   - Design note to preserve in the file's doc comment: `goal-achieved`/
     `goal-blocked` are **Pi tools** (model-invoked function calls), not Pi
     commands or Pi skills — Pi's "skill" mechanism (`/skill:name`) only
     expands inside text explicitly sent via `sendUserMessage`/`sendMessage`
     with `expandPromptTemplates: true`, or typed user input; it is never
     applied to the model's own generated assistant text, so it cannot serve
     as a model-invoked, zero-prose-parsing lever. `registerTool` is the only
     mechanism matching "explicit skill calls, zero prose parsing." `/goal`
     itself stays a `registerCommand` (user-invoked entry, matching the
     ticket's own "goal-entry **command**" wording and the existing
     `/ws-discuss` precedent).
5. **`agents-plugin-pi/src/index.ts`** — import `registerGoalLoop` from
   `./goal-loop.ts`; compute
   `const goalLoopConfigPath = join(pluginDir, "goal-loop-config.json");`
   next to the existing `modelCatalogPath` (`#L69`); call
   `registerGoalLoop(pi, { goalLoopConfigPath });` at factory top level,
   alongside the existing `ws-model-catalog-list`/`ws-discuss` registrations
   (`#L83-109`) — not inside `session_start`. Extend the file's top-of-file
   doc comment (`#L1-53`) with a short paragraph naming this ticket
   (`260903` Phase 1 goal-loop) — do not renumber it into the existing
   "Phase 2/3/4" MVP-ticket phase list, since those numbers belong to the
   prior `260902` MVP ticket.
6. **`agents-plugin-pi/test/goal-loop.test.ts`** (new file) — unit coverage
   for every pure export in step 4: `readGoalLoopConfig`
   (missing/malformed/empty/populated, per `model-catalog.test.ts`'s
   pattern), `resolveRunawayThreshold` (unset → default 10; valid override;
   non-numeric/negative → default), `buildGoalAnnouncement`/
   `buildGoalReminder` (content assertions, per `discuss.test.ts`'s style),
   `armGoal`/`disarmGoal`/`recordToolCall` (state transitions), and
   `decideOnSettle` (the runaway-threshold table: ignore when inactive,
   reinject + streak increments while under threshold, streak resets to 0 on
   a tool call, force-stop + full reset at exactly `threshold`). This
   satisfies the ticket's "Loop-guard / threshold logic unit-tested where
   seam-extractable."

## Verification Plan

- `cd agents-plugin-pi && node --test test/` — full unit suite green,
  including the new `goal-loop.test.ts`.
- Live gate (per the ticket's own five-point verification boundary, same
  style as the `/ws-discuss` and subagent-RPC-UX live gates — not
  reproducible as an automated test without a live model/API key): a
  `pi … --mode json` transcript showing (a) `/goal <goal>` arms the loop and
  emits the "Goal settled" announcement, (b) an ordinary settle re-injects
  the reminder, (c) both `goal-achieved` and `goal-blocked` tool calls
  terminate the loop (no further re-fire), (d) a settle with no `/goal` ever
  issued does not re-fire, (e) N consecutive tool-call-free re-fires
  force-stop at the configured threshold (test with a small
  `goal-loop-config.json` override, e.g. `{"runaway_threshold": 2}`, to keep
  the transcript short).
- Manual spot-check of the spawned-child guard: confirm a `ws-agent-spawn`ed
  worker's environment carries `WS_PI_AGENT_CHILD=1` (e.g. a temporary debug
  log line, removed before commit, or inspection via
  `ws-agent-transcript`/stderr) — this is a defense-in-depth path with no
  natural trigger in the Phase 1 verification transcript itself, so calling
  it out explicitly avoids it going unverified.

## Escalations

- None. Confidence is high: every mechanism the design section names
  (`agent_settled`, `pi.registerTool`, `pi.registerCommand`,
  `pi.sendUserMessage`, `tool_call`) is a real, precedented API already used
  elsewhere in this same package (`index.ts`, `spawner.ts`), confirmed
  against the installed `@earendil-works/pi-coding-agent` type defs and
  compiled source rather than assumed. The one non-obvious design call —
  implementing `/goal-achieved`/`/goal-blocked` as `registerTool` calls
  (model-invoked function calls) rather than `registerCommand`s — is
  resolved with high confidence by elimination: Pi's only model-invocable,
  zero-prose-parsing primitive is a tool call (confirmed by reading how
  Pi's `/skill:name` expansion and command dispatch are wired only into the
  *input* pipeline, never into assistant-generated output), and the ticket's
  own "explicit skill calls" framing plus the existing `ws-report-to-lead`
  precedent (a plain model-invoked tool, not a command) both point the same
  way. This is a resolvable implementation-detail judgment call, not a
  scope reduction or a strategic fork — flagged in the plan for the
  executor's awareness, not escalated.
