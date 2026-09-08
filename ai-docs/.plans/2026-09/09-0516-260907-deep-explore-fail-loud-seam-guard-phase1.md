# Plan: 260907-bug-ws-pi-deep-explore-missing-collection-tool — Phase 1: Fail loud when a child's custom-tool registration does not complete

## Relevant Ticket Contract

- Guard the `session_start` seam so a child that fails to register this
  extension's custom tools never comes up silently presenting only Pi's
  builtin `--tools` (read/grep/find/ls + parallel wrapper).
- On a `startBridge` (or `registerAgentTools`) failure in a spawned child,
  fail the child loudly with the underlying launch error rather than
  continuing toolless; for an explore child specifically, a researcher whose
  blocking `explore` (deep) or expected read surface did not register must
  surface an error to its parent, not a silent partial surface.
- Retain all approved permission, lifecycle, and model-selection constraints;
  do not widen any tool group as a workaround; do not change ordinary
  worker/execute-worker recon permissions.
- Adapter-only, Pi-track: all changes stay under `agents-plugin-pi/`. No
  ws-mcp or shared rsrc semantic changes. If diagnosis needs a new *public*
  contract, escalate instead of inventing one — an internal-only,
  adapter-private helper is not a public contract.
- Verification boundary (ticket text): (1) a regression fixture that drives
  the real `session_start` registration path with a forced `startBridge`
  failure, asserting a loud child failure / a parent-visible error, failing
  before the guard and passing after; (2) with `startBridge` succeeding, a
  fresh simple researcher still shows only read/grep/find/ls and a fresh deep
  researcher shows those reads plus a registered blocking `explore`; (3) run
  the adapter suite under clean and inherited role/mode environments, ordinary
  worker/execute-worker recon left unchanged.

## Out of Scope

- Phase 2 (live-dogfood trigger confirmation, restoring deep collection if a
  genuine defect remains) — blocked on live evidence, not this phase.
- `260907-bug-ws-pi-children-inherit-stale-bootstrap-binary-env` (the
  plausible proximate trigger) — a separate ticket; this phase's guard must
  work independent of which fault first makes `startBridge` throw.
- Any change to `agents-plugin-tool/` (ws-mcp Go) or shared playbook/rsrc
  text.
- Any change to `TOOL_GROUPS`/`resolveTools` contents in
  `agents-plugin-pi/src/spawner.ts` (the deep-group registration chain is
  already confirmed correct by the ticket's source trace) — this phase only
  guards the seam that runs those registrations, it does not touch what they
  register.
- Changing the interactive host lead's own failure UX beyond a loud
  `ctx.ui.notify` — the ticket's language ("a child", "a spawned child",
  "surface an error to its parent") targets spawned worker/explore/fork
  children, which have an actual RPC parent to notify; the host lead has no
  parent process to signal, so it is not hard-crashed by this guard (see
  Implementation Plan step 2 for the exact role split).

## Codebase Findings

- `agents-plugin-pi/src/index.ts#L356-L595` — the `session_start` handler.
  `handle = await startBridge(pi, {...})` (L399-L408) has no try/catch, and
  `registerAgentTools(...)` (L417) — which is what conditionally registers
  the blocking `explore` tool for a deep researcher
  (`spawner.ts#L3298-L3299`) — only runs if `startBridge` resolved. Every
  registration after L418 (`registerExecuteGateway`, `registerFork`,
  `registerAsk`, orphan revival, the agent widget, `computeSessionBootstrap`)
  depends on `handle`/`agentTools` already existing, so a failure here must
  short-circuit the rest of the handler, not just skip one call.
- `agents-plugin-pi/src/index.ts#L410-L418` — exact seam to guard:
  ```
  const onApprovalPending = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
  agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd }, onApprovalPending, undefined, exploreGuidePath, toolPreviewTuiRef);
  rpcRegistryRef.current = agentTools.rpcRegistry;
  ```
  `createApprovalRelay` must stay wired before `registerAgentTools` (existing
  comment at L411-L415 explains why); keep that order inside the guarded
  block.
