/**
 * The owner lead's feed into its session's ws dashboard.
 *
 * ws-mcp's adapter-only `dashboard.intake` tool takes ordered live events and
 * keeps the session's live snapshot and conversation ring. This module turns
 * the lead's own Pi events and the subagent registry into those events:
 *
 * - `reset` once when the feed starts, so state from an earlier process never
 *   lingers;
 * - `turn_start` / `turn_end` around each lead agent run (Pi `agent_start` to
 *   `agent_settled`), with the origin of the input or mailbox wake that
 *   started it;
 * - the lead's batch views, from a lead-side `AgentBatchBuffer` fed by the
 *   same event model the agent widget uses for children;
 * - one `agent` event per subagent whose state changed, keyed by its native
 *   agent id, and a `permission` event while an execute-worker waits on an
 *   approval;
 * - the lead's streamed reply text (the full text so far) and each finished
 *   reply with `final: true`;
 * - each finished user message with its origin.
 *
 * Pi itself has no tool permission prompt, so the lead never sends a
 * permission wait; only a subagent's pending `ws-worker-exec` approval does.
 *
 * The feed never affects the session. Event handlers only record state and
 * arm a timer; one flush at a time drains everything into a single intake
 * call, at most once per `FLUSH_INTERVAL_MS`, so streamed text, batch
 * changes, and subagent updates coalesce instead of costing one call per
 * token. Lead batches, reply text, and subagent state are read when a flush
 * drains, not per event; discrete events wait in a bounded list that drops
 * its oldest entry when full. Every call is fire-and-forget: rejections and
 * throws are swallowed, and a call that never answers is abandoned after
 * `INTAKE_CALL_TIMEOUT_MS` so the next flush can run.
 *
 * A call that fails (rejects, times out, answers with an error, or finds no
 * bridge or session key) counts as not delivered: its conversation events go
 * back to the front of the held list, the next call waits `RETRY_MS`, and
 * that call resyncs: `reset`, the open turn's `turn_start`, the lead's
 * batches, and every subagent again.
 *
 * The `pi.dashboard_enabled` knob is read at a flush at most once per
 * `KNOB_RECHECK_MS`. Off, the drained events are dropped, no intake call is
 * made, and the first call after it turns on again resyncs.
 */

import { ADAPTER_MESSAGE_LABEL, whiteboardLabeledBody } from "./adapter-label.ts";
import { batchViews, createBatchBuffer, observeBatchEvent, type AgentBatchBuffer, type BatchView } from "./agent-batches.ts";
import { classifyRegistryRowState, rowName, type AgentRowState } from "./agent-display.ts";
import { buildAgentViewModel } from "./agent-view.ts";
import type { GoalLoopConfig } from "./goal-loop.ts";
import { GATED_EXEC_TOOL_NAME, type RpcAgentRegistry } from "./spawner.ts";
import { selectSubtreeDescendants, type SubtreeDescendantRole } from "./subtree-lifecycle.ts";

/** The ws-mcp intake tool. Adapter-only: the bridge never registers it as a model tool. */
export const DASHBOARD_INTAKE_TOOL = "dashboard.intake";

/** The adapter name the intake records. */
export const DASHBOARD_ADAPTER_NAME = "pi";

/** Minimum gap between two intake calls: at most ten a second. */
export const FLUSH_INTERVAL_MS = 100;

/** A call still unanswered after this is abandoned, so a hung server cannot stop the feed. */
export const INTAKE_CALL_TIMEOUT_MS = 5000;

/** A knob read holds for this long: off, events drop without another read; on, flushes send without one. */
export const KNOB_RECHECK_MS = 2000;

/** Wait after a call that was not delivered before the next one. */
export const RETRY_MS = 2000;

/** Re-sample interval while any subagent was last sent as running, for changes no dirty mark reports. */
export const AGENT_POLL_MS = 1000;

/** Discrete events held between flushes; past it the oldest is dropped. */
export const PENDING_EVENT_CAP = 100;

/**
 * Characters of reply text sent. The intake keeps 64 KiB; one character more
 * than that lets it still mark the text truncated.
 */
