package wswhiteboard

import (
	"strings"
	"testing"
)

func topicNames(st *State) []string {
	var names []string
	for _, topic := range BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"}).Topics {
		names = append(names, topic.Name)
	}
	return names
}

func wantTopicOrder(t *testing.T, st *State, want ...string) {
	t.Helper()
	got := topicNames(st)
	if len(got) != len(want) {
		t.Fatalf("topic order %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("topic order %v, want %v", got, want)
		}
	}
}

func topicView(t *testing.T, st *State, layout Layout, name string) TopicView {
	t.Helper()
	for _, topic := range BuildView(st, ViewInput{Layout: layout, ReplyID: "r"}).Topics {
		if topic.Name == name {
			return topic
		}
	}
	t.Fatalf("no topic %q in the view", name)
	return TopicView{}
}

// The topic with the most recent activity comes first, whatever waits on the
// user; a quiet topic is ordered the same way.
func TestTopicsOrderByRecency(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "older", "a1", "x", "y")+addXML(ClassPolicy, "older", "a2", "x", "y"))
	mustApply(t, st, addXML(ClassPolicy, "newer", "b1", "x", "y"))
	wantTopicOrder(t, st, "newer", "older")

	// Answering an item in the older topic is activity; it rises above the
	// newer one.
	mustApply(t, st, `<wb:answer id="1" value="a">go with a</wb:answer>`)
	wantTopicOrder(t, st, "older", "newer")

	// A patch counts as activity too.
	mustApply(t, st, `<wb:patch id="3"><wb:title>b1 reworded</wb:title></wb:patch>`)
	wantTopicOrder(t, st, "newer", "older")

	// So does a change to a topic's body: a new body, an edit of it.
	mustApply(t, st, topicXML("older", "notes"))
	wantTopicOrder(t, st, "older", "newer")
	mustApply(t, st, `<wb:patch id="3"><wb:title>b1 again</wb:title></wb:patch>`)
	wantTopicOrder(t, st, "newer", "older")
	mustApply(t, st, `<wb:topic name="older"><wb:edit><wb:old>notes</wb:old><wb:new>more notes</wb:new></wb:edit></wb:topic>`)
	wantTopicOrder(t, st, "older", "newer")
	// The same body again is no activity.
	mustApply(t, st, `<wb:patch id="3"><wb:title>b1 third</wb:title></wb:patch>`)
	mustApply(t, st, topicXML("older", "more notes"))
	wantTopicOrder(t, st, "newer", "older")

	// A quiet topic that changed last leads the open ones.
	mustApply(t, st, addXML(ClassTechnical, "quiet", "c1"))
	mustApply(t, st, `<wb:answer id="4" value="ok">fine</wb:answer>`)
	wantTopicOrder(t, st, "quiet", "newer", "older")
}

// An open policy item does not float its topic: a topic changed more recently
// with nothing waiting sits above it.
func TestOpenPolicyItemDoesNotOutrankRecentTopic(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "waiting", "p", "x", "y"))
	mustApply(t, st, topicXML("notes", "a body"))
	wantTopicOrder(t, st, "notes", "waiting")
}

// Topics that rose in the same update order by why they rose: a new policy
// question, then a new acknowledgement item, then a body or recap change,
// then anything else. A topic ranks by its strongest reason, equal ranks keep
// first-appearance order, and a later update still outranks an earlier one.
func TestSameUpdateTopicsOrderByBumpReason(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "answered", "a")+addXML(ClassTechnical, "mixed", "m"))
	// Written in the reverse of the wanted order; the item topics appear
	// before the body-only ones.
	mustApply(t, st, `<wb:answer id="1" value="ok">ok</wb:answer><wb:answer id="2" value="ok">ok</wb:answer>`+
		topicXML("mixed", "mixed body")+
		addXML(ClassTechnical, "ack", "k")+
		addXML(ClassPolicy, "policy", "p", "x", "y")+
		addXML(ClassPolicy, "assumed", "s", "x", "y")+
		`<wb:answer id="5" value="a" assumed="true">fine</wb:answer>`+
		topicXML("doc", "a body")+
		topicXML("recapped", "another body"))
	// mixed ranks by its body over its answer; an item asked and settled in
	// the same update asks nothing, so assumed ranks with the answered one.
	wantTopicOrder(t, st, "policy", "ack", "mixed", "doc", "recapped", "answered", "assumed")

	// An answer in a later update outranks every reason of an earlier one.
	mustApply(t, st, `<wb:answer id="3" value="ok">ok</wb:answer>`)
	wantTopicOrder(t, st, "ack", "policy", "mixed", "doc", "recapped", "answered", "assumed")
}

