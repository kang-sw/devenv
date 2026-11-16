import { mkdtempSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, isAbsolute, join } from "node:path";
import type { Readable } from "node:stream";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient, type RpcClientOptions } from "@earendil-works/pi-coding-agent";
import type { ForkCacheNoticeOwner } from "./fork-cache-notice.ts";
import type { McpStdioClient } from "./mcp-stdio-client.ts";
import type { ModelCatalogEntry } from "./model-catalog.ts";
import { WS_PI_EXPLORE_MODE_ENV, WS_PI_FORK_AFFINITY_ENV, WS_PI_FORK_CONTEXT_ENV, WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV, type ExploreMode, type SpawnRole } from "./process-role.ts";
import { FORK_READINESS_KIND, captureForkContext, compareForkRegistrations, writePrivateJson, type ForkContext, type ForkReadiness } from "./fork-context.ts";
import { CHANNEL_BOOTSTRAP_ENVS, ParentChannel, type ChannelBindOptions } from "./agent-channel.ts";
import { isOwnedSessionPath, observeSessionWrite, readOwnership, touchOwnership, updateOwnership, writeOwnership, type AgentStorageContext } from "./agent-storage.ts";
import { DELEGATION_ENV, type DelegationPolicy, type PlaybookProfile, type RenderProvenance } from "./delegation-policy.ts";
import type { WriteScope } from "./write-scopes.ts";
import { WEB_HOME_ENV, WEB_READINESS_KIND } from "./web-readiness.ts";
import { publishSubtree } from "./subtree-lifecycle.ts";
import { closeOpenBatch } from "./agent-batches.ts";
import { scheduleAgentViewPublish } from "./agent-view.ts";
import { REPORT_LOG_CAP } from "./spawner-record.ts";
import type { RpcAgentRecord, RpcAgentRegistry, SpawnAgentRole } from "./spawner-record.ts";
import { triggerAgentWidgetRefresh, createTerminalDelivery, hasPendingTerminalDelivery, pushToLead, isOwnerHeld } from "./spawner-push.ts";
import { CHILD_BOOTSTRAP_OVERRIDE_ENVS, CHILD_MAILBOX_IDENTITY_ENVS } from "./spawner-tools.ts";
import type { ToolGroup, ResolvedModelInfo } from "./spawner-tools.ts";

// ---------------------------------------------------------------------------
// RPC-backed persistent-child engine (`ws-agent-spawn` / `ws-agent-send` /
// `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript`).
// ---------------------------------------------------------------------------

/**
 * Every `new RpcClient(...)` construction passes `cliPath: process.argv[1]`
 * explicitly — spawn and resume-from-dormant alike. It also receives the
 * exact entry path captured by the loaded manifest module, with ambient
 * extension discovery disabled, so a child cannot load a duplicate adapter. `RpcClient.start()` in
 * the installed `@earendil-works/pi-coding-agent` package always does
 * `spawn("node", [cliPath, ...args])` with `cliPath = this.options.cliPath
 * ?? "dist/cli.js"`; there is no bare-`pi` fallback inside `RpcClient`.
 * If `process.argv[1]` is ever missing/non-existent
 * there is no client-side fallback path — this is the ticket's settled
 * answer, carried forward as-is, not something to re-derive.
 */
const RPC_CLI_PATH = process.argv[1];

/**
 * Single funnel for every `client.prompt(...)` call in this adapter, so the
 * fan-in bookkeeping cannot drift from the actual dispatches: marks the child
 * `running` from the instant the prompt is ISSUED (not when `agent_start`
 * arrives — a lead ending its turn immediately after dispatch must already
 * see it counted), starts a fresh work generation, and stamps
 * `lastLeadPromptAt`. A prompt joins its active run when the child is already
 * running; only a resting child re-arms `runStartedAt` and clears `settledAt`.
 */
interface WriterOperation {
  writer: "lead" | "owner";
  leadPromptAt?: number;
  state: "pending" | "accepted";
}

interface WriterOperationState {
  baseWriter: RpcAgentRecord["lastWriter"];
  baseLeadPromptAt: number | undefined;
  operations: WriterOperation[];
}

const writerOperations = new WeakMap<RpcAgentRecord, WriterOperationState>();

const dispatchOrder = new WeakMap<RpcAgentRecord, { issued: number; accepted: number }>();

export function issueDispatch(record: RpcAgentRecord): number {
  const order = dispatchOrder.get(record) ?? { issued: 0, accepted: 0 };
  order.issued++;
  dispatchOrder.set(record, order);
  return order.issued;
}

export function acceptDispatch(record: RpcAgentRecord, sequence: number): void {
  const order = dispatchOrder.get(record);
  if (order) order.accepted = Math.max(order.accepted, sequence);
}

export function wasSupersededByAcceptedDispatch(record: RpcAgentRecord, sequence: number): boolean {
  return (dispatchOrder.get(record)?.accepted ?? 0) > sequence;
}

function projectWriter(record: RpcAgentRecord, state: WriterOperationState): void {
  let writer = state.baseWriter;
  let leadPromptAt = state.baseLeadPromptAt;
  for (const operation of state.operations) {
    writer = operation.writer;
    if (operation.leadPromptAt !== undefined) leadPromptAt = operation.leadPromptAt;
  }
  record.lastWriter = writer;
  record.lastLeadPromptAt = leadPromptAt;
}

