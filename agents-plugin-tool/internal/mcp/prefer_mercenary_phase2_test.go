package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestPreferMercenaryOnOffRenderGuidance(t *testing.T) {
	root := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	s := newTestServerWithHarness(t, "claude")

	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(wsconfig.Options{CacheHome: t.TempDir(), ConfigHome: t.TempDir()}, builtinConfigDefaults(), adapter, adapter)

	if err := resolver.Set(wsconfig.ItemWorkflowPreferMercenary, "on", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("enable workflow.prefer_mercenary: %v", err)
	}
	enabled, err := resolver.Get("", wsconfig.ItemWorkflowPreferMercenary)
	if err != nil {
		t.Fatalf("get after enable: %v", err)
	}
	if enabled.Value != "on" || enabled.Scope != wsconfig.ScopeGlobal {
		t.Fatalf("after enable: got=%s scope=%s, want on/global", enabled.Value, enabled.Scope)
	}

	bodyOn, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", canonicalPreferMercenaryValue(enabled.Value) == "on", "", nil)
	if err != nil {
		t.Fatalf("renderPlaybookBody on: %v", err)
	}
	if !strings.Contains(bodyOn, "prefer_mercenary active") {
		t.Errorf("guidance block must be present after enable:\n%s", bodyOn)
	}

	if err := resolver.Set(wsconfig.ItemWorkflowPreferMercenary, "off", wsconfig.SetOptions{}); err != nil {
		t.Fatalf("disable workflow.prefer_mercenary: %v", err)
	}
	disabled, err := resolver.Get("", wsconfig.ItemWorkflowPreferMercenary)
	if err != nil {
		t.Fatalf("get after disable: %v", err)
	}
	if disabled.Value != "off" || disabled.Scope != wsconfig.ScopeGlobal {
		t.Fatalf("after disable: got=%s scope=%s, want off/global", disabled.Value, disabled.Scope)
	}

	bodyOff, _, err := renderPlaybookBody(s, root, "impl-pb", nil, wsconfig.Options{}, "", "", canonicalPreferMercenaryValue(disabled.Value) == "on", "", nil)
	if err != nil {
		t.Fatalf("renderPlaybookBody off: %v", err)
	}
	if strings.Contains(bodyOff, "prefer_mercenary active") {
		t.Errorf("guidance block must be absent after disable:\n%s", bodyOff)
	}
}

func TestPreferMercenaryOnOffRenderGuidanceProductionPath(t *testing.T) {
	useLeadProfile(t)
	rsrcRoot := buildTestRsrcTree(t, map[string]string{
		"impl-pb/impl-pb.md": implementerPlaybookContent,
	})
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900002, root, nil))

	enableResp := callToolOnce(t, s, 1, "config.workflow_prefer_mercenary", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, enableResp), "workflow.prefer_mercenary: on [scope:global]") {
		t.Fatalf("enable call must succeed: %s", enableResp)
	}

	renderResp1 := callToolOnce(t, s, 2, "playbook.render", map[string]any{
		"name":        "impl-pb",
		"session_key": key,
	})
	renderedPath1 := strings.SplitN(strings.TrimSpace(toolText(t, renderResp1)), "\n", 2)[0]
	renderedBody1, err := os.ReadFile(renderedPath1)
	if err != nil {
		t.Fatalf("read rendered playbook (enabled): %v", err)
	}
	if !strings.Contains(string(renderedBody1), "prefer_mercenary active") {
		t.Errorf("guidance block must be present after enable via production path:\n%s", string(renderedBody1))
	}

	disableResp := callToolOnce(t, s, 3, "config.workflow_prefer_mercenary", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, disableResp), "workflow.prefer_mercenary: off [scope:global]") {
		t.Fatalf("disable call must succeed: %s", disableResp)
	}

	renderResp2 := callToolOnce(t, s, 4, "playbook.render", map[string]any{
		"name":        "impl-pb",
		"session_key": key,
	})
	renderedPath2 := strings.SplitN(strings.TrimSpace(toolText(t, renderResp2)), "\n", 2)[0]
	renderedBody2, err := os.ReadFile(renderedPath2)
	if err != nil {
		t.Fatalf("read rendered playbook (disabled): %v", err)
	}
	if strings.Contains(string(renderedBody2), "prefer_mercenary active") {
		t.Errorf("guidance block must be absent after disable via production path:\n%s", string(renderedBody2))
	}

	hideResp := callToolOnce(t, s, 5, "config.workflow_prefer_mercenary", map[string]any{
		"session_key": key,
		"value":       "hide",
	})
	if !strings.Contains(toolText(t, hideResp), "workflow.prefer_mercenary: hide [scope:global]") {
		t.Fatalf("hide call must succeed: %s", hideResp)
	}
	listResp := callToolsList(t, s)
	if strings.Contains(listResp, `"name":"mercenary.call"`) || strings.Contains(listResp, `"name":"mercenary.register"`) {
		t.Fatalf("keyless tools/list must re-hide ws.mercenary.* after explicit hide: %s", listResp)
	}
}

