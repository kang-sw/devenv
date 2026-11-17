/**
 * Live execution-only engine for ws-execute/ws-approve. Public native agent,
 * explore and fork registrations are retired. The low-level lifecycle helpers
 * below remain shared/testable; runtime registries admit execution workers only.
 *
 * Historical lifecycle notes (not the current public tool surface):
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-send` /
 * `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript` /
 * `ws-report-to-lead` / `explore`.
 *
 * Phase 1 replaces the one-shot `pi --mode json -p` worker spawner with
 * persistent `RpcClient` (`--mode rpc`) children: `ws-agent-spawn` starts a
 * long-lived `pi` subprocess wired through `@earendil-works/pi-coding-agent`'s
 * `RpcClient`, `ws-agent-send` drives it (prompt for idle/dormant targets,
 * steer for streaming targets; internal callers can still queue with
 * `followUp` — see the doc comment on `sendToAgent`),
 * `ws-agent-list` reports live/idle/dormant status plus each agent's last
 * report time, and `ws-agent-stop` gracefully stops a child's process while
 * keeping its `agent_id` -> session/prompt/model mapping registered for a
 * later auto-resume (D-C). `ws-agent-continue` folds into `ws-agent-send` (an
 * id with no live `RpcClient` is "dormant," not a separate tool).
 *
 * 260905 (`260905-feat-ws-pi-push-only-child-reports` Phase 1): there is no
 * harvest tool any more. `ws-agent-wait` is DELETED outright (not deprecated),
 * along with the whole buffered-report machinery it existed to drain
 * (`pendingReports`/`idlePending`/`RpcAgentRecord.waiters`). Every child
 * signal is instead PUSHED into the owning session the instant it happens, as
 * a Pi custom message (`pi.sendMessage(..., {deliverAs, triggerTurn:true})`)
 * in one of six child families — `ws-agent-report`, `ws-agent-settled`,
 * `ws-agent-question`, `ws-agent-approval`, `ws-agent-advisory`,
 * `ws-agent-orphaned` — each carrying `details.agent_id`, its own payload, and
 * a fan-in status line (`computeRunningStatusLine`) counting only children
 * still executing autonomously. The lead therefore ends its turn
 * after dispatching work and is woken by the pushes themselves; it never
 * blocks in a wait call, so an approval request can reach it mid-flight
 * instead of queueing behind an unfinished wait turn.
 *
 * A `followUp` push raised while the owning session is mid-turn is HELD
 * (`heldPushQueue`) and released at that turn's `agent_end` as one versioned
 * `ws-push-batch` follow-up. Validated goal replacements share that FIFO as
 * typed controls: they apply before delivery and appear in the structured TUI
 * items but are omitted from model content. The batch retains FIFO prose and
 * separate structured TUI items, so Pi steering mode cannot stretch one
 * boundary snapshot across multiple assistant turns. `agent_settled` plus a counted
 * wake remains the fallback for compaction, late arrival, or rejected sends;
 * confirmed wake starts release one batch as steering before their first
 * response. `steer` pushes that were never held remain immediate, since
 * interrupting is their purpose. A child's ordinary assistant answer becomes
 * terminal only at `agent_settled`; its queue admission is independently
 * tracked so pending delivery is recoverable without being called execution.
 * After a dashboard interrupt of the run, the FIFO and every push the run
 * did not take wait for the next prompt instead (`leadInterrupt`).
 *
 * The spawn tool's `model_name` param accepts one of the four fixed tiers
 * (`small`/`medium`/`large`/`xlarge`) or a concrete Pi catalog `provider/id`.
 * Tier names resolve through ws-mcp's `config.resolve_agent`; concrete IDs
 * validate directly against the live Pi catalog and configured auth. Omitting
 * the parameter retains parent-model inheritance. The caller still passes an
 * already-rendered `system_prompt_path` — this module never renders it.
 *
 * `explore` is one persistent researcher preset over `spawnAgent`. Every
 * eligible lead, fork, or worker receives the same schema and RPC lifecycle;
 * an intent mode selects a configured tier while the delegation envelope
 * remains authoritative for child depth and tools.
 *
 * A parent's local settle is not subtree completion. Its control channel
 * carries revisioned snapshots of active descendants and queued deliveries
 * (`subtree-lifecycle.ts`), plus the child's own wake accounting. A settle is
 * held while the subtree reads waiting, and also while it does not but the
 * launch's latest snapshot says a turn is still coming: a turn owed, or a turn
 * start the parent has not yet seen on the RPC stream (`attachEventListener`).
 * The wake turn's own settle then admits its terminal, or the hold is released
 * once the child reports quiescence with no turn owed and no unseen turn start
 * (`observeChildSubtree`).
 *
 * `--tools` curation (`read-only`/`read-only-explore`/`full-worker`)
 * lives only in the in-memory `TOOL_GROUPS` table and Pi CLI flags.
 *
 * Phase 2 adds the per-agent child->lead report channel: `ws-report-to-lead`
 * (child-side, `full-worker`-only) is observed purely from the existing
 * `RpcClient.onEvent()` stream's `tool_execution_start` events — no new
 * transport (see `applyRpcEvent`'s doc comment for the full trace) — and, as
 * of 260905, pushed straight to the lead rather than buffered. Only a bounded
 * `RpcAgentRecord.reportLog` (question kind + timestamp, no text) is retained
 * for control freshness and `ws-agent-list`'s last-report time; it is not a
 * completion ledger.
 * `ws-agent-transcript` (lead-side, not in any `TOOL_GROUPS`) returns the
 * already-tracked `sessionPath` with no RPC round-trip.
 */

import { readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { capRpcClientStderr } from "./rpc-stderr-cap.ts";
import { attachFirstTaskForkCacheNotice } from "./fork-cache-notice.ts";
import type { BridgeHandle } from "./bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { formatConcreteModelWarning, formatExploreTierRefusal, formatTierWarning } from "./model-catalog.ts";
import { readSpawnRole } from "./process-role.ts";
import { ParentChannel } from "./agent-channel.ts";
import { allocateAgentHome, observeSessionWrite, removedAgentMessage, touchOwnership, updateOwnership, type AgentStorageContext } from "./agent-storage.ts";
import { attachDescendantUsage } from "./agent-usage-rollup.ts";
import { childPolicy, readDelegationPolicy, type DelegationPolicy } from "./delegation-policy.ts";
import { createWebSearch } from "./web-search.ts";
import { verifyWebReadiness } from "./web-readiness.ts";
import { beginSubtreeDispatch, installSubtreePublisher, publishSubtree, type SubtreeUpstream } from "./subtree-lifecycle.ts";
import { isRemovedAgent, persistAgentCostCheckpoint, registerAgentCostOwner } from "./agent-cost.ts";
import { observeAgentView } from "./agent-view.ts";
import { isExecutionAgent } from "./native-agent-policy.ts";
import { resolveAgentRegistryCap, truncatePromptForStorage, getAgentRpcSnapshot, refreshAgentTelemetry, resolveAgentId } from "./spawner-record.ts";
import type { RpcAgentRegistry, RpcAgentRecord, AgentStatus } from "./spawner-record.ts";
import { validateCwdOverride, prepareForkLaunch, startOwnedSessionObserver, claimLaunch, buildRpcClientOptions, awaitChannelStage, CHANNEL_HELLO_TIMEOUT_MS, validateForkReadiness, awaitForkReadiness, awaitWebReadiness, captureForkSelection, promptAgent, clearLiveState, awaitExitedChildStdout, launchStoppedError, pushSpawnFailed, issueDispatch, stampWriter, acceptWriter, clearTerminalFacts, acceptDispatch, wasSupersededByAcceptedDispatch, rollbackWriter, markAgentExited, advanceWorkGeneration, stoppedLaunches, startLivenessProbe } from "./spawner-launch.ts";
import type { RpcSpawnCtx, SpawnAgentParams, WriteScopeDiagnostic, RpcResumeCtx } from "./spawner-launch.ts";
import { assertExecutionPersistenceAvailable, executionRegistries, resolveSpawnModel, effectiveModelEffort, resolveSpawnToolGroup, childLaunchPreflights, captureChildBootstrapFailure, resolveTools, registerChildLaunchPreflight, GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME } from "./spawner-tools.ts";
import type { ToolGroup } from "./spawner-tools.ts";
import { resolveSpawnAdmission, nextGeneratedAlias, runSpawnGuards, verifyResearchSelection, applyModelEffort } from "./spawner-admission.ts";
import { observeChildSubtree, attachEventListener } from "./spawner-events.ts";
import { onDescendantUsageChanged, onAgentViewChanged, triggerAgentWidgetRefresh, triggerAgentCostRefresh, hasPendingTerminalDelivery, isOwnerHeld, pushToLead, heldPushQueue, ownTurnRef } from "./spawner-push.ts";
export { ownerNotifyRef } from "./owner-notify.ts";
export { blockExecutionPersistence, assertExecutionPersistenceAvailable, REPORT_TO_LEAD_TOOL_NAME, GATED_EXEC_TOOL_NAME, registerChildLaunchPreflight, captureChildBootstrapFailure, TOOL_GROUPS, resolveTools, BACKEND_TO_PROVIDER, resolveModelForAliasViaWsMcp, effectiveModelEffort, inheritModelFromToolCtx, storageContextFromToolCtx, resolveSpawnToolGroup } from "./spawner-tools.ts";
export type { ToolGroup, ResolveAgentCallToolClient, TierResolution, ResolvedModelInfo } from "./spawner-tools.ts";
export { refreshAgentTelemetry, resolveAgentId, REPORT_LOG_CAP, WS_PI_AGENT_REGISTRY_CAP_ENV, DEFAULT_AGENT_REGISTRY_CAP, resolveAgentRegistryCap, PROMPT_STORAGE_CAP_BYTES, truncatePromptForStorage } from "./spawner-record.ts";
export type { PendingApprovalState, RpcAgentRecord, AgentReportLogEntry, RpcAgentRegistry, AgentStatus, SpawnAgentRole } from "./spawner-record.ts";
export { reserveAgentAlias, lastActivityAt, evictForCapacity, runSpawnGuards, nextGeneratedAlias, callerDelegationPolicy, leadParentPolicy, spawnAdmission } from "./spawner-admission.ts";
export { applyRpcEvent, questionReportOutcome, observeChildSubtree, attachApprovalChannel, attachEventListener } from "./spawner-events.ts";
export type { RpcEventOutcome, ApprovalChannelHost } from "./spawner-events.ts";
export { OWNER_TERMINAL_RETRY_DELAY_MS, promptAgent, probeAgentLiveness, markAgentExited, LIVENESS_PROBE_INTERVAL_MS, startLivenessProbe, pushSpawnFailed, prepareForkLaunch, CHANNEL_HELLO_TIMEOUT_MS, CHANNEL_READINESS_TIMEOUT_MS, awaitChannelStage, validateForkReadiness, buildRpcClientOptions, recordReport, syncOwnershipProtection, startOwnedSessionObserver } from "./spawner-launch.ts";
export type { SpawnAgentParams, WriteScopeDiagnostic, RpcSpawnCtx, ChannelLaunchOptions, RpcResumeCtx } from "./spawner-launch.ts";
export { PUSH_FAMILIES, shouldPushToLead, isOwnerHeld, computeRunningStatusLine, hasRunningAgents, buildPushContent, leadIdleRef, leadCompactingRef, leadWakeStartPendingRef, markLeadInterrupted, isLeadInterrupted, ownTurnRef, resetOwnTurn, clearWakeStart, reserveWakeStart, reserveAdapterPromptStart, buildPushWakeLine, isPushWakeLine, PUSH_WAKE_BACKOFF_BASE_MS, PUSH_WAKE_BACKOFF_CAP_MS, sendToLead, sendBatchToLead, agentWidgetRefreshRef, agentDashboardRefreshRef, agentCostRefreshRef, isOwningAgentIdle, heldPushQueue, prependRecoveredPushes, enqueueHeldGoalReplacement, flushHeldPushes, registerPushFlush, hasPendingTerminalDelivery, startForkFinish, pushToLead } from "./spawner-push.ts";
export type { PushFamily, PushDeliverAs, WakeStartOptions, RawPushBatchMode, TerminalDelivery, ForkFinishOperation, HeldGoalReplacementResult, HeldGoalReplacement, HeldInput } from "./spawner-push.ts";

export const WORKER_LIFECYCLE_GUIDE = `\n\n## Execution worker lifecycle\nCarry out the assigned execution task yourself. Every shell command uses ws-worker-exec and waits for a correlated lead decision; never bypass that gate. Use ws-report-to-lead only for intermediate progress, not completion. Your ordinary assistant answer is delivered when the worker settles. Do not spawn native children or stop mid-run to ask a question: decide and record assumptions, or settle with a blocker.\n`;

export async function spawnAgent(
  registry: RpcAgentRegistry,
  ctx: RpcSpawnCtx,
  params: SpawnAgentParams,
): Promise<{ agent_id: string; alias?: string; evicted?: string; write_scopes?: WriteScopeDiagnostic }> {
  assertExecutionPersistenceAvailable(registry);
  if (executionRegistries.has(registry) && (!isExecutionAgent(ctx) || ctx.forkFrom || ctx.explicitTools || params.writeScopes)) {
    throw new Error("ws-pi-agent: native delegation is retired; this registry accepts execution workers only");
  }
  const dispatchAdmission = beginSubtreeDispatch(registry);
  const finishDispatch = typeof dispatchAdmission === "function" ? dispatchAdmission : await dispatchAdmission;
  try {
  const cwdOverride = validateCwdOverride(params.cwdOverride);
  const admission = resolveSpawnAdmission(ctx, params.writeScopes);
  const delegation = admission.policy;
  const verifiedPrompt = ctx.provenance ? Buffer.from(ctx.provenance.promptBase64, "base64") : undefined;
  // Resolve exactly once before any guard, alias transfer, eviction, UUID, or
  // session allocation. Concrete ws-agent-spawn IDs validate locally and fail
  // closed. Named tiers retain their existing resolution/refusal behavior; an
  // omitted model_name (or tier transport/parse failure) still inherits.
  const resolution = await resolveSpawnModel(ctx.client, params.modelName, ctx.inheritModel, ctx.catalog, ctx.allowConcreteModel === true);
  if (resolution.concreteRejected) {
    const rejection = formatConcreteModelWarning(resolution.concreteRejected, ctx.catalog.length === 0);
    ctx.notifyTierWarning?.(rejection);
    throw new Error(`ws-pi-agent: ws-agent-spawn rejected: ${rejection}`);
  }
  if (ctx.requireTier && (resolution.source !== "tier" || !resolution.model || resolution.failure || resolution.rejected)) {
    const refusal = formatExploreTierRefusal(params.modelName ?? "small", resolution.failure, resolution.rejected);
    ctx.notifyTierWarning?.(refusal);
    throw new Error(`ws-pi-agent: ${refusal}`);
  }
  if (!ctx.requireTier && params.modelName && resolution.rejected) {
    // Same head convention as `reserveAgentAlias`/`evictForCapacity` below,
    // regardless of which tool (`ws-agent-spawn`/`ws-fork`/`ws-execute`)
    // actually called `spawnAgent` — no per-caller head special-casing.
    const rejection = formatTierWarning(params.modelName, resolution.rejected, ctx.inheritModel, ctx.catalog.length === 0);
    ctx.notifyTierWarning?.(rejection);
    throw new Error(`ws-pi-agent: ws-agent-spawn rejected: ${rejection}`);
  }

  // 260906 Phase 2: both refusal guards above have passed, so `resolution`
  // is a genuine launch — `resolution.rejected` is guaranteed absent here.
  // `resolvedEffort` is computed once and reused below (`record.modelEffort`)
  // rather than recomputed, so the pushed line and the stored record can
  // never drift apart.
  const resolvedEffort = effectiveModelEffort(params.modelEffort, resolution.effort, resolution.source, ctx.inheritEffort);
  ctx.onModelResolved?.({
    tier: resolution.source === "tier" ? params.modelName! : resolution.source,
    model: resolution.model,
    effort: resolvedEffort,
    inherited: resolution.source === "inherit",
  });

  if (ctx.spawnRole === "explore") await createWebSearch({ packageRoot: dirname(dirname(ctx.extensionPath)) }).probe();
  const promptBody = verifiedPrompt ?? (params.systemPromptPath ? readFileSync(params.systemPromptPath) : undefined);
  if (!params.systemPromptPath && !ctx.forkContext && !ctx.forkFrom) {
    throw new Error("ws-pi-agent: systemPromptPath is required for a non-fork spawn");
  }
  if (!ctx.storage) throw new Error("ws-pi-agent: missing Pi storage context for durable child allocation");
  const storage = ctx.storage;
  const role = ctx.spawnRole ?? (ctx.forkFrom ? "fork" : resolveSpawnToolGroup(ctx.toolGroup) === "execute-worker" ? "execute-worker" : "worker");
  const modelBase = resolution.model;
  const toolGroup: ToolGroup = resolveSpawnToolGroup(ctx.toolGroup);
  const tools = ctx.spawnRole === "fork" || ctx.forkFrom ? ctx.explicitTools ?? delegation.tools.join(",") : delegation.tools.join(",");

  // The control channel is bound before the record is registered: nothing can
  // send to or stop an agent that is not in the registry yet, so the bind's
  // await opens no window in which the record reads as dormant. Its bootstrap
  // rides in the child's env; a bind failure fails the spawn before any
  // launch file or half-registered record exists.
  // Before anything is registered or bound: a stop has no record to reach
  // during a long rebuild, and a failure leaves nothing to clean up.
  const preflight = childLaunchPreflights.get(registry);
  if (preflight) await preflight();
  const channel = await ParentChannel.bind(1, ctx.channel?.bind);
  // `register` runs without an await: the alias and capacity guards and the
  // registration they protect are one atomic step, so two concurrent spawns
  // can never both pass the guards before either is registered.
  let forkLaunch: { contextPath: string; affinityId?: string } | undefined;
  const register = () => {
    const alias = params.alias ?? (ctx.aliasPrefix ? nextGeneratedAlias(registry, ctx.aliasPrefix) : undefined);
    const eviction = runSpawnGuards(registry, alias, resolveAgentRegistryCap());
    if (!eviction.ok) throw new Error(eviction.error);
    const agentId = randomUUID();
    const ownership = allocateAgentHome(storage, agentId, role, ctx.exploreMode, delegation);
    const sessionPath = ownership.sessionPath!;
    let forkSourcePath = ctx.forkFrom;
    forkLaunch = ctx.forkFrom || ctx.spawnRole === "fork" ? prepareForkLaunch(ctx.forkContext) : undefined;
    if (forkLaunch && ctx.forkSourceEntries) {
      forkSourcePath = join(dirname(forkLaunch.contextPath), "source.jsonl");
      writeFileSync(forkSourcePath, ctx.forkSourceEntries.map(entry => JSON.stringify(entry)).join("\n") + "\n", { mode: 0o600 });
    }
    let promptPath = params.systemPromptPath;
    if (promptPath && role !== "fork") {
      promptPath = join(ownership.home, "prompt.md");
      writeFileSync(promptPath, Buffer.concat([promptBody!, Buffer.from(WORKER_LIFECYCLE_GUIDE)]), { mode: 0o600 });
    }
    const record: RpcAgentRecord = {
      agentId,
      alias,
      title: params.title,
      sessionPath,
      ownership,
      systemPromptPath: promptPath,
      // Pin the relay beyond this launch's listener so /audit dormant resume
      // cannot strand a new gated command after process loss.
      ...(toolGroup === "execute-worker" ? { onApprovalPending: ctx.onApprovalPending } : {}),
      delegation,
      modelBase,
      // Explicit effort wins; "default" retains the selected source's policy.
      // This is the single fold point for spawn and dormant resume, and reuses
      // the same value already published through `onModelResolved`.
      modelEffort: resolvedEffort,
      modelTier: params.modelName,
      modelSource: resolution.source,
      cwdOverride,
      wsToolNames: ctx.wsToolNames,
      toolGroup,
      explicitTools: ctx.explicitTools,
      spawnRole: role,
      exploreMode: ctx.exploreMode,
      forkContext: ctx.forkContext,
      streaming: false,
      running: false,
      reportLog: [],
      prompt: truncatePromptForStorage(params.prompt),
      channel,
      launchGeneration: channel.generation,
    };
    registry.set(agentId, record);
    observeChildSubtree(registry, record, channel);
    attachDescendantUsage(record, channel, onDescendantUsageChanged);
    observeAgentView(record, channel, onAgentViewChanged);
    startOwnedSessionObserver(record);
    return { agentId, eviction, sessionPath, forkSourcePath, record };
  };
  let registered: ReturnType<typeof register>;
  try {
    registered = register();
  } catch (error) {
    channel.close();
    if (forkLaunch) rmSync(dirname(forkLaunch.contextPath), { recursive: true, force: true });
    throw error;
  }
  const { agentId, eviction, sessionPath, forkSourcePath, record } = registered;

  // 260905: the record is registered BEFORE `start()`, so a failure anywhere
  // in the launch sequence would otherwise leave a half-registered zombie the
  // lead's fan-in count keeps waiting on. Push `spawn-failed` and re-throw
  // unchanged — the thrown error still surfaces to the `ws-agent-spawn` caller
  // exactly as before; the push is additive, for the M/N bookkeeping.
  let client: RpcClient | undefined;
  let bootstrapFailure: ReturnType<typeof captureChildBootstrapFailure> | undefined;
  const launch = claimLaunch(record);
  try {
    client = new RpcClient(
      buildRpcClientOptions(
        cwdOverride ?? ctx.cwd,
        modelBase,
        sessionPath,
        record.systemPromptPath,
        tools,
        forkSourcePath,
        ctx.forkContext?.parentSessionKey ?? ctx.parentSessionKey,
        ctx.spawnRole === "explore" ? "explore" : undefined,
        ctx.exploreMode,
        forkLaunch,
        ctx.extensionPath,
        delegation,
        channel,
      ),
    );
    record.client = client;
    bootstrapFailure = captureChildBootstrapFailure(client);
    await client.start();
    capRpcClientStderr(client);
    await awaitChannelStage(client, channel.hello(), ctx.channel?.helloTimeoutMs ?? CHANNEL_HELLO_TIMEOUT_MS, "hello");

    if (ctx.forkFrom) {
      const state = await client.getState();
      const forkedSessionFile = state?.sessionFile;
      if (!forkedSessionFile) {
        throw new Error(
          "ws-pi-agent: fork spawn: RpcClient.getState() returned no sessionFile — cannot determine the forked session's actual path",
        );
      }
      if (forkLaunch) validateForkReadiness(await awaitForkReadiness(client, channel, ctx.channel), record, state);
    }

    // Read the already-folded record value, not params.modelEffort directly
    // — record.modelEffort is the single source of truth for what effort a
    // spawned/resumed child should receive (see effectiveModelEffort above
    // and the dormant-resume call site in sendToAgent, which reads the same
    // field).
    if (record.spawnRole === "explore") {
      await verifyResearchSelection(client, record, true);
      verifyWebReadiness(await awaitWebReadiness(client, channel, ctx.channel));
    } else {
      await applyModelEffort(client, record.modelEffort);
    }
    // Capture the immutable pre-first-prompt boundary after all selection
    // work, before prompt() can append any attributable child turn.
    try { const snapshot = await getAgentRpcSnapshot(client); refreshAgentTelemetry(record, snapshot.state, { fresh: true, stats: snapshot.stats }); } catch { delete record.observedModel; delete record.observedEffort; if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; } }
    if (forkLaunch) await captureForkSelection(client, record);
    // Invariant: attach before the first prompt. The listener's observed
    // `agent_start` count must see every child turn; a missed one would hold
    // every direct settle of this launch (`attachEventListener`).
    attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    attachFirstTaskForkCacheNotice(record, client, ctx.forkCacheNoticeOwner);
    await promptAgent(record, client, params.prompt);
    publishSubtree(registry, true);
  } catch (err) {
    clearLiveState(record, registry);
    // stop() detaches the stdout reader before anything else, so an exited
    // child's queued last notify is read first (`awaitExitedChildStdout`).
    if (!launch.stopped()) await awaitExitedChildStdout(client);
    try { await client?.stop(); } catch { /* best effort */ }
    // A stop during the launch is the single terminal: its own settle is the
    // lead-visible outcome, and the spawn call still fails, saying so.
    if (launch.stopped()) throw launchStoppedError();
    const reported = bootstrapFailure?.decorate(err) ?? err;
    pushSpawnFailed(ctx.pi, registry, record, reported);
    throw reported;
  } finally {
    bootstrapFailure?.dispose();
    launch.release();
    if (forkLaunch) rmSync(dirname(forkLaunch.contextPath), { recursive: true, force: true });
  }

  // 260905 (live-agent widget ticket): a brand-new registry member, live and
  // running from its initial prompt — the widget's first sighting of it.
  triggerAgentWidgetRefresh();
  triggerAgentCostRefresh();
  return { agent_id: agentId, alias: record.alias, evicted: eviction.evictedLabel, ...(admission.diagnostic ? { write_scopes: admission.diagnostic } : {}) };
  } finally { finishDispatch(); }
}

