import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { createAgentSession, DefaultResourceLoader, ModelRuntime, SessionManager, SettingsManager } from "@earendil-works/pi-coding-agent";
import { parseOrphans, sidecarPath, writeSidecarAt } from "../src/agent-sidecar.ts";

const PLUGIN_DIR = join(process.cwd());

async function waitForFile(path: string): Promise<void> {
  for (let i = 0; i < 400; i += 1) {
    if (existsSync(path)) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error(`mailbox waiter never started: ${path}`);
}

test("Pi session generations use Python mailbox waits and preserve recovery across footer/shutdown races", async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-mailbox-handoff-"));
  const plugin = join(directory, "plugin");
  mkdirSync(plugin);
  for (const name of ["src", "runtime.json", "config-manifest.json", "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md"]) {
    cpSync(join(PLUGIN_DIR, name), join(plugin, name), { recursive: true });
  }
  symlinkSync(join(PLUGIN_DIR, "node_modules"), join(plugin, "node_modules"), process.platform === "win32" ? "junction" : "dir");
  const observed = join(directory, "observed-waits");
  const lookupDelay = join(directory, "delay-next-lookup");
  const lookupStarted = join(directory, "lookup-in-flight");
  const bridgePids = join(directory, "bridge-pids");
  const stopCalls = join(directory, "stop-calls");
  const footerTrigger = join(directory, "pause-final-footer");
  const footerEntered = join(directory, "final-footer-entered");
  const footerRelease = join(directory, "release-final-footer");
  const shutdownTrigger = join(directory, "pause-shutdown");
  const shutdownEntered = join(directory, "shutdown-claimed");
  const shutdownRelease = join(directory, "release-shutdown");
  const sourceScript = join(directory, process.platform === "win32" ? "fake-source-cli.exe" : "fake-source-cli");
  const runtimeJsonPath = join(plugin, "runtime.json");
  const runtimeVersion = JSON.parse(readFileSync(runtimeJsonPath, "utf8")).plugin_version;
  writeFileSync(sourceScript, "fake source build, never executable");
  // Stub only the Go build seam; session_start, bridge and wait subprocesses
  // remain production code. A later build can overwrite the fixed source.
  const bridgePath = join(plugin, "src", "bridge.ts");
  const bridgeSource = readFileSync(bridgePath, "utf8");
  // Session start goes through index.ts, which leaves BridgeOptions.runBuild
  // unset, so the default in the copied bridge is replaced instead.
  assert.ok(bridgeSource.includes("opts.runBuild ?? runGoBuild;"));
  writeFileSync(bridgePath, bridgeSource.replace("opts.runBuild ?? runGoBuild;", `opts.runBuild ?? (async (argv: string[]) => { const fs = await import('node:fs/promises'); const target = argv[argv.indexOf('-o') + 1]; await fs.copyFile(${JSON.stringify(sourceScript)}, target); await fs.chmod(target, 0o755); });`));
  // Deliberately pause the last awaited footer and the first shutdown await
  // in a copied plugin only. The production generation ownership path remains
  // unchanged, but this forces the adverse continuation order deterministically.
  const indexPath = join(plugin, "src", "index.ts");
  const indexSource = readFileSync(indexPath, "utf8");
  const footerCall = "    await applySessionStartAgentFooter(agentFooterLifecycle, bootstrapRole, ctx, agentTools.rpcRegistry, dispatchStorage);";
  const shutdownCall = "    await persistShutdownAgentSnapshots(claimedTools, claimedSidecar, claimedThreads);";
  assert.ok(indexSource.includes(footerCall) && indexSource.includes(shutdownCall));
  const pauseAt = (trigger: string, entered: string, release: string): string => [
    `const fs = await import('node:fs');`,
    `if (fs.existsSync(${JSON.stringify(trigger)}) && !fs.existsSync(${JSON.stringify(entered)})) {`,
    `  fs.writeFileSync(${JSON.stringify(entered)}, '');`,
    `  while (!fs.existsSync(${JSON.stringify(release)})) await new Promise(resolve => setTimeout(resolve, 5));`,
    `}`,
  ].join("\n");
  writeFileSync(indexPath, indexSource
    .replace(footerCall, `    await (async () => { const footer = applySessionStartAgentFooter(agentFooterLifecycle, bootstrapRole, ctx, agentTools.rpcRegistry, dispatchStorage); ${pauseAt(footerTrigger, footerEntered, footerRelease)} await footer; })();`)
    .replace(shutdownCall, `    ${pauseAt(shutdownTrigger, shutdownEntered, shutdownRelease)}\n${shutdownCall}`));
  const spawnerPath = join(plugin, "src", "spawner.ts");
  const spawnerSource = readFileSync(spawnerPath, "utf8");
  const stopMethod = "    async stopAll(): Promise<void> {";
  assert.equal(spawnerSource.split(stopMethod).length, 2);
  writeFileSync(spawnerPath, spawnerSource.replace(stopMethod, `${stopMethod}\n      await (await import('node:fs/promises')).appendFile(${JSON.stringify(stopCalls)}, 'stop\\n');`));
  const bin = join(plugin, "bin");
  mkdirSync(bin);
  const launcher = join(bin, "ws-mcp-launcher.py");
  // Fake Python transport: record waits without ever executing the bootstrap.
  writeFileSync(launcher, [
    "import json,os,sys,time",
    "if sys.argv[1:3]==['mailbox','wait']:",
    ` with open(${JSON.stringify(observed)}, 'a') as output: output.write(json.dumps({'binary':sys.argv[0],'args':sys.argv[1:]})+'\\n')`,
    " time.sleep(0.5)",
    " raise SystemExit(3)",
    "binary=os.environ.get('WS_MCP_BOOTSTRAP_BINARY', '')",
    "if not binary or not os.path.isfile(binary): raise SystemExit('source-built binary missing')",
    `with open(${JSON.stringify(bridgePids)}, 'a') as output: output.write(json.dumps({'binary':binary,'pid':os.getpid()})+'\\n')`,
    "for line in sys.stdin:",
    " q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}",
    ` if m=='initialize': r={'serverInfo':{'version':${JSON.stringify(runtimeVersion)}},'capabilities':{}}`,
    " elif m=='tools/list': r={'tools':[]}",
    " elif m=='tools/call':",
    "  name=p['name']",
    `  if name=='mailbox.lookup_peers' and os.path.exists(${JSON.stringify(lookupDelay)}) and not os.path.exists(${JSON.stringify(lookupStarted)}):`,
    `   open(${JSON.stringify(lookupStarted)}, 'w').close()`,
    "   time.sleep(0.8)",
    "  text=json.dumps({'session_key':'mailbox-test-key'}) if name=='ferrule' else ('# Manual\\n## Session Key\\nmailbox-test-key' if name=='workflow_manual' else ('# Manual' if name=='playbook.read' else '{}'))",
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
    const sessionManager = SessionManager.create(root, join(directory, "sessions"));
    ({ session } = await createAgentSession({ cwd: root, agentDir, sessionManager, resourceLoader: loader, modelRuntime, settingsManager: settings }));
    const ui = new Proxy({ notify: (message: string) => { diagnostics.push(message); }, setFooter: () => {} }, { get: (target: Record<string, unknown>, key: string) => target[key] ?? (() => {}) });
    await session.bindExtensions({ mode: "tui", uiContext: ui as never, onError: (error) => { throw error; } });
    await waitForFile(observed).catch((error) => { throw new Error(`${error.message}\n${diagnostics.join("\n")}`); });
    const waitsSoFar = () => readFileSync(observed, "utf8").trim().split("\n").map((line) => JSON.parse(line));
    assert.equal(waitsSoFar()[0].binary, launcher);
    assert.deepEqual(readdirSync(join(plugin, ".runtime", "local-devenv")), [], "bootstrap and metadata released after bridge startup");
    const originalRuntime = readFileSync(runtimeJsonPath, "utf8");
    const updatedRuntime = JSON.parse(originalRuntime);
    updatedRuntime.plugin_version = "99.99.99";
    updatedRuntime.release_tag = "v99.99.99";
    writeFileSync(runtimeJsonPath, JSON.stringify(updatedRuntime));
    for (let i = 0; i < 800 && waitsSoFar().length < 2; i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    const firstTwo = waitsSoFar();
    assert.equal(firstTwo.length, 2, `the mailbox waiter must re-arm after runtime.json changes: ${diagnostics.join(" | ")}`);
    assert.deepEqual(firstTwo.map((wait) => wait.binary), [launcher, launcher]);

    // Restore the bridge's pinned version before a new connection; changing
    // runtime.json was only the in-flight re-arm test, not a valid new launch.
    writeFileSync(runtimeJsonPath, originalRuntime);
    // Simulate a reload whose first generation stalls in self-slug lookup.
    // The next start supersedes it; the stale bridge's subprocess and unique
    // bootstrap must disappear rather than being lost when globals advance.
    const stopping = session.extensionRunner!.emit({ type: "session_shutdown", reason: "reload" });
    await stopping;
    writeFileSync(lookupDelay, "");
    const reloadOne = session.extensionRunner!.emit({ type: "session_start" });
    await waitForFile(lookupStarted).catch((error) => { throw new Error(`${error.message}\n${diagnostics.join("\n")}`); });
    const buildOutputs = () => readdirSync(join(plugin, ".runtime", "local-devenv")).filter((name) => name.startsWith("ws-mcp.") && !name.endsWith(".tmp"));
    const staleOutput = buildOutputs()[0];
    assert.ok(staleOutput, "the first reload owns a unique bootstrap while lookup is pending");
    const reloadTwo = session.extensionRunner!.emit({ type: "session_start" });
    await Promise.all([reloadOne, reloadTwo]);
    for (let i = 0; i < 800 && waitsSoFar().length < 3; i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    assert.equal(waitsSoFar().length >= 3, true, `new generation must arm: ${diagnostics.join(" | ")}`);
    assert.equal(waitsSoFar()[2].binary, launcher);
    for (let i = 0; i < 800 && waitsSoFar().length < 4; i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    assert.equal(waitsSoFar()[3]?.binary, launcher, "the new generation also re-arms through Python");
    assert.deepEqual(buildOutputs(), [], "both stale and active bootstraps are released after startup/disposal");
    const stalePath = join(plugin, ".runtime", "local-devenv", staleOutput);
    const pidRecord = readFileSync(bridgePids, "utf8").trim().split("\n").map((line) => JSON.parse(line)).find((record) => record.binary === stalePath);
    assert.ok(pidRecord, "the stale bridge must have launched its own subprocess");
    let staleAlive = true;
    for (let i = 0; i < 800 && staleAlive; i += 1) {
      try { process.kill(pidRecord.pid, 0); } catch { staleAlive = false; }
      if (staleAlive) await new Promise((resolve) => setTimeout(resolve, 10));
    }
    assert.equal(staleAlive, false, "stale generation's connected bridge process must be closed");

    // Seed a dormant recovery entry for the next generation. Startup reads
    // and clears this sidecar before the final footer await; shutdown must
    // reclaim it from that generation's captured registry, not later globals.
    await session.extensionRunner!.emit({ type: "session_shutdown", reason: "reload" });
    const sessionFile = sessionManager.getSessionFile();
    assert.ok(sessionFile, "a real Pi session must expose its recovery sidecar path");
    const recoveryPath = sidecarPath(sessionFile);
    const recoveredId = "dormant-footer-race";
    const dormantPrompt = join(directory, "dormant-prompt.md");
    writeFileSync(dormantPrompt, "Offline worker");
    writeSidecarAt(recoveryPath, [{ agentId: recoveredId, sessionPath: join(directory, "dormant.jsonl"), systemPromptPath: dormantPrompt, wsToolNames: ["ws__todo_list"], toolGroup: "full-worker", state: "idle" }]);
    const stopCount = () => existsSync(stopCalls) ? readFileSync(stopCalls, "utf8").trim().split("\n").filter(Boolean).length : 0;
    writeFileSync(footerTrigger, "");
    const racingStart = session.extensionRunner!.emit({ type: "session_start" });
    await waitForFile(footerEntered).catch((error) => { throw new Error(`${error.message}\n${diagnostics.join("\n")}`); });
    assert.equal(existsSync(recoveryPath), false, "startup consumed the seeded sidecar before its footer stalled");
    const firstClaimPid = JSON.parse(readFileSync(bridgePids, "utf8").trim().split("\n").at(-1)!).pid;
    const stopsBeforeClaim = stopCount();
    writeFileSync(shutdownTrigger, "");
    const racingShutdown = session.extensionRunner!.emit({ type: "session_shutdown", reason: "reload" });
    await waitForFile(shutdownEntered);
    writeFileSync(footerRelease, "");
    await racingStart;
    assert.equal(stopCount(), stopsBeforeClaim, "stale footer must not stop shutdown-owned recovered children");
    const waitsBeforeNew = waitsSoFar().length;
    const newerStart = session.extensionRunner!.emit({ type: "session_start" });
    await newerStart;
    for (let i = 0; i < 800 && waitsSoFar().length <= waitsBeforeNew; i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    assert.ok(waitsSoFar().length > waitsBeforeNew, "newer generation must arm its Python waiter");
    assert.equal(waitsSoFar().at(-1)!.binary, launcher);
    const newerPid = JSON.parse(readFileSync(bridgePids, "utf8").trim().split("\n").at(-1)!).pid;
    assert.notEqual(newerPid, firstClaimPid);
    writeFileSync(shutdownRelease, "");
    await racingShutdown;
    assert.equal(stopCount(), stopsBeforeClaim + 1, "shutdown owns exactly one stopAll for the claimed generation");
    assert.ok(parseOrphans(readFileSync(recoveryPath, "utf8")).some((orphan) => orphan.agentId === recoveredId), "read-and-cleared dormant recovery must be durably re-persisted");
    const isAlive = (pid: number): boolean => { try { process.kill(pid, 0); return true; } catch { return false; } };
    for (let i = 0; i < 800 && isAlive(firstClaimPid); i += 1) await new Promise((resolve) => setTimeout(resolve, 10));
    assert.equal(isAlive(firstClaimPid), false, "claimed bridge must close once after persistence");
    assert.equal(isAlive(newerPid), true, "older shutdown cannot close the newer generation");
    assert.equal(stopCount(), stopsBeforeClaim + 1, "newer generation's tools must remain running");
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
