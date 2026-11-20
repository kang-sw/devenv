// Pi event mapper state and RPC line dispatch.

package wsagent

import (
	"encoding/json"
	"github.com/kang-sw/devenv/internal/wslive"
	"strings"
	"sync/atomic"
	"time"
)

// piMapper maps Pi's RPC output to runner events: the tool loadout of each
// system message to the tool check, assistant text to final replies, tool
// calls to the lead's batch (one per assistant message), compaction to
// compact events, agent_settled to the turn's end, session statistics
// (asked for during and after each turn) to usage and the session id, and the hosted
// lead's subagent widget (piAgentsWidgetKey) to subagent rows.
type piMapper struct {
	spec    StartSpec
	surface piSurface
	now     func() time.Time
	batches batchTracker
	// aborted is set by Interrupt; an aborted run's error is the interrupt
	// asked for, not a failure.
	aborted atomic.Bool

	// nativeRun is bounded by Pi's agent_start/agent_settled, not the
	// completion of a prompt or compact RPC command. The owning stream's
	// feedMu also serializes probe registration/cleanup with mapLine.
	nativeRun        bool
	nativeKnown      bool
	nativeReplayGap  bool
	nativeGeneration uint64
	nativeProbes     map[string]uint64
	// A command answer seen before replay establishes native state is
	// retained with its source sequence, not yet a lifecycle boundary.
	pendingCommand *Event
	// A watchdog notify precedes Pi's public agent_start. Carry it across
	// that one boundary, whose TurnStart clears transient session errors.
	pendingLeadWarning string
	messages           int
	turnErr            string
	turnReason         string
	// Explicit opt-in from this process's native bundle. Legacy bundles keep
	// their reasonless hosted ends and existing answer-check behavior.
	summaryCapable bool
	lastStatsAt    int64
	// Receipt-time streaming span, excluding request and tool latency. A
	// missing/burst-flushed stream has no usable rate.
	outputStartedAt int64
	outputTps       *float64
	// Status replaces usage as a whole; evidence-only updates retain the
	// last reported figures rather than clearing them between stats replies.
	lastStatus wslive.Event
}

func newPiMapper(spec StartSpec) *piMapper {
	return &piMapper{spec: spec, surface: piToolSurface(spec), now: time.Now, batches: newBatchTracker("p"),
		nativeKnown: spec.Holder == nil || spec.Holder.id == ""}
}

// replayState runs under feedMu with the existing holder replay markers.
// An attached mapper has not seen the missing prefix: a command answer
// cannot establish idle until the whole history, a native boundary, or a
// recovery answer establishes the native state.
func (m *piMapper) replayState(ev Event) []Event {
	if ev.ReplayGap {
		m.nativeKnown, m.nativeReplayGap = false, true
		m.nativeGeneration++
		m.pendingCommand = nil
	}
	var out []Event
	if ev.ReplayDone {
		if !ev.ReplayLost && !m.nativeReplayGap {
			m.nativeKnown = true
			if done := m.pendingCommand; done != nil && !m.nativeRun {
				// Complete history confirms command-only completion. Keep
				// its sequence/replay identity so an old answer cannot close
				// a later persisted worker turn.
				out = m.endTurn(m.now().UnixMilli(), done.TurnError)
				for i := range out {
					out[i].Seq, out[i].Replay = done.Seq, done.Replay
				}
				if done.Seq > 0 {
					out = append(out, Event{Seq: done.Seq, Replay: done.Replay, LineEnd: true})
				}
			}
		}
		m.pendingCommand = nil
		m.nativeReplayGap = false
	}
	return out
}

