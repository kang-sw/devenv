/** Read-only, durable child-session telemetry.  This deliberately does not
 * use SessionManager: opening a production history can migrate/write it. */
import { existsSync, readFileSync } from "node:fs";
import { isDeepStrictEqual } from "node:util";

export interface TelemetryOrigin { sessionId: string; sessionPath: string; prefixEntryId?: string; emptyPrefix?: true }
export interface AgentTelemetry { version: 1; origin: TelemetryOrigin; model?: string; effort?: string; latestInput?: number; estimatedUsd?: number }
type Entry = { id: string; type: string; message?: { role?: string; usage?: unknown }; usage?: unknown };

const nonnegative = (v: unknown): number | undefined => typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : undefined;
function usageOf(value: unknown): { input?: number; cost?: number } | undefined {
  if (!value || typeof value !== "object") return undefined;
  const u = value as { input?: unknown; cost?: { total?: unknown } };
  const input = nonnegative(u.input), cost = nonnegative(u.cost?.total);
  return input === undefined && cost === undefined ? undefined : { input, cost };
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
  for (const k of ["latestInput", "estimatedUsd"] as const) { const n = nonnegative(t[k]); if (n !== undefined) out[k] = n; }
  return out;
}
export function readSessionEntries(path: string): { headerId: string; parentSession?: string; entries: Entry[] } | { transient: true } | undefined {
  if (!existsSync(path)) return undefined;
  let raw: string; try { raw = readFileSync(path, "utf8"); } catch { return undefined; }
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
/** Recomputes, never adds. Invalid complete input returns undefined so callers retain a stale valid snapshot. */
export function reduceTelemetry(origin: TelemetryOrigin, read: ReturnType<typeof readSessionEntries>): Pick<AgentTelemetry, "latestInput" | "estimatedUsd"> | undefined {
  if (!read || "transient" in read || read.headerId !== origin.sessionId) return undefined;
  let start = 0;
  if (origin.prefixEntryId) { const at = read.entries.findIndex(e => e.id === origin.prefixEntryId); if (at < 0) return undefined; start = at + 1; }
  else if (!origin.emptyPrefix) return undefined;
  let total = 0, observedCost = false, invalidCost = false, latest: number | undefined;
  for (const e of read.entries.slice(start)) {
    const assistant = e.type === "message" && e.message?.role === "assistant";
    const usage = usageOf(e.message?.usage ?? e.usage);
    if (!usage) { if (assistant) { invalidCost = true; latest = undefined; } continue; }
    if (assistant) latest = usage.input; // later summary entries below clear this.
    if (e.type === "compaction" || e.type === "branch_summary") latest = undefined;
    if (usage.cost === undefined) invalidCost = true; else { observedCost = true; total += usage.cost; }
  }
  return { ...(latest !== undefined ? { latestInput: latest } : {}), ...(!invalidCost && observedCost ? { estimatedUsd: total } : {}) };
}
export function refreshTelemetry(snapshot: AgentTelemetry): AgentTelemetry | undefined {
  const reduced = reduceTelemetry(snapshot.origin, readSessionEntries(snapshot.origin.sessionPath));
  if (reduced === undefined) return undefined;
  const next = { ...snapshot };
  delete next.latestInput; delete next.estimatedUsd;
  return { ...next, ...reduced };
}
