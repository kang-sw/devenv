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
 *   child  -> parent  hello.resume.approval = {pending?: cmd_id, consumed: cmd_id[]}
 *                     (in every hello; acted on for reconnect hellos)
 *
 * The child is the authority on consumption. A decision counts as consumed
 * only on positive evidence: its acknowledgment arrives, or the child's
 * reconnect hello lists the `cmd_id` in `consumed`. A `cmd_id` the hello
 * reports neither as pending nor as consumed was never consumed, so the
 * parent asks the lead again. Only a hello with no `consumed` array (a child
 * from before that field) keeps the older reading, where "not reported as
 * pending" counts as consumed. There is no durable "started" marker
 * anywhere: if the child process dies, its pending tool call dies with it, so
 * nothing can re-execute the command.
 *
 * This module holds the message shapes and the child-side wait; the
 * parent-side reconciliation (discard on disconnect, release or re-ask on a
 * reconnect hello) is `spawner.ts`'s `attachApprovalChannel`, and
 * `ws-approve` (execute-gateway.ts) is the sender. It imports nothing from
 * either so both can import it.
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

function approvalResumeSection(resume: Record<string, unknown> | undefined): Record<string, unknown> | undefined {
  const section = resume?.[APPROVAL_RESUME_KEY];
  return section && typeof section === "object" && !Array.isArray(section) ? section as Record<string, unknown> : undefined;
}

/** The pending `cmd_id` a hello's resume section reports, or `undefined` when it reports none. */
export function pendingApprovalFromResume(resume: Record<string, unknown> | undefined): string | undefined {
  const pending = approvalResumeSection(resume)?.pending;
  return typeof pending === "string" && pending ? pending : undefined;
}

/**
 * The recently consumed `cmd_id`s a hello's resume section reports, or
 * `undefined` when it carries no `consumed` array: a child from before that
 * field, whose hello the parent reads the older way (`attachApprovalChannel`).
 * An empty array is positive "nothing consumed", not absence.
 */
export function consumedApprovalsFromResume(resume: Record<string, unknown> | undefined): string[] | undefined {
  const consumed = approvalResumeSection(resume)?.consumed;
  if (!Array.isArray(consumed)) return undefined;
  return consumed.filter((cmdId): cmdId is string => typeof cmdId === "string" && cmdId !== "");
}

/** What the child-side wait needs from its channel; `ChildChannel` satisfies it. */
export interface ApprovalChildLink {
  send(msg: Record<string, unknown>): void;
  onMessage(cb: (msg: Record<string, unknown>) => void): () => void;
}

/** What `ChildApprovalGate.attach` needs: the messages, and the end of the connection they arrived on. `ChildChannel` satisfies it. */
export interface ApprovalChildConnection {
  onMessage(cb: (msg: Record<string, unknown>) => void): () => void;
  onDisconnect(cb: () => void): () => void;
}

/**
 * The child side of the decision hand-off. One gate per child process, shared
 * by every `ws-worker-exec` call: it knows which `cmd_id` is still waiting and
 * which were recently consumed, so the channel's hello can report both
 * (`resume()`), and it performs the wait itself.
 *
 * Consumption is acknowledged *before* the command starts. When that send
 * throws, the connection has already ended: the decision is discarded here,
 * the `cmd_id` stays pending, and the next hello reports it, at which point
 * the parent asks the user afresh. Only a decision sent over that new
 * connection can then be consumed. A decision for any other `cmd_id` is
 * ignored by this wait, so one command's approval can never satisfy another.
 *
 * The parent learns the `cmd_id` from Pi's `tool_execution_start`, which Pi
 * emits before `execute()` runs, so a fast decision can reach this process
 * before the wait exists. `attach` keeps such early decisions (bounded, one
 * per `cmd_id`) for the wait that follows, but only while the connection they
 * arrived on lasts: its end discards them, because the reconnect hello cannot
 * list them as consumed and the parent will ask the lead again.
 */
export class ChildApprovalGate {
  /**
   * Per-child bound applied separately to both retained collections: the
   * early decisions kept for a wait that has not started yet (`early`), and
   * the consumed `cmd_id`s a hello reports (`consumed`). Each drops its oldest
   * entry first.
   */
  static readonly RETAINED_DECISION_CAP = 16;
  /**
   * Waiting `cmd_id`s in wait order. The parent tracks one pending request
   * per child and the hello reports one, so only the latest is reported when
   * several wait (a parallel batch of gated calls is a pre-existing limit).
   */
  private readonly waiting: string[] = [];
  /** Decisions that arrived on the current connection before their wait, by `cmd_id`. */
  private readonly early = new Map<string, ApprovalDecision>();
  /** `cmd_id`s whose acknowledgment was sent, oldest first; the parent's positive consumption evidence. */
  private readonly consumed: string[] = [];

  /** The `cmd_id` still waiting for a decision, if any (the latest, when several wait). */
  get pending(): string | undefined { return this.waiting.at(-1); }

  /**
   * The hello resume contribution: always `{approval: {consumed}}`, even with
   * nothing consumed, plus `pending` while a wait is open. Omitting an empty
   * section would read as a pre-`consumed` child to the parent.
   */
  resume(): Record<string, unknown> {
    const pending = this.pending;
    return { [APPROVAL_RESUME_KEY]: { ...(pending ? { pending } : {}), consumed: [...this.consumed] } };
  }

  /**
   * Keeps decisions that arrive for a `cmd_id` with no open wait until
   * `waitForDecision` asks for them, and discards them all when the
   * connection they arrived on ends. Returns the detach.
   */
  attach(link: ApprovalChildConnection): () => void {
    const offMessage = link.onMessage((msg) => {
      const parsed = parseApprovalDecisionMessage(msg);
      if (!parsed || this.waiting.includes(parsed.cmdId)) return;
      this.early.delete(parsed.cmdId);
      this.early.set(parsed.cmdId, parsed.decision);
      while (this.early.size > ChildApprovalGate.RETAINED_DECISION_CAP) this.early.delete(this.early.keys().next().value!);
    });
    const offDisconnect = link.onDisconnect(() => { this.early.clear(); });
    return () => { offMessage(); offDisconnect(); };
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
      if (signal?.aborted) { this.early.delete(cmdId); resolve("aborted"); return; }
      // Consumption is acknowledged before the command starts; a send that
      // throws means the connection is gone: not consumed, still pending,
      // reported by the next hello, and absent from `consumed`.
      const consume = (decision: ApprovalDecision): boolean => {
        try { link.send(approvalConsumedMessage(cmdId)); } catch { return false; }
        this.consumed.push(cmdId);
        while (this.consumed.length > ChildApprovalGate.RETAINED_DECISION_CAP) this.consumed.shift();
        finish(decision);
        return true;
      };
      const earlyDecision = this.early.get(cmdId);
      this.early.delete(cmdId);
      this.waiting.push(cmdId);
      if (earlyDecision && consume(earlyDecision)) return;
      offMessage = link.onMessage((msg) => {
        if (settled) return;
        const parsed = parseApprovalDecisionMessage(msg);
        if (parsed && parsed.cmdId === cmdId) consume(parsed.decision);
      });
      signal?.addEventListener("abort", onAbort);
    });
  }
}
