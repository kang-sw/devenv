/**
 * Unit tests for dashboard-feed.ts: the owner lead's feed into ws-mcp's
 * `dashboard.intake`. Driven with a fake clock and scheduler and a recording
 * `callTool`, so every flush is explicit.
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  AGENT_POLL_MS,
  DASHBOARD_INTAKE_TOOL,
  DASHBOARD_LEAD_EVENTS,
  KNOB_RECHECK_MS,
  FLUSH_INTERVAL_MS,
  INTAKE_CALL_TIMEOUT_MS,
  RETRY_MS,
  createDashboardFeed,
  dashboardAgentEntries,
  dashboardUserMessage,
  inputTurnOrigin,
  registerDashboardLeadEvents,
  resolveDashboardEnabled,
  type DashboardAgentEntry,
  type DashboardEvent,
  type DashboardFeedOptions,
} from "../src/dashboard-feed.ts";
import { ADAPTER_MESSAGE_LABEL, WHITEBOARD_MESSAGE_LABEL, labelAdapterText, labelWhiteboardText, whiteboardLabeledBody } from "../src/adapter-label.ts";
import { createBatchBuffer, observeBatchEvent } from "../src/agent-batches.ts";
import { ADAPTER_ONLY_RAW_TOOLS, filterOutAdapterOnlyTools, filterOutMercenaryTools } from "../src/bridge.ts";
import { markAgentExited, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";

interface Call { name: string; args: { session_key: string; adapter: string; events: DashboardEvent[] } }

/** A fake clock and scheduler: timers run only when `advance` passes them. */
function harness(overrides: Partial<DashboardFeedOptions> & { answer?: (call: Call) => Promise<unknown> } = {}) {
  let clock = 10_000;
  let nextId = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  const calls: Call[] = [];
  const answer = overrides.answer ?? (async () => ({ content: [{ type: "text", text: "ok" }] }));
  const feed = createDashboardFeed({
    target: () => ({ sessionKey: "lead-key-1", callTool: (name, args) => { const call = { name, args: args as Call["args"] }; calls.push(call); return answer(call); } }),
    enabled: () => true,
    now: () => clock,
    schedule: (fn, ms) => {
      const id = nextId++;
      timers.set(id, { at: clock + ms, fn });
      return () => { timers.delete(id); };
    },
    ...overrides,
  });
  /** Settles pending promise callbacks (the knob read and the call). */
  const settle = async () => { for (let i = 0; i < 5; i++) await new Promise((resolve) => setImmediate(resolve)); };
  async function advance(ms: number): Promise<void> {
    const until = clock + ms;
    for (;;) {
      await settle();
      const due = [...timers].filter(([, timer]) => timer.at <= until).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      timers.delete(due[0]);
      clock = Math.max(clock, due[1].at);
      due[1].fn();
    }
    clock = until;
    await settle();
  }
  const events = () => calls.flatMap((call) => call.args.events);
  return { feed, calls, events, advance, timers };
}

const assistantEnd = (text: string, calls: Array<{ id: string; name: string; arguments?: unknown }> = []) => ({
  type: "message_end",
  message: { role: "assistant", content: [...(text ? [{ type: "text", text }] : []), ...calls.map((call) => ({ type: "toolCall", id: call.id, name: call.name, arguments: call.arguments ?? {} }))] },
});
const textDelta = (text: string) => ({
  type: "message_update",
  message: { role: "assistant", content: [{ type: "text", text }] },
  assistantMessageEvent: { type: "text_delta", contentIndex: 0, delta: text.slice(-1), partial: {} },
});
const userEnd = (text: string) => ({ type: "message_end", message: { role: "user", content: [{ type: "text", text }] } });

describe("dashboard knob", () => {
  test("only a literal false turns the feed off", () => {
    assert.equal(resolveDashboardEnabled(undefined), true);
    assert.equal(resolveDashboardEnabled({}), true);
    assert.equal(resolveDashboardEnabled({ dashboard_enabled: true }), true);
    assert.equal(resolveDashboardEnabled({ dashboard_enabled: "false" as never }), true);
    assert.equal(resolveDashboardEnabled({ dashboard_enabled: false }), false);
  });
});

