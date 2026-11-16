const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const wb = __dirname + '/../wswhiteboard/';
const read = file => fs.readFileSync(file, 'utf8');
function fixture(file, name) { return read(wb + file).split('const ' + name + ' = `')[1].split('`')[0]; }
function section(source, start, end) { return source.slice(source.indexOf(start), source.indexOf(end, source.indexOf(start))); }

test('pill IO/draft work coalesces and identical counts write nothing', () => {
  const context = { console }; vm.createContext(context);
  const model = fixture('pill_segments_test.go', 'pillSegPrelude');
  const instrumentation = `
var writes=0;
Object.defineProperty(E.prototype,'textContent',{get:function(){return this._text||'';},set:function(v){writes++;this._text=v;}});
Object.defineProperty(E.prototype,'hidden',{get:function(){return !!this._hidden;},set:function(v){writes++;this._hidden=v;}});
var setAttr=E.prototype.setAttribute;E.prototype.setAttribute=function(k,v){writes++;setAttr.call(this,k,v);};
`;
  vm.runInContext(model + instrumentation + read(wb + 'assets/pill.js') + '\nfire([]); writes=0; window.wbPill.update();window.wbPill.update();window.wbPill.update();', context);
  assert.equal(context.frames.length, 1);
  assert.equal(context.writes, 0);
  vm.runInContext('frame();', context);
  assert.equal(context.writes, 0);
  const contract = read(wb + 'pill_segments_test.go').split('script := pillSegPrelude + string(data) + `')[1].split('`')[0];
  context.process = { stdout: { write(value) { assert.equal(value, ''); } } };
  vm.runInContext(contract, context);
});

test('rail ghosts preserve scroll-only transitions with transform/opacity keyframes', () => {
  const contract = read(wb + 'rail_motion_test.go');
  const setup = contract.split('script := railPrelude + `')[1].split('` + string(data) + `')[0].replace('anims.push(this);', 'anims.push(this); keyframes.push(k);');
  const assertions = contract.split('` + string(data) + `')[1].split('`')[0];
  const context = { process: { stdout: { write(value) { assert.equal(value, ''); } } } };
  vm.createContext(context);
  vm.runInContext(fixture('rail_offsets_test.go', 'railPrelude') + '\nvar keyframes=[];\n' + setup + read(wb + 'assets/rail.js') + assertions, context);
  assert.ok(context.keyframes.length > 0);
  for (const frames of context.keyframes) for (const frame of frames) {
    assert.ok(Object.keys(frame).every(key => ['transform', 'opacity', 'offset'].includes(key)), JSON.stringify(frame));
  }
});

test('cards toolbar scrolls coalesce, reuse scroller, and read before writing', () => {
  const frames = [], handlers = {}, order = [], resize = [];
  const style = () => { const values = {}; return { getPropertyValue(k) { return values[k] || ''; }, setProperty(k,v) { order.push('write'); values[k] = v; }, removeProperty(k) { delete values[k]; } }; };
  const rect = (top, height, width = 300) => ({ top, height, width, bottom: top + height });
  const face = { getBoundingClientRect() { order.push('read'); return rect(0, 46); }, firstChild: { children: [{ getBoundingClientRect() { order.push('read'); return rect(0, 30); } }] } };
  const attrs = new Set(), toolbar = { firstChild: face, style: style(), hasAttribute: k => attrs.has(k), toggleAttribute(k,v) { order.push('write'); if (v) attrs.add(k); else attrs.delete(k); } };
  const board = { style: style(), getBoundingClientRect() { order.push('read'); return rect(-20, 1000); } };
  const scroller = { style: style() }; let walks = 0;
  const context = { toolbar, board, maxed: null,
    doc: { scrollingElement: scroller, querySelectorAll() { return []; }, getElementById() { return board; }, addEventListener(type, fn) { handlers[type] = fn; } },
    window: { addEventListener() {}, ResizeObserver: class { constructor(fn) { resize.push(fn); } observe() {} } },
    scroller() { walks++; return scroller; }, requestAnimationFrame(fn) { frames.push(fn); return frames.length; }
  };
  vm.createContext(context);
  vm.runInContext(section(read(__dirname + '/assets/cards.js'), '  function floatToolbar()', '  function filterMark(') + '\nfloatToolbar();', context);
  assert.equal(frames.length, 1); frames.shift()();
  assert.ok(order.lastIndexOf('read') < order.indexOf('write'));
  order.length = 0;
  handlers.scroll(); handlers.scroll(); handlers.scroll();
  assert.equal(frames.length, 1); frames.shift()();
  assert.equal(walks, 1);
  resize[0](); frames.shift()(); assert.equal(walks, 2);
});

