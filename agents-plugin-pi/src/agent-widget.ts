/**
 * 260905 (`260905-feat-ws-pi-live-agent-widget`): the live-agent widget — one
 * compact `belowEditor` panel listing every live agent and open owner
 * discussion thread, one row each, plus the footer `setStatus` count segment.
 * Phase 1 of that ticket also folds in the standalone `260904` "N pending
 * question(s)" `aboveEditor` widget (`ask.ts`'s deleted `refreshPendingWidget`)
 * — the pending-question surface is now a row in THIS widget instead of its
 * own panel.
 *
 * Source of truth is the two registries `index.ts` already owns — the RPC
 * agent registry (`spawner.ts`) and the owner-question thread registry
 * (`ask.ts`) — never a separate widget-owned model. `buildAgentRows` is pure
 * and reads both at render time; there is no cached/derived state to keep in
 * sync.
 *
 * Golden rule / placement: this module imports FROM `spawner.ts` and
 * `ask.ts` only (types, plus `ask.ts`'s pure `countPending` helper), never
 * the reverse. `spawner.ts` triggers a re-render through its own
 * `agentWidgetRefreshRef` mutable ref (filled by `index.ts`) precisely so it
 * never has to import this module — see that ref's doc comment. `ask.ts`'s
 * remaining widget call sites go through the same ref, not through this
 * module directly, for the identical reason.
 *
 * `buildAgentRows`/`buildWidgetLines`/`buildStatusSegment` are pure and unit
 * tested directly (`test/agent-widget.test.ts`) with duck-typed fake records
 * and threads, no live `pi` session. `createAgentWidgetController` is the IO
 * glue (`ctx.ui.setWidget`/`setStatus`, the 10-second elapsed timer) and is
 * left to `index.ts`'s own live-gate wiring, the same split `ask.ts` and
 * `spawner.ts` already use between their pure helpers and their `registerX`
 * IO functions.
 */

import { countPending, type ThreadRecord } from "./ask.ts";
import type { RpcAgentRecord, RpcAgentRegistry, SpawnAgentRole } from "./spawner.ts";
import { visibleWidth } from "./overlay-chat.ts";

/** `ctx.ui.setWidget` key for the live-agent panel (`belowEditor`, not a footer/header replacement). */
export const AGENT_WIDGET_KEY = "ws-agents";

/** `ctx.ui.setStatus` key for the footer's agent-count segment. Distinct from `goal-loop.ts`'s `GOAL_LOOP_YIELD_STATUS_KEY` — this widget must never double up that separate segment. */
export const AGENT_STATUS_KEY = "ws-agents-status";

/** Cap on rendered rows before a synthetic `+N more` tail — only `running` rows are ever trimmed; both awaiting states are always shown in full. */
export const AGENT_WIDGET_ROW_CAP = 5;

/** How often the widget repaints its elapsed clocks while it has at least one row. Mirrors `spawner.ts`'s `startLivenessProbe` arm/disarm-a-timer pattern. */
export const AGENT_WIDGET_TICK_MS = 10_000;

/** `buildWidgetLines`'s width bound when the caller supplies none — Pi's extension surface exposes no live terminal-column read, so this is a conservative fixed default rather than a probed value. */
export const DEFAULT_AGENT_WIDGET_WIDTH = 80;

/** One live-agent row's display role. `"thread"` overrides the record's own `spawnRole` label only for a `threadBound` record whose bound thread is `origin: "lead-ask"`. */
export type AgentRowRole = "worker" | "execute" | "fork" | "thread";

/** One live-agent row's state, in display precedence order (`awaiting-owner` first). Idle is deliberately not a state here — an idle, non-`threadBound` record is auto-parked (see `spawner.ts`'s `attachEventListener`) before it would ever read this way. */
export type AgentRowState = "awaiting-owner" | "awaiting-approval" | "running";

/** One rendered row of the live-agent widget. Pure data — no `RpcAgentRecord`/`ThreadRecord` reference — so `buildWidgetLines`/`buildStatusSegment` need no registry access of their own. */
export interface AgentRow {
  /** `alias > title > shortened uuid` (mirrors `ask.ts:351`'s short-uuid convention). */
  name: string;
  role: AgentRowRole;
  state: AgentRowState;
  /** Milliseconds since the clock this row's state uses — `ThreadRecord.touchedAt` for a `"thread"` row, `RpcAgentRecord.runStartedAt` otherwise. Never negative. */
  elapsedMs: number;
  /** The `/answer <id>` hint text, set only for a `"thread"` row (the ticket's merged-in owner-question cue). */
  answerHint?: string;
}

const STATE_RANK: Record<AgentRowState, number> = {
  "awaiting-owner": 0,
  "awaiting-approval": 1,
  running: 2,
};

const STATE_LABEL: Record<AgentRowState, string> = {
  "awaiting-owner": "awaiting owner",
  "awaiting-approval": "awaiting approval",
  running: "running",
};

