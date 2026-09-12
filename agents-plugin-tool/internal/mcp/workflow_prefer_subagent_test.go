package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowPreferSubagentWriterProductionPath(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900005, root, nil))

	onResp := callToolOnce(t, s, 1, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}
	showOn := toolText(t, callToolOnce(t, s, 2, "config.list", map[string]any{}))
	if !strings.Contains(showOn, "workflow.prefer_subagent: on  [scope:global]") {
		t.Fatalf("config.show must report workflow.prefer_subagent on/global: %s", showOn)
	}

	offResp := callToolOnce(t, s, 3, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "workflow.prefer_subagent: off [scope:global]") {
		t.Fatalf("subagent off call must succeed: %s", offResp)
	}
	showOff := toolText(t, callToolOnce(t, s, 4, "config.list", map[string]any{}))
	if !strings.Contains(showOff, "workflow.prefer_subagent: off  [scope:global]") {
		t.Fatalf("config.show must report workflow.prefer_subagent off/global: %s", showOff)
	}
}

// TestWorkflowPreferSubagentResetRestoresBuiltin verifies the reset-to-builtin
// unset path required by ticket 260702-bug-config-unset-asymmetry: reset:true
// removes the global override (rather than writing an explicit "off" value
// that would keep shadowing a future builtin default change), and config.show
// reports the value as builtin-sourced afterward.
func TestWorkflowPreferSubagentResetRestoresBuiltin(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900007, root, nil))

	onResp := callToolOnce(t, s, 1, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	resetResp := callToolOnce(t, s, 2, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"reset":       true,
	})
	resetText := toolText(t, resetResp)
	if !strings.Contains(resetText, "workflow.prefer_subagent: off [scope:builtin]") {
		t.Fatalf("reset must report the builtin-sourced value, not a re-shadowed global write: %s", resetText)
	}

	showAfterReset := toolText(t, callToolOnce(t, s, 3, "config.list", map[string]any{}))
	if !strings.Contains(showAfterReset, "workflow.prefer_subagent: off  [scope:builtin]") {
		t.Fatalf("config.show must report workflow.prefer_subagent as builtin-sourced after reset: %s", showAfterReset)
	}

	// reset and an explicit value are mutually exclusive.
	conflictResp := callToolOnce(t, s, 4, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "on",
		"reset":       true,
	})
	if !strings.Contains(conflictResp, `"isError":true`) {
		t.Fatalf("value+reset together must error: %s", conflictResp)
	}
	if msg := toolText(t, conflictResp); !strings.Contains(msg, "mutually exclusive") {
		t.Fatalf("value+reset error message must explain mutual exclusivity: %s", msg)
	}
}

func TestWorkflowPreferSubagentWorkflowManualPrintProductionPath(t *testing.T) {
	useLeadProfile(t)
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_SKILLS_ROOT", filepath.Join(t.TempDir(), "missing-skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)

	s := NewServer(root, "test")
	s.observeHarness("test", "codex")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900006, root, nil))

	offText := toolText(t, callToolOnce(t, s, 1, "playbook.read", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if strings.Contains(offText, "Use `ws:lead-delegate` for bounded investigation") {
		t.Fatalf("builtin/off workflow.prefer_subagent must not append lead-prefer-subagent:\n%s", offText)
	}

	onResp := callToolOnce(t, s, 2, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	onText := toolText(t, callToolOnce(t, s, 3, "playbook.read", map[string]any{
		"name": "lead-workflow-manual",
	}))
	for _, want := range []string{
		"Use `ws:lead-delegate` for bounded investigation",
		"ws:lead-delegate",
		"low-impact operational work",
	} {
		if !strings.Contains(onText, want) {
			t.Fatalf("prefer-subagent manual render missing %q:\n%s", want, onText)
		}
	}
	for _, retired := range []string{"lead-prefer-subagent", "Maximum-delegation posture", "\n# Delegate\n", "## Assignment"} {
		if strings.Contains(onText, retired) {
			t.Fatalf("manual must contain only an invocation hint, found %q", retired)
		}
	}
	if strings.Contains(onText, "ws:override:") || strings.Contains(onText, "ws:/override:") {
		t.Fatalf("prefer-subagent append must render through override marker stripping:\n%s", onText)
	}

	offResp := callToolOnce(t, s, 4, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "workflow.prefer_subagent: off [scope:global]") {
		t.Fatalf("subagent off call must succeed: %s", offResp)
	}
	offAgainText := toolText(t, callToolOnce(t, s, 5, "playbook.read", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if strings.Contains(offAgainText, "Use `ws:lead-delegate` for bounded investigation") {
		t.Fatalf("global off workflow.prefer_subagent must remove appended lead-prefer-subagent:\n%s", offAgainText)
	}
}

func TestWorkflowPreferSubagentWorkflowManualClaudeGetsInvocationHint(t *testing.T) {
	useLeadProfile(t)
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_SKILLS_ROOT", filepath.Join(t.TempDir(), "missing-skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900007, root, nil))

	onResp := callToolOnce(t, s, 1, "config.tune", map[string]any{
		"session_key": key,
		"key":         "workflow.prefer_subagent",
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	text := toolText(t, callToolOnce(t, s, 2, "playbook.read", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if !strings.Contains(text, "Use `ws:lead-delegate` for bounded investigation") {
		t.Fatalf("prefer-subagent manual render must add invocation hint for Claude:\n%s", text)
	}
	for _, want := range []string{
		"ws:lead-delegate",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Claude manual must include invocation hint %q:\n%s", want, text)
		}
	}
}