function compactWriterOperations(record: RpcAgentRecord, state: WriterOperationState): void {
  while (state.operations[0]?.state === "accepted") {
    const accepted = state.operations.shift()!;
    state.baseWriter = accepted.writer;
    if (accepted.leadPromptAt !== undefined) state.baseLeadPromptAt = accepted.leadPromptAt;
  }
  projectWriter(record, state);
  if (state.operations.length === 0) writerOperations.delete(record);
  syncOwnershipProtection(record);
  triggerAgentWidgetRefresh();
}

export function stampWriter(record: RpcAgentRecord, writer: "lead" | "owner", text?: string, leadPromptAt?: number): { operation: WriterOperation; ownerSend?: { text: string; at: number } } {
  let state = writerOperations.get(record);
  if (!state) {
    state = { baseWriter: record.lastWriter, baseLeadPromptAt: record.lastLeadPromptAt, operations: [] };
    writerOperations.set(record, state);
  }
  const operation: WriterOperation = { writer, ...(leadPromptAt === undefined ? {} : { leadPromptAt }), state: "pending" };
  const ownerSend = writer === "owner" && text !== undefined ? { text, at: Date.now() } : undefined;
  state.operations.push(operation);
  if (ownerSend) (record.ownerSends ??= []).push(ownerSend);
  projectWriter(record, state);
  syncOwnershipProtection(record);
  triggerAgentWidgetRefresh();
  return { operation, ownerSend };
}

export function acceptWriter(record: RpcAgentRecord, stamp: ReturnType<typeof stampWriter>): void {
  const state = writerOperations.get(record);
  if (!state || !state.operations.includes(stamp.operation)) return;
  stamp.operation.state = "accepted";
  compactWriterOperations(record, state);
}

export function rollbackWriter(record: RpcAgentRecord, stamp: ReturnType<typeof stampWriter>): void {
  const state = writerOperations.get(record);
  if (stamp.ownerSend) {
    const ownerIndex = record.ownerSends?.indexOf(stamp.ownerSend) ?? -1;
    if (ownerIndex >= 0) record.ownerSends!.splice(ownerIndex, 1);
  }
  if (!state) return;
  const index = state.operations.indexOf(stamp.operation);
  if (index >= 0) state.operations.splice(index, 1);
  compactWriterOperations(record, state);
}

export function claimLeadOwnership(record: RpcAgentRecord): void {
  const stamp = stampWriter(record, "lead");
  acceptWriter(record, stamp);
}

export const OWNER_TERMINAL_RETRY_DELAY_MS = 100;

export async function promptAgent(
  record: RpcAgentRecord,
  client: RpcClient,
  message: string,
  opts?: { isLeadPrompt?: boolean; writer?: "lead" | "owner" },
): Promise<void> {
  const writer = opts?.writer ?? "lead";
  const now = Date.now();
  const writerStamp = stampWriter(record, writer, writer === "owner" ? message : undefined, writer === "lead" && opts?.isLeadPrompt !== false ? now : undefined);
  const startsNewRun = !record.running && !record.streaming;
  record.running = true;
  record.pendingQueuedWork = undefined;
  advanceWorkGeneration(record);
  if (startsNewRun) {
    record.runStartedAt = now;
    record.settledAt = undefined;
  }
  if (record.ownership) observeSessionWrite(record.ownership.home, record.sessionPath);
  if (record.ownership) updateOwnership(record.ownership.home, { lastActivityAt: now, liveness: { lifecycle: "live", running: true, observedAt: now } });
  const workGeneration = record.workGeneration;
  try {
    await client.prompt(message);
  } catch (error) {
    rollbackWriter(record, writerStamp);
    throw error;
  }
  acceptWriter(record, writerStamp);
  if (record.delegation && record.workGeneration === workGeneration) syncOwnershipProtection(record);
}

/**
 * Transitions `record` to the dead/stopped resting state: no client, not
 * running, not streaming, listener detached. Shared by the liveness probe,
 * the in-flight-rejection paths, and `stopAgent`, so "what a stopped record
 * looks like" is defined once.
 */
export function clearTerminalFacts(record: RpcAgentRecord): void {
  record.terminalDelivery = undefined;
}

/** Start work whose assistant output must never inherit an earlier turn. */
export function advanceWorkGeneration(record: RpcAgentRecord): void {
  record.workGeneration = (record.workGeneration ?? 0) + 1;
  record.lastText = undefined;
  record.lastTextGeneration = undefined;
  record.lastRunFailed = undefined;
  clearTerminalFacts(record);
}

