package mcp

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestConfigTuneValueSchemaIsStringOrObject pins config.tune's published value
// schema to an explicit anyOf string/object. An untyped value made model-server
// tool-call parsers deliver agents.tier's object as a JSON-encoded string.
func TestConfigTuneValueSchemaIsStringOrObject(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "README.md", "# Test\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	server := NewServer(root, "test")
	props := toolPropertiesByName(t, callToolsList(t, server), "config.tune")
	value, ok := props["value"].(map[string]any)
	if !ok {
		t.Fatalf("config.tune schema missing value property: %v", props)
	}
	if _, hasType := value["type"]; hasType {
		t.Fatalf("config.tune value must carry its types in anyOf, not an outer type: %v", value)
	}
	description, _ := value["description"].(string)
	if !strings.Contains(description, "agents.tier") || !strings.Contains(description, "scalar knobs") {
		t.Fatalf("config.tune value description not preserved: %q", description)
	}
	want := []any{
		map[string]any{"type": "string"},
		map[string]any{"type": "object"},
	}
	if !reflect.DeepEqual(value["anyOf"], want) {
		encoded, _ := json.Marshal(value["anyOf"])
		t.Fatalf("config.tune value anyOf = %s, want exactly string and bare object", encoded)
	}
}

// TestConfigTuneRejectsNonStringValueForStringKeys covers the server side of the
// same contract: keys other than agents.tier read a string value, so any other
// present JSON type is an explicit error instead of a silent "" coercion, while
// absent and null values keep their existing handling.
func TestConfigTuneRejectsNonStringValueForStringKeys(t *testing.T) {
	useLeadProfile(t)
	rsrcRoot := filepath.Join("..", "..", "..", "agents-plugin", "rsrc")
	t.Setenv("WS_RSRC_ROOT", rsrcRoot)
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))

	s := NewServer(root, "test")
	s.observeHarness("test", "claude")
	key, _ := parseLoginResponse(t, callLogin(t, s, 910300, root, nil))

	id := 0
	call := func(args map[string]any) string {
		t.Helper()
		id++
		args["session_key"] = key
		return callToolOnce(t, s, id, "config.tune", args)
	}
	wantError := func(resp, substr string) {
		t.Helper()
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), substr) {
			t.Fatalf("want error containing %q, got: %s", substr, resp)
		}
	}
	wantOK := func(resp, substr string) {
		t.Helper()
		if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), substr) {
			t.Fatalf("want success containing %q, got: %s", substr, resp)
		}
	}
	object := map[string]any{"tier": "small"}

	// Writes with a non-string value name the key and the received JSON type.
	wantError(call(map[string]any{"key": "workflow.prefer_subagent", "value": object}),
		"workflow.prefer_subagent value must be a string; got object")
	wantError(call(map[string]any{"key": "workflow.prefer_subagent", "value": true}),
		"workflow.prefer_subagent value must be a string; got boolean")
	wantError(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "value": object}),
		"prompt.UserPreferenceSection value must be a string; got object")
	wantError(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "value": []any{"text"}}),
		"prompt.UserPreferenceSection value must be a string; got array")

	// Writes with null fall through to the existing empty-value errors.
	wantError(call(map[string]any{"key": "workflow.prefer_subagent", "value": nil}), `value must be one of`)
	wantError(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "value": nil}),
		"prompt must be non-empty")

	// String writes are unchanged, including enum lowercasing.
	wantOK(call(map[string]any{"key": "workflow.prefer_subagent", "value": " ON "}), "workflow.prefer_subagent: on [scope:global]")
	wantOK(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "value": "pref text"}),
		"prompt override set: UserPreferenceSection/claude (scope: session)")

	// Non-agents.tier resets reject a non-string value under the
	// mutual-exclusion framing, and treat null like an omitted value.
	wantError(call(map[string]any{"key": "workflow.prefer_subagent", "reset": true, "value": object}),
		"value and reset are mutually exclusive; workflow.prefer_subagent reset takes no value, got object")
	wantError(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "reset": true, "value": 1.0}),
		"value and reset are mutually exclusive; prompt.UserPreferenceSection reset takes no value, got number")
	wantOK(call(map[string]any{"key": "workflow.prefer_subagent", "reset": true, "value": nil}),
		"workflow.prefer_subagent: off [scope:builtin]")
	wantOK(call(map[string]any{"key": "prompt.UserPreferenceSection", "harness": "claude", "scope": "session", "reset": true, "value": nil}),
		"prompt override cleared: UserPreferenceSection/claude (scope: session)")

	// agents.tier still takes its object value for write and reset.
	wantOK(call(map[string]any{"key": "agents.tier", "value": map[string]any{"tier": "medium", "backend": "codex", "model": "m"}}), `"model":"m"`)
	wantOK(call(map[string]any{"key": "agents.tier", "reset": true, "value": map[string]any{"tier": "medium"}}), `"warnings"`)
}
