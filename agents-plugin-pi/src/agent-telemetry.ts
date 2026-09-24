/** Read-only, durable child-session telemetry.  This deliberately does not
 * use SessionManager: opening a production history can migrate/write it. */
import { readFileSync } from "node:fs";
import { isDeepStrictEqual } from "node:util";

export interface TelemetryOrigin { sessionId: string; sessionPath: string; prefixEntryId?: string; emptyPrefix?: true }
/**
 * Bounded cumulative cost estimate: the footer's unit and the unit a hop
 * reports upward as its descendant usage. `unknownContributors` makes an
 * incomplete sum render as `+ ?`; `descendants` counts contributing agents.
 */
export interface CumulativeCost {
  knownUsd: number;
  knownContributors: number;
  unknownContributors: number;
  descendants: number;
}
export interface AgentTelemetry {
  version: 1;
  origin: TelemetryOrigin;
  model?: string;
  effort?: string;
  /** Current context-window occupancy. Pi's live ContextUsage.tokens overrides the latest-call usage fallback. */
  contextTokens?: number;
  /** Complete child-attributable cumulative estimate. Existing widget semantics read only this field. */
  estimatedUsd?: number;
  /** Known subtotal when one or more attributable usage entries have unknown cost. */
  partialEstimatedUsd?: number;
  /**
   * The child's last reported usage of everything below it, excluding its own
   * usage (which the fields above reduce from its session). Written only by
   * the parent, from the child's channel reports; never derived from the
   * child's session file.
   */
  descendantUsage?: CumulativeCost;
}
type Entry = { id: string; type: string; message?: { role?: string; usage?: unknown }; usage?: unknown };

