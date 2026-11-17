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
async function settle() { for (let i = 0; i < 12; i++) await Promise.resolve(); }
function descendant(el, tag) {
  for (const child of el.children) { if (child.tagName === tag) return child; const found = descendant(child, tag); if (found) return found; }
  return null;
}
function fixture({ layout = 'auto', width = 1920, live = true, worktree = '', constructPane = false } = {}) {
  const events = [], owners = [], nodes = {};
  let context;
  function node(tag = 'div') {
    return { tagName: tag.toUpperCase(), style: {
      setProperty(k, v) { this[k] = v; }, removeProperty(k) { delete this[k]; }
    }, hidden: false, attrs: {}, children: [], listeners: {}, isConnected: true,
      setAttribute(k, v) { this.attrs[k] = v; }, getAttribute(k) { return this.attrs[k] || null; },
      hasAttribute(k) { return k in this.attrs; }, removeAttribute(k) { delete this.attrs[k]; },
      appendChild(n) { this.children.push(n); n.parentNode = this; return n; },
      insertBefore(n, before) { if (n.parentNode) n.parentNode.removeChild(n); this.children.splice(this.children.indexOf(before), 0, n); n.parentNode = this; },
      get firstChild() { return this.children[0]; },
      removeChild(n) { this.children.splice(this.children.indexOf(n), 1); n.parentNode = null; },
      querySelector(selector) { return selector === '.fq' ? search : null; },
      contains(n) { return this === n || this.children.some(child => child.contains(n)); },
      get textContent() { return this.text || this.children.map(child => child.textContent).join(''); },
      set textContent(value) {
        // Browser focus falls back to body when rendered focused content is removed.
        if (context && this.children.some(child => child.contains(context.doc.activeElement))) context.doc.activeElement = context.doc.body;
        this.children = []; this.text = value;
      },
      addEventListener(type, listener, options) { this.listeners[type] = listener; (this.listenerOptions ||= {})[type] = options; },
      fire(type, event = {}) { if (this.listeners[type]) this.listeners[type](event); },
      getBoundingClientRect: () => ({ left: 600, right: 1800 }),
      focus() { events.push('focus-search'); context.doc.activeElement = this; } };
  }
  const root = node(), search = node(), fpane = node(), grid = node(), area = node(), wrap = node(), chead = node();
  chead.querySelector = () => null;
  chead.getBoundingClientRect = () => ({ left: 24, width: 560 });
  root.clientWidth = width; wrap.paddingRight = '16px';
  // Closed layout stays capped even when the open preview must reach farther.
  area.getBoundingClientRect = grid.getBoundingClientRect = () => ({ top: 80, height: 900, right: 1664 });
  grid.closest = selector => { assert.ok(['.dbody', '.dwrap'].includes(selector)); return selector === '.dwrap' ? wrap : area; };
  fpane.querySelector = selector => selector === '.fq' ? search : null;
  nodes.bcol = node();
  context = vm.createContext({ live, worktree, layoutMode: layout, root, fpane, grid, dbody: area, chead,
    cardIdentity: 'session', doc: Object.assign(node('document'), { documentElement: root, body: node(), createElement: node, activeElement: search }),
    post() {}, browsePost() {}, toast() {}, copyText() {}, attachExplorerPath() {}, load() {}, save() {},
    setCtab(tab) { events.push('tab:' + tab); }, $: id => nodes[id],
    window: {
      innerWidth: width, addEventListener() {},
      matchMedia(query) { return { matches: root.clientWidth >= Number(query.match(/\d+/)[0]) }; },
      getComputedStyle(el) { assert.equal(el, wrap); return { paddingRight: wrap.paddingRight }; },
      WsCards: { dismissFiles() { events.push('dismiss-sheet'); }, register() {}, icon: name => name },
      WsPreview: { open(path, options) { events.push(['card', path, options]); } },
      WsFiles: { setup(hooks) { owners.push({ setup: hooks }); }, create(el, hooks) {
        const owner = { el, hooks, shown() { events.push('shown'); }, hidden() { events.push('hidden'); }, cancelPreview() { events.push('cancel-preview'); } };
        owners.push(owner); return owner;
      } }
    }
  });
  vm.runInContext('var filesTab, filesSheet;\n' + block('  var filePreview = null, filePreviewBack = null;', '  var cardBoard ='), context);
  if (constructPane) {
    context.document = context.doc; context.wsT = key => key;
    context.fetch = () => { throw new Error('unexpected preview request'); };
    vm.runInContext(readAssetSource('wsdashboard', 'preview.js'), context);
    context.window.WsPreview.open = (path, options) => events.push(['card', path, options]);
    // Execute the production constructor and its closed callback, not a source assertion.
    vm.runInContext(block('    if (live && !worktree && window.WsPreview) {', '    window.WsCards.mount(cardBoard,'), context);
    const pane = context.filePreview, open = pane.open, close = pane.close;
    pane.open = (path, options) => { events.push(['pane', path, options]); return open(path, options); };
    pane.close = () => { if (pane.current()) events.push('close'); return close(); };
    return { context, pane, root, owners, events, search };
  }
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
test('independent left and toolbar explorers receive pane routing; registered cards do not', () => {
  const { context: c, owners } = fixture();
  assert.equal(owners[0].setup.preview, undefined);
  assert.equal(owners[1].hooks.preview, c.previewLeftFile);
  c.filesSheet.shown(); c.filesSheet.shown(); c.filesSheet.hidden();
  assert.equal(owners.length, 3, 'sheet instance is lazy and reused');
  assert.equal(owners[2].hooks.preview, c.previewLeftFile);
  assert.notEqual(owners[1].hooks.load, owners[2].hooks.load);
  assert.notEqual(owners[1].hooks.save, owners[2].hooks.save);
  const worktree = fixture({ worktree: '/tree' });
  assert.equal(worktree.owners[1].hooks.preview, undefined);
});
test('toolbar never activates left Files and pane synchronization does not dismiss the sheet', () => {
  const { context: c, events } = fixture();
  assert.equal(c.filesSheet.activate, undefined);
  c.syncFilePreview();
  assert.deepEqual(events, []);
  assert.doesNotMatch(readAssetSource('wsdashboard', 'cards.js'), /owner\.activate/);
});
test('left single click opens pane across board and pin column; durable intent closes before card reveal', () => {
  const { context: c, pane, root, events, search } = fixture();
  const options = { root: '/other', line: 12, answer: { kind: 'text', text: 'preflight' } };
  c.previewLeftFile('a.txt', options);
  assert.equal(c.filePreviewBack, search);
  assert.equal(root.attrs['data-file-preview'], '');
  assert.equal(pane.current().options, options);
  assert.equal(pane.el.style.width, '1304px', 'reaches viewport gutter beyond capped board/grid');
  assert.deepEqual(events, [['pane', 'a.txt', options]]);
  events.length = 0;
  c.previewLeftFile('a.txt', { ...options, keep: true });
  assert.deepEqual(events, ['close', ['card', 'a.txt', { ...options, keep: true }]]);
  assert.equal(pane.current(), null); assert.equal(root.attrs['data-file-preview'], undefined);
});
test('linked file cards reveal the board instead of remaining behind explorer preview A', async () => {
  const { context: c, pane, root, events } = fixture({ constructPane: true });
  const cards = [], kinds = {};
  c.doc.dispatchEvent = () => {};
  c.Event = function () {};
  let boardTab = false;
  c.tabShown = () => boardTab;
  c.setTab = tab => { assert.equal(tab, 'board'); boardTab = true; };
  // Use the exact hook passed to the production card framework.
  const mount = block('    window.WsCards.mount(cardBoard,', '    if (worktree) {');
  const reveal = mount.match(/reveal: (function \(\) \{[\s\S]*?\}),\n      column:/);
  assert.ok(reveal);
  const revealBoard = vm.runInContext('(' + reveal[1] + ')', c);
  Object.assign(c.window.WsCards, {
    register(name, kind) { kinds[name] = kind; }, list: () => cards, relayout() {},
    open(kind, data) {
      const card = { data, el: c.doc.createElement('section'), body: c.doc.createElement('div'),
        fileState: {}, title() {}, actions() {}, paint() {}, save() {}, bump() {}, fold() {},
        show() { revealBoard(); } };
      cards.push(card); kinds[kind].open(card); return card;
    }
  });
  vm.runInContext(readAssetSource('wsdashboard', 'preview.js'), c);
  for (const options of [{ line: 12 }, { out: true }, { keep: true }]) {
    c.previewLeftFile('A.txt', { answer: { kind: 'text', text: 'explorer A' } }); await settle();
    assert.equal(root.hasAttribute('data-file-preview'), true);
    assert.equal(pane.current().path, 'A.txt');
    const card = c.window.WsPreview.open('B.md', { ...options, answer: { kind: 'markdown', html: '<p>linked B</p>' } });
    await settle();
    assert.equal(root.hasAttribute('data-file-preview'), false, 'card B must not be masked by A');
    assert.equal(pane.current(), null);
    assert.equal(pane.el.hidden, true);
    assert.equal(boardTab, true);
    assert.equal(card.data.path, 'B.md');
    assert.equal(card.el.getAttribute('data-preview-kind'), 'markdown');
    assert.equal(events.at(-1), 'cancel-preview', 'retire pending explorer intent too');
  }
  assert.equal(cards.length, 2, 'outside link remains distinct from the root-relative card');
  assert.equal(cards.find(card => !card.data.out).preview, false, 'keep opens a durable root-relative card');
  assert.equal(cards.find(card => card.data.out).preview, true, 'outside link keeps its preview slot');
  revealBoard(); // Already closed is harmless, including when the board tab was already active.
  assert.equal(events.filter(e => e === 'cancel-preview').length, 3);
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
test('leaving split uses actual sanitized pane close and transfers once without cached sandbox answer', async () => {
  const { context: c, pane, events } = fixture({ constructPane: true });
  const answer = { kind: 'frame', frame: '/cap/expired-tabs' };
  const options = { root: '/chosen', line: 5, keep: true, answer };
  pane.open('same.html', options); await settle();
  assert.equal(descendant(pane.el, 'IFRAME').src, answer.frame);
  events.length = 0; c.layoutMode = 'tabs'; c.syncFilePreview(); c.syncFilePreview();
  assert.deepEqual(plain(events), ['close', 'cancel-preview', ['card', 'same.html', { root: '/chosen', line: 5, keep: false }]]);
  const transferred = events.at(-1)[2];
  assert.equal('answer' in transferred, false, 'tabs fallback must revalidate instead of reusing expired capability');
  assert.notEqual(transferred, options); assert.equal(options.answer, answer);
  assert.equal(options.keep, true); assert.equal(pane.current(), null);
});
test('production pane closed callback cancels pending left preflight and restores focus captured before content removal', async () => {
  const { context: c, pane, root, events, search } = fixture({ constructPane: true });
  c.previewLeftFile('focus.txt', { answer: { kind: 'text', text: 'focused' } }); await settle();
  const content = descendant(pane.el, 'PRE'); c.doc.activeElement = content; events.length = 0;
  pane.el.fire('keydown', { key: 'Escape', target: content, preventDefault() {}, stopPropagation() {} });
  assert.deepEqual(events, ['close', 'cancel-preview', 'focus-search']);
  assert.equal(pane.el.contains(content), false, 'focused rendered node really was removed before callback');
  assert.equal(c.doc.activeElement, search); assert.equal(root.hasAttribute('data-file-preview'), false);
  assert.equal(pane.current(), null);
});
test('production closed callback cancels without stealing outside focus or focusing a detached origin', async () => {
  for (const detached of [false, true]) {
    const { context: c, pane, events, search } = fixture({ constructPane: true });
    c.previewLeftFile('a.txt', { answer: { kind: 'text', text: 'a' } }); await settle();
    c.doc.activeElement = detached ? descendant(pane.el, 'PRE') : c.doc.body;
    search.isConnected = !detached; events.length = 0; pane.close();
    assert.deepEqual(events, ['close', 'cancel-preview']);
    assert.equal(c.doc.activeElement, c.doc.body);
  }
});
for (const [width, expectedRight] of [[1920, 1904], [2560, 2544]]) {
  test('preview reaches right ' + expectedRight + ' at viewport ' + width + ' despite capped body', () => {
    const { context: c, pane } = fixture({ width });
    c.window.innerWidth = width + 17; // Use client viewport, not scrollbar-inclusive window width.
    c.previewLeftFile('a.txt', {});
    assert.equal(600 + parseFloat(pane.el.style.width), expectedRight);
    assert.equal(c.dbody.getBoundingClientRect().right, 1664, 'closed body cap remains unchanged');
    assert.equal(c.grid.getBoundingClientRect().right, 1664, 'closed grid is not expanded');
  });
}
test('split resize follows viewport and gutter, not capped grid, without opening cards', () => {
  const { context: c, pane, root, events } = fixture();
  c.previewLeftFile('a.txt', {}); events.length = 0;
  root.clientWidth = 2560; c.syncFilePreview();
  assert.equal(pane.el.style.width, '1944px'); assert.deepEqual(events, []);
  root.clientWidth = 1920;
  c.window.getComputedStyle = () => ({ paddingRight: '24.5px' }); c.syncFilePreview();
  assert.equal(pane.el.style.width, '1295.5px', 'resize rereads computed gutter');
});
test('closed or absent preview does not read geometry or write sizing styles', () => {
  const { context: c, pane, root } = fixture();
  const unexpected = () => { throw Error('closed pane must not touch geometry or styles'); };
  Object.defineProperty(root, 'clientWidth', { get: unexpected });
  c.$('bcol').getBoundingClientRect = unexpected;
  c.grid.closest = unexpected; c.window.getComputedStyle = unexpected;
  pane.el.style = new Proxy({}, { set: unexpected });
  assert.equal(pane.current(), null); c.sizeFilePreview();
  c.filePreview = null; c.sizeFilePreview();
});
test('revealPanes applies layout fallback before lifecycle callbacks and retains shown/hidden semantics', () => {
  const { context: c, pane, events } = fixture();
  Object.assign(c, { PANES: [], PANE: {}, paneVisible: {}, paneScroll: {}, filesSeen: false,
    convo: {}, pinConvo() {}, nearTop() {}, visible: n => n === c.fpane && c.layoutMode !== 'tabs', ctabDot: () => null });
  vm.runInContext(func('revealPanes'), c);
  c.revealPanes(); assert.deepEqual(events, ['shown']);
  events.length = 0; pane.open('a.txt', {}); events.length = 0; c.layoutMode = 'tabs';
  c.revealPanes();
  assert.deepEqual(plain(events), ['close', ['card', 'a.txt', { keep: false }], 'hidden']);
});
function mountSheetFramework(c) {
  const cardsSource = readAssetSource('wsdashboard', 'cards.js');
  Object.assign(c, {
    maxed: null, scrim: null, board: null, col: null, Card: function () {}, wsT: key => key,
    opt: { toolbar: { files: c.filesSheet } }, licon: () => '',
    buildStateFilter: () => c.doc.createElement('div'),
    mk(tag, cls, text) { const n = c.doc.createElement(tag); n.className = cls; if (text) n.textContent = text; return n; },
    ibtn(cls) { const n = c.doc.createElement('button'); n.className = cls; return n; }
  });
  c.doc.getElementById = id => c.$(id);
  for (const name of ['buildToolbar', 'filesColumn', 'sheetWidth', 'clearSheetWidth', 'sizeSheet', 'maximize']) {
    const match = cardsSource.match(new RegExp('\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n'));
    assert.ok(match, name); vm.runInContext(match[0], c);
  }
  c.window.WsCards.dismissFiles = () => c.maximize(null);
  c.buildToolbar();
  return c.toolbar.firstChild.firstChild.children.find(n => n.className === 'board-action board-files');
}
test('folder button docks its own sheet over the complete left column and keeps independent persisted state', () => {
  const { context: c, events, owners } = fixture();
  const saved = [], loaded = [];
  c.load = key => loaded.push(key); c.save = (key, value) => saved.push([key, value]);
  const folder = mountSheetFramework(c);
  folder.fire('click');
  const sheet = c.filesSheet.el;
  assert.equal(sheet.hidden, false); assert.equal(sheet.hasAttribute('data-docked'), true);
  assert.equal(folder.getAttribute('aria-expanded'), 'true');
  assert.equal(sheet.style['--files-dock-left'], '24px');
  assert.equal(sheet.style['--files-dock-top'], '80px');
  assert.equal(sheet.style['--files-dock-width'], '560px');
  assert.equal(sheet.style['--files-dock-height'], '900px');
  owners[1].hooks.load('view'); owners[2].hooks.load('view');
  owners[1].hooks.save('view', 'left'); owners[2].hooks.save('view', 'toolbar');
  assert.deepEqual(loaded, ['view', 'ws.dashboard.toolbar.view']);
  assert.deepEqual(saved, [['view', 'left'], ['ws.dashboard.toolbar.view', 'toolbar']]);
  assert.equal(events.some(e => typeof e === 'string' && e.startsWith('tab:')), false);
  c.chead.getBoundingClientRect = () => ({ left: 48, width: 400 });
  c.grid.getBoundingClientRect = () => ({ top: 90, height: 800 });
  c.sizeFileSurfaces();
  assert.equal(sheet.style['--files-dock-left'], '48px');
  assert.equal(sheet.style['--files-dock-width'], '400px');
  assert.equal(sheet.style['--files-dock-top'], '90px');
  assert.equal(sheet.style['--files-dock-height'], '800px');
  folder.fire('click');
  assert.equal(sheet.hidden, true); assert.equal(sheet.hasAttribute('data-docked'), false);
  assert.equal(folder.getAttribute('aria-expanded'), 'false');
});
test('docked toolbar clicks replace the memory-only preview; scrim click closes both and cancels pending intents', async () => {
  const { context: c, pane, owners, root, events } = fixture({ constructPane: true });
  mountSheetFramework(c).fire('click');
  const toolbar = owners[2];
  toolbar.hooks.preview('a.html', { answer: { kind: 'frame', frame: '/cap/a' } }); await settle();
  assert.equal(descendant(pane.el, 'IFRAME').src, '/cap/a');
  toolbar.hooks.preview('b.txt', { answer: { kind: 'text', text: 'b' } }); await settle();
  assert.equal(pane.current().path, 'b.txt'); assert.equal(descendant(pane.el, 'IFRAME'), null);
  assert.equal(c.filesSheet.el.hidden, false, 'pane reveal retains sheet');
  assert.equal(pane.el.hidden, false);
  pane.el.fire('click');
  assert.equal(c.filesSheet.el.hidden, false, 'pane is not the scrim or its descendant');
  assert.equal(c.scrim.contains(pane.el), false);
  events.length = 0; c.scrim.fire('click');
  assert.equal(c.filesSheet.el.hidden, true); assert.equal(pane.current(), null);
  assert.equal(root.hasAttribute('data-file-preview'), false); assert.equal(c.scrim.hidden, true);
  assert.equal(events.filter(e => e === 'cancel-preview').length, 3, 'sheet dismissal plus both pane owners');
});
test('dashboard capture Escape closes docked sheet with or without preview regardless of focused dashboard element', async () => {
  for (const preview of [false, true]) {
    const { context: c, pane, root } = fixture({ constructPane: true });
    mountSheetFramework(c).fire('click');
    if (preview) { c.previewLeftFile('a.txt', { answer: { kind: 'text', text: 'a' } }); await settle(); }
    assert.equal(c.doc.listenerOptions.keydown, true, 'capture precedes child Escape handlers');
    let prevented = false, stopped = false;
    c.doc.fire('keydown', { key: 'Escape', preventDefault() { prevented = true; }, stopPropagation() { stopped = true; } });
    assert.equal(prevented && stopped, true);
    assert.equal(c.filesSheet.el.hidden, true); assert.equal(pane.current(), null);
    assert.equal(root.hasAttribute('data-file-preview'), false);
  }
});
test('tabs and narrow sheets keep dropdown sizing and standard card preview; worktree hooks remain unchanged', () => {
  for (const options of [{ layout: 'tabs' }, { layout: 'auto', width: 1099 }, { layout: 'split', width: 759 }, { worktree: '/tree' }]) {
    const { context: c, owners, events } = fixture(options);
    mountSheetFramework(c).fire('click');
    assert.equal(c.filesSheet.el.hasAttribute('data-docked'), false);
    assert.equal(c.filesSheet.el.style['--files-dock-width'], undefined);
    if (!options.worktree) {
      owners[2].hooks.preview('a.txt', {});
      assert.deepEqual(events.at(-1), ['card', 'a.txt', {}]);
    } else assert.equal(owners[2].hooks.preview, undefined);
    c.doc.fire('keydown', { key: 'Escape', preventDefault() { throw Error('not docked'); } });
  }
});
test('leaving split undocks the open sheet without dismissing it and transfers preview to a card', () => {
  const { context: c, pane, events } = fixture();
  mountSheetFramework(c).fire('click'); c.previewLeftFile('a.txt', {});
  events.length = 0; c.layoutMode = 'tabs'; c.syncFilePreview();
  assert.equal(c.filesSheet.el.hasAttribute('data-docked'), false);
  assert.equal(c.filesSheet.el.hidden, false); assert.equal(pane.current(), null);
  assert.deepEqual(plain(events), ['close', ['card', 'a.txt', { keep: false }]]);
});
test('pane keep and toolbar durable intent close both surfaces before revealing a durable card', async () => {
  for (const intent of ['keep-button', 'durable-row']) {
    const { context: c, pane, owners } = fixture({ constructPane: true });
    mountSheetFramework(c).fire('click');
    c.tabShown = () => true;
    const reveal = block('    window.WsCards.mount(cardBoard,', '    if (worktree) {')
      .match(/reveal: (function \(\) \{[\s\S]*?\}),\n      column:/);
    const revealBoard = vm.runInContext('(' + reveal[1] + ')', c);
    let opened;
    c.window.WsPreview.open = (path, options) => { opened = { path, options }; revealBoard(); };
    owners[2].hooks.preview('a.txt', { answer: { kind: 'text', text: 'a' } }); await settle();
    if (intent === 'keep-button') pane.el.firstChild.children[2].fire('click');
    else owners[2].hooks.preview('a.txt', { keep: true });
    assert.equal(opened.path, 'a.txt'); assert.equal(opened.options.keep, true);
    assert.equal(opened.options.answer, undefined, 'promotion revalidates capabilities');
    assert.equal(c.filesSheet.el.hidden, true); assert.equal(pane.current(), null);
    assert.equal(c.scrim.hidden, true);
  }
});
test('docked geometry overrides dropdown insets and pane paints above the scrim', () => {
  const css = readAssetSource('wsdashboard', 'dashboard.css');
  assert.match(css, /#toolbar-files\[data-max\]\[data-docked\] \{[^}]*top: var\(--files-dock-top\);[^}]*left: var\(--files-dock-left\);[^}]*right: auto; bottom: auto;[^}]*width: var\(--files-dock-width\); height: var\(--files-dock-height\);/);
  assert.match(css, /body:has\(#toolbar-files\[data-max\]\[data-docked\]\) \.file-preview-pane \{ z-index: var\(--z-modal\);/);
  assert.match(css, /body:has\(#toolbar-files\[data-max\]\[data-docked\]\) \.board-toolbar \.sheet-width \{ display: none;/);
});
test('pane CSS preserves board/pin geometry and hides overlay while closed', () => {
  const css = readAssetSource('wsdashboard', 'dashboard.css');
  assert.match(css, /\.file-preview-pane\[hidden\]\s*\{\s*display:\s*none;/);
  const rule = css.match(/:root\[data-file-preview\] \.dgrid\.live > \.bcol,[^}]+\}/);
  assert.ok(rule, 'missing preview-only board/pin hiding rule');
  for (const selector of ['.bcol', '.dcv-edge', '.bpcol', '.bpzone']) assert.ok(rule[0].includes(selector));
  assert.match(rule[0], /visibility:\s*hidden/); assert.doesNotMatch(rule[0], /display:\s*none/);
});
