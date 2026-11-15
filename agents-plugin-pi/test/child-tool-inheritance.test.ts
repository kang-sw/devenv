import { test } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { startBridge, sanitizeToolName, type BridgeHandle } from "../src/bridge.ts";
import { childPolicy, DELEGATION_ENV, type DelegationPolicy } from "../src/delegation-policy.ts";
import { resolveTools, spawnAdmission } from "../src/spawner.ts";

const packageRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const ownerTools = ["ws__session_request_hire", "ws__session_roster"];
const rawWorkerTools = ["ferrule", "playbook.read", "playbook.render", "git.status", "session.children"];
const wsTools = [...rawWorkerTools.map(sanitizeToolName), ...ownerTools];
const root: DelegationPolicy = { version: 1, depth: 0, maxDepth: 2, authority: "lead", tools: wsTools };
const withoutOwnerTools = (tools: readonly string[]) => tools.filter(tool => !ownerTools.includes(tool));

function assertNoOwnerTools(tools: readonly string[]): void {
  for (const tool of ownerTools) assert.ok(!tools.includes(tool), `${tool} must not be inherited`);
  assert.ok(tools.includes("ws__session_children"), "ordinary session reads remain inheritable");
}

test("child capability derivation excludes owner-lead tools even from explicit requests", () => {
  const requested = [...resolveTools("full-worker", wsTools).split(","), ...ownerTools];
  const first = childPolicy(root, requested, "lead");
  assertNoOwnerTools(first.tools);
  const nested = childPolicy(first, requested, "delegate");
  assertNoOwnerTools(nested.tools);
  assert.equal(nested.depth, 2);
  assert.throws(() => childPolicy(first, [...requested, "ws__git_merge"], "delegate"), /child capability exceeds parent ceiling \(ws__git_merge\)/);
  assert.deepEqual(root.tools, wsTools, "the root's tool surface is not narrowed or widened");
});

test("full-worker and fork admission never re-add owner-lead tools", () => {
  assertNoOwnerTools(resolveTools("full-worker", wsTools).split(","));
  for (const spawnRole of ["worker", "fork"] as const) {
    const policy = spawnAdmission({ parentPolicy: root, wsToolNames: wsTools, toolGroup: "full-worker", spawnRole,
      explicitTools: [...resolveTools("full-worker", wsTools).split(","), ...ownerTools].join(",") } as never);
    assertNoOwnerTools(policy.tools);
  }
});

test("nested delegate-implementer render admits a child without inheriting hire or roster", async () => {
  // The parent envelope predates the two lead tools, but the child's bridge
  // discovers the current server inventory. Exercise the actual render preflight
  // with a disposable stdio fixture; never start or contact a dashboard daemon.
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-child-tools-"));
  const runtimeJsonPath = join(packageRoot, "runtime.json");
  const { plugin_version: version } = JSON.parse(readFileSync(runtimeJsonPath, "utf8"));
  const relative = "delegate-implementer/delegate-implementer.md";
  const source = readFileSync(join(packageRoot, "rsrc", relative));
  mkdirSync(join(directory, "rsrc", "delegate-implementer"), { recursive: true });
  writeFileSync(join(directory, "rsrc", relative), source);
  writeFileSync(join(directory, "rsrc", "manifest.json"), JSON.stringify({ files: { [relative]: createHash("sha256").update(source).digest("hex") } }));
  const promptPath = join(directory, "rendered.md");
  writeFileSync(promptPath, "**Your ws session_key: `grandchild-key`**\nImplement the bounded task.\n");
  const callsPath = join(directory, "calls.jsonl");
  const launcherPath = join(directory, "launcher.py");
  writeFileSync(launcherPath, `import json,sys
names=${JSON.stringify([...rawWorkerTools, "session.request_hire", "session.roster"])}
for line in sys.stdin:
 q=json.loads(line); m=q['method']
 if m=='initialize': r={'serverInfo':{'version':${JSON.stringify(version)}},'capabilities':{}}
 elif m=='tools/list': r={'tools':[{'name':n,'inputSchema':{'type':'object','properties':{}}} for n in names]}
 elif m=='tools/call':
  with open(${JSON.stringify(callsPath)},'a') as f: f.write(json.dumps(q['params'])+'\\n')
  text=${JSON.stringify(promptPath)} if q['params']['name']=='playbook.render' else 'fixture read'
  r={'content':[{'type':'text','text':text}]}
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);
  const parent = { ...childPolicy(root, withoutOwnerTools(resolveTools("full-worker", wsTools).split(",")), "lead"), sessionKey: "worker-key" };
  const saved = { policy: process.env[DELEGATION_ENV], role: process.env.WS_PI_SPAWN_ROLE };
  const registered = new Map<string, any>();
  let bridge: BridgeHandle | undefined;
  try {
    process.env[DELEGATION_ENV] = JSON.stringify(parent);
    process.env.WS_PI_SPAWN_ROLE = "worker";
    bridge = await startBridge({ registerTool: (tool: any) => registered.set(tool.name, tool), on() {}, appendEntry() {} } as never,
      { pluginDir: directory, launcherPath, runtimeJsonPath, cwd: directory, toolPreviewTuiRef: { current: undefined }, ui: { notify() {} } as never,
        runBuild: async () => { throw new Error("fixture must not build a runtime"); } });
    for (const tool of ownerTools) {
      assert.ok(bridge.wsToolNames.includes(tool), "the current inventory exposes the new owner tool to render derivation");
      await assert.rejects(() => registered.get(tool).execute("denied", {}), /exceeds this agent's capability ceiling/);
    }
    await registered.get("ws__playbook_render").execute("render", { name: "delegate-implementer" });
    const provenance = bridge.renderRegistry?.get(promptPath);
    assert.ok(provenance, "successful nested render records trusted provenance");
    const grandchild = spawnAdmission({ parentPolicy: parent, wsToolNames: bridge.wsToolNames, toolGroup: "full-worker", spawnRole: "worker", provenance } as never);
    assert.equal(grandchild.depth, 2);
    assertNoOwnerTools(grandchild.tools);
    assertNoOwnerTools(parent.tools);
    const calls = readFileSync(callsPath, "utf8").trim().split("\n").map(line => JSON.parse(line));
    assert.deepEqual(calls, [{ name: "playbook.render", arguments: { name: "delegate-implementer", session_key: "worker-key" } }], "only the admitted render reaches the fixture server");
  } finally {
    bridge?.shutdown();
    if (saved.policy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = saved.policy;
    if (saved.role === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = saved.role;
    rmSync(directory, { recursive: true, force: true });
  }
});
