/**
 * Background tasks of the lead's one-liner exec hatch: the process-wide
 * registry of running background commands and their whole lifecycle.
 *
 * Its own module because four readers need it and one of them may not reach
 * the others: `execute-gateway.ts` starts and aborts tasks, `spawner.ts`
 * counts them into the pushed fan-in status line (and never imports
 * `execute-gateway.ts`), `goal-loop.ts` yields while a timed task runs, and
 * `lead-compaction.ts` lists them in the compaction summary. This module
 * imports none of them; it reports a finished task through the `onComplete`
 * callback its starter supplies, and formatting the result for the model is
 * the starter's job.
 *
 * Lifecycle:
 * - A task is spawned directly (not through `pi.exec`, which buffers all
 *   output) with stdout and stderr captured, interleaved in arrival order,
 *   up to `capBytes + 1` bytes: enough for a formatter to see the output
 *   overflowed, never more, so a long-running monitor cannot grow memory.
 * - On POSIX it runs in its own process group, so a kill reaches the whole
 *   tree (the `tail` of `tail -f <log> | grep -m1 ERROR` included); on Windows
 *   a kill is a `taskkill /T` tree kill.
 * - Each task is reported exactly once. Whichever of these claims it first
 *   owns the report and removes it from the registry: its own process exit
 *   (an ordinary exit or one its timeout forced; reported through
 *   `onComplete`), an abort (reported as the abort's return value), or a
 *   session-end kill (reported nowhere; the session is going away). A claimed
 *   task is no longer listed, counted, or abortable.
 * - Exit is detected on the child's own `exit`, not on stream close: a
 *   descendant the command deliberately detached may hold stdout open. After
 *   exit the streams get a short idle grace to drain (re-armed by output, but
 *   never past a hard deadline fixed at exit), then are destroyed; such a
 *   detached descendant is left running, as the foreground hatch leaves it.
 */

import { spawn, type ChildProcess } from "node:child_process";
import { join } from "node:path";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { generateWordChainId } from "./word-chain-id.ts";

/** How long, after the task's own exit, output may still drain before the streams are cut. */
const EXIT_STDIO_GRACE_MS = 100;

/**
 * Hard ceiling on the post-exit drain, fixed at exit. Each output chunk
 * re-arms the idle grace above, so a descendant that inherited stdout and
 * keeps writing would otherwise hold the streams open forever, and the task
 * (already claimed and unlisted at exit) would never report. One second is
 * ten idle graces: room for a real tail of buffered output, short enough that
 * a completion push is never noticeably late.
 */
const EXIT_STDIO_MAX_DRAIN_MS = 1000;

/**
 * Node's largest timer delay (2^31-1 ms, about 24.8 days). A longer delay is
 * not an error to Node: it warns and fires after 1 ms, which would kill a task
 * with a very large `timeout_s` at once. Longer timeouts re-arm in steps.
 */
const MAX_TIMER_DELAY_MS = 2 ** 31 - 1;

/** Bumped by every session-end kill; a completion from an earlier session is dropped, never pushed into the next one. */
let sessionEpoch = 0;

export interface BackgroundTaskInfo {
  id: string;
  why: string;
  command: string;
  /** Present for a timed task; absent for a monitor. */
  timeoutS?: number;
  /** `Date.now()` at spawn. */
  startedAt: number;
}

export interface BackgroundTaskResult extends BackgroundTaskInfo {
  /** The process's exit code, `null` when it ended by a signal or never started. */
  code: number | null;
  signal: NodeJS.Signals | null;
  /** Interleaved stdout+stderr, at most `capBytes + 1` bytes of it. */
  output: string;
  /** `exited`: ended on its own; `timed-out`: killed by its own timeout; `aborted`: killed by an abort. */
  ending: "exited" | "timed-out" | "aborted";
  /** Set when the process could not be spawned at all. */
  spawnError?: string;
}

interface LiveTask {
  info: BackgroundTaskInfo;
  child: ChildProcess;
  claim?: "exit" | "abort" | "shutdown";
  timedOut: boolean;
  timer?: ReturnType<typeof setTimeout>;
  done: Promise<BackgroundTaskResult>;
}

const liveTasks = new Map<string, LiveTask>();

export interface StartBackgroundTaskOptions {
  shell: string;
  args: string[];
  cwd: string;
  command: string;
  why: string;
  /** Kill the task after this many seconds; absent makes it a monitor that runs until it exits, is aborted, or the session ends. */
  timeoutS?: number;
  /** Test seam: the timeout in milliseconds, overriding `timeoutS * 1000` while `timeoutS` stays the reported value. */
  timeoutMs?: number;
  capBytes: number;
  /** Called once when the task's own exit claimed it. Never called for an aborted or session-killed task. */
  onComplete: (result: BackgroundTaskResult) => void;
  /** Test seam for the id generator. */
  generateId?: () => string;
}

