import { test, describe, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { stripSummaryId } from "./fixtures/summary-id.ts";
import {
  buildPushContent,
  computeRunningStatusLine,
  hasRunningAgents,
  flushHeldPushes,
  heldPushQueue,
  leadCompactingRef,
  leadIdleRef,
  leadWakeStartPendingRef,
  isLeadInterrupted,
  markLeadInterrupted,
  pushSpawnFailed,
  pushToLead,
  registerPushFlush,
  shouldPushToLead,
  type RpcAgentRegistry,
  type TerminalDelivery,
} from "../src/spawner.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { join } from "node:path";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { installSubtreePublisher, SubtreeUpstream } from "../src/subtree-lifecycle.ts";
import { fakeUplink, idleOwnTurn } from "./fixtures/subtree-channels.ts";
import { freshRpcRecord, liveRpcRecord, fakePi } from "./fixtures/spawner-records.ts";

/** One event-loop turn: lets the zero-delay settle-time wake timer run. */
const macrotask = (): Promise<void> => new Promise((resolve) => setTimeout(resolve, 0));

describe("shouldPushToLead (the push gate)", () => {
  test("lead, fork, worker, and persistent Explore owners push to their direct parent", () => {
    assert.equal(shouldPushToLead({}), true, "no marker = host lead");
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "fork" }), true);
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "worker" }), true);
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "explore" }), true);
  });

  test("an unrecognized role value is treated as no marker (host lead), same as readSpawnRole", () => {
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "bogus" }), true);
  });
});

describe("computeRunningStatusLine (execution-only fan-in count)", () => {
  test("counts only running or streaming records", () => {
    const registry = new Map([
      ["run", freshRpcRecord({ agentId: "run", running: true })],
      ["stream", freshRpcRecord({ agentId: "stream", streaming: true })],
      ["owner", freshRpcRecord({ agentId: "owner", threadBound: true, lastWriter: "owner" })],
      ["delivery", freshRpcRecord({ agentId: "delivery", terminalDelivery: { generation: 1, family: "ws-agent-settled", payload: {}, state: "held" } })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "2 delegated agents still running");
    registry.get("run")!.running = false;
    registry.get("stream")!.streaming = false;
    assert.equal(computeRunningStatusLine(registry), "0 delegated agents still running");
  });
});

describe("hasRunningAgents (goal-loop yield predicate)", () => {
  test("tracks only autonomous execution", () => {
    const r = freshRpcRecord({ running: true, waitingOnChildren: true });
    const registry = new Map([[r.agentId, r]]);
    assert.equal(hasRunningAgents(registry), true);
    r.running = false;
    assert.equal(hasRunningAgents(registry), false);
    r.streaming = true;
    assert.equal(hasRunningAgents(registry), true);
  });
});

describe("buildPushContent", () => {
  test("renders a head line naming the family and agent, one key: value line per payload field, and the status line last", () => {
    const content = buildPushContent("ws-agent-report", "a1", { report: "done" }, "0 delegated agents still running");
    assert.equal(content, ["[ws-agent-report] agent a1", "report: done", "0 delegated agents still running"].join("\n"));
  });

  test("an agent-less family (ws-agent-orphaned) drops the agent from the head line", () => {
    const content = buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, "0 delegated agents still running");
    assert.equal(content, ["[ws-agent-orphaned]", "count: 2", "0 delegated agents still running"].join("\n"));
  });

  test("Edition: an absent status line contributes no trailing line at all", () => {
    const content = buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, undefined);
    assert.equal(content, ["[ws-agent-orphaned]", "count: 2"].join("\n"));
  });

  test("undefined/null/empty payload fields are omitted rather than rendered as blanks", () => {
    const content = buildPushContent("ws-agent-settled", "a1", { reason: "idle", last_message: undefined, error: "" }, "status");
    assert.equal(content, ["[ws-agent-settled] agent a1", "reason: idle", "status"].join("\n"));
  });

  test("a non-string value is JSON-stringified so an object payload never renders as [object Object]", () => {
    const content = buildPushContent("ws-agent-advisory", "a1", { detail: { a: 1 } }, "status");
    assert.match(content, /detail: \{"a":1\}/);
  });
});

