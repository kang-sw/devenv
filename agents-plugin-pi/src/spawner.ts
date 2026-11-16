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
import { countBackgroundTasks } from "./background-tasks.ts";
import { isLeadOrFork, readSpawnRole } from "./process-role.ts";
import { ParentChannel } from "./agent-channel.ts";
import { allocateAgentHome, observeSessionWrite, removedAgentMessage, touchOwnership, updateOwnership, type AgentStorageContext } from "./agent-storage.ts";
import { ownerNotifyRef } from "./owner-notify.ts";
import { attachDescendantUsage, evaluateDescendantUsage } from "./agent-usage-rollup.ts";
import { childPolicy, readDelegationPolicy, type DelegationPolicy } from "./delegation-policy.ts";
import { createWebSearch } from "./web-search.ts";
import { verifyWebReadiness } from "./web-readiness.ts";
import { beginSubtreeDispatch, installSubtreePublisher, publishSubtree, type OwnTurnState, type SubtreeUpstream } from "./subtree-lifecycle.ts";
import { GOAL_CONTROL_CUSTOM_TYPE, PUSH_BATCH_CUSTOM_TYPE, PUSH_BATCH_VERSION, type PushBatchItem, type PushBatchItemState } from "./push-protocol.ts";
import { labelAdapterContent, labelAdapterText } from "./adapter-label.ts";
import { withItemSummaryIds, withSummaryId } from "./summary-id.ts";
import { isRemovedAgent, persistAgentCostCheckpoint, registerAgentCostOwner } from "./agent-cost.ts";
import { observeAgentView, scheduleAgentViewPublish } from "./agent-view.ts";
import { isExecutionAgent } from "./native-agent-policy.ts";
import { resolveAgentRegistryCap, truncatePromptForStorage, getAgentRpcSnapshot, refreshAgentTelemetry, resolveAgentId } from "./spawner-record.ts";
import type { RpcAgentRecord, RpcAgentRegistry, AgentReportLogEntry, AgentStatus } from "./spawner-record.ts";
import { syncOwnershipProtection, claimLeadOwnership, OWNER_TERMINAL_RETRY_DELAY_MS, validateCwdOverride, prepareForkLaunch, startOwnedSessionObserver, claimLaunch, buildRpcClientOptions, awaitChannelStage, CHANNEL_HELLO_TIMEOUT_MS, validateForkReadiness, awaitForkReadiness, awaitWebReadiness, captureForkSelection, promptAgent, clearLiveState, awaitExitedChildStdout, launchStoppedError, pushSpawnFailed, issueDispatch, stampWriter, acceptWriter, clearTerminalFacts, acceptDispatch, wasSupersededByAcceptedDispatch, rollbackWriter, markAgentExited, advanceWorkGeneration, stoppedLaunches, startLivenessProbe } from "./spawner-launch.ts";
import type { RpcResumeCtx, RpcSpawnCtx, SpawnAgentParams, WriteScopeDiagnostic } from "./spawner-launch.ts";
import { harvestLastMessage, observeChildSubtree, attachEventListener } from "./spawner-events.ts";
import { assertExecutionPersistenceAvailable, executionRegistries, resolveSpawnModel, effectiveModelEffort, resolveSpawnToolGroup, childLaunchPreflights, captureChildBootstrapFailure, resolveTools, registerChildLaunchPreflight, GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME } from "./spawner-tools.ts";
import type { ToolGroup } from "./spawner-tools.ts";
import { resolveSpawnAdmission, nextGeneratedAlias, runSpawnGuards, verifyResearchSelection, applyModelEffort } from "./spawner-admission.ts";
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

/**
 * 260905: the push families. Each is a Pi custom-message `customType`
 * delivered by `pushToLead` into whichever session owns the child:
 *
 * - `ws-agent-report` — a `ws-report-to-lead` progress update or intermediate
 *   finding, pushed at once and never treated as terminal.
 * - `ws-agent-settled` — the child stopped producing: `reason` is `"idle"`
 *   (ordinary terminal settlement carrying `last_message`),
 *   `"stopped"` (an explicit `ws-agent-stop`), `"exited"` (its process died —
 *   see the liveness probe), or `"spawn-failed"`.
 * - `ws-agent-question` — dormant: formerly a headless `kind:"question"`
 *   report the lead itself had to answer. `applyRpcEvent` no longer emits it
 *   (only the uncalled `questionReportOutcome` does); the family and its
 *   hold handling stay registered.
 * - `ws-agent-approval` — an `execute-worker` is blocked on `ws-approve`.
 * - `ws-agent-advisory` — the adapter's own statement about a child. The
 *   `fork-question-thread` advisory (`followUp`) is dormant: only the uncalled
 *   `questionReportOutcome` emits it.
 * - `ws-agent-orphaned` — children that outlived their lead session and are
 *   revivable with `ws-agent-send` (shutdown sidecar, `agent-sidecar.ts`).
 * - `ws-background-task` (261016) — a background run of the lead's one-liner
 *   exec hatch finished on its own (exit or timeout); carries no agent.
 *   Being a family is what lets the ws-compact lever recovery and the push
 *   renderers cover it.
 */
export const PUSH_FAMILIES = [
  "ws-agent-report",
  "ws-agent-settled",
  "ws-agent-question",
  "ws-agent-approval",
  "ws-agent-advisory",
  "ws-agent-orphaned",
  "ws-background-task",
] as const;

export type PushFamily = (typeof PUSH_FAMILIES)[number];

/** Pi's own `sendMessage` delivery axis (`ExtensionAPI.sendMessage`'s `options.deliverAs`). */
export type PushDeliverAs = "steer" | "followUp" | "nextTurn";

/**
 * Every eligible dispatcher injects its direct children's events into its own
 * session. Its own reports separately travel outward over its parent RPC edge.
 */
export function shouldPushToLead(env: NodeJS.ProcessEnv = process.env): boolean {
  const role = readSpawnRole(env);
  return isLeadOrFork(role) || role === "worker" || role === "explore";
}

/**
 * The shared registry walk behind both `computeRunningStatusLine` (below) and
 * the goal-loop yield predicate (`hasRunningAgents`, 260905 Phase 2): skips
 * owner-routed records and reports whether anything counts as "present"
 * (any lead-owned registry member — dormant/parked included) plus how many
 * turns can still make autonomous progress. Delivery and descendant waits
 * are intentionally not execution.
 */
export function isOwnerHeld(record: Pick<RpcAgentRecord, "lastWriter"> | undefined): boolean {
  return record?.lastWriter === "owner";
}

function computeFanIn(registry: RpcAgentRegistry | undefined): { present: boolean; running: number } {
  let present = false;
  let running = 0;
  for (const record of registry?.values() ?? []) {
    if (record.threadBound || isOwnerHeld(record)) continue;
    present = true;
    if (record.running || record.streaming) running += 1;
  }
  return { present, running };
}

