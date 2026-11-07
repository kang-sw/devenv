/**
 * 261028 (`261028-feat-pi-agent-widget-expanded-mode`): the per-agent tool
 * batch model behind the live agent widget's expanded mode.
 *
 * A batch is one assistant message and the tool calls it made. An assistant
 * message with no tool calls makes no batch. The spawner feeds every direct
 * child's RPC event into that child's bounded `AgentBatchBuffer`
 * (`observeBatchEvent`, from its existing `client.onEvent` handler); the
 * widget renders the width-independent `BatchView` projection
 * (`batchViews`). The same projection is the unit later relayed for
 * descendants, so a local child and a relayed descendant render through one
 * code path.
 *
 * Pi runs one message's tool calls in parallel by default, so a running
 * batch can have several calls in flight; the view reports the OLDEST
 * in-flight call (the one holding the batch up), the finished-call count,
 * and how many other calls are still running.
 *
 * 261028 batch-row redesign: before a batch exists, the assistant message
 * that may become one is shown as a generating row. It opens at an assistant
 * `message_start`, follows the `message_update` block boundaries (thinking,
 * reply text, tool-call argument streaming with the streaming call's tool
 * name and character count), and at that message's `message_end` either
 * becomes the batch (tool calls) or disappears (none). It is held outside
 * the `AGENT_BATCH_CAP` list, so a tool-free message never evicts a batch.
 * Over RPC an assistant event carries no `partial` message, so a
 * `toolcall_start` names its tool directly and a `toolcall_delta` holds only
 * its `delta` text; every delta is O(1).
 *
 * The dashboard feed's lead buffer differs in two ways
 * (`createBatchBuffer({ retainUndelivered: true })`, `dashboardBatchViews`):
 * it keeps, beyond the newest `AGENT_BATCH_CAP`, every finished batch no
 * delivered intake call carried yet (up to `BATCH_RETAIN_CAP`;
 * `markBatchesDelivered` releases them), so ws-mcp counts each batch once
 * even when several finish between two calls; and a finished batch's view
 * carries its calls (tool name, clocks, failed; never an argument or a
 * result) and its message's token usage. Neither reaches the widget or a
 * relayed view.
 *
 * Pure data and pure functions only: no IO, no timers, no registry access.
 */

/** Batches retained per agent; also the per-agent cap a relayed view carries, since a child does not know the root's row count. */
export const AGENT_BATCH_CAP = 5;

/** Distinct tool names a batch view carries. */
export const BATCH_TOOL_NAME_CAP = 5;

/** Characters an args preview keeps before the widget's own width truncation. */
export const BATCH_ARGS_PREVIEW_CAP = 200;

/** Characters of a call's result kept on a direct child's buffer for a later summary request. */
export const BATCH_RESULT_PREVIEW_CAP = 400;

/** Batches a retaining buffer keeps at most while finished ones wait for a delivered call; past it the oldest go uncounted. */
export const BATCH_RETAIN_CAP = 32;

/** Calls a finished batch's dashboard view lists; the rest are only counted. */
export const BATCH_CALL_CAP = 32;

/** One assistant message's token usage, as the dashboard intake takes it. */
export interface BatchTokenUsage {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
}

/** One tool call inside a batch. Clocks are epoch milliseconds. */
export interface BatchCall {
  id: string;
  toolName: string;
  argsPreview: string;
  /** Set at `tool_execution_start`; absent while the call is only announced by its message. */
  startedAt?: number;
  /** Set at `tool_execution_end`. */
  endedAt?: number;
  isError?: boolean;
  resultPreview?: string;
}

export interface ToolBatch {
  /** Per-agent sequence number, the stable identity of this batch. */
  seq: number;
  startedAt: number;
  /** Present once the batch is finished. */
  endedAt?: number;
  calls: BatchCall[];
  /** The assistant message's token usage, when it reported any. */
  usage?: BatchTokenUsage;
  /** A retaining buffer's mark: a delivered intake call carried this batch finished. */
  delivered?: boolean;
}

/** Generating-phase kinds: thinking (also before the first content block), reply text, tool-call argument streaming. */
export type BatchGeneratingKind = "thinking" | "replying" | "tool";

