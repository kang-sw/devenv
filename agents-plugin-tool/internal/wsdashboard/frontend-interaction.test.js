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

test('native and shell visibility pause animations and notify clocks idempotently', () => {
  const attrs = new Set(), events = [], context = {
    shHidden: false, doc: { hidden: false, addEventListener() {}, dispatchEvent(e) { events.push(e.type); } },
    root: { hasAttribute: key => attrs.has(key), toggleAttribute(key, value) { if (value) attrs.add(key); else attrs.delete(key); } },
    CustomEvent: class { constructor(type) { this.type = type; } }
  };
  vm.createContext(context);
  vm.runInContext(section(read(__dirname + '/assets/dashboard.js'), '  function syncPageVisibility()', '  // --- browser storage:'), context);
  context.shHidden = true; context.syncPageVisibility(); context.syncPageVisibility();
  assert.equal(attrs.has('data-animation-paused'), true); assert.equal(events.length, 1);
  context.shHidden = false; context.doc.hidden = true; context.syncPageVisibility();
  assert.equal(events.length, 1);
  context.doc.hidden = false; context.syncPageVisibility();
  assert.equal(attrs.has('data-animation-paused'), false); assert.equal(events.length, 2);
  assert.match(read(__dirname + '/assets/dashboard-lifecycle.js'), /shHidden = !m\.shown;\s*syncPageVisibility\(\);/);
});
