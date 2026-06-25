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
	if strings.Contains(listResp, `"name":"ws.mercenary.call"`) || strings.Contains(listResp, `"name":"ws.mercenary.register"`) {
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
