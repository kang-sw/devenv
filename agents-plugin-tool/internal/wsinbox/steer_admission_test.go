package wsinbox

import (
	"errors"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestSteerAdmissionReusesEntryRead(t *testing.T) {
	for _, reject := range []bool{false, true} {
		dir := Dir(t.TempDir())
		name, _, err := Send(dir, Entry{Parts: []string{"held"}, Chips: []Chip{{ID: "01", Answer: "ok"}}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		refused := errors.New("refused")
		calls := 0
		before := wsstate.FileIOCounters()["file_io_read_file"]
		_, err = SteerWithAdmission(dir, name, func(entry Entry) error {
			calls++
			if entry.Steer || len(entry.Chips) != 1 || entry.Chips[0].Answer != "ok" {
				t.Fatalf("admission did not receive original entry: %+v", entry)
			}
			if reject {
				return refused
			}
			return nil
		})
		if got := wsstate.FileIOCounters()["file_io_read_file"] - before; got != 1 {
			t.Fatalf("steer admission did %d file reads, want the existing single entry read", got)
		}
		if calls != 1 || reject != errors.Is(err, refused) || (!reject && err != nil) {
			t.Fatalf("calls=%d, reject=%v, err=%v", calls, reject, err)
		}
		held, err := Held(dir)
		if err != nil || len(held) != 1 || held[0].Entry.Steer == reject {
			t.Fatalf("steer state after admission: %+v, %v", held, err)
		}
		if reject && held[0].Name != name {
			t.Fatal("refusal failed to restore original held name")
		}
	}
}