describe("pushToLead", () => {
  // 260906 Phase 1 (settle-timer reminder race ticket): module state shared
  // with goal-loop.ts's settle timer, mirroring the existing
  // `leadCompactingRef`/`heldPushQueue` reset convention elsewhere in this
  // file.
  beforeEach(() => {
    leadWakeStartPendingRef.current = false;
    heldPushQueue.length = 0;
  });

  afterEach(() => {
    leadWakeStartPendingRef.current = false;
    heldPushQueue.length = 0;
  });

  test("confirmed-start release retains each structured item with agent_id, payload, and status", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 1);
    const [{ message, options }] = pi.sent;
    assert.equal(message.customType, "ws-agent-report");
    assert.equal(message.display, true);
    assert.deepEqual(stripSummaryId(message.details), { agent_id: "a", report: "halfway", status: "1 delegated agent still running" });
    assert.deepEqual(options, { deliverAs: "steer", triggerTurn: true }, "confirmed start steers the held idle report before its first response");
  });

  test("an absent record still pushes (the orphan roll-call), and with nothing delegated it carries NO status field", () => {
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 2 }, "followUp");
    assert.deepEqual(stripSummaryId(pi.sent[0].message.details), { count: 2 }, "Edition: a zero line with nothing delegated told the lead nothing");
    assert.equal(pi.sent[0].message.content, ["[ws-agent-orphaned]", "count: 2"].join("\n"));
  });

  test("no pi (a resume driven from a call site with no push channel) is a silent no-op, not a throw", () => {
    assert.doesNotThrow(() => pushToLead(undefined, new Map(), undefined, "ws-agent-report", { report: "x" }, "followUp"));
  });

  test("a throwing sendMessage is swallowed — a torn-down session must not crash a child's event listener", () => {
    const pi = fakePi({
      sendMessage: () => {
        throw new Error("session is gone");
      },
    });
    assert.doesNotThrow(() => pushToLead(pi.api, new Map(), undefined, "ws-agent-report", { report: "x" }, "followUp"));
    assert.equal(heldPushQueue.length, 1, "synchronous batch rejection retains the item for fallback delivery");
  });

  test("a worker process receives child reports through the real push path", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const previous = process.env[WS_PI_SPAWN_ROLE_ENV];
    process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
    try {
      pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    } finally {
      if (previous === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV];
      else process.env[WS_PI_SPAWN_ROLE_ENV] = previous;
    }
    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.details?.report, "halfway");
  });

  test("the same call from the host lead (no role marker) does push — the gate is the role, not the arguments", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    assert.equal(pi.sent.length, 1);
  });

  test("260905 (alias/park/cap): the pushed message HEAD prints the alias, followed by the uuid — details.agent_id stays the bare uuid", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a1", alias: "scout", running: true });
    const registry: RpcAgentRegistry = new Map([["a1", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent[0].message.content?.split("\n")[0], "[ws-agent-report] agent scout (a1)");
    assert.deepEqual(stripSummaryId(pi.sent[0].message.details), { agent_id: "a1", report: "halfway", status: "1 delegated agent still running" });
  });

  test("260905 (alias/park/cap): with no alias, the head is unchanged (bare uuid)", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a1", running: true });
    const registry: RpcAgentRegistry = new Map([["a1", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent[0].message.content?.split("\n")[0], "[ws-agent-report] agent a1");
  });

  test("Phase 2: a pending reminder holds pushes until confirmed start", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    leadWakeStartPendingRef.current = true;

    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 1, "held until confirmed streaming start");
    leadWakeStartPendingRef.current = false;
    flushHeldPushes(pi.api, true);
    assert.deepEqual(pi.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
  });

  test("a terminal delivery becomes held only on FIFO admission and enqueued only on actual release", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "terminal", running: true });
    const terminal: TerminalDelivery = {};
    leadWakeStartPendingRef.current = true;

    pushToLead(pi.api, new Map([[record.agentId, record]]), record, "ws-agent-report", { report: "done" }, "followUp", terminal);
    assert.equal(terminal.state, "held");
    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 1);

    leadWakeStartPendingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(terminal.state, "enqueued");
    assert.equal(pi.sent.length, 1);
    assert.equal(flushHeldPushes(pi.api, true), 0, "the retained event is not admitted a second time");
  });

  test("skipped or rejected terminal sends never claim FIFO admission", () => {
    const skipped: TerminalDelivery = {};
    const pi = fakePi();
    const saved = leadIdleRef.current;
    leadIdleRef.current = undefined;
    try {
      pushToLead(pi.api, new Map(), undefined, "ws-agent-advisory", { advisory: "missing-final" }, "followUp", skipped);
    } finally {
      leadIdleRef.current = saved;
    }
    assert.equal(skipped.state, undefined);

    const rejected: TerminalDelivery = {};
    const broken = fakePi({ sendMessage: () => { throw new Error("gone"); } });
    const beforeRejected = leadIdleRef.current;
    leadIdleRef.current = () => false;
    try {
      pushToLead(broken.api, new Map(), undefined, "ws-agent-advisory", { advisory: "missing-final" }, "steer", rejected);
    } finally {
      leadIdleRef.current = beforeRejected;
    }
    assert.equal(rejected.state, undefined);
  });

  test("confirmed-start release retains triggerTurn: true when no reminder start is pending", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    leadWakeStartPendingRef.current = false;

    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 1);
    assert.deepEqual(pi.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
  });
});

