/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * isTerminalStopReason, buildSpawnArgs, AgentEventLineBuffer's
 * multibyte-split safety, handleAgentEvent's state-non-mutation invariant
 * (one-shot `explore` path), resolveModelForAlias (Phase 1's alias-first,
 * inherit-fallback resolution, replacing the old tier-based
 * resolveModelForTier), applyRpcEvent's streaming/idlePending bookkeeping,
 * firstIdlePendingAgentId's idle-edge-consume selection, listAgents's status
 * mapping, waitForAgents's consume/race/timeout/guard logic (never
 * constructs a real `RpcClient` — it only reads `record.client`/
 * `idlePending`/`waiters` and, on the winning path, `harvestLastMessage`
 * degrades to a plain field read when `record.client` is unset), and
 * sendToAgent's three LIVE branches (streaming+interrupt->steer,
 * streaming+no-interrupt->followUp, idle->prompt) via a duck-typed
 * `steer`/`followUp`/`prompt` stub cast as `RpcClient` — the RPC-backed
 * registry's seam-extractable pure/duck-typeable logic (Phase 1 ticket
 * verification boundary: "Registry/select logic unit-tested where
 * seam-extractable").
 *
 * NOT covered here — genuinely live-gate only, because each path
 * constructs a real `RpcClient` and calls `.start()`: `spawnAgent`,
 * `sendToAgent`'s dormant-auto-resume branch, `stopAgent`, and the one-shot
 * `exploreLeaf`. Exercised only by a lead-scoped Pi session spawning a real
 * `pi` child process, per the plan's Verification Plan split between unit
 * and live coverage.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  resolveTools,
  isTerminalStopReason,
  buildSpawnArgs,
  AgentEventLineBuffer,
  TOOL_GROUPS,
  handleAgentEvent,
  resolveModelForAlias,
  applyRpcEvent,
  firstIdlePendingAgentId,
  listAgents,
  waitForAgents,
  sendToAgent,
  type AgentRecord,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import type { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ModelCatalogConfig } from "../src/model-catalog.ts";

function freshRunningRecord(): AgentRecord {
  return {
    agentId: "test-agent",
    playbook: "implementer",
    noSession: false,
    state: "running",
    outputText: "",
    exitCode: null,
    exitSignal: null,
    selfReap: false,
    waiters: [],
  };
}

describe("TOOL_GROUPS / resolveTools", () => {
  test("read-only carries no bash and no ws__* tools", () => {
    assert.deepEqual([...TOOL_GROUPS["read-only"]], ["read", "grep", "find", "ls"]);
    assert.equal(resolveTools("read-only", ["ws__playbook_render"]), "read,grep,find,ls");
  });

  test("recon adds bash but still never appends ws__* tools", () => {
    assert.deepEqual([...TOOL_GROUPS.recon], ["read", "grep", "find", "ls", "bash"]);
    assert.equal(resolveTools("recon", ["ws__playbook_render", "ws__ferrule"]), "read,grep,find,ls,bash");
  });

  test("recon with no wsToolNames argument at all", () => {
    assert.equal(resolveTools("recon"), "read,grep,find,ls,bash");
  });

  test("full-worker includes built-ins plus the literal explore tool plus every passed ws__* name, in order", () => {
    assert.equal(
      resolveTools("full-worker", ["ws__playbook_render", "ws__ferrule"]),
      "read,bash,edit,write,grep,find,ls,explore,ws__playbook_render,ws__ferrule",
    );
  });

  test("full-worker with an empty ws tool list still includes explore (D-B: a worker can spawn explore)", () => {
    assert.equal(resolveTools("full-worker", []), "read,bash,edit,write,grep,find,ls,explore");
  });

  test("full-worker never includes any ws-agent-* driving/spawn tool name (D-B: depth stays lead -> worker -> explore-leaf)", () => {
    const resolved = resolveTools("full-worker", ["ws__playbook_render"]);
    assert.ok(!resolved.includes("ws-agent-"), `full-worker tools must never include a ws-agent-* name: ${resolved}`);
  });
});

