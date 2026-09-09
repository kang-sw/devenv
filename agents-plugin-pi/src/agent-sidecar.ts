/**
 * Shutdown sidecar for the RPC-backed agent registry (260905, push model).
 *
 * The problem it exists for: `RpcAgentRegistry` is an in-memory `Map`. Under
 * the pull model a lead that died with children running simply lost them —
 * `ws-agent-wait` was gone along with the process, and nothing referenced the
 * orphans again. Under the push model that silence is worse: the lead is
 * supposed to be told about every child signal, so a whole set of children
 * vanishing without a word is exactly the failure the model promises not to
 * have.
 *
 * So on `session_shutdown` the still-live records are serialized to a sibling
 * file of the lead's own session file (`<sessionFile>.ws-agents.json` — the
 * same naming convention `ask.ts`'s `<sessionFile>.ws-threads.json` already
 * uses), and the next `session_start` reads it, DELETES it (one revival per
 * crash, never a growing backlog), and re-registers each entry as a dormant
 * record. The lead can then `ws-agent-send` any of them — `sendToAgent`'s
 * existing dormant-auto-resume branch relaunches from the same `--session`
 * file.
 *
 * A single `ws-agent-orphaned` push announces the set, but only when at least
 * one entry was `"running"` (see `buildOrphanPush`): re-registration is
 * bookkeeping, whereas a child cut off mid-turn is work the lead has to
 * re-issue.
 *
 * Deliberately narrow: the fields `sendToAgent`'s resume branch actually reads,
 * plus two purely DESCRIPTIVE ones for the roll-call — `state` (what the child
 * was doing at shutdown) and `lastReportAt` (how long it had been quiet). No
 * live state round-trips (no `client`, no `unsubscribe`, no `reportLog`): a
 * revived record is dormant by definition and rebuilds its own state on the
 * resume. `state` is never restored ONTO the record; it only tells the lead
 * that a `"running"` child was cut off mid-turn and needs its instruction
 * re-issued, since a resume replays from the last flushed turn.
 *
 * Pure serialize/parse helpers are unit-tested directly
 * (test/agent-sidecar.test.ts); the two filesystem functions are thin and
 * best-effort by design (see `readAndClearSidecar`).
 */

import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { startOwnedSessionObserver, type RpcAgentRecord, type RpcAgentRegistry, type SpawnAgentRole, type ToolGroup } from "./spawner.ts";
import { parseForkContext, type ForkContext } from "./fork-context.ts";
import type { ExploreMode } from "./process-role.ts";
import { readOwnership, updateOwnership, validDescriptor, type AgentOwnership } from "./agent-storage.ts";
import { parseTelemetry, type AgentTelemetry, type TelemetryOrigin } from "./agent-telemetry.ts";

/** Sidecar file version. Bumped only on a breaking shape change; a mismatch is treated as "no sidecar". */
export const SIDECAR_VERSION = 1;

/**
 * One persisted orphan. Field-for-field the subset of `RpcAgentRecord` that
 * `sendToAgent`'s dormant-resume branch reads, plus `spawnRole` so a revival
 * can tell a task fork from a worker without guessing at `toolGroup`.
 */
export interface PersistedOrphan {
  agentId: string;
  /** 260905 (alias/park/cap ticket): see `RpcAgentRecord.alias`/`.title`/`.prompt`. Absent for an old-shape sidecar entry. */
  alias?: string;
  title?: string;
  /** Head-truncated at spawn (`truncatePromptForStorage`); round-trips verbatim, no re-truncation on revival. */
  prompt?: string;
  sessionPath: string;
  systemPromptPath?: string;
  forkContext?: ForkContext;
  modelBase?: string;
  modelEffort?: string;
  telemetry?: AgentTelemetry;
  telemetryInputFloor?: TelemetryOrigin;
  observedModel?: string;
  observedEffort?: string;
  observedLatestInput?: number;
  wsToolNames: string[];
  toolGroup: ToolGroup;
  explicitTools?: string;
  spawnRole?: SpawnAgentRole;
  /** Persistent explore identity; only valid with the coherent explore tuple. */
  exploreMode?: ExploreMode;
  /**
   * What the child was doing when the session went away: `"running"` means a
   * prompt was outstanding (`RpcAgentRecord.running`), `"idle"` means it was
   * live but between turns. Load-bearing for the roll-call, not for the
   * resume: a `"running"` orphan was cut off mid-turn and comes back from its
   * last FLUSHED turn, so the lead must re-issue whatever it had asked for
   * rather than assume the work continued. Absent in a sidecar written before
   * this field existed — `parseOrphans` reads that as `"idle"`, the
   * conservative default (no caveat claimed about work that may not have been
   * outstanding).
   */
  state?: OrphanState;
  /** ISO time of the newest `reportLog` entry at shutdown; omitted when the child never reported. */
  lastReportAt?: string;
  /** Additive durable ownership; absence keeps a legacy record resumable. */
  ownership?: AgentOwnership;
}