func TestWorkflowPreferMercenaryRejectsLegacyEnabledShape(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900003, root, nil))
	resp := callToolOnce(t, s, 1, "config.workflow_prefer_mercenary", map[string]any{
		"session_key": key,
		"enabled":     false,
	})
	if !toolIsError(t, resp) {
		t.Fatalf("legacy enabled shape must be rejected: %s", resp)
	}
	if !strings.Contains(toolText(t, resp), "value must be one of on, off, hide") {
		t.Fatalf("legacy enabled rejection should mention canonical value enum: %s", resp)
	}
}

func TestPreferMercenaryConfigShowReportsGlobalWorkflowKeys(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900004, root, nil))

	showDefault := callToolOnce(t, s, 1, "config.show", map[string]any{})
	defaultText := toolText(t, showDefault)
	for _, want := range []string{
		"workflow.prefer_subagent: off  [scope:builtin]",
		"workflow.prefer_mercenary: hide  [scope:builtin]",
	} {
		if !strings.Contains(defaultText, want) {
			t.Fatalf("config.show default missing %q:\n%s", want, defaultText)
		}
	}

	setResp := callToolOnce(t, s, 2, "config.workflow_prefer_mercenary", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, setResp), "workflow.prefer_mercenary: on [scope:global]") {
		t.Fatalf("set call did not succeed: %s", setResp)
	}

	showGlobal := callToolOnce(t, s, 3, "config.show", map[string]any{})
	globalText := toolText(t, showGlobal)
	if !strings.Contains(globalText, "workflow.prefer_mercenary: on  [scope:global]") {
		t.Fatalf("config.show must report global workflow.prefer_mercenary: %s", globalText)
	}
	if strings.Contains(globalText, "prefer_mercenary =") {
		t.Fatalf("config.show must not surface orphaned unprefixed prefer_mercenary as a registered item: %s", globalText)
	}
}

