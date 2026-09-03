/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * isTerminalStopReason, buildSpawnArgs, AgentEventLineBuffer's
 * multibyte-split safety, and handleAgentEvent's state-non-mutation
 * invariant.
 *
 * The async spawn/continue/wait engine (spawnAgent/continueAgent/waitAgents/
 * exploreLeaf) is exercised only by the live gate (a lead-scoped Pi session
 * spawning a real `pi` child process) — not here, per the plan's
 * Verification Plan split between unit and live coverage.
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
  resolveModelForTier,
  asModelTier,
  type AgentRecord,
} from "../src/spawner.ts";
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

describe("resolveModelForTier", () => {
  const catalog: ModelCatalogConfig = {
    tiers: { small: "openrouter/cheap-model", large: "openrouter/big-model" },
  };

  test("tier set + mapped in catalog -> resolved model", () => {
    assert.equal(resolveModelForTier(catalog, "small", "inherited/model"), "openrouter/cheap-model");
    assert.equal(resolveModelForTier(catalog, "large", "inherited/model"), "openrouter/big-model");
  });

  test("tier set + catalog present but that tier unmapped -> inherit", () => {
    assert.equal(resolveModelForTier(catalog, "medium", "inherited/model"), "inherited/model");
    assert.equal(resolveModelForTier(catalog, "xlarge", undefined), undefined);
  });

  test("no tier (spawn regression) -> inherit unchanged", () => {
    assert.equal(resolveModelForTier(catalog, undefined, "inherited/model"), "inherited/model");
    assert.equal(resolveModelForTier(catalog, undefined, undefined), undefined);
  });

  test("tier set but catalog unset -> inherit", () => {
    assert.equal(resolveModelForTier(undefined, "small", "inherited/model"), "inherited/model");
  });

  test("explore's implicit small tier -> resolved when catalog has tiers.small, inherit otherwise", () => {
    assert.equal(resolveModelForTier(catalog, "small", "inherited/model"), "openrouter/cheap-model");
    const unmappedSmall: ModelCatalogConfig = { tiers: { large: "openrouter/big-model" } };
    assert.equal(resolveModelForTier(unmappedSmall, "small", "inherited/model"), "inherited/model");
    assert.equal(resolveModelForTier(undefined, "small", "inherited/model"), "inherited/model");
  });
});

describe("asModelTier", () => {
  test("passes through each of the four valid tier values", () => {
    assert.equal(asModelTier("small"), "small");
    assert.equal(asModelTier("medium"), "medium");
    assert.equal(asModelTier("large"), "large");
    assert.equal(asModelTier("xlarge"), "xlarge");
  });

  test("an unrecognized tier value resolves to undefined (unset/inherit), not a validation error", () => {
    assert.equal(asModelTier("huge"), undefined);
    assert.equal(asModelTier(""), undefined);
  });

  test("undefined stays undefined", () => {
    assert.equal(asModelTier(undefined), undefined);
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
