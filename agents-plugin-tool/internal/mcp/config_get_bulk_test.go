package mcp

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Compare wire payloads, not resolver structs: this also checks omitted warnings,
// explicit null values, object types, and the absence of success fields on errors.
func assertConfigBulkParity(t *testing.T, server *Server, sessionKey string, keys []string) []map[string]any {
	t.Helper()
	resp := keyedCall(t, server, 951000, sessionKey, "config.get", map[string]any{"keys": keys, "format": "json"})
	if toolIsError(t, resp) {
		t.Fatalf("bulk config.get failed: %s", resp)
	}
	var envelope struct {
		Mode    string           `json:"mode"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(toolText(t, resp)), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Mode != "resolved-v1" || len(envelope.Results) != len(keys) {
		t.Fatalf("bulk envelope = %+v, want resolved-v1 and %d rows", envelope, len(keys))
	}
	var wantText strings.Builder
	for i, key := range keys {
		single := keyedCall(t, server, 951001+i, sessionKey, "config.get", map[string]any{"key": key, "format": "json"})
		var want map[string]any
		if toolIsError(t, single) {
			message := toolText(t, single)
			want = map[string]any{"key": key, "ok": false, "error": map[string]any{"message": message}}
			wantText.WriteString(key + ": " + strings.Join(strings.Fields(message), " ") + "\n")
		} else {
			if err := json.Unmarshal([]byte(toolText(t, single)), &want); err != nil {
				t.Fatal(err)
			}
			want["ok"] = true
			textResp := keyedCall(t, server, 952001+i, sessionKey, "config.get", map[string]any{"key": key})
			if toolIsError(t, textResp) {
				t.Fatalf("single text failed: %s", textResp)
			}
			wantText.WriteString(strings.ReplaceAll(strings.TrimSuffix(toolText(t, textResp), "\n"), "\n", " ") + "\n")
		}
		if !reflect.DeepEqual(envelope.Results[i], want) {
			t.Errorf("row %d (%s) = %#v, want single-key payload %#v", i, key, envelope.Results[i], want)
		}
	}
	textResp := keyedCall(t, server, 953000, sessionKey, "config.get", map[string]any{"keys": keys})
	if toolIsError(t, textResp) || toolText(t, textResp) != wantText.String() {
		t.Fatalf("bulk text = %q, want %q; response: %s", toolText(t, textResp), wantText.String(), textResp)
	}
	if lines := strings.Count(toolText(t, textResp), "\n"); lines != len(keys) {
		t.Fatalf("bulk text has %d lines, want %d", lines, len(keys))
	}
	return envelope.Results
}

func TestConfigGetBulkRequestValidation(t *testing.T) {
	server, _, key := manifestServer(t)
	cases := []struct {
		name string
		args map[string]any
	}{
		{"both", map[string]any{"key": "acme.limit", "keys": []string{"acme.limit"}}},
		{"neither", map[string]any{}},
		{"empty", map[string]any{"keys": []string{}}},
		{"duplicate", map[string]any{"keys": []string{"acme.limit", "acme.limit"}}},
		{"trimmed_duplicate", map[string]any{"keys": []string{"acme.limit", " acme.limit "}}},
		{"nonstring", map[string]any{"keys": []any{"acme.limit", 42}}},
		{"null_entry", map[string]any{"keys": []any{"acme.limit", nil}}},
		{"blank", map[string]any{"keys": []string{"acme.limit", " \t\n"}}},
		{"empty_entry", map[string]any{"keys": []string{""}}},
		{"not_array", map[string]any{"keys": "acme.limit"}},
		{"null_array", map[string]any{"keys": nil}},
		{"blank_single", map[string]any{"key": " \t"}},
		{"nonstring_single", map[string]any{"key": 42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.args["format"] = "json"
			resp := keyedCall(t, server, 954000, key, "config.get", tc.args)
			if !toolIsError(t, resp) || !strings.HasPrefix(toolText(t, resp), "config.get:") || strings.Contains(toolText(t, resp), `"mode":"resolved-v1"`) {
				t.Fatalf("malformed request must fail at request level: %s", resp)
			}
		})
	}
}

func TestConfigGetBulkLegacySingleBytes(t *testing.T) {
	server, root, key := manifestServer(t)
	mustWrite(t, root, ".ws-workflow/config.json", `{"overrides":{"acme.animate":"false","acme.label":"  spaced  "}}`)
	cases := []struct{ key, json, text string }{
		{"acme.limit", `{"key":"acme.limit","value":3,"scope":"builtin"}` + "\n", "acme.limit: 3 [scope:builtin]\n"},
		{"acme.animate", `{"key":"acme.animate","value":false,"scope":"repo"}` + "\n", "acme.animate: false [scope:repo]\n"},
		{"acme.label", `{"key":"acme.label","value":"  spaced  ","scope":"repo"}` + "\n", "acme.label:   spaced   [scope:repo]\n"},
		{"custom.absent", `{"key":"custom.absent","value":null,"scope":"unset"}` + "\n", "custom.absent: <unset>\n"},
	}
	for _, tc := range cases {
		for _, format := range []string{"json", ""} {
			args := map[string]any{"key": tc.key}
			want := tc.text
			if format != "" {
				args["format"] = format
				want = tc.json
			}
			resp := keyedCall(t, server, 955000, key, "config.get", args)
			if toolIsError(t, resp) || toolText(t, resp) != want {
				t.Fatalf("legacy %s (%s) = %q, want exact %q", tc.key, format, toolText(t, resp), want)
			}
		}
	}
}

func TestConfigGetBulkScopePrecedence(t *testing.T) {
	server, _, key := manifestServer(t)
	for i, scope := range []string{"builtin", "global", "repo", "project", "session"} {
		if scope != "builtin" {
			resp := keyedCall(t, server, 956000+i, key, "config.tune", map[string]any{"key": "acme.limit", "value": string(rune('3' + i)), "scope": scope})
			if toolIsError(t, resp) {
				t.Fatalf("fixture tune: %s", resp)
			}
		}
		rows := assertConfigBulkParity(t, server, key, []string{"custom.absent", "acme.limit"})
		if rows[1]["scope"] != scope || rows[1]["value"] != float64(3+i) {
			t.Fatalf("scope step %s = %#v", scope, rows[1])
		}
	}
}

func TestConfigGetBulkTypesWarningsAndStructured(t *testing.T) {
	server, root, key := manifestServer(t)
	server.sessionHarness = "pi"
	mustWrite(t, root, ".ws-workflow/config.json", `{
		"overrides":{"acme.limit":"lots","acme.animate":"false","acme.label":"  spaced  ","acme.mode":"fast","custom.flag":"x","workflow.whiteboard_enabled.pi":"on","workflow.whiteboard_enabled.all":"off"},
		"structured_overrides":{"agent.profile.bulk":{"backend":"pi"}}
	}`)
	keys := []string{"acme.mode", "custom.absent", "agent.profile.bulk", "acme.limit", "acme.animate", "acme.label", "custom.flag", "review_phase", "workflow.whiteboard_enabled", "acme.retention_days", "worktree.profile.full"}
	rows := assertConfigBulkParity(t, server, key, keys)
	if rows[3]["value"] != float64(3) || rows[3]["scope"] != "builtin" || len(rows[3]["warnings"].([]any)) != 1 {
		t.Fatalf("invalid stored integer must use typed default with warning: %#v", rows[3])
	}
	if rows[4]["value"] != false || rows[5]["value"] != "  spaced  " || rows[0]["value"] != "fast" || rows[6]["value"] != "x" {
		t.Fatalf("typed/string rows = %#v", rows)
	}
	if value, ok := rows[2]["value"].(map[string]any); !ok || value["backend"] != "pi" || rows[2]["scope"] != "repo" {
		t.Fatalf("structured profile = %#v", rows[2])
	}
	if rows[8]["value"] != "on" || rows[8]["scope"] != "repo" {
		t.Fatalf("harness bucket = %#v", rows[8])
	}
	if _, ok := rows[10]["value"].(map[string]any); !ok {
		t.Fatalf("builtin worktree profile must remain an object: %#v", rows[10])
	}
	assertConfigBulkParity(t, server, "", []string{"acme.limit", "custom.absent", "agent.profile.bulk"})
}

func TestConfigGetBulkResolverErrorIsolation(t *testing.T) {
	server, root, key := manifestServer(t)
	mustWrite(t, root, ".ws-workflow/config.json", `{"overrides":{"acme.animate":"false"},"structured_overrides":{"agent.profile.broken":{"backend":"gpt"}}}`)
	rows := assertConfigBulkParity(t, server, key, []string{"acme.animate", "agent.profile.broken", "acme.limit"})
	if rows[0]["ok"] != true || rows[1]["ok"] != false || rows[2]["ok"] != true {
		t.Fatalf("resolver error must not fail successful siblings: %#v", rows)
	}
	message := rows[1]["error"].(map[string]any)["message"].(string)
	if !strings.HasPrefix(message, "config.get: ") || !strings.Contains(message, "backend") {
		t.Fatalf("wrapped resolver error = %q", message)
	}
}

func TestConfigGetBulkDelegateAndLeafAuthorization(t *testing.T) {
	server, root, key := manifestServer(t)
	resp := keyedCall(t, server, 957000, key, "config.tune", map[string]any{"key": "acme.limit", "value": "8", "scope": "session"})
	if toolIsError(t, resp) {
		t.Fatalf("fixture tune: %s", resp)
	}
	delegate, leaf := "config-bulk-delegate", "config-bulk-leaf"
	writeSessionRecordForTest(t, server.sessions, delegate, root, roleDelegate, key)
	writeSessionRecordForTest(t, server.sessions, leaf, root, roleLeaf, delegate)
	for _, child := range []string{delegate, leaf} {
		rows := assertConfigBulkParity(t, server, child, []string{"acme.limit", "custom.absent"})
		if rows[0]["value"] != float64(8) || rows[0]["scope"] != "session" {
			t.Fatalf("%s did not inherit lead session override: %#v", child, rows[0])
		}
		for _, tool := range []string{"config.list", "config.tune"} {
			resp := keyedCall(t, server, 957001, child, tool, map[string]any{})
			var wire struct {
				Error *json.RawMessage `json:"error"`
			}
			if err := json.Unmarshal([]byte(resp), &wire); err != nil {
				t.Fatal(err)
			}
			if wire.Error == nil && !toolIsError(t, resp) {
				t.Fatalf("%s unexpectedly authorized %s: %s", child, tool, resp)
			}
		}
	}
	// Legacy single reads tolerate an unknown optional session key and use
	// non-session defaults; bulk must preserve that authorization behavior.
	assertConfigBulkParity(t, server, "config-bulk-unknown-key", []string{"acme.limit", "custom.absent"})
}
