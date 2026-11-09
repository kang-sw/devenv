package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerWikiReadsFollowStoreAndChanges pins two races of the wiki's
// on-demand reads (tracker.js wkLoadDoc / wkRefresh): a page read that
// answers 404 after the user switched stores opens no "new page" editor on
// the store shown now (it opens once the first store shows again), and a
// change signal that arrives while a read is in flight runs that read once
// more when it lands instead of being dropped, without the reads re-running
// each other forever. The cases execute under node when it is installed.
func TestTrackerWikiReadsFollowStoreAndChanges(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	loaders := regexp.MustCompile(`(?s)\n  function wkHas\(st, path\) \{.*?\n  function wkPicker\(\) \{`).FindString(string(data))
	if loaders == "" {
		t.Fatal("tracker.js holds no wkHas ... wkPicker block")
	}
	loaders = strings.TrimSuffix(loaders, "function wkPicker() {")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; wiki read cases not executed")
	}
	script := `
var ui = { store: 'A', wiki: {}, layout: 'wide' }, pending = [], starts = [], bad = [];
function render() {}
function tabState() { return {}; }
function wkStartEdit(path) { starts.push(ui.store + ':' + path); }
var AD = { wiki: {
  list: function (store, cb) { pending.push({ k: 'list', store: store, cb: cb }); },
  pages: function (store, name, cb) { pending.push({ k: 'pages', store: store, cb: cb }); },
  read: function (store, name, path, cb) { pending.push({ k: 'read', store: store, path: path, cb: cb }); }
} };
` + loaders + `
function answer(k, res) {
  var i = pending.findIndex(function (p) { return p.k === k; });
  if (i < 0) { bad.push('no ' + k + ' read pending'); return; }
  pending.splice(i, 1)[0].cb(res);
}
function kinds() { return pending.map(function (p) { return p.k; }).sort().join(','); }
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function state(over) {
  var st = { mode: 'wiki', name: 'w', list: [{ name: 'w' }], pages: { paths: ['index'], commit: 'c1' }, page: 'index',
    doc: { path: 'index', content: 'old', commit: 'c1', html: '' }, docErr: null, edit: null, conflict: null };
  for (var k in over) st[k] = over[k];
  return st;
}

// B1: a 404 for store A's page lands while store B shows.
var a = ui.wiki.A = state({ page: 'missing', doc: null });
ui.wiki.B = state({});
wkLoadDoc(a, 'A', 'w', 'missing');
ui.store = 'B';
answer('read', { ok: false, status: 404, error: 'not found' });
check('editors opened after a switch', starts, []);
check('A page after the switched 404', [a.docBusy, a.doc, a.docErr], [false, null, null]);
ui.store = 'A';
wkPull(a);
answer('read', { ok: false, status: 404, error: 'not found' });
check('editors opened once A shows', starts, ['A:missing']);

// B2: a change signal while every read is in flight.
pending = []; ui.store = 'A';
var s = ui.wiki.A = state({});
wkRefresh();
check('first signal reads', kinds(), 'list,pages,read');
wkRefresh();
check('second signal while busy', kinds(), 'list,pages,read');
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index'], commit: 'c1' });
answer('read', { ok: true, path: 'index', content: 'old', commit: 'c1', html: '' });
check('reads re-run after the stale answers', kinds(), 'list,pages,read');
check('stale doc drawn meanwhile', s.doc.content, 'old');
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index', 'new'], commit: 'c2' });
answer('read', { ok: true, path: 'index', content: 'new', commit: 'c2', html: '' });
check('fresh answers', [s.pages.commit, s.doc.content], ['c2', 'new']);
check('no reads after the re-run', kinds(), '');

// A signal that finds only the doc read busy re-runs the doc read alone.
wkRefresh();
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index', 'new'], commit: 'c2' });
wkRefresh();
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index', 'new'], commit: 'c2' });
check('only the doc read pending', kinds(), 'read');
answer('read', { ok: true, path: 'index', content: 'new', commit: 'c2', html: '' });
check('the doc read re-ran once', kinds(), 'read');
answer('read', { ok: true, path: 'index', content: 'newer', commit: 'c3', html: '' });
check('settled', [kinds(), s.doc.content], ['', 'newer']);

process.stdout.write(bad.join("\n"));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("wiki read cases failed:\n%s", s)
	}
}
