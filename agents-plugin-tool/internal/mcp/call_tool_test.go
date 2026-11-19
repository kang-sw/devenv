package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsagent"
)

func callResponseIsError(t *testing.T, raw string) bool {
	t.Helper()
	var resp response
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	return responseIsError(resp)
}

func assertCallMatchesDirect(t *testing.T, s *Server, name string, args map[string]any) {
	t.Helper()
	direct := callToolOnce(t, s, 1, name, args)
	indirect := callToolOnce(t, s, 1, "call", map[string]any{"tool": name, "arguments": args})
	var a, b any
	if err := json.Unmarshal([]byte(direct), &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(indirect), &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("%s: direct = %s\ncall = %s", name, direct, indirect)
	}
}

func assertCallListsConnection(t *testing.T, s *Server) {
	t.Helper()
	resp := callToolOnce(t, s, 1, "call", map[string]any{})
	names := strings.Split(toolText(t, resp), "\n")
	if !sort.StringsAreSorted(names) {
		t.Fatalf("call list is not sorted: %v", names)
	}
	advertised := listedToolNames(t, s)
	for i, name := range names {
		if !advertised[name] || (i > 0 && name == names[i-1]) {
			t.Fatalf("call listed an unadvertised or duplicate tool: %q", name)
		}
	}
	if !listedToolNames(t, s)["call"] {
		t.Fatal("call must be listed on this connection")
	}
}

func assertCallSchema(t *testing.T, s *Server, name string) {
	t.Helper()
	resp := callToolOnce(t, s, 1, "call", map[string]any{"tool": name})
	text := toolText(t, resp)
	var got map[string]any
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("schema: %v: %s", err, resp)
	}
	var want string
	// Marshal the same two fields from tools/list (including worker wording).
	for _, tool := range s.filteredTools() {
		if tool["name"] != name {
			continue
		}
		raw, err := json.Marshal(map[string]any{"description": tool["description"], "inputSchema": tool["inputSchema"]})
		if err != nil {
			t.Fatal(err)
		}
		want = string(raw)
	}
	if want == "" || text != want {
		t.Fatalf("schema = %q, want %q", text, want)
	}
}

func TestCallListSchemaAndDispatch(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envAllowedTools, "")
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	assertCallListsConnection(t, s)
	assertCallSchema(t, s, "runtime.read")
	assertCallMatchesDirect(t, s, "runtime.read", map[string]any{"format": "json"})
	assertCallMatchesDirect(t, s, "runtime.read", map[string]any{"unknown_argument": true})
	assertCallMatchesDirect(t, s, "not.a.tool", map[string]any{})
	unknown := callToolOnce(t, s, 1, "call", map[string]any{"tool": "not.a.tool"})
	if direct := callToolOnce(t, s, 1, "not.a.tool", map[string]any{}); unknown != direct {
		t.Fatalf("unknown schema = %s, direct = %s", unknown, direct)
	}
	for _, args := range []map[string]any{
		{"tool": "call"},
		{"tool": "call", "arguments": map[string]any{}},
		{"tool": 42},
		{"tool": "runtime.read", "arguments": nil},
		{"tool": "runtime.read", "arguments": []any{}},
	} {
		if resp := callToolOnce(t, s, 1, "call", args); !toolIsError(t, resp) {
			t.Fatalf("invalid envelope %v succeeded: %s", args, resp)
		}
	}
}