/** The generating row's current phase. */
export interface BatchGeneratingView {
  kind: BatchGeneratingKind;
  /** Start of the current phase, so thinking and argument-writing time read separately. */
  phaseStartedAt: number;
  /** The streaming call's tool name; present exactly when `kind` is `tool`. */
  toolName?: string;
  /** Characters of the streaming call's arguments so far; present exactly when `kind` is `tool`. */
  chars?: number;
}

/** An assistant message in flight, not yet a batch. */
export interface GeneratingState extends BatchGeneratingView {
  /** The message's `message_start`. */
  startedAt: number;
}

/** Bounded per-agent batch history. Only the newest batch may be open (no `endedAt`). */
export interface AgentBatchBuffer {
  /**
   * Identity of this buffer's lifetime. `seq` restarts at 1 whenever a buffer
   * is recreated (a relaunched intermediate agent revives its descendants with
   * fresh buffers), so the epoch is what keeps a batch's identity unique across
   * launches. Carried on every `BatchView`.
   */
  epoch: string;
  nextSeq: number;
  batches: ToolBatch[];
  /** The assistant message in flight, outside the capped `batches` list. */
  generating?: GeneratingState;
  /** Keep finished batches older than the newest `AGENT_BATCH_CAP` until they are delivered (the dashboard feed's lead). */
  retainUndelivered?: boolean;
}

/** The running batch's in-flight summary. */
export interface BatchRunningView {
  /** Oldest call still in flight; before any call starts, the first unfinished call. */
  toolName: string;
  argsPreview: string;
  startedAt: number;
  done: number;
  /** In-flight calls other than the one reported. */
  othersRunning: number;
}

/**
 * Width-independent projection of one batch: what the widget renders and what
 * a hop relays. Exactly one of `endedAt`, `running` and `generating` is
 * present.
 */
export interface BatchView {
  /** The producing buffer's `epoch`; absent from a peer that predates it. Pair with `seq` for a launch-unique identity. */
  epoch?: string;
  /** The batch's sequence number; a generating view carries the number its batch would take. */
  seq: number;
  /** Distinct tool names in call order, capped at `BATCH_TOOL_NAME_CAP`. */
  toolNames: string[];
  callCount: number;
  /** Failed calls: an error result, or a call still unfinished when its batch was closed. */
  failed: number;
  startedAt: number;
  /** Present exactly when the batch is finished. */
  endedAt?: number;
  /** Present exactly when the batch is still running. */
  running?: BatchRunningView;
  /** Present exactly when the view is an assistant message still generating. */
  generating?: BatchGeneratingView;
  /** A finished batch's calls, up to `BATCH_CALL_CAP`; dashboard views only. */
  calls?: { name: string; startedAt: number; endedAt?: number; failed: boolean }[];
  /** A finished batch's token usage; dashboard views only. */
  usage?: BatchTokenUsage;
}

export function createBatchBuffer(options: { retainUndelivered?: boolean } = {}): AgentBatchBuffer {
  return { epoch: Math.random().toString(36).slice(2, 10).padEnd(8, "0"), nextSeq: 1, batches: [], ...(options.retainUndelivered ? { retainUndelivered: true } : {}) };
}

/** An assistant message's `usage` as a `BatchTokenUsage`; undefined when absent or all zero. */
export function messageTokens(message: unknown): BatchTokenUsage | undefined {
  const u = (message as { usage?: unknown } | undefined)?.usage as { input?: unknown; output?: unknown; cacheRead?: unknown; cacheWrite?: unknown } | undefined;
  if (!u || typeof u !== "object") return undefined;
  const n = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) && v > 0 ? Math.round(v) : 0);
  const t = { input: n(u.input), output: n(u.output), cacheRead: n(u.cacheRead), cacheWrite: n(u.cacheWrite) };
  return t.input || t.output || t.cacheRead || t.cacheWrite ? t : undefined;
}

/**
 * A tool name is model-supplied; strip control characters before it can
 * reach a terminal row (relayed names get the same treatment in agent-view).
 */
export function cleanToolName(name: string): string {
  return name.replace(/[\u0000-\u001f\u007f-\u009f]/g, " ").trim() || "tool";
}

/**
 * Flattens a call's arguments to one display line: object entries as
 * `key: value` joined by `, ` (strings raw, everything else JSON), whitespace
 * runs collapsed to one space, control characters removed, and the result
 * capped at `cap` characters with a trailing ellipsis.
 */
