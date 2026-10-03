/**
 * Unit tests for goal-loop.ts's pure exports (threshold/
 * advisory-percent/context-window-override resolvers, message builders, the
 * `computeContextPercent` context-usage helper, the state machine, and the
 * spawned-child `isChildProcess` predicate — review fix, cycle 1: extracted
 * from the `agent_settled` handler's inline `process.env` check so the
 * lead-session-only guard has automated positive/negative coverage instead
 * of relying solely on a manual spot-check). No `pi.*` IO is exercised here
 * — `registerGoalLoop`'s IO glue is driven through fake `pi`/`ctx` seams in
 * the suites below; the summary builders themselves are covered by
 * `test/lead-compaction.test.ts`. The companion env-marker-placement coverage
 * (`buildRpcClientOptions`) lives in
 * `test/spawner.test.ts`.
 *
 * Phase 2 (260903) note: `decideOnSettle`'s `"reinject"` decision now carries
 * only the bare `goal` string, not a precomputed reminder — see that
 * function's own doc comment in goal-loop.ts for why (`ctx.getContextUsage()`/
 * the adapter config are IO-context-only). The old embedded-reminder assertions
 * below were updated to the new `{ action: "reinject", goal }` shape.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, after, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SessionManager, type ExtensionAPI, type ExtensionContext, type SessionEntry } from "@earendil-works/pi-coding-agent";
import {
  resolveAgentWaitAnimation,
  resolveRunawayThreshold,
  resolveCompactionAdvisoryPercent,
  resolveContextWindowOverride,
  resolveSettleDelayMs,
  resolveChildRetentionTtlDays,
  computeContextPercent,
  buildGoalAnnouncement,
  buildCompactionLeverResult,
  buildGoalReminder,
  buildCompactionObservation,
  initialGoalLoopState,
  armGoal,
  disarmGoal,
  recordToolCall,
  decideOnSettle,
  isChildProcess,
  registerGoalLoop,
  buildLeadCompactionResult,
  resolveDialogBudgetBytes,
  DEFAULT_RUNAWAY_THRESHOLD,
  DEFAULT_COMPACTION_ADVISORY_PERCENT,
  DEFAULT_SETTLE_DELAY_MS,
  DEFAULT_CHILD_RETENTION_TTL_DAYS,
  type GoalLoopConfig,
} from "../src/goal-loop.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { createWsConfigReader, staticConfigReader, type GoalLoopConfigReader } from "../src/adapter-config.ts";
import { flushHeldPushes, leadIdleRef, clearWakeStart, leadCompactingRef, leadWakeStartPendingRef, heldPushQueue, isOwningAgentIdle, type RpcAgentRegistry } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { DEFAULT_DIALOG_BUDGET_BYTES, NO_KEPT_ENTRY_ID, renderLeadProse } from "../src/lead-compaction.ts";

/** The lever's rendered prose for a single `current_work` field — the carry the old lever passed raw. */
const prose = (text: string): string => renderLeadProse({ current_work: text });

/** A `session_before_compact` event with Pi's preparation fields the lead handler reads. */
function compactionEvent(reason: "manual" | "threshold" | "overflow", branchEntries: unknown[] = [], extra: Record<string, unknown> = {}): never {
  return {
    type: "session_before_compact",
    reason,
    branchEntries,
    willRetry: false,
    signal: new AbortController().signal,
    preparation: {
      firstKeptEntryId: "kept-entry",
      tokensBefore: 1234,
      messagesToSummarize: [],
      turnPrefixMessages: [],
      isSplitTurn: false,
      fileOps: { read: new Set(["/tmp/read.ts"]), written: new Set(), edited: new Set(["/tmp/edit.ts"]) },
      settings: { enabled: true, reserveTokens: 16384, keepRecentTokens: 20000 },
    },
    ...extra,
  } as never;
}

const tmpDir = mkdtempSync(join(tmpdir(), "ws-goal-loop-test-"));
after(() => {
  rmSync(tmpDir, { recursive: true, force: true });
});

function writeConfig(name: string, contents: string): string {
  const path = join(tmpDir, name);
  writeFileSync(path, contents, "utf8");
  return path;
}

/**
 * A synchronous reader over a JSON file, re-read at each use: the test
 * stand-in for ws-mcp's store, so rewriting the file mid-test is a "tuned
 * value" the next read sees. A missing or malformed file reads as nothing
 * tuned, as an unreachable ws-mcp does.
 */
function fileReader(path: string): GoalLoopConfigReader {
  return (keys) => {
    let config: GoalLoopConfig = {};
    try {
      config = JSON.parse(readFileSync(path, "utf8")) as GoalLoopConfig;
    } catch {
      // nothing tuned
    }
    return staticConfigReader(config)(keys);
  };
}

describe("resolveAgentWaitAnimation", () => {
  test("defaults to enabled and only literal false disables the owner-wait cue", () => {
    assert.equal(resolveAgentWaitAnimation(undefined), true);
    assert.equal(resolveAgentWaitAnimation({}), true);
    assert.equal(resolveAgentWaitAnimation({ agent_wait_animation: true }), true);
    assert.equal(resolveAgentWaitAnimation({ agent_wait_animation: false }), false);
    assert.equal(resolveAgentWaitAnimation({ agent_wait_animation: "false" as unknown as boolean }), true);
  });
});

describe("resolveRunawayThreshold", () => {
  test("undefined config falls back to the default", () => {
    assert.equal(resolveRunawayThreshold(undefined), DEFAULT_RUNAWAY_THRESHOLD);
  });

  test("empty config falls back to the default", () => {
    assert.equal(resolveRunawayThreshold({}), DEFAULT_RUNAWAY_THRESHOLD);
  });

  test("a valid positive override is used", () => {
    assert.equal(resolveRunawayThreshold({ runaway_threshold: 2 }), 2);
  });

  test("zero falls back to the default (never hard-fail)", () => {
    assert.equal(resolveRunawayThreshold({ runaway_threshold: 0 }), DEFAULT_RUNAWAY_THRESHOLD);
  });

  test("a negative value falls back to the default", () => {
    assert.equal(resolveRunawayThreshold({ runaway_threshold: -5 }), DEFAULT_RUNAWAY_THRESHOLD);
  });

  test("a non-numeric value falls back to the default", () => {
    assert.equal(resolveRunawayThreshold({ runaway_threshold: "5" as unknown as number }), DEFAULT_RUNAWAY_THRESHOLD);
  });

  test("NaN/Infinity fall back to the default", () => {
    assert.equal(resolveRunawayThreshold({ runaway_threshold: Number.NaN }), DEFAULT_RUNAWAY_THRESHOLD);
    assert.equal(resolveRunawayThreshold({ runaway_threshold: Number.POSITIVE_INFINITY }), DEFAULT_RUNAWAY_THRESHOLD);
  });
});

describe("resolveChildRetentionTtlDays", () => {
  test("defaults invalid or missing values to 30 days", () => {
    assert.equal(resolveChildRetentionTtlDays(undefined), DEFAULT_CHILD_RETENTION_TTL_DAYS);
    assert.equal(resolveChildRetentionTtlDays({}), DEFAULT_CHILD_RETENTION_TTL_DAYS);
    for (const value of [-1, Number.NaN, Number.POSITIVE_INFINITY, "30", true, false, null]) {
      assert.equal(resolveChildRetentionTtlDays({ child_retention_ttl_days: value as never }), DEFAULT_CHILD_RETENTION_TTL_DAYS);
    }
  });

  test("accepts positive days and 0 disables pruning", () => {
    assert.equal(resolveChildRetentionTtlDays({ child_retention_ttl_days: 45 }), 45);
    assert.equal(resolveChildRetentionTtlDays({ child_retention_ttl_days: 0 }), false);
  });
});

describe("resolveCompactionAdvisoryPercent", () => {
  test("undefined config falls back to the default", () => {
    assert.equal(resolveCompactionAdvisoryPercent(undefined), DEFAULT_COMPACTION_ADVISORY_PERCENT);
  });

  test("empty config falls back to the default", () => {
    assert.equal(resolveCompactionAdvisoryPercent({}), DEFAULT_COMPACTION_ADVISORY_PERCENT);
  });

  test("a valid override in (0, 100] is used", () => {
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: 55 }), 55);
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: 100 }), 100);
  });

  test("zero falls back to the default (never hard-fail)", () => {
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: 0 }), DEFAULT_COMPACTION_ADVISORY_PERCENT);
  });

  test("a negative value falls back to the default", () => {
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: -5 }), DEFAULT_COMPACTION_ADVISORY_PERCENT);
  });

  test("a value above 100 falls back to the default", () => {
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: 101 }), DEFAULT_COMPACTION_ADVISORY_PERCENT);
  });

  test("a non-numeric value falls back to the default", () => {
    assert.equal(
      resolveCompactionAdvisoryPercent({ compaction_advisory_percent: "50" as unknown as number }),
      DEFAULT_COMPACTION_ADVISORY_PERCENT,
    );
  });

  test("NaN/Infinity fall back to the default", () => {
    assert.equal(resolveCompactionAdvisoryPercent({ compaction_advisory_percent: Number.NaN }), DEFAULT_COMPACTION_ADVISORY_PERCENT);
    assert.equal(
      resolveCompactionAdvisoryPercent({ compaction_advisory_percent: Number.POSITIVE_INFINITY }),
      DEFAULT_COMPACTION_ADVISORY_PERCENT,
    );
  });
});

describe("resolveContextWindowOverride", () => {
  test("undefined config returns undefined (no override)", () => {
    assert.equal(resolveContextWindowOverride(undefined), undefined);
  });

  test("empty config returns undefined", () => {
    assert.equal(resolveContextWindowOverride({}), undefined);
  });

  test("a valid positive override is used", () => {
    assert.equal(resolveContextWindowOverride({ context_window_override: 128_000 }), 128_000);
  });

  test("zero returns undefined (never hard-fail)", () => {
    assert.equal(resolveContextWindowOverride({ context_window_override: 0 }), undefined);
  });

  test("a negative value returns undefined", () => {
    assert.equal(resolveContextWindowOverride({ context_window_override: -1 }), undefined);
  });

  test("a non-numeric value returns undefined", () => {
    assert.equal(resolveContextWindowOverride({ context_window_override: "128000" as unknown as number }), undefined);
  });

  test("NaN/Infinity return undefined", () => {
    assert.equal(resolveContextWindowOverride({ context_window_override: Number.NaN }), undefined);
    assert.equal(resolveContextWindowOverride({ context_window_override: Number.POSITIVE_INFINITY }), undefined);
  });
});

describe("resolveSettleDelayMs", () => {
  test("undefined config falls back to the default", () => {
    assert.equal(resolveSettleDelayMs(undefined), DEFAULT_SETTLE_DELAY_MS);
  });

  test("empty config falls back to the default", () => {
    assert.equal(resolveSettleDelayMs({}), DEFAULT_SETTLE_DELAY_MS);
  });

  test("a valid positive override is used", () => {
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: 1000 }), 1000);
  });

  test("zero falls back to the default (never hard-fail)", () => {
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: 0 }), DEFAULT_SETTLE_DELAY_MS);
  });

  test("a negative value falls back to the default", () => {
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: -5 }), DEFAULT_SETTLE_DELAY_MS);
  });

  test("a non-numeric value falls back to the default", () => {
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: "5000" as unknown as number }), DEFAULT_SETTLE_DELAY_MS);
  });

  test("NaN/Infinity fall back to the default", () => {
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: Number.NaN }), DEFAULT_SETTLE_DELAY_MS);
    assert.equal(resolveSettleDelayMs({ settle_delay_ms: Number.POSITIVE_INFINITY }), DEFAULT_SETTLE_DELAY_MS);
  });
});

describe("computeContextPercent", () => {
  test("undefined usage returns null", () => {
    assert.equal(computeContextPercent(undefined), null);
  });

  test("null percent and null tokens returns null", () => {
    assert.equal(computeContextPercent({ tokens: null, contextWindow: 100_000, percent: null }), null);
  });

  test("null percent with known tokens recomputes from usage.contextWindow", () => {
    const result = computeContextPercent({ tokens: 25_000, contextWindow: 100_000, percent: null });
    assert.equal(result, 25);
  });

  test("null percent with known tokens and an override recomputes from the override", () => {
    const result = computeContextPercent({ tokens: 25_000, contextWindow: 100_000, percent: null }, 50_000);
    assert.equal(result, 50);
  });

  test("known percent with no override returns percent as-is", () => {
    assert.equal(computeContextPercent({ tokens: 25_000, contextWindow: 100_000, percent: 25 }), 25);
  });

  test("known percent with an override present recomputes from tokens/override, ignoring the stale percent", () => {
    const result = computeContextPercent({ tokens: 25_000, contextWindow: 100_000, percent: 25 }, 50_000);
    assert.equal(result, 50);
  });
});

describe("buildGoalAnnouncement", () => {
  test("wraps the goal verbatim per the ticket's pinned wording", () => {
    assert.equal(buildGoalAnnouncement("ship the widget"), "Goal armed: ship the widget");
  });
});

