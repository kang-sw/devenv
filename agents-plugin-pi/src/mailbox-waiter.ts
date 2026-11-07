/**
 * 260914: pi's native mailbox active-push adapter.
 *
 * Every other harness (Codex, Claude) surfaces cross-session mail only at a
 * turn boundary, through a Stop hook that merely nudges the model to call the
 * mailbox recv tool. Pi instead runs a session-bound background waiter that
 * ACTIVELY injects an arriving message into the running conversation as a
 * steering push, so mail can preempt a live turn.
 *
 * Mechanism (see the ws mailbox CLI/tools). `mailbox wait` BLOCKS until mail is
 * present but only PEEKS — it never dequeues — whereas the mailbox recv tool
 * DRAINS. This waiter therefore uses `mailbox wait` purely as the
 * block-until-mail SIGNAL: the only thing it reads is the CLI exit code (0 =
 * mail present, 3 = timed out, 130 = interrupted, other = error). The recv tool
 * is the authoritative source: on a wake the waiter drains, then admits the
 * whole drain through the shared push FIFO (spawner's `sendBatchToLead`) as one
 * unit of informational `ws-mailbox` items, so it rides one `ws-push-batch`
 * alongside every other system message. One unit matters while the lead is
 * busy: per-envelope steers would reach Pi's default `one-at-a-time` steering
 * queue as separate messages and land one per model call. Draining via recv,
 * rather than replaying `mailbox wait`'s own peeked payload, is what stops the
 * peek from re-delivering the same mail on the next re-arm (which would be a hot
 * loop) and guarantees pi pushes exactly what recv removed — nothing is
 * double-delivered or lost.
 *
 * The two side-effecting edges — the `mailbox wait` subprocess and the recv
 * drain — are injected as `runWait`/`drainMail`, so the arrival -> push
 * detection path is unit-testable with a fake waiter and no live mailbox. The
 * real edges are `createSubprocessWait` / `createBridgeDrain`.
 *
 * Adapter-only, best-effort: this never alters the host-neutral mailbox
 * `Envelope` or contract. Phase 1 armed the wait on the always-available
 * reply-id channel only (`--session-key`, no `--slug`); recv still drained
 * any owned named inbox too, so its mail was pushed opportunistically
 * whenever a reply-id wake fired.
 *
 * 260917 Phase 1 widens the wake signal: at arm time the caller resolves its
 * own registered named-inbox address via `mailbox.lookup_peers`'s
 * `self.address` (through the same `MailboxToolCall` seam `drainMail` already
 * uses — see `resolveMailboxSelfSlug`) and, when one resolves, passes it as
 * `--slug` so the block-until-mail signal also covers the owned named inbox.
 * `lookup_peers`, not `process.env.WS_MAILBOX`/`WS_MAILBOX_AUTO`, is the
 * source: a fresh CLI process re-deriving `WS_MAILBOX_AUTO` would mint a
 * different random stem than the already-registered server process (see
 * `wait.go`'s `WaitTarget.Slug` doc comment), so only the bridge's own
 * `lookup_peers` answer is a reliable identity here. No self slug (or a
 * `lookup_peers` transport failure) keeps today's reply-id-only arm exactly
 * as before — additive-only, never a regression.
 *
 * The same wait also carries messages the user sends from the dashboard page
 * (`board` deps, `--board-inbox`): the wait peeks the session's board-answer
 * inbox, the adapter-only `dashboard.inbox_drain` tool claims its entries,
 * and the drain is submitted as the user's own message. See
 * `startMailboxWaiter` for when a message is held until the turn ends, and
 * for the held message the user promoted to steering (`--board-steer`).
 */

