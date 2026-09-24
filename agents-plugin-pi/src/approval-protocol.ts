/**
 * Execute-approval decisions over the parent-child control channel
 * (260924-feat-pi-agent-channel-approval-decisions).
 *
 * The approval *request* still travels child-to-parent on the Pi RPC
 * `tool_execution_start` event (`spawner.ts`'s `applyRpcEvent`). The
 * *decision* comes back over the launch's control channel (`agent-channel.ts`)
 * as a message bound to its `cmd_id`, and the child answers it with a
 * consumption acknowledgment before it starts the command:
 *
 *   parent -> child   {t: "approval-decision", cmd_id, decision, reason?, command?}
 *   child  -> parent  {t: "approval-consumed", cmd_id}
 *   child  -> parent  hello.resume.approval = {pending: cmd_id}   (reconnect only)
 *
 * The child is the authority on consumption. A decision counts as consumed
 * exactly when its acknowledgment arrives, or when the child's reconnect
 * hello no longer reports that `cmd_id` as pending. There is no durable
 * "started" marker anywhere: if the child process dies, its pending tool call
 * dies with it, so nothing can re-execute the command.
 *
 * This module holds the message shapes and the child-side wait; the
 * parent-side reconciliation (discard on disconnect, re-ask on a reconnect
 * hello that still reports the `cmd_id`) is `spawner.ts`'s
 * `attachApprovalChannel`, and `ws-approve` (execute-gateway.ts) is the
 * sender. It imports nothing from either so both can import it.
 */

/** Parent -> child: the lead's decision for one pending `cmd_id`. */
export const APPROVAL_DECISION_MESSAGE = "approval-decision";
/** Child -> parent: sent for a `cmd_id` before that command starts. */
export const APPROVAL_CONSUMED_MESSAGE = "approval-consumed";
/** Key of the hello resume section the child reports its pending `cmd_id` under. */
export const APPROVAL_RESUME_KEY = "approval";

export interface ApprovalDecision {
  decision: "approve" | "deny" | "run-instead";
  reason?: string;
  command?: string;
}

const DECISIONS: ReadonlySet<string> = new Set(["approve", "deny", "run-instead"]);

export function approvalDecisionMessage(cmdId: string, decision: ApprovalDecision): Record<string, unknown> {
  return {
    t: APPROVAL_DECISION_MESSAGE,
    cmd_id: cmdId,
    decision: decision.decision,
    ...(decision.reason !== undefined ? { reason: decision.reason } : {}),
    ...(decision.command !== undefined ? { command: decision.command } : {}),
  };
}

export function approvalConsumedMessage(cmdId: string): Record<string, unknown> {
  return { t: APPROVAL_CONSUMED_MESSAGE, cmd_id: cmdId };
}

/** The decision carried by a channel message, or `undefined` for any other or malformed message. */
export function parseApprovalDecisionMessage(msg: Record<string, unknown>): { cmdId: string; decision: ApprovalDecision } | undefined {
  if (msg.t !== APPROVAL_DECISION_MESSAGE || typeof msg.cmd_id !== "string" || !msg.cmd_id) return undefined;
  if (typeof msg.decision !== "string" || !DECISIONS.has(msg.decision)) return undefined;
  return {
    cmdId: msg.cmd_id,
    decision: {
      decision: msg.decision as ApprovalDecision["decision"],
      ...(typeof msg.reason === "string" ? { reason: msg.reason } : {}),
      ...(typeof msg.command === "string" ? { command: msg.command } : {}),
    },
  };
}

/** The `cmd_id` a consumption acknowledgment names, or `undefined` for any other or malformed message. */
export function parseApprovalConsumedMessage(msg: Record<string, unknown>): string | undefined {
  return msg.t === APPROVAL_CONSUMED_MESSAGE && typeof msg.cmd_id === "string" && msg.cmd_id ? msg.cmd_id : undefined;
}

/** The pending `cmd_id` a hello's resume section reports, or `undefined` when it reports none. */
export function pendingApprovalFromResume(resume: Record<string, unknown> | undefined): string | undefined {
  const section = resume?.[APPROVAL_RESUME_KEY];
  if (!section || typeof section !== "object" || Array.isArray(section)) return undefined;
  const pending = (section as { pending?: unknown }).pending;
  return typeof pending === "string" && pending ? pending : undefined;
}

/** What the child-side wait needs from its channel; `ChildChannel` satisfies it. */
export interface ApprovalChildLink {
  send(msg: Record<string, unknown>): void;
  onMessage(cb: (msg: Record<string, unknown>) => void): () => void;
}

/**
 * The child side of the decision hand-off. One gate per child process, shared
 * by every `ws-worker-exec` call: it knows which `cmd_id` is still waiting so
 * the channel's reconnect hello can report it (`resume()`), and it performs
 * the wait itself.
 *
 * Consumption is acknowledged *before* the command starts. When that send
 * throws, the connection has already ended: the decision is discarded here,
 * the `cmd_id` stays pending, and the next hello reports it, at which point
 * the parent asks the user afresh. Only a decision sent over that new
 * connection can then be consumed. A decision for any other `cmd_id` is
 * ignored by this wait, so one command's approval can never satisfy another.
 */
export class ChildApprovalGate {
  /** Waiting `cmd_id`s in wait order. Gated exec calls run one at a time, so this holds at most one in practice. */
  private readonly waiting: string[] = [];

  /** The `cmd_id` still waiting for a decision, if any (the latest, when several wait). */
  get pending(): string | undefined { return this.waiting.at(-1); }

  /** The hello resume contribution: `{approval: {pending}}` while a wait is open, nothing otherwise. */
  resume(): Record<string, unknown> {
    const pending = this.pending;
    return pending ? { [APPROVAL_RESUME_KEY]: { pending } } : {};
  }

  /**
   * Resolves with the consumed decision, or `"aborted"` when `signal` fires
   * first (the `ws-agent-stop` -> `client.abort()` path, which aborts the
   * tool call's own signal). Never rejects and never times out: the parent
   * owns the human-decision window.
   */
  waitForDecision(link: ApprovalChildLink, cmdId: string, signal: AbortSignal | undefined): Promise<ApprovalDecision | "aborted"> {
    return new Promise((resolve) => {
      let settled = false;
      let offMessage: (() => void) | undefined;
      const finish = (result: ApprovalDecision | "aborted") => {
        if (settled) return;
        settled = true;
        offMessage?.();
        signal?.removeEventListener("abort", onAbort);
        const index = this.waiting.lastIndexOf(cmdId);
        if (index >= 0) this.waiting.splice(index, 1);
        resolve(result);
      };
      const onAbort = () => finish("aborted");
      if (signal?.aborted) { resolve("aborted"); return; }
      this.waiting.push(cmdId);
      offMessage = link.onMessage((msg) => {
        if (settled) return;
        const parsed = parseApprovalDecisionMessage(msg);
        if (!parsed || parsed.cmdId !== cmdId) return;
        try { link.send(approvalConsumedMessage(cmdId)); }
        catch { return; /* the connection is gone: not consumed, still pending, reported by the next hello */ }
        finish(parsed.decision);
      });
      signal?.addEventListener("abort", onAbort);
    });
  }
}
