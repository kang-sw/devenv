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

// TestTrackerInsertDepthMarkers pins the hover insert's depth markers: the
// indicator carries a tick strip drawn from its --ind alone (one dim tick
// per shallower level), visual only, beside the badge and the line.
func TestTrackerInsertDepthMarkers(t *testing.T) {
	if show := trackerFn(t, `function insShow\(list, g, x\)`, false); !strings.Contains(show, `'<div class="hins" aria-hidden="true"><span class="hdep"></span><span class="hl"></span>`) {
		t.Errorf("insShow draws no depth markers:\n%s", show)
	}
	css, err := dashboardFS.ReadFile("assets/tracker.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), ".tk .hins .hdep { position: absolute; top: 4px; height: 2px; left: 8px; width: calc(var(--ind, 0) * 16px); pointer-events: none;") {
		t.Error("tracker.css lacks the depth markers' strip (one 16px slot per shallower level, no pointer events)")
	}
}
