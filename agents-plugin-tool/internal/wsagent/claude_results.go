// Claude assistant results, tool results, and status events.

package wsagent

import (
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"strings"
)

// claudeSeenUUIDs is how many recent assistant line uuids the mapper keeps
// to recognise a compaction's re-emitted message.
const claudeSeenUUIDs = 64

// claudeAgentTask is the task type of a subagent the Agent tool started.
// Claude 2.1.319 runs it in the background: the Agent call's tool_result
// comes at once, and system task lines carry the subagent's start and end.
const claudeAgentTask = "local_agent"

// claudeTaskEnd maps a task status to the subagent's final status: done for
// completed, failed for any other terminal one, "" while it runs.
func claudeTaskEnd(status string) string {
	switch status {
	case "", "pending", "running":
		return ""
	case "completed":
		return wslive.StatusDone
	}
	return wslive.StatusFailed
}

// assistant maps the lead's assistant message: its text (left out when
// alerted, since the alert's line carries it) and its tool calls. A lead's
// message whose usage changes the context figure reports it (a status) at
// once, so the meter follows each model call rather than the turn's end; a
// message is often split over several lines carrying the same usage, and
// those report it once.
func (m *claudeMapper) assistant(raw json.RawMessage, now int64, alerted bool) []Event {
	var msg claudeMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	var out []Event
	if msg.Usage != nil && !alerted {
		// An alert's message is the harness's own, with no real usage.
		tokens := msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens + msg.Usage.OutputTokens
		changed := m.context == nil || m.context.Tokens == nil || *m.context.Tokens != tokens
		m.context = &wslive.UsageContext{Tokens: &tokens}
		if changed && m.lead() {
			out = append(out, m.status())
		}
	}
	var blocks []claudeBlock
	if json.Unmarshal(msg.Content, &blocks) != nil {
		return out
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" && !alerted {
				out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: b.Text, Final: true}})
			}
		case "tool_use":
			out = append(out, m.batches.addCall(msg.ID, b.ID, b.Name, argsPreview(b.Input), now)...)
			if !m.lead() {
				continue
			}
			if b.Name == "Agent" || b.Name == "Task" {
				m.spawns[b.ID] = claudeSpawnOf(b.Input)
			}
			if strings.HasSuffix(b.Name, claudeCompactionLever) {
				m.levers[b.ID] = true
			}
			if strings.HasSuffix(b.Name, claudeReconnectLever) {
				// The lead asks for an MCP reconnect: a server it needs is
				// gone, whatever the harness last reported.
				out = append(out, Event{MCPCheck: true})
			}
		}
	}
	if u := msg.Usage; u != nil && !alerted {
		out = append(out, m.batches.setUsage(msg.ID, &wslive.TokenUsage{
			Input: u.InputTokens, Output: u.OutputTokens,
			CacheRead: u.CacheReadInputTokens, CacheWrite: u.CacheCreationInputTokens,
		})...)
	}
	return out
}

func (m *claudeMapper) toolResults(raw json.RawMessage, now int64) []Event {
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
		if b.Type == "tool_result" {
			name := m.batches.callName(b.ToolUseID)
			out = append(out, m.batches.endCall(b.ToolUseID, b.IsError, now)...)
			if m.lead() && b.IsError && strings.HasPrefix(name, "mcp__") {
				// A failed MCP tool call may be a server that went away.
				out = append(out, Event{MCPCheck: true})
			}
			if m.levers[b.ToolUseID] {
				delete(m.levers, b.ToolUseID)
				m.lever = m.lever || !b.IsError
			}
			if tool := m.resultTool(name); !b.IsError && tool != "" {
				out = append(out, resultEvents(tool, claudeResultText(b.Content))...)
			}
		}
	}
	return out
}

// resultTool is resultTool under the name the attached ws-mcp's server has
// in this process: ReservedMCPServer, for a worker and a lead alike. A lead
// an older daemon started, and this one rejoined, runs a hosted bundle
// instead, whose server is plugin:<package>:<package> for either package,
// so a lead accepts those names too.
func (m *claudeMapper) resultTool(name string) string {
	if tool := resultTool(name, claudeMCPToolName); tool != "" || !m.lead() {
		return tool
	}
	for _, pkg := range hostedPluginPackages {
		server := claudePluginServer(pkg, pkg)
		if tool := resultTool(name, func(_, tool string) string { return claudeMCPToolName(server, tool) }); tool != "" {
			return tool
		}
	}
	return ""
}

// claudePluginServer is Claude Code's name for a plugin's MCP server.
func claudePluginServer(plugin, key string) string { return "plugin:" + plugin + ":" + key }

func (m *claudeMapper) status() Event {
	ev := wslive.Event{Kind: wslive.KindStatus, Cost: m.cost, LimitsAsOf: m.limitsAsOf}
	if m.context != nil {
		c := *m.context
		c.Window = m.window
		if m.compactWindow > 0 && m.compactModel == m.model {
			c.Window = m.compactWindow
		}
		ev.Context = &c
	}
	for _, kind := range []string{wslive.LimitFiveHour, wslive.LimitSevenDay} {
		if l, ok := m.limits[kind]; ok {
			ev.Limits = append(ev.Limits, l)
		}
	}
	return Event{Live: &ev}
}
