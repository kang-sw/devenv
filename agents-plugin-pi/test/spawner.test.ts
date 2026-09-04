/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * isTerminalStopReason, buildSpawnArgs, AgentEventLineBuffer's
 * multibyte-split safety, handleAgentEvent's state-non-mutation invariant
 * (one-shot `explore` path), resolveModelForAlias (Phase 1's alias-first,
 * inherit-fallback resolution, replacing the old tier-based
 * resolveModelForTier), applyRpcEvent's streaming/idlePending bookkeeping,
 * firstIdlePendingAgentId's idle-edge-consume selection, and listAgents's
 * status mapping — the RPC-backed registry's seam-extractable pure logic
 * (Phase 1 ticket verification boundary: "Registry/select logic
 * unit-tested where seam-extractable").
 *
 * The real RpcClient-backed spawn/send/wait/stop engine (spawnAgent/
 * sendToAgent/waitForAgents/stopAgent) and the one-shot exploreLeaf are
 * exercised only by the live gate (a lead-scoped Pi session spawning a real
 * `pi` child process) — not here, per the plan's Verification Plan split
 * between unit and live coverage; this file never mocks RpcClient itself.
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

  test("full-worker includes built-ins plus every passed ws__* name, in order", () => {
    assert.equal(
      resolveTools("full-worker", ["ws__playbook_render", "ws__ferrule"]),
      "read,bash,edit,write,grep,find,ls,ws__playbook_render,ws__ferrule",
    );
  });

  test("full-worker with an empty ws tool list", () => {
    assert.equal(resolveTools("full-worker", []), "read,bash,edit,write,grep,find,ls");
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
