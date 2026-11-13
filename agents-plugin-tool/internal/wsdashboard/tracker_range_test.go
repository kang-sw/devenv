package wsdashboard

import (
	"strings"
	"testing"
)

// rangeHarness is the shared node scaffold for the range picker cases: a
// fake menu, capsule, and inputs, a recording adapter, and the real menu
// close, apply, and discard functions over them.
func rangeHarness(t *testing.T) string {
	t.Helper()
	fns := ""
	for _, f := range []struct {
		open string
		one  bool
	}{
		{`function monAdd\(m, n\)`, true}, {`function rangeBad\(r\)`, true}, {`function rangeOf\(m\)`, false}, {`function rangeFit\(r\)`, false},
		{`function rangeAct\(k, a\)`, false}, {`function rangeInput\(t\)`, false}, {`function rangeSet\(r, side, day\)`, false}, {`function rangeEdge\(r, side, dir\)`, false},
		{`function rangePending\(m\)`, false}, {`function rangeRefused\(\)`, false}, {`function menuApplyClose\(refocus\)`, false},
		{`function menuDiscard\(\)`, false}, {`function menuHas\(t\)`, true}, {`function capOn\(\)`, true}, {`function menuOutside\(e, t\)`, false},
		{`function menuFocusOut\(e\)`, false}, {`function closeMenu\(refocus, quiet\)`, false}, {`function menuBack\(\)`, false}, {`function openMenu\(o, anchor, pt\)`, false},
		{`function wrote\(results\)`, false}, {`function commit\(events, then, prep, fail\)`, false}, {`function menuKey\(e\)`, false}, {`function nd\(\)`, true},
		{`function labelOf\(id\)`, true}, {`function labelsOver\(cur, set\)`, false}, {`function labelFlip\(set, cur, lid\)`, false}, {`function sameLabels\(a, b\)`, false},
		{`function labelPending\(m\)`, false}, {`var LPEND = \{\};`, true}, {`function labelBase\(it\)`, true}, {`function labelWrite\(ev, then\)`, false}, {`function labelPicks\(from, to\)`, false},
	} {
		fns += trackerFn(t, f.open, f.one)
	}
	return trackerCheckJS + `
var VS = { now: '2026-11-13T09:00:00Z', labels: [{ id: 'L1' }, { id: 'L2' }],
  items: { i1: { id: 'i1', title: 'T', start: '', due: '2026-11-19', labels: ['L1'] }, i2: { id: 'i2', title: 'U', due: '', labels: [] } } };
var ui = { store: 's1', nrow: {}, ned: null }, MN = null, editor = true, sent = [], toasts = [], refuse = null, draws = 0, drawn = [], renders = 0;
function canEdit() { return editor; }
function today() { return String(VS.now).slice(0, 10); }
function wsT(k) { return k; }
function esc(s) { return s; }
function toast(m) { toasts.push(m); }
function render() { renders++; }
function rangeDraw() { draws++; }
function drawMenu() { drawn.push(MN.step); }
function placeMenu() {}
function hideTip() {}
function labelStepRedraw() {}
function returnSel() { return '#opener'; }
function closeDialog() {}
var AD = { dispatch: function (store, ev, cb) { sent.push(ev.map(function (e) { return [e.type, e.payload]; })); cb(refuse ? { ok: false, error: refuse } : { ok: true, results: [] }); } };
// fake DOM: a node is inside the menu or the capsule when it says so
function node(o) {
  o = o || {};
  var n = { id: o.id || '', inMenu: !!o.inMenu, inCap: !!o.inCap, attrs: o.attrs || {}, value: o.value || '', type: o.type || '', focused: 0, offsetParent: {}, disabled: false,
    focus: function () { this.focused++; focusedEl = this; }, getAttribute: function (k) { return k in this.attrs ? this.attrs[k] : null; },
    hasAttribute: function (k) { return k in this.attrs; }, setAttribute: function (k, v) { this.attrs[k] = v; }, removeAttribute: function (k) { delete this.attrs[k]; },
    matches: function (q) { return !!o.field && /input/.test(q); }, closest: function (q) { return (/#tkmenu/.test(q) && this.inMenu) || (/#tkcap/.test(q) && this.inCap) ? this : null; },
    contains: function (x) { return x === this; }, getBoundingClientRect: function () { return { left: 0, top: 0, right: 10, bottom: 10, width: 10, height: 10 }; }, classList: { contains: function () { return false; }, add: function () {}, remove: function () {}, toggle: function () {} } };
  return n;
}
var focusedEl = null, menuCtl = [node({ inMenu: true }), node({ inMenu: true })];
var els = { tkmenu: node({ id: 'tkmenu' }), tkcap: node({ id: 'tkcap', inCap: true }), tkwin: node({ id: 'tkwin' }),
  tkrs: node({ id: 'tkrs', inMenu: true, field: true, type: 'date', attrs: { 'data-rin': 's' } }), tkrd: node({ id: 'tkrd', inMenu: true, field: true, type: 'date', attrs: { 'data-rin': 'd' } }) };
els.tkmenu.contains = function (x) { return !!x && !!x.inMenu; };
els.tkmenu.querySelectorAll = function (q) { return /data-mi/.test(q) && !/button/.test(q) ? [] : menuCtl.concat([els.tkrs, els.tkrd]); };
els.tkcap.contains = function (x) { return !!x && !!x.inCap; };
function $(id) { return els[id] || null; }
var document = { body: { classList: { add: function () {}, remove: function () {} } }, querySelector: function () { return opener; } };
var opener = node({ id: 'opener', attrs: { 'aria-haspopup': 'menu' } }); opener.isConnected = true; opener.matches = function () { return true; };
var CSS = { escape: function (s) { return s; } };
function ev(o) { var e = { prevented: 0, stopped: 0, preventDefault: function () { this.prevented++; }, stopPropagation: function () { this.stopped++; } }; Object.keys(o || {}).forEach(function (k) { e[k] = o[k]; }); return e; }
// the item menu's due step on item id: opened from the main menu (from 'main') or from a row badge (from null)
function picker(id, from) { MN = { kind: 'item', item: id, step: 'due', from: from || null, backEl: opener, back: '#opener', q: '' }; rangeOf(MN); sent = []; toasts = []; draws = 0; drawn = []; return MN.rg; }
// a chip's picker over the new row's draft
function chipPicker(n) { ui.nrow.s1 = n; MN = { kind: 'new', ed: false, step: 'due', from: null, backEl: opener, back: '#opener', q: '' }; rangeOf(MN); sent = []; renders = 0; return MN.rg; }
` + fns
}

