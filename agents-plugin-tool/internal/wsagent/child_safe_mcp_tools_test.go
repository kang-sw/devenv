package wsagent

import (
	"encoding/json"
	"slices"
	"testing"
)

var testMutatingAndExcludedWSTools = []string{
	"git.commit",
	"tickets.move",
	"tickets.close",
	"tickets.create_empty",
	"tickets.acquire",
	"tickets.release",
	"review.stamp",
	"note.write",
	"worktree.acquire",
	"worktree.release",
	"agent.spawn",
	"agent.interrupt",
	"agent.list",
	"agent.tiers",
	"whiteboard.update",
	"config.tune",
	"wiki.query",
	"tracker.query",
	"ferrule",
	"workflow_manual",
}

func TestChildSafeWSToolsAllowlist(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range ChildSafeWSTools {
		if !wsToolPattern.MatchString(name) {
			t.Errorf("ChildSafeWSTools entry %q does not match wsToolPattern", name)
		}
		if seen[name] {
			t.Errorf("ChildSafeWSTools contains duplicate %q", name)
		}
		seen[name] = true
		if !slices.Contains(AgentWSTools, name) {
			t.Errorf("ChildSafeWSTools entry %q is missing from AgentWSTools", name)
		}
	}
	for _, want := range []string{
		"path.generate", "path.write", "tickets.query", "tickets.template", "convention.read",
		"config.resolve_agent", "infra.read", "playbook.read", "note.query", "rationale.query", "git.log", "git.diff",
		"git.status", "git.merge_base", "api.list", "project_tree", "runtime.read",
		"review.marker",
		"mailbox.send", "mailbox.recv", "todo.list", "todo.read", "todo.add",
		"todo.check", "todo.erase", "todo.clear", "todo.reorder",
	} {
		if !seen[want] {
			t.Errorf("ChildSafeWSTools missing expected safe tool %q", want)
		}
	}
	for _, excluded := range testMutatingAndExcludedWSTools {
		if seen[excluded] {
			t.Errorf("ChildSafeWSTools must exclude %q", excluded)
		}
	}
}

func TestCodexChildSafeMCPToolsConfig(t *testing.T) {
	workerProfile := builtinProfile(t, AgentWorkerProfile(BackendCodex))
	for _, level := range []PermissionLevel{PermissionReadOnly, PermissionWorkdirWrite} {
		t.Run(string(level), func(t *testing.T) {
			cfg := codexThreadConfig(StartSpec{
				Root:            "/work",
				Profile:         workerProfile,
				MCPSession:      &MCPSession{URL: "http://127.0.0.1:9999/mcp/session"},
				PermissionLevel: level,
			})
			servers := cfg["mcp_servers"].(map[string]any)
			ws := servers["ws"].(map[string]any)
			if _, hasDefault := ws["default_tools_approval_mode"]; hasDefault {
				t.Fatalf("worker must not widen server-wide default_tools_approval_mode: %v", ws)
			}
			tools, ok := ws["tools"].(map[string]any)
			if !ok {
				t.Fatalf("worker mcp_servers.ws missing tools map: %v", ws)
			}
			if len(tools) != len(ChildSafeWSTools) {
				t.Fatalf("len(ws.tools) = %d, want %d", len(tools), len(ChildSafeWSTools))
			}
			for _, name := range ChildSafeWSTools {
				entry, ok := tools[name].(map[string]any)
				if !ok || entry["approval_mode"] != "approve" {
					t.Errorf("ws.tools[%q] = %v, want approval_mode=approve", name, tools[name])
				}
			}
			for _, excluded := range testMutatingAndExcludedWSTools {
				if _, ok := tools[excluded]; ok {
					t.Errorf("ws.tools must exclude mutating/restricted tool %q", excluded)
				}
			}
		})
	}

	t.Run("custom-subset-profile", func(t *testing.T) {
		cfg := codexThreadConfig(StartSpec{
			Root:            "/work",
			Profile:         Profile{Backend: BackendCodex, WSTools: []string{"path.generate", "git.commit"}},
			MCPSession:      &MCPSession{URL: "http://127.0.0.1:9999/mcp/session"},
			PermissionLevel: PermissionReadOnly,
		})
		ws := cfg["mcp_servers"].(map[string]any)["ws"].(map[string]any)
		tools := ws["tools"].(map[string]any)
		if len(tools) != 1 || tools["path.generate"].(map[string]any)["approval_mode"] != "approve" {
			t.Fatalf("custom subset ws.tools = %v, want path.generate alone", tools)
		}
		if _, ok := tools["git.commit"]; ok {
			t.Fatalf("custom subset ws.tools must exclude git.commit: %v", tools)
		}
	})

	t.Run("lead-unchanged", func(t *testing.T) {
		cfg := codexThreadConfig(StartSpec{
			Root:       "/work",
			Profile:    builtinProfile(t, LeadProfileCodex),
			MCPSession: &MCPSession{URL: "http://127.0.0.1:9999/mcp/session"},
			Lead:       &LeadSpec{DaemonVersion: "0.50.8"},
		})
		ws := cfg["mcp_servers"].(map[string]any)["ws"].(map[string]any)
		if _, hasTools := ws["tools"]; hasTools {
			t.Fatalf("lead mcp_servers.ws must remain unchanged without tools override: %v", ws)
		}
	})
}

