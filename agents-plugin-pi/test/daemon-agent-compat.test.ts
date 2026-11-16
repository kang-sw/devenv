import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { McpStdioClient, type McpToolCallResult } from "../src/mcp-stdio-client.ts";
import {
  DAEMON_AGENT_CAPABILITIES, daemonAgentInitializeCapabilities, PI_AGENT_COMPAT_GUIDE, adaptLegacyPiGuidance,
  adaptPiGuidanceContent, adaptPiPromptArtifact, callPiWithExploreFallback, isReadOnlyFallbackProfile,
} from "../src/daemon-agent-compat.ts";

const text = (value: string, isError = false): McpToolCallResult => ({ content: [{ type: "text", text: value }], ...(isError ? { isError } : {}) });
const unknown = text('pi.spawn: unknown agent profile "explore" for D:/repo; known: pi-worker, search-answer-pi', true);
const profile = { backend: "pi", tools: ["read", "grep", "find", "ls"] };
const args = { session_key: "own-key", profile: "explore", tier: "small", prompt: "Find local evidence; report paths and gaps", cwd: "subdir", label: "evidence" };
const config = (value: unknown) => text(JSON.stringify({ key: "agent.profile.search-answer-pi", value, scope: "builtin" }));

test("initialize capabilities default to {}, opt-in is standard experimental metadata; legacy response needs no echo", async () => {
  const sent: Array<{ method: string; params: any }> = [];
  // Request seam only: no McpStdioClient construction, process, launcher or transport.
  const receiver = { request: async (method: string, params: any) => {
    sent.push({ method, params });
    // A legacy daemon's normal initialize response ignores unknown capabilities.
    return { protocolVersion: params.protocolVersion, capabilities: {}, serverInfo: { name: "ws-mcp", version: "0.50.1" } };
  } };
  const initialize = McpStdioClient.prototype.initialize;
  const legacy = await initialize.call(receiver as never, { name: "ws-pi-bridge", version: "0.1.0" });
  const marked = await initialize.call(receiver as never, { name: "ws-pi-bridge", version: "0.1.0" }, DAEMON_AGENT_CAPABILITIES);
  assert.deepEqual(sent[0].params.capabilities, {});
  assert.deepEqual(sent[1].params.capabilities, { experimental: { wsPiDaemonAgents: true } });
  assert.equal(sent[1].method, "initialize");
  assert.deepEqual(marked, legacy);
});

test("only an owner lead without a curated delegation policy opts into daemon terminology", () => {
  assert.deepEqual(daemonAgentInitializeCapabilities(undefined, false), DAEMON_AGENT_CAPABILITIES);
  // Native execution workers are worker-role with a curated policy; neither a
  // missing role nor a missing policy alone may accidentally opt them in.
  for (const [role, hasPolicy] of [["worker", true], ["worker", false], [undefined, true], ["explore", true], ["fork", true]] as const) {
    assert.deepEqual(daemonAgentInitializeCapabilities(role, hasPolicy), {}, `${role}/${hasPolicy}`);
  }
});

test("legacy guidance carries explicit argument/lifecycle translation, while current prose is unchanged", () => {
  const original = "Use ws-agent-spawn and ws-agent-send to the same alias or agent id. Ask the `explore` researcher.";
  const adapted = adaptLegacyPiGuidance(original);
  assert.ok(adapted.startsWith(original));
  assert.ok(adapted.includes(PI_AGENT_COMPAT_GUIDE));
  assert.match(adapted, /write_scopes are not pi.spawn arguments/);
  assert.match(adapted, /if pi\.\* is unavailable/);
  assert.match(adapted, /ws-fork is retired/);
  assert.equal(adaptLegacyPiGuidance(adapted), adapted);
  assert.equal(adaptLegacyPiGuidance("Use ws/pi.spawn at tier small."), "Use ws/pi.spawn at tier small.");
  const image = { type: "image", data: "unchanged" };
  assert.deepEqual(adaptPiGuidanceContent([image, { type: "text", text: original }]), [image, { type: "text", text: adapted }]);
});

