package wslive

import "fmt"

// wakeEntry validates and bounds the optional wake payload, without changing
// turn origins: older daemons still accept every existing origin value.
func (st *State) wakeEntry(ev Event) (RingEntry, error) {
	if ev.AgentID != "" {
		return RingEntry{}, fmt.Errorf("wake is the lead's only; omit agentId")
	}
	switch ev.Cause {
	case "subagent_report", "subagent_done", "subagent_failed", "background_exit", "monitor", "mail", "message", "scheduled":
	default:
		return RingEntry{}, fmt.Errorf("unknown wake cause %q", ev.Cause)
	}
	caption := ""
	if ev.Caption != nil {
		caption = cleanLine(st.scrub.scrub(*ev.Caption), CaptionRunes)
	}
	text, truncated := capText(st.scrub.scrub(ev.Text))
	return RingEntry{Kind: RingWake, Cause: ev.Cause, Source: cleanLine(st.scrub.scrub(ev.Source), LabelRunes), Caption: caption, Text: text, Truncated: truncated}, nil
}
