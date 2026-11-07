package wslive

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func kinds(ring []RingEntry) []string {
	out := make([]string, len(ring))
	for i, e := range ring {
		out[i] = e.Kind
	}
	return out
}

func finishedAt(seq int64, start, end int64, calls, failed int, tools ...string) map[string]any {
	b := finished(seq, calls, failed, tools...)
	b["startedAt"], b["endedAt"] = start, end
	return b
}

// TestToolsRowFlushesBeforeTheNextConversationEntry pins each flush point:
// the finished lead batches of a gap become one tools entry written just
// before the reply, user message, or compaction that ends the gap, and a
// gap that had no finished batch writes none.
func TestToolsRowFlushesBeforeTheNextConversationEntry(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart, At: 1}, 1)
	if res := mustApply(t, st, Event{Kind: KindReply, Final: true, Text: "hello"}, 2); !reflect.DeepEqual(kinds(res.Ring), []string{RingReply}) {
		t.Fatalf("a gap with no finished batch ring = %v, want the reply alone", kinds(res.Ring))
	}

	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(1, 100, 200, 2, 0, "Bash"), running(2, 1, 0, "Read", ""))}, 3)
	res := mustApply(t, st, Event{Kind: KindReply, Final: true, Text: "next"}, 4)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingReply}) {
		t.Fatalf("reply flush ring = %v", kinds(res.Ring))
	}
	got := res.Ring[0].Tools
	want := &ToolSummary{Calls: 2, DurationMs: 100, Tools: []ToolTally{{Name: "Bash", Calls: 2}}, Batches: []ToolBatch{{Names: []string{"Bash"}, Calls: 2, DurationMs: 100}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary = %+v, want %+v", got, want)
	}

	// The gap is empty again; the running batch finishes into the next one.
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(2, 300, 450, 1, 1, "Read"))}, 5)
	res = mustApply(t, st, Event{Kind: KindUser, Text: "q"}, 6)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingUser}) || res.Ring[0].Tools.Failed != 1 {
		t.Fatalf("user flush ring = %+v", res.Ring)
	}

	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(3, 500, 600, 1, 0, "Grep"))}, 7)
	res = mustApply(t, st, Event{Kind: KindCompact, Phase: CompactEnd}, 8)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingCompact}) {
		t.Fatalf("compact flush ring = %v", kinds(res.Ring))
	}

	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(4, 700, 800, 1, 0, "Edit"))}, 9)
	if res := mustApply(t, st, Event{Kind: KindCompact, Phase: CompactStart}, 10); len(res.Ring) != 0 {
		t.Fatalf("a compaction start wrote %v", kinds(res.Ring))
	}
	res = mustApply(t, st, Event{Kind: KindTurnEnd, At: 1000}, 1000)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingTurn}) {
		t.Fatalf("turn end ring = %v", kinds(res.Ring))
	}
	if turn := res.Ring[1]; turn.Done != 4 || turn.Tools != nil {
		t.Fatalf("the turn line changed: %+v", turn)
	}
	if st.Snapshot(1000).Lead.ToolGap != nil || len(st.FlushTools()) != 0 {
		t.Fatal("the gap outlived its flush")
	}
}

// TestToolsRowOrderAtTurnBoundaries pins the order around a reply that is
// still streaming and a batch the turn end closes: tools, reply, tools the
// close finished, turn line.
func TestToolsRowOrderAtTurnBoundaries(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart, At: 1000}, 1000)
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(1, 1100, 1200, 1, 0, "Bash"), running(2, 2, 1, "Read", ""))}, 1300)
	mustApply(t, st, Event{Kind: KindReply, Text: "streaming"}, 1400)
	res := mustApply(t, st, Event{Kind: KindTurnEnd, At: 2000}, 2000)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingReply, RingTools, RingTurn}) {
		t.Fatalf("ring = %v", kinds(res.Ring))
	}
	closed := res.Ring[2].Tools
	if closed.Calls != 2 || closed.Failed != 1 {
		t.Fatalf("the batch the turn end closed = %+v, want 2 calls 1 failed", closed)
	}

	// turn_start flushes ahead of the reply it settles.
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, finishedAt(3, 2100, 2200, 1, 0, "Bash"))}, 2300)
	mustApply(t, st, Event{Kind: KindReply, Text: "left open"}, 2400)
	res = mustApply(t, st, Event{Kind: KindTurnStart, At: 3000}, 3000)
	if !reflect.DeepEqual(kinds(res.Ring), []string{RingTools, RingReply}) {
		t.Fatalf("turn start ring = %v", kinds(res.Ring))
	}
}

