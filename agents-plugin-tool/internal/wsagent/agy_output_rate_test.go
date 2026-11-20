package wsagent

import (
	"encoding/json"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsstate"
)

func agyChunkLine(text string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sid-1",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	})
	return raw
}

func agyThoughtLine(text string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sid-1",
			"update": map[string]any{
				"sessionUpdate": "agent_thought_chunk",
				"content":       map[string]any{"type": "text", "text": text},
			},
		},
	})
	return raw
}

func agyToolCallLine(callID, status string) []byte {
	kind := "tool_call"
	if status == "completed" || status == "failed" {
		kind = "tool_call_update"
	}
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sid-1",
			"update": map[string]any{
				"sessionUpdate": kind,
				"toolCallId":    callID,
				"name":          "read_file",
				"status":        status,
			},
		},
	})
	return raw
}

func agyUsageUpdateLine(used, size int64) []byte {
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sid-1",
			"update": map[string]any{
				"sessionUpdate": "usage_update",
				"used":          used,
				"size":          size,
			},
		},
	})
	return raw
}

func agyConfigUpdateLine(model, thoughtLevel string) []byte {
	opts := []map[string]any{
		{
			"id":           "model",
			"currentValue": model,
			"options":      []map[string]any{{"value": model}},
		},
	}
	if thoughtLevel != "" {
		opts = append(opts, map[string]any{
			"id":           "thought_level",
			"currentValue": thoughtLevel,
			"options":      []map[string]any{{"value": thoughtLevel}},
		})
	}
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]any{
			"sessionId": "sid-1",
			"update": map[string]any{
				"sessionUpdate": "config_option_update",
				"configOptions": opts,
			},
		},
	})
	return raw
}

func agyPromptResultLine(id, stopReason string, usage any) []byte {
	res := map[string]any{"stopReason": stopReason}
	if usage != nil {
		res["usage"] = usage
	}
	raw, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  res,
	})
	return raw
}

func agyRateEvents(events []Event) []*wslive.Event {
	var out []*wslive.Event
	for _, ev := range events {
		if ev.Live != nil && ev.Live.Kind == wslive.KindStatus && (ev.Live.OutputTps != nil || ev.Live.OutputRate != nil) {
			out = append(out, ev.Live)
		}
	}
	return out
}

func agyDownstreamEvents(events []Event) []Event {
	var visible []Event
	for _, event := range events {
		activity := event.ModelActivity
		event.ModelActivity = nil
		if activity != nil && reflect.DeepEqual(event, Event{}) {
			continue
		}
		visible = append(visible, event)
	}
	return visible
}

func assertNear(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s = %.6f, want %.6f", label, got, want)
	}
}

func agyOutputRateFileIO() int64 {
	var total int64
	for _, count := range agyOutputRateFileOps() {
		total += count
	}
	return total
}

func agyOutputRateFileOps() map[string]int64 {
	out := map[string]int64{}
	for site, count := range wsstate.FileIOCounters() {
		if strings.HasPrefix(site, "file_io_site:wslive/output_rate.go:") {
			op := site[strings.LastIndexByte(site, ':')+1:]
			out[op] += count
		}
	}
	return out
}

