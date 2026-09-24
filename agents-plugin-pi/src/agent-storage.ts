/** Durable, Pi-local storage for delegated-agent material. */
import { getAgentDir } from "@earendil-works/pi-coding-agent";
import { existsSync, lstatSync, mkdirSync, readFileSync, readdirSync, realpathSync, renameSync, rmSync, rmdirSync, statSync, writeFileSync } from "node:fs";
import { basename, dirname, join, resolve, relative, sep } from "node:path";
import { randomUUID } from "node:crypto";
import { isDeepStrictEqual } from "node:util";
import { renameWithWindowsRetry, type RenameRetryHooks } from "./atomic-write.ts";
import { parseDelegationPolicy, type DelegationPolicy } from "./delegation-policy.ts";
import { mergeCumulativeCost, parseCumulativeCost, parseTelemetry, type AgentTelemetry, type CumulativeCost } from "./agent-telemetry.ts";
import { normalizeStoredExploreMode, type ExploreMode } from "./process-role.ts";
import { ownerNotifyRef, type OwnershipOwnerNotificationFingerprint } from "./owner-notify.ts";

export const OWNERSHIP_VERSION = 1;
const SAFE_COMPONENT = /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/;

export interface AgentStorageContext { root: string; ownerSessionId: string; }

export type OwnershipDiagnosticFingerprint =
  | "observer-session-write"
  | "metadata-update"
  | "home-removal"
  | "retention-scan"
  | "retention-start";
const OWNERSHIP_DIAGNOSTICS: Partial<Record<OwnershipDiagnosticFingerprint, {
  ownerFingerprint: OwnershipOwnerNotificationFingerprint;
  message: string;
}>> = {
  "metadata-update": { ownerFingerprint: "ownership-metadata-update", message: "ws: could not persist owned-agent metadata; the affected operation was rejected." },
  "home-removal": { ownerFingerprint: "ownership-home-removal", message: "ws: could not remove an owned-agent home; it was retained for safety." },
  "retention-scan": { ownerFingerprint: "ownership-retention-scan", message: "ws: owned-agent retention could not scan part of its storage; uncertain homes were retained." },
  "retention-start": { ownerFingerprint: "ownership-retention-start", message: "ws: owned-agent retention could not start; no uncertain home was removed." },
};

/** Central ownership reporter: observers are silent; authoritative classes reuse the session's owner notification seam. */
export function reportOwnershipDiagnostic(fingerprint: OwnershipDiagnosticFingerprint, _error: unknown): void {
  const diagnostic = OWNERSHIP_DIAGNOSTICS[fingerprint];
  if (diagnostic) ownerNotifyRef.notifyOwnershipOnce(diagnostic.ownerFingerprint, diagnostic.message);
}

