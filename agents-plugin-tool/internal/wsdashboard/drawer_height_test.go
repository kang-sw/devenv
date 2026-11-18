package wsdashboard

import "testing"

// The handle observer pads a shallower watched header. Coalesce native
// deliveries outside that callback, reading current widths even if the
// drawer closed or its regions changed before the frame runs.
func TestDrawerRoomResizeFrame(t *testing.T) {
	observer := dashboardJSBlock(t, "\n    if (window.ResizeObserver) {\n      var drRoomFrame = 0;", "\n    // The panels grid's column count")
	runTrackerNode(t, "drawer room resize frame", `
var bad = [], frames = [], deliveries, watched = [], calls = [], width = 178;
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
var window = { ResizeObserver: true }, drawer = { isConnected: true, open: true }, drhandle = {}, gitRegion = {};
function ResizeObserver(fn) { deliveries = fn; this.observe = function (n) { watched.push(n); }; }
function requestAnimationFrame(fn) { frames.push(fn); return frames.length; }
function drRoom() { calls.push(width); }
`+observer+`
check('both handles are watched', watched.length, 2);
deliveries(); deliveries(); deliveries();
check('a resize burst queues one frame without writing the header', [frames.length, calls], [1, []]);
width = 167; drawer.open = false;
frames[0]();
check('a close before delivery still reads the current handle width', calls, [167]);
deliveries();
check('the completed frame permits another resize', frames.length, 2);
drawer.isConnected = false;
frames[1]();
check('a detached drawer makes no header write', calls, [167]);
drawer.isConnected = true; width = 100;
deliveries(); frames[2]();
check('a skipped frame does not leave the queue armed', calls, [167, 100]);
process.stdout.write(bad.join("\n"));
`)
}

// TestDrawersUseFixedMaximum pins the shared read-only maximum, standalone
// viewport fallback, resize/display updates and active-handle CSS contract.
func TestDrawersUseFixedMaximum(t *testing.T) {
	runTrackerNode(t, "drawer fixed maximum", drawerFixedMaxNode)
}

