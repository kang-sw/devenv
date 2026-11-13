package wsdashboard

import (
	"reflect"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestSentMarksTakeTheNewestAnswerAndHeldWins(t *testing.T) {
	rec := func(chips ...wsinbox.Chip) wsinbox.Record { return wsinbox.Record{Entry: wsinbox.Entry{Chips: chips}} }
	drained := []wsinbox.Record{
		rec(wsinbox.Chip{ID: "03", Answer: "a"}, wsinbox.Chip{ID: "04", Answer: "ok"}),
		rec(wsinbox.Chip{ID: "03", Answer: "b"}),
	}
	held := []wsinbox.Record{rec(wsinbox.Chip{ID: "04", Answer: "no", Note: "redo"})}
	got := sentMarks(drained, held, nil, 0)
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
	got := sentMarks(drained, held, reasked, 0)
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
	got = sentMarks([]wsinbox.Record{rec(50, chip("09", "ok", 50))}, []wsinbox.Record{rec(60, chip("09", "no", 60))}, map[int]int64{9: 70}, 0)
	if len(got) != 0 {
		t.Fatalf("every answer predates the re-ask: sentMarks = %+v", got)
	}
}

// TestSentMarksClearNoteOnlyChipsAtTurnSettle pins the note-only rule: a
// drained chip with a note and no value, sent before the newest settled
// turn's start, was delivered to a turn that finished, so it marks nothing
// and leaves no older value chip's mark behind. Every other chip keeps its
// mark.
func TestSentMarksClearNoteOnlyChipsAtTurnSettle(t *testing.T) {
	rec := func(chips ...wsinbox.Chip) wsinbox.Record { return wsinbox.Record{Entry: wsinbox.Entry{Chips: chips}} }
	note := func(id string, at int64) wsinbox.Chip { return wsinbox.Chip{ID: id, Note: "n" + id, SentAt: at} }
	value := func(id string, at int64) wsinbox.Chip { return wsinbox.Chip{ID: id, Answer: "ok", SentAt: at} }
	const settled = 1000
	cases := []struct {
		name          string
		drained, held []wsinbox.Record
		settled       int64
		want          []SentMark
	}{
		{"note before the settled start", []wsinbox.Record{rec(note("01", 900))}, nil, settled, []SentMark{}},
		{"note after the settled start", []wsinbox.Record{rec(note("01", 1100))}, nil, settled, []SentMark{{ID: "01", Note: "n01"}}},
		{"held note", nil, []wsinbox.Record{rec(note("01", 900))}, settled, []SentMark{{ID: "01", Note: "n01", Held: true}}},
		{"value before the settled start", []wsinbox.Record{rec(value("01", 900))}, nil, settled, []SentMark{{ID: "01", Answer: "ok"}}},
		{"cleared note over an older value", []wsinbox.Record{rec(value("01", 800)), rec(note("01", 900))}, nil, settled, []SentMark{}},
		{"note without a send time", []wsinbox.Record{rec(note("01", 0))}, nil, settled, []SentMark{{ID: "01", Note: "n01"}}},
		{"no settled turn", []wsinbox.Record{rec(note("01", 900))}, nil, 0, []SentMark{{ID: "01", Note: "n01"}}},
		{"a held answer after a cleared note", []wsinbox.Record{rec(note("01", 900))}, []wsinbox.Record{rec(value("01", 1100))}, settled, []SentMark{{ID: "01", Answer: "ok", Held: true}}},
	}
	for _, c := range cases {
		if got := sentMarks(c.drained, c.held, nil, c.settled); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: sentMarks = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestSettledTurnStartReadsTheNewestTurn pins the settled-turn rule: the
// newest turn record's time less its duration; no turn, or a newest turn
// with no duration, is no settled start (an older turn does not stand in).
func TestSettledTurnStartReadsTheNewestTurn(t *testing.T) {
	at := func(ms int64) time.Time { return time.UnixMilli(ms) }
	turn := func(ms, dur int64) wslive.RingRecord {
		return wslive.RingRecord{RingEntry: wslive.RingEntry{Kind: wslive.RingTurn, DurationMs: dur}, At: at(ms)}
	}
	user := wslive.RingRecord{RingEntry: wslive.RingEntry{Kind: wslive.RingUser, Text: "x"}, At: at(5000)}
	cases := []struct {
		name string
		ring []wslive.RingRecord
		want int64
	}{
		{"no ring", nil, 0},
		{"no turn", []wslive.RingRecord{user}, 0},
		{"newest turn", []wslive.RingRecord{turn(2000, 500), turn(4000, 1500), user}, 2500},
		{"newest turn has no duration", []wslive.RingRecord{turn(2000, 500), turn(4000, 0)}, 0},
	}
	for _, c := range cases {
		if got := settledTurnStart(c.ring); got != c.want {
			t.Errorf("%s: settledTurnStart = %d, want %d", c.name, got, c.want)
		}
	}
}
