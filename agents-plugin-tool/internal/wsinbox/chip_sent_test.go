package wsinbox

import (
	"testing"
	"time"
)

// TestEachChipKeepsItsOwnSendTime pins the per-chip send time a re-asked
// board item's sent mark is judged by: each send's chips carry that send's
// time, in that send's own entry, and a time the caller put on a chip is
// overwritten.
func TestEachChipKeepsItsOwnSendTime(t *testing.T) {
	dir := Dir(t.TempDir())
	t1 := time.UnixMilli(1_000_000)
	t2 := time.UnixMilli(2_000_000)
	if _, _, err := Send(dir, Entry{Chips: []Chip{{ID: "03", Answer: "a", SentAt: 5}, {ID: "04", Answer: "ok"}}}, t1); err != nil {
		t.Fatal(err)
	}
	if _, merged, err := Send(dir, Entry{Chips: []Chip{{ID: "04", Answer: "no"}, {ID: "05", Answer: "ok"}}}, t2); err != nil || merged {
		t.Fatalf("second send: merged=%v err=%v", merged, err)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 2 {
		t.Fatalf("held = %+v, %v", held, err)
	}
	for i, at := range []time.Time{t1, t2} {
		e := held[i].Entry
		if e.SentAt != at.UnixMilli() || len(e.Chips) != 2 {
			t.Fatalf("held[%d] = %+v, want SentAt %d and two chips", i, e, at.UnixMilli())
		}
		for _, c := range e.Chips {
			if c.SentAt != at.UnixMilli() {
				t.Fatalf("held[%d] chip %s SentAt = %d, want %d", i, c.ID, c.SentAt, at.UnixMilli())
			}
		}
	}
}