// TestAgyOutputRateFromRecordedFixtures drives the recorded agy ACP fixtures
// with synthetic local receipt times and checks visible-token computation
// (outputTokens - thoughtTokens), multi-run splitting, and mostly-thinking exclusion.
func TestAgyOutputRateFromRecordedFixtures(t *testing.T) {
	t.Run("subagent-removed visible tokens and mostly-thinking ineligible", func(t *testing.T) {
		clock := int64(10_000)
		r := &agyRun{m: newAgyMapper(""), answers: map[string]controlAnswer{}}
		r.m.now = func() int64 { return clock }
		ids := map[string]string{}
		n := 0
		chunkIdx := 0
		for _, line := range readLeadFixture(t, "agy-1.5.9-subagent-removed.jsonl") {
			var msg map[string]json.RawMessage
			if err := json.Unmarshal(line.raw, &msg); err != nil {
				t.Fatal(err)
			}
			_, hasMethod := msg["method"]
			id, hasID := msg["id"]
			if line.in {
				if !hasMethod || !hasID {
					continue
				}
				var method string
				_ = json.Unmarshal(msg["method"], &method)
				kind := agyFixtureKinds[method]
				if kind == "" {
					kind = "other"
				}
				n++
				wsID := agyRequestMark + "-" + kind + "-fix-" + strconv.Itoa(n)
				ids[string(id)] = wsID
				if kind == agyKindPrompt {
					r.m.promptSent(wsID)
				}
				continue
			}
			raw := line.raw
			if !hasMethod && hasID {
				if wsID, ok := ids[string(id)]; ok {
					msg["id"], _ = json.Marshal(wsID)
					raw, _ = json.Marshal(msg)
				}
			}
			// Advance clock across the two message chunks of prompt 1 by 500ms.
			if hasMethod {
				var p struct {
					Update struct {
						SessionUpdate string `json:"sessionUpdate"`
					} `json:"update"`
				}
				_ = json.Unmarshal(msg["params"], &p)
				switch p.Update.SessionUpdate {
				case "agent_thought_chunk":
					clock += 4_000 // thinking latency must not enter span
				case "agent_message_chunk":
					chunkIdx++
					if chunkIdx == 2 {
						clock += 500
					} else {
						clock += 100
					}
				default:
					clock += 50
				}
			} else {
				clock += 10
			}
			r.events = append(r.events, r.m.mapLine(raw, r)...)
		}
		rates := agyRateEvents(r.events)
		// Prompt 1: 1023 output - 988 thought = 35 visible tokens over 500ms = 70 t/s.
		// Prompt 2: 175 output - 169 thought = 6 visible tokens (and 1 chunk = 0ms) -> ineligible.
		if len(rates) != 1 {
			t.Fatalf("rate events = %d, want 1 (%+v)", len(rates), rates)
		}
		ev := rates[0]
		if ev.Observed != nil {
			t.Fatalf("agy emitted ModelObservation: %+v", ev.Observed)
		}
		if ev.OutputTps == nil || ev.OutputRate == nil {
			t.Fatalf("missing OutputTps or OutputRate: %+v", ev)
		}
		assertNear(t, "prompt 1 OutputTps", *ev.OutputTps, 70.0)
		assertNear(t, "prompt 1 OutputRate.OutputTps", ev.OutputRate.OutputTps, 70.0)
		if ev.Context == nil || ev.Context.Tokens == nil || *ev.Context.Tokens != 4771 {
			t.Fatalf("KindStatus rate event cleared Context: %+v", ev.Context)
		}
	})

	t.Run("hosted-lead multi-run split and missing usage", func(t *testing.T) {
		// Lines 40-48 of agy-1.5.10-hosted-lead.jsonl:
		// 2 chunks -> usage_update -> 1 chunk -> thought -> 2 chunks -> usage_update -> result (1144 out, 1072 thought = 72 visible).
		lines := readLeadFixture(t, "agy-1.5.10-hosted-lead.jsonl")
		clock := int64(50_000)
		r := &agyRun{m: newAgyMapper("sid"), answers: map[string]controlAnswer{}}
		r.m.now = func() int64 { return clock }
		r.m.promptSent("ws-p-dmxuearjjwyw-3")
		// Step by fixed deltas per fixture line (1-based lines 40..48 -> indices 39..47):
		// line 40 (msg): +0     -> run1 start 50000
		// line 41 (msg): +200   -> run1 end   50200 (200ms)
		// line 42 (usage): +500 -> closes run1
		// line 43 (msg): +1000  -> run2 single chunk (0ms)
		// line 44 (thought): +3000 -> closes run2
		// line 45 (msg): +1000  -> run3 start 55700
		// line 46 (msg): +300   -> run3 end   56000 (300ms)
		// line 47 (usage): +100 -> closes run3
		// line 48 (result): +10 -> total span = 200 + 0 + 300 = 500ms, visible = 72 -> 144 t/s
		deltas := []int64{0, 200, 500, 1000, 3000, 1000, 300, 100, 10}
		var evs []Event
		for i, d := range deltas {
			clock += d
			evs = append(evs, r.m.mapLine(lines[39+i].raw, r)...)
		}
		rates := agyRateEvents(evs)
		if len(rates) != 1 {
			t.Fatalf("hosted-lead multi-run rates = %d, want 1", len(rates))
		}
		assertNear(t, "multi-run rate", *rates[0].OutputTps, 72.0*1000/500.0)

		// Line 82 has no usage field -> must stay ineligible.
		r.m.promptSent("ws-p-dmxuearjjwyw-8")
		clock += 100
		r.m.mapLine(lines[80].raw, r)
		clock += 500
		evs2 := r.m.mapLine(lines[81].raw, r)
		if got := agyRateEvents(evs2); len(got) != 0 {
			t.Fatalf("missing-usage prompt emitted rate: %+v", got)
		}
	})
}

