package wsdashboard

import (
	"strings"
	"testing"
)

// trackerFn returns one top-level tracker.js function (or var) by its
// opening, up to its closing "  }" line, or its own line for a one-liner.
func trackerFn(t *testing.T, open string, oneLine bool) string {
	t.Helper()
	if oneLine {
		return trackerBlock(t, `  `+open, `\n`) + "\n"
	}
	return trackerBlock(t, `  `+open, `\n  \}\n`) + "\n  }\n"
}

// TestTrackerSelectionModeIsMultiSelect pins the Items list's selection mode
// (tracker.js mselPick): a plain click toggles the row and makes it the
// anchor, Shift-click adds the range from the anchor without clearing the
// rest, and a Shift-click with no anchor shown toggles. It also pins the
// selection badge: pinned atop the list body, not a header chip.
func TestTrackerSelectionModeIsMultiSelect(t *testing.T) {
	panel := trackerBlock(t, `var nsel = mselIds\(\)\.length`, `\n    TF = null;`)
	if !strings.Contains(panel, `'<div class="mselbar" role="status"><span class="chip mselc">'`) || !strings.Contains(panel, `attr('act', 'msel-clear')`) {
		t.Errorf("the selection badge is not the list's sticky bar with its clear button:\n%s", panel)
	}
	if ret := trackerBlock(t, `return \{ title: wsT\("js.tk.tab.items"\)`, `\n`); !strings.Contains(ret, "body: mbar + '<div class=\"plist\">'") || strings.Contains(ret, "mchip") {
		t.Errorf("the badge does not top the list body:\n%s", ret)
	}
	runTrackerCases(t, "selection", trackerCheckJS+trackerFn(t, `function mselPick\(`, false)+`
var keys = [['a', 'r0'], ['b', 'r1'], ['c', 'r2'], ['d', 'r3'], ['e', 'r4']];
function ids(s) { return Object.keys(s.ids).sort(); }
var s = { ids: {}, at: null };
mselPick(s, keys, 0, false);
check('first click selects and anchors', [ids(s), s.at], [['a'], 'r0']);
mselPick(s, keys, 2, false);
check('a plain click adds, keeping the rest', [ids(s), s.at], [['a', 'c'], 'r2']);
mselPick(s, keys, 2, false);
check('a plain click on a selected row drops it', [ids(s), s.at], [['a'], 'r2']);
mselPick(s, keys, 4, true);
check('shift adds the range from the anchor, keeping the rest', [ids(s), s.at], [['a', 'c', 'd', 'e'], 'r2']);
var s2 = { ids: { b: true }, at: 'gone' };
mselPick(s2, keys, 3, true);
check('shift with the anchor not shown toggles', [ids(s2), s2.at], [['b', 'd'], 'r3']);
done();
`)
}

// TestTrackerQuickAddRow pins the Items quick-add row: the kind is an icon
// toggle at the row's left edge (no select), the row carries priority and
// label chips, the kind switch keeps its rules (a topic hides status,
// priority, and due, and keeps one holder), and the create payload carries
// the picked labels and a non-default priority (a topic sends no priority).
// The kanban idea column's row stays title-only.
func TestTrackerQuickAddRow(t *testing.T) {
	row := trackerFn(t, `function newRowHtml\(\)`, false)
	for _, want := range []string{`id="tknkind"' + attr('act', 'nkind')`, `SI.svg(n.kind === 'topic' ? 'topic' : 'todo')`, `tipAttr(kl)`} {
		if !strings.Contains(row, want) {
			t.Errorf("newRowHtml lacks %q", want)
		}
	}
	if strings.Contains(row, "<select") || strings.Contains(row, "tki-plus") {
		t.Errorf("newRowHtml still draws the kind select or the leading plus:\n%s", row)
	}
	chips := trackerFn(t, `function nrowChips\(n\)`, false)
	for _, want := range []string{`chip('tknpri', 'pri'`, `chip('tknlab', 'labels'`} {
		if !strings.Contains(chips, want) {
			t.Errorf("nrowChips lacks %q", want)
		}
	}
	if k := trackerFn(t, `function knewRowHtml\(n\)`, false); strings.Contains(k, "nrowChips") || strings.Contains(k, "tknkind") {
		t.Errorf("the kanban idea row is no longer title-only:\n%s", k)
	}
	src := trackerFn(t, `function nrowFill\(n\)`, false) + trackerFn(t, `function nrowMine\(n\)`, true) +
		trackerFn(t, `var NROW_ST = `, true) + trackerFn(t, `function nrowStMode\(n\)`, false) + trackerFn(t, `function nrowSt\(n\)`, true) +
		trackerFn(t, `function nrowStOpts\(n\)`, true) + trackerFn(t, `function nrowPri\(n\)`, true) + trackerFn(t, `function nrowLabels\(n\)`, true) + trackerFn(t, `function nrowKind\(kind\)`, false) +
		trackerFn(t, `function createNew\(\)`, false) + trackerFn(t, `function asIdea\(cp, s\)`, false) + trackerFn(t, `function heldTopic\(cp, holder\)`, false)
	runTrackerCases(t, "quick-add", trackerCheckJS+`
var VS = { viewer: { member: 'me', reach: { kim: true } }, priority: { names: ['low', 'mid', 'high'], default: 1 },
  labels: [{ id: 'L1', name: 'docs', color: '#111111' }, { id: 'L2', name: 'dev', color: '#222222' }] };
var ui = { store: 's', nrow: {} }, title = 'T', sent = null;
function render() {}
function $(id) { return id === 'tknew' ? { value: title } : null; }
function commit(b) { sent = b; }
function asgEvents(item, sel, compl) { return sel.map(function (x) { return ['item.assign', { item: item, people: [x.id] }]; }); }
`+src+`
function draft(o) { var n = nrowFill({ val: '', kind: 'task' }); Object.keys(o || {}).forEach(function (k) { n[k] = o[k]; }); ui.nrow.s = n; return n; }
draft();
createNew();
check('defaults: create plus idea, no priority or labels', sent, [['item.create', { kind: 'task', title: 'T' }], ['item.status', { item: '$1', status: 'idea' }]]);
draft({ pri: 2, labels: ['L2', 'gone', 'L1'] });
createNew();
check('a picked level and labels ride the create (list order, removed labels dropped)', sent[0], ['item.create', { kind: 'task', title: 'T', labels: ['L1', 'L2'], priority: 2 }]);
draft({ pri: 1 });
createNew();
check('the default level sends no priority', sent[0], ['item.create', { kind: 'task', title: 'T' }]);
var n = draft({ who: ['kim', 'lee'], pri: 2, due: '2026-12-01', labels: ['L1'] });
nrowKind('topic');
check('the toggle to topic keeps one holder (the viewer)', [n.kind, n.who], ['topic', ['me']]);
createNew();
check('a topic sends labels, no priority or due', sent, [['item.create', { kind: 'topic', title: 'T', labels: ['L1'] }], ['item.assign', { item: '$1', mode: 'chosen', people: ['me'], completion: 'assignee' }]]);
nrowKind('task');
check('the toggle back keeps the chips', [n.kind, n.pri, n.due], ['task', 2, '2026-12-01']);
VS.priority = { names: ['only'], default: 0 };
check('a level the store no longer holds falls back to the default', nrowPri(n), 0);
done();
`)
}

// TestTrackerPriorityChipAndLabelPicker pins the two chips' pickers: the
// quick-add priority chip and the item window header's chip open the item
// menu's priority step (one radio group, priGroup); the header chip shows
// only to an editor of a task and replaces the chip row's read-only one; the
// quick-add label picker toggles labels and, for an admin, reuses the store
// settings' label dialog and the confirmed remove path.
func TestTrackerPriorityChipAndLabelPicker(t *testing.T) {
	menu := trackerFn(t, `function newMenuHtml\(\)`, false)
	for _, want := range []string{"if (MN.step === 'pri') return h + priGroup(nrowPri(n));", "if (MN.step === 'labels') return h + newLabelsHtml(n);"} {
		if !strings.Contains(menu, want) {
			t.Errorf("newMenuHtml lacks %q", want)
		}
	}
	if !strings.Contains(trackerFn(t, `function menuHtml\(\)`, false), "if (step === 'pri') return h + back + priGroup(it.priority);") {
		t.Error("the item menu's priority step does not use priGroup")
	}
	acts := trackerFn(t, `function headActs\(it\)`, false)
	for _, want := range []string{"if (canEdit(it) && it.kind !== 'topic') {", "attr('act', 'pri') + attr('item', it.id)"} {
		if !strings.Contains(acts, want) {
			t.Errorf("headActs lacks %q", want)
		}
	}
	if !strings.Contains(trackerFn(t, `function windowHtml\(v, it\)`, false), "(canEdit(it) ? '' : prChip(it))") {
		t.Error("the chip row still shows the read-only priority chip beside the header's")
	}
	labels := trackerFn(t, `function labelChecklist\(cur\)`, false)
	for _, want := range []string{"attr('act', 'm-ltog')", "admin = !!VS.viewer.admin", "attr('act', 'm-ledit')", "attr('act', 'm-lrm')", "mi('m-ladd'"} {
		if !strings.Contains(labels, want) {
			t.Errorf("labelChecklist lacks %q", want)
		}
	}
	act := trackerFn(t, `function newAct\(k, a\)`, false)
	for _, want := range []string{"return openDialog('slabel', k === 'm-ledit' ? { label: a.getAttribute('data-label') } : {});", "return labelRemove(a.getAttribute('data-label')"} {
		if !strings.Contains(act, want) {
			t.Errorf("newAct lacks %q", want)
		}
	}
	rm := trackerFn(t, `function labelRemove\(id, then\)`, false)
	if !strings.Contains(rm, `window.confirm(wsT("js.tk.set.labels.confirm", l.name))) commit([['roles.remove_label', { label: l.id }]], then);`) {
		t.Errorf("labelRemove does not confirm before the remove:\n%s", rm)
	}
	if !strings.Contains(trackerBlock(t, `else if \(k === 'slabel-rm'\)`, `\n`), "labelRemove(d.label);") {
		t.Error("the store settings' remove does not share labelRemove")
	}
}

