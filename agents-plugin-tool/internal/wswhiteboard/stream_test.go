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

func TestLegacyCloseEventDisplayPreservesHistory(t *testing.T) {
	const original = "Topic closed: private old conclusion"
	st := &State{EntrySeq: 3, ClosedTopics: []string{"t"}, Topics: []TopicDoc{{Name: "t", Entries: []Entry{
		{ID: "e1", Kind: EntryAgent, Text: "Current recap"},
		{ID: "e2", Kind: EntrySystem, Text: original},
		{ID: "e3", Kind: EntrySystem, Imported: true, Text: "Topic closed: imported text"},
	}}}}
	before := mustMarshal(t, st)
	for _, layout := range layouts() {
		view := BuildView(st, ViewInput{Layout: layout, ReplyID: "r"})
		out := render(t, view)
		wantAll(t, out, "Current recap")
		wantNone(t, out, original)
	}
	_, page, _ := streamView(st.Topics[0], true, time.UTC, nil)
	entries := pageEntries(page)
	if len(entries) != 3 || entries[1].Text != "Topic closed." || entries[2].Text != "Topic closed: imported text" {
		t.Fatalf("unexpected projected events: %+v", entries)
	}
	if !reflect.DeepEqual(before, mustMarshal(t, st)) {
		t.Fatal("rendering modified immutable history")
	}
}

func TestHistoricalCloseProjectionKeepsRunAndChronologicalCurrent(t *testing.T) {
	st := stateBoard(t)
	stateApply(t, st, `<wb:topic name="Work"><wb:state value="implementing" owner="worker" eta_minutes="10"/></wb:topic>`, stateNow)
	stateApply(t, st, `<wb:topic name="Work"><wb:close/></wb:topic>`, stateNow.Add(time.Minute))
	doc := st.topicDoc("Work")
	closeEntry := &doc.Entries[len(doc.Entries)-1]
	closeEntry.Text = "Topic closed: legacy secret; overwritten transition"
	doc.Entries = append(doc.Entries,
		Entry{ID: "e90", Kind: EntrySystem, Text: "Topic close result updated: replacement secret"},
		Entry{ID: "e91", Kind: EntryAgent, Text: "newer line", At: stateNow.Add(2 * time.Minute).Format(time.RFC3339Nano)},
		Entry{ID: "e92", Kind: EntryAgent, Text: "migrated older line", At: stateNow.Format(time.RFC3339Nano)},
	)
	before := mustMarshal(t, st)
	_, page, _ := streamView(*doc, true, time.UTC, nil)
	entries := pageEntries(page)
	var closeText, recloseText string
	var newerCurrent, olderCurrent bool
	for _, e := range entries {
		switch e.ID {
		case closeEntry.ID:
			closeText = e.Text
		case "e90":
			recloseText = e.Text
		case "e91":
			newerCurrent = e.Current
		case "e92":
			olderCurrent = e.Current
		}
	}
	for _, want := range []string{"Topic closed.", "implementing -> cleared", "owner worker", "estimate 10 minutes", "actual 60000 ms"} {
		if !strings.Contains(closeText, want) {
			t.Errorf("close text %q missing %q", closeText, want)
		}
	}
	if strings.Contains(closeText, "legacy secret") || recloseText != "Topic closed." || !newerCurrent || olderCurrent {
		t.Fatalf("close=%q reclose=%q newerCurrent=%v olderCurrent=%v", closeText, recloseText, newerCurrent, olderCurrent)
	}
	for _, layout := range layouts() {
		out := render(t, BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}))
		wantNone(t, out, "legacy secret", "replacement secret")
	}
	if !reflect.DeepEqual(before, mustMarshal(t, st)) {
		t.Fatal("projection modified history")
	}
}

