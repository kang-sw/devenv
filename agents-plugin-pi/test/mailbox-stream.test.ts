import { describe, test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter, getEventListeners } from "node:events";
import { PassThrough } from "node:stream";
import { spawn } from "node:child_process";
import {
  createDrainLedger,
  createMailboxWaiterSlot,
  createSubprocessWait,
  sessionMailboxWaitOptions,
  startMailboxWaiter,
  type MailboxWaitArm,
} from "../src/mailbox-waiter.ts";

const tick = (): Promise<void> => new Promise((resolve) => setImmediate(resolve));
const options = { launcherPath: "/plugin/launcher.py", pluginDir: "/plugin", sessionKey: "key", root: "/repo", slug: "self@worktree", timeoutArg: "1ms" };
const mailOnly = { boardInbox: false };

/** Pipe/event fakes preserve Node's exit-before-close ordering. */
class WaitChild extends EventEmitter {
  stdout = new PassThrough();
  stderr = new PassThrough();
  exitCode: number | null = null;
  signalCode: string | null = null;
  kills: string[] = [];
  closed = false;
  readonly argv: string[];
  constructor(argv: string[]) { super(); this.argv = argv; }
  arrival(kinds: string[]): void { this.stdout.write(JSON.stringify({ timed_out: false, kinds }) + "\n"); }
  exit(code: number | null, signal: string | null = null): void {
    this.exitCode = code;
    this.signalCode = signal;
    this.emit("exit", code, signal);
    this.stdout.end();
    this.stderr.end();
    setImmediate(() => { this.closed = true; this.emit("close", code, signal); });
  }
  kill(signal: string): boolean {
    this.kills.push(signal);
    queueMicrotask(() => this.exit(null, signal));
    return true;
  }
}

function subprocesses() {
  const children: WaitChild[] = [];
  const spawnChild = ((_command: string, argv: string[]) => {
    const child = new WaitChild(argv);
    children.push(child);
    return child;
  }) as unknown as typeof spawn;
  return { children, spawnChild };
}

