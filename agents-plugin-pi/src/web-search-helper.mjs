// One-shot execution boundary. Never import the upstream extension in the agent.
import { readFileSync, existsSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { createRequire } from 'node:module';
import { pathToFileURL } from 'node:url';

export const MAX_FRAME = 64 * 1024;
export const PROXY_KEYS = ['HTTP_PROXY', 'http_proxy', 'HTTPS_PROXY', 'https_proxy', 'ALL_PROXY', 'all_proxy'];
export function configPath(env = process.env) {
  if (env.PI_CODING_AGENT_DIR) return join(env.PI_CODING_AGENT_DIR, 'web-search.json');
  if (env.XDG_CONFIG_HOME) {
    const xdg = join(env.XDG_CONFIG_HOME, 'pi', 'web-search.json');
    const legacy = join(env.HOME || homedir(), '.pi', 'web-search.json');
    return existsSync(xdg) ? xdg : existsSync(legacy) ? legacy : xdg;
  }
  return join(env.HOME || homedir(), '.pi', 'agent', 'web-search.json');
}
export function hasProxy(env = process.env) {
  if (PROXY_KEYS.some(key => env[key]?.trim())) return true;
  const path = configPath(env);
  if (!existsSync(path)) return false;
  const config = JSON.parse(readFileSync(path, 'utf8'));
  if (config.proxy != null && typeof config.proxy !== 'string') throw Error('invalid config');
  return Boolean(config.proxy?.trim());
}
export function validateQuery(args) {
  if (!args || typeof args !== 'object' || Array.isArray(args) || Reflect.ownKeys(args).length !== 1 ||
      !Object.hasOwn(args, 'query') || typeof args.query !== 'string' || !args.query.trim() ||
      Buffer.byteLength(args.query) > 4096) throw Error('invalid query');
  // Upstream expands a JSON array string into concurrent independent searches.
  let parsed;
  try { parsed = JSON.parse(args.query.trim()); } catch { /* Ordinary bracketed prose remains a literal query. */ }
  if (Array.isArray(parsed) && parsed.every(entry => typeof entry === 'string')) throw Error('query arrays unsupported');
  return args.query.trim();
}
export function frame(value) {
  const body = Buffer.from(JSON.stringify(value));
  if (body.length > MAX_FRAME) throw Error('frame limit');
  const header = Buffer.alloc(4); header.writeUInt32BE(body.length);
  return Buffer.concat([header, body]);
}
export function parseFrame(buffer) {
  if (buffer.length < 4 || buffer.length > MAX_FRAME + 4 || buffer.readUInt32BE(0) !== buffer.length - 4) throw Error('invalid frame');
  return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(buffer.subarray(4)));
}
function strict(object, reject = () => { throw Error('unexpected API'); }) {
  return new Proxy(object, { get(target, key) {
    if (!Object.hasOwn(target, key)) return reject();
    return target[key];
  } });
}
const text = (value, cap) => typeof value === 'string' ? value.slice(0, cap).replace(/[\u0000-\u0008\u000b-\u001f\u007f]/g, '') : '';
export function normalizeSearch(data, query) {
  if (data?.type !== 'search' || !Array.isArray(data.queries) || data.queries.length !== 1) throw Error('invalid metadata');
  const result = data.queries[0];
  if (result.query !== query || result.error || !Array.isArray(result.results)) throw Error('provider failure');
  return { query, answer: text(result.answer, 8000), results: result.results.slice(0, 10).map(item => {
    const url = new URL(item.url);
    if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password) throw Error('invalid result URL');
    return { title: text(item.title, 500), url: text(url.href, 2000), snippet: text(item.snippet, 1500) };
  }) };
}
export async function createReadOnlyRegistry() {
  const { ModelRuntime, ModelRegistry, getAgentDir } = await import('@earendil-works/pi-coding-agent');
  const authPath = join(getAgentDir(), 'auth.json');
  const auth = existsSync(authPath) ? JSON.parse(readFileSync(authPath, 'utf8')) : {};
  if (!auth || typeof auth !== 'object' || Array.isArray(auth)) throw Error('invalid auth');
  // Never instantiate the SDK's file-backed credential/catalog stores: even
  // read operations can acquire filesystem locks. Refresh/login mutation fails.
  const credentials = {
    async read(providerId) { return structuredClone(auth[providerId]); },
    async list() { return Object.entries(auth).map(([providerId, entry]) => ({ providerId, type: entry.type })); },
    async modify() { throw Error('credential mutation unsupported'); },
    async delete() { throw Error('credential mutation unsupported'); },
  };
  const catalog = new Map();
  const runtime = await ModelRuntime.create({ credentials,
    modelsPath: join(getAgentDir(), 'models.json'), allowModelNetwork: false,
    modelsStore: {
      async read(key) { return structuredClone(catalog.get(key)); },
      async write(key, value) { catalog.set(key, structuredClone(value)); },
      async delete(key) { catalog.delete(key); },
    },
  });
  const registry = new ModelRegistry(runtime);
  return strict({ then: undefined, getAll: () => registry.getAll(), find: (...args) => registry.find(...args),
    getApiKeyAndHeaders: model => registry.getApiKeyAndHeaders(model) });
}
export function validateToolNames() {
  const defaultShortcuts = ['ctrl+shift+s', 'ctrl+shift+w'];
  const path = configPath();
  if (!existsSync(path)) return defaultShortcuts;
  const config = JSON.parse(readFileSync(path, 'utf8'));
  const expected = { webSearch: 'web_search', sourceCheck: 'source_check', fetchContent: 'fetch_content', getSearchContent: 'get_search_content' };
  if (config.toolNames !== undefined && (!config.toolNames || typeof config.toolNames !== 'object' || Array.isArray(config.toolNames))) throw Error('tool identity override');
  for (const [key, value] of Object.entries(config.toolNames ?? {})) {
    if (!Object.hasOwn(expected, key) || value !== expected[key]) throw Error('tool identity override');
  }
  const shortcuts = [config.shortcuts?.curate || defaultShortcuts[0], config.shortcuts?.activity || defaultShortcuts[1]];
  if (shortcuts.some(key => typeof key !== 'string')) throw Error('invalid shortcuts');
  return shortcuts;
}
export async function captureSearch(extension, modelRegistry, shortcutNames = ['ctrl+shift+s', 'ctrl+shift+w']) {
  let captured;
  let initializing = true;
  let entry;
  let violated = false;
  const reject = () => { violated = true; throw Error('unexpected API'); };
  let shortcuts = 0;
  const lifecycleRemaining = { before_agent_start: 1, agent_settled: 1, session_start: 2, session_tree: 2, session_shutdown: 2 };
  const commands = new Set();
  const knownTools = new Set(['web_search', 'source_check', 'fetch_content', 'get_search_content']);
  const registered = new Set();
  const api = strict({
    on(name) {
      if (!initializing || !Object.hasOwn(lifecycleRemaining, name) || lifecycleRemaining[name]-- <= 0) reject();
    },
    registerShortcut(name) { if (!initializing || shortcuts >= shortcutNames.length || name !== shortcutNames[shortcuts++]) reject(); },
    registerCommand(name) {
      if (!initializing || !['websearch', 'curator', 'search', 'google-account'].includes(name) || commands.has(name)) reject();
      commands.add(name);
    },
    registerTool(tool) {
      if (!initializing || !knownTools.has(tool.name) || registered.has(tool.name)) reject();
      registered.add(tool.name);
      if (tool.name !== 'web_search') return;
      if (captured || typeof tool.execute !== 'function') reject();
      captured = tool;
    },
    appendEntry(type, data) {
      if (initializing || entry || type !== 'web-search-results' || data?.type !== 'search') reject();
      entry = data;
    },
  }, reject);
  await extension(api);
  initializing = false;
  if (!captured || violated) throw Error('capture failed');
  return async query => {
    const ctx = strict({ hasUI: false, model: undefined, modelRegistry }, reject);
    await captured.execute('ws-search', { query, includeContent: false, workflow: 'none', proxy: '' },
      new AbortController().signal, undefined, ctx);
    if (violated) throw Error('unexpected API');
    return normalizeSearch(entry, query);
  };
}
async function main() {
  // Node permissions constrain audited optional credential commands, browser-cookie
  // copies and auth-refresh writes too, not merely the proxy/curl route.
  if (!process.permission || !process.permission.has('fs.read') ||
      ['fs.write', 'child', 'worker', 'addons'].some(scope => process.permission.has(scope))) throw Error('permissions required');
  let input = Buffer.alloc(0);
  for await (const chunk of process.stdin) {
    input = Buffer.concat([input, chunk]);
    if (input.length > 8192) throw Error('input limit');
  }
  const request = parseFrame(input);
  if (request.mode !== 'probe' && request.mode !== 'search') throw Error('invalid mode');
  const query = request.mode === 'search' ? validateQuery(request.args) : undefined;
  if (request.mode === 'search' && hasProxy()) return { error: 'web-search-proxy-unsupported' };
  const shortcutNames = validateToolNames();
  const root = process.argv[2];
  const manifest = JSON.parse(readFileSync(join(root, 'package.json'), 'utf8'));
  if (manifest.name !== 'pi-web-access' || manifest.version !== '0.29.0') throw Error('wrong package');
  const require = createRequire(import.meta.url);
  const { createJiti } = require('jiti');
  const jiti = createJiti(import.meta.url, { fsCache: false, moduleCache: false });
  const extension = await jiti.import(join(root, 'index.ts'));
  const registry = request.mode === 'search' ? await createReadOnlyRegistry() : undefined;
  const execute = await captureSearch(extension.default, registry, shortcutNames);
  return request.mode === 'probe' ? { ready: true } : { result: await execute(query) };
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  // A dedicated pipe remains open for parent lifetime, including after stdin EOF.
  // Parent crash/SIGKILL closes it; no orphan helper survives to its own deadline.
  const { Socket } = await import('node:net');
  // Child stdio pipes are socket pairs. fs.createReadStream would leave a
  // blocking libuv thread-pool read that can prevent process.exit from finishing.
  const lifetime = new Socket({ fd: 3, readable: true, writable: false });
  lifetime.on('end', () => process.exit(1));
  lifetime.on('error', () => process.exit(1));
  lifetime.resume();
  setTimeout(() => process.exit(1), 30_000).unref();
  try {
    const result = await main();
    process.stdout.write(frame(result), () => process.exit(0));
  } catch {
    process.stdout.write(frame({ error: 'web-search-tool-unavailable' }), () => process.exit(0));
  }
}
