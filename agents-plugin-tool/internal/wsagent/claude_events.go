// Claude stream event mapping and turn boundaries.

package wsagent

import (
	"encoding/json"
	"errors"
	"github.com/kang-sw/devenv/internal/wslive"
	"slices"
	"strings"
)

func (m *claudeMapper) mapLine(line []byte, replies lineReplies) []Event {
	evs := m.mapEvents(line, replies)
	if slices.ContainsFunc(evs, func(ev Event) bool { return ev.TurnStart }) {
		m.ended = false
	}
	return evs
}

func (m *claudeMapper) mapEvents(line []byte, replies lineReplies) []Event {
	var l claudeLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "control_response":
		replies.answer(l.Response.RequestID, controlAnswer{ok: l.Response.Subtype == "success", err: l.Response.Error, body: l.Response.Response})
		if strings.HasPrefix(l.Response.RequestID, claudeContextAsk) {
			return m.contextUsage(l)
		}
		if l.Response.RequestID == claudeCommandsAsk && m.lead() {
			return m.initialized(l)
		}
	case "control_request":
		if m.lead() {
			return m.approval(l, now, replies)
		}
	case "control_cancel_request":
		// The harness withdrew a prompt: it is settled with no answer.
		if m.lead() && l.RequestID != "" {
			for toolUse, id := range m.approvals {
				if id == l.RequestID {
					delete(m.approvals, toolUse)
				}
			}
			return []Event{{ApprovalDone: l.RequestID}}
		}
	case "system":
		if l.CompactResult == "failed" {
			// A compaction that failed: it settles as a failed compaction
			// line carrying the harness's own error text.
			m.compacted = false
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactFailed, Text: l.CompactError}}}
		}
		switch l.Subtype {
		case "api_retry":
			if !m.lead() {
				return nil
			}
			m.retrying = true
			r := wslive.Retry{Attempt: rawInt(l.Attempt), MaxRetries: rawInt(l.MaxRetries), Error: rawString(l.Error), Status: rawInt(l.ErrorStatus)}
			if r.MaxRetries < r.Attempt {
				r.MaxRetries = 0
			}
			return []Event{{Retry: &r}}
		case "init":
			var init claudeInit
			if json.Unmarshal(line, &init) != nil {
				return []Event{{Fatal: errors.New("launch refused: unreadable claude init message")}}
			}
			if m.lead() {
				return m.leadInit(init, replies)
			}
			if err := checkClaudeInit(init, m.spec); err != nil {
				return []Event{{Fatal: err}}
			}
			out := []Event{m.harnessReport(init)}
			if init.SessionID != "" {
				out = append(out, Event{SessionID: init.SessionID})
			}
			return out
		case "session_state_changed":
			// running opens a turn whether or not a send began it (a
			// background task's result, a pushed message), and again after
			// a prompt (requires_action) inside one, which the runner takes
			// as the turn it already runs; idle ends the turn.
			if !m.lead() {
				return nil
			}
			switch l.State {
			case HarnessRunning:
				m.states = true
				return []Event{{HarnessState: l.State}, {TurnStart: true}}
			case HarnessWaiting:
				m.states = true
				return []Event{{HarnessState: l.State}}
			case HarnessIdle:
				m.states = true
				if m.ended {
					// The result before it ended the turn (states).
					m.ended = false
					return []Event{{HarnessState: l.State}}
				}
				out := append([]Event{{HarnessState: l.State}}, m.closeTurn(now)...)
				out = append(out, m.status(), Event{TurnEnd: true, TurnError: m.turnErr, CompactRequested: m.leverEnd})
				m.turnErr, m.leverEnd = "", false
				return out
			}
		case "status":
			var out []Event
			if l.PermissionMode != "" {
				out = append(out, Event{PermissionMode: l.PermissionMode})
			}
			// A compaction is a turn of its own when no send started it.
			if l.Status == "compacting" {
				out = append(out, Event{TurnStart: true}, Event{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactStart}})
			}
			return out
		case "background_tasks_changed":
			if m.lead() && l.Tasks != nil {
				return []Event{m.backgroundCounts(*l.Tasks)}
			}
		case "compact_boundary":
			m.compacted = true
			trigger := ""
			if l.CompactMetadata != nil && (l.CompactMetadata.Trigger == wslive.TriggerAuto || l.CompactMetadata.Trigger == wslive.TriggerManual) {
				trigger = l.CompactMetadata.Trigger
			}
			return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: wslive.CompactEnd, Trigger: trigger}}}
		case "task_started":
			if m.lead() && l.TaskID != "" {
				m.rememberWakeTask(l, false)
			}
			if l.TaskType == claudeAgentTask && l.TaskID != "" {
				m.tasks[l.TaskID] = true
				ev := wslive.Event{Kind: wslive.KindAgent, AgentID: l.TaskID, Status: wslive.StatusRunning, Label: l.Description}
				if m.lead() && l.ToolUseID != "" {
					spawn, ok := m.spawns[l.ToolUseID]
					if !ok {
						typ, _ := json.Marshal(map[string]string{"subagent_type": l.SubagentType})
						spawn = claudeSpawnOf(typ)
					}
					delete(m.spawns, l.ToolUseID)
					m.subs[l.ToolUseID] = &claudeSub{task: l.TaskID}
					ev.Model, ev.Effort = spawn.model, spawn.effort
				}
				return []Event{{Live: &ev}}
			}
		case "task_updated", "task_notification":
			var out []Event
			if m.lead() && l.Subtype == "task_notification" {
				out = m.taskWake(l)
			}
			status := l.Status
			if l.Subtype == "task_updated" {
				status = l.Patch.Status
			}
			if end := claudeTaskEnd(status); end != "" && m.tasks[l.TaskID] {
				delete(m.tasks, l.TaskID)
				for toolUse, sub := range m.subs {
					if sub.task == l.TaskID {
						delete(m.subs, toolUse)
					}
				}
				out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindAgent, AgentID: l.TaskID, Status: end}})
			}
			return out
		}
	case "stream_event":
		if l.ParentToolUseID != "" || !m.lead() {
			return nil
		}
		return m.streamEvent(l.Event, now)
	case "assistant":
		if l.ParentToolUseID != "" {
			return m.subagentReply(l, now)
		}
		if l.UUID != "" {
			if slices.Contains(m.uuids, l.UUID) {
				if m.compacted {
					return nil
				}
			} else {
				m.uuids = append(m.uuids, l.UUID)
				if len(m.uuids) > claudeSeenUUIDs {
					m.uuids = m.uuids[len(m.uuids)-claudeSeenUUIDs:]
				}
			}
		}
		alert := m.alert(l)
		out := m.assistant(l.Message, now, alert != nil)
		var msg claudeMessage
		if json.Unmarshal(l.Message, &msg) == nil {
			if observed := m.observation(l, msg); observed != nil {
				status := m.status()
				status.Live.Observed = observed
				out = append(out, status)
			}
		}
		if alert != nil {
			out = append(out, Event{Alert: alert})
		}
		if m.lead() {
			if m.retrying {
				m.retrying = false
				out = append([]Event{{RetryEnd: true}}, out...)
			}
			// The harness works: a turn runs, whether or not a send began it.
			out = append([]Event{{TurnStart: true}}, out...)
		}
		return out
	case "user":
		settled := m.settled(l.Message)
		if l.ParentToolUseID != "" {
			return settled
		}
		return append(settled, m.toolResults(l.Message, now)...)
	case "rate_limit_event":
		if l.RateLimit != nil {
			m.rateLimit(*l.RateLimit)
			for kind, w := range l.RateLimit.UnifiedWindows {
				if (kind == wslive.LimitFiveHour || kind == wslive.LimitSevenDay) && w.Utilization != nil {
					m.limits[kind] = wslive.UsageLimit{Kind: kind, Percent: *w.Utilization * 100, ResetsAt: w.ResetsAt * 1000}
					m.limitsAsOf = now
				}
			}
			return []Event{m.status()}
		}
	case "result":
		out := m.closeTurn(now)
		if m.retrying {
			m.retrying = false
			out = append(out, Event{RetryEnd: true})
		}
		for _, mu := range l.ModelUsage {
			if mu.ContextWindow > m.window {
				m.window = mu.ContextWindow
			}
		}
		if l.TotalCostUSD != nil {
			m.cost = &wslive.UsageCost{USD: *l.TotalCostUSD, Estimate: true}
		}
		out = append(out, m.status())
		// An interrupted turn's lever call does not hold: the person is
		// taking over, and the mod parks the request without compacting.
		lever := m.lever && !strings.HasPrefix(l.TerminalState, "aborted")
		m.lever = false
		clear(m.levers)
		if m.states && m.background == 0 {
			// The idle state that follows ends the turn.
			m.turnErr, m.leverEnd = claudeResultError(l), lever
			return out
		}
		m.ended, m.turnErr, m.leverEnd = m.states, "", false
		return append(out, Event{TurnEnd: true, TurnError: claudeResultError(l), CompactRequested: lever})
	}
	return nil
}

