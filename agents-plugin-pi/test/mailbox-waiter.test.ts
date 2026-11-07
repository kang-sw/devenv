/**
 * Unit tests for mailbox-waiter.ts: the session-bound wait/drain/admit loop
 * (260914 pi native mailbox push). Injected fakes exercise arrival -> push;
 * small subprocess fixtures verify shutdown and release-launcher ordering
 * without a live mailbox.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { access, chmod, mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { dashboardUserMessage } from "../src/dashboard-feed.ts";
import {
  afterCompactionHold,
  attachMailboxRuntimeCleanup,
  boardSteerDeps,
  boardWakeOutcome,
  buildMailboxPushMessage,
  buildMailboxWaitArgv,
  createBridgeDrain,
  createBridgeInboxDrain,
  DASHBOARD_INBOX_DRAIN_TOOL,
  createMailboxWaiterSlot,
  createSubprocessWait,
  mapMailboxWaitExit,
  reportDistinctLinesOnce,
  resolveMailboxSelfSlug,
  sessionMailboxWaitOptions,
  shouldArmMailboxWaiter,
  stageMailboxRuntime,
  startMailboxWaiter,
  WS_MAILBOX_CUSTOM_TYPE,
  type BoardInboxDeps,
  type BoardSteerDeps,
  type MailboxEnvelope,
  type MailboxWaitArm,
  type MailboxWaitOutcome,
  type MailboxWaiterHandle,
} from "../src/mailbox-waiter.ts";

/**
 * A fake `runWait` that yields a scripted sequence of outcomes and then blocks
 * until abort (resolving `"stopped"`), mirroring the real blocking wait. Ending
 * a script with `"stopped"` lets the loop exit on its own with no dangling
 * timer.
 */
function scriptedWait(script: MailboxWaitOutcome[]): { runWait: (signal: AbortSignal) => Promise<MailboxWaitOutcome>; calls: () => number } {
  let index = 0;
  return {
    calls: () => index,
    runWait: (signal) => {
      if (index < script.length) return Promise.resolve(script[index++]!);
      return new Promise<MailboxWaitOutcome>((resolve) => {
        if (signal.aborted) return resolve("stopped");
        signal.addEventListener("abort", () => resolve("stopped"), { once: true });
      });
    },
  };
}

const immediateSleep = (): Promise<void> => Promise.resolve();

async function waitForFile(path: string): Promise<void> {
  for (let attempt = 0; attempt < 800; attempt += 1) {
    try {
      await access(path);
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
  }
  throw new Error(`timed out waiting for ${path}`);
}

describe("startMailboxWaiter", () => {
  test("a mail wake drains and admits the whole drain as one unit, in order", async () => {
    const envelopes: MailboxEnvelope[] = [
      { from: "alice@machine", content: "first", sent_at: "2026-09-15T10:00:00Z" },
      { reply_to: "id:abc", content: "second", sent_at: "2026-09-15T10:00:01Z" },
    ];
    const admitted: MailboxEnvelope[][] = [];
    let drainCalls = 0;
    const waiter = startMailboxWaiter({
      ...scriptedWait(["mail", "stopped"]),
      drainMail: () => { drainCalls += 1; return Promise.resolve(envelopes); },
      admit: (drained) => admitted.push(drained),
      sleep: immediateSleep,
    });
    await waiter.done;
    assert.equal(drainCalls, 1, "one wake drains exactly once");
    assert.deepEqual(admitted, [envelopes], "one drain is one admit, so a busy lead gets one batch rather than one steer per envelope");
  });

  test("a timeout re-arms without draining or admitting", async () => {
    const admitted: MailboxEnvelope[] = [];
    let drainCalls = 0;
    const script = scriptedWait(["timeout", "stopped"]);
    const waiter = startMailboxWaiter({
      ...script,
      drainMail: () => { drainCalls += 1; return Promise.resolve([]); },
      admit: (drained) => admitted.push(...drained),
      sleep: immediateSleep,
    });
    await waiter.done;
    assert.equal(drainCalls, 0, "a timeout is not a delivery — nothing is drained");
    assert.equal(admitted.length, 0);
    assert.equal(script.calls(), 2, "the loop re-armed after the timeout");
  });

  test("stop() aborts an in-flight blocking wait and ends the loop", async () => {
    const admitted: MailboxEnvelope[] = [];
    const waiter = startMailboxWaiter({
      ...scriptedWait([]), // immediately exhausted -> blocks until abort
      drainMail: () => Promise.resolve([]),
      admit: (drained) => admitted.push(...drained),
      sleep: immediateSleep,
    });
    waiter.stop();
    await waiter.done; // resolves only because stop() aborted the pending wait
    assert.equal(admitted.length, 0);
  });

  test("a wait failure that arrives after stop() does not notify through a stale diagnostic sink", async () => {
    let rejectWait!: (error: Error) => void;
    const diagnostics: string[] = [];
    const waiter = startMailboxWaiter({
      runWait: () => new Promise<MailboxWaitOutcome>((_resolve, reject) => { rejectWait = reject; }),
      drainMail: () => Promise.resolve([]),
      admit: () => assert.fail("the stopped waiter must not admit mail"),
      sleep: immediateSleep,
      onError: (message) => diagnostics.push(message),
    });
    waiter.stop();
    rejectWait(new Error("late subprocess error"));
    await waiter.done;
    assert.deepEqual(diagnostics, [], "a stopped waiter must not invoke its session-bound diagnostic sink");
  });

  test("a failed drain backs off before re-arming so it cannot hot-spin the peek", async () => {
    const sleeps: number[] = [];
    const errors: string[] = [];
    let drainCalls = 0;
    const waiter = startMailboxWaiter({
      ...scriptedWait(["mail", "stopped"]),
      drainMail: () => { drainCalls += 1; return Promise.reject(new Error("recv unavailable")); },
      admit: () => assert.fail("a failed drain must not admit anything"),
      sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); },
      onError: (message) => errors.push(message),
    });
    await waiter.done;
    assert.equal(drainCalls, 1);
    assert.equal(sleeps.length, 1, "a drain failure backs off exactly once before re-arming");
    assert.ok(sleeps[0]! > 0, "the backoff is a positive delay");
    assert.ok(errors.some((message) => message.includes("drain failed")), "the drain failure is reported");
  });

  test("an error outcome backs off, and a throwing wait is treated as an error", async () => {
    const sleeps: number[] = [];
    const errors: string[] = [];
    let waitCalls = 0;
    const waiter = startMailboxWaiter({
      runWait: (signal) => {
        waitCalls += 1;
        if (waitCalls === 1) return Promise.reject(new Error("spawn boom"));
        return new Promise<MailboxWaitOutcome>((resolve) => signal.addEventListener("abort", () => resolve("stopped"), { once: true }));
      },
      drainMail: () => Promise.resolve([]),
      admit: () => assert.fail("no mail, no admit"),
      sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); },
      onError: (message) => errors.push(message),
    });
    // Let the rejected wait + backoff + second (blocking) wait settle, then stop.
    await new Promise((resolve) => setTimeout(resolve, 0));
    waiter.stop();
    await waiter.done;
    assert.equal(sleeps.length, 1, "a wait failure backs off once");
    assert.ok(errors.some((message) => message.includes("wait failed")), "the wait failure is reported");
  });

  test("an omitted error sink does not write waiter failures to console.error", async (t) => {
    const consoleErrors: unknown[][] = [];
    t.mock.method(console, "error", (...args: unknown[]) => { consoleErrors.push(args); });
    let calls = 0;
    const waiter = startMailboxWaiter({
      runWait: async () => calls++ === 0 ? Promise.reject(new Error("spawn boom")) : "stopped",
      drainMail: () => Promise.resolve([]),
      admit: () => assert.fail("no mail is admitted after a failed wait"),
      sleep: immediateSleep,
    });
    await waiter.done;
    assert.deepEqual(consoleErrors, [], "an omitted sink must not write waiter failures to the terminal");
  });

  test("a mail wake that drains nothing (peek/drain race) re-arms without admitting or backing off", async () => {
    const admitted: MailboxEnvelope[] = [];
    const sleeps: number[] = [];
    const script = scriptedWait(["mail", "stopped"]);
    const waiter = startMailboxWaiter({
      ...script,
      drainMail: () => Promise.resolve([]), // recv drained nothing (already emptied elsewhere)
      admit: (drained) => admitted.push(...drained),
      sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); },
    });
    await waiter.done;
    assert.equal(admitted.length, 0, "an empty drain admits nothing");
    assert.equal(sleeps.length, 0, "an empty drain is a success, not a failure — no backoff");
    assert.equal(script.calls(), 2, "the loop re-armed immediately after the empty drain");
  });

  test("a throwing admit is reported and the loop re-arms without backoff", async () => {
    const errors: string[] = [];
    const sleeps: number[] = [];
    const script = scriptedWait(["mail", "stopped"]);
    const waiter = startMailboxWaiter({
      ...script,
      drainMail: () => Promise.resolve([{ from: "a", content: "boom" }]),
      admit: () => { throw new Error("send rejected"); },
      sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); },
      onError: (message) => errors.push(message),
    });
    await waiter.done;
    assert.ok(errors.some((message) => message.includes("admit failed")));
    assert.equal(sleeps.length, 0, "the mail was drained, so a re-arm cannot hot-spin on it");
    assert.equal(script.calls(), 2);
  });
});

