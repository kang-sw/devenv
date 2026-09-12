/**
 * Unit tests for spawner.ts's pure-logic seams: resolveTools,
 * resolveModelForAliasViaWsMcp (Phase 4:
 * async, ws-mcp-`config.resolve_agent`-backed tier resolution against a stub
 * `client.callTool`, replacing the old file-catalog-backed
 * `resolveModelForAlias`), applyRpcEvent's streaming/report bookkeeping and its
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
 * `shouldPushToLead` (the role gate), `computeRunningStatusLine` (the fan-in
 * running count), `buildPushContent`/`pushToLead` (message shape and
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
  TOOL_GROUPS,
  resolveModelForAliasViaWsMcp,
  effectiveModelEffort,
  applyRpcEvent,
  attachEventListener,
  buildPushContent,
  computeRunningStatusLine,
  hasRunningAgents,
  flushHeldPushes,
  heldPushQueue,
  leadCompactingRef,
  leadIdleRef,
  leadWakeStartPendingRef,
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
  startForkFinish,
  buildRpcClientOptions as buildRpcClientOptionsBase,
  inheritModelFromToolCtx,
  resolveSpawnToolGroup,
  resolveAgentId,
  resolveAgentRegistryCap,
  reserveAgentAlias,
  evictForCapacity,
  runSpawnGuards,
  truncatePromptForStorage,
  WS_PI_AGENT_REGISTRY_CAP_ENV,
  DEFAULT_AGENT_REGISTRY_CAP,
  PROMPT_STORAGE_CAP_BYTES,
  registerAgentTools as registerAgentToolsBase,
  type RpcAgentRecord,
  type RpcAgentRegistry,
  type TerminalDelivery,
  type ToolGroup,
} from "../src/spawner.ts";
import { WS_PI_PARENT_SESSION_KEY_ENV, WS_PI_SPAWN_ROLE_ENV, type SpawnRole } from "../src/process-role.ts";
// 260906 (registerAgentTools's role-keyed explore registration): a VALUE
// import, not `import type` — the new describe block below monkey-patches
// RpcClient.prototype.{start,onEvent,prompt} for the lead/fork execute() path
// so the real spawnAgent code runs to completion with no real subprocess
// (see that describe block's own doc comment for why this is safe and the
// worker/execute-worker leaf path deliberately never invokes execute() at all).
import { RpcClient } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { McpStdioClient, McpToolCallResult } from "../src/mcp-stdio-client.ts";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import { WEB_HOME_ENV, WEB_NONCE_ENV } from "../src/web-readiness.ts";
import { allocateAgentHome, createAgentStorageContext, updateOwnership } from "../src/agent-storage.ts";
import { PUSH_BATCH_CUSTOM_TYPE } from "../src/push-protocol.ts";
const REAL_EXTENSION_ENTRY = fileURLToPath(new URL("../src/index.ts", import.meta.url));
async function startRpcWithWebProof(this: { options?: { env?: Record<string, string> } }) {
  const env = this.options?.env ?? {};
  if (env[WEB_HOME_ENV]) writeFileSync(join(env[WEB_HOME_ENV], "web-tools-ready.json"), JSON.stringify({ nonce: env[WEB_NONCE_ENV], tools: ["web_search", "ws_web_fetch"] }));
}

// Deliberately contains spaces: argv is passed as an array, so this exact
// loaded-entry identity must reach the child without shell escaping/rebuilds.
const TEST_EXTENSION_ENTRY = "/tmp/loaded ws adapter/index copy.ts";
function buildRpcClientOptions(...args: any[]) {
  return buildRpcClientOptionsBase(
    args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], TEST_EXTENSION_ENTRY,
  );
}
function registerAgentTools(pi: any, bridge: any, sessionCtx: any, ...rest: any[]) {
  const register = pi.registerTool.bind(pi);
  pi.registerTool = (tool: any) => register({ ...tool, execute: (...args: any[]) => {
    if (args[1]?.system_prompt_path) {
      const path = join(storageRoot(), "prompt.md");
      writeFileSync(path, "Offline worker prompt");
      args[1] = { ...args[1], system_prompt_path: path };
    }
    return tool.execute(...args);
  } });
  return registerAgentToolsBase(pi, bridge, { ...sessionCtx, extensionPath: sessionCtx.extensionPath ?? REAL_EXTENSION_ENTRY }, ...rest);
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

describe("TOOL_GROUPS / resolveTools", () => {
  test("read-only carries no bash and no ws__* tools", () => {
    assert.deepEqual([...TOOL_GROUPS["read-only"]], ["read", "grep", "find", "ls", REPORT_TO_LEAD_TOOL_NAME]);
    assert.equal(resolveTools("read-only", ["ws__playbook_render"]), `read,grep,find,ls,${REPORT_TO_LEAD_TOOL_NAME}`);
  });

  test("full-worker includes built-ins plus the literal explore and ws-report-to-lead tools plus every passed ws__* name, in order", () => {
    assert.equal(
      resolveTools("full-worker", ["ws__playbook_render", "ws__ferrule"]),
      "read,bash,edit,write,grep,find,ls,ws-report-to-lead,ws-agent-spawn,ws-agent-send,ws-agent-list,ws-agent-stop,ws-agent-transcript,explore,ws__playbook_render,ws__ferrule",
    );
  });

  test("full-worker with an empty ws tool list still includes explore and ws-report-to-lead (D-B: a worker can spawn explore and report)", () => {
    assert.equal(resolveTools("full-worker", []), "read,bash,edit,write,grep,find,ls,ws-report-to-lead,ws-agent-spawn,ws-agent-send,ws-agent-list,ws-agent-stop,ws-agent-transcript,explore");
  });

  test("full-worker offers persistent direct-child management; depth admission derives the terminal profile", () => {
    const resolved = resolveTools("full-worker", ["ws__playbook_render"]).split(",");
    for (const name of ["ws-agent-spawn", "ws-agent-send", "ws-agent-list", "ws-agent-stop", "ws-agent-transcript"]) assert.ok(resolved.includes(name));
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
  });

  test("an explicit \"full-worker\" is indistinguishable from omission (both resolve to full-worker) — the intended no-op case", () => {
    assert.equal(resolveSpawnToolGroup("full-worker"), resolveSpawnToolGroup(undefined));
  });
});

import {
  formatConcreteModelWarning,
  formatTierWarning,
  modelCatalogFromToolCtx,
  suggestModels,
  tierWarningNotifierFromToolCtx,
  validateConcreteModel,
} from "../src/model-catalog.ts";

const tierCatalog = ["cheap-model", "big-model", "reviewer-model"].map(id => ({ provider: "openrouter", id, hasAuth: true }));

describe("resolveModelForAliasViaWsMcp", () => {
  function textResult(text: string): McpToolCallResult {
    return { content: [{ type: "text", text }] };
  }

  /** Builds a duck-typed `McpStdioClient` stub whose `callTool` is the given fake. */
  function stubClient(callTool: McpStdioClient["callTool"]): McpStdioClient {
    return { callTool } as unknown as McpStdioClient;
  }

  test("no alias (model_name omitted) -> inherit unchanged, no call made", async () => {
    let called = false;
    const client = stubClient(async () => {
      called = true;
      return textResult("{}");
    });
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, undefined, "inherited/model"), { model: "inherited/model" });
    assert.equal(called, false, "no alias means no config.resolve_agent round-trip at all");
  });

  test("a genuine pi hit wins: resolved_from pi + a provider/id model", async () => {
    const client = stubClient(async (name, args) => {
      assert.equal(name, "config.resolve_agent");
      assert.deepEqual(args, { tier: "small", format: "json" });
      return textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/cheap-model", effort: "low" }));
    });
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), {
      model: "openrouter/cheap-model",
      effort: "low",
    });
  });

  test("a non-pi resolved_from still returns the inherit model, but now reports an unset rejection (the resolver never decides to refuse — callers do)", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "codex", model: "gpt-5.6-terra" })));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), {
      model: "inherited/model", rejected: { model: "gpt-5.6-terra", resolvedFrom: "codex", why: "unset" },
    });
  });

  test("source is 'tier' only on a genuine accepted pi hit; every other outcome (unset/unknown/no-auth/transport/parse/omitted) is 'inherit'", async () => {
    const hit = await resolveModelForAliasViaWsMcp(stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/cheap-model" }))), "small", "inherited/model", tierCatalog);
    assert.equal(hit.source, "tier");
    const unset = await resolveModelForAliasViaWsMcp(stubClient(async () => textResult(JSON.stringify({ resolved_from: "codex", model: "gpt-5.6-terra" }))), "small", "inherited/model", tierCatalog);
    assert.equal(unset.source, "inherit");
    const unknown = await resolveModelForAliasViaWsMcp(stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-terra" }))), "small", "inherited/model", []);
    assert.equal(unknown.source, "inherit");
    const transportFailure = await resolveModelForAliasViaWsMcp(stubClient(async () => { throw new Error("boom"); }), "small", "inherited/model", tierCatalog);
    assert.equal(transportFailure.source, "inherit");
    const omitted = await resolveModelForAliasViaWsMcp(stubClient(async () => textResult("{}")), undefined, "inherited/model");
    assert.equal(omitted.source, "inherit");
  });

  test("backend expansion: a slash-less model prefixes via the fixed codex/claude provider map before the catalog check", async () => {
    const codexClient = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-high", backend: "codex" })));
    const codexResult = await resolveModelForAliasViaWsMcp(codexClient, "small", "inherited/model", [{ provider: "openai-codex", id: "gpt-5.6-high", hasAuth: true }]);
    assert.equal(codexResult.model, "openai-codex/gpt-5.6-high");
    assert.equal(codexResult.rejected, undefined);
    const claudeClient = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "opus", backend: "claude" })));
    const claudeResult = await resolveModelForAliasViaWsMcp(claudeClient, "small", "inherited/model", [{ provider: "anthropic", id: "opus", hasAuth: true }]);
    assert.equal(claudeResult.model, "anthropic/opus");
    assert.equal(claudeResult.rejected, undefined);
  });

  test("backend expansion carries the tier's effort through on a genuine hit (the expansion path, not just the pre-slashed one)", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-luna", backend: "codex", effort: "high" })));
    const result = await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", [{ provider: "openai-codex", id: "gpt-5.6-luna", hasAuth: true }]);
    assert.equal(result.model, "openai-codex/gpt-5.6-luna");
    assert.equal(result.rejected, undefined);
    assert.equal(result.effort, "high");
    assert.equal(result.source, "tier");
  });

  test("backend expansion never re-prefixes an already-slashed model", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/cheap-model", backend: "codex" })));
    const result = await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog);
    assert.equal(result.model, "openrouter/cheap-model");
    assert.equal(result.rejected, undefined);
  });

  test("an empty or unmapped backend leaves a slash-less model as-is, so it fails the catalog check as unknown (not silently passed through)", async () => {
    for (const backend of [undefined, "", "pi", "some-unmapped-backend"]) {
      const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-terra", ...(backend === undefined ? {} : { backend }) })));
      const result = await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", []);
      assert.equal(result.rejected?.why, "unknown", `backend ${JSON.stringify(backend)}`);
      assert.equal(result.rejected?.model, "gpt-5.6-terra", `backend ${JSON.stringify(backend)}`);
      assert.equal(result.rejected?.stored, undefined, `backend ${JSON.stringify(backend)}: no expansion happened, so no stored raw value`);
    }
  });

  test("expanded no-auth: a slash-less backend-tagged model whose expanded provider/id IS in the catalog but has no auth", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-luna", backend: "codex" })));
    const result = await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", [{ provider: "openai-codex", id: "gpt-5.6-luna", hasAuth: false }]);
    assert.deepEqual(result.rejected, { model: "openai-codex/gpt-5.6-luna", stored: "gpt-5.6-luna", resolvedFrom: "pi", why: "no-auth" });
    assert.equal(
      formatTierWarning("small", result.rejected!, result.model, false),
      'warning: tier small is set to "openai-codex/gpt-5.6-luna" (configured as "gpt-5.6-luna") for harness pi, but provider openai-codex has no configured auth. Set it via config.tune(key: "agents.tier", harness: "pi", value: {tier: "small", model: "<provider/id>"}).',
    );
  });

  test("a rejected expansion carries the raw configured value as `stored`, distinct from the expanded/checked `model`, and suggestions run against the EXPANDED string", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-lunar", backend: "codex" })));
    const result = await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", [{ provider: "openai-codex", id: "gpt-5.6-luna", hasAuth: true }]);
    assert.deepEqual(result.rejected, {
      model: "openai-codex/gpt-5.6-lunar",
      stored: "gpt-5.6-lunar",
      resolvedFrom: "pi",
      why: "unknown",
      suggestions: ["openai-codex/gpt-5.6-luna"],
    });
  });

  test("a non-string backend field is a parse failure, like the other malformed fields", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-high", backend: 5 })));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), { model: "inherited/model" });
  });

  test("a pi-labeled bare id inherits with rejected detail", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "gpt-5.6-terra" })));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", []), {
      model: "inherited/model", rejected: { model: "gpt-5.6-terra", resolvedFrom: "pi", why: "unknown", suggestions: [] },
    });
  });

  test("an isError result inherits", async () => {
    const client = stubClient(async () => ({ content: [{ type: "text", text: "boom" }], isError: true }));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), { model: "inherited/model" });
  });

  test("no text content inherits", async () => {
    const client = stubClient(async () => ({ content: [] }));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), { model: "inherited/model" });
  });

  test("unparsable JSON text inherits", async () => {
    const client = stubClient(async () => textResult("not json"));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), { model: "inherited/model" });
  });

  test("a thrown call inherits (never-hard-fail)", async () => {
    const client = stubClient(async () => {
      throw new Error("stdio pipe broke");
    });
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, "small", "inherited/model", tierCatalog), { model: "inherited/model" });
  });

  test("no alias and no inherit model -> undefined model, no call made", async () => {
    const client = stubClient(async () => textResult("{}"));
    assert.deepEqual(await resolveModelForAliasViaWsMcp(client, undefined, undefined), { model: undefined });
  });

  test("effort is carried through only on a genuine pi hit, and omitted when the resolved effort is empty", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/big-model", effort: "" })));
    const resolved = await resolveModelForAliasViaWsMcp(client, "large", "inherited/model", tierCatalog);
    assert.equal(resolved.model, "openrouter/big-model");
    assert.equal(resolved.effort, undefined, "an empty resolved effort string must not surface as a truthy value");
  });

  test("effort is absent from a non-pi (inherit) resolution even if the payload carried one", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "codex", model: "gpt-5.6-terra", effort: "high" })));
    const resolved = await resolveModelForAliasViaWsMcp(client, "large", "inherited/model", tierCatalog);
    assert.deepEqual(resolved, { model: "inherited/model", rejected: { model: "gpt-5.6-terra", resolvedFrom: "codex", why: "unset" } });
  });

  test("an arbitrary tier name still resolves normally (no closed vocabulary enforced by this function itself)", async () => {
    const client = stubClient(async () => textResult(JSON.stringify({ resolved_from: "pi", model: "openrouter/reviewer-model" })));
    const resolved = await resolveModelForAliasViaWsMcp(client, "reviewer", "inherited/model", tierCatalog);
    assert.equal(resolved.model, "openrouter/reviewer-model");
    assert.equal(resolved.effort, undefined);
  });
});

