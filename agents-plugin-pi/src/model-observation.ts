import { createHash } from "node:crypto";

/** Optional completed-response metadata; never RPC get_state or spawn settings. */
export interface ModelObservation {
  id: string;
  model: string;
  source: "pi.assistant";
  at: number;
  effort?: string;
  effortSource?: "message";
  outputTps?: number;
}

/** Same bounded transition window as the dashboard's discrete event queue. */
export const MODEL_OBSERVATION_CAP = 100;

const text = (value: unknown, cap: number): string | undefined => {
  if (typeof value !== "string") return undefined;
  const clean = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, "").trim().slice(0, cap);
  return clean || undefined;
};

function canonical(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, v]) => [key, canonical(v)]));
  return value;
}

/** Final message evidence with original time and deterministic replay identity.
 * Pi documents message.model as physical output; responseModel is preferred
 * when a provider records a more concrete identity. The fallback digest matches
 * the hosted Go parser's canonical JSON and stores no transcript content.
 */
export function modelObservation(message: unknown, sampleRate?: number): ModelObservation | undefined {
  if (!message || typeof message !== "object") return undefined;
  const m = message as Record<string, unknown>;
  if (m.role !== "assistant" || m.stopReason === "pending" || m.stopReason === "error" || m.stopReason === "aborted" || !Number.isSafeInteger(m.timestamp) || (m.timestamp as number) <= 0) return undefined;
  const model = text(m.responseModel, 80) ?? text(m.model, 80);
  if (!model) return undefined;
  const json = JSON.stringify(canonical(message)).replace(/[<>&\u2028\u2029]/g, (c) => `\\u${c.charCodeAt(0).toString(16).padStart(4, "0")}`);
  const id = text(m.responseId, 256) ?? `message:${m.timestamp}:${createHash("sha256").update(json).digest("hex")}`;
  const effort = text(m.thinkingLevel, 16);
  return { id, model, source: "pi.assistant", at: m.timestamp as number,
    ...(effort ? { effort, effortSource: "message" } : {}),
    ...(typeof sampleRate === "number" && Number.isFinite(sampleRate) && sampleRate > 0 && sampleRate <= 100_000 ? { outputTps: sampleRate } : {}),
  };
}

/** Validate only the additive metadata a descendant's owner supplied. */
export function parseModelObservation(raw: unknown): ModelObservation | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const v = raw as Record<string, unknown>;
  const id = text(v.id, 256), model = text(v.model, 80);
  if (!id || !model || v.source !== "pi.assistant" || !Number.isSafeInteger(v.at) || (v.at as number) <= 0) return undefined;
  const effort = v.effortSource === "message" ? text(v.effort, 16) : undefined;
  const rate = v.outputTps;
  return { id, model, source: "pi.assistant", at: v.at as number,
    ...(effort ? { effort, effortSource: "message" } : {}),
    ...(typeof rate === "number" && Number.isFinite(rate) && rate > 0 && rate <= 100_000 ? { outputTps: rate } : {}),
  };
}

/** First evidence wins; retries cannot move its timestamp or add a change. */
export function retainModelObservation(list: readonly ModelObservation[] | undefined, next: ModelObservation): ModelObservation[] {
  if (list?.some((o) => o.id === next.id && o.source === next.source)) return [...list];
  return [...(list ?? []), next].slice(-MODEL_OBSERVATION_CAP);
}

/** A resumed/forked lead restores its own branch, never pre-fork parent output. */
export function ownModelObservations(entries: readonly unknown[], header?: { parentSession?: unknown; timestamp?: unknown }): ModelObservation[] {
  const floor = header?.parentSession ? (typeof header.timestamp === "string" ? Date.parse(header.timestamp) : NaN) : 0;
  let out: ModelObservation[] = [];
  for (const raw of entries) {
    const entry = raw as { type?: unknown; message?: unknown } | undefined;
    if (entry?.type !== "message") continue;
    const o = modelObservation(entry.message);
    if (o && o.at >= floor) out = retainModelObservation(out, o);
  }
  return out;
}
