'use strict';
// Missing-only starter: the dashboard entry takes StartDaemon's lock and never
// calls the replacement-capable daemon-start/Ensure path.
const path = require('node:path');
const { spawn } = require('node:child_process');
const { cacheRoot, discover } = require('./endpoint');
const { selectDaemon } = require('./daemon-selection');
const { stageDaemon } = require('./daemon-stage');

function launch(binary, options) {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ['dashboard'], { ...options, detached: true, stdio: 'ignore', windowsHide: true });
    child.once('error', reject);
    child.once('spawn', () => { child.removeListener('error', reject); child.unref(); resolve(); });
  });
}
function launchEnv(inherited, staged, root) {
  const env = { ...inherited };
  // The initiating harness is not the daemon's identity. Leave the user's
  // cache and config selection intact; replace only package-bound paths.
  for (const key of ['WS_HOSTED_SESSION', 'WS_HOSTED_SESSION_KEY', 'WS_HOSTED_SESSION_ROW', 'WS_MCP_HARNESS_PID',
    'WS_MCP_PROJECT_ROOT', 'WS_MCP_DASHBOARD_DEV_SOURCE']) delete env[key];
  return { ...env, WS_CACHE_HOME: root, ...staged.env,
    WS_MCP_CONFIG_MANIFESTS: [path.join(staged.root, 'config-manifest.json'),
      path.join(staged.root, 'dashboard-config-manifest.json')].join(';'),
    WS_MCP_RUNTIME_BINARY: staged.binary, WS_MCP_RUNTIME_DIR: staged.root, CLAUDE_PLUGIN_ROOT: staged.root };
}
async function connectDaemon({ appRoot, userData, platform = process.platform }, deps = {}) {
  const env = deps.env || process.env;
  const root = cacheRoot(env);
  const find = deps.discover || discover;
  try { return await find(root); }
  catch { /* An absent or uncertain endpoint is never evidence a process died. */ }
  if (platform !== 'darwin') {
    // In particular, Windows' breakaway fallback may die with Electron. Do
    // not invoke it until an independent-lifetime mechanism is authorized.
    try { return await find(root); } catch { /* A concurrent healthy winner may still attach. */ }
    throw new Error(platform === 'win32'
      ? 'Cannot start dashboard on Windows: independent daemon lifetime is unavailable; connect to a running daemon or retry later'
      : 'Standalone dashboard start is unavailable on this platform; connect to a running daemon');
  }
  const select = deps.select || selectDaemon;
  const stage = deps.stage || stageDaemon;
  const selected = select({ appRoot, userData, platform });
  const staged = stage(selected, userData, platform, appRoot);
  // Staging can take time: attach to a winner instead of launching a loser.
  try { return await find(root); } catch { /* Only a new dashboard may start. */ }
  try {
    await (deps.launch || launch)(staged.binary, { cwd: staged.root, env: launchEnv(env, staged, root) });
  } catch (error) { throw new Error(`Cannot start dashboard daemon: ${error.message}`); }
  const sleep = deps.sleep || (ms => new Promise(resolve => setTimeout(resolve, ms)));
  const deadline = Date.now() + 10000; // Go lock wait (5s) plus startup.
  for (let i = 0; i < (deps.maxPolls || 50); i++) {
    try { return await find(root); }
    catch { if (Date.now() >= deadline) break; }
    await sleep(200);
  }
  throw new Error('Dashboard did not become healthy (another daemon may hold the lock); no process was stopped — retry');
}
module.exports = { connectDaemon, launchEnv };
