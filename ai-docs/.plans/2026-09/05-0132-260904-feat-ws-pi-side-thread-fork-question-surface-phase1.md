# Plan: 260904-feat-ws-pi-side-thread-fork-question-surface — Phase 1: Task-thread fork with relay baseline (`ws.fork` + anti-bleed loop)

## Relevant Ticket Contract

- Implement `ws.fork(prompt, model_name?, expects_commit?)` as a
  `ws-agent-spawn`-family variant with a `fork_from` seam
  (`pi --fork <lead session> --mode rpc`); the fork gets its own lead-scope
  key via the bridge's `WS_PI_SPAWN_ROLE=fork` + `WS_PI_PARENT_SESSION_KEY`
  marker plumbing (Decision §2) — the key-rewrite mechanism itself already
  landed, this ticket only sets the marker at spawn.
- Fork tools = lead's exact surface (`pi.getActiveTools()` on the lead at
  spawn time) − `[ws.fork, ws.ask, ws.resolve]` + `[ws-report-to-lead]`
  (Decision §3). Phase 1 implements only `ws.fork` — `ws.ask`/`ws.resolve`
  don't exist yet, so the only name to actually exclude in Phase 1 is
  `ws-fork` itself, in a form that stays forward-compatible with adding
  `ws-ask`/`ws-resolve` to the excluded set later.
- Depth: the fork is lateral, not a worker — no depth-budget consumption,
  termination unchanged (Decision §3).
- Approval routing = spawning parent: a fork's own `ws.execute` worker's
  approval request routes to the fork, not the top lead (Decision §3, amends
  `260904-feat-ws-pi-execute-approval-gateway` §4 — that ticket's §4 already
  states this rule prospectively, verbatim).
- Anti-bleed mechanical loop, task threads (Entry A) only (Decision §4):
  turn-end-with-no-tool-call → auto-nudge ≤2 times → fail loud with
  transcript tail; idle-without-`kind:"final"` → reported as incomplete
  (never harvested as a result); disambiguation is `kind:"question"` then end
  = question, `kind:"final"` = completion, neither = failure; required
  `kind:"final"` report shape `Outcome / Files changed / Verification /
  Blockers / Commit / Decisions` (`Commit:` always present, literal `none`
  when nothing committed); `expects_commit:true` + `Commit: none` is flagged
  non-completion; directive style is short natural language, no identity
  framing, no XML/all-caps overrides.
- Constraints: golden rule (`agents-plugin-tool/` Go and
  `agents-plugin/skills/` canonical text untouched — everything lives in
  `agents-plugin-pi/`); system prompt append-only; no prose-only bleed
  mitigation (report as residual if a Phase 1 measurement suggests one).
