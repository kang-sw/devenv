package wswhiteboard

import (
	"encoding/json"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeTickets resolves stems from a map; a stem it lacks is not found.
type fakeTickets map[string]string

func (f fakeTickets) TicketStatus(stem string) (string, bool) {
	s, ok := f[stem]
	return s, ok
}

func topicTickets(st *State, topic string) []string {
	if i := st.topicDocIndex(topic); i >= 0 {
		return st.Topics[i].Tickets
	}
	return nil
}

// TestTopicTicketsRoundTrip pins the wb:topic tickets attribute: a
// space-separated list of stems that replaces the topic's list whole (a
// repeat kept once), printed on the query's topic line, cleared by
// tickets="", and refused when a token is no ticket stem.
func TestTopicTicketsRoundTrip(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="ship"><wb:body>plan</wb:body></wb:topic>`)
	mustApply(t, st, `<wb:topic name="ship" tickets="261112-feat-a 261001-bug-b 261112-feat-a"/>`)
	if got := topicTickets(st, "ship"); !slices.Equal(got, []string{"261112-feat-a", "261001-bug-b"}) {
		t.Fatalf("tickets = %v", got)
	}
	if line := queryText(t, st, `{"depth":"topics"}`); !strings.Contains(line, "tickets: 261112-feat-a 261001-bug-b") {
		t.Fatalf("query topic line lacks the tickets:\n%s", line)
	}
	mustApply(t, st, `<wb:topic name="ship" tickets="261113-chore-c"/>`)
	if got := topicTickets(st, "ship"); !slices.Equal(got, []string{"261113-chore-c"}) {
		t.Fatalf("tickets did not replace the list whole: %v", got)
	}
	mustApply(t, st, `<wb:topic name="ship" tickets=""/>`)
	if got := topicTickets(st, "ship"); got != nil {
		t.Fatalf("tickets=\"\" left %v", got)
	}
	if line := queryText(t, st, `{"depth":"topics"}`); strings.Contains(line, "tickets:") {
		t.Fatalf("a cleared list still prints:\n%s", line)
	}
	wantProblems(t, refused(t, st, `<wb:topic name="ship" tickets="261112-feat-a not-a-stem"/>`), `"not-a-stem" is not a ticket stem`)
	wantProblems(t, refused(t, st, `<wb:topic name="nowhere" tickets=""/>`), `names no topic to clear tickets on`)

	// A labelled group links tickets as it adopts or renames.
	mustApply(t, st, `<wb:topic name="260101-feat-stem" label="Stemmed" tickets="260101-feat-stem"/>`)
	if got := topicTickets(st, "260101-feat-stem"); !slices.Equal(got, []string{"260101-feat-stem"}) || st.TopicLabel("260101-feat-stem") != "Stemmed" {
		t.Fatalf("labelled group: tickets %v, label %q", got, st.TopicLabel("260101-feat-stem"))
	}
}

// TestTopicWithOnlyTicketsPersists pins that a topic whose only content is
// its linked tickets is kept, including its default state history after clearing.
func TestTopicWithOnlyTicketsPersists(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="links" tickets="261112-feat-a"/>`)
	if !st.topicKnown("links") || len(st.Topics) != 1 {
		t.Fatalf("a topic with only tickets did not persist: %+v", st.Topics)
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: fakeTickets{"261112-feat-a": TicketReady}})
	if len(view.Topics) != 1 || len(view.Topics[0].Tickets) != 1 {
		t.Fatalf("view lacks the tickets-only topic: %+v", view.Topics)
	}
	if line := queryText(t, st, `{"depth":"topics"}`); !strings.Contains(line, "tickets: 261112-feat-a") {
		t.Fatalf("query lacks the tickets-only topic's links:\n%s", line)
	}
	for name, layout := range layouts() {
		page := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Tickets: fakeTickets{"261112-feat-a": TicketReady}}))
		if !strings.Contains(page, `data-topic="links"`) || strings.Count(page, `class="dw wbtk"`) != 1 || !strings.Contains(page, `data-ticket="261112-feat-a" data-status="ready"`) {
			t.Errorf("%s: tickets-only topic missing or its ticket icon absent", name)
		}
	}
	want := []Entry{{ID: "e1", Kind: EntrySystem, Text: "Topic state: discussing", At: testNow.Format(time.RFC3339), TopicState: json.RawMessage(`{"v":1,"op":"set","value":"discussing"}`)}}
	mustApply(t, st, `<wb:topic name="links" tickets=""/>`)
	var visible []Entry
	for _, e := range entries(t, st, "links") {
		if internalRevisionMarker(e) {
			continue
		}
		if boardRevisionMarker(e) {
			e.BoardNotice = nil
		}
		visible = append(visible, e)
	}
	if !st.topicKnown("links") || topicTickets(st, "links") != nil || !reflect.DeepEqual(visible, want) {
		t.Fatalf("clearing tickets lost or changed default history: %+v", st)
	}
	if view := st.TopicStates()["links"]; view.State == nil || view.State.Value != StateDiscussing || view.State.Entry != "e1" || view.Diagnostic != nil {
		t.Fatalf("clearing tickets lost default authority: %+v", view)
	}

	// Literal pre-default metadata has no history to retain once tickets clear.
	legacy := &State{Topics: []TopicDoc{{Name: "links", Tickets: []string{"261112-feat-a"}}}}
	mustApply(t, legacy, `<wb:topic name="links" tickets=""/>`)
	if legacy.topicKnown("links") || len(legacy.Topics) != 0 || legacy.EntrySeq != 0 || len(legacy.TopicStates()) != 0 {
		t.Fatalf("legacy empty topic persisted or gained history: %+v", legacy)
	}
}

