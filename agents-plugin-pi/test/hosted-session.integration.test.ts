/**
 * Hosted-session mode (hosted-session.ts) through the real extension entry:
 * under WS_HOSTED_SESSION with WS_HOSTED_SESSION_KEY, in the process the
 * registration at WS_HOSTED_SESSION_ROW names, the owner lead re-logs in
 * with the daemon key and never mints, runs no `mailbox wait`, and sends no
 * dashboard feed even with the knob on; a refused hosted key mints nothing;
 * the same variables in any other process, or without a registration path,
 * gate nothing; the flag without a key gates nothing and warns. A hosted
 * lead's subagents reach the daemon only as lines of the reserved widget.
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
import { serializeOrphans, sidecarPath } from "../src/agent-sidecar.ts";
import { HOSTED_AGENTS_WIDGET_KEY } from "../src/hosted-agent-rows.ts";
import { HOSTED_FLAG_ENV, HOSTED_KEY_ENV, HOSTED_ROW_ENV, MISSING_HOSTED_KEY_WARNING, ROW_POLL_MS, ROW_WAIT_MS, flagOn, readHostedMode, resolveHostedMode, rowVerdict, unconfirmedHostedWarning, type HostedProbe } from "../src/hosted-session.ts";

const ROLE_ENV_KEYS = ["WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE", "WS_PI_DELEGATION_POLICY", "WS_PI_PARENT_SESSION_KEY", HOSTED_FLAG_ENV, HOSTED_KEY_ENV, HOSTED_ROW_ENV] as const;
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

const row = (hostPid?: number) => JSON.stringify({ schema_version: 1, reply_id: "r", hosted: true, ...(hostPid === undefined ? {} : { host_pid: hostPid }) });

test("a registration names this process, another one, or none yet", () => {
  assert.equal(rowVerdict(row(4242), 4242), "mine");
  assert.equal(rowVerdict(row(5555), 4242), "other");
  for (const text of [row(0), row(), undefined, "", "{", "null", '{"host_pid":"4242"}', '{"host_pid":-1}', '{"host_pid":1.5}']) assert.equal(rowVerdict(text, 4242), "pending");
});

const HOSTED_VARS = { [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: " hosted-key ", [HOSTED_ROW_ENV]: " /state/registration.json " };

/** A probe over a registration a test rewrites, counting what the resolution ran. */
function probe(pid: number, text: string | undefined) {
  const p = { text, reads: [] as string[], slept: 0, onSleep: (_slept: number) => {} };
  const hooks: HostedProbe = {
    pid,
    readRow: async (path) => {
      p.reads.push(path);
      if (p.text === undefined) throw new Error("ENOENT");
      return p.text;
    },
    sleep: async (ms) => { p.slept += ms; p.onSleep(p.slept); },
  };
  return { p, hooks };
}

test("hosted only in the process the registration names; another process, or no registration path, is an ordinary session at once", async () => {
  const mine = probe(4242, row(4242));
  assert.deepEqual(await resolveHostedMode(HOSTED_VARS, mine.hooks), { mode: "hosted", key: "hosted-key" });
  assert.deepEqual(mine.p.reads, ["/state/registration.json"]);
  const other = probe(5555, row(4242));
  assert.deepEqual(await resolveHostedMode(HOSTED_VARS, other.hooks), { mode: "off" });
  assert.equal(other.p.slept, 0);
  for (const rowPath of [undefined, "", "  "]) {
    const none = probe(4242, row(4242));
    assert.deepEqual(await resolveHostedMode({ ...HOSTED_VARS, [HOSTED_ROW_ENV]: rowPath }, none.hooks), { mode: "off" });
    assert.deepEqual(none.p.reads, []);
  }
  const unread = probe(4242, row(4242));
  assert.deepEqual(await resolveHostedMode({ ...HOSTED_VARS, [HOSTED_FLAG_ENV]: "0" }, unread.hooks), { mode: "off" });
  assert.deepEqual(await resolveHostedMode({ ...HOSTED_VARS, [HOSTED_KEY_ENV]: " " }, unread.hooks), { mode: "missing-key" });
  assert.deepEqual(unread.p.reads, []);
});

