import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  resolveTools,
  applyRpcEvent,
  markAgentExited,
  promptAgent,
  pushSpawnFailed,
  recordReport,
  stopAgent,
  REPORT_LOG_CAP,
  REPORT_TO_LEAD_TOOL_NAME,
  GATED_EXEC_TOOL_NAME,
  listAgents,
  sendToAgent,
  buildRpcClientOptions as buildRpcClientOptionsBase,
  resolveAgentId,
  resolveAgentRegistryCap,
  reserveAgentAlias,
  evictForCapacity,
  runSpawnGuards,
  truncatePromptForStorage,
  WS_PI_AGENT_REGISTRY_CAP_ENV,
  DEFAULT_AGENT_REGISTRY_CAP,
  PROMPT_STORAGE_CAP_BYTES,
  type RpcAgentRecord,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV, type SpawnRole } from "../src/process-role.ts";
import { classifyRegistryRowState } from "../src/agent-widget.ts";
// 260906 (registerAgentTools's role-keyed explore registration): a VALUE
// import, not `import type` — the new describe block below monkey-patches
// RpcClient.prototype.{start,onEvent,prompt} for the lead/fork execute() path
// so the real spawnAgent code runs to completion with no real subprocess
// (see that describe block's own doc comment for why this is safe and the
// worker/execute-worker leaf path deliberately never invokes execute() at all).
import { RpcClient } from "@earendil-works/pi-coding-agent";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { allocateAgentHome, createAgentStorageContext, updateOwnership } from "../src/agent-storage.ts";
import { type AgentViewModel } from "../src/agent-view.ts";
import { batchViews, createBatchBuffer, observeBatchEvent } from "../src/agent-batches.ts";
import { freshRpcRecord, liveRpcRecord, fakePi, fakeRpcClient } from "./fixtures/spawner-records.ts";

// Deliberately contains spaces: argv is passed as an array, so this exact
// loaded-entry identity must reach the child without shell escaping/rebuilds.
const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";

function buildRpcClientOptions(...args: any[]) {
  return buildRpcClientOptionsBase(
    args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], TEST_EXTENSION_ENTRY, ...args.slice(10),
  );
}

describe("promptAgent (the single prompt funnel)", () => {
  test("preserves a running run anchor across nudges, then re-arms after settle", async (t) => {
    let now = 100;
    t.mock.method(Date, "now", () => now);
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ running: true, streaming: true, runStartedAt: 10 });

    await promptAgent(record, client, "nudge", { isLeadPrompt: false });
    assert.equal(record.runStartedAt, 10, "a nudge joins the active run rather than resetting its duration");

    now = 200;
    applyRpcEvent(record, { type: "agent_settled" });
    assert.equal(record.settledAt, 200);

    now = 300;
    await promptAgent(record, client, "next run");
    assert.equal(record.runStartedAt, 300);
    assert.equal(record.settledAt, undefined);
  });

  test("latches execution, advances the generation, and forwards the message", async () => {
    const { client, calls } = fakeRpcClient();
    const r = freshRpcRecord({ workGeneration: 4, terminalDelivery: { generation: 4, family: "ws-agent-settled", payload: {}, state: "enqueued" } });
    await promptAgent(r, client, "do work");
    assert.equal(r.running, true);
    assert.equal(r.workGeneration, 5);
    assert.equal(r.terminalDelivery, undefined);
    assert.deepEqual(calls, [["prompt", "do work"]]);
  });
});