/**
 * The fan-in status line every pushed message carries: `N delegated agents
 * still running`, computed fresh at push time over the shared registry.
 *
 * - The line is PRESENT whenever the registry holds any member that is NOT
 *   `threadBound` — 260905 (alias/park/cap ticket) keys this on registry
 *   membership, not on a live client: a dormant/parked record still counts as
 *   present, since automatic parking (see the settle handler below) now
 *   routinely turns a settled, non-threadBound child dormant. Persistent
 *   researchers are members of this same registry.
 * - N counts only turns still executing (`running || streaming`). A pending
 *   terminal delivery, descendant wait, or owner action is not execution.
 *   `0 delegated agents still running` is the lead's synthesis cue —
 *   and, per the presence rule above, `0 …` stays visible (not omitted) as
 *   long as any non-threadBound record — dormant included — remains
 *   registered; cap eviction is what eventually removes it.
 *
 * Owner decision after the second live run (2026-09-05): the former
 * running-out-of-total form with an id suffix is gone. The total only ever
 * grew across a session (an idle child keeps its process, so it stayed counted
 * until stopped/exited) and the id suffix duplicated what `ws-agent-list`
 * already answers, so both were noise. The omission rule survives unchanged:
 * when nothing at all is registered (or every member is threadBound) there is
 * no line at all (`undefined`), so a push that has nothing to do with
 * delegation fan-in — a `ws-agent-orphaned` roll-call at session start, a
 * `spawn-failed` for the only child — never ends with a contentless zero line.
 *
 * 261016: the lead's running background tasks (`background-tasks.ts`) extend
 * the line as `N delegated agents, M background tasks, L monitors still
 * running`. M counts timed tasks, L monitors (no timeout); each segment
 * appears only when nonzero, while the delegated-agents segment keeps the
 * presence rule above, and the line starts at the first present segment.
 * Sessions that never run a background task see the line unchanged.
 */
export function computeRunningStatusLine(
  registry: RpcAgentRegistry | undefined,
  background: { timed: number; monitors: number } = countBackgroundTasks(),
): string | undefined {
  const { present, running } = computeFanIn(registry);
  const segments = [
    ...(present ? [`${running} delegated agent${running === 1 ? "" : "s"}`] : []),
    ...(background.timed > 0 ? [`${background.timed} background task${background.timed === 1 ? "" : "s"}`] : []),
    ...(background.monitors > 0 ? [`${background.monitors} monitor${background.monitors === 1 ? "" : "s"}`] : []),
  ];
  if (segments.length === 0) return undefined;
  return `${segments.join(", ")} still running`;
}

/**
 * Phase 2 (260905) goal-loop yield predicate: true when N > 0 under the
 * same fan-in walk computeRunningStatusLine uses, so the yield decision
 * can never drift from the pushed status line's own arithmetic.
 */
export function hasRunningAgents(registry: RpcAgentRegistry | undefined): boolean {
  return computeFanIn(registry).running > 0;
}

/**
 * Renders a pushed message's human-readable `content`. `details` carries the
 * same fields structurally (that is what a renderer/tool would read); this
 * body is what the lead's model actually sees in its transcript, so it stays
 * plain text with the status line last. An absent status (nothing delegated,
 * see `computeRunningStatusLine`) contributes no line at all.
 */
export function buildPushContent(
  family: PushFamily,
  agentId: string | undefined,
  payload: Record<string, unknown>,
  status: string | undefined,
): string {
  const head = agentId ? `[${family}] agent ${agentId}` : `[${family}]`;
  // Orphan recovery keeps its tool-facing fields in `details`, but its model
  // copy is an action block rather than YAML-like metadata. In particular,
  // `idle_agent_ids` remains structured without enumerating dormant IDs in
  // prose. Other orphan-shaped payloads retain the ordinary generic rendering.
  const body = family === "ws-agent-orphaned" && typeof payload.agents === "string"
    ? payload.agents.split("\n")
    : Object.entries(payload)
      .filter(([, value]) => value !== undefined && value !== null && value !== "")
      .map(([key, value]) => `${key}: ${typeof value === "string" ? value : JSON.stringify(value)}`);
  return [head, ...body, ...(status ? [status] : [])].join("\n");
}

/** Live session idleness accessor, supplied at session_start. Missing or stale accessors never authorize a custom turn start. */
export const leadIdleRef: { current: (() => boolean) | undefined } = { current: undefined };

/** Independent compaction hold, set before any compaction and cleared only by deferred completion/failure or lever callbacks. agent_start is not proof of release. */
export const leadCompactingRef: { current: boolean } = { current: false };

/** Shared reservation for a push or reminder user prompt awaiting agent_start. All pushes stay held until confirmed start, settle, or recovery timeout clears this reservation. */
export const leadWakeStartPendingRef: { current: boolean } = { current: false };

/**
 * A dashboard interrupt of this session's own run: the user asked the
 * dashboard page to stop it. The mailbox waiter raises the mark with
 * `markLeadInterrupted` just before it asks Pi to abort the run, and the
 * next own `agent_start` clears it. Pi's interactive abort clears its
 * queues: untaken text steers move into the terminal editor, and queued
 * custom messages, every push this module steered into the run included,
 * are dropped. So until the interrupted run settles (`runOpen`), pushes are
 * held rather than steered, and neither the `agent_end` boundary batch nor a
 * push wake starts a turn. At that settle the pushes the run was not seen
 * taking (`untakenPushes`) and then the held FIFO are queued with `nextTurn`,
 * so they reach the model with the next prompt instead of starting a turn.
 * Pushes admitted after that settle behave as usual.
 *
 * Ordering limits. `nextTurn` items ride only on a `prompt()` turn, placed
 * after its own message: terminal input, a dashboard submission, or this
 * module's push wake line, so mail arriving while idle after the interrupt
 * carries them into its wake turn. A turn a custom message starts on its own
 * runs without them, and they wait for the next prompt; a session reload or
 * `/new` drops them. A non-TUI Pi mode has no abort handler: its abort keeps
 * the queues, and Pi's own continuation starts a new run from them (which
 * also clears the mark), so this hold does not apply there.
 */
let leadInterrupt: { runOpen: boolean } | undefined;

/** Raise the interrupted mark for this session's running run; see `leadInterrupt`. */
export function markLeadInterrupted(): void {
  leadInterrupt = { runOpen: true };
}

/** Whether the interrupted mark stands: from `markLeadInterrupted` until the next own `agent_start`. */
export function isLeadInterrupted(): boolean {
  return leadInterrupt !== undefined;
}

/** Pushes are held for `nextTurn` until the interrupted run settles. */
function interruptHolding(): boolean {
  return leadInterrupt?.runOpen === true;
}

type LeadCustomMessage = Parameters<ExtensionAPI["sendMessage"]>[0];

/**
 * Custom messages sent as a steer or follow-up that no `message_start` has
 * shown the lead taking yet, in send order. Pi emits `message_start` with
 * the very object it queued, whose `details` is the one passed to
 * `sendMessage`, when the run injects it; so a message still listed when an
 * interrupt clears Pi's queues was dropped. Emptied at every own settle:
 * what a run leaves queued past its settle is not this module's to recover.
 */
const untakenPushes: LeadCustomMessage[] = [];

/** Send a push into the lead as a steer or follow-up, tracked until the run takes it. */
function sendIntoRun(pi: ExtensionAPI, message: LeadCustomMessage, deliverAs: PushDeliverAs): void {
  pi.sendMessage(message, { deliverAs, triggerTurn: true });
  if (deliverAs !== "nextTurn" && message.details !== undefined) untakenPushes.push(message);
}

