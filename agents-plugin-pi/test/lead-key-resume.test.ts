/**
 * Lead session key persistence across resume, and adoption of the key a lead
 * revives with (261007, bridge.ts "Lead key persistence").
 *
 * The production `startBridge` runs against a fake ws-mcp launcher (a Python
 * stdio JSON-RPC loop) that mints `minted-<n>` keys, accepts a configured set
 * of keys for ferrule's hidden `relogin_session_key`, refuses every other
 * re-login, and journals every tools/call so a test can assert which keys
 * were re-logged in, minted, or forwarded. The trigger for reuse is the
 * `ws-pi-lead-key` session entry, not `SessionStartEvent.reason`: a reopened
 * file (`pi -c`, `-r`, `--session`) starts with `reason: "startup"`,
 * `/reload` re-runs the bootstrap in the same session, and an in-process
 * switch fires `resume` - `startBridge` sees none of them, only the entries
 * and the session id, so one bootstrap covers all three.
 */
import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import {
  LEAD_KEY_ENTRY,
  buildDefaultKeyChangedLine,
  reloginLeadKey,
  restoreLeadKey,
  revivedKeyToAdopt,
  startBridge,
  type BridgeOptions,
} from "../src/bridge.ts";
import { ADAPTER_MESSAGE_LABEL } from "../src/adapter-label.ts";
import { buildLeadCompactionSummary, renderLeadProse } from "../src/lead-compaction.ts";
import { DELEGATION_ENV } from "../src/delegation-policy.ts";
import type { McpToolCallResult } from "../src/mcp-stdio-client.ts";

const TEST_DIR = dirname(fileURLToPath(import.meta.url));
const RUNTIME_JSON = join(TEST_DIR, "..", "runtime.json");
const BUNDLED_RUNTIME = JSON.parse(readFileSync(RUNTIME_JSON, "utf8")) as { plugin_version: string };
const SENTINEL = "obsidian-latch";

interface LauncherConfig {
  /** Keys ferrule's re-login accepts; every other re-login is refused. */
  accept?: string[];
  /** Keys whose workflow_manual call fails. */
  failManual?: string[];
  /** When true, playbook.read fails so the bridge takes the verbatim workflow_manual path. */
  noSnapshot?: boolean;
}

interface Call { name: string; args: Record<string, unknown> }

function writeLauncher(directory: string, config: LauncherConfig): { launcher: string; calls: () => Call[] } {
  const launcher = join(directory, "launcher.py");
  const callsFile = join(directory, "calls.jsonl");
  const countFile = join(directory, "mint-count.txt");
  writeFileSync(launcher, `import json,sys,os
accept=set(${JSON.stringify(config.accept ?? [])})
fail_manual=set(${JSON.stringify(config.failManual ?? [])})
no_snapshot=${config.noSnapshot ? "True" : "False"}
calls_file=${JSON.stringify(callsFile)}
count_file=${JSON.stringify(countFile)}
def ok(t): return {'isError':False,'content':[{'type':'text','text':t}]}
def err(t): return {'isError':True,'content':[{'type':'text','text':t}]}
for line in sys.stdin:
 q=json.loads(line); m=q['method']
 if m=='initialize': r={'serverInfo':{'version':${JSON.stringify(BUNDLED_RUNTIME.plugin_version)}},'capabilities':{}}
 elif m=='tools/list': r={'tools':[{'name':n,'description':n,'inputSchema':{'type':'object','properties':{'session_key':{'type':'string'}},'required':['session_key']}} for n in ['ferrule','workflow_manual','playbook.read','config.resolve_agent']]}
 elif m=='tools/call':
  n=q['params']['name']; a=q['params'].get('arguments') or {}
  with open(calls_file,'a') as f: f.write(json.dumps({'name':n,'args':a})+'\\n')
  if n=='ferrule':
   k=a.get('relogin_session_key')
   if k is not None: r=ok(json.dumps({'session_key':k})) if k in accept else err('session bootstrap: relogin_session_key %r is not a known session key'%k)
   else:
    c=int(open(count_file).read()) if os.path.exists(count_file) else 0
    c+=1; open(count_file,'w').write(str(c)); r=ok(json.dumps({'session_key':'minted-%d'%c}))
  elif n=='playbook.read': r=err('no snapshot') if no_snapshot else ok('# Workflow Manual\\nstatic body')
  elif n=='config.resolve_agent': r=ok(json.dumps({'resolved_from':'default'}))
  elif n=='workflow_manual':
   k=a.get('session_key')
   r=err('unknown_session') if k in fail_manual else ok('# Workflow Manual\\nstatic body\\n\\n## Session Key\\n%s\\n\\n## Session State\\n### Todos\\n(no todos)'%k)
  else: r=err('unknown tool')
 print(json.dumps({'jsonrpc':'2.0','id':q['id'],'result':r}),flush=True)
`);
  return {
    launcher,
    calls: () => (existsSync(callsFile) ? readFileSync(callsFile, "utf8").trim().split("\n").filter(Boolean).map((l) => JSON.parse(l) as Call) : []),
  };
}

