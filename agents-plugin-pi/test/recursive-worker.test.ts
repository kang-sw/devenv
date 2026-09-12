import { test, afterEach } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import {
  CHILD_MANAGEMENT_TOOLS, DELEGATION_ENV, SUBTREE_ENV, RenderRegistry, assertPolicyTool, assertSessionAuthority,
  childPolicy, parseDelegationPolicy, playbookProfile, readDelegationPolicy, readOnlyWsTools, type DelegationPolicy,
} from "../src/delegation-policy.ts";
import { assertSubtreeFinal, beginSubtreeDispatch, installSubtreePublisher, readSubtreeSnapshot, subtreeWaiting } from "../src/subtree-lifecycle.ts";
import { applyRpcEvent, attachEventListener, evictForCapacity, flushHeldPushes, flushPendingFinal, hasRunningAgents, heldPushQueue, leadIdleRef, leadWakeStartPendingRef, listAgents, markAgentExited, promptAgent, registerPushFlush, resolveTools, sendToAgent, spawnAdmission, stopAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";
import { captureForkResume, rehydrateForkRecord } from "../src/ask.ts";

const dirs: string[] = [];
function home() { const dir = mkdtempSync(join(tmpdir(), "ws-subtree-test-")); dirs.push(dir); return dir; }
afterEach(() => { for (const dir of dirs.splice(0)) rmSync(dir, { recursive: true, force: true }); heldPushQueue.length = 0; leadIdleRef.current = undefined; leadWakeStartPendingRef.current = false; });
const full = resolveTools("full-worker", ["ws__playbook_render", "ws__git_diff", "ws__git_commit", "ws__ferrule"]).split(",");
const root: DelegationPolicy = { version: 1, depth: 0, maxDepth: 2, authority: "lead", tools: ["ws-agent-spawn"] };
const worker = () => childPolicy(root, full, "lead");
function record(id: string, overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return { agentId: id, sessionPath: join(home(), "session.jsonl"), systemPromptPath: "prompt.md", wsToolNames: [], toolGroup: "full-worker", streaming: false, running: false, reportLog: [], ...overrides };
}

test("only root expands authority; role-independent terminal full workers retain execution tools", () => {
  const parent = worker();
  const leaf = childPolicy(parent, full, "lead");
  assert.equal(leaf.depth, 2);
  assert.ok(leaf.tools.includes("bash") && leaf.tools.includes("write"));
  assert.ok(CHILD_MANAGEMENT_TOOLS.every(t => !leaf.tools.includes(t)));
  assert.throws(() => childPolicy(leaf, ["read"], "leaf"), /maximum delegation depth/);
  const deeper = childPolicy({ ...parent, maxDepth: 3 }, full, "lead", true);
  assert.ok(deeper.tools.includes("ws-agent-spawn"));
  assert.throws(() => childPolicy(parent, full, "lead", true), /requires children/);
});

test("native, bridged, lazy and session authority never exceed a nonroot ceiling", () => {
  const parent: DelegationPolicy = { ...worker(), authority: "leaf", tools: ["read", "grep", "find", "ls", "ws-report-to-lead", ...CHILD_MANAGEMENT_TOOLS] };
  for (const tool of ["edit", "write", "bash", "ws__git_commit", "ws__tickets_close"]) {
    assert.throws(() => childPolicy(parent, [...parent.tools, tool], "leaf"), /exceeds parent ceiling/);
    assert.throws(() => assertPolicyTool(parent, tool), /capability ceiling/);
  }
  assert.throws(() => childPolicy(parent, ["read"], "lead"), /leaf -> lead/);
  assert.throws(() => assertSessionAuthority(parent, { capability: "lead" }, new Set()), /session capability/);
  assert.throws(() => assertSessionAuthority(parent, { session_key: "foreign-lead" }, new Set(["own-leaf"])), /outside/);
  assert.throws(() => assertSessionAuthority({ ...parent, authority: "lead" }, { session_key: "parent-lead" }, new Set(["own-worker"])), /outside/, "lead-capability workers are still nonroot policy holders");
  assertSessionAuthority(parent, { session_key: "own-leaf", capability: "leaf" }, new Set(["own-leaf"]));
  assert.deepEqual(readOnlyWsTools(["ws__git_diff", "ws__git_commit", "ws__tickets_close", "ws__unknown_read"]), ["ws__git_diff"]);
  assert.equal(childPolicy(parent, ["read"], "leaf").authority, "leaf");
  assert.equal(childPolicy({ ...parent, sessionKey: "parent-lead" }, ["read"], "leaf", false, "own-leaf").parentSessionKey, undefined, "a rendered child must not receive its parent's key in the environment");
});

test("malformed persisted policy never falls back to root", () => {
  assert.throws(() => readDelegationPolicy({ [DELEGATION_ENV]: "{}" }), /malformed/);
  for (const value of [{ ...worker(), depth: -1 }, { ...worker(), maxDepth: 0 }, { ...worker(), tools: [42] }, { ...worker(), authority: "root" }, { ...worker(), authority: "toString" }]) assert.throws(() => parseDelegationPolicy(value));
});

test("playbook authority comes from shipped manifest and bridge render, not a filename", () => {
  const plugin = resolve(".");
  assert.equal(playbookProfile(plugin, "code-review-correctness").readOnly, true);
  assert.equal(playbookProfile(plugin, "ticket-worker").requiresChildren, true);
  assert.equal(playbookProfile(plugin, "implementer").requiresChildren, false);
  assert.throws(() => playbookProfile(plugin, "lead-run"), /lead-control/);
  assert.throws(() => playbookProfile(plugin, "../reviewer"), /unrecognized/);
  const path = join(home(), "lead-run.md");
  writeFileSync(path, "**Your ws session_key: `review-own`**\nreview body");
  const registry = new RenderRegistry();
  assert.equal(registry.get(path), undefined);
  registry.record(path, playbookProfile(plugin, "code-review-correctness"));
  assert.equal(registry.get(path)?.class, "reviewer");
  assert.equal(registry.get(path)?.sessionKey, "review-own");
  writeFileSync(path, "replace with privileged instructions");
  assert.throws(() => registry.get(path), /changed since authorization/);
});

test("spawn admission rejects unproven nested prompts and lead forks before allocation", () => {
  const ctx: any = { parentPolicy: worker(), wsToolNames: [], toolGroup: "full-worker" };
  assert.throws(() => spawnAdmission(ctx), /trusted render provenance/);
  assert.throws(() => spawnAdmission({ ...ctx, spawnRole: "fork" }), /only the root/);
  const terminal = spawnAdmission({ ...ctx, profile: { class: "delegate", readOnly: false, authority: "delegate", requiresChildren: false } });
  assert.ok(terminal.tools.includes("edit"));
  assert.ok(!terminal.tools.includes("ws-agent-spawn"));
});

test("one-edge snapshot distinguishes idle waiting from subtree quiescence and fails closed", () => {
  const channel = { path: join(home(), "subtree.json"), nonce: "launch-a" };
  assert.equal(subtreeWaiting(readSubtreeSnapshot(channel)), true);
  const child = record("leaf", { expectedReport: true });
  const registry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, channel, () => 0);
  assert.equal(readSubtreeSnapshot(channel)?.outstanding, 1);
  assert.throws(() => assertSubtreeFinal(registry), /final rejected/);
  child.expectedReport = false;
  assert.equal(assertSubtreeFinal(registry), 0);
  assert.equal(subtreeWaiting(readSubtreeSnapshot(channel)), false);
  assert.equal(readSubtreeSnapshot({ ...channel, nonce: "stale-launch" }), undefined);
  child.threadBound = true;
  assert.throws(() => assertSubtreeFinal(registry), /final rejected/);
  child.threadBound = false; child.ownerHeld = true;
  assert.throws(() => assertSubtreeFinal(registry), /final rejected/);
});

