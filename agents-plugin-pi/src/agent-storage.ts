/** Durable, Pi-local storage for delegated-agent material. */
import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, renameSync, rmSync, rmdirSync, statSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve, relative, sep } from "node:path";
import { randomUUID } from "node:crypto";
import { parseDelegationPolicy, type DelegationPolicy } from "./delegation-policy.ts";

export const OWNERSHIP_VERSION = 1;
const SAFE_COMPONENT = /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export interface AgentStorageContext { root: string; ownerSessionId: string; }
export interface AgentOwnership {
  version: number; ownerSessionId: string; agentId: string; home: string; delegation?: DelegationPolicy;
  role: "worker" | "execute-worker" | "fork" | "explore"; exploreMode?: "simple" | "deep"; sessionPath?: string;
}
export interface OwnershipMetadata extends AgentOwnership {
  createdAt: number; lastActivityAt: number; updatedAt: number;
  liveness: { lifecycle: "starting" | "live" | "stopping" | "stopped" | "unknown"; running?: boolean; observedAt?: number; pid?: number; instanceNonce?: string; threadBound?: boolean; ownerHeld?: boolean; pendingQuestion?: boolean; waitingOnChildren?: boolean; expectedReport?: boolean; pendingApprovalCommandId?: string; recovery?: "none" | "sidecar" | "thread" | "revived" };
  sessionSignature?: { mtimeMs: number; size: number };
}

function safe(value: string, label: string): string { if (!SAFE_COMPONENT.test(value)) throw new Error(`ws-pi-agent: unsafe ${label}`); return value; }
function contained(parent: string, child: string): boolean { const r = relative(parent, child); return r === "" || (!!r && !r.startsWith(`..${sep}`) && r !== ".."); }
function canonicalRoot(root: string): string { mkdirSync(root, { recursive: true, mode: 0o700 }); return realpathSync(root); }
function checkedDirectory(path: string, root: string): string { mkdirSync(path, { recursive: true, mode: 0o700 }); const real = realpathSync(path); if (!contained(root, real) || lstatSync(path).isSymbolicLink()) throw new Error("ws-pi-agent: owned path contains a symlink escape"); return real; }
function canonicalHome(home: string): string { const resolved = resolve(home); if (lstatSync(resolved).isSymbolicLink()) throw new Error("ws-pi-agent: owned home is symlinked"); const real = realpathSync(resolved); if (real !== resolved) throw new Error("ws-pi-agent: owned home escapes through symlink"); return real; }
function checkedSessionPath(home: string, sessionPath: string): void {
  const candidate = resolve(sessionPath);
  if (candidate !== sessionPath || candidate === home || !contained(home, candidate)) throw new Error("ws-pi-agent: session path escapes owned home");
  const parent = dirname(candidate);
  if (realpathSync(parent) !== parent) throw new Error("ws-pi-agent: session path escapes owned home through a symlink");
  const entry = lstatSync(candidate, { throwIfNoEntry: false });
  if (entry && (!entry.isFile() || realpathSync(candidate) !== candidate)) throw new Error("ws-pi-agent: owned session path must be a regular file without symlinks");
}
export function isOwnedSessionPath(home: string, sessionPath: string): boolean { try { checkedSessionPath(canonicalHome(home), sessionPath); return true; } catch { return false; } }

