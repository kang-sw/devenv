package wslive

import "strings"

// mergeSpawnAgents keeps the daemon's identity, nesting and lifecycle when a
// harness also reports that conversation. Preserve the harness's activity and
// telemetry, and redirect its children's parent ids to the retained row.
// This is snapshot-only: neither producer's later events lose their target.
func mergeSpawnAgents(agents []Agent) []Agent {
	byReply, byNative := map[string]int{}, map[string]int{}
	for i, a := range agents {
		if a.AgentKind != AgentKindPi {
			continue
		}
		if a.ReplyID != "" {
			byReply[a.ReplyID] = i
		}
		if strings.HasPrefix(a.ID, "pi:") {
			byNative[strings.TrimPrefix(a.ID, "pi:")] = i
		}
	}
	aliases := map[string]string{}
	drop := map[int]bool{}
	for i, a := range agents {
		if a.AgentKind == AgentKindPi {
			continue
		}
		j, ok := byReply[a.ReplyID]
		if !ok {
			j, ok = byNative[a.ID]
		}
		if !ok {
			continue
		}
		d := &agents[j]
		aliases[a.ID], drop[i] = d.ID, true
		d.Activity = a.Activity
		d.ContextTokens, d.OutputTps = a.ContextTokens, a.OutputTps
		if d.Model == "" {
			d.Model = a.Model
		}
		if d.Effort == "" {
			d.Effort = a.Effort
		}
		if d.Status != StatusRunning {
			d.OutputTps = 0
		}
	}
	out := make([]Agent, 0, len(agents)-len(drop))
	for i, a := range agents {
		if drop[i] {
			continue
		}
		if parent, ok := aliases[a.ParentID]; ok {
			a.ParentID = parent
		}
		out = append(out, a)
	}
	return out
}