func TestAgyOutputRateBoundariesAndIneligibleCases(t *testing.T) {
	runCase := func(name string, spanMs int64, stopReason string, usage any, singleChunk bool, replaying bool) []*wslive.Event {
		t.Helper()
		clock := int64(10_000)
		m := newAgyMapper("sid-1")
		m.now = func() int64 { return clock }
		var replies lineReplies = nopReplies{}
		if replaying {
			replies = silentReplies{}
		}
		id := "ws-p-e-1"
		m.promptSent(id)
		m.mapLine(agyChunkLine("hello "), replies)
		if !singleChunk {
			clock += spanMs
			m.mapLine(agyChunkLine("world"), replies)
		}
		clock += 10
		evs := m.mapLine(agyPromptResultLine(id, stopReason, usage), replies)
		return agyRateEvents(evs)
	}

	// Exact eligibility boundaries: 250ms and 8 visible tokens.
	if got := runCase("249ms", 249, "end_turn", map[string]any{"outputTokens": 8, "thoughtTokens": 0}, false, false); len(got) != 0 {
		t.Fatalf("249ms should be ineligible: %+v", got)
	}
	if got := runCase("7 tokens", 250, "end_turn", map[string]any{"outputTokens": 7, "thoughtTokens": 0}, false, false); len(got) != 0 {
		t.Fatalf("7 tokens should be ineligible: %+v", got)
	}
	if got := runCase("mostly thinking 175-169=6", 1000, "end_turn", map[string]any{"outputTokens": 175, "thoughtTokens": 169}, false, false); len(got) != 0 {
		t.Fatalf("175-169=6 should be ineligible: %+v", got)
	}
	if got := runCase("negative diff", 1000, "end_turn", map[string]any{"outputTokens": 50, "thoughtTokens": 60}, false, false); len(got) != 0 {
		t.Fatalf("negative visible tokens should be ineligible: %+v", got)
	}
	if got := runCase("non-numeric outputTokens", 1000, "end_turn", map[string]any{"outputTokens": "100"}, false, false); len(got) != 0 {
		t.Fatalf("string outputTokens should be ineligible: %+v", got)
	}
	if got := runCase("missing outputTokens", 1000, "end_turn", map[string]any{"thoughtTokens": 0}, false, false); len(got) != 0 {
		t.Fatalf("missing outputTokens should be ineligible: %+v", got)
	}
	if got := runCase("cancelled", 1000, "cancelled", map[string]any{"outputTokens": 100, "thoughtTokens": 0}, false, false); len(got) != 0 {
		t.Fatalf("cancelled response should be ineligible: %+v", got)
	}
	if got := runCase("single chunk", 1000, "end_turn", map[string]any{"outputTokens": 100, "thoughtTokens": 0}, true, false); len(got) != 0 {
		t.Fatalf("single chunk (0ms span) should be ineligible: %+v", got)
	}
	if got := runCase("replaying", 1000, "end_turn", map[string]any{"outputTokens": 100, "thoughtTokens": 0}, false, true); len(got) != 0 {
		t.Fatalf("replaying response should be ineligible: %+v", got)
	}

	// Exact boundary pass: 250ms, 8 tokens (thoughtTokens omitted -> 0) => 32 t/s.
	got := runCase("250ms 8 tokens absent thought", 250, "end_turn", map[string]any{"outputTokens": 8}, false, false)
	if len(got) != 1 {
		t.Fatalf("exact boundary should be eligible: %+v", got)
	}
	assertNear(t, "boundary rate", *got[0].OutputTps, 32.0)

	// Thought-only response (no agent_message_chunk) is ineligible.
	clock := int64(10_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }
	m.promptSent("ws-p-e-2")
	m.mapLine(agyThoughtLine("thinking..."), nopReplies{})
	clock += 1000
	m.mapLine(agyThoughtLine("still thinking"), nopReplies{})
	clock += 100
	if evs := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 500, "thoughtTokens": 400}), nopReplies{})); len(evs) != 0 {
		t.Fatalf("thought-only response emitted rate: %+v", evs)
	}
}