/** Spawns a background task and registers it; returns its info (with the readable id) immediately. */
export function startBackgroundTask(options: StartBackgroundTaskOptions): BackgroundTaskInfo {
  const generate = options.generateId ?? (() => generateWordChainId());
  let id = generate();
  while (liveTasks.has(id)) id = generate();
  const info: BackgroundTaskInfo = { id, why: options.why, command: options.command, timeoutS: options.timeoutS, startedAt: Date.now() };

  const child = spawn(options.shell, options.args, {
    cwd: options.cwd,
    stdio: ["ignore", "pipe", "pipe"],
    detached: process.platform !== "win32",
    windowsHide: true,
  });
  const chunks: Buffer[] = [];
  let captured = 0;
  const capture = (data: Buffer) => {
    const room = options.capBytes + 1 - captured;
    if (room <= 0) return;
    const piece = data.length > room ? data.subarray(0, room) : data;
    chunks.push(piece);
    captured += piece.length;
  };
  child.stdout?.on("data", capture);
  child.stderr?.on("data", capture);

  const task: LiveTask = { info, child, timedOut: false, done: undefined as never };
  const startEpoch = sessionEpoch;
  // Registered before `waitForExit`'s own listeners, so the claim is taken
  // synchronously at exit, ahead of the drain.
  child.once("exit", () => claim(task, "exit"));
  child.once("error", () => claim(task, "exit"));
  task.done = waitForExit(child).then((ended) => ({
    ...info,
    code: ended.code,
    signal: ended.signal,
    output: Buffer.concat(chunks).toString("utf8"),
    ending: task.claim === "abort" ? "aborted" : task.timedOut ? "timed-out" : "exited",
    // An `error` after a successful spawn (a failed kill) is not a failed start.
    ...(ended.error !== undefined && child.pid === undefined ? { spawnError: ended.error } : {}),
  }));
  void task.done.then((result) => {
    // A task that exited during a session end's drain window reports nowhere,
    // like the tasks that session end killed.
    if (task.claim === "exit" && startEpoch === sessionEpoch) options.onComplete(result);
  });

  const timeoutMs = options.timeoutMs ?? (options.timeoutS !== undefined ? options.timeoutS * 1000 : undefined);
  if (timeoutMs !== undefined) {
    const deadline = Date.now() + timeoutMs;
    const arm = (delay: number) => {
      task.timer = setTimeout(() => {
        if (task.claim) return;
        const remaining = deadline - Date.now();
        if (remaining > 0) return arm(Math.min(remaining, MAX_TIMER_DELAY_MS));
        task.timedOut = true;
        killProcessTree(child);
      }, delay);
      task.timer.unref?.();
    };
    arm(Math.min(timeoutMs, MAX_TIMER_DELAY_MS));
  }
  liveTasks.set(id, task);
  return info;
}

/** Takes the single report claim; false when another path already holds it. */
function claim(task: LiveTask, by: NonNullable<LiveTask["claim"]>): boolean {
  if (task.claim) return false;
  task.claim = by;
  if (task.timer) clearTimeout(task.timer);
  if (liveTasks.get(task.info.id) === task) liveTasks.delete(task.info.id);
  return true;
}

/**
 * Aborts a running task: claims it, kills its whole tree, and resolves with
 * its result once it has exited. `undefined` when no running task has this
 * id — never started, already finished (its exit claimed it), or already
 * aborted.
 */
export function abortBackgroundTask(id: string): Promise<BackgroundTaskResult> | undefined {
  const task = liveTasks.get(id);
  if (!task || !claim(task, "abort")) return undefined;
  killProcessTree(task.child);
  return task.done;
}

/**
 * Waits for an aborted task's result, but no longer than `waitMs` and no
 * longer than `signal` stays unaborted; `undefined` when neither the result
 * arrived in time nor the wait was allowed to continue. The abort already
 * holds the task's single report claim, so giving up here loses nothing but
 * the exit confirmation: a kill that did not take (a failed `taskkill` on
 * Windows, say) must not hang the caller.
 */
export function awaitAbortedTask(done: Promise<BackgroundTaskResult>, waitMs: number, signal?: AbortSignal): Promise<BackgroundTaskResult | undefined> {
  if (signal?.aborted) return Promise.resolve(undefined);
  return new Promise((resolve) => {
    const finish = (result: BackgroundTaskResult | undefined) => {
      clearTimeout(timer);
      signal?.removeEventListener("abort", onAbort);
      resolve(result);
    };
    const onAbort = () => finish(undefined);
    const timer = setTimeout(() => finish(undefined), waitMs);
    signal?.addEventListener("abort", onAbort, { once: true });
    void done.then(finish);
  });
}

