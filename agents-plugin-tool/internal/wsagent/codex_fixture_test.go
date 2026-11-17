package wsagent

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The codex-<version>-*.jsonl fixtures are recordings of `codex app-server`
// (stdio JSON-RPC, the "jsonrpc" member omitted on the wire) driven by a
// probe client, cut the way the claude-lead fixtures are: a line the client
// wrote is kept behind a ">> " mark, every other line is server output.
// Paths, account identity, hook commands and bearer tokens are scrubbed and
// mcpServerStatus/list results are reduced to server and tool names
// ("_fixtureTrimmed"); message order and every field a mapper reads are as
// recorded. 261115-feat-ws-hosted-codex-adapter-contract Phase 1 records
// which capability each fixture evidences. These tests pin the recorded
// protocol facts the Codex adapter is built against, so a re-recording on a
// new Codex version that changes one of them fails here first.

// codexMsg is one decoded fixture line.
type codexMsg struct {
	in     bool
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (m codexMsg) str(path ...string) string {
	s, _ := jsonAt(m.Params, path...).(string)
	return s
}

// jsonAt reads the value at path in raw; nil when any step is missing.
func jsonAt(raw json.RawMessage, path ...string) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	for _, k := range path {
		obj, _ := v.(map[string]any)
		v = obj[k]
	}
	return v
}

func readCodexFixture(t *testing.T, name string) []codexMsg {
	t.Helper()
	var out []codexMsg
	for _, line := range readLeadFixture(t, name) {
		var m codexMsg
		if err := json.Unmarshal(line.raw, &m); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		m.in = line.in
		out = append(out, m)
	}
	return out
}

// responseTo finds the server's response to the client request with id.
func responseTo(t *testing.T, msgs []codexMsg, id string) codexMsg {
	t.Helper()
	for _, m := range msgs {
		if !m.in && m.Method == "" && string(m.ID) == id {
			return m
		}
	}
	t.Fatalf("no response to request %s", id)
	return codexMsg{}
}

// codexFixtureLeak matches an e-mail address or an unredacted credit balance.
var codexFixtureLeak = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.]+|"balance":"[0-9]`)

func TestCodexFixturesScrubbed(t *testing.T) {
	names, err := filepath.Glob("testdata/codex-*.jsonl")
	if err != nil || len(names) == 0 {
		t.Fatalf("no codex fixtures: %v", err)
	}
	for _, path := range names {
		for _, line := range readLeadFixture(t, filepath.Base(path)) {
			for _, leak := range []string{"/Users/", "/private/", "/var/folders/", "wsprobe-token-", "orca-agent-hook", "Bearer "} {
				if strings.Contains(string(line.raw), leak) {
					t.Errorf("%s carries unscrubbed %q", path, leak)
				}
			}
			if m := codexFixtureLeak.Find(line.raw); m != nil && string(m) != "user@example.com" {
				t.Errorf("%s carries unscrubbed %q", path, m)
			}
		}
	}
}

func TestCodexHandshakeFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-handshake.jsonl")
	if r := responseTo(t, msgs, "1"); r.Error == nil || r.Error.Message != "Not initialized" {
		t.Fatalf("request before initialize: %+v", r.Error)
	}
	if r := responseTo(t, msgs, "2"); r.Error != nil || !strings.Contains(string(r.Result), `"codexHome"`) {
		t.Fatalf("initialize: %s", r.Result)
	}
	if r := responseTo(t, msgs, "3"); r.Error == nil || r.Error.Message != "Already initialized" {
		t.Fatalf("second initialize: %+v", r.Error)
	}
}