func TestAgyOutputRateToolExclusionAndRunSplitting(t *testing.T) {
	clock := int64(10_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }
	m.promptSent("ws-p-e-1")

	// Pre-text latency (5s) excluded.
	clock += 5_000
	// Run 1: 200ms
	m.mapLine(agyChunkLine("a"), nopReplies{})
	clock += 200
	m.mapLine(agyChunkLine("b"), nopReplies{})
	// Tool call lasting 10s excluded.
	clock += 100
	m.mapLine(agyToolCallLine("call-1", "in_progress"), nopReplies{})
	clock += 10_000
	m.mapLine(agyToolCallLine("call-1", "completed"), nopReplies{})
	// Thought chunk lasting 3s excluded.
	clock += 500
	m.mapLine(agyThoughtLine("pondering"), nopReplies{})
	clock += 3_000
	// Run 2: 300ms
	m.mapLine(agyChunkLine("c"), nopReplies{})
	clock += 300
	m.mapLine(agyChunkLine("d"), nopReplies{})
	// Trailing latency before result excluded.
	clock += 2_000
	evs := m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 150, "thoughtTokens": 100}), nopReplies{})
	rates := agyRateEvents(evs)
	if len(rates) != 1 {
		t.Fatalf("rate events = %d, want 1", len(rates))
	}
	// 50 visible tokens / (200 + 300 ms) = 100 t/s.
	assertNear(t, "tool+thought excluded rate", *rates[0].OutputTps, 100.0)
}

func TestAgyOutputRateWindowAndModelChangeReset(t *testing.T) {
	clock := int64(10_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }
	m.mapLine(agyConfigUpdateLine("gemini-3.8-flash", "high"), nopReplies{})
	published := wslive.NewState("rate-retention")

	complete := func(seq int, tokens, spanMs int64) float64 {
		id := "ws-p-e-" + strconv.Itoa(seq)
		m.promptSent(id)
		clock += 100
		m.mapLine(agyChunkLine("x"), nopReplies{})
		clock += spanMs
		m.mapLine(agyChunkLine("y"), nopReplies{})
		clock += 10
		evs := m.mapLine(agyPromptResultLine(id, "end_turn", map[string]any{"outputTokens": tokens, "thoughtTokens": 0}), nopReplies{})
		rates := agyRateEvents(evs)
		if len(rates) != 1 {
			t.Fatalf("seq %d: rates=%d, want 1", seq, len(rates))
		}
		if _, err := published.Apply(*rates[0], clock); err != nil {
			t.Fatalf("seq %d apply rate: %v", seq, err)
		}
		return *rates[0].OutputTps
	}

	// Sample 1: 10 tokens / 1000ms = 10 t/s.
	assertNear(t, "sample 1", complete(1, 10, 1000), 10.0)
	// Samples 2..8: 7 samples of 50 tokens / 500ms each.
	// Total after 8: (10 + 350) / (1000 + 3500) * 1000 = 360000 / 4500 = 80 t/s.
	var r8 float64
	for i := 2; i <= 8; i++ {
		r8 = complete(i, 50, 500)
	}
	assertNear(t, "after 8 samples", r8, 80.0)

	// Changing thought_level without changing model does NOT clear window.
	if evs := m.mapLine(agyConfigUpdateLine("gemini-3.8-flash", "low"), nopReplies{}); len(agyRateEvents(evs)) != 0 {
		t.Fatalf("thought_level change published rate: %+v", evs)
	}

	// Sample 9: 50 tokens / 500ms drops sample 1 -> all 8 samples are 50/500 = 100 t/s!
	assertNear(t, "after 9th drops 1st", complete(9, 50, 500), 100.0)

	// Model change clears the window and publishes nothing.
	if evs := agyDownstreamEvents(m.mapLine(agyConfigUpdateLine("claude-sonnet-4-6", ""), nopReplies{})); len(evs) != 0 {
		t.Fatalf("model change should publish nothing, got %+v", evs)
	}
	if got := published.Snapshot(clock).Lead.OutputRate; got == nil || got.OutputTps != 100.0 {
		t.Fatalf("model reset cleared prior published rate: %+v", got)
	}
	// An ineligible response after reset also publishes nothing and retains the
	// prior downstream value while the fresh window stays empty.
	m.promptSent("ws-p-e-10")
	clock += 100
	m.mapLine(agyChunkLine("x"), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("y"), nopReplies{})
	clock += 10
	if evs := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-10", "end_turn", map[string]any{"outputTokens": 7, "thoughtTokens": 0}), nopReplies{})); len(evs) != 0 {
		t.Fatalf("ineligible completion after reset published rate: %+v", evs)
	}
	if got := published.Snapshot(clock).Lead.OutputRate; got == nil || got.OutputTps != 100.0 {
		t.Fatalf("ineligible completion cleared prior published rate: %+v", got)
	}
	// Next eligible response starts a fresh window: 20 tokens / 1000ms = 20 t/s.
	assertNear(t, "fresh window after model change", complete(11, 20, 1000), 20.0)
}

