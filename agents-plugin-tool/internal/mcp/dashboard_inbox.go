package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// dashboard_inbox.go is the adapter-only drain of a session's board-answer
// inbox (internal/wsinbox): the messages the user sent from the dashboard
// page and the daemon held on disk. The adapter calls it at the end of a
// turn and injects what it returns as the user's own message.

const dashboardInboxDrainTool = "dashboard.inbox_drain"

// dashboardInboxMessage is one drained message in the JSON output.
type dashboardInboxMessage struct {
	ID     string         `json:"id"`
	SentAt int64          `json:"sent_at"`
	Text   string         `json:"text"`
	Parts  []string       `json:"parts"`
	Chips  []wsinbox.Chip `json:"chips"`
}

// sessionInboxDir is the board-answer inbox of the session behind
// sessionKey. It resolves from the key alone, so a drain works whatever the
// session's dashboard key says: an empty inbox just reports empty.
func (s *Server) sessionInboxDir(sessionKey string) (string, error) {
	dir, err := s.dashboard.stateDir()
	if err != nil {
		return "", err
	}
	return wsinbox.Dir(wsdashboard.SessionDir(dir, wsmailbox.ReplyID(sessionKey))), nil
}

func (s *Server) handleDashboardInboxDrain(id json.RawMessage, args map[string]any) response {
	const tool = dashboardInboxDrainTool
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if _, found := s.sessions.lookup(key); !found {
		return toolTextResponse(id, "", fmt.Errorf("%s: unknown_session: session key not found", tool))
	}
	dir, err := s.sessionInboxDir(key)
	if err != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, err))
	}
	records, drainErr := wsinbox.Drain(dir)
	if len(records) > 0 {
		// The daemon re-reads so the page's held bubble clears.
		s.dashboardNudge(key)
	} else if drainErr != nil {
		return toolTextResponse(id, "", fmt.Errorf("%s: %w", tool, drainErr))
	}

	if wantsJSON(args) {
		messages := make([]dashboardInboxMessage, 0, len(records))
		for _, r := range records {
			e := r.Entry
			parts, chips := e.Parts, e.Chips
			if parts == nil {
				parts = []string{}
			}
			if chips == nil {
				chips = []wsinbox.Chip{}
			}
			messages = append(messages, dashboardInboxMessage{ID: r.ID(), SentAt: e.SentAt, Text: e.Text(), Parts: parts, Chips: chips})
		}
		return toolJSONResponse(id, map[string]any{"messages": messages}, nil)
	}
	return toolTextResponse(id, formatDashboardInboxDrain(records), nil)
}

// formatDashboardInboxDrain is the compact text output: "inbox empty", or
// "drained N" and one "message <id> <time>" block per message.
func formatDashboardInboxDrain(records []wsinbox.Record) string {
	if len(records) == 0 {
		return "inbox empty\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "drained %d\n", len(records))
	for _, r := range records {
		sent := ""
		if r.Entry.SentAt > 0 {
			sent = " " + time.UnixMilli(r.Entry.SentAt).UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(&b, "message %s%s\n%s\n\n", r.ID(), sent, r.Entry.Text())
	}
	return b.String()
}