// TestTrackerRangePickerAppliesOnClose pins the range picker's close
// semantics (tracker.js closeMenu, menuApplyClose, menuDiscard, menuOutside,
// menuFocusOut, menuKey): every close applies the pick once, as one item.edit
// carrying only the ends that changed (never an unchanged due beside a
// changed start), or into a row's draft; Esc and the discard capsule drop it
// and write nothing; an inverted pair is refused by every close that can stay
// open (the picker stays, focus goes to the offending input, the click that
// tried is spent) and dropped by one that cannot (resize, scroll, stale); a
// gone item or edit right writes nothing; a late write keeps the picker open.
func TestTrackerRangePickerAppliesOnClose(t *testing.T) {
	runTrackerCases(t, "range picker close", rangeHarness(t)+`
var r = picker('i1');
check('the picker opens on the item\'s ends, one panel on the due\'s month', [r.s, r.d, r.lm, r.rm, r.err], ['', '2026-11-19', '2026-11', null, false]);
check('closing with nothing changed writes nothing', [closeMenu(false), sent, MN], [undefined, [], null]);

// a changed start alone: one item.edit with start only
r = picker('i1'); r.s = '2026-11-10';
check('an outside click applies once and is not spent', [menuOutside(ev(), node()), sent], [false, [[['item.edit', { item: 'i1', start: '2026-11-10' }]]]]);
check('the menu closed', MN, null);
r = picker('i1'); r.s = '2026-11-10'; r.d = '2026-11-21';
var e1 = ev({ key: 'Enter', target: els.tkrd });
menuKey(e1);
check('Enter in an input applies both ends in one item.edit', [sent, MN, e1.prevented], [[[['item.edit', { item: 'i1', start: '2026-11-10', due: '2026-11-21' }]]], null, 1]);
r = picker('i1'); r.d = '';
menuFocusOut({ relatedTarget: node() });
check('focus leaving for outside applies, a cleared due as null', [sent, MN], [[[['item.edit', { item: 'i1', due: null }]]], null]);
r = picker('i1'); r.s = '2026-11-02';
closeMenu(false);
check('a resize or scroll close (closeMenu) applies a valid pick once', sent, [[['item.edit', { item: 'i1', start: '2026-11-02' }]]]);

// an end another viewer wrote while the picker was open, which the user left alone, is kept
r = picker('i1'); r.s = '2026-11-05'; VS.items.i1.due = '2026-11-25';
closeMenu(false);
check('an untouched due another viewer moved is not written back', sent, [[['item.edit', { item: 'i1', start: '2026-11-05' }]]]);
VS.items.i1.due = '2026-11-19';
r = picker('i1'); r.s = '2026-11-05'; VS.items.i1.start = '2026-11-05';
closeMenu(false);
check('an end picked to what the item already holds is no write', sent, []);
VS.items.i1.start = '';

// Esc and the capsule discard
r = picker('i1'); r.s = '2026-11-10';
var e2 = ev({ key: 'Escape', target: els.tkrs });
menuKey(e2);
check('Esc discards: no write, the menu closes, the key stops there', [sent, MN, e2.stopped], [[], null, 1]);
r = picker('i1', 'main'); r.s = '2026-11-10';
menuKey(ev({ key: 'Escape', target: els.tkcap }));
check('Esc on the capsule of a sub-step: no write, back to the main step', [sent, MN && MN.step, MN && MN.rg, drawn], [[], 'main', null, ['main']]);
r = picker('i1'); r.s = '2026-11-10';
menuDiscard();
check('the capsule discards: no write, the menu closes', [sent, MN], [[], null]);

// the gone item and the gone edit right
r = picker('i1'); r.s = '2026-11-10'; editor = false;
closeMenu(false);
check('without the edit right nothing is written', sent, []);
editor = true;
r = picker('i1'); r.s = '2026-11-10'; var keep = VS.items.i1; delete VS.items.i1;
closeMenu(false);
check('a gone item writes nothing', sent, []);
VS.items.i1 = keep;

// an inverted pick is refused by the closes that can stay open
r = picker('i1'); r.s = '2026-11-25'; r.last = 's'; r.err = rangeBad(r); els.tkrs.focused = 0;
var e3 = ev();
check('an outside click is refused and spent', [menuOutside(e3, node()), e3.prevented, sent, MN && MN.step, r.err, els.tkrs.focused], [true, 1, [], 'due', true, 1]);
var chip = node({ attrs: { 'data-act': 'nchip' } });
check('a click on another item\'s chip is spent, the pick kept', [menuOutside(ev(), chip), MN.rg.s, sent], [true, '2026-11-25', []]);
menuKey(ev({ key: 'Enter', target: els.tkrs }));
check('Enter is refused', [MN && MN.step, sent], ['due', []]);
menuFocusOut({ relatedTarget: node() });
check('focus loss is refused and focus goes back to the input', [MN && MN.step, sent, focusedEl === els.tkrs], ['due', [], true]);
closeMenu(false);
check('a close that cannot stay open drops the inverted pick unwritten', [MN, sent], [null, []]);
VS.items.i1.start = '2026-11-25';
r = picker('i1');
check('a folded inverted pair opens with the error', [r.err, r.lm, r.rm], [true, '2026-11', null]);
check('closing it without a fix is refused', [menuApplyClose(false), MN && MN.step], [false, 'due']);
menuDiscard();
check('Esc or the capsule leave it unwritten', [MN, sent], [null, []]);
VS.items.i1.start = '';

// the back row of the sub-step and ArrowLeft on a non-field control
r = picker('i1', 'main'); r.s = '2026-11-10';
var back = node({ inMenu: true });
menuKey(ev({ key: 'ArrowLeft', target: back }));
check('ArrowLeft applies a valid pick; the written batch closes the item menu', [sent, MN], [[[['item.edit', { item: 'i1', start: '2026-11-10' }]]], null]);
r = picker('i1', 'main'); r.s = '2026-11-25'; r.last = 's';
menuKey(ev({ key: 'ArrowLeft', target: back }));
check('ArrowLeft on an inverted pick is refused', [sent, MN && MN.step, r.err], [[], 'due', true]);
r.s = ''; r.err = false;
menuBack();
check('the back row with nothing changed steps back to the main menu', [sent, MN.step, drawn], [[], 'main', ['main']]);
refuse = 'start 2026-11-21 is after due 2026-11-19';
r = picker('i1', 'main'); r.s = '2026-11-21'; r.d0 = r.d = '2026-11-30'; VS.items.i1.due = '2026-11-30';
menuBack();
check('a refused back-row write toasts and keeps the picker', [toasts, MN && MN.step, MN && MN.rsent], [['start 2026-11-21 is after due 2026-11-19'], 'due', false]);
refuse = null; VS.items.i1.due = '2026-11-19'; MN = null;

// paging and the capsule are inside: they apply nothing
r = picker('i1'); r.s = '2026-11-10';
rangeAct('m-rpage', node({ attrs: { 'data-side': 'l', 'data-dir': '1' } }));
check('a month arrow pages the panel and writes nothing', [r.lm, sent, MN && MN.step], ['2026-12', [], 'due']);
menuFocusOut({ relatedTarget: null });
check('a redraw that drops focus is not a focus loss', [MN && MN.step, sent], ['due', []]);
menuFocusOut({ relatedTarget: els.tkrd });
check('focus moving to an input stays inside', MN && MN.step, 'due');
menuFocusOut({ relatedTarget: els.tkcap });
check('tabbing to the capsule applies nothing', [MN && MN.step, sent], ['due', []]);
check('a click on the capsule is not an outside click', [menuOutside(ev(), els.tkcap), MN && MN.step, sent], [false, 'due', []]);
var e4 = ev({ key: 'Tab', target: els.tkcap });
menuKey(e4);
check('Tab from the capsule cycles back into the menu and applies nothing', [focusedEl === menuCtl[0], e4.prevented, MN && MN.step, sent], [true, 1, 'due', []]);
var e5 = ev({ key: 'Tab', target: els.tkrd });
els.tkcap.hidden = false; menuKey(e5);
check('Tab from the last control reaches the capsule', focusedEl === els.tkcap, true);

// a late write (a label write from an earlier close landing) keeps the range picker open
wrote([]);
check('a written batch does not close the range picker mid-pick', [MN && MN.step, MN && MN.rg.s], ['due', '2026-11-10']);
MN = null;

// a menu opened over the picker applies it first, or is refused
r = picker('i1'); r.s = '2026-11-10';
openMenu({ kind: 'item', item: 'i2', step: 'main' }, opener);
check('a menu opened over the picker applies the pick first', [sent, MN.item], [[[['item.edit', { item: 'i1', start: '2026-11-10' }]]], 'i2']);
r = picker('i1'); r.s = '2026-11-25'; r.last = 's';
openMenu({ kind: 'item', item: 'i2', step: 'main' }, opener);
check('an inverted pick keeps the picker over a menu opened over it', [MN.item, MN.rg.s, sent], ['i1', '2026-11-25', []]);
MN = null;

// the row's draft: a close changes only the draft (the write rides the row's save)
var n = { val: 'x', kind: 'task', who: ['me'], start: '', due: '2026-11-19', labels: [] };
r = chipPicker(n); r.s = '2026-11-10';
closeMenu(false);
check('a chip picker\'s close sets the draft and writes nothing', [n.start, n.due, sent, renders, MN], ['2026-11-10', '2026-11-19', [], 1, null]);
r = chipPicker(n); r.d = '';
menuOutside(ev(), node());
check('an outside click applies a cleared due to the draft', [n.start, n.due, sent], ['2026-11-10', '', []]);
r = chipPicker(n); r.d = '2026-11-12'; r.s = '2026-11-20'; r.last = 's';
check('an inverted draft pick is refused', [menuApplyClose(false), MN && MN.step, n.start], [false, 'due', '2026-11-10']);
menuKey(ev({ key: 'Escape', target: els.tkrd }));
check('Esc leaves the draft as it was', [n.start, n.due, MN], ['2026-11-10', '', null]);
r = chipPicker(n); r.s = '2026-11-01';
closeMenu(false, true);
check('a close from inside a render applies to the draft without rendering again', [n.start, renders], ['2026-11-01', 0]);
done();
`)
	click := trackerBlock(t, `document\.addEventListener\('click', function \(e\) \{`, `\n    document\.addEventListener\('submit'`)
	if !strings.Contains(click, "if (menuOutside(e, t)) return;") {
		t.Error("the document click handler does not route outside clicks through menuOutside")
	}
	keys := trackerBlock(t, `lastInput = 'key';`, `\n      // Shift\+F10`)
	if !strings.Contains(keys, "if (MN && menuHas(t)) { menuKey(e); return; }") {
		t.Error("keys on the capsule are not routed to menuKey")
	}
	listen := trackerFn(t, `function listen\(\)`, false)
	for _, want := range []string{
		"$('tkmenu').addEventListener('focusout', menuFocusOut);",
		"$('tkcap').addEventListener('focusout', menuFocusOut);",
		"$('tkcap').addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); menuDiscard(); });",
		"$('tkcap').addEventListener('mousedown', function (e) { e.preventDefault(); });",
		"window.addEventListener('resize', function () { if (MN) closeMenu(false);",
	} {
		if !strings.Contains(listen, want) {
			t.Errorf("listen lacks %q", want)
		}
	}
}