- Phase 1 verification (ticket's own list): (1) `--fork` CLI flag
  composition/position semantics — explicitly named as requiring a live
  measurement; (2) Bleed PoC on a real lead session — explicitly the ticket's
  go/no-go for Phase 2, explicitly live/model-spend-dependent; (3) depth —
  fork's allowlist lacks `ws-fork`, recursion fails at the tool layer; (4)
  completion checks (`expects_commit` + `Commit: none`, idle-without-final)
  enforced; (5) session-key isolation — parent-key rewrite + `ferrule(...,
  capability: lead, parent_session_key: ...)` mint, lead's todo/agenda
  untouched, `session.children` lists the fork.
- Task instruction (reality gate): items 1 and 2 above cannot run in this
  sandbox (no live `pi`/provider credentials) — maximize what unit tests can
  cover offline and mark the rest as a deferred manual gate, mirroring
  `260904-feat-ws-pi-execute-approval-gateway`'s own Phase 1 "Outstanding"
  precedent (`ai-docs/tickets/ready/260904-feat-ws-pi-execute-approval-gateway.md`
  Result section).

## Out of Scope

- Phase 2 in full: thread registry persistence, `ws.ask`/`ws.resolve`,
  `aboveEditor` widget, `/answer`/`/thread`, overlay chat component, lazy
  discussion fork, `entry_id` anchoring, injection-into-lead-session (custom
  message on idle), headless `ws.ask` baseline. Only touched here to the
  extent Phase 1 must stay forward-compatible (documented per-step below).
- §10 cut line / `lead-write-ticket` playbook content — playbook prose is
  canonical `agents-plugin/skills/` text and is not edited by this ticket at
  all (golden rule); Phase 1 only builds the mechanism the playbook will
  later be told to use.
- Editing `ai-docs/tickets/ready/260904-feat-ws-pi-execute-approval-gateway.md`
  — its §4 "Approver is the spawning parent ... for a worker spawned by a
  side-thread fork ... the approver is that fork" is already present
  verbatim; no edit needed for Phase 1.
- Any `agents-plugin-tool/` (ws-mcp Go) change, including for
  `ferrule(capability: lead, parent_session_key: ...)` — already supported
  server-side (confirmed: `parent_session_key` appears in
  `agents-plugin-tool/internal/mcp/server.go` and
  `agents-plugin-tool/internal/mcp/session_auth_test.go`).

## Codebase Findings

- `agents-plugin-pi/src/process-role.ts#L27-L60` — `SpawnRole` already
  includes `"fork"` and `WS_PI_PARENT_SESSION_KEY_ENV` is already reserved;
  `isLeadOrFork(role)` already treats `"fork"` the same as the host lead.
  Nothing here needs to change — Phase 1 only needs to *set* these at spawn.
- `agents-plugin-pi/src/bridge.ts#L308-345` (`normalizeSessionKey`) — already
  rewrites an explicit `session_key` equal to `WS_PI_PARENT_SESSION_KEY_ENV`
  (when set) to the bridge's own key; the inline comment at
  `bridge.ts#L432-437` states this env is unset "until a future
  fork-spawning ticket sets it." Already unit-tested for exactly this case
  (`agents-plugin-pi/test/bridge.test.ts#L184-198`,
  `normalizeSessionKey`/`parentLeadKey` describe block). Phase 1's only job
  on this axis is setting `WS_PI_SPAWN_ROLE=fork` +
  `WS_PI_PARENT_SESSION_KEY=<lead's own key>` as spawn env vars — the
  rewrite logic itself needs zero changes.
- `agents-plugin-pi/src/index.ts#L160-212` (`session_start`) — a spawned
  child re-runs this SAME handler (loads the same extension fresh). Setting
  `WS_PI_SPAWN_ROLE=fork` on the child process therefore automatically
  grants, with no new code: (a) the ws system-prompt block / pi-lead-guide
  append (`isLeadOrFork` gate, `L190`), (b) `computeLeadActiveTools`
  reshaping — bash/read removed, `ws-execute`/`ws-approve`/ugly-read added
  (`L209-211`), (c) its OWN independent `registerAgentTools`/
  `registerExecuteGateway` call with its own `rpcRegistry`/
  `onApprovalPending`. Consequence: a fork's own `ws-execute`-spawned worker's
  approval request is injected via *that fork's own* `pi.sendUserMessage`
  (its own `createApprovalRelay` closure) — landing in the fork's session,
  not the top lead's. **§3's "approval routing = spawning parent" falls out
  structurally from the existing per-process architecture; Phase 1 needs no
  new relay code for it**, only correct marker plumbing at fork-spawn time.
  Live confirmation still belongs to the deferred gate (real approval flow
  inside a real fork).
- **Risk signal (uniform reshaping would leak `ws-fork` into the fork's own
  surface).** `agents-plugin-pi/src/execute-gateway.ts#L184-215`
  (`computeLeadActiveTools`/`LEAD_ADDED_TOOL_NAMES`) is applied identically
  to lead and fork roles at `index.ts#L209-211` (`isLeadOrFork` is a single
  boolean, not role-differentiated). If a new `FORK_TOOL_NAME` were folded
  into that same shared `LEAD_ADDED_TOOL_NAMES` list, `computeLeadActiveTools`
  would re-add `ws-fork` to a *fork's own* active tools too — violating §3's
  "fork surface excludes `ws-fork`." Phase 1 must add `ws-fork` to the active
  list through a **separate, role-differentiated** step (`role === undefined`
  only), not by touching `execute-gateway.ts`'s shared function (keeps that
  module's existing responsibility unchanged, Code Standards #3).
- `agents-plugin-pi/src/spawner.ts#L689-757` (`RpcSpawnCtx`,
  `buildRpcClientOptions`) — the sole seam turning session-file + tools +
  prompt into `pi` CLI args; today always
  `["--session", sessionPath, "--append-system-prompt", ..., "--tools", ...]`,
  and `sessionPath` is always pre-computed by the caller via `mkdtempSync`
  *before* `client.start()` (`spawnAgent#L905-941`). This breaks for a fork
  spawn: `pi --fork <leadPath>` (confirmed CLI flag:
  `node_modules/@earendil-works/pi-coding-agent/docs/usage.md#L200-201`,
  `docs/sessions.md#L15`) has Pi itself create/name the new session file —
  its exact composition with `--mode rpc`/`--tools`/`--append-system-prompt`,
  and whether it clones at-leaf vs. before a message, is the ticket's own
  named live-verification item 1 (not resolvable offline). `RpcClient`
  exposes `getState(): Promise<RpcSessionState>` whose `sessionFile?: string`
  (`node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.d.ts#L95-97`,
  `rpc-types.d.ts#L148-161`) is the only discovered way to learn the actual
  forked session path post-`start()`.
- `agents-plugin-pi/src/spawner.ts#L160-180` (`TOOL_GROUPS`/`resolveTools`)
  — every existing tool surface is a *static* array keyed by a fixed
  `ToolGroup` literal. §3's fork surface is *dynamic*
  (`pi.getActiveTools()` on the lead at spawn time, minus/plus a small set)
  and cannot be a new static `TOOL_GROUPS` entry. `RpcAgentRecord.toolGroup`
  + `wsToolNames` (`#L636-683`) are what a dormant resume
  (`sendToAgent#L988-991`) uses to rebuild `--tools` via
  `resolveTools(record.toolGroup, record.wsToolNames)` — a fork record needs
  its exact computed tools string cached verbatim and reused unchanged on
  resume (mirrors how `systemPromptPath`/`modelBase` are already
  cached-and-reused, `#L642-650`), not re-derived from a group.
- `agents-plugin-pi/src/spawner.ts#L1448-1469` (`ws-report-to-lead`
  registration) and `#L823-847` (`applyRpcEvent`'s report branch) —
  currently `{message: string}` only, enqueued as a bare string onto
  `RpcAgentRecord.pendingReports: string[]`
  (`WaitForAgentsResult.reports: string[]`, `#L1046`). §4 needs an optional
  `kind: "question" | "final"` field. Extending this is backward compatible
  (existing `full-worker`/`execute-worker` callers omit `kind`, unaffected)
  but changes `pendingReports`'/`reports`' element type from `string` to a
  small `{message, kind?}` shape — `test/spawner.test.ts`'s existing
  `enqueueReport`/`drainReports`/`waitForAgents` assertions need matching
  (additive, not rewritten) updates.
- `agents-plugin-pi/pi-lead-guide.md#L27-50` (verb-routing table) — its
  trailer line ("This table grows as later tickets land more primitives (a
  fork/ask/resolve side-thread surface)") is the literal hook Phase 1 fills
  with one `ws-fork` row. The Decision §2 prose ("this ticket adds its
  ws.fork/ws.ask/ws.resolve rows") describes the ticket's *whole* arc across
  both phases — Phase 1 adds only the `ws-fork` row; `ws.ask`/`ws.resolve`
  don't exist yet.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L372`
  (tool `execute(toolCallId, params, signal, onUpdate, ctx: ExtensionContext)`)
  and `#L219` (`ExtensionContext.sessionManager: ReadonlySessionManager`) +
  `dist/core/session-manager.d.ts#L140` (`getSessionFile()`) — confirms the
  lead's own session path (needed for `--fork <path>`) is reachable from
  inside `ws-fork`'s `execute()` via `toolCtx.sessionManager.getSessionFile()`,
  the same `toolCtx` shape `inheritModelFromToolCtx` already reads
  (`spawner.ts#L346-349`). `types.d.ts#L995,999` confirm `pi.getActiveTools()`
  / `pi.setActiveTools()` exist on the top-level `ExtensionAPI`, exactly as
  Decision §3 assumes.
- `agents-plugin-pi/src/bridge.ts#L60` (`BridgeHandle.defaultSessionKeyRef`)
  — the lead's own default-filled session key, already threaded into
  `registerAgentTools`/`registerExecuteGateway` via the shared `bridge`
  handle; the new fork registration function needs the same handle for the
  same reason (source of the value written to
  `WS_PI_PARENT_SESSION_KEY_ENV`).
- `ai-docs/tickets/ready/260904-feat-ws-pi-execute-approval-gateway.md#L323-372`
  (Phase 1 "Result" section) — direct precedent for how to report a
  sandbox-blocked live gate: name exactly which verification items are
  deferred, why (no provider credentials), and record them as "Outstanding"
  rather than attempting to fake coverage. This plan's Verification Plan
  mirrors that structure.

## Implementation Plan

1. **`agents-plugin-pi/src/spawner.ts`** — extend the RPC spawn seam for
   forking, reusing `spawnAgent`/`sendToAgent` rather than duplicating them:
   - Add `forkFrom?: string` to `RpcSpawnCtx` (the lead's session file to
     fork from) and an `explicitTools?: string` override on the same ctx
     (bypasses `resolveTools(toolGroup, wsToolNames)` when set). Add a
     matching `explicitTools?: string` field to `RpcAgentRecord`, cached at
     spawn and reused verbatim on every resume (mirror
     `systemPromptPath`/`modelBase`'s existing cache-and-reuse contract).
   - `buildRpcClientOptions`: add an optional `forkFrom?: string` param;
     when set, emit `["--fork", forkFrom, "--append-system-prompt", ...,
     "--tools", ...]` instead of `["--session", sessionPath, ...]` — for the
     **initial** spawn only. `sendToAgent`'s dormant-resume branch must
     never pass `forkFrom` again (a resume uses the fork's own
     already-discovered `sessionPath` via `--session`, exactly like a normal
     worker resume) — thread `forkFrom` only through `spawnAgent`'s
     first-call path, never through `RpcResumeCtx`.
   - `spawnAgent`: after `client.start()`, when `forkFrom` was supplied, call
     `client.getState()` and overwrite `record.sessionPath` with the
     returned `sessionFile` (throw with a clear message if absent — mirrors
     the codebase's existing never-silently-degrade convention, e.g.
     `bridge.ts`'s ferrule-mint failure handling).
   - Extend `ws-report-to-lead`'s registered `parameters` schema with an
     optional `kind: {type:"string", enum:["question","final"]}`; extend
     `applyRpcEvent`'s report-matching branch and `enqueueReport` to carry
     `kind` through; change `RpcAgentRecord.pendingReports` and
     `WaitForAgentsResult.reports` from `string[]` to
     `Array<{message: string; kind?: "question" | "final"}>` (additive —
     existing worker/execute-worker callers keep omitting `kind`).
   - Update `test/spawner.test.ts`'s existing `enqueueReport`/
     `drainReports`/`waitForAgents` assertions for the new element shape,
     and add coverage for `buildRpcClientOptions`'s new `forkFrom` arg
     branch (pure arg-array assertion, no real spawn).

2. **New file `agents-plugin-pi/src/fork.ts`** (mirrors
   `execute-gateway.ts`'s placement/shape: imports FROM `spawner.ts` and
   `process-role.ts` only, never the reverse):
   - `FORK_TOOL_NAME = "ws-fork"`.
   - `FORK_EXCLUDED_TOOL_NAMES: ReadonlySet<string>` = `{"ws-fork"}` for
     Phase 1 (doc comment: Phase 2 adds `ws-ask`/`ws-resolve` here).
   - `computeForkToolSurface(leadActiveTools: readonly string[]): string[]`
     — pure; mirrors `computeLeadActiveTools`'s remove/add/dedupe shape:
     filter out `FORK_EXCLUDED_TOOL_NAMES`, then append
     `REPORT_TO_LEAD_TOOL_NAME` if absent.
   - `addForkToolIfLead(activeTools: readonly string[], role: SpawnRole | undefined): string[]`
     — pure; appends `FORK_TOOL_NAME` only when `role === undefined` (the
     true top lead), never for `"fork"`/`"worker"`/`"explore"` — the fix for
     the uniform-reshaping risk above.
   - Anti-bleed pure predicates (§4), each independently unit-testable
     against plain data, no `RpcClient`:
     - `MAX_FORK_NUDGES = 2` and `shouldNudge(nudgeCount: number): boolean`.
     - `classifyForkTurnOutcome(input: {hadToolCall: boolean; reportKind?: "question" | "final"}): "question" | "final" | "acknowledge-and-return" | "no-signal"`
       — the §4 disambiguation table.
     - `isIdleWithoutFinal(reportKinds: readonly (string | undefined)[]): boolean`
       — true when no `"final"` kind appears among the drained reports at
       idle.
   - `REQUIRED_FINAL_REPORT_FIELDS = ["Outcome", "Files changed", "Verification", "Blockers", "Commit", "Decisions"] as const`
     and `validateFinalReportShape(message: string): {ok:true} | {ok:false; missing: string[]}`
     — pure per-field-prefix presence check.
   - `checkExpectsCommitCompletion(expectsCommit: boolean, commitLine: string | undefined): {ok:true} | {ok:false; reason:string}`
     — `expectsCommit && (commitLine === undefined || /^\s*none\s*$/i.test(commitLine))`
     → non-completion.
   - `registerFork(pi, bridge: BridgeHandle, rpcRegistry: RpcAgentRegistry, sessionCtx: {cwd, modelCatalogPath})`
     — IO glue registering the `ws-fork` tool:
     - `{prompt, model_name?, expects_commit?}` params.
     - `forkFrom = toolCtx.sessionManager.getSessionFile()`.
     - `tools = computeForkToolSurface(pi.getActiveTools())`.
     - env for the child: `WS_PI_SPAWN_ROLE_ENV: "fork"`,
       `WS_PI_PARENT_SESSION_KEY_ENV: bridge.defaultSessionKeyRef.current`.
     - calls `spawnAgent` with `forkFrom`/`explicitTools` set (directive
       prompt built as short natural language, execution constraints only —
       no identity framing, no XML/all-caps, per §4's directive-style rule).
     - wires the anti-bleed loop onto the same `RpcClient.onEvent()` stream
       `ws-agent-*` already observes (a new callback hook parallel to
       `onApprovalPending`, e.g. `onForkTurnSettled`, invoked from
       `attachEventListener`/the harvesting path in `waitForAgents`) driving
       `shouldNudge`/`classifyForkTurnOutcome`/`isIdleWithoutFinal`/
       `validateFinalReportShape`/`checkExpectsCommitCompletion`; on
       fail-loud or incomplete, surfaces the failure plus the transcript
       tail (reuse `getAgentTranscriptPath`) to the lead instead of
       harvesting a false "success" — never routed through the fork's own
       `kind:"final"` report path.
   - New `agents-plugin-pi/test/fork.test.ts` covering every pure helper
     above with zero subprocess/`RpcClient` involved (mirrors
     `execute-gateway.test.ts`'s pure/IO split).

3. **`agents-plugin-pi/src/index.ts`** — in `session_start`, after
   `registerExecuteGateway` and the existing
   `pi.setActiveTools(computeLeadActiveTools(...))` call: call
   `registerFork(pi, handle, agentTools.rpcRegistry, { cwd: ctx.cwd, modelCatalogPath })`
   (same shared registry, per `AgentToolsHandle`'s own "one shared map" doc
   comment), then apply
   `pi.setActiveTools(addForkToolIfLead(pi.getActiveTools(), readSpawnRole(process.env)))`
   as a role-differentiated step separate from `computeLeadActiveTools` (see
   the risk-signal finding above — do not fold `FORK_TOOL_NAME` into
   `execute-gateway.ts`'s shared `LEAD_ADDED_TOOL_NAMES`).

4. **`agents-plugin-pi/pi-lead-guide.md`** — add one verb-routing-table row
   for `ws-fork` (task-thread delegation to a context-inheriting peer,
   report path is `ws-report-to-lead(kind:"question"|"final")` only); narrow
   the trailer sentence to note only `ws.ask`/`ws.resolve` as still pending
   (Phase 2), since the fork primitive now exists.

5. **`ai-docs/spec/pi-adapter-runtime.md`** — add one new anchor (stem
   convention, e.g. `{#260905-pi-side-thread-fork-task-thread}`) documenting:
   the `ws-fork` tool contract, the tool-surface formula, the anti-bleed
   loop and required report shape, the `expects_commit` check, and that
   approval-routing-to-spawning-parent is an emergent property of the
   existing per-process registration pattern (worth stating explicitly since
   it is non-obvious). Keep this light — exact prose is a commit-time doc
   chore per the repo's Commit Rules `## Spec` section, not a full spec
   draft in this plan.

## Verification Plan

- `cd agents-plugin-pi && npm test` (`node --test`) — new/extended offline
  coverage:
  - `buildRpcClientOptions`'s `--fork` vs. `--session` arg branching (pure
    arg-array assertion).
  - `computeForkToolSurface` / `addForkToolIfLead` tool-surface arithmetic,
    including the role-differentiation fix (fork never regains `ws-fork`).
  - Anti-bleed predicates: `shouldNudge`'s ≤2-then-fail-loud transition,
    `classifyForkTurnOutcome`'s question/final/acknowledge-and-return/
    no-signal disambiguation, `isIdleWithoutFinal`'s idle-without-`"final"`
    classification.
  - `validateFinalReportShape`'s required-field check (`Outcome / Files
    changed / Verification / Blockers / Commit / Decisions`).
  - `checkExpectsCommitCompletion`'s `Commit: none` + `expects_commit:true`
    → incomplete rule.
  - `applyRpcEvent`/`enqueueReport`/`drainReports`/`waitForAgents` extended
    for the `{message, kind?}` shape — regression-safe over existing
    worker/execute-worker report flow (existing tests updated, not
    rewritten).
  - `normalizeSessionKey`'s parent-key rewrite is already covered
    (`test/bridge.test.ts`) — no new test needed there, only the spawn-side
    marker-setting is new and belongs in `fork.test.ts`/`spawner.test.ts`.
- **Deferred live gate** (no live `pi`/provider credentials in this
  sandbox — mirrors `260904-feat-ws-pi-execute-approval-gateway`'s Phase 1
  "Outstanding" precedent; report which mode was achieved, same as that
  ticket did):
  - Ticket verification item 1: `pi --fork <file> --mode rpc --tools ...
    --append-system-prompt ...` flag composition, at-leaf clone confirmation,
    and `--fork`'s position semantics.
  - Ticket verification item 2: the Bleed PoC (≥3 real forks running
    `lead-write-ticket` Populate→Commit on a scratch ticket; the
    acknowledge-and-return rate with the loop on/off; forcing the fail-loud
    path at least once; confirming no narration reaches the lead as a
    result) — the ticket's own explicit go/no-go gate for Phase 2, and
    inherently model-spend/live-session dependent.
  - Ticket verification item 3 (depth): the *logic* is offline-covered
    (`computeForkToolSurface` excludes `ws-fork`); the live confirmation
    that recursion actually fails at Pi's tool layer (not by refusal prose)
    is part of the same deferred gate.
  - Ticket verification item 4 (completion checks): the *logic* is
    offline-covered (`validateFinalReportShape`,
    `checkExpectsCommitCompletion`); live end-to-end confirmation is
    deferred.
  - Ticket verification item 5 (session-key isolation): the rewrite logic is
    already unit-tested (`bridge.test.ts`); the live
    `parent_session_key`-mint + `session.children` listing + "lead's
    todo/agenda untouched after a fork ran `lead-write-ticket`" checks are
    deferred.

## Escalations

- None.
