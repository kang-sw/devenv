# Plan: 260902-feat-ws-pi-native-mvp — Phase 2: Self-built subagent spawner + explore primitive

## Relevant Ticket Contract

- `ws-agent-spawn({ playbook, task, tier? })` / `ws-agent-continue({ agentId, task })` /
  `ws-agent-wait({ agent-ids, policy: "any"|"all", timeout? })` — module-state registry
  (`Map<agentId, {proc, sessionPath, systemPromptPath, state, output, drainer}>`),
  pipe-deadlock-safe background stdout draining that parses `pi --mode json` events and
  flips `state:"done"` on terminal `stopReason`, `session_shutdown` cleanup, file-based
  continuation via `--session <ws-owned-path>` hidden from `/resume`
  (`ai-docs/tickets/ready/260902-feat-ws-pi-native-mvp.md#L207-215`).
- `explore({ query, async? })` — thin one-shot preset: fixed `playbook=explore`,
  `--tools=recon`, `--no-session`, self-reaping leaf, no continue (`#L216-218`).
- Playbook injection: render `ws/playbook.render(name)` once at spawn into a ws-owned
  temp path, pass as `--append-system-prompt`; reuse the same path on `continue`
  (`#L219-221`).
- `--tools` resolves from a **tool-group table created in this phase**
  (read-only / recon / full-worker → Pi tool-name allowlists); `--model` resolves as
  **inherit** this phase (tier→model map is Phase 3) (`#L222-227`).
- MVP depth is 0→1 leaf; no nested-spawn tool exposed to depth-1 workers (`#L227`).
- Gate: lead spawns a worker and an `explore` leaf, `wait` harvests both, `continue` on
  the worker resumes its session (`#L229-230`).
- Golden rule: ws-mcp Go source untouched; all Pi-specific policy lives in the adapter
  (ticket `## Constraints`).
- Zero ws agent-profile files — curation via `pi` CLI flags only, no `.pi/agents/` or
  global profile writes (ticket `## Decisions`, 3rd bullet).

## Out of Scope

- Phase 3 (tier→model catalog, `--model` resolution beyond inherit, `workflow_manual`
  unset-map advisory) and Phase 4 (`/ws-discuss` PoC command) — explicitly deferred by
  the ticket's own phase split.
- Durable depth-2 recursion, always-visible TODO, goal-loop, compaction hooks — ticket
  `## Phases` Phase 4 paragraph names these as follow-up-ticket expansion, not MVP.
- Re-litigating Phase 1's bridge/tool-sanitization/session-key behavior — already landed
  and documented in `ai-docs/spec/pi-adapter-runtime.md`; Phase 2 only consumes it.
- `--append-system-prompt` vs `--system-prompt` tuning — recorded as an explicit open,
  low-risk item deferred past MVP (`ai-docs/tickets/idea/260802-research-ws-pi-native-framework.md#L780-784`).

## Codebase Findings

- `agents-plugin-pi/src/index.ts#L48-69` — established extension-lifecycle pattern:
  module-scope mutable state (`let handle`), tools/state created in `session_start`
  (Pi forbids background-process start at top-level factory), torn down idempotently in
  `session_shutdown`. Phase 2's agent registry should follow the same
  session_start-init / session_shutdown-cleanup shape, adding a kill pass over any
  still-running spawned `pi` children on shutdown (no such cleanup exists yet).
