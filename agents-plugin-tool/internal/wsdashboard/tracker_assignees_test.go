package wsdashboard

import (
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// TestTrackerAssigneeListHidesStackedEndedRecords pins the item window's
// assignee filter (tracker.js visibleRecs): live and pending records always
// show, and a member who holds neither keeps only the latest ended record, so
// unassigning and reassigning never stacks rows. It also pins that the list,
// its empty check, its fold count, and its header count read the filtered
// records, and that an ended record's pill is its end-state text. The filter
// cases execute under node when it is installed.
func TestTrackerAssigneeListHidesStackedEndedRecords(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)

	list := regexp.MustCompile(`function assigneesHtml\(v, it, recs\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.HasPrefix(list, "function assigneesHtml(v, it, recs) {\n    recs = visibleRecs(recs);\n    if (!recs.length)") {
		t.Errorf("assigneesHtml does not filter before its empty check:\n%.300s", list)
	}
	for _, want := range []string{"hid = rows.length - A_SHOWN", `<span class="cnt">' + rows.length + '</span>`} {
		if !strings.Contains(list, want) {
			t.Errorf("assigneesHtml lacks %q", want)
		}
	}
	chip := regexp.MustCompile(`function recChip\(r\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if !strings.Contains(chip, `if (r.ended) return '<span class="chip ended">' + esc(endText(r.state)`) {
		t.Errorf("recChip does not render an ended record's end-state text:\n%s", chip)
	}

	fn := regexp.MustCompile(`function visibleRecs\(recs\) \{\n[\s\S]*?\n  \}\n`).FindString(js)
	if fn == "" {
		t.Fatal("tracker.js holds no visibleRecs")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; visibleRecs cases not executed")
	}
	type rec struct {
		ID    string          `json:"id"`
		To    string          `json:"to"`
		State string          `json:"state"`
		Ended json.RawMessage `json:"ended,omitempty"`
	}
	end := func(ts string) json.RawMessage {
		return json.RawMessage(`{"id":"e","type":"item.unassign","ts":"` + ts + `"}`)
	}
	cases := []struct {
		Name string   `json:"name"`
		Recs []rec    `json:"recs"`
		Want []string `json:"want"`
	}{
		{"live hides the member's ended records", []rec{
			{"a1>m", "m", "unassigned", end("2026-11-09T01:00:00Z")},
			{"a2>m", "m", "unassigned", end("2026-11-09T02:00:00Z")},
			{"a3>m", "m", "live", nil},
		}, []string{"a3>m"}},
		{"only the latest ended record stays", []rec{
			{"a1>m", "m", "unassigned", end("2026-11-09T09:00:00Z")},
			{"a2>m", "m", "declined", end("2026-11-09T10:00:00Z")},
		}, []string{"a2>m"}},
		{"latest by parsed time, not string order", []rec{
			{"a1>m", "m", "unassigned", end("2026-11-09T10:00:00+09:00")},
			{"a2>m", "m", "unassigned", end("2026-11-09T09:00:00Z")},
		}, []string{"a2>m"}},
		{"a tie goes to the later record", []rec{
			{"a1>m", "m", "unassigned", end("2026-11-09T09:00:00Z")},
			{"a2>m", "m", "returned", end("2026-11-09T09:00:00Z")},
		}, []string{"a2>m"}},
		{"one ended record per member", []rec{
			{"a1>m", "m", "unassigned", end("2026-11-09T01:00:00Z")},
			{"a2>n", "n", "unassigned", end("2026-11-09T01:00:00Z")},
			{"a3>m", "m", "unassigned", end("2026-11-09T03:00:00Z")},
			{"a4>n", "n", "unassigned", end("2026-11-09T02:00:00Z")},
		}, []string{"a3>m", "a4>n"}},
		{"pending hides the member's ended record", []rec{
			{"a1>m", "m", "withdrawn", end("2026-11-09T01:00:00Z")},
			{"a2>m", "m", "pending", nil},
		}, []string{"a2>m"}},
		{"live and pending all stay, in order", []rec{
			{"a1>m", "m", "live", nil},
			{"a2>n", "n", "pending", nil},
			{"a3>o", "o", "live", nil},
		}, []string{"a1>m", "a2>n", "a3>o"}},
	}
	in, _ := json.Marshal(cases)
	script := fn + `
var bad = [];
` + string(in) + `.forEach(function (c) {
  var got = visibleRecs(c.recs).map(function (r) { return r.id; });
  if (JSON.stringify(got) !== JSON.stringify(c.want)) bad.push(c.name + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(c.want));
});
process.stdout.write(bad.join("\n"));
`
	out, err := nodeScript(node, script).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		t.Errorf("visibleRecs cases failed:\n%s", s)
	}
}
