// Tracker batch tests.
package mcp

import (
	"context"
	"strings"
	"testing"
)

// One flagged command refuses the whole batch with its command position and
// line; the commands before it write nothing.
func TestTrackerUpdateIsAllOrNothing(t *testing.T) {
	f := newTrackerLeadFixture(t)
	f.trackerUpdate(t, trackerPersonal)
	h, err := f.s.trackerHost()
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	beforeEvents := len(before.Events)
	text := f.trackerRefused(t, "tracker.update", map[string]any{"commands": `<tk:store name="me"><tk:item.create kind="task" title="kept out"/>

<tk:item.hide item="$1"/>
<tk:item.hide item="$1"/>
</tk:store>`})
	if !strings.Contains(text, "command 3 <tk:item.hide> (line 4): the store would flag this write:") || !strings.Contains(text, "nothing was applied") {
		t.Fatalf("the refusal must cite the flagged command and its line:\n%s", text)
	}
	// Under any status: a partial write of command 1 alone leaves a visible
	// item, of commands 1 and 2 a hidden one.
	for _, status := range []string{"", "hidden"} {
		payload := map[string]any{"store": "me", "scope": "all"}
		if status != "" {
			payload["status"] = status
		}
		if out := f.trackerQuery(t, payload); strings.Contains(out, "kept out") {
			t.Fatalf("a refused batch must write nothing (status %q):\n%s", status, out)
		}
	}
	after, err := h.View(context.Background(), "me", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Items) != 0 || len(after.Events) != beforeEvents {
		t.Fatalf("a refused batch must write nothing: %d items, %d events (was %d)", len(after.Items), len(after.Events), beforeEvents)
	}
}

func TestParseTrackerID(t *testing.T) {
	for in, want := range map[string]string{
		"maya.pc-5":       "maya.pc#5",
		"maya.pc#5":       "maya.pc#5",
		"kim-ho.my-pc-12": "kim-ho.my-pc#12",
	} {
		if got, err := parseTrackerID(in); err != nil || got != want {
			t.Fatalf("parseTrackerID(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"maya", "maya-5", "maya.pc-0", "maya.pc#x", "maya.pc#5!c:1"} {
		if _, err := parseTrackerID(bad); err == nil {
			t.Fatalf("parseTrackerID(%q) must fail", bad)
		}
	}
}
