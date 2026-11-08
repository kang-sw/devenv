package wsagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

func piSpec(t *testing.T, raw string) StartSpec {
	t.Helper()
	spec := claudeSpec(t, raw)
	spec.Dir = "/sess"
	return spec
}

func TestPiArgsAllOffByDefault(t *testing.T) {
	args := piArgs(piSpec(t, `{"backend":"pi"}`), "")
	for _, banned := range []string{"--system-prompt", "--extension", "-e", "--tools", "--model", "--thinking"} {
		if slices.Contains(args, banned) {
			t.Fatalf("args must not carry %s: %v", banned, args)
		}
	}
	for _, want := range []string{"--no-extensions", "--no-skills", "--no-prompt-templates", "--no-context-files", "--no-approve", "--no-tools"} {
		if !slices.Contains(args, want) {
			t.Fatalf("args lack %s: %v", want, args)
		}
	}
	checks := map[string]string{
		"--mode":                 "rpc",
		"--session":              "/sess/pi/session.jsonl",
		"--session-dir":          "/sess/pi",
		"--append-system-prompt": "",
	}
	for flag, want := range checks {
		if got, ok := flagValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}
}

func TestPiArgsFromProfile(t *testing.T) {
	spec := piSpec(t, `{"backend":"pi","tools":["read","bash"],"ws_tools":["tickets.query","project.tree"],
		"mcp_servers":{"docs-x":{"command":"docs-mcp"}},"plugins":["ext.ts","/abs/other.ts"],"skills":true}`)
	spec.Instructions = "/sess/instructions.md"
	spec.Model, spec.Effort = "openai-codex/gpt-6.1-sol", "low"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	args := piArgs(spec, "/sess/pi/ws-agent-mcp.ts")

	var extensions []string
	for i, a := range args {
		if a == "--extension" {
			extensions = append(extensions, args[i+1])
		}
	}
	if want := []string{"builtin:mcp", "/sess/pi/ws-agent-mcp.ts", "/work/ext.ts", "/abs/other.ts"}; !slices.Equal(extensions, want) {
		t.Fatalf("extensions = %v, want %v", extensions, want)
	}
	if !slices.Contains(args, "--no-extensions") || slices.Contains(args, "--no-skills") || slices.Contains(args, "--no-tools") {
		t.Fatalf("explicit extensions load under --no-extensions; skills stay on: %v", args)
	}
	checks := map[string]string{
		"--tools":                "read,bash,mcp__ws__tickets_query,mcp__ws__project_tree,mcp__docs_x__*",
		"--append-system-prompt": "/sess/instructions.md",
		"--model":                "openai-codex/gpt-6.1-sol",
		"--thinking":             "low",
	}
	for flag, want := range checks {
		if got, ok := flagValue(args, flag); !ok || got != want {
			t.Fatalf("%s = %q (present %v), want %q", flag, got, ok, want)
		}
	}
}

func TestPiToolSurface(t *testing.T) {
	spec := piSpec(t, `{"backend":"pi","tools":["read"],"ws_tools":["tickets.query"],"mcp_servers":{"docs":{"command":"d"}}}`)
	if got := piToolSurface(spec); len(got.entries) != 2 || got.allows("mcp__ws__tickets_query") {
		t.Fatalf("ws tools join the surface only when ws-mcp is attached: %+v", got)
	}
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	s := piToolSurface(spec)
	for _, ok := range []string{"read", "mcp__ws__tickets_query", "mcp__docs__search"} {
		if !s.allows(ok) {
			t.Fatalf("%s should be allowed", ok)
		}
	}
	err := checkPiTools([]string{"read", "bash", "mcp__ws__git_commit", "mcp__docs__search"}, s)
	if err == nil || !strings.Contains(err.Error(), "launch refused") || !strings.Contains(err.Error(), "bash, mcp__ws__git_commit") || strings.Contains(err.Error(), "docs") {
		t.Fatalf("tools outside the profile should be refused by name, got %v", err)
	}
	if err := checkPiToolEntries(Profile{Tools: []string{"read", "bash(git status)"}}); err == nil || !strings.Contains(err.Error(), "permission rule") {
		t.Fatalf("a permission rule would widen to the whole tool on pi, got %v", err)
	}
}