/** See `PersistedOrphan.state`. */
export type OrphanState = "running" | "idle";

export interface SidecarFile {
  version: number;
  writtenAt: string;
  orphans: PersistedOrphan[];
}

/** `<leadSessionFile>.ws-agents.json` — sibling of the session file, same convention as ask.ts's thread registry. */
export function sidecarPath(leadSessionFile: string): string {
  return `${leadSessionFile}.ws-agents.json`;
}

/** Durable no-session locator. A fresh Pi identity never discovers another identity's registry. */
export function noSessionSidecarPath(agentDir: string, sessionId: string): string {
  return join(agentDir, "ws-agents", sessionId, "registry.ws-agents.json");
}

/**
 * Selects the records worth reviving: every non-thread-bound record,
 * live or dormant. A thread-bound one belongs to the owner surface, whose own
 * `<sessionFile>.ws-threads.json` already persists it — reviving it here
 * would announce the same agent twice.
 *
 * 260905 (alias/park/cap ticket): the `!record.client` half of the old skip
 * is gone — automatic parking now routinely turns a settled, non-threadBound
 * child dormant well before shutdown, so capturing only LIVE records would
 * silently lose every parked (alias/title/prompt included) agent on a
 * restart. Dormant records are already resumable; carrying them through the
 * sidecar too costs nothing and keeps the roll-call complete.
 *
 * Persistent simple/deep researchers are captured like every other
 * non-thread-bound record. Terminal collection leaves remain in their separate
 * self-reaping registry and are never sidecar records.
 */
export function captureOrphans(registry: RpcAgentRegistry): PersistedOrphan[] {
  const orphans: PersistedOrphan[] = [];
  for (const record of registry.values()) {
    if (record.threadBound) continue;
    orphans.push({
      agentId: record.agentId,
      alias: record.alias,
      title: record.title,
      prompt: record.prompt,
      sessionPath: record.sessionPath,
      systemPromptPath: record.systemPromptPath,
      ...(record.forkContext ? { forkContext: record.forkContext } : {}),
      modelBase: record.modelBase,
      modelEffort: record.modelEffort,
      ...(record.telemetry ? { telemetry: record.telemetry } : {}),
      ...(record.telemetryInputFloor ? { telemetryInputFloor: record.telemetryInputFloor } : {}),
      ...(record.observedModel ? { observedModel: record.observedModel } : {}),
      ...(record.observedEffort ? { observedEffort: record.observedEffort } : {}),
      ...(record.observedLatestInput !== undefined ? { observedLatestInput: record.observedLatestInput } : {}),
      wsToolNames: [...record.wsToolNames],
      toolGroup: record.toolGroup,
      explicitTools: record.explicitTools,
      spawnRole: record.spawnRole,
      ...(record.exploreMode ? { exploreMode: record.exploreMode } : {}),
      state: record.running ? "running" : "idle",
      // `undefined` (never reported) rather than an omitted key, matching every
      // other optional field above — `JSON.stringify` drops it on the way out
      // and `parseOrphans` reads it back the same way.
      lastReportAt: lastReportAt(record),
      ...(record.ownership ? { ownership: record.ownership } : {}),
    });
  }
  return orphans;
}

/**
 * ISO time of the newest `reportLog` entry, or `undefined` when the child
 * never reported.
 *
 * Review relay #1 (Important): falls back to `record.lastReportAtOverride`
 * when `reportLog` is empty — the same precedence `listAgents` and
 * `evictForCapacity` use — so a revived-but-never-reported-since orphan's
 * last-report time round-trips through a SECOND shutdown/revive cycle
 * instead of being dropped once `reportLog` is captured empty again. This
 * stays read-side only: the override rides on the existing `lastReportAt`
 * field rather than a second persisted key.
 */