describe("isTerminalStopReason", () => {
  test("stop/length/error/aborted are terminal", () => {
    assert.equal(isTerminalStopReason("stop"), true);
    assert.equal(isTerminalStopReason("length"), true);
    assert.equal(isTerminalStopReason("error"), true);
    assert.equal(isTerminalStopReason("aborted"), true);
  });

  test("toolUse is NOT terminal", () => {
    assert.equal(isTerminalStopReason("toolUse"), false);
  });

  test("undefined and unknown values are not terminal", () => {
    assert.equal(isTerminalStopReason(undefined), false);
    assert.equal(isTerminalStopReason("pending"), false);
    assert.equal(isTerminalStopReason(""), false);
  });
});

describe("buildSpawnArgs", () => {
  test("spawn mode: --session, --append-system-prompt, --tools, --model, task, in order", () => {
    const args = buildSpawnArgs({
      mode: "spawn",
      sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
      noSession: false,
      promptPath: "/tmp/ws-pi-agent-x/prompt.md",
      tools: "read,bash,edit,write,grep,find,ls,ws__playbook_render",
      model: "openrouter/some-model",
      task: "implement the thing",
    });
    assert.deepEqual(args, [
      "--mode",
      "json",
      "-p",
      "--session",
      "/tmp/ws-pi-agent-x/session.jsonl",
      "--append-system-prompt",
      "/tmp/ws-pi-agent-x/prompt.md",
      "--tools",
      "read,bash,edit,write,grep,find,ls,ws__playbook_render",
      "--model",
      "openrouter/some-model",
      "implement the thing",
    ]);
  });

  test("continue mode: same shape as spawn (reuses sessionPath/promptPath, no re-render)", () => {
    const args = buildSpawnArgs({
      mode: "continue",
      sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
      noSession: false,
      promptPath: "/tmp/ws-pi-agent-x/prompt.md",
      tools: "read,bash,edit,write,grep,find,ls",
      task: "follow-up task",
    });
    assert.ok(args.includes("--session"));
    assert.equal(args[args.indexOf("--session") + 1], "/tmp/ws-pi-agent-x/session.jsonl");
    assert.ok(!args.includes("--no-session"));
  });

  test("explore mode: --no-session, never --session", () => {
    const args = buildSpawnArgs({
      mode: "explore",
      noSession: true,
      promptPath: "/tmp/ws-pi-agent-y/prompt.md",
      tools: "read,grep,find,ls,bash",
      task: "where is X defined?",
    });
    assert.ok(args.includes("--no-session"));
    assert.ok(!args.includes("--session"));
  });

  test("--session and --no-session are mutually exclusive: passing both throws", () => {
    assert.throws(() =>
      buildSpawnArgs({
        mode: "spawn",
        sessionPath: "/tmp/x/session.jsonl",
        noSession: true,
        task: "x",
      }),
    );
  });

  test("neither sessionPath nor noSession is a caller bug and throws", () => {
    assert.throws(() =>
      buildSpawnArgs({
        mode: "spawn",
        noSession: false,
        task: "x",
      }),
    );
  });

  test("--model is omitted entirely when unset (inherit)", () => {
    const args = buildSpawnArgs({
      mode: "explore",
      noSession: true,
      tools: "read,grep,find,ls,bash",
      task: "q",
    });
    assert.ok(!args.includes("--model"));
  });

  test("--append-system-prompt is omitted when promptPath is unset", () => {
    const args = buildSpawnArgs({
      mode: "explore",
      noSession: true,
      tools: "read,grep,find,ls,bash",
      task: "q",
    });
    assert.ok(!args.includes("--append-system-prompt"));
  });

  test("--tools is omitted when tools is unset", () => {
    const args = buildSpawnArgs({
      mode: "explore",
      noSession: true,
      task: "q",
    });
    assert.ok(!args.includes("--tools"));
  });

  test("the task positional is always the final argument", () => {
    const args = buildSpawnArgs({
      mode: "explore",
      noSession: true,
      task: "final positional check",
    });
    assert.equal(args[args.length - 1], "final positional check");
  });
});

