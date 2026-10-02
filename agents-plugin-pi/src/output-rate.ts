/**
 * Approximate visible-output decode rate (tokens per second) for one agent's
 * assistant stream. Pure, in-memory state: fed by receipt-time block
 * boundaries (`*_start` / `*_end` assistant message events) and finalized at
 * the assistant `message_end`, whose usage carries the token count.
 *
 * Only block spans count, summed per block, so request latency before the
 * first block, gaps between blocks, and tool execution between messages never
 * enter the denominator. When the provider reports a reasoning breakdown, the
 * reasoning tokens and thinking spans are both excluded (opaque reasoning time
 * cannot be paired with its tokens); otherwise every block span counts.
 *
 * Deltas are ignored here, so the per-delta path stays O(1): Pi awaits
 * extension handlers per delta, and a slow handler stalls streaming.
 */

/** Messages aggregated per agent; the rate is token-weighted over this window. */
export const OUTPUT_RATE_WINDOW = 8;
/** A message whose counted span is shorter is burst-flushed and would inflate the rate. */
export const OUTPUT_RATE_MIN_SPAN_MS = 250;
/** A message with fewer counted tokens is too small to measure. */
export const OUTPUT_RATE_MIN_TOKENS = 8;

export interface OutputRateTracker {
  /** Feed a `message_start` / `message_update` / `message_end` event; anything else is ignored. */
  observe(event: unknown, now?: number): void;
  /** Token-weighted tokens per second over the window, or `undefined` before the first eligible message. */
  rate(): number | undefined;
}

type BlockKind = "visible" | "thinking";
interface Sample { tokens: number; ms: number }

function blockOf(type: string): { kind: BlockKind; edge: "start" | "end" } | undefined {
  switch (type) {
    case "text_start": case "toolcall_start": return { kind: "visible", edge: "start" };
    case "text_end": case "toolcall_end": return { kind: "visible", edge: "end" };
    case "thinking_start": return { kind: "thinking", edge: "start" };
    case "thinking_end": return { kind: "thinking", edge: "end" };
    default: return undefined;
  }
}

export function createOutputRateTracker(): OutputRateTracker {
  // Current message: open blocks keyed by contentIndex, closed span sums per kind.
  let open = new Map<number, { kind: BlockKind; startedAt: number }>();
  let visibleMs = 0, thinkingMs = 0;
  let window: Sample[] = [];
  let windowModel: string | undefined;

  const resetMessage = () => { open = new Map(); visibleMs = 0; thinkingMs = 0; };

  const finish = (message: unknown) => {
    const m = message as { role?: unknown; stopReason?: unknown; provider?: unknown; model?: unknown; usage?: { output?: unknown; reasoning?: unknown } } | undefined;
    const openKinds = new Set([...open.values()].map((block) => block.kind));
    const visible = visibleMs, thinking = thinkingMs;
    resetMessage();
    if (m?.stopReason === "error" || m?.stopReason === "aborted") return;
    const output = m?.usage?.output;
    if (typeof output !== "number") return;
    const reasoning = m?.usage?.reasoning;
    const excludesReasoning = typeof reasoning === "number";
    // A counted block that never closed (e.g. an event lost through the RPC pipe) makes the span unknowable.
    if (openKinds.has("visible") || (!excludesReasoning && openKinds.has("thinking"))) return;
    const tokens = excludesReasoning ? output - reasoning : output;
    const ms = excludesReasoning ? visible : visible + thinking;
    if (ms < OUTPUT_RATE_MIN_SPAN_MS || tokens < OUTPUT_RATE_MIN_TOKENS) return;
    const model = `${String(m?.provider ?? "")}/${String(m?.model ?? "")}`;
    if (model !== windowModel) { window = []; windowModel = model; }
    window.push({ tokens, ms });
    if (window.length > OUTPUT_RATE_WINDOW) window.shift();
  };

  return {
    observe(event, now = Date.now()) {
      const e = event as { type?: unknown; message?: { role?: unknown }; assistantMessageEvent?: { type?: unknown; contentIndex?: unknown } } | undefined;
      if (e?.type === "message_start" || e?.type === "message_end") {
        if (e.message?.role !== "assistant") return;
        if (e.type === "message_start") resetMessage(); else finish(e.message);
        return;
      }
      if (e?.type !== "message_update") return;
      const inner = e.assistantMessageEvent;
      if (typeof inner?.type !== "string" || typeof inner.contentIndex !== "number") return;
      const block = blockOf(inner.type);
      if (!block) return;
      if (block.edge === "start") { open.set(inner.contentIndex, { kind: block.kind, startedAt: now }); return; }
      const started = open.get(inner.contentIndex);
      if (!started || started.kind !== block.kind) return;
      open.delete(inner.contentIndex);
      const span = Math.max(0, now - started.startedAt);
      if (block.kind === "visible") visibleMs += span; else thinkingMs += span;
    },
    rate() {
      if (window.length === 0) return undefined;
      let tokens = 0, ms = 0;
      for (const sample of window) { tokens += sample.tokens; ms += sample.ms; }
      return ms > 0 ? tokens / (ms / 1000) : undefined;
    },
  };
}

/** Integer rate, rounded to nearest, with the shared `t/s` unit. */
export function formatOutputRate(rate: number): string {
  return `${Math.round(rate)}t/s`;
}
