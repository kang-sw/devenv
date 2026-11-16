import { StringDecoder } from "node:string_decoder";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ExploreMode } from "./process-role.ts";
import type { ForkContext } from "./fork-context.ts";
import { ParentChannel } from "./agent-channel.ts";
import { persistOwnershipDescendantUsage, persistOwnershipTelemetry, type AgentOwnership } from "./agent-storage.ts";
import { IncrementalSessionReader, reduceTelemetry, type AgentTelemetry, type CumulativeCost, type TelemetryOrigin } from "./agent-telemetry.ts";
import { descendantUsageOf, evaluateDescendantUsage } from "./agent-usage-rollup.ts";
import type { DelegationPolicy } from "./delegation-policy.ts";
import type { OwnTurnState, SubtreeDescendant, SubtreeSnapshot } from "./subtree-lifecycle.ts";
import type { OutputRateTracker } from "./output-rate.ts";
import type { ModelObservation } from "./model-observation.ts";
import type { AgentBatchBuffer } from "./agent-batches.ts";
import type { AgentViewModel } from "./agent-view.ts";
import type { ToolGroup } from "./spawner-tools.ts";
import type { ForkFinishOperation, TerminalDelivery } from "./spawner.ts";

/**
 * One gated command awaiting the lead's decision (see
 * `RpcAgentRecord.pendingApproval`). `decision` is absent while the request
 * is open, `"sent"` once `ws-approve` delivered a decision over the live
 * connection (a second `ws-approve` is rejected until the child acknowledges
 * consumption), and `"discarded"` when that connection ended before the
 * acknowledgment — the child's reconnect hello then decides whether the
 * command was consumed (`attachApprovalChannel`). `reissued` marks a request
 * pushed to the lead again after such a discard, and `issue` counts those
 * pushes (absent reads as 0, the original request): a held
 * `ws-agent-approval` push stays actionable only while the issue it was built
 * for is still the current one (`heldActionState`).
 */
export interface PendingApprovalState {
  cmdId: string;
  command: string;
  rationale?: string;
  /** The worker-supplied per-call cwd override; see `resolveApprovalContextCwd` (execute-gateway.ts). */
  cwd?: string;
  decision?: "sent" | "discarded";
  reissued?: boolean;
  issue?: number;
}