export interface AgentOwnership {
  version: number; ownerSessionId: string; agentId: string; home: string; delegation?: DelegationPolicy;
  role: "worker" | "execute-worker" | "fork" | "explore"; exploreMode?: ExploreMode; sessionPath?: string;
}
export interface OwnershipMetadata extends AgentOwnership {
  createdAt: number; lastActivityAt: number; updatedAt: number;
  /** Latest durable child-attributable usage projection, used by ancestor footer aggregation and eviction roll-up. */
  telemetry?: AgentTelemetry;
  /** `pid` is legacy: it held the allocating parent's pid, never the child's. New records omit it; old records keep it and stay valid. */
  liveness: { lifecycle: "starting" | "live" | "stopping" | "stopped" | "unknown"; running?: boolean; observedAt?: number; pid?: number; instanceNonce?: string; threadBound?: boolean; ownerHeld?: boolean; pendingQuestion?: boolean; waitingOnChildren?: boolean; pendingDelivery?: boolean; pendingApprovalCommandId?: string; recovery?: "none" | "sidecar" | "thread" | "revived" };
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
/**
 * A hidden owner-scoped bucket: one dot-prefixed component, optionally nested
 * (`.cost-estimate/evicted`). Every level gets the same canonical containment
 * and symlink refusal.
 */
function ownerArtifactDirectory(ctx: AgentStorageContext, bucket: string, create: boolean): string | undefined {
  const [head, ...nested] = bucket.split("/");
  if (!/^\.[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(head) || nested.some(part => !SAFE_COMPONENT.test(part))) return undefined;
  try {
    const namespacePath = join(ctx.root, "ws-agents");
    const namespace = create ? checkedDirectory(namespacePath, ctx.root) : existingCheckedDirectory(namespacePath, ctx.root);
    if (!namespace) return undefined;
    let directory: string | undefined = namespace;
    for (const part of [safe(ctx.ownerSessionId, "Pi session id"), head, ...nested]) {
      const path: string = join(directory, part);
      directory = create ? checkedDirectory(path, directory) : existingCheckedDirectory(path, directory);
      if (!directory) return undefined;
    }
    return directory;
  } catch { return undefined; }
}
/** A contained regular file in an existing bucket; undefined when absent, symlinked, or not a regular file. */
function ownerArtifactFile(ctx: AgentStorageContext, bucket: string, name: string): string | undefined {
  if (!SAFE_COMPONENT.test(name)) return undefined;
  const directory = ownerArtifactDirectory(ctx, bucket, false);
  if (!directory) return undefined;
  const path = join(directory, name);
  try {
    const entry = lstatSync(path, { throwIfNoEntry: false });
    return entry?.isFile() && realpathSync(path) === path ? path : undefined;
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
  return readOwnerArtifactsOrFail(ctx, bucket) ?? [];
}
/** Like `readOwnerArtifacts`, but a failed listing or entry read is undefined instead of an empty bucket. */
function readOwnerArtifactsOrFail(ctx: AgentStorageContext, bucket: string): Array<{ name: string; content: string }> | undefined {
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
  } catch { return undefined; }
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

/** Removes one contained regular file from an owner-scoped bucket. True when it is absent afterwards; a symlinked or non-regular entry is refused. */
export function removeOwnerArtifact(ctx: AgentStorageContext, bucket: string, name: string): boolean {
  if (!SAFE_COMPONENT.test(name)) return false;
  const directory = ownerArtifactDirectory(ctx, bucket, false);
  if (!directory) return true;
  const path = join(directory, name);
  try {
    const entry = lstatSync(path, { throwIfNoEntry: false });
    if (!entry) return true;
    if (!entry.isFile() || realpathSync(path) !== path) return false;
    rmSync(path);
    return true;
  } catch { return false; }
}

/**
 * A cheap change signal for an owner-scoped bucket's entry set: the
 * directory's identity and mtime, without listing it. `changedAt` lets a
 * caller refuse to trust a signal that is too recent to be distinct from a
 * write landing in the same mtime tick.
 */
export function ownerArtifactSignal(ctx: AgentStorageContext, bucket: string): { key: string; changedAt: number } {
  const directory = ownerArtifactDirectory(ctx, bucket, false);
  if (!directory) return { key: "absent", changedAt: 0 };
  try {
    const stat = statSync(directory);
    return { key: `${stat.ino}:${stat.mtimeMs}`, changedAt: stat.mtimeMs };
  } catch { return { key: "absent", changedAt: 0 }; }
}

/** The owner storage a descriptor's home lives under, when the home has the adapter-owned `ws-agents/<owner>/<agentId>` shape. */
export function ownerStorageOf(ownership: Pick<AgentOwnership, "home" | "ownerSessionId">): AgentStorageContext | undefined {
  const owner = dirname(ownership.home), namespace = dirname(owner);
  if (basename(namespace) !== "ws-agents" || basename(owner) !== ownership.ownerSessionId || !SAFE_COMPONENT.test(ownership.ownerSessionId)) return undefined;
  return { root: dirname(namespace), ownerSessionId: ownership.ownerSessionId };
}

/**
 * Per-child eviction records: `ws-agents/<owner>/.cost-estimate/evicted/<agentId>.json`
 * holds a removed direct child's subtree cost (own usage plus its stored
 * descendant usage). A hop's removed-children total is its checkpoint's legacy
 * `evictedBaseline` plus the sum of these records, computed at read time; no
 * step folds a record into the baseline. A record supersedes every live count
 * of its agentId (registry, checkpoint `agents[]`, sidecar revival), and a
 * recorded agentId is never relaunched. Written only by
 * `removeOwnedAgentHome` under the removal lock (opt-in) and, for an unowned
 * capacity-evicted record that has no home, by its owner.
 */
export const EVICTION_RECORD_BUCKET = ".cost-estimate/evicted";
const EVICTION_RECORD_VERSION = 1;
const EVICTION_RECORD_SUFFIX = ".json";
function evictionRecordName(agentId: string): string | undefined { return SAFE_COMPONENT.test(agentId) ? `${agentId}${EVICTION_RECORD_SUFFIX}` : undefined; }
function parseEvictionRecord(raw: string, agentId: string): CumulativeCost | undefined {
  try {
    const value = JSON.parse(raw) as { version?: unknown; agentId?: unknown; cost?: unknown } | null;
    return value && value.version === EVICTION_RECORD_VERSION && value.agentId === agentId ? parseCumulativeCost(value.cost) : undefined;
  } catch { return undefined; }
}

/** Every valid eviction record of one owner, read from disk. Malformed, symlinked, and temporary entries are skipped. */
export function readEvictionRecords(ctx: AgentStorageContext): Map<string, CumulativeCost> {
  return tryReadEvictionRecords(ctx) ?? new Map();
}
/** Every valid record, or undefined when the directory or one of its entries could not be read (a partial read is never returned). */
export function tryReadEvictionRecords(ctx: AgentStorageContext): Map<string, CumulativeCost> | undefined {
  const artifacts = readOwnerArtifactsOrFail(ctx, EVICTION_RECORD_BUCKET);
  if (!artifacts) return undefined;
  const records = new Map<string, CumulativeCost>();
  for (const artifact of artifacts) {
    if (!artifact.name.endsWith(EVICTION_RECORD_SUFFIX)) continue;
    const agentId = artifact.name.slice(0, -EVICTION_RECORD_SUFFIX.length);
    if (!SAFE_COMPONENT.test(agentId)) continue;
    const cost = parseEvictionRecord(artifact.content, agentId);
    if (cost) records.set(agentId, cost);
  }
  return records;
}

/** One agentId's valid eviction record, read from disk. */
export function readEvictionRecord(ctx: AgentStorageContext, agentId: string): CumulativeCost | undefined {
  const name = evictionRecordName(agentId);
  const path = name && ownerArtifactFile(ctx, EVICTION_RECORD_BUCKET, name);
  if (!path) return undefined;
  try { return parseEvictionRecord(readFileSync(path, "utf8"), agentId); } catch { return undefined; }
}

/** True when a valid eviction record exists for the descriptor's agentId under its own owner. */
export function hasEvictionRecord(ownership: Pick<AgentOwnership, "home" | "ownerSessionId" | "agentId">): boolean {
  const ctx = ownerStorageOf(ownership);
  return !!ctx && readEvictionRecord(ctx, ownership.agentId) !== undefined;
}

/**
 * Atomically writes one agentId's eviction record. A rewrite merges with the
 * existing record (monotonic floor) instead of letting the last write win, so
 * two writers recording the same removal leave one value.
 */
export function writeEvictionRecord(ctx: AgentStorageContext, agentId: string, cost: CumulativeCost): boolean {
  const name = evictionRecordName(agentId);
  if (!name) return false;
  const value = mergeCumulativeCost(readEvictionRecord(ctx, agentId), cost);
  return writeOwnerArtifact(ctx, EVICTION_RECORD_BUCKET, name, `${JSON.stringify({ version: EVICTION_RECORD_VERSION, agentId, cost: value }, null, 2)}\n`);
}

/** The refusal for any rehydrate or relaunch of a recorded agentId. */
export function removedAgentMessage(agentId: string): string {
  return `ws-pi-agent: agent "${agentId}" was removed by retention or eviction and cannot be resumed; start a new agent instead`;
}

/** Deletes one agentId's eviction record; true when none remains. Only a removal that ends with the home at its path calls this. */
function removeEvictionRecord(ctx: AgentStorageContext, agentId: string): boolean {
  const name = evictionRecordName(agentId);
  return !!name && removeOwnerArtifact(ctx, EVICTION_RECORD_BUCKET, name);
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
  writeOwnership({ ...ownership, createdAt: now, lastActivityAt: now, updatedAt: now, liveness: { lifecycle: "starting", observedAt: now, instanceNonce: randomUUID(), recovery: "none" } });
  return ownership;
}
interface OwnershipLock { home: string; release(): void; }
class OwnershipLockBusyError extends Error {
  constructor(cause: unknown) { super(String(cause)); this.name = "OwnershipLockBusyError"; }
}

function ownershipLockPath(home: string): string { return join(dirname(home), `.${basename(home)}.ownership-lock`); }

/**
 * True only when an owned home is gone for good: absent, with no claim held
 * on it, and still absent after that. A remover holding the claim may yet
 * rename a detached home back (and then delete its eviction record), so an
 * absent home under a held claim is not final. A claim left by a crashed
 * remover keeps this false until a later claimant reclaims it.
 */
export function isOwnedHomeGone(ownership: Pick<AgentOwnership, "home">): boolean {
  const home = resolve(ownership.home);
  if (existsSync(home)) return false;
  if (existsSync(ownershipLockPath(home))) return false;
  return !existsSync(home);
}

/** A sibling lock survives atomic home detachment, serializing writers with deletion across processes. */
function acquireOwnershipLock(home: string): OwnershipLock {
  const canonical = canonicalHome(home);
  const ownerRoot = dirname(canonical);
  if (lstatSync(ownerRoot).isSymbolicLink() || realpathSync(ownerRoot) !== ownerRoot) throw new Error("ws-pi-agent: owned-home lock ancestry is symlinked");
  const lock = ownershipLockPath(canonical);
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

function writeOwnershipUnlocked(metadata: OwnershipMetadata, home: string, hooks?: RenameRetryHooks): void {
  if (metadata.home !== home) throw new Error("ws-pi-agent: ownership home is not canonical");
  if (metadata.sessionPath) checkedSessionPath(home, metadata.sessionPath);
  const target = ownershipPath(home), temp = join(home, `.ownership-${process.pid}-${Date.now()}.tmp`);
  writeFileSync(temp, `${JSON.stringify(metadata, null, 2)}\n`, { mode: 0o600 });
  try { renameWithWindowsRetry(temp, target, hooks); }
  catch (error) { try { rmSync(temp, { force: true }); } catch { /* the rename failure wins */ } throw error; }
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

/** `hooks` is a focused deterministic test seam for the rename retry; production callers pass one argument. */
export function writeOwnership(metadata: OwnershipMetadata, hooks?: RenameRetryHooks): void {
  const lock = acquireOwnershipLock(metadata.home);
  try { writeOwnershipUnlocked(metadata, lock.home, hooks); } finally { lock.release(); }
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
    (l.waitingOnChildren === undefined || typeof l.waitingOnChildren === "boolean") && (l.pendingDelivery === undefined || typeof l.pendingDelivery === "boolean") &&
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
type OwnershipUpdate = Partial<Pick<OwnershipMetadata, "lastActivityAt" | "liveness" | "delegation" | "telemetry">>;
export function updateOwnership(home: string, update: OwnershipUpdate): OwnershipMetadata | undefined {
  return updateOwnershipWhen(home, update);
}
/** `needed` re-checks the locked current record; false returns it without a write. */
function updateOwnershipWhen(home: string, update: OwnershipUpdate, needed?: (current: OwnershipMetadata) => boolean): OwnershipMetadata | undefined {
  let lock: OwnershipLock | undefined;
  try {
    lock = acquireOwnershipLock(home);
    const current = readOwnershipUnlocked(lock.home);
    if (!current) throw new Error("ownership metadata is missing or unreadable");
    if (needed && !needed(current)) return current;
    const now = Date.now();
    const next = { ...current, ...update, liveness: { ...current.liveness, ...update.liveness }, lastActivityAt: Math.max(current.lastActivityAt, update.lastActivityAt ?? current.lastActivityAt), updatedAt: now };
    writeOwnershipUnlocked(next, lock.home);
    return next;
  } catch (error) {
    reportOwnershipDiagnostic("metadata-update", error);
    return undefined;
  } finally { lock?.release(); }
}
export function touchOwnership(home: string): boolean { return updateOwnership(home, { lastActivityAt: Date.now() }) !== undefined; }

function sameTelemetry(persisted: AgentTelemetry | undefined, next: AgentTelemetry | undefined): boolean {
  // The persisted side is parsed JSON; round-trip the in-memory side so
  // undefined-valued keys and key order cannot count as a change.
  return isDeepStrictEqual(persisted ?? null, next === undefined ? null : JSON.parse(JSON.stringify(next)));
}

/**
 * Writes telemetry only when it differs from the persisted record. An unchanged
 * refresh takes no lock and writes nothing; because the comparison is against
 * disk, a write that failed (for example on a busy lock) is retried by the
 * next refresh even when memory did not change again.
 */
export function persistOwnershipTelemetry(home: string, telemetry: AgentTelemetry | undefined): void {
  const persisted = readOwnership(home);
  if (persisted && sameTelemetry(persisted.telemetry, telemetry)) return;
  updateOwnershipWhen(home, { telemetry }, current => !sameTelemetry(current.telemetry, telemetry));
}

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
    if (liveness.threadBound || liveness.ownerHeld || liveness.pendingQuestion || liveness.waitingOnChildren || liveness.pendingDelivery || liveness.pendingApprovalCommandId) {
      return { status: "retained", reason: "child has a protected owner, question, approval, descendant wait, or delivery" };
    }
    if (!hasOnlyContainedRegularEntries(home, home)) return { status: "retained", reason: "owned home contains a symlink or non-regular entry" };
    return { status: "eligible", metadata };
  } catch {
    return { status: "retained", reason: "owned-home eligibility could not be established" };
  }
}

export interface OwnedHomeRemovalOptions {
  /**
   * Opt-in eviction record: the removed child's subtree cost, computed from
   * the metadata that passed the final check. It is written under the removal
   * lock before the detach; a missing value or a failed write aborts the
   * removal with the home in place. Retention and capacity eviction opt in;
   * the stale sidecar-duplicate discard does not, because the live
   * registration of the same agentId already carries its cost.
   */
  evictionCost?: (metadata: OwnershipMetadata) => CumulativeCost | undefined;
  /** Deterministic test seam between the record write and the detach: a throw is a detach failure, a process exit is a crash. */
  beforeDetach?: () => void;
}

/**
 * Best-effort exact-home removal. A cross-process claim serializes the final
 * eligibility check, the opt-in eviction record, and detachment. Whenever the
 * call holds the claim and ends with the home at its original path, it
 * deletes any eviction record of that agentId, so a record exists only for a
 * home that was, or is still being, removed; a home that ends off its path
 * keeps its record.
 */
export function removeOwnedAgentHome(
  ownership: AgentOwnership,
  remove: (path: string) => void = path => rmSync(path, { recursive: true, force: false }),
  stillEligible?: (metadata: OwnershipMetadata) => boolean,
  options: OwnedHomeRemovalOptions = {},
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
    if (options.evictionCost) {
      const storage = ownerStorageOf(ownership);
      const cost = options.evictionCost(final.metadata);
      if (!storage || !cost || !writeEvictionRecord(storage, ownership.agentId, cost)) throw new Error("ws-pi-agent: could not write the eviction record; the home was retained");
    }
    options.beforeDetach?.();
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
    reportOwnershipDiagnostic("home-removal", error);
    return { status: "failed", error: message };
  } finally {
    // Stale-record repair, still under the claim: the home is back (or never
    // left), so it is counted live again and must not also count through a
    // record. A home that ended off its path keeps the record.
    if (!deleted && existsSync(ownership.home)) {
      const storage = ownerStorageOf(ownership);
      if (storage) removeEvictionRecord(storage, ownership.agentId);
    }
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
  /** The removed child's eviction-record value, computed under the removal lock (see `OwnedHomeRemovalOptions.evictionCost`). */
  evictionCost?: OwnedHomeRemovalOptions["evictionCost"];
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
          const removal = options.removeOwned
            ? remove(metadata)
            : removeOwnedAgentHome(metadata, undefined, current => current.lastActivityAt <= cutoff, { evictionCost: options.evictionCost });
          if (removal.status === "deleted") result.deletedHomes.push(home);
          else if (removal.status === "failed") result.failed += 1;
          else result.retained += 1;
        }
      } catch (error) {
        result.failed += 1;
        reportOwnershipDiagnostic("retention-scan", error);
      }
    }
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    if (code !== "ENOENT") {
      result.failed += 1;
      reportOwnershipDiagnostic("retention-scan", error);
    }
  }
  return result;
}