func TestAgyOutputRateSteeredTurnAttribution(t *testing.T) {
	clock := int64(20_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }

	// Prompt 1 starts and streams 2 chunks (300ms).
	m.promptSent("ws-p-e-1")
	m.mapLine(agyChunkLine("first "), nopReplies{})
	clock += 300
	m.mapLine(agyChunkLine("part "), nopReplies{})

	// Steer arrives BEFORE prompt 1's result.
	m.promptSent("ws-p-e-2")
	// Chunk arriving before prompt 1's result belongs to oldest unanswered prompt (prompt 1 -> +200ms = 500ms).
	clock += 200
	m.mapLine(agyChunkLine("tail"), nopReplies{})

	// Prompt 1 answers mid-turn: publishes 40 visible / 500ms = 80 t/s, NO TurnEnd!
	clock += 10
	evs1 := m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 140, "thoughtTokens": 100}), nopReplies{})
	for _, ev := range evs1 {
		if ev.TurnEnd {
			t.Fatal("earlier steered prompt ended the turn")
		}
	}
	rates1 := agyRateEvents(evs1)
	if len(rates1) != 1 {
		t.Fatalf("prompt 1 rate events = %d, want 1", len(rates1))
	}
	for _, ev := range evs1 {
		if ev.Live != nil && ev.Live.OutputRate != nil && ev.LiveCoalescesTurnEnd {
			t.Fatal("mid-turn steered rate incorrectly coalesced without TurnEnd")
		}
	}
	assertNear(t, "steered prompt 1 rate", *rates1[0].OutputTps, 80.0)

	// Now chunks belong to prompt 2 (500ms span).
	clock += 100
	m.mapLine(agyChunkLine("second "), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("reply"), nopReplies{})
	clock += 10
	evs2 := m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 260, "thoughtTokens": 200}), nopReplies{})
	rates2 := agyRateEvents(evs2)
	if len(rates2) != 1 {
		t.Fatalf("prompt 2 rate events = %d, want 1", len(rates2))
	}
	// Token-weighted window: (40 + 60) / (500 + 500) * 1000 = 100 t/s.
	assertNear(t, "steered prompt 2 window rate", *rates2[0].OutputTps, 100.0)
	if !evs2[len(evs2)-1].TurnEnd {
		t.Fatalf("final steered prompt did not end with TurnEnd: %+v", evs2)
	}
	coalesced := false
	for _, ev := range evs2 {
		if ev.Live != nil && ev.Live.OutputRate != nil {
			coalesced = ev.LiveCoalescesTurnEnd
		}
	}
	if !coalesced {
		t.Fatal("final prompt rate did not coalesce with its immediate TurnEnd")
	}
}

func TestAgyOutputRateMatchesPromptResultByJSONRPCID(t *testing.T) {
	clock := int64(30_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }
	m.promptSent("ws-p-e-1")
	m.mapLine(agyChunkLine("first"), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine(" response"), nopReplies{})
	m.promptSent("ws-p-e-2")

	// JSON-RPC permits responses to arrive out of order. Prompt 2 owns no
	// chunks, so its result is ineligible; it must not consume prompt 1's span.
	clock += 10
	if got := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 1000, "thoughtTokens": 0}), nopReplies{})); len(got) != 0 {
		t.Fatalf("out-of-order prompt 2 stole prompt 1 span: %+v", got)
	}
	clock += 10
	got := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 10, "thoughtTokens": 0}), nopReplies{}))
	if len(got) != 1 {
		t.Fatalf("prompt 1 matching span rate events = %d, want 1", len(got))
	}
	assertNear(t, "prompt 1 JSON-RPC matched rate", *got[0].OutputTps, 20.0)
}

