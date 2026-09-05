/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * isTerminalStopReason, buildSpawnArgs, AgentEventLineBuffer's
 * multibyte-split safety, handleAgentEvent's state-non-mutation invariant
 * (one-shot `explore` path), resolveModelForAlias (Phase 1's alias-first,
 * inherit-fallback resolution, replacing the old tier-based
 * resolveModelForTier), applyRpcEvent's streaming/report bookkeeping and its
 * push OUTCOMES, listAgents's status mapping, and sendToAgent's three LIVE
 * branches (streaming+interrupt->steer, streaming+no-interrupt->followUp,
 * idle->prompt) via a duck-typed `steer`/`followUp`/`prompt` stub cast as
 * `RpcClient` — the RPC-backed registry's seam-extractable pure/duck-typeable
 * logic (Phase 1 ticket verification boundary: "Registry/select logic
 * unit-tested where seam-extractable").
 *
 * 260905 (push-only child reports) replaced the whole pull-side surface this
 * file used to cover. `ws-agent-wait` and its `waitForAgents` consume/race/
 * timeout logic, the `idlePending`/`waiters` latch, the `pendingReports` FIFO
 * (`enqueueReport`/`drainReports`/`REPORT_BUFFER_CAP`), and the three
 * `first*AgentId` selectors are all DELETED, not deprecated — their tests are
 * gone with them rather than rewritten, because nothing selects a winner any
 * more. In their place this file covers the push model's own seams:
 * `shouldPushToLead` (the role gate), `computeRunningStatusLine` (the `N of
 * M` fan-in arithmetic), `buildPushContent`/`pushToLead` (message shape and
 * best-effort delivery), `promptAgent` (the single `running`/
 * `terminalThisTurn`/`lastLeadPromptAt` funnel), `recordReport`/
 * `reportKindsSinceLeadPrompt` (the bounded report log that replaced
 * `pendingReports`), `probeAgentLiveness`/`markAgentExited`/
 * `startLivenessProbe` (the `getState()`-rejection exit detector), and
 * `stopAgent`'s new push + `silent` contract.
 *
 * Review relay #1 (test partition C2/C3, I7) widened that set: the settle
 * suppression the ticket names lives in `attachEventListener`, not in the pure
 * `applyRpcEvent`, so that listener is now exported and driven here against a
 * duck-typed `onEvent`/`getState`/`getLastAssistantText` client; `spawnAgent`'s
 * launch-failure push was extracted into `pushSpawnFailed` and covered
 * directly; and `pushToLead`'s role gate is exercised through the real call
 * path under a mutated `WS_PI_SPAWN_ROLE_ENV`, not only through the
 * `shouldPushToLead` predicate.
 *
 * NOT covered here — genuinely live-gate only, because each path
 * constructs a real `RpcClient` and calls `.start()`: `spawnAgent`,
 * `sendToAgent`'s dormant-auto-resume branch, and the one-shot
 * `exploreLeaf`. Exercised only by a lead-scoped Pi session spawning a real
 * `pi` child process, per the plan's Verification Plan split between unit
 * and live coverage.
 *
 * Review fix (cycle 1, 260903 Phase 1 goal-loop): also covers
 * `buildRpcClientOptions`/`buildChildProcessEnv`'s process-role env marker
 * placement at both spawn call sites — previously left to a manual
 * spot-check with zero automated coverage. 260904 Phase 1 renamed the
 * marker from the boolean `WS_PI_AGENT_CHILD_ENV` to the role-valued
 * `WS_PI_SPAWN_ROLE_ENV` (`process-role.ts`) — these tests now assert the
 * role values (`"worker"`/`"explore"`) instead of `"1"`.
 *
 * 260904 Phase 1 (execute-approve gateway) additionally covers: the new
 * `"execute-worker"` `TOOL_GROUPS` entry and its `resolveTools` threading
 * through a fake record's `toolGroup`; `applyRpcEvent`'s new
 * `pendingApproval`-capturing branch for `GATED_EXEC_TOOL_NAME`;
 * `buildRpcClientOptions`'s new `WS_PI_APPROVAL_DIR_ENV` placement; and
 * `inheritModelFromToolCtx` (exported out of `registerAgentTools`'s former
 * private closure for reuse by `execute-gateway.ts`). The gated-exec tool's
 * own `execute()` body, `ws-execute`/`ws-approve`'s tool registrations, and
 * the approval-request/decision file relay end-to-end are NOT covered here
 * — see test/execute-gateway.test.ts's header comment for that split.
 *
 * 260904 Phase 1 (side-thread fork) additionally covers: `buildRpcClientOptions`'s
 * new `forkFrom`/`parentSessionKey` params (the `--fork` vs `--session` arg
 * branch and the `"fork"` vs `"worker"` role marker). `fork.ts`'s own pure
 * predicates/IO glue are covered by test/fork.test.ts instead — see that
 * file's header comment for its own pure/live-gate split.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, beforeEach, afterEach } from "node:test";
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
  attachEventListener,
  buildPushContent,
  computeRunningStatusLine,
  flushHeldPushes,
  heldPushQueue,
  leadIdleRef,
  markAgentExited,
  probeAgentLiveness,
  promptAgent,
  pushSpawnFailed,
  pushToLead,
  recordReport,
  registerPushFlush,
  reportKindsSinceLeadPrompt,
  shouldPushToLead,
  startLivenessProbe,
  stopAgent,
  REPORT_LOG_CAP,
  REPORT_TO_LEAD_TOOL_NAME,
  GATED_EXEC_TOOL_NAME,
  WS_PI_APPROVAL_DIR_ENV,
  getAgentTranscriptPath,
  listAgents,
  sendToAgent,
  buildRpcClientOptions,
  buildChildProcessEnv,
  inheritModelFromToolCtx,
  resolveSpawnToolGroup,
  type AgentRecord,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  type ToolGroup,
} from "../src/spawner.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
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

  test("full-worker includes built-ins plus the literal explore and ws-report-to-lead tools plus every passed ws__* name, in order", () => {
    assert.equal(
      resolveTools("full-worker", ["ws__playbook_render", "ws__ferrule"]),
      "read,bash,edit,write,grep,find,ls,explore,ws-report-to-lead,ws__playbook_render,ws__ferrule",
    );
  });

  test("full-worker with an empty ws tool list still includes explore and ws-report-to-lead (D-B: a worker can spawn explore and report)", () => {
    assert.equal(resolveTools("full-worker", []), "read,bash,edit,write,grep,find,ls,explore,ws-report-to-lead");
  });

  test("full-worker never includes any ws-agent-* driving/spawn tool name (D-B: depth stays lead -> worker -> explore-leaf)", () => {
    const resolved = resolveTools("full-worker", ["ws__playbook_render"]);
    assert.ok(!resolved.includes("ws-agent-"), `full-worker tools must never include a ws-agent-* name: ${resolved}`);
  });

  test("execute-worker equals read-only plus the gated-exec, report, and explore tools, in order (260904 Phase 1)", () => {
    assert.deepEqual([...TOOL_GROUPS["execute-worker"]], ["read", "grep", "find", "ls", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME, "explore"]);
    assert.equal(resolveTools("execute-worker"), `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME},explore`);
  });

  test("execute-worker never appends ws__* bridge tool names (unlike full-worker) — a caller-passed wsToolNames is ignored", () => {
    assert.equal(resolveTools("execute-worker", ["ws__playbook_render"]), `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME},explore`);
  });

  test("execute-worker never includes bash/edit/write — those would let the worker bypass the approval gate", () => {
    const resolved = resolveTools("execute-worker");
    for (const forbidden of ["bash", "edit", "write"]) {
      assert.ok(!resolved.split(",").includes(forbidden), `execute-worker tools must never include "${forbidden}": ${resolved}`);
    }
  });

  test("resolveTools(record.toolGroup, ...) threading: a fake record with toolGroup:\"execute-worker\" resolves the execute-worker tool list, not full-worker's", () => {
    const record = freshRpcRecord({ toolGroup: "execute-worker" as ToolGroup, wsToolNames: ["ws__playbook_render"] });
    const resolved = resolveTools(record.toolGroup, record.wsToolNames);
    assert.equal(resolved, `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME},explore`);
    assert.notEqual(resolved, resolveTools("full-worker", record.wsToolNames), "must not fall back to the old hardcoded full-worker group");
  });

  test("resolveTools(record.toolGroup, ...) threading: a record whose toolGroup field is \"full-worker\" resolves full-worker's tool list", () => {
    const record = freshRpcRecord({ toolGroup: "full-worker", wsToolNames: ["ws__ferrule"] });
    assert.equal(resolveTools(record.toolGroup, record.wsToolNames), resolveTools("full-worker", ["ws__ferrule"]));
  });
});