// TestTrackerRangeDrawFollowsDrag pins that a drag or a clear redraws a
// focused input too (tracker.js rangeDraw): only the input being typed in
// (rangeInput's source) keeps its own text, so the field the picker opened
// focused never shows a stale end after a handle drag.
func TestTrackerRangeDrawFollowsDrag(t *testing.T) {
	if !strings.Contains(trackerFn(t, `function rangeDraw\(src\)`, false), "if (i !== src && i.value !== r[side]) i.value = r[side];") {
		t.Error("rangeDraw no longer updates every input but the typing source")
	}
	if !strings.Contains(trackerFn(t, `function rangeInput\(t\)`, false), "rangeFit(r); rangeDraw(t);") {
		t.Error("rangeInput no longer passes its own input as the redraw source")
	}
	if !strings.Contains(trackerFn(t, `function closeMenu\(refocus, quiet\)`, false), "if (m.rg && m.rg.drag) clearInterval(m.rg.drag.tm);") {
		t.Error("a menu closed mid-drag no longer stops the drag's edge timer")
	}
}

// TestTrackerPickerPressKeepsFocus pins that a press on a non-control part
// of a capsule picker (a day cell, a header) keeps focus where it was
// (tracker.js rangeDown), so Esc and Tab still reach menuKey; a press on a
// control, or on another menu, takes focus as usual.
func TestTrackerPickerPressKeepsFocus(t *testing.T) {
	runTrackerCases(t, "picker press focus", trackerCheckJS+`
var MN = null, draws = 0;
function rangeDraw() { draws++; }
function rangeFit() {}
function rangeTick() {}
var RG_EDGE_MS = 450;
function $() { return { setPointerCapture: function () {} }; }
`+trackerFn(t, `function capOn\(\)`, true)+trackerFn(t, `function rangeDown\(e\)`, false)+`
function press(target) { var e = { button: 0, prevented: 0, target: target, preventDefault: function () { this.prevented++; } }; rangeDown(e); return e.prevented; }
function el(ctl, day) { return { closest: function (q) { if (/data-rh/.test(q)) return null; if (/data-day/.test(q)) return day ? this : null; return ctl ? this : null; }, getAttribute: function () { return '2026-11-12'; } }; }
MN = { kind: 'item', step: 'due', rg: { s: '2026-11-10', d: '2026-11-19' } };
check('a day cell press keeps focus', press(el(false, true)), 1);
check('a press on an input or button takes focus', press(el(true, false)), 0);
MN = { kind: 'item', step: 'labels' };
check('the label picker keeps focus on its text too', press(el(false, false)), 1);
MN = { kind: 'item', step: 'main' };
check('other menus are left alone', press(el(false, false)), 0);
done();
`)
}

