package wswhiteboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"
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
// its linked tickets is kept (dropTopicDoc), and dropped once they clear.
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
		if !strings.Contains(page, `data-topic="links"`) || strings.Contains(page, `class="ttix"`) || strings.Contains(page, `class="wbtk `) {
			t.Errorf("%s: tickets-only topic missing or ticket slot rendered", name)
		}
	}
	mustApply(t, st, `<wb:topic name="links" tickets=""/>`)
	if st.topicKnown("links") {
		t.Fatal("a topic left with nothing persisted")
	}
}

// TestTopicTicketsAreQuietMetadata pins Decision 18: setting or clearing a
// topic's tickets moves no recency, reopens no closed topic, and records no
// entry.
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
			if doc.Changed != beforeDocs[i].Changed || doc.ChangedAt != beforeDocs[i].ChangedAt || len(doc.Entries) != len(beforeDocs[i].Entries) {
				t.Fatalf("%s touched %q: %+v", batch, doc.Name, doc)
			}
		}
		if !st.topicClosed("done") {
			t.Fatalf("%s reopened the closed topic", batch)
		}
	}
}

// TestTopicTicketsStayHidden pins UI-only hiding: internal ticket resolution
// remains intact, but pages and served fragments show neither the ticket slot
// nor ticket-derived glow. Authored content and items remain visible.
func TestTopicTicketsStayHidden(t *testing.T) {
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

	slot := regexp.MustCompile(`class="[^"]*\b(?:ttix|wbtk)\b`)
	glow := regexp.MustCompile(`<section class="topic[^"]*\blive\b`)
	check := func(t *testing.T, page string, body bool) {
		t.Helper()
		if slot.MatchString(page) || glow.MatchString(page) {
			t.Error("rendered a ticket slot or ticket-derived topic glow")
		}
		fragments := []string{`data-topic="t"`, "Authored 261112-feat-live content", "Review 261112-feat-live item"}
		if body {
			fragments = append(fragments, "Authored 261112-feat-live body")
		}
		for _, frag := range fragments {
			if !strings.Contains(page, frag) {
				t.Errorf("topic content missing %q", frag)
			}
		}
	}
	for _, c := range []struct {
		name    string
		tickets string
		resolve fakeTickets
	}{
		{"resolved", "261112-feat-live 261112-bug-shipped 261112-refactor-gone", resolve},
		{"missing", "261112-feat-missing", resolve},
		{"no-resolver", "261112-feat-live", nil},
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
						check(t, render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r", Tickets: c.resolve})), layout == LayoutFull)
					})
				}
				view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: c.resolve})
				regions, err := RenderRegions(view)
				if err != nil {
					t.Fatal(err)
				}
				check(t, string(regions.Board), true)
				if closed {
					page, err := RenderClosedTopics(BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Tickets: c.resolve, Closed: &ClosedWindow{From: 0, N: 30}}))
					if err != nil {
						t.Fatal(err)
					}
					check(t, string(page), true)
				}
			}
		})
	}
}

// TestTopicHeadHoldsTheSummaryLine pins the sticky head: an open topic's
// .tophead holds its bar, note, and a summary line with the latest agent
// line and the check pill (done/total, as the closed fold's header reads), the check rows staying in flow
// after the head; a topic with no checks has no pill; a closed topic's head
// has no summary line; and a legacy recap stays a card under the head.
func TestTopicHeadHoldsTheSummaryLine(t *testing.T) {
	st := checkBoard(t)
	mustApply(t, st, `<wb:topic name="t"><wb:log>agent line</wb:log></wb:topic>`)
	page := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	section := page[strings.Index(page, `data-topic="t"`):]
	head := section[:strings.Index(section, `<div class="checks">`)]
	for _, frag := range []string{`<div class="tophead"><div class="tbar">`, `<textarea class="note tnote"`, `<div class="tsub"><div class="trecap" data-entry=`, `agent line`, `<div class="ckpill"><button type="button" class="ckbtn" aria-expanded="true">`, `<span class="prog">answered 3 / 4</span>`} {
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
		raw, err := assetFS.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if loc := thead.FindIndex(raw); loc != nil {
			t.Errorf("%s uses the class thead: %q", name, raw[max(0, loc[0]-40):min(len(raw), loc[1]+40)])
		}
	}
	for _, name := range []string{"assets/page.css", "assets/page.tmpl", "assets/rail.js"} {
		raw, _ := assetFS.ReadFile(name)
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
	css, err := embeddedAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`\.tk(?:[^\w-]|$)`).FindString(string(css)); m != "" {
		t.Fatalf("page.css has a bare .tk selector that matches the tracker: %q", m)
	}
	if !strings.Contains(string(css), ".wbtk ") {
		t.Fatal("page.css lost the .wbtk badge rule")
	}
}