describe("AgentEventLineBuffer", () => {
  test("parses a single complete NDJSON event in one chunk", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    buf.feed(Buffer.from('{"type":"agent_start"}\n'));
    assert.equal(events.length, 1);
    assert.deepEqual(events[0], { type: "agent_start" });
  });

  test("parses an event split across two chunks", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    const full = '{"type":"message_end","message":{"role":"assistant","stopReason":"stop"}}\n';
    const splitAt = 25;
    buf.feed(Buffer.from(full.slice(0, splitAt)));
    assert.equal(events.length, 0, "must not emit until the newline arrives");
    buf.feed(Buffer.from(full.slice(splitAt)));
    assert.equal(events.length, 1);
    assert.deepEqual(events[0], { type: "message_end", message: { role: "assistant", stopReason: "stop" } });
  });

  test("decodes a multibyte UTF-8 codepoint split exactly across a chunk boundary", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    // em-dash U+2014 is 3 bytes in UTF-8: 0xE2 0x80 0x94.
    const payload = JSON.stringify({ type: "message_end", message: { role: "assistant", content: [{ type: "text", text: "before—after" }] } });
    const fullBuf = Buffer.from(`${payload}\n`, "utf8");
    const emDashByteOffset = fullBuf.indexOf(Buffer.from([0xe2, 0x80, 0x94]));
    assert.ok(emDashByteOffset > 0, "test setup: em-dash bytes must be present");
    const splitPoint = emDashByteOffset + 1;
    buf.feed(fullBuf.subarray(0, splitPoint));
    buf.feed(fullBuf.subarray(splitPoint));
    assert.equal(events.length, 1);
    assert.deepEqual(
      (events[0] as { message: { content: { text: string }[] } }).message.content[0].text,
      "before—after",
    );
  });

  test("handles a multibyte split across many single-byte chunks (arrow + box-drawing)", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    const text = "step → next ─── done";
    const payload = JSON.stringify({ type: "message_end", message: { role: "assistant", content: [{ type: "text", text }] } });
    const fullBuf = Buffer.from(`${payload}\n`, "utf8");
    for (let i = 0; i < fullBuf.length; i++) {
      buf.feed(fullBuf.subarray(i, i + 1));
    }
    assert.equal(events.length, 1);
    assert.deepEqual((events[0] as { message: { content: { text: string }[] } }).message.content[0].text, text);
  });

  test("reports a parse error for an invalid line without throwing", () => {
    const errors: string[] = [];
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer(
      (evt) => events.push(evt),
      (line) => errors.push(line),
    );
    buf.feed(Buffer.from("not json at all\n"));
    assert.equal(events.length, 0);
    assert.deepEqual(errors, ["not json at all"]);
  });

  test("end() flushes a final line with no trailing newline", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    buf.feed(Buffer.from('{"type":"agent_end","messages":[]}'));
    assert.equal(events.length, 0, "must not emit before end() without a trailing newline");
    buf.end();
    assert.equal(events.length, 1);
    assert.deepEqual(events[0], { type: "agent_end", messages: [] });
  });

  test("end() is a no-op when there is no pending partial line", () => {
    const events: unknown[] = [];
    const buf = new AgentEventLineBuffer((evt) => events.push(evt));
    buf.feed(Buffer.from('{"type":"agent_start"}\n'));
    assert.equal(events.length, 1);
    buf.end();
    assert.equal(events.length, 1, "end() must not re-emit or duplicate the already-flushed event");
  });
});

