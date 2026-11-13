package wsagent

import (
	"encoding/json"
	"strings"

	"github.com/kang-sw/devenv/internal/wslive"
)

func wakeEvent(cause, source, caption, text string) Event {
	if caption == "" {
		caption, _, _ = strings.Cut(strings.TrimSpace(text), "\n")
	}
	caption = strings.Join(strings.Fields(caption), " ")
	return Event{Live: &wslive.Event{Kind: wslive.KindWake, Cause: cause, Source: source, Caption: &caption, Text: text}}
}

func wakeCause(cause string) bool {
	switch cause {
	case "subagent_report", "subagent_done", "subagent_failed", "background_exit", "monitor", "mail", "message", "scheduled":
		return true
	}
	return false
}

func copyCount(n *int) *int {
	if n == nil {
		return nil
	}
	v := *n
	return &v
}

func sameCount(a, b *int) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

type claudeWakeTask struct {
	kind, label string
	background  bool
	owned       bool
}

func (m *claudeMapper) rememberWakeTask(l claudeLine, listed bool) {
	t := m.wakeTasks[l.TaskID]
	if l.TaskType != "" {
		t.kind = l.TaskType
	}
	if l.Description != "" {
		t.label = l.Description
	}
	if l.IsBackgrounded != nil {
		t.background = *l.IsBackgrounded
	}
	t.background = t.background || listed
	t.owned = t.owned || l.OwnedBySubagent
	m.wakeTasks[l.TaskID] = t
}

// backgroundCounts preserves the aggregate for lifecycle decisions. A split
// is known only if every task has a recognised type; empty is a known zero.
// Do not send a count-only live status: status also replaces usage figures.
func (m *claudeMapper) backgroundCounts(tasks []json.RawMessage) Event {
	n, shells, monitors := len(tasks), 0, 0
	m.background = n
	known := true
	for _, raw := range tasks {
		var l claudeLine
		if json.Unmarshal(raw, &l) != nil {
			known = false
			continue
		}
		if l.TaskID != "" {
			m.rememberWakeTask(l, true)
		}
		switch l.TaskType {
		case "local_bash":
			shells++
		case "monitor", "local_monitor":
			monitors++
		case claudeAgentTask:
			// A background subagent is counted in the aggregate, not shells.
		default:
			known = false
		}
	}
	ev := Event{BackgroundTasks: &n}
	if known {
		ev.Background, ev.Monitors = &shells, &monitors
	}
	return ev
}

// taskWake reads the notification, not task_updated: only the former carries
// the summary delivered to the lead. Foreground tools and a subagent's own
// shells are not lead wakeups. Missing task identity is never guessed.
func (m *claudeMapper) taskWake(l claudeLine) []Event {
	t, ok := m.wakeTasks[l.TaskID]
	if !ok || !t.background || t.owned {
		delete(m.wakeTasks, l.TaskID)
		return nil
	}
	end := claudeTaskEnd(l.Status)
	if end != "" {
		delete(m.wakeTasks, l.TaskID)
	}
	cause := ""
	switch t.kind {
	case claudeAgentTask:
		switch end {
		case wslive.StatusDone:
			// Recorded notifications repeat the child's result in Summary
			// (not just a lifecycle caption). Keep that content visible.
			cause = "subagent_done"
			if strings.TrimSpace(l.Summary) != "" {
				cause = "subagent_report"
			}
		case wslive.StatusFailed:
			cause = "subagent_failed"
		default:
			if l.Summary != "" {
				cause = "subagent_report"
			}
		}
	case "local_bash":
		if end != "" {
			cause = "background_exit"
		}
	case "monitor", "local_monitor":
		cause = "monitor"
	}
	if cause == "" {
		return nil
	}
	source := t.label
	if source == "" {
		source = l.TaskID
	}
	return []Event{wakeEvent(cause, source, "", l.Summary)}
}

// piEventsWidgetKey is the hosted extension's notification/count channel.
// It carries JSON live-event lines like the agent widget, but never allows
// an extension to reset the runner's state or replace its usage or turns.
const piEventsWidgetKey = "ws-dashboard-events"

func piHostedEvents(raw json.RawMessage) []Event {
	var lines []string
	if json.Unmarshal(raw, &lines) != nil {
		return nil
	}
	var out []Event
	for _, line := range lines {
		var ev wslive.Event
		if json.Unmarshal([]byte(line), &ev) != nil || ev.AgentID != "" {
			continue
		}
		switch ev.Kind {
		case wslive.KindWake:
			if wakeCause(ev.Cause) {
				caption := ""
				if ev.Caption != nil {
					caption = *ev.Caption
				}
				out = append(out, wakeEvent(ev.Cause, ev.Source, caption, ev.Text))
			}
		case wslive.KindStatus:
			if ev.Background != nil && ev.Monitors != nil && *ev.Background >= 0 && *ev.Monitors >= 0 {
				n := *ev.Background + *ev.Monitors
				if n >= *ev.Background { // reject overflow
					out = append(out, Event{BackgroundTasks: &n, Background: ev.Background, Monitors: ev.Monitors})
				}
			}
		}
	}
	return out
}
