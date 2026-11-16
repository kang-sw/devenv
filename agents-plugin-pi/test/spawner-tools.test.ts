import { test, describe } from "node:test";
import assert from "node:assert/strict";
import {
  resolveTools,
  spawnAdmission,
  TOOL_GROUPS,
  resolveModelForAliasViaWsMcp,
  effectiveModelEffort,
  REPORT_TO_LEAD_TOOL_NAME,
  GATED_EXEC_TOOL_NAME,
  inheritModelFromToolCtx,
  resolveSpawnToolGroup,
  type ToolGroup,
} from "../src/spawner.ts";
import type { McpStdioClient, McpToolCallResult } from "../src/mcp-stdio-client.ts";
import {
  formatConcreteModelWarning,
  formatTierWarning,
  modelCatalogFromToolCtx,
  suggestModels,
  tierWarningNotifierFromToolCtx,
  validateConcreteModel,
} from "../src/model-catalog.ts";
import { freshRpcRecord } from "./fixtures/spawner-records.ts";

describe("TOOL_GROUPS / resolveTools", () => {
  test("read-only carries no bash and no ws__* tools", () => {
    assert.deepEqual([...TOOL_GROUPS["read-only"]], ["read", "grep", "find", "ls"]);
    assert.equal(resolveTools("read-only", ["ws__playbook_render"]), "read,grep,find,ls");
  });

  test("legacy full-worker groups grant no retired child management or report tool", () => {
    assert.equal(resolveTools("full-worker", ["ws__playbook_render", "ws__ferrule"]), "read,bash,edit,write,grep,find,ls,ws__playbook_render,ws__ferrule");
    assert.equal(resolveTools("full-worker"), "read,bash,edit,write,grep,find,ls");
  });

  test("execute-worker is read-only plus gated execution and intermediate reporting", () => {
    assert.deepEqual([...TOOL_GROUPS["execute-worker"]], ["read", "grep", "find", "ls", GATED_EXEC_TOOL_NAME, REPORT_TO_LEAD_TOOL_NAME]);
    assert.equal(resolveTools("execute-worker"), `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME}`);
  });

  test("execute-worker appends exactly the four ws__ git read tools from the bridge list and no other ws__ tool", () => {
    const base = `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME}`;
    const bridge = ["ws__playbook_render", "ws__git_commit", "ws__git_merge", "ws__git_log", "ws__git_status", "ws__project_tree", "ws__git_diff", "ws__git_merge_base", "ws__git_followups", "ws__ferrule"];
    assert.equal(resolveTools("execute-worker", bridge), `${base},ws__git_log,ws__git_status,ws__git_diff,ws__git_merge_base`);
    assert.equal(resolveTools("execute-worker", ["ws__playbook_render"]), base, "no git tools offered by the bridge, none granted");
  });

  test("execute-worker spawn admission ceiling includes exactly the four ws__ git read tools and no ws__ mutator", () => {
    const bridge = ["ws__git_status", "ws__git_diff", "ws__git_log", "ws__git_merge_base", "ws__git_commit", "ws__git_merge", "ws__tickets_move", "ws__project_tree"];
    const parentPolicy = { version: 1 as const, depth: 0, maxDepth: 2, authority: "lead" as const, tools: resolveTools("full-worker", bridge).split(","), network: { search: true, fetch: true } };
    const policy = spawnAdmission({ parentPolicy, wsToolNames: bridge, toolGroup: "execute-worker", spawnRole: "execute-worker" } as never);
    assert.deepEqual(policy.tools.filter(name => name.startsWith("ws__")).sort(), ["ws__git_diff", "ws__git_log", "ws__git_merge_base", "ws__git_status"]);
    assert.equal(policy.authority, "leaf");
  });

  test("a resumed execute-worker rebuilds the same four ws__ git tools from toolGroup + wsToolNames", () => {
    const bridge = ["ws__git_status", "ws__git_diff", "ws__git_log", "ws__git_merge_base", "ws__git_commit"];
    const record = freshRpcRecord({ toolGroup: "execute-worker" as ToolGroup, wsToolNames: bridge });
    assert.deepEqual(resolveTools(record.toolGroup, record.wsToolNames), resolveTools("execute-worker", bridge));
    assert.ok(!resolveTools(record.toolGroup, record.wsToolNames).includes("ws__git_commit"));
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
    assert.equal(resolved, `read,grep,find,ls,${GATED_EXEC_TOOL_NAME},${REPORT_TO_LEAD_TOOL_NAME}`);
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