- `agents-plugin-pi/src/mcp-stdio-client.ts#L79-115` (`JsonRpcLineBuffer`) and
  `#L167-225` (`McpStdioClient` constructor) — the exact pipe-deadlock-safe /
  multibyte-safe draining pattern the ticket asks for: attach `.on("data")` to both
  stdout and stderr immediately at spawn, decode through `node:string_decoder`'s
  `StringDecoder` (not `Buffer#toString()` per chunk) so a UTF-8 codepoint split across
  a `'data'` chunk boundary decodes correctly, and register `proc.on("error")` /
  `proc.on("exit")` handlers that reject/settle pending work instead of hanging. The
  spawner's own line buffer is a different message shape (bare `AgentSessionEvent` NDJSON,
  not a JSON-RPC envelope), so this is a pattern to mirror in a small parallel class, not
  a class to import directly.
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/examples/extensions/subagent/index.ts#L249-263`
  (`getPiInvocation`) and `#L300-421` (`runSingleAgent`) — an **officially shipped Pi
  example doing almost exactly this phase's job** (spawns `pi` subprocess per
  delegation, `--mode json -p --no-session`, `--model`/`--tools`/`--append-system-prompt`
  flag construction, naive NDJSON line-split parsing, `stopReason` capture off
  `message_end.message.stopReason`, completion driven by `proc.on("close")`). Confirms
  every flag name/shape the ticket assumes is live-correct against the installed build
  (package.json pins `@earendil-works/pi-coding-agent` and the example ships inside the
  same installed package tree, version `0.84.4`). Two things NOT to copy: (1) its naive
  `data.toString()` buffer split (no `StringDecoder`) — reuse the multibyte-safe pattern
  from `mcp-stdio-client.ts` instead; (2) its per-role `.pi/agent/agents/*.md` profile-file
  discovery (`agents.ts`) — the ticket's Decisions section explicitly rejects on-disk
  agent-profile files for ws, curation must stay in adapter-owned in-memory tables plus
  CLI flags only.
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/usage.md`
  (`### Modes`, `### Model Options`, `### Session Options`, `### Tool Options`,
  `### Other Options` tables) — live-confirms every flag the ticket names on the
  installed 0.84.4 build: `--mode json`, `-p`/`--print`, `--session <path|id>`,
  `--no-session`, `--append-system-prompt <text>` (append, distinct from
  `--system-prompt` which replaces), `--tools <list>`/`-t <list>` ("Allowlist specific
  **built-in, extension, and custom** tools" — confirms the bridge's `ws__*` registered
  tool names are valid `--tools` allowlist entries alongside built-ins `read, bash,
  edit, write, grep, find, ls`), `--model <pattern>` (`provider/id[:thinking]`). No
  flag-name or semantic drift found versus the ticket's assumed contract.
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/json.md`
  and `docs/session-format.md#L88` — confirms `--mode json` NDJSON event shape
  (`AgentSessionEvent`) and the exact `stopReason` enum:
  `"stop" | "length" | "toolUse" | "error" | "aborted"` (`"pending"` is stream-only,
  never persisted/terminal). `"toolUse"` is **not** a terminal state — only
  `stop`/`length`/`error`/`aborted` should flip the registry entry to `state:"done"`.
- **Risk signal — terminal-state gating must key off process exit, not just the
  stream's terminal `stopReason`.** `ai-docs/tickets/idea/260802-research-ws-pi-native-framework.md#L760-766`
  ("Session file flush on process exit is complete (was Q9)") establishes that the
  `--session` file is only guaranteed flushed **after the child process exits**, and
  that guarantee is what makes an immediate `continue` after `wait` harvests safe. The
  shipped reference example (`index.ts#L401-404`) also only resolves completion from
  `proc.on("close", ...)`, using the last-seen `stopReason` merely to classify
  success/failure, not to signal completion. If the spawner instead flips
  `state:"done"` the moment a terminal `stopReason` is *observed in the stream* (before
  the process has actually exited/flushed), a `continue` issued in that window could
  read a not-yet-flushed session file. Implementation must gate `state:"done"` on the
  child's `exit`/`close` event, using the last captured `stopReason` only as metadata.
- `ai-docs/spec/mcp-tools.md#L1871-1896` — `playbook.render(session_key, name, context?,
  root_override?)` **already materializes the rendered prompt to a worktree-scoped temp
  file itself** and returns ordered lines (path first, then optional
  `recommended-tier`/`recommended-model`/`recommended-reasoning-effort`). The spawner
  does not need to write its own temp file for the system prompt — it calls the
  already-bridged `ws__playbook_render` tool and takes the first returned line as the
  `--append-system-prompt` path. This directly satisfies "reuse the same path on
  `continue`" (ticket `#L219-221`): cache that same path in the registry entry, do not
  re-render on continue.
- `ai-docs/spec/mcp-tools.md#L2179-2186` ("Render-minted child keys") — when the
  *calling* `session_key.role == lead` **and** the target playbook's frontmatter
  declares a delegate-eligible `role:`, `playbook.render` itself mints a fresh child
  session key and splices it into the rendered prompt — the spawner does not need to
  call `ferrule` itself for role-bearing playbooks (e.g. `implementer`, `tier: medium`,
  see `agents-plugin-pi/rsrc/implementer/implementer.md#L1-12`).
