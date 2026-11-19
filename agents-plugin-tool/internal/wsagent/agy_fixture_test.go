package wsagent

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// The agy-<version>-*.jsonl fixtures are recordings of `agy acp` (Agent
// Client Protocol v1: stdio JSON-RPC 2.0, the "jsonrpc" member present)
// driven by a probe client with an isolated --gemini-dir, cut the way the
// codex fixtures are: a line the client wrote is kept behind a ">> " mark,
// every other line is agent output. Paths, the account's plan
// (_meta.antigravity.tier) and bearer tokens are scrubbed; message order
// and every field a mapper reads are as recorded. Client and agent request
// ids are separate spaces (both start at 1), so a line is a request when it
// has a method and a response when it has none.
// 261117-feat-ws-hosted-antigravity-adapter Phase 1 records which
// capability each fixture evidences; these tests pin the recorded protocol
// facts the Antigravity adapter is built against, so a re-recording on a new
// agy version that changes one of them fails here first. The codex fixture
// helpers (readCodexFixture, responseTo, jsonAt, firstIn) read any
// JSON-RPC line, so they serve here too.

// acpUpdate is the sessionUpdate kind of a session/update line, else "".
func acpUpdate(m codexMsg) string {
	if m.Method != "session/update" {
		return ""
	}
	return m.str("update", "sessionUpdate")
}

// acpText joins the agent message text in msgs[from:to].
func acpText(msgs []codexMsg, from, to int) string {
	var b strings.Builder
	for _, m := range msgs[from:to] {
		if acpUpdate(m) == "agent_message_chunk" {
			b.WriteString(m.str("update", "content", "text"))
		}
	}
	return b.String()
}

// acpPrompts lists the indexes of the client's session/prompt lines.
func acpPrompts(msgs []codexMsg) []int {
	var out []int
	for i, m := range msgs {
		if m.in && m.Method == "session/prompt" {
			out = append(out, i)
		}
	}
	return out
}

// acpResponseAt is the index of the agent's response to the client request
// at msgs[req].
func acpResponseAt(t *testing.T, msgs []codexMsg, req int) int {
	t.Helper()
	for i := req + 1; i < len(msgs); i++ {
		if !msgs[i].in && msgs[i].Method == "" && string(msgs[i].ID) == string(msgs[req].ID) {
			return i
		}
	}
	t.Fatalf("no response to request %s", msgs[req].ID)
	return -1
}

func acpStopReason(m codexMsg) any { return jsonAt(m.Result, "stopReason") }

func TestAgyFixturesScrubbed(t *testing.T) {
	names, err := filepath.Glob("testdata/agy-*.jsonl")
	if err != nil || len(names) == 0 {
		t.Fatalf("no agy fixtures: %v", err)
	}
	for _, path := range names {
		for _, line := range readLeadFixture(t, filepath.Base(path)) {
			for _, leak := range []string{"/Users/", "/private/", "/var/folders/", "wsprobe-token-", "free-tier"} {
				if strings.Contains(string(line.raw), leak) {
					t.Errorf("%s carries unscrubbed %q", path, leak)
				}
			}
			if m := codexFixtureLeak.Find(line.raw); m != nil {
				t.Errorf("%s carries unscrubbed %q", path, m)
			}
		}
	}
}

