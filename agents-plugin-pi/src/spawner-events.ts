import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { ParentChannel, type ChannelConnection, type ChannelHello } from "./agent-channel.ts";
import { consumedApprovalsFromResume, parseApprovalConsumedMessage, pendingApprovalFromResume } from "./approval-protocol.ts";
import { observeSubtreeChannel, publishSubtree, scheduleSubtreeTelemetry } from "./subtree-lifecycle.ts";
import { createOutputRateTracker } from "./output-rate.ts";
import { modelObservation, retainModelObservation } from "./model-observation.ts";
import { closeOpenBatch, createBatchBuffer, observeBatchEvent } from "./agent-batches.ts";
import { scheduleAgentViewPublish } from "./agent-view.ts";
import { observeForkFinishEvent, advanceWorkGeneration, syncOwnershipProtection, recordReport, triggerAgentWidgetRefresh, triggerAgentCostRefresh, advanceForkFinish, finishResumeCtx, createTerminalDelivery, pushToLead, probeAgentLiveness, triggerAgentDashboardRefresh, clearTerminalFacts } from "./spawner.ts";
import type { PushFamily, PushDeliverAs } from "./spawner.ts";
import { getAgentRpcSnapshot, refreshAgentTelemetry } from "./spawner-record.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner-record.ts";
import { REPORT_TO_LEAD_TOOL_NAME, GATED_EXEC_TOOL_NAME } from "./spawner-tools.ts";

/**
 * What `attachEventListener` must DO about an event `applyRpcEvent` just
 * applied. 260905: `applyRpcEvent` stays pure (no `pi`, no `RpcClient` — the
 * convention its existing plain-fake-record tests depend on), so it describes
 * the push instead of performing it, and the IO glue one layer up
 * (`attachEventListener`, which already read this return value to fire
 * `onApprovalPending`) turns that description into a `pi.sendMessage`.
 */
export interface RpcEventOutcome {
  /** A push to emit verbatim, already resolved against the record's suppression hooks. */
  push?: { family: PushFamily; payload: Record<string, unknown>; deliverAs: PushDeliverAs };
  /** `true` on `agent_settled`: the caller decides whether a `ws-agent-settled` push follows (it needs an async `harvestLastMessage`). */
  settled?: boolean;
}

/**
 * Applies lifecycle and intermediate-report RPC events to a record. Every
 * report-tool call is an immediate informational push, whatever `kind` it
 * carries. No report-tool
 * value is terminal. `agent_settled` clears execution synchronously, while
 * `attachEventListener` performs asynchronous terminal transcript harvest and
 * queue admission. Gated execution events additionally capture approval data.
 */

/** Store output observation without refreshing: streaming calls this per token. */
function markAgentOutput(record: RpcAgentRecord): void {
  record.lastOutputAt = Date.now();
}

/**
 * Only assistant output is output: text, thinking and tool-call argument
 * deltas, and a text-bearing message_end; lifecycle and input events are not.
 * 261028 batch-row redesign: `toolcall_delta` counts too, so a long tool-call
 * argument stream (a large `write`) takes the O(1) streaming path instead of
 * a per-token subtree publish and widget refresh, and resets the activity
 * clock like any other sign the model is producing output.
 */
function isStreamedAssistantOutput(evt: { type?: string; assistantMessageEvent?: unknown; message?: unknown }): boolean {
  if (evt.type === "message_end") return assistantMessageText(evt.message) !== undefined;
  if (evt.type !== "message_update" || !evt.assistantMessageEvent || typeof evt.assistantMessageEvent !== "object") return false;
  const event = evt.assistantMessageEvent as { type?: unknown; delta?: unknown };
  return (event.type === "text_delta" || event.type === "thinking_delta" || event.type === "toolcall_delta") && typeof event.delta === "string";
}