describe("buildCompactionLeverResult", () => {
  test("names the requested compaction without echoing the prose the summary already carries", () => {
    const text = buildCompactionLeverResult();
    assert.ok(text.startsWith("Compaction requested; the conversation will resume from a summary carrying"));
    assert.match(text, /fixed headings/);
  });
});

describe("buildGoalReminder", () => {
  const info = { percent: 42, advisoryPercent: 50 };

  test("names the goal and all three lever tool names (two terminal, one compaction lever)", () => {
    const reminder = buildGoalReminder("ship the widget", info);
    assert.match(reminder, /ship the widget/);
    assert.match(reminder, /goal-achieved/);
    assert.match(reminder, /goal-blocked/);
    assert.match(reminder, /ws-compact/);
    assert.doesNotMatch(reminder, /goal-compact-and-continue/);
  });

  test("mentions the runaway force-stop caveat", () => {
    const reminder = buildGoalReminder("anything", info);
    assert.match(reminder, /force-stop/);
  });

  test("mentions the compression-safety heuristic advisory (phase-boundary vs non-phase)", () => {
    const reminder = buildGoalReminder("anything", info);
    assert.match(reminder, /phase-boundary/);
    assert.match(reminder, /advisory/i);
  });

  test("percent below the advisory point explicitly tells the model not to compact", () => {
    const reminder = buildGoalReminder("a goal", { percent: 42, advisoryPercent: 50 });
    assert.match(reminder, /Context usage: 42% of window — below the compaction advisory point \(50%\); do not call ws-compact\.$/m);
  });

  test("percent at the advisory point prioritizes compact-and-continue for weakly related next work", () => {
    const reminder = buildGoalReminder("a goal", { percent: 50, advisoryPercent: 50 });
    assert.match(reminder, /Context usage: 50% of window — at or above the advisory point/);
    assert.match(reminder, /prioritize ws-compact when the next work is weakly related to the current context/);
  });

  test("percent above the advisory point renders the stronger nudge phrase", () => {
    const reminder = buildGoalReminder("a goal", { percent: 85, advisoryPercent: 50 });
    assert.match(reminder, /Context usage: 85% of window — at or above the advisory point/);
  });

  test("null percent renders as unknown, not a crash", () => {
    const reminder = buildGoalReminder("a goal", { percent: null, advisoryPercent: 50 });
    assert.match(reminder, /Context usage: unknown\./);
  });
});

describe("buildCompactionObservation", () => {
  test("names the goal and the reason for each reason value", () => {
    for (const reason of ["manual", "threshold", "overflow"] as const) {
      const observation = buildCompactionObservation("ship the widget", reason);
      assert.match(observation, /ship the widget/);
      assert.match(observation, new RegExp(`reason: ${reason}`));
    }
  });

  test("is a plain observation line", () => {
    const observation = buildCompactionObservation("a goal", "threshold");
    assert.equal(observation, 'Compaction observed while goal-loop is active (goal: "a goal", reason: threshold).');
  });
});

describe("initialGoalLoopState / armGoal / disarmGoal", () => {
  test("initial state is inactive with a zeroed streak", () => {
    assert.deepEqual(initialGoalLoopState(), { active: false, noToolCallStreak: 0, sawToolCallThisCycle: false });
  });

  test("armGoal always returns a fresh active state, replacing any prior goal", () => {
    const armed = armGoal("first goal");
    assert.deepEqual(armed, { active: true, goal: "first goal", noToolCallStreak: 0, sawToolCallThisCycle: false });
    const rearmed = armGoal("second goal");
    assert.deepEqual(rearmed, { active: true, goal: "second goal", noToolCallStreak: 0, sawToolCallThisCycle: false });
  });

  test("disarmGoal returns to the initial inactive state", () => {
    const armed = armGoal("a goal");
    assert.deepEqual(disarmGoal(), initialGoalLoopState());
    // disarming does not mutate the previously-armed state object
    assert.equal(armed.active, true);
  });
});

describe("recordToolCall", () => {
  test("is a no-op when goal mode is inactive", () => {
    const state = initialGoalLoopState();
    // Reference identity (assert.equal), not just structural equality — the
    // inactive branch must return the SAME object, not an equivalent clone
    // (matches decideOnSettle's analogous "ignores an inactive state" test).
    assert.equal(recordToolCall(state), state);
  });

  test("sets sawToolCallThisCycle when active", () => {
    const state = armGoal("a goal");
    const next = recordToolCall(state);
    assert.equal(next.sawToolCallThisCycle, true);
    assert.equal(next.active, true);
    assert.equal(next.goal, "a goal");
  });
});

describe("decideOnSettle", () => {
  test("ignores an inactive state, leaving it unchanged", () => {
    const state = initialGoalLoopState();
    const { next, decision } = decideOnSettle(state, DEFAULT_RUNAWAY_THRESHOLD);
    assert.deepEqual(decision, { action: "ignore" });
    assert.equal(next, state);
  });

  test("reinjects and increments the streak while under threshold", () => {
    const state = armGoal("a goal");
    const { next, decision } = decideOnSettle(state, 10);
    // Phase 2: the reinject decision carries only the bare goal string, not a
    // precomputed reminder — decideOnSettle has no access to
    // ctx.getContextUsage()/config, so buildGoalReminder is called by the IO
    // glue (registerGoalLoop's agent_settled handler) instead.
    assert.deepEqual(decision, { action: "reinject", goal: "a goal" });
    assert.equal(next.noToolCallStreak, 1);
    assert.equal(next.sawToolCallThisCycle, false);
    assert.equal(next.active, true);
  });

  test("a tool call this cycle resets the streak to 0 on the next settle", () => {
    let state = armGoal("a goal");
    state = decideOnSettle(state, 10).next; // streak 1
    state = decideOnSettle(state, 10).next; // streak 2
    assert.equal(state.noToolCallStreak, 2);
    state = recordToolCall(state);
    const { next, decision } = decideOnSettle(state, 10);
    assert.equal(next.noToolCallStreak, 0);
    assert.deepEqual(decision, { action: "reinject", goal: "a goal" });
  });

  test("force-stops and fully resets exactly at threshold", () => {
    let state = armGoal("a goal");
    const threshold = 2;
    let decision;
    ({ next: state, decision } = decideOnSettle(state, threshold)); // streak 1 -> reinject
    assert.deepEqual(decision, { action: "reinject", goal: "a goal" });
    ({ next: state, decision } = decideOnSettle(state, threshold)); // streak 2 -> force-stop
    assert.deepEqual(decision, { action: "force-stop", reason: "2 consecutive re-fires with no tool call" });
    assert.deepEqual(state, initialGoalLoopState());
  });

  test("force-stop resets state so a subsequent settle is ignored (not re-armed)", () => {
    let state = armGoal("a goal");
    const threshold = 1;
    let decision;
    ({ next: state, decision } = decideOnSettle(state, threshold));
    assert.equal(decision.action, "force-stop");
    ({ next: state, decision } = decideOnSettle(state, threshold));
    assert.deepEqual(decision, { action: "ignore" });
  });

  test("Phase 2 (260905): yielding on an active goal neither re-injects nor advances the streak", () => {
    const state = armGoal("a goal");
    const { next, decision } = decideOnSettle(state, 10, true);
    assert.deepEqual(decision, { action: "yield" });
    // Reference identity, not just structural equality — no streak
    // increment, no sawToolCallThisCycle reset: a true no-op pass-through.
    assert.equal(next, state);
  });

  test("Phase 2 (260905): yielding on an INACTIVE goal still ignores — yielding never resurrects an inactive loop", () => {
    const state = initialGoalLoopState();
    const { next, decision } = decideOnSettle(state, 10, true);
    assert.deepEqual(decision, { action: "ignore" });
    assert.equal(next, state);
  });

  test("Phase 2 (260905): a yield decision followed by a non-yielding settle continues the same streak as if the yield had never happened", () => {
    let state = armGoal("a goal");
    state = decideOnSettle(state, 10).next; // streak 1

    // Branch A: an intervening yielding settle, then a non-yielding settle.
    let withYield = decideOnSettle(state, 10, true).next; // yield: no-op
    const afterYield = decideOnSettle(withYield, 10, false);

    // Branch B: the non-yielding settle called directly, no yield in between.
    const direct = decideOnSettle(state, 10, false);

    assert.deepEqual(afterYield.decision, direct.decision);
    assert.deepEqual(afterYield.next, direct.next);
  });

  test("Phase 2 (260905): omitting the third argument still defaults to false — pre-existing two-argument call sites are unaffected", () => {
    const state = armGoal("a goal");
    const withDefault = decideOnSettle(state, 10);
    const explicitFalse = decideOnSettle(state, 10, false);
    assert.deepEqual(withDefault, explicitFalse);
  });

  test("260906 (Phase 1): compacting on an active goal neither re-injects nor advances the streak", () => {
    const state = armGoal("a goal");
    const { next, decision } = decideOnSettle(state, 10, false, true);
    assert.deepEqual(decision, { action: "waiting" });
    // Reference identity, not just structural equality — a true no-op
    // pass-through, mirroring the yield branch's own assertion shape.
    assert.equal(next, state);
  });

  test("260906 (Phase 1): compacting dominates yielding — both true still reports waiting, unchanged", () => {
    const state = armGoal("a goal");
    const { next, decision } = decideOnSettle(state, 10, true, true);
    assert.deepEqual(decision, { action: "waiting" });
    assert.equal(next, state);
  });

  test("260906 (Phase 1): compacting on an INACTIVE goal still ignores — compacting never resurrects an inactive loop", () => {
    const state = initialGoalLoopState();
    const { next, decision } = decideOnSettle(state, 10, false, true);
    assert.deepEqual(decision, { action: "ignore" });
    assert.equal(next, state);
  });

  test("260906 (Phase 1): sawToolCallThisCycle and noToolCallStreak are untouched by a waiting decision", () => {
    let state = armGoal("a goal");
    state = decideOnSettle(state, 10).next; // streak 1
    state = recordToolCall(state);
    const { next, decision } = decideOnSettle(state, 10, false, true);
    assert.deepEqual(decision, { action: "waiting" });
    assert.equal(next.noToolCallStreak, 1, "unchanged from before the waiting settle");
    assert.equal(next.sawToolCallThisCycle, true, "unchanged from before the waiting settle");
  });

  test("260906 (Phase 1): omitting the fourth argument still defaults to false — pre-existing three-argument call sites are unaffected", () => {
    const state = armGoal("a goal");
    const withDefault = decideOnSettle(state, 10, true);
    const explicitFalse = decideOnSettle(state, 10, true, false);
    assert.deepEqual(withDefault, explicitFalse);
  });
});

describe("isChildProcess", () => {
  test("true when the spawned-child role marker is set to worker (matches process-role.ts's WS_PI_SPAWN_ROLE_ENV key)", () => {
    assert.equal(isChildProcess({ [WS_PI_SPAWN_ROLE_ENV]: "worker" }), true);
  });

  test("true when the spawned-child role marker is set to explore", () => {
    assert.equal(isChildProcess({ [WS_PI_SPAWN_ROLE_ENV]: "explore" }), true);
  });

  test("true when the spawned-child role marker is set to fork (reserved, conservatively still treated as child)", () => {
    assert.equal(isChildProcess({ [WS_PI_SPAWN_ROLE_ENV]: "fork" }), true);
  });

  test("false when an invalid/unrecognized role value is set (readSpawnRole rejects it, so no marker is seen)", () => {
    assert.equal(isChildProcess({ [WS_PI_SPAWN_ROLE_ENV]: "bogus" }), false);
  });

  test("false when the marker is absent (lead session)", () => {
    assert.equal(isChildProcess({}), false);
  });

  test("false when other env vars are present but the marker is not among them", () => {
    assert.equal(isChildProcess({ PATH: "/usr/bin", HOME: "/home/user" }), false);
  });
});

/**
 * 260906 (compaction push-hold ticket, Phase 1) + 260906 Phase 1 (settle-timer
 * reminder race ticket): `registerGoalLoop`'s IO glue around the compaction
 * race and the settle-timer boundary guard — previously left to the live
 * `pi --mode json` gate per this file's own top-of-file doc comment, now
 * covered here with a fake-`pi` + duck-typed `ctx` harness, mirroring
 * `test/ask.test.ts`'s `describe("closeThreadOnDone / injectDiscussionSummary
 * (fake pi)")` shape. `leadCompactingRef`/`heldPushQueue`/
 * `leadWakeStartPendingRef` are module state shared with `spawner.ts`, so
 * every test here resets all three.
 *
 * Every `registerGoalLoop` call below passes a `fakeClock()`'s
 * `scheduleTimer`/`clearTimer` pair, so a reminder never actually sends until
 * a test explicitly fires the pending timer — matching the real settle-timer
 * shape (an `agent_settled` arms the timer; `settle_delay_ms` later, the fire
 * callback decides whether to send).
 */
