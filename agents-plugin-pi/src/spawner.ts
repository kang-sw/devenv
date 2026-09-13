/**
 * Self-built delegation spawner: `ws-agent-spawn` / `ws-agent-send` /
 * `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript` /
 * `ws-report-to-lead` / `explore`.
 *
 * Phase 1 replaces the one-shot `pi --mode json -p` worker spawner with
 * persistent `RpcClient` (`--mode rpc`) children: `ws-agent-spawn` starts a
 * long-lived `pi` subprocess wired through `@earendil-works/pi-coding-agent`'s
 * `RpcClient`, `ws-agent-send` drives it (prompt/followUp/steer, branching on
 * locally-tracked streaming state — see the doc comment on `sendToAgent`),
 * `ws-agent-list` reports live/idle/dormant status plus each agent's last
 * report time, and `ws-agent-stop` gracefully stops a child's process while
 * keeping its `agent_id` -> session/prompt/model mapping registered for a
 * later auto-resume (D-C). `ws-agent-continue` folds into `ws-agent-send` (an
 * id with no live `RpcClient` is "dormant," not a separate tool).
 *
 * 260905 (`260905-feat-ws-pi-push-only-child-reports` Phase 1): there is no
 * harvest tool any more. `ws-agent-wait` is DELETED outright (not deprecated),
 * along with the whole buffered-report machinery it existed to drain
 * (`pendingReports`/`idlePending`/`RpcAgentRecord.waiters`). Every child
 * signal is instead PUSHED into the owning session the instant it happens, as
 * a Pi custom message (`pi.sendMessage(..., {deliverAs, triggerTurn:true})`)
 * in one of six families — `ws-agent-report`, `ws-agent-settled`,
 * `ws-agent-question`, `ws-agent-approval`, `ws-agent-advisory`,
 * `ws-agent-orphaned` — each carrying `details.agent_id`, its own payload, and
 * a fan-in status line (`computeRunningStatusLine`) counting only children
 * still executing autonomously. The lead therefore ends its turn
 * after dispatching work and is woken by the pushes themselves; it never
 * blocks in a wait call, so an approval request can reach it mid-flight
 * instead of queueing behind an unfinished wait turn.
 *
 * A `followUp` push raised while the owning session is mid-turn is HELD
 * (`heldPushQueue`) and released at that turn's `agent_end` as one versioned
 * `ws-push-batch` follow-up. The batch retains FIFO model content and separate
 * structured TUI items, so Pi steering mode cannot stretch one boundary
 * snapshot across multiple assistant turns. `agent_settled` plus a counted
 * wake remains the fallback for compaction, late arrival, or rejected sends;
 * confirmed wake starts release one batch as steering before their first
 * response. `steer` pushes that were never held remain immediate, since
 * interrupting is their purpose. A child's ordinary assistant answer becomes
 * terminal only at `agent_settled`; its queue admission is independently
 * tracked so pending delivery is recoverable without being called execution.
 *
 * The spawn tool's `model_name` param accepts one of the four fixed tiers
 * (`small`/`medium`/`large`/`xlarge`) or a concrete Pi catalog `provider/id`.
 * Tier names resolve through ws-mcp's `config.resolve_agent`; concrete IDs
 * validate directly against the live Pi catalog and configured auth. Omitting
 * the parameter retains parent-model inheritance. The caller still passes an
 * already-rendered `system_prompt_path` — this module never renders it.
 *
 * `explore` is one persistent researcher preset over `spawnAgent`. Every
 * eligible lead, fork, or worker receives the same schema and RPC lifecycle;
 * an intent mode selects a configured tier while the delegation envelope
 * remains authoritative for child depth and tools.
 *
 * A parent's local settle is not subtree completion. Its private channel
 * publishes active descendants and queued deliveries; a later synthesized
 * settled turn is required after the subtree becomes quiescent.
 *
 * `--tools` curation (`read-only`/`read-only-explore`/`full-worker`)
 * lives only in the in-memory `TOOL_GROUPS` table and Pi CLI flags.
 *
 * Phase 2 adds the per-agent child->lead report channel: `ws-report-to-lead`
 * (child-side, `full-worker`-only) is observed purely from the existing
 * `RpcClient.onEvent()` stream's `tool_execution_start` events — no new
 * transport (see `applyRpcEvent`'s doc comment for the full trace) — and, as
 * of 260905, pushed straight to the lead rather than buffered. Only a bounded
 * `RpcAgentRecord.reportLog` (question kind + timestamp, no text) is retained
 * for control freshness and `ws-agent-list`'s last-report time; it is not a
 * completion ledger.
 * `ws-agent-transcript` (lead-side, not in any `TOOL_GROUPS`) returns the
 * already-tracked `sessionPath` with no RPC round-trip.
 */

import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, relative, resolve, sep } from "node:path";
import { randomUUID } from "node:crypto";
import { StringDecoder } from "node:string_decoder";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { RpcClient, type RpcClientOptions } from "@earendil-works/pi-coding-agent";
import { attachFirstTaskForkCacheNotice, type ForkCacheNoticeOwner } from "./fork-cache-notice.ts";
import type { McpStdioClient, McpToolCallResult } from "./mcp-stdio-client.ts";
import type { BridgeHandle } from "./bridge.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { buildAgentSendSummary, buildAgentSpawnSummary, buildExploreSummary, createDispatchToolPreview } from "./tool-row-render.ts";
import {
  formatConcreteModelWarning,
  formatExploreTierRefusal,
  formatTierWarning,
  modelCatalogFromToolCtx,
  tierWarningNotifierFromToolCtx,
  validateCatalogModel,
  validateConcreteModel,
  type ConcreteModelRejection,
  type ModelCatalogEntry,
  type TierFailure,
  type TierRejection,
} from "./model-catalog.ts";
import { EXPLORE_MODE_TIERS, WS_PI_EXPLORE_MODE_ENV, WS_PI_FORK_AFFINITY_ENV, WS_PI_FORK_CONTEXT_ENV, WS_PI_FORK_READY_NONCE_ENV, WS_PI_FORK_READY_PATH_ENV, WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV, isLeadOrFork, readSpawnRole, type ExploreMode, type SpawnRole } from "./process-role.ts";
import { captureForkContext, compareForkRegistrations, removeForkTransport, writePrivateJson, type ForkContext, type ForkReadiness } from "./fork-context.ts";
import { allocateAgentHome, createAgentStorageContext, inspectOwnedHomeRemoval, isOwnedSessionPath, observeSessionWrite, readOwnership, removeOwnedAgentHome, touchOwnership, updateOwnership, writeOwnership, type AgentOwnership, type AgentStorageContext } from "./agent-storage.ts";
import { ownerNotifyRef } from "./owner-notify.ts";
export { ownerNotifyRef } from "./owner-notify.ts";
import { readSessionEntries, reduceTelemetry, type AgentTelemetry, type TelemetryOrigin } from "./agent-telemetry.ts";
import { CHILD_MANAGEMENT_TOOLS, DEFAULT_MAX_AGENT_DEPTH, DELEGATION_ENV, SUBTREE_ENV, READ_TOOLS, NETWORK_TOOLS, childPolicy, readDelegationPolicy, readOnlyWsTools, type DelegationPolicy, type PlaybookProfile, type RenderProvenance } from "./delegation-policy.ts";
import { createWebSearch } from "./web-search.ts";
import { clearWebReadiness, verifyWebReadiness, WEB_HOME_ENV, WEB_NONCE_ENV } from "./web-readiness.ts";
import { beginSubtreeDispatch, installSubtreePublisher, publishSubtree, readSubtreeChannel, readSubtreeSnapshot, subtreeWaiting, type SubtreeChannel } from "./subtree-lifecycle.ts";
import { PUSH_BATCH_CUSTOM_TYPE, PUSH_BATCH_VERSION, type PushBatchItem, type PushBatchItemState } from "./push-protocol.ts";
import { persistAgentCostCheckpoint, persistEvictedAgentCost, registerAgentCostOwner } from "./agent-footer.ts";

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

/**
 * Spawn-time env var carrying the child's own approvals directory
 * (`<sessionDir>/approvals`, see `buildRpcClientOptions` below) — the only
 * channel `ws-worker-exec`'s `execute()` has to learn where to poll for its
 * decision file, since neither `sessionDir` nor `sessionPath` is otherwise
 * passed to the child process. Parallel to `WS_PI_SPAWN_ROLE_ENV`
 * (`process-role.ts`), but kept here (not there) because it is spawner-owned
 * plumbing specific to the RPC-backed path, not a role marker every spawn
 * kind needs.
 */
export const WS_PI_APPROVAL_DIR_ENV = "WS_PI_APPROVAL_DIR";

/** Parent-shell bootstrap overrides select a forced ws-mcp install. They are
 * lead-launch policy, never child-launch policy. */
const CHILD_BOOTSTRAP_OVERRIDE_ENVS = ["WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL"] as const;

const READ_ONLY_BUILTINS: readonly string[] = READ_TOOLS;

export const TOOL_GROUPS: Record<ToolGroup, readonly string[]> = {
  "read-only": [...READ_ONLY_BUILTINS, REPORT_TO_LEAD_TOOL_NAME],
  "read-only-explore": [...READ_ONLY_BUILTINS, REPORT_TO_LEAD_TOOL_NAME, ...CHILD_MANAGEMENT_TOOLS],
  "full-worker": ["read", "bash", "edit", "write", "grep", "find", "ls", REPORT_TO_LEAD_TOOL_NAME, ...CHILD_MANAGEMENT_TOOLS],
  "execute-worker": [...READ_ONLY_BUILTINS, GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME, "explore"],
};