func TestPiBridgeRegistersOnlyTheProfileServers(t *testing.T) {
	spec := piSpec(t, `{"backend":"pi","ws_tools":["tickets.query"],"mcp_servers":{"docs":{"command":"docs-mcp"},"lazy":{"command":"l","exposure":"deferred"}}}`)
	spec.Root = "/work"
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp", Args: []string{"serve", "--stdio"}, Env: map[string]string{EnvAllowedTools: "tickets.query"}}
	servers, err := piMCPServers(spec)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path, err := writePiBridge(dir, servers)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 || path != filepath.Join(dir, piBridgeFile) {
		t.Fatalf("the bridge carries the conversation key's environment, so it is private: %s %v %v", path, info, err)
	}
	src, _ := os.ReadFile(path)
	text := string(src)
	start := strings.Index(text, "= ") + 2
	end := strings.Index(text, ";\n")
	var got map[string]map[string]any
	if err := json.Unmarshal([]byte(text[start:end]), &got); err != nil {
		t.Fatalf("bridge servers: %v\n%s", err, text)
	}
	if len(got) != 3 || got["ws"]["command"] != "/bin/ws-mcp" || got["ws"]["cwd"] != "/work" || got["ws"]["exposure"] != "direct" {
		t.Fatalf("ws-mcp should register direct at the root: %v", got)
	}
	if env, _ := got["ws"]["env"].(map[string]any); env[EnvAllowedTools] != "tickets.query" {
		t.Fatalf("ws-mcp's allowlist travels in its env: %v", got["ws"])
	}
	if got["docs"]["exposure"] != "direct" || got["lazy"]["exposure"] != "deferred" {
		t.Fatalf("a server's own exposure is kept, else direct: %v", got)
	}
	if !strings.Contains(text, "export default function") || !strings.Contains(text, "registerMcpServer(name, config)") {
		t.Fatalf("bridge source:\n%s", text)
	}
}

func TestPiEnv(t *testing.T) {
	got := piEnv([]string{"PATH=/bin", "WS_PI_ROLE=lead", "WS_MAILBOX=lead@m", "WS_MAILBOX_AUTO=1", "PI_SKIP_VERSION_CHECK=0", "HOME=/h"})
	slices.Sort(got)
	want := []string{"HOME=/h", "PATH=/bin", "PI_SKIP_VERSION_CHECK=1"}
	if !slices.Equal(got, want) {
		t.Fatalf("piEnv = %v, want %v", got, want)
	}
}

// recordedReplies keeps what a mapper answers and sends.
type recordedReplies struct {
	answers []string
	sent    []map[string]any
}

func (r *recordedReplies) answer(id string, _ controlAnswer) { r.answers = append(r.answers, id) }
func (r *recordedReplies) send(v any) {
	raw, _ := json.Marshal(v)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	r.sent = append(r.sent, m)
}

// replayPi runs a recorded RPC fixture through the mapper into a wslive
// state, with the runner's turn boundaries around each turn. before sees
// each line first, so a test can stand in for Send and Interrupt.
func replayPi(t *testing.T, fixture string, spec StartSpec, before func(line string, m *piMapper)) (*wslive.State, []wslive.RingEntry, []Event, *recordedReplies) {
	t.Helper()
	f, err := os.Open("testdata/" + fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m := newPiMapper(spec)
	clock := int64(1_000_000)
	m.now = func() time.Time { clock += 100; return time.UnixMilli(clock) }
	st := wslive.NewState("test-key")
	var ring []wslive.RingEntry
	var all []Event
	replies := &recordedReplies{}
	apply := func(ev wslive.Event) {
		res, err := st.Apply(ev, clock)
		if err != nil {
			t.Fatalf("apply %s: %v", ev.Kind, err)
		}
		ring = append(ring, res.Ring...)
	}
	inTurn := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if before != nil {
			before(line, m)
		}
		if !inTurn && strings.Contains(line, `"command":"prompt"`) {
			apply(wslive.Event{Kind: wslive.KindTurnStart, Origin: wslive.OriginWhiteboard})
			inTurn = true
		}
		for _, ev := range m.mapLine([]byte(line), replies) {
			all = append(all, ev)
			switch {
			case ev.Live != nil:
				apply(*ev.Live)
			case ev.TurnEnd:
				apply(wslive.Event{Kind: wslive.KindTurnEnd})
				inTurn = false
			}
		}
	}
	return st, ring, all, replies
}

func piToolTurnSpec(t *testing.T) StartSpec {
	spec := piSpec(t, `{"backend":"pi","tools":["read"],"ws_tools":["runtime.read","project.tree"]}`)
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	return spec
}