describe("whiteboard origin", () => {
  test("the whiteboard label is pinned and round-trips", () => {
    assert.equal(WHITEBOARD_MESSAGE_LABEL, "[user message from the ws whiteboard]");
    const sent = labelWhiteboardText("ship it");
    assert.equal(sent, `${WHITEBOARD_MESSAGE_LABEL}\nship it`);
    assert.equal(whiteboardLabeledBody(sent), "ship it");
    assert.equal(whiteboardLabeledBody("ship it"), undefined);
    assert.equal(whiteboardLabeledBody(`${WHITEBOARD_MESSAGE_LABEL} same line`), undefined);
  });

  test("a user message is whiteboard when it opens with the label, terminal otherwise; adapter traffic is not a user message", () => {
    assert.deepEqual(dashboardUserMessage(userEnd(labelWhiteboardText("from the page")).message), { text: "from the page", origin: "whiteboard" });
    assert.deepEqual(dashboardUserMessage(userEnd("typed here").message), { text: "typed here", origin: "terminal" });
    assert.deepEqual(dashboardUserMessage({ role: "user", content: "plain string" }), { text: "plain string", origin: "terminal" });
    assert.equal(dashboardUserMessage(userEnd(labelAdapterText("2 ws messages waiting")).message), undefined);
    assert.equal(dashboardUserMessage(userEnd(ADAPTER_MESSAGE_LABEL).message), undefined);
    assert.equal(dashboardUserMessage(userEnd("   ").message), undefined);
    assert.equal(dashboardUserMessage({ role: "custom", content: "push" }), undefined);
  });

  test("a turn's input origin: whiteboard label, then typed input as terminal, then none", () => {
    assert.equal(inputTurnOrigin(labelWhiteboardText("x"), "extension"), "whiteboard");
    assert.equal(inputTurnOrigin("hello", "interactive"), "terminal");
    assert.equal(inputTurnOrigin("hello", "rpc"), "terminal");
    assert.equal(inputTurnOrigin(labelAdapterText("reminder"), "extension"), undefined);
  });
});