/**
 * The delegation policy a dormant relaunch hands its child. A record
 * whose stored policy carries no pre-minted `sessionKey` bootstraps with a
 * fresh `ferrule`, so its `parentSessionKey` is re-stamped with the
 * dispatching session's current key: the stored one may be missing (a child
 * spawned without it), pruned by key retention (which fails `ferrule` as an
 * unknown parent), or superseded by a lead re-login. Forks keep lead authority
 * with this lineage; a pre-minted key needs no parent at all.
 */
export function relaunchDelegation(record: Pick<RpcAgentRecord, "delegation" | "spawnRole">, currentKey: string | undefined): DelegationPolicy | undefined {
  const stored = record.delegation;
  if (!stored || stored.sessionKey || !currentKey) return stored;
  return { ...stored, parentSessionKey: currentKey };
}

/**
 * Delivers `message` to `agentId`, branching on locally-tracked streaming
 * state. `followUp()`/`steer()` only *enqueue*; the queue is drained solely
 * inside an *active* agent-loop run.
 * A freshly-started or freshly-resumed idle client has no active run, so
 * calling `followUp()`/`steer()` against it would silently queue a message
 * that is never delivered. So:
 *
 * - Dormant (`!record.client`): rebuild a fresh `RpcClient` against the
 *   SAME cached session/prompt/model (auto-resume, D-C), then deliver via
 *   `promptAgent()` regardless of `interrupt` — nothing is running yet to
 *   interrupt or queue behind. This is also where D-C's "auto-resumed child
 *   on the SAME ws session_key" lineage falls out for free: the reused
 *   `systemPromptPath` already has any session key spliced in by the lead's
 *   own prior `playbook.render` call, and passing it unchanged via
 *   `--append-system-prompt` on every relaunch never re-derives or
 *   duplicates it. This branch is also how an ORPHANED child from a previous
 *   lead session is revived (260905's shutdown sidecar re-registers it as a
 *   plain dormant record; `ws-agent-send` needs no special case for it).
 * - Live and idle (including the instant after this function's own
 *   auto-resume branch, or right after `spawnAgent`'s initial prompt
 *   settles): also `promptAgent()`, regardless of `interrupt`.
 * - Live and streaming: internal callers choose `steer()` or `followUp()`
 *   with `interrupt`; the lead-facing `ws-agent-send` always chooses `steer()`.
 *
 * 260905: every delivery goes through `promptAgent` (or re-marks `running`
 * for the steer/followUp branch), so the fan-in count reflects a send the
 * moment it is issued; and any rejection from the live client is treated as
 * the child having exited (`markAgentExited`) before being re-thrown.
 */
