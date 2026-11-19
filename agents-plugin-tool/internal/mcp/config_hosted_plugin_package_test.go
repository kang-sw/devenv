package mcp

import (
	"path/filepath"
	"testing"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

func TestConfigHostedPluginPackage(t *testing.T) {
	useLeadProfile(t)
	useResourceFixture(t, buildOverrideTestTree(t))
	t.Setenv("WS_MCP_NO_AGENT", "1")
	root := t.TempDir()
	mustWrite(t, root, "ai-docs/_index.md", "# Index\n")
	initGit(t, root)
	t.Setenv("WS_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	configHome := filepath.Join(t.TempDir(), "config")
	t.Setenv("WS_CONFIG_HOME", configHome)

	s := NewServer(root, "test")
	key, _ := parseLoginResponse(t, callLogin(t, s, 900702, root, nil))
	const item = wsconfig.HostedPluginPackageKey
	entry, ok := resolveConfigEntryForKey(item)
	if !ok || !entry.GlobalOnly() || !entry.RequiresLeadAuthority || !entry.NoAgentVisible || entry.HarnessApplicable || entry.RepoScoped() {
		t.Fatalf("incorrect hosted package registry posture: %+v", entry)
	}
	check := func(value, scope string) {
		t.Helper()
		got := configGetJSON(t, s, 1, map[string]any{"key": item, "session_key": key})
		if got.Value != value || got.Scope != scope {
			t.Fatalf("config.get = %+v, want %s/%s", got, value, scope)
		}
	}
	check("wsflow", "builtin")
	catalog := parseTuningCatalogResponse(t, keyedCall(t, s, 2, key, "config.list", map[string]any{"format": "json"}))
	knob := requireTuningKnob(t, catalog, item)
	if knob.Writer.Tool != "config.tune" || knob.Writer.FixedArguments["key"] != item || knob.Reset == nil || knob.Reset.FixedArguments["reset"] != "true" {
		t.Fatalf("incorrect writer/reset: %+v", knob)
	}
	values := fieldEnum(knob.ValueFields, "value")
	if len(values) != 2 || values[0] != "ws" || values[1] != "wsflow" {
		t.Fatalf("package enum = %v", values)
	}
	for _, scope := range []string{"session", "project", "repo"} {
		for _, args := range []map[string]any{
			{"key": item, "value": "ws", "scope": scope},
			{"key": item, "reset": true, "scope": scope},
		} {
			if resp := keyedCall(t, s, 3, key, "config.tune", args); !toolIsError(t, resp) {
				t.Fatalf("accepted non-global tune: %s", resp)
			}
		}
	}
	for _, value := range []string{"", "other"} {
		if resp := keyedCall(t, s, 4, key, "config.tune", map[string]any{"key": item, "value": value}); !toolIsError(t, resp) {
			t.Fatalf("accepted invalid package %q: %s", value, resp)
		}
	}
	for _, value := range []string{"ws", "wsflow"} {
		if resp := keyedCall(t, s, 5, key, "config.tune", map[string]any{"key": item, "value": value}); toolIsError(t, resp) {
			t.Fatalf("no-agent tune failed: %s", resp)
		}
		check(value, "global")
		// Read a fresh resolver to prove the setting is persisted, not session state.
		r := wsconfig.NewResolver(wsconfig.Options{ConfigHome: configHome}, builtinConfigDefaults(), nil, nil)
		got, err := r.Get("", item)
		if err != nil || got.Value != value || got.Scope != wsconfig.ScopeGlobal {
			t.Fatalf("persisted package = %+v, %v", got, err)
		}
	}
	if resp := keyedCall(t, s, 6, key, "config.tune", map[string]any{"key": item, "reset": true, "scope": "global"}); toolIsError(t, resp) {
		t.Fatalf("reset failed: %s", resp)
	}
	check("wsflow", "builtin")
}
