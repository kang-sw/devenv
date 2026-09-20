/** Private, one-edge lifecycle transport. Pi's raw agent_settled is not vetoable. */
import { readFileSync } from "node:fs";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";
import { SUBTREE_ENV } from "./delegation-policy.ts";
import { writePrivateJson } from "./fork-context.ts";

export interface SubtreeChannel { path: string; nonce: string }
export type SubtreeDescendantRole = "worker" | "execute" | "fork" | "explore";
export interface SubtreeDescendant {
  id: string;
  parentId: string | null;
  depth: number;
  role: SubtreeDescendantRole;
  live: boolean;
}
export interface SubtreeSnapshot {
  nonce: string;
  outstanding: number;
  active: number;
  deliveries: number;
  delegated: boolean;
  revision: number;
  /** Advisory display identity only; never an input to wait/settle. */
  descendants: SubtreeDescendant[];
}
export const MAX_SUBTREE_DESCENDANTS = 128;
export const MAX_SUBTREE_DESCENDANT_DEPTH = 8;
const DESCENDANT_ROLES = new Set<SubtreeDescendantRole>(["worker", "execute", "fork", "explore"]);
export function readSubtreeChannel(env: NodeJS.ProcessEnv = process.env): SubtreeChannel | undefined {
  const raw = env[SUBTREE_ENV];
  if (!raw) return undefined;
  const value = JSON.parse(raw) as SubtreeChannel;
  if (!value || typeof value.path !== "string" || typeof value.nonce !== "string" || !value.path || !value.nonce) throw new Error("ws-pi-agent: malformed subtree channel");
  return value;
}
export function readSubtreeSnapshot(channel: SubtreeChannel): SubtreeSnapshot | undefined {
  try {
    const value = JSON.parse(readFileSync(channel.path, "utf8")) as Omit<SubtreeSnapshot, "descendants"> & { descendants?: unknown };
    if (value.nonce !== channel.nonce || ![value.outstanding, value.active, value.deliveries, value.revision].every(n => Number.isSafeInteger(n) && n >= 0) || typeof value.delegated !== "boolean") return undefined;
    const descendants = Array.isArray(value.descendants)
      ? value.descendants.filter((row): row is SubtreeDescendant => {
        if (!row || typeof row !== "object") return false;
        const candidate = row as Partial<SubtreeDescendant>;
        return typeof candidate.id === "string" && candidate.id.length > 0 &&
          (candidate.parentId === null || (typeof candidate.parentId === "string" && candidate.parentId.length > 0)) &&
          Number.isSafeInteger(candidate.depth) && (candidate.depth as number) >= 0 && (candidate.depth as number) <= MAX_SUBTREE_DESCENDANT_DEPTH &&
          typeof candidate.role === "string" && DESCENDANT_ROLES.has(candidate.role as SubtreeDescendantRole) &&
          typeof candidate.live === "boolean";
      }).slice(0, MAX_SUBTREE_DESCENDANTS)
      : [];
    return { ...value, descendants };
  } catch { return undefined; }
}
export function subtreeWaiting(snapshot: SubtreeSnapshot | undefined): boolean {
  // Missing/mismatched publication is unknown, never permission to stop a process.
  return !snapshot || snapshot.outstanding > 0 || snapshot.active > 0 || snapshot.deliveries > 0;
}
export function writeSubtreeSnapshot(channel: SubtreeChannel, snapshot: Omit<SubtreeSnapshot, "nonce">): void {
  writePrivateJson(channel.path, { ...snapshot, nonce: channel.nonce });
}
interface Publisher { revision: number; delegated: boolean; dispatching: number; channel?: SubtreeChannel; deliveries: () => number }
const publishers = new WeakMap<RpcAgentRegistry, Publisher>();
export function installSubtreePublisher(registry: RpcAgentRegistry, channel: SubtreeChannel | undefined, deliveries: () => number): void {
  publishers.set(registry, { revision: 0, delegated: false, dispatching: 0, channel, deliveries });
  publishSubtree(registry);
}
export function subtreeOutstanding(registry: RpcAgentRegistry): number {
  let count = 0;
  for (const record of registry.values()) {
    if (record.waitingOnChildren || (record.terminalDelivery && record.terminalDelivery.state !== "enqueued")) count++;
  }
  return count;
}
function descendantRole(record: RpcAgentRecord): SubtreeDescendantRole {
  if (record.spawnRole === "execute-worker") return "execute";
  if (record.spawnRole === "fork") return "fork";
  if (record.spawnRole === "explore") return "explore";
  return "worker";
}

/** Builds a bounded, parent-before-child advisory tree. Count accounting is intentionally separate. */
function subtreeDescendants(registry: RpcAgentRegistry): SubtreeDescendant[] {
  const rows: SubtreeDescendant[] = [];
  const included = new Set<string>();
  for (const record of registry.values()) {
    if (rows.length >= MAX_SUBTREE_DESCENDANTS || included.has(record.agentId)) continue;
    rows.push({ id: record.agentId, parentId: null, depth: 0, role: descendantRole(record), live: record.client !== undefined });
    included.add(record.agentId);
    for (const nested of record.subtreeDescendants ?? []) {
      if (rows.length >= MAX_SUBTREE_DESCENDANTS) break;
      const depth = nested.depth + 1;
      const parentId = nested.parentId ?? record.agentId;
      if (depth > MAX_SUBTREE_DESCENDANT_DEPTH || !included.has(parentId) || included.has(nested.id)) continue;
      rows.push({ ...nested, parentId, depth });
      included.add(nested.id);
    }
  }
  return rows;
}

export function publishSubtree(registry: RpcAgentRegistry | undefined, dispatched = false): SubtreeSnapshot | undefined {
  if (!registry) return undefined;
  const p = publishers.get(registry);
  if (!p) return undefined;
  if (dispatched) { p.revision++; p.delegated = true; }
  const outstanding = subtreeOutstanding(registry);
  let active = p.dispatching;
  for (const r of registry.values()) {
    if (r.running || r.streaming) active++;
  }
  p.delegated ||= registry.size > 0;
  const snapshot = { nonce: p.channel?.nonce ?? "local", outstanding, active, deliveries: p.deliveries(), delegated: p.delegated, revision: p.revision, descendants: subtreeDescendants(registry) };
  if (p.channel) writeSubtreeSnapshot(p.channel, snapshot);
  return snapshot;
}
export function beginSubtreeDispatch(registry: RpcAgentRegistry): () => void {
  const publisher = publishers.get(registry);
  if (!publisher) return () => {};
  publisher.dispatching++;
  publishSubtree(registry);
  return () => { publisher.dispatching--; publishSubtree(registry); };
}