export async function sendToAgent(
  registry: RpcAgentRegistry,
  ctx: RpcResumeCtx,
  agentId: string,
  message: string,
  interrupt?: boolean,
): Promise<{ agent_id: string }> {
  // Awaited only while a busy fence is owed, keeping the dormant-resume
  // check-and-claim below in one synchronous step for a root lead.
  const dispatchAdmission = beginSubtreeDispatch(registry);
  const finishDispatch = typeof dispatchAdmission === "function" ? dispatchAdmission : await dispatchAdmission;
  try {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid through the one
  // shared helper first; an unresolvable input falls back to the original
  // string so the existing "unknown agentId" error path is unchanged.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    // A live owner drops a removed child from its registry; name the removal rather than an unknown id.
    if (isRemovedAgent(registry, resolvedId)) throw new Error(removedAgentMessage(resolvedId));
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  if (executionRegistries.has(registry) && !isExecutionAgent(record)) {
    throw new Error("ws-pi-agent: retired native agent cannot be resumed; its session history is not a daemon agent");
  }
  // A launch in flight has claimed `record.client` but may not have started
  // its process yet; a prompt there would throw and read as an exit. Wait for
  // it to reach its first prompt (or fail, leaving the record dormant), then
  // read the record's state fresh.
  while (record.launching) await record.launching;
  const parentPolicy = readDelegationPolicy();
  if (parentPolicy) {
    if (!record.delegation) throw new Error("ws-pi-agent: legacy child lacks a resumable capability envelope");
    const admitted = childPolicy(parentPolicy, record.delegation.tools, record.delegation.authority, false, undefined, record.delegation.network, record.delegation.write);
    if (admitted.depth !== record.delegation.depth || parentPolicy.maxDepth < record.delegation.maxDepth) throw new Error("ws-pi-agent: recovered child exceeds the current delegation budget");
  }

  const writer = ctx.writer ?? "lead";

  // A recorded agentId (retention or eviction removed, or is removing, its
  // home) never relaunches: its session would start empty. Checked before the
  // activity claim, and again after it, since the record is written under the
  // same claim.
  const removed = () => isRemovedAgent(registry, record.agentId, record.ownership);
  if (!record.client && removed()) throw new Error(removedAgentMessage(record.agentId));

  // Claim activity before mutating a dormant record. If retention already
  // owns the cross-process deletion claim, this resume fails closed instead
  // of launching against a home that can disappear mid-start.
  const ownershipTouched = !record.ownership || touchOwnership(record.ownership.home);
  if (!record.client && !ownershipTouched) throw new Error("ws-pi-agent: owned session home is unavailable during resume");
  if (!record.client && removed()) throw new Error(removedAgentMessage(record.agentId));

  // A real new instruction supersedes an in-memory `/done` operation. Its
  // late settle/park callbacks must not affect this replacement work.
  if (record.forkFinish && ctx.finishToken !== record.forkFinish.token) {
    record.forkFinish = undefined;
    try { record.onForkFinishComplete?.(record, "superseded by new lead work"); } catch { /* best effort */ }
  }
  // See `RpcResumeCtx.leadSend`: the lead taking over the exchange releases a
  // thread bind the owner surface will never close (the headless
  // fork-raised-question path). A coordinator closeout is lead-attributed
  // text, but not an external takeover, so it never takes this branch.
  if (ctx.leadSend && !ctx.finishToken && record.threadBound) record.threadBound = false;

  if (!record.client) {
    if (record.spawnRole === "explore") {
      if (!record.delegation?.network?.search || !record.delegation.network.fetch) throw new Error("web-search-tool-unavailable: legacy Explore lacks network authority; start a new researcher");
      await createWebSearch({ packageRoot: dirname(dirname(ctx.extensionPath)) }).probe();
      // The probe yields: an in-process capacity eviction can record this agentId meanwhile.
      if (removed()) throw new Error(removedAgentMessage(record.agentId));
    }
    const forkLaunch = record.spawnRole === "fork" ? prepareForkLaunch(record.forkContext) : undefined;
    record.launchGeneration = (record.launchGeneration ?? 0) + 1;
    const generation = record.launchGeneration;
    // 260904 Phase 1 (side-thread fork): `forkFrom` is deliberately never
    // passed here — a dormant resume (including a stopped fork) always
    // resumes via `--session record.sessionPath` (the fork's own
    // already-discovered real session file, see `spawnAgent`'s
    // `getState()` overwrite), exactly like a normal worker resume.
    // `record.explicitTools` (when set) is reused verbatim, same
    // cache-and-reuse contract as `systemPromptPath`/`modelBase`.
    let client: RpcClient | undefined;
    let channel: ParentChannel | undefined;
    let bootstrapFailure: ReturnType<typeof captureChildBootstrapFailure> | undefined;
    const finishOwner = ctx.finishToken !== undefined && record.forkFinish?.token === ctx.finishToken
      ? record.forkFinish : undefined;
    const ownsFailure = () => record.launchGeneration === generation
      && (ctx.finishToken === undefined || (record.forkFinish === finishOwner
        && finishOwner?.token === ctx.finishToken && finishOwner.generation === generation));
    // This launch belongs to the coordinator only when its exact in-memory
    // token requested the dormant resume. A later ordinary send clears that
    // coordinator instead, retaining the generation fence for replacement
    // work and stale callbacks.
    if (ctx.finishToken !== undefined && record.forkFinish?.token === ctx.finishToken) {
      record.forkFinish.generation = record.launchGeneration;
    }
    const launch = claimLaunch(record);
    try {
      // The record is claimed synchronously (`record.client`) before the bind
      // awaits, so a concurrent send sees a live record instead of launching a
      // second process against the same session, and a concurrent stop finds
      // a client to clear. `RpcClient` reads its env at `start()`, so the
      // bootstrap of the freshly bound channel is merged in afterwards.
      const options = buildRpcClientOptions(
        record.cwdOverride ?? ctx.cwd,
        record.modelBase,
        record.sessionPath,
        record.systemPromptPath,
        record.explicitTools ?? record.delegation?.tools.join(",") ?? resolveTools(record.toolGroup, record.wsToolNames),
        undefined,
        ctx.leadSessionKey ?? record.forkContext?.parentSessionKey,
        record.spawnRole === "fork" ? "fork" : record.spawnRole === "explore" ? "explore" : "worker",
        record.exploreMode,
        forkLaunch,
        ctx.extensionPath,
        relaunchDelegation(record, ctx.leadSessionKey),
      );
      client = new RpcClient(options);
      record.client = client;
      // Every relaunch binds a fresh endpoint and credential under the new
      // generation; the previous launch's channel was closed with its client.
      channel = await ParentChannel.bind(generation, ctx.channel?.bind);
      const preflight = childLaunchPreflights.get(registry);
      if (preflight) await preflight();
      // A stop (or a replacement launch) during the bind or the preflight
      // already cleared or replaced this claim: the child must not be started
      // for it.
      if (record.client !== client) throw new Error("ws-pi-agent: launch stopped before the child started");
      record.channel = channel;
      observeChildSubtree(registry, record, channel);
      attachDescendantUsage(record, channel, onDescendantUsageChanged);
      observeAgentView(record, channel, onAgentViewChanged);
      Object.assign(options.env!, channel.bootstrapEnv());
      bootstrapFailure = captureChildBootstrapFailure(client);
      await client.start();
      capRpcClientStderr(client);
      await awaitChannelStage(client, channel.hello(), ctx.channel?.helloTimeoutMs ?? CHANNEL_HELLO_TIMEOUT_MS, "hello");
      if (forkLaunch) validateForkReadiness(await awaitForkReadiness(client, channel, ctx.channel), record, await client.getState());
      if (record.spawnRole === "explore") {
        await verifyResearchSelection(client, record, false);
        verifyWebReadiness(await awaitWebReadiness(client, channel, ctx.channel));
      } else {
        await applyModelEffort(client, record.modelEffort);
      }
      try { const snapshot = await getAgentRpcSnapshot(client); refreshAgentTelemetry(record, snapshot.state, { stats: snapshot.stats }); } catch { delete record.observedModel; delete record.observedEffort; if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; } }
      if (forkLaunch) await captureForkSelection(client, record);
      // Invariant: attach before the first prompt (`promptAgent` below), as
      // the fresh spawn does: the observed `agent_start` count must see every
      // child turn of this launch.
      attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    } catch (err) {
      // Cleanup may await while a new instruction replaces this operation or
      // launch. Only its owner may clear the record; always stop our own
      // client and close our own channel (a replacement launch owns its own).
      if (ownsFailure() && record.client === client) clearLiveState(record, registry);
      channel?.close();
      if (!launch.stopped()) await awaitExitedChildStdout(client);
      try { await client?.stop(); } catch { /* best effort */ }
      // A finish-owned failure is rethrown to the coordinator's sole terminal
      // selector. Ordinary resumes retain spawn-failed; stale work gets
      // neither, and neither does a launch a stop ended (the stop's own
      // outcome is its terminal).
      const stopped = launch.stopped();
      const reported = bootstrapFailure?.decorate(err) ?? err;
      if (ownsFailure() && record.client === undefined && !finishOwner && !stopped) {
        pushSpawnFailed(ctx.pi, registry, record, reported);
      }
      launch.release();
      bootstrapFailure?.dispose();
      throw stopped ? launchStoppedError() : reported;
    } finally {
      if (forkLaunch) rmSync(dirname(forkLaunch.contextPath), { recursive: true, force: true });
    }
    try {
      // Role wiring that needs a live client (see `RpcAgentRecord.onResume`).
      // Best effort: a wiring failure must not
      // turn a routine resume into a failed send.
      try {
        record.onResume?.(record);
      } catch {
        // ignored — see above.
      }
      await promptAgent(record, client!, message, { writer });
    } catch (err) {
      // The first prompt is still part of the launch: a child whose
      // bootstrap failed after hello surfaces its exit here.
      if (!launch.stopped()) await awaitExitedChildStdout(client);
      throw bootstrapFailure?.decorate(err) ?? err;
    } finally {
      bootstrapFailure?.dispose();
      launch.release();
    }
    publishSubtree(registry, true);
    return { agent_id: record.agentId };
  }

  const live = record.client;
  const dispatchSequence = issueDispatch(record);
  let writerStamp: ReturnType<typeof stampWriter> | undefined;
  try {
    if (record.streaming) {
      writerStamp = stampWriter(record, writer, writer === "owner" ? message : undefined, writer === "lead" ? Date.now() : undefined);
      const queuedBoundary = { message };
      (record.pendingQueuedWork ??= []).push(queuedBoundary);
      try {
        if (interrupt) {
          await live.steer(message);
        } else {
          await live.followUp(message);
        }
      } catch (error) {
        const index = record.pendingQueuedWork?.indexOf(queuedBoundary) ?? -1;
        if (index >= 0) record.pendingQueuedWork!.splice(index, 1);
        if (record.pendingQueuedWork?.length === 0) record.pendingQueuedWork = undefined;
        throw error;
      }
      acceptWriter(record, writerStamp);
      writerStamp = undefined;
      // Queue admission keeps the child outstanding, but the work generation
      // changes only when Pi emits the queued user message_start. Until that
      // boundary, assistant events still belong to the preceding turn.
      record.running = true;
      clearTerminalFacts(record);
    } else {
      await promptAgent(record, live, message, { writer });
    }
    acceptDispatch(record, dispatchSequence);
  } catch (err) {
    const superseded = wasSupersededByAcceptedDispatch(record, dispatchSequence);
    if (writerStamp) rollbackWriter(record, writerStamp);
    // A superseded closeout is no longer allowed to mutate or publish against
    // the replacement task. Its old RPC rejection is deliberately ignored by
    // lifecycle bookkeeping; the replacement owns the record now.
    if (ctx.finishToken !== undefined && record.forkFinish?.token !== ctx.finishToken) throw err;
    // 260905: an in-flight request rejection is the other deterministic
    // "the child is gone" signal (`RpcClient.send()` throws once the process
    // has exited) — treat it exactly like a failed liveness probe so the lead
    // is told rather than left counting a dead agent, then re-throw so the
    // caller still sees the failure.
    if (!superseded) markAgentExited(ctx.pi, registry, record, {
      // A coordinator-owned closeout chooses its own one advisory terminal;
      // do not let the generic exited path admit a competing terminal event.
      suppressTerminal: ctx.finishToken !== undefined,
    });
    throw err;
  }
  publishSubtree(registry, true);
  return { agent_id: record.agentId };
  } finally { finishDispatch(); }
}

