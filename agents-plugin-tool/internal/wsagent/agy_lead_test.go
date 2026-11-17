package wsagent

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// agyFake is an ACP agent on a fake holder's child: it answers the
// handshake and the controls ws writes, and reaches the ws MCP route with
// the token the session request carries, as agy does before it answers
// that request, unless refuseMCP is set (agy drops a refused server
// silently). Prompts it leaves unanswered: a test writes the turn.
type agyFake struct {
	h         *leadHarness
	mu        sync.Mutex
	refuseMCP bool
	tokens    []string
	// replyID names the session whose registration a session request
	// reads; rowPIDs are the host_pid it named at each one.
	replyID string
	rowPIDs []int
}

func newAgyLeadHarness(t *testing.T) (*leadHarness, *agyFake) {
	t.Helper()
	h := newLeadHarness(t)
	h.opts.Backends[BackendAgy] = BackendRegistration{Backend: AgyBackend{Command: "/opt/harness/agy"}, LeadProfile: LeadProfileAgy, PermissionModes: AgyPermissionModes, Catalog: AgyCatalog()}
	h.opts.WSMCP = func(root string) (MCPCommand, error) {
		return MCPCommand{Command: "python3", Env: map[string]string{"WS_MCP_PROJECT_ROOT": root}}, nil
	}
	f := &agyFake{h: h}
	h.holders.acp = f.answer
	return h, f
}

const agyFakeConfig = `[{"id":"model","currentValue":"gemini-3.8-flash","options":[{"value":"gemini-3.8-flash"},{"value":"claude-sonnet-4-6"}]},` +
	`{"id":"thought_level","currentValue":"high","options":[{"value":"low"},{"value":"medium"},{"value":"high"}]}]`