test('pane scroll bookkeeping uses observer visibility instead of geometry probes', () => {
  const frames = [], handlers = {}, order = []; let observer;
  const pane = { id: 'convo', scrollTop: 80, addEventListener(type, fn) { handlers[type] = fn; }, getClientRects() { throw Error('visibility geometry forbidden'); } };
  const context = { PANES: ['convo'], window: { ResizeObserver: true },
    ResizeObserver: class { constructor(fn) { observer = fn; } observe() {} },
    $(id) { return pane; }, doc: {}, visible() { throw Error('live visibility probe forbidden'); },
    requestAnimationFrame(fn) { frames.push(fn); return frames.length; }
  };
  vm.createContext(context);
  vm.runInContext(section(read(__dirname + '/assets/dashboard.js'), '  var paneScroll =', '  // Restore after'), context);
  observer([{ target: pane, contentRect: { width: 300, height: 200 } }]);
  handlers.scroll(); handlers.scroll(); assert.equal(frames.length, 1); frames.shift()();
  assert.equal(context.paneScroll.convo, 80);
  assert.equal(context.scrollState()[0][1], 80);
  observer([{ target: pane, contentRect: { width: 0, height: 0 } }]);
  pane.scrollTop = 0; handlers.scroll(); frames.shift()();
  assert.equal(context.paneScroll.convo, 80);
  assert.equal(context.scrollState().length, 0);
});

test('native and shell visibility keep pause state and notify each lifecycle transition', () => {
  const attrs = new Set(), events = [], context = {
    shHidden: false, doc: { hidden: false, addEventListener() {}, dispatchEvent(e) { events.push(e.type); } },
    root: { hasAttribute: key => attrs.has(key), toggleAttribute(key, value) { if (value) attrs.add(key); else attrs.delete(key); } },
    CustomEvent: class { constructor(type) { this.type = type; } }
  };
  vm.createContext(context);
  vm.runInContext(section(read(__dirname + '/assets/dashboard.js'), '  function syncPageVisibility()', '  // --- browser storage:'), context);
  context.shHidden = true; context.syncPageVisibility(); context.syncPageVisibility();
  assert.equal(attrs.has('data-animation-paused'), true); assert.equal(events.length, 3);
  context.shHidden = false; context.doc.hidden = true; context.syncPageVisibility();
  assert.equal(events.length, 4, 'shell state changes must notify streams even while the native tab stays hidden');
  context.doc.hidden = false; context.syncPageVisibility();
  assert.equal(attrs.has('data-animation-paused'), false); assert.equal(events.length, 5);
});

// A fake chat block for the msgacts handler: a batch entry holding two
// owner bubbles, each with a pill (rewind disabled, copy).
function chatBlocks() {
  class El {
    constructor(cls, attrs, parent) { this.cls = cls; this.attrs = Object.assign({}, attrs); this.parentNode = parent || null; this.kids = []; if (parent) parent.kids.push(this); }
    getAttribute(k) { return k in this.attrs ? this.attrs[k] : null; }
    setAttribute(k, v) { this.attrs[k] = String(v); }
    removeAttribute(k) { delete this.attrs[k]; }
    hasAttribute(k) { return k in this.attrs; }
    matches(sel) { return sel === '[data-entry]' ? this.hasAttribute('data-entry') : sel.split('.').filter(Boolean).every(c => this.cls.includes(c)); }
    closest(sel) { for (let n = this; n; n = n.parentNode) if (n.matches(sel)) return n; return null; }
    all(sel) { const out = []; for (const k of this.kids) { if (k.matches(sel)) out.push(k); out.push(...k.all(sel)); } return out; }
    querySelectorAll(sel) { return this.all(sel); }
    contains(n) { for (; n; n = n.parentNode) if (n === this) return true; return false; }
  }
  const box = new El(['conv', 'centries'], {});
  const batch = new El(['user-batch'], { 'data-entry': 'K1.json' }, box);
  const bubble = () => {
    const msg = new El(['msg', 'user'], {}, batch), bar = new El(['d-acts'], {}, msg);
    return { msg, bar, rewind: new El(['d-act'], { 'data-act': 'rewind', 'aria-disabled': 'true' }, bar), copy: new El(['d-act'], { 'data-act': 'copy' }, bar) };
  };
  return { box, first: bubble(), second: bubble() };
}

