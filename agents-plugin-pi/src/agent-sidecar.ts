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
 * crash, never a growing backlog), re-registers each entry as a dormant
 * record, and emits a single `ws-agent-orphaned` push naming them. The lead
 * can then `ws-agent-send` any of them — `sendToAgent`'s existing
 * dormant-auto-resume branch relaunches from the same `--session` file.
 *
 * Deliberately narrow: only the fields `sendToAgent`'s resume branch actually
 * reads are persisted. Nothing runtime-only (no `client`, no `unsubscribe`,
 * no `reportLog`, no `running`) round-trips — a revived record is dormant by
 * definition, and its live state is rebuilt on the resume, not restored.
 *
 * Pure serialize/parse helpers are unit-tested directly
 * (test/agent-sidecar.test.ts); the two filesystem functions are thin and
 * best-effort by design (see `readAndClearSidecar`).
 */

import { existsSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import type { RpcAgentRecord, RpcAgentRegistry, SpawnAgentRole, ToolGroup } from "./spawner.ts";

/** Sidecar file version. Bumped only on a breaking shape change; a mismatch is treated as "no sidecar". */
export const SIDECAR_VERSION = 1;

/**
 * One persisted orphan. Field-for-field the subset of `RpcAgentRecord` that
 * `sendToAgent`'s dormant-resume branch reads, plus `spawnRole` so a revival
 * can tell a task fork from a worker without guessing at `toolGroup`.
 */
export interface PersistedOrphan {
  agentId: string;
  sessionPath: string;
  systemPromptPath: string;
  modelBase?: string;
  modelEffort?: string;
  wsToolNames: string[];
  toolGroup: ToolGroup;
  explicitTools?: string;
  spawnRole?: SpawnAgentRole;
}

export interface SidecarFile {
  version: number;
  writtenAt: string;
  orphans: PersistedOrphan[];
}

/** `<leadSessionFile>.ws-agents.json` — sibling of the session file, same convention as ask.ts's thread registry. */
export function sidecarPath(leadSessionFile: string): string {
  return `${leadSessionFile}.ws-agents.json`;
}

/**
 * Selects the records worth reviving: those with a LIVE client (a dormant
 * record was already stopped deliberately, and a thread-bound one belongs to
 * the owner surface, whose own `<sessionFile>.ws-threads.json` already
 * persists it — reviving it here would announce the same agent twice).
 */
export function captureOrphans(registry: RpcAgentRegistry): PersistedOrphan[] {
  const orphans: PersistedOrphan[] = [];
  for (const record of registry.values()) {
    if (!record.client || record.threadBound) continue;
    orphans.push({
      agentId: record.agentId,
      sessionPath: record.sessionPath,
      systemPromptPath: record.systemPromptPath,
      modelBase: record.modelBase,
      modelEffort: record.modelEffort,
      wsToolNames: [...record.wsToolNames],
      toolGroup: record.toolGroup,
      explicitTools: record.explicitTools,
      spawnRole: record.spawnRole,
    });
  }
  return orphans;
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
    if (typeof o.systemPromptPath !== "string") continue;
    out.push({
      agentId: o.agentId,
      sessionPath: o.sessionPath,
      systemPromptPath: o.systemPromptPath,
      modelBase: typeof o.modelBase === "string" ? o.modelBase : undefined,
      modelEffort: typeof o.modelEffort === "string" ? o.modelEffort : undefined,
      wsToolNames: Array.isArray(o.wsToolNames) ? o.wsToolNames.filter((n): n is string => typeof n === "string") : [],
      toolGroup: (o.toolGroup ?? "full-worker") as ToolGroup,
      explicitTools: typeof o.explicitTools === "string" ? o.explicitTools : undefined,
      spawnRole: o.spawnRole,
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
 */
export function rehydrateOrphanRecord(orphan: PersistedOrphan): RpcAgentRecord {
  return {
    agentId: orphan.agentId,
    client: undefined,
    sessionPath: orphan.sessionPath,
    systemPromptPath: orphan.systemPromptPath,
    modelBase: orphan.modelBase,
    modelEffort: orphan.modelEffort,
    wsToolNames: [...orphan.wsToolNames],
    toolGroup: orphan.toolGroup,
    explicitTools: orphan.explicitTools,
    spawnRole: orphan.spawnRole,
    streaming: false,
    running: false,
    reportLog: [],
  };
}

/**
 * The `ws-agent-orphaned` push body. One message for the whole set (not one
 * per agent): a lead restarting after a crash wants a single roll-call it can
 * act on, not N interleaved notices.
 */
export function buildOrphanSummary(orphans: PersistedOrphan[]): string {
  return orphans.map((o) => `${o.agentId} (${o.spawnRole ?? "worker"})`).join(", ");
}

/** Best-effort sidecar write; a failure here must never break session shutdown. */
export function writeSidecar(leadSessionFile: string, orphans: PersistedOrphan[]): void {
  try {
    if (orphans.length === 0) return;
    writeFileSync(sidecarPath(leadSessionFile), serializeOrphans(orphans), "utf8");
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
  const path = sidecarPath(leadSessionFile);
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