export function resolveTools(group: ToolGroup, wsToolNames: readonly string[] = []): string {
  const builtins = TOOL_GROUPS[group];
  const extra = group === "full-worker" ? wsToolNames : [];
  return [...builtins, ...extra].join(",");
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

async function resolveSpawnModel(
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

// ---------------------------------------------------------------------------
// RPC-backed persistent-child engine (`ws-agent-spawn` / `ws-agent-send` /
// `ws-agent-list` / `ws-agent-stop` / `ws-agent-transcript`).
// ---------------------------------------------------------------------------

/**
 * Every `new RpcClient(...)` construction passes `cliPath: process.argv[1]`
 * explicitly — spawn and resume-from-dormant alike. It also receives the
 * exact entry path captured by the loaded manifest module, with ambient
 * extension discovery disabled, so a child cannot load a duplicate adapter. `RpcClient.start()` in
 * the installed `@earendil-works/pi-coding-agent` package always does
 * `spawn("node", [cliPath, ...args])` with `cliPath = this.options.cliPath
 * ?? "dist/cli.js"`; there is no bare-`pi` fallback inside `RpcClient`.
 * If `process.argv[1]` is ever missing/non-existent
 * there is no client-side fallback path — this is the ticket's settled
 * answer, carried forward as-is, not something to re-derive.
 */
const RPC_CLI_PATH = process.argv[1];

export interface RpcAgentRecord {
  agentId: string;
  /**
   * 260905 (alias/park/cap ticket): caller-supplied, human-chosen short name
   * for THIS agent record — a different concept from
   * `resolveModelForAliasViaWsMcp`'s "alias," which names a fixed *tier*
   * resolved through ws-mcp's `config.resolve_agent` tool (see above).
   * Optional; the adapter never derives one from the prompt. Resolved
   * through `resolveAgentId` alongside the raw `agentId` uuid on every
   * `ws-agent-send`/`ws-agent-stop`/`ws-agent-transcript`/`ws-approve` call.
   * Reusing an alias on a new spawn overwrites a dormant/idle holder (clearing
   * the holder's `alias`, not its `title`) or rejects the spawn outright when
   * the holder is `running`/`threadBound` — see `spawnAgent`.
   */
  alias?: string;
  /**
   * 260905: caller-supplied free-text label, independent of `alias` — never
   * used for resolution, purely descriptive (roll-call/list display). Reusing
   * an `alias` on a new spawn clears the prior holder's `alias` but leaves its
   * `title` untouched.
   */
  title?: string;
  /** The live RPC child, or `undefined` when dormant (stopped but resumable — D-C). */
  client?: RpcClient;
  /** Absolute path to the ws-owned `--session` file, reused unchanged across every (re)start. */
  sessionPath: string;
  /** Durable-home ownership. Absent only for pre-retention legacy records. */
  ownership?: AgentOwnership;
  /** Lead-rendered playbook prompt path, passed via `--append-system-prompt`; reused unchanged across resumes (no re-render). */
  systemPromptPath?: string;
  /** Resolved `provider/id`, or undefined to inherit pi's own default resolution. Cached so a dormant resume reuses the same model. */
  modelBase?: string;
  /** Effective thinking override, applied via `setThinkingLevel()` after every (re)start; absent means Pi/model default. */
  modelEffort?: string;
  /** Observed child selection and recomputed durable usage; launch intent stays above. */
  telemetry?: AgentTelemetry;
  /** Legacy-fork floor: permits post-launch context occupancy, never lifetime cost. */
  telemetryContextFloor?: TelemetryOrigin;
  observedModel?: string;
  observedEffort?: string;
  /** Context occupancy for a legacy fork whose child-attribution boundary is unavailable. */
  observedContextTokens?: number;
  /**
   * 260906 Phase 2 (YAML/TUI dispatch-row rendering): the raw model selection
   * requested at spawn, or `undefined` when omitted — display-only,
   * reconstructs `ws-agent-send`'s resolved-model line
   * for a target agent without a live resolution. NOT persisted to the
   * sidecar (`agent-sidecar.ts`); a record revived after a restart carries
   * neither field (see `modelSource`'s doc comment for the degrade rule).
   */
  modelTier?: string;
  /**
   * 260906 Phase 2: mirrors `TierResolution.source` at spawn time
   * (`"tier" | "concrete" | "inherit"`). NOT persisted to the sidecar — a `ws-agent-send`
   * to a record revived from a sidecar snapshot after a restart finds this
   * field `undefined` and treats that as `"inherit"` (the safe default),
   * never throwing or guessing a tier name.
   */
  modelSource?: "tier" | "concrete" | "inherit";
  /** Cached bridge `ws__*` tool names, for `--tools` re-resolution on a dormant resume. */
  wsToolNames: readonly string[];
  /** Curated `--tools` group this record was spawned with; reused unchanged on a dormant resume so `resolveTools` never silently widens/narrows a resumed child's tool surface. Set at spawn (`ctx.toolGroup ?? "full-worker"`), never mutated afterward. */
  toolGroup: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): a pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` entirely when set —
   * a fork's tool surface is dynamic (`computeForkToolSurface` over the
   * lead's own `pi.getActiveTools()` at spawn time, `fork.ts`), not one of
   * the static `TOOL_GROUPS` entries `toolGroup` indexes. Cached verbatim at
   * spawn and reused unchanged on every dormant resume (mirrors
   * `systemPromptPath`/`modelBase`'s existing cache-and-reuse contract).
   * `undefined` for every non-fork spawn — those keep resolving tools from
   * `toolGroup`/`wsToolNames` exactly as before.
   */
  explicitTools?: string;
  /** Immutable task/discussion capture retained across dormant recovery. */
  forkContext?: ForkContext;
  /**
   * 260905: which spawn shape produced this record, recorded at spawn time
   * rather than re-derived from `toolGroup`/`explicitTools` heuristics. Read
   * by the shutdown sidecar (`agent-sidecar.ts`) so a `session_start` revival
   * can re-arm the right role wiring for a resurrected orphan.
   */
  spawnRole?: SpawnAgentRole;
  /** Persisted authority and one-edge semantic lifecycle; independent of owner holds. */
  delegation?: DelegationPolicy;
  subtreeChannel?: SubtreeChannel;
  waitingOnChildren?: boolean;
  /** Last successful writer to this child. Absence is the legacy/lead default. */
  lastWriter?: "lead" | "owner";
  /** Owner-authored sends, in delivery order, used to attribute persisted user entries. */
  ownerSends?: Array<{ text: string; at: number }>;
  subtreeRevision?: number;
  workGeneration?: number;
  /** Work generation whose ordinary settlement has entered terminal admission. */
  settlementAdmissionGeneration?: number;
  /** Persistent exploration mode; meaningful only for explore records. */
  exploreMode?: ExploreMode;
  /** `true` while an agent run is actively looping (between `agent_start` and `agent_settled`). */
  streaming: boolean;
  /**
   * 260905 fan-in bookkeeping: `true` from the instant a prompt is ISSUED to
   * this child (`promptAgent`, i.e. before any `agent_start` event can arrive)
   * until it settles, is stopped, exits, or fails to spawn. Deliberately a
   * complementary flag to event-confirmed `streaming`; both mean executable
   * work is in flight and feed list/widget/fan-in status consistently.
   */
  running: boolean;
  /**
   * 260905 (live-agent widget ticket): epoch-ms stamp of the most recent
   * prompt ISSUED to this child, stamped unconditionally by `promptAgent`,
   * unlike `lastLeadPromptAt` below, because the widget's "running" row is meant to
   * show how long THIS turn has been going, and a nudge starts a new turn on
   * the wire even though it is not a new lead-issued task boundary. Read by
   * `agent-widget.ts`'s `buildAgentRows` as the running-row elapsed clock;
   * left untouched by `sendToAgent`'s `steer`/`followUp` join (see that
   * function's doc comment) — a mid-stream steer keeps ticking from the
   * turn's original start, by design.
   */
  runStartedAt?: number;
  /** Epoch-ms stamp of the last lead-issued prompt, retained for activity display and attribution. */
  lastLeadPromptAt?: number;
  /**
   * 260905: `true` for the whole lifetime of an owner discussion thread bound
   * to this agent — set by `ask.ts` on every open/reopen (`ensureRespondent`/
   * `openThread`) and on fork-raised question registration
   * (`handleForkRaisedQuestion`), cleared only when the thread actually closes
   * (`/done`, lead takeover, or `ws-withdraw-question`). While set, settled
   * output routes to the owner surface and the record is left out of the lead
   * fan-in status line: the exchange belongs to the owner, not the lead.
   */
  threadBound?: boolean;
  /** Same-process `/done` coordinator for a fork-raised owner thread; never persisted. */
  forkFinish?: ForkFinishOperation;
  /** Existing terminal push admission for the current work, if one exists. */
  terminalDelivery?: TerminalDelivery;
  /** Last-seen final assistant text, cached across `getLastAssistantText()` calls. */
  lastText?: string;
  /**
   * 260905: the head-truncated (`truncatePromptForStorage`,
   * `PROMPT_STORAGE_CAP_BYTES`) copy of the spawn's initial `prompt`, stashed
   * for `ws-agent-list`'s opt-in `include_prompt` reply and the shutdown
   * sidecar. Set once at spawn, never updated by later `ws-agent-send` calls.
   */
  prompt?: string;
  /** Detaches the current `client.onEvent(...)` listener; re-armed on every (re)start. */
  unsubscribe?: () => void;
  /**
   * 260905: bounded history of `ws-report-to-lead` observations on this
   * record — kind and timestamp only, never the text (the text is pushed to
   * the lead immediately and never retained). Replaces the deleted
   * `pendingReports` FIFO as the input to `fork.ts`'s `isIdleWithoutFinal`
   * and as `ws-agent-list`'s last-report time. Capped at `REPORT_LOG_CAP`
   * (drop-oldest) so a long-lived chatty child cannot grow it without bound.
   */
  reportLog: AgentReportLogEntry[];
  /**
   * 260905 (list-model/last-report-fidelity ticket): the sidecar-revival-only
   * fallback for `last_report_at` — `rehydrateOrphanRecord` fills this from
   * `PersistedOrphan.lastReportAt` (the newest `reportLog` entry at
   * shutdown, already an ISO string). `listAgents` and `evictForCapacity`
   * both prefer a real `reportLog` entry over this value whenever one is
   * present, so a revived record that has reported since falls back to its
   * own history rather than the stale shutdown snapshot.
   */
  lastReportAtOverride?: string;
  /**
   * 260904 Phase 1: set by `applyRpcEvent` the instant a `tool_execution_start`
   * for `GATED_EXEC_TOOL_NAME` is observed on this record's child; cleared by
   * `ws-approve` once a decision is written. `undefined` means "no gated
   * command is currently awaiting lead approval on this agent" — the
   * condition `validatePendingApproval` (execute-gateway.ts) rejects against.
   *
   * Review fix (relay #1): also carries `cwd`, captured from the gated-exec
   * tool call's own `args.cwd` override when the worker supplied one (the
   * same `ws-worker-exec` `cwd?` param `execute-gateway.ts`'s `execute()`
   * itself falls back on via `p.cwd ?? sessionCtx.cwd`) — the approval-relay
   * callback (`createApprovalRelay`) must scrape the SAME directory the
   * command will actually run in, not unconditionally the worker's base
   * `sessionCtx.cwd`, or a `cwd`-overridden command's ground-truth git
   * context (branch/dirty/ahead_behind) would silently describe the wrong
   * directory to the lead.
   */
  pendingApproval?: { cmdId: string; command: string; rationale?: string; cwd?: string; decisionWritten?: boolean };
  /**
   * 260904 Phase 2 (review relay #1 I6): consulted by `applyRpcEvent` the
   * instant a `kind:"question"` report is observed on this record. It may
   * return a REPLACEMENT message to enqueue for the lead in place of the
   * fork's own question text; returning `undefined` enqueues the original
   * unchanged (the headless baseline, byte-identical to Phase 1).
   *
   * §1 says the lead is not involved in a fork-raised question and §8 scopes
   * the lead relay to headless, so in TUI mode `ask.ts` registers the thread
   * on the owner surface and returns that notice — which, since 260905, is
   * the LEAD NOTICE to push: a defined return means the question itself is
   * answered on the owner surface, not by the lead, but the returned string
   * is still pushed to the lead as a `ws-agent-advisory`/`fork-question-thread`
   * message in place of the `ws-agent-question` the headless baseline would
   * send. Set by `fork.ts`'s `registerFork`; `spawner.ts`
   * stays generic and supplies no implementation, mirroring
   * `onApprovalPending`'s existing callback-injection convention.
   */
  onQuestionReport?: (record: RpcAgentRecord, message: string) => string | undefined;
  /**
   * 260908 same-process finish callback, owned by ask.ts. It persists the
   * ordinary thread snapshot only after this coordinator parks the fork.
   */
  onForkFinishComplete?: (record: RpcAgentRecord, failed?: string) => void;
  /**
   * 260905 (review relay #1, I1): fired by `sendToAgent`'s dormant-resume
   * branch right after the fresh client's event listener is attached. It
   * exists for role wiring that needs a LIVE client and therefore cannot be
   * re-armed at revival time — `fork.ts`'s `wireAntiBleedLoop`, which
   * subscribes to `client.onEvent` and returns early when the record is
   * dormant. A record revived from the shutdown sidecar carries this so its
   * first `ws-agent-send` restores the fork wiring rather than silently
   * degrading it to plain-worker behavior. Hook-only state, never serialized.
   */
  onResume?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I1): a per-RECORD fallback for the approval relay
   * that `RpcSpawnCtx`/`RpcResumeCtx` normally carry per CALL SITE.
   * `attachEventListener` prefers the ctx callback and falls back to this one,
   * so an `execute-worker` keeps its relay even when it is resumed from a call
   * site that has none — the shutdown sidecar's role-keyed revival sets it
   * (making the re-arm explicit rather than a coincidence of which resume path
   * ran), and `ask.ts`'s overlay channel resume, which passes no
   * `onApprovalPending` of its own, gets it for free.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
}

interface AgentContextStats {
  sessionFile?: string;
  sessionId?: string;
  contextUsage?: { tokens?: unknown };
}

type ContextReading = { kind: "value"; tokens: number } | { kind: "unknown" } | { kind: "unavailable" };

function contextReading(stats: AgentContextStats | undefined, path: string, sessionId: string | undefined): ContextReading {
  if (!stats || !sessionId || stats.sessionId !== sessionId || stats.sessionFile !== path || stats.contextUsage === undefined) return { kind: "unavailable" };
  const tokens = typeof stats.contextUsage.tokens === "number" && Number.isFinite(stats.contextUsage.tokens) && stats.contextUsage.tokens >= 0
    ? stats.contextUsage.tokens
    : undefined;
  return tokens === undefined ? { kind: "unknown" } : { kind: "value", tokens };
}

function nextContextTokens(previous: number | undefined, fallback: number | undefined, reading: ContextReading, refreshContext = true): number | undefined {
  if (!refreshContext) return previous;
  if (reading.kind === "value") return reading.tokens;
  if (reading.kind === "unknown") return previous;
  return fallback ?? previous;
}

async function getAgentRpcSnapshot(client: RpcClient, includeStats = true): Promise<{ state: Awaited<ReturnType<RpcClient["getState"]>>; stats?: AgentContextStats }> {
  const state = await client.getState();
  if (!includeStats || typeof client.getSessionStats !== "function") return { state };
  try { return { state, stats: await client.getSessionStats() }; }
  catch { return { state }; }
}

/** Binds once before the first prompt and only recomputes from durable IDs thereafter. */
export function refreshAgentTelemetry(
  record: RpcAgentRecord,
  state?: { sessionFile?: string; sessionId?: string; model?: { provider?: string; id?: string }; thinkingLevel?: string },
  opts?: { fresh?: boolean; stats?: AgentContextStats; refreshContext?: boolean },
): boolean {
  const snapshot = () => JSON.stringify({ telemetry: record.telemetry, floor: record.telemetryContextFloor, model: record.observedModel, effort: record.observedEffort, context: record.observedContextTokens });
  const before = snapshot();
  const finish = (): boolean => {
    const changed = before !== snapshot();
    if (record.ownership) updateOwnership(record.ownership.home, { telemetry: record.telemetry });
    return changed;
  };
  const path = state?.sessionFile ?? record.sessionPath;
  const read = readSessionEntries(path);
  const sessionId = state?.sessionId ?? (read && !("transient" in read) ? read.headerId : undefined);
  const reading = contextReading(opts?.stats, path, sessionId);
  const model = state?.model?.provider && state.model.id ? `${state.model.provider}/${state.model.id}` : undefined;
  if (model) record.observedModel = model; else if (state) delete record.observedModel;
  if (typeof state?.thinkingLevel === "string" && state.thinkingLevel) record.observedEffort = state.thinkingLevel; else if (state) delete record.observedEffort;
  if (!record.telemetry) {
    if (!sessionId) return finish();
    // A non-fork legacy child has no inherited history and can be recovered
    // completely. A fork without its saved boundary must remain unknown.
    if (read && !("transient" in read) && read.parentSession && opts?.fresh) {
      const anchor = read.entries.at(-1)?.id;
      if (!anchor) return finish();
      record.telemetry = { version: 1, origin: { sessionId, sessionPath: path, prefixEntryId: anchor } };
    } else if (read && !("transient" in read) && read.parentSession && !opts?.fresh) {
      const existingFloor = record.telemetryContextFloor;
      if (existingFloor && (existingFloor.sessionId !== sessionId || existingFloor.sessionPath !== path)) {
        delete record.telemetryContextFloor;
        delete record.observedContextTokens;
      }
      if (!record.telemetryContextFloor) record.telemetryContextFloor = { sessionId, sessionPath: path, ...(read.entries.at(-1)?.id ? { prefixEntryId: read.entries.at(-1)!.id } : { emptyPrefix: true }) };
      const floor = reduceTelemetry(record.telemetryContextFloor, read);
      record.observedContextTokens = nextContextTokens(record.observedContextTokens, floor?.contextTokens, reading, opts?.refreshContext !== false);
      if (record.observedContextTokens === undefined) delete record.observedContextTokens;
      return finish();
    }
    if (!read || ("transient" in (read ?? {}))) return finish();
    if (!record.telemetry) record.telemetry = { version: 1, origin: { sessionId, sessionPath: path, emptyPrefix: true } };
  }
  const telemetry = record.telemetry;
  if (telemetry.origin.sessionPath !== path || (sessionId && telemetry.origin.sessionId !== sessionId)) { delete record.telemetry; delete record.telemetryContextFloor; delete record.observedContextTokens; return finish(); }
  if (model) telemetry.model = model; else if (state) delete telemetry.model;
  if (typeof state?.thinkingLevel === "string" && state.thinkingLevel) telemetry.effort = state.thinkingLevel; else if (state) delete telemetry.effort;
  if (read && "transient" in read) return finish();
  const reduced = reduceTelemetry(telemetry.origin, read);
  if (!reduced) { delete record.telemetry; delete record.telemetryContextFloor; delete record.observedContextTokens; return finish(); }
  const previousContext = telemetry.contextTokens;
  delete telemetry.estimatedUsd; delete telemetry.partialEstimatedUsd;
  Object.assign(telemetry, reduced);
  telemetry.contextTokens = nextContextTokens(previousContext, reduced.contextTokens, reading, opts?.refreshContext !== false);
  if (telemetry.contextTokens === undefined) delete telemetry.contextTokens;
  return finish();
}

/**
 * One observed `ws-report-to-lead` call on a record: its `kind` (260904's
 * `"question"`/`"final"` fork disambiguation; absent for a plain
 * `full-worker`/`execute-worker` progress update) and the epoch-ms time it was
 * seen. The message text is deliberately NOT retained — under the push model
 * it has already been delivered to the lead by the time this entry is
 * appended.
 */
export interface AgentReportLogEntry {
  kind?: "question";
  at: number;
}

export type RpcAgentRegistry = Map<string, RpcAgentRecord>;

/**
 * 260905 (alias/park/cap ticket): the ONE alias-or-uuid resolution helper
 * every `agent_id` param goes through (`sendToAgent`, `stopAgent`,
 * `getAgentTranscriptPath`, and `execute-gateway.ts`'s `ws-approve`), so a
 * caller may pass either the raw uuid or a record's `alias` interchangeably.
 * Direct `registry.has(idOrAlias)` wins first (the uuid path, cheap and
 * unambiguous); otherwise scans for a record whose `alias` matches exactly.
 * Returns `undefined` on a genuine miss — each call site's existing "unknown
 * agentId" error path is left unchanged by falling back to the original
 * input (`resolveAgentId(registry, id) ?? id`).
 */
export function resolveAgentId(registry: RpcAgentRegistry, idOrAlias: string): string | undefined {
  if (registry.has(idOrAlias)) return idOrAlias;
  for (const record of registry.values()) {
    if (record.alias === idOrAlias) return record.agentId;
  }
  return undefined;
}

export type AgentStatus = "running" | "idle" | "dormant" | "waiting-on-children" | "pending-delivery";

/**
 * 260905: the three RPC-backed spawn shapes, recorded on the record at spawn
 * time (`RpcAgentRecord.spawnRole`). Narrower than `process-role.ts`'s
 * `SpawnRole` on purpose — that one describes the CHILD process's own view of
 * itself (`worker`/`fork`/`explore`) as carried in its env, while this one is
 * the PARENT's classification of what it spawned, and must distinguish an
 * `execute-worker` (approval-gated) from a plain worker so the shutdown
 * sidecar can re-arm the right wiring on revival.
 *
 * `"explore"` is the fourth shape: every eligible dispatcher creates the same
 * persistent RPC researcher record, sidecar-restored without worker/fork-specific
 * wiring.
 */
export type SpawnAgentRole = "worker" | "execute-worker" | "fork" | "explore";

/** Per-agent cap on retained `reportLog` entries (drop-oldest); see `RpcAgentRecord.reportLog`. */
export const REPORT_LOG_CAP = 64;

/**
 * 260905 (alias/park/cap ticket): env var the LEAD process reads at spawn
 * time to override the registry cap (`DEFAULT_AGENT_REGISTRY_CAP`). Never
 * forwarded to a child's env — this is lead-process-only config, unlike the
 * child-carried role markers in `process-role.ts`.
 */
export const WS_PI_AGENT_REGISTRY_CAP_ENV = "WS_PI_AGENT_REGISTRY_CAP";

/** Default registry cap when `WS_PI_AGENT_REGISTRY_CAP_ENV` is unset or unparsable. */
export const DEFAULT_AGENT_REGISTRY_CAP = 256;

/**
 * Pure env-param resolver for the registry cap, mirroring `shouldPushToLead`'s
 * testable-default-param shape. A missing, non-numeric, or non-positive value
 * falls back to `DEFAULT_AGENT_REGISTRY_CAP` rather than throwing or producing
 * a cap of zero/negative that would reject every spawn.
 */
export function resolveAgentRegistryCap(env: NodeJS.ProcessEnv = process.env): number {
  const raw = env[WS_PI_AGENT_REGISTRY_CAP_ENV];
  if (typeof raw !== "string" || raw.trim() === "") return DEFAULT_AGENT_REGISTRY_CAP;
  const parsed = Number(raw);
  return Number.isFinite(parsed) && parsed > 0 ? Math.floor(parsed) : DEFAULT_AGENT_REGISTRY_CAP;
}

/**
 * Byte cap on the stored copy of a spawn's `prompt` (`RpcAgentRecord.prompt`),
 * applied once at spawn time — bounds the record itself, the shutdown sidecar
 * that serializes it, and the `ws-agent-list` `include_prompt` reply alike.
 */
export const PROMPT_STORAGE_CAP_BYTES = 4096;

/** Marker line appended by `truncatePromptForStorage` when a prompt was cut. */
const PROMPT_TRUNCATION_MARKER = "\n…[truncated for storage]";

/**
 * Byte-safe head-truncation of a prompt for storage: cuts at `capBytes` UTF-8
 * bytes (never mid-codepoint — a naive `Buffer.slice` cut can straddle a
 * multibyte sequence and corrupt the tail) and appends a cut-marker line when
 * truncation actually happened.
 */
export function truncatePromptForStorage(prompt: string, capBytes: number = PROMPT_STORAGE_CAP_BYTES): string {
  const buf = Buffer.from(prompt, "utf8");
  if (buf.length <= capBytes) return prompt;
  const decoder = new StringDecoder("utf8");
  const head = decoder.write(buf.subarray(0, capBytes));
  return `${head}${PROMPT_TRUNCATION_MARKER}`;
}

/**
 * 260905: the six push families. Each is a Pi custom-message `customType`
 * delivered by `pushToLead` into whichever session owns the child:
 *
 * - `ws-agent-report` — a `ws-report-to-lead` progress update or intermediate
 *   finding, pushed at once and never treated as terminal.
 * - `ws-agent-settled` — the child stopped producing: `reason` is `"idle"`
 *   (ordinary terminal settlement carrying `last_message`),
 *   `"stopped"` (an explicit `ws-agent-stop`), `"exited"` (its process died —
 *   see the liveness probe), or `"spawn-failed"`.
 * - `ws-agent-question` — a headless `kind:"question"` report the lead itself
 *   must answer (in TUI the owner surface consumes it, and the lead instead
 *   gets the `fork-question-thread` advisory below).
 * - `ws-agent-approval` — an `execute-worker` is blocked on `ws-approve`.
 * - `ws-agent-advisory` — the adapter's own statement about a child, including
 *   this module's question branch registering a fork-raised thread
 *   (`advisory: "fork-question-thread"`, `followUp`).
 * - `ws-agent-orphaned` — children that outlived their lead session and are
 *   revivable with `ws-agent-send` (shutdown sidecar, `agent-sidecar.ts`).
 */
export const PUSH_FAMILIES = [
  "ws-agent-report",
  "ws-agent-settled",
  "ws-agent-question",
  "ws-agent-approval",
  "ws-agent-advisory",
  "ws-agent-orphaned",
] as const;

export type PushFamily = (typeof PUSH_FAMILIES)[number];

/** Pi's own `sendMessage` delivery axis (`ExtensionAPI.sendMessage`'s `options.deliverAs`). */
export type PushDeliverAs = "steer" | "followUp" | "nextTurn";

/**
 * Every eligible dispatcher injects its direct children's events into its own
 * session. Its own reports separately travel outward over its parent RPC edge.
 */
export function shouldPushToLead(env: NodeJS.ProcessEnv = process.env): boolean {
  const role = readSpawnRole(env);
  return isLeadOrFork(role) || role === "worker" || role === "explore";
}

/**
 * The shared registry walk behind both `computeRunningStatusLine` (below) and
 * the goal-loop yield predicate (`hasRunningAgents`, 260905 Phase 2): skips
 * owner-routed records and reports whether anything counts as "present"
 * (any lead-owned registry member — dormant/parked included) plus how many
 * turns can still make autonomous progress. Delivery and descendant waits
 * are intentionally not execution.
 */
export function isOwnerHeld(record: Pick<RpcAgentRecord, "lastWriter"> | undefined): boolean {
  return record?.lastWriter === "owner";
}

function computeFanIn(registry: RpcAgentRegistry | undefined): { present: boolean; running: number } {
  let present = false;
  let running = 0;
  for (const record of registry?.values() ?? []) {
    if (record.threadBound || isOwnerHeld(record)) continue;
    present = true;
    if (record.running || record.streaming) running += 1;
  }
  return { present, running };
}

/**
 * The fan-in status line every pushed message carries: `N delegated agents
 * still running`, computed fresh at push time over the shared registry.
 *
 * - The line is PRESENT whenever the registry holds any member that is NOT
 *   `threadBound` — 260905 (alias/park/cap ticket) keys this on registry
 *   membership, not on a live client: a dormant/parked record still counts as
 *   present, since automatic parking (see the settle handler below) now
 *   routinely turns a settled, non-threadBound child dormant. Persistent
 *   researchers are members of this same registry.
 * - N counts only turns still executing (`running || streaming`). A pending
 *   terminal delivery, descendant wait, or owner action is not execution.
 *   `0 delegated agents still running` is the lead's synthesis cue —
 *   and, per the presence rule above, `0 …` stays visible (not omitted) as
 *   long as any non-threadBound record — dormant included — remains
 *   registered; cap eviction is what eventually removes it.
 *
 * Owner decision after the second live run (2026-09-05): the former
 * running-out-of-total form with an id suffix is gone. The total only ever
 * grew across a session (an idle child keeps its process, so it stayed counted
 * until stopped/exited) and the id suffix duplicated what `ws-agent-list`
 * already answers, so both were noise. The omission rule survives unchanged:
 * when nothing at all is registered (or every member is threadBound) there is
 * no line at all (`undefined`), so a push that has nothing to do with
 * delegation fan-in — a `ws-agent-orphaned` roll-call at session start, a
 * `spawn-failed` for the only child — never ends with a contentless zero line.
 */
export function computeRunningStatusLine(registry: RpcAgentRegistry | undefined): string | undefined {
  const { present, running } = computeFanIn(registry);
  if (!present) return undefined;
  return `${running} delegated agent${running === 1 ? "" : "s"} still running`;
}

/**
 * Phase 2 (260905) goal-loop yield predicate: true when N > 0 under the
 * same fan-in walk computeRunningStatusLine uses, so the yield decision
 * can never drift from the pushed status line's own arithmetic.
 */
export function hasRunningAgents(registry: RpcAgentRegistry | undefined): boolean {
  return computeFanIn(registry).running > 0;
}

/**
 * Renders a pushed message's human-readable `content`. `details` carries the
 * same fields structurally (that is what a renderer/tool would read); this
 * body is what the lead's model actually sees in its transcript, so it stays
 * plain text with the status line last. An absent status (nothing delegated,
 * see `computeRunningStatusLine`) contributes no line at all.
 */
export function buildPushContent(
  family: PushFamily,
  agentId: string | undefined,
  payload: Record<string, unknown>,
  status: string | undefined,
): string {
  const head = agentId ? `[${family}] agent ${agentId}` : `[${family}]`;
  // Orphan recovery keeps its tool-facing fields in `details`, but its model
  // copy is an action block rather than YAML-like metadata. In particular,
  // `idle_agent_ids` remains structured without enumerating dormant IDs in
  // prose. Other orphan-shaped payloads retain the ordinary generic rendering.
  const body = family === "ws-agent-orphaned" && typeof payload.agents === "string"
    ? payload.agents.split("\n")
    : Object.entries(payload)
      .filter(([, value]) => value !== undefined && value !== null && value !== "")
      .map(([key, value]) => `${key}: ${typeof value === "string" ? value : JSON.stringify(value)}`);
  return [head, ...body, ...(status ? [status] : [])].join("\n");
}

/** Live session idleness accessor, supplied at session_start. Missing or stale accessors never authorize a custom turn start. */
export const leadIdleRef: { current: (() => boolean) | undefined } = { current: undefined };

/** Independent compaction hold, set before any compaction and cleared only by deferred completion/failure or lever callbacks. agent_start is not proof of release. */
export const leadCompactingRef: { current: boolean } = { current: false };

/** Shared reservation for a push or reminder user prompt awaiting agent_start. All pushes stay held until confirmed start, settle, or recovery timeout clears this reservation. */
export const leadWakeStartPendingRef: { current: boolean } = { current: false };

export interface WakeStartOptions {
  delayMs: () => number;
  scheduleTimer?: (cb: () => void, ms: number) => NodeJS.Timeout;
  clearTimer?: (handle: NodeJS.Timeout) => void;
}
let wakeOptions: WakeStartOptions | undefined;
let cancelWakeTimeout: (() => void) | undefined;

export function clearWakeStart(): void {
  cancelWakeTimeout?.();
  cancelWakeTimeout = undefined;
  leadWakeStartPendingRef.current = false;
}

/** Reserve before prompt preflight; recovery exists even when dispatch throws. */
export function reserveWakeStart(options: WakeStartOptions, onTimeout: () => void): boolean {
  if (leadWakeStartPendingRef.current || !shouldPushToLead()) return false;
  leadWakeStartPendingRef.current = true;
  const schedule = options.scheduleTimer ?? ((cb, ms) => {
    const timer = setTimeout(cb, ms);
    timer.unref?.();
    return timer;
  });
  const handle = schedule(() => {
    cancelWakeTimeout = undefined;
    leadWakeStartPendingRef.current = false;
    onTimeout();
  }, options.delayMs());
  cancelWakeTimeout = () => (options.clearTimer ?? clearTimeout)(handle);
  return true;
}

function requestPushWake(pi: ExtensionAPI): void {
  if (!wakeOptions || !heldPushQueue.length || !leadIdleRef.current || !isOwningAgentIdle()) return;
  if (!reserveWakeStart(wakeOptions, () => requestPushWake(pi))) return;
  try {
    pi.sendUserMessage(`${heldPushQueue.length} ws messages waiting; process the incoming reports.`, { deliverAs: "followUp" });
  } catch {
    // Keep the queue and timeout: handled input and rejected preflight need the same retry.
  }
}

/** Admit raw summaries through the very same FIFO as family-shaped reports. */
export function sendToLead(pi: ExtensionAPI, message: Parameters<ExtensionAPI["sendMessage"]>[0], deliverAs: PushDeliverAs): void {
  if (!shouldPushToLead() || !leadIdleRef.current) return;
  admitPush(pi, { kind: "raw", deliverAs, message });
}

function admitPush(pi: ExtensionAPI, held: HeldPush | HeldRawSend): void {
  try {
    leadIdleRef.current?.();
  } catch {
    return; // stale session accessor
  }
  if (leadCompactingRef.current || leadWakeStartPendingRef.current || isOwningAgentIdle() || held.deliverAs === "followUp" || heldPushQueue.length > 0) {
    if (held.kind === "push") {
      if (held.terminal) held.terminal.state = "held";
      held.actionGeneration = held.record?.workGeneration;
      if (held.family === "ws-agent-question") held.questionReport = held.record?.reportLog.at(-1);
    }
    heldPushQueue.push(held);
    requestPushWake(pi);
  } else if (held.kind === "raw") {
    pi.sendMessage(held.message, { deliverAs: held.deliverAs, triggerTurn: true });
  } else {
    sendPush(pi, held.registry, held.record, held.family, held.payload, held.deliverAs, held.terminal);
  }
}

/**
 * 260905 (live-agent widget ticket): the same mutable-ref seam as
 * `leadIdleRef`, filled by `index.ts`'s `session_start` (TUI lead only) with
 * a closure that recomputes `agent-widget.ts`'s rows and repaints the
 * `belowEditor` widget + `setStatus` segment. Lets every registry-transition
 * point in this module (spawn, settle, stop, exit, spawn-failed, an approval
 * or report event) trigger a re-render WITHOUT importing `agent-widget.ts` or
 * `ask.ts` — this module must stay the lower layer, mirroring `ask.ts`'s own
 * "imports FROM spawner.ts, never the reverse" rule. `undefined` outside a
 * TUI lead session (a worker/explore child, a headless lead, a test), in
 * which case `triggerAgentWidgetRefresh` is a silent no-op.
 */
export const agentWidgetRefreshRef: { current: (() => void) | undefined } = { current: undefined };

/**
 * Best-effort fire of `agentWidgetRefreshRef`, swallowing a throw — matches
 * every other push call site's swallow-and-continue convention. Called from
 * every registry-transition point a live-agent-widget row depends on: a
 * throwing or absent refresh must never turn a routine spawn/settle/stop into
 * a crashed event listener.
 */
function triggerAgentWidgetRefresh(): void {
  try {
    agentWidgetRefreshRef.current?.();
  } catch {
    // best effort — see doc comment above.
  }
}

/** Separate event seam for the footer's bounded telemetry reconciliation. */
export const agentCostRefreshRef: { current: (() => void) | undefined } = { current: undefined };
function triggerAgentCostRefresh(): void {
  try {
    agentCostRefreshRef.current?.();
  } catch {
    // Cosmetic accounting must never fail an agent lifecycle transition.
  }
}

/** Read current idleness with the independent compaction hold composed in. Delivery also requires an initialized, live accessor. */
export function isOwningAgentIdle(): boolean {
  if (leadCompactingRef.current) return false;
  const isIdle = leadIdleRef.current;
  if (!isIdle) return true;
  try {
    return isIdle() !== false;
  } catch {
    return false;
  }
}

/** One volatile terminal event's shared-FIFO lifecycle. This is admission
 * bookkeeping only: `enqueued` means Pi accepted the synchronous sendMessage
 * call, not that the model consumed it. */
export interface TerminalDelivery {
  state?: "held" | "enqueued";
  /** Re-evaluate parking and subtree protection after direct-parent admission. */
  afterEnqueue?: () => void;
  /** Retry a failed pre-queue admission without harvesting or notifying twice. */
  retry?: () => void;
}

/** In-memory finish state for one fork-raised `/done`. It intentionally has
 * no persistence representation: reload/death recovery remains best effort. */
export interface ForkFinishOperation {
  token: string;
  cwd: string;
  extensionPath: string;
  generation: number | undefined;
  phase: "waiting" | "evaluating" | "closeout" | "parking" | "complete";
  settled: boolean;
  closeoutIssued: boolean;
  /** `agent_start` observed for the closeout's own run. It fences a duplicate
   * settle from the pre-closeout run. */
  closeoutRunStarted: boolean;
  /** The observed settle that ended the work before closeout. Exact repeated
   * delivery of that event is not evidence that the closeout has settled. */
  preCloseoutSettleEvent?: object;
  terminal?: TerminalDelivery;
  advancing?: Promise<void>;
}

interface HeldPush {
  kind: "push";
  registry: RpcAgentRegistry | undefined;
  record: RpcAgentRecord | undefined;
  family: PushFamily;
  payload: Record<string, unknown>;
  /** Optional same-process terminal delivery reference. */
  terminal?: TerminalDelivery;
  /** Admission mode: governs whether busy delivery holds or interrupts. Confirmed-start delivery always overrides it to `steer`. */
  deliverAs: PushDeliverAs;
  /** Work generation at admission, used to reject controls superseded before the snapshot. */
  actionGeneration?: number;
  /** Exact report-log entry created for a queued headless question. */
  questionReport?: AgentReportLogEntry;
}

/** Pre-built summaries share the family-push FIFO and retain their original structured custom message. */
interface HeldRawSend {
  kind: "raw";
  deliverAs: PushDeliverAs;
  message: Parameters<ExtensionAPI["sendMessage"]>[0];
}

/**
 * Pushes that arrived while the owning session was mid-turn, in arrival order.
 *
 * Phase 1 Edition (live-run fix): Pi queues a `followUp` sent mid-turn in its
 * own `PendingMessageQueue` and delivers it after the turn ends — but there is
 * no extension hook at that delivery (`before_agent_start` fires only for
 * user prompts, including counted wakes; see `agent-session.ts`'s `prompt()` vs
 * `sendCustomMessage`), so a message built at ARRIVAL time carried a status
 * line already stale by the time the lead read it. A worker that finished
 * while the lead was still spawning its siblings reported zero still running
 * (its siblings were not registered yet), and so did the next — three
 * separate invitations to synthesize early. Holding the push here and building
 * the message at FLUSH
 * time is the fix: the status line is then computed against the registry as it
 * stands when the lead actually reads it.
 *
 * Exported for the flush handler and for test isolation only; nothing else may
 * write it. Held pushes are in-memory and die with the process, exactly like
 * the Pi queue they stand in for — `session_shutdown` drops them rather than
 * persisting them (the sidecar carries child IDENTITIES, never reports).
 *
 * 260906 (compaction push-hold ticket, Phase 1): also holds `HeldRawSend`
 * entries (see that type's doc comment) so `ask.ts`'s
 * `injectDiscussionSummary` can share this same queue/flush-ordering
 * mechanism instead of maintaining a parallel one.
 */
export const heldPushQueue: Array<HeldPush | HeldRawSend> = [];

function xmlEscape(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

function customContentText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return String(content ?? "");
  return content.map((part) => {
    const candidate = part as { type?: unknown; text?: unknown };
    return candidate?.type === "text" && typeof candidate.text === "string"
      ? candidate.text
      : JSON.stringify(part);
  }).join("\n");
}

function batchItemIdentity(item: PushBatchItem): { agentId?: string; commandId?: string } {
  const details = item.details as { agent_id?: unknown; cmd_id?: unknown } | undefined;
  return {
    agentId: typeof details?.agent_id === "string" ? details.agent_id : undefined,
    commandId: typeof details?.cmd_id === "string" ? details.cmd_id : undefined,
  };
}

/** Serialize a FIFO snapshot without truncation; actionable summaries follow every ordered message. */
function buildPushBatchContent(items: readonly PushBatchItem[]): string {
  const lines = [`<${PUSH_BATCH_CUSTOM_TYPE} version="${PUSH_BATCH_VERSION}">`];
  for (const item of items) {
    const { agentId, commandId } = batchItemIdentity(item);
    const attrs = [
      `type="${xmlEscape(item.customType)}"`,
      `state="${item.state}"`,
      ...(agentId ? [`agent-id="${xmlEscape(agentId)}"`] : []),
      ...(commandId ? [`command-id="${xmlEscape(commandId)}"`] : []),
    ];
    lines.push(`  <message ${attrs.join(" ")}>${xmlEscape(customContentText(item.content))}</message>`);
  }
  const actions = items.flatMap((item) => {
    if (item.state !== "actionable") return [];
    const { agentId, commandId } = batchItemIdentity(item);
    if (item.customType === "ws-agent-approval" && agentId && commandId) {
      return [`    <action type="approval" agent-id="${xmlEscape(agentId)}" command-id="${xmlEscape(commandId)}">Call ws-approve with agent_id=&quot;${xmlEscape(agentId)}&quot; and cmd_id=&quot;${xmlEscape(commandId)}&quot;.</action>`];
    }
    if (item.customType === "ws-agent-question" && agentId) {
      return [`    <action type="question" agent-id="${xmlEscape(agentId)}">Call ws-agent-send with agent_id=&quot;${xmlEscape(agentId)}&quot; and the answer.</action>`];
    }
    return [];
  });
  lines.push("  <action-summary>", ...(actions.length ? actions : ["    none"]), "  </action-summary>", `</${PUSH_BATCH_CUSTOM_TYPE}>`);
  return lines.join("\n");
}

function heldActionState(held: HeldPush): PushBatchItemState {
  if (held.family === "ws-agent-approval") {
    const cmdId = typeof held.payload.cmd_id === "string" ? held.payload.cmd_id : undefined;
    const pending = held.record?.pendingApproval;
    return cmdId && held.record?.workGeneration === held.actionGeneration && pending?.cmdId === cmdId && pending.decisionWritten !== true
      ? "actionable"
      : "superseded";
  }
  if (held.family === "ws-agent-question") {
    const latest = held.record?.reportLog.at(-1);
    return held.record?.workGeneration === held.actionGeneration && latest === held.questionReport && latest?.kind === "question"
      ? "actionable"
      : "superseded";
  }
  return "informational";
}

function materializeHeldPush(held: HeldPush | HeldRawSend): PushBatchItem {
  if (held.kind === "raw") {
    return {
      customType: held.message.customType,
      content: held.message.content as string | unknown[],
      display: held.message.display,
      details: held.message.details,
      state: "informational",
    };
  }
  const status = computeRunningStatusLine(held.registry);
  const base: Record<string, unknown> = held.record ? { agent_id: held.record.agentId, ...held.payload } : { ...held.payload };
  const details = status ? { ...base, status } : base;
  const displayId = held.record?.alias ? `${held.record.alias} (${held.record.agentId})` : held.record?.agentId;
  return {
    customType: held.family,
    content: buildPushContent(held.family, displayId, held.payload, status),
    display: true,
    details,
    state: heldActionState(held),
  };
}

/** Build current family status and send into a confirmed streaming run with the requested delivery mode. */
function sendPush(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
  terminal?: TerminalDelivery,
): void {
  const wasHeld = terminal?.state === "held";
  const status = computeRunningStatusLine(registry);
  const base: Record<string, unknown> = record ? { agent_id: record.agentId, ...payload } : { ...payload };
  const details = status ? { ...base, status } : base;
  // 260905 (alias/park/cap ticket): pushed-message heads print the alias when
  // there is one, followed by the uuid — a one-line, zero-signature-churn
  // change: compose the display id here and pass it through
  // `buildPushContent`'s existing bare-`agentId` parameter, leaving that
  // helper (and `details.agent_id`, which stays the raw uuid) untouched.
  const displayId = record?.alias ? `${record.alias} (${record.agentId})` : record?.agentId;
  try {
    pi.sendMessage(
      {
        customType: family,
        content: buildPushContent(family, displayId, payload, status),
        display: true,
        details: details as never,
      },
      { deliverAs, triggerTurn: true },
    );
    if (terminal) {
      terminal.state = "enqueued";
      if (wasHeld) terminal.afterEnqueue?.();
    }
  } catch {
    // Best effort: a push that cannot be delivered (a torn-down session, a
    // host that rejected the message) must never turn a child's routine
    // report into a crashed event listener.
  }
}

/**
 * Release one immutable prefix snapshot as one custom message. The prefix stays
 * queued until `sendMessage` returns synchronously; rejection restores every
 * terminal obligation and lets the ordinary settle/wake path retry it.
 */
function submitHeldPushBatch(pi: ExtensionAPI, deliverAs: "steer" | "followUp"): number {
  if (heldPushQueue.length === 0) return 0;
  const snapshot = heldPushQueue.slice();
  const terminalStates = snapshot.flatMap((held) => held.kind === "push" && held.terminal
    ? [{ terminal: held.terminal, wasHeld: held.terminal.state === "held" }]
    : []);
  const items = snapshot.map(materializeHeldPush);
  try {
    pi.sendMessage(
      {
        customType: PUSH_BATCH_CUSTOM_TYPE,
        content: buildPushBatchContent(items),
        display: true,
        details: { version: PUSH_BATCH_VERSION, items } as never,
      },
      { deliverAs, triggerTurn: true },
    );
  } catch {
    requestPushWake(pi);
    return 0;
  }
  heldPushQueue.splice(0, snapshot.length);
  for (const { terminal, wasHeld } of terminalStates) {
    terminal.state = "enqueued";
    if (wasHeld) terminal.afterEnqueue?.();
  }
  return snapshot.length;
}

/** Idle release requests one counted user wake without draining. Confirmed starts and lead turn boundaries each release one FIFO batch. */
export function flushHeldPushes(pi: ExtensionAPI | undefined, confirmedStart = false, agentEndBoundary = false): number {
  if (!pi || !shouldPushToLead() || !leadIdleRef.current || leadCompactingRef.current || leadWakeStartPendingRef.current) return 0;
  if (agentEndBoundary) return submitHeldPushBatch(pi, "followUp");
  if (!confirmedStart) {
    requestPushWake(pi);
    return 0;
  }
  return submitHeldPushBatch(pi, "steer");
}

/** Factory-scope wake lifecycle, also active in fork owners. Registration allocates no timers; only a reserved user wake does. Worker/explore roles never reserve wakes. */
export function registerPushFlush(pi: ExtensionAPI, options: WakeStartOptions): void {
  wakeOptions = options;
  pi.on("agent_start", () => {
    clearWakeStart();
    flushHeldPushes(pi, true);
  });
  pi.on("agent_end", () => {
    flushHeldPushes(pi, false, true);
  });
  pi.on("agent_settled", () => {
    clearWakeStart();
    flushHeldPushes(pi);
  });
  pi.on("session_shutdown", () => {
    clearWakeStart();
    heldPushQueue.length = 0;
    leadIdleRef.current = undefined;
    wakeOptions = undefined;
  });
}

/** `true` while a settled result still has not reached its direct parent's queue. */
export function hasPendingTerminalDelivery(record: Pick<RpcAgentRecord, "terminalDelivery">): boolean {
  return record.terminalDelivery !== undefined && record.terminalDelivery.state !== "enqueued";
}

function createTerminalDelivery(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry | undefined,
  pi: ExtensionAPI | undefined,
  generation: number | undefined,
  payload?: Record<string, unknown>,
): TerminalDelivery {
  const terminal: TerminalDelivery = {};
  terminal.afterEnqueue = () => {
    if (record.workGeneration !== generation || record.terminalDelivery !== terminal) return;
    syncOwnershipProtection(record);
    publishSubtree(registry);
    if (registry && record.client && !record.running && !record.streaming && !record.threadBound && !isOwnerHeld(record) && !record.waitingOnChildren) {
      void stopAgent(registry, record.agentId, pi, { silent: true });
    }
  };
  if (payload) terminal.retry = () => {
    if (terminal.state !== undefined || record.workGeneration !== generation || record.terminalDelivery !== terminal) return;
    pushToLead(pi, registry, record, "ws-agent-settled", payload, "followUp", terminal);
  };
  record.terminalDelivery = terminal;
  syncOwnershipProtection(record);
  publishSubtree(registry);
  return terminal;
}

/** Starts (or joins) one same-process finish operation for a fork-raised
 * owner `/done`. The operation deliberately has no persisted identity: its
 * only promise is race-safe coordination while this adapter instance lives. */
export function startForkFinish(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry,
  pi: ExtensionAPI,
  resumeCtx: Pick<RpcResumeCtx, "cwd" | "extensionPath">,
): ForkFinishOperation {
  if (record.forkFinish) return record.forkFinish;
  claimLeadOwnership(record);
  const operation: ForkFinishOperation = {
    token: randomUUID(),
    cwd: resumeCtx.cwd,
    extensionPath: resumeCtx.extensionPath,
    generation: record.launchGeneration,
    phase: "closeout",
    settled: false,
    closeoutIssued: true,
    // A follow-up joins an already-running turn and produces no fresh
    // `agent_start`; its next settle still belongs to this handoff.
    closeoutRunStarted: record.running || record.streaming,
  };
  record.forkFinish = operation;
  void sendToAgent(registry, { ...finishResumeCtx(operation), pi, finishToken: operation.token }, record.agentId,
    "The owner closed this side thread. Finish the task now. End with the requested final-response format in your ordinary assistant answer; do not use ws-report-to-lead as a completion channel.", false)
    .catch((err) => {
      if (record.forkFinish === operation && record.launchGeneration === operation.generation) {
        operation.phase = "complete";
        record.forkFinish = undefined;
        try { record.onForkFinishComplete?.(record, `closeout failed: ${err instanceof Error ? err.message : String(err)}`); } catch { /* best effort */ }
      }
    });
  return operation;
}

/** The single owner of finish advancement. Every call joins `advancing`, so
 * report/settle/duplicate-callback races cannot issue a second closeout. */
function advanceForkFinish(
  record: RpcAgentRecord,
  registry: RpcAgentRegistry,
  pi: ExtensionAPI,
  resumeCtx: Pick<RpcResumeCtx, "cwd" | "extensionPath">,
  operation: ForkFinishOperation,
): Promise<void> {
  if (operation.advancing) return operation.advancing;
  operation.advancing = (async () => {
    if (record.forkFinish !== operation || operation.phase === "complete") return;
    if (record.launchGeneration !== operation.generation || !operation.settled || record.waitingOnChildren) return;
    operation.phase = "evaluating";

    const existing = operation.terminal ?? record.terminalDelivery;
    if (existing?.state === "enqueued") {
      await parkForkFinish(record, registry, pi, operation);
      return;
    }
    if (existing?.state === "held") {
      operation.terminal = existing;
      return;
    }
    if (existing) {
      existing.retry?.();
      return;
    }

    const generation = record.workGeneration;
    const lastMessage = await harvestLastMessage(record);
    if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || record.workGeneration !== generation || record.running || record.streaming || record.waitingOnChildren) return;
    const terminal = createTerminalDelivery(record, registry, pi, generation, { reason: "idle", last_message: lastMessage });
    operation.terminal = terminal;
    terminal.afterEnqueue = () => {
      if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || record.workGeneration !== generation) return;
      syncOwnershipProtection(record);
      publishSubtree(registry);
      void parkForkFinish(record, registry, pi, operation);
    };
    terminal.retry?.();
  })().finally(() => {
    operation.advancing = undefined;
    // A settle can arrive while the closeout RPC promise is unresolved. The
    // listener joined the in-flight advance above; replay exactly once after
    // it releases rather than stranding an already-idle fork.
    if (record.forkFinish === operation && operation.phase === "closeout" && operation.settled) {
      void advanceForkFinish(record, registry, pi, resumeCtx, operation);
    }
  });
  return operation.advancing;
}

function finishResumeCtx(operation: ForkFinishOperation): Pick<RpcResumeCtx, "cwd" | "extensionPath"> {
  return { cwd: operation.cwd, extensionPath: operation.extensionPath };
}

async function parkForkFinish(record: RpcAgentRecord, registry: RpcAgentRegistry, pi: ExtensionAPI, operation: ForkFinishOperation, failure?: string): Promise<void> {
  if (record.forkFinish !== operation || record.launchGeneration !== operation.generation || operation.phase === "parking" || operation.phase === "complete" || record.waitingOnChildren) return;
  operation.phase = "parking";
  // Preserve the thread bind until this coordinator owns the park; stopAgent
  // performs the normal silent stop and clears it synchronously.
  try {
    let stopped = true;
    await stopAgent(registry, record.agentId, pi, { silent: true, onStopped: (success) => { stopped = success; } });
    if (!stopped) failure ??= "park failed: child stop did not complete";
  } catch (err) {
    failure ??= `park failed: ${err instanceof Error ? err.message : String(err)}`;
  }
  if (record.forkFinish !== operation || record.launchGeneration !== operation.generation) return;
  operation.phase = "complete";
  record.forkFinish = undefined;
  try { record.onForkFinishComplete?.(record, failure); } catch { /* ordinary thread persistence is best effort */ }
}

/** Feed report/settle/tool-result observations into an active coordinator. */
function observeForkFinishEvent(record: RpcAgentRecord, evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown }): void {
  const operation = record.forkFinish;
  if (!operation || record.launchGeneration !== operation.generation) return;
  if (evt.type === "agent_start") {
    if (operation.closeoutIssued) operation.closeoutRunStarted = true;
    return;
  }
  if (evt.type === "agent_settled") {
    // The original settle begins closeout. Once closeout starts, both its own
    // start AND a different settle event are required: RPC listeners can
    // replay the exact old object after the new run has begun.
    if (!operation.closeoutIssued) {
      operation.preCloseoutSettleEvent = evt;
      operation.settled = true;
    } else if (operation.closeoutRunStarted && evt !== operation.preCloseoutSettleEvent) {
      operation.settled = true;
    }
    return;
  }
}

/** Admit a family push through the shared FIFO. Idle, compacting, pending-start, and ordinary busy followUps are held; normal busy steers still interrupt. Idle wakes use sendUserMessage so before_agent_start composes the ws block for the run. */
export function pushToLead(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord | undefined,
  family: PushFamily,
  payload: Record<string, unknown>,
  deliverAs: PushDeliverAs,
  terminal?: TerminalDelivery,
): void {
  const ownerRouteAvailable = ownerNotifyRef.current !== undefined;
  if (record && (isOwnerHeld(record) || (record.threadBound && !record.forkFinish)) && ownerRouteAvailable && (family === "ws-agent-settled" || family === "ws-agent-advisory")) {
    const name = record.alias ?? record.title ?? record.agentId.slice(0, 8);
    const detail = family === "ws-agent-settled"
      ? `settled${typeof payload.last_message === "string" && payload.last_message.trim() ? `: ${payload.last_message.trim()}` : ""}`
      : `advisory: ${String(payload.detail ?? payload.advisory ?? "attention required")}`;
    try {
      ownerNotifyRef.current?.(`ws: ${name} ${detail}`, family === "ws-agent-advisory" ? "warning" : "info");
      if (terminal) {
        terminal.state = "enqueued";
        terminal.afterEnqueue?.();
      }
    } catch { /* human-only best effort; an undelivered terminal stays retryable */ }
    return;
  }
  if (!pi || !shouldPushToLead() || !leadIdleRef.current) return;
  admitPush(pi, { kind: "push", registry, record, family, payload, deliverAs, terminal });
  publishSubtree(registry);
}

/**
 * Single funnel for every `client.prompt(...)` call in this adapter, so the
 * fan-in bookkeeping cannot drift from the actual dispatches: marks the child
 * `running` from the instant the prompt is ISSUED (not when `agent_start`
 * arrives — a lead ending its turn immediately after dispatch must already
 * see it counted), starts a fresh work generation, and stamps
 * `lastLeadPromptAt`.
 *
 * 260905 (live-agent widget ticket): `runStartedAt` is stamped
 * unconditionally, unlike `lastLeadPromptAt` — the widget's elapsed clock
 * resets on a nudge too, since the nudge really did start a fresh turn on
 * the wire even though it is not a new lead-issued task boundary.
 */
interface WriterOperation {
  writer: "lead" | "owner";
  leadPromptAt?: number;
  state: "pending" | "accepted";
}
interface WriterOperationState {
  baseWriter: RpcAgentRecord["lastWriter"];
  baseLeadPromptAt: number | undefined;
  operations: WriterOperation[];
}
const writerOperations = new WeakMap<RpcAgentRecord, WriterOperationState>();
const dispatchOrder = new WeakMap<RpcAgentRecord, { issued: number; accepted: number }>();

function issueDispatch(record: RpcAgentRecord): number {
  const order = dispatchOrder.get(record) ?? { issued: 0, accepted: 0 };
  order.issued++;
  dispatchOrder.set(record, order);
  return order.issued;
}

function acceptDispatch(record: RpcAgentRecord, sequence: number): void {
  const order = dispatchOrder.get(record);
  if (order) order.accepted = Math.max(order.accepted, sequence);
}

function wasSupersededByAcceptedDispatch(record: RpcAgentRecord, sequence: number): boolean {
  return (dispatchOrder.get(record)?.accepted ?? 0) > sequence;
}

function projectWriter(record: RpcAgentRecord, state: WriterOperationState): void {
  let writer = state.baseWriter;
  let leadPromptAt = state.baseLeadPromptAt;
  for (const operation of state.operations) {
    writer = operation.writer;
    if (operation.leadPromptAt !== undefined) leadPromptAt = operation.leadPromptAt;
  }
  record.lastWriter = writer;
  record.lastLeadPromptAt = leadPromptAt;
}

function compactWriterOperations(record: RpcAgentRecord, state: WriterOperationState): void {
  while (state.operations[0]?.state === "accepted") {
    const accepted = state.operations.shift()!;
    state.baseWriter = accepted.writer;
    if (accepted.leadPromptAt !== undefined) state.baseLeadPromptAt = accepted.leadPromptAt;
  }
  projectWriter(record, state);
  if (state.operations.length === 0) writerOperations.delete(record);
  syncOwnershipProtection(record);
  triggerAgentWidgetRefresh();
}

function stampWriter(record: RpcAgentRecord, writer: "lead" | "owner", text?: string, leadPromptAt?: number): { operation: WriterOperation; ownerSend?: { text: string; at: number } } {
  let state = writerOperations.get(record);
  if (!state) {
    state = { baseWriter: record.lastWriter, baseLeadPromptAt: record.lastLeadPromptAt, operations: [] };
    writerOperations.set(record, state);
  }
  const operation: WriterOperation = { writer, ...(leadPromptAt === undefined ? {} : { leadPromptAt }), state: "pending" };
  const ownerSend = writer === "owner" && text !== undefined ? { text, at: Date.now() } : undefined;
  state.operations.push(operation);
  if (ownerSend) (record.ownerSends ??= []).push(ownerSend);
  projectWriter(record, state);
  syncOwnershipProtection(record);
  triggerAgentWidgetRefresh();
  return { operation, ownerSend };
}

function acceptWriter(record: RpcAgentRecord, stamp: ReturnType<typeof stampWriter>): void {
  const state = writerOperations.get(record);
  if (!state || !state.operations.includes(stamp.operation)) return;
  stamp.operation.state = "accepted";
  compactWriterOperations(record, state);
}

function rollbackWriter(record: RpcAgentRecord, stamp: ReturnType<typeof stampWriter>): void {
  const state = writerOperations.get(record);
  if (stamp.ownerSend) {
    const ownerIndex = record.ownerSends?.indexOf(stamp.ownerSend) ?? -1;
    if (ownerIndex >= 0) record.ownerSends!.splice(ownerIndex, 1);
  }
  if (!state) return;
  const index = state.operations.indexOf(stamp.operation);
  if (index >= 0) state.operations.splice(index, 1);
  compactWriterOperations(record, state);
}

function claimLeadOwnership(record: RpcAgentRecord): void {
  const stamp = stampWriter(record, "lead");
  acceptWriter(record, stamp);
}

export async function promptAgent(
  record: RpcAgentRecord,
  client: RpcClient,
  message: string,
  opts?: { isLeadPrompt?: boolean; writer?: "lead" | "owner" },
): Promise<void> {
  const writer = opts?.writer ?? "lead";
  const now = Date.now();
  const writerStamp = stampWriter(record, writer, writer === "owner" ? message : undefined, writer === "lead" && opts?.isLeadPrompt !== false ? now : undefined);
  record.running = true;
  record.workGeneration = (record.workGeneration ?? 0) + 1;
  clearTerminalFacts(record);
  record.runStartedAt = now;
  if (record.ownership) observeSessionWrite(record.ownership.home, record.sessionPath);
  if (record.ownership) updateOwnership(record.ownership.home, { lastActivityAt: now, liveness: { lifecycle: "live", running: true, observedAt: now } });
  const workGeneration = record.workGeneration;
  try {
    await client.prompt(message);
  } catch (error) {
    rollbackWriter(record, writerStamp);
    throw error;
  }
  acceptWriter(record, writerStamp);
  if (record.delegation && record.workGeneration === workGeneration) syncOwnershipProtection(record);
}

/**
 * Transitions `record` to the dead/stopped resting state: no client, not
 * running, not streaming, listener detached. Shared by the liveness probe,
 * the in-flight-rejection paths, and `stopAgent`, so "what a stopped record
 * looks like" is defined once.
 */
function clearTerminalFacts(record: RpcAgentRecord): void {
  record.terminalDelivery = undefined;
}

function clearLiveState(record: RpcAgentRecord): void {
  record.unsubscribe?.();
  record.unsubscribe = undefined;
  record.client = undefined;
  record.streaming = false;
  record.running = false;
}

/**
 * 260905 liveness probe. `RpcClient` exposes no public exit event, but its
 * `send()` throws synchronously once the child process has exited (the
 * bundled client sets `exitError` on the process's own `'exit'`/`'error'`),
 * so a `getState()` round-trip is a reliable liveness test: if it rejects,
 * the child is gone. Called on registry transitions (settle), on a periodic
 * timer while anything is outstanding, and implicitly by every in-flight
 * request rejection routed through `markAgentExited`.
 *
 * Returns `true` when the agent is (still) alive, `false` when this call
 * transitioned it to exited and pushed `ws-agent-settled` `reason:"exited"`.
 */
export async function probeAgentLiveness(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
): Promise<boolean> {
  const client = record.client;
  if (!client) return false;
  try {
    await client.getState();
    return true;
  } catch {
    markAgentExited(pi, registry, record);
    return false;
  }
}

/**
 * Records that `record`'s child process is gone and tells the lead once. Safe
 * to call repeatedly — a record already cleared of its client pushes nothing
 * a second time.
 */
export function markAgentExited(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  opts?: { suppressTerminal?: boolean },
): void {
  if (!record.client) return;
  clearLiveState(record);
  if (record.ownership) updateOwnership(record.ownership.home, { liveness: { lifecycle: "unknown", running: false, observedAt: Date.now() } });
  triggerAgentWidgetRefresh();
  if (opts?.suppressTerminal) return;
  const workGeneration = record.workGeneration;
  if (record.settlementAdmissionGeneration === workGeneration) {
    record.terminalDelivery?.retry?.();
    return;
  }
  record.settlementAdmissionGeneration = workGeneration;
  const terminal = createTerminalDelivery(record, registry, pi, workGeneration, { reason: "exited", last_message: record.lastText });
  terminal.retry?.();
}

/** Interval of the background liveness sweep, while at least one agent is outstanding. */
export const LIVENESS_PROBE_INTERVAL_MS = 30_000;

/**
 * Starts the periodic half of the liveness probe: every
 * `LIVENESS_PROBE_INTERVAL_MS`, probe each still-`running` record. Skipped
 * entirely while nothing is outstanding (N === 0), so an idle lead pays
 * nothing. `unref()`'d so a pending sweep never holds the process open.
 * Returns the stopper (`AgentToolsHandle.stopAll` calls it).
 */
export function startLivenessProbe(
  pi: ExtensionAPI,
  registry: RpcAgentRegistry,
  intervalMs: number = LIVENESS_PROBE_INTERVAL_MS,
): () => void {
  const timer = setInterval(() => {
    for (const record of [...registry.values()]) {
      if ((record.running || record.streaming || record.waitingOnChildren || hasPendingTerminalDelivery(record)) && record.client) {
        void probeAgentLiveness(pi, registry, record);
      }
    }
  }, intervalMs);
  timer.unref?.();
  return () => clearInterval(timer);
}

/**
 * The `spawn-failed` half of `spawnAgent`'s launch-failure handling: put the
 * half-registered record into its resting state and tell the owning session
 * once, so the fan-in count is not left waiting on a child that never started.
 * Every persistent record, including a researcher, remains parked/dormant for
 * ordinary resume and sidecar retention. The caller re-throws the original
 * error unchanged afterwards.
 *
 * Extracted (review relay #1, test partition C2) so this branch has offline
 * coverage — `spawnAgent` itself constructs a real `RpcClient` and is
 * live-gate only.
 */
export function pushSpawnFailed(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  err: unknown,
): void {
  clearLiveState(record);
  pushToLead(pi, registry, record, "ws-agent-settled", { reason: "spawn-failed", error: err instanceof Error ? err.message : String(err) }, "followUp");
  triggerAgentWidgetRefresh();
}

export interface SpawnAgentParams {
  /** Fork-family prompts live in their first user message; workers still require this path. */
  systemPromptPath?: string;
  prompt: string;
  modelName?: string;
  modelEffort?: string;
  /**
   * 260905 (alias/park/cap ticket): optional human-chosen short name for this
   * agent, resolved alongside the raw uuid by `resolveAgentId`. Reusing an
   * alias already held by a `running`/`threadBound` record rejects the spawn;
   * a dormant/idle holder's alias is overwritten (its `title` is untouched).
   * Never derived from `prompt` — the adapter only ever uses what the caller
   * passed.
   */
  alias?: string;
  /** 260905: optional free-text label, independent of `alias` — see `RpcAgentRecord.title`. */
  title?: string;
}

export interface RpcSpawnCtx {
  /** Trusted bridge render (or internal preset), never a public spawn argument. */
  profile?: PlaybookProfile;
  provenance?: RenderProvenance;
  parentPolicy?: DelegationPolicy;
  /** Parent TUI only; initial task-fork observation, never persisted or resumed. */
  forkCacheNoticeOwner?: ForkCacheNoticeOwner;
  /**
   * 260905: the spawning session's own `ExtensionAPI`, needed so every signal
   * this child produces can be PUSHED back into that session (`pushToLead`).
   * Required rather than optional: a spawn with no push channel would leave
   * its child's reports unreachable now that `ws-agent-wait` is gone.
   */
  pi: ExtensionAPI;
  cwd: string;
  /** Exact `fileURLToPath(import.meta.url)` captured by the loaded manifest entry. */
  extensionPath: string;
  /** Immediate dispatcher's configured Pi home and stable current session identity. */
  storage?: AgentStorageContext;
  /** `provider/id`, forwarded from the calling tool-execute ctx.model, or undefined to inherit pi's own default. */
  inheritModel?: string;
  /** Current thinking level, used only when the caller explicitly requests model_effort:"default" with inherited-model dispatch. */
  inheritEffort?: string;
  /** Current execute/command context's live getAll + configured-auth catalog. */
  catalog: readonly ModelCatalogEntry[];
  /** ws-agent-spawn-only public contract; other spawnAgent consumers retain tier-only model_name behavior. */
  allowConcreteModel?: boolean;
  /** UI-only adapter; never a lifecycle push. */
  notifyTierWarning?: (warning: string) => void;
  /** Bridge's sanitized `ws__*` registered tool names, for the `full-worker` group. */
  wsToolNames: readonly string[];
  /** ws-mcp client used to resolve `model_name` through `config.resolve_agent` (`resolveModelForAliasViaWsMcp`), read fresh per spawn. */
  client: McpStdioClient;
  /** Curated `--tools` group for this spawn. Omitted (or explicit `"full-worker"`) preserves every existing `ws-agent-spawn` caller's behavior unchanged — only `ws-execute` (execute-gateway.ts) passes `"execute-worker"`. */
  toolGroup?: ToolGroup;
  /**
   * 260904 Phase 1 (side-thread fork): the lead's own session file to fork
   * from (`toolCtx.sessionManager.getSessionFile()`, `fork.ts`'s
   * `registerFork`). When set, `buildRpcClientOptions` emits `--fork
   * <forkFrom>` instead of `--session <sessionPath>` for THIS initial spawn
   * only — never threaded through `RpcResumeCtx`/`sendToAgent`'s
   * dormant-resume branch, which always resumes via the fork's own
   * already-discovered `sessionPath` (see `spawnAgent`'s `client.getState()`
   * overwrite below). `undefined` for every non-fork spawn (unchanged
   * `--session` behavior).
   */
  forkFrom?: string;
  /** Private launch-only source for Pi's not-yet-flushed first-turn branch. */
  forkSourceEntries?: unknown[];
  /**
   * 260904 Phase 1 (side-thread fork): pre-computed `--tools` value that
   * bypasses `resolveTools(toolGroup, wsToolNames)` when set — see
   * `RpcAgentRecord.explicitTools`'s doc comment for the full rationale.
   * `undefined` for every non-fork spawn.
   */
  explicitTools?: string;
  /**
   * 260904 Phase 1 (side-thread fork): the lead's own default-filled
   * session key (`BridgeHandle.defaultSessionKeyRef.current`), forwarded to
   * `buildRpcClientOptions` so it can set `WS_PI_PARENT_SESSION_KEY_ENV` on
   * the fork child's env — the marker `normalizeSessionKey` (bridge.ts)
   * already rewrites an explicit sentinel `session_key` from. Only
   * meaningful alongside `forkFrom`; ignored otherwise.
   */
  parentSessionKey?: string;
  /**
   * 260904 Phase 1: fired right after `attachEventListener` observes a
   * freshly-set `record.pendingApproval` on this spawn's record — the
   * approval-request-relay injection hook, now told whether a lead-side
   * waiter was already woken by this event (260904 Phase 2, re-scoped
   * 2026-09-05). `spawner.ts` stays generic (no `pi.sendUserMessage` import)
   * by taking this as a plain callback; `execute-gateway.ts`'s
   * `createApprovalRelay` is the only real implementation. Never fires for a
   * non-`"execute-worker"` spawn in practice, since `GATED_EXEC_TOOL_NAME` is
   * reachable only from that group's `--tools` list.
   *
   * 260905 dropped the `info: {waiterWoken}` argument: with `ws-agent-wait`
   * deleted there is no wait return for the relay to be a stale duplicate of,
   * so it now pushes unconditionally.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260906 Phase 2 (YAML/TUI dispatch-row rendering): fired once, right after
   * `spawnAgent` resolves the model (both refusal guards already passed) and
   * before any launch side effect (`runSpawnGuards`/`mkdtempSync`). Every
   * tool-level `spawnAgent` caller forwards this into its own `onUpdate`
   * partial `details.resolved` and repeats it in the final `details` so the
   * dispatch row can show the resolved model/effort before the child
   * finishes. Never invoked when a refusal guard throws first.
   */
  onModelResolved?: (resolved: ResolvedModelInfo) => void;
  /**
   * 260905: recorded on the record as `spawnRole` so the shutdown sidecar can
   * re-arm the right wiring on revival. Defaults to `"fork"` when `forkFrom`
   * is set, `"execute-worker"` for that tool group, `"worker"` otherwise.
   */
  spawnRole?: SpawnAgentRole;
  /** Fail closed before any guard/allocation for simple exploration. */
  requireTier?: boolean;
  /** Internal generated alias prefix; allocation scans the common registry. */
  aliasPrefix?: string;
  /** Persistent exploration metadata; never supplied by the public schema. */
  exploreMode?: ExploreMode;
  /** Immutable lead capture supplied only to a task/discussion fork. */
  forkContext?: ForkContext;
}

export interface RpcResumeCtx {
  /** See `RpcSpawnCtx.pi`. Optional here only because a resume can be driven from a call site with no push channel of its own; pushes are then skipped rather than erroring. */
  pi?: ExtensionAPI;
  cwd: string;
  /** Exact loaded manifest entry path, propagated to a dormant child's replacement process. */
  extensionPath: string;
  /** See `RpcSpawnCtx.onApprovalPending` — threaded through `sendToAgent`'s dormant-auto-resume branch so a resumed `execute-worker`'s approval relay keeps working. */
  onApprovalPending?: (record: RpcAgentRecord) => void;
  /**
   * 260905 (review relay #1, I2): `true` only for the LEAD-facing
   * `ws-agent-send` tool. The lead driving a child directly is the headless
   * answer path for a fork-raised question — the fork was thread-bound from
   * registration and, with no owner surface to ever open or close that thread,
   * nothing else would release the bind before its own final. So a lead send
   * unbinds it: the exchange is now the lead's, and the child rejoins the
   * fan-in immediately rather than staying invisible until it completes.
   *
   * Deliberately NOT set by `ask.ts`'s overlay channel (`createForkChannel`),
   * which routes the OWNER's messages through this same function — an owner
   * typing into an open thread must leave the bind exactly as it is.
   */
  leadSend?: boolean;
  /** Attribution for the message. Omitted is a lead-side prompt. */
  writer?: "lead" | "owner";
  /** Internal token carried only by a coordinator-owned closeout send. */
  finishToken?: string;
}

/**
 * Exported (review fix, cycle 1) so `test/spawner.test.ts` can assert the
 * `WS_PI_SPAWN_ROLE_ENV` marker directly against the built options object
 * instead of leaving it covered only by a manual spot-check — `RpcClient`'s
 * own `env: {...process.env, ...this.options.env}` merge means this function
 * only needs to carry the marker itself (no `process.env` spread here).
 *
 * 260904 Phase 1: carries `WS_PI_SPAWN_ROLE_ENV: "worker"` (see
 * `process-role.ts`), replacing the old boolean `WS_PI_AGENT_CHILD_ENV: "1"`
 * marker this function used to set. Also now carries `WS_PI_APPROVAL_DIR`,
 * derived from `sessionPath`'s own directory (`dirname(sessionPath)`, the
 * same `sessionDir` `spawnAgent`/`sendToAgent` already `mkdtempSync`'d or
 * cached — no new parameter needed since the two paths are always siblings:
 * `sessionPath` is unconditionally `join(sessionDir, "session.jsonl")`).
 * Inert for a non-`"execute-worker"` spawn (nothing in its `--tools` list can
 * ever dispatch `ws-worker-exec` to read this var), so it is folded into the
 * env unconditionally rather than threaded as an extra opt-in parameter.
 *
 * 260904 Phase 1 (side-thread fork) adds the `forkFrom`/`parentSessionKey`
 * params: when `forkFrom` is given, the emitted args swap `["--session",
 * sessionPath, ...]` for `["--fork", forkFrom, ...]` (initial spawn only —
 * `sendToAgent`'s dormant-resume call site never passes `forkFrom`) and the
 * role marker becomes `"fork"` instead of `"worker"`; `parentSessionKey`
 * (only meaningful alongside `forkFrom`) additionally sets
 * `WS_PI_PARENT_SESSION_KEY_ENV` on the child's env, the marker
 * `normalizeSessionKey` (bridge.ts) already rewrites an explicit sentinel
 * `session_key` from — the fork's own bridge instance uses it to mint its
 * own lead-scope key instead of a fresh one. `--fork`'s exact composition
 * with `--mode rpc`/`--tools`/`--append-system-prompt` and its at-leaf vs.
 * before-a-message clone semantics is the ticket's own named live-
 * verification item (not resolvable offline) — this function only builds
 * the argv, it does not confirm Pi's own `--fork` behavior.
 *
 * 260906 (lead explore as an async RPC child) adds the trailing
 * `spawnRoleOverride` param: when given, it wins outright over the
 * `forkFrom ? "fork" : "worker"` default — the lead/fork `explore` preset is
 * the only caller that passes one (`"explore"`, from `process-role.ts`'s
 * `SpawnRole`), so its RPC child's own env reads as an explore leaf rather
 * than a plain worker. `undefined` (every other call site: `spawnAgent`'s
 * non-explore spawns, `sendToAgent`'s dormant-resume branch) leaves the
 * existing default completely unchanged.
 */
export function prepareForkLaunch(context: ForkContext | undefined) {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-fork-launch-"));
  const nonce = randomUUID();
  const contextPath = join(directory, "context.json");
  const readinessPath = join(directory, "ready.json");
  writePrivateJson(contextPath, { context: context ? captureForkContext(context) : undefined, ...(context ? {} : { legacy: true }), nonce, readinessPath });
  return { contextPath, readinessPath, nonce, affinityId: context?.parentAffinityId };
}

export function validateForkReadiness(launch: ReturnType<typeof prepareForkLaunch>, record: RpcAgentRecord, state: { sessionFile?: string; sessionId?: string }): void {
  let ready: ForkReadiness;
  try { ready = JSON.parse(readFileSync(launch.readinessPath, "utf8")); }
  catch { throw new Error("ws-pi-agent: fork did not publish readiness"); }
  if (ready.nonce !== launch.nonce || !ready.ownSessionKey?.trim() || ready.error ||
      !state.sessionFile || ready.sessionPath !== state.sessionFile || !ready.sessionId ||
      ready.ownSessionKey === record.forkContext?.parentSessionKey || record.forkContext?.parentSessionKeys?.includes(ready.ownSessionKey) || ready.sessionId === record.forkContext?.parentPiSessionId ||
      (state.sessionId && ready.sessionId !== state.sessionId)) {
    throw new Error(`ws-pi-agent: fork readiness rejected (${ready.error ?? "nonce/key/session mismatch"})`);
  }
  if (record.forkContext) {
    const mismatch = !Array.isArray(ready.registeredTools)
      ? "missing callable tool registrations"
      : compareForkRegistrations(record.forkContext.registeredTools, ready.registeredTools)
        ?? (JSON.stringify(ready.activeTools) !== JSON.stringify(record.forkContext.activeTools) ? "reordered callable tools" : undefined);
    if (mismatch) throw new Error(`ws-pi-agent: fork readiness rejected (${mismatch})`);
  }
  if (record.ownership && !containedOwnedPath(record.ownership.home, state.sessionFile)) throw new Error("ws-pi-agent: fork readiness rejected (session escaped owned home)");
  record.sessionPath = state.sessionFile;
  if (record.ownership) { record.ownership = { ...record.ownership, sessionPath: state.sessionFile }; const metadata = readOwnership(record.ownership.home); try { if (metadata) writeOwnership({ ...metadata, sessionPath: state.sessionFile, updatedAt: Date.now(), liveness: { ...metadata.liveness, lifecycle: "live", running: true, observedAt: Date.now() } }); } catch { /* durable facts remain conservative; readiness stays usable */ } }
  removeForkTransport(launch.contextPath);
  removeForkTransport(launch.readinessPath);
  rmSync(dirname(launch.contextPath), { recursive: true, force: true });
}

function containedOwnedPath(home: string, candidate: string): boolean {
  return isOwnedSessionPath(home, candidate);
}

async function captureForkSelection(client: RpcClient, record: RpcAgentRecord): Promise<void> {
  const state = await client.getState();
  if (state.model?.provider && state.model.id) record.modelBase = `${state.model.provider}/${state.model.id}`;
  if (typeof state.thinkingLevel === "string") record.modelEffort = state.thinkingLevel;
}

export function buildRpcClientOptions(
  cwd: string,
  model: string | undefined,
  sessionPath: string,
  systemPromptPath: string | undefined,
  tools: string,
  forkFrom?: string,
  parentSessionKey?: string,
  spawnRoleOverride?: SpawnRole,
  exploreMode?: ExploreMode,
  forkLaunch?: { contextPath: string; readinessPath: string; nonce: string; affinityId?: string },
  extensionPath: string,
  delegation?: DelegationPolicy,
  subtreeChannel?: SubtreeChannel,
): RpcClientOptions {
  if (!extensionPath) throw new Error("ws-pi-agent: missing loaded extension entry path for RPC child");
  const role = spawnRoleOverride ?? (forkFrom ? "fork" : "worker");
  forkLaunch = role === "fork" ? forkLaunch : undefined;
  const env: Record<string, string> = {
    [WS_PI_SPAWN_ROLE_ENV]: role,
    [WS_PI_APPROVAL_DIR_ENV]: join(dirname(sessionPath), "approvals"),
  };
  // RpcClient merges this object over process.env. An explicit empty marker
  // therefore clears an inherited deep mode for every non-research launch.
  env[WS_PI_EXPLORE_MODE_ENV] = role === "explore" && exploreMode ? exploreMode : "";
  // Explicitly clear every fork-only marker for worker/explore descendants.
  env[WS_PI_FORK_CONTEXT_ENV] = forkLaunch?.contextPath ?? "";
  env[WS_PI_FORK_READY_PATH_ENV] = forkLaunch?.readinessPath ?? "";
  env[WS_PI_FORK_READY_NONCE_ENV] = forkLaunch?.nonce ?? "";
  env[WS_PI_FORK_AFFINITY_ENV] = forkLaunch?.affinityId ?? "";
  env[WS_PI_PARENT_SESSION_KEY_ENV] = role === "fork" ? parentSessionKey ?? "" : "";
  env[DELEGATION_ENV] = delegation ? JSON.stringify(delegation) : "";
  env[SUBTREE_ENV] = subtreeChannel ? JSON.stringify(subtreeChannel) : "";
  env[WEB_HOME_ENV] = role === "explore" ? dirname(sessionPath) : "";
  env[WEB_NONCE_ENV] = role === "explore" ? subtreeChannel?.nonce ?? "" : "";
  // RpcClient overlays env onto process.env, so deletion here would preserve a
  // stale parent value. Empty values neutralize forced bootstrap selection.
  for (const override of CHILD_BOOTSTRAP_OVERRIDE_ENVS) env[override] = "";
  const args = forkFrom ? ["--fork", forkFrom] : ["--session", sessionPath];
  args.push("--session-dir", dirname(sessionPath));
  if (role !== "fork" && systemPromptPath) args.push("--append-system-prompt", systemPromptPath);
  // Do not let Pi discover an installed/cache copy alongside the exact parent
  // entry we explicitly load; `--tools` remains the sole active-surface gate.
  args.push("--no-extensions", "--extension", extensionPath, "--tools", tools);
  return {
    cliPath: RPC_CLI_PATH,
    cwd,
    env,
    model,
    args,
  };
}

/**
 * Appends one observed `ws-report-to-lead` call to `record.reportLog`,
 * drop-oldest past `REPORT_LOG_CAP`. `kind` is omitted from the entry
 * entirely (not stored as an explicit `undefined` property) when the caller
 * omits it, so a plain progress update round-trips as `{at}`.
 */
export function recordReport(record: RpcAgentRecord, kind: "question" | undefined, at: number = Date.now()): void {
  record.reportLog.push(kind === undefined ? { at } : { kind, at });
  if (record.reportLog.length > REPORT_LOG_CAP) {
    record.reportLog.shift();
  }
  if (record.ownership) touchOwnership(record.ownership.home);
}

/** Writes durable protection without treating a poll as activity. A new owner bind must check success before committing local state. */
export function syncOwnershipProtection(record: RpcAgentRecord): boolean {
  if (!record.ownership) return true;
  return updateOwnership(record.ownership.home, {
    liveness: {
      lifecycle: record.client ? (record.running ? "live" : "stopping") : "unknown",
      running: record.running,
      observedAt: Date.now(),
      threadBound: record.threadBound,
      ownerHeld: isOwnerHeld(record),
      waitingOnChildren: record.waitingOnChildren,
      pendingDelivery: hasPendingTerminalDelivery(record),
      pendingQuestion: record.threadBound,
      pendingApprovalCommandId: record.pendingApproval?.cmdId,
      recovery: record.client ? "none" : "revived",
    },
  }) !== undefined;
}

/** Unreferenced persistent-record observer; unchanged polling never renews activity. */
export function startOwnedSessionObserver(record: RpcAgentRecord, intervalMs = 5_000): void {
  if (!record.ownership || record.ownershipObserverStop) return;
  const sample = () => observeSessionWrite(record.ownership!.home, record.sessionPath);
  sample();
  const timer = setInterval(sample, intervalMs);
  timer.unref();
  record.ownershipObserverStop = () => { clearInterval(timer); record.ownershipObserverStop = undefined; };
}

/**
 * What `attachEventListener` must DO about an event `applyRpcEvent` just
 * applied. 260905: `applyRpcEvent` stays pure (no `pi`, no `RpcClient` — the
 * convention its existing plain-fake-record tests depend on), so it describes
 * the push instead of performing it, and the IO glue one layer up
 * (`attachEventListener`, which already read this return value to fire
 * `onApprovalPending`) turns that description into a `pi.sendMessage`.
 */
export interface RpcEventOutcome {
  /** A push to emit verbatim, already resolved against the record's suppression hooks. */
  push?: { family: PushFamily; payload: Record<string, unknown>; deliverAs: PushDeliverAs };
  /** `true` on `agent_settled`: the caller decides whether a `ws-agent-settled` push follows (it needs an async `harvestLastMessage`). */
  settled?: boolean;
}

/**
 * Applies lifecycle and intermediate-report RPC events to a record. A
 * `kind:"question"` report uses the owner-thread hook when available; every
 * other report-tool call is an immediate informational push. No report-tool
 * value is terminal. `agent_settled` clears execution synchronously, while
 * `attachEventListener` performs asynchronous terminal transcript harvest and
 * queue admission. Gated execution events additionally capture approval data.
 */
export function applyRpcEvent(
  record: RpcAgentRecord,
  evt: { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown },
): RpcEventOutcome {
  if (evt.type === "tool_execution_end") {
    observeForkFinishEvent(record, evt);
    return {};
  }
  if (evt.type === "agent_start") {
    const alreadyRunning = record.running;
    record.streaming = true;
    if (record.delegation && !alreadyRunning) {
      // A direct child can wake itself for its own children's reports, without
      // a new outer promptAgent call. This is a new own-turn generation.
      record.running = true;
      record.workGeneration = (record.workGeneration ?? 0) + 1;
      clearTerminalFacts(record);
    }
    observeForkFinishEvent(record, evt);
  } else if (evt.type === "agent_settled") {
    record.streaming = false;
    // The run is over: the child stops counting toward the fan-in the instant
    // it settles, whatever the caller decides to push about it.
    record.running = false;
    record.pendingApproval = undefined;
    syncOwnershipProtection(record);
    observeForkFinishEvent(record, evt);
    return { settled: true };
  } else if (evt.type === "tool_execution_start" && evt.toolName === REPORT_TO_LEAD_TOOL_NAME) {
    const args = evt.args as { message?: unknown; kind?: unknown } | undefined;
    const message = args?.message;
    if (typeof message === "string") {
      const kind = args?.kind === "question" ? args.kind : undefined;
      recordReport(record, kind);

      if (kind === "question") {
        // A defined (string) return is the registration notice for a fork
        // thread the hook just bound: push it to the lead as an advisory
        // instead of the raw question. `undefined` is the headless case;
        // a throwing hook degrades to that same baseline rather than
        // dropping the report.
        let notice: string | undefined;
        if (record.onQuestionReport) {
          try {
            notice = record.onQuestionReport(record, message);
          } catch {
            notice = undefined;
          }
        }
        return notice !== undefined
          ? { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: notice }, deliverAs: "followUp" } }
          : { push: { family: "ws-agent-question", payload: { question: message }, deliverAs: "steer" } };
      }

      return { push: { family: "ws-agent-report", payload: { report: message }, deliverAs: "followUp" } };
    }
  } else if (evt.type === "tool_execution_start" && evt.toolName === GATED_EXEC_TOOL_NAME) {
    const args = evt.args as { command?: unknown; rationale?: unknown; cwd?: unknown } | undefined;
    const command = args?.command;
    if (typeof command === "string" && typeof evt.toolCallId === "string") {
      record.pendingApproval = {
        cmdId: evt.toolCallId,
        command,
        rationale: typeof args?.rationale === "string" ? args.rationale : undefined,
        cwd: typeof args?.cwd === "string" ? args.cwd : undefined,
      };
      syncOwnershipProtection(record);
      // The approval PUSH itself is `createApprovalRelay`'s job
      // (execute-gateway.ts owns the §7 payload and the working-context
      // scrape); this branch only records what is pending. Fired from
      // `attachEventListener` below via `onApprovalPending`, unconditionally
      // now that there is no wait return for it to duplicate.
      return {};
    }
  }
  return {};
}

/**
 * Wires the child event stream to immediate reports and per-generation
 * terminal settlement. Settlement clears execution before any async work,
 * waits for the published descendant subtree, harvests the ordinary assistant
 * answer, admits one retryable terminal delivery, and parks only after enqueue.
 * Duplicate events join the generation latch; replacement work invalidates a
 * late harvest. Owner-held output uses the owner notification route.
 */
export function attachEventListener(
  pi: ExtensionAPI | undefined,
  registry: RpcAgentRegistry | undefined,
  record: RpcAgentRecord,
  client: RpcClient,
  onApprovalPending?: (record: RpcAgentRecord) => void,
): void {
  let refreshing = false;
  let dirty = false;
  let statsDirty = false;
  // Older/revived records may predate generation tracking. Normalize once so
  // the first settlement is not mistaken for a duplicate undefined latch.
  record.launchGeneration ??= 0;
  record.workGeneration ??= 0;
  const generation = record.launchGeneration;
  const refresh = (includeStats = false) => {
    dirty = true;
    statsDirty ||= includeStats;
    if (refreshing) return;
    refreshing = true;
    void (async () => {
      do {
        dirty = false;
        const readStats = statsDirty;
        statsDirty = false;
        try {
          const snapshot = await getAgentRpcSnapshot(client, readStats);
          if (record.client === client && record.launchGeneration === generation && refreshAgentTelemetry(record, snapshot.state, { stats: snapshot.stats, refreshContext: readStats })) {
            publishSubtree(registry);
            triggerAgentWidgetRefresh();
            triggerAgentCostRefresh();
          }
        } catch {
          if (record.client === client && record.launchGeneration === generation) {
            const changed = record.observedModel !== undefined || record.observedEffort !== undefined || record.telemetry?.model !== undefined || record.telemetry?.effort !== undefined;
            delete record.observedModel; delete record.observedEffort;
            if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; }
            if (changed) triggerAgentWidgetRefresh();
          }
        }
      } while (dirty && record.client === client && record.launchGeneration === generation);
      refreshing = false;
    })();
  };
  record.unsubscribe = client.onEvent((evt) => {
    const e = evt as { type?: string; toolName?: string; args?: unknown; toolCallId?: string; isError?: unknown; result?: unknown };
    if (record.client !== client || record.launchGeneration !== generation) return;
    if (record.subtreeChannel) {
      const snapshot = readSubtreeSnapshot(record.subtreeChannel);
      record.waitingOnChildren = subtreeWaiting(snapshot);
      record.subtreeRevision = snapshot?.revision;
    }
    const outcome = applyRpcEvent(record, e);
    publishSubtree(registry);
    if (e.type === "agent_start" || e.type === "agent_settled" || e.type === "message_end" || e.type === "message_update" || e.type === "thinking_level_changed" || e.type === "compaction_end") {
      // Context occupancy changes at completed message/compaction boundaries.
      // Streaming deltas retain the disk/state refresh without
      // hammering get_session_stats on every token.
      refresh(e.type === "agent_settled" || e.type === "message_end" || e.type === "compaction_end");
    }
    if (outcome.push) {
      pushToLead(pi, registry, record, outcome.push.family, outcome.push.payload, outcome.push.deliverAs);
    }
    if (e.type === "agent_start") {
      // 260905 (live-agent widget ticket): streaming just flipped true — a
      // fresh "running" row transition.
      triggerAgentWidgetRefresh();
    }
    if (outcome.settled && record.waitingOnChildren) {
      clearTerminalFacts(record);
      syncOwnershipProtection(record);
      publishSubtree(registry);
      triggerAgentWidgetRefresh();
      return;
    }
    if (outcome.settled) {
      const workGeneration = record.workGeneration;
      const stillSettled = () => record.client === client && record.launchGeneration === generation && record.workGeneration === workGeneration && !record.running && !record.streaming && !record.waitingOnChildren;
      const finish = record.forkFinish;
      if (finish) {
        void advanceForkFinish(record, registry ?? new Map([[record.agentId, record]]), pi as ExtensionAPI, finishResumeCtx(finish), finish);
      } else {
        if (record.settlementAdmissionGeneration === workGeneration) {
          record.terminalDelivery?.retry?.();
        } else {
          // Latch before the asynchronous transcript harvest so duplicate
          // settle events for one generation cannot enqueue twice.
          record.settlementAdmissionGeneration = workGeneration;
          const terminal = createTerminalDelivery(record, registry, pi, workGeneration);
          void (async () => {
            const lastMessage = await harvestLastMessage(record);
            if (!stillSettled() || record.terminalDelivery !== terminal) return;
            const payload = { reason: "idle", last_message: lastMessage };
            terminal.retry = () => {
              if (terminal.state !== undefined || !stillSettled() || record.terminalDelivery !== terminal) return;
              pushToLead(pi, registry, record, "ws-agent-settled", payload, "followUp", terminal);
            };
            terminal.retry();
            await probeAgentLiveness(pi, registry, record);
            triggerAgentWidgetRefresh();
          })();
        }
      }
    }
    if (record.forkFinish && e.type === "tool_execution_end") {
      const finish = record.forkFinish;
      void advanceForkFinish(record, registry ?? new Map([[record.agentId, record]]), pi as ExtensionAPI, finishResumeCtx(finish), finish);
    }
    if (e.type === "tool_execution_start" && (e.toolName === GATED_EXEC_TOOL_NAME || e.toolName === REPORT_TO_LEAD_TOOL_NAME)) {
      // 260905 (live-agent widget ticket): a gated command just went pending
      // approval, or a report (progress/question/final) was just observed —
      // both are widget-relevant transitions per the ticket's own list.
      triggerAgentWidgetRefresh();
    }
    // Ctx callback first, per-record fallback second (see
    // `RpcAgentRecord.onApprovalPending`).
    const approvalHook = onApprovalPending ?? record.onApprovalPending;
    if (approvalHook && e.type === "tool_execution_start" && e.toolName === GATED_EXEC_TOOL_NAME) {
      approvalHook(record);
    }
  });
}

/**
 * Best-effort `setThinkingLevel()` after `start()` for persistent children.
 * The same cached effort is reapplied after a dormant resume, independently
 * of whether a model was selected. Ordinary unsupported values degrade to a
 * no-op; researchers use strict verification before their prompt.
 */
async function applyModelEffort(client: RpcClient, modelEffort: string | undefined, strict = false): Promise<void> {
  if (!modelEffort) return;
  try {
    await client.setThinkingLevel(modelEffort as Parameters<RpcClient["setThinkingLevel"]>[0]);
  } catch (err) {
    if (strict) throw err;
    // Ordinary workers preserve their best-effort historical behavior.
  }
}

/**
 * Pi acknowledges setThinkingLevel even when it clamps to a model-supported
 * level. Researchers therefore read the actual RPC state before their first
 * prompt and on every resume. The initial launch adopts the actual default or
 * clamp, and every resume requires that frozen selection.
 */
async function verifyResearchSelection(client: RpcClient, record: RpcAgentRecord, initial: boolean): Promise<void> {
  if (!record.exploreMode || !record.modelBase) {
    throw new Error("ws-pi-agent: research record has no frozen model selection");
  }
  await applyModelEffort(client, record.modelEffort, true);
  const state = await client.getState();
  const model = state.model;
  const actualModel = model?.provider && model.id ? `${model.provider}/${model.id}` : undefined;
  const actualEffort = state.thinkingLevel;
  if (actualModel !== record.modelBase) {
    throw new Error(`ws-pi-agent: research model mismatch: expected ${record.modelBase}, got ${actualModel ?? "none"}`);
  }
  if (typeof actualEffort !== "string" || !actualEffort) {
    throw new Error("ws-pi-agent: research thinking level is unavailable");
  }
  if (initial) {
    // A selected tier may leave effort unset, and Pi may clamp a requested
    // effort. Persist what the child actually accepted for all later resumes.
    record.modelEffort = actualEffort;
    return;
  }
  if (record.modelEffort !== actualEffort) {
    throw new Error(`ws-pi-agent: research thinking mismatch: expected ${record.modelEffort ?? "default"}, got ${actualEffort}`);
  }
}

/**
 * 260905 (alias/park/cap ticket): the alias half of `spawnAgent`'s guard
 * clauses, run before any side effect (`mkdtempSync`/`randomUUID`). No-op
 * (`{ ok: true }`) when `alias` is unset. Otherwise scans for the current
 * holder of `alias`: a `running`/`threadBound` holder blocks the spawn
 * outright (rejection, not silent skip — the ticket's own wording); any other
 * holder (dormant/idle) is returned as `holder` for the caller to clear.
 *
 * Review relay #1 (Critical): this function does **not** mutate `holder`
 * itself — it only locates and validates. `runSpawnGuards` below commits
 * `holder.alias = undefined` only once every guard (this one and
 * `evictForCapacity`) has actually succeeded, so a rejected spawn (this
 * guard, `evictForCapacity`, or anything else `spawnAgent` throws before
 * registering) leaves the previous holder's alias completely untouched —
 * still resolvable by `ws-agent-send <alias>`. Exported for direct unit
 * coverage without a real `RpcClient`.
 */
export function reserveAgentAlias(
  registry: RpcAgentRegistry,
  alias: string | undefined,
): { ok: true; holder?: RpcAgentRecord } | { ok: false; error: string } {
  if (!alias) return { ok: true };
  for (const holder of registry.values()) {
    if (holder.alias !== alias) continue;
    if (holder.running || holder.streaming || holder.threadBound || isOwnerHeld(holder) || hasPendingTerminalDelivery(holder) || holder.waitingOnChildren) {
      const state = holder.running ? "running" : holder.threadBound ? "threadBound" : "held/outstanding";
      return {
        ok: false,
        error: `ws-pi-agent: ws-agent-spawn rejected: alias "${alias}" is held by agent ${holder.agentId}, which is ${state}`,
      };
    }
    return { ok: true, holder };
  }
  return { ok: true };
}

/**
 * 260908 (subagent audit window ticket): `max(lastLeadPromptAt, last
 * reportLog entry, or — for a revived orphan with no reportLog yet — its
 * lastReportAtOverride)`, pulled out of `evictForCapacity` below as its own
 * exported pure helper so the audit picker's dormant-tier sort (`260908`
 * sibling ticket) reuses the identical "last activity" formula rather than a
 * second, potentially-drifting copy. Pure refactor: `evictForCapacity`'s own
 * behavior is unchanged.
 */
export function lastActivityAt(record: RpcAgentRecord): number {
  const lastReportActivity = record.reportLog.at(-1)?.at ?? (record.lastReportAtOverride ? Date.parse(record.lastReportAtOverride) : 0);
  const lastOwnerActivity = record.ownerSends?.at(-1)?.at ?? 0;
  return Math.max(record.lastLeadPromptAt ?? 0, lastReportActivity, lastOwnerActivity);
}

/**
 * 260905 (alias/park/cap ticket): the registry-cap half of `spawnAgent`'s
 * guard clauses. While the registry is at or over `cap`, evicts the dormant
 * (`!record.client`), non-`running`, non-`threadBound` record with the
 * oldest last-activity stamp (`lastActivityAt`, above) until the new spawn
 * fits. Never evicts a live or protected record. An owned record is also
 * eligible only when durable metadata independently confirms it stopped;
 * missing or ambiguous metadata blocks both memory and disk eviction. Legacy
 * records retain registry-only eviction because their paths do not authorize
 * disk deletion. A deletion failure retains the registry record and rejects
 * the spawn so a later retention pass cannot fold the same cost a second time.
 */
export function evictForCapacity(
  registry: RpcAgentRegistry,
  cap: number,
  removeOwned: typeof removeOwnedAgentHome = removeOwnedAgentHome,
): { ok: true; evictedLabel?: string } | { ok: false; error: string } {
  const evictedLabels: string[] = [];
  while (registry.size >= cap) {
    let candidate: RpcAgentRecord | undefined;
    let candidateActivity = Number.POSITIVE_INFINITY;
    for (const record of registry.values()) {
      if (record.client || record.running || record.streaming || record.threadBound || isOwnerHeld(record) || record.pendingApproval || hasPendingTerminalDelivery(record) || record.waitingOnChildren) continue;
      if (record.ownership && inspectOwnedHomeRemoval(record.ownership).status !== "eligible") continue;
      const activity = lastActivityAt(record);
      if (activity < candidateActivity) {
        candidate = record;
        candidateActivity = activity;
      }
    }
    if (!candidate) {
      return {
        ok: false,
        error: `ws-pi-agent: ws-agent-spawn rejected: registry cap (${cap}) reached and every remaining record is live, protected, or durably unknown — nothing can be evicted to fit`,
      };
    }
    if (candidate.ownership) {
      const removal = removeOwned(candidate.ownership);
      if (removal.status !== "deleted") {
        return {
          ok: false,
          error: removal.status === "failed"
            ? "ws-pi-agent: ws-agent-spawn rejected: owned-home removal failed; the existing agent was retained for safety"
            : `ws-pi-agent: ws-agent-spawn rejected: registry cap (${cap}) candidate became protected or durably unknown before eviction`,
        };
      }
    }
    if (!persistEvictedAgentCost(registry, candidate)) {
      // The owned home may already be gone, but the in-memory record and its
      // cached telemetry remain retryable. Treat it as unowned on the retry.
      candidate.ownershipObserverStop?.();
      candidate.ownershipObserverStop = undefined;
      candidate.ownership = undefined;
      return { ok: false, error: `ws-pi-agent: ws-agent-spawn rejected: could not preserve evicted cost telemetry for ${candidate.agentId}` };
    }
    candidate.ownershipObserverStop?.();
    registry.delete(candidate.agentId);
    evictedLabels.push(candidate.alias ?? candidate.agentId);
  }
  return evictedLabels.length > 0 ? { ok: true, evictedLabel: evictedLabels.join(", ") } : { ok: true };
}

/**
 * 260905 (alias/park/cap ticket, review relay #1 CRITICAL fix): the single
 * gate `spawnAgent` calls before any side effect (`mkdtempSync`/
 * `randomUUID`/`registry.set`). Runs `reserveAgentAlias` then
 * `evictForCapacity` and returns the first failure unmutated — neither guard
 * commits anything on its own. Only once BOTH guards succeed does this
 * function clear the previous alias holder's `alias` (the actual transfer).
 * This ordering is load-bearing: the original implementation cleared the
 * holder's alias as a side effect of `reserveAgentAlias` itself, so a
 * subsequent `evictForCapacity` rejection (e.g. every other record is
 * running/threadBound/live) still left the spawn un-registered but had
 * already destroyed the previous holder's alias, making it unresolvable by
 * name even though its record was never touched otherwise. Exported for
 * direct unit coverage of the exact ordering guarantee without a real
 * `RpcClient`.
 */
export function runSpawnGuards(
  registry: RpcAgentRegistry,
  alias: string | undefined,
  cap: number,
): { ok: true; evictedLabel?: string } | { ok: false; error: string } {
  const aliasReservation = reserveAgentAlias(registry, alias);
  if (!aliasReservation.ok) return aliasReservation;
  const eviction = evictForCapacity(registry, cap);
  if (!eviction.ok) return eviction;
  if (aliasReservation.holder) {
    aliasReservation.holder.alias = undefined;
  }
  return eviction;
}

/** First free positive suffix, including parked/restored aliases. */
export function nextGeneratedAlias(registry: RpcAgentRegistry, prefix: string): string {
  for (let index = 1; ; index += 1) {
    const alias = `${prefix}-${index}`;
    if (![...registry.values()].some(record => record.alias === alias)) return alias;
  }
}

/**
 * Spawns a persistent `RpcClient` child from an already-rendered system
 * prompt file. Unlike the Phase 2-3 spawner, this performs **no**
 * `playbook.render` call itself (D-A): the caller (the lead) renders the
 * playbook and passes the resulting path directly as `systemPromptPath`.
 *
 * `modelBase` resolves a tier-shaped `model_name` through
 * `config.resolve_agent`, validates a concrete provider/id directly against
 * the live Pi catalog/auth state, or uses `ctx.inheritModel` when omitted.
 * Invalid concrete IDs and rejected tier answers fail before allocation. A
 * config-resolved effort is folded into `record.modelEffort`; explicit levels
 * win, while `"default"` retains source-specific defaults. `record.modelEffort`
 * is then the single value both the spawn-time and dormant-resume
 * `applyModelEffort` calls apply.
 *
 * Returns as soon as `client.prompt()` has sent the initial message — that
 * call only awaits transmission, not full-run completion (docs/rpc.md) — so
 * this satisfies the ticket's "returns immediately" contract while still
 * surfacing a synchronous spawn-time failure (bad `cliPath`, provider auth)
 * to the caller instead of swallowing it.
 *
 * 260904 Phase 1 (side-thread fork): when `ctx.forkFrom` is set, the
 * mkdtemp'd `sessionPath` computed below is only a PLACEHOLDER (it still
 * backs the approvals-dir derivation `buildRpcClientOptions` folds into the
 * child's env) — `pi --fork <forkFrom>` has Pi itself create/name the real
 * forked session file, discoverable only after `client.start()` via
 * `client.getState().sessionFile`. `record.sessionPath` is overwritten with
 * that real path immediately after `start()` resolves (before any event
 * listener/model-effort/prompt call), so every downstream consumer
 * (`getAgentTranscriptPath`, a dormant resume's own `--session
 * record.sessionPath`) sees the actual forked file, never the placeholder.
 * Throws — never silently degrades — when `getState()` returns no
 * `sessionFile`, mirroring the codebase's existing never-silently-degrade
 * convention (e.g. `bridge.ts`'s ferrule-mint failure handling).
 *
 * 260905 (alias/park/cap ticket): the very first thing this function does is
 * call `runSpawnGuards` (alias reservation + cap eviction, committed only
 * once both succeed) — see that function's doc comment for why the ordering
 * there is load-bearing. A guard rejection throws before any of the above
 * side effects (`mkdtempSync`, `randomUUID`, `registry.set`) run, so a
 * rejected spawn leaves no trace and no other record touched.
 */
export function callerDelegationPolicy(wsToolNames: readonly string[]): DelegationPolicy {
  const carried = readDelegationPolicy();
  if (carried) return carried;
  // Legacy spawned sessions predate the envelope. They never gain the root exception.
  return { version: 1, depth: readSpawnRole(process.env) ? 1 : 0, maxDepth: DEFAULT_MAX_AGENT_DEPTH,
    authority: "lead", tools: resolveTools("full-worker", wsToolNames).split(",") };
}

export function spawnAdmission(ctx: RpcSpawnCtx): DelegationPolicy {
  const parent = ctx.parentPolicy ?? callerDelegationPolicy(ctx.wsToolNames);
  const fork = ctx.spawnRole === "fork" || !!ctx.forkFrom;
  if (fork && parent.depth !== 0) throw new Error("ws-pi-agent: only the root lead may fork");
  const profile = ctx.provenance ?? ctx.profile;
  if (parent.depth > 0 && !profile && ctx.spawnRole !== "explore" && ctx.toolGroup !== "execute-worker") throw new Error("ws-pi-agent: nested spawn requires trusted render provenance");
  const group = resolveSpawnToolGroup(ctx.toolGroup);
  const tools = profile?.readOnly
    ? [...READ_TOOLS, REPORT_TO_LEAD_TOOL_NAME, ...CHILD_MANAGEMENT_TOOLS, ...readOnlyWsTools(ctx.wsToolNames)]
    : (ctx.explicitTools ?? resolveTools(group, ctx.wsToolNames)).split(",");
  if (ctx.spawnRole === "explore") tools.push(...NETWORK_TOOLS);
  const network = ctx.spawnRole === "explore" ? { search: true, fetch: true }
    : !profile?.readOnly && (group === "full-worker" || group === "execute-worker")
      ? parent.depth === 0 ? { search: true, fetch: true } : parent.network
      : undefined;
  const authority = profile?.authority ?? (ctx.spawnRole === "explore" || group === "execute-worker" ? "leaf" : "lead");
  const policy = childPolicy(parent, tools, authority, profile?.requiresChildren, ctx.provenance?.sessionKey, network);
  // A lateral fork's curated active names are not its execution ceiling: the
  // lead shell fallback and worker bash have equivalent native authority.
  if (fork) policy.tools = [...new Set([...policy.tools, GATED_EXEC_TOOL_NAME, ...resolveTools("full-worker", ctx.wsToolNames).split(",")])];
  return policy;
}

const WORKER_LIFECYCLE_GUIDE = `\n\n## Persistent delegation\nChild results return to this session, not directly to your caller. End your turn while children work; the adapter keeps the subtree outstanding and wakes you on their settled output. Continue the same child with ws-agent-send when its output is insufficient. After every descendant has settled and you have synthesized their results, end with the final-output shape required by your playbook in your ordinary assistant answer. Settlement delivers that answer; ws-report-to-lead is only for progress or a question before settlement.\n`;

export async function spawnAgent(
  registry: RpcAgentRegistry,
  ctx: RpcSpawnCtx,
  params: SpawnAgentParams,
): Promise<{ agent_id: string; alias?: string; evicted?: string }> {
  const finishDispatch = beginSubtreeDispatch(registry);
  try {
  const delegation = spawnAdmission(ctx);
  // Resolve exactly once before any guard, alias transfer, eviction, UUID, or
  // session allocation. Concrete ws-agent-spawn IDs validate locally and fail
  // closed. Named tiers retain their existing resolution/refusal behavior; an
  // omitted model_name (or tier transport/parse failure) still inherits.
  const resolution = await resolveSpawnModel(ctx.client, params.modelName, ctx.inheritModel, ctx.catalog, ctx.allowConcreteModel === true);
  if (resolution.concreteRejected) {
    const rejection = formatConcreteModelWarning(resolution.concreteRejected, ctx.catalog.length === 0);
    ctx.notifyTierWarning?.(rejection);
    throw new Error(`ws-pi-agent: ws-agent-spawn rejected: ${rejection}`);
  }
  if (ctx.requireTier && (resolution.source !== "tier" || !resolution.model || resolution.failure || resolution.rejected)) {
    const refusal = formatExploreTierRefusal(params.modelName ?? "small", resolution.failure, resolution.rejected);
    ctx.notifyTierWarning?.(refusal);
    throw new Error(`ws-pi-agent: ${refusal}`);
  }
  if (!ctx.requireTier && params.modelName && resolution.rejected) {
    // Same head convention as `reserveAgentAlias`/`evictForCapacity` below,
    // regardless of which tool (`ws-agent-spawn`/`ws-fork`/`ws-execute`)
    // actually called `spawnAgent` — no per-caller head special-casing.
    const rejection = formatTierWarning(params.modelName, resolution.rejected, ctx.inheritModel, ctx.catalog.length === 0);
    ctx.notifyTierWarning?.(rejection);
    throw new Error(`ws-pi-agent: ws-agent-spawn rejected: ${rejection}`);
  }

  // 260906 Phase 2: both refusal guards above have passed, so `resolution`
  // is a genuine launch — `resolution.rejected` is guaranteed absent here.
  // `resolvedEffort` is computed once and reused below (`record.modelEffort`)
  // rather than recomputed, so the pushed line and the stored record can
  // never drift apart.
  const resolvedEffort = effectiveModelEffort(params.modelEffort, resolution.effort, resolution.source, ctx.inheritEffort);
  ctx.onModelResolved?.({
    tier: resolution.source === "tier" ? params.modelName! : resolution.source,
    model: resolution.model,
    effort: resolvedEffort,
    inherited: resolution.source === "inherit",
  });

  if (ctx.spawnRole === "explore") await createWebSearch({ packageRoot: dirname(dirname(ctx.extensionPath)) }).probe();
  const promptBody = params.systemPromptPath ? readFileSync(params.systemPromptPath, "utf8") : undefined;
  const alias = params.alias ?? (ctx.aliasPrefix ? nextGeneratedAlias(registry, ctx.aliasPrefix) : undefined);
  const eviction = runSpawnGuards(registry, alias, resolveAgentRegistryCap());
  if (!eviction.ok) throw new Error(eviction.error);

  if (!params.systemPromptPath && !ctx.forkContext && !ctx.forkFrom) {
    throw new Error("ws-pi-agent: systemPromptPath is required for a non-fork spawn");
  }
  const agentId = randomUUID();
  const role = ctx.spawnRole ?? (ctx.forkFrom ? "fork" : resolveSpawnToolGroup(ctx.toolGroup) === "execute-worker" ? "execute-worker" : "worker");
  if (!ctx.storage) throw new Error("ws-pi-agent: missing Pi storage context for durable child allocation");
  const ownership = allocateAgentHome(ctx.storage, agentId, role, ctx.exploreMode);
  ownership.delegation = delegation;
  updateOwnership(ownership.home, { delegation });
  const sessionPath = ownership.sessionPath!;
  const forkLaunch = ctx.forkFrom || ctx.spawnRole === "fork" ? prepareForkLaunch(ctx.forkContext) : undefined;
  let forkSourcePath = ctx.forkFrom;
  if (forkLaunch && ctx.forkSourceEntries) {
    forkSourcePath = join(dirname(forkLaunch.contextPath), "source.jsonl");
    writeFileSync(forkSourcePath, ctx.forkSourceEntries.map(entry => JSON.stringify(entry)).join("\n") + "\n", { mode: 0o600 });
  }
  const modelBase = resolution.model;
  const toolGroup: ToolGroup = resolveSpawnToolGroup(ctx.toolGroup);
  const tools = ctx.spawnRole === "fork" || ctx.forkFrom ? ctx.explicitTools ?? delegation.tools.join(",") : delegation.tools.join(",");
  const subtreeChannel = { path: join(ownership.home, "subtree.json"), nonce: randomUUID() };
  let promptPath = params.systemPromptPath;
  if (promptPath && role !== "fork") {
    promptPath = join(ownership.home, "prompt.md");
    writeFileSync(promptPath, promptBody + WORKER_LIFECYCLE_GUIDE, { mode: 0o600 });
  }
  const record: RpcAgentRecord = {
    agentId,
    alias,
    title: params.title,
    sessionPath,
    ownership,
    systemPromptPath: promptPath,
    delegation,
    subtreeChannel,
    modelBase,
    // Explicit effort wins; "default" retains the selected source's policy.
    // This is the single fold point for spawn and dormant resume, and reuses
    // the same value already published through `onModelResolved`.
    modelEffort: resolvedEffort,
    modelTier: params.modelName,
    modelSource: resolution.source,
    wsToolNames: ctx.wsToolNames,
    toolGroup,
    explicitTools: ctx.explicitTools,
    spawnRole: role,
    exploreMode: ctx.exploreMode,
    forkContext: ctx.forkContext,
    streaming: false,
    running: false,
    reportLog: [],
    prompt: truncatePromptForStorage(params.prompt),
  };
  registry.set(agentId, record);
  startOwnedSessionObserver(record);
  if (role === "explore") clearWebReadiness(ownership.home);

  const client = new RpcClient(
    buildRpcClientOptions(
      ctx.cwd,
      modelBase,
      sessionPath,
      record.systemPromptPath,
      tools,
      forkSourcePath,
      ctx.forkContext?.parentSessionKey ?? ctx.parentSessionKey,
      ctx.spawnRole === "explore" ? "explore" : undefined,
      ctx.exploreMode,
      forkLaunch,
      ctx.extensionPath,
      delegation,
      subtreeChannel,
    ),
  );
  record.client = client;
  record.launchGeneration = (record.launchGeneration ?? 0) + 1;

  // 260905: the record is registered BEFORE `start()`, so a failure anywhere
  // in the launch sequence would otherwise leave a half-registered zombie the
  // lead's fan-in count keeps waiting on. Push `spawn-failed` and re-throw
  // unchanged — the thrown error still surfaces to the `ws-agent-spawn` caller
  // exactly as before; the push is additive, for the M/N bookkeeping.
  try {
    await client.start();

    if (ctx.forkFrom) {
      const state = await client.getState();
      const forkedSessionFile = state?.sessionFile;
      if (!forkedSessionFile) {
        throw new Error(
          "ws-pi-agent: fork spawn: RpcClient.getState() returned no sessionFile — cannot determine the forked session's actual path",
        );
      }
      if (forkLaunch) validateForkReadiness(forkLaunch, record, state);
    }

    // Read the already-folded record value, not params.modelEffort directly
    // — record.modelEffort is the single source of truth for what effort a
    // spawned/resumed child should receive (see effectiveModelEffort above
    // and the dormant-resume call site in sendToAgent, which reads the same
    // field).
    if (record.spawnRole === "explore") {
      await verifyResearchSelection(client, record, true);
      verifyWebReadiness(ownership.home, subtreeChannel.nonce);
    } else {
      await applyModelEffort(client, record.modelEffort);
    }
    // Capture the immutable pre-first-prompt boundary after all selection
    // work, before prompt() can append any attributable child turn.
    try { const snapshot = await getAgentRpcSnapshot(client); refreshAgentTelemetry(record, snapshot.state, { fresh: true, stats: snapshot.stats }); } catch { delete record.observedModel; delete record.observedEffort; if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; } }
    if (forkLaunch) await captureForkSelection(client, record);
    attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    attachFirstTaskForkCacheNotice(record, client, ctx.forkCacheNoticeOwner);
    await promptAgent(record, client, params.prompt);
    publishSubtree(registry, true);
  } catch (err) {
    clearLiveState(record);
    try { await client.stop(); } catch { /* best effort */ }
    pushSpawnFailed(ctx.pi, registry, record, err);
    throw err;
  } finally {
    if (forkLaunch) rmSync(dirname(forkLaunch.contextPath), { recursive: true, force: true });
  }

  // 260905 (live-agent widget ticket): a brand-new registry member, live and
  // running from its initial prompt — the widget's first sighting of it.
  triggerAgentWidgetRefresh();
  triggerAgentCostRefresh();
  return { agent_id: agentId, alias: record.alias, evicted: eviction.evictedLabel };
  } finally { finishDispatch(); }
}

/**
 * Delivers `message` to `agentId`, branching on locally-tracked streaming
 * state — this is a real behavior gap in the ticket's literal
 * `followUp()`/`steer()` tool mapping, traced through the installed
 * package's RPC mode and agent-loop source: `followUp`/`steer` only
 * *enqueue*; the queue is drained solely inside an *active* agent-loop run.
 * A freshly-started or freshly-resumed idle client has no active run, so
 * calling `followUp()`/`steer()` against it would silently queue a message
 * that is never delivered. So:
 *
 * - Dormant (`!record.client`): rebuild a fresh `RpcClient` against the
 *   SAME cached session/prompt/model (auto-resume, D-C), then deliver via
 *   `promptAgent()` regardless of `interrupt` — nothing is running yet to
 *   interrupt or queue behind. This is also where D-C's "auto-resumed child
 *   on the SAME ws session_key" lineage falls out for free: the reused
 *   `systemPromptPath` already has any session key spliced in by the lead's
 *   own prior `playbook.render` call, and passing it unchanged via
 *   `--append-system-prompt` on every relaunch never re-derives or
 *   duplicates it. This branch is also how an ORPHANED child from a previous
 *   lead session is revived (260905's shutdown sidecar re-registers it as a
 *   plain dormant record; `ws-agent-send` needs no special case for it).
 * - Live and idle (including the instant after this function's own
 *   auto-resume branch, or right after `spawnAgent`'s initial prompt
 *   settles): also `promptAgent()`, regardless of `interrupt`.
 * - Live and streaming: `interrupt ? steer() : followUp()`, per the
 *   ticket's literal flag semantics — this is the one case where an active
 *   run actually exists for the queue to drain into.
 *
 * 260905: every delivery goes through `promptAgent` (or re-marks `running`
 * for the steer/followUp branch), so the fan-in count reflects a send the
 * moment it is issued; and any rejection from the live client is treated as
 * the child having exited (`markAgentExited`) before being re-thrown.
 */
export async function sendToAgent(
  registry: RpcAgentRegistry,
  ctx: RpcResumeCtx,
  agentId: string,
  message: string,
  interrupt?: boolean,
): Promise<{ agent_id: string }> {
  const finishDispatch = beginSubtreeDispatch(registry);
  try {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid through the one
  // shared helper first; an unresolvable input falls back to the original
  // string so the existing "unknown agentId" error path is unchanged.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  const parentPolicy = readDelegationPolicy();
  if (parentPolicy) {
    if (!record.delegation) throw new Error("ws-pi-agent: legacy child lacks a resumable capability envelope");
    const admitted = childPolicy(parentPolicy, record.delegation.tools, record.delegation.authority, false, undefined, record.delegation.network);
    if (admitted.depth !== record.delegation.depth || parentPolicy.maxDepth < record.delegation.maxDepth) throw new Error("ws-pi-agent: recovered child exceeds the current delegation budget");
  }

  const writer = ctx.writer ?? "lead";

  // Claim activity before mutating a dormant record. If retention already
  // owns the cross-process deletion claim, this resume fails closed instead
  // of launching against a home that can disappear mid-start.
  const ownershipTouched = !record.ownership || touchOwnership(record.ownership.home);
  if (!record.client && !ownershipTouched) throw new Error("ws-pi-agent: owned session home is unavailable during resume");

  // A real new instruction supersedes an in-memory `/done` operation. Its
  // late settle/park callbacks must not affect this replacement work.
  if (record.forkFinish && ctx.finishToken !== record.forkFinish.token) {
    record.forkFinish = undefined;
    try { record.onForkFinishComplete?.(record, "superseded by new lead work"); } catch { /* best effort */ }
  }
  // See `RpcResumeCtx.leadSend`: the lead taking over the exchange releases a
  // thread bind the owner surface will never close (the headless
  // fork-raised-question path). A coordinator closeout is lead-attributed
  // text, but not an external takeover, so it never takes this branch.
  if (ctx.leadSend && !ctx.finishToken && record.threadBound) record.threadBound = false;

  if (!record.client) {
    if (record.spawnRole === "explore") {
      if (!record.delegation?.network?.search || !record.delegation.network.fetch || !record.subtreeChannel) throw new Error("web-search-tool-unavailable: legacy Explore lacks network authority; start a new researcher");
      await createWebSearch({ packageRoot: dirname(dirname(ctx.extensionPath)) }).probe();
      clearWebReadiness(dirname(record.sessionPath));
    }
    if (record.subtreeChannel) record.subtreeChannel = { ...record.subtreeChannel, nonce: randomUUID() };
    const forkLaunch = record.spawnRole === "fork" ? prepareForkLaunch(record.forkContext) : undefined;
    // 260904 Phase 1 (side-thread fork): `forkFrom` is deliberately never
    // passed here — a dormant resume (including a stopped fork) always
    // resumes via `--session record.sessionPath` (the fork's own
    // already-discovered real session file, see `spawnAgent`'s
    // `getState()` overwrite), exactly like a normal worker resume.
    // `record.explicitTools` (when set) is reused verbatim, same
    // cache-and-reuse contract as `systemPromptPath`/`modelBase`.
    const client = new RpcClient(
      buildRpcClientOptions(
        ctx.cwd,
        record.modelBase,
        record.sessionPath,
        record.systemPromptPath,
        record.explicitTools ?? record.delegation?.tools.join(",") ?? resolveTools(record.toolGroup, record.wsToolNames),
        undefined,
        record.forkContext?.parentSessionKey,
        record.spawnRole === "fork" ? "fork" : record.spawnRole === "explore" ? "explore" : "worker",
        record.exploreMode,
        forkLaunch,
        ctx.extensionPath,
        record.delegation,
        record.subtreeChannel,
      ),
    );
    record.client = client;
    record.launchGeneration = (record.launchGeneration ?? 0) + 1;
    const generation = record.launchGeneration;
    const finishOwner = ctx.finishToken !== undefined && record.forkFinish?.token === ctx.finishToken
      ? record.forkFinish : undefined;
    const ownsFailure = () => record.launchGeneration === generation
      && (ctx.finishToken === undefined || (record.forkFinish === finishOwner
        && finishOwner?.token === ctx.finishToken && finishOwner.generation === generation));
    // This launch belongs to the coordinator only when its exact in-memory
    // token requested the dormant resume. A later ordinary send clears that
    // coordinator instead, retaining the generation fence for replacement
    // work and stale callbacks.
    if (ctx.finishToken !== undefined && record.forkFinish?.token === ctx.finishToken) {
      record.forkFinish.generation = record.launchGeneration;
    }
    try {
      await client.start();
      if (forkLaunch) validateForkReadiness(forkLaunch, record, await client.getState());
      if (record.spawnRole === "explore") {
        await verifyResearchSelection(client, record, false);
        verifyWebReadiness(dirname(record.sessionPath), record.subtreeChannel?.nonce ?? "");
      } else {
        await applyModelEffort(client, record.modelEffort);
      }
      try { const snapshot = await getAgentRpcSnapshot(client); refreshAgentTelemetry(record, snapshot.state, { stats: snapshot.stats }); } catch { delete record.observedModel; delete record.observedEffort; if (record.telemetry) { delete record.telemetry.model; delete record.telemetry.effort; } }
      if (forkLaunch) await captureForkSelection(client, record);
      attachEventListener(ctx.pi, registry, record, client, ctx.onApprovalPending);
    } catch (err) {
      // Cleanup may await while a new instruction replaces this operation or
      // launch. Only its owner may clear the record; always stop our own client.
      if (ownsFailure() && record.client === client) clearLiveState(record);
      try { await client.stop(); } catch { /* best effort */ }
      // A finish-owned failure is rethrown to the coordinator's sole terminal
      // selector. Ordinary resumes retain spawn-failed; stale work gets neither.
      if (ownsFailure() && record.client === undefined && !finishOwner) {
        pushSpawnFailed(ctx.pi, registry, record, err);
      }
      throw err;
    } finally {
      if (forkLaunch) rmSync(dirname(forkLaunch.contextPath), { recursive: true, force: true });
    }
    // Role wiring that needs a live client (see `RpcAgentRecord.onResume`).
    // Best effort: a wiring failure must not
    // turn a routine resume into a failed send.
    try {
      record.onResume?.(record);
    } catch {
      // ignored — see above.
    }
    await promptAgent(record, client, message, { writer });
    publishSubtree(registry, true);
    return { agent_id: record.agentId };
  }

  const live = record.client;
  const dispatchSequence = issueDispatch(record);
  let writerStamp: ReturnType<typeof stampWriter> | undefined;
  try {
    if (record.streaming) {
      writerStamp = stampWriter(record, writer, writer === "owner" ? message : undefined, writer === "lead" ? Date.now() : undefined);
      if (interrupt) {
        await live.steer(message);
      } else {
        await live.followUp(message);
      }
      acceptWriter(record, writerStamp);
      writerStamp = undefined;
      // A steer/followUp joins the run already in flight, so the child is
      // outstanding again from the lead's point of view even though no fresh
      // prompt was issued. The accepted instruction starts a new generation.
      record.running = true;
      clearTerminalFacts(record);
      record.workGeneration = (record.workGeneration ?? 0) + 1;
    } else {
      await promptAgent(record, live, message, { writer });
    }
    acceptDispatch(record, dispatchSequence);
  } catch (err) {
    const superseded = wasSupersededByAcceptedDispatch(record, dispatchSequence);
    if (writerStamp) rollbackWriter(record, writerStamp);
    // A superseded closeout is no longer allowed to mutate or publish against
    // the replacement task. Its old RPC rejection is deliberately ignored by
    // lifecycle bookkeeping; the replacement owns the record now.
    if (ctx.finishToken !== undefined && record.forkFinish?.token !== ctx.finishToken) throw err;
    // 260905: an in-flight request rejection is the other deterministic
    // "the child is gone" signal (`RpcClient.send()` throws once the process
    // has exited) — treat it exactly like a failed liveness probe so the lead
    // is told rather than left counting a dead agent, then re-throw so the
    // caller still sees the failure.
    if (!superseded) markAgentExited(ctx.pi, registry, record, {
      // A coordinator-owned closeout chooses its own one advisory terminal;
      // do not let the generic exited path admit a competing terminal event.
      suppressTerminal: ctx.finishToken !== undefined,
    });
    throw err;
  }
  publishSubtree(registry, true);
  return { agent_id: record.agentId };
  } finally { finishDispatch(); }
}

/**
 * The agent's last assistant text, refreshed over RPC when the child is still
 * live and falling back to the cached `lastText` otherwise. This is the
 * former `ws-agent-wait` `reason:"idle"` payload, reused verbatim as the
 * `last_message` field of the `ws-agent-settled` push.
 */
async function harvestLastMessage(record: RpcAgentRecord): Promise<string | undefined> {
  if (!record.client) return record.lastText;
  try {
    const text = await record.client.getLastAssistantText();
    if (text !== null && text !== undefined) {
      record.lastText = text;
    }
  } catch {
    // best effort — fall back to whatever lastText was last cached.
  }
  return record.lastText;
}

/**
 * Maps every registered agent to a `{agent_id, status}` pair: `"dormant"`
 * when there is no live client (stopped, resumable — D-C), else
 * `"running"`/`"idle"` from the locally-tracked streaming flag. Pure — no
 * IO, no RPC round trip — so directly unit-testable against fake records.
 *
 * 260905 adds `last_report_at` (ISO, omitted when the agent has never
 * reported): with `ws-agent-wait` gone, a lead that missed or compacted a
 * pushed report needs some way to see how long an agent has been quiet.
 * `status` deliberately keeps deriving from `streaming` rather than the new
 * `running` flag — `streaming` is event-confirmed and is the right thing to
 * DISPLAY, while `running` is the narrower, earlier-set fan-in counter.
 *
 * 260905 (alias/park/cap ticket) adds `alias`/`title` on every row (both
 * omitted when unset), and an opt-in `opts.includePrompt` that additionally
 * includes the record's stored (already head-truncated at spawn) `prompt`.
 * Off by default — a full-registry dump with every prompt inlined would be
 * needlessly large for the common "what's out there" check.
 *
 * 260905 (list-model/last-report-fidelity ticket) adds `model`: the
 * effective resolved model the child was launched with (`modelBase`, plus
 * `/<effort>` when `modelEffort` is set), omitted only when the record
 * carries no `modelBase` at all — either a sidecar written before the field
 * existed, or a fresh spawn whose `resolveModelForAliasViaWsMcp` fallback
 * (`ctx.inheritModel`, via `inheritModelFromToolCtx`) itself came back
 * `undefined` because the tool-context model was absent or malformed.
 * Because that resolver otherwise falls back to the parent's own concrete
 * model on any catalog miss, an inheriting child's `modelBase` already IS
 * the parent's model name — no separate inherited-vs-explicit flag is
 * needed. `last_report_at` now falls back to `record.lastReportAtOverride`
 * (set only by `rehydrateOrphanRecord`) when `reportLog` is empty, so a
 * revived orphan does not read as never-reported.
 */
export function listAgents(
  registry: RpcAgentRegistry,
  opts?: { includePrompt?: boolean },
): Array<{ agent_id: string; status: AgentStatus; alias?: string; title?: string; model?: string; last_report_at?: string; owner_held?: true; prompt?: string }> {
  return [...registry.entries()].map(([agentId, record]) => {
    const lastReport = record.reportLog[record.reportLog.length - 1];
    const lastReportAt = lastReport ? new Date(lastReport.at).toISOString() : record.lastReportAtOverride;
    const model = record.modelBase ? (record.modelEffort ? `${record.modelBase}/${record.modelEffort}` : record.modelBase) : undefined;
    return {
      agent_id: agentId,
      status: record.running || record.streaming
        ? "running"
        : record.waitingOnChildren
          ? "waiting-on-children"
          : hasPendingTerminalDelivery(record)
            ? "pending-delivery"
            : record.client ? "idle" : "dormant",
      ...(record.alias ? { alias: record.alias } : {}),
      ...(record.title ? { title: record.title } : {}),
      ...(model ? { model } : {}),
      ...(lastReportAt ? { last_report_at: lastReportAt } : {}),
      ...(isOwnerHeld(record) ? { owner_held: true as const } : {}),
      ...(opts?.includePrompt && record.prompt ? { prompt: record.prompt } : {}),
    };
  });
}

/**
 * Best-effort graceful stop of `agentId`'s live RPC child (`abort()` then
 * `stop()`, both best-effort — a child that already exited or never
 * finished starting must not turn a routine stop into an unhandled
 * rejection). The registry entry is NEVER deleted here — per D-C, a stopped
 * agent stays registered as dormant/resumable; `ws-agent-send` auto-resumes
 * it later via the same cached session file. Throws only when `agentId`
 * itself is unknown.
 *
 * 260905: a stop is one of the four `ws-agent-settled` reasons, so a
 * non-silent stop pushes `reason:"stopped"` — the lead asked for it, but the
 * push is what removes the agent from its fan-in count in the same place
 * every other terminal transition does. `opts.silent` suppresses that for the
 * internal stops that are not a delegation outcome at all: `ask.ts` closing a
 * discussion thread (the owner's summary is the signal, not a stop notice)
 * and `stopAll()`'s shutdown sweep (the session is going away).
 */
export async function stopAgent(
  registry: RpcAgentRegistry,
  agentId: string,
  pi?: ExtensionAPI,
  opts?: { silent?: boolean; onStopped?: (success: boolean) => void; skipCostCheckpoint?: boolean },
): Promise<{ agent_id: string }> {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  const client = record.client;
  if (!opts?.silent) {
    record.waitingOnChildren = false;
    record.workGeneration = (record.workGeneration ?? 0) + 1;
    publishSubtree(registry);
  }
  if (record.ownership) touchOwnership(record.ownership.home);
  if (client) {
    const generation = record.launchGeneration;
    if (record.ownership) updateOwnership(record.ownership.home, { lastActivityAt: Date.now(), liveness: { lifecycle: "stopping", running: true, observedAt: Date.now() } });
    // Review relay #1 (Important, alias/park/cap): clear live state
    // SYNCHRONOUSLY, before either await below, not after both resolve. A
    // record that still reads `client`-live during `abort()`/`stop()` is a
    // race window: a concurrent `ws-agent-send` (or overlay `ForkChannel`
    // send) sees a live-idle record and calls `promptAgent` on a client
    // that's mid-teardown, losing the turn silently once `stop()` lands.
    // Clearing here first means that same racing send instead takes
    // `sendToAgent`'s dormant-resume branch, exactly as if this record had
    // already finished parking — the automatic park path (which now runs
    // after every settle, not just on an explicit stop) makes this window
    // hot enough to close rather than accept.
    clearLiveState(record);
    let stopped = true;
    try {
      await client.abort();
    } catch {
      stopped = false;
    }
    try {
      await client.stop();
    } catch {
      stopped = false;
    }
    // A new send can revive this record while the old client is stopping.
    // Never let the old stop clear the replacement's bind/final state.
    if (record.client !== undefined || record.launchGeneration !== generation) {
      return { agent_id: record.agentId };
    }
    // `message_end` can be persisted while abort/stop is in flight.  The
    // record is already synchronously dormant, so this final disk-only read
    // cannot revive a stale client or delay the stop race protection.
    refreshAgentTelemetry(record);
    if (record.ownership && record.launchGeneration === generation && !record.client) observeSessionWrite(record.ownership.home, record.sessionPath);
    // Review relay #1 (I2): a stop is a thread-close path too — the ticket
    // names "lead stop" alongside `/done`/fork final/`ws-withdraw-question`
    // (renamed by `260911` from `ws-resolve`). Releasing
    // the bind here keeps a stopped agent from carrying a latched flag into a
    // later `ws-agent-send` revival, where it would silently suppress every
    // settle push for the rest of the session.
    record.threadBound = false;
    if (record.ownership && record.launchGeneration === generation && !record.client) {
      updateOwnership(record.ownership.home, { liveness: {
        lifecycle: stopped ? "stopped" : "unknown", running: false, observedAt: Date.now(),
        threadBound: false, ownerHeld: isOwnerHeld(record), pendingQuestion: false,
        waitingOnChildren: record.waitingOnChildren, pendingDelivery: hasPendingTerminalDelivery(record),
        pendingApprovalCommandId: record.pendingApproval?.cmdId,
      } });
    }
    try { opts?.onStopped?.(stopped); } catch { /* internal observer only */ }
    if (!opts?.silent && !(record.delegation && readDelegationPolicy())) {
      // A delegated owner invoked this stop in its current turn, so the tool
      // result is the disposition. A self-generated follow-up would block
      // the owner's fresh subtree settlement.
      pushToLead(pi, registry, record, "ws-agent-settled", { reason: "stopped" }, "followUp");
    }
    // The final disk reconciliation above is the accounting boundary: refresh
    // the in-memory estimate first, then persist its one bounded checkpoint.
    triggerAgentWidgetRefresh();
    triggerAgentCostRefresh();
    if (!opts?.skipCostCheckpoint) persistAgentCostCheckpoint(registry);
  }
  publishSubtree(registry);
  return { agent_id: record.agentId };
}

/**
 * Lead-side introspection accessor (same family as `ws-agent-list`/
 * `ws-agent-stop`, not a driving/spawn tool): returns the absolute path to
 * `agentId`'s Pi session JSONL, unchanged since `spawnAgent` first computed
 * it. No RPC round-trip, no content marshalling — the lead greps the file
 * directly. Throws when `agentId` is unknown, same message convention as
 * `sendToAgent`/`stopAgent`.
 */
export function getAgentTranscriptPath(registry: RpcAgentRegistry, agentId: string): { transcript_path: string } {
  // 260905 (alias/park/cap ticket): resolve alias-or-uuid first — see
  // `sendToAgent`'s identical resolve-then-`.get()` shape.
  const resolvedId = resolveAgentId(registry, agentId) ?? agentId;
  const record = registry.get(resolvedId);
  if (!record) {
    throw new Error(`ws-pi-agent: unknown agentId "${agentId}"`);
  }
  if (record.ownership && !touchOwnership(record.ownership.home)) {
    throw new Error("ws-pi-agent: history unavailable — owned session home is busy, gone, or unreadable; retry the reference");
  }
  if (record.ownership) observeSessionWrite(record.ownership.home, record.sessionPath);
  return { transcript_path: record.sessionPath };
}

// ---------------------------------------------------------------------------
// Pi tool registration.
// ---------------------------------------------------------------------------

export interface ExploreParams {
  query: string;
  mode?: ExploreMode;
}

export interface AgentToolsHandle {
  /** Graceful teardown for `session_shutdown`: awaits a best-effort
   * `client.stop()` over every live RPC-backed child. */
  stopAll(): Promise<void>;
  /**
   * 260904 Phase 1: the same RPC-backed registry `ws-agent-*` already reads
   * and writes, exposed so `execute-gateway.ts`'s `ws-execute`/`ws-approve`
   * tools can spawn/inspect agents on ONE shared map — §4's "agent_id
   * disambiguates among all live and dormant/retained agents" requires this,
   * not a second parallel registry.
   */
  rpcRegistry: RpcAgentRegistry;
}

/**
 * Registers the six RPC-backed delegation tools (`ws-agent-spawn`,
 * `ws-agent-send`, `ws-agent-list`, `ws-agent-stop`, `ws-agent-transcript`,
 * `ws-report-to-lead`) plus the persistent `explore` preset. Every dispatcher
 * registers the same tool; the persisted delegation allowlist removes child
 * management, including Explore, at terminal depth.
 *
 * Phase 2 adds a child->lead report channel: `ws-report-to-lead` is the only
 * child-side tool this ticket adds (registered here but reachable only from
 * a worker's `full-worker` `--tools` allowlist, per `TOOL_GROUPS`); its
 * `execute()` is a no-op ack — the relay to the lead rides the existing
 * `RpcClient.onEvent()` wire via `applyRpcEvent`'s `tool_execution_start`
 * branch, not the tool's return value (see that function's doc comment).
 * `ws-agent-transcript` is a lead-side introspection tool (same family as
 * `ws-agent-list`/`ws-agent-stop`), never added to any `TOOL_GROUPS` entry, so
 * it is not reachable from a worker's own `--tools`.
 *
 * 260905: there is no seventh, harvesting tool — `ws-agent-wait` is deleted.
 * Everything a lead used to block for now arrives as a pushed custom message
 * (see this module's header and `pushToLead`), and the background liveness
 * probe started here is what turns a child that dies without settling into an
 * `exited` push instead of silence.
 *
 * Child management is available only within the persisted depth/capability
 * envelope. The tool-call gate enforces the same ceiling after lazy activation.
 */
export function registerAgentTools(
  pi: ExtensionAPI,
  bridge: BridgeHandle,
  sessionCtx: { cwd: string; storage?: AgentStorageContext; extensionPath: string },
  /**
   * 260904 Phase 1: see `RpcSpawnCtx.onApprovalPending`'s doc comment.
   * Threaded into both `ws-agent-spawn`'s `spawnAgent` call and
   * `ws-agent-send`'s `sendToAgent` call (dormant-resume branch) so a
   * `ws-execute`-spawned `execute-worker` keeps its approval relay wired even
   * if it is later driven through the generic `ws-agent-*` tools (shared
   * registry, §4). A no-op for every other `toolGroup` — `GATED_EXEC_TOOL_NAME`
   * is unreachable from their `--tools` lists, so this callback never fires.
   */
  onApprovalPending?: (record: RpcAgentRecord) => void,
  /** Adapter-owned persistent-research guide, wired by index.ts. */
  exploreGuidePath = "explore-guide.md",
  /** Shared ws-owned native/MCP presentation seam. */
  toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef(),
): AgentToolsHandle {
  const rpcRegistry: RpcAgentRegistry = new Map();
  registerAgentCostOwner(rpcRegistry, sessionCtx.storage);
  installSubtreePublisher(rpcRegistry, readSubtreeChannel(), () => heldPushQueue.length);
  const stopLivenessProbe = startLivenessProbe(pi, rpcRegistry);

  /** Cap on the head-truncated query used as a spawned explore's display title. */
  const EXPLORE_TITLE_CAP = 60;
  const EXPLORE_TITLE_TRUNCATION_MARKER = "…";

  /** Head-truncates `query` to `EXPLORE_TITLE_CAP` characters for use as the spawned record's `title`. */
  function deriveExploreTitle(query: string): string {
    return query.length > EXPLORE_TITLE_CAP ? `${query.slice(0, EXPLORE_TITLE_CAP)}${EXPLORE_TITLE_TRUNCATION_MARKER}` : query;
  }

  function composeExploreTask(query: string, mode: ExploreMode): string {
    return `Intent mode: ${mode}\n\nQuestion:\n${query}`;
  }

  registerWsTool(pi, {
    name: "ws-agent-spawn",
    label: "ws-agent-spawn",
    description:
      "Spawn a persistent RPC-backed pi subagent from an already-rendered system-prompt file (e.g. via ws/playbook.render). Returns {agent_id, alias?, evicted?} immediately after the initial prompt is sent. model_name accepts a configured tier alias or concrete Pi model ID; either is catalog/auth validated before allocation, while omission inherits the parent model. Do not wait for it: end your turn, and its reports, questions and completion arrive on their own as ws-agent-* messages carrying a running-count status line.",
    parameters: {
      type: "object",
      properties: {
        system_prompt_path: {
          type: "string",
          description: "Path to the lead-rendered playbook prompt file, appended as the child's system prompt via --append-system-prompt.",
        },
        prompt: { type: "string", description: "Initial task prompt sent to the spawned agent." },
        model_name: {
          type: "string",
          description:
            "Optional tier alias (small|medium|large|xlarge) resolved against harness pi agents.tier config, or a concrete Pi model ID (provider/id) validated with configured auth. Invalid named selections fail; omit to inherit the parent model.",
        },
        model_effort: {
          type: "string",
          description:
            "Optional Pi thinking level (off|minimal|low|medium|high|xhigh|max) or default. default applies no caller override: inherit keeps parent effort, tiers keep configured effort, and concrete models use their own default. Unsupported explicit values degrade to a no-op.",
        },
        alias: {
          type: "string",
          description:
            "Optional short name for this agent, usable in place of agent_id on ws-agent-send/stop/transcript/ws-approve. Reusing an alias held by a running, thread-bound, or owner-held agent rejects this spawn; a dormant/idle holder's alias is overwritten (its title is kept). Never derived automatically — omit to address this agent by uuid only.",
        },
        title: {
          type: "string",
          description: "Optional free-text label for this agent, independent of alias (display only, never used for resolution).",
        },
      },
      required: ["system_prompt_path", "prompt"],
    } as never,
    async execute(_toolCallId, params, _signal, onUpdate, toolCtx) {
      const p = params as {
        system_prompt_path: string;
        prompt: string;
        model_name?: string;
        model_effort?: string;
        alias?: string;
        title?: string;
      };
      let resolvedInfo: ResolvedModelInfo | undefined;
      const result = await spawnAgent(
        rpcRegistry,
        {
          pi,
          cwd: sessionCtx.cwd,
          storage: sessionCtx.storage ?? storageContextFromToolCtx(toolCtx),
          inheritModel: inheritModelFromToolCtx(toolCtx),
          inheritEffort: typeof (toolCtx as { thinkingLevel?: unknown }).thinkingLevel === "string"
            ? (toolCtx as { thinkingLevel: string }).thinkingLevel
            : undefined,
          catalog: modelCatalogFromToolCtx(toolCtx),
          allowConcreteModel: true,
          notifyTierWarning: tierWarningNotifierFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          extensionPath: sessionCtx.extensionPath,
          client: bridge.client,
          provenance: bridge.renderRegistry?.get(p.system_prompt_path),
          parentPolicy: { ...callerDelegationPolicy(bridge.wsToolNames), sessionKey: bridge.defaultSessionKeyRef.current },
          onApprovalPending,
          onModelResolved: (resolved) => {
            resolvedInfo = resolved;
            onUpdate?.({ content: [], details: { resolved } });
          },
        },
        {
          systemPromptPath: p.system_prompt_path,
          prompt: p.prompt,
          modelName: p.model_name,
          modelEffort: p.model_effort,
          alias: p.alias,
          title: p.title,
        },
      );
      return { content: [{ type: "text", text: JSON.stringify(result) }], details: { resolved: resolvedInfo } };
    },
    ...createDispatchToolPreview(toolPreviewTuiRef, "ws-agent-spawn", buildAgentSpawnSummary),
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "ws-agent-send",
    label: "ws-agent-send",
    description:
      "Send a message to a spawned agent. Delivers via prompt() when idle (including immediately after auto-resuming a dormant agent); while mid-stream, interrupt:true steers it and interrupt:false/omitted queues a follow-up. A dormant (ws-agent-stop'd) agent_id — including one revived from a ws-agent-orphaned message after a session restart — is auto-resumed from its cached session file first. Returns as soon as the message is delivered; the agent's answer arrives later as a pushed ws-agent-* message.",
    parameters: {
      type: "object",
      properties: {
        agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." },
        message: { type: "string", description: "Message text to deliver." },
        interrupt: {
          type: "boolean",
          description: "While the agent is mid-stream: true steers (interrupts) it, false/omitted queues a follow-up. Ignored while idle or dormant.",
        },
      },
      required: ["agent_id", "message"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string; message: string; interrupt?: boolean };
      // `leadSend: true`: this tool is the lead's own channel to a child (see
      // `RpcResumeCtx.leadSend`), unlike ask.ts's overlay channel.
      const result = await sendToAgent(
        rpcRegistry,
        { pi, cwd: sessionCtx.cwd, extensionPath: sessionCtx.extensionPath, onApprovalPending, leadSend: true },
        p.agent_id,
        p.message,
        p.interrupt,
      );
      // 260906 Phase 2: ws-agent-send never resolves a model itself — the
      // line reconstructs the TARGET agent's recorded model/effort instead.
      // A record revived from a sidecar snapshot (restart survivor) carries
      // no `modelSource`; that degrades to `inherited: true`/`tier:
      // "inherit"` rather than throwing or guessing a tier name.
      const record = rpcRegistry.get(result.agent_id);
      const inherited = record?.modelSource === undefined ? true : record.modelSource === "inherit";
      const resolved: ResolvedModelInfo = {
        tier: inherited ? "inherit" : record?.modelSource === "concrete" ? "concrete" : record?.modelTier ?? "?",
        model: record?.modelBase,
        effort: record?.modelEffort,
        inherited,
      };
      return { content: [{ type: "text", text: JSON.stringify(result) }], details: { resolved } };
    },
    ...createDispatchToolPreview(toolPreviewTuiRef, "ws-agent-send", buildAgentSendSummary),
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "ws-agent-list",
    label: "ws-agent-list",
    description:
      "List every tracked agent_id, its alias/title (when set), status (running, idle, dormant, waiting-on-children, or pending-delivery), model, last_report_at, and owner_held:true while the owner's last send retains settle ownership. Only running means autonomous execution; settled output is pushed without a wait tool.",
    parameters: {
      type: "object",
      properties: {
        include_prompt: {
          type: "boolean",
          description: "When true, also include each agent's stored (head-truncated) initial prompt. Off by default.",
        },
      },
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { include_prompt?: boolean };
      const result = listAgents(rpcRegistry, { includePrompt: p.include_prompt });
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "ws-agent-stop",
    label: "ws-agent-stop",
    description:
      "Gracefully stop a spawned agent's live RPC process. It stays registered as dormant/resumable — ws-agent-send auto-resumes it later via its cached session file.",
    parameters: {
      type: "object",
      properties: { agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." } },
      required: ["agent_id"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string };
      // Non-silent: an explicit stop is a delegation outcome the lead should
      // see land in its transcript like every other settle reason.
      const result = await stopAgent(rpcRegistry, p.agent_id, pi);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "ws-agent-transcript",
    label: "ws-agent-transcript",
    description:
      "Lead-side introspection: returns {transcript_path}, the absolute path to a spawned agent's Pi session JSONL. No content marshalling — grep/read the file directly.",
    parameters: {
      type: "object",
      properties: { agent_id: { type: "string", description: "agentId returned by ws-agent-spawn." } },
      required: ["agent_id"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { agent_id: string };
      const result = getAgentTranscriptPath(rpcRegistry, p.agent_id);
      return { content: [{ type: "text", text: JSON.stringify(result) }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: REPORT_TO_LEAD_TOOL_NAME,
    label: REPORT_TO_LEAD_TOOL_NAME,
    description:
      "Surface an async status update or intermediate finding to the lead immediately, distinct from your final answer. It is delivered to the lead the moment you call this — there is nothing to wait for on either side.",
    parameters: {
      type: "object",
      properties: {
        message: { type: "string", description: "Status update or intermediate finding to surface to the lead immediately." },
        kind: {
          type: "string",
          enum: ["question"],
          description:
            "Optional disambiguation for a question that needs the lead's input. Omit for a normal progress update; final output is the ordinary assistant answer delivered when the agent settles.",
        },
      },
      required: ["message"],
    } as never,
    async execute() {
      return { content: [{ type: "text", text: "reported" }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "explore",
    label: "explore",
    description:
      "Spawn a persistent exploration researcher. It returns exactly {agent_id, alias}; use ws-agent-send to continue the same child. Select mode by evidence intent, not importance: code-search traces bounded code/tests while diagnosis connects evidence into a cause; comparison evaluates alternatives while synthesis reconciles conflicting or architecture-level evidence.",
    parameters: {
      type: "object",
      properties: {
        query: { type: "string", description: "Exploration question." },
        mode: {
          type: "string",
          enum: Object.keys(EXPLORE_MODE_TIERS),
          description:
            "Intent mode; defaults to code-search. lookup locates one known fact; code-search traces bounded repository code/tests; history-search traces Git, tickets, or decisions; docs-search inspects local or installed documentation; web-search collects current external evidence; diagnosis connects code/tests/logs/runtime observations into a cause; comparison evaluates alternatives against evidence; synthesis reconciles conflicting evidence or produces a cross-source architecture conclusion.",
        },
      },
      required: ["query"],
      additionalProperties: false,
    } as never,
    async execute(_toolCallId, params, _signal, onUpdate, toolCtx) {
      const raw = params as Record<string, unknown>;
      if (Object.keys(raw).some((key) => key !== "query" && key !== "mode")) {
        throw new Error("ws-pi-agent: invalid explore arguments");
      }
      const p = params as ExploreParams;
      const mode = p.mode ?? "code-search";
      const tier = EXPLORE_MODE_TIERS[mode];
      if (!tier) throw new Error(`ws-pi-agent: unknown explore mode: ${String(p.mode)}`);
      let resolvedInfo: ResolvedModelInfo | undefined;
      const result = await spawnAgent(
        rpcRegistry,
        {
          pi,
          cwd: sessionCtx.cwd,
          storage: sessionCtx.storage ?? storageContextFromToolCtx(toolCtx),
          inheritModel: inheritModelFromToolCtx(toolCtx),
          catalog: modelCatalogFromToolCtx(toolCtx),
          notifyTierWarning: tierWarningNotifierFromToolCtx(toolCtx),
          wsToolNames: bridge.wsToolNames,
          extensionPath: sessionCtx.extensionPath,
          client: bridge.client,
          toolGroup: "read-only-explore",
          spawnRole: "explore",
          exploreMode: mode,
          parentPolicy: { ...callerDelegationPolicy(bridge.wsToolNames), sessionKey: bridge.defaultSessionKeyRef.current },
          requireTier: true,
          aliasPrefix: "explore",
          onApprovalPending,
          onModelResolved: (resolved) => {
            resolvedInfo = resolved;
            onUpdate?.({ content: [], details: { resolved } });
          },
        },
        {
          systemPromptPath: exploreGuidePath,
          prompt: composeExploreTask(p.query, mode),
          modelName: tier,
          title: deriveExploreTitle(p.query),
        },
      );
      return {
        content: [{ type: "text", text: JSON.stringify({ agent_id: result.agent_id, alias: result.alias }) }],
        details: { resolved: resolvedInfo },
      };
    },
    ...createDispatchToolPreview(toolPreviewTuiRef, "explore", buildExploreSummary),
  }, toolPreviewTuiRef);

  return {
    rpcRegistry,
    async stopAll(): Promise<void> {
      stopLivenessProbe();
      for (const record of rpcRegistry.values()) record.ownershipObserverStop?.();
      // Silent by construction: the session itself is going away, so a
      // per-agent "stopped" push would have nowhere to land. Routed through
      // stopAgent so shutdown leaves records in the same resting shape every
      // other stop does (the sidecar snapshot, index.ts, is taken BEFORE this
      // runs, while the records are still marked live).
      const rpcStops = [...rpcRegistry.keys()].map((agentId) => stopAgent(rpcRegistry, agentId, pi, { silent: true, skipCostCheckpoint: true }).catch(() => undefined));
      await Promise.allSettled(rpcStops);
    },
  };
}
