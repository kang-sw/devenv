package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func claudeSpec(t *testing.T, raw string) StartSpec {
	t.Helper()
	p, err := ParseProfile(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return StartSpec{Root: "/work", Profile: p}
}

func flagValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func TestClaudeArgsAllOffByDefault(t *testing.T) {
	args, err := claudeArgs(claudeSpec(t, `{"backend":"claude"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"--bare", "--system-prompt", "--system-prompt-file", "--no-session-persistence", "--dangerously-skip-permissions"} {
		if slices.Contains(args, banned) {
			t.Fatalf("args must never carry %s: %v", banned, args)
		}
	}
	for _, want := range []string{"-p", "--verbose", "--strict-mcp-config", "--disable-slash-commands"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args lack %s: %v", want, args)
		}
	}
	checks := map[string]string{
		"--input-format":    "stream-json",
		"--output-format":   "stream-json",
		"--setting-sources": "",
		"--tools":           "",
		"--permission-mode": "dontAsk",
		"--mcp-config":      `{"mcpServers":{}}`,
	}
	for flag, want := range checks {
		if got, ok := flagValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}
	settings, _ := flagValue(args, "--settings")
	if !strings.Contains(settings, `"cc-plugin-agents-md@builtin":false`) {
		t.Fatalf("--settings should turn the built-in plugins off: %s", settings)
	}
	if slices.Contains(args, "--allowedTools") || slices.Contains(args, "--append-system-prompt-file") || slices.Contains(args, "--resume") {
		t.Fatalf("an empty profile allows nothing, appends nothing, resumes nothing: %v", args)
	}
}

func TestClaudeArgsFromProfile(t *testing.T) {
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read","Bash(git status)","Bash(git log *)"],
		"ws_tools":["tickets.query"],"mcp_servers":{"docs":{"command":"docs-mcp"}},
		"plugins":["plug"],"skills":true}`)
	spec.Instructions = "/sess/instructions.md"
	spec.Model, spec.Effort, spec.SessionID = "sonnet", "high", "sess-1"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio"}, Env: map[string]string{EnvAllowedTools: "tickets.query"}}
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := flagValue(args, "--tools"); got != "Read,Bash" {
		t.Fatalf("--tools should hold each base tool once: %q", got)
	}
	for flag, want := range map[string]string{"--append-system-prompt-file": "/sess/instructions.md", "--model": "sonnet", "--effort": "high", "--resume": "sess-1", "--plugin-dir": filepath.Join("/work", "plug")} {
		if got, _ := flagValue(args, flag); got != want {
			t.Fatalf("%s = %q, want %q", flag, got, want)
		}
	}
	if slices.Contains(args, "--disable-slash-commands") {
		t.Fatalf("skills on keeps slash commands")
	}
	i := slices.Index(args, "--allowedTools")
	if i < 0 {
		t.Fatalf("no --allowedTools: %v", args)
	}
	allowed := args[i+1:]
	want := []string{"Read", "Bash(git status)", "Bash(git log *)", "mcp__ws__tickets_query", "mcp__docs"}
	if !slices.Equal(allowed, want) {
		t.Fatalf("--allowedTools must be last and hold %v, got %v", want, allowed)
	}
	mcp, _ := flagValue(args, "--mcp-config")
	var cfg struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(mcp), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.MCPServers["ws"].Command != "/bin/ws-mcp" || cfg.MCPServers["ws"].Env[EnvAllowedTools] != "tickets.query" || cfg.MCPServers["docs"].Command != "docs-mcp" {
		t.Fatalf("--mcp-config should hold ws and the profile's servers: %s", mcp)
	}
}

// TestClaudeArgsBuiltinSupportProfile: the builtin Claude support profile
// launches with Agent as its only built-in tool, in --tools and in the
// dontAsk allowlist beside its ws tools, so a subagent it spawns inherits
// the same limit.
func TestClaudeArgsBuiltinSupportProfile(t *testing.T) {
	p, err := ParseProfile(BuiltinProfiles()[ProfilePrefix+SupportProfileClaude])
	if err != nil {
		t.Fatal(err)
	}
	spec := StartSpec{Root: "/work", Profile: p, WSMCP: &MCPCommand{Command: "/bin/ws-mcp"}}
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := flagValue(args, "--tools"); got != "Agent" {
		t.Fatalf("--tools = %q, want Agent alone", got)
	}
	i := slices.Index(args, "--allowedTools")
	if i < 0 {
		t.Fatalf("no --allowedTools: %v", args)
	}
	want := []string{"Agent", "mcp__ws__tracker_query", "mcp__ws__tracker_update", "mcp__ws__tracker_show", "mcp__ws__tracker_ui", "mcp__ws__wiki_query", "mcp__ws__wiki_update", "mcp__ws__git_search", "mcp__ws__playbook_read"}
	if allowed := args[i+1:]; !slices.Equal(allowed, want) {
		t.Fatalf("--allowedTools = %v, want %v", allowed, want)
	}
}

func TestHarnessEnvDropsParentSession(t *testing.T) {
	env := []string{"PATH=/bin", "CLAUDECODE=1", "CLAUDE_CODE_PLUGIN_DIRS=/x", "CLAUDE_CODE_SESSION_ID=s", "CLAUDE_CONFIG_DIR=/cfg", "CLAUDE_CODE_OAUTH_TOKEN=t", "WS_MAILBOX=lead@machine", "WS_MAILBOX_AUTO=1", "HOME=/h"}
	got := claudeEnv(env)
	want := []string{"PATH=/bin", "CLAUDE_CONFIG_DIR=/cfg", "CLAUDE_CODE_OAUTH_TOKEN=t", "HOME=/h"}
	if !slices.Equal(got, want) {
		t.Fatalf("harnessEnv = %v, want %v", got, want)
	}
}

func TestCheckClaudeInit(t *testing.T) {
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read","Bash(git status)"],"ws_tools":["tickets.query"],"mcp_servers":{"docs":{"command":"d"}},"plugins":["plug"]}`)
	spec.WSMCP = &MCPCommand{Command: "ws-mcp"}
	ok := claudeInit{
		Tools:          []string{"Read", "Bash", "mcp__ws__tickets_query", "mcp__docs__search"},
		PermissionMode: "dontAsk",
	}
	ok.MCPServers = append(ok.MCPServers, struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}{Name: "ws"}, struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}{Name: "docs"})
	ok.Plugins = append(ok.Plugins, struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Source string `json:"source"`
	}{Name: "cc-plugin-telemetry", Path: "builtin", Source: "cc-plugin-telemetry@builtin"}, struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Source string `json:"source"`
	}{Name: "plug", Path: "/work/plug", Source: "plug@inline"})
	if err := checkClaudeInit(ok, spec); err != nil {
		t.Fatalf("a permitted init should pass: %v", err)
	}

	extraTool := ok
	extraTool.Tools = append(slices.Clone(ok.Tools), "Write", "mcp__ws__config_tune")
	if err := checkClaudeInit(extraTool, spec); err == nil || !strings.Contains(err.Error(), "Write") || !strings.Contains(err.Error(), "mcp__ws__config_tune") {
		t.Fatalf("extra tools should refuse the launch naming them, got %v", err)
	}
	extraServer := ok
	extraServer.MCPServers = append(slices.Clone(ok.MCPServers), struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}{Name: "claude.ai Gmail"})
	if err := checkClaudeInit(extraServer, spec); err == nil {
		t.Fatalf("an extra MCP server should refuse the launch")
	}
	// Claude's init names the Agent tool "Task": a profile allowing Agent
	// accepts it, and one that does not refuses it.
	task := ok
	task.Tools = append(slices.Clone(ok.Tools), "Task")
	if err := checkClaudeInit(task, spec); err == nil || !strings.Contains(err.Error(), "Task") {
		t.Fatalf("Task without Agent in the profile should refuse the launch, got %v", err)
	}
	agentSpec := spec
	agentSpec.Profile.Tools = append(slices.Clone(spec.Profile.Tools), "Agent")
	if err := checkClaudeInit(task, agentSpec); err != nil {
		t.Fatalf("Task with Agent in the profile should pass: %v", err)
	}
	mode := ok
	mode.PermissionMode = "default"
	if err := checkClaudeInit(mode, spec); err == nil {
		t.Fatalf("a permission mode other than dontAsk should refuse the launch")
	}
}