test("rendered artifact itself receives translation once, preserving key and prompt prefix", () => {
  const dir = mkdtempSync(join(tmpdir(), "pi-compat-"));
  try {
    const path = join(dir, "prompt.md");
    const original = '**Your ws session_key: `child-key`**\n\nContinue with ws-agent-send.\n';
    writeFileSync(path, original);
    adaptPiPromptArtifact(path);
    const adapted = readFileSync(path, "utf8");
    assert.ok(adapted.startsWith(original));
    assert.ok(adapted.includes(PI_AGENT_COMPAT_GUIDE));
    adaptPiPromptArtifact(path);
    assert.equal(readFileSync(path, "utf8"), adapted);
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test("effective fallback safety rejects whole-profile overrides and unknown capabilities", () => {
  assert.equal(isReadOnlyFallbackProfile(profile), true);
  assert.equal(isReadOnlyFallbackProfile({ ...profile, tools: ["read"] }), true);
  for (const change of [
    { backend: "claude" }, { tools: ["read", "write"] }, { tools: ["bash"] }, { tools: ["web_search"] },
    { tools: "read" }, { tools: [] }, { ws_tools: ["pi.spawn"] }, { ws_tools: "*" },
    { mcp_servers: { web: {} } }, { mcp_servers: [] }, { plugins: ["network.ts"] },
    { skills: true }, { lead: true }, { permission_mode: "bypassPermissions" }, { future_capability: true },
  ]) assert.equal(isReadOnlyFallbackProfile({ ...profile, ...change }), false, JSON.stringify(change));
  assert.equal(isReadOnlyFallbackProfile(null), false);
});

test("exact unknown-explore refusal verifies effective config before retrying, retaining tier and brief", async () => {
  const calls: Array<[string, Record<string, unknown>]> = [];
  const done = text('{"id":"daemon-child"}');
  const result = await callPiWithExploreFallback(async (name, input) => {
    calls.push([name, input]);
    return calls.length === 1 ? unknown : calls.length === 2 ? config(profile) : done;
  }, "pi.spawn", args);
  assert.equal(result, done);
  assert.deepEqual(calls, [
    ["pi.spawn", args],
    ["config.get", { key: "agent.profile.search-answer-pi", format: "json", session_key: "own-key" }],
    ["pi.spawn", { ...args, profile: "search-answer-pi" }],
  ]);
});

test("fallback failure is not retried again, including unknown outcomes", async () => {
  for (const failWithThrow of [false, true]) {
    let calls = 0;
    const failure = text("fallback launch failed", true);
    const pending = callPiWithExploreFallback(async () => {
      if (++calls === 1) return unknown;
      if (calls === 2) return config(profile);
      if (failWithThrow) throw new Error("fallback unknown outcome");
      return failure;
    }, "pi.spawn", args);
    if (failWithThrow) await assert.rejects(pending, /fallback unknown outcome/);
    else assert.equal(await pending, failure);
    assert.equal(calls, 3);
  }
});

test("unverifiable or unsafe effective fallback refuses without a second spawn", async () => {
  for (const response of [config({ ...profile, tools: ["write"] }), config({ ...profile, plugins: ["web.ts"] }), config({ ...profile, mcp_servers: { web: {} } }), config(null), text("not json"), text("config denied", true)]) {
    let calls = 0;
    const result = await callPiWithExploreFallback(async () => ++calls === 1 ? unknown : response, "pi.spawn", args);
    assert.equal(calls, 2);
    assert.equal(result.isError, true);
    assert.match(result.content[0].text!, /refusing fallback/);
  }
  let calls = 0;
  const result = await callPiWithExploreFallback(async () => {
    if (++calls === 1) return unknown;
    throw new Error("config transport failed");
  }, "pi.spawn", args);
  assert.equal(calls, 2);
  assert.equal(result.isError, true);
});

test("all other spawn failures and unknown outcomes are returned without retry", async () => {
  for (const failure of [
    text("pi.spawn: dashboard.pi_spawn_enabled is off", true),
    text("pi.spawn: unset tier small", true), text('pi.spawn: invalid profile "explore"', true),
    text('pi.spawn: unknown agent profile "other" for D:/repo', true),
    text('pi.spawn: child profile "explore" exceeds parent', true),
    text("launch failed", true), text("normal success"),
  ]) {
    let calls = 0;
    assert.equal(await callPiWithExploreFallback(async () => { calls++; return failure; }, "pi.spawn", args), failure);
    assert.equal(calls, 1);
  }
  for (const [name, input] of [["pi.send", args], ["pi.spawn", { ...args, profile: "other" }]] as const) {
    let calls = 0;
    assert.equal(await callPiWithExploreFallback(async () => { calls++; return unknown; }, name, input), unknown);
    assert.equal(calls, 1);
  }
  let calls = 0;
  await assert.rejects(callPiWithExploreFallback(async () => { calls++; throw new Error("unknown outcome"); }, "pi.spawn", args), /unknown outcome/);
  assert.equal(calls, 1);
});

test("guides describe daemon transport separately from retained execution approval", () => {
  const guide = readFileSync(new URL("../pi-lead-guide.md", import.meta.url), "utf8");
  for (const tool of ["spawn", "send", "list", "stop", "transcript", "wait"]) assert.ok(guide.includes(`ws/pi.${tool}`));
  assert.match(guide, /profile `explore` at tier\n`small`/);
  assert.match(guide, /not daemon children/);
  assert.doesNotMatch(guide, /ws-agent-(?:spawn|send|list|stop|transcript)|ws-fork/);
  const execution = readFileSync(new URL("../execute-worker-guide.md", import.meta.url), "utf8");
  assert.match(execution, /ws-worker-exec/);
  assert.match(execution, /ws-approve/);
  assert.doesNotMatch(execution, /`explore`|ws-agent-stop|ws-agent-spawn/);
  const compact = readFileSync(new URL("../lead-compact-guide.md", import.meta.url), "utf8");
  assert.match(compact, /daemon Pi child ids/);
});

test("bridge adapts artifacts before provenance and opts in without registering legacy aliases", () => {
  const bridge = readFileSync(new URL("../src/bridge.ts", import.meta.url), "utf8");
  assert.match(bridge, /}, daemonAgentInitializeCapabilities\(readSpawnRole\(process.env\), Boolean\(policy\)\)\)/);
  assert.ok(bridge.indexOf("adaptPiPromptArtifact(path)") < bridge.indexOf("renderRegistry.record(path"));
  assert.match(bridge, /callPiWithExploreFallback\(/);
  assert.doesNotMatch(bridge, /name: "ws-agent-(spawn|send|list|stop|transcript)"/);
  const bootstrap = readFileSync(new URL("../src/lead-bootstrap.ts", import.meta.url), "utf8");
  assert.match(bootstrap, /\$\{PI_AGENT_COMPAT_GUIDE\}/);
  assert.doesNotMatch(bootstrap, /addForkToolIfLead/);
});