export function clearLiveState(record: RpcAgentRecord, registry?: RpcAgentRegistry): void {
  record.unsubscribe?.();
  record.unsubscribe = undefined;
  record.client = undefined;
  // The channel lives exactly as long as the launch: closing it here (stop,
  // exit, failed launch) refuses every later connection with this credential,
  // so a lingering child's reconnect loop can never re-attach to a record
  // that has moved on to another generation.
  record.channel?.close();
  record.channel = undefined;
  record.streaming = false;
  record.running = false;
  record.pendingApproval = undefined;
  record.waitingOnChildren = false;
  record.subtreeRevision = undefined;
  record.subtreeTurn = undefined;
  record.heldSettlementGeneration = undefined;
  record.subtreeDescendants = [];
  // A launch that ends mid-batch must not leave a batch running forever.
  if (record.toolBatches) closeOpenBatch(record.toolBatches, Date.now());
  record.descendantViews = undefined;
  publishSubtree(registry);
  scheduleAgentViewPublish();
  triggerAgentWidgetRefresh();
}

/**
 * Launches `stopAgent` stopped while still in flight, keyed by their
 * `RpcAgentRecord.launching` mark. Separate from `launchGeneration`, which a
 * stop must leave unchanged: its own replacement check compares it.
 */
export const stoppedLaunches = new WeakSet<Promise<void>>();

/**
 * Marks a launch in flight on `record` (see `RpcAgentRecord.launching`) and
 * returns its release, which settles the wait whether the launch reached its
 * first prompt or failed (idempotent, and never clears a later launch's
 * mark), and `stopped`, which reports whether `stopAgent` stopped this launch
 * while it was in flight. A launch failure caused by that stop is the stop's
 * outcome, not a `spawn-failed` one.
 */
export function claimLaunch(record: RpcAgentRecord): { release: () => void; stopped: () => boolean } {
  let release!: () => void;
  const launching = new Promise<void>(resolve => { release = resolve; });
  record.launching = launching;
  return {
    release: () => {
      if (record.launching === launching) record.launching = undefined;
      release();
    },
    stopped: () => stoppedLaunches.has(launching),
  };
}

/** The error a launch stopped by `stopAgent` rejects with instead of its own failure. */
export function launchStoppedError(): Error {
  return new Error("ws-pi-agent: launch stopped (ws-agent-stop) before the agent started");
}

/** The exit `RpcClient` recorded for its process, if any: the one signal that a `getState()` rejection means "gone" rather than "slow". */
function recordedExitError(client: RpcClient): unknown {
  return (client as { exitError?: unknown }).exitError;
}

/** The child's stdout through `RpcClient`'s private process handle; `undefined` when upstream no longer exposes it. */
function childStdout(client: RpcClient): Readable | undefined {
  const stdout = (client as { process?: { stdout?: unknown } | null }).process?.stdout;
  return stdout && typeof (stdout as Readable).once === "function" ? stdout as Readable : undefined;
}

/** Bound on `awaitExitedChildStdout`: a pipe the exited child left behind ends within one poll turn. */
const EXITED_CHILD_STDOUT_WAIT_MS = 500;

/**
 * Before a launch failure is decorated (and before `client.stop()`, which
 * detaches the stdout reader first thing): when the child process already
 * exited, wait, bounded, for its stdout to end so a queued last line (the
 * bootstrap-failure notify) is parsed. The child's `exit` can be delivered
 * before the last stdout chunk in the same poll batch, and the exit rejects
 * the pending request that lands the launch in its catch. A live child (a
 * stop, a handshake timeout) or a missing process handle returns at once.
 */
export async function awaitExitedChildStdout(client: RpcClient | undefined): Promise<void> {
  if (!client || !recordedExitError(client)) return;
  const stdout = childStdout(client);
  if (!stdout || stdout.readableEnded || stdout.destroyed) return;
  await new Promise<void>(resolve => {
    const done = () => {
      clearTimeout(timer);
      stdout.off("end", done);
      stdout.off("close", done);
      resolve();
    };
    const timer = setTimeout(done, EXITED_CHILD_STDOUT_WAIT_MS);
    timer.unref?.();
    stdout.once("end", done);
    stdout.once("close", done);
  });
}

/**
 * 260905 liveness probe. `RpcClient` exposes no public exit event, but its
 * `send()` throws synchronously once the child process has exited (the
 * bundled client sets `exitError` on the process's own `'exit'`/`'error'`),
 * so a `getState()` round-trip is a reliable liveness test: if it rejects,
 * the child is gone. Called on registry transitions (settle), on a periodic
 * timer while anything is outstanding, and implicitly by every in-flight
 * request rejection routed through `markAgentExited`.
 *
 * Returns `true` when the agent is (still) alive, `false` when this call
 * transitioned it to exited and pushed `ws-agent-settled` `reason:"exited"`.
 */
export async function probeAgentLiveness(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
): Promise<boolean> {
  const client = record.client;
  if (!client) return false;
  try {
    await client.getState();
    return true;
  } catch {
    markAgentExited(pi, registry, record);
    return false;
  }
}

/**
 * Records that `record`'s child process is gone and tells the lead once. Safe
 * to call repeatedly — a record already cleared of its client pushes nothing
 * a second time.
 */