describe("streaming mailbox subprocess", () => {
  test("a real Node child stays alive across two split arrival lines and closes on abort", { timeout: 5_000 }, async (t) => {
    let launches = 0;
    let closed = false;
    const spawnChild = ((_command: string, argv: string[], spawnOptions: object) => {
      launches += 1;
      const child = spawn(process.execPath, ["-e", `
        process.stdout.write('{"kinds":');
        setTimeout(() => process.stdout.write('["mail"]}\\n{"kinds":["board"]}\\n'), 10);
        setInterval(() => {}, 1000);
      `, "--", ...argv.slice(1)], spawnOptions);
      child.once("close", () => { closed = true; });
      return child;
    }) as typeof spawn;
    const wait = createSubprocessWait({ ...options, pluginDir: process.cwd() }, spawnChild);
    t.after(() => wait.dispose?.());
    const arm = { boardInbox: true };
    assert.equal(await wait(new AbortController().signal, arm), "mail");
    const controller = new AbortController();
    assert.equal(await wait(controller.signal, arm), "board");
    assert.equal(launches, 1);
    assert.equal(closed, false);
    const blocked = wait(controller.signal, arm);
    controller.abort();
    assert.equal(await blocked, "stopped");
    assert.equal(closed, true);
  });

  test("queues split/CRLF and multiple arrival lines across fresh arm signals without relaunching", async (t) => {
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const first = new AbortController();
    const arm = { boardInbox: true, boardInterrupt: true };
    const wake = wait(first.signal, arm);
    const child = children[0]!;
    assert.deepEqual(child.argv, [options.launcherPath, "mailbox", "wait", "--session-key", "key", "--timeout", "1ms", "--format", "json", "--root", "/repo", "--slug", "self@worktree", "--board-inbox", "--board-interrupt", "--stream"]);
    child.stdout.write('{"timed_out":false,"ki');
    await tick();
    child.stdout.write('nds":["board"]}\r\n{"kinds":["interrupt","mail"]}\n{"kinds":["mail","board"]}\n');
    assert.equal(await wake, "board");
    const second = new AbortController();
    assert.deepEqual(await wait(second.signal, arm), { interrupt: true, also: "mail" });
    assert.equal(getEventListeners(first.signal, "abort").length, 0, "abort ownership transfers to the current arm");
    first.abort();
    assert.deepEqual(child.kills, []);
    const third = new AbortController();
    assert.equal(await wait(third.signal, arm), "mail+board");
    assert.equal(children.length, 1);
    const blocked = wait(third.signal, arm);
    third.abort();
    assert.equal(await blocked, "stopped");
    assert.equal(child.closed, true, "abort resolves only after child close");
    assert.equal(getEventListeners(third.signal, "abort").length, 0);
  });

  test("silent timeout windows do not finish the wait or spawn another child", async (t) => {
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const controller = new AbortController();
    let finished = false;
    const wake = wait(controller.signal).then((outcome) => { finished = true; return outcome; });
    // The real stream emits nothing on timeout; tolerate a legacy timeout
    // record too, without treating it (or non-JSON output) as an arrival.
    children[0]!.stdout.write('not an arrival\n{"timed_out":true}\n{"timed_out":false,"kinds":[]}\n{"timed_out":false}\n');
    await new Promise((resolve) => setTimeout(resolve, 20));
    assert.equal(finished, false);
    assert.equal(children.length, 1);
    children[0]!.arrival(["mail"]);
    assert.equal(await wake, "mail");
    await wait.dispose?.();
    assert.equal(children[0]!.closed, true);
    assert.equal(await wait(new AbortController().signal), "stopped");
  });

  test("idle config rechecks return timeout without replacing the stream", async (t) => {
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const wake = wait(new AbortController().signal);
    t.mock.timers.tick(10 * 60 * 1_000);
    assert.equal(await wake, "timeout");
    const again = wait(new AbortController().signal);
    children[0]!.arrival(["mail"]);
    assert.equal(await again, "mail");
    assert.equal(children.length, 1);
  });

  test("each effective board flag change replaces the child and drops old queued wakes", async (t) => {
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const modes: MailboxWaitArm[] = [
      { boardInbox: true, boardInterrupt: true },
      { boardInbox: false, boardSteer: true, boardInterrupt: true },
      { boardInbox: false, boardInterrupt: true },
      { boardInbox: false },
    ];
    for (const [index, arm] of modes.entries()) {
      const wake = wait(new AbortController().signal, arm);
      await tick();
      await tick();
      const child = children[index]!;
      if (index > 0) assert.equal(children[index - 1]!.closed, true);
      assert.equal(child.argv.includes("--board-inbox"), arm.boardInbox);
      assert.equal(child.argv.includes("--board-steer"), arm.boardSteer === true);
      assert.equal(child.argv.includes("--board-interrupt"), arm.boardInterrupt === true);
      child.arrival(["mail"]);
      child.arrival(["board"]); // Stale after the next mode change.
      assert.equal(await wake, "mail");
    }
    assert.equal(children.length, 4);
  });

  for (const rejection of ["unknown flag: --stream\n", "Usage: mailbox wait\n", ""]) {
    test(`falls back for the remaining factory lifetime on ${rejection.trim() || "nonzero before any arrival"}`, async (t) => {
      const { children, spawnChild } = subprocesses();
      const wait = createSubprocessWait(options, spawnChild);
      t.after(() => wait.dispose?.());
      const wake = wait(new AbortController().signal, mailOnly);
      // A usage diagnostic may be split across chunks, and even exit zero.
      children[0]!.stderr.write(rejection.slice(0, 3));
      children[0]!.stderr.write(rejection.slice(3));
      children[0]!.exit(rejection ? 0 : 1);
      await tick();
      const legacy = children[1]!;
      assert.equal(legacy.argv.includes("--stream"), false);
      legacy.exit(3);
      assert.equal(await wake, "timeout");
      const again = wait(new AbortController().signal, { boardInbox: true });
      assert.equal(children[2]!.argv.includes("--stream"), false);
      children[2]!.arrival(["board"]);
      children[2]!.exit(0);
      assert.equal(await again, "board");
    });
  }

  test("fallback survives factory recreation after a session-key rebind", async (t) => {
    const { children, spawnChild } = subprocesses();
    const streamSupport = {};
    const wait = createSubprocessWait(sessionMailboxWaitOptions({ ...options, cwd: "/repo", streamSupport }), spawnChild);
    t.after(() => wait.dispose?.());
    const wake = wait(new AbortController().signal);
    children[0]!.exit(1);
    await tick();
    assert.equal(children[1]!.argv.includes("--stream"), false);
    children[1]!.exit(3);
    assert.equal(await wake, "timeout");
    await wait.dispose?.();
    const rebound = createSubprocessWait(sessionMailboxWaitOptions({ ...options, sessionKey: "revived", cwd: "/repo", streamSupport }), spawnChild);
    t.after(() => rebound.dispose?.());
    const next = rebound(new AbortController().signal);
    await tick();
    assert.equal(children[2]!.argv.includes("--stream"), false, "the new factory reuses the session's fallback decision");
    assert.ok(children[2]!.argv.includes("revived"));
    children[2]!.exit(3);
    assert.equal(await next, "timeout");
  });

  test("non-arrival usage on stdout still triggers compatibility fallback", async (t) => {
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const wake = wait(new AbortController().signal);
    children[0]!.stdout.write("Usage: mailbox wait\n");
    children[0]!.exit(0);
    await tick();
    assert.equal(children[1]!.argv.includes("--stream"), false);
    children[1]!.exit(3);
    assert.equal(await wake, "timeout");
  });

  test("a crash after arrivals reports error then retries streaming, including queued arrivals before EOF", async (t) => {
    const { children, spawnChild } = subprocesses();
    const wait = createSubprocessWait(options, spawnChild);
    t.after(() => wait.dispose?.());
    const signal = new AbortController().signal;
    const wake = wait(signal, { boardInbox: true });
    children[0]!.stdout.write(JSON.stringify({ timed_out: false, kinds: ["mail"], reply: [{ content: "unknown flag: --stream; Usage: mailbox wait" }] }) + "\n");
    children[0]!.arrival(["board"]);
    children[0]!.exit(1);
    assert.equal(await wake, "mail");
    assert.equal(await wait(signal, { boardInbox: true }), "board");
    assert.equal(await wait(signal, { boardInbox: true }), "error");
    const retry = wait(signal);
    assert.equal(children[1]!.argv.includes("--stream"), true);
    children[1]!.arrival(["mail"]);
    assert.equal(await retry, "mail");
  });

  test("unexpected clean EOF or an external signal is an error, not a permanent stop or skew fallback", async (t) => {
    for (const [code, signal] of [[0, null], [null, "SIGTERM"]] as const) {
      const { children, spawnChild } = subprocesses();
      const wait = createSubprocessWait(options, spawnChild);
      t.after(() => wait.dispose?.());
      const wake = wait(new AbortController().signal);
      children[0]!.exit(code, signal);
      assert.equal(await wake, "error");
      const retry = wait(new AbortController().signal);
      assert.equal(children[1]!.argv.includes("--stream"), true);
      children[1]!.arrival(["mail"]);
      assert.equal(await retry, "mail");
    }
  });

  test("stop during a drain aborts the cached child and done awaits its close", async () => {
    const { children, spawnChild } = subprocesses();
    let finishDrain!: () => void;
    let draining = false;
    const waiter = startMailboxWaiter({
      runWait: createSubprocessWait(options, spawnChild),
      drainMail: async () => { draining = true; await new Promise<void>((resolve) => { finishDrain = resolve; }); return []; },
      admit: () => assert.fail("stopped"),
    });
    children[0]!.arrival(["mail"]);
    await tick();
    assert.equal(draining, true);
    waiter.stop();
    assert.deepEqual(children[0]!.kills, ["SIGTERM"]);
    finishDrain();
    await waiter.done;
    assert.equal(children[0]!.closed, true);
    assert.equal(children.length, 1);
  });

  test("leadSettled aborts the held-board arm and restores inbox mode without ending the loop", async () => {
    const { children, spawnChild } = subprocesses();
    let idle = false;
    const texts: string[][] = [];
    const waiter = startMailboxWaiter({
      runWait: createSubprocessWait(options, spawnChild),
      drainMail: async () => [],
      admit: () => {},
      board: {
        enabled: () => true,
        idle: () => idle,
        drain: async () => ["held"],
        inject: (messages) => { texts.push(messages); },
        steer: { steerable: () => !idle, drain: async () => [], inject: () => {} },
      },
    });
    try {
      await tick();
      children[0]!.arrival(["board"]);
      await tick();
      await tick();
      assert.ok(children[1]!.argv.includes("--board-steer"));
      assert.equal(children[0]!.closed, true);
      idle = true;
      waiter.leadSettled?.();
      await tick();
      await tick();
      assert.deepEqual(texts, [["held"]]);
      assert.ok(children[2]!.argv.includes("--board-inbox"));
      assert.equal(children[1]!.closed, true);
    } finally {
      waiter.stop();
      await waiter.done;
    }
  });

  test("rebind and slot disposal stop every session's cached stream", async () => {
    const { children, spawnChild } = subprocesses();
    const slot = createMailboxWaiterSlot({
      resolveSlug: async (key) => `${key}@worktree`,
      start: (sessionKey, slug) => startMailboxWaiter({
        runWait: createSubprocessWait({ ...options, sessionKey, slug }, spawnChild),
        drainMail: async () => [],
        admit: () => {},
      }),
    });
    await slot.arm("first");
    children[0]!.arrival(["mail"]);
    await tick();
    await slot.arm("second");
    await tick();
    assert.equal(children[0]!.closed, true);
    assert.ok(children[1]!.argv.includes("second@worktree"));
    slot.stop();
    await slot.done;
    assert.ok(children.every((child) => child.closed));
  });

  test("a buffered arrival the drain already received does not wake; mail after the drain still does", async () => {
    const { children, spawnChild } = subprocesses();
    const m1 = { from: "alice", content: "one", sent_at: "2026-11-13T10:00:00Z" };
    const m2 = { from: "bob", content: "two", sent_at: "2026-11-13T10:00:01Z" };
    const m3 = { from: "carol", content: "three", sent_at: "2026-11-13T10:00:05Z" };
    const line = (named: object[]): string => JSON.stringify({ timed_out: false, kinds: ["mail"], named, reply: [] }) + "\n";
    let queued = [m1];
    let drains = 0;
    const admitted: string[][] = [];
    let release!: () => void;
    const drainStarted = new Promise<void>((resolve) => { release = resolve; });
    const waiter = startMailboxWaiter({
      runWait: createSubprocessWait(options, spawnChild),
      drainMail: async () => {
        drains += 1;
        if (drains === 1) {
          // m2 lands before recv runs: its line is buffered behind this wake.
          queued.push(m2);
          children[0]!.stdout.write(line([m2]));
          release();
          await tick();
        }
        const mail = queued;
        queued = [];
        return mail;
      },
      admit: (envelopes) => { admitted.push(envelopes.map((e) => e.content)); },
    });
    try {
      await tick();
      children[0]!.stdout.write(line([m1]));
      await drainStarted;
      for (let i = 0; i < 5; i++) await tick();
      assert.deepEqual(admitted, [["one", "two"]]);
      assert.equal(drains, 1, "the buffered m2 line must not wake another drain");
      // Mail after the drain and before the re-arm reads it wakes promptly.
      queued = [m3];
      children[0]!.stdout.write(line([m3]));
      for (let i = 0; i < 5; i++) await tick();
      assert.equal(drains, 2);
      assert.deepEqual(admitted, [["one", "two"], ["three"]]);
      assert.equal(children.length, 1);
    } finally {
      waiter.stop();
      await waiter.done;
    }
  });
});

test("the drain ledger skips the line that woke the drain and clears on mail the drain did not get", () => {
  const m1 = { from: "alice", content: "one", sent_at: "2026-11-13T10:00:00Z" };
  const again = { ...m1, from: "bob" }; // same second, same text, sent after the drain
  const m3 = { from: "carol", content: "three", sent_at: "2026-11-13T10:00:05Z" };
  const line = (named: object[]): string => JSON.stringify({ timed_out: false, kinds: ["mail"], named, reply: [] }) + "\n";
  let ledger = createDrainLedger();
  assert.equal(ledger.filter(line([m1])), line([m1]), "wakes the drain");
  ledger.record([m1]);
  assert.equal(ledger.filter(line([again])), line([again]), "an identical later message must still wake");
  ledger = createDrainLedger();
  ledger.record([m1]); // received without a line, e.g. before this child started
  assert.equal(ledger.filter(line([m3])), line([m3]));
  assert.equal(ledger.filter(line([m1])), line([m1]), "mail the drain did not get clears the ledger");
});
