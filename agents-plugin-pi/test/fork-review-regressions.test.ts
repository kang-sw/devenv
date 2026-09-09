import { test } from "node:test";
import assert from "node:assert/strict";
import { rmSync } from "node:fs";
import { dirname } from "node:path";
import { applyForkAffinity, captureForkContext, effectiveForkDescriptor, frameForkInput, restoreForkKeys, writePrivateJson } from "../src/fork-context.ts";
import { buildChildProcessEnv, buildRpcClientOptions, prepareForkLaunch, validateForkReadiness } from "../src/spawner.ts";
import * as role from "../src/process-role.ts";
import * as context from "../src/fork-context.ts";
import { registerLeadBootstrap, type LeadPromptRef } from "../src/lead-bootstrap.ts";
import { normalizeSessionKey } from "../src/bridge.ts";

const fork = captureForkContext({ kind: "task", effectiveSystemPrompt: "original", parentSessionKey: "parent-key", parentPiSessionId: "parent-id", parentAffinityId: "parent-id", activeTools: ["read"], registeredTools: [{ name: "read", description: "Read", parameters: { type: "object" } }] });

test("C1: frame preserves every byte of task/discussion input and identifies own key and all refusals", () => {
  for (const body of ["task Ω\r\ntrailing  ", "discussion /done\r\n"]) {
    const framed = frameForkInput(body, "current-own");
    assert.ok(framed.endsWith(body));
    for (const text of ["current-own", "inherited parent", "historical own key", "ws-fork", "ws-ask", "ws-resolve"]) assert.ok(framed.includes(text));
  }
});

test("C2: every launch has fresh readiness; stale nonce/key/id/path/schema/order is rejected", () => {
  for (const patch of [ { nonce: "stale" }, { ownSessionKey: "" }, { ownSessionKey: "parent-key" }, { sessionId: "parent-id" }, { sessionId: "other-child" }, { sessionPath: "/wrong" }, { registeredTools: [] }, { activeTools: ["unknown"] }, { registeredTools: [{ ...fork.registeredTools[0], description: "changed" }] }, { error: "bootstrap failed" } ]) {
    const launch = prepareForkLaunch(fork);
    const record = { forkContext: fork, sessionPath: "/child" } as never;
    const ready = { nonce: launch.nonce, ownSessionKey: "child-key", sessionId: "child-id", sessionPath: "/child", registeredTools: fork.registeredTools, activeTools: fork.activeTools };
    try {
      writePrivateJson(launch.readinessPath, { ...ready, ...patch });
      assert.throws(() => validateForkReadiness(launch, record, { sessionFile: "/child", sessionId: "child-id" }), /readiness rejected/);
    } finally { rmSync(dirname(launch.contextPath), { recursive: true, force: true }); }
  }
  const first = prepareForkLaunch(undefined), second = prepareForkLaunch(undefined);
  try {
    assert.notEqual(first.nonce, second.nonce);
    assert.notEqual(first.contextPath, second.contextPath);
    assert.equal(context.readForkLaunchContext({ [role.WS_PI_FORK_CONTEXT_ENV]: first.contextPath })?.context, undefined, "explicit legacy exchange does not invent a historical prompt");
  } finally { for (const launch of [first, second]) rmSync(dirname(launch.contextPath), { recursive: true, force: true }); }
});

test("C3: own-key history is bound to child identity across two recovery generations", () => {
  const entry = (sessionId: string, current: string, previous: string[] = []) => ({ type: "custom", customType: context.FORK_KEYS_ENTRY, data: { sessionId, current, previous } });
  const inherited = entry("parent", "never-trust");
  const first = entry("child", "one");
  const second = entry("child", "two", restoreForkKeys([inherited, first], "child"));
  const third = entry("child", "three", restoreForkKeys(JSON.parse(JSON.stringify([inherited, first, second])), "child"));
  assert.deepEqual(restoreForkKeys([inherited, third], "child"), ["three", "one", "two"]);
  assert.deepEqual(restoreForkKeys([inherited, third], "other-child"), []);
  const oldKey = { type: "custom", customType: context.FORK_KEYS_ENTRY, data: { current: "old-owned" } };
  const bound = { type: "custom", customType: context.FORK_CONTEXT_ENTRY, data: { sessionId: "child", context: fork } };
  assert.deepEqual(restoreForkKeys([oldKey], "child"), []);
  assert.deepEqual(restoreForkKeys([bound, oldKey], "child"), ["old-owned"]);
  assert.deepEqual(restoreForkKeys([bound, oldKey], "other-child"), []);
  const oldRole = process.env[role.WS_PI_SPAWN_ROLE_ENV];
  process.env[role.WS_PI_SPAWN_ROLE_ENV] = "fork";
  try {
    for (const ownKey of [undefined, "current"]) assert.throws(() => normalizeSessionKey({ session_key: "parent" }, { ownKey, sentinel: "sentinel", parentLeadKey: "parent" }), /refuses its parent/);
  } finally { if (oldRole === undefined) delete process.env[role.WS_PI_SPAWN_ROLE_ENV]; else process.env[role.WS_PI_SPAWN_ROLE_ENV] = oldRole; }
});

