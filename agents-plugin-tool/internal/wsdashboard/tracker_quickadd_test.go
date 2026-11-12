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
		trackerFn(t, `function nrowPri\(n\)`, true) + trackerFn(t, `function nrowLabels\(n\)`, true) + trackerFn(t, `function nrowKind\(kind\)`, false) +
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
	labels := trackerFn(t, `function newLabelsHtml\(n\)`, false)
	for _, want := range []string{"attr('act', 'm-ltog')", "admin = !!VS.viewer.admin", "attr('act', 'm-ledit')", "attr('act', 'm-lrm')", "mi('m-ladd'"} {
		if !strings.Contains(labels, want) {
			t.Errorf("newLabelsHtml lacks %q", want)
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
// muted "add a label"); the step lists the store's labels as toggle chips
// and a toggle writes the item's labels at once (in the store's list order)
// with the step kept open; an empty store says so and, for an admin only,
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
		"case 'm-label': MN.lab = a.getAttribute('data-label'); return commit([['item.edit', { item: id, labels: labelToggled(it.labels, MN.lab) }]]);",
		"case 'm-ladd': closeMenu(false); return openDialog('slabel', {});",
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
		trackerFn(t, `function labelsProp\(it\)`, false) + trackerFn(t, `function labelToggle\(l, on, act, aria\)`, false) +
		trackerFn(t, `function labelStepHtml\(it\)`, false) + trackerFn(t, `function labelToggled\(cur, lid\)`, false)
	runTrackerCases(t, "property card labels", trackerCheckJS+`
var VS = { viewer: { member: 'me', admin: false }, labels: [{ id: 'L1', name: 'docs', color: '#111111' }, { id: 'L2', name: 'dev', color: '#222222' }] }, editor = true;
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
check('the step lists every label as a toggle chip, the picked one on, focus on the first', [has(st, 'class="chip lbl" data-act="m-label" data-label="L1" role="menuitemcheckbox" aria-checked="false" data-mi data-autofocus'), has(st, 'class="chip lbl on" data-act="m-label" data-label="L2" role="menuitemcheckbox" aria-checked="true" data-mi')], [true, true]);
check('a toggle adds in list order', labelToggled(['L2'], 'L1'), ['L1', 'L2']);
check('a toggle removes', labelToggled(['L1', 'L2'], 'L1'), ['L2']);
check('a toggle drops labels the list no longer holds', labelToggled(['gone', 'L1'], 'L2'), ['L1', 'L2']);
VS.labels = [];
check('an empty store says so, no add for a non-admin', labelStepHtml(it), '<p class="mempty">js.tk.labels.empty</p>');
VS.viewer.admin = true;
check('an admin gets the label dialog', has(labelStepHtml(it), 'data-act="m-ladd"'), true);
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
	if i, j := strings.Index(click, "rowClick(row, t)"), strings.Index(click, "var it = t.closest('[data-item]');"); i < 0 || j < 0 || i > j {
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
		trackerFn(t, `function today\(\)`, true) + irow + trackerFn(t, `function rowClick\(row, t\)`, false) + trackerFn(t, `function labelStepRedraw\(\)`, false) +
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
		".tk .irow:is(:hover, :focus-within, .sel) .rlab, .tk .rlab[aria-expanded=\"true\"] { display: inline-flex; }",
	} {
		if !strings.Contains(string(css), want) {
			t.Errorf("tracker.css lacks %q", want)
		}
	}
	if strings.Contains(string(css), "border-radius: 6px; cursor: pointer; }\n.tk .ir-main .title") {
		t.Error("the whole row still shows the pointer though only its title opens the item window")
	}
}
