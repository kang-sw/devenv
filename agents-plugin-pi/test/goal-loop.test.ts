/**
 * Unit tests for goal-loop.ts's pure exports (config reader, threshold/
 * advisory-percent/context-window-override resolvers, message builders, the
 * `computeContextPercent` context-usage helper, the state machine, and the
 * spawned-child `isChildProcess` predicate — review fix, cycle 1: extracted
 * from the `agent_settled` handler's inline `process.env` check so the
 * lead-session-only guard has automated positive/negative coverage instead
 * of relying solely on a manual spot-check). No `pi.*` IO is exercised here
 * — `registerGoalLoop`'s IO glue (including the new `goal-compact-and-continue`
 * tool and `session_before_compact` listener) is covered by the live
 * `pi --mode json` gate (see the 260903 Phase 1/2 plans' Verification Plans),
 * not by this unit suite. The companion env-marker-placement coverage
 * (`buildRpcClientOptions`/`buildChildProcessEnv`) lives in
 * `test/spawner.test.ts`.
 *
 * Phase 2 (260903) note: `decideOnSettle`'s `"reinject"` decision now carries
 * only the bare `goal` string, not a precomputed reminder — see that
 * function's own doc comment in goal-loop.ts for why (`ctx.getContextUsage()`/
 * the config file are IO-context-only). The old embedded-reminder assertions
 * below were updated to the new `{ action: "reinject", goal }` shape.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, after, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import {
  readGoalLoopConfig,
  resolveRunawayThreshold,
  resolveCompactionAdvisoryPercent,
  resolveContextWindowOverride,
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
  DEFAULT_RUNAWAY_THRESHOLD,
  DEFAULT_COMPACTION_ADVISORY_PERCENT,
  type GoalLoopConfig,
} from "../src/goal-loop.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { leadCompactingRef, heldPushQueue } from "../src/spawner.ts";

const tmpDir = mkdtempSync(join(tmpdir(), "ws-goal-loop-test-"));
after(() => {
  rmSync(tmpDir, { recursive: true, force: true });
});

function writeConfig(name: string, contents: string): string {
  const path = join(tmpDir, name);
  writeFileSync(path, contents, "utf8");
  return path;
}

describe("readGoalLoopConfig", () => {
  test("missing file returns undefined (never throws)", () => {
    const path = join(tmpDir, "does-not-exist.json");
    assert.doesNotThrow(() => readGoalLoopConfig(path));
    assert.equal(readGoalLoopConfig(path), undefined);
  });

  test("empty {} file parses to an empty object", () => {
    const path = writeConfig("empty.json", "{}");
    assert.deepEqual(readGoalLoopConfig(path), {});
  });

  test("populated file parses runaway_threshold", () => {
    const config: GoalLoopConfig = { runaway_threshold: 3 };
    const path = writeConfig("populated.json", JSON.stringify(config));
    assert.deepEqual(readGoalLoopConfig(path), config);
  });

  test("malformed JSON returns undefined (never throws)", () => {
    const path = writeConfig("malformed.json", "{not valid json");
    assert.doesNotThrow(() => readGoalLoopConfig(path));
    assert.equal(readGoalLoopConfig(path), undefined);
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
  test("names the requested compaction and carries the carry-forward argument verbatim", () => {
    const text = buildCompactionLeverResult("phase 1 done, phase 2 next");
    assert.ok(text.startsWith("Compaction requested"));
    assert.ok(text.includes("phase 1 done, phase 2 next"));
  });
});

describe("buildGoalReminder", () => {
  const info = { percent: 42, advisoryPercent: 70 };

  test("names the goal and all three lever tool names (two terminal, one compact-and-continue)", () => {
    const reminder = buildGoalReminder("ship the widget", info);
    assert.match(reminder, /ship the widget/);
    assert.match(reminder, /goal-achieved/);
    assert.match(reminder, /goal-blocked/);
    assert.match(reminder, /goal-compact-and-continue/);
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
    const reminder = buildGoalReminder("a goal", { percent: 42, advisoryPercent: 70 });
    assert.match(reminder, /Context usage: 42% of window — below the compaction advisory point \(70%\); do not call goal-compact-and-continue\.$/m);
  });

  test("percent at the advisory point renders the stronger nudge phrase", () => {
    const reminder = buildGoalReminder("a goal", { percent: 70, advisoryPercent: 70 });
    assert.match(reminder, /Context usage: 70% of window — at or above the advisory point/);
  });

  test("percent above the advisory point renders the stronger nudge phrase", () => {
    const reminder = buildGoalReminder("a goal", { percent: 85, advisoryPercent: 70 });
    assert.match(reminder, /Context usage: 85% of window — at or above the advisory point/);
  });

  test("null percent renders as unknown, not a crash", () => {
    const reminder = buildGoalReminder("a goal", { percent: null, advisoryPercent: 70 });
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

  test("is explicitly advisory-only, never a veto/override", () => {
    const observation = buildCompactionObservation("a goal", "threshold");
    assert.match(observation, /does not cancel or override/);
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
 * 260906 (compaction push-hold ticket, Phase 1): `registerGoalLoop`'s IO glue
 * around the compaction race — previously left to the live `pi --mode json`
 * gate per this file's own top-of-file doc comment, now covered here with a
 * fake-`pi` + duck-typed `ctx` harness, mirroring `test/ask.test.ts`'s
 * `describe("closeThreadOnDone / injectDiscussionSummary (fake pi)")` shape.
 * `leadCompactingRef`/`heldPushQueue` are module state shared with
 * `spawner.ts`, so every test here resets both.
 */
