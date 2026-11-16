// Pure Node tests: import only the watchdog with a fake Pi API; never a CLI.
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";

const source = readFileSync(new URL("../pi_lead_bridge.mjs", import.meta.url), "utf8");
async function watchdog(servers = null, launchWarning = "") {
  const prelude = `const servers = ${JSON.stringify(servers)}; const sessionKey = 'lead-key'; const launchWarning = ${JSON.stringify(launchWarning)};\n`;
  return import("data:text/javascript;base64," + Buffer.from(prelude + source).toString("base64"));
}
function host(t, version, name = "@earendil-works/pi-coding-agent") {
  const root = mkdtempSync(join(tmpdir(), "ws-pi-health-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(join(root, "dist/bundle"), { recursive: true });
  writeFileSync(join(root, "package.json"), JSON.stringify({ name, version }));
  const cli = join(root, "dist/bundle/cli.js");
  writeFileSync(cli, "throw new Error('must never execute');");
  return cli;
}
function api(register, initialTools) {
  const handlers = {}, notices = [], registered = {}, signals = {};
  let tools = initialTools;
  register({
    events: { on(event, handler) { signals[event] = handler; } },
    on(event, handler) { handlers[event] = handler; },
    getAllTools() { return tools.map((name) => ({ name })); },
    registerMcpServer(name, config) { registered[name] = config; },
  });
  const ctx = { ui: { notify(message, type) { notices.push({ message, type }); } } };
  return { handlers, notices, registered, ctx, ready() { signals["ws:lead-initialized"](); }, tools(names) { tools = names; } };
}

test("host range agrees with the pinned development baseline and peer declaration", async () => {
  const { supportedPiRange } = await watchdog();
  const pkg = JSON.parse(readFileSync(new URL("../../../../agents-plugin-pi/package.json", import.meta.url), "utf8"));
  assert.equal(pkg.devDependencies["@earendil-works/pi-coding-agent"], "0.84.4");
  assert.equal(pkg.peerDependencies["@earendil-works/pi-coding-agent"], supportedPiRange);
});

test("launch checks actual host metadata, including range boundaries and prereleases", async (t) => {
  const { piVersionWarning } = await watchdog();
  for (const version of ["0.84.4", "0.84.5", "0.85.0", "0.99.99", "0.84.4+local"]) {
    assert.equal(piVersionWarning(host(t, version)), "", version);
  }
  for (const version of ["0.84.3", "0.83.99", "1.0.0", "1.5.2", "0.84.4-rc.1", "garbage", "00.84.4", undefined]) {
    const warning = piVersionWarning(host(t, version));
    assert.match(warning, /UNSUPPORTED HOST/);
    assert.match(warning, /supported @earendil-works\/pi-coding-agent >=0.84.4 <1.0.0/);
    assert.match(warning, /Continuing so mail remains reachable/);
  }
  assert.match(piVersionWarning("missing-host"), /version unknown/);
  assert.match(piVersionWarning(host(t, "0.84.4", "unrelated")), /version unknown/);
});

test("version follows the CLI symlink instead of project-local dependencies", async (t) => {
  const { piVersionWarning } = await watchdog();
  const cli = host(t, "0.84.4");
  const alias = join(dirname(cli), "alias.js");
  try { symlinkSync(cli, alias); } catch (error) { if (process.platform === "win32") return t.skip(String(error)); throw error; }
  assert.equal(piVersionWarning(alias), "");
});

test("supported healthy host starts without fallback or a false alarm", async (t) => {
  const previous = process.argv[1];
  process.argv[1] = host(t, "0.84.4");
  t.after(() => { process.argv[1] = previous; });
  const { default: register } = await watchdog();
  const p = api(register, ["ws-execute", "ws__playbook_read"]);
  p.ready();
  p.handlers.session_start({}, p.ctx);
  p.handlers.agent_start({}, p.ctx);
  assert.equal(p.handlers.before_agent_start({ systemPrompt: "unchanged" }, p.ctx), undefined);
  assert.deepEqual(p.notices, []);
  assert.deepEqual(p.registered, {});
});

test("unsupported host warns at session start and in the first model system prompt", async (t) => {
  const previous = process.argv[1];
  process.argv[1] = host(t, "1.5.2");
  t.after(() => { process.argv[1] = previous; });
  const { default: register } = await watchdog();
  const p = api(register, ["ws-execute", "ws__playbook_read"]);
  p.ready();
  p.handlers.session_start({}, p.ctx);
  assert.match(p.notices[0].message, /UNSUPPORTED HOST: Pi 1.5.2/);
  assert.match(p.handlers.before_agent_start({ systemPrompt: "preserved" }, p.ctx).systemPrompt, /UNSUPPORTED HOST: Pi 1.5.2/);
  assert.deepEqual(p.registered, {}, "version skew alone does not replace a usable extension");
});

test("a failed native initialization gets a dashboard notification and first-turn system banner", async () => {
  const { default: register } = await watchdog();
  const p = api(register, ["read", "mcp__ws__playbook_read"]);
  p.handlers.session_start({}, p.ctx);
  assert.equal(p.notices.length, 1);
  assert.equal(p.notices[0].type, "error");
  assert.match(p.notices[0].message, /EXTENSION NOT LOADED OR INCOMPLETE.*ws-execute, ws__playbook_read/);
  const prompt = "Pi defaults\nUser APPEND_SYSTEM.md\nProject instructions";
  const result = p.handlers.before_agent_start({ systemPrompt: prompt }, p.ctx);
  assert(result.systemPrompt.startsWith(prompt + "\n\n"));
  assert.match(result.systemPrompt, /do not assume its tools exist/);
  assert.equal(p.notices.length, 1, "do not spam unchanged health notifications");
  p.handlers.agent_start({}, p.ctx);
  assert.equal(p.notices.length, 2, "repeat after the runner clears the prior turn's error");
  p.tools(["ws-execute", "ws__playbook_read"]);
  p.ready();
  const recovered = p.handlers.before_agent_start({ systemPrompt: prompt }, p.ctx);
  assert(!recovered?.systemPrompt.includes("EXTENSION NOT LOADED"));
  assert.deepEqual(p.registered, {}, "healthy load never registers an MCP fallback");
});

test("partial initialization with both sentinel tools still fails until completion is signalled", async (t) => {
  const previous = process.argv[1];
  process.argv[1] = host(t, "0.84.4");
  t.after(() => { process.argv[1] = previous; });
  const { default: register } = await watchdog();
  const p = api(register, ["ws-execute", "ws__playbook_read"]);
  // Native session_start registered the tools, then its final awaited work
  // threw. Pi catches the exception and still calls the watchdog's handler.
  p.handlers.session_start({}, p.ctx);
  assert.match(p.notices[0].message, /initialization did not complete/);
  assert.match(p.handlers.before_agent_start({ systemPrompt: "preserved" }, p.ctx).systemPrompt, /registered tools may be unusable/);
  p.ready();
  assert.equal(p.handlers.before_agent_start({ systemPrompt: "preserved" }, p.ctx), undefined);
  p.handlers.session_shutdown({}, p.ctx);
  assert.match(p.handlers.before_agent_start({ systemPrompt: "new session" }, p.ctx).systemPrompt, /initialization did not complete/, "shutdown must reset readiness");
});

test("native completion signal is emitted only after final awaited startup and epoch validation", () => {
  const native = readFileSync(new URL("../../../../agents-plugin-pi/src/index.ts", import.meta.url), "utf8");
  const start = native.indexOf("await applySessionStartAgentFooter(");
  const finish = native.slice(start, native.indexOf('pi.on("session_shutdown"', start));
  assert.match(finish, /if \(startEpoch !== sessionStartEpoch\) await disposeStaleBootstrap\(\);[\s\S]*else \{[\s\S]*pi\.events\.emit\("ws:lead-initialized", undefined\)/);
  assert.equal(native.split('pi.events.emit("ws:lead-initialized", undefined)').length, 2, "one successful completion emission site");
});

test("known bundle failure remains loud while the bridge preserves hosted identity and mail", async () => {
  const servers = { ws: { command: "never-executed", exposure: "direct" } };
  const warning = "[ws Pi] EXTENSION NOT LOADED: stale bundle; ws-execute, explore, ws__* unavailable; using the ws MCP bridge.";
  const { default: register } = await watchdog(servers, warning);
  const p = api(register, []);
  assert.deepEqual(p.registered, servers);
  p.handlers.session_start({}, p.ctx);
  const { systemPrompt } = p.handlers.before_agent_start({ systemPrompt: "preserved" }, p.ctx);
  assert(systemPrompt.startsWith("preserved\n\n" + warning));
  assert.match(systemPrompt, /Your ws session key is "lead-key"/);
  assert.match(systemPrompt, /do not mint a replacement/);
  assert.equal(p.notices[0].type, "error");
});