import { spawn } from "node:child_process";
import { chmodSync, copyFileSync, mkdtempSync, rmSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { whiteboardSubmissionText } from "./adapter-label.ts";
import type { SpawnRole } from "./process-role.ts";

/**
 * One queued mailbox message, mirroring the ws `Envelope` JSON shape
 * (`{from?, reply_to?, content, sent_at}`). Exactly one of `from`/`reply_to`
 * is set on delivery; `content` is free text and always present.
 */
export interface MailboxEnvelope {
  from?: string;
  reply_to?: string;
  content: string;
  sent_at?: string;
}

/**
 * `mailbox wait`'s result, distilled from its CLI exit code and, for a wait
 * armed with `--board-inbox`, from the `kinds` its JSON output reports:
 * `mail` is unread mail only, `board` a held dashboard message only, and
 * `mail+board` both.
 */
export type MailboxWaitOutcome = "mail" | "board" | "mail+board" | "timeout" | "stopped" | "error";

/**
 * How one wait is armed: `boardInbox` adds `--board-inbox`, which wakes on
 * any held dashboard message; `boardSteer` adds `--board-steer`, which wakes
 * only on one promoted to steering. At most one is set.
 */
export interface MailboxWaitArm {
  boardInbox: boolean;
  boardSteer?: boolean;
}

/** The custom-message `customType` every arriving mail item rides under. */
export const WS_MAILBOX_CUSTOM_TYPE = "ws-mailbox";

/** The informational custom message an arriving mail is admitted through the FIFO as. */
export interface MailboxPushMessage {
  customType: typeof WS_MAILBOX_CUSTOM_TYPE;
  content: string;
  display: true;
  details: { from?: string; reply_to?: string; content: string; sent_at?: string };
}

/**
 * Build the `ws-mailbox` custom message for one arriving envelope. The model
 * reads only `content`, so it opens with a human `mail from <handle>` head and
 * the free-text body; `details` carries the same fields structurally. `handle`
 * is the sender's shared-layer slug (`from`) when present, otherwise the
 * reply-id handle (`reply_to`).
 */
export function buildMailboxPushMessage(envelope: MailboxEnvelope): MailboxPushMessage {
  const handle = envelope.from?.trim() || envelope.reply_to?.trim() || "unknown";
  const stamp = envelope.sent_at?.trim();
  const head = stamp ? `mail from ${handle} (${stamp})` : `mail from ${handle}`;
  return {
    customType: WS_MAILBOX_CUSTOM_TYPE,
    content: `${head}:\n${envelope.content}`,
    display: true,
    details: {
      ...(envelope.from ? { from: envelope.from } : {}),
      ...(envelope.reply_to ? { reply_to: envelope.reply_to } : {}),
      content: envelope.content,
      ...(stamp ? { sent_at: stamp } : {}),
    },
  };
}

/**
 * Gate for arming the waiter: an owner lead (no spawn-role marker) that already
 * has its own session key. A fork/worker/explore child has no cross-session
 * inbox worth a background subprocess, and no key means bootstrap has not
 * resolved one yet. Narrows `sessionKey` to `string` on success so the caller
 * can pass it straight to the subprocess/drain factories.
 */
export function shouldArmMailboxWaiter(role: SpawnRole | undefined, sessionKey: string | undefined): sessionKey is string {
  return role === undefined && typeof sessionKey === "string" && sessionKey.length > 0;
}

/**
 * Submission of messages the user sent from the dashboard page. They wait
 * in the session's board-answer inbox until drained; a drained message is
 * claimed, so the dashboard can no longer merge into or cancel it.
 */
export interface BoardInboxDeps {
  /** The adapter knob; must not throw. `false` arms the wait without `--board-inbox` and drains nothing. */
  enabled: () => boolean | Promise<boolean>;
  /** `true` while the lead runs no turn. */
  idle: () => boolean;
  /** Claim every held message; resolve with their texts. Throws on a tool error. */
  drain: () => Promise<string[]>;
  /** Submit one drain as the user's own input. */
  inject: (texts: string[]) => void;
  /** Steering a held message the user promoted; absent, every held message waits for the turn end. */
  steer?: BoardSteerDeps;
}

/**
 * Delivery of held dashboard messages the user promoted to steering: drained
 * alone and sent into the running turn instead of after it.
 */
export interface BoardSteerDeps {
  /**
   * `true` while a steer reaches the running turn: the lead's run flag is
   * set and no compaction holds the session (Pi refuses a prompt then, and
   * the message would be lost). Must not throw.
   */
  steerable: () => boolean;
  /** Claim only the held messages promoted to steering; resolve with their texts. Throws on a tool error. */
  drain: () => Promise<string[]>;
  /** Send one steer drain into the running turn as the user's own input. */
  inject: (texts: string[]) => void;
}

/** What `boardSteerDeps` takes from the extension. */
export interface BoardSteerWiring {
  /** The lead's own run flag (`agent_start` to `agent_settled`). */
  turnOpen: () => boolean;
  /** The lead's compaction hold. */
  compacting: () => boolean;
  /** The steer-only inbox drain (`createBridgeInboxDrain` with `steerOnly`). */
  drain: () => Promise<string[]>;
  /** Pi's `sendUserMessage`. */
  sendUserMessage: (text: string, options: { deliverAs: "steer" }) => void;
}

/**
 * The extension's steer deps. A steer is a user message under the
 * whiteboard label, so the dashboard feed records it as a whiteboard user
 * message (`dashboardUserMessage`), as it records a follow-up submission.
 */
export function boardSteerDeps(wiring: BoardSteerWiring): BoardSteerDeps {
  return {
    steerable: () => wiring.turnOpen() && !wiring.compacting(),
    drain: wiring.drain,
    inject: (texts) => wiring.sendUserMessage(whiteboardSubmissionText(texts), { deliverAs: "steer" }),
  };
}

export interface MailboxWaiterDeps {
  /** Block until mail is present (or the wait times out); resolve with the outcome. */
  runWait: (signal: AbortSignal, arm: MailboxWaitArm) => Promise<MailboxWaitOutcome>;
  /** Authoritatively drain (dequeue) the session's mailbox; resolve with what was removed. */
  drainMail: () => Promise<MailboxEnvelope[]>;
  /** Admit one non-empty drain into the live conversation as one unit (through the push FIFO). */
  admit: (envelopes: MailboxEnvelope[]) => void;
  /** Delay after an error or a failed drain before re-arming, ms. */
  errorBackoffMs?: number;
  /** Injected sleep, so tests need no real timers. */
  sleep?: (ms: number) => Promise<void>;
  /** Diagnostic sink; omitted diagnostics are discarded rather than written to the host terminal. */
  onError?: (message: string) => void;
  /** Dashboard message submission; absent, the wait never carries `--board-inbox`. */
  board?: BoardInboxDeps;
}

export interface MailboxWaiterHandle {
  /** Stop the loop and abort any in-flight wait. Idempotent. */
  stop: () => void;
  /**
   * The lead's turn ended (or compaction finished). A dashboard message held
   * during the turn is drained and submitted now, and the wait is re-armed
   * with `--board-inbox`. A no-op when nothing is held.
   */
  leadSettled?: () => void;
  /** Resolves once the loop has fully exited, including subprocess close. */
  readonly done: Promise<void>;
}

export interface StagedMailboxRuntime {
  binaryPath: string;
  /** Best-effort removal; a locked file on Windows must never break shutdown. */
  cleanup: () => void;
}

/** Each session generation owns one copy, reused by every wait re-arm; reloads never replace it. */
export function stageMailboxRuntime(source: string, remove: typeof rmSync = rmSync): StagedMailboxRuntime {
  const directory = mkdtempSync(join(dirname(source), ".mailbox-"));
  const binaryPath = join(directory, process.platform === "win32" ? "ws-mcp.exe" : "ws-mcp");
  const cleanup = (): void => {
    try { remove(directory, { recursive: true, force: true, maxRetries: 3, retryDelay: 50 }); } catch { /* A Windows file lock can outlive close briefly. */ }
  };
  try {
    copyFileSync(source, binaryPath);
    if (process.platform !== "win32") chmodSync(binaryPath, statSync(source).mode & 0o777);
  } catch (error) {
    cleanup();
    throw error;
  }
  return { binaryPath, cleanup };
}

/** Cleanup belongs to this generation's completed wait, never to a newer session's handle. */
export function attachMailboxRuntimeCleanup(handle: MailboxWaiterHandle, staged: StagedMailboxRuntime): MailboxWaiterHandle {
  void handle.done.then(staged.cleanup, staged.cleanup);
  return handle;
}

const DEFAULT_ERROR_BACKOFF_MS = 5_000;

function defaultSleep(ms: number): Promise<void> {
  return new Promise<void>((resolve) => {
    const timer = setTimeout(resolve, ms);
    (timer as { unref?: () => void }).unref?.();
  });
}

/**
 * Run the session-bound wait/drain/admit loop until `stop()` is called. Every
 * edge is best-effort: a wait, drain, or admit that throws is reported and the
 * loop re-arms (after a backoff for a wait/drain failure, so a persistently
 * failing edge cannot hot-spin). A successful `mail` and a `timeout` both
 * re-arm immediately.
 *
 * With `board` deps and the knob on, the wait also watches the dashboard
 * inbox. A board wake while the lead is idle drains and injects at once. A
 * board wake during a turn leaves the message held in the inbox, so the
 * dashboard can still merge into or cancel it, and re-arms without
 * `--board-inbox`: the wait only peeks, so a held entry would wake it at
 * once. `leadSettled()` then ends that wait; the loop drains, injects, and
 * re-arms with the flag.
 *
 * While a message is held and the lead can take a steer (`steer` deps), the
 * wait is armed with `--board-steer` instead: a held follow-up does not wake
 * it, and a message the user promotes does. That wake drains the steered
 * messages alone and sends them into the running turn, ahead of any held
 * follow-up or mail. While a steer would be refused (a compaction hold, or
 * no run), the wait is armed without steer mode, so a steered entry left
 * held cannot wake it at once; `leadSettled()` (also called once compaction
 * ends) re-arms it. A steer drain that outlasts the run or enters a
 * compaction carries its texts to the next idle drain, which injects them
 * ahead of what it claims, so a claimed message is never dropped; an idle
 * drain that a turn or a compaction overtook carries its texts the same way.
 */
export function startMailboxWaiter(deps: MailboxWaiterDeps): MailboxWaiterHandle {
  const controller = new AbortController();
  const sleep = deps.sleep ?? defaultSleep;
  const backoffMs = deps.errorBackoffMs ?? DEFAULT_ERROR_BACKOFF_MS;
  const board = deps.board;
  const report = (message: string): void => {
    if (controller.signal.aborted) return;
    try {
      (deps.onError ?? (() => {}))(message);
    } catch {
      // A failing diagnostic sink must never break the loop.
    }
  };
  const describe = (error: unknown): string => error instanceof Error ? error.message : String(error);
  // A board message seen during a turn and left in the inbox until the turn ends.
  let held = false;
  // Steered messages claimed when a steer would no longer reach the turn:
  // the next idle drain injects them first.
  let carry: string[] = [];
  // The in-flight wait's own controller: `leadSettled` aborts it to re-arm,
  // while `controller` (stop) ends the loop.
  let waitController: AbortController | undefined;

  const boardEnabled = async (): Promise<boolean> => {
    try {
      return (await board!.enabled()) !== false;
    } catch {
      return true;
    }
  };
  const leadIdle = (): boolean => {
    try {
      return board?.idle() === true;
    } catch {
      return false;
    }
  };
  const leadSteerable = (): boolean => {
    try {
      return board?.steer?.steerable() === true;
    } catch {
      return false;
    }
  };
  /**
   * Drain (unless `drainHeld` is false) and inject, the carried steered
   * messages first; `false` when the drain failed, though the carried ones
   * are injected anyway. Clears the hold either way. When the lead is no
   * longer idle once the drain settles, everything claimed is carried to the
   * next settle instead of injected.
   */
  const drainBoard = async (drainHeld = true): Promise<boolean> => {
    held = false;
    const carried = carry;
    carry = [];
    let texts: string[] = [];
    let drained = true;
    if (drainHeld) {
      try {
        texts = await board!.drain();
      } catch (error) {
        report(`board drain failed: ${describe(error)}`);
        drained = false;
      }
    }
    texts = [...carried, ...texts];
    if (texts.length === 0 || controller.signal.aborted) return drained;
    // Re-checked after the drain's await, as steerBoard does: a turn or a
    // compaction (a manual /compact) may have begun meanwhile, where Pi would
    // refuse the submission and the claimed texts would be lost. Carried
    // instead, the next settle injects them.
    if (!leadIdle()) {
      carry = [...texts, ...carry];
      return drained;
    }
    try {
      board!.inject(texts);
    } catch (error) {
      report(`board inject failed: ${describe(error)}`);
    }
    return drained;
  };
  /** Drain the steered messages and steer them in; `false` when the drain failed. */
  const steerBoard = async (): Promise<boolean> => {
    let texts: string[];
    try {
      texts = await board!.steer!.drain();
    } catch (error) {
      report(`board steer drain failed: ${describe(error)}`);
      return false;
    }
    if (texts.length === 0 || controller.signal.aborted) return true;
    // Re-checked after the drain's await: the run may have ended or a
    // compaction begun, where Pi would refuse the steer and lose it.
    if (!leadSteerable()) {
      carry.push(...texts);
      return true;
    }
    try {
      board!.steer!.inject(texts);
    } catch (error) {
      report(`board steer failed: ${describe(error)}; delivering it after the turn instead`);
      carry.push(...texts);
    }
    return true;
  };

  const done = (async () => {
    while (!controller.signal.aborted) {
      // Without board deps the wait is armed synchronously, as before.
      const enabled = board ? await boardEnabled() : false;
      if (controller.signal.aborted) break;
      if (held && !enabled) held = false;
      // Checked here rather than only in leadSettled, so a turn that ended
      // while this loop was busy still releases the held message. Carried
      // steered messages go out even with the knob turned off since.
      if ((held || carry.length > 0) && leadIdle() && !(await drainBoard(held))) {
        // A failed drain leaves the entry held in the inbox; the next wait
        // (re-armed with the flag) wakes on it again after the backoff.
        if (!controller.signal.aborted) await sleep(backoffMs);
        continue;
      }
      if (controller.signal.aborted) break;

      const boardInbox = enabled && !held;
      const boardSteer = enabled && held && board?.steer !== undefined && leadSteerable();
      const wait = new AbortController();
      const onStop = (): void => wait.abort();
      controller.signal.addEventListener("abort", onStop, { once: true });
      waitController = wait;
      let outcome: MailboxWaitOutcome;
      try {
        outcome = await deps.runWait(wait.signal, { boardInbox, boardSteer });
      } catch (error) {
        outcome = "error";
        report(`wait failed: ${describe(error)}`);
      } finally {
        waitController = undefined;
        controller.signal.removeEventListener("abort", onStop);
      }
      if (controller.signal.aborted) break;
      if (outcome === "stopped") {
        // Our own re-arm (leadSettled) re-enters the loop; any other stop ends it.
        if (wait.signal.aborted) continue;
        break;
      }

      let failed = outcome === "error";
      if (outcome === "mail" || outcome === "mail+board") {
        try {
          const envelopes = await deps.drainMail();
          if (envelopes.length > 0 && !controller.signal.aborted) {
            try {
              deps.admit(envelopes);
            } catch (error) {
              report(`admit failed: ${describe(error)}`);
            }
          }
        } catch (error) {
          // A failed drain leaves the mail queued, so an immediate re-arm
          // would wake at once (peek) and hot-spin; back off first.
          failed = true;
          report(`drain failed: ${describe(error)}`);
        }
      }
      if ((outcome === "board" || outcome === "mail+board") && boardInbox && !controller.signal.aborted) {
        if (!leadIdle()) held = true;
        else if (!(await drainBoard())) failed = true;
      }
      if ((outcome === "board" || outcome === "mail+board") && boardSteer && !controller.signal.aborted) {
        // A steer refused now leaves the entry held; the next wait is armed
        // without steer mode, so it does not wake on it again at once.
        if (leadSteerable() && !(await steerBoard())) failed = true;
      }
      if (failed && !controller.signal.aborted) await sleep(backoffMs);
    }
  })();

  return {
    stop: () => controller.abort(),
    leadSettled: () => {
      if (held || carry.length > 0) waitController?.abort();
    },
    done,
  };
}

/**
 * Run `fn` once the lead's compaction hold has been released. The hold is
 * cleared in a `setImmediate` that a handler of the same compaction event
 * schedules, possibly after this call; a second check-phase turn runs after
 * every immediate queued during that event, so the hold is gone by then.
 */
export function afterCompactionHold(fn: () => void, defer: (callback: () => void) => void = setImmediate): void {
  defer(() => defer(fn));
}

export interface MailboxWaiterSlotDeps {
  /** Resolve the owned named-inbox slug for `sessionKey` (`resolveMailboxSelfSlug`); must not throw. */
  resolveSlug: (sessionKey: string) => Promise<string | undefined>;
  /** Start one waiter for `sessionKey`, armed with the slug just resolved for it. */
  start: (sessionKey: string, slug: string | undefined) => MailboxWaiterHandle;
}

/**
 * One bridge generation's mailbox waiter, re-armable for a new session key.
 * `stop()` and `done` make the slot itself the generation's waiter handle, so
 * every disposal path that stops and awaits a waiter covers it unchanged.
 */
export interface MailboxWaiterSlot extends MailboxWaiterHandle {
  /**
   * Resolve the slug for `sessionKey`, then replace the live waiter with one
   * armed for that key and slug, stopping the previous waiter only once its
   * successor runs. Resolves `true` when this call started a waiter, `false`
   * when a later `arm` or `stop()` superseded it while the slug was resolving.
   */
  arm: (sessionKey: string) => Promise<boolean>;
}

/**
 * The live waiter is re-armed whenever the default lead key changes: ws-mcp
 * moves the named-inbox owner to the new key, and a waiter still holding the
 * old `--session-key` falls back to a reply-id-only wait on the old queue,
 * so mail no longer wakes the session. The slug lookup is async, so two
 * guards keep at most one live waiter: a sequence number lets only the
 * newest `arm` start one, and `stop()` (generation disposal) refuses every
 * `arm` still resolving. `done` resolves only after `stop()` and once every
 * waiter this slot ever started has exited, so a staged runtime copy shared
 * by all of them is removed after the last one closes.
 */
export function createMailboxWaiterSlot(deps: MailboxWaiterSlotDeps): MailboxWaiterSlot {
  let latestArm = 0;
  let stopped = false;
  let live: MailboxWaiterHandle | undefined;
  const started: MailboxWaiterHandle[] = [];
  let markStopped: () => void = () => {};
  const stoppedSignal = new Promise<void>((resolve) => { markStopped = resolve; });
  const done = stoppedSignal.then(() => Promise.all(started.map((waiter) => waiter.done))).then(() => {});
  return {
    arm: async (sessionKey) => {
      if (stopped) return false;
      const armId = ++latestArm;
      const slug = await deps.resolveSlug(sessionKey);
      if (stopped || armId !== latestArm) return false;
      const next = deps.start(sessionKey, slug);
      started.push(next);
      const previous = live;
      live = next;
      previous?.stop();
      return true;
    },
    leadSettled: () => live?.leadSettled?.(),
    stop: () => {
      if (stopped) return;
      stopped = true;
      live?.stop();
      markStopped();
    },
    done,
  };
}

/**
 * Wrap a diagnostic sink so each distinct line reaches it at most once. A
 * waiter re-spawns `mailbox wait` on every timeout and every mail, and a
 * persistent condition prints the same stderr line each time; one wrapper
 * per armed waiter shows it once, and a re-arm (a new waiter) may show it
 * again.
 */
export function reportDistinctLinesOnce(sink: (line: string) => void): (line: string) => void {
  const seen = new Set<string>();
  return (line) => {
    if (seen.has(line)) return;
    seen.add(line);
    sink(line);
  };
}

/** The CLI exit code `ws-mcp mailbox wait` uses to signal a deadline with no unread mail. */
const MAILBOX_WAIT_EXIT_TIMEOUT = 3;

/**
 * Distill a finished `mailbox wait` child into an outcome.
 *
 * `aborted` (our own `stop()`) is authoritative: it is always `stopped`,
 * whatever the child's exit looks like. Otherwise exit 0 is `mail`, the timeout
 * code is `timeout`, and everything else — a nonzero code, or termination by a
 * signal we did not send (including the CLI's own 130 on an external
 * SIGINT/SIGTERM) — is `error`, so the loop re-arms with backoff. Mapping a
 * stray external signal to `error` rather than `stopped` is deliberate: a
 * `stopped` we did not cause would break the loop and silently disable
 * mail-push for the rest of the session.
 */
export function mapMailboxWaitExit(code: number | null, signal: NodeJS.Signals | null, aborted: boolean): MailboxWaitOutcome {
  if (aborted) return "stopped";
  if (code === 0) return "mail";
  if (code === MAILBOX_WAIT_EXIT_TIMEOUT) return "timeout";
  void signal;
  return "error";
}

export interface SubprocessWaitOptions {
  /** Absolute path to `bin/ws-mcp-launcher.py`; the launcher forwards the `mailbox wait` subcommand to the runtime. */
  launcherPath: string;
  /** Launcher cwd (the plugin dir), matching how the bridge spawns `serve --stdio`. */
  pluginDir: string;
  /** Immutable session-generation copy of the connected bridge's unique bootstrap. Absent for release-backed sessions. */
  runtimeBinary?: string;
  /** This session's own session key — the required `--session-key`; gives the reply-id queue to watch. */
  sessionKey: string;
  /**
   * Optional owned named-inbox address (`"name@scope"`), resolved for this
   * waiter's session key at arm time via `resolveMailboxSelfSlug`. When set, passed through as `--slug` so
   * the wait also covers the named inbox; omit (or leave `undefined`) for
   * today's reply-id-only wait.
   */
  slug?: string;
  /**
   * Optional session root (absolute path), passed through as `--root` so the
   * CLI resolves a worktree/clone-scope `--slug` against this session's store
   * rather than the launcher process's own working directory.
   */
  root?: string;
  /** `--timeout` value; a finite window self-heals a wedged wait and bounds the listening marker. */
  timeoutArg?: string;
  /** Adds `--board-inbox`, so a held dashboard message also wakes the wait. Set per wait by the loop. */
  boardInbox?: boolean;
  /** Adds `--board-steer` (when `boardInbox` is unset), so only a held message promoted to steering wakes the wait. Set per wait by the loop. */
  boardSteer?: boolean;
  /** Diagnostic sink for the child's stderr and spawn failures; omitted diagnostics are discarded. */
  onStderr?: (line: string) => void;
}

const DEFAULT_WAIT_TIMEOUT_ARG = "10m";

/**
 * Pure argv builder for the `mailbox wait` subprocess, factored out of
 * `createSubprocessWait` so the slug-arm/no-slug shapes are unit-testable
 * without spawning a real process (mirrors `mapMailboxWaitExit`'s split on the
 * exit side). `--root` and `--slug` are appended only when their option is a
 * non-empty string, so an invocation with neither stays byte-identical to the
 * reply-id-only shape.
 */
export function buildMailboxWaitArgv(options: SubprocessWaitOptions): string[] {
  const timeoutArg = options.timeoutArg ?? DEFAULT_WAIT_TIMEOUT_ARG;
  const argv = [options.launcherPath, "mailbox", "wait", "--session-key", options.sessionKey, "--timeout", timeoutArg, "--format", "json"];
  const root = options.root?.trim();
  if (root) argv.push("--root", root);
  const slug = options.slug?.trim();
  if (slug) argv.push("--slug", slug);
  if (options.boardInbox) argv.push("--board-inbox");
  else if (options.boardSteer) argv.push("--board-steer");
  return argv;
}

/**
 * Refine a successful `--board-inbox` or `--board-steer` wait by the `kinds` its JSON output
 * reports. Output that cannot be read counts as both kinds: a mail drain of
 * an empty queue and a board check of an empty inbox are both harmless,
 * while a missed kind would leave that arrival unhandled.
 */
export function boardWakeOutcome(stdout: string): MailboxWaitOutcome {
  let kinds: unknown;
  try {
    kinds = (JSON.parse(stdout) as { kinds?: unknown } | null)?.kinds;
  } catch {
    return "mail+board";
  }
  if (!Array.isArray(kinds)) return "mail+board";
  const mail = kinds.includes("mail");
  const board = kinds.includes("board");
  if (mail && board) return "mail+board";
  if (board) return "board";
  if (mail) return "mail";
  return "mail+board";
}

/**
 * Builds the arm-time `SubprocessWaitOptions` for a session: the wait's
 * `--root` is the session's own cwd, the same root the bridge's bootstrap
 * `ferrule` binds, so a worktree/clone-scope slug resolves the store that
 * session registered in even when Pi was launched from another directory.
 */
export function sessionMailboxWaitOptions(
  session: Omit<SubprocessWaitOptions, "root" | "timeoutArg"> & { cwd: string },
): SubprocessWaitOptions {
  return {
    launcherPath: session.launcherPath,
    pluginDir: session.pluginDir,
    runtimeBinary: session.runtimeBinary,
    sessionKey: session.sessionKey,
    slug: session.slug,
    root: session.cwd,
    onStderr: session.onStderr,
  };
}

/**
 * The real `runWait`: a source-loaded session invokes its immutable copy of
 * the binary used to bootstrap the connected bridge, avoiding mutable staging
 * and runtime contracts. Release-backed sessions retain the Python launcher.
 * Stdout — the peeked mail — is ignored (`stdio` drops it) except for a
 * `--board-inbox` wait, whose `kinds` alone are read: the drain, not the
 * peek, is the source of truth.
 */
export function createSubprocessWait(options: SubprocessWaitOptions): (signal: AbortSignal, arm?: MailboxWaitArm) => Promise<MailboxWaitOutcome> {
  const stderr = options.onStderr ?? (() => {});
  return (signal, arm) =>
    new Promise<MailboxWaitOutcome>((resolve) => {
      const boardInbox = arm?.boardInbox === true;
      const boardSteer = !boardInbox && arm?.boardSteer === true;
      // Only a board wait reads stdout: its `kinds` tell mail from a board
      // message. A mail-only wake needs no more than the exit code.
      const readsKinds = boardInbox || boardSteer;
      const reportStderr = (line: string): void => {
        if (signal.aborted) return;
        try {
          stderr(line);
        } catch {
          // A stale or otherwise failing diagnostic sink must not escape a child callback.
        }
      };
      if (signal.aborted) {
        resolve("stopped");
        return;
      }
      const argv = buildMailboxWaitArgv({ ...options, boardInbox, boardSteer });
      const child = spawn(options.runtimeBinary ?? "python3", options.runtimeBinary ? argv.slice(1) : argv, {
        cwd: options.pluginDir,
        stdio: ["ignore", readsKinds ? "pipe" : "ignore", "pipe"],
      });
      let stdout = "";
      let stdoutEnded = !readsKinds;
      let onStdoutEnd: (() => void) | undefined;
      child.stdout?.on("data", (chunk: Buffer) => { stdout += chunk.toString(); });
      child.stdout?.on("end", () => { stdoutEnded = true; onStdoutEnd?.(); });
      let settled = false;
      const finish = (outcome: MailboxWaitOutcome): void => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(readsKinds && outcome === "mail" ? boardWakeOutcome(stdout) : outcome);
      };
      const onAbort = (): void => {
        try {
          child.kill("SIGTERM");
        } catch {
          // Best effort — direct waits settle on close; release launchers on exit.
        }
      };
      signal.addEventListener("abort", onAbort, { once: true });
      child.stderr?.on("data", (chunk: Buffer) => {
        const text = chunk.toString().trimEnd();
        if (text) reportStderr(text);
      });
      child.on("error", (err) => {
        reportStderr(`wait spawn failed: ${err.message}`);
        // Node always emits close after an error, including a failed spawn.
      });
      // A direct local executable must be closed before its copy is removed.
      // Windows release launchers spawn ws-mcp as a grandchild whose inherited
      // pipe can keep `close` pending after Python exits; no copy is owned by
      // that path, so retain its original exit-based settlement.
      // A board wake read on exit gives its stdout a short grace to drain;
      // whatever is unread by then falls back to both kinds.
      if (!options.runtimeBinary) child.on("exit", (code, sig) => {
        const outcome = mapMailboxWaitExit(code, sig, signal.aborted);
        if (outcome !== "mail" || stdoutEnded) {
          finish(outcome);
          return;
        }
        const grace = setTimeout(() => finish(outcome), 250);
        (grace as { unref?: () => void }).unref?.();
        onStdoutEnd = () => { clearTimeout(grace); finish(outcome); };
      });
      child.on("close", (code, sig) => {
        finish(mapMailboxWaitExit(code, sig, signal.aborted));
      });
    });
}

