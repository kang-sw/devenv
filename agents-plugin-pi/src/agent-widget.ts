/**
 * 260905 (`260905-feat-ws-pi-live-agent-widget`): the live-agent widget — one
 * compact `belowEditor` panel listing every live agent and open owner
 * discussion thread, one row each. Phase 1 of that ticket also folds in the
 * standalone `260904` "N pending question(s)" `aboveEditor` widget (`ask.ts`'s
 * deleted `refreshPendingWidget`) — the pending-question surface is now a row
 * in THIS widget instead of its own panel. 260914 dropped the panel's `ws: N
 * agents` count heading (`buildHeadingLine`, added by `260909`): every row is
 * now prefixed by a state bullet (`AGENT_STATE_BULLET`) instead, and the
 * owner counts visible rows by eye.
 *
 * Source of truth is the two registries `index.ts` already owns — the RPC
 * agent registry (`spawner.ts`) and the owner-question thread registry
 * (`ask.ts`) — never a separate widget-owned model. `buildAgentRows` is pure
 * and reads the UNION of both at render time (not the RPC registry alone —
 * review relay #1's Critical fix: a pending `ws-queue-question` (ex-`ws-ask`) thread has no
 * respondent yet, and a fork-raised thread's respondent can come back from a
 * lead restart revived dormant with no `threadBound` re-set; both still owe
 * the owner a row); there is no cached/derived state to keep in sync.
 *
 * Golden rule / placement: this module imports FROM `spawner.ts` and
 * `ask.ts` only (types, plus `ask.ts`'s pure `countPending` helper), never
 * the reverse. `spawner.ts` triggers a re-render through its own
 * `agentWidgetRefreshRef` mutable ref (filled by `index.ts`) precisely so it
 * never has to import this module — see that ref's doc comment. `ask.ts`'s
 * remaining widget call sites go through the same ref, not through this
 * module directly, for the identical reason.
 *
 * `buildAgentRows`/`buildWidgetLines` are pure and unit tested directly
 * (`test/agent-widget.test.ts`) with duck-typed fake records and threads, no
 * live `pi` session. `createAgentWidgetController` is the IO
 * glue (`ctx.ui.setWidget`/`setStatus`, the 10-second elapsed timer) and is
 * left to `index.ts`'s own live-gate wiring, the same split `ask.ts` and
 * `spawner.ts` already use between their pure helpers and their `registerX`
 * IO functions.
 */

import type { ThreadRecord } from "./ask.ts";
import { isOwnerHeld, lastActivityAt, type RpcAgentRecord, type RpcAgentRegistry, type SpawnAgentRole } from "./spawner.ts";
import type { SubtreeDescendantRole } from "./subtree-lifecycle.ts";
import { classifyRegistryRowState, rowName, parseAgentDisplayDetail, type AgentDisplayDetail, type AgentRowState } from "./agent-display.ts";
export { classifyRegistryRowState, rowName, type AgentRowState } from "./agent-display.ts";
import { visibleWidth } from "./text-width.ts";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";
import { formatOutputRate } from "./output-rate.ts";
import { AGENT_BATCH_CAP, batchViews, type BatchView } from "./agent-batches.ts";
import { batchSummaryItem, batchSummaryKey, type BatchSummaryItem } from "./batch-summary.ts";

/** `ctx.ui.setWidget` key for the live-agent panel (`belowEditor`, not a footer/header replacement). */
export const AGENT_WIDGET_KEY = "ws-agents";

/** Retired `ctx.ui.setStatus` key for the former footer agent-count segment. Distinct from `goal-loop.ts`'s `GOAL_LOOP_YIELD_STATUS_KEY`; clear only this key when moving the count into the panel heading. */
export const AGENT_STATUS_KEY = "ws-agents-status";

/** Cap on rendered rows before a synthetic `+N more` tail — only `running` rows are ever trimmed; both awaiting states are always shown in full. */
export const AGENT_WIDGET_ROW_CAP = 5;

/** How often the widget repaints its elapsed clocks while it has at least one row. Mirrors `spawner.ts`'s `startLivenessProbe` arm/disarm-a-timer pattern. */
export const AGENT_WIDGET_TICK_MS = 10_000;

/** Owner-wait emphasis cadence. It is deliberately independent of the elapsed-clock timer. */
export const AGENT_WIDGET_ATTENTION_TICK_MS = 330;

/** `buildWidgetLines`'s width bound when the caller supplies none — Pi's extension surface exposes no live terminal-column read, so this is a conservative fixed default rather than a probed value. */
export const DEFAULT_AGENT_WIDGET_WIDTH = 80;

/** One live-agent row's display role. `"thread"` overrides the record's own `spawnRole` label only for a `threadBound` record whose bound thread is `origin: "lead-ask"`. `"explore"` is a persistent researcher role — see `roleFromSpawnRole`. */
export type AgentTreeRole = SubtreeDescendantRole;
export type AgentRowRole = AgentTreeRole | "thread";

/** One rendered row of the live-agent widget. Pure data — no `RpcAgentRecord`/`ThreadRecord` reference — so `buildWidgetLines` needs no registry access of its own. */
export interface AgentRow {
  /** `alias > title > shortened uuid` (mirrors `ask.ts:351`'s short-uuid convention). */
  name: string;
  role: AgentRowRole;
  state: AgentRowState;
  /** Milliseconds since the clock this row's state uses — `ThreadRecord.touchedAt` for a bound thread, otherwise the current run's `runStartedAt`, frozen at `settledAt`. Zero is an unused placeholder on `livenessOnly` rows. Never negative. */
  elapsedMs: number;
  /** Milliseconds since `spawner.ts`'s output-only `lastActivityAt` for an RPC-backed row; for a pure synthetic thread row (no backing record) this is the same `ThreadRecord.touchedAt` delta as `elapsedMs`, the best available activity signal. Zero is an unused placeholder on `livenessOnly` rows. Never negative. 260914: replaces the removed `ctx` label as the live panel's per-row activity cue. */
  lastActivityMs: number;
  /** The valid `/answer <id>` command for an owner-question row. */
  answerHint?: string;
  /** A supplied owner-held inspection affordance. Presentation preserves it but never invents one. */
  inspectionHint?: string;
  model?: string;
  effort?: string;
  contextTokens?: number;
  estimatedUsd?: number;
  /** Visible-output tokens per second for an RPC-backed row; absent until its first eligible message. */
  outputTps?: number;
  /** Propagated descendants alone carry depth; locally-owned and synthetic thread rows remain at depth zero without changing their established data shape. */
  depth?: number;
  /** Legacy or incomplete cross-process detail exposes liveness only, without invented clocks or telemetry. */
  livenessOnly?: true;
}

const BOLD = "\u001b[1m";
const RESET = "\u001b[22m";
const OWNER_ACTION_COLORS = ["error", "warning", "accent"] as const;
type OwnerActionColor = (typeof OWNER_ACTION_COLORS)[number];

function bold(text: string, enabled: boolean): string {
  return enabled ? `${BOLD}${text}${RESET}` : text;
}

/** The owner-action identity is the only non-telemetry span that changes semantic foreground. */
function semanticBold(text: string, color: OwnerActionColor, theme: AgentWidgetTheme | undefined): string {
  return bold(theme?.fg(color, text) ?? text, true);
}

/** Removes terminal/control input before it reaches a TUI row. */
function sanitizeDisplayTitle(title: string | undefined, fallback: string): string {
  const cleaned = title?.replace(/[\u0000-\u001f\u007f-\u009f]/g, "").trim();
  return cleaned || fallback;
}

export const AGENT_STATE_RANK: Readonly<Record<AgentRowState, number>> = {
  "awaiting-owner": 0,
  "idle-awaiting-owner": 0,
  "awaiting-approval": 1,
  "waiting-on-children": 2,
  "pending-delivery": 3,
  running: 4,
};

export const AGENT_STATE_LABEL: Readonly<Record<AgentRowState, string>> = {
  "awaiting-owner": "awaiting owner",
  "idle-awaiting-owner": "idle awaiting owner",
  "awaiting-approval": "awaiting approval",
  "waiting-on-children": "waiting on children",
  "pending-delivery": "pending delivery",
  running: "running",
};

/**
 * 260914: one glyph per `AgentRowState`, prefixed to every live-panel row so
 * state reads from the bullet at a glance (the ticket's replacement for the
 * dropped `ws: N agents` count heading). `awaiting-owner`/`idle-awaiting-owner`
 * share `●` — that glyph is styled by the caller with whatever
 * `ownerActionColor` the existing 330ms attention cycle (260908) computed for
 * the row, so the bullet participates in the flash instead of introducing a
 * second, competing animation.
 */
export const AGENT_STATE_BULLET: Readonly<Record<AgentRowState, string>> = {
  running: "·",
  "awaiting-approval": "▲",
  "awaiting-owner": "●",
  "idle-awaiting-owner": "●",
  "waiting-on-children": "◦",
  "pending-delivery": "▸",
};

