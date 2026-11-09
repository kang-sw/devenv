package wsinbox

import (
	"testing"
	"time"
)

// TestEachChipKeepsItsOwnSendTime pins the per-chip send time a re-asked
// board item's sent mark is judged by: a send merged into a held entry keeps
// the entry's first-send SentAt, but its chips carry their own send's time,
// and a time the caller put on a chip is overwritten.
func TestEachChipKeepsItsOwnSendTime(t *testing.T) {
	dir := Dir(t.TempDir())
	t1 := time.UnixMilli(1_000_000)
	t2 := time.UnixMilli(2_000_000)
	if _, _, err := Send(dir, Entry{Chips: []Chip{{ID: "03", Answer: "a", SentAt: 5}, {ID: "04", Answer: "ok"}}}, t1); err != nil {
		t.Fatal(err)
	}
	if _, merged, err := Send(dir, Entry{Chips: []Chip{{ID: "04", Answer: "no"}, {ID: "05", Answer: "ok"}}}, t2); err != nil || !merged {
		t.Fatalf("second send: merged=%v err=%v", merged, err)
	}
	held, err := Held(dir)
	if err != nil || len(held) != 1 {
		t.Fatalf("held = %+v, %v", held, err)
	}
	e := held[0].Entry
	if e.SentAt != t1.UnixMilli() {
		t.Fatalf("entry SentAt = %d, want the first send's %d", e.SentAt, t1.UnixMilli())
	}
	want := map[string]int64{"03": t1.UnixMilli(), "04": t2.UnixMilli(), "05": t2.UnixMilli()}
	if len(e.Chips) != len(want) {
		t.Fatalf("chips = %+v", e.Chips)
	}
	for _, c := range e.Chips {
		if c.SentAt != want[c.ID] {
			t.Fatalf("chip %s SentAt = %d, want %d (chips %+v)", c.ID, c.SentAt, want[c.ID], e.Chips)
		}
	}
}
