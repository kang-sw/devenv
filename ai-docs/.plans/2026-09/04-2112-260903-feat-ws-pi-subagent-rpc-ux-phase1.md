# Plan: 260903-feat-ws-pi-subagent-rpc-ux — Phase 1: Persistent RpcClient children + parent-drive surface

## Relevant Ticket Contract

- Replace the one-shot `pi --mode json -p` worker spawner with persistent
  `RpcClient` (mode rpc) children. New tool surface: `ws-agent-spawn({
  system_prompt_path, prompt, model_name?, model_effort? })` -> `{agent_id}`,
  `ws-agent-send(agent_id, message, interrupt?)`, `ws-agent-wait(agent_ids[],
  timeout?)`, `ws-agent-list()`, `ws-agent-stop(agent_id)`. `ws-agent-continue`
  folds into `ws-agent-send`.
- D-A (Shape A): the spawn tool carries **no tier** parameter. The lead reads
  `playbook.render`'s `recommended-model`/`recommended-reasoning-effort` and
  optionally passes `model_name`/`model_effort`; omitted = inherit parent
  model. `system_prompt_path` is the lead-rendered playbook file passed
  directly — **the spawner itself no longer calls `playbook.render`**.
  `model-catalog.json` is reframed from a `tiers` map to a generic
  name→`provider/id` alias resolver; the unset-advisory trigger re-keys from
  "`small` tier unset" to "alias table empty."
- D-B: worker `--tools` excludes all driving/spawn tools except the
  non-recursive `explore` leaf (depth ≤ 2: lead → worker → explore-leaf,
  `explore` cannot spawn `explore`).
- D-C: `agent_id` -> session mapping is retained across `ws-agent-stop`
  (dormant/resumable); `ws-agent-send` on a dormant id auto-resumes via
  `--session` and delivers. `ws-agent-continue` is gone.
- Retained MVP surfaces (not dropped): per-spawn `--tools` group curation
  (`read-only`/`recon`/`full-worker`), the one-shot `explore` recon leaf
  (unchanged one-shot machinery), the model-catalog data file (reframed
  shape only).
- Verification boundary (Phase 1): live `pi … --mode rpc` run showing a
  follow-up into a spawned child, a dormant child auto-resumed on the SAME
  ws `session_key`, `ws-agent-wait` over two children returning the first
  finisher + last message, `ws-agent-list` reporting live children/status, a
  worker spawning `explore` but not another worker, teardown on
  `session_shutdown`, `cliPath` resolving via `process.argv[1]`. Registry/
  select logic unit-tested where seam-extractable.
- Non-goal: no ws-mcp (Go) changes; adapter-local only.

## Out of Scope

- Phase 2: `ws-report-to-lead` child→lead channel, drain-all-FIFO bounded
  report buffer, `ws-agent-transcript`, idle-reap. None of these tools/state
  are added in Phase 1.
- Goal-loop, compaction hooks, always-visible TODO (post-MVP, other tickets).
- Editing `ai-docs/spec/pi-adapter-runtime.md` anchors — ticket says this
  happens via `lead-write-spec` at proceed, not as a code-implementation step.
- Validating `model_effort` against Pi's exact `ThinkingLevel` enum
  (`off`/`minimal`/`low`/`medium`/`high`/`xhigh`/`max`) — the adapter forwards
  the caller's string as-is (never hard-fail is the existing house style; an
  unsupported value degrading to a no-op/provider error is acceptable, same
  as today's unrecognized-tier handling).
- `/ws-discuss` PoC command (`src/discuss.ts`) and skill/package-topology
  copy machinery — untouched by this phase.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L379-455,457-499` — current `spawnAgent`/
  `continueAgent` call `client.callTool("playbook.render", ...)` themselves
  and gate on `tier`. Both responsibilities move out: Phase 1's
  `ws-agent-spawn` takes an already-rendered `system_prompt_path` (no
  render call in the spawner at all) and `model_name`/`model_effort` instead
  of `tier`.
- `agents-plugin-pi/src/spawner.ts#L338-377` (`spawnPiProcess`) and
  `#L179-228` (`AgentEventLineBuffer`) — this whole one-shot
  process/NDJSON-event machinery is **retained unchanged** for `explore`
  (`exploreLeaf`, `#L617-668`), which the ticket explicitly keeps as one-shot.
  Only `ws-agent-spawn`/`send`/`wait`/`list`/`stop` move to `RpcClient`;
  `explore` is a separate, still-one-shot code path in the same file.