export function flattenArgsPreview(args: unknown, cap: number = BATCH_ARGS_PREVIEW_CAP): string {
  let text: string;
  if (args === undefined || args === null) text = "";
  else if (typeof args === "string") text = args;
  else if (typeof args === "object" && !Array.isArray(args)) {
    text = Object.entries(args as Record<string, unknown>)
      .map(([key, value]) => `${key}: ${typeof value === "string" ? value : safeJson(value)}`)
      .join(", ");
  } else text = safeJson(args);
  return capText(text, cap);
}

function safeJson(value: unknown): string {
  try {
    return JSON.stringify(value) ?? String(value);
  } catch {
    return String(value);
  }
}

function capText(text: string, cap: number): string {
  const flat = text.replace(/[\u0000-\u0008\u000e-\u001f\u007f-\u009f]/g, "").replace(/\s+/g, " ").trim();
  const chars = [...flat];
  if (chars.length <= cap) return flat;
  return cap <= 1 ? "…".slice(0, cap) : chars.slice(0, cap - 1).join("") + "…";
}

/** Result text of a tool execution: the text content blocks when present, else JSON. */
function resultPreview(result: unknown): string {
  const content = (result as { content?: unknown } | undefined)?.content;
  if (Array.isArray(content)) {
    const text = content
      .map((block) => (block && typeof block === "object" && (block as { type?: unknown }).type === "text" ? String((block as { text?: unknown }).text ?? "") : ""))
      .filter((part) => part !== "")
      .join(" ");
    if (text !== "") return capText(text, BATCH_RESULT_PREVIEW_CAP);
  }
  return result === undefined ? "" : capText(safeJson(result), BATCH_RESULT_PREVIEW_CAP);
}

function openBatch(buffer: AgentBatchBuffer): ToolBatch | undefined {
  const last = buffer.batches[buffer.batches.length - 1];
  return last && last.endedAt === undefined ? last : undefined;
}

function startBatch(buffer: AgentBatchBuffer, now: number, calls: BatchCall[]): ToolBatch {
  const batch: ToolBatch = { seq: buffer.nextSeq++, startedAt: now, calls };
  buffer.batches.push(batch);
  trimBatches(buffer);
  return batch;
}

/** Keeps the newest `AGENT_BATCH_CAP`; a retaining buffer also keeps older undelivered ones, up to `BATCH_RETAIN_CAP` in all. */
function trimBatches(buffer: AgentBatchBuffer): void {
  const n = buffer.batches.length;
  let drop = 0;
  while (n - drop > AGENT_BATCH_CAP && (!buffer.retainUndelivered || buffer.batches[drop]!.delivered === true || n - drop > BATCH_RETAIN_CAP)) drop++;
  if (drop > 0) buffer.batches.splice(0, drop);
}

/**
 * A delivered intake call carried `views`: the finished batches among them
 * are delivered, and those older than the newest `AGENT_BATCH_CAP` leave a
 * retaining buffer.
 */
export function markBatchesDelivered(buffer: AgentBatchBuffer, views: readonly { epoch?: unknown; seq?: unknown; endedAt?: unknown }[]): void {
  for (const view of views) {
    if (view.epoch !== buffer.epoch || view.endedAt === undefined) continue;
    const batch = buffer.batches.find((candidate) => candidate.seq === view.seq);
    if (batch?.endedAt !== undefined) batch.delivered = true;
  }
  trimBatches(buffer);
}

/** Ends the open batch and drops any generating message. Returns whether anything changed. */
export function closeOpenBatch(buffer: AgentBatchBuffer, now: number): boolean {
  const hadGenerating = buffer.generating !== undefined;
  buffer.generating = undefined;
  const open = openBatch(buffer);
  if (!open) return hadGenerating;
  open.endedAt = now;
  return true;
}

/**
 * Applies one `message_update` to the generating state: `thinking_start`,
 * `text_start` and `toolcall_start` move the phase (the same kind again
 * keeps its phase clock, except a new tool call); `toolcall_delta` adds its
 * `delta` length to the streaming call's count. Every other update, and any
 * update with no generating message, changes nothing. O(1) per event.
 */