// TestTrackerPropertyCardLabelPicker pins the property card's label row: an
// editor's row is a button that opens the item menu's labels step (anyone
// else's only shows the chips, an empty set as "-"; an editor's empty set a
// muted "add a label"); the step lists the store's labels as a checklist
// drawing the user's picks over the item's current labels (in the store's
// list order) with the step kept open, the write owed at close
// (TestTrackerLabelStepAppliesOnClose); an empty store says so and, for an admin only,
// offers the store settings' label dialog. Escape on the step closes it,
// focus back on the row.
func TestTrackerPropertyCardLabelPicker(t *testing.T) {
	if !strings.Contains(trackerFn(t, `function propertyCard\(it\)`, false), "'</dt><dd>' + labelsProp(it) + '</dd>'") {
		t.Error("the property card's label row is not labelsProp")
	}
	if !strings.Contains(trackerFn(t, `var STEPS = `, true), "labels: 1") {
		t.Error("the labels act does not open an item menu step")
	}
	if !strings.Contains(trackerFn(t, `function menuHtml\(\)`, false), "if (step === 'labels') return h + back + labelStepHtml(it);") {
		t.Error("the item menu has no labels step")
	}
	act := trackerFn(t, `function menuAct\(k, a\)`, false)
	for _, want := range []string{
		"case 'm-ltog': MN.lab = a.getAttribute('data-label'); MN.lset = labelFlip(MN.lset, labelBase(it), MN.lab); return labelStepRedraw();",
		"case 'm-ladd': return labelDialog({});",
	} {
		if !strings.Contains(act, want) {
			t.Errorf("menuAct lacks %q", want)
		}
	}
	wr := trackerFn(t, `function wrote\(results\)`, false)
	for _, want := range []string{"var keep = !!(MN && MN.kind === 'item' && MN.step === 'labels');", "if (MN && MN.kind === 'item' && !keep) closeMenu(false);", "if (keep && MN) labelStepRedraw();"} {
		if !strings.Contains(wr, want) {
			t.Errorf("wrote lacks %q", want)
		}
	}
	if !strings.Contains(trackerFn(t, `function menuKey\(e\)`, false), "if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); if (MN.from) menuBack(); else closeMenu(true); return; }") {
		t.Error("Escape on a first menu step no longer closes it with focus back")
	}
	src := trackerFn(t, `function esc\(s\)`, false) + trackerFn(t, `function attr\(k, v\)`, true) + trackerFn(t, `function tipAttr\(text\)`, true) +
		trackerFn(t, `function dataAttrs\(data\)`, false) + trackerFn(t, `function mi\(act, label, data, o\)`, false) + trackerFn(t, `function msep\(\)`, true) +
		trackerFn(t, `function labelOf\(id\)`, true) + trackerFn(t, `function labelChip\(l, act, extra\)`, false) + trackerFn(t, `function labelChips\(it, act\)`, false) +
		trackerFn(t, `function labelsProp\(it\)`, false) + trackerFn(t, `function newLabelsHtml\(n\)`, true) +
		trackerFn(t, `function labelChecklist\(cur\)`, false) + trackerFn(t, `function labelStepHtml\(it\)`, false) + trackerFn(t, `function labelsOver\(cur, set\)`, false) +
		trackerFn(t, `var LPEND = \{\};`, true) + trackerFn(t, `function labelBase\(it\)`, true)
	runTrackerCases(t, "property card labels", trackerCheckJS+`
var VS = { viewer: { member: 'me', admin: false }, labels: [{ id: 'L1', name: 'docs', color: '#111111' }, { id: 'L2', name: 'dev', color: '#222222' }] }, editor = true, MN = { kind: 'item', step: 'labels' }, ui = { store: 's1' };
function wsT(k) { return k; }
function canEdit(it) { return editor; }
`+src+`
function has(s, x) { return s.indexOf(x) >= 0; }
var it = { id: 'i1', labels: ['L2'] };
var b = labelsProp(it);
check('an editor gets the row button opening the labels step', [has(b, '<button type="button" class="plab" id="iw-plabels" data-act="labels" data-item="i1" aria-haspopup="menu"'), has(b, '>dev</span>')], [true, true]);
check('an editor with no labels gets the add placeholder', has(labelsProp({ id: 'i2', labels: [] }), '<span class="muted">js.tk.prop.labels.add</span>'), true);
editor = false;
check('a viewer without the edit right sees the chips only', labelsProp(it), '<span class="chip lbl"><span class="ldot" style="background:#222222"></span>dev</span>');
check('a viewer without the edit right sees "-" for no labels', labelsProp({ id: 'i2', labels: [] }), '<span class="muted">-</span>');
var st = labelStepHtml(it);
check('the step is the quick-add checklist over the item\'s labels, the picked one checked', [has(st, 'role="menuitemcheckbox" aria-checked="false" data-mi data-act="m-ltog" data-label="L1"') || has(st, 'data-act="m-ltog" data-label="L1"'), has(st, 'aria-checked="true" data-mi'), has(st, 'class="cbx"'), has(st, 'class="mlrow"')], [true, true, true, true]);
check('drawing the step records no pick', MN.lset, undefined);
check('a pick on adds in list order', labelsOver(['L2'], { L1: true }), ['L1', 'L2']);
check('a pick off removes', labelsOver(['L1', 'L2'], { L1: false }), ['L2']);
check('labels the list no longer holds drop', labelsOver(['gone', 'L1'], { L2: true }), ['L1', 'L2']);
check('no picks: the labels as they are, in list order', labelsOver(['L2', 'L1'], null), ['L1', 'L2']);
MN.lset = { L1: true };
st = labelStepHtml(it);
check('the step draws the picks over the item\'s labels', [has(st, 'aria-checked="true" data-mi data-act="m-ltog" data-label="L1"'), has(st, 'aria-checked="true" data-mi data-act="m-ltog" data-label="L2"')], [true, true]);
it.labels = ['L2', 'L1'];
MN.lset = { L1: false };
check('the picks lay over the labels the item holds now: L1 picked off, L2 as held', [has(labelStepHtml(it), 'aria-checked="false" data-mi data-act="m-ltog" data-label="L1"'), has(labelStepHtml(it), 'aria-checked="true" data-mi data-act="m-ltog" data-label="L2"')], [true, true]);
VS.labels = [];
check('an empty store says so, no add for a non-admin', labelStepHtml(it), '<p class="mempty">js.tk.labels.empty</p>');
VS.viewer.admin = true;
check('an admin gets the label dialog', has(labelStepHtml(it), 'data-act="m-ladd"'), true);
done();
`)
}

// TestTrackerLabelStepAppliesOnClose pins the labels step's batching
// (tracker.js labelPending, called by closeMenu): a toggle only records the
// user's pick (labelFlip), and closing the menu owes one item.edit of the
// picks laid over the item's labels as they are at close, none without a pick
// or when the picks change nothing (a toggle and its undo), labels the list no
// longer holds dropped, nothing when the item or the edit right is gone. A
// label another viewer adds while the step is open, or one a previous close
// is still writing, is never written away by a stale copy: a step reopened
// while that write is in flight (labelWrite) draws and applies its picks over
// live labels with each pending write's own picks, until it lands or is refused.
func TestTrackerLabelStepAppliesOnClose(t *testing.T) {
	cm := trackerFn(t, `function closeMenu\(refocus\)`, false)
	for _, want := range []string{"var lw = labelPending(m);", "if (lw) labelWrite(lw);"} {
		if !strings.Contains(cm, want) {
			t.Errorf("closeMenu lacks %q", want)
		}
	}
	runTrackerCases(t, "label step applies on close", trackerCheckJS+`
var editor = true, VS = { items: { i1: { id: 'i1', labels: ['L1'] } }, labels: [{ id: 'L1' }, { id: 'L2' }, { id: 'L3' }] }, pend = [], ui = { store: 's1' };
function canEdit() { return editor; }
function toast() {}
function esc(s) { return s; }
function wsT(k) { return k; }
function wrote() {}
// the real commit dispatches; the adapter holds each callback until the case settles it
var AD = { dispatch: function (store, ev, cb) { pend.push({ store: store, ev: ev, ok: function () { cb({ ok: true }); }, no: function () { cb({ ok: false }); } }); } };
`+trackerFn(t, `function labelOf\(id\)`, true)+trackerFn(t, `function labelsOver\(cur, set\)`, false)+trackerFn(t, `function labelFlip\(set, cur, lid\)`, false)+
		trackerFn(t, `function sameLabels\(a, b\)`, false)+trackerFn(t, `function labelPending\(m\)`, false)+
		trackerFn(t, `var LPEND = \{\};`, true)+trackerFn(t, `function labelBase\(it\)`, true)+trackerFn(t, `function labelWrite\(ev, then\)`, false)+
		trackerFn(t, `function labelPicks\(from, to\)`, false)+trackerFn(t, `function commit\(events, then, prep, fail\)`, false)+`
// a labels step on i1: the toggles flip each label from how the step draws it (labelBase) then
function step() { var s = { kind: 'item', step: 'labels', item: 'i1', lset: null }; s.tog = function (lid) { s.lset = labelFlip(s.lset, labelBase(VS.items.i1), lid); return s; }; return s; }
function held(ls) { VS.items.i1.labels = ls; }
check('a pick on is one write over the labels', labelPending(step().tog('L2')), [['item.edit', { item: 'i1', labels: ['L1', 'L2'] }]]);
check('a toggle and its undo write nothing', labelPending(step().tog('L2').tog('L2')), null);
check('picking the only label off writes the empty list', labelPending(step().tog('L1')), [['item.edit', { item: 'i1', labels: [] }]]);
check('no toggle, nothing owed', labelPending(step()), null);
// scenario A: another viewer adds L3 while the step is open; closing without a toggle must not write it away
var a = step();
held(['L1', 'L3']);
check('a label added meanwhile, no toggle: no write', labelPending(a), null);
check('a label added meanwhile is kept beside the pick', labelPending(a.tog('L2')), [['item.edit', { item: 'i1', labels: ['L1', 'L2', 'L3'] }]]);
held(['L1']);
// scenario B: L2 picked and the step closed (its write in flight), reopened before the write lands, closed again
var b1 = step().tog('L2'), w1 = labelPending(b1), b2 = step();
check('the reopened step with no toggle owes nothing', labelPending(b2), null);
held(['L1', 'L2']); // w1 lands
check('nor after the first write lands', [w1, labelPending(b2)], [[['item.edit', { item: 'i1', labels: ['L1', 'L2'] }]], null]);
check('a toggle after it lands applies over the landed labels', labelPending(b2.tog('L3')), [['item.edit', { item: 'i1', labels: ['L1', 'L2', 'L3'] }]]);
held(['L1', 'gone']);
check('labels the list no longer holds are dropped from the write', labelPending(step().tog('L3')), [['item.edit', { item: 'i1', labels: ['L1', 'L3'] }]]);
check('a pick changing nothing listed is no change', labelPending({ kind: 'item', step: 'labels', item: 'i1', lset: { L1: true, gone: false } }), null);
held(['L1']);
check('another step owes nothing', labelPending({ kind: 'item', step: 'pri', item: 'i1', lset: { L2: true } }), null);
editor = false;
check('without the edit right nothing is written', labelPending(step().tog('L2')), null);
editor = true;
check('a gone item owes nothing', labelPending({ kind: 'item', step: 'labels', item: 'zz', lset: { L2: true } }), null);
// scenario C: pick L2 and close (written, in flight), reopen, pick L3, close before the first write lands
held(['L1']);
labelWrite(labelPending(step().tog('L2')));
var c2 = step();
check('the reopened step draws the in-flight pick', labelBase(VS.items.i1), ['L1', 'L2']);
check('untoggled, it owes nothing', labelPending(c2), null);
check('a second pick applies over the first, keeping L2', labelPending(c2.tog('L3')), [['item.edit', { item: 'i1', labels: ['L1', 'L2', 'L3'] }]]);
labelWrite(labelPending(c2));
held(['L1', 'L2']); pend[0].ok();
check('the first write landing leaves the second pending', labelBase(VS.items.i1), ['L1', 'L2', 'L3']);
held(['L1', 'L2', 'L3']); pend[1].ok();
check('the second landing clears the pending labels', [labelBase(VS.items.i1), Object.keys(LPEND)], [['L1', 'L2', 'L3'], []]);
labelWrite(labelPending(step().tog('L1')));
pend[2].no();
check('a refused write clears its pending labels', [labelBase(VS.items.i1), Object.keys(LPEND)], [['L1', 'L2', 'L3'], []]);
// a pending pick must not hide a label that arrives live before the next pick
VS.labels.push({ id: 'L4' }); held(['L1']); pend = [];
labelWrite(labelPending(step().tog('L2')));
held(['L1', 'L4']);
labelWrite(labelPending(step().tog('L3')));
check('the next dispatch keeps newer live labels beside both picks', pend[1].ev, [{ type: 'item.edit', payload: { item: 'i1', labels: ['L1', 'L2', 'L3', 'L4'] }, origin: 'human' }]);
held(['L1', 'L2', 'L4']); pend[0].ok();
held(['L1', 'L2', 'L3', 'L4']); pend[1].ok();
// a later failure must not forget an earlier write still in flight
held(['L1']); pend = [];
labelWrite(labelPending(step().tog('L2')));
labelWrite(labelPending(step().tog('L3')));
pend[1].no();
check('a failed second write leaves the first pick pending', labelBase(VS.items.i1), ['L1', 'L2']);
held(['L1', 'L4']);
check('the surviving pending pick still overlays live labels', labelBase(VS.items.i1), ['L1', 'L2', 'L4']);
held(['L1', 'L2', 'L4']); pend[0].ok();
check('the first write then clears only its own entry', Object.keys(LPEND), []);
// identical item ids in different stores have independent pending picks and settlement
held(['L1']); pend = [];
labelWrite(labelPending(step().tog('L2')));
ui.store = 's2'; held(['L4']);
check('another store does not inherit the pending pick', labelBase(VS.items.i1), ['L4']);
labelWrite(labelPending(step().tog('L3')));
check('the other store dispatches only its own labels', [pend[1].store, pend[1].ev[0].payload.labels], ['s2', ['L3', 'L4']]);
pend[0].no();
check('settling the first store keeps the second pending', labelBase(VS.items.i1), ['L3', 'L4']);
ui.store = 's1'; held(['L1']);
check('the first store is clear while the other is pending', labelBase(VS.items.i1), ['L1']);
pend[1].no();
check('settlement uses the captured store even after switching', Object.keys(LPEND), []);
done();
`)
}

