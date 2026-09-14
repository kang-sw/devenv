/** Theme-aware replacement for Pi's built-in footer with bounded cost estimates. */
import { homedir } from "node:os";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { readOwnerArtifacts, writeOwnerArtifact, type AgentStorageContext, type OwnershipMetadata } from "./agent-storage.ts";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";
import type { AgentTelemetry } from "./agent-telemetry.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";

const CHECKPOINT_BUCKET = ".cost-estimate";
const CHECKPOINT_FILE = "checkpoint.json";
const CHECKPOINT_VERSION = 1;
const retainedFailedCheckpoints = new Map<string, CostCheckpoint>();

export interface CumulativeCost {
  knownUsd: number;
  knownContributors: number;
  unknownContributors: number;
  descendants: number;
}

const emptyCost = (): CumulativeCost => ({ knownUsd: 0, knownContributors: 0, unknownContributors: 0, descendants: 0 });
function addCost(target: CumulativeCost, source: CumulativeCost): void {
  target.knownUsd += source.knownUsd;
  target.knownContributors += source.knownContributors;
  target.unknownContributors += source.unknownContributors;
  target.descendants += source.descendants;
}
function cloneCost(value: CumulativeCost): CumulativeCost { return { ...value }; }
function telemetryCost(value: AgentTelemetry | undefined): CumulativeCost {
  if (value?.estimatedUsd !== undefined) return { knownUsd: value.estimatedUsd, knownContributors: 1, unknownContributors: 0, descendants: 1 };
  if (value?.partialEstimatedUsd !== undefined) return { knownUsd: value.partialEstimatedUsd, knownContributors: 1, unknownContributors: 1, descendants: 1 };
  return { knownUsd: 0, knownContributors: 0, unknownContributors: 1, descendants: 1 };
}

/** Monotonic merge for one directly tracked agent's cumulative telemetry. */
function mergeAgentCost(previous: CumulativeCost | undefined, observed: CumulativeCost): CumulativeCost {
  if (!previous) return cloneCost(observed);
  const regressed = observed.knownUsd < previous.knownUsd;
  if (!regressed && observed.knownContributors > 0) return cloneCost(observed);
  return {
    knownUsd: previous.knownUsd,
    knownContributors: previous.knownContributors,
    unknownContributors: 1,
    descendants: 1,
  };
}

export function formatCumulativeCost(cost: CumulativeCost): string {
  if (cost.knownContributors === 0 && cost.unknownContributors > 0) return "—";
  const known = `~$${cost.knownUsd.toFixed(2)}`;
  return cost.unknownContributors > 0 ? `${known} + ?` : known;
}

export interface LeadUsageSummary {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  latestCacheHitRate?: number;
  cost: CumulativeCost;
}

interface StoredAgentCost { agentId: string; cost: CumulativeCost }
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
const nonnegativeInteger = (value: unknown): number | undefined => {
  const number = nonnegative(value);
  return number !== undefined && Number.isSafeInteger(number) ? number : undefined;
};
function safePart(value: string): boolean { return /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(value); }
function parseCost(value: unknown): CumulativeCost | undefined {
  const cost = value as Partial<CumulativeCost> | null;
  if (!cost) return undefined;
  const knownUsd = nonnegative(cost.knownUsd);
  const knownContributors = nonnegativeInteger(cost.knownContributors);
  const unknownContributors = nonnegativeInteger(cost.unknownContributors);
  const descendants = nonnegativeInteger(cost.descendants);
  return knownUsd === undefined || knownContributors === undefined || unknownContributors === undefined || descendants === undefined
    ? undefined : { knownUsd, knownContributors, unknownContributors, descendants };
}
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
  const home = record.ownership?.home;
  if (!home) return undefined;
  const owner = dirname(home), namespace = dirname(owner), root = dirname(namespace);
  if (basename(namespace) !== "ws-agents" || basename(owner) !== record.ownership!.ownerSessionId) return undefined;
  return { root, ownerSessionId: record.ownership!.ownerSessionId };
}

