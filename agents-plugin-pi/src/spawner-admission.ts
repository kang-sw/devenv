import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { BridgeHandle } from "./bridge.ts";
import { readSpawnRole } from "./process-role.ts";
import { inspectOwnedHomeRemoval, removeOwnedAgentHome } from "./agent-storage.ts";
import { CHILD_MANAGEMENT_TOOLS, DEFAULT_MAX_AGENT_DEPTH, READ_TOOLS, NETWORK_TOOLS, childPolicy, readDelegationPolicy, readOnlyWsTools, type DelegationPolicy } from "./delegation-policy.ts";
import { normalizeWriteScopes, type EffectiveWriteCapability, type WriteScope } from "./write-scopes.ts";
import { capacityEvictionCost, persistEvictedAgentCost } from "./agent-cost.ts";
import type { RpcAgentRecord, RpcAgentRegistry } from "./spawner-record.ts";
import { isOwnerHeld, hasPendingTerminalDelivery } from "./spawner.ts";
import type { WriteScopeDiagnostic, RpcSpawnCtx } from "./spawner.ts";
import { resolveTools, resolveSpawnToolGroup, REPORT_TO_LEAD_TOOL_NAME, GATED_EXEC_TOOL_NAME } from "./spawner-tools.ts";

/**
 * Best-effort `setThinkingLevel()` after `start()` for persistent children.
 * The same cached effort is reapplied after a dormant resume, independently
 * of whether a model was selected. Ordinary unsupported values degrade to a
 * no-op; researchers use strict verification before their prompt.
 */
export async function applyModelEffort(client: RpcClient, modelEffort: string | undefined, strict = false): Promise<void> {
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
export async function verifyResearchSelection(client: RpcClient, record: RpcAgentRecord, initial: boolean): Promise<void> {
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
 * Output-only activity high-water mark, shared by the live gutter, audit
 * picker, and dormant-capacity ordering. It deliberately excludes prompt,
 * owner-send, and report-log bookkeeping timestamps: those are inputs or
 * retained history, not proof that the agent is producing output.
 */
export function lastActivityAt(record: RpcAgentRecord): number {
  return record.lastOutputAt ?? 0;
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
 * disk deletion. An owned removal writes the candidate's eviction record
 * under its lock; a deletion failure leaves no record, retains the registry
 * record, and rejects the spawn.
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
      // The removal writes the candidate's eviction record under its lock.
      const evicted = candidate;
      const removal = removeOwned(candidate.ownership, undefined, undefined, { evictionCost: () => capacityEvictionCost(registry, evicted) });
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
      // Only an unowned record reaches here (an owned one is recorded by its
      // removal); it stays registered and retryable.
      return { ok: false, error: `ws-pi-agent: ws-agent-spawn rejected: could not preserve evicted cost telemetry for ${candidate.agentId}` };
    }
    candidate.ownershipObserverStop?.();
    delete candidate.telemetryReader;
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

/**
 * The parentPolicy a lead-side spawn passes: the caller's delegation policy
 * carrying the lead's session key, read at call time. The key becomes the
 * child's ferrule parent_session_key; a spawn site that omits it mints the
 * child parent-less, which can steal the lead's mailbox ownership.
 */
export function leadParentPolicy(bridge: Pick<BridgeHandle, "wsToolNames" | "defaultSessionKeyRef">): DelegationPolicy {
  return { ...callerDelegationPolicy(bridge.wsToolNames), sessionKey: bridge.defaultSessionKeyRef.current };
}

function requestedWriteCapability(tools: string[], writeScopes: readonly WriteScope[] | undefined): {
  tools: string[];
  write: EffectiveWriteCapability;
  diagnostic?: WriteScopeDiagnostic;
} {
  const naturallyUnrestricted = tools.includes("edit") && tools.includes("write");
  if (writeScopes === undefined) {
    return { tools, write: naturallyUnrestricted ? { mode: "unrestricted" } : { mode: "none" } };
  }
  const write = normalizeWriteScopes(writeScopes);
  if (naturallyUnrestricted) {
    return {
      tools,
      write: { mode: "unrestricted" },
      diagnostic: { status: "ignored", reason: "child already has unrestricted native edit/write authority" },
    };
  }
  return { tools: [...new Set([...tools, "edit", "write"])], write, diagnostic: { status: "bound", count: write.mode === "scoped" ? write.scopes.length : 0 } };
}

export function resolveSpawnAdmission(ctx: RpcSpawnCtx, writeScopes?: readonly WriteScope[]): { policy: DelegationPolicy; diagnostic?: WriteScopeDiagnostic } {
  const parent = ctx.parentPolicy ?? callerDelegationPolicy(ctx.wsToolNames);
  const fork = ctx.spawnRole === "fork" || !!ctx.forkFrom;
  if (fork && parent.depth !== 0) throw new Error("ws-pi-agent: only the root lead may fork");
  const profile = ctx.provenance ?? ctx.profile;
  if (parent.depth > 0 && !profile && ctx.spawnRole !== "explore" && ctx.toolGroup !== "execute-worker") throw new Error("ws-pi-agent: nested spawn requires trusted render provenance");
  const group = resolveSpawnToolGroup(ctx.toolGroup);
  if (profile?.reviewArtifact && (writeScopes?.length !== 1 || writeScopes[0]?.kind !== "file")) {
    throw new Error("ws-pi-agent: code reviewer requires exactly one file write scope");
  }
  const baseTools = profile?.readOnly
    ? [...READ_TOOLS, REPORT_TO_LEAD_TOOL_NAME, ...CHILD_MANAGEMENT_TOOLS, ...readOnlyWsTools(ctx.wsToolNames)]
    : (ctx.explicitTools ?? resolveTools(group, ctx.wsToolNames)).split(",");
  if (ctx.spawnRole === "explore") baseTools.push(...NETWORK_TOOLS);
  const binding = requestedWriteCapability(baseTools, writeScopes);
  const network = ctx.spawnRole === "explore" ? { search: true, fetch: true }
    : !profile?.readOnly && (group === "full-worker" || group === "execute-worker")
      ? parent.depth === 0 ? { search: true, fetch: true } : parent.network
      : undefined;
  const authority = profile?.authority ?? (ctx.spawnRole === "explore" || group === "execute-worker" ? "leaf" : "lead");
  const policy = childPolicy(parent, binding.tools, authority, profile?.requiresChildren, ctx.provenance?.sessionKey, network, binding.write);
  // A lateral fork's curated active names are not its execution ceiling: the
  // lead shell fallback and worker bash have equivalent native authority.
  if (fork) policy.tools = [...new Set([...policy.tools, GATED_EXEC_TOOL_NAME, ...resolveTools("full-worker", ctx.wsToolNames).split(",")])];
  return { policy, diagnostic: binding.diagnostic };
}

export function spawnAdmission(ctx: RpcSpawnCtx, writeScopes?: readonly WriteScope[]): DelegationPolicy {
  return resolveSpawnAdmission(ctx, writeScopes).policy;
}
