package mcp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func newPrecheckServer(t *testing.T) (*Server, string) {
	t.Helper()
	useLeadProfile(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	server := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, server, 1, root, nil))
	return server, key
}

// precheckSchemaFromText parses the inputSchema JSON the refusal ends with.
func precheckSchemaFromText(t *testing.T, text string) map[string]any {
	t.Helper()
	const marker = "Retry with arguments matching this inputSchema:\n"
	idx := strings.Index(text, marker)
	if idx < 0 {
		t.Fatalf("refusal carries no inputSchema dump:\n%s", text)
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(text[idx+len(marker):])), &schema); err != nil {
		t.Fatalf("dumped inputSchema is not parseable JSON: %v\n%s", err, text)
	}
	return schema
}

func TestPrecheckRefusesMissingRequiredBeforeHandler(t *testing.T) {
	server, key := newPrecheckServer(t)

	// todo.add requires key and title; send neither.
	line := callToolLineWithKey(t, server, 2, key, "todo.add", map[string]any{})
	if !toolIsError(t, line) {
		t.Fatalf("todo.add without required arguments was not refused: %s", line)
	}
	text := toolText(t, line)
	if !strings.Contains(text, "missing required: key, title\n") {
		t.Fatalf("refusal does not list every missing required property:\n%s", text)
	}
	if strings.Contains(text, "unknown arguments") {
		t.Fatalf("session_key was reported as unknown:\n%s", text)
	}
	schema := precheckSchemaFromText(t, text)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["title"]; !ok {
		t.Fatalf("dumped schema is not todo.add's: %v", schema)
	}

	// The handler did not run: no todo was stored.
	if got := callToolWithKey(t, server, 3, key, "todo.list", map[string]any{"mode": "full"}); got != "(no todos)\n" {
		t.Fatalf("refused todo.add still ran its handler; todo.list = %q", got)
	}
}

func TestPrecheckRefusesUnknownArgumentAndDumpsOnlyCalledTool(t *testing.T) {
	server, key := newPrecheckServer(t)

	line := callToolLineWithKey(t, server, 2, key, "git.commit", map[string]any{
		"paths":           []string{"x"},
		"ai_context":      "c",
		"expected_branch": "main",
		"message":         "subject line",
	})
	if !toolIsError(t, line) {
		t.Fatalf("git.commit with an unknown argument was not refused: %s", line)
	}
	text := toolText(t, line)
	for _, want := range []string{
		"git.commit: invalid arguments; the tool did not run.\n",
		"missing required: title\n",
		"unknown arguments: message\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("refusal missing %q:\n%s", want, text)
		}
	}
	schema := precheckSchemaFromText(t, text)
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["expected_branch"]; !ok {
		t.Fatalf("dumped schema is not git.commit's: %v", schema)
	}
	if _, ok := schema["name"]; ok {
		t.Fatalf("dump is a tool definition, want only the inputSchema: %v", schema)
	}
	// Only the called tool's schema: no other tool name or schema appears.
	for _, other := range []string{"todo.add", "tickets.query", `"tools"`} {
		if strings.Contains(text, other) {
			t.Fatalf("refusal leaks %s beyond the called tool:\n%s", other, text)
		}
	}
}

func TestPrecheckLeavesValidCallsAndExemptionsAlone(t *testing.T) {
	server, key := newPrecheckServer(t)

	if got := callToolWithKey(t, server, 2, key, "todo.add", map[string]any{"key": "a", "title": "A"}); strings.Contains(got, "invalid arguments") {
		t.Fatalf("valid todo.add was refused: %s", got)
	}
	// session_key is accepted on a tool whose schema does not declare it.
	if line := callToolLineWithKey(t, server, 3, key, "runtime.read", nil); toolIsError(t, line) {
		t.Fatalf("runtime.read with session_key was refused: %s", line)
	}
	// A retired name still reaches the handler's migration hint.
	line := callToolLineWithKey(t, server, 4, key, "tickets.query", map[string]any{"spec_stem": "260504-demo"})
	if text := toolText(t, line); !strings.Contains(text, "tickets tools use ticket_stem, not spec_stem") {
		t.Fatalf("spec_stem did not reach the handler's hint: %s", text)
	}
}

func TestSchemaClosedRule(t *testing.T) {
	cases := []struct {
		schema map[string]any
		want   bool
	}{
		{map[string]any{"properties": map[string]any{}}, true},
		{map[string]any{"additionalProperties": false}, true},
		{map[string]any{"additionalProperties": true}, false},
		{map[string]any{"additionalProperties": map[string]any{"type": "string"}}, false},
	}
	for _, c := range cases {
		if got := schemaClosed(c.schema); got != c.want {
			t.Fatalf("schemaClosed(%v) = %v, want %v", c.schema, got, c.want)
		}
	}
}

// TestHandlerOwnedArgumentsStayUndeclared keeps the exemption list honest: an
// exempt name that a schema later declares should leave the list.
func TestHandlerOwnedArgumentsStayUndeclared(t *testing.T) {
	for tool, names := range handlerOwnedArguments {
		schema := toolInputSchema(tool)
		if schema == nil {
			t.Fatalf("handlerOwnedArguments names unknown tool %s", tool)
		}
		props, _ := schema["properties"].(map[string]any)
		for _, name := range names {
			if _, declared := props[name]; declared {
				t.Fatalf("%s declares %s; drop it from handlerOwnedArguments", tool, name)
			}
		}
	}
}