// TestTrackerItemEditLabelPicks pins the item window edit form's label save
// (tracker.js saveEdit): like the row edit, it writes the picks made since
// startEdit over the item's labels as they are at save, so a label another
// viewer wrote meanwhile is kept, and an untouched label row writes none.
func TestTrackerItemEditLabelPicks(t *testing.T) {
	runTrackerCases(t, "item edit label picks", trackerCheckJS+`
var VS = { items: { i1: { id: 'i1', title: 'T', body: '', project: '', labels: ['L1'] } }, labels: [{ id: 'L1' }, { id: 'L2' }, { id: 'L3' }] };
var ui = { win: true, item: 'i1', edpv: {}, eatt: {} }, sent = [];
function wsT(k) { return k; }
function showItem() {}
function render() {}
function toast() {}
function endEdit() { ui.iedit = null; }
function $(id) { return id === 'iw-etitle' ? { value: VS.items.i1.title, focus: function () {} } : id === 'iw-ebody' ? { value: '' } : id === 'iw-eproj' ? { value: '' } : null; }
function commit(ev) { sent.push(ev); }
`+trackerFn(t, `function labelOf\(id\)`, true)+trackerFn(t, `function labelsOver\(cur, set\)`, false)+trackerFn(t, `function labelPicks\(from, to\)`, false)+
		trackerFn(t, `function sameLabels\(a, b\)`, false)+trackerFn(t, `function startEdit\(id\)`, false)+trackerFn(t, `function saveEdit\(\)`, false)+`
function save(mut) { startEdit('i1'); mut(ui.iedit); sent = []; saveEdit(); return sent; }
check('a pick writes the labels', save(function (e) { e.labels.push('L2'); }), [[['item.edit', { item: 'i1', labels: ['L1', 'L2'] }]]]);
check('a label added meanwhile, labels untouched: no write', save(function (e) { VS.items.i1.labels = ['L1', 'L3']; }), []);
VS.items.i1.labels = ['L1'];
check('a label added meanwhile is kept beside the pick', save(function (e) { VS.items.i1.labels = ['L1', 'L3']; e.labels = []; }), [[['item.edit', { item: 'i1', labels: ['L3'] }]]]);
done();
`)
}

