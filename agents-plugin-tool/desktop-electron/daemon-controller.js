'use strict';
// macOS prestarts a detached dashboard successor before authenticated drain.
// Windows daemon-start strictly breaks away and performs that ordering in Ensure.
const path = require('node:path');
const { spawn } = require('node:child_process');
const { cacheRoot, inspect, shutdown, sameDaemon } = require('./endpoint');
const { version, core } = require('./daemon-version');
const { selectDaemon } = require('./daemon-selection');
const { stageDaemon } = require('./daemon-stage');

function launch(binary, options, args = ['dashboard'], waitForExit = false) {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, args, { ...options, detached: true, stdio: 'ignore', windowsHide: true });
    child.once('error', reject);
    child.once('spawn', () => {
      child.unref();
      if (!waitForExit) { child.removeListener('error', reject); resolve(); }
    });
    if (waitForExit) child.once('exit', code => {
      if (code === 0) resolve();
      else reject(new Error(`daemon-start exited with status ${code}`));
    });
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
    if (!['darwin', 'win32'].includes(platform)) {
      notify('Replacement is unavailable on this platform; keeping the current daemon.');
      return running.url;
    }
    const staged = stage(selected, userData, platform, appRoot);
    // Stage may be slow. Never drain a newer or different winner discovered
    // while copying; only replace the same observed incumbent once.
    let latest;
    try { latest = await find(root); }
    catch { notify('Dashboard health became uncertain during staging; keeping the current connection. Retry later.'); return running.url; }
    if (!sameDaemon(latest, running)) return attachWinner(selected.metadata.version, latest, notify);
    const refreshed = version(selected.metadata.version, latest.health.version);
    if (refreshed === null || core(selected.metadata.version).slice(0, 2).some((n, i) => n !== core(latest.health.version)[i])) {
      notify('Daemon compatibility changed during staging; keeping the current connection. Update connected plugins before retrying.');
      return latest.url;
    }
    if (refreshed < 0) return latest.url;
    const options = { cwd: staged.root, env: launchEnv(env, staged, root) };
    if (platform === 'win32') {
      // Go strict Ensure holds launcher-start.lock and repeats the LAN,
      // compatibility and numeric-core guards at its fresh REST observation.
      try {
        await (deps.launch || launch)(staged.binary, options,
          ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s', '--replace'], true);
      } catch (error) {
        let still;
        try { still = await find(root); } catch { /* A late failure can leave no daemon. */ }
        if (still && sameDaemon(still, latest)) {
          notify(`Replacement did not drain the incumbent: ${error.message}. Retry after checking daemon health.`);
          return still.url;
        }
        if (still) return attachWinner(selected.metadata.version, still, notify);
        throw new Error(`Replacement starter failed after the incumbent changed; availability is uncertain: ${error.message}`);
      }
      const still = await find(root).catch(() => null);
      if (still && sameDaemon(still, latest)) {
        notify('Replacement was refused by the starter; keeping the current connection. Check connected plugin versions before retrying.');
        return still.url;
      }
    } else {
      try {
        // Detached process creation accepted by the OS is the approved
        // preflight, not a claim of health or arrival at dashboard.lock.
        await (deps.preflight || deps.launch || launch)(staged.binary, options,
          ['dashboard', '--port', String(latest.endpoint.port), '--lock-wait', '8s',
            ...(latest.health.lan ? ['--lan'] : [])]);
      } catch {
        notify('Replacement preflight failed; the incumbent was not drained. Retry later.');
        return latest.url;
      }
      try { await (deps.shutdown || shutdown)(root, latest); }
      catch {
        notify('Authenticated drain failed or the daemon changed; outcome is uncertain. Check dashboard health before retrying.');
        const still = await find(root).catch(() => null);
        if (still && sameDaemon(still, latest)) return still.url;
        if (still) return attachWinner(selected.metadata.version, still, notify);
        throw new Error('Authenticated drain outcome uncertain; no daemon can be attached — retry after inspecting health');
      }
    }
    return await waitForDaemon(find, root, deps, latest, selected.metadata.version,
      selected.metadata.binary_sha256, notify);
  }
  if (running) return running.url;
  if (!['darwin', 'win32'].includes(platform)) {
    try { return (await find(root)).url; } catch { /* A concurrent winner may still attach. */ }
    throw new Error('Standalone dashboard start is unavailable on this platform; connect to a running daemon');
  }
  const selected = select({ appRoot, userData, platform });
  const staged = stage(selected, userData, platform, appRoot);
  try { return (await find(root)).url; } catch { /* Only a new dashboard may start. */ }
  try { await (deps.launch || launch)(staged.binary, { cwd: staged.root, env: launchEnv(env, staged, root) },
    platform === 'win32' ? ['daemon-start', '--require-independent-lifetime', '--start-wait', '13s'] : ['dashboard'],
    platform === 'win32'); }
  catch (error) { throw new Error(`Cannot start dashboard daemon: ${error.message}`); }
  return waitForDaemon(find, root, deps);
}
function sameMinorCandidate(candidate, running) {
  const a = core(candidate), b = core(running);
  return a && b && a[0] === b[0] && a[1] === b[1];
}
function attachWinner(candidate, answer, notify) {
  const relation = version(candidate, answer.health?.version);
  if (relation === null || relation > 0) {
    throw new Error('An older or unidentified daemon won the handoff; retry after inspecting the current connection');
  }
  // Unknown client versions prohibit initiating a cross-boundary drain, not
  // observing a numerically newer daemon that already won the singleton.
  if (relation < 0 && !sameMinorCandidate(candidate, answer.health.version)) {
    notify?.('A newer daemon won the handoff across a minor/protocol boundary. Update connected plugins if their hello is refused; no further replacement was attempted.');
  }
  return answer.url;
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
      if (!sameDaemon(answer, predecessor)) {
        // An unrelated plugin may win the lock. Attach only if its numeric
        // core is at least our candidate's; never attribute its bytes to ours.
        const url = attachWinner(candidateVersion, answer, notify);
        if (answer.health?.content_hash && answer.health.content_hash !== candidateHash) {
          notify?.('A different daemon won the handoff; attaching to its verified REST identity, not attributing it to the selected package.');
        }
        return url;
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