// TestTopicTicketsAreQuietMetadata pins Decision 18: setting or clearing a
// topic's tickets moves no recency, reopens no closed topic, and adds no
// visible history entry.
func TestTopicTicketsAreQuietMetadata(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="old"><wb:log>first</wb:log></wb:topic>`)
	mustApply(t, st, `<wb:topic name="new"><wb:log>second</wb:log></wb:topic>`)
	mustApply(t, st, `<wb:topic name="done"><wb:log>third</wb:log><wb:close result="ok"/></wb:topic>`)
	order := func() []string {
		var names []string
		for _, topic := range BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}).Topics {
			names = append(names, topic.Name)
		}
		return names
	}
	before, beforeDocs := order(), slices.Clone(st.Topics)
	visibleCount := func(entries []Entry) int {
		n := 0
		for _, e := range entries {
			if !internalRevisionMarker(e) {
				n++
			}
		}
		return n
	}
	for _, batch := range []string{
		`<wb:topic name="old" tickets="261112-feat-a"/>`,
		`<wb:topic name="done" tickets="261112-feat-a"/>`,
		`<wb:topic name="old" tickets=""/>`,
	} {
		mustApply(t, st, batch)
		if got := order(); !slices.Equal(got, before) {
			t.Fatalf("%s moved recency: %v, was %v", batch, got, before)
		}
		for i, doc := range st.Topics {
			if doc.Changed != beforeDocs[i].Changed || doc.ChangedAt != beforeDocs[i].ChangedAt || visibleCount(doc.Entries) != visibleCount(beforeDocs[i].Entries) {
				t.Fatalf("%s touched %q: %+v", batch, doc.Name, doc)
			}
		}
		if !st.topicClosed("done") {
			t.Fatalf("%s reopened the closed topic", batch)
		}
	}
}

// TestTopicTicketIcon pins the linked-ticket icon: one ticket icon at the
// end of the bar's lead (right after the waiting clock when there is one), a
// count only from two tickets, and a popover listing each stem with its
// status grouped open/done/dropped/unknown. A topic without tickets renders
// no icon, and ticket-derived Live metadata never becomes a topic glow.
func TestTopicTicketIcon(t *testing.T) {
	resolve := fakeTickets{
		"261112-feat-live":     TicketTodo,
		"261112-bug-shipped":   TicketDone,
		"261112-refactor-gone": TicketDropped,
		"261112-research-open": TicketReady,
		"261112-chore-idea":    TicketIdea,
	}
	cases := []struct {
		tickets string
		live    bool
	}{
		{"261112-feat-live 261112-bug-shipped", true},
		{"261112-chore-idea", true},
		{"261112-bug-shipped 261112-refactor-gone", false},
		{"261112-research-open", false},
		{"261112-feat-missing", false},
	}
	for _, c := range cases {
		st := &State{}
		mustApply(t, st, `<wb:topic name="t" tickets="`+c.tickets+`"/>`)
		view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: resolve})
		if got := view.Topics[0].Live; got != c.live {
			t.Errorf("%s: live = %v, want %v", c.tickets, got, c.live)
		}
		if got := topicTickets(st, "t"); !slices.Equal(got, strings.Fields(c.tickets)) {
			t.Errorf("%s: stored tickets = %v", c.tickets, got)
		}
	}

	st := &State{}
	mustApply(t, st, `<wb:topic name="t" tickets="261112-feat-live 261112-bug-shipped 261112-refactor-gone 261112-feat-missing"/>`)
	badges := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: resolve}).Topics[0].Tickets
	want := []TicketBadge{
		{Stem: "261112-feat-live", Name: "feat-live", Status: TicketTodo},
		{Stem: "261112-bug-shipped", Name: "bug-shipped", Status: TicketDone},
		{Stem: "261112-refactor-gone", Name: "refactor-gone", Status: TicketDropped},
		{Stem: "261112-feat-missing", Name: "feat-missing", Status: TicketUnknown},
	}
	if !slices.Equal(badges, want) {
		t.Fatalf("badges = %+v", badges)
	}
	unknown := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}).Topics[0]
	if len(unknown.Tickets) != len(want) || unknown.Live {
		t.Fatalf("no-resolver metadata = %+v", unknown)
	}
	for i, badge := range unknown.Tickets {
		if badge.Stem != want[i].Stem || badge.Name != want[i].Name || badge.Status != TicketUnknown {
			t.Errorf("no-resolver badge %d = %+v", i, badge)
		}
	}

	glow := regexp.MustCompile(`<section class="topic[^"]*\blive\b`)
	// The icon block closes, then the lead closes, then the bar's own
	// elements follow (state badge, time, chips, or the note button).
	tail := regexp.MustCompile(`<span class="dw wbtk">.*?</span></span></span></span>(?:<span class="topic-state dw">|<time class="tago"|<span class="chip|<button)`)
	type row struct{ stem, status, group, text string }
	check := func(t *testing.T, page string, rows []row, body bool) {
		t.Helper()
		if glow.MatchString(page) {
			t.Error("rendered a ticket-derived topic glow")
		}
		if n := strings.Count(page, `class="dw wbtk"`); n != 1 {
			t.Fatalf("ticket icon count = %d, want 1", n)
		}
		lead, icon, tago := strings.Index(page, `<span class="tlead">`), strings.Index(page, `<span class="dw wbtk">`), strings.Index(page, `<time class="tago"`)
		if loc := tail.FindStringIndex(page); lead < 0 || icon < lead || tago < icon || loc == nil || loc[0] != icon || strings.Contains(page[lead:icon], "tnote-toggle") {
			t.Error("the ticket icon is not the last element of the bar's lead, before its time")
		}
		wantAll(t, page, `href="#i-ticket"`, `aria-label="Linked tickets: `+strconv.Itoa(len(rows))+`"`, `<span class="lb">Linked tickets: `+strconv.Itoa(len(rows))+`</span>`)
		if badge := `<span class="bd">` + strconv.Itoa(len(rows)) + `</span>`; (len(rows) > 1) != strings.Contains(page, badge) {
			t.Errorf("count badge for %d tickets: present = %v", len(rows), !(len(rows) > 1))
		}
		if len(rows) == 1 {
			wantNone(t, page, `<span class="bd">`)
		}
		for _, r := range rows {
			wantAll(t, page, `<span class="row `+r.group+`" data-ticket="`+r.stem+`" data-status="`+r.status+`">`, `title="`+r.stem+`">`+r.stem+`</span><span class="st">`+r.text+`</span>`)
		}
		fragments := []string{`data-topic="t"`, "Current content", "Review 261112-feat-live item"}
		if body {
			fragments = append(fragments, "Authored 261112-feat-live body", "Authored 261112-feat-live content")
		}
		wantAll(t, page, fragments...)
	}
	for _, c := range []struct {
		name    string
		tickets string
		resolve fakeTickets
		rows    []row
	}{
		{"resolved", "261112-feat-live 261112-bug-shipped 261112-refactor-gone 261112-chore-idea", resolve, []row{
			{"261112-feat-live", "todo", "open", "Open · todo"},
			{"261112-bug-shipped", "done", "done", "Done"},
			{"261112-refactor-gone", "dropped", "dropped", "Dropped"},
			{"261112-chore-idea", "idea", "open", "Open · idea"},
		}},
		{"missing", "261112-feat-missing", resolve, []row{{"261112-feat-missing", "unknown", "unknown", "Unknown"}}},
		{"no-resolver", "261112-feat-live", nil, []row{{"261112-feat-live", "unknown", "unknown", "Unknown"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := &State{}
			mustApply(t, st, `<wb:topic name="t" tickets="`+c.tickets+`"><wb:log>Authored 261112-feat-live content</wb:log></wb:topic>`+topicXML("t", "Authored 261112-feat-live body")+addXML(ClassTechnical, "t", "Review 261112-feat-live item"))
			for _, closed := range []bool{false, true} {
				if closed {
					mustApply(t, st, `<wb:topic name="t"><wb:close result="finished"/></wb:topic>`)
				}
				state := "/open"
				if closed {
					state = "/closed"
				}
				for name, layout := range layouts() {
					t.Run(name+state, func(t *testing.T) {
						check(t, render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Tickets: c.resolve})), c.rows, layout == LayoutFull)
					})
				}
				view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: c.resolve})
				regions, err := RenderRegions(view)
				if err != nil {
					t.Fatal(err)
				}
				check(t, string(regions.Board), c.rows, true)
				if closed {
					page, err := RenderClosedTopics(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: c.resolve, Closed: &ClosedWindow{From: 0, N: 30}}))
					if err != nil {
						t.Fatal(err)
					}
					check(t, string(page), c.rows, true)
				}
			}
			// Clearing the links removes the icon.
			mustApply(t, st, `<wb:topic name="t" tickets=""/>`)
			for name, layout := range layouts() {
				if page := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Tickets: c.resolve})); strings.Contains(page, `class="dw wbtk"`) || strings.Contains(page, `href="#i-ticket"`) || strings.Contains(page, `data-ticket=`) {
					t.Errorf("%s: a topic without tickets renders the ticket icon", name)
				}
			}
		})
	}

	// Beside the waiting clock: the ticket icon follows the dependency button.
	topic := TopicView{Name: "work", Label: "Work", Anchor: "topic-work", UpdatedAt: "2026-11-15T00:00:00Z", UpdatedClock: "00:00", State: &TopicState{Value: StateWaiting}, Dependencies: []DependencyView{{Token: "owner", Label: "owner"}}, Tickets: want[:1]}
	for _, layout := range layouts() {
		for _, out := range stateViewOutputs(t, View{Layout: layout, ReplyID: "r", Topics: []TopicView{topic}}) {
			bar := out[strings.Index(out, `<span class="tlead">`):]
			clock, icon := strings.Index(bar, `href="#i-clock"`), strings.Index(bar, `<span class="dw wbtk">`)
			state, tago := strings.Index(bar, `<span class="topic-state dw">`), strings.Index(bar, `<time class="tago"`)
			if clock < 0 || icon < clock || strings.Count(bar[:icon], `<span class="dw">`) != 1 || state < icon || tago < state {
				t.Errorf("ticket icon not right after the waiting clock: %.600s", bar)
			}
		}
	}
}