/** Static (non-flashing) bullet color per state — every state except the owner-held pair, whose bullet instead follows the caller-supplied `ownerActionColor` so it stays in lockstep with the existing 330ms cue (260908). Approval intentionally gets a plain, non-bold color: it must never join the owner-attention animation (see `isAttentionState`). */
const STATE_BULLET_COLOR: Readonly<Partial<Record<AgentRowState, "warning" | "error" | "dim" | "accent">>> = {
  running: "warning",
  "awaiting-approval": "error",
  "waiting-on-children": "dim",
  "pending-delivery": "accent",
};

/** `worker -> "worker"`, `execute-worker -> "execute"`, `fork -> "fork"`, `explore -> "explore"` (260906); an unset `spawnRole` (should not happen post-spawn, but never throw) falls back to `"worker"`. */
function roleFromSpawnRole(spawnRole: SpawnAgentRole | undefined): AgentTreeRole {
  if (spawnRole === "execute-worker") return "execute";
  if (spawnRole === "fork") return "fork";
  if (spawnRole === "explore") return "explore";
  return "worker";
}

/** Shared local-plus-propagated tree contract used by the live gutter now and the audit picker in Phase 3. */
export interface AgentTreeNode {
  id: string;
  parentId: string | null;
  depth: number;
  role: AgentTreeRole;
  /** `undefined` is the dormant tier. Remote detailed states are advisory and gated by process liveness. */
  state: AgentRowState | undefined;
  live: boolean;
  /** Only records owned by this process have a locally reachable conversation stream. */
  openable: boolean;
  /** Sanitized owner projection, present only on detailed remote nodes. */
  display?: AgentDisplayDetail;
}

/**
 * Builds one parent-before-child tree from this process's registry and every
 * direct child's propagated identity snapshot. Local records remain the
 * authoritative source for their rich state; cross-process detail is advisory
 * and falls back to `running` versus dormant when absent or incomplete.
 */
export function buildAgentTree(records: RpcAgentRegistry): AgentTreeNode[] {
  const nodes = new Map<string, AgentTreeNode>();
  const children = new Map<string, AgentTreeNode[]>();
  const roots: AgentTreeNode[] = [];
  const localIds = new Set(records.keys());

  const add = (node: AgentTreeNode): boolean => {
    if (nodes.has(node.id)) return false;
    nodes.set(node.id, node);
    if (node.parentId === null) roots.push(node);
    else {
      const siblings = children.get(node.parentId) ?? [];
      siblings.push(node);
      children.set(node.parentId, siblings);
    }
    return true;
  };

  for (const record of records.values()) {
    const root: AgentTreeNode = {
      id: record.agentId,
      parentId: null,
      depth: 0,
      role: roleFromSpawnRole(record.spawnRole),
      state: classifyRegistryRowState(record),
      live: record.client !== undefined,
      openable: true,
    };
    add(root);

    // Phase 1 guarantees parent-before-child snapshots. Keep the check here so
    // a stale or malformed advisory edge cannot detach a row into this tree.
    const branchIds = new Set([record.agentId]);
    for (const descendant of record.subtreeDescendants ?? []) {
      const parentId = descendant.parentId ?? record.agentId;
      const parent = nodes.get(parentId);
      if (!parent || !branchIds.has(parentId) || localIds.has(descendant.id)) continue;
      const display = parseAgentDisplayDetail(descendant.display);
      const node: AgentTreeNode = {
        id: descendant.id,
        parentId,
        depth: parent.depth + 1,
        role: descendant.role,
        state: descendant.live ? (display ? display.state ?? undefined : "running") : undefined,
        live: descendant.live,
        openable: false,
        ...(display ? { display } : {}),
      };
      if (add(node)) branchIds.add(node.id);
    }
  }

  const ordered: AgentTreeNode[] = [];
  const visit = (node: AgentTreeNode): void => {
    ordered.push(node);
    for (const child of children.get(node.id) ?? []) visit(child);
  };
  for (const root of roots) visit(root);
  return ordered;
}

/** A thread the widget still owes the owner an answer on — the two `countPending`-adjacent statuses that ever yield a row. Dormant/closed threads never do. */
function isLiveThreadStatus(status: ThreadRecord["status"]): boolean {
  return status === "pending" || status === "open";
}

/** `Math.max(0, deltaMs)`, but a malformed/missing clock (`Date.parse` of a bad `touchedAt`, review relay #1 Minor) collapses to 0 instead of propagating `NaN` through `formatCompactDuration`. */
function clampElapsed(deltaMs: number): number {
  return Number.isFinite(deltaMs) ? Math.max(0, deltaMs) : 0;
}

/**
 * Pure row builder over the UNION of the two registries — the RPC agent
 * registry and the owner-question thread registry — never the RPC registry
 * alone (review relay #1 Critical: a pending `ws-queue-question` (ex-`ws-ask`) thread has no
 * `respondentAgentId` until `/answer` lazily spawns its fork, and a
 * fork-raised thread's respondent can come back from a lead restart revived
 * dormant with `threadBound` unset (`agent-sidecar.ts`'s `reviveOrphans`) —
 * both cases used to render zero rows and silently drop the merged-in
 * pending-question surface entirely).
 *
 * RPC-side rows render only for an explicit classified state: owner action,
 * approval, execution, descendant waiting, or pending terminal delivery. A
 * retained but settled client is not implicitly running. A `threadBound`
 * record renders even while dormant
 * (`client === undefined`): that row is the owner's action cue, and it must
 * not disappear just because the respondent fork happens to be parked
 * between messages.
 *
 * Row inclusion, thread-registry side: every `isLiveThreadStatus` thread
 * whose `respondentAgentId` does NOT resolve to an RPC-side row above (no
 * respondent yet, or a respondent that exists but is not `threadBound`) gets
 * its own synthetic row — name is the thread's own `title` (there is no
 * agent identity to name it by), role is always `"thread"`, state is always
 * `"awaiting-owner"`, elapsed is `now - touchedAt`, and the `/answer <id>`
 * hint is always set. This is the ticket's merged-in pending-question row.
 *
 * State precedence keeps owner and approval actions ahead of execution, then
 * descendant waiting and terminal delivery as distinct non-running states.
 *
 * Hint/clock vs. role (review relay #1 Important #2): a `threadBound`
 * record's `/answer <id>` hint and `touchedAt`-based elapsed follow the
 * `awaiting-owner` STATE and apply whenever a matching live thread is found,
 * regardless of `origin` — a fork-raised (Entry A) respondent owes the owner
 * an answer exactly as much as a lead-ask (Entry B) one does. Only the internal ROLE
 * stays origin-dependent: `"thread"` applies only for a `lead-ask`
 * match (the ticket's Entry-B-only role override); a fork-raised match keeps
 * the record's own `spawnRole` label (typically `"fork"`).
 *
 * Sort: root-level subtree blocks use state rank first and elapsed descending
 * within each state, preserving the former flat ordering while descendants
 * stay immediately below their parent. No cap here — the display cap to
 * `AGENT_WIDGET_ROW_CAP` with its `+N more` tail is `buildWidgetLines`'s own
 * rendering concern, not a property of this function's row count (260914
 * dropped the `ws: N agents` heading that used to read this count; the owner
 * now counts visible rows by eye).
 */
export function buildAgentRows(records: RpcAgentRegistry, threads: readonly ThreadRecord[], now: number): AgentRow[] {
  return buildAgentRowEntries(records, threads, now).map((entry) => entry.row);
}

/**
 * 261028: one `buildAgentRows` row with the identity the expanded mode needs
 * beside it, kept off `AgentRow` so the row's established data shape stays
 * unchanged. `batchSource` marks a row that owns a tool-batch allotment in the
 * expanded mode: every agent row (a locally-owned record's batches come from
 * its own buffer, a descendant's from the relayed `agent-view` model, Phase 2);
 * a synthetic thread row has none.
 */
export interface AgentRowEntry {
  row: AgentRow;
  /** Agent id of an RPC-backed or propagated row; absent on a synthetic thread row. */
  agentId?: string;
  batchSource: boolean;
}

