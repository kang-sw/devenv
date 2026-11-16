import { randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { countBackgroundTasks } from "./background-tasks.ts";
import { isLeadOrFork, readSpawnRole } from "./process-role.ts";
import { ownerNotifyRef } from "./owner-notify.ts";
import { evaluateDescendantUsage } from "./agent-usage-rollup.ts";
import { publishSubtree, type OwnTurnState } from "./subtree-lifecycle.ts";
import { GOAL_CONTROL_CUSTOM_TYPE, PUSH_BATCH_CUSTOM_TYPE, PUSH_BATCH_VERSION, type PushBatchItem, type PushBatchItemState } from "./push-protocol.ts";
import { labelAdapterContent, labelAdapterText } from "./adapter-label.ts";
import { withItemSummaryIds, withSummaryId } from "./summary-id.ts";
import { scheduleAgentViewPublish } from "./agent-view.ts";
import type { RpcAgentRecord, RpcAgentRegistry, AgentReportLogEntry } from "./spawner-record.ts";
import { syncOwnershipProtection, claimLeadOwnership, OWNER_TERMINAL_RETRY_DELAY_MS } from "./spawner-launch.ts";
import type { RpcResumeCtx } from "./spawner-launch.ts";
import { stopAgent, sendToAgent } from "./spawner.ts";
import { harvestLastMessage } from "./spawner-events.ts";

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
export function onDescendantUsageChanged(): void {
  triggerAgentCostRefresh();
  evaluateDescendantUsage();
}

/** 261028 Phase 2: a direct child's relayed descendant batch model changed or dropped: repaint and forward upward. */
export function onAgentViewChanged(): void {
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