function forgetTakenPush(message: unknown): void {
  const taken = message as { role?: unknown; details?: unknown } | undefined;
  if (taken?.role !== "custom" || taken.details === undefined) return;
  const index = untakenPushes.findIndex((sent) => sent.details === taken.details);
  if (index >= 0) untakenPushes.splice(index, 1);
}

/**
 * The interrupted run settled: re-send whole, in send order, every push it
 * did not take, then the held FIFO as one batch, all with `nextTurn`.
 */
function releaseAfterInterrupt(pi: ExtensionAPI): void {
  if (leadInterrupt) leadInterrupt.runOpen = false;
  // Re-sent with no terminal bookkeeping: the first send already advanced it.
  for (const message of untakenPushes.splice(0)) {
    try {
      pi.sendMessage(message, { deliverAs: "nextTurn" });
    } catch {
      // A torn-down session: nothing left to queue into.
    }
  }
  submitHeldPushBatch(pi, "nextTurn");
}

/**
 * This process's own-turn accounting, reported upstream in every subtree
 * snapshot (`installSubtreePublisher`) so the parent can release a held settle
 * without guessing whether a wake turn is coming. One process is one launch
 * generation, so the counter never resets; nothing else may write it (tests
 * reset it, with the flags `owed` derives from, through `resetOwnTurn`).
 *
 * - `owed`: a delivery this process already enqueued still has a turn coming.
 *   Two sources, folded by `syncOwnTurnOwed`: an outstanding push wake
 *   reservation (`requestPushWake`), which owes exactly as long as the
 *   reservation lives; and an `agent_end` boundary batch that Pi continues
 *   with, which owes until the next own `agent_start` or the raw
 *   `agent_settled` (by then Pi has started or abandoned its continuation).
 *   The next own `agent_start` clears both, together with the `started`
 *   increment, before that handler's flush publishes. A steer or followUp
 *   consumed inside a running loop starts no new turn and never sets it. Each
 *   flip happens before a publish that already follows it (`pushToLead`, the
 *   batch's `afterEnqueue`, the `agent_settled` publish), so it adds no send
 *   of its own; a reservation that lapses on its timeout publishes itself.
 * - `started`: own `agent_start` events seen by this process.
 */
export const ownTurnRef: OwnTurnState = { owed: false, started: 0 };

let boundaryTurnOwed = false;

let pushWakeReserved = false;

function syncOwnTurnOwed(): void {
  ownTurnRef.owed = boundaryTurnOwed || pushWakeReserved;
}

/** Test isolation only: one process never resets its own-turn accounting. */
export function resetOwnTurn(): void {
  boundaryTurnOwed = false;
  pushWakeReserved = false;
  ownTurnRef.started = 0;
  syncOwnTurnOwed();
}

export interface WakeStartOptions {
  /** Recovery delay, read at each reservation; may resolve asynchronously (an adapter setting read through ws-mcp). */
  delayMs: () => number | Promise<number>;
  scheduleTimer?: (cb: () => void, ms: number) => NodeJS.Timeout;
  clearTimer?: (handle: NodeJS.Timeout) => void;
}

let wakeOptions: WakeStartOptions | undefined;

let cancelWakeTimeout: (() => void) | undefined;

/** Publishes this process's subtree snapshot; set by `registerPushFlush`, used where an `owed` flip has no publish of its own. */
let publishOwnTurn: (() => void) | undefined;

export function clearWakeStart(): void {
  cancelWakeTimeout?.();
  cancelWakeTimeout = undefined;
  leadWakeStartPendingRef.current = false;
  pushWakeReserved = false;
  syncOwnTurnOwed();
}

/** Reserve before prompt preflight; recovery exists even when dispatch throws. */
export function reserveWakeStart(options: WakeStartOptions, onTimeout: () => void): boolean {
  if (leadWakeStartPendingRef.current || !shouldPushToLead()) return false;
  leadWakeStartPendingRef.current = true;
  const schedule = options.scheduleTimer ?? ((cb, ms) => {
    const timer = setTimeout(cb, ms);
    timer.unref?.();
    return timer;
  });
  // The reservation is taken synchronously; only the recovery timer waits on
  // the delay. A cancel that lands while an asynchronous delay is still
  // being read prevents the timer from ever being scheduled.
  let cancelled = false;
  let handle: NodeJS.Timeout | undefined;
  const arm = (ms: number): void => {
    if (cancelled) return;
    handle = schedule(() => {
      cancelWakeTimeout = undefined;
      leadWakeStartPendingRef.current = false;
      // The reservation ended without a turn starting: it no longer owes one.
      pushWakeReserved = false;
      syncOwnTurnOwed();
      onTimeout();
    }, ms);
  };
  cancelWakeTimeout = () => {
    cancelled = true;
    if (handle !== undefined) (options.clearTimer ?? clearTimeout)(handle);
  };
  const delay = options.delayMs();
  // A rejected delay read recovers at once rather than leaving the reservation unrecoverable.
  if (delay instanceof Promise) delay.then(arm, () => arm(0));
  else arm(delay);
  return true;
}

/**
 * Reserve the shared start for an adapter-issued idle user prompt other than a
 * push wake, so pushes and the goal reminder stay held until it starts. The
 * recovery clock is the push lifecycle's; with no push lifecycle registered
 * there are no pushes to hold and nothing is reserved.
 */
export function reserveAdapterPromptStart(onTimeout: () => void): boolean {
  return wakeOptions !== undefined && reserveWakeStart(wakeOptions, onTimeout);
}

/** The idle push-wake user line, sent under the adapter label. Lead compaction (lead-compaction.ts) recognizes the label; `isPushWakeLine` stays for unlabeled entries recorded before it. */
export function buildPushWakeLine(count: number): string {
  return `${count} ws messages waiting; process the incoming reports.`;
}

/** True for exactly the text `buildPushWakeLine` produces, without the label. */
export function isPushWakeLine(text: string): boolean {
  return /^\d+ ws messages waiting; process the incoming reports\.$/.test(text);
}

/** First retry delay after a failed push wake; doubles per consecutive failure up to the cap. */
export const PUSH_WAKE_BACKOFF_BASE_MS = 1_000;

export const PUSH_WAKE_BACKOFF_CAP_MS = 30_000;

/**
 * Failed-wake breaker. A wake whose prompt reaches `agent_settled` without an
 * intervening `agent_start` failed: the host reported idle but rejected the
 * prompt (e.g. an inner run it still considers active). Its settle must not
 * wake again at once, or settle -> wake -> rejected prompt -> settle recurses
 * without ever yielding to the event loop. Consecutive failures back off
 * exponentially; any own `agent_start` resets the breaker. The held queue is
 * never touched here, so delivery resumes at the next real run.
 */
let pushWakeAwaitingStart = false;

let failedPushWakes = 0;

let settleWakeHandle: NodeJS.Timeout | undefined;

let cancelSettleWake: (() => void) | undefined;

function resetPushWakeBreaker(): void {
  cancelSettleWake?.();
  cancelSettleWake = undefined;
  settleWakeHandle = undefined;
  pushWakeAwaitingStart = false;
  failedPushWakes = 0;
}