// TestToolsRowIsTheLeadsAndCountsEachBatchOnce pins that subagent batches
// never enter the lead's gap, a batch repeated in later lists counts once,
// and the open gap shows in the snapshot as it grows.
func TestToolsRowIsTheLeadsAndCountsEachBatchOnce(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindAgent, AgentID: "sub", Status: StatusRunning, Batches: batchesJSON(t, finished(1, 9, 0, "Grep"))}, 10)
	mustApply(t, st, Event{Kind: KindAgent, AgentID: "sub", Status: StatusDone}, 11)
	if st.Snapshot(12).Lead.ToolGap != nil {
		t.Fatal("a subagent's batch entered the lead's gap")
	}

	b1 := finishedAt(1, 100, 200, 1, 0, "Bash")
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, b1)}, 20)
	if gap := st.Snapshot(21).Lead.ToolGap; gap == nil || gap.Calls != 1 {
		t.Fatalf("gap after one batch = %+v", gap)
	}
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, b1, finishedAt(2, 250, 400, 3, 0, "Bash"))}, 30)
	gap := st.Snapshot(31).Lead.ToolGap
	if gap == nil || gap.Calls != 4 || gap.DurationMs != 300 || len(gap.Batches) != 2 || gap.Tools[0] != (ToolTally{Name: "Bash", Calls: 4}) {
		t.Fatalf("gap after a repeated list = %+v", gap)
	}
	// The snapshot copy is the caller's: reading it never empties the gap.
	gap.Calls = 99
	if st.Snapshot(32).Lead.ToolGap.Calls != 4 {
		t.Fatal("the snapshot shares the gap's state")
	}
	res := mustApply(t, st, Event{Kind: KindUser, Text: "x"}, 40)
	if res.Ring[0].Tools.Calls != 4 {
		t.Fatalf("flushed %+v", res.Ring[0].Tools)
	}
}

// TestToolsRowCountsCallsPerTool pins the attribution a distinct-name batch
// allows: all calls to the one tool of a single-tool batch, one call each
// plus an unattributed remainder otherwise, most calls first.
func TestToolsRowCountsCallsPerTool(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t,
		finishedAt(1, 0, 10, 4, 0, "Bash"),
		finishedAt(2, 10, 20, 5, 1, "Read", "Bash"),
		finishedAt(3, 20, 30, 2, 0),
	)}, 40)
	got := st.Snapshot(41).Lead.ToolGap
	want := &ToolSummary{
		Calls: 11, Failed: 1, DurationMs: 30,
		Tools:      []ToolTally{{Name: "Bash", Calls: 5}, {Name: "Read", Calls: 1}},
		OtherCalls: 5,
		Batches: []ToolBatch{
			{Names: []string{"Bash"}, Calls: 4, DurationMs: 10},
			{Names: []string{"Read", "Bash"}, Calls: 5, Failed: 1, DurationMs: 10},
			{Calls: 2, DurationMs: 10},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary = %+v\nwant     %+v", got, want)
	}
	total := got.OtherCalls
	for _, tl := range got.Tools {
		total += tl.Calls
	}
	if total != got.Calls {
		t.Fatalf("tallies sum to %d, calls %d", total, got.Calls)
	}
}

// TestToolsRowIsBounded pins the caps: tool kinds past ToolSummaryKinds fold
// into the unattributed count, batches past ToolSummaryBatches are only
// counted, and the call total stays exact.
func TestToolsRowIsBounded(t *testing.T) {
	st := NewState(ownerKey)
	total := 0
	for i := 1; i <= ToolSummaryBatches+10; i++ {
		var batches []map[string]any
		batches = append(batches, finishedAt(int64(i), int64(i)*10, int64(i)*10+5, 1, 0, fmt.Sprintf("tool-%d", i)))
		total++
		mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, batches...)}, int64(i)*10)
	}
	gap := st.Snapshot(1_000).Lead.ToolGap
	if len(gap.Tools) != ToolSummaryKinds || len(gap.Batches) != ToolSummaryBatches || gap.BatchesOmitted != 10 {
		t.Fatalf("tools %d batches %d omitted %d", len(gap.Tools), len(gap.Batches), gap.BatchesOmitted)
	}
	if gap.Calls != total || gap.OtherCalls != total-ToolSummaryKinds {
		t.Fatalf("calls %d other %d, want %d and %d", gap.Calls, gap.OtherCalls, total, total-ToolSummaryKinds)
	}
	if st.gap.counts == nil || len(st.gap.counts) > ToolSummaryKinds {
		t.Fatalf("the accumulator holds %d kinds", len(st.gap.counts))
	}
}

