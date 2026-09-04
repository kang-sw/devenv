# Plan: 260904-feat-ws-pi-execute-approval-gateway — Phase 1: End-to-end approval gateway via the fallback relay

## Relevant Ticket Contract

- Deliver the WHOLE gateway through the "no special harness support" fallback
  relay (§8 baseline): gated-exec worker tool (every free-form command
  elevates) + mutation-incapable read-family on the worker `--tools`;
  `ws.execute(command?, prompt, complex?)` spawning a worker whose exec
  elevates, `complex?` -> model tier; the prompt-injection approval relay +
  `ws.approve(agent_id, cmd_id, decision, reason?, command?)` with
  `approve`/`deny(reason)`/`run-instead(command)`; the §7
  adapter-authoritative payload (`{agent_id, cmd_id, command, rationale,
  context:{cwd, worktree_root, branch, ahead_behind?, dirty}}`); `cmd_id`
  race-binding; abort via `ws-agent-stop` (unblock with "aborted",
  dormant+retain); lead `--tools` change (bash removed, `ws.execute`/
  `ws.approve` added, ugly-named read retained). `command?` runs verbatim
  first, then hands `{command, output}` to the worker.
- §2 invariant (record verbatim in spec, apply in guide prose): the gate
  exists because `ws.execute` proxies lead-consensus-caliber actions; general
  `ws-agent-spawn` workers carry no such gate.
- §6: the "when to use" rows for `ws.execute`/`ws.approve` go into
  `pi-lead-guide.md`'s existing verb table (already landed by
  260904-feat-ws-pi-lead-bootstrap-system-prompt) — this ticket does not
  author standalone lead guidance elsewhere.
- §8 linchpin: verify whether `pi.setActiveTools()` can hard-remove native
  `bash` from (and let a custom tool substitute for native `read` on) the
  HOST lead session, and report which mode (hard removal vs. soft
  system-prompt convention) Phase 1 achieved. Not an escalation trigger per
  task authority — resolved below (feasible).
- Verification boundary (Phase 1, ticket text): a live `pi … --mode rpc` run
  covering free reads (no gate), a mutating command elevating to
  `ws.approve`, `deny`/`run-instead` behavior, the context header + rationale,
  `cmd_id` stale-rejection, abort-unblocks-execute, and which lead
  tool-surface mode was achieved. "Race/registry/select logic unit-tested
  where seam-extractable."
- Golden rule: `agents-plugin-tool/` (ws-mcp Go) untouched;
  `agents-plugin/skills/` canonical text untouched; dependency
  adapter -> ws-mcp only. Everything lands in `agents-plugin-pi/`.
- Task-authorized deferral: the live `pi --mode rpc` end-to-end run has no
  provider credentials in this sandbox — plan it as a documented manual
  verification gate, not faked.

## Out of Scope

- Phase 2 (harness-native pause/resume, mid-task `complex` escalation,
  on-demand context-expand) — explicitly a later phase.
- `260524` `exec.*`/`exec.ask` reconciliation (§9) — deferred by the ticket
  itself to that ticket's landing.
