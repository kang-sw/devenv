/**
 * 261016: background mode of the lead's one-liner exec hatch — the task
 * registry's lifecycle (`background-tasks.ts`), the hatch's background
 * branch and `ws-background-abort` (`execute-gateway.ts`), the session-end
 * kill, the extended fan-in status line, and the readable task id.
 *
 * These spawn real processes (POSIX shell; skipped on native Windows) and
 * kill every leftover task after each test.
 */

import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  abortBackgroundTask,
  countBackgroundTasks,
  formatElapsed,
  hasRunningTimedBackgroundTask,
  killAllBackgroundTasks,
  listBackgroundTasks,
  registerBackgroundTaskShutdown,
  startBackgroundTask,
  type BackgroundTaskResult,
} from "../src/background-tasks.ts";
import {
  BACKGROUND_ABORT_TOOL_NAME,
  BACKGROUND_TASK_PUSH_FAMILY,
  ONE_LINER_EXEC_TOOL_NAME,
  ONE_LINER_OUTPUT_CAP_BYTES,
  backgroundTaskReportFields,
  registerExecuteGateway,
  validateOneLinerMode,
} from "../src/execute-gateway.ts";
import { computeRunningStatusLine, heldPushQueue, leadIdleRef, type RpcAgentRegistry } from "../src/spawner.ts";
import { buildPushRenderLines, STATUS_LINE_PATTERN } from "../src/push-render.ts";
import { generateWordChainId, wordChainPool } from "../src/word-chain-id.ts";

const posixOnly = { skip: process.platform === "win32" ? "POSIX process groups" : false };
const WORD_CHAIN = /^[a-z]+-[a-z]+-[a-z]+$/;

type Tool = { name: string; description: string; parameters: { properties: Record<string, { description?: string }> }; execute: (id: string, params: unknown, signal?: AbortSignal) => Promise<{ content: Array<{ text: string }> }> };
type Sent = { message: { customType: string; content: string; details: Record<string, unknown> }; options: { deliverAs: string } };

function gateway(options: { isFork?: boolean; registry?: RpcAgentRegistry } = {}) {
  const tools = new Map<string, Tool>();
  const sent: Sent[] = [];
  const pi = {
    registerTool: (def: Tool) => tools.set(def.name, def),
    sendMessage: (message: Sent["message"], opts: Sent["options"]) => sent.push({ message, options: opts }),
    sendUserMessage: () => {},
    exec: async () => { throw new Error("foreground exec is not expected here"); },
  } as unknown as ExtensionAPI;
  registerExecuteGateway(pi, {} as never, options.registry ?? new Map(), {
    cwd: tmpdir(),
    executeWorkerPromptPath: "/tmp/fake-execute-worker-guide.md",
    extensionPath: "/tmp/fake-extension.ts",
    isFork: () => options.isFork ?? false,
  });
  // A busy lead with an empty held queue: a steer push is sent at once.
  leadIdleRef.current = () => false;
  return { hatch: tools.get(ONE_LINER_EXEC_TOOL_NAME)!, abort: tools.get(BACKGROUND_ABORT_TOOL_NAME)!, sent };
}

function text(result: { content: Array<{ text: string }> }): string {
  return result.content.map((part) => part.text).join("\n");
}

function taskIdOf(started: string): string {
  const match = /background task started: ([a-z-]+)/.exec(started);
  assert.ok(match, `a task id in: ${started}`);
  return match[1];
}

async function waitFor(predicate: () => boolean, timeoutMs = 5000): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  while (!predicate()) {
    if (Date.now() > deadline) throw new Error("timed out waiting for condition");
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
}

