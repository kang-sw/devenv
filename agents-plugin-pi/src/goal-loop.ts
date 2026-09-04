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
 * is adapter-owned data-file config (`goal-loop-config.json`, sibling to
 * `model-catalog.json`), a built-in constant default overridden by a file
 * read fresh on every use — mirrors `model-catalog.ts`'s
 * never-hard-fail/no-caching convention exactly. Never lives in ws-mcp (Go
 * core) — the goal-loop is entirely adapter-local (golden rule).
 *
 * Settled cross-ticket fact: the goal-loop runs on the lead session only.
 * The `agent_settled` handler no-ops when the running process is itself a
 * spawned child (`WS_PI_AGENT_CHILD_ENV` set — see spawner.ts's
 * `buildRpcClientOptions`/`spawnPiProcess`, both of which now carry this env
 * marker on every spawned child) — defense-in-depth against a message that
 * happens to start with `/goal …` reaching a child's input pipeline (e.g. a
 * lead-authored `ws-agent-send` message), even though each spawned child
 * loads this same extension fresh with its own inert module-level state.
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
import { WS_PI_AGENT_CHILD_ENV } from "./spawner.ts";

// ---------------------------------------------------------------------------
// Config: adapter-owned runaway-threshold data file, sibling to
// model-catalog.json. Copies readModelCatalog's exact never-hard-fail,
// read-fresh-per-call shape.
// ---------------------------------------------------------------------------

export interface GoalLoopConfig {
  runaway_threshold?: number;
  /** Advisory context-usage percent (0, 100] surfaced in the reinject reminder as a nudge point — not a gate. */
  compaction_advisory_percent?: number;
  /** Optional context-window token override for `computeContextPercent`, used when the model's own `getContextUsage().contextWindow` should be superseded. */
  context_window_override?: number;
}

/** Default number of consecutive no-tool-call re-fires before the loop force-stops, absent (or overridden by) a config file. */
export const DEFAULT_RUNAWAY_THRESHOLD = 10;

/** Default advisory context-usage percent (adapter-chosen, no ticket-pinned value; config-tunable) surfaced in the reinject reminder. */
export const DEFAULT_COMPACTION_ADVISORY_PERCENT = 70;