test("a registration that names no process yet is read again until it does, and for no longer than the wait", async () => {
  for (const pending of [row(0), row(), undefined, "{"]) {
    for (const [written, want] of [[row(4242), { mode: "hosted", key: "hosted-key" }], [row(5555), { mode: "off" }]] as const) {
      const late = probe(4242, pending);
      late.p.onSleep = (slept) => { if (slept >= 5 * ROW_POLL_MS) late.p.text = written; };
      assert.deepEqual(await resolveHostedMode(HOSTED_VARS, late.hooks), want);
      assert.equal(late.p.slept, 5 * ROW_POLL_MS);
    }
  }
  const never = probe(4242, row(0));
  const got = await resolveHostedMode(HOSTED_VARS, never.hooks);
  assert.equal(got.mode, "unconfirmed");
  assert.equal(never.p.slept, ROW_WAIT_MS);
  assert.match(unconfirmedHostedWarning(got.mode === "unconfirmed" ? got.reason : ""), /^ws-pi: WS_HOSTED_SESSION is set but \/state\/registration\.json named no harness process within 3 s/);
});

type Session = { calls: () => { name: string; args: Record<string, unknown> }[]; waits: () => string[][]; notices: string[]; widgets: { key: string; lines: unknown }[]; session: any };

/**
 * A lead session under `env`. With `hostPid` given, the session's
 * registration file names that process (0: none yet) and the environment
 * points at it; `lateHostPid` is written to it shortly after the session
 * starts, while its start still waits on the registration. `seed` runs
 * with the session's file path before the session starts.
 */
async function withSession(env: Record<string, string>, body: (s: Session) => Promise<void>, hostPid?: number, lateHostPid?: number, seed?: (sessionFile: string) => void): Promise<void> {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-hosted-"));
  const savedEnv = { ...process.env };
  let session: any;
  let lateWrite: ReturnType<typeof setTimeout> | undefined;
  try {
    const rowPath = join(directory, "registration.json");
    if (hostPid !== undefined) {
      writeFileSync(rowPath, row(hostPid));
      env = { ...env, [HOSTED_ROW_ENV]: rowPath };
    }
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
    const widgets: Session["widgets"] = [];
    const noop = () => {};
    const ui = new Proxy({ notify: (message: string) => { notices.push(message); }, setWidget: (key: string, lines: unknown) => { widgets.push({ key, lines }); }, custom: async () => undefined, setFooter: noop }, { get: (target: any, key) => target[key] ?? noop });
    seed?.(session.sessionManager.getSessionFile());
    if (lateHostPid !== undefined) lateWrite = setTimeout(() => writeFileSync(rowPath, row(lateHostPid)), 5 * ROW_POLL_MS);
    await session.bindExtensions({ mode: "rpc", uiContext: ui, onError: (e: any) => { throw e; } });
    const lines = (path: string) => existsSync(path) ? readFileSync(path, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line)) : [];
    await body({ calls: () => lines(calls), waits: () => lines(waits), notices, widgets, session });
  } finally {
    clearTimeout(lateWrite);
    if (session) { await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" }); session.dispose(); }
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(directory, { recursive: true, force: true });
  }
}

const ferrules = (calls: { name: string; args: Record<string, unknown> }[]) => calls.filter((c) => c.name === "ferrule").map((c) => c.args);
const intakes = (calls: { name: string }[]) => calls.filter((c) => c.name === "dashboard.intake");
const agentRows = (widgets: Session["widgets"]) => widgets.filter((w) => w.key === HOSTED_AGENTS_WIDGET_KEY);

test("hosted: the daemon key is re-logged in and nothing minted; no mailbox wait, no feed with the knob on; a foreign revive adopts nothing", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices, widgets, session }) => {
    await until(() => ferrules(calls()).length >= 1, "the bootstrap ferrule", WAIT);
    await settle(1_500);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), ["hosted-key"]);
    assert.deepEqual(waits(), [], "no mailbox wait runs in a hosted session");
    assert.deepEqual(intakes(calls()), [], "no dashboard feed in a hosted session");
    assert.deepEqual(agentRows(widgets), [], "with no subagent, no rows and nothing lead-level");
    assert.deepEqual(notices.filter((n) => n.includes(HOSTED_FLAG_ENV)), []);
    const manual = session.extensionRunner.getToolDefinition("ws__workflow_manual");
    await manual.execute("revive", { session_key: "foreign-key" }, undefined, undefined, { sessionManager: { buildContextEntries: () => [] } });
    await settle(500);
    assert.deepEqual(waits(), [], "a refused revive arms nothing");
    assert.ok(!ferrules(calls()).some((a) => a.relogin_session_key === undefined), "nothing was minted");
  }, process.pid);
});

