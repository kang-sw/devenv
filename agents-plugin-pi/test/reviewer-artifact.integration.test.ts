import assert from "node:assert/strict";
import { afterEach, test } from "node:test";
import { existsSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient, createEventBus, discoverAndLoadExtensions, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { captureOrphans, parseOrphans, rehydrateOrphanRecord, serializeOrphans } from "../src/agent-sidecar.ts";
import { DELEGATION_ENV, playbookProfile, type DelegationPolicy } from "../src/delegation-policy.ts";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { resolveTools, spawnAdmission, spawnAgent, stopAgent, type RpcAgentRegistry } from "../src/spawner.ts";

const roots: string[] = [];
const originalRpc = Object.fromEntries(
  ["start", "stop", "abort", "onEvent", "prompt", "getState", "getSessionStats", "setThinkingLevel"].map(
    name => [name, RpcClient.prototype[name as keyof RpcClient]],
  ),
);

afterEach(() => {
  Object.assign(RpcClient.prototype, originalRpc);
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

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

test("spawned reviewers publish clean and non-clean artifacts through one immutable binding", async () => {
  Object.assign(RpcClient.prototype, {
    start: async () => {},
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