- `agents-plugin-pi/src/spawner.ts#L273-279` (`getPiInvocation`) — existing
  precedent for resolving the runnable pi entry: prefers re-invoking
  `process.execPath` against `process.argv[1]` (the currently-running script)
  over a bare `pi` command. `RpcClient.start()` in the installed package
  (`/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.js#L26-46`)
  **always** does `spawn("node", [cliPath, ...args])` with
  `cliPath = this.options.cliPath ?? "dist/cli.js"` — there is no bare-`pi`
  fallback inside `RpcClient` itself. So `ws-agent-spawn`/auto-resume must
  pass `cliPath: process.argv[1]` explicitly on every `new RpcClient(...)`;
  this is exactly the ticket's "gap #6 settled by `process.argv[1]`" note.
  If `process.argv[1]` is ever missing/non-existent there is no client-side
  fallback path (carry this forward as-is; it's the ticket's settled answer,
  not something to re-derive).
- `RpcClientOptions` (`.../dist/modes/rpc/rpc-client.d.ts#L15-26`) — `model?`
  and `args?: string[]` are separate; `start()` appends
  `--provider`/`--model` before the raw `args` array. So per-spawn
  `--session <path>`, `--append-system-prompt <path>`, `--tools <list>` all
  go through `args`, while `model_name`'s resolved `provider/id` goes through
  the dedicated `model` option (cleaner than string-splicing into `args`).
