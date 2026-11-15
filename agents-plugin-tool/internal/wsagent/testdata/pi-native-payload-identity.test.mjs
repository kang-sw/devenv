import assert from "node:assert/strict";
import childProcess, { spawn as namedSpawn } from "node:child_process";
import { createHash } from "node:crypto";
import { EventEmitter } from "node:events";
import fs from "node:fs";
import { syncBuiltinESMExports } from "node:module";
import net, { createServer as namedCreateServer } from "node:net";
import { delimiter, dirname, isAbsolute, join, relative, resolve, sep } from "node:path";
import { PassThrough, Writable } from "node:stream";
import test from "node:test";
import { pathToFileURL } from "node:url";

// Opt-in, dedicated Node test process; do not use --test-isolation=none.
// Inputs: the actual CLI-adjacent bundled SDK, an already-materialized native
// root, its independently recorded entry SHA256, and a fresh empty fixture root.
// This file never materializes, rebuilds, installs, launches a child, or removes
// evidence. Builtin/client mocks and the fixture environment remain installed
// through process exit, including failure/deadline paths. These trusted-code
// mocks are observations of known boundaries, not an operating-system sandbox.
const input = {
  sdk: process.env.WS_PI_TEST_SDK,
  native: process.env.WS_PI_TEST_NATIVE_ROOT,
  sha256: process.env.WS_PI_TEST_NATIVE_SHA256,
  fixture: process.env.WS_PI_TEST_FIXTURE,
};
const requested = Object.values(input).some(Boolean);
const hash = bytes => createHash("sha256").update(bytes).digest("hex");
const json = path => JSON.parse(fs.readFileSync(path, "utf8"));

function canonical(path) {
  assert.ok(isAbsolute(path), "fixture inputs must be absolute");
  assert.equal(resolve(path), fs.realpathSync(path), "use canonical, non-symlink paths");
  return resolve(path);
}
function inside(root, path) {
  const rel = relative(root, path);
  return rel !== "" && rel !== ".." && !rel.startsWith(`..${sep}`) && !isAbsolute(rel);
}
function inventory(root) {
  const entries = [];
  function walk(dir) {
    for (const name of fs.readdirSync(dir).sort()) {
      const path = join(dir, name), stat = fs.lstatSync(path);
      assert.ok(!stat.isSymbolicLink(), `payload link: ${path}`);
      const rel = relative(root, path).split(sep).join("/");
      if (stat.isDirectory()) { entries.push([rel, "directory", stat.mode & 0o777]); walk(path); }
      else { assert.ok(stat.isFile(), `payload special file: ${path}`); entries.push([rel, stat.mode & 0o777, hash(fs.readFileSync(path))]); }
    }
  }
  walk(root);
  return entries;
}