// TestTrackerRowTitleEdit pins the list row's hover pencil: only an editor
// gets it, it turns the row into the new row's edit row (same chips) prefilled
// from the item, and its save sends only what the user changed since the
// editor opened - one item.edit, the viewer's own status, the assignee delta
// (item.assign for added people, item.unassign for dropped ones) - so a field
// another viewer wrote meanwhile is kept, while Escape, the x, and an
// unchanged draft write nothing. An unheld topic prefills no holder. A save
// closes only its own draft. There is no blur rule, as in the new row.
func TestTrackerRowTitleEdit(t *testing.T) {
	if !strings.Contains(trackerFn(t, `function irow\(o\)`, false), "return editRowHtml(o);") {
		t.Error("the list row does not give way to the edit row")
	}
	if !strings.Contains(trackerFn(t, `function irow\(o\)`, false), "titleEdHtml(it)") {
		t.Error("the list row does not draw the pencil")
	}
	if !strings.Contains(trackerFn(t, `function editRowHtml\(o\)`, false), "nrowChips(n)") {
		t.Error("the edit row does not reuse the new row's chips")
	}
	js, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(js), "e.target.id !== 'tkned'") {
		t.Error("the edit row saves on blur; the new row's convention is none")
	}
	runTrackerCases(t, "row edit", trackerCheckJS+`
var editor = true, sent = [], rec = null, canAsg = true, ui = {}, renders = 0, toasts = 0, elv = { value: '', focus: function () {}, select: function () {} };
var VS = { viewer: { member: 'me' }, priority: { default: 1, names: ['low', 'mid', 'high'] }, labels: [{ id: 'L1' }, { id: 'L2' }],
  items: { i1: { id: 'i1', kind: 'task', title: 'Old', status: 'idea', due: '2026-12-01', priority: 2, labels: ['L1'], author: 'me', assignments: [] } } };
function canEdit() { return editor; }
function canAssign() { return canAsg; }
function mineLive() { return rec; }
function holders(it) { return it.assignments; }
function wsT(k) { return k; }
function render() { renders++; }
function toast() { toasts++; }
function $() { return ui.ned ? elv : null; }
var held = null; // while an array, a save stays in flight: its prep waits there
var AD = { dispatch: function (store, ev, cb) { sent.push(ev.map(function (e) { return [e.type, e.payload]; })); if (held) held.push(function () { cb({ ok: true }); }); else cb({ ok: true }); } };
function wrote() {}
function labelOf(id) { return VS.labels.filter(function (l) { return l.id === id; })[0]; }
function nrowSt(n) { return n.ed && rec ? n.st : ''; }
function asgEvents(item, sel, compl) { return [['item.assign', { item: item, who: sel.map(function (o) { return o.id; }), compl: compl }]]; }
function esc(s) { return s; }
function attr(k, v) { return ' data-' + k + '="' + v + '"'; }
function tipAttr(x) { return ''; }
function partIcon(i) { return i; }
`+trackerFn(t, `function titleEdHtml\(it\)`, false)+trackerFn(t, `function editHeld\(it\)`, false)+trackerFn(t, `function editWho\(it\)`, false)+trackerFn(t, `function startRowEdit\(id, rk\)`, false)+trackerFn(t, `function editRowEvents\(n, it, title\)`, false)+trackerFn(t, `function endRowEdit\(\)`, false)+trackerFn(t, `function saveRowEdit\(\)`, false)+
		trackerFn(t, `function labelsOver\(cur, set\)`, false)+trackerFn(t, `function labelPicks\(from, to\)`, false)+trackerFn(t, `function sameLabels\(a, b\)`, false)+
		trackerFn(t, `function nrowPri\(n\)`, true)+trackerFn(t, `function commit\(events, then, prep, fail\)`, false)+`
var document = { querySelector: function () { return null; } }, CSS = { escape: function (x) { return x; } };
function draft() { startRowEdit('i1', 'r'); return ui.ned; }
function save(mut, title) { var n = draft(); if (mut) mut(n); elv.value = title == null ? n.val : title; sent = []; saveRowEdit(); return sent; }
check('an editor gets the pencil', titleEdHtml(VS.items.i1).indexOf('data-act="tedit" data-item="i1"') > 0, true);
editor = false;
check('anyone else gets none', titleEdHtml(VS.items.i1), '');
check('and cannot start the edit', (startRowEdit('i1', 'r'), ui.ned), undefined);
editor = true;
var n = draft();
check('the draft is prefilled from the item', [n.val, n.st, n.who, n.due, n.pri, n.labels, n.canAsg], ['Old', 'idea', ['me'], '2026-12-01', 2, ['L1'], true]);
check('an untouched draft writes nothing, says so, and closes the row', [save(), ui.ned === null, toasts], [[], true, 1]);
ui.ned = null;
check('title, priority, due, and labels go in one item.edit', save(function (n) { n.pri = 0; n.due = ''; n.labels = ['L2']; }, '  New  '),
  [[['item.edit', { item: 'i1', title: 'New', priority: 0, due: null, labels: ['L2'] }]]]);
check('a successful save closes the row', ui.ned, null);
check('a blank title keeps the old one', save(function (n) { n.pri = 0; }, '   '), [[['item.edit', { item: 'i1', priority: 0 }]]]);
check('labels the list no longer holds are dropped', save(function (n) { n.labels = ['L1', 'gone']; }), []);
// another viewer adds L2 after the draft was prefilled: an untouched labels chip writes nothing, a pick applies over L2
check('a label added meanwhile, labels untouched: no labels write', save(function (n) { VS.items.i1.labels = ['L1', 'L2']; }), []);
VS.items.i1.labels = ['L1'];
check('a label added meanwhile is kept beside the draft\'s pick', save(function (n) { VS.items.i1.labels = ['L1', 'L2']; n.labels = []; }),
  [[['item.edit', { item: 'i1', labels: ['L2'] }]]]);
VS.items.i1.labels = ['L1'];
rec = { status: 'idea' };
check('a changed own status is its own item.status', save(function (n) { n.st = 'doing'; }), [[['item.status', { item: 'i1', status: 'doing' }]]]);
check('done closes the item', save(function (n) { n.st = 'done'; }), [[['item.close', { item: 'i1' }]]]);
check('a changed assignee list is the assign', save(function (n) { n.who = ['you']; }), [[['item.assign', { item: 'i1', who: ['you'], compl: null }]]]);
check('a status written meanwhile, status untouched: no status write', save(function (n) { rec.status = 'blocked'; }), []);
rec.status = 'idea';
check('edit, status, and assign are one batch in that order', save(function (n) { n.st = 'todo'; n.pri = 0; n.who = ['you']; }),
  [[['item.edit', { item: 'i1', priority: 0 }], ['item.status', { item: 'i1', status: 'todo' }], ['item.assign', { item: 'i1', who: ['you'], compl: null }]]]);
// another viewer moves due and priority after the open
var it1 = VS.items.i1;
it1.title = 'Old ';
check('an untouched padded title does not overwrite a concurrent rename', save(function () { it1.title = 'Theirs'; }), []);
it1.title = 'Old';
check('an untouched priority stays clean when available levels shrink', save(function () { VS.priority.names = ['low', 'mid']; }), []);
VS.priority.names = ['low', 'mid', 'high'];
it1.priority = null;
check('an untouched default priority stays clean when the default changes', save(function () { VS.priority.default = 0; }), []);
it1.priority = 2; VS.priority.default = 1;
check('fields written meanwhile and left alone are kept: only the title goes', save(function (n) { it1.due = '2027-01-01'; it1.priority = 0; }, 'Mine'),
  [[['item.edit', { item: 'i1', title: 'Mine' }]]]);
it1.due = '2026-12-01'; it1.priority = 2;
check('a field the user changed to what another viewer wrote meanwhile is no write', save(function (n) { it1.due = '2027-01-01'; n.due = '2027-01-01'; }), []);
it1.due = '2026-12-01';
// the assignees are a delta over who holds the item now
it1.assignments = [{ to: 'A', state: 'live' }];
check('the prefill is the real assignee', draft().who, ['A']);
check('a swap assigns the new person and unassigns the dropped one', save(function (n) { n.who = ['B']; }),
  [[['item.assign', { item: 'i1', who: ['B'], compl: null }], ['item.unassign', { item: 'i1', assignee: 'A' }]]]);
check('removing an assignee is the unassign alone', save(function (n) { n.who = []; }), [[['item.unassign', { item: 'i1', assignee: 'A' }]]]);
check('a completion pick alone writes nothing', save(function (n) { n.compl = 'assigner'; }), []);
check('a completion pick rides on the added people only', save(function (n) { n.who = ['A', 'B']; n.compl = 'assigner'; }),
  [[['item.assign', { item: 'i1', who: ['B'], compl: 'assigner' }]]]);
check('a person another viewer assigned meanwhile is not assigned again', save(function (n) { it1.assignments.push({ to: 'B', state: 'live' }); n.who = ['A', 'B']; }), []);
it1.assignments = [{ to: 'A', state: 'live' }];
check('adding B does not resurrect A unassigned meanwhile', save(function (n) { it1.assignments = []; n.who = ['A', 'B']; }),
  [[['item.assign', { item: 'i1', who: ['B'], compl: null }]]]);
it1.assignments = [{ to: 'A', state: 'live' }];
check('a person unassigned meanwhile is not unassigned again', save(function (n) { it1.assignments = []; n.who = []; }), []);
it1.assignments = [{ to: 'me', state: 'live', fallback: true }];
check('a task held by its fallback prefills the creator', draft().who, ['me']);
check('and a swap from it is the assign alone (the fallback is not unassigned)', save(function (n) { n.who = ['B']; }), [[['item.assign', { item: 'i1', who: ['B'], compl: null }]]]);
it1.assignments = [];
// an unheld topic once prefilled its creator as holder, so picking the creator wrote nothing
VS.items.t1 = { id: 't1', kind: 'topic', title: 'Topic', labels: [], author: 'me', assignments: [] };
startRowEdit('t1', 'r');
check('an unheld topic prefills no holder', ui.ned.who, []);
ui.ned.who = ['me']; elv.value = 'Topic'; sent = []; saveRowEdit();
check('picking the creator assigns them', sent, [[['item.assign', { item: 't1', who: ['me'], compl: null }]]]);
// a save's success once cleared whichever draft was open, a newer row's included
held = [];
save(function (n) { n.pri = 0; });
var a = ui.ned;
startRowEdit('t1', 'r'); ui.ned.val = 'typing';
held[0]([]); held = null;
check('an earlier save landing keeps the newer draft', [ui.ned !== a, ui.ned.item, ui.ned.val], [true, 't1', 'typing']);
held = [];
save(function (n) { n.pri = 0; });
held[0]([]); held = null;
check('a save landing closes its own draft', ui.ned, null);
canAsg = false;
check('without the assign right the assignees are not written', save(function (n) { n.who = ['you']; }), []);
ui.ned = null; renders = 0;
draft(); sent = []; endRowEdit();
check('cancel closes the row and writes nothing', [sent, ui.ned, renders], [[], null, 2]);
done();
`)
}

// TestTrackerEditRowWrappedStatus pins the row edit's status chip on the
// viewer's done or dropped holding (tracker.js nrowStMode): the store takes no
// item.status there ("reopen it first"), so, as the item menu (canStatus), the
// chip offers no status and a save writes none; a live holding still offers
// the viewer's own statuses.
func TestTrackerEditRowWrappedStatus(t *testing.T) {
	runTrackerCases(t, "row edit wrapped status", trackerCheckJS+`
var rec = null, VS = { viewer: { member: 'me', reach: {} }, priority: { default: 1, names: ['low', 'mid', 'high'] }, labels: [],
  items: { i1: { id: 'i1', kind: 'task', title: 'T', priority: 1, labels: [], author: 'me', assignments: [] } } };
function mineLive() { return rec; }
`+trackerFn(t, `function wrapped\(s\)`, true)+trackerFn(t, `function nrowMine\(n\)`, true)+trackerFn(t, `var NROW_ST = `, true)+trackerFn(t, `function nrowStMode\(n\)`, false)+
		trackerFn(t, `function nrowSt\(n\)`, true)+trackerFn(t, `function nrowStOpts\(n\)`, true)+`
function draft(st) { rec = { status: st }; return { ed: 'i1', kind: 'task', st: st, who: ['me'], canAsg: true }; }
['done', 'dropped'].forEach(function (s) {
  var n = draft(s);
  check('a ' + s + ' holding offers no status chip', [nrowStMode(n), nrowSt(n)], ['', '']);
  n.st = 'todo';
  check('nor writes a picked one (' + s + ')', nrowSt(n), '');
});
var n = draft('doing');
check('a live holding offers the own statuses', [nrowStMode(n), nrowSt(n)], ['own', 'doing']);
n = draft('in-review');
check('an in-review holding offers them plus its own status', [nrowStOpts(n), nrowSt(n)], [['idea', 'todo', 'doing', 'blocked', 'in-review'], 'in-review']);
done();
`)
}