func TestClaudeChildSafeMCPToolsArgs(t *testing.T) {
	workerProfile := builtinProfile(t, AgentWorkerProfile(BackendClaude))

	t.Run("read-only", func(t *testing.T) {
		spec := StartSpec{
			Root:            "/work",
			Profile:         workerProfile,
			WSMCP:           &MCPCommand{Command: "/bin/ws-mcp"},
			PermissionLevel: PermissionReadOnly,
			RemoveSubagents: true,
		}
		args, err := claudeArgs(spec)
		if err != nil {
			t.Fatal(err)
		}
		if mode, ok := flagValue(args, "--permission-mode"); !ok || mode != "dontAsk" {
			t.Fatalf("--permission-mode = %q (ok %v), want dontAsk", mode, ok)
		}
		i := slices.Index(args, "--allowedTools")
		if i < 0 {
			t.Fatalf("missing --allowedTools: %v", args)
		}
		allowed := args[i+1:]
		for _, name := range ChildSafeWSTools {
			rule := claudeMCPToolName(ReservedMCPServer, name)
			if !slices.Contains(allowed, rule) {
				t.Errorf("read-only --allowedTools missing safe tool rule %q", rule)
			}
		}
		for _, excluded := range testMutatingAndExcludedWSTools {
			rule := claudeMCPToolName(ReservedMCPServer, excluded)
			if slices.Contains(allowed, rule) {
				t.Errorf("read-only --allowedTools must exclude mutating/restricted tool rule %q", rule)
			}
		}
		if got, want := len(allowed), len(workerProfile.Tools)+len(ChildSafeWSTools); got != want {
			t.Fatalf("read-only --allowedTools count = %d, want %d (native %d + safe ws %d): %v", got, want, len(workerProfile.Tools), len(ChildSafeWSTools), allowed)
		}
	})

	t.Run("workdir-write-preserves-existing", func(t *testing.T) {
		spec := StartSpec{
			Root:            "/work",
			Profile:         workerProfile,
			WSMCP:           &MCPCommand{Command: "/bin/ws-mcp"},
			PermissionLevel: PermissionWorkdirWrite,
			RemoveSubagents: true,
		}
		args, err := claudeArgs(spec)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.Index(args, "--allowedTools")
		if i < 0 {
			t.Fatalf("missing --allowedTools: %v", args)
		}
		allowed := args[i+1:]
		for _, name := range append(slices.Clone(ChildSafeWSTools), "git.commit", "tickets.move", "agent.spawn", "review.stamp") {
			rule := claudeMCPToolName(ReservedMCPServer, name)
			if !slices.Contains(allowed, rule) {
				t.Errorf("workdir-write --allowedTools missing %q", rule)
			}
		}
	})

	t.Run("lead-unchanged", func(t *testing.T) {
		spec := claudePluginLeadSpec(t)
		args, err := claudeArgs(spec)
		if err != nil {
			t.Fatal(err)
		}
		i := slices.Index(args, "--allowedTools")
		if i < 0 {
			t.Fatalf("missing --allowedTools: %v", args)
		}
		if got := args[i+1:]; !slices.Equal(got, []string{"mcp__ws"}) {
			t.Fatalf("lead --allowedTools = %v, want [mcp__ws]", got)
		}
	})
}

func TestCodexThreadConfigJSONRoundTrip(t *testing.T) {
	cfg := codexThreadConfig(StartSpec{
		Root:            "/work",
		Profile:         builtinProfile(t, AgentWorkerProfile(BackendCodex)),
		MCPSession:      &MCPSession{URL: "http://127.0.0.1:9999/mcp/session"},
		PermissionLevel: PermissionReadOnly,
	})
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		MCPServers map[string]struct {
			Tools map[string]struct {
				ApprovalMode string `json:"approval_mode"`
			} `json:"tools"`
		} `json:"mcp_servers"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	wsTools := decoded.MCPServers["ws"].Tools
	if wsTools["path.generate"].ApprovalMode != "approve" {
		t.Fatalf("decoded ws.tools[path.generate] = %+v", wsTools["path.generate"])
	}
	if _, ok := wsTools["git.commit"]; ok {
		t.Fatalf("decoded ws.tools unexpectedly includes git.commit")
	}
}