// A re-ask counts as its class's add; an edit that asks nothing does not.
func TestSameUpdateReaskRanksAsItsClassAdd(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "edited", "e")+
		addXML(ClassTechnical, "reacked", "k")+
		addXML(ClassPolicy, "reasked", "p", "x", "y"))
	mustApply(t, st, `<wb:answer id="2" value="ok">ok</wb:answer>`)
	mustApply(t, st, `<wb:patch id="1"><wb:why>more context</wb:why></wb:patch>`+
		`<wb:patch id="2" status="open"/>`+
		`<wb:patch id="3"><wb:title>p reworded</wb:title></wb:patch>`)
	wantTopicOrder(t, st, "reasked", "reacked", "edited")
}

// A topic with an open body is not Quiet; topics order by recency alone.
func TestTopicWithABodyIsNotQuiet(t *testing.T) {
	st := &State{}
	settleItems(t, st, "quiet", 1)
	settleItems(t, st, "filler", ExpandedSettled) // pushes the quiet topic's item out of the expanded ten
	mustApply(t, st, topicXML("notes", "a body"))
	mustApply(t, st, addXML(ClassTechnical, "acks", "k"))
	mustApply(t, st, addXML(ClassPolicy, "policy", "p", "x", "y"))
	wantTopicOrder(t, st, "policy", "acks", "notes", "filler", "quiet")
	if notes := topicView(t, st, LayoutFull, "notes"); notes.Quiet || notes.Doc != "a body" || !notes.DocOpen(false) {
		t.Fatalf("notes = %+v", notes)
	}
	if quiet := topicView(t, st, LayoutFull, "quiet"); !quiet.Quiet || quiet.Doc != "" {
		t.Fatalf("quiet = %+v", quiet)
	}
	// A quiet topic that gains a body is no longer quiet.
	mustApply(t, st, topicXML("quiet", "now with notes"))
	if quiet := topicView(t, st, LayoutFull, "quiet"); quiet.Quiet || !quiet.DocOpen(false) {
		t.Fatalf("a quiet topic that gains an open body is not quiet: %+v", quiet)
	}
	wantTopicOrder(t, st, "quiet", "policy", "acks", "notes", "filler")
}

// A board stored before AnswerSeq existed derives the answer point from its
// answered items rather than ranking every body as unread.
func TestLastAnswerSeqFallsBackForOlderBoards(t *testing.T) {
	st := &State{}
	settleItems(t, st, "settled", 1)
	mustApply(t, st, topicXML("notes", "a body"))
	st.AnswerSeq = 0
	if got := st.LastAnswerSeq(); got == 0 {
		t.Fatalf("LastAnswerSeq = 0 with an answered item on the board")
	}
	if (&State{}).LastAnswerSeq() != 0 {
		t.Fatal("an empty board has no answer")
	}
}

// A topic with a body and no items is a topic block of its own; closing it
// makes it Quiet with its body behind a closed fold.
func TestContentOnlyTopicInTheView(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("notes", "just notes"))
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if len(view.Topics) != 1 || view.Answerable || view.BulkAck {
		t.Fatalf("view = %+v", view)
	}
	notes := view.Topics[0]
	if notes.Name != "notes" || notes.Doc != "just notes" || notes.Quiet || notes.Closed || !notes.DocOpen(false) || notes.SettledCount() != 0 {
		t.Fatalf("notes = %+v", notes)
	}
	mustApply(t, st, `<wb:topic name="notes"><wb:close result="done"/></wb:topic>`)
	notes = topicView(t, st, LayoutFull, "notes")
	if !notes.Quiet || !notes.Closed || notes.Doc != "just notes" || notes.DocOpen(false) || notes.DocOpen(true) {
		t.Fatalf("closed notes = %+v", notes)
	}
}

func TestBodyOpenOnTheFullPageUnlessClosed(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "body"))
	if tv := topicView(t, st, LayoutFull, "t"); !tv.DocOpen(false) || tv.DocNew {
		t.Fatalf("full page topic = %+v", tv)
	}
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	if tv := topicView(t, st, LayoutFull, "t"); tv.DocOpen(false) || tv.Doc != "body" {
		t.Fatalf("closed topic on the full page = %+v", tv)
	}
	// No body, nothing to open.
	if (TopicView{}).DocOpen(false) || (TopicView{}).DocOpen(true) {
		t.Fatal("a topic without a body has no open body")
	}
}