function sameSignature(persisted: OwnershipMetadata["sessionSignature"], stat: { mtimeMs: number; size: number }): boolean {
  return !!persisted && persisted.mtimeMs === stat.mtimeMs && persisted.size === stat.size;
}

/** Stats the session file; undefined means a pending first write, which is not a change. */
function statObservedSession(current: OwnershipMetadata, sessionPath: string): { mtimeMs: number; size: number } | undefined {
  try { return statSync(sessionPath); } catch (error) {
    // Pi writes a new session lazily. Absence before the first observed write
    // is pending observation, while disappearance of known history is not.
    if ((error as NodeJS.ErrnoException).code === "ENOENT" && !current.sessionSignature && sessionPath === current.sessionPath) return undefined;
    throw error;
  }
}

/**
 * Samples the actual session file under the same cross-process claim used by
 * deletion. A sample with nothing new to persist takes no lock and writes
 * nothing. Because the comparison is against disk, a changed signature whose
 * write failed (for example on a busy lock) is retried by the next sample.
 */
export function observeSessionWrite(home: string, sessionPath: string): void {
  // Unlocked pre-check; anything but a clean no-change falls through to the locked path.
  try {
    const persisted = readOwnership(home);
    if (persisted) {
      const stat = statObservedSession(persisted, sessionPath);
      if (!stat || sameSignature(persisted.sessionSignature, stat)) return;
    }
  } catch { /* the locked path classifies the failure */ }
  let lock: OwnershipLock | undefined;
  let current: OwnershipMetadata | undefined;
  try {
    lock = acquireOwnershipLock(home);
    current = readOwnershipUnlocked(lock.home); if (!current) return;
    const stat = statObservedSession(current, sessionPath);
    if (!stat) return;
    const signature = { mtimeMs: stat.mtimeMs, size: stat.size };
    // A timestamp alone is not a change: an unchanged signature writes nothing.
    if (sameSignature(current.sessionSignature, signature)) return;
    const now = Date.now();
    writeOwnershipUnlocked({ ...current, sessionSignature: signature, lastActivityAt: Math.max(current.lastActivityAt, now), updatedAt: now }, lock.home);
  } catch (error) {
    if (error instanceof OwnershipLockBusyError) return;
    // Unknown observation remains conservative; never infer a write from directory metadata.
    reportOwnershipDiagnostic("observer-session-write", error);
    // An already-unknown record is not rewritten just to refresh timestamps.
    if (lock && current && current.liveness.lifecycle !== "unknown") {
      try { writeOwnershipUnlocked({ ...current, updatedAt: Date.now(), liveness: { ...current.liveness, lifecycle: "unknown", observedAt: Date.now() } }, lock.home); } catch { /* the original observation failure remains diagnostic */ }
    }
  } finally { lock?.release(); }
}
