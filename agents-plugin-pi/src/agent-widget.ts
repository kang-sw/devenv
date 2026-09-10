/**
 * 260905 (`260905-feat-ws-pi-live-agent-widget`): the live-agent widget — one
 * compact `belowEditor` panel listing every live agent and open owner
 * discussion thread, one row each, headed by the uncapped count segment.
 * Phase 1 of that ticket also folds in the standalone `260904` "N pending
 * question(s)" `aboveEditor` widget (`ask.ts`'s deleted `refreshPendingWidget`)
 * — the pending-question surface is now a row in THIS widget instead of its
 * own panel.
 *
 * Source of truth is the two registries `index.ts` already owns — the RPC
 * agent registry (`spawner.ts`) and the owner-question thread registry
 * (`ask.ts`) — never a separate widget-owned model. `buildAgentRows` is pure
 * and reads the UNION of both at render time (not the RPC registry alone —
 * review relay #1's Critical fix: a pending `ws-ask` thread has no
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
 * `buildAgentRows`/`buildWidgetLines`/`buildHeadingLine` are pure and unit
 * tested directly (`test/agent-widget.test.ts`) with duck-typed fake records
 * and threads, no live `pi` session. `createAgentWidgetController` is the IO
 * glue (`ctx.ui.setWidget`/`setStatus`, the 10-second elapsed timer) and is
 * left to `index.ts`'s own live-gate wiring, the same split `ask.ts` and
 * `spawner.ts` already use between their pure helpers and their `registerX`
 * IO functions.
 */

import { countPending, type ThreadRecord } from "./ask.ts";
import type { RpcAgentRecord, RpcAgentRegistry, SpawnAgentRole } from "./spawner.ts";
import { visibleWidth } from "./text-width.ts";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";

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
export type AgentRowRole = "worker" | "execute" | "fork" | "thread" | "explore";

/** One live-agent row's state, in display precedence order (`awaiting-owner` first). Idle is deliberately not a state here — an idle, non-`threadBound` record is auto-parked (see `spawner.ts`'s `attachEventListener`) before it would ever read this way. */
export type AgentRowState = "awaiting-owner" | "idle-awaiting-owner" | "awaiting-approval" | "running";

/** One rendered row of the live-agent widget. Pure data — no `RpcAgentRecord`/`ThreadRecord` reference — so `buildWidgetLines`/`buildHeadingLine` need no registry access of their own. */
export interface AgentRow {
  /** `alias > title > shortened uuid` (mirrors `ask.ts:351`'s short-uuid convention). */
  name: string;
  role: AgentRowRole;
  state: AgentRowState;
  /** Milliseconds since the clock this row's state uses — `ThreadRecord.touchedAt` for a `"thread"` row, `RpcAgentRecord.runStartedAt` otherwise. Never negative. */
  elapsedMs: number;
  /** The `/answer <id>` hint text, set only for a `"thread"` row (the ticket's merged-in owner-question cue). */
  answerHint?: string;
  /** Human-readable question phrase. It is display-only and never a resolution key. */
  answerDisplay?: string;
  /** A supplied owner-held inspection affordance. Presentation preserves it but never invents one. */
  inspectionHint?: string;
  model?: string;
  effort?: string;
  latestInput?: number;
  estimatedUsd?: number;
}

const BOLD = "\u001b[1m";
const RESET = "\u001b[22m";

function bold(text: string, enabled: boolean): string {
  return enabled ? `${BOLD}${text}${RESET}` : text;
}

/** Removes terminal/control input before it reaches a TUI row. */
function sanitizeDisplayTitle(title: string | undefined, fallback: string): string {
  const cleaned = title?.replace(/[\u0000-\u001f\u007f-\u009f]/g, "").trim();
  return cleaned || fallback;
}

const STATE_RANK: Record<AgentRowState, number> = {
  "awaiting-owner": 0,
  "idle-awaiting-owner": 0,
  "awaiting-approval": 1,
  running: 2,
};

const STATE_LABEL: Record<AgentRowState, string> = {
  "awaiting-owner": "awaiting owner",
  "idle-awaiting-owner": "idle awaiting owner",
  "awaiting-approval": "awaiting approval",
  running: "running",
};

/** `worker -> "worker"`, `execute-worker -> "execute"`, `fork -> "fork"`, `explore -> "explore"` (260906); an unset `spawnRole` (should not happen post-spawn, but never throw) falls back to `"worker"`. */
function roleFromSpawnRole(spawnRole: SpawnAgentRole | undefined): AgentRowRole {
  if (spawnRole === "execute-worker") return "execute";
  if (spawnRole === "fork") return "fork";
  if (spawnRole === "explore") return "explore";
  return "worker";
}

