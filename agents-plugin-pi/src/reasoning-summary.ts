/** Explicit provider summaries only. Raw events and replay/signature data never leave this observer. */
import { randomUUID } from "node:crypto";

export const REASONING_SUMMARY_CAPABILITY = "reasoning_summary_v1";
export const SUMMARY_TEXT_BYTES = 8192;
export const SUMMARY_PROJECTION_BYTES = 24576;
export const SUMMARY_FRAME_BYTES = 49152;
export const SUMMARY_ATTEMPTS = 16;
export const SUMMARY_PARTS = 64;
export type SummaryStatus = "streaming" | "complete" | "aborted" | "failed" | "incomplete";
export interface SummaryPart {
  output: number;
  part: number;
  text: string;
  observedBytes: number;
  retainedBytes: number;
  truncated: boolean;
  status: SummaryStatus;
}
export interface SummaryRequest {
  request: number;
  attempt: number;
  provider: string;
  api: string;
  model: string;
  status: SummaryStatus;
  truncated: boolean;
  parts: SummaryPart[];
}
export interface ReasoningSummary {
  v: 1;
  producer: string;
  turn: number;
  revision: number;
  activeRequest: { request: number; attempt: number } | null;
  truncated: boolean;
  requests: SummaryRequest[];
}

const object = (value: unknown): Record<string, unknown> | undefined => value !== null && typeof value === "object" ? value as Record<string, unknown> : undefined;
const index = (value: unknown): value is number => typeof value === "number" && Number.isSafeInteger(value) && value >= 0;
const id = (value: unknown): value is string => typeof value === "string" && value.length > 0 && Buffer.byteLength(value, "utf8") <= 128;
const display = (value: unknown, bytes: number): value is string => typeof value === "string" && value.length > 0 && Buffer.byteLength(value, "utf8") <= bytes;
const next = (value: number): number | undefined => Number.isSafeInteger(value) && value < Number.MAX_SAFE_INTEGER ? value + 1 : undefined;

/** Slice without decoding a cut UTF-8 codepoint or adding text not received from the provider. */
export function summaryPrefix(text: string, bytes: number): string {
  let used = 0, end = 0;
  for (const char of text) {
    const size = Buffer.byteLength(char, "utf8");
    if (used + size > bytes) break;
    used += size;
    end += char.length;
  }
  return text.slice(0, end);
}

/** Flush-time only: bounded full projection, optionally fitted to its actual transport envelope. */
export function fitReasoningSummary(source: ReasoningSummary, fits: (value: ReasoningSummary) => boolean = () => true): ReasoningSummary | undefined {
  const value: ReasoningSummary = {
    ...source,
    activeRequest: source.activeRequest ? { ...source.activeRequest } : null,
    requests: source.requests.map((request) => ({ ...request, parts: request.parts.map((part) => ({ ...part })) })),
  };
  const qualifies = () => {
    const json = JSON.stringify(value);
    // The receiver's Go JSON encoder also escapes HTML and Unicode separators.
    const receiverJson = json.replace(/[<>&\u2028\u2029]/g, (char) => `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}`);
    return Buffer.byteLength(receiverJson, "utf8") <= SUMMARY_PROJECTION_BYTES && fits(value);
  };
  const select = () => {
    if (!value.requests.some((r) => r.request === value.activeRequest?.request && r.attempt === value.activeRequest?.attempt)) value.activeRequest = null;
  };
  select();
  // Keep terminal facts and exact prefixes first; then omit trailing records/parts if metadata alone cannot fit.
  while (!qualifies()) {
    const request = [...value.requests].reverse().find((r) => r.parts.some((p) => p.text.length > 0));
    const part = request && [...request.parts].reverse().find((p) => p.text.length > 0);
    value.truncated = true;
    if (request && part) {
      part.text = summaryPrefix(part.text, Math.max(0, part.retainedBytes - Math.max(1, Math.ceil(part.retainedBytes / 4))));
      part.retainedBytes = Buffer.byteLength(part.text, "utf8");
      part.truncated = request.truncated = true;
    } else if (value.requests.length) {
      value.requests.pop();
      select();
    } else return undefined;
  }
  return value;
}

/** Structural 1.5.2-only gate: no new SDK type imports in the 0.84.4 development build. */
export function eligibleSummaryModel(version: unknown, context: unknown): { provider: string; api: string; model: string } | undefined {
  if (version !== "1.5.2") return undefined;
  const ctx = object(context), model = object(ctx?.model), registry = object(ctx?.modelRegistry);
  const provider = model?.provider, api = model?.api, name = model?.id;
  if (!((provider === "openai" && api === "openai-responses") || (provider === "openai-codex" && api === "openai-codex-responses")) || !display(name, 80 * 4) || [...name].length > 80) return undefined;
  try {
    const config = registry?.getRegisteredProviderConfig, native = registry?.getRegisteredNativeProvider;
    if (typeof config !== "function" || typeof native !== "function") return undefined;
    // Always execute BOTH public checks, even if the first reports an override.
    let configured: unknown, registered: unknown, failed = false;
    try { configured = config.call(registry, provider); } catch { failed = true; }
    try { registered = native.call(registry, provider); } catch { failed = true; }
    if (failed || configured !== undefined || registered !== undefined) return undefined;
  } catch { return undefined; }
  return { provider, api, model: name };
}