// No initialize gate; initialize reports the version the catalog resolves
// at, the session capabilities and the system-prompt append extension, and
// no mcpCapabilities although HTTP MCP servers are accepted (http-mcp).
func TestAgyHandshakeFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-handshake.jsonl")
	if r := responseTo(t, msgs, "1"); r.Error != nil || jsonAt(r.Result, "sessionId") == nil {
		t.Fatalf("session/new before initialize: %+v %s", r.Error, r.Result)
	}
	init := responseTo(t, msgs, "2")
	// A bare dotted version, unlike codex's "codex-cli 0.160.0": the catalog's
	// version parser reads it as is.
	if v := jsonAt(init.Result, "agentInfo", "version"); v != "1.5.9" {
		t.Fatalf("agentInfo.version = %v", v)
	}
	caps := jsonAt(init.Result, "agentCapabilities")
	for _, c := range []string{"resume", "load", "close", "list"} {
		if jsonAt(init.Result, "agentCapabilities", "sessionCapabilities", c) == nil {
			t.Errorf("sessionCapabilities.%s missing: %v", c, caps)
		}
	}
	if jsonAt(init.Result, "agentCapabilities", "_meta", "antigravity", "systemPrompt", "append") != true {
		t.Errorf("no systemPrompt.append extension: %v", caps)
	}
	if jsonAt(init.Result, "agentCapabilities", "mcpCapabilities") != nil {
		t.Errorf("mcpCapabilities is now advertised: %v", caps)
	}
	if r := responseTo(t, msgs, "3"); r.Error != nil {
		t.Fatalf("second initialize: %+v", r.Error)
	}
	if r := responseTo(t, msgs, "4"); r.Error == nil || r.Error.Code != -32601 {
		t.Fatalf("unknown method: %+v", r.Error)
	}
}

// An HTTP MCP server passed on session/new, with the bearer token in its
// headers, reaches the model as mcp_<server>_<tool>, asks for approval and
// runs; the system-prompt append reaches the model.
func TestAgyHTTPMCPFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-http-mcp.jsonl")
	newReq := firstIn(t, msgs, "session/new")
	if jsonAt(newReq.Params, "mcpServers") == nil {
		t.Fatal("no mcpServers on session/new")
	}
	var server map[string]any
	if s, _ := jsonAt(newReq.Params, "mcpServers").([]any); len(s) == 1 {
		server, _ = s[0].(map[string]any)
	}
	if server["type"] != "http" || server["name"] != "ws" {
		t.Fatalf("mcp server entry: %v", server)
	}
	if h, _ := server["headers"].([]any); len(h) != 1 || !jsonEqual(h[0], map[string]any{"name": "Authorization", "value": "Bearer <token>"}) {
		t.Fatalf("mcp server headers: %v", server["headers"])
	}
	if jsonAt(newReq.Params, "_meta", "antigravity", "systemPrompt", "append") == nil {
		t.Fatal("no system-prompt append on session/new")
	}
	completed := false
	for _, m := range msgs {
		if acpUpdate(m) == "tool_call_update" && m.str("update", "name") == "mcp_ws_probe_echo" && m.str("update", "status") == "completed" {
			completed = true
		}
	}
	text := acpText(msgs, 0, len(msgs))
	if !completed || !strings.Contains(text, "ZEBRA-7") || !strings.Contains(text, "ECHO:hi") {
		t.Fatalf("mcp call completed %v, reply %q", completed, text)
	}
}

// A permission request offers exactly allow (allow_once) and deny
// (reject_once); a denied call is reported failed and the turn ends
// normally.
func TestAgyApprovalFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	requests := 0
	for i, m := range msgs {
		if m.in || m.Method != "session/request_permission" {
			continue
		}
		requests++
		opts, _ := json.Marshal(jsonAt(m.Params, "options"))
		if string(opts) != `[{"kind":"allow_once","name":"Allow","optionId":"allow"},{"kind":"reject_once","name":"Deny","optionId":"deny"}]` {
			t.Fatalf("permission options: %s", opts)
		}
		call := m.str("toolCall", "toolCallId")
		// The tool call is announced (in_progress, then pending) before the
		// request, and the client answers before the call moves on.
		seen := ""
		for _, p := range msgs[:i] {
			if k := acpUpdate(p); (k == "tool_call" || k == "tool_call_update") && p.str("update", "toolCallId") == call {
				seen += p.str("update", "status") + ","
			}
		}
		if seen != "in_progress,pending," {
			t.Fatalf("call %s before its request: %s", call, seen)
		}
		if a := msgs[i+1]; !a.in || string(a.ID) != string(m.ID) || jsonAt(a.Result, "outcome", "outcome") != "selected" {
			t.Fatalf("answer to request %s: %s", m.ID, a.Result)
		}
		if requests == 1 {
			if msgs[i+1].Result == nil || jsonAt(msgs[i+1].Result, "outcome", "optionId") != "deny" {
				t.Fatalf("first request answer: %s", msgs[i+1].Result)
			}
			end := ""
			for _, p := range msgs[i+1:] {
				if acpUpdate(p) == "tool_call_update" && p.str("update", "toolCallId") == call {
					end = p.str("update", "status")
				}
				if !p.in && p.Method == "" && len(p.Result) > 0 {
					break
				}
			}
			if end != "failed" {
				t.Fatalf("denied call ends %q", end)
			}
		}
	}
	if requests != 4 {
		t.Fatalf("permission requests: %d", requests)
	}
}