func TestStreamViewOrderFoldsAndLatestAgent(t *testing.T) {
	st := streamFixture()
	before := mustMarshal(t, st)
	tv := topicView(t, st, LayoutFull, "t")
	if tv.LatestAgent == nil || tv.LatestAgent.ID != "e5" {
		t.Fatalf("latest-log identity changed: %+v", tv.LatestAgent)
	}
	if tv.Head == nil || tv.Head.ID != "e9" || tv.Summary() != "recap e9" {
		t.Fatalf("latest panel did not replace head summary: %+v", tv.Head)
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
	if closed.Quiet {
		t.Fatal("closed topic's whole history must remain visible")
	}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	wantAll(t, out, `<div class="topic-stream">`, `class="wb-entry panel historical" id="e3"`, `data-key="entry:e3:history"`, `<details class="system-run`)
	wantNone(t, out, `<details class="topic-stream"`, `data-key="history:t"`, `<details class="settled"`, `data-key="versions:`)
}

// Every panel post appends a version at its own write position. An earlier
// version stays folded in place, links to the newest version and keeps its
// exact eN link, while the latest keeps the pN target. Panel post events are
// left out of the view but stay in the store; a later agent log replaces
// the panel recap in the header.
func TestPanelVersionsKeepWriteOrderAndLinks(t *testing.T) {
	st := &State{}
	at := func(minutes int, commands string) {
		t.Helper()
		env := testEnv()
		env.Now = testNow.Add(time.Duration(minutes) * time.Minute)
		if _, problems := Apply(st, commands, env); len(problems) > 0 {
			t.Fatalf("Apply(%s) refused: %v", commands, problems)
		}
	}
	at(0, `<wb:topic name="t"><wb:panel name="plan" recap="Plan draft">PLAN-V1</wb:panel>`+
		`<wb:add class="policy"><wb:title>Pick a plan</wb:title><wb:option>A</wb:option><wb:option>B</wb:option></wb:add></wb:topic>`)
	at(5, `<wb:topic name="t"><wb:log>between versions</wb:log></wb:topic>`)
	at(10, panelCommandXML("t", `id="p1" recap="Plan revised"`, "PLAN-V2")) // id-only, as legacy reposts are
	at(15, panelCommandXML("t", `recap="Other"`, "OTHER-V1"))
	at(20, panelCommandXML("t", `name="plan" recap="Plan final"`, "PLAN-V3"))
	at(25, `<wb:topic name="t"><wb:log>latest agent line</wb:log></wb:topic>`)
	var ids []string
	events := 0
	for _, e := range st.Topics[0].Entries {
		if e.Kind == EntryPanel {
			ids = append(ids, e.ID)
		}
		if panelEvent(e) {
			events++
		}
	}
	if len(ids) != 4 || events != 4 {
		t.Fatalf("store must keep every version and its post event: panels %v, events %d", ids, events)
	}
	v1, v2, other, v3 := ids[0], ids[1], ids[2], ids[3]
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r", Location: time.UTC})
	tv := view.Topics[0]
	if tv.Summary() != "latest agent line" || tv.PanelVersions() != 4 || len(tv.PanelRecaps) != 2 {
		t.Fatalf("header %q, versions %d, latest panels %d", tv.Summary(), tv.PanelVersions(), len(tv.PanelRecaps))
	}
	var got []string
	for _, e := range pageEntries(tv.Stream) {
		if panelEvent(e.Entry) {
			t.Fatalf("panel post event %q shown in the history", e.Text)
		}
		if e.Kind == EntryPanel {
			got = append(got, fmt.Sprintf("%s v%d/%d %s hist=%v newest=%s prior=%s", e.ID, e.Version, e.Of, e.BoundName, e.Historical, e.Newest.ID, e.Prior.Label()))
		}
	}
	want := []string{
		v1 + " v1/3 plan hist=true newest=" + v3 + " prior=v0",
		v2 + " v2/3 plan hist=true newest=" + v3 + " prior=v1 · 09:00",
		other + " v1/1  hist=false newest=" + other + " prior=v0",
		v3 + " v3/3 plan hist=false newest=" + v3 + " prior=v2 · 09:10",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("versions\n got %q\nwant %q", got, want)
	}
	out := markup(render(t, view))
	previous, rows := -1, 0
	for _, e := range st.Topics[0].Entries {
		if e.Kind != EntryPanel && e.Kind != EntryAgent {
			continue
		}
		rows++
		index := strings.Index(out, `id="`+e.ID+`"`)
		if index < previous || index < 0 {
			t.Fatalf("%s %q moved from its write position", e.ID, e.Text)
		}
		previous = index
	}
	if rows != 6 {
		t.Fatalf("logs and panels: %d", rows)
	}
	wantAll(t, out, "PLAN-V1", "PLAN-V2", "PLAN-V3", "OTHER-V1",
		`<article class="wb-entry panel historical" id="`+v1+`"`,
		// The question it leads cannot reopen an earlier version.
		`data-key="entry:`+v1+`:history"><summary>`,
		`<span class="pfname" title="Panel name">plan</span><span class="pfver old" title="Version 1 of 3">v1</span>`,
		`<a class="pfnewer" href="#`+v3+`" title="Go to the newer version">→ v3 · 09:20</a><a class="mref pfexact" href="#`+v1+`" title="Link to this version (fixed)">#`+v1+`</a>`,
		`Earlier version v1 · 09:00. <a href="#`+v3+`">Latest is v3 · 09:20 ↓</a></div>`,
		`<span class="pfver" title="Version 3 of 3 · latest">v3</span>`,
		`<a class="mref" href="#p1" title="Link to the latest panel">#p1</a>`,
		`<div class="pfprev"><a href="#`+v2+`">Earlier version v2 · 09:10 ↑</a></div>`,
		`data-ver="v1 · 09:00" data-newest="v3 · 09:20"`, `data-ver="v3 · 09:20" data-newest="v3"`,
		`panels 2 · versions 4 · log `)
	wantNone(t, out, "re-posted: Plan", "added: Plan", `class="panel-versions"`)
	if strings.Count(out, `id="p1"`) != 1 || strings.Count(out, `class="pfver`) != 3 || strings.Count(out, `class="pfoldnote"`) != 2 {
		t.Fatal("one pN target, versions only on the three-version panel, notes only on earlier versions")
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
	live := regexp.MustCompile(`(?s)<template data-history-gap>.*?</template>`).ReplaceAllString(out, "")
	if strings.Count(live, `class="wb-entry agent`) != 1 || !strings.Contains(live, `id="e123"`) || strings.Contains(live, `id="e122"`) {
		t.Fatal("initial live DOM must hold the current line and legacy latest entry, not a fifty-entry tail")
	}
	// All log entries are visible (no run-fold disclosure); only e123 is current.
	wantAll(t, live, `class="wb-entry agent current" id="e123"`)
	wantNone(t, live, `class="run-fold"`)
	if strings.Count(live, ` current"`) != 1 {
		t.Fatal("more than one line is current")
	}
	wantAll(t, out, `<div class="history-gap" data-count="122"`, `<template data-history-gap>`, `data-load="up"`, `data-load="down" hidden`)
}

// The history header stays a plain row. Loading controls now belong to
// individual gaps, never a global older button or a clipped tail.
func TestStreamHistoryHeaderIsOneRow(t *testing.T) {
	st := &State{Topics: []TopicDoc{{Name: "long"}}}
	for i := 1; i <= 60; i++ {
		st.Topics[0].Entries = append(st.Topics[0].Entries, Entry{ID: fmt.Sprintf("e%d", i), Kind: EntryAgent, Text: fmt.Sprintf("line %d", i), At: testNow.Format(time.RFC3339)})
	}
	mustApply(t, st, `<wb:topic name="short"><wb:log>one</wb:log></wb:topic>`+
		`<wb:topic name="done"><wb:log>final</wb:log><wb:close result="Shipped."/></wb:topic>`)
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	head := func(name string) string {
		return `<div class="topic-stream"><div class="wb-history" data-history="` + name + `" aria-label="History"><div class="mhead"><span class="wb-mtitle">`
	}
	wantAll(t, out, head("long"), head("short"), head("done"),
		`<span class="wb-mlabel">History</span><span class="n">60</span></span><span></span><span class="mmeta">`,
		// "short" holds its log and its creation-default state event.
		`<span class="n">2</span></span><span></span><span class="mmeta">`)
	wantNone(t, out, `<details class="topic-stream"`, `data-key="history:`, `<summary class="mhead">`, `class="history-older"`)
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	wantAll(t, string(css), `.wb-history > .mhead { display: grid; grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr); min-height: 26px; }`,
		`.gap-actions { display: inline-flex; gap: 4px; }`,
		`min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }`,
		`.wb-mtitle > .wb-mlabel { min-width: 0; overflow: hidden; text-overflow: ellipsis; }`,
		`.wb-mtitle > svg, .wb-mtitle > .n { flex: none; }`)
	wantNone(t, string(css), `.history-clip::after`, `linear-gradient(var(--wb-bg), transparent)`, `.topic-stream > summary`)
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
	// The stream is chronological; p1's earlier version (e3) keeps its slot.
	previous := -1
	for i := 1; i <= 10; i++ {
		index := strings.Index(topic, fmt.Sprintf(`id="e%d"`, i))
		if index < previous || index < 0 {
			t.Fatalf("entry %d moved from its chronological slot", i)
		}
		previous = index
	}
	wantAll(t, board, `data-entry="e1" data-kind="system" data-entry-item="17"`, `System events 2 · `, `data-key="entry:e3:history"><summary`, `data-key="entry:e9" id="p1" open>`, `<span class="pfver old" title="Version 1 of 2">v1</span>`, `data-doc="entry:e3" data-doc-key="entry:e3" data-topic="t" data-entry="e3"`, `data-doc="topic:t/e3" data-doc-key="topic:t/e3" data-topic="t/e3"`)
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
		wantAll(t, d, `<!DOCTYPE html><html>`, frameCSSLink(t), `<meta name="wb-doc-key" content="`+key+`">`)
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

// Segments interleaves latest panel cards with log runs, every row of a run
// shown. An owner answer is its own row, and other system events share one
// chip.
func TestStreamSegmentsNeverFoldRuns(t *testing.T) {
	v := func(id, kind string) EntryView { return EntryView{Entry: Entry{ID: id, Kind: kind}} }
	panel := v("p", EntryPanel)
	panel.PanelID = "p1"
	owner := v("o", EntrySystem)
	owner.ItemID, owner.Text = 4, "Item {{4}} answered [a]"
	page := EntryPage{Runs: []EntryRun{
		{Entry: v("a1", EntryAgent)}, {Systems: []EntryView{v("s1", EntrySystem), v("s2", EntrySystem)}},
		{Entry: v("a2", EntryAgent)}, {Entry: v("a3", EntryAgent)},
		{Entry: panel},
		{Systems: []EntryView{v("s3", EntrySystem), owner, v("s4", EntrySystem)}},
		{Entry: v("a4", EntryAgent)}, {Entry: v("a5", EntryAgent)},
	}}
	shape := func(segs []StreamSegment) []string {
		var out []string
		for _, s := range segs {
			if s.IsPanel() {
				out = append(out, "panel:"+s.Panel.ID)
				continue
			}
			out = append(out, fmt.Sprintf("rows:%d", len(s.Rows)))
		}
		return out
	}
	segs := page.Segments()
	if got := fmt.Sprint(shape(segs)); got != "[rows:4 panel:p rows:5]" {
		t.Fatalf("segments %s", got)
	}
	// s3 | owner | s4: the owner answer splits the chips around it.
	last := segs[2]
	if len(last.Rows[0].Systems) != 1 || last.Rows[0].Systems[0].ID != "s3" || last.Rows[1].Entry.ID != "o" || !last.Rows[1].Owner() || len(last.Rows[2].Systems) != 1 || last.Rows[2].Systems[0].ID != "s4" {
		t.Fatal("an owner answer is not its own row between system chips")
	}
}

func TestExpandedQuietTopicCardCSS(t *testing.T) {
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	// One section surface aligns both body variants and the stream with the
	// bar. A dashboard fold stays transparent unless temporarily maximized;
	// standalone details acquire the surface only while open.
	wantAll(t, string(css),
		`.topic { background: var(--wb-card); border-radius: var(--wb-radius); box-shadow: var(--wb-shadow);`,
		`.topic.quiet { box-shadow: none; background: transparent; }`,
		`.topic.quiet[data-size]:not([data-size="fold"]),
.topic.quiet:not([data-size]):has(> details.tdoc[open]),
.topic.quiet[data-max] { background: var(--wb-card); border-radius: var(--wb-radius); box-shadow: var(--wb-shadow); }`,
		`.entry-text { flex: 1; min-width: 0; overflow-wrap: anywhere; }`,
		`.topic.quiet .tbar details.settled[open] { flex: 1 0 100%; width: 100%; min-width: 0; padding-bottom: 4px; }`,
		`.topic.quiet .tbar details.settled .chead { flex-wrap: wrap; }`,
	)
	// The native disclosure must not draw a second, inset card inside the
	// section; framed div.tdoc has no native open attribute at all.
	wantNone(t, string(css), `.topic.quiet details.tdoc[open] {`)
	for _, selector := range regexp.MustCompile(`\.topic\.quiet[^,{]*:has\([^)]*(?:div\.tdoc|\.topic-stream)[^)]*\)`).FindAllString(string(css), -1) {
		guard := strings.SplitN(selector, ":has(", 2)[0]
		if !strings.Contains(guard, "[data-size]") {
			t.Errorf("quiet surface has a body/stream condition without an explicit dashboard size: %s", selector)
		}
	}
}

func TestStandaloneClosedTopicWithHistoryKeepsBodyCollapsed(t *testing.T) {
	st := streamFixture()
	st.ClosedTopics = []string{"t"}
	out := markup(render(t, BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})))
	// Panel diff annotations can put newlines in the rendered body.
	topic := regexp.MustCompile(`(?s)<section class="topic[^>]* data-topic="t"[^>]*>.*?</section>`).FindString(out)
	if topic == "" {
		t.Fatal("closed topic missing")
	}
	wantAll(t, topic, `class="topic closed has-doc"`, `<details class="tdoc" data-key="doc:t"`, `<div class="topic-stream">`)
	wantNone(t, topic, `class="topic quiet`, `data-size=`, `data-max`, `<details class="topic-stream"`)
	disclosure := regexp.MustCompile(`<details class="tdoc"[^>]*>`).FindString(topic)
	wantNone(t, disclosure, ` open`)
	// Whole history stays visible on a closed full-page topic; only its body
	// retains the standalone disclosure, independently of dashboard sizing.
}

// Owner rule: a log row's role marker sits on the first line beside the clock,
// never vertically centred against wrapped text.
func TestStreamRoleMarkerPinsToFirstLine(t *testing.T) {
	css, err := whiteboardTestAsset("page.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(css), `.entry-line > .evi { align-self: flex-start;`) {
		t.Error("page.css does not pin the stream role marker to the first line")
	}
	if regexp.MustCompile(`\.evi \{[^}]*align-self: center`).Match(css) {
		t.Error("page.css centres the role marker on wrapped rows")
	}
}