/** Minimal MCP tool-call surface the drain and self-slug resolution need — the bridge's connected client satisfies it. */
export type MailboxToolCall = (
  name: string,
  args: Record<string, unknown>,
) => Promise<{ content: { type: string; text?: string }[]; isError?: boolean }>;

/**
 * The real `drainMail`: call the mailbox recv tool (`format: "json"` returns
 * the bare array of drained envelopes) through the bridge's already-connected
 * client. A tool-level error (`isError`) THROWS, so the loop treats it as a
 * failed drain and backs off rather than re-arming instantly — otherwise a
 * persistently failing drain-write over still-queued mail would hot-spin, since
 * `mailbox wait` keeps waking immediately on the un-drained mail. A
 * successful-but-malformed response (missing/non-JSON/non-array text) is
 * genuinely "no usable mail" and degrades to `[]` without a throw.
 */
export function createBridgeDrain(callTool: MailboxToolCall, sessionKey: string): () => Promise<MailboxEnvelope[]> {
  return async () => {
    const result = await callTool("mailbox.recv", { session_key: sessionKey, format: "json" });
    if (result.isError) {
      throw new Error(result.content.find((item) => item.type === "text")?.text ?? "mailbox.recv failed");
    }
    const text = result.content.find((item) => item.type === "text")?.text;
    if (!text) return [];
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      return [];
    }
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (candidate): candidate is MailboxEnvelope =>
        !!candidate && typeof candidate === "object" && typeof (candidate as { content?: unknown }).content === "string",
    );
  };
}

