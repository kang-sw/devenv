package wsagent

import (
	"bufio"
	"encoding/json"
	"os"
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
	for flag, want := range map[string]string{"--append-system-prompt-file": "/sess/instructions.md", "--model": "sonnet", "--effort": "high", "--resume": "sess-1", "--plugin-dir": "/work/plug"} {
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
