/** Durable, Pi-local storage for delegated-agent material. */
import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { mkdirSync, lstatSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname, join, resolve, relative, sep } from "node:path";

export const OWNERSHIP_VERSION = 1;
const SAFE_COMPONENT = /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export interface AgentStorageContext { root: string; ownerSessionId: string; }
export interface AgentOwnership {
  version: number; ownerSessionId: string; agentId: string; home: string;
  role: "worker" | "execute-worker" | "fork" | "explore"; exploreMode?: "simple" | "deep"; sessionPath?: string;
}
export interface OwnershipMetadata extends AgentOwnership {
  createdAt: number; lastActivityAt: number; updatedAt: number;
  liveness: { lifecycle: "starting" | "live" | "stopping" | "stopped" | "unknown"; running?: boolean; observedAt?: number; pid?: number; instanceNonce?: string; threadBound?: boolean; pendingQuestion?: boolean; pendingApprovalCommandId?: string; recovery?: "none" | "sidecar" | "thread" | "revived" };
}

function safe(value: string, label: string): string { if (!SAFE_COMPONENT.test(value)) throw new Error(`ws-pi-agent: unsafe ${label}`); return value; }
function contained(parent: string, child: string): boolean { const r = relative(parent, child); return r === "" || (!!r && !r.startsWith(`..${sep}`) && r !== ".."); }
function canonicalRoot(root: string): string { mkdirSync(root, { recursive: true, mode: 0o700 }); return resolve(root); }

export function createAgentStorageContext(sessionId: string, agentDir = getAgentDir()): AgentStorageContext {
  return { root: canonicalRoot(agentDir), ownerSessionId: safe(sessionId, "Pi session id") };
}
export function ownershipPath(home: string): string { return join(home, "ownership.json"); }
export function allocateAgentHome(ctx: AgentStorageContext, agentId: string, role: AgentOwnership["role"], exploreMode?: "simple" | "deep", noSession = false): AgentOwnership {
  safe(agentId, "agent id");
  const ownerRoot = join(ctx.root, "ws-agents", safe(ctx.ownerSessionId, "Pi session id"));
  const home = resolve(ownerRoot, agentId);
  if (!contained(ownerRoot, home)) throw new Error("ws-pi-agent: agent home escapes configured Pi directory");
  mkdirSync(home, { recursive: true, mode: 0o700 });
  if (lstatSync(home).isSymbolicLink()) throw new Error("ws-pi-agent: agent home must not be a symlink");
  const sessionPath = noSession ? undefined : join(home, "session.jsonl");
  const now = Date.now();
  const ownership: AgentOwnership = { version: OWNERSHIP_VERSION, ownerSessionId: ctx.ownerSessionId, agentId, home, role, ...(exploreMode ? { exploreMode } : {}), ...(sessionPath ? { sessionPath } : {}) };
  writeOwnership({ ...ownership, createdAt: now, lastActivityAt: now, updatedAt: now, liveness: { lifecycle: "starting", observedAt: now } });
  return ownership;
}
export function writeOwnership(metadata: OwnershipMetadata): void {
  const home = resolve(metadata.home); mkdirSync(home, { recursive: true, mode: 0o700 });
  const target = ownershipPath(home), temp = join(home, `.ownership-${process.pid}-${Date.now()}.tmp`);
  writeFileSync(temp, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 }); renameSync(temp, target);
}
export function readOwnership(home: string): OwnershipMetadata | undefined {
  try { const value = JSON.parse(readFileSync(ownershipPath(home), "utf8")) as OwnershipMetadata; return validOwnership(value) ? value : undefined; } catch { return undefined; }
}
export function validOwnership(value: unknown): value is OwnershipMetadata {
  const o = value as Partial<OwnershipMetadata> | null;
  return !!o && o.version === OWNERSHIP_VERSION && typeof o.ownerSessionId === "string" && SAFE_COMPONENT.test(o.ownerSessionId) && typeof o.agentId === "string" && SAFE_COMPONENT.test(o.agentId) && typeof o.home === "string" && typeof o.role === "string" && Number.isFinite(o.createdAt) && Number.isFinite(o.lastActivityAt) && Number.isFinite(o.updatedAt) && !!o.liveness && typeof o.liveness.lifecycle === "string";
}
export function updateOwnership(home: string, update: Partial<Pick<OwnershipMetadata, "lastActivityAt" | "liveness">>): OwnershipMetadata | undefined {
  const current = readOwnership(home); if (!current) return undefined;
  const now = Date.now(); const next = { ...current, ...update, lastActivityAt: Math.max(current.lastActivityAt, update.lastActivityAt ?? current.lastActivityAt), updatedAt: now };
  writeOwnership(next); return next;
}
export function touchOwnership(home: string): void { updateOwnership(home, { lastActivityAt: Date.now() }); }
