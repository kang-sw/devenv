import assert from "node:assert/strict";
import { existsSync, mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync, realpathSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath, pathToFileURL } from "node:url";
import test from "node:test";
import { installResolver, isWS } from "../pi_lead.mjs";

function fixture(t) {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-lead-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}
function file(path, text) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, text);
  return path;
}
function json(path, obj) { return file(path, JSON.stringify(obj)); }
function resource(path, enabled = true, metadata = {}) { return { path, enabled, metadata }; }

// No harness installation is required for these tests.
test("restoration retains native ordering, enabled filters and metadata at each trust pass", async () => {
  const global = resource("/user/a.ts", true, { scope: "user" });
  const project = resource("/project/b.ts", true, { scope: "project" });
  class PM {
    trusted = false;
    async resolve() { return { extensions: [global, resource("/disabled.ts", false), ...(this.trusted ? [project] : []), resource("builtin:mcp")] }; }
    async resolveExtensionSources() { return { extensions: [resource("/bundle.ts"), resource("builtin:mcp")], skills: ["untouched"] }; }
  }
  installResolver(PM, (path) => path);
  const pm = new PM();
  await pm.resolve();
  let result = await pm.resolveExtensionSources([]);
  assert.deepEqual(result.extensions.map((r) => r.path), ["/bundle.ts", "builtin:mcp", "/user/a.ts"]);
  assert.equal(result.extensions[2], global);
  assert.deepEqual(result.skills, ["untouched"]);
  pm.trusted = true;
  await pm.resolve();
  result = await pm.resolveExtensionSources([]);
  assert.deepEqual(result.extensions.map((r) => r.path), ["/bundle.ts", "builtin:mcp", "/user/a.ts", "/project/b.ts"]);
  // A different loader (session replacement) must not inherit the old cwd set.
  const other = new PM();
  await other.resolve();
  assert.equal((await other.resolveExtensionSources([])).extensions.some((r) => r.path === project.path), false);
});

test("identity uses package manifests, not fuzzy names or checkout ancestry", (t) => {
  const root = fixture(t);
  json(join(root, "package.json"), { name: "ws-pi-bridge", pi: { extensions: ["agents-plugin-pi/src/index.ts"] } });
  const entry = file(join(root, "agents-plugin-pi/src/index.ts"), "");
  const unrelated = file(join(root, ".pi/extensions/ws-helper.ts"), "");
  assert.equal(isWS(resource(entry)), true);
  assert.equal(isWS(resource(unrelated)), false);
  assert.equal(isWS(resource("builtin:mcp")), false);
  for (const name of ["ws", "wsflow", "ws-pi-bridge", "wsflow-pi-bridge"]) {
    const pkg = join(root, name);
    json(join(pkg, "package.json"), { name });
    assert.equal(isWS(resource(join(pkg, "x.ts"), true, { packageRoot: pkg })), true);
  }
});

test("bootstrap runs original CLI in-process with unchanged argv and cwd", (t) => {
  const root = fixture(t);
  json(join(root, "package.json"), { type: "module" });
  const sdk = file(join(root, "index.js"), `export class DefaultPackageManager {
    async resolve() { return {extensions: [{path: '/user/extension.ts', enabled: true}]}; }
    async resolveExtensionSources() { return {extensions: []}; }
  }`);
  file(join(root, "core/source-info.js"), "export const getResourcePathKey = (path) => path;");
  const cli = file(join(root, "cli.js"), `import {DefaultPackageManager} from './index.js';
    const pm = new DefaultPackageManager(); await pm.resolve();
    console.log(JSON.stringify({argv: process.argv.slice(1), cwd: process.cwd(), paths: (await pm.resolveExtensionSources([])).extensions.map(r => r.path)}));`);
  const args = ["--mode", "rpc", "--session", "terminal session.jsonl", "--model", "p/m", "--thinking", "high", "--no-extensions", "--extension", "/bundle.ts"];
  const adapter = fileURLToPath(new URL("../pi_lead.mjs", import.meta.url));
  const run = spawnSync(process.execPath, [adapter, sdk, cli, ...args], { cwd: root, encoding: "utf8" });
  assert.equal(run.status, 0, run.stderr);
  const output = JSON.parse(run.stdout);
  assert.deepEqual(output.argv, [cli, ...args]);
  assert.equal(realpathSync(output.cwd), realpathSync(root));
  assert.deepEqual(output.paths, ["/user/extension.ts"]);
});

