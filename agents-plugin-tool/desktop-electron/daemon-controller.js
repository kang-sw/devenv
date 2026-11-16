'use strict';
// The dashboard entry takes StartDaemon's lock. Replacement prestarts a
// detached macOS successor before authenticated drain; Windows remains gated
// until the approved strict daemon-start implementation is integrated.
const path = require('node:path');
const { spawn } = require('node:child_process');
const { cacheRoot, inspect, shutdown } = require('./endpoint');
const { version, core } = require('./daemon-version');
const { selectDaemon } = require('./daemon-selection');
const { stageDaemon } = require('./daemon-stage');

function launch(binary, options, args = ['dashboard']) {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, args, { ...options, detached: true, stdio: 'ignore', windowsHide: true });
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
async function connectDaemon({ appRoot, userData, platform = process.platform, replace = false }, deps = {}) {
  const env = deps.env || process.env;
  const root = cacheRoot(env);
  // Injected discovery in older tests supplies a URL only, never a fabricated
  // version or endpoint credential. Real decisions use inspect's REST health.
  const find = deps.inspect || (deps.discover ? async r => ({ url: await deps.discover(r) }) : inspect);
  const select = deps.select || selectDaemon;
  const stage = deps.stage || stageDaemon;
  const notify = deps.onNotice || (() => {});
  let running;
  try { running = await find(root); }
  catch { /* An absent or uncertain endpoint is never evidence a process died. */ }
  if (running?.health) {
    let selected;
    try { selected = select({ appRoot, userData, platform }); }
    catch (error) {
      if (replace) notify(`Cannot replace daemon: ${error.message}`);
      return running.url; // Candidate failure never breaks a healthy attachment.
    }
    const relation = version(selected.metadata.version, running.health.version);
    if (relation === null) {
      notify('Cannot determine daemon core from REST health; keeping the current connection. Update the plugin before retrying.');
      return running.url;
    }
    if (relation < 0 || relation === 0 && !replace) return running.url;
    // Current REST does not enumerate connected hello versions/protocols. A
    // cross-minor swap would strand unknown clients, even when requested.
    const candidateCore = core(selected.metadata.version), currentCore = core(running.health.version);
    if (candidateCore[0] !== currentCore[0] || candidateCore[1] !== currentCore[1]) {
      notify('Cannot safely replace across minor/protocol boundary: attached client versions are unknown. Keeping this daemon; update connected plugins, then retry after verifying compatibility.');
      return running.url;
    }
    if (platform !== 'darwin') {
      notify('Replacement requires independently detached daemon-start --require-independent-lifetime; integration is pending. Keeping the current daemon.');
      return running.url;
    }
    const staged = stage(selected, userData, platform, appRoot);
    // Stage may be slow. Never drain a newer or different winner discovered
    // while copying; only replace the same observed incumbent once.
    let latest;
    try { latest = await find(root); }
    catch { notify('Dashboard health became uncertain during staging; keeping the current connection. Retry later.'); return running.url; }
    if (latest.endpoint?.pid !== running.endpoint.pid || latest.endpoint.port !== running.endpoint.port ||
        latest.health.started_at !== running.health.started_at) return latest.url;
    const refreshed = version(selected.metadata.version, latest.health.version);
    if (refreshed === null || core(selected.metadata.version).slice(0, 2).some((n, i) => n !== core(latest.health.version)[i])) {
      notify('Daemon compatibility changed during staging; keeping the current connection. Update connected plugins before retrying.');
      return latest.url;
    }
    if (refreshed < 0) return latest.url;
    // The existing dashboard entry exposes no signal proving that its detached
    // successor reached the lock wait. A spawn event alone cannot authorize
    // draining a healthy daemon. Until the Go startup seam supplies a verified
    // preflight, production replacement stays attached (no child, no drain).
    if (!deps.preflight) {
      notify('Replacement needs a verified detached successor preflight before drain; startup integration is pending. Keeping the current daemon.');
      return latest.url;
    }
    try {
      // Injected preflight models a confirmed independent successor waiting
      // on the lock; an injected spawn event alone is NOT that confirmation.
      await deps.preflight(staged.binary, { cwd: staged.root, env: launchEnv(env, staged, root) },
        ['dashboard', '--port', String(latest.endpoint.port), '--lock-wait', '8s',
          ...(latest.health.lan ? ['--lan'] : [])]);
    } catch {
      notify('Replacement preflight failed; the incumbent was not drained. Retry later.');
      return running.url;
    }
    try { await (deps.shutdown || shutdown)(root, latest); }
    catch {
      notify('Authenticated drain failed or the daemon changed; replacement outcome is uncertain. Check dashboard health before retrying.');
      return running.url;
    }
    return await waitForDaemon(find, root, deps, latest, selected.metadata.version,
      selected.metadata.binary_sha256, notify);
  }
  if (running) return running.url;
  if (platform !== 'darwin') {
    try { return (await find(root)).url; } catch { /* A concurrent winner may still attach. */ }
    throw new Error(platform === 'win32'
      ? 'Cannot start dashboard on Windows: independent daemon lifetime is unavailable (strict integration pending); connect to a running daemon or retry later'
      : 'Standalone dashboard start is unavailable on this platform; connect to a running daemon');
  }
  const selected = select({ appRoot, userData, platform });
  const staged = stage(selected, userData, platform, appRoot);
  try { return (await find(root)).url; } catch { /* Only a new dashboard may start. */ }
  try { await (deps.launch || launch)(staged.binary, { cwd: staged.root, env: launchEnv(env, staged, root) }); }
  catch (error) { throw new Error(`Cannot start dashboard daemon: ${error.message}`); }
  return waitForDaemon(find, root, deps);
}
async function waitForDaemon(find, root, deps, predecessor, candidateVersion, candidateHash, notify) {
  const sleep = deps.sleep || (ms => new Promise(resolve => setTimeout(resolve, ms)));
  const deadline = Date.now() + (predecessor ? 13000 : 10000); // Drain + successor lock + startup.
  for (let i = 0; i < (deps.maxPolls || (predecessor ? 65 : 50)); i++) {
    let answer;
    try { answer = await find(root); }
    catch { /* The old endpoint can disappear during handoff. */ }
    if (answer) {
      if (!predecessor) return answer.url;
      if (answer.endpoint?.pid !== predecessor.endpoint.pid ||
          answer.health?.started_at && answer.health.started_at !== predecessor.health.started_at) {
        // An unrelated plugin may win the lock. Attach only if its numeric
        // core is at least our candidate's; never attribute its bytes to ours.
        const relation = version(candidateVersion, answer.health?.version);
        if (relation === null || relation > 0) {
          throw new Error('An older or unidentified daemon won the handoff; retry after inspecting the current connection');
        }
        if (answer.health?.content_hash && answer.health.content_hash !== candidateHash) {
          notify?.('A different daemon won the handoff; attaching to its verified REST identity, not attributing it to the selected package.');
        }
        return answer.url;
      }
    }
    if (Date.now() >= deadline) break;
    await sleep(200);
  }
  throw new Error(predecessor
    ? 'Replacement drain was accepted but no new daemon identity became healthy; predecessor survival is not guaranteed — retry'
    : 'Dashboard did not become healthy (another daemon may hold the lock); no process was stopped — retry');
}
module.exports = { connectDaemon, launchEnv };