/** Wake retries after a failed wake wait for the backoff timer; every other path stays immediate. */
function pushWakeBackingOff(): boolean {
  return failedPushWakes > 0 && settleWakeHandle !== undefined;
}

/**
 * Runs the settle-time flush on a macrotask, never inside Pi's settle
 * emission: a wake sent from there is awaited by that emission, so a prompt
 * that settles again without starting would re-enter this handler in the
 * same microtask chain. The deferred flush re-reads every guard when it fires.
 */
function scheduleSettleWake(pi: ExtensionAPI, delayMs: number): void {
  const options = wakeOptions;
  if (!options) return;
  cancelSettleWake?.();
  const schedule = options.scheduleTimer ?? ((cb, ms) => {
    const timer = setTimeout(cb, ms);
    timer.unref?.();
    return timer;
  });
  const handle = schedule(() => {
    if (settleWakeHandle !== handle) return;
    settleWakeHandle = undefined;
    cancelSettleWake = undefined;
    if (wakeOptions !== options) return;
    flushHeldPushes(pi);
    // The flush may reserve a wake (`owed` flips) or apply a control-only
    // queue; neither has a publish of its own here.
    publishOwnTurn?.();
  }, delayMs);
  settleWakeHandle = handle;
  cancelSettleWake = () => (options.clearTimer ?? clearTimeout)(handle);
}

function requestPushWake(pi: ExtensionAPI): void {
  if (interruptHolding() || !wakeOptions || !heldPushQueue.length || !leadIdleRef.current || !isOwningAgentIdle() || pushWakeBackingOff()) return;
  // A lapsed reservation's retry may exit early, leaving `owed` false with no
  // publish of its own; the parent's held settle waits on that flip.
  if (!reserveWakeStart(wakeOptions, () => { requestPushWake(pi); publishOwnTurn?.(); })) return;
  pushWakeReserved = true;
  syncOwnTurnOwed();
  pushWakeAwaitingStart = true;
  try {
    pi.sendUserMessage(labelAdapterText(buildPushWakeLine(heldPushQueue.length)), { deliverAs: "followUp" });
  } catch {
    // Keep the queue and timeout: handled input and rejected preflight need the same retry.
  }
}

export type RawPushBatchMode = "when-held" | "always";

/**
 * A raw summary as sent on its own: the label opens its content and a fresh
 * display-summary row id joins its details. Held raw sends keep the unlabeled
 * message, since a batch carries the label once and stamps each item itself.
 */
function labelRawSend(message: Parameters<ExtensionAPI["sendMessage"]>[0]): Parameters<ExtensionAPI["sendMessage"]>[0] {
  return { ...message, content: labelAdapterContent(message.content), details: withSummaryId(message.details) as never };
}

/** Admit raw summaries through the very same FIFO as family-shaped reports. */
export function sendToLead(
  pi: ExtensionAPI,
  message: Parameters<ExtensionAPI["sendMessage"]>[0],
  deliverAs: PushDeliverAs,
  batchMode: RawPushBatchMode = "when-held",
): void {
  if (!shouldPushToLead() || !leadIdleRef.current) return;
  admitPush(pi, { kind: "raw", deliverAs, message, batchMode });
}

/**
 * Admit several raw messages as one FIFO unit: held or submitted together, so
 * a busy lead receives them as one `ws-push-batch` steer. One steer each would
 * land one per model call under Pi's default `one-at-a-time` steering queue.
 */
export function sendBatchToLead(
  pi: ExtensionAPI,
  messages: readonly Parameters<ExtensionAPI["sendMessage"]>[0][],
  deliverAs: PushDeliverAs,
): void {
  if (messages.length === 0 || !shouldPushToLead() || !leadIdleRef.current) return;
  const [head, ...tail] = messages.map((message): HeldRawSend => ({ kind: "raw", deliverAs, message, batchMode: "always" }));
  admitPush(pi, head!, tail);
}

/** `batchTail` rides behind an always-batch raw head as one unit; every other caller admits a single item. */
function admitPush(pi: ExtensionAPI, held: HeldPush | HeldRawSend, batchTail: readonly HeldRawSend[] = []): void {
  try {
    leadIdleRef.current?.();
  } catch {
    return; // stale session accessor
  }
  if (leadCompactingRef.current || leadWakeStartPendingRef.current || isOwningAgentIdle() || held.deliverAs === "followUp" || heldPushQueue.length > 0 || interruptHolding()) {
    if (held.kind === "push") {
      if (held.terminal) held.terminal.state = "held";
      held.actionGeneration = held.record?.workGeneration;
      if (held.family === "ws-agent-question") held.questionReport = held.record?.reportLog.at(-1);
      if (held.family === "ws-agent-approval") held.approvalIssue = held.record?.pendingApproval?.issue ?? 0;
    }
    heldPushQueue.push(held, ...batchTail);
    requestPushWake(pi);
  } else if (held.kind === "raw") {
    if (held.batchMode === "always") {
      // Queue-then-submit preserves shared batch materialization for callers
      // that require it without changing direct delivery for other raw families.
      heldPushQueue.push(held, ...batchTail);
      submitHeldPushBatch(pi, held.deliverAs === "steer" ? "steer" : "followUp");
    } else {
      sendIntoRun(pi, labelRawSend(held.message), held.deliverAs);
    }
  } else {
    sendPush(pi, held.registry, held.record, held.family, held.payload, held.deliverAs, held.terminal);
  }
}

/**
 * 260905 (live-agent widget ticket): the same mutable-ref seam as
 * `leadIdleRef`, filled by `index.ts`'s `session_start` (TUI lead only) with
 * a closure that recomputes `agent-widget.ts`'s rows and repaints the
 * `belowEditor` widget + `setStatus` segment. Lets every registry-transition
 * point in this module (spawn, settle, stop, exit, spawn-failed, an approval
 * or report event) trigger a re-render WITHOUT importing `agent-widget.ts` or
 * `ask.ts` — this module must stay the lower layer, mirroring `ask.ts`'s own
 * "imports FROM spawner.ts, never the reverse" rule. `undefined` outside a
 * TUI lead session (a worker/explore child, a headless lead, a test), in
 * which case `triggerAgentWidgetRefresh` is a silent no-op.
 */
export const agentWidgetRefreshRef: { current: (() => void) | undefined } = { current: undefined };

/**
 * Best-effort fire of `agentWidgetRefreshRef`, swallowing a throw — matches
 * every other push call site's swallow-and-continue convention. Called from
 * every registry-transition point a live-agent-widget row depends on: a
 * throwing or absent refresh must never turn a routine spawn/settle/stop into
 * a crashed event listener.
 */
export function triggerAgentWidgetRefresh(): void {
  try {
    agentWidgetRefreshRef.current?.();
  } catch {
    // best effort — see doc comment above.
  }
  triggerAgentDashboardRefresh();
}

/**
 * Dirty notification for the dashboard feed (`dashboard-feed.ts`), filled by
 * `index.ts` for the owner lead: every widget transition plus every batch
 * change, streaming deltas included. The feed samples the registry on its own
 * throttle, so a call here is O(1).
 */
export const agentDashboardRefreshRef: { current: (() => void) | undefined } = { current: undefined };

