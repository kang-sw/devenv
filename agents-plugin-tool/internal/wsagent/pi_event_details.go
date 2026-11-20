// Pi command, agent, message, and usage event details.

package wsagent

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/kang-sw/devenv/internal/wslive"
	"strconv"
	"strings"
)

// piGetCommands asks a lead's pi for its slash commands.
var piGetCommands = map[string]any{"type": "get_commands"}

// piCompact is the compact command as the command list shows it: pi's
// get_commands leaves its built-in commands out, and compact is the one
// the runner maps (RunCommand).
var piCompact = wslive.SlashCommand{Name: "compact", Description: "Manually compact the session context", Source: "builtin"}

// piCommands maps a get_commands answer to a commands event: each command
// with its description and its source (extension, prompt, skill) as pi
// names it, which tells where it comes from, not who may invoke it, so no
// layer is set; compact is added when pi does not list it. An unreadable
// answer changes nothing.
func piCommands(raw json.RawMessage) []Event {
	var d struct {
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Source      string `json:"source"`
		} `json:"commands"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return nil
	}
	list := []wslive.SlashCommand{}
	compact := false
	for _, c := range d.Commands {
		compact = compact || c.Name == piCompact.Name
		list = append(list, wslive.SlashCommand{Name: c.Name, Description: c.Description, Source: c.Source})
	}
	if !compact {
		list = append([]wslive.SlashCommand{piCompact}, list...)
	}
	return []Event{{Live: &wslive.Event{Kind: wslive.KindCommands, Commands: list}}}
}

// piAgentsWidgetKey is the widget a hosted Pi lead's ws extension sets to
// report its native subagents: each line is one JSON live event, the
// agent and permission events its dashboard feed sends outside a hosted
// session. RPC mode carries an extension's output to stdout only as UI
// requests, and a widget passes string lines through unchanged.
const piAgentsWidgetKey = "ws-dashboard-agents"

// piAgentEvents decodes the subagent widget's lines. Only agent and
// permission events naming a subagent are taken: the lead, its turns, and
// a reset stay the runner's own. A line that does not decode is skipped.
func piAgentEvents(raw json.RawMessage) []Event {
	var lines []string
	if json.Unmarshal(raw, &lines) != nil {
		return nil
	}
	var out []Event
	for _, line := range lines {
		var ev wslive.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.AgentID == "" {
			continue
		}
		if ev.Kind == wslive.KindAgent || ev.Kind == wslive.KindPermission {
			out = append(out, Event{Live: &ev})
		}
	}
	return out
}

// commandDone completes a command-only turn. Extension commands can run
// during streaming or start a native run before their handled response;
// that response (or a refused concurrent command) must not settle the run.
// An attached stream whose prefix was lost is unknown, not confirmed idle.
func (m *piMapper) commandDone(now int64, errText string) []Event {
	if !m.nativeKnown {
		m.pendingCommand = &Event{TurnEnd: true, TurnError: errText}
	}
	if !m.nativeKnown || m.nativeRun {
		if errText != "" {
			return []Event{{CommandError: errText}}
		}
		return nil
	}
	return m.endTurn(now, errText)
}

func (m *piMapper) endTurn(now int64, turnErr string) []Event {
	var out []Event
	if ev := m.batches.finish(now, true); ev != nil {
		out = append(out, *ev)
	}
	reason := ""
	if m.summaryCapable {
		reason = m.turnReason
	}
	m.turnErr, m.turnReason = "", ""
	return append(out, Event{TurnEnd: true, TurnError: turnErr, TurnReason: reason})
}

func (m *piMapper) message(raw json.RawMessage, now int64) []Event {
	var msg piMessage
	if json.Unmarshal(raw, &msg) != nil {
		return nil
	}
	switch msg.Role {
	case "user":
		// A non-lead run took this message: a steered child's report it
		// took is not sent again (children.go).
		if m.spec.Lead == nil {
			if text := piUserText(msg.Content); text != "" {
				return []Event{{UserTaken: text}}
			}
		}
	case "system":
		var added []string
		for _, t := range msg.ToolsAdded {
			added = append(added, t.Name)
		}
		// A lead runs the user's own loadout; only a profile's is checked.
		if m.spec.Lead == nil {
			if err := checkPiTools(added, m.surface); err != nil {
				return []Event{{Fatal: err}}
			}
		}
	case "assistant":
		if u := msg.Usage; u != nil && msg.StopReason != "error" && msg.StopReason != "aborted" && m.outputStartedAt != 0 && now-m.outputStartedAt >= 250 && u.Output >= 8 {
			rate := float64(u.Output) * 1000 / float64(now-m.outputStartedAt)
			m.outputTps = &rate
		}
		m.outputStartedAt = 0
		m.messages++
		group := strconv.Itoa(m.messages)
		m.turnErr, m.turnReason = "", ""
		switch msg.StopReason {
		case "aborted":
			m.turnReason = wslive.TurnAborted
		case "error":
			m.turnReason = "error"
		case "length":
			m.turnReason = "incomplete"
		}
		if msg.StopReason == "error" {
			m.turnErr = strings.TrimSpace(msg.ErrorMessage)
			if m.turnErr == "" {
				m.turnErr = "turn failed"
			}
		}
		var blocks []piBlock
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return nil
		}
		var out []Event
		if observed := piObservation(msg, raw); observed != nil {
			status := m.lastStatus
			status.Kind, status.Observed = wslive.KindStatus, observed
			out = append(out, Event{Live: &status})
		}
		for _, b := range blocks {
			switch b.Type {
			case "text":
				if strings.TrimSpace(b.Text) != "" {
					out = append(out, Event{Live: &wslive.Event{Kind: wslive.KindReply, Text: b.Text, Final: true}})
				}
			case "toolCall":
				out = append(out, m.batches.addCall(group, b.ID, b.Name, argsPreview(b.Arguments), now)...)
			}
		}
		out = append(out, m.batches.setUsage(group, msg.Usage)...)
		return out
	}
	return nil
}

// piObservation reads only a completed response. Pi's message model names
// physical output; responseModel is the provider's more specific identity when
// supplied. Without responseId, a digest of the canonical completed message
// plus its original timestamp identifies replay without persisting content.
// The legacy first-update rate is ineligible.
func piObservation(msg piMessage, raw json.RawMessage) *wslive.ModelObservation {
	model := strings.TrimSpace(msg.ResponseModel)
	if model == "" {
		model = strings.TrimSpace(msg.Model)
	}
	if msg.Role != "assistant" || (msg.StopReason == "pending" || msg.StopReason == "error" || msg.StopReason == "aborted") || model == "" || msg.Timestamp <= 0 {
		return nil
	}
	id := msg.ResponseID
	if id == "" {
		var message any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if decoder.Decode(&message) != nil {
			return nil
		}
		canonical, err := json.Marshal(message)
		if err != nil {
			return nil
		}
		id = fmt.Sprintf("message:%d:%x", msg.Timestamp, sha256.Sum256(canonical))
	}
	observed := &wslive.ModelObservation{ID: id, Model: model, Source: "pi.assistant", At: msg.Timestamp}
	if msg.ThinkingLevel != "" {
		observed.Effort, observed.EffortSource = msg.ThinkingLevel, "message"
	}
	return observed
}

func (m *piMapper) stats(raw json.RawMessage) []Event {
	var s piStats
	if json.Unmarshal(raw, &s) != nil {
		return nil
	}
	var out []Event
	if s.SessionID != "" {
		out = append(out, Event{SessionID: s.SessionID})
	}
	ev := wslive.Event{Kind: wslive.KindStatus, OutputTps: m.outputTps}
	if s.Cost != nil {
		ev.Cost = &wslive.UsageCost{USD: *s.Cost, Estimate: true}
	}
	if c := s.ContextUsage; c != nil {
		ev.Context = &wslive.UsageContext{Tokens: c.Tokens, Window: c.ContextWindow, Percent: c.Percent}
	}
	if ev.Cost == nil && ev.Context == nil {
		return out
	}
	m.lastStatus = ev
	return append(out, Event{Live: &ev})
}
