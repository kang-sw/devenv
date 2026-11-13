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
	// the page-path and seed helpers wkGoPage and wkStartEdit call
	helpers := trackerJSBlock(t, "\n  // A page path as the daemon answers it", "\n  // Reads land on the state object") +
		trackerJSBlock(t, "\n  // An edit of an HTML page", "\n  // Line regions of side")
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
function esc(s) { return String(s); }
` + helpers + nav + act + `
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

// trackerWikiHTMLStubs is the node prelude the HTML page cases share: the
// helpers the extracted wiki code calls, a fake frame layer whose
// innerHTML assignments mint a new child (so element identity tells a kept
// frame from a re-inserted one), and drawn(), the subtree a draw filled,
// holding the read view's slot when the markup has one.
const trackerWikiHTMLStubs = `
var bad = [], refs = [], renders = 0, frames = 0;
var ui = { store: 'A', wiki: {}, layout: 'wide', win: false, edpv: {}, eatt: {} };
var document = { body: { tag: 'body' } }, window = {};
function render() { renders++; }
function tabState() { return {}; }
function wk() { return ui.wiki[ui.store]; }
function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); }
function unesc(s) { return String(s).replace(/&quot;/g, '"').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&amp;/g, '&'); }
function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }
function wsT(k) { return k; }
function tipAttr(t) { return ' data-tip="' + esc(t) + '"'; }
function wkCrumb(p) { return '<nav>' + esc(p) + '</nav>'; }
function wkSha(c) { return String(c || '').slice(0, 7); }
function wkIc(id) { return '<svg>' + id + '</svg>'; }
function wkMoreMenu() { return ''; }
function refLinksHtml(h) { refs.push(h); return h; }
function getComputedStyle() { return { overflowX: 'visible', overflowY: 'visible' }; }
var layer = { hidden: true, style: {}, firstElementChild: null, sets: 0,
  set innerHTML(v) { this.sets++; this.firstElementChild = v ? { n: ++frames, html: v } : null; },
  get innerHTML() { return this.firstElementChild ? this.firstElementChild.html : ''; } };
var nodes = {};
function $(id) { return id === 'tkframe' ? layer : nodes[id] || null; }
function drawn(html) {
  var m = /<div class="wk-fslot" data-fkey="([^"]*)"><\/div>/.exec(html || ''), key = m && unesc(m[1]);
  var slot = m && { parentElement: null, getAttribute: function (k) { return k === 'data-fkey' ? key : null; },
    getBoundingClientRect: function () { return { top: 10, left: 20, bottom: 110, right: 220, width: 200, height: 100 }; } };
  return { querySelector: function (sel) { return sel === '.wk-fslot' ? slot : null; } };
}
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
var FRAME = '<iframe title="g.html" sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox" srcdoc="&lt;p&gt;hi"></iframe>';
function state(over) {
  var st = { mode: 'wiki', name: 'w', list: [{ name: 'w' }], pages: { paths: ['index', 'g.html', 'notes/a'], commit: 'c1' }, page: 'g.html',
    doc: { path: 'g.html', kind: 'html', content: '<p>hi', commit: 'c1', html: FRAME }, docErr: null, edit: null, conflict: null, more: false, pick: false };
  for (var k in over) st[k] = over[k];
  return st;
}
`

