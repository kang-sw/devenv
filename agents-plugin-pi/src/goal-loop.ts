/**
 * Goal-mode arming + `agent_settled` re-injection loop (260903 Phase 1).
 *
 * Design: a user-invoked `/goal <goal>` command arms an in-memory state
 * machine and announces the goal via `pi.sendUserMessage`. While armed, every
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
 * tool. `/goal` itself stays a `registerCommand` (user-invoked entry,
 * matching the ticket's own "goal-entry **command**" wording and the
 * existing `/ws-discuss` precedent in index.ts).
 *
 * Runaway backstop: N consecutive re-fires with no intervening tool call
 * force-stop the loop (disarm goal mode) — Pi has no session-kill primitive
 * that fits here (`ctx.shutdown()` exits the whole process). The threshold
 * is adapter-owned data-file config (`goal-loop-config.json`), a built-in
 * constant default overridden by a file read fresh on every use — the same
 * never-hard-fail/no-caching convention this module's own
 * `readGoalLoopConfig` codifies below. Never lives in ws-mcp (Go core) — the
 * goal-loop is entirely adapter-local (golden rule).
 *
 * Settled cross-ticket fact: the goal-loop runs on the lead session only.
 * The `agent_settled` handler no-ops when the running process is itself a
 * spawned child (any `WS_PI_SPAWN_ROLE_ENV` role set — see
 * `process-role.ts`'s `readSpawnRole`, and spawner.ts's
 * `buildRpcClientOptions`/`buildChildProcessEnv`, both of which carry that
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
 * Following the bridge.ts/spawner.ts convention (not discuss.ts's
 * single-call-site convention): this one file mixes pure, unit-tested
 * state-machine/config-reader functions with the `registerGoalLoop` IO glue,
 * since the goal-loop's IO surface (1 command + 3 tools + 3 event listeners)
 * is closer in shape to spawner.ts than to discuss.ts.
 *
 * Phase 2 (260903) adds a third, non-terminal lever: `goal-compact-and-continue`
 * compacts context with model-supplied carry-forward prose via `ctx.compact()`
 * and re-enters the loop (it does not call `disarmGoal()`). Compaction stays
 * model-driven, not extension-gated: the reinject reminder surfaces
 * `ctx.getContextUsage().percent` plus a static compression-safety heuristic
 * (phase-boundary/merge-gate stops are generally safe to compact, non-phase
 * stops are not) as advisory prose only — the model decides, the extension
 * never autonomously compacts. A `session_before_compact` listener is
 * observe-only (never `cancel`s, never overrides `compaction`): it notifies
 * when a compaction fires while goal mode is active, leaving Pi's own
 * `reason: "threshold"` overflow auto-compaction as the untouched last-resort
 * backstop. Both the compaction-advisory percent and an optional
 * context-window override are additional knobs on the same
 * `goal-loop-config.json` file as Phase 1's `runaway_threshold`.
 */