describe("resolveSpawnToolGroup (review fix, relay #1, TEST finding #3)", () => {
  test("an omitted (undefined) explicit toolGroup defaults to full-worker — spawnAgent's actual ctx.toolGroup ?? \"full-worker\" seam", () => {
    assert.equal(resolveSpawnToolGroup(undefined), "full-worker");
  });

  test("an explicit toolGroup is passed through unchanged, never overridden by the default", () => {
    assert.equal(resolveSpawnToolGroup("execute-worker"), "execute-worker");
    assert.equal(resolveSpawnToolGroup("read-only"), "read-only");
    assert.equal(resolveSpawnToolGroup("recon"), "recon");
  });

  test("an explicit \"full-worker\" is indistinguishable from omission (both resolve to full-worker) — the intended no-op case", () => {
    assert.equal(resolveSpawnToolGroup("full-worker"), resolveSpawnToolGroup(undefined));
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

describe("inheritModelFromToolCtx", () => {
  test("extracts provider/id from a well-formed toolCtx.model", () => {
    assert.equal(inheritModelFromToolCtx({ model: { provider: "openrouter", id: "some-model" } }), "openrouter/some-model");
  });

  test("returns undefined when toolCtx is undefined, has no model, or model is missing provider/id", () => {
    assert.equal(inheritModelFromToolCtx(undefined), undefined);
    assert.equal(inheritModelFromToolCtx({}), undefined);
    assert.equal(inheritModelFromToolCtx({ model: {} }), undefined);
    assert.equal(inheritModelFromToolCtx({ model: { provider: "openrouter" } }), undefined);
    assert.equal(inheritModelFromToolCtx({ model: { id: "some-model" } }), undefined);
  });
});

function freshRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return {
    agentId: "rpc-agent-1",
    sessionPath: "/tmp/ws-pi-agent-x/session.jsonl",
    systemPromptPath: "/tmp/ws-pi-agent-x/prompt.md",
    wsToolNames: [],
    toolGroup: "full-worker",
    streaming: false,
    running: false,
    reportLog: [],
    ...overrides,
  };
}

/**
 * A record in the LIVE resting state: `client` present is what
 * `computeRunningStatusLine` reads for M (a dormant/stopped/exited record has
 * none), so every fan-in fixture needs one. Never a real `RpcClient`.
 */
function liveRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return freshRpcRecord({ client: {} as RpcClient, ...overrides });
}

/**
 * Duck-typed `ExtensionAPI` stand-in exposing only `sendMessage` — the one
 * method `pushToLead` touches. Every push assertion below reads `sent`
 * rather than a live Pi session, the same plain-object convention the rest
 * of this file uses for `RpcClient`.
 */
function fakePi(overrides: { sendMessage?: (message: unknown, options?: unknown) => void } = {}): {
  api: Parameters<typeof pushToLead>[0];
  sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }>;
} {
  const sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }> = [];
  const api = {
    sendMessage:
      overrides.sendMessage ??
      ((message: unknown, options?: unknown) => {
        sent.push({ message: message as never, options: options as never });
      }),
  };
  return { api: api as unknown as Parameters<typeof pushToLead>[0], sent };
}

describe("applyRpcEvent", () => {
  test("agent_start flips streaming true and returns an empty outcome (no push)", () => {
    const record = freshRpcRecord({ running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "agent_start" }), {});
    assert.equal(record.streaming, true);
    assert.equal(record.running, true, "agent_start must not disturb the fan-in latch set at prompt time");
  });

  test("agent_settled flips streaming false, clears running, and reports settled so the caller can decide on the push", () => {
    const record = freshRpcRecord({ streaming: true, running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "agent_settled" }), { settled: true });
    assert.equal(record.streaming, false);
    assert.equal(record.running, false, "the run is over — the child stops counting toward the fan-in whatever the caller pushes");
  });

  test("other event types (e.g. message_update) are ignored — no streaming/running mutation, no push", () => {
    const record = freshRpcRecord({ streaming: true, running: true });
    assert.deepEqual(applyRpcEvent(record, { type: "message_update" }), {});
    assert.equal(record.streaming, true);
    assert.equal(record.running, true);
  });
});