function lastReportAt(record: RpcAgentRecord): string | undefined {
  const newest = record.reportLog[record.reportLog.length - 1];
  return newest ? new Date(newest.at).toISOString() : record.lastReportAtOverride;
}

/** Pure serializer — pretty-printed so a stranded sidecar is readable by hand during a post-mortem. */
export function serializeOrphans(orphans: PersistedOrphan[], writtenAt = new Date().toISOString()): string {
  const file: SidecarFile = { version: SIDECAR_VERSION, writtenAt, orphans };
  return `${JSON.stringify(file, null, 2)}\n`;
}

/**
 * Pure parser. Every failure mode — malformed JSON, wrong version, a
 * non-array `orphans`, an entry missing a load-bearing field — degrades to
 * "no orphans" rather than throwing: this runs inside `session_start`, where
 * a corrupt sidecar must never stop the extension from coming up.
 */
export function parseOrphans(raw: string): PersistedOrphan[] {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  const file = parsed as Partial<SidecarFile> | null;
  if (!file || typeof file !== "object" || file.version !== SIDECAR_VERSION || !Array.isArray(file.orphans)) return [];
  const out: PersistedOrphan[] = [];
  for (const entry of file.orphans) {
    const o = entry as Partial<PersistedOrphan> | null;
    if (!o || typeof o !== "object") continue;
    if (typeof o.agentId !== "string" || !o.agentId) continue;
    if (typeof o.sessionPath !== "string" || !o.sessionPath) continue;
    if (o.systemPromptPath !== undefined && typeof o.systemPromptPath !== "string") continue;
    let forkContext: ForkContext | undefined;
    try { forkContext = parseForkContext(o.forkContext); } catch { continue; }
    if (!o.systemPromptPath && !forkContext) continue;
    const ownership = o.ownership && validDescriptor(o.ownership) && o.ownership.agentId === o.agentId && o.ownership.sessionPath === o.sessionPath && (() => { const disk = readOwnership(o.ownership!.home); return !!disk && disk.home === o.ownership!.home && disk.ownerSessionId === o.ownership!.ownerSessionId && disk.agentId === o.ownership!.agentId && disk.sessionPath === o.ownership!.sessionPath && disk.role === o.ownership!.role && disk.exploreMode === o.ownership!.exploreMode; })() ? o.ownership : undefined;
    const toolGroup = o.toolGroup;
    const isKnownToolGroup = toolGroup === undefined || toolGroup === "read-only" || toolGroup === "read-only-explore" || toolGroup === "recon" || toolGroup === "full-worker" || toolGroup === "execute-worker";
    const isKnownRole = o.spawnRole === undefined || o.spawnRole === "worker" || o.spawnRole === "execute-worker" || o.spawnRole === "fork" || o.spawnRole === "explore";
    if (!isKnownToolGroup || !isKnownRole) continue;
    const hasExploreMode = Object.prototype.hasOwnProperty.call(o, "exploreMode");
    const exploreMode = o.exploreMode === "simple" || o.exploreMode === "deep" ? o.exploreMode : undefined;
    // Any research-shaped field makes the entire tuple strict. In particular,
    // do not discard an invalid mode and accidentally revive it as a worker.
    const hasResearchMetadata = o.spawnRole === "explore" || hasExploreMode || toolGroup === "read-only" || toolGroup === "read-only-explore";
    if (hasResearchMetadata && (
      o.spawnRole !== "explore" || !exploreMode || typeof o.modelBase !== "string" || !o.modelBase ||
      typeof o.modelEffort !== "string" || !o.modelEffort || Object.prototype.hasOwnProperty.call(o, "explicitTools") ||
      (exploreMode === "simple" ? toolGroup !== "read-only" : toolGroup !== "read-only-explore")
    )) continue;
    out.push({
      agentId: o.agentId,
      alias: typeof o.alias === "string" ? o.alias : undefined,
      title: typeof o.title === "string" ? o.title : undefined,
      prompt: typeof o.prompt === "string" ? o.prompt : undefined,
      sessionPath: o.sessionPath,
      systemPromptPath: o.systemPromptPath,
      ...(forkContext ? { forkContext } : {}),
      modelBase: typeof o.modelBase === "string" ? o.modelBase : undefined,
      modelEffort: typeof o.modelEffort === "string" ? o.modelEffort : undefined,
      ...(parseTelemetry(o.telemetry) ? { telemetry: parseTelemetry(o.telemetry) } : {}),
      ...(parseTelemetry({ version: 1, origin: o.telemetryInputFloor })?.origin ? { telemetryInputFloor: parseTelemetry({ version: 1, origin: o.telemetryInputFloor })!.origin } : {}),
      ...(typeof o.observedModel === "string" && o.observedModel ? { observedModel: o.observedModel } : {}),
      ...(typeof o.observedEffort === "string" && o.observedEffort ? { observedEffort: o.observedEffort } : {}),
      ...(typeof o.observedLatestInput === "number" && Number.isFinite(o.observedLatestInput) && o.observedLatestInput >= 0 ? { observedLatestInput: o.observedLatestInput } : {}),
      wsToolNames: Array.isArray(o.wsToolNames) ? o.wsToolNames.filter((n): n is string => typeof n === "string") : [],
      toolGroup: (o.toolGroup ?? "full-worker") as ToolGroup,
      explicitTools: typeof o.explicitTools === "string" ? o.explicitTools : undefined,
      spawnRole: o.spawnRole,
      ...(exploreMode ? { exploreMode } : {}),
      // An older sidecar (or a corrupt value) has no state to trust; "idle" is
      // the conservative read — it claims nothing about outstanding work.
      state: o.state === "running" ? "running" : "idle",
      // Review relay #1 (Minor a): a hand-edited/corrupt sidecar could carry a
      // non-date string; `typeof === "string"` alone would let it through to
      // feed `Date.parse` arithmetic in `evictForCapacity` (poisoning
      // `Math.max` with `NaN`, making the record permanently un-evictable)
      // and to `listAgents`'s `last_report_at`, which the tool description
      // and spec both declare ISO. `Number.isFinite(Date.parse(...))` rejects
      // anything that does not parse as a date.
      lastReportAt: typeof o.lastReportAt === "string" && Number.isFinite(Date.parse(o.lastReportAt)) ? o.lastReportAt : undefined,
      ...(ownership ? { ownership } : {}),
    });
  }
  return out;
}

