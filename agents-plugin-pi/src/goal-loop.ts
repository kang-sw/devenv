/**
 * Goal-mode arming + `agent_settled` re-injection loop (260903 Phase 1).
 *
 * Design: a user-invoked idle `/goal <goal>` command arms an in-memory state
 * machine and announces the goal via `pi.sendUserMessage`; during active work
 * the same validated command enters the shared held-input FIFO as control
 * state, applies at the next safe boundary, and never becomes model prose.
 * The exact `/goal stop|clear|reset` aliases disarm future automatic
 * continuation immediately without interrupting current work. While armed, every
 * `agent_settled` event (fired after a run has fully settled with no
 * automatic retry/compaction/continuation queued — see
 * `AgentSettledEvent`'s doc comment in the installed Pi type defs) re-injects
 * a reminder naming the goal and its levers (two terminal, one non-terminal —
 * see Phase 2 below), UNLESS a runaway backstop trips first. A settle outside
 * goal mode is an ordinary stop — the handler no-ops.
 *
 * Two terminal levers end the run: `goal-achieved` and `goal-blocked`, both
 * implemented as `pi.registerTool()` calls (model-invoked function calls),
 * NOT `pi.registerCommand()`s. This is a deliberate elimination, not a
 * default: the ticket's design section requires "explicit skill calls, zero
 * prose parsing" — state transitions triggered only by a model-invoked call,
 * never by parsing the model's generated text. Pi's `/skill:name` expansion
 * and command dispatch are wired only into the *input* pipeline (typed user
 * input, or text explicitly sent via `sendUserMessage`/`sendMessage` with
 * `expandPromptTemplates: true`) — never applied to the model's own
 * assistant-generated output — so a `registerCommand` cannot serve as a
 * model-invoked lever. `registerTool` is the only primitive the model can
 * invoke directly as a function call, matching the existing
 * `ws-report-to-lead` precedent (spawner.ts) of a plain, non-bridged, custom
 * tool. `/goal` itself stays a `registerCommand` because it is a
 * user-invoked entry, matching the ticket's own "goal-entry **command**"
 * wording.
 *
 * Runaway backstop: N consecutive re-fires with no intervening tool call
 * force-stop the loop (disarm goal mode) — Pi has no session-kill primitive
 * that fits here (`ctx.shutdown()` exits the whole process). The threshold
 * is an adapter-declared ws setting (`pi.runaway_threshold`, see
 * adapter-config.ts), read through ws-mcp's `config.get` at each use and
 * defaulted when absent or unreachable. This reverses the earlier rule that
 * goal-loop knobs never live in ws-mcp config: ws-mcp now stores and
 * validates them from the adapter's own manifest, while the goal-loop
 * behavior itself stays entirely adapter-local.
 *
 * Settled cross-ticket fact: the goal-loop runs on the lead session only.
 * The `agent_settled` handler no-ops when the running process is itself a
 * spawned child (any `WS_PI_SPAWN_ROLE_ENV` role set — see
 * `process-role.ts`'s `readSpawnRole`, and spawner.ts's
 * `buildRpcClientOptions`, which carries that
 * marker on every spawned child) — defense-in-depth against a message that
 * happens to start with `/goal …` reaching a child's input pipeline (e.g. a
 * lead-authored `ws-agent-send` message), even though each spawned child
 * loads this same extension fresh with its own inert module-level state.
 * 260904 Phase 1: this marker is now a role value (`worker`/`explore`/
 * `fork`), not the old boolean `WS_PI_AGENT_CHILD_ENV`; `isChildProcess`
 * treats presence of ANY role as "child" — conservatively including a future
 * `fork`, until the not-yet-landed side-thread-fork ticket decides
 * otherwise.
 *
 * Following the bridge.ts/spawner.ts convention, this one file mixes pure,
 * unit-tested state-machine/config-reader functions with the `registerGoalLoop`
 * IO glue because its command, tools, and lifecycle listeners are closer in
 * shape to spawner.ts.
 *
 * 261002 (ws-owned lead compaction) folds 260903's `goal-compact-and-continue`
 * into one lead compaction lever, `ws-compact` (`LEAD_COMPACT_TOOL_NAME`): the
 * lead fills its prose under fixed headings (lead-compaction.ts), the lever
 * calls `ctx.compact()`, and the lead session's `session_before_compact`
 * handler returns `{ compaction }` built from adapter-filled deterministic
 * sections plus that prose, replacing Pi's native summarizer for the lead.
 * The lever works with or without an active goal; only while a goal is active
 * does it re-arm the goal loop (it never calls `disarmGoal()`). With no goal,
 * a lever call that cut work short (autonomous, or after the hard cut) is
 * followed by one resume message from the lever's completion callback, once
 * an idle release has flushed held pushes (261003); the lead can also request
 * that resume explicitly with `continue_after_compact: true` (261004), which
 * ORs with the route-based eligibility. Spawned
 * worker/explore/fork sessions keep Pi's native compaction. The lead is led
 * to the lever by a preparation message carrying `lead-compact-guide.md`: an
 * advisory nudge at `agent_end`, a hard-cut steer at `turn_end`, or a user
 * `/compact`, which is cancelled and rerouted; two guide-less milestone wake
 * turns at `agent_end` (261007) sit between the advisory and the hard cut. A threshold or overflow
 * compaction that arrives without lever prose gets an in-hook fallback
 * summary from the session model. The reinject
 * reminder still surfaces `ctx.getContextUsage().percent` against the
 * compaction-advisory percent; that knob, the context-window override, and
 * the new compaction knobs are adapter-declared ws settings alongside
 * `runaway_threshold`.
 */

import { randomUUID } from "node:crypto";
import { convertToLlm, serializeConversation, sessionEntryToContextMessages, type CompactionResult, type ContextUsage, type ExtensionAPI, type ExtensionContext, type SessionBeforeCompactEvent } from "@earendil-works/pi-coding-agent";
import {
  buildFallbackSummaryPrompt,
  buildLeadCompactionSummary,
  buildContextMilestoneMessage,
  buildPreparationMessage,
  DEFAULT_DIALOG_BUDGET_BYTES,
  DEFAULT_REREAD_BUDGET_BYTES,
  extractLeadProse,
  FALLBACK_SYSTEM_PROMPT,
  GOAL_ANNOUNCEMENT_PREFIX,
  GOAL_REMINDER_MARKER_PREFIX,
  isLeadCompactionDetails,
  LEAD_COMPACT_CUSTOM_TYPE,
  LEAD_COMPACT_TOOL_NAME,
  LEAD_COMPACTION_DETAILS_KIND,
  LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE,
  leadProseParameterSchema,
  NO_KEPT_ENTRY_ID,
  normalizeFileList,
  readLeadCompactGuide,
  renderLeadProse,
  type LeadCompactionDetails,
  type LeadFileLists,
  type LeadProse,
  type PreparationTrigger,
} from "./lead-compaction.ts";
import { labelAdapterText } from "./adapter-label.ts";
import { readSpawnRole } from "./process-role.ts";
import { staticConfigReader, thenOrNow, type GoalLoopConfigKey, type GoalLoopConfigReader } from "./adapter-config.ts";
import { createToolPreviewTuiRef, registerWsTool, type ToolPreviewTuiRef } from "./tool-result-render.ts";
import { clearWakeStart, enqueueHeldGoalReplacement, reserveWakeStart, heldPushQueue, flushHeldPushes, hasRunningAgents, isOwningAgentIdle, leadCompactingRef, reserveAdapterPromptStart, leadWakeStartPendingRef, type HeldGoalReplacementResult, type RpcAgentRegistry } from "./spawner.ts";

// ---------------------------------------------------------------------------
// Config: adapter-declared ws settings (adapter-config.ts). Every knob is
// optional; each resolver below maps an absent or malformed value to its
// default and never hard-fails.
// ---------------------------------------------------------------------------

export interface GoalLoopConfig {
  /** Whether an owner lead animates actionable child waits in the live-agent widget. Only literal false disables the 330ms cue. */
  agent_wait_animation?: boolean;
  runaway_threshold?: number;
  /** Advisory context-usage percent (0, 100]: surfaced in the reinject reminder, and (261002) the point where the lead is nudged once to prepare for compaction. */
  compaction_advisory_percent?: number;
  /** Hard context-usage percent (0, 100] where the lead is steered into compaction preparation at the next tool-call boundary (261002). */
  compaction_hard_percent?: number;
  /** Optional context-window token override for `computeContextPercent`, used when the model's own `getContextUsage().contextWindow` should be superseded. */
  context_window_override?: number;
  /**
   * Delay in milliseconds, after an `agent_settled` fires (goal active, lead
   * process, not compacting), before the settle timer re-evaluates the loop
   * and — if the fire condition still holds — sends the reinject reminder
   * (260906 Phase 1, settle-timer reminder race ticket). Read fresh on every
   * arm, mirroring `runaway_threshold`'s never-hard-fail shape.
   */
  settle_delay_ms?: number;
  /** Age-based child-home retention in days; 0 disables age pruning. */
  child_retention_ttl_days?: number;
  /** UTF-8 byte budget for the lead compaction summary's `## Dialog` section (261003). */
  compaction_dialog_budget_bytes?: number;
  /** UTF-8 byte budget for the lead compaction summary's inlined required re-reads (261007). */
  compaction_reread_budget_bytes?: number;
}

/** Literal `false` opts out of animation. Malformed, missing, and every other value retain the enabled default. */
export function resolveAgentWaitAnimation(config: GoalLoopConfig | undefined): boolean {
  return config?.agent_wait_animation !== false;
}

/** Default number of consecutive no-tool-call re-fires before the loop force-stops, absent a tuned value. */
export const DEFAULT_RUNAWAY_THRESHOLD = 10;

/** Default advisory context-usage percent (adapter-chosen, no ticket-pinned value; config-tunable) surfaced in the reinject reminder. */
export const DEFAULT_COMPACTION_ADVISORY_PERCENT = 50;

/** Default hard compaction percent (261002): a forcing point below Pi's own automatic compaction. */
export const DEFAULT_COMPACTION_HARD_PERCENT = 80;

/** Default settle-timer delay in milliseconds, absent a tuned value (260906 Phase 1). */
export const DEFAULT_SETTLE_DELAY_MS = 5000;

/** Default age since last real child activity before an owned home becomes prune-eligible. */
export const DEFAULT_CHILD_RETENTION_TTL_DAYS = 30;

/**
 * Resolves the effective runaway threshold: the tuned
 * `runaway_threshold` when it is a positive finite number, else
 * `DEFAULT_RUNAWAY_THRESHOLD`. Never hard-fails on a malformed value
 * (non-numeric, zero, negative, `NaN`/`Infinity`) — falls back to the
 * default instead.
 */
export function resolveRunawayThreshold(config: GoalLoopConfig | undefined): number {
  const value = config?.runaway_threshold;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : DEFAULT_RUNAWAY_THRESHOLD;
}