describe("lead feed", () => {
  test("start sends reset, then a lead turn with tool calls sends turn boundaries, batches, the reply, and the summary", async () => {
    const { feed, calls, events, advance } = harness();
    feed.start();
    await advance(0);
    assert.equal(calls.length, 1);
    assert.equal(calls[0]!.name, DASHBOARD_INTAKE_TOOL);
    assert.equal(calls[0]!.args.session_key, "lead-key-1");
    assert.equal(calls[0]!.args.adapter, "pi");
    assert.deepEqual(calls[0]!.args.events, [{ kind: "reset" }]);

    feed.leadEvent({ type: "input", text: "read the file", source: "interactive" });
    feed.leadEvent(userEnd("read the file"));
    feed.leadEvent({ type: "agent_start" });
    feed.leadEvent({ type: "message_start", message: { role: "assistant", content: [] } });
    feed.leadEvent(assistantEnd("Reading it.", [{ id: "c1", name: "read", arguments: { path: "a.ts" } }]));
    feed.leadEvent({ type: "tool_execution_start", toolCallId: "c1", toolName: "read", args: { path: "a.ts" } });
    await advance(FLUSH_INTERVAL_MS);
    feed.leadEvent({ type: "tool_execution_end", toolCallId: "c1", toolName: "read", result: { content: [] }, isError: false });
    feed.leadEvent({ type: "turn_end" });
    feed.leadEvent({ type: "message_start", message: { role: "assistant", content: [] } });
    feed.leadEvent(assistantEnd("Done: it exports one function."));
    feed.leadEvent({ type: "agent_end" });
    feed.leadEvent({ type: "agent_settled" });
    await advance(FLUSH_INTERVAL_MS);

    const sent = events().slice(1);
    const kinds = sent.map((event) => event.kind);
    assert.deepEqual(sent.find((event) => event.kind === "user"), { kind: "user", text: "read the file", origin: "terminal", at: sent.find((event) => event.kind === "user")!.at });
    assert.equal(sent.find((event) => event.kind === "turn_start")!.origin, "terminal");
    assert.ok(kinds.indexOf("user") < kinds.indexOf("turn_start"));
    const finals = sent.filter((event) => event.kind === "reply" && event.final === true).map((event) => event.text);
    assert.deepEqual(finals, ["Reading it.", "Done: it exports one function."]);
    assert.equal(kinds.at(-1), "turn_end", "the finished reply and its batches reach the intake before turn_end");
    assert.ok(kinds.lastIndexOf("reply") < kinds.indexOf("turn_end"));
    const leadAgents = sent.filter((event) => event.kind === "agent" && event.agentId === undefined);
    assert.ok(leadAgents.length > 0, "lead batch views are sent with no agentId");
    const running = leadAgents.find((event) => (event.batches as Array<{ running?: unknown }>).some((batch) => batch.running));
    assert.ok(running, "a running batch reaches the intake mid-turn");
    const last = leadAgents.at(-1)!.batches as Array<{ toolNames: string[]; callCount: number; failed: number; endedAt?: number; seq: number }>;
    assert.equal(last.length, 1);
    assert.deepEqual(last[0]!.toolNames, ["read"]);
    assert.equal(last[0]!.callCount, 1);
    assert.equal(last[0]!.failed, 0);
    assert.ok(last[0]!.endedAt !== undefined);
    assert.equal(calls.length <= 3, true, "the whole turn coalesced into a few calls");
  });

  test("an in-process toolcall_start is named from its partial message, like an RPC event", async () => {
    const { feed, events, advance } = harness();
    feed.start();
    feed.leadEvent({ type: "agent_start" });
    feed.leadEvent({ type: "message_start", message: { role: "assistant", content: [] } });
    feed.leadEvent({ type: "message_update", message: { role: "assistant", content: [] }, assistantMessageEvent: { type: "toolcall_start", contentIndex: 0, partial: { content: [{ type: "toolCall", id: "c1", name: "bash", arguments: {} }] } } });
    await advance(0);
    const generating = events().filter((event) => event.kind === "agent").at(-1)!.batches as Array<{ generating?: { kind: string; toolName?: string } }>;
    assert.deepEqual(generating.at(-1)!.generating?.kind, "tool");
    assert.equal(generating.at(-1)!.generating?.toolName, "bash");
  });

  test("streamed reply deltas coalesce into at most one call per interval, each carrying the full text so far", async () => {
    const { feed, calls, events, advance } = harness();
    feed.start();
    await advance(0);
    feed.leadEvent({ type: "agent_start" });
    let text = "";
    for (let i = 0; i < 200; i++) {
      text += "w";
      feed.leadEvent(textDelta(text));
      if (i % 20 === 19) await advance(30);
    }
    await advance(FLUSH_INTERVAL_MS);
    const replyCalls = calls.filter((call) => call.args.events.some((event) => event.kind === "reply"));
    assert.ok(replyCalls.length >= 2 && replyCalls.length <= 5, `200 deltas over 300 ms took ${replyCalls.length} calls`);
    const streamed = events().filter((event) => event.kind === "reply");
    assert.ok(streamed.every((event) => event.final === undefined));
    assert.equal(streamed.at(-1)!.text, text, "the newest call carries the whole text");
    for (let i = 1; i < calls.length; i++) {
      assert.ok(calls[i]!.args.events.length > 0);
    }

    feed.leadEvent(assistantEnd(text));
    await advance(FLUSH_INTERVAL_MS);
    const final = events().at(-1)!;
    assert.equal(final.kind, "reply");
    assert.equal(final.final, true);
    assert.equal(final.text, text);
  });

  test("a whiteboard-labelled input starts a whiteboard turn and its message is tagged whiteboard", async () => {
    const { feed, events, advance } = harness();
    feed.start();
    const sent = labelWhiteboardText("please continue");
    feed.leadEvent({ type: "input", text: sent, source: "extension" });
    feed.leadEvent(userEnd(sent));
    feed.leadEvent({ type: "agent_start" });
    await advance(0);
    assert.deepEqual(events().filter((event) => event.kind === "user").map(({ text, origin }) => ({ text, origin })), [{ text: "please continue", origin: "whiteboard" }]);
    assert.equal(events().find((event) => event.kind === "turn_start")!.origin, "whiteboard");
  });

  test("a mailbox wake of an idle lead starts a mail turn; mail arriving mid-turn does not retag the next one", async () => {
    const { feed, events, advance } = harness();
    feed.start();
    feed.noteMailWake();
    // The adapter's own wake message for held mail arrives as extension input first.
    feed.leadEvent({ type: "input", text: labelAdapterText("1 ws message waiting"), source: "extension" });
    feed.leadEvent({ type: "agent_start" });
    feed.noteMailWake();
    feed.leadEvent({ type: "agent_settled" });
    feed.leadEvent({ type: "agent_start" });
    await advance(0);
    const starts = events().filter((event) => event.kind === "turn_start");
    assert.equal(starts.length, 2);
    assert.equal(starts[0]!.origin, "mail");
    assert.equal(starts[1]!.origin, undefined);
  });

  test("Pi exposes no lead permission prompt, so the lead never sends a permission event", async () => {
    const { feed, events, advance } = harness();
    feed.start();
    feed.leadEvent({ type: "agent_start" });
    feed.leadEvent({ type: "ui_prompt_start", reason: "ui_prompt", kind: "confirm" });
    feed.leadEvent({ type: "tool_execution_start", toolCallId: "c1", toolName: "bash", args: {} });
    await advance(0);
    assert.equal(events().some((event) => event.kind === "permission"), false);
  });
});