describe("recordReport", () => {
  test("keeps a bounded informational report log", () => {
    const r = freshRpcRecord();
    for (let i = 0; i < REPORT_LOG_CAP + 2; i++) recordReport(r, i % 2 ? "question" : undefined, i);
    assert.equal(r.reportLog.length, REPORT_LOG_CAP);
    assert.equal(r.reportLog[0]?.at, 2);
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
    const chatty = freshRpcRecord({ agentId: "chatty", reportLog: [{ at: 1_700_000_000_000 }, { at: 1_700_000_060_000 }] });
    const registry: RpcAgentRegistry = new Map([
      ["quiet", quiet],
      ["chatty", chatty],
    ]);
    assert.deepEqual(listAgents(registry), [
      { agent_id: "quiet", status: "dormant" },
      { agent_id: "chatty", status: "dormant", last_report_at: new Date(1_700_000_060_000).toISOString() },
    ]);
  });

  test("status exposes execution, descendant waiting, and pending delivery distinctly", () => {
    const running = freshRpcRecord({ agentId: "a", running: true });
    const waiting = freshRpcRecord({ agentId: "b", waitingOnChildren: true });
    const delivery = freshRpcRecord({ agentId: "c", terminalDelivery: { generation: 1, family: "ws-agent-settled", payload: {}, state: "held" } });
    const registry: RpcAgentRegistry = new Map([["a", running], ["b", waiting], ["c", delivery]]);
    assert.deepEqual(listAgents(registry).map(({ agent_id, status }) => ({ agent_id, status })), [
      { agent_id: "a", status: "running" },
      { agent_id: "b", status: "waiting-on-children" },
      { agent_id: "c", status: "pending-delivery" },
    ]);
  });

  test("260905 (alias/park/cap): alias/title are included on a row when set, omitted when not", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", freshRpcRecord({ agentId: "a", alias: "scout", title: "Reviews auth" })],
      ["b", freshRpcRecord({ agentId: "b" })],
    ]);
    assert.deepEqual(listAgents(registry), [
      { agent_id: "a", status: "dormant", alias: "scout", title: "Reviews auth" },
      { agent_id: "b", status: "dormant" },
    ]);
  });

  test("260905 (alias/park/cap): prompt is omitted by default and included only when include_prompt is set", () => {
    const record = freshRpcRecord({ agentId: "a", prompt: "review src/auth.ts" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "dormant" }], "off by default");
    assert.deepEqual(listAgents(registry, { includePrompt: true }), [{ agent_id: "a", status: "dormant", prompt: "review src/auth.ts" }]);
  });

  test("260905 (alias/park/cap): include_prompt on a record with no stored prompt adds no prompt key", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(listAgents(registry, { includePrompt: true }), [{ agent_id: "a", status: "dormant" }]);
  });

  test("owner-held records expose owner_held:true and lead-held records omit the field", () => {
    const owner = freshRpcRecord({ agentId: "owner", lastWriter: "owner" });
    const lead = freshRpcRecord({ agentId: "lead", lastWriter: "lead" });
    assert.deepEqual(listAgents(new Map([["owner", owner], ["lead", lead]])).map((row) => row.owner_held), [true, undefined]);
  });

  test("260905 (list-model/last-report-fidelity): modelBase + modelEffort lists model as \"<base>/<effort>\"", () => {
    const record = freshRpcRecord({ agentId: "a", modelBase: "claude-opus-4", modelEffort: "high" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "dormant", model: "claude-opus-4/high" }]);
  });

  test("260905 (list-model/last-report-fidelity): modelBase alone lists the bare base", () => {
    const record = freshRpcRecord({ agentId: "a", modelBase: "claude-opus-4" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "dormant", model: "claude-opus-4" }]);
  });

  test("260905 (list-model/last-report-fidelity): a record with neither modelBase nor modelEffort has no model key", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(listAgents(registry), [{ agent_id: "a", status: "dormant" }]);
  });

  test("an ordinary spawned record's row carries no `warning` key (the field was removed: children never report parent-model substitution as a list-row warning)", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    const [row] = listAgents(registry);
    assert.deepEqual(row, { agent_id: "a", status: "dormant" });
    assert.ok(!("warning" in row), "listAgents row must never carry a warning key");
  });
});

describe("sendToAgent", () => {
  test("internal sends keep steer/followUp selection while idle sends use prompt", async () => {
    const activeClient = fakeRpcClient();
    const active = freshRpcRecord({ agentId: "active", client: activeClient.client, running: true, streaming: true });
    const idleClient = fakeRpcClient();
    const idle = freshRpcRecord({ agentId: "idle", client: idleClient.client });
    const registry = new Map([[active.agentId, active], [idle.agentId, idle]]);
    await sendToAgent(registry, { cwd: "/tmp" }, "active", "queued", false);
    await sendToAgent(registry, { cwd: "/tmp" }, "active", "urgent", true);
    await sendToAgent(registry, { cwd: "/tmp" }, "idle", "next");
    assert.deepEqual(activeClient.calls, [["followUp", "queued"], ["steer", "urgent"]]);
    assert.deepEqual(idleClient.calls, [["prompt", "next"]]);
    assert.equal(idle.running, true);
  });

  test("a real successor instruction invalidates an earlier terminal delivery", async () => {
    const fake = fakeRpcClient();
    const r = freshRpcRecord({ agentId: "a", client: fake.client, terminalDelivery: { generation: 1, family: "ws-agent-settled", payload: {}, state: "enqueued" } });
    await sendToAgent(new Map([[r.agentId, r]]), { cwd: "/tmp" }, r.agentId, "replacement");
    assert.equal(r.terminalDelivery, undefined);
    assert.equal(r.workGeneration, 1);
  });

  test("unknown ids fail loudly", async () => {
    await assert.rejects(() => sendToAgent(new Map(), { cwd: "/tmp" }, "missing", "hi"), /unknown agentId/);
  });
});

