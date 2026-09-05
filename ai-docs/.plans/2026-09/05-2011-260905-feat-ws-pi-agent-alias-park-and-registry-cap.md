# Plan: 260905-feat-ws-pi-agent-alias-park-and-registry-cap — Phase 1: Alias, title, park, cap

## Relevant Ticket Contract

- `alias`/`title` are optional `ws-agent-spawn` params, persisted on
  `RpcAgentRecord`; adapter never derives either from the prompt.
- Every `agent_id` param (`ws-agent-send`, `ws-agent-stop`,
  `ws-agent-transcript`, `ws-approve`) resolves alias-or-uuid through **one
  helper**; spawn returns both `agent_id` and `alias`.
- Alias reuse **overwrites**: a dormant/idle holder's alias is cleared
  (title stays) and the new spawn takes it; a `running` or `threadBound`
  holder makes the spawn **fail** (rejection, not silent skip). No
  `ws-agent-close`; no auto-generated alias/title.
- Automatic park: **last step of settle handling**, after
  `flushPendingFinal` and the (separately-registered) advisory/nudge
  judgment; parks (silent stop) iff `!record.threadBound && !record.running`
  at that point. No grace period, no extra push for the park itself.
  `threadBound` records are never parked; a record the nudge re-prompted
  (`record.running` true again) is not parked. Resume seam stays
  `sendToAgent`'s dormant branch.
- `computeRunningStatusLine` presence keys on **any non-threadBound registry
  member** (dormant included), not on live client — N (the running count)
  is unchanged. `hasRunningAgents` must stay derived from the same walk.
- Sidecar (`agent-sidecar.ts`): drop the `!record.client` skip in
  `captureOrphans` (keep the `threadBound` skip); persist `alias`, `title`,
  `prompt`; `reviveOrphans`/`parseOrphans` restore/tolerate them; old-shape
  entries (no alias/title/prompt, only live records) still revive.
  Roll-call (`buildOrphanSummary`) uses alias/title.
- Registry capped at 256 (env `WS_PI_AGENT_REGISTRY_CAP` overrides, read by
  the lead process at spawn time). Over cap: evict dormant entries by
  oldest last-activity (last send or last report) until the new spawn fits;
  running/threadBound records are never evicted; a spawn that cannot fit
  without evicting one of those fails with an error. Spawn result carries
  one line naming what was evicted (alias or uuid). Eviction only forgets
  the registry entry, not the session file. Sidecar is written under the
  same cap (automatic, since the registry itself never exceeds it).
- `ws-agent-list` gains `alias`, `title`, opt-in `include_prompt` (off by
  default). Stored prompt is head-truncated to 4 KB at spawn (marker line
  notes the cut) — bounds the record, the sidecar, and the `include_prompt`
  reply alike.
- `ws-agent-stop` keeps its meaning (explicit park, `reason:"stopped"`
  pushed); the automatic park is silent (no push at all).
- No ws-mcp changes: `agents-plugin-tool/` and `agents-plugin/skills/` stay
  untouched (already respected — nothing in this phase touches them).

## Out of Scope

- Phase 2 / the live-agent-widget ticket (`260905-feat-ws-pi-live-agent-widget`) —
  rendering the always-visible widget with alias/title is a separate ticket.
- The sibling ticket `260905-feat-ws-pi-push-only-child-reports` is already
  `.done/` — its `hasRunningAgents`/goal-loop yield wiring
  (`src/goal-loop.ts:79,405`) is landed and must be left byte-compatible:
  it consumes `hasRunningAgents`, which shares `computeFanIn` with
  `computeRunningStatusLine`, so the one-line presence fix keeps both
  consistent without touching `goal-loop.ts`.
- Validating `model_effort`/thinking-level strings, `--fork` live-verification
  nuances, and anything under `ws-fork`'s own spawn path beyond what alias/
  title/park/cap generically apply to it (fork wiring order is read-only
  evidence here, not touched).