/**
 * Rebuilds a DORMANT `RpcAgentRecord` from a persisted orphan — `client`
 * absent is the load-bearing part (it is what routes a later
 * `ws-agent-send` into `sendToAgent`'s relaunch branch), and `running:
 * false` keeps a revived orphan out of the fan-in status line until the lead
 * actually prompts it.
 *
 * 260905 (list-model/last-report-fidelity ticket): `lastReportAtOverride` is
 * a direct passthrough of `orphan.lastReportAt` (already ISO, already the
 * newest `reportLog` entry at shutdown) — the revived record's `reportLog`
 * itself stays empty (no synthetic entry), so `listAgents` and
 * `evictForCapacity` read the shutdown snapshot only until the record
 * reports again for real.
 */
export function rehydrateOrphanRecord(orphan: PersistedOrphan): RpcAgentRecord {
  return {
    agentId: orphan.agentId,
    alias: orphan.alias,
    title: orphan.title,
    prompt: orphan.prompt,
    client: undefined,
    sessionPath: orphan.sessionPath,
    ...(orphan.ownership ? { ownership: orphan.ownership } : {}),
    systemPromptPath: orphan.systemPromptPath,
    ...(orphan.forkContext ? { forkContext: orphan.forkContext } : {}),
    modelBase: orphan.modelBase,
    modelEffort: orphan.modelEffort,
    ...(orphan.telemetry ? { telemetry: orphan.telemetry } : {}),
    ...(orphan.telemetryInputFloor ? { telemetryInputFloor: orphan.telemetryInputFloor } : {}),
    ...(orphan.observedModel ? { observedModel: orphan.observedModel } : {}),
    ...(orphan.observedEffort ? { observedEffort: orphan.observedEffort } : {}),
    ...(orphan.observedLatestInput !== undefined ? { observedLatestInput: orphan.observedLatestInput } : {}),
    wsToolNames: [...orphan.wsToolNames],
    toolGroup: orphan.toolGroup,
    explicitTools: orphan.explicitTools,
    spawnRole: orphan.spawnRole,
    exploreMode: orphan.exploreMode,
    streaming: false,
    running: false,
    reportLog: [],
    lastReportAtOverride: orphan.lastReportAt,
  };
}

