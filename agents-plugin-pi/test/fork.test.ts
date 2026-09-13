/**
 * Unit tests for fork.ts's pure-logic seams (260904 Phase 1, side-thread
 * fork ticket): `computeForkToolSurface`/`addForkToolIfLead` (§3's dynamic
 * tool-surface formula, including the role-differentiation fix — a fork
 * must never regain `ws-fork`), `shouldNudge`/`classifyForkTurnOutcome`/
 * `isIdleWithoutFinal` (§4's anti-bleed disambiguation), `extractReportField`/
 * `validateFinalReportShape`/`checkExpectsCommitCompletion` (§4's required
 * report shape + `expects_commit` non-completion rule), `tailLines`, and
 * `getForkSourceSessionFile`.
 *
 * `wireAntiBleedLoop`'s own event handling is additionally driven here
 * `wireAntiBleedLoop`'s own event handling is additionally driven here
 * against a duck-typed fake client/record for the seams 260904 Phase 2 added
 * (the thread-bound suppression, and the question-report hook through its
 * real `applyRpcEvent` call site) and for 260905's push model (every advisory
 * is a `ws-agent-advisory` push, the nudge routes through `promptAgent`, and
 * the idle-without-final judgment reads `reportLog` filtered by the last lead
 * prompt).
 * Review relay #1 (C1/I1/I4) adds: `buildForkSpawnCtx` — the `ws-fork` spawn
 * ctx, extracted so its (required, silently droppable) `pi` field is asserted
 * rather than assumed — with a push-on-final check through the real
 * `attachEventListener`; `armForkRoleWiring`, the shared question-routing +
 * anti-bleed arm used by both a fresh spawn and the shutdown sidecar's orphan
 * revival; and `deliverAs`/status-line assertions on the advisory pushes (the
 * harness now captures `sendMessage`'s options argument, which it previously
 * dropped).
 *
 * NOT covered here — genuinely live-gate only, mirroring
 * test/execute-gateway.test.ts's own pure/IO split: `registerFork`'s tool
 * `execute()` body (needs a live `pi --mode rpc` session or a real
 * `RpcClient`). Exercised only by the plan's documented manual verification
 * gate (no provider credentials in this sandbox — deferred, not faked).
 *
 * 260906 Phase 2 addendum: narrows the above — `registerFork`'s
 * `onModelResolved` forwarding IS unit-testable via the same
 * `installRpcHarness` `RpcClient.prototype` monkey-patch technique used by
 * test/spawner.test.ts and test/execute-gateway.test.ts, since `spawnAgent`'s
 * only non-injectable dependency is that same transport. See the
 * "ws-fork: onModelResolved forwarding" describe block below. The rest of
 * `registerFork`'s execute() (anti-bleed wiring, question routing, etc.)
 * remains live-gate only.
 *
 * Run with: node --test test/  (from agents-plugin-pi/).
 */

import { afterEach, test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  FORK_TOOL_NAME,
  FORK_EXCLUDED_TOOL_NAMES,
  computeForkToolSurface,
  addForkToolIfLead,
  getForkSourceSessionFile,
  buildForkDirectiveText,
  buildForkInitialMessage,
  armForkRoleWiring,
  buildForkSpawnCtx,
} from "../src/fork.ts";
import { registerFork as registerForkBase } from "../src/fork.ts";
import { leadIdleRef, registerPushFlush, flushHeldPushes, applyRpcEvent, attachEventListener, REPORT_TO_LEAD_TOOL_NAME, type RpcAgentRecord, type RpcAgentRegistry } from "../src/spawner.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
import { WS_PI_FORK_READY_NONCE_ENV, WS_PI_FORK_READY_PATH_ENV } from "../src/process-role.ts";
import type { BridgeHandle } from "../src/bridge.ts";
import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { mkdtempSync, realpathSync, rmSync, writeFileSync } from "node:fs";