- A separate `ws-agent-close` tool (explicitly rejected by the ticket).
- Spec anchor prose changes beyond the three named in Spec Impact
  (`{#260903-pi-spawner-completion-gating}`, `{#260904-pi-report-to-lead-channel}`,
  `{#260903-pi-delegation-spawner-tools}`).

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L944-953` — `computeFanIn`, the single
  shared walk behind both `computeRunningStatusLine` and `hasRunningAgents`:
  `if (record.threadBound || !record.client) continue;`. The presence-rule
  change is a **one-line fix**: drop the `|| !record.client` half of the
  skip (keep the `threadBound` skip). Because both status-line and
  goal-loop-yield consumers already share this function, the fix cannot
  make them drift — confirmed no other call site duplicates this walk
  (`grep` for `hasRunningAgents`/`computeRunningStatusLine` shows only
  `src/goal-loop.ts:79,405` and `src/spawner.ts` itself).
- `agents-plugin-pi/src/spawner.ts#L664-857` — `RpcAgentRecord`. Needs three
  new optional fields (`alias?: string`, `title?: string`, `prompt?: string`
  — the head-truncated stored copy). No existing field name collides;
  `resolveModelForAlias`/`resolveAlias` (L342-366) use "alias" to mean a
  *model-catalog* alias — a different concept from the new agent `alias`.
  Not a bug, but worth a clear doc comment on the new field so readers don't
  conflate the two "alias" concepts already coexisting in this file.
- `agents-plugin-pi/src/spawner.ts#L1338-1343` (`SpawnAgentParams`) and
  `#L1790-1849` (`spawnAgent`) — spawn params/record construction. Alias
  overwrite-or-reject and the cap-eviction check both belong as guard
  clauses at the top of `spawnAgent`, before `mkdtempSync`/`randomUUID`
  side effects and before `registry.set(agentId, record)` — a rejected
  spawn (running/threadBound alias holder, or cap cannot be satisfied
  without evicting a protected record) must leave no trace (no temp dir,
  no half-registered record), mirroring the existing
  register-before-`start()`-then-`pushSpawnFailed`-on-throw pattern already
  used for launch failures (L1826-1846) but applied one step earlier.
- `agents-plugin-pi/src/spawner.ts#L1884-1966` (`sendToAgent`),
  `#L2028-2067` (`stopAgent`), `#L2077-2083` (`getAgentTranscriptPath`) —
  each does `registry.get(agentId)` directly and throws
  `` `ws-pi-agent: unknown agentId "${agentId}"` `` on a miss. The alias
  resolution helper should return the canonical uuid (or `undefined`) so
  these three call sites can resolve-then-`.get()` with their existing
  error path unchanged on a genuine miss.
- `agents-plugin-pi/src/execute-gateway.ts#L593-613` (`ws-approve` execute)
  — `rpcRegistry.get(p.agent_id)` at L595 is the fourth resolution call
  site; needs the same helper, imported from `spawner.ts`.
- `agents-plugin-pi/src/spawner.ts#L1709-1739` (`attachEventListener`) —
  the settle branch is `void (async () => { if (!flushPendingFinal(...) &&
  !record.threadBound && !record.terminalThisTurn) { ...push idle... }
  await probeAgentLiveness(...); })();`. Park is a new statement appended
  **after** `probeAgentLiveness` resolves, re-checking
  `!record.threadBound && !record.running` at that point and calling
  `stopAgent(registry, record.agentId, pi, { silent: true })` (the
  "existing silent-stop path" the ticket names — already used by
  `stopAll()`'s shutdown sweep and `ask.ts`'s thread-close). No new
  plumbing is needed for the "after the advisory/nudge judgment" ordering:
  `fork.ts`'s `wireAntiBleedLoop` (registered as a **second**, independent
  `client.onEvent` listener — see `armForkRoleWiring`, `fork.ts#L609-627`,
  called only *after* `spawnAgent`/`attachEventListener` returns, per
  `fork.ts#L693-708`) fires synchronously inside the same event dispatch,
  ahead of this async IIFE resuming past its own `await`. Confirmed by
  `promptAgent` (`spawner.ts#L1215-1230`) setting `record.running = true`
  synchronously before its own `await client.prompt(...)` — so a nudge
  that runs before the IIFE resumes has already flipped `record.running`
  by the time park's check reads it. This is exactly the ticket's own
  "defense in depth, not the mechanism" note — no explicit call from
  `attachEventListener` into `fork.ts` is required or should be added.
