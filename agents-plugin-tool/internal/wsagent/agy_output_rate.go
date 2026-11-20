package wsagent

import (
	"bytes"
	"encoding/json"
	"math"

	"github.com/kang-sw/devenv/internal/wslive"
)

const (
	// agyOutputRateWindow is the maximum number of recent eligible responses
	// aggregated into the token-weighted output rate.
	agyOutputRateWindow = 8
	// agyOutputRateMinSpanMs rejects burst-flushed responses whose summed
	// message-delta span is too short to measure reliably.
	agyOutputRateMinSpanMs = 250
	// agyOutputRateMinTokens rejects responses with too few visible tokens.
	agyOutputRateMinTokens = 8
)

type agyRateSample struct {
	tokens int64
	ms     int64
}

// agyPromptSpan tracks the local receipt span of consecutive
// agent_message_chunk runs for one in-flight session/prompt request.
type agyPromptSpan struct {
	id       string
	spanMs   int64
	inRun    bool
	runFirst int64
	runLast  int64
	replayed bool
}

func (s *agyPromptSpan) recordMessageChunk(now int64) {
	if !s.inRun {
		s.inRun = true
		s.runFirst = now
		s.runLast = now
		return
	}
	if now > s.runLast {
		s.runLast = now
	}
}

func (s *agyPromptSpan) closeRun() {
	if !s.inRun {
		return
	}
	if s.runLast > s.runFirst {
		s.spanMs += s.runLast - s.runFirst
	}
	s.inRun = false
}

func (s *agyPromptSpan) finish() int64 {
	s.closeRun()
	return s.spanMs
}

// agyOutputRateTracker holds in-memory per-prompt spans and the rolling window
// of eligible completed responses for one agyMapper.
type agyOutputRateTracker struct {
	spans  []*agyPromptSpan
	window []agyRateSample
}

func (t *agyOutputRateTracker) promptSent(id string) {
	t.spans = append(t.spans, &agyPromptSpan{id: id})
}

func (t *agyOutputRateTracker) promptDropped(id string) {
	for i, s := range t.spans {
		if s.id == id {
			t.spans = append(t.spans[:i], t.spans[i+1:]...)
			return
		}
	}
}

// dropUntracked discards leading spans opened without a tracked prompt id
// (for example, an unsolicited background-wakeup turn that closed quietly or
// was superseded by a tracked prompt).
func (t *agyOutputRateTracker) dropUntracked() {
	for len(t.spans) > 0 && t.spans[0].id == "" {
		t.spans = t.spans[1:]
	}
}

// messageChunk attributes a visible text chunk to the oldest unanswered prompt.
func (t *agyOutputRateTracker) messageChunk(now int64, replaying bool) {
	if len(t.spans) == 0 {
		t.spans = append(t.spans, &agyPromptSpan{replayed: replaying})
	}
	s := t.spans[0]
	if replaying {
		s.replayed = true
	}
	s.recordMessageChunk(now)
}

// breakRun closes any open message-delta run on the oldest unanswered prompt
// when another session update (thought chunk, tool call, usage update, etc.)
// intervenes.
func (t *agyOutputRateTracker) breakRun(replaying bool) {
	if len(t.spans) == 0 {
		return
	}
	s := t.spans[0]
	if replaying {
		s.replayed = true
	}
	s.closeRun()
}

// clearWindow resets the rolling window on a model change without publishing.
func (t *agyOutputRateTracker) clearWindow() {
	t.window = nil
}

func (t *agyOutputRateTracker) popSpan(id string) *agyPromptSpan {
	for i, s := range t.spans {
		if s.id == id {
			t.spans = append(t.spans[:i], t.spans[i+1:]...)
			return s
		}
	}
	// A live result from a prompt sent by an earlier daemon can close the
	// leading untracked response, but it cannot supply the JSON-RPC identity
	// that opened that span in this mapper. Drop it without publishing.
	if len(t.spans) > 0 && t.spans[0].id == "" {
		t.spans = t.spans[1:]
	}
	return nil
}