describe("subagent feed", () => {
  const entry = (overrides: Partial<DashboardAgentEntry> = {}): DashboardAgentEntry => ({ agentId: "a1", label: "reviewer", status: "running", caption: "worker", batches: [], ...overrides });

  test("a subagent's session key is sent as sessionKey, never in label or caption, and only changed agents are resent", async () => {
    let entries: DashboardAgentEntry[] = [entry({ agentId: "f1", label: "side-thread", sessionKey: "fork-key-secret", caption: "fork" }), entry({ agentId: "w1", parentId: "f1" })];
    const { feed, calls, events, advance } = harness({ agents: () => entries });
    feed.start();
    await advance(0);
    const agents = events().filter((event) => event.kind === "agent");
    assert.equal(agents.length, 2);
    const fork = agents.find((event) => event.agentId === "f1")!;
    assert.equal(fork.sessionKey, "fork-key-secret");
    assert.ok(!String(fork.label).includes("fork-key-secret") && !String(fork.caption).includes("fork-key-secret"));
    assert.equal(agents.find((event) => event.agentId === "w1")!.parentId, "f1");

    const before = calls.length;
    feed.agentsChanged();
    await advance(FLUSH_INTERVAL_MS);
    assert.equal(calls.length, before, "an unchanged tree sends nothing");

    entries = [entries[0]!, entry({ agentId: "w1", parentId: "f1", status: "done" })];
    feed.agentsChanged();
    await advance(FLUSH_INTERVAL_MS);
    assert.deepEqual(calls.at(-1)!.args.events.map((event) => [event.agentId, event.status]), [["w1", "done"]]);
  });

  test("a failed agent is sent failed; one that leaves the registry while running ends done", async () => {
    let entries: DashboardAgentEntry[] = [entry({ agentId: "a1" }), entry({ agentId: "a2" })];
    const { feed, events, advance } = harness({ agents: () => entries });
    feed.start();
    await advance(0);
    entries = [entry({ agentId: "a1", status: "failed" })];
    feed.agentsChanged();
    await advance(FLUSH_INTERVAL_MS);
    const last = events().slice(-2);
    assert.deepEqual(last.map((event) => [event.agentId, event.status]), [["a1", "failed"], ["a2", "done"]]);
  });

  test("an approval wait is a permission event, cleared when it ends", async () => {
    let entries: DashboardAgentEntry[] = [entry({ permission: { tool: "ws-worker-exec", args: { command: "make test" } } })];
    const { feed, events, advance } = harness({ agents: () => entries });
    feed.start();
    await advance(0);
    assert.deepEqual(events().find((event) => event.kind === "permission"), { kind: "permission", agentId: "a1", tool: "ws-worker-exec", args: { command: "make test" } });
    entries = [entry()];
    feed.agentsChanged();
    await advance(FLUSH_INTERVAL_MS);
    assert.deepEqual(events().at(-1), { kind: "permission", agentId: "a1", clear: true });
  });

  test("while an agent runs the tree is re-sampled without a dirty mark", async () => {
    let entries: DashboardAgentEntry[] = [entry()];
    const { feed, events, advance } = harness({ agents: () => entries });
    feed.start();
    await advance(0);
    entries = [entry({ caption: "worker · awaiting approval" })];
    await advance(AGENT_POLL_MS + FLUSH_INTERVAL_MS);
    assert.equal(events().at(-1)!.caption, "worker · awaiting approval");
  });
});