// An interrupted turn ends with status interrupted while the command item it
// started gets no item/completed inside it; the item completes later, during
// the next turn, still naming the interrupted turn.
func TestCodexInterruptLeavesItemOpenFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	interrupted, item, endAt := "", "", -1
	for i, m := range msgs {
		if m.Method == "turn/completed" && m.str("turn", "status") == "interrupted" {
			interrupted, endAt = m.str("turn", "id"), i
			break
		}
	}
	if interrupted == "" {
		t.Fatal("no interrupted turn")
	}
	for _, m := range msgs[:endAt] {
		if m.Method == "item/started" && m.str("turnId") == interrupted && m.str("item", "type") == "commandExecution" {
			item = m.str("item", "id")
		}
		if m.Method == "item/completed" && item != "" && m.str("item", "id") == item {
			t.Fatalf("item %s completed inside the interrupted turn", item)
		}
	}
	if item == "" {
		t.Fatal("interrupted turn started no command item")
	}
	late := false
	for _, m := range msgs[endAt:] {
		if m.Method == "item/completed" && m.str("item", "id") == item {
			late = true
			if m.str("turnId") != interrupted {
				t.Fatalf("late completion names turn %q, want %q", m.str("turnId"), interrupted)
			}
		}
	}
	if !late {
		t.Fatal("the interrupted command item never completed")
	}
}

// turn/steer needs the active turn id, answers with it, starts no new turn,
// and the steered text arrives as a userMessage item inside the same turn.
func TestCodexSteerFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	var steerIDs []string
	for _, m := range msgs {
		if m.in && m.Method == "turn/steer" {
			steerIDs = append(steerIDs, string(m.ID))
		}
	}
	if len(steerIDs) != 3 {
		t.Fatalf("steer requests: %v", steerIDs)
	}
	if r := responseTo(t, msgs, steerIDs[0]); r.Error == nil || !strings.Contains(r.Error.Message, "expected active turn id") {
		t.Fatalf("steer with a stale turn id: %+v", r.Error)
	}
	ok := responseTo(t, msgs, steerIDs[1])
	var res struct {
		TurnID string `json:"turnId"`
	}
	if ok.Error != nil || json.Unmarshal(ok.Result, &res) != nil || res.TurnID == "" {
		t.Fatalf("steer: %s %+v", ok.Result, ok.Error)
	}
	if r := responseTo(t, msgs, steerIDs[2]); r.Error == nil || r.Error.Message != "no active turn to steer" {
		t.Fatalf("steer while idle: %+v", r.Error)
	}
	users, steerAt := 0, -1
	for i, m := range msgs {
		if m.in && string(m.ID) == steerIDs[1] {
			steerAt = i
		}
		if m.Method == "item/started" && m.str("turnId") == res.TurnID && m.str("item", "type") == "userMessage" {
			users++
		}
		if steerAt >= 0 && m.Method == "turn/started" && m.str("turn", "id") == res.TurnID {
			t.Fatal("steer started the turn again")
		}
	}
	if users != 2 {
		t.Fatalf("user messages in the steered turn: %d, want the prompt and the steer", users)
	}
}

// A command approval runs item/started, the server request, the client's
// answer, serverRequest/resolved, then item/completed with the outcome.
func TestCodexCommandApprovalFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	var order []string
	item := ""
	for _, m := range msgs {
		switch {
		case m.Method == "item/commandExecution/requestApproval":
			item = m.str("itemId")
			order = append(order, "request")
		case item == "" && m.Method == "item/started" && m.str("item", "type") == "commandExecution":
			order = append(order[:0], "started:"+m.str("item", "id"))
		case item != "" && m.in && m.Method == "" && len(m.Result) > 0 && len(order) == 2:
			order = append(order, "answer")
		case item != "" && m.Method == "serverRequest/resolved" && len(order) == 3:
			order = append(order, "resolved")
		case item != "" && m.Method == "item/completed" && m.str("item", "id") == item:
			order = append(order, "completed:"+m.str("item", "status"))
		}
		if len(order) == 5 {
			break
		}
	}
	want := []string{"started:" + item, "request", "answer", "resolved", "completed:declined"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("approval order %v, want %v", order, want)
	}
}

// An interrupt while an approval waits ends the turn, then resolves the
// request. The recording ends with the client's two late answers to the
// resolved request: the probe saw no reply and no file written, which the
// recording itself cannot show, so this pins only the order up to them.
func TestCodexStaleApprovalFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-stale-approval.jsonl")
	ended, resolved, answers := -1, -1, 0
	for i, m := range msgs {
		switch {
		case m.Method == "turn/completed" && m.str("turn", "status") == "interrupted":
			ended = i
		case m.Method == "serverRequest/resolved":
			resolved = i
		case m.in && m.Method == "" && resolved >= 0:
			answers++
		}
	}
	if ended < 0 || resolved < ended || answers != 2 {
		t.Fatalf("turn end %d, resolved %d, late answers %d", ended, resolved, answers)
	}
}