// session/cancel closes the open tool call (failed) before the prompt
// answers with stopReason cancelled: no item outlives the turn.
func TestAgyCancelFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	cancel := -1
	for i, m := range msgs {
		if m.in && m.Method == "session/cancel" {
			cancel = i
		}
	}
	if cancel < 0 {
		t.Fatal("no session/cancel")
	}
	prompts := acpPrompts(msgs)
	var req int
	for _, p := range prompts {
		if p < cancel {
			req = p
		}
	}
	end := acpResponseAt(t, msgs, req)
	if acpStopReason(msgs[end]) != "cancelled" {
		t.Fatalf("cancelled prompt: %s", msgs[end].Result)
	}
	failed := false
	for _, m := range msgs[cancel:end] {
		if acpUpdate(m) == "tool_call_update" && m.str("update", "status") == "failed" {
			failed = true
		}
	}
	if !failed {
		t.Fatal("the open tool call was not closed before the cancelled response")
	}
}

// A session/prompt sent while a turn runs steers it: the earlier prompt
// answers end_turn at once, and the rest of the run (the tool's completion,
// the reply that uses the steer) belongs to the later prompt. A prompt
// response is the turn end only when no later prompt is outstanding.
func TestAgySteerFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	var first, second = -1, -1
	prompts := acpPrompts(msgs)
	for k := 0; k+1 < len(prompts); k++ {
		if acpResponseAt(t, msgs, prompts[k]) > prompts[k+1] {
			first, second = prompts[k], prompts[k+1]
		}
	}
	if first < 0 {
		t.Fatal("no overlapping prompts")
	}
	firstEnd, secondEnd := acpResponseAt(t, msgs, first), acpResponseAt(t, msgs, second)
	if acpStopReason(msgs[firstEnd]) != "end_turn" || firstEnd != second+1 {
		t.Fatalf("steered prompt answered at %d (%s), steer at %d", firstEnd, msgs[firstEnd].Result, second)
	}
	toolDone := false
	for _, m := range msgs[firstEnd:secondEnd] {
		if acpUpdate(m) == "tool_call_update" && m.str("update", "status") == "completed" {
			toolDone = true
		}
	}
	if !toolDone || acpText(msgs, firstEnd, secondEnd) != "QUOKKA" {
		t.Fatalf("after the steer: tool completed %v, reply %q", toolDone, acpText(msgs, firstEnd, secondEnd))
	}
}

// Thinking level and mode change in-session, with no restart. The model
// half is not evidenced: the recorded model calls re-set the current
// model, and session/set_model answered {} with configOptions unchanged.
func TestAgyConfigFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	set := firstIn(t, msgs, "session/set_config_option")
	if jsonAt(set.Params, "configId") != "thought_level" {
		t.Fatalf("first config change: %s", set.Params)
	}
	r := responseTo(t, msgs, string(set.ID))
	opts, _ := jsonAt(r.Result, "configOptions").([]any)
	level := any(nil)
	for _, o := range opts {
		if obj, _ := o.(map[string]any); obj["id"] == "thought_level" {
			level = obj["currentValue"]
		}
	}
	if level != "low" {
		t.Fatalf("thought_level after the change: %v", level)
	}
	modes := []string{}
	for _, m := range msgs {
		if acpUpdate(m) == "current_mode_update" {
			modes = append(modes, m.str("update", "currentModeId"))
		}
	}
	if strings.Join(modes, ",") != "accept-edits,default" {
		t.Fatalf("mode updates: %v", modes)
	}
}