// TestTrackerLabelPickerDiscard pins the discard capsule and Esc on the label
// pickers (tracker.js menuDiscard): the item labels step's pending toggles
// drop and nothing is written; the new row's label toggles, which edit the
// draft live, go back to what the picker opened on; the capsule shows for
// the range and label pickers only.
func TestTrackerLabelPickerDiscard(t *testing.T) {
	runTrackerCases(t, "label picker discard", rangeHarness(t)+`
MN = { kind: 'item', item: 'i1', step: 'labels', from: null, backEl: opener, back: '#opener', lset: null };
MN.lset = labelFlip(MN.lset, labelBase(VS.items.i1), 'L2');
menuKey(ev({ key: 'Escape', target: menuCtl[0] }));
check('Esc on the labels step discards its toggles', [sent, MN], [[], null]);
MN = { kind: 'item', item: 'i1', step: 'labels', from: null, backEl: opener, back: '#opener', lset: null };
MN.lset = labelFlip(MN.lset, labelBase(VS.items.i1), 'L2');
menuDiscard();
check('the capsule discards them too', [sent, MN], [[], null]);
MN = { kind: 'item', item: 'i1', step: 'labels', from: null, backEl: opener, back: '#opener', lset: null };
MN.lset = labelFlip(MN.lset, labelBase(VS.items.i1), 'L2');
check('the capsule is inside: a click on it applies nothing', [menuOutside(ev(), els.tkcap), sent, MN && MN.step], [false, [], 'labels']);
menuFocusOut({ relatedTarget: els.tkcap });
check('tabbing to it applies nothing', [sent, MN && MN.step], [[], 'labels']);
menuFocusOut({ relatedTarget: node() });
check('focus leaving for outside still applies the toggles', sent, [[['item.edit', { item: 'i1', labels: ['L1', 'L2'] }]]]);
var n = { val: 'x', kind: 'task', who: ['me'], labels: ['L1'] };
ui.nrow.s1 = n; sent = [];
MN = { kind: 'new', ed: false, step: 'labels', from: null, backEl: opener, back: '#opener', lsnap: ['L1'] };
n.labels = ['L1', 'L2'];
menuDiscard();
check('a row label pick discarded goes back to the snapshot', [n.labels, MN, sent], [['L1'], null, []]);
MN = { kind: 'item', step: 'labels' };
check('the capsule shows on the label step', capOn(), true);
MN = { kind: 'new', step: 'due' };
check('and on a chip\'s range picker', capOn(), true);
MN = { kind: 'item', step: 'pri' };
check('not on other steps', capOn(), false);
MN = { kind: 'sync' };
check('nor on the sync popover', capOn(), false);
done();
`)
	nm := trackerFn(t, `function newMenu\(step, anchor, ed\)`, false)
	if !strings.Contains(nm, "lsnap: step === 'labels' ? (n.labels || []).slice() : null") {
		t.Error("the new row's label picker keeps no snapshot to discard back to")
	}
}