func TestWorkflowPreferSubagentWriterProductionPath(t *testing.T) {
	useLeadProfile(t)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900005, root, nil))

	onResp := callToolOnce(t, s, 1, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}
	showOn := toolText(t, callToolOnce(t, s, 2, "config.show", map[string]any{}))
	if !strings.Contains(showOn, "workflow.prefer_subagent: on  [scope:global]") {
		t.Fatalf("config.show must report workflow.prefer_subagent on/global: %s", showOn)
	}

	offResp := callToolOnce(t, s, 3, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "workflow.prefer_subagent: off [scope:global]") {
		t.Fatalf("subagent off call must succeed: %s", offResp)
	}
	showOff := toolText(t, callToolOnce(t, s, 4, "config.show", map[string]any{}))
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

	onResp := callToolOnce(t, s, 1, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	resetResp := callToolOnce(t, s, 2, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"reset":       true,
	})
	resetText := toolText(t, resetResp)
	if !strings.Contains(resetText, "workflow.prefer_subagent: off [scope:builtin]") {
		t.Fatalf("reset must report the builtin-sourced value, not a re-shadowed global write: %s", resetText)
	}

	showAfterReset := toolText(t, callToolOnce(t, s, 3, "config.show", map[string]any{}))
	if !strings.Contains(showAfterReset, "workflow.prefer_subagent: off  [scope:builtin]") {
		t.Fatalf("config.show must report workflow.prefer_subagent as builtin-sourced after reset: %s", showAfterReset)
	}

	// reset and an explicit value are mutually exclusive.
	conflictResp := callToolOnce(t, s, 4, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
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
	t.Setenv("WS_SKILLS_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)

	s := NewServer(root, "test")
	s.observeHarness("test", "codex")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900006, root, nil))

	offText := toolText(t, callToolOnce(t, s, 1, "playbook.print", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if strings.Contains(offText, `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`) {
		t.Fatalf("builtin/off workflow.prefer_subagent must not append lead-prefer-subagent:\n%s", offText)
	}

	onResp := callToolOnce(t, s, 2, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	onText := toolText(t, callToolOnce(t, s, 3, "playbook.print", map[string]any{
		"name": "lead-workflow-manual",
	}))
	for _, want := range []string{
		`<playbook name="lead-prefer-subagent" title="Prefer Subagent">`,
		"Maximum-delegation posture for this session",
		"spawn_agent(fork_context:true, message:<prompt>)",
		"</playbook>",
	} {
		if !strings.Contains(onText, want) {
			t.Fatalf("prefer-subagent manual render missing %q:\n%s", want, onText)
		}
	}
	if strings.Contains(onText, "ws:override:") || strings.Contains(onText, "ws:/override:") {
		t.Fatalf("prefer-subagent append must render through override marker stripping:\n%s", onText)
	}

	offResp := callToolOnce(t, s, 4, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "off",
	})
	if !strings.Contains(toolText(t, offResp), "workflow.prefer_subagent: off [scope:global]") {
		t.Fatalf("subagent off call must succeed: %s", offResp)
	}
	offAgainText := toolText(t, callToolOnce(t, s, 5, "playbook.print", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if strings.Contains(offAgainText, `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`) {
		t.Fatalf("global off workflow.prefer_subagent must remove appended lead-prefer-subagent:\n%s", offAgainText)
	}
}

// TestWorkflowPreferSubagentWorkflowManualClaudeGetsStaticSkillBody verifies
// that the lead-prefer-subagent append is now a static SKILL.md body loaded
// via wsrsrc.LoadSkillBody, identical across harnesses. This replaces the
// prior per-harness override-marker divergence test
// (TestWorkflowPreferSubagentWorkflowManualClaudeOmitsCodexGuidance): the
// PreferSubagentInvocationGuidance override point and its Codex-only builtin
// default were retired when the skill body was inlined, so Claude and Codex
// now both see the same host-conditional prose, including the literal
// spawn_agent fallback wording.
func TestWorkflowPreferSubagentWorkflowManualClaudeGetsStaticSkillBody(t *testing.T) {
	useLeadProfile(t)
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	t.Setenv("WS_SKILLS_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "skills"))
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900007, root, nil))

	onResp := callToolOnce(t, s, 1, "config.workflow_prefer_subagent", map[string]any{
		"session_key": key,
		"value":       "on",
	})
	if !strings.Contains(toolText(t, onResp), "workflow.prefer_subagent: on [scope:global]") {
		t.Fatalf("subagent on call must succeed: %s", onResp)
	}

	text := toolText(t, callToolOnce(t, s, 2, "playbook.print", map[string]any{
		"name": "lead-workflow-manual",
	}))
	if !strings.Contains(text, `<playbook name="lead-prefer-subagent" title="Prefer Subagent">`) {
		t.Fatalf("prefer-subagent manual render must append wrapper for Claude:\n%s", text)
	}
	for _, want := range []string{
		"Maximum-delegation posture for this session",
		"spawn_agent(fork_context:true, message:<prompt>)",
		"`agent_type: explorer`",
		"`agent_type: worker`",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Claude appended playbook must include static skill body %q:\n%s", want, text)
		}
	}
}