// resume of a missing id fails; resume of a real one (MCP servers re-passed)
// continues the conversation without replay; /compact is a plain prompt
// whose turn carries no compaction event; an HTTP MCP server given a wrong
// bearer does not fail session/new: its tools are just absent.
func TestAgyResumeFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-resume.jsonl")
	if r := responseTo(t, msgs, "2"); r.Error == nil || !strings.Contains(r.Error.Message, "Failed to read session") {
		t.Fatalf("resume of a missing session: %+v", r.Error)
	}
	if r := responseTo(t, msgs, "4"); r.Error != nil || jsonAt(r.Result, "sessionId") == nil {
		t.Fatalf("resume: %+v", r.Error)
	}
	prompts := acpPrompts(msgs)
	for _, m := range msgs[:prompts[0]] {
		if k := acpUpdate(m); k == "user_message_chunk" || k == "agent_message_chunk" || k == "tool_call" {
			t.Fatal("resume replayed history")
		}
	}
	if len(prompts) != 4 {
		t.Fatalf("prompts: %d", len(prompts))
	}
	r1 := acpText(msgs, prompts[0], acpResponseAt(t, msgs, prompts[0]))
	if !strings.Contains(r1, "ECHO:resumed") || !strings.Contains(r1, "QUOKKA") {
		t.Fatalf("resumed reply: %q", r1)
	}
	if jsonAt(msgs[prompts[1]].Params, "prompt") == nil || !strings.Contains(string(msgs[prompts[1]].Params), `"/compact"`) {
		t.Fatalf("compact prompt: %s", msgs[prompts[1]].Params)
	}
	compactEnd := acpResponseAt(t, msgs, prompts[1])
	for _, m := range msgs[prompts[1]:compactEnd] {
		if k := acpUpdate(m); k != "" && k != "usage_update" {
			t.Fatalf("compact turn carries %s", k)
		}
	}
	if acpStopReason(msgs[compactEnd]) != "end_turn" {
		t.Fatalf("compact turn: %s", msgs[compactEnd].Result)
	}
	var bad codexMsg
	for _, m := range msgs {
		if m.in && m.Method == "session/new" {
			bad = m
		}
	}
	if !strings.Contains(string(bad.Params), "Bearer wrong-token") {
		t.Fatalf("bad-token session/new: %s", bad.Params)
	}
	if r := responseTo(t, msgs, string(bad.ID)); r.Error != nil {
		t.Fatalf("bad-token session/new failed loudly: %+v", r.Error)
	}
	if got := acpText(msgs, prompts[3], acpResponseAt(t, msgs, prompts[3])); got != "NO-WS-TOOL" {
		t.Fatalf("reply with the refused server: %q", got)
	}
}

// session/load replays the history as session/update lines before its
// response; tool calls replay with their final status only.
func TestAgyLoadReplayFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-load-replay.jsonl")
	load := firstIn(t, msgs, "session/load")
	var at int
	for i, m := range msgs {
		if m.in && m.Method == "session/load" {
			at = i
		}
	}
	end := acpResponseAt(t, msgs, at)
	users, tools := 0, 0
	for _, m := range msgs[at:end] {
		switch acpUpdate(m) {
		case "user_message_chunk":
			users++
		case "tool_call":
			tools++
			if s := m.str("update", "status"); s != "completed" && s != "failed" {
				t.Fatalf("replayed tool call status %q", s)
			}
		case "tool_call_update":
			t.Fatal("replay carries a tool_call_update")
		}
	}
	if users == 0 || tools == 0 || jsonAt(load.Params, "sessionId") == nil {
		t.Fatalf("replay: %d user messages, %d tool calls", users, tools)
	}
}