const nonnegative = (v: unknown): number | undefined => typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : undefined;
const nonnegativeInteger = (v: unknown): number | undefined => { const n = nonnegative(v); return n !== undefined && Number.isSafeInteger(n) ? n : undefined; };
export function parseCumulativeCost(value: unknown): CumulativeCost | undefined {
  const cost = value as Partial<CumulativeCost> | null;
  if (!cost || typeof cost !== "object") return undefined;
  const knownUsd = nonnegative(cost.knownUsd);
  const knownContributors = nonnegativeInteger(cost.knownContributors);
  const unknownContributors = nonnegativeInteger(cost.unknownContributors);
  const descendants = nonnegativeInteger(cost.descendants);
  return knownUsd === undefined || knownContributors === undefined || unknownContributors === undefined || descendants === undefined
    ? undefined : { knownUsd, knownContributors, unknownContributors, descendants };
}
function usageOf(value: unknown): { contextTokens?: number; cost?: number } | undefined {
  if (!value || typeof value !== "object") return undefined;
  const u = value as { input?: unknown; output?: unknown; cacheRead?: unknown; cacheWrite?: unknown; totalTokens?: unknown; cost?: { total?: unknown } };
  const explicitTotal = nonnegative(u.totalTokens);
  const parts = [u.input, u.output, u.cacheRead, u.cacheWrite].map(nonnegative).filter((part): part is number => part !== undefined);
  const summed = parts.length > 0 ? parts.reduce((total, part) => total + part, 0) : undefined;
  const contextTokens = explicitTotal ?? (summed !== undefined && Number.isFinite(summed) ? summed : undefined);
  const cost = nonnegative(u.cost?.total);
  return contextTokens === undefined && cost === undefined ? undefined : { contextTokens, cost };
}
export function parseTelemetry(value: unknown): AgentTelemetry | undefined {
  const t = value as Partial<AgentTelemetry> | null;
  if (!t || t.version !== 1 || !t.origin || typeof t.origin.sessionId !== "string" || !t.origin.sessionId || typeof t.origin.sessionPath !== "string" || !t.origin.sessionPath) return undefined;
  const o = t.origin;
  if (o.prefixEntryId !== undefined && (typeof o.prefixEntryId !== "string" || !o.prefixEntryId)) return undefined;
  if (!o.emptyPrefix && !o.prefixEntryId) return undefined;
  const out: AgentTelemetry = { version: 1, origin: { sessionId: o.sessionId, sessionPath: o.sessionPath, ...(o.prefixEntryId ? { prefixEntryId: o.prefixEntryId } : { emptyPrefix: true }) } };
  if (typeof t.model === "string" && t.model) out.model = t.model;
  if (typeof t.effort === "string" && t.effort) out.effort = t.effort;
  for (const k of ["contextTokens", "estimatedUsd", "partialEstimatedUsd"] as const) { const n = nonnegative(t[k]); if (n !== undefined) out[k] = n; }
  if (out.estimatedUsd !== undefined && out.partialEstimatedUsd !== undefined) delete out.partialEstimatedUsd;
  const descendantUsage = parseCumulativeCost(t.descendantUsage);
  if (descendantUsage) out.descendantUsage = descendantUsage;
  return out;
}
export function readSessionEntries(path: string): { headerId: string; parentSession?: string; entries: Entry[] } | { transient: true } | undefined {
  // Missing or unreachable files do not contradict an already validated origin.
  let raw: string; try { raw = readFileSync(path, "utf8"); } catch { return { transient: true }; }
  const lines = raw.split("\n"); if (lines.at(-1) === "") lines.pop();
  const parsed: unknown[] = [];
  for (let i = 0; i < lines.length; i++) { try { parsed.push(JSON.parse(lines[i])); } catch { return i === lines.length - 1 ? { transient: true } : undefined; } }
  const h = parsed.shift() as { type?: unknown; version?: unknown; id?: unknown; parentSession?: unknown } | undefined;
  if (!h || h.type !== "session" || h.version !== 3 || typeof h.id !== "string" || !h.id) return undefined;
  const entries: Entry[] = [];
  const byId = new Map<string, Entry>();
  for (const raw of parsed) {
    if (!raw || typeof raw !== "object" || typeof (raw as Entry).id !== "string" || !(raw as Entry).id) return undefined;
    const e = raw as Entry, prior = byId.get(e.id);
    if (prior) { if (!isDeepStrictEqual(prior, e)) return undefined; continue; }
    byId.set(e.id, e); entries.push(e);
  }
  return { headerId: h.id, ...(typeof h.parentSession === "string" && h.parentSession ? { parentSession: h.parentSession } : {}), entries };
}
/** Recomputes, never adds. Unavailable or invalid input returns undefined; callers classify the read before choosing fallback. */
export function reduceTelemetry(origin: TelemetryOrigin, read: ReturnType<typeof readSessionEntries>): Pick<AgentTelemetry, "contextTokens" | "estimatedUsd" | "partialEstimatedUsd"> | undefined {
  if (!read || "transient" in read || read.headerId !== origin.sessionId) return undefined;
  let start = 0;
  if (origin.prefixEntryId) { const at = read.entries.findIndex(e => e.id === origin.prefixEntryId); if (at < 0) return undefined; start = at + 1; }
  else if (!origin.emptyPrefix) return undefined;
  let total = 0, observedCost = false, invalidCost = false, contextTokens: number | undefined;
  for (const e of read.entries.slice(start)) {
    const assistant = e.type === "message" && e.message?.role === "assistant";
    const summary = e.type === "compaction" || e.type === "branch_summary";
    const rawUsage = e.message?.usage ?? e.usage;
    const usage = usageOf(rawUsage);
    if (!usage) { if (assistant || (rawUsage !== undefined && (summary || e.type === "message"))) invalidCost = true; if (assistant || summary) contextTokens = undefined; continue; }
    if (assistant) contextTokens = usage.contextTokens; // a later summary makes pre-compaction occupancy unknown.
    if (summary) contextTokens = undefined;
    if (usage.cost === undefined) invalidCost = true; else { observedCost = true; total += usage.cost; }
  }
  return {
    ...(contextTokens !== undefined ? { contextTokens } : {}),
    ...(!invalidCost && observedCost ? { estimatedUsd: total } : invalidCost && observedCost ? { partialEstimatedUsd: total } : {}),
  };
}
export function refreshTelemetry(snapshot: AgentTelemetry): AgentTelemetry | undefined {
  const reduced = reduceTelemetry(snapshot.origin, readSessionEntries(snapshot.origin.sessionPath));
  if (reduced === undefined) return undefined;
  const next = { ...snapshot };
  // Context is a last-valid snapshot: a compaction or provider gap makes the
  // current value unknown, but must not replace a valid same-session value
  // with a false near-zero fallback. Cost remains a full recomputation.
  delete next.estimatedUsd; delete next.partialEstimatedUsd;
  return { ...next, ...reduced };
}