/**
 * Resolves the effective compaction-advisory percent: the tuned
 * `compaction_advisory_percent` when it is a finite number in `(0, 100]`,
 * else `DEFAULT_COMPACTION_ADVISORY_PERCENT`. Never hard-fails on a
 * malformed value — falls back to the default instead. Mirrors
 * `resolveRunawayThreshold`'s exact never-hard-fail shape.
 */
export function resolveCompactionAdvisoryPercent(config: GoalLoopConfig | undefined): number {
  const value = config?.compaction_advisory_percent;
  return typeof value === "number" && Number.isFinite(value) && value > 0 && value <= 100 ? value : DEFAULT_COMPACTION_ADVISORY_PERCENT;
}

/** Resolves `compaction_hard_percent` with `resolveCompactionAdvisoryPercent`'s never-hard-fail shape. */
export function resolveCompactionHardPercent(config: GoalLoopConfig | undefined): number {
  const value = config?.compaction_hard_percent;
  return typeof value === "number" && Number.isFinite(value) && value > 0 && value <= 100 ? value : DEFAULT_COMPACTION_HARD_PERCENT;
}

/** A compaction-trigger boundary: a `turn_end` that continues the run (tool results), a run's final `turn_end`, or `agent_end`. */
export type TriggerBoundary = "tool-turn" | "final-turn" | "run";

/** One compaction trigger threshold (261006): the advisory, an interim milestone, or the hard point. */
export interface CompactionThreshold {
  kind: "advisory" | "milestone" | "hard";
  percent: number;
}

/** The trigger latch value before any threshold has been delivered. */
export const NO_TRIGGER_LATCH = Number.NEGATIVE_INFINITY;

/**
 * The ordered trigger thresholds (261006): the advisory, milestones one and
 * two thirds of the way from it to the hard point, and the hard point. Exact
 * and possibly fractional; only message text rounds. An advisory at or above
 * the hard point never fires (as before 261006), so it and the milestones
 * are omitted.
 */
export function compactionThresholds(advisoryPercent: number, hardPercent: number): CompactionThreshold[] {
  if (advisoryPercent >= hardPercent) return [{ kind: "hard", percent: hardPercent }];
  const step = (hardPercent - advisoryPercent) / 3;
  return [
    { kind: "advisory", percent: advisoryPercent },
    { kind: "milestone", percent: advisoryPercent + step },
    { kind: "milestone", percent: advisoryPercent + 2 * step },
    { kind: "hard", percent: hardPercent },
  ];
}

/** The highest threshold at or below `percent`, or `NO_TRIGGER_LATCH` when none is. */
export function highestThresholdAtOrBelow(thresholds: CompactionThreshold[], percent: number): number {
  let latch = NO_TRIGGER_LATCH;
  for (const t of thresholds) if (t.percent <= percent && t.percent > latch) latch = t.percent;
  return latch;
}

/**
 * Resolves an optional context-window override: the tuned
 * `context_window_override` when it is a finite positive number, else
 * `undefined` (no override — the model's own `getContextUsage().contextWindow`
 * is used as-is). Never hard-fails on a malformed value.
 */