// A cancel while a permission request waits fails the call and answers the
// prompt cancelled, with no message withdrawing the request; the client's
// late answer gets no reply and the next prompt runs normally.
func TestAgyStaleApprovalFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-stale-approval.jsonl")
	req, cancel, end, late := -1, -1, -1, -1
	for i, m := range msgs {
		switch {
		case !m.in && m.Method == "session/request_permission":
			req = i
		case m.in && m.Method == "session/cancel":
			cancel = i
		case req >= 0 && m.in && m.Method == "" && string(m.ID) == string(msgs[req].ID):
			late = i
		}
	}
	if req < 0 || cancel < req || late < 0 {
		t.Fatalf("request %d, cancel %d, late answer %d", req, cancel, late)
	}
	for i := cancel; i < late; i++ {
		if !msgs[i].in && msgs[i].Method == "" && acpStopReason(msgs[i]) == "cancelled" {
			end = i
		}
		if msgs[i].Method != "" && msgs[i].Method != "session/update" && !msgs[i].in {
			t.Fatalf("agent sent %s after the cancel", msgs[i].Method)
		}
	}
	if end < 0 {
		t.Fatal("no cancelled response before the late answer")
	}
	prompts := acpPrompts(msgs)
	next := prompts[len(prompts)-1]
	if next < late || acpStopReason(msgs[acpResponseAt(t, msgs, next)]) != "end_turn" {
		t.Fatal("the prompt after the late answer did not run")
	}
	// The late answer draws nothing: no agent line names its id, and no
	// error arrives, through the end of the recording.
	for _, m := range msgs[late+1:] {
		if !m.in && (m.Error != nil || (m.Method == "" && string(m.ID) == string(msgs[late].ID))) {
			t.Fatalf("the late answer drew %s %+v", m.Result, m.Error)
		}
	}
	// Agent request ids are JSON strings, client ids numbers (or strings:
	// the probe's initialize used "attach1-1" and was answered under it).
	if msgs[req].ID[0] != '"' {
		t.Fatalf("agent request id %s is not a string", msgs[req].ID)
	}
	if r := responseTo(t, msgs, `"attach1-1"`); r.Error != nil || jsonAt(r.Result, "protocolVersion") == nil {
		t.Fatalf("string-id initialize: %+v", r.Error)
	}
}

// The session/new system-prompt append persists: a resumed session in a new
// process, with no append re-sent, still knows the marker that never
// appeared in the conversation.
func TestAgyResumeMarkerFixture(t *testing.T) {
	stale := readCodexFixture(t, "agy-1.5.9-stale-approval.jsonl")
	if v, _ := jsonAt(firstIn(t, stale, "session/new").Params, "_meta", "antigravity", "systemPrompt", "append").(string); !strings.Contains(v, "PAPAYA-9") {
		t.Fatalf("append: %q", v)
	}
	for _, m := range stale {
		if m.in && strings.Contains(string(m.Params), "PAPAYA") && m.Method != "session/new" {
			t.Fatal("the marker appeared in the conversation")
		}
	}
	msgs := readCodexFixture(t, "agy-1.5.9-resume-marker.jsonl")
	if jsonAt(firstIn(t, msgs, "session/resume").Params, "_meta") != nil {
		t.Fatal("resume re-sent _meta")
	}
	if got := acpText(msgs, 0, len(msgs)); got != "PAPAYA-9" {
		t.Fatalf("marker after the restart: %q", got)
	}
}

