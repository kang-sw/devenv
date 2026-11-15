package wswhiteboard

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

func TestBodyTimeAgeAcrossMinuteBoundaryUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; advancing-clock body-age regression not executed")
	}
	ago, err := embeddedAsset("ago.js")
	if err != nil {
		t.Fatal(err)
	}
	chg, err := embeddedAsset("chg.js")
	if err != nil {
		t.Fatal(err)
	}
	source, err := json.Marshal(string(wsi18n.SubsetScript("ko", frameKeyPrefix)) + "\n" + string(ago) + "\n" + string(chg))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "-")
	cmd.Stdin = strings.NewReader("const shipped = " + string(source) + ";\n" + bodyTimeMinuteBoundaryCases)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("advancing-clock body age: %v\n%s", err, out)
	}
}

const bodyTimeMinuteBoundaryCases = `
const assert = require('node:assert/strict'), vm = require('node:vm');
const at = Date.parse('2026-11-14T13:00:00Z'), stamp = new Date(at).toISOString();
let now = at + 359000, readStep = 0, reads = 0;
function make(framed) {
  class E {
    constructor(cls = '') { this.className = cls; this.attrs = {}; this.style = {}; this.children = []; this.events = {}; this.hidden = false; this._text = ''; }
    get classList() { return { contains: c => this.className.split(' ').includes(c) }; }
    set textContent(v) { this._text = v; this.children = []; }
    get textContent() { return this._text; }
    setAttribute(k, v) { this.attrs[k] = String(v); }
    getAttribute(k) { return this.attrs[k] ?? null; }
    appendChild(e) { this.children.push(e); e.parentNode = this; return e; }
    insertBefore(e) { this.appendChild(e); body.nextElementSibling = e; }
    addEventListener(k, f) { this.events[k] = f; }
    remove() { this.removed = true; }
    closest() { return this.topic || null; }
    querySelector() { return null; }
    getBoundingClientRect() { return { left: 0, top: 0, right: 300, bottom: 20, width: 300, height: 20 }; }
  }
  const env = { timers: [], listeners: {}, frames: [] };
  const topic = { querySelector: sel => sel === '.tdoc iframe.doc-frame' ? env.frames[0] || null : env.header };
  env.replaceHeader = () => { env.header = new E('body-age'); env.header.topic = topic; env.header.setAttribute('data-body-at', stamp); };
  env.replaceHeader();
  const key = new E(), meta = new E(); key.setAttribute('content', 'topic:body'); meta.setAttribute('data-body-at', stamp);
  const body = new E('doc-body'), parent = new E(), block = new E();
  body.parentNode = parent; body.topic = framed ? null : topic;
  const doc = {
    body: new E(),
    querySelector: sel => sel === 'meta[name="wb-doc-key"]' ? (framed ? key : null) : meta,
    querySelectorAll: sel => sel === '.tdoc iframe.doc-frame' ? env.frames : sel === '[data-body-age]' ?
      [...(framed ? [] : [env.header]), ...doc.body.children.filter(e => e.attrs['data-body-age'] !== undefined)] : [],
    createElement: () => new E(),
    createTreeWalker: () => { let used = false; return { nextNode: () => used ? null : (used = true, { data: 'wbchg:new', nextElementSibling: block }) }; }
  };
  const s = { document: doc, NodeFilter: { SHOW_COMMENT: 128 }, console,
    innerWidth: 360, innerHeight: 900, readClock: () => now + readStep * reads++,
    getComputedStyle: () => ({ paddingLeft: '14px' }),
    setInterval: fn => { env.timers.push(fn); return env.timers.length; },
    addEventListener: (name, fn) => { (env.listeners[name] ||= []).push(fn); }
  };
  s.window = s; s.parent = s;
  env.s = s; env.doc = doc; env.body = body; env.meta = meta;
  env.load = () => { vm.createContext(s); vm.runInContext('Date.now = readClock;', s); vm.runInContext(shipped, s); s.wbWatchChanges(body); };
  env.dispatch = (name, event) => (env.listeners[name] || []).forEach(fn => fn(event));
  env.enter = () => body.nextElementSibling.children[0].events.pointerenter();
  env.leave = () => body.nextElementSibling.children[0].events.pointerleave();
  env.tip = () => body.nextElementSibling.wbTip;
  env.frameElement = () => { const f = new E('doc-frame'); f.topic = topic; f.setAttribute('data-doc-key', 'topic:body'); f.setAttribute('data-doc', 'topic:body'); return f; };
  return env;
}
const host = make(false); host.load();
assert.equal(host.header.textContent, '본문 5분 전 갱신');
now += 2000; // 5m59s -> 6m01s, without the header's periodic timer.
host.enter();
assert.equal(host.header.textContent, '본문 6분 전 갱신');
assert.equal(host.tip().textContent, '본문 6분 전 갱신');
assert.equal(host.tip().hidden, false);
now += 60000; host.timers[0]();
assert.equal(host.header.textContent, '본문 7분 전 갱신');
assert.equal(host.tip().textContent, '본문 7분 전 갱신', 'continued visible hover follows the same tick');
// A tick itself may straddle a boundary: all labels must use one sample.
now = at + 480000 - 1; readStep = 2; reads = 0; host.timers[0]();
assert.equal(host.header.textContent, '본문 7분 전 갱신');
assert.equal(host.tip().textContent, '본문 7분 전 갱신');
readStep = 0; host.leave();

// Independent VM globals and queued messages model the opaque-origin boundary.
// No formatter, pointer handler or timer callback is mocked away.
now = at + 359000; host.timers[0]();
const child = make(true), queue = [];
const parentWire = { postMessage: data => queue.push({ dest: host, source: childWire, data }) };
const childWire = { postMessage: data => queue.push({ dest: child, source: parentWire, data }) };
Object.defineProperty(parentWire, 'document', { get() { throw Error('opaque frame must not access parent DOM'); } });
child.s.parent = parentWire;
const frame = host.frameElement(); frame.contentWindow = childWire; host.frames = [frame];
child.load();
function deliver() { const m = queue.shift(); assert.ok(m, 'expected queued message'); m.dest.dispatch('message', { source: m.source, data: m.data }); }
function flush() { let left = 30; while (queue.length) { assert.ok(left-- > 0, 'bounded message exchange'); deliver(); } }
function both(want) { assert.equal(host.header.textContent, want); assert.equal(child.tip().textContent, want); assert.equal(child.tip().hidden, false); }
now += 2000; child.enter();
assert.equal(child.tip().hidden, true, 'wait hidden for the host display sample');
const request = queue[0].data;
host.dispatch('message', { source: { postMessage() { throw Error('unowned sender must be rejected'); } }, data: request });
host.dispatch('message', { source: childWire, data: { ...request, docKey: 'wrong' } });
assert.equal(frame.wbBodyAgeHold, undefined, 'source and document key both bind a request');
deliver(); // The host pins six minutes, but the response is still queued.
assert.equal(host.header.textContent, '본문 6분 전 갱신');
child.dispatch('message', { source: {}, data: queue[0].data });
child.dispatch('message', { source: parentWire, data: { ...queue[0].data, docKey: 'wrong' } });
assert.equal(child.tip().hidden, true, 'untrusted replies cannot reveal the tooltip');
now += 60000; child.timers[0](); host.timers[0]();
deliver(); both('본문 6분 전 갱신');
now += 60000; host.timers[0](); child.timers[0](); both('본문 6분 전 갱신');
// A frame graft keeps the iframe but replaces its surrounding header.
host.replaceHeader(); host.timers[0](); both('본문 6분 전 갱신');
child.leave();
assert.equal(child.tip().hidden, true, 'hide before the host is allowed to advance');
host.timers[0](); assert.equal(host.header.textContent, '본문 6분 전 갱신');
flush(); assert.equal(host.header.textContent, '본문 8분 전 갱신');
assert.equal(frame.wbBodyAgeHold, undefined);
// A delayed response must not resurrect a cancelled hover.
child.enter(); deliver(); child.leave(); flush(); assert.equal(child.tip().hidden, true);
// A redraw cancels the visible hover too, not only pointerleave.
child.enter(); flush(); both('본문 8분 전 갱신');
now += 60000; child.s.wbChangeBars(child.body);
assert.equal(child.tip().hidden, true); flush();
assert.equal(host.header.textContent, '본문 9분 전 갱신');
assert.equal(frame.wbBodyAgeHold, undefined);
// Unknown remains exact through the same exchange and independent timers.
host.header.setAttribute('data-body-at', ''); child.meta.setAttribute('data-body-at', '');
child.enter(); flush(); both('변경 시각 모름');
now += 60000; host.timers[0](); child.timers[0](); both('변경 시각 모름');
child.dispatch('pagehide'); flush();
assert.equal(child.tip().hidden, true); assert.equal(frame.wbBodyAgeHold, undefined);
assert.equal(host.timers.length, 1); assert.equal(child.timers.length, 1);
console.log('advancing-clock body age: shared tick, opaque hover hold/release, timer order and cancellation passed');
`