describe("dashboardAgentEntries", () => {
  test("an agent whose finished work was delivered stays done when its idle process exits later; one cut off mid-run is failed", () => {
    const base = { sessionPath: "/tmp/x/session.jsonl", wsToolNames: [], toolGroup: "full-worker" as const, streaming: false, reportLog: [] };
    const finished: RpcAgentRecord = { ...base, agentId: "done1", running: false, runStartedAt: 10, settledAt: 20, client: {} as never };
    const cutOff: RpcAgentRecord = { ...base, agentId: "cut1", running: true, runStartedAt: 10, client: {} as never };
    const registry: RpcAgentRegistry = new Map([["done1", finished], ["cut1", cutOff]]);
    markAgentExited(undefined, undefined, finished, { suppressTerminal: true });
    markAgentExited(undefined, undefined, cutOff, { suppressTerminal: true });
    const status = new Map(dashboardAgentEntries(registry).map((item) => [item.agentId, item.status]));
    assert.equal(status.get("done1"), "done");
    assert.equal(status.get("cut1"), "failed");
  });

  function rec(agentId: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
    return { agentId, sessionPath: "/tmp/x/session.jsonl", wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [], client: {} as never, ...overrides };
  }

  test("projects the registry: label, status, failure, fork key, batches, nested descendants, and approval waits", () => {
    const buffer = createBatchBuffer();
    observeBatchEvent(buffer, { type: "message_end", message: { role: "assistant", content: [{ type: "toolCall", id: "c1", name: "grep", arguments: { q: "x" } }] } }, 100);
    const registry: RpcAgentRegistry = new Map([
      ["w1", rec("w1", { alias: "builder", running: true, runStartedAt: 50, toolBatches: buffer, spawnRole: "execute-worker", pendingApproval: { cmdId: "1", command: "make test" }, subtreeDescendants: [{ id: "g1", parentId: null, depth: 0, role: "explore", live: true, display: { name: "scout", state: "running", runStartedAt: 60, lastOutputAt: 0 } }] })],
      ["f1", rec("f1", { title: "side", spawnRole: "fork", ownSessionKey: "fork-own-key", runStartedAt: 10, settledAt: 20 })],
      ["x1", rec("x1", { lastRunFailed: true, client: undefined })],
    ]);
    const entries = dashboardAgentEntries(registry);
    const byId = new Map(entries.map((item) => [item.agentId, item]));
    assert.deepEqual([...byId.keys()], ["w1", "g1", "f1", "x1"]);
    const w1 = byId.get("w1")!;
    assert.equal(w1.label, "builder");
    assert.equal(w1.status, "running");
    assert.equal(w1.caption, "execute · awaiting approval");
    assert.deepEqual(w1.permission, { tool: "ws-worker-exec", args: { command: "make test" } });
    assert.deepEqual(w1.batches[0]!.toolNames, ["grep"]);
    assert.equal(w1.sessionKey, undefined, "a worker's own key is unknown to its parent");
    const g1 = byId.get("g1")!;
    assert.equal(g1.parentId, "w1");
    assert.equal(g1.label, "scout");
    assert.equal(g1.status, "running");
    const f1 = byId.get("f1")!;
    assert.equal(f1.sessionKey, "fork-own-key");
    assert.equal(f1.status, "done");
    assert.equal(f1.label, "side");
    assert.equal(byId.get("x1")!.status, "failed");
    assert.equal(byId.get("x1")!.parentId, undefined);
  });
});