func TestPiMapperToolTurn(t *testing.T) {
	_, ring, events, replies := replayPi(t, "pi-tool-turn.jsonl", piToolTurnSpec(t), nil)
	ends := 0
	for _, ev := range events {
		if ev.Fatal != nil {
			t.Fatalf("a permitted fixture should not refuse: %v", ev.Fatal)
		}
		if ev.TurnEnd {
			ends++
			if ev.TurnError != "" {
				t.Fatalf("a clean turn ends with no error: %q", ev.TurnError)
			}
		}
	}
	if ends != 1 {
		t.Fatalf("one prompt is one turn, got %d ends", ends)
	}
	var reply string
	for _, e := range ring {
		if e.Kind == wslive.RingReply {
			reply = e.Text
		}
	}
	batches := map[int64]bool{}
	for _, ev := range events {
		if ev.Live != nil && ev.Live.Kind == wslive.KindAgent {
			var b wireBatch
			if err := json.Unmarshal(ev.Live.Batches[0], &b); err != nil {
				t.Fatal(err)
			}
			batches[b.Seq] = true
		}
	}
	last := ring[len(ring)-1]
	if reply != "alpha" || last.Kind != wslive.RingTurn || last.Done != 2 || last.Failed != 1 {
		t.Fatalf("reply %q, last %+v: want the reply and three calls, one failing", reply, last)
	}
	if len(batches) != 3 {
		t.Fatalf("each assistant message's calls are one batch: %v", batches)
	}
	if len(replies.sent) != 1 || replies.sent[0]["type"] != "get_session_stats" {
		t.Fatalf("the settled turn asks for session statistics: %v", replies.sent)
	}
}

func TestPiMapperRefusesUnlistedTool(t *testing.T) {
	spec := piSpec(t, `{"backend":"pi","tools":["read"]}`)
	_, _, events, _ := replayPi(t, "pi-tool-turn.jsonl", spec, nil)
	if len(events) == 0 || events[0].Fatal == nil || !strings.Contains(events[0].Fatal.Error(), "mcp__ws__project_tree, mcp__ws__runtime_read") {
		t.Fatalf("the declared tool set should be refused before any call, got %+v", events)
	}
}

// TestPiMapperAbortAndResume: an interrupted run ends with no error and its
// cut-off call failed; the resumed session answers the next prompt, and the
// statistics carry the session id, context, and cost.
func TestPiMapperAbortAndResume(t *testing.T) {
	spec := piSpec(t, `{"backend":"pi","tools":["bash"]}`)
	st, ring, events, _ := replayPi(t, "pi-abort-resume.jsonl", spec, func(line string, m *piMapper) {
		switch {
		case strings.Contains(line, `"id":"p1"`):
			m.aborted.Store(true) // Interrupt during the first prompt
		case strings.Contains(line, `"id":"p2"`):
			m.aborted.Store(false) // Send of the second prompt
		}
	})
	var ends []Event
	sessionID := ""
	for _, ev := range events {
		if ev.Fatal != nil {
			t.Fatalf("unexpected refusal: %v", ev.Fatal)
		}
		if ev.TurnEnd {
			ends = append(ends, ev)
		}
		if ev.SessionID != "" {
			sessionID = ev.SessionID
		}
	}
	if len(ends) != 2 || ends[0].TurnError != "" || ends[1].TurnError != "" {
		t.Fatalf("an interrupt is no error: %+v", ends)
	}
	var turns []wslive.RingEntry
	for _, e := range ring {
		if e.Kind == wslive.RingTurn {
			turns = append(turns, e)
		}
	}
	if len(turns) != 2 || turns[0].Failed != 1 || ring[len(ring)-2].Text != "alpha" {
		t.Fatalf("the aborted call fails and the resumed turn replies: turns %+v", turns)
	}
	if sessionID != "01a1b8ce-7aa7-7047-86e9-1abee8a00288" {
		t.Fatalf("session id from the statistics = %q", sessionID)
	}
	snap := st.Snapshot(2_000_000)
	if snap.Usage == nil || snap.Usage.Context == nil || snap.Usage.Context.Window != 500000 || snap.Usage.Cost == nil || !snap.Usage.Cost.Estimate {
		t.Fatalf("usage should carry the context window and an estimated cost: %+v", snap.Usage)
	}

	// The same run without an interrupt asked for: the abort is the error.
	_, _, events, _ = replayPi(t, "pi-abort-resume.jsonl", spec, nil)
	for _, ev := range events {
		if ev.TurnEnd {
			if ev.TurnError != "This operation was aborted" {
				t.Fatalf("an abort no one asked for should fail the turn, got %q", ev.TurnError)
			}
			break
		}
	}
}

