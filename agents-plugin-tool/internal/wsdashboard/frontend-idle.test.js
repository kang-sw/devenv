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
  let now = 60000, paused = false, queries = 0, timer;
  const handlers = {};
  const clock = { textContent: '', getAttribute() { return '0'; } };
  const document = {
    hidden: false,
    documentElement: { hasAttribute(name) { return name === 'data-animation-paused' && paused; } },
    querySelectorAll(selector) { queries++; return selector === 'time.tago[data-at]' ? [clock] : []; },
    addEventListener(type, fn) { (handlers[type] ||= []).push(fn); }
  };
  const window = { wsT: true, wbBodyAgeHover: true };
  const context = { window, document, wsT: (key, n) => key + (n === undefined ? '' : ':' + n), Date: { now: () => now }, setInterval(fn) { timer = fn; return 1; } };
  vm.createContext(context);
  vm.runInContext(code, context);
  assert.equal(clock.textContent, 'js.wb.ago.min:1');
  document.hidden = true;
  now = 180000;
  const before = queries;
  timer(); window.wbAgo(document); handlers.visibilitychange[0]();
  assert.equal(queries, before);
  assert.equal(clock.textContent, 'js.wb.ago.min:1');
  document.hidden = false; paused = true;
  handlers.visibilitychange[0](); timer();
  assert.equal(queries, before);
  paused = false;
  handlers['ws-page-visibility'][0]();
  assert.equal(clock.textContent, 'js.wb.ago.min:3');
  vm.runInContext(code, context);
  assert.equal(handlers.visibilitychange.length, 1);
  assert.equal(handlers['ws-page-visibility'].length, 1);
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
  assert.match(layout, /#bcol \{ contain: layout style;/);
  // Files is portaled to body, but its toolbar controls must remain above
  // the body-level scrim: the toolbar's data-files state releases containment.
  const exception = layout.match(/#bcol:has\(([^\n]+)\) \{ contain: style; \}/);
  assert.ok(exception);
  for (const state of ['[data-max]', '.board-toolbar[data-files]', '.board-state-pop:not([hidden])']) assert.ok(exception[1].includes(state));
  assert.match(read(dashboard, 'cards.js'), /toolbar\.setAttribute\("data-files", ""\)/);
  const board = read(dashboard, 'dashboard-board.js');
  assert.match(board, /sheet\.id = "toolbar-files"/);
  assert.match(board, /doc\.body\.appendChild\(sheet\)/);
  assert.match(clocks, /time\.tago, time\.entry-at\[data-at\] \{[^}]*contain: layout style;/);
});