describe("buildRpcClientOptions (WS_PI_SPAWN_ROLE_ENV placement)", () => {
  test("neutralizes stale bootstrap overrides in the effective RPC environment for workers, forks, persistent explores, and dormant resumes", () => {
    const parent = {
      WS_MCP_BOOTSTRAP_BINARY: "/stale/ws-mcp",
      WS_MCP_BOOTSTRAP_URL: "https://example.test/stale-ws-mcp",
      CHILD_SENTINEL: "preserved",
    };
    const cases = [
      buildRpcClientOptions("/repo", undefined, "/tmp/worker.jsonl", undefined, "read"),
      buildRpcClientOptions("/repo", undefined, "/tmp/fork.jsonl", undefined, "read", "/lead.jsonl"),
      buildRpcClientOptions("/repo", undefined, "/tmp/explore.jsonl", undefined, "read", undefined, undefined, "explore"),
      // Dormant resume calls this same builder with the record's stored role.
      buildRpcClientOptions("/repo", undefined, "/tmp/resume.jsonl", undefined, "read", undefined, undefined, "worker"),
    ];

    for (const options of cases) {
      assert.equal(options.env?.WS_MCP_BOOTSTRAP_BINARY, "");
      assert.equal(options.env?.WS_MCP_BOOTSTRAP_URL, "");
      const effective = { ...parent, ...options.env };
      assert.equal(effective.WS_MCP_BOOTSTRAP_BINARY, "");
      assert.equal(effective.WS_MCP_BOOTSTRAP_URL, "");
      assert.equal(effective.CHILD_SENTINEL, "preserved");
    }
    assert.deepEqual(parent, {
      WS_MCP_BOOTSTRAP_BINARY: "/stale/ws-mcp",
      WS_MCP_BOOTSTRAP_URL: "https://example.test/stale-ws-mcp",
      CHILD_SENTINEL: "preserved",
    }, "building RPC options never mutates the parent environment");
  });

  test("clears the lead's mailbox identity env for every spawn role, forks and ask forks included", () => {
    // RpcClient overlays options.env onto process.env inside the Pi library,
    // so the explicit empty markers below are the whole contract this module
    // owns; there is no local overlay seam to exercise.
    const cases: Array<{ label: string; options: ReturnType<typeof buildRpcClientOptions> }> = [
      { label: "worker", options: buildRpcClientOptions("/repo", undefined, "/tmp/worker.jsonl", undefined, "read") },
      { label: "execute-worker", options: buildRpcClientOptions("/repo", undefined, "/tmp/exec.jsonl", undefined, "read", undefined, undefined, "execute-worker") },
      { label: "explore", options: buildRpcClientOptions("/repo", undefined, "/tmp/explore.jsonl", undefined, "read", undefined, undefined, "explore") },
      { label: "session fork", options: buildRpcClientOptions("/repo", undefined, "/tmp/fork.jsonl", undefined, "read", "/lead.jsonl") },
      // ws-fork and the ask discussion fork reach this builder through
      // spawnAgent with an explicit fork role and a fork-context launch.
      { label: "context fork (ws-fork / ask)", options: buildRpcClientOptions("/repo", undefined, "/tmp/ask.jsonl", undefined, "read", undefined, "parent-key", "fork", undefined, { contextPath: "/tmp/ctx.md" }) },
    ];

    for (const { label, options } of cases) {
      assert.equal(options.env?.WS_MAILBOX, "", `${label} clears WS_MAILBOX`);
      assert.equal(options.env?.WS_MAILBOX_AUTO, "", `${label} clears WS_MAILBOX_AUTO`);
    }
  });

  test("every persistent role disables ambient discovery and loads exactly the captured entry without changing its tools", () => {
    const cases: Array<{ role: SpawnRole; tools: string }> = [
      { role: "worker", tools: "read,bash,ws-report-to-lead" },
      { role: "execute-worker", tools: `read,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME}` },
      { role: "fork", tools: "read,ws-execute" },
      { role: "explore", tools: "read,grep,find,ls" },
    ];
    for (const { role, tools } of cases) {
      const options = buildRpcClientOptions("/repo", undefined, "/tmp/resume.jsonl", "/tmp/prompt.md", tools, undefined, undefined, role);
      const noExtensions = options.args!.indexOf("--no-extensions");
      assert.ok(noExtensions >= 0, `${role} disables ambient extension discovery`);
      assert.deepEqual(options.args!.slice(noExtensions, noExtensions + 3), ["--no-extensions", "--extension", TEST_EXTENSION_ENTRY], `${role} receives the exact loaded entry path`);
      assert.equal(options.args![options.args!.indexOf("--tools") + 1], tools, `${role} keeps its existing allowlist`);
    }
  });

  test("built options carry the worker role marker and no approvals dir: the decision rides the control channel, so the child gets no directory to watch", () => {
    const options = buildRpcClientOptions("/repo", "provider/model", "/tmp/ws-pi-agent-x/session.jsonl", "/tmp/system.md", "read,bash");
    assert.deepEqual(options.env, {
      [WS_PI_SPAWN_ROLE_ENV]: "worker",
      WS_PI_EXPLORE_MODE: "",
      WS_PI_FORK_CONTEXT: "",
      WS_PI_FORK_AFFINITY: "",
      [WS_PI_PARENT_SESSION_KEY_ENV]: "",
      WS_PI_DELEGATION_POLICY: "",
      WS_PI_WEB_HOME: "",
      WS_PI_CHANNEL_ENDPOINT: "",
      WS_PI_CHANNEL_CREDENTIAL: "",
      WS_PI_CHANNEL_GENERATION: "",
      WS_MCP_BOOTSTRAP_BINARY: "",
      WS_MCP_BOOTSTRAP_URL: "",
      WS_MAILBOX: "",
      WS_MAILBOX_AUTO: "",
    });
  });

  test("env overrides an inherited exploration mode while preserving the role marker", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-y/session.jsonl", "/tmp/system.md", "read");
    assert.deepEqual(new Set(Object.keys(options.env ?? {})), new Set([WS_PI_SPAWN_ROLE_ENV, "WS_PI_EXPLORE_MODE", "WS_PI_FORK_CONTEXT", "WS_PI_FORK_AFFINITY", WS_PI_PARENT_SESSION_KEY_ENV, "WS_PI_DELEGATION_POLICY", "WS_PI_WEB_HOME", "WS_PI_CHANNEL_ENDPOINT", "WS_PI_CHANNEL_CREDENTIAL", "WS_PI_CHANNEL_GENERATION", "WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL", "WS_MAILBOX", "WS_MAILBOX_AUTO"]));
    assert.equal(options.env?.WS_PI_EXPLORE_MODE, "");
  });

  test("260924: no role, execute-worker included, receives any approval env — the retired decision-file rendezvous has no env at all", () => {
    for (const tools of [resolveTools("full-worker"), resolveTools("execute-worker")]) {
      const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-z/session.jsonl", "/tmp/system.md", tools);
      assert.ok(!Object.keys(options.env ?? {}).some((key) => key.includes("APPROVAL")), tools);
    }
  });

  test('forks keep their copied session under the owned --session-dir', () => {
    const options = buildRpcClientOptions(
      "/repo",
      undefined,
      "/tmp/ws-pi-agent-w/session.jsonl",
      "/tmp/system.md",
      "read,bash",
      "/lead/session.jsonl",
    );
    assert.deepEqual(options.args, ["--fork", "/lead/session.jsonl", "--session-dir", "/tmp/ws-pi-agent-w", "--no-extensions", "--extension", TEST_EXTENSION_ENTRY, "--tools", "read,bash"]);
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

  test("every role exports its policy parent, preferring it over a stale fork-context parent", () => {
    for (const role of ["worker", "explore", "fork"] as const) {
      const policy = { version: 1 as const, depth: 1, maxDepth: 2, authority: "lead" as const, tools: ["read"], parentSessionKey: "current-lead" };
      const options = buildRpcClientOptions("/repo", undefined, "/tmp/child/session.jsonl", undefined, "read", undefined, "old-lead", role, undefined, undefined, policy);
      assert.equal(options.env?.[WS_PI_PARENT_SESSION_KEY_ENV], "current-lead");
    }
    const roleOnly = buildRpcClientOptions("/repo", undefined, "/tmp/child/session.jsonl", undefined, "read", undefined, "current-lead", "worker");
    assert.equal(roleOnly.env?.[WS_PI_PARENT_SESSION_KEY_ENV], "current-lead");
  });

  test("forkFrom without a parentSessionKey clears inherited WS_PI_PARENT_SESSION_KEY_ENV", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w3/session.jsonl", "/tmp/system.md", "read", "/lead/session.jsonl");
    assert.equal(options.env?.[WS_PI_PARENT_SESSION_KEY_ENV], "");
  });

  test("workers provide their owned session directory", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w4/session.jsonl", "/tmp/system.md", "read");
    assert.deepEqual(options.args, ["--session", "/tmp/ws-pi-agent-w4/session.jsonl", "--session-dir", "/tmp/ws-pi-agent-w4", "--append-system-prompt", "/tmp/system.md", "--no-extensions", "--extension", TEST_EXTENSION_ENTRY, "--tools", "read"]);
    assert.equal(options.env?.[WS_PI_SPAWN_ROLE_ENV], "worker");
    assert.equal(options.env?.[WS_PI_PARENT_SESSION_KEY_ENV], "");
  });

  test("260906 (lead explore as an async RPC child): spawnRoleOverride:\"explore\" wins outright over the forkFrom?fork:worker default", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w5/session.jsonl", "/tmp/system.md", "read,grep,find,ls,bash", undefined, undefined, "explore");
    assert.equal(options.env?.[WS_PI_SPAWN_ROLE_ENV], "explore");
    assert.deepEqual(options.args, ["--session", "/tmp/ws-pi-agent-w5/session.jsonl", "--session-dir", "/tmp/ws-pi-agent-w5", "--append-system-prompt", "/tmp/system.md", "--no-extensions", "--extension", TEST_EXTENSION_ENTRY, "--tools", "read,grep,find,ls,bash"]);
  });

  test("260906: omitting spawnRoleOverride preserves today's forkFrom?fork:worker behavior unchanged", () => {
    const withoutFork = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w6/session.jsonl", "/tmp/system.md", "read");
    assert.equal(withoutFork.env?.[WS_PI_SPAWN_ROLE_ENV], "worker");
    const withFork = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-w7/session.jsonl", "/tmp/system.md", "read", "/lead/session.jsonl");
    assert.equal(withFork.env?.[WS_PI_SPAWN_ROLE_ENV], "fork");
  });
});