describe("concrete Pi model validation", () => {
  test("accepts exact authenticated provider/id membership without consulting tier config", () => {
    assert.deepEqual(validateConcreteModel("openrouter/cheap-model", tierCatalog), { model: "openrouter/cheap-model" });
  });

  test("rejects unknown and unauthenticated concrete models with catalog-specific diagnostics", () => {
    const unknown = validateConcreteModel("openrouter/cheap-modl", tierCatalog);
    assert.deepEqual(unknown, {
      rejected: { model: "openrouter/cheap-modl", why: "unknown", suggestions: ["openrouter/cheap-model"] },
    });
    assert.match(formatConcreteModelWarning(unknown.rejected!, false), /Did you mean openrouter\/cheap-model\?/);

    const noAuth = validateConcreteModel("openrouter/cheap-model", [{ provider: "openrouter", id: "cheap-model", hasAuth: false }]);
    assert.deepEqual(noAuth, { rejected: { model: "openrouter/cheap-model", why: "no-auth" } });
    assert.match(formatConcreteModelWarning(noAuth.rejected!, false), /provider openrouter has no configured auth/);
  });
});

import { armForkRoleWiring, registerFork } from "../src/fork.ts";
import { registerExecuteGateway } from "../src/execute-gateway.ts";
import { ensureRespondent, createThreadRegistryHandle } from "../src/ask.ts";