type piLine struct {
	Type       string          `json:"type"`
	ID         string          `json:"id"`
	Command    string          `json:"command"`
	Success    bool            `json:"success"`
	Error      string          `json:"error"`
	Data       json.RawMessage `json:"data"`
	Message    json.RawMessage `json:"message"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	// Result is a tool_execution_end's result, read only for a card or UI call:
	// left raw so no other line's result shape can fail the line's decode.
	Result       json.RawMessage `json:"result"`
	IsError      bool            `json:"isError"`
	Reason       string          `json:"reason"`
	Aborted      bool            `json:"aborted"`
	ErrorMessage string          `json:"errorMessage"`
	Method       string          `json:"method"`
	// WidgetKey and WidgetLines are a setWidget request's. The lines stay
	// raw so a widget of another shape cannot fail the line's decode.
	WidgetKey   string          `json:"widgetKey"`
	WidgetLines json.RawMessage `json:"widgetLines"`
}

type piMessage struct {
	Model         string          `json:"model"`
	ResponseModel string          `json:"responseModel"`
	ResponseID    string          `json:"responseId"`
	ThinkingLevel string          `json:"thinkingLevel"`
	Role          string          `json:"role"`
	Content       json.RawMessage `json:"content"`
	ToolsAdded    []struct {
		Name string `json:"name"`
	} `json:"toolsAdded"`
	StopReason   string             `json:"stopReason"`
	ErrorMessage string             `json:"errorMessage"`
	Timestamp    int64              `json:"timestamp"`
	Usage        *wslive.TokenUsage `json:"usage"`
}

type piBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type piStats struct {
	SessionID    string   `json:"sessionId"`
	Cost         *float64 `json:"cost"`
	ContextUsage *struct {
		Tokens        *int64   `json:"tokens"`
		ContextWindow int64    `json:"contextWindow"`
		Percent       *float64 `json:"percent"`
	} `json:"contextUsage"`
}

func (m *piMapper) mapLine(line []byte, replies lineReplies) []Event {
	var l piLine
	if json.Unmarshal(line, &l) != nil {
		return nil
	}
	now := m.now().UnixMilli()
	switch l.Type {
	case "response":
		if l.Command == "get_state" {
			// A replay may have lost agent_start. Reconcile in stream order,
			// before another command answer can consult the native state.
			generation, probed := m.nativeProbes[l.ID]
			if probed && l.Success && generation == m.nativeGeneration {
				var d struct {
					IsStreaming *bool `json:"isStreaming"`
				}
				if json.Unmarshal(l.Data, &d) == nil && d.IsStreaming != nil {
					m.nativeRun, m.nativeKnown = *d.IsStreaming, true
					m.pendingCommand = nil
				}
			}
			delete(m.nativeProbes, l.ID)
		}
		if l.ID != "" {
			replies.answer(l.ID, controlAnswer{ok: l.Success, err: l.Error, body: l.Data})
		}
		switch l.Command {
		case "prompt":
			if !l.Success {
				return m.commandDone(now, "prompt refused: "+l.Error)
			}
			var d struct {
				Disposition string `json:"disposition"`
			}
			if json.Unmarshal(l.Data, &d) == nil && d.Disposition == "handled" {
				return m.commandDone(now, "")
			}
		case "get_session_stats":
			if l.Success {
				return m.stats(l.Data)
			}
		case "get_commands":
			if l.Success {
				return piCommands(l.Data)
			}
		case "compact":
			// RPC compact has the same response contract in both roles.
			errText := ""
			if !l.Success {
				errText = "compact refused: " + l.Error
			}
			if !m.nativeKnown && !m.nativeRun {
				if _, replaying := replies.(silentReplies); !replaying {
					return m.endTurn(now, errText)
				}
			}
			return m.commandDone(now, errText)
		case "steer":
			// A non-lead process steers a child's report, which the runner
			// sends as a turn instead when pi refuses it (children.go).
			if m.spec.Lead == nil {
				answer := SteerAnswer{}
				if !l.Success {
					answer.Refusal = l.Error
					if answer.Refusal == "" {
						answer.Refusal = "refused"
					}
				}
				return []Event{{SteerAnswer: &answer}}
			}
			// A lead's message was taken from the inbox already; a refusal
			// is all that is left of it.
			if !l.Success {
				return []Event{{Notice: "pi refused a steered message: " + l.Error}}
			}
		}
	case "agent_start":
		// Workers need the same boundary when rebuilding a held RPC stream.
		m.nativeRun, m.nativeKnown = true, true
		m.nativeGeneration++
		m.pendingCommand = nil
		out := []Event{{TurnStart: true}}
		if warning := m.pendingLeadWarning; warning != "" {
			out = append(out, Event{CommandError: warning})
			m.pendingLeadWarning = ""
		}
		return out
	case "message_start":
		var msg piMessage
		if json.Unmarshal(l.Message, &msg) == nil && msg.Role == "assistant" {
			m.outputStartedAt = 0
		}
	case "message_update":
		if m.outputStartedAt == 0 {
			m.outputStartedAt = now
		}
	case "message_end":
		out := m.message(l.Message, now)
		var msg piMessage
		if json.Unmarshal(l.Message, &msg) == nil && msg.Role == "assistant" {
			m.askStats(replies, now, false)
		}
		return out
	case "turn_end":
		m.askStats(replies, now, false)
	case "tool_execution_end":
		out := m.batches.endCall(l.ToolCallID, l.IsError, now)
		var result struct {
			Content []piBlock `json:"content"`
		}
		tool := resultTool(l.ToolName, piMCPToolName)
		if l.IsError || tool == "" {
			return out
		}
		// A result that does not decode reads as empty text: a tracker
		// write still counts, and no card or UI line parses.
		var parts []string
		if json.Unmarshal(l.Result, &result) == nil {
			for _, b := range result.Content {
				if b.Type == "text" {
					parts = append(parts, b.Text)
				}
			}
		}
		return append(out, resultEvents(tool, strings.Join(parts, "\n"))...)
	case "compaction_start", "compaction_end":
		trigger := ""
		switch l.Reason {
		case "manual":
			trigger = wslive.TriggerManual
		case "threshold", "overflow":
			trigger = wslive.TriggerAuto
		}
		phase := wslive.CompactStart
		if l.Type == "compaction_end" {
			phase = wslive.CompactEnd
			if l.Aborted || l.ErrorMessage != "" {
				phase = wslive.CompactFailed
			}
		}
		return []Event{{Live: &wslive.Event{Kind: wslive.KindCompact, Phase: phase, Trigger: trigger, Text: l.ErrorMessage}}}
	case "agent_settled":
		m.nativeRun, m.nativeKnown = false, true
		m.nativeGeneration++
		m.pendingCommand = nil
		m.askStats(replies, now, true)
		if m.spec.Lead != nil {
			replies.send(piGetCommands)
		}
		err := m.turnErr
		if m.aborted.Load() || (m.summaryCapable && l.Aborted) {
			err = ""
			m.turnReason = wslive.TurnAborted
		}
		return m.endTurn(now, err)
	case "extension_ui_request":
		// No one can answer a dialog in a headless conversation; a waiting
		// dialog would hold the run forever.
		switch l.Method {
		case "select", "confirm", "input", "editor":
			replies.send(map[string]any{"type": "extension_ui_response", "id": l.ID, "cancelled": true})
		case "notify":
			// The dependency-free launch watchdog must reach the dashboard,
			// even when the bundled extension never initialized its own feed.
			var text string
			if m.spec.Lead != nil && json.Unmarshal(l.Message, &text) == nil && strings.HasPrefix(text, "[ws Pi] ") {
				m.pendingLeadWarning = text
				return []Event{{CommandError: text}}
			}
		case "setWidget":
			if l.WidgetKey == piEventsWidgetKey {
				out := piHostedEvents(l.WidgetLines)
				kept := out[:0]
				for _, ev := range out {
					if m.spec.Lead == nil {
						// Workers report activity on the same RPC channel, but do
						// not gain the hosted lead's wake or summary authority.
						if ev.BackgroundTasks != nil {
							kept = append(kept, Event{BackgroundTasks: ev.BackgroundTasks, Background: ev.Background, Monitors: ev.Monitors})
						}
						continue
					}
					if ev.summaryCapability != nil {
						m.summaryCapable = *ev.summaryCapability && len(line)+1 <= wslive.SummaryFrameBytes
						ev.summaryCapability = nil
					}
					if ev.ReasoningSummary != nil && (!m.summaryCapable || len(line)+1 > wslive.SummaryFrameBytes) {
						continue
					}
					kept = append(kept, ev)
				}
				return kept
			}
			if l.WidgetKey == piAgentsWidgetKey {
				return piAgentEvents(l.WidgetLines)
			}
		}
	}
	return nil
}

// askStats coalesces busy message/turn boundaries. Settlement always gets a
// final refresh, even when the last mid-turn request was recent.
func (m *piMapper) askStats(replies lineReplies, now int64, settled bool) {
	if !settled && m.lastStatsAt != 0 && now-m.lastStatsAt < 1000 {
		return
	}
	m.lastStatsAt = now
	replies.send(map[string]any{"type": "get_session_stats"})
}
