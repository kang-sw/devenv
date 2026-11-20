// Claude lead launch and process control tests.

package wsagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestClaudeLeadArgs(t *testing.T) {
	spec := leadSpec()
	spec.Root = t.TempDir()
	spec.Env = []string{"CLAUDE_CONFIG_DIR=" + t.TempDir()}
	spec.Lead.PermissionMode = "plan"
	spec.Model, spec.Effort, spec.SessionID = "opus", "high", "sess-1"
	// The instruction file (the hosted key and manual) appends to the
	// lead's prompt; the daemon's ws-mcp is its ws server, beside the
	// user's own servers (no strict config).
	spec.Instructions = "/sess/instructions.md"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	// A hosted lead removes the native subagent tool from the user's setup,
	// and only that: the one allow rule is the ws server's, and no tool
	// list replaces the user's tools.
	spec.RemoveSubagents = RemovesSubagents(Record{Backend: BackendClaude, Lead: true})
	args, err := claudeArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"--plugin-dir", "--setting-sources", "--strict-mcp-config", "--tools", "--disable-slash-commands",
		"--append-system-prompt", "--bare", "--system-prompt", "--system-prompt-file", "--dangerously-skip-permissions", "--no-session-persistence"} {
		if slices.Contains(args, banned) {
			t.Fatalf("a lead's args must not carry %s: %v", banned, args)
		}
	}
	for _, want := range []string{"-p", "--verbose"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args lack %s: %v", want, args)
		}
	}
	for flag, want := range map[string]string{
		"--input-format": "stream-json", "--output-format": "stream-json",
		"--permission-prompt-tool": "stdio", "--permission-mode": "plan",
		"--model": "opus", "--effort": "high", "--resume": "sess-1",
		"--settings":                  `{"enabledPlugins":{}}`,
		"--disallowedTools":           "Agent,Task",
		"--append-system-prompt-file": "/sess/instructions.md",
	} {
		if got, ok := flagValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}
	if server := leadMCPServer(t, args); server["type"] != "stdio" || server["command"] != "/bin/ws-mcp" {
		t.Fatalf("the ws server is the daemon's ws-mcp: %v", server)
	}
	if n := len(args); !slices.Equal(args[n-2:], []string{"--allowedTools", "mcp__ws"}) {
		t.Fatalf("the ws allow rule must come last: %v", args)
	}
	// The variadic option's one value is followed by an option, never by
	// an argument it would swallow.
	if i := slices.Index(args, "--disallowedTools"); i+2 < len(args) && !strings.HasPrefix(args[i+2], "--") {
		t.Fatalf("--disallowedTools is followed by a bare argument: %v", args)
	}

	plainSpec := leadSpec()
	plainSpec.Root, plainSpec.Env, plainSpec.WSMCP = spec.Root, spec.Env, spec.WSMCP
	plain, err := claudeArgs(plainSpec)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := flagValue(plain, "--permission-mode"); got != DefaultLeadPermissionMode {
		t.Fatalf("--permission-mode = %q, want the default %q", got, DefaultLeadPermissionMode)
	}
	if got, _ := flagValue(plain, "--permission-prompt-tool"); got != "stdio" {
		t.Fatalf("--permission-prompt-tool = %q", got)
	}
	for _, absent := range []string{"--resume", "--model", "--effort", "--disallowedTools", "--append-system-prompt-file"} {
		if slices.Contains(plain, absent) {
			t.Fatalf("a new lead session with no choices carries no %s: %v", absent, plain)
		}
	}
}