describe("resolveAgentRegistryCap", () => {
  test("defaults to DEFAULT_AGENT_REGISTRY_CAP (256) when the env var is unset", () => {
    assert.equal(resolveAgentRegistryCap({}), DEFAULT_AGENT_REGISTRY_CAP);
    assert.equal(DEFAULT_AGENT_REGISTRY_CAP, 256);
  });

  test("a positive numeric override is honored", () => {
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "10" }), 10);
  });

  test("a non-numeric, empty, zero or negative override falls back to the default rather than producing an unusable cap", () => {
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "not-a-number" }), DEFAULT_AGENT_REGISTRY_CAP);
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "" }), DEFAULT_AGENT_REGISTRY_CAP);
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "0" }), DEFAULT_AGENT_REGISTRY_CAP);
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "-5" }), DEFAULT_AGENT_REGISTRY_CAP);
  });

  test("a fractional override is floored", () => {
    assert.equal(resolveAgentRegistryCap({ [WS_PI_AGENT_REGISTRY_CAP_ENV]: "10.7" }), 10);
  });
});

describe("truncatePromptForStorage (byte-safe head-truncation for stored prompts)", () => {
  test("a prompt at or under the cap round-trips unchanged", () => {
    assert.equal(truncatePromptForStorage("short prompt", 4096), "short prompt");
    assert.equal(truncatePromptForStorage("x".repeat(PROMPT_STORAGE_CAP_BYTES)).length, PROMPT_STORAGE_CAP_BYTES);
  });

  test("a prompt over the cap is cut and gets a truncation marker appended", () => {
    const prompt = "y".repeat(20);
    const truncated = truncatePromptForStorage(prompt, 10);
    assert.ok(truncated.startsWith("y".repeat(10)));
    assert.ok(truncated.includes("truncated"), "a cut-marker line must say the prompt was cut");
  });

  test("a byte-boundary cut never splits a multibyte UTF-8 codepoint — the partial trailing sequence is dropped cleanly", () => {
    // Each "🙂" is 4 UTF-8 bytes; cutting at 10 bytes lands mid-emoji (byte 8
    // is the second byte of the third emoji).
    const prompt = "🙂🙂🙂🙂🙂";
    const truncated = truncatePromptForStorage(prompt, 10);
    // The two complete emoji (8 bytes) must survive; nothing corrupt (no
    // replacement character, no half-codepoint) may appear before the marker.
    assert.ok(truncated.startsWith("🙂🙂"));
    assert.ok(!truncated.slice(0, truncated.indexOf("\n")).includes("�"), "no replacement character from a split codepoint");
  });

  test("default cap is PROMPT_STORAGE_CAP_BYTES (4096)", () => {
    assert.equal(PROMPT_STORAGE_CAP_BYTES, 4096);
    const prompt = "z".repeat(5000);
    const truncated = truncatePromptForStorage(prompt);
    assert.ok(truncated.length < prompt.length);
  });
});