export const REPLY_TEXT_SEND_CAP = 64 * 1024 + 1;

/** Literal `false` turns the feed off; absent, malformed, and every other value keep it on. */
export function resolveDashboardEnabled(config: GoalLoopConfig | undefined): boolean {
  return config?.dashboard_enabled !== false;
}

export type DashboardOrigin = "terminal" | "whiteboard" | "mail";

export interface DashboardEvent {
  kind: "reset" | "turn_start" | "turn_end" | "agent" | "permission" | "reply" | "user";
  [field: string]: unknown;
}

export interface DashboardPermission {
  tool: string;
  args: unknown;
}

/** One subagent as the feed sends it. */
export interface DashboardAgentEntry {
  agentId: string;
  parentId?: string;
  label: string;
  /** A delegate's own ws session key; ws-mcp shows its reply-id, never the key. */
  sessionKey?: string;
  status: "running" | "done" | "failed";
  caption: string;
  batches: BatchView[];
  permission?: DashboardPermission;
}

export interface DashboardTarget {
  callTool(name: string, args: Record<string, unknown>): Promise<unknown>;
  sessionKey: string | undefined;
}

export interface DashboardFeedOptions {
  /** The live bridge, read at each flush; `undefined` sends nothing. */
  target: () => DashboardTarget | undefined;
  /** The `pi.dashboard_enabled` read; must not throw, and a rejection counts as on. */
  enabled: () => boolean | Promise<boolean>;
  /** The subagent tree, read when a flush drains after a dirty mark. */
  agents?: () => DashboardAgentEntry[];
  now?: () => number;
  /** Arms a timer and returns its cancel. Defaults to an unref'd `setTimeout`. */
  schedule?: (fn: () => void, ms: number) => () => void;
}

export interface DashboardFeed {
  /** Clears all feed state and queues `reset`. */
  start(): void;
  /** Cancels timers and drops everything held; a stopped feed sends nothing more. */
  stop(): void;
  /** One lead Pi event: the input, agent, turn, message, and tool events. */
  leadEvent(event: unknown): void;
  /** A mailbox drain is being admitted: an idle lead's next turn is mail-started. */
  noteMailWake(): void;
  /** Dirty mark for the subagent tree. */
  agentsChanged(): void;
}

function defaultSchedule(fn: () => void, ms: number): () => void {
  const timer = setTimeout(fn, ms);
  timer.unref?.();
  return () => clearTimeout(timer);
}

/** The text blocks of a message's content, joined by newlines. */
function contentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((block) => block && typeof block === "object" && (block as { type?: unknown }).type === "text" && typeof (block as { text?: unknown }).text === "string")
    .map((block) => (block as { text: string }).text)
    .join("\n");
}

/** An assistant message's reply text: its text blocks only, never thinking or tool calls. */
export function assistantReplyText(message: unknown): string {
  const m = message as { role?: unknown; content?: unknown } | undefined;
  return m?.role === "assistant" ? contentText(m.content) : "";
}

/**
 * A finished user message as the feed records it: the whiteboard label is
 * stripped and becomes the `whiteboard` origin; every other message is
 * `terminal`. A message opening with the adapter label is adapter traffic,
 * not the user's, and is `undefined`, as is an empty one.
 */
export function dashboardUserMessage(message: unknown): { text: string; origin: "terminal" | "whiteboard" } | undefined {
  const m = message as { role?: unknown; content?: unknown } | undefined;
  if (m?.role !== "user") return undefined;
  const text = contentText(m.content);
  if (text.trim() === "" || text === ADAPTER_MESSAGE_LABEL || text.startsWith(`${ADAPTER_MESSAGE_LABEL}\n`)) return undefined;
  const body = whiteboardLabeledBody(text);
  return body !== undefined ? { text: body, origin: "whiteboard" } : { text, origin: "terminal" };
}

/**
 * The origin of a turn an input starts: the whiteboard label wins; text the
 * user typed (interactive or RPC) is `terminal`; any other extension-sent
 * input carries no origin.
 */