test("in-flight dispatch blocks synthesis without inventing an accepted-report obligation", () => {
  const registry = new Map<string, RpcAgentRecord>();
  installSubtreePublisher(registry, undefined, () => 0);
  const finish = beginSubtreeDispatch(registry);
  assert.equal(registry.size, 0);
  assert.throws(() => assertSubtreeFinal(registry), /final rejected/);
  finish();
  assert.equal(assertSubtreeFinal(registry), 0);
});

test("a stale accepted-result revision cannot reach an owner final hook", () => {
  let hooked = false;
  const r = record("parent", { delegation: worker(), expectedReport: true, requiresFreshFinal: true, subtreeRevision: 2, onFinalReport: () => { hooked = true; return true; } });
  applyRpcEvent(r, { type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "old", args: { kind: "final", message: "stale" } });
  applyRpcEvent(r, { type: "tool_execution_end", toolCallId: "old", result: { details: { subtreeRevision: 1 } } });
  assert.equal(hooked, false);
  assert.equal(r.pendingFinal, undefined);
  assert.equal(r.expectedReport, true);
});

test("a late prompt acknowledgement cannot reopen an already completed own turn", async () => {
  let accept!: () => void;
  const r = record("child", { delegation: worker() });
  const pending = promptAgent(r, { prompt: () => new Promise<void>(resolve => { accept = resolve; }) } as any, "work");
  applyRpcEvent(r, { type: "agent_start" });
  applyRpcEvent(r, { type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "final", args: { kind: "final", message: "done" } });
  applyRpcEvent(r, { type: "tool_execution_end", toolCallId: "final", isError: false });
  applyRpcEvent(r, { type: "agent_settled" });
  const sent: unknown[] = [];
  const pi = { sendMessage: (message: unknown) => sent.push(message) } as any;
  leadIdleRef.current = () => false;
  flushPendingFinal(pi, new Map([[r.agentId, r]]), r, "idle");
  assert.equal(r.expectedReport, true, "an accepted report stays outstanding until its direct-parent delivery is accepted");
  flushHeldPushes(pi, true);
  assert.equal(sent.length, 1);
  accept(); await pending;
  assert.equal(r.expectedReport, false);
});