/** `buildAgentRows` with each row's identity; same inclusion, order, and data. */
export function buildAgentRowEntries(records: RpcAgentRegistry, threads: readonly ThreadRecord[], now: number): AgentRowEntry[] {
  const tree = buildAgentTree(records);
  const coveredThreadIds = new Set<string>();
  const rowById = new Map<string, AgentRow>();

  for (const node of tree) {
    if (!node.openable) {
      // Cached detail cannot restore liveness or grant local command access.
      if (!node.live || node.state === undefined) continue;
      if (node.display) {
        const { name, runStartedAt, settledAt, lastOutputAt, model, effort, contextTokens, estimatedUsd, outputTps } = node.display;
        rowById.set(node.id, {
          name, role: node.role, state: node.state,
          elapsedMs: clampElapsed((settledAt ?? now) - runStartedAt),
          lastActivityMs: clampElapsed(now - lastOutputAt),
          depth: node.depth,
          ...(model !== undefined ? { model } : {}),
          ...(effort !== undefined ? { effort } : {}),
          ...(contextTokens !== undefined ? { contextTokens } : {}),
          ...(estimatedUsd !== undefined ? { estimatedUsd } : {}),
          ...(outputTps !== undefined ? { outputTps } : {}),
        });
        continue;
      }
      rowById.set(node.id, {
        name: node.id.slice(0, 8),
        role: node.role,
        state: "running",
        elapsedMs: 0,
        lastActivityMs: 0,
        depth: node.depth,
        livenessOnly: true,
      });
      continue;
    }

    const record = records.get(node.id);
    const state = node.state;
    if (!record || state === undefined) continue;
    const boundThread = record.threadBound
      ? threads.find((t) => t.respondentAgentId === record.agentId && isLiveThreadStatus(t.status))
      : undefined;
    if (boundThread) coveredThreadIds.add(boundThread.threadId);

    const isAwaitingOwnerWithThread = state === "awaiting-owner" && boundThread !== undefined;
    // Bound-thread rows retain their distinct owner-thread clock. Every other
    // row shows one run, ending at settlement while lingering for delivery.
    const runEndsAt = record.settledAt ?? now;
    const outputTps = record.outputRate?.rate();
    const elapsedMs = isAwaitingOwnerWithThread
      ? clampElapsed(now - Date.parse(boundThread!.touchedAt))
      : clampElapsed(runEndsAt - (record.runStartedAt ?? runEndsAt));
    rowById.set(node.id, {
      name: rowName(record),
      role: isAwaitingOwnerWithThread && boundThread!.origin === "lead-ask" ? "thread" : node.role,
      state,
      elapsedMs,
      lastActivityMs: clampElapsed(now - lastActivityAt(record)),
      ...(isAwaitingOwnerWithThread ? { answerHint: `/answer ${boundThread!.threadId}` } : {}),
      ...(isOwnerHeld(record) ? { inspectionHint: `/audit ${record.alias ?? record.agentId}` } : {}),
      ...(record.telemetry?.model ?? record.observedModel ? { model: record.telemetry?.model ?? record.observedModel } : {}),
      ...(record.telemetry?.effort ?? record.observedEffort ? { effort: record.telemetry?.effort ?? record.observedEffort } : {}),
      ...((record.telemetry?.contextTokens ?? record.observedContextTokens) !== undefined ? { contextTokens: record.telemetry?.contextTokens ?? record.observedContextTokens } : {}),
      ...(record.telemetry?.estimatedUsd !== undefined ? { estimatedUsd: record.telemetry.estimatedUsd } : {}),
      ...(outputTps !== undefined ? { outputTps } : {}),
    });
  }

  // Preserve each subtree as a contiguous block while retaining the flat
  // panel's state/elapsed ordering between root-level blocks.
  const blocks: AgentRowEntry[][] = [];
  let block: AgentRowEntry[] | undefined;
  for (const node of tree) {
    if (node.depth === 0) {
      block = [];
      blocks.push(block);
    }
    const row = rowById.get(node.id);
    // Every agent row owns an allotment: a local record's batches come from
    // its own buffer, a descendant's from the relayed `agent-view` model.
    if (row) block?.push({ row, agentId: node.id, batchSource: true });
  }

  for (const thread of threads) {
    if (!isLiveThreadStatus(thread.status) || coveredThreadIds.has(thread.threadId)) continue;
    const elapsedMs = clampElapsed(now - Date.parse(thread.touchedAt));
    blocks.push([{ batchSource: false, row: {
      name: sanitizeDisplayTitle(thread.title, thread.threadId),
      role: "thread",
      state: "awaiting-owner",
      elapsedMs,
      // No backing RpcAgentRecord for a pure synthetic thread row, so there is
      // no spawner.ts `lastActivityAt` to read — the thread's own touchedAt
      // delta (identical to elapsedMs here) is the best available signal.
      lastActivityMs: elapsedMs,
      answerHint: `/answer ${thread.threadId}`,
      ...(thread.forkResume?.telemetry?.model ?? thread.forkResume?.observedModel ? { model: thread.forkResume?.telemetry?.model ?? thread.forkResume?.observedModel } : {}),
      ...(thread.forkResume?.telemetry?.effort ?? thread.forkResume?.observedEffort ? { effort: thread.forkResume?.telemetry?.effort ?? thread.forkResume?.observedEffort } : {}),
      ...((thread.forkResume?.telemetry?.contextTokens ?? thread.forkResume?.observedContextTokens) !== undefined ? { contextTokens: thread.forkResume?.telemetry?.contextTokens ?? thread.forkResume?.observedContextTokens } : {}),
      ...(thread.forkResume?.telemetry?.estimatedUsd !== undefined ? { estimatedUsd: thread.forkResume.telemetry.estimatedUsd } : {}),
    } }]);
  }

  const nonEmptyBlocks = blocks.filter((candidate) => candidate.length > 0);
  nonEmptyBlocks.sort((a, b) => {
    const rankDiff = AGENT_STATE_RANK[a[0].row.state] - AGENT_STATE_RANK[b[0].row.state];
    return rankDiff !== 0 ? rankDiff : b[0].row.elapsedMs - a[0].row.elapsedMs;
  });
  return nonEmptyBlocks.flat();
}

