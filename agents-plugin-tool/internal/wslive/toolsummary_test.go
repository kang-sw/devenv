package wslive

import (
	"fmt"
	"reflect"
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
	want := &ToolSummary{Calls: 2, DurationMs: 100, Tools: []ToolTally{{Name: "Bash", Calls: 2}}, Batches: []ToolBatch{{Names: []string{"Bash"}, Calls: 2}}}
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
			{Names: []string{"Bash"}, Calls: 4},
			{Names: []string{"Read", "Bash"}, Calls: 5, Failed: 1},
			{Calls: 2},
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