- `260904-feat-ws-pi-side-thread-fork-question-surface` (fork/ask/resolve) —
  separate ticket, only its reserved `"fork"` role/env var are touched (none
  of this phase's code needs to change them).
- Concurrent multi-`execute` orchestration — structurally allowed, not built.
- Command-type-adaptive context enrichment (git ahead/behind is included
  since the ticket names it explicitly; richer per-command-type enrichment is
  explicitly deferred by §7).

## Codebase Findings

### Spawn/registry machinery to extend (not replace)

- `agents-plugin-pi/src/spawner.ts#L70` — `export type ToolGroup =
  "read-only" | "recon" | "full-worker";` needs a 4th member,
  `"execute-worker"`.
- `agents-plugin-pi/src/spawner.ts#L125-L129` — `TOOL_GROUPS` already defines
  `"read-only": ["read", "grep", "find", "ls"]` — this IS the ticket's §5
  "mutation-incapable read family" (Pi's actual built-in name for the
  Claude-style "glob" tool is `"find"`, not `"glob"`; the ticket's prose uses
  the Claude naming loosely). Reuse this array verbatim for the new
  `"execute-worker"` group; do not re-derive it.
- `agents-plugin-pi/src/spawner.ts#L756-L790` (`spawnAgent`) and `#L825-L866`
  (`sendToAgent`) — **risk signal**: both HARDCODE
  `resolveTools("full-worker", ctx.wsToolNames)` /
  `resolveTools("full-worker", record.wsToolNames)` (lines ~780 and ~839).
  `ws.execute`'s worker needs the new `"execute-worker"` group instead, so
  `spawnAgent`/`sendToAgent` cannot be reused unmodified — `RpcAgentRecord`
  (`#L571-L598`) needs a new `toolGroup: ToolGroup` field (set at spawn,
  read back unchanged on dormant resume), and `RpcSpawnCtx`/`SpawnAgentParams`
  (`#L604-L619`) need a `toolGroup` field threaded through both
  `resolveTools(...)` call sites in place of the literal `"full-worker"`.
  Existing `ws-agent-spawn` callers keep passing `"full-worker"` explicitly —
  no behavior change for them.
- `agents-plugin-pi/src/spawner.ts#L702-L715` (`applyRpcEvent`) — the
  established one-way report-relay pattern (`tool_execution_start` matched by
  `toolName`, args destructured). The approval-request relay reuses this
  exact observation point for a 2nd tool name (the new gated-exec tool),
  writing into a new `record.pendingApproval` field instead of
  `pendingReports`. `ToolExecutionStartEvent` (confirmed via
  `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L608-L613`)
  carries `{type, toolCallId, toolName, args}`, and
  `dist/modes/json-event.d.ts` confirms `toJsonEvent` passes
  `tool_execution_start` through unchanged (only `message_update` is
  transformed) — so `toolCallId` survives the RPC wire and is available on
  the observed event. **This `toolCallId` IS the natural `cmd_id`** — no new
  id needs minting/synchronizing between parent and child.
- `agents-plugin-pi/src/spawner.ts#L1081-L1091` (`AgentToolsHandle`) and
  `#L1122-L1354` (`registerAgentTools`) — `rpcRegistry` is a private closure
  today. `AgentToolsHandle` needs a new `rpcRegistry: RpcAgentRegistry` field
  so a sibling module can read/write `pendingApproval` and call
  `spawnAgent`/`sendToAgent`/`stopAgent` on the same map `ws-agent-*` already
  uses (§4's "agent_id disambiguates among all live and dormant/retained
  agents" requires ONE shared registry, not a second one).
- `agents-plugin-pi/src/spawner.ts#L307-L326` (`AgentCallCtx`) unaffected;
  `ws.execute` does not touch the one-shot `explore` registry.

### Host-lead tool-surface reshaping (§8 linchpin) — VERIFIED feasible

- `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L992-L999`
  — `pi.getActiveTools()` / `pi.setActiveTools(toolNames)` are methods on
  `ExtensionAPI` itself (the `pi` passed into the extension factory), not
  `ExtensionContext` — callable directly from `index.ts`'s `session_start`
  handler exactly where `startBridge`/`registerAgentTools` already run.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L659-L672`
  (`setActiveToolsByName`) — operates on `this._toolRegistry`, which holds
  BUILT-IN tools (including `"bash"`) and extension/custom tools in the SAME
  map, keyed by name. **Confirms hard removal covers built-in `bash`** — it
  is just excluded from the filtered active list, no special-casing needed.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/agent-session.js#L2098-L2173`
  (`_refreshToolRegistry`) and `#L2195-L2231` (`_buildRuntime`/`reload()`) —
  **confirms the removal survives `/reload`**: `reload()` calls
  `_buildRuntime({ activeToolNames: this.getActiveToolNames(), ...,
  includeAllExtensionTools: true })`, i.e. it explicitly re-passes the
  CURRENT (already-reduced) active list forward and only auto-adds
  extension-registered tools on top (never re-adds an excluded builtin like
  `bash`, since that branch is skipped whenever `activeToolNames` is
  supplied).
- **Risk signal (new, must be handled, not just noted)**: on a **fresh**
  (non-reload) session, `_refreshToolRegistry`'s "auto-include every
  newly-registered tool name" branch runs when `activeToolNames` is
  `undefined`/omitted, which is why `ws-agent-*`/`explore`/`goal-*` etc. are
  already implicitly active for the lead today with no explicit allowlist
  call. If Phase 1 registers the new gated-exec tool globally (needed so a
  worker's own `pi -e` process, loading the same extension, can activate it
  via `--tools`), that SAME auto-include branch would also silently make
  gated-exec active on the **lead's own session** — a footgun: the lead
  could call the worker-only gated-exec tool directly, bypassing
  `ws.execute`/`ws.approve` entirely, and since nothing on the lead's own
  session observes its own `tool_execution_start` the same way a parent
  observes a spawned child, a lead-invoked gated-exec call would hang
  forever waiting on a decision file nobody will ever write. **Fix**: the
  explicit `pi.setActiveTools(...)` call this ticket adds must exclude the
  gated-exec tool name from the lead's list, not just add `ws-execute`/
  `ws-approve`/the ugly-read tool and remove `bash`/`read`.
- `node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/types.d.ts#L992-L993`
  (`exec(command, args, options): Promise<ExecResult>`) and
  `dist/core/exec.d.ts` / `dist/core/exec.js` — `ExtensionContext.exec` is a
  `spawn(command, args, {shell:false})` wrapper (NOT a shell). The gated-exec
  tool must invoke `ctx.exec("sh", ["-c", command], {cwd, signal, timeout})`
  to get shell semantics (`&&`, redirection, etc.) — matches the LLM-supplied
  `command` being a shell string per §5's own compound-command examples.
  `ExecOptions.signal`/`.cwd` are directly usable for abort-wiring and the
  optional per-call `cwd` override (see below).

### Approval relay transport (the "new primitive," §8) — concrete, buildable design

Pi's RPC surface has NO channel that can resolve an **in-flight** tool call:
`rpc.md` documents `steer` as "delivered after the current assistant turn
finishes executing its tool calls" and `follow_up` as delivered only once
the agent is fully idle — both are turn-boundary-only (confirmed against
`node_modules/@earendil-works/pi-coding-agent/docs/rpc.md`'s `#### steer`/
`#### follow_up` sections and `RpcClient`'s own doc comments in
`dist/modes/rpc/rpc-client.d.ts#L66-L74`). So the gated-exec tool's blocked
`execute()` promise cannot be resolved by any `prompt`/`steer`/`followUp`
call from the parent without deadlocking on the very tool call it's trying
to unblock. Concrete resolution (adapter-owned, no ws-mcp/Pi-core change):

- Reuse the SAME per-agent temp `sessionDir` `spawnAgent` already creates
  (`agents-plugin-pi/src/spawner.ts#L757-L759`,
  `mkdtempSync(join(tmpdir(), "ws-pi-agent-"))`). Add a sibling
  `approvals/` directory under it (lazily created).
- The gated-exec tool's `execute(toolCallId, params, signal, _onUpdate, ctx)`
  writes nothing (the PARENT already observes `{command, rationale}` via the
  existing `tool_execution_start` forwarding — no request file needed) and
  polls (fixed-interval `setInterval`, cleared on resolve; also resolves
  early on `signal`'s `"abort"` event, mirroring `exec.js`'s own
  `execCommand` pattern) for
  `<approvalsDir>/<toolCallId>.decision.json` to appear. The dir path is
  passed to the child via a new spawn-time env var (parallel to
  `WS_PI_SPAWN_ROLE_ENV`), since the child has no other reliable way to
  learn its own `sessionDir`.
- `ws.approve` (parent-side tool) validates `(agent_id, cmd_id)` against
  `record.pendingApproval` (race-binding: reject if no pending approval, or
  its `cmdId` doesn't match `cmd_id` — this is the ticket's race-prevention
  requirement, and it is pure/unit-testable), then writes
  `<approvalsDir>/<cmd_id>.decision.json` with
  `{decision, reason?, command?}` and clears `record.pendingApproval`.
- This is the "adapter relays the decision back to the worker" half of §8's
  baseline; the child->parent half (request surfacing) reuses the
  already-landed report-relay wiring pattern verbatim.

### Approval-request delivery to the lead ("interleaved injected lead turns," §3/§8)

- `agents-plugin-pi/src/goal-loop.ts#L365-L387` (`agent_settled` handler) is
  the direct precedent: adapter code reacting to an event calls
  `pi.sendUserMessage(...)` to inject a new turn into the LEAD's own running
  session. This ticket generalizes the same mechanism to a DIFFERENT trigger
  (a child's forwarded `tool_execution_start` for the gated-exec tool,
  observed via `record`'s event listener) instead of the lead's own
  `agent_settled`.
- `pi.sendUserMessage` and `pi.setActiveTools`/`pi.getActiveTools` are all on
  `ExtensionAPI` itself (module-scope `pi`, confirmed
  `types.d.ts` lines ~930-1078), so the relay code (which fires from a raw
  `RpcClient.onEvent()` callback with no `ExtensionContext` available) CAN
  call `pi.sendUserMessage(payload, { deliverAs: "steer" })` directly.
  `deliverAs: "steer"` is the safe unconditional choice here (no `ctx` is
  available to check `isIdle()` from this callback site, and `steer` is
  valid whether or not the lead is currently streaming, per `rpc.md`).
- Phase 1 reuses `ws-agent-wait`/reports machinery for nothing new: the
  approval-request push is the ONE genuinely new async-delivery path;
  ordinary `ws-agent-wait`/`ws-report-to-lead` stay byte-for-byte as landed.

### `ws.execute`'s worker: fixed system prompt, not lead-rendered

- `agents-plugin-pi/src/index.ts#L96-L97` — precedent for a plain
  adapter-owned root file resolved directly (`piLeadGuidePath = join(pluginDir,
  "pi-lead-guide.md")`), loaded via `readFileSync`, NOT through ws-mcp's
  `playbook.render`. The new execute-worker prompt must follow this same
  pattern (a new root file, e.g. `agents-plugin-pi/execute-worker-guide.md`),
  not `rsrc/<name>/<name>.md` (that directory is exclusively for ws-mcp
  `playbook.render` sources hand-synced from `agents-plugin/rsrc/`, which is
  explicitly the wrong mechanism per the ticket: "the lead authors no prompt
  prose, so it is NOT a lead-rendered playbook").
- `agents-plugin-pi/package.json#L12-L20` (`"files"`) lists
  `pi-lead-guide.md` explicitly for npm packaging — the new file needs the
  same treatment.

### Naming choices (engineering plumbing, not ticket-mandated strings)

- Registered tool names: adapter-local custom tools in this codebase use
  hyphens (`ws-agent-spawn`, `ws-report-to-lead`, `goal-achieved`), never the
  bridged `ws__*` dotted-sanitized form (that's reserved for ws-mcp-proxied
  tools, see `bridge.ts#L264-L266`). Plan uses `ws-execute` / `ws-approve`
  (hyphenated) as the registered names, matching sibling convention; the
  ticket's `ws.execute`/`ws.approve` notation is conceptual/prose, mirroring
  how `ws/playbook.render` prose already maps to a differently-shaped
  registered name for bridged tools.
- Gated-exec tool name: `ws-worker-exec` (distinct from `ws-execute`, the
  lead-facing verb, to avoid confusion in tool-call logs/prompts).
- Ugly-named read tool: the ticket itself proposes a concrete example name —
  `do-i-really-have-to-read-this-myself` — adopted verbatim to avoid a
  needless bikeshed.
- Model alias keys for `complex?`: reuse the EXISTING `"small"` alias
  (already the implicit key `explore` resolves through,
  `model-catalog.ts#L13-L16`) for the non-`complex` default, and introduce
  one new key, `"complex"`, for `complex: true` — zero new curation burden
  for a user who already set up `small` for `explore`.

## Implementation Plan

1. **Spec anchors first** (`ai-docs/spec/pi-adapter-runtime.md`, target file
   per the ticket's Spec Impact section). Follow the existing `260904-pi-*`
   anchor-naming convention already used for sibling landed sections (e.g.
   `#L322` `{#260904-pi-spawner-bounded-depth-explore-leaf}`). Add anchors
   for: the `ws.execute`/`ws.approve` contract + approval vocabulary
   (`approve`/`deny(reason)`/`run-instead(command)`), the §2 gate-scope
   invariant verbatim, the gated-exec tool + mutation-incapable read-family,
   the §7 payload/context-header shape, and the lead tool-surface change
   (bash removed, ugly-read retained, gated-exec excluded from the lead's
   active list). Read `ws/convention.read` for spec conventions before
   writing (per AGENTS.md's "Before editing... Specs" rule); do not invent
   the contract prose beyond what Decisions §2-§8 already settle verbatim.

2. **`agents-plugin-pi/src/spawner.ts` — thread a caller-supplied tool group
   through spawn/resume.**
   - Extend `ToolGroup` (`#L70`) with `"execute-worker"`.
   - Extend `TOOL_GROUPS` (`#L125-L129`) with
     `"execute-worker": [...TOOL_GROUPS["read-only"], GATED_EXEC_TOOL_NAME,
     REPORT_TO_LEAD_TOOL_NAME, "explore"]` (import `GATED_EXEC_TOOL_NAME`
     from the new `execute-gateway.ts`, mirroring how `REPORT_TO_LEAD_TOOL_NAME`
     is already a named export consumed by `TOOL_GROUPS` in the same file —
     or, to avoid a circular import, define `GATED_EXEC_TOOL_NAME` in
     `spawner.ts` itself next to `REPORT_TO_LEAD_TOOL_NAME` and have
     `execute-gateway.ts` import it, same direction as the existing
     `REPORT_TO_LEAD_TOOL_NAME` usage).
   - Add `toolGroup: ToolGroup` to `RpcAgentRecord` (`#L571-L598`) and
     `RpcSpawnCtx`/`SpawnAgentParams` (`#L604-L619`) — plumb through
     `spawnAgent` (set `record.toolGroup = ctx.toolGroup ?? "full-worker"` so
     existing `ws-agent-spawn` callers are unaffected) and replace both
     hardcoded `resolveTools("full-worker", ...)` call sites (`#L780`,
     `#L839`) with `resolveTools(record.toolGroup, ...)`.
   - Extend `applyRpcEvent` (`#L702-L715`) with a new branch: when
     `evt.type === "tool_execution_start" && evt.toolName ===
     GATED_EXEC_TOOL_NAME`, set `record.pendingApproval = { cmdId:
     evt.toolCallId, command: evt.args.command, rationale: evt.args.rationale
     }` (extend the function's narrow event-shape param type to also accept
     `toolCallId?: string`). Add `pendingApproval?: {cmdId, command,
     rationale}` to `RpcAgentRecord`.
   - Extend `AgentToolsHandle` (`#L1081-L1091`) with `rpcRegistry:
     RpcAgentRegistry` and return it from `registerAgentTools` (`#L1336+`)
     alongside `stopAll`.

3. **New file `agents-plugin-pi/execute-worker-guide.md`** — the fixed,
   adapter-authored execute-worker system prompt (append-system-prompt
   content). Content: you have free `read`/`grep`/`find`/`ls`; ANY shell
   command — including a "read" that mutates via redirection/`-exec` — must
   go through `ws-worker-exec`, which will pause for lead approval; call
   `ws-report-to-lead` for progress; call `explore` for a scoped read-only
   sub-question; on `deny`, re-plan and resubmit a revised command; on
   `run-instead`, treat the substituted command's output as authoritative.
   Add to `package.json`'s `"files"` array.

4. **New file `agents-plugin-pi/src/execute-gateway.ts`.** Pure helpers
   first (unit-tested), then IO glue:
   - `GATED_EXEC_TOOL_NAME` constant (or re-export from spawner.ts per step
     2's import-direction note).
   - `buildExecuteWorkerPrompt({command?, output?, prompt}): string` — pure,
     composes the initial prompt handed to the spawned worker (`Verbatim
     command already run: ...` block only when `command` is given, followed
     by the lead's `prompt`).
   - `resolveExecuteModelAlias(complex?: boolean): string` — pure, returns
     `"complex"` or `"small"`.
   - `validatePendingApproval(pending: {cmdId,...} | undefined, cmdId:
     string): {ok: true} | {ok: false, reason: string}` — pure race-binding
     check (no pending -> reject; `cmdId` mismatch -> reject; match -> ok).
     This is the `cmd_id` race-prevention logic named in the ticket's
     verification boundary, directly unit-testable with fake records.
   - `computeLeadActiveTools(currentActive: readonly string[]): string[]` —
     pure: removes `"bash"`, `"read"`, and `GATED_EXEC_TOOL_NAME` from
     `currentActive`; adds `"ws-execute"`, `"ws-approve"`,
     `"do-i-really-have-to-read-this-myself"` if not already present;
     dedupes. This is the concrete fix for the auto-include footgun found
     above.
   - `buildApprovalPromptText(payload: {agent_id, cmd_id, command, rationale,
     context}): string` — pure, formats the §7 payload into the text handed
     to `pi.sendUserMessage`, instructing the lead to call `ws-approve`.
   - `scrapeWorkingContext(cwd: string): {cwd, worktree_root?, branch?,
     ahead_behind?: string, dirty?: boolean}` — IO wrapper (git CLI via
     `node:child_process.execFileSync`, each call independently try/caught,
     never throws — matches `model-catalog.ts`'s never-hard-fail
     convention). Runs on the PARENT side (same machine/filesystem as the
     worker's `cwd`, which the parent already knows from spawn time — no
     worker self-report involved, satisfying §7's "adapter-scraped ground
     truth, not worker-reported").
   - `approvalDecisionPath(sessionDir: string, cmdId: string): string` —
     pure path builder (`join(sessionDir, "approvals", `${cmdId}.decision.json`)`).
   - `waitForDecisionFile(path, signal, pollMs=200): Promise<Decision |
     "aborted">` — IO: `setInterval` + `existsSync`/`readFile`+cleanup;
     resolves `"aborted"` on `signal`'s `"abort"` event.
   - `registerExecuteGateway(pi, bridge, rpcRegistry, sessionCtx: {cwd,
     modelCatalogPath, executeWorkerPromptPath})`:
     - Registers `ws-worker-exec` (declarative, global — reachable only via
       the new `"execute-worker"` `--tools` group, same pattern as
       `ws-report-to-lead`): `execute(toolCallId, {command, rationale, cwd?},
       signal, _onUpdate, ctx)` calls
       `waitForDecisionFile(approvalDecisionPath(<own sessionDir>, toolCallId),
       signal)`; the child derives its own `sessionDir` from the new
       spawn-time env var added in step 2/5 below. On the resolved decision:
       `approve` -> `ctx.exec("sh", ["-c", command], {cwd: args.cwd ??
       process.cwd(), signal})`; `deny` -> return the reason as a re-plan
       instruction, no execution; `run-instead` -> execute the substituted
       command, tell the model the lead substituted it; `"aborted"` -> return
       an aborted result.
     - Registers `ws-execute` (declarative, global; visible only to the lead
       via step 6's `setActiveTools`): validates `{command?, prompt,
       complex?}`, optionally runs `command` verbatim via `ctx.exec("sh",
       ["-c", command], {cwd: sessionCtx.cwd})` in the LEAD's own process
       (no gate — the lead itself supplied this exact string as a tool-call
       param, already at lead/user-consensus trust per §2), builds the
       initial prompt via `buildExecuteWorkerPrompt`, and calls the (now
       `toolGroup`-aware) `spawnAgent` from `spawner.ts` with `toolGroup:
       "execute-worker"`, `systemPromptPath: sessionCtx.executeWorkerPromptPath`,
       `modelName: resolveExecuteModelAlias(complex)`. Returns
       `{agent_id}` immediately (fire-and-return, per §3 — never awaits
       worker completion).
     - Registers `ws-approve`: `{agent_id, cmd_id, decision, reason?,
       command?}` -> look up `record = rpcRegistry.get(agent_id)`, validate
       via `validatePendingApproval(record?.pendingApproval, cmd_id)`
       (reject stale/mismatched `cmd_id`), write the decision file via
       `approvalDecisionPath` + `fs.writeFileSync`, clear
       `record.pendingApproval`.
     - Attaches, per spawned `RpcAgentRecord`, a listener (piggybacking on
       the SAME `client.onEvent`/`applyRpcEvent` wiring `spawner.ts` already
       attaches in `attachEventListener`) that reacts to a freshly-set
       `record.pendingApproval` by calling `scrapeWorkingContext` +
       `buildApprovalPromptText` + `pi.sendUserMessage(text, {deliverAs:
       "steer"})`. Concretely: extend `attachEventListener`
       (`spawner.ts#L717-L719`) to accept an optional `onApprovalPending`
       callback invoked right after `applyRpcEvent` sets
       `record.pendingApproval`, and have `registerAgentTools` wire that
       callback (only when `record.toolGroup === "execute-worker"`) to a
       function this new module exports — keeps `spawner.ts` generic
       (no `pi.sendUserMessage` import) while `execute-gateway.ts` owns the
       injection behavior.

5. **Spawn-time env var for the child's approval directory.** Extend
   `buildRpcClientOptions` (`spawner.ts#L636-L650`) with an optional 6th
   param (or fold into the existing `env` object unconditionally, since it's
   inert for a `"full-worker"` spawn that never dispatches
   `ws-worker-exec`): add `WS_PI_APPROVAL_DIR: join(sessionDir, "approvals")`
   to the child's env alongside `WS_PI_SPAWN_ROLE_ENV`. `ws-worker-exec`'s
   `execute()` reads `process.env.WS_PI_APPROVAL_DIR` to build its own
   decision-file path — no other channel exists for the child to learn its
   `sessionDir`.

6. **`agents-plugin-pi/src/index.ts` — lead tool-surface reshaping and gateway
   wiring.**
   - Resolve `executeWorkerGuidePath = join(pluginDir,
     "execute-worker-guide.md")` alongside the existing `piLeadGuidePath`
     (`#L97`).
   - In the `session_start` handler (`#L143-L171`), after
     `registerAgentTools` returns `handle`/`agentTools`, call
     `registerExecuteGateway(pi, handle, agentTools.rpcRegistry, { cwd:
     ctx.cwd, modelCatalogPath, executeWorkerPromptPath: executeWorkerGuidePath
     })` — call once regardless of role (mirrors `registerAgentTools`'s own
     unconditional call; the tools are simply never exposed to a
     worker/explore's own `--tools` list).
   - Gated on `isLeadOrFork(readSpawnRole(process.env))` (same gate already
     used for the ws block at `#L160`), call
     `pi.setActiveTools(computeLeadActiveTools(pi.getActiveTools()))`.

7. **`agents-plugin-pi/pi-lead-guide.md` — extend the verb table.** Add two
   rows (`ws-execute` / `ws-approve`) with the §2 one-line gate-scope
   invariant folded into the `ws-execute` row's description, and remove
   "an execute/approve gateway" from the closing "grows as later tickets
   land" footnote's parenthetical (it has now landed) — keep "a fork/ask/
   resolve side-thread surface" since that ticket is still pending.

8. **Tests.**
   - `agents-plugin-pi/test/spawner.test.ts`: extend for the new
     `"execute-worker"` `TOOL_GROUPS` entry (equals `read-only` + 3 named
     custom tools), `applyRpcEvent`'s new `pendingApproval`-setting branch
     (mirrors the existing `pendingReports` test shape), and
     `resolveTools(record.toolGroup, ...)` threading (a fake record with
     `toolGroup: "execute-worker"` produces the execute-worker tool list).
   - New `agents-plugin-pi/test/execute-gateway.test.ts`: unit-test every
     pure function listed in step 4 (`buildExecuteWorkerPrompt`,
     `resolveExecuteModelAlias`, `validatePendingApproval`,
     `computeLeadActiveTools`, `buildApprovalPromptText`,
     `approvalDecisionPath`). Do NOT attempt to unit-test
     `scrapeWorkingContext`/`waitForDecisionFile`/the tool `execute()`
     bodies — these need a real filesystem/subprocess/live `pi` session; call
     that out explicitly as live-verification-only, matching
     `spawner.test.ts`'s own documented split (its header comment already
     lists `spawnAgent`/`stopAgent`/`exploreLeaf` as "genuinely live-gate
     only").

## Verification Plan

- `cd agents-plugin-pi && npm test` (baseline: 235 tests / 50 suites passing
  before this change — re-run after each landing commit; the new pure
  helpers should land as additional passing tests with the existing 235
  untouched).
- `node --check` on every changed/new `.ts` file
  (`src/spawner.ts`, `src/execute-gateway.ts`, `src/index.ts`).
- Manual/live gate (explicitly deferred — no provider credentials in this
  sandbox, per task authority): a `pi --mode rpc` session exercising the
  ticket's own Phase 1 verification list — free worker reads (no gate), a
  mutating command elevating to an injected `ws-approve` prompt with the §7
  context header + rationale, `deny(reason)` producing a revised command,
  `run-instead(command)` substituting with output routed to the worker,
  `cmd_id` staleness rejection (approve a re-used/old `cmd_id` and confirm
  `ws-approve` rejects it via `validatePendingApproval`), `ws-agent-stop`
  aborting mid-plan and unblocking `ws.execute`, and — the ticket's own
  required Phase 1 report — confirming `pi.getActiveTools()` on the lead
  session no longer lists `bash`/`read` and does list `ws-execute`/
  `ws-approve`/the ugly-read tool but NOT `ws-worker-exec`, both immediately
  after `session_start` and after a manual `/reload`. This gate cannot run
  in this sandbox; document it as the open manual step in the landing
  commit(s), do not claim it as executed.

## Escalations

- None. The §8 linchpin (hard tool-surface reshaping) is resolved as
  feasible and reload-durable by direct inspection of the installed
  `@earendil-works/pi-coding-agent` package (`agent-session.js`'s
  `setActiveToolsByName`/`_refreshToolRegistry`/`reload()`), consistent with
  the task's instruction not to escalate this point. The approval-relay
  transport (a new, ticket-acknowledged primitive) has a concrete, buildable
  design reusing existing per-agent `sessionDir` plumbing and the landed
  report-relay/goal-loop injection patterns — an implementation/engineering
  choice, not an unresolved contract or strategy question. No scope
  reduction is proposed: every Phase 1 element enumerated in the ticket's
  Phase 1 paragraph is carried into the Implementation Plan above.