export function applyRpcEvent(
  record: RpcAgentRecord,
  evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown },
): RpcEventOutcome {
  // Every tool call/result is agent output, including report and approval
  // tools. Lead prompts, owner sends, and scheduler triggers never arrive here.
  if (evt.type === "tool_execution_start" || evt.type === "tool_execution_end") markAgentOutput(record);
  if (evt.type === "tool_execution_end") {
    observeForkFinishEvent(record, evt);
    return {};
  }
  if (evt.type === "agent_start") {
    const alreadyRunning = record.running;
    record.streaming = true;
    if (record.delegation && !alreadyRunning) {
      // A direct child can wake itself for its own children's reports, without
      // a new outer promptAgent call. This is a new own-turn generation.
      record.running = true;
      advanceWorkGeneration(record);
      record.runStartedAt = Date.now();
      record.settledAt = undefined;
    }
    observeForkFinishEvent(record, evt);
  } else if (evt.type === "agent_settled") {
    // Duplicate terminal events belong to the same completed run, so the
    // first observation is the fixed duration boundary.
    record.settledAt ??= Date.now();
    record.streaming = false;
    // The run is over: the child stops counting toward the fan-in the instant
    // it settles, whatever the caller decides to push about it.
    record.running = false;
    record.pendingApproval = undefined;
    syncOwnershipProtection(record);
    observeForkFinishEvent(record, evt);
    return { settled: true };
  } else if (evt.type === "tool_execution_start" && evt.toolName === REPORT_TO_LEAD_TOOL_NAME) {
    // Every report is a plain progress/finding report. `kind` left the
    // schema, but a caller can still send it (the schema is open, and a fork
    // revived from a pre-change session file still holds the old "ask with
    // kind:question" directive), so any residual `kind` is ignored here and
    // never reaches the dormant `questionReportOutcome`.
    const args = evt.args as { message?: unknown } | undefined;
    const message = args?.message;
    if (typeof message === "string") {
      recordReport(record, undefined);
      return { push: { family: "ws-agent-report", payload: { report: message }, deliverAs: "followUp" } };
    }
  } else if (evt.type === "tool_execution_start" && evt.toolName === GATED_EXEC_TOOL_NAME) {
    const args = evt.args as { command?: unknown; rationale?: unknown; cwd?: unknown } | undefined;
    const command = args?.command;
    if (typeof command === "string" && typeof evt.toolCallId === "string") {
      record.pendingApproval = {
        cmdId: evt.toolCallId,
        command,
        rationale: typeof args?.rationale === "string" ? args.rationale : undefined,
        cwd: typeof args?.cwd === "string" ? args.cwd : undefined,
      };
      syncOwnershipProtection(record);
      // The approval PUSH itself is `createApprovalRelay`'s job
      // (execute-gateway.ts owns the §7 payload and the working-context
      // scrape); this branch only records what is pending. Fired from
      // `attachEventListener` below via `onApprovalPending`, unconditionally
      // now that there is no wait return for it to duplicate.
      return {};
    }
  }
  return {};
}

/**
 * Dormant: the former `kind:"question"` branch of `applyRpcEvent`, kept with
 * the rest of the disconnected owner-question code but no longer called. A
 * defined (string) `onQuestionReport` return is the registration notice for a
 * fork thread the hook just bound, pushed as an advisory instead of the raw
 * question; `undefined` is the headless case, and a throwing hook degrades to
 * that same baseline rather than dropping the report.
 */
export function questionReportOutcome(record: RpcAgentRecord, message: string): RpcEventOutcome {
  recordReport(record, "question");
  let notice: string | undefined;
  if (record.onQuestionReport) {
    try {
      notice = record.onQuestionReport(record, message);
    } catch {
      notice = undefined;
    }
  }
  return notice !== undefined
    ? { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }
    : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
}

/**
 * Mirrors one launch's channel-delivered subtree view onto the record and
 * republishes it upstream. Called as soon as the launch's channel is on the
 * record, before the child can say hello, so its first snapshot is never
 * missed; the view reads waiting until that snapshot arrives. Only the
 * record's current channel may write: a closed launch's late callbacks are
 * dropped.
 */