// complete closes the prompt span matching id, evaluates eligibility against
// the prompt result's visible tokens (outputTokens - thoughtTokens) and span,
// and returns the updated token-weighted rolling rate when eligible.
func (t *agyOutputRateTracker) complete(id string, msg agyMessage, replaying bool) (float64, bool) {
	span := t.popSpan(id)
	if replaying || span == nil || span.replayed {
		return 0, false
	}
	spanMs := span.finish()
	if msg.Error != nil || spanMs < agyOutputRateMinSpanMs {
		return 0, false
	}
	visibleTokens, ok := agyVisibleOutputTokens(msg.Result)
	if !ok || visibleTokens < agyOutputRateMinTokens {
		return 0, false
	}
	sampleRate := float64(visibleTokens) * 1000 / float64(spanMs)
	if math.IsNaN(sampleRate) || math.IsInf(sampleRate, 0) || sampleRate <= 0 || sampleRate > wslive.MaxOutputTps {
		return 0, false
	}
	next := t.window
	if len(next) >= agyOutputRateWindow {
		next = next[len(next)-(agyOutputRateWindow-1):]
	}
	totalTokens := visibleTokens
	totalMs := spanMs
	for _, s := range next {
		totalTokens += s.tokens
		totalMs += s.ms
	}
	if totalMs <= 0 {
		return 0, false
	}
	rate := float64(totalTokens) * 1000 / float64(totalMs)
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > wslive.MaxOutputTps {
		return 0, false
	}
	t.window = append(next, agyRateSample{tokens: visibleTokens, ms: spanMs})
	return rate, true
}

// agyVisibleOutputTokens parses usage.outputTokens - usage.thoughtTokens from
// an ACP session/prompt result. Missing or non-numeric outputTokens, a
// non-numeric thoughtTokens, a non-normal stopReason, or a negative difference
// returns ok=false. An absent or null thoughtTokens counts as 0.
func agyVisibleOutputTokens(result json.RawMessage) (int64, bool) {
	var r struct {
		StopReason string `json:"stopReason"`
		Usage      *struct {
			OutputTokens  json.RawMessage `json:"outputTokens"`
			ThoughtTokens json.RawMessage `json:"thoughtTokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(result, &r) != nil || r.Usage == nil {
		return 0, false
	}
	if r.StopReason != "" && r.StopReason != "end_turn" {
		return 0, false
	}
	outTokens, ok := parseNonNegJSONInt(r.Usage.OutputTokens)
	if !ok {
		return 0, false
	}
	var thoughtTokens int64
	if raw := bytes.TrimSpace(r.Usage.ThoughtTokens); len(raw) > 0 && !bytes.Equal(raw, []byte("null")) {
		thoughtTokens, ok = parseNonNegJSONInt(raw)
		if !ok {
			return 0, false
		}
	}
	visible := outTokens - thoughtTokens
	if visible < 0 {
		return 0, false
	}
	return visible, true
}

func parseNonNegJSONInt(raw json.RawMessage) (int64, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, false
	}
	// Reject JSON strings, booleans, arrays, and objects.
	if raw[0] != '-' && (raw[0] < '0' || raw[0] > '9') {
		return 0, false
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, n >= 0
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || math.Trunc(f) != f {
		return 0, false
	}
	return int64(f), true
}

func cloneUsageContext(ctx *wslive.UsageContext) *wslive.UsageContext {
	if ctx == nil {
		return nil
	}
	c := *ctx
	if ctx.Tokens != nil {
		v := *ctx.Tokens
		c.Tokens = &v
	}
	if ctx.Percent != nil {
		v := *ctx.Percent
		c.Percent = &v
	}
	if ctx.CacheHit != nil {
		v := *ctx.CacheHit
		c.CacheHit = &v
	}
	if ctx.Compaction != nil {
		comp := *ctx.Compaction
		if len(comp.Milestones) > 0 {
			comp.Milestones = append([]int64(nil), comp.Milestones...)
		} else {
			comp.Milestones = nil
		}
		c.Compaction = &comp
	}
	return &c
}