export function createAgentStorageContext(sessionId: string, agentDir = getAgentDir()): AgentStorageContext {
  return { root: canonicalRoot(agentDir), ownerSessionId: safe(sessionId, "Pi session id") };
}
export function ownershipPath(home: string): string { return join(home, "ownership.json"); }
export function allocateAgentHome(ctx: AgentStorageContext, agentId: string, role: AgentOwnership["role"], exploreMode?: "simple" | "deep", noSession = false): AgentOwnership {
  safe(agentId, "agent id");
  const namespace = checkedDirectory(join(ctx.root, "ws-agents"), ctx.root);
  const ownerRoot = checkedDirectory(join(namespace, safe(ctx.ownerSessionId, "Pi session id")), ctx.root);
  const home = resolve(ownerRoot, agentId);
  if (!contained(ownerRoot, home)) throw new Error("ws-pi-agent: agent home escapes configured Pi directory");
  checkedDirectory(home, ownerRoot);
  const sessionPath = noSession ? undefined : join(home, "session.jsonl");
  const now = Date.now();
  const ownership: AgentOwnership = { version: OWNERSHIP_VERSION, ownerSessionId: ctx.ownerSessionId, agentId, home, role, ...(exploreMode ? { exploreMode } : {}), ...(sessionPath ? { sessionPath } : {}) };
  writeOwnership({ ...ownership, createdAt: now, lastActivityAt: now, updatedAt: now, liveness: { lifecycle: "starting", observedAt: now, pid: process.pid, instanceNonce: randomUUID(), recovery: "none" } });
  return ownership;
}
export function writeOwnership(metadata: OwnershipMetadata): void {
  const home = canonicalHome(metadata.home); if (metadata.home !== home) throw new Error("ws-pi-agent: ownership home is not canonical"); if (metadata.sessionPath) checkedSessionPath(home, metadata.sessionPath);
  const target = ownershipPath(home), temp = join(home, `.ownership-${process.pid}-${Date.now()}.tmp`);
  writeFileSync(temp, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 }); renameSync(temp, target);
}
export function readOwnership(home: string): OwnershipMetadata | undefined {
  try { const canonical = canonicalHome(home); const value = JSON.parse(readFileSync(ownershipPath(canonical), "utf8")) as OwnershipMetadata; return validOwnership(value) && value.home === canonical && (!value.sessionPath || (checkedSessionPath(canonical, value.sessionPath), true)) ? value : undefined; } catch { return undefined; }
}
export function validOwnership(value: unknown): value is OwnershipMetadata {
  const o = value as Partial<OwnershipMetadata> | null;
  const l = o?.liveness;
  const signature = o?.sessionSignature;
  return validDescriptor(value) && Number.isFinite(o.createdAt) && Number.isFinite(o.lastActivityAt) && Number.isFinite(o.updatedAt) && !!l && ["starting","live","stopping","stopped","unknown"].includes(l.lifecycle as string) &&
    (l.running === undefined || typeof l.running === "boolean") && (l.observedAt === undefined || Number.isFinite(l.observedAt)) &&
    (l.pid === undefined || (Number.isInteger(l.pid) && l.pid > 0)) && (l.instanceNonce === undefined || SAFE_COMPONENT.test(l.instanceNonce)) &&
    (l.threadBound === undefined || typeof l.threadBound === "boolean") && (l.ownerHeld === undefined || typeof l.ownerHeld === "boolean") &&
    (l.pendingQuestion === undefined || typeof l.pendingQuestion === "boolean") &&
    (l.waitingOnChildren === undefined || typeof l.waitingOnChildren === "boolean") && (l.expectedReport === undefined || typeof l.expectedReport === "boolean") &&
    (l.pendingApprovalCommandId === undefined || SAFE_COMPONENT.test(l.pendingApprovalCommandId)) &&
    (l.recovery === undefined || ["none","sidecar","thread","revived"].includes(l.recovery)) &&
    (signature === undefined || (Number.isFinite(signature.mtimeMs) && Number.isFinite(signature.size) && signature.size >= 0));
}
export function validDescriptor(value: unknown): value is AgentOwnership { const o = value as Partial<AgentOwnership> | null; return !!o && o.version === OWNERSHIP_VERSION && typeof o.ownerSessionId === "string" && SAFE_COMPONENT.test(o.ownerSessionId) && typeof o.agentId === "string" && SAFE_COMPONENT.test(o.agentId) && typeof o.home === "string" && ["worker","execute-worker","fork","explore"].includes(o.role as string) && (o.sessionPath === undefined || typeof o.sessionPath === "string") && validDelegationDescriptor(o.delegation); }
function validDelegationDescriptor(value: unknown): boolean {
  if (value === undefined) return true;
  try { parseDelegationPolicy(value); return true; } catch { return false; }
}
export function updateOwnership(home: string, update: Partial<Pick<OwnershipMetadata, "lastActivityAt" | "liveness" | "delegation">>): OwnershipMetadata | undefined {
  const current = readOwnership(home); if (!current) return undefined;
  const now = Date.now(); const next = { ...current, ...update, liveness: { ...current.liveness, ...update.liveness }, lastActivityAt: Math.max(current.lastActivityAt, update.lastActivityAt ?? current.lastActivityAt), updatedAt: now };
  try { writeOwnership(next); return next; } catch { return undefined; }
}
export function touchOwnership(home: string): void { updateOwnership(home, { lastActivityAt: Date.now() }); }

export type OwnedHomeRemovalResult =
  | { status: "eligible"; metadata: OwnershipMetadata }
  | { status: "retained"; reason: string }
  | { status: "deleted" }
  | { status: "failed"; error: string };

function sameOwnershipIdentity(expected: AgentOwnership, actual: OwnershipMetadata): boolean {
  return expected.version === actual.version && expected.ownerSessionId === actual.ownerSessionId &&
    expected.agentId === actual.agentId && expected.home === actual.home && expected.role === actual.role &&
    expected.exploreMode === actual.exploreMode && expected.sessionPath === actual.sessionPath;
}

function hasOnlyContainedRegularEntries(path: string, home: string): boolean {
  for (const entry of readdirSync(path, { withFileTypes: true })) {
    const child = join(path, entry.name);
    const stat = lstatSync(child);
    if (stat.isSymbolicLink()) return false;
    const real = realpathSync(child);
    if (!contained(home, real) || real !== child) return false;
    if (stat.isDirectory()) {
      if (!hasOnlyContainedRegularEntries(child, home)) return false;
    } else if (!stat.isFile()) {
      return false;
    }
  }
  return true;
}