describe("registerGoalLoop IO glue (fake pi): compaction release (260906 Phase 1)", () => {
  beforeEach(() => {
    leadCompactingRef.current = false;
    heldPushQueue.length = 0;
  });

  afterEach(() => {
    leadCompactingRef.current = false;
    heldPushQueue.length = 0;
  });

  const configPath = join(tmpDir, "does-not-exist-compaction-260906.json");

  function fakePi(): {
    api: ExtensionAPI;
    handlers: Map<string, (event: unknown, ctx: ExtensionContext) => void>;
    commands: Map<string, (args: string, ctx: ExtensionContext) => Promise<void> | void>;
    tools: Map<string, { execute: (...args: unknown[]) => Promise<unknown> }>;
    sentUserMessages: Array<{ content: unknown; options?: unknown }>;
  } {
    const handlers = new Map<string, (event: unknown, ctx: ExtensionContext) => void>();
    const commands = new Map<string, (args: string, ctx: ExtensionContext) => Promise<void> | void>();
    const tools = new Map<string, { execute: (...args: unknown[]) => Promise<unknown> }>();
    const sentUserMessages: Array<{ content: unknown; options?: unknown }> = [];
    const api = {
      on: (event: string, handler: (event: unknown, ctx: ExtensionContext) => void) => {
        handlers.set(event, handler);
      },
      registerCommand: (name: string, def: { handler: (args: string, ctx: ExtensionContext) => Promise<void> | void }) => {
        commands.set(name, def.handler);
      },
      registerTool: (def: { name: string; execute: (...args: unknown[]) => Promise<unknown> }) => {
        tools.set(def.name, def);
      },
      sendUserMessage: (content: unknown, options?: unknown) => {
        sentUserMessages.push({ content, options });
      },
    };
    return { api: api as unknown as ExtensionAPI, handlers, commands, tools, sentUserMessages };
  }

  /** A duck-typed `ExtensionContext` with a no-op `compact` — tests override it to capture the lever's callbacks. */
  function fakeCtx(isIdle: () => boolean = () => true): {
    ctx: ExtensionContext;
    notifications: Array<{ message: string; level: string }>;
  } {
    const notifications: Array<{ message: string; level: string }> = [];
    const ctx = {
      ui: {
        notify: (message: string, level: string) => notifications.push({ message, level }),
        setStatus: () => {},
      },
      isIdle,
      getContextUsage: () => undefined,
      compact: () => {},
    };
    return { ctx: ctx as unknown as ExtensionContext, notifications };
  }

  test("release runs once when both the lever's onComplete and session_compact arrive", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    assert.equal(pi.sentUserMessages.length, 1, "the armed announcement");

    let compactCall: { onComplete?: () => void; onError?: (error: Error) => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("goal-compact-and-continue")!.execute("call-1", { carry_forward: "phase 1 done" }, undefined, undefined, ctx);
    assert.equal(leadCompactingRef.current, true, "set before ctx.compact is even called");

    compactCall!.onComplete!();
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 2, "the re-armed reminder");

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 2, "already released by onComplete — session_compact's own release is a no-op");
  });

  test("release triggered by session_compact is deferred — nothing sent synchronously inside that handler", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = () => {};
    await pi.tools.get("goal-compact-and-continue")!.execute("call-1", { carry_forward: "x" }, undefined, undefined, ctx);
    assert.equal(pi.sentUserMessages.length, 1, "only the armed announcement so far");

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    assert.equal(pi.sentUserMessages.length, 1, "nothing sent synchronously inside the session_compact handler itself");
    assert.equal(leadCompactingRef.current, true, "still marked compacting until the deferred release runs");

    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(pi.sentUserMessages.length, 2, "the deferred release ran after the macrotask queue drained");
    assert.equal(leadCompactingRef.current, false);
  });

  test("onError alone releases with the failure reason folded into the reminder", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx);
    let compactCall: { onError?: (error: Error) => void } | undefined;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts as never;
    };
    await pi.tools.get("goal-compact-and-continue")!.execute("call-1", { carry_forward: "x" }, undefined, undefined, ctx);

    compactCall!.onError!(new Error("boom"));
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 2);
    const reminder = pi.sentUserMessages[1]!.content as string;
    assert.match(reminder, /Compaction failed: boom/);
    assert.match(reminder, /Do not retry goal-compact-and-continue/);
  });

  test("agent_start clears the flag without sending a reminder or touching the held queue", () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx();

    // A defensively-set flag (e.g. an owner-typed /compact) with the goal
    // never even armed — the backstop must still fire.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    assert.equal(leadCompactingRef.current, true);

    let flushed = false;
    heldPushQueue.push({ kind: "raw", send: () => { flushed = true; } });

    pi.handlers.get("agent_start")!({}, ctx);
    assert.equal(leadCompactingRef.current, false);
    assert.equal(pi.sentUserMessages.length, 0, "no reminder — this is a pure backstop clear");
    assert.equal(flushed, false, "no queue touch either — that is releaseAfterCompaction's job, not this backstop's");
    assert.equal(heldPushQueue.length, 1, "left untouched for that turn's own settle/flush handler");
  });

  test("a non-lever session_compact (no pendingRearm) releases held pushes but sends no reminder", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx();

    await pi.commands.get("goal")!("ship the widget", ctx); // state active; 1 message so far (armed)

    // Owner-typed /compact: session_before_compact sets the flag defensively,
    // but pendingRearm is never set — the lever was never called.
    pi.handlers.get("session_before_compact")!({ reason: "manual" }, ctx);
    assert.equal(leadCompactingRef.current, true);

    let flushed = false;
    heldPushQueue.push({ kind: "raw", send: () => { flushed = true; } });

    pi.handlers.get("session_compact")!({ reason: "manual" }, ctx);
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(leadCompactingRef.current, false);
    assert.equal(flushed, true, "held pushes still release on any compaction, lever-originated or not");
    assert.equal(pi.sentUserMessages.length, 1, "still just the armed announcement — no synthesized reminder for a non-lever compaction");
  });

  test("release while the agent is not idle sends nothing; a subsequent settle re-arms the loop normally", async () => {
    const pi = fakePi();
    registerGoalLoop(pi.api, { goalLoopConfigPath: configPath });
    const { ctx } = fakeCtx(() => true);

    await pi.commands.get("goal")!("ship the widget", ctx); // 1 message (armed)
    let compactCall: unknown;
    (ctx as unknown as { compact: (opts: unknown) => void }).compact = (opts: unknown) => {
      compactCall = opts;
    };
    await pi.tools.get("goal-compact-and-continue")!.execute("call-1", { carry_forward: "x" }, undefined, undefined, ctx);
    assert.ok(compactCall, "ctx.compact was called");
    assert.equal(leadCompactingRef.current, true);

    let flushed = false;
    heldPushQueue.push({ kind: "raw", send: () => { flushed = true; } });

    // The release-time ctx reports NOT idle — e.g. agent_start's own backstop
    // raced this call and a fresh turn is already underway.
    const notIdle = fakeCtx(() => false).ctx;
    pi.handlers.get("session_compact")!({ reason: "manual" }, notIdle);
    await new Promise((resolve) => setImmediate(resolve));

    assert.equal(leadCompactingRef.current, false, "the flag is still cleared even when nothing else fires");
    assert.equal(pi.sentUserMessages.length, 1, "nothing sent while the agent already looks busy again");
    assert.equal(flushed, false, "the held queue is left for that turn's own settle, not drained here");

    // A subsequent settle re-evaluates normally: leadCompactingRef is false
    // again, so decideOnSettle sees compacting=false and reinjects as usual.
    const settled = fakeCtx(() => true).ctx;
    pi.handlers.get("agent_settled")!({}, settled);
    assert.equal(pi.sentUserMessages.length, 2, "the ordinary reinject reminder fires on the next settle");
  });
});
