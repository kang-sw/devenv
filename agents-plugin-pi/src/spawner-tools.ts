import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { McpToolCallResult } from "./mcp-stdio-client.ts";
import { validateCatalogModel, validateConcreteModel, type ConcreteModelRejection, type ModelCatalogEntry, type TierFailure, type TierRejection } from "./model-catalog.ts";
import { createAgentStorageContext, type AgentStorageContext } from "./agent-storage.ts";
import { READ_TOOLS, inheritableTools, readOnlyWsTools } from "./delegation-policy.ts";
import { withoutRetiredNativeTools } from "./native-agent-policy.ts";
import type { RpcAgentRegistry } from "./spawner.ts";

// Only the retained execution gateway owns a native runtime registry. Keep
// lower-level lifecycle helpers testable without reviving public delegation.
export const executionRegistries = new WeakSet<RpcAgentRegistry>();

const persistenceBlockedRegistries = new WeakSet<RpcAgentRegistry>();

/** A retained, unreadable/unquarantined sidecar must not be replaced by new execution history. */
export function blockExecutionPersistence(registry: RpcAgentRegistry): void {
  persistenceBlockedRegistries.add(registry);
}

export function assertExecutionPersistenceAvailable(registry: RpcAgentRegistry): void {
  if (persistenceBlockedRegistries.has(registry)) throw new Error("ws-pi-agent: persistent execution is blocked while the existing native sidecar/history cannot be safely preserved; resolve the reported preservation failure first");
}

// ---------------------------------------------------------------------------
// Pure helpers shared by persistent RPC-backed child paths.
// ---------------------------------------------------------------------------

export type ToolGroup = "read-only" | "read-only-explore" | "full-worker" | "execute-worker";

/** Sole source of truth for the child-side report tool's name, shared by `TOOL_GROUPS`, its registration, and the event-matching branch in `applyRpcEvent`. */
export const REPORT_TO_LEAD_TOOL_NAME = "ws-report-to-lead";

/**
 * Sole source of truth for the gated-exec worker tool's name (260904 Phase 1:
 * end-to-end approval gateway). Defined here rather than in
 * `execute-gateway.ts` to avoid a circular import: `TOOL_GROUPS` below and
 * `applyRpcEvent`'s new pendingApproval-observation branch both need this
 * name, and `execute-gateway.ts` (which registers the tool itself) already
 * imports plumbing FROM `spawner.ts` (`spawnAgent`, `RpcAgentRecord`, etc) —
 * the reverse import would be circular. Mirrors `REPORT_TO_LEAD_TOOL_NAME`'s
 * placement for the same reason.
 */
export const GATED_EXEC_TOOL_NAME = "ws-worker-exec";

/** Parent-shell bootstrap overrides select a forced ws-mcp install. They are
 * lead-launch policy, never child-launch policy. */
export const CHILD_BOOTSTRAP_OVERRIDE_ENVS = ["WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL"] as const;

/** Mailbox identity is the lead's alone. A child's own ws-mcp would otherwise
 * resolve the same named inbox and contest the lead's presence record, or mint
 * a phantom auto-identity peer; children answer over the reply-id channel. */
export const CHILD_MAILBOX_IDENTITY_ENVS = ["WS_MAILBOX", "WS_MAILBOX_AUTO"] as const;

const READ_ONLY_BUILTINS: readonly string[] = READ_TOOLS;

/**
 * Per-registry child-launch preflight (the bridge's `ensureChildRuntime`),
 * keyed like `registerAgentCostOwner` so every launch kind that shares a
 * registry (spawn, ask, execute, explore, fork, and their dormant resumes)
 * passes the one guard without each call site threading it.
 */
export const childLaunchPreflights = new WeakMap<RpcAgentRegistry, () => Promise<void>>();

export function registerChildLaunchPreflight(registry: RpcAgentRegistry, preflight: (() => Promise<void>) | undefined): void {
  if (preflight) childLaunchPreflights.set(registry, preflight);
  else childLaunchPreflights.delete(registry);
}