/** The adapter-only ws-mcp tool that claims the session's held dashboard messages. */
export const DASHBOARD_INBOX_DRAIN_TOOL = "dashboard.inbox_drain";

/**
 * The real board `drain`: claim every held dashboard message through the
 * bridge's client (`format: "json"` returns `{messages: [{text, ...}]}`) and
 * resolve with their texts in order; with `steerOnly`, only the messages
 * promoted to steering (the steer `drain`). A tool-level error throws, so
 * the loop backs off; a malformed success degrades to no messages.
 */
export function createBridgeInboxDrain(callTool: MailboxToolCall, sessionKey: string, options: { steerOnly?: boolean } = {}): () => Promise<string[]> {
  const args = { session_key: sessionKey, ...(options.steerOnly ? { steer_only: true } : {}), format: "json" };
  return async () => {
    const result = await callTool(DASHBOARD_INBOX_DRAIN_TOOL, args);
    const text = result.content.find((item) => item.type === "text")?.text;
    if (result.isError) throw new Error(text ?? `${DASHBOARD_INBOX_DRAIN_TOOL} failed`);
    if (!text) return [];
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      return [];
    }
    const messages = (parsed as { messages?: unknown } | null)?.messages;
    if (!Array.isArray(messages)) return [];
    return messages.flatMap((message) => {
      const body = (message as { text?: unknown } | null)?.text;
      return typeof body === "string" && body.trim() !== "" ? [body] : [];
    });
  };
}