export function observeChildSubtree(registry: RpcAgentRegistry | undefined, record: RpcAgentRecord, channel: ParentChannel): void {
  record.subtreeRevision = undefined;
  record.subtreeTurn = undefined;
  observeSubtreeChannel(channel, view => {
    if (record.channel !== channel) return;
    record.waitingOnChildren = view.waiting;
    record.subtreeRevision = view.snapshot?.revision;
    if (view.snapshot) record.subtreeTurn = { owed: view.snapshot.turnOwed, started: view.snapshot.turnsStarted };
    record.subtreeDescendants = (view.snapshot?.descendants ?? []).map(row => view.disconnected ? { ...row, live: false } : row);
    // Every not-waiting view re-evaluates a held settle against the child's
    // own wake accounting; `releaseSettlementHold` decides and is a no-op
    // when nothing is held, so a duplicate snapshot cannot admit twice. It
    // runs before the publish so a release's drop of the held settle from
    // `subtreeOutstanding` rides this send, as on the direct admission path.
    if (!view.waiting && view.snapshot) record.releaseSettlementHold?.(view.snapshot);
    publishSubtree(registry);
    triggerAgentWidgetRefresh();
  });
}

/** What the parent-side approval reconciliation needs from the launch's channel; `ParentChannel` satisfies it. */
export interface ApprovalChannelHost {
  onMessage(cb: (msg: Record<string, unknown>) => void): () => void;
  onDisconnect(cb: () => void): () => void;
  onConnection(cb: (conn: ChannelConnection, hello: ChannelHello) => void): () => void;
}

/**
 * Parent side of the channel-delivered approval decision
 * (260924-feat-pi-agent-channel-approval-decisions; the message shapes and
 * the child's wait are `approval-protocol.ts`). The child is the authority
 * on consumption, so this never infers that a command started:
 *
 * - `approval-consumed` for the pending `cmd_id` releases the request and,
 *   with it, the ownership protection `syncOwnershipProtection` derives from
 *   `pendingApproval` — the one release path besides the child's exit.
 * - A disconnect discards a sent-but-unacknowledged decision (`"discarded"`;
 *   `ws-approve` records the same state when it finds no live connection).
 *   It is never re-sent: whether the child consumed it before the drop is
 *   learned only from the reconnect hello.
 * - A reconnect hello settles a discarded decision only on positive
 *   evidence. When it lists the `cmd_id` in `consumed`, the child ran the
 *   command (its acknowledgment was lost with the connection), so the request
 *   is released. Otherwise the decision was lost, lost in transit, or kept
 *   early on the old connection (which the child discards with it): the
 *   request goes back to the lead as a fresh push (`reissued`, `issue` + 1),
 *   and only a decision sent over the new connection can be consumed. A hello
 *   with no `consumed` array comes from a child that predates it; there,
 *   "not reported as pending" still counts as consumed. A request with no
 *   decision sent is left alone either way: the RPC event stream owns the
 *   request, and the lead already holds it.
 *
 * Returns the detach; `clearLiveState` closes the channel itself.
 */
export function attachApprovalChannel(
  record: RpcAgentRecord,
  channel: ApprovalChannelHost,
  approvalHook: () => ((record: RpcAgentRecord) => void) | undefined,
  registry?: RpcAgentRegistry,
): () => void {
  const release = () => {
    record.pendingApproval = undefined;
    syncOwnershipProtection(record);
    publishSubtree(registry);
    triggerAgentWidgetRefresh();
  };
  const offMessage = channel.onMessage((msg) => {
    const cmdId = parseApprovalConsumedMessage(msg);
    if (cmdId !== undefined && record.pendingApproval?.cmdId === cmdId) release();
  });
  const offDisconnect = channel.onDisconnect(() => {
    const pending = record.pendingApproval;
    if (pending?.decision === "sent") record.pendingApproval = { ...pending, decision: "discarded" };
  });
  const offConnection = channel.onConnection((_conn, hello) => {
    const pending = record.pendingApproval;
    if (!hello.reconnect || pending?.decision !== "discarded") return;
    const consumed = consumedApprovalsFromResume(hello.resume);
    const wasConsumed = consumed ? consumed.includes(pending.cmdId) : pendingApprovalFromResume(hello.resume) !== pending.cmdId;
    if (wasConsumed) { release(); return; }
    const { decision: _discarded, ...request } = pending;
    record.pendingApproval = { ...request, reissued: true, issue: (pending.issue ?? 0) + 1 };
    syncOwnershipProtection(record);
    approvalHook()?.(record);
  });
  return () => { offMessage(); offDisconnect(); offConnection(); };
}

