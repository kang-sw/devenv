package wsagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsholder"
	"github.com/kang-sw/devenv/internal/wslive"
)

// This opt-in acceptance uses the installed Codex and its existing login,
// the real Runner and Python holder, and a loopback MCP stand-in. It never
// starts/replaces the shared daemon or edits the user's configuration/auth.
// The ordinary suite uses recorded streams and synthetic holders instead.
func TestCodexInstalledAcceptance(t *testing.T) {
	if os.Getenv("WS_CODEX_LIVE") != "1" {
		t.Skip("set WS_CODEX_LIVE=1 to authorize installed-CLI/provider acceptance")
	}
	command, err := exec.LookPath("codex")
	if err != nil {
		t.Fatal(err)
	}
	h := newLeadHarness(t)
	// Replace the synthetic harness's daemon environment with this machine's,
	// but keep hook/launcher state away from the owner's shared ws state.
	environ = func() []string {
		return append(os.Environ(), "WS_CACHE_HOME="+h.opts.Config.CacheHome, "WS_CONFIG_HOME="+h.opts.Config.ConfigHome)
	}
	manager := wsholder.New(wsholder.Options{Dir: filepath.Join(h.dir, "holders"), Log: h.log})
	t.Cleanup(manager.Close)
	capture := &codexLiveCapture{ManagerHost: ManagerHost{Manager: manager}}
	manager.AddSink(capture)
	h.opts.Holders = capture
	h.opts.Backends[BackendCodex] = BackendRegistration{
		Backend: CodexBackend{Command: command}, LeadProfile: LeadProfileCodex,
		PermissionModes: CodexPermissionModes, Catalog: CodexCatalog(),
	}
	h.opts.WSMCP = func(root string) (MCPCommand, error) {
		return MCPCommand{Env: map[string]string{"WS_MCP_PROJECT_ROOT": root}}, nil
	}
	h.opts.LeadManual = func(root, key, backend string) (string, error) {
		return "Adapter acceptance: use only the read-only codex_probe_echo tool when asked. Do not use shell or other tools. Answer plainly.", nil
	}

	var mu sync.Mutex
	calls, stdioChildren := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		hello, _, valid := h.runner.ResolveMCPToken(token)
		if !valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.NewDecoder(req.Body).Decode(&message) != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(message.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any = map[string]any{}
		switch message.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}},
				"serverInfo": map[string]any{"name": "ws-adapter-acceptance", "version": "1"},
			}
		case "tools/list":
			result = map[string]any{"tools": []any{map[string]any{
				"name": "codex_probe_echo", "description": "Read-only adapter acceptance echo. No side effects.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}, "required": []string{"text"}},
				"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false},
			}}}
		case "tools/call":
			var params struct {
				Name      string `json:"name"`
				Arguments struct {
					Text string `json:"text"`
				} `json:"arguments"`
			}
			_ = json.Unmarshal(message.Params, &params)
			if params.Name != "codex_probe_echo" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			children := codexLiveStdioChildren(hello.OwnerPID)
			mu.Lock()
			calls++
			stdioChildren += children
			mu.Unlock()
			result = map[string]any{"content": []any{map[string]any{"type": "text", "text": "ECHO:" + params.Arguments.Text}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": message.ID, "result": result})
	}))
	defer server.Close()
	h.startAt(server.URL)
	id := h.lead(BackendCodex, LeadOptions{Effort: "low", PermissionMode: "read-only"}).ID
	t.Cleanup(func() { _, _ = h.runner.CloseConversation(id) })

	await := func(what string, done func(State) bool) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Minute)
		for time.Now().Before(deadline) {
			state := h.state(id)
			if state.Error != "" {
				t.Fatalf("%s: %s\n%s", what, state.Error, h.log.String())
			}
			// The only approval this test may grant is the read-only echo.
			for _, approval := range state.Approvals {
				if approval.Tool != "mcpServer/elicitation/request" || !strings.Contains(string(approval.Input), "codex_probe_echo") {
					t.Fatalf("unexpected live approval: %s", approval.Tool)
				}
				if _, err := h.runner.AnswerApproval(id, approval.ID, true, ""); err != nil {
					t.Fatal(err)
				}
			}
			if done(state) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %s\n%s", what, h.log.String())
	}
	send := func(text string) {
		t.Helper()
		if _, err := h.runner.Send(id, text); err != nil {
			t.Fatal(err)
		}
	}
	finished := func(marker string) func(State) bool {
		return func(state State) bool {
			if !state.Process || state.Status != StatusIdle {
				return false
			}
			return slices.ContainsFunc(ringTexts(h.ring(id), wslive.RingReply), func(text string) bool { return strings.Contains(text, marker) })
		}
	}
	send("Call codex_probe_echo with text CODEX-ADAPTER-OK, then reply with its exact result. No other tool.")
	await("live echo", finished("ECHO:CODEX-ADAPTER-OK"))
	first := h.record(id)
	if first.SessionID == "" || first.HostPID == 0 || first.HolderID == "" {
		t.Fatalf("missing native/holder identity: %+v", h.state(id))
	}
	mu.Lock()
	gotCalls, gotChildren := calls, stdioChildren
	mu.Unlock()
	if gotCalls != 1 || gotChildren != 0 {
		t.Fatalf("echo calls=%d, descendant stdio ws-mcp processes=%d", gotCalls, gotChildren)
	}

	// Exercise the real native compaction lifecycle, not just its schema.
	before := len(h.ring(id))
	h.sendCommand(id, "compact", "")
	await("native compaction", func(state State) bool {
		if state.Status != StatusIdle {
			return false
		}
		for _, entry := range h.ring(id)[before:] {
			if entry.Kind == wslive.RingCompact {
				return true
			}
		}
		return false
	})

	// Verify that the installed paginated thread also supports the full
	// history read used only for a lost-ring reconciliation.
	var process *codexProcess
	h.runner.conv(id).do(func() { process = h.runner.conv(id).proc.(*codexProcess) })
	if _, err := process.TurnRunning(); err != nil {
		t.Fatalf("installed recovery read: %v", err)
	}

	// A stopped process resumes the same persistent thread with a fresh MCP
	// token. Its response and the next turn supply installed-version proof.
	if _, err := h.runner.Stop(id); err != nil {
		t.Fatal(err)
	}
	send("Call codex_probe_echo with text CODEX-RESUME-OK, then reply with its exact result. No other tool.")
	await("live resume", finished("ECHO:CODEX-RESUME-OK"))
	second := h.record(id)
	if second.SessionID != first.SessionID || second.MCPToken == first.MCPToken || second.HolderID == first.HolderID {
		t.Fatal("resume did not preserve thread identity and refresh transport credentials")
	}
	models, efforts, err := (CodexBackend{}).Choices(context.Background())
	if err != nil || len(models) == 0 || len(efforts) == 0 {
		t.Fatalf("installed model/list choices: %d models %v efforts: %v", len(models), efforts, err)
	}
	t.Logf("installed acceptance: thread retained; create, echo, compact, stop/resume; models=%d efforts=%v; stdio ws-mcp descendants=%d", len(models), efforts, gotChildren)
	if path := os.Getenv("WS_CODEX_LIVE_CAPTURE"); path != "" {
		home, _ := os.UserHomeDir()
		capture.save(t, path, map[string]string{
			home: "/home/user", h.root: "/work/acceptance", h.dir: "/work/ws-state",
			first.WSKey: "fixture-lead-key", first.MCPToken: "fixture-token-1", second.MCPToken: "fixture-token-2",
			server.URL: "http://127.0.0.1:54321",
		})
	}
}

