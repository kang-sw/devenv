package wswhiteboard

import (
	"fmt"
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func streamFixture() *State {
	at := testNow.Format(time.RFC3339Nano)
	var entries []Entry
	add := func(kind, panel, text string) {
		e := Entry{ID: fmt.Sprintf("e%d", len(entries)+1), Kind: kind, PanelID: panel, Text: text, At: at}
		if kind == EntryPanel {
			e.Recap = "recap " + e.ID
		}
		entries = append(entries, e)
	}
	add(EntrySystem, "", "first system")
	add(EntrySystem, "", "second system")
	add(EntryPanel, "p1", "old p1 payload")
	add(EntrySystem, "", "panel event")
	add(EntryAgent, "", "agent state")
	add(EntryPanel, "p2", "p2 payload")
	add(EntryPanel, "p3", "p3 payload")
	add(EntryPanel, "p4", "p4 payload")
	add(EntryPanel, "p1", "latest p1 payload")
	add(EntrySystem, "", "last system, not a summary")
	return &State{Seq: 2, EntrySeq: len(entries), PanelCounter: 4, Topics: []TopicDoc{{Name: "t", Body: "current content", Entries: entries}}}
}

func pageEntries(page EntryPage) []EntryView {
	var out []EntryView
	for _, run := range page.Runs {
		if len(run.Systems) > 0 {
			out = append(out, run.Systems...)
		} else {
			out = append(out, run.Entry)
		}
	}
	return out
}

func TestStreamViewOrderFoldsAndLatestAgent(t *testing.T) {
	st := streamFixture()
	before := mustMarshal(t, st)
	tv := topicView(t, st, LayoutFull, "t")
	if tv.LatestAgent == nil || tv.LatestAgent.ID != "e5" || tv.Summary() != "agent state" {
		t.Fatalf("summary replaced by non-agent entry: %+v", tv.LatestAgent)
	}
	if tv.Stream.Count != 10 || len(tv.Stream.Runs[0].Systems) != 2 || tv.Stream.Runs[1].Entry.ID != "e3" || len(tv.Stream.Runs[2].Systems) != 1 {
		t.Fatalf("system folding swallowed/reordered a panel: %+v", tv.Stream)
	}
	var expanded []string
	for i, e := range pageEntries(tv.Stream) {
		if e.ID != fmt.Sprintf("e%d", i+1) || e.At != testNow.Format(time.RFC3339Nano) {
			t.Fatalf("equal-time order lost at %d: %+v", i, e)
		}
		if e.Open {
			expanded = append(expanded, e.ID)
		}
		if e.ID == "e3" && !e.Historical {
			t.Fatal("the original p1 slot must remain a folded historical version")
		}
	}
	if !reflect.DeepEqual(expanded, []string{"e7", "e8", "e9"}) {
		t.Fatalf("expanded %v, want latest three distinct current panel versions", expanded)
	}
	if string(before) != string(mustMarshal(t, st)) {
		t.Fatal("view construction changed durable state")
	}
	st.ClosedTopics = []string{"t"}
	closed := topicView(t, st, LayoutFull, "t")
	for _, e := range pageEntries(closed.Stream) {
		if e.Open {
			t.Fatal("closed topic expanded a panel")
		}
	}
	if !closed.Quiet {
		t.Fatal("closed topic must keep its stream folded")
	}
}

// A previously expanded latest version must not inherit that open state once
// a re-post makes it historical. Later manual historical disclosures still
// retain their own stable key across refreshes.
func TestPanelRepostChangesHistoricalDisclosureKey(t *testing.T) {
	st := streamFixture()
	st.Topics[0].Entries = st.Topics[0].Entries[:8]
	before := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, before, `data-key="entry:e3" id="p1"`)
	st.Topics[0].Entries = append(st.Topics[0].Entries, Entry{ID: "e9", Kind: EntryPanel, PanelID: "p1", Recap: "new version", Text: "replacement", At: testNow.Format(time.RFC3339)})
	after := render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}))
	wantAll(t, after, `data-key="entry:e3:history"><summary`, `data-key="entry:e9" id="p1" open>`)
	wantNone(t, after, `data-key="entry:e3"`)
}

