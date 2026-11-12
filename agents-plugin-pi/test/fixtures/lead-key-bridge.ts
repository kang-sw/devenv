/**
 * The lead-key bootstrap harness: the production `startBridge` against a fake
 * ws-mcp launcher (a Python stdio JSON-RPC loop) that mints `minted-<n>` keys,
 * accepts a configured set of keys for ferrule's hidden `relogin_session_key`,
 * refuses a configured set as held by a hosted session, refuses every other
 * re-login, and journals every tools/call so a test can assert which keys were
 * re-logged in, minted, or forwarded, and with which arguments.
 *
 * The session's working directory is a subdirectory of the fixture directory
 * (HARNESS_CWD_SUFFIX), so it differs from the plugin directory and from any
 * session file a test passes.
 */
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { HOSTED_SESSION_HELD_MARKER, LEAD_KEY_ENTRY, startBridge, type BridgeOptions } from "../../src/bridge.ts";
import { DELEGATION_ENV } from "../../src/delegation-policy.ts";

const FIXTURE_DIR = dirname(fileURLToPath(import.meta.url));
const RUNTIME_JSON = join(FIXTURE_DIR, "..", "..", "runtime.json");
const BUNDLED_RUNTIME = JSON.parse(readFileSync(RUNTIME_JSON, "utf8")) as { plugin_version: string };

/** The session's working directory, relative to the fixture directory. */
export const HARNESS_CWD_SUFFIX = join("work", "app");

/** The refusal text the fake launcher answers a held re-login of `key` with. */
export const heldRefusal = (key: string): string =>
  `session bootstrap: relogin_session_key "${key}" is refused (${HOSTED_SESSION_HELD_MARKER}): the dashboard hosts this session. No session key was issued.`;

export interface LauncherConfig {
  /** Keys ferrule's re-login accepts; every other re-login is refused. */
  accept?: string[];
  /** Keys whose re-login is refused as held by a hosted session (heldRefusal). */
  held?: string[];
  /** Keys whose workflow_manual call fails. */
  failManual?: string[];
  /** When true, playbook.read fails so the bridge takes the verbatim workflow_manual path. */
  noSnapshot?: boolean;
}

export interface Call { name: string; args: Record<string, unknown> }

function writeLauncher(directory: string, config: LauncherConfig): { launcher: string; calls: () => Call[] } {
  const launcher = join(directory, "launcher.py");
  const callsFile = join(directory, "calls.jsonl");
  const countFile = join(directory, "mint-count.txt");
  const held = Object.fromEntries((config.held ?? []).map((key) => [key, heldRefusal(key)]));
  writeFileSync(launcher, `import json,sys,os
accept=set(${JSON.stringify(config.accept ?? [])})
held=json.loads(${JSON.stringify(JSON.stringify(held))})
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
   if k is not None: r=ok(json.dumps({'session_key':k})) if k in accept else err(held[k]) if k in held else err('session bootstrap: relogin_session_key %r is not a known session key'%k)
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

export const leadKeyEntry = (sessionId: string, key: string) => ({ type: "custom", customType: LEAD_KEY_ENTRY, data: { sessionId, key } });

export interface Harness {
  handle: Awaited<ReturnType<typeof startBridge>>;
  tools: Map<string, any>;
  appended: Array<{ type: string; customType: string; data: unknown }>;
  notices: Array<{ message: string; level: string }>;
  /** The session's working directory: the ferrule root and the reported harness directory. */
  cwd: string;
  calls: () => Call[];
  relogins: () => string[];
  mints: () => number;
  restore: () => void;
}

export async function boot(config: LauncherConfig, options: { entries?: unknown[]; sessionId?: string; sessionFile?: string; policy?: Record<string, unknown> }): Promise<Harness> {
  const directory = mkdtempSync(join(tmpdir(), "ws-pi-lead-key-"));
  const cwd = join(directory, HARNESS_CWD_SUFFIX);
  mkdirSync(cwd, { recursive: true });
  const { launcher, calls } = writeLauncher(directory, config);
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
    const bridgeOptions: BridgeOptions = {
      launcherPath: launcher,
      pluginDir: directory,
      runtimeJsonPath: RUNTIME_JSON,
      cwd,
      toolPreviewTuiRef: { current: undefined },
      ui: { notify: (message: string, level = "info") => notices.push({ message, level }) } as never,
      sessionEntries: options.entries,
      sessionId: options.sessionId,
      sessionFile: options.sessionFile,
    };
    const handle = await startBridge(pi, bridgeOptions);
    const sinceBoot = () => calls().slice(before);
    return {
      handle, tools, appended, notices, cwd, calls: sinceBoot, restore,
      relogins: () => sinceBoot().filter((c) => c.name === "ferrule" && "relogin_session_key" in c.args).map((c) => c.args.relogin_session_key as string),
      mints: () => sinceBoot().filter((c) => c.name === "ferrule" && !("relogin_session_key" in c.args)).length,
    };
  } catch (error) {
    restore();
    throw error;
  }
}