// TestTrackerRangePickerGrid pins the picker's grid and panels (tracker.js
// rangeMonth, rangeFit, rangeSet, rangeEdge): weeks run Sunday first; the
// span runs from start to due across the panels shown; a due-only item has
// a ghost start handle and a start-only one a ghost due; two panels open
// when the ends fall in different months (not adjacent ones only) and
// collapse when they share one; a drag clamps at the other end; a drag past
// a panel edge opens the neighbouring month or pages that end's panel only.
func TestTrackerRangePickerGrid(t *testing.T) {
	runTrackerCases(t, "range picker grid", trackerCheckJS+`
var VS = { now: '2026-11-13T09:00:00Z' }, MN = null;
function today() { return String(VS.now).slice(0, 10); }
function wsT(k) { return k; }
function rangeDraw() {}
`+trackerFn(t, `function esc\(s\)`, false)+trackerFn(t, `function attr\(k, v\)`, true)+trackerFn(t, `function tipAttr\(text\)`, true)+trackerFn(t, `function dayNum\(d\)`, true)+
		trackerFn(t, `function monAdd\(m, n\)`, true)+trackerFn(t, `function rangeBad\(r\)`, true)+trackerFn(t, `function rangeFit\(r\)`, false)+trackerFn(t, `function calDow\(\)`, false)+
		trackerFn(t, `function rangeHtml\(r\)`, false)+trackerFn(t, `function rangeField\(r, side\)`, false)+trackerFn(t, `function rangeCal\(r\)`, true)+trackerFn(t, `function rangeMonth\(r, m, side\)`, false)+
		trackerFn(t, `function rangeSet\(r, side, day\)`, false)+trackerFn(t, `function rangeEdge\(r, side, dir\)`, false)+`
function has(s, x) { return s.indexOf(x) >= 0; }
function cells(h) { return h.match(/class="rgd[^"]*"( data-day="[^"]*")?/g); }
function rg(s, d) { var r = { s: s, d: d, lm: null, rm: null }; rangeFit(r); r.err = rangeBad(r); return r; }
var r = rg('2026-11-10', '2026-11-19'), h = rangeMonth(r, '2026-11', 'l');
check('the header runs Sun..Sat', h.match(/class="rgw">[^<]*/g).map(function (x) { return x.slice(11); }), ['js.tk.cal.sun', 'js.tk.cal.mon', 'js.tk.cal.tue', 'js.tk.cal.wed', 'js.tk.cal.thu', 'js.tk.cal.fri', 'js.tk.cal.sat']);
check('November 2026 starts on a Sunday: no blank', cells(h)[0], 'class="rgd" data-day="2026-11-01"');
var oct = cells(rangeMonth(rg('', ''), '2026-10', 'l'));
check('October 2026 starts on a Thursday: four blanks', [oct.slice(0, 4).every(function (c) { return c === 'class="rgd off"'; }), oct[4]], [true, 'class="rgd" data-day="2026-10-01"']);
check('the span runs start..due with the handles at its ends', [has(h, 'class="rgd in s" data-day="2026-11-10"><span class="rgk ks" data-rh="s">'), has(h, 'class="rgd in" data-day="2026-11-15"'), has(h, 'class="rgd in d" data-day="2026-11-19"><span class="rgk kd" data-rh="d">'), has(h, 'class="rgd now" data-day="2026-11-13"'), has(h, 'data-day="2026-11-20" class')], [true, true, true, false, false]);
check('today is marked inside the span', has(h, 'class="rgd in now" data-day="2026-11-13"'), true);
var c = rangeMonth(rg('', '2026-11-19'), '2026-11', 'l');
check('a due-only item: the due handle and a ghost start on the due day', has(c, 'data-day="2026-11-19"><span class="rgk kd" data-rh="d"></span><span class="rgk ks ghost" data-rh="s"></span>'), true);
var so = rangeMonth(rg('2026-11-10', ''), '2026-11', 'l');
check('a start-only item: the start handle and a ghost due', has(so, 'data-day="2026-11-10"><span class="rgk ks" data-rh="s"></span><span class="rgk kd ghost" data-rh="d"></span>'), true);
check('no span without both ends', [has(c, ' in'), has(so, ' in')], [false, false]);
var B = rg('2026-09-21', '2026-12-04');
check('ends in different months: two panels, start left and due right, not adjacent', [B.lm, B.rm], ['2026-09', '2026-12']);
var bh = rangeHtml(B);
check('each panel highlights its share of the span', [has(bh, 'data-day="2026-09-30"'), has(bh, 'class="rgd in" data-day="2026-09-30"'), has(bh, 'class="rgd in" data-day="2026-12-01"'), has(bh, 'class="rgd" data-day="2026-12-05"'), has(bh, 'class="rgp two"')], [true, true, true, true, true]);
check('each panel has its own arrows', [has(bh, 'data-act="m-rpage" data-side="l" data-dir="-1"'), has(bh, 'data-act="m-rpage" data-side="r" data-dir="1"')], [true, true]);
var bad = rg('2026-11-25', '2026-11-19'), xh = rangeHtml(bad);
check('an inverted pair: no span, the error shown and set on the inputs', [has(xh, ' in"'), has(xh, 'class="rgp bad"'), has(xh, '<p class="rgerr" id="tkrerr" role="alert">js.tk.range.err</p>'), (xh.match(/aria-invalid="true" aria-describedby="tkrerr"/g) || []).length], [false, true, true, 2]);
check('the inputs carry the ends with a clear button each', [has(bh, 'id="tkrs" data-rin="s" value="2026-09-21"'), has(bh, 'id="tkrd" data-rin="d" data-autofocus value="2026-12-04"'), has(bh, 'data-act="m-rclr" data-side="s"'), has(bh, 'data-act="m-rclr" data-side="d"')], [true, true, true, true]);
check('day cells are not buttons (out of the Tab ring)', /<button[^>]*data-day/.test(bh), false);
check('the same month collapses to one panel', [rg('2026-11-02', '2026-11-28').rm, rg('2026-11-02', '').rm, rg('', '').lm], [null, null, '2026-11']);

// drags clamp at the other end
var d = rg('2026-11-10', '2026-11-19');
rangeSet(d, 's', '2026-11-25');
check('a start dragged past the due clamps at it', [d.s, d.d, d.err], ['2026-11-19', '2026-11-19', false]);
rangeSet(d, 'd', '2026-11-02');
check('a due dragged before the start clamps at it', [d.s, d.d], ['2026-11-19', '2026-11-19']);
var g = rg('', '2026-11-19');
rangeSet(g, 's', '2026-11-12');
check('the ghost dragged out creates the start', [g.s, g.d], ['2026-11-12', '2026-11-19']);

// edges: one panel opens the neighbouring month; two panels page the dragged end's panel only
var e = rg('2026-11-10', '2026-11-19');
rangeEdge(e, 'd', 1);
check('a due dragged past the right edge opens the next month on the right', [e.lm, e.rm], ['2026-11', '2026-12']);
rangeEdge(e, 'd', 1);
check('then pages the right panel only', [e.lm, e.rm], ['2026-11', '2027-01']);
rangeEdge(e, 's', -1);
check('a start past the left panel\'s edge pages that panel only', [e.lm, e.rm], ['2026-10', '2027-01']);
var f = rg('2026-11-10', '2026-11-19');
rangeEdge(f, 's', -1);
check('a start past the edge of one panel opens the previous month on the left', [f.lm, f.rm], ['2026-10', '2026-11']);
var o = rg('2026-11-10', '2026-11-19');
rangeEdge(o, 's', 1);
check('a start dragged toward the due past the edge opens nothing', [o.lm, o.rm], ['2026-11', null]);
var one = rg('', '2026-11-19');
one.d = '2026-11-19'; rangeEdge(one, 'd', 1);
check('an end alone pages its one panel', [one.lm, one.rm], ['2026-12', null]);
var tw = rg('2026-10-10', '2026-11-19');
rangeEdge(tw, 's', 1); rangeEdge(tw, 's', 1);
check('the start panel never pages past the due panel', [tw.lm, tw.rm], ['2026-11', '2026-11']);
check('the month after December is January', [monAdd('2026-12', 1), monAdd('2026-01', -1)], ['2027-01', '2025-12']);
done();
`)
}