/**
 * Role-keyed wiring re-armed on a revived orphan. The callbacks themselves are
 * closures the revival's caller owns (`index.ts` composes `fork.ts`'s
 * `armForkRoleWiring` and `execute-gateway.ts`'s approval relay), so this
 * module stays free of both imports and directly testable.
 */
export interface OrphanRoleWiring {
  /** A `ws-fork`/discussion fork: question routing (§1) plus the §4 anti-bleed loop. */
  fork?: (record: RpcAgentRecord) => void;
  /** A `ws-execute` worker: the approval relay's `onApprovalPending`. */
  executeWorker?: (record: RpcAgentRecord) => void;
  /** A plain `ws-agent-spawn` worker: nothing role-specific to re-arm. */
  worker?: (record: RpcAgentRecord) => void;
}

/**
 * Puts each parsed orphan back on `registry` as a dormant record and re-arms
 * its role wiring.
 *
 * Review relay #1 (I1): the re-arm is the load-bearing half and was missing —
 * `spawnRole` was persisted and parsed but read only for the roll-call text,
 * so a revived FORK came back as a plain record with no `onQuestionReport` and
 * no anti-bleed loop. Its next `kind:"question"` would then be pushed straight
 * at the lead as `ws-agent-question` instead of routing to the owner surface,
 * a direct §1 violation.
 *
 * An id already present on the registry is left untouched (a live child always
 * wins over a stale sidecar entry) and is not returned.
 */
export function reviveOrphans(registry: RpcAgentRegistry, orphans: PersistedOrphan[], wiring: OrphanRoleWiring = {}): RpcAgentRecord[] {
  const revived: RpcAgentRecord[] = [];
  for (const orphan of orphans) {
    if (registry.has(orphan.agentId)) continue;
    const record = rehydrateOrphanRecord(orphan);
    startOwnedSessionObserver(record);
    if (record.ownership) updateOwnership(record.ownership.home, { liveness: { lifecycle: "unknown", running: false, observedAt: Date.now(), recovery: "sidecar", threadBound: record.threadBound, pendingApprovalCommandId: record.pendingApproval?.cmdId } });
    registry.set(orphan.agentId, record);
    const arm = orphan.spawnRole === "fork" ? wiring.fork : orphan.spawnRole === "execute-worker" ? wiring.executeWorker : orphan.spawnRole === "explore" ? undefined : wiring.worker;
    try {
      arm?.(record);
    } catch {
      // A wiring failure must not stop the remaining orphans from being
      // announced — the record is still registered and revivable, just without
      // its role hooks.
    }
    revived.push(record);
  }
  return revived;
}

/**
 * The caveat a `"running"` orphan carries. A child cut off mid-turn resumes
 * from its last FLUSHED turn, so whatever the lead had asked for is gone with
 * the process — re-issuing the instruction is the only way to get it done, and
 * a lead that assumes the work merely paused would wait forever for a report
 * nobody is writing.
 */
export const MID_TURN_ORPHAN_CAVEAT = "was mid-turn at shutdown; resumes from its last flushed turn — re-issue the instruction after ws-agent-send";

/**
 * Splits a revived set by what the lead has to DO about each entry. Every
 * entry is re-registered either way (an idle reviewer must stay reachable
 * through `ws-agent-send`); only the `"running"` ones carry lost work.
 */
export function partitionOrphansByState(orphans: PersistedOrphan[]): {
  running: PersistedOrphan[];
  idle: PersistedOrphan[];
} {
  const running: PersistedOrphan[] = [];
  const idle: PersistedOrphan[] = [];
  for (const orphan of orphans) {
    ((orphan.state ?? "idle") === "running" ? running : idle).push(orphan);
  }
  return { running, idle };
}

/**
 * The `ws-agent-orphaned` push body. One message for the whole set (not one
 * per agent): a lead restarting after a crash wants a single roll-call it can
 * act on, not N interleaved notices. One LINE per agent that was mid-turn —
 * each carries its role, its state at shutdown, its last-report time and the
 * re-issue caveat, which is more than fits a comma-joined run — plus, when
 * there were also idle entries, one closing line naming them together.
 *
 * Edition (live-run fix): the idle entries are a summary line rather than a
 * line each. They lost nothing and need no instruction re-issued; naming them
 * at length invited the lead to treat re-registration as a task.
 */