// TestToolsEntryRoundTripsThroughTheRing pins the entry's file form.
func TestToolsEntryRoundTripsThroughTheRing(t *testing.T) {
	dir := t.TempDir()
	entry := RingEntry{Kind: RingTools, Tools: &ToolSummary{Calls: 3, Failed: 1, DurationMs: 5, Tools: []ToolTally{{Name: "Bash", Calls: 3}}, Batches: []ToolBatch{{Names: []string{"Bash"}, Calls: 3, Failed: 1}}}}
	if err := AppendRing(dir, []RingEntry{entry}); err != nil {
		t.Fatal(err)
	}
	recs, err := ReadRing(dir)
	if err != nil || len(recs) != 1 || !reflect.DeepEqual(recs[0].RingEntry, entry) {
		t.Fatalf("read %+v err %v", recs, err)
	}
}

func withCalls(b map[string]any, calls ...map[string]any) map[string]any {
	b["calls"] = calls
	return b
}

func call(name string, start, end int64, failed bool) map[string]any {
	return map[string]any{"name": name, "startedAt": start, "endedAt": end, "failed": failed}
}

// TestToolsRowIsExactWithPerCallData pins the exact path: a batch carrying
// its calls credits each call to its own tool (overall and per batch, in
// call order), keeps its duration and failed count, and its request's
// tokens add to the gap's totals. A batch without calls beside it still
// takes the approximate path.
func TestToolsRowIsExactWithPerCallData(t *testing.T) {
	st := NewState(ownerKey)
	b1 := withCalls(finishedAt(1, 100, 1300, 4, 1, "Read", "Bash"),
		call("Read", 110, 200, false), call("Bash", 110, 1300, true), call("Read", 120, 210, false), call("Read", 130, 220, false))
	b1["usage"] = map[string]any{"input": 12, "output": 340, "cacheRead": 45_000, "cacheWrite": 1_200}
	b2 := withCalls(finishedAt(2, 1400, 1500, 1, 0, "Grep"), call("Grep", 1400, 1500, false))
	b2["usage"] = map[string]any{"input": 3, "output": 60, "cacheRead": 46_000}
	b3 := finishedAt(3, 1600, 1700, 3, 0, "Edit", "Bash") // an older adapter's batch: no calls, no usage
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, b1, b2, b3)}, 2000)

	got := st.Snapshot(2001).Lead.ToolGap
	want := &ToolSummary{
		Calls: 8, Failed: 1, DurationMs: 1600,
		Tools:      []ToolTally{{Name: "Read", Calls: 3}, {Name: "Bash", Calls: 2}, {Name: "Edit", Calls: 1}, {Name: "Grep", Calls: 1}},
		OtherCalls: 1,
		Batches: []ToolBatch{
			{Names: []string{"Read", "Bash"}, Tools: []ToolTally{{Name: "Read", Calls: 3}, {Name: "Bash", Calls: 1}}, Calls: 4, Failed: 1, DurationMs: 1200,
				Usage: &TokenUsage{Input: 12, Output: 340, CacheRead: 45_000, CacheWrite: 1_200}},
			{Names: []string{"Grep"}, Tools: []ToolTally{{Name: "Grep", Calls: 1}}, Calls: 1, DurationMs: 100, Usage: &TokenUsage{Input: 3, Output: 60, CacheRead: 46_000}},
			{Names: []string{"Edit", "Bash"}, Calls: 3, DurationMs: 100},
		},
		Usage: &TokenUsage{Input: 15, Output: 400, CacheRead: 91_000, CacheWrite: 1_200},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary = %+v\nwant      %+v", got, want)
	}
	// The ring entry is the same summary, and it round-trips through a file.
	res := mustApply(t, st, Event{Kind: KindReply, Final: true, Text: "done"}, 2100)
	if !reflect.DeepEqual(res.Ring[0].Tools, want) {
		t.Fatalf("ring summary = %+v", res.Ring[0].Tools)
	}
	dir := t.TempDir()
	if err := AppendRing(dir, res.Ring[:1]); err != nil {
		t.Fatal(err)
	}
	if recs, err := ReadRing(dir); err != nil || !reflect.DeepEqual(recs[0].Tools, want) {
		t.Fatalf("read back %+v err %v", recs, err)
	}
	// The snapshot's batch list never carries the per-call data.
	raw, err := json.Marshal(st.Snapshot(2200))
	if err != nil || strings.Contains(string(raw), `"calls"`) || strings.Contains(string(raw), "cacheRead") {
		t.Fatalf("snapshot carries per-call data: %s", raw)
	}
}

