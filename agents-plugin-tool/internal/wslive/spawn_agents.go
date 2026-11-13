package wslive

import "strings"

// spawnAgentTargets identifies native/daemon aliases for both retention and
// presentation. Known aliases survive eviction/revival even if a later native
// event omits the delegate key it originally supplied.
func spawnAgentTargets(agents []Agent, known map[string]string) map[int]int {
	byID, byReply, byNative := map[string]int{}, map[string]int{}, map[string]int{}
	for i, a := range agents {
		if a.AgentKind != AgentKindPi {
			continue
		}
		byID[a.ID] = i
		if a.ReplyID != "" {
			byReply[a.ReplyID] = i
		}
		if strings.HasPrefix(a.ID, "pi:") {
			byNative[strings.TrimPrefix(a.ID, "pi:")] = i
		}
	}
	targets := map[int]int{}
	for i, a := range agents {
		if a.AgentKind == AgentKindPi {
			continue
		}
		j, ok := byID[known[a.ID]]
		if !ok {
			j, ok = byReply[a.ReplyID]
		}
		if !ok {
			j, ok = byNative[a.ID]
		}
		if !ok {
			continue
		}
		targets[i] = j
	}
	return targets
}

// mergeSpawnAgents keeps daemon identity/nesting/lifecycle and native activity
// without consuming either producer's future mutable event target.
func mergeSpawnAgents(agents []Agent, known map[string]string) []Agent {
	aliases := map[string]string{}
	targets := spawnAgentTargets(agents, known)
	for i, j := range targets {
		a, d := agents[i], &agents[j]
		aliases[a.ID] = d.ID
		if len(d.Batches) == 0 {
			d.Activity = a.Activity
		}
		// Hosted children have their own telemetry. A native alias may
		// carry activity alone; its absent figures must not erase those.
		if d.ContextTokens == 0 {
			d.ContextTokens = a.ContextTokens
		}
		if a.OutputTps > 0 {
			d.OutputTps = a.OutputTps
		}
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
	out := make([]Agent, 0, len(agents)-len(targets))
	for i, a := range agents {
		if _, drop := targets[i]; drop {
			continue
		}
		if parent, ok := aliases[a.ParentID]; ok {
			a.ParentID = parent
		}
		out = append(out, a)
	}
	return out
}
