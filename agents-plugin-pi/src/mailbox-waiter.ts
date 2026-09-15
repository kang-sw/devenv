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
 * is the authoritative source: on a wake the waiter drains, then admits each
 * drained envelope through the shared push FIFO (spawner's `sendToLead`) as a
 * single informational `ws-mailbox` custom message so it rides the existing
 * `ws-push-batch` alongside every other system message. Draining via recv,
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
 * `Envelope` or contract. Phase 1 arms the wait on the always-available
 * reply-id channel only (`--session-key`, no `--slug`); recv still drains any
 * owned named inbox too, so its mail is pushed opportunistically whenever a
 * reply-id wake fires.
 */

import { spawn } from "node:child_process";
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

/** `mailbox wait`'s result, distilled from its CLI exit code. */
export type MailboxWaitOutcome = "mail" | "timeout" | "stopped" | "error";

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

export interface MailboxWaiterDeps {
  /** Block until mail is present (or the wait times out); resolve with the outcome. */
  runWait: (signal: AbortSignal) => Promise<MailboxWaitOutcome>;
  /** Authoritatively drain (dequeue) the session's mailbox; resolve with what was removed. */
  drainMail: () => Promise<MailboxEnvelope[]>;
  /** Admit one arriving envelope into the live conversation (through the push FIFO). */
  admit: (envelope: MailboxEnvelope) => void;
  /** Delay after an error or a failed drain before re-arming, ms. */
  errorBackoffMs?: number;
  /** Injected sleep, so tests need no real timers. */
  sleep?: (ms: number) => Promise<void>;
  /** Diagnostic sink; defaults to `console.error`. */
  onError?: (message: string) => void;
}

export interface MailboxWaiterHandle {
  /** Stop the loop and abort any in-flight wait. Idempotent. */
  stop: () => void;
  /** Resolves once the loop has fully exited — for deterministic teardown in tests. */
  readonly done: Promise<void>;
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
 */
export function startMailboxWaiter(deps: MailboxWaiterDeps): MailboxWaiterHandle {
  const controller = new AbortController();
  const sleep = deps.sleep ?? defaultSleep;
  const backoffMs = deps.errorBackoffMs ?? DEFAULT_ERROR_BACKOFF_MS;
  const report = (message: string): void => {
    try {
      (deps.onError ?? ((m: string) => console.error(`[ws-mailbox] ${m}`)))(message);
    } catch {
      // A failing diagnostic sink must never break the loop.
    }
  };

  const done = (async () => {
    while (!controller.signal.aborted) {
      let outcome: MailboxWaitOutcome;
      try {
        outcome = await deps.runWait(controller.signal);
      } catch (error) {
        outcome = "error";
        report(`wait failed: ${error instanceof Error ? error.message : String(error)}`);
      }
      if (controller.signal.aborted || outcome === "stopped") break;

      if (outcome === "mail") {
        let drainFailed = false;
        try {
          const envelopes = await deps.drainMail();
          for (const envelope of envelopes) {
            if (controller.signal.aborted) break;
            try {
              deps.admit(envelope);
            } catch (error) {
              report(`admit failed: ${error instanceof Error ? error.message : String(error)}`);
            }
          }
        } catch (error) {
          drainFailed = true;
          report(`drain failed: ${error instanceof Error ? error.message : String(error)}`);
        }
        // A failed drain leaves the mail queued, so an immediate re-arm would
        // wake at once (peek) and hot-spin; back off first.
        if (drainFailed && !controller.signal.aborted) await sleep(backoffMs);
      } else if (outcome === "error" && !controller.signal.aborted) {
        await sleep(backoffMs);
      }
    }
  })();

  return { stop: () => controller.abort(), done };
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
  /** This session's own session key — the required `--session-key`; gives the reply-id queue to watch. */
  sessionKey: string;
  /** `--timeout` value; a finite window self-heals a wedged wait and bounds the listening marker. */
  timeoutArg?: string;
  /** Diagnostic sink for the child's stderr and spawn failures. */
  onStderr?: (line: string) => void;
}

const DEFAULT_WAIT_TIMEOUT_ARG = "10m";

/**
 * The real `runWait`: spawn `python3 <launcher> mailbox wait ...` (the launcher
 * forwards the subcommand verbatim to the resolved `ws-mcp` binary) and map its
 * exit code to an outcome. Its stdout — the peeked mail — is intentionally
 * ignored (`stdio` drops it): the drain, not the peek, is the source of truth.
 */
export function createSubprocessWait(options: SubprocessWaitOptions): (signal: AbortSignal) => Promise<MailboxWaitOutcome> {
  const timeoutArg = options.timeoutArg ?? DEFAULT_WAIT_TIMEOUT_ARG;
  const stderr = options.onStderr ?? ((line: string) => console.error(`[ws-mailbox] ${line}`));
  return (signal) =>
    new Promise<MailboxWaitOutcome>((resolve) => {
      if (signal.aborted) {
        resolve("stopped");
        return;
      }
      const child = spawn(
        "python3",
        [options.launcherPath, "mailbox", "wait", "--session-key", options.sessionKey, "--timeout", timeoutArg, "--format", "json"],
        { cwd: options.pluginDir, stdio: ["ignore", "ignore", "pipe"] },
      );
      let settled = false;
      const finish = (outcome: MailboxWaitOutcome): void => {
        if (settled) return;
        settled = true;
        signal.removeEventListener("abort", onAbort);
        resolve(outcome);
      };
      const onAbort = (): void => {
        try {
          child.kill("SIGTERM");
        } catch {
          // Best effort — the exit handler still resolves.
        }
        finish("stopped");
      };
      signal.addEventListener("abort", onAbort, { once: true });
      child.stderr?.on("data", (chunk: Buffer) => {
        const text = chunk.toString().trimEnd();
        if (text) stderr(text);
      });
      child.on("error", (err) => {
        stderr(`wait spawn failed: ${err.message}`);
        finish("error");
      });
      child.on("exit", (code, sig) => {
        finish(mapMailboxWaitExit(code, sig, signal.aborted));
      });
    });
}

/** Minimal MCP tool-call surface the drain needs — the bridge's connected client satisfies it. */
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