/**
 * Phase 1 Edition (live-run fix): a `followUp` push raised while the owning
 * session is MID-TURN is held and released on that turn's `agent_settled`,
 * with its status line computed at RELEASE time.
 *
 * What went wrong without this: Pi queues a mid-turn `followUp` in its own
 * `PendingMessageQueue` and delivers it after the turn, but offers no hook at
 * that delivery — so the status line was frozen at arrival time. A worker that
 * finished while the lead was still spawning its siblings delivered a zero
 * count (its siblings were not registered yet), and so did the next: three
 * separate invitations to synthesize before the fan-out was in.
 *
 * `leadIdleRef` and `heldPushQueue` are module state, so every test here
 * resets both.
 */
describe("pushToLead: holding a mid-turn push until the lead's turn settles", () => {
  let idle = true;

  beforeEach(() => {
    idle = true;
    heldPushQueue.length = 0;
    leadIdleRef.current = () => idle;
    leadCompactingRef.current = false;
  });

  afterEach(() => {
    heldPushQueue.length = 0;
    leadIdleRef.current = undefined;
    leadCompactingRef.current = false;
  });

  test("an IDLE lead receives its push after the fake user wake confirms start", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, []);
  });

  test("a MID-TURN lead is not pushed to at all until the turn settles", () => {
    idle = false;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    assert.deepEqual(pi.sent, []);
    assert.equal(heldPushQueue.length, 1);

    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, [], "the queue is drained, so a second settle re-sends nothing");
  });

  test("no idleness accessor means no send into an uninitialized or torn-down session", () => {
    const pi = fakePi();
    leadIdleRef.current = undefined;
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 0, "uninitialized or torn-down sessions never start custom runs");
  });

  test("a throwing idleness accessor is a torn-down session: no send or hold", () => {
    leadIdleRef.current = () => {
      throw new Error("ctx is gone");
    };
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 0);
  });

  test("steer families (an approval, a headless question) bypass the hold — interrupting is their whole purpose", () => {
    idle = false;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "c1" }, "steer");
    pushToLead(pi.api, registry, record, "ws-agent-question", { question: "which anchor?" }, "steer");
    assert.deepEqual(
      pi.sent.map((entry) => entry.message.customType),
      ["ws-agent-approval", "ws-agent-question"],
    );
    assert.deepEqual(heldPushQueue, [], "a blocked child cannot wait for the lead's turn to end");
  });

  test("260906 (Phase 1): a steer push is held while a compaction is in flight, even on an otherwise-idle lead", () => {
    leadCompactingRef.current = true;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "c1" }, "steer");
    assert.deepEqual(pi.sent, [], "sendCustomMessage bypasses Pi's own compaction guard — this hold is the only thing stopping it");
    assert.equal(heldPushQueue.length, 1);

    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent[0]!.options?.deliverAs, "steer", "confirmed-start steering preserves a held steer");
  });

  test("260906 (Phase 1): a steer push mid-turn but NOT compacting still bypasses the hold as before", () => {
    idle = false;
    leadCompactingRef.current = false;
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-approval", { cmd_id: "c1" }, "steer");
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, []);
  });

  test("260906 (Phase 1): a followUp push is held while compacting even when leadIdleRef itself reports idle", () => {
    leadCompactingRef.current = true;
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-report", { report: "mid-compaction" }, "followUp");
    assert.deepEqual(pi.sent, []);
    assert.equal(heldPushQueue.length, 1);

    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent[0]!.options?.deliverAs, "steer", "confirmed-start release upgrades held followUp to steering");
  });

  test("260906 (Phase 1): registerPushFlush's agent_settled handler does not flush while compacting", async () => {
    leadCompactingRef.current = true;
    const sent: string[] = [];
    let settled: (() => void) | undefined;
    let start: (() => void) | undefined;
    const api = {
      on: (event: string, handler: () => void) => { if (event === "agent_settled") settled = handler; if (event === "agent_start") start = handler; },
      sendUserMessage: () => start?.(),
      sendMessage: (message: { customType?: string }) => void sent.push(message.customType ?? ""),
    } as unknown as Parameters<typeof registerPushFlush>[0];
    registerPushFlush(api, { delayMs: () => 10 });

    heldPushQueue.push({ kind: "push", registry: undefined, record: undefined, family: "ws-agent-report", payload: { report: "held" }, deliverAs: "followUp" });
    settled?.();
    await macrotask();
    assert.deepEqual(sent, [], "the abort inside ctx.compact() settles the doomed turn before Pi's own compaction flag is set — this gate is what stops a premature flush into it");
    assert.equal(heldPushQueue.length, 1, "left for releaseAfterCompaction to flush once the compaction actually finishes");

    leadCompactingRef.current = false;
    settled?.();
    await macrotask();
    assert.deepEqual(sent, [PUSH_BATCH_CUSTOM_TYPE], "an ordinary settle once compaction is over flushes one batch normally");
  });

  test("the live-run failure itself: three workers, two finals landing mid-turn, read 1 then 0 — never a premature 0", () => {
    idle = false;
    const pi = fakePi();
    const ids = ["w1", "w2", "w3"];
    const registry: RpcAgentRegistry = new Map(ids.map((id) => [id, liveRpcRecord({ agentId: id, running: true })] as const));

    // The lead is still mid-turn (it is spawning w3) when w1 and w2 finish.
    for (const id of ["w1", "w2"]) {
      const record = registry.get(id)!;
      record.running = false;
      pushToLead(pi.api, registry, record, "ws-agent-report", { report: `Outcome: ${id}` }, "followUp");
    }
    assert.deepEqual(pi.sent, [], "nothing is delivered while the lead is mid-turn");

    // w3 is still working when the lead's turn ends: the held pair is released
    // now, against the registry as it stands at THIS instant.
    idle = true;
    assert.equal(flushHeldPushes(pi.api, true), 2);
    assert.deepEqual(
      pi.sent.map((entry) => (entry.message.details as { status?: string }).status),
      ["1 delegated agent still running", "1 delegated agent still running"],
      "at release time one worker is genuinely still out — arrival-time lines said zero",
    );
    assert.deepEqual(
      pi.sent.map((entry) => (entry.message.details as { report?: string }).report),
      ["Outcome: w1", "Outcome: w2"],
      "arrival order is preserved",
    );

    // w3 finishes during the run those two started.
    const w3 = registry.get("w3")!;
    w3.running = false;
    idle = false;
    pushToLead(pi.api, registry, w3, "ws-agent-report", { report: "Outcome: w3" }, "followUp");
    idle = true;
    flushHeldPushes(pi.api, true);

    assert.equal(
      (pi.sent[2].message.details as { status?: string }).status,
      "0 delegated agents still running",
      "the synthesis cue lands on the message that actually completes the fan-out",
    );
  });

  test("a push issued from inside the flush is held for the NEXT settle, not drained re-entrantly", () => {
    idle = false;
    const registry: RpcAgentRegistry = new Map();
    const reentrant = fakePi({
      sendMessage: () => {
        // The flush's first send starts a lead run, so the session is busy
        // again for everything that follows.
        idle = false;
        pushToLead(reentrant.api, registry, undefined, "ws-agent-advisory", { advisory: "raised during the run" }, "followUp");
      },
    });
    pushToLead(reentrant.api, registry, undefined, "ws-agent-report", { report: "first" }, "followUp");

    idle = true;
    assert.equal(flushHeldPushes(reentrant.api, true), 1);
    assert.equal(heldPushQueue.length, 1, "the re-entrant push waits for the next settle rather than joining this drain");
  });

  test("registerPushFlush requests a wake on settle and releases at confirmed start", async () => {
    idle = false;
    const sent: string[] = [];
    let settled: (() => void) | undefined;
    let start: (() => void) | undefined;
    const api = {
      on: (event: string, handler: () => void) => { if (event === "agent_settled") settled = handler; if (event === "agent_start") start = handler; },
      sendUserMessage: () => start?.(),
      sendMessage: (message: { customType?: string }) => void sent.push(message.customType ?? ""),
    } as unknown as Parameters<typeof registerPushFlush>[0];
    registerPushFlush(api, { delayMs: () => 10 });

    pushToLead(api as never, new Map(), undefined, "ws-agent-report", { report: "held" }, "followUp");
    assert.deepEqual(sent, []);

    idle = true;
    settled?.();
    assert.deepEqual(sent, [], "the settle-time wake waits for a macrotask, never inside the settle emission");
    await macrotask();
    assert.deepEqual(sent, [PUSH_BATCH_CUSTOM_TYPE]);
  });
});