/** `worker -> "worker"`, `execute-worker -> "execute"`, `fork -> "fork"`; an unset `spawnRole` (should not happen post-spawn, but never throw) falls back to `"worker"`. */
function roleFromSpawnRole(spawnRole: SpawnAgentRole | undefined): AgentRowRole {
  if (spawnRole === "execute-worker") return "execute";
  if (spawnRole === "fork") return "fork";
  return "worker";
}

/** `alias > title > shortened uuid` — the ticket's name-precedence rule, mirroring `ask.ts:351`'s `agentId.slice(0, 8)` convention. */
function rowName(record: RpcAgentRecord): string {
  return record.alias ?? record.title ?? record.agentId.slice(0, 8);
}

/**
 * Pure row builder: one row per registry member, no widget-owned state.
 *
 * Row inclusion (a record the widget cares about at all):
 * `record.threadBound || record.pendingApproval !== undefined || record.client
 * !== undefined`. A plain, non-`threadBound` idle record never satisfies any
 * of these — the automatic-park step in `spawner.ts`'s `attachEventListener`
 * has already cleared `client` by the time it would otherwise read that way
 * — which is what makes "idle is not a row state" true without this function
 * needing to check `streaming`/`running` itself. A `threadBound` record
 * renders even while dormant (`client === undefined`): that row is the
 * owner's action cue, and it must not disappear just because the respondent
 * fork happens to be parked between messages.
 *
 * State precedence: `threadBound` (awaiting owner) beats `pendingApproval`
 * (awaiting approval) beats the default `"running"`.
 *
 * Role: a `threadBound` record renders as `"thread"` only when a thread in
 * `threads` has `respondentAgentId === record.agentId` AND `origin ===
 * "lead-ask"` — the ticket's Entry-B-only override. A `threadBound`
 * fork-raised (Entry A) respondent, or any non-`threadBound` record, keeps
 * its own role from `record.spawnRole`.
 *
 * Elapsed: a `"thread"` row uses `now - Date.parse(thread.touchedAt)`; every
 * other row uses `now - (record.runStartedAt ?? now)` (0 when the record has
 * never been prompted, which should not occur for an included record but
 * must never go negative or throw).
 *
 * Sort: state rank first (awaiting owner, then awaiting approval, then
 * running), elapsed descending within each state. No cap here — `N` for the
 * footer's `setStatus` segment is this deduped, UNCAPPED row count (the
 * ticket's "the setStatus count is the deduped row count"); the display cap
 * to `AGENT_WIDGET_ROW_CAP` with its `+N more` tail is `buildWidgetLines`'s
 * own rendering concern, not a property of the underlying agent count.
 */
export function buildAgentRows(records: RpcAgentRegistry, threads: readonly ThreadRecord[], now: number): AgentRow[] {
  const rows: AgentRow[] = [];

  for (const record of records.values()) {
    const included = record.threadBound === true || record.pendingApproval !== undefined || record.client !== undefined;
    if (!included) continue;

    const boundThread = record.threadBound
      ? threads.find((t) => t.respondentAgentId === record.agentId && t.origin === "lead-ask")
      : undefined;
    const isThreadRow = boundThread !== undefined;

    const state: AgentRowState = record.threadBound ? "awaiting-owner" : record.pendingApproval !== undefined ? "awaiting-approval" : "running";

    const elapsedMs = isThreadRow ? Math.max(0, now - Date.parse(boundThread!.touchedAt)) : Math.max(0, now - (record.runStartedAt ?? now));

    rows.push({
      name: rowName(record),
      role: isThreadRow ? "thread" : roleFromSpawnRole(record.spawnRole),
      state,
      elapsedMs,
      answerHint: isThreadRow ? `/answer ${boundThread!.threadId}` : undefined,
    });
  }

  rows.sort((a, b) => {
    const rankDiff = STATE_RANK[a.state] - STATE_RANK[b.state];
    return rankDiff !== 0 ? rankDiff : b.elapsedMs - a.elapsedMs;
  });

  return rows;
}

/** `Xs` under a minute, `Xm` under an hour, else `XhYYm` — a compact, always-non-negative elapsed label. */
function formatElapsed(elapsedMs: number): string {
  const totalSeconds = Math.max(0, Math.floor(elapsedMs / 1000));
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  if (totalMinutes < 60) return `${totalMinutes}m`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  return `${hours}h${String(minutes).padStart(2, "0")}m`;
}

/** `name · role · state · elapsed`, plus the `/answer <id>` hint for a `"thread"` row — the ticket's literal row shape. */
function formatRow(row: AgentRow): string {
  const base = `${row.name} · ${row.role} · ${STATE_LABEL[row.state]} · ${formatElapsed(row.elapsedMs)}`;
  return row.answerHint ? `${base} — ${row.answerHint}` : base;
}

