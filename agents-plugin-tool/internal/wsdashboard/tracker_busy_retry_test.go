package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// runTrackerNode runs script under node and fails the test with what it
// printed; it skips when node is not installed.
func runTrackerNode(t *testing.T, what, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; " + what + " cases not executed")
	}
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("%s cases failed:\n%s", what, s)
	}
}

// trackerJSBlock returns the part of tracker.js from start up to (not
// including) end, both regexp-quoted literals.
func trackerJSBlock(t *testing.T, start, end string) string {
	t.Helper()
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(start) + `.*?` + regexp.QuoteMeta(end)).FindString(string(data))
	if block == "" {
		t.Fatalf("tracker.js holds no %q ... %q block", start, end)
	}
	return strings.TrimSuffix(block, end)
}

// TestTrackerWikiBusyReadsRetry pins the wiki reads' answer to a busy store
// (tracker.js wkRetry): a 503 for the wiki list, page list, or page is not
// drawn as an error but read again after the retry delay, the read staying
// busy meanwhile so nothing starts a second one; a store switched away from
// during the wait is not read; the retries stop after WK_RETRIES in a row
// and the error shows.
func TestTrackerWikiBusyReadsRetry(t *testing.T) {
	loaders := trackerJSBlock(t, "\n  function wkHas(st, path) {", "\n  function wkPicker() {")
	runTrackerNode(t, "wiki busy retry", `
var ui = { store: 'A', wiki: {}, layout: 'wide' }, pending = [], timers = [], bad = [];
function setTimeout(fn, ms) { timers.push({ fn: fn, ms: ms }); }
function render() {}
function tabState() { return {}; }
function wkStartEdit(path) { bad.push('editor opened for ' + path); }
var AD = { wiki: {
  list: function (store, cb) { pending.push({ k: 'list', store: store, cb: cb }); },
  pages: function (store, name, cb) { pending.push({ k: 'pages', store: store, cb: cb }); },
  read: function (store, name, path, cb) { pending.push({ k: 'read', store: store, path: path, cb: cb }); }
} };
`+loaders+`
var BUSY = { ok: false, status: 503, error: 'store busy' };
function answer(k, res) {
  var i = pending.findIndex(function (p) { return p.k === k; });
  if (i < 0) { bad.push('no ' + k + ' read pending'); return; }
  pending.splice(i, 1)[0].cb(res);
}
function fire() { var t = timers.shift(); if (!t) { bad.push('no timer'); return; } t.fn(); }
function kinds() { return pending.map(function (p) { return p.k; }).sort().join(','); }
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function state(over) {
  var st = { mode: 'wiki', name: 'w', list: [{ name: 'w' }], pages: { paths: ['index'], commit: 'c1' }, page: 'index',
    doc: null, docErr: null, edit: null, conflict: null };
  for (var k in over) st[k] = over[k];
  return st;
}

// The list: a 503 waits WK_RETRY_MS and reads again; no error is drawn.
var s = ui.wiki.A = state({ list: null });
wkLoadList(s, 'A');
answer('list', BUSY);
check('list after a 503', [s.listErr || null, s.list, kinds(), timers.length, timers[0] && timers[0].ms], [null, null, '', 1, 1000]);
wkLoadList(s, 'A');
check('no second list read during the wait', kinds(), '');
fire();
check('list read again', kinds(), 'list');
answer('list', { ok: true, wikis: [{ name: 'w' }] });
check('list after the retry', s.list, [{ name: 'w' }]);

// The page list.
s = ui.wiki.A = state({ pages: null });
wkLoadPages(s, 'A', 'w');
answer('pages', BUSY);
check('pages after a 503', [s.pagesErr || null, s.pages, kinds()], [null, null, '']);
fire();
answer('pages', { ok: true, pages: ['index'], commit: 'c2' });
check('pages after the retry', s.pages, { paths: ['index'], commit: 'c2' });

// The page: a change signal during the wait starts no second read, and a 503 is not
// taken for a missing page.
s = ui.wiki.A = state({});
wkLoadDoc(s, 'A', 'w', 'index');
answer('read', BUSY);
check('page after a 503', [s.doc, s.docErr, kinds()], [null, null, '']);
wkRefresh();
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index'], commit: 'c1' });
check('no page read during the wait', kinds(), '');
fire();
check('page read again', kinds(), 'read');
answer('read', { ok: true, path: 'index', content: 'body', commit: 'c1', html: '' });
check('page after the retry', s.doc && s.doc.content, 'body');
pending = [];

// A store switched away from during the wait is not read when it ends.
timers = [];
s = ui.wiki.A = state({ list: null });
wkLoadList(s, 'A');
answer('list', BUSY);
ui.store = 'B';
fire();
check('no read for a store not shown', [kinds(), s.listBusy], ['', false]);
ui.store = 'A';

// The retries stop after WK_RETRIES in a row, and the error shows.
timers = [];
s = ui.wiki.A = state({ list: null });
wkLoadList(s, 'A');
for (var i = 0; i < 5; i++) { answer('list', BUSY); fire(); }
answer('list', BUSY);
check('after the last retry', [s.listErr, timers.length, kinds()], ['store busy', 0, '']);

process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerStoreImagesReload pins the reload of a failed store image
// (tracker.js imgRetry): an attachment or wiki image whose load fails
// loads again after the delay with a fresh query, at most IMG_RETRIES
// times per element; other images, and an element a redraw removed, are
// left alone. listen() wires it as a capture-phase error listener.
func TestTrackerStoreImagesReload(t *testing.T) {
	retry := trackerJSBlock(t, "\n  var IMG_RETRY_MS", "\n  function listen() {")
	listen := trackerJSBlock(t, "\n  function listen() {", "\n  function mount(")
	if !strings.Contains(listen, "document.addEventListener('error', imgRetry, true)") {
		t.Fatal("listen() does not wire imgRetry as a capture-phase error listener")
	}
	runTrackerNode(t, "store image reload", `
var timers = [], bad = [];
function setTimeout(fn, ms) { timers.push({ fn: fn, ms: ms }); }
var location = { href: 'http://127.0.0.1:9/', origin: 'http://127.0.0.1:9' };
`+retry+`
function img(src) {
  return { tagName: 'IMG', src: src, isConnected: true, a: {},
    getAttribute: function (k) { return k in this.a ? this.a[k] : null; },
    setAttribute: function (k, v) { this.a[k] = v; } };
}
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function fire() { var t = timers.shift(); if (!t) { bad.push('no timer'); return; } t.fn(); }

var oid = 'ab'.repeat(32);
var w = img('http://127.0.0.1:9/tracker/stores/team/wiki/docs/attachments/' + oid);
for (var n = 1; n <= 3; n++) {
  imgRetry({ target: w });
  check('wiki image timer ' + n, [timers.length, timers[0] && timers[0].ms], [1, 1500]);
  fire();
  check('wiki image reload ' + n, w.src, '/tracker/stores/team/wiki/docs/attachments/' + oid + '?retry=' + n);
}
imgRetry({ target: w });
check('no reload past the bound', timers.length, 0);

var a = img('/tracker/stores/team/attachments/' + oid);
imgRetry({ target: a });
fire();
check('attachment reload', a.src, '/tracker/stores/team/attachments/' + oid + '?retry=1');

var gone = img('/tracker/stores/team/attachments/' + oid);
imgRetry({ target: gone });
gone.isConnected = false;
fire();
check('removed element left alone', gone.src, '/tracker/stores/team/attachments/' + oid);

imgRetry({ target: img('/static/logo.png') });
imgRetry({ target: img('http://elsewhere.invalid/tracker/stores/team/attachments/' + oid) });
imgRetry({ target: { tagName: 'SCRIPT', src: '/tracker/stores/team/attachments/' + oid } });
check('other errors ignored', timers.length, 0);

process.stdout.write(bad.join("\n"));
`)
}
