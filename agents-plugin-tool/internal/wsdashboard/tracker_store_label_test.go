package wsdashboard

import (
	"regexp"
	"strings"
	"testing"
)

// TestTrackerStoreLabelTitles pins how the tracker renderer names a store
// with a display label: the card-strip handle shows the label with the id
// in its tooltip, and the drawer title reads "Label (id)" with the id
// muted. StoreInfo.label repeats the id when no label is set, so a label
// equal to the name draws the id alone, as before labels existed. The
// title helpers execute under node when it is installed.
func TestTrackerStoreLabelTitles(t *testing.T) {
	data, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"b.querySelector('.hl').textContent = vs.store.label || name;",
		"b.title = storeTitle(vs.store) +",
		"'<div class=\"tk-title\">' + storeTitleHtml(vs.store) +",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("tracker.js lacks %q", want)
		}
	}
	esc := regexp.MustCompile(`(?s)\n  function esc\(s\) \{.*?\n  \}\n`).FindString(src)
	titles := regexp.MustCompile(`(?m)^  function storeTitle(Html)?\(st\) \{.*\}$`).FindAllString(src, -1)
	if esc == "" || len(titles) != 2 {
		t.Fatalf("tracker.js: esc found %v, %d storeTitle helpers (want 2)", esc != "", len(titles))
	}
	runNode(t, "store title", esc+strings.Join(titles, "\n")+`
var bad = [];
function check(what, got, want) { if (got !== want) bad.push(what + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want)); }
var lab = { name: "mylocaltest", label: "Team <Board>" }, none = { name: "mylocaltest", label: "mylocaltest" }, empty = { name: "mylocaltest", label: "" };
check("labelled text", storeTitle(lab), "Team <Board> (mylocaltest)");
check("labelled html", storeTitleHtml(lab), '<b>Team &lt;Board&gt;</b> <span class="muted">(mylocaltest)</span>');
check("unlabelled text", storeTitle(none), "mylocaltest");
check("unlabelled html", storeTitleHtml(none), "<b>mylocaltest</b>");
check("empty label text", storeTitle(empty), "mylocaltest");
check("empty label html", storeTitleHtml(empty), "<b>mylocaltest</b>");
process.stdout.write(bad.join("\n"));
`)
}
