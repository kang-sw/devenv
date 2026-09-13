/** Durable, Pi-local storage for delegated-agent material. */
import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, renameSync, rmSync, rmdirSync, statSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve, relative, sep } from "node:path";
import { randomUUID } from "node:crypto";
import { parseDelegationPolicy, type DelegationPolicy } from "./delegation-policy.ts";
import { parseTelemetry, type AgentTelemetry } from "./agent-telemetry.ts";
import { normalizeStoredExploreMode, type ExploreMode } from "./process-role.ts";

export const OWNERSHIP_VERSION = 1;
const SAFE_COMPONENT = /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export interface AgentStorageContext { root: string; ownerSessionId: string; }
export interface AgentOwnership {
  version: number; ownerSessionId: string; agentId: string; home: string; delegation?: DelegationPolicy;
  role: "worker" | "execute-worker" | "fork" | "explore"; exploreMode?: ExploreMode; sessionPath?: string;
}
export interface OwnershipMetadata extends AgentOwnership {
  createdAt: number; lastActivityAt: number; updatedAt: number;
  /** Latest durable child-attributable usage projection, used by ancestor footer aggregation and eviction roll-up. */
  telemetry?: AgentTelemetry;
  liveness: { lifecycle: "starting" | "live" | "stopping" | "stopped" | "unknown"; running?: boolean; observedAt?: number; pid?: number; instanceNonce?: string; threadBound?: boolean; ownerHeld?: boolean; pendingQuestion?: boolean; waitingOnChildren?: boolean; expectedReport?: boolean; pendingApprovalCommandId?: string; recovery?: "none" | "sidecar" | "thread" | "revived" };
  sessionSignature?: { mtimeMs: number; size: number };
}

function safe(value: string, label: string): string { if (!SAFE_COMPONENT.test(value)) throw new Error(`ws-pi-agent: unsafe ${label}`); return value; }
function contained(parent: string, child: string): boolean { const r = relative(parent, child); return r === "" || (!!r && !r.startsWith(`..${sep}`) && r !== ".."); }
function canonicalRoot(root: string): string { mkdirSync(root, { recursive: true, mode: 0o700 }); return realpathSync(root); }
function checkedDirectory(path: string, root: string): string { mkdirSync(path, { recursive: true, mode: 0o700 }); const real = realpathSync(path); if (!contained(root, real) || lstatSync(path).isSymbolicLink()) throw new Error("ws-pi-agent: owned path contains a symlink escape"); return real; }
function existingCheckedDirectory(path: string, root: string): string | undefined {
  try {
    const resolved = resolve(path), real = realpathSync(resolved);
    return real === resolved && contained(root, real) && !lstatSync(resolved).isSymbolicLink() && statSync(resolved).isDirectory() ? resolved : undefined;
  } catch { return undefined; }
}
function ownerArtifactDirectory(ctx: AgentStorageContext, bucket: string, create: boolean): string | undefined {
  if (!/^\.[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(bucket)) return undefined;
  try {
    const namespacePath = join(ctx.root, "ws-agents");
    const namespace = create ? checkedDirectory(namespacePath, ctx.root) : existingCheckedDirectory(namespacePath, ctx.root);
    if (!namespace) return undefined;
    const ownerPath = join(namespace, safe(ctx.ownerSessionId, "Pi session id"));
    const owner = create ? checkedDirectory(ownerPath, namespace) : existingCheckedDirectory(ownerPath, namespace);
    if (!owner) return undefined;
    const bucketPath = join(owner, bucket);
    return create ? checkedDirectory(bucketPath, owner) : existingCheckedDirectory(bucketPath, owner);
  } catch { return undefined; }
}
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

/** Reads contained regular files from a hidden owner-scoped adapter bucket. */
export function readOwnerArtifacts(ctx: AgentStorageContext, bucket: string): Array<{ name: string; content: string }> {
  const directory = ownerArtifactDirectory(ctx, bucket, false);
  if (!directory) return [];
  const out: Array<{ name: string; content: string }> = [];
  try {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      if (!entry.isFile() || !SAFE_COMPONENT.test(entry.name)) continue;
      const path = join(directory, entry.name);
      if (lstatSync(path).isSymbolicLink() || realpathSync(path) !== path) continue;
      out.push({ name: entry.name, content: readFileSync(path, "utf8") });
    }
  } catch { return []; }
  return out;
}