describe("applyRpcEvent: ws-report-to-lead (260905 push outcomes)", () => {
  test("a plain progress report logs an entry and returns a ws-agent-report push delivered as followUp", () => {
    const record = freshRpcRecord();
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "halfway done" } });
    assert.deepEqual(result, { push: { family: "ws-agent-report", payload: { report: "halfway done" }, deliverAs: "followUp" } });
    assert.equal(record.reportLog.length, 1);
    assert.equal(record.reportLog[0].kind, undefined);
    assert.equal(record.terminalThisTurn, undefined, "a plain progress update is not terminal — the child is still running");
  });

  test("a tool_execution_start for a different toolName is ignored (no push, no log entry)", () => {
    const record = freshRpcRecord();
    assert.deepEqual(applyRpcEvent(record, { type: "tool_execution_start", toolName: "bash", args: { message: "not a report" } }), {});
    assert.deepEqual(record.reportLog, []);
  });

  test("a missing or non-string args.message is ignored", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: {} });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: 42 } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME });
    assert.deepEqual(record.reportLog, []);
  });

  test('Edition: a kind:"final" report pushes NOTHING now — it is stashed for the end of the child\'s turn', () => {
    const record = freshRpcRecord();
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "all done", kind: "final" } });
    assert.deepEqual(result, {}, "the child is still mid-turn at this instant — announcing completion here is premature");
    assert.equal(record.pendingFinal, "all done");
    assert.equal(record.reportLog[0].kind, "final");
    assert.equal(record.terminalThisTurn, true, "the sender removes itself from N at once, and suppresses the redundant settle");
  });

  test("Edition: two finals in one turn keep the LAST text — a corrected final supersedes the first", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "first cut", kind: "final" } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "corrected", kind: "final" } });
    assert.equal(record.pendingFinal, "corrected");
  });

  test("Edition: promptAgent clears an un-pushed final — it belonged to the task being replaced", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ client, pendingFinal: "old answer" });
    await promptAgent(record, client, "new task");
    assert.equal(record.pendingFinal, undefined);
  });

  test('a kind:"question" report with no hook pushes ws-agent-question as STEER — the headless lead must act on it mid-turn', () => {
    const record = freshRpcRecord();
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "which branch?", kind: "question" } });
    assert.deepEqual(result, { push: { family: "ws-agent-question", payload: { question: "which branch?" }, deliverAs: "steer" } });
    assert.equal(record.terminalThisTurn, true);
  });

  test("an unrecognized or non-string kind is dropped — logged and pushed as a plain progress report", () => {
    const record = freshRpcRecord();
    const bogus = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "progress", kind: "bogus" } });
    const numeric = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "more progress", kind: 42 } });
    assert.deepEqual(bogus, { push: { family: "ws-agent-report", payload: { report: "progress" }, deliverAs: "followUp" } });
    assert.deepEqual(numeric, { push: { family: "ws-agent-report", payload: { report: "more progress" }, deliverAs: "followUp" } });
    assert.deepEqual(record.reportLog.map((e) => e.kind), [undefined, undefined]);
  });

  test("onQuestionReport returning a string SUPPRESSES the push entirely — the TUI owner surface consumed the question (§1)", () => {
    const seen: string[] = [];
    const record = freshRpcRecord({
      onQuestionReport: (_rec, message) => {
        seen.push(message);
        return "[ws] registered as thread q1";
      },
    });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "which branch?", kind: "question" } });
    assert.deepEqual(seen, ["which branch?"]);
    assert.deepEqual(result, {}, "the lead is not part of a fork-raised question exchange");
    assert.equal(record.reportLog.length, 1, "the report is still logged — suppression is about the push, not the bookkeeping");
  });

  test("onQuestionReport returning undefined (headless) keeps the ws-agent-question push", () => {
    const record = freshRpcRecord({ onQuestionReport: () => undefined });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "which branch?", kind: "question" } });
    assert.deepEqual(result, { push: { family: "ws-agent-question", payload: { question: "which branch?" }, deliverAs: "steer" } });
  });

  test("a throwing onQuestionReport degrades to the headless baseline rather than dropping the question", () => {
    const record = freshRpcRecord({
      onQuestionReport: () => {
        throw new Error("hook exploded");
      },
    });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "which branch?", kind: "question" } });
    assert.deepEqual(result, { push: { family: "ws-agent-question", payload: { question: "which branch?" }, deliverAs: "steer" } });
  });

  test('onFinalReport returning true SUPPRESSES the ws-agent-report push (a lead-ask thread already sends its own summary message)', () => {
    const seen: string[] = [];
    const record = freshRpcRecord({
      onFinalReport: (_rec, message) => {
        seen.push(message);
        return true;
      },
    });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "decided: merge", kind: "final" } });
    assert.deepEqual(seen, ["decided: merge"]);
    assert.deepEqual(result, {});
    assert.equal(record.pendingFinal, undefined, "a consumed final is not even stashed — it is not the lead's message");
  });

  test("onFinalReport returning falsy (a fork-raised task fork) keeps the report — its final IS the completion signal", () => {
    const record = freshRpcRecord({ onFinalReport: () => false });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "all done", kind: "final" } });
    assert.deepEqual(result, {});
    assert.equal(record.pendingFinal, "all done", "stashed for the turn end, not dropped");
  });

  test("onFinalReport never sees a question or a plain progress report", () => {
    const seen: string[] = [];
    const record = freshRpcRecord({ onFinalReport: (_rec, message) => void seen.push(message) });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "need input", kind: "question" } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "plain progress" } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "decided: merge", kind: "final" } });
    assert.deepEqual(seen, ["decided: merge"]);
  });

  test("a throwing onFinalReport is swallowed and the report is stashed anyway", () => {
    const record = freshRpcRecord({
      onFinalReport: () => {
        throw new Error("hook exploded");
      },
    });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "all done", kind: "final" } });
    assert.deepEqual(result, {});
    assert.equal(record.pendingFinal, "all done");
  });
});

describe("applyRpcEvent: ws-worker-exec (260904 Phase 1 approval-request capture)", () => {
  test("a tool_execution_start for the gated-exec tool with a string toolCallId + command sets pendingApproval from the event's own toolCallId (the cmd_id)", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-123",
      args: { command: "rm -rf build", rationale: "clean stale build output" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-123", command: "rm -rf build", rationale: "clean stale build output", cwd: undefined });
  });

  test("rationale is omitted (undefined) when args.rationale is not a string", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-456",
      args: { command: "echo hi" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-456", command: "echo hi", rationale: undefined, cwd: undefined });
  });

  test("a missing toolCallId is ignored — pendingApproval is never set without a usable cmd_id", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, args: { command: "echo hi" } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a missing or non-string args.command is ignored", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: {} });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: 42 } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a tool_execution_start for a different toolName (e.g. ws-report-to-lead) never sets pendingApproval", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "call-1", args: { message: "hi", command: "echo hi" } });
    assert.equal(record.pendingApproval, undefined);
  });

  test("a later gated-exec event overwrites a still-pending approval from an earlier one (the newest command is the one awaiting a decision)", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo one" } });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: "echo two" } });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-2", command: "echo two", rationale: undefined, cwd: undefined });
  });

  test("review fix (relay #1, CORRECTNESS finding #1): a string args.cwd override is captured onto pendingApproval.cwd", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-1",
      args: { command: "echo hi", cwd: "/repo/subdir" },
    });
    assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "echo hi", rationale: undefined, cwd: "/repo/subdir" });
  });
  test("260905: the gated-exec branch returns an empty outcome — the approval PUSH is createApprovalRelay's job, not applyRpcEvent's", () => {
    const record = freshRpcRecord();
    const result = applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: GATED_EXEC_TOOL_NAME,
      toolCallId: "call-1",
      args: { command: "echo hi" },
    });
    assert.deepEqual(result, {}, "execute-gateway.ts owns the §7 payload and the working-context scrape");
    assert.deepEqual(record.pendingApproval, { cmdId: "call-1", command: "echo hi", rationale: undefined, cwd: undefined });
  });

  test("260905: an approval request does NOT mark the child terminal — it is still outstanding, and the lead is what unblocks it", () => {
    const record = freshRpcRecord({ running: true });
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo hi" } });
    assert.equal(record.terminalThisTurn, undefined);
    assert.equal(record.running, true);
  });


  test("cwd is omitted (undefined) when args.cwd is missing or not a string", () => {
    const record = freshRpcRecord();
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-1", args: { command: "echo hi" } });
    assert.equal(record.pendingApproval?.cwd, undefined);
    applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "call-2", args: { command: "echo hi", cwd: 42 } });
    assert.equal(record.pendingApproval?.cwd, undefined);
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

describe("shouldPushToLead (the push gate)", () => {
  test("the host lead (no role marker) and a fork push; a worker and an explore leaf do not", () => {
    assert.equal(shouldPushToLead({}), true, "no marker = host lead");
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "fork" }), true);
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "worker" }), false, "a worker's reports travel to ITS parent over RPC, not into its own transcript");
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "explore" }), false);
  });

  test("an unrecognized role value is treated as no marker (host lead), same as readSpawnRole", () => {
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "bogus" }), true);
  });
});