func (f *agyFake) answer(_, method string, params json.RawMessage) (string, string, bool) {
	switch method {
	case "initialize":
		return `{"protocolVersion":1,"agentInfo":{"name":"Antigravity ACP Server","version":"1.5.10"},"agentCapabilities":{"sessionCapabilities":{"resume":{}}}}`, "", true
	case "session/new", "session/resume":
		var p struct {
			SessionID  string `json:"sessionId"`
			MCPServers []struct {
				Headers []struct {
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"mcpServers"`
		}
		_ = json.Unmarshal(params, &p)
		token := strings.TrimPrefix(p.MCPServers[0].Headers[0].Value, "Bearer ")
		f.mu.Lock()
		f.tokens = append(f.tokens, token)
		refuse := f.refuseMCP
		if row, err := wsdashboard.ReadRegistration(f.h.dir, f.replyID); err == nil {
			f.rowPIDs = append(f.rowPIDs, row.HostPID)
		} else {
			f.rowPIDs = append(f.rowPIDs, -1)
		}
		f.mu.Unlock()
		if !refuse {
			f.h.runner.ResolveMCPToken(token)
		}
		sid := p.SessionID
		if sid == "" {
			sid = "agy-s1"
		}
		return `{"sessionId":"` + sid + `","modes":{"currentModeId":"default","availableModes":[{"id":"default"},{"id":"accept-edits"},{"id":"auto"},{"id":"plan"}]},"configOptions":` + agyFakeConfig + `}`, "", true
	case "session/set_config_option":
		var p struct {
			ConfigID string `json:"configId"`
			Value    string `json:"value"`
		}
		_ = json.Unmarshal(params, &p)
		var opts []map[string]any
		_ = json.Unmarshal([]byte(agyFakeConfig), &opts)
		for _, o := range opts {
			if o["id"] == p.ConfigID {
				o["currentValue"] = p.Value
			}
		}
		raw, _ := json.Marshal(map[string]any{"configOptions": opts})
		return string(raw), "", true
	case "session/set_mode":
		return `{}`, "", true
	}
	return "", "", false
}

func acpField(t *testing.T, raw json.RawMessage, path ...string) any {
	t.Helper()
	return jsonAt(raw, path...)
}

// promptIDs are the ids of the prompts written to the holder's child.
func agyPromptIDs(h *leadHarness, holder string) []string {
	var out []string
	for _, line := range h.holders.inputs(holder, "") {
		var d struct {
			ID     string `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal([]byte(line), &d) == nil && d.Method == "session/prompt" {
			out = append(out, d.ID)
		}
	}
	return out
}

// TestAgyLeadOverACP: a hosted agy lead starts `agy acp
// --process-per-session` under a holder, opens its session with the ws
// MCP server and its token in the session request (never in argv or the
// environment), applies the conversation's model, effort and mode, sends
// the turn as a prompt, takes an approval by its tool call, and ends the
// turn at the prompt's answer; a later daemon rejoins the process without
// a new handshake and its next turn goes to the same session.
func TestAgyLeadOverACP(t *testing.T) {
	h, f := newAgyLeadHarness(t)
	const endpoint = "http://127.0.0.1:1"
	h.startAt(endpoint)
	id := h.lead(BackendAgy, LeadOptions{Model: "claude-sonnet-4-6", Effort: "low", PermissionMode: "auto"}).ID
	f.replyID = id
	h.send(id, "hello")
	holder := h.holder(id)
	eventually(t, "the first prompt", func() bool { return len(agyPromptIDs(h, holder)) == 1 })

	spawn := h.holders.spawned(holder)
	if n := len(spawn.Argv); n < 3 || !slices.Equal(spawn.Argv[n-2:], []string{"acp", "--process-per-session"}) {
		t.Fatalf("argv %q", spawn.Argv)
	}
	rec := h.record(id)
	token := rec.MCPToken
	if len(token) != 64 || strings.Contains(strings.Join(spawn.Argv, " "), token) {
		t.Fatalf("a token is drawn and stays out of argv: %q", token)
	}
	for name, value := range spawn.Env {
		if strings.Contains(value, token) || strings.HasPrefix(name, "WS_MCP_") || strings.HasPrefix(name, "CLAUDE") || name == "WS_MAILBOX" {
			t.Fatalf("the lead's environment carries %s=%s", name, value)
		}
	}
	if spawn.Env[EnvHostedSession] != "1" || spawn.Env[EnvHostedSessionKey] != rec.WSKey {
		t.Fatalf("the hosted-session variables: %v", spawn.Env)
	}
	news := h.holders.acpInputs(holder, "session/new")
	if len(news) != 1 || h.holders.acpInputs(holder, "initialize") == nil {
		t.Fatalf("one handshake: %d session/new", len(news))
	}
	if acpField(t, news[0], "mcpServers") == nil || !sameDir(acpField(t, news[0], "cwd").(string), h.root) {
		t.Fatalf("session/new %s", news[0])
	}
	var server struct {
		MCPServers []struct {
			Type, Name, URL string
			Headers         []struct{ Name, Value string }
		} `json:"mcpServers"`
	}
	_ = json.Unmarshal(news[0], &server)
	if s := server.MCPServers; len(s) != 1 || s[0].Type != "http" || s[0].Name != ReservedMCPServer || s[0].URL != endpoint+wsdashboard.ProxyHTTPPath ||
		len(s[0].Headers) != 1 || s[0].Headers[0].Name != "Authorization" || s[0].Headers[0].Value != "Bearer "+token {
		t.Fatalf("the ws MCP server entry: %+v", s)
	}
	var settings []string
	for _, p := range h.holders.acpInputs(holder, "session/set_config_option") {
		settings = append(settings, acpField(t, p, "configId").(string)+"="+acpField(t, p, "value").(string))
	}
	for _, p := range h.holders.acpInputs(holder, "session/set_mode") {
		settings = append(settings, "mode="+acpField(t, p, "modeId").(string))
	}
	if !slices.Equal(settings, []string{"model=claude-sonnet-4-6", "thought_level=low", "mode=auto"}) {
		t.Fatalf("settings applied at start: %v", settings)
	}
	prompt := h.holders.acpInputs(holder, "session/prompt")[0]
	if acpField(t, prompt, "sessionId") != "agy-s1" || acpField(t, prompt, "prompt") == nil {
		t.Fatalf("the prompt: %s", prompt)
	}
	var text struct {
		Prompt []struct{ Type, Text string } `json:"prompt"`
	}
	_ = json.Unmarshal(prompt, &text)
	if len(text.Prompt) != 1 || text.Prompt[0].Text != h.envelopes("hello")[0] {
		t.Fatalf("the prompt text: %+v", text.Prompt)
	}
	eventually(t, "the session id recorded", func() bool { return h.record(id).SessionID == "agy-s1" })
	if hello, _, ok := h.runner.ResolveMCPToken(token); !ok || !sameDir(hello.ProjectRoot, h.root) || hello.Env[EnvHostedSessionKey] != rec.WSKey {
		t.Fatalf("the token resolves to the lead's ws-mcp context: %+v %v", hello, ok)
	}
	if len(f.tokens) != 1 || f.tokens[0] != token {
		t.Fatalf("agy got the recorded token: %v", f.tokens)
	}
	// ws-mcp confirms the hosted session by the row's host_pid while agy
	// connects inside its session request.
	if len(f.rowPIDs) != 1 || f.rowPIDs[0] != rec.HostPID || rec.HostPID == 0 {
		t.Fatalf("the row named %v at the session request; the process is %d", f.rowPIDs, rec.HostPID)
	}

	// An approval, keyed by its tool call, answered by option kind with
	// the request's raw id.
	h.holders.out(holder,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"working"}}}}`,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"tool_call","toolCallId":"c1","name":"run_command","status":"in_progress","args":{"CommandLine":"ls"}}}}`,
		`{"jsonrpc":"2.0","method":"session/request_permission","params":{"sessionId":"agy-s1","toolCall":{"toolCallId":"c1","name":"run_command","title":"ls","args":{"CommandLine":"ls"}},"options":[{"optionId":"allow","kind":"allow_once"},{"optionId":"deny","kind":"reject_once"}]},"id":"1"}`,
	)
	eventually(t, "the approval", func() bool {
		a := h.state(id).Approvals
		return len(a) == 1 && a[0].ID == "c1" && a[0].Tool == "run_command"
	})
	if _, err := h.runner.AnswerApproval(id, "c1", true, ""); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the answer", func() bool {
		return slices.ContainsFunc(h.holders.inputs(holder, ""), func(line string) bool {
			return strings.Contains(line, `"id":"1"`) && strings.Contains(line, `"optionId":"allow"`)
		})
	})
	h.holders.out(holder,
		`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"agy-s1","update":{"sessionUpdate":"tool_call_update","toolCallId":"c1","name":"run_command","status":"completed"}}}`,
		`{"jsonrpc":"2.0","result":{"stopReason":"end_turn"},"id":"`+agyPromptIDs(h, holder)[0]+`"}`,
	)
	h.idle(id)
	if st := h.state(id); len(st.Approvals) != 0 || st.Error != "" {
		t.Fatalf("after the turn: %+v", st)
	}

	// The next daemon rejoins: no new process, no new handshake, the token
	// restored, the next turn on the same session.
	h.runner.Close()
	h.holders.restart()
	h.startAt(endpoint)
	eventually(t, "the token restored", func() bool {
		_, _, ok := h.runner.ResolveMCPToken(token)
		return ok
	})
	h.send(id, "again")
	eventually(t, "the second prompt", func() bool { return len(agyPromptIDs(h, holder)) == 2 })
	if h.holders.opCount("spawn") != 1 || len(h.holders.acpInputs(holder, "initialize")) != 1 {
		t.Fatalf("a rejoin starts nothing: %v", h.holders.opList())
	}
	if got := h.holders.acpInputs(holder, "session/prompt")[1]; acpField(t, got, "sessionId") != "agy-s1" {
		t.Fatalf("the rejoined prompt: %s", got)
	}
	ids := agyPromptIDs(h, holder)
	if ids[0] == ids[1] {
		t.Fatalf("a later daemon's prompt ids are its own: %v", ids)
	}
	h.holders.out(holder, `{"jsonrpc":"2.0","result":{"stopReason":"end_turn"},"id":"`+ids[1]+`"}`)
	h.idle(id)
}

