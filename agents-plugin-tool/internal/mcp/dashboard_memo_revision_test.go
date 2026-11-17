package mcp

import (
	"encoding/json"
	"reflect"
	"strconv"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestDashboardBoardMemoFolderRevision(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	f.update(t, `<wb:topic name="topic"><wb:body>initial body</wb:body></wb:topic>`)
	folder := whiteboardFolder(f.root, f.key)
	read := func() sessionRecord {
		t.Helper()
		_, _, rec, ok, err := src.lookupRecord(reg)
		if err != nil || !ok || rec.Whiteboard == nil || rec.boardRevision == 0 {
			t.Fatalf("lookup must carry folder read identity: ok=%t revision=%d err=%v", ok, rec.boardRevision, err)
		}
		return rec
	}
	previous := read()
	seq := previous.Whiteboard.Seq
	base := func(rec sessionRecord) string {
		return boardMemoBase(f.key, rec, "en", nil, "")
	}
	// Compare memoized and full rendering while also proving that stable
	// immutable reads actually hit, not merely return equivalent fresh output.
	checkRender := func(rec sessionRecord) {
		t.Helper()
		for i := 0; i < 2; i++ {
			if _, ok := src.renderBoard(f.key, rec, reg.ReplyID, "en", nil); !ok {
				t.Fatal("render failed")
			}
		}
		before := boardRenders(src)
		cached, ok := src.renderBoard(f.key, rec, reg.ReplyID, "en", nil)
		if !ok || boardRenders(src) != before {
			t.Fatal("unchanged folder read did not reuse memo")
		}
		full, ok := src.renderBoardNow(f.key, rec, reg.ReplyID, "en")
		if !ok || cached.regions != full.regions || !reflect.DeepEqual(cached.units, full.units) {
			t.Fatal("memoized render differs from full render")
		}
	}
	checkRender(previous)
	for _, tc := range []struct {
		name   string
		change func(*wswhiteboard.State)
	}{
		{"metadata", func(s *wswhiteboard.State) { s.Labels = map[string]string{"topic": "new label"} }},
		{"log", func(s *wswhiteboard.State) {
			s.EntrySeq++
			s.Topics[0].Entries = append(s.Topics[0].Entries, wswhiteboard.Entry{ID: "e" + strconv.Itoa(s.EntrySeq), Kind: wswhiteboard.EntryAgent, Text: "new history", At: "2026-11-15T00:00:00Z"})
		}},
		{"panel", func(s *wswhiteboard.State) {
			s.EntrySeq++
			s.PanelCounter++
			s.Topics[0].Entries = append(s.Topics[0].Entries, wswhiteboard.Entry{ID: "e" + strconv.Itoa(s.EntrySeq), Kind: wswhiteboard.EntryPanel, PanelID: "p" + strconv.Itoa(s.PanelCounter), Text: "new panel", At: "2026-11-15T00:00:00Z"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := folder.Update(nil, func(s *wswhiteboard.State) error { tc.change(s); return nil }); err != nil {
				t.Fatal(err)
			}
			rec := read()
			if rec.Whiteboard.Seq != seq || rec.boardRevision <= previous.boardRevision || base(rec) == base(previous) {
				t.Fatal("changed authority at unchanged Seq did not invalidate memo base")
			}
			before := boardRenders(src)
			if _, ok := src.renderBoard(f.key, rec, reg.ReplyID, "en", nil); !ok || boardRenders(src) == before {
				t.Fatal("changed authority reused old board render")
			}
			checkRender(rec)
			previous = rec
		})
	}
	// A forgotten or independent cache read cannot collide with a held memo.
	wswhiteboard.SharedFolderCache.Forget(folder.Dir)
	reopened := read()
	if base(reopened) == base(previous) {
		t.Fatal("Forget reused a held memo identity")
	}
	other := dashboardPageSource(t, nil)
	_, _, independent, ok, err := other.lookupRecord(reg)
	if err != nil || !ok || base(independent) == base(reopened) {
		t.Fatalf("independent cache collided with held memo: %v", err)
	}
	// The unexported identity must never enter session persistence or raw JSON.
	raw, err := json.Marshal(reopened)
	if err != nil {
		t.Fatal(err)
	}
	reopened.boardRevision++
	changed, _ := json.Marshal(reopened)
	if string(raw) != string(changed) {
		t.Fatal("process-local board revision leaked into JSON")
	}
}