export function markAgentExited(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  opts?: { suppressTerminal?: boolean },
): void {
  if (!record.client) return;
  // Only an exit that cuts off work in flight is a failure: an idle child
  // whose finished work was already delivered may exit later without one.
  if (record.launching !== undefined || record.running || record.streaming) record.lastRunFailed = true;
  const hadQueuedSuccessor = (record.pendingQueuedWork?.length ?? 0) > 0;
  clearLiveState(record, registry);
  record.pendingQueuedWork = undefined;
  if (record.ownership) updateOwnership(record.ownership.home, { liveness: { lifecycle: "unknown", running: false, observedAt: Date.now() } });
  triggerAgentWidgetRefresh();
  if (opts?.suppressTerminal) return;
  const workGeneration = record.workGeneration;
  if (record.settlementAdmissionGeneration === workGeneration) {
    record.terminalDelivery?.retry?.();
    return;
  }
  record.settlementAdmissionGeneration = workGeneration;
  const lastMessage = !hadQueuedSuccessor && record.lastTextGeneration === workGeneration ? record.lastText : undefined;
  const terminal = createTerminalDelivery(record, registry, pi, workGeneration, { reason: "exited", last_message: lastMessage });
  terminal.retry?.();
}

/** Interval of the background liveness sweep, while at least one agent is outstanding. */
export const LIVENESS_PROBE_INTERVAL_MS = 30_000;

/**
 * Starts the periodic half of the liveness probe: every
 * `LIVENESS_PROBE_INTERVAL_MS`, probe each still-`running` record. Skipped
 * entirely while nothing is outstanding (N === 0), so an idle lead pays
 * nothing. `unref()`'d so a pending sweep never holds the process open.
 * Returns the stopper (`AgentToolsHandle.stopAll` calls it).
 */
export function startLivenessProbe(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry,
  intervalMs: number = LIVENESS_PROBE_INTERVAL_MS,
): () => void {
  const timer = setInterval(() => {
    for (const record of [...registry.values()]) {
      if ((record.running || record.streaming || record.waitingOnChildren || hasPendingTerminalDelivery(record)) && record.client) {
        void probeAgentLiveness(pi, registry, record);
      }
    }
  }, intervalMs);
  timer.unref?.();
  return () => clearInterval(timer);
}

/**
 * The `spawn-failed` half of `spawnAgent`'s launch-failure handling: put the
 * half-registered record into its resting state and tell the owning session
 * once, so the fan-in count is not left waiting on a child that never started.
 * Every persistent record, including a researcher, remains parked/dormant for
 * ordinary resume and sidecar retention. The caller re-throws the original
 * error unchanged afterwards.
 *
 * Extracted (review relay #1, test partition C2) so this branch has offline
 * coverage — `spawnAgent` itself constructs a real `RpcClient` and is
 * live-gate only.
 */
export function pushSpawnFailed(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  err: unknown,
): void {
  record.lastRunFailed = true;
  clearLiveState(record, registry);
  pushToLead(pi, registry, record, "ws-agent-settled", { reason: "spawn-failed", error: err instanceof Error ? err.message : String(err) }, "followUp");
  triggerAgentWidgetRefresh();
}

export interface SpawnAgentParams {
  /** Fork-family prompts live in their first user message; workers still require this path. */
  systemPromptPath?: string;
  prompt: string;
  modelName?: string;
  modelEffort?: string;
  /**
   * 260905 (alias/park/cap ticket): optional human-chosen short name for this
   * agent, resolved alongside the raw uuid by `resolveAgentId`. Reusing an
   * alias already held by a `running`/`threadBound` record rejects the spawn;
   * a dormant/idle holder's alias is overwritten (its `title` is untouched).
   * Never derived from `prompt` — the adapter only ever uses what the caller
   * passed.
   */
  alias?: string;
  /** 260905: optional free-text label, independent of `alias` — see `RpcAgentRecord.title`. */
  title?: string;
  /** Optional bounded native edit/write grants for a child that is otherwise restricted. */
  writeScopes?: WriteScope[];
  /** Optional absolute existing directory where the child process starts. */
  cwdOverride?: string;
}

export function validateCwdOverride(cwdOverride: string | undefined): string | undefined {
  if (cwdOverride === undefined) return undefined;
  if (!isAbsolute(cwdOverride)) throw new Error("ws-pi-agent: cwd_override must be an absolute path");
  try {
    if (!statSync(cwdOverride).isDirectory()) throw new Error("not a directory");
  } catch {
    throw new Error("ws-pi-agent: cwd_override must be an existing directory");
  }
  return cwdOverride;
}

export type WriteScopeDiagnostic =
  | { status: "bound"; count: number }
  | { status: "ignored"; reason: "child already has unrestricted native edit/write authority" };