/**
 * A fake board-aware `runWait`: records each arm's `--board-inbox` flag and
 * yields scripted outcomes, then blocks until its own signal aborts. An
 * abort from `leadSettled` resolves `stopped` like the real child does.
 */
function boardWait(script: MailboxWaitOutcome[]): { runWait: (signal: AbortSignal, arm: MailboxWaitArm) => Promise<MailboxWaitOutcome>; arms: boolean[]; modes: string[]; blocked: () => boolean } {
  const arms: boolean[] = [];
  // Each arm's board mode: `inbox`, `steer`, or `none`.
  const modes: string[] = [];
  let index = 0;
  let blocking = false;
  return {
    arms,
    modes,
    blocked: () => blocking,
    runWait: (signal, arm) => {
      arms.push(arm.boardInbox);
      modes.push(arm.boardInbox ? "inbox" : arm.boardSteer ? "steer" : "none");
      if (index < script.length) return Promise.resolve(script[index++]!);
      blocking = true;
      return new Promise<MailboxWaitOutcome>((resolve) => {
        const end = (): void => { blocking = false; resolve("stopped"); };
        if (signal.aborted) return end();
        signal.addEventListener("abort", end, { once: true });
      });
    },
  };
}

function fakeBoard(overrides: Partial<BoardInboxDeps> & { held?: string[][] } = {}): BoardInboxDeps & { injected: string[][]; drains: () => number; idleRef: { current: boolean } } {
  const injected: string[][] = [];
  const queue = overrides.held ?? [["hello from the page"]];
  let drains = 0;
  const idleRef = { current: true };
  return {
    injected,
    idleRef,
    drains: () => drains,
    enabled: overrides.enabled ?? (() => true),
    idle: overrides.idle ?? (() => idleRef.current),
    drain: overrides.drain ?? (() => { drains += 1; return Promise.resolve(queue.shift() ?? []); }),
    inject: overrides.inject ?? ((texts) => { injected.push(texts); }),
    ...(overrides.steer ? { steer: overrides.steer } : {}),
  };
}

async function until(predicate: () => boolean, label: string): Promise<void> {
  for (let attempt = 0; attempt < 500; attempt += 1) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 2));
  }
  throw new Error(`timed out waiting for ${label}`);
}

describe("startMailboxWaiter: dashboard board inbox", () => {
  test("an idle board wake drains and injects at once, then re-arms with the inbox flag", async () => {
    const wait = boardWait(["board"]);
    const board = fakeBoard();
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => assert.fail("a board-only wake drains no mail"), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["hello from the page"]]);
    assert.deepEqual(wait.arms, [true, true]);
  });

  test("a mid-turn board wake leaves the entry held, re-arms without the flag, and injects only at the lead's settle", async () => {
    const wait = boardWait(["board"]);
    const board = fakeBoard();
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the held wait");
    assert.deepEqual(wait.arms, [true, false], "the held entry would wake a flagged wait at once, so the flag is dropped");
    assert.equal(board.drains(), 0, "nothing is drained while the turn runs, so merge and cancel still act on the entry");
    assert.deepEqual(board.injected, []);

    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.injected.length > 0 && wait.blocked(), "the settle drain and re-arm");
    waiter.stop();
    await waiter.done;
    assert.equal(board.drains(), 1);
    assert.deepEqual(board.injected, [["hello from the page"]]);
    assert.deepEqual(wait.arms, [true, false, true], "the flag is restored after the drain");
  });

  test("a settle with nothing held neither re-arms nor drains", async () => {
    const wait = boardWait([]);
    const board = fakeBoard();
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the first wait");
    waiter.leadSettled!();
    await new Promise((resolve) => setTimeout(resolve, 10));
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.arms, [true]);
    assert.equal(board.drains(), 0);
  });

  test("a turn that ends before the hold is recorded still releases the message before the next wait", async () => {
    const wait = boardWait(["board"]);
    // Busy at the wake, idle by the time the loop re-arms.
    let checks = 0;
    const board = fakeBoard({ idle: () => checks++ > 0 });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["hello from the page"]]);
    assert.deepEqual(wait.arms, [true, true]);
  });

  test("a held entry the page cancelled is never injected", async () => {
    const wait = boardWait(["board"]);
    const board = fakeBoard({ held: [[]] });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the held wait");
    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.drains() === 1 && wait.blocked(), "the settle drain");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [], "the cancel claimed the entry, so the drain found nothing");
    assert.deepEqual(wait.arms, [true, false, true]);
  });

  test("a merged entry is injected once, as one message", async () => {
    const wait = boardWait(["board"]);
    const board = fakeBoard({ held: [["first send\n\nsecond send\n\n(03) a"]] });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["first send\n\nsecond send\n\n(03) a"]]);
  });

  test("with the knob off the wait is armed without the inbox flag and nothing is drained", async () => {
    const wait = boardWait(["timeout"]);
    const board = fakeBoard({ enabled: () => Promise.resolve(false) });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the second wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.arms, [false, false]);
    assert.equal(board.drains(), 0);
  });

  test("a knob read that throws counts as on", async () => {
    const wait = boardWait([]);
    const board = fakeBoard({ enabled: () => { throw new Error("config unavailable"); } });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(wait.blocked, "the first wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.arms, [true]);
  });

  test("a wake with both kinds drains the mail and the board", async () => {
    const wait = boardWait(["mail+board"]);
    const board = fakeBoard();
    const admitted: MailboxEnvelope[][] = [];
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([{ from: "peer@machine", content: "hi" }]), admit: (envelopes) => admitted.push(envelopes), sleep: immediateSleep, board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.equal(admitted.length, 1);
    assert.deepEqual(board.injected, [["hello from the page"]]);
  });

  test("a failed board drain backs off and re-arms with the flag, so the level-triggered wait retries it", async () => {
    const wait = boardWait(["board"]);
    const sleeps: number[] = [];
    const errors: string[] = [];
    const board = fakeBoard({ drain: () => Promise.reject(new Error("drain refused")) });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); }, errorBackoffMs: 42, onError: (message) => errors.push(message), board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(sleeps, [42]);
    assert.deepEqual(errors, ["board drain failed: drain refused"]);
    assert.deepEqual(wait.arms, [true, true]);
  });

  test("a throwing inject is reported and the loop keeps running", async () => {
    const wait = boardWait(["board"]);
    const errors: string[] = [];
    const board = fakeBoard({ inject: () => { throw new Error("no session"); } });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, onError: (message) => errors.push(message), board });
    await until(wait.blocked, "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(errors, ["board inject failed: no session"]);
  });

  test("without board deps every wait is armed without the flag", async () => {
    const wait = boardWait(["mail"]);
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep });
    await until(wait.blocked, "the re-armed wait");
    assert.equal(waiter.leadSettled !== undefined, true);
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.arms, [false, false]);
  });
});

