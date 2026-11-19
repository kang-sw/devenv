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

const agyWorkerTestEnv = "WSAGENT_AGY_WORKER_LOG"

// agyWorkerStandin is the test binary's ACP mode, with no provider access.
// Each start appends its inputs to one log so resume and token rotation can
// be checked across actual, non-held processes.
func agyWorkerStandin(path string) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		os.Exit(2)
	}
	defer f.Close()
	log := json.NewEncoder(f)
	out := json.NewEncoder(os.Stdout)
	_ = log.Encode(map[string]any{"argv": os.Args[1:], "env": os.Environ()})
	mode := os.Getenv("WSAGENT_AGY_WORKER_MODE")
	model, effort := "gemini-4-hydrogen", "high"
	if mode == "config-suffix" {
		model += "-high"
	}
	config := func() []any {
		values := []any{}
		for _, base := range []string{"gemini-4-hydrogen", "gemini-4-argon", "gemini-4-carbon"} {
			if mode == "config-suffix" {
				for _, e := range []string{"medium", "high"} {
					values = append(values, map[string]any{"value": base + "-" + e})
				}
			} else {
				values = append(values, map[string]any{"value": base})
			}
		}
		opts := []any{map[string]any{"id": "model", "currentValue": model, "options": values}}
		if mode == "" || mode == "ignored" {
			opts = append(opts, map[string]any{"id": "thought_level", "currentValue": effort, "options": []any{map[string]any{"value": "medium"}, map[string]any{"value": "high"}}})
		}
		return opts
	}
	var prompt json.RawMessage
	for sc := bufio.NewScanner(os.Stdin); sc.Scan(); {
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params struct {
				ConfigID, Value string
				SessionID       string `json:"sessionId"`
				ModelID         string `json:"modelId"`
				MCPServers      []struct {
					URL     string
					Headers []struct{ Name, Value string }
				}
			} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			os.Exit(3)
		}
		_ = log.Encode(json.RawMessage(sc.Bytes()))
		var result any = map[string]any{}
		switch msg.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"version": "1.5.10"}, "agentCapabilities": map[string]any{"sessionCapabilities": map[string]any{"resume": map[string]any{}}}}
		case "session/new", "session/resume":
			if len(msg.Params.MCPServers) != 1 {
				os.Exit(4)
			}
			s := msg.Params.MCPServers[0]
			req, _ := http.NewRequest(http.MethodPost, s.URL, strings.NewReader(`{}`))
			for _, h := range s.Headers {
				req.Header.Set(h.Name, h.Value)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				os.Exit(5)
			}
			_ = resp.Body.Close()
			sid := msg.Params.SessionID
			if sid == "" {
				sid = "agy-worker-session"
			}
			variants := []any{}
			if mode == "acp-suffix" {
				variants = append(variants, map[string]any{"modelId": "gemini-4-hydrogen-medium"})
			}
			result = map[string]any{"sessionId": sid, "configOptions": config(), "models": map[string]any{"currentModelId": model, "availableModels": variants}, "modes": map[string]any{"currentModeId": "default", "availableModes": []any{map[string]any{"id": "default"}}}}
		case "session/set_config_option":
			if msg.Params.ConfigID == "model" {
				model = msg.Params.Value
			} else if mode != "ignored" {
				effort = msg.Params.Value
			}
			result = map[string]any{"configOptions": config()}
		case "session/set_model":
			model = msg.Params.ModelID
			result = map[string]any{"models": map[string]any{"currentModelId": model}}
		case "session/prompt":
			prompt = slices.Clone(msg.ID)
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": "permission-1", "method": "session/request_permission", "params": map[string]any{"toolCall": map[string]any{"toolCallId": "call-1", "name": "run_command"}, "options": []any{map[string]any{"kind": "allow_once", "optionId": "yes"}, map[string]any{"kind": "reject_once", "optionId": "no"}}}})
			continue
		case "":
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "worker complete"}}}})
			_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": prompt, "result": map[string]any{"stopReason": "end_turn"}})
			continue
		}
		_ = out.Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "result": result})
	}
}

func agyWorkerLog(t *testing.T, path string) []map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]json.RawMessage
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatal(err)
		}
		out = append(out, msg)
	}
	return out
}