/** `alias > title > shortened uuid` — the ticket's name-precedence rule, mirroring `ask.ts:351`'s `agentId.slice(0, 8)` convention. Exported (260908 audit-window ticket) so the audit picker reuses the identical naming rule verbatim. */
export function rowName(record: RpcAgentRecord): string {
  return record.alias ?? record.title ?? record.agentId.slice(0, 8);
}

/**
 * 260908 (subagent audit window ticket): the row-inclusion/state
 * classification half of `buildAgentRows`'s per-record loop below, pulled
 * out as its own pure predicate so the audit picker's three live tiers
 * (`260908` sibling ticket) reuse the identical inclusion rule and state
 * precedence rather than a second copy. `undefined` means "not included by
 * the widget" — i.e. the record is dormant (`client === undefined &&
 * !threadBound && pendingApproval === undefined`), exactly the complement
 * `buildAgentRows`'s own doc comment already describes. Pure refactor:
 * `buildAgentRows`'s own output is unchanged.
 */
export function classifyRegistryRowState(record: RpcAgentRecord): AgentRowState | undefined {
  if (record.threadBound === true) return "awaiting-owner";
  if (record.pendingApproval !== undefined) return "awaiting-approval";
  if (record.client !== undefined) return "running";
  return undefined;
}

/** A thread the widget still owes the owner an answer on — the two `countPending`-adjacent statuses that ever yield a row. Dormant/closed threads never do. */
function isLiveThreadStatus(status: ThreadRecord["status"]): boolean {
  return status === "pending" || status === "open";
}

/** `Math.max(0, deltaMs)`, but a malformed/missing clock (`Date.parse` of a bad `touchedAt`, review relay #1 Minor) collapses to 0 instead of propagating `NaN` through `formatElapsed`. */
function clampElapsed(deltaMs: number): number {
  return Number.isFinite(deltaMs) ? Math.max(0, deltaMs) : 0;
}

/**
 * Pure row builder over the UNION of the two registries — the RPC agent
 * registry and the owner-question thread registry — never the RPC registry
 * alone (review relay #1 Critical: a pending `ws-ask` thread has no
 * `respondentAgentId` until `/answer` lazily spawns its fork, and a
 * fork-raised thread's respondent can come back from a lead restart revived
 * dormant with `threadBound` unset (`agent-sidecar.ts`'s `reviveOrphans`) —
 * both cases used to render zero rows and silently drop the merged-in
 * pending-question surface entirely).
 *
 * Row inclusion, RPC-registry side (a record the widget cares about even
 * with no matching thread): `record.threadBound || record.pendingApproval
 * !== undefined || record.client !== undefined`. A plain, non-`threadBound`
 * idle record never satisfies any of these — the automatic-park step in
 * `spawner.ts`'s `attachEventListener` has already cleared `client` by the
 * time it would otherwise read that way — which is what makes "idle is not a
 * row state" true without this function needing to check `streaming`/
 * `running` itself. A `threadBound` record renders even while dormant
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
 * State precedence: `threadBound` (awaiting owner) beats `pendingApproval`
 * (awaiting approval) beats the default `"running"`.
 *
 * Hint/clock vs. role (review relay #1 Important #2): a `threadBound`
 * record's `/answer <id>` hint and `touchedAt`-based elapsed follow the
 * `awaiting-owner` STATE and apply whenever a matching live thread is found,
 * regardless of `origin` — a fork-raised (Entry A) respondent owes the owner
 * an answer exactly as much as a lead-ask (Entry B) one does. Only the ROLE
 * LABEL stays origin-dependent: `"thread"` renders only for a `lead-ask`
 * match (the ticket's Entry-B-only role override); a fork-raised match keeps
 * the record's own `spawnRole` label (typically `"fork"`).
 *
 * Sort: state rank first (awaiting owner, then awaiting approval, then
 * running), elapsed descending within each state. No cap here — `N` for the
 * panel heading is this deduped, UNCAPPED row count; the display cap
 * to `AGENT_WIDGET_ROW_CAP` with its `+N more` tail is `buildWidgetLines`'s
 * own rendering concern, not a property of the underlying agent count.
 */
