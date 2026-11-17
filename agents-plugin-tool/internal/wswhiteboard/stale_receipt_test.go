package wswhiteboard

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestStaleReceiptIsInvisibleAndDoesNotRefreshAge(t *testing.T) {
	s := &State{}
	applyAt(t, s, bodies("A", "B", "C", "D", "E"), testNow)
	before := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if out := applyAt(t, s, "", testNow.Add(time.Hour)); nudgeOf(out) == "" {
		t.Fatal("expected initial stale nudge")
	}
	after := BuildView(s, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	if !reflect.DeepEqual(before.Topics, after.Topics) {
		t.Fatal("stale receipt changed projected topic views")
	}
	s.appendEntry("B", Entry{Kind: EntrySystem, Text: "reconciliation"}, testNow.Add(61*time.Minute))
	out := applyAt(t, s, `<wb:topic name="E"><wb:log>progress</wb:log></wb:topic>`, testNow.Add(61*time.Minute))
	if !strings.HasPrefix(nudgeOf(out), "4 of 5 open topics unchanged for 60+ min (A, B, C, D)") {
		t.Fatalf("system events refreshed stale age: %s", nudgeOf(out))
	}
}
