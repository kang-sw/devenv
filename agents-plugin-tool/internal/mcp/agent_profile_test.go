package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
)

// TestConfigTuneAgentProfile drives the agent.profile.<name> family through
// config.tune and config.get: refused writes touch no file, a repo write is
// stored in canonical form and read back with its scope, a project write
// shadows it, and reset falls back through the scopes to unset.
func TestConfigTuneAgentProfile(t *testing.T) {
	server, root, key := profileServer(t)
	repoFile := filepath.Join(root, ".ws-workflow", "config.json")
	profile := map[string]any{
		"backend":  "claude",
		"tools":    []any{"Read", "Bash(git status)"},
		"ws_tools": []any{"tickets.query"},
	}

	rejects := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"key": "agent.profile.Triage", "value": profile, "scope": "repo"}, "must match"},
		{map[string]any{"key": "agent.profile.", "value": profile, "scope": "repo"}, "must match"},
		{map[string]any{"key": "agent.profile.triage", "value": profile, "scope": "session"}, "never session-scoped"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "gpt"}, "scope": "repo"}, "backend must be"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"description": "no backend"}, "scope": "repo"}, "backend is required"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "claude", "bogus": true}, "scope": "repo"}, "unknown field"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "claude", "mcp_servers": map[string]any{"ws": map[string]any{}}}, "scope": "repo"}, "may not name"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "claude", "mcp_servers": map[string]any{"docs": map[string]any{"command": "d"}}}, "scope": "repo"}, "may not start programs"},
		{map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "pi", "plugins": []any{"ext.ts"}}, "scope": "repo"}, "may not start programs"},
		{map[string]any{"key": "agent.profile.triage", "value": "{}", "scope": "repo"}, "must be a profile object"},
		{map[string]any{"key": "agent.profile.triage", "scope": "repo"}, "value is required"},
	}
	for i, tc := range rejects {
		resp := keyedCall(t, server, 941100+i, key, "config.tune", tc.args)
		if !toolIsError(t, resp) || !strings.Contains(toolText(t, resp), tc.want) {
			t.Fatalf("config.tune(%v) = %s, want error containing %q", tc.args, resp, tc.want)
		}
	}
	if _, err := os.Stat(repoFile); !os.IsNotExist(err) {
		t.Fatalf("a refused write touched %s: %v", repoFile, err)
	}

	resp := keyedCall(t, server, 941200, key, "config.tune", map[string]any{"key": "agent.profile.triage", "value": profile, "scope": "repo"})
	if toolIsError(t, resp) {
		t.Fatalf("repo profile write failed: %s", resp)
	}
	text := toolText(t, resp)
	if !strings.Contains(text, "written: "+repoFile) || !strings.Contains(text, "git.commit") {
		t.Fatalf("repo write result must name the file and the git.commit obligation: %s", text)
	}
	members := map[string]map[string]json.RawMessage{}
	raw, err := os.ReadFile(repoFile)
	if err != nil {
		t.Fatalf("read repo file: %v", err)
	}
	if err := json.Unmarshal(raw, &members); err != nil {
		t.Fatalf("repo file: %v\n%s", err, raw)
	}
	stored, err := wsagent.ParseProfile(members["structured_overrides"]["agent.profile.triage"])
	if err != nil || stored.Backend != "claude" || len(stored.Tools) != 2 || stored.WSTools[0] != "tickets.query" {
		t.Fatalf("stored profile = %+v, %v\n%s", stored, err, raw)
	}

	got := configGetJSON(t, server, 941210, map[string]any{"key": "agent.profile.triage", "session_key": key})
	if got.Scope != "repo" {
		t.Fatalf("config.get scope = %q, want repo", got.Scope)
	}
	value, _ := json.Marshal(got.Value)
	if !strings.Contains(string(value), `"backend":"claude"`) || !strings.Contains(string(value), "tickets.query") {
		t.Fatalf("config.get value = %s", value)
	}
	resp = keyedCall(t, server, 941211, key, "config.get", map[string]any{"key": "agent.profile.triage"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), `agent.profile.triage: {"backend":"claude"`) || !strings.Contains(toolText(t, resp), "[scope:repo]") {
		t.Fatalf("config.get text = %s", resp)
	}

	// A project write wins over repo; a second repo write reports it shadowed.
	resp = keyedCall(t, server, 941220, key, "config.tune", map[string]any{"key": "agent.profile.triage", "value": map[string]any{"backend": "pi"}, "scope": "project"})
	if toolIsError(t, resp) {
		t.Fatalf("project profile write failed: %s", resp)
	}
	resp = keyedCall(t, server, 941221, key, "config.tune", map[string]any{"key": "agent.profile.triage", "value": profile, "scope": "repo"})
	if !strings.Contains(toolText(t, resp), "shadowed: effective agent.profile.triage [scope:project]") {
		t.Fatalf("shadowed repo write must say so: %s", toolText(t, resp))
	}

	// Reset removes the profile from the selected scope only.
	resp = keyedCall(t, server, 941230, key, "config.tune", map[string]any{"key": "agent.profile.triage", "reset": true, "scope": "project"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "effective: agent.profile.triage [scope:repo]") {
		t.Fatalf("project reset = %s", resp)
	}
	resp = keyedCall(t, server, 941231, key, "config.tune", map[string]any{"key": "agent.profile.triage", "reset": true, "scope": "repo"})
	if toolIsError(t, resp) || !strings.Contains(toolText(t, resp), "agent.profile.triage is not defined in any scope") {
		t.Fatalf("repo reset = %s", resp)
	}
	if got := configGetJSON(t, server, 941232, map[string]any{"key": "agent.profile.triage", "session_key": key}); got.Scope != configGetUnsetScope {
		t.Fatalf("config.get after reset = %+v, want unset", got)
	}
}

