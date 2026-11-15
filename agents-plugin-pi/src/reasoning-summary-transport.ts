/** Transport budgets include JSON escaping, actual RPC identity and LF, never just provider-text length. */
import { fitReasoningSummary, SUMMARY_FRAME_BYTES, type ReasoningSummary } from "./reasoning-summary.ts";

export function serializeMcpRequest(payload: { jsonrpc: "2.0"; id: number; method: string; params?: unknown }): string {
  const params = payload.params as { name?: unknown; arguments?: { events?: Array<Record<string, unknown>> } } | undefined;
  const events = params?.arguments?.events;
  if (payload.method !== "tools/call" || params?.name !== "dashboard.intake" || !Array.isArray(events) || !events.some((e) => e.reasoningSummary !== undefined)) return `${JSON.stringify(payload)}\n`;
  const summaryEvents = events.filter((e) => e.reasoningSummary !== undefined);
  if (summaryEvents.length !== 1 || events.length !== 1) throw new Error("ws-pi-bridge: summary requires one isolated intake event");
  const event = summaryEvents[0]!;
  const encode = (reasoningSummary: ReasoningSummary) => `${JSON.stringify({ ...payload, params: { ...params, arguments: { ...params.arguments, events: [{ ...event, reasoningSummary }] } } })}\n`;
  const fitted = fitReasoningSummary(event.reasoningSummary as ReasoningSummary, (value) => Buffer.byteLength(encode(value), "utf8") <= SUMMARY_FRAME_BYTES);
  if (!fitted) throw new Error("ws-pi-bridge: summary intake envelope exceeds frame budget");
  return encode(fitted);
}

// Exact 1.5.2 rpc-mode.js: setWidget allocates a UUID and omits undefined widgetPlacement.
// All UUIDs use the same ASCII length; this stand-in measures BOTH JSON encoding layers exactly.
export function hostedSummaryFrame(line: string): string {
  return `${JSON.stringify({ type: "extension_ui_request", id: "00000000-0000-4000-8000-000000000000", method: "setWidget", widgetKey: "ws-dashboard-events", widgetLines: [line] })}\n`;
}

export function hostedSummaryLine(summary: ReasoningSummary): string | undefined {
  const encode = (reasoningSummary: ReasoningSummary) => JSON.stringify({ kind: "agent", reasoningSummary });
  const fitted = fitReasoningSummary(summary, (value) => Buffer.byteLength(hostedSummaryFrame(encode(value)), "utf8") <= SUMMARY_FRAME_BYTES);
  return fitted ? encode(fitted) : undefined;
}