- **`model_effort` mechanism (resolves the ticket's open question).** Pi has
  no separate `--reasoning-effort` CLI flag. `docs/usage.md#L192` documents
  `--model <pattern>` supporting an optional `:<thinking>` suffix (e.g.
  `pi --model sonnet:high`), and `RpcClient` separately exposes
  `setThinkingLevel(level: ThinkingLevel)` as a live post-`start()` RPC call
  (`rpc-client.d.ts#L123`, wired to `docs/rpc.md`'s `set_thinking_level`
  command). Levels: `off|minimal|low|medium|high|xhigh|max`. Recommendation:
  use `setThinkingLevel()` after `client.start()` and before the first
  `prompt()` when `model_effort` is given — this decouples it from whether
  `model_name` was also given (string-suffixing `--model` would require a
  resolved model string to exist first; `setThinkingLevel()` doesn't).
- **`followUp()`/`steer()` require an active run — a real behavior gap in
  the ticket's literal tool mapping.** Traced through
  `.../dist/modes/rpc/rpc-mode.js#L325-328` -> `AgentSession.followUp`
  (`.../dist/core/agent-session.js#L1033-1042`) -> `Agent.followUp`
  (`.../node_modules/@earendil-works/pi-agent-core/dist/agent.js#L177-178`):
  `followUp`/`steer` only **enqueue**; the queue is drained solely inside the
  active agent-loop's `getFollowUpMessages`/steering poll
  (`agent.js#L296-323`), which only runs while `runWithLifecycle` has an
  active run. **A freshly-started or freshly-resumed idle `RpcClient` has no
  active run**, so calling `followUp()`/`steer()` against an idle child
  silently queues a message that is never delivered. `ws-agent-send` must
  branch on locally-tracked streaming state: while the child is mid-stream,
  use `steer()` (interrupt) / `followUp()` (queue) per the `interrupt` flag
  as the ticket says; while the child is idle (including immediately after
  an auto-resume `start()`), use `prompt()` instead — `interrupt` is
  meaningless with nothing running. Track streaming state locally via the
  `agent_start`/`agent_settled` events already needed for idle-wait (avoids
  an extra `getState()` round trip per send).
- **No local dependency on `@earendil-works/pi-coding-agent` exists yet.**
  `agents-plugin-pi/package.json` declares no `dependencies` at all, and
  there is no `agents-plugin-pi/node_modules/`. The three existing imports
  (`bridge.ts#L28`, `index.ts#L57`, `spawner.ts#L39`) are all `import type`
  — erased by Node's native TypeScript stripping, so they need no runtime
  resolution today. `RpcClient` is a runtime **value** import
  (`import { RpcClient } from "@earendil-works/pi-coding-agent"`), which
  Node must resolve for real. This requires adding
  `"@earendil-works/pi-coding-agent"` to `agents-plugin-pi/package.json`'s
  `dependencies` (installed version on this machine: `0.84.4`, confirmed via
  `/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/package.json`)
  and running `npm install` in `agents-plugin-pi/` so `node_modules/` is
  materialized. Without this, any code path constructing `new RpcClient(...)`
  throws `ERR_MODULE_NOT_FOUND` at runtime.
- `RpcClient` top-level export confirmed: `dist/index.d.ts#L27` re-exports
  `RpcClient`/`RpcClientOptions`/`ModelInfo`/`JsonAgentSessionEvent` etc. from
  `./modes/index.ts`, so `import { RpcClient, type RpcClientOptions } from
  "@earendil-works/pi-coding-agent"` is the correct top-level import (no deep
  `dist/modes/rpc/...` path needed).
- `RpcClient.stop()` (`rpc-client.js#L89-108`) is `async`: SIGTERM, wait up to
  1s, SIGKILL fallback. `agents-plugin-pi/src/index.ts#L123-130`'s current
  `session_shutdown` handler is synchronous and calls `agentTools.killRunning()`
  fire-and-forget (`spawner.ts#L848-860`, plain `proc.kill()`). The
  replacement teardown must `await` stopping every live `RpcClient`, so
  `session_shutdown`'s handler needs to become `async` (Pi's own
  `session_before_switch`/other lifecycle hooks are already documented as
  awaitable, per `docs/extensions.md#L432-433`'s cleanup guidance).
- `RpcClient.getLastAssistantText()` (`rpc-client.d.ts` / rpc.md
  `get_last_assistant_text`) directly gives the "last message" `ws-agent-wait`
  needs to auto-attach — no need to re-parse message content arrays the way
  `handleAgentEvent` (`spawner.ts#L301-324`) does for the old one-shot path.
- **D-C session-key lineage is satisfied "for free."** The child's rendered
  system prompt (with any ws session key already spliced in by the lead's own
  prior `playbook.render` call, per `ai-docs/spec/mcp-tools.md#L2100-2104`)
  is a static file at `system_prompt_path`. As long as auto-resume re-passes
  the SAME cached `systemPromptPath` via `--append-system-prompt` on every
  relaunch — exactly what the existing (soon-removed) `continueAgent`
  already did (`spawner.ts#L457-462` doc comment: "Reuses the cached
  systemPromptPath unchanged — no re-render — so the append-system-prompt
  text does not duplicate across resumes") — no new session-key bookkeeping
  is required in the extension; the lineage is already baked into the
  reused file, not re-derived per resume.
- `agents-plugin-pi/src/spawner.ts#L281-290` (`settleWaiters`/`waitForDone`)
  — directly reusable pattern for the new idle-edge-consume `ws-agent-wait`:
  swap the "done" trigger for an "idle" trigger (fired from the
  `agent_settled` event listener instead of the child-process `close` event),
  and swap `record.state` for a `record.streaming`/`idlePending` pair.
  `waitAgents`'s timeout handling (`#L549-598`, `Promise.race` against a
  `setTimeout` promise, never killing on timeout) is reusable verbatim for
  the new tool's timeout arg, minus the `policy: "any"|"all"` axis — Phase
  1's `ws-agent-wait(agent_ids[], timeout?)` signature (ticket phase text)
  drops `policy` entirely and always behaves as first-finisher.
- `agents-plugin-pi/src/model-catalog.ts` (whole file) and
  `agents-plugin-pi/model-catalog.json` (currently `{}`) — `ModelCatalogConfig
  .tiers: Partial<Record<ModelTier,string>>` + `resolveTierModel` +
  `isModelCatalogUnset` (keyed on `tiers.small`) all need reframing to a
  generic `aliases?: Record<string,string>` map + `resolveAlias(config,
  name)` + `isModelCatalogUnset` keyed on "aliases table absent/empty."
  `bridge.ts#L83-110` (`MODEL_CATALOG_ADVISORY` text + call site) needs
  wording updated to reference the alias table instead of "tier map"/"small
  tier"; `maybeAppendModelCatalogAdvisory`'s signature/logic is otherwise
  unaffected since it only forwards to `isModelCatalogUnset`.
- `explore`'s implicit model resolution (`spawner.ts#L838-843`,
  `resolveModel(toolCtx, "small")`) has no tier to resolve through anymore.
  Simplest same-behavior carry-forward: keep a fixed alias-key lookup (e.g.
  `resolveAlias(config, "small")`) against the reframed `aliases` table —
  nothing stops a user from naming an alias `"small"` in the new generic
  table, so `explore`'s cheap-model-by-default behavior is unchanged in
  practice, only the accessor's shape changes. This is a judgment call (the
  ticket doesn't respell explore's resolution key); flagged below.