export interface RpcAgentRecord {
  agentId: string;
  /**
   * 260905 (alias/park/cap ticket): caller-supplied, human-chosen short name
   * for THIS agent record — a different concept from
   * `resolveModelForAliasViaWsMcp`'s "alias," which names a fixed *tier*
   * resolved through ws-mcp's `config.resolve_agent` tool (see above).
   * Optional; the adapter never derives one from the prompt. Resolved
   * through `resolveAgentId` alongside the raw `agentId` uuid on every
   * `ws-agent-send`/`ws-agent-stop`/`ws-agent-transcript`/`ws-approve` call.
   * Reusing an alias on a new spawn overwrites a dormant/idle holder (clearing
   * the holder's `alias`, not its `title`) or rejects the spawn outright when
   * the holder is `running`/`threadBound` — see `spawnAgent`.
   */
  alias?: string;
  /**
   * 260905: caller-supplied free-text label, independent of `alias` — never
   * used for resolution, purely descriptive (roll-call/list display). Reusing
   * an `alias` on a new spawn clears the prior holder's `alias` but leaves its
   * `title` untouched.
   */
  title?: string;
  /** The live RPC child, or `undefined` when dormant (stopped but resumable — D-C). */
  client?: RpcClient;
  /** Absolute path to the ws-owned `--session` file, reused unchanged across every (re)start. */
  sessionPath: string;
  /** Durable-home ownership. Absent only for pre-retention legacy records. */
  ownership?: AgentOwnership;
  /** Lead-rendered playbook prompt path, passed via `--append-system-prompt`; reused unchanged across resumes (no re-render). */
  systemPromptPath?: string;
  /** Resolved `provider/id`, or undefined to inherit pi's own default resolution. Cached so a dormant resume reuses the same model. */
  modelBase?: string;
  /** Effective thinking override, applied via `setThinkingLevel()` after every (re)start; absent means Pi/model default. */
  modelEffort?: string;
  /** Observed child selection and recomputed durable usage; launch intent stays above. */
  telemetry?: AgentTelemetry;
  /** Legacy-fork floor: permits post-launch context occupancy, never lifetime cost. */
  telemetryContextFloor?: TelemetryOrigin;
  /**
   * Live-only incremental reader of this child's `session.jsonl` for
   * `refreshAgentTelemetry`, so a refresh parses only the bytes appended since
   * the last one. Never persisted; dropped with the record when it leaves the
   * registry so a finished child's projections are not retained.
   */
  telemetryReader?: IncrementalSessionReader;
  observedModel?: string;
  observedEffort?: string;
  /** Context occupancy for a legacy fork whose child-attribution boundary is unavailable. */
  observedContextTokens?: number;
  /**
   * 260906 Phase 2 (YAML/TUI dispatch-row rendering): the raw model selection
   * requested at spawn, or `undefined` when omitted — display-only,
   * reconstructs `ws-agent-send`'s resolved-model line
   * for a target agent without a live resolution. NOT persisted to the
   * sidecar (`agent-sidecar.ts`); a record revived after a restart carries
   * neither field (see `modelSource`'s doc comment for the degrade rule).
   */
  modelTier?: string;
  /**
   * 260906 Phase 2: mirrors `TierResolution.source` at spawn time
   * (`"tier" | "concrete" | "inherit"`). NOT persisted to the sidecar — a `ws-agent-send`
   * to a record revived from a sidecar snapshot after a restart finds this
   * field `undefined` and treats that as `"inherit"` (the safe default),
   * never throwing or guessing a tier name.
   */
  modelSource?: "tier" | "concrete" | "inherit";
  /** Explicit spawn cwd, retained so a dormant resume launches in the same directory. */
  cwdOverride?: string;
  /** Cached bridge `ws__*` tool names, for `--tools` re-resolution on a dormant resume. */
  wsToolNames: readonly string[];
  /** Curated `--tools` group this record was spawned with; reused unchanged on a dormant resume so `resolveTools` never silently widens/narrows a resumed child's tool surface. Set at spawn (`ctx.toolGroup ?? "full-worker"`), never mutated afterward. */
  toolGroup: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): a pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` entirely when set —
   * a fork's tool surface is dynamic (`computeForkToolSurface` over the
   * lead's own `pi.getActiveTools()` at spawn time, `fork.ts`), not one of
   * the static `TOOL_GROUPS` entries `toolGroup` indexes. Cached verbatim at
   * spawn and reused unchanged on every dormant resume (mirrors
   * `systemPromptPath`/`modelBase`'s existing cache-and-reuse contract).
   * `undefined` for every non-fork spawn — those keep resolving tools from
   * `toolGroup`/`wsToolNames` exactly as before.
   */
  explicitTools?: string;
  /** Immutable task/discussion capture retained across dormant recovery. */
  forkContext?: ForkContext;
  /**
   * 260905: which spawn shape produced this record, recorded at spawn time
   * rather than re-derived from `toolGroup`/`explicitTools` heuristics. Read
   * by the shutdown sidecar (`agent-sidecar.ts`) so a `session_start` revival
   * can re-arm the right role wiring for a resurrected orphan.
   */
  spawnRole?: SpawnAgentRole;
  /** Persisted authority and one-edge semantic lifecycle; independent of owner holds. */
  delegation?: DelegationPolicy;
  /**
   * Parent side of the per-launch control channel (`agent-channel.ts`), bound
   * before the child process is spawned and closed by `clearLiveState` with
   * every other live-only field. Fresh endpoint and credential per launch; the
   * child's hello, stage-2 readiness, and subtree snapshots travel over it
   * instead of files.
   */
  channel?: ParentChannel;
  /** Bumped once per process launch (spawn, dormant resume, relaunch); the channel accepts only this generation. */
  launchGeneration?: number;
  /**
   * The child's last accepted descendant-usage report (`agent-usage-rollup.ts`):
   * everything below it, excluding its own usage. Persisted as the ownership
   * record's sibling `descendantUsage` field, outside `telemetry`, so a
   * telemetry reset cannot drop it.
   */
  descendantUsage?: CumulativeCost;
  /** Live-only ordering state for descendant-usage reports: the accepted launch generation and its highest sequence. */
  descendantUsageOrder?: { generation: number; seq: number };
  /**
   * Settles (never rejects) when the launch that claimed `client` has either
   * reached its first prompt or failed. A send that lands in between waits on
   * it rather than prompting a client whose process has not started.
   */
  launching?: Promise<void>;
  /** Last advisory identity tree published by this child process. */
  subtreeDescendants?: SubtreeDescendant[];
  /**
   * 261028: bounded recent tool batches of this direct child, fed by
   * `attachEventListener`'s `client.onEvent` handler (`observeBatchEvent`)
   * and rendered by the live agent widget's expanded mode. Live-only, never
   * persisted.
   */
  toolBatches?: AgentBatchBuffer;
  /**
   * 261028 Phase 2: the batch model this child last relayed for its own
   * descendants over the launch's channel (`agent-view.ts`). Dropped when the
   * child disconnects or exits; live-only, never persisted.
   */
  descendantViews?: AgentViewModel;
  /** Parent's view of this child's subtree (`observeSubtreeChannel`): true while unknown, disconnected, or busy. */
  waitingOnChildren?: boolean;
  /**
   * The child's own ws session key, known only for a fork (its stage-2
   * readiness carries it). Live-only, never persisted; the dashboard feed
   * hands it to ws-mcp, which shows the delegate by its reply-id instead.
   */
  ownSessionKey?: string;
  /** Live-only: the current work ended by its process exiting or its launch failing. Cleared when new work starts. */
  lastRunFailed?: boolean;
  /** Last successful writer to this child. Absence is the legacy/lead default. */
  lastWriter?: "lead" | "owner";
  /** Owner-authored sends, in delivery order, used to attribute persisted user entries. */
  ownerSends?: Array<{ text: string; at: number }>;
  /** Last subtree revision accepted from the current launch; scoped to `launchGeneration`. */
  subtreeRevision?: number;
  workGeneration?: number;
  /** Work generation whose ordinary settlement has entered terminal admission. */
  settlementAdmissionGeneration?: number;
  /**
   * Set by `attachEventListener` for the attached client: re-runs a settlement
   * that was held (on `waitingOnChildren`, or on `subtreeTurn` saying a turn is
   * still coming) once `observeChildSubtree` applies a not-waiting snapshot
   * whose wake accounting says no wake turn is coming. No-op when nothing is
   * held.
   */
  releaseSettlementHold?: (snapshot: SubtreeSnapshot) => void;
  /**
   * Work generation whose settle `attachEventListener` holds for
   * `releaseSettlementHold`. A held settle is a terminal still to come, so it
   * counts as outstanding (`subtreeOutstanding`). Cleared on admission and
   * with the launch (`clearLiveState`).
   */
  heldSettlementGeneration?: number;
  /**
   * Wake accounting of the latest subtree snapshot accepted from the current
   * launch (`observeChildSubtree`); undefined until that launch's first
   * snapshot, and for a channel-less child.
   */
  subtreeTurn?: OwnTurnState;
  /** Persistent exploration mode; meaningful only for explore records. */
  exploreMode?: ExploreMode;
  /** `true` while an agent run is actively looping (between `agent_start` and `agent_settled`). */
  streaming: boolean;
  /**
   * 260905 fan-in bookkeeping: `true` from the instant a prompt is ISSUED to
   * this child (`promptAgent`, i.e. before any `agent_start` event can arrive)
   * until it settles, is stopped, exits, or fails to spawn. Deliberately a
   * complementary flag to event-confirmed `streaming`; both mean executable
   * work is in flight and feed list/widget/fan-in status consistently.
   */
  running: boolean;
  /**
   * Epoch-ms anchor for the current run. It is set when a run begins, remains
   * stable across nudges, and is read with `settledAt` by the gutter and audit
   * picker to show that run's duration.
   */
  runStartedAt?: number;
  /** Epoch-ms terminal boundary for the current run, cleared when a new run begins. */
  settledAt?: number;
  /**
   * Epoch-ms high-water mark of observed agent output only: tool execution or
   * streamed assistant text/thinking. The gutter and audit picker both read
   * this through `lastActivityAt`; lead/owner inputs never advance it.
   */
  lastOutputAt?: number;
  /** In-memory visible-output decode rate for this launch; replaced on every (re)start, never persisted. */
  outputRate?: OutputRateTracker;
  /** Completed output evidence, separate from RPC-state observedModel/effort. */
  modelObservations?: ModelObservation[];
  /** Epoch-ms stamp of the last lead-issued prompt, retained for attribution, not activity display. */
  lastLeadPromptAt?: number;
  /**
   * 260905: `true` for the whole lifetime of an owner discussion thread bound
   * to this agent — set by `ask.ts` on every open/reopen (`ensureRespondent`/
   * `openThread`) and on fork-raised question registration
   * (`handleForkRaisedQuestion`), cleared only when the thread actually closes
   * (`/done`, lead takeover, or `ws-withdraw-question`). While set, settled
   * output routes to the owner surface and the record is left out of the lead
   * fan-in status line: the exchange belongs to the owner, not the lead.
   */
  threadBound?: boolean;
  /** Same-process `/done` coordinator for a fork-raised owner thread; never persisted. */
  forkFinish?: ForkFinishOperation;
  /** Existing terminal push admission for the current work, if one exists. */
  terminalDelivery?: TerminalDelivery;
  /** Last assistant text observed from this work generation's message_end event. */
  lastText?: string;
  /** Work generation that produced lastText; absent text is never borrowed across turns. */
  lastTextGeneration?: number;
  /** Accepted steer/follow-up instructions awaiting their actual user message_start boundary. */
  pendingQueuedWork?: Array<{ message: string }>;
  /**
   * 260905: the head-truncated (`truncatePromptForStorage`,
   * `PROMPT_STORAGE_CAP_BYTES`) copy of the spawn's initial `prompt`, stashed
   * for `ws-agent-list`'s opt-in `include_prompt` reply and the shutdown
   * sidecar. Set once at spawn, never updated by later `ws-agent-send` calls.
   */
  prompt?: string;
  /** Detaches the current `client.onEvent(...)` listener; re-armed on every (re)start. */
  unsubscribe?: () => void;
  /**
   * 260905: bounded history of `ws-report-to-lead` observations on this
   * record — kind and timestamp only, never the text (the text is pushed to
   * the lead immediately and never retained). Replaces the deleted
   * `pendingReports` FIFO as the input to `fork.ts`'s `isIdleWithoutFinal`
   * and as `ws-agent-list`'s last-report time. Capped at `REPORT_LOG_CAP`
   * (drop-oldest) so a long-lived chatty child cannot grow it without bound.
   */
  reportLog: AgentReportLogEntry[];
  /**
   * 260905 (list-model/last-report-fidelity ticket): the sidecar-revival-only
   * fallback for `last_report_at` — `rehydrateOrphanRecord` fills this from
   * `PersistedOrphan.lastReportAt` (the newest `reportLog` entry at
   * shutdown, already an ISO string). `listAgents` and `evictForCapacity`
   * both prefer a real `reportLog` entry over this value whenever one is
   * present, so a revived record that has reported since falls back to its
   * own history rather than the stale shutdown snapshot.
   */
  lastReportAtOverride?: string;
  /**
   * 260904 Phase 1: set by `applyRpcEvent` the instant a `tool_execution_start`
   * for `GATED_EXEC_TOOL_NAME` is observed on this record's child. `undefined`
   * means "no gated command is currently awaiting lead approval on this
   * agent" — the condition `validatePendingApproval` (execute-gateway.ts)
   * rejects against.
   *
   * 260924 (channel-delivered decisions): `ws-approve` sends the decision over
   * the launch's control channel and marks `decision: "sent"`; the record is
   * released only by the child's consumption acknowledgment, its reconnect
   * hello no longer reporting the `cmd_id`, or the child's exit
   * (`attachApprovalChannel`, `clearLiveState`). A disconnect before the
   * acknowledgment turns "sent" into "discarded": that decision is never
   * re-sent, and when the reconnect hello still reports the `cmd_id` the
   * request is pushed to the lead again with `reissued: true`. Ownership
   * protection (`pendingApprovalCommandId`) therefore holds from the request
   * until consumption, not until the decision is sent.
   *
   * Review fix (relay #1): also carries `cwd`, captured from the gated-exec
   * tool call's own `args.cwd` override when the worker supplied one (the
   * same `ws-worker-exec` `cwd?` param `execute-gateway.ts`'s `execute()`
   * itself falls back on via `p.cwd ?? sessionCtx.cwd`) — the approval-relay
   * callback (`createApprovalRelay`) must scrape the SAME directory the
   * command will actually run in, not unconditionally the worker's base
   * `sessionCtx.cwd`, or a `cwd`-overridden command's ground-truth git
   * context (branch/dirty/ahead_behind) would silently describe the wrong
   * directory to the lead.
   */
  pendingApproval?: PendingApprovalState;
  /**
   * Dormant: nothing arms this since the owner-question wiring was
   * disconnected; only `questionReportOutcome` (itself uncalled) reads it.
   *
   * 260904 Phase 2 (review relay #1 I6): consulted by `applyRpcEvent` the
   * instant a `kind:"question"` report is observed on this record. It may
   * return a REPLACEMENT message to enqueue for the lead in place of the
   * fork's own question text; returning `undefined` enqueues the original
   * unchanged (the headless baseline, byte-identical to Phase 1).
   *
   * §1 says the lead is not involved in a fork-raised question and §8 scopes
   * the lead relay to headless, so in TUI mode `ask.ts` registers the thread
   * on the owner surface and returns that notice — which, since 260905, is
   * the LEAD NOTICE to push: a defined return means the question itself is
   * answered on the owner surface, not by the lead, but the returned string
   * is still pushed to the lead as a `ws-agent-advisory`/`fork-question-thread`
   * message in place of the `ws-agent-question` the headless baseline would
   * send. Set by `fork.ts`'s `registerFork`; `spawner.ts`
   * stays generic and supplies no implementation, mirroring
   * `onApprovalPending`'s existing callback-injection convention.
   */
  onQuestionReport?: (record: RpcAgentRecord, message: string) => string | undefined;
  /**
   * 260908 same-process finish callback, owned by ask.ts. It persists the
   * ordinary thread snapshot only after this coordinator parks the fork.
   */
  onForkFinishComplete?: (record: RpcAgentRecord, failed?: string) => void;
  /**
   * 260905 (review relay #1, I1): fired by `sendToAgent`'s dormant-resume
   * branch right after the fresh client's event listener is attached. It
   * exists for role wiring that needs a LIVE client and therefore cannot be
   * re-armed at revival time — `fork.ts`'s `wireAntiBleedLoop`, which
   * subscribes to `client.onEvent` and returns early when the record is
   * dormant. A record revived from the shutdown sidecar carries this so its
   * first `ws-agent-send` restores the fork wiring rather than silently
   * degrading it to plain-worker behavior. Hook-only state, never serialized.
   */
  onResume?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I1): a per-RECORD fallback for the approval relay
   * that `RpcSpawnCtx`/`RpcResumeCtx` normally carry per CALL SITE.
   * `attachEventListener` prefers the ctx callback and falls back to this one,
   * so an `execute-worker` keeps its relay even when it is resumed from a call
   * site that has none — the shutdown sidecar's role-keyed revival sets it
   * (making the re-arm explicit rather than a coincidence of which resume path
   * ran), and `ask.ts`'s overlay channel resume, which passes no
   * `onApprovalPending` of its own, gets it for free.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

interface AgentContextStats {
  sessionFile?: string;
  sessionId?: string;
  contextUsage?: { tokens?: unknown };
}

type ContextReading = { kind: "value"; tokens: number } | { kind: "unknown" } | { kind: "unavailable" };

function contextReading(stats: AgentContextStats | undefined, path: string, sessionId: string | undefined): ContextReading {
  if (!stats || !sessionId || stats.sessionId !== sessionId || stats.sessionFile !== path || stats.contextUsage === undefined) return { kind: "unavailable" };
  const tokens = typeof stats.contextUsage.tokens === "number" && Number.isFinite(stats.contextUsage.tokens) && stats.contextUsage.tokens >= 0
    ? stats.contextUsage.tokens
    : undefined;
  return tokens === undefined ? { kind: "unknown" } : { kind: "value", tokens };
}

function nextContextTokens(previous: number | undefined, fallback: number | undefined, reading: ContextReading, refreshContext = true): number | undefined {
  if (!refreshContext) return previous;
  if (reading.kind === "value") return reading.tokens;
  if (reading.kind === "unknown") return previous;
  return fallback ?? previous;
}

export async function getAgentRpcSnapshot(client: RpcClient, includeStats = true): Promise<{ state: Awaited<ReturnType<RpcClient["getState"]>>; stats?: AgentContextStats }> {
  const state = await client.getState();
  if (!includeStats || typeof client.getSessionStats !== "function") return { state };
  try { return { state, stats: await client.getSessionStats() }; }
  catch { return { state }; }
}

/** Binds once before the first prompt and only recomputes from durable IDs thereafter. */
export function refreshAgentTelemetry(
  record: RpcAgentRecord,
  state?: { sessionFile?: string; sessionId?: string; model?: { provider?: string; id?: string }; thinkingLevel?: string },
  opts?: { fresh?: boolean; stats?: AgentContextStats; refreshContext?: boolean },
): boolean {
  const snapshot = () => JSON.stringify({ telemetry: record.telemetry, floor: record.telemetryContextFloor, model: record.observedModel, effort: record.observedEffort, context: record.observedContextTokens });
  const before = snapshot();
  // The reported descendant value is not derived from this session: it
  // survives every reset of the own-usage telemetry below.
  const descendants = descendantUsageOf(record);
  const finish = (): boolean => {
    if (descendants) record.descendantUsage = descendants;
    // The legacy location is read, never written: the value lives in the
    // record field and the ownership record's sibling field.
    if (record.telemetry) delete record.telemetry.descendantUsage;
    const changed = before !== snapshot();
    // Compared against the persisted record, not `changed`: the snapshot covers
    // more than the durable telemetry, and a failed write must retry next refresh.
    // The telemetry write migrates a legacy on-disk copy before dropping it.
    if (record.ownership) persistOwnershipTelemetry(record.ownership.home, record.telemetry);
    // Retry a failed report write, but only for a value accepted live in this
    // process: a revived record's value came from disk or an older snapshot.
    if (record.ownership && record.descendantUsageOrder && record.descendantUsage) persistOwnershipDescendantUsage(record.ownership.home, record.descendantUsage);
    // Evaluation point 1: this hop's own reduction of a direct child ran.
    evaluateDescendantUsage();
    return changed;
  };
  const path = state?.sessionFile ?? record.sessionPath;
  const read = (record.telemetryReader ??= new IncrementalSessionReader()).read(path);
  const sessionId = state?.sessionId ?? (read && !("transient" in read) ? read.headerId : undefined);
  const reading = contextReading(opts?.stats, path, sessionId);
  const model = state?.model?.provider && state.model.id ? `${state.model.provider}/${state.model.id}` : undefined;
  if (model) record.observedModel = model; else if (state) delete record.observedModel;
  if (typeof state?.thinkingLevel === "string" && state.thinkingLevel) record.observedEffort = state.thinkingLevel; else if (state) delete record.observedEffort;
  if (!record.telemetry) {
    if (!sessionId) return finish();
    // A non-fork legacy child has no inherited history and can be recovered
    // completely. A fork without its saved boundary must remain unknown.
    if (read && !("transient" in read) && read.parentSession && opts?.fresh) {
      const anchor = read.entries.at(-1)?.id;
      if (!anchor) return finish();
      record.telemetry = { version: 1, origin: { sessionId, sessionPath: path, prefixEntryId: anchor } };
    } else if (read && !("transient" in read) && read.parentSession && !opts?.fresh) {
      const existingFloor = record.telemetryContextFloor;
      if (existingFloor && (existingFloor.sessionId !== sessionId || existingFloor.sessionPath !== path)) {
        delete record.telemetryContextFloor;
        delete record.observedContextTokens;
      }
      if (!record.telemetryContextFloor) record.telemetryContextFloor = { sessionId, sessionPath: path, ...(read.entries.at(-1)?.id ? { prefixEntryId: read.entries.at(-1)!.id } : { emptyPrefix: true }) };
      const floor = reduceTelemetry(record.telemetryContextFloor, read);
      record.observedContextTokens = nextContextTokens(record.observedContextTokens, floor?.contextTokens, reading, opts?.refreshContext !== false);
      if (record.observedContextTokens === undefined) delete record.observedContextTokens;
      return finish();
    }
    if (!read || ("transient" in (read ?? {}))) return finish();
    if (!record.telemetry) record.telemetry = { version: 1, origin: { sessionId, sessionPath: path, emptyPrefix: true } };
  }
  const telemetry = record.telemetry;
  if (telemetry.origin.sessionPath !== path || (sessionId && telemetry.origin.sessionId !== sessionId)) { delete record.telemetry; delete record.telemetryContextFloor; delete record.observedContextTokens; return finish(); }
  if (model) telemetry.model = model; else if (state) delete telemetry.model;
  if (typeof state?.thinkingLevel === "string" && state.thinkingLevel) telemetry.effort = state.thinkingLevel; else if (state) delete telemetry.effort;
  if (read && "transient" in read) return finish();
  const reduced = reduceTelemetry(telemetry.origin, read);
  if (!reduced) { delete record.telemetry; delete record.telemetryContextFloor; delete record.observedContextTokens; return finish(); }
  const previousContext = telemetry.contextTokens;
  delete telemetry.estimatedUsd; delete telemetry.partialEstimatedUsd;
  Object.assign(telemetry, reduced);
  telemetry.contextTokens = nextContextTokens(previousContext, reduced.contextTokens, reading, opts?.refreshContext !== false);
  if (telemetry.contextTokens === undefined) delete telemetry.contextTokens;
  return finish();
}

/**
 * One observed `ws-report-to-lead` call on a record: its `kind` (260904's
 * `"question"`/`"final"` fork disambiguation; absent for a plain
 * `full-worker`/`execute-worker` progress update) and the epoch-ms time it was
 * seen. The message text is deliberately NOT retained — under the push model
 * it has already been delivered to the lead by the time this entry is
 * appended.
 */
export interface AgentReportLogEntry {
  kind?: "question";
  at: number;
}

export type RpcAgentRegistry = Map<string, RpcAgentRecord>;

/**
 * 260905 (alias/park/cap ticket): the ONE alias-or-uuid resolution helper
 * every `agent_id` param goes through (`sendToAgent`, `stopAgent`,
 * `getAgentTranscriptPath`, and `execute-gateway.ts`'s `ws-approve`), so a
 * caller may pass either the raw uuid or a record's `alias` interchangeably.
 * Direct `registry.has(idOrAlias)` wins first (the uuid path, cheap and
 * unambiguous); otherwise scans for a record whose `alias` matches exactly.
 * Returns `undefined` on a genuine miss — each call site's existing "unknown
 * agentId" error path is left unchanged by falling back to the original
 * input (`resolveAgentId(registry, id) ?? id`).
 */
export function resolveAgentId(registry: RpcAgentRegistry, idOrAlias: string): string | undefined {
  if (registry.has(idOrAlias)) return idOrAlias;
  for (const record of registry.values()) {
    if (record.alias === idOrAlias) return record.agentId;
  }
  return undefined;
}

export type AgentStatus = "running" | "idle" | "dormant" | "waiting-on-children" | "pending-delivery";

/**
 * 260905: the three RPC-backed spawn shapes, recorded on the record at spawn
 * time (`RpcAgentRecord.spawnRole`). Narrower than `process-role.ts`'s
 * `SpawnRole` on purpose — that one describes the CHILD process's own view of
 * itself (`worker`/`fork`/`explore`) as carried in its env, while this one is
 * the PARENT's classification of what it spawned, and must distinguish an
 * `execute-worker` (approval-gated) from a plain worker so the shutdown
 * sidecar can re-arm the right wiring on revival.
 *
 * `"explore"` is the fourth shape: every eligible dispatcher creates the same
 * persistent RPC researcher record, sidecar-restored without worker/fork-specific
 * wiring.
 */
export type SpawnAgentRole = "worker" | "execute-worker" | "fork" | "explore";

/** Per-agent cap on retained `reportLog` entries (drop-oldest); see `RpcAgentRecord.reportLog`. */
export const REPORT_LOG_CAP = 64;

/**
 * 260905 (alias/park/cap ticket): env var the LEAD process reads at spawn
 * time to override the registry cap (`DEFAULT_AGENT_REGISTRY_CAP`). Never
 * forwarded to a child's env — this is lead-process-only config, unlike the
 * child-carried role markers in `process-role.ts`.
 */
export const WS_PI_AGENT_REGISTRY_CAP_ENV = "WS_PI_AGENT_REGISTRY_CAP";

/** Default registry cap when `WS_PI_AGENT_REGISTRY_CAP_ENV` is unset or unparsable. */
export const DEFAULT_AGENT_REGISTRY_CAP = 256;

/**
 * Pure env-param resolver for the registry cap, mirroring `shouldPushToLead`'s
 * testable-default-param shape. A missing, non-numeric, or non-positive value
 * falls back to `DEFAULT_AGENT_REGISTRY_CAP` rather than throwing or producing
 * a cap of zero/negative that would reject every spawn.
 */
export function resolveAgentRegistryCap(env: NodeJS.ProcessEnv = process.env): number {
  const raw = env[WS_PI_AGENT_REGISTRY_CAP_ENV];
  if (typeof raw !== "string" || raw.trim() === "") return DEFAULT_AGENT_REGISTRY_CAP;
  const parsed = Number(raw);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_AGENT_REGISTRY_CAP;
}

/**
 * Byte cap on the stored copy of a spawn's `prompt` (`RpcAgentRecord.prompt`),
 * applied once at spawn time — bounds the record itself, the shutdown sidecar
 * that serializes it, and the `ws-agent-list` `include_prompt` reply alike.
 */
export const PROMPT_STORAGE_CAP_BYTES = 4096;

/** Marker line appended by `truncatePromptForStorage` when a prompt was cut. */
const PROMPT_TRUNCATION_MARKER = "\n…[truncated for storage]";

/**
 * Byte-safe head-truncation of a prompt for storage: cuts at `capBytes` UTF-8
 * bytes (never mid-codepoint — a naive `Buffer.slice` cut can straddle a
 * multibyte sequence and corrupt the tail) and appends a cut-marker line when
 * truncation actually happened.
 */
export function truncatePromptForStorage(prompt: string, capBytes: number = PROMPT_STORAGE_CAP_BYTES): string {
  const buf = Buffer.from(prompt, "utf8");
  if (buf.length <= capBytes) return prompt;
  const decoder = new StringDecoder("utf8");
  const head = decoder.write(buf.subarray(0, capBytes));
  return `${head}${PROMPT_TRUNCATION_MARKER}`;
}
