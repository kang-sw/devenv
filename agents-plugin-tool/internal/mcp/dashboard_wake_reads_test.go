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

func TestDashboardStampReadsReuseFreshContactFiles(t *testing.T) {
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
	_, replies := src.replies.Counts()
	_, scopes := src.scopes.Counts()
	for i := 0; i < 20; i++ {
		if next, _ := src.MailPrint(reg); next != first {
			t.Fatal("unchanged mail print moved")
		}
		src.ReadContacts(reg)
	}
	if _, n := src.replies.Counts(); n != replies {
		t.Fatalf("unchanged reply registry reopened %d times", n-replies)
	}
	if _, n := src.scopes.Counts(); n != scopes {
		t.Fatalf("unchanged scope store reopened %d times", n-scopes)
	}
	path, err := wsmailbox.ReplyRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := wsmailbox.WithReplyLock(path, func(s *wsmailbox.ReplyStore) error {
		s.Queues[reg.ReplyID] = append(s.Queues[reg.ReplyID], wsmailbox.Envelope{From: "alice@worktree", Content: "new"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// This raw registry write bypasses the Index signal path. The fixed
	// source clock controls contact expiry, not the cache's one-second TTL.
	waitForDashboardValidation(t)
	if next, _ := src.MailPrint(reg); next == first {
		t.Fatal("mail write did not invalidate cache within one second plus margin")
	}
	if _, n := src.replies.Counts(); n != replies+1 {
		t.Fatal("changed registry did not read exactly once")
	}
	now = now.Add(11 * time.Minute)
	if next, _ := src.ReadContacts(reg); len(next) != 1 || next[0].PresenceLive {
		t.Fatal("cached contact ignored expiry")
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
		// The implicit render caller also runs BeginRender inside its uncached
		// folder transaction, not on the state retained by DashboardSessionSource.
		f.s.refreshWhiteboardAfter(map[string]any{"session_key": f.key}, response{})
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
