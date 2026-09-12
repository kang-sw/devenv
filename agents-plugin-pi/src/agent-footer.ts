/** Theme-aware replacement for Pi's built-in footer with descendant cost telemetry. */
import { existsSync, lstatSync, readdirSync, realpathSync, statSync, watch, type FSWatcher } from "node:fs";
import { homedir } from "node:os";
import { basename, dirname, join, relative, resolve, sep } from "node:path";
import { readSessionEntries, refreshTelemetry, type AgentTelemetry } from "./agent-telemetry.ts";
import { readOwnerArtifacts, readOwnership, writeOwnerArtifact, type AgentStorageContext, type OwnershipMetadata } from "./agent-storage.ts";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner.ts";

const ROLLUP_DIR = ".cost-rollup";
const ROLLUP_VERSION = 1;

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
function telemetryCost(value: AgentTelemetry | undefined): CumulativeCost {
  if (value?.estimatedUsd !== undefined) return { knownUsd: value.estimatedUsd, knownContributors: 1, unknownContributors: 0, descendants: 1 };
  if (value?.partialEstimatedUsd !== undefined) return { knownUsd: value.partialEstimatedUsd, knownContributors: 1, unknownContributors: 1, descendants: 1 };
  return { knownUsd: 0, knownContributors: 0, unknownContributors: 1, descendants: 1 };
}

export function formatCumulativeCost(cost: CumulativeCost): string {
  if (cost.knownContributors === 0 && cost.unknownContributors > 0) return "—";
  const known = `~$${cost.knownUsd.toFixed(2)}`;
  return cost.unknownContributors > 0 ? `${known} + ?` : known;
}

interface RollupEntry extends CumulativeCost { version: 1; agentId: string }
function safePart(value: string): boolean { return /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(value); }
function ownerRoot(storage: AgentStorageContext, ownerSessionId = storage.ownerSessionId): string {
  return join(storage.root, "ws-agents", ownerSessionId);
}
function rollupDirectory(storage: AgentStorageContext, ownerSessionId = storage.ownerSessionId): string {
  return join(ownerRoot(storage, ownerSessionId), ROLLUP_DIR);
}
function parseRollup(value: unknown): RollupEntry | undefined {
  const entry = value as Partial<RollupEntry> | null;
  if (!entry || entry.version !== 1 || typeof entry.agentId !== "string" || !safePart(entry.agentId)) return undefined;
  if (![entry.knownUsd, entry.knownContributors, entry.unknownContributors, entry.descendants].every(number => typeof number === "number" && Number.isFinite(number) && number >= 0)) return undefined;
  if (![entry.knownContributors, entry.unknownContributors, entry.descendants].every(Number.isSafeInteger)) return undefined;
  return entry as RollupEntry;
}
function readRollups(storage: AgentStorageContext, ownerSessionId: string): Map<string, RollupEntry> {
  const entries = new Map<string, RollupEntry>();
  const scoped = { ...storage, ownerSessionId };
  for (const { name, content } of readOwnerArtifacts(scoped, ROLLUP_DIR)) {
    if (!name.endsWith(".json")) continue;
    try {
      const parsed = parseRollup(JSON.parse(content) as unknown);
      if (parsed && name === `${parsed.agentId}.json`) entries.set(parsed.agentId, parsed);
    } catch { /* one corrupt roll-up never hides its siblings */ }
  }
  return entries;
}
function sessionIdFor(metadata: Pick<OwnershipMetadata, "sessionPath" | "telemetry"> | RpcAgentRecord): string | undefined {
  if (metadata.telemetry?.origin.sessionId) return metadata.telemetry.origin.sessionId;
  if (!metadata.sessionPath) return undefined;
  const read = readSessionEntries(metadata.sessionPath);
  return read && !("transient" in read) ? read.headerId : undefined;
}
function validOwnedMetadata(metadata: OwnershipMetadata | undefined, ownerSessionId: string, agentId: string): metadata is OwnershipMetadata {
  return !!metadata && metadata.ownerSessionId === ownerSessionId && metadata.agentId === agentId && basename(metadata.home) === agentId && basename(dirname(metadata.home)) === ownerSessionId;
}

