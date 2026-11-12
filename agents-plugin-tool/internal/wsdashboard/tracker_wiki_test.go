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

// B1 with a change signal: the signal marks the in-flight read to run again, and the 404
// lands while B shows; the re-run must not read A's page softly (a soft 404 is an error,
// which keeps A's new-page editor from opening when A shows again).
pending = []; starts = []; ui.store = 'A';
a = ui.wiki.A = state({ page: 'missing', doc: null });
wkLoadDoc(a, 'A', 'w', 'missing');
wkRefresh();
ui.store = 'B';
answer('read', { ok: false, status: 404, error: 'not found' });
check('reads pending after the switched 404', kinds(), 'list,pages');
answer('list', { ok: true, wikis: [{ name: 'w' }] });
answer('pages', { ok: true, pages: ['index'], commit: 'c1' });
check('A page after the switched 404 and signal', [a.doc, a.docErr], [null, null]);
ui.store = 'A';
wkPull(a);
answer('read', { ok: false, status: 404, error: 'not found' });
check('editors opened once A shows after the signal', starts, ['A:missing']);

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
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("wiki read cases failed:\n%s", s)
	}
}

// TestTrackerWikiNavigationAsksBeforeDroppingAnEdit pins the leave guard of
// the wiki panel (tracker.js wkLeaveOk): following a wiki: link, opening a
// page, switching wiki, and starting a new wiki ask before they drop an edit
// whose text changed (a new page's seeded heading counts as unchanged);
// declining keeps the editor, the page, and an open item window; a clean
// editor, a pick of the shown wiki, and the reopen after a save do not ask.
// The cases execute under node when it is installed.
func TestTrackerWikiNavigationAsksBeforeDroppingAnEdit(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	nav := regexp.MustCompile(`(?s)\n  function wkShow\(\) \{.*?\n  function wkCancel\(\) \{`).FindString(src)
	act := regexp.MustCompile(`(?s)\n  function wkAct\(k, a\) \{.*?\n  function cardTeam\(v\) \{`).FindString(src)
	if nav == "" || act == "" {
		t.Fatal("tracker.js holds no wkShow ... wkCancel or wkAct ... cardTeam block")
	}
	nav = strings.TrimSuffix(nav, "function wkCancel() {")
	act = strings.TrimSuffix(act, "function cardTeam(v) {")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; wiki leave guard cases not executed")
	}
	script := `
var asked = 0, answer = false, bad = [], closed = 0, st;
var window = { confirm: function (m) { asked++; if (m !== 'LEAVE') bad.push('prompt text ' + m); return answer; } };
var ui = { layout: 'wide', win: false, edpv: {}, eatt: {} };
function wsT(k) { return k === 'js.tk.wk.leave' ? 'LEAVE' : k; }
function wk() { return st; }
function render() {}
function $() { return null; }
function tabState() { return {}; }
function closeItemWindow() { closed++; ui.win = false; }
function wkHas(s, p) { return s.pages.paths.indexOf(p) >= 0; }
function el(attrs) { return { getAttribute: function (k) { return attrs[k]; } }; }
` + nav + act + `
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function fresh() {
  st = { mode: 'wiki', name: 'w', list: [{ name: 'w' }, { name: 'v' }], pages: { paths: ['index', 'a'], commit: 'c1' }, page: 'index',
    doc: { path: 'index', content: 'old', commit: 'c1' }, docErr: null, edit: null, conflict: null, pick: true };
  asked = 0; closed = 0; ui.win = false;
}
function dirty() { wkStartEdit('index', false, false); st.edit.text = 'changed'; }
var navs = {
  link: function () { wkOpenLink('w', 'a'); },
  open: function () { wkAct('wk-open', el({ 'data-path': 'a' })); },
  pick: function () { wkAct('wk-pick', el({ 'data-wiki': 'v' })); },
  newwiki: function () { wkAct('wk-newwiki', el({})); }
};
Object.keys(navs).forEach(function (n) {
  fresh(); dirty(); answer = false; var ed = st.edit;
  navs[n]();
  check(n + ' declined: asked', asked, 1);
  check(n + ' declined: kept', [st.edit === ed, st.edit.text, st.name, st.page, !!st.creating], [true, 'changed', 'w', 'index', false]);
  fresh(); dirty(); answer = true;
  navs[n]();
  check(n + ' accepted: asked', asked, 1);
  check(n + ' accepted: dropped', st.edit, null);
  fresh(); wkStartEdit('index', false, false);
  navs[n]();
  check(n + ' clean edit: asked', asked, 0);
  fresh();
  navs[n]();
  check(n + ' no edit: asked', asked, 0);
});

// a new page is clean until its text leaves the seeded heading; its conflict merge base stays ''
fresh(); wkStartEdit('dir/new-page', true, false);
check('new page seed', [st.edit.text, st.edit.orig], ['# new page\n\n', '']);
wkOpenLink('w', 'a');
check('untouched new page: asked', asked, 0);
fresh(); wkStartEdit('dir/new-page', true, false); st.edit.text += 'body'; answer = false;
wkOpenLink('w', 'a');
check('typed new page: asked', asked, 1);

// declining a link from an item window leaves the window open; accepting closes it once
fresh(); dirty(); ui.win = true; answer = false;
wkOpenLink('w', 'a');
check('declined link from window', [asked, closed, ui.win], [1, 0, true]);
fresh(); dirty(); ui.win = true; answer = true;
wkOpenLink('w', 'a');
check('accepted link from window', [asked, closed, ui.win, st.page], [1, 1, false, 'a']);

// a pick of the wiki shown drops nothing and does not ask
fresh(); dirty();
wkAct('wk-pick', el({ 'data-wiki': 'w' }));
check('pick of the shown wiki', [asked, st.edit && st.edit.text], [0, 'changed']);

// Edit again after a refused save is dirty against the saved "theirs" text
fresh(); st.conflict = { c: { path: 'index', theirs: 'c2', theirsText: 'theirs' }, mine: 'mine', orig: 'old' };
wkAct('wk-again', el({})); answer = false;
wkOpenLink('w', 'a');
check('edit again: asked', asked, 1);

// the reopen after a save (the save clears the edit) does not ask
fresh(); dirty(); st.edit = null;
wkOpenLink('w', 'a');
check('after save: asked', asked, 0);

process.stdout.write(bad.join("\n"));
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("wiki leave guard cases failed:\n%s", s)
	}
}
