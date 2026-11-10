package wslive

import (
	"reflect"
	"strings"
	"testing"
)

// TestUIFollowsTheReplyItEnds pins a ui line's place and checks: it ends
// the streamed reply like a card, a search query becomes one scrubbed line,
// and every op or argument outside the tracker view's offer is refused
// without a ring entry.
func TestUIFollowsTheReplyItEnds(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart}, 100)
	mustApply(t, st, Event{Kind: KindReply, Text: "Here they are:"}, 110)
	res, err := st.UI(UICommand{Op: UISearch, Arg: "sync\n" + ownerKey + " ribbon"}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ring) != 2 || res.Ring[0].Kind != RingReply || res.Ring[1].Kind != RingUI || !res.Changed || !res.Urgent {
		t.Fatalf("ui ring: %+v", res)
	}
	if c := res.Ring[1].UI; c == nil || c.Op != UISearch || strings.Contains(c.Arg, ownerKey) || strings.Contains(c.Arg, "\n") || !strings.HasPrefix(c.Arg, "sync ") {
		t.Fatalf("ui command: %+v", res.Ring[1].UI)
	}
	if st.Snapshot(120).Lead.Reply != nil {
		t.Fatal("the ui line should end the streamed reply")
	}
	for _, good := range []UICommand{
		{Op: UIFocus, Arg: "ada.c89-3"},
		{Op: UIFocus, Arg: "ada.c89-3", Comment: "ada.c89-7"},
		{Op: UIScope, Arg: "team"},
		{Op: UIView, Arg: "kanban"},
	} {
		res, err := st.UI(good, 130)
		if err != nil || len(res.Ring) != 1 || !reflect.DeepEqual(*res.Ring[0].UI, good) {
			t.Fatalf("ui %+v: %v %+v", good, err, res)
		}
	}
	for _, bad := range []UICommand{
		{Op: "press", Arg: "accept"},
		{Op: "", Arg: "x"},
		{Op: UISearch, Arg: "  \n "},
		{Op: UISearch, Arg: "x", Comment: "ada.c89-7"},
		{Op: UIFocus, Arg: "<b>"},
		{Op: UIFocus, Arg: "ada.c89-3", Comment: "-x"},
		{Op: UIScope, Arg: "everyone"},
		{Op: UIView, Arg: "tree"},
		{Op: UIView, Arg: "list", Comment: "ada.c89-7"},
	} {
		if res, err := st.UI(bad, 140); err == nil || len(res.Ring) != 0 {
			t.Fatalf("ui %+v: want a refusal, got %+v", bad, res)
		}
	}
}

// TestUIRingReplaysUnchanged pins the ring as the command's carrier: a ui
// line read back (a page's reload or reconnect replay) is the command as
// written, with nothing marking it consumed.
func TestUIRingReplaysUnchanged(t *testing.T) {
	dir := RingDir(t.TempDir())
	st := NewState(ownerKey)
	res, err := st.UI(UICommand{Op: UIFocus, Arg: "ada.c89-3", Comment: "ada.c89-7"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := AppendRing(dir, res.Ring); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		ring, err := ReadRing(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(ring) != 1 || ring[0].Kind != RingUI || ring[0].UI == nil || *ring[0].UI != (UICommand{Op: UIFocus, Arg: "ada.c89-3", Comment: "ada.c89-7"}) {
			t.Fatalf("replayed ring: %+v", ring)
		}
	}
}