/**
 * A fake steer side: `steerable` answers from `answers` in order and then
 * from `ref`, so a test can flip it at a chosen check.
 */
function fakeSteer(overrides: Partial<BoardSteerDeps> & { held?: string[][]; answers?: boolean[] } = {}): BoardSteerDeps & { steered: string[][]; drains: () => number; ref: { current: boolean } } {
  const steered: string[][] = [];
  const queue = overrides.held ?? [["now please"]];
  const answers = [...(overrides.answers ?? [])];
  const ref = { current: true };
  let drains = 0;
  return {
    steered,
    ref,
    drains: () => drains,
    steerable: overrides.steerable ?? (() => (answers.length > 0 ? answers.shift()! : ref.current)),
    drain: overrides.drain ?? (() => { drains += 1; return Promise.resolve(queue.shift() ?? []); }),
    inject: overrides.inject ?? ((texts) => { steered.push(texts); }),
  };
}

describe("startMailboxWaiter: steering a promoted dashboard message", () => {
  test("a held message arms the steer-mode wait; a promoted one wakes it, is drained alone, and is steered into the running turn", async () => {
    // The first wake holds a follow-up mid-turn; the second is the promotion.
    const wait = boardWait(["board", "board"]);
    const steer = fakeSteer();
    const board = fakeBoard({ held: [["a follow-up"]], steer });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(() => steer.steered.length > 0 && wait.blocked(), "the steer and re-arm");
    assert.deepEqual(wait.modes, ["inbox", "steer", "steer"], "a held follow-up does not wake the steer-mode wait, so it re-arms in steer mode");
    assert.deepEqual(steer.steered, [["now please"]]);
    assert.equal(steer.drains(), 1);
    assert.equal(board.drains(), 0, "the full drain never runs mid-turn");
    assert.deepEqual(board.injected, []);

    steer.ref.current = false;
    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.injected.length > 0 && wait.blocked(), "the settle drain");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["a follow-up"]], "the follow-up arrives after the steered message, at the settle");
    assert.equal(wait.modes.at(-1), "inbox");
  });

  test("while a steer would be refused the wait is armed without steer mode, so a steered entry cannot wake it at once; leadSettled restores steer mode", async () => {
    // The fake reads its script at each arm: the promotion's wake is added once steer mode is back.
    const script: MailboxWaitOutcome[] = ["board"];
    const wait = boardWait(script);
    const steer = fakeSteer();
    steer.ref.current = false; // a compaction holds the session
    const board = fakeBoard({ steer });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(() => wait.modes.length === 2, "the held wait");
    assert.deepEqual(wait.modes, ["inbox", "none"]);
    assert.equal(steer.drains(), 0);

    // The compaction ends mid-turn: its leadSettled re-arms in steer mode, and the promoted entry is steered.
    steer.ref.current = true;
    script.push("board");
    waiter.leadSettled!();
    await until(() => steer.steered.length > 0 && wait.blocked(), "the steer after the hold");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.modes, ["inbox", "none", "steer", "steer"]);
    assert.deepEqual(steer.steered, [["now please"]]);
  });

  test("a steer wake that finds a steer refused leaves the entry held and re-arms without steer mode", async () => {
    const wait = boardWait(["board", "board"]);
    // Steerable when armed, refused at the wake, and after.
    const steer = fakeSteer({ answers: [true, false] });
    steer.ref.current = false;
    const board = fakeBoard({ steer });
    board.idleRef.current = false;
    const sleeps: number[] = [];
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); }, board });
    await until(() => wait.modes.length === 3 && wait.blocked(), "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(wait.modes, ["inbox", "steer", "none"]);
    assert.equal(steer.drains(), 0, "nothing is claimed while a steer would be refused");
    assert.deepEqual(sleeps, [], "no backoff is needed: the next wait cannot wake on the left entry");
  });

  test("a steer drain that outlasts the run carries its texts to the settle, injected ahead of the held follow-up", async () => {
    const wait = boardWait(["board", "board"]);
    // Steerable when armed and at the wake; the run ends during the drain.
    const steer = fakeSteer({ answers: [true, true, false] });
    steer.ref.current = false;
    const board = fakeBoard({ held: [["a follow-up"]], steer });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(() => steer.drains() === 1 && wait.blocked(), "the steer drain");
    assert.deepEqual(steer.steered, [], "a steer the run would refuse is not sent");
    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.injected.length > 0 && wait.blocked(), "the settle drain");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["now please", "a follow-up"]]);
  });

  test("an idle drain overtaken by a compaction carries its texts to the settle instead of a submission Pi would refuse", async () => {
    const wait = boardWait(["board"]);
    let releaseDrain!: (texts: string[]) => void;
    let drainStarted = false;
    const board = fakeBoard({
      drain: () => {
        drainStarted = true;
        return new Promise<string[]>((resolve) => { releaseDrain = resolve; });
      },
    });
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, board });
    await until(() => drainStarted, "the idle drain");
    // A manual /compact starts while the drain call is in flight.
    board.idleRef.current = false;
    releaseDrain(["pick B"]);
    await until(wait.blocked, "the re-armed wait");
    assert.deepEqual(board.injected, [], "nothing is submitted while the compaction holds the session");
    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.injected.length > 0 && wait.blocked(), "the settle");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["pick B"]]);
  });

  test("a throwing steer inject is reported and its texts go out at the settle; a carried text goes out even with the knob turned off", async () => {
    const wait = boardWait(["board", "board"]);
    const errors: string[] = [];
    const steer = fakeSteer({ inject: () => { throw new Error("no session"); } });
    let knob = true;
    const board = fakeBoard({ held: [["a follow-up"]], steer, enabled: () => knob });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: immediateSleep, onError: (message) => errors.push(message), board });
    await until(() => steer.drains() === 1 && wait.blocked(), "the steer attempt");
    assert.deepEqual(errors, ["board steer failed: no session; delivering it after the turn instead"]);
    knob = false;
    board.idleRef.current = true;
    waiter.leadSettled!();
    await until(() => board.injected.length > 0 && wait.blocked(), "the settle");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(board.injected, [["now please"]], "the carried text goes out; the held follow-up is not drained with the knob off");
    assert.equal(board.drains(), 0);
  });

  test("a failed steer drain backs off and re-arms in steer mode, so the level-triggered wait retries it", async () => {
    const wait = boardWait(["board", "board"]);
    const sleeps: number[] = [];
    const errors: string[] = [];
    const steer = fakeSteer({ drain: () => Promise.reject(new Error("drain refused")) });
    const board = fakeBoard({ steer });
    board.idleRef.current = false;
    const waiter = startMailboxWaiter({ runWait: wait.runWait, drainMail: () => Promise.resolve([]), admit: () => {}, sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); }, errorBackoffMs: 42, onError: (message) => errors.push(message), board });
    await until(() => wait.modes.length === 3 && wait.blocked(), "the re-armed wait");
    waiter.stop();
    await waiter.done;
    assert.deepEqual(sleeps, [42]);
    assert.deepEqual(errors, ["board steer drain failed: drain refused"]);
    assert.deepEqual(wait.modes, ["inbox", "steer", "steer"]);
  });
});