// replayClaude runs a recorded stream-json fixture through the mapper into a
// wslive state, with the runner's own turn boundaries around it.
func replayClaude(t *testing.T, fixture string, spec StartSpec) (*wslive.State, []wslive.RingEntry, []Event) {
	t.Helper()
	f, err := os.Open("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := newClaudeMapper(spec)
	clock := int64(1_000_000)
	m.now = func() time.Time { clock += 100; return time.UnixMilli(clock) }
	st := wslive.NewState("test-key")
	var ring []wslive.RingEntry
	var all []Event
	apply := func(ev wslive.Event) {
		res, err := st.Apply(ev, clock)
		if err != nil {
			t.Fatalf("apply %s: %v", ev.Kind, err)
		}
		ring = append(ring, res.Ring...)
	}
	apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: wslive.OriginWhiteboard})
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		for _, ev := range m.mapLine(sc.Bytes(), nopReplies{}) {
			all = append(all, ev)
			switch {
			case ev.Live != nil:
				apply(*ev.Live)
			case ev.TurnEnd:
				apply(wslive.Event{Kind: wslive.KindTurnEnd})
			}
		}
	}
	return st, ring, all
}

func TestClaudeMapperToolTurn(t *testing.T) {
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read"]}`)
	st, ring, events := replayClaude(t, "claude-tool-turn.jsonl", spec)
	var sessionID string
	var turnEnd *Event
	for i, ev := range events {
		if ev.Fatal != nil {
			t.Fatalf("a permitted fixture should not refuse: %v", ev.Fatal)
		}
		if ev.SessionID != "" {
			sessionID = ev.SessionID
		}
		if ev.TurnEnd {
			turnEnd = &events[i]
		}
	}
	if sessionID == "" || turnEnd == nil || turnEnd.TurnError != "" {
		t.Fatalf("want a session id and a clean turn end: id %q, end %+v", sessionID, turnEnd)
	}
	var kinds []string
	var tools, turn *wslive.RingEntry
	for i, e := range ring {
		kinds = append(kinds, e.Kind)
		switch e.Kind {
		case wslive.RingTools:
			tools = &ring[i]
		case wslive.RingTurn:
			turn = &ring[i]
		}
	}
	if len(ring) == 0 || ring[len(ring)-1].Kind != wslive.RingTurn {
		t.Fatalf("the ring should end with the turn summary: %v", kinds)
	}
	var reply string
	for _, e := range ring {
		if e.Kind == wslive.RingReply {
			reply = e.Text
		}
	}
	if reply != "alpha" {
		t.Fatalf("the final reply should be in the ring, got %q (kinds %v)", reply, kinds)
	}
	if tools == nil || turn == nil || turn.Done != 1 || turn.Failed != 1 {
		t.Fatalf("two Read calls, one failing, should count done 1 failed 1: tools %+v turn %+v", tools, turn)
	}
	snap := st.Snapshot(2_000_000)
	if snap.Usage == nil || snap.Usage.Context == nil || snap.Usage.Context.Window != 1_000_000 || snap.Usage.Cost == nil || len(snap.Usage.Limits) != 1 || snap.Usage.Limits[0].Kind != wslive.LimitSevenDay {
		t.Fatalf("usage should carry context, window, cost, and the weekly limit: %+v", snap.Usage)
	}
	if snap.Lead.Phase != wslive.PhaseIdle {
		t.Fatalf("after the turn the lead should be idle, got %s", snap.Lead.Phase)
	}
}