const leadKeyEntry = (sessionId: string, key: string) => ({ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId, key } });

interface Harness {
  handle: Awaited<ReturnType<typeof startBridge>>;
  tools: Map<string, any>;
  appended: Array<{ type: string; customType: string; data: unknown }>;
  notices: Array<{ message: string; level: string }>;
  calls: () => Call[];
  relogins: () => string[];
  mints: () => number;
  restore: () => void;
}

async function boot(config: LauncherConfig, options: Partial<BridgeOptions> & { entries?: unknown[]; sessionId?: string; policy?: Record<string, unknown> }, reuse?: { directory: string; calls: () => Call[]; launcher: string }): Promise<Harness> {
  const directory = reuse?.directory ?? mkdtempSync(join(tmpdir(), "ws-pi-lead-key-"));
  const { launcher, calls } = reuse ?? writeLauncher(directory, config);
  const tools = new Map<string, any>();
  const appended: Harness["appended"] = [];
  const notices: Harness["notices"] = [];
  const pi = {
    registerTool: (definition: any) => tools.set(definition.name, definition),
    on() {},
    appendEntry: (customType: string, data: unknown) => appended.push({ type: "custom", customType, data }),
  } as unknown as ExtensionAPI;
  const oldRole = process.env.WS_PI_SPAWN_ROLE;
  const oldPolicy = process.env[DELEGATION_ENV];
  delete process.env.WS_PI_SPAWN_ROLE;
  if (options.policy) process.env[DELEGATION_ENV] = JSON.stringify(options.policy); else delete process.env[DELEGATION_ENV];
  const restore = () => {
    if (oldRole === undefined) delete process.env.WS_PI_SPAWN_ROLE; else process.env.WS_PI_SPAWN_ROLE = oldRole;
    if (oldPolicy === undefined) delete process.env[DELEGATION_ENV]; else process.env[DELEGATION_ENV] = oldPolicy;
  };
  const before = calls().length;
  try {
    const handle = await startBridge(pi, {
      launcherPath: launcher,
      pluginDir: directory,
      runtimeJsonPath: RUNTIME_JSON,
      cwd: directory,
      toolPreviewTuiRef: { current: undefined },
      ui: { notify: (message: string, level = "info") => notices.push({ message, level }) } as never,
      sessionEntries: options.entries,
      sessionId: options.sessionId,
    });
    const sinceBoot = () => calls().slice(before);
    return {
      handle, tools, appended, notices, calls: sinceBoot, restore,
      relogins: () => sinceBoot().filter((c) => c.name === "ferrule" && "relogin_session_key" in c.args).map((c) => c.args.relogin_session_key as string),
      mints: () => sinceBoot().filter((c) => c.name === "ferrule" && !("relogin_session_key" in c.args)).length,
    };
  } catch (error) {
    restore();
    throw error;
  }
}

const ctx = { sessionManager: { buildContextEntries: () => [] } };
const texts = (content: any[]): string[] => content.filter((item) => item.type === "text").map((item) => item.text as string);

