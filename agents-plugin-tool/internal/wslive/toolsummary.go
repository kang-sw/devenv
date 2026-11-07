package wslive

import "sort"

// The lead's tool-usage summary: the finished tool batches between two
// conversation entries, folded into one row. The state accumulates them as
// the batches finish (toolGap), shows the open gap in the snapshot
// (Lead.ToolGap), and writes it to the ring as one RingTools entry just
// before the next conversation entry. Only what the daemon already receives
// is used (BatchView), so no adapter changes and no argument previews.
//
// A BatchView lists distinct tool names, not one per call, so a batch of
// several calls to several tools cannot say how many calls each tool made.
// A batch whose calls share one tool credits all of them to it; otherwise
// each listed tool is credited one call and the rest of the batch's calls
// count as OtherCalls. The call total stays exact either way.

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

// ToolBatch is one finished batch in a ToolSummary.
type ToolBatch struct {
	Names  []string `json:"names,omitempty"`
	Calls  int      `json:"calls"`
	Failed int      `json:"failed,omitempty"`
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
	credited := 0
	for _, name := range b.ToolNames {
		n := 1
		if len(b.ToolNames) == 1 {
			n = b.CallCount
		}
		if _, ok := g.counts[name]; !ok && len(g.counts) >= ToolSummaryKinds {
			continue
		}
		g.counts[name] += n
		credited += n
	}
	g.otherCalls += max(b.CallCount-credited, 0)
	if len(g.batches) >= ToolSummaryBatches {
		g.batchesOmitted++
		return
	}
	g.batches = append(g.batches, ToolBatch{Names: append([]string(nil), b.ToolNames...), Calls: b.CallCount, Failed: b.Failed})
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
		Batches:        append([]ToolBatch(nil), g.batches...),
		BatchesOmitted: g.batchesOmitted,
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

// flush returns the gap's ring entry, nil when it is empty, and empties it.
func (g *toolGap) flush() *RingEntry {
	s := g.summary()
	*g = toolGap{}
	if s == nil {
		return nil
	}
	return &RingEntry{Kind: RingTools, Tools: s}
}
