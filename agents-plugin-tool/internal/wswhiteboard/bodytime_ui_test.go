package wswhiteboard

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsi18n"
)

// This executes the shipped JS, with only browser primitives stubbed. Actual
// hit testing, CSS geometry and opaque frames are covered by the finite probe.
func TestBodyTimeUIBehaviorUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; body-time JS cases not executed")
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
	cmd := exec.CommandContext(ctx, node, "-e", bodyTimeNodePrelude+"\nconst shipped = "+string(source)+";\n"+bodyTimeNodeCases)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("body-time JS: %v\n%s", err, out)
	}
}

// Exercise early browser-command exits without launching any subprocess from
// the Python runner. An absent/partial/launching owner is unknown, not clean;
// cleanup must preserve the command failure and never signal an unverified PID.
func TestBodyTimeVerificationRunnerEarlyExitCleanup(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed; finite-runner regression not executed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", bodyTimeRunnerCases)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("body-time finite runner: %v\n%s", err, out)
	}
}

// This imports the prepared supervisor but mocks its Popen, inventory and
// signals. It checks receipt semantics, not the host's actual process state.
func TestBodyTimeNextSupervisorReceipts(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not installed; next-supervisor receipt regression not executed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, "-c", bodyTimeNextRunnerCases)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("body-time next supervisor receipts: %v\n%s", err, out)
	}
}

const bodyTimeNextRunnerCases = `
import importlib.util, json, pathlib, signal, subprocess, tempfile, time
from unittest.mock import patch
spec = importlib.util.spec_from_file_location('bodytime_next', 'testdata/bodytime-verify-next.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
root = dict(pid=12345, ppid=123, pgid=12345, start='Sat Nov 14 14:00:00 2026')
class Child:
    pid = root['pid']
    returncode = None
    def __init__(self, code): self.code = code
    def poll(self): self.returncode = self.code; return self.code
    def wait(self, timeout): self.returncode = self.code; return self.code

def events(path): return [json.loads(line) for line in path.read_text().splitlines()]
for browser, code in ((False, 0), (True, 0), (True, -9)):
    with tempfile.TemporaryDirectory() as tmp:
        m.OUT = pathlib.Path(tmp)
        receipts = m.Receipts(m.OUT / 'receipts.jsonl')
        job = m.Job(1, ['mock-command'], 1, {}, receipts, time.monotonic()+30, browser)
        with patch.object(m.subprocess, 'Popen', return_value=Child(code)), \
             patch.object(m, 'snapshot', side_effect=[{12345: root}, {12345: root}, {}, {}]), \
             patch.object(m.os, 'killpg') as group_signal, patch.object(m.os, 'kill') as pid_signal:
            try:
                job.run()
            except subprocess.CalledProcessError as err:
                assert browser and code == -9 and err.returncode == code
            except m.Block:
                assert browser and code == 0
            else:
                assert not browser and code == 0
            group_signal.assert_not_called()
            pid_signal.assert_not_called()
        rows = events(receipts.path)
        kinds = [row['event'] for row in rows]
        assert 'command_launch' in kinds and 'owned_identity' in kinds and 'command_reap' in kinds
        assert 'identity_absent' in kinds and 'group_empty' in kinds
        if browser:
            assert 'cleanup_block' in kinds and 'job_quiescent' not in kinds
        else:
            assert 'job_quiescent' in kinds and 'cleanup_block' not in kinds
for reused in (False, True):
    with tempfile.TemporaryDirectory() as tmp:
        m.OUT = pathlib.Path(tmp)
        receipts = m.Receipts(m.OUT / 'receipts.jsonl')
        job = m.Job(1, ['mock-command'], 1, {}, receipts, time.monotonic()+10)
        job.register(root, 'fixture-owned identity')
        observed = dict(root, start='replacement identity') if reused else root
        with patch.object(m, 'snapshot', return_value={12345: observed}), \
             patch.object(m.os, 'killpg') as group_signal:
            job.signal_groups(signal.SIGTERM, time.monotonic()+1)
            if reused: group_signal.assert_not_called()
            else: group_signal.assert_called_once_with(12345, signal.SIGTERM)
        kinds = [row['event'] for row in events(receipts.path)]
        assert ('cleanup_block' in kinds) == reused
        assert ('signal' in kinds) == (not reused)
print('next supervisor receipts: launch, identity, reap, scoped absence, first failure and PID reuse')
`