func TestAgyWorkerLaunchApprovalAndResume(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "acp.jsonl")
	restore := environ
	environ = func() []string { return append(os.Environ(), agyWorkerTestEnv+"="+path) }
	t.Cleanup(func() { environ = restore })
	h, got := agentHarness(t, nil)
	h.runner.Close()
	h.config.Backends[BackendAgy] = BackendRegistration{Backend: AgyBackend{Command: exe}, WorkerCapable: true, WorkerProfile: AgentWorkerProfile(BackendAgy), WorkerHTTPMCP: true, WorkerAutoApprove: true, Catalog: AgyCatalog()}
	var mu sync.Mutex
	var tokens []string
	var hellos []wsdashboard.ProxyHello
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		hello, _, ok := h.runner.ResolveMCPToken(token)
		if !ok {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		mu.Lock()
		tokens, hellos = append(tokens, token), append(hellos, hello)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	h.endpoint = server.URL
	h.start()
	id := h.agentSpawn(h.leadCaller(), AgentSpawnRequest{Prompt: "work", Tier: "agy.small", Authority: "read-only", SystemPrompt: "WORKER-SPINE"})
	eventually(t, "the agy worker's report", func() bool { return len(got.list()) == 1 })
	if report := got.list()[0]; report.Result.Status != TurnDone || report.Result.Message != "worker complete" {
		t.Fatalf("report: %+v", report)
	}
	rec := h.record(id)
	if rec.Lead || rec.HolderID != "" || rec.MCPToken != "" || rec.Capability != "" || rec.Model != "gemini-4-hydrogen" || rec.Effort != "medium" || rec.SessionID != "agy-worker-session" {
		t.Fatalf("worker record: %+v", rec)
	}
	if st := h.state(id); len(st.Approvals) != 0 || st.Error != "" || st.Capabilities == nil || st.Capabilities.Grade(ItemSteer) != GradeNative {
		t.Fatalf("worker state: %+v", st)
	}
	mu.Lock()
	first := tokens[0]
	hello := hellos[0]
	mu.Unlock()
	if hello.OwnerPID <= 0 || hello.Env[EnvHostedSession] != "" || hello.Env[EnvAgentSessionKey] != rec.WSKey || hello.Env[EnvAllowedTools] != strings.Join(AgentWSTools, ",") || hello.Env[EnvPermissionLevel] != "read-only" || hello.Env[EnvPiToken] == "" {
		t.Fatalf("worker's MCP context: %+v", hello)
	}
	log := agyWorkerLog(t, path)
	var argv []string
	_ = json.Unmarshal(log[0]["argv"], &argv)
	if !slices.Equal(argv, []string{"acp", "--process-per-session"}) {
		t.Fatalf("default agent in the user's home: %q", argv)
	}
	text, _ := os.ReadFile(path)
	for _, want := range []string{"WORKER-SPINE", "invoke_subagent", "agent.spawn", rec.WSKey, `"optionId":"yes"`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("ACP log lacks %q", want)
		}
	}
	h.asleep(id)
	if _, _, ok := h.runner.ResolveMCPToken(first); ok {
		t.Fatal("stopped worker token still resolves")
	}
	if _, err := h.runner.Deliver(id, Delivery{Text: "resume"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the resumed report", func() bool { return len(got.list()) == 2 })
	mu.Lock()
	fresh := tokens[len(tokens)-1]
	mu.Unlock()
	if first == fresh {
		t.Fatal("a resumed worker reused its token")
	}
	text, _ = os.ReadFile(path)
	if !strings.Contains(string(text), `"method":"session/resume"`) {
		t.Fatal("wake did not resume the existing ACP session")
	}
	h.runner.Close()
	if _, _, ok := h.runner.ResolveMCPToken(fresh); ok {
		t.Fatal("daemon shutdown retained a non-held worker token")
	}
	h.start()
	if st := h.state(id); st.Process || st.Status == StatusClosed {
		t.Fatalf("restart restores an open sleeping worker: %+v", st)
	}
}

func TestAgyWorkerEffortMechanisms(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"", "config-suffix", "config-default", "acp-suffix", "unsupported", "ignored"} {
		t.Run(fmt.Sprintf("mode=%s", mode), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "acp.jsonl")
			spec := StartSpec{Root: t.TempDir(), Model: "gemini-4-hydrogen", Effort: "medium", Env: append(os.Environ(), agyWorkerTestEnv+"="+path, "WSAGENT_AGY_WORKER_MODE="+mode)}
			if mode == "config-default" {
				spec.Effort = ""
				spec.Env = append(spec.Env, "WSAGENT_AGY_WORKER_MODE=config-suffix")
			}
			connected := make(chan struct{})
			close(connected)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
			defer server.Close()
			spec.MCPSession = &MCPSession{URL: server.URL, Token: "fixture", Connected: connected}
			proc, err := (AgyBackend{Command: exe}).Start(context.Background(), spec)
			if mode == "unsupported" || mode == "ignored" {
				if err == nil {
					_ = proc.Close()
					t.Fatal("unhonored effort must fail a worker start")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer proc.Close()
			p := proc.(*agyProcess)
			if p.holder != nil {
				t.Fatal("worker acquired a holder")
			}
			wantEffort := "medium"
			if mode == "config-default" {
				wantEffort = "high"
			}
			found := false
			for len(p.events) > 0 {
				ev := <-p.events
				if strings.Contains(ev.Notice, "honored effort "+wantEffort+" via") {
					found = true
				}
			}
			if !found {
				t.Fatal("the honored effort mechanism was not recorded")
			}
			models, _, _ := (AgyBackend{}).Choices(context.Background())
			for _, model := range models {
				if base, suffix := agyBaseModel(model.ID); suffix != "" || base != model.ID {
					t.Errorf("choice exposes a level suffix: %s", model.ID)
				}
			}
		})
	}
}
