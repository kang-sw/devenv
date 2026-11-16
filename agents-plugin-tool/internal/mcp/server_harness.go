package mcp

import (
	"encoding/json"
	"strings"
)

func codexWorkspaceRoots(meta map[string]any) []string {
	turn, ok := meta["x-codex-turn-metadata"].(map[string]any)
	if !ok {
		return nil
	}
	rawWorkspaces, ok := turn["workspaces"].(map[string]any)
	if !ok || len(rawWorkspaces) == 0 {
		return nil
	}
	roots := make([]string, 0, len(rawWorkspaces))
	for root := range rawWorkspaces {
		if strings.TrimSpace(root) != "" {
			roots = append(roots, root)
		}
	}
	return roots
}

func (s *Server) observeHarness(source, harness string) {
	harness = normalizedHarness(harness)
	if harness == "" {
		return
	}
	s.rootMu.Lock()
	defer s.rootMu.Unlock()
	if s.sessionHarness == "" {
		s.sessionHarness = harness
		s.debugEvent("harness.detected", map[string]any{"source": source, "harness": harness})
		return
	}
	if s.sessionHarness != harness {
		s.debugEvent("harness.conflict", map[string]any{
			"source":   source,
			"current":  s.sessionHarness,
			"observed": harness,
		})
	}
}

// observePiDaemonAgents records the client's explicit model-facing opt-in.
// Missing or malformed metadata never implies support.
func (s *Server) observePiDaemonAgents(raw json.RawMessage) {
	var params struct {
		Capabilities struct {
			Experimental struct {
				PiDaemonAgents bool `json:"wsPiDaemonAgents"`
			} `json:"experimental"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		params.Capabilities.Experimental.PiDaemonAgents = false
	}
	s.rootMu.Lock()
	s.piDaemonAgents = params.Capabilities.Experimental.PiDaemonAgents
	s.rootMu.Unlock()
}

func (s *Server) clientPiDaemonAgents() bool {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.piDaemonAgents
}

func (s *Server) currentHarness() string {
	s.rootMu.RLock()
	defer s.rootMu.RUnlock()
	return s.sessionHarness
}

// detectHarnessFromInitializeParams parses the "initialize" request's
// clientInfo.name via structured JSON first: an exact match on
// "ws-pi-bridge" identifies the Pi harness. Any other (or absent) clientInfo
// falls through to the substring-based detectHarnessFromRaw, which stays
// byte-identical for Codex/Claude detection.
func detectHarnessFromInitializeParams(raw json.RawMessage) string {
	var params struct {
		ClientInfo struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	if err := json.Unmarshal(raw, &params); err == nil {
		if strings.TrimSpace(params.ClientInfo.Name) == "ws-pi-bridge" {
			return "pi"
		}
	}
	return detectHarnessFromRaw(raw)
}

func detectHarnessFromRaw(raw json.RawMessage) string {
	text := strings.ToLower(string(raw))
	if text == "" {
		return ""
	}
	if strings.Contains(text, "x-codex-turn-metadata") || strings.Contains(text, "codex") {
		return "codex"
	}
	if strings.Contains(text, "claude") || strings.Contains(text, "anthropic") {
		return "claude"
	}
	return ""
}

func detectHarnessFromMeta(meta map[string]any) string {
	if len(codexWorkspaceRoots(meta)) > 0 {
		return "codex"
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return ""
	}
	return detectHarnessFromRaw(raw)
}

func normalizedHarness(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "codex":
		return "codex"
	case "claude":
		return "claude"
	case "pi":
		return "pi"
	default:
		return ""
	}
}
