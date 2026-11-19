package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

const codexWorkerTestEnv = "WSAGENT_CODEX_WORKER_LOG"

// codexWorkerStandin is the test binary's app-server mode, with no provider
// access. It logs every input line and behaves as Codex does where the
// worker's sandbox matters: thread/start connects the ws MCP server it is
// given, and a turn asks for an approval unless approvalPolicy is never.
func codexWorkerStandin(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	defer f.Close()
	log := json.NewEncoder(f)
	out := json.NewEncoder(os.Stdout)
	_ = log.Encode(map[string]any{"argv": os.Args[1:], "token": os.Getenv(EnvMCPSessionToken) != ""})
	reply := func(id json.RawMessage, result any) { _ = out.Encode(map[string]any{"id": id, "result": result}) }
	notify := func(method string, params any) { _ = out.Encode(map[string]any{"method": method, "params": params}) }
	sid, turns, turn := "", 0, ""
	complete := func() {
		item := map[string]any{"id": "reply-" + turn, "type": "agentMessage", "text": "worker complete"}
		notify("turn/completed", map[string]any{"threadId": sid, "turn": map[string]any{"id": turn, "status": "completed", "items": []any{item}}})
	}
	for sc := bufio.NewScanner(os.Stdin); sc.Scan(); {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ThreadID       string `json:"threadId"`
				ApprovalPolicy string `json:"approvalPolicy"`
				Config         struct {
					MCPServers map[string]struct {
						URL    string `json:"url"`
						Bearer string `json:"bearer_token_env_var"`
					} `json:"mcp_servers"`
				} `json:"config"`
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			os.Exit(3)
		}
		_ = log.Encode(json.RawMessage(sc.Bytes()))
		switch msg.Method {
		case "initialize":
			reply(msg.ID, map[string]any{"codexHome": "/synthetic/codex-home"})
		case "thread/start", "thread/resume":
			server := msg.Params.Config.MCPServers["ws"]
			req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{}`))
			req.Header.Set("Authorization", "Bearer "+os.Getenv(server.Bearer))
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				os.Exit(5)
			}
			_ = resp.Body.Close()
			if sid = msg.Params.ThreadID; sid == "" {
				sid = "codex-worker-session"
			}
			reply(msg.ID, map[string]any{"thread": map[string]any{"id": sid, "cliVersion": "0.160.0", "model": "synthetic-model"}, "approvalPolicy": msg.Params.ApprovalPolicy, "sandbox": map[string]any{"type": "synthetic"}})
		case "model/list":
			reply(msg.ID, map[string]any{"data": []any{map[string]any{"model": "synthetic-model", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}}}}})
		case "turn/start":
			turns++
			turn = fmt.Sprintf("t%d", turns)
			reply(msg.ID, map[string]any{"turn": map[string]any{"id": turn, "status": "inProgress"}})
			notify("turn/started", map[string]any{"threadId": sid, "turn": map[string]any{"id": turn, "status": "inProgress"}})
			if msg.Params.ApprovalPolicy == "never" {
				complete()
				continue
			}
			_ = out.Encode(map[string]any{"id": "req-" + turn, "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": sid, "turnId": turn, "itemId": "cmd-" + turn, "command": "git commit"}})
		case "":
			// The answer to the approval request ends the turn.
			complete()
		}
	}
}

// codexWorkerCalls are the params of every logged request of method.
func codexWorkerCalls(t *testing.T, path, method string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range agyWorkerLog(t, path) {
		var m string
		_ = json.Unmarshal(line["method"], &m)
		if m != method {
			continue
		}
		var params map[string]any
		if err := json.Unmarshal(line["params"], &params); err != nil {
			t.Fatal(err)
		}
		out = append(out, params)
	}
	return out
}

// TestCodexWorkerLaunchAuthorityAndApproval runs a Codex worker through the
// Runner's worker start path, so the runner's PermissionLevel assignment is
// what selects the sandbox.
func TestCodexWorkerLaunchAuthorityAndApproval(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		authority, sandbox, kind, policy string
		approved                         bool
	}{
		{"read-only", "read-only", "readOnly", "never", false},
		{"workdir-write", "workspace-write", "workspaceWrite", "on-request", true},
	} {
		t.Run(tc.authority, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app-server.jsonl")
			restore := environ
			environ = func() []string { return append(os.Environ(), codexWorkerTestEnv+"="+path) }
			t.Cleanup(func() { environ = restore })
			h, got := agentHarness(t, nil)
			h.runner.Close()
			h.config.Backends[BackendCodex] = BackendRegistration{Backend: CodexBackend{Command: exe}, LeadProfile: LeadProfileCodex, PermissionModes: CodexPermissionModes, Catalog: CodexCatalog(), WorkerCapable: true, WorkerProfile: AgentWorkerProfile(BackendCodex), WorkerHTTPMCP: true, WorkerAutoApprove: true}
			var mu sync.Mutex
			var hellos []wsdashboard.ProxyHello
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				hello, _, ok := h.runner.ResolveMCPToken(strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "))
				if !ok {
					http.Error(w, "bad token", http.StatusUnauthorized)
					return
				}
				mu.Lock()
				hellos = append(hellos, hello)
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			h.endpoint = server.URL
			h.start()

			id := h.agentSpawn(h.leadCaller(), AgentSpawnRequest{Prompt: "work", Tier: "codex.small", Authority: tc.authority, SystemPrompt: "WORKER-SPINE"})
			eventually(t, "the codex worker's report", func() bool { return len(got.list()) == 1 })
			if report := got.list()[0]; report.Result.Status != TurnDone || report.Result.Message != "worker complete" {
				t.Fatalf("report: %+v", report)
			}
			rec := h.record(id)
			if rec.Lead || rec.HolderID != "" || rec.MCPToken != "" || rec.Capability != "" || rec.Backend != BackendCodex || rec.Profile != AgentWorkerProfile(BackendCodex) || rec.Model == "" || rec.SessionID != "codex-worker-session" {
				t.Fatalf("worker record: %+v", rec)
			}
			if st := h.state(id); len(st.Approvals) != 0 || st.Error != "" || st.Capabilities == nil || st.Capabilities.Grade(ItemApproval) != GradeSubstitute {
				t.Fatalf("worker state: %+v", st)
			}
			mu.Lock()
			hello := hellos[0]
			mu.Unlock()
			if hello.OwnerPID <= 0 || hello.Env[EnvHostedSession] != "" || hello.Env[EnvAgentSessionKey] != rec.WSKey || hello.Env[EnvAllowedTools] != strings.Join(AgentWSTools, ",") || hello.Env[EnvPermissionLevel] != tc.authority {
				t.Fatalf("worker's MCP context: %+v", hello)
			}
			first := agyWorkerLog(t, path)[0]
			var argv []string
			_ = json.Unmarshal(first["argv"], &argv)
			if !slices.Equal(argv, []string{"app-server"}) || string(first["token"]) != "true" {
				t.Fatalf("launch: argv %q, token %s", argv, first["token"])
			}

			open := codexWorkerCalls(t, path, "thread/start")
			if len(open) != 1 {
				t.Fatalf("thread/start calls: %v", open)
			}
			params := open[0]
			config, _ := json.Marshal(params["config"])
			if params["sandbox"] != tc.sandbox || params["approvalPolicy"] != tc.policy || params["model"] != rec.Model || !sameDir(params["cwd"].(string), h.root) ||
				!strings.Contains(string(config), `"enabled":false`) || !strings.Contains(string(config), server.URL) {
				t.Fatalf("thread/start: %v", params)
			}
			text, _ := params["developerInstructions"].(string)
			for _, want := range []string{"WORKER-SPINE", "spawn_agent", "agent.spawn", rec.WSKey} {
				if !strings.Contains(text, want) {
					t.Errorf("developerInstructions lack %q: %q", want, text)
				}
			}
			turn := codexWorkerCalls(t, path, "turn/start")
			if len(turn) != 1 || turn[0]["approvalPolicy"] != tc.policy || turn[0]["sandboxPolicy"].(map[string]any)["type"] != tc.kind {
				t.Fatalf("turn/start: %v", turn)
			}
			answered := false
			for _, line := range agyWorkerLog(t, path) {
				var raw string
				_ = json.Unmarshal(line["id"], &raw)
				answered = answered || (strings.HasPrefix(raw, "req-") && strings.Contains(string(line["result"]), `"accept"`))
			}
			if answered != tc.approved {
				t.Fatalf("auto-allowed approval = %v, want %v", answered, tc.approved)
			}

			// A wake resumes the thread under the same authority.
			h.asleep(id)
			if _, err := h.runner.Deliver(id, Delivery{Text: "again"}); err != nil {
				t.Fatal(err)
			}
			eventually(t, "the resumed report", func() bool { return len(got.list()) == 2 })
			resumed := codexWorkerCalls(t, path, "thread/resume")
			if len(resumed) != 1 || resumed[0]["threadId"] != "codex-worker-session" || resumed[0]["sandbox"] != tc.sandbox || resumed[0]["approvalPolicy"] != tc.policy {
				t.Fatalf("thread/resume: %v", resumed)
			}
		})
	}
}

func TestCodexWorkerModeAndStartRefusals(t *testing.T) {
	for level, want := range map[PermissionLevel][2]string{
		PermissionReadOnly:     {"read-only", "never"},
		PermissionWorkdirWrite: {"workspace-write", "on-request"},
		PermissionFull:         {"workspace-write", "on-request"},
	} {
		if sandbox, policy, err := codexWorkerMode(level); err != nil || sandbox != want[0] || policy != want[1] {
			t.Errorf("%s: %s %s %v", level, sandbox, policy, err)
		}
	}
	if _, _, err := codexWorkerMode(""); err == nil || !strings.Contains(err.Error(), "needs an authority") {
		t.Fatalf("a worker with no authority must be refused: %v", err)
	}
	// Every refusal comes before any process: Command is not a program.
	b := CodexBackend{Command: filepath.Join(t.TempDir(), "no-such-codex")}
	for name, spec := range map[string]StartSpec{
		"worker without authority": {Root: t.TempDir(), MCPSession: &MCPSession{}},
		"lead without holder":      {Root: t.TempDir(), Lead: &LeadSpec{}},
		"worker with holder":       {Root: t.TempDir(), Holder: &holderChild{}, PermissionLevel: PermissionReadOnly},
	} {
		if _, err := b.Start(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "codex backend:") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// A worker's sandbox is its authority: the mode control a lead has is
// refused, so a read-only worker cannot be widened through it.
func TestCodexWorkerRefusesModeChange(t *testing.T) {
	p := &codexProcess{worker: true, mode: "read-only", policy: "never"}
	if err := p.SetPermissionMode("danger-full-access"); err == nil || p.mode != "read-only" || p.policy != "never" {
		t.Fatalf("mode change: %v, %s %s", err, p.mode, p.policy)
	}
	lead := &codexProcess{mode: "read-only", policy: "on-request"}
	if err := lead.SetPermissionMode("danger-full-access"); err != nil || lead.mode != "danger-full-access" || lead.policy != "never" {
		t.Fatalf("lead mode change: %v, %s %s", err, lead.mode, lead.policy)
	}
}
