/**
 * Hosted-session mode (hosted-session.ts) through the real extension entry:
 * under WS_HOSTED_SESSION with WS_HOSTED_SESSION_KEY the owner lead re-logs
 * in with the daemon key and never mints, runs no `mailbox wait`, and sends
 * no dashboard feed even with the knob on; a refused hosted key mints
 * nothing; the flag without a key gates nothing and warns.
 *
 * Harness pattern: `mailbox-rearm.integration.test.ts` (a copied adapter with
 * an offline MCP launcher that journals every tool call and wait argv).
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as sdk from "@earendil-works/pi-coding-agent";
import { until } from "./fixtures/subtree-channels.ts";
import { HOSTED_FLAG_ENV, HOSTED_KEY_ENV, MISSING_HOSTED_KEY_WARNING, flagOn, readHostedMode } from "../src/hosted-session.ts";

const ROLE_ENV_KEYS = ["WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE", "WS_PI_DELEGATION_POLICY", "WS_PI_PARENT_SESSION_KEY", HOSTED_FLAG_ENV, HOSTED_KEY_ENV] as const;
const WAIT = { timeoutMs: 20_000, intervalMs: 10 };
const settle = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

test("the hosted mode follows ws-mcp's flag spellings and needs a non-empty key", () => {
  for (const value of ["1", "TRUE", " yes ", "on"]) assert.equal(flagOn(value), true);
  for (const value of [undefined, "", "0", "off", "hosted"]) assert.equal(flagOn(value), false);
  assert.deepEqual(readHostedMode({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: " k " }), { mode: "hosted", key: "k" });
  assert.deepEqual(readHostedMode({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "  " }), { mode: "missing-key" });
  assert.deepEqual(readHostedMode({ [HOSTED_FLAG_ENV]: "1" }), { mode: "missing-key" });
  assert.deepEqual(readHostedMode({ [HOSTED_KEY_ENV]: "k" }), { mode: "off" });
});

async function withSession(env: Record<string, string>, body: (s: { calls: () => { name: string; args: Record<string, unknown> }[]; waits: () => string[][]; notices: string[]; session: any }) => Promise<void>): Promise<void> {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-hosted-"));
  const savedEnv = { ...process.env };
  let session: any;
  try {
    const plugin = join(directory, "plugin");
    mkdirSync(plugin);
    for (const name of ["src", "runtime.json", "config-manifest.json", "pi-lead-guide.md", "lead-compact-guide.md", "execute-worker-guide.md", "explore-guide.md"]) cpSync(join(process.cwd(), name), join(plugin, name), { recursive: true });
    symlinkSync(join(process.cwd(), "node_modules"), join(plugin, "node_modules"), "junction");
    mkdirSync(join(plugin, "bin"));
    const version = JSON.parse(readFileSync(join(plugin, "runtime.json"), "utf8")).plugin_version;
    const waits = join(directory, "waits.jsonl");
    const calls = join(directory, "calls.jsonl");
    // ws-mcp accepts a re-login only with `hosted-key`; a plain ferrule mints `minted-key`.
    writeFileSync(join(plugin, "bin/ws-mcp-launcher.py"), `import sys,json,time
if sys.argv[1:3]==['mailbox','wait']:
 with open(${JSON.stringify(waits)},'a') as f: f.write(json.dumps(sys.argv[3:])+'\\n')
 time.sleep(0.1)
 raise SystemExit(3)
def ok(t): return {'isError':False,'content':[{'type':'text','text':t}]}
def err(t): return {'isError':True,'content':[{'type':'text','text':t}]}
for line in sys.stdin:
 q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}
 if m=='initialize': r={'serverInfo':{'name':'offline','version':${JSON.stringify(version)}},'capabilities':{}}
 elif m=='tools/list': r={'tools':[{'name':'workflow_manual','description':'workflow_manual','inputSchema':{'type':'object','properties':{'session_key':{'type':'string'}},'required':['session_key']}}]}
 elif m=='tools/call':
  n=p['name']; a=p.get('arguments') or {}
  with open(${JSON.stringify(calls)},'a') as f: f.write(json.dumps({'name':n,'args':a})+'\\n')
  if n=='ferrule':
   k=a.get('relogin_session_key')
   r=(ok(json.dumps({'session_key':k})) if k=='hosted-key' else err('refused: key is not the hosted key')) if k is not None else ok(json.dumps({'session_key':'minted-key'}))
  elif n=='workflow_manual': r=ok('# Workflow Manual\\nstatic body\\n\\n## Session Key\\n%s\\n'%a.get('session_key'))
  elif n=='playbook.read': r=ok('# Workflow Manual\\nstatic body')
  elif n=='mailbox.lookup_peers': r=ok(json.dumps({'self':{},'peers':[]}))
  elif n=='mailbox.recv': r=ok('[]')
  elif n=='config.get' and str(a.get('key','')).endswith('dashboard_enabled'): r=ok(json.dumps({'value': True}))
  elif n=='dashboard.intake': r=ok('ok: 1 of 1 events applied\\n')
  else: r=ok('{}')
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);
    for (const key of ROLE_ENV_KEYS) delete process.env[key];
    Object.assign(process.env, env);
    process.env.PI_OFFLINE = "1";
    const agentDir = join(directory, "agent");
    mkdirSync(agentDir);
    const settingsManager = sdk.SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } });
    const modelRuntime = await sdk.ModelRuntime.create({ authPath: join(agentDir, "auth.json"), modelsPath: join(agentDir, "models.json"), modelsStorePath: join(agentDir, "store.json"), allowModelNetwork: false });
    const resourceLoader = new sdk.DefaultResourceLoader({ cwd: directory, agentDir, settingsManager, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true, additionalExtensionPaths: [join(plugin, "src/index.ts")] });
    await resourceLoader.reload();
    assert.deepEqual(resourceLoader.getExtensions().errors, []);
    ({ session } = await sdk.createAgentSession({ cwd: directory, agentDir, sessionManager: sdk.SessionManager.create(directory, join(directory, "sessions")), resourceLoader, modelRuntime, settingsManager }));
    const notices: string[] = [];
    const noop = () => {};
    const ui = new Proxy({ notify: (message: string) => { notices.push(message); }, custom: async () => undefined, setFooter: noop }, { get: (target: any, key) => target[key] ?? noop });
    await session.bindExtensions({ mode: "rpc", uiContext: ui, onError: (e: any) => { throw e; } });
    const lines = (path: string) => existsSync(path) ? readFileSync(path, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line)) : [];
    await body({ calls: () => lines(calls), waits: () => lines(waits), notices, session });
  } finally {
    if (session) { await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" }); session.dispose(); }
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(directory, { recursive: true, force: true });
  }
}

const ferrules = (calls: { name: string; args: Record<string, unknown> }[]) => calls.filter((c) => c.name === "ferrule").map((c) => c.args);
const intakes = (calls: { name: string }[]) => calls.filter((c) => c.name === "dashboard.intake");

test("hosted: the daemon key is re-logged in and nothing minted; no mailbox wait, no feed with the knob on; a foreign revive adopts nothing", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices, session }) => {
    await until(() => ferrules(calls()).length >= 1, "the bootstrap ferrule", WAIT);
    await settle(1_500);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), ["hosted-key"]);
    assert.deepEqual(waits(), [], "no mailbox wait runs in a hosted session");
    assert.deepEqual(intakes(calls()), [], "no dashboard feed in a hosted session");
    assert.deepEqual(notices.filter((n) => n.includes(HOSTED_KEY_ENV)), []);
    const manual = session.extensionRunner.getToolDefinition("ws__workflow_manual");
    await manual.execute("revive", { session_key: "foreign-key" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    await settle(500);
    assert.deepEqual(waits(), [], "a refused revive arms nothing");
    assert.ok(!ferrules(calls()).some((a) => a.relogin_session_key === undefined), "nothing was minted");
  });
});

test("hosted: a refused daemon key mints nothing and warns", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "on", [HOSTED_KEY_ENV]: "stale-key" }, async ({ calls, waits, notices }) => {
    await until(() => notices.some((n) => n.includes("stale-key")), "the refusal warning", WAIT);
    await settle(500);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), ["stale-key"]);
    assert.match(notices.find((n) => n.includes("stale-key"))!, /WS_HOSTED_SESSION_KEY.*no key was minted/);
    assert.deepEqual(waits(), []);
  });
});

test("the flag without a key gates nothing: a warning, a minted key, a mailbox wait, and the feed", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1" }, async ({ calls, waits, notices }) => {
    await until(() => waits().length >= 1, "a mailbox wait on the minted key", WAIT);
    assert.ok(notices.includes(MISSING_HOSTED_KEY_WARNING), notices.join(" | "));
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), [undefined]);
    await until(() => intakes(calls()).length >= 1, "a dashboard intake call", WAIT);
  });
});
