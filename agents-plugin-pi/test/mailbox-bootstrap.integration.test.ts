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

test("Pi session_start stages one immutable mailbox binary across re-arms and runtime.json mutation", { skip: process.platform === "win32" && "the Python shebang fixture needs POSIX; Windows uses the opt-in real-Go smoke" }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-mailbox-handoff-"));
  const plugin = join(directory, "plugin");
  mkdirSync(plugin);
  for (const name of ["src", "runtime.json", "goal-loop-config.json", "pi-lead-guide.md", "execute-worker-guide.md", "explore-guide.md"]) {
    cpSync(join(PLUGIN_DIR, name), join(plugin, name), { recursive: true });
  }
  symlinkSync(join(PLUGIN_DIR, "node_modules"), join(plugin, "node_modules"), process.platform === "win32" ? "junction" : "dir");
  const observed = join(directory, "observed-waits");
  const launcherUsed = join(directory, "unexpected-launcher-wait");
  const sourceScript = join(directory, "fake-source-cli");
  const runtimeJsonPath = join(plugin, "runtime.json");
  const runtimeVersion = JSON.parse(readFileSync(runtimeJsonPath, "utf8")).plugin_version;
  const writeSource = (): void => writeFileSync(sourceScript, [
    "#!/usr/bin/env python3",
    "import json,os,sys,time",
    `runtime=json.load(open(${JSON.stringify(runtimeJsonPath)}))`,
    `with open(${JSON.stringify(observed)}, 'a') as output:`,
    ` output.write(json.dumps({'binary':sys.argv[0],'version':runtime['plugin_version'],'args':sys.argv[1:]})+'\\n')`,
    "time.sleep(0.5)",
    "raise SystemExit(3)",
    "",
  ].join("\n"));
  writeSource();
  // Stub only the Go build seam; session_start, bridge and wait subprocesses
  // remain production code. A later build can overwrite the fixed source.
  const bridgePath = join(plugin, "src", "bridge.ts");
  const bridgeSource = readFileSync(bridgePath, "utf8");
  assert.ok(bridgeSource.includes("runBuild: runGoBuild,"));
  writeFileSync(bridgePath, bridgeSource.replace("runBuild: runGoBuild,", `runBuild: async (argv) => { const fs = await import('node:fs/promises'); const target = argv[argv.indexOf('-o') + 1]; await fs.copyFile(${JSON.stringify(sourceScript)}, target); await fs.chmod(target, 0o755); },`));
  const bin = join(plugin, "bin");
  mkdirSync(bin);
  const launcher = join(bin, "ws-mcp-launcher.py");
  // Deliberately no runtime cache: the bridge transport requires a source
  // bootstrap and refuses every mailbox wait, so a primed cache or fallback
  // launcher cannot mask a missing direct-binary handoff.
  writeFileSync(launcher, [
    "import json,os,sys,time",
    "binary=os.environ.get('WS_MCP_BOOTSTRAP_BINARY', '')",
    "if not binary or not os.path.isfile(binary): raise SystemExit('source-built binary missing')",
    "if sys.argv[1:3]==['mailbox','wait']:",
    ` open(${JSON.stringify(launcherUsed)}, 'w').write(binary)`,
    " raise SystemExit('mailbox must bypass launcher')",
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
  let stagedBinary: string | undefined;
  const diagnostics: string[] = [];
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
    const ui = new Proxy({ notify: (message: string) => { diagnostics.push(message); }, setFooter: () => {} }, { get: (target: Record<string, unknown>, key: string) => target[key] ?? (() => {}) });
    await session.bindExtensions({ mode: "tui", uiContext: ui as never, onError: (error) => { throw error; } });
    await waitForFile(observed).catch((error) => { throw new Error(`${error.message}\n${diagnostics.join("\n")}`); });
    const waitsSoFar = () => readFileSync(observed, "utf8").trim().split("\n").map((line) => JSON.parse(line));
    const firstBinary = waitsSoFar()[0].binary;
    stagedBinary = firstBinary;
    assert.match(firstBinary, /[/\\]local-devenv[/\\]\.mailbox-[^/\\]+[/\\]ws-mcp$/);
    assert.deepEqual(readdirSync(join(plugin, ".runtime", "local-devenv")).filter((name) => name.startsWith("ws-mcp.")), [], "the validated unique bootstrap is released after staging");
    assert.ok(existsSync(firstBinary));
    const updatedRuntime = JSON.parse(readFileSync(runtimeJsonPath, "utf8"));
    updatedRuntime.plugin_version = "99.99.99";
    updatedRuntime.release_tag = "v99.99.99";
    writeFileSync(runtimeJsonPath, JSON.stringify(updatedRuntime));
    for (let i = 0; i < 200 && waitsSoFar().length < 2; i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    const firstTwo = waitsSoFar();
    assert.equal(firstTwo.length, 2, `the mailbox waiter must re-arm after runtime.json changes: ${diagnostics.join(" | ")}`);
    assert.deepEqual(firstTwo.map((wait) => [wait.binary, wait.version]), [[firstBinary, runtimeVersion], [firstBinary, "99.99.99"]]);
    assert.equal(existsSync(launcherUsed), false, "no re-arm may re-enter the release-backed launcher");
    assert.deepEqual(readdirSync(runtimeCache), [], "a primed launcher cache cannot mask the missing handoff");
  } finally {
    if (session) {
      await session.extensionRunner?.emit({ type: "session_shutdown", reason: "quit" });
      if (stagedBinary) assert.equal(existsSync(stagedBinary), false, "normal shutdown awaits child close and staged cleanup");
      session.dispose();
    }
    for (const key of Object.keys(process.env)) if (!(key in prior)) delete process.env[key];
    Object.assign(process.env, prior);
    rmSync(directory, { recursive: true, force: true });
  }
});