// The embed shows a body open only when it changed since the previous embed.
func TestEmbedShowsABodyOpenOnlyWhenChanged(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "a", "x")+topicXML("a", "body a")+topicXML("b", "body b"))
	// The first embed: both bodies are new.
	for _, name := range []string{"a", "b"} {
		if tv := topicView(t, st, LayoutEmbed, name); !tv.DocNew || !tv.DocOpen(true) || tv.Quiet {
			t.Fatalf("%s before any embed = %+v", name, tv)
		}
	}
	st.LastEmbedSeq = st.Seq
	// Change only b's body.
	mustApply(t, st, `<wb:topic name="b"><wb:edit><wb:old>body b</wb:old><wb:new>body b2</wb:new></wb:edit></wb:topic>`)
	a, b := topicView(t, st, LayoutEmbed, "a"), topicView(t, st, LayoutEmbed, "b")
	if a.DocNew || a.DocOpen(true) || !b.DocNew || !b.DocOpen(true) {
		t.Fatalf("embed a = %+v b = %+v", a, b)
	}
	// The full page is unaffected by the embed's freshness.
	if a.Doc != "body a" || b.Doc != "body b2" {
		t.Fatalf("bodies = %q %q", a.Doc, b.Doc)
	}
	if full := topicView(t, st, LayoutFull, "a"); full.DocNew || !full.DocOpen(false) {
		t.Fatalf("full page a = %+v", full)
	}
	if bq := topicView(t, st, LayoutEmbed, "b"); bq.Quiet {
		t.Fatalf("a topic whose body changed is not quiet in the embed: %+v", bq)
	}
	// The same body again is no change, so it stays closed on the embed.
	st.LastEmbedSeq = st.Seq
	mustApply(t, st, topicXML("b", "body b2"))
	if tv := topicView(t, st, LayoutEmbed, "b"); tv.DocNew || tv.DocOpen(true) || !tv.Quiet {
		t.Fatalf("an unchanged body on the embed = %+v", tv)
	}
	// A closed topic never opens its body, changed or not.
	mustApply(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`)
	if tv := topicView(t, st, LayoutEmbed, "a"); tv.DocOpen(true) || !tv.Closed {
		t.Fatalf("closed topic on the embed = %+v", tv)
	}
}

// A body's change marks go stale (dimmed) once an item of its topic changes
// after the body's latest edit; a log entry does not count, and the next
// body edit makes them fresh again.
func TestDocStaleFollowsItemChangesAfterTheBodyEdit(t *testing.T) {
	stale := func(st *State, name string) bool { return topicView(t, st, LayoutFull, name).DocStale }
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+addXML(ClassTechnical, "other", "y"))
	mustApply(t, st, topicXML("t", "one\n\ntwo"))
	mustApply(t, st, topicXML("t", "one\n\ntwo\n\nthree"))
	if stale(st, "t") {
		t.Fatal("a body edit with no item change after it is fresh")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:log>a log line</wb:log></wb:topic>`)
	if stale(st, "t") {
		t.Fatal("a log entry does not stale the marks")
	}
	mustApply(t, st, addXML(ClassTechnical, "other", "z"))
	if stale(st, "t") {
		t.Fatal("an item of another topic does not stale the marks")
	}
	mustApply(t, st, addXML(ClassTechnical, "t", "added"))
	if !stale(st, "t") {
		t.Fatal("an item added after the body edit stales the marks")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>three</wb:old><wb:new>THREE</wb:new></wb:edit></wb:topic>`)
	if stale(st, "t") {
		t.Fatal("a later body edit is fresh again")
	}
	mustApply(t, st, `<wb:answer id="1" value="a">A</wb:answer>`)
	if !stale(st, "t") {
		t.Fatal("an answer after the body edit stales the marks")
	}
	mustApply(t, st, topicXML("t", "one\n\ntwo\n\nfour"))
	mustApply(t, st, `<wb:patch id="1"><wb:why>p</wb:why></wb:patch>`)
	if !stale(st, "t") {
		t.Fatal("a patch after the body edit stales the marks")
	}
}

// TestDocStaleClassReachesBothRenderPaths pins the class chg.js dims stale
// change marks from: a stale body carries chg-stale on the disk page's
// inline body, and on a served page both on the frame's box and on the body
// inside the frame, where chg.js runs; a fresh body carries it nowhere.
func TestDocStaleClassReachesBothRenderPaths(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x"))
	mustApply(t, st, topicXML("t", "one\n\ntwo"))
	mustApply(t, st, topicXML("t", "one\n\ntwo\n\nthree"))
	fresh := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	mustApply(t, st, addXML(ClassTechnical, "t", "added"))
	stale := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})

	if page := render(t, stale); !strings.Contains(page, `<div class="doc-body md chg-stale">`) {
		t.Error("the disk page's stale body lacks chg-stale")
	}
	regions, err := RenderRegions(stale)
	if err != nil {
		t.Fatal(err)
	}
	board := string(regions.Board)
	if !strings.Contains(board, `<div class="doc-body framed chg-stale"><iframe`) {
		t.Error("the served page's stale body frame box lacks chg-stale")
	}
	if doc := frameDocs(t, board)["t"]; !strings.Contains(doc, `<body class="doc-frame"><div class="doc-body md chg-stale">`) {
		t.Errorf("the stale body inside its frame lacks chg-stale:\n%.400s", doc)
	}

	// The stylesheet and chg.js name the class too, so the checks are on the
	// body boxes.
	if page := render(t, fresh); !strings.Contains(page, `<div class="doc-body md">`) || strings.Contains(page, `md chg-stale"`) {
		t.Error("the disk page marks a fresh body stale")
	}
	regions, err = RenderRegions(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if board := string(regions.Board); !strings.Contains(board, `<div class="doc-body framed"><iframe`) || strings.Contains(board, `chg-stale"`) {
		t.Error("the served page marks a fresh body stale")
	}
}
