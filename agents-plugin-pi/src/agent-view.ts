/**
 * 261028 Phase 2 (`261028-feat-pi-agent-widget-expanded-mode`): descendant
 * tool batches over the parent-child control channel (`agent-channel.ts`).
 *
 * A spawner's `client.onEvent` sees only its direct children, so the root
 * widget learns a grandchild's batches only through this relay. Every process
 * computes a width-independent batch model (`AgentViewModel`, agent id ->
 * `BatchView[]`) from its own direct children's buffers and the models those
 * children forwarded, and sends it to its parent as one `agent-view` message.
 * The root widget alone renders, since only it knows the width, the
 * per-agent allotment and the theme.
 *
 * Deliberately not part of the subtree snapshot (`subtree-lifecycle.ts`):
 * that snapshot is acknowledged lifecycle state behind the busy fence and the
 * hello resume, while batches change on every tool call. This message is
 * advisory display only:
 *
 * - always sent, unacknowledged, latest value wins;
 * - throttled at `SUBTREE_TELEMETRY_INTERVAL_MS`, skipped when unchanged;
 * - dropped while disconnected (no queue, no resume key); the child forgets
 *   its last-sent value and republishes on reconnect, so idle descendants'
 *   rows return without waiting for a new tool call;
 * - a parent drops a child's forwarded model when that child disconnects or
 *   exits;
 * - never an input to wait or settle.
 *
 * Past `MAX_SUBTREE_DESCENDANTS`, a hop keeps the same agents the subtree
 * snapshot keeps (`selectSubtreeDescendants`, parent-before-child). The
 * receiver computes elapsed time from each running call's start time, so a
 * running batch needs no per-second publish.
 *
 * 261028 batch-row redesign: each batch carries a `failed` count, and an
 * assistant message still generating travels as a view with `generating`
 * (phase kind, phase start, and while a tool call streams its tool name and
 * character count). The count changes with every streamed delta, so a
 * streaming child republishes at the throttle rate; no compatibility
 * fallback exists, since every RPC child loads the parent's exact extension
 * entry.
 */

import type { ChildChannel, ParentChannel } from "./agent-channel.ts";
import { AGENT_BATCH_CAP, BATCH_ARGS_PREVIEW_CAP, BATCH_TOOL_NAME_CAP, batchViews, type BatchGeneratingView, type BatchRunningView, type BatchView } from "./agent-batches.ts";
import { MAX_SUBTREE_DESCENDANTS, SUBTREE_TELEMETRY_INTERVAL_MS, selectSubtreeDescendants } from "./subtree-lifecycle.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";

/** Channel message type `t` of a batch model report: `{ t, agents: [{ id, batches }] }`. */
export const AGENT_VIEW_MESSAGE = "agent-view";

/** Agent id -> that agent's batch views, oldest first. Insertion order is parent-before-child. */
export type AgentViewModel = Map<string, BatchView[]>;

/** Characters a relayed tool name keeps. */
const TOOL_NAME_CAP = 64;

/** A relayed epoch becomes part of a summary key, so only plain tokens are kept. */
const EPOCH_PATTERN = /^[A-Za-z0-9_-]{1,32}$/;

function isCount(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) >= 0;
}

function isClock(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}

function capString(value: string, cap: number): string {
  const chars = [...value.replace(/[\u0000-\u001f\u007f-\u009f]/g, " ")];
  return chars.length <= cap ? chars.join("") : chars.slice(0, cap - 1).join("") + "…";
}

function parseRunning(raw: unknown): BatchRunningView | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const value = raw as Partial<Record<keyof BatchRunningView, unknown>>;
  if (typeof value.toolName !== "string" || typeof value.argsPreview !== "string" || !isClock(value.startedAt) || !isCount(value.done) || !isCount(value.othersRunning)) return undefined;
  return {
    toolName: capString(value.toolName, TOOL_NAME_CAP),
    argsPreview: capString(value.argsPreview, BATCH_ARGS_PREVIEW_CAP),
    startedAt: value.startedAt,
    done: value.done,
    othersRunning: value.othersRunning,
  };
}