export interface RpcSpawnCtx {
  /** Trusted bridge render (or internal preset), never a public spawn argument. */
  profile?: PlaybookProfile;
  provenance?: RenderProvenance;
  parentPolicy?: DelegationPolicy;
  /** Parent TUI only; initial task-fork observation, never persisted or resumed. */
  forkCacheNoticeOwner?: ForkCacheNoticeOwner;
  /**
   * 260905: the spawning session's own `ExtensionAPI`, needed so every signal
   * this child produces can be PUSHED back into that session (`pushToLead`).
   * Required rather than optional: a spawn with no push channel would leave
   * its child's reports unreachable now that `ws-agent-wait` is gone.
   */
  pi: ExtensionAPI;
  cwd: string;
  /** Exact `fileURLToPath(import.meta.url)` captured by the loaded manifest entry. */
  extensionPath: string;
  /** Immediate dispatcher's configured Pi home and stable current session identity. */
  storage?: AgentStorageContext;
  /** `provider/id`, forwarded from the calling tool-execute ctx.model, or undefined to inherit pi's own default. */
  inheritModel?: string;
  /** Current thinking level, used only when the caller explicitly requests model_effort:"default" with inherited-model dispatch. */
  inheritEffort?: string;
  /** Current execute/command context's live getAll + configured-auth catalog. */
  catalog: readonly ModelCatalogEntry[];
  /** ws-agent-spawn-only public contract; other spawnAgent consumers retain tier-only model_name behavior. */
  allowConcreteModel?: boolean;
  /** UI-only adapter; never a lifecycle push. */
  notifyTierWarning?: (warning: string) => void;
  /** Bridge's sanitized `ws__*` registered tool names, for the `full-worker` group. */
  wsToolNames: readonly string[];
  /** ws-mcp client used to resolve `model_name` through `config.resolve_agent` (`resolveModelForAliasViaWsMcp`), read fresh per spawn. */
  client: McpStdioClient;
  /** Curated `--tools` group for this spawn. Omitted (or explicit `"full-worker"`) preserves every existing `ws-agent-spawn` caller's behavior unchanged — only `ws-execute` (execute-gateway.ts) passes `"execute-worker"`. */
  toolGroup?: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): the lead's own session file to fork
   * from (`toolCtx.sessionManager.getSessionFile()`, `fork.ts`'s
   * `registerFork`). When set, `buildRpcClientOptions` emits `--fork
   * <forkFrom>` instead of `--session <sessionPath>` for THIS initial spawn
   * only — never threaded through `RpcResumeCtx`/`sendToAgent`'s
   * dormant-resume branch, which always resumes via the fork's own
   * already-discovered `sessionPath` (see `spawnAgent`'s `client.getState()`
   * overwrite below). `undefined` for every non-fork spawn (unchanged
   * `--session` behavior).
   */
  forkFrom?: string;
  /** Private launch-only source for Pi's not-yet-flushed first-turn branch. */
  forkSourceEntries?: unknown[];
  /**
   * 260904 Phase 1 (side-thread fork): pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` when set — see
   * `RpcAgentRecord.explicitTools`'s doc comment for the full rationale.
   * `undefined` for every non-fork spawn.
   */
  explicitTools?: string;
  /**
   * Immediate launching lead's default key, forwarded to the child's
   * WS_PI_PARENT_SESSION_KEY when its policy carries no parent. Forks also
   * use the env marker to refuse accidental use of the parent's key.
   */
  parentSessionKey?: string;
  /**
   * 260904 Phase 1: fired right after `attachEventListener` observes a
   * freshly-set `record.pendingApproval` on this spawn's record — the
   * approval-request-relay injection hook, now told whether a lead-side
   * waiter was already woken by this event (260904 Phase 2, re-scoped
   * 2026-09-05). `spawner.ts` stays generic (no `pi.sendUserMessage` import)
   * by taking this as a plain callback; `execute-gateway.ts`'s
   * `createApprovalRelay` is the only real implementation. Never fires for a
   * non-`"execute-worker"` spawn in practice, since `GATED_EXEC_TOOL_NAME` is
   * reachable only from that group's `--tools` list.
   *
   * 260905 dropped the `info: {waiterWoken}` argument: with `ws-agent-wait`
   * deleted there is no wait return for the relay to be a stale duplicate of,
   * so it now pushes unconditionally.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260906 Phase 2 (YAML/TUI dispatch-row rendering): fired once, right after
   * `spawnAgent` resolves the model (both refusal guards already passed) and
   * before any launch side effect (`runSpawnGuards`/`mkdtempSync`). Every
   * tool-level `spawnAgent` caller forwards this into its own `onUpdate`
   * partial `details.resolved` and repeats it in the final `details` so the
   * dispatch row can show the resolved model/effort before the child
   * finishes. Never invoked when a refusal guard throws first.
   */
  onModelResolved?: (resolved: ResolvedModelInfo) => void;
  /**
   * 260905: recorded on the record as `spawnRole` so the shutdown sidecar can
   * re-arm the right wiring on revival. Defaults to `"fork"` when `forkFrom`
   * is set, `"execute-worker"` for that tool group, `"worker"` otherwise.
   */
  spawnRole?: SpawnAgentRole;
  /** Fail closed before any guard/allocation for simple exploration. */
  requireTier?: boolean;
  /** Internal generated alias prefix; allocation scans the common registry. */
  aliasPrefix?: string;
  /** Persistent exploration metadata; never supplied by the public schema. */
  exploreMode?: ExploreMode;
  /** Immutable lead capture supplied only to a task/discussion fork. */
  forkContext?: ForkContext;
  /** Test seam for the control channel: forced backend and wait bounds. Production leaves it unset. */
  channel?: ChannelLaunchOptions;
}

/**
 * Control-channel knobs injected by tests. `bind` forces a backend (no env
 * knob exists on purpose); the timeouts shorten the hello/readiness waits.
 */
export interface ChannelLaunchOptions {
  bind?: ChannelBindOptions;
  helloTimeoutMs?: number;
  readinessTimeoutMs?: number;
}