test("fallback bridge preserves effective prompt and supplies hosted session identity", async () => {
  const source = readFileSync(new URL("../pi_lead_bridge.mjs", import.meta.url), "utf8");
  const servers = { ws: { command: "bundled-ws", exposure: "direct" } };
  const sessionKey = 'lead-key-"quoted"';
  const prelude = `const servers = ${JSON.stringify(servers)}; const sessionKey = ${JSON.stringify(sessionKey)};\n`;
  const { default: register } = await import("data:text/javascript;base64," + Buffer.from(prelude + source).toString("base64"));
  const registered = {};
  let handler;
  register({ registerMcpServer(name, config) { registered[name] = config; }, on(event, fn) { assert.equal(event, "before_agent_start"); handler = fn; } });
  assert.deepEqual(registered, servers);
  const result = handler({ systemPrompt: "Pi defaults\nUser APPEND_SYSTEM.md\nProject instructions" });
  assert(result.systemPrompt.startsWith("Pi defaults\nUser APPEND_SYSTEM.md\nProject instructions\n\n"));
  assert(result.systemPrompt.includes(JSON.stringify(sessionKey)));
  assert(result.systemPrompt.includes("Pass it as session_key"));
});

test("real npm-bin startup restores ambient extensions using the CLI's own SDK graph", { skip: !process.env.WS_PI_TEST_SDK }, (t) => {
  let packageRoot = dirname(process.env.WS_PI_TEST_SDK);
  let pkg;
  for (;;) {
    try { pkg = JSON.parse(readFileSync(join(packageRoot, "package.json"), "utf8")); } catch { pkg = undefined; }
    if (pkg?.name === "@earendil-works/pi-coding-agent" || pkg?.name === "@mariozechner/pi-coding-agent") break;
    assert.notEqual(dirname(packageRoot), packageRoot, "Pi package manifest not found");
    packageRoot = dirname(packageRoot);
  }
  // Follow the published npm executable, not package.main or a synthetic CLI.
  const cli = realpathSync(join(packageRoot, typeof pkg.bin === "string" ? pkg.bin : pkg.bin.pi));
  const sdk = join(dirname(cli), "index.js");
  const root = fixture(t);
  const cwd = join(root, "project");
  const agentDir = join(root, "agent");
  const marker = file(join(root, "initialized.jsonl"), "");
  const extension = (path, name, trust = false) => file(path, `import {appendFileSync} from 'node:fs';
    export default function(pi) {
      appendFileSync(${JSON.stringify(marker)}, ${JSON.stringify(JSON.stringify(name) + "\n")});
      ${trust ? "pi.on('project_trust', () => ({trusted:'yes'}));" : ""}
    }`);
  extension(join(agentDir, "extensions/user.ts"), "user", true);
  extension(join(cwd, ".pi/extensions/project.ts"), "project");
  const explicit = extension(join(root, "explicit.ts"), "explicit");
  const other = join(root, "other-package");
  json(join(other, "package.json"), { name: "other", pi: { extensions: ["index.ts"] } });
  extension(join(other, "index.ts"), "package");
  const installed = join(root, "installed-ws");
  json(join(installed, "package.json"), { name: "ws-pi-bridge", pi: { extensions: ["index.ts"] } });
  extension(join(installed, "index.ts"), "installed-ws");
  const bundled = extension(join(root, "bundle.ts"), "bundled");
  json(join(agentDir, "settings.json"), { packages: [installed, other], extensions: [explicit], defaultProjectTrust: "ask" });
  const adapter = fileURLToPath(new URL("../pi_lead.mjs", import.meta.url));
  const run = (sdkPath) => {
    file(marker, "");
    const child = spawnSync(process.execPath, [adapter, sdkPath, cli,
      "--offline", "--mode", "rpc", "--no-session", "--list-models", "--no-extensions", "--extension", bundled], {
      cwd, env: { ...process.env, PI_CODING_AGENT_DIR: agentDir, PI_SKIP_VERSION_CHECK: "1" }, encoding: "utf8", timeout: 30000,
    });
    assert.equal(child.status, 0, child.stderr + child.stdout + String(child.error ?? ""));
    return readFileSync(marker, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line));
  };
  const initialized = run(sdk);
  assert.deepEqual([...initialized].sort(), ["bundled", "explicit", "package", "project", "user"]);
  t.diagnostic(JSON.stringify({ npmBin: cli, sdk, initialized }));
  const unbundledSDK = join(packageRoot, "dist/index.js");
  if (sdk !== unbundledSDK) {
    // Reproduce R1: patching package.main must *not* satisfy this test's
    // ambient assertions when npm's executable belongs to another graph.
    assert.deepEqual(run(unbundledSDK), ["bundled"]);
    t.diagnostic("R1 negative control: unbundled SDK + real bundled npm-bin initialized only bundled (ambient missing)");
  }
});

test("incompatible SDK fails closed", () => {
  assert.throws(() => installResolver(class {}, (p) => p), /compatible extension resolver/);
});