func TestCodexResumeAndAdmissionFixture(t *testing.T) {
	msgs := readCodexFixture(t, "codex-0.160.0-resume.jsonl")
	if r := responseTo(t, msgs, "2"); r.Error == nil || !strings.Contains(r.Error.Message, "no rollout found") {
		t.Fatalf("resume of a missing thread: %+v", r.Error)
	}
	r := responseTo(t, msgs, "3")
	if r.Error != nil {
		t.Fatalf("resume: %+v", r.Error)
	}
	// The version source the catalog resolves at (parseHarnessVersion cannot
	// read `codex --version`'s "codex-cli 0.160.0"), and the effort a
	// turn/start override left on the thread.
	if v := jsonAt(r.Result, "thread", "cliVersion"); v != "0.160.0" {
		t.Fatalf("thread.cliVersion = %v", v)
	}
	if e := jsonAt(r.Result, "thread", "reasoningEffort"); e != "low" {
		t.Fatalf("resumed reasoningEffort = %v, want the earlier turn/start override", e)
	}
	last := msgs[0]
	for _, m := range msgs {
		if m.in && m.Method == "thread/start" {
			last = m
		}
	}
	bad := responseTo(t, msgs, string(last.ID))
	if bad.Error == nil || !strings.Contains(bad.Error.Message, "required MCP servers failed to initialize") || !strings.Contains(bad.Error.Message, "HTTP 401") {
		t.Fatalf("required HTTP MCP server refusing the token: %+v", bad.Error)
	}
	if jsonAt(last.Params, "config", "mcp_servers", "wsprobe", "required") != true || jsonAt(last.Params, "config", "mcp_servers", "wsprobe", "bearer_token_env_var") == nil {
		t.Fatalf("admission probe config: %s", last.Params)
	}
}

// subagentItems counts the native subagent items a fixture's stream started.
func subagentItems(msgs []codexMsg) int {
	n := 0
	for _, m := range msgs {
		if m.Method == "item/started" {
			switch m.str("item", "type") {
			case "subAgentActivity", "collabAgentToolCall":
				n++
			}
		}
	}
	return n
}

// firstIn returns the first client line calling method.
func firstIn(t *testing.T, msgs []codexMsg, method string) codexMsg {
	t.Helper()
	for _, m := range msgs {
		if m.in && m.Method == method {
			return m
		}
	}
	t.Fatalf("no %s request", method)
	return codexMsg{}
}

// finalText is the agent text of the last completed turn.
func finalText(msgs []codexMsg) string {
	text := ""
	for _, m := range msgs {
		if m.Method != "turn/completed" {
			continue
		}
		items, _ := jsonAt(m.Params, "turn", "items").([]any)
		for _, it := range items {
			if obj, _ := it.(map[string]any); obj["type"] == "agentMessage" {
				text, _ = obj["text"].(string)
			}
		}
	}
	return text
}