// claudeResultError is the error a result line ends its turn with, or
// empty. An interrupted turn ends as error_during_execution with an
// aborted_* terminal reason (aborted_streaming, aborted_tools); that is the
// interrupt asked for, not a failure.
func claudeResultError(l claudeLine) string {
	if (!l.IsError && l.Subtype == "success") || strings.HasPrefix(l.TerminalState, "aborted") {
		return ""
	}
	if msg := strings.TrimSpace(l.Result); msg != "" {
		return msg
	}
	if msg := strings.Join(l.Errors, "; "); msg != "" {
		return msg
	}
	return "turn failed: " + l.Subtype
}

// closeTurn is what a result and any end of a turn settle: a prompt still
// open will never be answered, the open tool batch is done, and a
// compaction's re-emitted message has passed.
func (m *claudeMapper) closeTurn(now int64) []Event {
	var out []Event
	m.compacted = false
	for toolUse, id := range m.approvals {
		delete(m.approvals, toolUse)
		out = append(out, Event{ApprovalDone: id})
	}
	if b := m.batches.finish(now, true); b != nil {
		out = append(out, *b)
	}
	return out
}

// leadInit maps a lead's init message, which every user turn brings: the
// session id, the turn's start, the permission mode the harness reports
// (it may differ from the one asked for), its MCP servers' status, and its
// slash commands (names only; the state keeps the list unchanged when a
// later init repeats it). It
// also asks for the compaction window when none is known for the init's
// model (askContextUsage).
func (m *claudeMapper) leadInit(init claudeInit, replies lineReplies) []Event {
	m.model = init.Model
	m.askContextUsage(replies)
	// The report goes first: the command list below is offered by it.
	out := []Event{m.harnessReport(init)}
	if init.SessionID != "" {
		out = append(out, Event{SessionID: init.SessionID})
	}
	out = append(out, Event{TurnStart: true})
	if init.PermissionMode != "" {
		out = append(out, Event{PermissionMode: init.PermissionMode})
	}
	if len(init.MCPServers) > 0 {
		servers := make([]MCPServer, 0, len(init.MCPServers))
		for _, s := range init.MCPServers {
			servers = append(servers, MCPServer{Name: s.Name, Status: s.Status})
		}
		out = append(out, Event{MCPServers: servers})
	}
	if init.SlashCommands != nil {
		list := make([]wslive.SlashCommand, 0, len(*init.SlashCommands))
		for _, name := range *init.SlashCommands {
			list = append(list, wslive.SlashCommand{Name: name, Description: m.described[name]})
		}
		out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}})
	}
	return out
}
