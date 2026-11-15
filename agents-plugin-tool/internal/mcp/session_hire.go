package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

func sessionHireTool(name string) bool {
	return name == "session.request_hire" || name == "session.roster"
}

type sessionHireHost interface {
	RequestHire(string, wsdashboard.HireRequest) (wsdashboard.HireProposal, error)
}

func (s *Server) handleSessionHire(id json.RawMessage, name string, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)
	entry, ok := s.sessions.lookup(key)
	if !ok || entry.scope != roleLead || (name == "session.request_hire" && !entry.isTopLevelLead()) {
		return toolTextResponse(id, "", fmt.Errorf("%s requires a lead session key; hiring requests require a top-level lead", name))
	}
	replyID := wsmailbox.ReplyID(key)
	if name == "session.request_hire" {
		host, ok := s.live.host.(sessionHireHost)
		if !ok {
			return toolTextResponse(id, "", fmt.Errorf("hiring requests require the dashboard daemon"))
		}
		request := wsdashboard.HireRequest{}
		raw, _ := json.Marshal(args)
		if err := json.Unmarshal(raw, &request); err != nil {
			return toolTextResponse(id, "", err)
		}
		// Open the env-less reply channel before accepting a request. Posting an
		// outcome alone neither opens the piggyback nor publishes this return id.
		if _, err := s.publishReplyID(key, entry.root, ""); err != nil {
			return toolTextResponse(id, "", err)
		}
		s.markMailboxReplyOpened(key)
		p, err := host.RequestHire(replyID, request)
		if err != nil {
			return toolTextResponse(id, "", err)
		}
		if wantsJSON(args) {
			return toolJSONResponse(id, map[string]any{"request_id": p.ID, "status": p.Status, "notice": "The outcome will arrive by mail; accepting the request does not mean a session exists."}, nil)
		}
		return toolTextResponse(id, fmt.Sprintf("request_id: %s\nstatus: pending\nThe outcome will arrive by mail; accepting the request does not mean a session exists.\n", p.ID), nil)
	}
	rows, err := s.readSessionRoster(replyID)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, rows, nil)
	}
	if len(rows) == 0 {
		return toolTextResponse(id, "No sessions nested under this session.\n", nil)
	}
	var out strings.Builder
	for _, row := range rows {
		model, activity := "unknown", "unknown"
		if row.Model != nil {
			model = *row.Model
		}
		if row.LastActivity != nil {
			activity = time.UnixMilli(*row.LastActivity).UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(&out, "%s: harness=%s model=%s label=%q state=%s running_subagents=%d last_activity=%s\n", row.Mailbox, row.Harness, model, row.Label, row.State, row.RunningSubagents, activity)
	}
	return toolTextResponse(id, out.String(), nil)
}

func (s *Server) readSessionRoster(replyID string) ([]wsdashboard.SessionRosterRow, error) {
	dir, err := s.dashboard.stateDir()
	if err != nil {
		return nil, err
	}
	ep, err := wsdashboard.ReadEndpoint(dir)
	if err != nil {
		return nil, fmt.Errorf("read dashboard daemon endpoint: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/session-hire/roster?requester=%s", ep.Port, url.QueryEscape(replyID)), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("session roster: %s", strings.TrimSpace(string(raw)))
	}
	var rows []wsdashboard.SessionRosterRow
	err = json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rows)
	return rows, err
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

func sessionHireTools() []map[string]any {
	return []map[string]any{
		{"name": "session.request_hire", "description": "Request an owner-approved hosted peer session without blocking. Only a top-level lead enabled by the owner in the dashboard may request hiring. reason explains the request to the owner; recovery_context preserves enough context to recover after compaction (at most 16 KiB). Optional harness, model, effort, and label prefill the owner's controls, not final settings. The session will use your worktree. Acceptance is not creation: approved, rejected, expired (30 minutes), or withdrawn outcomes arrive by mail and echo recovery_context. A hired peer has no hiring entitlement and grants you no stop/close authority.", "inputSchema": objectSchema(map[string]any{
			"session_key":      stringProperty("Caller's lead session key."),
			"reason":           stringProperty("Owner-facing explanation (required)."),
			"recovery_context": stringProperty("Context to recover this request after compaction; required, at most 16 KiB."),
			"label":            stringProperty("Suggested session label."),
			"harness":          stringProperty("Suggested harness: claude or pi; omitted uses the new-session default."),
			"model":            stringProperty("Suggested model; omitted uses the new-session default."),
			"effort":           stringProperty("Suggested effort; omitted uses the new-session default."),
			"format":           stringProperty("json for structured output; compact text otherwise."),
		}, "session_key", "reason", "recovery_context")},
		{"name": "session.roster", "description": "Read the sessions nested under this lead in the dashboard rail hierarchy. Owner drag-to-nest/detach changes this scope; the hierarchy grants no control or authority. Each row has exactly mailbox, harness, observed model (null when unknown/stale), label, idle/busy state, running direct-subagent count, and last tool activity (null when unknown). Any lead-scope session may read it.", "inputSchema": objectSchema(map[string]any{
			"session_key": stringProperty("Caller's lead session key."),
			"format":      stringProperty("json for structured rows; compact text otherwise."),
		}, "session_key")},
	}
}