// TestTrackerWikiHTMLPageFrame pins how an HTML page shows (tracker.js
// wkPageRead, wkFrameSync): the read view draws an empty slot instead of the
// article and never passes the frame markup through refLinksHtml, while a
// Markdown page keeps its article; the frame lives in the persistent layer,
// inserted as the host built it; a draw that keeps (store, wiki, path,
// commit) keeps the same iframe element; a draw without the slot (item
// window, editor, drawer closed, the page menu open) hides the layer and
// keeps the frame, so it comes back without a reload; a change of any of
// the four replaces it, and one while the layer is hidden empties it. It
// also pins the page read's kind and the canonical .md page path.
func TestTrackerWikiHTMLPageFrame(t *testing.T) {
	loaders := trackerJSBlock(t, "\n  function wkHas(st, path) {", "\n  function wkPicker() {")
	read := trackerJSBlock(t, "\n  function wkPageRead() {", "\n  function wkPageEdit() {")
	frame := trackerJSBlock(t, "\n  var WKF = ", "\n  // The panel shows the wiki:")
	runTrackerNode(t, "wiki HTML page frame", trackerWikiHTMLStubs+`
var pending = [];
function wkStartEdit(path) { bad.push('editor opened for ' + path); }
var AD = { wiki: { read: function (store, name, path, cb) { pending.push({ store: store, path: path, cb: cb }); } } };
`+loaders+read+frame+`
function draw() { var h = wkPageRead(); wkFrameSync(drawn(h)); return h; }

// A Markdown page keeps its article and its refLinksHtml pass; the layer stays empty and hidden.
var a = ui.wiki.A = state({ page: 'notes/a', doc: { path: 'notes/a', kind: 'markdown', content: 'x', commit: 'c1', html: '<p>md</p>' } });
var h = draw();
check('markdown article', [h.indexOf('<article class="wk-doc md"><p>md</p></article>') >= 0, h.indexOf('wk-fslot') >= 0, refs], [true, false, ['<p>md</p>']]);
check('markdown layer', [layer.hidden, layer.innerHTML], [true, '']);

// An HTML page: the slot, no article, no refLinksHtml; the layer holds the host's markup as given, over the slot.
refs = [];
a = ui.wiki.A = state({});
h = draw();
check('html read view', [h.indexOf('<article') >= 0, h.indexOf('wk-fslot') >= 0, h.indexOf('<iframe') >= 0, refs], [false, true, false, []]);
check('html layer', [layer.hidden, layer.innerHTML, layer.sets], [false, FRAME, 1]);
check('layer over the slot', [layer.style.left, layer.style.top, layer.style.width, layer.style.height, layer.style.clipPath], ['20px', '10px', '200px', '100px', 'inset(0px 0px 0px 0px)']);
var f1 = layer.firstElementChild;

// Draws that keep the key keep the very element.
draw(); draw();
check('same key keeps the frame', [layer.firstElementChild === f1, layer.sets, layer.hidden], [true, 1, false]);

// No slot with the key unchanged: hidden, frame kept, back without a reload.
function away(what, leave, back) {
  leave(); check(what + ': hidden', [layer.hidden, layer.firstElementChild === f1], [true, true]);
  back(); check(what + ': back', [layer.hidden, layer.firstElementChild === f1, layer.sets], [false, true, 1]);
}
away('item window', function () { ui.win = true; draw(); }, function () { ui.win = false; draw(); });
away('editor', function () { a.edit = { path: 'g.html' }; wkFrameSync(drawn('<div class="wk-edit"></div>')); }, function () { a.edit = null; draw(); });
away('drawer closed', function () { wkFrameSync(null); }, function () { draw(); });
away('items panel', function () { wkFrameSync(drawn('<div class="list"></div>')); }, function () { draw(); });
away('page menu', function () { a.more = true; draw(); }, function () { a.more = false; draw(); });

// A change of each part of the key replaces the frame.
function replaced(what, change) {
  var before = layer.firstElementChild, sets = layer.sets;
  change(); draw();
  check(what + ' replaces the frame', [layer.firstElementChild !== before, !!layer.firstElementChild, layer.sets, layer.hidden], [true, true, sets + 2, false]);
}
replaced('commit', function () { a.doc = { path: 'g.html', kind: 'html', content: 'v2', commit: 'c2', html: FRAME + ' ' }; });
replaced('path', function () { a.page = 'h.html'; a.doc = { path: 'h.html', kind: 'html', content: 'h', commit: 'c2', html: FRAME }; });
replaced('wiki', function () { a.name = 'v'; });
replaced('store', function () { ui.store = 'B'; ui.wiki.B = state({ doc: a.doc, page: a.page }); });
check('the new frame is the new page', layer.innerHTML, FRAME);

// A key change while hidden empties the layer; the new frame goes in when its slot shows.
ui.store = 'A';
var b = ui.wiki.B;
wkFrameSync(null);
check('store switched while hidden', [layer.hidden, layer.innerHTML], [true, '']);
a.doc = { path: 'g.html', kind: 'html', content: 'v3', commit: 'c3', html: FRAME };
a.edit = { path: 'g.html' };
wkFrameSync(drawn(''));
check('no slot for the new key: still empty', [layer.hidden, layer.innerHTML], [true, '']);
a.edit = null; draw();
check('slot for the new key: inserted', [layer.hidden, layer.innerHTML], [false, FRAME]);
// another page that is Markdown empties it too
a.page = 'notes/a'; a.doc = { path: 'notes/a', kind: 'markdown', content: 'x', commit: 'c3', html: '<p>md</p>' };
draw();
check('a Markdown page empties the layer', [layer.hidden, layer.innerHTML], [true, '']);
// a slot left over from another key is not used
a.page = 'g.html'; a.doc = { path: 'g.html', kind: 'html', content: 'v3', commit: 'c3', html: FRAME };
wkFrameSync(drawn('<div class="wk-fslot" data-fkey="other"></div>'));
check('a stale slot is ignored', [layer.hidden, layer.innerHTML], [true, '']);

// The page read records the kind; an answer without one (an older daemon) is Markdown.
ui.store = 'A'; pending = [];
a = ui.wiki.A = state({ page: 'g.html', doc: null });
wkLoadDoc(a, 'A', 'w', 'g.html');
pending.shift().cb({ ok: true, path: 'g.html', kind: 'html', content: '<p>hi', commit: 'c1', html: FRAME });
check('kind html', [a.doc.kind, a.doc.html], ['html', FRAME]);
a.doc = null; a.docBusy = false;
wkLoadDoc(a, 'A', 'w', 'g.html');
pending.shift().cb({ ok: true, path: 'g.html', content: 'x', commit: 'c1', html: '<p>x</p>' });
check('kind missing', a.doc.kind, 'markdown');

// The canonical page path: name.md is page name; HTML paths and refused names stay as written.
check('wkCanon', ['notes/a.md', 'a.md', 'g.html', 'a.html.md', 'a.md.md', '.md', 'x.MD', 'notes/a'].map(wkCanon),
  ['notes/a', 'a', 'g.html', 'a.html.md', 'a.md.md', '.md', 'x.MD', 'notes/a']);
check('wkIsHtml', ['g.html', 'f/g.html', '.html', 'f/.html', 'g.HTML', 'g.html.md', 'g'].map(wkIsHtml), [true, true, false, false, false, false, false]);
check('wkHas through .md', [wkHas(a, 'notes/a.md'), wkHas(a, 'notes/a'), wkHas(a, 'g.html'), wkHas(a, 'g')], [true, true, true, false]);

process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerWikiHTMLPageEditor pins the editor of an HTML page (tracker.js
// wkPageEdit, editorHtml src, edImages, wkSave, wkPathKind): an existing or
// new .html page edits as source with no Write / Preview toggle and no
// suggestions, a new one seeds an HTML document, an image pasted or dropped
// into it is not uploaded and its save sends no attachments; a new page's
// path typed into the other kind swaps an untouched seed; Markdown editing
// keeps its preview and uploads. A wiki: link to x.html opens that page and
// one to x.md opens page x.
func TestTrackerWikiHTMLPageEditor(t *testing.T) {
	helpers := trackerJSBlock(t, "\n  // A page path as the daemon answers it", "\n  // Reads land on the state object")
	edit := trackerJSBlock(t, "\n  function wkPageEdit() {", "\n  // Line regions of side")
	nav := trackerJSBlock(t, "\n  function wkShow() {", "\n  function wkCancel() {")
	save := trackerJSBlock(t, "\n  function wkSave() {", "\n  function wkAct(k, a) {")
	ed := trackerJSBlock(t, "\n  function editorHtml(id, o) {", "\n  function pvClass(")
	img := trackerJSBlock(t, "\n  var IMG_TYPES", "\n  // Puts s at box's caret")
	runTrackerNode(t, "wiki HTML page editor", trackerWikiHTMLStubs+`
var MAC = false, VS = { attachments: { max_bytes: 1000 } }, uploads = [], saves = [], toasts = [];
var AD = { upload: function (store, f) { uploads.push(f.name); }, wiki: { save: function (store, name, req, cb) { saves.push(req); } } };
function toast(m) { toasts.push(m); }
function btn(act, label) { return '<button data-act="' + act + '">' + label + '</button>'; }
function pvClass() { return 'tked-pv'; }
function pvHtml() { return ''; }
function savedAtts(id, text) { return [{ oid: 'o1' }]; }
function closeItemWindow() { ui.win = false; }
function wkHas(s, p) { return !!(s.pages && s.pages.paths.indexOf(wkCanon(p)) >= 0); }
var window = { confirm: function () { return true; } };
`+helpers+edit+nav+save+ed+img+`
function fresh(over) { ui.wiki.A = state(over || {}); uploads = []; saves = []; toasts = []; renders = 0; ui.edpv = {}; ui.eatt = {}; return ui.wiki.A; }
var PNG = { name: 'p.png', type: 'image/png', size: 10 };
function editor(h) { return [h.indexOf('data-act="ed-mode"') >= 0, h.indexOf('js.tk.ed.preview') >= 0, h.indexOf('data-mention') >= 0, h.indexOf(' data-src') >= 0]; }

// An existing HTML page: source only.
var st = fresh();
wkStartEdit('g.html', false, false);
check('existing html edit text', st.edit.text, '<p>hi');
check('existing html editor', editor(wkPageEdit()), [false, false, false, true]);
edImages('wk-ebody', [PNG], false);
check('html image paste ignored', [uploads, toasts, ui.eatt['wk-ebody'] || null], [[], ['js.tk.ed.img.off'], null]);
wkSave();
check('html save', [saves.length, saves[0].path, saves[0].attachments], [1, 'g.html', []]);

// A new HTML page (a wiki: link to notes/x.html): the document skeleton, source only, no attachments sent.
st = fresh({ page: 'notes/x.html', doc: null });
wkStartEdit('notes/x.html', true, false);
check('html seed', st.edit.text, '<!DOCTYPE html>\n<html>\n<head>\n<meta charset="utf-8">\n<title>x</title>\n</head>\n<body>\n\n</body>\n</html>\n');
check('html seed is clean', wkDirty(st.edit), false);
check('new html editor', editor(wkPageEdit()), [false, false, false, true]);
ui.eatt['wk-ebody'] = [{ oid: 'o1' }];
st.edit.text += 'attachment:o1';
wkSave();
check('new html save', [saves[0].path, saves[0].attachments, saves[0].content.indexOf('<!DOCTYPE html>')], ['notes/x.html', [], 0]);

// Markdown editing keeps the toggle, suggestions, and uploads.
st = fresh({ page: 'notes/a', doc: { path: 'notes/a', kind: 'markdown', content: 'md', commit: 'c1', html: '' } });
wkStartEdit('notes/a', false, false);
check('markdown editor', editor(wkPageEdit()), [true, true, true, false]);
edImages('wk-ebody', [PNG], false);
check('markdown upload', uploads, ['p.png']);
wkSave();
check('markdown save attachments', saves[0].attachments, [{ oid: 'o1' }]);

// A new page's typed path: an untouched seed follows the kind, a typed text stays.
st = fresh();
wkStartEdit('notes/', true, false);
check('folder seed', st.edit.text, '# \n\n');
renders = 0;
wkPathKind(st.edit, 'notes/y.html');
check('typed .html', [st.edit.text.indexOf('<title>y</title>') > 0, wkDirty(st.edit), renders, editor(wkPageEdit())], [true, false, 1, [false, false, false, true]]);
wkPathKind(st.edit, 'notes/y.htm');
check('typed back to markdown', [st.edit.text, renders, editor(wkPageEdit())[0]], ['# y.htm\n\n', 2, true]);
wkPathKind(st.edit, 'notes/y.ht');
check('same kind: no redraw', renders, 2);
st.edit.text = 'mine';
wkPathKind(st.edit, 'notes/y.html');
check('typed text kept', [st.edit.text, renders], ['mine', 3]);
wkSave();
check('typed html path saved', [saves[0].path, saves[0].attachments], ['notes/y.html', []]);
st = fresh();
wkStartEdit('notes/', true, false);
wkPathKind(st.edit, ' /notes/z.md/ ');
wkSave();
check('a .md path saves canonical', saves[0].path, 'notes/z');

// wiki: links: x.html opens like any page; x.md opens page x.
st = fresh({ page: 'index', doc: null });
wkOpenLink('w', 'g.html');
check('link to g.html', [st.page, st.edit, renders > 0], ['g.html', null, true]);
wkOpenLink('w', 'notes/a.md');
check('link to notes/a.md', [st.page, st.edit], ['notes/a', null]);

process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerNoFrameMessages pins that tracker.js takes no messages from a
// frame (an HTML wiki page's frame must reach no host action) and builds no
// sandbox attribute of its own: the host's markup is inserted as given.
func TestTrackerNoFrameMessages(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	if regexp.MustCompile(`addEventListener\(\s*['"]message['"]|onmessage`).MatchString(src) {
		t.Error("tracker.js registers a message listener")
	}
	for _, s := range []string{"allow-scripts", "allow-same-origin", "sandbox="} {
		if strings.Contains(src, s) {
			t.Errorf("tracker.js builds frame sandbox markup (%q)", s)
		}
	}
}
