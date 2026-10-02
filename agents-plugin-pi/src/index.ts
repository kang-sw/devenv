/**
 * ws-pi-bridge: Pi extension entry point.
 *
 * Bridges ws-mcp (the harness-neutral MCP server backing the ws workflow —
 * Go source untouched, see AGENTS.md's golden rule) onto Pi:
 *   - Spawns the ws-mcp launcher as a subprocess on session_start, closes it
 *     on session_shutdown (docs/extensions.md#L220-224, #L516-526 — no
 *     background processes from the top-level factory).
 *   - Re-registers every ws-mcp tool via pi.registerTool (bridge.ts) under a
 *     provider-legal sanitized name derived from the `ws/<tool>` prose form
 *     (`/` -> `__`, `.` -> `_`, e.g. `ws__playbook_read`) — SKILL.md prose
 *     stays untouched as literal `ws/playbook.read(...)` calls; the model
 *     maps that prose to the sanitized registered name itself.
 *   - Exposes ws skills through resources_discover via src/skills-dir.ts.
 *     Every startup/reload from the monorepo cleanly regenerates the ignored
 *     `agents-plugin-pi/skills/` tree from canonical `agents-plugin/skills/`;
 *     installed tarballs validate their already-generated tree against the
 *     current package-local rsrc manifest before Pi exposes it.
 *
 * Phase 2 adds the self-built delegation spawner (`ws-agent-spawn` /
 * `ws-agent-continue` / `explore`, see src/spawner.ts) on
 * top of the Phase 1 bridge. Phase 3 added (and the
 * `260905-feat-ws-pi-harness-config-layer` ticket's Phase 4 retired) an
 * adapter-owned tier-curation data file: tier-aware `--model` resolution now
 * goes through ws-mcp's `config.resolve_agent` tool
 * (`resolveModelForAliasViaWsMcp`, spawner.ts) instead of a hand-edited JSON
 * file, the unset-tier advisory in bridge.ts is sourced from the same tool,
 * and the read-only `ws-model-catalog-list` command below still exercises
 * Pi's `ctx.scopedModels` read API but now points the user at `config.tune
 * agents.tier harness:pi` / `lead-tune` for curation instead of a data file.
 *
 * The 260903 ticket's Phase 1 adds the goal-mode arming + `agent_settled`
 * re-injection loop (src/goal-loop.ts, `registerGoalLoop`): a `/goal <goal>`
 * command arms the loop, an armed `agent_settled` re-fire re-injects a
 * reminder naming the goal and its two terminal levers
 * (`goal-achieved`/`goal-blocked`, both model-invoked `pi.registerTool()`
 * calls per the ticket's "explicit skill calls, zero prose parsing" design
 * constraint), and a config-tunable runaway backstop force-stops the loop
 * after N consecutive no-tool-call re-fires. Registered at factory top
 * level alongside the other commands/tools below — no subprocess involved,
 * so it needs no `session_start` gating either.
 *
 * The 260904 ticket's Phase 1 adds the system-prompt bootstrap
 * (src/lead-bootstrap.ts, `registerLeadBootstrap`): a `before_agent_start`
 * handler appends a fixed ws block (the static part of the session-start
 * `workflow_manual` snapshot — session state and notes are cut, reaching the
 * model only through live `workflow_manual` calls — plus `pi-lead-guide.md`)
 * to the system prompt on every turn, for
 * the host lead and a future `fork` child only (never `worker`/`explore`).
 * `registerLeadBootstrap` itself is declarative (factory top level, no
 * subprocess); the actual snapshot fetch happens inside `startBridge`
 * (bridge.ts), and this file fills `wsBlockBaseRef.current` from that result
 * once `session_start`'s `startBridge` call resolves — same seam
 * `registerAgentTools` already uses.
 *
 * The 260904 "execute-approve-gateway" ticket's Phase 1 adds the end-to-end
 * `ws-execute`/`ws-approve` approval gateway (src/execute-gateway.ts,
 * `registerExecuteGateway`): a fixed-prompt `execute-worker` (spawner.ts's
 * new `"execute-worker"` `toolGroup`) whose every shell command elevates
 * through a lead-approval gate. `session_start` builds the approval-request
 * injection callback (`createApprovalRelay`) BEFORE calling
 * `registerAgentTools` (so it can be threaded into that call too — a
 * dormant-resumed execute-worker keeps its relay wired even if later driven
 * through the generic `ws-agent-*` tools), then calls
 * `registerExecuteGateway`, then — lead/fork sessions only —
 * `pi.setActiveTools(computeLeadActiveTools(...))` to remove native
 * `bash`/`read` (and exclude the gated-exec tool itself, the auto-include
 * footgun fix — see execute-gateway.ts's doc comment) while adding
 * `ws-execute`/`ws-approve`/the ugly-named read tool.
 *
 * The 260904 "side-thread fork question surface" ticket's Phase 1 adds
 * `ws-fork` (src/fork.ts, `registerFork`): a `pi --fork <own session>`
 * lateral peer sharing the caller's full context and ordinary settlement
 * lifecycle. `session_start` calls `registerFork` right after
 * `registerExecuteGateway` (same shared `agentTools.rpcRegistry`), then,
 * inside the same lead/fork-only `isLeadOrFork` block as
 * `computeLeadActiveTools` above, applies `addForkToolIfLead` as a
 * SEPARATE, role-differentiated `setActiveTools` step — `role === undefined`
 * (the true top lead) only, never a fork — so a fork's own active-tools
 * surface never regains `ws-fork` (no recursive forking; see fork.ts's own
 * doc comment for the full risk-signal trace).
 *
 * That ticket's Phase 2 adds the owner-question surface (src/ask.ts, built on
 * src/conversation-view.ts's shared component): `ws-queue-question`/`ws-withdraw-question` (renamed by `260911`
 * from `ws-ask`/`ws-resolve`), a
 * persisted per-lead-session thread registry, `/thread`, `/answer <id>`
 * (which lazily forks a discussion thread at the lead's tip AT OPEN TIME and
 * attaches a live conversation view to it), and the `/done` summary injected
 * back into the lead as a Pi custom message. `session_start` re-captures `ctx`
 * into the registry handle on every firing (§5's captured-ctx staleness
 * rule), hydrates the registry from its sibling state file, threads
 * `handleForkRaisedQuestion` into `registerFork` as its new `onQuestion`
 * callback, and applies `addAskToolsIfLead` as a third role-differentiated
 * active-tools step.
 *
 * `260930-feat-ws-pi-disconnect-owner-question-wiring` disconnects both entry
 * points while keeping the code: the top lead's active tools now EXCLUDE
 * `ws-queue-question`/`ws-withdraw-question` (`computeSessionBootstrap`), and
 * `onForkQuestion` is no longer armed on fresh or revived forks. Spawned
 * agents decide and record, or settle with a blocker, instead of asking.
 * Registration, hydration, `/thread`, and `/answer` stay so a thread persisted
 * before the cut can still be drained.
 *
 * 260905 Phase 1 Edition (push delivery): three factory/session_start hooks
 * serve the pushed child-report channel. `registerPushFlush` (factory scope)
 * releases the pushes the spawner held while this session was mid-turn, on
 * this session's own `agent_settled`; `session_start` fills
 * `spawner.ts`'s `leadIdleRef` with this session's `ctx.isIdle` (the seam that
 * decides hold vs send) and, in TUI only, registers the compact
 * `push-render.ts` renderers for the six families; `session_shutdown` drops
 * the held queue with the session it belonged to.
 *
 * The `260905-feat-ws-pi-live-agent-widget` ticket's Phase 1 adds the
 * live-agent widget (src/agent-widget.ts, `createAgentWidgetController`): one
 * compact `belowEditor` panel listing every live agent and owner discussion
 * thread, plus a `setStatus` footer segment (`ws: N agents · M questions`).
 * It also folds the 260904 owner-question surface's standalone `N pending`
 * `aboveEditor` widget into this one panel — `ask.ts` no longer owns any
 * widget of its own. `session_start`, in the same TUI-lead-only block that
 * hydrates the thread registry, (re)creates the controller over
 * `agentTools.rpcRegistry`/`threadHandle.threads` and points
 * `spawner.ts`'s `agentWidgetRefreshRef` at its `refresh()`; every
 * registry-transition point in `spawner.ts` and every widget-refresh call
 * site left in `ask.ts` fire through that same ref, so neither module
 * imports `agent-widget.ts` directly. `session_shutdown` stops its 10-second
 * elapsed timer and clears both the widget and the status segment.
 *
 * The `260906-bug-ws-pi-lead-cannot-see-or-load-skills` ticket's Phase 1 adds
 * the lead/fork skill surface (src/lead-skills.ts): removing native
 * `read`/`bash` from the reshaped lead/fork tool surface (above) also
 * silently dropped Pi's own `<available_skills>` system-prompt block and its
 * `read`-the-SKILL.md loading path, leaving the lead with no way to see or
 * load a skill it was not told about via `/skill:<name>`. The fix is
 * adapter-owned, mirroring the workflow-manual/tool-reshape split already in
 * this file: a dedicated `<available_skills>` block (pointing at `ws-skill`,
 * never `read`) is appended as the third ordered item of the ws
 * system-prompt block, and `ws-skill(name, args?)` is registered globally
 * and added to the active-tools surface for lead AND fork alike
 * (`isLeadOrFork`, not the narrower lead-only gate
 * `addForkToolIfLead`/`addAskToolsIfLead` use). This ticket also collapses
 * the previously separate `buildWsBlock` call and three sequential
 * `pi.setActiveTools()`/`pi.getActiveTools()` reshape steps below into one
 * `computeSessionBootstrap` call (`lead-bootstrap.ts`) — a single pure
 * function producing the whole lead/fork session-start outcome, testable
 * end-to-end without a fake `ExtensionAPI`.
 *
 * A later dogfood fix corrected this ticket's original `session_start`-time
 * `pi.getCommands()` snapshot: Pi actually runs `session_start` FIRST and
 * only afterwards merges an extension's own `resources_discover` skills into
 * the live command list (`extendResourcesFromExtensions`, confirmed against
 * the installed `agent-session.js`), so that snapshot predated every ws
 * skill and both `ws-skill` and the `<available_skills>` block saw only
 * whatever other extension (e.g. `imagegen`) had registered first. Both now
 * resolve `pi.getCommands()` LIVE instead — `ws-skill` inside its own
 * `execute()` (`lead-skills.ts`), the block inside `before_agent_start`
 * (`lead-bootstrap.ts`'s `computeSkillsBlockCached`, cached in
 * `skillsBlockCacheRef` below) — so no `session_start`-scoped skill snapshot
 * exists in this file at all any more.
 *
 * HAND-SYNC NOTE: bin/ws-mcp-launcher.py, runtime.json, and rsrc/ in this
 * package are byte-identical copies of the same-named files under
 * agents-plugin/ (same precedent as agents-plugin-wsflow's copies — no
 * cross-root relative reference). agents-plugin-tool/scripts/bump-ws-version.sh
 * resyncs all three automatically at every ws version bump (260914); this
 * note lives here rather than as a header comment inside
 * bin/ws-mcp-launcher.py so the copy stays byte-identical with
 * agents-plugin/bin/ws-mcp-launcher.py (verifiable via `diff`) instead of
 * silently drifting from it. A manual hand-edit outside the bump helper still
 * needs the same care: when agents-plugin/bin/ws-mcp-launcher.py,
 * agents-plugin/runtime.json, or agents-plugin/rsrc/ change, re-copy the
 * changed file(s) here verbatim — all three copies must stay in lockstep.
 * A stale/missing rsrc/ copy surfaces at call time, e.g. workflow_manual:
 * "render playbook: rsrc manifest missing".
 *
 * `skills/` is a separate, fourth carried copy with a different sync model:
 * scripts/copy-skills.mjs generates it for prepack/prepare, and
 * resources_discover regenerates it on local startup/reload when the canonical
 * sibling tree exists. Both paths remove stale entries and validate shim
 * playbook targets against the package-local rsrc manifest. The copy stays
 * gitignored and is never hand-synced.
 */