class CostEstimateState {
  readonly storage: AgentStorageContext;
  readonly registry: RpcAgentRegistry;
  readonly lead: LeadUsageSummary;
  readonly evictedBaseline: CumulativeCost;
  readonly agents: Map<string, CumulativeCost>;
  private directTotal = emptyCost();
  private acceptedObjects = new WeakSet<object>();

  constructor(storage: AgentStorageContext, registry: RpcAgentRegistry, checkpoint: CostCheckpoint) {
    this.storage = storage;
    this.registry = registry;
    this.lead = { ...checkpoint.lead, cost: cloneCost(checkpoint.lead.cost) };
    this.evictedBaseline = cloneCost(checkpoint.evictedBaseline);
    this.agents = new Map(checkpoint.agents.map(agent => [agent.agentId, cloneCost(agent.cost)]));
    this.reconcile();
  }

  reconcile(omit: ReadonlySet<string> = new Set()): void {
    for (const [agentId, record] of this.registry) {
      if (omit.has(agentId)) continue;
      this.agents.set(agentId, mergeAgentCost(this.agents.get(agentId), telemetryCost(record.telemetry)));
    }
    this.recomputeDirectTotal();
  }

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

  foldAndPersist(record: RpcAgentRecord): boolean {
    this.reconcile();
    const stable = this.snapshot();
    const cost = mergeAgentCost(this.agents.get(record.agentId), telemetryCost(record.telemetry));
    addCost(this.evictedBaseline, cost);
    this.agents.delete(record.agentId);
    this.recomputeDirectTotal();
    if (this.persist(new Set([record.agentId]))) return true;
    this.restore(stable);
    retainFailedCheckpoint(this.storage, stable);
    return false;
  }

  presentation(): { lead: LeadUsageSummary; leadCost: string; directCost: string } {
    return {
      lead: { ...this.lead, cost: cloneCost(this.lead.cost) },
      leadCost: formatCumulativeCost(this.lead.cost),
      directCost: formatCumulativeCost(this.directTotal),
    };
  }

