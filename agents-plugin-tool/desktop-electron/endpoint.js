'use strict';
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const crypto = require('node:crypto');
const observedTokens = new WeakMap();
const os = require('node:os');

const MAX_JSON = 64 * 1024;
// Windows sets no HOME; os.homedir() (USERPROFILE there) is where the Go
// daemon's os.UserHomeDir puts the same cache root.
function cacheRoot(env = process.env, cwd = process.cwd(), homedir = os.homedir) {
  let root = env.WS_CACHE_HOME;
  if (!root) {
    const home = env.HOME || homedir();
    if (!home) throw new Error('Home directory is unavailable');
    root = path.join(home, '.cache/ws@kang-sw-devenv');
  }
  root = path.resolve(cwd, root);
  try { return fs.realpathSync(root); } catch { return root; }
}
function endpointFile(root) {
  let fd;
  try { fd = fs.openSync(path.join(root, 'dashboard', 'endpoint.json'), 'r'); }
  catch { throw new Error('No readable dashboard endpoint; start the dashboard normally'); }
  let raw;
  try {
    const buffer = Buffer.alloc(MAX_JSON + 1);
    let size = 0, n;
    while (size < buffer.length && (n = fs.readSync(fd, buffer, size, buffer.length - size, null))) size += n;
    if (size > MAX_JSON) throw new Error('Dashboard endpoint is too large');
    raw = buffer.subarray(0, size).toString('utf8');
  } finally { fs.closeSync(fd); }
  let value;
  try { value = JSON.parse(raw); } catch { throw new Error('Invalid dashboard endpoint'); }
  if (!value || !Number.isInteger(value.port) || value.port < 1 || value.port > 65535 ||
      !Number.isInteger(value.pid) || value.pid < 1 || value.pid > 0xffffffff) {
    throw new Error('Invalid dashboard endpoint port or PID');
  }
  return value;
}
// Discovery never returns credentials. Only the controlled shutdown path reads
// the bearer token, immediately before sending it to this same endpoint.
function readEndpoint(root) {
  const { port, pid } = endpointFile(root);
  return { port, pid };
}
function readShutdownEndpoint(root, expected) {
  const value = endpointFile(root);
  if (value.port !== expected.port || value.pid !== expected.pid ||
      typeof value.token !== 'string' || !/^[a-f0-9]{64}$/.test(value.token)) {
    throw new Error('Dashboard identity or shutdown credential changed; retry');
  }
  return { port: value.port, pid: value.pid, token: value.token };
}
function requestJSON(ep, method, route, token, timeout) {
  return new Promise((resolve, reject) => {
    let request;
    const fail = message => { request?.destroy(); reject(new Error(message)); };
    // node:http is direct (no environment proxy) and never follows redirects.
    request = http.request({ hostname: '127.0.0.1', port: ep.port, path: route,
      method, agent: false, headers: token ? { Authorization: `Bearer ${token}` } : {} }, response => {
      if (response.statusCode !== (token ? 202 : 200)) return fail(token
        ? 'Dashboard did not accept authenticated shutdown'
        : 'Dashboard health check did not return HTTP 200');
      let size = 0;
      const chunks = [];
      response.on('data', chunk => {
        size += chunk.length;
        if (size > MAX_JSON) return fail('Invalid dashboard health response');
        chunks.push(chunk);
      });
      response.on('error', () => fail('Invalid dashboard health response'));
      response.on('end', () => {
        let value;
        try { value = JSON.parse(Buffer.concat(chunks).toString('utf8')); }
        catch { return fail(token ? 'Invalid dashboard shutdown response' : 'Invalid dashboard health response'); }
        if (!token && (!value || value.service !== 'ws-dashboard' || value.pid !== ep.pid ||
            (value.port != null && value.port !== ep.port))) {
          return fail('Health response is not from the recorded dashboard daemon');
        }
        if (token && value?.shutting_down !== true) return fail('Dashboard shutdown response is invalid');
        resolve(value);
      });
    });
    const timer = setTimeout(() => fail(token ? 'Dashboard shutdown timed out' : 'Dashboard health check failed; start the dashboard normally'), timeout);
    request.on('close', () => clearTimeout(timer));
    request.on('error', () => reject(new Error(token ? 'Dashboard shutdown failed' : 'Dashboard health check failed; start the dashboard normally')));
    request.end();
  });
}
async function checkHealth(ep, timeout = 2000) {
  const health = await requestJSON(ep, 'GET', '/health', null, timeout);
  return `http://127.0.0.1:${ep.port}/`;
}
async function inspect(root = cacheRoot()) {
  const ep = readEndpoint(root);
  const health = await requestJSON(ep, 'GET', '/health', null, 2000);
  const after = readEndpoint(root);
  if (ep.port !== after.port || ep.pid !== after.pid) throw new Error('Dashboard endpoint changed; retry discovery');
  const token = endpointFile(root).token;
  if (typeof token === 'string' && /^[a-f0-9]{64}$/.test(token)) {
    observedTokens.set(ep, crypto.createHash('sha256').update(token).digest('hex'));
  }
  return { url: `http://127.0.0.1:${ep.port}/`, endpoint: ep, health };
}
async function shutdown(root, observed) {
  const ep = readShutdownEndpoint(root, observed.endpoint);
  if (!observedTokens.has(observed.endpoint) ||
      observedTokens.get(observed.endpoint) !== crypto.createHash('sha256').update(ep.token).digest('hex')) {
    throw new Error('Dashboard shutdown credential changed; retry');
  }
  // Reinspect immediately before draining: never send a stale token to a
  // different daemon. A concurrent winner after this check needs a Go guard.
  const current = await inspect(root);
  if (current.endpoint.port !== ep.port || current.endpoint.pid !== ep.pid ||
      (observed.health.started_at && current.health.started_at !== observed.health.started_at)) {
    throw new Error('Dashboard changed before shutdown; retry');
  }
  if (readShutdownEndpoint(root, ep).token !== ep.token) throw new Error('Dashboard shutdown credential changed; retry');
  await requestJSON(ep, 'POST', '/shutdown', ep.token, 3000);
}
async function discover(root = cacheRoot()) { return (await inspect(root)).url; }
module.exports = { cacheRoot, readEndpoint, checkHealth, discover, inspect, shutdown, MAX_JSON };