export function resolveContextWindowOverride(config: GoalLoopConfig | undefined): number | undefined {
  const value = config?.context_window_override;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

/**
 * Resolves the effective settle-timer delay: the tuned
 * `settle_delay_ms` when it is a positive finite number, else
 * `DEFAULT_SETTLE_DELAY_MS`. Never hard-fails on a malformed value — falls
 * back to the default instead. Mirrors `resolveRunawayThreshold`'s exact
 * never-hard-fail shape.
 */
export function resolveSettleDelayMs(config: GoalLoopConfig | undefined): number {
  const value = config?.settle_delay_ms;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : DEFAULT_SETTLE_DELAY_MS;
}

function positiveOr(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : fallback;
}

/** Resolves the `## Dialog` section's byte budget for the lead compaction summary; a malformed value falls back to the default. */
export function resolveDialogBudgetBytes(config: GoalLoopConfig | undefined): number {
  return positiveOr(config?.compaction_dialog_budget_bytes, DEFAULT_DIALOG_BUDGET_BYTES);
}

/** Resolves the inlined required re-reads' byte budget for the lead compaction summary; a malformed value falls back to the default. */
export function resolveRereadBudgetBytes(config: GoalLoopConfig | undefined): number {
  return positiveOr(config?.compaction_reread_budget_bytes, DEFAULT_REREAD_BUDGET_BYTES);
}

/** Resolves the child retention policy: `0` disables age pruning (`false`); anything but a positive finite number keeps the default. */
export function resolveChildRetentionTtlDays(config: GoalLoopConfig | undefined): number | false {
  const value = config?.child_retention_ttl_days;
  if (value === 0) return false;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : DEFAULT_CHILD_RETENTION_TTL_DAYS;
}

/** Knobs one settle cycle reads at arm time: the delay, then (at fire) the runaway threshold and the reminder's context percent. The reminder's wake-recovery timer reuses this cycle's `settle_delay_ms`. */
export const SETTLE_CONFIG_KEYS: readonly GoalLoopConfigKey[] = ["settle_delay_ms", "runaway_threshold", "compaction_advisory_percent", "context_window_override"];

/** Knobs one compaction-trigger check reads. */
export const COMPACTION_TRIGGER_CONFIG_KEYS: readonly GoalLoopConfigKey[] = ["compaction_advisory_percent", "compaction_hard_percent", "context_window_override"];

/** Knobs the lead compaction summary reads. */
export const COMPACTION_BUDGET_CONFIG_KEYS: readonly GoalLoopConfigKey[] = ["compaction_dialog_budget_bytes", "compaction_reread_budget_bytes"];

// ---------------------------------------------------------------------------
// Pure message builders.
// ---------------------------------------------------------------------------

/** The goal-entry announcement injected by `/goal <goal>`. Wording pinned verbatim by the ticket; the adapter label is added at the send. */
export function buildGoalAnnouncement(goal: string): string {
  return `${GOAL_ANNOUNCEMENT_PREFIX}${goal}`;
}

/**
 * The `ws-compact` lever's returned tool text (260905 pinned the opening
 * clause naming the in-flight compaction). The prose itself is not echoed:
 * it already sits in the lever call's own arguments and in the summary.
 */
export function buildCompactionLeverResult(): string {
  return "Compaction requested; the conversation will resume from a summary carrying the session state and your prose under the fixed headings.";
}

/** One `ws-compact` file-list entry's JSON schema (261007). */
function fileListSchema(description: string) {
  return {
    type: "array",
    description,
    items: {
      type: "object",
      properties: {
        path: { type: "string", description: "Path relative to the working directory, or absolute." },
        lines: { type: "string", description: "\"start-end\", 1-based and inclusive; omit for the whole file." },
        why: { type: "string", description: "Why the step needs it; the symbol or heading goes here." },
      },
      required: ["path", "why"],
    },
  };
}

/**
 * The `ws-compact` lever's parameter schema (261004): the prose-only
 * `leadProseParameterSchema` plus the optional `continue_after_compact`
 * boolean and (261007) the optional `required_rereads` and `references` file
 * lists. Composed here, not in lead-compaction.ts, because
 * `LEAD_PROSE_SECTIONS` also drives the fallback summary prompt, which must
 * not gain them.
 */
export function leadCompactParameterSchema() {
  const prose = leadProseParameterSchema();
  return {
    ...prose,
    properties: {
      ...prose.properties,
      continue_after_compact: {
        type: "boolean",
        description: "Set true only when you hold known remaining work that does not await the user: a resume message then follows compaction even after the advisory nudge, a milestone, or a user /compact. Omit or false otherwise, and when the next move needs a user answer.",
      },
      required_rereads: fileListSchema(
        "Files the immediate next step needs, each with `lines` (\"start-end\", 1-based) when only part is needed, and why. Every entry lands in the next context: the adapter inlines as many as fit its budget, smallest first, and lists the rest to be read first, so list only what that step needs. Take line numbers from your earlier reads; put the symbol or heading in `why`.",
      ),
      references: fileListSchema(
        "Files the work ahead may need later, opened only when a step calls for them. Same entry shape as required_rereads; never inlined.",
      ),
    },
  };
}

/** The lever's file lists out of its arguments (261007). */
export function leadFileListsFrom(params: unknown): LeadFileLists {
  const p = (typeof params === "object" && params !== null ? params : {}) as { required_rereads?: unknown; references?: unknown };
  return { requiredRereads: normalizeFileList(p.required_rereads), references: normalizeFileList(p.references) };
}

/** What opened the pending preparation boundary: a preparation message's trigger, or a milestone wake turn (261007). */
type PreparationKind = PreparationTrigger["kind"] | "milestone";

/**
 * One host compaction operation. `route` is set only by the `ws-compact`
 * lever (261003): the preparation trigger that led to the call, or
 * `"autonomous"` when no preparation was pending. `continueAfterCompact`
 * (261004) is the lead's explicit "I have work after compaction" request,
 * snapshotted from the lever's strict-`true` argument. `resumeOwed` is set by
 * an idle, successful release of a goal-less operation whose route resumes or
 * that requested continuation, and consumed by the lever's `onComplete`, the
 * only place the resume is sent.
 */
type CompactionOperation = {
  id: number;
  generation: number | undefined;
  route?: PreparationKind | "autonomous";
  continueAfterCompact?: boolean;
  resumeOwed?: boolean;
};

/**
 * Whether a goal-less lever compaction reached by `route` resumes the lead
 * (261003). Only the routes whose run was mid-work when the abort landed do;
 * after the advisory nudge, a milestone wake turn (261007), or a user
 * `/compact` the next move is the user's
 * unless the lead set `continue_after_compact` (261004), the other
 * eligibility term, which the release check ORs with this predicate.
 */
export function resumesAfterCompaction(route: CompactionOperation["route"]): boolean {
  return route === "hard" || route === "autonomous";
}

/**
 * The goal-less resume message (261003): Pi's `compact()` aborts the run that
 * called the lever and never continues it, so after a compaction that cut
 * work short this user-role `followUp` starts the next turn.
 */
export function buildCompactionResumeMessage(sessionKey: string | undefined): string {
  const key = sessionKey?.trim() ? `\`${sessionKey}\`` : "(recover it first)";
  return `Compaction complete. Invoke \`lead-revive\` (\`ws-skill lead-revive\`) with session key ${key}, then continue the immediate next step; if it awaits the user, end your turn.`;
}

/**
 * Pure helper: resolves the effective context-usage percent from
 * `ctx.getContextUsage()` and an optional config override, without ever
 * dividing by zero or crashing on `null` fields. `usage.percent`/`tokens` are
 * `null` right after a compaction until the next LLM response — this
 * function returns `null` in that case (and whenever nothing computable is
 * available), letting callers render "unknown" gracefully instead.
 *
 * - No `usage` at all -> `null`.
 * - `usage.percent` is a number and no override is set -> that percent, as-is.
 * - `usage.tokens` is a number, and either an override is set or
 *   `usage.percent` is `null` -> recomputed as
 *   `(tokens / (contextWindowOverride ?? usage.contextWindow)) * 100`.
 * - Otherwise -> `null`.
 */
export function computeContextPercent(usage: ContextUsage | undefined, contextWindowOverride?: number): number | null {
  if (!usage) return null;
  if (typeof usage.percent === "number" && contextWindowOverride === undefined) {
    return usage.percent;
  }
  if (typeof usage.tokens === "number" && (contextWindowOverride !== undefined || usage.percent === null)) {
    const window = contextWindowOverride ?? usage.contextWindow;
    return (usage.tokens / window) * 100;
  }
  return null;
}

/**
 * The re-injected reminder on each armed `agent_settled` re-fire. Names the
 * goal and all three lever tool names (two terminal, one non-terminal
 * compact-and-continue), the runaway-backstop caveat, the current
 * context-usage percent (surfaced from IO-glue-only `ctx.getContextUsage()`,
 * hence the caller-supplied `info` rather than this function reaching for it
 * itself), and a static compression-safety heuristic. The heuristic is
 * advisory prose the model weighs — never a computed gate the extension
 * enforces. Exact wording is a presentation detail beyond the ticket's
 * pinned framing of "the model decides."
 */
export function buildGoalReminder(goal: string, info: { percent: number | null; advisoryPercent: number }): string {
  const { percent, advisoryPercent } = info;
  const usageLine =
    percent === null
      ? "Context usage: unknown."
      : percent >= advisoryPercent
        ? `Context usage: ${Math.round(percent)}% of window — at or above the advisory point (${advisoryPercent}%); prioritize ${LEAD_COMPACT_TOOL_NAME} when the next work is weakly related to the current context and you are at a safe compaction point.`
        : `Context usage: ${Math.round(percent)}% of window — below the compaction advisory point (${advisoryPercent}%); do not call ${LEAD_COMPACT_TOOL_NAME}.`;
  return (
    `Goal yet running: "${goal}". Call goal-achieved <summary> or goal-blocked <reason> for a state ` +
    `transition, or ${LEAD_COMPACT_TOOL_NAME} to compact context and keep pursuing the ` +
    "same goal. Silence keeps re-injecting this reminder; enough consecutive re-fires with no tool call " +
    "force-stops the goal loop.\n" +
    `${usageLine}\n` +
    "Compression-safety heuristic (advisory only — you weigh it, the extension never gates or auto-compacts): " +
    "a phase-boundary or merge-gate stop is generally safe to compact; a non-phase-boundary stop is generally not."
  );
}

// ---------------------------------------------------------------------------
// Pure state machine.
// ---------------------------------------------------------------------------

export interface GoalLoopState {
  active: boolean;
  goal?: string;
  noToolCallStreak: number;
  sawToolCallThisCycle: boolean;
  /** Raw lever payload, retained across waits/yields until the next reminder send; never persisted. */
  pendingCarryForward?: string;
}

/** The inert/disarmed state — also the state after a terminal lever fires or the runaway backstop trips. */
export function initialGoalLoopState(): GoalLoopState {
  return { active: false, noToolCallStreak: 0, sawToolCallThisCycle: false };
}

/** Arms goal mode. Idempotent/replacing: always returns a fresh active state regardless of any prior state. */
export function armGoal(goal: string): GoalLoopState {
  return { active: true, goal, noToolCallStreak: 0, sawToolCallThisCycle: false };
}

/** Disarms goal mode, returning to the inert initial state — used by both terminal levers. */
export function disarmGoal(): GoalLoopState {
  return initialGoalLoopState();
}

/**
 * Pure predicate: `true` when `env` carries ANY spawned-child process-role
 * marker (`WS_PI_SPAWN_ROLE_ENV`, set by spawner.ts on every spawned
 * child's process environment — see `process-role.ts`). Extracted from the
 * `agent_settled` handler (review fix, cycle 1) — mirroring
 * `decideOnSettle`'s own pure-reducer extraction — so the "no-op in a
 * spawned child" guard is unit-testable without spawning a real process.
 *
 * 260904 Phase 1: reads presence of any role via `readSpawnRole` (subsumes
 * the old boolean `WS_PI_AGENT_CHILD_ENV` equality check) — a `worker`,
 * `explore`, or (reserved, not yet spawned) `fork` child are all still
 * treated as "child" here, keeping this contract intact even though `fork`
 * is treated as lead-or-fork by bridge.ts's separate `isLeadOrFork` gate.
 */
export function isChildProcess(env: NodeJS.ProcessEnv): boolean {
  return Boolean(readSpawnRole(env));
}

/**
 * Records that a tool call happened this cycle. No-op when goal mode is
 * inactive. Any tool call resets the runaway streak on the next settle — not
 * just the goal-lever tools — matching the ticket's "no tool call" wording.
 */
export function recordToolCall(state: GoalLoopState): GoalLoopState {
  if (!state.active) return state;
  return { ...state, sawToolCallThisCycle: true };
}

export type SettleDecision =
  | { action: "ignore" }
  | { action: "yield" }
  | { action: "waiting" }
  | { action: "reinject"; goal: string }
  | { action: "force-stop"; reason: string };

/**
 * Pure reducer for an `agent_settled` firing: decides whether to ignore
 * (goal mode inactive), wait (a compaction is in flight — 260906 Phase 1),
 * yield (goal mode active but a persistent child is still mid-turn — Phase 2,
 * 260905), re-inject a reminder (under threshold), or force-stop (streak
 * reached `threshold`). Streak resets to 0 whenever a tool call happened this
 * cycle; otherwise it increments.
 *
 * `compacting` (default `false`, 260906 Phase 1) is checked BEFORE `yielding`
 * — compaction dominates: while it holds, nothing about running children
 * matters, since the settle this reducer is being asked to judge is the one
 * `ctx.compact()`'s own internal abort just produced, not an ordinary turn
 * end. The state passes through completely unchanged (no streak mutation, no
 * `sawToolCallThisCycle` reset) and the decision is `{ action: "waiting" }` —
 * `goal-loop.ts`'s `releaseAfterCompaction` is what re-injects the reminder
 * once the compaction actually finishes, not this settle.
 *
 * `yielding` (default `false`) is the Phase 2 fan-in gate: when `true` the
 * state passes through completely unchanged (no streak mutation, no
 * `sawToolCallThisCycle` reset) and the decision is `{ action: "yield" }` —
 * neither re-injecting the reminder nor advancing the runaway streak, exactly
 * as if this settle had never fired. The inactive check runs first, so
 * neither `compacting` nor `yielding` ever resurrects an inactive loop.
 *
 * The `"reinject"` decision carries only the bare `goal` string, not a
 * precomputed reminder: this reducer has no access to `ctx.getContextUsage()`
 * or the adapter config (both IO-context-only), so `buildGoalReminder`
 * is called by `registerGoalLoop`'s IO glue instead, right where that context
 * is already available (Phase 2, 260903).
 */
export function decideOnSettle(
  state: GoalLoopState,
  threshold: number,
  yielding = false,
  compacting = false,
): { next: GoalLoopState; decision: SettleDecision } {
  if (!state.active) {
    return { next: state, decision: { action: "ignore" } };
  }
  if (compacting) {
    return { next: state, decision: { action: "waiting" } };
  }
  if (yielding) {
    return { next: state, decision: { action: "yield" } };
  }
  const streak = state.sawToolCallThisCycle ? 0 : state.noToolCallStreak + 1;
  if (streak >= threshold) {
    return {
      next: initialGoalLoopState(),
      decision: { action: "force-stop", reason: `${threshold} consecutive re-fires with no tool call` },
    };
  }
  return {
    next: { ...state, noToolCallStreak: streak, sawToolCallThisCycle: false },
    decision: { action: "reinject", goal: state.goal! },
  };
}

/**
 * Pure builder for the informational `ctx.ui.notify` message emitted by the
 * `session_before_compact` listener while goal mode is active. The goal loop
 * itself never vetoes a compaction; lead compaction ownership (261002) is a
 * separate concern of the same listener.
 */
export function buildCompactionObservation(goal: string, reason: "manual" | "threshold" | "overflow"): string {
  return `Compaction observed while goal-loop is active (goal: "${goal}", reason: ${reason}).`;
}

/**
 * The messages from Pi's kept-tail cut point (`preparation.firstKeptEntryId`)
 * to the end of the branch. Pi excludes them from `messagesToSummarize`
 * because it would keep them raw; this adapter keeps no raw tail (261003), so
 * the fallback summary must read them too or the newest work is lost.
 */
export function keptTailMessages(event: Pick<SessionBeforeCompactEvent, "preparation" | "branchEntries">): ReturnType<typeof sessionEntryToContextMessages> {
  const start = event.branchEntries.findIndex((entry) => entry.id === event.preparation.firstKeptEntryId);
  if (start < 0) return [];
  return event.branchEntries.slice(start).filter((entry) => entry.type !== "compaction").flatMap(sessionEntryToContextMessages);
}

/**
 * Builds the `CompactionResult` the lead's `session_before_compact` handler
 * returns: the summary from lead-compaction.ts, `NO_KEPT_ENTRY_ID` so no raw
 * pre-compaction entry (and so no tool output) is kept after the summary
 * (261003; the `## Dialog` section carries the discussion instead), and
 * details stamped as this adapter's so `session_compact` can recognize the
 * stored entry.
 */
export function buildLeadCompactionResult(
  event: Pick<SessionBeforeCompactEvent, "preparation" | "branchEntries">,
  input: {
    sessionKey: string | undefined;
    registry: RpcAgentRegistry | undefined;
    prose: string;
    dialogBudgetBytes: number;
    sessionFile: string | undefined;
    source: LeadCompactionDetails["source"];
    /** 261007: the lever's file lists; the fallback summary has none. */
    fileLists?: LeadFileLists;
    cwd?: string;
    rereadBudgetBytes?: number;
  },
): CompactionResult<LeadCompactionDetails> {
  const summary = buildLeadCompactionSummary({
    sessionKey: input.sessionKey,
    branchEntries: event.branchEntries,
    registry: input.registry,
    prose: input.prose,
    dialogBudgetBytes: input.dialogBudgetBytes,
    sessionFile: input.sessionFile,
    fileLists: input.fileLists,
    cwd: input.cwd,
    rereadBudgetBytes: input.rereadBudgetBytes,
  });
  return {
    summary,
    firstKeptEntryId: NO_KEPT_ENTRY_ID,
    tokensBefore: event.preparation.tokensBefore,
    details: { kind: LEAD_COMPACTION_DETAILS_KIND, version: 1, source: input.source },
  };
}

// ---------------------------------------------------------------------------
// IO glue: command + tool + event registration.
// ---------------------------------------------------------------------------

type OwnCompactionResult = { compaction: CompactionResult<LeadCompactionDetails> } | undefined;

/** Phase 2 (260905) goal-loop yield status key: cleared unconditionally on the next `agent_start`. */
const GOAL_LOOP_YIELD_STATUS_KEY = "ws-goal-loop-yield";

export interface RegisterGoalLoopOptions {
  /**
   * Reads adapter settings at each use (adapter-config.ts). Production passes
   * the ws-mcp reader; omitted means every knob at its default. A
   * synchronous reader keeps every listener synchronous (the test seam).
   */
  readConfig?: GoalLoopConfigReader;
  /**
   * Phase 2 (260905): the shared RPC registry ref, filled by `index.ts` inside
   * `session_start` (mirrors `execute-gateway.ts`'s `createApprovalRelay`
   * `registryRef?` convention). Optional and degrade-gracefully: an
   * undefined ref (or a ref whose `.current` is still undefined, e.g. before
   * the first `session_start` or in a headless harness that never ran one)
   * means "nothing known to be running" — `hasRunningAgents` resolves to
   * `false` and the loop never yields.
   */
  rpcRegistryRef?: { current: RpcAgentRegistry | undefined };
  /**
   * Injectable timer seam (260906 Phase 1, settle-timer reminder race
   * ticket) — test-only; production callers omit both and get the real
   * `setTimeout`/`clearTimeout`, with `.unref?.()` called on the real handle
   * so the timer never keeps the process alive (mirrors `spawner.ts`'s
   * `startLivenessProbe`). A test passes a fake pair that records `{ cb, ms
   * }` and lets the test invoke `cb()` directly instead of waiting on a real
   * clock — the fake-clock seam the ticket requires, with no new dependency.
   */
  scheduleTimer?: (cb: () => void, ms: number) => NodeJS.Timeout;
  clearTimer?: (handle: NodeJS.Timeout) => void;
  /** The lead's own ws session key, filled by `index.ts` at `session_start`; carried verbatim into the compaction summary. */
  sessionKeyRef?: { current: string | undefined };
  /** Path to `lead-compact-guide.md`, read fresh into every preparation message (261002); absent means a one-line fallback guide. */
  leadCompactGuidePath?: string;
}

/**
 * Handle returned by `registerGoalLoop` so `index.ts`'s `session_shutdown`
 * can reset this module INSTANCE's compaction-related state — mirroring
 * `spawner.ts`'s `heldPushQueue.length = 0` / `leadIdleRef.current =
 * undefined` reset convention, for the closure-private state that has no
 * exported ref of its own (260906 review relay #1, Minor).
 */
export interface GoalLoopShutdownHandle {
  /**
   * Invalidates the goal generation and outstanding reminder correlation,
   * clears `leadCompactingRef` and both compaction markers (`pendingRearm`,
   * `settleSwallowedWhileCompacting`), cancels a pending settle timer and
   * boundary-guard fallback timer (260906 Phase 1), and clears
   * `leadWakeStartPendingRef`. Without this, a `session_shutdown`/
   * `/reload` that lands while a compaction is still in flight would leave
   * `leadCompactingRef.current` stuck `true` into the replacement session,
   * where `isOwningAgentIdle()` (spawner.ts) reports `false` forever and
   * every `followUp` push plus `ask.ts`'s `injectDiscussionSummary` is held
   * with nothing left to release them; and a stuck `leadWakeStartPendingRef`
   * would hold every replacement-session push without a live wake.
   */
  resetCompactionStateForShutdown(): void;
}

/**
 * Registers the `/goal` command, the `goal-achieved`/`goal-blocked`/
 * `ws-compact` tools, and the
 * `tool_call`/`agent_settled`/`session_before_compact` listeners that drive
 * the goal-loop state machine above. Called at extension factory top level
 * (not inside `session_start`) — command/tool registration is declarative
 * here, same as every other command/tool in index.ts.
 */
export function registerGoalLoop(
  pi: ExtensionAPI,
  opts: RegisterGoalLoopOptions,
  toolPreviewTuiRef: ToolPreviewTuiRef = createToolPreviewTuiRef(),
): GoalLoopShutdownHandle {
  let state: GoalLoopState = initialGoalLoopState();
  /**
   * Monotonic identity for the currently armed goal. Every explicit arm and
   * every disarm advances it, so callbacks captured by an older timer or
   * compaction completion can still release shared lifecycle holds without
   * submitting or mutating a replacement goal.
   */
  let goalGeneration = 0;
  /** Invalidates queued replacements only when an immediate command or terminal transition supersedes them. */
  let goalControlGeneration = 0;
  let shuttingDown = false;

  /**
   * Pi's user-message admission is void-returning: after sendUserMessage
   * returns, the adapter cannot tell whether the reminder is still in
   * asynchronous preflight, queued behind an existing run, or executing.
   * Keep one adapter-owned correlation until the public user message_start
   * event exposes that exact payload. This is deliberately independent of
   * the shared push wake reservation, which may clear on agent_start before a
   * queued follow-up reminder is consumed.
   */
  let reminderHandoffSequence = 0;
  let outstandingReminderHandoff: { id: string; generation: number } | undefined;

  // 260906 (compaction push-hold ticket, Phase 1): true only between the
  // `ws-compact` lever (under an active goal) setting `leadCompactingRef` and the
  // settle timer's fire callback consuming it — marks a compaction as
  // LEVER-ORIGINATED, the only kind that should ever synthesize the lever's
  // own re-armed reminder text (with a failure reason folded in when
  // present). An owner-typed `/compact` or Pi's own threshold/overflow
  // auto-compaction also sets `leadCompactingRef` (via
  // `session_before_compact`, defensively) but never this flag; see
  // `settleSwallowedWhileCompacting` below for how THOSE compactions still
  // get the loop re-evaluated once they end a turn outright.
  //
  // 260906 Phase 1 (settle-timer reminder race ticket): survives from
  // `releaseAfterCompaction`'s idle branch (which now only arms the settle
  // timer, never clears this) until the timer's fire callback
  // (`onSettleTimerFire`) actually consumes it — see that function for why
  // the marker must outlive the arm-to-fire delay.
  let pendingRearm = false;

  /**
   * The compaction failure reason to fold into the pending lever reminder,
   * captured by `releaseAfterCompaction` at arm time (260906 Phase 1) and
   * consumed alongside `pendingRearm` by `onSettleTimerFire` — kept as a
   * separate variable rather than inside a combined payload object so the
   * two existing booleans above stay simple flags.
   */
  let pendingRearmFailureReason: string | undefined;
  let pendingRearmGeneration: number | undefined;

  /**
   * 260906 review relay #1 (Critical): true when an `agent_settled` fired
   * while a compaction was in flight and `decideOnSettle` would have judged
   * it `{action:"waiting"}` — this settle's would-be reinject/force-stop
   * outcome was SWALLOWED, not merely deferred. This matters because Pi's
   * own threshold/overflow auto-compaction can end a turn outright with
   * nothing queued to follow it (no `willRetry`, no queued owner input): no
   * further `agent_settled`/`agent_start` ever fires to re-evaluate the
   * loop, so without this marker an armed goal would stop dead — stuck on
   * the "waiting for compaction" footer — at the first such auto-compaction.
   * `releaseAfterCompaction`'s idle branch arms the settle timer when this is
   * set and no lever reminder already covers the same settle; the timer's
   * fire callback (`onSettleTimerFire`, 260906 Phase 1) is what actually
   * replays exactly one ordinary settle decision (same reducer, streak
   * accounting, and force-stop path as a live `agent_settled`, against a
   * freshly-read context percent). Cleared by whichever path in
   * `onSettleTimerFire` actually resolves this settle, by the not-idle
   * handoff, and by deferred busy release — see each site below.
   */
  let settleSwallowedWhileCompacting = false;
  let settleSwallowedGeneration: number | undefined;

  /** One host compaction operation; its id prevents a late duplicate callback from releasing a newer hold. */
  let compactionSequence = 0;
  let activeCompaction: CompactionOperation | undefined;

  /**
   * 261002: the lead's rendered prose, set by the `ws-compact` lever right
   * before `ctx.compact()` and consumed by the next `reason: "manual"`
   * `session_before_compact`. Its presence is what tells a lever-initiated
   * manual compaction apart from any other manual one. Cleared by that
   * consumption or by the lever's own completion/failure callback.
   */
  let pendingLever: { prose: string; fileLists: LeadFileLists; operationId: number } | undefined;

  /**
   * 261002 Phase 2 trigger state (lead only). `triggerLatch` (261006) is the
   * highest compaction threshold already delivered (`NO_TRIGGER_LATCH` when
   * none): each threshold fires once per crossing, a compaction resets the
   * latch, and usage observed below it lowers it (see `fireCompactionTriggers`).
   * `preparation` is set
   * when a preparation message or (261007) a milestone wake turn is sent and
   * blocks every trigger until the
   * next `agent_end`: Pi drains its steer and follow-up queues before that
   * event, so by then the message has either run or been dropped (an abort
   * clears the queues), and a dropped one must not disable the triggers.
   * `pendingReroute` carries a cancelled `/compact`'s focus text to the
   * failure event that follows the cancel. `expectOwnCompaction` marks that
   * this adapter answered the current compaction, so a stored entry that is
   * not ours means another extension's result won.
   */
  let triggerLatch = NO_TRIGGER_LATCH;
  /** Bumped by every trigger delivery and latch reset, so a background baseline can tell it was overtaken. */
  let latchEpoch = 0;
  let preparation = false;
  /** 261003: the trigger kind of the pending preparation; set and cleared with `preparation`. */
  let preparationKind: PreparationKind | undefined;
  let pendingReroute: { focus?: string } | undefined;
  let expectOwnCompaction = false;
  let competingNoticeShown = false;

  const scheduleTimer =
    opts.scheduleTimer ??
    ((cb: () => void, ms: number): NodeJS.Timeout => {
      const handle = setTimeout(cb, ms);
      handle.unref?.();
      return handle;
    });
  const clearTimer = opts.clearTimer ?? ((handle: NodeJS.Timeout) => clearTimeout(handle));
  const readConfig = opts.readConfig ?? staticConfigReader();

  /**
   * The single settle timer (260906 Phase 1, settle-timer reminder race
   * ticket): armed by every `agent_settled` that finds goal mode active and
   * not compacting, and by `releaseAfterCompaction`'s idle branch when a
   * lever reminder or a swallowed settle is pending release. `undefined`
   * whenever nothing is pending — the sole condition `resetCompactionStateForShutdown`
   * and the cancel points below need to check before clearing it.
   */
  let settleTimer: NodeJS.Timeout | undefined;
  let settleTimerSequence = 0;
  let activeSettleTimerId: number | undefined;

  function cancelSettleTimer(): void {
    if (settleTimer !== undefined) clearTimer(settleTimer);
    settleTimer = undefined;
    activeSettleTimerId = undefined;
  }

  function isCurrentArmedGeneration(generation: number): boolean {
    return !shuttingDown && state.active && goalGeneration === generation;
  }

  /** Clear goal-owned scheduling without touching compaction or child-report wake ownership. */
  function clearGoalOwnedWork(): void {
    cancelSettleTimer();
    pendingRearm = false;
    pendingRearmFailureReason = undefined;
    pendingRearmGeneration = undefined;
    settleSwallowedWhileCompacting = false;
    settleSwallowedGeneration = undefined;
  }

  /** Invalidate active and queued goal-owned work. */
  function invalidateGoal(): void {
    goalGeneration += 1;
    goalControlGeneration += 1;
    clearGoalOwnedWork();
    state = disarmGoal();
  }

  /** Apply one FIFO replacement without invalidating later replacements admitted in the same generation. */
  function applyQueuedGoal(goal: string, generation: number): HeldGoalReplacementResult {
    if (shuttingDown) {
      return { outcome: "failed", message: `Goal update failed: session ended before "${goal}" could be applied.` };
    }
    if (generation !== goalControlGeneration) {
      return { outcome: "failed", message: `Goal update failed: "${goal}" was invalidated by a newer immediate or terminal goal transition.` };
    }
    const next = armGoal(goal);
    clearGoalOwnedWork();
    goalGeneration += 1;
    state = next;
    return { outcome: "applied", message: `Goal update applied: ${goal}` };
  }

  function beginCompaction(generation: number | undefined): CompactionOperation {
    const operation: CompactionOperation = { id: ++compactionSequence, generation };
    activeCompaction = operation;
    leadCompactingRef.current = true;
    return operation;
  }

  /**
   * Runs a timer callback body inside a try/catch that reports (best effort)
   * via `ctx.ui.notify` and never throws (260906 Phase 1 review relay #1,
   * Minor). Unlike an extension event handler, a `setTimeout` callback is not
   * caught by Pi's own runner error boundary, and Pi's interactive mode
   * installs an `uncaughtException` listener that calls `process.exit(1)` —
   * so an unhandled throw here (most realistically a stale captured
   * `pi`/`ctx` failing `assertActive()` after a reload/session replacement
   * that beat `session_shutdown`'s cancel) would otherwise crash the whole
   * process instead of, at worst, leaving the goal loop stalled.
   */
  function runTimerCallback(ctx: ExtensionContext, label: string, fn: () => void): void {
    try {
      fn();
    } catch (error) {
      try {
        const message = error instanceof Error ? error.message : String(error);
        ctx.ui.notify(`Goal loop: internal error in ${label}: ${message}`, "error");
      } catch {
        // best effort — never let the report itself throw.
      }
    }
  }

  /**
   * Arms (re-arming cancels any prior pending timer first) the settle timer
   * that will re-evaluate the loop after `settle_delay_ms` (260906 Phase 1).
   * Sets the `Goal loop: settling` footer unconditionally — every caller
   * (the live `agent_settled` listener and `releaseAfterCompaction`'s idle
   * branch alike) wants this status the instant a re-evaluation is pending.
   *
   * The cycle's adapter settings are read here, once per arm, and carried to
   * the fire. The timer id is claimed before the read, so an asynchronous
   * read that resolves after `cancelSettleTimer` (an `agent_start`, a re-arm,
   * a disarm) schedules nothing. Callers never wait on the read: a failure
   * past it is reported through `runTimerCallback`, never thrown.
   */
  function armSettleTimer(ctx: ExtensionContext, generation = goalGeneration): void {
    if (!isCurrentArmedGeneration(generation)) return;
    cancelSettleTimer();
    // Settle evaluation must not cancel recovery for a held-push wake.
    const timerId = ++settleTimerSequence;
    activeSettleTimerId = timerId;
    ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: settling");
    thenOrNow(readConfig(SETTLE_CONFIG_KEYS), (config) => {
      runTimerCallback(ctx, "settle timer arm", () => {
        if (activeSettleTimerId !== timerId || !isCurrentArmedGeneration(generation)) return;
        settleTimer = scheduleTimer(
          () => runTimerCallback(ctx, "settle timer", () => onSettleTimerFire(ctx, generation, timerId, config)),
          resolveSettleDelayMs(config),
        );
      });
    });
  }

  /**
   * Sends one correlation-marked reinject reminder as a `followUp` turn and
   * arms the boundary-guard fallback timer (260906 Phase 1) — the single call site
   * for actually dispatching a reminder, used by both `onSettleTimerFire`'s
   * `pendingRearm` branch and its ordinary/replayed-settle `"reinject"`
   * branch, so the boundary-guard bookkeeping can never drift between them.
   * `followUp` is always correct here (not just for the replay case the
   * pre-timer code distinguished): the timer only ever fires this once
   * `ctx.isIdle()` is confirmed fresh, so a bare send would also work, but
   * `followUp` additionally survives a push that raced in and started a
   * turn between the fire-condition check and this call. The correlation is
   * cleared only by a matching public user `message_start`; timeout recovery
   * never resubmits while that handoff is unconfirmed.
   */
  function fireReminder(
    ctx: ExtensionContext,
    config: GoalLoopConfig | undefined,
    reminder: string,
    generation: number,
  ): void {
    if (!isCurrentArmedGeneration(generation) || outstandingReminderHandoff) return;

    // Shared reservation and recovery precede dispatch, including a throwing send.
    if (!reserveWakeStart({ delayMs: () => resolveSettleDelayMs(config), scheduleTimer, clearTimer }, () => {
      runTimerCallback(ctx, "wake fallback timer", () => {
        if (heldPushQueue.length) {
          flushHeldPushes(pi);
        } else if (isCurrentArmedGeneration(generation)) {
          if (outstandingReminderHandoff) {
            // Pi accepted this exact marked user message, but has not exposed
            // its consumption yet. Retrying here would hand off a duplicate.
            ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: awaiting reminder handoff");
          } else {
            armSettleTimer(ctx, generation);
            ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: reminder did not start a turn, retrying");
          }
        }
      });
    })) return;

    const handoff = { id: `${generation}-${++reminderHandoffSequence}`, generation };
    outstandingReminderHandoff = handoff;
    reminder += `\n\n${GOAL_REMINDER_MARKER_PREFIX}${handoff.id} -->`;
    if (state.pendingCarryForward !== undefined) {
      reminder += `\n\nCarried forward verbatim from before compaction:\n${state.pendingCarryForward}`;
    }
    try {
      pi.sendUserMessage(labelAdapterText(reminder), { deliverAs: "followUp" });
    } catch (error) {
      if (outstandingReminderHandoff?.id === handoff.id) outstandingReminderHandoff = undefined;
      throw error;
    }
    // Consume only after dispatch returns: a synchronous send throw retains
    // the payload for the next eligible reminder, regardless of its origin.
    state.pendingCarryForward = undefined;
  }

  /**
   * Shared decision dispatch for a `SettleDecision` produced by
   * `decideOnSettle` at fire time (260906 Phase 1: only ever called from
   * `onSettleTimerFire`, with `yielding`/`compacting` both `false` since the
   * fire-time gate already excluded those cases — `"waiting"`/`"yield"` are
   * therefore unreachable in practice here but handled for defense in depth,
   * matching the reducer's own full `SettleDecision` union).
   */
  function dispatchSettleDecision(ctx: ExtensionContext, config: GoalLoopConfig | undefined, decision: SettleDecision): void {
    if (decision.action === "ignore") return;
    if (decision.action === "waiting") {
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: waiting for compaction");
      return;
    }
    if (decision.action === "yield") {
      // 260906 Phase 1 review relay #1 (Minor): unreachable via this dispatch
      // path — `decideOnSettle` is only ever called from `onSettleTimerFire`
      // with `yielding: false` (the fire-time gate already returns before
      // reaching it for a running child), so this branch never actually
      // fires. No status string here: the footer is left reading `Goal loop:
      // settling`, matching what a real yield-at-fire-time now shows. Kept as
      // a bare no-op only for defense in depth against `SettleDecision`'s
      // full union.
      return;
    }
    if (decision.action === "force-stop") {
      invalidateGoal();
      ctx.ui.notify(`Goal loop force-stopped: ${decision.reason}`, "warning");
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
      return;
    }
    const advisoryPercent = resolveCompactionAdvisoryPercent(config);
    const contextWindowOverride = resolveContextWindowOverride(config);
    const percent = computeContextPercent(ctx.getContextUsage(), contextWindowOverride);
    const reminder = buildGoalReminder(decision.goal, { percent, advisoryPercent });
    fireReminder(ctx, config, reminder, goalGeneration);
  }

  /**
   * The settle timer's fire callback (260906 Phase 1, settle-timer reminder
   * race ticket) — the single reminder-dispatch path in this module. Runs
   * `settle_delay_ms` after whichever `armSettleTimer` call scheduled it.
   *
   * Fire condition, evaluated FRESH here (not at arm/settle time): yields —
   * no send, no status-line change (the footer is left reading `Goal loop:
   * settling`) — whenever the owning session is not idle or any delegated
   * child is still running; the next live `agent_settled` re-arms in both
   * cases. A compaction now in flight is handled separately and FIRST (see
   * below): it marks `settleSwallowedWhileCompacting` before yielding, since
   * nothing else will re-arm the timer once a compaction is running (260906
   * Phase 1 review relay #1, Important #1). This is what lets a child push
   * that lands during the settle delay, or a compaction that starts during
   * it, cancel the reminder without a special-cased handler: the timer
   * itself is the single place that ever decides to send.
   *
   * Past the gate, at most one of three origins consumes this fire (each
   * must survive un-cleared from settle/arm time until here, since the gate
   * above can yield any number of times first): a lever-originated
   * `pendingRearm` reminder (with its captured failure reason folded in),
   * a replayed `settleSwallowedWhileCompacting` settle, or an ordinary
   * live-settle evaluation — the last two share the same
   * `decideOnSettle`/`dispatchSettleDecision` path since both need nothing
   * more than the pure reducer against the current `state`.
   */
  function onSettleTimerFire(ctx: ExtensionContext, generation: number, timerId: number, config: GoalLoopConfig): void {
    if (activeSettleTimerId !== timerId) return;
    settleTimer = undefined;
    activeSettleTimerId = undefined;
    if (!isCurrentArmedGeneration(generation)) return;

    // 260906 Phase 1 review relay #1 (Important #1): a compaction that starts
    // DURING the settle delay dominates, mirroring `decideOnSettle`'s own
    // "compacting checked before yielding" ordering. Unlike the `notIdle`/
    // `yielding` yields below, a compacting yield here would otherwise strand
    // this settle forever: Pi's `ctx.compact()` never re-enters
    // `_runAgentPrompt`, so no further `agent_settled`/`agent_start` fires to
    // re-arm the timer on its own. Marking it swallowed makes
    // `releaseAfterCompaction`'s idle branch re-arm the timer once the
    // in-flight compaction actually finishes, exactly as it already does for
    // a settle that arrived while compacting was already true.
    if (leadCompactingRef.current) {
      settleSwallowedWhileCompacting = true;
      settleSwallowedGeneration = generation;
      return;
    }

    const notIdle = !ctx.isIdle();
    const yielding = hasRunningAgents(opts.rpcRegistryRef?.current);
    if (notIdle || yielding || leadWakeStartPendingRef.current) return;
    // Pushes own the next wake; do not consume origins, carry, or streak.
    if (heldPushQueue.length) {
      flushHeldPushes(pi);
      return;
    }
    if (outstandingReminderHandoff) {
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: awaiting reminder handoff");
      return;
    }

    if (pendingRearm && pendingRearmGeneration === generation) {
      pendingRearm = false;
      pendingRearmGeneration = undefined;
      settleSwallowedWhileCompacting = false; // exactly one reminder for this settle
      settleSwallowedGeneration = undefined;
      const failureReason = pendingRearmFailureReason;
      pendingRearmFailureReason = undefined;
      if (!state.active || !state.goal) return; // nothing lever-originated to say
      const advisoryPercent = resolveCompactionAdvisoryPercent(config);
      const contextWindowOverride = resolveContextWindowOverride(config);
      const percent = computeContextPercent(ctx.getContextUsage(), contextWindowOverride);
      let reminder = buildGoalReminder(state.goal, { percent, advisoryPercent });
      if (failureReason) {
        // `failureReason` is caller-formatted (see the lever's `onError` and
        // the `session_compact_failed` listener below) — Pi's own
        // `errorMessage` already reads `"Compaction failed: …"` /
        // `"Auto-compaction failed: …"`, so this must not add a second
        // prefix on top of it.
        reminder = `${failureReason} Do not retry ${LEAD_COMPACT_TOOL_NAME} — call goal-achieved or goal-blocked instead.\n${reminder}`;
      }
      fireReminder(ctx, config, reminder, generation);
      return;
    }

    if (settleSwallowedWhileCompacting && settleSwallowedGeneration !== generation) return;
    settleSwallowedWhileCompacting = false; // no-op if it was already false
    settleSwallowedGeneration = undefined;
    const threshold = resolveRunawayThreshold(config);
    const { next, decision } = decideOnSettle(state, threshold, false, false);
    state = next;
    dispatchSettleDecision(ctx, config, decision);
  }

  /**
   * Adapter prompts raised while another adapter prompt is still awaiting its
   * `agent_start`; delivered as followUps by the next `agent_start`.
   */
  const promptsAwaitingStart: Array<{ text: string; wanted: () => boolean }> = [];

  /**
   * One adapter-issued idle prompt in flight at a time. Pi's `prompt()` marks
   * the session streaming only after several awaits, so a second idle
   * `sendUserMessage` issued before the first one's `agent_start` also passes
   * the idle check (`deliverAs` is ignored when not streaming); its inner
   * prompt then throws "already processing" and its settle clears the host's
   * run flag while the first run continues, leaving the session reading idle
   * mid-run. So an idle prompt holds the shared wake reservation until
   * `agent_start` (holding pushes and the goal reminder meanwhile), and a
   * prompt raised while any reservation is pending waits for that start and
   * then queues as a followUp behind the streaming turn. A busy session
   * queues a followUp itself and needs neither.
   */
  function sendAdapterPrompt(prompt: string, wanted: () => boolean, options?: { deliverAs: "followUp" }): void {
    // Labeled once here, so a prompt held for the next start carries it too.
    const text = labelAdapterText(prompt);
    if (leadWakeStartPendingRef.current) {
      promptsAwaitingStart.push({ text, wanted });
      return;
    }
    // Recovery mirrors the reminder's: a lapsed reservation releases held
    // pushes; the accepted prompt itself is never resubmitted.
    if (isOwningAgentIdle()) reserveAdapterPromptStart(() => { if (heldPushQueue.length) flushHeldPushes(pi); });
    pi.sendUserMessage(text, options);
  }

  /**
   * 261003: sends the goal-less resume owed by `operation`, at most once.
   * Called only from the lever's `onComplete`, after its own release attempt:
   * Pi's `prompt()` throws while its `_compactionAbortController` is set, and
   * Pi clears that only after awaiting every `session_compact` handler, so
   * the `session_compact` release's `setImmediate` can run first when another
   * extension's handler is async, while `onComplete` always runs after
   * `compact()` returned. A push wake that release just issued has not
   * started yet, so the resume cannot queue behind it: `sendAdapterPrompt`
   * holds it until that wake's `agent_start`.
   */
  function sendOwedResume(operation: CompactionOperation): void {
    if (!operation.resumeOwed) return;
    operation.resumeOwed = false;
    if (shuttingDown || state.active) return;
    sendAdapterPrompt(buildCompactionResumeMessage(opts.sessionKeyRef?.current), () => !shuttingDown && !state.active, { deliverAs: "followUp" });
  }

  /**
   * Deferred completion/failure owns release of the independent compaction hold.
   * Idle release requests a push wake without draining, then arms pending goal
   * evaluation. Busy release leaves pushes to the run's settle and clears only
   * reminder origins; verbatim carry remains until an eligible reminder.
   */
  function releaseAfterCompaction(
    ctx: ExtensionContext,
    failureReason?: string,
    operation: CompactionOperation | undefined = activeCompaction,
    failed = false,
  ): void {
    if (!leadCompactingRef.current) return; // idempotent: already released
    if (activeCompaction && operation?.id !== activeCompaction.id) return; // stale callback for an older operation
    leadCompactingRef.current = false;
    if (!activeCompaction || operation?.id === activeCompaction.id) activeCompaction = undefined;

    const generation = operation?.generation;
    const pendingBelongsToOperation = pendingRearm && pendingRearmGeneration === generation;
    const swallowedBelongsToOperation = settleSwallowedWhileCompacting && settleSwallowedGeneration === generation;

    if (!ctx.isIdle()) {
      // Preserve settle-time delivery rather than racing an already-busy run.
      if (pendingBelongsToOperation) {
        pendingRearm = false;
        pendingRearmGeneration = undefined;
        pendingRearmFailureReason = undefined;
      }
      if (swallowedBelongsToOperation) {
        settleSwallowedWhileCompacting = false;
        settleSwallowedGeneration = undefined;
      }
      return;
    }
    const flushed = flushHeldPushes(pi);
    // 261003: only marks the resume owed; `sendOwedResume` sends it from the
    // lever's onComplete (see there for why never from here).
    if (operation && !failed && !shuttingDown && generation === undefined && !state.active && (resumesAfterCompaction(operation.route) || operation.continueAfterCompact === true)) {
      operation.resumeOwed = true;
    }
    const rearmIsCurrent = generation !== undefined && isCurrentArmedGeneration(generation);
    if (!rearmIsCurrent) {
      if (pendingBelongsToOperation) {
        pendingRearm = false;
        pendingRearmGeneration = undefined;
        pendingRearmFailureReason = undefined;
      }
      if (swallowedBelongsToOperation) {
        settleSwallowedWhileCompacting = false;
        settleSwallowedGeneration = undefined;
      }
      if (flushed > 0 && heldPushQueue.length === 0 && state.active) armSettleTimer(ctx, goalGeneration);
      return;
    }
    if (pendingBelongsToOperation) pendingRearmFailureReason = failureReason;
    if (pendingBelongsToOperation || swallowedBelongsToOperation) armSettleTimer(ctx, generation);
  }

  const goalStopAliases = ["stop", "clear", "reset"] as const;
  pi.registerCommand("goal", {
    description: "Arm goal mode with <goal>, or stop automatic continuation with the exact aliases stop, clear, or reset.",
    getArgumentCompletions: (prefix) => {
      const matches = goalStopAliases.filter((alias) => alias.startsWith(prefix));
      return matches.length ? matches.map((alias) => ({ value: alias, label: alias })) : null;
    },
    handler: async (args, ctx) => {
      const goal = args.trim();
      if (!goal) {
        ctx.ui.notify("Usage: /goal <goal> | /goal stop | /goal clear | /goal reset", "warning");
        return;
      }
      if ((goalStopAliases as readonly string[]).includes(goal)) {
        invalidateGoal();
        ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
        ctx.ui.notify("Automatic goal continuation stopped.", "info");
        return;
      }
      if (!ctx.isIdle()) {
        const generation = goalControlGeneration;
        enqueueHeldGoalReplacement({
          kind: "goal-replacement",
          goal,
          generation,
          apply: () => applyQueuedGoal(goal, generation),
          report: (result) => ctx.ui.notify(result.message, result.outcome === "applied" ? "info" : "error"),
        });
        ctx.ui.notify(`Goal update queued: ${goal}`, "info");
        return;
      }
      invalidateGoal();
      state = armGoal(goal);
      const generation = goalGeneration;
      sendAdapterPrompt(buildGoalAnnouncement(goal), () => isCurrentArmedGeneration(generation));
    },
  });

  // Any tool call (built-in, custom, or the goal levers themselves) resets
  // the runaway streak — fires for every tool call in the session, matching
  // the ticket's "no tool call" wording (not scoped to the goal-lever tools).
  pi.on("tool_call", () => {
    state = recordToolCall(state);
  });

  pi.on("agent_settled", (_event, ctx) => {
    // 260906 Phase 1 (settle-timer reminder race ticket): clear point "on
    // agent_settled" — a real settle is proof the reminder's run at least
    // started, so the boundary guard and its own fallback timer have
    // nothing left to guard against for this cycle. Runs before the
    // child-process guard below since it is unconditional bookkeeping, not
    // goal-mode-only.
    clearWakeStart();

    // Defense-in-depth: never re-fire inside a spawned child process (the
    // goal-loop is lead-session-only per the ticket's settled cross-ticket
    // fact). Each child also starts with its own inert module-level state,
    // but this guard protects against a `/goal …`-prefixed message reaching
    // a child's input pipeline regardless.
    if (isChildProcess(process.env)) return;
    if (!state.active) return;

    if (leadCompactingRef.current) {
      // 260906 review relay #1 (Critical): this settle's outcome (neither a
      // reinject nor a force-stop) is about to be SWALLOWED — see
      // `settleSwallowedWhileCompacting`'s doc comment for why marking this
      // is required (Pi's own threshold/overflow auto-compaction can end the
      // turn outright with nothing else left to re-evaluate the loop).
      settleSwallowedWhileCompacting = true;
      settleSwallowedGeneration = goalGeneration;
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: waiting for compaction");
      return;
    }

    // 260906 Phase 1: arm the settle timer instead of deciding now — the
    // fire condition (idle / not compacting / no running children) is
    // re-evaluated fresh at fire time, `settle_delay_ms` later, in
    // `onSettleTimerFire`.
    armSettleTimer(ctx);
  });

  // Clears the yield status on the very next lead turn regardless of what
  // started it (owner-typed prompt, or a pushed `steer`/`followUp` message
  // with `triggerTurn: true`) — no per-push-family special-casing needed.
  // Factory scope, registered once — never inside `session_start`, matching
  // this file's own no-duplicate-handlers-across-`/reload` convention (see
  // the `tool_call` listener above).
  //
  // Review fix (relay 1, minor): `!state.active` is guarded here because the
  // status key can only ever have been SET while a goal is active (the
  // `agent_settled` handler above returns before the yield/settling branches
  // when `!state.active`), so this listener has nothing to clear for the
  // common case of a session that never armed a goal. Without this guard,
  // `--mode rpc` would emit one no-op `extension_ui_request` notification per
  // turn forever on every headless session. Safe against a same-turn
  // goal-achieved/goal-blocked disarm: `agent_start` fires before any tool
  // call in its turn, so `state.active` here reflects the state as of the
  // PREVIOUS turn's settle, and a yield never flips `active` — only a
  // terminal lever or force-stop does, both of which run inside a turn whose
  // own `agent_start` already cleared the key on entry.
  pi.on("message_start", (event) => {
    if (event.message.role !== "user" || !outstandingReminderHandoff) return;
    const content = event.message.content;
    const text = typeof content === "string"
      ? content
      : content.filter((part) => part.type === "text").map((part) => part.text).join("");
    const marker = `${GOAL_REMINDER_MARKER_PREFIX}${outstandingReminderHandoff.id} -->`;
    if (text.includes(marker)) outstandingReminderHandoff = undefined;
  });

  // 261002 Phase 2: context-usage triggers (lead only). The hard cut is
  // checked at every turn end and sent as a steer, so it lands at the next
  // tool-call boundary instead of waiting for the run to settle; the
  // advisory nudge waits for the run to end. 261007: interim milestones
  // between the two are guide-less wake turns at agent_end, like the
  // advisory, and open a preparation boundary of their own. None fires while
  // a preparation turn or a compaction is in progress.
  function sendPreparation(trigger: PreparationTrigger, deliverAs: "steer" | "followUp"): void {
    preparation = true;
    preparationKind = trigger.kind;
    pi.sendMessage(
      {
        customType: LEAD_COMPACT_CUSTOM_TYPE,
        content: buildPreparationMessage(trigger, readLeadCompactGuide(opts.leadCompactGuidePath)),
        display: true,
        details: { trigger: trigger.kind },
      },
      { deliverAs, triggerTurn: true },
    );
  }

  function checkCompactionTriggers(ctx: ExtensionContext, boundary: TriggerBoundary): void | Promise<void> {
    if (isChildProcess(process.env) || leadCompactingRef.current || preparation) return;
    return thenOrNow(readConfig(COMPACTION_TRIGGER_CONFIG_KEYS), (config) => {
      // Re-checked: an asynchronous read leaves a gap a compaction or another preparation can enter.
      if (leadCompactingRef.current || preparation) return;
      fireCompactionTriggers(ctx, boundary, config);
    });
  }

  function fireCompactionTriggers(ctx: ExtensionContext, boundary: TriggerBoundary, config: GoalLoopConfig): void {
    const percent = computeContextPercent(ctx.getContextUsage(), resolveContextWindowOverride(config));
    if (percent === null) return;
    const hard = resolveCompactionHardPercent(config);
    const thresholds = compactionThresholds(resolveCompactionAdvisoryPercent(config), hard);
    if (percent < triggerLatch) triggerLatch = highestThresholdAtOrBelow(thresholds, percent);
    const crossed = thresholds.filter((t) => t.percent > triggerLatch && t.percent <= percent);
    // One message per observation, hard > advisory > milestone. A crossing
    // whose boundary does not allow delivery leaves the latch unchanged so it
    // stays pending; a milestone also waits while the advisory is pending.
    const pick = crossed.find((t) => t.kind === "hard") ?? crossed.find((t) => t.kind === "advisory") ?? crossed.find((t) => t.kind === "milestone");
    if (!pick) return;
    if (pick.kind === "hard") {
      sendPreparation({ kind: "hard", percent, threshold: hard }, boundary === "run" ? "followUp" : "steer");
    } else if (pick.kind === "advisory") {
      if (boundary !== "run") return;
      const [first, second] = thresholds.filter((t) => t.kind === "milestone").map((t) => t.percent);
      sendPreparation({ kind: "advisory", percent, threshold: pick.percent, hardPercent: hard, milestonePercents: [first!, second!] }, "followUp");
    } else {
      // 261007: a dedicated wake turn at a run boundary, like the advisory; a
      // steer mid-run was always (rightly) read past. The text is that of the
      // highest milestone at or below usage, the one delivery latches to.
      if (boundary !== "run") return;
      const milestones = thresholds.filter((t) => t.kind === "milestone");
      const delivered = milestones.filter((t) => t.percent <= percent).length as 1 | 2;
      preparation = true;
      preparationKind = "milestone";
      pi.sendMessage(
        {
          customType: LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE,
          content: buildContextMilestoneMessage(delivered, percent, hard),
          display: true,
          details: { milestone: milestones[delivered - 1]!.percent },
        },
        { deliverAs: "followUp", triggerTurn: true },
      );
    }
    // Delivering covers every skipped lower threshold.
    triggerLatch = highestThresholdAtOrBelow(thresholds, percent);
    latchEpoch++;
  }

  /**
   * 261006: re-baselines the latch from the current branch's usage at
   * `session_start` (restart/resume) and `session_tree` (rewind), so a resumed
   * session past the advisory point gets no duplicate advisory and a rewind
   * below it re-arms the advisory. Unknown usage (only a compaction since the
   * last response) means the context is post-compaction: unlatched. The hard
   * point is never baselined: a session resumed or rewound past it gets the
   * hard steer once more, since Pi's own auto compaction sits far above it.
   * Usage is read at the event, not when the config read resolves: a late
   * baseline computed from later usage would latch a crossing still pending
   * delivery. A delivery or reset since the event already set the latch, so
   * the baseline then yields.
   */
  function baselineTriggerLatch(ctx: ExtensionContext): void | Promise<void> {
    if (isChildProcess(process.env)) return;
    const usage = ctx.getContextUsage();
    const epoch = latchEpoch;
    return thenOrNow(readConfig(COMPACTION_TRIGGER_CONFIG_KEYS), (config) => {
      if (latchEpoch !== epoch) return;
      const percent = computeContextPercent(usage, resolveContextWindowOverride(config));
      const softThresholds = compactionThresholds(resolveCompactionAdvisoryPercent(config), resolveCompactionHardPercent(config))
        .filter((t) => t.kind !== "hard");
      triggerLatch = percent === null ? NO_TRIGGER_LATCH : highestThresholdAtOrBelow(softThresholds, percent);
    });
  }

  pi.on("turn_end", (event, ctx) => checkCompactionTriggers(ctx, event?.toolResults?.length ? "tool-turn" : "final-turn"));
  // Not awaited: the production config read goes through the ws-mcp bridge,
  // which another session_start handler is still (re)starting, so awaiting it
  // here would stall session startup on a stale client's read timeout.
  const baselineInBackground = (ctx: ExtensionContext): void => {
    void Promise.resolve(baselineTriggerLatch(ctx)).catch(() => undefined);
  };
  pi.on("session_start", (_event, ctx) => baselineInBackground(ctx));
  pi.on("session_tree", (_event, ctx) => baselineInBackground(ctx));

  pi.on("agent_end", (_event, ctx) => {
    // Any preparation message queued before this run ended has run or was
    // dropped by now; whether the lead compacted or not, later crossings may
    // nudge again. A message queued below, at this boundary, blocks the
    // triggers until the continuation run carrying it ends.
    preparation = false;
    preparationKind = undefined;
    return checkCompactionTriggers(ctx, "run");
  });

  pi.on("agent_start", (_event, ctx) => {
    // 260906 Phase 1 (settle-timer reminder race ticket): cancel points
    // "on agent_start" for the settle timer, the boundary-guard fallback
    // timer, and the boundary guard itself — unconditional, before every
    // check below, since a fresh turn starting is proof neither has
    // anything left to guard against.
    cancelSettleTimer();
    clearWakeStart();
    // The run is streaming now, so a held adapter prompt queues behind it.
    for (const held of promptsAwaitingStart.splice(0)) {
      if (!held.wanted()) continue;
      try {
        pi.sendUserMessage(held.text, { deliverAs: "followUp" });
      } catch {
        // A rejected followUp must not break this run's start handling.
      }
    }

    // A run can start before deferred compaction release. Start is not
    // proof that this independent hold reason has finished.
    if (leadCompactingRef.current) return;
    if (isChildProcess(process.env)) return;
    if (!state.active) return;
    ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
  });

  // 260906 (compaction push-hold ticket, Phase 1): sets `leadCompactingRef`
  // unconditionally, as the very first line, for ANY compaction reason and
  // ANY process role (the lever already set it before calling
  // `ctx.compact()`, so this is defensive coverage for an owner-typed
  // `/compact` and Pi's own threshold/overflow auto-compaction, neither of
  // which goes through the lever).
  //
  // 261002: in the lead session only, a lever-initiated manual compaction is
  // answered with `{ compaction }` (deterministic sections + the lead's
  // prose). Spawned worker/explore/fork sessions return nothing and keep
  // Pi's native compaction.
  pi.on("session_before_compact", (event, ctx) => {
    if (!activeCompaction) beginCompaction(state.active ? goalGeneration : undefined);
    else leadCompactingRef.current = true;
    if (isChildProcess(process.env)) return undefined;
    expectOwnCompaction = false;
    const lever = pendingLever;
    if (event.reason === "manual" && !lever) {
      // A user /compact (or any manual compaction not started by the lever)
      // is cancelled; the failure event Pi emits for the cancel releases the
      // hold and then queues the preparation turn with the focus text.
      pendingReroute = { focus: event.customInstructions };
      return { cancel: true };
    }
    if (state.active && state.goal) ctx.ui.notify(buildCompactionObservation(state.goal, event.reason), "info");
    if (lever) {
      pendingLever = undefined;
      return ownCompaction(event, ctx, lever.prose, "lever", lever.fileLists);
    }
    // Threshold or overflow compaction with no lever prose: summarize in-hook.
    // No session model means Pi's own summarizer cannot run either; leave it
    // to report that.
    if (!ctx.model) return undefined;
    return fallbackCompaction(event, ctx);
  });

  /**
   * The in-hook fallback (261002 Phase 2): one tool-less call to the session
   * model writes the lead prose under the fixed headings from the
   * conversation being summarized, and the deterministic sections are added
   * as for the lever. Any failure (a provider error, an empty answer, an
   * abort) is reported and answered with nothing, so Pi's native summarizer
   * still compacts.
   */
  async function fallbackCompaction(
    event: SessionBeforeCompactEvent,
    ctx: ExtensionContext,
  ): Promise<{ compaction: CompactionResult<LeadCompactionDetails> } | undefined> {
    try {
      const model = ctx.model!;
      const { messagesToSummarize, turnPrefixMessages, previousSummary, settings } = event.preparation;
      // No raw tail is kept, so Pi's would-be kept tail is summarized too -- except on overflow, where the
      // context already exceeds the window and feeding the tail to the summarizer would overflow it as well.
      const tail = event.reason === "overflow" ? [] : keptTailMessages(event);
      const conversationText = serializeConversation(convertToLlm([...messagesToSummarize, ...turnPrefixMessages, ...tail]));
      const prompt = buildFallbackSummaryPrompt(conversationText, previousSummary ? extractLeadProse(previousSummary) : undefined);
      const reserve = Math.floor(0.8 * settings.reserveTokens);
      const response = await ctx.modelRegistry.complete(
        model,
        { systemPrompt: FALLBACK_SYSTEM_PROMPT, messages: [{ role: "user", content: [{ type: "text", text: prompt }], timestamp: Date.now() }] },
        { maxTokens: model.maxTokens > 0 ? Math.min(reserve, model.maxTokens) : reserve, signal: event.signal, cacheRetention: "none", sessionId: randomUUID() },
      );
      if (response.stopReason === "error" || response.stopReason === "aborted") {
        throw new Error(response.errorMessage ?? `summary request ${response.stopReason}`);
      }
      const prose = response.content
        .filter((part): part is { type: "text"; text: string } => part.type === "text")
        .map((part) => part.text)
        .join("\n")
        .trim();
      if (!prose) throw new Error("the summary model returned no text");
      const result = await ownCompaction(event, ctx, prose, "fallback");
      if (result) result.compaction.usage = response.usage;
      return result;
    } catch (error) {
      if (event.signal.aborted) return undefined;
      const message = error instanceof Error ? error.message : String(error);
      ctx.ui.notify(`ws lead compaction fallback summary failed (${message}); Pi's native compaction runs instead.`, "warning");
      return undefined;
    }
  }

  /**
   * Assembles the adapter's compaction result. A build failure (a malformed
   * event, an unexpected registry shape) is reported and answered with
   * nothing, so Pi's native summarizer still compacts rather than the whole
   * compaction failing.
   */
  function ownCompaction(
    event: SessionBeforeCompactEvent,
    ctx: ExtensionContext,
    prose: string,
    source: LeadCompactionDetails["source"],
    fileLists?: LeadFileLists,
  ): OwnCompactionResult | Promise<OwnCompactionResult> {
    return thenOrNow(readConfig(COMPACTION_BUDGET_CONFIG_KEYS), (config): OwnCompactionResult => {
      try {
        const compaction = buildLeadCompactionResult(event, {
          sessionKey: opts.sessionKeyRef?.current,
          registry: opts.rpcRegistryRef?.current,
          prose,
          dialogBudgetBytes: resolveDialogBudgetBytes(config),
          sessionFile: ctx.sessionManager?.getSessionFile?.(),
          source,
          fileLists,
          cwd: ctx.cwd,
          rereadBudgetBytes: resolveRereadBudgetBytes(config),
        });
        expectOwnCompaction = true;
        return { compaction };
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        ctx.ui.notify(`ws lead compaction could not build its summary (${message}); Pi's native compaction runs instead.`, "warning");
        return undefined;
      }
    });
  }

  // 260906 (compaction push-hold ticket, Phase 1): deferred via `setImmediate`
  // so neither handler ever sends a prompt from inside a `session_*compact*`
  // event — `session_compact` fires while Pi's own
  // `_compactionAbortController` is still non-`undefined` (cleared right
  // after), so anything synchronous here would race that same internal
  // state Pi has not finished unwinding yet.
  pi.on("session_compact", (event, ctx) => {
    if (!isChildProcess(process.env)) {
      const ours = isLeadCompactionDetails(event?.compactionEntry?.details);
      if (ours) {
        // 261002: the stored summary is this adapter's and carries the lever
        // prose verbatim, so the next goal reminder need not repeat it. When
        // another summary landed instead, the carry stays for the reminder.
        state.pendingCarryForward = undefined;
      } else if ((expectOwnCompaction || event?.fromExtension) && !competingNoticeShown) {
        // Pi keeps the last non-empty session_before_compact result, so a
        // later extension's result silently replaced this adapter's.
        competingNoticeShown = true;
        ctx.ui.notify(
          "Another extension's compaction replaced the ws lead summary, so the session key, child agents, and carried-forward prose were lost. Disable the competing compaction extension for ws lead sessions.",
          "warning",
        );
      }
      // A compaction re-arms every trigger, drops a pending milestone, and ends any preparation.
      expectOwnCompaction = false;
      triggerLatch = NO_TRIGGER_LATCH;
      latchEpoch++;
      preparation = false;
      preparationKind = undefined;
      pendingReroute = undefined;
    }
    // Defer beyond Pi's own compaction flag; start alone never clears our hold.
    const operation = activeCompaction;
    setImmediate(() => releaseAfterCompaction(ctx, undefined, operation));
  });
  pi.on("session_compact_failed", (event, ctx) => {
    // `event.errorMessage` is passed through as-is: Pi already formats it
    // as `"Compaction failed: …"` / `"Auto-compaction failed: …"` / `"Context
    // overflow recovery failed: …"`, so `releaseAfterCompaction` must not
    // add its own prefix on top (Review relay #1, Minor).
    const operation = activeCompaction;
    expectOwnCompaction = false;
    const reroute = pendingReroute;
    pendingReroute = undefined;
    setImmediate(() => {
      releaseAfterCompaction(ctx, event.errorMessage, operation, true);
      // 261002: the cancelled /compact becomes the preparation turn, queued
      // after the release flushed any held pushes.
      if (reroute && !shuttingDown) sendPreparation({ kind: "reroute", focus: reroute.focus }, "followUp");
    });
  });

  registerWsTool(pi, {
    name: "goal-achieved",
    label: "goal-achieved",
    description: "Terminal lever: declare the active goal achieved and stop the goal-loop re-injection. Call this instead of describing completion in prose.",
    parameters: {
      type: "object",
      properties: {
        summary: { type: "string", description: "Brief summary of how the goal was achieved." },
      },
      required: ["summary"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { summary: string };
      invalidateGoal();
      return { content: [{ type: "text", text: `Goal achieved: ${p.summary}` }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: "goal-blocked",
    label: "goal-blocked",
    description: "Terminal lever: declare the active goal blocked and stop the goal-loop re-injection. Call this instead of describing a blocker in prose.",
    parameters: {
      type: "object",
      properties: {
        reason: { type: "string", description: "Why the goal is blocked." },
      },
      required: ["reason"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { reason: string };
      invalidateGoal();
      return { content: [{ type: "text", text: `Goal blocked: ${p.reason}` }] };
    },
  }, toolPreviewTuiRef);

  registerWsTool(pi, {
    name: LEAD_COMPACT_TOOL_NAME,
    label: LEAD_COMPACT_TOOL_NAME,
    description:
      "Compact the lead's context now. Fill every heading with your carry-forward prose (empty when there is nothing): for content already persisted (tickets, commits, notes, agenda, todos) give its path or pointer; for content that lives only in the conversation, summarize it as precisely as possible. The adapter adds the session key, active ticket and playbook, child agents, and the recent dialog (user messages, your replies, branch summaries, one line per tool call) itself. After compaction, an active goal keeps running. With no goal, a resume message follows when you called this on your own or after the hard-threshold notice; after the advisory nudge, a milestone, or a user /compact, the next move is the user's unless you set continue_after_compact to true because you hold known remaining work that does not await the user.",
    parameters: leadCompactParameterSchema() as never,
    async execute(_toolCallId, params, _signal, _onUpdate, ctx: ExtensionContext) {
      if (isChildProcess(process.env)) {
        throw new Error(`${LEAD_COMPACT_TOOL_NAME} is lead-only; spawned sessions keep Pi's native compaction.`);
      }
      const prose = renderLeadProse(params as LeadProse);
      // ctx.compact() aborts the in-flight turn (the one that invoked this
      // very tool call). Everything it may synchronously trigger — the abort's
      // own settle, the hook below, even completion — must already see the
      // hold, the lever payload, and (under a goal) the rearm marker, so all
      // three are set before the call.
      //
      // 261002: goal-loop state is touched only while a goal is active. With
      // no goal the lever still compacts through the shared compaction hold
      // (pushes stay held until release) but arms no reminder and stores no
      // carry, keeping 260913's no-side-effect property for goal state.
      const goalActive = state.active;
      const operation = beginCompaction(goalActive ? goalGeneration : undefined);
      // 261003: the route is copied before ctx.compact() aborts the run, since
      // that abort's agent_end and the session_compact handler clear the
      // preparation state. A preparation already cleared (its run ended) is
      // an autonomous call.
      operation.route = preparation ? preparationKind : "autonomous";
      // 261004: strict `true` only; Pi's argument validation has already
      // coerced "true"/1, so there is no adapter-side coercion.
      operation.continueAfterCompact = (params as { continue_after_compact?: unknown }).continue_after_compact === true;
      if (goalActive) {
        // Under a goal, `pendingRearm` makes `releaseAfterCompaction`
        // synthesize the re-armed reminder; the invoking turn's own
        // abort-produced settle is swallowed (`settleSwallowedWhileCompacting`).
        pendingRearm = true;
        pendingRearmGeneration = operation.generation;
        // Goal-scoped, not rearm-marker-scoped: busy release and agent_start
        // may clear those markers before an ordinary reminder can carry this.
        // Cleared again once the stored entry proves the summary carried it.
        state.pendingCarryForward = prose;
      }
      pendingLever = { prose, fileLists: leadFileListsFrom(params), operationId: operation.id };
      const clearLever = (): void => {
        if (pendingLever?.operationId === operation.id) pendingLever = undefined;
      };
      ctx.compact({
        // Steers Pi's native summarizer only if this adapter's own result does
        // not land (a build failure, or another extension's result winning).
        customInstructions: prose,
        onComplete: () => {
          clearLever();
          ctx.ui.notify("Compaction completed", "info");
          releaseAfterCompaction(ctx, undefined, operation);
          sendOwedResume(operation);
        },
        onError: (error) => {
          clearLever();
          // Review relay #1 (Minor): the "Compaction failed: " prefix is
          // applied HERE, at the lever's own call site — `error.message` is a
          // raw, unprefixed string, unlike `SessionCompactFailedEvent.errorMessage`
          // (already Pi-formatted; see the `session_compact_failed` listener
          // below), so `releaseAfterCompaction` must not add a prefix of its
          // own or a non-lever failure would double it.
          const failureReason = `Compaction failed: ${error.message}`;
          ctx.ui.notify(failureReason, "error");
          releaseAfterCompaction(ctx, failureReason, operation, true);
        },
      });
      return { content: [{ type: "text", text: buildCompactionLeverResult() }] };
    },
  }, toolPreviewTuiRef);

  return {
    resetCompactionStateForShutdown() {
      shuttingDown = true;
      invalidateGoal();
      outstandingReminderHandoff = undefined;
      promptsAwaitingStart.length = 0;
      activeCompaction = undefined;
      pendingLever = undefined;
      triggerLatch = NO_TRIGGER_LATCH;
      latchEpoch++;
      preparation = false;
      preparationKind = undefined;
      pendingReroute = undefined;
      expectOwnCompaction = false;
      competingNoticeShown = false;
      leadCompactingRef.current = false;
      // 260906 Phase 1 (settle-timer reminder race ticket): cancel point
      // "session shutdown" — a replacement session must not inherit a
      // pending settle/boundary-guard timer from the torn-down one.
      clearWakeStart();
    },
  };
}
