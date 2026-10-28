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
}

/** Bounded per-agent batch history. Only the newest batch may be open (no `endedAt`). */
export interface AgentBatchBuffer {
  nextSeq: number;
  batches: ToolBatch[];
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

/** Width-independent projection of one batch: what the widget renders and what a hop relays. */
export interface BatchView {
  seq: number;
  /** Distinct tool names in call order, capped at `BATCH_TOOL_NAME_CAP`. */
  toolNames: string[];
  callCount: number;
  startedAt: number;
  /** Present exactly when the batch is finished. */
  endedAt?: number;
  /** Present exactly when the batch is still running. */
  running?: BatchRunningView;
}

export function createBatchBuffer(): AgentBatchBuffer {
  return { nextSeq: 1, batches: [] };
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
  if (buffer.batches.length > AGENT_BATCH_CAP) buffer.batches.splice(0, buffer.batches.length - AGENT_BATCH_CAP);
  return batch;
}

/** Ends the open batch, if any. Returns whether anything changed. */
export function closeOpenBatch(buffer: AgentBatchBuffer, now: number): boolean {
  const open = openBatch(buffer);
  if (!open) return false;
  open.endedAt = now;
  return true;
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
 * - assistant `message_end` with tool calls: ends any open batch and opens a
 *   new one announcing those calls (no tool calls: no batch).
 * - `tool_execution_start` / `tool_execution_end`: start and finish a call of
 *   the open batch (an unannounced call joins it, or opens one). The batch
 *   ends when every call has finished.
 * - `turn_end`, `agent_end`, `agent_settled`: end the open batch, so an
 *   aborted or failed turn never leaves a batch running.
 */
export function observeBatchEvent(
  buffer: AgentBatchBuffer,
  evt: { type?: string; message?: unknown; toolCallId?: string; toolName?: string; args?: unknown; result?: unknown; isError?: unknown },
  now: number,
): boolean {
  switch (evt.type) {
    case "message_end": {
      const calls = assistantToolCalls(evt.message);
      if (!calls || calls.length === 0) return false;
      closeOpenBatch(buffer, now);
      startBatch(buffer, now, calls);
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
export function batchView(batch: ToolBatch): BatchView {
  const toolNames: string[] = [];
  for (const call of batch.calls) {
    if (toolNames.length >= BATCH_TOOL_NAME_CAP) break;
    if (!toolNames.includes(call.toolName)) toolNames.push(call.toolName);
  }
  const view: BatchView = { seq: batch.seq, toolNames, callCount: batch.calls.length, startedAt: batch.startedAt };
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

/** The buffer's views, oldest first. */
export function batchViews(buffer: AgentBatchBuffer | undefined): BatchView[] {
  return buffer ? buffer.batches.map(batchView) : [];
}
