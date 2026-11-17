package mcp

import (
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func TestDashboardRunnerReadSkipsNonLiveStoresButNudgeRefreshesThem(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="t"><wb:add class="technical"><wb:title>question</wb:title></wb:add></wb:topic>`)
	dir := t.TempDir()
	now := time.Now().UTC()
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root,
		RegisteredAt: now.Format(time.RFC3339Nano), HeartbeatAt: now.Format(time.RFC3339Nano)}
	if err := wsdashboard.WriteRegistration(dir, reg); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	src := &DashboardSessionSource{store: f.s.sessions, stateDir: func() (string, error) { return dir, nil },
		nonLiveRead: func(part string) { counts[part]++ }}
	src.SetLiveSnapshot(dir, reg.ReplyID, wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000})
	idx := wsdashboard.NewIndex(wsdashboard.IndexOptions{Dir: dir, Source: src})
	idx.Refresh()
	before, ok := idx.Session(reg.ReplyID)
	if !ok {
		t.Fatal("missing initial session")
	}
	assertCounts := func(want int) {
		t.Helper()
		for _, part := range []string{"record", "board", "tickets", "mailbox-history"} {
			if counts[part] != want {
				t.Fatalf("%s refresh groups = %d, want %d", part, counts[part], want)
			}
		}
	}
	assertCounts(1)
	clear(counts)
	// Change both halves. The live write must update the snapshot while
	// leaving the new record/board/ticket/history evidence for a full nudge.
	f.update(t, `<wb:topic name="t"><wb:add class="check"><wb:title>second question</wb:title></wb:add></wb:topic>`)
	writeTicket(t, f.root, "todo", "261117-feat-live-test", "Ticket changed")
	if _, err := appendMailboxHistory("", f.key, []mailboxHistoryEntry{{Direction: mailboxHistorySent, Counterparty: "id:peer", Body: "new mail"}}); err != nil {
		t.Fatal(err)
	}
	src.SetLiveSnapshot(dir, reg.ReplyID, wslive.Snapshot{SchemaVersion: 1, Adapter: "pi", UpdatedAt: now.UnixMilli(), HeartbeatMs: 30000,
		Lead: wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseReplying}, Reply: &wslive.Reply{Text: "runner output"}}})
	_, misses := src.records.Counts()
	idx.RereadLive(reg.ReplyID)
	idx.Settle()
	assertCounts(0)
	if _, after := src.records.Counts(); after != misses {
		t.Fatalf("live write reopened %d record files", after-misses)
	}
	live, _ := idx.Session(reg.ReplyID)
	if live.Rev != before.Rev+1 || live.CountPolicy != before.CountPolicy {
		t.Fatalf("live read did not preserve board evidence: before=%+v after=%+v", before, live)
	}
	if !idx.Nudge(reg.ReplyID, reg.Project) {
		t.Fatal("nudge refused")
	}
	idx.Settle()
	assertCounts(1)
	full, _ := idx.Session(reg.ReplyID)
	if full.Rev != live.Rev+1 || full.CountPolicy != before.CountPolicy+1 {
		t.Fatalf("nudge did not apply pending board change: %+v", full)
	}
	idx.Reread(reg.ReplyID) // full board poke
	idx.Settle()
	assertCounts(2)
	idx.Refresh()
	assertCounts(3)
	t.Log("per warm runner read: record/board/ticket/history refresh groups 1/1/1/1 -> 0/0/0/0; Nudge, board poke and Refresh retain 1/1/1/1")
}
