package mcp

import (
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func TestDashboardIncarnationAdmissionUsesMemory(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	defer f.s.Close()
	f.update(t, `<wb:topic name="topic"><wb:body>initial</wb:body></wb:topic>`)
	src := dashboardPageSource(t, nil)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	page, ok, err := src.ReadPage(reg)
	if err != nil || !ok {
		t.Fatalf("page = %v, %v", ok, err)
	}
	before := wsstate.FileIOCounters()["file_io_total"]
	unlock, ok := src.LockBoardIncarnation(reg, page.BoardIncarnation)
	if !ok {
		t.Fatal("rendered incarnation refused")
	}
	unlock()
	if _, ok := src.LockBoardIncarnation(reg, "stale"); ok {
		t.Fatal("stale incarnation admitted")
	}
	if after := wsstate.FileIOCounters()["file_io_total"]; after != before {
		t.Fatalf("incarnation comparison added %d file operations", after-before)
	}
}