// Offline registered-wrapper coverage: only these test-process RPC methods are stubbed.
// No installed Pi source is edited and no child/provider is started.
describe("catalog validation and warning copy", () => {
  const catalog = [
    { provider: "openai-codex", id: "gpt-5.6-luna", hasAuth: true },
    { provider: "locked", id: "gpt-5.6-luna", hasAuth: false },
    { provider: "router", id: "org/nested:high", hasAuth: true },
  ];
  const resolve = (model: string, resolved_from = "pi", entries = catalog) => resolveModelForAliasViaWsMcp({
    callTool: async () => ({ content: [{ type: "text", text: JSON.stringify({ model, resolved_from, effort: "low" }) }] }),
  }, "small", "lead/model", entries);

  for (const value of ["gpt-5.6-luna", "openai-codex/gpt-5.6-lun", "unknown/gpt-5.6-luna", "openai-codex/gpt-5.6-luna:high", "", "far-away-model-name"]) {
    test(`unknown ${JSON.stringify(value)} inherits without effort`, async () => {
      const result = await resolve(value);
      assert.equal(result.model, "lead/model");
      assert.equal(result.effort, undefined);
      assert.deepEqual(result.rejected, { model: value, resolvedFrom: "pi", why: "unknown", suggestions: suggestModels(value, catalog) });
      assert.equal(effectiveModelEffort("high", result.effort), "high");
    });
  }
  test("exact membership preserves nested ids and literal colons", async () => {
    assert.deepEqual(await resolve("router/org/nested:high"), { model: "router/org/nested:high", effort: "low" });
    assert.deepEqual(await resolve("openai-codex/gpt-5.6-luna"), { model: "openai-codex/gpt-5.6-luna", effort: "low" });
  });
  for (const source of ["codex", "default", "tiers", "claude", ""]) {
    test(`${source || "empty"} resolved_from reports an unset rejection while still returning the inherit model`, async () =>
      assert.deepEqual(await resolve("gpt-5.6-luna", source), { model: "lead/model", rejected: { model: "gpt-5.6-luna", resolvedFrom: source, why: "unset" } }));
  }
  test("no-auth names provider and has no suggestion tail", async () => {
    const result = await resolve("locked/gpt-5.6-luna");
    assert.deepEqual(result, { model: "lead/model", rejected: { model: "locked/gpt-5.6-luna", resolvedFrom: "pi", why: "no-auth" } });
    assert.equal(
      formatTierWarning("small", result.rejected!, result.model, false),
      'warning: tier small is set to "locked/gpt-5.6-luna" for harness pi, but provider locked has no configured auth. Set it via config.tune(key: "agents.tier", harness: "pi", value: {tier: "small", model: "<provider/id>"}).',
    );
  });
  test("exact unknown copy and all three tails", async () => {
    const result = await resolve("gpt-5.6-luna");
    assert.equal(
      formatTierWarning("small", result.rejected!, result.model, false),
      'warning: tier small is set to "gpt-5.6-luna" for harness pi, which is not a provider/id entry in Pi\'s model catalog. Did you mean openai-codex/gpt-5.6-luna, locked/gpt-5.6-luna? Set it via config.tune(key: "agents.tier", harness: "pi", value: {tier: "small", model: "<provider/id>"}).',
    );
    const far = await resolve("far-away-model-name");
    assert.match(formatTierWarning("small", far.rejected!, far.model, false), / No close match in Pi's model catalog\. Set it via config\.tune\(/);
    const empty = await resolve("", "pi", []);
    assert.match(formatTierWarning("small", empty.rejected!, empty.model, true), / Pi's model catalog is empty\. Set it via config\.tune\(/);
  });
  test("one-line escaping preserves raw detail", async () => {
    const raw = 'bad"\n\r\t\x1b\u0085\u2028\u2029/value';
    const result = await resolve(raw);
    assert.equal(result.rejected!.model, raw);
    const warning = formatTierWarning("small", result.rejected!, result.model, false);
    assert.doesNotMatch(warning, /[\x00-\x1f\x7f-\x9f\u2028\u2029]/);
    assert.ok(warning.includes('bad\\"\\n\\r\\t\\u001b'));
  });
  test("suggestions rank stably, deduplicate, cap three, use id after first slash and distance two", () => {
    const entries = ["lun", "luna-plus", "luna", "luna", "lune", "luna-max"].map(id => ({ provider: "p", id, hasAuth: false }));
    assert.deepEqual(suggestModels("typo/luna", entries), ["p/luna", "p/lun", "p/luna-plus"]);
    assert.deepEqual(suggestModels("org/nested:high", catalog), ["router/org/nested:high"]); // containment after first separator
    assert.deepEqual(suggestModels("typo/org/nested:high", catalog), ["router/org/nested:high"]);
    assert.deepEqual(suggestModels("abXYef", [{ provider: "p", id: "abcdef", hasAuth: false }]), ["p/abcdef"]);
    assert.deepEqual(suggestModels("aXYZef", [{ provider: "p", id: "abcdef", hasAuth: true }]), []);
    assert.deepEqual(suggestModels("", entries), []);
    assert.deepEqual(suggestModels("LUNA", entries), ["p/lun", "p/luna-plus", "p/luna"]);
  });
  test("live getAll/auth source ignores available/scoped lists and preserves receivers", async () => {
    let auth = false;
    let models = [{ provider: "live", id: "model" }];
    const registry = {
      getAll() { assert.equal(this, registry); return models; },
      hasConfiguredAuth(model: unknown) { assert.equal(this, registry); assert.equal(model, models[0]); return auth; },
      getAvailable() { assert.fail("cached availability is not a validation catalog"); },
    };
    const ctx = { modelRegistry: registry, get scopedModels() { assert.fail("scoped models are not validation"); } };
    assert.equal((await resolve("live/model", "pi", modelCatalogFromToolCtx(ctx))).rejected?.why, "no-auth");
    auth = true;
    assert.equal((await resolve("live/model", "pi", modelCatalogFromToolCtx(ctx))).rejected, undefined);
    models = [];
    assert.equal((await resolve("live/model", "pi", modelCatalogFromToolCtx(ctx))).rejected?.why, "unknown");
    const notices: unknown[] = [];
    const ui = { notify(message: string, level: string) { assert.equal(this, ui); notices.push([message, level]); } };
    tierWarningNotifierFromToolCtx({ hasUI: true, ui })!("warning");
    assert.deepEqual(notices, [["warning See /ws-model-catalog-list for the models usable here.", "warning"]]);
    assert.equal(tierWarningNotifierFromToolCtx({ hasUI: false, ui }), undefined);
  });
});

/**
 * `ws-agent-spawn` tool-level coverage of the drain-bug fix itself: an
 * ordinary (non-explore) spawn given a NAMED tier that `resolveModelForAliasViaWsMcp`
 * comes back `rejected` on now throws before any side effect, instead of the
 * old warn-and-inherit behavior. Drives the real registered tool wrapper
 * (`registerAgentTools`) with only `RpcClient`'s process transport replaced
 * (same technique as `test/persistent-explore.test.ts`'s `installRpcHarness`
 * — no real subprocess, no provider credentials needed) so `spawnAgent`'s
 * actual guard ordering (resolve -> refuse-or-continue -> alias/cap guards ->
 * `mkdtempSync` -> registry.set) runs for real, not just its pure resolver.
 */
describe("spawnAgent (ws-agent-spawn tool level): ordinary rejection refuses instead of inheriting", () => {
  interface CapturedTool {
    name: string;
    description: string;
    parameters: { properties: Record<string, { description?: string }> };
    execute: (id: string, params: unknown, signal?: AbortSignal, update?: unknown, ctx?: unknown) => Promise<{ content: Array<{ text: string }> }>;
  }

  function installRpcHarness(onThinking?: (level: string) => void) {
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      start: async () => {}, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {},
      setThinkingLevel: async (level: string) => { onThinking?.(level); },
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => Object.assign(RpcClient.prototype, original) };
  }

  function jsonResult(payload: unknown): McpToolCallResult {
    return { content: [{ type: "text", text: JSON.stringify(payload) }] };
  }

  function harness(
    callTool: McpStdioClient["callTool"],
    catalog = [{ provider: "openai-codex", id: "gpt-5.6-high", hasAuth: true }],
  ) {
    const tools = new Map<string, CapturedTool>();
    const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
    const bridge = { client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" } } as never;
    const handle = registerAgentTools(pi, bridge, { cwd: "/tmp" });
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
    return { tool: tools.get("ws-agent-spawn")!, handle, ctx };
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

  test("tool help advertises tier-or-concrete selection and the default effort sentinel", () => {
    const { tool } = harness(async () => jsonResult({}));
    assert.match(tool.description, /tier alias or concrete Pi model ID/);
    assert.match(tool.parameters.properties.model_name!.description!, /concrete Pi model ID/);
    assert.match(tool.parameters.properties.model_effort!.description!, /default/);
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

  test("a real owned-home removal failure is diagnostic-only and the cap-triggering spawn still succeeds", async (t) => {
    const rpc = installRpcHarness();
    const previousCap = process.env[WS_PI_AGENT_REGISTRY_CAP_ENV];
    const diagnostics = t.mock.method(console, "error", () => {});
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

      const parsed = JSON.parse((await tool.execute("call", { system_prompt_path: "/tmp/p.md", prompt: "replacement" }, undefined, undefined, ctx)).content[0]!.text);
      assert.ok(parsed.agent_id);
      assert.equal(handle.rpcRegistry.has("blocked-delete"), false, "eligible record is evicted despite best-effort disk failure");
      assert.equal(handle.rpcRegistry.has(parsed.agent_id), true, "the replacement spawn is admitted");
      assert.equal(existsSync(ownership.home), true, "failed removal was rolled back for retry");
      assert.ok(diagnostics.mock.callCount() >= 1);
      if (existsSync(locked)) chmodSync(locked, 0o700);
      await handle.stopAll();
    } finally {
      if (locked && existsSync(locked)) chmodSync(locked, 0o700);
      if (previousCap === undefined) delete process.env[WS_PI_AGENT_REGISTRY_CAP_ENV]; else process.env[WS_PI_AGENT_REGISTRY_CAP_ENV] = previousCap;
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
      start: async () => {}, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => Object.assign(RpcClient.prototype, original) };
  }

  function jsonResult(payload: unknown): McpToolCallResult {
    return { content: [{ type: "text", text: JSON.stringify(payload) }] };
  }

  function harness(callTool: McpStdioClient["callTool"]) {
    const tools = new Map<string, CapturedTool>();
    const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
    const bridge = { client: { callTool }, wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" } } as never;
    const handle = registerAgentTools(pi, bridge, { cwd: "/tmp" });
    const ctx = { sessionManager: { getSessionId: () => "test-lead" }, agentStorageRoot: storageRoot(), model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "openai-codex", id: "gpt-5.6-high" }], hasConfiguredAuth: () => true } };
    return { tool: tools.get("ws-agent-spawn")!, sendTool: tools.get("ws-agent-send")!, handle, ctx };
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

/**
 * The persistent `explore` tool publishes model resolution through
 * `spawnAgent` identically for lead and worker dispatchers.
 */
describe("explore tool: onModelResolved / resolved-line publishing (260906 Phase 2)", () => {
  interface CapturedTool {
    name: string;
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
      start: startRpcWithWebProof, stop: async () => {}, abort: async () => {},
      onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/ws-pi-agent-test/session.jsonl" }),
    });
    return { restore: () => Object.assign(RpcClient.prototype, original) };
  }

  test("lead code-search Explore publishes the resolved line through spawnAgent and repeats it in the final details", async () => {
    const rpc = installRpcHarness();
    try {
      const tools = new Map<string, CapturedTool>();
      const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool), sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
      const bridge = {
        client: { callTool: async (name: string) => { assert.equal(name, "config.resolve_agent"); return { content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "pi/small" }) }] }; } },
        wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" },
      } as never;
      const handle = registerAgentTools(pi, bridge, { cwd: "/tmp" });
      // Matches installRpcHarness's fixed `getState()` model — spawnRole
      // "explore"'s post-start `verifyResearchSelection` compares against it.
      const ctx = { sessionManager: { getSessionId: () => "test-lead" }, agentStorageRoot: storageRoot(), model: { provider: "lead", id: "large" }, thinkingLevel: "high", modelRegistry: { getAll: () => [{ provider: "pi", id: "small" }], hasConfiguredAuth: () => true } };
      const tool = tools.get("explore")!;
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { query: "why does this fail" }, undefined, (partial) => updates.push(partial), ctx);
      const expected = { tier: "small", model: "pi/small", effort: undefined, inherited: false };
      assert.equal(updates.length, 1);
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, expected);
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, expected);
      await handle.stopAll();
    } finally { rpc.restore(); }
  });

  test("worker persistent explore publishes the same resolved line as lead exploration", async () => {
    const rpc = installRpcHarness();
    const previousRole = process.env[WS_PI_SPAWN_ROLE_ENV];
    const previousPolicy = process.env[DELEGATION_ENV];
    process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
    process.env[DELEGATION_ENV] = JSON.stringify({ version: 1, depth: 1, maxDepth: 2, authority: "lead", tools: resolveTools("full-worker").split(","), network: { search: true, fetch: true } });
    try {
      const tools = new Map<string, CapturedTool>();
      const pi = { registerTool: (tool: CapturedTool) => tools.set(tool.name, tool) } as unknown as ExtensionAPI;
      const bridge = {
        client: { callTool: async () => ({ content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "provider/id", effort: "high" }) }] }) },
        wsToolNames: [], defaultSessionKeyRef: { current: "lead-key" },
      } as never;
      const handle = registerAgentTools(pi, bridge, { cwd: "/tmp" });
      const ctx = { sessionManager: { getSessionId: () => "test-worker" }, agentStorageRoot: storageRoot(), modelRegistry: { getAll: () => [{ provider: "provider", id: "id" }], hasConfiguredAuth: () => true } };
      RpcClient.prototype.getState = async () => ({ model: { provider: "provider", id: "id" }, thinkingLevel: "high" }) as any;
      const tool = tools.get("explore")!;
      const updates: Array<{ content: unknown[]; details?: unknown }> = [];
      const raw = await tool.execute("call", { query: "why does this fail" }, undefined, (partial) => updates.push(partial), ctx);
      const expected = { tier: "small", model: "provider/id", effort: "high", inherited: false };
      assert.equal(updates.length, 1);
      assert.deepEqual((updates[0]!.details as { resolved: unknown }).resolved, expected);
      assert.deepEqual((raw.details as { resolved?: unknown } | undefined)?.resolved, expected);
      assert.equal(handle.rpcRegistry.size, 1);
      await handle.stopAll();
    } finally {
      rpc.restore();
      if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
      if (previousRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = previousRole;
    }
  });

  test("a long synchronous explore query is head-truncated in the registered tool's own call-row summary", async () => {
    const { createDispatchToolPreview } = await import("../src/tool-row-render.ts");
    const { createToolPreviewTuiRef } = await import("../src/tool-result-render.ts");
    const { buildExploreSummary } = await import("../src/tool-row-render.ts");
    const ref = createToolPreviewTuiRef();
    ref.current = {
      Text: class { text = ""; setText(t: string) { this.text = t; } render() { return this.text.split("\n"); } invalidate() {} },
      Box: class { children: Array<{ render(width: number): string[] }> = []; addChild(c: { render(width: number): string[] }) { this.children.push(c); } setBgFn() {} render(width: number) { return this.children.flatMap((c) => c.render(width)); } invalidate() {} },
      stripTerminalSequences: (t: string) => t,
      truncateToWidth: (t: string) => t,
    } as never;
    const preview = createDispatchToolPreview(ref, "explore", buildExploreSummary);
    const longQuery = "why does this fail ".repeat(20);
    const call = preview.renderCall({ query: longQuery }, { fg: (_: string, x: string) => x, bold: (x: string) => x }, { state: {}, argsComplete: true, isPartial: false, lastComponent: undefined });
    const rendered = (call as { render(width: number): string[] }).render(200).join("\n");
    assert.ok(rendered.includes("…"), "the long query is head-truncated with the ellipsis marker");
    assert.ok(!rendered.includes(longQuery.trim()), "the untruncated full query never appears");
  });
});

describe("effectiveModelEffort (review relay #1, Critical: the modelEffort merge rule)", () => {
  test("an explicit, non-empty caller effort wins over a resolved one", () => {
    assert.equal(effectiveModelEffort("high", "low"), "high");
  });

  test("no caller effort falls back to the resolved effort", () => {
    assert.equal(effectiveModelEffort(undefined, "low"), "low");
  });

  test("an empty-string caller effort is treated as absent, not an explicit win (fixes the `??` vs `||` Minor)", () => {
    assert.equal(effectiveModelEffort("", "low"), "low");
  });

  test("neither side set -> undefined", () => {
    assert.equal(effectiveModelEffort(undefined, undefined), undefined);
  });

  test("caller set, nothing resolved -> the caller value", () => {
    assert.equal(effectiveModelEffort("medium", undefined), "medium");
  });

  test("default preserves each selection source's own effort policy", () => {
    assert.equal(effectiveModelEffort("default", "low", "tier", "high"), "low", "tier keeps configured effort");
    assert.equal(effectiveModelEffort("default", undefined, "inherit", "high"), "high", "inherit keeps parent effort");
    assert.equal(effectiveModelEffort("default", undefined, "concrete", "high"), undefined, "concrete keeps the selected model default");
  });

  test("omitting effort retains the legacy behavior instead of acting as the explicit default sentinel", () => {
    assert.equal(effectiveModelEffort(undefined, undefined, "inherit", "high"), undefined);
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
 * A record in the LIVE resting state. 260905 (alias/park/cap ticket):
 * presence no longer depends on `client` — any non-threadBound registry
 * member (dormant/parked included) keeps `computeRunningStatusLine`'s line
 * present — but most fan-in fixtures below still want a live client to
 * exercise `running`/`streaming` transitions. Never a real `RpcClient`.
 */
function liveRpcRecord(overrides: Partial<RpcAgentRecord> = {}): RpcAgentRecord {
  return freshRpcRecord({ client: {} as RpcClient, ...overrides });
}

/**
 * Duck-typed `ExtensionAPI` stand-in exposing only the push methods. `sent`
 * is the item-level delivery view used by the lifecycle assertions below:
 * a `ws-push-batch` is flattened back to its structured `details.items` while
 * dedicated push-wake tests assert the real one-envelope transport.
 */
function fakePi(overrides: { sendMessage?: (message: unknown, options?: unknown) => void } = {}): {
  api: Parameters<typeof pushToLead>[0];
  wakes: unknown[];
  sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }>;
} {
  const sent: Array<{ message: { customType?: string; content?: string; display?: boolean; details?: unknown }; options?: { deliverAs?: string; triggerTurn?: boolean } }> = [];
  const wakes: unknown[] = [];
  leadIdleRef.current ??= () => true;
  const handlers = new Map<string, () => void>();
  const api = {
    on: (event: string, handler: () => void) => handlers.set(event, handler),
    sendUserMessage: (content: unknown) => {
      wakes.push(content);
      const accessor = leadIdleRef.current;
      leadIdleRef.current = () => false;
      handlers.get("agent_start")?.();
      leadIdleRef.current = accessor;
    },
    sendMessage:
      overrides.sendMessage ??
      ((message: unknown, options?: unknown) => {
        const candidate = message as { customType?: string; details?: { items?: unknown[] } };
        if (candidate.customType === PUSH_BATCH_CUSTOM_TYPE && Array.isArray(candidate.details?.items)) {
          for (const item of candidate.details.items) sent.push({ message: item as never, options: options as never });
        } else sent.push({ message: message as never, options: options as never });
      }),
  };
  registerPushFlush(api as never, { delayMs: () => 10 });
  return { api: api as unknown as Parameters<typeof pushToLead>[0], sent, wakes };
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

  test("onQuestionReport returning a string PUSHES the registration notice as ws-agent-advisory — the lead is told a thread exists, not asked to answer (§1)", () => {
    const seen: string[] = [];
    const record = freshRpcRecord({
      onQuestionReport: (_rec, message) => {
        seen.push(message);
        return "[ws] registered as thread q1";
      },
    });
    const result = applyRpcEvent(record, { type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "which branch?", kind: "question" } });
    assert.deepEqual(seen, ["which branch?"]);
    assert.deepEqual(
      result,
      { push: { family: "ws-agent-advisory", payload: { advisory: "fork-question-thread", detail: "[ws] registered as thread q1" }, deliverAs: "followUp" } },
      "the lead is not part of a fork-raised question exchange, but must still see the notice",
    );
    assert.equal(record.reportLog.length, 1, "the report is still logged — the push shape change doesn't affect bookkeeping");
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
  test("lead, fork, worker, and persistent Explore owners push to their direct parent", () => {
    assert.equal(shouldPushToLead({}), true, "no marker = host lead");
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "fork" }), true);
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "worker" }), true);
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "explore" }), true);
  });

  test("an unrecognized role value is treated as no marker (host lead), same as readSpawnRole", () => {
    assert.equal(shouldPushToLead({ [WS_PI_SPAWN_ROLE_ENV]: "bogus" }), true);
  });
});

