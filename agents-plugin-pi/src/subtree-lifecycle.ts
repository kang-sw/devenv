/** Private, one-edge lifecycle transport. Pi's raw agent_settled is not vetoable. */
import { readFileSync } from "node:fs";
import type { RpcAgentRegistry } from "./spawner.ts";
import { SUBTREE_ENV } from "./delegation-policy.ts";
import { writePrivateJson } from "./fork-context.ts";

export interface SubtreeChannel { path: string; nonce: string }
export interface SubtreeSnapshot { nonce: string; outstanding: number; active: number; deliveries: number; delegated: boolean; revision: number }
export function readSubtreeChannel(env: NodeJS.ProcessEnv = process.env): SubtreeChannel | undefined {
  const raw = env[SUBTREE_ENV];
  if (!raw) return undefined;
  const value = JSON.parse(raw) as SubtreeChannel;
  if (!value || typeof value.path !== "string" || typeof value.nonce !== "string" || !value.path || !value.nonce) throw new Error("ws-pi-agent: malformed subtree channel");
  return value;
}
export function readSubtreeSnapshot(channel: SubtreeChannel): SubtreeSnapshot | undefined {
  try {
    const value = JSON.parse(readFileSync(channel.path, "utf8")) as SubtreeSnapshot;
    return value.nonce === channel.nonce && [value.outstanding, value.active, value.deliveries, value.revision].every(n => Number.isSafeInteger(n) && n >= 0) && typeof value.delegated === "boolean" ? value : undefined;
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
export function publishSubtree(registry: RpcAgentRegistry | undefined, dispatched = false): SubtreeSnapshot | undefined {
  if (!registry) return undefined;
  const p = publishers.get(registry);
  if (!p) return undefined;
  if (dispatched) { p.revision++; p.delegated = true; }
  let outstanding = 0, active = p.dispatching;
  for (const r of registry.values()) {
    if (r.expectedReport || r.waitingOnChildren) outstanding++;
    if (r.running || r.streaming || r.threadBound || r.lastWriter === "owner") active++;
  }
  p.delegated ||= registry.size > 0;
  const snapshot = { nonce: p.channel?.nonce ?? "local", outstanding, active, deliveries: p.deliveries(), delegated: p.delegated, revision: p.revision };
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
export function assertSubtreeFinal(registry: RpcAgentRegistry): number {
  const state = publishSubtree(registry);
  if (state && subtreeWaiting(state)) throw new Error("ws-pi-agent: final rejected while child results are outstanding; consume their reports, follow up or explicitly stop them, then submit a fresh final");
  return state?.revision ?? 0;
}