describe("resolveModelForAlias", () => {
  const catalog: ModelCatalogConfig = {
    aliases: { small: "openrouter/cheap-model", large: "openrouter/big-model" },
  };

  test("alias set + mapped in catalog -> resolved model", () => {
    assert.equal(resolveModelForAlias(catalog, "small", "inherited/model"), "openrouter/cheap-model");
    assert.equal(resolveModelForAlias(catalog, "large", "inherited/model"), "openrouter/big-model");
  });

  test("alias set + catalog present but that alias unmapped -> inherit", () => {
    assert.equal(resolveModelForAlias(catalog, "medium", "inherited/model"), "inherited/model");
    assert.equal(resolveModelForAlias(catalog, "xlarge", undefined), undefined);
  });

  test("no alias (model_name omitted) -> inherit unchanged", () => {
    assert.equal(resolveModelForAlias(catalog, undefined, "inherited/model"), "inherited/model");
    assert.equal(resolveModelForAlias(catalog, undefined, undefined), undefined);
  });

  test("alias set but catalog unset -> inherit", () => {
    assert.equal(resolveModelForAlias(undefined, "small", "inherited/model"), "inherited/model");
  });

  test("explore's implicit small alias -> resolved when catalog has aliases.small, inherit otherwise", () => {
    assert.equal(resolveModelForAlias(catalog, "small", "inherited/model"), "openrouter/cheap-model");
    const unmappedSmall: ModelCatalogConfig = { aliases: { large: "openrouter/big-model" } };
    assert.equal(resolveModelForAlias(unmappedSmall, "small", "inherited/model"), "inherited/model");
    assert.equal(resolveModelForAlias(undefined, "small", "inherited/model"), "inherited/model");
  });

  test("an arbitrary user-chosen alias name (not one of the old four tier names) resolves normally", () => {
    const reviewerCatalog: ModelCatalogConfig = { aliases: { reviewer: "openrouter/big-model" } };
    assert.equal(resolveModelForAlias(reviewerCatalog, "reviewer", "inherited/model"), "openrouter/big-model");
  });
});

function freshRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "rpc-agent-1",
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: [],
    streaming: false,
    idlePending: false,
    waiters: [],
    ...overrides,
  };
}

describe("applyRpcEvent", () => {
  test("agent_start flips streaming true, leaves idlePending untouched", () => {
    const record = freshRpcRecord({ idlePending: true });
    applyRpcEvent(record, { type: "agent_start" });
    assert.equal(record.streaming, true);
    assert.equal(record.idlePending, true, "agent_start must not clear a still-latched idlePending flag");
  });

  test("agent_settled flips streaming false, latches idlePending, and settles waiters", () => {
    const record = freshRpcRecord({ streaming: true });
    let settled = false;
    record.waiters.push(() => {
      settled = true;
    });
    applyRpcEvent(record, { type: "agent_settled" });
    assert.equal(record.streaming, false);
    assert.equal(record.idlePending, true);
    assert.equal(settled, true, "agent_settled must drain and resolve pending waiters");
    assert.deepEqual(record.waiters, [], "waiters array must be drained after settling");
  });

  test("other event types (e.g. message_update) are ignored — no streaming/idlePending mutation", () => {
    const record = freshRpcRecord({ streaming: true, idlePending: false });
    applyRpcEvent(record, { type: "message_update" });
    assert.equal(record.streaming, true);
    assert.equal(record.idlePending, false);
  });
});

describe("firstIdlePendingAgentId", () => {
  test("returns undefined when no record has idlePending latched", () => {
    const records = [
      { id: "a", record: freshRpcRecord({ agentId: "a" }) },
      { id: "b", record: freshRpcRecord({ agentId: "b" }) },
    ];
    assert.equal(firstIdlePendingAgentId(records), undefined);
  });

  test("returns the first (in given order) record with idlePending latched", () => {
    const records = [
      { id: "a", record: freshRpcRecord({ agentId: "a" }) },
      { id: "b", record: freshRpcRecord({ agentId: "b", idlePending: true }) },
      { id: "c", record: freshRpcRecord({ agentId: "c", idlePending: true }) },
    ];
    assert.equal(firstIdlePendingAgentId(records), "b");
  });
});

/**
 * Duck-typed `RpcClient` stand-in exposing only the methods these tests
 * drive (`steer`/`followUp`/`prompt`/`getLastAssistantText`), cast as
 * `RpcClient` — mirroring the existing `client: {} as RpcClient` pattern
 * already used by the `listAgents` tests above. Never a real `RpcClient`
 * construction, never a subprocess.
 */
function fakeRpcClient(overrides: {
  steer?: (message: string) => Promise<void>;
  followUp?: (message: string) => Promise<void>;
  prompt?: (message: string) => Promise<void>;
  getLastAssistantText?: () => Promise<string | null>;
} = {}): { client: RpcClient; calls: Array<[string, string]> } {
  const calls: Array<[string, string]> = [];
  const client = {
    steer: overrides.steer ?? (async (message: string) => void calls.push(["steer", message])),
    followUp: overrides.followUp ?? (async (message: string) => void calls.push(["followUp", message])),
    prompt: overrides.prompt ?? (async (message: string) => void calls.push(["prompt", message])),
    getLastAssistantText: overrides.getLastAssistantText ?? (async () => null),
  };
  return { client: client as unknown as RpcClient, calls };
}

