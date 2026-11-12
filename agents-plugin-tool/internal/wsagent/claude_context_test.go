package wsagent

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

// statuses are the status events among evs.
func statuses(evs []Event) []wslive.Event {
	var out []wslive.Event
	for _, ev := range evs {
		if ev.Live != nil && ev.Live.Kind == wslive.KindStatus {
			out = append(out, *ev.Live)
		}
	}
	return out
}

// usageLine is a lead assistant line whose usage counts tokens in all;
// parent, when set, makes it a subagent's.
func usageLine(tokens int64, parent string) []byte {
	line := map[string]any{"type": "assistant", "message": map[string]any{"id": "msg-1", "model": "claude-opus-5-5",
		"content": []map[string]any{{"type": "text", "text": "working"}}, "usage": map[string]any{"input_tokens": 2, "cache_read_input_tokens": tokens - 3, "output_tokens": 1}}}
	if parent != "" {
		line["parent_tool_use_id"] = parent
	}
	return []byte(jsonLine(line))
}

// contextOf is the one status among evs and its context figures.
func contextOf(t *testing.T, what string, evs []Event) wslive.UsageContext {
	t.Helper()
	got := statuses(evs)
	if len(got) != 1 || got[0].Context == nil || got[0].Context.Tokens == nil {
		t.Fatalf("%s: want one status with a context figure, got %+v", what, got)
	}
	return *got[0].Context
}

// TestClaudeLeadContextPerModelCall: a lead's assistant message whose
// usage changes the token count reports it at once; the same usage again
// (a message split over lines), a subagent's message, and an alert's do
// not, and neither does a profile conversation's message.
func TestClaudeLeadContextPerModelCall(t *testing.T) {
	m := newClaudeMapper(leadSpec())
	if c := contextOf(t, "first call", m.mapLine(usageLine(1200, ""), nopReplies{})); *c.Tokens != 1200 {
		t.Fatalf("tokens = %d, want 1200", *c.Tokens)
	}
	if got := statuses(m.mapLine(usageLine(1200, ""), nopReplies{})); len(got) != 0 {
		t.Fatalf("the same usage again reports nothing new, got %+v", got)
	}
	if c := contextOf(t, "next call", m.mapLine(usageLine(1800, ""), nopReplies{})); *c.Tokens != 1800 {
		t.Fatalf("tokens = %d, want 1800", *c.Tokens)
	}
	if got := statuses(m.mapLine(usageLine(90_000, "toolu_sub"), nopReplies{})); len(got) != 0 {
		t.Fatalf("a subagent's usage is not the lead's context, got %+v", got)
	}
	if got := statuses(m.mapLine([]byte(failureLine(map[string]any{"error": "authentication_failed"}, "Not logged in")), nopReplies{})); len(got) != 0 {
		t.Fatalf("an alert's message carries no real usage, got %+v", got)
	}
	result := `{"type":"result","subtype":"success","modelUsage":{"claude-opus-5-5":{"contextWindow":1000000}}}`
	if c := contextOf(t, "result", m.mapLine([]byte(result), nopReplies{})); *c.Tokens != 1800 {
		t.Fatalf("the context stays the lead's last call: %d", *c.Tokens)
	}

	profile := newClaudeMapper(claudeSpec(t, `{"backend":"claude"}`))
	if got := statuses(profile.mapLine(usageLine(1200, ""), nopReplies{})); len(got) != 0 {
		t.Fatalf("a profile conversation reports its context at the result only, got %+v", got)
	}
}

// TestClaudeLeadCompactionWindow: a lead's init asks get_context_usage
// once per model, and its rawMaxTokens is the window from then on, over the
// result's modelUsage.contextWindow; until an answer comes, or when the ask
// is refused, the window is the result's. A model change asks again and
// falls back until answered.
func TestClaudeLeadCompactionWindow(t *testing.T) {
	m := newClaudeMapper(leadSpec())
	replies := &recordedReplies{}
	feed := func(line string) []Event { return m.mapLine([]byte(line), replies) }
	init := func(model string) string {
		return `{"type":"system","subtype":"init","session_id":"s-1","model":"` + model + `","permissionMode":"default"}`
	}
	askID := func(i int) string {
		t.Helper()
		if len(replies.sent) <= i {
			t.Fatalf("want ask %d, sent %v", i, replies.sent)
		}
		req, _ := replies.sent[i]["request"].(map[string]any)
		id, _ := replies.sent[i]["request_id"].(string)
		if replies.sent[i]["type"] != "control_request" || req["subtype"] != "get_context_usage" || !strings.HasPrefix(id, claudeContextAsk) {
			t.Fatalf("ask %d = %v", i, replies.sent[i])
		}
		return id
	}
	answer := func(id string, raw int64) string {
		return jsonLine(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": id,
			"response": map[string]any{"totalTokens": 13805, "maxTokens": raw, "rawMaxTokens": raw, "autocompactSource": "env"}}})
	}
	result := func(window int64) string {
		return jsonLine(map[string]any{"type": "result", "subtype": "success", "modelUsage": map[string]any{"m": map[string]any{"contextWindow": window}}})
	}

	feed(init("claude-opus-5-5"))
	first := askID(0)
	if c := contextOf(t, "before any answer", feed(string(usageLine(5000, "")))); c.Window != 0 {
		t.Fatalf("no window is known before a result or an answer: %d", c.Window)
	}
	if c := contextOf(t, "fallback", feed(result(1_000_000))); c.Window != 1_000_000 {
		t.Fatalf("with no answer the window is the model's: %d", c.Window)
	}
	if c := contextOf(t, "answer", feed(answer(first, 300_000))); c.Window != 300_000 || *c.Tokens != 5000 {
		t.Fatalf("the answer reports the compaction window at once: %+v", c)
	}
	if got := statuses(feed(answer(first, 300_000))); len(got) != 0 {
		t.Fatalf("the same window again reports nothing new, got %+v", got)
	}
	if c := contextOf(t, "result after answer", feed(result(1_000_000))); c.Window != 300_000 {
		t.Fatalf("the compaction window wins over modelUsage: %d", c.Window)
	}
	feed(init("claude-opus-5-5"))
	if len(replies.sent) != 1 {
		t.Fatalf("a known window is not asked again: %v", replies.sent)
	}

	// A model change: the next turn's init names it and asks again; the
	// window falls back until the answer.
	feed(init("claude-sonnet-5-5"))
	second := askID(1)
	if c := contextOf(t, "new model", feed(result(1_000_000))); c.Window != 1_000_000 {
		t.Fatalf("an answer for another model does not count: %d", c.Window)
	}
	refused := jsonLine(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": second, "error": "get_context_usage is not supported in this context"}})
	if got := statuses(feed(refused)); len(got) != 0 {
		t.Fatalf("a refused ask reports nothing, got %+v", got)
	}
	feed(init("claude-sonnet-5-5"))
	third := askID(2)
	// A replayed answer is recognised by its prefix whatever its number.
	if c := contextOf(t, "answer for the new model", feed(answer(claudeContextAsk+"99", 200_000))); c.Window != 200_000 {
		t.Fatalf("window = %d, want 200000 (ask %s)", c.Window, third)
	}
	if got := statuses(feed(answer("ws-7", 400_000))); len(got) != 0 || m.compactWindow != 200_000 {
		t.Fatalf("another control request's answer is not a window: %+v, window %d", got, m.compactWindow)
	}
}
