package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

func TestDashboardAnswerRevisionWritesResolvedBoard(t *testing.T) {
	t.Setenv("WS_MCP_NO_DASHBOARD", "1")
	f := newWhiteboardFixture(t)
	f.update(t, `<wb:topic name="topic"><wb:body>initial</wb:body></wb:topic>`)
	src := dashboardPageSource(t, nil, f.s.sessions.boards)
	reg := wsdashboard.Registration{ReplyID: f.replyID(), Project: f.root}
	at := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	if err := src.RecordDashboardAnswer(reg, "inbox-id", at); err != nil {
		t.Fatal(err)
	}
	rec, err := f.s.sessions.readWhiteboard(f.key)
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range rec.Whiteboard.Topics {
		for _, entry := range doc.Entries {
			if strings.HasPrefix(entry.Text, `Dashboard answer received: "inbox-id"`) {
				return
			}
		}
	}
	t.Fatal("answer marker missing from board")
}