function chatActions(answer, extra) {
  const blocks = chatBlocks(), fetched = [], copied = [], toasts = [], timers = [];
  let handler = null;
  const context = Object.assign({
    rid: 'sess', Promise, Array, Blob: class { constructor(parts) { this.text = parts.join(''); } },
    window: { fetch: true }, navigator: {},
    doc: { addEventListener(type, fn) { if (type === 'click') handler = fn; } },
    entriesBox: () => blocks.box,
    fetch(url) { fetched.push(url); return answer instanceof Error ? Promise.reject(answer) : Promise.resolve({ ok: true, json: () => Promise.resolve(answer) }); },
    copyText(s, quiet) { copied.push([s, quiet]); return Promise.resolve(true); },
    toast(s) { toasts.push(s); }, wsT: k => k,
    setTimeout(fn) { timers.push(fn); return timers.length; }, clearTimeout() {}
  }, extra);
  vm.createContext(context);
  vm.runInContext(section(read(__dirname + '/assets/dashboard-conversation.js'), '  var ACT_CONFIRM_MS', '  // --- older history:'), context);
  const click = target => { const e = { target, prevented: false, preventDefault() { this.prevented = true; } }; handler(e); return e; };
  const settle = () => new Promise(r => setImmediate(r));
  return { blocks, fetched, copied, toasts, timers, click, settle };
}

test('chat block copy copies the fetched text of its own bubble, quietly, then confirms', async () => {
  const t = chatActions({ blocks: ['first raw', 'second **raw**'], truncated: false });
  assert.equal(t.click(t.blocks.second.copy).prevented, true);
  await t.settle();
  assert.deepEqual(t.fetched, ['/s/sess/entry?key=K1.json']);
  assert.deepEqual(t.copied, [['second **raw**', true]]);
  assert.equal(t.blocks.second.copy.hasAttribute('data-done'), true);
  assert.equal(t.blocks.second.bar.hasAttribute('data-pinned'), true);
  assert.equal(t.toasts.length, 0);
  t.timers.pop()();
  assert.equal(t.blocks.second.copy.hasAttribute('data-done'), false);
  assert.equal(t.blocks.second.bar.hasAttribute('data-pinned'), false);
});

test('chat block copy writes a pending clipboard item inside the click when the browser takes one', async () => {
  const written = [];
  const t = chatActions({ blocks: ['only'], truncated: false }, {
    window: { fetch: true, ClipboardItem: true, isSecureContext: true },
    ClipboardItem: class { constructor(parts) { this.parts = parts; } },
    navigator: { clipboard: { write(items) { written.push(items[0].parts['text/plain']); return Promise.resolve(); } } }
  });
  t.click(t.blocks.first.copy);
  assert.equal(written.length, 1, 'the write must start inside the click');
  assert.equal((await written[0]).text, 'only');
  await t.settle();
  assert.deepEqual(t.copied, []);
  assert.equal(t.blocks.first.copy.hasAttribute('data-done'), true);
});

test('chat block disabled actions only show their tooltip, and a failed fetch toasts', async () => {
  const t = chatActions(new Error('daemon gone'));
  t.click(t.blocks.first.rewind);
  assert.equal(t.blocks.first.rewind.hasAttribute('data-tipshow'), true);
  assert.deepEqual(t.fetched, []);
  t.click(t.blocks.first.copy);
  await t.settle();
  assert.deepEqual(t.copied, []);
  assert.deepEqual(t.toasts, ['js.entry.copyfail']);
  assert.equal(t.blocks.first.copy.hasAttribute('data-done'), false);
});
