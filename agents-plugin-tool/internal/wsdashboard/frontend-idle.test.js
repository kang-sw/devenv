const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const dashboard = path.join(__dirname, 'assets');
const whiteboard = path.join(__dirname, '../wswhiteboard/assets');
function read(dir, file) { return fs.readFileSync(path.join(dir, file), 'utf8'); }

test('whiteboard clock skips hidden pages and catches up once shown', () => {
  const code = read(whiteboard, 'ago.js');
  let now = 60000, paused = false, queries = 0, timer, starts = 0, stops = 0;
  const handlers = {};
  const clock = { textContent: '', getAttribute() { return '0'; } };
  const document = {
    hidden: false,
    documentElement: { hasAttribute(name) { return name === 'data-animation-paused' && paused; } },
    querySelectorAll(selector) { queries++; return selector === 'time.tago[data-at]' ? [clock] : []; },
    addEventListener(type, fn) { (handlers[type] ||= []).push(fn); }
  };
  const window = { wsT: true, wbBodyAgeHover: true };
  const context = { window, document, wsT: (key, n) => key + (n === undefined ? '' : ':' + n), Date: { now: () => now }, setInterval(fn) { starts++; timer = fn; return starts; }, clearInterval() { stops++; timer = null; } };
  vm.createContext(context);
  vm.runInContext(code, context);
  assert.equal(clock.textContent, 'js.wb.ago.min:1');
  document.hidden = true;
  now = 180000;
  const before = queries;
  handlers.visibilitychange[0]();
  assert.equal(timer, null);
  assert.equal(stops, 1);
  window.wbAgo(document);
  assert.equal(queries, before);
  assert.equal(clock.textContent, 'js.wb.ago.min:1');
  document.hidden = false; paused = true;
  handlers.visibilitychange[0]();
  assert.equal(timer, null);
  assert.equal(queries, before);
  paused = false;
  handlers['ws-page-visibility'][0]();
  assert.equal(starts, 2);
  assert.equal(clock.textContent, 'js.wb.ago.min:3');
  vm.runInContext(code, context);
  assert.equal(handlers.visibilitychange.length, 1);
  assert.equal(handlers['ws-page-visibility'].length, 1);
  assert.equal(starts, 2, 're-executing the asset does not add a clock');
  paused = true; handlers['ws-page-visibility'][0]();
  assert.equal(timer, null);
});

test('dashboard tick interval stops and restarts across visibility changes', () => {
  const code = read(dashboard, 'dashboard.js');
  const block = code.match(/  var tickTimer = null;[\s\S]*?  syncTickTimer\(\);/)[0];
  const handlers = {}, active = new Map(); let id = 0, ticks = 0;
  const doc = { hidden: false, addEventListener(type, fn) { handlers[type] = fn; } };
  const ctx = { doc, shHidden: false, tickTimes() { ticks++; },
    setInterval(fn) { active.set(++id, fn); return id; }, clearInterval(n) { active.delete(n); } };
  vm.runInNewContext(block, ctx);
  assert.equal(active.size, 1); assert.equal(ticks, 1);
  ctx.shHidden = true; handlers['ws-page-visibility']();
  assert.equal(active.size, 0);
  doc.hidden = true; ctx.shHidden = false; handlers['ws-page-visibility']();
  assert.equal(active.size, 0);
  doc.hidden = false; handlers['ws-page-visibility'](); handlers['ws-page-visibility']();
  assert.equal(active.size, 1); assert.equal(ticks, 3);
});

test('idle CSS bounds animation rates and contains without paint or size containment', () => {
  const base = read(dashboard, 'dashboard.css');
  const rail = read(dashboard, 'dashboard-rail.css');
  const conversation = read(dashboard, 'dashboard-conversation.css');
  const layout = read(dashboard, 'dashboard-layout.css');
  const clocks = read(whiteboard, 'page-items.css');
  assert.match(base, /dash-spin \.8s steps\(12\) infinite/);
  assert.match(base, /dash-pulse 1\.6s steps\(4\) infinite/);
  assert.match(rail, /rail-orbit 1\.3s steps\(12\) infinite/);
  assert.match(rail, /rail-blink 1\.4s steps\(4\) infinite/);
  assert.match(conversation, /dash-spin \.8s steps\(12\) infinite/);
  assert.match(base, /:root\[data-animation-paused\][^{]+\{\s*animation-play-state: paused/);
  assert.match(conversation, /\.composer \{ contain: layout style;/);
  // The board column takes no containment: the descendant :has() that
  // released it re-matched the whole board on every DOM change. Uncontained,
  // fixed sheets and popovers use the viewport, and Files, portaled to body,
  // keeps its toolbar controls above the body-level scrim.
  assert.doesNotMatch(layout, /#bcol[^{]*\{[^}]*contain/);
  assert.doesNotMatch(layout, /#bcol:has\(/);
  const board = read(dashboard, 'dashboard-board.js');
  assert.match(board, /sheet\.id = "toolbar-files"/);
  assert.match(board, /doc\.body\.appendChild\(sheet\)/);
  assert.match(clocks, /time\.tago, time\.entry-at\[data-at\] \{[^}]*contain: layout style;/);
});