  persist(omit: ReadonlySet<string> = new Set()): boolean {
    this.reconcile(omit);
    const stable = this.snapshot();
    const activeIds = new Set([...this.registry.keys()].filter(id => !omit.has(id)));
    for (const [agentId, cost] of [...this.agents]) {
      if (activeIds.has(agentId)) continue;
      addCost(this.evictedBaseline, cost);
      this.agents.delete(agentId);
    }
    // The serialized identity set can never exceed the live registry it snapshots.
    while (this.agents.size > activeIds.size) {
      const first = this.agents.entries().next().value as [string, CumulativeCost] | undefined;
      if (!first) break;
      addCost(this.evictedBaseline, first[1]); this.agents.delete(first[0]);
    }
    this.recomputeDirectTotal();
    const written = writeCheckpoint(this.storage, this.snapshot());
    if (!written) {
      this.restore(stable);
      retainFailedCheckpoint(this.storage, stable);
    }
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

  private restore(checkpoint: CostCheckpoint): void {
    Object.assign(this.lead, checkpoint.lead, { cost: cloneCost(checkpoint.lead.cost) });
    if (checkpoint.lead.latestCacheHitRate === undefined) delete this.lead.latestCacheHitRate;
    Object.assign(this.evictedBaseline, checkpoint.evictedBaseline);
    this.agents.clear();
    for (const agent of checkpoint.agents) this.agents.set(agent.agentId, cloneCost(agent.cost));
    this.recomputeDirectTotal();
  }

  private recomputeDirectTotal(): void {
    const total = cloneCost(this.evictedBaseline);
    for (const cost of this.agents.values()) addCost(total, cost);
    this.directTotal = total;
  }
}

const registryStorage = new WeakMap<RpcAgentRegistry, AgentStorageContext>();
const registryEstimates = new WeakMap<RpcAgentRegistry, CostEstimateState>();
const foldedAgentRecords = new WeakSet<object>();
export function registerAgentCostOwner(registry: RpcAgentRegistry, storage: AgentStorageContext | undefined): void {
  if (storage) registryStorage.set(registry, storage);
}

/** Persists the mounted registry's cached estimate at an explicit lifecycle boundary. */
export function persistAgentCostCheckpoint(registry: RpcAgentRegistry): boolean {
  return registryEstimates.get(registry)?.persist() ?? true;
}

/** Folds one evicted direct record into the scalar baseline before registry/home removal. */
export function persistEvictedAgentCost(registry: RpcAgentRegistry, record: RpcAgentRecord): boolean {
  if (foldedAgentRecords.has(record)) return true;
  const storage = registryStorage.get(registry) ?? storageFromRecord(record);
  if (!safePart(record.agentId)) return false;
  if (!storage) { foldedAgentRecords.add(record); return true; }
  registryStorage.set(registry, storage);
  const state = registryEstimates.get(registry) ?? new CostEstimateState(storage, registry, loadCheckpoint(storage).checkpoint);
  const written = state.foldAndPersist(record);
  if (written) foldedAgentRecords.add(record);
  return written;
}

/** Retention folds only this direct owned record; it never discovers descendants. */
export function persistOwnedTelemetryRollup(metadata: OwnershipMetadata): boolean {
  const storage = storageFromRecord({ ownership: metadata } as Pick<RpcAgentRecord, "ownership">);
  if (!storage || !safePart(metadata.agentId)) return false;
  const { checkpoint } = loadCheckpoint(storage);
  const agents = new Map(checkpoint.agents.map(agent => [agent.agentId, cloneCost(agent.cost)]));
  const observed = mergeAgentCost(agents.get(metadata.agentId), telemetryCost(metadata.telemetry));
  addCost(checkpoint.evictedBaseline, observed);
  agents.delete(metadata.agentId);
  checkpoint.agents = [...agents].map(([agentId, cost]) => ({ agentId, cost }));
  return writeCheckpoint(storage, checkpoint);
}

export interface FooterPrimitives {
  visibleWidth(text: string): number;
  truncateToWidth(text: string, width: number, ellipsis?: string): string;
}
interface FooterTheme { fg(color: "text" | "dim" | "accent" | "warning" | "error", text: string): string }
interface FooterData {
  getGitBranch(): string | null;
  getExtensionStatuses(): ReadonlyMap<string, string>;
  getAvailableProviderCount?(): number;
  onBranchChange(callback: () => void): () => void;
}
interface FooterTui { requestRender(): void }
export interface AgentFooterComponent { render(width: number): string[]; invalidate(): void; dispose?(): void }
export interface AgentFooterContext {
  cwd: string;
  model?: { provider?: string; id?: string; reasoning?: boolean; contextWindow?: number };
  thinkingLevel?: string;
  sessionManager: { getEntries(): readonly unknown[]; getSessionName?(): string | undefined; getCwd?(): string };
  getContextUsage?(): { tokens?: number | null; percent?: number | null; contextWindow?: number } | undefined;
  ui: { setFooter(factory: ((tui: FooterTui, theme: FooterTheme, data: FooterData) => AgentFooterComponent) | undefined): void };
}
export interface AgentFooterController {
  refresh(): void;
  refreshAgents(): void;
  acceptUsage(source: unknown): void;
  checkpoint(): boolean;
  stop(): void;
}
export interface AgentFooterSessionLifecycle {
  start(role: SpawnRole | undefined, ctx: AgentFooterContext & { mode?: string }, registry: RpcAgentRegistry, storage: AgentStorageContext): Promise<void>;
  refresh(): void;
  refreshAgents(): void;
  acceptUsage(source: unknown): void;
  checkpoint(): void;
  stop(): void;
}

/** Owns replacement/reload/mode-transition/shutdown semantics for index.ts. */
export function createAgentFooterSessionLifecycle(
  loadPrimitives: () => Promise<FooterPrimitives>,
  createController: typeof createAgentFooterController = createAgentFooterController,
): AgentFooterSessionLifecycle {
  let current: AgentFooterController | undefined;
  let generation = 0;
  return {
    async start(role, ctx, registry, storage) {
      const ownGeneration = ++generation;
      current?.stop(); current = undefined;
      if (!shouldArmAgentFooter(role, ctx.mode)) return;
      const primitives = await loadPrimitives();
      if (ownGeneration !== generation) return;
      current = createController(ctx, registry, storage, primitives);
    },
    refresh() { current?.refresh(); },
    refreshAgents() { current?.refreshAgents(); },
    acceptUsage(source) { current?.acceptUsage(source); },
    checkpoint() { current?.checkpoint(); },
    stop() { generation += 1; current?.stop(); current = undefined; },
  };
}

export function shouldArmAgentFooter(role: SpawnRole | undefined, mode: string | undefined): boolean {
  return isLeadOrFork(role) && mode === "tui";
}
function formatTokens(count: number): string {
  if (count < 1000) return String(count);
  if (count < 10_000) return `${(count / 1000).toFixed(1)}k`;
  if (count < 1_000_000) return `${Math.round(count / 1000)}k`;
  if (count < 10_000_000) return `${(count / 1_000_000).toFixed(1)}M`;
  return `${Math.round(count / 1_000_000)}M`;
}
function displayCwd(cwd: string): string {
  const home = homedir();
  const rel = relative(home, resolve(cwd));
  return rel === "" ? "~" : rel !== ".." && !rel.startsWith(`..${sep}`) ? `~${sep}${rel}` : cwd;
}
function sanitizeStatus(text: string): string { return text.replace(/[\r\n\t]/g, " ").replace(/ +/g, " ").trim(); }
function styleStats(plain: string, values: readonly string[], contextUsedPart: string, contextPart: string, contextPercent: number | null | undefined, theme: FooterTheme): string {
  const ranges: Array<{ start: number; text: string; color: "dim" | "accent" | "warning" | "error" }> = [];
  let searchAt = 0;
  for (const value of values) {
    const money = /^~\$\d+\.\d{2}/.exec(value)?.[0];
    if (!money) continue;
    const start = plain.indexOf(money, searchAt);
    if (start < 0) continue;
    ranges.push({ start, text: money, color: "accent" }); searchAt = start + money.length;
  }
  const contextStart = plain.indexOf(contextPart);
  if (contextStart >= 0) ranges.push({ start: contextStart, text: contextUsedPart, color: contextPercent != null && contextPercent > 70 ? "error" : "text" });
  ranges.sort((a, b) => a.start - b.start);
  let at = 0, styled = "";
  for (const range of ranges) {
    if (range.start < at) continue;
    styled += theme.fg("dim", plain.slice(at, range.start)); styled += theme.fg(range.color, range.text); at = range.start + range.text.length;
  }
  return styled + theme.fg("dim", plain.slice(at));
}

export function createAgentFooterController(
  ctx: AgentFooterContext,
  registry: RpcAgentRegistry,
  storage: AgentStorageContext,
  primitives: FooterPrimitives,
): AgentFooterController {
  registerAgentCostOwner(registry, storage);
  const loaded = loadCheckpoint(storage);
  const state = new CostEstimateState(storage, registry, loaded.checkpoint);
  if (!loaded.found && ctx.sessionManager.getEntries().length > 0) {
    state.lead.cost.unknownContributors = 1;
    state.lead.cost.descendants = 1;
  }
  registryEstimates.set(registry, state);
  let presentation = state.presentation();
  let renderRequest: (() => void) | undefined;
  let componentDispose: (() => void) | undefined;
  let stopped = false;
  const updatePresentation = () => { presentation = state.presentation(); if (!stopped) renderRequest?.(); };

  ctx.ui.setFooter((tui, theme, footerData) => {
    renderRequest = () => tui.requestRender();
    const unbranch = footerData.onBranchChange(renderRequest);
    let disposed = false;
    componentDispose = () => {
      if (disposed) return;
      disposed = true; unbranch();
      if (renderRequest) renderRequest = undefined;
      componentDispose = undefined;
    };
    return {
      invalidate() { /* Theme is read from the host callback on every render. */ },
      dispose: componentDispose,
      render(width: number): string[] {
        if (width <= 0) return [""];
        const usage = presentation.lead;
        const leadCost = presentation.leadCost, directCost = presentation.directCost;
        const tokenParts = [
          usage.input ? `↑${formatTokens(usage.input)}` : undefined,
          usage.output ? `↓${formatTokens(usage.output)}` : undefined,
          usage.cacheRead ? `R${formatTokens(usage.cacheRead)}` : undefined,
          usage.cacheWrite ? `W${formatTokens(usage.cacheWrite)}` : undefined,
          (usage.cacheRead || usage.cacheWrite) && usage.latestCacheHitRate !== undefined ? `CH${usage.latestCacheHitRate.toFixed(1)}%` : undefined,
        ].filter((part): part is string => !!part);
        const context = ctx.getContextUsage?.();
        const window = context?.contextWindow ?? ctx.model?.contextWindow ?? 0;
        const contextUsedPart = context?.tokens == null ? "?" : formatTokens(context.tokens);
        const contextPart = `${contextUsedPart}/${formatTokens(window)}`;
        const required = [`L ${leadCost} + D ${directCost}`];
        let optional = [...tokenParts, contextPart];
        const modelId = ctx.model?.id ?? "no-model";
        let model = ctx.model?.reasoning ? `${modelId} • ${ctx.thinkingLevel ?? "off"}` : modelId;
        if ((footerData.getAvailableProviderCount?.() ?? 1) > 1 && ctx.model?.provider) model = `(${ctx.model.provider}) ${model}`;
        const buildLeft = () => [...optional, ...required].join(" ");
        while (optional.length > 0 && primitives.visibleWidth(buildLeft()) + 2 + primitives.visibleWidth(model) > width) optional.shift();
        let left = buildLeft();
        if (primitives.visibleWidth(left) + 2 + primitives.visibleWidth(model) > width) model = "";
        if (primitives.visibleWidth(left) > width) left = primitives.truncateToWidth(left, width, "...");
        const padding = model ? " ".repeat(Math.max(2, width - primitives.visibleWidth(left) - primitives.visibleWidth(model))) : "";
        let statsPlain = left + padding + model;
        if (primitives.visibleWidth(statsPlain) > width) statsPlain = primitives.truncateToWidth(statsPlain, width, "");
        const stats = styleStats(statsPlain, [leadCost, directCost], contextUsedPart, contextPart, context?.percent, theme);

        let path = displayCwd(ctx.sessionManager.getCwd?.() ?? ctx.cwd);
        const branch = footerData.getGitBranch(); if (branch) path += ` (${branch})`;
        const name = ctx.sessionManager.getSessionName?.(); if (name) path += ` • ${name}`;
        const lines = [primitives.truncateToWidth(theme.fg("dim", path), width, theme.fg("dim", "...")), stats];
        const statuses = [...footerData.getExtensionStatuses().entries()].sort(([a], [b]) => a.localeCompare(b)).map(([, text]) => sanitizeStatus(text));
        if (statuses.length) lines.push(primitives.truncateToWidth(statuses.join(" "), width, theme.fg("dim", "...")));
        return lines;
      },
    };
  });

  return {
    refresh() { if (!stopped) renderRequest?.(); },
    refreshAgents() { if (stopped) return; state.reconcile(); updatePresentation(); },
    acceptUsage(source) { if (stopped) return; state.acceptUsage(source); updatePresentation(); },
    checkpoint() { return stopped ? true : state.persist(); },
    stop() {
      if (stopped) return;
      state.persist(); stopped = true;
      ctx.ui.setFooter(undefined); componentDispose?.(); renderRequest = undefined;
      if (registryEstimates.get(registry) === state) registryEstimates.delete(registry);
    },
  };
}
