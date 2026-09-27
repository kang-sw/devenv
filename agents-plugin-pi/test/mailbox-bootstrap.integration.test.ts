import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } from "@earendil-works/pi-coding-agent";

const PLUGIN_DIR = join(process.cwd());

async function waitForFile(path: string): Promise<void> {
  for (let i = 0; i < 200; i += 1) {
    if (existsSync(path)) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`mailbox waiter never started: ${path}`);
}

test("real Pi session startup hands its source-built bridge binary to an uncached mailbox wait", async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-mailbox-handoff-"));
  const plugin = join(directory, "plugin");
  mkdirSync(plugin);
  for (const name of ["src", "runtime.json", "goal-loop-config.json", "pi-lead-guide.md", "execute-worker-guide.md", "explore-guide.md"]) {
    cpSync(join(PLUGIN_DIR, name), join(plugin, name), { recursive: true });
  }
  symlinkSync(join(PLUGIN_DIR, "node_modules"), join(plugin, "node_modules"), process.platform === "win32" ? "junction" : "dir");
  // Stub only the Go build seam in the copied bridge; the session_start,
  // startBridge, handle and mailbox waiter paths remain production code.
  const bridgePath = join(plugin, "src", "bridge.ts");
  const bridgeSource = readFileSync(bridgePath, "utf8");
  assert.ok(bridgeSource.includes("runBuild: runGoBuild,"));
  writeFileSync(bridgePath, bridgeSource.replace("runBuild: runGoBuild,", "runBuild: async (argv) => { await (await import('node:fs/promises')).writeFile(argv[argv.indexOf('-o') + 1], 'source-built'); },"));
  const bin = join(plugin, "bin");
  mkdirSync(bin);
  const observed = join(directory, "observed-wait-binary");
  const launcher = join(bin, "ws-mcp-launcher.py");
  const runtimeVersion = JSON.parse(readFileSync(join(plugin, "runtime.json"), "utf8")).plugin_version;
  // Deliberately no runtime cache: this launcher accepts only the explicit
  // source-built override and cannot fall back to a release asset or stamp.
  writeFileSync(launcher, [
    "import json,os,sys,time",
    "binary=os.environ.get('WS_MCP_BOOTSTRAP_BINARY', '')",
    "if not binary or not os.path.isfile(binary): raise SystemExit('source-built binary missing')",
    "if sys.argv[1:3]==['mailbox','wait']:",
    ` open(${JSON.stringify(observed)}, 'w').write(binary)`,
    " time.sleep(30)",
    " raise SystemExit(3)",
    "for line in sys.stdin:",
    " q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}",
    ` if m=='initialize': r={'serverInfo':{'version':${JSON.stringify(runtimeVersion)}},'capabilities':{}}`,
    " elif m=='tools/list': r={'tools':[]}",
    " elif m=='tools/call':",
    "  name=p['name']; text=json.dumps({'session_key':'mailbox-test-key'}) if name=='ferrule' else ('# Manual\\n## Session Key\\nmailbox-test-key' if name=='workflow_manual' else ('# Manual' if name=='playbook.read' else '{}'))",
    "  r={'content':[{'type':'text','text':text}],'isError':False}",
    " print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)",
    "",
  ].join("\n"));
  const root = join(PLUGIN_DIR, "..");
  writeFileSync(join(plugin, ".local-devenv-runtime"), JSON.stringify({
    schema_version: 1,
    source_root: root,
    tool_dir: join(root, "agents-plugin-tool"),
    go: process.execPath,
  }));
  const runtimeCache = join(directory, "uncached-runtime");
  mkdirSync(runtimeCache);
  const prior = { ...process.env };
  let session: Awaited<ReturnType<typeof createAgentSession>>["session"] | undefined;
  try {
    delete process.env.WS_PI_SPAWN_ROLE;
    delete process.env.WS_MCP_BOOTSTRAP_BINARY;
    delete process.env.WS_MCP_BOOTSTRAP_URL;
    process.env.WS_MCP_RUNTIME_DIR = runtimeCache;
    process.env.PI_OFFLINE = "1";
    const settings = SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } });
    const agentDir = join(directory, "agent");
    mkdirSync(agentDir);
    const modelRuntime = await ModelRuntime.create({ authPath: join(agentDir, "auth.json"), modelsPath: join(agentDir, "models.json"), modelsStorePath: join(agentDir, "store.json"), allowModelNetwork: false });
    const loader = new DefaultResourceLoader({ cwd: root, agentDir, settingsManager: settings, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true, additionalExtensionPaths: [join(plugin, "src/index.ts")] });
    await loader.reload();
    assert.deepEqual(loader.getExtensions().errors, []);
    ({ session } = await createAgentSession({ cwd: root, agentDir, sessionManager: SessionManager.create(root, join(directory, "sessions")), resourceLoader: loader, modelRuntime, settingsManager: settings }));
    const ui = new Proxy({ notify: () => {}, setFooter: () => {} }, { get: (target: Record<string, unknown>, key: string) => target[key] ?? (() => {}) });
    await session.bindExtensions({ mode: "tui", uiContext: ui as never, onError: (error) => { throw error; } });
    await waitForFile(observed);
    const binary = readFileSync(observed, "utf8");
    assert.equal(binary, join(plugin, ".runtime", "local-devenv", "ws-mcp"));
    assert.equal(readFileSync(binary, "utf8"), "source-built");
    assert.deepEqual(readdirSync(runtimeCache), [], "a primed launcher cache cannot mask the missing handoff");
  } finally {
    if (session) {
      await session.extensionRunner?.emit({ type: "session_shutdown", reason: "quit" });
      session.dispose();
    }
    for (const key of Object.keys(process.env)) if (!(key in prior)) delete process.env[key];
    Object.assign(process.env, prior);
    rmSync(directory, { recursive: true, force: true });
  }
});