describe("failure isolation", () => {
  test("a rejecting intake call is swallowed and the feed retries after a wait", async () => {
    const { feed, calls, advance } = harness({ answer: async () => { throw new Error("ws-mcp gone"); } });
    feed.start();
    await advance(0);
    feed.leadEvent(userEnd("hello"));
    await advance(FLUSH_INTERVAL_MS);
    assert.equal(calls.length, 1, "no retry storm");
    await advance(RETRY_MS);
    assert.equal(calls.length, 2);
    assert.equal(calls[1]!.args.events[0]!.kind, "reset");
  });

  test("a rejected call resyncs: reset, the open turn's turn_start, the lost conversation events, the lead, and every running agent", async () => {
    let failNext = false;
    const { feed, calls, advance } = harness({
      agents: () => [{ agentId: "a1", label: "x", status: "running", caption: "worker", batches: [] }],
      answer: async () => {
        if (failNext) { failNext = false; throw new Error("lost"); }
        return { content: [] };
      },
    });
    feed.start();
    feed.leadEvent({ type: "agent_start" });
    await advance(0);
    const turnStart = calls[0]!.args.events.find((event) => event.kind === "turn_start")!;
    failNext = true;
    feed.leadEvent(userEnd("steer this way"));
    feed.leadEvent(assistantEnd("on it"));
    await advance(FLUSH_INTERVAL_MS);
    assert.equal(calls.length, 2);
    await advance(RETRY_MS);
    assert.equal(calls.length, 3);
    const resent = calls[2]!.args.events;
    assert.deepEqual(resent.slice(0, 2), [{ kind: "reset" }, turnStart]);
    assert.ok(resent.some((event) => event.kind === "user" && event.text === "steer this way"));
    assert.ok(resent.some((event) => event.kind === "reply" && event.final === true && event.text === "on it"));
    assert.ok(resent.some((event) => event.kind === "agent" && event.agentId === undefined), "the lead's batches");
    assert.ok(resent.some((event) => event.kind === "agent" && event.agentId === "a1" && event.status === "running"));
  });

  test("a turn whose turn_start was lost gets it back ahead of its turn_end, so the summary is written", async () => {
    let failNext = false;
    const { feed, calls, advance } = harness({ answer: async () => { if (failNext) { failNext = false; throw new Error("lost"); } return { content: [] }; } });
    feed.start();
    await advance(0);
    failNext = true;
    feed.leadEvent({ type: "agent_start" });
    await advance(FLUSH_INTERVAL_MS);
    feed.leadEvent({ type: "agent_settled" });
    await advance(RETRY_MS);
    const kinds = calls.at(-1)!.args.events.map((event) => event.kind);
    assert.equal(kinds[0], "reset");
    assert.equal(kinds.filter((kind) => kind === "turn_start").length, 1);
    assert.ok(kinds.indexOf("turn_start") < kinds.indexOf("turn_end"));
  });

  test("a hanging intake call holds later events (one call in flight, no unbounded queue) until it is abandoned", async () => {
    const { feed, calls, events, advance } = harness({ answer: () => new Promise(() => {}) });
    feed.start();
    await advance(0);
    for (let i = 0; i < 300; i++) feed.leadEvent(userEnd(`message ${i}`));
    await advance(FLUSH_INTERVAL_MS * 5);
    assert.equal(calls.length, 1, "nothing else is sent while the call hangs");
    await advance(INTAKE_CALL_TIMEOUT_MS + RETRY_MS);
    assert.equal(calls.length, 2);
    const users = events().filter((event) => event.kind === "user");
    assert.ok(users.length <= 100, "held events are bounded");
    assert.equal(users.at(-1)!.text, "message 299", "the newest events are kept");
  });

  test("event handlers never throw, never await, and return nothing, even for malformed events or a throwing tree", () => {
    const { feed } = harness({ agents: () => { throw new Error("registry gone"); }, enabled: () => { throw new Error("config gone"); } });
    feed.start();
    for (const event of [undefined, null, 7, "x", { type: "message_end" }, { type: "message_update", assistantMessageEvent: { type: "toolcall_start", partial: null } }, { type: "message_end", message: { role: "user", content: [null, 5] } }]) {
      assert.equal(feed.leadEvent(event), undefined);
    }
    assert.equal(feed.agentsChanged(), undefined);
    assert.equal(feed.noteMailWake(), undefined);
  });

  test("registered lead handlers forward to the current feed and swallow its throw", () => {
    const handlers = new Map<string, (event: unknown) => void>();
    let current: ReturnType<typeof harness>["feed"] | undefined;
    registerDashboardLeadEvents({ on: (name, handler) => { handlers.set(name, handler); } }, () => current);
    assert.deepEqual([...handlers.keys()], [...DASHBOARD_LEAD_EVENTS]);
    assert.equal(handlers.get("agent_start")!({ type: "agent_start" }), undefined, "no feed: nothing happens");
    current = { start() {}, stop() {}, noteMailWake() {}, agentsChanged() {}, leadEvent() { throw new Error("boom"); } };
    assert.equal(handlers.get("message_end")!({ type: "message_end" }), undefined);
  });
});