import { readFileSync } from "node:fs";
import type { ContextUsage, ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import { readSpawnRole } from "./process-role.ts";
import { flushHeldPushes, hasRunningAgents, leadCompactingRef, leadReminderStartPendingRef, type RpcAgentRegistry } from "./spawner.ts";

// ---------------------------------------------------------------------------
// Config: adapter-owned runaway-threshold data file. Never-hard-fail,
// read-fresh-per-call shape — no module-level caching.
// ---------------------------------------------------------------------------

export interface GoalLoopConfig {
  runaway_threshold?: number;
  /** Advisory context-usage percent (0, 100] surfaced in the reinject reminder as a nudge point — not a gate. */
  compaction_advisory_percent?: number;
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
}

/** Default number of consecutive no-tool-call re-fires before the loop force-stops, absent (or overridden by) a config file. */
export const DEFAULT_RUNAWAY_THRESHOLD = 10;

/** Default advisory context-usage percent (adapter-chosen, no ticket-pinned value; config-tunable) surfaced in the reinject reminder. */
export const DEFAULT_COMPACTION_ADVISORY_PERCENT = 70;

/** Default settle-timer delay in milliseconds, absent (or overridden by) a config file (260906 Phase 1). */
export const DEFAULT_SETTLE_DELAY_MS = 5000;

/**
 * Reads and parses the goal-loop config data file. Returns `undefined` —
 * never throws — when the file is missing, unreadable, or not valid JSON, so
 * "unset" (fall back to `DEFAULT_RUNAWAY_THRESHOLD`) is the expected default
 * state. Read fresh on every call by design (no module-level caching).
 */
export function readGoalLoopConfig(path: string): GoalLoopConfig | undefined {
  let raw: string;
  try {
    raw = readFileSync(path, "utf8");
  } catch {
    return undefined;
  }
  try {
    return JSON.parse(raw) as GoalLoopConfig;
  } catch {
    return undefined;
  }
}

/**
 * Resolves the effective runaway threshold: the config file's
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
 * Resolves the effective compaction-advisory percent: the config file's
 * `compaction_advisory_percent` when it is a finite number in `(0, 100]`,
 * else `DEFAULT_COMPACTION_ADVISORY_PERCENT`. Never hard-fails on a
 * malformed value — falls back to the default instead. Mirrors
 * `resolveRunawayThreshold`'s exact never-hard-fail shape.
 */
export function resolveCompactionAdvisoryPercent(config: GoalLoopConfig | undefined): number {
  const value = config?.compaction_advisory_percent;
  return typeof value === "number" && Number.isFinite(value) && value > 0 && value <= 100 ? value : DEFAULT_COMPACTION_ADVISORY_PERCENT;
}

/**
 * Resolves an optional context-window override: the config file's
 * `context_window_override` when it is a finite positive number, else
 * `undefined` (no override — the model's own `getContextUsage().contextWindow`
 * is used as-is). Never hard-fails on a malformed value.
 */
export function resolveContextWindowOverride(config: GoalLoopConfig | undefined): number | undefined {
  const value = config?.context_window_override;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

/**
 * Resolves the effective settle-timer delay: the config file's
 * `settle_delay_ms` when it is a positive finite number, else
 * `DEFAULT_SETTLE_DELAY_MS`. Never hard-fails on a malformed value — falls
 * back to the default instead. Mirrors `resolveRunawayThreshold`'s exact
 * never-hard-fail shape.
 */
export function resolveSettleDelayMs(config: GoalLoopConfig | undefined): number {
  const value = config?.settle_delay_ms;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : DEFAULT_SETTLE_DELAY_MS;
}

// ---------------------------------------------------------------------------
// Pure message builders.
// ---------------------------------------------------------------------------

/** The goal-entry announcement injected by `/goal <goal>`. Wording pinned verbatim by the ticket. */
export function buildGoalAnnouncement(goal: string): string {
  return `Goal armed: ${goal}`;
}

/**
 * The `goal-compact-and-continue` lever's returned tool text. Extracted as a
 * pure helper so the wording is unit-testable without stubbing `ctx.compact`
 * (this file's `registerGoalLoop` IO glue is otherwise covered by the live
 * `pi --mode json` gate, not this unit suite).
 */
export function buildCompactionLeverResult(carryForward: string): string {
  return `Compaction requested; the conversation will resume from a summary carrying: ${carryForward}`;
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
        ? `Context usage: ${Math.round(percent)}% of window — at or above the advisory point (${advisoryPercent}%); consider goal-compact-and-continue if you are at a safe compaction point.`
        : `Context usage: ${Math.round(percent)}% of window — below the compaction advisory point (${advisoryPercent}%); do not call goal-compact-and-continue.`;
  return (
    `Goal yet running: "${goal}". Call goal-achieved <summary> or goal-blocked <reason> for a state ` +
    "transition, or goal-compact-and-continue <carry-forward> to compact context and keep pursuing the " +
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
 * or the goal-loop config file (both IO-context-only), so `buildGoalReminder`
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
 * Pure builder for the observational `ctx.ui.notify` message emitted by the
 * `session_before_compact` listener while goal mode is active. Never a veto,
 * never a `compaction` override — purely informational, matching the
 * ticket's resolved "not an extension gate" design. Extracted for unit
 * coverage while keeping the listener itself thin IO glue.
 */
export function buildCompactionObservation(goal: string, reason: "manual" | "threshold" | "overflow"): string {
  return `Compaction observed while goal-loop is active (goal: "${goal}", reason: ${reason}). Advisory-only observation — the goal loop does not cancel or override this compaction.`;
}

// ---------------------------------------------------------------------------
// IO glue: command + tool + event registration.
// ---------------------------------------------------------------------------

/** Phase 2 (260905) goal-loop yield status key: cleared unconditionally on the next `agent_start`. */
const GOAL_LOOP_YIELD_STATUS_KEY = "ws-goal-loop-yield";

export interface RegisterGoalLoopOptions {
  /** Path to the adapter-owned goal-loop config data file, read fresh per settle. */
  goalLoopConfigPath: string;
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
   * Clears `leadCompactingRef` and both compaction markers (`pendingRearm`,
   * `settleSwallowedWhileCompacting`), cancels a pending settle timer and
   * boundary-guard fallback timer (260906 Phase 1), and clears
   * `leadReminderStartPendingRef`. Without this, a `session_shutdown`/
   * `/reload` that lands while a compaction is still in flight would leave
   * `leadCompactingRef.current` stuck `true` into the replacement session,
   * where `isOwningAgentIdle()` (spawner.ts) reports `false` forever and
   * every `followUp` push plus `ask.ts`'s `injectDiscussionSummary` is held
   * with nothing left to release them; and a stuck `leadReminderStartPendingRef`
   * would make every push into the replacement session append inert
   * (`triggerTurn: false`) with nothing left to consume it.
   */
  resetCompactionStateForShutdown(): void;
}

/**
 * Registers the `/goal` command, the `goal-achieved`/`goal-blocked`/
 * `goal-compact-and-continue` tools, and the
 * `tool_call`/`agent_settled`/`session_before_compact` listeners that drive
 * the goal-loop state machine above. Called at extension factory top level
 * (not inside `session_start`) — command/tool registration is declarative
 * here, same as every other command/tool in index.ts.
 */
export function registerGoalLoop(pi: ExtensionAPI, opts: RegisterGoalLoopOptions): GoalLoopShutdownHandle {
  let state: GoalLoopState = initialGoalLoopState();
  // 260906 (compaction push-hold ticket, Phase 1): true only between the
  // `goal-compact-and-continue` lever setting `leadCompactingRef` and the
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
   * handoff, and by the `agent_start` backstop — see each site below.
   */
  let settleSwallowedWhileCompacting = false;

  const scheduleTimer =
    opts.scheduleTimer ??
    ((cb: () => void, ms: number): NodeJS.Timeout => {
      const handle = setTimeout(cb, ms);
      handle.unref?.();
      return handle;
    });
  const clearTimer = opts.clearTimer ?? ((handle: NodeJS.Timeout) => clearTimeout(handle));

  /**
   * The single settle timer (260906 Phase 1, settle-timer reminder race
   * ticket): armed by every `agent_settled` that finds goal mode active and
   * not compacting, and by `releaseAfterCompaction`'s idle branch when a
   * lever reminder or a swallowed settle is pending release. `undefined`
   * whenever nothing is pending — the sole condition `resetCompactionStateForShutdown`
   * and the cancel points below need to check before clearing it.
   */
  let settleTimer: NodeJS.Timeout | undefined;

  /**
   * The boundary-guard's own fallback timer (260906 Phase 1): armed
   * alongside `leadReminderStartPendingRef.current = true`, right after the
   * reminder's own `sendUserMessage` call, so a reminder whose run never
   * actually started (an `agent_start`/`agent_settled` that should have
   * followed it never arrives within the settle delay) does not leave the
   * boundary guard — and every subsequent push's `triggerTurn` — stuck
   * `false` forever.
   */
  let reminderTimeoutHandle: NodeJS.Timeout | undefined;

  function cancelSettleTimer(): void {
    if (settleTimer !== undefined) {
      clearTimer(settleTimer);
      settleTimer = undefined;
    }
  }

  function cancelReminderTimeout(): void {
    if (reminderTimeoutHandle !== undefined) {
      clearTimer(reminderTimeoutHandle);
      reminderTimeoutHandle = undefined;
    }
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
   */
  function armSettleTimer(ctx: ExtensionContext): void {
    cancelSettleTimer();
    // 260906 Phase 1 review relay #1 (Minor): a live reminder-fallback timer
    // has nothing left to guard once the settle timer is (re-)armed — this
    // path is reached both from a live `agent_settled` (which already cleared
    // it) and from the fallback timeout's own retry arm (which is about to
    // become stale the instant this new settle timer takes over) and from
    // `releaseAfterCompaction` (which could otherwise arm a fresh settle
    // timer while an unrelated reminder fallback from an earlier cycle was
    // still ticking). Cancelling here, alongside `fireReminder`'s own cancel
    // below, makes "at most one timer of each kind" (settle timer, reminder
    // fallback) an enforced invariant rather than an accident of call order.
    cancelReminderTimeout();
    const config = readGoalLoopConfig(opts.goalLoopConfigPath);
    const delayMs = resolveSettleDelayMs(config);
    settleTimer = scheduleTimer(() => runTimerCallback(ctx, "settle timer", () => onSettleTimerFire(ctx)), delayMs);
    ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: settling");
  }

  /**
   * Sends the reinject reminder as a `followUp` turn and arms the
   * boundary-guard fallback timer (260906 Phase 1) — the single call site
   * for actually dispatching a reminder, used by both `onSettleTimerFire`'s
   * `pendingRearm` branch and its ordinary/replayed-settle `"reinject"`
   * branch, so the boundary-guard bookkeeping can never drift between them.
   * `followUp` is always correct here (not just for the replay case the
   * pre-timer code distinguished): the timer only ever fires this once
   * `ctx.isIdle()` is confirmed fresh, so a bare send would also work, but
   * `followUp` additionally survives a push that raced in and started a
   * turn between the fire-condition check and this call.
   */
  function fireReminder(ctx: ExtensionContext, config: GoalLoopConfig | undefined, reminder: string): void {
    // 260906 Phase 1 review relay #1 (Minor): cancel a leftover
    // reminder-fallback timer before arming a fresh one — see
    // `armSettleTimer`'s matching cancel for the full "at most one timer of
    // each kind" rationale. `onSettleTimerFire`'s three consumers
    // (`pendingRearm`, replayed `settleSwallowedWhileCompacting`, ordinary
    // reinject) all funnel through this one function, so this is the single
    // place that needs the cancel.
    cancelReminderTimeout();
    leadReminderStartPendingRef.current = true;
    if (state.pendingCarryForward !== undefined) {
      reminder += `\n\nCarried forward verbatim from before compaction:\n${state.pendingCarryForward}`;
    }
    pi.sendUserMessage(reminder, { deliverAs: "followUp" });
    // Consume only after dispatch returns: a synchronous send throw retains
    // the payload for the next eligible reminder, regardless of its origin.
    state.pendingCarryForward = undefined;
    const delayMs = resolveSettleDelayMs(config);
    reminderTimeoutHandle = scheduleTimer(
      () =>
        runTimerCallback(ctx, "reminder fallback timer", () => {
          reminderTimeoutHandle = undefined;
          if (leadReminderStartPendingRef.current) {
            leadReminderStartPendingRef.current = false;
            // 260906 Phase 1 review relay #1 (Minor): arm FIRST, set the retry
            // status LAST — `armSettleTimer`'s own first action overwrites the
            // status key with "Goal loop: settling", so setting the retry
            // string before arming made it unobservable (immediately
            // clobbered in the same synchronous tick). Reversing the order
            // lets the retry string actually be the status the footer shows
            // once this callback returns.
            armSettleTimer(ctx);
            ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, "Goal loop: reminder did not start a turn, retrying");
          }
        }),
      delayMs,
    );
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
      ctx.ui.notify(`Goal loop force-stopped: ${decision.reason}`, "warning");
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
      return;
    }
    const advisoryPercent = resolveCompactionAdvisoryPercent(config);
    const contextWindowOverride = resolveContextWindowOverride(config);
    const percent = computeContextPercent(ctx.getContextUsage(), contextWindowOverride);
    const reminder = buildGoalReminder(decision.goal, { percent, advisoryPercent });
    fireReminder(ctx, config, reminder);
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
  function onSettleTimerFire(ctx: ExtensionContext): void {
    settleTimer = undefined;

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
      return;
    }

    const notIdle = !ctx.isIdle();
    const yielding = hasRunningAgents(opts.rpcRegistryRef?.current);
    if (notIdle || yielding) return;

    const config = readGoalLoopConfig(opts.goalLoopConfigPath);

    if (pendingRearm) {
      pendingRearm = false;
      settleSwallowedWhileCompacting = false; // exactly one reminder for this settle
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
        reminder = `${failureReason} Do not retry goal-compact-and-continue — call goal-achieved or goal-blocked instead.\n${reminder}`;
      }
      fireReminder(ctx, config, reminder);
      return;
    }

    settleSwallowedWhileCompacting = false; // no-op if it was already false
    const threshold = resolveRunawayThreshold(config);
    const { next, decision } = decideOnSettle(state, threshold, false, false);
    state = next;
    dispatchSettleDecision(ctx, config, decision);
  }

  /**
   * Idempotent release of an in-flight compaction, deferred past Pi's own
   * compaction flag by every event-driven caller (never called synchronously
   * from inside a `session_*compact*` handler — see the two listeners
   * below). Three callers: the deferred `session_compact`/
   * `session_compact_failed` listeners, the lever's `onComplete`/`onError`
   * (backstop — covers the case where Pi never fires those events, e.g. a
   * compaction that fails before reaching them), and `agent_start` (pure
   * backstop clear, no reminder, no queue touch — see that listener below).
   *
   * Idle branch: flushes every held push (both ordinary family pushes and
   * `ask.ts`'s raw `ws-thread-summary` send). Then, if a lever-originated
   * reminder (`pendingRearm`) or a swallowed settle (`settleSwallowedWhileCompacting`)
   * is pending, arms the settle timer (260906 Phase 1) — it does NOT clear
   * either marker here; that moved to `onSettleTimerFire`, which needs both
   * markers to survive the arm-to-fire delay to know what to send once it
   * actually fires. Not-idle branch: sends nothing and clears both markers
   * (and the captured failure reason) — the agent has ALREADY started a
   * fresh turn by the time this deferred call lands (e.g. Pi's own
   * `compaction_end` flushed owner-queued input before `setImmediate`
   * fired), so the held queue and any pending reminder are left to that
   * turn's own `agent_settled` instead of racing it.
   */
  function releaseAfterCompaction(ctx: ExtensionContext, failureReason?: string): void {
    if (!leadCompactingRef.current) return; // idempotent: already released
    leadCompactingRef.current = false;
    if (!ctx.isIdle()) {
      // A fresh turn is already underway — leave everything to it rather
      // than racing it (see this function's doc comment for why this is NOT
      // the "agent_start already cleared the flag" case: that case returns
      // at the idempotency guard above, before ever reaching here).
      pendingRearm = false;
      settleSwallowedWhileCompacting = false;
      pendingRearmFailureReason = undefined;
      return;
    }
    flushHeldPushes(pi);
    if (pendingRearm) pendingRearmFailureReason = failureReason;
    if (pendingRearm || settleSwallowedWhileCompacting) armSettleTimer(ctx);
  }

  pi.registerCommand("goal", {
    description: "Arm goal mode: announce <goal> and re-inject a reminder on every settle until goal-achieved/goal-blocked is called.",
    handler: async (args, ctx) => {
      const goal = args.trim();
      if (!goal) {
        ctx.ui.notify("Usage: /goal <goal>", "warning");
        return;
      }
      if (!ctx.isIdle()) {
        ctx.ui.notify("Agent is busy — try again when idle.", "warning");
        return;
      }
      cancelSettleTimer(); // re-arm cancel point (260906 Phase 1)
      state = armGoal(goal);
      pi.sendUserMessage(buildGoalAnnouncement(goal));
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
    leadReminderStartPendingRef.current = false;
    cancelReminderTimeout();

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
  pi.on("agent_start", (_event, ctx) => {
    // 260906 Phase 1 (settle-timer reminder race ticket): cancel points
    // "on agent_start" for the settle timer, the boundary-guard fallback
    // timer, and the boundary guard itself — unconditional, before every
    // check below, since a fresh turn starting is proof neither has
    // anything left to guard against.
    cancelSettleTimer();
    cancelReminderTimeout();
    leadReminderStartPendingRef.current = false;

    // 260906 (compaction push-hold ticket, Phase 1): backstop clear — runs
    // before the goal-mode-only checks below because `leadCompactingRef` can
    // be set defensively by `session_before_compact` for ANY compaction
    // reason regardless of whether a goal is active, and a fresh
    // `agent_start` is proof the session has moved on from whatever
    // compaction set it. No reminder, no queue touch here: that is
    // `releaseAfterCompaction`'s job, and this branch only fires when THAT
    // never ran (e.g. `session_compact`/`session_compact_failed` never fired
    // for this compaction) — a pure safety net against a stuck flag.
    if (leadCompactingRef.current) {
      leadCompactingRef.current = false;
      // Review relay #1 (Important/Critical): both markers must die with the
      // stuck flag they rode in on — otherwise a stale `pendingRearm` makes
      // the NEXT, unrelated compaction synthesize a lever reminder that was
      // never requested, and a stale `settleSwallowedWhileCompacting` makes
      // it replay a settle a second time.
      pendingRearm = false;
      settleSwallowedWhileCompacting = false;
      pendingRearmFailureReason = undefined;
      // Review relay #1 (Minor): clear the footer status BEFORE returning —
      // this turn is proof the loop moved on, so leaving a stale "waiting for
      // compaction"/"settling" footer up for its whole duration would be a
      // lie the next branch's unconditional clear was supposed to have
      // already told.
      ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
      return;
    }
    if (isChildProcess(process.env)) return;
    if (!state.active) return;
    ctx.ui.setStatus(GOAL_LOOP_YIELD_STATUS_KEY, undefined);
  });

  // Observe-only: never `cancel`s, never supplies a `compaction` override.
  // Fires for both our own `/goal-compact-and-continue`-triggered manual
  // compaction (reason: "manual") and Pi's own overflow/threshold
  // auto-compaction backstop — matching the ticket's resolved "not an
  // extension gate" design (see this file's top-of-file doc comment).
  //
  // 260906 (compaction push-hold ticket, Phase 1): sets `leadCompactingRef`
  // unconditionally, as the very first line, for ANY compaction reason (the
  // lever already set it before calling `ctx.compact()`, so this is
  // defensive coverage for an owner-typed `/compact` and Pi's own
  // threshold/overflow auto-compaction, neither of which goes through the
  // lever). The advisory `ctx.ui.notify` stays gated on goal mode being
  // active, unchanged.
  pi.on("session_before_compact", (event, ctx) => {
    leadCompactingRef.current = true;
    if (isChildProcess(process.env)) return;
    if (!state.active || !state.goal) return;
    ctx.ui.notify(buildCompactionObservation(state.goal, event.reason), "info");
  });

  // 260906 (compaction push-hold ticket, Phase 1): deferred via `setImmediate`
  // so neither handler ever sends a prompt from inside a `session_*compact*`
  // event — `session_compact` fires while Pi's own
  // `_compactionAbortController` is still non-`undefined` (cleared right
  // after), so anything synchronous here would race that same internal
  // state Pi has not finished unwinding yet.
  pi.on("session_compact", (_event, ctx) => {
    // Review relay #2 (Minor, accepted narrow gap): unlike the lever, a
    // non-lever compaction has no `onComplete`/`onError` backstop — if Pi's
    // internal `savedCompactionEntry` lookup ever misses this event outright,
    // only `agent_start`'s backstop can still clear a stuck flag/marker.
    setImmediate(() => releaseAfterCompaction(ctx));
  });
  pi.on("session_compact_failed", (event, ctx) => {
    // `event.errorMessage` is passed through as-is: Pi already formats it
    // as `"Compaction failed: …"` / `"Auto-compaction failed: …"` / `"Context
    // overflow recovery failed: …"`, so `releaseAfterCompaction` must not
    // add its own prefix on top (Review relay #1, Minor).
    setImmediate(() => releaseAfterCompaction(ctx, event.errorMessage));
  });

  pi.registerTool({
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
      cancelSettleTimer(); // cancel point (260906 Phase 1): terminal lever
      state = disarmGoal();
      return { content: [{ type: "text", text: `Goal achieved: ${p.summary}` }] };
    },
  });

  pi.registerTool({
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
      cancelSettleTimer(); // cancel point (260906 Phase 1): terminal lever
      state = disarmGoal();
      return { content: [{ type: "text", text: `Goal blocked: ${p.reason}` }] };
    },
  });

  pi.registerTool({
    name: "goal-compact-and-continue",
    label: "goal-compact-and-continue",
    description:
      "Non-terminal lever: compact context now, using <carry_forward> prose to steer the summary and delivering it verbatim once in the next eligible goal reminder, then continue pursuing the same active goal. Call this at a safe compaction point (phase boundary / merge gate) instead of manually summarizing progress in prose.",
    parameters: {
      type: "object",
      properties: {
        carry_forward: { type: "string", description: "Prose passed unchanged as compaction custom instructions and carried verbatim into the next eligible goal reminder." },
      },
      required: ["carry_forward"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, ctx: ExtensionContext) {
      const p = params as { carry_forward: string };
      // Does NOT call disarmGoal() — non-terminal. ctx.compact() aborts the
      // in-flight turn (the one that invoked this very tool call) and, once
      // compaction completes, `releaseAfterCompaction` arms the settle timer
      // for a fresh reminder — no separate manual "continue" call is
      // needed here beyond returning this tool's own result and triggering
      // the compaction (see this file's top-of-file doc comment's
      // risk-signal note). 260906 review relay #1 (Critical): the invoking
      // turn's own abort-produced settle is judged `waiting` and swallowed
      // (see `settleSwallowedWhileCompacting`) — `pendingRearm` below is what
      // actually re-arms the loop, not that settle's own reinject path.
      //
      // 260906 (compaction push-hold ticket, Phase 1): marks this compaction
      // as LEVER-ORIGINATED (`pendingRearm`) before calling `ctx.compact`, so
      // `releaseAfterCompaction` knows to synthesize a re-armed reminder once
      // it finishes — set BEFORE the call since `ctx.compact()`'s own internal
      // abort can settle the invoking turn synchronously within this call.
      leadCompactingRef.current = true;
      pendingRearm = true;
      // Goal-scoped, not rearm-marker-scoped: busy release and agent_start
      // may clear those markers before an ordinary reminder can carry this.
      state.pendingCarryForward = p.carry_forward;
      ctx.compact({
        customInstructions: p.carry_forward,
        onComplete: () => {
          ctx.ui.notify("Compaction completed", "info");
          releaseAfterCompaction(ctx);
        },
        onError: (error) => {
          // Review relay #1 (Minor): the "Compaction failed: " prefix is
          // applied HERE, at the lever's own call site — `error.message` is a
          // raw, unprefixed string, unlike `SessionCompactFailedEvent.errorMessage`
          // (already Pi-formatted; see the `session_compact_failed` listener
          // below), so `releaseAfterCompaction` must not add a prefix of its
          // own or a non-lever failure would double it.
          const failureReason = `Compaction failed: ${error.message}`;
          ctx.ui.notify(failureReason, "error");
          releaseAfterCompaction(ctx, failureReason);
        },
      });
      return { content: [{ type: "text", text: buildCompactionLeverResult(p.carry_forward) }] };
    },
  });

  return {
    resetCompactionStateForShutdown() {
      state.pendingCarryForward = undefined;
      leadCompactingRef.current = false;
      pendingRearm = false;
      settleSwallowedWhileCompacting = false;
      pendingRearmFailureReason = undefined;
      // 260906 Phase 1 (settle-timer reminder race ticket): cancel point
      // "session shutdown" — a replacement session must not inherit a
      // pending settle/boundary-guard timer from the torn-down one.
      cancelSettleTimer();
      cancelReminderTimeout();
      leadReminderStartPendingRef.current = false;
    },
  };
}
