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
import { rmSync } from "node:fs";
import { join } from "node:path";
import { type ExtensionAPI, type ExtensionContext } from "@earendil-works/pi-coding-agent";
import {
  buildCompactionResumeMessage,
  armGoal,
  recordToolCall,
  decideOnSettle,
  registerGoalLoop,
} from "../src/goal-loop.ts";
import { flushHeldPushes, leadIdleRef, clearWakeStart, leadCompactingRef, leadWakeStartPendingRef, heldPushQueue, isOwningAgentIdle, type RpcAgentRegistry } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { labeled, prose, compactionEvent, tmpDir, writeConfig, fileReader } from "./fixtures/goal-loop.ts";

after(() => {
  rmSync(tmpDir, { recursive: true, force: true });
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
        assert.equal(pi.sentUserMessages.at(-1)!.content, labeled(`Goal armed: ${text}`));
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
      assert.deepEqual(pi.sentUserMessages.map((message) => message.content), [labeled("Goal armed: old"), labeled("Goal armed: new")]);

      pi.handlers.get("agent_settled")!({}, ctx);
      clock.fire();
      assert.match(pi.sentUserMessages.at(-1)!.content as string, /^\[system message from ws-pi-plugin\]\nGoal yet running: "new"/, "the goal reminder opens with the adapter label");
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
      assert.deepEqual(pi.sentUserMessages.map((message) => message.content), [labeled("Goal armed: ship")]);

      const recoveredClock = fakeClock();
      const recoveredPi = fakePi();
      registerGoalLoop(recoveredPi.api, { readConfig: fileReader(configPath), ...recoveredClock });
      const { ctx: recoveredCtx } = fakeCtx();
      recoveredPi.handlers.get("agent_settled")!({}, recoveredCtx);
      assert.equal(recoveredClock.pendingCount(), 0, "session recovery does not restore the stopped goal or its timer");
      assert.deepEqual(recoveredPi.sentUserMessages, [], "the first recovered settle cannot create an automatic turn");
    });
  });

  test("ws-compact compacts with no active goal without touching goal-loop state (reverses 260913's rejection) and resumes the lead once (261003)", async () => {
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
    assert.deepEqual(pi.sentUserMessages, [{ content: labeled(buildCompactionResumeMessage(undefined)), options: { deliverAs: "followUp" } }], "an autonomous lever call is followed by exactly one resume message, never a goal reminder");

    await pi.commands.get("goal")!("ship", ctx);
    pi.handlers.get("agent_settled")!({}, ctx);
    clock.fire();
    assert.equal(pi.sentUserMessages.length, 3, "a later active goal still follows the ordinary reminder path");
    assert.ok(!(pi.sentUserMessages[2]!.content as string).includes(carryHeading), "the goal-less prose never becomes a goal carry");
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
        assert.match(pi.sentUserMessages[1]!.content as string, /^\[system message from ws-pi-plugin\]\nCompaction failed: boom Do not retry/);
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
});