/**
 * 260905 (alias/park/cap ticket): the alias half of `spawnAgent`'s guard
 * clauses, extracted for direct coverage since `spawnAgent` itself
 * constructs a real `RpcClient` and is live-gate only.
 *
 * Review relay #1 (Critical fix): `reserveAgentAlias` itself no longer
 * mutates a holder's `alias` — it only locates and validates, returning the
 * holder (when one is found and not blocked) for the caller to clear. The
 * actual clear-and-commit is `runSpawnGuards`'s job (see its own describe
 * block below), run only after every guard has succeeded.
 */
describe("reserveAgentAlias", () => {
  test("no alias requested is always a no-op ok", () => {
    const registry: RpcAgentRegistry = new Map();
    assert.deepEqual(reserveAgentAlias(registry, undefined), { ok: true });
  });

  test("an alias with no current holder is a no-op ok (fresh alias)", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(reserveAgentAlias(registry, "scout"), { ok: true });
  });

  test("a dormant holder is returned as `holder`, unmutated — title AND alias both untouched by this function alone", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", title: "keep me" });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = reserveAgentAlias(registry, "scout");
    assert.deepEqual(result, { ok: true, holder });
    assert.equal(holder.alias, "scout", "reserveAgentAlias alone never clears it — that's runSpawnGuards's job");
    assert.equal(holder.title, "keep me");
  });

  test("an idle (live but not running) holder is also returned as `holder`, unmutated", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", client: {} as RpcClient, running: false });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    assert.deepEqual(reserveAgentAlias(registry, "scout"), { ok: true, holder });
    assert.equal(holder.alias, "scout");
  });

  test("a RUNNING holder's alias rejects the spawn — reused, not silently skipped", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", running: true });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = reserveAgentAlias(registry, "scout");
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.match(result.error, /scout/);
      assert.match(result.error, /running/);
    }
    assert.equal(holder.alias, "scout", "a rejected spawn leaves the holder's alias untouched");
  });

  test("a threadBound holder's alias also rejects the spawn", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", threadBound: true });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = reserveAgentAlias(registry, "scout");
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.match(result.error, /threadBound/);
    }
  });

  test("an owner-held holder's alias also rejects the spawn", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", lastWriter: "owner" });
    const result = reserveAgentAlias(new Map([["a", holder]]), "scout");
    assert.equal(result.ok, false);
    if (!result.ok) assert.match(result.error, /held\/outstanding/);
  });
});

