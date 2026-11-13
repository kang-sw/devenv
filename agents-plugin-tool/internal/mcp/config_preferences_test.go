package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestPreferenceSlotsTuneAndRender(t *testing.T) {
	for _, pkg := range []string{"agents-plugin", "agents-plugin-wsflow"} {
		t.Run(pkg, func(t *testing.T) {
			useLeadProfile(t)
			t.Setenv("WS_RSRC_ROOT", filepath.Join("..", "..", "..", pkg, "rsrc"))
			if pkg == "agents-plugin-wsflow" {
				t.Setenv("WS_MCP_NO_AGENT", "1")
			}
			t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
			t.Setenv("WS_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
			root := t.TempDir()
			mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
			initGit(t, root)
			s := NewServer(root, "test")
			s.observeHarness("test", "codex")
			key, _ := parseLoginResponse(t, callLogin(t, s, 900800, root, nil))
			id := 1
			call := func(tool string, args map[string]any) string {
				t.Helper()
				id++
				resp := keyedCall(t, s, id, key, tool, args)
				if toolIsError(t, resp) {
					t.Fatalf("%s: %s", tool, resp)
				}
				return toolText(t, resp)
			}
			tune := func(k, value, scope string) string {
				t.Helper()
				return call("config.tune", map[string]any{"key": k, "value": value, "scope": scope})
			}
			reset := func(k, scope string) {
				t.Helper()
				call("config.tune", map[string]any{"key": k, "scope": scope, "reset": true})
			}
			section := func() string {
				t.Helper()
				body := call("playbook.read", map[string]any{"name": "lead-workflow-manual"})
				_, rest, ok := strings.Cut(body, "### User preferences\n")
				if !ok {
					t.Fatal("missing preference heading")
				}
				text, _, _ := strings.Cut(rest, "### Asking the user")
				return strings.TrimSpace(text)
			}
			const seed = "Use the project's terminology and communication style."
			if got := section(); got != seed {
				t.Fatalf("seed = %q", got)
			}
			if echo := tune("preference.z", "Last", ""); !strings.Contains(echo, "[scope:global]") {
				t.Fatal(echo)
			}
			tune("preference.a", "Global A", "global")
			tune("preference.b", "Repo B", "repo")
			tune("preference.c", "Project C", "project")
			tune("preference.d", "Session D", "session")
			tune("preference.a", "Session A", "session")
			wantOthers := "Session A\n\nRepo B\n\nProject C\n\nSession D\n\nLast"
			if got := section(); got != seed+"\n\n"+wantOthers {
				t.Fatalf("accumulation/order = %q", got)
			}
			call("config.tune", map[string]any{"key": "prompt.UserPreferenceSection", "harness": "*", "value": "Legacy", "scope": "global"})
			if got := section(); got != "Legacy\n\n"+wantOthers {
				t.Fatalf("legacy all fallback = %q", got)
			}
			call("config.tune", map[string]any{"key": "prompt.UserPreferenceSection", "harness": "codex", "value": "Harness legacy", "scope": "global"})
			tune("preference.default", "Default", "global")
			if got := section(); got != "Default\n\n"+wantOthers {
				t.Fatalf("default first = %q", got)
			}
			tune("workflow.lang", "Korean", "global")
			tune("preference.default", "", "session")
			got := section()
			if !strings.HasPrefix(got, wantOthers+"\n") || strings.Contains(got, "Default") || strings.Contains(got, "legacy") || strings.Count(got, "Respond to the user in Korean") != 1 {
				t.Fatalf("clear/language = %q", got)
			}
			tune("preference.a", "", "session")
			if got := section(); strings.Contains(got, "Session A") || strings.Contains(got, "Global A") {
				t.Fatalf("tombstone = %q", got)
			}
			reset("preference.a", "session")
			if got := section(); !strings.Contains(got, "Global A") {
				t.Fatalf("reset = %q", got)
			}
			reset("preference.default", "session")
			reset("preference.default", "global")
			if got := section(); !strings.HasPrefix(got, "Harness legacy\n\n") || !strings.Contains(got, "Respond to the user in Korean") {
				t.Fatalf("legacy/lang = %q", got)
			}
			reset("workflow.lang", "global")
			if got := section(); strings.Contains(got, "Respond to the user in") {
				t.Fatalf("unset lang = %q", got)
			}
			catalog := parseTuningCatalogResponse(t, keyedCall(t, s, 900801, key, "config.list", map[string]any{"format": "json"}))
			knob := requireTuningKnob(t, catalog, "preference.<slot>")
			if fieldEnum(knob.SelectorFields, "harness") != nil || !knob.RepoScope || knob.Reset == nil || !strings.Contains(mustMarshalJSON(t, knob.Current), `"key":"preference.z"`) {
				t.Fatalf("catalog = %+v", knob)
			}
			for _, args := range []map[string]any{
				{"key": "preference.", "value": "x"},
				{"key": "preference.Bad", "value": "x"},
				{"key": "preference.a.b", "value": "x"},
				{"key": "preference._bad", "value": "x"},
				{"key": "preference.valid"},
				{"key": "preference.valid", "value": nil},
				{"key": "preference.valid", "value": 2},
			} {
				id++
				if resp := keyedCall(t, s, id, key, "config.tune", args); !toolIsError(t, resp) {
					t.Fatalf("accepted %+v: %s", args, resp)
				}
			}
			for _, row := range []struct{ slot, scope string }{
				{"a", "global"}, {"b", "repo"}, {"c", "project"}, {"d", "session"}, {"z", "global"},
			} {
				reset("preference."+row.slot, row.scope)
			}
			tune("workflow.lang", "Korean", "global")
			if got := section(); !strings.HasPrefix(got, "Harness legacy\n") || strings.Count(got, "Respond to the user in Korean") != 1 {
				t.Fatalf("legacy-only language = %q", got)
			}
			tune("preference.default", "", "session")
			if got := section(); strings.Contains(got, seed) || strings.Contains(got, "legacy") || !strings.HasPrefix(got, "Respond to the user in Korean") {
				t.Fatalf("all slots hidden = %q", got)
			}
			if body := call("workflow_manual", nil); !strings.Contains(body, "Respond to the user in Korean") || strings.Contains(body, "Harness legacy") {
				t.Fatalf("workflow_manual slots/language = %q", body)
			}
			// The stored scalar text is not trimmed or lowercased.
			tune("preference.exact", "  Mixed Case\ntext  ", "session")
			r := s.sessionResolver(key, nil)
			rv, err := r.Get(key, "preference.exact")
			if err != nil || rv.Value != "  Mixed Case\ntext  " || rv.Scope != wsconfig.ScopeSession {
				t.Fatalf("verbatim = %+v, %v", rv, err)
			}
		})
	}
}
