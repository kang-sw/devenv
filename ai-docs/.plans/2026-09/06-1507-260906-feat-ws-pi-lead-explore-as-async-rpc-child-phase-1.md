# Plan: 260906-feat-ws-pi-lead-explore-as-async-rpc-child — Phase 1: Role-keyed explore registration

## Relevant Ticket Contract

- Adapter-only (`agents-plugin-pi/`); no ws-mcp change. Reuse the `explore`
  playbook, the `small` alias resolution, and the `recon` tool group as-is.
- Lead and fork: `explore` becomes a preset over `spawnAgent` — same tool
  name and `query` param, no `async` param — spawning with `toolGroup:
  "recon"` explicit, the `small` alias (or inherited model when unset, as
  today), an auto alias `explore-N`, a title derived from the query, and
  returning `{agent_id, alias}` immediately.
- Answer delivery is the settle push's `last_message`
  (`harvestLastMessage`) — the explore child has no report tool.
- One-shot record: `RpcAgentRecord` gains `oneShot: true` for the preset.
  `sendToAgent` refuses a one-shot record (naming it as an explore);
  `ws-agent-stop`/`ws-agent-transcript` stay allowed. On settle, the settle
  IIFE in `attachEventListener` runs the existing silent `stopAgent` and
  THEN deletes the registry entry itself (deletion lives in the IIFE, not in
  `stopAgent` — its D-C "never delete here" invariant is unchanged). An
  owner-cancelled explore (`ws-agent-stop` on a one-shot) is deleted by the
  stop tool's own body right after `stopAgent` returns, same reason/layer.
  One-shot records are excluded from the shutdown sidecar snapshot.
- Live-agent widget: a lead explore renders a live row (`explore`) while
  running, gone at settle. `SpawnAgentRole` and the widget's
  `AgentRowRole`/`roleFromSpawnRole` both gain `explore`; `buildRpcClientOptions`
  takes the child-process role from a new override fed by `spawnAgent`
  (instead of always hardcoding `fork`/`worker`); the preset sets
  `record.spawnRole = "explore"`.
- Worker/execute-worker keep the current blocking `exploreLeaf` under the
  same `explore` name, minus the `async` param (its only consumer was the
  lead). An explore child itself still gets no `explore` tool (`recon`
  group), so the depth cap is unchanged.
- `registerAgentTools` registers the lead preset when `readSpawnRole` says
  lead or fork, and the blocking leaf otherwise; role is known from the
  process environment at factory time. The tool description text differs by
  shape. `pi-lead-guide.md`'s dispatch row for `explore` changes from
  "answer" to "id now, answer on the settle push".
- Fallout absorbed here: the goal-loop yield gate and fan-in line already
  count a lead explore automatically (same registry) — no code change
  needed there, just verification.
- Phase 1's test list (ticket, verbatim scope): lead process registers the
  preset and not the leaf, worker process the reverse, fork the preset; the
  child env and the record both carry `explore` and `roleFromSpawnRole` maps
  it to the `explore` row; the preset returns an id and alias and its child
  is counted by the fan-in line and the goal-loop gate; the child's answer
  arrives as the settle push's `last_message` and the record is gone after
  settle; the widget row reads `explore` while live and is absent after
  settle; `ws-agent-send` to an explore id is refused while stop and
  transcript work, and a stopped one-shot is gone from the registry; a
  one-shot record is not in the sidecar snapshot; the worker leaf still
  blocks and self-reaps. Amend the spec passages under Spec Impact. (The
  owner-run live check is manual, out of scope for this executor plan.)

## Out of Scope

- ws-mcp / `agents-plugin-tool/` / `agents-plugin/skills/` — untouched
  (adapter-only constraint).
- The `explore` playbook, `small` alias resolution mechanism, and `recon`
  tool-group contents — reused unchanged.
- The worker/execute-worker `exploreLeaf` blocking behavior and its
  self-reap mechanics — unchanged except dropping the `async` param.
- `computeForkToolSurface` (`fork.ts`) — already passes `explore` through
  unchanged; no edit needed there.