/**
 * Truncates `text` to at most `width` display columns (`visibleWidth`,
 * reused from `overlay-chat.ts` rather than duplicated), appending a single
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
 * Renders `rows` (as produced by `buildAgentRows`, already sorted) into the
 * widget's display lines, applying the ticket's cap: every awaiting-state row
 * is kept, `running` rows are trimmed so the total is `AGENT_WIDGET_ROW_CAP`
 * with a synthetic `+N more` trailing line — only `running` rows are ever
 * folded into that tail. Every line is bounded to `width` display columns via
 * `truncateToWidth`. `undefined` when `rows` is empty — the widget's
 * hide-on-empty behavior — regardless of `width`.
 */
export function buildWidgetLines(rows: readonly AgentRow[], width: number = DEFAULT_AGENT_WIDGET_WIDTH): string[] | undefined {
  if (rows.length === 0) return undefined;

  const awaiting = rows.filter((row) => row.state !== "running");
  const running = rows.filter((row) => row.state === "running");

  let shown: readonly AgentRow[];
  let hiddenRunning = 0;
  if (awaiting.length + running.length <= AGENT_WIDGET_ROW_CAP) {
    shown = rows;
  } else {
    const runningSlots = Math.max(0, AGENT_WIDGET_ROW_CAP - awaiting.length);
    shown = [...awaiting, ...running.slice(0, runningSlots)];
    hiddenRunning = running.length - runningSlots;
  }

  const lines = shown.map((row) => truncateToWidth(formatRow(row), width));
  if (hiddenRunning > 0) lines.push(truncateToWidth(`+${hiddenRunning} more`, width));
  return lines;
}

/**
 * The footer `setStatus` segment: `ws: N agents` (`N = rows.length`, the
 * deduped row count `buildAgentRows` already produced — see that function's
 * doc comment for why this is uncapped) plus ` · M question(s)` only while
 * `pendingCount > 0`. `undefined` when there is nothing to show at all —
 * `rows.length === 0` and `pendingCount <= 0` — which clears the segment
 * (`ctx.ui.setStatus(key, undefined)`).
 */
export function buildStatusSegment(rows: readonly AgentRow[], pendingCount: number): string | undefined {
  if (rows.length === 0 && pendingCount <= 0) return undefined;
  const questionPart = pendingCount > 0 ? ` · ${pendingCount} question${pendingCount === 1 ? "" : "s"}` : "";
  return `ws: ${rows.length} agents${questionPart}`;
}

// ---------------------------------------------------------------------------
// IO glue: the setWidget/setStatus repaint plus the arm-while-non-empty
// elapsed timer. Not unit tested here — see this file's header comment.
// ---------------------------------------------------------------------------

/** Minimal duck-typed `ctx` surface this controller needs — the same convention `ask.ts`'s `AskUiCtx` uses. */
export interface AgentWidgetUiCtx {
  ui?: {
    setWidget?(key: string, content: string[] | undefined, options?: { placement?: string }): void;
    setStatus?(key: string, text: string | undefined): void;
  };
}

export interface AgentWidgetController {
  /** Recomputes rows from the live registries and repaints the widget + status segment. Arms the elapsed timer when rows just became non-empty, disarms it when they just became empty. */
  refresh(): void;
  /** Disarms the timer and clears both the widget and the status segment. Call once, from `session_shutdown`. */
  stop(): void;
}

/**
 * Builds the IO controller `index.ts` wires into `spawner.ts`'s
 * `agentWidgetRefreshRef` (and calls directly from its own `session_start`/
 * `session_shutdown`). `registry`/`threads` are read fresh on every
 * `refresh()` call — no cached row state — so a caller may safely hold this
 * controller for the whole session lifetime.
 *
 * The 10-second timer (`AGENT_WIDGET_TICK_MS`) is armed only while the most
 * recently computed row set is non-empty, mirroring `spawner.ts`'s
 * `startLivenessProbe` arm/disarm-a-timer-only-while-outstanding pattern —
 * an idle lead that has never spawned anything, or one whose registry has
 * gone fully quiet, pays nothing for elapsed-clock upkeep.
 */
export function createAgentWidgetController(ctx: AgentWidgetUiCtx, registry: RpcAgentRegistry, threads: Map<string, ThreadRecord>): AgentWidgetController {
  let timer: ReturnType<typeof setInterval> | undefined;

  function paint(): void {
    const threadList = [...threads.values()];
    const rows = buildAgentRows(registry, threadList, Date.now());
    ctx.ui?.setWidget?.(AGENT_WIDGET_KEY, buildWidgetLines(rows), { placement: "belowEditor" });
    ctx.ui?.setStatus?.(AGENT_STATUS_KEY, buildStatusSegment(rows, countPending(threadList)));

    if (rows.length > 0 && !timer) {
      timer = setInterval(paint, AGENT_WIDGET_TICK_MS);
      timer.unref?.();
    } else if (rows.length === 0 && timer) {
      clearInterval(timer);
      timer = undefined;
    }
  }

  return {
    refresh: paint,
    stop() {
      if (timer) {
        clearInterval(timer);
        timer = undefined;
      }
      ctx.ui?.setWidget?.(AGENT_WIDGET_KEY, undefined, { placement: "belowEditor" });
      ctx.ui?.setStatus?.(AGENT_STATUS_KEY, undefined);
    },
  };
}