/**
 * Resolve the caller's own registered named-inbox address (`"name@scope"`)
 * for arming `mailbox wait --slug`, via `mailbox.lookup_peers`'s `self.address`
 * field — never `process.env.WS_MAILBOX`/`WS_MAILBOX_AUTO` (see this file's
 * top doc comment on why a freshly-spawned CLI process cannot re-derive an
 * auto-minted stem). `scope` is a required argument on the tool but — per
 * `mailbox.lookup_peers`'s server-side implementation — `self.address` is
 * populated from the caller's own resolved identity regardless of which
 * scope is queried, so any valid scope value reaches it; `"worktree"` is
 * passed as an arbitrary fixed choice, not a claim about the session's actual
 * scope.
 *
 * Best-effort by design (Decision: "an env-less session ... keep today's
 * reply-id-only behavior"): a tool-level error, a transport failure, a
 * malformed response, or a genuinely absent `self.address` (no owned named
 * inbox) all resolve to `undefined` rather than throwing, so a resolution
 * failure can never block arming — the caller falls back to exactly the
 * reply-id-only wait it would have armed before this existed.
 */
export async function resolveMailboxSelfSlug(callTool: MailboxToolCall, sessionKey: string): Promise<string | undefined> {
  try {
    const result = await callTool("mailbox.lookup_peers", { session_key: sessionKey, scope: "worktree", format: "json" });
    if (result.isError) return undefined;
    const text = result.content.find((item) => item.type === "text")?.text;
    if (!text) return undefined;
    let parsed: unknown;
    try {
      parsed = JSON.parse(text);
    } catch {
      return undefined;
    }
    if (!parsed || typeof parsed !== "object") return undefined;
    const self = (parsed as { self?: unknown }).self;
    if (!self || typeof self !== "object") return undefined;
    const address = (self as { address?: unknown }).address;
    return typeof address === "string" && address.trim() ? address.trim() : undefined;
  } catch {
    return undefined;
  }
}
