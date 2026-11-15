package wsdashboard

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEntryAndPanelStreamWaitsPastQuietWindow(t *testing.T) {
	for _, name := range []string{regionEntries, regionPanels} {
		t.Run(name, func(t *testing.T) {
			events := make(chan sseEvent, 2)
			events <- sseEvent{name: "assets", data: `{}`}
			want := sseEvent{name: "region", data: `{"name":"` + name + `","hash":"late","html":"full"}`}
			// Deliberately arrive after collect's quiet window. The helper must
			// wait for this event rather than mistaking silence for completion.
			timer := time.AfterFunc(2*settle, func() { events <- want })
			defer timer.Stop()
			var got sseEvent
			if name == regionEntries {
				got = entryStreamEvent(t, events)
			} else {
				got = panelStreamEvent(t, events)
			}
			if got != want {
				t.Fatalf("late event=%+v want=%+v", got, want)
			}
		})
	}
}

// Arrival is bounded by the test deadline, not the quiet window. Only after
// the first relevant event arrives do we use silence to check for duplicates.
func keyedStreamEvent(t *testing.T, events <-chan sseEvent, region, delta string) sseEvent {
	t.Helper()
	matches := func(e sseEvent) bool {
		if e.name == delta {
			return true
		}
		if e.name == "region" {
			var r regionEvent
			if err := json.Unmarshal([]byte(e.data), &r); err != nil {
				t.Fatal(err)
			}
			return r.Name == region
		}
		return false
	}
	deadline, ok := t.Deadline()
	if !ok {
		deadline = time.Now().Add(10 * time.Second)
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	var all []sseEvent
arrival:
	for {
		select {
		case e, open := <-events:
			if !open {
				t.Fatalf("stream closed before %s event: %+v", region, all)
			}
			all = append(all, e)
			if matches(e) {
				break arrival
			}
		case <-timer.C:
			t.Fatalf("deadline waiting for %s event: %+v", region, all)
		}
	}
	more, closed := collect(events)
	all = append(all, more...)
	var found []sseEvent
	for _, e := range all {
		if matches(e) {
			found = append(found, e)
		}
	}
	if closed || len(found) != 1 {
		t.Fatalf("closed=%v %s events=%+v all=%+v", closed, region, found, all)
	}
	return found[0]
}