/**
 * Maps every registered agent to a `{agent_id, status}` pair: `"dormant"`
 * when there is no live client (stopped, resumable — D-C), else
 * `"running"`/`"idle"` from the locally-tracked streaming flag. Pure — no
 * IO, no RPC round trip — so directly unit-testable against fake records.
 *
 * 260905 adds `last_report_at` (ISO, omitted when the agent has never
 * reported): with `ws-agent-wait` gone, a lead that missed or compacted a
 * pushed report needs some way to see how long an agent has been quiet.
 * `status` deliberately keeps deriving from `streaming` rather than the new
 * `running` flag — `streaming` is event-confirmed and is the right thing to
 * DISPLAY, while `running` is the narrower, earlier-set fan-in counter.
 *
 * 260905 (alias/park/cap ticket) adds `alias`/`title` on every row (both
 * omitted when unset), and an opt-in `opts.includePrompt` that additionally
 * includes the record's stored (already head-truncated at spawn) `prompt`.
 * Off by default — a full-registry dump with every prompt inlined would be
 * needlessly large for the common "what's out there" check.
 *
 * 260905 (list-model/last-report-fidelity ticket) adds `model`: the
 * effective resolved model the child was launched with (`modelBase`, plus
 * `/<effort>` when `modelEffort` is set), omitted only when the record
 * carries no `modelBase` at all — either a sidecar written before the field
 * existed, or a fresh spawn whose `resolveModelForAliasViaWsMcp` fallback
 * (`ctx.inheritModel`, via `inheritModelFromToolCtx`) itself came back
 * `undefined` because the tool-context model was absent or malformed.
 * Because that resolver otherwise falls back to the parent's own concrete
 * model on any catalog miss, an inheriting child's `modelBase` already IS
 * the parent's model name — no separate inherited-vs-explicit flag is
 * needed. `last_report_at` now falls back to `record.lastReportAtOverride`
 * (set only by `rehydrateOrphanRecord`) when `reportLog` is empty, so a
 * revived orphan does not read as never-reported.
 */