- `agents-plugin-pi/src/spawner.ts#L2000-2009` (`listAgents`) — needs
  `alias?`, `title?` on every row, plus `prompt?` gated by a new
  `includePrompt` option threaded from the `ws-agent-list` tool's new
  `include_prompt` param (registration at `#L2254-2264`).
- `agents-plugin-pi/src/spawner.ts#L1005-1016` (`buildPushContent`) and its
  one production caller `sendPush` (`#L1072-1099`, the `buildPushContent`
  call at `#L1088`) — minimal-diff option: leave `buildPushContent`'s
  signature untouched (it is also called directly, with a bare id, by
  `test/push-render.test.ts` and `test/spawner.test.ts`) and instead have
  `sendPush` pass a pre-composed display id (e.g. `` `${record.alias}
  (${record.agentId})` `` when `record?.alias` is set, else
  `record?.agentId` unchanged) as the existing `agentId` parameter. This
  satisfies "pushed-message heads ... print the alias when there is one,
  followed by the uuid" with a one-line change and zero signature churn.
  `push-render.ts` needs no code change under this approach — its
  `buildPushRenderLines` already treats `lines[0]` as an opaque head
  string (`push-render.ts#L75-91`) — but its existing hardcoded-head
  assertions in `test/push-render.test.ts` (e.g. `head:
  "[ws-agent-report] agent w1"`) are unaffected since those tests call
  `buildPushContent` directly with a bare id, bypassing `sendPush`; new
  tests covering the alias-prefixed head belong in `test/spawner.test.ts`
  (which owns `sendPush`/push-shape coverage) instead.
- `agents-plugin-pi/src/spawner.ts#L2174-2220` (`ws-agent-spawn` tool),
  `#L2254-2264` (`ws-agent-list` tool) — parameter schema additions
  (`alias`, `title` on spawn; `include_prompt` on list) and passthrough
  into `spawnAgent`/`listAgents`.
