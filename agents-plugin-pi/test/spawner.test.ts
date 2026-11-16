/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * resolveModelForAliasViaWsMcp (Phase 4:
 * async, ws-mcp-`config.resolve_agent`-backed tier resolution against a stub
 * `client.callTool`, replacing the old file-catalog-backed
 * `resolveModelForAlias`), applyRpcEvent's streaming/report bookkeeping and its
 * push OUTCOMES, listAgents's status mapping, and sendToAgent's internal LIVE
 * branches (streaming+interrupt->steer, streaming+no-interrupt->followUp,
 * idle->prompt) plus the lead tool's always-steer streaming path, via a
 * duck-typed `steer`/`followUp`/`prompt` stub cast as
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
 * `shouldPushToLead` (the role gate), `computeRunningStatusLine` (the fan-in
 * running count), `buildPushContent`/`pushToLead` (message shape and
 * best-effort delivery), `promptAgent` (the single running/generation
 * funnel), `recordReport` (the bounded intermediate report log), and
 * `probeAgentLiveness`/`markAgentExited`/
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
 * constructs a real `RpcClient` and calls `.start()`: `spawnAgent` and
 * `sendToAgent`'s dormant-auto-resume branch. Exercised only by a lead-scoped Pi session spawning a real
 * `pi` child process, per the plan's Verification Plan split between unit
 * and live coverage.
 *
 * Review fix (cycle 1, 260903 Phase 1 goal-loop): also covers
 * `buildRpcClientOptions`'s process-role env marker placement — previously left to a manual
 * spot-check with zero automated coverage. 260904 Phase 1 renamed the
 * marker from the boolean `WS_PI_AGENT_CHILD_ENV` to the role-valued
 * `WS_PI_SPAWN_ROLE_ENV` (`process-role.ts`) — these tests now assert the
 * role values (`"worker"`/`"explore"`) instead of `"1"`.
 *
 * 260904 Phase 1 (execute-approve gateway) additionally covers: the new
 * `"execute-worker"` `TOOL_GROUPS` entry and its `resolveTools` threading
 * through a fake record's `toolGroup`; `applyRpcEvent`'s new
 * `pendingApproval`-capturing branch for `GATED_EXEC_TOOL_NAME`;
 * `buildRpcClientOptions`'s role-marker placement; and
 * `inheritModelFromToolCtx` (exported out of `registerAgentTools`'s former
 * private closure for reuse by `execute-gateway.ts`). The gated-exec tool's
 * own `execute()` body, `ws-execute`/`ws-approve`'s tool registrations, and
 * the approval-request/decision relay end-to-end are NOT covered here
 * — see test/execute-gateway.test.ts's header comment for that split.
 *
 * 260924 (channel approval decisions): `attachApprovalChannel`, the parent
 * side of the decision hand-off, is covered over a fake channel host below
 * (consumption acknowledgment, discard on disconnect, reconnect-hello
 * reconciliation, ownership protection until the acknowledgment).
 *
 * 260904 Phase 1 (side-thread fork) additionally covers: `buildRpcClientOptions`'s
 * new `forkFrom`/`parentSessionKey` params (the `--fork` vs `--session` arg
 * branch and the `"fork"` vs `"worker"` role marker). `fork.ts`'s own pure
 * predicates/IO glue are covered by test/fork.test.ts instead — see that
 * file's header comment for its own pure/live-gate split.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { test, describe, afterEach } from "node:test";