func TestClaudeMapperInterruptIsNoError(t *testing.T) {
	spec := claudeSpec(t, `{"backend":"claude","tools":["Bash(sleep *)"]}`)
	_, ring, events := replayClaude(t, "claude-interrupt-turn.jsonl", spec)
	ended := false
	for _, ev := range events {
		if ev.TurnEnd {
			ended = true
			if ev.TurnError != "" {
				t.Fatalf("an interrupted turn is no error, got %q", ev.TurnError)
			}
		}
	}
	if !ended {
		t.Fatalf("the interrupted turn should end")
	}
	last := ring[len(ring)-1]
	if last.Kind != wslive.RingTurn || last.Failed != 1 {
		t.Fatalf("the cut-short call should count as failed: %+v", last)
	}
}

func TestClaudeMapperRefusesUnlistedPlugin(t *testing.T) {
	// Recorded with a parent session's inline plugin directory leaking in
	// through the environment, before the child environment was scrubbed.
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read"]}`)
	_, _, events := replayClaude(t, "claude-inline-plugin-turn.jsonl", spec)
	if len(events) == 0 || events[0].Fatal == nil || !strings.Contains(events[0].Fatal.Error(), "plugin") {
		t.Fatalf("the init check should refuse an unlisted plugin first, got %+v", events)
	}
}

func TestClaudeMapperTurnError(t *testing.T) {
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	evs := m.mapLine([]byte(`{"type":"result","subtype":"success","is_error":true,"result":"You're out of extra usage"}`), nopReplies{})
	end := evs[len(evs)-1]
	if !end.TurnEnd || end.TurnError != "You're out of extra usage" {
		t.Fatalf("a refused turn should end with its error, got %+v", end)
	}
}