func TestCallWorkerAllowlistAndAuthority(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	// A custom profile need not explicitly include call.
	t.Setenv(envAllowedTools, "git.status")
	s := NewServer(root, "test")
	defer s.Close()
	assertCallListsConnection(t, s)
	if got := toolText(t, callToolOnce(t, s, 1, "call", map[string]any{})); got != "call\ngit.status" {
		t.Fatalf("worker list = %q", got)
	}
	for _, name := range []string{"tickets.query", "not.a.tool"} {
		assertCallMatchesDirect(t, s, name, map[string]any{})
		direct := callToolOnce(t, s, 1, name, map[string]any{})
		schema := callToolOnce(t, s, 1, "call", map[string]any{"tool": name})
		if schema != direct || !strings.Contains(direct, "tool not available") {
			t.Fatalf("allowlist schema = %s, direct = %s", schema, direct)
		}
	}
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	assertCallMatchesDirect(t, s, "git.status", map[string]any{"session_key": key})

	// With no profile filter, the inner key's role must still refuse a lead tool.
	t.Setenv(envAllowedTools, "")
	roleServer := NewServer(root, "test")
	defer roleServer.Close()
	assertCallMatchesDirect(t, roleServer, "workflow_state", map[string]any{"session_key": key})
	if resp := callToolOnce(t, roleServer, 1, "workflow_state", map[string]any{"session_key": key}); !strings.Contains(resp, "tool not available") {
		t.Fatalf("expected a role refusal: %s", resp)
	}
	// The read-only permission gate must not be bypassed by the envelope.
	t.Setenv(wsagent.EnvPermissionLevel, "read-only")
	readOnly := NewServer(root, "test")
	defer readOnly.Close()
	assertCallMatchesDirect(t, readOnly, "git.commit", map[string]any{"session_key": key})
	assertCallMatchesDirect(t, readOnly, "git.status", map[string]any{}) // root/key refusal too
}

func TestCallAgentlessGate(t *testing.T) {
	setupMailboxTestEnv(t)
	t.Setenv(envNoAgent, "1")
	t.Setenv(envAllowedTools, "")
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	assertCallListsConnection(t, s)
	assertCallMatchesDirect(t, s, "exec.shell", map[string]any{})
	direct := callToolOnce(t, s, 1, "exec.shell", map[string]any{})
	schema := callToolOnce(t, s, 1, "call", map[string]any{"tool": "exec.shell"})
	if schema != direct || !strings.Contains(direct, "agentless mode disables") {
		t.Fatalf("agentless schema = %s, direct = %s", schema, direct)
	}
}

func TestCallHostedKeyAndAliases(t *testing.T) {
	s, f := hostedBackendConnection(t, wsagent.BackendAgy, descriptorRegistrations())
	defer s.Close()
	assertCallListsConnection(t, s)
	assertCallSchema(t, s, "todo_list")
	assertCallMatchesDirect(t, s, "todo_list", map[string]any{})
	assertCallMatchesDirect(t, s, "todo.list", map[string]any{})
	if resp := callToolOnce(t, s, 1, "call", map[string]any{"tool": "todo_list", "arguments": map[string]any{}}); toolIsError(t, resp) {
		t.Fatalf("hosted inner omitted key: %s", resp)
	}
	assertCallMatchesDirect(t, s, "todo_list", map[string]any{"session_key": f.peer})
}

func TestCallForwardingWorker(t *testing.T) {
	setupMailboxTestEnv(t)
	route := &fakeAgentRoute{}
	daemon := httptest.NewServer(route)
	defer daemon.Close()
	t.Setenv(wsagent.EnvPiToken, "tok123")
	t.Setenv(wsagent.EnvPiEndpoint, daemon.URL)
	t.Setenv(envAllowedTools, strings.Join(wsagent.AgentWSTools, ","))
	root := t.TempDir()
	initGit(t, root)
	s := NewServer(root, "test")
	defer s.Close()
	assertCallListsConnection(t, s)
	assertCallSchema(t, s, "agent.spawn")
	key, err := s.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	assertCallMatchesDirect(t, s, "agent.list", map[string]any{"session_key": key})
	if call := route.last(); call.path != wsagent.RoutePrefix+"/by-token/agent.list" || call.auth != "Bearer tok123" {
		t.Fatalf("inner call did not use forwarding authority: %+v", call)
	}
}