/**
 * 260905 (alias/park/cap ticket): the roll-call's per-agent label — `alias
 * (agentId)` when an alias is set (mirrors the pushed-message head convention
 * in `spawner.ts`'s `sendPush`), else the bare `agentId`; a `title`, when
 * set, is appended for extra context.
 */
function orphanLabel(o: PersistedOrphan): string {
  const idPart = o.alias ? `${o.alias} (${o.agentId})` : o.agentId;
  return o.title ? `${idPart} "${o.title}"` : idPart;
}

export function buildOrphanSummary(orphans: PersistedOrphan[]): string {
  const { running, idle } = partitionOrphansByState(orphans);
  const lines = running.map((o) => {
    const facts = [o.spawnRole ?? "worker", "running", ...(o.lastReportAt ? [`last report ${o.lastReportAt}`] : ["no reports"])];
    return `${orphanLabel(o)} (${facts.join(", ")}) — ${MID_TURN_ORPHAN_CAVEAT}`;
  });
  if (idle.length > 0) {
    lines.push(`${idle.length} idle agent${idle.length === 1 ? "" : "s"} re-registered dormant: ${idle.map((o) => orphanLabel(o)).join(", ")}`);
  }
  return lines.join("\n");
}

/**
 * The whole `ws-agent-orphaned` payload for a revived set, or `undefined` when
 * the set is worth no message at all.
 *
 * Edition (live-run fix): a `/reload` after three workers had all finished
 * announced all three, and the lead had nothing to do with any of them. A
 * roll-call is worth a message only when something was CUT OFF: an entry that
 * was mid-turn resumes from its last flushed turn and needs its instruction
 * re-issued, while an idle one is simply reachable again. So the push happens
 * only when at least one entry was `"running"`; the idle ones ride along in
 * the summary, and an all-idle set leaves no trace but `ws-agent-list`.
 *
 * Pure and exported (rather than inlined at the `session_start` call site) so
 * this decision has direct coverage — the glue around it is live-gate only.
 */
export function buildOrphanPush(orphans: PersistedOrphan[]): Record<string, unknown> | undefined {
  const { running, idle } = partitionOrphansByState(orphans);
  if (running.length === 0) return undefined;
  return {
    count: running.length,
    agents: buildOrphanSummary(orphans),
    ...(idle.length > 0 ? { idle_agent_ids: idle.map((o) => o.agentId) } : {}),
    detail:
      "A previous run of this session left these delegated agents behind. Every agent named here is registered as dormant: ws-agent-send revives one from its own session file, ws-agent-transcript reads what it did, ws-agent-stop drops it. The agents listed individually were mid-turn when the session went away — they resume from their last flushed turn, so re-issue that instruction when you revive one rather than waiting for a report nobody is writing.",
  };
}

/** Best-effort sidecar write; a failure here must never break session shutdown. */
export function writeSidecar(leadSessionFile: string, orphans: PersistedOrphan[]): void {
  writeSidecarAt(sidecarPath(leadSessionFile), orphans);
}

export function writeSidecarAt(path: string, orphans: PersistedOrphan[]): void {
  try {
    if (orphans.length === 0) return;
    mkdirSync(dirname(path), { recursive: true, mode: 0o700 });
    writeFileSync(path, serializeOrphans(orphans), { mode: 0o600 });
  } catch {
    // Nothing to fall back to — the orphans are simply not announced next run.
  }
}

/**
 * Reads and DELETES the sidecar in one step. The delete is unconditional (and
 * happens even when parsing yields nothing) so a single crash produces a
 * single revival: leaving the file behind would re-announce the same stale
 * agents on every subsequent start.
 */
export function readAndClearSidecar(leadSessionFile: string): PersistedOrphan[] {
  return readAndClearSidecarAt(sidecarPath(leadSessionFile));
}

export function readAndClearSidecarAt(path: string): PersistedOrphan[] {
  let raw: string | undefined;
  try {
    if (!existsSync(path)) return [];
    raw = readFileSync(path, "utf8");
  } catch {
    return [];
  } finally {
    try {
      rmSync(path, { force: true });
    } catch {
      // A sidecar that cannot be removed would re-announce next start; that
      // is noisy but harmless, and far better than failing session_start.
    }
  }
  return raw === undefined ? [] : parseOrphans(raw);
}