func TestAgyOutputRateRejectsUntrackedPromptResult(t *testing.T) {
	clock := int64(40_000)
	m := newAgyMapper("sid-1")
	m.now = func() int64 { return clock }

	// A resumed response can stream before this mapper has the request id. Its
	// result must close the untracked span without turning it into a rate sample.
	m.mapLine(agyChunkLine("resumed"), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine(" response"), nopReplies{})
	clock += 10
	if got := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-old-1", "end_turn", map[string]any{"outputTokens": 100, "thoughtTokens": 0}), nopReplies{})); len(got) != 0 {
		t.Fatalf("untracked prompt result published rate: %+v", got)
	}

	// The next locally sent prompt opens a matched span and remains measurable.
	m.promptSent("ws-p-e-2")
	clock += 100
	m.mapLine(agyChunkLine("tracked"), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine(" response"), nopReplies{})
	clock += 10
	got := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 10, "thoughtTokens": 0}), nopReplies{}))
	if len(got) != 1 {
		t.Fatalf("tracked prompt after resumed response rates = %d, want 1", len(got))
	}
	assertNear(t, "tracked prompt after resumed response", *got[0].OutputTps, 20.0)
}

func TestAgyOutputRateRowHoverRetentionAndReopen(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_700_000_000, 0)
	r := NewRunner(Options{Dir: dir, Now: func() time.Time { return now }})
	rec := Record{
		ReplyID: "agychild1",
		Backend: "agy",
		Model:   "gemini-3.8-flash",
		Effort:  "high",
		WSKey:   "key-agychild1",
		Parent:  &ParentRef{Kind: ParentLead, ReplyID: "parentlead"},
	}
	sessionDir := wsdashboard.SessionDir(dir, rec.ReplyID)
	c := &conv{
		r:    r,
		dir:  sessionDir,
		rec:  rec,
		turn: true,
		live: newLiveWriter(sessionDir, rec.WSKey, rec.Backend, r.opts.Now, io.Discard),
	}

	// Turn 1: eligible completion while turn is running.
	clock := now.UnixMilli()
	m := newAgyMapper("sid-child")
	m.now = func() int64 { return clock }
	m.mapLine(agyUsageUpdateLine(4771, 1048576), nopReplies{})
	m.promptSent("ws-p-e-1")
	m.mapLine(agyChunkLine("chunk one "), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("chunk two"), nopReplies{})
	clock += 10
	for _, ev := range m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 1023, "thoughtTokens": 988}), nopReplies{}) {
		if ev.Live != nil {
			if ev.LiveCoalescesTurnEnd {
				c.applyCoalesced(*ev.Live, false)
			} else {
				c.apply(*ev.Live)
			}
		}
		if ev.TurnEnd {
			c.setStatus(func() { c.turn = false })
			c.apply(wslive.Event{Kind: wslive.KindTurnEnd, At: clock})
		}
	}

	// Idle after Turn 1: parent row hides rate (not running), hover record keeps 70 t/s, Observed stays nil.
	idleState := c.state()
	if idleState.OutputTps != nil {
		t.Fatalf("idle subagent exposed row OutputTps: %v", *idleState.OutputTps)
	}
	if idleState.Observed != nil {
		t.Fatalf("agy subagent fabricated Observed: %+v", idleState.Observed)
	}
	snap := c.live.st.Snapshot(clock + 100)
	if snap.Lead.Observed != nil || snap.Lead.OutputRate == nil {
		t.Fatalf("snapshot after turn 1 = %+v", snap.Lead)
	}
	assertNear(t, "hover OutputRate", snap.Lead.OutputRate.OutputTps, 70.0)

	// Turn 2 starts (generating): row exposes previous rate (70 t/s), hover keeps 70 t/s.
	c.setStatus(func() { c.turn = true })
	c.apply(wslive.Event{Kind: wslive.KindTurnStart, At: clock + 200})
	runningState := c.state()
	if runningState.OutputTps == nil {
		t.Fatal("running turn 2 subagent missing prior OutputTps")
	}
	assertNear(t, "turn 2 State.OutputTps", *runningState.OutputTps, 70.0)
	rowEv := SpawnAgentEvent(runningState, rec.WSKey)
	if rowEv.OutputTps == nil || *rowEv.OutputTps != 70.0 || rowEv.Observed != nil {
		t.Fatalf("SpawnAgentEvent = %+v", rowEv)
	}

	// Simulate old daemon overwriting snapshot.json without lead.outputRate, then reopen.
	legacy, _ := json.Marshal(map[string]any{
		"schemaVersion": 1,
		"replyId":       c.live.st.ReplyID(),
		"adapter":       "agy",
		"updatedAt":     clock + 300,
		"lead":          map[string]any{"phase": wslive.PhaseIdle},
	})
	if err := os.WriteFile(wslive.SnapshotPath(sessionDir), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	reopenedLive := newLiveWriter(sessionDir, rec.WSKey, rec.Backend, r.opts.Now, io.Discard)
	reopenedSnap := reopenedLive.st.Snapshot(clock + 400)
	if reopenedSnap.Lead.Observed != nil || reopenedSnap.Lead.OutputRate == nil {
		t.Fatalf("reopened snapshot lost OutputRate: %+v", reopenedSnap.Lead)
	}
	assertNear(t, "reopened OutputRate", reopenedSnap.Lead.OutputRate.OutputTps, 70.0)
}