describe("boardSteerDeps", () => {
  test("steerable is the run flag without a compaction hold", () => {
    const state = { open: false, compacting: false };
    const deps = boardSteerDeps({ turnOpen: () => state.open, compacting: () => state.compacting, drain: () => Promise.resolve([]), sendUserMessage: () => {} });
    assert.equal(deps.steerable(), false, "no run");
    state.open = true;
    assert.equal(deps.steerable(), true);
    state.compacting = true;
    assert.equal(deps.steerable(), false, "a compaction refuses the prompt");
  });

  test("a steer is a labelled user message delivered as steer, which the feed records as a whiteboard user message", () => {
    const sent: [string, unknown][] = [];
    const deps = boardSteerDeps({ turnOpen: () => true, compacting: () => false, drain: () => Promise.resolve([]), sendUserMessage: (text, options) => { sent.push([text, options]); } });
    deps.inject(["now please", "(03) a"]);
    assert.equal(sent.length, 1);
    assert.deepEqual(sent[0]![1], { deliverAs: "steer" });
    // Pi turns sendUserMessage text into a role-user message, the one the feed reads at message_end.
    assert.deepEqual(dashboardUserMessage({ role: "user", content: [{ type: "text", text: sent[0]![0] }] }), { text: "now please\n\n(03) a", origin: "whiteboard" });
  });
});

describe("afterCompactionHold", () => {
  test("runs after a hold release that the same compaction event schedules later", async () => {
    let compacting = true;
    const seen: boolean[] = [];
    // The settle hook's handler runs first; the hold owner's handler then
    // schedules its own deferred release, as Pi dispatches one event's handlers in order.
    afterCompactionHold(() => seen.push(compacting));
    setImmediate(() => { compacting = false; });
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.deepEqual(seen, [false]);
  });
});

describe("boardWakeOutcome", () => {
  test("the kinds a board-inbox wait reports pick the outcome", () => {
    assert.equal(boardWakeOutcome(JSON.stringify({ kinds: ["board"], board: 1, unread: 0 })), "board");
    assert.equal(boardWakeOutcome(JSON.stringify({ kinds: ["mail"], board: 0, unread: 2 })), "mail");
    assert.equal(boardWakeOutcome(JSON.stringify({ kinds: ["mail", "board"] })), "mail+board");
  });

  test("output that cannot be read counts as both kinds", () => {
    for (const stdout of ["", "not json", "null", JSON.stringify({}), JSON.stringify({ kinds: "board" }), JSON.stringify({ kinds: [] })]) {
      assert.equal(boardWakeOutcome(stdout), "mail+board", stdout);
    }
  });
});

describe("createBridgeInboxDrain", () => {
  test("calls the adapter-only drain tool with the session key and json format and returns the texts in order", async () => {
    const calls: [string, Record<string, unknown>][] = [];
    const drain = createBridgeInboxDrain(async (name, args) => {
      calls.push([name, args]);
      return { content: [{ type: "text", text: JSON.stringify({ messages: [{ id: "a", text: "first" }, { id: "b", text: "second\n\n(01) ok" }] }) }] };
    }, "my-key");
    assert.deepEqual(await drain(), ["first", "second\n\n(01) ok"]);
    assert.deepEqual(calls, [[DASHBOARD_INBOX_DRAIN_TOOL, { session_key: "my-key", format: "json" }]]);
    assert.equal(DASHBOARD_INBOX_DRAIN_TOOL, "dashboard.inbox_drain");
  });

  test("steerOnly asks the tool for the steered messages alone", async () => {
    const calls: Record<string, unknown>[] = [];
    const drain = createBridgeInboxDrain(async (_name, args) => {
      calls.push(args);
      return { content: [{ type: "text", text: JSON.stringify({ messages: [{ id: "a", text: "now please" }] }) }] };
    }, "my-key", { steerOnly: true });
    assert.deepEqual(await drain(), ["now please"]);
    assert.deepEqual(calls, [{ session_key: "my-key", steer_only: true, format: "json" }]);
  });

  test("throws on a tool-level error so the loop backs off", async () => {
    const drain = createBridgeInboxDrain(async () => ({ isError: true, content: [{ type: "text", text: "unknown_session" }] }), "k");
    await assert.rejects(drain(), /unknown_session/);
  });

  test("an empty inbox, malformed output, and blank texts give no messages", async () => {
    for (const text of [JSON.stringify({ messages: [] }), "inbox empty", JSON.stringify({}), JSON.stringify({ messages: [{ text: "  " }, { text: 3 }, null] }), undefined]) {
      const drain = createBridgeInboxDrain(async () => ({ content: text === undefined ? [] : [{ type: "text", text }] }), "k");
      assert.deepEqual(await drain(), [], String(text));
    }
  });
});

describe("buildMailboxPushMessage", () => {
  test("a shared-layer sender uses its slug handle and carries the full envelope in details", () => {
    const message = buildMailboxPushMessage({ from: "scout@worktree", content: "run the ready ticket", sent_at: "2026-09-15T12:00:00Z" });
    assert.equal(message.customType, WS_MAILBOX_CUSTOM_TYPE);
    assert.equal(message.display, true);
    assert.equal(message.content, "mail from scout@worktree (2026-09-15T12:00:00Z):\nrun the ready ticket");
    assert.deepEqual(message.details, { from: "scout@worktree", content: "run the ready ticket", sent_at: "2026-09-15T12:00:00Z" });
  });

  test("a reply-id-only sender falls back to its reply_to handle", () => {
    const message = buildMailboxPushMessage({ reply_to: "id:headlessroutineremodelerpassage", content: "ack" });
    assert.equal(message.content, "mail from id:headlessroutineremodelerpassage:\nack");
    assert.deepEqual(message.details, { reply_to: "id:headlessroutineremodelerpassage", content: "ack" });
  });

  test("a handle-less envelope degrades to a placeholder rather than an empty head", () => {
    const message = buildMailboxPushMessage({ content: "orphan mail" });
    assert.equal(message.content, "mail from unknown:\norphan mail");
    assert.deepEqual(message.details, { content: "orphan mail" });
  });

  test("if both handles are present (contract says exactly one) from wins the head and both echo in details", () => {
    const message = buildMailboxPushMessage({ from: "alice@machine", reply_to: "id:beef", content: "hi", sent_at: "  2026-09-15T00:00:00Z  " });
    assert.equal(message.content, "mail from alice@machine (2026-09-15T00:00:00Z):\nhi");
    assert.deepEqual(message.details, { from: "alice@machine", reply_to: "id:beef", content: "hi", sent_at: "2026-09-15T00:00:00Z" });
  });
});

