/**
 * The owner lead's mailbox waiter follows a revive adoption (bridge.ts
 * `adoptRevivedKey`). ws-mcp moves the named-inbox owner to the re-logged-in
 * key, so a waiter still armed with the old `--session-key` degrades to a
 * reply-id-only wait on the old queue and mail stops waking the lead; the
 * real extension entry must re-arm on the new key with its newly resolved
 * `--slug`, stop the old waiter, and show a persistent stderr warning once
 * per armed waiter rather than on every 10-minute re-spawn.
 *
 * Harness pattern: `push-flush-order.integration.test.ts` (a copied adapter
 * with an offline MCP launcher, real SDK session and extension runner). The
 * launcher is release-backed (no local-devenv marker), so every `mailbox
 * wait` re-enters it and it journals each wait's argv.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as sdk from "@earendil-works/pi-coding-agent";
import { until } from "./fixtures/subtree-channels.ts";

const ROLE_ENV_KEYS = ["WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE", "WS_PI_DELEGATION_POLICY", "WS_PI_PARENT_SESSION_KEY"] as const;
const WAIT = { timeoutMs: 20_000, intervalMs: 10 };
const WARNING = "warning: persistent mailbox condition";

test("a revive adoption re-arms the lead's mailbox waiter on the new key and slug; a re-arm racing shutdown starts nothing", { timeout: 120_000 }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-mailbox-rearm-"));
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
    const lateLookupStarted = join(directory, "late-lookup-started");
    // Offline ws MCP server and `mailbox wait` CLI in one script. Each wait
    // prints the same warning on stderr, then times out (exit 3) so the
    // waiter re-spawns it like the real 10-minute window. `late-key`'s slug
    // lookup stalls so a shutdown can land while its re-arm is pending.
    writeFileSync(join(plugin, "bin/ws-mcp-launcher.py"), `import sys,json,time
if sys.argv[1:3]==['mailbox','wait']:
 with open(${JSON.stringify(waits)},'a') as f: f.write(json.dumps(sys.argv[3:])+'\\n')
 print(${JSON.stringify(WARNING)},file=sys.stderr,flush=True)
 time.sleep(0.1)
 raise SystemExit(3)
accept={'revived-key','late-key'}
def ok(t): return {'isError':False,'content':[{'type':'text','text':t}]}
def err(t): return {'isError':True,'content':[{'type':'text','text':t}]}
for line in sys.stdin:
 q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}
 if m=='initialize': r={'serverInfo':{'name':'offline','version':${JSON.stringify(version)}},'capabilities':{}}
 elif m=='tools/list': r={'tools':[{'name':'workflow_manual','description':'workflow_manual','inputSchema':{'type':'object','properties':{'session_key':{'type':'string'}},'required':['session_key']}}]}
 elif m=='tools/call':
  n=p['name']; a=p.get('arguments') or {}
  if n=='ferrule':
   k=a.get('relogin_session_key')
   r=(ok(json.dumps({'session_key':k})) if k in accept else err('unknown key')) if k is not None else ok(json.dumps({'session_key':'minted-key'}))
  elif n=='workflow_manual': r=ok('# Workflow Manual\\nstatic body\\n\\n## Session Key\\n%s\\n'%a.get('session_key'))
  elif n=='playbook.read': r=ok('# Workflow Manual\\nstatic body')
  elif n=='mailbox.lookup_peers':
   k=a.get('session_key')
   if k=='late-key':
    open(${JSON.stringify(lateLookupStarted)},'w').close(); time.sleep(0.8)
   r=ok(json.dumps({'self':{'address':'inbox-%s@worktree'%k},'peers':[]}))
  elif n=='mailbox.recv': r=ok('[]')
  else: r=ok('{}')
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);

    for (const key of ROLE_ENV_KEYS) delete process.env[key];
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

    const waitArgs = (): string[][] => existsSync(waits) ? readFileSync(waits, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line)) : [];
    const flag = (args: string[], name: string): string | undefined => { const at = args.indexOf(name); return at < 0 ? undefined : args[at + 1]; };
    const waitsFor = (key: string): string[][] => waitArgs().filter((args) => flag(args, "--session-key") === key);
    const warnings = (): number => notices.filter((notice) => notice === `[ws-mailbox] ${WARNING}`).length;

    await until(() => waitsFor("minted-key").length >= 3, "three re-spawned waits on the bootstrap key", WAIT);
    assert.equal(flag(waitsFor("minted-key")[0]!, "--slug"), "inbox-minted-key@worktree");
    assert.equal(warnings(), 1, `a persistent warning shows once per armed waiter: ${notices.join(" | ")}`);

    const manual = session.extensionRunner.getToolDefinition("ws__workflow_manual");
    const ctx = { sessionManager: { buildContextEntries: () => [] } };
    await manual.execute("revive", { session_key: "revived-key" }, undefined, undefined, ctx);
    await until(() => waitsFor("revived-key").length >= 3, "re-spawned waits on the adopted key", WAIT);
    assert.equal(flag(waitsFor("revived-key")[0]!, "--slug"), "inbox-revived-key@worktree", "the slug is re-resolved for the adopted key");
    const mintedAfterRearm = waitsFor("minted-key").length;
    const revivedSoFar = waitsFor("revived-key").length;
    await until(() => waitsFor("revived-key").length >= revivedSoFar + 2, "further waits on the adopted key", WAIT);
    assert.equal(waitsFor("minted-key").length, mintedAfterRearm, "the old key's waiter is stopped");
    assert.equal(warnings(), 2, `the re-armed waiter may show the warning once more: ${notices.join(" | ")}`);

    // An unchanged key adopts nothing, so nothing re-arms.
    await manual.execute("same", { session_key: "revived-key" }, undefined, undefined, ctx);
    // A refused re-login adopts nothing either.
    await manual.execute("refused", { session_key: "foreign-key" }, undefined, undefined, ctx);
    const lookupsBefore = waitArgs().length;
    await until(() => waitArgs().length >= lookupsBefore + 2, "the live waiter keeps re-spawning", WAIT);
    assert.deepEqual([...new Set(waitArgs().slice(lookupsBefore).map((args) => flag(args, "--session-key")))], ["revived-key"]);
    assert.equal(warnings(), 2, "no re-arm, so no repeated warning");

    // A re-arm whose slug lookup is still pending when the session shuts down
    // must start nothing: no waiter of this generation outlives its bridge.
    await manual.execute("late", { session_key: "late-key" }, undefined, undefined, ctx);
    await until(() => existsSync(lateLookupStarted), "the late key's slug lookup", WAIT);
    await session.extensionRunner.emit({ type: "session_shutdown", reason: "reload" });
    const afterShutdown = waitArgs().length;
    await new Promise((resolve) => setTimeout(resolve, 1_200));
    assert.deepEqual(waitsFor("late-key"), [], "the superseded re-arm never spawned a wait");
    assert.equal(waitArgs().length, afterShutdown, "no waiter survives shutdown");
  } finally {
    if (session) { await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" }); session.dispose(); }
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(directory, { recursive: true, force: true });
  }
});

test("dashboard messages: an idle wake injects at once with the whiteboard label; a mid-turn one is held and injected at the settle; the knob off arms no inbox flag", { timeout: 120_000 }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-board-inbox-"));
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
    const drains = join(directory, "drains.jsonl");
    const held = join(directory, "held");
    const knobOff = join(directory, "knob-off");
    const inputs = join(directory, "inputs.jsonl");
    // The wait peeks: with --board-inbox it wakes at once while `held`
    // exists and reports the board kind; the drain claims (removes) it.
    writeFileSync(join(plugin, "bin/ws-mcp-launcher.py"), `import sys,json,time,os
HELD=${JSON.stringify(held)}
if sys.argv[1:3]==['mailbox','wait']:
 with open(${JSON.stringify(waits)},'a') as f: f.write(json.dumps(sys.argv[3:])+'\\n')
 if '--board-inbox' in sys.argv and os.path.exists(HELD):
  print(json.dumps({'timed_out':False,'unread':0,'board':1,'kinds':['board']}),flush=True)
  raise SystemExit(0)
 time.sleep(0.1)
 raise SystemExit(3)
def ok(t): return {'isError':False,'content':[{'type':'text','text':t}]}
for line in sys.stdin:
 q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}
 if m=='initialize': r={'serverInfo':{'name':'offline','version':${JSON.stringify(version)}},'capabilities':{}}
 elif m=='tools/list': r={'tools':[]}
 elif m=='tools/call':
  n=p['name']; a=p.get('arguments') or {}
  if n=='ferrule': r=ok(json.dumps({'session_key':'board-key'}))
  elif n=='workflow_manual': r=ok('# Workflow Manual\\n\\n## Session Key\\nboard-key\\n')
  elif n=='mailbox.lookup_peers': r=ok(json.dumps({'self':{},'peers':[]}))
  elif n=='mailbox.recv': r=ok('[]')
  elif n=='config.get' and str(a.get('key','')).endswith('dashboard_enabled'): r=ok(json.dumps({'value': not os.path.exists(${JSON.stringify(knobOff)})}))
  elif n=='dashboard.inbox_drain':
   msgs=[]
   if os.path.exists(HELD):
    msgs=[{'id':'e1','text':open(HELD).read()}]; os.remove(HELD)
   with open(${JSON.stringify(drains)},'a') as f: f.write(json.dumps([x['text'] for x in msgs])+'\\n')
   r=ok(json.dumps({'messages':msgs}))
  else: r=ok('{}')
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);
    // Records every input the session receives, so an injection is seen
    // without a model to run the turn.
    const probe = join(directory, "probe.ts");
    writeFileSync(probe, `import { appendFileSync } from "node:fs";
export default function (pi: any) {
  pi.on("input", (event: any) => { appendFileSync(${JSON.stringify(inputs)}, JSON.stringify({ text: event.text, source: event.source }) + "\\n"); });
}
`);

    for (const key of ROLE_ENV_KEYS) delete process.env[key];
    process.env.PI_OFFLINE = "1";
    const agentDir = join(directory, "agent");
    mkdirSync(agentDir);
    const settingsManager = sdk.SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } });
    const modelRuntime = await sdk.ModelRuntime.create({ authPath: join(agentDir, "auth.json"), modelsPath: join(agentDir, "models.json"), modelsStorePath: join(agentDir, "store.json"), allowModelNetwork: false });
    const resourceLoader = new sdk.DefaultResourceLoader({ cwd: directory, agentDir, settingsManager, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true, additionalExtensionPaths: [join(plugin, "src/index.ts"), probe] });
    await resourceLoader.reload();
    assert.deepEqual(resourceLoader.getExtensions().errors, []);
    ({ session } = await sdk.createAgentSession({ cwd: directory, agentDir, sessionManager: sdk.SessionManager.create(directory, join(directory, "sessions")), resourceLoader, modelRuntime, settingsManager }));
    const noop = () => {};
    const ui = new Proxy({ notify: noop, custom: async () => undefined, setFooter: noop }, { get: (target: any, key) => target[key] ?? noop });
    // With no model the injected prompt fails after its input event; that error is expected here.
    await session.bindExtensions({ mode: "rpc", uiContext: ui, onError: noop });

    const lines = (path: string): any[] => existsSync(path) ? readFileSync(path, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line)) : [];
    const flagged = (): boolean[] => lines(waits).map((args: string[]) => args.includes("--board-inbox"));
    const injected = (): string[] => lines(inputs).map((input) => input.text);
    const label = "[user message from the ws whiteboard]";

    await until(() => flagged().length >= 2, "the first waits", WAIT);
    assert.ok(flagged().every(Boolean), "an idle lead's wait carries the inbox flag");

    // Idle: injected at once.
    writeFileSync(held, "idle hello\n\n(01) ok");
    await until(() => injected().length === 1, "the idle injection", WAIT);
    assert.equal(injected()[0], `${label}\nidle hello\n\n(01) ok`);
    assert.equal(lines(inputs)[0].source, "extension");
    assert.equal(existsSync(held), false);

    // Mid-turn: held until the settle.
    await session.extensionRunner.emit({ type: "agent_start" });
    const beforeHold = flagged().length;
    writeFileSync(held, "busy hello");
    await until(() => flagged().slice(beforeHold).includes(false), "a re-arm without the inbox flag", WAIT);
    const unflaggedAt = flagged().length;
    await until(() => flagged().length >= unflaggedAt + 2, "further waits during the turn", WAIT);
    assert.ok(flagged().slice(unflaggedAt).every((flag) => !flag), "the wait stays unflagged while the entry is held");
    assert.equal(existsSync(held), true, "the held entry is not drained mid-turn, so merge and cancel still act on it");
    assert.equal(injected().length, 1);
    await session.extensionRunner.emit({ type: "agent_settled" });
    await until(() => injected().length === 2, "the settle injection", WAIT);
    assert.equal(injected()[1], `${label}\nbusy hello`);
    const afterSettle = flagged().length;
    await until(() => flagged().length >= afterSettle + 1, "the re-armed wait", WAIT);
    assert.equal(flagged().at(-1), true, "the flag is restored after the drain");
    assert.deepEqual(lines(drains).filter((texts: string[]) => texts.length > 0), [["idle hello\n\n(01) ok"], ["busy hello"]], "each entry is drained exactly once");

    // Knob off: no inbox flag, nothing drained.
    writeFileSync(knobOff, "");
    const beforeOff = flagged().length;
    await until(() => flagged().slice(beforeOff).includes(false), "a wait armed after the knob turned off", WAIT);
    const offAt = flagged().length;
    writeFileSync(held, "while off");
    await until(() => flagged().length >= offAt + 2, "further waits with the knob off", WAIT);
    assert.ok(flagged().slice(offAt).every((flag) => !flag));
    assert.equal(existsSync(held), true);
    assert.equal(injected().length, 2);
  } finally {
    if (session) { await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" }); session.dispose(); }
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(directory, { recursive: true, force: true });
  }
});