/** Atomically writes one contained regular file in an owner-scoped adapter bucket. */
export function writeOwnerArtifact(ctx: AgentStorageContext, bucket: string, name: string, content: string): boolean {
  if (!SAFE_COMPONENT.test(name)) return false;
  const directory = ownerArtifactDirectory(ctx, bucket, true);
  if (!directory) return false;
  const target = join(directory, name), temporary = join(directory, `.${name}-${process.pid}-${randomUUID()}.tmp`);
  try {
    writeFileSync(temporary, content, { mode: 0o600 });
    renameSync(temporary, target);
    return true;
  } catch {
    try { rmSync(temporary, { force: true }); } catch { /* original failure wins */ }
    return false;
  }
}

export function ownershipPath(home: string): string { return join(home, "ownership.json"); }
export function allocateAgentHome(ctx: AgentStorageContext, agentId: string, role: AgentOwnership["role"], exploreMode?: ExploreMode): AgentOwnership {
  safe(agentId, "agent id");
  const namespace = checkedDirectory(join(ctx.root, "ws-agents"), ctx.root);
  const ownerRoot = checkedDirectory(join(namespace, safe(ctx.ownerSessionId, "Pi session id")), ctx.root);
  const home = resolve(ownerRoot, agentId);
  if (!contained(ownerRoot, home)) throw new Error("ws-pi-agent: agent home escapes configured Pi directory");
  checkedDirectory(home, ownerRoot);
  const sessionPath = join(home, "session.jsonl");
  const now = Date.now();
  const ownership: AgentOwnership = { version: OWNERSHIP_VERSION, ownerSessionId: ctx.ownerSessionId, agentId, home, role, ...(exploreMode ? { exploreMode } : {}), ...(sessionPath ? { sessionPath } : {}) };
  writeOwnership({ ...ownership, createdAt: now, lastActivityAt: now, updatedAt: now, liveness: { lifecycle: "starting", observedAt: now, pid: process.pid, instanceNonce: randomUUID(), recovery: "none" } });
  return ownership;
}
interface OwnershipLock { home: string; release(): void; }
class OwnershipLockBusyError extends Error {
  constructor(cause: unknown) { super(String(cause)); this.name = "OwnershipLockBusyError"; }
}

/** A sibling lock survives atomic home detachment, serializing writers with deletion across processes. */
function acquireOwnershipLock(home: string): OwnershipLock {
  const canonical = canonicalHome(home);
  const ownerRoot = dirname(canonical);
  if (lstatSync(ownerRoot).isSymbolicLink() || realpathSync(ownerRoot) !== ownerRoot) throw new Error("ws-pi-agent: owned-home lock ancestry is symlinked");
  const lock = join(ownerRoot, `.${basename(canonical)}.ownership-lock`);
  const ownerFile = join(lock, "owner.json");
  for (let attempt = 0; attempt < 2; attempt += 1) {
    try {
      mkdirSync(lock, { mode: 0o700 });
      try { writeFileSync(ownerFile, `${JSON.stringify({ pid: process.pid })}\n`, { mode: 0o600 }); }
      catch (error) { try { rmSync(lock, { recursive: true, force: true }); } catch { /* original write failure wins */ } throw error; }
      let held = true;
      return { home: canonical, release: () => { if (!held) return; held = false; try { rmSync(lock, { recursive: true, force: true }); } catch { /* a failed release conservatively blocks later deletion */ } } };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== "EEXIST") throw error;
      // A process killed while holding the claim must not strand the home
      // forever. PID reuse and unreadable/malformed owner facts retain it.
      let pid: number | undefined;
      try {
        const parsed = JSON.parse(readFileSync(ownerFile, "utf8")) as { pid?: unknown };
        if (typeof parsed.pid === "number" && Number.isInteger(parsed.pid) && parsed.pid > 0) pid = parsed.pid;
      } catch { throw error; }
      if (!pid) throw error;
      let stale = false;
      try { process.kill(pid, 0); }
      catch (probeError) {
        if ((probeError as NodeJS.ErrnoException).code !== "ESRCH") throw error;
        stale = true;
      }
      if (!stale) throw new OwnershipLockBusyError(error);
      if (attempt > 0) throw error;
      const abandoned = `${lock}.abandoned-${process.pid}-${randomUUID()}`;
      renameSync(lock, abandoned);
      try { rmSync(abandoned, { recursive: true, force: true }); } catch { /* detached stale claim cannot block retry */ }
    }
  }
  throw new Error("ws-pi-agent: could not acquire owned-home claim");
}