// Set this to the installed Pi CLI's adjacent index.js on the remote
// verification host. These tests invoke only resource loading, never a model.
test("native Pi discovery, trust hooks, package filtering, builtins and reload", { skip: !process.env.WS_PI_TEST_SDK }, async (t) => {
  const sdkURL = pathToFileURL(process.env.WS_PI_TEST_SDK);
  const { DefaultPackageManager, DefaultResourceLoader, SettingsManager, ProjectTrustStore } = await import(sdkURL.href);
  const localCore = new URL("./core/", sdkURL);
  const core = existsSync(fileURLToPath(localCore)) ? localCore : new URL("../core/", sdkURL);
  const { getResourcePathKey } = await import(new URL("source-info.js", core).href);
  const { resolveProjectTrusted } = await import(new URL("project-trust.js", core).href);
  installResolver(DefaultPackageManager, getResourcePathKey);
  for (const trust of ["saved", "never", "ask", "extension"]) {
    await t.test(trust, async (t) => {
      const root = fixture(t);
      const cwd = join(root, "project");
      const agentDir = join(root, "agent");
      const extension = (path) => file(path, `export default function(pi) {
        globalThis.__wsTestLoads ??= {};
        globalThis.__wsTestLoads[${JSON.stringify(path)}] = (globalThis.__wsTestLoads[${JSON.stringify(path)}] ?? 0) + 1;
      }`);
      const user = extension(join(agentDir, "extensions/user.ts"));
      const explicit = extension(join(root, "explicit.ts"));
      const project = extension(join(cwd, ".pi/extensions/project.ts"));
      const pkg = join(root, "other-package");
      json(join(pkg, "package.json"), { name: "other", pi: { extensions: ["src/*.ts"] } });
      const keep = extension(join(pkg, "src/keep.ts"));
      file(join(pkg, "src/disabled.ts"), "throw new Error('disabled package extension executed')");
      const installed = [];
      for (const name of ["ws-pi-bridge", "wsflow"]) {
        const dir = join(root, "installed-" + name);
        json(join(dir, "package.json"), { name, pi: { extensions: ["src/index.ts"] } });
        file(join(dir, "src/index.ts"), "throw new Error('installed ws extension executed')");
        installed.push(dir);
      }
      const bundle = join(root, "bundled");
      json(join(bundle, "package.json"), { name: "ws-pi-bridge", pi: { extensions: ["src/index.ts"] } });
      const bundled = extension(join(bundle, "src/index.ts"));
      const extensions = [explicit, "-builtin:mcp"];
      if (trust === "extension") {
        extensions.push(file(join(agentDir, "extensions/trust.ts"), `export default function(pi) { pi.on('project_trust', () => ({trusted:'yes'})); }`));
      }
      const settings = { defaultProjectTrust: trust === "never" ? "never" : "ask", extensions,
        packages: [...installed, { source: pkg, extensions: ["src/keep.ts"] }] };
      json(join(agentDir, "settings.json"), settings);
      const manager = SettingsManager.create(cwd, agentDir, { projectTrusted: false });
      const trustStore = new ProjectTrustStore(agentDir);
      if (trust === "saved") trustStore.set(cwd, true);
      const loader = new DefaultResourceLoader({ cwd, agentDir, settingsManager: manager,
        noExtensions: true, additionalExtensionPaths: [bundled],
        extensionFactories: ["mcp", "codemode", "tool_search"].map((name) => ({ name, builtin: true, factory() {} })) });
      await loader.reload({ resolveProjectTrust: ({ extensionsResult }) => resolveProjectTrusted({
        cwd, trustStore, defaultProjectTrust: manager.getDefaultProjectTrust(), extensionsResult,
        projectTrustContext: { cwd, mode: "rpc", hasUI: false, ui: {} },
      }) });
      assert.deepEqual(loader.getExtensions().errors, []);
      const paths = () => loader.getExtensions().extensions.map((e) => getResourcePathKey(e.path));
      const has = (path) => paths().includes(getResourcePathKey(path));
      assert(has(bundled)); assert(has(user)); assert(has(explicit)); assert(has(keep));
      assert.equal(has(project), trust === "saved" || trust === "extension");
      assert.equal(has("builtin:mcp"), false);
      assert(has("builtin:codemode")); assert(has("builtin:tool_search"));
      assert.equal(paths().length, new Set(paths()).size);
      // The final order is bundled first followed by exactly Pi's native
      // enabled non-ws resolver output, with native path identities.
      const native = new DefaultPackageManager({ cwd, agentDir, settingsManager: manager, builtinExtensions: ["mcp", "codemode", "tool_search"] });
      const resolved = await native.resolve();
      const expected = [bundled, ...resolved.extensions.filter((r) => r.enabled && !isWS(r)).map((r) => r.path)].map(getResourcePathKey);
      assert.deepEqual(paths(), [...new Set(expected)]);
      for (const path of [user, explicit, keep, bundled, ...(has(project) ? [project] : [])]) {
        assert.equal(globalThis.__wsTestLoads[path], 1, "factory initialized once across trust passes: " + path);
      }
      // Native reload must re-read settings, not reuse a launch-time snapshot.
      settings.extensions.push("-" + user);
      json(join(agentDir, "settings.json"), settings);
      await loader.reload();
      assert.deepEqual(loader.getExtensions().errors, []);
      assert.equal(has(user), false);
      assert.equal(has(project), trust === "saved" || trust === "extension");
      assert(has(bundled));
    });
  }
});