/**
 * Reads and parses the goal-loop config data file. Returns `undefined` —
 * never throws — when the file is missing, unreadable, or not valid JSON, so
 * "unset" (fall back to `DEFAULT_RUNAWAY_THRESHOLD`) is the expected default
 * state. Read fresh on every call by design (no module-level caching),
 * mirroring `model-catalog.ts#readModelCatalog` exactly.
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

// ---------------------------------------------------------------------------
// Pure message builders.
// ---------------------------------------------------------------------------

/** The goal-entry announcement injected by `/goal <goal>`. Wording pinned verbatim by the ticket. */
export function buildGoalAnnouncement(goal: string): string {
  return `Goal settled: ${goal}`;
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
        : `Context usage: ${Math.round(percent)}% of window.`;
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
 * Pure predicate: `true` when `env` carries the spawned-child marker
 * (`WS_PI_AGENT_CHILD_ENV`, set by spawner.ts on every spawned child's
 * process environment). Extracted from the `agent_settled` handler (review
 * fix, cycle 1) — mirroring `decideOnSettle`'s own pure-reducer extraction —
 * so the "no-op in a spawned child" guard is unit-testable without spawning
 * a real process.
 */
export function isChildProcess(env: NodeJS.ProcessEnv): boolean {
  return Boolean(env[WS_PI_AGENT_CHILD_ENV]);
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
  | { action: "reinject"; goal: string }
  | { action: "force-stop"; reason: string };

/**
 * Pure reducer for an `agent_settled` firing: decides whether to ignore
 * (goal mode inactive), re-inject a reminder (under threshold), or
 * force-stop (streak reached `threshold`). Streak resets to 0 whenever a
 * tool call happened this cycle; otherwise it increments.
 *
 * The `"reinject"` decision carries only the bare `goal` string, not a
 * precomputed reminder: this reducer has no access to `ctx.getContextUsage()`
 * or the goal-loop config file (both IO-context-only), so `buildGoalReminder`
 * is called by `registerGoalLoop`'s IO glue instead, right where that context
 * is already available (Phase 2, 260903).
 */
export function decideOnSettle(state: GoalLoopState, threshold: number): { next: GoalLoopState; decision: SettleDecision } {
  if (!state.active) {
    return { next: state, decision: { action: "ignore" } };
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

export interface RegisterGoalLoopOptions {
  /** Path to the adapter-owned goal-loop config data file, read fresh per settle. */
  goalLoopConfigPath: string;
}

/**
 * Registers the `/goal` command, the `goal-achieved`/`goal-blocked`/
 * `goal-compact-and-continue` tools, and the
 * `tool_call`/`agent_settled`/`session_before_compact` listeners that drive
 * the goal-loop state machine above. Called at extension factory top level
 * (not inside `session_start`) — command/tool registration is declarative
 * here, same as every other command/tool in index.ts.
 */
export function registerGoalLoop(pi: ExtensionAPI, opts: RegisterGoalLoopOptions): void {
  let state: GoalLoopState = initialGoalLoopState();

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
    // Defense-in-depth: never re-fire inside a spawned child process (the
    // goal-loop is lead-session-only per the ticket's settled cross-ticket
    // fact). Each child also starts with its own inert module-level state,
    // but this guard protects against a `/goal …`-prefixed message reaching
    // a child's input pipeline regardless.
    if (isChildProcess(process.env)) return;

    const config = readGoalLoopConfig(opts.goalLoopConfigPath);
    const threshold = resolveRunawayThreshold(config);
    const { next, decision } = decideOnSettle(state, threshold);
    state = next;

    if (decision.action === "ignore") return;
    if (decision.action === "force-stop") {
      ctx.ui.notify(`Goal loop force-stopped: ${decision.reason}`, "warning");
      return;
    }
    const advisoryPercent = resolveCompactionAdvisoryPercent(config);
    const contextWindowOverride = resolveContextWindowOverride(config);
    const percent = computeContextPercent(ctx.getContextUsage(), contextWindowOverride);
    pi.sendUserMessage(buildGoalReminder(decision.goal, { percent, advisoryPercent }));
  });

  // Observe-only: never `cancel`s, never supplies a `compaction` override.
  // Fires for both our own `/goal-compact-and-continue`-triggered manual
  // compaction (reason: "manual") and Pi's own overflow/threshold
  // auto-compaction backstop — matching the ticket's resolved "not an
  // extension gate" design (see this file's top-of-file doc comment).
  pi.on("session_before_compact", (event, ctx) => {
    if (isChildProcess(process.env)) return;
    if (!state.active || !state.goal) return;
    ctx.ui.notify(buildCompactionObservation(state.goal, event.reason), "info");
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
      state = disarmGoal();
      return { content: [{ type: "text", text: `Goal blocked: ${p.reason}` }] };
    },
  });

  pi.registerTool({
    name: "goal-compact-and-continue",
    label: "goal-compact-and-continue",
    description:
      "Non-terminal lever: compact context now, carrying <carry_forward> prose into the compaction summary, then continue pursuing the same active goal. Call this at a safe compaction point (phase boundary / merge gate) instead of manually summarizing progress in prose.",
    parameters: {
      type: "object",
      properties: {
        carry_forward: { type: "string", description: "Prose carried forward into the compaction summary as custom instructions." },
      },
      required: ["carry_forward"],
    } as never,
    async execute(_toolCallId, params, _signal, _onUpdate, ctx: ExtensionContext) {
      const p = params as { carry_forward: string };
      // Does NOT call disarmGoal() — non-terminal. ctx.compact() aborts the
      // in-flight turn (the one that invoked this very tool call) and, once
      // compaction completes, the resulting fresh settle re-enters through
      // the existing armed `agent_settled` reinject path above — no separate
      // manual "continue" call is needed here beyond returning this tool's
      // own result and triggering the compaction (see this file's top-of-file
      // doc comment's risk-signal note).
      ctx.compact({
        customInstructions: p.carry_forward,
        onComplete: () => ctx.ui.notify("Compaction completed", "info"),
        onError: (error) => ctx.ui.notify(`Compaction failed: ${error.message}`, "error"),
      });
      return { content: [{ type: "text", text: `Compacting and continuing goal with carry-forward: ${p.carry_forward}` }] };
    },
  });
}