// TestClaudeBackendStart drives Start against a stand-in claude that records
// its command line (NUL-separated: the setting-sources value is empty),
// working directory, and environment.
func TestClaudeBackendStart(t *testing.T) {
	fake, out, outEnv := standin(t)
	root := t.TempDir()
	spec := claudeSpec(t, `{"backend":"claude","tools":["Read","Bash(git status)"],"ws_tools":["tickets.query"]}`)
	spec.Root, spec.Dir = root, t.TempDir()
	spec.Model, spec.SessionID, spec.Instructions = "opus", "sess-1", filepath.Join(spec.Dir, "instructions.md")
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	spec.Env = []string{"PATH=" + os.Getenv("PATH"), "CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli",
		"CLAUDE_CONFIG_DIR=/cfg", "WS_MAILBOX_ADDRESS=lead", "HOME=/home/u", outEnv}
	proc, err := ClaudeBackend{Command: fake}.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// Close SIGTERMs a child that outlives closeGrace; close only once the
	// stand-in finished writing its records.
	ready := filepath.Join(out, "ready")
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the stand-in claude never finished writing its records")
		}
	}
	proc.Close()

	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	want, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(read("args"), "\x00")
	if !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
	for flag, value := range map[string]string{"--input-format": "stream-json", "--output-format": "stream-json",
		"--model": "opus", "--resume": "sess-1", "--append-system-prompt-file": spec.Instructions, "--permission-mode": "dontAsk"} {
		if v, _ := flagValue(got, flag); v != value {
			t.Errorf("%s = %q, want %q", flag, v, value)
		}
	}
	if got[0] != "-p" || !slices.Contains(got, "--strict-mcp-config") {
		t.Errorf("print mode with a strict MCP config: %q", got)
	}
	if cwd := read("cwd"); !sameDir(cwd, root) {
		t.Fatalf("claude runs at the working root: %q, want %q", cwd, root)
	}
	env := strings.Split(read("env"), "\n")
	for _, kv := range []string{"CLAUDE_CONFIG_DIR=/cfg", "HOME=/home/u"} {
		if !slices.Contains(env, kv) {
			t.Errorf("child env lacks %s", kv)
		}
	}
	for _, prefix := range []string{"CLAUDECODE=", "CLAUDE_CODE_ENTRYPOINT=", "WS_MAILBOX_"} {
		for _, kv := range env {
			if strings.HasPrefix(kv, prefix) {
				t.Errorf("child env keeps the parent session's %s", kv)
			}
		}
	}

	restore := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = restore })
	if _, err := (ClaudeBackend{}).Start(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "claude backend") {
		t.Fatalf("a missing claude names the backend, got %v", err)
	}
}

// claudeResponder is a stand-in claude that logs each input line and
// answers every control_request: success, except set_model "bogus" (a
// plain error) and set_model "legacy" (an unsupported subtype).
const claudeResponder = `import sys, json
log = open(sys.argv[1], "a", newline="")
for line in sys.stdin:
    log.write(line); log.flush()
    d = json.loads(line)
    if d.get("type") != "control_request":
        continue
    model = d["request"].get("model")
    r = {"subtype": "success", "request_id": d["request_id"]}
    if model == "bogus":
        r = {"subtype": "error", "request_id": d["request_id"], "error": "model not found"}
    elif model == "legacy":
        r = {"subtype": "error", "request_id": d["request_id"], "error": "Unsupported control request subtype: set_model"}
    print(json.dumps({"type": "control_response", "response": r}), flush=True)
`

