package mcp

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsdoc"
)

func TestDashboardTicketIdeasAbsentBoardIsEmpty(t *testing.T) {
	f := newWhiteboardFixture(t)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	rows, ok, err := src.ReadTicketIdeas(wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root})
	if !ok || err != nil || len(rows) != 0 {
		t.Fatalf("absent board demand = %+v %v %v", rows, ok, err)
	}
}

func TestDashboardTicketIdeasOnlyOnDemand(t *testing.T) {
	f := newWhiteboardFixture(t)
	writeTicket(t, f.root, "idea", "261117-feat-idea", "Idea <one>")
	writeTicket(t, f.root, "ready", "261117-bug-ready", "Ready")
	for day := 1; day <= 12; day++ {
		writeTicket(t, f.root, "todo", "261117-feat-todo-"+strings.Repeat("x", day), "Todo")
	}
	f.update(t, ``)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	original := whiteboardTicketsList
	var reads [][]string
	whiteboardTicketsList = func(root string, opts wsdoc.TicketListOptions) ([]wsdoc.TicketInfo, error) {
		if root != f.root {
			t.Fatalf("wrong ticket root: %s", root)
		}
		reads = append(reads, append([]string(nil), opts.Statuses...))
		return original(root, opts)
	}
	t.Cleanup(func() { whiteboardTicketsList = original })
	page := readPageOK(t, src, reg)
	if len(reads) == 0 {
		t.Fatal("live page read no ticket panel")
	}
	for _, statuses := range reads {
		if !reflect.DeepEqual(statuses, []string{"ready", "todo"}) {
			t.Fatalf("live page read %v", statuses)
		}
	}
	if strings.Contains(string(page.Board.Panels), "Idea &lt;one&gt;") || strings.Count(string(page.Board.Panels), `class="kcard"`) != 13 {
		t.Fatalf("live panel did not list all ready/todo alone: %s", page.Board.Panels)
	}
	for _, watch := range page.Watch {
		if strings.Contains(watch, filepath.Join("tickets", "idea")) {
			t.Fatalf("idea joined live watches: %s", watch)
		}
	}
	reads = nil
	rows, ok, err := src.ReadTicketIdeas(reg)
	if !ok || err != nil || len(rows) != 1 || rows[0].Category != "feat" || rows[0].Title != "Idea <one>" || rows[0].Path != "ai-docs/tickets/idea/261117-feat-idea.md" || !reflect.DeepEqual(reads, [][]string{{"idea"}}) {
		t.Fatalf("demand = %+v %v %v reads=%v", rows, ok, err, reads)
	}
	reads = nil
	if _, ok, err := src.ReadTicketIdeas(wsdashboard.Registration{ReplyID: "nosuchsession", Project: f.root}); ok || err != nil || len(reads) != 0 {
		t.Fatal("unknown session read ticket files")
	}
}