describe("a dashboard interrupt of the lead's run", () => {
  type Sent = { message: { customType?: string; details?: { report?: string; items?: Array<{ details?: { report?: string } }> } }; options?: { deliverAs?: string; triggerTurn?: boolean } };
  let shutdown: (() => void) | undefined;

  /**
   * The owner lead's push lifecycle against a minimal Pi: `start`, `end`, and
   * `settle` emit the run events, `take` emits the `message_start` Pi gives a
   * custom message it injects (its own copy with `role: "custom"`, holding
   * the very `details` passed to `sendMessage`), `prompted` the one for a
   * user prompt, and `runTimers` lets the pending deferred flushes and wake
   * timers fire once.
   */
  function harness(startIdle = false) {
    const handlers = new Map<string, Array<(event?: unknown) => void>>();
    const timers = new Map<number, () => void>();
    let nextTimer = 0;
    let idle = startIdle;
    const users: unknown[] = [];
    const sent: Sent[] = [];
    const api = {
      on: (event: string, fn: (event?: unknown) => void) => { handlers.set(event, [...(handlers.get(event) ?? []), fn]); },
      sendUserMessage: (content: unknown) => { users.push(content); },
      sendMessage: (message: unknown, options?: unknown) => { sent.push({ message: message as never, options: options as never }); },
    };
    leadIdleRef.current = () => idle;
    registerPushFlush(api as never, {
      delayMs: () => 10,
      scheduleTimer: (cb) => { timers.set(++nextTimer, cb); return nextTimer as never; },
      clearTimer: (handle) => { timers.delete(handle as unknown as number); },
    });
    const emit = (event: string, payload?: unknown): void => { for (const fn of handlers.get(event) ?? []) fn(payload); };
    shutdown = () => emit("session_shutdown");
    return {
      users,
      sent,
      start: () => { idle = false; emit("agent_start"); },
      take: (message: Sent["message"]) => emit("message_start", { message: { role: "custom", ...message, timestamp: 0 } }),
      prompted: (content: unknown) => emit("message_start", { message: { role: "user", content, timestamp: 0 } }),
      end: () => emit("agent_end"),
      settle: () => { idle = true; emit("agent_settled"); },
      runTimers: () => { for (const [key, cb] of [...timers]) { timers.delete(key); cb(); } },
      push: (deliverAs: "steer" | "followUp", report: string, terminal?: TerminalDelivery) =>
        pushToLead(api as never, new Map(), undefined, "ws-agent-report", { report }, deliverAs, terminal),
    };
  }
  const reports = (entry: Sent): Array<string | undefined> => (entry.message.details?.items ?? []).map((item) => item.details?.report);

  beforeEach(() => {
    heldPushQueue.length = 0;
    leadCompactingRef.current = false;
    leadWakeStartPendingRef.current = false;
  });

  afterEach(() => {
    shutdown?.();
    shutdown = undefined;
    heldPushQueue.length = 0;
    leadIdleRef.current = undefined;
    leadWakeStartPendingRef.current = false;
  });

  test("held FIFO items, a steer arriving after the abort among them, wait for the next prompt as one nextTurn batch, with their terminal bookkeeping", () => {
    const h = harness();
    h.start();
    const calls: string[] = [];
    const terminal = { afterEnqueue: () => calls.push("after") } as unknown as TerminalDelivery;
    h.push("followUp", "held before", terminal);
    markLeadInterrupted();
    h.push("steer", "steered after");
    assert.deepEqual(h.sent, [], "a steer into a run being cut would be dropped with Pi's queues");
    assert.equal(heldPushQueue.length, 2);
    h.end();
    h.settle();
    assert.equal(h.sent.length, 1);
    assert.equal(h.sent[0]!.message.customType, PUSH_BATCH_CUSTOM_TYPE);
    assert.deepEqual(h.sent[0]!.options, { deliverAs: "nextTurn" });
    assert.deepEqual(reports(h.sent[0]!), ["held before", "steered after"], "arrival order is kept");
    assert.deepEqual(heldPushQueue, []);
    assert.equal(terminal.state, "enqueued");
    assert.deepEqual(calls, ["after"]);
  });

  test("no agent_end boundary batch and no wake line starts a turn after the interrupt; a push admitted after the settle behaves as usual", () => {
    const h = harness();
    h.start();
    h.push("followUp", "first");
    markLeadInterrupted();
    h.end();
    assert.deepEqual(h.sent, [], "a boundary batch would be taken by a continuation run");
    h.settle();
    h.runTimers();
    assert.deepEqual(h.users, [], "nothing wakes the lead for what the cut run left");
    assert.deepEqual(h.sent.map((entry) => entry.options?.deliverAs), ["nextTurn"]);
    h.push("followUp", "after the settle");
    assert.equal(h.users.length, 1, "the next push wakes the idle lead, and that prompt carries the nextTurn batch");
    assert.equal(heldPushQueue.length, 1);
  });

  test("pushes steered into the run that it was not seen taking are re-sent whole as nextTurn at its settle; a taken one is not", () => {
    const h = harness();
    h.start();
    h.push("steer", "taken");
    h.push("steer", "untaken");
    assert.deepEqual(h.sent.map((entry) => entry.options), [{ deliverAs: "steer", triggerTurn: true }, { deliverAs: "steer", triggerTurn: true }]);
    h.take(h.sent[0]!.message);
    markLeadInterrupted();
    h.end();
    h.settle();
    assert.equal(h.sent.length, 3);
    assert.strictEqual(h.sent[2]!.message, h.sent[1]!.message, "the very message Pi's abort dropped");
    assert.deepEqual(h.sent[2]!.options, { deliverAs: "nextTurn" });
  });

  test("an untaken batch released at a confirmed start is re-sent whole as nextTurn", () => {
    const h = harness(true);
    h.push("followUp", "one");
    h.push("followUp", "two");
    assert.equal(h.users.length, 1, "the idle lead is woken");
    h.start();
    assert.equal(h.sent.length, 1);
    assert.deepEqual(h.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
    h.prompted(h.users[0]);
    markLeadInterrupted();
    h.end();
    h.settle();
    assert.equal(h.sent.length, 2);
    assert.strictEqual(h.sent[1]!.message, h.sent[0]!.message);
    assert.deepEqual(reports(h.sent[1]!), ["one", "two"]);
    assert.deepEqual(h.sent[1]!.options, { deliverAs: "nextTurn" });
  });

  test("an ordinary settle forgets what its run left untaken, so a later interrupt re-sends none of it", () => {
    const h = harness();
    h.start();
    h.push("steer", "left over");
    h.end();
    h.settle();
    h.runTimers();
    h.start();
    markLeadInterrupted();
    h.end();
    h.settle();
    assert.equal(h.sent.length, 1, "only the original steer");
  });

  test("the next agent_start clears the mark: pushes steer into the new run again", () => {
    const h = harness();
    h.start();
    markLeadInterrupted();
    assert.equal(isLeadInterrupted(), true);
    h.end();
    h.settle();
    assert.equal(isLeadInterrupted(), true, "the mark stands until a new run starts");
    h.start();
    assert.equal(isLeadInterrupted(), false);
    h.push("steer", "into the new run");
    assert.deepEqual(h.sent.at(-1)!.options, { deliverAs: "steer", triggerTurn: true });
  });

  test("a continuation run that starts before the settle (an abort that kept Pi's queues) clears the mark and takes the held FIFO as usual", () => {
    const h = harness();
    h.start();
    h.push("followUp", "held");
    markLeadInterrupted();
    h.end();
    h.start();
    assert.equal(isLeadInterrupted(), false);
    assert.equal(h.sent.length, 1);
    assert.deepEqual(h.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
    assert.deepEqual(reports(h.sent[0]!), ["held"]);
  });
});

/**
 * Review relay #1, test partition C2: the settle-push suppression the ticket
 * names lives HERE, at the IO boundary, so it needs direct coverage. Driven
 * with a duck-typed client
 * (`onEvent`/`getState`/`getLastAssistantText`); never a real `RpcClient`.
 */
describe("pushSpawnFailed (spawnAgent's launch-failure branch)", () => {
  test("parks the half-registered record and pushes ws-agent-settled reason:spawn-failed with the error text", () => {
    const pi = fakePi();
    let unsubscribed = 0;
    const record = liveRpcRecord({ agentId: "a", running: true, streaming: true, unsubscribe: () => void (unsubscribed += 1) });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushSpawnFailed(pi.api, registry, record, new Error("spawn ENOENT"));

    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.customType, "ws-agent-settled");
    assert.deepEqual(
      stripSummaryId(pi.sent[0].message.details),
      { agent_id: "a", failed: true, reason: "spawn-failed", error: "spawn ENOENT", status: "0 delegated agents still running" },
      "260905 (alias/park/cap): the failed record stays registered (dormant), so presence — keyed on registry membership, not a live client — keeps the status line at zero",
    );
    assert.equal(record.client, undefined, "a failed spawn leaves no live client behind");
    assert.equal(record.running, false, "and stops counting toward the fan-in immediately");
    assert.equal(unsubscribed, 1);
  });

  test("cleanup publication with the upstream channel down preserves the primary spawn error", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "cleanup-failure", running: true, streaming: true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    const uplink = fakeUplink();
    installSubtreePublisher(registry, new SubtreeUpstream(uplink.channel), () => 0, idleOwnTurn);
    uplink.disconnect();
    const primary = new Error("primary spawn failure");
    assert.doesNotThrow(() => pushSpawnFailed(pi.api, registry, record, primary));
    assert.equal((pi.sent[0].message.details as { error?: string }).error, primary.message);
  });

  test("a non-Error throw is stringified rather than dropped", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a" });
    pushSpawnFailed(pi.api, new Map(), record, "boom");
    assert.equal((pi.sent[0].message.details as { error?: string }).error, "boom");
  });

  test("a failed persistent explore stays registered as a dormant record", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", spawnRole: "explore" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushSpawnFailed(pi.api, registry, record, new Error("client.start() failed"));

    assert.equal(pi.sent.length, 1, "the lead still learns the explore failed");
    assert.equal(pi.sent[0].message.details && (pi.sent[0].message.details as { reason?: string }).reason, "spawn-failed");
    assert.equal(registry.has("a"), true, "persistent explore failures retain a resumable registry record");
    assert.equal(record.client, undefined);
  });

  test("a persistent record's launch failure still parks dormant", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushSpawnFailed(pi.api, registry, record, new Error("client.start() failed"));

    assert.equal(registry.has("a"), true, "an ordinary spawn failure keeps parking as today");
    assert.equal(registry.get("a")?.client, undefined);
  });
});
