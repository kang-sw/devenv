// Claude initialization, context, approval, and partial stream events.

package wsagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wslive"
	"strconv"
	"strings"
)

// A tool whose prompt is a question to the user, not a permission, is
// denied with a message that tells the model what to do instead: an allow
// without the user's answers breaks such a tool, and the dashboard renders
// no form for it. claudeChatOnlyDenial is for a question (AskUserQuestion,
// or any request flagged as needing user interaction); claudePlanDenial is
// ExitPlanMode's, since only the user's own mode change leaves plan mode.
const (
	claudeChatOnlyDenial  = "This session has no interactive form for this tool. Put the question in your reply as plain text and end your turn; the user answers in the chat."
	claudePlanDenial      = "This session has no interactive form for approving a plan. Present the plan in your reply as plain text and end your turn. The session stays in plan mode until the user changes the permission mode from the session's menu; do not call ExitPlanMode again before the user says the mode changed."
	claudeMalformedDenial = "The permission request was malformed (no tool name, tool call id, or input), so it was denied."
)

// claudeCommandsAsk is the request id of the initialize control a lead's
// launch sends for its slash commands (the control answers in print mode
// without a model turn, seen on 2.1.323).
const claudeCommandsAsk = "ws-commands"

// initialized reads the initialize answer's commands: the page's list until
// the first turn's init, which names the commands a message may run and
// replaces it. A name the harness hides (a leading "__") stays out.
func (m *claudeMapper) initialized(l claudeLine) []Event {
	var d struct {
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"commands"`
	}
	if l.Response.Subtype != "success" || json.Unmarshal(l.Response.Response, &d) != nil || len(d.Commands) == 0 {
		return nil
	}
	m.described = map[string]string{}
	list := make([]wslive.SlashCommand, 0, len(d.Commands))
	for _, c := range d.Commands {
		if c.Name == "" || strings.HasPrefix(c.Name, "__") {
			continue
		}
		m.described[c.Name] = c.Description
		list = append(list, wslive.SlashCommand{Name: c.Name, Description: c.Description})
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}}}
}

// claudeContextAsk prefixes the request id of a get_context_usage ask the
// mapper sends; its answer is told from other control responses by it, also
// when replayed under a later mapper whose ask count differs.
const claudeContextAsk = "ws-context-"

// askContextUsage asks the harness for its context usage (Claude Code's
// get_context_usage control, answered in print mode without a model turn,
// seen on 2.1.322) unless a compaction window is known for the current
// model. A set_model applies from the next turn, whose init names the new
// model, so a model change asks again; an unanswered or refused ask is
// repeated at the next init. A compaction does not change the window and
// asks nothing.
func (m *claudeMapper) askContextUsage(replies lineReplies) {
	if m.compactWindow > 0 && m.compactModel == m.model {
		return
	}
	m.asks++
	m.askModel = m.model
	replies.send(claudeProtocol{}.requestLine(claudeContextAsk+strconv.Itoa(m.asks), map[string]any{"subtype": "get_context_usage"}))
}

// contextUsage reads a get_context_usage answer's compaction window
// (rawMaxTokens, the figure the terminal band shows) and reports the new
// window when a context figure is already shown.
func (m *claudeMapper) contextUsage(l claudeLine) []Event {
	var d struct {
		RawMaxTokens int64 `json:"rawMaxTokens"`
	}
	if l.Response.Subtype != "success" || json.Unmarshal(l.Response.Response, &d) != nil || d.RawMaxTokens <= 0 {
		return nil
	}
	changed := d.RawMaxTokens != m.compactWindow || m.askModel != m.compactModel
	m.compactWindow, m.compactModel = d.RawMaxTokens, m.askModel
	if !changed || m.context == nil {
		return nil
	}
	return []Event{m.status()}
}

// claudeCanUseTool is a can_use_tool control request's body.
type claudeCanUseTool struct {
	Subtype     string          `json:"subtype"`
	ToolName    string          `json:"tool_name"`
	Input       json.RawMessage `json:"input"`
	Description string          `json:"description"`
	ToolUseID   string          `json:"tool_use_id"`
	AgentID     string          `json:"agent_id"`
	// RequiresUserInteraction marks a tool whose prompt is a question to
	// the user (either spelling).
	RequiresUserInteraction  bool `json:"requires_user_interaction"`
	RequiresUserInteraction2 bool `json:"requiresUserInteraction"`
}

// approval maps a lead's control request. A can_use_tool request becomes a
// waiting prompt; one that names no tool, tool call, or input is denied at
// once rather than left hanging, and so is a tool whose prompt is a
// question to the user. Any other control request has no handler here: it
// is refused with an error answer, so the harness does not wait on it, and
// noted for the log.
func (m *claudeMapper) approval(l claudeLine, now int64, replies lineReplies) []Event {
	if l.RequestID == "" {
		return nil
	}
	var req claudeCanUseTool
	if json.Unmarshal(l.Request, &req) != nil || req.Subtype != "can_use_tool" {
		replies.send(claudeControlError(l.RequestID, fmt.Sprintf("this session's host does not handle the control request %q", req.Subtype)))
		return []Event{{Notice: fmt.Sprintf("the harness sent a control request this session does not handle (%q); it was answered with an error", req.Subtype)}}
	}
	ar := ApprovalRequest{ID: l.RequestID, Tool: req.ToolName, Input: req.Input, Description: req.Description, ToolUseID: req.ToolUseID, AgentID: req.AgentID, StartedAt: now}
	input := bytes.TrimSpace(req.Input)
	switch {
	case req.ToolName == "" || req.ToolUseID == "" || len(input) == 0 || input[0] != '{':
		replies.send(claudeApprovalResponse(ar, false, claudeMalformedDenial))
		return nil
	case req.ToolName == "ExitPlanMode":
		replies.send(claudeApprovalResponse(ar, false, claudePlanDenial))
		return nil
	case req.ToolName == "AskUserQuestion" || req.RequiresUserInteraction || req.RequiresUserInteraction2:
		replies.send(claudeApprovalResponse(ar, false, claudeChatOnlyDenial))
		return nil
	}
	m.approvals[req.ToolUseID] = l.RequestID
	return []Event{{Approval: &ar}}
}

// settled returns an ApprovalDone for each waiting prompt whose tool call a
// user message brings the result of: the prompt was answered. A subagent's
// tool results count too.
func (m *claudeMapper) settled(raw json.RawMessage) []Event {
	if len(m.approvals) == 0 {
		return nil
	}
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return nil
	}
	var out []Event
	for _, b := range blocks {
		if id, ok := m.approvals[b.ToolUseID]; ok && b.Type == "tool_result" {
			delete(m.approvals, b.ToolUseID)
			out = append(out, Event{ApprovalDone: id})
		}
	}
	return out
}

// streamEvent maps a lead's partial-message stream event: a text block's
// deltas accumulate and are sent as the reply so far (not final), at most
// every claudePartialEvery ms. The block's whole assistant line follows and
// finishes the reply with its own text, so a stream that stops short (an
// interrupt) is finished by the turn's end instead. Other events (thinking,
// tool input) carry no reply text.
func (m *claudeMapper) streamEvent(raw json.RawMessage, now int64) []Event {
	var ev struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}
	if json.Unmarshal(raw, &ev) != nil {
		return nil
	}
	switch ev.Type {
	case "message_start", "content_block_start":
		m.partial.Reset()
		m.partialAt = 0
		return nil
	case "content_block_delta":
		if ev.Delta.Type != "text_delta" || ev.Delta.Text == "" {
			return nil
		}
		m.partial.WriteString(ev.Delta.Text)
		if m.partialAt != 0 && now-m.partialAt < claudePartialEvery {
			return nil
		}
		m.partialAt = now
		if strings.TrimSpace(m.partial.String()) == "" {
			return nil
		}
		// A turn the harness began on its own opens here, ahead of its
		// first reply text, as an assistant line opens it.
		return []Event{{TurnStart: true}, {Live: &wslive.Event{Kind: wslive.KindReply, Text: m.partial.String()}}}
	}
	return nil
}
