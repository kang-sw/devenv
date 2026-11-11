package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// trackerBlock returns the tracker.js source from the regexp start up to,
// not including, the regexp end.
func trackerBlock(t *testing.T, start, end string) string {
	t.Helper()
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?s)(` + start + `.*?)` + end).FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("tracker.js holds no block from %q to %q", start, end)
	}
	return m[1]
}

// runTrackerCases runs script under node; it skips when node is not installed.
func runTrackerCases(t *testing.T, what, script string) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; " + what + " cases not executed")
	}
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("%s cases failed: %v\n%s", what, err, strings.TrimSpace(string(out)))
	}
}

const trackerCheckJS = `
var bad = [];
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }
function done() { if (bad.length) { console.log(bad.join('\n')); process.exit(1); } }
`

// TestTrackerRelatedScope pins the related scope's list (tracker.js
// listRelated, relSecHtml): the CC section comes before the Mentions
// section, each with its count and open by default; a folded section keeps
// its header and drops its rows; closed items go to the shared closed list;
// under a filter an empty section is left out and an empty scope shows the
// filter's empty state. It also pins the scope's place beside mine, its
// list-only mode, and the chip's count. The cases execute under node when it
// is installed.
func TestTrackerRelatedScope(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"['mine', \"js.tk.scope.mine\"",
		"\n    ['related', \"js.tk.scope.related\", \"js.tk.scope.related.tip\"],\n",
		"related: [['list', \"js.tk.mode.list\"]],",
		"related: listRelated",
		"return wsTn(\"js.tk.scope.chip.items\", listOf(rl.cc).length + listOf(rl.mentioned).length);",
		"else if (k === 'rel-fold')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("tracker.js lacks %q", want)
		}
	}
	if strings.Index(js, "['mine', \"js.tk.scope.mine\"") > strings.Index(js, "['related', \"js.tk.scope.related\"") {
		t.Error("the related scope does not follow mine")
	}

	block := trackerBlock(t, `\n  function listRelated\(\) \{`, `\n  function listGiven\(\)`)
	script := trackerCheckJS + `
var VS, FILT = false, HIDE = {}, closed = [], ps = {};
function listOf(x) { return Array.isArray(x) ? x : []; }
function keepItem(id) { return !HIDE[id]; }
function closeTs(it) { return it.ts; }
function addClosed(ts, n, html) { closed.push([ts, n, html]); }
function irow(o) { return 'row(' + o.rk + (o.closed ? ',closed' : '') + ')'; }
function filterEmpty() { return 'EMPTY'; }
function panelState() { return ps; }
function wsT(k) { return k; }
function attr(k, v) { return ' data-' + k + '="' + v + '"'; }
function treeRows(rows, pre) { return '[' + pre + rows.map(function (r) { return r.rk; }).join(',') + ']'; }
` + block + `
function secs(h) { var out = [], re = /data-rsec="([a-z]+)"[^>]*>[\s\S]*?<span class="subn">(\d+)<\/span><\/button>([\s\S]*?)<\/section>/g, m; while ((m = re.exec(h))) out.push([m[1], +m[2], m[3]]); return out; }
VS = { items: { 'x#3': { ts: '2026-11-01T00:00:00Z' } }, my: { related: { cc: ['x#1'], mentioned: ['x#2'], closed: ['x#3'] } } };
var h = listRelated();
check('sections in order, open, with rows', secs(h), [['cc', 1, '[rel:cc:rel:cc:x#1]'], ['mentioned', 1, '[rel:mentioned:rel:mentioned:x#2]']]);
check('both open', (h.match(/class="rsec open"/g) || []).length, 2);
check('closed to the closed list', closed, [['2026-11-01T00:00:00Z', 1, 'row(rel:x:x#3,closed)']]);
// an empty section shows its none line
VS.my.related = { cc: [], mentioned: ['x#2'] }; closed = [];
check('empty CC section', secs(listRelated())[0], ['cc', 0, '<p class="muted">js.tk.list.rel.cc.none</p>']);
// a folded section keeps its header and count, without rows
ps.relFold = { cc: true }; VS.my.related = { cc: ['x#1'], mentioned: ['x#2'] };
h = listRelated();
check('folded CC', secs(h)[0], ['cc', 1, '']);
check('folded CC not open', /class="rsec" data-rsec="cc"/.test(h), true);
ps.relFold = {};
// under a filter: an empty section drops out, an empty scope is the filter's empty state
FILT = true; HIDE = { 'x#2': true };
check('filtered sections', secs(listRelated()).map(function (s) { return s[0]; }), ['cc']);
HIDE = { 'x#1': true, 'x#2': true };
check('filtered to nothing', listRelated(), 'EMPTY');
done();
`
	runTrackerCases(t, "related scope", script)
}

// TestTrackerCCNotices pins the CC notices in the notification feed
// (tracker.js ccNoticeTxt, feedEntries): each my.ccNotices entry becomes a
// feed entry of kind cc keyed by the notice's key, with a sentence per event
// type (an own status write reads as such; a note or comment rides along),
// an unknown type is left out, and a notice on a hidden item is dropped.
// The cases execute under node when it is installed.
func TestTrackerCCNotices(t *testing.T) {
	block := trackerBlock(t, `\n  var CC_KEY = \{`, `\n  function readMap\(\)`)
	script := trackerCheckJS + `
var VS, EV = {};
function listOf(x) { return Array.isArray(x) ? x : []; }
function vals(o) { return Object.keys(o).map(function (k) { return o[k]; }); }
function today() { return '2026-11-06'; }
function A(id) { return id; }
function esc(s) { return String(s); }
function nm(id) { return id ? id.toUpperCase() : String(id); }
function stLabel(s) { return '<' + s + '>'; }
function snippet(s) { return '"' + s + '"'; }
function wsT(k) { return k + '(' + [].slice.call(arguments, 1).join('|') + ')'; }
` + block + `
EV = { 'r#5': { payload: { item: 'x#1', status: 'blocked', note: 'waiting' } }, 'r#6': { payload: { item: 'x#1' } }, 'r#7': { payload: { item: 'x#1', body: 'hi' } } };
VS = {
  items: { 'x#1': { id: 'x#1', mentions: [], author: 'raj', ts: '2026-11-01T00:00:00Z' }, 'x#9': { id: 'x#9', hidden: true, mentions: [], author: 'raj', ts: '2026-11-01T00:00:00Z' } },
  comments: { 'r#7': { id: 'r#7', item: 'x#1', author: 'tia', body: 'hi', mentions: [], ts: '2026-11-05T12:00:00Z' } },
  notices: [], requests: {},
  my: { assignments: [], received: { pending: [] }, today: { confirms: [] }, ccNotices: [
    { key: 'cc:r#5', type: 'item.status', item: 'x#1', event: 'r#5', by: 'pia', to: 'pia', status: 'blocked', ts: '2026-11-05T10:00:00Z' },
    { key: 'cc:r#4', type: 'item.status', item: 'x#1', event: 'r#4', by: 'raj', to: 'pia', status: 'doing', ts: '2026-11-05T09:00:00Z' },
    { key: 'cc:r#6', type: 'item.close', item: 'x#1', event: 'r#6', by: 'pia', to: 'pia', ts: '2026-11-05T11:00:00Z' },
    { key: 'cc:r#7', type: 'comment.add', item: 'x#1', event: 'r#7', by: 'tia', to: null, ts: '2026-11-05T12:00:00Z' },
    { key: 'cc:r#8', type: 'item.unknown', item: 'x#1', event: 'r#8', by: 'tia', to: 'tia', ts: '2026-11-05T13:00:00Z' },
    { key: 'cc:r#9', type: 'item.reopen', item: 'x#9', event: 'r#9', by: 'raj', to: 'pia', ts: '2026-11-05T14:00:00Z' }
  ] }
};
check('cc feed entries', feedEntries('maya').map(function (e) { return [e.key, e.kind, e.item, e.ev, e.html]; }), [
  ['cc:r#7', 'cc', 'x#1', 'r#7', 'js.tk.feed.cc.comment(TIA|"hi")'],
  ['cc:r#6', 'cc', 'x#1', 'r#6', 'js.tk.feed.cc.close(PIA|PIA)'],
  ['cc:r#5', 'cc', 'x#1', 'r#5', 'js.tk.feed.cc.status.own(PIA|<blocked>)js.tk.ev.tail("waiting")'],
  ['cc:r#4', 'cc', 'x#1', 'r#4', 'js.tk.feed.cc.status(RAJ|PIA|<doing>)']
]);
done();
`
	runTrackerCases(t, "CC notice", script)
}

// TestTrackerItemWindowCC pins the item window's CC list (tracker.js ccHtml):
// one row per CC'd member with a remove control, a "+" that opens the item
// menu's CC step, no controls on a hidden item (and no section when it has
// no CC), and the writes the controls send: item.cc {item, member} from the
// CC step's pick and item.uncc {item, member} from a row's remove. The
// cases execute under node when it is installed.
func TestTrackerItemWindowCC(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"case 'uncc': put([['item.uncc', { item: item, member: who }]]); break;",
		"case 'm-cc': return commit([['item.cc', { item: id, member: a.getAttribute('data-to') }]]);",
		"case 'item.cc': return wsT(\"js.tk.ev.cc\", esc(p.member), ref(p.item));",
		"case 'item.uncc': return wsT(\"js.tk.ev.uncc\", esc(p.member), ref(p.item));",
		"if (step === 'cc') {",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("tracker.js lacks %q", want)
		}
	}
	win := regexp.MustCompile(`function windowHtml\(v, it\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(win, "assigneesHtml(v, it, recs) + ccHtml(it) + takeHtml(v, it)") {
		t.Error("windowHtml does not draw the CC list after the assignees")
	}

	block := regexp.MustCompile(`function ccHtml\(it\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if block == "" {
		t.Fatal("tracker.js holds no ccHtml")
	}
	script := trackerCheckJS + `
function listOf(x) { return Array.isArray(x) ? x : []; }
function attr(k, v) { return ' data-' + k + '="' + v + '"'; }
function esc(s) { return String(s); }
function tipAttr() { return ''; }
function wsT(k) { return k; }
function nm(id) { return id; }
function avHtml(id) { return ''; }
function whoName(id) { return id; }
` + block + `
function rows(h) { var out = [], re = /data-ccmember="([^"]+)"/g, m; while ((m = re.exec(h))) out.push(m[1]); return out; }
function acts(h) { var out = [], re = /data-act="([^"]+)"/g, m; while ((m = re.exec(h))) out.push(m[1]); return out; }
var h = ccHtml({ id: 'x#1', cc: ['maya', 'lena'] });
check('rows', rows(h), ['maya', 'lena']);
check('controls', acts(h), ['cc', 'uncc', 'uncc']);
check('remove targets', (h.match(/data-who="[^"]+"/g) || []), ['data-who="maya"', 'data-who="lena"']);
check('empty list', /js\.tk\.cc\.none/.test(ccHtml({ id: 'x#1', cc: [] })), true);
check('empty list keeps the add', acts(ccHtml({ id: 'x#1' })), ['cc']);
h = ccHtml({ id: 'x#1', cc: ['maya'], hidden: true });
check('hidden: rows without controls', [rows(h), acts(h)], [['maya'], []]);
check('hidden without CC: no section', ccHtml({ id: 'x#1', hidden: true }), '');
done();
`
	runTrackerCases(t, "item window CC", script)
}
