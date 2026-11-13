package wsagent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wslive"
)

// Exercise the wire decoder and finished ring, not just the mapper's raw JSON:
// those are the batches the conversation renderer reads for its token chips.
func TestHostedBatchUsageFromAssistantMessages(t *testing.T) {
	for _, backend := range []string{BackendClaude, BackendPi} {
		t.Run(backend, func(t *testing.T) {
			st := wslive.NewState("lead")
			var ring []wslive.RingEntry
			apply := func(evs []Event) {
				for _, ev := range evs {
					if ev.Live == nil {
						continue
					}
					res, err := st.Apply(*ev.Live, 1000)
					if err != nil {
						t.Fatal(err)
					}
					ring = append(ring, res.Ring...)
				}
			}
			_, _ = st.Apply(wslive.Event{Kind: wslive.KindTurnStart}, 1)
			var batches *batchTracker
			if backend == BackendClaude {
				m := newClaudeMapper(StartSpec{})
				batches = &m.batches
				apply(m.assistant(json.RawMessage(`{"id":"a","content":[{"type":"tool_use","id":"c1","name":"Read","input":{}},{"type":"tool_use","id":"c2","name":"Grep","input":{}}],"usage":{"input_tokens":12,"output_tokens":4,"cache_read_input_tokens":30,"cache_creation_input_tokens":5}}`), 100, false))
				// A split line replaces counts, it must not sum them per call
				// or accidentally count the same message twice.
				update := json.RawMessage(`{"id":"a","content":[],"usage":{"input_tokens":12,"output_tokens":8,"cache_read_input_tokens":30,"cache_creation_input_tokens":5}}`)
				apply(m.assistant(update, 110, false))
				apply(m.assistant(update, 120, false))
			} else {
				m := newPiMapper(StartSpec{})
				batches = &m.batches
				apply(m.message(json.RawMessage(`{"role":"assistant","content":[{"type":"toolCall","id":"c1","name":"read","arguments":{}},{"type":"toolCall","id":"c2","name":"grep","arguments":{}}],"usage":{"input":12,"output":8,"cacheRead":30,"cacheWrite":5,"cost":{"total":0.1}}}`), 100))
			}
			want := wslive.TokenUsage{Input: 12, Output: 8, CacheRead: 30, CacheWrite: 5}
			if b := st.Snapshot(150).Lead.Batches; len(b) != 1 || b[0].Running == nil {
				t.Fatal("missing running batch")
			}
			apply(batches.endCall("c1", false, 200))
			apply(batches.endCall("c2", false, 300))
			apply([]Event{{Live: &wslive.Event{Kind: wslive.KindReply, Text: "done", Final: true}}})
			if len(ring) != 2 || ring[0].Tools == nil || len(ring[0].Tools.Batches) != 1 {
				t.Fatalf("finished batches: %+v", ring)
			}
			b := ring[0].Tools.Batches[0]
			if b.Usage == nil || *b.Usage != want || b.Calls != 2 {
				t.Fatalf("finished usage = %+v, calls = %d", b.Usage, b.Calls)
			}
			// A later batch with no usage stays absent, not the previous
			// message's usage or a fabricated zero-count chip.
			apply(batches.addCall("next", "c3", "read", "", 400))
			apply(batches.endCall("c3", false, 500))
			apply([]Event{{Live: &wslive.Event{Kind: wslive.KindReply, Text: "done again", Final: true}}})
			if b := ring[len(ring)-2].Tools.Batches[0]; b.Usage != nil {
				t.Fatalf("usage leaked between batches: %+v", b)
			}
		})
	}
}

func TestPiMidTurnStatsThrottleAndOwnOutputRate(t *testing.T) {
	m := newPiMapper(StartSpec{})
	r := &recordedReplies{}
	clock := int64(10_000)
	m.now = func() time.Time { return time.UnixMilli(clock) }
	line := func(s string) []Event { return m.mapLine([]byte(s), r) }
	line(`{"type":"message_start","message":{"role":"assistant"}}`)
	line(`{"type":"message_update"}`)
	clock += 500
	line(`{"type":"message_end","message":{"role":"assistant","stopReason":"toolUse","content":[],"usage":{"output":20}}}`)
	if len(r.sent) != 1 || r.sent[0]["type"] != "get_session_stats" {
		t.Fatalf("running message did not ask for stats: %+v", r.sent)
	}
	line(`{"type":"turn_end"}`)
	clock += 999
	line(`{"type":"turn_end"}`)
	if len(r.sent) != 1 {
		t.Fatalf("mid-turn stats not throttled: %+v", r.sent)
	}
	clock++
	line(`{"type":"turn_end"}`)
	if len(r.sent) != 2 {
		t.Fatalf("stats not refreshed after a second: %+v", r.sent)
	}
	evs := line(`{"type":"response","command":"get_session_stats","success":true,"data":{"contextUsage":{"tokens":1234,"contextWindow":10000}}}`)
	if len(evs) != 1 || evs[0].Live == nil || evs[0].Live.OutputTps == nil || *evs[0].Live.OutputTps != 40 || *evs[0].Live.Context.Tokens != 1234 {
		t.Fatalf("child's own context/rate missing: %+v", evs)
	}
	line(`{"type":"agent_settled"}`)
	if len(r.sent) != 3 {
		t.Fatalf("settlement must still request final stats: %+v", r.sent)
	}
}