describe("computeRunningStatusLine (fan-in running count)", () => {
  test("Edition: an empty/absent registry produces NO line at all — a push with nothing delegated once carried a contentless zero line", () => {
    assert.equal(computeRunningStatusLine(new Map()), undefined);
    assert.equal(computeRunningStatusLine(undefined), undefined);
  });

  test("260905 (alias/park/cap): a registry whose every record is dormant/stopped now KEEPS the line, at zero — presence keys on registry membership, not on a live client", () => {
    const registry: RpcAgentRegistry = new Map([["gone", freshRpcRecord({ agentId: "gone" })]]);
    assert.equal(computeRunningStatusLine(registry), "0 delegated agents still running");
  });

  test("260905 (alias/park/cap): a registry whose only member is threadBound still produces no line", () => {
    const registry: RpcAgentRegistry = new Map([["t", freshRpcRecord({ agentId: "t", threadBound: true })]]);
    assert.equal(computeRunningStatusLine(registry), undefined);
  });

  test("counts the live, non-threadBound records with no terminal report this turn — no total, no ids", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true })],
      ["b", liveRpcRecord({ agentId: "b", running: true, terminalThisTurn: true })],
      ["c", liveRpcRecord({ agentId: "c", running: true })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "2 delegated agents still running");
  });

  test("Edition: live records that are all settled/reported keep the line present at zero — the synthesis cue", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true, terminalThisTurn: true })],
      ["b", liveRpcRecord({ agentId: "b" })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "0 delegated agents still running");
  });

  test("I3: the count descends across a real 3-way fan-out — 2, 1, 0 — with the line present throughout", () => {
    // The event order the runtime actually produces: each worker files its
    // final (terminalThisTurn) and then settles (running cleared) BEFORE the
    // next one reports. Presence is keyed on `client`, not `running`, so the
    // settled siblings keep the line present and the zero cue is reachable.
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
      "2 delegated agents still running",
      "1 delegated agent still running",
      "0 delegated agents still running",
    ]);
  });

  test("I3: a live child that settled idle keeps the line present and only leaves the count", () => {
    const idle = liveRpcRecord({ agentId: "idle", running: true });
    const registry: RpcAgentRegistry = new Map([
      ["busy", liveRpcRecord({ agentId: "busy", running: true })],
      ["idle", idle],
    ]);
    applyRpcEvent(idle, { type: "agent_settled" });
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
  });

  test("stopped/exited/dormant records (no client) neither count nor keep the line present", () => {
    const registry: RpcAgentRegistry = new Map([
      ["live", liveRpcRecord({ agentId: "live", running: true })],
      ["stopped", freshRpcRecord({ agentId: "stopped", running: false })],
      ["dormant", freshRpcRecord({ agentId: "dormant" })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
  });

  test("a threadBound agent is excluded from BOTH the count and the presence check — the owner exchange is not the lead's fan-in", () => {
    const registry: RpcAgentRegistry = new Map([
      ["worker", liveRpcRecord({ agentId: "worker", running: true })],
      ["discussing", liveRpcRecord({ agentId: "discussing", running: true, threadBound: true })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
    assert.equal(
      computeRunningStatusLine(new Map([["discussing", liveRpcRecord({ agentId: "discussing", running: true, threadBound: true })]])),
      undefined,
      "a registry whose only live record is threadBound produces no line at all",
    );
  });

  test("an approval-blocked child is still counted — it is outstanding, and the lead is what unblocks it", () => {
    const registry: RpcAgentRegistry = new Map([
      ["a", liveRpcRecord({ agentId: "a", running: true, pendingApproval: { cmdId: "c1", command: "echo hi" } })],
    ]);
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
  });

  test("a child counts from the prompt-issue instant, before any agent_start event has arrived", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", client });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    await promptAgent(record, client, "go");
    assert.equal(record.streaming, false, "no agent_start has been observed yet");
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
  });

  test("singular/plural agreement follows the running count", () => {
    assert.equal(computeRunningStatusLine(new Map([["a", liveRpcRecord({ agentId: "a", running: true })]])), "1 delegated agent still running");
    assert.equal(computeRunningStatusLine(new Map([["a", liveRpcRecord({ agentId: "a" })]])), "0 delegated agents still running");
  });
});

describe("hasRunningAgents (goal-loop yield predicate)", () => {
  test("an empty/absent registry is false — nothing known to be running", () => {
    assert.equal(hasRunningAgents(new Map()), false);
    assert.equal(hasRunningAgents(undefined), false);
  });

  test("one running child is true", () => {
    const registry: RpcAgentRegistry = new Map([["a", liveRpcRecord({ agentId: "a", running: true })]]);
    assert.equal(hasRunningAgents(registry), true);
  });

  test("a persistent Explore record counts exactly like any other non-threadBound running record", () => {
    const registry: RpcAgentRegistry = new Map([
      ["e", liveRpcRecord({ agentId: "e", running: true, spawnRole: "explore" })],
    ]);
    assert.equal(hasRunningAgents(registry), true);
    assert.equal(computeRunningStatusLine(registry), "1 delegated agent still running");
  });

  test("a dormant record (never spawned live, no client) is false", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.equal(hasRunningAgents(registry), false);
  });

  test("an idle record (live client, settled: running/streaming false, per listAgents's own 'idle' vocabulary) is false", () => {
    const registry: RpcAgentRegistry = new Map([["a", liveRpcRecord({ agentId: "a", running: false, streaming: false })]]);
    assert.equal(hasRunningAgents(registry), false);
  });

  test("a stopped/exited record (previously live, client cleared by clearLiveState) is false", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a", running: false })]]);
    assert.equal(hasRunningAgents(registry), false);
  });

  test("a running child that already filed its final/question this turn (terminalThisTurn) is false", () => {
    const registry: RpcAgentRegistry = new Map([["a", liveRpcRecord({ agentId: "a", running: true, terminalThisTurn: true })]]);
    assert.equal(hasRunningAgents(registry), false);
  });

  test("a threadBound respondent ALONE is false — the owner exchange is not the lead's fan-in", () => {
    const registry: RpcAgentRegistry = new Map([["discussing", liveRpcRecord({ agentId: "discussing", running: true, threadBound: true })]]);
    assert.equal(hasRunningAgents(registry), false);
  });

  test("a threadBound running record plus an ordinary running record is true — the threadBound one is excluded but the other still counts", () => {
    const registry: RpcAgentRegistry = new Map([
      ["discussing", liveRpcRecord({ agentId: "discussing", running: true, threadBound: true })],
      ["worker", liveRpcRecord({ agentId: "worker", running: true })],
    ]);
    assert.equal(hasRunningAgents(registry), true);
  });

  test("Phase 2 ticket case: a child found dead by the probe while yielding ends the yield through its exited push — probeAgentLiveness's getState() rejection clears hasRunningAgents (mirrors computeRunningStatusLine's I3 fan-out chaining through the same computeFanIn walk)", async () => {
    const client = {
      getState: async () => {
        throw new Error("process exited");
      },
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "a", running: true, client });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.equal(hasRunningAgents(registry), true, "yielding while the child is believed alive and running");

    const alive = await probeAgentLiveness(undefined, registry, record);

    assert.equal(alive, false, "the probe detected the dead child");
    assert.equal(hasRunningAgents(registry), false, "the probe's exited transition (clearLiveState) ends the yield — the next settle re-fires normally");
  });

  test("Phase 2 ticket case (registry-state half only): a pushed ws-agent-settled/ws-agent-report's normal completion also clears hasRunningAgents — the offline-expressible part of 'starts the turn that continues the goal'", () => {
    // NOT FIXED (offline) for the full ticket bullet: Pi actually scheduling a
    // new turn from pushToLead's `triggerTurn: true` is live-only — nothing in
    // this suite mocks Pi's own turn scheduler. What IS offline-expressible,
    // and asserted here, is the registry-state transition the bullet depends
    // on: once the running child's normal completion clears `running` (the
    // same `applyRpcEvent("agent_settled")` transition `computeRunningStatusLine`'s
    // own I3 fan-out test chains through), the lead's NEXT `agent_settled`
    // sees `hasRunningAgents(registry) === false` and re-fires normally
    // instead of yielding again.
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    assert.equal(hasRunningAgents(registry), true, "yielding while the child is still running");

    applyRpcEvent(record, { type: "agent_settled" });

    assert.equal(hasRunningAgents(registry), false, "the child's own settle clears the fan-in — the lead's next settle re-fires normally");
  });
});

