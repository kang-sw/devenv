import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient, createEventBus, discoverAndLoadExtensions, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";
import { DELEGATION_ENV, RenderRegistry, playbookProfile, type DelegationPolicy } from "../src/delegation-policy.ts";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { registerAgentTools, resolveTools, spawnAdmission, spawnAgent, stopAgent, type RpcAgentRegistry } from "../src/spawner.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";

const roots: string[] = [];
const originalRpc = Object.fromEntries(
  ["start", "stop", "abort", "onEvent", "prompt", "getState", "getSessionStats", "setThinkingLevel"].map(
    name => [name, RpcClient.prototype[name as keyof RpcClient]],
  ),
);

afterEach(() => {
  closeFakeChildren();
  Object.assign(RpcClient.prototype, originalRpc);
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

async function startWithFakeChild(this: { options?: { env?: Record<string, string>; args?: string[] } }) {
  await connectFakeChild(this.options?.env, this.options?.args);
}

function tempRoot(): string {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-review-artifact-")));
  roots.push(root);
  return root;
}

const pluginDir = dirname(fileURLToPath(new URL("../package.json", import.meta.url)));
const wsTools = ["ws__git_diff", "ws__git_log", "ws__git_status"];
function workerPolicy(): DelegationPolicy {
  return {
    version: 1,
    depth: 1,
    maxDepth: 2,
    authority: "lead",
    tools: resolveTools("full-worker", wsTools).split(","),
    network: { search: true, fetch: true },
    write: { mode: "unrestricted" },
  };
}

function reviewerContext(root: string) {
  return {
    profile: playbookProfile(pluginDir, "reviewer"),
    parentPolicy: workerPolicy(),
    pi: { sendMessage() {} } as unknown as ExtensionAPI,
    cwd: root,
    extensionPath: fileURLToPath(new URL("../src/index.ts", import.meta.url)),
    storage: createAgentStorageContext("worker-session", join(root, "agent-home")),
    inheritModel: "offline/reviewer",
    catalog: [{ provider: "offline", id: "reviewer", hasAuth: true }],
    wsToolNames: wsTools,
    client: { callTool: async () => { throw new Error("tier lookup is not expected"); } } as never,
  };
}

test("code-review provenance requires exactly one exact-file write scope", () => {
  const root = tempRoot();
  const context = reviewerContext(root);
  const first = join(root, "first.md");
  const second = join(root, "second.md");

  assert.equal(context.profile.reviewArtifact, true);
  assert.equal(playbookProfile(pluginDir, "code-review-correctness").reviewArtifact, true);
  assert.equal(playbookProfile(pluginDir, "code-review-fit").reviewArtifact, true);
  assert.equal(playbookProfile(pluginDir, "code-review-test").reviewArtifact, true);
  assert.equal(playbookProfile(pluginDir, "ticket-reviewer-design").reviewArtifact, false);
  assert.throws(() => spawnAdmission(context), /code reviewer requires exactly one file write scope/);
  assert.throws(
    () => spawnAdmission(context, [{ path: first, kind: "file" }, { path: second, kind: "file" }]),
    /code reviewer requires exactly one file write scope/,
  );
  assert.throws(
    () => spawnAdmission(context, [{ path: root, kind: "tree", include: ["**\/*.md"] }]),
    /code reviewer requires exactly one file write scope/,
  );

  const admitted = spawnAdmission(context, [{ path: first, kind: "file" }]);
  assert.deepEqual(admitted.write, { mode: "scoped", scopes: [{ path: first, kind: "file" }] });
  assert.equal(admitted.tools.includes("bash"), false);
  assert.equal(admitted.tools.includes("edit"), true);
  assert.equal(admitted.tools.includes("write"), true);
});

test("registered nested reviewer dispatch launches verified bytes across async model resolution", async () => {
  Object.assign(RpcClient.prototype, {
    start: startWithFakeChild, stop: async () => {}, abort: async () => {},
    onEvent: () => () => {}, prompt: async () => {}, setThinkingLevel: async () => {},
    getState: async () => ({ sessionFile: "/tmp/offline-reviewer.jsonl", model: { provider: "offline", id: "reviewer" } }),
    getSessionStats: async () => { throw new Error("offline"); },
  });
  const root = tempRoot();
  const context = reviewerContext(root);
  const previousPolicy = process.env[DELEGATION_ENV];
  process.env[DELEGATION_ENV] = JSON.stringify(workerPolicy());
  const tools = new Map<string, any>();
  const pi = { registerTool: (tool: any) => tools.set(tool.name, tool), sendMessage() {} } as unknown as ExtensionAPI;
  const renders = new RenderRegistry();
  let modelCalls = 0;
  let entered = Promise.withResolvers<void>();
  let resume = Promise.withResolvers<void>();
  const bridge = {
    wsToolNames: wsTools, renderRegistry: renders, defaultSessionKeyRef: { current: "worker-key" },
    client: { callTool: async (name: string) => {
      assert.equal(name, "config.resolve_agent");
      modelCalls++;
      entered.resolve();
      await resume.promise;
      return { content: [{ type: "text", text: JSON.stringify({ resolved_from: "pi", model: "offline/reviewer" }) }] };
    } },
  };
  const handle = registerAgentTools(pi, bridge as never, context);
  const tool = tools.get("ws-agent-spawn");
  const toolCtx = {
    model: { provider: "offline", id: "reviewer" },
    modelRegistry: { getAll: () => [{ provider: "offline", id: "reviewer" }], hasConfiguredAuth: () => true },
  };
  try {
    for (const name of ["reviewer", "code-review-correctness", "code-review-fit", "code-review-test"]) {
      const path = join(root, `${name}-prompt.md`);
      // Include non-ASCII and a non-UTF8 byte to pin byte identity, not just decoded text.
      const verified = Buffer.concat([Buffer.from(`**Your ws session_key: \`child-${name}\`**\nTrusted review — ${name}\n`), Buffer.from([0xff])]);
      writeFileSync(path, verified);
      renders.record(path, playbookProfile(pluginDir, name));
      const findings = join(root, `${name}-findings.md`);
      const args = { system_prompt_path: path, prompt: "Review", model_name: "small", write_scopes: [{ path: findings, kind: "file" }] };
      const call = (input = args) => tool.execute("spawn", input, undefined, undefined, toolCtx);
      const callsBefore = modelCalls;
      const homesBefore = handle.rpcRegistry.size;

      const copied = join(root, `${name}-copied.md`);
      writeFileSync(copied, verified);
      await assert.rejects(call({ ...args, system_prompt_path: copied }), /requires trusted render provenance/);
      writeFileSync(path, "Changed before admission");
      await assert.rejects(call(), /rendered prompt changed since authorization/);
      writeFileSync(path, verified);
      for (const scopes of [undefined, [{ path: root, kind: "tree" }], [{ path: findings, kind: "file" }, { path: copied, kind: "file" }], [{ path: findings, kind: "file", include: ["*.md"] }]]) {
        await assert.rejects(call({ ...args, write_scopes: scopes } as never), /code reviewer requires exactly one file write scope|file scopes do not accept include patterns/);
      }
      assert.equal(modelCalls, callsBefore, "untrusted prompts and invalid grants fail before model resolution");
      assert.equal(handle.rpcRegistry.size, homesBefore, "rejections allocate no child");

      entered = Promise.withResolvers<void>();
      resume = Promise.withResolvers<void>();
      const pending = call();
      await entered.promise;
      writeFileSync(path, "Changed after admission; must never execute");
      resume.resolve();
      const result = JSON.parse((await pending).content[0].text);
      const child = handle.rpcRegistry.get(result.agent_id)!;
      assert.notEqual(child.systemPromptPath, path);
      const launched = readFileSync(child.systemPromptPath!);
      assert.deepEqual(launched.subarray(0, verified.length), verified);
      assert.match(launched.subarray(verified.length).toString(), /^\n\n## Persistent delegation/);
      assert.equal(launched.includes(Buffer.from("Changed after admission")), false);
      assert.equal(child.delegation?.sessionKey, `child-${name}`);
      assert.deepEqual(child.delegation?.write, { mode: "scoped", scopes: [{ path: findings, kind: "file" }] });
      assert.equal(child.delegation?.tools.includes("bash"), false);
    }
    assert.throws(() => playbookProfile(pluginDir, "code-reviewer"), /lacks trusted shipped provenance/);
  } finally {
    resume.resolve();
    await handle.stopAll();
    if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
  }
});

test("restored provenance revalidates disk and builds a fresh immutable snapshot", () => {
  const root = tempRoot();
  const path = join(root, "prompt.md");
  writeFileSync(path, "Trusted prompt");
  const renders = new RenderRegistry();
  const metadata = renders.record(path, playbookProfile(pluginDir, "reviewer"));
  assert.equal("promptBase64" in metadata, false, "snapshots are not persisted with render metadata");
  const restored = new RenderRegistry();
  restored.restore([{ ...metadata, promptBase64: Buffer.from("Untrusted persisted snapshot").toString("base64") }]);
  const admitted = restored.get(path)!;
  assert.ok(Object.isFrozen(admitted));
  assert.equal(Buffer.from(admitted.promptBase64, "base64").toString(), "Trusted prompt");
  assert.equal("promptBase64" in restored.values()[0]!, false);
  writeFileSync(path, "Edited cached prompt");
  assert.throws(() => restored.get(path), /changed since authorization/);
  const stale = new RenderRegistry();
  stale.restore([metadata]);
  assert.equal(stale.get(path), undefined);
});

test("spawned reviewers publish clean and non-clean artifacts through one immutable binding", async () => {
  Object.assign(RpcClient.prototype, {
    start: startWithFakeChild,
    stop: async () => {},
    abort: async () => {},
    onEvent: () => () => {},
    prompt: async () => {},
    getState: async () => ({
      sessionFile: "/tmp/offline-reviewer.jsonl",
      sessionId: "offline-reviewer",
      model: { provider: "offline", id: "reviewer" },
      thinkingLevel: "off",
    }),
    getSessionStats: async () => { throw new Error("offline"); },
    setThinkingLevel: async () => {},
  });

  const previousPolicy = process.env[DELEGATION_ENV];
  const previousRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  const root = tempRoot();
  const systemPromptPath = join(root, "reviewer-prompt.md");
  writeFileSync(systemPromptPath, "Offline code reviewer\n");
  const registry: RpcAgentRegistry = new Map();
  const context = reviewerContext(root);
  const reports = [
    { path: join(root, "clean.md"), content: "## Review findings: clean\nNo findings.\n" },
    { path: join(root, "non-clean.md"), content: "## Review findings: non-clean\n### Important\n- defect\n" },
  ];
  const loadChildTools = async (policy: DelegationPolicy, suffix: string) => {
    process.env[WS_PI_SPAWN_ROLE_ENV] = "worker";
    process.env[DELEGATION_ENV] = JSON.stringify(policy);
    const loaded = await discoverAndLoadExtensions(
      [context.extensionPath],
      root,
      join(root, `child-agent-${suffix}`),
      createEventBus(),
    );
    assert.deepEqual(loaded.errors, []);
    const extension = loaded.extensions.find(candidate => candidate.resolvedPath === context.extensionPath);
    assert.ok(extension, "the spawned child loads the real Pi adapter entry");
    assert.equal(extension.tools.has("bash"), false, "reviewer initialization does not add Bash");
    const edit = extension.tools.get("edit")?.definition;
    const write = extension.tools.get("write")?.definition;
    assert.ok(edit?.execute && write?.execute, "reviewer initialization installs the same-name scoped wrappers");
    return { edit, write };
  };

  try {
    for (const [index, report] of reports.entries()) {
      const result = await spawnAgent(registry, context, {
        systemPromptPath,
        prompt: `Review and publish ${index}`,
        writeScopes: [{ path: report.path, kind: "file" }],
      });
      const record = registry.get(result.agent_id)!;
      assert.match(readFileSync(record.systemPromptPath!, "utf8"), /write_scopes/);
      assert.equal(record.delegation?.tools.includes("bash"), false);
      assert.deepEqual(record.delegation?.write, { mode: "scoped", scopes: [{ path: report.path, kind: "file" }] });

      const tools = await loadChildTools(record.delegation!, String(index));
      await tools.write.execute("publish", { path: report.path, content: report.content }, undefined, undefined, { cwd: root } as never);
      assert.equal(readFileSync(report.path, "utf8"), report.content, "the worker consumes the complete child-published report");
      await assert.rejects(
        tools.write.execute("second", { path: join(root, `unbound-${index}.md`), content: "escape" }, undefined, undefined, { cwd: root } as never),
        /outside delegated write scopes/,
      );
      assert.equal(existsSync(join(root, `unbound-${index}.md`)), false);
    }

    const first = registry.values().next().value!;
    const [restored] = parseOrphans(serializeOrphans(captureOrphans(new Map([[first.agentId, first]]))));
    const revived = rehydrateOrphanRecord(restored);
    assert.deepEqual(revived.delegation?.write, first.delegation?.write, "reload/resume keeps the exact file binding immutable");
    const revivedTools = await loadChildTools(revived.delegation!, "revived");
    await revivedTools.edit.execute(
      "resume-edit",
      { path: reports[0]!.path, edits: [{ oldText: "No findings.", newText: "No findings after resume." }] },
      undefined,
      undefined,
      { cwd: root } as never,
    );
    assert.match(readFileSync(reports[0]!.path, "utf8"), /after resume/);
    await assert.rejects(
      revivedTools.write.execute("resume-escape", { path: reports[1]!.path, content: "cross-artifact" }, undefined, undefined, { cwd: root } as never),
      /outside delegated write scopes/,
    );
  } finally {
    for (const id of [...registry.keys()]) await stopAgent(registry, id, context.pi, { silent: true });
    if (previousPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = previousPolicy;
    if (previousRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = previousRole;
  }
});