- `agents-plugin-pi/src/agent-sidecar.ts#L51-75` (`PersistedOrphan`),
  `#L97-119` (`captureOrphans`), `#L139-172` (`parseOrphans`),
  `#L181-197` (`rehydrateOrphanRecord`), `#L285-295` (`buildOrphanSummary`)
  — field additions (`alias?`, `title?`, `prompt?`) end-to-end through the
  capture/serialize/parse/revive/summary pipeline. `captureOrphans`'s skip
  condition (`#L100`: `if (!record.client || record.threadBound) continue;`)
  drops the `!record.client` half — same one-line shape as the
  `computeFanIn` fix above. `captureOrphans`'s own doc comment
  ("Selects... those with a LIVE client") and `index.ts`'s
  `session_shutdown` comment (`index.ts#L410-413`: "after stopAll() every
  record is dormant and captureOrphans would return nothing") both go
  stale once dormant records are captured too — comments only, no
  behavior risk, since `writeSidecar`/`captureOrphans` already run
  *before* `stopAll()` (`index.ts#L408-416`) and will simply now also
  capture what were previously already-dormant entries.
- `agents-plugin-pi/src/index.ts#L334-357` — orphan revival call site;
  `reviveOrphans(..., { fork, executeWorker })` passes no `worker` wiring
  key, which is already correct (`OrphanRoleWiring.worker` is documented
  as "nothing role-specific to re-arm") and needs no change — plain
  parked workers will now revive far more often (every settled child,
  not just ones that were still live at shutdown), but the existing
  no-op path already handles that.
- `agents-plugin-pi/src/process-role.ts` — the established pattern for a
  named, exported env-var constant (`WS_PI_SPAWN_ROLE_ENV`,
  `WS_PI_PARENT_SESSION_KEY_ENV`) and a pure `read*(env: NodeJS.ProcessEnv)`
  resolver, mirrored by `spawner.ts#L932-934`'s
  `shouldPushToLead(env = process.env)`. `WS_PI_AGENT_REGISTRY_CAP` is a
  lead-process-only config (never forwarded to a child's env), so its
  constant/resolver belongs in `spawner.ts` near `spawnAgent`, not in
  `process-role.ts` (which is specifically about child-carried role
  markers). Follow the same default-param-for-testability shape:
  `resolveAgentRegistryCap(env: NodeJS.ProcessEnv = process.env): number`.
- `agents-plugin-pi/src/spawner.ts#L887-888` — `REPORT_LOG_CAP` is the
  precedent for a small exported numeric cap constant
  (`export const REPORT_LOG_CAP = 64;`); mirror it for
  `DEFAULT_AGENT_REGISTRY_CAP = 256` and a `PROMPT_STORAGE_CAP_BYTES = 4096`
  (or similar) constant for the head-truncation cap.
- `agents-plugin-pi/src/spawner.ts#L290-330` (`AgentEventLineBuffer`) —
  the file's existing convention for multibyte-safe string handling
  (`StringDecoder`); the new prompt head-truncation helper should not
  split a multi-byte UTF-8 sequence when cutting at a byte boundary — a
  naive `Buffer.slice` cut mid-codepoint would corrupt the tail. Use
  `Buffer.from(prompt, "utf8")`, slice to the byte cap, then decode with a
  `StringDecoder("utf8")` (drops a partial trailing sequence cleanly)
  before appending the cut marker line.
- `agents-plugin-pi/pi-lead-guide.md#L33-38,#L58-82` — verb-routing table
  rows for `ws-agent-spawn`/`ws-agent-list` need alias/title mentions; the
  "when you have nothing delegated the line is absent entirely" prose
  (`#L80-82`) is now only true for a genuinely empty (or fully-threadBound)
  registry — once any non-threadBound record exists (dormant/parked
  included), the line is present indefinitely at `0 …` until cap eviction
  removes it. This is a real behavior/doc change, not just a wording nit
  — flagged since a lead reading the old text would misjudge an empty
  fan-in as "nothing has ever been delegated."
- `agents-plugin-pi/test/spawner.test.ts#L1387-1436` — the
  `attachEventListener`/settle-suppression test block (`listenerHarness`,
  `liveRpcRecord`, `fakePi`) and `#L1604-1684` (`stopAgent` block,
  `stoppableClient` helper providing duck-typed `abort`/`stop`) are the
  direct patterns to extend for park tests: emit `agent_settled` via the
  captured listener, `await new Promise((r) => setImmediate(r))` (possibly
  twice, since park now runs after an additional `await` inside the same
  IIFE) to let the async settle body finish, then assert `record.client
  === undefined` and no additional push beyond the existing idle-settle
  one.
- `agents-plugin-pi/test/agent-sidecar.test.ts` — existing round-trip
  coverage (`captureOrphans` -> `serializeOrphans` -> `parseOrphans` ->
  `reviveOrphans`) is the pattern to extend with alias/title/prompt
  fields and an old-shape (no alias/title/prompt) fixture for backward
  compatibility.
- `ai-docs/spec/pi-adapter-runtime.md` — the three anchors named in the
  ticket's Spec Impact section exist as written:
  `{#260903-pi-spawner-completion-gating}` (L309), `{#260904-pi-report-to-lead-channel}`
  (L426), `{#260903-pi-delegation-spawner-tools}` (L225). Each needs the
  prose correction the ticket already specifies verbatim (park replaces
  "stays alive"/"only grows" wording; `ws-agent-list` status vocabulary
  note that `idle` is now transient).

## Implementation Plan

1. `spawner.ts`: add `WS_PI_AGENT_REGISTRY_CAP_ENV` constant +
   `resolveAgentRegistryCap(env = process.env)` (mirrors
   `shouldPushToLead`'s testable-env-param shape) and
   `DEFAULT_AGENT_REGISTRY_CAP = 256`, near `REPORT_LOG_CAP` (L887-888).
2. `spawner.ts`: add `PROMPT_STORAGE_CAP_BYTES` constant and a pure
   `truncatePromptForStorage(prompt: string, capBytes = PROMPT_STORAGE_CAP_BYTES): string`
   helper (byte-safe cut via `Buffer`+`StringDecoder`, appends a cut-marker
   line when truncated).
3. `spawner.ts`: add `alias?`, `title?`, `prompt?` to `RpcAgentRecord`
   (L664-857) with a doc comment distinguishing this "agent alias" from
   the pre-existing "model alias" concept in the same file.
4. `spawner.ts`: add an exported alias-or-uuid resolver, e.g.
   `resolveAgentId(registry: RpcAgentRegistry, idOrAlias: string): string | undefined`
   — direct `registry.has(idOrAlias)` first (uuid path), else scan
   `registry.values()` for `record.alias === idOrAlias`.
5. `spawner.ts`: extend `SpawnAgentParams` (L1338-1343) with `alias?:
   string`, `title?: string`. In `spawnAgent` (L1790), before any side
   effect:
   - if `params.alias` is set, find an existing holder by `record.alias
     === params.alias`; if found and (`holder.running || holder.threadBound`),
     throw a rejection error naming the alias and the blocking state;
     otherwise clear `holder.alias = undefined` (leave `holder.title`).
   - enforce the cap: while `registry.size >= resolveAgentRegistryCap()`,
     pick the dormant (`!record.client`), non-`running`, non-`threadBound`
     record with the oldest `Math.max(record.lastLeadPromptAt ?? 0,
     record.reportLog.at(-1)?.at ?? 0)`; if none exists, throw (spawn
     cannot fit without evicting a protected record); else
     `registry.delete(evictedId)` and remember `{alias, agentId}` evicted
     for the result line.
   - set `record.alias`, `record.title`, and
     `record.prompt = truncatePromptForStorage(params.prompt)`.
   - append one evicted-line field to the returned result (e.g.
     `evicted: "<alias-or-uuid>"`), and `alias: record.alias` alongside
     `agent_id`.
6. `spawner.ts`: `sendToAgent` (L1884), `stopAgent` (L2028),
   `getAgentTranscriptPath` (L2077) — resolve `agentId` through
   `resolveAgentId(registry, agentId) ?? agentId` before the existing
   `registry.get`/throw, so an unresolvable id still throws the existing
   `unknown agentId` message unchanged.
7. `execute-gateway.ts` (`ws-approve`, L593-613): import
   `resolveAgentId` from `spawner.ts`, resolve `p.agent_id` the same way
   before `rpcRegistry.get(...)`.
8. `spawner.ts`: `computeFanIn` (L944-953) — drop the `|| !record.client`
   half of the skip condition, keeping only `if (record.threadBound)
   continue;`. Update the doc comments on `computeFanIn`/
   `computeRunningStatusLine` (L936-982) to describe the new
   dormant-included presence rule.
9. `spawner.ts`: `attachEventListener`'s settle IIFE (L1722-1730) — after
   the existing `await probeAgentLiveness(pi, registry, record);`, add:
   if `registry` is defined and `!record.threadBound && !record.running`,
   call `await stopAgent(registry, record.agentId, pi, { silent: true })`
   (swallow its own errors the same best-effort way the surrounding code
   already does, since a park failure must not crash the settle handler).
10. `spawner.ts`: `sendPush` (L1072-1099) — compose the display id
    (`record?.alias ? \`${record.alias} (${record.agentId})\` : record?.agentId`)
    and pass it as `buildPushContent`'s `agentId` argument; leave
    `buildPushContent`'s own signature untouched.
11. `spawner.ts`: `listAgents` (L2000-2009) — add an `opts?: {
    includePrompt?: boolean }` param; include `alias`/`title` on every
    row when set, and `prompt` when `opts?.includePrompt`.
12. `spawner.ts`: `ws-agent-spawn` tool registration (L2174-2220) — add
    `alias`/`title` string params, pass through to `spawnAgent`. Response
    JSON already carries the new `alias`/`evicted` fields from step 5.
    `ws-agent-list` tool (L2254-2264) — add `include_prompt` boolean
    param, thread into `listAgents`.
13. `agent-sidecar.ts`: `PersistedOrphan` (L51-75) — add `alias?`,
    `title?`, `prompt?`. `captureOrphans` (L97-119) — drop the
    `!record.client` half of its skip (L100), populate the three new
    fields. `parseOrphans` (L139-172) — read the three fields
    defensively (string-or-undefined, same pattern as existing optional
    fields). `rehydrateOrphanRecord` (L181-197) — restore them onto the
    revived record. `buildOrphanSummary` (L285-295) — prefer
    `alias`/`title` over the bare `agentId` in each rendered line.
14. `index.ts`: no functional change required at the `session_shutdown`
    call site (L408-416); update the stale "after stopAll() ... would
    return nothing" comment to reflect that dormant records are now
    captured too.
15. `pi-lead-guide.md`: update the `ws-agent-spawn`/`ws-agent-list` rows
    (recommend alias+title on every spawn; mention `include_prompt`), add
    a line describing automatic parking and resume-on-send, and correct
    the "line is absent entirely" prose (L80-82) to describe the new
    dormant-inclusive presence rule and the cap-eviction line.
16. `ai-docs/spec/pi-adapter-runtime.md`: apply the three anchor
    corrections named verbatim in the ticket's Spec Impact section
    (`{#260903-pi-spawner-completion-gating}`,
    `{#260904-pi-report-to-lead-channel}`,
    `{#260903-pi-delegation-spawner-tools}`).
17. Tests (extend `test/spawner.test.ts`, `test/agent-sidecar.test.ts`,
    `test/execute-gateway.test.ts`, `test/fork.test.ts` as needed) per the
    ticket's own Phase 1 test list: spawn with/without alias+title; alias
    overwrite on dormant vs. rejection on running/threadBound; resolution
    by alias on send/stop/transcript/approve; park after settle (final
    still delivered, `0 …` line present via the new presence rule); no
    park for `threadBound` or nudge-reprompted records; a parked record
    resumed via the overlay `ForkChannel` path and via the nudge path
    (not only `ws-agent-send`); park -> resume -> `ws-agent-transcript`
    still holds the parked turn; thread-bound exclusion from eviction;
    prompt head-truncation at 4 KB (byte-safe on multibyte input); cap
    eviction order + result line; sidecar round-trip old- and new-shape;
    `ws-agent-list` rows with/without `include_prompt`; head rendering
    with an alias.

## Verification Plan

- `cd agents-plugin-pi && npm test` (all suites, per the ticket's own
  Phase 1 verification line).
- Manual/live (ticket-named, not automatable offline): one live run with
  three aliased workers — check push heads show alias+uuid,
  `ws-agent-list` shows them dormant after their finals, `ws-agent-send
  <alias>` resumes one, and a `/reload` keeps all three in the list.

## Escalations

- None.