// codexLiveCapture observes the real holder wire, not environment/credentials.
type codexLiveCapture struct {
	ManagerHost
	mu    sync.Mutex
	lines []string
}

func (c *codexLiveCapture) Input(id string, data []byte, eof bool) error {
	c.mu.Lock()
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			c.lines = append(c.lines, ">> "+line)
		}
	}
	c.mu.Unlock()
	return c.ManagerHost.Input(id, data, eof)
}

func (c *codexLiveCapture) HolderEvent(_ string, event wsholder.Event) {
	if event.Event == "output" && event.Stream == "stdout" {
		c.mu.Lock()
		c.lines = append(c.lines, strings.TrimSpace(string(event.Data)))
		c.mu.Unlock()
	}
}

func (c *codexLiveCapture) save(t *testing.T, path string, replacements map[string]string) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	text := strings.Join(c.lines, "\n") + "\n"
	for from, to := range replacements {
		if from != "" {
			text = strings.ReplaceAll(text, from, to)
		}
	}
	// macOS may report the canonical /private spelling of a temp cwd.
	text = strings.ReplaceAll(text, "/private/work/", "/work/")
	text, err := sanitizeCodexCapture(text)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Keep protocol structure and opaque conversation ids, not account state or
// machine identity. The substituted account/limit values are deliberately
// synthetic; they are not evidence of the recording account's subscription.
func sanitizeCodexCapture(text string) (string, error) {
	var out strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		prefix := ""
		if strings.HasPrefix(line, ">> ") {
			prefix, line = ">> ", strings.TrimPrefix(line, ">> ")
		}
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			return "", err
		}
		if message["method"] == "remoteControl/status/changed" {
			if params, ok := message["params"].(map[string]any); ok {
				message["params"] = map[string]any{"status": params["status"]}
			}
		}
		sanitizeCodexCaptureValue(message)
		raw, err := json.Marshal(message)
		if err != nil {
			return "", err
		}
		out.WriteString(prefix)
		out.Write(raw)
		out.WriteByte('\n')
	}
	return out.String(), nil
}

func sanitizeCodexCaptureValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
			switch normalized {
			case "cwd", "hostname", "accountid", "email", "installationid", "environmentid", "projectid",
				"codexhome", "runtimeworkspaceroots", "sourcepath", "path", "gitinfo", "instructionsources",
				"credits", "individuallimit", "spendcontrolreached", "ratelimitreachedtype", "availableaccessprograms":
				delete(value, key)
			case "plantype":
				value[key] = "synthetic-plan"
			case "authmode":
				value[key] = "synthetic-auth"
			case "usedpercent":
				value[key] = float64(42)
			case "resetsat":
				value[key] = float64(2_000_000_000)
			case "balance":
				value[key] = "<redacted>"
			case "useragent":
				value[key] = "synthetic-agent"
			default:
				sanitizeCodexCaptureValue(child)
			}
		}
	case []any:
		for _, child := range value {
			sanitizeCodexCaptureValue(child)
		}
	}
}

// Count the actual Codex process's descendant stdio ws-mcp launchers, not
// unrelated shared-daemon sessions. A negative result fails acceptance.
func codexLiveStdioChildren(pid int) int {
	raw, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil || pid <= 0 {
		return -1
	}
	type process struct {
		pid, parent int
		command     string
	}
	var processes []process
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		child, _ := strconv.Atoi(fields[0])
		parent, _ := strconv.Atoi(fields[1])
		processes = append(processes, process{child, parent, strings.Join(fields[2:], " ")})
	}
	descendants := map[int]bool{pid: true}
	for changed := true; changed; {
		changed = false
		for _, process := range processes {
			if descendants[process.parent] && !descendants[process.pid] {
				descendants[process.pid], changed = true, true
			}
		}
	}
	count := 0
	for _, process := range processes {
		if process.pid != pid && descendants[process.pid] && strings.Contains(process.command, "ws-mcp") && strings.Contains(process.command, "--stdio") {
			count++
		}
	}
	return count
}