func TestStreamHistoryWindowsAreFiftyChronologicalEntries(t *testing.T) {
	st := &State{Topics: []TopicDoc{{Name: "t"}}}
	for i := 1; i <= 123; i++ {
		st.Topics[0].Entries = append(st.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", i), Kind: EntryAgent, Text: fmt.Sprintf("line %d", i), At: testNow.Format(time.RFC3339)})
	}
	tv := topicView(t, st, LayoutFull, "t")
	if tv.Stream.Count != 50 || len(tv.Older) != 2 || tv.Older[0].Count != 50 || tv.Older[1].Count != 23 {
		t.Fatalf("history window sizes: live %d, older %+v", tv.Stream.Count, tv.Older)
	}
	for i, page := range []EntryPage{tv.Stream, tv.Older[0], tv.Older[1]} {
		got := pageEntries(page)
		wantFirst := []string{"e74", "e24", "e1"}[i]
		wantLast := []string{"e123", "e73", "e23"}[i]
		if got[0].ID != wantFirst || got[len(got)-1].ID != wantLast {
			t.Fatalf("page %d range %s..%s", i, got[0].ID, got[len(got)-1].ID)
		}
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	live := out[strings.Index(out, `<div class="history-live">`):strings.Index(out, `<template data-history-page`)]
	if strings.Count(live, `class="wb-entry agent"`) != 50 || !strings.Contains(live, `id="e74"`) || strings.Contains(live, `id="e73"`) {
		t.Fatal("initial live DOM does not hold exactly the newest 50 entries")
	}
	wantAll(t, out, `<template data-history-page data-count="50">`, `<template data-history-page data-count="23">`, "Load 50 older entries")
}

func TestStreamRenderMetadataChronologyAndSandbox(t *testing.T) {
	st := streamFixture()
	st.Topics[0].Entries[0].ItemID = 17
	st.Topics[0].Entries[2].Text = "old <script>window.parent.stolen=true</script>"
	st.Topics[0].Entries[8].Text = "latest **body**"
	st.Topics = append(st.Topics, TopicDoc{Name: "t/e3", Body: "distinct topic body"})
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	topic := board[:strings.Index(board, `data-topic="t/e3"`)]
	previous := -1
	for i := 1; i <= 10; i++ {
		index := strings.Index(topic, fmt.Sprintf(`id="e%d"`, i))
		if index < previous || index < 0 {
			t.Fatalf("entry %d moved from its chronological slot", i)
		}
		previous = index
	}
	wantAll(t, board, `data-entry="e1" data-kind="system" data-entry-item="17"`, `system events 2`, `data-key="entry:e3:history"><summary`, `data-key="entry:e9" id="p1" open>`, `class="latest-version" href="#p1"`, `data-doc="entry:e3" data-doc-key="entry:e3" data-topic="t" data-entry="e3"`, `data-doc="topic:t/e3" data-doc-key="topic:t/e3" data-topic="t/e3"`)
	if strings.Count(board, `id="p1"`) != 1 || strings.Contains(board, `data-id="17"`) {
		t.Fatal("panel target duplicated or event attribution became an answer control")
	}
	docs := frameDocs(t, board)
	if len(docs) != 7 { // two topic bodies plus five immutable panel versions
		t.Fatalf("one distinct frame per content payload: %d", len(docs))
	}
	wantAll(t, docs["entry:e3"], "window.parent.stolen=true")
	wantNone(t, board, "<script>window.parent.stolen", "allow-same-origin")
	for name, d := range docs {
		key := name
		if !strings.HasPrefix(key, "entry:") {
			key = "topic:" + key
		}
		wantAll(t, d, `<!DOCTYPE html><html>`, `--wb-bg:`, `<meta name="wb-doc-key" content="`+key+`">`)
	}
	for _, theme := range []string{"light", "dark"} {
		themed := frameDocs(t, string(ThemeFrames(regions.Board, theme)))
		for _, doc := range themed {
			wantAll(t, doc, `<html data-theme="`+theme+`">`)
		}
	}
}

func TestPanelTargetsBeyondClosedTopicCapStayNavigable(t *testing.T) {
	st := &State{Topics: []TopicDoc{{Name: "open", Body: "See {{p1}}"}}}
	for i := 1; i <= ClosedCap+2; i++ {
		name := fmt.Sprintf("closed-%d", i)
		doc := TopicDoc{Name: name, Entries: []Entry{{ID: fmt.Sprintf("e%d", i), Kind: EntryPanel, PanelID: fmt.Sprintf("p%d", i), Recap: "closed panel", Text: "closed payload"}}}
		if i == 1 {
			for n := 1; n <= 60; n++ {
				doc.Entries = append(doc.Entries, Entry{ID: fmt.Sprintf("e%d", ClosedCap+2+n), Kind: EntryAgent, Text: "later line"})
			}
		}
		st.Topics = append(st.Topics, doc)
		st.ClosedTopics = append(st.ClosedTopics, name)
	}
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if len(view.Topics) != ClosedCap+1 || len(view.DeferredTopics) != 2 {
		t.Fatalf("closed cap/window changed: shown %d deferred %d", len(view.Topics), len(view.DeferredTopics))
	}
	out := markup(render(t, view))
	wantAll(t, out, `See <a href="#p1">p1</a>`, `<template data-history-topic="closed-1">`, `id="p1"`)
	if strings.Index(out, `id="p1"`) < strings.Index(out, `<template data-history-topic="closed-1">`) {
		t.Fatal("capped topic panel is live before explicit navigation")
	}
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(regions.Board), `<template data-history-topic="closed-1">`, `data-doc="entry:e1"`)
}

func TestStreamEmbedOnlyLatestLineAndPanelRecaps(t *testing.T) {
	st := streamFixture()
	st.Topics[0].Body = strings.Repeat("CURRENT-CONTENT-PAYLOAD", 50000)
	for i := range st.Topics[0].Entries {
		if st.Topics[0].Entries[i].Kind == EntryPanel {
			st.Topics[0].Entries[i].Text = strings.Repeat("PANEL-PAYLOAD", 50000)
		}
	}
	view := BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r", FullBoardLink: "file:///board/index.html"})
	out := render(t, view)
	wantAll(t, out, `data-entry="e5" data-kind="agent"`, "agent state", `href="file:///board/index.html">Open full board</a>`, `href="file:///board/index.html#p1"`, "recap e9")
	wantNone(t, markup(out), "CURRENT-CONTENT-PAYLOAD", "PANEL-PAYLOAD", "first system", "last system", "recap e3", `class="wb-history"`, "srcdoc=", `class="doc-body`)
	if len(out) > 512*1024*44/100 {
		t.Fatalf("compact embed exceeds 44%% of 512 KiB: %d bytes", len(out))
	}
	if view.Topics[0].Stream.Count != 0 || len(view.Topics[0].Older) != 0 || len(view.Topics[0].PanelRecaps) != 4 {
		t.Fatal("embed carries accumulated stream data")
	}
}

func TestPanelAuthoredReferencesAcrossAllSurfaces(t *testing.T) {
	st := streamFixture()
	st.Items = []Item{{ID: 1, Class: ClassPolicy, Topic: "t", Title: "title {{p1}} and `{{p1}}`", Status: StatusOpen,
		Options: []string{"option {{p1}}", "other"}, Recommendation: "a: rec {{p1}}", Rationale: "why {{p1}}", Links: []string{"link {{p1}} and `{{p1}}`"}}}
	st.Topics[0].Body = "content {{p1}} {{1}} {{p99}}\n\n`{{p1}}`\n\n```txt\n{{p1}}\n```\n\n<div data-ref=\"{{p1}}\">{{p1}}</div>"
	st.Topics[0].Entries[4].Text = "log {{p1}} {{1}} {{p99}} and `{{p1}}`"
	st.Topics[0].Entries[8].Text = "panel {{p1}} {{1}} {{p99}} and `{{p1}}`"
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	out := markup(render(t, view))
	for _, field := range []string{"title", "option", "rec", "why", "link", "content", "log", "panel"} {
		wantAll(t, out, field+` <a href="#p1">p1</a>`)
	}
	wantAll(t, out, `<code>{{p1}}</code>`, `<div data-ref="{{p1}}">{{p1}}</div>`, "{{p99}}", `<a href="#item-1">(01)</a>`)
	wantNone(t, out, `href="#p99"`, `<code><a href="#p1"`)
	regions, err := RenderRegions(view)
	if err != nil {
		t.Fatal(err)
	}
	docs := frameDocs(t, string(regions.Board))
	wantAll(t, docs["t"], `content <a href="#p1">p1</a>`, `<div data-ref="{{p1}}">{{p1}}</div>`)
	wantAll(t, docs["entry:e9"], `panel <a href="#p1">p1</a>`)
	// A known target outside this topic window is still linked: older
	// templates contain it and wbHistory materializes it before navigation.
	st.Topics[0].Entries = append(st.Topics[0].Entries, make([]Entry, 60)...)
	for i := 10; i < len(st.Topics[0].Entries); i++ {
		st.Topics[0].Entries[i] = Entry{ID: fmt.Sprintf("e%d", i+1), Kind: EntryAgent, Text: "tail {{p1}}", At: time.Now().UTC().Format(time.RFC3339)}
	}
	out = markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	wantAll(t, out, `tail <a href="#p1">p1</a>`)
	// A template cannot accidentally become raw host HTML on the served page.
	if strings.Contains(html.UnescapeString(regexp.MustCompile(`srcdoc="[^"]*"`).ReplaceAllString(out, "")), "allow-same-origin") {
		t.Fatal("unexpected privileged panel sandbox")
	}
}
