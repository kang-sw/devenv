/** R1 I1–I3 only. Filesystem and in-process RPC/channel fixtures; no launcher or executable. */
import assert from "node:assert/strict";
import childProcess from "node:child_process";
import fs, { existsSync, mkdtempSync, readFileSync, readdirSync, rmSync, statSync, writeFileSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { auditResumeCtx } from "../src/audit.ts";
import { createThreadRegistryHandle } from "../src/ask.ts";
import { readExecutionSidecarAt, serializeOrphans, writeSidecarAt, type PersistedOrphan } from "../src/agent-sidecar.ts";
import { allocateAgentHome, createAgentStorageContext, inspectOwnedHomeRemoval, readOwnership, writeOwnership } from "../src/agent-storage.ts";
import { createApprovalRelay, registerExecuteGateway } from "../src/execute-gateway.ts";
import { applySessionStartAgentRetention, persistShutdownAgentSnapshots, prepareExecutionSidecar } from "../src/index.ts";
import { captureOrphans } from "../src/agent-sidecar.ts";
import { GATED_EXEC_TOOL_NAME, leadIdleRef, markAgentExited, registerAgentTools, sendToAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";

const extensionPath = "/unused/loaded-adapter/index.ts";
function temporary(t: { after(fn: () => void): void }): string {
  const root = mkdtempSync(join(tmpdir(), "native-r1-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function runtime(root: string) {
  const tools = new Map<string, any>();
  const messages: any[] = [];
  const pi = {
    registerTool: (tool: any) => tools.set(tool.name, tool), on() {},
    sendMessage: (message: any) => messages.push(message), sendUserMessage() {},
    exec: () => assert.fail("no command or executable may be launched by these regressions"),
  } as unknown as ExtensionAPI;
  const bridge = { client: { callTool: () => assert.fail("no daemon call expected") }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-current" } } as never;
  const handle = registerAgentTools(pi, bridge, { cwd: root, storage: createAgentStorageContext("owner", root), extensionPath });
  return { pi, bridge, handle, tools, messages };
}
function expiredHistory(root: string, agentId: string, now: number, role: "worker" | "execute-worker" = "worker", owner = "legacy-owner") {
  const ownership = allocateAgentHome(createAgentStorageContext(owner, root), agentId, role);
  const transcript = Buffer.from(`retained transcript ${agentId}\n\u0000\u00ff`);
  const prompt = Buffer.from(`retained prompt ${agentId}\n\u0000\u00fe`);
  writeFileSync(ownership.sessionPath!, transcript);
  const promptPath = join(ownership.home, "prompt.md");
  writeFileSync(promptPath, prompt);
  const signature = statSync(ownership.sessionPath!);
  const metadata = readOwnership(ownership.home)!;
  writeOwnership({ ...metadata, lastActivityAt: now - 10 * 86_400_000,
    sessionSignature: { mtimeMs: signature.mtimeMs, size: signature.size },
    telemetry: { version: 1, origin: { sessionId: agentId, sessionPath: ownership.sessionPath!, emptyPrefix: true }, estimatedUsd: .4 },
    liveness: { lifecycle: "stopped", running: false },
  });
  const orphan: PersistedOrphan = { agentId, ownership, sessionPath: ownership.sessionPath!, systemPromptPath: promptPath, spawnRole: role, toolGroup: role === "worker" ? "full-worker" : "execute-worker", wsToolNames: [], state: "idle" };
  assert.equal(inspectOwnedHomeRemoval(ownership).status, "eligible");
  return { ownership, orphan, transcript, prompt, promptPath };
}

test("I1 fresh production execution gateway keeps its relay after process loss and audit-style dormant resume", async t => {
  const root = temporary(t);
  const r = runtime(root);
  const oldIdle = leadIdleRef.current;
  leadIdleRef.current = () => false; // A running lead receives the real approval steer immediately.
  t.after(() => { leadIdleRef.current = oldIdle; });
  const listeners = new Map<RpcClient, Set<(event: unknown) => void>>();
  const sessionOf = (client: any) => {
    const args: string[] = client.options.args;
    const index = args.indexOf("--session");
    return index >= 0 ? args[index + 1] : join(args[args.indexOf("--session-dir") + 1], "session.jsonl");
  };
  t.mock.method(childProcess, "execFileSync", () => { throw new Error("working-context commands deliberately unavailable"); });
  syncBuiltinESMExports();
  t.mock.method(RpcClient.prototype, "start", async function(this: any) {
    const session = sessionOf(this);
    if (!existsSync(session)) writeFileSync(session, "");
    await connectFakeChild(this.options.env, this.options.args);
  });
  for (const name of ["stop", "abort", "prompt", "setThinkingLevel"] as const) t.mock.method(RpcClient.prototype, name, async () => {});
  t.mock.method(RpcClient.prototype, "getState", async function(this: any) { return { sessionId: "execution-r1", sessionFile: sessionOf(this), model: { provider: "offline", id: "test" }, thinkingLevel: "off" }; });
  t.mock.method(RpcClient.prototype, "getSessionStats", async () => { throw new Error("offline stats"); });
  t.mock.method(RpcClient.prototype, "onEvent", function(this: RpcClient, listener: (event: unknown) => void) {
    const current = listeners.get(this) ?? new Set();
    current.add(listener); listeners.set(this, current);
    return () => { current.delete(listener); };
  });
  t.after(async () => { await r.handle.stopAll(); closeFakeChildren(); t.mock.restoreAll(); syncBuiltinESMExports(); });
  const promptPath = join(root, "execute-guide.md");
  writeFileSync(promptPath, "Offline execute-worker guide");
  const relay = createApprovalRelay(r.pi, { cwd: root }, { current: r.handle.rpcRegistry });
  const calls: string[] = [];
  const onApprovalPending = (record: RpcAgentRecord) => { calls.push(record.pendingApproval!.cmdId); relay(record); };
  registerExecuteGateway(r.pi, r.bridge, r.handle.rpcRegistry, { cwd: root, extensionPath, executeWorkerPromptPath: promptPath, onApprovalPending });
  const result = await r.tools.get("ws-execute").execute("dispatch", { prompt: "execution task", complex: true }, undefined, undefined,
    { sessionManager: { getSessionId: () => "owner" }, agentStorageRoot: root, model: { provider: "offline", id: "test" }, modelRegistry: { getAll: () => [], hasConfiguredAuth: () => true } });
  const record = r.handle.rpcRegistry.get(JSON.parse(result.content[0].text).agent_id)!;
  assert.equal(record.onApprovalPending, onApprovalPending, "the fresh record, not just the initial listener, owns its relay");
  const oldClient = record.client!;
  assert.deepEqual(calls, []);
  markAgentExited(undefined, r.handle.rpcRegistry, record, { suppressTerminal: true });
  assert.equal(record.client, undefined);
  assert.equal(listeners.get(oldClient)?.size, 0, "process loss detached the initial listener");
  await sendToAgent(r.handle.rpcRegistry, { ...auditResumeCtx({ pi: r.pi, cwd: root, extensionPath, sessionKeyRef: { current: "lead-current" } }), writer: "owner" }, record.agentId, "resume execution");
  assert.notEqual(record.client, oldClient);
  for (const listener of [...listeners.get(record.client!)!]) listener({ type: "tool_execution_start", toolName: GATED_EXEC_TOOL_NAME, toolCallId: "after-loss-command", args: { command: "not executed", rationale: "correlation regression" } });
  assert.deepEqual(calls, ["after-loss-command"], "the replacement listener invokes the pinned relay exactly once");
  assert.equal(record.pendingApproval?.cmdId, "after-loss-command");
  const approvals = r.messages.filter(message => message.customType === "ws-agent-approval");
  assert.equal(approvals.length, 1);
  assert.equal(approvals[0].details.cmd_id, "after-loss-command");
  assert.equal(approvals[0].details.agent_id, record.agentId);
  await assert.rejects(r.tools.get("ws-approve").execute("stale", { agent_id: record.agentId, cmd_id: "other-command", decision: "approve" }), /rejected/);
  await r.tools.get("ws-approve").execute("deny", { agent_id: record.agentId, cmd_id: "after-loss-command", decision: "deny", reason: "test only" });
  assert.equal(record.pendingApproval?.decision, "sent");
  assert.deepEqual(calls, ["after-loss-command"]);
});

test("I2 real expired owned transcript, prompt and raw sidecar survive quarantine and subsequent normal global TTL scans", t => {
  const root = temporary(t);
  let now = Date.parse("2026-11-16T00:00:00.000Z");
  t.mock.method(Date, "now", () => now);
  const legacy = expiredHistory(root, "retired-history", now);
  const control = expiredHistory(root, "ordinary-expired-execution", now, "execute-worker");
  const sidecar = join(root, "registry.json");
  const raw = serializeOrphans([legacy.orphan]);
  writeFileSync(sidecar, raw);
  const notices: string[] = [];
  const recovery = readExecutionSidecarAt(sidecar, text => notices.push(text));
  assert.equal(recovery.writable, true);
  assert.equal(recovery.retentionSafe, true);
  assert.deepEqual(recovery.orphans, []);
  assert.equal(readOwnership(legacy.ownership.home)?.liveness.ownerHeld, true);
  assert.match(notices.join("\n"), /Permanent owner-held history protection/);
  assert.match(notices.join("\n"), /no live-process claim/);
  assert.match(notices.join("\n"), new RegExp(legacy.ownership.agentId));
  applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, recovery.orphans, undefined, recovery);
  assert.equal(existsSync(control.ownership.home), false, "the real global pruner ran and removed an eligible control");
  const archive = join(root, readdirSync(root).find(name => name.startsWith("registry.json.retired-"))!);
  assert.equal(readFileSync(archive, "utf8"), raw);
  assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
  assert.deepEqual(readFileSync(legacy.promptPath), legacy.prompt);
  now += 100 * 86_400_000;
  const nextStartup = readExecutionSidecarAt(sidecar, () => assert.fail("no sidecar remains to announce"));
  applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, nextStartup.orphans, undefined, nextStartup);
  assert.equal(readOwnership(legacy.ownership.home)?.liveness.ownerHeld, true, "later startup does not clear the durable old-reader-aware hold");
  assert.equal(inspectOwnedHomeRemoval(legacy.ownership).status, "retained");
  assert.equal(readFileSync(archive, "utf8"), raw);
  assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
  assert.deepEqual(readFileSync(legacy.promptPath), legacy.prompt);
});

for (const fault of ["rename", "read"] as const) {
  test(`I3 ${fault} failure blocks all later writes and nonempty shutdown snapshots, new execution and retention`, async t => {
    const root = temporary(t);
    const legacy = expiredHistory(root, "retired-history", Date.now());
    const sidecar = join(root, "registry.json");
    const raw = serializeOrphans([legacy.orphan]);
    writeFileSync(sidecar, raw);
    const r = runtime(root);
    t.after(() => r.handle.stopAll());
    const notices: string[] = [];
    const original = fault === "read" ? fs.readFileSync : fs.renameSync;
    const mocked = t.mock.method(fs, fault === "read" ? "readFileSync" : "renameSync", ((...args: any[]) => {
      if (String(args[0]) === sidecar) throw Object.assign(new Error(`injected ${fault} fault`), { code: "EACCES" });
      return (original as any)(...args);
    }) as never);
    syncBuiltinESMExports();
    const recovery = prepareExecutionSidecar(r.handle.rpcRegistry, sidecar, text => notices.push(text));
    mocked.mock.restore(); syncBuiltinESMExports();
    assert.deepEqual(recovery, { orphans: [], writable: false, retentionSafe: false });
    assert.match(notices.join("\n"), fault === "read" ? /Could not read/ : /Could not quarantine/);
    assert.match(notices.join("\n"), /Sidecar writes, new persistent execution and automatic retention are blocked/);
    applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, [], undefined, recovery);
    assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
    registerExecuteGateway(r.pi, r.bridge, r.handle.rpcRegistry, { cwd: root, extensionPath, executeWorkerPromptPath: "/unused/prompt.md" });
    await assert.rejects(r.tools.get("ws-execute").execute("new", { command: "MUST NOT RUN", prompt: "must not spawn" }), /persistent execution is blocked/);
    r.handle.rpcRegistry.set("new-execution", { agentId: "new-execution", toolGroup: "execute-worker", spawnRole: "execute-worker", sessionPath: join(root, "new-execution.jsonl"), systemPromptPath: join(root, "new-prompt.md"), wsToolNames: [], running: false, streaming: false, reportLog: [] });
    assert.equal(captureOrphans(r.handle.rpcRegistry).length, 1, "shutdown has a nonempty new execution snapshot");
    assert.equal(await persistShutdownAgentSnapshots(r.handle, sidecar, createThreadRegistryHandle()), false);
    assert.equal(writeSidecarAt(sidecar, []), false, "even empty writes report the blocked path");
    assert.equal(writeSidecarAt(sidecar, [legacy.orphan]), false);
    assert.equal(readFileSync(sidecar, "utf8"), raw, "a writable source still keeps its exact raw bytes after the fault has gone away");
    // A later readable startup can establish the durable hold and quarantine,
    // rather than relying on a process-local exclusion to protect old history.
    const nextStartup = readExecutionSidecarAt(sidecar, () => {});
    assert.equal(nextStartup.retentionSafe, true);
    applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, nextStartup.orphans, undefined, nextStartup);
    assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
    assert.deepEqual(readFileSync(legacy.promptPath), legacy.prompt);
  });
}

test("I2/I3 unrecognized metadata keeps the source and fails retention closed on subsequent startups", t => {
  const root = temporary(t);
  const legacy = expiredHistory(root, "unknown-history", Date.now());
  const sidecar = join(root, "registry.json");
  const raw = '{"version":99,"orphans":[]}';
  writeFileSync(sidecar, raw);
  for (let startup = 0; startup < 2; startup++) {
    const recovery = readExecutionSidecarAt(sidecar, () => {});
    assert.equal(recovery.retentionSafe, false);
    applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, [], undefined, recovery);
    assert.equal(readFileSync(sidecar, "utf8"), raw);
    assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
    assert.deepEqual(readFileSync(legacy.promptPath), legacy.prompt);
  }
  const otherNamespace = expiredHistory(root, "unreadable-other-lead-history", Date.now(), "worker", "other-lead");
  // Another lead's normal startup scans all namespaces, even though it never
  // reads this lead's malformed sidecar. The production retention option must
  // durably hold every retired role it encounters, not just returned orphans.
  const unrelatedStartup = readExecutionSidecarAt(join(root, "another-lead.json"), () => {});
  const globalNotices: string[] = [];
  applySessionStartAgentRetention(undefined, root, { child_retention_ttl_days: 1 }, [], undefined, unrelatedStartup, message => globalNotices.push(message));
  assert.equal(globalNotices.length, 2);
  assert.ok(globalNotices.every(message => message.includes("ownerHeld:true") && message.includes("no live-process claim")));
  assert.equal(readOwnership(legacy.ownership.home)?.liveness.ownerHeld, true);
  assert.equal(readOwnership(otherNamespace.ownership.home)?.liveness.ownerHeld, true);
  assert.equal(readFileSync(sidecar, "utf8"), raw);
  assert.deepEqual(readFileSync(legacy.ownership.sessionPath!), legacy.transcript);
  assert.deepEqual(readFileSync(otherNamespace.ownership.sessionPath!), otherNamespace.transcript);
});