func TestCallDiscoveryRespectsPermissionAndRole(t *testing.T) {
	setupMailboxTestEnv(t)
	root := t.TempDir()
	initGit(t, root)
	t.Setenv(envAllowedTools, "git.commit,git.status,workflow_state")
	keys := NewServer(root, "test")
	defer keys.Close()
	delegate, err := keys.sessions.mint(root, roleDelegate, "")
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := keys.sessions.mint(root, roleLeaf, "")
	if err != nil {
		t.Fatal(err)
	}
	lead, err := keys.sessions.mint(root, roleLead, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, level, key, want string
	}{
		{"read-only delegate", "read-only", delegate, "call\ngit.status"},
		{"read-only lead", "read-only", lead, "call\ngit.status\nworkflow_state"},
		{"writing delegate", "workdir-write", delegate, "call\ngit.commit\ngit.status"},
		{"leaf", "workdir-write", leaf, "call\ngit.status"},
		{"lead", "workdir-write", lead, "call\ngit.commit\ngit.status\nworkflow_state"},
		{"keyless", "workdir-write", "", "call\ngit.status"},
		{"unknown key", "workdir-write", "not-a-key", "call\ngit.status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(wsagent.EnvPermissionLevel, tc.level)
			s := NewServer(root, "test")
			defer s.Close()
			args := map[string]any{}
			if tc.key != "" {
				args["session_key"] = tc.key
			}
			list := callToolOnce(t, s, 1, "call", args)
			if got := toolText(t, list); got != tc.want {
				t.Fatalf("discovery list = %q, want %q", got, tc.want)
			}
			// Schema visibility must exactly match list visibility, even
			// though tools/list still advertises the profile's wider surface.
			for _, name := range []string{"git.commit", "git.status", "workflow_state"} {
				args["tool"] = name
				schema := callToolOnce(t, s, 1, "call", args)
				visible := strings.Contains("\n"+tc.want+"\n", "\n"+name+"\n")
				if callResponseIsError(t, schema) == visible {
					t.Fatalf("%s visibility disagrees with list: %s", name, schema)
				}
				if !visible && tc.key != "" && tc.key != "not-a-key" {
					direct := callToolOnce(t, s, 1, name, map[string]any{"session_key": tc.key})
					if schema != direct {
						t.Fatalf("%s schema refusal = %s, direct = %s", name, schema, direct)
					}
				}
			}
		})
	}
}

func TestCallDiscoveryUsesHostedOrEnvelopeKey(t *testing.T) {
	s, f := hostedBackendConnection(t, wsagent.BackendAgy, descriptorRegistrations())
	defer s.Close()
	for _, tc := range []struct {
		key     string
		visible bool
	}{
		{"", true}, // omitted: the hosted lead's own key
		{f.child, false},
	} {
		args := map[string]any{}
		if tc.key != "" {
			args["session_key"] = tc.key
		}
		list := toolText(t, callToolOnce(t, s, 1, "call", args))
		if strings.Contains("\n"+list+"\n", "\nworkflow_state\n") != tc.visible {
			t.Fatalf("hosted list with key %q has wrong role visibility", tc.key)
		}
		args["tool"] = "workflow_state"
		if schema := callToolOnce(t, s, 1, "call", args); callResponseIsError(t, schema) == tc.visible {
			t.Fatalf("hosted schema with key %q: %s", tc.key, schema)
		}
	}
}

func TestCallDefinitionSize(t *testing.T) {
	setupMailboxTestEnv(t)
	s := NewServer(t.TempDir(), "test")
	defer s.Close()
	for _, tool := range listedToolSizes(t, s) {
		if tool.name == "call" {
			t.Logf("call: %d bytes", tool.size)
			if tool.size >= 600 {
				t.Fatalf("call costs %d bytes; must stay under 600", tool.size)
			}
			return
		}
	}
	t.Fatal("call is missing from tools/list")
}
