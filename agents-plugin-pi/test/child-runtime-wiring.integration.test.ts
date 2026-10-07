/**
 * The shipped bridge-to-spawner wiring of the child-runtime ensurer (261007).
 * `local-devenv.test.ts` covers the ensurer with an injected baseline, build
 * and launcher, and `child-launch-guard.test.ts` with a hand-registered
 * preflight; this file drives the production `startBridge` (only the
 * `BridgeOptions.runBuild` seam stands in for `go build`) and the production
 * `registerAgentTools`, then launches children through the registry it
 * returns. That pins what neither unit suite sees: the gate on a startup
 * build, the baseline hashed from the startup `runtime.json` bytes, the
 * ensurer on the returned handle, its registration as the registry's launch
 * preflight, and the real `runLauncherOnce` argv and cwd.
 *
 * The launcher is a fake Python script: `serve --stdio` serves the bridge's
 * MCP startup; any other argv is the one-shot install run, which
 * journals its argv, cwd and bootstrap binary and prints the `plugin_version`
 * of the `runtime.json` in its cwd.
 */
import { afterEach, describe, test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { RpcClient, type ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { startBridge } from "../src/bridge.ts";
import { createAgentStorageContext } from "../src/agent-storage.ts";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import { WS_PI_SPAWN_ROLE_ENV } from "../src/process-role.ts";
import { registerAgentTools, spawnAgent, type RpcAgentRecord } from "../src/spawner.ts";
import { closeFakeChildren, connectFakeChild } from "./fixtures/channel-child.ts";

const PACKAGE_ROOT = dirname(dirname(fileURLToPath(import.meta.url)));
const EXTENSION_ENTRY = join(PACKAGE_ROOT, "src", "index.ts");
const BUNDLED_RUNTIME = readFileSync(join(PACKAGE_ROOT, "runtime.json"), "utf8");
const FORK_CONTEXT = { version: 1, kind: "task", effectiveSystemPrompt: "captured", activeTools: [], registeredTools: [] };

interface InstallRun { argv: string[]; cwd: string; binary: string; binaryExisted: boolean }

const roots: string[] = [];
afterEach(() => { for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true }); });