interface AggregateOptions {
  direct?: RpcAgentRegistry;
  watchPaths?: Set<string>;
  visitedOwners?: Set<string>;
}
function aggregateOwner(storage: AgentStorageContext, ownerSessionId: string, options: AggregateOptions): CumulativeCost {
  const result = emptyCost();
  const visited = options.visitedOwners ?? new Set<string>();
  if (!safePart(ownerSessionId) || visited.has(ownerSessionId)) return result;
  visited.add(ownerSessionId);
  const root = ownerRoot(storage, ownerSessionId);
  options.watchPaths?.add(storage.root);
  options.watchPaths?.add(join(storage.root, "ws-agents"));
  options.watchPaths?.add(root);
  const rollups = readRollups(storage, ownerSessionId);
  options.watchPaths?.add(rollupDirectory(storage, ownerSessionId));
  let names: string[] = [];
  try { names = readdirSyncless(root); } catch { /* an owner with only a not-yet-created namespace has no descendants yet */ }
  const ids = new Set(names.filter(name => !name.startsWith(".") && safePart(name)));
  if (options.direct) for (const id of options.direct.keys()) ids.add(id);
  const accounted = new Set<string>();
  for (const agentId of ids) {
    const direct = options.direct?.get(agentId);
    const home = direct?.ownership?.home ?? join(root, agentId);
    options.watchPaths?.add(home);
    const metadata = readOwnership(home);
    if (!direct && !validOwnedMetadata(metadata, ownerSessionId, agentId)) {
      const rolled = rollups.get(agentId);
      if (rolled) { addCost(result, rolled); accounted.add(agentId); }
      continue;
    }
    const own = telemetryCost(direct?.telemetry ?? metadata?.telemetry);
    addCost(result, own);
    accounted.add(agentId);
    const childSessionId = sessionIdFor(direct ?? metadata!);
    if (childSessionId) addCost(result, aggregateOwner(storage, childSessionId, { watchPaths: options.watchPaths, visitedOwners: visited }));
  }
  for (const [agentId, rolled] of rollups) if (!accounted.has(agentId)) addCost(result, rolled);
  return result;
}
function readdirSyncless(path: string): string[] { return readdirSync(path, { withFileTypes: true }).filter(entry => entry.isDirectory()).map(entry => entry.name); }

/** Reconstructs all descendants reachable from this session's exact ownership namespace. */
export function aggregateDescendantCosts(storage: AgentStorageContext, registry: RpcAgentRegistry): CumulativeCost {
  const total = aggregateOwner(storage, storage.ownerSessionId, { direct: registry, visitedOwners: new Set() });
  for (const [agentId, rolled] of registryRollups.get(registry) ?? []) if (!registry.has(agentId)) addCost(total, rolled);
  return total;
}

const registryStorage = new WeakMap<RpcAgentRegistry, AgentStorageContext>();
const registryRollups = new WeakMap<RpcAgentRegistry, Map<string, CumulativeCost>>();
export function registerAgentCostOwner(registry: RpcAgentRegistry, storage: AgentStorageContext | undefined): void {
  if (storage) registryStorage.set(registry, storage);
}
function storageFromRecord(record: RpcAgentRecord): AgentStorageContext | undefined {
  const home = record.ownership?.home;
  if (!home) return undefined;
  const owner = dirname(home), namespace = dirname(owner), root = dirname(namespace);
  if (basename(namespace) !== "ws-agents" || basename(owner) !== record.ownership!.ownerSessionId) return undefined;
  return { root, ownerSessionId: record.ownership!.ownerSessionId };
}