export function triggerAgentDashboardRefresh(): void {
  try {
    agentDashboardRefreshRef.current?.();
  } catch {
    // The dashboard feed is advisory and never fails an agent lifecycle event.
  }
}

/** Separate event seam for the footer's bounded telemetry reconciliation. */
export const agentCostRefreshRef: { current: (() => void) | undefined } = { current: undefined };

export function triggerAgentCostRefresh(): void {
  try {
    agentCostRefreshRef.current?.();
  } catch {
    // Cosmetic accounting must never fail an agent lifecycle transition.
  }
}

/** A direct child's descendant report changed its stored value: refresh the footer and forward (evaluation point 2). */
function onDescendantUsageChanged(): void {
  triggerAgentCostRefresh();
  evaluateDescendantUsage();
}

/** 261028 Phase 2: a direct child's relayed descendant batch model changed or dropped: repaint and forward upward. */
function onAgentViewChanged(): void {
  triggerAgentWidgetRefresh();
  scheduleAgentViewPublish();
}

/** Read current idleness with the independent compaction hold composed in. Delivery also requires an initialized, live accessor. */
export function isOwningAgentIdle(): boolean {
  if (leadCompactingRef.current) return false;
  const isIdle = leadIdleRef.current;
  if (!isIdle) return true;
  try {
    return isIdle() !== false;
  } catch {
    return false;
  }
}

/** One volatile terminal event's shared-FIFO lifecycle. This is admission
 * bookkeeping only: `enqueued` means Pi accepted the synchronous sendMessage
 * call, not that the model consumed it. */
export interface TerminalDelivery {
  state?: "held" | "enqueued";
  /** Re-evaluate parking and subtree protection after direct-parent admission. */
  afterEnqueue?: () => void;
  /** Retry a failed pre-queue admission without harvesting or notifying twice. */
  retry?: () => void;
}

/** In-memory finish state for one fork-raised `/done`. It intentionally has
 * no persistence representation: reload/death recovery remains best effort. */
export interface ForkFinishOperation {
  token: string;
  cwd: string;
  extensionPath: string;
  leadSessionKey?: string;
  generation: number | undefined;
  phase: "waiting" | "evaluating" | "closeout" | "parking" | "complete";
  settled: boolean;
  closeoutIssued: boolean;
  /** `agent_start` observed for the closeout's own run. It fences a duplicate
   * settle from the pre-closeout run. */
  closeoutRunStarted: boolean;
  /** The observed settle that ended the work before closeout. Exact repeated
   * delivery of that event is not evidence that the closeout has settled. */
  preCloseoutSettleEvent?: object;
  terminal?: TerminalDelivery;
  advancing?: Promise<void>;
}

interface HeldPush {
  kind: "push";
  registry: RpcAgentRegistry | undefined;
  record: RpcAgentRecord | undefined;
  family: PushFamily;
  payload: Record<string, unknown>;
  /** Optional same-process terminal delivery reference. */
  terminal?: TerminalDelivery;
  /** Admission mode: governs whether busy delivery holds or interrupts. Confirmed-start delivery always overrides it to `steer`. */
  deliverAs: PushDeliverAs;
  /** Work generation at admission, used to reject controls superseded before the snapshot. */
  actionGeneration?: number;
  /** For `ws-agent-approval`: the request's `issue` at admission; a reissue supersedes this push. */
  approvalIssue?: number;
  /** Exact report-log entry created for a queued headless question. */
  questionReport?: AgentReportLogEntry;
}

/** Pre-built summaries share the family-push FIFO and retain their original structured custom message. */
interface HeldRawSend {
  kind: "raw";
  deliverAs: PushDeliverAs;
  message: Parameters<ExtensionAPI["sendMessage"]>[0];
  /** Whether a no-prefix active delivery must still use ws-push-batch materialization. */
  batchMode: RawPushBatchMode;
  /** Batch item state; absent is `informational`. Set only by `prependRecoveredPushes`, whose items keep their send-time state. */
  state?: PushBatchItemState;
}

/** Delivery-time outcome for a queued `/goal <goal>` replacement. */
export interface HeldGoalReplacementResult {
  outcome: "applied" | "failed";
  message: string;
}

/**
 * Control state carried in the shared held-input FIFO. `result` latches the
 * first safe-boundary application so a rejected prose batch can retry without
 * applying the goal replacement twice.
 */
export interface HeldGoalReplacement {
  kind: "goal-replacement";
  goal: string;
  generation: number;
  apply: () => HeldGoalReplacementResult;
  report: (result: HeldGoalReplacementResult) => void;
  result?: HeldGoalReplacementResult;
}

export type HeldInput = HeldPush | HeldRawSend | HeldGoalReplacement;

/**
 * Pushes that arrived while the owning session was mid-turn, in arrival order.
 *
 * Phase 1 Edition (live-run fix): Pi queues a `followUp` sent mid-turn in its
 * own `PendingMessageQueue` and delivers it after the turn ends — but there is
 * no extension hook at that delivery (`before_agent_start` fires only for
 * user prompts, including counted wakes; see `agent-session.ts`'s `prompt()` vs
 * `sendCustomMessage`), so a message built at ARRIVAL time carried a status
 * line already stale by the time the lead read it. A worker that finished
 * while the lead was still spawning its siblings reported zero still running
 * (its siblings were not registered yet), and so did the next — three
 * separate invitations to synthesize early. Holding the push here and building
 * the message at FLUSH
 * time is the fix: the status line is then computed against the registry as it
 * stands when the lead actually reads it.
 *
 * Exported for the flush handler and for test isolation only; nothing else may
 * write it. Held pushes are in-memory and die with the process, exactly like
 * the Pi queue they stand in for — `session_shutdown` drops them rather than
 * persisting them (the sidecar carries child IDENTITIES, never reports).
 *
 * 260906 (compaction push-hold ticket, Phase 1): also holds `HeldRawSend`
 * entries (see that type's doc comment) so `ask.ts`'s
 * `injectDiscussionSummary` can share this same queue/flush-ordering
 * mechanism instead of maintaining a parallel one. Goal replacements add a
 * third typed entry: controls resolve in FIFO order at a safe boundary, stay
 * out of model content, and retain their result across a rejected prose batch.
 */
export const heldPushQueue: HeldInput[] = [];

/**
 * 261015: put push items back at the FRONT of the held queue, in the given
 * order. Lead compaction calls this for steers Pi drained into the aborted
 * `ws-compact` lever run, which its no-raw-tail summary would otherwise drop.
 * Front, because `admitPush` steers directly only into an empty queue, so
 * every item held now was admitted after them. Each item replays its
 * send-time content and `state` as a raw entry with no terminal reference:
 * the lost steer already advanced its terminal delivery to `enqueued` and ran
 * `afterEnqueue`, and re-delivery must do neither again.
 */
export function prependRecoveredPushes(items: readonly PushBatchItem[]): void {
  heldPushQueue.unshift(...items.map((item): HeldRawSend => ({
    kind: "raw",
    deliverAs: "steer",
    batchMode: "always",
    state: item.state,
    message: { customType: item.customType, content: item.content as never, display: item.display, details: item.details },
  })));
}

/** Admit a validated goal replacement without creating a parallel control queue. */
export function enqueueHeldGoalReplacement(entry: HeldGoalReplacement): void {
  heldPushQueue.push(entry);
}