function observeGenerating(buffer: AgentBatchBuffer, update: unknown, now: number): boolean {
  const generating = buffer.generating;
  if (!generating || !update || typeof update !== "object") return false;
  const event = update as { type?: unknown; delta?: unknown; toolName?: unknown };
  switch (event.type) {
    case "toolcall_delta":
      if (generating.kind !== "tool" || typeof event.delta !== "string" || event.delta.length === 0) return false;
      generating.chars = (generating.chars ?? 0) + event.delta.length;
      return true;
    case "thinking_start":
    case "text_start": {
      const kind: BatchGeneratingKind = event.type === "thinking_start" ? "thinking" : "replying";
      if (generating.kind === kind) return false;
      buffer.generating = { startedAt: generating.startedAt, kind, phaseStartedAt: now };
      return true;
    }
    case "toolcall_start":
      buffer.generating = { startedAt: generating.startedAt, kind: "tool", phaseStartedAt: now, toolName: typeof event.toolName === "string" ? cleanToolName(event.toolName) : "tool", chars: 0 };
      return true;
    default:
      return false;
  }
}

function assistantToolCalls(message: unknown): BatchCall[] | undefined {
  const m = message as { role?: unknown; content?: unknown } | undefined;
  if (!m || m.role !== "assistant" || !Array.isArray(m.content)) return undefined;
  const calls: BatchCall[] = [];
  for (const block of m.content) {
    const b = block as { type?: unknown; id?: unknown; name?: unknown; arguments?: unknown } | undefined;
    if (!b || b.type !== "toolCall" || typeof b.name !== "string") continue;
    calls.push({ id: typeof b.id === "string" ? b.id : "", toolName: cleanToolName(b.name), argsPreview: flattenArgsPreview(b.arguments) });
  }
  return calls;
}

/**
 * Applies one RPC event to `buffer`. Returns whether the buffer changed.
 *
 * - assistant `message_start`: opens the generating message (phase
 *   `thinking` until its first content block says otherwise).
 * - `message_update`: moves the generating phase or counts tool-call
 *   argument characters (`observeGenerating`).
 * - assistant `message_end`: drops the generating message; with tool calls,
 *   ends any open batch and opens a new one announcing those calls (no tool
 *   calls: no batch).
 * - `tool_execution_start` / `tool_execution_end`: start and finish a call of
 *   the open batch (an unannounced call joins it, or opens one). The batch
 *   ends when every call has finished.
 * - `turn_end`, `agent_end`, `agent_settled`: end the open batch and drop
 *   the generating message, so an aborted or failed turn never leaves either
 *   running. Calls still unfinished then count as failed (`batchView`).
 */
export function observeBatchEvent(
  buffer: AgentBatchBuffer,
  evt: { type?: string; message?: unknown; assistantMessageEvent?: unknown; toolCallId?: string; toolName?: string; args?: unknown; result?: unknown; isError?: unknown },
  now: number,
): boolean {
  switch (evt.type) {
    case "message_start": {
      if ((evt.message as { role?: unknown } | undefined)?.role !== "assistant") return false;
      buffer.generating = { startedAt: now, kind: "thinking", phaseStartedAt: now };
      return true;
    }
    case "message_update":
      return observeGenerating(buffer, evt.assistantMessageEvent, now);
    case "message_end": {
      const calls = assistantToolCalls(evt.message);
      if (!calls) return false;
      const hadGenerating = buffer.generating !== undefined;
      buffer.generating = undefined;
      if (calls.length === 0) return hadGenerating;
      const open = openBatch(buffer);
      if (open) open.endedAt = now;
      const batch = startBatch(buffer, now, calls);
      const usage = messageTokens(evt.message);
      if (usage) batch.usage = usage;
      return true;
    }
    case "tool_execution_start": {
      const id = typeof evt.toolCallId === "string" ? evt.toolCallId : "";
      const toolName = typeof evt.toolName === "string" ? cleanToolName(evt.toolName) : "tool";
      const batch = openBatch(buffer) ?? startBatch(buffer, now, []);
      let call = batch.calls.find((candidate) => candidate.id === id && candidate.startedAt === undefined);
      if (!call) {
        call = { id, toolName, argsPreview: flattenArgsPreview(evt.args) };
        batch.calls.push(call);
      } else if (evt.args !== undefined) call.argsPreview = flattenArgsPreview(evt.args);
      call.startedAt = now;
      return true;
    }
    case "tool_execution_end": {
      const id = typeof evt.toolCallId === "string" ? evt.toolCallId : "";
      const batch = openBatch(buffer);
      const call = batch?.calls.find((candidate) => candidate.id === id && candidate.endedAt === undefined);
      if (!batch || !call) return false;
      call.startedAt ??= now;
      call.endedAt = now;
      call.isError = evt.isError === true;
      call.resultPreview = resultPreview(evt.result);
      if (batch.calls.every((candidate) => candidate.endedAt !== undefined)) batch.endedAt = now;
      return true;
    }
    case "turn_end":
    case "agent_end":
    case "agent_settled":
      return closeOpenBatch(buffer, now);
    default:
      return false;
  }
}