/** Upserts one identity-keyed durable roll-up before registry/home removal. */
export function persistEvictedAgentCost(registry: RpcAgentRegistry, record: RpcAgentRecord): boolean {
  const storage = registryStorage.get(registry) ?? storageFromRecord(record);
  if (!safePart(record.agentId)) return false;
  const durable = record.ownership ? readOwnership(record.ownership.home)?.telemetry : undefined;
  const total = telemetryCost(record.telemetry ?? durable);
  const childSessionId = sessionIdFor(record);
  if (storage && childSessionId) addCost(total, aggregateOwner(storage, childSessionId, { visitedOwners: new Set([storage.ownerSessionId]) }));
  if (!storage) {
    let rollups = registryRollups.get(registry);
    if (!rollups) { rollups = new Map(); registryRollups.set(registry, rollups); }
    rollups.set(record.agentId, total);
    return true;
  }
  const prior = readRollups(storage, storage.ownerSessionId).get(record.agentId);
  if (prior) {
    total.knownUsd = Math.max(total.knownUsd, prior.knownUsd);
    total.knownContributors = Math.max(total.knownContributors, prior.knownContributors);
    total.unknownContributors = Math.max(total.unknownContributors, prior.unknownContributors);
    total.descendants = Math.max(total.descendants, prior.descendants);
  }
  const entry: RollupEntry = { version: ROLLUP_VERSION, agentId: record.agentId, ...total };
  return writeOwnerArtifact(storage, ROLLUP_DIR, `${record.agentId}.json`, `${JSON.stringify(entry, null, 2)}\n`);
}

/** Retention uses the same identity-keyed roll-up before detaching an owned home. */
export function persistOwnedTelemetryRollup(metadata: OwnershipMetadata): boolean {
  const telemetry = metadata.telemetry ? refreshTelemetry(metadata.telemetry) ?? metadata.telemetry : undefined;
  const record = { agentId: metadata.agentId, sessionPath: metadata.sessionPath ?? "", ownership: metadata, telemetry } as RpcAgentRecord;
  return persistEvictedAgentCost(new Map(), record);
}

function aggregateWithWatchPaths(storage: AgentStorageContext, registry: RpcAgentRegistry): { cost: CumulativeCost; paths: Set<string> } {
  const paths = new Set<string>();
  const cost = aggregateOwner(storage, storage.ownerSessionId, { direct: registry, watchPaths: paths, visitedOwners: new Set() });
  return { cost, paths };
}
function existingDirectories(paths: Iterable<string>): string[] {
  const out: string[] = [];
  for (const path of paths) {
    try { if (existsSync(path) && statSync(path).isDirectory() && !lstatSync(path).isSymbolicLink() && realpathSync(path) === resolve(path)) out.push(path); } catch { /* raced with retention */ }
  }
  return out;
}
export function watchDescendantCosts(storage: AgentStorageContext, registry: RpcAgentRegistry, onChange: () => void): () => void {
  const watchers = new Map<string, FSWatcher>();
  let stopped = false;
  let scheduled: ReturnType<typeof setTimeout> | undefined;
  let fingerprint = JSON.stringify(aggregateDescendantCosts(storage, registry));
  const reconcile = () => {
    if (stopped) return;
    const snapshot = aggregateWithWatchPaths(storage, registry);
    const next = JSON.stringify(snapshot.cost);
    for (const path of existingDirectories(snapshot.paths)) {
      if (watchers.has(path)) continue;
      try { watchers.set(path, watch(path, { persistent: false }, schedule)); } catch { /* direct refresh can retry later */ }
    }
    for (const [path, watcher] of watchers) if (!snapshot.paths.has(path) || !existsSync(path)) { watcher.close(); watchers.delete(path); }
    if (next !== fingerprint) { fingerprint = next; onChange(); }
  };
  const schedule = () => {
    if (stopped || scheduled) return;
    scheduled = setTimeout(() => { scheduled = undefined; reconcile(); }, 20);
    scheduled.unref?.();
  };
  reconcile();
  // `fs.watch` cannot observe through a directory that did not exist when it
  // was armed on every host; a cheap unref'd reconciliation closes that gap.
  const fallback = setInterval(reconcile, 250);
  fallback.unref?.();
  return () => {
    stopped = true;
    clearInterval(fallback);
    if (scheduled) clearTimeout(scheduled);
    for (const watcher of watchers.values()) watcher.close();
    watchers.clear();
  };
}

