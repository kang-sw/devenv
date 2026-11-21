package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

func isCompactionKey(key string) bool { _, ok := wsconfig.CompactionDefaults[key]; return ok }
func isCompactionTuneKey(key string) bool {
	if isCompactionKey(key) {
		return true
	}
	switch key {
	case "claude.compaction_advisory_enabled",
		"claude.compaction_advisory_percent",
		"pi.compaction_advisory_percent",
		"pi.compaction_hard_percent":
		return true
	}
	return false
}
func notifyCompactionTune(key string) {
	if isCompactionTuneKey(key) {
		wsagent.NotifyCompactionConfigChanged()
	}
}
func compactionKeyEntry(key string) configKeyEntry {
	return configKeyEntry{Key: key, WriterTool: "config.tune", ResetTool: "config.tune", ResolverBacked: true, HarnessApplicable: true, NoAgentVisible: true,
		SelectorFields: []tuningField{{Name: "harness", Description: "Harness override; * writes the shared layer value.", Enum: promptHarnessEnum}, {Name: "scope", Enum: wsconfig.ScopeSchemaEnum()}},
		ValueFields:    []tuningField{{Name: "value", Description: "Percent (1-100), tokens (150k, 1.5m), or one of each joined by |; whichever occurs first."}},
		Normalize:      func(value string) (string, error) { _, err := wsconfig.ParseThreshold(value); return value, err },
	}
}
func (s *Server) compactionValue(r *wsconfig.Resolver, key, item, harness string) (wsconfig.ResolvedValue, string, error) {
	return resolveCompactionValue(r, key, item, harness, s.agentBackends()...)
}
func resolveCompactionValue(r *wsconfig.Resolver, key, item, harness string, registries ...map[string]wsagent.BackendRegistration) (wsconfig.ResolvedValue, string, error) {
	p := wsagent.BuiltinCompaction(harness)
	for _, regs := range registries {
		if reg, ok := regs[harness]; ok {
			p = reg.CompactionProtocol()
			break
		}
	}
	return r.Compaction(key, item, harness, p.LegacyKey(item))
}

// compactTool is discoverable before use; an older daemon refuses the unknown
// tool. It never falls back to sending a slash prompt from a caller.
func compactTool() map[string]any {
	return map[string]any{"name": "compact", "description": "Queue native compaction at the hosted session's next idle boundary. Write durable state to your agenda first. Returns queued, unsupported or error with a reason; queued is not completion. Discover this tool before calling an older daemon.", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{
		"session_key": map[string]any{"type": "string", "description": "Your ws session key."},
		"target":      map[string]any{"type": "string", "description": "Optional self key or owned descendant key / spawn address id:<id>; defaults to self."},
		"resume":      map[string]any{"type": "boolean", "description": "Start one continue turn after confirmed compaction and restoration. Defaults true for self, false for a descendant."},
		"format":      map[string]any{"type": "string", "enum": []string{"text", "json"}},
	}, "required": []string{"session_key"}}}
}

type compactRunner interface {
	Compact(string, string, *bool) wsagent.CompactResult
}

func (s *Server) handleCompact(ctx context.Context, id json.RawMessage, args map[string]any) response {
	key, _ := args["session_key"].(string)
	key = strings.TrimSpace(key)
	if _, ok := s.lookupSession(key); !ok {
		return toolTextResponse(id, "", fmt.Errorf("compact: unknown_session"))
	}
	target, _ := args["target"].(string)
	var resume *bool
	if b, ok := args["resume"].(bool); ok {
		resume = &b
	}
	out := wsagent.CompactResult{Status: "unsupported", Reason: "native compaction requires a hosted session"}
	if runner, ok := s.pi.agents.(compactRunner); ok {
		out = runner.Compact(key, target, resume)
	} else if s.piForwards() {
		// The token route limits this request to the token's own key.
		if err := s.piCall(ctx, "compact", map[string]any{"target": target, "resume": resume}, piForwardTimeout, &out); err != nil {
			out = wsagent.CompactResult{Status: "error", Reason: err.Error()}
		}
	}
	if wantsJSON(args) {
		return toolJSONResponse(id, out, nil)
	}
	return toolTextResponse(id, fmt.Sprintf("%s: %s", out.Status, out.Reason), nil)
}