- The related tickets' own carve-outs:
  `260906-bug-ws-pi-goal-reminder-races-child-push-at-settle`'s
  async-explore carve-out lives only in that ticket's Phase 1 body
  (L138-141, L213), which already carries a `### Result` and is therefore
  immutable; `ai-docs/spec/pi-adapter-runtime.md`'s goal-loop passage
  carries no explore exclusion (grepped, no hits). Nothing to delete; the
  lead-explore spec amendment (anchor `{#260903-pi-explore-recon-leaf}`)
  should state that a lead explore counts for the goal-loop yield gate and
  the fan-in line like any other RPC child.
  `260906-feat-ws-pi-tool-result-yaml-tui-rendering` Phase 2's
  `onModelResolved` wiring — not touched here; whichever ticket lands second
  reconciles it.
- The owner-run live-check ("lead calls explore, sees the id at once...") —
  manual verification, not an automated step.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L657-708` — `exploreLeaf`: the current
  blocking one-shot leaf (renders `explore` playbook, `--tools=recon`,
  `--no-session`, `params.async` returns early with a running entry). Keep
  this function for the worker/execute-worker path; only drop `async` from
  `ExploreParams` (`#L628-636`) and its early-return branch (`#L701-703`).
- `agents-plugin-pi/src/spawner.ts#L448-450` — `resolveSpawnToolGroup`:
  pure `explicit ?? "full-worker"` default; the preset must pass
  `toolGroup: "recon"` explicitly through `RpcSpawnCtx.toolGroup` so this
  default is never relied on for explore.
