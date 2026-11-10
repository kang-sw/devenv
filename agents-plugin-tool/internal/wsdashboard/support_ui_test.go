package wsdashboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// TestUIEntryRendersTheCommandLine pins a ui line's markup: one muted
// line per command, carrying the op and arguments the support chat acts
// on as data attributes, the same whether the page meets it live or in
// history.
func TestUIEntryRendersTheCommandLine(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingReply, Text: "Opening it:"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingUI, UI: &wslive.UICommand{Op: wslive.UIFocus, Arg: "ada.c89-3", Comment: "ada.c89-7"}}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingUI, UI: &wslive.UICommand{Op: wslive.UISearch, Arg: `<b>"sync"</b>`}}),
		ringRecord(4, wslive.RingEntry{Kind: wslive.RingUI, UI: &wslive.UICommand{Op: wslive.UIScope, Arg: "team"}}),
		ringRecord(5, wslive.RingEntry{Kind: wslive.RingUI, UI: &wslive.UICommand{Op: wslive.UIView, Arg: "kanban"}}),
	}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, nil), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	for _, want := range []string{
		`<div class="sysline uiline" data-entry="` + ring[1].Name + `" data-ui-op="focus" data-ui-arg="ada.c89-3" data-ui-comment="ada.c89-7"`,
		`opened ada.c89-3 at comment ada.c89-7</div>`,
		`data-ui-op="search" data-ui-arg="&lt;b&gt;&#34;sync&#34;&lt;/b&gt;" title=`,
		`searched items for “&lt;b&gt;&#34;sync&#34;&lt;/b&gt;”</div>`,
		`data-ui-op="scope" data-ui-arg="team" title=`,
		`items scope: team</div>`,
		`data-ui-op="view" data-ui-arg="kanban" title=`,
		`items view: kanban</div>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("ui entries lack %q:\n%s", want, html)
		}
	}
	if strings.Count(html, `class="sysline uiline"`) != 4 {
		t.Errorf("want one line per ui entry:\n%s", html)
	}
}

// TestUIScopesMatchTheTrackerRenderer pins wslive's copy of the Items
// panel's keys: the scope keys of tracker.js's SCOPES, in order, and the
// view mode keys its SCOPE_MODES offers across the scopes.
func TestUIScopesMatchTheTrackerRenderer(t *testing.T) {
	raw, err := dashboardFS.ReadFile("assets/tracker.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	block := func(from, to string) string {
		i := strings.Index(src, from)
		if i < 0 {
			t.Fatalf("tracker.js lacks %q", from)
		}
		j := strings.Index(src[i:], to)
		if j < 0 {
			t.Fatalf("tracker.js lacks %q after %q", to, from)
		}
		return src[i : i+j]
	}
	keys := func(text, prefix string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`\['([a-z]+)', "`+regexp.QuoteMeta(prefix)).FindAllStringSubmatch(text, -1) {
			if !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
		return out
	}
	scopes := keys(block("var SCOPES = [", "];"), "js.tk.scope.")
	if !slices.Equal(scopes, wslive.UIScopes) {
		t.Errorf("tracker.js SCOPES keys %v, wslive.UIScopes %v", scopes, wslive.UIScopes)
	}
	modes := keys(block("var ASSIGN_MODES = [", "var CARD_MODES"), "js.tk.mode.")
	slices.Sort(modes)
	want := slices.Clone(wslive.UIViewModes)
	slices.Sort(want)
	if !slices.Equal(modes, want) {
		t.Errorf("tracker.js SCOPE_MODES keys %v, wslive.UIViewModes %v", modes, wslive.UIViewModes)
	}
}