/**
 * Keeps the most recent error-level `ui.notify` a child sends over its RPC
 * stdout. A child whose session bootstrap fails reports through exactly that
 * notify and exits with blank stderr (stderr would corrupt the parent TUI),
 * so the launch error the parent reports would otherwise say only
 * "Agent process exited". Subscribe before `client.start()`.
 */
export function captureChildBootstrapFailure(client: Pick<RpcClient, "onEvent">): { decorate(err: unknown): unknown; dispose(): void } {
  let last: string | undefined;
  const dispose = client.onEvent((event) => {
    const e = event as { type?: unknown; method?: unknown; notifyType?: unknown; message?: unknown };
    if (e.type === "extension_ui_request" && e.method === "notify" && e.notifyType === "error" && typeof e.message === "string") last = e.message;
  });
  return {
    decorate(err) {
      if (last === undefined) return err;
      const original = err instanceof Error ? err : new Error(String(err));
      return new Error(`${original.message}\nChild reported: ${last}`, { cause: original });
    },
    dispose,
  };
}

export const TOOL_GROUPS: Record<ToolGroup, readonly string[]> = {
  // Legacy names remain readable in persisted metadata, not as delegation permissions.
  "read-only": [...READ_ONLY_BUILTINS],
  "read-only-explore": [...READ_ONLY_BUILTINS],
  "full-worker": ["read", "bash", "edit", "write", "grep", "find", "ls"],
  "execute-worker": [...READ_ONLY_BUILTINS, GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME],
};

/**
 * The execute-worker's structured git read tools (tool-shape allowlist per the
 * approval-gateway design: no command-string matching). Always intersected with
 * `readOnlyWsTools`, so a name here that is not read authority is never granted.
 */
const EXECUTE_WORKER_WS_TOOLS: ReadonlySet<string> = new Set(["ws__git_status", "ws__git_diff", "ws__git_log", "ws__git_merge_base"]);

export function resolveTools(group: ToolGroup, wsToolNames: readonly string[] = []): string {
  const builtins = TOOL_GROUPS[group];
  const extra = group === "full-worker" ? inheritableTools(wsToolNames)
    : group === "execute-worker" ? readOnlyWsTools(wsToolNames).filter(name => EXECUTE_WORKER_WS_TOOLS.has(name))
    : [];
  return withoutRetiredNativeTools([...builtins, ...extra]).join(",");
}

/** Shared tier resolution for persistent spawn and the manual advisory.
 * Accept only pi-labeled exact catalog membership with configured auth. Unknown
 * or unauthenticated pi values inherit with rejected detail and no tier effort.
 * Non-pi, omitted tiers and transport/parse failures silently inherit. Registry
 * and auth reads stay outside this injected-MCP resolver; no config writes.
 */
/**
 * Minimal `callTool`-shaped interface `resolveModelForAliasViaWsMcp` needs —
 * a real `McpStdioClient` satisfies this structurally, but so does
 * `bridge.ts`'s own duck-typed `callTool` closure (`WorkflowManualMappingDeps["callTool"]`),
 * letting `computePiAliasTableReport` call this resolver without a circular
 * import (`spawner.ts` already imports `type BridgeHandle` from
 * `bridge.ts` — a type-only import that is erased at build/runtime, so a
 * VALUE import in the other direction, `bridge.ts` importing this function
 * from `spawner.ts`, does not create a runtime cycle).
 */
export interface ResolveAgentCallToolClient {
  callTool: (name: string, args: Record<string, unknown>) => Promise<McpToolCallResult>;
}

/**
 * Fixed backend-tag -> provider-id expansion applied to a slash-less
 * `config.resolve_agent` `model` value once `resolved_from === "pi"`, before
 * the catalog membership check — `config.resolve_agent` can answer a bare
 * model id (e.g. `gpt-5.6-high`) tagged with which backend it came from
 * (`backend: "codex"`), and the catalog only ever holds `provider/id` pairs.
 * A slashed model is never re-prefixed; a slash-less model with an
 * empty/`"pi"`/unmapped `backend` is left as-is (and therefore fails the
 * catalog check as `unknown`, not silently passed through). Exported so
 * `resolveModelForAliasViaWsMcp`'s two real provider ids are one shared
 * source for tests/advisory rather than duplicated literals.
 */