/** `Xs` under a minute, `Xm` under an hour, else `XhYYm` — a compact, always-non-negative elapsed label. */
export function formatCompactDuration(elapsedMs: number): string {
  const totalSeconds = Math.max(0, Math.floor(Number.isFinite(elapsedMs) ? elapsedMs / 1000 : 0));
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  if (totalMinutes < 60) return `${totalMinutes}m`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${hours}h${String(minutes).padStart(2, "0")}m`;
}

/** Answer-capable owner rows lead with their valid `/answer qN` cue. */
function isAttentionState(state: AgentRowState): boolean {
  // Approval waits remain actionable lead-agent work through `ws-approve`, not
  // owner work. Only owner-held `/answer` paths receive the loud cue.
  return state === "awaiting-owner" || state === "idle-awaiting-owner";
}

/** Labeled current context-window occupancy; unknown is explicit across compaction gaps. */
export function formatContextTokens(tokens: number | undefined): string {
  return tokens === undefined ? "ctx ?" : `ctx ${(tokens / 1_000).toFixed(1)}k`;
}

/** Unlabeled counterpart for the live row; `/audit` retains the labeled formatter. */
function formatLiveContextTokens(tokens: number | undefined): string {
  return tokens === undefined ? "?" : `${(tokens / 1_000).toFixed(1)}k`;
}

function formatEstimatedUsd(usd: number | undefined): string {
  if (usd === undefined) return "—";
  return String(Number(usd.toFixed(3)));
}

function ownerAnswerCue(answerHint: string, width: number): string {
  const fullCue = `⚠ OWNER ACTION · ${answerHint}`;
  if (visibleWidth(fullCue) <= width) return fullCue;
  const compactCue = `⚠ ${answerHint}`;
  if (visibleWidth(compactCue) <= width) return compactCue;
  if (visibleWidth(answerHint) <= width) return answerHint;
  // A partial `/answer` text could look like a different command. When the
  // command itself cannot fit, retain only the non-command owner identity.
  return truncateToWidth("⚠ OWNER ACTION", width);
}

function truncateWithProtectedPrimary(primary: string, base: string, width: number): string {
  if (visibleWidth(base) <= width) return base;
  const primaryWidth = visibleWidth(primary);
  if (primaryWidth >= width) return primary;
  const suffixWidth = width - primaryWidth;
  return suffixWidth <= 1 ? primary : primary + truncateToWidth(base.slice(primary.length), suffixWidth);
}

/**
 * 260914: the leading state-bullet glyph (`AGENT_STATE_BULLET`), reserving its
 * own width slot ahead of the row body and degrading gracefully at narrow
 * widths (bare glyph with no trailing space at `width === glyphWidth`,
 * nothing at all below it) — the same narrow-width discipline `truncateToWidth`
 * uses elsewhere in this file. The bullet is COLOR-ONLY, never bold: the
 * owner-held pair (`awaiting-owner` / `idle-awaiting-owner`) takes the
 * caller's `ownerActionColor` so it tracks the existing 330ms attention cycle
 * (260908) without a second, independently-flashing bold span — the cycle's
 * one bold treatment stays on the `⚠ OWNER ACTION` cue text exactly as
 * before. Every other state gets a static color; `awaiting-approval` in
 * particular must never bold-flash (that toggle is reserved for owner-held
 * rows, see `isAttentionState`).
 */
function formatBullet(state: AgentRowState, width: number, ownerActionColor: OwnerActionColor | undefined, theme: AgentWidgetTheme | undefined): { text: string; width: number } {
  const glyph = AGENT_STATE_BULLET[state];
  const glyphWidth = visibleWidth(glyph);
  const candidate = width >= glyphWidth + 1 ? `${glyph} ` : width >= glyphWidth ? glyph : "";
  if (candidate === "") return { text: "", width: 0 };
  const color = isAttentionState(state) ? ownerActionColor : STATE_BULLET_COLOR[state];
  const styled = color && theme ? theme.fg(color, candidate) : candidate;
  return { text: styled, width: visibleWidth(candidate) };
}

/** One dim `│ ` lane per propagated depth, clipped before the state bullet at narrow widths. */
function formatDepthGutter(depth: number, width: number, theme: AgentWidgetTheme | undefined): { text: string; width: number } {
  let plain = "";
  for (let lane = 0; lane < depth && visibleWidth(plain) < width; lane++) {
    const remaining = width - visibleWidth(plain);
    plain += remaining >= 2 ? "│ " : "│";
  }
  return { text: theme && plain ? theme.fg("dim", plain) : plain, width: visibleWidth(plain) };
}

/**
 * 261028 batch-row redesign: the expanded mode's tree cue on a header row is
 * plain indentation, two spaces per depth; its batch rows carry the `│`.
 */
function formatDepthIndent(depth: number, width: number): { text: string; width: number } {
  const text = " ".repeat(Math.max(0, Math.min(2 * depth, width)));
  return { text, width: text.length };
}

function formatRow(row: AgentRow, width = DEFAULT_AGENT_WIDGET_WIDTH, ownerActionColor: OwnerActionColor | undefined, theme?: AgentWidgetTheme, treeCue: "lanes" | "indent" = "lanes"): string {
  const gutter = treeCue === "indent" ? formatDepthIndent(row.depth ?? 0, width) : formatDepthGutter(row.depth ?? 0, width, theme);
  const rowWidth = Math.max(0, width - gutter.width);
  const bullet = formatBullet(row.state, rowWidth, ownerActionColor, theme);
  const bodyWidth = Math.max(0, rowWidth - bullet.width);
  const ownerAction = isAttentionState(row.state);
  const primary = row.answerHint
    ? ownerAnswerCue(row.answerHint, bodyWidth)
    : ownerAction
      ? `⚠ OWNER ACTION · ${sanitizeDisplayTitle(row.name, "owner action")}`
      : row.name;
  const stateLabel = AGENT_STATE_LABEL[row.state];
  if (row.livenessOnly) {
    return gutter.text + bullet.text + truncateToWidth(`${primary} · ${stateLabel}`, bodyWidth);
  }
  const durationPrefix = `${primary} · ${stateLabel} · ${formatCompactDuration(row.elapsedMs)} (`;
  const activity = formatCompactDuration(row.lastActivityMs);
  const baseWithoutTps = `${durationPrefix}${activity})`;
  const model = row.model ?? "—";
  const effort = row.effort ?? "—";
  // Keep the compact occupancy value in its established telemetry slot while
  // `/audit` retains its labeled `formatContextTokens` presentation.
  const contextTokens = formatLiveContextTokens(row.contextTokens);
  const estimate = `$${formatEstimatedUsd(row.estimatedUsd)}`;
  const tps = row.outputTps !== undefined ? formatOutputRate(row.outputTps) : undefined;
  const baseWithTps = tps === undefined ? baseWithoutTps : `${durationPrefix}${activity}, ${tps})`;
  const telemetry = ` · ${model} (${effort}) · ${contextTokens} · ${estimate}`;
  // Drop TPS from the duration first; only then does the all-or-nothing rule
  // for the remaining telemetry group apply.
  const pickBase = (available: number) => visibleWidth(baseWithTps + telemetry) <= available ? baseWithTps : baseWithoutTps;
  const protectedHint = row.inspectionHint;
  const hint = protectedHint ? ` — ${protectedHint}` : "";
  // A supplied inspection affordance remains the only protected tail. The
  // valid owner-answer command is protected inside the leading primary cue.
  let line: string;
  let base: string;
  let appendedHint = false;
  let appendedTelemetry = false;
  if (protectedHint && visibleWidth(hint) <= bodyWidth) {
    const available = bodyWidth - visibleWidth(hint);
    base = pickBase(available);
    const withTelemetry = base + telemetry;
    appendedTelemetry = visibleWidth(withTelemetry) <= available;
    line = appendedTelemetry ? withTelemetry + hint : (row.answerHint ? truncateWithProtectedPrimary(primary, base, available) : truncateToWidth(base, available)) + hint;
    appendedHint = true;
  } else {
    base = pickBase(bodyWidth);
    appendedTelemetry = visibleWidth(base + telemetry) <= bodyWidth;
    line = appendedTelemetry ? base + telemetry : row.answerHint ? truncateWithProtectedPrimary(primary, base, bodyWidth) : truncateToWidth(base, bodyWidth);
  }
  // Add ANSI only after width truncation: styling before truncation can leave
  // an incomplete escape sequence in a narrow terminal.
  let content = appendedHint ? line.slice(0, -hint.length) : line;
  const suffix = appendedHint ? hint : "";
  if (appendedTelemetry && theme) {
    const styledTelemetry =
      theme.fg("dim", " · ") +
      theme.fg("accent", model) +
      theme.fg("dim", ` (${effort})`) +
      theme.fg("dim", " · ") +
      theme.fg("syntaxNumber", contextTokens) +
      theme.fg("dim", " · ") +
      theme.fg("warning", estimate);
    content = base + styledTelemetry;
  }
  if (theme && base === baseWithTps && tps !== undefined && appendedTelemetry) {
    const rateStart = durationPrefix.length + activity.length + 2;
    content = content.slice(0, rateStart) + theme.fg("syntaxNumber", tps) + content.slice(rateStart + tps.length);
  }
  // Style only the surviving activity value, after plain-text truncation.
  // It now belongs to the duration field even when telemetry does not fit.
  if (theme && content.length > durationPrefix.length) {
    const end = Math.min(durationPrefix.length + activity.length, content.length);
    const visibleActivity = content.slice(durationPrefix.length, end);
    content = content.slice(0, durationPrefix.length) +
      theme.fg("syntaxNumber", visibleActivity) + content.slice(end);
  }
  if (ownerActionColor && ownerAction && content.length > 0) {
    // The owner-action identity/cue is the first structured field. It stays
    // bold in every color phase; state, separators, and telemetry retain
    // their existing semantic styling. Styling after truncation avoids partial
    // escape sequences at narrow widths.
    const cueEnd = Math.min(content.length, primary.length);
    content = semanticBold(content.slice(0, cueEnd), ownerActionColor, theme) + content.slice(cueEnd);
  }
  return gutter.text + bullet.text + content + suffix;
}

/**
 * Truncates `text` to at most `width` display columns (`visibleWidth`,
 * reused from `text-width.ts` rather than duplicated), appending a single
 * ellipsis character when truncation actually occurs. Never throws or
 * produces a wider-than-`width` result, even for `width <= 1`.
 */
function truncateToWidth(text: string, width: number): string {
  if (width <= 0) return "";
  if (visibleWidth(text) <= width) return text;
  const ellipsis = "…";
  const ellipsisWidth = visibleWidth(ellipsis);
  if (width <= ellipsisWidth) return ellipsis.slice(0, width);
  let result = "";
  let usedWidth = 0;
  for (const char of text) {
    const charWidth = visibleWidth(char);
    if (usedWidth + charWidth > width - ellipsisWidth) break;
    result += char;
    usedWidth += charWidth;
  }
  return result + ellipsis;
}

/**
 * Renders `rows` (as produced by `buildAgentRows`, already sorted) with no
 * separate heading line. 260914 dropped the `ws: N agents` count heading
 * (superseding 260909's `buildHeadingLine`): with every row visible, the
 * owner counts by eye, and each row's own state bullet (`AGENT_STATE_BULLET`)
 * plus its `AGENT_STATE_LABEL` already carry what the heading used to
 * summarize. `pendingCount` remains the visibility guard's pending-question
 * fallback — every live/open thread `buildAgentRows` counts already yields a
 * row of its own, so this is normally redundant with `rows.length`, but
 * keeping it preserves the exact prior hide/show contract. Rows do not
 * consume the ticket's five-row cap by themselves: every awaiting-state row
 * is kept, `running` rows are trimmed so the body has `AGENT_WIDGET_ROW_CAP`
 * rows with a synthetic `+N more` trailing line. Every line is bounded to
 * `width` display columns via `truncateToWidth`. `undefined` only when rows
 * and pending questions are both absent.
 */
export function buildWidgetLines(rows: readonly AgentRow[], pendingCount: number, width: number = DEFAULT_AGENT_WIDGET_WIDTH, emphasizeAttention = false, theme?: AgentWidgetTheme, ownerActionColor: OwnerActionColor = "error"): string[] | undefined {
  if (rows.length === 0 && pendingCount <= 0) return undefined;
  const emphasizeOwnerAttention = emphasizeAttention && rows.some((row) => isAttentionState(row.state));

  const awaiting = rows.filter((row) => row.state !== "running");
  const running = rows.filter((row) => row.state === "running");

  let shown: readonly AgentRow[];
  let hiddenRunning = 0;
  if (awaiting.length + running.length <= AGENT_WIDGET_ROW_CAP) {
    shown = rows;
  } else {
    let runningSlots = Math.max(0, AGENT_WIDGET_ROW_CAP - awaiting.length);
    const retained: AgentRow[] = [];
    for (const row of rows) {
      if (row.state !== "running") retained.push(row);
      else if (runningSlots > 0) {
        retained.push(row);
        runningSlots--;
      } else hiddenRunning++;
    }
    shown = retained;
  }

  const lines = shown.map((row) => formatRow(row, width, emphasizeOwnerAttention && isAttentionState(row.state) ? ownerActionColor : undefined, theme));
  if (hiddenRunning > 0) lines.push(truncateToWidth(`+${hiddenRunning} more`, width));
  return lines;
}

// ---------------------------------------------------------------------------
// 261028: expanded mode — per-agent tool batch rows under each agent header.
// ---------------------------------------------------------------------------

/** Default batch rows per agent in the expanded mode (`pi.agent_widget_rows`). */
export const DEFAULT_AGENT_WIDGET_BATCH_ROWS = 3;

/** Upper bound on batch rows per agent: a relayed descendant view carries at most this many batches. */
export const MAX_AGENT_WIDGET_BATCH_ROWS = AGENT_BATCH_CAP;

/** Repaint cadence while expanded and a batch is running, for its elapsed field and spinner. */
export const AGENT_WIDGET_EXPANDED_TICK_MS = 1_000;

/** Transcript rows the expanded widget always leaves visible. */
export const MIN_TRANSCRIPT_ROWS = 5;

/**
 * Conservative estimate of the dock rows outside this widget (editor, footer,
 * status line, widget spacers) when the host's components cannot be measured.
 * Overestimating only shrinks the widget; underestimating would push more
 * transcript into scrollback.
 */
export const ESTIMATED_DOCK_ROWS = 10;

/** Terminal height assumed when the host exposes none. */
const FALLBACK_TERMINAL_ROWS = 24;

const SPINNER_FRAMES = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"] as const;

/** Clamps a configured batch row count to `1..MAX_AGENT_WIDGET_BATCH_ROWS`; anything else is the default. */
export function resolveAgentWidgetRows(value: unknown): number {
  return typeof value === "number" && Number.isInteger(value) && value >= 1
    ? Math.min(value, MAX_AGENT_WIDGET_BATCH_ROWS)
    : DEFAULT_AGENT_WIDGET_BATCH_ROWS;
}

/** Rows the expanded widget may use: terminal rows minus the rest of the dock and `MIN_TRANSCRIPT_ROWS`, never below one. */
export function computeExpandedBudget(terminalRows: number, reservedRows: number): number {
  return Math.max(1, Math.floor(terminalRows) - Math.max(0, Math.floor(reservedRows)) - MIN_TRANSCRIPT_ROWS);
}

/**
 * The uniform per-agent batch allotment: the largest `n <= rowsPerAgent` at
 * which every header plus `n` rows for each batch source fits `budget`, down
 * to one. Below one row each, `planExpandedRows` folds agents into a tail.
 */
export function chooseBatchAllotment(entries: readonly AgentRowEntry[], rowsPerAgent: number, budget: number): number {
  const sources = entries.filter((entry) => entry.batchSource).length;
  for (let n = Math.max(1, rowsPerAgent); n > 1; n--) {
    if (entries.length + sources * n <= budget) return n;
  }
  return 1;
}

export interface ExpandedPlanItem {
  entry: AgentRowEntry;
  /** Batch rows under this header: the allotment, or zero for a header kept only for visibility. */
  batchRows: number;
}

export interface ExpandedPlan {
  items: ExpandedPlanItem[];
  /** Agents folded into the `+N more` tail. */
  hidden: number;
  /** Blank lines appended so a folded widget keeps exactly `budget` rows. */
  padding: number;
}

/** Index of each entry's nearest preceding shallower entry: its parent in the parent-before-child row order. */
function parentIndexes(entries: readonly AgentRowEntry[]): number[] {
  const parents: number[] = [];
  const stack: number[] = [];
  entries.forEach((entry, index) => {
    const depth = entry.row.depth ?? 0;
    while (stack.length > 0 && (entries[stack[stack.length - 1]].row.depth ?? 0) >= depth) stack.pop();
    parents.push(stack.length > 0 ? stack[stack.length - 1] : -1);
    stack.push(index);
  });
  return parents;
}

/**
 * Which agents the expanded widget shows, and with how many batch rows.
 *
 * When every header plus the allotment for each batch source fits `budget`,
 * everything is shown. Otherwise running agents fold into a `+N more` tail:
 * an agent in any non-running state is never folded and keeps at least its
 * header, together with its ancestors' headers so its tree marks stay intact;
 * the remaining budget then goes, in row order, to those agents' batch rows
 * and to running agents (header, any hidden ancestors, and the allotment).
 * When the kept headers alone exceed `budget`, `budget` is exceeded rather
 * than such a row hidden. A folded widget pads to exactly `budget` rows.
 */
export function planExpandedRows(entries: readonly AgentRowEntry[], allotment: number, budget: number): ExpandedPlan {
  const sources = entries.filter((entry) => entry.batchSource).length;
  if (entries.length + sources * allotment <= budget) {
    return { items: entries.map((entry) => ({ entry, batchRows: entry.batchSource ? allotment : 0 })), hidden: 0, padding: 0 };
  }
  const parents = parentIndexes(entries);
  const shown = entries.map(() => false);
  const batchRows = entries.map(() => 0);
  const showWithAncestors = (index: number): void => {
    for (let at = index; at >= 0 && !shown[at]; at = parents[at]) shown[at] = true;
  };
  entries.forEach((entry, index) => {
    if (entry.row.state !== "running") showWithAncestors(index);
  });
  let remaining = budget - shown.filter(Boolean).length - 1;
  entries.forEach((entry, index) => {
    const rows = entry.batchSource ? allotment : 0;
    if (shown[index]) {
      if (rows > 0 && remaining >= rows) {
        batchRows[index] = rows;
        remaining -= rows;
      }
      return;
    }
    let hiddenAncestors = 0;
    for (let at = parents[index]; at >= 0 && !shown[at]; at = parents[at]) hiddenAncestors++;
    const cost = 1 + hiddenAncestors + rows;
    if (cost > remaining) return;
    showWithAncestors(index);
    batchRows[index] = rows;
    remaining -= cost;
  });
  const items = entries.flatMap((entry, index) => (shown[index] ? [{ entry, batchRows: batchRows[index] }] : []));
  const hidden = entries.length - items.length;
  const height = items.length + batchRows.reduce((sum, rows) => sum + rows, 0) + (hidden > 0 ? 1 : 0);
  return { items, hidden, padding: Math.max(0, budget - height) };
}

/** `Xs ago`-style relative time from the shared compact duration formatter. */
function formatRelative(ms: number): string {
  return `${formatCompactDuration(ms)} ago`;
}

function plural(count: number, noun: string): string {
  return `${count} ${noun}${count === 1 ? "" : "s"}`;
}

/** Mark-column glyphs of a batch row (a running or generating row's mark is the spinner). */
export const BATCH_FINISHED_MARK = "✓";
export const BATCH_FAILED_MARK = "✗";
export const BATCH_PLACEHOLDER_MARK = "-";

/** ` · N failed` when any call failed, else nothing. */
function failedMetric(view: BatchView): string {
  return view.failed > 0 ? ` · ${view.failed} failed` : "";
}

/** The metric column of a finished batch: `N calls · N failed · <rel> ago`, `N failed` only when any call failed. */
export function formatFinishedBatchMetrics(view: BatchView, now: number): string {
  return `${plural(view.callCount, "call")}${failedMetric(view)} · ${formatRelative(now - (view.endedAt ?? now))}`;
}

/**
 * The metric column of a running batch: `<done>/<n> · +<k> · N failed ·
 * <elapsed>`, with `+<k>` (other calls still in flight) only when k > 0,
 * `N failed` only when a call already failed, and the elapsed time of the
 * reported (oldest in-flight) call.
 */
export function formatRunningBatchMetrics(view: BatchView, now: number): string {
  const running = view.running;
  if (!running) return formatFinishedBatchMetrics(view, now);
  const others = running.othersRunning > 0 ? ` · +${running.othersRunning}` : "";
  return `${running.done}/${view.callCount}${others}${failedMetric(view)} · ${formatCompactDuration(clampElapsed(now - running.startedAt))}`;
}

/** A generating row's prose: `thinking`, `replying`, or the streaming call's tool name. */
export function formatGeneratingProse(view: BatchView): string {
  const generating = view.generating;
  if (!generating) return "";
  if (generating.kind === "tool") return sanitizeDisplayTitle(generating.toolName, "tool");
  return generating.kind === "thinking" ? "thinking" : "replying";
}

/** A generating row's metrics: `<N> chars · <elapsed>` while a tool call streams, else `<elapsed>`; elapsed from the phase's start. */
export function formatGeneratingMetrics(view: BatchView, now: number): string {
  const generating = view.generating;
  if (!generating) return "";
  const elapsed = formatCompactDuration(clampElapsed(now - generating.phaseStartedAt));
  return generating.kind === "tool" ? `${plural(generating.chars ?? 0, "char")} · ${elapsed}` : elapsed;
}

/** The spinner frame for `now`: one step per expanded tick. */
function spinnerFrame(now: number): string {
  return SPINNER_FRAMES[Math.floor(Math.max(0, now) / AGENT_WIDGET_EXPANDED_TICK_MS) % SPINNER_FRAMES.length];
}

/**
 * Fits a batch row's prose into `width`. A running call's prose is
 * `<tool> (<args>)`: its args preview alone truncates, inside the
 * parentheses, while `<tool> ()` and two columns of args fit.
 */
function fitBatchProse(head: string, args: string, width: number): string {
  if (args === "") return truncateToWidth(head, width);
  const fixed = visibleWidth(`${head} ()`);
  if (fixed + 2 > width) return truncateToWidth(`${head} (${args})`, width);
  return `${head} (${truncateToWidth(args, width - fixed)})`;
}

/**
 * A batch row's body after its mark: the prose left, the metric column
 * right-aligned to exactly `width` columns with at least one space between.
 * The prose truncates first; when the metrics alone do not fit, they
 * truncate and the prose is dropped.
 */
export function layoutBatchRowBody(prose: { head: string; args?: string }, metrics: string, width: number): string {
  if (width <= 0) return "";
  const metricsWidth = visibleWidth(metrics);
  if (metricsWidth > width) return truncateToWidth(metrics, width);
  const proseWidth = width - metricsWidth - 1;
  const fitted = proseWidth > 0 ? fitBatchProse(prose.head, prose.args ?? "", proseWidth) : "";
  return fitted + " ".repeat(width - visibleWidth(fitted) - metricsWidth) + metrics;
}

type BatchRowStyle = "dim" | "error" | undefined;

/**
 * One expanded-mode batch row: the agent's indent (two spaces per depth), a
 * dim `│`, the mark column, then the body. Parts are clipped to `width` in
 * plain text and styled afterwards, so a narrow row never cuts an escape.
 */
function composeBatchRow(depth: number, width: number, mark: string, markStyle: BatchRowStyle, body: string, bodyStyle: BatchRowStyle, theme: AgentWidgetTheme | undefined): string {
  const indent = formatDepthIndent(depth, width);
  const parts: Array<[string, BatchRowStyle]> = [["│", "dim"], [" ", undefined], [mark, markStyle]];
  if (body !== "") parts.push([" ", undefined], [body, bodyStyle]);
  let remaining = width - indent.width;
  let out = indent.text;
  for (const [text, style] of parts) {
    if (remaining <= 0 || text === "") break;
    const clipped = visibleWidth(text) <= remaining ? text : truncateToWidth(text, remaining);
    remaining -= visibleWidth(clipped);
    out += style && theme && clipped.trim() !== "" ? theme.fg(style, clipped) : clipped;
  }
  return out;
}

/** The running batch row: the spinner, then `<tool> (<args>)` and its running metrics. */
function formatRunningBatchRow(view: BatchView, depth: number, now: number, width: number, theme: AgentWidgetTheme | undefined): string {
  const running = view.running!;
  const available = Math.max(0, width - 2 * depth - 4);
  const body = layoutBatchRowBody({ head: sanitizeDisplayTitle(running.toolName, "tool"), args: running.argsPreview }, formatRunningBatchMetrics(view, now), available);
  return composeBatchRow(depth, width, spinnerFrame(now), undefined, body, undefined, theme);
}

/** The generating row, bright: the spinner, the phase label or streaming tool, and its metrics. */
function formatGeneratingBatchRow(view: BatchView, depth: number, now: number, width: number, theme: AgentWidgetTheme | undefined): string {
  const available = Math.max(0, width - 2 * depth - 4);
  const body = layoutBatchRowBody({ head: formatGeneratingProse(view) }, formatGeneratingMetrics(view, now), available);
  return composeBatchRow(depth, width, spinnerFrame(now), undefined, body, undefined, theme);
}

/**
 * A finished batch row, dimmed: `✓` (or `✗` in the error color when a call
 * failed), the summary or tool names, and its metrics.
 */
function formatFinishedBatchRow(view: BatchView, summary: string | undefined, depth: number, now: number, width: number, theme: AgentWidgetTheme | undefined): string {
  const available = Math.max(0, width - 2 * depth - 4);
  const body = layoutBatchRowBody({ head: summary ?? view.toolNames.join(", ") }, formatFinishedBatchMetrics(view, now), available);
  const failed = view.failed > 0;
  return composeBatchRow(depth, width, failed ? BATCH_FAILED_MARK : BATCH_FINISHED_MARK, failed ? "error" : "dim", body, "dim", theme);
}

/** An unfilled history slot: the `│`, then a dim `-` in the mark column. */
function formatPlaceholderBatchRow(depth: number, width: number, theme: AgentWidgetTheme | undefined): string {
  return composeBatchRow(depth, width, BATCH_PLACEHOLDER_MARK, "dim", "", undefined, theme);
}

export interface ExpandedWidgetOptions {
  width?: number;
  now: number;
  allotment: number;
  budget: number;
  /** Each agent's batch views, oldest first. */
  batches: ReadonlyMap<string, readonly BatchView[]>;
  /** Display summary of a finished batch, when one exists. */
  summaryFor?: (agentId: string, view: BatchView) => string | undefined;
  emphasizeAttention?: boolean;
  theme?: AgentWidgetTheme;
  ownerActionColor?: OwnerActionColor;
}

/**
 * Renders the expanded mode: each planned agent's header row (the normal
 * mode's `formatRow`, indented two spaces per depth instead of the `│ `
 * lanes), then its latest batch rows (oldest first, the running batch last),
 * then `-` placeholder rows up to its allotment, then the `+N more` tail and
 * any blank fold padding. Every batch row starts with a dim `│` under its
 * agent's bullet; prose sits left and metrics right-aligned. Finished rows
 * are dimmed, `✓` included, and only a failed batch's `✗` takes the error
 * color; the running row and the generating row (an assistant message not
 * yet a batch) are bright. `undefined` under
 * the same hide rule as `buildWidgetLines`.
 */
export function buildExpandedWidgetLines(entries: readonly AgentRowEntry[], pendingCount: number, options: ExpandedWidgetOptions): string[] | undefined {
  if (entries.length === 0 && pendingCount <= 0) return undefined;
  const width = options.width ?? DEFAULT_AGENT_WIDGET_WIDTH;
  const theme = options.theme;
  const emphasizeOwnerAttention = options.emphasizeAttention === true && entries.some((entry) => isAttentionState(entry.row.state));
  const plan = planExpandedRows(entries, options.allotment, options.budget);
  const lines: string[] = [];
  for (const { entry, batchRows } of plan.items) {
    const row = entry.row;
    const depth = row.depth ?? 0;
    lines.push(formatRow(row, width, emphasizeOwnerAttention && isAttentionState(row.state) ? options.ownerActionColor ?? "error" : undefined, theme, "indent"));
    if (batchRows === 0) continue;
    const views = (entry.agentId !== undefined ? options.batches.get(entry.agentId) : undefined) ?? [];
    const shown = views.slice(-batchRows);
    for (const view of shown) {
      if (view.generating) {
        lines.push(formatGeneratingBatchRow(view, depth, options.now, width, theme));
        continue;
      }
      if (view.running) {
        lines.push(formatRunningBatchRow(view, depth, options.now, width, theme));
        continue;
      }
      const summary = entry.agentId !== undefined ? options.summaryFor?.(entry.agentId, view) : undefined;
      lines.push(formatFinishedBatchRow(view, summary || undefined, depth, options.now, width, theme));
    }
    for (let slot = shown.length; slot < batchRows; slot++) lines.push(formatPlaceholderBatchRow(depth, width, theme));
  }
  if (plan.hidden > 0) lines.push(truncateToWidth(`+${plan.hidden} more`, width));
  for (let pad = 0; pad < plan.padding; pad++) lines.push("");
  return lines;
}

/** Whether any shown agent has a running or generating batch — the expanded mode's 1 s repaint condition. */
export function hasRunningBatch(entries: readonly AgentRowEntry[], batches: ReadonlyMap<string, readonly BatchView[]>): boolean {
  return entries.some((entry) => entry.agentId !== undefined && (batches.get(entry.agentId) ?? []).some((view) => view.running !== undefined || view.generating !== undefined));
}

/** Every local record's batch views plus each record's relayed descendant views, keyed by agent id; a local buffer wins. */
export function collectBatchViews(records: RpcAgentRegistry): Map<string, BatchView[]> {
  const out = new Map<string, BatchView[]>();
  for (const record of records.values()) {
    if (record.toolBatches) out.set(record.agentId, batchViews(record.toolBatches));
  }
  for (const record of records.values()) {
    for (const [id, views] of record.descendantViews ?? []) {
      if (!out.has(id)) out.set(id, views);
    }
  }
  return out;
}

/** Duck-typed host component: anything with `render(width)` and, for containers, `children`. */
interface MeasurableComponent {
  render?(width: number): unknown;
  children?: unknown;
}

/**
 * Measures the host dock rows outside this widget: every top-level TUI child
 * after the first (the transcript), with this widget's own component skipped
 * inside its container. `undefined` when the host does not expose that shape
 * or this widget is not found in it, so the caller falls back to
 * `ESTIMATED_DOCK_ROWS`.
 */
export function measureDockRows(tui: unknown, self: unknown, width: number): number | undefined {
  try {
    const children = (tui as { children?: unknown } | undefined)?.children;
    if (!Array.isArray(children) || children.length < 2) return undefined;
    const height = (component: MeasurableComponent): number => {
      const lines = component?.render?.(width);
      return Array.isArray(lines) ? lines.length : 0;
    };
    let total = 0;
    let found = false;
    for (const child of children.slice(1) as MeasurableComponent[]) {
      if (child === self) {
        found = true;
        continue;
      }
      const nested = child?.children;
      if (Array.isArray(nested) && nested.includes(self)) {
        found = true;
        for (const sibling of nested as MeasurableComponent[]) if (sibling !== self) total += height(sibling);
        continue;
      }
      total += height(child);
    }
    return found ? total : undefined;
  } catch {
    return undefined;
  }
}

/** An entry's identity for the expanded layout cache: who it is and whether it owns an allotment, not its state or order. */
function entryLayoutKey(entry: AgentRowEntry): string {
  return `${entry.agentId ?? `thread:${entry.row.name}`}:${entry.batchSource ? 1 : 0}`;
}

/** Terminal height from the host TUI, when exposed. */
function terminalRowsOf(tui: unknown): number | undefined {
  const rows = (tui as { terminal?: { rows?: unknown } } | undefined)?.terminal?.rows;
  return typeof rows === "number" && Number.isFinite(rows) && rows > 0 ? rows : undefined;
}

/**
 * 260905 review relay #1 (Important #5): the widget's own arming gate,
 * extracted out of `index.ts`'s `session_start` into a small pure predicate
 * so "the widget is wired only for a TUI lead/fork session" is a directly
 * testable invariant again — the deleted `ask.ts` `refreshPendingWidget`
 * used to carry a directly-tested `ctx.mode !== "tui"` guard of its own; that
 * coverage lapsed when the guard moved into `index.ts`'s `session_start`,
 * which has no test file. Mirrors the exact inline gate `index.ts` used
 * (`isLeadOrFork(readSpawnRole(process.env)) && ctx.mode === "tui"`) —
 * `index.ts` calls this instead of repeating the two conditions itself.
 */
export function shouldArmAgentWidget(role: SpawnRole | undefined, mode: string | undefined): boolean {
  return isLeadOrFork(role) && mode === "tui";
}

/** 261028: the slash command that toggles the expanded mode. Checked against the host's built-in commands: no clash. */
export const AGENT_WIDGET_COMMAND = "agents";

/** Duck-typed slice of `pi.registerCommand` this module uses. */
export interface AgentWidgetCommandHost {
  registerCommand(name: string, options: { description: string; handler: (args: string, ctx: { ui?: { notify?(message: string, type?: string): void } }) => unknown }): void;
}

/**
 * Registers `/agents`, a toggle between the widget's normal and expanded
 * modes. `handle` is read at each invocation: it is undefined outside a TUI
 * lead or fork session, where the command only reports that.
 */
export function registerAgentWidgetCommand(pi: AgentWidgetCommandHost, handle: () => AgentWidgetController | undefined): void {
  pi.registerCommand(AGENT_WIDGET_COMMAND, {
    description: "Toggle the live agent widget between its normal and expanded mode (recent tool batches per agent).",
    handler: (_args, ctx) => {
      const controller = handle();
      if (!controller) {
        ctx.ui?.notify?.("The live agent widget is available only in a TUI lead session.", "warning");
        return;
      }
      controller.toggleExpanded();
    },
  });
}

// ---------------------------------------------------------------------------
// IO glue: the setWidget repaint, retired-status clear, plus the arm-while-visible
// elapsed timer. Controller-facing coverage is in `test/agent-widget.test.ts`.
// ---------------------------------------------------------------------------

/**
 * Duck-typed `Component` surface this controller's `setWidget` factory
 * returns — mirrors `pi-tui`'s `Component.render(width)` contract, the same
 * one `conversation-view.ts`'s `ConversationViewComponent.render` implements.
 */
export interface AgentWidgetComponent {
  render(width: number): string[];
  invalidate(): void;
}

/** Theme subset used by the widget's semantic owner-action and telemetry accents. */
export interface AgentWidgetTheme {
  fg(color: "error" | "warning" | "accent" | "dim" | "syntaxNumber", text: string): string;
}

function asAgentWidgetTheme(theme: unknown): AgentWidgetTheme | undefined {
  return typeof (theme as { fg?: unknown } | undefined)?.fg === "function" ? theme as AgentWidgetTheme : undefined;
}

/**
 * Minimal duck-typed `ctx` surface this controller needs — the same
 * convention `ask.ts`'s `AskUiCtx` uses. `setWidget`'s content param accepts
 * either a plain line array or a `(tui, theme) => Component` factory
 * (`ExtensionUIContext.setWidget`'s second overload,
 * `@earendil-works/pi-coding-agent`'s `types.d.ts`); this controller always
 * uses the factory overload (review relay #1 Important #3) so `render(width)`
 * is called with the REAL terminal width at repaint time instead of the
 * fixed `DEFAULT_AGENT_WIDGET_WIDTH`. `tui`/`theme` are typed `unknown`
 * because the factory below never reads either.
 */
export interface AgentWidgetUiCtx {
  ui?: {
    setWidget?(
      key: string,
      content: string[] | ((tui: unknown, theme: unknown) => AgentWidgetComponent) | undefined,
      options?: { placement?: string },
    ): void;
    setStatus?(key: string, text: string | undefined): void;
  };
}

export interface AgentWidgetController {
  /** Recomputes rows from the live registries and repaints the widget. Arms the elapsed timer when the panel becomes visible, disarms it when it becomes empty. */
  refresh(): void;
  /** 261028: switches between the normal and the expanded mode, repaints, and returns whether the widget is now expanded. Session-only state. */
  toggleExpanded(): boolean;
  isExpanded(): boolean;
  /** Disarms the timer, clears the widget, and clears the retired status segment. Call once, from `session_shutdown`. */
  stop(): void;
}

export interface AgentWidgetControllerOptions {
  /** Only the host lead owns the attention timer; forks retain the ordinary panel. */
  ownerLead?: boolean;
  /**
   * Read the animation setting afresh at each `refresh()`. An asynchronous
   * answer repaints when it lands; the ticks between refreshes reuse the
   * last answer rather than re-reading at every frame. Absent means enabled.
   */
  animationEnabled?: () => boolean | Promise<boolean>;
  /**
   * 261028: the expanded mode's batch rows per agent (`pi.agent_widget_rows`),
   * read afresh at each `refresh()` the same way as `animationEnabled`.
   * Absent means `DEFAULT_AGENT_WIDGET_BATCH_ROWS`.
   */
  rowsPerAgent?: () => number | Promise<number>;
  /**
   * 261028 Phase 3: the batch summarizer (`batch-summary.ts`), owner lead
   * only. While expanded, every finished batch the widget shows is offered
   * to it once; its summaries replace the fallback row text. The summarizer
   * itself makes no model call unless its flag and model are configured.
   */
  batchSummaries?: { request(items: readonly BatchSummaryItem[]): void; get(key: string): string | undefined };
}

/**
 * The finished batches an expanded widget shows (each agent's latest
 * `rowsPerAgent`), as summary items. A local record's item carries its calls'
 * previews; a relayed descendant's carries tool names only.
 */
export function visibleBatchSummaryItems(entries: readonly AgentRowEntry[], batches: ReadonlyMap<string, readonly BatchView[]>, records: RpcAgentRegistry, rowsPerAgent: number): BatchSummaryItem[] {
  const items: BatchSummaryItem[] = [];
  for (const entry of entries) {
    if (!entry.batchSource || entry.agentId === undefined) continue;
    const local = records.get(entry.agentId)?.toolBatches;
    for (const view of (batches.get(entry.agentId) ?? []).slice(-rowsPerAgent)) {
      if (view.running || view.endedAt === undefined) continue;
      const calls = local?.batches.find((batch) => batch.seq === view.seq)?.calls;
      items.push(batchSummaryItem(entry.agentId, entry.row.name, view, calls));
    }
  }
  return items;
}

/**
 * Builds the IO controller `index.ts` wires into `spawner.ts`'s
 * `agentWidgetRefreshRef` (and calls directly from its own `session_start`/
 * `session_shutdown`). `registry`/`threads` are read fresh on every
 * `refresh()` call — no cached row state — so a caller may safely hold this
 * controller for the whole session lifetime.
 *
 * The 10-second timer (`AGENT_WIDGET_TICK_MS`) is armed only while the most
 * recently computed panel is visible, mirroring `spawner.ts`'s
 * `startLivenessProbe` arm/disarm-a-timer-only-while-outstanding pattern —
 * an idle lead that has never spawned anything, or one whose registry has
 * gone fully quiet, pays nothing for elapsed-clock upkeep.
 */
export function createAgentWidgetController(ctx: AgentWidgetUiCtx, registry: RpcAgentRegistry, threads: Map<string, ThreadRecord>, options: AgentWidgetControllerOptions = {}): AgentWidgetController {
  let timer: ReturnType<typeof setInterval> | undefined;
  let attentionTimer: ReturnType<typeof setInterval> | undefined;
  let attentionPhase = 0;
  let animationEnabled = true;
  let stopped = false;
  let timerMs: number | undefined;
  // 261028: expanded mode. Every launch starts in normal mode.
  let expanded = false;
  let rowsPerAgent = DEFAULT_AGENT_WIDGET_BATCH_ROWS;
  // The allotment and budget hold until an agent enters or leaves, the
  // terminal resizes, or the row count changes, so the expanded widget's
  // height does not move while batches come and go.
  let layout: { signature: string; allotment: number; budget: number } | undefined;

  function clearAttentionTimer(): void {
    if (attentionTimer) {
      clearInterval(attentionTimer);
      attentionTimer = undefined;
    }
  }

  function paint(): void {
    const threadList = [...threads.values()];
    const now = Date.now();
    const entries = buildAgentRowEntries(registry, threadList, now);
    const rows = entries.map((entry) => entry.row);
    const expandedNow = expanded;
    const batches = expandedNow ? collectBatchViews(registry) : undefined;
    const fastTick = batches !== undefined && hasRunningBatch(entries, batches);
    const rowsNow = rowsPerAgent;
    const summaries = options.batchSummaries;
    if (batches && summaries) {
      try {
        // The last layout's allotment, when it has shrunk below the
        // configured rows, bounds what is shown and so what is summarized.
        summaries.request(visibleBatchSummaryItems(entries, batches, registry, Math.min(rowsNow, layout?.allotment ?? rowsNow)));
      } catch {
        // Cosmetic: a summarizer failure never costs a repaint.
      }
    }
    const summaryFor = summaries ? (agentId: string, view: BatchView) => summaries.get(batchSummaryKey(agentId, view.seq, view.epoch)) : undefined;
    const pendingCount = threadList.filter((thread) => thread.status === "pending").length;
    const visible = rows.length > 0 || pendingCount > 0;
    const qualifying = rows.some((row) => isAttentionState(row.state));
    const animate = options.ownerLead === true && animationEnabled && qualifying;
    const ownerActionColor = qualifying && (!animationEnabled || animate)
      ? (animationEnabled ? OWNER_ACTION_COLORS[attentionPhase] : "error")
      : undefined;
    try {
      // 260905 review relay #1 (Important #3): pass the factory overload, not
      // a pre-rendered line array, so `render(width)` is called by the host
      // with the REAL terminal width at paint time — `buildWidgetLines`
      // itself stays pure and width-agnostic. `rows` is captured by this
      // closure at THIS paint's freshness; a resize between paints re-renders
      // the same rows at the new width, matching `OverlayChatComponent`'s own
      // width-is-a-render-time-input contract.
      ctx.ui?.setWidget?.(
        AGENT_WIDGET_KEY,
        visible ? (tui, theme) => {
          const component: AgentWidgetComponent = {
            render: (width: number) => {
              if (!batches) return buildWidgetLines(rows, pendingCount, width, ownerActionColor !== undefined, asAgentWidgetTheme(theme), ownerActionColor ?? "error") ?? [];
              const terminalRows = terminalRowsOf(tui) ?? FALLBACK_TERMINAL_ROWS;
              const signature = [terminalRows, width, rowsNow, ...entries.map(entryLayoutKey).sort()].join("\u0000");
              if (layout?.signature !== signature) {
                const budget = computeExpandedBudget(terminalRows, measureDockRows(tui, component, width) ?? ESTIMATED_DOCK_ROWS);
                layout = { signature, budget, allotment: chooseBatchAllotment(entries, rowsNow, budget) };
              }
              return buildExpandedWidgetLines(entries, pendingCount, {
                width, now, allotment: layout.allotment, budget: layout.budget, batches, summaryFor,
                emphasizeAttention: ownerActionColor !== undefined, theme: asAgentWidgetTheme(theme), ownerActionColor: ownerActionColor ?? "error",
              }) ?? [];
            },
            // Rendering is stateless and consults the live Theme object each time.
            invalidate() {},
          };
          return component;
        } : undefined,
        { placement: "belowEditor" },
      );
      ctx.ui?.setStatus?.(AGENT_STATUS_KEY, undefined);
    } catch {
      // 260905 review relay #1 (Important #4): this function is also the bare
      // `setInterval` callback (below) and is called bare from `index.ts`'s
      // `session_start` — a throwing `setWidget`/`setStatus` (e.g. a
      // torn-down TUI surface) must cost one lost repaint, not crash the
      // timer/event loop the way every other call site in this ticket already
      // guards against (`triggerAgentWidgetRefresh`/`refreshAgentWidget`).
    }

    // 261028: a running batch in the expanded mode needs its elapsed field
    // every second; otherwise the normal 10 s clock tick.
    const wantedTickMs = visible ? (fastTick ? AGENT_WIDGET_EXPANDED_TICK_MS : AGENT_WIDGET_TICK_MS) : undefined;
    if (timer && timerMs !== wantedTickMs) {
      clearInterval(timer);
      timer = undefined;
      timerMs = undefined;
    }
    if (wantedTickMs !== undefined && !timer) {
      timer = setInterval(paint, wantedTickMs);
      timerMs = wantedTickMs;
      timer.unref?.();
    }
    if (animate && !attentionTimer) {
      attentionTimer = setInterval(() => {
        attentionPhase = (attentionPhase + 1) % OWNER_ACTION_COLORS.length;
        paint();
      }, AGENT_WIDGET_ATTENTION_TICK_MS);
      attentionTimer.unref?.();
    } else if (!animate) {
      attentionPhase = 0;
      clearAttentionTimer();
    }
  }

  function refresh(): void {
    let read: boolean | Promise<boolean> | undefined;
    try {
      read = options.animationEnabled?.();
    } catch {
      read = undefined;
    }
    if (read instanceof Promise) {
      // Paint now with the last answer; repaint only if the fresh one differs.
      read.then((enabled) => {
        if (stopped || enabled === animationEnabled) return;
        animationEnabled = enabled;
        paint();
      }, () => {});
    } else {
      animationEnabled = read !== false;
    }
    let rowsRead: number | Promise<number> | undefined;
    try {
      rowsRead = options.rowsPerAgent?.();
    } catch {
      rowsRead = undefined;
    }
    if (rowsRead instanceof Promise) {
      rowsRead.then((value) => {
        const resolved = resolveAgentWidgetRows(value);
        if (stopped || resolved === rowsPerAgent) return;
        rowsPerAgent = resolved;
        paint();
      }, () => {});
    } else {
      rowsPerAgent = resolveAgentWidgetRows(rowsRead);
    }
    paint();
  }

  return {
    refresh,
    toggleExpanded() {
      expanded = !expanded;
      layout = undefined;
      refresh();
      return expanded;
    },
    isExpanded: () => expanded,
    stop() {
      stopped = true;
      if (timer) {
        clearInterval(timer);
        timer = undefined;
        timerMs = undefined;
      }
      clearAttentionTimer();
      ctx.ui?.setWidget?.(AGENT_WIDGET_KEY, undefined, { placement: "belowEditor" });
      ctx.ui?.setStatus?.(AGENT_STATUS_KEY, undefined);
    },
  };
}