describe("restoreLeadKey", () => {
  test("returns the newest entry recorded under the session id and ignores other sessions and malformed entries", () => {
    const entries = [
      leadKeyEntry("s1", "older"),
      { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "s1" } },
      leadKeyEntry("s2", "foreign"),
      { type: "custom", customType: "ws-pi-fork-keys", data: { sessionId: "s1", current: "fork" } },
      leadKeyEntry("s1", "newest"),
      null,
    ];
    assert.equal(restoreLeadKey(entries, "s1"), "newest");
    assert.equal(restoreLeadKey(entries, "s2"), "foreign");
    assert.equal(restoreLeadKey(entries, "s3"), undefined);
    assert.equal(restoreLeadKey(entries, undefined), undefined);
    assert.equal(restoreLeadKey([], "s1"), undefined);
  });

  test("a newest entry with a blank key yields nothing rather than an older key", () => {
    assert.equal(restoreLeadKey([leadKeyEntry("s1", "older"), leadKeyEntry("s1", "  ")], "s1"), undefined);
  });
});

describe("revivedKeyToAdopt", () => {
  test("an explicit key other than the default is adopted; the sentinel, the default, blanks, and non-strings are not", () => {
    assert.equal(revivedKeyToAdopt("revived", "own", SENTINEL), "revived");
    assert.equal(revivedKeyToAdopt(" revived ", "own", SENTINEL), "revived");
    assert.equal(revivedKeyToAdopt("revived", undefined, SENTINEL), "revived", "a degraded bootstrap adopts the revived key");
    assert.equal(revivedKeyToAdopt("own", "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(SENTINEL, "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(SENTINEL, undefined, SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt("", "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(undefined, "own", SENTINEL), undefined);
    assert.equal(revivedKeyToAdopt(42, "own", SENTINEL), undefined);
  });
});

describe("buildDefaultKeyChangedLine", () => {
  test("opens with the shared adapter label line and names both keys", () => {
    const line = buildDefaultKeyChangedLine("old-key", "new-key");
    assert.equal(line, `${ADAPTER_MESSAGE_LABEL}\nDefault session key changed: old-key -> new-key. Calls that omit session_key and compaction summaries now use new-key.`);
    assert.match(buildDefaultKeyChangedLine(undefined, "new-key"), /changed: \(unset\) -> new-key\./);
  });
});

describe("reloginLeadKey", () => {
  const ok = (text: string): McpToolCallResult => ({ isError: false, content: [{ type: "text", text }] });
  test("ok only when ws-mcp returns the same key; a refusal, a thrown call, or another key is not ok", async () => {
    const seen: Array<[string, Record<string, unknown>]> = [];
    const same = await reloginLeadKey(async (name, args) => { seen.push([name, args]); return ok(JSON.stringify({ session_key: "k" })); }, "/root", "k");
    assert.equal(same.ok, true);
    assert.deepEqual(seen, [["ferrule", { root: "/root", format: "json", relogin_session_key: "k" }]]);
    const refused = await reloginLeadKey(async () => ({ isError: true, content: [{ type: "text", text: "nope" }] }), "/root", "k");
    assert.deepEqual(refused, { ok: false, reason: "nope" });
    const threw = await reloginLeadKey(async () => { throw new Error("transport down"); }, "/root", "k");
    assert.deepEqual(threw, { ok: false, reason: "transport down" });
    const other = await reloginLeadKey(async () => ok(JSON.stringify({ session_key: "z" })), "/root", "k");
    assert.equal(other.ok, false);
    assert.match((other as { reason: string }).reason, /returned z instead of k/);
  });
});

describe("lead key persistence at bootstrap", () => {
  test("reuses the key recorded under the same session id with no mint, and records nothing new (startup, reload, and resume alike)", async () => {
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "original-key");
      assert.deepEqual(h.relogins(), ["original-key"]);
      assert.equal(h.mints(), 0);
      assert.deepEqual(h.appended, [], "the newest entry already holds this key");
      const manual = h.calls().find((c) => c.name === "workflow_manual");
      assert.equal(manual?.args.session_key, "original-key", "the session-start snapshot fetch runs under the reused key");
      assert.deepEqual(h.notices.filter((n) => n.level === "warning"), []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("the newest entry for the session wins over an older one", async () => {
    const h = await boot({ accept: ["older-key", "newer-key"] }, { entries: [leadKeyEntry("sess-1", "older-key"), leadKeyEntry("sess-1", "newer-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "newer-key");
      assert.deepEqual(h.relogins(), ["newer-key"]);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("mints and records when no entry matches: a new session, and a fork whose file carries the lead's entry under the lead's session id", async () => {
    const fresh = await boot({ accept: ["lead-key"] }, { entries: [], sessionId: "new-sess" });
    try {
      assert.equal(fresh.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(fresh.relogins(), []);
      assert.equal(fresh.mints(), 1);
      assert.deepEqual(fresh.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "new-sess", key: "minted-1" } }]);
    } finally { fresh.handle.shutdown(); fresh.restore(); }

    const fork = await boot({ accept: ["lead-key"] }, { entries: [leadKeyEntry("lead-sess", "lead-key")], sessionId: "fork-sess" });
    try {
      assert.equal(fork.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(fork.relogins(), [], "the lead's key is accepted by ws-mcp but never tried: the session id does not match");
      assert.deepEqual(fork.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "fork-sess", key: "minted-1" } }]);
    } finally { fork.handle.shutdown(); fork.restore(); }
  });

  test("a refused recorded key falls back to a fresh mint with a UI warning only, and the fresh key is recorded", async () => {
    const h = await boot({ accept: [] }, { entries: [leadKeyEntry("sess-1", "stale-key")], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), ["stale-key"]);
      assert.equal(h.mints(), 1);
      const warnings = h.notices.filter((n) => n.level === "warning");
      assert.equal(warnings.length, 1);
      assert.match(warnings[0]!.message, /recorded ws key stale-key was refused \(.*not a known session key.*\); minting a fresh key/);
      assert.deepEqual(h.appended, [{ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "minted-1" } }]);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("without a session id nothing is reused or recorded", async () => {
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: undefined });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), []);
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a child process keeps its policy key and never consults the entries", async () => {
    const policy = { version: 1, depth: 1, maxDepth: 2, tools: ["ws__workflow_manual"], authority: "lead", sessionKey: "child-key", parentSessionKey: "lead-key" };
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1", policy });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "child-key");
      assert.deepEqual(h.calls().filter((c) => c.name === "ferrule"), [], "a policy key needs no ferrule at all");
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a child process without a policy key mints under its policy and never re-logs in", async () => {
    const policy = { version: 1, depth: 1, maxDepth: 2, tools: ["ws__workflow_manual"], authority: "delegate", parentSessionKey: "lead-key" };
    const h = await boot({ accept: ["original-key"] }, { entries: [leadKeyEntry("sess-1", "original-key")], sessionId: "sess-1", policy });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(h.relogins(), []);
      const mint = h.calls().find((c) => c.name === "ferrule");
      assert.equal(mint?.args.capability, "delegate");
      assert.equal(mint?.args.parent_session_key, "lead-key");
      assert.deepEqual(h.appended, []);
    } finally { h.handle.shutdown(); h.restore(); }
  });
});

describe("adopting the key a lead revives with", () => {
  test("a lead workflow_manual with another accepted key re-logs in, adopts it, records it, appends the pinned line once, and the next compaction summary names it", async () => {
    const h = await boot({ accept: ["revived-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      const manual = h.tools.get("ws__workflow_manual");

      const revived = await manual.execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      const lines = texts(revived.content);
      assert.equal(lines.at(-1), buildDefaultKeyChangedLine("minted-1", "revived-key"), "the change line is the last text item");
      assert.match(lines[0]!, /## Session Key\nrevived-key/, "the manual result itself is the revived key's");
      assert.equal(h.handle.defaultSessionKeyRef.current, "revived-key");
      assert.deepEqual(h.relogins(), ["revived-key"]);
      assert.deepEqual(h.appended, [
        { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "minted-1" } },
        { type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId: "sess-1", key: "revived-key" } },
      ]);

      // Reviving with the key that is already the default: no re-login, no line.
      const again = await manual.execute("two", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.ok(!texts(again.content).some((t) => t.startsWith(ADAPTER_MESSAGE_LABEL)), "no change line when the key is unchanged");
      assert.deepEqual(h.relogins(), ["revived-key"]);
      assert.equal(h.appended.length, 2);

      // An omitted key now fills the adopted key.
      await manual.execute("three", {}, undefined, undefined, ctx);
      assert.equal(h.calls().filter((c) => c.name === "workflow_manual").at(-1)?.args.session_key, "revived-key");

      const summary = buildLeadCompactionSummary({ sessionKey: h.handle.defaultSessionKeyRef.current, branchEntries: [], registry: undefined, prose: renderLeadProse({ current_work: "x" }), dialogBudgetBytes: 4096, sessionFile: "/sessions/lead.jsonl" });
      assert.match(summary, /ws session key: `revived-key` \(preserve verbatim\)/);
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("the verbatim workflow_manual path (no static snapshot) adopts the same way", async () => {
    const h = await boot({ accept: ["revived-key"], noSnapshot: true }, { entries: [], sessionId: "sess-1" });
    try {
      assert.equal(h.handle.staticBodySnapshotRef.current, undefined);
      const manual = h.tools.get("ws__workflow_manual");
      const revived = await manual.execute("one", { session_key: "revived-key" }, undefined, undefined, ctx);
      assert.equal(texts(revived.content).at(-1), buildDefaultKeyChangedLine("minted-1", "revived-key"));
      assert.equal(h.handle.defaultSessionKeyRef.current, "revived-key");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("no adoption on a refused re-login, a failed workflow_manual, the bootstrap sentinel, or an explicit key to another tool", async () => {
    const h = await boot({ accept: [], failManual: ["dead-key"] }, { entries: [], sessionId: "sess-1" });
    try {
      const manual = h.tools.get("ws__workflow_manual");
      const noLine = (content: any[]) => assert.ok(!texts(content).some((t) => t.startsWith(ADAPTER_MESSAGE_LABEL)));

      // Refused re-login (ws-mcp knows no such lead key): the manual result
      // stands, nothing is adopted.
      const refused = await manual.execute("one", { session_key: "foreign-key" }, undefined, undefined, ctx);
      noLine(refused.content);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");

      // Failed workflow_manual: the call throws before any re-login.
      await assert.rejects(manual.execute("two", { session_key: "dead-key" }, undefined, undefined, ctx), /unknown_session/);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");

      // The bootstrap sentinel is a fresh-bootstrap request, not a revive.
      const sentinel = await manual.execute("three", { session_key: SENTINEL }, undefined, undefined, ctx);
      noLine(sentinel.content);
      assert.deepEqual(h.relogins(), ["foreign-key"]);

      // An explicit key to any other tool (a child's key, say) is forwarded, never adopted.
      await h.tools.get("ws__playbook_read").execute("four", { name: "lead-workflow-manual", session_key: "child-key" }, undefined, undefined, ctx);
      assert.deepEqual(h.relogins(), ["foreign-key"]);
      assert.equal(h.handle.defaultSessionKeyRef.current, "minted-1");
      assert.equal(h.appended.length, 1, "only the bootstrap mint was recorded");
    } finally { h.handle.shutdown(); h.restore(); }
  });

  test("a legacy session with no key entry mints on resume, then converges on the first revive with its original key", async () => {
    // Legacy file: compaction summaries and fork entries, but no lead key entry.
    const legacyEntries = [{ type: "custom", customType: "ws-pi-fork-keys", data: { sessionId: "other", current: "fork-key", previous: [] } }];
    const first = await boot({ accept: ["original-key"] }, { entries: legacyEntries, sessionId: "legacy-sess" });
    let appended: Harness["appended"];
    try {
      assert.equal(first.handle.defaultSessionKeyRef.current, "minted-1");
      assert.deepEqual(first.relogins(), []);
      const revived = await first.tools.get("ws__workflow_manual").execute("one", { session_key: "original-key" }, undefined, undefined, ctx);
      assert.equal(texts(revived.content).at(-1), buildDefaultKeyChangedLine("minted-1", "original-key"));
      assert.equal(first.handle.defaultSessionKeyRef.current, "original-key");
      appended = first.appended;
    } finally { first.handle.shutdown(); first.restore(); }

    // The next restart of the same session file reuses the adopted key.
    const second = await boot({ accept: ["original-key"] }, { entries: [...legacyEntries, ...appended], sessionId: "legacy-sess" });
    try {
      assert.equal(second.handle.defaultSessionKeyRef.current, "original-key");
      assert.deepEqual(second.relogins(), ["original-key"]);
      assert.equal(second.mints(), 0);
      assert.deepEqual(second.appended, []);
    } finally { second.handle.shutdown(); second.restore(); }
  });
});
