import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  resolveAgentWaitAnimation,
  resolveRunawayThreshold,
  parseCompactionThreshold,
  resolveCompactionAdvisoryThreshold,
  resolveCompactionHardThreshold,
  resolveContextWindow,
  resolveThresholdPercent,
  goalReminderUsage,
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
  DEFAULT_RUNAWAY_THRESHOLD,
  DEFAULT_COMPACTION_ADVISORY_PERCENT,
  DEFAULT_COMPACTION_HARD_PERCENT,
  DEFAULT_SETTLE_DELAY_MS,
  DEFAULT_CHILD_RETENTION_TTL_DAYS,
} from "../src/goal-loop.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";

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

describe("parseCompactionThreshold", () => {
  test("parses each form of the grammar", () => {
    assert.deepEqual(parseCompactionThreshold("50"), { percent: 50 });
    assert.deepEqual(parseCompactionThreshold("50%"), { percent: 50 });
    assert.deepEqual(parseCompactionThreshold("150k"), { tokens: 150_000 });
    assert.deepEqual(parseCompactionThreshold("1.5m"), { tokens: 1_500_000 });
    assert.deepEqual(parseCompactionThreshold("50%|150k"), { percent: 50, tokens: 150_000 });
    assert.deepEqual(parseCompactionThreshold("150k|50%"), { percent: 50, tokens: 150_000 });
    assert.deepEqual(parseCompactionThreshold("0.5k"), { tokens: 500 });
    assert.deepEqual(parseCompactionThreshold("1.0004k"), { tokens: 1_000 }, "rounded to the nearest whole token");
    assert.deepEqual(parseCompactionThreshold("1.0006k"), { tokens: 1_001 });
  });

  test("keeps the old 1-100 integer range for percent terms", () => {
    assert.deepEqual(parseCompactionThreshold("1"), { percent: 1 });
    assert.deepEqual(parseCompactionThreshold("100"), { percent: 100 });
    for (const value of ["0", "0%", "101", "101%", "50.5", "050", "-5"]) assert.equal(parseCompactionThreshold(value), undefined, value);
  });

  test("rejects anything outside the grammar", () => {
    for (const value of ["", "0k", "0.0m", "150K", "1.5M", "150", "50%|60%", "150k|1m", "50%|150k|1m", "50% | 150k", " 50", "50 ", "15ok", "k", ".5k", "5.k", "50%%", "|50%", "50%|"]) {
      assert.equal(parseCompactionThreshold(value), undefined, JSON.stringify(value));
    }
  });

  test("a finite number in (0, 100] is a percent; other values are rejected", () => {
    assert.deepEqual(parseCompactionThreshold(55), { percent: 55 });
    assert.deepEqual(parseCompactionThreshold(100), { percent: 100 });
    for (const value of [0, -5, 101, Number.NaN, Number.POSITIVE_INFINITY, undefined, null, true, {}]) assert.equal(parseCompactionThreshold(value), undefined, String(value));
  });
});

describe("resolveCompactionAdvisoryThreshold / resolveCompactionHardThreshold", () => {
  test("an absent value falls back to the default percent", () => {
    assert.deepEqual(resolveCompactionAdvisoryThreshold(undefined), { percent: DEFAULT_COMPACTION_ADVISORY_PERCENT });
    assert.deepEqual(resolveCompactionAdvisoryThreshold({}), { percent: DEFAULT_COMPACTION_ADVISORY_PERCENT });
    assert.deepEqual(resolveCompactionHardThreshold({}), { percent: DEFAULT_COMPACTION_HARD_PERCENT });
  });

  test("stored bare numbers keep their percent meaning", () => {
    assert.deepEqual(resolveCompactionAdvisoryThreshold({ compaction_advisory_percent: "50" }), { percent: 50 });
    assert.deepEqual(resolveCompactionHardThreshold({ compaction_hard_percent: "80" }), { percent: 80 });
    assert.deepEqual(resolveCompactionAdvisoryThreshold({ compaction_advisory_percent: 55 }), { percent: 55 });
  });

  test("token and combined forms resolve", () => {
    assert.deepEqual(resolveCompactionAdvisoryThreshold({ compaction_advisory_percent: "150k" }), { tokens: 150_000 });
    assert.deepEqual(resolveCompactionHardThreshold({ compaction_hard_percent: "80%|400k" }), { percent: 80, tokens: 400_000 });
  });

  test("a malformed value falls back to the default (never hard-fail)", () => {
    for (const value of ["15ok", "0", "101", "50%|60%", 0, -5, 101, Number.NaN]) {
      assert.deepEqual(resolveCompactionAdvisoryThreshold({ compaction_advisory_percent: value }), { percent: DEFAULT_COMPACTION_ADVISORY_PERCENT }, String(value));
      assert.deepEqual(resolveCompactionHardThreshold({ compaction_hard_percent: value }), { percent: DEFAULT_COMPACTION_HARD_PERCENT }, String(value));
    }
  });
});