describe("waitForAgents", () => {
  test("empty agentIds throws (fail-fast, never races zero promises)", async () => {
    const registry: RpcAgentRegistry = new Map();
    await assert.rejects(() => waitForAgents(registry, []), /requires at least one agentId/);
  });

  test("unknown agentId throws", async () => {
    const registry: RpcAgentRegistry = new Map();
    await assert.rejects(() => waitForAgents(registry, ["missing"]), /unknown agentId/);
  });

  test("already-idle fast path harvests a clientless (dormant) record immediately, with no race", async () => {
    const record = freshRpcRecord({ agentId: "a", idlePending: true, lastText: "cached final answer" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    const result = await waitForAgents(registry, ["a"]);
    assert.deepEqual(result, { agent_id: "a", last_message: "cached final answer", timed_out: false });
    assert.equal(record.idlePending, false, "the fast path must consume (clear) idlePending, not just read it");
  });

  test("timeout-no-finisher returns a timed_out marker and leaves the agent registered/untouched", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    const result = await waitForAgents(registry, ["a"], 15);
    assert.deepEqual(result, { timed_out: true });
    assert.equal(registry.has("a"), true, "a timed-out agent must stay registered, never killed/removed");
    assert.equal(record.streaming, true, "timeout must not mutate the agent's tracked state");
  });

  test("guards against an all-dormant/clientless agent set with no timeout instead of hanging forever", async () => {
    const record = freshRpcRecord({ agentId: "a" }); // no client, idlePending false
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    await assert.rejects(() => waitForAgents(registry, ["a"]), /dormant.*no timeout|no timeout.*dormant/i);
  });

  test("an all-dormant set WITH a timeout still returns a timed_out marker instead of throwing", async () => {
    const record = freshRpcRecord({ agentId: "a" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    const result = await waitForAgents(registry, ["a"], 15);
    assert.deepEqual(result, { timed_out: true });
  });

  test("winning race: applyRpcEvent's agent_settled resolves the pending wait with the settled agent's last message", async () => {
    const { client } = fakeRpcClient({ getLastAssistantText: async () => "the final answer" });
    const record = freshRpcRecord({ agentId: "a", client, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const resultPromise = waitForAgents(registry, ["a"]);
    // waitForAgents synchronously registers its waiter (via the winner
    // Promise executor) before its first `await`, so the settle below is
    // guaranteed to land on an already-armed waiter — same technique
    // applyRpcEvent's own waiter-drain test above uses.
    applyRpcEvent(record, { type: "agent_settled" });

    const result = await resultPromise;
    assert.deepEqual(result, { agent_id: "a", last_message: "the final answer", timed_out: false });
    assert.equal(record.idlePending, false, "the winning path must also consume idlePending");
  });

  test("winning race among multiple agentIds: the one that settles first wins, others stay pending/registered", async () => {
    const slow = freshRpcRecord({ agentId: "slow", client: fakeRpcClient().client, streaming: true });
    const { client: fastClient } = fakeRpcClient({ getLastAssistantText: async () => "fast agent done" });
    const fast = freshRpcRecord({ agentId: "fast", client: fastClient, streaming: true });
    const registry: RpcAgentRegistry = new Map([
      ["slow", slow],
      ["fast", fast],
    ]);

    const resultPromise = waitForAgents(registry, ["slow", "fast"]);
    applyRpcEvent(fast, { type: "agent_settled" });

    const result = await resultPromise;
    assert.equal(result.agent_id, "fast");
    assert.equal(result.last_message, "fast agent done");
    assert.equal(registry.has("slow"), true, "the non-winning agent must stay registered");
  });
});

describe("listAgents", () => {
  test("maps a record with no live client to status dormant", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "dormant" }]);
  });

  test("maps a live, non-streaming record to status idle", () => {
    const record = freshRpcRecord({ agentId: "a", client: {} as RpcClient, streaming: false });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "idle" }]);
  });

  test("maps a live, streaming record to status running", () => {
    const record = freshRpcRecord({ agentId: "a", client: {} as RpcClient, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "running" }]);
  });

  test("maps multiple agents in insertion order with independent statuses", () => {
    const registry: RpcAgentRegistry = new Map([
      ["dormant-one", freshRpcRecord({ agentId: "dormant-one" })],
      ["running-one", freshRpcRecord({ agentId: "running-one", client: {} as RpcClient, streaming: true })],
    ]);
    assert.deepEqual(listAgents(registry), [
      { agent_id: "dormant-one", status: "dormant" },
      { agent_id: "running-one", status: "running" },
    ]);
  });
});

