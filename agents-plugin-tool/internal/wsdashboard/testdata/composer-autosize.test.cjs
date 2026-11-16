// Deterministic layout-access contract, not a browser/layout acceptance test.
const assert = require('node:assert/strict');
const vm = require('node:vm');
const { readAssetSource } = require('./asset-source.cjs');
const source = readAssetSource('wsdashboard', 'dashboard.js');
const start = source.indexOf('  var cmeasure =');
const end = source.indexOf('  // post sends', start);
assert(start >= 0 && end > start, 'production autosize setup exists');
const sizing = source.slice(start, end);
const input = source.match(/ctext\.addEventListener\("input", function \(\) \{ grow\(\);[^\n]+/);
assert(input, 'production input owner exists');

function harness({ capsules = false, native = false } = {}) {
  let reads = 0, writes = [], measured = 55, made = 0, frameId = 0;
  const frames = [], listeners = {}, observers = [], fontReady = [];
  const calls = { markers: 0, track: 0, paint: 0 };
  const style = {};
  Object.defineProperty(style, 'height', {
    get() { return this._height || ''; },
    set(value) { assert.notEqual(value, 'auto', 'never reset visible height'); writes.push(value); this._height = value; }
  });
  function forbidden() { throw new Error('visible-field layout read in input path'); }
  const editor = { style };
  for (const name of ['scrollHeight', 'offsetHeight', 'offsetWidth']) Object.defineProperty(editor, name, { get: forbidden });
  const field = { style, value: '', ph: 'wrapped placeholder', offsetWidth: 100,
    getAttribute() { return this.ph; },
    addEventListener(type, fn) { listeners[type] = fn; },
    parentNode: { appendChild(el) { field.measure = el; } }
  };
  Object.defineProperty(field, 'scrollHeight', { get: forbidden });
  if (capsules) field.wsc = { editor };
  const context = { ctext: field, chl: null,
    doc: { createElement() { made++; return { style: {}, textContent: '', setAttribute() {}, get offsetHeight() { reads++; return measured; } }; },
      fonts: { ready: { then(fn) { fontReady.push(fn); } } } },
    ResizeObserver: class { constructor(fn) { observers.push(fn); } observe(el) { assert.equal(el, field); } },
    requestAnimationFrame(fn) { frames.push(fn); return ++frameId; },
    syncMarkers() { calls.markers++; }, trackMentions() { calls.track++; }, paintMentions() { calls.paint++; }
  };
  if (native) context.CSS = { supports(property, value) { assert.equal(property, 'field-sizing'); assert.equal(value, 'content'); return true; } };
  context.window = context;
  vm.runInNewContext(sizing + '\n' + input[0], context);
  return { field, calls, writes, observers, fontReady,
    input() { listeners.input(); }, grow() { context.grow(); },
    frame() { assert.equal(frames.length, 1, 'one pending sizing callback'); frames.shift()(); },
    set measured(value) { measured = value; },
    get reads() { return reads; }, get pending() { return frames.length; }, get made() { return made; }
  };
}

// Native input, IME bursts, and programmatic grow calls all stay layout-free.
for (const mode of [{ capsules: true }, { native: true }]) {
  const h = harness(mode);
  for (let i = 0; i < 30; i++) { h.input(); h.grow(); }
  assert.deepEqual(h.calls, { markers: 30, track: 30, paint: 30 }, 'other input work still runs once');
  assert.equal(h.pending, 0, 'CSS-sized editors never schedule measurements');
  assert.equal(h.reads, 0);
  assert.deepEqual(h.writes, []);
  assert.equal(h.made, 0);
  assert.equal(h.observers.length, 0, 'no width/height observer on CSS-sized fields');
  assert.equal(h.fontReady.length, 0, 'no font-ready layout measurement on CSS-sized fields');
}

const h = harness();
for (let i = 0; i < 30; i++) h.input();
assert.equal(h.pending, 1, 'initial/IME/input requests coalesce');
assert.equal(h.reads, 0, 'legacy input never measures synchronously');
assert.deepEqual(h.writes, [], 'legacy input never writes height synchronously');
h.frame();
assert.equal(h.reads, 1);
assert.deepEqual(h.writes, ['55px']);
assert.equal(h.field.measure.className, 'chl chm');
assert.equal(h.field.measure.textContent, 'wrapped placeholder');
h.input(); h.frame();
assert.deepEqual(h.writes, ['55px'], 'unchanged height does not dirty the visible field');
assert.equal(h.made, 1, 'reuse measuring layer');
h.measured = 500; h.input(); h.frame();
assert.equal(h.writes.at(-1), '160px', 'cap long content');
h.measured = 20; h.field.value = 'x'; h.input(); h.frame();
assert.equal(h.writes.at(-1), '38px', 'shrink after delete, retaining minimum');
assert.equal(h.field.measure.textContent, 'x', 'nonempty text ignores placeholder');
h.measured = 94; h.field.value = 'a\n'; h.input(); h.frame();
assert.equal(h.writes.at(-1), '94px');
assert.equal(h.field.measure.textContent, 'a\n\u200b', 'trailing newline retains its final line');
h.field.value = ''; h.field.ph = ''; h.input(); h.frame();
assert.equal(h.field.measure.textContent, '\u200b', 'empty field measures a line');
h.observers[0](); h.frame();
h.observers[0](); assert.equal(h.pending, 0, 'height-only resize cannot loop');
h.field.offsetWidth = 200; h.observers[0](); h.frame();
h.fontReady[0](); h.frame();

const css = readAssetSource('wsdashboard', 'dashboard.css');
assert.match(css, /#ctext\s*\{\s*field-sizing:\s*content;/);
assert.match(css, /\.composer textarea, \.composer textarea \+ \.wsc-editor \{[^}]*min-height: 38px; max-height: 160px;/);
assert.match(css, /\.composer textarea \+ \.wsc-editor \{[^}]*box-sizing: border-box; overflow: auto;/);
console.log('composer autosize: layout-free CSS paths, coalesced legacy measurement, unchanged-height guard, min/max, placeholder, newline, width/fonts passed');