// TestConfigListAgentProfileKnob: the catalog lists the family with the
// wsagent value schema and the resolved profiles with provenance; a bad
// committed profile surfaces as the knob's error instead of failing
// config.list.
func TestConfigListAgentProfileKnob(t *testing.T) {
	server, root, key := profileServer(t)
	mustWrite(t, root, ".ws-workflow/config.json", `{"structured_overrides":{"agent.profile.triage":{"backend":"pi"}}}`)

	knob := func(id int) tuningKnob {
		t.Helper()
		resp := keyedCall(t, server, id, key, "config.list", map[string]any{"format": "json"})
		if toolIsError(t, resp) {
			t.Fatalf("config.list failed: %s", resp)
		}
		var view struct {
			Knobs []tuningKnob `json:"knobs"`
		}
		if err := json.Unmarshal([]byte(toolText(t, resp)), &view); err != nil {
			t.Fatalf("config.list json: %v", err)
		}
		for _, k := range view.Knobs {
			if k.ID == "agent.profile.<name>" {
				return k
			}
		}
		t.Fatalf("config.list has no agent.profile.<name> knob")
		return tuningKnob{}
	}

	k := knob(941400)
	schema, _ := json.Marshal(k.ValueSchema)
	want, _ := json.Marshal(wsagent.ProfileValueSchema())
	if string(schema) != string(want) || k.RepoMember != "structured_overrides" || !k.RepoScope {
		t.Fatalf("knob = %+v schema %s", k, schema)
	}
	scopes := fieldEnum(k.SelectorFields, "scope")
	if strings.Join(scopes, ",") != "project,repo,global" {
		t.Fatalf("scope enum = %v, want project, repo, global (never session)", scopes)
	}
	current, _ := json.Marshal(k.Current)
	var profiles []wsagent.ResolvedProfile
	if err := json.Unmarshal(current, &profiles); err != nil {
		t.Fatalf("current: %v\n%s", err, current)
	}
	if len(profiles) != 1 || profiles[0].Name != "triage" || profiles[0].Scope != wsconfig.ScopeRepo || profiles[0].Source != filepath.Join(root, ".ws-workflow", "config.json") {
		t.Fatalf("current profiles = %+v, want only triage from the repo file (no builtin profile)", profiles)
	}

	mustWrite(t, root, ".ws-workflow/config.json", `{"structured_overrides":{"agent.profile.triage":{"backend":"gpt"}}}`)
	k = knob(941401)
	if !strings.Contains(k.Error, "agent.profile.triage") || !strings.Contains(k.Error, ".ws-workflow") || k.Current != nil {
		t.Fatalf("bad profile knob = %+v", k)
	}
}

// TestAgentConfigNamespaceReserved: an adapter manifest may not declare keys
// under agent., which the built-in agent.profile.<name> family owns.
func TestAgentConfigNamespaceReserved(t *testing.T) {
	declared := loadDeclaredConfig(writeManifest(t, "agent.json", `{"schema_version":1,"namespace":"agent.","keys":[{"key":"agent.limit","type":"integer","default":1,"description":"d"}]}`))
	if len(declared.rejected) != 1 || !strings.Contains(declared.rejected[0], "reserved") {
		t.Fatalf("agent. manifest rejected = %v, want one reserved-namespace rejection", declared.rejected)
	}
	if _, ok := declared.lookup("agent.limit"); ok {
		t.Fatalf("agent.limit was declared despite the reserved namespace")
	}
}
