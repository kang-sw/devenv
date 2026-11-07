package wslive

import (
	"slices"
	"sort"
)

// The lead's tool-usage summary: the finished tool batches between two
// conversation entries, folded into one row. The state accumulates them as
// the batches finish (toolGap), shows the open gap in the snapshot
// (Lead.ToolGap), and writes it to the ring as one RingTools entry just
// before the next conversation entry. No argument or result content enters
// it: only tool names, counts, clocks, and token counts.
//
// A batch that carries its calls (BatchView.Calls) credits each call to its
// own tool, so the per-tool counts are exact; calls past BatchCallCap count
// as OtherCalls. A batch without them (an older adapter) lists only distinct
// tool names: a batch whose calls share one tool credits all of them to it;
// otherwise each listed tool is credited one call and the rest of the
// batch's calls count as OtherCalls. The call total stays exact either way.
// A batch's usage adds to the gap's token totals; a gap none of whose
// batches carried any shows none.

const (
	// ToolSummaryKinds bounds the distinct tool names one summary tallies;
	// calls of any further tool count as OtherCalls.
	ToolSummaryKinds = 16
	// ToolSummaryBatches bounds the per-batch list one summary keeps; the
	// batches beyond it are only counted (BatchesOmitted).
	ToolSummaryBatches = 40
)

// ToolTally is one tool's call count in a ToolSummary.
type ToolTally struct {
	Name  string `json:"name"`
	Calls int    `json:"calls"`
}

// ToolBatch is one finished batch in a ToolSummary: one model request and
// the tool calls it made.
type ToolBatch struct {
	// Names are the batch's distinct tool names; Tools, when the batch
	// carried its calls, each name's exact call count in call order.
	Names  []string    `json:"names,omitempty"`
	Tools  []ToolTally `json:"tools,omitempty"`
	Calls  int         `json:"calls"`
	Failed int         `json:"failed,omitempty"`
	// DurationMs runs from the batch's start to its end.
	DurationMs int64 `json:"durationMs,omitempty"`
	// Usage is the request's tokens, when the adapter reported them.
	Usage *TokenUsage `json:"usage,omitempty"`
}

// ToolSummary is the lead's finished tool batches over one gap.
type ToolSummary struct {
	// Calls is every call in the gap, Failed those that failed.
	Calls  int `json:"calls"`
	Failed int `json:"failed,omitempty"`
	// DurationMs runs from the first batch's start to the last one's end.
	DurationMs int64 `json:"durationMs,omitempty"`
	// Tools are the per-tool call counts, most calls first; OtherCalls are
	// the calls no listed name accounts for.
	Tools      []ToolTally `json:"tools,omitempty"`
	OtherCalls int         `json:"otherCalls,omitempty"`
	// Batches lists the gap's batches in order, up to ToolSummaryBatches;
	// BatchesOmitted counts the later ones left out.
	Batches        []ToolBatch `json:"batches,omitempty"`
	BatchesOmitted int         `json:"batchesOmitted,omitempty"`
	// Usage sums the tokens of the gap's batches that reported them, the
	// omitted ones included; absent when none did.
	Usage *TokenUsage `json:"usage,omitempty"`
}

// toolGap accumulates the lead's finished batches since the last
// conversation entry. Its size is bounded whatever the turn does.
type toolGap struct {
	counts         map[string]int
	otherCalls     int
	calls, failed  int
	startedAt      int64
	endedAt        int64
	batches        []ToolBatch
	batchesOmitted int
	usage          TokenUsage
	hasUsage       bool
	any            bool
}

func (g *toolGap) add(b BatchView) {
	if g.counts == nil {
		g.counts = map[string]int{}
	}
	if !g.any || b.StartedAt < g.startedAt {
		g.startedAt = b.StartedAt
	}
	g.endedAt = max(g.endedAt, b.EndedAt)
	g.any = true
	g.calls += b.CallCount
	g.failed += b.Failed
	if b.Usage != nil {
		g.usage.add(*b.Usage)
		g.hasUsage = true
	}
	credited := 0
	credit := func(name string, n int) {
		if _, ok := g.counts[name]; !ok && len(g.counts) >= ToolSummaryKinds {
			return
		}
		g.counts[name] += n
		credited += n
	}
	var tallies []ToolTally
	if len(b.Calls) > 0 {
		for _, c := range b.Calls {
			credit(c.Name, 1)
			i := slices.IndexFunc(tallies, func(t ToolTally) bool { return t.Name == c.Name })
			if i < 0 {
				tallies = append(tallies, ToolTally{Name: c.Name})
				i = len(tallies) - 1
			}
			tallies[i].Calls++
		}
	} else {
		for _, name := range b.ToolNames {
			n := 1
			if len(b.ToolNames) == 1 {
				n = b.CallCount
			}
			credit(name, n)
		}
	}
	g.otherCalls += max(b.CallCount-credited, 0)
	if len(g.batches) >= ToolSummaryBatches {
		g.batchesOmitted++
		return
	}
	tb := ToolBatch{Names: append([]string(nil), b.ToolNames...), Tools: tallies, Calls: b.CallCount, Failed: b.Failed, DurationMs: max(b.EndedAt-b.StartedAt, 0)}
	if b.Usage != nil {
		u := *b.Usage
		tb.Usage = &u
	}
	g.batches = append(g.batches, tb)
}

// summary is the gap as a ToolSummary, nil when no batch finished in it.
func (g *toolGap) summary() *ToolSummary {
	if !g.any {
		return nil
	}
	s := &ToolSummary{
		Calls:          g.calls,
		Failed:         g.failed,
		DurationMs:     max(g.endedAt-g.startedAt, 0),
		OtherCalls:     g.otherCalls,
		Batches:        cloneToolBatches(g.batches),
		BatchesOmitted: g.batchesOmitted,
	}
	if g.hasUsage {
		u := g.usage
		s.Usage = &u
	}
	for name, n := range g.counts {
		s.Tools = append(s.Tools, ToolTally{Name: name, Calls: n})
	}
	sort.Slice(s.Tools, func(i, j int) bool {
		if s.Tools[i].Calls != s.Tools[j].Calls {
			return s.Tools[i].Calls > s.Tools[j].Calls
		}
		return s.Tools[i].Name < s.Tools[j].Name
	})
	return s
}

// cloneToolBatches is a deep copy, so a snapshot never shares the gap's
// slices or pointers.
func cloneToolBatches(in []ToolBatch) []ToolBatch {
	if in == nil {
		return nil
	}
	out := make([]ToolBatch, len(in))
	for i, b := range in {
		b.Names = append([]string(nil), b.Names...)
		b.Tools = append([]ToolTally(nil), b.Tools...)
		if b.Usage != nil {
			u := *b.Usage
			b.Usage = &u
		}
		out[i] = b
	}
	return out
}

// flush returns the gap's ring entry, nil when it is empty, and empties it.
func (g *toolGap) flush() *RingEntry {
	s := g.summary()
	*g = toolGap{}
	if s == nil {
		return nil
	}
	return &RingEntry{Kind: RingTools, Tools: s}
}
