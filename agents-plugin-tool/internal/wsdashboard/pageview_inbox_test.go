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
	got := sentMarks(drained, held, nil)
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

// TestSentMarksDropAnswersSentBeforeTheItemsReask pins the re-ask rule: a
// chip, drained or held, sent before its item's latest re-ask marks nothing,
// so the item waits on the user again; one sent at the re-ask's millisecond
// or later marks it, even merged into a held record created before the
// re-ask (the chip's own time counts, not the record's). A chip with no send
// time, and an item never re-asked, keep their marks.
func TestSentMarksDropAnswersSentBeforeTheItemsReask(t *testing.T) {
	rec := func(sentAt int64, chips ...wsinbox.Chip) wsinbox.Record {
		return wsinbox.Record{Entry: wsinbox.Entry{SentAt: sentAt, Chips: chips}}
	}
	chip := func(id, answer string, at int64) wsinbox.Chip {
		return wsinbox.Chip{ID: id, Answer: answer, SentAt: at}
	}
	drained := []wsinbox.Record{
		rec(100, chip("03", "ok", 100), chip("04", "ok", 100), chip("05", "ok", 100), wsinbox.Chip{ID: "06", Answer: "ok"}),
	}
	held := []wsinbox.Record{
		// Created before the re-asks; 07's stale answer, then 08's answer
		// merged in after its re-ask.
		rec(150, chip("07", "a", 150), chip("08", "b", 300)),
	}
	reasked := map[int]int64{3: 200, 4: 100, 6: 200, 7: 200, 8: 200}
	got := sentMarks(drained, held, reasked)
	want := []SentMark{
		{ID: "04", Answer: "ok"}, // sent at the re-ask's millisecond
		{ID: "05", Answer: "ok"}, // never re-asked
		{ID: "06", Answer: "ok"}, // no send time: today's behavior
		{ID: "08", Answer: "b", Held: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sentMarks = %+v, want %+v", got, want)
	}
	// A stale held chip leaves the earlier delivered mark dropped too, not
	// revived: the drained chip is older still.
	got = sentMarks([]wsinbox.Record{rec(50, chip("09", "ok", 50))}, []wsinbox.Record{rec(60, chip("09", "no", 60))}, map[int]int64{9: 70})
	if len(got) != 0 {
		t.Fatalf("every answer predates the re-ask: sentMarks = %+v", got)
	}
}
