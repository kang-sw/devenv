package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestTrackerStoreSettingsLeaveTheRolesDialog pins the Team tab's store
// settings (tracker.js settingsHtml): the priority levels and the
// attachment limit are a section of the Team card with their own dialogs,
// and the roles-event dialog no longer offers them, names each type it does
// offer by a table key, and shows only the selected type's fields.
func TestTrackerStoreSettingsLeaveTheRolesDialog(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	types := regexp.MustCompile(`var ROLE_TYPES = \[[^\]]*\];`).FindString(js)
	if types == "" {
		t.Fatal("tracker.js holds no ROLE_TYPES")
	}
	for _, gone := range []string{"set_priority_levels", "set_attachment_limit"} {
		if strings.Contains(types, gone) {
			t.Errorf("the roles-event dialog still offers %s: %s", gone, types)
		}
	}

	admin := regexp.MustCompile(`      case 'admin': \{\n[\s\S]*?\n        break;\n      \}\n`).FindString(js)
	if admin == "" {
		t.Fatal("dialogHtml holds no admin case")
	}
	for _, want := range []string{"wsT(ROLE_NAME[t])", "rfld(mt, ", "rfld('roles.add_member roles.set_lead', ", "rfld('roles.add_member roles.add_key roles.revoke_key', "} {
		if !strings.Contains(admin, want) {
			t.Errorf("the admin dialog lacks %q", want)
		}
	}
	for _, gone := range []string{"pnames", "pdefault", "maxbytes"} {
		if strings.Contains(admin, gone) {
			t.Errorf("the admin dialog still carries the %s field", gone)
		}
	}
	if !strings.Contains(js, "    rtypeShow(f);\n") || !strings.Contains(js, "if (t.name === 'rtype') rtypeShow(df);") {
		t.Error("the type fields are not shown per type on open and on change")
	}

	team := regexp.MustCompile(`function cardTeam\(v\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(team, "h += settingsHtml(!!me.admin);") {
		t.Error("the Team card does not draw the store settings")
	}
	set := regexp.MustCompile(`function settingsHtml\(admin\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{"(admin ? btn(act, ", "'sprio')", "'satt')"} {
		if !strings.Contains(set, want) {
			t.Errorf("settingsHtml lacks %q", want)
		}
	}
	if !strings.Contains(js, "var DIALOGS = { profile: 1, admin: 1, sprio: 1, satt: 1") {
		t.Error("the settings dialogs are not dialog kinds")
	}
}

// TestTrackerLabelsInTheDashboard pins the label surfaces of tracker.js: the
// store settings list the labels (admin: add, edit, remove), rows and the
// item window draw an item's label chips, a chip click searches label:,
// and the item window's edit mode picks labels and writes them only when
// they changed.
func TestTrackerLabelsInTheDashboard(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	set := regexp.MustCompile(`function settingsHtml\(admin\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{"btn('slabel', wsT(\"js.tk.set.labels.add\")", "btn('slabel-rm', ", "labelChip(l)"} {
		if !strings.Contains(set, want) {
			t.Errorf("settingsHtml lacks %q", want)
		}
	}
	irow := regexp.MustCompile(`function irow\(o\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(irow, "labelChips(it, 'lsearch')") {
		t.Error("a row does not draw its label chips")
	}
	win := regexp.MustCompile(`function windowHtml\(v, it\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{"labelChips(it, 'lsearch')", "labelPickHtml(ed)"} {
		if !strings.Contains(win, want) {
			t.Errorf("the item window lacks %q", want)
		}
	}
	search := regexp.MustCompile(`function labelSearch\(id\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(search, `panelState().q = 'label:"' + l.name + '"';`) {
		t.Errorf("a label chip does not search label:\n%s", search)
	}
	save := regexp.MustCompile(`function saveEdit\(\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(save, "if (picked.slice().sort().join(' ') !== (it.labels || []).slice().sort().join(' ')) ep.labels = picked;") {
		t.Errorf("saveEdit does not write changed labels:\n%s", save)
	}
	for _, typ := range []string{"roles.add_label", "roles.edit_label", "roles.remove_label"} {
		if !strings.Contains(js, "case '"+typ+"':") {
			t.Errorf("the activity sentences lack %s", typ)
		}
	}
}

// TestTrackerPriorityDialogRows pins the priority-levels dialog (tracker.js
// prioRow, the sprio dialog's rows, and the input listener that appends a
// row), executed under node when it is installed: labels count from 1
// while the radio value and the field name stay 0-based; the dialog shows
// the levels plus two blank rows, up to 9; typing into the last row appends
// the next blank row, and the rows stop at 9.
func TestTrackerPriorityDialogRows(t *testing.T) {
	row := trackerBlock(t, `\n  var PRIO_MAX = 9;`, `\n  var DIALOGS = `)
	rows := trackerBlock(t, `\n        var pn = VS.priority.names, rows = '';`, `\n        T = \[wsT\("js.tk.dlg.prio.title"\)`)
	grow := trackerBlock(t, `\n    // typing into the last priority row`, `\n    document.addEventListener\('input', function \(e\) \{\n      var f = `)
	runTrackerCases(t, "priority dialog rows", trackerCheckJS+`
function wsT(k, n) { return k + ':' + n; }
function esc(s) { return String(s); }
function tipAttr(s) { return ' data-tip="' + s + '"'; }
var onInput = null, document = { addEventListener: function (t, fn) { if (t === 'input') onInput = fn; } };
`+row+`
function dialogRows(VS) {`+rows+`
  return rows;
}
`+grow+`
// form parses rows into a dialog form: its pname fields, each with the row
// it stands in, which takes the appended row's markup after it.
function form(html) {
  var f = { html: html, elements: {} };
  f.closest = function (sel) { return sel === '#tkdlg form[data-kind="sprio"]' ? f : null; };
  function field(name) {
    return { name: name, value: '', closest: f.closest, parentNode: { insertAdjacentHTML: function (where, h) { f.html += h; scan(h); } } };
  }
  function scan(h) { (h.match(/name="pname\d+"/g) || []).forEach(function (m) { var n = m.slice(6, -1); f.elements[n] = field(n); }); }
  scan(html);
  return f;
}
function type(f, name, text) { var e = f.elements[name]; e.value = text; onInput({ target: e }); }
function count(f) { return Object.keys(f.elements).length; }

var two = dialogRows({ priority: { names: ['low', 'high'], default: 1 } });
check('two levels show four rows, the level names first', [two.match(/name="pname\d+" value="[^"]*"/g)], [['name="pname0" value="low"', 'name="pname1" value="high"', 'name="pname2" value=""', 'name="pname3" value=""']]);
check('radio values stay 0-based', two.match(/name="pdef" value="\d+"/g), ['name="pdef" value="0"', 'name="pdef" value="1"', 'name="pdef" value="2"', 'name="pdef" value="3"']);
check('the default level is checked', two.match(/value="\d+" checked/g), ['value="1" checked']);
check('level labels count from 1', two.match(/placeholder="js\.tk\.form\.plevel:\d+"/g), ['placeholder="js.tk.form.plevel:1"', 'placeholder="js.tk.form.plevel:2"', 'placeholder="js.tk.form.plevel:3"', 'placeholder="js.tk.form.plevel:4"']);
check('default labels count from 1', two.match(/aria-label="js\.tk\.form\.pdefault:\d+"/g), ['aria-label="js.tk.form.pdefault:1"', 'aria-label="js.tk.form.pdefault:2"', 'aria-label="js.tk.form.pdefault:3"', 'aria-label="js.tk.form.pdefault:4"']);
check('eight levels show the 9 cap', count(form(dialogRows({ priority: { names: ['1', '2', '3', '4', '5', '6', '7', '8'], default: 0 } }))), 9);

var f = form(two);
type(f, 'pname2', 'mid');
check('typing into a row before the last appends nothing', count(f), 4);
type(f, 'pname3', '   ');
check('blank text in the last row appends nothing', count(f), 4);
type(f, 'pname3', 'top');
check('typing into the last row appends the next blank row', [count(f), /name="pname4" value=""/.test(f.html), /placeholder="js\.tk\.form\.plevel:5"/.test(f.html), /name="pdef" value="4"/.test(f.html)], [5, true, true, true]);
for (var i = 4; i < 12; i++) if (f.elements['pname' + i]) type(f, 'pname' + i, 'x' + i);
check('the rows stop at 9', [count(f), !!f.elements.pname8, !!f.elements.pname9], [9, true, false]);
done();
`)
}