describe("knob off", () => {
  test("with the knob off no intake call is made, and the knob is not re-read for a while", async () => {
    let reads = 0;
    const { feed, calls, advance } = harness({ enabled: () => { reads++; return false; }, agents: () => [{ agentId: "a1", label: "x", status: "running", caption: "worker", batches: [] }] });
    feed.start();
    await advance(0);
    feed.leadEvent({ type: "agent_start" });
    for (let i = 0; i < 50; i++) feed.leadEvent(textDelta("x".repeat(i + 1)));
    feed.leadEvent(assistantEnd("done"));
    feed.leadEvent({ type: "agent_settled" });
    feed.agentsChanged();
    await advance(KNOB_RECHECK_MS / 2);
    assert.equal(calls.length, 0);
    assert.equal(reads, 1);
    feed.leadEvent(userEnd("inside the window"));
    await advance(KNOB_RECHECK_MS);
    assert.equal(reads, 1, "dropped inside the window without a read");
    feed.leadEvent(userEnd("after the window"));
    await advance(FLUSH_INTERVAL_MS);
    assert.equal(calls.length, 0);
    assert.equal(reads, 2, "read again once the recheck window passed");
  });

  test("an asynchronous knob read is honored, and turning it back on resends a reset first", async () => {
    let on = false;
    const { feed, calls, advance } = harness({ enabled: async () => on, agents: () => [{ agentId: "a1", label: "x", status: "running", caption: "worker", batches: [] }] });
    feed.start();
    await advance(0);
    assert.equal(calls.length, 0);
    on = true;
    await advance(KNOB_RECHECK_MS);
    feed.leadEvent(userEnd("hello"));
    await advance(FLUSH_INTERVAL_MS);
    assert.equal(calls.length, 1);
    const kinds = calls[0]!.args.events.map((event) => event.kind);
    assert.equal(kinds[0], "reset");
    assert.ok(kinds.includes("user"));
    assert.ok(calls[0]!.args.events.some((event) => event.kind === "agent" && event.agentId === "a1" && event.status === "running"), "a still-running agent is resent");
  });

  test("an on knob is re-read at most once per recheck window while a reply streams", async () => {
    let reads = 0;
    const { feed, calls, advance } = harness({ enabled: () => { reads++; return true; } });
    feed.start();
    feed.leadEvent({ type: "agent_start" });
    let text = "";
    for (let i = 0; i < 30; i++) {
      text += "w";
      feed.leadEvent(textDelta(text));
      await advance(FLUSH_INTERVAL_MS);
    }
    assert.ok(calls.length >= 20, `${calls.length} calls`);
    assert.ok(reads <= 2, `${reads} knob reads over ${calls.length} calls`);
  });

  test("a knob read that resolves after its call was abandoned sends nothing", async () => {
    let release: ((value: boolean) => void) | undefined;
    const { feed, calls, advance } = harness({ enabled: () => new Promise<boolean>((resolve) => { release = resolve; }) });
    feed.start();
    await advance(INTAKE_CALL_TIMEOUT_MS);
    release!(true);
    await advance(0);
    assert.equal(calls.length, 0);
  });

  test("no bridge or no session key sends nothing", async () => {
    for (const target of [() => undefined, () => ({ sessionKey: undefined, callTool: async () => { throw new Error("must not be called"); } })]) {
      const { feed, calls, advance } = harness({ target });
      feed.start();
      feed.leadEvent(userEnd("hello"));
      await advance(FLUSH_INTERVAL_MS);
      assert.equal(calls.length, 0);
    }
  });

  test("a stopped feed sends nothing more", async () => {
    const { feed, calls, advance } = harness();
    feed.start();
    feed.stop();
    feed.leadEvent(userEnd("hello"));
    feed.agentsChanged();
    await advance(FLUSH_INTERVAL_MS * 3);
    assert.equal(calls.length, 0);
  });
});

describe("model-visible tools", () => {
  test("the intake tool is adapter-only and never registered for the model", () => {
    assert.ok(ADAPTER_ONLY_RAW_TOOLS.has(DASHBOARD_INTAKE_TOOL));
    const listed = [{ name: "playbook.read" }, { name: DASHBOARD_INTAKE_TOOL }, { name: "mercenary.call" }, { name: "mailbox.send" }];
    assert.deepEqual(filterOutAdapterOnlyTools(filterOutMercenaryTools(listed)).map((tool) => tool.name), ["playbook.read", "mailbox.send"]);
  });
});
