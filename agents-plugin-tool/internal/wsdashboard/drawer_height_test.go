package wsdashboard

import "testing"

// TestDrawersShareOneHeight pins the one drawer height (dashboard.js drH,
// drSetH, the drawer drag; tracker.js drHeight, fitDrawer, the store
// handle drag): a height never set is DR_DEF (320px) for the session
// drawer and every tracker store, the content's height playing no part;
// dragging the git handle opens the git sheet in the panels' place and
// sets the one height; a store handle's drag sets the height the panels
// drawer and every other store open at, through the host's drawerHeight
// option; without that option the renderer keeps one height for all its
// stores, not one per store.
func TestDrawersShareOneHeight(t *testing.T) {
	vars := dashboardJSBlock(t, "\n  var DR_OPEN = \"ws.dashboard.drawer\"", "\n  // ghandle is the git handle now")
	clamp := dashboardJSBlock(t, "\n  function drClamp(h) {", "\n  // drApply lays the drawer out")
	drag := dashboardJSBlock(t, "\n    var drDrag = null, drDragged = false;\n    function drDragStart(e) {", "\n    drhandle.addEventListener(\"click\"")
	tk := trackerJSBlock(t, "\n  var DR_MIN = 160, DR_DEF = 320, DR_SHUT = 80;", "\n  // One handle per store")
	runTrackerNode(t, "drawer height", `
var bad = [], stored = {};
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function setTimeout(fn) { fn(); }
var window = { innerHeight: 1000 };
function el(id) {
  var cls = {};
  return { id: id, style: { props: {}, setProperty: function (k, v) { this.props[k] = v; }, removeProperty: function (k) { delete this.props[k]; } },
    classList: { add: function (c) { cls[c] = true; }, remove: function (c) { delete cls[c]; }, toggle: function (c, on) { if (on) cls[c] = true; else delete cls[c]; } },
    top: 0, getBoundingClientRect: function () { return { top: this.top, bottom: this.top + 34 }; }, setPointerCapture: function () {} };
}
function press(target, y) { return { target: { closest: function () { return target; } }, clientY: y, button: 0, pointerId: 1 }; }

// The tracker renderer in its own scope: it has its own drClamp and DR_* names.
function tracker(O, saved) {
  var ui = { store: 'A', open: null, drh: saved || 0 }, dsaves = [], tkdrawer = el('tkdrawer');
  function $(id) { return id === 'tkdrawer' ? tkdrawer : null; }
  function dsSave(k, m) { dsaves.push([k, m]); }
  function openStore(n) { ui.open = n; ui.store = n; }
  function collapse() { ui.open = null; }
  O.header = { getBoundingClientRect: function () { return { bottom: 40 }; } };
`+tk+`
  return {
    ui: ui, dsaves: dsaves,
    shown: function (store) { ui.store = store; fitDrawer(); return tkdrawer.style.props['--tkh']; },
    drag: function (store, to) {
      var h = el('tkh-' + store); h.getAttribute = function () { return store; };
      drDragStart(press(h, 0)); drDragMove({ clientY: 3 }); drDragMove({ clientY: to }); drDragEnd();
    }
  };
}

// The dashboard's drawer: its variables, height, and drag, with the frame stubbed.
var loadVal = null;
function load(k) { return k === 'ws.dashboard.drawer.h' ? loadVal : null; }
function save(k, v) { stored[k] = v; }
`+vars+`
check('never-set dashboard height', drH, 320);
`+clamp+`
function drMax() { return 600; }
var drawer = el('drawer'), drsheet = el('drsheet'), drclip = el('drclip'), drhandle = el('drhandle'), gitRegion = el('r-git');
var ghandleEl = el('ghandle'), applied = [], announced = 0, closed = 0;
function drMeasure() { drTop = 40; }
function drAnnounce() { announced++; }
function drApply() { applied.push(drMode + ':' + drOpen + ':' + drH); }
function drSetOpen(open) { if (!open) { drOpen = false; closed++; } }
function drDot() {}
var doc = { addEventListener: function () {} };
`+drag+`

// A never-set height is DR_DEF in every store, host option or not.
var host = tracker({ drawerHeight: { get: function () { return 0; }, set: function () { bad.push('set on a show'); } } });
check('never-set store A (host)', host.shown('A'), '320px');
check('never-set store B (host)', host.shown('B'), '320px');
var own = tracker({});
check('never-set store (no host)', own.shown('A'), '320px');

// The git handle drags: past the threshold it opens the git sheet in the
// panels' place, rides its region, and the release sets the one height.
drOpen = true; drMode = 'panels'; drWant = true;
drDragStart(press(ghandleEl, 10));
drDragMove({ clientY: 12 });
check('a move under the threshold is no drag', applied.length, 0);
drDragMove({ clientY: 300 });
check('git drag brings the git sheet', drMode, 'git');
check('git drag moves the git region', gitRegion.style.top, '290px');
check('git drag leaves the panels handle', drhandle.style.top, undefined);
drEnd();
check('git drag sets the height', [drH, stored['ws.dashboard.drawer.h']], [250, '250']);
check('git drag leaves the git sheet open', [drMode, drOpen, drWant, stored['ws.dashboard.drawer']], ['git', true, false, null]);
check('a drag is no click', drDragged, false);

// A press on the git handle that never moves stays a click.
applied = [];
drDragStart(press(ghandleEl, 10)); drEnd();
check('a press stays a click', [applied.length, drH], [0, 250]);

// The panels handle over the open git sheet brings the panels; a release
// short of DR_SHUT closes.
drDragStart(press(drhandle, 0));
drDragMove({ clientY: 100 });
check('panels drag brings the panels', drMode, 'panels');
drDragMove({ clientY: 50 });
drEnd();
check('a short release closes', [closed, drH], [1, 250]);

// A store handle's drag sets the host's one height: the panels drawer and
// every other store open at it.
host = tracker({ drawerHeight: { get: function () { return drH; }, set: drSetH } });
check('store B opens at the dashboard height', host.shown('B'), '250px');
host.drag('A', 440);
check('store drag sets the dashboard height', [drH, stored['ws.dashboard.drawer.h']], [400, '400']);
check('store drag keeps no own height', host.dsaves, []);
check('store B opens at the dragged height', host.shown('B'), '400px');
drOpen = false; drMode = 'panels'; applied = [];
drDragStart(press(drhandle, 0)); drDragMove({ clientY: 3 }); drDragMove({ clientY: 440 }); drEnd();
check('panels drag after a store drag', drH, 400);

// Without the host option the renderer keeps one height for all stores.
own = tracker({});
own.drag('A', 340);
check('own height stored once', own.dsaves, [['drawer.height', { h: 300 }]]);
check('own height in store B', own.shown('B'), '300px');
check('a saved own height', tracker({}, 280).shown('C'), '280px');

// The dashboard reads its stored height at load.
loadVal = '275';
(function () {
`+vars+`
check('stored dashboard height', drH, 275);
})();

process.stdout.write(bad.join("\n"));
`)
}