- `agents-plugin-pi/test/spawner.test.ts` (`describe("resolveModelForTier")`
  `#L287-317`, `describe("asModelTier")` `#L319-335`) and
  `agents-plugin-pi/test/model-catalog.test.ts` (whole file, `tiers`-shaped
  fixtures throughout) both hard-code the old tier shape and need rewriting
  to the alias shape; `bridge.test.ts#L166-180` also builds a
  `{ tiers: { small: ... } }` fixture for the advisory test.

## Implementation Plan

1. **`agents-plugin-pi/package.json`** — add
   `"@earendil-works/pi-coding-agent": "^0.84.4"` under a new `dependencies`
   block (matches the machine-installed version); run `npm install` in
   `agents-plugin-pi/` so `RpcClient` resolves at runtime. Keep the `files`
   whitelist as-is (deps aren't shipped in a normal npm publish's `files`
   list the same way, but this is a `dependencies` entry so npm handles it
   as an ordinary transitive install — no `files` change needed).
2. **`agents-plugin-pi/src/model-catalog.ts`** — reframe:
   - `ModelCatalogConfig`: replace `tiers?: Partial<Record<ModelTier,
     string>>` with `aliases?: Record<string, string>` (keep `catalog?`
     unchanged). Decide whether `ModelTier`/`VALID_MODEL_TIERS` types are
     still needed anywhere (only `explore`'s fixed `"small"` lookup key
     needs a string constant now, not a closed union — can drop the type).
   - `resolveTierModel(config, tier)` -> `resolveAlias(config, name: string):
     string | undefined` returning `config?.aliases?.[name]`.
   - `isModelCatalogUnset(config)` -> `true` when `!config?.aliases ||
     Object.keys(config.aliases).length === 0`.
   - Update doc comments (they currently describe the tier map).
3. **`agents-plugin-pi/src/bridge.ts`** — update `MODEL_CATALOG_ADVISORY`
   wording (`#L83-90`) to describe curating the alias table instead of "the
   `small` tier"; no signature changes to `maybeAppendModelCatalogAdvisory`/
   `isModelCatalogUnset` call site (`#L236`) since the function contract is
   unchanged (`ModelCatalogConfig | undefined` in, bool out).