/** Kills every running task with no report; the session-end backstop. Returns how many were killed. */
export function killAllBackgroundTasks(): number {
  sessionEpoch += 1;
  let killed = 0;
  for (const task of [...liveTasks.values()]) {
    if (!claim(task, "shutdown")) continue;
    killProcessTree(task.child);
    killed += 1;
  }
  return killed;
}

/** Running tasks, oldest first. */
export function listBackgroundTasks(): BackgroundTaskInfo[] {
  return [...liveTasks.values()].map((task) => task.info).sort((a, b) => a.startedAt - b.startedAt);
}

/** Running tasks split by kind: `timed` carry a timeout, `monitors` do not. */
export function countBackgroundTasks(): { timed: number; monitors: number } {
  let timed = 0;
  let monitors = 0;
  for (const task of liveTasks.values()) {
    if (task.info.timeoutS !== undefined) timed += 1;
    else monitors += 1;
  }
  return { timed, monitors };
}

/** Goal-loop yield input: a timed task is outstanding work; a monitor is not. */
export function hasRunningTimedBackgroundTask(): boolean {
  return countBackgroundTasks().timed > 0;
}

/** `45s`, `3m05s`, `2h07m`. */
export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  if (hours > 0) return `${hours}h${String(minutes).padStart(2, "0")}m`;
  if (minutes > 0) return `${minutes}m${String(seconds).padStart(2, "0")}s`;
  return `${seconds}s`;
}

/** One listing line, shared by the abort tool's list and the compaction summary. */
export function formatBackgroundTaskLine(info: BackgroundTaskInfo, now: number = Date.now()): string {
  const kind = info.timeoutS !== undefined ? `timed, timeout_s ${info.timeoutS}` : "monitor";
  return `- ${info.id} [${kind}] running ${formatElapsed(now - info.startedAt)}; why: ${info.why}`;
}

/**
 * Session end kills every remaining task. Pi emits `session_shutdown` on quit
 * and on every session switch (`reload`, `new`, `resume`, `fork`), and with no
 * fixed ceiling on a monitor this is the only backstop against orphans.
 * Registered at extension-factory time.
 */
export function registerBackgroundTaskShutdown(pi: ExtensionAPI): void {
  pi.on("session_shutdown", () => {
    killAllBackgroundTasks();
  });
}

function killProcessTree(child: ChildProcess): void {
  const pid = child.pid;
  if (pid === undefined) return;
  if (process.platform === "win32") {
    try {
      const killer = spawn(join(process.env.SystemRoot ?? "C:\\Windows", "System32", "taskkill.exe"), ["/F", "/T", "/PID", String(pid)], { stdio: "ignore", windowsHide: true });
      killer.once("error", () => {});
    } catch {
      // Already gone, or taskkill unavailable.
    }
    return;
  }
  try {
    process.kill(-pid, "SIGKILL");
  } catch {
    try {
      child.kill("SIGKILL");
    } catch {
      // Already gone.
    }
  }
}

function waitForExit(child: ChildProcess): Promise<{ code: number | null; signal: NodeJS.Signals | null; error?: string }> {
  return new Promise((resolve) => {
    let settled = false;
    let exited = false;
    let code: number | null = null;
    let signal: NodeJS.Signals | null = null;
    let stdoutEnded = !child.stdout;
    let stderrEnded = !child.stderr;
    let grace: ReturnType<typeof setTimeout> | undefined;
    let drainDeadline: ReturnType<typeof setTimeout> | undefined;
    const finalize = (error?: string) => {
      if (settled) return;
      settled = true;
      if (grace) clearTimeout(grace);
      if (drainDeadline) clearTimeout(drainDeadline);
      child.stdout?.destroy();
      child.stderr?.destroy();
      resolve({ code, signal, ...(error !== undefined ? { error } : {}) });
    };
    const maybeFinalize = () => {
      if (exited && stdoutEnded && stderrEnded) finalize();
    };
    const armGrace = () => {
      if (!exited || settled) return;
      if (grace) clearTimeout(grace);
      grace = setTimeout(() => finalize(), EXIT_STDIO_GRACE_MS);
    };
    child.stdout?.on("end", () => { stdoutEnded = true; maybeFinalize(); });
    child.stderr?.on("end", () => { stderrEnded = true; maybeFinalize(); });
    child.stdout?.on("data", armGrace);
    child.stderr?.on("data", armGrace);
    child.once("exit", (exitCode, exitSignal) => {
      exited = true;
      code = exitCode;
      signal = exitSignal;
      maybeFinalize();
      if (!settled) drainDeadline = setTimeout(() => finalize(), EXIT_STDIO_MAX_DRAIN_MS);
      armGrace();
    });
    child.once("error", (error) => finalize(error.message));
  });
}