describe("computeRunningStatusLine (fan-in N of M)", () => {
  test("Edition: an empty/absent registry produces NO line at all — a push with nothing delegated said `0 of 0`", () => {
    assert.equal(computeRunningStatusLine(new Map()), undefined);
    assert.equal(computeRunningStatusLine(undefined), undefined);
  });

  test("Edition: a registry whose every record is dormant/stopped also produces no line", () => {
    const registry: RpcAgentRegistry = new Map([["gone", freshRpcRecord({ agentId: "gone" })]]);
    assert.equal(computeRunningStatusLine(registry), undefined);
  });

  test("M counts every live, non-threadBound record; N is the subset with no terminal report this turn, named by id", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true })],
      ["b", liveRpcRecord({ agentId: "b", running: true, terminalThisTurn: true })],
      ["c", liveRpcRecord({ agentId: "c", running: true })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "2 of 3 delegated agents still running: a, c");
  });

  test("Edition: the ids are listed only while N > 0 — a `0 of M` line has nothing to name", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true, terminalThisTurn: true })],
      ["b", liveRpcRecord({ agentId: "b" })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "0 of 2 delegated agents still running");
  });

  test("I3: the denominator holds across a real 3-way fan-out — 2 of 3, 1 of 3, 0 of 3", () => {
    // The event order the runtime actually produces: each worker files its
    // final (terminalThisTurn) and then settles (running cleared) BEFORE the
    // next one reports. Keying M on `running` made this read 2 of 3 -> 1 of 2
    // -> 0 of 1, so the ticket's "0 of 3" completion cue could never occur.
    const ids = ["a", "b", "c"];
    const registry: RpcAgentRegistry = new Map(ids.map((id) => [id, liveRpcRecord({ agentId: id, running: true })] as const));
    const lines: Array<string | undefined> = [];
    for (const id of ids) {
      const record = registry.get(id)!;
      applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: x" } });
      lines.push(computeRunningStatusLine(registry));
      applyRpcEvent(record, { type: "agent_settled" });
    }
    assert.deepEqual(lines, [
      "2 of 3 delegated agents still running: b, c",
      "1 of 3 delegated agents still running: c",
      "0 of 3 delegated agents still running",
    ]);
  });

  test("I3: a live child that settled idle stays in M and only leaves N", () => {
    const idle = liveRpcRecord({ agentId: "idle", running: true });
    const registry: RpcAgentRegistry = new Map([
      ["busy", liveRpcRecord({ agentId: "busy", running: true })],
      ["idle", idle],
    ]);
    applyRpcEvent(idle, { type: "agent_settled" });
    assert.equal(computeRunningStatusLine(registry), "1 of 2 delegated agents still running: busy");
  });

  test("stopped/exited/dormant records (no client) are absent from M entirely", () => {
    const registry: RpcAgentRegistry = new Map([
      ["live", liveRpcRecord({ agentId: "live", running: true })],
      ["stopped", freshRpcRecord({ agentId: "stopped", running: false })],
      ["dormant", freshRpcRecord({ agentId: "dormant" })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 of 1 delegated agent still running: live");
  });

  test("a threadBound agent is excluded from BOTH N and M — the owner exchange is not the lead's fan-in", () => {
    const registry: RpcAgentRegistry = new Map([
      ["worker", liveRpcRecord({ agentId: "worker", running: true })],
      ["discussing", liveRpcRecord({ agentId: "discussing", running: true, threadBound: true })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 of 1 delegated agent still running: worker");
  });

  test("an approval-blocked child is still counted — it is outstanding, and the lead is what unblocks it", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true, pendingApproval: { cmdId: "c1", command: "echo hi" } })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 of 1 delegated agent still running: a");
  });

  test("a child counts from the prompt-issue instant, before any agent_start event has arrived", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    await promptAgent(record, client, "go");
    assert.equal(record.streaming, false, "no agent_start has been observed yet");
    assert.equal(computeRunningStatusLine(registry), "1 of 1 delegated agent still running: a");
  });

  test("singular/plural agreement follows M", () => {
    assert.equal(computeRunningStatusLine(new Map([["a", liveRpcRecord({ agentId: "a", running: true })]])), "1 of 1 delegated agent still running: a");
  });
});

describe("buildPushContent", () => {
  test("renders a head line naming the family and agent, one key: value line per payload field, and the status line last", () => {
    const content = buildPushContent("ws-agent-report", "a1", { kind: "final", report: "done" }, "0 of 1 delegated agent still running");
    assert.equal(content, ["[ws-agent-report] agent a1", "kind: final", "report: done", "0 of 1 delegated agent still running"].join("\n"));
  });

  test("an agent-less family (ws-agent-orphaned) drops the agent from the head line", () => {
    const content = buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, "0 of 1 delegated agent still running");
    assert.equal(content, ["[ws-agent-orphaned]", "count: 2", "0 of 1 delegated agent still running"].join("\n"));
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
  test("sends one custom message per family, with details carrying agent_id, the payload, and the status line", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 1);
    const [{ message, options }] = pi.sent;
    assert.equal(message.customType, "ws-agent-report");
    assert.equal(message.display, true);
    assert.deepEqual(message.details, { agent_id: "a", report: "halfway", status: "1 of 1 delegated agent still running: a" });
    assert.deepEqual(options, { deliverAs: "followUp", triggerTurn: true }, "triggerTurn is what makes an IDLE lead act on the signal at once");
  });

  test("an absent record still pushes (the orphan roll-call), and with nothing delegated it carries NO status field", () => {
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 2 }, "followUp");
    assert.deepEqual(pi.sent[0].message.details, { count: 2 }, "Edition: `0 of 0 delegated agents still running` told the lead nothing");
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
  });

  test("review relay #1 (I7): a worker-role process pushes NOTHING through the real call path, not just through the predicate", () => {
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
    assert.deepEqual(pi.sent, [], "a worker's reports travel to ITS parent over RPC, never into its own transcript");
  });

  test("the same call from the host lead (no role marker) does push — the gate is the role, not the arguments", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    assert.equal(pi.sent.length, 1);
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
 * finished while the lead was still spawning its siblings delivered `0 of 1`,
 * the next `0 of 2`, and only the last `0 of 3`: three separate invitations to
 * synthesize before the fan-out was in.
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
  });

  afterEach(() => {
    heldPushQueue.length = 0;
    leadIdleRef.current = undefined;
  });

  test("an IDLE lead is pushed to immediately — holding would only delay the wake", () => {
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

    assert.equal(flushHeldPushes(pi.api), 1);
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, [], "the queue is drained, so a second settle re-sends nothing");
  });

  test("no idleness accessor at all (a headless path, a torn-down session) sends straight through", () => {
    leadIdleRef.current = undefined;
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 1, "a held push nothing ever flushes would be a lost report");
  });

  test("a throwing idleness accessor degrades to sending, not to holding", () => {
    leadIdleRef.current = () => {
      throw new Error("ctx is gone");
    };
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 1);
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

  test("the live-run failure itself: three workers, two finals landing mid-turn, read 1 of 3 then 0 of 3 — never 0 of 1", () => {
    idle = false;
    const pi = fakePi();
    const ids = ["w1", "w2", "w3"];
    const registry: RpcAgentRegistry = new Map(ids.map((id) => [id, liveRpcRecord({ agentId: id, running: true })] as const));

    // The lead is still mid-turn (it is spawning w3) when w1 and w2 finish.
    for (const id of ["w1", "w2"]) {
      const record = registry.get(id)!;
      record.terminalThisTurn = true;
      record.running = false;
      pushToLead(pi.api, registry, record, "ws-agent-report", { kind: "final", report: `Outcome: ${id}` }, "followUp");
    }
    assert.deepEqual(pi.sent, [], "nothing is delivered while the lead is mid-turn");

    // w3 is still working when the lead's turn ends: the held pair is released
    // now, against the registry as it stands at THIS instant.
    idle = true;
    assert.equal(flushHeldPushes(pi.api), 2);
    assert.deepEqual(
      pi.sent.map((entry) => (entry.message.details as { status?: string }).status),
      ["1 of 3 delegated agents still running: w3", "1 of 3 delegated agents still running: w3"],
      "at release time one worker is genuinely still out — arrival-time lines said `0 of 1` and `0 of 2`",
    );
    assert.deepEqual(
      pi.sent.map((entry) => (entry.message.details as { report?: string }).report),
      ["Outcome: w1", "Outcome: w2"],
      "arrival order is preserved",
    );

    // w3 finishes during the run those two started.
    const w3 = registry.get("w3")!;
    w3.terminalThisTurn = true;
    w3.running = false;
    idle = false;
    pushToLead(pi.api, registry, w3, "ws-agent-report", { kind: "final", report: "Outcome: w3" }, "followUp");
    idle = true;
    flushHeldPushes(pi.api);

    assert.equal(
      (pi.sent[2].message.details as { status?: string }).status,
      "0 of 3 delegated agents still running",
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
    assert.equal(flushHeldPushes(reentrant.api), 1);
    assert.equal(heldPushQueue.length, 1, "the re-entrant push waits for the next settle rather than joining this drain");
  });

  test("a worker-role process neither holds nor sends — it has no lead session of its own", () => {
    idle = false;
    const pi = fakePi();
    const previous = process.env[WS_PI_SPAWN_ROLE_ENV];
    process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
    try {
      pushToLead(pi.api, new Map(), undefined, "ws-agent-report", { report: "x" }, "followUp");
      assert.deepEqual(heldPushQueue, [], "holding a push a worker will never flush would leak it");
      assert.deepEqual(pi.sent, []);

      // And the flush handler itself is a no-op there, even with a stale entry.
      heldPushQueue.push({ registry: undefined, record: undefined, family: "ws-agent-report", payload: { report: "stale" } });
      let settled: (() => void) | undefined;
      const api = { on: (event: string, handler: () => void) => void (event === "agent_settled" && (settled = handler)), sendMessage: () => assert.fail("a worker process must not push") };
      registerPushFlush(api as unknown as Parameters<typeof registerPushFlush>[0]);
      settled?.();
      assert.equal(heldPushQueue.length, 1, "left untouched rather than delivered into a worker's own transcript");
    } finally {
      heldPushQueue.length = 0;
      if (previous === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV];
      else process.env[WS_PI_SPAWN_ROLE_ENV] = previous;
    }
  });

  test("registerPushFlush releases the held pushes on the lead's own agent_settled", () => {
    idle = false;
    const sent: string[] = [];
    let settled: (() => void) | undefined;
    const api = {
      on: (event: string, handler: () => void) => void (event === "agent_settled" && (settled = handler)),
      sendMessage: (message: { customType?: string }) => void sent.push(message.customType ?? ""),
    } as unknown as Parameters<typeof registerPushFlush>[0];
    registerPushFlush(api);

    pushToLead(api as never, new Map(), undefined, "ws-agent-report", { report: "held" }, "followUp");
    assert.deepEqual(sent, []);

    idle = true;
    settled?.();
    assert.deepEqual(sent, ["ws-agent-report"]);
  });
});

