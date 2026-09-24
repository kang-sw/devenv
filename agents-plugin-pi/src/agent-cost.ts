/**
 * Every hop's cost estimate and owner checkpoint, footer or not: the value a
 * hop reports upward as its descendant usage (`descendantUsageValue`), the
 * value an eviction record holds, and the removed-agent gate
 * (`isRemovedAgent`) that reads the same owner storage. The footer
 * (agent-footer.ts) renders this estimate and imports from here; nothing here
 * imports the footer.
 *
 * Side effect on the registry: every entry point that reads a cost first
 * prunes against the owner's eviction records (`pruneAndRecompute`), and a
 * recorded direct child whose home is gone for good is dropped from the
 * registry there (its observer stopped). The records are read only at that
 * point, so the drop lives with them.
 */
import { EVICTION_RECORD_BUCKET, durableDescendantUsage, hasEvictionRecord, isOwnedHomeGone, ownerArtifactSignal, ownerStorageOf, readEvictionRecord, readOwnerArtifacts, tryReadEvictionRecords, writeEvictionRecord, writeOwnerArtifact, type AgentOwnership, type AgentStorageContext, type OwnershipMetadata } from "./agent-storage.ts";
import { formatCumulativeCost, mergeCumulativeCost as mergeAgentCost, parseCumulativeCost as parseCost, type AgentTelemetry, type CumulativeCost } from "./agent-telemetry.ts";
import { descendantUsageOf } from "./agent-usage-rollup.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";

const CHECKPOINT_BUCKET = ".cost-estimate";
const CHECKPOINT_FILE = "checkpoint.json";
const CHECKPOINT_VERSION = 1;
/** An eviction-record directory signal younger than this is not trusted as unchanged: a write may share its mtime tick. */
const EVICTED_SIGNAL_SETTLE_MS = 2_000;
const retainedFailedCheckpoints = new Map<string, CostCheckpoint>();

const emptyCost = (): CumulativeCost => ({ knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 });
function addCost(target: CumulativeCost, source: CumulativeCost): void {
  target.knownUsd += source.knownUsd;
  target.knownContributors += source.knownContributors;
  target.unknownContributors += source.unknownContributors;
  target.descendants += source.descendants;
}
function cloneCost(value: CumulativeCost): CumulativeCost { return { ...value }; }
function ownCost(value: AgentTelemetry | undefined): CumulativeCost {
  if (value?.estimatedUsd !== undefined) return { knownUsd: value.estimatedUsd, knownContributors: 1, unknownContributors: 0, descendants: 1 };
  if (value?.partialEstimatedUsd !== undefined) return { knownUsd: value.partialEstimatedUsd, knownContributors: 1, unknownContributors: 1, descendants: 1 };
  return { knownUsd: 0, knownContributors: 0, unknownContributors: 1, descendants: 1 };
}
/** A direct child's subtree total: its own reduced usage plus its last reported descendant usage. */
function subtreeCost(telemetry: AgentTelemetry | undefined, descendants: CumulativeCost | undefined): CumulativeCost {
  const total = ownCost(telemetry);
  if (descendants) addCost(total, descendants);
  return total;
}
function recordSubtreeCost(record: RpcAgentRecord): CumulativeCost { return subtreeCost(record.telemetry, descendantUsageOf(record)); }

export interface LeadUsageSummary {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  latestCacheHitRate?: number;
  cost: CumulativeCost;
}

interface StoredAgentCost { agentId: string; cost: CumulativeCost }
/**
 * Written only by its owner process (`persist`). `evictedBaseline` is legacy
 * and read-only: loaded, summed, and written back unchanged, never increased.
 * Removed children count through per-child eviction records instead (see
 * `EVICTION_RECORD_BUCKET` in agent-storage.ts).
 */
interface CostCheckpoint {
  version: 1;
  lead: LeadUsageSummary;
  evictedBaseline: CumulativeCost;
  agents: StoredAgentCost[];
}