test("hosted: the subagent tree reaches the daemon as reserved widget lines, never as an intake", { timeout: 120_000 }, async () => {
  const child = { agentId: "child-1", alias: "audit", title: "audit", prompt: "look", sessionPath: "/nowhere/child-1.jsonl", systemPromptPath: "/nowhere/prompt.md", wsToolNames: [], toolGroup: "full-worker" };
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, widgets }) => {
    await until(() => agentRows(widgets).length >= 1, "the subagent rows widget", WAIT);
    await settle(500);
    const events = agentRows(widgets).flatMap((w) => (w.lines as string[]).map((line) => JSON.parse(line)));
    assert.ok(events.length > 0);
    for (const event of events) {
      assert.ok(event.kind === "agent" || event.kind === "permission", JSON.stringify(event));
      assert.equal(event.agentId, "child-1");
    }
    assert.equal(events[0].kind, "agent");
    assert.equal(events[0].status, "done", "a revived dormant child is not running");
    assert.deepEqual(intakes(calls()), [], "still no dashboard feed in a hosted session");
  }, process.pid, undefined, (sessionFile) => { writeFileSync(sidecarPath(sessionFile), serializeOrphans([child as never])); });
});

test("hosted once the registration names this process: the bootstrap waits for a registration that names none yet", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices }) => {
    await until(() => ferrules(calls()).length >= 1, "the bootstrap ferrule", WAIT);
    await settle(1_500);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), ["hosted-key"]);
    assert.deepEqual(waits(), []);
    assert.deepEqual(intakes(calls()), []);
    assert.deepEqual(notices.filter((n) => n.includes(HOSTED_FLAG_ENV)), []);
  }, 0, process.pid);
});

test("a registration that never names a process: an ordinary session after the wait, with one warning", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices }) => {
    await until(() => waits().length >= 1, "a mailbox wait on the minted key", WAIT);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), [undefined]);
    const warned = notices.filter((n) => n.includes(HOSTED_FLAG_ENV));
    assert.equal(warned.length, 1, warned.join(" | "));
    assert.match(warned[0], /registration\.json named no harness process within 3 s/);
  }, 0);
});

test("hosted variables inherited by another process gate nothing and warn of nothing: a minted key, a mailbox wait, and the feed", { timeout: 120_000 }, async () => {
  // PID 1 is never this process: the registration names another harness.
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices, widgets }) => {
    await until(() => waits().length >= 1, "a mailbox wait on the minted key", WAIT);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), [undefined]);
    await until(() => intakes(calls()).length >= 1, "a dashboard intake call", WAIT);
    assert.deepEqual(notices.filter((n) => n.includes(HOSTED_FLAG_ENV)), []);
    assert.deepEqual(agentRows(widgets), [], "an ordinary session sends no rows widget");
  }, 1);
});

test("hosted variables without a registration path gate nothing and warn of nothing", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1", [HOSTED_KEY_ENV]: "hosted-key" }, async ({ calls, waits, notices }) => {
    await until(() => waits().length >= 1, "a mailbox wait on the minted key", WAIT);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), [undefined]);
    assert.deepEqual(notices.filter((n) => n.includes(HOSTED_FLAG_ENV)), []);
  });
});

test("hosted: a refused daemon key mints nothing and warns", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "on", [HOSTED_KEY_ENV]: "stale-key" }, async ({ calls, waits, notices }) => {
    await until(() => notices.some((n) => n.includes("stale-key")), "the refusal warning", WAIT);
    await settle(500);
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), ["stale-key"]);
    assert.match(notices.find((n) => n.includes("stale-key"))!, /WS_HOSTED_SESSION_KEY.*no key was minted/);
    assert.deepEqual(waits(), []);
  }, process.pid);
});

test("the flag without a key gates nothing: a warning, a minted key, a mailbox wait, and the feed", { timeout: 120_000 }, async () => {
  await withSession({ [HOSTED_FLAG_ENV]: "1" }, async ({ calls, waits, notices }) => {
    await until(() => waits().length >= 1, "a mailbox wait on the minted key", WAIT);
    assert.ok(notices.includes(MISSING_HOSTED_KEY_WARNING), notices.join(" | "));
    assert.deepEqual(ferrules(calls()).map((a) => a.relogin_session_key), [undefined]);
    await until(() => intakes(calls()).length >= 1, "a dashboard intake call", WAIT);
  });
});