func TestClaudeLeadEnv(t *testing.T) {
	spec := leadSpec()
	spec.Env = []string{
		"PATH=/bin", "HOME=/h", "WS_CACHE_HOME=/c", "WS_CONFIG_HOME=/cfg",
		// The daemon's own session: its harness variables, mailbox
		// identity, ws-mcp launcher state, and hosted-session variables.
		"CLAUDECODE=1", "CLAUDE_CODE_ENTRYPOINT=cli", "CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=0", "CLAUDE_CONFIG_DIR=/claude",
		"WS_MAILBOX=lead@m", "WS_MAILBOX_AUTO=1", "WS_SKILLS_ROOT=/starter/skills",
		"WS_MCP_ROOT=/elsewhere", "WS_MCP_ALLOWED_TOOLS=tracker.query", "WS_MCP_NO_DASHBOARD=1", "WS_RSRC=/rsrc", "WS_RSRC_DIR=/rsrc",
		"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=the-daemons-own", "WS_HOSTED_SESSION_ROW=/old/row", "WS_HOSTED_OTHER=x",
	}
	spec.Lead.Env = map[string]string{EnvHostedSession: "1", EnvHostedSessionKey: "lead-key", EnvHostedSessionRow: "/state/row.json"}
	got := claudeLeadEnv(spec)
	slices.Sort(got)
	want := []string{
		"CLAUDE_CODE_EMIT_SESSION_STATE_EVENTS=1", "CLAUDE_CONFIG_DIR=/claude", "HOME=/h", "PATH=/bin", "WS_CACHE_HOME=/c", "WS_CONFIG_HOME=/cfg",
		"WS_HOSTED_SESSION=1", "WS_HOSTED_SESSION_KEY=lead-key", "WS_HOSTED_SESSION_ROW=/state/row.json",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("claudeLeadEnv =\n%v\nwant\n%v", got, want)
	}
}

// loopChild is a streamProcess child that records each input line and
// answers every control request with a bare success, as Claude 2.1.322
// answers apply_flag_settings.
type loopChild struct {
	p     *streamProcess
	mu    sync.Mutex
	lines [][]byte
}

func (c *loopChild) name() string { return "claude" }

func (c *loopChild) write(line []byte) error {
	c.mu.Lock()
	c.lines = append(c.lines, bytes.TrimSpace(line))
	c.mu.Unlock()
	var d struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(line, &d) == nil && d.Type == "control_request" {
		answer, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": d.RequestID}})
		go c.p.feed(answer, 0, false)
	}
	return nil
}

func (c *loopChild) kill()                {}
func (c *loopChild) stop(<-chan struct{}) {}

func (c *loopChild) written() [][]byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([][]byte(nil), c.lines...)
}

// controlRequest splits a control_request line into its request id and its
// request body.
func controlRequest(t *testing.T, line []byte) (id string, request any) {
	t.Helper()
	var d struct {
		Type      string `json:"type"`
		RequestID string `json:"request_id"`
		Request   any    `json:"request"`
	}
	if err := json.Unmarshal(line, &d); err != nil || d.Type != "control_request" {
		t.Fatalf("not a control request: %s (%v)", line, err)
	}
	return d.RequestID, d.Request
}