4. **`agents-plugin-pi/src/spawner.ts`** — the core rewrite. Keep unchanged:
   `TOOL_GROUPS`, `resolveTools`, `AgentEventLineBuffer`,
   `handleAgentEvent`, `isTerminalStopReason`, `buildSpawnArgs`,
   `spawnPiProcess`, `getPiInvocation`, `exploreLeaf` and its `explore` tool
   registration (still one-shot, `--tools=recon`, `--no-session`,
   self-reaping) — only its model resolution call switches from
   `resolveModelForTier(config, "small", inherit)` to
   `resolveModelForAlias(config, "small", inherit)`. Remove: `spawnAgent`,
   `continueAgent`, `waitAgents`, `ModelTier`-based `asModelTier`/
   `VALID_MODEL_TIERS`/`resolveModelForTier`, the `ws-agent-continue` tool
   registration. Add:
   - `AgentRecord` (new shape for RPC-backed agents, separate from the
     explore-only `AgentRecord` fields still used by `exploreLeaf`, or a
     shared type with optional fields — executor's call): `agentId`,
     `client?: RpcClient` (undefined when dormant), `sessionPath`,
     `systemPromptPath`, `modelBase?: string`, `modelEffort?: string`,
     `wsToolNames` (cached, for tools re-resolution on resume), `streaming:
     boolean`, `idlePending: boolean` (edge-consume flag), `waiters:
     Array<() => void>`, `lastText?: string`.
   - `spawnAgent(registry, ctx, { systemPromptPath, prompt, modelName?,
     modelEffort? })`: NO `playbook.render` call. Resolve
     `modelBase = modelName ? resolveModelForAlias(readModelCatalog(...),
     modelName, ctx.inheritModel) : ctx.inheritModel`. `mkdtempSync` a
     session dir + `session.jsonl` path (same as today). Build `new
     RpcClient({ cliPath: process.argv[1], cwd: ctx.cwd, model: modelBase,
     args: ["--session", sessionPath, "--append-system-prompt",
     systemPromptPath, "--tools", resolveTools("full-worker",
     ctx.wsToolNames)] })`. `await client.start()`. If `modelEffort`: `await
     client.setThinkingLevel(modelEffort as ThinkingLevel)` wrapped in
     try/catch (never-hard-fail — an unsupported level degrades to inert,
     matching the ticket's own fallback wording). Subscribe
     `client.onEvent(evt => { if (evt.type==="agent_start")
     record.streaming=true; if (evt.type==="agent_settled") {
     record.streaming=false; record.idlePending=true;
     settleWaiters(record); } })`. Fire `await client.prompt(prompt)`
     (do not await idle — return immediately per ticket's "returns
     immediately" contract). Register in registry, return `{ agent_id:
     agentId }`.
   - `sendToAgent(registry, ctx, agentId, message, interrupt?)`: look up
     record. If dormant (`!record.client`): rebuild a new `RpcClient` with
     the SAME cached `sessionPath`/`systemPromptPath`/`modelBase`/
     `wsToolNames` (args: `["--session", record.sessionPath,
     "--append-system-prompt", record.systemPromptPath, "--tools",
     resolveTools("full-worker", record.wsToolNames)]`), `start()`,
     re-subscribe events, then `await client.prompt(message)` (never
     steer/followUp on a just-started client — see Codebase Findings). If
     live and `record.streaming`: `interrupt ? client.steer(message) :
     client.followUp(message)`. If live and NOT streaming (idle): `await
     client.prompt(message)` regardless of `interrupt` (nothing to
     interrupt).
   - `waitForAgents(registry, agentIds, timeoutMs?)`: for each id, if
     `record.idlePending` is already true, consume it immediately
     (`idlePending=false`) and return that agent as the finisher; else
     race `Promise.race` over a per-agent one-shot waiter (pushed onto
     `record.waiters`, resolved by the `agent_settled` listener) plus an
     optional timeout promise (reuse the existing `waitAgents` timeout
     pattern, never kill on timeout). Attach `getLastAssistantText()`
     (await it for the winning agent only) as the "last message." Return a
     timed-out marker with no agent harvested when the timeout fires first,
     leaving all children registered (matches D-D's stated timeout
     behavior, which Phase 1's wait tool already needs for correctness).
   - `listAgents(registry)`: map registry entries to `{ agent_id, status:
     record.client ? (record.streaming ? "running" : "idle") : "dormant" }`.
   - `stopAgent(registry, agentId)`: `record.client` — best-effort `await
     client.abort().catch(()=>{})` then `await client.stop()`; unsubscribe
     the event listener; set `record.client = undefined` (record stays in
     the registry — dormant/resumable, per D-C). Error if the id is unknown.
   - `registerAgentTools`: register `ws-agent-spawn`, `ws-agent-send`,
     `ws-agent-wait`, `ws-agent-list`, `ws-agent-stop` (drop
     `ws-agent-continue`); keep `explore`'s existing registration. Return an
     `AgentToolsHandle` whose teardown method is `async` (e.g.
     `stopAll(): Promise<void>` awaiting `Promise.allSettled(...)` over
     every registry entry's `client.stop()` best-effort) — rename from
     `killRunning` since the shutdown semantics changed from SIGTERM to a
     graceful RPC stop; keep `explore`'s in-flight one-shot processes killed
     the old way (`proc.kill()`) since that machinery is untouched.
5. **`agents-plugin-pi/src/index.ts`** — `pi.on("session_shutdown", () =>
   {...})` (`#L123-130`) becomes `async (_event, _ctx) => { await
   agentTools?.stopAll(); ...; }` to actually wait for graceful RPC child
   teardown before the bridge connection closes, matching the doc comment's
   ordering requirement ("kill spawned children before tearing down the
   bridge connection they dispatch through").
6. **Tests** — update/add in `agents-plugin-pi/test/`:
   - `model-catalog.test.ts`: rewrite all `tiers`-shaped fixtures to
     `aliases`-shaped; rename `resolveTierModel` tests to `resolveAlias`.
   - `bridge.test.ts` (`#L166-180`): update the `ModelCatalogConfig` fixture
     to `{ aliases: { small: "..." } }` (or whatever the trigger key ends up
     being) so `maybeAppendModelCatalogAdvisory`'s unset-detection test
     still exercises the real trigger condition.
   - `spawner.test.ts`: remove `resolveModelForTier`/`asModelTier` describe
     blocks (`#L287-335`); add unit coverage for the new pure/seam-extractable
     pieces — `resolveModelForAlias`, the idle-edge-consume waiter logic
     (can be tested by driving a fake record + synthetic `agent_settled`-like
     calls without a real subprocess, mirroring how `handleAgentEvent` is
     tested today via direct event-object injection), and `listAgents`'s
     status-mapping. RpcClient-backed spawn/send/wait/stop themselves are
     only verifiable live (per the ticket's verification boundary) — do not
     attempt to mock `RpcClient` for a full integration test in this phase.

## Verification Plan

- `cd agents-plugin-pi && npm install && node --test test/` — unit suite
  green after the model-catalog/spawner/bridge test rewrites.
- Live gate (per ticket's stated verification boundary, matches the
  precedent of prior phases' live `pi -e … --mode json -p` / `--mode rpc`
  transcripts): spawn one worker via `ws-agent-spawn`, send a follow-up via
  `ws-agent-send` while it is idle (confirms the `prompt()`-not-`followUp()`
  branch actually delivers), `ws-agent-stop` it, `ws-agent-send` again on
  the now-dormant id (confirms auto-resume via the same cached
  `system_prompt_path`/`session` file), `ws-agent-wait` over two live
  children and confirm first-finisher + last message, `ws-agent-list`
  reflecting live/dormant status, a worker spawning an `explore` leaf but
  failing to spawn another worker (its `--tools` allowlist excludes
  `ws-agent-*`), and children torn down cleanly on `session_shutdown`.
- No automated test can exercise a real `pi --mode rpc` child process in
  this repo's `node --test` suite (no live model/API key in CI); the live
  gate above is manual/local, consistent with how `/ws-discuss` and the
  Phase 2-3 gates were verified.

## Escalations

- None. Confidence is high: every open technical question in the ticket
  (`cliPath` resolution, whether a reasoning-effort launch flag exists, how
  `followUp`/`steer` behave against an idle child) was resolved by reading
  the actual installed `@earendil-works/pi-coding-agent@0.84.4` source and
  docs rather than assumption, and D-A–D-D already settle the remaining
  design questions. The one judgment call left open (`explore`'s fixed
  alias-lookup key staying `"small"` under the reframed table) is a
  same-behavior, low-risk default documented above, not a scope decision —
  the executor can proceed with it and adjust trivially if reviewed
  otherwise.