interface BoundResponse {
  id: string;
  record: SummaryRequest;
  sequence: number;
  items: Map<string, number>;
  blocked: boolean;
}

export function createReasoningSummaryObserver(version: unknown, producer = randomUUID()) {
  let alive = true, open = false, disabled = false;
  let turn = 0, revision = 0, request = 0, attempt = 1;
  let model: ReturnType<typeof eligibleSummaryModel>;
  let response: BoundResponse | undefined;
  let failedReservation = false;
  let truncated = false;
  let requests: SummaryRequest[] = [];
  // A bounded producer watermark: once exhausted, fail closed until session replacement.
  const retired = new Set<string>();
  const retiredItems = new Set<string>();
  const bump = () => { const n = next(revision); if (n === undefined) disabled = true; else revision = n; };
  const active = () => response && !response.blocked && response.record.status === "streaming" ? response.record : undefined;
  const terminal = (status: SummaryStatus) => {
    const record = active();
    if (!record) return;
    record.status = status;
    for (const part of record.parts) if (part.status === "streaming") part.status = status;
    bump();
  };
  const retire = () => {
    if (!response) return;
    retired.add(response.id);
    for (const item of response.items.keys()) retiredItems.add(item);
    if (retired.size >= 1024 || retiredItems.size >= 4096) disabled = true;
    response = undefined;
  };
  function raw(event: Record<string, unknown>): void {
    if (!model || event.provider !== model.provider || event.api !== model.api || event.model !== model.model) return;
    const data = object(event.data);
    if (!data) return;
    const type = data.type;
    if (type === "response.created") {
      const rid = object(data.response)?.id;
      if (!id(rid) || !index(data.sequence_number) || retired.has(rid)) return;
      if (response?.id === rid) return;
      if (response && response.record.status !== "failed") {
        // Unexplained overlap disables this reservation, rather than guessing which stream owns later deltas.
        response.blocked = true;
        bump();
        return;
      }
      if (response || failedReservation) {
        const n = next(attempt);
        if (n === undefined) { disabled = true; return; }
        attempt = n;
      }
      retire();
      if (disabled) return;
      failedReservation = false;
      if (requests.length >= SUMMARY_ATTEMPTS) { truncated = true; model = undefined; bump(); return; }
      const record: SummaryRequest = { request, attempt, ...model, status: "streaming", truncated: false, parts: [] };
      requests.push(record);
      response = { id: rid, record, sequence: data.sequence_number, items: new Map(), blocked: false };
      bump();
      return;
    }
    // Codex's explicit error event can precede response.created (missing-continuation recovery).
    // Copy no error fields. A response-less failed reservation never creates a history record.
    if (type === "error") {
      const errorResponse = object(data.response)?.id ?? data.response_id;
      if (errorResponse !== undefined && errorResponse !== response?.id) return;
      if (data.sequence_number !== undefined && !index(data.sequence_number)) return;
      if (response && data.sequence_number !== undefined && data.sequence_number <= response.sequence) return;
      if (response && data.sequence_number !== undefined) response.sequence = data.sequence_number as number;
      failedReservation = true;
      terminal("failed");
      return;
    }
    const bound = response;
    if (!bound || bound.blocked || bound.record.status !== "streaming" || !index(data.sequence_number) || data.sequence_number <= bound.sequence) return;
    const rid = object(data.response)?.id;
    if (rid !== undefined && rid !== bound.id) return;
    if (type === "response.failed" || type === "response.completed" || type === "response.done" || type === "response.incomplete") {
      if (rid !== bound.id) return;
      const status = object(data.response)?.status;
      const fact = type === "response.failed" ? "failed" : type === "response.incomplete" ? "incomplete" : status === "completed" ? "complete" : status === "incomplete" ? "incomplete" : status === "failed" ? "failed" : status === "cancelled" ? "aborted" : undefined;
      if (fact) { bound.sequence = data.sequence_number; terminal(fact); }
      return;
    }
    if (type === "response.output_item.added") {
      const item = object(data.item), iid = item?.id;
      if (item?.type !== "reasoning" || !id(iid) || !index(data.output_index) || retiredItems.has(iid)) return;
      const prior = bound.items.get(iid);
      if ((prior !== undefined && prior !== data.output_index) || [...bound.items].some(([key, output]) => output === data.output_index && key !== iid)) { bound.blocked = true; bump(); return; }
      if (bound.items.size >= SUMMARY_PARTS && prior === undefined) { truncated = bound.record.truncated = true; bump(); return; }
      bound.items.set(iid, data.output_index);
      bound.sequence = data.sequence_number;
      return;
    }
    const delta = type === "response.reasoning_summary_text.delta";
    const textDone = type === "response.reasoning_summary_text.done";
    const partAdded = type === "response.reasoning_summary_part.added";
    const partDone = type === "response.reasoning_summary_part.done";
    if (!delta && !textDone && !partAdded && !partDone) return;
    if (!id(data.item_id) || !index(data.output_index) || !index(data.summary_index) || bound.items.get(data.item_id) !== data.output_index) return;
    const rawPart = object(data.part);
    if ((partAdded || partDone) && rawPart?.type !== "summary_text") return;
    const text = delta ? data.delta : textDone ? data.text : rawPart?.text;
    if (typeof text !== "string") return;
    let part = bound.record.parts.find((p) => p.output === data.output_index && p.part === data.summary_index);
    if (partAdded && part) return;
    if (delta && part && part.status !== "streaming") return;
    bound.sequence = data.sequence_number;
    if (!part) {
      if (requests.reduce((sum, r) => sum + r.parts.length, 0) >= SUMMARY_PARTS) { truncated = bound.record.truncated = true; bump(); return; }
      part = { output: data.output_index, part: data.summary_index, text: "", observedBytes: 0, retainedBytes: 0, truncated: false, status: "streaming" };
      bound.record.parts.push(part);
      bound.record.parts.sort((a, b) => a.output - b.output || a.part - b.part);
    }
    const observed = (delta ? part.observedBytes : 0) + Buffer.byteLength(text, "utf8");
    if (!Number.isSafeInteger(observed)) { disabled = true; return; }
    const used = requests.reduce((sum, r) => sum + r.parts.reduce((n, p) => n + p.retainedBytes, 0), 0) - part.retainedBytes;
    // A clipped delta remains a prefix: never append later bytes after a gap.
    const retained = delta ? part.truncated ? part.text : summaryPrefix(part.text + summaryPrefix(text, SUMMARY_TEXT_BYTES), SUMMARY_TEXT_BYTES - used) : summaryPrefix(text, SUMMARY_TEXT_BYTES - used);
    part.text = retained;
    part.observedBytes = observed;
    part.retainedBytes = Buffer.byteLength(retained, "utf8");
    part.truncated = part.retainedBytes < observed;
    part.status = part.status === "incomplete" || (partDone && data.status === "incomplete") ? "incomplete" : partDone || textDone ? "complete" : "streaming";
    if (part.truncated) truncated = bound.record.truncated = true;
    bump();
  }
  return {
    observe(value: unknown, context?: unknown): void {
      if (!alive || disabled) return;
      const event = object(value);
      if (!event || event.agentId !== undefined) return;
      if (event.type === "agent_start") {
        if (open) return;
        const n = next(turn);
        if (n === undefined) { disabled = true; return; }
        turn = n; open = true; request = 0; attempt = 1; requests = []; truncated = false; model = undefined; retire();
      } else if (event.type === "before_provider_request") {
        if (!open) return;
        const n = next(request);
        if (n === undefined) { disabled = true; return; }
        request = n; attempt = 1; retire(); failedReservation = false;
        model = eligibleSummaryModel(version, context);
      } else if (event.type === "provider_stream_event") {
        if (open && request > 0) raw(event);
      } else if (event.type === "message_end" || event.type === "turn_end") {
        const message = object(event.message);
        if (!open || !model || message?.role !== "assistant" || message.responseId !== response?.id || message.provider !== model.provider || message.api !== model.api || message.model !== model.model) return;
        const status = message.stopReason === "aborted" ? "aborted" : message.stopReason === "error" ? "failed" : message.stopReason === "length" ? "incomplete" : message.stopReason === "stop" || message.stopReason === "toolUse" ? "complete" : undefined;
        if (status) terminal(status);
      } else if (event.type === "agent_settled") {
        if (!open) return;
        if (event.aborted === true) terminal("aborted");
        open = false; model = undefined;
        bump();
      }
    },
    projection(): ReasoningSummary | undefined {
      if (!alive || disabled || !requests.some((r) => r.parts.some((p) => p.observedBytes > 0))) return undefined;
      const selected = active();
      return fitReasoningSummary({ v: 1, producer, turn, revision, activeRequest: open && selected ? { request: selected.request, attempt: selected.attempt } : null, truncated, requests });
    },
    retire(): void { alive = false; model = undefined; response = undefined; requests = []; retired.clear(); retiredItems.clear(); },
  };
}