export interface LeadUsageSummary {
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  latestCacheHitRate?: number;
  cost: CumulativeCost;
}
const nonnegative = (value: unknown): number | undefined => typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : undefined;
export function summarizeLeadUsage(entries: readonly unknown[]): LeadUsageSummary {
  const summary: LeadUsageSummary = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cost: emptyCost() };
  let sawCost = false, unknownCost = false;
  for (const raw of entries) {
    const entry = raw as { type?: string; message?: { role?: string; usage?: unknown }; usage?: unknown };
    const relevant = entry.type === "message" && entry.message?.role === "assistant"
      ? entry.message.usage
      : entry.type === "message" && entry.message?.role === "toolResult" && entry.message.usage
        ? entry.message.usage
        : (entry.type === "branch_summary" || entry.type === "compaction") && entry.usage
          ? entry.usage : undefined;
    if (relevant === undefined) {
      if (entry.type === "message" && entry.message?.role === "assistant") unknownCost = true;
      continue;
    }
    const usage = relevant as { input?: unknown; output?: unknown; cacheRead?: unknown; cacheWrite?: unknown; cost?: { total?: unknown } };
    const input = nonnegative(usage.input) ?? 0, output = nonnegative(usage.output) ?? 0, cacheRead = nonnegative(usage.cacheRead) ?? 0, cacheWrite = nonnegative(usage.cacheWrite) ?? 0;
    summary.input += input; summary.output += output; summary.cacheRead += cacheRead; summary.cacheWrite += cacheWrite;
    if (entry.type === "message" && entry.message?.role === "assistant") {
      const prompt = input + cacheRead + cacheWrite;
      summary.latestCacheHitRate = prompt > 0 ? cacheRead / prompt * 100 : undefined;
    }
    const cost = nonnegative(usage.cost?.total);
    if (cost === undefined) unknownCost = true; else { sawCost = true; summary.cost.knownUsd += cost; }
  }
  summary.cost.knownContributors = sawCost ? 1 : 0;
  summary.cost.unknownContributors = unknownCost ? 1 : 0;
  summary.cost.descendants = sawCost || unknownCost ? 1 : 0;
  return summary;
}

export interface FooterPrimitives {
  visibleWidth(text: string): number;
  truncateToWidth(text: string, width: number, ellipsis?: string): string;
}
interface FooterTheme { fg(color: "dim" | "accent" | "warning" | "error", text: string): string }
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
  getContextUsage?(): { percent?: number | null; contextWindow?: number } | undefined;
  ui: { setFooter(factory: ((tui: FooterTui, theme: FooterTheme, data: FooterData) => AgentFooterComponent) | undefined): void };
}
export interface AgentFooterController { refresh(): void; stop(): void }
export interface AgentFooterSessionLifecycle {
  start(role: SpawnRole | undefined, ctx: AgentFooterContext & { mode?: string }, registry: RpcAgentRegistry, storage: AgentStorageContext): Promise<void>;
  refresh(): void;
  stop(): void;
}
interface ControllerOptions { watchCosts?: typeof watchDescendantCosts }