const emptyLeadUsage = (): LeadUsageSummary => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: emptyCost() });
const emptyCheckpoint = (): CostCheckpoint => ({ version: CHECKPOINT_VERSION, lead: emptyLeadUsage(), evictedBaseline: emptyCost(), agents: [] });
function cloneCheckpoint(value: CostCheckpoint): CostCheckpoint {
  return {
    version: CHECKPOINT_VERSION,
    lead: { ...value.lead, cost: cloneCost(value.lead.cost) },
    evictedBaseline: cloneCost(value.evictedBaseline),
    agents: value.agents.map(agent => ({ agentId: agent.agentId, cost: cloneCost(agent.cost) })),
  };
}
function checkpointKey(storage: AgentStorageContext): string { return `${storage.root}\0${storage.ownerSessionId}`; }
function retainFailedCheckpoint(storage: AgentStorageContext, checkpoint: CostCheckpoint): void {
  retainedFailedCheckpoints.set(checkpointKey(storage), cloneCheckpoint(checkpoint));
}
const nonnegative = (value: unknown): number | undefined => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
function safePart(value: string): boolean { return /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(value); }
function parseLeadUsage(value: unknown): LeadUsageSummary | undefined {
  const lead = value as Partial<LeadUsageSummary> | null;
  if (!lead) return undefined;
  const input = nonnegative(lead.input), output = nonnegative(lead.output), cacheRead = nonnegative(lead.cacheRead), cacheWrite = nonnegative(lead.cacheWrite);
  const latestCacheHitRate = lead.latestCacheHitRate === undefined ? undefined : nonnegative(lead.latestCacheHitRate);
  const cost = parseCost(lead.cost);
  if ([input, output, cacheRead, cacheWrite].some(number => number === undefined) || !cost || (latestCacheHitRate !== undefined && latestCacheHitRate > 100)) return undefined;
  return { input: input!, output: output!, cacheRead: cacheRead!, cacheWrite: cacheWrite!, ...(latestCacheHitRate !== undefined ? { latestCacheHitRate } : {}), cost };
}
function parseCheckpoint(raw: string): CostCheckpoint | undefined {
  let value: unknown;
  try { value = JSON.parse(raw); } catch { return undefined; }
  const file = value as Partial<CostCheckpoint> | null;
  const lead = parseLeadUsage(file?.lead), evictedBaseline = parseCost(file?.evictedBaseline);
  if (!file || file.version !== CHECKPOINT_VERSION || !lead || !evictedBaseline || !Array.isArray(file.agents)) return undefined;
  const agents: StoredAgentCost[] = [];
  const seen = new Set<string>();
  for (const rawAgent of file.agents) {
    const agent = rawAgent as Partial<StoredAgentCost> | null;
    const cost = parseCost(agent?.cost);
    if (!agent || typeof agent.agentId !== "string" || !safePart(agent.agentId) || !cost || seen.has(agent.agentId)) return undefined;
    seen.add(agent.agentId); agents.push({ agentId: agent.agentId, cost });
  }
  return { version: CHECKPOINT_VERSION, lead, evictedBaseline, agents };
}
function loadCheckpoint(storage: AgentStorageContext): { checkpoint: CostCheckpoint; found: boolean } {
  const retained = retainedFailedCheckpoints.get(checkpointKey(storage));
  if (retained) return { checkpoint: cloneCheckpoint(retained), found: true };
  const artifact = readOwnerArtifacts(storage, CHECKPOINT_BUCKET).find(entry => entry.name === CHECKPOINT_FILE);
  if (!artifact) return { checkpoint: emptyCheckpoint(), found: false };
  const checkpoint = parseCheckpoint(artifact.content);
  return checkpoint ? { checkpoint, found: true } : { checkpoint: emptyCheckpoint(), found: false };
}
function writeCheckpoint(storage: AgentStorageContext, checkpoint: CostCheckpoint): boolean {
  const written = writeOwnerArtifact(storage, CHECKPOINT_BUCKET, CHECKPOINT_FILE, `${JSON.stringify(checkpoint, null, 2)}\n`);
  if (written) retainedFailedCheckpoints.delete(checkpointKey(storage));
  else {
    retainFailedCheckpoint(storage, checkpoint);
    console.error(`ws-pi-agent: cost checkpoint write failed for owner ${storage.ownerSessionId}; retaining the latest estimate until retry`);
  }
  return written;
}
function storageFromRecord(record: Pick<RpcAgentRecord, "ownership">): AgentStorageContext | undefined {
  return record.ownership ? ownerStorageOf(record.ownership) : undefined;
}