func TestAgyOutputRateHostedApplyDefersSidecarToRecoveryFlush(t *testing.T) {
	dir := t.TempDir()
	host := &summarySnapshotHost{}
	slot, ok := host.OpenHostedSlot(dir)
	if !ok {
		t.Fatal("hosted slot refused")
	}
	w := newLiveWriter(dir, "agy-rate-io", "agy", time.Now, io.Discard)
	w.setSlot(slot)
	defer w.closeSlot()

	before := agyOutputRateFileIO()
	w.apply(wslive.Event{Kind: wslive.KindStatus, OutputRate: &wslive.OutputRate{OutputTps: 42, At: 1000}})
	if got := agyOutputRateFileIO() - before; got != 0 {
		t.Fatalf("hosted rate apply touched sidecar %d times, want 0", got)
	}
	if snap, ok := host.snapshot(dir); !ok || snap.Lead.OutputRate == nil || snap.Lead.OutputRate.OutputTps != 42 {
		t.Fatalf("hosted RAM snapshot = %+v, ok=%v", snap.Lead.OutputRate, ok)
	}

	w.apply(wslive.Event{Kind: wslive.KindTurnEnd, At: 1100})
	if got := agyOutputRateFileIO() - before; got != 4 {
		t.Fatalf("turn-end recovery flush sidecar operations = %d, want mkdir+open+stat+write", got)
	}
	if got, err := wslive.ReadOutputRate(dir); err != nil || got == nil || got.OutputTps != 42 {
		t.Fatalf("recovery sidecar = %+v, %v", got, err)
	}
}

func TestAgyOutputRateFileOnlyCompletionCoalescesSnapshotAndCountsIO(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	w := newLiveWriter(dir, "agy-rate-file-only", "agy", func() time.Time { return now }, io.Discard)
	writes := 0
	w.writeSnapshotFile = func(string, []byte, os.FileMode) error {
		writes++
		return nil
	}
	w.apply(wslive.Event{Kind: wslive.KindTurnStart})

	checkAction := func(label string, rate wslive.OutputRate, wantOps map[string]int64) {
		t.Helper()
		beforeWrites := writes
		before := agyOutputRateFileOps()
		w.applyCoalesced(wslive.Event{Kind: wslive.KindStatus, OutputRate: &rate})
		if writes != beforeWrites {
			t.Fatalf("%s rate status wrote snapshot before TurnEnd", label)
		}
		w.apply(wslive.Event{Kind: wslive.KindTurnEnd})
		if writes-beforeWrites != 1 {
			t.Fatalf("%s completion snapshot writes = %d, want 1", label, writes-beforeWrites)
		}
		after := agyOutputRateFileOps()
		for op, want := range wantOps {
			if got := after[op] - before[op]; got != want {
				t.Fatalf("%s %s operations = %d, want %d", label, op, got, want)
			}
		}
	}

	checkAction("first response", wslive.OutputRate{OutputTps: 42, At: 1000}, map[string]int64{
		"mkdir_all": 1, "open_file": 1, "stat": 1, "write": 1, "close": 0,
	})
	w.apply(wslive.Event{Kind: wslive.KindTurnStart})
	checkAction("steady response", wslive.OutputRate{OutputTps: 48, At: 2000}, map[string]int64{
		"mkdir_all": 0, "open_file": 0, "stat": 0, "write": 1, "close": 0,
	})
	beforeClose := agyOutputRateFileOps()
	if err := w.outputRates.Close(); err != nil {
		t.Fatal(err)
	}
	afterClose := agyOutputRateFileOps()
	if got := afterClose["close"] - beforeClose["close"]; got != 1 {
		t.Fatalf("session close operations = %d, want 1", got)
	}
}