import assert from "node:assert/strict";
import {
  resolveTools,
  heldPushQueue,
  ownerNotifyRef,
  stopAgent,
  sendToAgent,
  inheritModelFromToolCtx,
  WS_PI_AGENT_REGISTRY_CAP_ENV,
  spawnAgent,
  leadParentPolicy,
  type RpcAgentRegistry,
} from "../src/spawner.ts";
// 260906 (registerAgentTools's role-keyed explore registration): a VALUE
// import, not `import type` — the new describe block below monkey-patches
// RpcClient.prototype.{start,onEvent,prompt} for the lead/fork execute() path
// so the real spawnAgent code runs to completion with no real subprocess
// (see that describe block's own doc comment for why this is safe and the
// worker/execute-worker leaf path deliberately never invokes execute() at all).
import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { McpStdioClient, McpToolCallResult } from "../src/mcp-stdio-client.ts";
import fs, { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";
import { allocateAgentHome, createAgentStorageContext, readOwnership, updateOwnership, type OwnershipMetadata } from "../src/agent-storage.ts";
import { ParentChannel } from "../src/agent-channel.ts";
import { installSubtreePublisher, SubtreeUpstream } from "../src/subtree-lifecycle.ts";
import { fakeUplink, idleOwnTurn, until } from "./fixtures/subtree-channels.ts";
import {
  modelCatalogFromToolCtx,
  tierWarningNotifierFromToolCtx,
} from "../src/model-catalog.ts";
import { freshRpcRecord, fakeRpcClient } from "./fixtures/spawner-records.ts";

const REAL_EXTENSION_ENTRY = fileURLToPath(new URL("../src/index.ts", import.meta.url));

// Stands in for the child half of the control channel: the hello plus the
// role's stage-2 readiness (web for Explore, fork for a fork launch).
async function startRpcWithFakeChild(this: { options?: { env?: Record<string, string>; args?: string[] } }) {
  await connectFakeChild(this.options?.env, this.options?.args);
}

/** Test-only adapters over retained low-level lifecycle functions, NOT production
 * registrations. Public tool absence is covered in native-agent-retirement.test.ts. */
function createEngineTestHarness(pi: any, bridge: any, sessionCtx: any) {
  const rpcRegistry: RpcAgentRegistry = new Map();
  const extensionPath = sessionCtx.extensionPath ?? REAL_EXTENSION_ENTRY;
  installSubtreePublisher(rpcRegistry, sessionCtx.subtreeUpstream, () => heldPushQueue.length, idleOwnTurn);
  pi.registerTool({ name: "engine-spawn", async execute(_id: string, p: any, _signal: unknown, update: any, ctx: any) {
    const path = join(storageRoot(), "prompt.md");
    writeFileSync(path, "Offline worker prompt");
    let resolved: unknown;
    const result = await spawnAgent(rpcRegistry, {
      pi, cwd: sessionCtx.cwd, extensionPath,
      storage: createAgentStorageContext(ctx.sessionManager.getSessionId(), ctx.agentStorageRoot),
      inheritModel: inheritModelFromToolCtx(ctx), inheritEffort: ctx.thinkingLevel,
      catalog: modelCatalogFromToolCtx(ctx), allowConcreteModel: true,
      notifyTierWarning: tierWarningNotifierFromToolCtx(ctx), wsToolNames: bridge.wsToolNames,
      client: bridge.client, parentPolicy: leadParentPolicy(bridge),
      provenance: bridge.renderRegistry?.get(path),
      onModelResolved: value => { resolved = value; update?.({ content: [], details: { resolved } }); },
    }, { systemPromptPath: path, prompt: p.prompt, modelName: p.model_name, modelEffort: p.model_effort, alias: p.alias, title: p.title, cwdOverride: p.cwd_override, writeScopes: p.write_scopes });
    return { content: [{ text: JSON.stringify(result) }], details: { resolved } };
  } });
  pi.registerTool({ name: "engine-send", async execute(_id: string, p: any) {
    const result = await sendToAgent(rpcRegistry, { pi, cwd: sessionCtx.cwd, extensionPath, leadSend: true, leadSessionKey: bridge.defaultSessionKeyRef.current }, p.agent_id, p.message, true);
    const record = rpcRegistry.get(result.agent_id)!;
    const inherited = record.modelSource === undefined || record.modelSource === "inherit";
    return { content: [{ text: JSON.stringify(result) }], details: { resolved: { tier: inherited ? "inherit" : record.modelSource === "concrete" ? "concrete" : record.modelTier ?? "?", model: record.modelBase, effort: record.modelEffort, inherited } } };
  } });
  return { rpcRegistry, async stopAll() { await Promise.all([...rpcRegistry.keys()].map(id => stopAgent(rpcRegistry, id, pi, { silent: true, skipCostCheckpoint: true }))); } };
}

const storageRoots = new Set<string>();

afterEach(() => {
  for (const root of storageRoots) rmSync(root, { recursive: true, force: true });
  storageRoots.clear();
});

function storageRoot(): string {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-storage-test-"));
  storageRoots.add(root);
  return root;
}

/** Low-level engine regression coverage, with test-only argument adapters and
 * an in-process RPC transport. These are not public native registrations. */
describe("retained low-level engine: model rejection and lifecycle guards", () => {
  interface CapturedTool {
    name: string;
    description: string;
    parameters: { properties: Record<string, { description?: string }> };
    execute: (id: string, params: unknown, signal?: AbortSignal, update?: unknown, ctx?: unknown) => Promise<{ content: Array<{ text: string }> }>;
  }

  function installRpcHarness(
    onThinking?: (level: string) => void,
    onStart?: (cwd: string | undefined) => void,
    onPrompt?: (message: string) => void,
  ) {
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      start: async function(this: { options?: { cwd?: string; env?: Record<string, string>; args?: string[] } }) { onStart?.(this.options?.cwd); await connectFakeChild(this.options?.env, this.options?.args); }, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async (message: string) => { onPrompt?.(message); },
      setThinkingLevel: async (level: string) => { onThinking?.(level); },
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => { closeFakeChildren(); Object.assign(RpcClient.prototype, original); } };
  }

  function jsonResult(payload: unknown): McpToolCallResult {
    return { content: [{ type: "text", text: JSON.stringify(payload) }] };
  }

  function harness(
    callTool: McpStdioClient["callTool"],
    catalog = [{ provider: "openai-codex", id: "gpt-5.6-high", hasAuth: true }],
    provenance?: { class: "reviewer"; authority: "delegate"; readOnly: true; requiresChildren: false },
    subtreeUpstream?: SubtreeUpstream,
  ) {
    const tools = new Map<string, CapturedTool>();
    const sent: Array<{ message: unknown; options: unknown }> = [];
    const pi = {
      registerTool: (tool: CapturedTool) => tools.set(tool.name, tool),
      sendMessage: (message: unknown, options: unknown) => sent.push({ message, options }),
      sendUserMessage() {},
    } as unknown as ExtensionAPI;
    const bridge = {
      client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" },
      ...(provenance ? { renderRegistry: { get: () => ({ ...provenance, promptBase64: Buffer.from("Test reviewer prompt").toString("base64") }) } } : {}),
    } as never;
    const handle = createEngineTestHarness(pi, bridge, { cwd: "/tmp", subtreeUpstream });
    const ctx = {
      sessionManager: { getSessionId: () => "test-lead" },
      agentStorageRoot: storageRoot(),
      model: { provider: "lead", id: "large" },
      thinkingLevel: "high",
      modelRegistry: {
        getAll: () => catalog.map(({ provider, id }) => ({ provider, id })),
        hasConfiguredAuth: (model: { provider: string; id: string }) => catalog.some(entry => entry.provider === model.provider && entry.id === model.id && entry.hasAuth),
      },
    };
    return { tool: tools.get("engine-spawn")!, sendTool: tools.get("engine-send")!, handle, ctx, sent };
  }

  test("a named tier resolved 'unset' (resolved_from !== pi) refuses BEFORE any side effect: throws, no registry record, no session directory, no alias hold", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async (name) => { assert.equal(name, "config.resolve_agent"); return jsonResult({ resolved_from: "default", model: "gpt-5.6-terra" }); });
      await assert.rejects(
        () => tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small", alias: "wanted-alias" }, undefined, undefined, ctx),
        /ws-pi-agent: ws-agent-spawn rejected:/,
      );
      assert.equal(handle.rpcRegistry.size, 0, "a rejected spawn creates no registry entry");
      assert.equal([...handle.rpcRegistry.values()].some(r => r.alias === "wanted-alias"), false, "no alias hold either");
    } finally { rpc.restore(); }
  });

  test("an unknown or no-auth named tier also refuses — not only the unset case", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async () => jsonResult({ resolved_from: "pi", model: "not-in-catalog" }));
      await assert.rejects(
        () => tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small" }, undefined, undefined, ctx),
        /ws-pi-agent: ws-agent-spawn rejected:/,
      );
      assert.equal(handle.rpcRegistry.size, 0);
    } finally { rpc.restore(); }
  });

  test("unknown and unauthenticated concrete IDs refuse before allocation and never consult tier config", async () => {
    const rpc = installRpcHarness();
    try {
      let calls = 0;
      const unknown = harness(async () => { calls++; return jsonResult({}); });
      const unknownHomes = join(realpathSync(unknown.ctx.agentStorageRoot), "ws-agents", "test-lead");
      assert.equal(existsSync(unknownHomes), false);
      await assert.rejects(
        () => unknown.tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "openrouter/missing" }, undefined, undefined, unknown.ctx),
        /concrete model.*not a provider\/id entry/,
      );
      assert.equal(unknown.handle.rpcRegistry.size, 0);
      assert.equal(existsSync(unknownHomes), false, "unknown concrete selection allocates no owned home");

      const locked = harness(async () => { calls++; return jsonResult({}); }, [{ provider: "openrouter", id: "locked", hasAuth: false }]);
      const lockedHomes = join(realpathSync(locked.ctx.agentStorageRoot), "ws-agents", "test-lead");
      assert.equal(existsSync(lockedHomes), false);
      await assert.rejects(
        () => locked.tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "openrouter/locked" }, undefined, undefined, locked.ctx),
        /provider openrouter has no configured auth/,
      );
      assert.equal(locked.handle.rpcRegistry.size, 0);
      assert.equal(existsSync(lockedHomes), false, "unauthenticated concrete selection allocates no owned home");
      assert.equal(calls, 0, "concrete selection is catalog-local and never calls config.resolve_agent");
    } finally { rpc.restore(); }
  });

  test("a dropped busy acknowledgment or a down parent channel refuses the dispatch before RpcClient launch", async () => {
    const starts: Array<string | undefined> = [];
    const rpc = installRpcHarness(undefined, cwd => starts.push(cwd));
    try {
      for (const [label, uplink, refusal] of [
        ["dropped", fakeUplink(), /nested dispatch refused: the parent did not acknowledge revision 2 within 30ms/],
        ["down", Object.assign(fakeUplink(), { connected: false }), /nested dispatch refused: the channel to the parent is down/],
      ] as const) {
        const registered = harness(async () => jsonResult({}), undefined, undefined, new SubtreeUpstream(uplink.channel, { ackTimeoutMs: 30 }));
        await assert.rejects(
          () => registered.tool.execute(label, { system_prompt_path: "/tmp/p.md", prompt: "must not launch" }, undefined, undefined, registered.ctx),
          refusal,
        );
        assert.deepEqual(starts, [], `${label}: RpcClient.start is unreachable without the acknowledgment`);
        assert.equal(registered.handle.rpcRegistry.size, 0, `${label}: a refused admission allocates no grandchild record`);
        if (label === "dropped") assert.deepEqual(uplink.snapshots().map(s => s.active), [0, 1, 0], "the busy edge was published, then withdrawn");
      }
    } finally { rpc.restore(); }
  });

  test("a delayed busy acknowledgment holds RpcClient launch until the parent acknowledges the busy revision", async () => {
    const starts: Array<string | undefined> = [];
    const rpc = installRpcHarness(undefined, cwd => starts.push(cwd));
    const uplink = fakeUplink();
    let registered: ReturnType<typeof harness> | undefined;
    try {
      registered = harness(async () => jsonResult({}), undefined, undefined, new SubtreeUpstream(uplink.channel));
      const spawned = registered.tool.execute("delayed", { system_prompt_path: "/tmp/p.md", prompt: "launch after ack" }, undefined, undefined, registered.ctx);
      await until(() => uplink.snapshots().some(s => s.active === 1), "the busy edge");
      const busy = uplink.snapshots().find(s => s.active === 1);
      assert.ok(busy, "the busy edge is published before the wait");
      await new Promise(resolve => setTimeout(resolve, 50));
      assert.deepEqual(starts, [], "no launch before the acknowledgment");
      assert.equal(registered.handle.rpcRegistry.size, 0, "no record before the acknowledgment");
      uplink.ack(busy.revision);
      const result = JSON.parse((await spawned).content[0]!.text);
      assert.equal(starts.length, 1, "the acknowledgment admits exactly one launch");
      assert.ok(registered.handle.rpcRegistry.has(result.agent_id));
    } finally {
      await registered?.handle.stopAll();
      rpc.restore();
    }
  });

  test("ws-agent-send holds a live prompt and a dormant relaunch behind the busy acknowledgment, refuses on a dropped one, and the relaunch observes its new channel", async () => {
    const starts: Array<string | undefined> = [];
    const prompts: string[] = [];
    const rpc = installRpcHarness(undefined, cwd => starts.push(cwd), message => prompts.push(message));
    const uplink = fakeUplink();
    const hold = () => new Promise(resolve => setTimeout(resolve, 50));
    /** Runs `op` up to its busy edge, checks nothing dispatched while unacknowledged, then acknowledges it. */
    const fenced = async <T>(op: () => Promise<T>, dispatched: () => number): Promise<T> => {
      const before = uplink.sent.length;
      const count = dispatched();
      const running = op();
      await until(() => uplink.sent.length > before, "the busy edge");
      await hold();
      assert.equal(dispatched(), count, "nothing is dispatched before the acknowledgment");
      uplink.ack(uplink.snapshots().at(-1)!.revision);
      return running;
    };
    let registered: ReturnType<typeof harness> | undefined;
    try {
      registered = harness(async () => jsonResult({}), undefined, undefined, new SubtreeUpstream(uplink.channel, { ackTimeoutMs: 500 }));
      const { tool, sendTool, handle, ctx } = registered;
      const spawned = JSON.parse((await fenced(() => tool.execute("spawn", { system_prompt_path: "/tmp/p.md", prompt: "first" }, undefined, undefined, ctx), () => starts.length)).content[0]!.text);
      const record = handle.rpcRegistry.get(spawned.agent_id)!;
      await until(() => record.subtreeRevision !== undefined, "the child's first snapshot");

      await fenced(() => sendTool.execute("live", { agent_id: spawned.agent_id, message: "live follow-up" }), () => prompts.length);
      assert.equal(prompts.at(-1), "live follow-up");

      const promptsBeforeDrop = prompts.length;
      await assert.rejects(
        () => sendTool.execute("dropped", { agent_id: spawned.agent_id, message: "never delivered" }),
        /nested dispatch refused: the parent did not acknowledge revision \d+ within 500ms/,
      );
      assert.equal(prompts.length, promptsBeforeDrop, "a refused send prompts nothing");

      await stopAgent(handle.rpcRegistry, spawned.agent_id, undefined, { silent: true });
      const firstChannel = record.channel;
      assert.equal(record.subtreeRevision, undefined);
      await fenced(() => sendTool.execute("resume", { agent_id: spawned.agent_id, message: "resume" }), () => starts.length);
      assert.equal(starts.length, 2);
      assert.equal(prompts.at(-1), "resume");
      assert.notEqual(record.channel, firstChannel);
      await until(() => record.subtreeRevision !== undefined, "the relaunched child's first snapshot");
      assert.equal(record.waitingOnChildren, false, "the relaunch observes its own channel");

      // A relaunched child that never reports reads as waiting, not as the cleared default.
      await stopAgent(handle.rpcRegistry, spawned.agent_id, undefined, { silent: true });
      RpcClient.prototype.start = async function(this: { options?: { env?: Record<string, string>; args?: string[] } }) { await connectFakeChild(this.options?.env, this.options?.args, { subtree: null }); };
      await fenced(() => sendTool.execute("silent", { agent_id: spawned.agent_id, message: "silent relaunch" }), () => prompts.length);
      await hold();
      assert.equal(record.subtreeRevision, undefined);
      assert.equal(record.waitingOnChildren, true);
    } finally {
      await registered?.handle.stopAll();
      rpc.restore();
    }
  });

  test("cwd_override survives stop/resume, dormant ws-agent-send uses prompt, and paths validate before allocation", async () => {
    const starts: Array<string | undefined> = [];
    const prompts: string[] = [];
    const rpc = installRpcHarness(undefined, cwd => starts.push(cwd), message => prompts.push(message));
    try {
      const { tool, sendTool, handle, ctx } = harness(async () => jsonResult({}));
      const override = realpathSync(ctx.agentStorageRoot);
      const spawned = JSON.parse((await tool.execute("override", {
        system_prompt_path: "/tmp/p.md", prompt: "override", cwd_override: override,
      }, undefined, undefined, ctx)).content[0]!.text);
      assert.equal(handle.rpcRegistry.get(spawned.agent_id)!.cwdOverride, override);
      prompts.length = 0;
      await stopAgent(handle.rpcRegistry, spawned.agent_id, undefined, { silent: true });
      await sendTool.execute("resume", { agent_id: spawned.agent_id, message: "resume" });
      assert.deepEqual(prompts, ["resume"]);
      await tool.execute("inherited", { system_prompt_path: "/tmp/p.md", prompt: "inherited" }, undefined, undefined, ctx);
      assert.deepEqual(starts, [override, override, "/tmp"]);

      const beforeInvalid = handle.rpcRegistry.size;
      await assert.rejects(
        () => tool.execute("relative", { system_prompt_path: "/tmp/p.md", prompt: "relative", cwd_override: "relative" }, undefined, undefined, ctx),
        /cwd_override must be an absolute path/,
      );
      await assert.rejects(
        () => tool.execute("missing", { system_prompt_path: "/tmp/p.md", prompt: "missing", cwd_override: join(override, "missing") }, undefined, undefined, ctx),
        /cwd_override must be an existing directory/,
      );
      const file = join(override, "not-a-directory");
      writeFileSync(file, "file");
      await assert.rejects(
        () => tool.execute("file", { system_prompt_path: "/tmp/p.md", prompt: "file", cwd_override: file }, undefined, undefined, ctx),
        /cwd_override must be an existing directory/,
      );
      assert.equal(handle.rpcRegistry.size, beforeInvalid, "invalid cwd overrides allocate no child record");
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("spawn admission preserves omitted legacy authority and persists restricted-child bindings", async () => {
    const rpc = installRpcHarness();
    try {
      const legacy = harness(async () => jsonResult({}));
      const legacyResult = JSON.parse((await legacy.tool.execute("legacy", { system_prompt_path: "/tmp/p.md", prompt: "legacy" }, undefined, undefined, legacy.ctx)).content[0]!.text);
      assert.deepEqual(legacy.handle.rpcRegistry.get(legacyResult.agent_id)!.delegation!.write, { mode: "unrestricted" });
      await legacy.handle.stopAll();

      const scoped = harness(async () => jsonResult({}), undefined, { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false });
      const target = join(realpathSync(scoped.ctx.agentStorageRoot), "future.txt");
      const scopedResult = JSON.parse((await scoped.tool.execute("scoped", {
        system_prompt_path: "/tmp/p.md", prompt: "bounded",
        write_scopes: [{ path: target, kind: "file" }],
      }, undefined, undefined, scoped.ctx)).content[0]!.text);
      const scopedDelegation = scoped.handle.rpcRegistry.get(scopedResult.agent_id)!.delegation!;
      assert.deepEqual(scopedDelegation.write, {
        mode: "scoped", scopes: [{ path: target, kind: "file" }],
      });
      assert.equal(scopedDelegation.tools.includes("edit"), true);
      assert.equal(scopedDelegation.tools.includes("write"), true);
      for (const absent of ["bash", "delete", "rename"]) assert.equal(scopedDelegation.tools.includes(absent), false);
      assert.deepEqual(scopedResult.write_scopes, { status: "bound", count: 1 });
      await scoped.handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("spawn persists delegation once before registration and start, retaining later lifecycle writes", async t => {
    let starts = 0;
    let checkBeforeStart = () => {};
    const rpc = installRpcHarness(undefined, () => { checkBeforeStart(); starts++; });
    const rename = fs.renameSync;
    const writes: OwnershipMetadata[] = [];
    t.mock.method(fs, "renameSync", (from, to) => {
      if (String(to).endsWith("ownership.json")) writes.push(JSON.parse(readFileSync(from, "utf8")));
      return rename(from, to);
    });
    syncBuiltinESMExports();
    let registered: ReturnType<typeof harness> | undefined;
    try {
      for (const scoped of [false, true]) {
        writes.length = 0;
        registered = harness(async () => jsonResult({}), undefined, scoped ? { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false } : undefined);
        const { tool, handle, ctx } = registered;
        const target = join(realpathSync(ctx.agentStorageRoot), "result.md");
        const expectedWrite = scoped ? { mode: "scoped", scopes: [{ path: target, kind: "file" }] } : { mode: "unrestricted" };
        const startsBefore = starts;
        const set = handle.rpcRegistry.set.bind(handle.rpcRegistry);
        const registration = t.mock.method(handle.rpcRegistry, "set", (id, record) => {
          assert.equal(starts, startsBefore, "persistence and registration precede child start");
          assert.equal(writes.length, 1, "there is no immediate delegation-only update before registration");
          assert.deepEqual(writes[0]!.delegation, record.delegation);
          assert.deepEqual(record.ownership!.delegation, record.delegation);
          assert.deepEqual(writes[0]!.delegation!.write, expectedWrite);
          assert.deepEqual(readOwnership(record.ownership!.home)?.delegation, record.delegation);
          return set(id, record);
        });
        checkBeforeStart = () => {
          assert.equal(writes.length, 1, "start observes the complete initial write, not a patched descriptor");
          assert.equal(handle.rpcRegistry.size, 1);
        };
        const result = JSON.parse((await tool.execute("first-write", {
          system_prompt_path: "/tmp/p.md", prompt: "bounded",
          ...(scoped ? { write_scopes: [{ path: target, kind: "file" }] } : {}),
        }, undefined, undefined, ctx)).content[0]!.text);
        assert.equal(registration.mock.callCount(), 1);
        assert.equal(starts, startsBefore + 1);
        const record = handle.rpcRegistry.get(result.agent_id)!;
        assert.ok(writes.slice(1).some(metadata => metadata.liveness.lifecycle === "live" && metadata.liveness.running), "the legitimate later prompt lifecycle update remains");
        for (const metadata of writes) assert.deepEqual(metadata.delegation, record.delegation, "later updates preserve the original authority");
        await handle.stopAll();
        registered = undefined;
      }
    } finally {
      await registered?.handle.stopAll();
      rpc.restore(); t.mock.restoreAll(); syncBuiltinESMExports();
    }
  });

  test("initial ownership persistence failure rejects before registration or child start and closes the channel", async t => {
    let starts = 0;
    const rpc = installRpcHarness(undefined, () => { starts++; });
    const registered = harness(async () => jsonResult({}), undefined, { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false });
    const { tool, handle, ctx } = registered;
    const registration = t.mock.method(handle.rpcRegistry, "set");
    const close = t.mock.method(ParentChannel.prototype, "close");
    const rename = fs.renameSync;
    const failure = Object.assign(new Error("fixture initial ownership write failure"), { code: "EIO" });
    let rejected: OwnershipMetadata | undefined;
    let attempts = 0;
    t.mock.method(fs, "renameSync", (from, to) => {
      if (String(to).endsWith("ownership.json")) {
        attempts++;
        rejected = JSON.parse(readFileSync(from, "utf8"));
        throw failure;
      }
      return rename(from, to);
    });
    syncBuiltinESMExports();
    try {
      const target = join(realpathSync(ctx.agentStorageRoot), "result.md");
      await assert.rejects(() => tool.execute("failed-first-write", {
        system_prompt_path: "/tmp/p.md", prompt: "must not launch",
        write_scopes: [{ path: target, kind: "file" }],
      }, undefined, undefined, ctx), error => error === failure);
      assert.equal(attempts, 1, "an initial EIO rejects allocation instead of becoming a later best-effort update");
      assert.deepEqual(rejected?.delegation?.write, { mode: "scoped", scopes: [{ path: target, kind: "file" }] });
      assert.equal(starts, 0);
      assert.equal(registration.mock.callCount(), 0);
      assert.equal(handle.rpcRegistry.size, 0);
      assert.equal(close.mock.callCount(), 1, "the already-bound channel is closed on synchronous allocation failure");
      assert.deepEqual(readdirSync(rejected!.home), [], "the failed atomic replacement leaves no descriptor or temp file");
      assert.equal(existsSync(join(rejected!.home, "..", `.${rejected!.agentId}.ownership-lock`)), false, "the write lock is released");
    } finally {
      await handle.stopAll();
      rpc.restore(); t.mock.restoreAll(); syncBuiltinESMExports();
    }
  });

  test("an unrestricted child reports redundant scopes as ignored rather than implying confinement", async () => {
    const rpc = installRpcHarness();
    try {
      const unrestricted = harness(async () => jsonResult({}));
      const result = JSON.parse((await unrestricted.tool.execute("redundant", {
        system_prompt_path: "/tmp/p.md", prompt: "already unrestricted",
        write_scopes: [{ path: realpathSync(unrestricted.ctx.agentStorageRoot), kind: "tree" }],
      }, undefined, undefined, unrestricted.ctx)).content[0]!.text);
      assert.deepEqual(result.write_scopes, { status: "ignored", reason: "child already has unrestricted native edit/write authority" });
      assert.deepEqual(unrestricted.handle.rpcRegistry.get(result.agent_id)!.delegation!.write, { mode: "unrestricted" });
      const beforeInvalid = unrestricted.handle.rpcRegistry.size;
      await assert.rejects(
        () => unrestricted.tool.execute("invalid-redundant", {
          system_prompt_path: "/tmp/p.md", prompt: "invalid", write_scopes: [{ path: "relative", kind: "tree" }],
        }, undefined, undefined, unrestricted.ctx),
        /path must be absolute/,
      );
      assert.equal(unrestricted.handle.rpcRegistry.size, beforeInvalid, "invalid redundant scopes still fail before allocation");
      await unrestricted.handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("spawn-time monotonicity accepts narrowing and rejects widening before child allocation", async () => {
    const rpc = installRpcHarness();
    const previousPolicy = process.env[DELEGATION_ENV];
    try {
      const narrowed = harness(async () => jsonResult({}), undefined, { class: "reviewer", authority: "delegate", readOnly: true, requiresChildren: false });
      const root = realpathSync(narrowed.ctx.agentStorageRoot);
      mkdirSync(join(root, "allowed"));
      process.env[DELEGATION_ENV] = JSON.stringify({
        version: 1, depth: 0, maxDepth: 2, authority: "lead",
        tools: resolveTools("full-worker").split(","), network: { search: false, fetch: false },
        write: { mode: "scoped", scopes: [{ path: root, kind: "tree", include: ["allowed/**"] }] },
      });
      const accepted = JSON.parse((await narrowed.tool.execute("narrow", {
        system_prompt_path: "/tmp/p.md", prompt: "narrow",
        write_scopes: [{ path: join(root, "allowed", "result.md"), kind: "file" }],
      }, undefined, undefined, narrowed.ctx)).content[0]!.text);
      assert.equal(narrowed.handle.rpcRegistry.has(accepted.agent_id), true);
      const sizeBeforeWiden = narrowed.handle.rpcRegistry.size;
      await assert.rejects(
        () => narrowed.tool.execute("widen", {
          system_prompt_path: "/tmp/p.md", prompt: "widen",
          write_scopes: [{ path: join(root, "outside.md"), kind: "file" }],
        }, undefined, undefined, narrowed.ctx),
        /child write capability exceeds parent ceiling/,
      );
      assert.equal(narrowed.handle.rpcRegistry.size, sizeBeforeWiden, "a denied widening allocates no child record");
      await narrowed.handle.stopAll();
    } finally {
      if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
      rpc.restore();
    }
  });

  test("a concurrent concrete dispatch neither reads nor changes the tier dispatch selection", async () => {
    const rpc = installRpcHarness();
    try {
      let tierCalls = 0;
      const catalog = [
        { provider: "openrouter", id: "one-off", hasAuth: true },
        { provider: "openai-codex", id: "gpt-5.6-high", hasAuth: true },
      ];
      const { tool, handle, ctx } = harness(async (name, args) => {
        tierCalls++;
        assert.equal(name, "config.resolve_agent");
        assert.deepEqual(args, { tier: "small", format: "json" });
        await Promise.resolve();
        return jsonResult({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex", effort: "low" });
      }, catalog);
      const [concreteRaw, tierRaw] = await Promise.all([
        tool.execute("concrete", { system_prompt_path: "/tmp/p.md", prompt: "one off", model_name: "openrouter/one-off", model_effort: "default" }, undefined, undefined, ctx),
        tool.execute("tier", { system_prompt_path: "/tmp/p.md", prompt: "tier", model_name: "small", model_effort: "default" }, undefined, undefined, ctx),
      ]);
      const concrete = handle.rpcRegistry.get(JSON.parse(concreteRaw.content[0]!.text).agent_id)!;
      const tier = handle.rpcRegistry.get(JSON.parse(tierRaw.content[0]!.text).agent_id)!;
      assert.equal(tierCalls, 1, "only the tier dispatch reads shared tier configuration");
      assert.deepEqual({ model: concrete.modelBase, effort: concrete.modelEffort, source: concrete.modelSource }, { model: "openrouter/one-off", effort: undefined, source: "concrete" });
      assert.deepEqual({ model: tier.modelBase, effort: tier.modelEffort, source: tier.modelSource }, { model: "openai-codex/gpt-5.6-high", effort: "low", source: "tier" });
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("concrete default makes no thinking override while an explicit supported effort is sent", async () => {
    const thinkingCalls: string[] = [];
    const rpc = installRpcHarness(level => thinkingCalls.push(level));
    try {
      const { tool, handle, ctx } = harness(
        async () => { assert.fail("concrete selection must not consult tier config"); },
        [{ provider: "openrouter", id: "one-off", hasAuth: true }],
      );
      await tool.execute("default", { system_prompt_path: "/tmp/p.md", prompt: "default", model_name: "openrouter/one-off", model_effort: "default" }, undefined, undefined, ctx);
      assert.deepEqual(thinkingCalls, [], "default leaves concrete model effort to Pi");
      await tool.execute("explicit", { system_prompt_path: "/tmp/p.md", prompt: "explicit", model_name: "openrouter/one-off", model_effort: "high" }, undefined, undefined, ctx);
      assert.deepEqual(thinkingCalls, ["high"], "explicit supported effort reaches setThinkingLevel");
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("an omitted model_name never calls config.resolve_agent and still spawns on the inherited model", async () => {
    const rpc = installRpcHarness();
    try {
      let called = false;
      const { tool, handle, ctx } = harness(async () => { called = true; return jsonResult({}); });
      const parsed = JSON.parse((await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi" }, undefined, undefined, ctx)).content[0]!.text);
      assert.equal(called, false);
      assert.ok(parsed.agent_id);
      assert.equal(handle.rpcRegistry.size, 1);
      const record = handle.rpcRegistry.get(parsed.agent_id)!;
      assert.equal(record.ownership?.home, join(realpathSync(ctx.agentStorageRoot), "ws-agents", "test-lead", parsed.agent_id));
      assert.equal(record.ownership?.sessionPath, record.sessionPath);
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("a transport/parse failure on a named tier still inherits and spawns — only a genuine `rejected` verdict refuses", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async () => { throw new Error("transport down"); });
      const parsed = JSON.parse((await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small" }, undefined, undefined, ctx)).content[0]!.text);
      assert.ok(parsed.agent_id);
      assert.equal(handle.rpcRegistry.size, 1);
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("a real owned-home removal failure retains the record and rejects the cap-triggering spawn", async (t) => {
    const rpc = installRpcHarness();
    const previousCap = process.env[WS_PI_AGENT_REGISTRY_CAP_ENV];
    const diagnostics = t.mock.method(console, "error", () => {});
    const notices: string[] = [];
    ownerNotifyRef.current = message => notices.push(message);
    let locked: string | undefined;
    try {
      const { tool, handle, ctx } = harness(async () => jsonResult({}));
      const ownership = allocateAgentHome(createAgentStorageContext("test-lead", ctx.agentStorageRoot), "blocked-delete", "worker");
      locked = join(ownership.home, "locked");
      mkdirSync(locked);
      writeFileSync(join(locked, "payload"), "keep until retry");
      chmodSync(locked, 0o500);
      updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
      handle.rpcRegistry.set("blocked-delete", freshRpcRecord({ agentId: "blocked-delete", ownership, sessionPath: ownership.sessionPath }));
      process.env[WS_PI_AGENT_REGISTRY_CAP_ENV] = "1";

      await assert.rejects(
        () => tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "replacement" }, undefined, undefined, ctx),
        error => {
          assert.match(String(error), /owned-home removal failed/);
          assert.doesNotMatch(String(error), /blocked-delete|payload|EACCES|permission denied/);
          return true;
        },
      );
      assert.equal(handle.rpcRegistry.has("blocked-delete"), true, "failed deletion keeps the only copy of the record retryable");
      assert.equal(handle.rpcRegistry.size, 1, "the replacement spawn is not admitted without an exactly-once eviction");
      assert.equal(existsSync(ownership.home), true, "failed removal was rolled back for retry");
      assert.equal(diagnostics.mock.callCount(), 0);
      assert.deepEqual(notices, ["ws: could not remove an owned-agent home; it was retained for safety."]);
      if (existsSync(locked)) chmodSync(locked, 0o700);
      await handle.stopAll();
    } finally {
      if (locked && existsSync(locked)) chmodSync(locked, 0o700);
      if (previousCap === undefined) delete process.env[WS_PI_AGENT_REGISTRY_CAP_ENV]; else process.env[WS_PI_AGENT_REGISTRY_CAP_ENV] = previousCap;
      ownerNotifyRef.current = undefined;
      rpc.restore();
    }
  });
});

/**
 * 260906 Phase 2 (YAML/TUI dispatch-row rendering): `spawnAgent`'s
 * `ctx.onModelResolved` callback and the two new `RpcAgentRecord` fields it
 * feeds (`modelTier`/`modelSource`). Reuses the same `installRpcHarness`
 * monkey-patch technique as the "ordinary rejection" describe block above
 * (own local copy — that block's helpers are scoped to its own callback).
 */
describe("spawnAgent: onModelResolved (260906 Phase 2 dispatch-row rendering)", () => {
  interface CapturedTool {
    name: string;
    parameters: { properties: Record<string, unknown> };
    execute: (
      id: string,
      params: unknown,
      signal?: AbortSignal,
      update?: (partial: { content: unknown[]; details?: unknown }) => void,
      ctx?: unknown,
    ) => Promise<{ content: Array<{ text: string }>; details?: unknown }>;
  }

  function installRpcHarness() {
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      start: startRpcWithFakeChild, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => { closeFakeChildren(); Object.assign(RpcClient.prototype, original); } };
  }

  function jsonResult(payload: unknown): McpToolCallResult {
    return { content: [{ type: "text", text: JSON.stringify(payload) }] };
  }

  function harness(callTool: McpStdioClient["callTool"]) {
    const tools = new Map<string, CapturedTool>();
    const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
    const bridge = { client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" } } as never;
    const handle = createEngineTestHarness(pi, bridge, { cwd: "/tmp" });
    const ctx = { sessionManager: { getSessionId: () => "test-lead" }, agentStorageRoot: storageRoot(), model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "openai-codex", id: "gpt-5.6-high" }], hasConfiguredAuth: () => true } };
    return { tool: tools.get("engine-spawn")!, sendTool: tools.get("engine-send")!, handle, ctx };
  }

  test("a tier hit invokes onModelResolved once with the correct shape, forwarded as onUpdate details and repeated in the final return; records modelTier/modelSource", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async (name) => { assert.equal(name, "config.resolve_agent"); return jsonResult({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex" }); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small", model_effort: "high" }, undefined, (partial) => updates.push(partial), ctx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.ok(parsed.agent_id);
      const expected = { tier: "small", model: "openai-codex/gpt-5.6-high", effort: "high", inherited: false };
      assert.equal(updates.length, 1, "onModelResolved fires exactly once, before mkdtempSync/registry.set");
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, expected);
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, expected, "the final return repeats the same shape");
      const record = handle.rpcRegistry.get(parsed.agent_id)!;
      assert.equal(record.modelTier, "small");
      assert.equal(record.modelSource, "tier");
      assert.equal(record.modelEffort, "high", "record.modelEffort reuses the same resolvedEffort onModelResolved was called with");
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("an omitted model_name inherit publishes tier:\"inherit\"/inherited:true and records modelSource:\"inherit\" with no modelTier", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async () => { assert.fail("config.resolve_agent must not be called for an omitted model_name"); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi" }, undefined, (partial) => updates.push(partial), ctx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.equal(updates.length, 1);
      const resolved = (updates[0]!.details as { resolved: { tier: string; model?: string; effort?: string; inherited: boolean } }).resolved;
      assert.equal(resolved.tier, "inherit");
      assert.equal(resolved.inherited, true);
      assert.equal(resolved.model, "lead/large", "inherits the ctx.model snapshot");
      const record = handle.rpcRegistry.get(parsed.agent_id)!;
      assert.equal(record.modelTier, undefined);
      assert.equal(record.modelSource, "inherit");
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("model_effort:default with inherited-model dispatch keeps the captured parent effort", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async () => { assert.fail("config.resolve_agent must not be called for inherited dispatch"); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_effort: "default" }, undefined, (partial) => updates.push(partial), ctx);
      const record = handle.rpcRegistry.get(JSON.parse(raw.content[0]!.text).agent_id)!;
      assert.equal(record.modelEffort, "high");
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, { tier: "inherit", model: "lead/large", effort: "high", inherited: true });
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("a transport-failure-forced inherit on a NAMED tier still publishes tier:\"inherit\"/source:\"inherit\" (never the raw requested tier name)", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, handle, ctx } = harness(async () => { throw new Error("transport down"); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small" }, undefined, (partial) => updates.push(partial), ctx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.equal(updates.length, 1);
      const resolved = (updates[0]!.details as { resolved: { tier: string; inherited: boolean } }).resolved;
      assert.equal(resolved.tier, "inherit", "source is \"inherit\" despite a named tier having been requested");
      assert.equal(resolved.inherited, true);
      const record = handle.rpcRegistry.get(parsed.agent_id)!;
      assert.equal(record.modelTier, "small", "the raw requested tier is still recorded on the record even though resolution fell back");
      assert.equal(record.modelSource, "inherit");
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("onModelResolved is never invoked when the ordinary-spawn refusal guard throws first", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, ctx } = harness(async () => jsonResult({ resolved_from: "default", model: "gpt-5.6-terra" }));
      const updates: unknown[] = [];
      await assert.rejects(
        () => tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small" }, undefined, (partial) => updates.push(partial), ctx),
        /ws-pi-agent: ws-agent-spawn rejected:/,
      );
      assert.equal(updates.length, 0);
    } finally { rpc.restore(); }
  });

  test("internal send steers streaming targets and prompts live-idle targets", async () => {
    const { sendTool, handle } = harness(async () => jsonResult({}));
    const streamingClient = fakeRpcClient();
    handle.rpcRegistry.set("streaming", freshRpcRecord({
      agentId: "streaming",
      client: streamingClient.client,
      streaming: true,
      running: true,
    }));
    const idleClient = fakeRpcClient();
    handle.rpcRegistry.set("idle", freshRpcRecord({ agentId: "idle", client: idleClient.client }));

    await sendTool.execute("streaming-send", { agent_id: "streaming", message: "steer this way" });
    assert.deepEqual(streamingClient.calls, [["steer", "steer this way"]]);
    await sendTool.execute("idle-send", { agent_id: "idle", message: "start a new run" });
    assert.deepEqual(idleClient.calls, [["prompt", "start a new run"]]);
    handle.rpcRegistry.clear();
  });

  test("ws-agent-send reconstructs the target's resolved line from the record, including the no-modelSource revived-record fallback", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, sendTool, handle, ctx } = harness(async () => jsonResult({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex" }));
      const spawnRaw = await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "hi", model_name: "small" }, undefined, undefined, ctx);
      const spawned = JSON.parse(spawnRaw.content[0]!.text);

      const sendRaw = await sendTool.execute("call2", { agent_id: spawned.agent_id, message: "hello" });
      assert.deepEqual((sendRaw.details as { resolved?: unknown } | undefined)?.resolved, { tier: "small", model: "openai-codex/gpt-5.6-high", effort: undefined, inherited: false });

      // Simulate a record revived from a sidecar snapshot (modelTier/modelSource are NOT persisted there).
      const record = handle.rpcRegistry.get(spawned.agent_id)!;
      record.modelTier = undefined;
      record.modelSource = undefined;
      const revivedSendRaw = await sendTool.execute("call3", { agent_id: spawned.agent_id, message: "hi again" });
      assert.deepEqual(
        (revivedSendRaw.details as { resolved?: unknown } | undefined)?.resolved,
        { tier: "inherit", model: "openai-codex/gpt-5.6-high", effort: undefined, inherited: true },
        "missing modelSource degrades to inherited:true/tier:\"inherit\" rather than throwing or guessing",
      );
      await handle.stopAll();
    } finally { rpc.restore(); }
  });
});
