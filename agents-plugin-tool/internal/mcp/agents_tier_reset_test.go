package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func tierTune(t *testing.T, s *Server, id int, args map[string]any) map[string]any {
	t.Helper()
	resp := callToolOnce(t, s, id, "config.tune", args)
	if toolIsError(t, resp) {
		t.Fatalf("config.tune failed: %s", resp)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(toolText(t, resp)), &result); err != nil {
		t.Fatalf("decode config.tune response: %v", err)
	}
	if _, ok := result["warnings"].([]any); !ok {
		t.Fatalf("agents.tier response has no warnings array: %#v", result)
	}
	return result
}

func tierWarnings(t *testing.T, result map[string]any) []any {
	t.Helper()
	return result["warnings"].([]any)
}

func TestConfigTuneAgentsTierResetAndShadow(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260923-feat-config-tune-agents-tier-reset")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	s := NewServer(root, "test")
	write := func(id int, scope, harness, model string) map[string]any {
		t.Helper()
		return tierTune(t, s, id, map[string]any{"key": "agents.tier", "scope": scope, "harness": harness,
			"value": map[string]any{"tier": "medium", "backend": "codex", "model": model}})
	}
	reset := func(id int, scope, harness string) map[string]any {
		t.Helper()
		return tierTune(t, s, id, map[string]any{"key": "agents.tier", "scope": scope, "harness": harness,
			"reset": true, "value": map[string]any{"tier": "core"}})
	}
	resolve := func(id int, harness, model string) {
		t.Helper()
		resp := callToolOnce(t, s, id, "config.resolve_agent", map[string]any{"tier": "medium", "harness": harness, "format": "json"})
		if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), `"model":"`+model+`"`) {
			t.Fatalf("resolve %s: %s, want %s", harness, resp, model)
		}
	}
	if w := tierWarnings(t, write(1, "global", "pi", "global-pi")); len(w) != 0 {
		t.Fatalf("unshadowed global write warned: %v", w)
	}
	write(2, "global", "claude", "global-claude")
	write(3, "project", "pi", "project-pi")
	write(4, "project", "claude", "project-claude")
	resolve(5, "pi", "project-pi")
	resolve(6, "claude", "project-claude")
	if w := tierWarnings(t, write(7, "global", "pi", "global-pi-2")); len(w) != 1 || !strings.Contains(w[0].(string), "project scope") {
		t.Fatalf("shadowed global write warnings = %v", w)
	}
	if w := tierWarnings(t, reset(8, "project", "pi")); len(w) != 0 {
		t.Fatalf("project reset warned: %v", w)
	}
	resolve(9, "pi", "global-pi-2")
	resolve(10, "claude", "project-claude")
	if w := tierWarnings(t, reset(11, "project", "pi")); len(w) != 1 || !strings.Contains(w[0].(string), "absent") {
		t.Fatalf("absent reset warnings = %v", w)
	}
	if w := tierWarnings(t, reset(12, "global", "claude")); len(w) != 1 || !strings.Contains(w[0].(string), "project scope") {
		t.Fatalf("shadowed global reset warnings = %v", w)
	}
	resolve(13, "claude", "project-claude")
	if w := tierWarnings(t, reset(14, "global", "pi")); len(w) != 0 {
		t.Fatalf("unshadowed global reset warned: %v", w)
	}
}

func TestConfigTuneAgentsTierResetValidationAndFallbackWarning(t *testing.T) {
	useLeadProfile(t)
	root := initTicketRepo(t, "260923-feat-config-tune-agents-tier-reset")
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	s := NewServer(root, "test")
	for i, field := range []string{"backend", "model", "effort"} {
		resp := callToolOnce(t, s, i+1, "config.tune", map[string]any{"key": "agents.tier", "reset": true,
			"value": map[string]any{"tier": "medium", field: ""}})
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "only tier") {
			t.Fatalf("reset with %s should be rejected: %s", field, resp)
		}
	}
	for _, value := range []any{nil, map[string]any{}, "medium"} {
		resp := callToolOnce(t, s, 10, "config.tune", map[string]any{"key": "agents.tier", "reset": true, "value": value})
		if !toolIsError(t, resp) {
			t.Fatalf("reset without tier object succeeded: %s", resp)
		}
	}
	tierTune(t, s, 11, map[string]any{"key": "agents.tier", "scope": "project", "harness": "default",
		"value": map[string]any{"tier": "medium", "model": "project-default"}})
	if w := tierWarnings(t, tierTune(t, s, 12, map[string]any{"key": "agents.tier", "scope": "global", "harness": "pi",
		"value": map[string]any{"tier": "medium", "model": "global-pi"}})); len(w) != 0 {
		t.Fatalf("harness fallback counted as shadow: %v", w)
	}
	if w := tierWarnings(t, tierTune(t, s, 13, map[string]any{"key": "agents.tier", "scope": "global", "harness": "pi",
		"reset": true, "value": map[string]any{"tier": "medium"}})); len(w) != 0 {
		t.Fatalf("harness fallback counted as reset shadow: %v", w)
	}
	projectPath, err := wsconfig.Path(wsconfig.Options{})
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"default"`) {
		t.Fatalf("project fallback leaf lost: %s", content)
	}
}

func TestLeadTuneRenderExplainsAgentsTierReset(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	root := initTicketRepo(t, "260923-feat-config-tune-agents-tier-reset")
	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 1, root, nil))
	resp := callToolOnce(t, s, 2, "playbook.render", map[string]any{"name": "lead-tune", "session_key": key})
	if toolIsError(t, resp) {
		t.Fatalf("playbook.render lead-tune: %s", resp)
	}
	path := strings.SplitN(toolText(t, resp), "\n", 2)[0]
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered lead-tune: %v", err)
	}
	for _, want := range []string{"reset removes only that scope's selected", "value` containing only `tier`", "Relay any warnings", "effort clears only effort"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("rendered lead-tune missing %q: %s", want, body)
		}
	}
}

func TestConfigCatalogAgentsTierResetWriter(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "global"))
	root := initTicketRepo(t, "260923-feat-config-tune-agents-tier-reset")
	s := NewServer(root, "test")
	resp := callToolOnce(t, s, 1, "config.list", map[string]any{"format": "json"})
	if toolIsError(t, resp) {
		t.Fatal(resp)
	}
	text := toolText(t, resp)
	var listing tuningCatalog
	if err := json.Unmarshal([]byte(text), &listing); err != nil {
		t.Fatal(err)
	}
	knob := requireTuningKnob(t, listing, "agents.tier")
	if knob.Reset == nil || knob.Reset.Tool != "config.tune" || knob.Reset.FixedArguments["reset"] != "true" {
		t.Fatalf("agents.tier reset writer missing: %+v", knob.Reset)
	}
	if !strings.Contains(knob.Description, "value.tier") {
		t.Fatalf("reset value guidance missing: %+v", knob)
	}
}