function processAlive(pattern: string): boolean {
  try {
    execFileSync("pgrep", ["-f", pattern], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

/** A unique `tail -f <file> | grep -m1 <never>` pipeline: the shape whose `tail` outlives a direct-child-only kill. */
function pipelineMonitor(): { command: string; tailPattern: string; cleanup: () => void } {
  const dir = mkdtempSync(join(tmpdir(), "ws-bg-pipeline-"));
  const file = join(dir, "watched.log");
  writeFileSync(file, "");
  return { command: `tail -f ${file} | grep -m1 NEVER_MATCHES`, tailPattern: `tail -f ${file}`, cleanup: () => rmSync(dir, { recursive: true, force: true }) };
}

afterEach(() => {
  killAllBackgroundTasks();
  leadIdleRef.current = undefined;
  heldPushQueue.length = 0;
});

describe("readable task id (word-chain port)", () => {
  test("is three lowercase EFF words joined by '-'", () => {
    for (let index = 0; index < 20; index += 1) assert.match(generateWordChainId(), WORD_CHAIN);
  });

  test("the bundled wordlist is byte-identical to the ws tooling's copy", () => {
    const here = dirname(fileURLToPath(import.meta.url));
    const goCopy = join(here, "../../agents-plugin-tool/internal/wskey/eff_large_wordlist.txt");
    let canonical: Buffer;
    try {
      canonical = readFileSync(goCopy);
    } catch {
      return; // outside the source monorepo
    }
    assert.ok(canonical.equals(readFileSync(join(here, "../src/eff_large_wordlist.txt"))));
    assert.equal(wordChainPool().length, 7772);
  });

  test("a collision with a live task regenerates the id", posixOnly, () => {
    const ids = ["same-same-same", "same-same-same", "other-other-other"];
    const start = () => startBackgroundTask({ shell: "/bin/sh", args: ["-c", "sleep 30"], cwd: tmpdir(), command: "sleep 30", why: "w", capBytes: 64, onComplete: () => {}, generateId: () => ids.shift()! });
    assert.equal(start().id, "same-same-same");
    assert.equal(start().id, "other-other-other");
  });
});

describe("validateOneLinerMode", () => {
  test("rejects timeout_s in foreground", () => {
    const result = validateOneLinerMode({ timeout_s: 10 }, false);
    assert.equal(result.ok, false);
    assert.match((result as { error: string }).error, /only with background: true/);
  });

  test("rejects a non-positive or non-integer timeout_s", () => {
    for (const timeout_s of [0, -5, 1.5, "10"]) {
      assert.equal(validateOneLinerMode({ background: true, timeout_s }, false).ok, false, `timeout_s ${String(timeout_s)}`);
    }
  });

  test("rejects background mode in a fork", () => {
    assert.match((validateOneLinerMode({ background: true }, true) as { error: string }).error, /lead only/);
  });

  test("accepts foreground, a monitor, and a timed task", () => {
    assert.deepEqual(validateOneLinerMode({}, false), { ok: true, background: false });
    assert.deepEqual(validateOneLinerMode({ background: true }, false), { ok: true, background: true });
    assert.deepEqual(validateOneLinerMode({ background: true, timeout_s: 60 }, false), { ok: true, background: true, timeoutS: 60 });
  });
});

describe("hatch schema", () => {
  test("carries background and timeout_s, with the one-line goal-loop rule and the monitor self-termination rule", () => {
    const { hatch } = gateway();
    assert.ok(hatch.parameters.properties.background);
    assert.match(hatch.parameters.properties.timeout_s.description!, /A background task with no timeout is treated as a monitor and does not block goal-loop re-firing\./);
    assert.match(hatch.description, /must exit when the event occurs/);
  });

  test("the tool rejects foreground timeout_s and non-positive timeout_s with an error", async () => {
    const { hatch } = gateway();
    await assert.rejects(hatch.execute("c1", { command: "true", why: "w", timeout_s: 5 }), /only with background: true/);
    await assert.rejects(hatch.execute("c2", { command: "true", why: "w", background: true, timeout_s: 0 }), /positive integer/);
    assert.equal(listBackgroundTasks().length, 0, "nothing was started");
  });
});

describe("background lifecycle through the tools", posixOnly, () => {
  test("background returns a readable id at once; completion pushes why, exit code, and capped output", async () => {
    const { hatch, sent } = gateway();
    const started = text(await hatch.execute("c1", { command: `head -c ${ONE_LINER_OUTPUT_CAP_BYTES * 3} /dev/zero | tr '\\0' 'x'; echo; echo done`, why: "wait for the build log", background: true }));
    const id = taskIdOf(started);
    assert.match(id, WORD_CHAIN);
    assert.match(started, /monitor \(no timeout\)/);
    await waitFor(() => sent.length === 1);
    const push = sent[0];
    assert.equal(push.message.customType, BACKGROUND_TASK_PUSH_FAMILY);
    assert.equal(push.options.deliverAs, "steer");
    assert.equal(push.message.details.task_id, id);
    assert.equal(push.message.details.why, "wait for the build log");
    assert.equal(push.message.details.exit_code, "0");
    const output = String(push.message.details.output);
    assert.match(output, /output truncated/, "the 4KB cap applies with its drop hint");
    assert.ok(Buffer.byteLength(output) < ONE_LINER_OUTPUT_CAP_BYTES + 200);
    assert.match(push.message.content, /why: wait for the build log/);
    assert.equal(listBackgroundTasks().length, 0, "a finished task leaves the registry");
  });

  test("timeout expiry kills the task and reports it as a timeout", async () => {
    const { hatch, sent } = gateway();
    const id = taskIdOf(text(await hatch.execute("c1", { command: "echo started; sleep 30", why: "poll", background: true, timeout_s: 1 })));
    assert.equal(hasRunningTimedBackgroundTask(), true);
    await waitFor(() => sent.length === 1);
    const details = sent[0].message.details;
    assert.equal(details.task_id, id);
    assert.match(String(details.exit_code), /killed — not a clean exit/);
    assert.equal(details.note, "timed out after 1s");
    assert.match(String(details.output), /started/);
  });

  test("abort by id stops the process and returns its result inline; no push follows", async () => {
    const { hatch, abort, sent } = gateway();
    const id = taskIdOf(text(await hatch.execute("c1", { command: "echo partial; sleep 30", why: "watch", background: true })));
    await new Promise((resolve) => setTimeout(resolve, 200));
    const result = text(await abort.execute("a1", { task_id: id }));
    assert.match(result, new RegExp(`task_id: ${id}`));
    assert.match(result, /exit_code: .*killed — not a clean exit/);
    assert.match(result, /note: aborted with ws-background-abort/);
    assert.match(result, /output: partial/);
    await new Promise((resolve) => setTimeout(resolve, 300));
    assert.equal(sent.length, 0, "an aborted task produces no completion push");
    await assert.rejects(abort.execute("a2", { task_id: id }), /no running background task/, "an aborted id is no longer abortable");
  });

  test("abort without an id lists running tasks, each marked timed or monitor", async () => {
    const { hatch, abort } = gateway();
    assert.match(text(await abort.execute("a0", {})), /No background tasks are running/);
    const timed = taskIdOf(text(await hatch.execute("c1", { command: "sleep 30", why: "deploy wait", background: true, timeout_s: 600 })));
    const monitor = taskIdOf(text(await hatch.execute("c2", { command: "sleep 30", why: "error watch", background: true })));
    const listing = text(await abort.execute("a1", {}));
    assert.match(listing, /2 background tasks running/);
    assert.match(listing, new RegExp(`- ${timed} \\[timed, timeout_s 600\\] running \\d+s; why: deploy wait`));
    assert.match(listing, new RegExp(`- ${monitor} \\[monitor\\] running \\d+s; why: error watch`));
  });

  test("an unknown id and an already-finished id both get the unknown-id error", async () => {
    const { hatch, abort, sent } = gateway();
    await assert.rejects(abort.execute("a1", { task_id: "never-was-here" }), /no running background task "never-was-here"/);
    const id = taskIdOf(text(await hatch.execute("c1", { command: "true", why: "quick", background: true })));
    await waitFor(() => sent.length === 1);
    await assert.rejects(abort.execute("a2", { task_id: id }), /no running background task/);
  });

  test("abort racing the process's own exit yields exactly one report", async () => {
    for (const delayMs of [0, 40, 80, 100, 120, 160]) {
      const { hatch, abort, sent } = gateway();
      const id = taskIdOf(text(await hatch.execute("c1", { command: "sleep 0.1", why: "race", background: true })));
      await new Promise((resolve) => setTimeout(resolve, delayMs));
      let abortReports = 0;
      try {
        await abort.execute("a1", { task_id: id });
        abortReports = 1;
      } catch {
        // the exit claimed it first
      }
      await new Promise((resolve) => setTimeout(resolve, 400));
      const pushes = sent.filter((entry) => entry.message.details.task_id === id).length;
      assert.equal(abortReports + pushes, 1, `delay ${delayMs}ms: abort ${abortReports}, push ${pushes}`);
    }
  });

  test("a fork rejects background:true; its abort tool lists nothing and errors on any id", async () => {
    const lead = gateway();
    const leadId = taskIdOf(text(await lead.hatch.execute("c1", { command: "sleep 30", why: "lead task", background: true })));
    const fork = gateway({ isFork: true });
    await assert.rejects(fork.hatch.execute("c2", { command: "sleep 30", why: "w", background: true }), /lead only/);
    // A fork is its own process: its registry holds none of the lead's tasks.
    // In this single test process the lead's task is visible, so the fork's
    // empty view is shown by stopping it first.
    killAllBackgroundTasks();
    assert.match(text(await fork.abort.execute("a1", {})), /No background tasks are running/);
    await assert.rejects(fork.abort.execute("a2", { task_id: leadId }), /no running background task/);
  });
});

describe("process-tree kill", posixOnly, () => {
  for (const how of ["abort", "timeout", "shutdown"] as const) {
    test(`a pipeline descendant is gone after ${how}`, async () => {
      const monitor = pipelineMonitor();
      try {
        const results: BackgroundTaskResult[] = [];
        const task = startBackgroundTask({
          shell: "/bin/bash", args: ["-c", monitor.command], cwd: tmpdir(), command: monitor.command, why: "watch",
          capBytes: 64, onComplete: (result) => results.push(result),
          ...(how === "timeout" ? { timeoutS: 1, timeoutMs: 300 } : {}),
        });
        await waitFor(() => processAlive(monitor.tailPattern));
        if (how === "abort") {
          const result = await abortBackgroundTask(task.id);
          assert.equal(result?.ending, "aborted");
        } else if (how === "timeout") {
          await waitFor(() => results.length === 1);
          assert.equal(results[0].ending, "timed-out");
        } else {
          const handlers = new Map<string, (event: unknown) => void>();
          registerBackgroundTaskShutdown({ on: (name: string, handler: (event: unknown) => void) => handlers.set(name, handler) } as unknown as ExtensionAPI);
          handlers.get("session_shutdown")!({ reason: "quit" });
        }
        await waitFor(() => !processAlive(monitor.tailPattern));
        if (how === "shutdown") {
          await new Promise((resolve) => setTimeout(resolve, 200));
          assert.equal(results.length, 0, "a session-killed task reports nowhere");
        }
      } finally {
        monitor.cleanup();
      }
    });
  }

  for (const reason of ["quit", "reload"]) {
    test(`session_shutdown with reason ${reason} kills every remaining task`, async () => {
      const handlers = new Map<string, (event: unknown) => void>();
      registerBackgroundTaskShutdown({ on: (name: string, handler: (event: unknown) => void) => handlers.set(name, handler) } as unknown as ExtensionAPI);
      const marker = `ws-bg-shutdown-${reason}-${process.pid}`;
      const results: BackgroundTaskResult[] = [];
      for (const timeoutS of [undefined, 600]) {
        startBackgroundTask({ shell: "/bin/bash", args: ["-c", `sleep 30; echo ${marker}`], cwd: tmpdir(), command: "sleep", why: "w", timeoutS, capBytes: 64, onComplete: (result) => results.push(result) });
      }
      await waitFor(() => processAlive(marker));
      handlers.get("session_shutdown")!({ reason });
      assert.equal(listBackgroundTasks().length, 0);
      await waitFor(() => !processAlive(marker));
      await new Promise((resolve) => setTimeout(resolve, 200));
      assert.equal(results.length, 0);
    });
  }
});

describe("bounded capture", posixOnly, () => {
  test("keeps at most capBytes + 1 bytes of a large output", async () => {
    const results: BackgroundTaskResult[] = [];
    startBackgroundTask({ shell: "/bin/bash", args: ["-c", "head -c 200000 /dev/zero | tr '\\0' 'y'"], cwd: tmpdir(), command: "big", why: "w", capBytes: 1000, onComplete: (result) => results.push(result) });
    await waitFor(() => results.length === 1);
    assert.equal(Buffer.byteLength(results[0].output), 1001);
    assert.match(backgroundTaskReportFields(results[0], 1000).output, /output truncated/);
  });
});

describe("fan-in status line with background tasks", () => {
  const registryWith = (running: boolean) => new Map([["a", { threadBound: false, running, streaming: false }]]) as unknown as RpcAgentRegistry;
  const cases: Array<[RpcAgentRegistry | undefined, { timed: number; monitors: number }, string | undefined]> = [
    [undefined, { timed: 0, monitors: 0 }, undefined],
    [registryWith(false), { timed: 0, monitors: 0 }, "0 delegated agents still running"],
    [registryWith(true), { timed: 0, monitors: 0 }, "1 delegated agent still running"],
    [registryWith(false), { timed: 2, monitors: 0 }, "0 delegated agents, 2 background tasks still running"],
    [registryWith(true), { timed: 1, monitors: 3 }, "1 delegated agent, 1 background task, 3 monitors still running"],
    [registryWith(false), { timed: 0, monitors: 1 }, "0 delegated agents, 1 monitor still running"],
    [undefined, { timed: 1, monitors: 0 }, "1 background task still running"],
    [undefined, { timed: 2, monitors: 2 }, "2 background tasks, 2 monitors still running"],
    [undefined, { timed: 0, monitors: 1 }, "1 monitor still running"],
  ];

  test("renders each segment combination; agents-only is unchanged", () => {
    for (const [registry, background, expected] of cases) {
      assert.equal(computeRunningStatusLine(registry, background), expected, JSON.stringify(background));
    }
  });

  test("push-render recognizes every form positionally", () => {
    for (const [, , line] of cases) {
      if (!line) continue;
      assert.match(line, STATUS_LINE_PATTERN);
      const parts = buildPushRenderLines({ content: `[ws-agent-settled] agent a\nreason: idle\n${line}` });
      assert.equal(parts?.status, line);
    }
    for (const notStatus of ["1 monitor, 1 background task still running", "still running", "0 delegated agents, still running"]) {
      assert.doesNotMatch(notStatus, STATUS_LINE_PATTERN);
    }
  });

  test("defaults to the live registry: a running monitor joins the line", posixOnly, () => {
    startBackgroundTask({ shell: "/bin/sh", args: ["-c", "sleep 30"], cwd: tmpdir(), command: "sleep 30", why: "w", capBytes: 64, onComplete: () => {} });
    assert.deepEqual(countBackgroundTasks(), { timed: 0, monitors: 1 });
    assert.equal(hasRunningTimedBackgroundTask(), false);
    assert.equal(computeRunningStatusLine(undefined), "1 monitor still running");
  });
});

test("formatElapsed", () => {
  assert.equal(formatElapsed(45_000), "45s");
  assert.equal(formatElapsed(185_000), "3m05s");
  assert.equal(formatElapsed(7_620_000), "2h07m");
});
