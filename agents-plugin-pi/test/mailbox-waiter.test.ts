/**
 * Unit tests for mailbox-waiter.ts: the session-bound wait/drain/admit loop
 * (260914 pi native mailbox push), driven entirely through injected fakes so
 * the arrival -> push detection path is exercised without a live mailbox or a
 * real `mailbox wait` subprocess.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { access, mkdtemp, rm, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import {
  buildMailboxPushMessage,
  buildMailboxWaitArgv,
  createBridgeDrain,
  createSubprocessWait,
  mapMailboxWaitExit,
  resolveMailboxSelfSlug,
  sessionMailboxWaitOptions,
  shouldArmMailboxWaiter,
  startMailboxWaiter,
  WS_MAILBOX_CUSTOM_TYPE,
  type MailboxEnvelope,
  type MailboxWaitOutcome,
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
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      await access(path);
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 5));
    }
  }
  throw new Error(`timed out waiting for ${path}`);
}

describe("startMailboxWaiter", () => {
  test("a mail wake drains and admits every drained envelope in order", async () => {
    const envelopes: MailboxEnvelope[] = [
      { from: "alice@machine", content: "first", sent_at: "2026-09-15T10:00:00Z" },
      { reply_to: "id:abc", content: "second", sent_at: "2026-09-15T10:00:01Z" },
    ];
    const admitted: MailboxEnvelope[] = [];
    let drainCalls = 0;
    const waiter = startMailboxWaiter({
      ...scriptedWait(["mail", "stopped"]),
      drainMail: () => { drainCalls += 1; return Promise.resolve(envelopes); },
      admit: (envelope) => admitted.push(envelope),
      sleep: immediateSleep,
    });
    await waiter.done;
    assert.equal(drainCalls, 1, "one wake drains exactly once");
    assert.deepEqual(admitted, envelopes, "each drained envelope is admitted in arrival order");
  });

  test("a timeout re-arms without draining or admitting", async () => {
    const admitted: MailboxEnvelope[] = [];
    let drainCalls = 0;
    const script = scriptedWait(["timeout", "stopped"]);
    const waiter = startMailboxWaiter({
      ...script,
      drainMail: () => { drainCalls += 1; return Promise.resolve([]); },
      admit: (envelope) => admitted.push(envelope),
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
      admit: (envelope) => admitted.push(envelope),
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
      admit: (envelope) => admitted.push(envelope),
      sleep: (ms) => { sleeps.push(ms); return Promise.resolve(); },
    });
    await waiter.done;
    assert.equal(admitted.length, 0, "an empty drain admits nothing");
    assert.equal(sleeps.length, 0, "an empty drain is a success, not a failure — no backoff");
    assert.equal(script.calls(), 2, "the loop re-armed immediately after the empty drain");
  });

  test("one throwing admit is isolated — the rest of the batch still lands", async () => {
    const errors: string[] = [];
    const admitted: string[] = [];
    const envelopes: MailboxEnvelope[] = [
      { from: "a", content: "boom" },
      { from: "b", content: "ok" },
    ];
    const waiter = startMailboxWaiter({
      ...scriptedWait(["mail", "stopped"]),
      drainMail: () => Promise.resolve(envelopes),
      admit: (envelope) => {
        if (envelope.content === "boom") throw new Error("send rejected");
        admitted.push(envelope.content);
      },
      sleep: immediateSleep,
      onError: (message) => errors.push(message),
    });
    await waiter.done;
    assert.deepEqual(admitted, ["ok"], "a later envelope is admitted even after an earlier one throws");
    assert.ok(errors.some((message) => message.includes("admit failed")));
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
    const message = buildMailboxPushMessage({ reply_to: "id:deadbeef", content: "ack" });
    assert.equal(message.content, "mail from id:deadbeef:\nack");
    assert.deepEqual(message.details, { reply_to: "id:deadbeef", content: "ack" });
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

describe("createSubprocessWait", () => {
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
    const options = sessionMailboxWaitOptions({ launcherPath: "/plug/bin/l.py", pluginDir: "/plug", sessionKey: "k", slug: "scout@worktree", cwd: "/work/tree", onStderr });
    assert.deepEqual(options, { launcherPath: "/plug/bin/l.py", pluginDir: "/plug", sessionKey: "k", slug: "scout@worktree", root: "/work/tree", onStderr });
    assert.deepEqual(buildMailboxWaitArgv(options).slice(-4), ["--root", "/work/tree", "--slug", "scout@worktree"]);
  });
});