/** Rechecks durable ownership and protection without treating uncertainty as eligibility. */
export function inspectOwnedHomeRemoval(ownership: AgentOwnership): OwnedHomeRemovalResult {
  try {
    const home = canonicalHome(ownership.home);
    const ownerRoot = dirname(home);
    const namespace = dirname(ownerRoot);
    if (home !== ownership.home || basename(home) !== ownership.agentId || basename(ownerRoot) !== ownership.ownerSessionId || basename(namespace) !== "ws-agents") {
      return { status: "retained", reason: "owned-home identity does not match its canonical locator" };
    }
    if ([ownerRoot, namespace].some(path => lstatSync(path).isSymbolicLink() || realpathSync(path) !== path)) {
      return { status: "retained", reason: "owned-home ancestry is symlinked" };
    }
    const metadata = readOwnership(home);
    if (!metadata || !sameOwnershipIdentity(ownership, metadata)) return { status: "retained", reason: "ownership metadata is missing, unreadable, or mismatched" };
    const liveness = metadata.liveness;
    if (liveness.lifecycle !== "stopped" || liveness.running !== false) return { status: "retained", reason: "child liveness is not confirmed stopped" };
    if (liveness.threadBound || liveness.ownerHeld || liveness.pendingQuestion || liveness.waitingOnChildren || liveness.expectedReport || liveness.pendingApprovalCommandId) {
      return { status: "retained", reason: "child has a protected owner, question, approval, or report wait" };
    }
    if (!hasOnlyContainedRegularEntries(home, home)) return { status: "retained", reason: "owned home contains a symlink or non-regular entry" };
    return { status: "eligible", metadata };
  } catch {
    return { status: "retained", reason: "owned-home eligibility could not be established" };
  }
}

function removeTreeEntry(path: string): void {
  const stat = lstatSync(path);
  if (!stat.isDirectory()) { rmSync(path, { force: false }); return; }
  for (const entry of readdirSync(path)) removeTreeEntry(join(path, entry));
  rmdirSync(path);
}

function removeOwnedHomeTree(home: string): void {
  const metadataPath = ownershipPath(home);
  // Ownership is the retry authorization. Remove it only after every payload
  // entry succeeds, then remove the now-empty home itself.
  for (const entry of readdirSync(home)) {
    const path = join(home, entry);
    if (path !== metadataPath) removeTreeEntry(path);
  }
  rmSync(metadataPath, { force: false });
  rmdirSync(home);
}

/** Best-effort exact-home removal. Eligibility is checked again immediately before deletion. */
export function removeOwnedAgentHome(
  ownership: AgentOwnership,
  remove: (path: string) => void = removeOwnedHomeTree,
): OwnedHomeRemovalResult {
  const first = inspectOwnedHomeRemoval(ownership);
  if (first.status !== "eligible") return first;
  const final = inspectOwnedHomeRemoval(ownership);
  if (final.status !== "eligible") return final;
  try {
    remove(ownership.home);
    try { rmdirSync(dirname(ownership.home)); } catch { /* another child or sidecar still owns the lead subtree */ }
    return { status: "deleted" };
  } catch (error) {
    // A late failure after metadata removal (for example a concurrent writer
    // racing the final rmdir) gets one best-effort authorization restore.
    if (existsSync(ownership.home) && !readOwnership(ownership.home)) {
      try { writeOwnership(final.metadata); } catch { /* original failure remains the diagnostic */ }
    }
    const message = String(error);
    console.error(`ws-pi-agent: could not remove owned agent home: ${message}`);
    return { status: "failed", error: message };
  }
}

/** Samples the actual session file. A stat failure records no invented write/activity. */
export function observeSessionWrite(home: string, sessionPath: string): void {
  const current = readOwnership(home); if (!current) return;
  try {
    let stat;
    try { stat = statSync(sessionPath); } catch (error) {
      // Pi writes a new session lazily. Absence before the first observed write
      // is pending observation, while disappearance of known history is not.
      if ((error as NodeJS.ErrnoException).code === "ENOENT" && !current.sessionSignature && sessionPath === current.sessionPath) return;
      throw error;
    }
    const signature = { mtimeMs: stat.mtimeMs, size: stat.size };
    const changed = !current.sessionSignature || current.sessionSignature.mtimeMs !== signature.mtimeMs || current.sessionSignature.size !== signature.size;
    const now = Date.now();
    writeOwnership({ ...current, sessionSignature: signature, ...(changed ? { lastActivityAt: Math.max(current.lastActivityAt, now) } : {}), updatedAt: now });
  } catch (error) {
    // Unknown observation remains conservative; never infer a write from directory metadata.
    console.error(`ws-pi-agent: could not observe owned session write: ${String(error)}`);
    updateOwnership(home, { liveness: { lifecycle: "unknown", observedAt: Date.now() } });
  }
}