/**
 * Wires the child event stream to immediate reports and per-generation
 * terminal settlement. Settlement clears execution before any async work,
 * holds while the channel-delivered subtree view reads waiting or the child
 * reports a turn still coming (released by `releaseSettlementHold` once a
 * not-waiting snapshot says no turn is coming), harvests the ordinary
 * assistant answer, admits one retryable terminal delivery, and parks
 * only after enqueue.
 * Duplicate events join the generation latch; replacement work invalidates a
 * late harvest. Owner-held output uses the owner notification route.
 */
export function attachEventListener(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  client: RpcClient,
  onApprovalPending?: (record: RpcAgentRecord) => void,
): void {
  let refreshing = false;
  let dirty = false;
  let statsDirty = false;
  // Older/revived records may predate generation tracking. Normalize once so
  // the first settlement is not mistaken for a duplicate undefined latch.
  record.launchGeneration ??= 0;
  record.workGeneration ??= 0;
  const generation = record.launchGeneration;
  // Spans exist only as receipt times of this launch's live deltas.
  const outputRate = createOutputRateTracker();
  record.outputRate = outputRate;
  // Batches outlive a launch (the widget keeps showing the latest ones), but a
  // batch left open by the previous launch can never finish.
  const toolBatches = record.toolBatches ??= createBatchBuffer();
  closeOpenBatch(toolBatches, Date.now());
  const refresh = (includeStats = false) => {
    dirty = true;
    statsDirty ||= includeStats;
    if (refreshing) return;
    refreshing = true;
    void (async () => {
      do {
        dirty = false;
        const readStats = statsDirty;
        statsDirty = false;
        try {
          const snapshot = await getAgentRpcSnapshot(client, readStats);
          if (record.client === client && record.launchGeneration === generation && refreshAgentTelemetry(record, snapshot.state, { stats: snapshot.stats, refreshContext: readStats })) {
            publishSubtree(registry);
            triggerAgentWidgetRefresh();
            triggerAgentCostRefresh();
          }
        } catch {
          if (record.client === client && record.launchGeneration === generation) {
            const changed = record.observedModel !== undefined || record.observedEffort !== undefined || record.telemetry?.model !== undefined || record.telemetry?.effort !== undefined;
            delete record.observedModel; delete record.observedEffort;
            if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; }
            if (changed) { publishSubtree(registry); triggerAgentWidgetRefresh(); }
          }
        }
      } while (dirty && record.client === client && record.launchGeneration === generation);
      refreshing = false;
    })();
  };
  // Terminal admission for the current work generation. A held settle records
  // its generation on the record (`heldSettlementGeneration`) for
  // `releaseSettlementHold`.
  record.heldSettlementGeneration = undefined;
  // Child `agent_start` events this launch's RPC stream has delivered; the
  // closure is per client, so a relaunch starts again at zero, as the new
  // child process's own count does. Exact only because both launch sites
  // attach this listener before the child's first prompt.
  let observedTurnStarts = 0;
  // The child reports whether a turn is still coming; the parent does not
  // infer it. A snapshot that owes a turn, or counts a turn start stdout has
  // not delivered yet (the socket ran ahead), leaves admission to that turn:
  // its `agent_start` opens a new work generation and its own settle admits.
  // Admitting now would report the previous generation's answer ahead of it.
  const turnComing = (owed: boolean, started: number) => owed || started > observedTurnStarts;
  const admitSettlement = () => {
    record.heldSettlementGeneration = undefined;
    const workGeneration = record.workGeneration;
    const stillSettled = () => record.client === client && record.launchGeneration === generation && record.workGeneration === workGeneration && !record.running && !record.streaming && !record.waitingOnChildren;
    const finish = record.forkFinish;
    if (finish) {
      void advanceForkFinish(record, registry ?? new Map([[record.agentId, record]]), pi as ExtensionAPI, finishResumeCtx(finish), finish);
    } else {
      if (record.settlementAdmissionGeneration === workGeneration) {
        record.terminalDelivery?.retry?.();
      } else {
        // Latch before the asynchronous transcript harvest so duplicate
        // settle events for one generation cannot enqueue twice.
        record.settlementAdmissionGeneration = workGeneration;
        const terminal = createTerminalDelivery(record, registry, pi, workGeneration);
        void (async () => {
          const lastMessage = await harvestLastMessage(record);
          if (!stillSettled() || record.terminalDelivery !== terminal) return;
          const payload = { reason: "idle", last_message: lastMessage };
          terminal.retry = () => {
            if (terminal.state !== undefined || !stillSettled() || record.terminalDelivery !== terminal) return;
            pushToLead(pi, registry, record, "ws-agent-settled", payload, "followUp", terminal);
          };
          terminal.retry();
          await probeAgentLiveness(pi, registry, record);
          triggerAgentWidgetRefresh();
        })();
      }
    }
  };
  record.releaseSettlementHold = (snapshot) => {
    if (record.client !== client || record.launchGeneration !== generation) return;
    const held = record.heldSettlementGeneration;
    if (held === undefined || held !== record.workGeneration) return;
    if (record.running || record.streaming || record.waitingOnChildren) return;
    if (turnComing(snapshot.turnOwed, snapshot.turnsStarted)) return;
    admitSettlement();
  };
  const unsubscribeEvents = client.onEvent((evt) => {
    const e = evt as { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown; message?: unknown; assistantMessageEvent?: unknown };
    if (record.client !== client || record.launchGeneration !== generation) return;
    if (e.type === "agent_start") observedTurnStarts += 1;
    if (e.type === "message_start") observeQueuedWorkBoundary(record, e.message);
    const agentOutput = isStreamedAssistantOutput(e);
    const streamingDelta = e.type === "message_update" && agentOutput;
    // Token deltas update only the O(1) high-water field. Their timing and
    // subtree/telemetry refreshes wait for message_end or the periodic gutter
    // cadence, preventing a per-token render fan-out.
    if (agentOutput) {
      markAgentOutput(record);
      // Dirty notification only: one trailing sampler reads the newest output
      // clock without rebuilding or forwarding a tree for every token.
      if (streamingDelta) scheduleSubtreeTelemetry(registry);
    }
    // Block-boundary receipt times only (O(1)); the rate changes at message_end.
    if (e.type === "message_update" || e.type === "message_start" || e.type === "message_end") {
      const before = e.type === "message_end" ? outputRate.rate() : undefined;
      outputRate.observe(e);
      if (e.type === "message_end" && outputRate.rate() !== before) triggerAgentWidgetRefresh();
    }
    if (e.type === "message_end") {
      const observed = modelObservation(e.message, outputRate.sampleRate?.());
      if (observed) record.modelObservations = retainModelObservation(record.modelObservations, observed);
      const text = assistantMessageText(e.message);
      if (text !== undefined) {
        record.lastText = text;
        record.lastTextGeneration = record.workGeneration;
      }
    }
    // Tool batches and the generating row: O(1) per event. A streaming delta
    // changes at most the streaming call's character count, which the widget
    // reads at its 1 s tick and the agent-view relay at its throttle.
    const batchChanged = observeBatchEvent(toolBatches, e, Date.now());
    const outcome = applyRpcEvent(record, e);
    // A settle publishes only after its terminal admission below: published
    // here, the run's end would read quiescent before the terminal delivery
    // that keeps it outstanding exists, a snapshot owing no turn for a
    // delivery about to wake this process.
    if (!streamingDelta && !outcome.settled) publishSubtree(registry);
    if (!streamingDelta && (e.type === "agent_start" || e.type === "agent_settled" || e.type === "message_end" || e.type === "message_update" || e.type === "thinking_level_changed" || e.type === "compaction_end")) {
      // Context occupancy changes at completed message/compaction boundaries.
      refresh(e.type === "agent_settled" || e.type === "message_end" || e.type === "compaction_end");
    }
    if (outcome.push) {
      pushToLead(pi, registry, record, outcome.push.family, outcome.push.payload, outcome.push.deliverAs);
    }
    // Dirty mark only: the publish is throttled and returns at once while its
    // timer is pending, so this stays O(1) on the streaming-delta path.
    if (batchChanged) {
      scheduleAgentViewPublish();
      if (streamingDelta) triggerAgentDashboardRefresh();
    }
    if (e.type === "agent_start" || (batchChanged && !streamingDelta)) {
      // 260905 (live-agent widget ticket): streaming just flipped true — a
      // fresh "running" row transition. 261028: a tool batch started, ended,
      // or changed its in-flight calls, or the generating row changed phase.
      triggerAgentWidgetRefresh();
    }
    // Held while the view reads waiting, and also while the view is not
    // waiting but the launch's latest snapshot says a turn is still coming
    // (the same check `releaseSettlementHold` applies). A launch with no
    // snapshot yet admits directly: a channel-less child never fails closed,
    // and a channel's view reads waiting until its first snapshot anyway.
    const turn = record.subtreeTurn;
    if (outcome.settled && (record.waitingOnChildren || (turn !== undefined && turnComing(turn.owed, turn.started)))) {
      record.heldSettlementGeneration = record.workGeneration;
      clearTerminalFacts(record);
      syncOwnershipProtection(record);
      publishSubtree(registry);
      triggerAgentWidgetRefresh();
      return;
    }
    if (outcome.settled) {
      admitSettlement();
      publishSubtree(registry);
    }
    if (record.forkFinish && e.type === "tool_execution_end") {
      const finish = record.forkFinish;
      void advanceForkFinish(record, registry ?? new Map([[record.agentId, record]]), pi as ExtensionAPI, finishResumeCtx(finish), finish);
    }
    if (e.type === "tool_execution_start" && (e.toolName === GATED_EXEC_TOOL_NAME || e.toolName === REPORT_TO_LEAD_TOOL_NAME)) {
      // 260905 (live-agent widget ticket): a gated command just went pending
      // approval, or a report (progress or finding) was just observed —
      // both are widget-relevant transitions per the ticket's own list.
      triggerAgentWidgetRefresh();
    }
    // Ctx callback first, per-record fallback second (see
    // `RpcAgentRecord.onApprovalPending`).
    const approvalHook = onApprovalPending ?? record.onApprovalPending;
    if (approvalHook && e.type === "tool_execution_start" && e.toolName === GATED_EXEC_TOOL_NAME) {
      approvalHook(record);
    }
  });
  // The decision hand-off rides the launch's channel; the hook resolves per
  // call because `record.onApprovalPending` may be armed after this attach.
  const unsubscribeApproval = record.channel ? attachApprovalChannel(record, record.channel, () => onApprovalPending ?? record.onApprovalPending, registry) : undefined;
  record.unsubscribe = () => {
    unsubscribeEvents();
    unsubscribeApproval?.();
  };
}

