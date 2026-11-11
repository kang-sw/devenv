package wsagent

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// The typed failure lines below take the shapes Claude Code's stream-json
// output gives them (as read by Orca's Claude adapter, and the SDK's
// SDKAssistantMessage error field): the assistant line of a failed request
// carries error (authentication_failed, model_not_found, rate_limit) and,
// for a usage limit, api_error usage_limit_reached with the limit's
// rate_limit_info; the message's text is the harness's own words.

func failureLine(fields map[string]any, text string) string {
	line := map[string]any{"type": "assistant", "parent_tool_use_id": nil, "message": map[string]any{"id": "msg-err", "model": "<synthetic>", "role": "assistant",
		"content": []map[string]any{{"type": "text", "text": text}}, "usage": map[string]any{"input_tokens": 0, "output_tokens": 0}}}
	for k, v := range fields {
		line[k] = v
	}
	return jsonLine(line)
}

func rateLimitLine(status, kind string, resetsAt int64) string {
	return jsonLine(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": status, "rateLimitType": kind, "resetsAt": resetsAt}})
}

// alerts maps lines through a lead's mapper and returns the alerts.
func alerts(t *testing.T, m *claudeMapper, lines ...string) (out []wslive.Notice, evs []Event) {
	t.Helper()
	for _, l := range lines {
		for _, ev := range m.mapLine([]byte(l), nopReplies{}) {
			evs = append(evs, ev)
			if ev.Alert != nil {
				out = append(out, *ev.Alert)
			}
		}
	}
	return out, evs
}

// TestClaudeLeadTypedFailures: each typed failure becomes one alert of its
// kind carrying the harness's text, and not a reply as well; a usage limit
// resets when every refusing limit does, as the refusal's own report or
// the rate-limit lines before it tell, and not when one's time is unknown.
// A profile conversation reads none of them.
func TestClaudeLeadTypedFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lines  []string
		want   wslive.Notice
		reply0 bool
	}{
		{name: "login", lines: []string{failureLine(map[string]any{"error": "authentication_failed"}, "Not logged in · Please run /login")},
			want: wslive.Notice{Kind: wslive.NoticeLogin, Text: "Not logged in · Please run /login"}},
		{name: "model", lines: []string{failureLine(map[string]any{"error": "model_not_found"}, "There's an issue with the selected model (claude-nope).")},
			want: wslive.Notice{Kind: wslive.NoticeModel, Text: "There's an issue with the selected model (claude-nope)."}},
		{name: "usage limit from its own report", lines: []string{failureLine(map[string]any{"error": "rate_limit", "api_error": "usage_limit_reached",
			"api_error_params": map[string]any{"rate_limit_info": map[string]any{"status": "rejected", "rateLimitType": "five_hour", "resetsAt": 1_800_000_000}}}, "You've hit your limit · resets 3pm")},
			want: wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "You've hit your limit · resets 3pm", ResetsAt: 1_800_000_000_000}},
		{name: "usage limit from rate-limit lines", lines: []string{
			rateLimitLine("rejected", "five_hour", 1_800_000_000), rateLimitLine("rejected", "seven_day", 1_800_100_000),
			failureLine(map[string]any{"error": "rate_limit"}, "You've hit your limit")},
			want: wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "You've hit your limit", ResetsAt: 1_800_100_000_000}},
		{name: "usage limit allowed again", lines: []string{
			rateLimitLine("rejected", "five_hour", 1_800_000_000), rateLimitLine("allowed", "five_hour", 1_800_000_000), rateLimitLine("rejected", "seven_day", 0),
			failureLine(map[string]any{"error": "rate_limit"}, "You've hit your limit")},
			want: wslive.Notice{Kind: wslive.NoticeUsageLimit, Text: "You've hit your limit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, evs := alerts(t, newClaudeMapper(leadSpec()), tc.lines...)
			if len(got) != 1 || got[0] != tc.want {
				t.Fatalf("alerts = %+v, want %+v", got, tc.want)
			}
			if n := countEvents(evs, func(ev Event) bool { return ev.Live != nil && ev.Live.Kind == wslive.KindReply }); n != 0 {
				t.Fatalf("an alert's text is no reply: %+v", evs)
			}
			if n := countEvents(evs, func(ev Event) bool { return ev.TurnStart }); n != 1 {
				t.Fatalf("an alert's line is turn output: %+v", evs)
			}
			profile := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
			if got, _ := alerts(t, profile, tc.lines...); len(got) != 0 {
				t.Fatalf("a profile conversation reads no alert: %+v", got)
			}
		})
	}
	// A rate_limit refusal with another typed cause is no usage limit, and
	// an ordinary reply alerts nothing.
	got, _ := alerts(t, newClaudeMapper(leadSpec()),
		failureLine(map[string]any{"error": "rate_limit", "api_error": "overloaded_error"}, "Overloaded"),
		replyLine("u1", "fine"))
	if len(got) != 0 {
		t.Fatalf("no alert expected, got %+v", got)
	}
}

