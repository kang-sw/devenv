import { test, describe, after, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { stripSummaryId } from "./fixtures/summary-id.ts";
import { writeFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { type ExtensionAPI, type ExtensionContext } from "@earendil-works/pi-coding-agent";
import {
  buildCompactionResumeMessage,
  leadCompactParameterSchema,
  resumesAfterCompaction,
  registerGoalLoop,
  compactionThresholds,
  highestThresholdAtOrBelow,
  NO_TRIGGER_LATCH,
  DEFAULT_SETTLE_DELAY_MS,
  type GoalLoopConfig,
} from "../src/goal-loop.ts";
import { createWsConfigReader, staticConfigReader, type GoalLoopConfigReader } from "../src/adapter-config.ts";
import { leadIdleRef, clearWakeStart, leadCompactingRef, leadWakeStartPendingRef, heldPushQueue, registerPushFlush, buildPushWakeLine } from "../src/spawner.ts";
import { LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE, LEAD_PROSE_SECTIONS, leadProseParameterSchema } from "../src/lead-compaction.ts";
import { labeled, prose, compactionEvent, tmpDir } from "./fixtures/goal-loop.ts";

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

  describe("standing-intent advisory, interim milestones, and latch baselines (261006)", () => {
    const guidePath = join(tmpDir, "lead-compact-guide-261006.md");
    writeFileSync(guidePath, "GUIDE BODY 261006");

    function milestoneRun(config: GoalLoopConfig = { compaction_advisory_percent: 50, compaction_hard_percent: 80 }) {
      const pi = fakePi();
      registerGoalLoop(pi.api, { readConfig: staticConfigReader(config), ...fakeClock(), leadCompactGuidePath: guidePath, sessionKeyRef: { current: "lead-key" } });
      const { ctx } = fakeCtx();
      const usage = { percent: 10 as number | null };
      (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: usage.percent });
      const byType = (type: string) => pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === type);
      const preparations = () => byType("ws-lead-compact");
      const milestones = () => byType(LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE);
      const at = (percent: number | null) => { usage.percent = percent; };
      const toolTurn = () => pi.handlers.get("turn_end")!({ toolResults: [{ role: "toolResult" }] }, ctx);
      const finalTurn = () => pi.handlers.get("turn_end")!({ toolResults: [] }, ctx);
      const runEnd = () => pi.handlers.get("agent_end")!({}, ctx);
      const kindOf = (m: { content: unknown }) => (m.content as { details?: { trigger?: string } }).details?.trigger;
      return { pi, ctx, usage, preparations, milestones, at, toolTurn, finalTurn, runEnd, kindOf };
    }
    const ourDetails = { kind: "ws-pi-lead-compaction", version: 1, source: "lever" };

    test("thresholds sit one and two thirds of the way from the advisory to the hard point, exact and unrounded", () => {
      assert.deepEqual(compactionThresholds(50, 80).map((t) => [t.kind, t.percent]), [["advisory", 50], ["milestone", 60], ["milestone", 70], ["hard", 80]]);
      const tuned = compactionThresholds(50, 85).map((t) => t.percent);
      assert.equal(tuned[1], 50 + 35 / 3);
      assert.equal(tuned[2], 50 + 70 / 3);
      assert.deepEqual(compactionThresholds(90, 80), [{ kind: "hard", percent: 80 }], "an advisory at or above the hard point never fires, so it carries no milestones");
      assert.equal(highestThresholdAtOrBelow(compactionThresholds(50, 80), 49.9), NO_TRIGGER_LATCH);
      assert.equal(highestThresholdAtOrBelow(compactionThresholds(50, 80), 69.9), 60);
    });

    test("a milestone is a guide-less wake turn at agent_end only, using the tuned knobs and fractional thresholds", () => {
      const { at, toolTurn, finalTurn, runEnd, preparations, milestones } = milestoneRun({ compaction_advisory_percent: 50, compaction_hard_percent: 85 });
      at(55);
      runEnd();
      assert.equal(preparations().length, 1, "advisory sent");
      assert.match((preparations()[0]!.content as { content: string }).content, / Reminders follow at 62% \/ 617 tokens and 73% \/ 733 tokens\.\n/, "the advisory names the rounded milestone points");
      runEnd();
      at(61.6);
      runEnd();
      assert.equal(milestones().length, 0, "61.6 is below the exact 61.67 milestone");
      at(61.7);
      toolTurn();
      finalTurn();
      assert.equal(milestones().length, 0, "no turn_end delivers a milestone");
      runEnd();
      assert.equal(milestones().length, 1);
      const milestone = milestones()[0]!;
      assert.deepEqual(milestone.options, { deliverAs: "followUp", triggerTurn: true });
      assert.equal((milestone.content as { display: boolean }).display, true);
      assert.equal((milestone.content as { content: string }).content, "[system message from ws-pi-plugin]\nContext window: 62% / 85% (forced compaction point, 850 tokens). Past 85%, compaction is forced, mid-work if need be. From here on, a good boundary is worth taking; if this is one, run the preparation from the advisory. Otherwise end this turn without replying.");
      assert.deepEqual(stripSummaryId((milestone.content as { details: unknown }).details), { milestone: 50 + 35 / 3 });
      assert.equal(preparations().length, 1, "a milestone carries no guide and is not a preparation message");

      at(74);
      toolTurn();
      finalTurn();
      assert.equal(milestones().length, 1, "turn_ends inside the milestone turn deliver nothing");
      runEnd();
      assert.equal(milestones().length, 2, "the second milestone at the exact 73.33, at the milestone turn's own agent_end");
      // 261007: each summarized send carries its own display-summary row id.
      const rowIds = [preparations()[0]!, ...milestones()].map((sent) => stripSummaryId((sent.content as { details: unknown }).details) && (sent.content as { details: { ws_summary_id: string } }).details.ws_summary_id);
      assert.equal(new Set(rowIds).size, 3, "the advisory and both milestones carry distinct row ids");
      assert.equal((milestones()[1]!.content as { content: string }).content, "[system message from ws-pi-plugin]\nContext window: 74% / 85% (forced compaction point, 850 tokens). This is the last reading before compaction is forced. Compacting at a boundary you choose keeps the summary in your hands; take this one if it fits, and run the preparation from the advisory. Otherwise end this turn without replying, and take the next good boundary.");
      runEnd();
      at(80);
      runEnd();
      assert.equal(milestones().length, 2, "nothing new is crossed");
    });

    test("two milestones crossed at once send only the second milestone's text and latch past the first", () => {
      const { at, toolTurn, finalTurn, runEnd, milestones } = milestoneRun();
      at(55);
      runEnd();
      runEnd();
      at(72);
      toolTurn();
      finalTurn();
      assert.equal(milestones().length, 0, "crossed mid-run, pending until agent_end");
      runEnd();
      assert.equal(milestones().length, 1);
      assert.equal((milestones()[0]!.content as { content: string }).content, "[system message from ws-pi-plugin]\nContext window: 72% / 80% (forced compaction point, 800 tokens). This is the last reading before compaction is forced. Compacting at a boundary you choose keeps the summary in your hands; take this one if it fits, and run the preparation from the advisory. Otherwise end this turn without replying, and take the next good boundary.");
      assert.deepEqual(stripSummaryId((milestones()[0]!.content as { details: unknown }).details), { milestone: 70 });
      runEnd();
      at(75);
      runEnd();
      assert.equal(milestones().length, 1, "the first milestone is covered");
    });

    test("the milestone wake turn holds the preparation flag until its agent_end", () => {
      const { at, toolTurn, finalTurn, runEnd, preparations, milestones, kindOf } = milestoneRun();
      at(55);
      runEnd();
      runEnd();
      at(62);
      runEnd();
      assert.equal(milestones().length, 1);
      at(85);
      toolTurn();
      finalTurn();
      assert.equal(preparations().length, 1, "no hard steer inside the milestone turn");
      runEnd();
      assert.equal(preparations().length, 2, "the milestone turn's agent_end clears the flag and checks again");
      assert.equal(kindOf(preparations()[1]!), "hard");
      assert.deepEqual(preparations()[1]!.options, { deliverAs: "followUp", triggerTurn: true });
    });

    test("a milestone waits while the advisory is undelivered within one run; the advisory then latches past it", () => {
      const { at, toolTurn, runEnd, preparations, milestones, kindOf } = milestoneRun();
      at(45);
      toolTurn();
      at(52);
      toolTurn();
      at(62);
      toolTurn();
      assert.equal(milestones().length, 0);
      assert.equal(preparations().length, 0);
      runEnd();
      assert.equal(preparations().length, 1);
      assert.equal(kindOf(preparations()[0]!), "advisory");
      assert.equal(milestones().length, 0, "the advisory goes first");
      runEnd();
      assert.equal(milestones().length, 0, "the advisory latched to 60, covering the skipped milestone");
      at(70);
      runEnd();
      assert.equal(milestones().length, 1, "the next milestone still fires");
    });

    test("one message per observation by priority hard > advisory > milestone", () => {
      const { at, runEnd, preparations, milestones, kindOf } = milestoneRun();
      at(49);
      runEnd();
      at(63);
      runEnd();
      assert.equal(preparations().length, 1);
      assert.equal(kindOf(preparations()[0]!), "advisory");
      assert.match((preparations()[0]!.content as { content: string }).content, /^\[system message from ws-pi-plugin\]\nContext usage is 63% of the window/);
      assert.equal(milestones().length, 0);
      runEnd();

      const other = milestoneRun();
      other.at(49);
      other.runEnd();
      other.at(85);
      other.toolTurn();
      assert.equal(other.preparations().length, 1);
      assert.equal(other.kindOf(other.preparations()[0]!), "hard");
      assert.deepEqual(other.preparations()[0]!.options, { deliverAs: "steer", triggerTurn: true });
      assert.equal(other.milestones().length, 0);
      other.runEnd();
      other.runEnd();
      assert.equal(other.preparations().length + other.milestones().length, 1, "the hard delivery covers the advisory and both milestones");
    });

    test("an ignored or aborted hard steer is not re-sent while usage keeps climbing", () => {
      const { at, toolTurn, finalTurn, runEnd, preparations, milestones } = milestoneRun();
      at(82);
      finalTurn();
      assert.equal(preparations().length, 1, "the hard steer goes out at any turn_end");
      assert.deepEqual(preparations()[0]!.options, { deliverAs: "steer", triggerTurn: true }, "a steer even at a run's final, tool-less turn_end");
      runEnd();
      at(88);
      toolTurn();
      runEnd();
      at(95);
      toolTurn();
      runEnd();
      assert.equal(preparations().length, 1);
      assert.equal(milestones().length, 0);
    });

    test("a resume or rewind past the hard point gets the hard steer once more, and only once", () => {
      for (const event of ["session_start", "session_tree"] as const) {
        const { pi, ctx, at, toolTurn, runEnd, preparations, milestones, kindOf } = milestoneRun();
        at(82);
        toolTurn();
        runEnd();
        assert.equal(preparations().length, 1, event);
        at(90);
        pi.handlers.get(event)!({}, ctx);
        toolTurn();
        assert.equal(preparations().length, 2, `${event}: the baseline stops below the hard point`);
        assert.equal(kindOf(preparations()[1]!), "hard");
        assert.equal(milestones().length, 0, `${event}: the baseline still covers the milestones`);
        runEnd();
        at(93);
        toolTurn();
        runEnd();
        assert.equal(preparations().length, 2, `${event}: delivery latches the hard point again`);
      }
    });

    test("a hard crossing first seen at agent_end goes out as a follow-up that triggers a turn", () => {
      const { at, runEnd, preparations, kindOf } = milestoneRun();
      at(49);
      runEnd();
      at(85);
      runEnd();
      assert.equal(preparations().length, 1);
      assert.equal(kindOf(preparations()[0]!), "hard");
      assert.deepEqual(preparations()[0]!.options, { deliverAs: "followUp", triggerTurn: true });
    });

    describe("with an asynchronous config read (the production bridge reader)", () => {
      const config: GoalLoopConfig = { compaction_advisory_percent: 50, compaction_hard_percent: 80 };
      function deferredRun() {
        const reads: Array<{ resolve: () => void; reject: () => void }> = [];
        const pi = fakePi();
        registerGoalLoop(pi.api, {
          readConfig: () => new Promise<GoalLoopConfig>((resolve, reject) => { reads.push({ resolve: () => resolve(config), reject: () => reject(new Error("bridge gone")) }); }),
          ...fakeClock(),
          leadCompactGuidePath: guidePath,
          sessionKeyRef: { current: "lead-key" },
        });
        const { ctx } = fakeCtx();
        const usage = { percent: 10 };
        (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: usage.percent });
        const preparations = () => pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact");
        const flush = () => new Promise((resolve) => setImmediate(resolve));
        return { pi, ctx, reads, preparations, flush, at: (percent: number) => { usage.percent = percent; } };
      }

      test("the baseline is not awaited, and a rejected read is swallowed", async () => {
        const { pi, ctx, reads, flush } = deferredRun();
        assert.equal(pi.handlers.get("session_start")!({ reason: "resume" }, ctx), undefined, "session_start hands Pi nothing to await");
        assert.equal(pi.handlers.get("session_tree")!({ newLeafId: "a", oldLeafId: "b" }, ctx), undefined);
        for (const read of reads) read.reject();
        await flush();
      });

      test("a late baseline uses the usage at its event, so it cannot latch a crossing still pending delivery", async () => {
        const { pi, ctx, reads, preparations, flush, at } = deferredRun();
        at(49);
        pi.handlers.get("session_start")!({ reason: "resume" }, ctx);
        at(52);
        void pi.handlers.get("turn_end")!({ toolResults: [{ role: "toolResult" }] }, ctx);
        for (const read of reads.splice(0)) read.resolve();
        await flush();
        void pi.handlers.get("agent_end")!({}, ctx);
        for (const read of reads.splice(0)) read.resolve();
        await flush();
        assert.equal(preparations().length, 1, "the advisory crossed after the resume still goes out");
      });

      test("a late baseline yields to a delivery made after its event", async () => {
        const { pi, ctx, reads, preparations, flush, at } = deferredRun();
        at(81);
        pi.handlers.get("session_start")!({ reason: "resume" }, ctx);
        const baselineRead = reads.shift()!;
        void pi.handlers.get("turn_end")!({ toolResults: [{ role: "toolResult" }] }, ctx);
        for (const read of reads.splice(0)) read.resolve();
        await flush();
        assert.equal(preparations().length, 1, "the hard steer went out before the baseline resolved");
        baselineRead.resolve();
        await flush();
        void pi.handlers.get("agent_end")!({}, ctx);
        for (const read of reads.splice(0)) read.resolve();
        await flush();
        assert.equal(preparations().length, 1, "the baseline did not lower the latch below the delivered hard point");
      });
    });

    test("session_tree re-baselines the latch from the rewound branch's usage", () => {
      const { pi, ctx, at, toolTurn, runEnd, preparations, milestones } = milestoneRun();
      at(65);
      runEnd();
      assert.equal(preparations().length, 1, "advisory sent, latched to 60");
      runEnd();
      at(30);
      pi.handlers.get("session_tree")!({ newLeafId: "a", oldLeafId: "b" }, ctx);
      at(55);
      runEnd();
      assert.equal(preparations().length, 2, "a rewind below the advisory point re-arms the advisory");
      runEnd();

      at(72);
      pi.handlers.get("session_tree")!({ newLeafId: "c", oldLeafId: "a" }, ctx);
      toolTurn();
      runEnd();
      assert.equal(milestones().length, 0, "a rewind to 72% latches to the second milestone");
      assert.equal(preparations().length, 2);
    });

    test("session_start baselines the latch from the current branch, unlatched when usage is unknown", () => {
      const resumed = milestoneRun();
      resumed.at(65);
      resumed.pi.handlers.get("session_start")!({ reason: "resume" }, resumed.ctx);
      resumed.runEnd();
      assert.equal(resumed.preparations().length, 0, "no duplicate advisory for a resumed session past the advisory point");
      resumed.runEnd();
      assert.equal(resumed.milestones().length, 0);
      resumed.at(71);
      resumed.runEnd();
      assert.equal(resumed.milestones().length, 1, "the next milestone still fires");

      const fresh = milestoneRun();
      fresh.at(null);
      fresh.pi.handlers.get("session_start")!({ reason: "resume" }, fresh.ctx);
      fresh.at(55);
      fresh.runEnd();
      assert.equal(fresh.preparations().length, 1, "unknown usage (only a compaction since the last response) starts unlatched");
    });

    test("a compaction drops a pending milestone", async () => {
      const { pi, ctx, at, toolTurn, finalTurn, runEnd, preparations, milestones } = milestoneRun();
      at(55);
      runEnd();
      runEnd();
      at(62);
      toolTurn();
      finalTurn();
      assert.equal(milestones().length, 0, "crossed mid-run, pending until agent_end");
      pi.handlers.get("session_compact")!({ reason: "manual", fromExtension: true, compactionEntry: { details: ourDetails } }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      runEnd();
      assert.equal(milestones().length, 0, "the latch was reset; the re-armed advisory goes first and latches past the milestone");
      assert.equal(preparations().length, 2, "the advisory is re-armed instead");
    });

    test("no milestone is sent while a preparation turn or a compaction is in progress", () => {
      const { pi, ctx, at, toolTurn, finalTurn, runEnd, preparations, milestones } = milestoneRun();
      at(55);
      runEnd();
      assert.equal(preparations().length, 1, "advisory queued; its run has not ended");
      at(62);
      toolTurn();
      finalTurn();
      assert.equal(milestones().length, 0, "the preparation turn is running");
      pi.handlers.get("session_before_compact")!(compactionEvent("threshold"), ctx);
      assert.equal(leadCompactingRef.current, true);
      runEnd();
      assert.equal(milestones().length, 0, "a compaction in progress blocks the milestone");
    });
  });

  describe("goal-less resume after a lever compaction (261003)", () => {
    const guidePath = join(tmpDir, "lead-compact-guide-261003.md");
    writeFileSync(guidePath, "GUIDE BODY 261003");
    const resumeText = buildCompactionResumeMessage("lead-key");
    /** The resume exactly as sent. */
    const sentResume = labeled(resumeText);

    function resumeRun() {
      const clock = fakeClock();
      const pi = fakePi();
      const handle = registerGoalLoop(pi.api, { readConfig: staticConfigReader({ compaction_advisory_percent: 50, compaction_hard_percent: 80 }), ...clock, leadCompactGuidePath: guidePath, sessionKeyRef: { current: "lead-key" } });
      const idle = { current: true };
      const { ctx } = fakeCtx(() => idle.current);
      const usage = { percent: 10 as number | null };
      (ctx as unknown as { getContextUsage: () => unknown }).getContextUsage = () => ({ tokens: null, contextWindow: 1000, percent: usage.percent });
      let compactCall: Parameters<ExtensionContext["compact"]>[0] | undefined;
      ctx.compact = (opts) => { compactCall = opts; };
      const preparations = () => pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === "ws-lead-compact");
      const resumes = () => pi.sentUserMessages.filter((m) => m.content === sentResume);
      /** The lever call, then the abort's own agent_end, as Pi's compact() produces them. */
      const lever = async (args: Record<string, unknown> = {}) => {
        await pi.tools.get("ws-compact")!.execute("c", { current_work: "mid-task", ...args }, undefined, undefined, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx);
      };
      const ours = { reason: "manual", fromExtension: true, compactionEntry: { details: { kind: "ws-pi-lead-compaction", version: 1, source: "lever" } } };
      /** Success through both completion paths for one operation. */
      const complete = async () => {
        pi.handlers.get("session_compact")!(ours, ctx);
        compactCall!.onComplete!({} as never);
        await new Promise((resolve) => setImmediate(resolve));
      };
      return { clock, pi, handle, ctx, idle, usage, preparations, resumes, lever, complete, compactCall: () => compactCall!, ours };
    }

    test("buildCompactionResumeMessage names lead-revive with the key and bounds the continuation", () => {
      assert.equal(
        resumeText,
        "Compaction complete. Invoke `lead-revive` (`ws-skill lead-revive`) with session key `lead-key`, then continue the immediate next step; if it awaits the user, end your turn.",
      );
      assert.match(buildCompactionResumeMessage(undefined), /with session key \(recover it first\),/);
      assert.deepEqual(["hard", "autonomous", "advisory", "milestone", "reroute", undefined].map((route) => resumesAfterCompaction(route as never)), [true, true, false, false, false, false]);
    });

    test("a lever compaction after a hard preparation sends exactly one resume followUp after release", async () => {
      const { pi, ctx, usage, preparations, resumes, lever, complete, clock } = resumeRun();
      usage.percent = 85;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 1, "hard steer sent");
      await lever();
      assert.equal(resumes().length, 0, "nothing is sent before release");
      await complete();
      assert.equal(resumes().length, 1);
      assert.deepEqual(resumes()[0]!.options, { deliverAs: "followUp" });
      assert.equal(pi.sentUserMessages.length, 1, "the resume is the only user message");
      assert.equal(clock.pendingCount(), 0, "no goal reminder is scheduled");
    });

    for (const completion of ["callback", "both"] as const) {
      test(`an autonomous lever compaction resumes exactly once after ${completion} release`, async () => {
        const { pi, ctx, resumes, lever, compactCall, ours } = resumeRun();
        await lever();
        compactCall().onComplete!({} as never);
        if (completion === "both") {
          pi.handlers.get("session_compact")!(ours, ctx);
          compactCall().onComplete!({} as never);
        }
        await new Promise((resolve) => setImmediate(resolve));
        assert.equal(resumes().length, 1);
        assert.equal(pi.sentUserMessages.length, 1);
      });
    }

    for (const busyAtCallback of [false, true]) {
      test(`a session_compact release that runs before onComplete defers the resume to onComplete${busyAtCallback ? " (busy by then)" : ""}`, async () => {
        // Another extension's async session_compact handler lets ws's deferred
        // release run while Pi's prompt() would still throw; the resume waits
        // for the lever's onComplete, which Pi calls after compact() returned.
        const { pi, ctx, idle, resumes, lever, compactCall, ours } = resumeRun();
        await lever();
        pi.handlers.get("session_compact")!(ours, ctx);
        await new Promise((resolve) => setImmediate(resolve));
        assert.equal(leadCompactingRef.current, false, "the session_compact release ran first");
        assert.deepEqual(pi.sentUserMessages, [], "no resume from the session_compact release");
        // A run started by a flushed push only queues the followUp behind it.
        if (busyAtCallback) idle.current = false;
        compactCall().onComplete!({} as never);
        assert.equal(resumes().length, 1);
        assert.deepEqual(resumes()[0]!.options, { deliverAs: "followUp" });
        compactCall().onComplete!({} as never);
        assert.equal(pi.sentUserMessages.length, 1, "exactly one resume per operation");
      });
    }

    test("a lever compaction after an advisory preparation sends nothing", async () => {
      const { pi, ctx, usage, preparations, lever, complete } = resumeRun();
      usage.percent = 60;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1, "advisory nudge queued");
      await lever(); // inside the nudge's own run
      await complete();
      assert.deepEqual(pi.sentUserMessages, []);
    });

    test("a lever compaction from a milestone wake turn takes the milestone route: no resume unless continue_after_compact is set", async () => {
      for (const [args, expected] of [[{}, 0], [{ continue_after_compact: true }, 1]] as const) {
        const { pi, ctx, usage, preparations, resumes, lever, complete } = resumeRun();
        usage.percent = 55;
        pi.handlers.get("agent_end")!({}, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        usage.percent = 62;
        pi.handlers.get("agent_end")!({}, ctx);
        assert.equal(preparations().length, 1, "only the advisory is a preparation message");
        assert.equal(pi.sentMessages.filter((m) => (m.content as { customType?: string }).customType === LEAD_CONTEXT_MILESTONE_CUSTOM_TYPE).length, 1, "milestone wake turn queued");
        await lever(args); // inside the milestone's own run
        await complete();
        assert.equal(resumes().length, expected, JSON.stringify(args));
        assert.equal(pi.sentUserMessages.length, expected);
      }
    });

    test("a lever compaction after a reroute preparation sends nothing", async () => {
      const { pi, ctx, preparations, lever, complete } = resumeRun();
      assert.deepEqual(pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx), { cancel: true });
      pi.handlers.get("session_compact_failed")!({ reason: "manual", aborted: true, errorMessage: undefined, willRetry: false, fromExtension: false }, ctx);
      await new Promise((resolve) => setImmediate(resolve));
      assert.equal(preparations().length, 1, "reroute preparation queued");
      assert.deepEqual(pi.sentUserMessages, [], "the cancelled /compact itself resumes nothing");
      await lever();
      await complete();
      assert.deepEqual(pi.sentUserMessages, []);
    });

    test("a preparation whose run ended before the lever call counts as autonomous and resumes", async () => {
      const { pi, ctx, usage, preparations, resumes, lever, complete } = resumeRun();
      usage.percent = 60;
      pi.handlers.get("agent_end")!({}, ctx);
      assert.equal(preparations().length, 1, "advisory nudge queued");
      pi.handlers.get("agent_end")!({}, ctx); // the nudge's run ended without the lever
      await lever();
      await complete();
      assert.equal(resumes().length, 1);
    });

    for (const point of ["before release", "after the session_compact release"] as const) {
      test(`a session shutdown ${point} sends no resume`, async () => {
        const { pi, ctx, handle, lever, compactCall, ours } = resumeRun();
        await lever();
        if (point === "after the session_compact release") {
          pi.handlers.get("session_compact")!(ours, ctx);
          await new Promise((resolve) => setImmediate(resolve));
        }
        handle.resetCompactionStateForShutdown();
        pi.handlers.get("session_compact")!(ours, ctx);
        compactCall().onComplete!({} as never);
        await new Promise((resolve) => setImmediate(resolve));
        assert.deepEqual(pi.sentUserMessages, []);
      });
    }

    test("a non-idle session at release sends nothing", async () => {
      const { pi, idle, lever, complete } = resumeRun();
      await lever();
      idle.current = false; // owner input queued during compaction already started a run
      await complete();
      assert.deepEqual(pi.sentUserMessages, []);
      assert.equal(leadCompactingRef.current, false, "the hold is still released");
    });

    for (const failure of ["callback", "event"] as const) {
      test(`a failed compaction (${failure}) sends no resume`, async () => {
        const { pi, ctx, lever, compactCall } = resumeRun();
        await lever();
        if (failure === "callback") compactCall().onError!(new Error("boom"));
        else pi.handlers.get("session_compact_failed")!({ reason: "manual", aborted: true, errorMessage: undefined, willRetry: false, fromExtension: false }, ctx);
        await new Promise((resolve) => setImmediate(resolve));
        if (failure === "event") compactCall().onError!(new Error("boom"));
        assert.equal(leadCompactingRef.current, false);
        assert.deepEqual(pi.sentUserMessages, []);
      });
    }

    test("the resume waits for the held-push wake's agent_start instead of issuing a second idle prompt", async () => {
      const wakePi = { on: (event: string, fn: () => void) => { if (event === "session_shutdown") shutdown = fn; } } as unknown as ExtensionAPI;
      let shutdown: (() => void) | undefined;
      registerPushFlush(wakePi, { delayMs: () => 10, scheduleTimer: () => 0 as unknown as NodeJS.Timeout, clearTimer: () => {} });
      try {
        const { pi, ctx, resumes, lever, complete } = resumeRun();
        await lever();
        heldPushQueue.push({
          kind: "raw",
          deliverAs: "followUp",
          message: { customType: "ws-agent-advisory", content: "held fixture", display: true, details: { advisory: "held-fixture" } },
        });
        await complete();
        // Pi marks a prompt streaming only after async preflight, so a second
        // idle prompt here would also pass its idle check and be rejected as
        // "already processing", desyncing the host's run flag.
        assert.deepEqual(pi.sentUserMessages.map((m) => m.content), [labeled(buildPushWakeLine(1))], "exactly one adapter prompt in flight before its start");
        assert.equal(leadWakeStartPendingRef.current, true);
        pi.streaming.current = true;
        pi.handlers.get("agent_start")!({}, ctx);
        assert.deepEqual(pi.sentUserMessages.map((m) => m.content), [labeled(buildPushWakeLine(1)), sentResume], "the resume follows the wake's run");
        assert.deepEqual(pi.sentUserMessages[1]!.options, { deliverAs: "followUp" });
        assert.equal(resumes().length, 1);
        pi.handlers.get("agent_start")!({}, ctx);
        assert.equal(resumes().length, 1, "delivered once");
      } finally {
        shutdown?.();
      }
    });

    test("with a goal active only the goal reminder is sent, for a hard route too", async () => {
      const { pi, ctx, usage, preparations, lever, complete, clock } = resumeRun();
      await pi.commands.get("goal")!("ship", ctx);
      usage.percent = 85;
      pi.handlers.get("turn_end")!({}, ctx);
      assert.equal(preparations().length, 1, "hard steer sent");
      await lever();
      await complete();
      clock.fire();
      assert.equal(pi.sentUserMessages.length, 2);
      assert.equal(pi.sentUserMessages[0]!.content, labeled("Goal armed: ship"));
      assert.match(pi.sentUserMessages[1]!.content as string, /Goal yet running: "ship"/);
      assert.ok(!pi.sentUserMessages.some((m) => m.content === sentResume), "no resume beside the goal reminder");
    });

    test("the ws-compact description no longer limits resumption to an active goal", () => {
      const { pi } = resumeRun();
      const description = (pi.tools.get("ws-compact") as unknown as { description: string }).description;
      assert.ok(description.endsWith(
        "After compaction, an active goal keeps running. With no goal, a resume message follows when you called this on your own or after the hard-threshold notice; after the advisory nudge, a milestone, or a user /compact, the next move is the user's unless you set continue_after_compact to true because you hold known remaining work that does not await the user.",
      ));
      assert.doesNotMatch(description, /Under an active goal the goal loop continues/);
    });

    describe("explicit continue_after_compact (261004)", () => {
      test("the lever schema adds an optional boolean on top of the required prose fields", () => {
        const { pi } = resumeRun();
        const schema = (pi.tools.get("ws-compact") as unknown as { parameters: ReturnType<typeof leadCompactParameterSchema> }).parameters;
        const prose = leadProseParameterSchema();
        assert.deepEqual(schema.required, prose.required, "the flag is not required");
        assert.deepEqual(schema.required, LEAD_PROSE_SECTIONS.map((section) => section.key));
        for (const key of prose.required) assert.deepEqual(schema.properties[key], prose.properties[key]);
        assert.equal(schema.properties.continue_after_compact.type, "boolean");
        assert.deepEqual(Object.keys(schema.properties), [...Object.keys(prose.properties), "continue_after_compact", "required_rereads", "references"]);
        assert.match(schema.properties.continue_after_compact.description, /even after the advisory nudge, a milestone, or a user \/compact\./);
      });

      for (const route of ["advisory", "reroute"] as const) {
        test(`a ${route} lever call with true resumes exactly once after idle release`, async () => {
          const { pi, ctx, usage, preparations, resumes, lever, complete } = resumeRun();
          if (route === "advisory") {
            usage.percent = 60;
            pi.handlers.get("agent_end")!({}, ctx);
          } else {
            assert.deepEqual(pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx), { cancel: true });
            pi.handlers.get("session_compact_failed")!({ reason: "manual", aborted: true, errorMessage: undefined, willRetry: false, fromExtension: false }, ctx);
            await new Promise((resolve) => setImmediate(resolve));
          }
          assert.equal(preparations().length, 1, `${route} preparation queued`);
          await lever({ continue_after_compact: true }); // the abort's agent_end and compact hook have already cleared the preparation
          assert.equal(resumes().length, 0, "nothing is sent before release");
          await complete();
          assert.equal(resumes().length, 1);
          assert.deepEqual(resumes()[0]!.options, { deliverAs: "followUp" });
          assert.equal(pi.sentUserMessages.length, 1, "the resume is the only user message");
        });

        test(`a ${route} lever call with false or omitted keeps the non-resume default`, async () => {
          for (const args of [{ continue_after_compact: false }, {}, { continue_after_compact: "true" }, { continue_after_compact: 1 }]) {
            const { pi, ctx, usage, lever, complete } = resumeRun();
            if (route === "advisory") {
              usage.percent = 60;
              pi.handlers.get("agent_end")!({}, ctx);
            } else {
              pi.handlers.get("session_before_compact")!(compactionEvent("manual"), ctx);
              pi.handlers.get("session_compact_failed")!({ reason: "manual", aborted: true, errorMessage: undefined, willRetry: false, fromExtension: false }, ctx);
              await new Promise((resolve) => setImmediate(resolve));
            }
            await lever(args);
            await complete();
            assert.deepEqual(pi.sentUserMessages.filter((m) => m.content === sentResume), [], JSON.stringify(args));
          }
        });
      }

      test("false does not disable the resume of a hard or autonomous route", async () => {
        const hard = resumeRun();
        hard.usage.percent = 85;
        hard.pi.handlers.get("turn_end")!({}, hard.ctx);
        assert.equal(hard.preparations().length, 1, "hard steer sent");
        await hard.lever({ continue_after_compact: false });
        await hard.complete();
        assert.equal(hard.resumes().length, 1);
        const auto = resumeRun();
        await auto.lever({ continue_after_compact: false });
        await auto.complete();
        assert.equal(auto.resumes().length, 1);
      });

      test("true on a resuming route still sends exactly one resume", async () => {
        const { resumes, lever, complete } = resumeRun();
        await lever({ continue_after_compact: true });
        await complete();
        assert.equal(resumes().length, 1);
      });

      test("an active goal keeps its reminder path with no flag-driven resume", async () => {
        const { pi, ctx, lever, complete, clock } = resumeRun();
        await pi.commands.get("goal")!("ship", ctx);
        await lever({ continue_after_compact: true });
        await complete();
        clock.fire();
        assert.ok(!pi.sentUserMessages.some((m) => m.content === sentResume), "no resume beside the goal reminder");
        assert.match(pi.sentUserMessages[pi.sentUserMessages.length - 1]!.content as string, /Goal yet running: "ship"/);
      });

      test("failure, shutdown and a non-idle release suppress the flag-driven resume", async () => {
        const failed = resumeRun();
        await failed.lever({ continue_after_compact: true });
        failed.compactCall().onError!(new Error("boom"));
        await new Promise((resolve) => setImmediate(resolve));
        assert.deepEqual(failed.pi.sentUserMessages, []);

        const down = resumeRun();
        await down.lever({ continue_after_compact: true });
        down.handle.resetCompactionStateForShutdown();
        down.pi.handlers.get("session_compact")!(down.ours, down.ctx);
        down.compactCall().onComplete!({} as never);
        await new Promise((resolve) => setImmediate(resolve));
        assert.deepEqual(down.pi.sentUserMessages, []);

        const busy = resumeRun();
        await busy.lever({ continue_after_compact: true });
        busy.idle.current = false;
        await busy.complete();
        assert.deepEqual(busy.pi.sentUserMessages, []);
      });

      test("the flag belongs to its own operation: a later unflagged advisory call does not resume", async () => {
        const { pi, ctx, usage, preparations, resumes, lever, complete } = resumeRun();
        usage.percent = 60;
        pi.handlers.get("agent_end")!({}, ctx);
        await lever({ continue_after_compact: true });
        await complete();
        assert.equal(resumes().length, 1);
        // A second, unflagged advisory operation in the same session.
        pi.handlers.get("agent_start")!({}, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        usage.percent = 10;
        pi.handlers.get("agent_end")!({}, ctx);
        usage.percent = 60;
        pi.handlers.get("agent_start")!({}, ctx);
        pi.handlers.get("agent_end")!({}, ctx);
        assert.ok(preparations().length >= 2, "second advisory preparation queued");
        await lever();
        await complete();
        assert.equal(resumes().length, 1, "still only the first operation's resume");
      });
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
      assert.match((steers[0]!.content as { content: string }).content, /hard compaction point \(60% \/ 600 tokens\)/);
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
