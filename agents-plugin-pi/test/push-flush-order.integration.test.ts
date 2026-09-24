/**
 * The handler registration order in the real extension entry (`src/index.ts`):
 * `registerPushFlush`'s lifecycle handlers update the own-turn accounting
 * before the publish handlers registered after it send the subtree snapshot.
 * Pi runs one extension's handlers in registration order, so only a load of
 * the real factory through Pi's own resource loader and extension runner
 * observes that order. The `childProcess()` harness in
 * `recursive-worker.test.ts` registers copies of those handlers and cannot.
 *
 * Harness pattern: `fork-lifecycle.integration.test.ts` (a copied adapter
 * with an offline MCP launcher, real SDK session and extension runner). The
 * session is a channel child: the factory reads a bootstrap for a
 * `ParentChannel` this test binds, and the parent's view is observed with the
 * production `observeSubtreeChannel`.
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { cpSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import * as sdk from "@earendil-works/pi-coding-agent";
import { ParentChannel } from "../src/agent-channel.ts";
import { observeSubtreeChannel, type SubtreeSnapshot } from "../src/subtree-lifecycle.ts";
import { until } from "./fixtures/subtree-channels.ts";

const ROLE_ENV_KEYS = ["WS_PI_SPAWN_ROLE", "WS_PI_EXPLORE_MODE", "WS_PI_DELEGATION_POLICY", "WS_PI_PARENT_SESSION_KEY"] as const;

const WAIT = { timeoutMs: 20_000, intervalMs: 10 };

test("the real extension entry registers the push flush before the subtree publish handlers: a turn start's own snapshot carries the new count", { timeout: 120_000 }, async () => {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-flush-order-"));
  const savedEnv = { ...process.env };
  let parent: ParentChannel | undefined;
  let session: any;
  try {
    const plugin = join(directory, "plugin");
    mkdirSync(plugin);
    for (const name of ["src", "runtime.json", "goal-loop-config.json", "pi-lead-guide.md", "execute-worker-guide.md", "explore-guide.md"]) cpSync(join(process.cwd(), name), join(plugin, name), { recursive: true });
    // A junction needs no symlink privilege on Windows; the type is ignored elsewhere.
    symlinkSync(join(process.cwd(), "node_modules"), join(plugin, "node_modules"), "junction");
    mkdirSync(join(plugin, "bin"));
    const version = JSON.parse(readFileSync(join(plugin, "runtime.json"), "utf8")).plugin_version;
    // Offline stand-in for the ws MCP server: enough for the bridge to start and register agent tools.
    writeFileSync(join(plugin, "bin/ws-mcp-launcher.py"), `import sys,json,uuid\nkey='own-'+str(uuid.uuid4())\nfor line in sys.stdin:\n q=json.loads(line); m=q['method']; p=q.get('params',{}); r={}\n if m=='initialize': r={'serverInfo':{'name':'offline','version':${JSON.stringify(version)}},'capabilities':{}}\n elif m=='tools/list': r={'tools':[]}\n elif m=='tools/call':\n  n=p['name']; text=json.dumps({'session_key':key}) if n=='ferrule' else ('lead manual '+key if n=='workflow_manual' else '{}')\n  r={'content':[{'type':'text','text':text}],'isError':False}\n print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)\n`);

    parent = await ParentChannel.bind(1, { socketDir: directory });
    const snapshots: SubtreeSnapshot[] = [];
    observeSubtreeChannel(parent, view => { if (view.snapshot && view.snapshot !== snapshots.at(-1)) snapshots.push(view.snapshot); });

    // A lead-role session that is also a channel child: the factory reads (and deletes) the bootstrap.
    for (const key of ROLE_ENV_KEYS) delete process.env[key];
    Object.assign(process.env, { PI_OFFLINE: "1" }, parent.bootstrapEnv());
    const agentDir = join(directory, "agent");
    mkdirSync(agentDir);
    const settingsManager = sdk.SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } });
    const modelRuntime = await sdk.ModelRuntime.create({ authPath: join(agentDir, "auth.json"), modelsPath: join(agentDir, "models.json"), modelsStorePath: join(agentDir, "store.json"), allowModelNetwork: false });
    await modelRuntime.setRuntimeApiKey("anthropic", "offline-test-key");
    const model = { provider: "anthropic", api: "anthropic-messages", id: "offline-model", name: "offline", reasoning: false, input: ["text"], contextWindow: 128000, maxTokens: 8192, baseUrl: "https://offline.invalid/v1", cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } };
    const resourceLoader = new sdk.DefaultResourceLoader({ cwd: directory, agentDir, settingsManager, noSkills: true, noThemes: true, noPromptTemplates: true, noContextFiles: true, additionalExtensionPaths: [join(plugin, "src/index.ts")] });
    await resourceLoader.reload();
    assert.deepEqual(resourceLoader.getExtensions().errors, []);
    ({ session } = await sdk.createAgentSession({ cwd: directory, agentDir, sessionManager: sdk.SessionManager.create(directory, join(directory, "sessions")), resourceLoader, modelRuntime, model: model as never, thinkingLevel: "off", settingsManager }));
    const errors: string[] = [];
    const noop = () => {};
    const ui = new Proxy({ notify: (message: string, level?: string) => { if (level === "error") errors.push(message); }, custom: async () => undefined, setFooter: noop }, { get: (target: any, key) => target[key] ?? noop });
    await session.bindExtensions({ mode: "rpc", uiContext: ui, onError: (e: any) => errors.push(String(e.error ?? e.message ?? e)) });
    assert.deepEqual(errors, []);

    await until(() => snapshots.length > 0, "the session_start snapshot from the real publisher", WAIT);
    assert.equal(snapshots.at(-1)!.turnsStarted, 0);
    const before = snapshots.length;
    await session.extensionRunner.emit({ type: "agent_start" });
    // Correct order: the flush handler counts the start, then the publish
    // handler sends it. Swapped: the publish handler runs first and sees no
    // change (nothing is sent), and the count is left unsent until some later
    // publish, so this wait times out.
    await until(() => snapshots.length > before, "the turn start's snapshot", WAIT);
    assert.deepEqual(snapshots.slice(before).map(s => ({ turnsStarted: s.turnsStarted, turnOwed: s.turnOwed })), [{ turnsStarted: 1, turnOwed: false }],
      "the turn start's one snapshot already carries the new count");
  } finally {
    if (session) { await session.extensionRunner.emit({ type: "session_shutdown", reason: "quit" }); session.dispose(); }
    parent?.close();
    for (const key of Object.keys(process.env)) if (!(key in savedEnv)) delete process.env[key];
    Object.assign(process.env, savedEnv);
    rmSync(directory, { recursive: true, force: true });
  }
});