// TestClaudeLeadProcessControls pins a lead's in-stream controls against
// the lines the recording's driver sent: apply_flag_settings for the
// effort, set_permission_mode for the mode, and the two approval answers.
// A return to the default effort is a null level and to the default model
// a set_model that names none (both seen to clear the override on Claude
// 2.1.322). A profile conversation takes the effort and model controls
// too, and has no mode control.
func TestClaudeLeadProcessControls(t *testing.T) {
	child := &loopChild{}
	sp := newStreamProcess(newClaudeMapper(leadSpec()), claudeProtocol{})
	sp.child, child.p = child, sp
	p := claudeProcess{streamProcess: sp, lead: true}

	if err := p.SetEffort("low"); err != nil {
		t.Fatalf("SetEffort: %v", err)
	}
	if err := p.SetPermissionMode("acceptEdits"); err != nil {
		t.Fatalf("SetPermissionMode: %v", err)
	}
	if err := p.SetEffort(""); err != nil {
		t.Fatalf("SetEffort to the default: %v", err)
	}
	if err := p.SetModel("opus"); err != nil {
		t.Fatalf("SetModel: %v", err)
	}
	if err := p.SetModel(""); err != nil {
		t.Fatalf("SetModel to the default: %v", err)
	}
	if err := p.SetPermissionMode(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("an empty mode has no in-stream form, got %v", err)
	}
	allow := readLeadFixture(t, "claude-lead-approval-allow.jsonl")
	deny := readLeadFixture(t, "claude-lead-approval-deny.jsonl")
	approval := func(lines []leadLine) ApprovalRequest {
		evs := newClaudeMapper(leadSpec()).mapLine(lines[findLine(t, lines, 0, "can_use_tool", isCanUseTool)].raw, nopReplies{})
		if len(evs) != 1 || evs[0].Approval == nil {
			t.Fatalf("no approval in the fixture: %+v", evs)
		}
		return *evs[0].Approval
	}
	if err := p.AnswerApproval(approval(allow), true, "ignored for an allow"); err != nil {
		t.Fatal(err)
	}
	if err := p.AnswerApproval(approval(deny), false, "probe"); err != nil {
		t.Fatal(err)
	}
	// A deny with no message still tells the model who refused.
	bare := approval(deny)
	if err := p.AnswerApproval(bare, false, "  "); err != nil {
		t.Fatal(err)
	}
	// An allow of a request recorded with no input hands back an object.
	bare.Input = nil
	if err := p.AnswerApproval(bare, true, ""); err != nil {
		t.Fatal(err)
	}

	got := child.written()
	if len(got) != 9 {
		t.Fatalf("input lines = %d, want 9:\n%s", len(got), bytes.Join(got, []byte("\n")))
	}
	settings := readLeadFixture(t, "claude-lead-settings.jsonl")
	sent := func(subtype string) []byte {
		return settings[findLine(t, settings, 0, subtype, func(l leadLine) bool { return l.in && bytes.Contains(l.raw, []byte(`"`+subtype+`"`)) })].raw
	}
	for i, want := range [][]byte{
		sent("apply_flag_settings"), sent("set_permission_mode"),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"apply_flag_settings","settings":{"effortLevel":null}}}`),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"set_model","model":"opus"}}`),
		[]byte(`{"type":"control_request","request_id":"-","request":{"subtype":"set_model"}}`),
	} {
		id, request := controlRequest(t, got[i])
		_, recorded := controlRequest(t, want)
		if id == "" || !reflect.DeepEqual(request, recorded) {
			t.Fatalf("control request %d =\n%s\nwant the request of\n%s", i, got[i], want)
		}
	}
	for i, want := range [][]byte{
		allow[findLine(t, allow, 0, "sent control_response", isAnswer)].raw,
		deny[findLine(t, deny, 0, "sent control_response", isAnswer)].raw,
		[]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"dedd1842-7da7-40fc-91f4-a30bf7f23279","response":{"behavior":"deny","message":"The user denied this tool call."}}}`),
		[]byte(`{"type":"control_response","response":{"subtype":"success","request_id":"dedd1842-7da7-40fc-91f4-a30bf7f23279","response":{"behavior":"allow","updatedInput":{}}}}`),
	} {
		if !sameJSON(t, json.RawMessage(got[5+i]), want) {
			t.Fatalf("approval answer %d =\n%s\nwant\n%s", i, got[5+i], want)
		}
	}

	// A profile conversation's process: the effort control too (its
	// launch settings stay, since the control merges into them), and no
	// mode control.
	plain := &loopChild{}
	psp := newStreamProcess(newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`)), claudeProtocol{})
	psp.child, plain.p = plain, psp
	pp := claudeProcess{streamProcess: psp}
	if err := pp.SetEffort("high"); err != nil {
		t.Fatalf("SetEffort on a profile conversation: %v", err)
	}
	if err := pp.SetPermissionMode("plan"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("a profile conversation's mode is not the user's to change, got %v", err)
	}
	lines := plain.written()
	if len(lines) != 1 {
		t.Fatalf("input lines = %s", bytes.Join(lines, []byte("\n")))
	}
	if _, request := controlRequest(t, lines[0]); !reflect.DeepEqual(request, map[string]any{"subtype": "apply_flag_settings", "settings": map[string]any{"effortLevel": "high"}}) {
		t.Fatalf("effort request = %s", lines[0])
	}
}
