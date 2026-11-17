package wsagent

import (
	"encoding/json"
	"path/filepath"
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
	var v any
	_ = json.Unmarshal(m.Params, &v)
	for _, k := range path {
		obj, _ := v.(map[string]any)
		v = obj[k]
	}
	s, _ := v.(string)
	return s
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

func TestCodexFixturesScrubbed(t *testing.T) {
	names, err := filepath.Glob("testdata/codex-*.jsonl")
	if err != nil || len(names) == 0 {
		t.Fatalf("no codex fixtures: %v", err)
	}
	for _, path := range names {
		for _, line := range readLeadFixture(t, filepath.Base(path)) {
			for _, leak := range []string{"/Users/", "@gmail", "wsprobe-token-", "orca-agent-hook"} {
				if strings.Contains(string(line.raw), leak) {
					t.Errorf("%s carries unscrubbed %q", path, leak)
				}
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
// request; answers sent after that (twice) get no reply and change nothing.
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
		case !m.in && answers > 0:
			t.Fatalf("server answered a stale approval answer: %s %s", m.Method, m.Result)
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
	if r := responseTo(t, msgs, "3"); r.Error != nil {
		t.Fatalf("resume: %+v", r.Error)
	}
	last := msgs[0]
	for _, m := range msgs {
		if m.in && m.Method == "thread/start" {
			last = m
		}
	}
	if r := responseTo(t, msgs, string(last.ID)); r.Error == nil || !strings.Contains(r.Error.Message, "required MCP servers failed to initialize") {
		t.Fatalf("required HTTP MCP server refusing the token: %+v", r.Error)
	}
}

// agents.enabled=false removes the native subagent tool; features.multi_agent
// does not (the baseline spawns one in the same prompt).
func TestCodexSubagentRemovalFixture(t *testing.T) {
	subagentItems := func(name string) int {
		n := 0
		for _, m := range readCodexFixture(t, name) {
			if m.Method == "item/started" {
				switch m.str("item", "type") {
				case "subAgentActivity", "collabAgentToolCall":
					n++
				}
			}
		}
		return n
	}
	if subagentItems("codex-0.160.0-subagent-baseline.jsonl") == 0 {
		t.Fatal("baseline spawned no subagent")
	}
	if n := subagentItems("codex-0.160.0-subagent-removed.jsonl"); n != 0 {
		t.Fatalf("agents.enabled=false still ran %d subagent items", n)
	}
}