function parseGenerating(raw: unknown): BatchGeneratingView | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const value = raw as Partial<Record<keyof BatchGeneratingView, unknown>>;
  if (!isClock(value.phaseStartedAt)) return undefined;
  if (value.kind === "thinking" || value.kind === "replying") return { kind: value.kind, phaseStartedAt: value.phaseStartedAt };
  if (value.kind !== "tool" || typeof value.toolName !== "string" || !isCount(value.chars)) return undefined;
  return { kind: "tool", phaseStartedAt: value.phaseStartedAt, toolName: capString(value.toolName, TOOL_NAME_CAP), chars: value.chars };
}

/**
 * One relayed batch, or `undefined` when malformed. Exactly one of `endedAt`
 * (finished), `running` and `generating` must be present, and `failed` is a
 * count no larger than `callCount`.
 */
function parseBatch(raw: unknown): BatchView | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const value = raw as Partial<Record<keyof BatchView, unknown>>;
  if (!Number.isSafeInteger(value.seq) || (value.seq as number) <= 0 || !isCount(value.callCount) || !isCount(value.failed) || value.failed > value.callCount || !isClock(value.startedAt) || !Array.isArray(value.toolNames)) return undefined;
  const toolNames = (value.toolNames as unknown[])
    .filter((name): name is string => typeof name === "string" && name.length > 0)
    .slice(0, BATCH_TOOL_NAME_CAP)
    .map((name) => capString(name, TOOL_NAME_CAP));
  const view: BatchView = { ...(typeof value.epoch === "string" && EPOCH_PATTERN.test(value.epoch) ? { epoch: value.epoch } : {}), seq: value.seq as number, toolNames, callCount: value.callCount, failed: value.failed, startedAt: value.startedAt };
  const present = [value.endedAt, value.running, value.generating].filter((field) => field !== undefined).length;
  if (present !== 1) return undefined;
  if (value.endedAt !== undefined) {
    if (!isClock(value.endedAt)) return undefined;
    view.endedAt = value.endedAt;
  } else if (value.running !== undefined) {
    const running = parseRunning(value.running);
    if (!running) return undefined;
    view.running = running;
  } else {
    const generating = parseGenerating(value.generating);
    if (!generating) return undefined;
    view.generating = generating;
  }
  return view;
}

/**
 * Parses one `agent-view` message body. Malformed entries and batches are
 * dropped individually; the result is bounded to `MAX_SUBTREE_DESCENDANTS`
 * agents (first occurrence of an id wins) and each agent's newest
 * `AGENT_BATCH_CAP` batches. `undefined` only when `agents` is not an array.
 */
export function parseAgentViewMessage(raw: unknown): AgentViewModel | undefined {
  const agents = (raw as { agents?: unknown } | null | undefined)?.agents;
  if (!Array.isArray(agents)) return undefined;
  const model: AgentViewModel = new Map();
  for (const entry of agents) {
    if (model.size >= MAX_SUBTREE_DESCENDANTS) break;
    const value = entry as { id?: unknown; batches?: unknown } | null;
    if (!value || typeof value !== "object" || typeof value.id !== "string" || value.id.length === 0 || model.has(value.id) || !Array.isArray(value.batches)) continue;
    const batches = (value.batches as unknown[]).map(parseBatch).filter((batch): batch is BatchView => batch !== undefined).slice(-AGENT_BATCH_CAP);
    if (batches.length > 0) model.set(value.id, batches);
  }
  return model;
}

/**
 * This hop's model: each direct child's own batches plus the models its
 * children forwarded, restricted to the agents the subtree snapshot keeps and
 * in its parent-before-child order. A direct child's own buffer wins over any
 * forwarded entry for the same id; agents with no batches are omitted.
 */
export function buildAgentViewModel(registry: RpcAgentRegistry): AgentViewModel {
  const sources = new Map<string, BatchView[]>();
  for (const record of registry.values()) {
    const own = batchViews(record.toolBatches);
    if (own.length > 0) sources.set(record.agentId, own);
  }
  for (const record of registry.values()) {
    for (const [id, batches] of record.descendantViews ?? []) {
      if (!sources.has(id) && !registry.has(id)) sources.set(id, batches);
    }
  }
  const model: AgentViewModel = new Map();
  for (const row of selectSubtreeDescendants(registry)) {
    const batches = sources.get(row.id);
    if (batches && !model.has(row.id)) model.set(row.id, batches);
  }
  return model;
}

/** The wire body of a model. */
export function agentViewMessage(model: AgentViewModel): { t: string; agents: Array<{ id: string; batches: BatchView[] }> } {
  return { t: AGENT_VIEW_MESSAGE, agents: [...model].map(([id, batches]) => ({ id, batches })) };
}