- `agents-plugin-pi/src/index.ts#L249-L252, L357` — existing precedent for a
  "loud but non-crashing" failure signal already in this file:
  `forkContextError` is captured, then `ctx.ui.notify(forkContextError,
  "error")` is called at the top of `session_start`, followed by `return` —
  but this precedent alone is **not sufficient** for a headless spawned
  child (see next finding): it only makes the failure visible to a human
  looking at that same process's UI, which a headless RPC-driven
  worker/explore child does not have.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/core/extensions/runner.js#L623-L649` — confirms the ticket's fact #2: `emit()` wraps
  every handler call in `try { await handler(...) } catch (err) {
  this.emitError(...) }`; `emitError` (L411-L415) just calls registered
  listeners. A thrown `session_start` handler does not crash the process and
  does not, by itself, propagate to a spawning parent.
- `agents-plugin-pi/node_modules/@earendil-works/pi-coding-agent/dist/modes/rpc/rpc-client.js` — this is the RPC transport `spawner.ts` uses for every
  persistent worker/explore/fork child (`RpcClient`, imported by
  `spawner.ts`). Confirmed exit-handling that a fix can reuse **without any
  new protocol**:
  - `start()` (L26-L85) spawns `node <cliPath> --mode rpc ...` as a real
    child process, waits 100ms, and **throws** if the child's `exitCode` is
    already non-null (L79-L84).
  - `once("exit", ...)` (L53-L59) sets `exitError` and rejects every pending
    request the instant the child process exits, for any exit after that
    100ms window.
  - `send()` (L437-L455) also throws immediately if `exitCode !== null` or
    `exitError` is set.
  This means: **if a spawned child's process actually terminates (exit code
  != 0) during or shortly after `session_start`, the parent's
  `RpcClient.start()`/next RPC call already throws a real,
  already-tested error** — no readiness file, no new IPC channel needed. This
  is the cheapest way to make "the parent sees an error instead of a
  toolless researcher" literally true, reusing existing machinery.
- `agents-plugin-pi/src/spawner.ts#L2608-L2637` — `spawnAgent`'s existing
  try/catch around `client.start()` / `verifyResearchSelection` /
  `promptAgent` already converts any such thrown error into a rejection
  surfaced to the `ws-agent-spawn`/`explore` MCP caller (the lead). No
  changes needed here — this is the consumer of the failure signal the guard
  will produce.
- `agents-plugin-pi/src/goal-loop.ts#L531-L541` — doc comment confirms Pi's
  own global `uncaughtException` → `process.exit(1)` safety net is described
  as **interactive-mode**-scoped; do not rely on it to cover a headless
  spawned child. The guard must call `process.exit` itself rather than just
  re-throwing and hoping something else exits the process.
- `agents-plugin-pi/src/bridge.ts#L899-L902` — `startBridge`'s own outer
  catch already calls `shutdown()` (closing the half-started ws-mcp
  subprocess) before re-throwing, so a caller-side guard has nothing left to
  clean up on a `startBridge` failure.
- `agents-plugin-pi/src/process-role.ts#L28, L65-L78` — `SpawnRole =
  "worker" | "explore" | "fork"`; `readSpawnRole(env)` returns `undefined`
  for the host lead. `role !== undefined` is exactly "this process is a
  spawned child with an RPC parent," the split point Implementation step 2
  needs (`isLeadOrFork` is the wrong predicate here — it lumps `fork` in with
  the lead, but a `fork` child genuinely has a parent process and an RPC/CLI
  connection like `worker`/`explore` do).
- Risk signal (shortcut risk, reuse mechanism already present): a naive fix
  might invent a NEW file-based readiness/error-signaling protocol mirroring
  `fork`'s `readinessPath` (`spawner.ts#L1920-L1942`,
  `index.ts#L572-L594`). That is unnecessary extra adapter-internal surface
  for this phase — the RpcClient exit-based signal above already gives the
  parent a real error with zero new IPC. Do not build a parallel readiness
  file for worker/explore.