// TestTrackerEditRowClickOpensNothing pins the row edit's click rule (tracker.js
// clickItem, editRowHtml): the edit row keeps the row's data-item, yet a click
// anywhere in it (the kind icon, the room between its chips, the title input)
// opens no item window, while a list row's title and a feed entry still do and
// the dialog never does. The kind icon is plain and names the kind: the store
// cannot change an item's kind (item.edit has no kind field), so it carries
// neither the new row's toggle act nor its "click to make it a ..." tooltip.
func TestTrackerEditRowClickOpensNothing(t *testing.T) {
	click := trackerBlock(t, `document\.addEventListener\('click', function \(e\) \{`, `\n    document\.addEventListener\('submit'`)
	if !strings.Contains(click, "var it = clickItem(t);\n      if (it) { e.preventDefault(); showItem(") {
		t.Error("the click handler's item-window fallback does not go through clickItem")
	}
	runTrackerCases(t, "edit row click", trackerCheckJS+`
var ui = { ned: { item: 'i1', rk: 'r:i1', kind: 'task', val: 'Old' } }, SI = { svg: function (k) { return '<svg>' + k + '</svg>'; } };
function wsT(k) { return k; }
function esc(s) { return s; }
function attr(k, v) { return ' data-' + k + '="' + v + '"'; }
function tipAttr(x) { return ' data-tip="' + x + '"'; }
function kindName(k) { return 'kind:' + k; }
function nrowChips() { return '<chips>'; }
`+trackerFn(t, `function editRowHtml\(o\)`, false)+trackerFn(t, `function clickItem\(t\)`, false)+`
// a fake element tree whose closest() takes the simple selectors clickItem uses (tag, #id, .class, [attr], comma lists)
function matches(e, sel) {
  var m = /^([a-z]*)(#[\w-]+)?((?:\.[\w-]+)*)(?:\[([\w-]+)\])?$/.exec(sel.trim());
  if (!m) throw new Error('unsupported selector ' + sel);
  return (!m[1] || e.tag === m[1]) && (!m[2] || e.id === m[2].slice(1)) &&
    m[3].split('.').filter(Boolean).every(function (c) { return e.cls.indexOf(c) >= 0; }) && (!m[4] || m[4] in e.attrs);
}
function el(tag, o, parent) {
  return { tag: tag, id: o.id || '', cls: (o.cls || '').split(' ').filter(Boolean), attrs: o.attrs || {}, parent: parent || null,
    closest: function (sel) { for (var e = this; e; e = e.parent) if (sel.split(',').some(function (s) { return matches(e, s); })) return e; return null; } };
}
var h = editRowHtml({});
check('the edit row is marked as the edit and keeps the row ids', [h.indexOf('<div class="nrow ned" data-item="i1" data-rk="r:i1">') === 0], [true]);
check('its kind icon is a plain span naming the kind, no toggle', [h.indexOf('<span class="nkind" data-tip="kind:task"><svg>todo</svg></span>') > 0, h.indexOf('nkind"' + ' data-act') < 0, h.indexOf('js.tk.new.kind') < 0], [true, true, true]);
ui.ned.kind = 'topic';
check('a topic names the topic kind', editRowHtml({}).indexOf('<span class="nkind" data-tip="kind:topic"><svg>topic</svg></span>') > 0, true);
var row = el('div', { cls: 'nrow ned', attrs: { 'data-item': 'i1' } }), kind = el('span', { cls: 'nkind' }, row), icon = el('svg', {}, kind), inp = el('input', { id: 'tkned' }, row);
check('a click on the edit row\'s kind icon, its gaps, or its input opens nothing', [clickItem(kind), clickItem(icon), clickItem(row), clickItem(inp)], [null, null, null, null]);
var irow = el('div', { cls: 'irow', attrs: { 'data-item': 'i2' } }), title = el('a', { cls: 'title', attrs: { 'data-item': 'i2' } }, el('div', { cls: 'ir-main' }, irow));
check('a list row\'s title still opens its item', clickItem(title) === title, true);
var entry = el('div', { cls: 'row click', attrs: { 'data-item': 'i3' } }), snip = el('span', {}, entry);
check('a feed entry still opens its item', clickItem(snip) === entry, true);
var dlg = el('div', { id: 'tkdlg' }), inDlg = el('span', { attrs: { 'data-item': 'i4' } }, dlg);
check('nothing in the dialog opens an item', clickItem(inDlg), null);
done();
`)
}

// TestTrackerInsertDepthMarkers pins the hover insert's depth markers: every
// level slot from the root to one under the row above, but the badge's own,
// carries a marker; a tick where a place is (insOpts), an x where none is.
// The guide line starts past the deepest slot when the badge is shallower.
func TestTrackerInsertDepthMarkers(t *testing.T) {
	show := trackerFn(t, `function insShow\(list, g, x\)`, false)
	for _, want := range []string{
		`'<div class="hins" aria-hidden="true"><span class="hdep"></span><span class="hl"></span>`,
		`max = g.a ? rowDepth(g.a) + 1 : 0, marks = insMarks(opts, o.depth, max)`,
		`s.style.setProperty('--hmax', max);`,
		`s.classList.toggle('hdeep', max > o.depth);`,
	} {
		if !strings.Contains(show, want) {
			t.Errorf("insShow lacks %q:\n%s", want, show)
		}
	}
	runTrackerCases(t, "insert depth markers", trackerCheckJS+"var INS_X = 'X';\n"+trackerFn(t, `function insMarks\(opts, pick, max\)`, false)+`
function o(ds) { return ds.map(function (d) { return { depth: d }; }); }
check('shallower ticks, deeper x where a parent cannot take the item', insMarks(o([0, 2, 3]), 0, 3),
  '<span class="hx" style="--d:1">X</span><span class="hk" style="--d:2"></span><span class="hk" style="--d:3"></span>');
check('levels above the row below are blocked', insMarks(o([1, 2]), 2, 2),
  '<span class="hx" style="--d:0">X</span><span class="hk" style="--d:1"></span>');
check('the top of the list: only the badge slot, no markers', insMarks(o([0]), 0, 0), '');
done();
`)
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		".tk .hins.hdeep .hl { left: calc(4px + var(--hmax, 0) * 16px + 16px); }",
		".tk .hins .hdep { position: absolute; inset: 0; pointer-events: none; }",
		".tk .hins .hk, .tk .hins .hx { position: absolute; left: calc(8px + var(--d, 0) * 16px); width: 8px; }",
		"background: color-mix(in srgb, var(--wb-fg) 34%, transparent)",
		"color: color-mix(in srgb, var(--wb-fail) 60%, transparent)",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("tracker.css lacks the depth markers' %q", want)
		}
	}
}

// TestTrackerInsertParentOutline pins the hover insert's parent outline:
// the row that would parent the badge's place (the nearest row above the gap
// one level shallower, when it shows the place's parent) carries .hpar; a
// root place outlines nothing; a depth change moves the outline; hiding the
// insert clears it, and so does a re-render.
func TestTrackerInsertParentOutline(t *testing.T) {
	if show := trackerFn(t, `function insShow\(list, g, x\)`, false); !strings.Contains(show, "insPar(insParRow(g.a, o));") {
		t.Errorf("insShow does not outline the place's parent:\n%s", show)
	}
	if n := strings.Count(trackerBlock(t, `\n  function render\(`, `\n  function `), "insPar(null); HV = null;"); n != 2 {
		t.Errorf("render clears the parent outline at %d of its 2 insert resets", n)
	}
	src := trackerFn(t, `function rowDepth\(el\)`, true) + trackerFn(t, `function insParRow\(a, o\)`, false) +
		trackerFn(t, `function insPar\(el\)`, false) + trackerFn(t, `function insHide\(\)`, false)
	runTrackerCases(t, "insert parent outline", trackerCheckJS+`
var HV = null, HP = null;
`+src+`
function row(id, d, cls) {
  var c = {};
  return { id: id, cls: cls || 'irow', style: { getPropertyValue: function () { return String(d); } },
    getAttribute: function () { return id; }, matches: function (q) { return q === '.' + this.cls; },
    classList: { add: function (k) { c[k] = true; }, remove: function (k) { delete c[k]; }, has: function (k) { return !!c[k]; } } };
}
// A > A1 > A1a, A > A2, a section head, B
var rows = [row('A', 0), row('A1', 1), row('A1a', 2), row('A2', 1), row('h', 0, 'sub'), row('B', 0)];
rows.forEach(function (r, i) { r.previousElementSibling = rows[i - 1] || null; });
function outlined() { return rows.filter(function (r) { return r.classList.has('hpar'); }).map(function (r) { return r.id; }); }
function at(a, depth, parent) { insPar(insParRow(rows[a], { depth: depth, parent: parent })); return outlined(); }
check('first child of A1a: A1a', at(2, 3, 'A1a'), ['A1a']);
check('after A1a under A1: A1', at(2, 2, 'A1'), ['A1']);
check('after A1 under A: A, past the deeper row', at(2, 1, 'A'), ['A']);
check('a root place: none', at(2, 0, null), []);
check('after A2 under A: A, past its sibling', at(3, 1, 'A'), ['A']);
check('a row the place does not name: none', at(3, 1, 'X'), []);
check('first child of B past a section head: B', at(5, 1, 'B'), ['B']);
check('above the first row: none', (insPar(insParRow(null, { depth: 0, parent: null })), outlined()), []);
at(2, 2, 'A1');
HV = { list: { classList: { remove: function () {} }, querySelector: function () { return { hidden: false }; } } };
insHide();
check('hiding the insert clears the outline', [outlined(), HP, HV], [[], null, null]);
done();
`)
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".tk .irow.hpar > .ir-main { box-shadow: inset 0 0 0 1px color-mix(in srgb, var(--wb-primary) 45%, transparent); }") {
		t.Error("tracker.css lacks the parent outline")
	}
}

// trackerTakeJS is the take-rule copies of tracker.js with the helpers they
// read, for node cases over a hand-built VS.
func trackerTakeJS(t *testing.T) string {
	return trackerFn(t, `function wrapped\(s\)`, true) + trackerFn(t, `function A\(id\)`, true) + trackerFn(t, `function listOf\(x\)`, true) +
		trackerFn(t, `function isCreator\(it\)`, false) + trackerFn(t, `function canAnswer\(it, q\)`, true) + trackerFn(t, `function canApprove\(it, q\)`, true) +
		trackerFn(t, `function fallbackRec\(it\)`, true) + trackerFn(t, `function holdsOn\(it, who\)`, true) + trackerFn(t, `function canOpen\(it\)`, false) +
		trackerFn(t, `function canUnopen\(it\)`, true) + trackerFn(t, `function reqsOf\(it\)`, true) + trackerFn(t, `function reqTargets\(it\)`, false)
}