class CostEstimateState {
  readonly storage: AgentStorageContext;
  readonly registry: RpcAgentRegistry;
  readonly lead: LeadUsageSummary;
  /** Legacy, read-only: loaded and summed, never increased. */
  readonly evictedBaseline: CumulativeCost;
  readonly agents: Map<string, CumulativeCost>;
  private evictionRecords = new Map<string, CumulativeCost>();
  private evictionSignal: string | undefined;
  private directTotal = emptyCost();
  private acceptedObjects = new WeakSet<object>();

  constructor(storage: AgentStorageContext, registry: RpcAgentRegistry, checkpoint: CostCheckpoint) {
    this.storage = storage;
    this.registry = registry;
    this.lead = { ...checkpoint.lead, cost: cloneCost(checkpoint.lead.cost) };
    this.evictedBaseline = cloneCost(checkpoint.evictedBaseline);
    this.agents = new Map(checkpoint.agents.map(agent => [agent.agentId, cloneCost(agent.cost)]));
    this.pruneAndRecompute();
  }

  /**
   * The one ordered sync every cost entry point runs: refresh the eviction
   * records, prune the registry against them (`pruneRemovedAgents`, which
   * mutates `registry`), then recompute the direct children's totals
   * (`recomputeAgents`). Prune and compute both read the record snapshot this
   * call just refreshed; records are not re-read between them.
   */
  pruneAndRecompute(omit: ReadonlySet<string> = new Set()): void {
    this.refreshEvictionRecords();
    this.pruneRemovedAgents();
    this.recomputeAgents(omit);
  }

  /**
   * Registry mutation: drops each recorded registry entry whose home is gone
   * for good (or that has none), stopping its ownership observer through the
   * record's own `ownershipObserverStop`. A recorded entry that is live
   * (`client`) or launching stays. A home-present one is removal-pending and
   * stays registered, and so does one whose home is absent under a held
   * removal claim (`isOwnedHomeGone` is false then): that remover may still
   * rename the home back and delete the record, after which the entry must
   * count again. Reads the records the caller just refreshed.
   */
  private pruneRemovedAgents(): void {
    const removed: string[] = [];
    for (const [agentId, record] of this.registry) {
      if (this.evictionRecords.has(agentId) && !record.client && !record.launching && (!record.ownership || isOwnedHomeGone(record.ownership))) removed.push(agentId);
    }
    for (const agentId of removed) {
      const record = this.registry.get(agentId);
      record?.ownershipObserverStop?.();
      if (record) record.ownershipObserverStop = undefined;
      this.registry.delete(agentId);
    }
  }

  /**
   * Refreshes each registry-resident direct child's subtree total and drops
   * identities that left the registry. Dropping never folds: a removed child
   * counts only through its eviction record, which supersedes every live
   * count of the same agentId, so a recorded registry entry is excluded from
   * the sum.
   */
  private recomputeAgents(omit: ReadonlySet<string>): void {
    for (const [agentId, record] of this.registry) {
      if (this.evictionRecords.has(agentId) || omit.has(agentId)) continue;
      this.agents.set(agentId, mergeAgentCost(this.agents.get(agentId), recordSubtreeCost(record)));
    }
    for (const agentId of [...this.agents.keys()]) {
      if (omit.has(agentId) || this.evictionRecords.has(agentId) || !this.registry.has(agentId)) this.agents.delete(agentId);
    }
    this.recomputeDirectTotal();
  }