function xmlEscape(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

function customContentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return String(content ?? "");
  return content.map((part) => {
    const candidate = part as { type?: unknown; text?: unknown };
    return candidate?.type === "text" && typeof candidate.text === "string"
      ? candidate.text
      : JSON.stringify(part);
  }).join("\n");
}

function batchItemIdentity(item: PushBatchItem): { agentId?: string; commandId?: string } {
  const details = item.details as { agent_id?: unknown; cmd_id?: unknown } | undefined;
  return {
    agentId: typeof details?.agent_id === "string" ? details.agent_id : undefined,
    commandId: typeof details?.cmd_id === "string" ? details.cmd_id : undefined,
  };
}

/** Serialize a FIFO snapshot without truncation; actionable summaries follow every ordered message. */
function buildPushBatchContent(items: readonly PushBatchItem[]): string {
  const lines = [`<${PUSH_BATCH_CUSTOM_TYPE} version="${PUSH_BATCH_VERSION}">`];
  for (const item of items) {
    const { agentId, commandId } = batchItemIdentity(item);
    const attrs = [
      `type="${xmlEscape(item.customType)}"`,
      `state="${item.state}"`,
      ...(agentId ? [`agent-id="${xmlEscape(agentId)}"`] : []),
      ...(commandId ? [`command-id="${xmlEscape(commandId)}"`] : []),
    ];
    lines.push(`  <message ${attrs.join(" ")}>${xmlEscape(customContentText(item.content))}</message>`);
  }
  const actions = items.flatMap((item) => {
    if (item.state !== "actionable") return [];
    const { agentId, commandId } = batchItemIdentity(item);
    if (item.customType === "ws-agent-approval" && agentId && commandId) {
      return [`    <action type="approval" agent-id="${xmlEscape(agentId)}" command-id="${xmlEscape(commandId)}">Call ws-approve with agent_id=&quot;${xmlEscape(agentId)}&quot; and cmd_id=&quot;${xmlEscape(commandId)}&quot;.</action>`];
    }
    if (item.customType === "ws-agent-question" && agentId) {
      return [`    <action type="question" agent-id="${xmlEscape(agentId)}">Call ws-agent-send with agent_id=&quot;${xmlEscape(agentId)}&quot; and the answer.</action>`];
    }
    return [];
  });
  lines.push("  <action-summary>", ...(actions.length ? actions : ["    none"]), "  </action-summary>", `</${PUSH_BATCH_CUSTOM_TYPE}>`);
  return lines.join("\n");
}

function heldActionState(held: HeldPush): PushBatchItemState {
  if (held.family === "ws-agent-approval") {
    const cmdId = typeof held.payload.cmd_id === "string" ? held.payload.cmd_id : undefined;
    const pending = held.record?.pendingApproval;
    // A decision in flight or discarded supersedes the held request: the
    // reconnect hello either releases it or re-issues it as a fresh push. A
    // reissue clears `decision` again, so the issue number keeps this push
    // superseded next to the reissued one: one actionable prompt per cmd_id.
    return cmdId && held.record?.workGeneration === held.actionGeneration && pending?.cmdId === cmdId && pending.decision === undefined
      && (pending.issue ?? 0) === held.approvalIssue
      ? "actionable"
      : "superseded";
  }
  if (held.family === "ws-agent-question") {
    const latest = held.record?.reportLog.at(-1);
    return held.record?.workGeneration === held.actionGeneration && latest === held.questionReport && latest?.kind === "question"
      ? "actionable"
      : "superseded";
  }
  return "informational";
}

function resolveHeldGoalReplacement(held: HeldGoalReplacement): HeldGoalReplacementResult {
  if (held.result) return held.result;
  try {
    held.result = held.apply();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    held.result = { outcome: "failed", message: `Goal update failed: ${message}` };
  }
  try {
    held.report(held.result);
  } catch {
    // Goal state remains authoritative even if owner-visible feedback is unavailable.
  }
  return held.result;
}

function materializeHeldInput(held: HeldInput): PushBatchItem {
  if (held.kind === "raw") {
    return {
      customType: held.message.customType,
      content: held.message.content as string | unknown[],
      display: held.message.display,
      details: held.message.details,
      state: held.state ?? "informational",
    };
  }
  if (held.kind === "goal-replacement") {
    const result = resolveHeldGoalReplacement(held);
    return {
      customType: GOAL_CONTROL_CUSTOM_TYPE,
      content: result.message,
      display: true,
      details: { goal: held.goal, generation: held.generation, outcome: result.outcome },
      state: "informational",
    };
  }
  const status = computeRunningStatusLine(held.registry);
  const base: Record<string, unknown> = held.record ? { agent_id: held.record.agentId, ...(held.family === "ws-agent-settled" ? { failed: held.record.lastRunFailed === true } : {}), ...held.payload } : { ...held.payload };
  const details = status ? { ...base, status } : base;
  const displayId = held.record?.alias ? `${held.record.alias} (${held.record.agentId})` : held.record?.agentId;
  return {
    customType: held.family,
    content: buildPushContent(held.family, displayId, held.payload, status),
    display: true,
    details,
    state: heldActionState(held),
  };
}

/** Build current family status and send into a confirmed streaming run with the requested delivery mode. */
function sendPush(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
  terminal?: TerminalDelivery,
): void {
  const wasHeld = terminal?.state === "held";
  const status = computeRunningStatusLine(registry);
  const base: Record<string, unknown> = record ? { agent_id: record.agentId, ...(family === "ws-agent-settled" ? { failed: record.lastRunFailed === true } : {}), ...payload } : { ...payload };
  const details = status ? { ...base, status } : base;
  // 260905 (alias/park/cap ticket): pushed-message heads print the alias when
  // there is one, followed by the uuid — a one-line, zero-signature-churn
  // change: compose the display id here and pass it through
  // `buildPushContent`'s existing bare-`agentId` parameter, leaving that
  // helper (and `details.agent_id`, which stays the raw uuid) untouched.
  const displayId = record?.alias ? `${record.alias} (${record.agentId})` : record?.agentId;
  try {
    sendIntoRun(
      pi,
      {
        customType: family,
        content: labelAdapterText(buildPushContent(family, displayId, payload, status)),
        display: true,
        details: withSummaryId(details) as never,
      },
      deliverAs,
    );
    if (terminal) {
      terminal.state = "enqueued";
      if (wasHeld) terminal.afterEnqueue?.();
    }
  } catch {
    // Best effort: a push that cannot be delivered (a torn-down session, a
    // host that rejected the message) must never turn a child's routine
    // report into a crashed event listener.
  }
}

/**
 * Release one immutable prefix snapshot as one custom message. The prefix stays
 * queued until `sendMessage` returns synchronously; rejection restores every
 * terminal obligation and lets the ordinary settle/wake path retry it.
 */
