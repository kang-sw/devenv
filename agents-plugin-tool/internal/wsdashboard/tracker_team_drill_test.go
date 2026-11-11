package wsdashboard

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerTeamDrillMatchesTheCounts pins the team scope's drill-down
// (tracker.js teamDrillRecs): a count on a person's group header narrows the
// list to the records that count stands for, by the rules the view's team
// roll-up uses. A status key takes the person's live, non-fallback records of
// that status on a visible, non-topic item; load takes those neither idea nor
// done; stale and overdue are subsets of load; fallback takes the live
// fallback holdings not done; unaccepted and waiting are the row's own
// pending and waiting lists. It also pins that listTeam reads the drill and
// that the header's counts and the clear control dispatch through data-act.
// The cases execute under node when it is installed.
func TestTrackerTeamDrillMatchesTheCounts(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	list := regexp.MustCompile(`function listTeam\(\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	for _, want := range []string{"teamDrill()", "teamDrillRecs(p, on, VS.assignments, VS.items, today()).filter(keepRec)", `attr('act', 'tdrill')`, `wsT("js.tk.team.outside")`} {
		if !strings.Contains(list, want) {
			t.Errorf("listTeam lacks %q", want)
		}
	}
	if !strings.Contains(js, "else if (k === 'tdrill')") {
		t.Error("the click handler does not dispatch tdrill")
	}

	fn := regexp.MustCompile(`function teamDrillRecs\(p, key, recs, items, day\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if fn == "" {
		t.Fatal("tracker.js holds no teamDrillRecs")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; teamDrillRecs cases not executed")
	}
	type rec struct {
		ID       string `json:"id"`
		Item     string `json:"item"`
		To       string `json:"to"`
		State    string `json:"state"`
		Status   string `json:"status"`
		Fallback bool   `json:"fallback"`
		Stale    bool   `json:"stale"`
	}
	type item struct {
		Kind   string  `json:"kind"`
		Hidden bool    `json:"hidden"`
		Due    *string `json:"due"`
	}
	day := func(s string) *string { return &s }
	recs := map[string]rec{}
	for _, r := range []rec{
		{"todo>m", "t1", "m", "live", "todo", false, false},
		{"doing>m", "t2", "m", "live", "doing", false, true},
		{"blocked>m", "t3", "m", "live", "blocked", false, false},
		{"idea>m", "t4", "m", "live", "idea", false, false},
		{"done>m", "t5", "m", "live", "done", false, false},
		{"fb-todo>m", "t6", "m", "live", "todo", true, false},
		{"fb-done>m", "t7", "m", "live", "done", true, false},
		{"fb-idea>m", "t4b", "m", "live", "idea", true, false},
		{"topic>m", "topic", "m", "live", "todo", false, false},
		{"hidden>m", "gone", "m", "live", "todo", false, false},
		{"pending>m", "t8", "m", "pending", "todo", false, false},
		{"ended>m", "t9", "m", "unassigned", "todo", false, false},
		{"todo>n", "t1", "n", "live", "todo", false, false},
		{"over-done>m", "past", "m", "live", "done", false, false},
		{"over>m", "past2", "m", "live", "blocked", false, false},
		{"over-idea>m", "past3", "m", "live", "idea", false, false},
		{"over-fb>m", "past4", "m", "live", "todo", true, false},
	} {
		recs[r.ID] = r
	}
	items := map[string]item{
		"t1": {}, "t2": {Due: day("2026-11-11")}, "t3": {Due: day("2026-11-12")}, "t4": {}, "t4b": {}, "t5": {}, "t6": {}, "t7": {}, "t8": {}, "t9": {},
		"topic": {Kind: "topic"}, "gone": {Hidden: true},
		"past": {Due: day("2026-11-01")}, "past2": {Due: day("2026-11-10")}, "past3": {Due: day("2026-11-01")}, "past4": {Due: day("2026-11-01")},
	}
	person := map[string]any{"id": "m", "pending": []string{"pending>m"}, "waiting": []string{"sent>x"}}
	recs["sent>x"] = rec{"sent>x", "t1", "x", "pending", "todo", false, false}
	cases := []struct {
		Key  string   `json:"key"`
		Day  string   `json:"day"`
		Want []string `json:"want"`
	}{
		// a status key leaves out fallback, topic, hidden, pending, ended, and other people's records
		{"todo", "2026-11-11", []string{"todo>m"}},
		{"blocked", "2026-11-11", []string{"blocked>m", "over>m"}},
		{"idea", "2026-11-11", []string{"idea>m", "over-idea>m"}},
		{"done", "2026-11-11", []string{"done>m", "over-done>m"}},
		// load: neither idea nor done, no fallback holding
		{"load", "2026-11-11", []string{"todo>m", "doing>m", "blocked>m", "over>m"}},
		{"stale", "2026-11-11", []string{"doing>m"}},
		// overdue: due strictly before today, inside the load (a done, idea, or fallback record is not)
		{"overdue", "2026-11-11", []string{"over>m"}},
		{"overdue", "2026-11-13", []string{"doing>m", "blocked>m", "over>m"}},
		{"overdue", "", []string{}},
		// fallback: live fallback holdings not done, idea included
		{"fallback", "2026-11-11", []string{"fb-todo>m", "fb-idea>m", "over-fb>m"}},
		{"unaccepted", "2026-11-11", []string{"pending>m"}},
		{"waiting", "2026-11-11", []string{"sent>x"}},
	}
	in, _ := json.Marshal(map[string]any{"recs": recs, "items": items, "p": person, "cases": cases})
	script := fn + `
var bad = [], D = ` + string(in) + `;
D.cases.forEach(function (c) {
  var got = teamDrillRecs(D.p, c.key, D.recs, D.items, c.day).map(function (r) { return r.id; }).sort();
  var want = c.want.slice().sort();
  if (JSON.stringify(got) !== JSON.stringify(want)) bad.push(c.key + ' @' + c.day + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want));
});
process.stdout.write(bad.join('\n'));
`
	out, err := exec.Command(node, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("teamDrillRecs cases failed:\n%s", s)
	}
}