function writeOwnershipUnlocked(metadata: OwnershipMetadata, home: string): void {
  if (metadata.home !== home) throw new Error("ws-pi-agent: ownership home is not canonical");
  if (metadata.sessionPath) checkedSessionPath(home, metadata.sessionPath);
  const target = ownershipPath(home), temp = join(home, `.ownership-${process.pid}-${Date.now()}.tmp`);
  writeFileSync(temp, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 });
  renameSync(temp, target);
}

function readOwnershipUnlocked(home: string): OwnershipMetadata | undefined {
  try {
    const raw = JSON.parse(readFileSync(ownershipPath(home), "utf8")) as OwnershipMetadata;
    const mode = normalizeStoredExploreMode(raw.exploreMode);
    if (raw.exploreMode !== undefined && !mode) return undefined;
    const value = raw.exploreMode === undefined ? raw : { ...raw, exploreMode: mode };
    return validOwnership(value) && value.home === home && (!value.sessionPath || (checkedSessionPath(home, value.sessionPath), true)) ? value : undefined;
  } catch { return undefined; }
}

export function writeOwnership(metadata: OwnershipMetadata): void {
  const lock = acquireOwnershipLock(metadata.home);
  try { writeOwnershipUnlocked(metadata, lock.home); } finally { lock.release(); }
}
export function readOwnership(home: string): OwnershipMetadata | undefined {
  try { return readOwnershipUnlocked(canonicalHome(home)); } catch { return undefined; }
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
    (o.telemetry === undefined || parseTelemetry(o.telemetry) !== undefined) &&
    (l.recovery === undefined || ["none","sidecar","thread","revived"].includes(l.recovery)) &&
    (signature === undefined || (Number.isFinite(signature.mtimeMs) && Number.isFinite(signature.size) && signature.size >= 0));
}
export function validDescriptor(value: unknown): value is AgentOwnership { const o = value as Partial<AgentOwnership> | null; return !!o && o.version === OWNERSHIP_VERSION && typeof o.ownerSessionId === "string" && SAFE_COMPONENT.test(o.ownerSessionId) && typeof o.agentId === "string" && SAFE_COMPONENT.test(o.agentId) && typeof o.home === "string" && ["worker","execute-worker","fork","explore"].includes(o.role as string) && (o.exploreMode === undefined || normalizeStoredExploreMode(o.exploreMode) === o.exploreMode) && (o.sessionPath === undefined || typeof o.sessionPath === "string") && validDelegationDescriptor(o.delegation); }
function validDelegationDescriptor(value: unknown): boolean {
  if (value === undefined) return true;
  try { parseDelegationPolicy(value); return true; } catch { return false; }
}
export function updateOwnership(home: string, update: Partial<Pick<OwnershipMetadata, "lastActivityAt" | "liveness" | "delegation" | "telemetry">>): OwnershipMetadata | undefined {
  let lock: OwnershipLock | undefined;
  try {
    lock = acquireOwnershipLock(home);
    const current = readOwnershipUnlocked(lock.home);
    if (!current) throw new Error("ownership metadata is missing or unreadable");
    const now = Date.now();
    const next = { ...current, ...update, liveness: { ...current.liveness, ...update.liveness }, lastActivityAt: Math.max(current.lastActivityAt, update.lastActivityAt ?? current.lastActivityAt), updatedAt: now };
    writeOwnershipUnlocked(next, lock.home);
    return next;
  } catch (error) {
    console.error(`ws-pi-agent: could not update owned agent home ${home}: ${String(error)}`);
    return undefined;
  } finally { lock?.release(); }
}
export function touchOwnership(home: string): boolean { return updateOwnership(home, { lastActivityAt: Date.now() }) !== undefined; }

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