/**
 * Review relay #1, test partition C2: the settle-push suppression the ticket
 * names lives HERE, in the IO listener, not in the pure `applyRpcEvent` — so
 * it needs coverage here or the `!threadBound && !terminalThisTurn` guard
 * could be deleted with the suite still green. Driven with a duck-typed client
 * (`onEvent`/`getState`/`getLastAssistantText`); never a real `RpcClient`.
 */
describe("attachEventListener (the settle-suppression IO gate)", () => {
  function listenerHarness() {
    let listener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      getState: async () => ({ sessionFile: "/tmp/s.jsonl" }),
      getLastAssistantText: async () => "the last thing it said",
    } as unknown as RpcClient;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true, client });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    attachEventListener(pi.api, registry, record, client);
    return { pi, record, registry, emit: (evt: unknown) => listener?.(evt) };
  }

  /** The settle push is async (it awaits `harvestLastMessage`), so drain the microtask queue. */
  const settleDrain = () => new Promise((resolve) => setImmediate(resolve));

  const families = (pi: ReturnType<typeof fakePi>) => pi.sent.map((s) => s.message.customType);

  test("a plain settle pushes ws-agent-settled reason:idle carrying the harvested last message", async () => {
    const h = listenerHarness();
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(families(h.pi), ["ws-agent-settled"]);
    assert.deepEqual(h.pi.sent[0].message.details, {
      agent_id: "a",
      reason: "idle",
      last_message: "the last thing it said",
      status: "0 of 1 delegated agent still running",
    });
    assert.equal(h.pi.sent[0].options?.deliverAs, "followUp");
  });

  test("Edition: a final is silent until the child's turn ends, then arrives ONCE as the report — never also a settle", async () => {
    const h = listenerHarness();
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: done" } });
    assert.deepEqual(h.pi.sent, [], "the child filed its answer but is still mid-turn — committing, cleaning up");

    h.emit({ type: "agent_settled" });
    await settleDrain();

    assert.deepEqual(families(h.pi), ["ws-agent-report"], "one child turn is one message to the lead");
    assert.deepEqual(h.pi.sent[0].message.details, {
      agent_id: "a",
      kind: "final",
      report: "Outcome: done",
      settled_reason: "idle",
      status: "0 of 1 delegated agent still running",
    });
    assert.equal(h.record.pendingFinal, undefined, "released, not re-pushable");
  });

  test("Edition: two finals in one turn release the LAST one at settle", async () => {
    const h = listenerHarness();
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: first" } });
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: corrected" } });
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(families(h.pi), ["ws-agent-report"]);
    assert.equal(h.pi.sent[0].message.details?.report, "Outcome: corrected");
  });

  test("Edition: a hook-consumed final (a lead-ask thread) pushes nothing at settle either", async () => {
    const h = listenerHarness();
    h.record.onFinalReport = () => true;
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "decided: merge" } });
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(h.pi.sent, [], "the thread sends its own ws-thread-summary; the settle notice would duplicate a consumed report");
  });

  test("a settle right after a headless question pushes the question only", async () => {
    const h = listenerHarness();
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "which anchor?" } });
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(families(h.pi), ["ws-agent-question"]);
    assert.equal(h.pi.sent[0].options?.deliverAs, "steer");
  });

  test("a threadBound record settles SILENTLY — the owner exchange's turn boundaries are not the lead's business", async () => {
    const h = listenerHarness();
    h.record.threadBound = true;
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(h.pi.sent, []);
  });

  test("the same record settles loudly once the thread closes — suppression is scoped to the bind, not permanent", async () => {
    const h = listenerHarness();
    h.record.threadBound = true;
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(h.pi.sent, []);
    h.record.threadBound = false;
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(families(h.pi), ["ws-agent-settled"]);
  });

  test("a plain progress report is pushed as ws-agent-report/followUp with no settle involved", () => {
    const h = listenerHarness();
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "halfway" } });
    assert.deepEqual(families(h.pi), ["ws-agent-report"]);
    assert.equal(h.pi.sent[0].options?.deliverAs, "followUp");
  });

  test("a hook-consumed question (the TUI owner surface) is not pushed at all", () => {
    const h = listenerHarness();
    h.record.onQuestionReport = () => "[ws] thread q1 — the owner answers this.";
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "which anchor?" } });
    assert.deepEqual(h.pi.sent, []);
  });

  test("a dead child's settle transitions it to exited and pushes once (the liveness probe on the transition)", async () => {
    let listener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      getState: async () => {
        throw new Error("client is not running");
      },
      getLastAssistantText: async () => null,
    } as unknown as RpcClient;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true, client, terminalThisTurn: true });
    attachEventListener(pi.api, new Map([["a", record]]), record, client);
    listener?.({ type: "agent_settled" });
    await new Promise((resolve) => setImmediate(resolve));
    assert.deepEqual(
      pi.sent.map((s) => (s.message.details as { reason?: string }).reason),
      ["exited"],
    );
    assert.equal(record.client, undefined);
    assert.equal(record.running, false);
  });

  test("the ctx approval callback wins over the per-record fallback, and the fallback fires when there is no ctx one", () => {
    const ctxCalls: string[] = [];
    const recordCalls: string[] = [];
    let listener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      getState: async () => ({}),
    } as unknown as RpcClient;
    const approvalEvent = { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "c1", args: { command: "echo hi" } };

    const withCtx = liveRpcRecord({ agentId: "a", client, onApprovalPending: (r) => recordCalls.push(r.agentId) });
    attachEventListener(fakePi().api, new Map(), withCtx, client, (r) => ctxCalls.push(r.agentId));
    listener?.(approvalEvent);
    assert.deepEqual(ctxCalls, ["a"]);
    assert.deepEqual(recordCalls, [], "the ctx callback is preferred — never both");

    const fallbackOnly = liveRpcRecord({ agentId: "b", client, onApprovalPending: (r) => recordCalls.push(r.agentId) });
    attachEventListener(fakePi().api, new Map(), fallbackOnly, client);
    listener?.(approvalEvent);
    assert.deepEqual(recordCalls, ["b"], "a resume with no ctx relay still reaches the record's own");
  });
});