/**
 * Read the agent's ordinary assistant answer at settlement without borrowing
 * from an earlier generation. `getLastAssistantText()` remains the native RPC
 * read, while the current generation's assistant `message_end` is the proof
 * that the returned transcript tail belongs to this turn. When the RPC skips
 * an empty/aborted message or disagrees with that event, preserve the event's
 * exact text; when no current assistant event exists, report missing output.
 */
function messageText(message: unknown, role: "assistant" | "user"): string | undefined {
  if (!message || typeof message !== "object" || (message as { role?: unknown }).role !== role) return undefined;
  const content = (message as { content?: unknown }).content;
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content.flatMap((part) => part && typeof part === "object" && (part as { type?: unknown }).type === "text" && typeof (part as { text?: unknown }).text === "string"
    ? [(part as { text: string }).text]
    : []).join("");
}

function observeQueuedWorkBoundary(record: RpcAgentRecord, message: unknown): void {
  if (messageText(message, "user") === undefined || !record.pendingQueuedWork?.length) return;
  // Pi can expand a queued skill/template before emitting its user message, so
  // text equality is not a stable identity. User queue consumption is the
  // actual FIFO boundary; steering may overtake follow-ups, but either accepted
  // item starts one successor generation and removes exactly one obligation.
  record.pendingQueuedWork.shift();
  if (record.pendingQueuedWork.length === 0) record.pendingQueuedWork = undefined;
  advanceWorkGeneration(record);
}

function assistantMessageText(message: unknown): string | undefined {
  return messageText(message, "assistant");
}

export async function harvestLastMessage(record: RpcAgentRecord): Promise<string | undefined> {
  const workGeneration = record.workGeneration;
  const observed = record.lastTextGeneration === workGeneration ? record.lastText : undefined;
  if (record.lastTextGeneration !== workGeneration) return undefined;
  if (record.client) {
    try {
      const rpcText = await record.client.getLastAssistantText();
      if (rpcText === observed) return rpcText ?? undefined;
    } catch { /* the generation-scoped event text remains authoritative */ }
  }
  return observed;
}