func TestPiMapperPromptAnswersAndDialogs(t *testing.T) {
	m := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	r := &recordedReplies{}
	cases := []struct {
		line    string
		end     bool
		turnErr string
	}{
		{`{"id":"ws-1","type":"response","command":"prompt","success":false,"error":"busy"}`, true, "prompt refused: busy"},
		{`{"id":"ws-2","type":"response","command":"prompt","success":true,"data":{"disposition":"handled"}}`, true, ""},
		{`{"id":"ws-3","type":"response","command":"prompt","success":true,"data":{"disposition":"started"}}`, false, ""},
		{`{"id":"ws-4","type":"response","command":"prompt","success":true,"data":{"disposition":"queued"}}`, false, ""},
	}
	for _, tc := range cases {
		evs := m.mapLine([]byte(tc.line), r)
		ended := len(evs) > 0 && evs[len(evs)-1].TurnEnd
		if ended != tc.end || (ended && evs[len(evs)-1].TurnError != tc.turnErr) {
			t.Fatalf("%s: events %+v, want end %v error %q", tc.line, evs, tc.end, tc.turnErr)
		}
	}
	if !slices.Equal(r.answers, []string{"ws-1", "ws-2", "ws-3", "ws-4"}) {
		t.Fatalf("every response answers its request: %v", r.answers)
	}

	m.mapLine([]byte(`{"type":"extension_ui_request","id":"u1","method":"confirm","title":"Allow?"}`), r)
	m.mapLine([]byte(`{"type":"extension_ui_request","id":"u2","method":"notify","message":"hi"}`), r)
	if len(r.sent) != 1 || r.sent[0]["type"] != "extension_ui_response" || r.sent[0]["id"] != "u1" || r.sent[0]["cancelled"] != true {
		t.Fatalf("a blocking dialog is cancelled, a notification ignored: %v", r.sent)
	}

	start := m.mapLine([]byte(`{"type":"compaction_start","reason":"threshold"}`), r)
	end := m.mapLine([]byte(`{"type":"compaction_end","reason":"manual"}`), r)
	if len(start) != 1 || start[0].Live.Kind != wslive.KindCompact || start[0].Live.Phase != wslive.CompactStart || start[0].Live.Trigger != wslive.TriggerAuto ||
		len(end) != 1 || end[0].Live.Phase != wslive.CompactEnd || end[0].Live.Trigger != wslive.TriggerManual {
		t.Fatalf("compaction maps to compact events: %+v %+v", start, end)
	}
}

// piResponder is a stand-in pi that answers every command with an id and
// logs each line it reads.
const piResponder = `import sys, json
log = open(sys.argv[1], "a")
for line in sys.stdin:
    log.write(line); log.flush()
    d = json.loads(line)
    if "id" in d:
        ok = d.get("type") != "set_thinking_level" or d.get("level") != "bogus"
        print(json.dumps({"type": "response", "id": d["id"], "command": d["type"], "success": ok, "error": "" if ok else "bad level"}), flush=True)
`

func TestPiProcessCommands(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	logPath := filepath.Join(t.TempDir(), "commands.jsonl")
	m := newPiMapper(piSpec(t, `{"backend":"pi"}`))
	sp, err := startStreamProcess(exec.Command(python, "-I", "-c", piResponder, logPath), m, piProtocol{})
	if err != nil {
		t.Fatal(err)
	}
	p := &piProcess{streamProcess: sp, mapper: m}
	var drained sync.WaitGroup
	drained.Add(1)
	go func() {
		defer drained.Done()
		for range p.Events() {
		}
	}()

	for _, model := range []string{"", "gpt", "openai/gpt:high", "/x"} {
		if err := p.SetModel(model); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("SetModel(%q) needs a restart, got %v", model, err)
		}
	}
	if err := p.SetEffort(""); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("a return to the default effort needs a restart, got %v", err)
	}
	if err := p.SetModel("openai-codex/gpt-a"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetEffort("high"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetEffort("bogus"); err == nil || !strings.Contains(err.Error(), "bad level") {
		t.Fatalf("a refused level is the error, got %v", err)
	}
	if err := p.SetModel("openai-codex/gpt-b"); err != nil {
		t.Fatal(err)
	}
	if err := p.Send("hello"); err != nil || m.aborted.Load() {
		t.Fatal(err)
	}
	if err := p.Interrupt(); err != nil || !m.aborted.Load() {
		t.Fatalf("Interrupt: %v", err)
	}
	if err := p.Send("again"); err != nil || m.aborted.Load() {
		t.Fatalf("a send clears the interrupt: %v", err)
	}
	p.Close()
	drained.Wait()

	raw, _ := os.ReadFile(logPath)
	var got []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var d map[string]any
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		s := d["type"].(string)
		for _, k := range []string{"modelId", "level", "message"} {
			if v, ok := d[k].(string); ok {
				s += " " + v
			}
		}
		got = append(got, s)
	}
	want := []string{
		"set_model gpt-a",
		"set_thinking_level high", "set_thinking_level bogus",
		// set_model resets pi's thinking level, so the set effort follows it.
		"set_model gpt-b", "set_thinking_level high",
		"prompt hello", "abort", "prompt again",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("commands = %q, want %q", got, want)
	}
}