// TestTrackerTakeRules pins the dashboard's copies of the store's take rules
// (check.go item.approve / item.deny): the holder of the named assignment
// or the creator (an admin once the creator was removed) answers a pending
// request; approving also needs that assignment not done or dropped
// (check.go takeWrapped), while a denial stays open.
func TestTrackerTakeRules(t *testing.T) {
	runTrackerCases(t, "take rules", trackerCheckJS+trackerTakeJS(t)+`
var VS = { viewer: { member: 'me', admin: false, isMember: true }, members: { ada: {}, me: {}, kim: {}, gone: { removed: true } }, assignments: {} };
function rec(id, to, status, o) { var r = { id: id, to: to, status: status, state: 'live', fallback: false, ruleRequest: false }; Object.keys(o || {}).forEach(function (k) { r[k] = o[k]; }); VS.assignments[id] = r; return r; }
var it = { id: 'I', author: 'ada' };
rec('I>me', 'me', 'doing');
var q = { id: 'Q', by: 'kim', assignee: 'me', assignment: 'I>me', state: 'pending' };
check('the holder answers and approves', [canAnswer(it, q), canApprove(it, q)], [true, true]);
check('another member does not', [canAnswer({ id: 'I', author: 'ada' }, { by: 'kim', assignee: 'ada', assignment: 'I>ada', state: 'pending' })], [false]);
check('the creator does', canAnswer({ id: 'I', author: 'me' }, { by: 'kim', assignee: 'ada', assignment: 'I>ada', state: 'pending' }), true);
VS.viewer.admin = true;
check('an admin once the creator was removed', canAnswer({ id: 'I', author: 'gone' }, { by: 'kim', assignee: 'ada', assignment: 'I>ada', state: 'pending' }), true);
check('not while the creator stays', canAnswer({ id: 'I', author: 'ada' }, { by: 'kim', assignee: 'ada', assignment: 'I>ada', state: 'pending' }), false);
VS.viewer.admin = false;
check('an answered request is not answered again', canAnswer(it, { by: 'kim', assignee: 'me', assignment: 'I>me', state: 'denied' }), false);
VS.assignments['I>me'].status = 'done';
check('a done assignment: deny only', [canAnswer(it, q), canApprove(it, q)], [true, false]);
VS.assignments['I>me'].status = 'dropped';
check('a dropped assignment: deny only', [canAnswer(it, q), canApprove(it, q)], [true, false]);
VS.assignments['I>me'].status = 'in-review';
check('in review is not wrapped up', canApprove(it, q), true);
done();
`)
}

// TestTrackerAcceptRule pins the dashboard's copy of the store's accept
// rule (check.go item.accept / handoffWrapped): the viewer accepts while
// they hold a pending record on the item and none of their pending records
// is a handoff of a done or dropped assignment; actList still offers the
// decline then.
func TestTrackerAcceptRule(t *testing.T) {
	if !strings.Contains(trackerFn(t, `function actList\(it\)`, false), "if (canAccept(it)) out.push(['accept', 'accept', wsT(\"js.tk.act.accept\"), { item: id }, 1]);\n      out.push(['note', 'decline'") {
		t.Error("actList does not gate accept alone on canAccept")
	}
	runTrackerCases(t, "accept rule", trackerCheckJS+trackerFn(t, `function wrapped\(s\)`, true)+trackerFn(t, `function A\(id\)`, true)+trackerFn(t, `function canAccept\(it\)`, false)+`
var VS = { viewer: { member: 'me' }, assignments: {} };
function rec(id, to, state, status, o) { var r = { id: id, to: to, state: state, status: status }; Object.keys(o || {}).forEach(function (k) { r[k] = o[k]; }); VS.assignments[id] = r; return id; }
var it = { id: 'I', assignments: [rec('I>kim', 'kim', 'live', 'doing'), rec('H>me', 'me', 'pending', 'todo', { handoffOf: 'I>kim' })] };
check('a pending handoff of a live assignment is accepted', canAccept(it), true);
VS.assignments['I>kim'].status = 'done';
check('not once that assignment is done', canAccept(it), false);
VS.assignments['I>kim'].status = 'dropped';
check('nor dropped', canAccept(it), false);
VS.assignments['I>kim'].status = 'in-review';
check('in review is not wrapped up', canAccept(it), true);
check('a plain pending assignment is accepted', canAccept({ id: 'J', assignments: [rec('J>me', 'me', 'pending', 'todo')] }), true);
check('nothing pending: nothing to accept', canAccept({ id: 'K', assignments: [rec('K>me', 'me', 'live', 'doing')] }), false);
done();
`)
}

// TestTrackerOpenAndRequestRules pins the dashboard's copies of the store's
// opening and take-request rules. canOpen copies check.go item.open: the
// creator (an admin once the creator was removed) opens a task, not hidden,
// not already open, that nobody but its fallback holds (a rule request
// holds nothing), the fallback not done or dropped. canUnopen copies
// item.unopen: the creator withdraws an open opening. reqTargets copies
// item.request: the live, explicit (not fallback), not wrapped-up
// assignments of others on a shown item, none while the viewer holds a live
// or pending record there, and not one the viewer already asked for.
func TestTrackerOpenAndRequestRules(t *testing.T) {
	runTrackerCases(t, "open and request rules", trackerCheckJS+trackerTakeJS(t)+`
var VS = { viewer: { member: 'me', admin: false, isMember: true }, members: { ada: {}, me: {}, kim: {}, lee: {}, gone: { removed: true } }, assignments: {}, requests: {} };
function rec(id, to, state, status, o) { var r = { id: id, to: to, state: state, status: status, fallback: false, ruleRequest: false }; Object.keys(o || {}).forEach(function (k) { r[k] = o[k]; }); VS.assignments[id] = r; return id; }
function item(author, recs, o) { var it = { id: 'I', author: author, kind: 'task', hidden: false, open: null, assignments: recs, requests: [] }; Object.keys(o || {}).forEach(function (k) { it[k] = o[k]; }); return it; }
var fb = rec('fb', 'me', 'live', 'todo', { fallback: true }), fbDone = rec('fbDone', 'me', 'live', 'done', { fallback: true });
check('open: the creator, a task held only by its fallback', canOpen(item('me', [fb])), true);
check('open: not another member', canOpen(item('ada', [rec('fbA', 'ada', 'live', 'todo', { fallback: true })])), false);
VS.viewer.admin = true;
check('open: an admin once the creator was removed', canOpen(item('gone', [rec('fbG', 'gone', 'live', 'todo', { fallback: true })])), true);
check('open: not an admin while the creator stays', canOpen(item('ada', ['fbA'])), false);
VS.viewer.admin = false;
check('open: not hidden', canOpen(item('me', [fb], { hidden: true })), false);
check('open: not a topic', canOpen(item('me', [fb], { kind: 'topic' })), false);
check('open: not already open', canOpen(item('me', [fb], { open: { by: 'me', scope: 'team' } })), false);
check('open: not while someone else holds it', canOpen(item('me', [rec('kimLive', 'kim', 'live', 'doing')])), false);
check('open: not while a record is pending', canOpen(item('me', [fb, rec('kimPend', 'kim', 'pending', 'todo')])), false);
check('open: a pending rule request holds nothing', canOpen(item('me', [fb, rec('rr', 'kim', 'pending', 'todo', { ruleRequest: true })])), true);
check('open: not a done fallback', canOpen(item('me', [fbDone])), false);

check('unopen: the creator, while open', canUnopen(item('me', [fb], { open: { by: 'me' } })), true);
check('unopen: not while not open', canUnopen(item('me', [fb])), false);
check('unopen: not another member', canUnopen(item('ada', ['fbA'], { open: { by: 'ada' } })), false);
VS.viewer.admin = true;
check('unopen: an admin once the creator was removed', canUnopen(item('gone', ['fbG'], { open: { by: 'gone' } })), true);
VS.viewer.admin = false;

function ids(l) { return l.map(function (r) { return r.id; }); }
var kim = rec('kim', 'kim', 'live', 'doing'), lee = rec('lee', 'lee', 'live', 'done'), leeDrop = rec('leeDrop', 'lee', 'live', 'dropped');
check('request: a live assignment of someone else', ids(reqTargets(item('ada', [kim]))), ['kim']);
check('request: not a done or dropped one', ids(reqTargets(item('ada', [kim, lee]))).concat(ids(reqTargets(item('ada', [leeDrop])))), ['kim']);
check('request: not a pending one', ids(reqTargets(item('ada', [rec('kimP', 'kim', 'pending', 'todo')]))), []);
check('request: not a fallback holding', ids(reqTargets(item('ada', [rec('fbA2', 'ada', 'live', 'todo', { fallback: true })]))), []);
check('request: not the viewer own', ids(reqTargets(item('ada', [rec('meLive', 'me', 'live', 'doing')]))), []);
check('request: none while the viewer holds a live record', ids(reqTargets(item('ada', [kim, 'meLive']))), []);
check('request: none while the viewer holds a pending record', ids(reqTargets(item('ada', [kim, rec('meP', 'me', 'pending', 'todo')]))), []);
check('request: a rule request to the viewer holds nothing', ids(reqTargets(item('ada', [kim, rec('meRR', 'me', 'pending', 'todo', { ruleRequest: true })]))), ['kim']);
check('request: not on a hidden item', ids(reqTargets(item('ada', [kim], { hidden: true }))), []);
VS.viewer.isMember = false;
check('request: not by a non-member', ids(reqTargets(item('ada', [kim]))), []);
VS.viewer.isMember = true;
VS.requests = { Q1: { id: 'Q1', by: 'me', assignee: 'kim', assignment: 'kim', state: 'pending' }, Q2: { id: 'Q2', by: 'me', assignee: 'kim', assignment: 'kim', state: 'denied' }, Q3: { id: 'Q3', by: 'ada', assignee: 'kim', assignment: 'kim', state: 'pending' } };
check('request: not one the viewer already asked for', ids(reqTargets(item('ada', [kim], { requests: ['Q1'] }))), []);
check('request: an answered request or another member pending one does not block', ids(reqTargets(item('ada', [kim], { requests: ['Q2', 'Q3'] }))), ['kim']);
done();
`)
}