describe("stageMailboxRuntime", () => {
  test("overlapping generations keep distinct executables across source replacement, including simulated Windows locks", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-generation-"));
    const sourceDir = join(fixtureDir, "runtime");
    await mkdir(sourceDir);
    const source = join(sourceDir, "ws-mcp");
    await writeFile(source, "first");
    await chmod(source, 0o751);
    const first = stageMailboxRuntime(source);
    const locked = stageMailboxRuntime(source, () => { throw Object.assign(new Error("sharing violation"), { code: "EPERM" }); });
    try {
      await writeFile(source, "second");
      const second = stageMailboxRuntime(source);
      try {
        assert.notEqual(first.binaryPath, second.binaryPath);
        assert.notEqual(locked.binaryPath, second.binaryPath);
        assert.equal(await readFile(first.binaryPath, "utf8"), "first");
        assert.equal(await readFile(second.binaryPath, "utf8"), "second");
        if (process.platform !== "win32") assert.equal((await stat(second.binaryPath)).mode & 0o777, 0o751);
        assert.doesNotThrow(locked.cleanup, "Windows locked-file cleanup must not break shutdown");
        first.cleanup();
        assert.equal(existsSync(first.binaryPath), false);
        assert.equal(existsSync(second.binaryPath), true, "a delayed old-generation cleanup cannot delete the new generation");
      } finally {
        second.cleanup();
      }
    } finally {
      first.cleanup();
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });

  test("stop waits for actual child close before removing its staged executable", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-close-"));
    const sourceDir = join(fixtureDir, "runtime");
    await mkdir(sourceDir);
    const source = join(sourceDir, process.platform === "win32" ? "ws-mcp.exe" : "ws-mcp");
    const ready = join(fixtureDir, "ready");
    if (process.platform === "win32") {
      const goFile = join(fixtureDir, "wait-probe.go");
      await writeFile(goFile, [
        "package main",
        `import ("os";"time")`,
        `func main() { _=os.WriteFile(${JSON.stringify(ready)},[]byte("ready"),0600); time.Sleep(30*time.Second) }`,
      ].join("\n"));
      execFileSync(process.env.WS_PI_TEST_GO ?? "go", ["build", "-o", source, goFile], { cwd: fixtureDir, timeout: 120_000 });
    } else {
      await writeFile(source, [
        "#!/usr/bin/env python3",
        "import signal,time,sys",
        "def stop(_signal, _frame):",
        " time.sleep(0.25)",
        " sys.exit(130)",
        "signal.signal(signal.SIGTERM, stop)",
        `open(${JSON.stringify(ready)}, 'w').close()`,
        "time.sleep(30)",
        "",
      ].join("\n"));
      await chmod(source, 0o755);
    }
    const staged = stageMailboxRuntime(source);
    const waiter = attachMailboxRuntimeCleanup(startMailboxWaiter({
      runWait: createSubprocessWait({ launcherPath: "unused", pluginDir: fixtureDir, runtimeBinary: staged.binaryPath, sessionKey: "key" }),
      drainMail: async () => [], admit: () => {},
    }), staged);
    try {
      await waitForFile(ready);
      waiter.stop();
      assert.equal(existsSync(staged.binaryPath), true, "abort alone cannot unlink a still-running executable");
      await waiter.done;
      assert.equal(existsSync(staged.binaryPath), false);
    } finally {
      waiter.stop();
      await waiter.done;
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });
});

describe("createSubprocessWait", () => {
  test("a release-backed session retains the Python launcher invocation without an override", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-release-"));
    const launcherPath = join(fixtureDir, "inspect-argv.py");
    const observedPath = join(fixtureDir, "observed");
    await writeFile(launcherPath, [
      "import json,sys",
      `with open(${JSON.stringify(observedPath)}, "w") as output:`,
      "    json.dump(sys.argv[1:], output)",
      "raise SystemExit(3)",
      "",
    ].join("\n"));
    try {
      const options = sessionMailboxWaitOptions({ launcherPath, pluginDir: fixtureDir, sessionKey: "my-key", cwd: fixtureDir });
      assert.equal(options.runtimeBinary, undefined);
      assert.equal(await createSubprocessWait(options)(new AbortController().signal), "timeout");
      assert.deepEqual(JSON.parse(await readFile(observedPath, "utf8")), buildMailboxWaitArgv(options).slice(1));
    } finally {
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });

  test("a board-inbox wait passes the flag and reads which kind woke it from stdout", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-board-"));
    const launcherPath = join(fixtureDir, "board.py");
    await writeFile(launcherPath, [
      "import json,sys",
      "kinds=['board'] if '--board-inbox' in sys.argv else ['mail']",
      "print(json.dumps({'timed_out':False,'unread':0,'board':1,'kinds':kinds}),flush=True)",
      "",
    ].join("\n"));
    try {
      const wait = createSubprocessWait({ launcherPath, pluginDir: fixtureDir, sessionKey: "k" });
      assert.equal(await wait(new AbortController().signal, { boardInbox: true }), "board");
      assert.equal(await wait(new AbortController().signal, { boardInbox: false }), "mail", "a mail-only wait reads the exit code alone");
      await writeFile(launcherPath, [
        "import json,sys",
        "kinds=['board'] if '--board-steer' in sys.argv and '--board-inbox' not in sys.argv else ['mail']",
        "print(json.dumps({'timed_out':False,'unread':0,'board':1,'kinds':kinds}),flush=True)",
        "",
      ].join("\n"));
      assert.equal(await wait(new AbortController().signal, { boardInbox: false, boardSteer: true }), "board", "a steer-mode wait passes its flag and reads the kinds too");
    } finally {
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });

  test("release launcher settles on exit even if a Windows-style grandchild holds stderr open", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-grandchild-"));
    const launcherPath = join(fixtureDir, "launcher.py");
    const pidPath = join(fixtureDir, "grandchild-pid");
    await writeFile(launcherPath, [
      "import subprocess,sys",
      "child=subprocess.Popen([sys.executable,'-c','import time;time.sleep(15)'],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=sys.stderr)",
      `open(${JSON.stringify(pidPath)}, 'w').write(str(child.pid))`,
      "raise SystemExit(3)",
      "",
    ].join("\n"));
    try {
      const wait = createSubprocessWait({ launcherPath, pluginDir: fixtureDir, sessionKey: "my-key" });
      let timer: ReturnType<typeof setTimeout> | undefined;
      const timeout = new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error("wait held by grandchild's stderr")), 5_000); });
      try {
        assert.equal(await Promise.race([wait(new AbortController().signal), timeout]), "timeout");
      } finally {
        clearTimeout(timer);
      }
    } finally {
      if (existsSync(pidPath)) {
        try { process.kill(Number(await readFile(pidPath, "utf8"))); } catch { /* The child may already have exited. */ }
      }
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });

  test("child stderr is silent by default and reaches an injected diagnostic sink", async (t) => {
    const consoleErrors: unknown[][] = [];
    t.mock.method(console, "error", (...args: unknown[]) => { consoleErrors.push(args); });
    const missingLauncher = join(process.cwd(), "__ws-mailbox-waiter-missing-launcher__.py");

    const defaultOutcome = await createSubprocessWait({
      launcherPath: missingLauncher,
      pluginDir: process.cwd(),
      sessionKey: "my-key",
    })(new AbortController().signal);
    assert.equal(defaultOutcome, "error");
    assert.deepEqual(consoleErrors, [], "an omitted sink must not write child stderr to the terminal");

    const diagnostics: string[] = [];
    const injectedOutcome = await createSubprocessWait({
      launcherPath: missingLauncher,
      pluginDir: process.cwd(),
      sessionKey: "my-key",
      onStderr: (line) => diagnostics.push(line),
    })(new AbortController().signal);
    assert.equal(injectedOutcome, "error");
    assert.ok(diagnostics.length > 0, "the injected sink receives the child's stderr");
    assert.deepEqual(consoleErrors, [], "an injected sink replaces the raw-terminal default");
  });

  test("child stderr emitted after abort is not delivered to the diagnostic sink", async () => {
    const fixtureDir = await mkdtemp(join(tmpdir(), "ws-mailbox-waiter-"));
    const launcherPath = join(fixtureDir, "late-stderr.py");
    const readyPath = join(fixtureDir, "ready");
    await writeFile(launcherPath, [
      "import signal",
      "import sys",
      "import time",
      `open(${JSON.stringify(readyPath)}, \"w\").close()`,
      "def on_term(_signum, _frame):",
      "    sys.stderr.write('late after stop\\n')",
      "    sys.stderr.flush()",
      "    raise SystemExit(0)",
      "signal.signal(signal.SIGTERM, on_term)",
      "while True:",
      "    time.sleep(1)",
      "",
    ].join("\n"));
    const diagnostics: string[] = [];
    const controller = new AbortController();
    try {
      const outcome = createSubprocessWait({
        launcherPath,
        pluginDir: fixtureDir,
        sessionKey: "my-key",
        onStderr: (line) => diagnostics.push(line),
      })(controller.signal);
      await waitForFile(readyPath);
      controller.abort();
      assert.equal(await outcome, "stopped");
      await new Promise((resolve) => setTimeout(resolve, 25));
      assert.deepEqual(diagnostics, [], "a stopped subprocess must not reach its session-bound sink");
    } finally {
      await rm(fixtureDir, { recursive: true, force: true });
    }
  });

  test("a throwing diagnostic sink cannot escape a child stderr callback", async () => {
    const missingLauncher = join(process.cwd(), "__ws-mailbox-waiter-throwing-sink__.py");
    let sinkCalls = 0;
    const outcome = await createSubprocessWait({
      launcherPath: missingLauncher,
      pluginDir: process.cwd(),
      sessionKey: "my-key",
      onStderr: () => {
        sinkCalls += 1;
        throw new Error("stale extension context");
      },
    })(new AbortController().signal);
    assert.equal(outcome, "error");
    assert.ok(sinkCalls > 0, "the child reached the injected sink without letting its throw escape");
  });
});