// Also runnable without Go: node testdata/drawer-height.test.cjs
// The standalone runner executes these fixtures without spawning child processes.
const drawerFixedMaxNode = `
var fs = require('fs'), vm = require('vm'), bad = [];
function read(name) { return fs.readFileSync('assets/' + name, 'utf8'); }
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function funcs(src, names) {
  return names.map(function (name) {
    var re = new RegExp('\\n  function ' + name + '\\([^\\n]*\\{[^\\n]*\\}\\n|\\n  function ' + name + '\\([\\s\\S]*?\\n  \\}\\n');
    var m = src.match(re);
    if (!m) throw new Error('missing function ' + name);
    return m[0];
  }).join('\n');
}
function el() {
  var cls = {};
  return { hidden: false, offsetHeight: 34, style: { props: {}, setProperty: function (k, v) { this.props[k] = v; } },
    classList: { toggle: function (k, on) { cls[k] = !!on; }, contains: function (k) { return !!cls[k]; } }, setAttribute: function () {} };
}
var dash = read('dashboard-drawer.js'), render = read('tracker-render.js'), mount = read('tracker-mount.js');
var headerBottom = 40, composerTop = 800, composerVisible = true;
var header = { getBoundingClientRect: function () { return { bottom: headerBottom }; } };
var composer = { getBoundingClientRect: function () { return { top: composerTop }; } };
var win = { innerHeight: 1000 }, ctx = {
  window: win, drheader: header, drhandle: el(), drawer: el(), drsheet: el(), drclip: el(), drbody: el(), gbody: el(), sbody: el(), shandle: el(), gitRegion: el(),
  drMode: 'panels', drOpen: true, drShown: '', drTop: -500, tkApi: null,
  $: function (id) { return id === 'composer' ? composer : null; }, visible: function () { return composerVisible; },
  wsT: function (s) { return s; }, ghSync: function () {}, drTitle: function () {}, drFill: function () {},
  drFoldShow: function () {}, wtLoad: function () {}, hdLoad: function () {}, notesLoad: function () {}, gitOpened: function () {}
};
vm.createContext(ctx);
vm.runInContext(funcs(dash, ['drHeaderBottom', 'drMax', 'drApply']), ctx);
var tkdrawer = el(), tk = { window: win, O: { header: header, drawerHeight: ctx.drMax }, ui: { store: 'A', page: null },
  $: function () { return tkdrawer; } };
vm.createContext(tk);
vm.runInContext(funcs(render, ['fitDrawer']), tk);
function allHeights(label, want) {
  ['panels', 'git', 'search'].forEach(function (mode) {
    ctx.drMode = mode; ctx.drApply(false);
    check(label + ' ' + mode, [ctx.drsheet.style.height, ctx.drclip.style.height], [want + 'px', want + 'px']);
    check(label + ' active mode', ['panels', 'git', 'search'].filter(function (k) { return ctx.drawer.classList.contains(k); }), [mode]);
  });
  ['A', 'B'].forEach(function (store) { tk.ui.store = store; tk.fitDrawer(); check(label + ' store ' + store, tkdrawer.style.props['--tkh'], want + 'px'); });
}
check('host reads the header rather than stale drTop', ctx.drMax(), 720);
allHeights('composer clearance retains 34px + 6px', 720);
composerTop = 980;
allHeights('no vh cap', 900);
composerVisible = false;
allHeights('hidden composer uses viewport minus header', 960);
headerBottom = 100;
check('host maximum follows header before dashboard apply', ctx.drMax(), 900);
allHeights('header wraps', 900);
win.innerHeight = 700;
allHeights('viewport shrinks', 600);
composerVisible = true; composerTop = 125;
allHeights('no space never goes negative', 0);
composerTop = 650;
allHeights('composer reappears', 510);
ctx.drOpen = false; ctx.drApply(false);
check('closed clip remains collapsed', ctx.drclip.style.height, '0px');
delete tk.O.drawerHeight;
tk.fitDrawer(); check('standalone full viewport below header', tkdrawer.style.props['--tkh'], '600px');
headerBottom = 800; tk.fitDrawer(); check('standalone short viewport', tkdrawer.style.props['--tkh'], '0px');
headerBottom = -10; tk.fitDrawer(); check('standalone negative header bottom', tkdrawer.style.props['--tkh'], '700px');

// Exercise the actual mount observer without a browser or a window resize.
var start = mount.indexOf('    if (window.ResizeObserver) {'), end = mount.indexOf("    window.addEventListener('scroll', wkFramePlace", start);
if (start < 0 || end < 0) throw new Error('missing tracker boundary observer');
var frames = [], watched = [], delivery, measures = 0, placements = 0;
tk.window.ResizeObserver = true;
tk.O.drawerHeight = ctx.drMax; tk.O.drawerBoundary = composer;
tk.ResizeObserver = function (fn) { delivery = fn; this.observe = function (n) { watched.push(n); }; };
tk.requestAnimationFrame = function (fn) { frames.push(fn); return frames.length; };
tk.measure = function () { measures++; };
tk.wkFramePlace = function () { placements++; };
vm.runInContext(mount.slice(start, end), tk);
check('header and composer observed', [watched[0] === header, watched[1] === composer], [true, true]);
headerBottom = 40; composerTop = 650;
delivery(); delivery(); check('observer coalesces callbacks outside delivery', [frames.length, measures], [1, 0]);
frames[0](); check('observer refits after header change', [tkdrawer.style.props['--tkh'], measures, placements], ['570px', 1, 1]);
composerVisible = false; delivery(); frames[1]();
check('observer refits after display none', tkdrawer.style.props['--tkh'], '660px');
composerVisible = true; composerTop = 600; delivery(); frames[2]();
check('observer refits after composer resize/show', tkdrawer.style.props['--tkh'], '520px');
tk.ui.page = 'A'; composerTop = 500; delivery(); frames[3]();
check('store page does not refit hidden drawer', tkdrawer.style.props['--tkh'], '520px');

function cssContract(src, selector, label) {
  var rules = src.replace(/\/\*[\s\S]*?\*\//g, '').match(/[^{}]+\{[^{}]*\}/g) || [];
  var rule = rules.filter(function (r) { return r.slice(0, r.indexOf('{')).split(',').some(function (s) { return s.trim() === selector; }); })[0] || '';
  check(label + ' hidden but measurable', /visibility:\s*hidden/.test(rule) && /pointer-events:\s*none/.test(rule) && !/display:\s*none/.test(rule), true);
}
var css = read('tracker.css'), dashboardCSS = read('dashboard-drawer.css');
cssContract(css, '.tk .tk-handle.open', 'active tracker opener');
['.drawer.open.panels #drhandle', '.drawer.open.git #r-git', '.drawer.open.search #shandle'].forEach(function (s) { cssContract(dashboardCSS, s, s); });
check('tracker clip uses maximum directly', /\.tk\.tk-drawer\.open \.tk-dr-clip\s*\{\s*height:\s*var\(--tkh\)/.test(css), true);
check('tracker has no drawer drag CSS', /--tkext|\.tk-drawer\.drag/.test(css), false);
check('tracker no longer loads or writes a height or wires handle drags', /drawer\.height|ui\.drh|drDrag|drSetHeight|drClamp/.test(render + mount + read('tracker.js')), false);
check('dashboard no longer persists or drags a height', /\bdrH\b|\bDR_H\b|drClamp|drHeight|drDragged|drDrag|drSetH/.test(dash), false);
var adapter = read('dashboard-tracker.js');
check('dashboard passes read-only maximum and composer boundary', /drawerHeight:\s*drMax,\s*drawerBoundary:\s*\$\("composer"\)/.test(adapter), true);
process.stdout.write(bad.join('\n'));
if (bad.length) process.exitCode = 1;
`