  /**
   * Records are always read from disk (never the retained failed-checkpoint
   * cache), but only when the directory's change signal moved, so a
   * sync with nothing new does not list the directory.
   */
  private refreshEvictionRecords(): void {
    const signal = ownerArtifactSignal(this.storage, EVICTION_RECORD_BUCKET);
    if (this.evictionSignal !== undefined && signal.key === this.evictionSignal) return;
    const records = tryReadEvictionRecords(this.storage);
    // A failed read keeps the last good records and caches nothing, so the
    // next sync retries instead of hiding every record until evicted/ changes.
    if (records) this.evictionRecords = records;
    this.evictionSignal = records && Date.now() - signal.changedAt > EVICTED_SIGNAL_SETTLE_MS ? signal.key : undefined;
  }

  /** The value an eviction record of this direct child holds: its tracked floor merged with its current subtree cost. Prunes first. */
  evictionCost(record: RpcAgentRecord): CumulativeCost {
    this.pruneAndRecompute();
    return mergeAgentCost(this.agents.get(record.agentId), recordSubtreeCost(record));
  }

  /** This hop's descendant usage: the legacy evicted baseline, its eviction records, and the direct children's subtree totals. */
  descendantUsage(): CumulativeCost { return cloneCost(this.directTotal); }

  acceptUsage(source: unknown): void {
    if (!source || typeof source !== "object" || this.acceptedObjects.has(source as object)) return;
    this.acceptedObjects.add(source as object);
    const entry = source as { type?: string; role?: string; message?: { role?: string; usage?: unknown }; usage?: unknown };
    const role = entry.role ?? entry.message?.role;
    const usageValue = entry.message?.usage ?? entry.usage;
    const assistant = role === "assistant";
    const toolResult = role === "toolResult";
    const summary = entry.type === "compaction" || entry.type === "branch_summary";
    if ((!assistant && !toolResult && !summary) || !usageValue || typeof usageValue !== "object") {
      if (assistant) this.lead.cost.unknownContributors = 1;
      return;
    }
    const usage = usageValue as { input?: unknown; output?: unknown; cacheRead?: unknown; cacheWrite?: unknown; cost?: { total?: unknown } };
    const input = nonnegative(usage.input) ?? 0, output = nonnegative(usage.output) ?? 0, cacheRead = nonnegative(usage.cacheRead) ?? 0, cacheWrite = nonnegative(usage.cacheWrite) ?? 0;
    this.lead.input += input; this.lead.output += output; this.lead.cacheRead += cacheRead; this.lead.cacheWrite += cacheWrite;
    if (assistant) {
      const prompt = input + cacheRead + cacheWrite;
      this.lead.latestCacheHitRate = prompt > 0 ? cacheRead / prompt * 100 : undefined;
    }
    const cost = nonnegative(usage.cost?.total);
    if (cost === undefined) this.lead.cost.unknownContributors = 1;
    else { this.lead.cost.knownUsd += cost; this.lead.cost.knownContributors = 1; }
    this.lead.cost.descendants = this.lead.cost.knownContributors || this.lead.cost.unknownContributors ? 1 : 0;
  }

  presentation(): { lead: LeadUsageSummary; leadCost: string; directCost: string } {
    return {
      lead: { ...this.lead, cost: cloneCost(this.lead.cost) },
      leadCost: formatCumulativeCost(this.lead.cost),
      directCost: formatCumulativeCost(this.directTotal),
    };
  }