// TestTrackerCalendarIsSundayFirst pins the calendar view's week start
// (tracker.js calendarBody): the header runs Sun..Sat and the first of a
// month sits on its Sunday-first column, the same grid the range picker draws.
func TestTrackerCalendarIsSundayFirst(t *testing.T) {
	runTrackerCases(t, "calendar week start", trackerCheckJS+`
var VS = { now: '2026-11-13T09:00:00Z', items: {} }, ui = { calMonth: { all: '2026-10' } };
function wsT(k) { return k; }
function btn() { return ''; }
function itemFilterBar() { return ''; }
`+trackerFn(t, `function esc\(s\)`, false)+trackerFn(t, `function attr\(k, v\)`, true)+trackerFn(t, `function today\(\)`, true)+trackerFn(t, `function dayNum\(d\)`, true)+
		trackerFn(t, `function calDow\(\)`, false)+trackerFn(t, `function calendarBody\(key, rows\)`, false)+`
var h = calendarBody('all', []);
check('the header runs Sun..Sat', h.match(/class="cdow">[^<]*/g).map(function (x) { return x.slice(13); }), ['js.tk.cal.sun', 'js.tk.cal.mon', 'js.tk.cal.tue', 'js.tk.cal.wed', 'js.tk.cal.thu', 'js.tk.cal.fri', 'js.tk.cal.sat']);
check('October 2026 (a Thursday first) has four leading blanks', (h.match(/<div class="cday off"><\/div>/g) || []).length, 4);
ui.calMonth.all = '2026-11';
check('November 2026 (a Sunday first) has none', (calendarBody('all', []).match(/cday off/g) || []).length, 0);
done();
`)
}