export function inputTurnOrigin(text: unknown, source: unknown): DashboardOrigin | undefined {
  if (typeof text === "string" && whiteboardLabeledBody(text) !== undefined) return "whiteboard";
  return source === "interactive" || source === "rpc" ? "terminal" : undefined;
}

/**
 * In-process Pi hands a `toolcall_start` the partial message instead of the
 * tool name RPC events carry; name it so the lead buffer reads like a child's.
 */
function withToolcallName(event: { assistantMessageEvent?: unknown }): unknown {
  const update = event.assistantMessageEvent as { type?: unknown; toolName?: unknown; contentIndex?: unknown; partial?: { content?: unknown } } | undefined;
  if (update?.type !== "toolcall_start" || typeof update.toolName === "string") return event;
  const content = update.partial?.content;
  const block = Array.isArray(content) && typeof update.contentIndex === "number" ? content[update.contentIndex] as { name?: unknown } | undefined : undefined;
  return { ...event, assistantMessageEvent: { ...update, ...(typeof block?.name === "string" ? { toolName: block.name } : {}) } };
}

/** States in which an agent still works (or waits on something it started). */
const ACTIVE_STATES = new Set<AgentRowState>(["running", "awaiting-approval", "waiting-on-children"]);

const STATE_CAPTIONS: Record<AgentRowState, string | undefined> = {
  "running": undefined,
  "awaiting-approval": "awaiting approval",
  "waiting-on-children": "waiting on its agents",
  "awaiting-owner": "in a thread with the user",
  "idle-awaiting-owner": "waiting for the user",
  "pending-delivery": "result pending delivery",
};

function caption(role: SubtreeDescendantRole, state: AgentRowState | null | undefined): string {
  const phrase = state ? STATE_CAPTIONS[state] : undefined;
  return phrase ? `${role} · ${phrase}` : role;
}

/**
 * The subagent tree for the feed: the same agents, order, and batch views the
 * agent widget and the descendant relay use. A direct child's status comes
 * from its record (running while working or launching; failed when its work
 * ended by exit or failed launch); a deeper descendant's from the display row
 * its owner relayed. Only a fork's own session key is known to its parent.
 */
export function dashboardAgentEntries(registry: RpcAgentRegistry): DashboardAgentEntry[] {
  const views = buildAgentViewModel(registry);
  return selectSubtreeDescendants(registry).map((row) => {
    const record = registry.get(row.id);
    const state = record ? classifyRegistryRowState(record) : row.display?.state;
    const active = record
      ? record.launching !== undefined || record.running || record.streaming || (state !== undefined && ACTIVE_STATES.has(state))
      : state != null && ACTIVE_STATES.has(state);
    const status = active ? "running" : record?.lastRunFailed ? "failed" : "done";
    const entry: DashboardAgentEntry = {
      agentId: row.id,
      ...(row.parentId ? { parentId: row.parentId } : {}),
      label: row.display?.name ?? (record ? rowName(record) : row.id.slice(0, 8)),
      ...(record?.ownSessionKey ? { sessionKey: record.ownSessionKey } : {}),
      status,
      caption: caption(row.role, state),
      batches: views.get(row.id) ?? [],
    };
    const approval = record?.pendingApproval;
    if (approval && approval.decision !== "sent") entry.permission = { tool: GATED_EXEC_TOOL_NAME, args: { command: approval.command } };
    return entry;
  });
}

interface SentAgent {
  key: string;
  status: DashboardAgentEntry["status"];
  permission: string;
}