// TestBatchCallsAndUsageAreBounded pins normalization of the new fields: at
// most BatchCallCap calls (never more than callCount), a malformed call or
// usage is dropped without the batch, names are scrubbed of session keys and
// capped, and negative token counts read 0.
func TestBatchCallsAndUsageAreBounded(t *testing.T) {
	st := NewState(ownerKey)
	var many []map[string]any
	for i := 0; i < BatchCallCap+8; i++ {
		many = append(many, call("Read", 1, 2, false))
	}
	big := withCalls(finishedAt(1, 0, 10, BatchCallCap+8, 0, "Read"), many...)
	short := withCalls(finishedAt(2, 10, 20, 1, 0, "Bash"), call("Bash", 10, 20, false), call("Bash", 10, 20, false))
	odd := finishedAt(3, 20, 30, 3, 0, "Bash")
	odd["calls"] = []any{"not a call", map[string]any{"name": "  "}, map[string]any{"name": "run " + ownerKey + " " + strings.Repeat("x", 200), "startedAt": 25, "endedAt": 21}}
	odd["usage"] = map[string]any{"input": -5, "output": 7}
	bad := withCalls(finishedAt(4, 30, 40, 1, 0, "Grep"), call("Grep", 30, 40, false))
	bad["usage"] = "lots"
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, big, short, odd, bad)}, 100)

	batches := st.lead.batches
	if len(batches) != 4 {
		t.Fatalf("kept %d batches, want all 4 (a malformed new field drops only itself)", len(batches))
	}
	if n := len(batches[0].Calls); n != BatchCallCap {
		t.Fatalf("calls kept %d, want %d", n, BatchCallCap)
	}
	if n := len(batches[1].Calls); n != 1 {
		t.Fatalf("calls past callCount kept: %d", n)
	}
	c := batches[2].Calls
	if len(c) != 1 || strings.Contains(c[0].Name, ownerKey) || len([]rune(c[0].Name)) > ToolNameRunes || c[0].StartedAt != 25 || c[0].EndedAt != 0 {
		t.Fatalf("odd calls = %+v", c)
	}
	if u := batches[2].Usage; u == nil || *u != (TokenUsage{Output: 7}) {
		t.Fatalf("odd usage = %+v", u)
	}
	if batches[3].Usage != nil || len(batches[3].Calls) != 1 {
		t.Fatalf("malformed usage kept or calls lost: %+v", batches[3])
	}
	gap := st.Snapshot(101).Lead.ToolGap
	if gap.Calls != BatchCallCap+8+1+3+1 || gap.OtherCalls != 8+2 {
		t.Fatalf("gap calls %d other %d", gap.Calls, gap.OtherCalls)
	}
}

// TestFinishedBatchesPastTheWindowCountOnce pins the batch-loss fix: an
// adapter lists every finished batch no delivered call carried yet, beyond
// the newest BatchCap, and each is counted exactly once whether it arrives
// in one list, again in a resent list, or after it left the list.
func TestFinishedBatchesPastTheWindowCountOnce(t *testing.T) {
	st := NewState(ownerKey)
	mustApply(t, st, Event{Kind: KindTurnStart, At: 1}, 1)
	var list []map[string]any
	for i := int64(1); i <= 8; i++ {
		list = append(list, withCalls(finishedAt(i, i*10, i*10+5, 1, 0, "Bash"), call("Bash", i*10, i*10+5, false)))
	}
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, list...)}, 100)
	snap := st.Snapshot(101)
	if len(snap.Lead.Batches) != BatchCap || snap.Lead.Batches[0].Seq != 4 {
		t.Fatalf("kept batches %+v, want the newest %d", snap.Lead.Batches, BatchCap)
	}
	if snap.Lead.ToolGap.Calls != 8 || snap.Lead.Done != 8 {
		t.Fatalf("gap calls %d done %d, want 8 and 8", snap.Lead.ToolGap.Calls, snap.Lead.Done)
	}
	// A call that was delivered but whose answer was lost: the same list again.
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, list...)}, 110)
	// The adapter learned of delivery: only the window, plus a new batch.
	next := append(append([]map[string]any(nil), list[4:]...), withCalls(finishedAt(9, 90, 95, 2, 0, "Read"), call("Read", 90, 95, false), call("Read", 90, 95, false)))
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, next...)}, 120)
	snap = st.Snapshot(121)
	if snap.Lead.ToolGap.Calls != 10 || snap.Lead.Done != 10 || len(snap.Lead.ToolGap.Batches) != 9 {
		t.Fatalf("after resend: gap calls %d done %d batches %d, want 10, 10, 9", snap.Lead.ToolGap.Calls, snap.Lead.Done, len(snap.Lead.ToolGap.Batches))
	}
	// The wire cap keeps the newest BatchWireCap of an oversized list.
	var huge []map[string]any
	for i := int64(100); i < 100+BatchWireCap+5; i++ {
		huge = append(huge, finishedAt(i, i, i+1, 1, 0, "Grep"))
	}
	mustApply(t, st, Event{Kind: KindAgent, Batches: batchesJSON(t, huge...)}, 130)
	if got := st.Snapshot(131).Lead.ToolGap.Calls; got != 10+BatchWireCap {
		t.Fatalf("gap calls after an oversized list = %d, want %d", got, 10+BatchWireCap)
	}
}