func TestHostedPiRowShowsRunningTool(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{})
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(child.ID)
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"message_end","message":{"role":"assistant","stopReason":"toolUse","content":[{"type":"toolCall","id":"c1","name":"read","arguments":{"path":"example"}}]}}`)
	waitRunning := func(want bool) {
		t.Helper()
		eventually(t, "child tool activity in hosted row", func() bool {
			for _, a := range h.snapshot(lead.ID).Agents {
				if a.ID != "pi:"+child.ID {
					continue
				}
				for _, b := range a.Batches {
					if b.Running != nil && b.Running.ToolName == "read" {
						return want
					}
				}
				return !want
			}
			return false
		})
	}
	waitRunning(true)
	// An activity-only native alias must not mask the child's batches.
	h.runner.conv(lead.ID).do(func() {
		h.runner.conv(lead.ID).apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "native-worker", SessionKey: h.record(child.ID).WSKey, Caption: strPtr("working")})
	})
	waitRunning(true)
	h.holders.out(holder, `{"type":"tool_execution_end","toolCallId":"c1","isError":false}`)
	waitRunning(false)
}

func TestHostedPiRowUsesOwnUsageWhileRunningAndAfterRecovery(t *testing.T) {
	h, _ := newFakePiRecoveryHarness(t)
	lead := h.lead(BackendClaude, LeadOptions{})
	child, err := h.runner.Spawn(SpawnSpec{Profile: PiWorkerProfile, Root: h.root, Prompt: "work", Parent: &ParentRef{Kind: ParentLead, ReplyID: lead.ID}})
	if err != nil {
		t.Fatal(err)
	}
	holder := h.holder(child.ID)
	h.holders.out(holder, `{"type":"agent_start"}`, `{"type":"response","command":"get_session_stats","success":true,"data":{"contextUsage":{"tokens":48211,"contextWindow":100000}}}`)
	waitTokens := func(status string) {
		t.Helper()
		eventually(t, "child usage in hosted row", func() bool {
			for _, a := range h.snapshot(lead.ID).Agents {
				if a.ID == "pi:"+child.ID {
					return a.ContextTokens == 48211 && a.Status == status
				}
			}
			return false
		})
	}
	waitTokens(wslive.StatusRunning)
	// A native activity-only alias must not erase the daemon's own usage.
	h.runner.conv(lead.ID).do(func() {
		h.runner.conv(lead.ID).apply(wslive.Event{Kind: wslive.KindAgent, AgentID: "native-worker", SessionKey: h.record(child.ID).WSKey, Caption: strPtr("working")})
	})
	waitTokens(wslive.StatusRunning)
	// The row's rate source is also the child, never the lead's context.
	c := h.runner.conv(child.ID)
	c.do(func() {
		rate := 42.0
		tokens := int64(48211)
		c.apply(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{Tokens: &tokens}, OutputTps: &rate})
	})
	eventually(t, "child output rate", func() bool {
		for _, a := range h.snapshot(lead.ID).Agents {
			if a.ID == "pi:"+child.ID {
				return a.ContextTokens == 48211 && a.OutputTps == 42
			}
		}
		return false
	})
	c.do(func() { c.apply(wslive.Event{Kind: wslive.KindStatus, Context: &wslive.UsageContext{}}) })
	eventually(t, "unknown child context clears the row", func() bool {
		for _, a := range h.snapshot(lead.ID).Agents {
			if a.ID == "pi:"+child.ID {
				return a.ContextTokens == 0
			}
		}
		return false
	})
	h.holders.out(holder, `{"type":"response","command":"get_session_stats","success":true,"data":{"contextUsage":{"tokens":48211,"contextWindow":100000}}}`)
	waitTokens(wslive.StatusRunning)
	h.restart()
	waitTokens(wslive.StatusRunning)
	if _, err := h.runner.CloseConversation(child.ID); err != nil {
		t.Fatal(err)
	}
	waitTokens(wslive.StatusDone)
	for _, a := range h.snapshot(lead.ID).Agents {
		if a.ID == "pi:"+child.ID && a.OutputTps != 0 {
			t.Fatalf("settled row kept streaming rate: %+v", a)
		}
	}
	h.restart()
	waitTokens(wslive.StatusDone)
}