describe("buildPushContent", () => {
  test("renders a head line naming the family and agent, one key: value line per payload field, and the status line last", () => {
    const content = buildPushContent("ws-agent-report", "a1", { kind: "final", report: "done" }, "0 delegated agents still running");
    assert.equal(content, ["[ws-agent-report] agent a1", "kind: final", "report: done", "0 delegated agents still running"].join("\n"));
  });

  test("an agent-less family (ws-agent-orphaned) drops the agent from the head line", () => {
    const content = buildPushContent("ws-agent-orphaned", undefined, { count: 2 }, "0 delegated agents still running");
    assert.equal(content, ["[ws-agent-orphaned]", "count: 2", "0 delegated agents still running"].join("\n"));
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
  // 260906 Phase 1 (settle-timer reminder race ticket): module state shared
  // with goal-loop.ts's settle timer, mirroring the existing
  // `leadCompactingRef`/`heldPushQueue` reset convention elsewhere in this
  // file.
  beforeEach(() => {
    leadWakeStartPendingRef.current = false;
    heldPushQueue.length = 0;
  });

  afterEach(() => {
    leadWakeStartPendingRef.current = false;
    heldPushQueue.length = 0;
  });

  test("confirmed-start release retains each structured item with agent_id, payload, and status", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 1);
    const [{ message, options }] = pi.sent;
    assert.equal(message.customType, "ws-agent-report");
    assert.equal(message.display, true);
    assert.deepEqual(message.details, { agent_id: "a", report: "halfway", status: "1 delegated agent still running" });
    assert.deepEqual(options, { deliverAs: "steer", triggerTurn: true }, "confirmed start steers the held idle report before its first response");
  });

  test("an absent record still pushes (the orphan roll-call), and with nothing delegated it carries NO status field", () => {
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 2 }, "followUp");
    assert.deepEqual(pi.sent[0].message.details, { count: 2 }, "Edition: a zero line with nothing delegated told the lead nothing");
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
    assert.equal(heldPushQueue.length, 1, "synchronous batch rejection retains the item for fallback delivery");
  });

  test("a worker process receives child reports through the real push path", () => {
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
    assert.equal(pi.sent.length, 1);
    assert.equal(pi.sent[0].message.details?.report, "halfway");
  });

  test("the same call from the host lead (no role marker) does push — the gate is the role, not the arguments", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");
    assert.equal(pi.sent.length, 1);
  });

  test("260905 (alias/park/cap): the pushed message HEAD prints the alias, followed by the uuid — details.agent_id stays the bare uuid", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a1", alias: "scout", running: true });
    const registry: RpcAgentRegistry = new Map([["a1", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent[0].message.content?.split("\n")[0], "[ws-agent-report] agent scout (a1)");
    assert.deepEqual(pi.sent[0].message.details, { agent_id: "a1", report: "halfway", status: "1 delegated agent still running" });
  });

  test("260905 (alias/park/cap): with no alias, the head is unchanged (bare uuid)", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a1", running: true });
    const registry: RpcAgentRegistry = new Map([["a1", record]]);

    pushToLead(pi.api, registry, record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent[0].message.content?.split("\n")[0], "[ws-agent-report] agent a1");
  });

  test("Phase 2: a pending reminder holds pushes until confirmed start", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    leadWakeStartPendingRef.current = true;

    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 1, "held until confirmed streaming start");
    leadWakeStartPendingRef.current = false;
    flushHeldPushes(pi.api, true);
    assert.deepEqual(pi.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
  });

  test("a terminal delivery becomes held only on FIFO admission and enqueued only on actual release", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "terminal", running: true });
    const terminal: TerminalDelivery = {};
    leadWakeStartPendingRef.current = true;

    pushToLead(pi.api, new Map([[record.agentId, record]]), record, "ws-agent-report", { report: "done" }, "followUp", terminal);
    assert.equal(terminal.state, "held");
    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 1);

    leadWakeStartPendingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(terminal.state, "enqueued");
    assert.equal(pi.sent.length, 1);
    assert.equal(flushHeldPushes(pi.api, true), 0, "the retained event is not admitted a second time");
  });

  test("skipped or rejected terminal sends never claim FIFO admission", () => {
    const skipped: TerminalDelivery = {};
    const pi = fakePi();
    const saved = leadIdleRef.current;
    leadIdleRef.current = undefined;
    try {
      pushToLead(pi.api, new Map(), undefined, "ws-agent-advisory", { advisory: "missing-final" }, "followUp", skipped);
    } finally {
      leadIdleRef.current = saved;
    }
    assert.equal(skipped.state, undefined);

    const rejected: TerminalDelivery = {};
    const broken = fakePi({ sendMessage: () => { throw new Error("gone"); } });
    const beforeRejected = leadIdleRef.current;
    leadIdleRef.current = () => false;
    try {
      pushToLead(broken.api, new Map(), undefined, "ws-agent-advisory", { advisory: "missing-final" }, "steer", rejected);
    } finally {
      leadIdleRef.current = beforeRejected;
    }
    assert.equal(rejected.state, undefined);
  });

  test("confirmed-start release retains triggerTurn: true when no reminder start is pending", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    leadWakeStartPendingRef.current = false;

    pushToLead(pi.api, new Map([["a", record]]), record, "ws-agent-report", { report: "halfway" }, "followUp");

    assert.equal(pi.sent.length, 1);
    assert.deepEqual(pi.sent[0]!.options, { deliverAs: "steer", triggerTurn: true });
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
 * finished while the lead was still spawning its siblings delivered a zero
 * count (its siblings were not registered yet), and so did the next: three
 * separate invitations to synthesize before the fan-out was in.
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
    leadCompactingRef.current = false;
  });

  afterEach(() => {
    heldPushQueue.length = 0;
    leadIdleRef.current = undefined;
    leadCompactingRef.current = false;
  });

  test("an IDLE lead receives its push after the fake user wake confirms start", () => {
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

    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, [], "the queue is drained, so a second settle re-sends nothing");
  });

  test("no idleness accessor means no send into an uninitialized or torn-down session", () => {
    const pi = fakePi();
    leadIdleRef.current = undefined;
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 0, "uninitialized or torn-down sessions never start custom runs");
  });

  test("a throwing idleness accessor is a torn-down session: no send or hold", () => {
    leadIdleRef.current = () => {
      throw new Error("ctx is gone");
    };
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-orphaned", { count: 1 }, "followUp");
    assert.equal(pi.sent.length, 0);
    assert.equal(heldPushQueue.length, 0);
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

  test("260906 (Phase 1): a steer push is held while a compaction is in flight, even on an otherwise-idle lead", () => {
    leadCompactingRef.current = true;
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);
    pushToLead(pi.api, registry, record, "ws-agent-approval", { cmd_id: "c1" }, "steer");
    assert.deepEqual(pi.sent, [], "sendCustomMessage bypasses Pi's own compaction guard — this hold is the only thing stopping it");
    assert.equal(heldPushQueue.length, 1);

    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent[0]!.options?.deliverAs, "steer", "confirmed-start steering preserves a held steer");
  });

  test("260906 (Phase 1): a steer push mid-turn but NOT compacting still bypasses the hold as before", () => {
    idle = false;
    leadCompactingRef.current = false;
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-approval", { cmd_id: "c1" }, "steer");
    assert.equal(pi.sent.length, 1);
    assert.deepEqual(heldPushQueue, []);
  });

  test("260906 (Phase 1): a followUp push is held while compacting even when leadIdleRef itself reports idle", () => {
    leadCompactingRef.current = true;
    const pi = fakePi();
    pushToLead(pi.api, new Map(), undefined, "ws-agent-report", { report: "mid-compaction" }, "followUp");
    assert.deepEqual(pi.sent, []);
    assert.equal(heldPushQueue.length, 1);

    leadCompactingRef.current = false;
    assert.equal(flushHeldPushes(pi.api, true), 1);
    assert.equal(pi.sent[0]!.options?.deliverAs, "steer", "confirmed-start release upgrades held followUp to steering");
  });

  test("260906 (Phase 1): registerPushFlush's agent_settled handler does not flush while compacting", () => {
    leadCompactingRef.current = true;
    const sent: string[] = [];
    let settled: (() => void) | undefined;
    let start: (() => void) | undefined;
    const api = {
      on: (event: string, handler: () => void) => { if (event === "agent_settled") settled = handler; if (event === "agent_start") start = handler; },
      sendUserMessage: () => start?.(),
      sendMessage: (message: { customType?: string }) => void sent.push(message.customType ?? ""),
    } as unknown as Parameters<typeof registerPushFlush>[0];
    registerPushFlush(api, { delayMs: () => 10 });

    heldPushQueue.push({ kind: "push", registry: undefined, record: undefined, family: "ws-agent-report", payload: { report: "held" }, deliverAs: "followUp" });
    settled?.();
    assert.deepEqual(sent, [], "the abort inside ctx.compact() settles the doomed turn before Pi's own compaction flag is set — this gate is what stops a premature flush into it");
    assert.equal(heldPushQueue.length, 1, "left for releaseAfterCompaction to flush once the compaction actually finishes");

    leadCompactingRef.current = false;
    settled?.();
    assert.deepEqual(sent, [PUSH_BATCH_CUSTOM_TYPE], "an ordinary settle once compaction is over flushes one batch normally");
  });

  test("the live-run failure itself: three workers, two finals landing mid-turn, read 1 then 0 — never a premature 0", () => {
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
    assert.equal(flushHeldPushes(pi.api, true), 2);
    assert.deepEqual(
      pi.sent.map((entry) => (entry.message.details as { status?: string }).status),
      ["1 delegated agent still running", "1 delegated agent still running"],
      "at release time one worker is genuinely still out — arrival-time lines said zero",
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
    flushHeldPushes(pi.api, true);

    assert.equal(
      (pi.sent[2].message.details as { status?: string }).status,
      "0 delegated agents still running",
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
    assert.equal(flushHeldPushes(reentrant.api, true), 1);
    assert.equal(heldPushQueue.length, 1, "the re-entrant push waits for the next settle rather than joining this drain");
  });

  test("registerPushFlush requests a wake on settle and releases at confirmed start", () => {
    idle = false;
    const sent: string[] = [];
    let settled: (() => void) | undefined;
    let start: (() => void) | undefined;
    const api = {
      on: (event: string, handler: () => void) => { if (event === "agent_settled") settled = handler; if (event === "agent_start") start = handler; },
      sendUserMessage: () => start?.(),
      sendMessage: (message: { customType?: string }) => void sent.push(message.customType ?? ""),
    } as unknown as Parameters<typeof registerPushFlush>[0];
    registerPushFlush(api, { delayMs: () => 10 });

    pushToLead(api as never, new Map(), undefined, "ws-agent-report", { report: "held" }, "followUp");
    assert.deepEqual(sent, []);

    idle = true;
    settled?.();
    assert.deepEqual(sent, [PUSH_BATCH_CUSTOM_TYPE]);
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
  function listenerHarness(overrides: Partial<RpcAgentRecord> = {}) {
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
    const record = liveRpcRecord({ agentId: "a", running: true, client, ...overrides });
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
      status: "0 delegated agents still running",
    });
    assert.equal(h.pi.sent[0].options?.deliverAs, "steer", "the synchronous fake wake confirms start before delivery");
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
      status: "0 delegated agents still running",
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

  test("a persistent explore record parks after settle and remains resumable", async () => {
    const h = listenerHarness({ spawnRole: "explore" });
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.deepEqual(families(h.pi), ["ws-agent-settled"], "the answer still arrives as the settle push's last_message");
    assert.equal(h.pi.wakes.length, 1, "shared-registry explore completion wakes the idle lead through user preflight");
    assert.match(h.pi.wakes[0] as string, /1 ws messages waiting/);
    assert.equal(h.registry.has("a"), true, "persistent exploration remains in the registry for resume/restart");
    assert.equal(h.record.client, undefined, "settled explorer is parked");
  });

  test("a persistent record is parked (dormant) at settle, preserving the D-C invariant", async () => {
    const h = listenerHarness();
    h.emit({ type: "agent_settled" });
    await settleDrain();
    assert.equal(h.registry.has("a"), true, "still registered, dormant/resumable");
    assert.equal(h.record.client, undefined, "parked");
  });

  test("a plain progress report is admitted as followUp and released as ws-agent-report/steer at confirmed start", () => {
    const h = listenerHarness();
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { message: "halfway" } });
    assert.deepEqual(families(h.pi), ["ws-agent-report"]);
    assert.equal(h.pi.sent[0].options?.deliverAs, "steer");
  });

  test("a hook-consumed question (the TUI owner surface) pushes the registration notice as ws-agent-advisory", () => {
    const h = listenerHarness();
    h.record.onQuestionReport = () => "[ws] thread q1 — the owner answers this.";
    h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "question", message: "which anchor?" } });
    assert.deepEqual(families(h.pi), ["ws-agent-advisory"]);
    assert.equal(h.pi.sent[0].message.details && (h.pi.sent[0].message.details as Record<string, unknown>).advisory, "fork-question-thread");
    assert.equal(h.pi.sent[0].message.details && (h.pi.sent[0].message.details as Record<string, unknown>).detail, "[ws] thread q1 — the owner answers this.");
    assert.equal(h.pi.sent[0].options?.deliverAs, "steer", "the advisory's followUp admission is overridden only at confirmed start");
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

  /**
   * 260905 (alias/park/cap ticket): automatic park is the LAST step of settle
   * handling, run after `probeAgentLiveness` resolves. It parks (silent
   * `stopAgent`) iff `!record.threadBound && !record.running` at that point.
   */
  describe("automatic park (260905 alias/park/cap ticket)", () => {
    function stoppableListenerHarness() {
      const listeners: Array<(evt: unknown) => void> = [];
      const stopCalls: string[] = [];
      const client = {
        onEvent(l: (evt: unknown) => void) {
          listeners.push(l);
          return () => {};
        },
        getState: async () => ({ sessionFile: "/tmp/s.jsonl" }),
        getLastAssistantText: async () => "the last thing it said",
        abort: async () => void stopCalls.push("abort"),
        stop: async () => void stopCalls.push("stop"),
        prompt: async () => {},
      } as unknown as RpcClient;
      const pi = fakePi();
      const record = liveRpcRecord({ agentId: "a", running: true, client });
      const registry: RpcAgentRegistry = new Map([["a", record]]);
      attachEventListener(pi.api, registry, record, client);
      return { pi, record, registry, client, stopCalls, emit: (evt: unknown) => listeners.forEach((l) => l(evt)) };
    }

    const drainTwice = async () => {
      await new Promise((resolve) => setImmediate(resolve));
      await new Promise((resolve) => setImmediate(resolve));
    };

    test("a settled, non-threadBound, non-running record is parked (silent stop) after the settle push, with no extra push for the park itself", async () => {
      const h = stoppableListenerHarness();
      h.emit({ type: "agent_settled" });
      await drainTwice();

      assert.equal(h.record.client, undefined, "parked: client cleared");
      assert.deepEqual(h.stopCalls, ["abort", "stop"], "the park goes through the existing silent-stop path");
      assert.deepEqual(
        h.pi.sent.map((s) => s.message.customType),
        ["ws-agent-settled"],
        "the settle notice is the only push — park itself is silent",
      );
    });

    test("a threadBound record is never parked, even though it settles with no push at all", async () => {
      const h = stoppableListenerHarness();
      h.record.threadBound = true;
      h.emit({ type: "agent_settled" });
      await drainTwice();

      assert.notEqual(h.record.client, undefined, "a threadBound record must stay live — the owner exchange is not the lead's business");
      assert.deepEqual(h.stopCalls, []);
      assert.deepEqual(h.pi.sent, []);
    });

    test("a record the nudge re-prompted before the settle IIFE resumed is not parked", async () => {
      const h = stoppableListenerHarness();
      // Simulate fork.ts's wireAntiBleedLoop: a SECOND, independent
      // `client.onEvent` listener registered after attachEventListener's own,
      // which fires synchronously within the same event dispatch and
      // re-prompts the record — flipping `running` back to true — before this
      // module's own settle IIFE resumes past its first `await`.
      h.client.onEvent(() => {
        void promptAgent(h.record, h.client, "nudge", { isLeadPrompt: false });
      });

      h.emit({ type: "agent_settled" });
      await drainTwice();

      assert.notEqual(h.record.client, undefined, "not parked — the nudge re-prompted it");
      assert.equal(h.record.running, true);
      assert.deepEqual(h.stopCalls, [], "no park attempt at all");
    });

    test("a final still flows through the settle push before the park happens", async () => {
      const h = stoppableListenerHarness();
      h.emit({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, args: { kind: "final", message: "Outcome: done" } });
      h.emit({ type: "agent_settled" });
      await drainTwice();

      assert.deepEqual(
        h.pi.sent.map((s) => s.message.customType),
        ["ws-agent-report"],
        "the final report is delivered exactly as before",
      );
      assert.equal((h.pi.sent[0].message.details as { report?: string }).report, "Outcome: done");
      assert.equal(h.record.client, undefined, "and the child is still parked afterward");
    });
  });
});