/**
 * Review relay #1, test partition C3: `spawnAgent`'s launch-failure branch is
 * live-gate only (it constructs a real `RpcClient`), so its push half was
 * extracted into `pushSpawnFailed` and is covered here.
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
      pi.sent[0].message.details,
      { agent_id: "a", reason: "spawn-failed", error: "spawn ENOENT" },
      "Edition: the only child failed to start, so there is no fan-in left to describe",
    );
    assert.equal(record.client, undefined, "a failed spawn leaves no live client behind");
    assert.equal(record.running, false, "and stops counting toward the fan-in immediately");
    assert.equal(unsubscribed, 1);
  });

  test("a non-Error throw is stringified rather than dropped", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a" });
    pushSpawnFailed(pi.api, new Map(), record, "boom");
    assert.equal((pi.sent[0].message.details as { error?: string }).error, "boom");
  });
});

describe("promptAgent (the single prompt funnel)", () => {
  test("latches running, clears terminalThisTurn, stamps lastLeadPromptAt, and forwards the message to prompt()", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ terminalThisTurn: true });
    const before = Date.now();

    await promptAgent(record, client, "do the thing");

    assert.equal(record.running, true);
    assert.equal(record.terminalThisTurn, false);
    assert.ok((record.lastLeadPromptAt ?? 0) >= before);
    assert.deepEqual(calls, [["prompt", "do the thing"]]);
  });

  test("isLeadPrompt:false (the anti-bleed nudge) still latches running but must NOT move lastLeadPromptAt", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ lastLeadPromptAt: 1_000 });

    await promptAgent(record, client, "nudge", { isLeadPrompt: false });

    assert.equal(record.running, true);
    assert.equal(record.lastLeadPromptAt, 1_000, "moving the watermark would hide the very stale idle-without-final the nudge exists to serve");
  });
});

describe("recordReport / reportKindsSinceLeadPrompt", () => {
  test("a kind-less report round-trips as {at} with no kind key at all", () => {
    const record = freshRpcRecord();
    recordReport(record, undefined, 5);
    assert.deepEqual(record.reportLog, [{ at: 5 }]);
  });

  test("appending past REPORT_LOG_CAP drops the OLDEST entry, never the newest", () => {
    const record = freshRpcRecord();
    for (let i = 0; i <= REPORT_LOG_CAP; i += 1) recordReport(record, undefined, i);
    assert.equal(record.reportLog.length, REPORT_LOG_CAP);
    assert.equal(record.reportLog[0].at, 1, "at:0 must be the one dropped");
    assert.equal(record.reportLog[record.reportLog.length - 1].at, REPORT_LOG_CAP);
  });

  test("only reports at or after lastLeadPromptAt are returned — a final from a PREVIOUS task stops counting", () => {
    const record = freshRpcRecord({ lastLeadPromptAt: 100 });
    recordReport(record, "final", 50);
    recordReport(record, undefined, 150);
    recordReport(record, "question", 200);
    assert.deepEqual(reportKindsSinceLeadPrompt(record), [undefined, "question"]);
  });

  test("with no lastLeadPromptAt ever stamped, the whole log is in scope", () => {
    const record = freshRpcRecord();
    recordReport(record, "final", 1);
    assert.deepEqual(reportKindsSinceLeadPrompt(record), ["final"]);
  });
});

describe("markAgentExited / probeAgentLiveness", () => {
  test("a getState() rejection transitions the record to exited and pushes ws-agent-settled reason:exited", async () => {
    const pi = fakePi();
    const client = {
      getState: async () => {
        throw new Error("process exited");
      },
    } as unknown as RpcClient;
    const record = freshRpcRecord({ agentId: "a", client, running: true, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const alive = await probeAgentLiveness(pi.api, registry, record);

    assert.equal(alive, false);
    assert.equal(record.client, undefined);
    assert.equal(record.running, false);
    assert.equal(record.streaming, false);
    assert.equal(pi.sent.length, 1);
    assert.equal((pi.sent[0].message.details as { reason?: string }).reason, "exited");
  });

  test("a resolving getState() leaves the record alone and pushes nothing", async () => {
    const pi = fakePi();
    const client = { getState: async () => ({ sessionFile: "/tmp/s.jsonl" }) } as unknown as RpcClient;
    const record = freshRpcRecord({ agentId: "a", client, running: true });

    assert.equal(await probeAgentLiveness(pi.api, new Map([["a", record]]), record), true);
    assert.equal(record.client, client);
    assert.deepEqual(pi.sent, []);
  });

  test("probing a record with no client reports not-alive and pushes nothing (it was already stopped)", async () => {
    const pi = fakePi();
    const record = freshRpcRecord({ agentId: "a" });
    assert.equal(await probeAgentLiveness(pi.api, new Map(), record), false);
    assert.deepEqual(pi.sent, []);
  });

  test("Edition: a child that filed a final and then died surfaces the report with settled_reason:exited, not a bare exit notice", () => {
    const pi = fakePi();
    const record = freshRpcRecord({ agentId: "a", client: {} as RpcClient, running: true, pendingFinal: "Outcome: done" });
    markAgentExited(pi.api, new Map(), record);
    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.customType, "ws-agent-report");
    assert.deepEqual(pi.sent[0].message.details, {
      agent_id: "a",
      kind: "final",
      report: "Outcome: done",
      settled_reason: "exited",
    });
  });

  test("markAgentExited is idempotent — a second call on an already-cleared record pushes nothing", () => {
    const pi = fakePi();
    const record = freshRpcRecord({ agentId: "a", client: {} as RpcClient, running: true });
    markAgentExited(pi.api, new Map(), record);
    markAgentExited(pi.api, new Map(), record);
    assert.equal(pi.sent.length, 1, "a dead child is announced once, not once per observation");
  });

  test("the record's unsubscribe is called when its live state is cleared, so a dead client's listener is detached", () => {
    const pi = fakePi();
    let detached = false;
    const record = freshRpcRecord({
      agentId: "a",
      client: {} as RpcClient,
      running: true,
      unsubscribe: () => {
        detached = true;
      },
    });
    markAgentExited(pi.api, new Map(), record);
    assert.equal(detached, true);
    assert.equal(record.unsubscribe, undefined);
  });
});

describe("stopAgent (260905 push + silent)", () => {
  function stoppableClient(): { client: RpcClient; calls: string[] } {
    const calls: string[] = [];
    return {
      client: {
        abort: async () => void calls.push("abort"),
        stop: async () => void calls.push("stop"),
      } as unknown as RpcClient,
      calls,
    };
  }

  test("a live stop aborts, stops, clears live state, and pushes ws-agent-settled reason:stopped", async () => {
    const pi = fakePi();
    const { client, calls } = stoppableClient();
    const record = freshRpcRecord({ agentId: "a", client, running: true, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    assert.deepEqual(await stopAgent(registry, "a", pi.api), { agent_id: "a" });

    assert.deepEqual(calls, ["abort", "stop"]);
    assert.equal(record.client, undefined);
    assert.equal(record.running, false);
    assert.equal(registry.has("a"), true, "D-C: a stopped agent stays registered as dormant/resumable");
    assert.equal(pi.sent.length, 1);
    assert.equal((pi.sent[0].message.details as { reason?: string }).reason, "stopped");
  });

  test("silent:true (ask.ts's thread close, stopAll's shutdown sweep) suppresses the push but still stops the child", async () => {
    const pi = fakePi();
    const { client, calls } = stoppableClient();
    const record = freshRpcRecord({ agentId: "a", client, running: true });

    await stopAgent(new Map([["a", record]]), "a", pi.api, { silent: true });

    assert.deepEqual(calls, ["abort", "stop"]);
    assert.equal(record.client, undefined);
    assert.deepEqual(pi.sent, [], "the owner's summary is the signal there, not a stop notice");
  });

  test("Edition: a stop releases a stashed final as the report itself, with settled_reason:stopped — not a bare stop notice", async () => {
    const pi = fakePi();
    const { client } = stoppableClient();
    const record = freshRpcRecord({ agentId: "a", client, running: true, pendingFinal: "Outcome: done" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    await stopAgent(registry, "a", pi.api);

    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.customType, "ws-agent-report");
    assert.deepEqual(pi.sent[0].message.details, {
      agent_id: "a",
      kind: "final",
      report: "Outcome: done",
      settled_reason: "stopped",
    });
    assert.equal(record.pendingFinal, undefined);
  });

  test("Edition: a SILENT stop drops the stashed final rather than surfacing it from an adapter-internal teardown", async () => {
    const pi = fakePi();
    const { client } = stoppableClient();
    const record = freshRpcRecord({ agentId: "a", client, running: true, pendingFinal: "Outcome: done" });

    await stopAgent(new Map([["a", record]]), "a", pi.api, { silent: true });

    assert.deepEqual(pi.sent, []);
    assert.equal(record.pendingFinal, undefined, "left stashed, a revival would replay it as the answer to a new task");
  });

  test("stopping an already-dormant record is a no-op with no push", async () => {
    const pi = fakePi();
    const record = freshRpcRecord({ agentId: "a" });
    await stopAgent(new Map([["a", record]]), "a", pi.api);
    assert.deepEqual(pi.sent, []);
  });

  test("unknown agentId throws", async () => {
    await assert.rejects(() => stopAgent(new Map(), "missing"), /unknown agentId/);
  });
});

describe("startLivenessProbe", () => {
  test("probes every running record on each tick and reports a dead one as exited", async () => {
    const pi = fakePi();
    const dead = freshRpcRecord({
      agentId: "dead",
      running: true,
      client: {
        getState: async () => {
          throw new Error("gone");
        },
      } as unknown as RpcClient,
    });
    const dormant = freshRpcRecord({ agentId: "dormant", running: true }); // no client — never probed
    const registry: RpcAgentRegistry = new Map([
      ["dead", dead],
      ["dormant", dormant],
    ]);

    const stop = startLivenessProbe(pi.api, registry, 1);
    await new Promise((resolve) => setTimeout(resolve, 25));
    stop();

    assert.equal(dead.client, undefined, "the dead child was detected by the sweep");
    assert.ok(pi.sent.length >= 1);
    assert.equal((pi.sent[0].message.details as { reason?: string }).reason, "exited");
  });

  test("the returned stopper clears the timer — no further probes after stopAll()", async () => {
    const pi = fakePi();
    let probes = 0;
    const record = freshRpcRecord({
      agentId: "a",
      running: true,
      client: { getState: async () => void (probes += 1) } as unknown as RpcClient,
    });
    const stop = startLivenessProbe(pi.api, new Map([["a", record]]), 1);
    await new Promise((resolve) => setTimeout(resolve, 15));
    stop();
    const after = probes;
    await new Promise((resolve) => setTimeout(resolve, 15));
    assert.equal(probes, after, "the interval must be cleared, not merely unref'd");
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

  test("260905: last_report_at carries the newest reportLog entry as ISO, and the key is omitted entirely when nothing was ever reported", () => {
    const quiet = freshRpcRecord({ agentId: "quiet" });
    const chatty = freshRpcRecord({ agentId: "chatty", reportLog: [{ at: 1_700_000_000_000 }, { kind: "final", at: 1_700_000_060_000 }] });
    const registry: RpcAgentRegistry = new Map([
      ["quiet", quiet],
      ["chatty", chatty],
    ]);
    assert.deepEqual(listAgents(registry), [
      { agent_id: "quiet", status: "dormant" },
      { agent_id: "chatty", status: "dormant", last_report_at: new Date(1_700_000_060_000).toISOString() },
    ]);
  });

  test("260905: status still derives from `streaming`, not the narrower fan-in `running` flag", () => {
    const record = freshRpcRecord({ agentId: "a", client: {} as RpcClient, streaming: false, running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "idle" }], "a just-prompted-but-not-yet-started child displays as idle, and is still counted as running by computeRunningStatusLine");
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
    assert.equal(record.running, true, "a steer joins the run already in flight — the child is outstanding again");
  });

  test("live + streaming + interrupt falsy -> followUp(), never steer/prompt", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "a", "queue this");

    assert.deepEqual(result, { agent_id: "a" });
    assert.deepEqual(calls, [["followUp", "queue this"]]);
    assert.equal(record.running, true);
  });

  test("live + idle (streaming:false) -> prompt(), regardless of interrupt", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: false });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "a", "new message", true);

    assert.deepEqual(result, { agent_id: "a" });
    assert.deepEqual(calls, [["prompt", "new message"]], "interrupt must be ignored while idle — nothing is running to interrupt");
  });

  test("260905: a live send goes through promptAgent — running latches and terminalThisTurn from the PREVIOUS turn is cleared", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client, streaming: false, terminalThisTurn: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    await sendToAgent(registry, { cwd: "/tmp" }, "a", "next task");

    assert.equal(record.running, true, "the child counts toward the fan-in from the moment the prompt is issued");
    assert.equal(record.terminalThisTurn, false, "last turn's terminal report must not suppress this turn's settle push");
    assert.ok((record.lastLeadPromptAt ?? 0) > 0, "a lead send stamps the watermark reportKindsSinceLeadPrompt filters on");
    assert.deepEqual(calls, [["prompt", "next task"]]);
  });

  test("260905 review relay: a live streaming send (followUp branch) clears a stale pendingFinal so the next settle is a settle, not the old final", async () => {
    let listener: ((evt: unknown) => void) | undefined;
    const calls: Array<[string, string]> = [];
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      followUp: async (message: string) => void calls.push(["followUp", message]),
      steer: async (message: string) => void calls.push(["steer", message]),
      getState: async () => ({ sessionFile: "/tmp/s.jsonl" }),
      getLastAssistantText: async () => "the last thing it said",
    } as unknown as RpcClient;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true, streaming: true, client });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    attachEventListener(pi.api, registry, record, client);

    // The child files its answer to the OLD task but has not settled yet.
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: old answer" } });
    assert.equal(record.pendingFinal, "Outcome: old answer");

    await sendToAgent(registry, { pi: pi.api, cwd: "/tmp" }, "a", "actually, do this instead");

    assert.deepEqual(calls, [["followUp", "actually, do this instead"]], "the streaming branch, not promptAgent");
    assert.equal(record.pendingFinal, undefined, "the stale final belongs to the replaced task");
    assert.equal(record.running, true);
    assert.equal(record.terminalThisTurn, false);

    listener?.({ type: "agent_settled" });
    await new Promise((resolve) => setImmediate(resolve));

    assert.deepEqual(
      pi.sent.map((s) => s.message.customType),
      ["ws-agent-settled"],
      "the settle after the new instruction must not flush the old final as its answer",
    );
  });

  test("260905: a rejecting live client is treated as an exited child (markAgentExited) and the error still propagates", async () => {
    const { client } = fakeRpcClient({
      prompt: async () => {
        throw new Error("child is gone");
      },
    });
    const record = freshRpcRecord({ agentId: "a", client, streaming: false, running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    const pi = fakePi();

    await assert.rejects(() => sendToAgent(registry, { pi: pi.api, cwd: "/tmp" }, "a", "hi"), /child is gone/);

    assert.equal(record.client, undefined, "the dead child's client is cleared, so the next send takes the resume branch");
    assert.equal(record.running, false, "a dead child must stop counting toward the fan-in");
    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.customType, "ws-agent-settled");
    assert.equal((pi.sent[0].message.details as { reason?: string }).reason, "exited");
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

describe("getAgentTranscriptPath", () => {
  test("known agent id returns { transcript_path: record.sessionPath }", () => {
    const record = freshRpcRecord({ agentId: "a", sessionPath: "/tmp/ws-pi-agent-x/session.jsonl" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(getAgentTranscriptPath(registry, "a"), { transcript_path: "/tmp/ws-pi-agent-x/session.jsonl" });
  });

  test("unknown agent id throws matching /unknown agentId/", () => {
    const registry: RpcAgentRegistry = new Map();
    assert.throws(() => getAgentTranscriptPath(registry, "missing"), /unknown agentId/);
  });
});

// ---------------------------------------------------------------------------
// Spawned-child process-role env marker (review fix, cycle 1; renamed
// 260904 Phase 1 from the boolean `WS_PI_AGENT_CHILD_ENV` to the
// role-valued `WS_PI_SPAWN_ROLE_ENV`, see process-role.ts): placement,
// previously covered only by a manual spot-check. Each spawn call site's
// env-building is a pure function (buildRpcClientOptions for the RPC path,
// buildChildProcessEnv for the one-shot `explore` path via spawnPiProcess),
// so both are asserted directly without spawning a real process. See
// goal-loop.test.ts's `isChildProcess` suite and process-role.test.ts for
// the consuming-side coverage.
// ---------------------------------------------------------------------------

describe("buildRpcClientOptions (WS_PI_SPAWN_ROLE_ENV / WS_PI_APPROVAL_DIR_ENV placement)", () => {
  test("built options carry the worker role marker and the approvals dir derived from sessionPath's own directory", () => {
    const options = buildRpcClientOptions("/repo", "provider/model", "/tmp/ws-pi-agent-x/session.jsonl", "/tmp/system.md", "read,bash");
    assert.deepEqual(options.env, {
      [WS_PI_SPAWN_ROLE_ENV]: "worker",
      [WS_PI_APPROVAL_DIR_ENV]: "/tmp/ws-pi-agent-x/approvals",
    });
  });

  test("env carries exactly the role marker and the approvals dir — nothing else (RpcClient.start() merges it over process.env itself, so this function must not pre-spread it)", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-y/session.jsonl", "/tmp/system.md", "read");
    assert.deepEqual(new Set(Object.keys(options.env ?? {})), new Set([WS_PI_SPAWN_ROLE_ENV, WS_PI_APPROVAL_DIR_ENV]));
  });

  test("260904 Phase 1: the approvals dir is inert-but-present even for a non-execute-worker (full-worker) spawn — WS_PI_APPROVAL_DIR is always derived from sessionPath, not gated on tools", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-z/session.jsonl", "/tmp/system.md", resolveTools("full-worker"));
    assert.equal(options.env?.[WS_PI_APPROVAL_DIR_ENV], "/tmp/ws-pi-agent-z/approvals");
  });

  test('260904 Phase 1 (side-thread fork): forkFrom set emits ["--fork", forkFrom, ...] instead of ["--session", sessionPath, ...], and sets the role marker to "fork"', () => {
    const options = buildRpcClientOptions(
      "/repo",
      undefined,
      "/tmp/ws-pi-agent-w/session.jsonl",
      "/tmp/system.md",
      "read,bash",
      "/lead/session.jsonl",
    );
    assert.deepEqual(options.args, ["--fork", "/lead/session.jsonl", "--append-system-prompt", "/tmp/system.md", "--tools", "read,bash"]);
    assert.equal(options.env?.[WS_PI_SPAWN_ROLE_ENV], "fork");
  });

  test("forkFrom + parentSessionKey sets WS_PI_PARENT_SESSION_KEY_ENV on the child's env", () => {
    const options = buildRpcClientOptions(
      "/repo",
      undefined,
      "/tmp/ws-pi-agent-w2/session.jsonl",
      "/tmp/system.md",
      "read",
      "/lead/session.jsonl",
      "lead-key-123",
    );
    assert.equal(options.env?.[WS_PI_PARENT_SESSION_KEY_ENV], "lead-key-123");
  });

  test("forkFrom without a parentSessionKey omits WS_PI_PARENT_SESSION_KEY_ENV entirely", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w3/session.jsonl", "/tmp/system.md", "read", "/lead/session.jsonl");
    assert.equal(WS_PI_PARENT_SESSION_KEY_ENV in (options.env ?? {}), false);
  });

  test("no forkFrom (the existing worker/execute-worker path): --session branch and role=worker are unchanged", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w4/session.jsonl", "/tmp/system.md", "read");
    assert.deepEqual(options.args, ["--session", "/tmp/ws-pi-agent-w4/session.jsonl", "--append-system-prompt", "/tmp/system.md", "--tools", "read"]);
    assert.equal(options.env?.[WS_PI_SPAWN_ROLE_ENV], "worker");
    assert.equal(WS_PI_PARENT_SESSION_KEY_ENV in (options.env ?? {}), false);
  });
});

describe("buildChildProcessEnv (WS_PI_SPAWN_ROLE_ENV placement for spawnPiProcess)", () => {
  test("sets the spawned-child marker to \"explore\"", () => {
    const env = buildChildProcessEnv({});
    assert.equal(env[WS_PI_SPAWN_ROLE_ENV], "explore");
  });

  test("preserves every inherited variable from the base env (no dropped vars)", () => {
    const env = buildChildProcessEnv({ PATH: "/usr/bin", HOME: "/home/user" });
    assert.equal(env.PATH, "/usr/bin");
    assert.equal(env.HOME, "/home/user");
    assert.equal(env[WS_PI_SPAWN_ROLE_ENV], "explore");
  });

  test("an existing WS_PI_SPAWN_ROLE value in the base env is overwritten to \"explore\"", () => {
    const env = buildChildProcessEnv({ [WS_PI_SPAWN_ROLE_ENV]: "stale" });
    assert.equal(env[WS_PI_SPAWN_ROLE_ENV], "explore");
  });
});