export function listAgents(
  registry: RpcAgentRegistry,
  opts?: { includePrompt?: boolean },
): Array<{ agent_id: string; status: AgentStatus; alias?: string; title?: string; model?: string; last_report_at?: string; owner_held?: true; prompt?: string }> {
  return [...registry.entries()].map(([agentId, record]) => {
    const lastReport = record.reportLog[record.reportLog.length - 1];
    const lastReportAt = lastReport ? new Date(lastReport.at).toISOString() : record.lastReportAtOverride;
    const model = record.modelBase ? (record.modelEffort ? `${record.modelBase}/${record.modelEffort}` : record.modelBase) : undefined;
    return {
      agent_id: agentId,
      status: record.running || record.streaming
        ? "running"
        : record.waitingOnChildren
          ? "waiting-on-children"
          : hasPendingTerminalDelivery(record)
            ? "pending-delivery"
            : record.client ? "idle" : "dormant",
      ...(record.alias ? { alias: record.alias } : {}),
      ...(record.title ? { title: record.title } : {}),
      ...(model ? { model } : {}),
      ...(lastReportAt ? { last_report_at: lastReportAt } : {}),
      ...(isOwnerHeld(record) ? { owner_held: true as const } : {}),
      ...(opts?.includePrompt && record.prompt ? { prompt: record.prompt } : {}),
    };
  });
}

