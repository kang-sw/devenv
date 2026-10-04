/** Shared direct-child display projection. No lifecycle decision consumes this data. */
import type { RpcAgentRecord } from "./spawner.ts";

export type AgentRowState = "awaiting-owner" | "idle-awaiting-owner" | "awaiting-approval" | "waiting-on-children" | "pending-delivery" | "running";

/** Owner-provided epoch-ms facts let every ancestor advance clocks without traffic. */
export interface AgentDisplayDetail {
  name: string;
  /** null means the owner has no classified visible row (not implicit execution). */
  state: AgentRowState | null;
  runStartedAt: number;
  settledAt?: number;
  lastOutputAt: number;
  model?: string;
  effort?: string;
  contextTokens?: number;
  estimatedUsd?: number;
  outputTps?: number;
}

export function rowName(record: RpcAgentRecord): string {
  return record.alias ?? record.title ?? record.agentId.slice(0, 8);
}

export function classifyRegistryRowState(record: RpcAgentRecord): AgentRowState | undefined {
  if (record.threadBound === true) return "awaiting-owner";
  if (record.pendingApproval !== undefined) return "awaiting-approval";
  if (record.lastWriter === "owner" && !record.running && !record.streaming) return "idle-awaiting-owner";
  if (record.running || record.streaming) return "running";
  if (record.waitingOnChildren) return "waiting-on-children";
  if (record.terminalDelivery && record.terminalDelivery.state !== "enqueued") return "pending-delivery";
  return undefined;
}

/** Mirrors the local gutter's own attribution and omission semantics. */
export function agentDisplayDetail(record: RpcAgentRecord): AgentDisplayDetail | undefined {
  if (!finiteNonnegative(record.runStartedAt)) return undefined;
  return parseAgentDisplayDetail({
    name: rowName(record), state: classifyRegistryRowState(record) ?? null,
    runStartedAt: record.runStartedAt, settledAt: record.settledAt, lastOutputAt: record.lastOutputAt ?? 0,
    model: record.telemetry?.model ?? record.observedModel,
    effort: record.telemetry?.effort ?? record.observedEffort,
    contextTokens: record.telemetry?.contextTokens ?? record.observedContextTokens,
    estimatedUsd: record.telemetry?.estimatedUsd, outputTps: record.outputRate?.rate(),
  });
}

function finiteNonnegative(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value >= 0;
}
function text(value: unknown, max: number): string | undefined {
  if (typeof value !== "string") return undefined;
  const clean = value.replace(/[\u0000-\u001f\u007f-\u009f]/g, "").trim().slice(0, max);
  return clean || undefined;
}
const states = new Set<AgentRowState>(["awaiting-owner", "idle-awaiting-owner", "awaiting-approval", "waiting-on-children", "pending-delivery", "running"]);

/** Malformed optional display never compromises the surrounding authoritative snapshot. */
export function parseAgentDisplayDetail(raw: unknown): AgentDisplayDetail | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const v = raw as Record<string, unknown>;
  const name = text(v.name, 256);
  if (!name || !(v.state === null || states.has(v.state as AgentRowState)) || !finiteNonnegative(v.runStartedAt) || !finiteNonnegative(v.lastOutputAt)) return undefined;
  const result: AgentDisplayDetail = { name, state: v.state as AgentRowState | null, runStartedAt: v.runStartedAt, lastOutputAt: v.lastOutputAt };
  if (finiteNonnegative(v.settledAt)) result.settledAt = v.settledAt;
  const model = text(v.model, 256), effort = text(v.effort, 64);
  if (model) result.model = model;
  if (effort) result.effort = effort;
  for (const key of ["contextTokens", "estimatedUsd", "outputTps"] as const) if (finiteNonnegative(v[key])) result[key] = v[key];
  return result;
}