// TestTrackerMoveRules pins the dashboard's copy of the store's move rule
// (check.go checkMove): the item rule is the creator or a live assignee (the
// fallback included; an admin too once the creator was removed); only when
// the parent changes, the destination must be shown, the viewer's (created
// or live-assigned, with no removed-creator fallback), and not the item or
// inside its subtree. A same-parent reorder and a move to the top level take
// the item rule alone. It also pins moveRoots: the members of a selection
// with no ancestor in it, in tree order.
func TestTrackerMoveRules(t *testing.T) {
	src := trackerFn(t, `function alias\(id\)`, true) + trackerFn(t, `function A\(id\)`, true) + trackerFn(t, `function mineLive\(it\)`, true) +
		trackerFn(t, `function canMove\(it\)`, false) + trackerFn(t, `function moveWhy\(id, dest\)`, false) + trackerFn(t, `function moveRoots\(ids\)`, false)
	runTrackerCases(t, "move rules", trackerCheckJS+src+`
function wsT(k, a) { return k.replace('js.tk.mv.why.', '') + ':' + a; }
var VS = { viewer: { member: 'me', admin: false }, members: { me: {}, kim: {}, gone: { removed: true } }, items: {}, itemOrder: [],
  assignments: { cMe: { to: 'me', state: 'live' }, fbMe: { to: 'me', state: 'live', fallback: true }, dPend: { to: 'me', state: 'pending' } } };
function it(id, author, parent, o) { var x = { id: id, alias: id, author: author, parent: parent || null, hidden: false, assignments: [] }; Object.keys(o || {}).forEach(function (k) { x[k] = o[k]; }); VS.items[id] = x; VS.itemOrder.push(id); }
// A (me) > A1 (me) > A1a (kim); B (kim); C (kim, me live); F (kim, me as fallback); P (kim, me pending); H (me, hidden); G (removed creator)
it('A', 'me'); it('A1', 'me', 'A'); it('A1a', 'kim', 'A1'); it('B', 'kim'); it('C', 'kim', null, { assignments: ['cMe'] });
it('F', 'kim', null, { assignments: ['fbMe'] }); it('P', 'kim', null, { assignments: ['dPend'] }); it('H', 'me', null, { hidden: true }); it('G', 'gone');
check('to the top level: the item rule alone', moveWhy('A1', null), '');
check('a same-parent reorder: the item rule alone', moveWhy('A1', 'A'), '');
check('not an item the viewer neither created nor holds', moveWhy('B', null), 'mine:B');
check('a pending assignee does not hold it', moveWhy('P', null), 'mine:P');
check('a live assignee moves it', moveWhy('C', 'A'), '');
check('the fallback holder moves it', moveWhy('F', 'A'), '');
check('not a hidden item', moveWhy('H', null), 'hidden:H');
check('not under a hidden parent', moveWhy('A1', 'H'), 'hidden:H');
check('not under an unknown parent', moveWhy('A1', 'x#9'), 'hidden:x-9');
check('not under a parent the viewer neither created nor holds', moveWhy('A1', 'B'), 'mine:B');
check('under a parent the viewer holds live', moveWhy('A1', 'C'), '');
check('not under itself', moveWhy('A', 'A'), 'sub:A');
check('not into its own subtree', moveWhy('A', 'A1'), 'sub:A');
check('a removed creator item: not for a member', moveWhy('G', null), 'mine:G');
VS.viewer.admin = true;
check('a removed creator item: an admin moves it', moveWhy('G', 'A'), '');
check('a removed creator item is no destination even for an admin', moveWhy('A1', 'G'), 'mine:G');
VS.viewer.admin = false;
check('roots: an ancestor in the set carries its subtree; unknown ids drop', moveRoots(['A1a', 'B', 'A', 'zz']), ['A', 'B']);
check('roots: in tree order, not selection order', moveRoots(['C', 'A1a', 'A1']), ['A1', 'C']);
done();
`)
}

// TestTrackerRowPartsOpenTheirPickers pins the list row's click targets:
// only the title opens the item window; a click elsewhere on the row only
// focuses it (Enter on the focused row still opens it); the assignee
// avatars, the priority, and the due date open the item menu's assign,
// pri, and due steps when the viewer may change them (canAssign, canEdit on
// a task) and stay plain read-only spans otherwise; an empty assignee or
// due slot the viewer may set holds a dim icon, and an editor gets a label
// icon opening the labels step while the label chips keep searching. In
// selection mode a part's click stays the row's selection toggle, and the
// labels step opened from a row re-anchors on that row's icon after a
// toggle redraws the list; the scroll a render's restore fires keeps the
// item menu while its opener stays put.
func TestTrackerRowPartsOpenTheirPickers(t *testing.T) {
	irow := trackerFn(t, `function irow\(o\)`, false)
	for _, want := range []string{
		"ed = canEdit(it) && it.kind !== 'topic'",
		"labelChips(it, 'lsearch') + warnHtml(it) + dotHtml(it) + labelEdHtml(it) + '<span class=\"grow\"></span>'",
		"whoHtml(it, canAssign(it, VS.viewer.member)) + priHtml(it, ed) + dueHtml(it, st, ed)",
	} {
		if !strings.Contains(irow, want) {
			t.Errorf("irow lacks %q", want)
		}
	}
	steps := trackerFn(t, `var STEPS = `, true)
	for _, k := range []string{"assign: 1", "pri: 1", "due: 1", "labels: 1"} {
		if !strings.Contains(steps, k) {
			t.Errorf("STEPS lacks %q, so a row part's act opens no item menu step", k)
		}
	}
	click := trackerBlock(t, `document\.addEventListener\('click', function \(e\) \{`, `\n    document\.addEventListener\('submit'`)
	for _, want := range []string{
		"else if (STEPS[k]) itemMenu(d.item, a, null, { step: k, op: d.op, who: d.who, req: d.req });",
		"var prow = mselOn() && a.classList.contains('rp') && a.closest('#card-items .irow');\n        if (prow) mselClick(prow, e);",
		"if (row && !rowClick(row, t)) { e.preventDefault(); return; }",
	} {
		if !strings.Contains(click, want) {
			t.Errorf("the click handler lacks %q", want)
		}
	}
	if i, j := strings.Index(click, "rowClick(row, t)"), strings.Index(click, "var it = clickItem(t);"); i < 0 || j < 0 || i > j {
		t.Error("a row click does not stop before the [data-item] fallback that opens the item window")
	}
	if !strings.Contains(trackerBlock(t, `// keyboard: a focused row shows`, `\n`+"      // ArrowRight"), "if (e.key === 'Enter' && e.target.classList && e.target.classList.contains('irow') && own(e.target)) { e.preventDefault(); showItem(e.target.getAttribute('data-item')); return; }") {
		t.Error("Enter on a focused row no longer opens its item window")
	}

	src := trackerFn(t, `function esc\(s\)`, false) + trackerFn(t, `function attr\(k, v\)`, true) + trackerFn(t, `function tipAttr\(text\)`, true) +
		trackerFn(t, `function labelOf\(id\)`, true) + trackerFn(t, `function labelChip\(l, act, extra\)`, false) + trackerFn(t, `function labelChips\(it, act\)`, false) +
		trackerFn(t, `function partHtml\(cls, tip, inner, it, step\)`, false) + trackerFn(t, `function partIcon\(icon\)`, true) +
		trackerFn(t, `function whoHtml\(it, ed\)`, false) + trackerFn(t, `function priHtml\(it, ed\)`, false) + trackerFn(t, `var ENDED = `, true) +
		trackerFn(t, `function dueHtml\(it, st, ed\)`, false) + trackerFn(t, `function labelEdHtml\(it\)`, false) + trackerFn(t, `function dayNum\(d\)`, true) +
		trackerFn(t, `function today\(\)`, true) + trackerFn(t, `function titleEdHtml\(it\)`, false) + irow + trackerFn(t, `function rowClick\(row, t\)`, false) + trackerFn(t, `function labelStepRedraw\(\)`, false) +
		trackerFn(t, `function openerMoved\(\)`, false)
	if !strings.Contains(trackerBlock(t, `document\.addEventListener\('scroll', function \(e\) \{`, `\n    \}, true\);`), "&& openerMoved()) closeMenu(false);") {
		t.Error("a scroll closes the item menu whether or not its opener moved")
	}
	runTrackerCases(t, "row parts", trackerCheckJS+`
var VS = { now: '2026-11-12T09:00:00Z', viewer: { member: 'me', admin: false }, priority: { names: ['low', 'mid', 'high'], default: 1 },
  labels: [{ id: 'L1', name: 'docs', color: '#111111' }], items: {} };
var ui = { sel: null }, MSEEN = null, edit = {}, assign = {};
function wsT(k) { return k; }
function wsTn(k) { return k; }
var SI = { info: function (s) { return { label: s }; } };
function holders(it) { return it.hs; }
function avHtml(id) { return '<i class="av">' + id + '</i>'; }
function avRel() { return { t: '' }; }
function nmo(id) { return id; }
function stKey(it) { return it.status; }
function mselHas() { return false; }
function canMove() { return false; }
function canEdit(it) { return !!edit[it.id]; }
function canAssign(it) { return !!assign[it.id]; }
function mcbHtml() { return ''; }
function stBtn() { return '<st>'; }
function openChip() { return ''; }
function warnHtml() { return ''; }
function dotHtml() { return ''; }
function moreBtn() { return '<more>'; }
function detailsHtml() { return ''; }
`+src+`
function has(s, x) { return s.indexOf(x) >= 0; }
function item(id, o) { var x = { id: id, title: 'T ' + id, kind: 'task', status: 'todo', hs: [], labels: [], priority: 1, priorityLevel: 1, priorityName: 'mid', due: '' }; Object.keys(o || {}).forEach(function (k) { x[k] = o[k]; }); VS.items[id] = x; return x; }
item('a', { hs: [{ to: 'kim', state: 'live' }], due: '2026-11-15', labels: ['L1'] });
edit.a = assign.a = true;
var ra = irow({ rk: 'r:a', item: 'a' });
check('the title is the item window link', has(ra, '<a class="title" href="#" tabindex="-1" data-item="a">T a</a>'), true);
check('the avatars open the assign step', has(ra, '<button type="button" class="who rp" data-act="assign" data-item="a" tabindex="-1" aria-haspopup="dialog"'), true);
check('the priority opens the pri step', has(ra, '<button type="button" class="pri rp" data-act="pri" data-item="a" tabindex="-1" aria-haspopup="menu"'), true);
check('the due date opens the due step', has(ra, '<button type="button" class="due2 soon rp" data-act="due" data-item="a" tabindex="-1" aria-haspopup="menu"'), true);
check('a label chip still searches', has(ra, 'class="chip lbl" data-act="lsearch" data-label="L1"'), true);
check('the label icon opens the labels step', has(ra, '<button type="button" class="rlab rpe rp" data-act="labels" data-item="a"'), true);
check('filled slots carry no dim icon', [has(ra, 'who rpe'), has(ra, 'due2 rpe')], [false, false]);

item('b');
edit.b = assign.b = true;
var rb = irow({ rk: 'r:b', item: 'b' });
check('an empty assignee slot the viewer may set: the dim assign icon', has(rb, '<button type="button" class="who rpe rp" data-act="assign" data-item="b"') && has(rb, '#tki-assign'), true);
check('an empty due slot the viewer may set: the dim calendar icon', has(rb, '<button type="button" class="due2 rpe rp" data-act="due" data-item="b"') && has(rb, '#tki-calendar'), true);

item('c', { hs: [{ to: 'kim', state: 'live' }], due: '2026-12-24', labels: ['L1'] });
var rc = irow({ rk: 'r:c', item: 'c' });
check('no right: every part a plain span with its tooltip, no picker', [has(rc, '<span class="who" data-tip='), has(rc, '<span class="pri" data-tip='), has(rc, '<span class="due2" data-tip='), has(rc, 'data-act="assign"'), has(rc, 'data-act="pri"'), has(rc, 'data-act="due"'), has(rc, 'data-act="labels"'), has(rc, ' rp"')], [true, true, true, false, false, false, false, false]);
item('d');
var rd = irow({ rk: 'r:d', item: 'd' });
check('no right, empty slots: quiet, no icon', [has(rd, '<span class="who"></span>'), has(rd, '<span class="due2"></span>'), has(rd, 'rpe')], [true, true, false]);
edit.c = true;
check('an editor who may not assign: priority and due open, the avatars stay inert', [has(irow({ rk: 'r:c', item: 'c' }), '<span class="who" data-tip='), has(irow({ rk: 'r:c', item: 'c' }), 'data-act="pri"')], [true, true]);

item('t', { kind: 'topic', priorityName: '' });
edit.t = assign.t = true;
var rt = irow({ rk: 'r:t', item: 't' });
check('a topic: the holder slot opens assign; no priority or due', [has(rt, 'data-act="assign"'), has(rt, 'data-act="pri"'), has(rt, 'data-act="due"'), has(rt, 'data-act="labels"')], [true, false, false, true]);
VS.labels = [];
check('no labels in the store: no label icon for a member', has(irow({ rk: 'r:a', item: 'a' }), 'data-act="labels"'), false);
VS.viewer.admin = true;
check('no labels in the store: an admin, who can add one, gets it', has(irow({ rk: 'r:a', item: 'a' }), 'data-act="labels"'), true);

// rowClick over a fake row: the title goes on to open the item window, anything else only focuses the row
var focused = 0, row = { focus: function () { focused++; } };
function tgt(title) { return { closest: function (q) { return q === 'a.title' && title ? {} : null; } }; }
check('a title click goes on to open the item window', [rowClick(row, tgt(true)), focused], [true, 0]);
check('an empty-space click only focuses the row', [rowClick(row, tgt(false)), focused], [false, 1]);

// the labels step opened from a row re-anchors on the redrawn row's label icon; from the property card on its button
var closed = 0, drawn = 0, btn = null, MN, box = { innerHTML: '', querySelector: function () { return null; } };
function closeMenu() { closed++; }
function menuHtml() { drawn++; return ''; }
function placeMenu() {}
function $(id) { return id === 'tkmenu' ? box : null; }
var CSS = { escape: function (s) { return s; } };
function mkBtn(n) { return { n: n, attrs: {}, matches: function (q) { return q === '[data-act="labels"]'; }, setAttribute: function (k, v) { this.attrs[k] = v; }, getBoundingClientRect: function () { return { n: n }; } }; }
var rowBtn = mkBtn('row'), cardBtn = mkBtn('card');
var rowEl = { matches: function () { return false; }, querySelector: function (q) { return q === '[data-act="labels"]' ? rowBtn : null; } };
var document = { querySelector: function (q) { return q === '[data-rk="r:a"]' ? rowEl : q === '#iw-plabels' ? cardBtn : null; } };
VS.labels = [{ id: 'L1', name: 'docs', color: '#111111' }];
MN = { item: 'a', back: '[data-rk="r:a"]' };
labelStepRedraw();
check('from a row: anchored on the row icon, kept open', [MN.backEl && MN.backEl.n, MN.rect.n, rowBtn.attrs['aria-expanded'], closed, drawn], ['row', 'row', 'true', 0, 1]);
MN = { item: 'a', back: '#iw-plabels' };
labelStepRedraw();
check('from the property card: anchored on its button', [MN.backEl && MN.backEl.n, closed], ['card', 0]);
edit.a = false;
labelStepRedraw();
check('the edit right gone: the step closes', closed, 1);

function opener(top, left, on) { return { isConnected: on !== false, getBoundingClientRect: function () { return { top: top, left: left }; } }; }
MN = { backEl: opener(100, 40), rect: { top: 100, left: 40 } };
check('a scroll with the opener in place keeps the menu', openerMoved(), false);
MN.backEl = opener(80, 40);
check('a scroll that moved the opener closes it', openerMoved(), true);
MN.backEl = opener(100, 40, false);
check('a detached opener closes it', openerMoved(), true);
MN = { backEl: null, rect: null };
check('no opener closes it', openerMoved(), true);
done();
`)
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		".tk :where(button.rp) { font: inherit; color: inherit; border: 0; background: none; margin: 0; padding: 0; }",
		".tk .irow:is(:hover, :focus-within, .sel) .rpe, .tk .rpe[aria-expanded=\"true\"] { visibility: visible; }",
		".tk .irow:is(:hover, :focus-within, .sel) :is(.rlab, .rtit), .tk .rlab[aria-expanded=\"true\"] { display: inline-flex; }",
		".tk .irow:is(:hover, :focus-visible, :has(:focus-visible)) > .ir-more { visibility: visible; }",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("tracker.css lacks %q", want)
		}
	}
	if strings.Contains(string(css), ":focus-within, .sel) > .ir-more") {
		t.Error("a clicked row keeps its preview after the pointer leaves: the preview shows on hover and keyboard focus only")
	}
	if strings.Contains(string(css), "border-radius: 6px; cursor: pointer; }\n.tk .ir-main .title") {
		t.Error("the whole row still shows the pointer though only its title opens the item window")
	}
}