describe("mapMailboxWaitExit", () => {
  test("our own abort is stopped regardless of how the child exited", () => {
    assert.equal(mapMailboxWaitExit(0, null, true), "stopped");
    assert.equal(mapMailboxWaitExit(130, "SIGTERM", true), "stopped");
    assert.equal(mapMailboxWaitExit(null, "SIGKILL", true), "stopped");
  });

  test("exit codes map to outcomes when we did not abort", () => {
    assert.equal(mapMailboxWaitExit(0, null, false), "mail");
    assert.equal(mapMailboxWaitExit(3, null, false), "timeout");
    assert.equal(mapMailboxWaitExit(1, null, false), "error", "any other nonzero code is an error");
  });

  test("a stray external signal we did not send re-arms via error, never a permanent stop", () => {
    assert.equal(mapMailboxWaitExit(130, null, false), "error", "the CLI's own interrupt code, unaborted, is not a stop");
    assert.equal(mapMailboxWaitExit(null, "SIGTERM", false), "error", "signal-terminated but not by our stop() -> error, so the loop re-arms with backoff");
  });
});

describe("shouldArmMailboxWaiter", () => {
  test("arms for an owner lead (no spawn role) with a session key", () => {
    assert.equal(shouldArmMailboxWaiter(undefined, "my-key"), true);
  });

  test("does not arm a fork/worker/explore child", () => {
    assert.equal(shouldArmMailboxWaiter("fork", "my-key"), false);
    assert.equal(shouldArmMailboxWaiter("worker", "my-key"), false);
    assert.equal(shouldArmMailboxWaiter("explore", "my-key"), false);
  });

  test("does not arm without a resolved session key", () => {
    assert.equal(shouldArmMailboxWaiter(undefined, undefined), false);
    assert.equal(shouldArmMailboxWaiter(undefined, ""), false);
  });
});

describe("createBridgeDrain", () => {
  test("calls the recv tool with the session key and json format and returns the parsed envelopes", async () => {
    const calls: { name: string; args: Record<string, unknown> }[] = [];
    const drain = createBridgeDrain(async (name, args) => {
      calls.push({ name, args });
      return { content: [{ type: "text", text: JSON.stringify([{ from: "x", content: "hi", sent_at: "t" }]) }] };
    }, "my-key");
    const envelopes = await drain();
    assert.deepEqual(calls, [{ name: "mailbox.recv", args: { session_key: "my-key", format: "json" } }]);
    assert.deepEqual(envelopes, [{ from: "x", content: "hi", sent_at: "t" }]);
  });

  test("throws on a tool-level error response so the loop backs off instead of hot-spinning", async () => {
    const drain = createBridgeDrain(async () => ({ isError: true, content: [{ type: "text", text: "mailbox.recv: reply-id drain: disk full" }] }), "k");
    await assert.rejects(drain(), /disk full/);
  });

  test("degrades a missing, non-JSON, or non-array response to no mail and filters malformed items", async () => {
    const empty = createBridgeDrain(async () => ({ content: [] }), "k");
    assert.deepEqual(await empty(), []);
    const garbage = createBridgeDrain(async () => ({ content: [{ type: "text", text: "not json" }] }), "k");
    assert.deepEqual(await garbage(), []);
    const object = createBridgeDrain(async () => ({ content: [{ type: "text", text: JSON.stringify({ unread: 0 }) }] }), "k");
    assert.deepEqual(await object(), []);
    const mixed = createBridgeDrain(
      async () => ({ content: [{ type: "text", text: JSON.stringify([{ content: "keep" }, { from: "no-content" }, null]) }] }),
      "k",
    );
    assert.deepEqual(await mixed(), [{ content: "keep" }]);
  });
});