  /** Prunes first, then writes the checkpoint. */
  persist(omit: ReadonlySet<string> = new Set()): boolean {
    // `pruneAndRecompute` bounds the serialized identity set to the live registry.
    this.pruneAndRecompute(omit);
    const snapshot = this.snapshot();
    const written = writeCheckpoint(this.storage, snapshot);
    if (!written) retainFailedCheckpoint(this.storage, snapshot);
    return written;
  }

  private snapshot(): CostCheckpoint {
    return {
      version: CHECKPOINT_VERSION,
      lead: { ...this.lead, cost: cloneCost(this.lead.cost) },
      evictedBaseline: cloneCost(this.evictedBaseline),
      agents: [...this.agents].map(([agentId, cost]) => ({ agentId, cost: cloneCost(cost) })),
    };
  }

  private recomputeDirectTotal(): void {
    const total = cloneCost(this.evictedBaseline);
    for (const cost of this.evictionRecords.values()) addCost(total, cost);
    for (const cost of this.agents.values()) addCost(total, cost);
    this.directTotal = total;
  }
}

const registryStorage = new WeakMap<RpcAgentRegistry, AgentStorageContext>();
const registryEstimates = new WeakMap<RpcAgentRegistry, CostEstimateState>();
export function registerAgentCostOwner(registry: RpcAgentRegistry, storage: AgentStorageContext | undefined): void {
  if (storage) registryStorage.set(registry, storage);
}

/** The registry's estimate, loading its owner checkpoint on first use. Every hop that owns children has one, footer or not. */
function costEstimateFor(registry: RpcAgentRegistry, storage = registryStorage.get(registry)): CostEstimateState | undefined {
  const existing = registryEstimates.get(registry);
  if (existing && (!storage || checkpointKey(existing.storage) === checkpointKey(storage))) return existing;
  if (!storage) return undefined;
  const state = new CostEstimateState(storage, registry, loadCheckpoint(storage).checkpoint);
  registryEstimates.set(registry, state);
  return state;
}

/**
 * The footer's estimate for `registry`, registered as the registry's estimate
 * (the footer controller's half of the attach/release pair). Reuses an
 * estimate this hop already built for the same owner (an eviction or a
 * descendant report can precede the footer mount); a second one would
 * diverge. With no owner checkpoint on disk and a session that already has
 * entries, the lead's own cost is marked incomplete.
 */
export function attachFooterCostEstimate(registry: RpcAgentRegistry, storage: AgentStorageContext, sessionHasEntries: () => boolean): CostEstimateState {
  registerAgentCostOwner(registry, storage);
  const loaded = loadCheckpoint(storage);
  const existing = registryEstimates.get(registry);
  const state = existing && checkpointKey(existing.storage) === checkpointKey(storage) ? existing : new CostEstimateState(storage, registry, loaded.checkpoint);
  if (!loaded.found && sessionHasEntries()) {
    state.lead.cost.unknownContributors = 1;
    state.lead.cost.descendants = 1;
  }
  registryEstimates.set(registry, state);
  return state;
}

/** Unregisters the footer's estimate on footer stop, unless another estimate has replaced it since. */
export function releaseFooterCostEstimate(registry: RpcAgentRegistry, state: CostEstimateState): void {
  if (registryEstimates.get(registry) === state) registryEstimates.delete(registry);
}

/**
 * This hop's descendant usage, the value it reports to its parent: the
 * subtree totals of its registry-resident direct children, dormant ones
 * included, plus its removed children (the checkpoint's legacy evicted
 * baseline and its eviction records). After a restart the sum is rebuilt from
 * the revived records' durable usage, the checkpoint, and the records.
 * Reads no session file. Undefined without an owner storage. Prunes first, so
 * it may drop a removed direct child from `registry` (see the module header).
 */
export function descendantUsageValue(registry: RpcAgentRegistry): CumulativeCost | undefined {
  const state = costEstimateFor(registry);
  if (!state) return undefined;
  state.pruneAndRecompute();
  return state.descendantUsage();
}