/**
 * 260905 (alias/park/cap ticket): the cap half of `spawnAgent`'s guard
 * clauses, extracted for direct coverage for the same reason as
 * `reserveAgentAlias`.
 */
describe("pending approval retirement", () => {
  test("process exits and explicit stops clear pending approval, hide the gutter row, and permit eviction", async () => {
    const exited = liveRpcRecord({ agentId: "exited", running: true, pendingApproval: { cmdId: "call-1", command: "echo hi" } });
    const exitedRegistry: RpcAgentRegistry = new Map([[exited.agentId, exited]]);
    markAgentExited(undefined, exitedRegistry, exited, { suppressTerminal: true });
    assert.equal(exited.pendingApproval, undefined);
    assert.equal(classifyRegistryRowState(exited), undefined);
    assert.deepEqual(evictForCapacity(exitedRegistry, 1), { ok: true, evictedLabel: "exited" });

    const stopped = freshRpcRecord({
      agentId: "stopped",
      client: {
        abort: async () => undefined,
        stop: async () => undefined,
      } as unknown as RpcClient,
      running: true,
      pendingApproval: { cmdId: "call-2", command: "echo bye" },
    });
    const stoppedRegistry: RpcAgentRegistry = new Map([[stopped.agentId, stopped]]);
    await stopAgent(stoppedRegistry, stopped.agentId, undefined, { silent: true, skipCostCheckpoint: true });
    assert.equal(stopped.pendingApproval, undefined);
    assert.equal(classifyRegistryRowState(stopped), undefined);
    assert.deepEqual(evictForCapacity(stoppedRegistry, 1), { ok: true, evictedLabel: "stopped" });
  });
});

/**
 * 261028 relay wiring: `clearLiveState` ends every launch (stop, exit, failed
 * launch). A launch that ends mid-batch must not leave the batch running, and
 * the relayed descendant model belongs to the launch, so it is dropped with it.
 */
describe("clearLiveState: tool batch and relayed model on launch end (261028)", () => {
  /** A live record with one batch still open and a relayed model for a descendant. */
  function relayRecord(): RpcAgentRecord {
    const toolBatches = createBatchBuffer();
    observeBatchEvent(toolBatches, { type: "message_end", message: { role: "assistant", content: [{ type: "toolCall", id: "c1", name: "read", arguments: { path: "a.ts" } }] } }, 1_000);
    observeBatchEvent(toolBatches, { type: "tool_execution_start", toolCallId: "c1", toolName: "read" }, 1_001);
    assert.equal(toolBatches.batches[0].endedAt, undefined, "precondition: the batch is open");
    const descendantViews: AgentViewModel = new Map([["grandchild", batchViews(createBatchBuffer())]]);
    return liveRpcRecord({
      agentId: "relay",
      running: true,
      toolBatches,
      descendantViews,
      client: { abort: async () => undefined, stop: async () => undefined } as unknown as RpcClient,
    });
  }

  function assertLaunchEnded(record: RpcAgentRecord): void {
    assert.notEqual(record.toolBatches!.batches[0].endedAt, undefined, "the open batch is closed, not left running");
    assert.equal(record.toolBatches!.batches.length, 1, "history is kept");
    assert.equal(record.descendantViews, undefined, "the launch's relayed model is dropped");
  }

  test("a process exit", () => {
    const record = relayRecord();
    markAgentExited(undefined, new Map([[record.agentId, record]]), record, { suppressTerminal: true });
    assertLaunchEnded(record);
  });

  test("a stop", async () => {
    const record = relayRecord();
    await stopAgent(new Map([[record.agentId, record]]), record.agentId, undefined, { silent: true, skipCostCheckpoint: true });
    assertLaunchEnded(record);
  });

  test("a failed launch", () => {
    const record = relayRecord();
    pushSpawnFailed(fakePi().api, new Map([[record.agentId, record]]), record, new Error("client.start() failed"));
    assertLaunchEnded(record);
  });
});