describe("resolveMailboxSelfSlug", () => {
  test("calls lookup_peers with the session key and json format and returns self.address", async () => {
    const calls: { name: string; args: Record<string, unknown> }[] = [];
    const slug = await resolveMailboxSelfSlug(async (name, args) => {
      calls.push({ name, args });
      return { content: [{ type: "text", text: JSON.stringify({ self: { address: "scout@worktree", auto: true }, peers: [] }) }] };
    }, "my-key");
    assert.deepEqual(calls, [{ name: "mailbox.lookup_peers", args: { session_key: "my-key", scope: "worktree", format: "json" } }]);
    assert.equal(slug, "scout@worktree");
  });

  test("resolves to undefined when self has no address (env-less session)", async () => {
    const slug = await resolveMailboxSelfSlug(
      async () => ({ content: [{ type: "text", text: JSON.stringify({ self: { reply_id: "id:abc" }, peers: [] }) }] }),
      "k",
    );
    assert.equal(slug, undefined);
  });

  test("resolves to undefined on a tool-level error, a rejected call, or a malformed response", async () => {
    assert.equal(await resolveMailboxSelfSlug(async () => ({ isError: true, content: [{ type: "text", text: "boom" }] }), "k"), undefined);
    assert.equal(await resolveMailboxSelfSlug(async () => { throw new Error("transport down"); }, "k"), undefined);
    assert.equal(await resolveMailboxSelfSlug(async () => ({ content: [{ type: "text", text: "not json" }] }), "k"), undefined);
    assert.equal(await resolveMailboxSelfSlug(async () => ({ content: [] }), "k"), undefined);
  });

  test("resolves to undefined on a whitespace-only address, a non-object self, or a non-object top-level response", async () => {
    assert.equal(
      await resolveMailboxSelfSlug(async () => ({ content: [{ type: "text", text: JSON.stringify({ self: { address: "   " } }) }] }), "k"),
      undefined,
      "a whitespace-only address is treated the same as absent",
    );
    assert.equal(
      await resolveMailboxSelfSlug(async () => ({ content: [{ type: "text", text: JSON.stringify({ self: "scout@worktree" }) }] }), "k"),
      undefined,
      "a non-object self never reaches into a bare string for .address",
    );
    assert.equal(
      await resolveMailboxSelfSlug(async () => ({ content: [{ type: "text", text: "42" }] }), "k"),
      undefined,
      "a non-object top-level JSON value (still valid JSON) is not treated as {self}",
    );
  });
});

describe("buildMailboxWaitArgv", () => {
  test("boardInbox adds --board-inbox, and its absence leaves the argv unchanged", () => {
    const base = { launcherPath: "/l.py", pluginDir: "/p", sessionKey: "k" };
    assert.deepEqual(buildMailboxWaitArgv({ ...base, boardInbox: true }), [...buildMailboxWaitArgv(base), "--board-inbox"]);
    assert.deepEqual(buildMailboxWaitArgv({ ...base, boardInbox: false }), buildMailboxWaitArgv(base));
    assert.deepEqual(buildMailboxWaitArgv({ ...base, boardSteer: true }), [...buildMailboxWaitArgv(base), "--board-steer"]);
    assert.deepEqual(buildMailboxWaitArgv({ ...base, boardInbox: true, boardSteer: true }), [...buildMailboxWaitArgv(base), "--board-inbox"], "the inbox flag already wakes on a steered entry");
  });


  test("with no slug, the invocation is unchanged (reply-id-only, Phase 1 shape)", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key" });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "10m", "--format", "json"]);
  });

  test("a resolved self.address reaches the invocation as --slug", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key", slug: "scout@worktree" });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "10m", "--format", "json", "--slug", "scout@worktree"]);
  });

  test("an empty/whitespace slug is treated as absent", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key", slug: "   " });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "10m", "--format", "json"]);
  });

  test("a custom timeoutArg is respected alongside a slug", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key", slug: "scout@worktree", timeoutArg: "5m" });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "5m", "--format", "json", "--slug", "scout@worktree"]);
  });

  test("a root option reaches the invocation as --root", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key", slug: "scout@worktree", root: "/work/tree" });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "10m", "--format", "json", "--root", "/work/tree", "--slug", "scout@worktree"]);
  });

  test("an empty/whitespace root is treated as absent", () => {
    const argv = buildMailboxWaitArgv({ launcherPath: "/l", pluginDir: "/p", sessionKey: "my-key", root: "  " });
    assert.deepEqual(argv, ["/l", "mailbox", "wait", "--session-key", "my-key", "--timeout", "10m", "--format", "json"]);
  });
});

describe("sessionMailboxWaitOptions", () => {
  test("the session cwd becomes the wait's --root, not the launcher dir", () => {
    const onStderr = (): void => {};
    const options = sessionMailboxWaitOptions({ launcherPath: "/plug/bin/l.py", pluginDir: "/plug", runtimeBinary: "/local/ws-mcp", sessionKey: "k", slug: "scout@worktree", cwd: "/work/tree", onStderr });
    assert.deepEqual(options, { launcherPath: "/plug/bin/l.py", pluginDir: "/plug", runtimeBinary: "/local/ws-mcp", sessionKey: "k", slug: "scout@worktree", root: "/work/tree", onStderr });
    assert.deepEqual(buildMailboxWaitArgv(options).slice(-4), ["--root", "/work/tree", "--slug", "scout@worktree"]);
  });

  test("a reply-id-only session wait still carries the session root", () => {
    const options = sessionMailboxWaitOptions({ launcherPath: "/l", pluginDir: "/p", sessionKey: "k", slug: undefined, cwd: "/work/tree" });
    assert.equal(options.runtimeBinary, undefined, "release-backed sessions use the launcher");
    assert.deepEqual(buildMailboxWaitArgv(options), ["/l", "mailbox", "wait", "--session-key", "k", "--timeout", "10m", "--format", "json", "--root", "/work/tree"]);
  });
});

/** A controllable stand-in for one started waiter: `stop()` is observable and `done` settles on `close()`. */
function fakeWaiter(sessionKey: string, slug: string | undefined): MailboxWaiterHandle & { sessionKey: string; slug: string | undefined; stopped: boolean; close: () => void } {
  let close: () => void = () => {};
  const done = new Promise<void>((resolve) => { close = resolve; });
  const waiter = { sessionKey, slug, stopped: false, done, close: () => close(), stop: () => { waiter.stopped = true; } };
  return waiter;
}

/** A slug resolver whose lookups stay pending until the test releases them, in any order. */
function deferredSlugs(): { resolveSlug: (key: string) => Promise<string | undefined>; release: (key: string, slug?: string) => void; pending: () => string[] } {
  const waiting = new Map<string, (slug: string | undefined) => void>();
  return {
    resolveSlug: (key) => new Promise((resolve) => { waiting.set(key, resolve); }),
    release: (key, slug) => { const resolve = waiting.get(key); assert.ok(resolve, `no lookup pending for ${key}`); waiting.delete(key); resolve(slug); },
    pending: () => [...waiting.keys()],
  };
}