export interface RpcResumeCtx {
  /** See `RpcSpawnCtx.pi`. Optional here only because a resume can be driven from a call site with no push channel of its own; pushes are then skipped rather than erroring. */
  pi?: ExtensionAPI;
  cwd: string;
  /** Exact loaded manifest entry path, propagated to a dormant child's replacement process. */
  extensionPath: string;
  /** See `RpcSpawnCtx.onApprovalPending` — threaded through `sendToAgent`'s dormant-auto-resume branch so a resumed `execute-worker`'s approval relay keeps working. */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I2): `true` only for the LEAD-facing
   * `ws-agent-send` tool. The lead driving a child directly is the headless
   * answer path for a fork-raised question — the fork was thread-bound from
   * registration and, with no owner surface to ever open or close that thread,
   * nothing else would release the bind before its own final. So a lead send
   * unbinds it: the exchange is now the lead's, and the child rejoins the
   * fan-in immediately rather than staying invisible until it completes.
   *
   * Deliberately NOT set by `ask.ts`'s overlay channel (`createForkChannel`),
   * which routes the OWNER's messages through this same function — an owner
   * typing into an open thread must leave the bind exactly as it is.
   */
  leadSend?: boolean;
  /** Attribution for the message. Omitted is a lead-side prompt. */
  writer?: "lead" | "owner";
  /** Internal token carried only by a coordinator-owned closeout send. */
  finishToken?: string;
  /** See `RpcSpawnCtx.channel`. */
  channel?: ChannelLaunchOptions;
  /**
   * The dispatching session's current own key
   * (`BridgeHandle.defaultSessionKeyRef.current`), read at send time. A
   * dormant non-fork relaunch re-stamps it as the child's ferrule parent
   * (`relaunchDelegation`). Omitted, the stored policy launches unchanged.
   */
  leadSessionKey?: string;
}

/**
 * Exported (review fix, cycle 1) so `test/spawner.test.ts` can assert the
 * `WS_PI_SPAWN_ROLE_ENV` marker directly against the built options object
 * instead of leaving it covered only by a manual spot-check — `RpcClient`'s
 * own `env: {...process.env, ...this.options.env}` merge means this function
 * only needs to carry the marker itself (no `process.env` spread here).
 *
 * 260904 Phase 1: carries `WS_PI_SPAWN_ROLE_ENV: "worker"` (see
 * `process-role.ts`), replacing the old boolean `WS_PI_AGENT_CHILD_ENV: "1"`
 * marker this function used to set. (The `WS_PI_APPROVAL_DIR` it once carried
 * is gone with the decision file: 260924 moved approval decisions onto the
 * control channel, so the child needs no approvals directory.)
 *
 * 260904 Phase 1 (side-thread fork) adds the `forkFrom`/`parentSessionKey`
 * params: when `forkFrom` is given, the emitted args swap `["--session",
 * sessionPath, ...]` for `["--fork", forkFrom, ...]` (initial spawn only —
 * `sendToAgent`'s dormant-resume call site never passes `forkFrom`) and the
 * role marker becomes `"fork"` instead of `"worker"`; `parentSessionKey`
 * (only meaningful alongside `forkFrom`) additionally sets
 * `WS_PI_PARENT_SESSION_KEY_ENV` on the child's env, the marker
 * `normalizeSessionKey` (bridge.ts) already rewrites an explicit sentinel
 * `session_key` from — the fork's own bridge instance uses it to mint its
 * own lead-scope key instead of a fresh one. `--fork`'s exact composition
 * with `--mode rpc`/`--tools`/`--append-system-prompt` and its at-leaf vs.
 * before-a-message clone semantics is the ticket's own named live-
 * verification item (not resolvable offline) — this function only builds
 * the argv, it does not confirm Pi's own `--fork` behavior.
 *
 * 260906 (lead explore as an async RPC child) adds the trailing
 * `spawnRoleOverride` param: when given, it wins outright over the
 * `forkFrom ? "fork" : "worker"` default — the lead/fork `explore` preset is
 * the only caller that passes one (`"explore"`, from `process-role.ts`'s
 * `SpawnRole`), so its RPC child's own env reads as an explore leaf rather
 * than a plain worker. `undefined` (every other call site: `spawnAgent`'s
 * non-explore spawns, `sendToAgent`'s dormant-resume branch) leaves the
 * existing default completely unchanged.
 */
export function prepareForkLaunch(context: ForkContext | undefined) {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-launch-"));
  const contextPath = join(directory, "context.json");
  writePrivateJson(contextPath, { context: context ? captureForkContext(context) : undefined, ...(context ? {} : { legacy: true }) });
  return { contextPath, affinityId: context?.parentAffinityId };
}

/** Bounds on the child's channel stages. 30 s matches `RpcClient`'s own per-request timeout, so a hung child is reported once, not twice. */
export const CHANNEL_HELLO_TIMEOUT_MS = 30_000;

export const CHANNEL_READINESS_TIMEOUT_MS = 30_000;

const CHANNEL_LIVENESS_PROBE_MS = 500;

/**
 * Waits for one channel stage (hello or a readiness kind) while observing the
 * child's lifecycle: the wait ends on the message, on `timeoutMs`, or as soon
 * as the child process is gone. Liveness is probed the same way
 * `probeAgentLiveness` does, through `getState()`; a rejection counts as an
 * exit only when `RpcClient` recorded one (`exitError`), so a slow first
 * response never masquerades as a crash.
 */
