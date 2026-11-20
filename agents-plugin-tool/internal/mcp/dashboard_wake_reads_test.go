package mcp

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

// TestDashboardStampReadsFollowTheMemoryStore pins the mail print and the
// contacts over the memory-first mailbox store: unchanged state prints the
// same, a store write moves the print at once (no file to revalidate), and
// the source clock still expires a contact's presence.
func TestDashboardStampReadsFollowTheMemoryStore(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	hf := newHistoryFixture(t, now)
	mustSend(t, hf.serverB, 50, hf.keyB, "alice@worktree", "hello")
	rec, ok := hf.serverB.sessions.readState(hf.keyB, partAll)
	if !ok {
		t.Fatal("missing record")
	}
	reg := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(hf.keyB), Project: rec.Root}
	src := dashboardPageSource(t, &now)
	first, _ := src.MailPrint(reg)
	contacts, _ := src.ReadContacts(reg)
	if len(contacts) != 1 {
		t.Fatal("contact fixture missing")
	}
	for i := 0; i < 5; i++ {
		if next, _ := src.MailPrint(reg); next != first {
			t.Fatal("unchanged mail print moved")
		}
	}
	if err := (wsmailbox.Paths{}).Store().WithReplyLock(func(s *wsmailbox.ReplyStore) error {
		s.Queues[reg.ReplyID] = append(s.Queues[reg.ReplyID], wsmailbox.Envelope{From: "alice@worktree", Content: "new"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if next, _ := src.MailPrint(reg); next == first {
		t.Fatal("a store write did not move the mail print")
	}
	now = now.Add(11 * time.Minute)
	if next, _ := src.ReadContacts(reg); len(next) != 1 || next[0].PresenceLive {
		t.Fatal("contact ignored expiry")
	}
}

// The actual MCP update/render transaction must not alias the frozen board
// simultaneously consumed by dashboard paths. No dashboard daemon is started.
func TestDashboardFolderSharedReadsDuringUpdateAndRender(t *testing.T) {
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="topic"><wb:body>original body</wb:body></wb:topic>`)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	_, _, record, ok, err := src.lookupRecord(reg)
	if !ok || err != nil {
		t.Fatalf("lookup: %v", err)
	}
	frozen := record.Whiteboard
	before, _ := json.Marshal(frozen)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 6; n++ {
				_, _ = json.Marshal(frozen)
				view := wswhiteboard.BuildView(frozen, wswhiteboard.ViewInput{Layout: wswhiteboard.LayoutFull})
				if _, err := wswhiteboard.RenderRegions(view); err != nil {
					t.Error(err)
				}
				if _, ok, err := src.ReadPage(reg); !ok || err != nil {
					t.Errorf("page: %v", err)
				}
			}
		}()
	}
	for n := 0; n < 4; n++ {
		f.update(t, `<wb:topic name="topic"><wb:body>committed update</wb:body><wb:log>update history</wb:log></wb:topic>`)
		// Panel writes refresh the served view without mutating the frozen board.
		f.call(t, "agenda.set", map[string]any{"key": "refresh", "value": map[string]any{"round": n}})
	}
	wg.Wait()
	after, _ := json.Marshal(frozen)
	if string(before) != string(after) {
		t.Fatal("MCP transaction mutated a cached dashboard generation")
	}
	// The latest batch's log line is live on the board; the body version
	// committed earlier sits in lazy history the page fetches on expand.
	frags, _ := src.ReadTopicHistory(reg, "", []string{"topic"}, "")
	if !strings.Contains(string(readPageOK(t, src, reg).Board.Board), "update history") || len(frags) != 1 || !strings.Contains(string(frags[0].HTML), "committed update") {
		t.Fatal("dashboard missed committed mutation")
	}
}