// TestTopicHeadHoldsTheSummaryLine pins the sticky head: an open topic's
// .tophead holds its bar, note, and a summary line with the latest log or
// panel recap and the check pill (done/total, as the closed fold's header reads), the check rows staying in flow
// after the head; a topic with no checks has no pill; a closed topic's head
// has no summary line; and a legacy recap stays a card under the head.
func TestTopicHeadHoldsTheSummaryLine(t *testing.T) {
	st := checkBoard(t)
	mustApply(t, st, `<wb:topic name="t"><wb:log>agent line</wb:log></wb:topic>`)
	page := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	section := page[strings.Index(page, `data-topic="t"`):]
	head := section[:strings.Index(section, `<div class="tbody wb-asks">`)]
	// The full page carries no check-answer chip: open checks stack at the
	// card bottom, answered ones stay in the history.
	wantNone(t, section, `class="ckpill"`, `answered 3 / 4`)
	for _, frag := range []string{`<div class="tophead"><div class="tbar">`, `<textarea class="note tnote"`, `<div class="tsub"><div class="trecap" data-entry=`, `agent line`} {
		if !strings.Contains(head, frag) {
			t.Errorf("head lacks %q:\n%s", frag, head)
		}
	}
	if strings.Contains(section, `class="chead"`) {
		t.Error("an open topic keeps the in-flow check header")
	}
	embed := render(t, BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"}))
	if !strings.Contains(embed, `<div class="ckpill"><span class="ckbtn">`) {
		t.Error("the embed's pill is not a plain span")
	}

	plain := &State{}
	mustApply(t, plain, `<wb:topic name="p"><wb:log>only a line</wb:log></wb:topic>`)
	if page := render(t, BuildView(plain, ViewInput{Layout: LayoutFull, ReplyID: "r"})); strings.Contains(page, `class="ckpill"`) || !strings.Contains(page, `<div class="tsub"><div class="trecap"`) {
		t.Error("a topic with no checks: pill shown or agent line missing from the head")
	}

	panel := &State{}
	mustApply(t, panel, `<wb:topic name="panel-only"><wb:panel recap="only a panel">body</wb:panel></wb:topic>`)
	for _, layout := range layouts() {
		page := render(t, BuildView(panel, ViewInput{Layout: layout, ReplyID: "r"}))
		wantAll(t, page, `<div class="tsub"><div class="trecap" data-entry="e1" data-kind="panel"`,
			`class="evi wb-evi-panel"`, `class="rtext md" title="only a panel">only a panel</div>`)
		wantNone(t, page, `class="ckpill"`)
	}

	closed := &State{}
	mustApply(t, closed, `<wb:topic name="c"><wb:log>end</wb:log><wb:close result="ok"/></wb:topic>`)
	if page := render(t, BuildView(closed, ViewInput{Layout: LayoutFull, ReplyID: "r"})); strings.Contains(page, `class="tsub"`) {
		t.Error("a closed topic's head carries the summary line")
	}

	legacy := &State{Topics: []TopicDoc{{Name: "l", Recap: "old recap", Changed: 1}}}
	page = render(t, BuildView(legacy, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	if !strings.Contains(page, `</textarea></div><div class="trecap">`) {
		t.Error("a legacy recap is not a card under the head")
	}
}

// TestTopicHeadClassIsTheBoardsOwn pins the head's class: .tophead, never
// .thead, which the dashboard's conversation panel already uses for its own
// header (Decision 12).
func TestTopicHeadClassIsTheBoardsOwn(t *testing.T) {
	thead := regexp.MustCompile(`\bthead\b`)
	for _, name := range []string{"assets/page.css", "assets/page.tmpl", "assets/rail.js", "assets/form.js"} {
		raw, err := whiteboardTestAsset(name)
		if err != nil {
			t.Fatal(err)
		}
		if loc := thead.FindIndex(raw); loc != nil {
			t.Errorf("%s uses the class thead: %q", name, raw[max(0, loc[0]-40):min(len(raw), loc[1]+40)])
		}
	}
	for _, name := range []string{"assets/page.css", "assets/page.tmpl", "assets/rail.js"} {
		raw, _ := whiteboardTestAsset(name)
		if !strings.Contains(string(raw), "tophead") {
			t.Errorf("%s does not name the head tophead", name)
		}
	}
}

// TestTopicTicketsSurviveTheFolderStore pins that linked tickets persist in
// the topic's authoritative meta.json through the folder transaction.
func TestTopicTicketsSurviveTheFolderStore(t *testing.T) {
	dir := t.TempDir()
	folderOpen(t, dir, &State{Topics: []TopicDoc{{Name: "t", Body: "body"}}})
	if err := (FolderStore{Dir: dir}).Update(nil, func(s *State) error {
		mustApply(t, s, `<wb:topic name="t" tickets="261112-feat-a 261112-bug-b"/>`)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := topicTickets(folderOpen(t, dir, nil), "t"); !slices.Equal(got, []string{"261112-feat-a", "261112-bug-b"}) {
		t.Fatalf("tickets after reopen = %v", got)
	}
	if tree := folderTree(t, dir); !strings.Contains(tree["t1/meta.json"], `"tickets":["261112-feat-a","261112-bug-b"]`) {
		t.Fatalf("t1/meta.json lacks the tickets: %s", tree["t1/meta.json"])
	}
}

// TestPageCSSLeavesTrackerTkAlone pins the badge class name: page.css is
// inlined into the dashboard page, so a bare .tk selector there also matches
// the tracker's own .tk root and dialogs (the Ctrl+K palette squashed into a
// 17px pill once).
func TestPageCSSLeavesTrackerTkAlone(t *testing.T) {
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`\.tk(?:[^\w-]|$)`).FindString(string(css)); m != "" {
		t.Fatalf("page.css has a bare .tk selector that matches the tracker: %q", m)
	}
	if !strings.Contains(string(css), ".wbtk ") {
		t.Fatal("page.css lost the .wbtk ticket icon rule")
	}
	// No ticket-derived glow: neither a .topic.live rule nor a live class.
	if regexp.MustCompile(`\.topic\.live\b`).Match(css) {
		t.Fatal("page.css styles ticket-derived .topic.live glow")
	}
}