- `agents-plugin-pi/rsrc/explore/explore.md#L1-8` — the `explore` playbook **has no
  `role:` frontmatter field** (only `kind: render`, `delegates: true`, `variables`),
  unlike `implementer.md`. Per the mechanism above, rendering it will **not** auto-mint
  or splice a child session key. This is consistent with the ticket's `explore` design
  (no continuation, self-reaping, `--tools=recon` presumably built-in-only) but is
  worth the executor confirming: if the resolved `recon` tool group ever includes any
  `ws__*` bridge tool, the exploded worker will have no ws session key embedded in its
  prompt and those calls will fall back to the bridge's own default-filled key
  (`bridge.ts#L109-123`), not a scoped child key.
- `agents-plugin-pi/test/bridge.test.ts#L1-40` and `package.json#L10-12` — established
  test convention: `node --test` (Node native TS type-stripping, zero build step), pure
  logic factored out and unit-tested without spawning real subprocesses; a fixed
  fixture snapshot stands in for live `tools/list` data rather than re-fetching it per
  test run. Phase 2's tool-group resolution, arg-building, and stopReason
  classification should follow the same pure-function/no-subprocess unit-test shape.
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/extensions.md#L1013-1017`
  — `ctx.model` (active model, `{provider, id, ...}`) and `ctx.thinkingLevel` are
  available inside a registered tool's `execute(toolCallId, params, signal, onUpdate,
  ctx)`. The shipped reference example forwards these as its own "inherit" default
  (`index.ts#L301-306`: `dispatchDefaults.model = ctx.model ? ... : undefined`). This is
  the concrete, precedented mechanism for the ticket's "Phase 2 resolves `--model` as
  inherit": either omit `--model` entirely (pi's own default resolution) or forward
  `ctx.model`/`ctx.thinkingLevel` explicitly, matching the shipped pattern.
- `/home/linuxbrew/.linuxbrew/lib/node_modules/@earendil-works/pi-coding-agent/CHANGELOG.md`
  `## [0.84.4] - 2026-08-28` — "Fixed resumed sessions corrupting the next appended
  entry when their JSONL file lacks a trailing newline." The installed build (0.84.4,
  matches `agents-plugin-pi/`'s pinned expectation) already carries this fix, so the
  Q8/Q9 resume-append guarantees the research anchor spiked at 0.83.0 are not known to
  have regressed by 0.84.4 — no drift found between the anchor's spike version and the
  installed version for any flag/behavior this phase depends on.
- `ai-docs/spec/pi-adapter-runtime.md#L121-125` — the Phase 1 spec doc already
  anticipates this phase, explicitly noting the delegation spawner/`explore` are "not
  yet implemented" and "not part of this contract yet." Phase 2 implementation should
  add a new `##`-level section to this same spec file for the spawner surface (not a
  new spec stem) — flagged here as a required doc-update step, not a code step.

## Implementation Plan

1. Add `agents-plugin-pi/src/spawner.ts` with pure, unit-testable pieces first:
   - `TOOL_GROUPS` table (`read-only`, `recon`, `full-worker` → Pi tool-name arrays),
     built-ins from `docs/usage.md`'s `read, bash, edit, write, grep, find, ls`;
     `full-worker` additionally needs the bridge's live `ws__*` names — take them as a
     parameter (from the already-listed `tools` in `bridge.ts#L145`) rather than
     hardcoding, so the group tracks ws-mcp's actual tool set instead of drifting.
   - `resolveTools(group, wsToolNames): string` → comma-joined `--tools` value.
   - `isTerminalStopReason(reason): boolean` → true only for
     `"stop"|"length"|"error"|"aborted"` (`session-format.md#L88`).
   - `buildSpawnArgs({ mode: "spawn"|"continue"|"explore", sessionPath?, promptPath?,
     tools, model?, noSession, task }): string[]` mirroring
     `examples/extensions/subagent/index.ts#L300-341`'s flag-ordering pattern
     (`--mode json -p [--session <path> | --no-session] [--append-system-prompt <path>]
     --tools <list> [--model <pattern>] "<task>"`).
   - A small NDJSON line buffer modeled on `mcp-stdio-client.ts#L79-115`'s
     `JsonRpcLineBuffer` (StringDecoder-based, multibyte-safe) but parsing bare
     `AgentSessionEvent` lines instead of JSON-RPC envelopes.