function submitHeldPushBatch(pi: ExtensionAPI, deliverAs: "steer" | "followUp" | "nextTurn", turnBoundary = false): number {
  if (heldPushQueue.length === 0) return 0;
  const snapshot = heldPushQueue.slice();
  const terminalStates = snapshot.flatMap((held) => held.kind === "push" && held.terminal
    ? [{ terminal: held.terminal, wasHeld: held.terminal.state === "held" }]
    : []);
  // Each item card is summarized on its own, so each gets its own row id;
  // the model content below reads only item content and identity.
  const items = withItemSummaryIds(snapshot.map(materializeHeldInput));
  const modelItems = items.filter((_item, index) => snapshot[index]!.kind !== "goal-replacement");
  if (modelItems.length > 0) {
    const batch: LeadCustomMessage = {
      customType: PUSH_BATCH_CUSTOM_TYPE,
      content: labelAdapterText(buildPushBatchContent(modelItems)),
      display: true,
      details: { version: PUSH_BATCH_VERSION, items } as never,
    };
    try {
      if (deliverAs === "nextTurn") pi.sendMessage(batch, { deliverAs });
      else sendIntoRun(pi, batch, deliverAs);
    } catch {
      // A rejected `nextTurn` queue means a torn-down session; no wake for it.
      if (deliverAs !== "nextTurn") requestPushWake(pi);
      return 0;
    }
    // Queued at `agent_end`, the batch is picked up by Pi's post-run
    // continuation, a fresh turn; set before `afterEnqueue` publishes below.
    if (turnBoundary) {
      boundaryTurnOwed = true;
      syncOwnTurnOwed();
    }
  }
  heldPushQueue.splice(0, snapshot.length);
  for (const { terminal, wasHeld } of terminalStates) {
    terminal.state = "enqueued";
    if (wasHeld) terminal.afterEnqueue?.();
  }
  return snapshot.length;
}

/** Idle release applies a control-only queue or reserves one counted wake for mixed/prose input. Confirmed starts and lead turn boundaries release one FIFO batch. */
export function flushHeldPushes(pi: ExtensionAPI | undefined, confirmedStart = false, agentEndBoundary = false): number {
  // A run a dashboard interrupt is cutting takes nothing more; its settle queues the FIFO for the next prompt.
  if (!pi || !shouldPushToLead() || !leadIdleRef.current || leadCompactingRef.current || leadWakeStartPendingRef.current || interruptHolding()) return 0;
  if (agentEndBoundary) return submitHeldPushBatch(pi, "followUp", true);
  if (!confirmedStart) {
    const snapshot = heldPushQueue.slice();
    if (snapshot.length > 0 && snapshot.every((held) => held.kind === "goal-replacement")) {
      for (const held of snapshot) resolveHeldGoalReplacement(held);
      heldPushQueue.splice(0, snapshot.length);
      return snapshot.length;
    }
    requestPushWake(pi);
    return 0;
  }
  return submitHeldPushBatch(pi, "steer");
}

/** Factory-scope wake lifecycle, also active in fork owners. Registration allocates no timers; only a reserved user wake does. Worker/explore roles never reserve wakes. */
export function registerPushFlush(pi: ExtensionAPI, options: WakeStartOptions & { publish?: () => void }): void {
  resetPushWakeBreaker();
  // A fresh registration tracks only its own sends and its own interrupts.
  untakenPushes.length = 0;
  leadInterrupt = undefined;
  wakeOptions = options;
  publishOwnTurn = options.publish;
  pi.on("agent_start", () => {
    // Before the flush below publishes: this start both counts and discharges
    // any owed turn in one snapshot (see `ownTurnRef`).
    ownTurnRef.started += 1;
    boundaryTurnOwed = false;
    // A new run: a dashboard interrupt's mark ends here (see `leadInterrupt`).
    leadInterrupt = undefined;
    resetPushWakeBreaker();
    clearWakeStart();
    flushHeldPushes(pi, true);
  });
  pi.on("message_start", (event) => {
    forgetTakenPush((event as { message?: unknown } | undefined)?.message);
  });
  pi.on("agent_end", () => {
    flushHeldPushes(pi, false, true);
  });
  pi.on("agent_settled", () => {
    // Synchronously, before the deferred settle flush below can wake on the
    // held queue: the interrupted run's leftovers wait for the next prompt.
    if (interruptHolding()) releaseAfterInterrupt(pi);
    untakenPushes.length = 0;
    clearWakeStart();
    if (pushWakeAwaitingStart) {
      pushWakeAwaitingStart = false;
      failedPushWakes += 1;
      scheduleSettleWake(pi, Math.min(PUSH_WAKE_BACKOFF_BASE_MS * 2 ** (failedPushWakes - 1), PUSH_WAKE_BACKOFF_CAP_MS));
    } else if (!pushWakeBackingOff()) {
      // Another settle (not this breaker's failed wake) keeps a pending backoff.
      scheduleSettleWake(pi, 0);
    }
    // Pi emits the raw settle only after its post-run continuation started or
    // was abandoned (a throwing `agent.continue()`), so a boundary batch still
    // unstarted here is owed no turn: it starts with the next prompted turn.
    // With the flush deferred, `owed` here is false; a still-held queue keeps
    // the parent waiting through `deliveries` until the deferred flush
    // publishes its own reservation flip.
    // No send of its own: the publish handler `index.ts` registers after this
    // one carries the flip, which is why this handler must stay first.
    boundaryTurnOwed = false;
    syncOwnTurnOwed();
  });
  pi.on("session_shutdown", () => {
    resetPushWakeBreaker();
    clearWakeStart();
    heldPushQueue.length = 0;
    untakenPushes.length = 0;
    leadInterrupt = undefined;
    boundaryTurnOwed = false;
    syncOwnTurnOwed();
    leadIdleRef.current = undefined;
    wakeOptions = undefined;
    publishOwnTurn = undefined;
  });
}

/** `true` while a settled result still has not reached its direct parent's queue. */
export function hasPendingTerminalDelivery(record: Pick<RpcAgentRecord, "terminalDelivery">): boolean {
  return record.terminalDelivery !== undefined && record.terminalDelivery.state !== "enqueued";
}

export function createTerminalDelivery(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry | undefined,
  pi: ExtensionAPI | undefined,
  generation: number | undefined,
  payload?: Record<string, unknown>,
): TerminalDelivery {
  const terminal: TerminalDelivery = {};
  terminal.afterEnqueue = () => {
    if (record.workGeneration !== generation || record.terminalDelivery !== terminal) return;
    syncOwnershipProtection(record);
    publishSubtree(registry);
    if (registry && record.client && !record.running && !record.streaming && !record.threadBound && !isOwnerHeld(record) && !record.waitingOnChildren) {
      void stopAgent(registry, record.agentId, pi, { silent: true });
    }
  };
  if (payload) terminal.retry = () => {
    if (terminal.state !== undefined || record.workGeneration !== generation || record.terminalDelivery !== terminal) return;
    pushToLead(pi, registry, record, "ws-agent-settled", payload, "followUp", terminal);
  };
  record.terminalDelivery = terminal;
  syncOwnershipProtection(record);
  publishSubtree(registry);
  return terminal;
}

/** Starts (or joins) one same-process finish operation for a fork-raised
 * owner `/done`. The operation deliberately has no persisted identity: its
 * only promise is race-safe coordination while this adapter instance lives. */