export function buildAgentRows(records: RpcAgentRegistry, threads: readonly ThreadRecord[], now: number): AgentRow[] {
  const rows: AgentRow[] = [];
  const coveredThreadIds = new Set<string>();

  for (const record of records.values()) {
    const state = classifyRegistryRowState(record);
    if (state === undefined) continue;

    const boundThread = record.threadBound
      ? threads.find((t) => t.respondentAgentId === record.agentId && isLiveThreadStatus(t.status))
      : undefined;
    if (boundThread) coveredThreadIds.add(boundThread.threadId);

    const isAwaitingOwnerWithThread = state === "awaiting-owner" && boundThread !== undefined;

    const elapsedMs = isAwaitingOwnerWithThread ? clampElapsed(now - Date.parse(boundThread!.touchedAt)) : clampElapsed(now - (record.runStartedAt ?? now));

    rows.push({
      name: rowName(record),
      role: isAwaitingOwnerWithThread && boundThread!.origin === "lead-ask" ? "thread" : roleFromSpawnRole(record.spawnRole),
      state,
      elapsedMs,
      ...(isAwaitingOwnerWithThread ? {
        answerHint: `/answer ${boundThread!.threadId}`,
        answerDisplay: sanitizeDisplayTitle(boundThread!.title, boundThread!.threadId),
      } : {}),
      ...(record.telemetry?.model ?? record.observedModel ? { model: record.telemetry?.model ?? record.observedModel } : {}),
      ...(record.telemetry?.effort ?? record.observedEffort ? { effort: record.telemetry?.effort ?? record.observedEffort } : {}),
      ...((record.telemetry?.latestInput ?? record.observedLatestInput) !== undefined ? { latestInput: record.telemetry?.latestInput ?? record.observedLatestInput } : {}),
      ...(record.telemetry?.estimatedUsd !== undefined ? { estimatedUsd: record.telemetry.estimatedUsd } : {}),
    });
  }

  for (const thread of threads) {
    if (!isLiveThreadStatus(thread.status) || coveredThreadIds.has(thread.threadId)) continue;
    rows.push({
      name: thread.title,
      role: "thread",
      state: "awaiting-owner",
      elapsedMs: clampElapsed(now - Date.parse(thread.touchedAt)),
      answerHint: `/answer ${thread.threadId}`,
      answerDisplay: sanitizeDisplayTitle(thread.title, thread.threadId),
      ...(thread.forkResume?.telemetry?.model ?? thread.forkResume?.observedModel ? { model: thread.forkResume?.telemetry?.model ?? thread.forkResume?.observedModel } : {}),
      ...(thread.forkResume?.telemetry?.effort ?? thread.forkResume?.observedEffort ? { effort: thread.forkResume?.telemetry?.effort ?? thread.forkResume?.observedEffort } : {}),
      ...((thread.forkResume?.telemetry?.latestInput ?? thread.forkResume?.observedLatestInput) !== undefined ? { latestInput: thread.forkResume?.telemetry?.latestInput ?? thread.forkResume?.observedLatestInput } : {}),
      ...(thread.forkResume?.telemetry?.estimatedUsd !== undefined ? { estimatedUsd: thread.forkResume.telemetry.estimatedUsd } : {}),
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
function isAttentionState(state: AgentRowState): boolean {
  return state === "awaiting-owner" || state === "idle-awaiting-owner" || state === "awaiting-approval";
}

function formatInputTokens(tokens: number | undefined): string {
  return tokens === undefined ? "—" : `${(tokens / 1_000).toFixed(1)}k`;
}

function formatEstimatedUsd(usd: number | undefined): string {
  if (usd === undefined) return "—";
  return String(Number(usd.toFixed(3)));
}

function formatRow(row: AgentRow, width = DEFAULT_AGENT_WIDGET_WIDTH, emphasizeAttention = false): string {
  const primary = row.answerHint ? `/answer ${row.answerDisplay ?? row.name}` : row.name;
  const stateLabel = STATE_LABEL[row.state];
  const base = `${primary} · ${row.role} · ${stateLabel} · ${formatElapsed(row.elapsedMs)}`;
  const selection = `${row.model ?? "—"} (${row.effort ?? "—"})`;
  const telemetry = ` · ${selection} · in ${formatInputTokens(row.latestInput)} · est $${formatEstimatedUsd(row.estimatedUsd)}`;
  const protectedHint = row.answerHint ?? row.inspectionHint;
  const hint = protectedHint ? ` — ${protectedHint}` : "";
  // The owner action is the only non-negotiable tail.  Allocate its columns
  // first, then progressively omit telemetry and identity detail.
  let line: string;
  let appendedHint = false;
  if (protectedHint && visibleWidth(hint) <= width) {
    const available = width - visibleWidth(hint);
    const withTelemetry = base + telemetry;
    line = visibleWidth(withTelemetry) <= available ? withTelemetry + hint : truncateToWidth(base, available) + hint;
    appendedHint = true;
  } else {
    line = visibleWidth(base + telemetry) <= width ? base + telemetry : truncateToWidth(base, width);
  }
  // Add ANSI only after width truncation: styling before truncation can leave
  // an incomplete escape sequence in a narrow terminal.
  if (!emphasizeAttention || !isAttentionState(row.state)) return line;
  // A protected hint that does not fit was never appended. Track that fact
  // rather than inferring it from row metadata, or narrow styling would
  // reconstruct an over-width tail after the bounded line was built.
  const content = appendedHint ? line.slice(0, -hint.length) : line;
  const suffix = appendedHint ? hint : "";
  if (content.length === 0) return line;
  if (row.answerHint) {
    // The question cue is the first structured field. Its visible prefix is
    // the only styled part even if width truncation removes later fields.
    const cueEnd = Math.min(content.length, primary.length);
    return bold(content.slice(0, cueEnd), true) + content.slice(cueEnd) + suffix;
  }
  // State begins after fixed, structured name and role fields. Never search
  // rendered text: names may contain the state label or separator glyphs.
  const stateStart = primary.length + 3 + row.role.length + 3;
  const stateEnd = stateStart + stateLabel.length;
  if (content.length < stateEnd) return line;
  return content.slice(0, stateStart) + bold(stateLabel, true) + content.slice(stateEnd) + suffix;
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
 * The panel heading: `ws: N agents` (`N = rows.length`, the deduped row count
 * `buildAgentRows` already produced) plus ` · M question(s)` only while
 * `pendingCount > 0`. It is shown whenever rows or pending questions exist.
 */
export function buildHeadingLine(rows: readonly AgentRow[], pendingCount: number, width: number = DEFAULT_AGENT_WIDGET_WIDTH, emphasizeAttention = false): string | undefined {
  if (rows.length === 0 && pendingCount <= 0) return undefined;
  const questionPart = pendingCount > 0 ? ` · ${pendingCount} question${pendingCount === 1 ? "" : "s"}` : "";
  const heading = `ws: ${rows.length} agents${questionPart}`;
  return bold(truncateToWidth(heading, width), emphasizeAttention);
}

/**
 * Renders the panel heading followed by `rows` (as produced by
 * `buildAgentRows`, already sorted). The heading does not consume the
 * ticket's five-row cap: every awaiting-state row is kept, `running` rows are
 * trimmed so the body has `AGENT_WIDGET_ROW_CAP` rows with a synthetic `+N
 * more` trailing line. Every line is bounded to `width` display columns via
 * `truncateToWidth`. `undefined` only when rows and pending questions are
 * both absent.
 */
export function buildWidgetLines(rows: readonly AgentRow[], pendingCount: number, width: number = DEFAULT_AGENT_WIDGET_WIDTH, emphasizeAttention = false): string[] | undefined {
  const heading = buildHeadingLine(rows, pendingCount, width, emphasizeAttention);
  if (heading === undefined) return undefined;

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

  const lines = [heading, ...shown.map((row) => formatRow(row, width, emphasizeAttention && isAttentionState(row.state)))];
  if (hiddenRunning > 0) lines.push(truncateToWidth(`+${hiddenRunning} more`, width));
  return lines;
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
  /** Disarms the timer, clears the widget, and clears the retired status segment. Call once, from `session_shutdown`. */
  stop(): void;
}

export interface AgentWidgetControllerOptions {
  /** Only the host lead owns the attention timer; forks retain the ordinary panel. */
  ownerLead?: boolean;
  /** Read the adapter-local config afresh at each refresh. */
  animationEnabled?: () => boolean;
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
  let attentionPhase = true;

  function clearAttentionTimer(): void {
    if (attentionTimer) {
      clearInterval(attentionTimer);
      attentionTimer = undefined;
    }
  }

  function paint(): void {
    const threadList = [...threads.values()];
    const rows = buildAgentRows(registry, threadList, Date.now());
    const pendingCount = countPending(threadList);
    const visible = rows.length > 0 || pendingCount > 0;
    const qualifying = rows.some((row) => isAttentionState(row.state));
    const animationEnabled = options.animationEnabled?.() !== false;
    const animate = options.ownerLead === true && animationEnabled && qualifying;
    const emphasize = qualifying && (!animationEnabled || (animate && attentionPhase));
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
        visible ? () => ({ render: (width: number) => buildWidgetLines(rows, pendingCount, width, emphasize) ?? [] }) : undefined,
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

    if (visible && !timer) {
      timer = setInterval(paint, AGENT_WIDGET_TICK_MS);
      timer.unref?.();
    } else if (!visible && timer) {
      clearInterval(timer);
      timer = undefined;
    }
    if (animate && !attentionTimer) {
      attentionTimer = setInterval(() => {
        attentionPhase = !attentionPhase;
        paint();
      }, AGENT_WIDGET_ATTENTION_TICK_MS);
      attentionTimer.unref?.();
    } else if (!animate) {
      attentionPhase = true;
      clearAttentionTimer();
    }
  }

  return {
    refresh: paint,
    stop() {
      if (timer) {
        clearInterval(timer);
        timer = undefined;
      }
      clearAttentionTimer();
      ctx.ui?.setWidget?.(AGENT_WIDGET_KEY, undefined, { placement: "belowEditor" });
      ctx.ui?.setStatus?.(AGENT_STATUS_KEY, undefined);
    },
  };
}
