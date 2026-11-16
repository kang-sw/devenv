const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function asset(name) {
  return fs.readFileSync(path.join(__dirname, 'assets', name), 'utf8');
}

function section(source, start, end) {
  const a = source.indexOf(start), b = source.indexOf(end, a);
  assert.ok(a >= 0 && b > a, 'asset section exists');
  return source.slice(a, b);
}

test('drawer fill coalesces and reads geometry before room and span writes', () => {
  const source = section(asset('dashboard-drawer.js'), '  var drFilled =', '  function drDot()');
  const frames = [], events = [];
  const cards = Array.from({ length: 5 }, () => ({
    children: [],
    style: { set gridRow(value) { events.push('span:' + value); } }
  }));
  const body = { children: cards, hidden: false };
  const context = {
    drbody: body,
    requestAnimationFrame(fn) { frames.push(fn); return frames.length; },
    getComputedStyle(el) {
      events.push('read');
      return el === body
        ? { display: 'grid', gridTemplateColumns: '100px 100px 100px' }
        : { display: 'block', gridRowStart: 'auto', gridRowEnd: 'auto', gridColumnStart: 'auto', gridColumnEnd: 'auto' };
    },
    drRoom() { events.push('room-write'); }
  };
  vm.createContext(context);
  vm.runInContext(source, context);
  context.drFill(true);
  context.drFill(true);
  assert.equal(frames.length, 1);
  frames.shift()();
  assert.equal(events.filter(e => e === 'room-write').length, 1);
  assert.equal(events.filter(e => e === 'span:span 2').length, 1);
  assert.ok(events.lastIndexOf('read') < events.indexOf('room-write'));
  assert.ok(events.indexOf('room-write') < events.indexOf('span:span 2'));

  events.length = 0;
  body.hidden = true;
  context.drFill();
  assert.deepEqual(events, ['span:']);
  frames.shift()();
  assert.equal(events.filter(e => e === 'span:span 2').length, 0);
});

test('lifecycle post coalesces on rAF and reads layout before lifting the frame', () => {
  const source = section(asset('dashboard-lifecycle.js'), '    function post() {', '    doc.addEventListener("ws-overview", queue);');
  const frames = [], events = [], messages = [];
  const top = { offsetHeight: 42 }, head = { innerHTML: 'header' }, qs = {};
  const root = {
    get clientWidth() { events.push('read:width'); return 990; },
    hasAttribute() { events.push('read:pincol'); return true; },
    setAttribute() { events.push('write:lifted'); },
    removeAttribute() { events.push('write:lifted'); }
  };
  const context = {
    queued: null, root,
    doc: { querySelector() { return top; } },
    region() { return head; },
    $(id) { return id === 'qsslot' ? qs : null; },
    inline() { return 'search'; },
    window: { innerWidth: 1000 },
    getComputedStyle() { events.push('read:style'); return { paddingRight: '5px' }; },
    q() { return [{ classList: { contains() { return false; } }, getClientRects() { events.push('read:rects'); return [1]; } }]; },
    dim() { events.push('read:dim'); return 0.5; },
    send(kind, value) { messages.push([kind, value]); },
    requestAnimationFrame(fn) { frames.push(fn); return frames.length; }
  };
  vm.createContext(context);
  vm.runInContext(source, context);
  context.queue(); context.queue();
  assert.equal(frames.length, 1);
  assert.deepEqual(events, []);
  frames.shift()();
  assert.ok(events.includes('read:dim'));
  assert.ok(events.lastIndexOf('read:dim') < events.indexOf('write:lifted'));
  assert.ok(events.filter(e => e.startsWith('read:')).every(e => events.indexOf(e) < events.indexOf('write:lifted')));
  assert.equal(messages[0][0], 'head');
  assert.equal(messages[0][1].pad, 15);
  assert.equal(messages[1][0], 'layout');
  assert.equal(messages[1][1].dim, 0.5);
  assert.equal(messages[1][1].popover, true);
  context.queue();
  assert.equal(frames.length, 1);
});