// TestClaudeProcessControls pins the stream-json input lines: the user line,
// the interrupt and set_model control requests, and how an error answer maps
// (an unsupported one to ErrUnsupported, which makes the runner restart).
func TestClaudeProcessControls(t *testing.T) {
	python := lookPython(t)
	logPath := filepath.Join(t.TempDir(), "input.jsonl")
	sp, err := startStreamProcess(exec.Command(python, "-I", "-c", claudeResponder, logPath),
		newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`)), claudeProtocol{})
	if err != nil {
		t.Fatal(err)
	}
	p := claudeProcess{streamProcess: sp}
	go func() {
		for range p.Events() {
		}
	}()

	if err := p.Send("hello"); err != nil {
		t.Fatal(err)
	}
	if err := p.Interrupt(); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	if err := p.SetModel("opus"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if err := p.SetModel(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("a return to the default model needs a restart, got %v", err)
	}
	if err := p.SetEffort("high"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("effort has no in-stream form, got %v", err)
	}
	if err := p.SetModel("bogus"); err == nil || errors.Is(err, ErrUnsupported) || err.Error() != "model not found" {
		t.Fatalf("a refused model is the error, got %v", err)
	}
	if err := p.SetModel("legacy"); !errors.Is(err, ErrUnsupported) || !strings.Contains(err.Error(), "Unsupported control request subtype") {
		t.Fatalf("an unsupported answer wraps ErrUnsupported, got %v", err)
	}
	p.Close()

	raw, _ := os.ReadFile(logPath)
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{
		`{"message":{"content":"hello","role":"user"},"type":"user"}`,
		`{"request":{"subtype":"interrupt"},"request_id":"ws-1","type":"control_request"}`,
		`{"request":{"model":"opus","subtype":"set_model"},"request_id":"ws-2","type":"control_request"}`,
		`{"request":{"model":"bogus","subtype":"set_model"},"request_id":"ws-3","type":"control_request"}`,
		`{"request":{"model":"legacy","subtype":"set_model"},"request_id":"ws-4","type":"control_request"}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("input lines =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestClaudeMapperSubagentTask pins the support badge's source: Claude runs
// an Agent call's subagent as a background task, whose task_started line
// adds a running subagent to the snapshot and whose completion ends it; the
// subagent's own lines (parent_tool_use_id set) stay out of the lead's
// reply. Recorded with Claude 2.1.319, --tools Agent.
func TestClaudeMapperSubagentTask(t *testing.T) {
	spec := claudeSpec(t, `{"backend":"claude","tools":["Agent"]}`)
	st, ring, events := replayClaude(t, "claude-subagent-turn.jsonl", spec)
	var states []string
	for _, ev := range events {
		if ev.Fatal != nil {
			t.Fatalf("a permitted fixture should not refuse: %v", ev.Fatal)
		}
		if ev.Live != nil && ev.Live.AgentID != "" {
			states = append(states, ev.Live.AgentID+" "+ev.Live.Status+" "+ev.Live.Label)
		}
	}
	want := []string{"a8019a344cc37a38c running say hi", "a8019a344cc37a38c done "}
	if strings.Join(states, "|") != strings.Join(want, "|") {
		t.Fatalf("subagent events = %q, want %q", states, want)
	}
	agents := st.Snapshot(2_000_000).Agents
	if len(agents) != 1 || agents[0].ID != "a8019a344cc37a38c" || agents[0].Status != wslive.StatusDone || agents[0].Label != "say hi" {
		t.Fatalf("snapshot agents: %+v", agents)
	}
	for _, e := range ring {
		if e.Kind == wslive.RingReply && strings.TrimSpace(e.Text) == "hi" {
			t.Fatalf("the subagent's text became a lead reply: %+v", ring)
		}
	}
}

// TestClaudeMapperSubagentTaskEndsOnce: an end for a task never started, a
// non-agent task, and a second end are dropped, so no finished agent appears
// from nothing; a killed task ends failed.
func TestClaudeMapperSubagentTaskEndsOnce(t *testing.T) {
	m := newClaudeMapper(claudeSpec(t, `{"backend":"claude","tools":["Agent"]}`))
	lines := []string{
		`{"type":"system","subtype":"task_notification","task_id":"ghost","status":"completed"}`,
		`{"type":"system","subtype":"task_started","task_id":"sh1","task_type":"local_bash","description":"sleep"}`,
		`{"type":"system","subtype":"task_started","task_id":"ag1","task_type":"local_agent","description":"look"}`,
		`{"type":"system","subtype":"task_updated","task_id":"ag1","patch":{"status":"running"}}`,
		`{"type":"system","subtype":"task_updated","task_id":"ag1","patch":{"status":"killed"}}`,
		`{"type":"system","subtype":"task_notification","task_id":"ag1","status":"killed"}`,
	}
	var got []string
	for _, line := range lines {
		for _, ev := range m.mapLine([]byte(line), nopReplies{}) {
			if ev.Live != nil {
				got = append(got, ev.Live.AgentID+" "+ev.Live.Status)
			}
		}
	}
	if want := "ag1 running|ag1 failed"; strings.Join(got, "|") != want {
		t.Fatalf("events = %q, want %q", got, want)
	}
}

// TestLiveWriterEndAgents: a process gone ends its running subagents failed
// and leaves finished ones as they were.
func TestLiveWriterEndAgents(t *testing.T) {
	w := newLiveWriter(t.TempDir(), "end-test-key", "claude", time.Now, io.Discard)
	w.apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "a", Status: wslive.StatusRunning})
	w.apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "b", Status: wslive.StatusRunning})
	w.apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "b", Status: wslive.StatusDone})
	w.endAgents()
	got := map[string]string{}
	for _, a := range w.st.Snapshot(time.Now().UnixMilli()).Agents {
		got[a.ID] = a.Status
	}
	if got["a"] != wslive.StatusFailed || got["b"] != wslive.StatusDone {
		t.Fatalf("agents after endAgents: %v", got)
	}
}