test("expected obligation is created after acceptance, not at prompt issuance", async () => {
  let accept!: () => void;
  const r = record("child", { delegation: worker() });
  const p = promptAgent(r, { prompt: () => new Promise<void>(resolve => { accept = resolve; }) } as any, "work");
  assert.equal(r.expectedReport, undefined);
  accept(); await p;
  assert.equal(r.expectedReport, true);
  applyRpcEvent(r, { type: "agent_settled" });
  assert.equal(r.expectedReport, true);
  const registry = new Map([[r.agentId, r]]);
  assert.equal(hasRunningAgents(registry), true);
  assert.equal(evictForCapacity(registry, 1).ok, false);
  await stopAgent(registry, r.agentId);
  assert.equal(r.expectedReport, false, "explicit disposition releases even a dormant child");
});

test("held and failed terminal delivery preserve the obligation until direct-parent enqueue", () => {
  const r = record("child", { delegation: worker(), expectedReport: true, pendingFinal: "done", pendingFinalAccepted: true });
  const registry = new Map([[r.agentId, r]]);
  const sent: any[] = [];
  const pi = { sendMessage: (message: unknown) => sent.push(message) } as any;
  leadIdleRef.current = () => false;
  assert.equal(flushPendingFinal(pi, registry, r, "idle"), true);
  assert.equal(r.expectedReport, true);
  assert.equal(parseOrphans(serializeOrphans(captureOrphans(registry)))[0]?.state, "running");
  flushHeldPushes(pi, true);
  assert.equal(r.expectedReport, false);
  assert.equal(sent.length, 1);
  assert.match(sent[0].content, /0 delegated agents still running/);

  const failed = record("failed", { delegation: worker(), expectedReport: true, pendingFinal: "retry", pendingFinalAccepted: true });
  const failedRegistry = new Map([[failed.agentId, failed]]);
  assert.equal(flushPendingFinal(pi, failedRegistry, failed, "idle"), true);
  flushHeldPushes({ sendMessage: () => { throw new Error("session torn down"); } } as any, true);
  assert.equal(failed.expectedReport, true, "a rejected enqueue stays recoverably outstanding");
});

test("exited delegated child releases its obligation only with the terminal failure delivery", () => {
  const r = record("child", { client: {} as RpcClient, delegation: worker(), expectedReport: true, running: true });
  const registry = new Map([[r.agentId, r]]);
  const sent: unknown[] = [];
  const pi = { sendMessage: (message: unknown) => sent.push(message) } as any;
  leadIdleRef.current = () => false;
  markAgentExited(pi, registry, r);
  assert.equal(r.expectedReport, true);
  flushHeldPushes(pi, true);
  assert.equal(r.expectedReport, false);
  assert.equal(sent.length, 1);
});

