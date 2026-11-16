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
// Discovery never returns credentials. Controlled shutdown and browser-login
// helpers read the bearer only in the main process, immediately before use.
function readEndpoint(root) {
  const { port, pid } = endpointFile(root);
  return { port, pid };
}
function readBearerEndpoint(root, expected) {
  const value = endpointFile(root);
  if (value.port !== expected.port || value.pid !== expected.pid ||
      typeof value.token !== 'string' || !/^[a-f0-9]{64}$/.test(value.token)) {
    throw new Error('Dashboard identity or credential changed; retry');
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
  const initial = endpointFile(root);
  const ep = { port: initial.port, pid: initial.pid }; // Never return a bearer credential.
  const health = await requestJSON(ep, 'GET', '/health', null, 2000);
  const after = endpointFile(root);
  if (ep.port !== after.port || ep.pid !== after.pid || initial.token !== after.token) {
    throw new Error('Dashboard endpoint changed; retry discovery');
  }
  if (typeof initial.token === 'string' && /^[a-f0-9]{64}$/.test(initial.token)) {
    observedTokens.set(ep, crypto.createHash('sha256').update(initial.token).digest('hex'));
  }
  return { url: `http://127.0.0.1:${ep.port}/`, endpoint: ep, health };
}
function sameDaemon(a, b) {
  return a?.endpoint?.pid === b?.endpoint?.pid && a?.endpoint?.port === b?.endpoint?.port &&
    a?.health?.started_at === b?.health?.started_at &&
    observedTokens.get(a.endpoint) === observedTokens.get(b.endpoint);
}
async function checkedEndpoint(root, observed) {
  const ep = readBearerEndpoint(root, observed.endpoint);
  if (!observedTokens.has(observed.endpoint) ||
      observedTokens.get(observed.endpoint) !== crypto.createHash('sha256').update(ep.token).digest('hex')) {
    throw new Error('Dashboard credential changed; retry');
  }
  // Reinspect immediately before authenticated use: never send a stale token
  // to a different observed daemon. The route still verifies the bearer.
  const current = await inspect(root);
  if (!sameDaemon(observed, current)) {
    throw new Error('Dashboard changed before authenticated request; retry');
  }
  if (readBearerEndpoint(root, ep).token !== ep.token) throw new Error('Dashboard credential changed; retry');
  return ep;
}
async function shutdown(root, observed) {
  const ep = await checkedEndpoint(root, observed);
  await requestJSON(ep, 'POST', '/shutdown', ep.token, 3000);
}
async function authJSON(root, observed, route, body = {}) {
  const ep = await checkedEndpoint(root, observed);
  return new Promise((resolve, reject) => {
    let request;
    const fail = () => { request?.destroy(); reject(new Error('Dashboard browser login request failed')); };
    const payload = JSON.stringify(body);
    // Direct loopback only; no Origin, cookies, proxy or redirect following.
    request = http.request({ hostname: '127.0.0.1', port: ep.port, path: route,
      method: 'POST', agent: false, headers: { Authorization: `Bearer ${ep.token}`,
        'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(payload) } }, response => {
      // An old daemon has no auth route. Do not try another credential path.
      if (response.statusCode === 404) { response.resume(); resolve(undefined); return; }
      if (response.statusCode !== 200) return fail();
      let size = 0;
      const chunks = [];
      response.on('data', chunk => {
        size += chunk.length;
        if (size > MAX_JSON) return fail();
        chunks.push(chunk);
      });
      response.on('error', fail);
      response.on('aborted', fail);
      response.on('end', () => {
        try { resolve(JSON.parse(Buffer.concat(chunks).toString('utf8'))); }
        catch { fail(); }
      });
    });
    const timer = setTimeout(fail, 3000);
    request.on('close', () => clearTimeout(timer));
    request.on('error', fail);
    request.end(payload);
  });
}
async function browserCookie(root, observed) {
  const value = await authJSON(root, observed, '/auth/cookie');
  if (value === undefined) return null;
  if (!value || typeof value.name !== 'string' || !/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(value.name) ||
      typeof value.value !== 'string' || !/^[\x21\x23-\x2b\x2d-\x3a\x3c-\x5b\x5d-\x7e]+$/.test(value.value) ||
      !Number.isInteger(value.maxAge) || value.maxAge <= 0 || value.maxAge > 34560000) {
    throw new Error('Invalid dashboard browser cookie response');
  }
  return { name: value.name, value: value.value, maxAge: value.maxAge };
}
async function browserLoginLink(root, observed, path = '/') {
  const value = await authJSON(root, observed, '/auth/link', { path });
  let url;
  try { url = new URL(value?.url); } catch { throw new Error('Invalid dashboard browser login link'); }
  if (url.origin !== new URL(observed.url).origin || url.username || url.password || !url.searchParams.get('login')) {
    throw new Error('Invalid dashboard browser login link');
  }
  return url.href;
}
async function discover(root = cacheRoot()) { return (await inspect(root)).url; }
module.exports = { cacheRoot, readEndpoint, checkHealth, discover, inspect, shutdown, sameDaemon, browserCookie, browserLoginLink, MAX_JSON };