2. Add the module-state registry (`Map<agentId, AgentRecord>`) and the async
   spawn/continue/wait engine in `spawner.ts`:
   - `spawnAgent(client, ctx, { playbook, task, tier? })`: calls
     `client.callTool("playbook.render", { session_key, name: playbook })` (reuse the
     bridge's already-connected `McpStdioClient`, threaded out of `startBridge` —
     `bridge.ts#L125-210` currently keeps it local; expose it via the returned
     `BridgeHandle` or a shared module so the spawner can issue its own `playbook.render`
     / `ferrule` calls), take the first response line as `systemPromptPath`, generate a
     ws-owned `sessionPath` (fresh temp path outside `~/.pi/agent/sessions/`), spawn `pi`
     via `node:child_process.spawn` with `buildSpawnArgs(...)`, attach stdout/stderr
     drains immediately (deadlock-safe per `mcp-stdio-client.ts#L187-199` precedent),
     and register the entry as `state:"running"`.
   - On the child's `exit`/`close` event (not on an in-stream terminal `stopReason` —
     see the Q9 risk finding above): record the last captured `stopReason`, flip
     `state:"done"`.
   - `continueAgent(agentId, task)`: reject unless `state==="done"`; re-spawn with the
     **same** `sessionPath` and `systemPromptPath` (no re-render — this is what makes
     "non-duplicated system prompt across resumes" hold, per the Q9-bonus finding).
   - `waitAgents(agentIds, policy, timeout)`: partitions done/running/timedOut by
     polling registry state; never kills a running process on timeout (ticket `#L215`).
   - `exploreLeaf(client, ctx, { query, async? })`: fixed `playbook: "explore"`,
     `resolveTools("recon", ...)`, `noSession: true`, spawn-and-self-reap (drop the
     registry entry once done; no continue path).
3. Register the four new tools (`ws-agent-spawn`, `ws-agent-continue`, `ws-agent-wait`,
   `explore`) in `agents-plugin-pi/src/index.ts`'s existing `session_start` handler
   (`#L55-63`), alongside the bridge. Add a kill pass over any still-`running` registry
   entries in the `session_shutdown` handler (`#L65-68`), matching the existing
   `handle?.shutdown()` idempotency pattern.
4. Add `agents-plugin-pi/test/spawner.test.ts` (`node --test`, following
   `test/bridge.test.ts`'s shape) covering: `resolveTools` per group,
   `isTerminalStopReason` for all five enum values, `buildSpawnArgs` for each of
   spawn/continue/explore shapes (flag presence/absence, `--session` vs `--no-session`
   mutual exclusion, `--model` omitted when `tier` unset), and the NDJSON line buffer's
   multibyte-split-safety (mirroring `mcp-stdio-client.test.ts`'s existing multibyte
   test, if present — confirm during implementation).
5. Add a new `## Delegation spawner` section to `ai-docs/spec/pi-adapter-runtime.md`
   (append after the existing Phase 1 sections; do not edit the Phase 1 text) covering
   the four new tools' caller-visible contract, matching the doc-update pattern Phase 1
   already established for this same file.

## Verification Plan

- `cd agents-plugin-pi && node --test` — unit coverage for the new pure logic
  (tool-group resolution, arg-building, stopReason classification, line-buffer
  multibyte safety), same command Phase 1 used.
- Live gate (manual, against openrouter — the only ready provider per Phase 1's
  Result): from a lead-scoped Pi session, `ws-agent-spawn` a worker on the
  `implementer` playbook and separately call `explore` on a trivial query; confirm both
  produce running registry entries; call `ws-agent-wait` with `policy: "all"` and
  confirm it harvests both after their processes exit (not merely after a terminal
  `stopReason` appears in-stream); call `ws-agent-continue` on the worker immediately
  after `wait` reports it done and confirm the resumed turn sees prior-turn context
  (mirrors the anchor's own Q8 semantic-recall check,
  `ai-docs/tickets/idea/260802-research-ws-pi-native-framework.md#L757-759`).
- Confirm `agents-plugin-pi/rsrc/`, `runtime.json`, and `bin/ws-mcp-launcher.py` are
  untouched by this phase (golden-rule / hand-sync-copy check, same as Phase 1's
  verification).

## Escalations

None.