// TestPiBackendStart drives Start against a stand-in pi that records its
// command line, working directory, and environment.
func TestPiBackendStart(t *testing.T) {
	out := t.TempDir()
	fake := filepath.Join(t.TempDir(), "pi")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + out + "/args\npwd -P > " + out + "/cwd\nenv > " + out + "/env\ntouch " + out + "/ready\ncat > /dev/null\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Close SIGTERMs a child that outlives closeGrace, so under load it could
	// kill the script before it wrote its records. Close only once the script
	// reports it finished writing.
	ready := filepath.Join(out, "ready")
	closeWhenWritten := func(proc Process) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for {
			if _, err := os.Stat(ready); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the stand-in pi never finished writing its records")
			}
			time.Sleep(5 * time.Millisecond)
		}
		_ = os.Remove(ready)
		proc.Close()
	}
	root, dir := t.TempDir(), t.TempDir()
	spec := piSpec(t, `{"backend":"pi","tools":["read"],"ws_tools":["tickets.query"]}`)
	spec.Root, spec.Dir = root, dir
	spec.WSMCP = &MCPCommand{Command: "/bin/ws-mcp"}
	spec.Env = []string{"PATH=" + os.Getenv("PATH"), "WS_PI_ROLE=lead"}
	proc, err := PiBackend{Command: fake}.Start(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	closeWhenWritten(proc)

	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	args := strings.Split(strings.TrimSpace(read("args")), "\n")
	bridge := filepath.Join(dir, "pi", piBridgeFile)
	if got, _ := flagValue(args, "--tools"); got != "read,mcp__ws__tickets_query" || !slices.Contains(args, bridge) {
		t.Fatalf("args = %v", args)
	}
	if info, err := os.Stat(bridge); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("bridge: %v %v", info, err)
	}
	wantRoot, _ := filepath.EvalSymlinks(root)
	if cwd := strings.TrimSpace(read("cwd")); cwd != wantRoot {
		t.Fatalf("pi runs at the working root: %q, want %q", cwd, wantRoot)
	}
	if env := read("env"); !strings.Contains(env, "PI_SKIP_VERSION_CHECK=1") || strings.Contains(env, "WS_PI_ROLE") {
		t.Fatalf("child env:\n%s", env)
	}

	// No MCP server allowed: no bridge, no MCP extension.
	plain := piSpec(t, `{"backend":"pi","tools":["read"]}`)
	plain.Root, plain.Dir, plain.Env = root, t.TempDir(), spec.Env
	proc, err = PiBackend{Command: fake}.Start(context.Background(), plain)
	if err != nil {
		t.Fatal(err)
	}
	closeWhenWritten(proc)
	if args := read("args"); strings.Contains(args, "builtin:mcp") {
		t.Fatalf("no MCP server, no MCP extension: %s", args)
	}
	if _, err := os.Stat(filepath.Join(plain.Dir, "pi", piBridgeFile)); !os.IsNotExist(err) {
		t.Fatalf("no bridge is written without a server: %v", err)
	}

	rule := piSpec(t, `{"backend":"pi","tools":["bash(git status)"]}`)
	rule.Root, rule.Dir = root, t.TempDir()
	if _, err := (PiBackend{Command: fake}).Start(context.Background(), rule); err == nil || !strings.Contains(err.Error(), "permission rule") {
		t.Fatalf("a permission rule is refused before launch, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(rule.Dir, "pi")); !os.IsNotExist(err) {
		t.Fatalf("a refused launch leaves no session state: %v", err)
	}

	restore := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = restore })
	if _, err := (PiBackend{}).Start(context.Background(), plain); err == nil || !strings.Contains(err.Error(), "pi backend") {
		t.Fatalf("a missing pi names the backend, got %v", err)
	}
}