function writeLauncher(pluginDir: string): { launcher: string; installs: () => InstallRun[] } {
  // Under bin/, as shipped, so the launcher's own directory is not the plugin dir.
  mkdirSync(join(pluginDir, "bin"));
  const launcher = join(pluginDir, "bin", "ws-mcp-launcher.py");
  const journal = join(pluginDir, "install-runs.jsonl");
  writeFileSync(launcher, `import json,os,sys
if sys.argv[1:]!=['serve','--stdio']:
 binary=os.environ.get('WS_MCP_BOOTSTRAP_BINARY','')
 with open(${JSON.stringify(journal)},'a') as f: f.write(json.dumps({'argv':sys.argv,'cwd':os.getcwd(),'binary':binary,'binaryExisted':os.path.isfile(binary)})+'\\n')
 print(json.load(open(os.path.join(os.getcwd(),'runtime.json')))['plugin_version'])
 sys.exit(0)
version=json.load(open(os.path.join(os.getcwd(),'runtime.json')))['plugin_version']
def ok(t): return {'isError':False,'content':[{'type':'text','text':t}]}
for line in sys.stdin:
 q=json.loads(line); m=q['method']
 if m=='initialize': r={'serverInfo':{'version':version},'capabilities':{}}
 elif m=='tools/list': r={'tools':[{'name':n,'description':n,'inputSchema':{'type':'object','properties':{'session_key':{'type':'string'}}}} for n in ['ferrule','workflow_manual','playbook.read']]}
 elif m=='tools/call':
  n=q['params']['name']
  if n=='ferrule': r=ok(json.dumps({'session_key':'wiring-lead'}))
  elif n=='playbook.read': r=ok('# Workflow Manual\\nstatic body')
  else: r=ok('# Workflow Manual\\nstatic body\\n\\n## Session Key\\nwiring-lead')
 else: r={}
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);
  return {
    launcher,
    installs: () => (existsSync(journal) ? readFileSync(journal, "utf8").trim().split("\n").filter(Boolean).map((l) => JSON.parse(l) as InstallRun) : []),
  };
}

/** A real git repo for `source_root`, a `cmd/ws-mcp` tool dir and an executable `go`: the marker validates, and the build seam never runs `go`. */
function writeMarker(root: string, pluginDir: string): void {
  const source = join(root, "source");
  mkdirSync(source);
  execFileSync("git", ["init", "-q"], { cwd: source });
  execFileSync("git", ["-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--allow-empty", "-q", "-m", "init"], { cwd: source });
  const toolDir = join(root, "tool");
  mkdirSync(join(toolDir, "cmd", "ws-mcp"), { recursive: true });
  const go = join(root, "go-stub");
  writeFileSync(go, "#!/bin/sh\nexit 1\n", { mode: 0o755 });
  writeFileSync(join(pluginDir, ".local-devenv-runtime"), JSON.stringify({ schema_version: 1, source_root: source, tool_dir: toolDir, go }));
}

/** RpcClient stand-in: `start()` records how many builds and install runs preceded it, then performs the fake child's hello. */
function installRpcHarness(observe: () => string) {
  const proto = RpcClient.prototype as any;
  const names = ["start", "stop", "abort", "onEvent", "prompt", "getState", "setThinkingLevel"];
  const saved = Object.fromEntries(names.map((name) => [name, proto[name]]));
  const starts: string[] = [];
  Object.assign(proto, {
    async start(this: any) { starts.push(observe()); await connectFakeChild(this.options?.env, this.options?.args); },
    async stop() {}, async abort() {}, async prompt() {}, async setThinkingLevel() {},
    onEvent() { return () => {}; },
    async getState(this: any) {
      const args = this.options?.args ?? [];
      const dir = args[args.indexOf("--session-dir") + 1];
      return { model: { provider: "pi", id: "small" }, thinkingLevel: "medium", sessionFile: `${dir}/session.jsonl`, sessionId: "fork-child-session-id" };
    },
  });
  return { starts, restore() { closeFakeChildren(); Object.assign(proto, saved); } };
}

async function session(opts: { marker: boolean }) {
  const root = realpathSync(mkdtempSync(join(tmpdir(), "ws-pi-child-runtime-wiring-")));
  roots.push(root);
  const pluginDir = join(root, "plugin");
  mkdirSync(pluginDir);
  const runtimeJsonPath = join(pluginDir, "runtime.json");
  writeFileSync(runtimeJsonPath, BUNDLED_RUNTIME);
  if (opts.marker) writeMarker(root, pluginDir);
  const { launcher, installs } = writeLauncher(pluginDir);
  const builds: string[][] = [];
  const runBuild = async (argv: string[]) => {
    builds.push(argv);
    writeFileSync(argv[argv.indexOf("-o") + 1], "fake-ws-mcp", { mode: 0o755 });
  };

  const oldRole = process.env[WS_PI_SPAWN_ROLE_ENV];
  const oldPolicy = process.env[DELEGATION_ENV];
  delete process.env[WS_PI_SPAWN_ROLE_ENV];
  delete process.env[DELEGATION_ENV];
  const pi = { registerTool() {}, on() {}, appendEntry() {}, sendMessage() {}, sendUserMessage() {} } as unknown as ExtensionAPI;
  let bridge: Awaited<ReturnType<typeof startBridge>>;
  try {
    bridge = await startBridge(pi, {
      launcherPath: launcher,
      pluginDir,
      runtimeJsonPath,
      cwd: root,
      toolPreviewTuiRef: { current: undefined },
      ui: { notify() {} } as never,
      runBuild,
    });
  } finally {
    if (oldRole === undefined) delete process.env[WS_PI_SPAWN_ROLE_ENV]; else process.env[WS_PI_SPAWN_ROLE_ENV] = oldRole;
    if (oldPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = oldPolicy;
  }
  const storage = createAgentStorageContext("wiring-lead", root);
  const tools = registerAgentTools(pi, bridge, { cwd: root, storage, extensionPath: EXTENSION_ENTRY });
  const spawnCtx = {
    storage, pi, cwd: root, inheritModel: "pi/small", catalog: [], wsToolNames: [], extensionPath: EXTENSION_ENTRY,
    client: bridge.client, forkFrom: "/tmp/wiring-parent.jsonl", spawnRole: "fork", forkContext: FORK_CONTEXT,
  };
  return {
    pluginDir,
    launcher,
    builds,
    installs,
    spawn: () => spawnAgent(tools.rpcRegistry as Map<string, RpcAgentRecord>, spawnCtx as never, { prompt: "task" }),
    bump: () => {
      const contract = JSON.parse(BUNDLED_RUNTIME) as { plugin_version: string };
      writeFileSync(runtimeJsonPath, JSON.stringify({ ...contract, plugin_version: `${contract.plugin_version}-bumped` }));
    },
    async close() { await tools.stopAll(); bridge.shutdown(); },
  };
}

describe("child-runtime ensurer wiring: startBridge -> registerAgentTools -> child launch", () => {
  test("with the marker: an unchanged runtime.json launches with no rebuild; a mid-session bump rebuilds and installs through the real launcher before the child starts", async () => {
    let s: Awaited<ReturnType<typeof session>> | undefined;
    const rpc = installRpcHarness(() => `builds=${s!.builds.length} installs=${s!.installs().length}`);
    try {
      s = await session({ marker: true });
      assert.equal(s.builds.length, 1, "the startup build ran through the BridgeOptions.runBuild seam");
      assert.equal(s.installs().length, 0, "startup installs through the bridge's own launcher, not a one-shot run");

      await s.spawn();
      assert.deepEqual(rpc.starts, ["builds=1 installs=0"], "the baseline is the startup runtime.json bytes: no drift, no rebuild, no install run");

      s.bump();
      await s.spawn();
      assert.equal(rpc.starts.at(-1), "builds=2 installs=1", "the drift rebuilt and installed before the child started");
      const rebuild = s.builds[1];
      assert.match(rebuild[rebuild.indexOf("-ldflags") + 1], /main\.version=\S+-bumped /, "the rebuild is stamped with the bumped plugin_version");
      const [install] = s.installs();
      assert.deepEqual(install.argv, [s.launcher, "version"], "runLauncherOnce runs `python3 <launcher> version`");
      assert.equal(install.cwd, s.pluginDir, "runLauncherOnce runs in the plugin dir");
      assert.match(install.binary, /\.runtime[/\\]local-devenv[/\\]ws-mcp\./);
      assert.equal(install.binaryExisted, true, "the install run sees the fresh build");

      await s.spawn();
      assert.equal(rpc.starts.at(-1), "builds=2 installs=1", "the settled contract is not rebuilt again");
    } finally {
      await s?.close();
      rpc.restore();
    }
  });

  test("without the marker: no build and no install run precede a child launch, even after a runtime.json change", async () => {
    let s: Awaited<ReturnType<typeof session>> | undefined;
    const rpc = installRpcHarness(() => `builds=${s!.builds.length} installs=${s!.installs().length}`);
    try {
      s = await session({ marker: false });
      await s.spawn();
      s.bump();
      await s.spawn();
      assert.deepEqual(rpc.starts, ["builds=0 installs=0", "builds=0 installs=0"]);
    } finally {
      await s?.close();
      rpc.restore();
    }
  });
});
