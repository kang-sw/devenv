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

/** Best-effort exact-home removal. Eligibility is checked again immediately before deletion. */
export function removeOwnedAgentHome(
  ownership: AgentOwnership,
  remove: (path: string) => void = path => rmSync(path, { recursive: true, force: false }),
): OwnedHomeRemovalResult {
  const first = inspectOwnedHomeRemoval(ownership);
  if (first.status !== "eligible") return first;
  const final = inspectOwnedHomeRemoval(ownership);
  if (final.status !== "eligible") return final;
  const ownerRoot = dirname(ownership.home);
  const staged = join(ownerRoot, `.${ownership.agentId}.deleting-${process.pid}-${randomUUID()}`);
  let moved = false;
  try {
    // Atomic detachment closes the checked-path race: a concurrent replacement
    // at the old name is never traversed. fs.rm treats any later symlink swap
    // under the private staged tree as the link entry itself, not its target.
    renameSync(ownership.home, staged);
    moved = true;
    const stagedEntry = lstatSync(staged);
    if (!stagedEntry.isDirectory() || stagedEntry.isSymbolicLink() || realpathSync(staged) !== staged) {
      throw new Error("owned home changed type during deletion");
    }
    remove(staged);
    moved = false;
    try { rmdirSync(ownerRoot); } catch { /* another child or sidecar still owns the lead subtree */ }
    return { status: "deleted" };
  } catch (error) {
    if (moved && existsSync(staged) && !existsSync(ownership.home)) {
      try { renameSync(staged, ownership.home); moved = false; } catch { /* diagnostic below; never chase a replacement path */ }
    }
    // A late partial failure after metadata removal gets one best-effort
    // authorization restore, but only at the original canonical home.
    if (!moved && existsSync(ownership.home) && !readOwnership(ownership.home)) {
      try { writeOwnership(final.metadata); } catch { /* original failure remains the diagnostic */ }
    }
    const message = String(error);
    console.error(`ws-pi-agent: could not remove owned agent home: ${message}`);
    return { status: "failed", error: message };
  }
}

export interface StaleAgentPruneResult {
  scanned: number;
  retained: number;
  failed: number;
  deletedHomes: string[];
}

interface StaleAgentPruneOptions {
  now?: () => number;
  observeSession?: (home: string, sessionPath: string) => void;
  removeOwned?: (ownership: AgentOwnership) => OwnedHomeRemovalResult;
}

/**
 * Scans only the adapter-owned `<agentDir>/ws-agents/<owner>/<child>` shape.
 * Unknown entries and uncertain metadata are retained; the ordinary Pi session
 * tree and legacy paths are never scanned. Session-file writes are sampled
 * before age is decided so an unobserved final write can renew activity.
 */
export function pruneStaleAgentHomes(root: string, ttlDays: number | false, options: StaleAgentPruneOptions = {}): StaleAgentPruneResult {
  const result: StaleAgentPruneResult = { scanned: 0, retained: 0, failed: 0, deletedHomes: [] };
  if (ttlDays === false || !Number.isFinite(ttlDays) || ttlDays <= 0) return result;
  const now = options.now?.() ?? Date.now();
  const cutoff = now - ttlDays * 86_400_000;
  const observe = options.observeSession ?? observeSessionWrite;
  const remove = options.removeOwned ?? removeOwnedAgentHome;
  try {
    const canonical = realpathSync(resolve(root));
    const namespace = join(canonical, "ws-agents");
    const namespaceEntry = lstatSync(namespace, { throwIfNoEntry: false });
    if (!namespaceEntry) return result;
    if (!namespaceEntry.isDirectory() || namespaceEntry.isSymbolicLink() || realpathSync(namespace) !== namespace) {
      result.retained += 1;
      return result;
    }
    for (const ownerEntry of readdirSync(namespace, { withFileTypes: true })) {
      const ownerRoot = join(namespace, ownerEntry.name);
      if (!ownerEntry.isDirectory() || ownerEntry.isSymbolicLink()) continue;
      try {
        if (realpathSync(ownerRoot) !== ownerRoot) { result.retained += 1; continue; }
        for (const childEntry of readdirSync(ownerRoot, { withFileTypes: true })) {
          if (!childEntry.isDirectory() || childEntry.isSymbolicLink()) continue;
          result.scanned += 1;
          const home = join(ownerRoot, childEntry.name);
          let metadata = readOwnership(home);
          if (!metadata) { result.retained += 1; continue; }
          if (metadata.sessionPath) observe(home, metadata.sessionPath);
          metadata = readOwnership(home);
          if (!metadata || metadata.lastActivityAt > cutoff) { result.retained += 1; continue; }
          const removal = remove(metadata);
          if (removal.status === "deleted") result.deletedHomes.push(home);
          else if (removal.status === "failed") result.failed += 1;
          else result.retained += 1;
        }
      } catch (error) {
        result.failed += 1;
        console.error(`ws-pi-agent: could not scan owned lead subtree for retention: ${String(error)}`);
      }
    }
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code !== "ENOENT") {
      result.failed += 1;
      console.error(`ws-pi-agent: could not scan owned agent homes for retention: ${String(error)}`);
    }
  }
  return result;
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