export function startForkFinish(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry,
  pi: ExtensionAPI,
  resumeCtx: Pick<RpcResumeCtx, "cwd" | "extensionPath" | "leadSessionKey">,
): ForkFinishOperation {
  if (record.forkFinish) return record.forkFinish;
  claimLeadOwnership(record);
  const operation: ForkFinishOperation = {
    token: randomUUID(),
    cwd: resumeCtx.cwd,
    extensionPath: resumeCtx.extensionPath,
    leadSessionKey: resumeCtx.leadSessionKey,
    generation: record.launchGeneration,
    phase: "closeout",
    settled: false,
    closeoutIssued: true,
    // A follow-up joins an already-running turn and produces no fresh
    // `agent_start`; its next settle still belongs to this handoff.
    closeoutRunStarted: record.running || record.streaming,
  };
  record.forkFinish = operation;
  void sendToAgent(registry, { ...finishResumeCtx(operation), pi, finishToken: operation.token }, record.agentId,
    "The owner closed this side thread. Finish the task now. End with the requested final-response format in your ordinary assistant answer; do not use ws-report-to-lead as a completion channel.", false)
    .catch((err) => {
      if (record.forkFinish === operation && record.launchGeneration === operation.generation) {
        operation.phase = "complete";
        record.forkFinish = undefined;
        try { record.onForkFinishComplete?.(record, `closeout failed: ${err instanceof Error ? err.message : String(err)}`); } catch { /* best effort */ }
      }
    });
  return operation;
}

/** The single owner of finish advancement. Every call joins `advancing`, so
 * report/settle/duplicate-callback races cannot issue a second closeout. */
export function advanceForkFinish(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry,
  pi: ExtensionAPI,
  resumeCtx: Pick<RpcResumeCtx, "cwd" | "extensionPath">,
  operation: ForkFinishOperation,
): Promise<void> {
  if (operation.advancing) return operation.advancing;
  operation.advancing = (async () => {
    if (record.forkFinish !== operation || operation.phase === "complete") return;
    if (record.launchGeneration !== operation.generation || !operation.settled || record.waitingOnChildren) return;
    operation.phase = "evaluating";

    const existing = operation.terminal ?? record.terminalDelivery;
    if (existing?.state === "enqueued") {
      await parkForkFinish(record, registry, pi, operation);
      return;
    }
    if (existing?.state === "held") {
      operation.terminal = existing;
      return;
    }
    if (existing) {
      existing.retry?.();
      return;
    }

    const generation = record.workGeneration;
    const lastMessage = await harvestLastMessage(record);
    if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || record.workGeneration !== generation || record.running || record.streaming || record.waitingOnChildren) return;
    const terminal = createTerminalDelivery(record, registry, pi, generation, { reason: "idle", last_message: lastMessage });
    operation.terminal = terminal;
    terminal.afterEnqueue = () => {
      if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || record.workGeneration !== generation) return;
      syncOwnershipProtection(record);
      publishSubtree(registry);
      void parkForkFinish(record, registry, pi, operation);
    };
    terminal.retry?.();
  })().finally(() => {
    operation.advancing = undefined;
    // A settle can arrive while the closeout RPC promise is unresolved. The
    // listener joined the in-flight advance above; replay exactly once after
    // it releases rather than stranding an already-idle fork.
    if (record.forkFinish === operation && operation.phase === "closeout" && operation.settled) {
      void advanceForkFinish(record, registry, pi, resumeCtx, operation);
    }
  });
  return operation.advancing;
}

export function finishResumeCtx(operation: ForkFinishOperation): Pick<RpcResumeCtx, "cwd" | "extensionPath" | "leadSessionKey"> {
  return { cwd: operation.cwd, extensionPath: operation.extensionPath, leadSessionKey: operation.leadSessionKey };
}

async function parkForkFinish(record: RpcAgentRecord, registry: RpcAgentRegistry, pi: ExtensionAPI, operation: ForkFinishOperation, failure?: string): Promise<void> {
  if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || operation.phase === "parking" || operation.phase === "complete" || record.waitingOnChildren) return;
  operation.phase = "parking";
  // Preserve the thread bind until this coordinator owns the park; stopAgent
  // performs the normal silent stop and clears it synchronously.
  try {
    let stopped = true;
    await stopAgent(registry, record.agentId, pi, { silent: true, onStopped: (success) => { stopped = success; } });
    if (!stopped) failure ??= "park failed: child stop did not complete";
  } catch (err) {
    failure ??= `park failed: ${err instanceof Error ? err.message : String(err)}`;
  }
  if (record.forkFinish !== operation || record.launchGeneration !== operation.generation) return;
  operation.phase = "complete";
  record.forkFinish = undefined;
  try { record.onForkFinishComplete?.(record, failure); } catch { /* ordinary thread persistence is best effort */ }
}

/** Feed report/settle/tool-result observations into an active coordinator. */
export function observeForkFinishEvent(record: RpcAgentRecord, evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown }): void {
  const operation = record.forkFinish;
  if (!operation || record.launchGeneration !== operation.generation) return;
  if (evt.type === "agent_start") {
    if (operation.closeoutIssued) operation.closeoutRunStarted = true;
    return;
  }
  if (evt.type === "agent_settled") {
    // The original settle begins closeout. Once closeout starts, both its own
    // start AND a different settle event are required: RPC listeners can
    // replay the exact old object after the new run has begun.
    if (!operation.closeoutIssued) {
      operation.preCloseoutSettleEvent = evt;
      operation.settled = true;
    } else if (operation.closeoutRunStarted && evt !== operation.preCloseoutSettleEvent) {
      operation.settled = true;
    }
    return;
  }
}

/** Admit a family push through the shared FIFO. Idle, compacting, pending-start, and ordinary busy followUps are held; normal busy steers still interrupt. Idle wakes use sendUserMessage so before_agent_start composes the ws block for the run. */
export function pushToLead(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
  terminal?: TerminalDelivery,
): void {
  const ownerRouteAvailable = ownerNotifyRef.current !== undefined;
  if (record && (isOwnerHeld(record) || (record.threadBound && !record.forkFinish)) && ownerRouteAvailable && (family === "ws-agent-settled" || family === "ws-agent-advisory")) {
    const name = record.alias ?? record.title ?? record.agentId.slice(0, 8);
    const detail = family === "ws-agent-settled"
      ? `settled${typeof payload.last_message === "string" && payload.last_message.trim() ? `: ${payload.last_message.trim()}` : ""}`
      : `advisory: ${String(payload.detail ?? payload.advisory ?? "attention required")}`;
    try {
      ownerNotifyRef.current?.(`ws: ${name} ${detail}`, family === "ws-agent-advisory" ? "warning" : "info");
      if (terminal) {
        terminal.state = "enqueued";
        terminal.afterEnqueue?.();
      }
    } catch {
      // Owner UI routes can be temporarily unavailable during overlay/session
      // transitions. Keep the terminal obligation live and actively retry it;
      // a successor generation clears record.terminalDelivery and makes the
      // scheduled callback a no-op.
      if (terminal?.retry && terminal.state === undefined) {
        const timer = setTimeout(() => terminal.retry?.(), OWNER_TERMINAL_RETRY_DELAY_MS);
        timer.unref?.();
      }
    }
    return;
  }
  if (!pi || !shouldPushToLead() || !leadIdleRef.current) return;
  admitPush(pi, { kind: "push", registry, record, family, payload, deliverAs, terminal });
  publishSubtree(registry);
}

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