/** Owns replacement/reload/mode-transition/shutdown semantics for index.ts. */
export function createAgentFooterSessionLifecycle(
  loadPrimitives: () => Promise<FooterPrimitives>,
  createController: typeof createAgentFooterController = createAgentFooterController,
): AgentFooterSessionLifecycle {
  let current: AgentFooterController | undefined;
  return {
    async start(role, ctx, registry, storage) {
      current?.stop();
      current = undefined;
      if (!shouldArmAgentFooter(role, ctx.mode)) return;
      current = createController(ctx, registry, storage, await loadPrimitives());
    },
    refresh() { current?.refresh(); },
    stop() { current?.stop(); current = undefined; },
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
function styleStats(plain: string, values: readonly string[], contextPart: string, contextPercent: number | null | undefined, theme: FooterTheme): string {
  const ranges: Array<{ start: number; text: string; color: "dim" | "accent" | "warning" | "error" }> = [];
  let searchAt = 0;
  for (const value of values) {
    const money = /^~\$\d+\.\d{2}/.exec(value)?.[0];
    if (!money) continue;
    const start = plain.indexOf(money, searchAt);
    if (start < 0) continue;
    ranges.push({ start, text: money, color: "accent" });
    searchAt = start + money.length;
  }
  const contextStart = plain.indexOf(contextPart);
  if (contextStart >= 0 && contextPercent != null && contextPercent > 70) {
    ranges.push({ start: contextStart, text: contextPart, color: contextPercent > 90 ? "error" : "warning" });
  }
  ranges.sort((a, b) => a.start - b.start);
  let at = 0, styled = "";
  for (const range of ranges) {
    if (range.start < at) continue;
    styled += theme.fg("dim", plain.slice(at, range.start));
    styled += theme.fg(range.color, range.text);
    at = range.start + range.text.length;
  }
  return styled + theme.fg("dim", plain.slice(at));
}

export function createAgentFooterController(
  ctx: AgentFooterContext,
  registry: RpcAgentRegistry,
  storage: AgentStorageContext,
  primitives: FooterPrimitives,
  options: ControllerOptions = {},
): AgentFooterController {
  registerAgentCostOwner(registry, storage);
  let renderRequest: (() => void) | undefined;
  let stopped = false;
  ctx.ui.setFooter((tui, theme, footerData) => {
    renderRequest = () => tui.requestRender();
    const unbranch = footerData.onBranchChange(renderRequest);
    const unwatch = (options.watchCosts ?? watchDescendantCosts)(storage, registry, renderRequest);
    let disposed = false;
    return {
      invalidate() { /* Theme is read from the host callback on every render; no themed cache exists. */ },
      dispose() {
        if (disposed) return;
        disposed = true; unbranch(); unwatch();
        if (renderRequest) renderRequest = undefined;
      },
      render(width: number): string[] {
        if (width <= 0) return [""];
        const usage = summarizeLeadUsage(ctx.sessionManager.getEntries());
        const descendants = aggregateDescendantCosts(storage, registry);
        const leadCost = formatCumulativeCost(usage.cost), childCost = formatCumulativeCost(descendants);
        const tokenParts = [
          usage.input ? `↑${formatTokens(usage.input)}` : undefined,
          usage.output ? `↓${formatTokens(usage.output)}` : undefined,
          usage.cacheRead ? `R${formatTokens(usage.cacheRead)}` : undefined,
          usage.cacheWrite ? `W${formatTokens(usage.cacheWrite)}` : undefined,
          (usage.cacheRead || usage.cacheWrite) && usage.latestCacheHitRate !== undefined ? `CH${usage.latestCacheHitRate.toFixed(1)}%` : undefined,
        ].filter((part): part is string => !!part);
        const context = ctx.getContextUsage?.();
        const window = context?.contextWindow ?? ctx.model?.contextWindow ?? 0;
        const percent = context?.percent == null ? "?" : context.percent.toFixed(1);
        const contextPart = `${percent}%/${formatTokens(window)}`.replace("?%", "?");
        const required = [`Lead ${leadCost}`, `Subagents ${childCost}`];
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
        const stats = styleStats(statsPlain, [leadCost, childCost], contextPart, context?.percent, theme);

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
    stop() { if (stopped) return; stopped = true; ctx.ui.setFooter(undefined); renderRequest = undefined; },
  };
}
