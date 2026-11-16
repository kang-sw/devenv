import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { assertPolicyTool, DELEGATION_ENV, inheritableTools, terminalTools } from "../src/delegation-policy.ts";
import { isExecutionAgent, RETIRED_NATIVE_AGENT_TOOLS, withoutRetiredNativeTools } from "../src/native-agent-policy.ts";
import { applyRpcEvent, GATED_EXEC_TOOL_NAME, registerAgentTools, REPORT_TO_LEAD_TOOL_NAME, resolveTools, sendToAgent, spawnAgent, TOOL_GROUPS, type RpcAgentRecord } from "../src/spawner.ts";
import { buildOrphanSummary, readExecutionSidecarAt, serializeOrphans, type PersistedOrphan } from "../src/agent-sidecar.ts";
import { notifyNativeSidecarRetirement, reviveSessionOrphans } from "../src/index.ts";
import { registerExecuteGateway } from "../src/execute-gateway.ts";
import { dashboardAgentEntries } from "../src/dashboard-feed.ts";
import { addForkToolIfLead, computeForkToolSurface } from "../src/fork.ts";
import { allocateAgentHome, createAgentStorageContext } from "../src/agent-storage.ts";

function harness() {
  const tools = new Map<string, any>();
  const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), on() {}, sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  const bridge = { client: {}, wsToolNames: [], defaultSessionKeyRef: { current: "lead" } } as never;
  const handle = registerAgentTools(pi, bridge, { cwd: tmpdir(), extensionPath: "/unused/index.ts" });
  return { tools, pi, bridge, handle };
}
function execution(overrides: Partial<PersistedOrphan> = {}): PersistedOrphan {
  return { agentId: "execute-1", sessionPath: "/missing/session.jsonl", systemPromptPath: "/missing/prompt.md", toolGroup: "execute-worker", spawnRole: "execute-worker", wsToolNames: [], state: "running", ...overrides };
}
function temp(t: { after(fn: () => void): void }): string {
  const root = mkdtempSync(join(tmpdir(), "native-retirement-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function withRole(role: string | undefined, tools: string[], fn: () => void): void {
  const oldRole = process.env.WS_PI_SPAWN_ROLE;
  const oldPolicy = process.env[DELEGATION_ENV];
  try {
    if (role === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = role;
    process.env[DELEGATION_ENV] = JSON.stringify({ version: 1, depth: role ? 1 : 0, maxDepth: 2, authority: "leaf", tools });
    fn();
  } finally {
    if (oldRole === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = oldRole;
    if (oldPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = oldPolicy;
  }
}

test("retired names are absent from registration, permissions and lazy/helper activation at every depth", async () => {
  for (const role of [undefined, "worker", "fork", "explore"]) {
    withRole(role, ["read", ...RETIRED_NATIVE_AGENT_TOOLS], () => {
      const h = harness();
      try {
        for (const name of RETIRED_NATIVE_AGENT_TOOLS) assert.equal(h.tools.has(name), false, `${role}: ${name} cannot be found or activated`);
        assert.equal(h.tools.has(REPORT_TO_LEAD_TOOL_NAME), false);
      } finally { void h.handle.stopAll(); }
    });
  }
  for (const group of Object.keys(TOOL_GROUPS) as Array<keyof typeof TOOL_GROUPS>) {
    for (const name of RETIRED_NATIVE_AGENT_TOOLS) assert.equal(resolveTools(group).split(",").includes(name), false);
  }
  for (const depth of [0, 1, 2]) assert.deepEqual(terminalTools(["read", ...RETIRED_NATIVE_AGENT_TOOLS], depth, 2), ["read"]);
  assert.deepEqual(inheritableTools(["read", ...RETIRED_NATIVE_AGENT_TOOLS]), ["read"]);
  assert.deepEqual(withoutRetiredNativeTools(["read", ...RETIRED_NATIVE_AGENT_TOOLS]), ["read"]);
  for (const role of [undefined, "worker", "fork"] as const) assert.deepEqual(addForkToolIfLead(["read", ...RETIRED_NATIVE_AGENT_TOOLS], role), ["read"]);
  assert.deepEqual(computeForkToolSurface(["read", ...RETIRED_NATIVE_AGENT_TOOLS]), ["read"]);
  for (const name of RETIRED_NATIVE_AGENT_TOOLS) assert.throws(() => assertPolicyTool(undefined, name), /retired/);
});

test("progress tool is registered only for the internal execution worker", () => {
  withRole("worker", ["read", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME], () => {
    const h = harness();
    try {
      assert.deepEqual([...h.tools.keys()], [REPORT_TO_LEAD_TOOL_NAME]);
      assert.deepEqual(Object.keys(h.tools.get(REPORT_TO_LEAD_TOOL_NAME).parameters.properties), ["message"]);
    } finally { void h.handle.stopAll(); }
  });
  withRole(undefined, [GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME], () => {
    const h = harness();
    try { assert.equal(h.tools.has(REPORT_TO_LEAD_TOOL_NAME), false); }
    finally { void h.handle.stopAll(); }
  });
});

test("retirement notices reach hosted/RPC diagnostics and model context without an initialization wake", () => {
  const notifications: unknown[] = [];
  const messages: unknown[] = [];
  notifyNativeSidecarRetirement({ sendMessage: (message, options) => messages.push({ message, options }) } as never,
    { notify: (text, type) => notifications.push({ text, type }) }, "preserved at retained.json.retired-id; NOT resumed");
  assert.deepEqual(notifications, [{ text: "[ws Pi] preserved at retained.json.retired-id; NOT resumed", type: "warning" }]);
  assert.deepEqual(messages, [{ message: { customType: "ws-native-retirement", content: "preserved at retained.json.retired-id; NOT resumed", display: true }, options: { deliverAs: "nextTurn" } }]);
});

test("legacy execution metadata without a role or envelope recovers narrow approval-capable permissions", () => {
  const registry = new Map();
  const approval = () => {};
  const restored = reviveSessionOrphans({} as never, registry, [execution({ spawnRole: undefined })], approval);
  assert.equal(restored[0].spawnRole, "execute-worker");
  assert.equal(restored[0].onApprovalPending, approval);
  assert.equal(restored[0].delegation?.authority, "leaf");
  assert.deepEqual(restored[0].delegation?.tools, ["read", "grep", "find", "ls", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME]);
  assert.deepEqual(restored[0].delegation?.write, { mode: "none" });
});

test("runtime registry refuses general/fork launches and resumes before any transport or allocation", async t => {
  const h = harness();
  t.after(() => h.handle.stopAll());
  for (const ctx of [{}, { toolGroup: "full-worker" }, { toolGroup: "execute-worker", spawnRole: "fork" }, { toolGroup: "execute-worker", forkFrom: "/missing" }]) {
    await assert.rejects(spawnAgent(h.handle.rpcRegistry, ctx as never, { prompt: "no launch" }), /execution workers only/);
  }
  const retired = { ...execution({ toolGroup: "full-worker", spawnRole: "worker" }), running: false, streaming: false, reportLog: [] } as RpcAgentRecord;
  h.handle.rpcRegistry.set(retired.agentId, retired);
  await assert.rejects(sendToAgent(h.handle.rpcRegistry, {} as never, retired.agentId, "do not resume"), /retired native agent cannot be resumed/);
  h.handle.rpcRegistry.clear();
});

test("mixed sidecar is quarantined byte-for-byte; only execution records are recoverable", t => {
  const root = temp(t);
  const path = join(root, "registry.json");
  const owned = ["general", "fork", "ambiguous"].map(id => allocateAgentHome(createAgentStorageContext("legacy-owner", root), id, id === "fork" ? "fork" : "worker"));
  const history = owned[0].sessionPath!;
  writeFileSync(history, "retained history\n");
  const raw = serializeOrphans([
    execution(),
    execution({ agentId: "general", toolGroup: "full-worker", spawnRole: "worker", sessionPath: history, ownership: owned[0] }),
    execution({ agentId: "fork", toolGroup: "full-worker", spawnRole: "fork", sessionPath: owned[1].sessionPath!, ownership: owned[1] }),
    execution({ agentId: "ambiguous", spawnRole: "worker", sessionPath: owned[2].sessionPath!, ownership: owned[2] }),
  ]);
  writeFileSync(path, raw);
  const notices: string[] = [];
  const recovery = readExecutionSidecarAt(path, text => notices.push(text));
  assert.equal(recovery.writable, true);
  assert.equal(recovery.retentionSafe, true);
  const recovered = recovery.orphans;
  assert.deepEqual(recovered.map(o => o.agentId), ["execute-1"]);
  assert.equal(existsSync(path), false);
  const archive = readdirSync(root).find(name => name.startsWith("registry.json.retired-"))!;
  assert.equal(readFileSync(join(root, archive), "utf8"), raw);
  assert.equal(readFileSync(history, "utf8"), "retained history\n");
  assert.match(notices[0], /NOT resumed or adopted/);
  assert.match(notices[0], /preserved at/);
  assert.deepEqual(readExecutionSidecarAt(path, text => notices.push(text)).orphans, []);
  assert.equal(notices.length, 1, "consumed source is not falsely recovered twice");
});

test("unrecognized sidecars are preserved instead of silently consumed", t => {
  const root = temp(t);
  for (const [index, raw] of ["{broken", '{"version":99,"orphans":[]}', '{"version":1,"orphans":[{"agentId":"unknown","extra":"preserve me"}]}'].entries()) {
    const path = join(root, `registry-${index}.json`);
    writeFileSync(path, raw);
    const notices: string[] = [];
    const recovery = readExecutionSidecarAt(path, text => notices.push(text));
    assert.deepEqual(recovery.orphans, []);
    assert.equal(recovery.writable, false);
    assert.equal(recovery.retentionSafe, false);
    assert.equal(readFileSync(path, "utf8"), raw);
    assert.match(notices[0], /NOT resumed or adopted/);
    assert.match(notices[0], /retention are blocked/);
  }
});

test("execution-only recovery retains approval hook, progress and dashboard identity without claiming a live process", async t => {
  const root = temp(t);
  const path = join(root, "registry.json");
  writeFileSync(path, serializeOrphans([execution({ delegation: { version: 1, depth: 1, maxDepth: 2, authority: "leaf", tools: ["read", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME, "explore", "ws-agent-spawn"] } })]));
  const recovered = readExecutionSidecarAt(path, () => assert.fail("execution-only sidecar is not quarantined")).orphans;
  assert.equal(existsSync(path), false);
  assert.deepEqual(recovered[0].delegation?.tools, ["read", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME]);
  const h = harness();
  t.after(() => h.handle.stopAll());
  const approval = () => {};
  const revived = reviveSessionOrphans(h.pi, h.handle.rpcRegistry, [...recovered, execution({ agentId: "retired", toolGroup: "full-worker" })], approval);
  assert.equal(revived.length, 1);
  const record = revived[0];
  assert.equal(record.client, undefined);
  assert.equal(record.running, false);
  assert.equal(record.pendingApproval, undefined, "an old command approval is never replayed");
  assert.equal(record.onApprovalPending, approval);
  assert.equal(isExecutionAgent(record), true);
  assert.equal(dashboardAgentEntries(h.handle.rpcRegistry).length, 1);
  const progress = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "checking" } });
  assert.equal(progress.push?.family, "ws-agent-report");
  assert.equal(record.terminalDelivery, undefined, "progress is not completion");
  applyRpcEvent(record, { type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "fresh-command", args: { command: "not executed", rationale: "test" } });
  assert.equal(record.pendingApproval?.cmdId, "fresh-command");
  const sent: unknown[] = [];
  record.channel = { live: true, send: (message: unknown) => sent.push(message) } as never;
  registerExecuteGateway(h.pi, h.bridge, h.handle.rpcRegistry, { cwd: root, executeWorkerPromptPath: "/unused/guide.md", extensionPath: "/unused/index.ts" });
  const approve = h.tools.get("ws-approve");
  await assert.rejects(approve.execute("tool", { agent_id: record.agentId, cmd_id: "stale", decision: "approve" }), /rejected/);
  assert.equal(sent.length, 0);
  await approve.execute("tool", { agent_id: record.agentId, cmd_id: "fresh-command", decision: "deny", reason: "test" });
  assert.equal(sent.length, 1);
  await assert.rejects(approve.execute("tool", { agent_id: record.agentId, cmd_id: "fresh-command", decision: "approve" }), /rejected/);
  assert.equal(sent.length, 1, "a decision is correlated and sent only once");
  record.channel = undefined;
  const notice = buildOrphanSummary(recovered);
  assert.match(notice, /\/audit/);
  assert.match(notice, /No process, in-flight command approval, or report history was recovered/);
  assert.doesNotMatch(notice, /ws-agent-(send|transcript|list)/);
});
