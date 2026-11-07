package wswhiteboard

import "testing"

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

// Within one rank the topic with the most recent activity comes first, and
// the rank stays the primary key.
func TestTopicsOrderByRecencyWithinRank(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "older", "a1", "x", "y")+addXML(ClassPolicy, "older", "a2", "x", "y"))
	mustApply(t, st, addXML(ClassPolicy, "newer", "b1", "x", "y"))
	wantTopicOrder(t, st, "newer", "older")

	// Answering an item in the older topic is activity; it stays in the same
	// rank (a2 is still an open policy item) and rises above the newer one.
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
	mustApply(t, st, `<wb:edit topic="older"><wb:old>notes</wb:old><wb:new>more notes</wb:new></wb:edit>`)
	wantTopicOrder(t, st, "older", "newer")
	// The same body again is no activity.
	mustApply(t, st, `<wb:patch id="3"><wb:title>b1 third</wb:title></wb:patch>`)
	mustApply(t, st, topicXML("older", "more notes"))
	wantTopicOrder(t, st, "newer", "older")

	// Rank outranks recency: a quiet topic stays below open ones however
	// recently it changed.
	mustApply(t, st, addXML(ClassTechnical, "quiet", "c1"))
	mustApply(t, st, `<wb:answer id="4" value="ok">fine</wb:answer>`)
	wantTopicOrder(t, st, "newer", "older", "quiet")
}

// A topic with an open body ranks with the expanded topics: below anything
// waiting on the user, above a quiet one.
func TestTopicWithABodyRanksAmongExpandedTopics(t *testing.T) {
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
	wantTopicOrder(t, st, "policy", "quiet", "acks", "notes", "filler")
}

// A body changed after the board's latest user answer ranks like an open
// acknowledgement: below open policy topics, above settled and expanded ones.
// A later answer anywhere on the board drops it back; an assumed answer is
// not the user's, so it does not.
func TestChangedBodyRanksLikeAnOpenAcknowledgement(t *testing.T) {
	st := &State{}
	settleItems(t, st, "settled", 1)
	mustApply(t, st, topicXML("notes", "a body"))
	mustApply(t, st, addXML(ClassPolicy, "policy", "p", "x", "y"))
	// The settled item's answer predates the body: the body is unread.
	wantTopicOrder(t, st, "policy", "notes", "settled")
	if notes := topicView(t, st, LayoutFull, "notes"); notes.Quiet || !notes.DocOpen(false) {
		t.Fatalf("notes = %+v", notes)
	}
	// An assumption is the lead's, not the user's: it adds an assumed item
	// (rank 1, above the body) and leaves the body unread.
	mustApply(t, st, addXML(ClassTechnical, "settled", "again"))
	mustApply(t, st, `<wb:answer id="3" value="ok" assumed="true">assumed</wb:answer>`)
	wantTopicOrder(t, st, "policy", "settled", "notes")
	if st.AnswerSeq == st.Seq {
		t.Fatalf("an assumed answer moved AnswerSeq to %d", st.AnswerSeq)
	}
	// Any user answer after the change counts as having seen the body.
	mustApply(t, st, `<wb:answer id="3" value="ok">fine</wb:answer>`)
	wantTopicOrder(t, st, "policy", "settled", "notes")
	if st.AnswerSeq != st.Seq {
		t.Fatalf("AnswerSeq = %d, want the answering render %d", st.AnswerSeq, st.Seq)
	}
	// Changing the body again after that answer ranks it once more.
	mustApply(t, st, topicXML("notes", "a newer body"))
	if !rankedAbove(t, st, "notes", "settled") {
		t.Fatalf("a body changed after the last answer should outrank a settled topic: %v", topicNames(st))
	}
	// A closed topic's body never ranks.
	mustApply(t, st, `<wb:close topic="notes"/>`)
	if rankedAbove(t, st, "notes", "settled") {
		t.Fatalf("a closed topic's body ranked: %v", topicNames(st))
	}
}

func rankedAbove(t *testing.T, st *State, a, b string) bool {
	t.Helper()
	names := topicNames(st)
	ia, ib := -1, -1
	for i, n := range names {
		switch n {
		case a:
			ia = i
		case b:
			ib = i
		}
	}
	if ia < 0 || ib < 0 {
		t.Fatalf("topics %q, %q not both in %v", a, b, names)
	}
	return ia < ib
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
	mustApply(t, st, `<wb:close topic="notes"/>`)
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
	mustApply(t, st, `<wb:close topic="t"/>`)
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
	mustApply(t, st, `<wb:edit topic="b"><wb:old>body b</wb:old><wb:new>body b2</wb:new></wb:edit>`)
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
	mustApply(t, st, `<wb:close topic="a"/>`)
	if tv := topicView(t, st, LayoutEmbed, "a"); tv.DocOpen(true) || !tv.Closed {
		t.Fatalf("closed topic on the embed = %+v", tv)
	}
}