export const BACKEND_TO_PROVIDER: Record<string, string> = { codex: "openai-codex", claude: "anthropic" };

export interface TierResolution {
  model?: string;
  effort?: string;
  rejected?: TierRejection;
  concreteRejected?: ConcreteModelRejection;
  source: "tier" | "concrete" | "inherit";
  failure?: TierFailure;
}

/**
 * 260906 Phase 2 (YAML/TUI dispatch-row rendering): the display-only shape
 * `ctx.onModelResolved` hands back the instant `spawnAgent` finishes model
 * selection (all refusal guards already passed). `tier` is the raw tier name
 * for a tier hit, `"concrete"` for a direct catalog selection, or `"inherit"`
 * for parent-model inheritance. `effort` matches `record.modelEffort` exactly.
 */
export interface ResolvedModelInfo {
  tier: string;
  model?: string;
  effort?: string;
  inherited: boolean;
}

function tierResolution(
  base: { model?: string; effort?: string; rejected?: TierRejection; concreteRejected?: ConcreteModelRejection },
  source: TierResolution["source"],
  failure?: TierFailure,
): TierResolution {
  // New policy callers read source/failure explicitly; legacy advisory
  // consumers retain their model/rejected object enumeration.
  return Object.defineProperties(base, {
    source: { value: source, enumerable: false },
    ...(failure ? { failure: { value: failure, enumerable: false } } : {}),
  }) as TierResolution;
}

export async function resolveModelForAliasViaWsMcp(
  client: ResolveAgentCallToolClient,
  alias: string | undefined,
  inheritModel: string | undefined,
  catalog: readonly ModelCatalogEntry[] = [],
): Promise<TierResolution> {
  if (!alias) return tierResolution({ model: inheritModel }, "inherit");
  let result: McpToolCallResult;
  try {
    result = await client.callTool("config.resolve_agent", { tier: alias, format: "json" });
  } catch {
    return tierResolution({ model: inheritModel }, "inherit", { kind: "transport" });
  }
  if (result.isError) return tierResolution({ model: inheritModel }, "inherit", { kind: "transport" });
  const text = result.content.find((item) => item.type === "text")?.text;
  if (!text) return tierResolution({ model: inheritModel }, "inherit", { kind: "parse" });
  let parsed: { model?: unknown; effort?: unknown; resolved_from?: unknown; backend?: unknown };
  try {
    parsed = JSON.parse(text) as { model?: unknown; effort?: unknown; resolved_from?: unknown; backend?: unknown };
  } catch {
    return tierResolution({ model: inheritModel }, "inherit", { kind: "parse" });
  }
  if (
    !parsed || typeof parsed !== "object" || typeof parsed.resolved_from !== "string" || typeof parsed.model !== "string" ||
    (parsed.effort !== undefined && typeof parsed.effort !== "string") || (parsed.backend !== undefined && typeof parsed.backend !== "string")
  ) {
    return tierResolution({ model: inheritModel }, "inherit", { kind: "parse" });
  }
  if (parsed.resolved_from !== "pi") {
    const rejected: TierRejection = { model: parsed.model, resolvedFrom: parsed.resolved_from, why: "unset" };
    return tierResolution({ model: inheritModel, rejected }, "inherit", { kind: "unset", model: parsed.model, resolvedFrom: parsed.resolved_from });
  }
  // Backend expansion only applies here — between the resolved_from === "pi"
  // gate above and the catalog check below — never to the non-pi "unset"
  // branch, whose raw value never reaches the catalog at all.
  const provider = parsed.model.includes("/") ? undefined : BACKEND_TO_PROVIDER[parsed.backend ?? ""];
  const checkedModel = provider ? `${provider}/${parsed.model}` : parsed.model;
  const stored = provider ? parsed.model : undefined;
  const catalogResult = validateCatalogModel(checkedModel, catalog);
  if (catalogResult.rejected?.why === "unknown") {
    const rejected: TierRejection = { ...catalogResult.rejected, resolvedFrom: parsed.resolved_from, ...(stored !== undefined ? { stored } : {}) };
    return tierResolution({ model: inheritModel, rejected }, "inherit", { kind: "unknown", model: checkedModel, resolvedFrom: parsed.resolved_from, catalogEmpty: catalog.length === 0 });
  }
  if (catalogResult.rejected?.why === "no-auth") {
    const rejected: TierRejection = { ...catalogResult.rejected, resolvedFrom: parsed.resolved_from, ...(stored !== undefined ? { stored } : {}) };
    return tierResolution({ model: inheritModel, rejected }, "inherit", { kind: "no-auth", model: checkedModel, resolvedFrom: parsed.resolved_from });
  }
  return tierResolution({ model: catalogResult.model, effort: parsed.effort || undefined }, "tier");
}