const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function registerFork(pi: any, bridge: any, registry: any, sessionCtx: any, ...rest: any[]) {
  return registerForkBase(pi, bridge, registry, { ...sessionCtx, extensionPath: sessionCtx.extensionPath ?? TEST_EXTENSION_ENTRY }, ...rest);
}
import { tmpdir } from "node:os";
import { join } from "node:path";

// Phase 2: these payload-focused fixtures model a user wake followed by
// confirmed streaming start, rather than the retired undefined-idle fallback.
function initializePushLifecycle(pi: ExtensionAPI): void {
  let idle = true;
  leadIdleRef.current = () => idle;
  const handlers = new Map<string, () => void>();
  pi.on = ((event: string, handler: () => void) => handlers.set(event, handler)) as ExtensionAPI["on"];
  pi.sendUserMessage = (content, options) => {
    assert.match(String(content), /^\d+ ws messages waiting;[^\n]+$/);
    assert.deepEqual(options, { deliverAs: "followUp" });
    idle = false;
    handlers.get("agent_start")?.();
    idle = true;
  };
  registerPushFlush(pi, { delayMs: () => 10 });
}

describe("FORK_TOOL_NAME / FORK_EXCLUDED_TOOL_NAMES", () => {
  test("FORK_TOOL_NAME is the literal ws-fork", () => {
    assert.equal(FORK_TOOL_NAME, "ws-fork");
  });

  test("has no schema exclusions: role handlers refuse side-thread operations", () => {
    assert.deepEqual([...FORK_EXCLUDED_TOOL_NAMES], []);
  });
});

describe("computeForkToolSurface", () => {
  test("returns an ordered copy unchanged, including duplicate and role-restricted registrations", () => {
    const source = ["bash", "ws-queue-question", FORK_TOOL_NAME, "ws-queue-question"];
    const result = computeForkToolSurface(source);
    assert.deepEqual(result, source);
    assert.notEqual(result, source);
  });

  test("empty input remains empty", () => {
    assert.deepEqual(computeForkToolSurface([]), []);
  });
});

describe("addForkToolIfLead (risk-signal fix: role-differentiated, never folded into computeLeadActiveTools)", () => {
  test("role undefined (true top lead) gains ws-fork when absent", () => {
    const result = addForkToolIfLead(["bash", "edit"], undefined);
    assert.ok(result.includes(FORK_TOOL_NAME));
  });

  test("role undefined does not duplicate ws-fork if already present", () => {
    const result = addForkToolIfLead(["bash", FORK_TOOL_NAME], undefined);
    assert.equal(result.filter((name) => name === FORK_TOOL_NAME).length, 1);
  });

  test('role "fork" NEVER regains ws-fork, even if somehow present in the input (defense in depth)', () => {
    const withoutIt = addForkToolIfLead(["bash", "edit"], "fork");
    assert.ok(!withoutIt.includes(FORK_TOOL_NAME));
    const alreadyPresent = addForkToolIfLead(["bash", FORK_TOOL_NAME], "fork");
    assert.deepEqual(alreadyPresent, ["bash", FORK_TOOL_NAME], "an existing entry is left as-is, but never newly added");
  });

  test('role "worker" and "explore" never gain ws-fork', () => {
    assert.ok(!addForkToolIfLead(["bash"], "worker").includes(FORK_TOOL_NAME));
    assert.ok(!addForkToolIfLead(["bash"], "explore").includes(FORK_TOOL_NAME));
  });

  test("an empty active-tools list for the true lead ends up with exactly ws-fork", () => {
    assert.deepEqual(addForkToolIfLead([], undefined), [FORK_TOOL_NAME]);
  });
});