/**
 * Review relay #1, test partition C3: `spawnAgent`'s launch-failure branch is
 * live-gate only (it constructs a real `RpcClient`), so its push half was
 * extracted into `pushSpawnFailed` and is covered here.
 */
describe("same-process fork /done finish coordinator", () => {
  const drain = () => new Promise((resolve) => setImmediate(resolve));

  test("an already-idle fork issues one closeout, accepts its successful final once, and silently parks", async () => {
    const pi = fakePi();
    const prompts: string[] = [];
    const stops: string[] = [];
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({ sessionFile: "/tmp/fork.jsonl" }),
      prompt: async (message: string) => void prompts.push(message),
      abort: async () => void stops.push("abort"),
      stop: async () => void stops.push("stop"),
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "fork", client, spawnRole: "fork", threadBound: true, validateForkFinal: () => true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);

    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    assert.equal(prompts.length, 1, "idle /done evaluates immediately instead of waiting for a never-coming settle");
    assert.equal(record.forkFinish?.closeoutIssued, true);

    listener?.({ type: "agent_start" });
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "report-1", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "report-1", isError: false });
    // Duplicate report callbacks may arrive around the settle boundary; the
    // coordinator keeps one selected terminal event and one silent park.
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "report-1", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "report-1", isError: false });
    listener?.({ type: "agent_settled" });
    listener?.({ type: "agent_settled" });
    await drain();
    await drain();

    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-report"]);
    assert.equal((pi.sent[0].message.details as { report?: string }).report, "Outcome: done");
    assert.deepEqual(stops, ["abort", "stop"], "the coordinator owns one silent park");
    assert.equal(record.forkFinish, undefined);
  });

  test("a final racing /done waits for its successful report-tool end and is delivered without a closeout", async () => {
    const pi = fakePi();
    const prompts: string[] = [];
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}), prompt: async (message: string) => void prompts.push(message),
      abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "racing-final", client, threadBound: true, validateForkFinal: () => true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);

    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "racing-report", args: { kind: "final", message: "Outcome: done" } });
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    assert.deepEqual(prompts, [], "a report start alone cannot be rejected or trigger a second closeout");

    listener?.({ type: "tool_execution_end", toolCallId: "racing-report", isError: false });
    await drain();
    await drain();
    assert.deepEqual(prompts, []);
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-report"]);
  });

  test("a failed closeout final emits one missing-final advisory and never retries", async () => {
    const pi = fakePi();
    const prompts: string[] = [];
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}), prompt: async (message: string) => void prompts.push(message),
      abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "failed-final", client, threadBound: true, validateForkFinal: () => true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);

    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    listener?.({ type: "agent_start" });
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "failed-report", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "failed-report", isError: true });
    listener?.({ type: "agent_settled" });
    await drain();
    await drain();

    assert.deepEqual(prompts.length, 1, "a failed report never causes a resend");
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
    assert.equal((pi.sent[0].message.details as { advisory?: string }).advisory, "missing-final");
  });

  test("a running fork waits for its actual settle before issuing a single closeout", async () => {
    const pi = fakePi();
    const prompts: string[] = [];
    let listener: ((event: unknown) => void) | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}), prompt: async (message: string) => void prompts.push(message),
      abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "running-fork", client, running: true, streaming: true, threadBound: true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);
    const first = startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    assert.equal(startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" }), first, "repeated /done joins the same operation");
    await drain();
    assert.deepEqual(prompts, [], "a running child is never interrupted for closeout");

    listener?.({ type: "agent_settled" });
    await drain();
    assert.equal(prompts.length, 1);
  });

  test("a terminal event already held is neither re-admitted nor followed by a closeout", async () => {
    const pi = fakePi();
    const prompts: string[] = [];
    const client = {
      prompt: async (message: string) => void prompts.push(message),
      abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "fork", client, threadBound: true, terminalDelivery: { state: "held" } });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    assert.deepEqual(prompts, []);
    assert.deepEqual(pi.sent, []);
    assert.equal(record.forkFinish, undefined, "the already-held event is the sole terminal obligation");
  });

  test("a duplicate prior-run settle cannot finish a closeout that has not started", async () => {
    const pi = fakePi();
    let listener: ((event: unknown) => void) | undefined;
    let releasePrompt!: () => void;
    const pendingPrompt = new Promise<void>((resolve) => { releasePrompt = resolve; });
    const calls: string[] = [];
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}),
      prompt: async (message: string) => { calls.push(message); await pendingPrompt; },
      abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "duplicate-settle", client, running: true, streaming: true, threadBound: true, validateForkFinal: () => true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);
    const operation = startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });

    listener?.({ type: "agent_settled" });
    await drain();
    listener?.({ type: "agent_settled" });
    await drain();
    assert.equal(calls.length, 1);
    assert.equal(operation.settled, false, "the duplicate belongs to the pre-closeout run");
    assert.deepEqual(pi.sent, []);

    // The actual closeout turn can start and settle before prompt() resolves;
    // it is replayed after that promise, rather than being stranded.
    listener?.({ type: "agent_start" });
    listener?.({ type: "agent_settled" });
    await drain();
    assert.deepEqual(pi.sent, []);
    releasePrompt();
    await drain();
    await drain();
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
  });

  test("a dormant coordinator transfers to its own resumed launch before processing closeout events", async () => {
    const pi = fakePi();
    let listener: ((event: unknown) => void) | undefined;
    const prompts: string[] = [];
    const stops: string[] = [];
    const original = Object.fromEntries(["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"].map(name => [name, RpcClient.prototype[name as keyof RpcClient]]));
    Object.assign(RpcClient.prototype, {
      start: async function(this: RpcClient) {
        const env = (this as unknown as { options: { env?: Record<string, string> } }).options.env;
        writeFileSync(env?.WS_PI_FORK_READY_PATH ?? "", JSON.stringify({
          nonce: env?.WS_PI_FORK_READY_NONCE,
          ownSessionKey: "dormant-child",
          sessionPath: "/tmp/dormant-fork.jsonl",
          sessionId: "dormant-child-session",
        }));
      },
      stop: async () => void stops.push("stop"),
      abort: async () => void stops.push("abort"),
      onEvent: (callback: (event: unknown) => void) => { listener = callback; return () => {}; },
      prompt: async (message: string) => void prompts.push(message),
      getState: async () => ({ model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: "/tmp/dormant-fork.jsonl" }),
      setThinkingLevel: async () => {},
    });
    try {
      const record = freshRpcRecord({ agentId: "dormant-finish", spawnRole: "fork", threadBound: true, validateForkFinal: () => true });
      const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
      const operation = startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
      await drain();
      assert.equal(prompts.length, 1);
      assert.equal(operation.generation, record.launchGeneration, "the finish moves to its coordinator-owned dormant resume");

      listener?.({ type: "agent_start" });
      listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "resumed-final", args: { kind: "final", message: "Outcome: done" } });
      listener?.({ type: "tool_execution_end", toolCallId: "resumed-final", isError: false });
      listener?.({ type: "agent_settled" });
      await drain();
      await drain();

      assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-report"]);
      assert.deepEqual(stops, ["abort", "stop"]);
    } finally {
      Object.assign(RpcClient.prototype, original);
    }
  });

  test("a failed dormant finish launch selects exactly one advisory terminal", async () => {
    const pi = fakePi();
    const original = { start: RpcClient.prototype.start, stop: RpcClient.prototype.stop };
    let stops = 0;
    Object.assign(RpcClient.prototype, {
      start: async () => { throw new Error("resume launch rejected"); },
      stop: async () => { stops += 1; },
    });
    try {
      const record = freshRpcRecord({
        agentId: "failed-dormant-finish", spawnRole: "fork", threadBound: true,
        onForkFinishComplete: (finished) => { finished.threadBound = false; },
      });
      const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
      const operation = startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
      await operation.advancing;
      assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
      assert.equal((pi.sent[0].message.details as { advisory?: string }).advisory, "missing-final");
      assert.equal(operation.phase, "complete");
      assert.equal(record.client, undefined);
      assert.equal(record.threadBound, false);
      assert.equal(stops, 1);
    } finally {
      Object.assign(RpcClient.prototype, original);
    }
  });

  test("ordinary dormant resume launch failure retains its spawn-failed terminal", async () => {
    const pi = fakePi();
    const original = { start: RpcClient.prototype.start, stop: RpcClient.prototype.stop };
    Object.assign(RpcClient.prototype, {
      start: async () => { throw new Error("ordinary launch rejected"); },
      stop: async () => {},
    });
    try {
      const record = freshRpcRecord({ agentId: "ordinary-failed-resume", spawnRole: "fork", threadBound: true });
      await assert.rejects(sendToAgent(new Map([[record.agentId, record]]), { pi: pi.api, cwd: "/tmp", extensionPath: "/tmp/extension.ts" }, record.agentId, "new work"), /ordinary launch rejected/);
      assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-settled"]);
      assert.equal((pi.sent[0].message.details as { reason?: string }).reason, "spawn-failed");
    } finally {
      Object.assign(RpcClient.prototype, original);
    }
  });

  test("superseding a dormant finish during launch failure cleanup prevents stale terminal delivery", async () => {
    for (const boundary of ["start", "stop"] as const) {
      const pi = fakePi();
      const original = { start: RpcClient.prototype.start, stop: RpcClient.prototype.stop };
      let release!: () => void;
      const gate = new Promise<void>((resolve) => { release = resolve; });
      Object.assign(RpcClient.prototype, {
        start: async () => { if (boundary === "start") await gate; throw new Error("old launch rejected"); },
        stop: async () => { if (boundary === "stop") await gate; },
      });
      try {
        const record = freshRpcRecord({ agentId: `superseded-dormant-${boundary}`, spawnRole: "fork", threadBound: true });
        const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
        const operation = startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
        await drain();
        const replacement = { prompt: async () => {} } as unknown as RpcClient;
        record.client = replacement;
        record.launchGeneration = (record.launchGeneration ?? 0) + 1;
        await sendToAgent(registry, { pi: pi.api, cwd: "/tmp" }, record.agentId, "replacement work");
        record.pendingFinal = "replacement final";
        release();
        await operation.advancing;
        assert.deepEqual(pi.sent, [], boundary);
        assert.equal(record.client, replacement, boundary);
        assert.equal(record.pendingFinal, "replacement final", boundary);
        assert.equal(record.running, true, boundary);
      } finally {
        release();
        Object.assign(RpcClient.prototype, original);
      }
    }
  });

  test("a duplicate original settle after closeout start cannot settle the closeout run", async () => {
    const pi = fakePi();
    let listener: ((event: unknown) => void) | undefined;
    const prompts: string[] = [];
    const stops: string[] = [];
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}),
      prompt: async (message: string) => void prompts.push(message),
      abort: async () => void stops.push("abort"),
      stop: async () => void stops.push("stop"),
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "late-duplicate", client, running: true, streaming: true, threadBound: true, validateForkFinal: () => true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();

    const originalSettle = { type: "agent_settled" };
    listener?.(originalSettle);
    await drain();
    assert.equal(prompts.length, 1);
    listener?.({ type: "agent_start" });
    listener?.(originalSettle);
    await drain();
    assert.deepEqual(pi.sent, [], "the repeated original event is not closeout completion");
    assert.deepEqual(stops, []);

    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "closeout-final", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "closeout-final", isError: false });
    listener?.({ type: "agent_settled" });
    await drain();
    await drain();
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-report"]);
    assert.deepEqual(stops, ["abort", "stop"]);
  });

  test("a closeout question overrides an earlier valid final without reopening owner routing", async () => {
    const pi = fakePi();
    let listener: ((event: unknown) => void) | undefined;
    let ownerQuestions = 0;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}), prompt: async () => {}, abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "question-wins", client, threadBound: true, validateForkFinal: () => true, onQuestionReport: () => { ownerQuestions += 1; } });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    listener?.({ type: "agent_start" });
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "final", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "final", isError: false });
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "question", args: { kind: "question", message: "Need owner input" } });
    listener?.({ type: "agent_settled" });
    await drain();
    await drain();
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
    assert.equal(ownerQuestions, 0, "finish owns the question instead of reopening the owner thread");
  });

  test("the actual fork validation policy rejects malformed and Commit: none finals", async () => {
    for (const [message, expectsCommit] of [["Outcome: incomplete", false], ["Outcome: done\nFiles changed: x\nVerification: pass\nBlockers: none\nCommit: none", true]] as const) {
      const pi = fakePi();
      let listener: ((event: unknown) => void) | undefined;
      const client = {
        onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
        getState: async () => ({}), prompt: async () => {}, abort: async () => {}, stop: async () => {},
      } as unknown as RpcClient;
      const record = liveRpcRecord({ agentId: `policy-${expectsCommit}`, client, threadBound: true });
      const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
      armForkRoleWiring(pi.api as ExtensionAPI, registry, record, undefined, expectsCommit);
      attachEventListener(pi.api, registry, record, client);
      startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
      await drain();
      listener?.({ type: "agent_start" });
      listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "final", args: { kind: "final", message } });
      listener?.({ type: "tool_execution_end", toolCallId: "final", isError: false });
      listener?.({ type: "agent_settled" });
      await drain();
      await drain();
      assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
    }
  });

  test("a rejected closeout chooses one advisory terminal instead of exited plus advisory", async () => {
    const pi = fakePi();
    const client = {
      onEvent: () => () => {}, getState: async () => ({}),
      prompt: async () => { throw new Error("gone"); }, abort: async () => {}, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "rejected-closeout", client, threadBound: true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    await drain();
    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-advisory"]);
  });

  test("a fresh lead send supersedes an unresolved closeout without an old terminal or park", async () => {
    const pi = fakePi();
    let releaseCloseout!: () => void;
    const closeoutGate = new Promise<void>((resolve) => { releaseCloseout = resolve; });
    let promptCalls = 0;
    const client = {
      onEvent: () => () => {}, getState: async () => ({}),
      prompt: async () => { promptCalls += 1; if (promptCalls === 1) await closeoutGate; },
      abort: async () => { throw new Error("old closeout must not park"); }, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "superseded", client, threadBound: true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    await sendToAgent(registry, { pi: pi.api, cwd: "/tmp" }, record.agentId, "replacement task");
    releaseCloseout();
    await drain();
    await drain();

    assert.equal(record.forkFinish, undefined);
    assert.equal(record.threadBound, true, "the replacement owns its bind");
    assert.deepEqual(pi.sent, []);
  });

  test("a failed silent park reports the operational failure once without a second closeout", async () => {
    const pi = fakePi();
    let listener: ((event: unknown) => void) | undefined;
    let completionFailure: string | undefined;
    const client = {
      onEvent(callback: (event: unknown) => void) { listener = callback; return () => {}; },
      getState: async () => ({}), prompt: async () => {},
      abort: async () => { throw new Error("cannot abort"); }, stop: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({ agentId: "park-failure", client, threadBound: true, validateForkFinal: () => true, onForkFinishComplete: (_record, failure) => { completionFailure = failure; } });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    attachEventListener(pi.api, registry, record, client);
    startForkFinish(record, registry, pi.api!, { cwd: "/tmp", extensionPath: "/tmp/extension.ts" });
    await drain();
    listener?.({ type: "agent_start" });
    listener?.({ type: "tool_execution_start", toolName: REPORT_TO_LEAD_TOOL_NAME, toolCallId: "final", args: { kind: "final", message: "Outcome: done" } });
    listener?.({ type: "tool_execution_end", toolCallId: "final", isError: false });
    listener?.({ type: "agent_settled" });
    await drain();
    await drain();

    assert.deepEqual(pi.sent.map((entry) => entry.message.customType), ["ws-agent-report"]);
    assert.match(completionFailure ?? "", /park failed/);
  });
});

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
      { agent_id: "a", reason: "spawn-failed", error: "spawn ENOENT", status: "0 delegated agents still running" },
      "260905 (alias/park/cap): the failed record stays registered (dormant), so presence — keyed on registry membership, not a live client — keeps the status line at zero",
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

  test("a failed persistent explore stays registered as a dormant record", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a", spawnRole: "explore" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushSpawnFailed(pi.api, registry, record, new Error("client.start() failed"));

    assert.equal(pi.sent.length, 1, "the lead still learns the explore failed");
    assert.equal(pi.sent[0].message.details && (pi.sent[0].message.details as { reason?: string }).reason, "spawn-failed");
    assert.equal(registry.has("a"), true, "persistent explore failures retain a resumable registry record");
    assert.equal(record.client, undefined);
  });

  test("a persistent record's launch failure still parks dormant", () => {
    const pi = fakePi();
    const record = liveRpcRecord({ agentId: "a" });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    pushSpawnFailed(pi.api, registry, record, new Error("client.start() failed"));

    assert.equal(registry.has("a"), true, "an ordinary spawn failure keeps parking as today");
    assert.equal(registry.get("a")?.client, undefined);
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

  test("260905: stamps runStartedAt unconditionally, overwriting any stale prior value", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ runStartedAt: 1_000 });
    const before = Date.now();

    await promptAgent(record, client, "do the thing");

    assert.ok((record.runStartedAt ?? 0) >= before, "runStartedAt must be re-stamped to the new run's start, not left at the stale value");
  });

  test("260905: the anti-bleed nudge (isLeadPrompt:false) ALSO stamps runStartedAt — the widget's elapsed clock tracks any prompt, not just lead prompts", async () => {
    const { client } = fakeRpcClient();
    const record = freshRpcRecord({ runStartedAt: 1_000 });
    const before = Date.now();

    await promptAgent(record, client, "nudge", { isLeadPrompt: false });

    assert.ok((record.runStartedAt ?? 0) >= before);
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
      // 260905 (alias/park/cap): the stopped record stays registered
      // (dormant), so presence — keyed on registry membership, not a live
      // client — keeps the status line at zero.
      status: "0 delegated agents still running",
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

  test("stopAgent and getAgentTranscriptPath both succeed against a persistent Explore record", async () => {
    const pi = fakePi();
    const { client, calls } = stoppableClient();
    const record = freshRpcRecord({ agentId: "e", client, running: true, spawnRole: "explore", sessionPath: "/tmp/ws-pi-agent-e/session.jsonl" });
    const registry: RpcAgentRegistry = new Map([["e", record]]);

    assert.deepEqual(await stopAgent(registry, "e", pi.api), { agent_id: "e" }, "persistent Explore uses ordinary stop semantics");
    assert.deepEqual(calls, ["abort", "stop"]);
    assert.equal(registry.has("e"), true, "stop retains the persistent record");
    assert.deepEqual(getAgentTranscriptPath(registry, "e"), { transcript_path: "/tmp/ws-pi-agent-e/session.jsonl" });
  });

  test("unknown agentId throws", async () => {
    await assert.rejects(() => stopAgent(new Map(), "missing"), /unknown agentId/);
  });

  test("260905 (alias/park/cap): resolves by alias — ws-agent-stop <alias> stops the aliased holder", async () => {
    const pi = fakePi();
    const { client, calls } = stoppableClient();
    const record = freshRpcRecord({ agentId: "a", alias: "scout", client, running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await stopAgent(registry, "scout", pi.api);

    assert.deepEqual(result, { agent_id: "a" }, "the canonical uuid is returned, not the alias");
    assert.deepEqual(calls, ["abort", "stop"]);
    assert.equal(record.client, undefined);
  });

  test("260905 review relay #1 (Important): record.client/running are cleared SYNCHRONOUSLY, before abort()/stop() resolve — closes the mid-park send race", async () => {
    // A concurrent `ws-agent-send` (or overlay ForkChannel) races the park
    // path: it must see the record as dormant (client undefined) during the
    // window `stopAgent` awaits `abort()`/`stop()`, not just after both
    // complete — otherwise it would take the live branch and prompt a
    // client that's mid-teardown, losing the turn silently.
    let releaseAbort: () => void = () => {};
    const abortGate = new Promise<void>((resolve) => {
      releaseAbort = resolve;
    });
    const calls: string[] = [];
    const client = {
      abort: async () => {
        calls.push("abort");
        await abortGate;
      },
      stop: async () => {
        calls.push("stop");
      },
    } as unknown as RpcClient;
    const record = freshRpcRecord({ agentId: "a", client, running: true });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const stopPromise = stopAgent(registry, "a", undefined, { silent: true });

    // No `await` needed: an async function runs synchronously up to its
    // first `await` (inside client.abort() here), so by the time control
    // returns to this line, clearLiveState has already run.
    assert.equal(record.client, undefined, "a racing send must see this record as dormant WHILE abort() is still pending");
    assert.equal(record.running, false);
    assert.deepEqual(calls, ["abort"], "abort() has been called but not yet resolved");

    releaseAbort();
    await stopPromise;
    assert.deepEqual(calls, ["abort", "stop"]);
  });

  test("a completed old stop cannot clear a replacement launch's owner bind or final", async () => {
    let releaseAbort!: () => void;
    const abortGate = new Promise<void>((resolve) => { releaseAbort = resolve; });
    const oldClient = {
      abort: async () => { await abortGate; }, stop: async () => {},
    } as unknown as RpcClient;
    const record = freshRpcRecord({ agentId: "a", client: oldClient, launchGeneration: 1, running: true, threadBound: true });
    const registry: RpcAgentRegistry = new Map([[record.agentId, record]]);
    const stopping = stopAgent(registry, record.agentId, undefined, { silent: true });

    record.client = { prompt: async () => {} } as unknown as RpcClient;
    record.launchGeneration = 2;
    record.threadBound = true;
    record.pendingFinal = "replacement final";
    releaseAbort();
    await stopping;

    assert.equal(record.threadBound, true);
    assert.equal(record.pendingFinal, "replacement final");
    assert.notEqual(record.client, undefined);
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

  test("a busy real instruction clears terminal delivery facts but an anti-bleed nudge preserves them", async () => {
    const client = {
      followUp: async () => {}, steer: async () => {}, prompt: async () => {},
    } as unknown as RpcClient;
    const record = liveRpcRecord({
      agentId: "a", client, running: true, streaming: true,
      pendingFinal: "old", pendingFinalToolCallId: "old-call", pendingFinalAccepted: true,
      terminalDelivery: { state: "enqueued" },
    });
    await sendToAgent(new Map([[record.agentId, record]]), { cwd: "/tmp" }, record.agentId, "replacement");
    assert.equal(record.pendingFinal, undefined);
    assert.equal(record.pendingFinalToolCallId, undefined);
    assert.equal(record.pendingFinalAccepted, undefined);
    assert.equal(record.terminalDelivery, undefined);

    record.terminalDelivery = { state: "held" };
    await promptAgent(record, client, "anti-bleed", { isLeadPrompt: false });
    assert.equal(record.terminalDelivery?.state, "held", "the internal nudge is not a task boundary");
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

  test("a persistent explore record accepts ws-agent-send", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "e", client, spawnRole: "explore" });
    const registry: RpcAgentRegistry = new Map([["e", record]]);

    await sendToAgent(registry, { cwd: "/tmp" }, "e", "hi");

    assert.deepEqual(calls, [["prompt", "hi"]]);
  });

  test("260905 (alias/park/cap): resolves by alias — ws-agent-send <alias> drives the aliased holder", async () => {
    const { client, calls } = fakeRpcClient();
    const record = freshRpcRecord({ agentId: "a", alias: "scout", client, streaming: false });
    const registry: RpcAgentRegistry = new Map([["a", record]]);

    const result = await sendToAgent(registry, { cwd: "/tmp" }, "scout", "hello");

    assert.deepEqual(result, { agent_id: "a" }, "the canonical uuid is returned, not the alias");
    assert.deepEqual(calls, [["prompt", "hello"]]);
  });
});

describe("buildRpcClientOptions (WS_PI_SPAWN_ROLE_ENV / WS_PI_APPROVAL_DIR_ENV placement)", () => {
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

  test("built options carry the worker role marker and the approvals dir derived from sessionPath's own directory", () => {
    const options = buildRpcClientOptions("/repo", "provider/model", "/tmp/ws-pi-agent-x/session.jsonl", "/tmp/system.md", "read,bash");
    assert.deepEqual(options.env, {
      [WS_PI_SPAWN_ROLE_ENV]: "worker",
      [WS_PI_APPROVAL_DIR_ENV]: "/tmp/ws-pi-agent-x/approvals",
      WS_PI_EXPLORE_MODE: "",
      WS_PI_FORK_CONTEXT: "",
      WS_PI_FORK_READY_PATH: "",
      WS_PI_FORK_READY_NONCE: "",
      WS_PI_FORK_AFFINITY: "",
      [WS_PI_PARENT_SESSION_KEY_ENV]: "",
      WS_PI_DELEGATION_POLICY: "",
      WS_PI_SUBTREE_CHANNEL: "",
      WS_PI_WEB_HOME: "",
      WS_PI_WEB_READY_NONCE: "",
      WS_MCP_BOOTSTRAP_BINARY: "",
      WS_MCP_BOOTSTRAP_URL: "",
    });
  });

  test("env overrides an inherited exploration mode while preserving role and approvals markers", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-y/session.jsonl", "/tmp/system.md", "read");
    assert.deepEqual(new Set(Object.keys(options.env ?? {})), new Set([WS_PI_SPAWN_ROLE_ENV, WS_PI_APPROVAL_DIR_ENV, "WS_PI_EXPLORE_MODE", "WS_PI_FORK_CONTEXT", "WS_PI_FORK_READY_PATH", "WS_PI_FORK_READY_NONCE", "WS_PI_FORK_AFFINITY", WS_PI_PARENT_SESSION_KEY_ENV, "WS_PI_DELEGATION_POLICY", "WS_PI_SUBTREE_CHANNEL", "WS_PI_WEB_HOME", "WS_PI_WEB_READY_NONCE", "WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL"]));
    assert.equal(options.env?.WS_PI_EXPLORE_MODE, "");
  });

  test("260904 Phase 1: the approvals dir is inert-but-present even for a non-execute-worker (full-worker) spawn — WS_PI_APPROVAL_DIR is always derived from sessionPath, not gated on tools", () => {
    const options = buildRpcClientOptions("/repo", undefined, "/tmp/ws-pi-agent-z/session.jsonl", "/tmp/system.md", resolveTools("full-worker"));
    assert.equal(options.env?.[WS_PI_APPROVAL_DIR_ENV], "/tmp/ws-pi-agent-z/approvals");
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
});

/**
 * 260905 (alias/park/cap ticket): the cap half of `spawnAgent`'s guard
 * clauses, extracted for direct coverage for the same reason as
 * `reserveAgentAlias`.
 */
describe("evictForCapacity", () => {
  test("under the cap is a no-op ok with no eviction", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a" })]]);
    assert.deepEqual(evictForCapacity(registry, 5), { ok: true });
    assert.equal(registry.size, 1);
  });

  test("at the cap evicts the dormant record with the OLDEST last-activity stamp", () => {
    const oldest = freshRpcRecord({ agentId: "old", lastLeadPromptAt: 1_000 });
    const newer = freshRpcRecord({ agentId: "new", lastLeadPromptAt: 5_000 });
    const registry: RpcAgentRegistry = new Map([
      ["old", oldest],
      ["new", newer],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "old" });
    assert.equal(registry.has("old"), false);
    assert.equal(registry.has("new"), true);
  });

  test("last-activity is max(lastLeadPromptAt, newest reportLog entry) — a quiet-but-recently-reported record is not the oldest", () => {
    const staleReport = freshRpcRecord({ agentId: "stale", lastLeadPromptAt: 1_000, reportLog: [{ at: 1_500 }] });
    const freshlyReported = freshRpcRecord({ agentId: "fresh", lastLeadPromptAt: 1_000, reportLog: [{ at: 9_000 }] });
    const registry: RpcAgentRegistry = new Map([
      ["stale", staleReport],
      ["fresh", freshlyReported],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "stale" });
  });

  test("evicted label prefers the record's alias over its bare uuid", () => {
    const registry: RpcAgentRegistry = new Map([["a", freshRpcRecord({ agentId: "a", alias: "scout" })]]);
    assert.deepEqual(evictForCapacity(registry, 1), { ok: true, evictedLabel: "scout" });
  });

  test("running and threadBound records are never evicted", () => {
    const running = freshRpcRecord({ agentId: "running-one", running: true });
    const threadBound = freshRpcRecord({ agentId: "bound-one", threadBound: true });
    const registry: RpcAgentRegistry = new Map([
      ["running-one", running],
      ["bound-one", threadBound],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.equal(result.ok, false);
    if (!result.ok) {
      assert.match(result.error, /cap/);
    }
    assert.equal(registry.size, 2, "a rejected eviction must not remove anything");
  });

  test("a live (client-holding) idle record is never evicted either — only a fully dormant record is a candidate", () => {
    const live = freshRpcRecord({ agentId: "live-one", client: {} as RpcClient, running: false });
    const registry: RpcAgentRegistry = new Map([["live-one", live]]);
    const result = evictForCapacity(registry, 1);
    assert.equal(result.ok, false);
  });

  test("a cap of 1 with 3 dormant entries evicts all of them, oldest-first, and joins the labels — the spawn itself will occupy the sole slot", () => {
    const a = freshRpcRecord({ agentId: "a", lastLeadPromptAt: 1_000 });
    const b = freshRpcRecord({ agentId: "b", lastLeadPromptAt: 2_000 });
    const c = freshRpcRecord({ agentId: "c", lastLeadPromptAt: 3_000 });
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
      assert.deepEqual(readdirSync(join(realpathSync(root), "ws-agents", "lead-1")), [".cost-rollup"], "eviction retains only the durable cost roll-up directory");
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

  test("an owned-home deletion failure remains diagnostic-only after eligibility and does not reject capacity eviction", () => {
    const root = mkdtempSync(join(tmpdir(), "ws-pi-cap-test-"));
    try {
      const ownership = allocateAgentHome(createAgentStorageContext("lead-1", root), "owned-failure", "worker");
      updateOwnership(ownership.home, { liveness: { lifecycle: "stopped", running: false } });
      const registry: RpcAgentRegistry = new Map([["owned-failure", freshRpcRecord({ agentId: "owned-failure", ownership, sessionPath: ownership.sessionPath })]]);
      const result = evictForCapacity(registry, 1, () => ({ status: "failed", error: "permission denied" }));
      assert.deepEqual(result, { ok: true, evictedLabel: "owned-failure" });
      assert.equal(registry.size, 0);
      assert.ok(readFileSync(join(ownership.home, "ownership.json"), "utf8"), "metadata remains for a later retry");
    } finally { rmSync(root, { recursive: true, force: true }); }
  });

  test("260905 (list-model/last-report-fidelity): prefers to drop a never-active record over a revived orphan whose lastReportAtOverride is newer", () => {
    // Review relay #1 (Critical): both records must have distinct, non-zero
    // activity under the FIXED formula, and "revived" is inserted first so
    // insertion-order tie-breaking cannot coincidentally produce the right
    // answer for the wrong reason. "never" gets a small lastLeadPromptAt
    // (100) that is unambiguously below the override (9_000) only once the
    // override is actually honored — under the pre-fix formula (which
    // ignores lastReportAtOverride entirely), "revived" scores activity 0
    // (lowest) and would be evicted instead, so this test fails if the
    // lastReportAtOverride fallback in evictForCapacity regresses.
    const revived = freshRpcRecord({ agentId: "revived", lastReportAtOverride: new Date(9_000).toISOString() });
    const neverActive = freshRpcRecord({ agentId: "never", lastLeadPromptAt: 100 });
    const registry: RpcAgentRegistry = new Map([
      ["revived", revived],
      ["never", neverActive],
    ]);
    const result = evictForCapacity(registry, 2);
    assert.deepEqual(result, { ok: true, evictedLabel: "never" });
    assert.equal(registry.has("revived"), true);
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