- Test-infra findings for the regression fixture:
  - `agents-plugin-pi/test/native-tool-registration.test.ts#L131-L151` — an
    existing test ("actual MCP startup registers through the same cold then
    late-filled shared ref") already calls the REAL `startBridge(pi, {
    launcherPath, pluginDir, runtimeJsonPath, cwd, toolPreviewTuiRef })`
    against a hand-written Python "fake ws-mcp" script
    (`fake-mcp.py`, written inline via `writeFileSync`) that answers
    `initialize`/`tools/list` over stdio JSON-RPC. This is the reusable
    pattern for a REAL (not mocked) `startBridge` call in a fast, offline
    unit test.
  - `agents-plugin-pi/src/mcp-stdio-client.ts#L239` — confirms the MCP stdio
    client already rejects pending calls with `"ws-mcp process exited
    unexpectedly (code=…, signal=…)"` when the launched process exits
    early — so a fake launcher script that does nothing but `sys.exit(1)`
    immediately (no JSON-RPC response at all) is a **real, deterministic,
    fast** way to make `client.initialize()` inside `startBridge` genuinely
    reject, without needing a slow or flaky real ws-mcp/Go binary.
  - `agents-plugin-pi/test/persistent-explore.test.ts#L95-L118` — this is
    the "generic dynamic allowlist probe" the ticket says already passed and
    did not catch the live failure: it calls `registerAgentTools` directly
    against a fake `bridge` object, entirely bypassing `startBridge`/
    `session_start`. Keep this test as-is (still useful role/mode coverage);
    it does not satisfy the new verification bullet because it never
    exercises the guarded seam.
  - No existing test imports `agents-plugin-pi/src/index.ts` or exercises its
    `session_start` handler at all (`grep` for `wsPiBridgeExtension`/`from
    "../src/index` across `test/*.ts` returns nothing). The regression
    fixture is new, not an extension of an existing index.ts test.
  - `agents-plugin-pi/test/fork-lifecycle.integration.test.ts#L1-L45` shows
    this repo *can* drive the real `wsPiBridgeExtension` through Pi's actual
    SDK (`sdk.createAgentSession` / `session.bindExtensions`) with a
    controllable fake launcher (sentinel files toggle simulated tool
    failures). This is a heavier alternative available if the lighter
    direct-function-call approach below turns out insufficient, but is not
    needed for this phase (see Implementation Plan step 3).

## Implementation Plan