test("unchanged materialized native payload uses the actual Pi host graph", {
  skip: !requested,
  concurrency: false,
}, async t => {
  assert.ok(Object.values(input).every(Boolean), "all four explicit fixture inputs are required");
  assert.notEqual(process.platform, "win32", "this POSIX socket fixture does not establish native Windows behavior");
  assert.match(input.sha256, /^[0-9a-f]{64}$/);
  const sdkPath = canonical(input.sdk), nativeRoot = canonical(input.native), root = canonical(input.fixture);
  assert.ok(fs.statSync(root).isDirectory());
  assert.deepEqual(fs.readdirSync(root), [], "never reuse a fixture or retained incident evidence");
  assert.ok(!inside(root, nativeRoot) && !inside(nativeRoot, root) && root !== nativeRoot);
  const entry = join(nativeRoot, "dist", "index.js");
  assert.equal(hash(fs.readFileSync(entry)), input.sha256, "independently recorded materialized entry");
  assert.ok(!fs.existsSync(join(nativeRoot, ".local-devenv-runtime")));
  assert.ok(!fs.existsSync(join(nativeRoot, ".runtime")));
  const before = inventory(nativeRoot);
  assert.ok(!before.some(([path]) => /(^|\/)node_modules\/@(?:earendil-works|mariozechner)\/pi-(?:coding-agent|agent-core|ai|tui)(?:\/|$)/.test(path)), "no physical host Pi copies in the payload");
  const owner = json(join(nativeRoot, "workflow-owner.json")).plugin;
  assert.ok(owner === "ws" || owner === "wsflow");
  const runtime = json(join(nativeRoot, "runtime.json"));
  assert.equal(runtime.plugin, owner);

  let hostRoot = dirname(sdkPath), host;
  for (;;) {
    const manifest = join(hostRoot, "package.json");
    if (fs.existsSync(manifest) && json(manifest).name === "@earendil-works/pi-coding-agent") { host = json(manifest); break; }
    assert.notEqual(dirname(hostRoot), hostRoot, "actual host package manifest is required");
    hostRoot = dirname(hostRoot);
  }
  assert.equal(host.version, "1.5.2", "reviewed SDK version; reread its docs before changing this pin");
  const cli = canonical(join(hostRoot, typeof host.bin === "string" ? host.bin : host.bin.pi));
  assert.equal(sdkPath, join(dirname(cli), "index.js"), "not package.main's separate module graph");
  assert.equal(cli, join(hostRoot, "dist", "bundle", "cli.js"));

  const cwd = join(root, "project"), agentDir = join(root, "pi-agent"), tmp = join(root, "tmp");
  for (const dir of [cwd, agentDir, tmp, join(root, "home"), join(root, "cache"), join(root, "config"), join(root, "npm-cache")]) fs.mkdirSync(dir, { mode: 0o700 });
  const environment = {
    HOME: join(root, "home"), USERPROFILE: join(root, "home"),
    TMPDIR: tmp, TMP: tmp, TEMP: tmp,
    XDG_CACHE_HOME: join(root, "cache"), XDG_CONFIG_HOME: join(root, "config"),
    WS_CACHE_HOME: join(root, "cache"), WS_CONFIG_HOME: join(root, "config"),
    npm_config_cache: join(root, "npm-cache"), PI_CODING_AGENT_DIR: agentDir,
    PI_OFFLINE: "1", PI_SKIP_VERSION_CHECK: "1", PI_TELEMETRY: "0",
    JITI_FS_CACHE: "0", TERM: "dumb", NO_COLOR: "1",
    PATH: [dirname(process.execPath), "/usr/bin", "/bin"].join(delimiter),
  };
  for (const key of Object.keys(process.env)) delete process.env[key];
  Object.assign(process.env, environment);
  process.chdir(cwd);
  process.argv[1] = cli; // The unchanged bundle captures this at import time.
  const payloadEnvironment = {
    ...environment, WS_MCP_NAMESPACE: owner, WS_MCP_SETUP_TOOL: owner === "wsflow" ? "setup" : "ws.setup",
    WS_MCP_NO_AGENT: owner === "wsflow" ? "1" : "",
    WS_RSRC_ROOT: join(nativeRoot, "rsrc"), WS_SKILLS_ROOT: join(nativeRoot, "skills"),
  };
  const childEnvironment = {
    ...payloadEnvironment,
    WS_MCP_CONFIG_MANIFESTS: ["config-manifest.json", "dashboard-config-manifest.json"].map(name => join(nativeRoot, name)).join(delimiter),
  };
  const violations = [], operations = [], protocol = [], notices = [], runnerErrors = [], registryReads = [];
  const counts = { mcpAttempts: 0, mcp: 0, kill: 0, final: 0, server: 0, listen: 0, close: 0, start: 0, stop: 0, startup: 0, shutdown: 0 };
  const fakeDeliveries = new Set(), liveIntervals = new Set(), teardownFailures = [];
  let scheduledDeliveries = 0, completedDeliveries = 0, intervalStarts = 0;
  let hasPrimaryFailure = false, primaryFailure, shutdownCompleted = false;
  function scheduleFake(label, fn) {
    let finish;
    const delivery = new Promise(resolve => { finish = resolve; });
    fakeDeliveries.add(delivery); scheduledDeliveries++;
    queueMicrotask(() => {
      try { fn(); }
      catch (error) { violations.push(`fake delivery ${label}: ${error.message}`); }
      finally { completedDeliveries++; fakeDeliveries.delete(delivery); finish(); }
    });
  }
  async function drainFakeDeliveries() {
    await Promise.all([...fakeDeliveries]);
    // One checkpoint for the known Promise-only config/feed continuations of
    // these deliveries, not a proof of general scheduler quiescence.
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(fakeDeliveries.size, 0);
    assert.equal(completedDeliveries, scheduledDeliveries);
  }
  const refused = name => (..._args) => { operations.push({ refused: name }); const error = new Error(`unexpected operation: ${name}`); violations.push(error.message); throw error; };
  function boundary(name, fn) {
    try { return fn(); }
    catch (error) { violations.push(`${name}: ${error.message}`); throw error; }
  }
  let dashboardRead;
  const dashboardSeen = new Promise(resolve => { dashboardRead = resolve; });
  const configSeen = new Set();
  // Five required requests: initialize, tools/list, keyless ferrule, then the
  // retention/dashboard reads in either order. A sixth may ONLY repeat the
  // identical disabled-dashboard read after both initial config reads (2s TTL).
  function reply(frame) {
    return boundary("MCP transcript", () => {
      protocol.push(frame);
      assert.equal(frame.jsonrpc, "2.0");
      assert.equal(frame.id, protocol.length);
      assert.deepEqual(Object.keys(frame).sort(), ["id", "jsonrpc", "method", "params"]);
      if (protocol.length === 1) {
        assert.equal(frame.method, "initialize");
        assert.deepEqual(frame.params, { protocolVersion: "2025-03-26", capabilities: {}, clientInfo: { name: "ws-pi-bridge", version: "0.1.0" } });
        return { protocolVersion: "2025-03-26", capabilities: {}, serverInfo: { name: "identity-fixture", version: runtime.plugin_version } };
      }
      if (protocol.length === 2) { assert.equal(frame.method, "tools/list"); assert.deepEqual(frame.params, {}); return { tools: [] }; }
      assert.equal(frame.method, "tools/call");
      if (protocol.length === 3) {
        assert.deepEqual(frame.params, { name: "ferrule", arguments: { root: cwd, format: "json" } });
        return { content: [{ type: "text", text: "{}" }] }; // Deliberately no live or synthetic key.
      }
      assert.ok(protocol.length <= 6);
      const key = frame.params.arguments?.key;
      assert.ok(key === "pi.child_retention_ttl_days" || key === "pi.dashboard_enabled");
      if (configSeen.has(key)) {
        assert.equal(key, "pi.dashboard_enabled", "only disabled-dashboard may be reread");
        assert.equal(configSeen.size, 2); assert.equal(protocol.length, 6);
      }
      assert.deepEqual(frame.params, { name: "config.get", arguments: { key, format: "json" } });
      configSeen.add(key);
      if (key === "pi.dashboard_enabled") dashboardRead();
      return { content: [{ type: "text", text: JSON.stringify({ value: key === "pi.dashboard_enabled" ? false : 0 }) }] };
    });
  }
  const proc = new EventEmitter();
  proc.stdout = new PassThrough(); proc.stderr = new PassThrough();
  proc.stdin = new Writable({
    write(bytes, _encoding, done) {
      let frame;
      try {
        const text = bytes.toString();
        assert.ok(text.endsWith("\n") && text.indexOf("\n") === text.length - 1, "one complete JSONL request per write");
        frame = JSON.parse(text);
        const result = reply(frame);
        scheduleFake("MCP response", () => { proc.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", id: frame.id, result })}\n`); done(); });
      } catch (error) {
        violations.push(`fake stdin: ${error.message}`);
        if (frame?.id !== undefined) scheduleFake("MCP error", () => proc.stdout.write(`${JSON.stringify({ jsonrpc: "2.0", id: frame.id, error: { code: -32600, message: error.message } })}\n`));
        done(error);
      }
    },
    final(done) { counts.final++; done(); },
  });
  proc.stdin.on("error", () => {});
  let fakeExit;
  const exited = new Promise(resolve => { fakeExit = resolve; });
  proc.kill = signal => boundary("fake MCP close", () => {
    operations.push({ fake: "MCP kill", signal });
    assert.equal(signal, undefined); assert.equal(++counts.kill, 1);
    scheduleFake("MCP exit", () => { proc.stdout.end(); proc.stderr.end(); proc.emit("exit", 0, null); fakeExit(); });
    return true;
  });
  // Patch the actual builtin object, then update already-instantiated named ESM
  // exports. No original process function is called, even on mismatches.
  for (const name of ["exec", "execSync", "execFile", "execFileSync", "fork", "spawnSync"]) childProcess[name] = refused(`child_process.${name}`);
  childProcess.spawn = (...args) => boundary("MCP spawn", () => {
    operations.push({ fake: "MCP spawn", command: args[0], args: args[1], options: args[2] });
    assert.equal(++counts.mcpAttempts, 1); assert.equal(args.length, 3);
    assert.equal(args[0], "python3");
    assert.deepEqual(args[1], [join(nativeRoot, "bin", "ws-mcp-launcher.py"), "serve", "--stdio"]);
    assert.deepEqual(args[2], { cwd: nativeRoot, stdio: ["pipe", "pipe", "pipe"], env: childEnvironment });
    counts.mcp++; // Acquired only after validation, immediately before returning.
    return proc;
  });
  const socketDir = join(tmp, `ws-pi-${process.getuid()}`);
  assert.ok(!fs.existsSync(socketDir), "no ambient/stale socket directory may be swept");
  const server = new EventEmitter();
  server.unref = () => server;
  server.address = refused("fake server.address / TCP fallback");
  server.listen = target => boundary("fake channel listen", () => {
    operations.push({ fake: "channel listen", target });
    assert.equal(++counts.listen, 1); assert.deepEqual(Object.keys(target), ["path"]);
    assert.equal(dirname(target.path), socketDir);
    assert.match(target.path.slice(socketDir.length + 1), new RegExp(`^${process.pid}-[0-9a-f]{12}\\.sock$`));
    assert.ok(inside(tmp, target.path));
    scheduleFake("channel listening", () => server.emit("listening"));
    return server;
  });
  server.close = () => boundary("fake channel close", () => {
    operations.push({ fake: "channel close" }); assert.equal(++counts.close, 1);
    scheduleFake("channel close", () => server.emit("close")); return server;
  });
  net.createServer = (...args) => boundary("fake channel server", () => {
    operations.push({ fake: "channel createServer" }); assert.equal(args.length, 0);
    assert.equal(++counts.server, 1); return server;
  });
  net.connect = refused("net.connect"); net.createConnection = refused("net.createConnection");
  net.Socket.prototype.connect = refused("net.Socket.connect");
  globalThis.fetch = refused("fetch");
  syncBuiltinESMExports();
  assert.equal(namedSpawn, childProcess.spawn);
  assert.equal(namedCreateServer, net.createServer);
  const deadline = setTimeout(() => {
    fs.writeSync(2, `${JSON.stringify({ failure: "native identity fixture deadline", primaryFailure: hasPrimaryFailure ? String(primaryFailure) : undefined, teardownFailures: teardownFailures.map(({ label, error }) => ({ label, error: String(error) })), shutdownCompleted, activeSteps, pendingFakeDeliveries: fakeDeliveries.size, liveIntervals: liveIntervals.size, counts, operations, protocol, violations, runnerErrors })}\n`);
    process.exit(1); // Terminates pending work with the boundaries still mocked.
  }, 30_000);
  let runner, load, activeSteps = 0, witness, observedOptions, observedReceiver;
  const sentinel = new Error("identity-fixture: stop before RpcClient.start");
  // Counts only awaited step() calls, not SDK continuations or background work.
  async function step(label, fn) {
    activeSteps++;
    try { return await fn(); }
    catch (error) { t.diagnostic(`${label}: ${error.message}`); throw error; }
    finally { activeSteps--; }
  }
  async function teardown(label, fn) {
    try { await fn(); }
    catch (error) { teardownFailures.push({ label, error }); }
  }
  try {
    // First non-builtin import: the actual npm CLI's adjacent bundled SDK.
    const sdk = await step("SDK import", () => import(pathToFileURL(sdkPath).href));
    sdk.RpcClient.prototype.start = async function () {
      observedReceiver = this;
      observedOptions = structuredClone(this.options);
      counts.start++;
      // Do not assert only inside this boundary: the adapter catches errors.
      // The receiver, options, count and unchanged null process are checked below.
      throw sentinel;
    };
    sdk.RpcClient.prototype.stop = async function () {
      boundary("unstarted RpcClient.stop", () => { assert.equal(++counts.stop, 1); assert.equal(this, observedReceiver); assert.equal(this.process, null); });
    };
    for (const name of ["prompt", "steer", "followUp", "abort", "getState", "getSessionStats", "getLastAssistantText", "setThinkingLevel"]) sdk.RpcClient.prototype[name] = refused(`RpcClient.${name}`);
    const model = Object.freeze({ provider: "identity-fixture", id: "identity-only", reasoning: false, contextWindow: 4096 });
    const modelRuntime = new Proxy({
      getModels: () => [model], getAvailableSnapshot: () => [model],
      hasConfiguredAuth: provider => boundary("catalog auth metadata", () => { assert.equal(provider, model.provider); return true; }),
    }, { get(target, key) { return Object.hasOwn(target, key) ? target[key] : refused(`model runtime.${String(key)}`)(); } });
    const registry = new sdk.ModelRegistry(modelRuntime);
    for (const name of ["getAll", "getAvailable", "hasConfiguredAuth"]) {
      const original = registry[name];
      registry[name] = function (...args) { registryReads.push({ name, receiver: this }); return original.apply(this, args); };
    }
    const bus = sdk.createEventBus();
    const offWitness = bus.on("identity-fixture:host", value => { assert.equal(witness, undefined); witness = value; });
    const witnessPath = join(root, "host-witness.mjs");
    fs.writeFileSync(witnessPath, [
      'import { Box, Text } from "@earendil-works/pi-tui";',
      'import { RpcClient, ModelRegistry } from "@earendil-works/pi-coding-agent";',
      'export default function (pi) { pi.events.emit("identity-fixture:host", { Box, Text, RpcClient, ModelRegistry }); }',
      "",
    ].join("\n"), { flag: "wx" });
    const settings = sdk.SettingsManager.inMemory({ packages: [], extensions: [], skills: [], prompts: [], themes: [], defaultTools: [], compaction: { enabled: false }, retry: { enabled: false }, cacheWarming: "off", enableInstallTelemetry: false, enableAnalytics: false });
    const loader = new sdk.DefaultResourceLoader({
      cwd, agentDir, settingsManager: settings, eventBus: bus,
      additionalExtensionPaths: [entry, witnessPath], extensionFactories: [],
      additionalSkillPaths: [join(nativeRoot, "skills")],
      noExtensions: true, noSkills: false, noPromptTemplates: true, noThemes: true, noContextFiles: true,
      systemPrompt: "identity fixture; never prompt", appendSystemPrompt: [],
    });
    await step("load unchanged payload", () => loader.reload());
    const loadedSkills = loader.getSkills();
    assert.deepEqual(loadedSkills.diagnostics, []);
    assert.deepEqual(loadedSkills.skills.map(skill => skill.filePath).sort(), before.filter(([path]) => path.startsWith("skills/") && path.endsWith("/SKILL.md")).map(([path]) => join(nativeRoot, path)).sort());
    load = loader.getExtensions();
    assert.deepEqual(load.errors, []); assert.deepEqual(load.warnings ?? [], []);
    assert.deepEqual(load.extensions.map(extension => extension.resolvedPath).sort(), [entry, witnessPath].sort());
    assert.equal(witness.RpcClient, sdk.RpcClient); assert.equal(witness.ModelRegistry, sdk.ModelRegistry);
    offWitness();
    const native = load.extensions.find(extension => extension.resolvedPath === entry);
    const manager = sdk.SessionManager.inMemory(cwd);
    assert.equal(manager.getSessionFile(), undefined);
    runner = new sdk.ExtensionRunner(load.extensions, load.runtime, cwd, manager, registry);
    runner.onError(error => { runnerErrors.push(error); });
    const toolInfo = () => runner.getAllRegisteredTools().map(({ definition, sourceInfo }) => ({ ...definition, sourceInfo, exposure: definition.exposure ?? "direct" }));
    let active = toolInfo().map(tool => tool.name);
    runner.bindCore({
      sendMessage: refused("sendMessage"), sendUserMessage: refused("sendUserMessage"), appendEntry: refused("appendEntry"),
      setSessionName: refused("setSessionName"), getSessionName: () => undefined, setLabel: refused("setLabel"),
      getActiveTools: () => [...active], getAllTools: toolInfo, getSettings: () => settings.getSettings(),
      setActiveTools: names => { active = [...names]; }, refreshTools: () => {},
      getCommands: () => [], setModel: refused("setModel"), getThinkingLevel: () => "off", setThinkingLevel: refused("setThinkingLevel"),
    }, {
      getModel: () => model, getScopedModels: () => [],
      // Busy only for push admission: a failed spawn stays held, never requests
      // a real/model turn. No agent or provider is running in this SDK fixture.
      isIdle: () => false, isProjectTrusted: () => false, getSignal: () => undefined,
      abort: refused("context.abort"), hasPendingMessages: () => false, shutdown: refused("context.shutdown"),
      getContextUsage: () => undefined, compact: refused("compact"), getSystemPrompt: () => "identity fixture",
      executeTool: refused("nested executeTool"), getCallableTools: () => [],
    }, Object.fromEntries(["registerProvider", "registerNativeProvider", "unregisterProvider", "registerVirtualModel", "unregisterVirtualModel"].map(name => [name, refused(name)])));
    const expectedNotices = [
      ["ws-pi-bridge: ferrule response carried no session_key", "warning"],
      [`ws-pi-bridge: registered 0 ws__* tools from ws-mcp ${runtime.plugin_version}`, "info"],
      ["All available models (1):\nidentity-fixture/identity-only", "info"],
    ];
    runner.setUIContext({
      notify(message, type = "info") { notices.push([message, type]); boundary("expected notification", () => assert.deepEqual(notices.at(-1), expectedNotices[notices.length - 1])); },
      setWidget: refused("setWidget"), setFooter: refused("setFooter"), setStatus: refused("setStatus"), custom: refused("ui.custom"),
    }, "rpc");
    // Track the actual global interval handles created by this startup/spawn
    // (liveness and ownership observers); no claim covers unrelated schedulers.
    const setInterval = globalThis.setInterval, clearInterval = globalThis.clearInterval;
    globalThis.setInterval = (...args) => {
      const handle = setInterval(...args); liveIntervals.add(handle); intervalStarts++; return handle;
    };
    globalThis.clearInterval = handle => { liveIntervals.delete(handle); return clearInterval(handle); };
    counts.startup++;
    await step("one mocked startup", () => runner.emit({ type: "session_start", reason: "startup" }));
    await step("dashboard disabled read", () => dashboardSeen);
    await drainFakeDeliveries();
    assert.deepEqual(runnerErrors, []); assert.equal(counts.mcp, 1);
    assert.ok(protocol.length === 5 || protocol.length === 6); assert.equal(configSeen.size, 2);
    assert.deepEqual({ ...process.env }, payloadEnvironment);
    assert.equal(runner.getModelRegistry(), registry);
    const catalog = native.commands.get("ws-model-catalog-list");
    assert.ok(catalog);
    await step("loaded catalog", () => catalog.handler("", runner.createCommandContext()));
    const renderer = native.messageRenderers.get("ws-agent-report");
    assert.equal(typeof renderer, "function");
    const component = renderer({ content: "[ws-agent-report] agent identity-fixture", details: {} }, { expanded: true, outputPad: 1, keyHint: () => "", keyText: () => "" }, { fg: (_color, text) => text, bg: (_color, text) => text, bold: text => text });
    assert.equal(Object.getPrototypeOf(component), witness.Box.prototype);
    assert.ok(component.children.some(child => Object.getPrototypeOf(child) === witness.Text.prototype));
    assert.ok(component.render(80).some(line => line.includes("identity-fixture")));

    const prompt = join(root, "prompt.md");
    fs.writeFileSync(prompt, "Identity observation only; no child or model may run.\n", { flag: "wx" });
    const spawn = native.tools.get("ws-agent-spawn").definition;
    await assert.rejects(step("one loaded spawn", () => spawn.execute("identity-spawn", { system_prompt_path: prompt, prompt: "not delivered", alias: "identity-fixture" }, undefined, undefined, runner.createToolContext("identity-spawn", undefined))), error => error === sentinel);
    assert.equal(counts.start, 1);
    assert.equal(Object.getPrototypeOf(observedReceiver), sdk.RpcClient.prototype);
    assert.equal(observedReceiver.constructor, witness.RpcClient);
    assert.equal(observedReceiver.process, null);
    assert.equal(observedOptions.cliPath, cli); assert.equal(observedOptions.cwd, cwd);
    assert.equal(observedOptions.model, "identity-fixture/identity-only");
    const args = observedOptions.args;
    assert.deepEqual(args.slice(0, 1), ["--session"]);
    assert.ok(inside(agentDir, args[1]));
    assert.deepEqual(args.slice(2, 7), ["--session-dir", dirname(args[1]), "--append-system-prompt", join(dirname(args[1]), "prompt.md"), "--no-extensions"]);
    assert.deepEqual(args.slice(7, 10), ["--extension", entry, "--tools"]);
    assert.equal(args.length, 11); assert.ok(args[10].split(",").includes("ws-report-to-lead"));
    assert.ok(fs.readFileSync(args[5], "utf8").startsWith(fs.readFileSync(prompt, "utf8")));
    const childEnv = observedOptions.env;
    const endpoint = JSON.parse(childEnv.WS_PI_CHANNEL_ENDPOINT);
    assert.deepEqual(endpoint, { kind: "pipe", path: operations.find(operation => operation.fake === "channel listen").target.path });
    assert.match(childEnv.WS_PI_CHANNEL_CREDENTIAL, /^[A-Za-z0-9_-]{43}$/);
    assert.deepEqual(Object.keys(childEnv).sort(), ["WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE", "WS_PI_FORK_CONTEXT", "WS_PI_FORK_AFFINITY", "WS_PI_PARENT_SESSION_KEY", "WS_PI_DELEGATION_POLICY", "WS_PI_WEB_HOME", "WS_PI_CHANNEL_ENDPOINT", "WS_PI_CHANNEL_CREDENTIAL", "WS_PI_CHANNEL_GENERATION", "WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL", "WS_MAILBOX", "WS_MAILBOX_AUTO"].sort());
    assert.equal(childEnv.WS_PI_SPAWN_ROLE, "worker"); assert.equal(childEnv.WS_PI_CHANNEL_GENERATION, "1");
    for (const key of ["WS_PI_EXPLORE_MODE", "WS_PI_FORK_CONTEXT", "WS_PI_FORK_AFFINITY", "WS_PI_PARENT_SESSION_KEY", "WS_PI_WEB_HOME", "WS_MCP_BOOTSTRAP_BINARY", "WS_MCP_BOOTSTRAP_URL", "WS_MAILBOX", "WS_MAILBOX_AUTO"]) assert.equal(childEnv[key], "");
    assert.equal(JSON.parse(childEnv.WS_PI_DELEGATION_POLICY).depth, 1);
    const listed = await step("loaded list", () => native.tools.get("ws-agent-list").definition.execute("identity-list", { include_prompt: true }));
    const rows = JSON.parse(listed.content[0].text);
    assert.equal(rows.length, 1); assert.equal(rows[0].alias, "identity-fixture"); assert.equal(rows[0].status, "dormant");
    const transcript = await step("loaded transcript", () => native.tools.get("ws-agent-transcript").definition.execute("identity-transcript", { agent_id: rows[0].agent_id }));
    assert.deepEqual(JSON.parse(transcript.content[0].text), { transcript_path: args[1] });
    assert.equal(Object.getPrototypeOf(registry), witness.ModelRegistry.prototype);
    assert.deepEqual(registryReads.map(read => read.name), ["getAvailable", "getAll", "hasConfiguredAuth"]);
    assert.ok(registryReads.every(read => read.receiver === registry));
    assert.deepEqual(notices, expectedNotices);
  } catch (error) {
    hasPrimaryFailure = true; primaryFailure = error;
  } finally {
    await teardown("mocked shutdown", async () => {
      if (runner) {
        counts.shutdown++;
        await step("mocked shutdown", () => runner.emit({ type: "session_shutdown", reason: "quit" }));
        shutdownCompleted = true;
      }
    });
    // A rejected spawn attempt owns no child and must not wait for its exit.
    if (counts.mcp) await teardown("acquired fake MCP exit", () => exited);
    // index.ts shutdown awaits stopAll (observer intervals), stops the dashboard
    // feed (flush/poll timers), then closes the bridge. Drain tracked fake events
    // and their known Promise continuations before the external ledger checks.
    await teardown("scheduled fake deliveries", drainFakeDeliveries);
    await teardown("fake streams", () => Promise.all([proc.stdin, proc.stdout, proc.stderr].map(stream => new Promise(resolve => {
      if (stream.closed) { resolve(); return; }
      stream.once("close", resolve);
      stream.destroy();
    }))));
    // These are scoped completion assertions, not a universal quiescence proof.
    await teardown("awaited steps", () => assert.equal(activeSteps, 0));
    await teardown("known shutdown", () => assert.equal(shutdownCompleted, true));
    await teardown("observer intervals", () => { assert.equal(intervalStarts, 2); assert.equal(liveIntervals.size, 0); });
    await teardown("runner errors", () => assert.deepEqual(runnerErrors, []));
    await teardown("boundary violations", () => assert.deepEqual(violations, []));
    await teardown("expected counts", () => assert.deepEqual(counts, { mcpAttempts: 1, mcp: 1, kill: 1, final: 1, server: 1, listen: 1, close: 1, start: 1, stop: 1, startup: 1, shutdown: 1 }));
    await teardown("finite transcript", () => { assert.ok(protocol.length === 5 || protocol.length === 6); assert.equal(configSeen.size, 2); });
    await teardown("unchanged payload", () => assert.deepEqual(inventory(nativeRoot), before, "actual materialized payload is unchanged"));
    await teardown("no socket file", () => assert.ok(!fs.existsSync(operations.find(operation => operation.fake === "channel listen").target.path), "fake listener created no socket file"));
    t.diagnostic(JSON.stringify({ primaryFailure: hasPrimaryFailure ? String(primaryFailure) : undefined, teardownFailures: teardownFailures.map(({ label, error }) => ({ label, error: String(error) })), shutdownCompleted, activeSteps, scheduledDeliveries, completedDeliveries, intervalStarts, liveIntervals: liveIntervals.size, counts, operations, protocol, notices, runnerErrors, violations }));
    clearTimeout(deadline);
    // Intentionally no restore or cleanup, even on failure. Preserve the primary
    // exception; teardown failures are separately labelled in the diagnostic.
    if (hasPrimaryFailure) throw primaryFailure;
    if (teardownFailures.length) throw new AggregateError(teardownFailures.map(({ label, error }) => new Error(label, { cause: error })), "native identity fixture teardown failed");
  }
});