/**
 * Best-effort graceful stop of `agentId`'s live RPC child (`abort()` then
 * `stop()`, both best-effort — a child that already exited or never
 * finished starting must not turn a routine stop into an unhandled
 * rejection). The registry entry is NEVER deleted here — per D-C, a stopped
 * agent stays registered as dormant/resumable; `ws-agent-send` auto-resumes
 * it later via the same cached session file. Throws only when `agentId`
 * itself is unknown.
 *
 * 260905: a stop is one of the four `ws-agent-settled` reasons, so a
 * non-silent stop pushes `reason:"stopped"` — the lead asked for it, but the
 * push is what removes the agent from its fan-in count in the same place
 * every other terminal transition does. `opts.silent` suppresses that for the
 * internal stops that are not a delegation outcome at all: `ask.ts` closing a
 * discussion thread (the owner's summary is the signal, not a stop notice)
 * and `stopAll()`'s shutdown sweep (the session is going away).
 */
export async function stopAgent(
  registry: RpcAgentRegistry,
  agentId: string,
  pi?: ExtensionAPI,
  opts?: { silent?: boolean; onStopped?: (success: boolean) => void; skipCostCheckpoint?: boolean },
): Promise<{ agent_id: string }> {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  const client = record.client;
  if (!opts?.silent) {
    record.waitingOnChildren = false;
    record.pendingQueuedWork = undefined;
    advanceWorkGeneration(record);
    publishSubtree(registry);
  }
  if (record.ownership) touchOwnership(record.ownership.home);
  if (client) {
    const generation = record.launchGeneration;
    // A launch still in flight fails once its client is cleared below; this
    // mark keeps that failure from pushing `spawn-failed` on top of the stop.
    if (record.launching) stoppedLaunches.add(record.launching);
    if (record.ownership) updateOwnership(record.ownership.home, { lastActivityAt: Date.now(), liveness: { lifecycle: "stopping", running: true, observedAt: Date.now() } });
    // Review relay #1 (Important, alias/park/cap): clear live state
    // SYNCHRONOUSLY, before either await below, not after both resolve. A
    // record that still reads `client`-live during `abort()`/`stop()` is a
    // race window: a concurrent `ws-agent-send` (or overlay `ForkChannel`
    // send) sees a live-idle record and calls `promptAgent` on a client
    // that's mid-teardown, losing the turn silently once `stop()` lands.
    // Clearing here first means that same racing send instead takes
    // `sendToAgent`'s dormant-resume branch, exactly as if this record had
    // already finished parking — the automatic park path (which now runs
    // after every settle, not just on an explicit stop) makes this window
    // hot enough to close rather than accept.
    clearLiveState(record, registry);
    let stopped = true;
    try {
      await client.abort();
    } catch {
      stopped = false;
    }
    try {
      await client.stop();
    } catch {
      stopped = false;
    }
    // A new send can revive this record while the old client is stopping.
    // Never let the old stop clear the replacement's bind/final state.
    if (record.client !== undefined || record.launchGeneration !== generation) {
      return { agent_id: record.agentId };
    }
    // `message_end` can be persisted while abort/stop is in flight.  The
    // record is already synchronously dormant, so this final disk-only read
    // cannot revive a stale client or delay the stop race protection.
    refreshAgentTelemetry(record);
    if (record.ownership && record.launchGeneration === generation && !record.client) observeSessionWrite(record.ownership.home, record.sessionPath);
    // Review relay #1 (I2): a stop is a thread-close path too — the ticket
    // names "lead stop" alongside `/done`/fork final/`ws-withdraw-question`
    // (renamed by `260911` from `ws-resolve`). Releasing
    // the bind here keeps a stopped agent from carrying a latched flag into a
    // later `ws-agent-send` revival, where it would silently suppress every
    // settle push for the rest of the session.
    record.threadBound = false;
    if (record.ownership && record.launchGeneration === generation && !record.client) {
      updateOwnership(record.ownership.home, { liveness: {
        lifecycle: stopped ? "stopped" : "unknown", running: false, observedAt: Date.now(),
        threadBound: false, ownerHeld: isOwnerHeld(record), pendingQuestion: false,
        waitingOnChildren: record.waitingOnChildren, pendingDelivery: hasPendingTerminalDelivery(record),
        pendingApprovalCommandId: record.pendingApproval?.cmdId,
      } });
    }
    try { opts?.onStopped?.(stopped); } catch { /* internal observer only */ }
    if (!opts?.silent && !(record.delegation && readDelegationPolicy())) {
      // A delegated owner invoked this stop in its current turn, so the tool
      // result is the disposition. A self-generated follow-up would block
      // the owner's fresh subtree settlement.
      pushToLead(pi, registry, record, "ws-agent-settled", { reason: "stopped" }, "followUp");
    }
    // The final disk reconciliation above is the accounting boundary: refresh
    // the in-memory estimate first, then persist its one bounded checkpoint.
    triggerAgentWidgetRefresh();
    triggerAgentCostRefresh();
    if (!opts?.skipCostCheckpoint) persistAgentCostCheckpoint(registry);
  }
  publishSubtree(registry);
  return { agent_id: record.agentId };
}