// agents.enabled=false removes the native subagent tool; the same prompt
// spawns one on the baseline. features.multi_agent=false does not remove it:
// on a resumed thread a subagent still ran.
func TestCodexSubagentRemovalFixture(t *testing.T) {
	base := readCodexFixture(t, "codex-0.160.0-subagent-baseline.jsonl")
	removed := readCodexFixture(t, "codex-0.160.0-subagent-removed.jsonl")
	if subagentItems(base) == 0 {
		t.Fatal("baseline spawned no subagent")
	}
	if n := subagentItems(removed); n != 0 {
		t.Fatalf("agents.enabled=false still ran %d subagent items", n)
	}
	if v := jsonAt(firstIn(t, removed, "thread/start").Params, "config", "agents", "enabled"); v != false {
		t.Fatalf("removed fixture config agents.enabled = %v", v)
	}
	if v := jsonAt(firstIn(t, base, "thread/start").Params, "config", "agents"); v != nil {
		t.Fatalf("baseline config sets agents: %v", v)
	}
	prompt := func(msgs []codexMsg) any { return jsonAt(firstIn(t, msgs, "turn/start").Params, "input") }
	if p1, p2 := prompt(base), prompt(removed); !jsonEqual(p1, p2) {
		t.Fatalf("baseline and removed prompts differ: %v vs %v", p1, p2)
	}
	if !strings.Contains(finalText(removed), "NO-SUBAGENT-TOOL") {
		t.Fatalf("removed reply: %q", finalText(removed))
	}

	resume := readCodexFixture(t, "codex-0.160.0-resume.jsonl")
	// The first resume is the missing-thread probe; the last is the real one.
	var resumed codexMsg
	for _, m := range resume {
		if m.in && m.Method == "thread/resume" {
			resumed = m
		}
	}
	if v := jsonAt(resumed.Params, "config", "features", "multi_agent"); v != false {
		t.Fatalf("resume config features.multi_agent = %v", v)
	}
	if subagentItems(resume) == 0 {
		t.Fatal("features.multi_agent=false on resume: no subagent ran, so the fixture no longer shows it is not a removal")
	}
	off := readCodexFixture(t, "codex-0.160.0-multi-agent-feature-off.jsonl")
	if v := jsonAt(firstIn(t, off, "thread/start").Params, "config", "features", "shell_tool"); v != false {
		t.Fatalf("feature-off config shell_tool = %v", v)
	}
	for _, m := range off {
		if m.Method == "item/started" && m.str("item", "type") == "commandExecution" {
			t.Fatal("features.shell_tool=false still ran a command")
		}
	}

	// A resumed lead's stream also carries the spawned subagent's own thread
	// notifications (another threadId), interleaved inside the lead's turn: a
	// mapper scopes every notification by threadId.
	leadThread := jsonAt(resumed.Params, "threadId")
	foreign := 0
	for _, m := range resume {
		if !m.in && (m.Method == "turn/started" || m.Method == "turn/completed" || m.Method == "thread/status/changed") {
			if id := jsonAt(m.Params, "threadId"); id != nil && id != leadThread {
				foreign++
			}
		}
	}
	if foreign == 0 {
		t.Fatal("no subagent-thread notification on the lead's stream")
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// Stream facts the catalog grades and the mapper rely on.
func TestCodexStreamFactsFixture(t *testing.T) {
	lead := readCodexFixture(t, "codex-0.160.0-lead-session.jsonl")
	usage, waiting := false, false
	for _, m := range lead {
		if m.Method == "thread/tokenUsage/updated" && jsonAt(m.Params, "tokenUsage", "modelContextWindow") != nil && jsonAt(m.Params, "tokenUsage", "total", "totalTokens") != nil {
			usage = true
		}
		if m.Method == "thread/status/changed" && strings.Contains(string(m.Params), `"waitingOnApproval"`) {
			waiting = true
		}
	}
	if !usage || !waiting {
		t.Fatalf("pushed token usage %v, waitingOnApproval status %v", usage, waiting)
	}

	block := readCodexFixture(t, "codex-0.160.0-plugin-mcp-block.jsonl")
	refused := false
	for _, m := range block {
		if !m.in && m.Error != nil && strings.Contains(m.Error.Message, "invalid transport") {
			refused = true
		}
	}
	if !refused {
		t.Fatal("bare mcp_servers.ws.enabled=false was not refused")
	}

	shadow := readCodexFixture(t, "codex-0.160.0-ws-mcp-shadow.jsonl")
	if v := jsonAt(firstIn(t, shadow, "thread/start").Params, "config", "mcp_servers", "ws", "url"); v == nil {
		t.Fatal("shadow fixture injects no ws url")
	}
	starting := 0
	for _, m := range shadow {
		if m.Method == "mcpServer/startupStatus/updated" && m.str("name") == "ws" && m.str("status") == "starting" {
			starting++
		}
	}
	if starting != 1 {
		t.Fatalf("ws server starts %d times on the shadowed thread, want 1", starting)
	}
}
