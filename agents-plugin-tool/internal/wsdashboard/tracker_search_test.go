package wsdashboard

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerSearchRequests pins the Items search box's requests
// (tracker.js searchFor): typing sends one request after the debounce, an
// answer to an older request is dropped, a changed view re-sends the same
// query at once (a write can change what matches), a malformed query keeps
// its message with no ids, and an empty box forgets the state. The cases
// execute under node when it is installed.
func TestTrackerSearchRequests(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n  var SQ = '', SIDS = null.*?\n  // a row with an assignment record passes on the record`).FindString(string(data))
	if block == "" {
		t.Fatal("tracker.js holds no SQ ... keepRec block")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; search request cases not executed")
	}
	script := `
var ui = { store: 'A', psr: {} }, VS = { v: 1 }, sent = [], timers = [], renders = 0, bad = [];
function render() { renders++; }
function setTimeout(fn, ms) { timers.push({ fn: fn, ms: ms }); return timers.length; }
function clearTimeout(id) { if (id) timers[id - 1] = null; }
function fire() { var due = timers.filter(Boolean); timers = []; due.forEach(function (t) { t.fn(); }); return due.map(function (t) { return t.ms; }); }
var AD = { search: function (n, q, done) { sent.push({ n: n, q: q, done: done }); } };
` + block + `
function check(what, got, want) { if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(what + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want)); }

// typing: each keystroke's render replaces the pending request; one goes out after the debounce
searchFor('A', 's'); searchFor('A', 'st'); searchFor('A', 'sta');
check('debounce delays', fire(), [SEARCH_DEBOUNCE]);
check('requests after typing', sent.map(function (x) { return x.q; }), ['sta']);
// a render with the same query and view sends nothing
searchFor('A', 'sta');
check('timers for an unchanged render', timers.filter(Boolean).length, 0);
// a newer query's answer wins; the older answer arriving late is dropped
searchFor('A', 'status:doing'); fire();
sent[1].done({ ok: true, ids: ['x#2'] });
sent[0].done({ ok: true, ids: ['x#1'] });
check('ids after the late older answer', ui.psr.A.ids, { 'x#2': true });
check('answered query', ui.psr.A.q, 'status:doing');
check('renders on the answer', renders, 1);
// a changed view re-sends the same query at once
VS = { v: 2 };
searchFor('A', 'status:doing');
check('re-send delay on a view change', fire(), [0]);
check('re-sent query', sent[2].q, 'status:doing');
sent[2].done({ ok: true, ids: [] });
check('ids after the re-send', ui.psr.A.ids, {});
// a malformed query: the message, no ids
searchFor('A', 'due:<x'); fire();
sent[3].done({ ok: false, error: 'search "due:<x": due takes a date' });
check('malformed state', [ui.psr.A.ids, ui.psr.A.err], [null, 'search "due:<x": due takes a date']);
// matchItem filters by the ids of the render in progress
SQ = 'q'; SIDS = { 'x#2': true };
check('matchItem by ids', [matchItem({ id: 'x#2' }), matchItem({ id: 'x#1', title: 'q' })], [true, false]);
SQ = 'q'; SIDS = null;
check('matchItem substring fallback', matchItem({ id: 'x#1', title: 'Q here' }), true);
// another store's answer does not render the store shown
searchFor('B', 'b'); fire(); ui.store = 'A'; renders = 0;
sent[4].done({ ok: true, ids: ['b#1'] });
check('renders for another store', renders, 0);
// an empty box forgets the state; an answer still in flight is dropped
searchFor('A', 'late'); fire();
searchFor('A', '');
check('state after clearing', ui.psr.A, null);
sent[5].done({ ok: true, ids: ['y'] });
check('state after the dropped answer', ui.psr.A, null);
if (bad.length) { console.log(bad.join('\n')); process.exit(1); }
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("search request cases failed: %v\n%s", err, strings.TrimSpace(string(out)))
	}
}