describe("getForkSourceSessionFile", () => {
  test("extracts the session file path from a well-formed toolCtx.sessionManager.getSessionFile()", () => {
    const toolCtx = { sessionManager: { getSessionFile: () => "/tmp/lead-session.jsonl" } };
    assert.equal(getForkSourceSessionFile(toolCtx), "/tmp/lead-session.jsonl");
  });

  test("returns undefined when toolCtx, sessionManager, or getSessionFile is missing", () => {
    assert.equal(getForkSourceSessionFile(undefined), undefined);
    assert.equal(getForkSourceSessionFile({}), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: {} }), undefined);
  });

  test("returns undefined when getSessionFile returns an empty string or a non-string", () => {
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => "" } }), undefined);
    assert.equal(getForkSourceSessionFile({ sessionManager: { getSessionFile: () => undefined } }), undefined);
  });
});

describe("buildForkDirectiveText", () => {
  test("keeps questions intermediate and the structured ordinary answer terminal without adapter parsing", () => {
    const text = buildForkDirectiveText();
    assert.ok(text.includes(REPORT_TO_LEAD_TOOL_NAME));
    assert.ok(text.includes('kind:"question"'));
    assert.ok(!text.includes('kind:"final"'));
    for (const field of ["Outcome", "Files changed", "Verification", "Blockers", "Commit", "Decisions"]) {
      assert.ok(text.includes(`${field}:`), `expected the directive to name requested field "${field}"`);
    }
    assert.match(text, /adapter does not parse or approve/i);
  });

  test("expects_commit stays visible in the prompt without changing settlement semantics", () => {
    assert.match(buildForkDirectiveText(true), /Commit: <required commit hash or range>/);
    assert.match(buildForkDirectiveText(false), /literal "none"/);
  });

  test("carries no identity-framing persona opener", () => {
    const text = buildForkDirectiveText();
    assert.ok(!/\byou\s+are\s+a\b/i.test(text), `directive text must not open with "you are a ..." identity framing: ${text}`);
  });

  test("carries no ALL-CAPS override-style words", () => {
    const text = buildForkDirectiveText();
    const allCapsWords = text.match(/\b[A-Z]{4,}\b/g) ?? [];
    assert.deepEqual(allCapsWords, [], `directive text must not carry ALL-CAPS override word(s): ${JSON.stringify(allCapsWords)}`);
  });
});

describe("buildForkInitialMessage (260905 structural anti-bleed frame)", () => {
  const task = "Run `od -An -N8 -tx1 /dev/urandom` and report the hex.";

  test("fences the task, demotes inherited context, and keeps the task inline", () => {
    const msg = buildForkInitialMessage(task);
    assert.ok(msg.includes(task), "the lead's task text must survive inside the frame");
    assert.ok(/reference\/background only/i.test(msg), "must demote inherited context to reference");
    assert.ok(msg.includes("--- Message from the lead ---"), "must fence the lead's message");
    assert.ok(msg.includes("--- end of message ---"), "must close the fence");
    assert.ok(msg.includes(REPORT_TO_LEAD_TOOL_NAME), "must keep the report contract pointer");
  });

  test("stays calm — no ALL-CAPS override words (chosen over the aggressive header)", () => {
    const allCapsWords = buildForkInitialMessage(task).match(/\b[A-Z]{4,}\b/g) ?? [];
    assert.deepEqual(allCapsWords, [], `framed message must stay calm: ${JSON.stringify(allCapsWords)}`);
  });
});

/**
 * 260904 Phase 2 (review relay #1 C1/I6), rewritten for 260905's push model:
 * the loop's thread-bound suppression, its advisory PUSHES (formerly
 * `pi.sendUserMessage` steers), the nudge routed through `promptAgent`, and
 * the question-report hook's new suppression contract — driven against a
 * duck-typed fake client/record (no subprocess, no real `RpcClient`). The
 * event stream is replayed by hand through the listener `wireAntiBleedLoop`
 * registers; the hook is exercised through the real `applyRpcEvent` (its
 * actual call site) rather than through the loop.
 */