- `agents-plugin-pi/src/spawner.ts#L1020` — `SpawnAgentRole = "worker" |
  "execute-worker" | "fork"`. Add `"explore"` here (single source of truth
  read by `RpcAgentRecord.spawnRole`, `RpcSpawnCtx.spawnRole`, and
  `agent-widget.ts`'s `roleFromSpawnRole`).
- `agents-plugin-pi/src/spawner.ts#L728-973` — `RpcAgentRecord`. Add
  `oneShot?: boolean` (mirror the doc-comment density of neighboring fields,
  e.g. near `spawnRole` at `#L776-782`): `true` only for the explore preset;
  drives the `sendToAgent` refusal, the settle-IIFE deletion, the
  `ws-agent-stop` tool-body deletion, and the sidecar exclusion.
- `agents-plugin-pi/src/spawner.ts#L1703-1770` — `RpcSpawnCtx`. Already
  carries `spawnRole?: SpawnAgentRole` (used for `record.spawnRole`, doc'd
  `#L1764-1769`) and `toolGroup?: ToolGroup` (`#L1719`) — the preset reuses
  both unchanged. Add `oneShot?: boolean`, threaded into the record
  construction the same way `spawnRole`/`toolGroup` already are.
- `agents-plugin-pi/src/spawner.ts#L1827-1853` — `buildRpcClientOptions`:
  hardcodes `[WS_PI_SPAWN_ROLE_ENV]: forkFrom ? "fork" : "worker"`. Needs a
  new optional trailing param (e.g. `spawnRoleOverride?: SpawnRole` from
  `process-role.ts`, which ALREADY includes `"explore"` in its 3-value enum
  at `process-role.ts#L27` — no change needed there) so the env can read
  `"explore"`. `SpawnAgentRole` (4 values, includes `"execute-worker"`)
  and `SpawnRole` (3 values, no `"execute-worker"`) are different enums —
  do not conflate them; compute the override at the call site
  (`spawnAgent`) as `ctx.spawnRole === "explore" ? "explore" : undefined`,
  leaving the existing `forkFrom ? "fork" : "worker"` fallback untouched for
  every other role (including `execute-worker`, which has no distinct env
  value today and must keep getting `"worker"`).
- `agents-plugin-pi/src/spawner.ts#L2316-2402` — `spawnAgent`: the single
  call site (`#L2361-2363`) that builds `RpcClientOptions` — thread the new
  override through here. `record.spawnRole` fold at `#L2353` already reads
  `ctx.spawnRole ?? (...)`; the preset supplies `ctx.spawnRole = "explore"`
  so this line needs no change, only the new `oneShot` field added to the
  record literal (`#L2337-2358`).
- `agents-plugin-pi/src/spawner.ts#L2437-2523` — `sendToAgent`: add the
  one-shot refusal right after the `if (!record) throw` guard
  (`#L2449-2451`), before the `ctx.leadSend` branch — throw naming the
  record as an explore (matches the existing "unknown agentId" throw-style
  convention at `#L2450`).
- `agents-plugin-pi/src/spawner.ts#L2093-2149` (`attachEventListener`) and
  specifically the settle IIFE at `#L2111-2134` and the automatic-park guard
  at `#L2122-2128` (`if (registry && !record.threadBound && !record.running)
  { await stopAgent(..., {silent:true}) }`): add `if (record.oneShot)
  registry.delete(record.agentId);` immediately after the `stopAgent` call,
  inside the same `if` block — the ticket's "runs the existing silent
  stopAgent and then deletes the registry entry itself, after the push"
  (the push already happened earlier in the same IIFE, `#L2114-2117`).
- `agents-plugin-pi/src/spawner.ts#L2614-2670` — `stopAgent`: NO change
  here (D-C invariant, "the registry entry is NEVER deleted here", stays
  literally true). The deletion for an owner-cancelled one-shot belongs in
  the `ws-agent-stop` TOOL's `execute()` body instead (see below).
- `agents-plugin-pi/src/spawner.ts#L2900-2917` — the `ws-agent-stop` tool
  registration: after `const result = await stopAgent(rpcRegistry,
  p.agent_id, pi);`, resolve the record via `resolveAgentId` (already used
  identically inside `sendToAgent`/`stopAgent`, `#L2447`/`#L2622`) and
  `rpcRegistry.delete(resolvedId)` when `record?.oneShot`.
- `agents-plugin-pi/src/spawner.ts#L2531-2542` — `harvestLastMessage`: reused
  verbatim by the settle push for the explore preset's answer; no change.
- `agents-plugin-pi/src/spawner.ts#L1151-1152` (doc comment inside
  `computeRunningStatusLine`) — states "An `explore` leaf is never in this
  registry at all (it has its own)." This becomes stale for the LEAD path
  once the preset lands in the same `rpcRegistry` (still true for the
  worker/execute-worker leaf's separate `exploreRegistry`). Update the
  comment on contact; `computeFanIn`/`hasRunningAgents`
  (`#L1132-1141`, `#L1182-1184`) themselves need no logic change — they
  already walk every non-`threadBound` `RpcAgentRegistry` member generically.
- `agents-plugin-pi/src/spawner.ts#L2748-3019` (`registerAgentTools`) — the
  registration function. It already imports `readSpawnRole` from
  `process-role.ts` (`#L84`); call `readSpawnRole(process.env)` /
  `isLeadOrFork(...)` inside this function (role is fixed for the process's
  whole lifetime, known at factory time — no plumbing needed from
  `index.ts`, whose one call site at `index.ts#L355` needs no signature
  change). Branch the `explore` tool registration
  (currently `#L2965-2994`, one unconditional block using `exploreLeaf` +
  `exploreRegistry`) into two variants: lead/fork gets the new preset (calls
  `spawnAgent` on `rpcRegistry`), worker/execute-worker (and, harmlessly,
  an explore-role process, which can never reach this tool anyway per the
  depth cap) keeps today's `exploreLeaf` call unchanged except dropping
  `async` from `ExploreParams`.
- `agents-plugin-pi/src/spawner.ts#L2775-2778` — `resolveExploreModel`
  (the existing IO wrapper for the leaf's implicit `"small"` lookup via
  `resolveModelForAliasViaWsMcp`). The lead preset should NOT reuse this
  helper directly; instead pass `modelName: "small"` in the `spawnAgent`
  params so `spawnAgent`'s own internal `resolveModelForAliasViaWsMcp`
  call (`#L2333`, already falling back to `ctx.inheritModel` exactly like
  `resolveExploreModel` does) resolves it — this is the ticket's own
  ordering note ("if this ticket lands first, the lead explore path
  resolves through `spawnAgent`"). `resolveExploreModel` stays as-is for the
  leaf's `exploreLeaf` call only.
- `agents-plugin-pi/src/spawner.ts#L663-673` — the `playbook.render("explore")`
  call and `systemPromptPath` extraction inside `exploreLeaf`. The lead
  preset needs the identical render step (same `client.callTool
  ("playbook.render", {session_key, name: "explore"})` shape) before calling
  `spawnAgent` — `spawnAgent` never renders playbooks itself (D-A, doc'd
  `#L2275-2278`). Extracting a small shared render helper is reasonable but
  not required; duplicating the ~10-line block is also acceptable given the
  two call sites now differ in what they do with the result.
- `agents-plugin-pi/src/spawner.ts#L2695-2713` (`AgentToolsHandle`) — no
  change; `rpcRegistry` is already the shared map both paths need.
- `agents-plugin-pi/src/process-role.ts#L27-50` — `SpawnRole`/`readSpawnRole`/
  `isLeadOrFork`. `SpawnRole` already includes `"explore"` (used today only
  by the worker-leaf's own child marker via `buildChildProcessEnv`,
  `spawner.ts#L137-139`). No change needed in this file; the new lead-side
  explore child reuses the same literal value through the new
  `buildRpcClientOptions` override.
- `agents-plugin-pi/src/agent-widget.ts#L56-57` — `AgentRowRole = "worker" |
  "execute" | "fork" | "thread"`. Add `"explore"`.
- `agents-plugin-pi/src/agent-widget.ts#L86-91` — `roleFromSpawnRole`: add an
  `if (spawnRole === "explore") return "explore";` branch (mirrors the
  existing `execute-worker`/`fork` branches; unset still falls back to
  `"worker"`, unchanged).
- `agents-plugin-pi/src/agent-widget.ts#L157-201` (`buildAgentRows`) — no
  change needed: row inclusion is already `record.threadBound ||
  record.pendingApproval !== undefined || record.client !== undefined`,
  which a live explore record satisfies via `client !== undefined` exactly
  like a worker; it disappears once the settle IIFE deletes it from the
  registry (`spawner.ts`).
- `agents-plugin-pi/src/agent-sidecar.ts#L109-134` — `captureOrphans`: add
  `record.oneShot` to the skip guard at `#L112` (`if (record.threadBound)
  continue;` -> `if (record.threadBound || record.oneShot) continue;`).
  Called from `index.ts#L516` before `stopAll()` — ordering unaffected.
- `agents-plugin-pi/src/index.ts#L355` — the sole `registerAgentTools(...)`
  call site; no signature change, since the role read moves inside
  `registerAgentTools` itself.
- `agents-plugin-pi/pi-lead-guide.md#L40` — dispatch row: `| Answer one
  scoped, read-only exploration question | `explore` |` — change wording to
  reflect "id now, answer on the settle push" per the ticket's own phrasing.
- `ai-docs/spec/pi-adapter-runtime.md` anchors to amend (Spec Impact):
  - `{#260903-pi-explore-recon-leaf}` (`#L440-452`) — split into the lead
    preset (RPC child, immediate id, settle-push answer, one-shot removal)
    and the worker leaf (blocking, self-reaping), keyed on role. The current
    text's "`explore` is not a registry member... outside the pushed status
    line's running count" is FALSE for the lead path post-change.
  - `{#260904-pi-spawner-bounded-depth-explore-leaf}` (`#L469-481`) —
    restate the depth cap for all three shapes (lead → explore child,
    worker → blocking leaf, lead → fork → explore child).
  - `{#260903-pi-spawner-tool-groups}` (`#L454-467`) and
    `{#260903-pi-delegation-spawner-tools}` (`#L285-...`, the `ws-agent-send`
    bullet around `#L319-324`) — the `explore` name in worker groups is the
    leaf; the lead's `explore` is the preset; note `ws-agent-send` refuses a
    one-shot record.
  - `{#260905-pi-live-agent-widget}` (`#L1076-1094`) — replace "there is no
    idle, dormant, or explore row, since... a one-shot explore never enters
    the registry" (`#L1082-1084`) with the live `explore` row that
    disappears at settle; add `explore` to the `role` enum list (`#L1080`).
  - `{#260903-pi-spawner-model-tier-inherit}` (`#L483-503`) — only the
    sentence describing explore's effort surface, split by shape (lead
    resolves through `spawnAgent`'s `effectiveModelEffort`/
    `setThinkingLevel`; the worker leaf still has no effort surface).
- Test fakes / patterns to reuse:
  - `agents-plugin-pi/test/execute-gateway.test.ts#L543-557`
    (`registerAndCapture`) — the pattern for testing a `registerX(pi, ...)`
    function: a fake `pi = { registerTool: (def) => registered.set(def.name,
    def) }`, then invoke the captured tool's `execute()` directly. No
    existing test calls `registerAgentTools` directly (`grep` confirms zero
    hits outside `index.ts`/its own definition) — the new
    `describe("registerAgentTools ...")` block will need to build this fake
    from scratch, plus a fake `bridge: BridgeHandle` (`{ client, wsToolNames:
    [], defaultSessionKeyRef: { current: "k" } }`) and a fake `sessionCtx:
    {cwd}`. Toggle role by setting/restoring `process.env[WS_PI_SPAWN_ROLE_ENV]`
    around the call, mirroring `test/spawner.test.ts#L1201-1207` /
    `#L1471-1488`'s existing save/restore pattern.
  - `agents-plugin-pi/test/spawner.test.ts#L2458-2515`
    (`describe("buildRpcClientOptions ...")`) — add cases for the new
    override param here.
  - `agents-plugin-pi/test/spawner.test.ts#L1518-...`
    (`describe("attachEventListener ...")`) — add the one-shot
    deletion-after-settle case here; the block already drives
    `attachEventListener` with a duck-typed fake `client` (per that
    function's own doc comment, `spawner.ts#L2085-2091`).
  - `agents-plugin-pi/test/spawner.test.ts#L2237-...`
    (`describe("sendToAgent (live branches only...)`) — add the refusal
    case for `record.oneShot === true` here.
  - `agents-plugin-pi/test/spawner.test.ts#L1968-...`
    (`describe("stopAgent (260905 push + silent)")`) — `stopAgent` itself is
    untouched, so no new case belongs here; the deletion-on-cancel case
    belongs in the new `registerAgentTools`/`ws-agent-stop`-tool-body test
    instead (see above).
  - `agents-plugin-pi/test/agent-widget.test.ts#L68-...` (the
    `roleFromSpawnRole` test) — add an `explore -> explore` case alongside
    the existing worker/execute/fork ones.
  - `agents-plugin-pi/test/agent-sidecar.test.ts#L76-...`
    (`describe("captureOrphans")`) — add a case asserting a `oneShot: true`
    record is excluded from the returned array.

## Implementation Plan

1. `agents-plugin-pi/src/process-role.ts` — no code change; confirmed
   `SpawnRole` already carries `"explore"` (`#L27`).
2. `agents-plugin-pi/src/spawner.ts`:
   a. `SpawnAgentRole` (`#L1020`): add `"explore"`.
   b. `RpcAgentRecord` (`#L728-973`): add `oneShot?: boolean` field with a
      short doc comment (one-shot preset marker; drives refusal/deletion/
      sidecar-exclusion).
   c. `RpcSpawnCtx` (`#L1703-1770`): add `oneShot?: boolean`, doc'd as
      mirroring `spawnRole`'s "recorded on the record" convention.
   d. `buildRpcClientOptions` (`#L1827-1853`): add a trailing
      `spawnRoleOverride?: SpawnRole` param (import `type SpawnRole` from
      `./process-role.ts` alongside the existing named imports at `#L84`);
      env line becomes `[WS_PI_SPAWN_ROLE_ENV]: spawnRoleOverride ??
      (forkFrom ? "fork" : "worker")`.
   e. `ExploreParams` (`#L628-636`): drop `async?: boolean`.
   f. `exploreLeaf` (`#L657-708`): drop the `params.async` early-return
      branch (`#L701-703`); the leaf always blocks now (worker/execute-worker
      path only).
   g. `spawnAgent` (`#L2316-2402`): thread `oneShot: ctx.oneShot === true`
      into the record literal (`#L2337-2358`); thread the new
      `spawnRoleOverride` into the `buildRpcClientOptions` call
      (`#L2361-2363`) as `ctx.spawnRole === "explore" ? "explore" :
      undefined`.
   h. `sendToAgent` (`#L2437-2523`): right after the `if (!record) throw`
      guard, add:
      ```ts
      if (record.oneShot) {
        throw new Error(`ws-pi-agent: ws-agent-send refused: agent ${resolvedId} is a one-shot explore — read its answer from the settle push or ws-agent-transcript`);
      }
      ```
   i. `attachEventListener`'s settle IIFE automatic-park block
      (`#L2122-2128`): after the `await stopAgent(...)` try/catch, add
      `if (record.oneShot) registry.delete(record.agentId);` inside the same
      `if (registry && !record.threadBound && !record.running)` guard.
   j. `registerAgentTools` (`#L2748-3019`): at the top of the function body
      (near the existing `resolveExploreModel` helper, `#L2775-2778`),
      compute `const isLeadRole = isLeadOrFork(readSpawnRole(process.env));`
      (both already imported at `#L84`). Add a small counter/helper for the
      auto alias, e.g. a closure-scoped `let exploreCounter = 0;` incremented
      per lead-preset spawn, producing `explore-${++exploreCounter}`
      (simplest correct approach; a registry-scan-based alternative is also
      acceptable but not required). Add a short title-from-query helper
      (head-truncate the query, e.g. to ~60 chars, matching the truncation
      style already used elsewhere, e.g. `truncateToWidth` in
      `agent-widget.ts` or `truncatePromptForStorage` in this file — a new
      tiny local truncation, not a shared import, is fine here since neither
      existing helper's signature fits directly).
      Replace the current unconditional `explore` tool block
      (`#L2965-2994`) with a role-branched registration:
      - `parameters`: `{ query: string }` only (no `async`) in both
        branches, since the leaf also drops `async` per step (f).
      - `description`: keep close to today's leaf wording for
        worker/execute-worker; for lead/fork use wording matching the
        ticket's "returns `{agent_id, alias}` immediately... arrives later
        on the settle push" framing (mirror `ws-agent-spawn`'s own
        "do not wait for it" phrasing at `#L2784`).
      - `execute()`, lead/fork branch: render the `explore` playbook (same
        `client.callTool("playbook.render", {session_key:
        bridge.defaultSessionKeyRef.current ?? "", name: "explore"})` +
        `firstText`/`systemPromptPath` extraction shape as `exploreLeaf`
        `#L663-673`, throwing the same way on failure), then call
        `spawnAgent(rpcRegistry, { pi, cwd: sessionCtx.cwd, inheritModel:
        inheritModelFromToolCtx(toolCtx), wsToolNames: bridge.wsToolNames,
        client: bridge.client, toolGroup: "recon", spawnRole: "explore",
        oneShot: true, onApprovalPending }, { systemPromptPath, prompt:
        p.query, modelName: "small", alias: autoExploreAlias(), title:
        deriveExploreTitle(p.query) })`, then return `{ content: [{ type:
        "text", text: JSON.stringify({ agent_id: result.agent_id, alias:
        result.alias }) }] }` (do not surface `evicted` — not part of the
        ticket's stated return contract `{agent_id, alias}`, though
        returning it too would not break anything if simpler to keep
        uniform with `ws-agent-spawn`'s own return shape; either is
        acceptable, prefer the literal `{agent_id, alias}` per the ticket
        text).
      - `execute()`, worker/execute-worker branch: unchanged call into
        `exploreLeaf(bridge.client, exploreRegistry, {...}, p)` (same as
        today, `#L2983-2992`, minus the dropped `async` field).
   k. Fix the stale doc comment at `#L1151-1152` inside
      `computeRunningStatusLine` (or move the caveat to only describe the
      worker-leaf's separate `exploreRegistry`).
3. `agents-plugin-pi/src/agent-widget.ts`:
   a. `AgentRowRole` (`#L57`): add `"explore"`.
   b. `roleFromSpawnRole` (`#L86-91`): add `if (spawnRole === "explore")
      return "explore";` before the final `worker` fallback.
4. `agents-plugin-pi/src/agent-sidecar.ts`:
   a. `captureOrphans` (`#L109-134`): change the skip guard at `#L112` to
      `if (record.threadBound || record.oneShot) continue;`.
5. `agents-plugin-pi/pi-lead-guide.md#L40`: reword the dispatch row's answer
   column from "Answer one scoped, read-only exploration question" /
   `explore` to reflect "returns an id immediately; the answer arrives later
   on the settle push", per the ticket's own "id now, answer on the settle
   push" phrasing.
6. `ai-docs/spec/pi-adapter-runtime.md`: amend the six anchors listed in
   Codebase Findings' Spec Impact section (`{#260903-pi-explore-recon-leaf}`,
   `{#260904-pi-spawner-bounded-depth-explore-leaf}`,
   `{#260903-pi-spawner-tool-groups}`, `{#260903-pi-delegation-spawner-tools}`,
   `{#260905-pi-live-agent-widget}`, `{#260903-pi-spawner-model-tier-inherit}`).

## Verification Plan

- `cd agents-plugin-pi && npm test` — full suite must stay green (node:test).
- Focused new/updated test coverage (add to the `describe` blocks named in
  Codebase Findings' "Test fakes / patterns to reuse" section):
  - New `describe("registerAgentTools (role-keyed explore registration)")`
    in `test/spawner.test.ts`: with `WS_PI_SPAWN_ROLE_ENV` unset (lead) and
    set to `"fork"`, the registered `explore` tool's `execute()` calls
    `spawnAgent` (assert via a fake `bridge.client`/a spy, or assert the
    returned shape is `{agent_id, alias}` with no `output`/`state` fields
    the leaf would return) and NOT `exploreLeaf`; with it set to `"worker"`
    and `"execute-worker"`, the reverse (leaf behavior: blocks, returns
    `{agent_id, state, output?, stopReason?}`).
  - `buildRpcClientOptions`: new cases asserting `spawnRoleOverride:
    "explore"` sets `WS_PI_SPAWN_ROLE_ENV` to `"explore"`, and that omitting
    it preserves today's `forkFrom ? "fork" : "worker"` behavior unchanged
    (regression guard on the existing `#L2458-2515` cases).
  - `roleFromSpawnRole`: `spawnRole: "explore"` maps to `role: "explore"`.
  - `attachEventListener` settle path: a `oneShot: true` record is deleted
    from the registry after settle (and after its push), while a non-oneShot
    record is only parked (stays in the registry, dormant).
  - `sendToAgent`: a `oneShot: true` record throws on send with a message
    naming it as an explore; `stopAgent`/`getAgentTranscriptPath` against
    the same record still succeed (no new guard added to either).
  - `ws-agent-stop` tool body (via the same `registerAgentTools` fake-`pi`
    harness): stopping a `oneShot: true` agent removes it from the registry
    entirely (not just parks it dormant).
  - `captureOrphans`: a `oneShot: true` record is excluded from the
    returned array; a non-oneShot record is unaffected (existing behavior).
  - Fan-in / goal-loop gate: confirm (no new production code needed) via a
    `hasRunningAgents`/`computeRunningStatusLine` test using a registry
    entry with `spawnRole: "explore"` and `running: true` — should count
    exactly like any other non-`threadBound` record. Consider one assertion
    in `test/goal-loop.test.ts` (or reuse an existing `hasRunningAgents`
    case in `spawner.test.ts#L1043-...`) confirming an explore-role record
    yields `yielding: true` the same way a worker does.
  - Worker leaf regression: `test/spawner.test.ts` has no existing case that
    calls `exploreLeaf` directly or exercises `params.async`/`async: true`
    (confirmed via grep — `exploreLeaf` is live-gate only per the module's
    own doc comment, `spawner.ts#L44`), so dropping `async` from
    `ExploreParams` needs no test deletion; just confirm `npm test` stays
    green and the `explore` tool's registered `parameters` schema (both
    branches) no longer lists `async`.
- No manual verification is required for Phase 1 beyond the owner's own
  live check named in the ticket (out of scope for this plan/executor).

## Escalations

- None.