/** What the child half needs from its channel; `ChildChannel` satisfies it. */
export type AgentViewChildChannel = Pick<ChildChannel, "connected" | "closed" | "send" | "onReconnect">;

export interface AgentViewUpstreamOptions {
  intervalMs?: number;
  now?: () => number;
}

/**
 * Child half: one per process with a parent channel. `schedule()` is the
 * dirty notification (a batch changed below this hop); it publishes at once
 * when the last send is at least one interval old, otherwise once at the end
 * of the interval. A publish skips a model equal to the last one sent and
 * drops itself while disconnected.
 */
export class AgentViewUpstream {
  private source: (() => AgentViewModel) | undefined;
  private lastKey: string | undefined;
  private lastSentAt: number | undefined;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private readonly channel: AgentViewChildChannel;
  private readonly intervalMs: number;
  private readonly now: () => number;

  constructor(channel: AgentViewChildChannel, opts: AgentViewUpstreamOptions = {}) {
    this.channel = channel;
    this.intervalMs = opts.intervalMs ?? SUBTREE_TELEMETRY_INTERVAL_MS;
    this.now = opts.now ?? Date.now;
    channel.onReconnect(() => {
      // The parent dropped this hop's model at the disconnect.
      this.lastKey = undefined;
      this.publish();
    });
  }

  /** Replaces the model source (a session replacement brings a new registry); `undefined` pauses publishing. */
  setSource(source: (() => AgentViewModel) | undefined): void {
    this.source = source;
    if (!source) this.cancel();
  }

  schedule(): void {
    if (this.timer || !this.source || this.channel.closed) return;
    const delay = this.lastSentAt === undefined ? 0 : this.lastSentAt + this.intervalMs - this.now();
    if (delay <= 0) {
      this.publish();
      return;
    }
    this.timer = setTimeout(() => {
      this.timer = undefined;
      this.publish();
    }, delay);
    this.timer.unref?.();
  }

  /** Sends the current model now unless unchanged, disconnected, or without a source. */
  publish(): void {
    if (!this.source || this.channel.closed || !this.channel.connected) return;
    let message: ReturnType<typeof agentViewMessage>;
    try {
      message = agentViewMessage(this.source());
    } catch {
      return; // advisory display never fails a lifecycle event
    }
    const key = JSON.stringify(message.agents);
    if (key === this.lastKey) {
      // The build was the cost: an unchanged model still holds off the next
      // one for an interval, or every streaming delta rebuilds the subtree.
      this.lastSentAt = this.now();
      return;
    }
    try {
      this.channel.send(message);
    } catch {
      return; // a socket destroyed but not yet reported ended; the reconnect republishes
    }
    this.lastKey = key;
    this.lastSentAt = this.now();
  }

  cancel(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = undefined;
  }
}

/** What the parent half needs from one launch's channel; `ParentChannel` satisfies it. */
export type AgentViewParentChannel = Pick<ParentChannel, "onMessage" | "onDisconnect">;

/**
 * Parent half for one launch: stores each accepted model on the record as
 * `descendantViews` and drops it on disconnect. Only the record's current
 * channel may write. `onChanged` runs after every store or drop.
 */
export function observeAgentView(record: RpcAgentRecord, channel: AgentViewParentChannel & object, onChanged: () => void): () => void {
  const offMessage = channel.onMessage((msg) => {
    if (msg.t !== AGENT_VIEW_MESSAGE || record.channel !== channel) return;
    const model = parseAgentViewMessage(msg);
    if (!model) return;
    record.descendantViews = model;
    onChanged();
  });
  const offDisconnect = channel.onDisconnect(() => {
    if (record.channel !== channel || record.descendantViews === undefined) return;
    record.descendantViews = undefined;
    onChanged();
  });
  return () => {
    offMessage();
    offDisconnect();
  };
}

/** This process's child half, installed by `index.ts` when it has a parent channel. */
export const agentViewUpstreamRef: { current: AgentViewUpstream | undefined } = { current: undefined };

/** Dirty notification from the spawner: a direct child's batches or forwarded model changed. */
export function scheduleAgentViewPublish(): void {
  try {
    agentViewUpstreamRef.current?.schedule();
  } catch {
    // advisory display never fails a lifecycle event
  }
}
