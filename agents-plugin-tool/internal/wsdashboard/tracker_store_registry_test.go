package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// dashboardJSBlock returns the part of dashboard.js from start up to (not
// including) end, both regexp-quoted literals.
func dashboardJSBlock(t *testing.T, start, end string) string {
	t.Helper()
	data, err := dashboardFS.ReadFile("assets/dashboard.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(start) + `.*?` + regexp.QuoteMeta(end)).FindString(string(data))
	if block == "" {
		t.Fatalf("dashboard.js holds no %q ... %q block", start, end)
	}
	return strings.TrimSuffix(block, end)
}

// TestTrackerStoreRegistryUI pins the renderer's store registry actions
// (tracker.js, the store registry block): a store that leaves the list,
// hidden or unregistered, drops from the open drawer, the store page and
// its route, the store menu, its dialogs, and its page-lifetime state,
// with no sync, toast, or retry, also when it runs again; a dialog about
// a store still hidden here stays. The hidden list offers show and
// unregister; show redraws the list; hide leaves the store as a collapse
// or leaving its page does and toasts once. Unregistering a store with no
// remote warns that the data is lost and enables only once the store id
// is typed, which goes to the host as the confirmation; a store with a
// remote asks for nothing typed. A refusal shows the host's sentence in
// the dialog; success closes it and toasts once.
func TestTrackerStoreRegistryUI(t *testing.T) {
	block := trackerJSBlock(t, "\n  // The store registry on this machine", "\n  function asciiId(")
	runTrackerNode(t, "store registry", `
var bad = [], toasts = [], opened = [], calls = [], replaced = [], cleared = [], menuClosed = 0, dialogsClosed = 0;
function esc(s) { return String(s == null ? '' : s).replace(/[&<>"]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]; }); }
function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }
function tipAttr(s) { return ' data-tip="' + esc(s) + '"'; }
function wsT(k) { var a = Array.prototype.slice.call(arguments, 1); return a.length ? k + '(' + a.join(',') + ')' : k; }
function fld(label, inner) { return '<fld ' + label + '>' + inner; }
function mi(act, label, data, o) { return '<mi ' + act + (o && o.cls ? ' ' + o.cls : '') + '>'; }
function msep() { return '<sep>'; }
function toast(h) { toasts.push(h); }
function closeMenu() { menuClosed++; MN = null; }
function closeDialog() { dialogsClosed++; dlg.open = false; }
function openDialog(kind, d) { opened.push([kind, d.store]); }
function leavePage() { calls.push('leavePage'); }
function collapse() { calls.push('collapse'); }
function clearTimeout(id) { cleared.push(id); }
function regMsg(form, text, warn) { var m = form.querySelector('#regmsg'); m.textContent = text; m.hidden = !text; m.warn = !!warn; }
var dlg = { open: false, form: null, hid: null, querySelector: function (s) { return s === 'form' ? dlg.form : s === '.tkhid' ? dlg.hid : null; } };
function $(id) { return id === 'tkdlg' ? dlg : null; }
var location = { hash: '', pathname: '/p', search: '' };
var history = { replaceState: function (a, b, u) { replaced.push(u); location.hash = ''; } };
var INFO = { A: { id: 'A', label: 'Lbl', remote: '', hidden: false }, B: { id: 'B', label: '', remote: 'https://x.invalid/b.git', hidden: false }, H: { id: 'H', label: 'Hid', remote: 'https://x.invalid/h.git', hidden: true } };
var HIDDEN = ['H'], pend = [];
var AD = {
  info: function (id) { return INFO[id] || null; },
  hidden: function () { return HIDDEN.slice(); },
  setHidden: function (id, h, done) { pend.push({ k: 'hidden', id: id, v: h, done: done }); },
  relabel: function (id, l, done) { pend.push({ k: 'label', id: id, v: l, done: done }); },
  unregister: function (id, c, done) { pend.push({ k: 'unregister', id: id, v: c, done: done }); },
  sync: function () { bad.push('a sync ran'); }
};
var order = ['A', 'B'], MN = null, ui = {};
function mkForm(store, confirm) {
  var bt = { disabled: false, textContent: 'go' }, msg = { textContent: '', hidden: true, warn: false };
  return { attrs: { 'data-fstore': store }, isConnected: true, bt: bt, msg: msg,
    elements: confirm === undefined ? { label: { value: '' } } : { confirm: { value: confirm } },
    getAttribute: function (k) { return k in this.attrs ? this.attrs[k] : null; },
    setAttribute: function (k, v) { this.attrs[k] = String(v); },
    removeAttribute: function (k) { delete this.attrs[k]; },
    hasAttribute: function (k) { return k in this.attrs; },
    querySelector: function (s) { return s === '#regmsg' ? msg : s === 'button[type="submit"]' ? bt : null; } };
}
`+block+`
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function has(what, s, sub, want) { if ((s.indexOf(sub) >= 0) !== want) bad.push(what + ': ' + (want ? 'missing ' : 'unexpected ') + sub + ' in ' + s); }

// the hidden list: each hidden store with show and unregister; none: an empty hidden box
var hl = regHiddenHtml();
has('hidden list', hl, 'Hid (H)', true);
has('hidden list', hl, 'data-act="store-show" data-store="H"', true);
has('hidden list', hl, 'data-act="store-unreg" data-store="H"', true);
has('hidden list buttons do not submit the register form', hl, 'type="button"', true);
dlg.hid = { outerHTML: hl };
showStore('H');
check('show request', [pend[0].k, pend[0].id, pend[0].v], ['hidden', 'H', false]);
HIDDEN = []; pend.shift().done({ ok: true });
check('the list after show', dlg.hid.outerHTML, '<div class="tkhid" hidden></div>');

// the store menu
MN = { kind: 'store', store: 'A' };
var sm = storeMenuHtml();
['m-s-label', 'm-s-hide', 'm-s-unreg danger'].forEach(function (x) { has('store menu', sm, '<mi ' + x + '>', true); });
has('store menu title', sm, 'Lbl (A)', true);
storeAct('m-s-unreg');
check('unregister from the menu', [opened.pop(), menuClosed, MN], [['unregister', 'A'], 1, null]);

// hide: the apply path first, then one toast
ui = { page: 'A', open: null };
hideStore('A');
check('hide from the page', [calls, pend[0].k, pend[0].id, pend[0].v], [['leavePage'], 'hidden', 'A', true]);
pend.shift().done({ ok: true });
check('hide toast', toasts, ['js.tk.toast.hidden(A)']);
ui = { page: null, open: 'B' }; calls = []; toasts = [];
hideStore('B');
check('hide from the drawer', calls, ['collapse']);
pend.shift().done({ ok: false, error: 'nope' });
check('a refused hide toasts its reason once', toasts, ['nope']);
toasts = [];

// dropGone: A left the list while its page, drawer, menu, and relabel dialog showed
order = ['B'];
location.hash = '#/A';
ui = { store: 'A', open: 'A', page: 'A', item: 'x', win: true, sel: 'r', sub: {}, iedit: {}, cedit: {}, replyTo: 'c', last: [],
  wiki: { A: {}, B: {} }, psr: { A: { timer: 7 } }, nrow: { A: {} }, knew: {}, tab: { A: {}, B: {} }, panel: { A: {} },
  tabSeen: { 'A|s': {}, 'B|s': {} }, loaded: {}, mdet: { 'A|m': true }, wfold: { 'A|1': true } };
MN = { kind: 'store', store: 'A' }; menuClosed = 0;
dlg.open = true; dlg.form = mkForm('A'); dlg.form.attrs['data-kind'] = 'relabel'; dialogsClosed = 0;
dropGone();
check('shown, open, and routed', [ui.store, ui.open, ui.page, ui.item, ui.win, ui.sel, ui.iedit], ['B', null, null, null, false, null, null]);
check('route cleared without a history entry', replaced, ['/p']);
check('menu and dialog closed', [menuClosed, dialogsClosed], [1, 1]);
check('per-store state', [Object.keys(ui.wiki), Object.keys(ui.psr), Object.keys(ui.nrow), Object.keys(ui.tab), Object.keys(ui.panel)], [['B'], [], [], ['B'], []]);
check('per-key state', [Object.keys(ui.tabSeen), Object.keys(ui.mdet), Object.keys(ui.wfold)], [['B|s'], [], []]);
check('search timer stopped', cleared, [7]);
dropGone();
check('a second run changes nothing', [replaced.length, menuClosed, dialogsClosed, ui.store], [1, 1, 1, 'B']);
check('no toast', toasts, []);

// a dialog about a store still hidden here stays; a store-less dialog closes with its store, the register dialog stays
HIDDEN = ['H']; dlg.open = true; dlg.form = mkForm('H'); dlg.form.attrs['data-kind'] = 'unregister'; dialogsClosed = 0;
dropGone();
check('unregister dialog of a hidden store', [dlg.open, dialogsClosed], [true, 0]);
ui.store = 'A'; dlg.form = mkForm(''); delete dlg.form.attrs['data-fstore']; dlg.form.attrs['data-kind'] = 'register';
dropGone();
check('register dialog stays', dlg.open, true);
ui.store = 'A'; dlg.form.attrs['data-kind'] = 'profile';
dropGone();
check('profile dialog of the gone store', [dlg.open, dialogsClosed], [false, 1]);
dlg.open = false; dlg.form = null;

// unregister, no remote: the loss warning, the typed id, and the confirm sent
var T = unregT({ store: 'A' });
check('no-remote intro', T[1], 'js.tk.dlg.unreg.intro.local(Lbl (A))');
has('no-remote body', T[2], 'js.tk.dlg.unreg.lost(Lbl (A))', true);
has('no-remote body', T[2], '<fld js.tk.dlg.unreg.confirm(A)><input type="text" name="confirm"', true);
var f = mkForm('A', '');
dlg.open = true; dlg.form = f;
unregReady(f);
check('disabled until typed', f.bt.disabled, true);
submitUnregister(f);
check('nothing sent untyped', pend.length, 0);
f.elements.confirm.value = 'a';
unregReady(f);
check('a wrong id stays disabled', f.bt.disabled, true);
f.elements.confirm.value = ' A ';
unregReady(f);
check('enabled once typed', f.bt.disabled, false);
submitUnregister(f);
check('request', [pend[0].k, pend[0].id, pend[0].v], ['unregister', 'A', 'A']);
check('busy', [f.bt.disabled, f.bt.textContent], [true, 'js.tk.dlg.unreg.busy']);
unregReady(f);
check('typing while busy keeps it disabled', f.bt.disabled, true);
pend.shift().done({ ok: false, error: 'The store A was not unregistered: a sync is running.', kind: 'busy' });
check('refusal in the dialog', [f.msg.textContent, f.msg.hidden, f.msg.warn, f.bt.disabled, f.bt.textContent, dlg.open, toasts], ['The store A was not unregistered: a sync is running.', false, true, false, 'go', true, []]);
submitUnregister(f);
dialogsClosed = 0;
pend.shift().done({ ok: true });
check('success', [dialogsClosed, toasts], [1, ['js.tk.toast.unreg(A)']]);
toasts = [];

// unregister, with a remote: nothing typed, an empty confirm
T = unregT({ store: 'B' });
check('remote intro', T[1], 'js.tk.dlg.unreg.intro(B)');
has('remote body', T[2], 'name="confirm"', false);
has('remote body', T[2], 'js.tk.dlg.unreg.lost', false);
f = mkForm('B'); delete f.elements.label;
dlg.open = true; dlg.form = f;
unregReady(f);
check('enabled at once', f.bt.disabled, false);
submitUnregister(f);
check('request with a remote', [pend[0].id, pend[0].v], ['B', '']);
pend.shift();

// relabel: prefilled, an unchanged label sends nothing, an empty one clears, a refusal shows in the dialog
T = relabelT({ store: 'A' });
has('relabel field', T[2], 'name="label" maxlength="80" autocomplete="off" value="Lbl"', true);
f = mkForm('A'); f.elements.label.value = ' Lbl '; dlg.open = true; dialogsClosed = 0;
submitRelabel(f);
check('unchanged label', [pend.length, dialogsClosed], [0, 1]);
f = mkForm('A'); f.elements.label.value = '  '; dlg.open = true;
submitRelabel(f);
check('cleared label', [pend[0].k, pend[0].id, pend[0].v], ['label', 'A', '']);
pend.shift().done({ ok: false, error: 'bad store label: too long' });
check('relabel refusal', [f.msg.textContent, f.bt.disabled, toasts], ['bad store label: too long', false, []]);

process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerAdapterHiddenStores pins the page adapter's store list
// (dashboard.js tkReadAll): a hidden store stays out of the handle order
// and its view is not read, while hidden() lists it and info() holds
// every listed store's registry entry; a store that left the list loses
// its cached view, error, and seen marks.
func TestTrackerAdapterHiddenStores(t *testing.T) {
	block := dashboardJSBlock(t, "\n  function tkErrText(status, v) {", "\n  // tkReload re-reads")
	runTrackerNode(t, "adapter hidden stores", `
var tkApi = {}, tkOrder = [], tkHidden = [], tkInfo = {}, tkViews = {}, tkErrs = {}, tkSeen = {}, tkHostErr = null, tkSupport = false, tkLastRev = null;
var fetched = [], bad = [], tkSubs = [];
function wsT(k) { return k; }
function tkUrl(id, op) { return "/tracker/stores/" + id + "/" + op; }
function tkFetch(path, body, done) {
  fetched.push(path);
  if (path === "/tracker/stores") { done(200, { rev: 1, stores: [{ id: "vis", label: "V", remote: "r" }, { id: "hid", label: "", remote: "", hidden: true }] }); return; }
  done(200, { view: { v: path } });
}
`+block+`
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
tkViews.hid = { v: "old" }; tkErrs.gone = "x"; tkSeen.hid = { badge: {}, read: {} };
var done = 0;
tkReadAll(function () {}, function () { done++; });
check("order", tkOrder, ["vis"]);
check("hidden", tkHidden, ["hid"]);
check("info", [tkInfo.vis, tkInfo.hid], [{ id: "vis", label: "V", remote: "r", hidden: false }, { id: "hid", label: "", remote: "", hidden: true }]);
check("views read", fetched, ["/tracker/stores", "/tracker/stores/vis/view"]);
check("caches pruned", [Object.keys(tkViews), Object.keys(tkErrs), Object.keys(tkSeen)], [["vis"], ["vis"], ["vis"]]);
check("done", done, 1);
process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerAdapterRegistryWrites pins the adapter's registry writes
// (dashboard.js tkRegistryWrite): a write the daemon took answers after
// the reload that shows it; a refusal answers at once with the daemon's
// message and kind, and reloads nothing.
func TestTrackerAdapterRegistryWrites(t *testing.T) {
	block := dashboardJSBlock(t, "\n  function tkRegistryWrite(", "\n  var tkAdapter = {")
	runTrackerNode(t, "adapter registry writes", `
var posts = [], reloads = [], answers = [], bad = [], next = null;
function tkUrl(id, op) { return "/tracker/stores/" + id + "/" + op; }
function tkErrText(status, v) { return v.error ? String(v.error) : "HTTP " + status; }
function tkFetch(path, body, done) { posts.push([path, body]); done(next[0], next[1]); }
function tkReload(then) { reloads.push(then); }
`+block+`
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
next = [204, {}];
tkRegistryWrite("a", "unregister", { confirm: "a" }, function (r) { answers.push(r); });
check("post", posts, [["/tracker/stores/a/unregister", { confirm: "a" }]]);
check("no answer before the reload", answers, []);
reloads.shift()();
check("answer after the reload", answers, [{ ok: true }]);
next = [409, { error: "The store a was not unregistered: sync first.", kind: "unpushed" }];
answers = [];
tkRegistryWrite("a", "unregister", { confirm: "" }, function (r) { answers.push(r); });
check("refusal", [answers, reloads.length], [[{ ok: false, error: "The store a was not unregistered: sync first.", kind: "unpushed" }], 0]);
next = [200, { id: "a", label: "L" }];
answers = [];
tkRegistryWrite("a", "label", { label: "L" }, function (r) { answers.push(r); });
reloads.shift()();
check("relabel", answers, [{ ok: true }]);
process.stdout.write(bad.join("\n"));
`)
}

// TestTrackerSupportChatOfAGoneStoreDrops pins the support chat cache and
// the tab-label baseline (dashboard.js supRendered, tlTracker): a store no
// longer listed loses its chat (stream closed, root detached, entry
// deleted) and its notification baseline and waiting arrivals, while a
// listed store's are kept.
func TestTrackerSupportChatOfAGoneStoreDrops(t *testing.T) {
	sup := dashboardJSBlock(t, "\n  function supRendered(", "\n  // supLoad reads")
	tl := dashboardJSBlock(t, "\n  var tlBaseKeys = {}", "\n  function tlNotice(")
	runTrackerNode(t, "support chat of a gone store", `
var bad = [], closed = [], removed = [], tkOrder = ['B'], supH, supStore, supView, tlTk = 0, inputMode = 'enter';
var doc = { querySelector: function () { return null; }, hidden: false };
function supShown() { return false; }
function supClose(c) { closed.push(c.id); }
function supPop() {}
function tlRetitle() {}
function tlNotice() {}
function chat(id) { var root = { parentNode: { removeChild: function () { removed.push(id); root.parentNode = null; } } }; return { id: id, root: root, pop: false }; }
var supChats = { A: chat('A'), B: chat('B') };
`+sup+tl+`
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
supRendered(null, null, {});
check("chats kept", Object.keys(supChats), ["B"]);
check("streams closed", closed, ["A", "B"]);
check("root detached", removed, ["A"]);
tlBaseKeys = { A: { k: 1 }, B: { j: 1 } }; tlHidden = [{ store: 'A', key: 'k' }, { store: 'B', key: 'j' }];
tlTracker({ store: 'B', ready: true, entries: [{ key: 'j' }] });
check("baselines kept", Object.keys(tlBaseKeys), ["B"]);
check("waiting arrivals kept", tlHidden, [{ store: 'B', key: 'j' }]);
process.stdout.write(bad.join("\n"));
`)
}
