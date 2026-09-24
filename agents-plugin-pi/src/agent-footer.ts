/**
 * Theme-aware replacement for Pi's built-in footer with bounded cost
 * estimates: rendering, the footer controller, and the footer's session
 * lifecycle. The cost estimate it renders (and every hop's cost accounting,
 * footer or not) lives in agent-cost.ts; this module only imports from it.
 */
import { homedir } from "node:os";
import { createFooterGitCache, type GitCacheOptions } from "./footer-git-status.ts";
import { relative, resolve, sep } from "node:path";
import type { AgentStorageContext } from "./agent-storage.ts";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";
import { attachFooterCostEstimate, releaseFooterCostEstimate } from "./agent-cost.ts";
import type { RpcAgentRegistry } from "./spawner.ts";

export { formatCumulativeCost } from "./agent-telemetry.ts";

export interface FooterPrimitives {
  visibleWidth(text: string): number;
  truncateToWidth(text: string, width: number, ellipsis?: string): string;
}
interface FooterTheme { fg(color: "text" | "dim" | "accent" | "warning" | "error" | "success", text: string): string }
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
  isIdle?(): boolean;
  model?: { provider?: string; id?: string; reasoning?: boolean; contextWindow?: number };
  thinkingLevel?: string;
  sessionManager: { getEntries(): readonly unknown[]; getSessionName?(): string | undefined; getCwd?(): string };
  getContextUsage?(): { tokens?: number | null; percent?: number | null; contextWindow?: number } | undefined;
  ui: { setFooter(factory: ((tui: FooterTui, theme: FooterTheme, data: FooterData) => AgentFooterComponent) | undefined): void };
}
export interface AgentFooterController {
  turnEnd(): void;
  input(): void;
  refresh(): void;
  refreshAgents(): void;
  acceptUsage(source: unknown): void;
  checkpoint(): boolean;
  stop(): void;
}
export interface AgentFooterSessionLifecycle {
  start(role: SpawnRole | undefined, ctx: AgentFooterContext & { mode?: string }, registry: RpcAgentRegistry, storage: AgentStorageContext): Promise<void>;
  turnEnd(): void;
  input(): void;
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
    turnEnd() { current?.turnEnd(); },
    input() { current?.input(); },
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
  gitQuery?: GitCacheOptions["query"],
): AgentFooterController {
  // Reuses an estimate this hop already built (an eviction or a descendant
  // report can precede the footer mount); a second one would diverge.
  const state = attachFooterCostEstimate(registry, storage, () => ctx.sessionManager.getEntries().length > 0);
  let presentation = state.presentation();
  let renderRequest: (() => void) | undefined;
  let componentDispose: (() => void) | undefined;
  let stopped = false;
  const updatePresentation = () => { presentation = state.presentation(); if (!stopped) renderRequest?.(); };
  const git = createFooterGitCache({
    cwd: () => ctx.sessionManager.getCwd?.() ?? ctx.cwd,
    isIdle: () => ctx.isIdle?.() ?? false,
    changed: () => { if (!stopped) renderRequest?.(); },
    query: gitQuery,
  });

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

        const cwd = ctx.sessionManager.getCwd?.() ?? ctx.cwd;
        let path = displayCwd(cwd);
        const branch = footerData.getGitBranch(); if (branch) path += ` (${branch})`;
        const name = ctx.sessionManager.getSessionName?.();
        const nameSuffix = name ? ` • ${name}` : "";
        const spans = git.spans(cwd);
        const gitPlain = spans.length ? " " + spans.map(span => span.text).join(" ") : "";
        const showGit = primitives.visibleWidth(path + gitPlain + nameSuffix) <= width;
        const gitStyled = showGit && spans.length ? " " + spans.map(span => theme.fg(span.color, span.text)).join(" ") : "";
        const pathLine = theme.fg("dim", path) + gitStyled + theme.fg("dim", nameSuffix);
        const lines = [primitives.truncateToWidth(pathLine, width, theme.fg("dim", "...")), stats];
        const statuses = [...footerData.getExtensionStatuses().entries()].sort(([a], [b]) => a.localeCompare(b)).map(([, text]) => sanitizeStatus(text));
        if (statuses.length) lines.push(primitives.truncateToWidth(statuses.join(" "), width, theme.fg("dim", "...")));
        return lines;
      },
    };
  });

  return {
    turnEnd() { git.turnEnd(); },
    input() { git.input(); },
    refresh() { if (!stopped) renderRequest?.(); },
    // Prunes removed children from `registry` before recomputing (agent-cost.ts).
    refreshAgents() { if (stopped) return; state.pruneAndRecompute(); updatePresentation(); },
    acceptUsage(source) { if (stopped) return; state.acceptUsage(source); updatePresentation(); },
    checkpoint() { return stopped ? true : state.persist(); },
    stop() {
      if (stopped) return;
      state.persist(); stopped = true; git.stop();
      ctx.ui.setFooter(undefined); componentDispose?.(); renderRequest = undefined;
      releaseFooterCostEstimate(registry, state);
    },
  };
}