const bodyTimeRunnerCases = `
import contextlib, importlib.util, io, pathlib, subprocess, tempfile
from unittest.mock import patch
spec = importlib.util.spec_from_file_location('bodytime_verify', 'testdata/bodytime-verify.py')
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
for code in (1, -9):
    for record in (None, '{', '{"phase":"launching"}'):
        with tempfile.TemporaryDirectory() as tmp:
            m.OUT = pathlib.Path(tmp)
            m.OWNERS = m.OUT / 'browser-owner.json'
            if record is not None: m.OWNERS.write_text(record)
            class Child:
                pid = 12345
                def wait(self, timeout): return code
            output = io.StringIO()
            with patch.object(m.subprocess, 'Popen', return_value=Child()), \
                 patch.object(m, 'identity', return_value='owned direct child'), \
                 patch.object(m.os, 'killpg') as group_signal, \
                 patch.object(m.os, 'kill') as pid_signal, \
                 contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
                try:
                    m.run(['node', 'probe'], 1, browser_command=True)
                except subprocess.CalledProcessError as err:
                    assert err.returncode == code, (err.returncode, code)
                else:
                    raise AssertionError('original first failure was lost')
                assert 'CLEANUP BLOCK:' in output.getvalue(), output.getvalue()
                group_signal.assert_not_called()
                pid_signal.assert_not_called()
print('finite runner preserves early failure and blocks unknown browser ownership')
`

const bodyTimeNodePrelude = `
const assert = require('node:assert/strict'), vm = require('node:vm');
class E {
  constructor(cls = '') { this.className = cls; this.style = {}; this.attrs = {}; this.children = []; this.events = {}; this.hidden = false; this._text = ''; }
  get classList() { return { contains: c => this.className.split(' ').includes(c) }; }
  set textContent(v) { this._text = v; this.children = []; }
  get textContent() { return this._text; }
  setAttribute(k, v) { this.attrs[k] = String(v); }
  getAttribute(k) { return this.attrs[k] ?? null; }
  appendChild(e) { this.children.push(e); e.parentNode = this; return e; }
  insertBefore(e) { this.appendChild(e); body.nextElementSibling = e; }
  remove() { this.removed = true; }
  addEventListener(k, f) { this.events[k] = f; }
  querySelector() { return this.nested || null; }
  closest() { return this.topic || null; }
  getBoundingClientRect() { return this.rect || { left: 0, top: 0, right: 120, bottom: 20, width: 120, height: 20 }; }
}
const header = new E(), activity = new E(), body = new E('doc-body'), parent = new E();
body.parentNode = parent;
body.rect = { left: 10, top: 10, right: 310, bottom: 210, width: 300, height: 200 };
body.topic = { querySelector: () => header };
let comments = [], timers = [], observed = 0, loads = [];
const doc = {
  body: new E(), activeElement: { id: 'keep-focus' },
  querySelector: () => header,
  querySelectorAll: sel => sel === 'time.tago[data-at]' ? [activity] : [header, ...doc.body.children.filter(e => e.attrs['data-body-age'] !== undefined)],
  createElement: () => new E(),
  createTreeWalker: () => { let i = 0; return { nextNode: () => comments[i++] || null }; }
};
activity.setAttribute('data-at', '1794651300000');
const s = { document: doc, NodeFilter: { SHOW_COMMENT: 128 }, console,
  innerWidth: 360, innerHeight: 100, setInterval: f => { timers.push(f); return timers.length; },
  addEventListener: (k, f) => loads.push(f),
  getComputedStyle: () => ({ paddingLeft: '14px' }),
  ResizeObserver: class { constructor(f) { this.f = f; } observe() { observed++; } }
};
s.window = s;
function rect(top, height) { return { left: 24, top, bottom: top + height, width: 250, height }; }
function mark(kind, top, height, nested) {
  const el = new E(); el.rect = rect(top, height);
  if (nested) { el.nested = new E(); el.nested.rect = rect(top + nested, height - nested); }
  return { data: 'wbchg:' + kind, nextElementSibling: el, previousElementSibling: el };
}
`