// TestTrackerAssignCompletionBelowTheViewer pins the assign popover's
// completion for a target below the viewer (the store's write-time rule,
// check.go checkWrite): such a target is always written with completion
// assigner, so the radio hides "assignee" while one is picked; a target not
// below keeps the user's pick or the default class. The quick-add row's
// picks go through the same asgEvents.
func TestTrackerAssignCompletionBelowTheViewer(t *testing.T) {
	draw := trackerFn(t, `function asgDraw\(\)`, false)
	if !strings.Contains(draw, `b.hidden = b.getAttribute('data-c') === 'assignee' && asgBelow(S);`) {
		t.Errorf("asgDraw does not hide the assignee radio for a pick below the viewer:\n%s", draw)
	}
	if key := trackerFn(t, `function asgMenuKey\(e\)`, false); !strings.Contains(key, `'[data-c]:not([hidden]):not([data-c="'`) {
		t.Errorf("the radio's arrow keys can land on the hidden option:\n%s", key)
	}
	src := trackerFn(t, `function esc\(s\)`, false) + trackerFn(t, `function attr\(k, v\)`, true) + trackerFn(t, `function tipAttr\(text\)`, true) +
		trackerFn(t, `function complText\(c\)`, true) + trackerFn(t, `function asgState\(\)`, true) + trackerFn(t, `function complClass\(o\)`, false) +
		trackerFn(t, `function asgClass\(o, compl\)`, true) + trackerFn(t, `function asgBelow\(S\)`, true) + trackerFn(t, `function asgEvents\(item, sel, compl\)`, false) +
		trackerFn(t, `function asgCompl\(S\)`, false) + trackerFn(t, `function asgHtml\(it\)`, false)
	runTrackerCases(t, "assign completion", trackerCheckJS+src+`
function wsT(k) { return k; }
function asgListHtml() { return ''; }
function asgChipsHtml() { return ''; }
// kim and lee are below me; pat is a peer
var VS = { viewer: { member: 'me', reach: { kim: 1, lee: 2 } } }, MN = {};
function pick(sel, compl) { MN.as = { q: '', sel: sel, compl: compl || null, idx: 0, opts: [] }; return MN.as; }
function offered(S) { var h = asgHtml({ kind: 'task' }); return ['assignee', 'assigner'].filter(function (c) { return h.indexOf('data-c="' + c + '">') >= 0; }); }
var P = function (id) { return { t: 'p', id: id }; };

var S = pick([P('pat')]);
check('a peer: both offered', offered(S), ['assignee', 'assigner']);
check('a peer: assignee by default', [asgCompl(S), asgEvents('I', S.sel, S.compl)], ['assignee', [['item.assign', { item: 'I', targets: ['pat'], completion: 'assignee' }]]]);
S = pick([P('pat')], 'assigner');
check('a peer: the pick holds', asgEvents('I', S.sel, S.compl), [['item.assign', { item: 'I', targets: ['pat'], completion: 'assigner' }]]);

S = pick([P('kim')]);
check('below: assignee not offered', offered(S), ['assigner']);
check('below: assigner', [asgCompl(S), asgEvents('I', S.sel, S.compl)], ['assigner', [['item.assign', { item: 'I', targets: ['kim'], completion: 'assigner' }]]]);
S = pick([P('lee')], 'assignee');
check('below at depth 2, assignee picked earlier: still assigner', [offered(S), asgCompl(S), asgEvents('I', S.sel, S.compl)],
  [['assigner'], 'assigner', [['item.assign', { item: 'I', targets: ['lee'], completion: 'assigner' }]]]);
S = pick([P('kim'), P('pat')], 'assignee');
check('below and a peer, assignee picked: the peer keeps it', [offered(S), asgCompl(S), asgEvents('I', S.sel, S.compl)],
  [['assigner'], 'mixed', [['item.assign', { item: 'I', targets: ['kim'], completion: 'assigner' }], ['item.assign', { item: 'I', targets: ['pat'], completion: 'assignee' }]]]);
S = pick([{ t: 'r', root: 'me', levels: 1 }]);
check('a rule rooted at me: assigner only', [offered(S), asgEvents('I', S.sel, 'assignee')], [['assigner'], [['item.assign', { item: 'I', targets: [{ root: 'me', levels: 1 }], completion: 'assigner' }]]]);
S = pick([{ t: 'r', root: 'pat' }]);
check('a rule rooted at a peer: both offered', offered(S), ['assignee', 'assigner']);
S = pick([P('me')], 'assignee');
check('the viewer themself: assignee holds', [offered(S), asgEvents('I', S.sel, S.compl)], [['assignee', 'assigner'], [['item.assign', { item: 'I', targets: ['me'], completion: 'assignee' }]]]);
check('the quick-add row: a pick below goes as assigner', asgEvents('$1', [P('kim')], 'assignee'), [['item.assign', { item: '$1', targets: ['kim'], completion: 'assigner' }]]);
check('nothing picked: the pick, or none', [asgCompl(pick([])), asgCompl(pick([], 'assignee'))], [null, 'assignee']);
done();
`)
}