import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI, ExtensionUIContext } from "@earendil-works/pi-coding-agent";
import { resolveSessionKey, startBridge, type BridgeHandle } from "./bridge.ts";
import {
  agentCostRefreshRef,
  agentWidgetRefreshRef,
  heldPushQueue,
  leadIdleRef,
  ownerNotifyRef,
  pushToLead,
  registerAgentTools,
  registerPushFlush,
  sendToLead,
  type AgentToolsHandle,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "./spawner.ts";
import { createAgentWidgetController, shouldArmAgentWidget, type AgentWidgetController } from "./agent-widget.ts";
import { registerPushMessageRenderers } from "./push-render.ts";
import { buildOrphanPush, captureOrphans, noSessionSidecarPath, readAndClearSidecarAt, reviveOrphans, sidecarPath, writeSidecarAt, type PersistedOrphan } from "./agent-sidecar.ts";
import { registerGoalLoop, resolveAgentWaitAnimation, resolveChildRetentionTtlDays, resolveSettleDelayMs, type GoalLoopConfig } from "./goal-loop.ts";
import { createWsConfigReader, thenOrNow } from "./adapter-config.ts";
import { registerSkillResources } from "./skills-dir.ts";
import { computeSessionBootstrap, registerLeadBootstrap, type LeadPromptRef, type SkillsBlockCache, type WsBlockBase } from "./lead-bootstrap.ts";
import { registerModelPrompts } from "./model-prompts.ts";
import { applyForkAffinity, captureRegisteredTools, classifyForkRegistrations, compareForkRegistrations, effectiveForkDescriptor, formatForkRegistrationMismatch, frameForkInput, readForkLaunchContext, removeForkTransport, restoreForkContext, restoreForkKeys, FORK_READINESS_KIND, type ForkContext } from "./fork-context.ts";
import { ChildChannel, readAndDeleteChannelBootstrap } from "./agent-channel.ts";
import { ChildApprovalGate } from "./approval-protocol.ts";
import { isLeadOrFork, readSpawnRole, WS_PI_FORK_CONTEXT_ENV, WS_PI_PARENT_SESSION_KEY_ENV, type SpawnRole } from "./process-role.ts";
import { createApprovalRelay, registerExecuteGateway } from "./execute-gateway.ts";
import { attachMailboxRuntimeCleanup, buildMailboxPushMessage, createBridgeDrain, createSubprocessWait, resolveMailboxSelfSlug, sessionMailboxWaitOptions, shouldArmMailboxWaiter, stageMailboxRuntime, startMailboxWaiter, type MailboxToolCall, type MailboxWaiterHandle } from "./mailbox-waiter.ts";
import { armForkRoleWiring, registerFork } from "./fork.ts";
import {
  createThreadRegistryHandle,
  type ThreadRegistryHandle,
  captureForkResume,
  hydrateThreadRegistry,
  registerAsk,
  registerThreadCommands,
  threadRegistryPath,
  saveThreadRegistryFile,
} from "./ask.ts";
import { registerAuditCommands } from "./audit.ts";
import { registerWsSkillTool } from "./lead-skills.ts";
import { createToolPreviewTuiRef, loadToolResultTuiModules } from "./tool-result-render.ts";
import { createAgentStorageContext, pruneStaleAgentHomes, reportOwnershipDiagnostic, type AgentStorageContext } from "./agent-storage.ts";
import { createAgentFooterSessionLifecycle, type AgentFooterContext, type AgentFooterSessionLifecycle } from "./agent-footer.ts";
import { descendantUsageValue, retentionEvictionCost } from "./agent-cost.ts";
import { createDescendantUsageReporter, descendantUsageReporterRef, evaluateDescendantUsage } from "./agent-usage-rollup.ts";
import { loadHostPiTui } from "./pi-tui.ts";
import { assertPolicyTool, readDelegationPolicy } from "./delegation-policy.ts";
import { registerScopedWriteTools } from "./write-scopes.ts";
import { registerWebTools } from "./web-tools.ts";
import { publishSubtree, SubtreeUpstream } from "./subtree-lifecycle.ts";

// This is the exact physical entry module Pi loaded (whether from `-e`, an
// installed package, or a cache). Every RPC child receives this path verbatim
// rather than rediscovering an ambient extension copy.
const extensionEntryPath = fileURLToPath(import.meta.url);
const srcDir = dirname(extensionEntryPath);
const pluginDir = dirname(srcDir); // agents-plugin-pi/
const repoRoot = dirname(pluginDir);
const launcherPath = join(pluginDir, "bin", "ws-mcp-launcher.py");
const runtimeJsonPath = join(pluginDir, "runtime.json");
const piLeadGuidePath = join(pluginDir, "pi-lead-guide.md");
const leadCompactGuidePath = join(pluginDir, "lead-compact-guide.md");
const executeWorkerGuidePath = join(pluginDir, "execute-worker-guide.md");
const exploreGuidePath = join(pluginDir, "explore-guide.md");

/** Installs one bounded reporter for the active adapter session. Only a TUI owner lead has a notification surface. */
export function applySessionStartOwnershipDiagnostics(
  role: SpawnRole | undefined,
  ctx: Pick<ExtensionUIContext, "mode" | "ui">,
): void {
  ownerNotifyRef.current = role === undefined && ctx.mode === "tui"
    ? (message, type) => ctx.ui.notify(message, type)
    : undefined;
}

export function applySessionShutdownOwnershipDiagnostics(): void {
  ownerNotifyRef.current = undefined;
}

/** Controller-session retention seam: child workers never run global disk maintenance. */
export async function applySessionStartAgentFooter(
  lifecycle: AgentFooterSessionLifecycle,
  role: SpawnRole | undefined,
  ctx: AgentFooterContext & { mode?: string },
  registry: RpcAgentRegistry,
  storage: AgentStorageContext,
): Promise<void> {
  await lifecycle.start(role, ctx, registry, storage);
}

export function applySessionShutdownAgentFooter(lifecycle: AgentFooterSessionLifecycle): void {
  lifecycle.stop();
}

/** Factory-scoped hooks: reload replaces the controller, not these subscriptions. */
export function registerAgentFooterGitEvents(pi: Pick<ExtensionAPI, "on">, lifecycle: AgentFooterSessionLifecycle): void {
  pi.on("turn_end", () => { lifecycle.turnEnd(); });
  pi.on("input", () => { lifecycle.input(); });
}

/** The lead's own message stream feeds the footer's output-rate tracker and usage. */
export function registerAgentFooterOutputEvents(pi: Pick<ExtensionAPI, "on">, lifecycle: AgentFooterSessionLifecycle): void {
  pi.on("message_start", (event) => { lifecycle.observeOutput(event); });
  pi.on("message_update", (event) => { lifecycle.observeOutput(event); });
  // The rate is finalized before acceptUsage requests the repaint.
  pi.on("message_end", (event) => { lifecycle.observeOutput(event); lifecycle.acceptUsage(event.message); });
}

/**
 * The one session-start orphan revival wiring, shared by both `session_start`
 * revival sites so they cannot drift. A revived fork gets `armForkRoleWiring`
 * with no owner-question callback (owner-question wiring is disconnected, so
 * no `onQuestionReport` is armed); a revived execute-worker gets the approval
 * relay pinned to the record itself, so it no longer depends on which call
 * site happens to resume it.
 */
export function reviveSessionOrphans(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry,
  orphans: PersistedOrphan[],
  onApprovalPending: ((record: RpcAgentRecord) => void) | undefined,
): RpcAgentRecord[] {
  return reviveOrphans(registry, orphans, {
    fork: (record) => armForkRoleWiring(pi, registry, record),
    executeWorker: (record) => { record.onApprovalPending = onApprovalPending; },
  });
}

export function applySessionStartAgentRetention(
  role: SpawnRole | undefined,
  root: string,
  config: GoalLoopConfig,
  recovered: PersistedOrphan[],
  prune: typeof pruneStaleAgentHomes = pruneStaleAgentHomes,
): PersistedOrphan[] {
  // Tree-root lead only (no spawn role): the prune scans every owner namespace
  // on the machine, so a fork child must not repeat it from inside a tree.
  if (role !== undefined) return recovered;
  try {
    const retention = prune(root, resolveChildRetentionTtlDays(config), { evictionCost: retentionEvictionCost });
    if (retention.deletedHomes.length === 0) return recovered;
    const deletedHomes = new Set(retention.deletedHomes);
    return recovered.filter(orphan => !orphan.ownership || !deletedHomes.has(orphan.ownership.home));
  } catch (error) {
    reportOwnershipDiagnostic("retention-start", error);
    return recovered;
  }
}

/** Shutdown's durable boundary: preserve pre-stop status, then persist the
 * same snapshots enriched from final child disk reconciliation. */
export async function persistShutdownAgentSnapshots(
  agentTools: AgentToolsHandle | undefined,
  sidecar: string | undefined,
  threads: ThreadRegistryHandle,
): Promise<void> {
  const orphans = agentTools ? captureOrphans(agentTools.rpcRegistry) : undefined;
  await agentTools?.stopAll();
  if (!sidecar || !agentTools || !orphans) return;
  for (const orphan of orphans) {
    const record = agentTools.rpcRegistry.get(orphan.agentId);
    if (!record) continue;
    orphan.telemetry = record.telemetry;
    orphan.telemetryContextFloor = record.telemetryContextFloor;
    orphan.observedModel = record.observedModel;
    orphan.observedEffort = record.observedEffort;
    orphan.observedContextTokens = record.observedContextTokens;
  }
  writeSidecarAt(sidecar, orphans);
  for (const thread of threads.threads.values()) {
    if (!thread.respondentAgentId) continue;
    const record = agentTools.rpcRegistry.get(thread.respondentAgentId);
    if (record) thread.forkResume = captureForkResume(record);
  }
  if (threads.pathRef.current) saveThreadRegistryFile(threads.pathRef.current, [...threads.threads.values()]);
}

/**
 * `260907-bug-ws-pi-deep-explore-missing-collection-tool` Phase 1: guards the
 * `session_start` seam so a `startBridge`/`registerAgentTools` failure never
 * leaves a session up with a partial/toolless surface. On failure this
 * always notifies loudly (`ctx.ui.notify(..., "error")`); for a spawned
 * child (`role !== undefined` — `worker`/`explore`/`fork`, every role with an
 * RPC-connected parent process) it ALSO calls `exitProcess` so the process
 * actually terminates, which makes the parent's `RpcClient` (already-tested
 * exit-rejection machinery, see spawner.ts's use of it) surface a real error
 * to the `ws-agent-spawn`/`explore` caller instead of a silent toolless
 * researcher. The host lead (`role === undefined`) has no RPC parent to
 * signal, so it only gets the notify + an early `return` from the caller
 * (see the `if (!bootstrap) return;` guard below) — loud, but not a crash of
 * the user's own interactive terminal.
 *
 * Kept dependency-free of any per-`session_start` closure state (no refs, no
 * `pi` beyond what `bootstrap()` itself captures), which makes this testable
 * without a full fake `ExtensionAPI`/`ExtensionContext`.
 */
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

/**
 * Replays only a missing task-fork tool's captured provider metadata. The
 * handler remains local and truthful: the parent extension is absent, so no
 * dispatch route exists in this child process.
 */
function installMissingTaskForkTools(
  pi: ExtensionAPI,
  context: ForkContext | undefined,
): { unavailableTools: string[]; error?: string } {
  if (context?.kind !== "task") return { unavailableTools: [] };
  const comparison = classifyForkRegistrations(context.registeredTools, captureRegisteredTools(pi.getActiveTools(), pi.getAllTools()));
  const structuralError = formatForkRegistrationMismatch({ ...comparison, missing: [] });
  if (structuralError) return { unavailableTools: [], error: structuralError };
  for (const tool of comparison.missing) {
    pi.registerTool({
      name: tool.name,
      label: tool.name,
      description: tool.description,
      parameters: tool.parameters as never,
      async execute() {
        throw new Error(`ws-pi-agent: unavailable fork tool "${tool.name}": its parent extension was not loaded`);
      },
    });
  }
  if (comparison.missing.length) pi.setActiveTools([...context.activeTools]);
  return { unavailableTools: comparison.missing.map((tool) => tool.name) };
}

export default async function wsPiBridgeExtension(pi: ExtensionAPI) {
  // First action of the factory, before anything this process could spawn
  // (the ws-mcp stdio client in `startBridge`, bash tools): the channel
  // bootstrap is read from the env and deleted so no descendant inherits the
  // parent's endpoint or credential. A child launched by the adapter then
  // connects and says hello here; a hello failure rejects the factory, which
  // Pi reports as a failed extension load and exits — the parent sees a
  // failed launch. No bootstrap (an interactive lead, or a Pi started from a
  // worker's shell) means no channel: readiness publishing is a no-op, and
  // with no subtree upstream there is no parent to fence nested dispatch on.
  const channelBootstrap = readAndDeleteChannelBootstrap(process.env);
  let subtreeUpstream: SubtreeUpstream | undefined;
  // The gate outlives any one connection: a reconnect hello reports the
  // cmd_id `ws-worker-exec` is still waiting on so the parent can re-ask.
  const approvalGate = new ChildApprovalGate();
  // Every hello, the first one too, carries the latest subtree snapshot and
  // any open approval wait in its resume section (distinct keys per feature).
  // The descendant-usage reporter below registers its `descendantUsage` key
  // through `provideResume` after connect; `ChildChannel.composeResume`
  // merges both sources.
  const channel = channelBootstrap ? await ChildChannel.connect(channelBootstrap, { resume: () => ({ ...(subtreeUpstream?.resume() ?? {}), ...approvalGate.resume() }) }) : undefined;
  subtreeUpstream = channel ? new SubtreeUpstream(channel) : undefined;
  if (channel) approvalGate.attach(channel);
  // One reporter per channel keeps the report sequence monotonic for the
  // whole launch; each session_start points it at that session's registry.
  // A factory re-run without a bootstrap (a session replacement that reuses
  // the cached module) keeps it. A reload re-imports this module with a fresh
  // ref and no bootstrap, so reporting stops for the rest of that launch;
  // RPC children have no reload command.
  if (channel) descendantUsageReporterRef.current = createDescendantUsageReporter(channel);
  const delegation = readDelegationPolicy();
  // Same-name wrappers preserve Pi's native schema, diff renderer, queue, and
  // result shape while the explicit policy — not tool visibility — authorizes
  // each target. A missing native delegation seam fails extension startup and
  // therefore child allocation rather than falling back to broad write tools.
  if (delegation?.write?.mode === "scoped") registerScopedWriteTools(pi, delegation.write);
  // CLI visibility alone is not authority: deferred activation may expose a
  // name later. Enforce the immutable ceiling at every actual tool call.
  pi.on("tool_call", event => {
    try { assertPolicyTool(delegation, event.toolName); }
    catch (error) { return { block: true, reason: String(error) }; }
  });
  pi.on("before_agent_start", () => {
    if (delegation && readSpawnRole(process.env) !== "fork") pi.setActiveTools(pi.getActiveTools().filter(name => delegation.tools.includes(name)));
  });
  // Filled before the bridge starts so native tool renderers are available
  // independently of async MCP startup; absent helpers retain Pi fallback.
  const toolPreviewTuiRef = createToolPreviewTuiRef();
  registerWebTools(pi, extensionEntryPath, toolPreviewTuiRef, process.env, channel);
  let handle: BridgeHandle | undefined;
  // Adapter settings (`pi.*`, adapter-config.ts) read through the live bridge
  // at each use; no bridge means every knob at its default.
  const readAdapterConfig = createWsConfigReader(() => handle ? { client: handle.client, sessionKey: handle.defaultSessionKeyRef.current } : undefined);
  let agentTools: AgentToolsHandle | undefined;
  // The manual-snapshot + guide-text half of the ws block, filled once per
  // `session_start`. The `<available_skills>` half is deliberately NOT held
  // here — see `skillsBlockCacheRef` below and `lead-bootstrap.ts`'s
  // `computeSkillsBlockCached` for why that piece is resolved live instead.
  const wsBlockBaseRef: { current: WsBlockBase | undefined } = { current: undefined };
  // Dogfood fix: the `<available_skills>` block cache, built lazily inside
  // `registerLeadBootstrap`'s `before_agent_start` handler against a LIVE
  // `pi.getCommands()` read (never a `session_start`-time snapshot — Pi
  // merges this adapter's own skills into `pi.getCommands()` AFTER
  // `session_start` returns, so a snapshot taken here would predate them,
  // same live-ref convention as `wsBlockBaseRef`/`rpcRegistryRef`). Lives at
  // this factory scope (survives across every `session_start` on this loaded
  // extension instance), but that is fine: `computeSkillsBlockCached` keys
  // its cache on the live entry-path set and rebuilds on its own whenever
  // that set changes, so no external reset is needed here.
  const skillsBlockCacheRef: { current: SkillsBlockCache | undefined } = { current: undefined };
  const effectivePromptRef: LeadPromptRef = { current: undefined };
  const sessionKeyRef: { current: string | undefined } = { current: undefined };
  // Present malformed metadata is a launch error, while no metadata remains the legacy fallback.
  let forkContextError: string | undefined;
  let deliveredFork: ReturnType<typeof readForkLaunchContext>;
  try { deliveredFork = readSpawnRole(process.env) === "fork" ? readForkLaunchContext(process.env) : undefined; }
  catch (error) { forkContextError = String(error); }
  const durableForkContextRef: { current: ForkContext | undefined } = { current: deliveredFork?.context };
  const inheritedForkPromptRef: { current: string | undefined } = { current: deliveredFork?.context?.effectiveSystemPrompt };
  // Absence keeps local prompt fallback, never a missing own-key bypass.
  let forkReady = readSpawnRole(process.env) !== "fork";
  let firstForkInput = true;
  let unavailableForkTools: string[] = [];
  let forkRegistrationError: string | undefined;
  let previousOwnKeys: string[] = [];
  // 260905 (push model): the shared RPC registry, published as a mutable ref
  // so `createApprovalRelay` — which must be constructed BEFORE
  // `registerAgentTools` creates that registry — can still read it at push
  // time for the fan-in status line every push carries.
  const rpcRegistryRef: { current: RpcAgentRegistry | undefined } = { current: undefined };
  // The lead's own session file, captured on session_start so the shutdown
  // handler (which gets a ctx of its own, but only after teardown has begun)
  // knows where to write the orphan sidecar.
  let leadSessionFile: string | undefined;
  let leadSidecarPath: string | undefined;
  // 260904 Phase 2 (side-thread question surface): one thread registry per
  // extension instance. Its in-memory map is hydrated from (and written back
  // to) a sibling file of the lead's own session file on every session_start
  // — see ask.ts's header for why that file exists at all.
  const threadHandle = createThreadRegistryHandle();
  // 260905 (live-agent widget ticket): the IO controller behind
  // `spawner.ts`'s `agentWidgetRefreshRef` — created once per TUI-lead
  // `session_start`, torn down (timer stopped, widget/status cleared) on
  // `session_shutdown`. `undefined` in every non-TUI or non-lead/fork process,
  // which is also what keeps `agentWidgetRefreshRef.current` unset there.
  let agentWidgetHandle: AgentWidgetController | undefined;
  // 260914 (pi native mailbox push): the session-bound background waiter that
  // drives `ws-mcp mailbox wait` and actively steers arriving mail into the
  // live conversation. Owner-lead only (`readSpawnRole` === undefined), started
  // after bootstrap once this session's own key exists, stopped on
  // `session_shutdown`. `undefined` in every other role/process.
  let mailboxWaiterHandle: MailboxWaiterHandle | undefined;
  // 260917 review fix: monotonic epoch guarding the async self-slug
  // resolution the arm site awaits below. Before that await existed, every
  // stop()-then-start() of `mailboxWaiterHandle` ran fully synchronously, so
  // it was always atomic relative to any other `session_start`/
  // `session_shutdown` on this same event loop. Awaiting
  // `resolveMailboxSelfSlug` opens a window where a second `session_start`
  // (a rapid double `/reload`) or a `session_shutdown` can run while an
  // earlier arm attempt is still resolving. A new arm or shutdown bumps
  // this epoch; a mismatched arm disposes its own published generation
  // unless shutdown claimed it, rather than overwriting a newer waiter.
  let mailboxWaiterEpoch = 0;
  // Unlike the waiter epoch, this starts before ANY session_start await: a
  // slow bridge bootstrap or self-slug lookup cannot install a superseded
  // bridge after a newer startup (or shutdown) has taken ownership.
  let sessionStartEpoch = 0;
  type BridgeGeneration = {
    handle: BridgeHandle;
    tools: AgentToolsHandle;
    waiter?: MailboxWaiterHandle;
    sidecar?: string;
    owner: "active" | "shutdown" | "stale";
  };
  let publishedGeneration: BridgeGeneration | undefined;
  // The footer has the same TUI lead/fork lifetime as the widget, but remains
  // a separate component: replacing the footer never touches belowEditor cards.
  const agentFooterLifecycle = createAgentFooterSessionLifecycle(async () => {
    const hostTui = await loadHostPiTui();
    return { truncateToWidth: hostTui.truncateToWidth, visibleWidth: hostTui.visibleWidth };
  });
  registerAgentFooterGitEvents(pi, agentFooterLifecycle);
  registerAgentFooterOutputEvents(pi, agentFooterLifecycle);
  pi.on("session_compact", (event) => { agentFooterLifecycle.acceptUsage(event.compactionEntry); agentFooterLifecycle.checkpoint(); });
  pi.on("session_tree", (event) => { if (event.summaryEntry) agentFooterLifecycle.acceptUsage(event.summaryEntry); });
  for (const event of ["model_select", "thinking_level_select", "session_info_changed"] as const) {
    pi.on(event, () => { agentFooterLifecycle.refresh(); });
  }
  // This event fires for both startup and /reload, so local workflow syncs
  // replace the ignored generated tree before Pi rebuilds its skill list.
  registerSkillResources(pi, pluginDir, repoRoot);

  // Read-only: lists Pi's currently scoped (or, if unscoped, all available)
  // models as `provider/id` candidates for the user to hand-copy into a
  // `config.tune agents.tier harness:pi` write (see lead-tune). No writes —
  // curation stays a ws-mcp config edit, not an adapter-owned data file.
  pi.registerCommand("ws-model-catalog-list", {
    description: "List provider/id model candidates for curating harness pi's agents.tier entries via config.tune / lead-tune.",
    handler: async (_args, ctx) => {
      const models = ctx.scopedModels.length > 0 ? ctx.scopedModels.map((sm) => sm.model) : ctx.modelRegistry.getAvailable();
      const lines = models.map((m) => `${m.provider}/${m.id}`);
      const header = ctx.scopedModels.length > 0 ? `Scoped models (${lines.length}):` : `All available models (${lines.length}):`;
      ctx.ui.notify([header, ...lines].join("\n"));
    },
  });

  // The live bridge key wins over the session_start snapshot: a later key
  // adoption on the bridge must reach the next compaction summary.
  const goalLoopHandle = registerGoalLoop(pi, {
    readConfig: readAdapterConfig,
    rpcRegistryRef,
    leadCompactGuidePath,
    sessionKeyRef: { get current() { return handle?.defaultSessionKeyRef.current ?? sessionKeyRef.current; } },
  }, toolPreviewTuiRef);
  registerLeadBootstrap(pi, wsBlockBaseRef, skillsBlockCacheRef, effectivePromptRef, inheritedForkPromptRef, sessionKeyRef);
  registerModelPrompts(pi, inheritedForkPromptRef);
  pi.on("input", (event, ctx) => {
    if (readSpawnRole(process.env) !== "fork") return undefined;
    let error = forkContextError ?? forkRegistrationError ?? (!forkReady || !handle?.defaultSessionKeyRef.current?.trim() ? "fork bootstrap is not ready: no valid own key" : undefined);
    try {
      if (!error && durableForkContextRef.current) error = compareForkRegistrations(durableForkContextRef.current.registeredTools, captureRegisteredTools(pi.getActiveTools(), pi.getAllTools()));
    } catch (cause) { error = String(cause); }
    if (error) {
      ctx.ui.notify(`ws-pi-agent: ${error}`, "error");
      return { action: "handled" };
    }
    if (firstForkInput) {
      firstForkInput = false;
      return { action: "transform", text: frameForkInput(event.text, handle!.defaultSessionKeyRef.current!, unavailableForkTools), images: event.images };
    }
    return undefined;
  });
  pi.on("before_provider_request", async (event, ctx) => {
    if (readSpawnRole(process.env) !== "fork") return undefined;
    return applyForkAffinity(event.payload, durableForkContextRef.current, await effectiveForkDescriptor(ctx, pi.getThinkingLevel()), ctx.sessionManager.getSessionId());
  });
  // 260906 Phase 1: declarative/global, same placement as registerFork/
  // registerAsk above it — a fork child re-runs session_start too and needs
  // ws-skill registered so addSkillToolIfLeadOrFork has something to
  // activate. Whether it is ever ACTIVE is that gate's job, not this call's.
  // Dogfood fix: takes only `pi` now — it reads `pi.getCommands()` live
  // inside its own `execute()`, never a ref filled at `session_start`.
  registerWsSkillTool(pi, toolPreviewTuiRef);
  // 260905 Edition: releases the child pushes that arrived while this session
  // was mid-turn, each with a status line computed at release time. Factory
  // scope (like registerGoalLoop above, never inside session_start) so a
  // /reload cannot stack duplicate agent_settled handlers.
  // Order is load-bearing: Pi runs one extension's handlers in registration
  // order, so `registerPushFlush`'s `agent_start` / `agent_settled` handlers
  // update the own-turn accounting (`ownTurnRef`) before the publish handlers
  // below send it. Registered the other way round, the settled snapshot would
  // still carry a stale owed turn and hold the parent's direct settle.
  registerPushFlush(pi, {
    delayMs: () => thenOrNow(readAdapterConfig(["settle_delay_ms"]), resolveSettleDelayMs),
    publish: () => { publishSubtree(rpcRegistryRef.current); },
  });
  for (const event of ["agent_start", "agent_settled", "tool_execution_end"] as const) {
    pi.on(event, () => { publishSubtree(rpcRegistryRef.current); });
  }
  // Whether the compact push renderers have been registered in THIS process.
  // Registration is per-process and idempotent (Pi keys renderers by
  // customType), but it costs a dynamic import, so a second session_start
  // does not repeat it.
  let pushRenderersRegistered = false;

  pi.on("session_start", async (_event, ctx) => {
    const startEpoch = ++sessionStartEpoch;
    const sessionRole = readSpawnRole(process.env);
    applySessionStartOwnershipDiagnostics(sessionRole, ctx);
    if (forkContextError) { ctx.ui.notify(forkContextError, "error"); return; }
    if (readSpawnRole(process.env) === "fork" && !durableForkContextRef.current) {
      durableForkContextRef.current = restoreForkContext(ctx.sessionManager.getEntries(), ctx.sessionManager.getSessionId());
      inheritedForkPromptRef.current = durableForkContextRef.current?.effectiveSystemPrompt;
    }
    if (readSpawnRole(process.env) === "fork") {
      forkReady = false;
      firstForkInput = true;
      unavailableForkTools = [];
      forkRegistrationError = undefined;
      previousOwnKeys = restoreForkKeys(ctx.sessionManager.getEntries(), ctx.sessionManager.getSessionId());
    }
    // 260905 Edition: hand the spawner this session's idleness accessor (the
    // same captured-ctx-per-session_start seam wsBlockBaseRef uses), so a
    // followUp push raised while this session is mid-turn is held until its
    // turn settles instead of going out with an already-stale status line.
    leadIdleRef.current = () => ctx.isIdle();
    // TUI only: replace Pi's default custom-message rendering for the six
    // push families, whose own content already opens with the family label
    // the default would print again. `registerPushMessageRenderers` now
    // always resolves `pi-tui` through `pi-tui.ts`'s `loadHostPiTui()` (see
    // that file's Addendum doc comment) rather than degrading to a no-op —
    // the `.catch()` below still guards a genuinely different failure mode
    // (a runtime rejection during teardown), not import-unavailability.
    if (!pushRenderersRegistered && ctx.mode === "tui" && isLeadOrFork(readSpawnRole(process.env))) {
      pushRenderersRegistered = true;
      void registerPushMessageRenderers(pi)
        .then((registered) => {
          pushRenderersRegistered = registered;
        })
        .catch(() => {
          // A rejection (e.g. Pi's assertActive() during teardown) must not
          // surface as an unhandled rejection nor pin the flag at `true`,
          // which would permanently skip the retry on the next session_start.
          pushRenderersRegistered = false;
        });
    }

    // Do not make native presentation depend on a connected MCP bridge.
    // `loadToolResultTuiModules` always resolves `pi-tui` through
    // `pi-tui.ts`'s `loadHostPiTui()` now (see that file's Addendum doc
    // comment).
    toolPreviewTuiRef.current = await loadToolResultTuiModules();

    // 260907 Phase 1: guard the seam so a `startBridge`/`registerAgentTools`
    // failure never falls through into a partial/toolless registration — see
    // `bootstrapOrFailLoud`'s doc comment above. `createApprovalRelay` stays
    // built BEFORE `registerAgentTools` inside the guarded closure (not
    // after, unlike registerExecuteGateway below) so it can be threaded into
    // that call too — see spawner.ts's registerAgentTools doc comment for why
    // a dormant-resumed execute-worker needs the SAME callback wired through
    // ws-agent-send's auto-resume branch, not just ws-execute's own spawn.
    const sessionBootstrap = await bootstrapOrFailLoud(ctx.ui, readSpawnRole(process.env), async () => {
      const h = await startBridge(pi, {
        launcherPath,
        pluginDir,
        runtimeJsonPath,
        cwd: ctx.cwd,
        toolPreviewTuiRef,
        ui: ctx.ui,
        forkContext: durableForkContextRef.current,
        previousOwnKeys,
        sessionEntries: ctx.sessionManager.getEntries(),
      });
      try {
        const approval = createApprovalRelay(pi, { cwd: ctx.cwd }, rpcRegistryRef);
        const tools = registerAgentTools(pi, h, { cwd: ctx.cwd, storage: createAgentStorageContext(ctx.sessionManager.getSessionId()), extensionPath: extensionEntryPath, subtreeUpstream }, approval, exploreGuidePath, toolPreviewTuiRef);
        return { handle: h, agentTools: tools, onApprovalPending: approval };
      } catch (error) {
        h.shutdown();
        throw error;
      }
    });
    if (!sessionBootstrap) return; // notified (and, for a spawned child, already exited) inside bootstrapOrFailLoud — never fall through to a partial/toolless registration.
    const generation: BridgeGeneration = { handle: sessionBootstrap.handle, tools: sessionBootstrap.agentTools, owner: "active" };
    const disposeStaleBootstrap = async (): Promise<void> => {
      // Claim synchronously. Shutdown may already own this published generation
      // and its recovery sidecar; a stale footer must never stop it first.
      if (generation.owner !== "active") return;
      generation.owner = "stale";
      if (publishedGeneration === generation) {
        publishedGeneration = undefined;
        // Reloads of the same Pi session share a sidecar PATH, so pathname
        // equality alone cannot prove this generation still owns the global.
        if (leadSidecarPath === generation.sidecar) leadSidecarPath = undefined;
      }
      if (mailboxWaiterHandle === generation.waiter) mailboxWaiterHandle = undefined;
      if (handle === generation.handle) handle = undefined;
      if (agentTools === generation.tools) agentTools = undefined;
      if (rpcRegistryRef.current === generation.tools.rpcRegistry) rpcRegistryRef.current = undefined;
      generation.waiter?.stop();
      try {
        // A published generation may already have consumed a recovery sidecar.
        // Persist before stopAll even when it was superseded without shutdown.
        await persistShutdownAgentSnapshots(generation.tools, generation.sidecar, threadHandle);
      } finally {
        try { await generation.waiter?.done; } finally { generation.handle.shutdown(); }
      }
    };
    if (startEpoch !== sessionStartEpoch) {
      await disposeStaleBootstrap();
      return;
    }
    publishedGeneration = generation;
    handle = sessionBootstrap.handle;
    sessionKeyRef.current = handle.defaultSessionKeyRef.current;
    const onApprovalPending = sessionBootstrap.onApprovalPending;
    agentTools = sessionBootstrap.agentTools;
    rpcRegistryRef.current = agentTools.rpcRegistry;
    registerExecuteGateway(pi, handle, agentTools.rpcRegistry, {
      cwd: ctx.cwd,
      executeWorkerPromptPath: executeWorkerGuidePath,
      extensionPath: extensionEntryPath,
      onApprovalPending,
      channel,
      approvalGate,
    }, toolPreviewTuiRef);

    // 260914 (pi native mailbox push): arm the session-bound mail waiter for an
    // owner lead once its own session key is known. Stop any prior waiter first
    // (a `/reload` re-runs session_start) so its subprocess never outlives the
    // bridge/client it drains through. Owner-lead only — a fork/worker/explore
    // child has no cross-session inbox worth an extra background subprocess.
    // The waiter uses `mailbox wait` purely as a block-until-mail signal and
    // drains through the bridge's connected client, admitting each envelope via
    // the shared push FIFO (`sendToLead` -> held-batch / idle-wake) exactly like
    // every other pushed system message — see mailbox-waiter.ts.
    //
    // 260917 Phase 1: before arming, resolve this session's own registered
    // named-inbox address (if any) via `mailbox.lookup_peers` through the same
    // bridge client `drainMail` uses, and pass it as `--slug` so the wait also
    // covers the owned named inbox, not just the reply-id queue. Best-effort —
    // `resolveMailboxSelfSlug` never throws — so a lookup failure just leaves
    // `selfSlug` undefined and arming falls back to reply-id-only exactly as
    // before.
    // Retention runs further below; its TTL is read now, through the bridge
    // just published, under the same superseded-start guard as the mailbox
    // lookup. Only the tree-root lead prunes, so only it reads.
    const retentionConfig = readSpawnRole(process.env) === undefined ? await readAdapterConfig(["child_retention_ttl_days"]) : {};
    if (startEpoch !== sessionStartEpoch) {
      await disposeStaleBootstrap();
      return;
    }

    mailboxWaiterHandle?.stop();
    mailboxWaiterHandle = undefined;
    const armEpoch = ++mailboxWaiterEpoch;
    // The waiter owns a child process, whose stderr must never write directly
    // to the terminal Pi's TUI owns. UI notifications are rendered through Pi's
    // TUI instead, while headless contexts retain their host-provided handling.
    const reportMailboxWaiterDiagnostic = (message: string): void => {
      ctx.ui.notify(`[ws-mailbox] ${message}`, "warning");
    };
    const mailboxSessionKey = handle.defaultSessionKeyRef.current;
    if (shouldArmMailboxWaiter(readSpawnRole(process.env), mailboxSessionKey)) {
      const mailboxHandle = handle;
      const mailboxCallTool: MailboxToolCall = (name, args) => mailboxHandle.client.callTool(name, args);
      const selfSlug = await resolveMailboxSelfSlug(mailboxCallTool, mailboxSessionKey);
      // A newer start/shutdown may have superseded this generation while
      // lookup awaited. Dispose its captured bridge AND unique bootstrap;
      // clearing only the waiter pointer would orphan both.
      if (armEpoch !== mailboxWaiterEpoch || startEpoch !== sessionStartEpoch) {
        await disposeStaleBootstrap();
        return;
      }
      try {
        // Stage once after bridge validation from this bridge's unique
        // bootstrap; overlapping reloads can neither cross-copy outputs
        // nor replace an executable held open by the old waiter on Windows.
        const staged = handle.localRuntimeBinary ? stageMailboxRuntime(handle.localRuntimeBinary) : undefined;
        // The bridge runs the launcher's installed runtime; after staging,
        // neither process needs this unique bootstrap build output.
        handle.releaseLocalBootstrap();
        const waiter = startMailboxWaiter({
          runWait: createSubprocessWait(sessionMailboxWaitOptions({
            launcherPath,
            pluginDir,
            runtimeBinary: staged?.binaryPath,
            sessionKey: mailboxSessionKey,
            slug: selfSlug,
            cwd: ctx.cwd,
            onStderr: reportMailboxWaiterDiagnostic,
          })),
          drainMail: createBridgeDrain(mailboxCallTool, mailboxSessionKey),
          admit: (envelope) => sendToLead(pi, buildMailboxPushMessage(envelope), "steer", "always"),
          onError: reportMailboxWaiterDiagnostic,
        });
        generation.waiter = staged ? attachMailboxRuntimeCleanup(waiter, staged) : waiter;
        mailboxWaiterHandle = generation.waiter;
      } catch (error) {
        reportMailboxWaiterDiagnostic(`could not stage local runtime: ${error instanceof Error ? error.message : String(error)}`);
      }
    } else {
      handle.releaseLocalBootstrap();
    }

    // 260904 Phase 1 (side-thread fork): registered declaratively/globally,
    // same pattern as registerExecuteGateway above — a fork child re-runs
    // session_start too and needs ws-fork registered so computeForkToolSurface's
    // own exclusion of it has something to exclude. Whether it is ever ACTIVE
    // is addForkToolIfLead's job, applied below via computeSessionBootstrap,
    // not this registration.
    // Owner-question wiring is disconnected: no onQuestion callback is passed,
    // here or on orphan revival below, so no fork-raised owner thread is ever
    // created. The dormant hook (`armForkRoleWiring`'s `onQuestion`,
    // `handleForkRaisedQuestion`) stays in the tree unarmed.
    registerFork(pi, handle, agentTools.rpcRegistry, { cwd: ctx.cwd, effectivePromptRef, extensionPath: extensionEntryPath }, undefined, toolPreviewTuiRef);

    // 260904 Phase 2 (owner question surface), same declarative/global
    // registration placement as registerFork above: ws-queue-question/
    // ws-withdraw-question stay registered, but no role activates them:
    // computeSessionBootstrap filters them out of the top lead's active tools.
    //
    // §5's captured-ctx staleness rule: re-capture ctx on EVERY session_start
    // (never a factory-scope ctx), and hydrate the persisted registry so
    // pending questions and dormant threads survive a lead restart. Only a
    // lead/fork session owns a thread registry — a worker/explore child has
    // none.
    threadHandle.ctxRef.current = ctx;
    const dispatchSessionFile = ctx.sessionManager.getSessionFile();
    leadSessionFile = dispatchSessionFile ?? undefined;
    const dispatchStorage = createAgentStorageContext(ctx.sessionManager.getSessionId());
    leadSidecarPath = dispatchSessionFile ? sidecarPath(dispatchSessionFile) : noSessionSidecarPath(dispatchStorage.root, dispatchStorage.ownerSessionId);
    generation.sidecar = leadSidecarPath;
    let recoveredRegistry = readAndClearSidecarAt(leadSidecarPath);
    recoveredRegistry = applySessionStartAgentRetention(readSpawnRole(process.env), dispatchStorage.root, retentionConfig, recoveredRegistry);
    if (recoveredRegistry.length > 0) reviveSessionOrphans(pi, agentTools.rpcRegistry, recoveredRegistry, onApprovalPending);
    publishSubtree(agentTools.rpcRegistry);
    // A hop with a parent reports its descendant usage. After a restart the
    // value is rebuilt here from the revived records and the checkpoint.
    const usageRegistry = agentTools.rpcRegistry;
    descendantUsageReporterRef.current?.setSource(() => descendantUsageValue(usageRegistry));
    evaluateDescendantUsage();
    if (readSpawnRole(process.env) === "worker" || readSpawnRole(process.env) === "explore") {
      const orphanPush = buildOrphanPush(recoveredRegistry);
      if (orphanPush) pi.sendMessage({ customType: "ws-agent-orphaned", content: JSON.stringify(orphanPush), display: true, details: orphanPush }, { deliverAs: "nextTurn" });
    }
    if (isLeadOrFork(readSpawnRole(process.env))) {
      const sessionFile = dispatchSessionFile;
      if (sessionFile) {
        hydrateThreadRegistry(threadHandle, threadRegistryPath(sessionFile));
        // 260905 orphan revival: a previous run of THIS lead session died (or
        // was shut down) with children still live. Read-and-delete the
        // sidecar, put each orphan back on the registry as a dormant record
        // (ws-agent-send relaunches it from the same --session file), and —
        // when any of them was cut off mid-turn — tell the lead once. Runs
        // before `registerAsk`/`registerThreadCommands` only incidentally —
        // nothing below depends on it.
        const orphans = recoveredRegistry;
        if (orphans.length > 0) {
          // Role-keyed wiring re-arm (review relay #1, I1): `spawnRole` is
          // persisted precisely so a revived FORK comes back with its fork-role
          // wiring rather than silently degrading to plain-worker behavior on
          // the next ws-agent-send (see reviveSessionOrphans).
          reviveSessionOrphans(pi, agentTools.rpcRegistry, orphans, onApprovalPending);
          // Edition: EVERY entry is re-registered above (an idle reviewer
          // must stay reachable through ws-agent-send), but only a set
          // containing cut-off work is announced — see buildOrphanPush.
          const orphanPush = buildOrphanPush(orphans);
          if (orphanPush) {
            if (readSpawnRole(process.env) === "fork") {
              // Startup custom messages bypass input; queue without triggering a
              // model turn so restored work first passes the own-key guard.
              pi.sendMessage({ customType: "ws-agent-orphaned", content: orphanPush, display: true, details: orphanPush as never }, { deliverAs: "nextTurn" });
            } else pushToLead(pi, agentTools.rpcRegistry, undefined, "ws-agent-orphaned", orphanPush, "followUp");
          }
        }
      }
      // 260905 (live-agent widget ticket): TUI-lead-only, via
      // `shouldArmAgentWidget` (review relay #1 Important #5: extracted into
      // agent-widget.ts's own pure predicate, directly unit tested, so this
      // gate is no longer only a doc comment) — mirrors the same
      // `ctx.mode === "tui"` check already used at #L249 for the push
      // renderers. The outer `isLeadOrFork` block above also runs headless
      // (hydration/orphan revival apply there too), but the widget itself
      // must not. A prior controller (a `/reload`) is stopped first so its
      // timer never outlives the registry/threads it closed over.
      const spawnRole = readSpawnRole(process.env);
      agentCostRefreshRef.current = undefined;
      if (shouldArmAgentWidget(spawnRole, ctx.mode)) {
        agentWidgetHandle?.stop();
        agentWidgetHandle = createAgentWidgetController(ctx, agentTools.rpcRegistry, threadHandle.threads, {
          ownerLead: spawnRole === undefined,
          animationEnabled: () => thenOrNow(readAdapterConfig(["agent_wait_animation"]), resolveAgentWaitAnimation),
        });
        agentWidgetRefreshRef.current = () => { agentWidgetHandle?.refresh(); };
        agentCostRefreshRef.current = () => { agentFooterLifecycle.refreshAgents(); };
        agentWidgetHandle.refresh();
      }
    }
    registerAsk(pi, threadHandle, agentTools.rpcRegistry, toolPreviewTuiRef);
    registerThreadCommands(pi, handle, agentTools.rpcRegistry, threadHandle, { cwd: ctx.cwd, extensionPath: extensionEntryPath, effectivePromptRef });
    // 260908 (subagent audit window ticket): stricter than `isLeadOrFork`
    // above — `shouldRegisterAudit` requires a true lead (no spawn-role
    // marker at all), so a fork child never registers `/audit`. Neither
    // `pi.registerCommand` nor `pi.registerShortcut` is called when the gate
    // is false (see `audit.ts`'s own doc comment).
    registerAuditCommands(pi, agentTools.rpcRegistry, readSpawnRole(process.env), ctx.mode, { cwd: ctx.cwd, extensionPath: extensionEntryPath, sessionKeyRef: handle.defaultSessionKeyRef });

    // A task fork inherits the parent's ordered callable surface. A missing
    // parent-only extension may be represented only by a metadata-identical
    // local stub; all other drift remains a pre-prompt readiness failure.
    if (readSpawnRole(process.env) === "fork") {
      try {
        const unavailable = installMissingTaskForkTools(pi, durableForkContextRef.current);
        unavailableForkTools = unavailable.unavailableTools;
        forkRegistrationError = unavailable.error;
      } catch (error) {
        forkRegistrationError = String(error);
      }
    }

    // §1/§4/260906: one pure call produces BOTH the ws block's static base
    // (manual snapshot + Pi lead guide) AND the fully reshaped lead/fork
    // tool surface — see lead-bootstrap.ts's `computeSessionBootstrap` doc
    // comment for why this replaced three separate
    // `pi.setActiveTools()`/`pi.getActiveTools()` round-trips. The
    // `<available_skills>` piece is NOT computed here (dogfood fix — see
    // this file's header and `computeSkillsBlockCached`): it is resolved
    // live inside `registerLeadBootstrap`'s `before_agent_start` handler
    // instead, against a `pi.getCommands()` read taken well after Pi's own
    // post-`session_start` skill merge. `wsBlockBase` stays `undefined` for
    // `worker`/`explore`, or when `startBridge` produced no manual snapshot
    // (degraded bootstrap) — `wsBlockBaseRef.current` is then left
    // untouched, and `computeBeforeAgentStartResult`'s own guard already
    // treats that as "no override" for every `before_agent_start` firing.
    //
    // Review cycle 1 (Minor): the guide read is gated the same way the
    // pre-260906 code gated it — `isLeadOrFork` AND a present manual
    // snapshot — rather than running for every role. `guideText` is only
    // ever consumed inside `buildWsBlock` (now called from
    // `before_agent_start`), which itself only runs when `manualSnapshot` is
    // present, so reading it for `worker`/`explore` or a degraded bootstrap
    // was pure waste with no observable effect.
    const bootstrapRole = readSpawnRole(process.env);
    let guideText = "";
    if (isLeadOrFork(bootstrapRole) && handle.manualSnapshotRef.current) {
      try {
        guideText = readFileSync(piLeadGuidePath, "utf8");
      } catch {
        // Tolerate a missing guide file (e.g. a dev -e run against a source
        // tree that hasn't copied it yet) — the manual snapshot alone is
        // still a strict improvement over no ws block at all.
      }
    }
    const bootstrap = computeSessionBootstrap({
      role: bootstrapRole,
      manualSnapshot: handle.manualSnapshotRef.current,
      guideText,
      currentActiveTools: pi.getActiveTools(),
    });
    if (bootstrap.wsBlockBase !== undefined) {
      wsBlockBaseRef.current = bootstrap.wsBlockBase;
    }
    // Review cycle 1 (Minor): gated on `isLeadOrFork` again, matching the
    // pre-260906 code — for `worker`/`explore`, `computeSessionBootstrap`
    // already returns `activeTools` unchanged, so calling `setActiveTools`
    // there was a semantic no-op that still re-ran Pi's internal
    // `_rebuildSystemPrompt` for a role that previously never took that
    // path at all.
    if (isLeadOrFork(bootstrapRole)) {
      pi.setActiveTools(bootstrap.activeTools);
    } else if (delegation) {
      pi.setActiveTools(delegation.tools.filter(name => pi.getAllTools().some(tool => tool.name === name)));
    }

    if (bootstrapRole === "fork") {
      const actual = captureRegisteredTools(pi.getActiveTools(), pi.getAllTools());
      const registrationError = durableForkContextRef.current ? compareForkRegistrations(durableForkContextRef.current.registeredTools, actual) : undefined;
      const ownKey = handle.defaultSessionKeyRef.current;
      const keyError = !ownKey?.trim() || ownKey === (durableForkContextRef.current?.parentSessionKey ?? process.env[WS_PI_PARENT_SESSION_KEY_ENV]) || previousOwnKeys.includes(ownKey) || durableForkContextRef.current?.parentSessionKeys?.includes(ownKey)
        ? "fork bootstrap did not issue a distinct current own key" : undefined;
      const readinessError = forkRegistrationError ?? registrationError ?? keyError;
      const readiness = {
        sessionId: ctx.sessionManager.getSessionId(),
        sessionPath: ctx.sessionManager.getSessionFile(),
        ownSessionKey: handle.defaultSessionKeyRef.current,
        activeTools: [...pi.getActiveTools()],
        registeredTools: actual,
        ...(readinessError ? { error: readinessError } : {}),
      };
      // Child entries, rather than parent transcript copies or launch files, own restart lifetime.
      if (durableForkContextRef.current) pi.appendEntry("ws-pi-fork-context", { sessionId: ctx.sessionManager.getSessionId(), context: durableForkContextRef.current });
      pi.appendEntry("ws-pi-fork-keys", { sessionId: ctx.sessionManager.getSessionId(), current: handle.defaultSessionKeyRef.current, previous: previousOwnKeys });
      // Stage-2 readiness: the parent validates this payload over the
      // authenticated channel (`validateForkReadiness`) before its first prompt.
      channel?.publishReadiness(FORK_READINESS_KIND, readiness);
      removeForkTransport(process.env[WS_PI_FORK_CONTEXT_ENV]);
      forkReady = !readinessError;
    }
    // Mount only after this session's tools and fork readiness are complete:
    // the host TUI import is asynchronous, and yielding earlier would let Pi
    // snapshot the active tool set before later question tools were registered.
    await applySessionStartAgentFooter(agentFooterLifecycle, bootstrapRole, ctx, agentTools.rpcRegistry, dispatchStorage);
    if (startEpoch !== sessionStartEpoch) await disposeStaleBootstrap();
  });

  pi.on("session_shutdown", async (event, _ctx) => {
    const shutdownEpoch = ++sessionStartEpoch;
    // Claim the published generation before ANY await. A startup resuming
    // from its final footer await sees this claim and cannot stop the registry
    // whose recovered sidecar shutdown must persist. A later startup may own
    // the globals by the time shutdown resumes, so only captured refs below
    // are used for teardown and persistence.
    const claimed = publishedGeneration;
    if (claimed?.owner === "active") claimed.owner = "shutdown";
    publishedGeneration = undefined;
    const claimedTools = claimed?.tools;
    const claimedWaiter = claimed?.waiter;
    const claimedSidecar = claimed?.sidecar;
    const claimedThreads: ThreadRegistryHandle = {
      threads: new Map([...threadHandle.threads].map(([key, thread]) => [key, { ...thread }])),
      ctxRef: { current: threadHandle.ctxRef.current },
      pathRef: { current: threadHandle.pathRef.current },
    };
    if (handle === claimed?.handle) handle = undefined;
    if (agentTools === claimedTools) agentTools = undefined;
    if (mailboxWaiterHandle === claimedWaiter) mailboxWaiterHandle = undefined;
    if (rpcRegistryRef.current === claimedTools?.rpcRegistry) rpcRegistryRef.current = undefined;
    if (leadSidecarPath === claimedSidecar) leadSidecarPath = undefined;
    leadSessionFile = undefined;
    mailboxWaiterEpoch++;
    claimedWaiter?.stop();
    // Session replacement (`reload`/`new`/`resume`/`fork`) re-runs this
    // factory in the same process, where the deleted bootstrap can never be
    // read again; the adapter does not drive children through it, but the
    // channel must outlive anything short of the process's own quit.
    if ((event?.reason ?? "quit") === "quit") channel?.close();
    // captureOrphans snapshots pre-stop child state, including dormant
    // records read-and-cleared from the sidecar. Persist using the claimed
    // registry/path before stopAll can mutate it, never newer globals.
    await persistShutdownAgentSnapshots(claimedTools, claimedSidecar, claimedThreads);
    if (shutdownEpoch === sessionStartEpoch) {
      descendantUsageReporterRef.current?.setSource(undefined);
      // Held inputs die with the session, exactly like the Pi followUp queue
      // they stand in for: report registries are about to be discarded and goal
      // replacement controls are intentionally volatile. The sidecar written
      // above carries child IDENTITIES forward; reports and controls are not
      // persisted (see spawner.ts's heldPushQueue).
      heldPushQueue.length = 0;
      // Review relay #1 (Minor, 260906): reset the compaction-in-flight flag
      // and both of goal-loop.ts's private markers beside the held-push queue
      // they gate — otherwise a shutdown/`/reload` that lands mid-compaction
      // leaves `leadCompactingRef` stuck `true` into the replacement session,
      // where every `followUp` push and `injectDiscussionSummary` would hold
      // forever with nothing left to release them.
      goalLoopHandle.resetCompactionStateForShutdown();
      leadIdleRef.current = undefined;
      applySessionShutdownOwnershipDiagnostics();
      agentWidgetHandle?.stop();
      agentWidgetHandle = undefined;
      applySessionShutdownAgentFooter(agentFooterLifecycle);
      agentWidgetRefreshRef.current = undefined;
      agentCostRefreshRef.current = undefined;
    }
    // The waiter owns its staged executable until close. Its captured bridge
    // is torn down only after the claimed registry is durably snapshotted.
    try { await claimedWaiter?.done; } finally { claimed?.handle.shutdown(); }
  });
}