describe("buildForkSpawnCtx (the ws-fork push channel)", () => {
  const bridge = {
    wsToolNames: ["ws__ferrule"],
    defaultSessionKeyRef: { current: "amber-otter-canyon" },
    client: { callTool: async () => ({ content: [] }) },
  } as unknown as BridgeHandle;
  const pi = { sendMessage() {} } as unknown as ExtensionAPI;
  const catalog = [{ provider: "p", id: "m", hasAuth: true }];
  const notifyTierWarning = () => {};

  test("carries the spawning session's own pi — without it a fork has no report channel at all", () => {
    const ctx = buildForkSpawnCtx(pi, bridge, { cwd: "/repo" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      catalog, notifyTierWarning,
      explicitTools: "read,grep",
    });
    assert.equal(ctx.pi, pi, "a ws-fork spawn must push into the session that spawned it");
  });

  test("carries the fork spawn shape: --fork source, explicit tools, parent session key, spawnRole fork, and the bridge's client", () => {
    const ctx = buildForkSpawnCtx(pi, bridge, { cwd: "/repo" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      catalog, notifyTierWarning,
      explicitTools: "read,grep",
      inheritModel: "openrouter/some-model",
    });
    assert.equal(ctx.catalog, catalog);
    assert.equal(ctx.notifyTierWarning, notifyTierWarning);
    assert.equal(ctx.forkFrom, "/tmp/lead-session.jsonl");
    assert.equal(ctx.explicitTools, "read,grep");
    assert.equal(ctx.parentSessionKey, "amber-otter-canyon");
    assert.equal(ctx.spawnRole, "fork");
    assert.equal(ctx.inheritModel, "openrouter/some-model");
    assert.equal(ctx.cwd, "/repo");
    assert.deepEqual([...ctx.wsToolNames], ["ws__ferrule"]);
    assert.equal(ctx.client, bridge.client, "must read the ws-mcp client off the bridge");
  });

  test("C1: a record wired through that ctx's pi pushes its ordinary settled result", async () => {
    const sent: Array<{ customType?: string; details?: Record<string, unknown> }> = [];
    const pushPi = {
      sendMessage: (message: { customType?: string; details?: { items?: Array<{ customType?: string; details?: Record<string, unknown> }> } }) => {
        if (message.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(message.details?.items)) sent.push(...message.details.items);
        else sent.push(message);
      },
    } as unknown as ExtensionAPI;
    initializePushLifecycle(pushPi);
    const ctx = buildForkSpawnCtx(pushPi, bridge, { cwd: "/repo" }, {
      forkFrom: "/tmp/lead-session.jsonl",
      catalog, notifyTierWarning,
      explicitTools: "read",
    });

    let listener: ((evt: unknown) => void) | undefined;
    const client = {
      onEvent(l: (evt: unknown) => void) {
        listener = l;
        return () => {};
      },
      getState: async () => ({}),
      getLastAssistantText: async () => "Outcome: shipped",
    } as unknown as RpcClient;
    const record = {
      agentId: "fork-1",
      sessionPath: "/tmp/f.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: true,
      reportLog: [],
      client,
    } as unknown as RpcAgentRecord;
    const registry: RpcAgentRegistry = new Map([["fork-1", record]]);

    // Exactly what spawnAgent does with the ctx it is handed.
    attachEventListener(ctx.pi, registry, record, client);
    listener?.({ type: "message_end", message: { role: "assistant", content: [{ type: "text", text: "Outcome: shipped" }] } });
    listener?.({ type: "agent_settled" });
    await new Promise((resolve) => setTimeout(resolve, 10));
    flushHeldPushes(pushPi, true);

    assert.deepEqual(
      sent.map((m) => m.customType),
      ["ws-agent-settled"],
      "the ordinary settled answer is the fork's terminal result",
    );
    assert.equal(sent[0].details?.last_message, "Outcome: shipped");
    assert.equal(sent[0].details?.reason, "idle");
    assert.equal(sent[0].details?.agent_id, "fork-1");
  });
});