describe("createMailboxWaiterSlot", () => {
  test("re-arming with a new key starts a waiter on that key and its newly resolved slug, then stops the old one", async () => {
    const started: ReturnType<typeof fakeWaiter>[] = [];
    const lookups: string[] = [];
    const slot = createMailboxWaiterSlot({
      resolveSlug: async (key) => { lookups.push(key); return `exec-${key}@worktree`; },
      start: (key, slug) => { const waiter = fakeWaiter(key, slug); started.push(waiter); return waiter; },
    });
    assert.equal(await slot.arm("old-key"), true);
    assert.equal(await slot.arm("new-key"), true);
    assert.deepEqual(lookups, ["old-key", "new-key"], "the slug is re-resolved for the new key");
    assert.deepEqual(started.map((waiter) => [waiter.sessionKey, waiter.slug]), [["old-key", "exec-old-key@worktree"], ["new-key", "exec-new-key@worktree"]]);
    assert.equal(started[0]!.stopped, true, "the old key's waiter is stopped");
    assert.equal(started[1]!.stopped, false, "the new key's waiter stays live");
    slot.stop();
    assert.equal(started[1]!.stopped, true);
    for (const waiter of started) waiter.close();
    await slot.done;
  });

  test("the old waiter keeps running until its successor starts", async () => {
    const slugs = deferredSlugs();
    const started: ReturnType<typeof fakeWaiter>[] = [];
    const slot = createMailboxWaiterSlot({ resolveSlug: slugs.resolveSlug, start: (key, slug) => { const waiter = fakeWaiter(key, slug); started.push(waiter); return waiter; } });
    const first = slot.arm("old-key");
    slugs.release("old-key");
    await first;
    const second = slot.arm("new-key");
    assert.equal(started[0]!.stopped, false, "no gap while the new key's slug resolves");
    slugs.release("new-key", "exec@worktree");
    await second;
    assert.equal(started[0]!.stopped, true);
    slot.stop();
  });

  test("an arm superseded by a newer arm while its lookup is pending starts nothing, in either completion order", async () => {
    const slugs = deferredSlugs();
    const started: ReturnType<typeof fakeWaiter>[] = [];
    const slot = createMailboxWaiterSlot({ resolveSlug: slugs.resolveSlug, start: (key, slug) => { const waiter = fakeWaiter(key, slug); started.push(waiter); return waiter; } });
    const stale = slot.arm("key-a");
    const fresh = slot.arm("key-b");
    slugs.release("key-b");
    assert.equal(await fresh, true);
    slugs.release("key-a");
    assert.equal(await stale, false);
    assert.deepEqual(started.map((waiter) => waiter.sessionKey), ["key-b"]);

    const slugs2 = deferredSlugs();
    const started2: ReturnType<typeof fakeWaiter>[] = [];
    const slot2 = createMailboxWaiterSlot({ resolveSlug: slugs2.resolveSlug, start: (key, slug) => { const waiter = fakeWaiter(key, slug); started2.push(waiter); return waiter; } });
    const stale2 = slot2.arm("key-a");
    const fresh2 = slot2.arm("key-b");
    slugs2.release("key-a");
    assert.equal(await stale2, false);
    slugs2.release("key-b");
    assert.equal(await fresh2, true);
    assert.deepEqual(started2.map((waiter) => waiter.sessionKey), ["key-b"]);
    slot.stop();
    slot2.stop();
  });

  test("an arm superseded by stop() (a newer session_start or a shutdown) leaves no live waiter from the stale generation", async () => {
    const slugs = deferredSlugs();
    const started: ReturnType<typeof fakeWaiter>[] = [];
    const slot = createMailboxWaiterSlot({ resolveSlug: slugs.resolveSlug, start: (key, slug) => { const waiter = fakeWaiter(key, slug); started.push(waiter); return waiter; } });
    const first = slot.arm("old-key");
    slugs.release("old-key");
    await first;
    const rearm = slot.arm("new-key");
    slot.stop();
    assert.equal(started[0]!.stopped, true, "disposal stops the live waiter");
    slugs.release("new-key", "exec@worktree");
    assert.equal(await rearm, false);
    assert.equal(await slot.arm("later-key"), false, "a stopped slot refuses every later arm");
    assert.deepEqual(started.map((waiter) => waiter.sessionKey), ["old-key"], "the pending re-arm never started a waiter");
  });

  test("done resolves only after stop() and after every waiter the slot started has exited", async () => {
    const started: ReturnType<typeof fakeWaiter>[] = [];
    const slot = createMailboxWaiterSlot({ resolveSlug: async () => undefined, start: (key, slug) => { const waiter = fakeWaiter(key, slug); started.push(waiter); return waiter; } });
    await slot.arm("old-key");
    await slot.arm("new-key");
    let settled = false;
    void slot.done.then(() => { settled = true; });
    started[0]!.close();
    started[1]!.close();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(settled, false, "a live slot is not done even when its waiters have closed");
    slot.stop();
    await slot.done;
    assert.equal(settled, true);

    const replaced: ReturnType<typeof fakeWaiter>[] = [];
    const lingering = createMailboxWaiterSlot({ resolveSlug: async () => undefined, start: (key, slug) => { const waiter = fakeWaiter(key, slug); replaced.push(waiter); return waiter; } });
    await lingering.arm("old-key");
    await lingering.arm("new-key");
    lingering.stop();
    replaced[1]!.close();
    let lingeringDone = false;
    void lingering.done.then(() => { lingeringDone = true; });
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(lingeringDone, false, "a replaced waiter still closing keeps the slot (and its staged runtime) alive");
    replaced[0]!.close();
    await lingering.done;
  });

  test("a slot that never armed is done once stopped, so a staged runtime is still cleaned up", async () => {
    const slot = createMailboxWaiterSlot({ resolveSlug: async () => undefined, start: () => { throw new Error("must not start"); } });
    let cleaned = 0;
    attachMailboxRuntimeCleanup(slot, { binaryPath: "unused", cleanup: () => { cleaned += 1; } });
    slot.stop();
    await slot.done;
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(cleaned, 1);
  });

  test("leadSettled reaches only the live waiter", async () => {
    const settled: string[] = [];
    const slot = createMailboxWaiterSlot({
      resolveSlug: async () => undefined,
      start: (key) => ({ stop: () => {}, leadSettled: () => settled.push(key), done: Promise.resolve() }),
    });
    slot.leadSettled!();
    await slot.arm("old-key");
    await slot.arm("new-key");
    slot.leadSettled!();
    assert.deepEqual(settled, ["new-key"]);
    slot.stop();
    await slot.done;
  });

  test("real waiters: a re-arm stops the old loop and the new loop admits mail for the new key", async () => {
    const drains: string[] = [];
    const admitted: MailboxEnvelope[] = [];
    const slot = createMailboxWaiterSlot({
      resolveSlug: async () => undefined,
      start: (key) => startMailboxWaiter({
        runWait: scriptedWait(key === "new-key" ? ["mail"] : []).runWait,
        drainMail: async () => { drains.push(key); return [{ reply_to: key, content: `for ${key}` }]; },
        admit: (drained) => admitted.push(...drained),
        sleep: immediateSleep,
      }),
    });
    await slot.arm("old-key");
    await slot.arm("new-key");
    for (let i = 0; i < 20 && admitted.length === 0; i += 1) await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(drains, ["new-key"]);
    assert.deepEqual(admitted.map((envelope) => envelope.content), ["for new-key"]);
    slot.stop();
    await slot.done;
  });
});

describe("reportDistinctLinesOnce", () => {
  test("repeated identical lines reach the sink once; distinct lines each reach it once", () => {
    const seen: string[] = [];
    const report = reportDistinctLinesOnce((line) => seen.push(line));
    const warning = "warning: named inbox exec@worktree is not currently owned by this --session-key; falling back to a reply-id-only wait";
    report(warning);
    report(warning);
    report("other diagnostic");
    report(warning);
    assert.deepEqual(seen, [warning, "other diagnostic"]);
  });
});
