// Offline ownership/routing contracts, not a replacement for browser layout checks.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
function block(from, to) {
  const start = source.indexOf(from), end = source.indexOf(to, start);
  assert.ok(start >= 0 && end > start, 'missing production block: ' + from);
  return source.slice(start, end);
}
function func(name) {
  const match = source.match(new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
  assert.ok(match, 'missing production function: ' + name); return match[0];
}
const plain = value => JSON.parse(JSON.stringify(value));
function fixture({ layout = 'auto', width = 1920, live = true, worktree = '' } = {}) {
  const events = [], owners = [], nodes = {};
  function node() {
    return { style: {}, hidden: false, attrs: {},
      setAttribute(k, v) { this.attrs[k] = v; }, getAttribute(k) { return this.attrs[k] || null; },
      removeAttribute(k) { delete this.attrs[k]; }, appendChild(n) { return n; },
      getBoundingClientRect: () => ({ left: 600, right: 1800 }),
      focus() { events.push('focus-search'); } };
  }
  const root = node(), search = node(), fpane = node(), grid = node(), area = node();
  area.getBoundingClientRect = () => ({ right: 2400 }); grid.closest = () => area;
  fpane.querySelector = selector => selector === '.fq' ? search : null;
  nodes.bcol = node();
  const context = vm.createContext({ live, worktree, layoutMode: layout, root, fpane, grid, chead: null,
    cardIdentity: 'session', doc: { body: node(), createElement: node, activeElement: search },
    post() {}, browsePost() {}, toast() {}, copyText() {}, attachExplorerPath() {}, load() {}, save() {},
    setCtab(tab) { events.push('tab:' + tab); }, $: id => nodes[id],
    window: {
      matchMedia(query) { return { matches: width >= Number(query.match(/\d+/)[0]) }; },
      WsCards: { dismissFiles() { events.push('dismiss-sheet'); } },
      WsPreview: { open(path, options) { events.push(['card', path, options]); } },
      WsFiles: { setup(hooks) { owners.push({ setup: hooks }); }, create(el, hooks) {
        const owner = { el, hooks, shown() { events.push('shown'); }, hidden() { events.push('hidden'); } };
        owners.push(owner); return owner;
      } }
    }
  });
  vm.runInContext('var filesTab, filesSheet;\n' + block('  var filePreview = null, filePreviewBack = null;', '  var cardBoard ='), context);
  let current = null;
  const pane = { el: node(), current: () => current,
    open(path, options) { current = { path, options }; events.push(['pane', path, options]); },
    close() { if (!current) return null; const previous = current; current = null; root.removeAttribute('data-file-preview'); events.push('close'); return previous; } };
  context.filePreview = pane;
  return { context, pane, root, owners, events, search };
}
for (const [layout, below, at] of [['split', 759, 760], ['auto', 1099, 1100], ['', 1099, 1100]]) {
  test('split explorer uses existing ' + (layout || 'unset') + ' threshold', () => {
    assert.equal(fixture({ layout, width: below }).context.splitFiles(), false);
    assert.equal(fixture({ layout, width: at }).context.splitFiles(), true);
  });
}
test('tabs, non-live and worktree pages never route through split explorer', () => {
  for (const options of [{ layout: 'tabs', width: 2560 }, { live: false }, { worktree: '/tree' }]) {
    assert.equal(fixture(options).context.splitFiles(), false);
  }
});
test('only left session explorer receives preview override; lazy sheet and registered card hooks do not', () => {
  const { context: c, owners } = fixture();
  assert.equal(owners[0].setup.preview, undefined);
  assert.equal(owners[1].hooks.preview, c.previewLeftFile);
  c.filesSheet.shown(); c.filesSheet.shown(); c.filesSheet.hidden();
  assert.equal(owners.length, 3, 'sheet instance is lazy and reused');
  assert.equal(owners[2].hooks.preview, undefined);
  const worktree = fixture({ worktree: '/tree' });
  assert.equal(worktree.owners[1].hooks.preview, undefined);
});
test('split toolbar activates left Files and search; tabs leaves independent sheet to framework', () => {
  const { context: c, events } = fixture();
  assert.equal(c.filesSheet.activate(), true);
  assert.deepEqual(events, ['dismiss-sheet', 'tab:files', 'focus-search']);
  events.length = 0; c.layoutMode = 'tabs';
  assert.equal(c.filesSheet.activate(), false); assert.deepEqual(events, []);
});
test('left single click opens pane across board and pin column; durable intent closes before card reveal', () => {
  const { context: c, pane, root, events, search } = fixture();
  const options = { root: '/other', line: 12, answer: { kind: 'text', text: 'preflight' } };
  c.previewLeftFile('a.txt', options);
  assert.equal(c.filePreviewBack, search);
  assert.equal(root.attrs['data-file-preview'], '');
  assert.equal(pane.current().options, options);
  assert.equal(pane.el.style.width, '1800px', 'includes pin column beyond grid right');
  assert.deepEqual(events, [['pane', 'a.txt', options]]);
  events.length = 0;
  c.previewLeftFile('a.txt', { ...options, keep: true });
  assert.deepEqual(events, ['close', ['card', 'a.txt', { ...options, keep: true }]]);
  assert.equal(pane.current(), null); assert.equal(root.attrs['data-file-preview'], undefined);
});
test('narrow left clicks retain board preview; absent pane is safe fallback', () => {
  for (const options of [{ layout: 'tabs' }, { layout: 'auto', width: 1099 }, { layout: 'split', width: 759 }]) {
    const { context: c, events } = fixture(options);
    c.previewLeftFile('a.txt', {});
    assert.deepEqual(events, [['card', 'a.txt', {}]]);
  }
  const { context: c, events } = fixture(); c.filePreview = null;
  c.previewLeftFile('a.txt', {}); assert.deepEqual(events, [['card', 'a.txt', {}]]);
});
test('leaving split transfers same file/options into transient card once, without mutating source options', () => {
  const { context: c, pane, events } = fixture();
  const options = { root: '/chosen', line: 5, keep: true, answer: { kind: 'text', text: 'cached' } };
  pane.open('same.txt', options); events.length = 0; c.layoutMode = 'tabs';
  c.syncFilePreview(); c.syncFilePreview();
  assert.deepEqual(plain(events), ['close', ['card', 'same.txt', { ...options, keep: false }]]);
  assert.equal(options.keep, true); assert.equal(pane.current(), null);
});
test('split resize dismisses obsolete sheet and resizes current pane without opening cards', () => {
  const { context: c, pane, events } = fixture();
  c.previewLeftFile('a.txt', {}); events.length = 0;
  c.grid.getBoundingClientRect = () => ({ right: 2600 }); c.syncFilePreview();
  assert.equal(pane.el.style.width, '2000px'); assert.deepEqual(events, ['dismiss-sheet']);
});
test('revealPanes applies layout fallback before lifecycle callbacks and retains shown/hidden semantics', () => {
  const { context: c, pane, events } = fixture();
  Object.assign(c, { PANES: [], PANE: {}, paneVisible: {}, paneScroll: {}, filesSeen: false,
    convo: {}, pinConvo() {}, nearTop() {}, visible: n => n === c.fpane && c.layoutMode !== 'tabs', ctabDot: () => null });
  vm.runInContext(func('revealPanes'), c);
  c.revealPanes(); assert.deepEqual(events, ['dismiss-sheet', 'shown']);
  events.length = 0; pane.open('a.txt', {}); events.length = 0; c.layoutMode = 'tabs';
  c.revealPanes();
  assert.deepEqual(plain(events), ['close', ['card', 'a.txt', { keep: false }], 'hidden']);
});
test('pane CSS preserves board/pin geometry and hides overlay while closed', () => {
  const css = readAssetSource('wsdashboard', 'dashboard.css');
  assert.match(css, /\.file-preview-pane\[hidden\]\s*\{\s*display:\s*none;/);
  const rule = css.match(/:root\[data-file-preview\] \.dgrid\.live > \.bcol,[^}]+\}/);
  assert.ok(rule, 'missing preview-only board/pin hiding rule');
  for (const selector of ['.bcol', '.dcv-edge', '.bpcol', '.bpzone']) assert.ok(rule[0].includes(selector));
  assert.match(rule[0], /visibility:\s*hidden/); assert.doesNotMatch(rule[0], /display:\s*none/);
});
