package wsdashboard

import (
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func toolsSummary() *wslive.ToolSummary {
	return &wslive.ToolSummary{
		Calls: 11, Failed: 1, DurationMs: 75_000,
		Tools:      []wslive.ToolTally{{Name: "Bash", Calls: 5}, {Name: "mcp__plugin_ws_ws__whiteboard.update", Calls: 2}},
		OtherCalls: 4,
		Batches: []wslive.ToolBatch{
			{Names: []string{"Bash"}, Calls: 5},
			{Names: []string{"Read", "mcp__plugin_ws_ws__whiteboard.update"}, Calls: 6, Failed: 1},
		},
		BatchesOmitted: 3,
	}
}

// TestToolsEntryRendersACollapsedSummaryRow pins the tools ring entry: one
// collapsed details row whose summary carries the per-tool counts, failed
// count, and elapsed time, and whose body lists each batch with its tool
// names. The open state is kept across region swaps by data-key.
func TestToolsEntryRendersACollapsedSummaryRow(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingUser, Origin: wslive.OriginTerminal, Text: "go"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingTools, Tools: toolsSummary()}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingReply, Text: "done"}),
	}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, nil), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	name := ring[1].Name
	for _, want := range []string{
		`<details class="toolsline" data-entry="` + name + `" data-key="tools-` + name + `">`,
		`Bash</span> 5`,
		`whiteboard.update</span> 2`,
		` · +4 other`,
		`1 failed`,
		`1m 15s`,
		`<li class="brow done">`,
		`<li class="brow fail">`,
		`6 calls · 1 failed`,
		`+3 more batches`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("tools row lacks %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "<details class=\"toolsline\" open") || strings.Contains(html, "mcp__plugin_ws_ws__whiteboard.update</span>") {
		t.Errorf("tools row is open by default or shows an unstripped tool name:\n%s", html)
	}
	if strings.Index(html, `data-entry="`+ring[0].Name) > strings.Index(html, `data-entry="`+name) || strings.Index(html, `data-entry="`+name) > strings.Index(html, `data-entry="`+ring[2].Name) {
		t.Errorf("tools row is not between the user message and the reply:\n%s", html)
	}
}

// TestToolsRowWithoutNamedToolsShowsTheCallTotal pins the summary when no
// listed name accounts for any call.
func TestToolsRowWithoutNamedToolsShowsTheCallTotal(t *testing.T) {
	ring := []wslive.RingRecord{ringRecord(1, wslive.RingEntry{Kind: wslive.RingTools, Tools: &wslive.ToolSummary{Calls: 2, OtherCalls: 2, Batches: []wslive.ToolBatch{{Calls: 2}}}})}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, nil), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	if html := string(set[regionEntries].HTML); !strings.Contains(html, `<span class="tsum">2 tool calls</span>`) {
		t.Errorf("summary = %s", html)
	}
}

// TestOpenGapShowsALiveToolsRow pins the live row: the snapshot's open tool
// gap renders at the head of the convlive region, so progress shows during a
// long turn, and a snapshot without one renders no row.
func TestOpenGapShowsALiveToolsRow(t *testing.T) {
	view := func(gap *wslive.ToolSummary) string {
		lead := wslive.Lead{Activity: wslive.Activity{Phase: wslive.PhaseTool}, ToolGap: gap}
		v := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, lead), Now: testNow})
		set, err := renderSessionRegions(v, newEntryCache())
		if err != nil {
			t.Fatal(err)
		}
		return string(set[regionConvLive].HTML)
	}
	live := view(toolsSummary())
	for _, want := range []string{`<details class="toolsline live" data-key="tools-live">`, `Bash</span> 5`, `<li class="brow fail">`} {
		if !strings.Contains(live, want) {
			t.Errorf("live row lacks %q:\n%s", want, live)
		}
	}
	if strings.Contains(live, `data-entry=`) {
		t.Errorf("the live row carries a ring entry key:\n%s", live)
	}
	if none := view(nil); strings.Contains(none, "toolsline") {
		t.Errorf("a snapshot without an open gap rendered a tools row:\n%s", none)
	}
}