export async function resolveSpawnModel(
  client: ResolveAgentCallToolClient,
  selection: string | undefined,
  inheritModel: string | undefined,
  catalog: readonly ModelCatalogEntry[],
  allowConcreteModel: boolean,
): Promise<TierResolution> {
  if (!allowConcreteModel || !selection?.includes("/")) return resolveModelForAliasViaWsMcp(client, selection, inheritModel, catalog);
  const concrete = validateConcreteModel(selection, catalog);
  return concrete.rejected
    ? tierResolution({ concreteRejected: concrete.rejected }, "concrete")
    : tierResolution({ model: concrete.model }, "concrete");
}

/**
 * Pure merge rule for `spawnAgent`'s effective effort. A non-empty explicit
 * Pi level wins. The `"default"` sentinel applies no caller override: an
 * inherited model keeps the captured parent effort, a tier keeps its configured
 * effort, and a concrete model leaves effort unset for Pi/model defaulting.
 * Omission and the historical empty string retain the prior merge behavior.
 */
export function effectiveModelEffort(
  callerEffort: string | undefined,
  resolvedEffort: string | undefined,
  source: TierResolution["source"] = "tier",
  inheritEffort?: string,
): string | undefined {
  if (callerEffort === "default") {
    if (source === "inherit") return inheritEffort;
    if (source === "concrete") return undefined;
    return resolvedEffort;
  }
  return callerEffort || resolvedEffort;
}

/**
 * Pure extraction of the calling tool-execute `toolCtx`'s current model as a
 * `provider/id` string, or `undefined` when absent/malformed. Exported
 * (260904 Phase 1) so `execute-gateway.ts`'s `ws-execute` tool can resolve
 * its own inherit-fallback model the same way `ws-agent-spawn`/`explore`
 * already do here, without duplicating this shape-tolerant extraction.
 */
export function inheritModelFromToolCtx(toolCtx: unknown): string | undefined {
  const model = (toolCtx as { model?: { provider?: string; id?: string } } | undefined)?.model;
  return model?.provider && model?.id ? `${model.provider}/${model.id}` : undefined;
}

/** Extracts the immediate dispatcher's stable Pi identity from a live tool call. */
export function storageContextFromToolCtx(toolCtx: unknown): AgentStorageContext {
  const value = toolCtx as { sessionManager?: { getSessionId?: () => string }; agentStorageRoot?: string } | undefined;
  const id = value?.sessionManager?.getSessionId?.();
  if (!id) throw new Error("ws-pi-agent: current Pi session identity is unavailable");
  return createAgentStorageContext(id, value?.agentStorageRoot);
}

/**
 * Review fix (relay #1, TEST finding #3): pure extraction of `spawnAgent`'s
 * `ctx.toolGroup ?? "full-worker"` default — previously inlined directly in
 * `spawnAgent`, leaving no seam a unit test could exercise independent of a
 * real `RpcClient` spawn. `RpcSpawnCtx.toolGroup`'s own doc comment already
 * states the contract this codifies: "Omitted (or explicit `"full-worker"`)
 * preserves every existing `ws-agent-spawn` caller's behavior unchanged."
 * Mirrors `inheritModelFromToolCtx`'s own extraction-for-testability shape
 * immediately above.
 */
export function resolveSpawnToolGroup(explicit: ToolGroup | undefined): ToolGroup {
  return explicit ?? "full-worker";
}