export async function awaitChannelStage<T>(
  client: RpcClient,
  pending: Promise<T>,
  timeoutMs: number,
  stage: string,
  timeoutError: () => Error = () => new Error(`ws-pi-agent: child channel ${stage} timed out after ${timeoutMs}ms`),
  exitError: (reason: string) => Error = reason => new Error(`ws-pi-agent: child exited before channel ${stage}: ${reason}`),
): Promise<T> {
  let timer: NodeJS.Timeout | undefined;
  let probing = true;
  const timeout = new Promise<never>((_, reject) => { timer = setTimeout(() => reject(timeoutError()), timeoutMs); timer.unref?.(); });
  const exited = (async (): Promise<never> => {
    while (probing) {
      try { await client.getState(); } catch (error) {
        const recorded = recordedExitError(client);
        if (recorded) throw exitError(recorded instanceof Error ? recorded.message : String(recorded ?? error));
      }
      if (!probing) break;
      await new Promise(resolve => { const nap = setTimeout(resolve, CHANNEL_LIVENESS_PROBE_MS); nap.unref?.(); });
    }
    return new Promise<never>(() => { /* stage settled elsewhere */ });
  })();
  exited.catch(() => { /* raced away; the rejection reached the race when it mattered */ });
  try { return await Promise.race([pending, timeout, exited]); }
  finally { probing = false; clearTimeout(timer); }
}

/** Stage-2 waits keep today's failure texts: a timeout reads exactly as the missing file used to, and a child exit keeps that text as its prefix. */
export function awaitForkReadiness(client: RpcClient, channel: ParentChannel, opts: ChannelLaunchOptions | undefined): Promise<unknown> {
  const text = "ws-pi-agent: fork did not publish readiness";
  return awaitChannelStage(client, channel.readiness(FORK_READINESS_KIND), opts?.readinessTimeoutMs ?? CHANNEL_READINESS_TIMEOUT_MS, "fork readiness",
    () => new Error(text), reason => new Error(`${text} (child exited before channel fork readiness: ${reason})`));
}

export function awaitWebReadiness(client: RpcClient, channel: ParentChannel, opts: ChannelLaunchOptions | undefined): Promise<unknown> {
  const text = "web-search-tool-unavailable: Explore web facade readiness was not proved";
  return awaitChannelStage(client, channel.readiness(WEB_READINESS_KIND), opts?.readinessTimeoutMs ?? CHANNEL_READINESS_TIMEOUT_MS, "web readiness",
    () => new Error(text), reason => new Error(`${text} (child exited before channel web readiness: ${reason})`));
}

export function validateForkReadiness(payload: unknown, record: RpcAgentRecord, state: { sessionFile?: string; sessionId?: string }): void {
  if (!payload || typeof payload !== "object") throw new Error("ws-pi-agent: fork did not publish readiness");
  const ready = payload as Partial<ForkReadiness>;
  if (!ready.ownSessionKey?.trim() || ready.error ||
      !state.sessionFile || ready.sessionPath !== state.sessionFile || !ready.sessionId ||
      ready.ownSessionKey === record.forkContext?.parentSessionKey || record.forkContext?.parentSessionKeys?.includes(ready.ownSessionKey) || ready.sessionId === record.forkContext?.parentPiSessionId ||
      (state.sessionId && ready.sessionId !== state.sessionId)) {
    throw new Error(`ws-pi-agent: fork readiness rejected (${ready.error ?? "key/session mismatch"})`);
  }
  if (record.forkContext) {
    const mismatch = !Array.isArray(ready.registeredTools)
      ? "missing callable tool registrations"
      : compareForkRegistrations(record.forkContext.registeredTools, ready.registeredTools)
        ?? (JSON.stringify(ready.activeTools) !== JSON.stringify(record.forkContext.activeTools) ? "reordered callable tools" : undefined);
    if (mismatch) throw new Error(`ws-pi-agent: fork readiness rejected (${mismatch})`);
  }
  if (record.ownership && !containedOwnedPath(record.ownership.home, state.sessionFile)) throw new Error("ws-pi-agent: fork readiness rejected (session escaped owned home)");
  record.sessionPath = state.sessionFile;
  record.ownSessionKey = ready.ownSessionKey;
  if (record.ownership) { record.ownership = { ...record.ownership, sessionPath: state.sessionFile }; const metadata = readOwnership(record.ownership.home); try { if (metadata) writeOwnership({ ...metadata, sessionPath: state.sessionFile, updatedAt: Date.now(), liveness: { ...metadata.liveness, lifecycle: "live", running: true, observedAt: Date.now() } }); } catch { /* durable facts remain conservative; readiness stays usable */ } }
}

function containedOwnedPath(home: string, candidate: string): boolean {
  return isOwnedSessionPath(home, candidate);
}

export async function captureForkSelection(client: RpcClient, record: RpcAgentRecord): Promise<void> {
  const state = await client.getState();
  if (state.model?.provider && state.model.id) record.modelBase = `${state.model.provider}/${state.model.id}`;
  if (typeof state.thinkingLevel === "string") record.modelEffort = state.thinkingLevel;
}

