// Opt-in, read-only production-page replay. Only Node and a cached Chromium
// run locally: never a Go/ws executable, launcher or daemon mutation.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const { readAssetSource } = require('./asset-source.cjs');
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
class CDP {
  constructor(ws) {
    this.ws = ws; this.id = 0; this.pending = new Map(); this.listeners = [];
    ws.onmessage = event => {
      const m = JSON.parse(event.data), p = this.pending.get(m.id);
      if (p) { this.pending.delete(m.id); clearTimeout(p.timer); m.error ? p.reject(Error(JSON.stringify(m.error))) : p.resolve(m.result); }
      else this.listeners.slice().forEach(fn => fn(m));
    };
  }
  send(method, params = {}, sessionId) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(Error('CDP timeout: ' + method)); }, 30000);
      this.pending.set(id, { resolve, reject, timer });
      this.ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    });
  }
  wait(method) {
    return new Promise(resolve => {
      const fn = m => { if (m.method === method) { this.listeners.splice(this.listeners.indexOf(fn), 1); resolve(m.params); } };
      this.listeners.push(fn);
    });
  }
}
const probe = `(() => {
  const p = window.__cacheProbe = { timer: 0, raf: 0, messages: 0, requests: 0, sockets: 0, intervals: 0, lastLongTask: 0, lastHeight: 0 };
  new PerformanceObserver(list => { if (list.getEntries().length) p.lastLongTask=performance.now(); }).observe({type:'longtask',buffered:true});
  window.addEventListener('message',e=>{if(e.data?.wbDoc==='height')p.lastHeight=performance.now();});
  const interval = window.setInterval, clear = window.clearInterval, timeout = window.setTimeout, raf = window.requestAnimationFrame;
  const intervals = new Set();
  window.setInterval = function(fn, ms, ...args) { const id = interval.call(this, () => { p.timer++; typeof fn === 'function' ? fn(...args) : (0,eval)(fn); }, ms); intervals.add(id); p.intervals = intervals.size; return id; };
  window.clearInterval = function(id) { intervals.delete(id); p.intervals = intervals.size; return clear.call(this,id); };
  window.setTimeout = function(fn, ms, ...args) { return timeout.call(this, () => { p.timer++; typeof fn === 'function' ? fn(...args) : (0,eval)(fn); }, ms); };
  window.requestAnimationFrame = function(fn) { return raf.call(this, t => { p.raf++; fn(t); }); };
  const Socket = window.WebSocket;
  window.WebSocket = class extends Socket { constructor(...args) { super(...args); let opened=false; this.addEventListener('open',()=>{opened=true;p.sockets++;}); this.addEventListener('close',()=>{if(opened)p.sockets--;}); this.addEventListener('message',()=>p.messages++); } };
  const fetch = window.fetch; window.fetch = function(...args) { p.requests++; return fetch.apply(this,args); };
})();`;
function overlay(html, limit) {
  return html.replace(/<script([^>]*)>([\s\S]*?)<\/script>/g, (all, attrs, body) => {
    if (body.includes('// window.WsShell:')) body = readAssetSource('wsdashboard', 'shell.js').replace('var CACHE_LIMIT = 5;', 'var CACHE_LIMIT = ' + limit + ';');
    else if (body.includes('var shFrames = []')) body = readAssetSource('wsdashboard', 'dashboard.js');
    else if (body.includes('// Updated-ago:')) body = readAssetSource('wswhiteboard', 'ago.js');
    else if (body.includes('window.WsTerm = {')) body = readAssetSource('wsdashboard', 'terminal.js');
    else return all;
    return '<script' + attrs + '>' + body + '</script>';
  });
}
async function replay(origin, limit, ids, snapshots) {
  const sockets = new Set(), requests = [];
  const server = http.createServer(async (req, res) => {
    try {
      const u = new URL(req.url, origin);
      if (req.method !== 'GET') { res.writeHead(405); res.end('read-only replay'); return; }
      requests.push(u.pathname + u.search);
      if (/\/dashboard-[a-f0-9]+\.js$/.test(u.pathname)) {
        res.setHeader('Content-Type', 'application/javascript'); res.end(readAssetSource('wsdashboard', 'dashboard.js')); return;
      }
      const snapshot = snapshots.get(u.pathname + u.search);
      const upstream = snapshot || await fetch(u);
      let body = snapshot ? snapshot.body : Buffer.from(await upstream.arrayBuffer());
      const type = upstream.headers.get('content-type') || '';
      res.statusCode = upstream.status;
      for (const key of ['content-type', 'cache-control', 'content-security-policy']) if (upstream.headers.has(key)) res.setHeader(key, upstream.headers.get(key));
      if (type.includes('text/html')) body = Buffer.from(overlay(body.toString(), limit));
      res.end(body);
    } catch (e) { res.statusCode = 502; res.end(e.message); }
  });
  server.on('connection', socket => { sockets.add(socket); socket.on('close', () => sockets.delete(socket)); });
  server.on('upgrade', (req, socket, head) => {
    // Freeze the captured document revision in both runs. Real runtime
    // WebSockets receive a 1 Hz application ping, not changing owner data;
    // no WebSocket or write reaches the source daemon.
    if (!/^\/(?:rail|s\/[a-z0-9]+)?\/?events(?:\?|$)/.test(req.url)) { socket.destroy(); return; }
    const accept = crypto.createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64');
    socket.write('HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ' + accept + '\r\n\r\n');
    const payload = Buffer.from('{"event":"ping"}'), message = Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
    const timer = setInterval(() => { if (!socket.destroyed) socket.write(message); }, 1000);
    socket.on('data', data => { if ((data[0] & 15) === 8) socket.end(Buffer.from([0x88, 0])); });
    socket.on('close', () => clearInterval(timer)); socket.on('error', () => socket.destroy());
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  return { url: 'http://127.0.0.1:' + server.address().port + '/s/' + ids[0], requests,
    close() { for (const socket of sockets) socket.destroy(); return new Promise(resolve => server.close(resolve)); } };
}
async function browser(executable, profile) {
  const child = spawn(executable, ['--headless=new', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update', '--disable-extensions', '--remote-debugging-port=0', '--user-data-dir=' + profile, 'about:blank'], { stdio: ['ignore', 'ignore', 'pipe'], env: { ...process.env, TEMP: path.dirname(profile), TMP: path.dirname(profile) } });
  const address = await new Promise((resolve, reject) => {
    let text = ''; const timer = setTimeout(() => reject(Error('Chromium startup timeout: ' + text.slice(-2000))), 30000);
    child.once('error', e => { clearTimeout(timer); reject(e); });
    child.stderr.on('data', data => { text += data; const found = text.match(/DevTools listening on (ws:\/\/\S+)/); if (found) { clearTimeout(timer); resolve(found[1]); } });
  });
  const ws = new WebSocket(address); await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
  return { child, cdp: new CDP(ws), ws };
}
async function measure(executable, origin, ids, limit, root, snapshots) {
  const server = await replay(origin, limit, ids, snapshots); let chrome;
  try {
    chrome = await browser(executable, path.join(root, 'chrome-' + limit));
    const c = chrome.cdp, target = await c.send('Target.createTarget', { url: 'about:blank' });
    const { sessionId: session } = await c.send('Target.attachToTarget', { targetId: target.targetId, flatten: true });
    await c.send('Page.enable', {}, session); await c.send('Runtime.enable', {}, session); await c.send('Performance.enable', {}, session);
    await c.send('Page.addScriptToEvaluateOnNewDocument', { source: probe }, session);
    const errors = [];
    c.listeners.push(m => { if (m.sessionId === session && m.method === 'Runtime.exceptionThrown') errors.push(m.params.exceptionDetails.text + ': ' + (m.params.exceptionDetails.exception?.description || '')); });
    async function evaluate(expression) {
      const result = await c.send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }, session);
      if (result.exceptionDetails) throw Error(result.exceptionDetails.exception?.description || result.exceptionDetails.text);
      return result.result.value;
    }
    async function until(expression) {
      const end = Date.now() + 30000;
      while (Date.now() < end) { if (await evaluate(expression)) return; await delay(25); }
      throw Error('Page did not settle: ' + expression);
    }
    const active = `document.querySelector('#frames > iframe.on')`;
    const ready = id => `(() => {const f=${active};try{return !!f && f.contentWindow.location.pathname==='/s/${id}' && f.contentWindow.document.readyState==='complete' && typeof f.contentWindow.wsCanCacheSession==='function';}catch{return false;}})()`;
    await c.send('Page.navigate', { url: server.url }, session); await until(ready(ids[0])); await delay(2500);
    const initial = await evaluate(`(() => {const w=${active}.contentWindow;w.__cacheIdentity='original';return {topics:w.document.querySelectorAll('section.topic[data-topic]').length,nested:w.document.querySelectorAll('iframe.doc-frame').length,safe:w.wsCanCacheSession()};})()`);
    assert.equal(initial.safe, true, 'representative session must satisfy real cache eligibility');
    async function navigate(id) {
      const start = await evaluate(`performance.now()`), before = server.requests.length;
      await evaluate(`(() => {const a=document.createElement('a');a.href='/s/${id}';document.body.append(a);a.click();a.remove();})()`);
      await until(ready(id));
      const outerReadyMs = +(await evaluate('performance.now()') - start).toFixed(1);
      await until(`performance.now()-${start}>=500 && (()=>{const w=${active}.contentWindow,p=w.__cacheProbe;return w.performance.now()-Math.max(p.lastLongTask,p.lastHeight)>=500;})()`);
      return { outerReadyMs, settledMs: +(await evaluate('performance.now()') - start).toFixed(1), documentRequests: server.requests.slice(before).filter(u => u === '/s/' + id + '?frame=1').length };
    }
    await navigate(ids[1]); await delay(2500);
    const hidden = `Array.from(document.querySelectorAll('#frames > iframe')).find(f=>new URL(f.src).pathname==='/s/${ids[0]}')`;
    const hiddenInfo = `(() => {const f=${hidden};if(!f)return null;const w=f.contentWindow,d=w.document;return {...w.__cacheProbe,nested:d.querySelectorAll('iframe').length,paused:d.documentElement.hasAttribute('data-animation-paused'),runningAnimations:d.getAnimations().filter(a=>a.playState==='running').length};})()`;
    const before = await evaluate(hiddenInfo);
    if (limit) { assert.ok(before); assert.equal(before.paused, true); assert.equal(before.nested, 0); assert.equal(before.intervals, 0); assert.equal(before.sockets, 0); }
    else assert.equal(before, null);
    const metric = result => Object.fromEntries(result.metrics.map(m => [m.name, m.value]));
    const metricsBefore = metric(await c.send('Performance.getMetrics', {}, session));
    await c.send('Tracing.start', { categories: 'devtools.timeline,blink,cc,gpu,disabled-by-default-devtools.timeline', transferMode: 'ReturnAsStream' });
    await delay(10000); // No browser-side polling, screenshots, mouse or CPU profiler.
    const completed = c.wait('Tracing.tracingComplete'); await c.send('Tracing.end');
    const { stream } = await completed; let trace = '';
    for (;;) { const part = await c.send('IO.read', { handle: stream }); trace += part.data; if (part.eof) break; }
    await c.send('IO.close', { handle: stream });
    const metricsAfter = metric(await c.send('Performance.getMetrics', {}, session));
    const after = await evaluate(hiddenInfo), hiddenDelta = {};
    if (limit) for (const key of ['timer', 'raf', 'messages', 'requests']) { hiddenDelta[key] = after[key] - before[key]; assert.equal(hiddenDelta[key], 0, 'hidden cached ' + key + ' work'); }
    const events = JSON.parse(trace).traceEvents, main = new Set(events.filter(e => e.name === 'thread_name' && e.args?.name === 'CrRendererMain').map(e => e.pid + ':' + e.tid));
    const busy = events.filter(e => main.has(e.pid + ':' + e.tid) && e.ph === 'X' && e.name === 'RunTask').reduce((n,e) => n + e.dur, 0) / 1000;
    const back = await navigate(ids[0]);
    const reused = await evaluate(`${active}.contentWindow.__cacheIdentity==='original'`);
    assert.equal(reused, limit > 0); assert.equal(back.documentRequests, limit ? 0 : 1);
    assert.deepEqual(errors, [], 'production replay page errors');
    return { cacheLimit: limit, initial, hiddenDelta: limit ? hiddenDelta : null, retainedFrames: await evaluate(`document.querySelectorAll('#frames > iframe').length`),
      idle10s: { taskMs: +((metricsAfter.TaskDuration-metricsBefore.TaskDuration)*1000).toFixed(2), layoutMs: +((metricsAfter.LayoutDuration-metricsBefore.LayoutDuration)*1000).toFixed(2), styleMs: +((metricsAfter.RecalcStyleDuration-metricsBefore.RecalcStyleDuration)*1000).toFixed(2), traceRendererRunTaskMs: +busy.toFixed(2) }, returnSwitch: back, reused };
  } finally {
    if (chrome) {
      try { await chrome.cdp.send('Browser.close'); } catch {}
      chrome.ws.close();
      if (chrome.child.exitCode === null && chrome.child.signalCode === null) await new Promise(resolve => {
        const timer = setTimeout(() => chrome.child.kill(), 10000);
        chrome.child.once('exit', () => { clearTimeout(timer); resolve(); });
      });
    }
    await server.close();
  }
}
test('production replay: cached return avoids document reload and hidden descendants are idle for 10 s', { timeout: 180000, skip: !process.env.WS_CACHE_BROWSER || !process.env.WS_CACHE_SOURCE_URL || !process.env.WS_CACHE_SESSION_IDS }, async () => {
  const parent = 'D:/Temp/iframe-cache', ids = process.env.WS_CACHE_SESSION_IDS.split(',');
  assert.equal(ids.length, 2); assert.ok(ids.every(id => /^[a-z0-9]{1,64}$/.test(id)));
  const executable = process.env.WS_CACHE_BROWSER, origin = new URL(process.env.WS_CACHE_SOURCE_URL);
  assert.equal(origin.hostname, '127.0.0.1', 'only an explicitly supplied loopback daemon is read');
  fs.mkdirSync(parent, { recursive: true });
  const root = fs.mkdtempSync(path.join(parent, 'run-'));
  try {
    const snapshots = new Map();
    for (const id of ids) for (const suffix of ['', '?frame=1']) {
      const route = '/s/' + id + suffix, response = await fetch(new URL(route, origin));
      assert.equal(response.status, 200);
      snapshots.set(route, { status: response.status, headers: response.headers, body: Buffer.from(await response.arrayBuffer()) });
    }
    const off = await measure(executable, origin, ids, 0, root, snapshots), on = await measure(executable, origin, ids, 5, root, snapshots);
    assert.equal(off.initial.topics, on.initial.topics, 'identical captured board revision');
    console.log('SESSION_CACHE_MEASUREMENT ' + JSON.stringify({ method: 'frozen production HTML replay + 1Hz event ping; actual source runtime; 500ms quiet after last height/long task; 10s Chrome timeline/CDP idle, no screenshots/CPU profiler; GET-only source', off, on }));
  } finally {
    await fs.promises.rm(root, { recursive: true, force: true, maxRetries: 20, retryDelay: 200 });
    try { fs.rmdirSync(parent); } catch (e) { if (e.code !== 'ENOTEMPTY' && e.code !== 'ENOENT') throw e; }
  }
});