/** Persists the registry's cached estimate at an explicit lifecycle boundary. Prunes first. */
export function persistAgentCostCheckpoint(registry: RpcAgentRegistry): boolean {
  return registryEstimates.get(registry)?.persist() ?? true;
}

/**
 * Capacity eviction's record value for an owned direct child, passed to
 * `removeOwnedAgentHome` so the record is written under the removal lock:
 * the in-memory tracked floor merged with the child's subtree cost. Prunes
 * first when an estimate exists.
 */
export function capacityEvictionCost(registry: RpcAgentRegistry, record: RpcAgentRecord): CumulativeCost {
  const state = costEstimateFor(registry, registryStorage.get(registry) ?? storageFromRecord(record));
  return state ? state.evictionCost(record) : recordSubtreeCost(record);
}

/**
 * Capacity eviction's cost step, after any owned home removal and before the
 * registry drop. An owned record's removal already wrote its eviction record
 * (`capacityEvictionCost`), so nothing is written again; an unowned record
 * has no home, so its owner writes the record here. The legacy evicted
 * baseline is never increased. False keeps the record for a retry. Prunes
 * after the write, so the new record is pruned against in the same call.
 */
export function persistEvictedAgentCost(registry: RpcAgentRegistry, record: RpcAgentRecord): boolean {
  if (!safePart(record.agentId)) return false;
  const storage = registryStorage.get(registry) ?? storageFromRecord(record);
  if (!storage) return true;
  registryStorage.set(registry, storage);
  const state = costEstimateFor(registry, storage)!;
  if (!record.ownership && !writeEvictionRecord(storage, record.agentId, state.evictionCost(record))) {
    console.error(`ws-pi-agent: eviction record write failed for owner ${storage.ownerSessionId}; the agent was retained`);
    return false;
  }
  state.pruneAndRecompute();
  return true;
}

/**
 * Retention's record value for a direct owned child (see
 * `OwnedHomeRemovalOptions.evictionCost`): the owner checkpoint's tracked
 * floor merged with the child's subtree cost (own plus stored descendant
 * usage). Read-only: retention never writes the checkpoint.
 */
export function retentionEvictionCost(metadata: OwnershipMetadata): CumulativeCost | undefined {
  const storage = ownerStorageOf(metadata);
  if (!storage || !safePart(metadata.agentId)) return undefined;
  const tracked = loadCheckpoint(storage).checkpoint.agents.find(agent => agent.agentId === metadata.agentId);
  return mergeAgentCost(tracked?.cost, subtreeCost(metadata.telemetry, durableDescendantUsage(metadata)));
}

/**
 * The relaunch and rehydrate refusal: true when an eviction record marks
 * `agentId` removed, under the descriptor's own owner or this hop's owner
 * storage. Record only and fail-closed, and it works from `agentId` alone
 * (no descriptor needed). A removed agentId is never rehydrated or
 * relaunched: its relaunched session would start empty, so its cost would be
 * lost or its identity conflated.
 *
 * Call sites: `sendToAgent`'s unknown-id message and dormant-relaunch gate
 * (spawner.ts) and fork-thread rehydration (ask.ts). There "removed, or is
 * removing" must refuse even under a held removal claim, so neither
 * claim-aware predicate fits: `ownedHomeRemovalState` answers `"claimed"`
 * and `isOwnedHomeGone` false while a removal is in flight, and both need
 * a descriptor the unknown-id path does not have.
 */
export function isRemovedAgent(registry: RpcAgentRegistry, agentId: string, ownership?: Pick<AgentOwnership, "home" | "ownerSessionId" | "agentId">): boolean {
  if (ownership && hasEvictionRecord(ownership)) return true;
  const storage = registryStorage.get(registry);
  return !!storage && safePart(agentId) && readEvictionRecord(storage, agentId) !== undefined;
}