/** Best-effort exact-home removal. A cross-process claim serializes the final eligibility check through detachment. */
export function removeOwnedAgentHome(
  ownership: AgentOwnership,
  remove: (path: string) => void = path => rmSync(path, { recursive: true, force: false }),
  stillEligible?: (metadata: OwnershipMetadata) => boolean,
): OwnedHomeRemovalResult {
  let lock: OwnershipLock;
  try { lock = acquireOwnershipLock(ownership.home); }
  catch { return { status: "retained", reason: "owned home is unavailable or busy" }; }
  const ownerRoot = dirname(ownership.home);
  const staged = join(ownerRoot, `.${ownership.agentId}.deleting-${process.pid}-${randomUUID()}`);
  let moved = false;
  let deleted = false;
  let checked: OwnershipMetadata | undefined;
  try {
    const final = inspectOwnedHomeRemoval(ownership);
    if (final.status !== "eligible") return final;
    if (stillEligible && !stillEligible(final.metadata)) return { status: "retained", reason: "owned-home eligibility changed before deletion" };
    checked = final.metadata;
    // The sibling lock stays at the original locator while the checked home is
    // atomically detached, so a concurrent touch/resume cannot recreate or
    // mutate the path between eligibility and recursive removal.
    renameSync(ownership.home, staged);
    moved = true;
    const stagedEntry = lstatSync(staged);
    if (!stagedEntry.isDirectory() || stagedEntry.isSymbolicLink() || realpathSync(staged) !== staged) throw new Error("owned home changed type during deletion");
    remove(staged);
    moved = false;
    deleted = true;
    return { status: "deleted" };
  } catch (error) {
    if (moved && existsSync(staged) && !existsSync(ownership.home)) {
      try { renameSync(staged, ownership.home); moved = false; } catch { /* diagnostic below; never chase a replacement path */ }
    }
    if (!moved && checked && existsSync(ownership.home) && !readOwnership(ownership.home)) {
      try { writeOwnershipUnlocked(checked, canonicalHome(ownership.home)); } catch { /* original failure remains the diagnostic */ }
    }
    const message = String(error);
    console.error(`ws-pi-agent: could not remove owned agent home: ${message}`);
    return { status: "failed", error: message };
  } finally {
    lock.release();
    if (deleted) try { rmdirSync(ownerRoot); } catch { /* another child or sidecar still owns the lead subtree */ }
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
  /** Called under the same final-age decision before removal; false retains the home. */
  beforeRemove?: (metadata: OwnershipMetadata) => boolean;
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
          if (childEntry.name.startsWith(".") || !childEntry.isDirectory() || childEntry.isSymbolicLink()) continue;
          result.scanned += 1;
          const home = join(ownerRoot, childEntry.name);
          let metadata = readOwnership(home);
          if (!metadata) { result.retained += 1; continue; }
          if (metadata.sessionPath) observe(home, metadata.sessionPath);
          metadata = readOwnership(home);
          if (!metadata || metadata.lastActivityAt > cutoff) { result.retained += 1; continue; }
          if (options.beforeRemove && !options.beforeRemove(metadata)) { result.retained += 1; continue; }
          const removal = options.removeOwned
            ? remove(metadata)
            : removeOwnedAgentHome(metadata, undefined, current => current.lastActivityAt <= cutoff);
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

/** Samples the actual session file under the same cross-process claim used by deletion. */
export function observeSessionWrite(home: string, sessionPath: string): void {
  let lock: OwnershipLock | undefined;
  let current: OwnershipMetadata | undefined;
  try {
    lock = acquireOwnershipLock(home);
    current = readOwnershipUnlocked(lock.home); if (!current) return;
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
    writeOwnershipUnlocked({ ...current, sessionSignature: signature, ...(changed ? { lastActivityAt: Math.max(current.lastActivityAt, now) } : {}), updatedAt: now }, lock.home);
  } catch (error) {
    if (error instanceof OwnershipLockBusyError) return;
    // Unknown observation remains conservative; never infer a write from directory metadata.
    console.error(`ws-pi-agent: could not observe owned session write: ${String(error)}`);
    if (lock && current) {
      try { writeOwnershipUnlocked({ ...current, updatedAt: Date.now(), liveness: { ...current.liveness, lifecycle: "unknown", observedAt: Date.now() } }, lock.home); } catch { /* the original observation failure remains diagnostic */ }
    }
  } finally { lock?.release(); }
}
