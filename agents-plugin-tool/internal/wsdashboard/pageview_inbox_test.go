package wsdashboard

import (
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

func TestSentMarksTakeTheNewestAnswerAndHeldWins(t *testing.T) {
	rec := func(chips ...wsinbox.Chip) wsinbox.Record { return wsinbox.Record{Entry: wsinbox.Entry{Chips: chips}} }
	drained := []wsinbox.Record{
		rec(wsinbox.Chip{ID: "03", Answer: "a"}, wsinbox.Chip{ID: "04", Answer: "ok"}),
		rec(wsinbox.Chip{ID: "03", Answer: "b"}),
	}
	held := []wsinbox.Record{rec(wsinbox.Chip{ID: "04", Answer: "no", Note: "redo"})}
	got := sentMarks(drained, held)
	want := []SentMark{{ID: "03", Answer: "b"}, {ID: "04", Answer: "no", Note: "redo", Held: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sentMarks = %+v, want %+v", got, want)
	}
}

func TestBoardFirstViewReadsNoInbox(t *testing.T) {
	v := buildSessionView(pageInputs{Held: []wsinbox.Record{{Name: "x.json", Entry: wsinbox.Entry{Parts: []string{"x"}}}}})
	if v.Adapter || len(v.Conversation.Held) != 0 || len(v.Conversation.Sent) != 0 {
		t.Fatalf("a session without an adapter shows inbox state: %+v", v.Conversation)
	}
}
