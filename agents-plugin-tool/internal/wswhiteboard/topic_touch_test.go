package wswhiteboard

import (
	"strings"
	"testing"
	"time"
)

// TestTopicHeaderTimeIsLastTopicTouch pins the topic bar's time (UpdatedAt,
// the "last change" tago, which also orders dashboard cards) to the newest
// touch of any kind: an item add, answer, or patch, a body edit or clear, a
// log line, a panel add or republish, a close, and the log that reopens it.
// Another topic's activity leaves it alone.
func TestTopicHeaderTimeIsLastTopicTouch(t *testing.T) {
	st := &State{}
	base := time.Date(2026, 11, 14, 15, 0, 0, 0, time.UTC)
	step := 0
	touch := func(what, commands string) {
		t.Helper()
		step++
		now := base.Add(time.Duration(step) * time.Minute)
		applyAt(t, st, commands, now)
		if got, want := topicUpdated(t, st, "t"), now.Format(time.RFC3339); got != want {
			t.Fatalf("after %s: header time %q, want %q", what, got, want)
		}
	}
	touch("item add", addXML(ClassCheck, "t", "look"))
	touch("body edit", `<wb:topic name="t"><wb:body>first body</wb:body></wb:topic>`)
	touch("log", `<wb:topic name="t"><wb:log>a log line</wb:log></wb:topic>`)
	touch("panel add", `<wb:topic name="t"><wb:panel recap="a panel">panel body</wb:panel></wb:topic>`)
	panel := ""
	for _, e := range st.Topics[st.topicDocIndex("t")].Entries {
		if e.Kind == EntryPanel {
			panel = e.PanelID
		}
	}
	touch("panel republish", `<wb:topic name="t"><wb:panel id="`+panel+`" recap="the panel, again">panel body v2</wb:panel></wb:topic>`)
	touch("item answer", `<wb:answer id="1" value="ok">ok</wb:answer>`)
	touch("item patch", `<wb:patch id="1" status="open"/>`)
	touch("second item add", addXML(ClassCheck, "t", "another look"))
	touch("close (withdrawing the open item)", `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	touch("reopening log", `<wb:topic name="t"><wb:log>back at it</wb:log></wb:topic>`)
	touch("body clear", `<wb:topic name="t"><wb:body/></wb:topic>`)
	last := topicUpdated(t, st, "t")
	applyAt(t, st, `<wb:topic name="other"><wb:log>elsewhere</wb:log></wb:topic>`, base.Add(time.Hour))
	if got := topicUpdated(t, st, "t"); got != last {
		t.Fatalf("another topic's log moved the header time: %q, want %q", got, last)
	}
}

func topicUpdated(t *testing.T, st *State, name string) string {
	t.Helper()
	v := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for _, tv := range append(v.Topics, v.DeferredTopics...) {
		if tv.Name == name {
			out := render(t, v)
			if !strings.Contains(out, `<time class="tago" datetime="`+tv.UpdatedAt+`" title=`) {
				t.Fatalf("topic bar does not render UpdatedAt %q", tv.UpdatedAt)
			}
			return tv.UpdatedAt
		}
	}
	t.Fatalf("topic %q not in view", name)
	return ""
}