describe("registerGoalLoop IO glue (fake pi): compaction release (260906 Phase 1)", () => {
  beforeEach(() => {
    leadCompactingRef.current = false;
    heldPushQueue.length = 0;
    leadWakeStartPendingRef.current = false;
  });

  afterEach(() => {
    clearWakeStart();
    leadIdleRef.current = undefined;
    leadCompactingRef.current = false;
    heldPushQueue.length = 0;
  });

  const configPath = join(tmpDir, "does-not-exist-compaction-260906.json");

  /**
   * Fake clock (260906 Phase 1, settle-timer reminder race ticket): records
   * every scheduled `{ cb, ms }` pair so a test can fire it directly instead
   * of waiting on a real timer. By construction the goal loop has at most ONE
   * active timer at a time — `armSettleTimer` cancels any prior settle timer
   * before scheduling a new one, and the boundary-guard's own fallback timer
   * is armed only after the settle timer that led to it has already fired
   * (clearing itself first) — so `fire()` throws if more than one timer is
   * pending, doubling as a regression guard against ever violating that
   * invariant.
   */
  function fakeClock(): {
    scheduleTimer: (cb: () => void, ms: number) => NodeJS.Timeout;
    clearTimer: (handle: NodeJS.Timeout) => void;
    pendingCount: () => number;
    fire: () => void;
    cancelled: NodeJS.Timeout[];
    scheduledDelays: number[];
    scheduledCallbacks: Array<() => void>;
  } {
    let nextId = 1;
    const pending = new Map<number, () => void>();
    const cancelled: NodeJS.Timeout[] = [];
    const scheduledDelays: number[] = [];
    const scheduledCallbacks: Array<() => void> = [];
    const scheduleTimer = (cb: () => void, ms: number): NodeJS.Timeout => {
      const id = nextId++;
      pending.set(id, cb);
      scheduledDelays.push(ms);
      scheduledCallbacks.push(cb);
      return id as unknown as NodeJS.Timeout;
    };
    const clearTimer = (handle: NodeJS.Timeout): void => {
      const id = handle as unknown as number;
      if (pending.delete(id)) cancelled.push(handle);
    };
    const fire = (): void => {
      const entries = [...pending.entries()];
      if (entries.length === 0) throw new Error("fakeClock.fire(): no pending timer to fire");
      if (entries.length > 1) {
        throw new Error("fakeClock.fire(): more than one pending timer — the goal loop should never have two active at once");
      }
      const [id, cb] = entries[0]!;
      pending.delete(id);
      cb();
    };
    return { scheduleTimer, clearTimer, pendingCount: () => pending.size, fire, cancelled, scheduledDelays, scheduledCallbacks };
  }

  /**
   * `streaming`: models Pi's real `isStreaming`/`prompt()` guard (review
   * relay #2, Critical) — `sendMessage(..., { triggerTurn: true })` (what a
   * flushed `HeldPush`/`HeldRawSend` actually calls) flips it true, and
   * `sendUserMessage` then throws exactly like the real
   * `agent-session.js:860-863` guard unless called with `deliverAs:
   * "followUp"` or `"steer"`. Existing tests never call `sendMessage`, so
   * `streaming` stays false and this is a no-op for them.
   */
  function fakePi(): {
    api: ExtensionAPI;
    handlers: Map<string, (event: unknown, ctx: ExtensionContext) => void>;
    commands: Map<string, (args: string, ctx: ExtensionContext) => Promise<void> | void>;
    commandDefs: Map<string, { description?: string; getArgumentCompletions?: (prefix: string) => Array<{ value: string; label: string }> | null }>;
    tools: Map<string, { execute: (...args: unknown[]) => Promise<unknown> }>;
    sentUserMessages: Array<{ content: unknown; options?: unknown }>;
    sentMessages: Array<{ content: unknown; options?: unknown }>;
    streaming: { current: boolean };
  } {
    const handlers = new Map<string, (event: unknown, ctx: ExtensionContext) => void>();
    const commands = new Map<string, (args: string, ctx: ExtensionContext) => Promise<void> | void>();
    const commandDefs = new Map<string, { description?: string; getArgumentCompletions?: (prefix: string) => Array<{ value: string; label: string }> | null }>();
    const tools = new Map<string, { execute: (...args: unknown[]) => Promise<unknown> }>();
    const sentUserMessages: Array<{ content: unknown; options?: unknown }> = [];
    const sentMessages: Array<{ content: unknown; options?: unknown }> = [];
    const streaming = { current: false };
    const api = {
      on: (event: string, handler: (event: unknown, ctx: ExtensionContext) => void) => {
        handlers.set(event, handler);
      },
      registerCommand: (name: string, def: { description?: string; getArgumentCompletions?: (prefix: string) => Array<{ value: string; label: string }> | null; handler: (args: string, ctx: ExtensionContext) => Promise<void> | void }) => {
        commands.set(name, def.handler);
        commandDefs.set(name, def);
      },
      registerTool: (def: { name: string; execute: (...args: unknown[]) => Promise<unknown> }) => {
        tools.set(def.name, def);
      },
      sendUserMessage: (content: unknown, options?: unknown) => {
        const deliverAs = (options as { deliverAs?: string } | undefined)?.deliverAs;
        if (streaming.current && deliverAs !== "followUp" && deliverAs !== "steer") {
          throw new Error("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message.");
        }
        sentUserMessages.push({ content, options });
      },
      sendMessage: (content: unknown, options?: unknown) => {
        sentMessages.push({ content, options });
        if ((options as { triggerTurn?: boolean } | undefined)?.triggerTurn) {
          streaming.current = true;
        }
      },
    };
    return { api: api as unknown as ExtensionAPI, handlers, commands, commandDefs, tools, sentUserMessages, sentMessages, streaming };
  }

  /**
   * A duck-typed `ExtensionContext` with a no-op `compact` — tests override
   * it to capture the lever's callbacks. `statusCalls` records every
   * `setStatus` call in order (review relay #1, Tests: previously a no-op
   * stub, so nothing could assert on the "waiting for compaction" footer).
   */
  function fakeCtx(isIdle: () => boolean = () => true): {
    ctx: ExtensionContext;
    notifications: Array<{ message: string; level: string }>;
    statusCalls: Array<{ key: string; value: string | undefined }>;
  } {
    const notifications: Array<{ message: string; level: string }> = [];
    const statusCalls: Array<{ key: string; value: string | undefined }> = [];
    const ctx = {
      ui: {
        notify: (message: string, level: string) => notifications.push({ message, level }),
        setStatus: (key: string, value: string | undefined) => statusCalls.push({ key, value }),
      },
      isIdle,
      getContextUsage: () => undefined,
      compact: () => {},
    };
    leadIdleRef.current = isIdle;
    return { ctx: ctx as unknown as ExtensionContext, notifications, statusCalls };
  }

  const carryHeading = "\n\nCarried forward verbatim from before compaction:\n";
  const exactCarry = "  Ω preserve\tthis\r\n한글 🦦\n  final\t ";

  function assertCarry(content: unknown, payload: string): void {
    assert.equal(typeof content, "string");
    const parts = (content as string).split(carryHeading);
    assert.equal(parts.length, 2, "exactly one carry heading");
    assert.equal(parts[1], payload, "raw suffix preserves every character");
  }

  function reminderMarker(content: unknown): string {
    assert.equal(typeof content, "string");
    const match = (content as string).match(/<!-- ws-pi-goal-reminder:[^ ]+ -->/);
    assert.ok(match, "reminder carries an adapter-owned correlation marker");
    return match[0];
  }

  function acknowledgeLatestReminder(pi: ReturnType<typeof fakePi>, ctx: ExtensionContext): void {
    const content = pi.sentUserMessages.at(-1)!.content;
    reminderMarker(content);
    pi.handlers.get("message_start")!({ message: { role: "user", content } }, ctx);
  }

  describe("explicit /goal stop controls (260909)", () => {
    test("help and completion reserve only stop, clear, and reset while ordinary goal text still arms", async () => {
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath) });
      const { ctx, notifications } = fakeCtx();
      const def = pi.commandDefs.get("goal")!;

      assert.match(def.description!, /stop.*clear.*reset/i);
      assert.deepEqual(def.getArgumentCompletions!(""), [
        { value: "stop", label: "stop" },
        { value: "clear", label: "clear" },
        { value: "reset", label: "reset" },
      ]);
      assert.deepEqual(def.getArgumentCompletions!("cl"), [{ value: "clear", label: "clear" }]);
      assert.equal(def.getArgumentCompletions!("ship"), null);

      await pi.commands.get("goal")!("", ctx);
      assert.match(notifications.at(-1)!.message, /\/goal <goal>.*stop.*clear.*reset/i);

      for (const text of ["stopping", "reset plan", "STOP"]) {
        await pi.commands.get("goal")!(text, ctx);
        assert.equal(pi.sentUserMessages.at(-1)!.content, `Goal armed: ${text}`);
      }
    });

    test("active-turn replacements queue, apply in FIFO order, and only the resulting goal is reminded", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      let idle = true;
      const { ctx, notifications } = fakeCtx(() => idle);
      await pi.commands.get("goal")!("original", ctx);

      idle = false;
      await pi.commands.get("goal")!("first replacement", ctx);
      await pi.commands.get("goal")!("final replacement", ctx);
      assert.deepEqual(
        notifications.slice(-2).map(({ message, level }) => ({ message, level })),
        [
          { message: "Goal update queued: first replacement", level: "info" },
          { message: "Goal update queued: final replacement", level: "info" },
        ],
      );
      assert.deepEqual(heldPushQueue.map((entry) => entry.kind), ["goal-replacement", "goal-replacement"]);

      assert.equal(flushHeldPushes(pi.api, true), 2);
      assert.equal(pi.sentMessages.length, 0, "control-only drainage creates no synthetic model message or turn");
      assert.deepEqual(
        notifications.slice(-2).map(({ message, level }) => ({ message, level })),
        [
          { message: "Goal update applied: first replacement", level: "info" },
          { message: "Goal update applied: final replacement", level: "info" },
        ],
      );

      idle = true;
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.match(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running: "final replacement"/);
      assert.doesNotMatch(pi.sentUserMessages.at(-1)!.content as string, /original|first replacement/);
    });

    test("terminal control invalidates a queued replacement without rearming the terminated goal", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      let idle = true;
      const { ctx, notifications } = fakeCtx(() => idle);
      await pi.commands.get("goal")!("original", ctx);

      idle = false;
      await pi.commands.get("goal")!("stale replacement", ctx);
      await pi.commands.get("goal")!("stop", ctx);
      assert.equal(flushHeldPushes(pi.api, true), 1);
      assert.equal(pi.sentMessages.length, 0);
      assert.deepEqual(notifications.at(-1), {
        message: 'Goal update failed: "stale replacement" was invalidated by a newer immediate or terminal goal transition.',
        level: "error",
      });

      idle = true;
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 0, "failed stale replacement leaves the terminal goal state disarmed");
      assert.equal(pi.sentUserMessages.length, 1, "the terminated original goal is never reminded");
    });

    test("terminal control invalidates older queued replacements while a later replacement uses the new generation", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      let idle = true;
      const { ctx, notifications } = fakeCtx(() => idle);
      await pi.commands.get("goal")!("original", ctx);

      idle = false;
      await pi.commands.get("goal")!("stale replacement", ctx);
      await pi.commands.get("goal")!("stop", ctx);
      await pi.commands.get("goal")!("post-terminal replacement", ctx);
      assert.equal(flushHeldPushes(pi.api, true), 2);
      assert.equal(pi.sentMessages.length, 0);
      assert.deepEqual(
        notifications.slice(-2).map(({ message, level }) => ({ message, level })),
        [
          { message: 'Goal update failed: "stale replacement" was invalidated by a newer immediate or terminal goal transition.', level: "error" },
          { message: "Goal update applied: post-terminal replacement", level: "info" },
        ],
      );

      idle = true;
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.match(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running: "post-terminal replacement"/);
      assert.doesNotMatch(pi.sentUserMessages.at(-1)!.content as string, /stale replacement/);
    });

    for (const alias of ["stop", "clear", "reset"] as const) {
      test(`${alias} disarms while busy without interrupting work and repeated use is harmless`, async () => {
        const clock = fakeClock();
        const pi = fakePi();
        registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
        let idle = true;
        const { ctx, notifications, statusCalls } = fakeCtx(() => idle);
        await pi.commands.get("goal")!("ship", ctx);
        pi.handlers.get("agent_settled")!({}, ctx);
        assert.equal(clock.pendingCount(), 1);

        idle = false;
        await pi.commands.get("goal")!(`  ${alias}  `, ctx);
        assert.equal(clock.pendingCount(), 0, "stop cancels local goal scheduling even during an active response");
        assert.equal(pi.sentUserMessages.length, 1, "stop does not inject, abort, compact, or replace the current response");
        assert.match(notifications.at(-1)!.message, /automatic goal continuation stopped/i);
        assert.deepEqual(statusCalls.at(-1), { key: "ws-goal-loop-yield", value: undefined });

        await pi.commands.get("goal")!(alias, ctx);
        assert.equal(pi.sentUserMessages.length, 1, "repeated stop remains inert");
        assert.match(notifications.at(-1)!.message, /automatic goal continuation stopped/i);
      });
    }

    test("a cancelled timer callback cannot submit after stop or replace a newly armed goal", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("old", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      const stale = clock.scheduledCallbacks.at(-1)!;

      await pi.commands.get("goal")!("stop", ctx);
      await pi.commands.get("goal")!("new", ctx);
      stale();
      assert.deepEqual(pi.sentUserMessages.map((message) => message.content), ["Goal armed: old", "Goal armed: new"]);

      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.match(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running: "new"/);
      assert.doesNotMatch(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running: "old"/);
    });

    test("one reminder handoff remains outstanding until its matching public user message_start", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2);
      const marker = reminderMarker(pi.sentUserMessages[1]!.content);

      pi.handlers.get("message_start")!({ message: { role: "assistant", content: marker } }, ctx);
      pi.handlers.get("message_start")!({ message: { role: "user", content: "unrelated owner message" } }, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2, "unrelated messages do not clear or duplicate the outstanding handoff");

      pi.handlers.get("message_start")!({ message: { role: "user", content: [{ type: "text", text: `queued ${marker}` }] } }, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 3, "matching queued-consumption message_start clears the handoff without requiring agent_start");
      reminderMarker(pi.sentUserMessages[2]!.content);
    });

    test("wake timeout does not resubmit an unconfirmed reminder handoff", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx, statusCalls } = fakeCtx();
      await pi.commands.get("goal")!("ship", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire(); // reminder handoff; wake timeout is now pending
      assert.equal(pi.sentUserMessages.length, 2);

      clock.fire(); // wake timeout expires before message_start
      assert.equal(pi.sentUserMessages.length, 2, "no duplicate reminder is handed to Pi");
      assert.equal(clock.pendingCount(), 0, "unconfirmed handoff suppresses recovery rearm");
      assert.match(statusCalls.at(-1)!.value!, /awaiting reminder handoff/i);
    });

    test("stop preserves one already-handed reminder but prevents its settle from rearming", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      const marker = reminderMarker(pi.sentUserMessages[1]!.content);
      assert.equal(clock.pendingCount(), 1, "handoff recovery is active before stop");

      await pi.commands.get("goal")!("stop", ctx);
      assert.equal(clock.pendingCount(), 1, "stop does not clear the shared wake reservation after host handoff");
      pi.handlers.get("message_start")!({ message: { role: "user", content: marker } }, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 0);
      assert.equal(pi.sentUserMessages.length, 2, "the accepted reminder may execute once, but cannot rearm a stopped goal");
    });

    test("stale compaction completion releases the shared hold but cannot revive old rearm or carry state", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("old", ctx);
      let compactCall!: Parameters<ExtensionContext["compact"]>[0];
      ctx.compact = (opts) => { compactCall = opts; };
      await pi.tools.get("ws-compact")!.execute("carry", { current_work: "old carry" }, undefined, undefined, ctx);
      assert.equal(leadCompactingRef.current, true);

      await pi.commands.get("goal")!("stop", ctx);
      await pi.commands.get("goal")!("new", ctx);
      compactCall!.onComplete!({} as never);
      assert.equal(leadCompactingRef.current, false, "old completion still releases the independent compaction hold");
      assert.equal(clock.pendingCount(), 0, "old completion cannot schedule against the new generation");

      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.match(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running: "new"/);
      assert.doesNotMatch(pi.sentUserMessages.at(-1)!.content as string, /old carry/);
    });

    test("shutdown invalidates stale callbacks and a recovered session starts disarmed", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      const handle = registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      const stale = clock.scheduledCallbacks.at(-1)!;
      handle.resetCompactionStateForShutdown();
      stale();
      assert.deepEqual(pi.sentUserMessages.map((message) => message.content), ["Goal armed: ship"]);

      const recoveredClock = fakeClock();
      const recoveredPi = fakePi();
      registerGoalLoop(recoveredPi.api, { readConfig: fileReader(configPath), ...recoveredClock });
      const { ctx: recoveredCtx } = fakeCtx();
      recoveredPi.handlers.get("agent_settled")!({}, recoveredCtx);
      assert.equal(recoveredClock.pendingCount(), 0, "session recovery does not restore the stopped goal or its timer");
      assert.deepEqual(recoveredPi.sentUserMessages, [], "the first recovered settle cannot create an automatic turn");
    });
  });

  test("ws-compact compacts with no active goal without touching goal-loop state (reverses 260913's rejection)", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
    const { ctx, notifications, statusCalls } = fakeCtx();
    let compactCall: Parameters<ExtensionContext["compact"]>[0] | undefined;
    let compactCalls = 0;
    ctx.compact = (opts) => { compactCalls += 1; compactCall = opts; };

    const result = await pi.tools.get("ws-compact")!.execute("carry", { current_work: "no goal here" }, undefined, undefined, ctx);
    assert.match((result as { content: Array<{ text: string }> }).content[0]!.text, /^Compaction requested/);
    assert.equal(compactCalls, 1, "the lever compacts without an active goal");
    assert.equal(leadCompactingRef.current, true, "the shared push hold still covers the compaction");
    const hookResult = pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx) as unknown as { compaction: { summary: string } };
    assert.ok(hookResult?.compaction, "the lever's manual compaction is answered with the adapter's summary");
    assert.match(hookResult.compaction.summary, /no goal here/);

    pi.handlers.get("agent_settled")!({}, ctx);
    compactCall!.onComplete!({} as never);
    pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: true, compactionEntry: { details: { kind: "ws-pi-lead-compaction", version: 1, source: "lever" } } }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(leadCompactingRef.current, false, "the hold is released");
    assert.equal(clock.pendingCount(), 0, "no goal reminder is scheduled");
    assert.deepEqual(statusCalls, [], "goal-loop status is untouched");
    assert.deepEqual(notifications.map((n) => n.message), ["Compaction completed"]);
    assert.deepEqual(pi.sentUserMessages, [], "nothing is injected");

    await pi.commands.get("goal")!("ship", ctx);
    pi.handlers.get("agent_settled")!({}, ctx);
    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "a later active goal still follows the ordinary reminder path");
    assert.ok(!(pi.sentUserMessages[1]!.content as string).includes(carryHeading), "the goal-less prose never becomes a goal carry");
  });

  for (const completion of ["event", "callback", "both", "error", "failed-event"] as const) {
    test(`verbatim carry is sent once after ${completion} release`, async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship", ctx);
      let compactCall!: Parameters<ExtensionContext["compact"]>[0];
      ctx.compact = (opts) => { compactCall = opts; };
      const result = await pi.tools.get("ws-compact")!.execute("carry", { current_work: exactCarry }, undefined, undefined, ctx);
      assert.equal((result as { terminate?: boolean }).terminate, undefined, "lever stays non-terminal");
      assert.equal(compactCall!.customInstructions, prose(exactCarry), "native-summarizer steering is the rendered prose");
      pi.handlers.get("agent_settled")!({}, ctx);
      if (completion === "callback" || completion === "both") compactCall!.onComplete!({} as never);
      if (completion === "error") compactCall!.onError!(new Error("boom"));
      if (completion === "event" || completion === "both") pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
      if (completion === "failed-event") pi.handlers.get("session_compact_failed")!({ errorMessage: "Compaction failed: boom" }, ctx);
      assert.equal(pi.sentUserMessages.length, 1, "no synchronous event send");
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(pi.sentUserMessages.length, 1, "release only arms the settle timer");
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2);
      assertCarry(pi.sentUserMessages[1]!.content, prose(exactCarry));
      assert.deepEqual(pi.sentUserMessages[1]!.options, { deliverAs: "followUp" });
      if (completion === "error" || completion === "failed-event") {
        assert.match(pi.sentUserMessages[1]!.content as string, /^Compaction failed: boom Do not retry/);
      }
      pi.handlers.get("agent_start")!({}, ctx);
      acknowledgeLatestReminder(pi, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 3);
      assert.ok(!(pi.sentUserMessages[2]!.content as string).includes(carryHeading));
    });
  }

  test("empty carry is present, captured before compact can synchronously complete and dispatch", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
    const { ctx } = fakeCtx();
    await pi.commands.get("goal")!("ship", ctx);
    ctx.compact = (opts) => {
      assert.equal(opts!.customInstructions, prose(""));
      opts!.onComplete!({} as never);
      clock.fire(); // Probe capture ordering before ctx.compact returns.
      assertCarry(pi.sentUserMessages[1]!.content, prose(""));
    };
    await pi.tools.get("ws-compact")!.execute("carry", { current_work: "" }, undefined, undefined, ctx);
  });

  for (const interruption of ["busy-release", "start-before-release", "idle-yield", "child-yield"] as const) {
    test(`carry survives ${interruption} until an eligible ordinary reminder`, async () => {
      const clock = fakeClock();
      const pi = fakePi();
      const registry = new Map([["child", { threadBound: false, running: false }]]) as unknown as RpcAgentRegistry;
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock, rpcRegistryRef: { current: registry } });
      let idle = true;
      const { ctx } = fakeCtx(() => idle);
      await pi.commands.get("goal")!("ship", ctx);
      await pi.tools.get("ws-compact")!.execute("carry", { current_work: exactCarry }, undefined, undefined, ctx);
      if (interruption === "start-before-release") {
        idle = false;
        pi.handlers.get("agent_start")!({}, ctx);
        assert.equal(leadCompactingRef.current, true, "start does not release compaction");
        pi.handlers.get("session_compact")!({}, ctx);
        await new Promise((resolve) => setImmediate(resolve));
      } else {
        if (interruption === "busy-release") idle = false;
        pi.handlers.get("session_compact")!({}, ctx);
        await new Promise((resolve) => setImmediate(resolve));
        if (interruption === "idle-yield" || interruption === "child-yield") {
          if (interruption === "idle-yield") idle = false;
          else registry.get("child")!.running = true;
          clock.fire();
        }
      }
      assert.equal(pi.sentUserMessages.length, 1, "interruption sends nothing");
      assert.equal(clock.pendingCount(), 0);
      idle = true;
      registry.get("child")!.running = false;
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assertCarry(pi.sentUserMessages[1]!.content, prose(exactCarry));
    });
  }

  for (const cleanup of ["new-goal", "goal-achieved", "goal-blocked", "shutdown"] as const) {
    test(`${cleanup} discards unsent carry`, async () => {
      const clock = fakeClock();
      const pi = fakePi();
      const handle = registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("old", ctx);
      await pi.tools.get("ws-compact")!.execute("carry", { current_work: exactCarry }, undefined, undefined, ctx);
      if (cleanup === "shutdown") handle.resetCompactionStateForShutdown();
      else if (cleanup === "new-goal") await pi.commands.get("goal")!("new", ctx);
      else await pi.tools.get(cleanup)!.execute("stop", { summary: "done", reason: "blocked" });
      pi.handlers.get("session_compact")!({}, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      pi.handlers.get("agent_settled")!({}, ctx);
      if (clock.pendingCount()) clock.fire();
      if (cleanup === "goal-achieved" || cleanup === "goal-blocked") {
        assert.equal(pi.sentUserMessages.length, 1, "terminal lever sends no reminder");
        await pi.commands.get("goal")!("new", ctx);
        pi.handlers.get("agent_settled")!({}, ctx);
        clock.fire();
      }
      if (cleanup === "shutdown") assert.equal(pi.sentUserMessages.length, 1, "shutdown leaves the old loop inert");
      else assert.match(pi.sentUserMessages.at(-1)!.content as string, /Goal yet running/);
      for (const message of pi.sentUserMessages) assert.ok(!(message.content as string).includes(carryHeading));
    });
  }

  test("synchronous send failure does not consume carry", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
    const { ctx, notifications } = fakeCtx();
    await pi.commands.get("goal")!("ship", ctx);
    await pi.tools.get("ws-compact")!.execute("carry", { current_work: exactCarry }, undefined, undefined, ctx);
    pi.handlers.get("session_compact")!({}, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    const send = pi.api.sendUserMessage;
    pi.api.sendUserMessage = () => { throw new Error("send failed"); };
    clock.fire();
    assert.match(notifications.at(-1)!.message, /send failed/);
    assert.equal(pi.sentUserMessages.length, 1);
    pi.api.sendUserMessage = send;
    pi.handlers.get("agent_settled")!({}, ctx);
    clock.fire();
    assertCarry(pi.sentUserMessages[1]!.content, prose(exactCarry));
  });

  test("reducer preserves pending carry through waits and tool calls but discards it on force-stop", () => {
    const pending = { ...armGoal("ship"), pendingCarryForward: exactCarry };
    assert.equal(recordToolCall(pending).pendingCarryForward, exactCarry);
    assert.equal(decideOnSettle(pending, 2, false, true).next.pendingCarryForward, exactCarry);
    assert.equal(decideOnSettle(pending, 2, true).next.pendingCarryForward, exactCarry);
    const first = decideOnSettle(pending, 2);
    assert.equal(first.next.pendingCarryForward, exactCarry);
    const stopped = decideOnSettle(first.next, 2);
    assert.equal(stopped.decision.action, "force-stop");
    assert.equal(stopped.next.pendingCarryForward, undefined);
  });

  test("ordinary reminder without a lever has no carry heading", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock });
    const { ctx } = fakeCtx();
    await pi.commands.get("goal")!("ship", ctx);
    pi.handlers.get("agent_settled")!({}, ctx);
    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2);
    assert.ok(!(pi.sentUserMessages[1]!.content as string).includes(carryHeading));
  });

  test("release runs once when both the lever's onComplete and session_compact arrive", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    assert.equal(pi.sentUserMessages.length, 1, "the armed announcement");

    let compactCall: { onComplete?: () => void; onError?: (error: Error) => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "phase 1 done" }, undefined, undefined, ctx);
    // Proves the lever's own tool-call handler sets the flag as one of its
    // synchronous effects (alongside calling the fake's no-op `ctx.compact`)
    // — not a claim about ordering relative to the real `ctx.compact`, which
    // this fake does not model.
    assert.equal(leadCompactingRef.current, true, "set by the lever's tool call before it returns");

    compactCall!.onComplete!();
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 1, "no reminder sent yet — the settle timer was armed, not fired");
    assert.equal(clock.pendingCount(), 1, "the settle timer is pending");

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(clock.pendingCount(), 1, "already armed by onComplete — session_compact's own release arms nothing new");

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the re-armed reminder, sent once the settle timer fires");
  });

  test("release triggered by session_compact is deferred — nothing sent synchronously inside that handler", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = () => {};
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.equal(pi.sentUserMessages.length, 1, "only the armed announcement so far");

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    assert.equal(pi.sentUserMessages.length, 1, "nothing sent synchronously inside the session_compact handler itself");
    assert.equal(leadCompactingRef.current, true, "still marked compacting until the deferred release runs");

    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 1, "the deferred release only arms the settle timer — nothing sent yet");
    assert.equal(clock.pendingCount(), 1);

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the settle timer fired the re-armed reminder");
  });

  test("onError alone releases with the failure reason folded into the reminder", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    let compactCall: { onError?: (error: Error) => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);

    compactCall!.onError!(new Error("boom"));
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 1, "the settle timer was armed, not fired — the failure reason travels with it");

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2);
    const reminder = pi.sentUserMessages[1]!.content as string;
    assert.match(reminder, /Compaction failed: boom/);
    assert.match(reminder, /Do not retry ws-compact/);
  });

  test("agent_start preserves the compaction flag and held queue", () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath) });
    const { ctx } = fakeCtx();

    // A defensively-set flag (e.g. an owner-typed /compact) with the goal
    // never even armed — the backstop must still fire.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    assert.equal(leadCompactingRef.current, true);

    heldPushQueue.push({
      kind: "raw",
      deliverAs: "followUp",
      message: { customType: "ws-agent-advisory", content: "held fixture", display: true, details: { advisory: "held-fixture" } },
    });

    pi.handlers.get("agent_start")!({}, ctx);
    assert.equal(leadCompactingRef.current, true, "start cannot clear an independent compaction hold");
    assert.equal(pi.sentUserMessages.length, 0, "no reminder during compaction");
    assert.equal(pi.sentMessages.length, 0, "no queue touch either — that is releaseAfterCompaction's job, not this backstop's");
    assert.equal(heldPushQueue.length, 1, "left untouched for that turn's own settle/flush handler");
  });

  test("a non-lever session_compact holds pushes until confirmed start and sends no reminder", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath) });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // state active; 1 message so far (armed)

    // Owner-typed /compact: session_before_compact sets the flag defensively,
    // but pendingRearm is never set — the lever was never called.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    assert.equal(leadCompactingRef.current, true);

    heldPushQueue.push({
      kind: "raw",
      deliverAs: "followUp",
      message: { customType: "ws-agent-advisory", content: "held fixture", display: true, details: { advisory: "held-fixture" } },
    });

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentMessages.length, 0, "idle release cannot directly send custom messages");
    assert.equal(flushHeldPushes(pi.api, true), 1, "confirmed start releases the held push");
    assert.equal(pi.sentMessages.length, 1);
    assert.equal(pi.sentUserMessages.length, 1, "still just the armed announcement — no synthesized reminder for a non-lever compaction");
  });

  test("release while the agent is not idle sends nothing; a subsequent settle re-arms the loop normally", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx(() => true);

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    let compactCall: unknown;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.ok(compactCall, "ctx.compact was called");
    assert.equal(leadCompactingRef.current, true);

    heldPushQueue.push({
      kind: "raw",
      deliverAs: "followUp",
      message: { customType: "ws-agent-advisory", content: "held fixture", display: true, details: { advisory: "held-fixture" } },
    });

    // The release-time ctx reports NOT idle — e.g. agent_start's own backstop
    // raced this call and a fresh turn is already underway.
    const notIdle = fakeCtx(() => false).ctx;
    pi.handlers.get("session_compact")!({ reason: "manual" }, notIdle);
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(leadCompactingRef.current, false, "the flag is still cleared even when nothing else fires");
    assert.equal(pi.sentUserMessages.length, 1, "nothing sent while the agent already looks busy again");
    assert.equal(pi.sentMessages.length, 0, "the held queue is left for that turn's own settle, not drained here");
    assert.equal(clock.pendingCount(), 0, "the not-idle branch clears pendingRearm rather than arming a timer");

    // A subsequent settle re-evaluates normally: leadCompactingRef is false
    // again, so decideOnSettle sees compacting=false and reinjects as usual.
    const settled = fakeCtx(() => true).ctx;
    flushHeldPushes(pi.api, true); // model the push-woken start before its next settle
    pi.handlers.get("agent_settled")!({}, settled);
    assert.equal(pi.sentUserMessages.length, 1, "the settle timer was armed, not fired yet");

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the ordinary reinject reminder fires once the settle timer fires");
  });

  test("260906 review relay #1 (Tests): a settle that fires mid-compaction sets the waiting-for-compaction footer", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath) });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)

    // Simulate a compaction already in flight when the settle lands — this
    // is the general shape both the lever's abort and Pi's own threshold
    // auto-compaction produce, without needing to drive either end-to-end.
    leadCompactingRef.current = true;
    const { ctx: settledCtx, statusCalls } = fakeCtx();
    pi.handlers.get("agent_settled")!({}, settledCtx);

    assert.equal(pi.sentUserMessages.length, 1, "no reminder — this settle's outcome is swallowed, not sent");
    assert.deepEqual(
      statusCalls,
      [{ key: "ws-goal-loop-yield", value: "Goal loop: waiting for compaction" }],
      "the footer is set exactly once, with the waiting-for-compaction text",
    );
  });

  test("confirmed-start flush precedes the lever reminder without consuming carry", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    let compactCall: { onComplete?: () => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.equal(leadCompactingRef.current, true);

    const order: string[] = [];
    heldPushQueue.push({
      kind: "raw",
      deliverAs: "followUp",
      message: { customType: "ws-agent-advisory", content: "held fixture", display: true, details: { advisory: "held-fixture" } },
    });
    const originalPush = pi.api.sendMessage;
    pi.api.sendMessage = (content, options) => {
      order.push("flush");
      originalPush(content, options);
    };
    const originalSend = (pi.api as unknown as { sendUserMessage: (content: unknown, options?: unknown) => void }).sendUserMessage;
    (pi.api as unknown as { sendUserMessage: (content: unknown, options?: unknown) => void }).sendUserMessage = (content, options) => {
      order.push("reminder");
      originalSend(content, options);
    };

    compactCall!.onComplete!();
    assert.deepEqual(order, [], "idle release holds the batch for a user-woken start");
    flushHeldPushes(pi.api, true);
    clock.fire();
    assert.deepEqual(order, ["flush", "reminder"], "held pushes flush before the pending reminder is sent");
    assert.equal(pi.sentUserMessages.length, 2, "the armed announcement, then the re-armed reminder");
    assertCarry(pi.sentUserMessages[1]!.content, prose("x"));
  });

  test("260906 review relay #1 (Critical): a threshold auto-compaction's swallowed settle is replayed by the deferred release — exactly one ordinary reminder, streak advanced", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed), streak 0

    // Pi's own threshold auto-compaction: session_before_compact sets the
    // flag defensively (no lever call, so pendingRearm stays false), then
    // session_compact fires, then — same microtask turn, before the
    // setImmediate release fires — agent_settled lands with the flag still
    // true (the Critical finding's exact sequence).
    pi.handlers.get("session_before_compact")!({ reason: "threshold" }, ctx);
    assert.equal(leadCompactingRef.current, true);
    pi.handlers.get("session_compact")!({ reason: "threshold" }, ctx);

    const { ctx: settledCtx } = fakeCtx();
    pi.handlers.get("agent_settled")!({}, settledCtx);
    assert.equal(pi.sentUserMessages.length, 1, "the settle's own outcome is swallowed, not sent yet");
    assert.equal(leadCompactingRef.current, true, "still marked compacting until the deferred release runs");
    assert.equal(clock.pendingCount(), 0, "no settle timer armed while compacting");

    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 1, "the deferred release only arms the settle timer to replay the swallowed settle");
    assert.equal(clock.pendingCount(), 1);

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the settle timer fired, replaying the swallowed settle as one ordinary reminder");
    const reminder = pi.sentUserMessages[1]!.content as string;
    assert.doesNotMatch(reminder, /Compaction failed/, "an ordinary reinject, not a lever failure reminder");
    acknowledgeLatestReminder(pi, ctx);

    // Streak advanced: a second replayed settle force-stops at threshold 2.
    pi.handlers.get("session_before_compact")!({ reason: "threshold" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "threshold" }, ctx);
    pi.handlers.get("agent_settled")!({}, fakeCtx().ctx);
    await new Promise((resolve) => setImmediate(resolve));
    clock.fire();
    // DEFAULT_RUNAWAY_THRESHOLD is well above 2, so this should still reinject,
    // not force-stop — this leg only proves the streak moved forward at all.
    assert.equal(pi.sentUserMessages.length, 3, "the streak advanced past the first replayed reinject rather than resetting");
  });

  test("260906 review relay #1 (Critical): the lever's own pendingRearm wins over a same-settle swallow marker — exactly one reminder, both markers consumed", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)

    // The normal lever flow: ctx.compact()'s internal abort produces its own
    // "waiting" settle for the invoking turn (setting the swallow marker)
    // before the lever's onComplete/session_compact ever run — both
    // pendingRearm (from the lever call below) and the swallow marker are
    // true by the time release runs.
    let compactCall: { onComplete?: () => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.equal(leadCompactingRef.current, true);

    pi.handlers.get("agent_settled")!({}, fakeCtx().ctx);
    assert.equal(pi.sentUserMessages.length, 1, "the invoking turn's own settle is swallowed, not sent");
    assert.equal(clock.pendingCount(), 0, "no settle timer armed while compacting");

    compactCall!.onComplete!();
    assert.equal(pi.sentUserMessages.length, 1, "settle timer armed, not fired yet");
    assert.equal(clock.pendingCount(), 1);

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "exactly one reminder — the lever's, not a second replayed settle");
    const reminder = pi.sentUserMessages[1]!.content as string;
    assert.doesNotMatch(reminder, /Compaction failed/);

    // Both markers are consumed: a later non-lever compaction sends no
    // reminder at all (no stale pendingRearm), and a later ordinary settle
    // does not replay a second time (no stale swallow marker).
    const pendingBeforeUnrelatedCompaction = clock.pendingCount(); // the reminder's own still-pending fallback timer
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 2, "no stale pendingRearm leaking into this unrelated compaction");
    assert.equal(
      clock.pendingCount(),
      pendingBeforeUnrelatedCompaction,
      "no new timer armed for this unrelated, non-lever release — the reminder's own fallback timer, if any, is untouched",
    );
  });

  test("swallowed-settle replay retains followUp mode after confirmed-start delivery", async () => {
    const clock = fakeClock();
    const threshold2Path = writeConfig("relay2-threshold-2.json", JSON.stringify({ runaway_threshold: 2 }));
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(threshold2Path), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed), streak 0

    // Threshold auto-compaction sequence: session_before_compact ->
    // session_compact -> agent_settled lands with the flag still true, so the
    // settle is swallowed (matching the review-relay-1 Critical fix's own
    // reproduction shape).
    pi.handlers.get("session_before_compact")!({ reason: "threshold" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "threshold" }, ctx);
    pi.handlers.get("agent_settled")!({}, fakeCtx().ctx);
    assert.equal(pi.sentUserMessages.length, 1, "swallowed, not sent yet");

    // A held raw item is batched with every other pending push at the next
    // confirmed start; the fixture is the original item, not a send callback.
    heldPushQueue.push({
      kind: "raw",
      deliverAs: "followUp",
      message: { customType: "ws-agent-report", content: "held report", display: true, details: { report: "held report" } },
    });

    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(pi.streaming.current, false, "idle release cannot start a custom run");
    flushHeldPushes(pi.api, true);
    assert.equal(pi.streaming.current, true, "confirmed-start flush delivers the batch");
    assert.equal((pi.sentMessages[0]!.content as { customType?: string }).customType, PUSH_BATCH_CUSTOM_TYPE, "the confirmed start transports the held item in one batch envelope");
    assert.deepEqual(pi.sentMessages[0]!.options, { deliverAs: "steer", triggerTurn: true }, "the batch honors confirmed-start steering");
    assert.equal(pi.sentUserMessages.length, 1, "the deferred release only armed the settle timer");
    assert.equal(clock.pendingCount(), 1);

    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the replayed reminder was delivered, not thrown away mid-stream");
    const replay = pi.sentUserMessages[1]!;
    assert.equal(
      (replay.options as { deliverAs?: string } | undefined)?.deliverAs,
      "followUp",
      "queues behind the flush's turn instead of throwing — a bare call would hit the streaming guard above",
    );
    acknowledgeLatestReminder(pi, ctx);

    // The reminder's own boundary-guard fallback timer is now pending; a real
    // settle proves its turn started, clearing that guard (agent_settled's
    // clear point) before this settle re-arms the settle timer below.
    const nextSettle = fakeCtx();
    pi.handlers.get("agent_settled")!({}, nextSettle.ctx);
    assert.equal(pi.sentUserMessages.length, 2, "the settle timer re-armed, not yet fired");

    // The streak advanced by exactly one for the replay above: with
    // runaway_threshold 2, exactly one more ordinary settle reaches the
    // threshold and force-stops — it would already have force-stopped on
    // THIS settle (streak 0 -> 2 in one call is not how the reducer works)
    // or would still be short of it here if the replay had not advanced the
    // streak at all.
    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "force-stop notifies; it does not send a third reminder");
    assert.equal(nextSettle.notifications.length, 1);
    assert.match(nextSettle.notifications[0]!.message, /Goal loop force-stopped/);
  });

  test("260906 review relay #2 (Test Important): the not-idle branch's marker clearing is observable across a later, unrelated compaction", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    let compactCall: unknown;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts;
    };
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.ok(compactCall, "ctx.compact was called");
    assert.equal(leadCompactingRef.current, true, "pendingRearm is now true");

    // Released while NOT idle: the not-idle branch must clear pendingRearm
    // (and the swallow marker) even though it sends nothing itself.
    const notIdle = fakeCtx(() => false).ctx;
    pi.handlers.get("session_compact")!({ reason: "manual" }, notIdle);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 1, "nothing sent by the not-idle release itself");
    assert.equal(clock.pendingCount(), 0, "the not-idle branch clears the markers instead of arming a timer");

    // That turn's own settle re-arms normally — unaffected by the cleared markers.
    pi.handlers.get("agent_settled")!({}, fakeCtx().ctx);
    assert.equal(clock.pendingCount(), 1);
    clock.fire();
    assert.equal(pi.sentUserMessages.length, 2, "the ordinary reinject reminder fires on that turn's own settle");

    // A SECOND, unrelated, non-lever compaction cycle: if the not-idle branch
    // above had failed to clear pendingRearm, this idle release would
    // wrongly synthesize a lever reminder that was never requested.
    const pendingBeforeUnrelatedCompaction = clock.pendingCount(); // the reminder's own still-pending fallback timer
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 2, "no stale pendingRearm fabricating a reminder on this unrelated compaction");
    assert.equal(clock.pendingCount(), pendingBeforeUnrelatedCompaction, "no new timer armed by this unrelated release");
  });

  test("start before release preserves the hold; subsequent busy release clears origins before unrelated compaction", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = () => {};
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.equal(leadCompactingRef.current, true, "pendingRearm is now true");

    // agent_start's own backstop fires before session_compact ever does —
    // it must clear pendingRearm too, not just the flag.
    pi.handlers.get("agent_start")!({}, ctx);
    assert.equal(leadCompactingRef.current, true, "start leaves pending compaction intact");
    pi.handlers.get("session_compact")!({}, fakeCtx(() => false).ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(leadCompactingRef.current, false, "deferred busy release owns marker cleanup");
    assert.equal(pi.sentUserMessages.length, 1, "start and busy release send nothing");
    assert.equal(clock.pendingCount(), 0);

    // A later, unrelated, non-lever compaction cycle: a stale pendingRearm
    // would wrongly synthesize a lever reminder here.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 1, "no stale pendingRearm fabricating a reminder on this unrelated compaction");
    assert.equal(clock.pendingCount(), 0);
  });

  test("260906 review relay #2 (Test Minor): GoalLoopShutdownHandle resets the flag and both markers, and a following push is not held", async () => {
    const clock = fakeClock();
    const pi = fakePi();
    const handle = registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = () => {};
    await pi.tools.get("ws-compact")!.execute("call-1", { current_work: "x" }, undefined, undefined, ctx);
    assert.equal(leadCompactingRef.current, true);

    assert.equal(isOwningAgentIdle(), false, "isOwningAgentIdle() is forced false while pendingRearm's compaction is in flight");

    // A shutdown/`/reload` lands mid-compaction, before session_compact ever
    // fires for it.
    handle.resetCompactionStateForShutdown();
    assert.equal(leadCompactingRef.current, false, "the flag is reset");
    assert.equal(clock.pendingCount(), 0, "no lingering settle/boundary-guard timer");
    assert.equal(leadWakeStartPendingRef.current, false);

    // A following push is not held: `spawner.ts`'s `isOwningAgentIdle()` —
    // the exact predicate `pushToLead`'s `followUp` hold and `ask.ts`'s
    // `injectDiscussionSummary` both check — is no longer forced false by
    // the stale flag, so a fresh push would send immediately rather than
    // queuing on `heldPushQueue`.
    assert.equal(isOwningAgentIdle(), true, "no longer forced false — a following push is not held");

    // A later, unrelated, non-lever compaction cycle: a stale pendingRearm or
    // swallow marker (had the handle not cleared them too) would wrongly
    // synthesize a reminder/replay here.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 1, "no stale marker fabricating a reminder after the shutdown reset");
    assert.equal(clock.pendingCount(), 0);
  });

  /**
   * 260906 Phase 1 (settle-timer reminder race ticket): the ticket's own
   * Phase 1 test list — the settle timer's arm/fire/cancel mechanics, the
   * boundary guard's clear points, and the streak-advances-only-on-fired-
   * reminders invariant.
   */
  describe("settle timer mechanics (260906 Phase 1)", () => {
    test("a running child at fire time yields — no send, and a later settle with nothing running re-arms and fires normally", () => {
      const clock = fakeClock();
      const pi = fakePi();
      const registry = new Map([["child-1", { threadBound: false, running: true }]]) as unknown as RpcAgentRegistry;
      const rpcRegistryRef = { current: registry };
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer, rpcRegistryRef });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);

      clock.fire();
      assert.equal(pi.sentUserMessages.length, 1, "yields — a running child was found at fire time; no reminder, no re-arm by the fire callback itself");
      assert.equal(clock.pendingCount(), 0, "the loop does not re-arm on a yield — only the next live agent_settled does");

      // The child stops running before the NEXT agent_settled — re-arms and
      // fires normally.
      registry.get("child-1")!.running = false;
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2, "fires normally once nothing is running at fire time");
    });

    test("settle alone fires exactly once at the delay, reading settle_delay_ms fresh from the config file", () => {
      const settleDelayPath = writeConfig("settle-delay-1500.json", JSON.stringify({ settle_delay_ms: 1500 }));
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(settleDelayPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      assert.deepEqual(clock.scheduledDelays, [1500], "the configured settle_delay_ms, not the built-in default");

      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2, "exactly one reminder for one settle");
      assert.equal(
        clock.pendingCount(),
        1,
        "fired exactly once — the reminder's own boundary-guard fallback timer is the only thing pending now, not a second settle timer",
      );
    });

    for (const [label, trigger] of [
      ["agent_start", (pi2: ReturnType<typeof fakePi>, ctx2: ExtensionContext) => pi2.handlers.get("agent_start")!({}, ctx2)],
      [
        "goal-achieved",
        async (pi2: ReturnType<typeof fakePi>, ctx2: ExtensionContext) =>
          pi2.tools.get("goal-achieved")!.execute("call-1", { summary: "done" }, undefined, undefined, ctx2),
      ],
      [
        "goal-blocked",
        async (pi2: ReturnType<typeof fakePi>, ctx2: ExtensionContext) =>
          pi2.tools.get("goal-blocked")!.execute("call-1", { reason: "stuck" }, undefined, undefined, ctx2),
      ],
    ] as const) {
      test(`${label} cancels a pending settle timer`, async () => {
        const clock = fakeClock();
        const pi = fakePi();
        registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
        const { ctx } = fakeCtx();
        await pi.commands.get("goal")!("ship the widget", ctx);

        pi.handlers.get("agent_settled")!({}, ctx);
        assert.equal(clock.pendingCount(), 1, "the settle timer is pending");
        const armedHandle = clock.cancelled.length; // baseline before this cancel

        await trigger(pi, ctx);
        assert.equal(clock.pendingCount(), 0, "cancelled");
        assert.equal(clock.cancelled.length, armedHandle + 1, "clearTimer was called for the pending settle timer");
      });
    }

    test("/goal re-arm cancels a pending settle timer", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);

      await pi.commands.get("goal")!("a new goal entirely", ctx);
      assert.equal(clock.pendingCount(), 0, "re-arming (a fresh /goal) cancels the prior pending settle timer");
    });

    test("force-stop leaves nothing pending — the fire callback that force-stopped already cleared its own timer", () => {
      const threshold1Path = writeConfig("force-stop-threshold-1.json", JSON.stringify({ runaway_threshold: 1 }));
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(threshold1Path), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 1, "force-stopped on the very first no-tool-call settle at threshold 1 — no reminder sent");
      assert.equal(clock.pendingCount(), 0, "nothing left pending after a force-stop");

      // A stopped goal is inert: a further agent_settled arms nothing (state
      // is no longer active).
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 0);
    });

    test("resetCompactionStateForShutdown cancels a pending settle timer", () => {
      const clock = fakeClock();
      const pi = fakePi();
      const handle = registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);

      handle.resetCompactionStateForShutdown();
      assert.equal(clock.pendingCount(), 0, "the shutdown handle cancels the pending settle timer");
    });

    test("leadCompactingRef true AT FIRE TIME yields — status/streak untouched, no send", () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx, statusCalls } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      const statusCallsBeforeFire = statusCalls.length;

      // A compaction starts DURING the settle delay, after the timer armed.
      leadCompactingRef.current = true;
      clock.fire();

      assert.equal(pi.sentUserMessages.length, 1, "yields — compacting at fire time, not sent");
      assert.equal(statusCalls.length, statusCallsBeforeFire, "no additional status-line change on a yield");
      assert.equal(clock.pendingCount(), 0, "the fire callback itself does not re-arm on a yield");
    });

    test("260906 Phase 1 review relay #1 (Important #1): a compaction starting during the settle delay no longer stalls the loop dead", async () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx, statusCalls } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)

      // Live settle arms the timer.
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);
      assert.equal(statusCalls.at(-1)?.value, "Goal loop: settling");

      // An owner-typed /compact (or Pi's own auto-compaction) starts DURING
      // the settle delay, before the timer ever fires — session_before_compact
      // marks leadCompactingRef defensively, exactly as it does for any
      // compaction reason.
      pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
      assert.equal(leadCompactingRef.current, true);

      // The fire callback now finds compacting true: it must yield WITHOUT
      // just dropping this settle on the floor — Pi's ctx.compact() never
      // re-enters _runAgentPrompt, so nothing else will ever re-evaluate the
      // loop for this settle unless the fire callback marks it swallowed.
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 1, "yields — compacting at fire time, nothing sent");
      assert.equal(clock.pendingCount(), 0, "the fire callback itself does not re-arm");

      // session_compact lands; its release is deferred past the microtask.
      pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
      assert.equal(pi.sentUserMessages.length, 1, "nothing sent synchronously inside session_compact itself");
      await new Promise((resolve) => setImmediate(resolve));

      // The deferred release must have re-armed the settle timer — this is
      // the crux of the fix: without settleSwallowedWhileCompacting set at
      // fire time, releaseAfterCompaction's `if (pendingRearm ||
      // settleSwallowedWhileCompacting)` guard would see both false and never
      // re-arm, leaving the goal armed but the loop stalled forever.
      assert.equal(leadCompactingRef.current, false);
      assert.equal(clock.pendingCount(), 1, "the timer was re-armed by the deferred release");
      assert.equal(statusCalls.at(-1)?.value, "Goal loop: settling", "footer is settling again, not stuck on the earlier yield");

      // Firing the re-armed timer sends exactly one ordinary reminder — not a
      // lever/failure-reason reminder, since this was never lever-originated.
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2, "the re-armed timer fires and sends exactly one ordinary reminder");
      const reminder = pi.sentUserMessages[1]!.content as string;
      assert.doesNotMatch(reminder, /Compaction failed/, "an ordinary reinject, not a lever failure reminder");
      assert.match(reminder, /Goal yet running/, "the ordinary reinject wording, proving the loop is not stuck");
    });

    test("the shared wake guard clears on start/settle while an unconfirmed reminder suppresses timeout retry", () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer });
      const { ctx, statusCalls } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(leadWakeStartPendingRef.current, true, "set right before the reminder's own sendUserMessage call");
      assert.equal(clock.pendingCount(), 1, "the boundary-guard fallback timer is now the sole pending timer");

      // Clear point 1: agent_start.
      pi.handlers.get("agent_start")!({}, ctx);
      assert.equal(leadWakeStartPendingRef.current, false);
      assert.equal(clock.pendingCount(), 0, "agent_start cancelled the fallback timer too");
      acknowledgeLatestReminder(pi, ctx);

      // Re-drive to the same point to test clear point 2: agent_settled.
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 1);
      clock.fire();
      assert.equal(leadWakeStartPendingRef.current, true);
      acknowledgeLatestReminder(pi, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(leadWakeStartPendingRef.current, false, "a real settle is proof the reminder's run at least started");
      assert.equal(
        clock.pendingCount(),
        1,
        "the fallback timer was cancelled, but this same settle (state still active, not compacting) immediately arms a fresh settle timer",
      );

      // Re-drive once more to test the fallback timeout itself. Without a
      // matching message_start, the adapter must not hand Pi a duplicate.
      clock.fire(); // sends the reminder, arms the fallback timer
      assert.equal(leadWakeStartPendingRef.current, true);
      const statusCallsBeforeTimeout = statusCalls.length;
      clock.fire(); // no matching message_start ever arrived
      assert.equal(leadWakeStartPendingRef.current, false, "shared reservation cleared by its own timeout");
      assert.deepEqual(
        statusCalls.slice(statusCallsBeforeTimeout).map((c) => c.value),
        ["Goal loop: awaiting reminder handoff"],
      );
      assert.equal(clock.pendingCount(), 0, "unconfirmed reminder handoff is not resubmitted");
    });

    test("the streak advances only on fired reminders — a yielded tick leaves noToolCallStreak untouched", () => {
      const threshold3Path = writeConfig("streak-threshold-3.json", JSON.stringify({ runaway_threshold: 3 }));
      const clock = fakeClock();
      const pi = fakePi();
      const registry = new Map([["child-1", { threadBound: false, running: true }]]) as unknown as RpcAgentRegistry;
      const rpcRegistryRef = { current: registry };
      registerGoalLoop(pi.api, { readConfig: fileReader(threshold3Path), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer, rpcRegistryRef });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);

      // Two yielded ticks (a running child at fire time each time) — matches
      // the existing streak-advance assertion pattern in this file (see the
      // earlier review-relay-2 Critical test's streak-advance leg).
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 1, "both ticks yielded — no reminder sent, streak untouched by either");

      registry.get("child-1")!.running = false;
      // Two ordinary reinjects would force-stop at threshold 3 if the two
      // yielded ticks above had wrongly advanced the streak (0 -> 2, then
      // this pair would push it to 4); since they did not, this pair only
      // reaches streak 2 — still below threshold 3.
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      acknowledgeLatestReminder(pi, ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 3, "two ordinary reinjects — no force-stop yet, since the yields never advanced the streak");
    });
  });

  describe("lead compaction ownership (261002 Phase 1)", () => {
    function leverRun(sessionKey = "lead-key") {
      const clock = fakeClock();
      const pi = fakePi();
      const registry = new Map([
        ["w-1", { agentId: "w-1", alias: "w1", spawnRole: "worker", running: true, streaming: false, reportLog: [] }],
      ]) as unknown as RpcAgentRegistry;
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), ...clock, rpcRegistryRef: { current: registry }, sessionKeyRef: { current: sessionKey } });
      const { ctx, notifications } = fakeCtx();
      return { clock, pi, ctx, notifications };
    }
    const branch = [
      { type: "message", id: "u1", parentId: null, timestamp: "2026-10-02T10:00:00.000Z", message: { role: "user", content: "the human request", timestamp: 0 } },
      { type: "custom_message", id: "c1", parentId: "u1", timestamp: "2026-10-02T10:01:00.000Z", customType: PUSH_BATCH_CUSTOM_TYPE, content: "PUSH BATCH BODY", display: true },
    ];

    test("the lever's compaction returns the adapter summary, keeps no raw tail, and passes Pi's token count through", async () => {
      const { pi, ctx } = leverRun();
      (ctx as unknown as { sessionManager: unknown }).sessionManager = { getSessionFile: () => "/sessions/lead.jsonl" };
      await pi.tools.get("ws-compact")!.execute("c", { current_work: "PROSE", next_step: '"do the thing"' }, undefined, undefined, ctx);
      const result = pi.handlers.get("session_before_compact")!(compactionEvent("manual", branch), ctx) as unknown as { compaction: { summary: string; firstKeptEntryId: string; tokensBefore: number; details: unknown } };
      assert.equal(result.compaction.firstKeptEntryId, NO_KEPT_ENTRY_ID, "Pi's kept-tail cut point is not used");
      assert.equal(result.compaction.tokensBefore, 1234);
      assert.deepEqual(result.compaction.details, { kind: "ws-pi-lead-compaction", version: 1, source: "lever" });
      const summary = result.compaction.summary;
      assert.match(summary, /ws session key: `lead-key`/);
      assert.match(summary, /- w1 \(w-1\) \[worker\]: ticket none named; running/);
      assert.match(summary, /## Dialog\nAll 1 dialog items of this session\.\n.*`\/sessions\/lead\.jsonl`.*\n--- user \(.+\) ---\nthe human request/);
      assert.match(summary, /### Current work\nPROSE/);
      assert.match(summary, /### Immediate next step\n"do the thing"/);
      assert.doesNotMatch(summary, /PUSH BATCH BODY/, "push-batch traffic is not a human message");
      assert.doesNotMatch(summary, /\/tmp\/(read|edit)\.ts|read-files|modified-files/, "no file lists");
    });

    test("the lever's prose is consumed once; a later manual compaction is not answered with it", async () => {
      const { pi, ctx } = leverRun();
      let compactCall: Parameters<ExtensionContext["compact"]>[0] | undefined;
      ctx.compact = (opts) => { compactCall = opts; };
      await pi.tools.get("ws-compact")!.execute("c", { current_work: "PROSE" }, undefined, undefined, ctx);
      assert.ok(pi.handlers.get("session_before_compact")!(compactionEvent("manual", branch), ctx));
      compactCall!.onComplete!({} as never);
      pi.handlers.get("session_compact")!({ reason: "manual", compactionEntry: { details: { kind: "ws-pi-lead-compaction", version: 1, source: "lever" } } }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(pi.handlers.get("session_before_compact")!(compactionEvent("threshold", branch), ctx), undefined, "with no session model a threshold compaction stays native");
    });

    test("worker sessions keep Pi's native compaction and cannot call the lever", async () => {
      const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
      process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
      try {
        const { pi, ctx } = leverRun();
        await assert.rejects(() => pi.tools.get("ws-compact")!.execute("c", { current_work: "x" }, undefined, undefined, ctx), /lead/);
        for (const reason of ["manual", "threshold", "overflow"] as const) {
          assert.equal(pi.handlers.get("session_before_compact")!(compactionEvent(reason, branch), ctx), undefined, reason);
        }
      } finally {
        if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
      }
    });

    test("a summary-build failure degrades to native compaction with a warning", async () => {
      const { pi, ctx, notifications } = leverRun();
      await pi.tools.get("ws-compact")!.execute("c", { current_work: "x" }, undefined, undefined, ctx);
      const broken = { reason: "manual", branchEntries: branch, preparation: undefined } as never;
      assert.equal(pi.handlers.get("session_before_compact")!(broken, ctx), undefined);
      assert.match(notifications.at(-1)!.message, /could not build its summary/);
    });
  });

  describe("no raw kept tail (261003)", () => {
    test("the dialog budget resolves from its byte knob and falls back to 40 KiB", () => {
      assert.equal(resolveDialogBudgetBytes(undefined), DEFAULT_DIALOG_BUDGET_BYTES);
      assert.equal(DEFAULT_DIALOG_BUDGET_BYTES, 40960);
      assert.equal(resolveDialogBudgetBytes({ compaction_dialog_budget_bytes: 512 }), 512);
      for (const bad of [0, -1, Number.NaN, "512"]) assert.equal(resolveDialogBudgetBytes({ compaction_dialog_budget_bytes: bad as never }), DEFAULT_DIALOG_BUDGET_BYTES, String(bad));
    });

    test("appended through Pi's own session manager, the compaction keeps no raw pre-compaction entry and reloads cleanly", () => {
      const dir = mkdtempSync(join(tmpDir, "sessions-"));
      const sm = SessionManager.create(tmpDir, dir);
      const ids = [
        sm.appendMessage({ role: "user", content: "HUMAN ASK", timestamp: 1 } as never),
        sm.appendMessage({ role: "assistant", content: [{ type: "text", text: "LEAD REPLY" }, { type: "toolCall", id: "call-1", name: "Bash", arguments: { command: "ls" } }], timestamp: 2 } as never),
        sm.appendMessage({ role: "toolResult", toolCallId: "call-1", toolName: "Bash", content: [{ type: "text", text: "RAW TOOL OUTPUT" }], isError: false, timestamp: 3 } as never),
        sm.appendMessage({ role: "assistant", content: [{ type: "text", text: "LATEST REPLY" }], timestamp: 4 } as never),
      ];
      const branchEntries = sm.getBranch() as SessionEntry[];
      const result = buildLeadCompactionResult(
        { branchEntries, preparation: { firstKeptEntryId: ids[2]!, tokensBefore: 99 } as never },
        { sessionKey: "k", registry: undefined, prose: prose("PROSE"), dialogBudgetBytes: DEFAULT_DIALOG_BUDGET_BYTES, sessionFile: sm.getSessionFile(), source: "lever" },
      );
      assert.ok(!ids.includes(result.firstKeptEntryId), "the kept-tail id names no session entry");
      assert.match(result.summary, /--- user \(.+\) ---\nHUMAN ASK\n--- assistant \(.+\) ---\nLEAD REPLY\n\u2192 Bash \{"command":"ls"\}\n--- assistant \(.+\) ---\nLATEST REPLY/);
      assert.doesNotMatch(result.summary, /RAW TOOL OUTPUT/);
      assert.ok(result.summary.includes(`\`${sm.getSessionFile()}\``), "the summary names the session file to search");

      sm.appendCompaction(result.summary, result.firstKeptEntryId, result.tokensBefore, result.details, true);
      sm.appendMessage({ role: "user", content: "AFTER COMPACTION", timestamp: 5 } as never);
      const live = sm.buildContextEntries();
      assert.deepEqual(live.map((entry) => entry.type), ["compaction", "message"]);
      assert.ok(!live.some((entry) => ids.includes(entry.id)), "no raw entry from before the compaction");

      const reloaded = SessionManager.open(sm.getSessionFile()!, dir);
      assert.deepEqual(reloaded.buildContextEntries().map((entry) => entry.id), live.map((entry) => entry.id), "a reload rebuilds the same context");
      const messages = reloaded.buildSessionContext().messages as Array<{ role: string; summary?: string }>;
      assert.deepEqual(messages.map((message) => message.role), ["compactionSummary", "user"]);
      assert.equal(messages[0]!.summary, result.summary);
      assert.ok(readFileSync(sm.getSessionFile()!, "utf8").includes("RAW TOOL OUTPUT"), "the session file still holds the tool output");
    });
  });

  describe("lead compaction triggers and backstops (261002 Phase 2)", () => {
    const guidePath = join(tmpDir, "lead-compact-guide-261002.md");
    writeFileSync(guidePath, "GUIDE BODY 261002");

    function triggerRun(configOverride?: GoalLoopConfig) {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: configOverride ? staticConfigReader(configOverride) : fileReader(configPath), ...clock, leadCompactGuidePath: guidePath, sessionKeyRef: { current: "lead-key" } });
      const { ctx, notifications } = fakeCtx();
      const usage = { percent: 10 as number | null };
      (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: usage.percent });
      const preparations = () => pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact");
      const stored = (details: unknown) => ({ reason: "manual", fromExtension: true, compactionEntry: { details } });
      return { clock, pi, ctx, notifications, usage, preparations, stored };
    }
    const ourDetails = { kind: "ws-pi-lead-compaction", version: 1, source: "lever" };

    test("the advisory nudge fires once per crossing at the run's end, carries the guide, and re-arms after compaction", async () => {
      const { pi, ctx, usage, preparations, stored } = triggerRun();
      usage.percent = 49;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 0, "below the advisory point");

      usage.percent = 55;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 0, "the advisory nudge never fires mid-run");
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1);
      const nudge = preparations()[0]!;
      assert.deepEqual(nudge.options, { deliverAs: "followUp", triggerTurn: true });
      assert.match((nudge.content as { content: string }).content, /^Context usage is 55% of the window \(advisory point: 50%\)\. This is a light\nnudge[\s\S]*hard point\n\(80%\), where compaction is no longer optional\.[\s\S]*\n\nGUIDE BODY 261002$/);

      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 60;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1, "an ignored nudge does not repeat within the same crossing");

      pi.handlers.get("session_compact")!(stored(ourDetails), ctx);
      await new Promise((resolve) => setImmediate(resolve));
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 2, "a compaction re-arms the nudge");

      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 30;
      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 52;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 3, "dropping below and crossing again is a new crossing");
    });

    test("the hard cut is a steer at the next turn end, fires once, and suppresses the advisory nudge", () => {
      const { pi, ctx, usage, preparations } = triggerRun({ compaction_advisory_percent: 40, compaction_hard_percent: 70 });
      usage.percent = 71;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 1);
      const steer = preparations()[0]!;
      assert.deepEqual(steer.options, { deliverAs: "steer", triggerTurn: true });
      assert.match((steer.content as { content: string }).content, /^Context usage is 71% .*hard compaction point \(70%\)\. Stop the current work now[\s\S]*GUIDE BODY 261002$/);
      assert.equal((steer.content as { display: boolean }).display, true);

      pi.handlers.get("turn_end")!({}, ctx);
      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 75;
      pi.handlers.get("turn_end")!({}, ctx);
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1, "no re-nudge after the hard cut, and no advisory nudge on top of it");
    });

    test("neither trigger fires while a preparation turn or a compaction is in progress", () => {
      const { pi, ctx, usage, preparations } = triggerRun();
      usage.percent = 60;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1);
      usage.percent = 95;
      pi.handlers.get("turn_end")!({}, ctx);
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 1, "the preparation turn is running");
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 2, "an ignored preparation turn has ended; the hard crossing still fires");

      pi.handlers.get("session_before_compact")!(compactionEvent("threshold"), ctx);
      pi.handlers.get("agent_end")!({}, ctx);
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(leadCompactingRef.current, true);
      assert.equal(preparations().length, 2, "a compaction in progress blocks both triggers");
    });

    test("a preparation message dropped before it ran (an abort clears Pi's queues) does not disable the triggers", () => {
      const { pi, ctx, usage, preparations } = triggerRun();
      usage.percent = 85;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 1, "hard steer queued");
      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 40;
      pi.handlers.get("agent_end")!({}, ctx);
      usage.percent = 90;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 2, "the next crossing fires again");
    });

    test("spawned sessions never receive a preparation trigger", () => {
      const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
      process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
      try {
        const { pi, ctx, usage, preparations } = triggerRun();
        usage.percent = 99;
        pi.handlers.get("turn_end")!({}, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        assert.equal(preparations().length, 0);
      } finally {
        if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
      }
    });

    test("a user /compact is cancelled and rerouted into a preparation turn carrying its focus text", async () => {
      const { pi, ctx, notifications, preparations } = triggerRun();
      const result = pi.handlers.get("session_before_compact")!(compactionEvent("manual", [], { customInstructions: "keep the API notes" }), ctx);
      assert.deepEqual(result, { cancel: true });
      assert.equal(leadCompactingRef.current, true, "the hold still covers the cancelled compaction");
      assert.equal(preparations().length, 0, "nothing is sent from inside the compaction event");

      pi.handlers.get("session_compact_failed")!({ reason: "manual", aborted: true, errorMessage: undefined, willRetry: false, fromExtension: false }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(leadCompactingRef.current, false, "the cancel releases the hold");
      assert.equal(preparations().length, 1);
      const reroute = preparations()[0]!;
      assert.deepEqual(reroute.options, { deliverAs: "followUp", triggerTurn: true });
      assert.match((reroute.content as { content: string }).content, /^The user ran \/compact[\s\S]*focus text, to honor in your prose:\nkeep the API notes\n\nGUIDE BODY 261002$/);
      assert.deepEqual(notifications, [], "no failure is reported for the reroute");

      let compactCall: Parameters<ExtensionContext["compact"]>[0] | undefined;
      ctx.compact = (opts) => { compactCall = opts; };
      await pi.tools.get("ws-compact")!.execute("c", { current_work: "prepared" }, undefined, undefined, ctx);
      const own = pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx) as unknown as { compaction?: { summary: string } };
      assert.match(own.compaction!.summary, /prepared/, "the lever's own manual compaction is not rerouted");
      assert.ok(compactCall);
    });

    test("a threshold or overflow compaction without preparation gets the fallback summary from the session model", async () => {
      for (const reason of ["threshold", "overflow"] as const) {
        const { pi, ctx } = triggerRun();
        const calls: Array<{ model: unknown; context: { systemPrompt?: string; messages: Array<{ role: string; content: Array<{ text: string }> }> }; options: Record<string, unknown> }> = [];
        const model = { id: "session-model", maxTokens: 4096 };
        Object.assign(ctx, {
          model,
          modelRegistry: {
            complete: async (m: unknown, context: never, options: never) => {
              calls.push({ model: m, context, options });
              return { role: "assistant", stopReason: "stop", usage: { input: 1, output: 2 }, content: [{ type: "text", text: "### Current work\nFALLBACK PROSE" }] };
            },
          },
        });
        const branch = [{ type: "message", id: "u1", parentId: null, timestamp: "2026-10-02T10:00:00.000Z", message: { role: "user", content: "HUMAN ASK", timestamp: 0 } }];
        const event = compactionEvent(reason, branch);
        (event as { preparation: { messagesToSummarize: unknown[]; previousSummary?: string } }).preparation.messagesToSummarize = [{ role: "user", content: "summarize me", timestamp: 0 }];
        const result = await (pi.handlers.get("session_before_compact")!(event, ctx) as unknown as Promise<{ compaction: { summary: string; firstKeptEntryId: string; details: unknown; usage: unknown } }>);
        assert.equal(calls.length, 1, reason);
        assert.equal(calls[0]!.model, model, "the session model");
        assert.match(calls[0]!.context.systemPrompt!, /context summarization assistant/);
        assert.match(calls[0]!.context.messages[0]!.content[0]!.text, /<conversation>\n\[User\]: summarize me\n<\/conversation>/);
        assert.equal(calls[0]!.options.maxTokens, 4096, "capped by the model's own output limit");
        assert.equal((calls[0]!.options as { signal?: unknown }).signal !== undefined, true);
        assert.equal(result.compaction.firstKeptEntryId, NO_KEPT_ENTRY_ID, "the fallback keeps no raw tail either");
        assert.deepEqual(result.compaction.details, { kind: "ws-pi-lead-compaction", version: 1, source: "fallback" });
        assert.deepEqual(result.compaction.usage, { input: 1, output: 2 });
        const summary = result.compaction.summary;
        assert.match(summary, /ws session key: `lead-key`/);
        assert.match(summary, /HUMAN ASK/);
        assert.match(summary, /## Carried forward by the lead\n### Current work\nFALLBACK PROSE/);
        assert.doesNotMatch(summary, /\/tmp\/(read|edit)\.ts/);
      }
    });

    test("a failed fallback call degrades to native compaction with a warning", async () => {
      const { pi, ctx, notifications } = triggerRun();
      Object.assign(ctx, {
        model: { id: "m", maxTokens: 0 },
        modelRegistry: { complete: async () => ({ role: "assistant", stopReason: "error", errorMessage: "provider down", content: [] }) },
      });
      const result = await (pi.handlers.get("session_before_compact")!(compactionEvent("threshold"), ctx) as unknown as Promise<unknown>);
      assert.equal(result, undefined);
      assert.match(notifications.at(-1)!.message, /fallback summary failed \(provider down\)/);
    });

    test("the competing-extension notice fires once per session when the stored entry is not ours", async () => {
      const { pi, ctx, notifications, stored } = triggerRun();
      const competing = () => notifications.filter((n) => /Another extension's compaction replaced/.test(n.message));
      pi.handlers.get("session_compact")!({ reason: "threshold", fromExtension: false, compactionEntry: { details: { readFiles: [] } } }, ctx);
      assert.equal(competing().length, 0, "Pi's native summary after our fallback declined is not a competitor");

      await pi.tools.get("ws-compact")!.execute("c", { current_work: "x" }, undefined, undefined, ctx);
      pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx);
      pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: false, compactionEntry: { details: undefined } }, ctx);
      assert.equal(competing().length, 1, "our result was overridden by another extension's empty result");
      assert.equal(competing()[0]!.level, "warning");

      pi.handlers.get("session_compact")!(stored({ other: "extension" }), ctx);
      assert.equal(competing().length, 1, "once per session");
      pi.handlers.get("session_compact")!(stored(ourDetails), ctx);
      assert.equal(competing().length, 1);
      await new Promise((resolve) => setImmediate(resolve));
    });
  });

  describe("adapter settings read through ws-mcp config.get", () => {
    /**
     * A fake bridge client answering `config.get` as ws-mcp does: a tuned
     * `pi.*` key returns its typed value, an untuned one the manifest
     * default, which the reader then sees as present. `reachable: false`
     * rejects every call, as a dead launcher does.
     */
    function fakeWsStore(tuned: Record<string, unknown>, reachable = true) {
      const calls: Array<Record<string, unknown>> = [];
      const client = {
        async callTool(name: string, args: Record<string, unknown>) {
          calls.push({ name, ...args });
          if (!reachable) throw new Error("ws-mcp exited");
          const key = String(args.key);
          const value = key in tuned ? tuned[key] : null;
          return { content: [{ type: "text" as const, text: JSON.stringify({ key, value, scope: key in tuned ? "session" : "unset" }) }] };
        },
      };
      return { calls, reader: createWsConfigReader(() => ({ client, sessionKey: "lead-key" }), 50) };
    }
    const settle = () => new Promise((resolve) => setImmediate(resolve));

    test("a tuned settle delay and runaway threshold drive the goal loop", async () => {
      const store = fakeWsStore({ "pi.settle_delay_ms": 1500, "pi.runaway_threshold": 1 });
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: store.reader, ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship the widget", ctx);

      pi.handlers.get("agent_settled")!({}, ctx);
      assert.equal(clock.pendingCount(), 0, "the timer waits for the read");
      await settle();
      assert.deepEqual(clock.scheduledDelays, [1500], "the tuned delay, not the default");
      assert.ok(store.calls.every((call) => call.name === "config.get" && call.session_key === "lead-key" && String(call.key).startsWith("pi.")));

      clock.fire();
      assert.equal(pi.sentUserMessages.length, 1, "a tuned threshold of 1 force-stops on the first tool-less settle; only the goal announcement was sent");
      assert.equal(clock.pendingCount(), 0);
    });

    test("an agent_start during the settle read schedules nothing", async () => {
      const store = fakeWsStore({});
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: store.reader, ...clock });
      const { ctx } = fakeCtx();
      await pi.commands.get("goal")!("ship the widget", ctx);
      pi.handlers.get("agent_settled")!({}, ctx);
      pi.handlers.get("agent_start")!({}, ctx);
      await settle();
      assert.equal(clock.pendingCount(), 0);
    });

    test("a tuned hard percent moves the compaction steer", async () => {
      const store = fakeWsStore({ "pi.compaction_hard_percent": 60 });
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: store.reader, ...fakeClock() });
      const { ctx } = fakeCtx();
      (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: 65 });
      await pi.handlers.get("turn_end")!({}, ctx);
      const steers = pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact");
      assert.equal(steers.length, 1, "65% crosses the tuned 60% hard point (the default is 80%)");
      assert.match((steers[0]!.content as { content: string }).content, /hard compaction point \(60%\)/);
    });

    /** A reader whose answers wait until the test releases them. */
    function deferredReader(config: GoalLoopConfig = {}) {
      const waiting: Array<() => void> = [];
      const reader: GoalLoopConfigReader = (keys) => new Promise((resolve) => { waiting.push(() => resolve(staticConfigReader(config)(keys) as GoalLoopConfig)); });
      return { reader, release: () => { for (const go of waiting.splice(0)) go(); } };
    }

    test("a compaction that starts during the trigger read suppresses the steer", async () => {
      const deferred = deferredReader();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: deferred.reader, ...fakeClock() });
      const { ctx } = fakeCtx();
      (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: 90 });
      const pending = pi.handlers.get("turn_end")!({}, ctx);
      leadCompactingRef.current = true;
      deferred.release();
      await pending;
      assert.equal(pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact").length, 0);
    });

    test("the lever's compaction result waits on the budget read", async () => {
      const deferred = deferredReader({ compaction_dialog_budget_bytes: 100 });
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: deferred.reader, ...fakeClock(), sessionKeyRef: { current: "lead-key" } });
      const { ctx } = fakeCtx();
      await pi.tools.get("ws-compact")!.execute("c", { current_work: "x" }, undefined, undefined, ctx);
      const pending = pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx) as unknown as Promise<{ compaction: { details: { source: string } } } | undefined>;
      assert.ok(pending instanceof Promise, "an asynchronous reader makes the hook asynchronous");
      deferred.release();
      const result = await pending;
      assert.equal(result?.compaction.details.source, "lever");
      pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: true, compactionEntry: { details: result!.compaction.details } }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
    });

    test("unreachable ws-mcp leaves every knob at its default", async () => {
      for (const reader of [
        fakeWsStore({ "pi.settle_delay_ms": 1500 }, false).reader,
        createWsConfigReader(() => undefined),
        createWsConfigReader(() => ({ client: { callTool: () => new Promise<never>(() => {}) } }), 5),
      ]) {
        const clock = fakeClock();
        const pi = fakePi();
        registerGoalLoop(pi.api, { readConfig: reader, ...clock });
        const { ctx } = fakeCtx();
        (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: 65 });
        await pi.commands.get("goal")!("ship the widget", ctx);
        pi.handlers.get("agent_settled")!({}, ctx);
        await new Promise((resolve) => setTimeout(resolve, 20));
        assert.deepEqual(clock.scheduledDelays, [DEFAULT_SETTLE_DELAY_MS]);
        await pi.handlers.get("turn_end")!({}, ctx);
        assert.equal(pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact").length, 0, "65% is under the default hard point");
      }
    });
  });
});