// TestClaudeLeadRetryAndFailedCompaction: an api_retry line is a retry
// with its attempt, bound, error, and status, settled by the next
// assistant output or the turn's result; a compaction that failed is a
// failed compaction line with the harness's error text.
func TestClaudeLeadRetryAndFailedCompaction(t *testing.T) {
	m := newClaudeMapper(leadSpec())
	retry := jsonLine(map[string]any{"type": "system", "subtype": "api_retry", "attempt": 2, "max_retries": 10, "retry_delay_ms": 1200, "error_status": 529, "error": "overloaded"})
	evs := m.mapLine([]byte(retry), nopReplies{})
	if len(evs) != 1 || evs[0].Retry == nil || *evs[0].Retry != (wslive.Retry{Attempt: 2, MaxRetries: 10, Error: "overloaded", Status: 529}) {
		t.Fatalf("retry events = %+v", evs)
	}
	evs = m.mapLine([]byte(replyLine("u1", "back")), nopReplies{})
	if countEvents(evs, func(ev Event) bool { return ev.RetryEnd }) != 1 {
		t.Fatalf("a reply settles the retry: %+v", evs)
	}
	if evs = m.mapLine([]byte(replyLine("u2", "again")), nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.RetryEnd }) != 0 {
		t.Fatalf("a settled retry ends once: %+v", evs)
	}
	m.mapLine([]byte(retry), nopReplies{})
	if evs = m.mapLine([]byte(resultLine()), nopReplies{}); countEvents(evs, func(ev Event) bool { return ev.RetryEnd }) != 1 {
		t.Fatalf("the result settles the retry: %+v", evs)
	}
	if evs := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`)).mapLine([]byte(retry), nopReplies{}); len(evs) != 0 {
		t.Fatalf("a profile conversation reads no retry: %+v", evs)
	}

	failed := jsonLine(map[string]any{"type": "system", "subtype": "status", "status": nil, "compact_result": "failed", "compact_error": "Conversation too long to compact"})
	evs = m.mapLine([]byte(failed), nopReplies{})
	if len(evs) != 1 || evs[0].Live == nil || evs[0].Live.Kind != wslive.KindCompact || evs[0].Live.Phase != wslive.CompactFailed || evs[0].Live.Text != "Conversation too long to compact" {
		t.Fatalf("failed compaction events = %+v", evs)
	}
}

// TestClaudeLeadMCPSignals: init reports the MCP servers' status; a call of
// the reconnect lever, or a failed MCP tool call, asks for a check; a
// failed call of any other tool does not.
func TestClaudeLeadMCPSignals(t *testing.T) {
	m := newClaudeMapper(leadSpec())
	init := jsonLine(map[string]any{"type": "system", "subtype": "init", "session_id": "s", "permissionMode": "auto",
		"mcp_servers": []map[string]any{{"name": "plugin:ws:ws", "status": "connected", "source": "plugin"}, {"name": "github", "status": "failed"}}})
	var servers []MCPServer
	for _, ev := range m.mapLine([]byte(init), nopReplies{}) {
		if ev.MCPServers != nil {
			servers = ev.MCPServers
		}
	}
	if !reflect.DeepEqual(servers, []MCPServer{{Name: "plugin:ws:ws", Status: MCPConnected}, {Name: "github", Status: MCPFailed}}) {
		t.Fatalf("servers = %+v", servers)
	}
	call := func(uuid, toolUse, name string) string {
		return jsonLine(map[string]any{"type": "assistant", "uuid": uuid, "message": map[string]any{"id": "msg-" + uuid, "content": []map[string]any{{"type": "tool_use", "id": toolUse, "name": name, "input": map[string]any{}}}}})
	}
	failedResult := func(toolUse string) string {
		return jsonLine(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": []map[string]any{{"type": "tool_result", "tool_use_id": toolUse, "is_error": true, "content": "MCP error -32000: Connection closed"}}}})
	}
	checks := func(lines ...string) int {
		n := 0
		for _, l := range lines {
			n += countEvents(m.mapLine([]byte(l), nopReplies{}), func(ev Event) bool { return ev.MCPCheck })
		}
		return n
	}
	if n := checks(call("u1", "t1", "mcp__ws__request_mcp_reconnect")); n != 1 {
		t.Fatalf("the lever's call asks for %d checks, want 1", n)
	}
	if n := checks(call("u2", "t2", "mcp__plugin_ws_ws__tickets_query"), failedResult("t2")); n != 1 {
		t.Fatalf("a failed MCP call asks for %d checks, want 1", n)
	}
	if n := checks(call("u3", "t3", "Bash"), failedResult("t3"), call("u4", "t4", "mcp__plugin_ws_ws__tickets_query"), toolResultLine("t4")); n != 0 {
		t.Fatalf("a failed Bash call or a good MCP call asks for %d checks, want 0", n)
	}
}

// TestClaudeLeadSubagentTelemetry: a subagent started by the lead's Agent
// call carries the effort its subagent type names and the model the call
// asked for; its replies give the answering model and the request's
// context size; its end stops them. Recorded with Claude 2.1.322, with the
// Agent call's subagent type and model set to an effort carrier.
func TestClaudeLeadSubagentTelemetry(t *testing.T) {
	lines := readLeadFixture(t, "claude-subagent-turn.jsonl")
	for i, l := range lines {
		if !l.in && l.field("type") == "assistant" && bytes.Contains(l.raw, []byte(`"name":"Agent"`)) {
			var line map[string]any
			_ = json.Unmarshal(l.raw, &line)
			msg := line["message"].(map[string]any)
			for _, b := range msg["content"].([]any) {
				if b := b.(map[string]any); b["name"] == "Agent" {
					input := b["input"].(map[string]any)
					input["subagent_type"], input["model"] = "ws:effort-high", "haiku"
				}
			}
			raw, _ := json.Marshal(line)
			lines[i] = leadLine{raw: raw}
		}
	}
	r := newLeadReplay(t)
	r.all(lines)
	agents := r.st.Snapshot(r.clock).Agents
	if len(agents) != 1 {
		t.Fatalf("agents = %+v", agents)
	}
	a := agents[0]
	if a.Effort != "high" || a.Model != "claude-haiku-5-5" || a.ContextTokens != 2+3944 || a.Status != wslive.StatusDone || a.OutputTps != 0 {
		t.Fatalf("subagent telemetry = %+v", a)
	}
	// The spawn's model stands until a reply names the answering one.
	m := newClaudeMapper(leadSpec())
	var started *wslive.Event
	for _, l := range lines {
		for _, ev := range m.mapLine(l.raw, nopReplies{}) {
			if ev.Live != nil && ev.Live.Kind == wslive.KindAgent && ev.Live.Status == wslive.StatusRunning {
				started = ev.Live
			}
		}
		if started != nil {
			break
		}
	}
	if started == nil || started.Model != "haiku" || started.Effort != "high" {
		t.Fatalf("the started subagent = %+v", started)
	}
	// A task the lead did not see the Agent call of takes the effort from
	// the task's own subagent type.
	m = newClaudeMapper(leadSpec())
	start := jsonLine(map[string]any{"type": "system", "subtype": "task_started", "task_id": "a1", "tool_use_id": "toolu_x", "description": "probe", "subagent_type": "ws:effort-low", "task_type": "local_agent"})
	evs := m.mapLine([]byte(start), nopReplies{})
	if len(evs) != 1 || evs[0].Live == nil || evs[0].Live.Effort != "low" || evs[0].Live.Model != "" {
		t.Fatalf("task start = %+v", evs)
	}
	if got := claudeSpawnOf(json.RawMessage(`{"subagent_type":"general-purpose"}`)); got != (claudeSpawn{}) {
		t.Fatalf("a type that is no effort carrier names no effort: %+v", got)
	}
	if !slices.Contains([]string{"low", "medium", "high", "xhigh", "max"}, claudeSpawnOf(json.RawMessage(`{"subagent_type":"effort-max"}`)).effort) {
		t.Fatal("an unprefixed effort carrier names its effort")
	}
}