test("C4: capture uses actual before-handler options/block and restores owned snapshots before a new turn", () => {
  const handlers = new Map<string, Function>();
  const entries: any[] = [];
  const ref: LeadPromptRef = { current: undefined };
  const base = { current: { manualSnapshot: "manual-original", guideText: "guide-original" } };
  const pi = { on: (name: string, fn: Function) => handlers.set(name, fn), getCommands: () => [], appendEntry: (customType: string, data: unknown) => entries.push({ type: "custom", customType, data }) };
  registerLeadBootstrap(pi as never, base, { current: undefined }, ref);
  const ctx = { getSystemPrompt: () => "custom base", getSystemPromptOptions: () => ({ appendSystemPrompt: "explicit Ω\r\n  " }), sessionManager: { getEntries: () => entries, getSessionId: () => "lead", getHeader: () => ({ id: "lead" }) } };
  const initial = ref.resolve!(ctx);
  assert.match(initial.effectiveSystemPrompt, /manual-original/);
  assert.deepEqual(initial.basePromptOptions, ctx.getSystemPromptOptions());
  const options = { appendSystemPrompt: "actual event Ω\r\n  " };
  const before = handlers.get("before_agent_start")!({ systemPrompt: "base with explicit append", systemPromptOptions: options });
  base.current = { manualSnapshot: "changed", guideText: "changed" };
  const finalPrompt = before.systemPrompt + "\nLATER HANDLER";
  handlers.get("context")!({}, { ...ctx, getSystemPrompt: () => finalPrompt });
  assert.equal(ref.current?.effectiveSystemPrompt, finalPrompt);
  assert.deepEqual(ref.current?.basePromptOptions, options);
  assert.match(ref.current?.wsBlock ?? "", /manual-original/);
  ref.current = undefined;
  handlers.get("session_start")!({}, ctx);
  assert.equal(ref.resolve!(ctx).effectiveSystemPrompt, finalPrompt);
});

test("C5: affinity compares complete model, auth endpoint/headers, and effective effort without storing secrets", async () => {
  const model = { provider: "openai-codex", api: "openai-codex-responses", id: "m", baseUrl: "https://endpoint", compat: { supportsStore: true }, headers: { authorization: "sensitive-model-value" } };
  const auth = { auth: { apiKey: "sensitive-key", headers: { authorization: "sensitive-auth" }, baseUrl: "https://effective-endpoint", env: { EXTRA: "sensitive-env" } } };
  const ctx = (m = model, a = auth) => ({ model: m, modelRegistry: { getProviderAuth: async () => a } });
  const descriptor = await effectiveForkDescriptor(ctx(), "high");
  const captured = captureForkContext({ ...fork, modelDescriptor: descriptor });
  assert.equal(JSON.stringify(captured).includes("sensitive"), false);
  const payload = { instructions: "original", tools: [], input: [], model: "m", prompt_cache_key: "child-id" };
  assert.deepEqual(applyForkAffinity(payload, captured, await effectiveForkDescriptor(ctx(), "high"), "child-id"), { ...payload, prompt_cache_key: "parent-id" });
  for (const changed of [await effectiveForkDescriptor(ctx(), "low"), await effectiveForkDescriptor(ctx({ ...model, id: "override" }), "high"), await effectiveForkDescriptor(ctx({ ...model, compat: { supportsStore: false } }), "high"), await effectiveForkDescriptor(ctx(model, { auth: { ...auth.auth, baseUrl: "https://changed" } }), "high"), await effectiveForkDescriptor(ctx(model, { auth: { ...auth.auth, apiKey: "different-secret" } }), "high")]) assert.equal(applyForkAffinity(payload, captured, changed, "child-id"), undefined);
  for (const bad of [{ ...payload, prompt_cache_key: undefined }, { ...payload, prompt_cache_key: "" }, { prompt_cache_key: "child-id" }]) assert.equal(applyForkAffinity(bad, captured, descriptor, "child-id"), undefined);
});

test("C6/I1/I2: source adapter argv and canonical markers isolate every descendant launch", () => {
  assert.equal(context.FORK_CONTEXT_ENV, role.WS_PI_FORK_CONTEXT_ENV);
  assert.equal(context.FORK_AFFINITY_ENV, role.WS_PI_FORK_AFFINITY_ENV);
  const markers = [role.WS_PI_FORK_CONTEXT_ENV, role.WS_PI_FORK_READY_PATH_ENV, role.WS_PI_FORK_READY_NONCE_ENV, role.WS_PI_FORK_AFFINITY_ENV, role.WS_PI_PARENT_SESSION_KEY_ENV];
  const poison = Object.fromEntries(markers.map(key => [key, "poison"]));
  const terminal = buildChildProcessEnv(poison);
  for (const key of markers) assert.equal(key in terminal, false);
  for (const spawnRole of ["worker", "execute-worker", "explore"] as const) for (const mode of ["simple", "deep"] as const) {
    const options = buildRpcClientOptions("/repo", undefined, "/child", "/worker-prompt", "read", undefined, undefined, spawnRole, mode);
    const merged = { ...poison, ...options.env };
    for (const key of markers) assert.equal(merged[key], "");
    assert.deepEqual(options.args, ["--session", "/child", "--session-dir", "/", "--append-system-prompt", "/worker-prompt", "--tools", "read"]);
  }
  for (const source of [undefined, "/lead"]) {
    const options = buildRpcClientOptions("/repo", undefined, "/child", "/old-directive", "read", source, "original-parent", "fork");
    assert.equal(options.args?.includes("--append-system-prompt"), false);
    assert.equal(options.args?.[options.args.indexOf("--extension") + 1], new URL("../src/index.ts", import.meta.url).pathname);
    assert.equal(options.env?.[role.WS_PI_PARENT_SESSION_KEY_ENV], "original-parent");
  }
});