// TestTrackerGanttBarStart pins a gantt bar's start (tracker.js ganttSpan):
// the item's start date, else its first doing, else its creation; a start
// after the due (concurrent edits can fold one) is a one-day bar at the due;
// no due, no bar.
func TestTrackerGanttBarStart(t *testing.T) {
	runTrackerCases(t, "gantt start", trackerCheckJS+trackerFn(t, `function dayNum\(d\)`, true)+trackerFn(t, `function ganttSpan\(it\)`, false)+`
function days(r) { return r && r.map(function (n) { return new Date(n * 86400000).toISOString().slice(0, 10); }); }
var base = { kind: 'task', due: '2026-11-19', started: '2026-11-08T10:00:00Z', ts: '2026-11-01T10:00:00Z' };
function it(o) { var x = {}; Object.keys(base).forEach(function (k) { x[k] = base[k]; }); Object.keys(o).forEach(function (k) { x[k] = o[k]; }); return x; }
check('the start date first', days(ganttSpan(it({ start: '2026-11-10' }))), ['2026-11-10', '2026-11-20']);
check('else the first doing', days(ganttSpan(it({}))), ['2026-11-08', '2026-11-20']);
check('else the creation', days(ganttSpan(it({ started: null }))), ['2026-11-01', '2026-11-20']);
check('an inverted pair is one day at the due', days(ganttSpan(it({ start: '2026-11-25' }))), ['2026-11-19', '2026-11-20']);
check('no due, no bar', ganttSpan(it({ due: '', start: '2026-11-10' })), null);
check('a topic has none', ganttSpan(it({ kind: 'topic' })), null);
done();
`)
	if !strings.Contains(trackerFn(t, `function ganttBody\(key, rows\)`, false), "function own(id) { return ganttSpan(VS.items[id]); }") {
		t.Error("the gantt does not place its bars with ganttSpan")
	}
}

// TestTrackerStartBadge pins the row's date badge (tracker.js dueHtml) with a
// start: a start-only task shows "from MM-DD" in the neutral badge, opening
// the picker for an editor; a task with both ends keeps the due's D-N form,
// marked as a range, the start in its tooltip.
func TestTrackerStartBadge(t *testing.T) {
	runTrackerCases(t, "start badge", trackerCheckJS+`
var VS = { now: '2026-11-13T09:00:00Z' };
function wsT(k, a, b) { return k + (a != null ? '(' + a + ')' : '') + (b != null ? '(' + b + ')' : ''); }
function wsTn(k, n, a) { return k + '(' + n + ')(' + a + ')'; }
var SI = { info: function (s) { return { label: s }; } };
`+trackerFn(t, `function esc\(s\)`, false)+trackerFn(t, `function attr\(k, v\)`, true)+trackerFn(t, `function tipAttr\(text\)`, true)+trackerFn(t, `function partHtml\(cls, tip, inner, it, step\)`, false)+
		trackerFn(t, `function partIcon\(icon\)`, true)+trackerFn(t, `var ENDED = `, true)+trackerFn(t, `function dueHtml\(it, st, ed\)`, false)+trackerFn(t, `function dayNum\(d\)`, true)+trackerFn(t, `function today\(\)`, true)+`
function has(s, x) { return s.indexOf(x) >= 0; }
var so = dueHtml({ id: 'a', kind: 'task', start: '2026-11-20', due: '' }, 'todo', true);
check('start only: "from" in the neutral badge, opening the picker', [has(so, 'class="due2 from rp" data-act="due"'), has(so, '>js.tk.due.from(11-20)<'), has(so, 'data-tip="js.tk.due.tip.from(2026-11-20)"')], [true, true, true]);
check('start only without the edit right: a plain badge', has(dueHtml({ id: 'a', kind: 'task', start: '2026-11-20', due: '' }, 'todo', false), '<span class="due2 from" data-tip='), true);
var both = dueHtml({ id: 'b', kind: 'task', start: '2026-11-10', due: '2026-11-16' }, 'todo', true);
check('both ends: the due\'s D-N form, marked as a range, the start in the tooltip', [has(both, 'class="due2 soon rng rp"'), has(both, '>js.tk.due.dminus(3)<'), has(both, 'data-tip="js.tk.due.tip.from(2026-11-10) · ')], [true, true, true]);
check('due only is unchanged', has(dueHtml({ id: 'c', kind: 'task', due: '2026-11-16' }, 'todo', true), 'class="due2 soon rp"'), true);
done();
`)
}