// TestDrawerFoldButtons pins the fold button in the hidden active handle's
// slot. It shows only for the open sheet, takes focus from a keyboard-opened
// opener, closes with the click animation, and restores keyboard focus to
// its opener. Tracker fold buttons never show on the store page, and closing
// preserves the handle's signals.
func TestDrawerFoldButtons(t *testing.T) {
	fold := dashboardJSBlock(t, "\n  function drFoldShow(mode) {", "\n  function drTitle() {")
	apply := dashboardJSBlock(t, "\n  function drApply(anim) {", "\n  // ghSync marks")
	setOpen := dashboardJSBlock(t, "\n  function drSetOpen(open, mode) {", "\n  if (drawer) {")
	handles := trackerJSBlock(t, "\n  var handleSig = null;", "\n  // The item window makes")
	toggle := trackerJSBlock(t, "\n  function collapse() {", "\n  // openItemWindow(storeId")
	runTrackerNode(t, "drawer fold buttons", `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function setTimeout(fn) { fn(); }
var window = { innerHeight: 1000 };
function el(id) {
  var cls = {};
  return { id: id, hidden: false, attrs: {}, style: { props: {}, setProperty: function (k, v) { this.props[k] = v; } },
    classList: { add: function (c) { cls[c] = true; }, remove: function (c) { delete cls[c]; }, toggle: function (c, on) { if (on) cls[c] = true; else delete cls[c]; }, contains: function (c) { return !!cls[c]; } },
    setAttribute: function (k, v) { this.attrs[k] = v; },
    contains: function () { return false; }, focus: function () { doc.activeElement = this; } };
}
// The dashboard's drawer, the frame stubbed.
var doc = { activeElement: null, addEventListener: function () {} };
var drawer = el('drawer'), drbody = el('drbody'), gbody = el('gbody'), sbody = el('sbody'), shandle = el('shandle'), drsheet = el('drsheet'), drclip = el('drclip');
var drhandle = el('drhandle'), gitRegion = el('r-git'), ghandleEl = el('ghandle'), drfold = el('drfold');
var drheader = {getBoundingClientRect:function(){return {bottom:40};}};
var drOpen = false, drMode = 'panels', drWant = false, drTop = 40, drShown = '', drHas = true, drSwap = false, DR_OPEN = 'k', DR_SEARCH = 'search', tkApi = null;
function ghandle() { return ghandleEl; }
function drMax() { return 600; }
function ghSync() {} function drTitle() {} function drFill() {} function wtLoad() {} function hdLoad() {} function notesLoad() {} function gitOpened() {} function drDot() {} function drAnnounce() {}
var stored = {};
function save(k, v) { stored[k] = v; }
function wsT(k) { return k; }
`+dashboardFuncs(t, "drHeaderBottom")+fold+apply+setOpen+`
function shown() { return drfold.hidden ? '' : drfold.classList.contains('git') ? 'git' : drfold.classList.contains('search') ? 'search' : 'panels'; }

drApply(false);
check('a closed drawer has no fold button', shown(), '');
drSetOpen(true, 'panels');
check('the panels sheet: the fold button in the panels slot', shown(), 'panels');
check('the panels sheet fills the maximum below the header', [drsheet.style.height, drclip.style.height, drawer.style.props['--drtop']], ['600px', '600px', '40px']);
check('panels keeps its open state', [stored[DR_OPEN], stored[DR_SEARCH]], ['1', null]);
drSetOpen(true, 'git');
check('the git sheet: the fold button in the git slot', shown(), 'git');

drSetOpen(true, 'search');
check('search shows only its sheet and fold slot', [shown(), drbody.hidden, gbody.hidden, sbody.hidden], ['search', true, true, false]);
check('search persists independently', [stored[DR_OPEN], stored[DR_SEARCH]], [null, '1']);
doc.activeElement = drfold;
drFold();
check('search fold closes and restores focus', [drOpen, shown(), doc.activeElement.id, stored[DR_SEARCH]], [false, '', 'shandle', null]);
drSetOpen(true, 'git');

// A click on the fold button closes as the handle's click does, animated,
// and keyboard focus goes to the shown sheet's handle.
doc.activeElement = drfold;
drFold();
check('the fold closes the git sheet', [drOpen, drawer.classList.contains('open'), drawer.classList.contains('anim'), shown()], [false, false, true, '']);
check('focus goes to the git handle', doc.activeElement && doc.activeElement.id, 'ghandle');
drSetOpen(true, 'panels');
doc.activeElement = drfold;
drFold();
check('the fold closes the panels sheet', [drOpen, shown()], [false, '']);
check('focus goes to the panels handle', doc.activeElement && doc.activeElement.id, 'drhandle');
drFold();
check('a fold click on a closed drawer does nothing', drOpen, false);

// A keyboard open hides the focused opener: focus moves to the fold button
// in its slot, not to the body, and the fold hands it back.
[['panels', drhandle], ['git', ghandleEl], ['search', shandle]].forEach(function (c) {
  doc.activeElement = c[1];
  drSetOpen(true, c[0]);
  check(c[0] + ' keyboard open focuses the fold button', [shown(), doc.activeElement && doc.activeElement.id], [c[0], 'drfold']);
  drFold();
  check(c[0] + ' fold hands focus back to its opener', doc.activeElement && doc.activeElement.id, c[1].id);
});

// The tracker's store handles: each in a slot with its fold button.
(function () {
  var nodes = {}, ui = { store: 'A', open: null, page: null }, calls = [];
  var document = { activeElement: null, dispatchEvent: function (e) { calls.push(e.type); } };
  function CustomEvent(type) { this.type = type; }
  function node(id) {
    if (!nodes[id]) nodes[id] = { id: id, hidden: false, innerHTML: '', attrs: {}, className: '', title: '', textContent: '',
      setAttribute: function (k, v) { this.attrs[k] = v; }, focus: function () { document.activeElement = this; },
      querySelector: function (s) { return node(id + ' ' + s); } };
    return nodes[id];
  }
  function $(id) { return id === 'tkhandles' || /^tk[hfb]/.test(id) ? node(id) : null; }
  function esc(s) { return String(s); }
  function attr(k, v) { return ' data-' + k + '="' + esc(v) + '"'; }
  function wsT(k) { return k; }
  function newKeys() { return ['unread']; }
  function storeTitle(st) { return st.name; }
  function syncNow(n) { calls.push('sync ' + n); }
  function bringUp() {}
  function keepItemDraft() {}
  function render() { renderHandles(); }
  var order = [], vss = {}, AD = { stores: function () { return ['A', 'B']; }, error: function () { return ''; } };
`+handles+toggle+`
  syncHandles();
  var row = $('tkhandles').innerHTML;
  if (!/<span class="tk-slot"><button type="button" class="tk-fold" id="tkf-A" data-fold="A" [^>]*hidden>.*?<\/button><button class="tk-handle" id="tkh-A"/.test(row)) bad.push('store A has no fold button in its slot: ' + row);
  if (row.indexOf('aria-label="js.tk.handle.fold"') < 0) bad.push('the fold button has no label: ' + row);
  renderHandles();
  check('closed stores: no fold button', [$('tkf-A').hidden, $('tkf-B').hidden], [true, true]);
  openStore('A');
  check('store A open: its fold button only', [$('tkf-A').hidden, $('tkf-B').hidden], [false, true]);
  check('only the active store handle gets the hidden-open CSS class', [$('tkh-A').className, $('tkh-B').className], ['tk-handle on open', 'tk-handle']);
  ui.page = 'A'; renderHandles();
  check('the store page: no fold button', $('tkf-A').hidden, true);
  ui.page = null; renderHandles();
  foldStore('B');
  check('a fold click on a closed store does nothing', ui.open, 'A');
  document.activeElement = $('tkf-A');
  foldStore('A');
  check('the fold closes the drawer through the collapse', [ui.open, $('tkf-A').hidden, calls.indexOf('sync A') >= 0], [null, true, true]);
  check('focus goes to the store handle', document.activeElement.id, 'tkh-A');
  check('closed handle keeps selection, unread badge and expansion meaning', [$('tkh-A').className, $('tkh-A').attrs['aria-expanded'], $('tkh-A').querySelector('.tk-badge').hidden, $('tkh-A').querySelector('.tk-badge').textContent], ['tk-handle on', 'false', false, 1]);
  openStore('B');
  check('another store opens without hiding the selected closed opener', [$('tkh-A').className, $('tkh-B').className], ['tk-handle', 'tk-handle on open']);
  document.activeElement = $('tkh-A');
  openStore('A');
  check('a keyboard open focuses the store fold button, not the body', [ui.open, document.activeElement.id], ['A', 'tkf-A']);
})();

process.stdout.write(bad.join("\n"));
`)
}