export function createDashboardFeed(options: DashboardFeedOptions): DashboardFeed {
  const now = options.now ?? Date.now;
  const schedule = options.schedule ?? defaultSchedule;

  let stopped = true;
  let pending: DashboardEvent[] = [];
  let lead: AgentBatchBuffer = createBatchBuffer();
  let leadDirty = false;
  let agentsDirty = false;
  let sentAgents = new Map<string, SentAgent>();
  let streamMessage: unknown;
  let lastStreamText: string | undefined;
  let turnOpen = false;
  let nextTurnOrigin: DashboardOrigin | undefined;
  let resyncDue = false;
  let disabledUntil = 0;
  let enabledUntil = 0;
  let retryAt = 0;
  /** The open turn's `turn_start`, or the last turn's once it closed, for a resync to resend. */
  let lastTurnStart: DashboardEvent | undefined;
  let inFlight = false;
  let lastFlushAt: number | undefined;
  let cancelFlush: (() => void) | undefined;
  let cancelPoll: (() => void) | undefined;

  function hasWork(): boolean {
    return pending.length > 0 || leadDirty || agentsDirty || streamMessage !== undefined;
  }

  function scheduleFlush(): void {
    if (stopped || inFlight || cancelFlush || !hasWork()) return;
    const delay = Math.max(0, lastFlushAt === undefined ? 0 : lastFlushAt + FLUSH_INTERVAL_MS - now(), retryAt - now());
    cancelFlush = schedule(() => {
      cancelFlush = undefined;
      flush();
    }, delay);
  }

  function push(event: DashboardEvent): void {
    pending.push(event);
    if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP);
  }

  function leadEventNow(): DashboardEvent {
    return { kind: "agent", batches: batchViews(lead) };
  }

  /** Streamed text, when it changed since the last send. */
  function streamEvent(): DashboardEvent | undefined {
    const message = streamMessage;
    streamMessage = undefined;
    if (message === undefined) return undefined;
    const text = assistantReplyText(message).slice(0, REPLY_TEXT_SEND_CAP);
    if (text === lastStreamText) return undefined;
    lastStreamText = text;
    return { kind: "reply", text };
  }

  /** Puts held lead and stream state ahead of a discrete event, so ordering survives coalescing. */
  function settleBefore(event: DashboardEvent): void {
    if (leadDirty) {
      leadDirty = false;
      push(leadEventNow());
    }
    if (event.kind === "reply" && event.final === true) {
      streamMessage = undefined;
      lastStreamText = undefined;
    } else {
      const stream = streamEvent();
      if (stream) push(stream);
    }
    push(event);
    scheduleFlush();
  }

  function agentEvents(): DashboardEvent[] {
    agentsDirty = false;
    if (!options.agents) return [];
    let entries: DashboardAgentEntry[];
    try {
      entries = options.agents();
    } catch {
      return [];
    }
    const out: DashboardEvent[] = [];
    const seen = new Set<string>();
    for (const entry of entries) {
      seen.add(entry.agentId);
      const event: DashboardEvent = {
        kind: "agent",
        agentId: entry.agentId,
        ...(entry.parentId ? { parentId: entry.parentId } : {}),
        label: entry.label,
        ...(entry.sessionKey ? { sessionKey: entry.sessionKey } : {}),
        status: entry.status,
        caption: entry.caption,
        batches: entry.batches,
      };
      const key = JSON.stringify(event);
      const permission = entry.permission ? JSON.stringify(entry.permission) : "";
      const sent = sentAgents.get(entry.agentId);
      if (sent?.key !== key) out.push(event);
      if ((sent?.permission ?? "") !== permission) {
        out.push(entry.permission ? { kind: "permission", agentId: entry.agentId, tool: entry.permission.tool, args: entry.permission.args } : { kind: "permission", agentId: entry.agentId, clear: true });
      }
      sentAgents.set(entry.agentId, { key, status: entry.status, permission });
    }
    for (const [agentId, sent] of sentAgents) {
      if (seen.has(agentId)) continue;
      // Gone from the registry: one that was still running ends as done.
      if (sent.status === "running") out.push({ kind: "agent", agentId, status: "done" });
      sentAgents.delete(agentId);
    }
    return out;
  }

  function drain(): DashboardEvent[] {
    const events = pending;
    pending = [];
    if (leadDirty) {
      leadDirty = false;
      events.push(leadEventNow());
    }
    if (agentsDirty) events.push(...agentEvents());
    const stream = streamEvent();
    if (stream) events.push(stream);
    return events;
  }

  /** The feed reached nobody: what ws-mcp holds is stale, so resend everything once it does. */
  function markResync(): void {
    resyncDue = true;
    sentAgents = new Map();
    lastStreamText = undefined;
    leadDirty = true;
    agentsDirty = options.agents !== undefined;
  }

  /** The conversation events of an undelivered call, which a resync cannot rebuild from current state. */
  function requeue(events: DashboardEvent[]): void {
    const kept = events.filter((event) => event.kind === "user" || event.kind === "turn_start" || event.kind === "turn_end" || (event.kind === "reply" && event.final === true));
    pending = [...kept, ...pending];
    if (pending.length > PENDING_EVENT_CAP) pending.splice(0, pending.length - PENDING_EVENT_CAP);
  }

  /** `reset` and, when the turn it belongs to is not already in `events`, the turn's `turn_start`. */
  function resyncPrefix(events: DashboardEvent[]): DashboardEvent[] {
    const firstStart = events.findIndex((event) => event.kind === "turn_start");
    const firstEnd = events.findIndex((event) => event.kind === "turn_end");
    const needStart = lastTurnStart !== undefined && (firstStart === -1 ? turnOpen || firstEnd !== -1 : firstEnd !== -1 && firstEnd < firstStart);
    return needStart ? [{ kind: "reset" }, lastTurnStart!] : [{ kind: "reset" }];
  }

  function armPoll(): void {
    if (stopped || cancelPoll || ![...sentAgents.values()].some((sent) => sent.status === "running")) return;
    cancelPoll = schedule(() => {
      cancelPoll = undefined;
      agentsChanged();
    }, AGENT_POLL_MS);
  }

  function flush(): void {
    if (stopped || inFlight) return;
    if (now() < disabledUntil) {
      // Recently read off: drop without building the tree or reading again.
      pending = [];
      leadDirty = false;
      agentsDirty = false;
      streamMessage = undefined;
      markResync();
      return;
    }
    let events = drain();
    if (events.length === 0) return;
    inFlight = true;
    lastFlushAt = now();
    let finished = false;
    let outcome: "delivered" | "off" | "failed" = "failed";
    let cancelTimeout: (() => void) | undefined;
    const done = (): void => {
      if (finished) return;
      finished = true;
      cancelTimeout?.();
      inFlight = false;
      lastFlushAt = now();
      if (!stopped && outcome !== "delivered") {
        // Dropped (off) or maybe lost (failed): ws-mcp's state is stale either way.
        markResync();
        if (outcome === "failed") {
          requeue(events);
          retryAt = now() + RETRY_MS;
        }
      }
      armPoll();
      scheduleFlush();
    };
    cancelTimeout = schedule(done, INTAKE_CALL_TIMEOUT_MS);
    const send = (enabled: unknown): unknown => {
      // A read that resolves after its call was abandoned must not send a stale batch.
      if (stopped || finished) return undefined;
      if (enabled === false) {
        disabledUntil = now() + KNOB_RECHECK_MS;
        enabledUntil = 0;
        outcome = "off";
        return undefined;
      }
      enabledUntil = Math.max(enabledUntil, now() + KNOB_RECHECK_MS);
      const target = options.target();
      if (!target?.sessionKey) return undefined;
      if (resyncDue) {
        resyncDue = false;
        events = [...resyncPrefix(events), ...events];
      }
      return Promise.resolve(target.callTool(DASHBOARD_INTAKE_TOOL, { session_key: target.sessionKey, adapter: DASHBOARD_ADAPTER_NAME, events })).then((result) => {
        if (!finished && (result as { isError?: unknown } | undefined)?.isError !== true) outcome = "delivered";
      });
    };
    let run: Promise<unknown>;
    try {
      const enabled = now() < enabledUntil ? true : options.enabled();
      run = Promise.resolve(enabled).then((value) => value, () => true).then(send);
    } catch {
      run = Promise.resolve();
    }
    run.then(done, done);
  }

  function agentsChanged(): void {
    if (stopped) return;
    agentsDirty = true;
    scheduleFlush();
  }

  function onLeadEvent(event: unknown): void {
    const e = event as { type?: string; message?: unknown; assistantMessageEvent?: unknown; text?: unknown; source?: unknown };
    switch (e.type) {
      case "input": {
        // An input with no origin of its own (the adapter's wake message for
        // held mail, say) keeps the origin already noted for the next turn.
        const origin = inputTurnOrigin(e.text, e.source);
        if (!turnOpen && origin) nextTurnOrigin = origin;
        return;
      }
      case "agent_start":
        if (turnOpen) return;
        turnOpen = true;
        lastTurnStart = { kind: "turn_start", ...(nextTurnOrigin ? { origin: nextTurnOrigin } : {}), at: now() };
        settleBefore(lastTurnStart);
        nextTurnOrigin = undefined;
        return;
      case "agent_settled":
        if (observeBatchEvent(lead, { type: e.type }, now())) leadDirty = true;
        if (!turnOpen) {
          scheduleFlush();
          return;
        }
        turnOpen = false;
        settleBefore({ kind: "turn_end", at: now() });
        return;
      case "message_update": {
        if (observeBatchEvent(lead, withToolcallName(e) as Parameters<typeof observeBatchEvent>[1], now())) leadDirty = true;
        const kind = (e.assistantMessageEvent as { type?: unknown } | undefined)?.type;
        if ((e.message as { role?: unknown } | undefined)?.role === "assistant" && (kind === "text_start" || kind === "text_delta" || kind === "text_end")) streamMessage = e.message;
        scheduleFlush();
        return;
      }
      case "message_end": {
        if (observeBatchEvent(lead, e as Parameters<typeof observeBatchEvent>[1], now())) leadDirty = true;
        const text = assistantReplyText(e.message);
        if (text !== "") {
          settleBefore({ kind: "reply", text: text.slice(0, REPLY_TEXT_SEND_CAP), final: true, at: now() });
          return;
        }
        const user = dashboardUserMessage(e.message);
        if (user) {
          settleBefore({ kind: "user", text: user.text, origin: user.origin, at: now() });
          return;
        }
        scheduleFlush();
        return;
      }
      case "message_start":
      case "tool_execution_start":
      case "tool_execution_end":
      case "turn_end":
      case "agent_end":
        if (observeBatchEvent(lead, e as Parameters<typeof observeBatchEvent>[1], now())) leadDirty = true;
        scheduleFlush();
        return;
      default:
        return;
    }
  }

  function reset(): void {
    cancelFlush?.();
    cancelFlush = undefined;
    cancelPoll?.();
    cancelPoll = undefined;
    pending = [];
    lead = createBatchBuffer();
    leadDirty = false;
    agentsDirty = false;
    sentAgents = new Map();
    streamMessage = undefined;
    lastStreamText = undefined;
    turnOpen = false;
    nextTurnOrigin = undefined;
    resyncDue = false;
    disabledUntil = 0;
    enabledUntil = 0;
    retryAt = 0;
    lastTurnStart = undefined;
  }

  return {
    start() {
      reset();
      stopped = false;
      push({ kind: "reset" });
      agentsDirty = options.agents !== undefined;
      scheduleFlush();
    },
    stop() {
      reset();
      stopped = true;
    },
    leadEvent(event) {
      if (stopped) return;
      try {
        onLeadEvent(event);
      } catch {
        // The feed is advisory: a malformed event never reaches the session.
      }
    },
    noteMailWake() {
      if (stopped || turnOpen) return;
      nextTurnOrigin = "mail";
    },
    agentsChanged() {
      try {
        agentsChanged();
      } catch {
        // advisory
      }
    },
  };
}

/** The lead Pi events the feed reads. */
export const DASHBOARD_LEAD_EVENTS = [
  "input", "agent_start", "agent_end", "agent_settled", "turn_end",
  "message_start", "message_update", "message_end", "tool_execution_start", "tool_execution_end",
] as const;

/**
 * Subscribes the lead events once per extension load (Pi offers no
 * unsubscribe, so a reload must not stack handlers); each handler forwards to
 * whichever feed is current and returns nothing, so it never changes an event.
 */
export function registerDashboardLeadEvents(pi: { on(event: string, handler: (event: unknown) => void): void }, feed: () => DashboardFeed | undefined): void {
  for (const name of DASHBOARD_LEAD_EVENTS) {
    pi.on(name, (event) => {
      try {
        feed()?.leadEvent(event);
      } catch {
        // advisory
      }
    });
  }
}