export function buildRpcClientOptions(
  cwd: string,
  model: string | undefined,
  sessionPath: string,
  systemPromptPath: string | undefined,
  tools: string,
  forkFrom?: string,
  parentSessionKey?: string,
  spawnRoleOverride?: SpawnRole,
  exploreMode?: ExploreMode,
  forkLaunch?: { contextPath: string; affinityId?: string },
  extensionPath: string,
  delegation?: DelegationPolicy,
  channel?: ParentChannel,
): RpcClientOptions {
  if (!extensionPath) throw new Error("ws-pi-agent: missing loaded extension entry path for RPC child");
  const role = spawnRoleOverride ?? (forkFrom ? "fork" : "worker");
  forkLaunch = role === "fork" ? forkLaunch : undefined;
  const env: Record<string, string> = { [WS_PI_SPAWN_ROLE_ENV]: role };
  // RpcClient merges this object over process.env. An explicit empty marker
  // therefore clears an inherited deep mode for every non-research launch.
  env[WS_PI_EXPLORE_MODE_ENV] = role === "explore" && exploreMode ? exploreMode : "";
  // Explicitly clear fork context markers for worker/explore descendants.
  env[WS_PI_FORK_CONTEXT_ENV] = forkLaunch?.contextPath ?? "";
  env[WS_PI_FORK_AFFINITY_ENV] = forkLaunch?.affinityId ?? "";
  // Every child launch carries its immediate parent, never an inherited ancestor.
  env[WS_PI_PARENT_SESSION_KEY_ENV] = delegation?.parentSessionKey ?? parentSessionKey ?? "";
  env[DELEGATION_ENV] = delegation ? JSON.stringify(delegation) : "";
  env[WEB_HOME_ENV] = role === "explore" ? dirname(sessionPath) : "";
  // The control-channel bootstrap: the child reads and deletes these before it
  // spawns anything of its own. Cleared explicitly so a grandchild never sees
  // its grandparent's endpoint through inheritance.
  for (const key of CHANNEL_BOOTSTRAP_ENVS) env[key] = "";
  if (channel) Object.assign(env, channel.bootstrapEnv());
  // RpcClient overlays env onto process.env, so deletion here would preserve a
  // stale parent value. Empty values neutralize forced bootstrap selection.
  for (const override of CHILD_BOOTSTRAP_OVERRIDE_ENVS) env[override] = "";
  // Blank is inert to ws-mcp's mailbox identity resolution; forks included.
  for (const key of CHILD_MAILBOX_IDENTITY_ENVS) env[key] = "";
  const args = forkFrom ? ["--fork", forkFrom] : ["--session", sessionPath];
  args.push("--session-dir", dirname(sessionPath));
  if (role !== "fork" && systemPromptPath) args.push("--append-system-prompt", systemPromptPath);
  // Do not let Pi discover an installed/cache copy alongside the exact parent
  // entry we explicitly load; `--tools` remains the sole active-surface gate.
  args.push("--no-extensions", "--extension", extensionPath, "--tools", tools);
  return {
    cliPath: RPC_CLI_PATH,
    cwd,
    env,
    model,
    args,
  };
}

/**
 * Appends one observed `ws-report-to-lead` call to `record.reportLog`,
 * drop-oldest past `REPORT_LOG_CAP`. `kind` is omitted from the entry
 * entirely (not stored as an explicit `undefined` property) when the caller
 * omits it, so a plain progress update round-trips as `{at}`.
 */
export function recordReport(record: RpcAgentRecord, kind: "question" | undefined, at: number = Date.now()): void {
  record.reportLog.push(kind === undefined ? { at } : { kind, at });
  if (record.reportLog.length > REPORT_LOG_CAP) {
    record.reportLog.shift();
  }
  if (record.ownership) touchOwnership(record.ownership.home);
}

/** Writes durable protection without treating a poll as activity. A new owner bind must check success before committing local state. */
export function syncOwnershipProtection(record: RpcAgentRecord): boolean {
  if (!record.ownership) return true;
  return updateOwnership(record.ownership.home, {
    liveness: {
      lifecycle: record.client ? (record.running ? "live" : "stopping") : "unknown",
      running: record.running,
      observedAt: Date.now(),
      threadBound: record.threadBound,
      ownerHeld: isOwnerHeld(record),
      waitingOnChildren: record.waitingOnChildren,
      pendingDelivery: hasPendingTerminalDelivery(record),
      pendingQuestion: record.threadBound,
      pendingApprovalCommandId: record.pendingApproval?.cmdId,
      recovery: record.client ? "none" : "revived",
    },
  }) !== undefined;
}

/** Unreferenced persistent-record observer; unchanged polling never renews activity. */
export function startOwnedSessionObserver(record: RpcAgentRecord, intervalMs = 5_000): void {
  if (!record.ownership || record.ownershipObserverStop) return;
  const sample = () => observeSessionWrite(record.ownership!.home, record.sessionPath);
  sample();
  const timer = setInterval(sample, intervalMs);
  timer.unref();
  record.ownershipObserverStop = () => { clearInterval(timer); record.ownershipObserverStop = undefined; };
}