test("sidecar round-trips depth, ceiling and outstanding subtree protection", () => {
  const r = record("parent", { delegation: worker(), expectedReport: true, waitingOnChildren: true, requiresFreshFinal: true, subtreeChannel: { path: join(home(), "state"), nonce: "n" } });
  const [saved] = parseOrphans(serializeOrphans(captureOrphans(new Map([[r.agentId, r]]))));
  assert.equal(saved.state, "running");
  const restored = rehydrateOrphanRecord(saved);
  assert.equal(restored.client, undefined);
  assert.deepEqual(restored.delegation, r.delegation);
  assert.equal(restored.expectedReport, true);
  assert.equal(restored.waitingOnChildren, true);
  assert.equal(evictForCapacity(new Map([[restored.agentId, restored]]), 1).ok, false);
  const thread = rehydrateForkRecord(r.agentId, captureForkResume(r));
  assert.deepEqual(thread.delegation, r.delegation);
  assert.equal(thread.waitingOnChildren, true);
  assert.equal(thread.expectedReport, true);
});

function edge(r: RpcAgentRecord, registry: Map<string, RpcAgentRecord>, sent: any[]) {
  let listener: (e: any) => void = () => {};
  let stopped = 0;
  const hooks = new Map<string, Function>();
  const pi: any = { on: (name: string, fn: Function) => hooks.set(name, fn), sendMessage: (message: any) => sent.push(message), sendUserMessage: () => hooks.get("agent_start")?.() };
  registerPushFlush(pi, { delayMs: () => 0 });
  leadIdleRef.current = () => true;
  const client: any = { onEvent: (fn: any) => { listener = fn; return () => {}; }, getState: async () => ({}), getLastAssistantText: async () => "leaf answer", stop: async () => { stopped++; }, abort: async () => {}, prompt: async () => {} };
  r.client = client; r.running = true;
  attachEventListener(pi, registry, r, client);
  return { emit: (e: any) => listener(e), pi, client, stops: () => stopped };
}
const drain = () => new Promise(resolve => setImmediate(resolve));

test("lead -> worker -> leaf: local settle cannot report or park parent; fresh accepted final propagates once", async () => {
  const channel = { path: join(home(), "subtree.json"), nonce: "n" };
  const parent = record("worker", { delegation: worker(), expectedReport: true, subtreeChannel: channel });
  const grandchild = record("reviewer", { delegation: childPolicy(worker(), ["read", "ws-report-to-lead"], "delegate"), expectedReport: true, running: true });
  const inner = new Map([[grandchild.agentId, grandchild]]);
  installSubtreePublisher(inner, channel, () => 0);
  const sent: any[] = [];
  const h = edge(parent, new Map([[parent.agentId, parent]]), sent);
  h.emit({ type: "agent_settled" }); await drain();
  assert.equal(sent.length, 0); assert.equal(h.stops(), 0);
  assert.equal(listAgents(new Map([[parent.agentId, parent]]))[0].status, "waiting-on-children");
  h.emit({ type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "old", args: { kind: "final", message: "premature" } });
  h.emit({ type: "tool_execution_end", toolName: "ws-report-to-lead", toolCallId: "old", isError: true });
  h.emit({ type: "agent_settled" }); await drain();
  assert.equal(sent.length, 0);
  const localReports: any[] = [];
  const innerEdge = edge(grandchild, inner, localReports);
  innerEdge.emit({ type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "review", args: { kind: "final", message: "review findings" } });
  innerEdge.emit({ type: "tool_execution_end", toolName: "ws-report-to-lead", toolCallId: "review", isError: false });
  innerEdge.emit({ type: "agent_settled" }); await drain();
  assert.equal(localReports.length, 1, "reviewer report wakes only its direct worker owner");
  assert.equal(sent.length, 0, "no grandchild result leaks upward before synthesis");
  h.emit({ type: "agent_start" });
  h.emit({ type: "tool_execution_start", toolName: "ws-report-to-lead", toolCallId: "new", args: { kind: "final", message: "synthesized reviewer findings" } });
  h.emit({ type: "tool_execution_end", toolName: "ws-report-to-lead", toolCallId: "new", result: { details: { subtreeRevision: assertSubtreeFinal(inner) } }, isError: false });
  h.emit({ type: "agent_settled" }); await drain();
  assert.equal(sent.length, 1);
  assert.equal(sent[0].details.report, "synthesized reviewer findings");
  assert.equal(h.stops(), 1);
  assert.equal(parent.expectedReport, false);
  h.emit({ type: "agent_settled" }); await drain();
  assert.equal(sent.length, 1);
});

