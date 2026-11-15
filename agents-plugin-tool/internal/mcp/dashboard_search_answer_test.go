package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestDashboardSearchAnswerSnapshotRetainedHistory(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="open"><wb:body>current open topic</wb:body><wb:add class="technical"><wb:title>retained question</wb:title></wb:add><wb:log>immutable old log</wb:log><wb:panel recap="old panel">old panel payload</wb:panel><wb:panel id="p1" recap="new panel">new panel payload</wb:panel></wb:topic>`)
	f.update(t, `<wb:topic name="closed"><wb:body>closed topic body</wb:body><wb:log>closed immutable log</wb:log><wb:panel recap="closed panel">closed panel payload</wb:panel><wb:close result="retained close result"/></wb:topic>`)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	root, data, err := src.SearchAnswerSnapshot(reg)
	if err != nil || root != f.root {
		t.Fatalf("snapshot root: %s %v", root, err)
	}
	var board wswhiteboard.State
	if err := json.Unmarshal(data, &board); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"current open topic", "retained question", "immutable old log", "old panel payload", "new panel payload", "closed topic body", "closed immutable log", "closed panel payload", "retained close result"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("snapshot missing %s: %s", want, data)
		}
	}
	if len(board.ClosedTopics) != 1 || board.ClosedTopics[0] != "closed" || len(board.Topics[0].Entries) != 3 {
		t.Fatalf("incomplete history: %+v", board)
	}
	if strings.Contains(string(data), f.key) {
		t.Fatal("session credential leaked into snapshot")
	}
	// Later writes do not mutate an already materialized question snapshot.
	f.update(t, `<wb:topic name="open"><wb:log>later log</wb:log></wb:topic>`)
	if strings.Contains(string(data), "later log") {
		t.Fatal("question snapshot changed")
	}
	_, fresh, err := src.SearchAnswerSnapshot(reg)
	if err != nil || !strings.Contains(string(fresh), "later log") {
		t.Fatalf("fresh board missing log: %s %v", fresh, err)
	}
	// The authoritative lock is respected, rather than reading raw folders
	// around recovery or serving a stale cached board on a busy lock.
	lock := flock.New(filepath.Join(whiteboardFolder(f.root, f.key).Dir, ".lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Unlock()
	if _, _, err := src.SearchAnswerSnapshot(reg); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("snapshot bypassed folder lock: %v", err)
	}
}

func TestDashboardSearchAnswerSnapshotIdentityAndAbsentBoard(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	root, data, err := src.SearchAnswerSnapshot(reg)
	if err != nil || root != f.root || string(data) != "null" {
		t.Fatalf("absent board: %s %s %v", root, data, err)
	}
	if _, err := os.Stat(whiteboardFolder(f.root, f.key).Dir); !os.IsNotExist(err) {
		t.Fatalf("read created board: %v", err)
	}
	for _, bad := range []wsdashboard.Registration{{ReplyID: reg.ReplyID, Project: t.TempDir()}, {ReplyID: "missing", Project: f.root}, {ReplyID: reg.ReplyID, Project: f.root, Agent: true}} {
		if _, _, err := src.SearchAnswerSnapshot(bad); !os.IsNotExist(err) {
			t.Fatalf("bad identity accepted: %+v: %v", bad, err)
		}
	}
}