/**
 * Lead-side introspection accessor (same family as `ws-agent-list`/
 * `ws-agent-stop`, not a driving/spawn tool): returns the absolute path to
 * `agentId`'s Pi session JSONL, unchanged since `spawnAgent` first computed
 * it. No RPC round-trip, no content marshalling — the lead greps the file
 * directly. Throws when `agentId` is unknown, same message convention as
 * `sendToAgent`/`stopAgent`.
 */
export function getAgentTranscriptPath(registry: RpcAgentRegistry, agentId: string): { transcript_path: string } {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  if (record.ownership && !touchOwnership(record.ownership.home)) {
    throw new Error("ws-pi-agent: history unavailable — owned session home is busy, gone, or unreadable; retry the reference");
  }
  if (record.ownership) observeSessionWrite(record.ownership.home, record.sessionPath);
  return { transcript_path: record.sessionPath };
}

// ---------------------------------------------------------------------------
// Pi tool registration.
// ---------------------------------------------------------------------------

export interface AgentToolsHandle {
  /** Graceful teardown for `session_shutdown`: awaits a best-effort
   * `client.stop()` over every live RPC-backed child. */
  stopAll(): Promise<void>;
  /** Live execution workers, including dormant recovery and approval correlation. */
  rpcRegistry: RpcAgentRegistry;
}

/**
 * Initializes the live execution-only engine used by ws-execute/ws-approve.
 * No public child-management tools are registered. Only an internal execution
 * worker registers ws-report-to-lead; its no-op ack is observed by the parent
 * RPC event stream, preserving intermediate progress separately from settlement.
 */
export function registerAgentTools(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  sessionCtx: {
    cwd: string;
    storage?: AgentStorageContext;
    extensionPath: string;
    /** This process's channel to its own parent, when it was launched by the adapter; absent for a root lead. */
    subtreeUpstream?: SubtreeUpstream;
  },
  /** Shared ws-owned native/MCP presentation seam. */
  toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef(),
): AgentToolsHandle {
  const rpcRegistry: RpcAgentRegistry = new Map();
  executionRegistries.add(rpcRegistry);
  registerAgentCostOwner(rpcRegistry, sessionCtx.storage);
  registerChildLaunchPreflight(rpcRegistry, bridge.ensureChildRuntime);
  installSubtreePublisher(rpcRegistry, sessionCtx.subtreeUpstream, () => heldPushQueue.length, () => ({ ...ownTurnRef }));
  const stopLivenessProbe = startLivenessProbe(pi, rpcRegistry);

  // Progress is an internal execution-worker facility, not a lead tool that
  // a helper (tool_search/codemode) could reactivate.
  if (readSpawnRole(process.env) === "worker" && readDelegationPolicy()?.tools.includes(GATED_EXEC_TOOL_NAME)) {
    registerWsTool(pi, {
      name: REPORT_TO_LEAD_TOOL_NAME,
      label: REPORT_TO_LEAD_TOOL_NAME,
      description:
        "Surface an async status update or intermediate finding to the lead immediately, distinct from your final answer. It is delivered to the lead the moment you call this — there is nothing to wait for on either side.",
      parameters: {
        type: "object",
        properties: {
          message: { type: "string", description: "Status update or intermediate finding to surface to the lead immediately. Final output is the ordinary assistant answer delivered when the agent settles." },
        },
        required: ["message"],
      } as never,
      async execute() {
        return { content: [{ type: "text", text: "reported" }] };
      },
    }, toolPreviewTuiRef);
  }

  return {
    rpcRegistry,
    async stopAll(): Promise<void> {
      stopLivenessProbe();
      for (const record of rpcRegistry.values()) record.ownershipObserverStop?.();
      // Silent by construction: the session itself is going away, so a
      // per-agent "stopped" push would have nowhere to land. Routed through
      // stopAgent so shutdown leaves records in the same resting shape every
      // other stop does (the sidecar snapshot, index.ts, is taken BEFORE this
      // runs, while the records are still marked live).
      const rpcStops = [...rpcRegistry.keys()].map((agentId) => stopAgent(rpcRegistry, agentId, pi, { silent: true, skipCostCheckpoint: true }).catch(() => undefined));
      await Promise.allSettled(rpcStops);
    },
  };
}