describe("resolveThresholdPercent", () => {
  test("a percent term is used as-is; a token term is tokens / window", () => {
    assert.equal(resolveThresholdPercent({ percent: 50 }, 1_000_000), 50);
    assert.equal(resolveThresholdPercent({ tokens: 150_000 }, 1_000_000), 15);
    assert.equal(resolveThresholdPercent({ tokens: 150_000 }, 200_000), 75);
  });

  test("the combined form takes whichever side is reached first", () => {
    const spec = { percent: 50, tokens: 150_000 };
    assert.equal(resolveThresholdPercent(spec, 1_000_000), 15, "on a large window the token side wins");
    assert.equal(resolveThresholdPercent(spec, 200_000), 50, "on a small window the percent side wins");
  });

  test("a token threshold beyond the window is not clamped", () => {
    assert.equal(resolveThresholdPercent({ tokens: 300_000 }, 200_000), 150);
    assert.equal(resolveThresholdPercent({ percent: 80, tokens: 300_000 }, 200_000), 80);
  });

  test("with no known window a token term never fires", () => {
    assert.equal(resolveThresholdPercent({ tokens: 150_000 }, undefined), Number.POSITIVE_INFINITY);
    assert.equal(resolveThresholdPercent({ percent: 50, tokens: 150_000 }, undefined), 50);
  });
});

describe("resolveContextWindow", () => {
  test("the override wins over the model window; an invalid window is undefined", () => {
    const usage = { tokens: 10, contextWindow: 1_000_000, percent: 0.001 };
    assert.equal(resolveContextWindow(usage, {}), 1_000_000);
    assert.equal(resolveContextWindow(usage, { context_window_override: 200_000 }), 200_000);
    assert.equal(resolveContextWindow(undefined, {}), undefined);
    assert.equal(resolveContextWindow({ tokens: null, contextWindow: 0, percent: null }, {}), undefined);
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

  test("with a known window the advisory point is shown in percent and tokens", () => {
    const reminder = buildGoalReminder("a goal", { percent: 12, advisoryPercent: 15, contextWindow: 1_000_000 });
    assert.match(reminder, /Context usage: 12% of window — below the compaction advisory point \(15% \/ 150k tokens\); do not call ws-compact\.$/m);
  });
});

describe("goalReminderUsage", () => {
  test("resolves the advisory threshold against the same window as the usage", () => {
    const usage = { tokens: 200_000, contextWindow: 1_000_000, percent: 20 };
    assert.deepEqual(goalReminderUsage(usage, { compaction_advisory_percent: "150k" }), { percent: 20, advisoryPercent: 15, contextWindow: 1_000_000 });
    assert.deepEqual(goalReminderUsage(usage, { compaction_advisory_percent: "150k", context_window_override: 400_000 }), { percent: 50, advisoryPercent: 37.5, contextWindow: 400_000 });
    assert.match(buildGoalReminder("g", goalReminderUsage(usage, { compaction_advisory_percent: "50%|150k" })), /at or above the advisory point \(15% \/ 150k tokens\)/);
  });

  test("defaults to the 50% advisory and reports unknown usage as null", () => {
    assert.deepEqual(goalReminderUsage({ tokens: null, contextWindow: 1000, percent: null }, {}), { percent: null, advisoryPercent: 50, contextWindow: 1000 });
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
