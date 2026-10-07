package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestConfigTuneWorkflowLangKeepsCaseAndResets: workflow.lang is a free-form
// knob, so config.tune stores the language name as written (no lowercasing,
// no enum check), config.get/config.list report it, and reset returns it to
// unset.
func TestConfigTuneWorkflowLangKeepsCaseAndResets(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", "agents-plugin", "rsrc"))

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900700, root, nil))

	resp := keyedCall(t, s, 1, key, "config.tune", map[string]any{
		"key":   wsconfig.ItemWorkflowLang,
		"value": " Korean ",
	})
	if toolIsError(t, resp) {
		t.Fatalf("config.tune workflow.lang failed: %s", resp)
	}
	if got := toolText(t, resp); !strings.Contains(got, "workflow.lang: Korean [scope:global]") {
		t.Fatalf("config.tune workflow.lang response = %q", got)
	}

	got := configGetJSON(t, s, 2, map[string]any{"key": wsconfig.ItemWorkflowLang, "session_key": key})
	if got.Value != "Korean" || got.Scope != "global" {
		t.Fatalf("config.get workflow.lang = %+v, want Korean [scope:global]", got)
	}

	catalog := parseTuningCatalogResponse(t, keyedCall(t, s, 3, key, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, wsconfig.ItemWorkflowLang)
	if knob.Writer.Tool != "config.tune" || knob.Writer.FixedArguments["key"] != wsconfig.ItemWorkflowLang {
		t.Fatalf("workflow.lang writer mismatch: %+v", knob.Writer)
	}
	if knob.Reset == nil || knob.Reset.FixedArguments["reset"] != "true" {
		t.Fatalf("workflow.lang reset mismatch: %+v", knob.Reset)
	}
	assertFieldNoEnum(t, knob.ValueFields, "value")
	if current := mustMarshalJSON(t, knob.Current); !strings.Contains(current, `"value":"Korean"`) || !strings.Contains(current, `"scope":"global"`) {
		t.Fatalf("config.list workflow.lang current = %s, want Korean/global", current)
	}
	if !strings.Contains(knob.Description, "lead responses") || !strings.Contains(knob.Description, "summaries") {
		t.Fatalf("workflow.lang description must name lead responses and adapter summaries: %q", knob.Description)
	}

	if resp := keyedCall(t, s, 4, key, "config.tune", map[string]any{"key": wsconfig.ItemWorkflowLang, "value": "  "}); !toolIsError(t, resp) {
		t.Fatalf("config.tune accepted a blank workflow.lang value: %s", resp)
	}

	if resp := keyedCall(t, s, 5, key, "config.tune", map[string]any{"key": wsconfig.ItemWorkflowLang, "reset": true}); toolIsError(t, resp) {
		t.Fatalf("config.tune workflow.lang reset failed: %s", resp)
	}
	got = configGetJSON(t, s, 6, map[string]any{"key": wsconfig.ItemWorkflowLang, "session_key": key})
	if got.Value != "" || got.Scope != string(wsconfig.ScopeBuiltin) {
		t.Fatalf("config.get workflow.lang after reset = %+v, want unset (empty, builtin)", got)
	}
}

// TestConfigTuningCatalogNoAgentListsWorkflowLang: the agentless catalog keeps
// workflow.lang, since the wsflow lead-tune mirror routes language requests to it.
func TestConfigTuningCatalogNoAgentListsWorkflowLang(t *testing.T) {
	useLeadProfile(t)
	t.Setenv("WS_RSRC_ROOT", buildOverrideTestTree(t))
	t.Setenv("WS_MCP_NO_AGENT", "1")

	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900701, root, nil))

	catalog := parseTuningCatalogResponse(t, keyedCall(t, s, 1, key, "config.list", map[string]any{"format": "json"}))
	requireTuningKnob(t, catalog, wsconfig.ItemWorkflowLang)
	if resp := keyedCall(t, s, 2, key, "config.tune", map[string]any{"key": wsconfig.ItemWorkflowLang, "value": "Korean", "scope": "session"}); toolIsError(t, resp) {
		t.Fatalf("no-agent config.tune workflow.lang failed: %s", resp)
	}
}

// TestPlaybookPrintLeadTuneRoutesResponseLanguageToWorkflowLang pins the
// tune-target routing: a response-language preference goes to the
// workflow.lang knob, while style and wording stay with the prompt override.
func TestPlaybookPrintLeadTuneRoutesResponseLanguageToWorkflowLang(t *testing.T) {
	t.Setenv(envNoAgent, "")
	t.Setenv(envNamespace, "")
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	s := newTestServerWithHarness(t, "codex")

	body, _, err := printPlaybook(s, rsrcRoot, "lead-tune", nil, isolatedPlaybookConfigOptions(t), "", mustOverrideLookup(t, s, ""))
	if err != nil {
		t.Fatalf("printPlaybook lead-tune: %v", err)
	}
	var langLine, prefLine string
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.Contains(line, "-> scalar knob `workflow.lang`"):
			langLine = line
		case strings.Contains(line, "-> prompt override (`UserPreferenceSection`)"):
			prefLine = line
		}
	}
	if !strings.Contains(langLine, `"answer me in Korean"`) {
		t.Fatalf("lead-tune must route a response-language preference to workflow.lang; got line %q:\n%s", langLine, body)
	}
	if prefLine == "" || strings.Contains(prefLine, "language") {
		t.Fatalf("UserPreferenceSection routing must keep style/wording and drop language; got line %q", prefLine)
	}
	for _, want := range []string{"style", "terminology", "wording"} {
		if !strings.Contains(prefLine, want) {
			t.Fatalf("UserPreferenceSection routing line lost %q: %q", want, prefLine)
		}
	}
}