describe("evictForCapacity", () => {
  test("under the cap is a no-op ok with no eviction", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(evictForCapacity(registry, 5), { ok: true });
    assert.equal(registry.size, 1);
  });

  test("at the cap evicts the dormant record with the OLDEST last-activity stamp", () => {
    const oldest = freshRpcRecord({ agentId: "old", lastOutputAt: 1_000 });
    const newer = freshRpcRecord({ agentId: "new", lastOutputAt: 5_000 });
    const registry: RpcAgentRegistry = new Map([
      ["old", oldest],
      ["new", newer],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "old" });
    assert.equal(registry.has("old"), false);
    assert.equal(registry.has("new"), true);
  });

  test("last-activity follows the output high-water mark rather than input or report bookkeeping", () => {
    const staleOutput = freshRpcRecord({ agentId: "stale", lastOutputAt: 1_500, lastLeadPromptAt: 9_000, reportLog: [{ at: 10_000 }] });
    const freshOutput = freshRpcRecord({ agentId: "fresh", lastOutputAt: 9_000 });
    const registry: RpcAgentRegistry = new Map([
      ["stale", staleOutput],
      ["fresh", freshOutput],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "stale" });
  });

  test("evicted label prefers the record's alias over its bare uuid", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a", alias: "scout" })]]);
    assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "scout" });
  });

  test("running, threadBound, and owner-held records are never evicted", () => {
    const running = freshRpcRecord({ agentId: "running-one", running: true });
    const threadBound = freshRpcRecord({ agentId: "bound-one", threadBound: true });
    const ownerHeld = freshRpcRecord({ agentId: "owner-one", lastWriter: "owner" });
    const registry: RpcAgentRegistry = new Map([
      ["running-one", running],
      ["bound-one", threadBound],
      ["owner-one", ownerHeld],
    ]);
    const result = evictForCapacity(registry, 3);
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.match(result.error, /cap/);
    }
    assert.equal(registry.size, 3, "a rejected eviction must not remove anything");
  });

  test("a live (client-holding) idle record is never evicted either — only a fully dormant record is a candidate", () => {
    const live = freshRpcRecord({ agentId: "live-one", client: {} as RpcClient, running: false });
    const registry: RpcAgentRegistry = new Map([["live-one", live]]);
    const result = evictForCapacity(registry, 1);
    assert.equal(result.ok, false);
  });

  test("a cap of 1 with 3 dormant entries evicts all of them, oldest-first, and joins the labels — the spawn itself will occupy the sole slot", () => {
    const a = freshRpcRecord({ agentId: "a", lastOutputAt: 1_000 });
    const b = freshRpcRecord({ agentId: "b", lastOutputAt: 2_000 });
    const c = freshRpcRecord({ agentId: "c", lastOutputAt: 3_000 });
    const registry: RpcAgentRegistry = new Map([
      ["a", a],
      ["b", b],
      ["c", c],
    ]);
    const result = evictForCapacity(registry, 1);
    assert.deepEqual(result, { ok: true, evictedLabel: "a, b, c" });
    assert.equal(registry.size, 0);
  });

  test("eviction removes a confirmed-stopped owned home as well as its registry entry", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-cap-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "owned-a", "worker");
      updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
      const registry: RpcAgentRegistry = new Map([["owned-a", freshRpcRecord({ agentId: "owned-a", ownership, sessionPath: ownership.sessionPath })]]);
      assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "owned-a" });
      assert.equal(registry.size, 0);
      assert.equal(existsSync(ownership.home), false);
      assert.deepEqual(readdirSync(join(realpathSync(root), "ws-agents")), ["lead-1"]);
      assert.deepEqual(readdirSync(join(realpathSync(root), "ws-agents", "lead-1")), [".cost-estimate"], "eviction retains only the bounded cost checkpoint directory");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("durably unknown or protected owned records and in-memory approval waits are not cap candidates", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-cap-test-"));
    try {
      const unknown = allocateAgentHome(createAgentStorageContext("lead-1", root), "unknown-a", "worker");
      const protectedOwnership = allocateAgentHome(createAgentStorageContext("lead-1", root), "protected-a", "execute-worker");
      updateOwnership(protectedOwnership.home, { liveness: { lifecycle: "stopped", running: false, pendingApprovalCommandId: "call-1" } });
      for (const record of [
        freshRpcRecord({ agentId: "unknown-a", ownership: unknown, sessionPath: unknown.sessionPath }),
        freshRpcRecord({ agentId: "protected-a", ownership: protectedOwnership, sessionPath: protectedOwnership.sessionPath }),
        freshRpcRecord({ agentId: "legacy-approval", pendingApproval: { cmdId: "call-2", command: "echo hi" } }),
      ]) {
        const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
        const result = evictForCapacity(registry, 1);
        assert.equal(result.ok, false, record.agentId);
        assert.equal(registry.size, 1, record.agentId);
      }
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a legacy cap eviction forgets only the registry record and never deletes its unowned path", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-legacy-cap-test-"));
    try {
      const legacyHome = join(root, "ws-pi-agent-legacy");
      mkdirSync(legacyHome);
      const sessionPath = join(legacyHome, "session.jsonl");
      writeFileSync(sessionPath, "legacy history");
      const registry: RpcAgentRegistry = new Map([["legacy", freshRpcRecord({ agentId: "legacy", sessionPath })]]);
      assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "legacy" });
      assert.equal(registry.size, 0);
      assert.equal(readFileSync(sessionPath, "utf8"), "legacy history");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a late durable-retention result aborts capacity eviction and keeps the record", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-cap-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "late-protected", "worker");
      updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
      const registry: RpcAgentRegistry = new Map([["late-protected", freshRpcRecord({ agentId: "late-protected", ownership, sessionPath: ownership.sessionPath })]]);
      const result = evictForCapacity(registry, 1, () => ({ status: "retained", reason: "protection changed" }));
      assert.equal(result.ok, false);
      assert.equal(registry.has("late-protected"), true);
      assert.equal(existsSync(ownership.home), true);
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("an owned-home deletion failure retains the candidate instead of risking a later duplicate fold", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-cap-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "owned-failure", "worker");
      updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
      const registry: RpcAgentRegistry = new Map([["owned-failure", freshRpcRecord({ agentId: "owned-failure", ownership, sessionPath: ownership.sessionPath })]]);
      const result = evictForCapacity(registry, 1, () => ({ status: "failed", error: "permission denied at /private/owned-failure" }));
      assert.deepEqual(result, { ok: false, error: "ws-pi-agent: ws-agent-spawn rejected: owned-home removal failed; the existing agent was retained for safety" });
      assert.doesNotMatch(result.error ?? "", /permission|private|owned-failure/);
      assert.equal(registry.size, 1);
      assert.ok(readFileSync(join(ownership.home, "ownership.json"), "utf8"), "metadata remains for a later retry");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("a revived report timestamp does not impersonate fresh agent output during capacity eviction", () => {
    // Report metadata is retained for listAgents, but the output-only clock
    // must not protect a revived record that has produced no observed output.
    const revived = freshRpcRecord({ agentId: "revived", lastReportAtOverride: new Date(9_000).toISOString() });
    const outputObserved = freshRpcRecord({ agentId: "output", lastOutputAt: 100 });
    const registry: RpcAgentRegistry = new Map([
      ["revived", revived],
      ["output", outputObserved],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "revived" });
    assert.equal(registry.has("output"), true);
  });
});