describe("sendToAgent (live branches only — dormant auto-resume is live-gate only, see module doc comment)", () => {
  test("live + streaming + interrupt:true -> steer(), never followUp/prompt", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "a", "interrupt this", true);

    assert.deepEqual(result, { agent_id: "a" });
    assert.deepEqual(calls, [["steer", "interrupt this"]]);
  });

  test("live + streaming + interrupt falsy -> followUp(), never steer/prompt", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "a", "queue this");

    assert.deepEqual(result, { agent_id: "a" });
    assert.deepEqual(calls, [["followUp", "queue this"]]);
  });

  test("live + idle (streaming:false) -> prompt(), regardless of interrupt", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: false });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "a", "new message", true);

    assert.deepEqual(result, { agent_id: "a" });
    assert.deepEqual(calls, [["prompt", "new message"]], "interrupt must be ignored while idle — nothing is running to interrupt");
  });

  test("REGRESSION (C2 fix): live-idle send clears a stale idlePending latched by the PREVIOUS run before starting the new one", async () => {
    const { client, calls } = fakeRpcClient();
    // Simulates: an earlier run already settled (idlePending latched by
    // applyRpcEvent's agent_settled handler) but nothing has consumed it via
    // ws-agent-wait yet — then a new send starts a fresh run.
    const record = freshRpcRecord({ agentId: "a", client, streaming: false, idlePending: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    await sendToAgent(registry, { cwd: "/tmp" }, "a", "follow-up while stale-idle");

    assert.equal(record.idlePending, false, "idlePending from the PREVIOUS run must be cleared before/at the new prompt() call");
    assert.deepEqual(calls, [["prompt", "follow-up while stale-idle"]]);
  });

  test("unknown agentId throws", async () => {
    const registry: RpcAgentRegistry = new Map();
    await assert.rejects(() => sendToAgent(registry, { cwd: "/tmp" }, "missing", "hi"), /unknown agentId/);
  });
});

describe("handleAgentEvent", () => {
  test("a terminal stopReason updates record.stopReason but NEVER flips record.state (load-bearing: only proc.on('close') may do that)", () => {
    const record = freshRunningRecord();
    handleAgentEvent(record, { type: "message_end", message: { role: "assistant", stopReason: "stop" } });
    assert.equal(record.stopReason, "stop");
    assert.equal(record.state, "running", "state must stay unchanged by an in-stream terminal stopReason");
  });

  test("a non-terminal stopReason (toolUse) also updates stopReason without touching state", () => {
    const record = freshRunningRecord();
    handleAgentEvent(record, { type: "message_end", message: { role: "assistant", stopReason: "toolUse" } });
    assert.equal(record.stopReason, "toolUse");
    assert.equal(record.state, "running");
  });

  test("captures final assistant text and errorMessage without touching state", () => {
    const record = freshRunningRecord();
    handleAgentEvent(record, {
      type: "message_end",
      message: { role: "assistant", stopReason: "error", errorMessage: "boom", content: [{ type: "text", text: "partial answer" }] },
    });
    assert.equal(record.outputText, "partial answer");
    assert.equal(record.errorMessage, "boom");
    assert.equal(record.state, "running");
  });

  test("ignores non-message_end events and non-assistant roles", () => {
    const record = freshRunningRecord();
    handleAgentEvent(record, { type: "agent_start" });
    handleAgentEvent(record, { type: "message_end", message: { role: "toolResult", stopReason: "stop" } });
    assert.equal(record.stopReason, undefined);
    assert.equal(record.state, "running");
  });
});
