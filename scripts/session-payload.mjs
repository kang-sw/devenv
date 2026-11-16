#!/usr/bin/env node
// Scenario (a) of board-perf.mjs, with handler/payload and framed-page evidence.
// Build internal/mcp with go test -c, then pass --bin <test binary> --out <dir>.
// Uses the same synthetic board and command protocol as board-perf.mjs.
import { spawn } from 'node:child_process';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import path from 'node:path';
import readline from 'node:readline';
import { fileURLToPath } from 'node:url';
const args = Object.fromEntries(process.argv.slice(2).reduce((a, v, i, all) => i % 2 ? a : [...a, [v.replace(/^--/, ''), all[i + 1]]], []));
const out = path.resolve(args.out);
fs.mkdirSync(out, { recursive: true });
const moduleDir = process.env.WS_PLAYWRIGHT_MODULE || process.env.PLAYWRIGHT_DIR;
if (!moduleDir) throw new Error('set WS_PLAYWRIGHT_MODULE to a cached playwright package');
const { chromium } = createRequire(path.join(moduleDir, 'package.json'))(moduleDir);
const proc = spawn(path.resolve(args.bin), ['-test.run', '^TestBoardPerfServe$', '-test.timeout', '180s'], {
  cwd: path.join(args.tree || path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../agents-plugin-tool'), 'internal/mcp'),
  env: { ...process.env, WS_BOARD_PERF_SERVE: '1', WS_BOARD_PERF_FRAMED: '1', WS_BOARD_PERF_SHAPE: args.shape || '8,6,4,2,60' },
  stdio: ['pipe', 'pipe', 'inherit'],
});
const lines = readline.createInterface({ input: proc.stdout });
const measures = [];
let ready, measured;
const url = new Promise((resolve, reject) => {
  ready = resolve;
  proc.once('exit', code => reject(new Error(`fixture exited ${code}`)));
});
lines.on('line', line => {
  if (!line.startsWith('BOARD-PERF ')) process.stderr.write(line + '\n');
  if (line.startsWith('BOARD-PERF READY ')) ready(line.slice(17));
  if (line.startsWith('BOARD-PERF MEASURE ')) measures.push(JSON.parse(line.slice(19)));
  if (line === 'BOARD-PERF OK' && measured) { measured(); measured = null; }
});
let browser;
try {
  const link = await url;
  // Run before the browser so the first sample includes the cold board memo.
  const done = new Promise(resolve => { measured = resolve; });
  proc.stdin.write('measure\n');
  await done;
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1600, height: 1000 }, colorScheme: 'light', reducedMotion: 'reduce' });
  const page = await context.newPage();
  const errors = [], wire = [], connections = [];
  page.on('pageerror', e => errors.push(e.message));
  page.on('websocket', socket => {
    connections.push(new URL(socket.url()).pathname);
    socket.on('framereceived', ({ payload }) => {
      try {
        const e = JSON.parse(String(payload));
        wire.push({ event: e.event, name: e.data?.name, bytes: Buffer.byteLength(String(payload)) });
      } catch {}
    });
  });
  const shellURL = new URL(link);
  shellURL.searchParams.delete('frame');
  await page.goto(shellURL.href, { waitUntil: 'load' });
  const session = page.frameLocator('#frames iframe.on');
  await session.locator('#r-board section.topic').first().waitFor({ state: 'attached' }).catch(async e => {
    fs.writeFileSync(path.join(out, 'failed.html'), await page.content());
    await page.screenshot({ path: path.join(out, 'failed.png') });
    throw e;
  });
  await page.waitForTimeout(2500);
  // This is the framed document itself, not the shell or a standalone page.
  const pageURL = await session.locator('html').evaluate(() => location.href);
  const response = await context.request.get(pageURL);
  const html = await response.text();
  fs.writeFileSync(path.join(out, 'page.html'), html);
  const urls = [...html.matchAll(/<(?:script|link)\b[^>]*(?:src|href)="(\/s\/[^"?]+\.(?:js|css))"/g)].map(m => m[1]);
  urls.push(...[...html.matchAll(/wb-frame-[0-9a-f]+\.(?:js|css)/g)].map(m => '/s/' + m[0]));
  const assets = [];
  for (const url of new Set(urls)) {
    const r = await context.request.get(new URL(url, pageURL).href);
    assets.push({ url, status: r.status(), bytes: (await r.body()).length, cache: r.headers()['cache-control'], type: r.headers()['content-type'] });
  }
  // Mask the ephemeral shell header/rail and wall-clock time elements.
  await page.locator('#frames iframe.on').screenshot({ path: path.join(out, 'framed.png'), animations: 'disabled', mask: [page.locator('.top'), page.locator('.rail'), session.locator('.top'), session.locator('time')] });
  const initialWire = wire.slice();
  const initialConnections = connections.length;
  const initialHash = await session.locator('#r-board').getAttribute('data-h');
  const updated = new Promise(resolve => { measured = resolve; });
  proc.stdin.write('log 1\n');
  await updated;
  await session.locator('#r-board').evaluate((el, hash) => new Promise((resolve, reject) => {
    if (el.getAttribute('data-h') !== hash) return resolve();
    const timer = setTimeout(() => { observer.disconnect(); reject(new Error('board update did not apply')); }, 10000);
    const observer = new MutationObserver(() => { if (el.getAttribute('data-h') !== hash) { clearTimeout(timer); observer.disconnect(); resolve(); } });
    observer.observe(el, { attributes: true, attributeFilter: ['data-h'] });
  }), initialHash);
  await page.waitForTimeout(1200);
  const reconnects = connections.length - initialConnections;
  const report = { shape: args.shape || '8,6,4,2,60', measures, htmlBytes: Buffer.byteLength(html), assets, initialWire, updateWire: wire.slice(initialWire.length), reconnects, errors };
  if (reconnects) throw new Error('board delta caused a reconnect');
  if (args['expect-skip'] && initialWire.some(e => e.name === 'board')) throw new Error('initial held board was resent');
  fs.writeFileSync(path.join(out, 'payload.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
  if (errors.length) throw new Error('page errors: ' + errors.join('; '));
} finally {
  if (browser) await browser.close();
  proc.stdin.write('quit\n');
  proc.stdin.end();
}