/** Projects one batch to its width-independent view. */
export function batchView(batch: ToolBatch, epoch?: string): BatchView {
  const toolNames: string[] = [];
  for (const call of batch.calls) {
    if (toolNames.length >= BATCH_TOOL_NAME_CAP) break;
    if (!toolNames.includes(call.toolName)) toolNames.push(call.toolName);
  }
  const failed = batch.calls.filter((call) => call.isError === true || (batch.endedAt !== undefined && call.endedAt === undefined)).length;
  const view: BatchView = { ...(epoch !== undefined ? { epoch } : {}), seq: batch.seq, toolNames, callCount: batch.calls.length, failed, startedAt: batch.startedAt };
  if (batch.endedAt !== undefined) {
    view.endedAt = batch.endedAt;
    return view;
  }
  const done = batch.calls.filter((call) => call.endedAt !== undefined).length;
  const inFlight = batch.calls.filter((call) => call.startedAt !== undefined && call.endedAt === undefined);
  let reported: BatchCall | undefined;
  for (const call of inFlight) if (!reported || call.startedAt! < reported.startedAt!) reported = call;
  reported ??= batch.calls.find((call) => call.endedAt === undefined);
  view.running = {
    toolName: reported?.toolName ?? "tool",
    argsPreview: reported?.argsPreview ?? "",
    startedAt: reported?.startedAt ?? batch.startedAt,
    done,
    othersRunning: Math.max(0, inFlight.length - (reported && reported.startedAt !== undefined ? 1 : 0)),
  };
  return view;
}

/** The generating message's view: no calls yet, the sequence number its batch would take. */
function generatingView(buffer: AgentBatchBuffer, generating: GeneratingState): BatchView {
  const phase: BatchGeneratingView = { kind: generating.kind, phaseStartedAt: generating.phaseStartedAt };
  if (generating.kind === "tool") {
    phase.toolName = generating.toolName ?? "tool";
    phase.chars = generating.chars ?? 0;
  }
  return { epoch: buffer.epoch, seq: buffer.nextSeq, toolNames: [], callCount: 0, failed: 0, startedAt: generating.startedAt, generating: phase };
}

/**
 * The buffer's views for the dashboard intake: `batchViews`, each finished
 * batch also carrying its calls (names, clocks, failed) and token usage.
 */
export function dashboardBatchViews(buffer: AgentBatchBuffer): BatchView[] {
  const views = batchViews(buffer);
  for (const batch of buffer.batches) {
    if (batch.endedAt === undefined) continue;
    const view = views.find((candidate) => candidate.seq === batch.seq && candidate.generating === undefined);
    if (!view) continue;
    view.calls = batch.calls.slice(0, BATCH_CALL_CAP).map((call) => ({
      name: call.toolName,
      startedAt: call.startedAt ?? batch.startedAt,
      ...(call.endedAt !== undefined ? { endedAt: call.endedAt } : {}),
      failed: call.isError === true || call.endedAt === undefined,
    }));
    if (batch.usage) view.usage = { ...batch.usage };
  }
  return views;
}

/** The buffer's views, oldest first; a generating message's view is last. */
export function batchViews(buffer: AgentBatchBuffer | undefined): BatchView[] {
  if (!buffer) return [];
  const views = buffer.batches.map((batch) => batchView(batch, buffer.epoch));
  if (buffer.generating) views.push(generatingView(buffer, buffer.generating));
  return views;
}