const bodyTimeNodeCases = `
vm.createContext(s);
vm.runInContext('Date.now = () => Date.parse("2026-11-14T13:00:00Z")', s);
vm.runInContext(shipped, s);
const now = Date.parse('2026-11-14T13:00:00Z');
const atMinutes = n => new Date(now - n * 60000).toISOString();
for (const [minutes, want] of [[0, '방금'], [1, '1분 전'], [59, '59분 전'], [60, '1시간 전'],
  [1439, '23시간 전'], [1440, '1일 전'], [29*1440, '29일 전'], [30*1440, '1개월 전'],
  [364*1440, '12개월 전'], [365*1440, '1년 전'], [730*1440, '2년 전']]) {
  assert.equal(s.wbBodyTimeText(atMinutes(minutes)), '본문 ' + want + ' 갱신');
}
assert.equal(s.wbBodyTimeText(''), '변경 시각 모름');
assert.equal(s.wbBodyTimeText(null), '변경 시각 모름');
assert.equal(s.wbBodyTimeText('not-a-time'), '변경 시각 모름');
assert.equal(s.wbBodyTimeText(atMinutes(-10)), '본문 방금 갱신');
// Existing activity/stream behavior remains day-based, even for old entries.
assert.equal(s.wbAgoText(now - 730*86400000), '730일 전');
header.setAttribute('data-body-at', atMinutes(5));
s.wbAgo(doc);
assert.equal(header.textContent, '본문 5분 전 갱신');
assert.notEqual(activity.textContent, header.textContent);
vm.runInContext(shipped, s);
assert.equal(timers.length, 1, 'board reruns must not accumulate timers');
comments = [mark('new', 20, 60, 25), mark('mod', 90, 30), mark('del', 130, 20), mark('delafter', 160, 20)];
s.wbWatchChanges(body);
assert.equal(observed, 1);
let layer = body.nextElementSibling;
assert.equal(layer.children.length, 4);
assert.deepEqual(layer.children.map(h => h.children[0].className), ['wb-chg-bar new', 'wb-chg-bar mod', 'wb-chg-bar del', 'wb-chg-bar del']);
assert.equal(layer.children[0].children[0].style.height, '25px', 'nested list keeps its own marks');
assert.equal(layer.children[2].style.height, '12px', 'deletion hit area is usable');
assert.equal(layer.children[3].children[0].style.top, '5px');
assert.equal(layer.children[3].style.top, '174px', 'end deletion stays at last block bottom');
// Regression: the transparent left-edge target must survive topic clipping.
// A deletion used to begin 3px outside the body, although its tick was inside.
layer.children.forEach((hit, i) => {
  const left = parseFloat(hit.style.left), width = parseFloat(hit.style.width);
  const gutterLeft = body.rect.left, textLeft = gutterLeft + 14;
  assert.equal(width, 12, 'stable useful hit width');
  assert.ok(left + 1 >= gutterLeft && left + 1 < textLeft, 'transparent-edge target remains inside the clipped gutter');
  assert.ok(left >= gutterLeft && left + width <= textLeft, 'hit box does not cross the clip edge or text column');
  assert.equal(left + parseFloat(hit.children[0].style.left), gutterLeft + (i < 2 ? 5 : 2), 'painted mark keeps its original horizontal anchor');
});
const focus = doc.activeElement;
for (const hit of layer.children) {
  const before = JSON.stringify(hit.style);
  hit.events.pointerenter();
  assert.equal(layer.wbTip.textContent, header.textContent);
  assert.equal(layer.wbTip.hidden, false);
  assert.equal(layer.wbTip.attrs.role, 'tooltip');
  assert.equal(hit.attrs.tabindex, undefined);
  assert.equal(JSON.stringify(hit.style), before, 'hover must not resize the hit box');
  hit.events.pointerleave();
  assert.equal(layer.wbTip.hidden, true);
}
assert.equal(doc.activeElement, focus);
// Refresh an already visible tooltip on the same timer as the header.
layer.children[0].events.pointerenter();
header.setAttribute('data-body-at', atMinutes(6));
layer.wbTip.setAttribute('data-body-at', atMinutes(6));
timers[0]();
assert.equal(header.textContent, layer.wbTip.textContent);
// Isolated frame reads local metadata, never topic activity or a parent.
body.topic = null;
header.attrs = {};
s.wbChangeBars(body);
layer = body.nextElementSibling;
layer.children[0].events.pointerenter();
assert.equal(layer.wbTip.textContent, '변경 시각 모름');
assert.equal(doc.body.children.length, 1, 'redraw reuses the document tooltip');
header.setAttribute('data-body-at', atMinutes(730*1440));
layer.children[0].events.pointerenter();
assert.equal(layer.wbTip.textContent, '본문 2년 전 갱신');
body.className = 'doc-body chg-stale';
s.wbChangeBars(body);
assert.equal(layer.style.opacity, '.35');
assert.equal(layer.wbTip.hidden, true, 'redraw hides a disconnected hit target tooltip');
comments = [];
s.wbChangeBars(body);
assert.equal(layer.removed, true);
console.log('body-time formatter and gutter behavior passed');
`