// TestTrackerRowEditStart pins the start date in the row's draft: the edit
// row prefills it, a save sends a changed start in the same item.edit as the
// other changed fields and an unchanged one not at all, a start another
// viewer wrote meanwhile and the user left alone is kept, the new row's
// create carries it beside due, and a save the store refuses (a start after
// a due another viewer moved) toasts the refusal and keeps the draft.
func TestTrackerRowEditStart(t *testing.T) {
	runTrackerCases(t, "row edit start", trackerCheckJS+`
var sent = [], toasts = [], refuse = null, ui = {}, elv = { value: 'Old', focus: function () {}, select: function () {} };
var VS = { viewer: { member: 'me' }, priority: { default: 1, names: ['low', 'mid', 'high'] }, labels: [],
  items: { i1: { id: 'i1', kind: 'task', title: 'Old', status: 'idea', start: '2026-11-10', due: '2026-12-01', priority: 1, labels: [], author: 'me', assignments: [] } } };
function canEdit() { return true; }
function canAssign() { return false; }
function mineLive() { return null; }
function holders(it) { return it.assignments; }
function wsT(k) { return k; }
function render() {}
function toast(m) { toasts.push(m); }
function esc(s) { return s; }
function $() { return ui.ned ? elv : null; }
var AD = { dispatch: function (store, ev, cb) { sent.push(ev.map(function (e) { return [e.type, e.payload]; })); cb(refuse ? { ok: false, error: refuse } : { ok: true, results: [] }); } };
function wrote() {}
function labelOf() { return null; }
function nrowSt() { return ''; }
function asgEvents() { return []; }
var document = { querySelector: function () { return null; } }, CSS = { escape: function (x) { return x; } };
`+trackerFn(t, `function editHeld\(it\)`, false)+trackerFn(t, `function editWho\(it\)`, false)+trackerFn(t, `function startRowEdit\(id, rk\)`, false)+trackerFn(t, `function editRowEvents\(n, it, title\)`, false)+
		trackerFn(t, `function endRowEdit\(\)`, false)+trackerFn(t, `function saveRowEdit\(\)`, false)+trackerFn(t, `function labelsOver\(cur, set\)`, false)+trackerFn(t, `function labelPicks\(from, to\)`, false)+
		trackerFn(t, `function sameLabels\(a, b\)`, false)+trackerFn(t, `function nrowPri\(n\)`, true)+trackerFn(t, `function commit\(events, then, prep, fail\)`, false)+`
function save(mut) { startRowEdit('i1', 'r'); var n = ui.ned; if (mut) mut(n); sent = []; toasts = []; saveRowEdit(); return sent; }
startRowEdit('i1', 'r');
check('the draft is prefilled with the start', [ui.ned.start, ui.ned.orig.start], ['2026-11-10', '2026-11-10']);
check('a changed start alone: start only in the item.edit', save(function (n) { n.start = '2026-11-12'; }), [[['item.edit', { item: 'i1', start: '2026-11-12' }]]]);
check('start and due in one item.edit', save(function (n) { n.start = '2026-11-12'; n.due = '2026-12-05'; }), [[['item.edit', { item: 'i1', start: '2026-11-12', due: '2026-12-05' }]]]);
check('a cleared start is null', save(function (n) { n.start = ''; }), [[['item.edit', { item: 'i1', start: null }]]]);
check('an unchanged start sends nothing', save(function (n) { n.pri = 2; }), [[['item.edit', { item: 'i1', priority: 2 }]]]);
check('a start another viewer wrote meanwhile is kept', save(function (n) { VS.items.i1.start = '2026-11-01'; n.pri = 0; }), [[['item.edit', { item: 'i1', priority: 0 }]]]);
VS.items.i1.start = '2026-11-10';
refuse = 'start 2026-11-30 is after due 2026-11-20';
var out = save(function (n) { n.start = '2026-11-30'; n.pri = 0; });
check('a refused save is one batch, toasted, the draft kept', [out.length, toasts, !!ui.ned, ui.ned && ui.ned.start], [1, ['start 2026-11-30 is after due 2026-11-20'], true, '2026-11-30']);
done();
`)
	cn := trackerFn(t, `function createNew\(\)`, false)
	if !strings.Contains(cn, "if (n.start) cp.start = n.start;") {
		t.Error("the new row's create does not carry the start")
	}
}