// TestAgyLeadRefusesAStartWithoutWSTools: a start with no HTTP MCP route,
// or one whose session never reaches the route with its token (agy only
// drops a refused server), fails loudly instead of running a lead without
// ws tools.
func TestAgyLeadRefusesAStartWithoutWSTools(t *testing.T) {
	restore := agyMCPConnectWait
	agyMCPConnectWait = 200 * time.Millisecond
	t.Cleanup(func() { agyMCPConnectWait = restore })

	t.Run("no route", func(t *testing.T) {
		h, _ := newAgyLeadHarness(t)
		h.start()
		id := h.lead(BackendAgy, LeadOptions{}).ID
		if _, err := h.runner.Send(id, "hello"); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the refused start", func() bool { return strings.Contains(h.state(id).Error, "HTTP MCP route") })
		if n := h.holders.opCount("spawn"); n != 0 {
			t.Fatalf("no process starts: %v", h.holders.opList())
		}
	})
	t.Run("route not reached", func(t *testing.T) {
		h, f := newAgyLeadHarness(t)
		f.refuseMCP = true
		h.startAt("http://127.0.0.1:1")
		id := h.lead(BackendAgy, LeadOptions{}).ID
		f.replyID = id
		if _, err := h.runner.Send(id, "hello"); err != nil {
			t.Fatal(err)
		}
		eventually(t, "the refused start", func() bool { return strings.Contains(h.state(id).Error, "did not reach the ws MCP route") })
		h.quiesce(id)
		if rec := h.record(id); rec.MCPToken != "" || rec.HolderID != "" || rec.HostPID != 0 {
			t.Fatalf("the failed start leaves no token or holder: %+v", rec)
		}
		if row, err := wsdashboard.ReadRegistration(h.dir, id); err != nil || row.HostPID != 0 || !row.HostStopped {
			t.Fatalf("the row the start named its process in says stopped: %+v %v", row, err)
		}
		h.runner.mu.Lock()
		live := len(h.runner.mcpTokens)
		h.runner.mu.Unlock()
		if live != 0 {
			t.Fatalf("%d live tokens after the failed start", live)
		}
	})
}

// TestAgyIsLeadOnly: agy has a builtin lead profile and no agent-worker
// profile, so no agy tier is spawnable.
func TestAgyIsLeadOnly(t *testing.T) {
	if LeadProfileFor(BackendAgy) != LeadProfileAgy {
		t.Fatalf("lead profile %q", LeadProfileFor(BackendAgy))
	}
	if agentWorkerBackends[BackendAgy] {
		t.Fatal("agy is not a worker backend")
	}
	for name, raw := range BuiltinProfiles() {
		var p Profile
		if json.Unmarshal(raw, &p) == nil && p.Backend == BackendAgy && !p.Lead {
			t.Fatalf("builtin %s is a non-lead agy profile", name)
		}
	}
	if _, err := (AgyBackend{}).Start(context.Background(), StartSpec{}); err == nil || !strings.Contains(err.Error(), "lead") {
		t.Fatalf("a non-lead start is refused: %v", err)
	}
}