/**
 * 260905 review relay #1 (Critical fix): `runSpawnGuards` is the single gate
 * `spawnAgent` calls — it runs `reserveAgentAlias` then `evictForCapacity`
 * and commits the alias clear only once BOTH succeed, so a guard rejection
 * never leaves a previous holder's alias half-cleared. This is the direct
 * regression test for the bug: the original code cleared the alias as a
 * side effect of `reserveAgentAlias` itself, so a subsequent cap rejection
 * still left that holder's alias destroyed even though the spawn never
 * registered.
 */
describe("runSpawnGuards (260905 review relay #1: alias-clear-then-cap-reject ordering)", () => {
  test("both guards pass: the previous holder's alias IS cleared", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", title: "keep me" });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = runSpawnGuards(registry, "scout", 10);
    assert.deepEqual(result, { ok: true });
    assert.equal(holder.alias, undefined);
    assert.equal(holder.title, "keep me");
  });

  test("alias reservation rejects (running holder): nothing is touched, cap eviction never even runs", () => {
    const holder = freshRpcRecord({ agentId: "a", alias: "scout", running: true });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = runSpawnGuards(registry, "scout", 1);
    assert.equal(result.ok, false);
    assert.equal(holder.alias, "scout");
    assert.equal(registry.size, 1, "the cap guard never ran, so nothing was evicted either");
  });

  test("CRITICAL regression: a dormant holder's alias reservation succeeds but cap eviction then rejects — the holder's alias is left completely intact, still resolvable by ws-agent-send <alias>", () => {
    // The exact reachable scenario from the review: a registry at cap whose
    // only non-running/non-threadBound member is LIVE-idle (excluded from
    // eviction, but would have been alias-cleared by the old ordering) and
    // holds the reused alias.
    const liveIdleHolder = freshRpcRecord({ agentId: "a", alias: "scout", client: {} as RpcClient, running: false });
    const runningOther = freshRpcRecord({ agentId: "b", running: true });
    const registry: RpcAgentRegistry = new Map([
      ["a", liveIdleHolder],
      ["b", runningOther],
    ]);
    // cap === registry.size: evictForCapacity must evict someone to fit, but
    // neither "a" (live) nor "b" (running) is a legal candidate.
    const result = runSpawnGuards(registry, "scout", 2);
    assert.equal(result.ok, false, "the cap guard correctly rejects the spawn");
    if (!result.ok) {
      assert.match(result.error, /cap/);
    }
    assert.equal(liveIdleHolder.alias, "scout", "the previous holder's alias survives a downstream guard rejection");
    assert.equal(resolveAgentId(registry, "scout"), "a", "still resolvable by alias — ws-agent-send <alias> works unchanged");
    assert.equal(registry.size, 2, "nothing was evicted or removed");
  });

  test("no alias requested: cap guard still runs on its own", () => {
    const holder = freshRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", holder]]);
    const result = runSpawnGuards(registry, undefined, 1);
    assert.equal(result.ok, false);
  });
});