// The default agent spawns a subagent through invoke_subagent, visible only
// as the parent's tool calls (no update names another session); a custom
// main agent whose tools list leaves the subagent tools out removes them.
// ask_question with no client elicitation becomes agent text: the call stays
// pending and the turn ends, so the next prompt is the answer.
func TestAgySubagentFixture(t *testing.T) {
	base := readCodexFixture(t, "agy-1.5.9-subagent-baseline.jsonl")
	removed := readCodexFixture(t, "agy-1.5.9-subagent-removed.jsonl")
	calls := func(msgs []codexMsg) map[string]bool {
		out := map[string]bool{}
		for _, m := range msgs {
			if acpUpdate(m) == "tool_call" {
				out[m.str("update", "name")] = true
			}
		}
		return out
	}
	if !calls(base)["invoke_subagent"] {
		t.Fatal("baseline spawned no subagent")
	}
	if calls(removed)["invoke_subagent"] {
		t.Fatal("the custom agent still spawned a subagent")
	}
	sid := jsonAt(responseTo(t, base, "2").Result, "sessionId")
	for _, m := range base {
		if m.Method == "session/update" && jsonAt(m.Params, "sessionId") != sid {
			t.Fatalf("an update names another session: %s", m.Params)
		}
	}
	bp, rp := acpPrompts(base), acpPrompts(removed)
	if !jsonEqual(jsonAt(base[bp[1]].Params, "prompt"), jsonAt(removed[rp[1]].Params, "prompt")) {
		t.Fatal("baseline and removed spawn prompts differ")
	}
	// The subagent's result reaches the parent inside the same prompt.
	if got := acpText(base, bp[1], acpResponseAt(t, base, bp[1])); !strings.HasSuffix(got, "KIWI") {
		t.Fatalf("baseline spawn reply: %q", got)
	}
	if got := acpText(removed, rp[1], acpResponseAt(t, removed, rp[1])); got != "NO-SUBAGENT-TOOL" {
		t.Fatalf("removed reply: %q", got)
	}
	if list := acpText(removed, rp[0], acpResponseAt(t, removed, rp[0])); strings.Contains(list, "subagent") || strings.Contains(list, "mcp_ws_") {
		t.Fatalf("custom agent tool list: %q", list)
	}

	ask, askEnd := bp[2], acpResponseAt(t, base, bp[2])
	status := ""
	for _, m := range base[ask:askEnd] {
		if k := acpUpdate(m); (k == "tool_call" || k == "tool_call_update") && m.str("update", "name") == "ask_question" {
			status = m.str("update", "status")
		}
		if !m.in && m.Method != "" && m.Method != "session/update" {
			t.Fatalf("ask_question sent %s", m.Method)
		}
	}
	if status != "pending" || !strings.Contains(acpText(base, ask, askEnd), "A and B") || acpStopReason(base[askEnd]) != "end_turn" {
		t.Fatalf("ask_question: status %q, text %q", status, acpText(base, ask, askEnd))
	}
}

// A custom agent's tools list naming an unknown tool fails every turn as
// agent text, not as an error; naming the session's MCP server in its
// mcpServers reaches it only through the lazy call_mcp_tool gateway.
func TestAgyCustomAgentFixtures(t *testing.T) {
	bad := readCodexFixture(t, "agy-1.5.9-custom-agent-bad-tool.jsonl")
	for _, p := range acpPrompts(bad) {
		end := acpResponseAt(t, bad, p)
		if bad[end].Error != nil || acpStopReason(bad[end]) != "end_turn" || !strings.Contains(acpText(bad, p, end), "failed to construct executor") {
			t.Fatalf("bad tool turn: %s %q", bad[end].Result, acpText(bad, p, end))
		}
	}
	msgs := readCodexFixture(t, "agy-1.5.9-custom-agent-mcp.jsonl")
	names := []string{}
	for _, m := range msgs {
		if acpUpdate(m) == "tool_call" {
			names = append(names, m.str("update", "name"))
		}
	}
	if strings.Join(names, ",") != "read_file,call_mcp_tool" || !strings.Contains(acpText(msgs, 0, len(msgs)), "ECHO:v") {
		t.Fatalf("custom agent MCP calls: %v", names)
	}
}

// Usage is pushed: usage_update {used, size} after model calls, and each
// prompt response carries its token usage.
func TestAgyUsageFixture(t *testing.T) {
	msgs := readCodexFixture(t, "agy-1.5.9-lead-session.jsonl")
	updates := 0
	for _, m := range msgs {
		if acpUpdate(m) == "usage_update" {
			updates++
			if jsonAt(m.Params, "update", "used") == nil || jsonAt(m.Params, "update", "size") == nil {
				t.Fatalf("usage_update: %s", m.Params)
			}
		}
	}
	for _, p := range acpPrompts(msgs) {
		if jsonAt(msgs[acpResponseAt(t, msgs, p)].Result, "usage", "totalTokens") == nil {
			t.Fatalf("prompt %s response has no usage", msgs[p].ID)
		}
	}
	if updates == 0 {
		t.Fatal("no usage_update")
	}
}
