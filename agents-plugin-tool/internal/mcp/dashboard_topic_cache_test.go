package mcp

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func topicRenderCounts(src *DashboardSessionSource) map[string]int {
	src.boards.mu.Lock()
	defer src.boards.mu.Unlock()
	return maps.Clone(src.boards.topicRenders)
}

func TestDashboardBoardTopicCacheRetainsImageWatch(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	src.boards.topicRenders = map[string]int{}
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	mustWrite(t, f.root, "shots/a.png", "image-one")
	f.update(t, `<wb:topic name="A"><wb:body>![shot](shots/a.png)</wb:body></wb:topic><wb:topic name="B"><wb:body>other</wb:body></wb:topic>`)
	for i := 0; i < 3; i++ {
		readPageOK(t, src, reg)
	}
	before := topicRenderCounts(src)
	f.update(t, `<wb:topic name="B"><wb:body>edited other</wb:body></wb:topic>`)
	page := readPageOK(t, src, reg)
	if topicRenderCounts(src)["A"] != before["A"] {
		t.Fatal("unrelated body edit rendered image topic")
	}
	path := filepath.Join(f.root, "shots", "a.png")
	if !slices.Contains(page.Watch, path) {
		t.Fatal("cache hit lost image watch")
	}
	version := regexp.MustCompile(`shots%2Fa\.png&(?:amp;)*v=([0-9a-z.]+)`)
	old := version.FindStringSubmatch(string(page.Board.Board))
	if old == nil {
		t.Fatalf("the image relay carries no version:\n%.1200s", page.Board.Board)
	}
	mustWrite(t, f.root, "shots/a.png", "replacement-image-two")
	at := time.Now().Add(time.Second)
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
	// The external image edit sends no signal; its watch stamp may be
	// reused for one second, but must then invalidate the retained topic.
	waitForDashboardValidation(t)
	// The relayed image's version changes, so its frame remounts.
	page = readPageOK(t, src, reg)
	if next := version.FindStringSubmatch(string(page.Board.Board)); next == nil || next[1] == old[1] {
		t.Fatal("image edit was hidden by topic cache")
	}
}

func TestDashboardBoardTopicCache(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	src.boards.topicRenders = map[string]int{}
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	if err := whiteboardFolder(f.root, f.key).Update(nil, func(s *wswhiteboard.State) error {
		s.Seq = 1
		for i := 0; i < 112; i++ {
			s.Topics = append(s.Topics, wswhiteboard.TopicDoc{Name: fmt.Sprintf("topic-%03d", i), Body: "Initial body", Changed: 1, ChangedAt: "2026-11-15T01:00:00Z"})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	settle := func() wsdashboard.PageContent {
		readPageOK(t, src, reg)
		readPageOK(t, src, reg)
		return readPageOK(t, src, reg)
	}
	first := settle()
	before := topicRenderCounts(src)
	// A non-board live write must preserve the source's immutable units too.
	dir, _ := src.stateDir()
	path := filepath.Join(wsdashboard.SessionDir(dir, reg.ReplyID), "snapshot.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"updatedAt":123}`), 0600); err != nil {
		t.Fatal(err)
	}
	page := readPageOK(t, src, reg)
	if !reflect.DeepEqual(before, topicRenderCounts(src)) || first.BoardUnits != page.BoardUnits {
		t.Fatal("non-board change rendered topics or replaced units")
	}
	change := func(commands string, want ...string) {
		t.Helper()
		before := topicRenderCounts(src)
		f.update(t, commands)
		page = readPageOK(t, src, reg)
		expected := map[string]bool{}
		for _, id := range want {
			expected[id] = true
		}
		after := topicRenderCounts(src)
		for id, count := range after {
			delta := count - before[id]
			if delta != map[bool]int{true: 1}[expected[id]] {
				t.Fatalf("%s rendered %d times, wanted changed topics %v", id, delta, want)
			}
		}
		_, _, rec, ok, err := src.lookupRecord(reg)
		if !ok || err != nil {
			t.Fatal("record", err)
		}
		full, ok := src.renderBoardNow(f.key, rec, reg.ReplyID, "")
		if !ok {
			t.Fatal("full render")
		}
		if page.Board != full.regions {
			t.Fatal("cached render differs from uncached full render")
		}
	}
	change(`<wb:topic name="topic-000"><wb:body>Small edit</wb:body></wb:topic>`, "topic-000")
	change(`<wb:topic name="topic-000"><wb:state value="waiting" on="topic:topic-001"/></wb:topic>`, "topic-000", "topic-001")
	// A sequence-only DB write changes no topic output.
	before = topicRenderCounts(src)
	if err := f.s.sessions.boards.use(f.root, f.key, nil, true, func(slot *whiteboardSlot) error {
		return slot.board.Update(func(s *wswhiteboard.State) error { s.Seq++; return nil })
	}); err != nil {
		t.Fatal(err)
	}
	readPageOK(t, src, reg)
	if !reflect.DeepEqual(before, topicRenderCounts(src)) {
		t.Fatal("sequence-only DB change invalidated unrelated topics")
	}
	// Shared watched inputs conservatively invalidate every topic.
	settle()
	before = topicRenderCounts(src)
	writeTicket(t, f.root, "ready", "261101-feat-topic-cache", "A ticket")
	at := time.Now().Add(time.Second)
	os.Chtimes(filepath.Join(f.root, "ai-docs", "tickets", "ready"), at, at)
	waitForDashboardValidation(t) // external shared watch, no Index signal
	readPageOK(t, src, reg)
	for id, count := range topicRenderCounts(src) {
		if count-before[id] != 1 {
			t.Fatalf("shared watched input did not invalidate %s", id)
		}
	}
}