func TestAgyOutputRateUnsolicitedTurnPromptTakeover(t *testing.T) {
	clock := int64(30_000)
	m := newAgyMapper("sid-unsolicited")
	m.now = func() int64 { return clock }

	// Live unsolicited wakeup streams chunks with no tracked prompt.
	m.mapLine(agyChunkLine("background wakeup "), nopReplies{})
	clock += 400
	m.mapLine(agyChunkLine("done"), nopReplies{})

	// A tracked prompt is sent before closeUnsolicited fires; it must drop the
	// unsolicited untracked span so its own chunks are attributed to ws-p-e-1.
	clock += 100
	m.promptSent("ws-p-e-1")
	m.mapLine(agyChunkLine("tracked "), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("reply"), nopReplies{})
	clock += 10
	evs1 := m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 140, "thoughtTokens": 100}), nopReplies{})
	rates1 := agyRateEvents(evs1)
	if len(rates1) != 1 {
		t.Fatalf("prompt 1 after unsolicited wakeup: rates=%d, want 1", len(rates1))
	}
	// 40 visible tokens / 500ms = 80 t/s (unsolicited 400ms must not leak in).
	assertNear(t, "prompt 1 rate", *rates1[0].OutputTps, 80.0)

	// Subsequent turn must also track cleanly.
	clock += 100
	m.promptSent("ws-p-e-2")
	m.mapLine(agyChunkLine("next "), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("turn"), nopReplies{})
	clock += 10
	evs2 := m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 160, "thoughtTokens": 100}), nopReplies{})
	rates2 := agyRateEvents(evs2)
	if len(rates2) != 1 {
		t.Fatalf("prompt 2 after unsolicited wakeup: rates=%d, want 1", len(rates2))
	}
	// Window: (40 + 60) / (500 + 500) * 1000 = 100 t/s.
	assertNear(t, "prompt 2 window rate", *rates2[0].OutputTps, 100.0)
}

func TestAgyOutputRateOutlierDoesNotPoisonWindow(t *testing.T) {
	clock := int64(40_000)
	m := newAgyMapper("sid-outlier")
	m.now = func() int64 { return clock }

	// Extreme outlier above wslive.MaxOutputTps (e.g. 500,000 tokens in 250ms = 2,000,000 t/s).
	m.promptSent("ws-p-e-1")
	m.mapLine(agyChunkLine("a"), nopReplies{})
	clock += 250
	m.mapLine(agyChunkLine("b"), nopReplies{})
	clock += 10
	if rates := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-1", "end_turn", map[string]any{"outputTokens": 500_000, "thoughtTokens": 0}), nopReplies{})); len(rates) != 0 {
		t.Fatalf("outlier above MaxOutputTps emitted rate: %+v", rates)
	}

	// Next eligible response must still publish cleanly without the outlier in the window.
	clock += 100
	m.promptSent("ws-p-e-2")
	m.mapLine(agyChunkLine("c"), nopReplies{})
	clock += 500
	m.mapLine(agyChunkLine("d"), nopReplies{})
	clock += 10
	rates := agyRateEvents(m.mapLine(agyPromptResultLine("ws-p-e-2", "end_turn", map[string]any{"outputTokens": 50, "thoughtTokens": 0}), nopReplies{}))
	if len(rates) != 1 {
		t.Fatalf("eligible response after outlier: rates=%d, want 1", len(rates))
	}
	assertNear(t, "post-outlier rate", *rates[0].OutputTps, 100.0)
}
