import { test, describe, after, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { abortBackgroundTask, killAllBackgroundTasks, startBackgroundTask } from "../src/background-tasks.ts";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { SessionManager, type ExtensionAPI, type ExtensionContext, type SessionEntry } from "@earendil-works/pi-coding-agent";
import {
  registerGoalLoop,
  buildLeadCompactionResult,
  resolveDialogBudgetBytes,
  resolveRereadBudgetBytes,
  type GoalLoopConfig,
} from "../src/goal-loop.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { staticConfigReader } from "../src/adapter-config.ts";
import { flushHeldPushes, leadIdleRef, clearWakeStart, leadCompactingRef, leadWakeStartPendingRef, heldPushQueue, registerPushFlush, buildPushWakeLine, pushToLead, sendBatchToLead, type RpcAgentRegistry, type TerminalDelivery } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { DEFAULT_DIALOG_BUDGET_BYTES, DEFAULT_REREAD_BUDGET_BYTES, extractLeadProse, NO_KEPT_ENTRY_ID, renderLeadProse } from "../src/lead-compaction.ts";
import { LABEL, labeled, prose, compactionEvent, tmpDir, writeConfig, fileReader } from "./fixtures/goal-loop.ts";

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

  /**
   * 260906 Phase 1 (settle-timer reminder race ticket): the ticket's own
   * Phase 1 test list — the settle timer's arm/fire/cancel mechanics, the
   * boundary guard's clear points, and the streak-advances-only-on-fired-
   * reminders invariant.
   */
  describe("settle timer mechanics (260906 Phase 1)", () => {
    test("261016: a running timed background task yields; a running monitor alone does not", { skip: process.platform === "win32" ? "POSIX shell" : false }, () => {
      const clock = fakeClock();
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), scheduleTimer: clock.scheduleTimer, clearTimer: clock.clearTimer, rpcRegistryRef: { current: new Map() as unknown as RpcAgentRegistry } });
      const { ctx } = fakeCtx();
      pi.commands.get("goal")!("ship the widget", ctx);
      const start = (timeoutS?: number) => startBackgroundTask({ shell: "/bin/sh", args: ["-c", "sleep 30"], cwd: tmpdir(), command: "sleep 30", why: "w", timeoutS, capBytes: 64, onComplete: () => {} });
      try {
        const timed = start(600);
        start();
        pi.handlers.get("agent_settled")!({}, ctx);
        clock.fire();
        assert.equal(pi.sentUserMessages.length, 1, "yields while a timed task runs, even beside a monitor");

        abortBackgroundTask(timed.id);
        pi.handlers.get("agent_settled")!({}, ctx);
        clock.fire();
        assert.equal(pi.sentUserMessages.length, 2, "a running monitor alone never blocks re-firing");
      } finally {
        killAllBackgroundTasks();
      }
    });

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

    test("settle alone fires exactly once at the delay, reading settle_delay_ms fresh through the config reader (ws-mcp config.get)", () => {
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
      assert.match(summary, /## Dialog\nAll 1 dialog items of this session\.\n.*`\/sessions\/lead\.jsonl`.*\n<dialog>\n<message role="user" timestamp=".+">\nthe human request\n<\/message>\n<\/dialog>/);
      assert.match(summary, /### Current work\nPROSE/);
      assert.match(summary, /### Immediate next step\n"do the thing"/);
      assert.doesNotMatch(summary, /PUSH BATCH BODY/, "push-batch traffic is not a human message");
      assert.doesNotMatch(summary, /\/tmp\/(read|edit)\.ts|read-files|modified-files/, "no file lists");
      assert.doesNotMatch(summary, /## Required re-reads|## On-demand references/, "no curated list was given");
    });

    test("the lever's file lists reach the summary, read relative to the session's working directory (261007)", async () => {
      const { pi, ctx } = leverRun();
      const cwd = mkdtempSync(join(tmpDir, "reread-cwd-"));
      writeFileSync(join(cwd, "next.ts"), "export const next = 1;\n");
      (ctx as unknown as { cwd: string }).cwd = cwd;
      await pi.tools.get("ws-compact")!.execute("c", {
        current_work: "PROSE",
        required_rereads: [{ path: "next.ts", why: "the step edits it" }, { path: "gone.ts", lines: "1-2", why: "was here" }],
        references: [{ path: "later.md", why: "maybe later" }],
      }, undefined, undefined, ctx);
      const result = pi.handlers.get("session_before_compact")!(compactionEvent("manual", branch), ctx) as unknown as { compaction: { summary: string } };
      const summary = result.compaction.summary;
      assert.ok(summary.includes("## Required re-reads (inlined)\n<file path=\"next.ts\" why=\"the step edits it\">\nexport const next = 1;\n</file>\n\n## Required re-reads (to read first)\n- `gone.ts` lines 1-2 - was here (not found)\n\n## On-demand references\n- `later.md` - maybe later\n\n## Dialog\n"));
      assert.equal(extractLeadProse(summary), renderLeadProse({ current_work: "PROSE" }));
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

  describe("pushes steered into the lever turn survive the lever compaction (261015)", () => {
    const leadDetails = { kind: "ws-pi-lead-compaction", version: 1, source: "lever" };

    function setup() {
      const pi = fakePi();
      const record = { agentId: "w-1", alias: "w1", spawnRole: "worker", running: true, streaming: false, reportLog: [] };
      const registry = new Map([["w-1", record]]) as unknown as RpcAgentRegistry;
      registerGoalLoop(pi.api, { readConfig: fileReader(configPath), rpcRegistryRef: { current: registry }, sessionKeyRef: { current: "lead-key" } });
      const idle = { current: false }; // the lever turn is streaming
      const { ctx } = fakeCtx(() => idle.current);
      let compactCall: Parameters<ExtensionContext["compact"]>[0] | undefined;
      ctx.compact = (opts) => { compactCall = opts; };
      return { pi, ctx, idle, registry, record: record as never, compact: () => compactCall! };
    }

    /** Pi records a drained custom steer as a `custom_message` entry after the lever's tool result (agent-session.js message_end). */
    function leverBranch(steers: Array<{ content: unknown }>): unknown[] {
      const recorded = steers.map((sent, index) => {
        const message = sent.content as { customType: string; content: unknown; display: boolean; details: unknown };
        return { type: "custom_message", id: `s${index}`, parentId: null, timestamp: "2026-10-15T00:00:01.000Z", customType: message.customType, content: message.content, display: message.display, details: message.details };
      });
      return [
        { type: "message", id: "u1", parentId: null, timestamp: "2026-10-15T00:00:00.000Z", message: { role: "user", content: "go", timestamp: 0 } },
        { type: "message", id: "a1", parentId: "u1", timestamp: "2026-10-15T00:00:00.000Z", message: { role: "assistant", content: [{ type: "toolCall", id: "t1", name: "ws-compact", arguments: {} }] } },
        { type: "message", id: "r1", parentId: "a1", timestamp: "2026-10-15T00:00:00.000Z", message: { role: "toolResult", toolCallId: "t1", toolName: "ws-compact", content: [{ type: "text", text: "ok" }], isError: false } },
        ...recorded,
        { type: "message", id: "a2", parentId: null, timestamp: "2026-10-15T00:00:02.000Z", message: { role: "assistant", content: [], stopReason: "aborted" } },
      ];
    }

    function lastBatch(pi: ReturnType<typeof fakePi>): { content: string; details: { items: Array<{ customType: string; content: unknown; state: string }> } } {
      const sent = pi.sentMessages.at(-1)!.content as { customType: string; content: string; details: { items: Array<{ customType: string; content: unknown; state: string }> } };
      assert.equal(sent.customType, PUSH_BATCH_CUSTOM_TYPE);
      return sent;
    }

    test("a direct approval steer is re-delivered first, actionable and labeled once, ahead of pushes held during compaction; its terminal delivery is untouched", async () => {
      const { pi, ctx, idle, registry, record, compact } = setup();
      let afterEnqueueCalls = 0;
      const terminal = { afterEnqueue: () => { afterEnqueueCalls += 1; } } as TerminalDelivery;
      pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "cmd-1", command: "rm -rf build" }, "steer", terminal);
      assert.equal(pi.sentMessages.length, 1, "the busy lead takes the approval as a direct steer");
      assert.equal(terminal.state, "enqueued");
      const steered = pi.sentMessages[0]!;

      await pi.tools.get("ws-compact")!.execute("t1", { current_work: "PROSE" }, undefined, undefined, ctx);
      pushToLead(pi.api, registry, record, "ws-agent-report", { report: "held during compaction" }, "steer");
      assert.equal(heldPushQueue.length, 1, "a push during compaction is held");

      assert.ok(pi.handlers.get("session_before_compact")!(compactionEvent("manual", leverBranch([steered])), ctx));
      pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: true, compactionEntry: { details: leadDetails } }, ctx);
      assert.equal(heldPushQueue.length, 2, "the lost steer is back in the held queue");
      idle.current = true;
      compact().onComplete!({} as never);
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(leadCompactingRef.current, false);
      assert.equal(flushHeldPushes(pi.api, true), 2, "the post-compaction start releases both");

      const batch = lastBatch(pi);
      assert.deepEqual(batch.details.items.map((item) => [item.customType, item.state]), [["ws-agent-approval", "actionable"], ["ws-agent-report", "informational"]]);
      assert.equal(batch.content.split(LABEL).length - 1, 1, "the batch carries the label once; the recovered item is not labeled again");
      assert.ok(!String(batch.details.items[0]!.content).includes(LABEL));
      assert.match(batch.content, /<action type="approval" agent-id="w-1" command-id="cmd-1">/);
      assert.equal(terminal.state, "enqueued", "not reset to held");
      assert.equal(afterEnqueueCalls, 0, "afterEnqueue does not run for the re-delivery");
    });

    test("a mailbox batch steer is unpacked into its items, in order, with no nested batch, and the release wakes the lead for it", async () => {
      const wakePi = { on: (event: string, fn: () => void) => { if (event === "session_shutdown") shutdown = fn; } } as unknown as ExtensionAPI;
      let shutdown: (() => void) | undefined;
      registerPushFlush(wakePi, { delayMs: () => 10, scheduleTimer: () => 0 as unknown as NodeJS.Timeout, clearTimer: () => {} });
      try {
        const { pi, ctx, idle, compact } = setup();
        sendBatchToLead(pi.api, [
          { customType: "ws-mailbox", content: "mail one", display: true, details: { n: 1 } },
          { customType: "ws-mailbox", content: "mail two", display: true, details: { n: 2 } },
        ], "steer");
        assert.equal(pi.sentMessages.length, 1);
        const steered = pi.sentMessages[0]!;

        await pi.tools.get("ws-compact")!.execute("t1", { current_work: "PROSE" }, undefined, undefined, ctx);
        pi.handlers.get("session_before_compact")!(compactionEvent("manual", leverBranch([steered])), ctx);
        pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: true, compactionEntry: { details: leadDetails } }, ctx);
        idle.current = true;
        compact().onComplete!({} as never);
        await new Promise((resolve) => setImmediate(resolve));
        assert.deepEqual(pi.sentUserMessages.map((sent) => sent.content)[0], labeled(buildPushWakeLine(2)), "the release wakes the lead for a queue holding only recovered items");
        clearWakeStart();
        assert.equal(flushHeldPushes(pi.api, true), 2, "both mailbox items are back in the held queue");
        assert.equal(pi.sentMessages.length, 2, "re-delivered as a new batch");

        const batch = lastBatch(pi);
        assert.deepEqual(batch.details.items.map((item) => [item.customType, item.content, item.state]), [["ws-mailbox", "mail one", "informational"], ["ws-mailbox", "mail two", "informational"]]);
        assert.doesNotMatch(batch.content, /type="ws-push-batch"/, "no nested batch");
      } finally {
        shutdown?.();
      }
    });

    for (const outcome of ["failed", "native summary"] as const) {
      test(`a ${outcome} lever compaction recovers nothing`, async () => {
        const { pi, ctx, idle, registry, record, compact } = setup();
        pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "cmd-1" }, "steer");
        const steered = pi.sentMessages[0]!;
        await pi.tools.get("ws-compact")!.execute("t1", { current_work: "PROSE" }, undefined, undefined, ctx);
        pi.handlers.get("session_before_compact")!(compactionEvent("manual", leverBranch([steered])), ctx);
        idle.current = true;
        if (outcome === "failed") {
          pi.handlers.get("session_compact_failed")!({ reason: "manual", errorMessage: "Compaction failed: boom" }, ctx);
          compact().onError!(new Error("boom"));
        } else {
          pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: false, compactionEntry: { details: undefined } }, ctx);
          compact().onComplete!({} as never);
        }
        await new Promise((resolve) => setImmediate(resolve));
        assert.equal(heldPushQueue.length, 0, "the steer is still in context, so nothing is re-delivered");
      });
    }

    test("a non-lever compaction recovers nothing, even with a lever result in the branch", async () => {
      const { pi, ctx, idle, registry, record } = setup();
      pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "cmd-1" }, "steer");
      const steered = pi.sentMessages[0]!;
      pi.handlers.get("session_before_compact")!(compactionEvent("threshold", leverBranch([steered])), ctx);
      idle.current = true;
      pi.handlers.get("session_compact")!({ reason: "threshold", fromExtension: true, compactionEntry: { details: { ...leadDetails, source: "fallback" } } }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(heldPushQueue.length, 0);
    });
  });

  describe("no raw kept tail (261003)", () => {
    test("the dialog budget resolves from its byte knob and falls back to 40 KiB", () => {
      assert.equal(resolveDialogBudgetBytes(undefined), DEFAULT_DIALOG_BUDGET_BYTES);
      assert.equal(DEFAULT_DIALOG_BUDGET_BYTES, 40960);
      assert.equal(resolveDialogBudgetBytes({ compaction_dialog_budget_bytes: 512 }), 512);
      for (const bad of [0, -1, Number.NaN, "512"]) assert.equal(resolveDialogBudgetBytes({ compaction_dialog_budget_bytes: bad as never }), DEFAULT_DIALOG_BUDGET_BYTES, String(bad));
    });

    test("the required re-read budget resolves from its byte knob and falls back to 40 KiB (261007)", () => {
      assert.equal(resolveRereadBudgetBytes(undefined), DEFAULT_REREAD_BUDGET_BYTES);
      assert.equal(DEFAULT_REREAD_BUDGET_BYTES, 40960);
      assert.equal(resolveRereadBudgetBytes({ compaction_reread_budget_bytes: 512 }), 512);
      for (const bad of [0, -1, Number.NaN, "512"]) assert.equal(resolveRereadBudgetBytes({ compaction_reread_budget_bytes: bad as never }), DEFAULT_REREAD_BUDGET_BYTES, String(bad));
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
      assert.match(result.summary, /<dialog>\n<message role="user" timestamp=".+">\nHUMAN ASK\n<\/message>\n<message role="assistant" timestamp=".+">\nLEAD REPLY\n<\/message>\n<tool-calls>\n\u2192 Bash \{"command":"ls"\}\n<\/tool-calls>\n<message role="assistant" timestamp=".+">\nLATEST REPLY\n<\/message>\n<\/dialog>/);
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

    /** A trigger run whose usage is set in tokens against a mutable model window. */
    function tokenRun(config: GoalLoopConfig) {
      const run = triggerRun(config);
      const usage = { tokens: 0, contextWindow: 1_000_000 };
      (run.ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ ...usage, percent: (usage.tokens / usage.contextWindow) * 100 });
      const text = (i: number) => (run.preparations()[i]!.content as { content: string }).content;
      const milestones = () => run.pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-context-milestone");
      return { ...run, usage, text, milestones, runEnd: () => run.pi.handlers.get("agent_end")!({}, run.ctx), toolTurn: () => run.pi.handlers.get("turn_end")!({ toolResults: [{}] }, run.ctx) };
    }

    test("token thresholds resolve against the window; milestones sit between the resolved token counts", () => {
      const { usage, preparations, text, milestones, runEnd, toolTurn } = tokenRun({ compaction_advisory_percent: "300k", compaction_hard_percent: "50%|600k" });
      usage.tokens = 299_000;
      runEnd();
      assert.equal(preparations().length, 0, "below the 300k advisory on a 1M window");
      usage.tokens = 310_000;
      runEnd();
      assert.equal(preparations().length, 1);
      assert.match(text(0), /Context usage is 31% of the window \(advisory point: 30% \/ 300k tokens\)\. Compaction becomes forced at 50% \/ 500k tokens\./, "the combined hard threshold takes its smaller, percent side");
      assert.match(text(0), / Reminders follow at 37% \/ 366.7k tokens and 43% \/ 433.3k tokens\.\n/);
      runEnd();
      usage.tokens = 370_000;
      runEnd();
      assert.equal(milestones().length, 1, "the first milestone sits a third of the way from 300k to 500k");
      assert.match((milestones()[0]!.content as { content: string }).content, /Context window: 37% \/ 50% \(forced compaction point, 500k tokens\)\./);
      runEnd();
      usage.tokens = 500_000;
      toolTurn();
      assert.equal(preparations().length, 2);
      assert.match(text(1), /past the hard compaction point \(50% \/ 500k tokens\)/);
    });

    test("the combined form takes whichever side the window reaches first", () => {
      const small = tokenRun({ compaction_advisory_percent: "50%|150k" });
      small.usage.contextWindow = 200_000;
      small.usage.tokens = 99_000;
      small.runEnd();
      assert.equal(small.preparations().length, 0, "on a 200k window the 50% side (100k) wins");
      small.usage.tokens = 101_000;
      small.runEnd();
      assert.match(small.text(0), /advisory point: 50% \/ 100k tokens/);

      const large = tokenRun({ compaction_advisory_percent: "50%|150k" });
      large.usage.tokens = 151_000;
      large.runEnd();
      assert.match(large.text(0), /advisory point: 15% \/ 150k tokens/, "on a 1M window the 150k side wins");
    });

    test("thresholds resolve against the context-window override when set", () => {
      const { usage, preparations, text, runEnd } = tokenRun({ compaction_advisory_percent: "50%|300k", context_window_override: 400_000 });
      usage.tokens = 199_000;
      runEnd();
      assert.equal(preparations().length, 0, "50% of the 400k override is 200k, below the 300k token side");
      usage.tokens = 201_000;
      runEnd();
      assert.match(text(0), /Context usage is 50% of the window \(advisory point: 50% \/ 200k tokens\)/);
    });

    test("a token threshold beyond the window never fires", () => {
      const { usage, preparations, milestones, runEnd, toolTurn } = tokenRun({ compaction_advisory_percent: "300k", compaction_hard_percent: "400k" });
      usage.contextWindow = 200_000;
      usage.tokens = 199_000;
      toolTurn();
      runEnd();
      assert.equal(preparations().length + milestones().length, 0, "no clamp: Pi's own compaction takes over");
    });

    test("an advisory at or above the resolved hard point fires neither the advisory nor the milestones", () => {
      const { usage, preparations, milestones, text, runEnd } = tokenRun({ compaction_advisory_percent: "60%", compaction_hard_percent: "100k" });
      usage.contextWindow = 200_000;
      usage.tokens = 110_000;
      runEnd();
      assert.equal(preparations().length, 1);
      assert.equal(milestones().length, 0);
      assert.match(text(0), /past the hard compaction point \(50% \/ 100k tokens\)/);
    });

    test("a resume past a token advisory point is baselined against the window, and a rewind below re-arms it", () => {
      const { pi, ctx, usage, preparations, runEnd } = tokenRun({ compaction_advisory_percent: "50%|150k" });
      usage.tokens = 160_000;
      pi.handlers.get("session_start")!({}, ctx);
      runEnd();
      assert.equal(preparations().length, 0, "the 150k side is already past at resume: no duplicate advisory");
      usage.tokens = 100_000;
      pi.handlers.get("session_tree")!({}, ctx);
      usage.tokens = 160_000;
      runEnd();
      assert.equal(preparations().length, 1, "a rewind below the token threshold re-arms it");
    });

    test("a token threshold fires once per crossing and re-arms below it", () => {
      const { usage, preparations, runEnd } = tokenRun({ compaction_advisory_percent: "150k" });
      usage.tokens = 160_000;
      runEnd();
      runEnd();
      usage.tokens = 170_000;
      runEnd();
      assert.equal(preparations().length, 1, "staying above the threshold does not repeat the advisory");
      usage.tokens = 100_000;
      runEnd();
      usage.tokens = 155_000;
      runEnd();
      assert.equal(preparations().length, 2, "dropping below and crossing again is a new crossing");
    });

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
      assert.match((nudge.content as { content: string }).content, /^\[system message from ws-pi-plugin\]\nContext usage is 55% of the window \(advisory point: 50% \/ 500 tokens\)\. Compaction becomes forced at 80% \/ 800 tokens\.\n[\s\S]*keep looking for one as you work\.\n\nGUIDE BODY 261002$/);

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
      assert.match((steer.content as { content: string }).content, /^\[system message from ws-pi-plugin\]\nContext usage is 71% .*hard compaction point \(70% \/ 700 tokens\)\. Stop the current work now[\s\S]*GUIDE BODY 261002$/);
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
        pi.handlers.get("session_start")!({ reason: "startup" }, ctx);
        usage.percent = 62;
        pi.handlers.get("turn_end")!({ toolResults: [{}] }, ctx);
        usage.percent = 99;
        pi.handlers.get("turn_end")!({}, ctx);
        pi.handlers.get("turn_end")!({ toolResults: [{}] }, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        assert.equal(preparations().length, 0);
        assert.equal(pi.sentMessages.length, 0, "no milestone either");
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
      assert.match((reroute.content as { content: string }).content, /^\[system message from ws-pi-plugin\]\nThe user ran \/compact[\s\S]*focus text, to honor in your prose:\nkeep the API notes\n\nGUIDE BODY 261002$/);
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
        const branch = [
          { type: "message", id: "u1", parentId: null, timestamp: "2026-10-02T10:00:00.000Z", message: { role: "user", content: "HUMAN ASK", timestamp: 0 } },
          { type: "message", id: "kept-entry", parentId: "u1", timestamp: "2026-10-02T10:01:00.000Z", message: { role: "assistant", content: [{ type: "text", text: "TAIL WORK" }], timestamp: 0 } },
          { type: "message", id: "r1", parentId: "kept-entry", timestamp: "2026-10-02T10:02:00.000Z", message: { role: "toolResult", toolCallId: "c", toolName: "Bash", content: [{ type: "text", text: "TAIL TOOL OUTPUT" }], isError: false, timestamp: 0 } },
        ];
        const event = compactionEvent(reason, branch);
        (event as { preparation: { messagesToSummarize: unknown[]; previousSummary?: string } }).preparation.messagesToSummarize = [{ role: "user", content: "summarize me", timestamp: 0 }];
        const result = await (pi.handlers.get("session_before_compact")!(event, ctx) as unknown as Promise<{ compaction: { summary: string; firstKeptEntryId: string; details: unknown; usage: unknown } }>);
        assert.equal(calls.length, 1, reason);
        assert.equal(calls[0]!.model, model, "the session model");
        assert.match(calls[0]!.context.systemPrompt!, /context summarization assistant/);
        const conversation = calls[0]!.context.messages[0]!.content[0]!.text;
        if (reason === "overflow") {
          // The context already exceeds the window; adding the kept tail to the summary input would overflow the summarizer too.
          assert.match(conversation, /<conversation>\n\[User\]: summarize me\n[\s\S]*<\/conversation>/);
          assert.doesNotMatch(conversation, /TAIL WORK|TAIL TOOL OUTPUT/, "an overflow compaction leaves Pi's would-be kept tail out of the summary input");
        } else {
          assert.match(conversation, /<conversation>\n\[User\]: summarize me\n[\s\S]*TAIL WORK[\s\S]*TAIL TOOL OUTPUT[\s\S]*<\/conversation>/, "Pi's would-be kept tail is summarized, since no raw tail is kept");
        }
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
        assert.doesNotMatch(summary, /## Required re-reads|## On-demand references/, "the fallback summary carries no file lists");
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
});