/**
 * Review relay #1, I1: role wiring must be re-armable on a record the shutdown
 * sidecar revived as DORMANT, not only on a freshly-spawned live one.
 */
describe("armForkRoleWiring (fresh spawn and sidecar revival)", () => {
  const pi = { sendMessage() {} } as unknown as ExtensionAPI;

  function dormantForkRecord(): RpcAgentRecord {
    return {
      agentId: "fork-1",
      sessionPath: "/tmp/f.jsonl",
      systemPromptPath: "/tmp/p.md",
      wsToolNames: [],
      toolGroup: "full-worker",
      spawnRole: "fork",
      streaming: false,
      running: false,
      reportLog: [],
    } as unknown as RpcAgentRecord;
  }

  test("arms question routing on a dormant record — a revived fork's question still reaches the owner surface", () => {
    const record = dormantForkRecord();
    const asked: Array<{ agentId: string; message: string }> = [];
    armForkRoleWiring(pi, new Map([["fork-1", record]]), record, (agentId, message) => {
      asked.push({ agentId, message });
      return "[ws] thread q1 — the owner answers this.";
    });

    const outcome = applyRpcEvent(record, {
      type: "tool_execution_start",
      toolName: REPORT_TO_LEAD_TOOL_NAME,
      args: { kind: "question", message: "which anchor?" },
    });
    assert.deepEqual(asked, [{ agentId: "fork-1", message: "which anchor?" }]);
    assert.deepEqual(
      outcome,
      { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: "[ws] thread q1 — the owner answers this." }, deliverAs: "followUp" } },
      "§1: routed to the owner, and the lead sees the registration notice, not a ws-agent-question",
    );
  });

});

/**
 * 260906 Phase 2 (dispatch-row rendering): `ws-fork`'s `onModelResolved`
 * forwarding, both for a named `model_name` tier hit and an omitted-tier
 * inherit — mirrors `test/spawner.test.ts`'s "spawnAgent: onModelResolved"
 * and `test/execute-gateway.test.ts`'s "ws-execute: onModelResolved
 * forwarding" describe blocks, driven at the `ws-fork` tool level.
 */