1. In `agents-plugin-pi/src/index.ts`, add a small, generic, exported helper
   near the top of the file (module scope, after the existing imports,
   before `wsPiBridgeExtension`):
   ```ts
   export async function bootstrapOrFailLoud<T>(
     ui: Pick<ExtensionUIContext, "notify">,
     role: SpawnRole | undefined,
     bootstrap: () => Promise<T>,
     exitProcess: (code: number) => never = (code) => process.exit(code),
   ): Promise<T | undefined> {
     try {
       return await bootstrap();
     } catch (error) {
       const message = error instanceof Error ? error.message : String(error);
       ui.notify(`ws-pi-agent: session bootstrap failed — this session has no ws-mcp bridge or custom tools (${message})`, "error");
       if (role !== undefined) exitProcess(1);
       return undefined;
     }
   }
   ```
   Add `ExtensionUIContext` to the existing `import type { ExtensionAPI }
   from "@earendil-works/pi-coding-agent";` (L178), and add `type SpawnRole`
   to the existing `import { isLeadOrFork, readSpawnRole, ... } from
   "./process-role.ts";` (L198).
   Keep this function dependency-free of any per-`session_start` closure
   state (no refs, no `pi` beyond what's passed) — mirrors the existing
   default-injectable-dependency pattern already used by
   `registerAgentTools`'s `runExploreLeaf: typeof exploreLeaf = exploreLeaf`
   parameter (`spawner.ts#L3033`), which is what makes this testable without
   a full fake `ExtensionAPI`/`ExtensionContext`.

2. In the `session_start` handler (`index.ts#L399-L418`), replace:
   ```ts
   handle = await startBridge(pi, {
     launcherPath, pluginDir, runtimeJsonPath, cwd: ctx.cwd,
     toolPreviewTuiRef, ui: ctx.ui,
     forkContext: durableForkContextRef.current, previousOwnKeys,
   });
   sessionKeyRef.current = handle.defaultSessionKeyRef.current;

   const onApprovalPending = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
   agentTools = registerAgentTools(pi, handle, { cwd: ctx.cwd }, onApprovalPending, undefined, exploreGuidePath, toolPreviewTuiRef);
   rpcRegistryRef.current = agentTools.rpcRegistry;
   ```
   with:
   ```ts
   const bootstrap = await bootstrapOrFailLoud(ctx.ui, readSpawnRole(process.env), async () => {
     const h = await startBridge(pi, {
       launcherPath, pluginDir, runtimeJsonPath, cwd: ctx.cwd,
       toolPreviewTuiRef, ui: ctx.ui,
       forkContext: durableForkContextRef.current, previousOwnKeys,
     });
     const approval = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
     const tools = registerAgentTools(pi, h, { cwd: ctx.cwd }, approval, undefined, exploreGuidePath, toolPreviewTuiRef);
     return { handle: h, agentTools: tools, onApprovalPending: approval };
   });
   if (!bootstrap) return; // notified (and, for a spawned child, already exited) inside bootstrapOrFailLoud — never fall through to a partial/toolless registration.
   handle = bootstrap.handle;
   sessionKeyRef.current = handle.defaultSessionKeyRef.current;
   const onApprovalPending = bootstrap.onApprovalPending;
   agentTools = bootstrap.agentTools;
   rpcRegistryRef.current = agentTools.rpcRegistry;
   ```
   Everything from the old `registerExecuteGateway(...)` call onward
   (`index.ts#L419` through the end of the handler at `L595`) stays
   unchanged — it already reads `handle`/`agentTools`/`onApprovalPending` by
   name, which remain populated identically on success.
   Role split rationale (do not special-case `isLeadOrFork` here): `role !==
   undefined` covers `worker`/`explore`/`fork` — every role that is a spawned
   child with an RPC-connected parent process, which is exactly who benefits
   from `exitProcess(1)` (see Codebase Findings' `RpcClient` notes: an exited
   child process makes the parent's `client.start()`/next RPC call throw a
   real error). The host lead (`role === undefined`) has no such parent to
   signal, so it gets the loud `ctx.ui.notify` and an early `return` (no
   custom tools, no ws block — same "never silently toolless" property,
   without crashing the user's interactive terminal on top of an already
   broken bridge).

3. Regression fixture (new file, e.g.
   `agents-plugin-pi/test/session-bootstrap-guard.test.ts`):
   - Import `bootstrapOrFailLoud` from `../src/index.ts` and the real
     `startBridge` from `../src/bridge.ts` (same imports
     `native-tool-registration.test.ts` already uses for its `startBridge`
     test, `L1-L12`).
   - Write a **broken** fake launcher script to a temp dir mirroring
     `native-tool-registration.test.ts#L134-L135`'s `fake-mcp.py`
     construction, but instead have it do nothing but exit immediately
     (`writeFileSync(launcher, "import sys\nsys.exit(1)\n")`), so
     `startBridge`'s real `client.initialize()` call genuinely rejects via
     `mcp-stdio-client.ts`'s existing "process exited unexpectedly" path —
     a real forced failure, not a mocked one.
   - Case A (spawned child): call
     `bootstrapOrFailLoud(fakeUi, "explore", async () => { const h = await
     startBridge(fakePi, { launcherPath: brokenLauncher, pluginDir: tmpDir,
     runtimeJsonPath: <real runtime.json path, same as
     native-tool-registration.test.ts#L140>, cwd: tmpDir, toolPreviewTuiRef:
     ref }); throw new Error("unreachable — startBridge above must reject
     first"); }, exitSpy)` where `fakeUi.notify` and `exitSpy` are
     `node:test` `mock.fn()`s (`exitSpy` returns/throws a sentinel instead
     of truly exiting, so the test process survives). Assert: the returned
     value is `undefined`; `fakeUi.notify` was called once with `"error"`
     severity and a message containing the real underlying error text;
     `exitSpy` was called exactly once with `1`.
   - Case B (host lead): same broken-launcher setup with `role = undefined`.
     Assert: return is `undefined`; `notify` fired; `exitSpy` was **not**
     called.
   - Case C (happy path, guard does not fire): reuse
     `native-tool-registration.test.ts#L135`'s working `fake-mcp.py`
     verbatim. Assert `bootstrapOrFailLoud` resolves to the real `{ handle,
     agentTools, onApprovalPending }` object, `notify`/`exitSpy` never
     called, and remember to call `bootstrap.handle.shutdown()` /
     `bootstrap.agentTools.stopAll()` for cleanup (mirrors
     `native-tool-registration.test.ts#L147`/`L78`).
   - "Fails before / passes after" check for the implementer: since
     `bootstrapOrFailLoud` is new, temporarily test Case A against the
     *pre-guard* shape (call `startBridge` + `registerAgentTools` inline,
     with the same `fakeUi`/`exitSpy` never wired to anything) to confirm
     neither `notify` nor `exitSpy` would ever have fired for today's
     unguarded code — i.e., confirm the assertions in Case A are genuinely
     new coverage, not something the old code already satisfied by
     accident. Do this as a manual double-check while implementing, not as
     a permanent second test path.

4. Verification bullet 2 (healthy simple/deep researcher tool surface
   unaffected by the new guard): add two cases (same file as step 3, or
   extend `native-tool-registration.test.ts`) that call the SAME
   `bootstrapOrFailLoud` success path (Case C's working `fake-mcp.py`) with
   `process.env.WS_PI_SPAWN_ROLE = "explore"` and
   `process.env.WS_PI_EXPLORE_MODE = "simple"` / `"deep"` set beforehand
   (mirroring `native-tool-registration.test.ts#L137-L138`'s
   save/restore-env pattern), then inspect the fake `pi.registerTool` capture
   map (same `Captured`/`tools` harness as
   `native-tool-registration.test.ts#L14-L41`) for: simple → no `"explore"`,
   no `"bash"`; deep → `"explore"` present (and still no `"bash"`). This is a
   guard-focused regression, distinct from and complementary to
   `persistent-explore.test.ts#L95-L118`'s existing spawner-level dynamic
   allowlist probe — do not remove or replace that test.

5. Verification bullet 3: run the full adapter test suite twice — once with
   a clean env, once with `WS_PI_SPAWN_ROLE`/`WS_PI_EXPLORE_MODE` pre-set in
   the invoking shell before `npm test` — and record full output. Per the
   orchestrator's baseline note, `npm test`'s default script already runs
   `env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE node --test` (clean-env
   case is the default `npm test`); for the inherited-env case, export both
   vars in the shell first and run `node --test` directly (without the `-u`
   strip) or run the new/targeted files with `env
   WS_PI_SPAWN_ROLE=worker node --test <files>` variants. Compare against the
   documented ~130 pre-existing `WS_PI_SPAWN_ROLE`/fork-prefix failures from
   the missing installed Pi SDK bundle: report the delta (byte-identical
   failing-name set ⇒ zero regressions), not a raw pass/fail count. Confirm
   `persistent-explore.test.ts` (worker/execute-worker recon cases) and
   `execute-gateway.test.ts` still pass unchanged.

## Verification Plan

- `env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE node --test
  test/session-bootstrap-guard.test.ts test/native-tool-registration.test.ts
  test/persistent-explore.test.ts` — new fixture plus the two closest
  existing suites, clean env.
- Same command with `WS_PI_SPAWN_ROLE=worker` (and separately
  `WS_PI_SPAWN_ROLE=explore WS_PI_EXPLORE_MODE=deep`) exported first —
  inherited-env case from ticket verification bullet 3.
- Full `npm test`, both clean and inherited-env invocations; diff the
  failing-test-name set against the known ~130-failure baseline and report
  the delta in the implementation's final status.
- Manual/temporary check described in Implementation step 3's last bullet
  (confirm the new assertions are genuinely new coverage against the
  pre-guard code shape).

## Escalations

- None.