test("plain managed leaf settle is delivered but keeps the obligation and process until disposition", async () => {
  const sent: any[] = [];
  const r = record("leaf", { delegation: worker(), expectedReport: true });
  const registry = new Map([[r.agentId, r]]);
  const h = edge(r, registry, sent);
  h.emit({ type: "agent_settled" }); await drain();
  assert.equal(sent[0].customType, "ws-agent-settled");
  assert.equal(h.stops(), 0);
  assert.equal(r.expectedReport, true);
  await stopAgent(registry, r.agentId, h.pi);
  assert.equal(h.stops(), 1);
  assert.equal(r.expectedReport, false);
});

test("worker stop disposition is quiescent for a fresh parent final", async () => {
  const parentPolicy = worker();
  const previous = process.env[DELEGATION_ENV];
  process.env[DELEGATION_ENV] = JSON.stringify(parentPolicy);
  const child = record("reviewer", {
    client: { abort: async () => {}, stop: async () => {} } as RpcClient,
    delegation: childPolicy(parentPolicy, ["read", "ws-report-to-lead"], "delegate"),
    expectedReport: false,
    running: true,
  });
  const registry = new Map([[child.agentId, child]]);
  installSubtreePublisher(registry, undefined, () => heldPushQueue.length);
  leadIdleRef.current = () => false;
  try {
    await stopAgent(registry, child.agentId, { sendMessage: () => { throw new Error("stop disposition must not push"); } } as any);
    assert.equal(heldPushQueue.length, 0);
    assert.equal(assertSubtreeFinal(registry), 0);
    await stopAgent(registry, child.agentId);
    assert.equal(assertSubtreeFinal(registry), 0, "repeated dormant stop stays quiescent");
    const outstanding = record("other", { delegation: child.delegation, expectedReport: true });
    registry.set(outstanding.agentId, outstanding);
    assert.throws(() => assertSubtreeFinal(registry), /final rejected/);
  } finally {
    if (previous === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previous;
  }
});

test("dormant continuation keeps the same session and capability envelope", async () => {
  const r = record("child", { delegation: worker(), expectedReport: true });
  const path = r.sessionPath;
  const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
  let requests = 0;
  Object.assign(RpcClient.prototype, { start: async () => {}, stop: async () => {}, abort: async () => {}, onEvent: () => () => {}, prompt: async () => { requests++; }, getState: async () => ({}) });
  const registry = new Map([[r.agentId, r]]);
  try {
    await sendToAgent(registry, { cwd: home(), extensionPath: "index.ts" }, r.agentId, "continue");
    assert.equal(requests, 1);
    assert.equal(r.sessionPath, path);
    assert.equal(registry.size, 1);
    assert.deepEqual(r.delegation, worker());
    assert.equal(r.expectedReport, true);
    await stopAgent(registry, r.agentId);
    await sendToAgent(registry, { cwd: home(), extensionPath: "index.ts" }, r.agentId, "another guarded turn");
    assert.equal(requests, 2);
    assert.equal(r.sessionPath, path);
    await stopAgent(registry, r.agentId);
  } finally { Object.assign(RpcClient.prototype, original); }
});

test("late async harvest cannot settle or park replacement work", async () => {
  const sent: any[] = [];
  const r = record("leaf", { delegation: worker(), expectedReport: true });
  const registry = new Map([[r.agentId, r]]);
  const h = edge(r, registry, sent);
  let release!: (text: string) => void;
  h.client.getLastAssistantText = () => new Promise(resolve => { release = resolve; });
  h.emit({ type: "agent_settled" });
  await sendToAgent(registry, { pi: h.pi, cwd: ".", extensionPath: "index.ts" }, r.agentId, "replacement");
  release("old answer"); await drain();
  assert.equal(sent.length, 0); assert.equal(h.stops(), 0); assert.equal(r.expectedReport, true);
});
