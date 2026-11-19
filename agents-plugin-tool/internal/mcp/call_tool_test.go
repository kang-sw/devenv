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
	var want []string
	for _, tool := range s.filteredTools() {
		want = append(want, tool["name"].(string))
	}
	sort.Strings(want)
	resp := callToolOnce(t, s, 1, "call", map[string]any{})
	if got := toolText(t, resp); got != strings.Join(want, "\n") {
		t.Fatalf("call list = %q, want %v", got, want)
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
