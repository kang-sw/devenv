package wslive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestNoticeFollowsTheReplyItEnds pins a notice line: it ends the streamed
// reply like a card, its text is scrubbed and capped to one line, a usage
// limit keeps its reset time, and a kind outside the three is refused
// without a ring entry.
func TestNoticeFollowsTheReplyItEnds(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart}, 100)
	mustApply(t, st, Event{Kind: KindReply, Text: "Working on it"}, 110)
	res, err := st.Notice(Notice{Kind: NoticeUsageLimit, Text: "limit\nreached " + ownerKey, ResetsAt: 9_000}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ring) != 2 || res.Ring[0].Kind != RingReply || res.Ring[1].Kind != RingNotice || !res.Changed {
		t.Fatalf("notice ring: %+v", res)
	}
	n := res.Ring[1].Notice
	if n == nil || n.Kind != NoticeUsageLimit || n.ResetsAt != 9_000 || strings.Contains(n.Text, ownerKey) || strings.Contains(n.Text, "\n") || !strings.HasPrefix(n.Text, "limit reached") {
		t.Fatalf("notice: %+v", n)
	}
	if st.Snapshot(120).Lead.Reply != nil {
		t.Fatal("the notice line should end the streamed reply")
	}
	for _, kind := range []string{NoticeLogin, NoticeModel} {
		if res, err := st.Notice(Notice{Kind: kind, Text: "x"}, 130); err != nil || len(res.Ring) != 1 || res.Ring[0].Notice.Kind != kind {
			t.Fatalf("notice %s: %v %+v", kind, err, res)
		}
	}
	if res, err := st.Notice(Notice{Kind: "overloaded"}, 140); err == nil || len(res.Ring) != 0 {
		t.Fatalf("an unknown kind should be refused, got %+v", res)
	}
}

// TestHostStateInTheSnapshot pins the host state a runner sets: the
// snapshot carries a copy, its texts are scrubbed and its server names
// sorted, an unchanged set reports no change, an adapter's reset keeps it,
// and an empty one clears it from the snapshot's JSON.
func TestHostStateInTheSnapshot(t *testing.T) {
	st := NewState(ownerKey)
	if st.Snapshot(1).Host != nil {
		t.Fatal("a new state has no host state")
	}
	h := &Host{BackgroundTasks: 2, Failure: &Notice{Kind: NoticeLogin, Text: "log in " + ownerKey}, MCPBroken: []string{"plugin:ws:ws", "github"}, Retry: &Retry{Attempt: 2, MaxRetries: 10, Error: "overloaded", Status: 529}}
	if !st.SetHost(h) {
		t.Fatal("a first host state is a change")
	}
	h.MCPBroken[0] = "changed after"
	got := st.Snapshot(2).Host
	want := &Host{BackgroundTasks: 2, Failure: &Notice{Kind: NoticeLogin, Text: "log in " + redactedOwnerKey(t, st)}, MCPBroken: []string{"github", "plugin:ws:ws"}, Retry: &Retry{Attempt: 2, MaxRetries: 10, Error: "overloaded", Status: 529}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot host = %+v, want %+v", got, want)
	}
	got.MCPBroken[0] = "mutated"
	if st.Snapshot(3).Host.MCPBroken[0] != "github" {
		t.Fatal("the snapshot's host state should be a copy")
	}
	if st.SetHost(&Host{BackgroundTasks: 2, Failure: &Notice{Kind: NoticeLogin, Text: "log in " + ownerKey}, MCPBroken: []string{"github", "plugin:ws:ws"}, Retry: &Retry{Attempt: 2, MaxRetries: 10, Error: "overloaded", Status: 529}}) {
		t.Fatal("the same host state is no change")
	}
	mustApply(t, st, Event{Kind: KindReset}, 4)
	if st.Snapshot(4).Host == nil {
		t.Fatal("an adapter's reset keeps the host state")
	}
	if !st.SetHost(&Host{}) || st.Snapshot(5).Host != nil {
		t.Fatal("an empty host state clears it")
	}
	raw, _ := json.Marshal(st.Snapshot(6))
	if strings.Contains(string(raw), `"host"`) {
		t.Fatalf("a snapshot with no host state has no host key: %s", raw)
	}
	if st.SetHost(nil) {
		t.Fatal("nil after empty is no change")
	}
}

// redactedOwnerKey is what the state's scrubber turns the owner key into.
func redactedOwnerKey(t *testing.T, st *State) string {
	t.Helper()
	return st.scrub.scrub(ownerKey)
}

// TestFailedCompactionKeepsItsError: a host that has a failed compaction's
// error text puts it on the compaction line, scrubbed; an ended one has
// none.
func TestFailedCompactionKeepsItsError(t *testing.T) {
	st := NewState(ownerKey)
	res := mustApply(t, st, Event{Kind: KindCompact, Phase: CompactFailed, Text: "Conversation too long " + ownerKey}, 100)
	if len(res.Ring) != 1 || !res.Ring[0].CompactFailed || !strings.HasPrefix(res.Ring[0].Text, "Conversation too long") || strings.Contains(res.Ring[0].Text, ownerKey) {
		t.Fatalf("failed compaction: %+v", res.Ring)
	}
	res = mustApply(t, st, Event{Kind: KindCompact, Phase: CompactEnd, Text: "ignored"}, 200)
	if len(res.Ring) != 1 || res.Ring[0].Text != "" {
		t.Fatalf("ended compaction: %+v", res.Ring)
	}
}