describe("ws-fork: onModelResolved forwarding (260906 Phase 2)", () => {
  interface CapturedTool {
    execute: (
      id: string,
      params: unknown,
      signal?: AbortSignal,
      update?: (partial: { content: unknown[]; details?: unknown }) => void,
      ctx?: unknown,
    ) => Promise<{ content: Array<{ type: string; text: string }>; details?: unknown }>;
  }

  // A `ws-fork` spawn adds one extra handshake beyond the ordinary
  // RpcClient-transport seam `test/spawner.test.ts`'s own `installRpcHarness`
  // patches: `validateForkReadiness` (spawner.ts) reads a real
  // `ready.json` file that a genuine forked child process would write via
  // `WS_PI_FORK_READY_PATH_ENV`/`WS_PI_FORK_READY_NONCE_ENV` (its own env,
  // set by `buildRpcClientOptions`). `prepareForkLaunch` creates that
  // directory and writes the nonce/path BEFORE `client.start()` runs, so the
  // patched `start()` below can safely write the matching readiness file
  // itself once it exists — no real child process needed. `this.options` is
  // a private TS field but a plain runtime property; reading it here is the
  // only way to recover the per-spawn nonce/path pair from inside a
  // patched prototype method with no other injectable seam.
  function installRpcHarness() {
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      async start(this: { options?: { env?: Record<string, string>; args?: string[] } }) {
        const env = this.options?.env;
        const sessionDir = this.options?.args?.[this.options.args.indexOf("--session-dir") + 1];
        const readinessPath = env?.[WS_PI_FORK_READY_PATH_ENV];
        const nonce = env?.[WS_PI_FORK_READY_NONCE_ENV];
        if (readinessPath && nonce) {
          writeFileSync(readinessPath, JSON.stringify({
            nonce,
            ownSessionKey: "fork-child-key",
            sessionPath: `${sessionDir}/session.jsonl`,
            sessionId: "fork-child-session-id",
          }));
        }
      },
      stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
      getState: async function(this: { options?: { args?: string[] } }) { const dir = this.options?.args?.[this.options.args.indexOf("--session-dir") + 1]; return { model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: `${dir}/session.jsonl`, sessionId: "fork-child-session-id" }; },
    });
    return { restore: () => Object.assign(RpcClient.prototype, original) };
  }

  function harness(callTool: (name: string, args?: unknown) => Promise<{ content: Array<{ type: string; text: string }> }>) {
    const tools = new Map<string, CapturedTool & { name: string }>();
    const pi = {
      registerTool: (def: { name: string } & CapturedTool) => tools.set(def.name, def),
      sendMessage() {}, sendUserMessage() {}, on() {},
      getActiveTools: () => ["bash", "read", "edit", "ws-agent-spawn"],
      getAllTools: () => [],
      getThinkingLevel: () => "high",
    } as unknown as ExtensionAPI;
    const bridge = { client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" } } as unknown as BridgeHandle;
    const registry: RpcAgentRegistry = new Map();
    registerFork(pi, bridge, registry, { cwd: "/tmp" });
    const toolCtx = {
      sessionManager: { getSessionFile: () => "/tmp/fake-fork-source.jsonl", getSessionId: () => "test-lead" },
      agentStorageRoot: storageRoot(),
      model: { provider: "lead", id: "large" },
      thinkingLevel: "high",
      modelRegistry: { getAll: () => [{ provider: "openai-codex", id: "gpt-5.6-high" }, { provider: "pi", id: "small" }], hasConfiguredAuth: () => true },
    };
    return { tool: tools.get(FORK_TOOL_NAME)!, registry, toolCtx };
  }

  test("a named model_name tier hit forwards onModelResolved as onUpdate details and the final return", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, registry, toolCtx } = harness(async (name) => { assert.equal(name, "config.resolve_agent"); return { content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex" }) }] }; });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { prompt: "work on this", model_name: "small" }, undefined, (partial) => updates.push(partial), toolCtx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.ok(parsed.agent_id);
      const expected = { tier: "small", model: "openai-codex/gpt-5.6-high", effort: undefined, inherited: false };
      assert.equal(updates.length, 1, "onModelResolved fires exactly once");
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, expected);
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, expected, "the final return repeats the same shape");
      const record = registry.get(parsed.agent_id)!;
      assert.equal(record.ownership?.home, join(realpathSync(toolCtx.agentStorageRoot), "ws-agents", "test-lead", parsed.agent_id));
      assert.equal(record.ownership?.sessionPath, record.sessionPath);
      assert.equal(record.modelTier, "small");
      assert.equal(record.modelSource, "tier");
    } finally { rpc.restore(); }
  });

  test("an omitted model_name never calls config.resolve_agent and publishes an inherited resolved line", async () => {
    const rpc = installRpcHarness();
    try {
      const { tool, registry, toolCtx } = harness(async () => { assert.fail("config.resolve_agent must not be called for an omitted model_name"); });
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { prompt: "work on this" }, undefined, (partial) => updates.push(partial), toolCtx);
      const parsed = JSON.parse(raw.content[0]!.text);
      assert.ok(parsed.agent_id);
      assert.equal(updates.length, 1);
      const resolved = (updates[0]!.details as { resolved: { tier: string; model?: string; effort?: string; inherited: boolean } }).resolved;
      assert.equal(resolved.tier, "inherit");
      assert.equal(resolved.inherited, true);
      assert.equal(resolved.model, "lead/large", "inherits the ctx.model snapshot");
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, resolved);
      const record = registry.get(parsed.agent_id)!;
      assert.equal(record.modelTier, undefined);
      assert.equal(record.modelSource, "inherit");
    } finally { rpc.restore(); }
  });
});
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
