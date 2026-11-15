// Test-only launch/network accounting, modeled on the isolated effects fixture.
// Loaded with NODE_OPTIONS only by the opt-in TASK3 fixture, never by a page.
const fs = require('node:fs');
const path = require('node:path');
const cp = require('node:child_process');
const { fileURLToPath } = require('node:url');
const output = process.env.WS_BOARD_ACCEPTANCE_OUTPUT;
const gate = process.env.WS_BOARD_ACCEPTANCE_GATE;
if (!output || !gate) throw new Error('Acceptance requires retained output and an owned launch gate');
const origins = new Set(JSON.parse(process.env.WS_BOARD_ACCEPTANCE_ORIGINS || '[]'));
const files = new Set(JSON.parse(process.env.WS_BOARD_ACCEPTANCE_FILES || '[]').map(p => path.resolve(p)));
for (const origin of origins) {
  const u = new URL(origin);
  if (!['127.0.0.1', 'localhost', '[::1]'].includes(u.hostname) || !u.port || u.port === '29417') throw new Error('Only test-owned non-shared origins are permitted');
}
fs.mkdirSync(output, { recursive: true });
const record = data => fs.appendFileSync(path.join(output, 'browser-observation.jsonl'), JSON.stringify({ utc: new Date().toISOString(), pid: process.pid, ...data }) + '\n');
const originalSpawn = cp.spawn;
cp.spawn = function(file, args, options) {
  if (!Array.isArray(args)) throw new Error('Refuse an unregistered child launch');
  record({ kind: 'spawn-intent', file, args, detached: !!options?.detached });
  return originalSpawn.call(this, '/opt/homebrew/bin/python3', [gate, file, ...args], options);
};
const { chromium } = require(process.env.WS_PLAYWRIGHT_MODULE);
const launch = chromium.launch.bind(chromium);
chromium.launch = async function(options) {
  record({ kind: 'browser-launch', options });
  const browser = await launch(options);
  const newContext = browser.newContext.bind(browser);
  let serial = 0;
  const contexts = new Set();
  const closeBrowser = browser.close.bind(browser);
  browser.close = async function(...args) {
    let failure;
    for (const context of [...contexts]) {
      try { await context.close(); } catch (error) { failure = error; process.exitCode = 1; }
    }
    await closeBrowser(...args);
    if (failure) throw failure;
  };
  browser.newContext = async function(options) {
    const context = await newContext({ ...options, serviceWorkers: 'allow' });
    contexts.add(context);
    const id = ++serial, errors = [], consoleErrors = [], denied = [], attempts = [], workers = [];
    record({ kind: 'context', id, options });
    context.setDefaultTimeout(10000);
    await context.exposeBinding('__wsAcceptanceWorkerAttempt', ({ frame }, scriptURL) => {
      const event = { frame: frame.url(), scriptURL }; attempts.push(event); record({ kind: 'service-worker-attempt', id, ...event });
    });
    context.on('serviceworker', worker => { workers.push(worker.url()); record({ kind: 'service-worker-created', id, url: worker.url() }); });
    await context.addInitScript(() => {
      // Opaque sandbox frames retain the browser's native restriction. Reading
      // their navigator.serviceWorker itself throws, so do not inspect it.
      if (self.origin === 'null') return;
      const serviceWorkers = navigator.serviceWorker;
      if (!serviceWorkers) return;
      Object.getPrototypeOf(serviceWorkers).register = async function(scriptURL) {
        await globalThis.__wsAcceptanceWorkerAttempt(typeof scriptURL === 'string' ? scriptURL : { type: typeof scriptURL });
        throw new Error('Service worker registration is forbidden in this test fixture');
      };
    });
    await context.route('**/*', route => {
      const request = route.request(), u = new URL(request.url());
      const allowed = ['http:', 'https:'].includes(u.protocol) ? origins.has(u.origin) :
        u.protocol === 'file:' ? files.has(path.resolve(fileURLToPath(u))) : ['data:', 'about:', 'blob:'].includes(u.protocol);
      record({ kind: 'request-admission', id, method: request.method(), url: request.url(), allowed });
      if (!allowed) { denied.push(request.url()); return route.abort('blockedbyclient'); }
      return route.continue();
    });
    context.on('page', page => {
      page.on('pageerror', error => { errors.push(error.message); record({ kind: 'pageerror', id, name: error.name, message: error.message, stack: error.stack }); });
      page.on('console', message => { if (message.type() === 'error') consoleErrors.push(message.text()); record({ kind: 'console', id, type: message.type(), text: message.text(), location: message.location() }); });
      page.on('request', request => record({ kind: 'request', id, method: request.method(), url: request.url() }));
      page.on('response', response => record({ kind: 'response', id, url: response.url(), status: response.status() }));
      page.on('requestfailed', request => record({ kind: 'requestfailed', id, url: request.url(), failure: request.failure() }));
    });
    const close = context.close.bind(context);
    context.close = async function(...args) {
      const remaining = context.serviceWorkers().map(worker => worker.url());
      const summary = { id, errors, consoleErrors, denied, attempts, workers, remaining, opaqueFrames: 'Native restriction retained; registration API not inspected in opaque frames.' };
      fs.writeFileSync(path.join(output, `context-${id}.json`), JSON.stringify(summary, null, 2));
      record({ kind: 'context-close', ...summary });
      await close(...args);
      contexts.delete(context);
      if ([errors, consoleErrors, denied, attempts, workers, remaining].some(a => a.length)) {
        process.exitCode = 1;
        throw new Error('Acceptance browser errors or forbidden activity: ' + JSON.stringify(summary));
      }
    };
    return context;
  };
  return browser;
};
record({ kind: 'node-ready', argv: process.argv });
