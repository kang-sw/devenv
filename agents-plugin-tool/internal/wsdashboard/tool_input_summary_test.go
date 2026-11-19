package wsdashboard

import (
	"encoding/json"
	stdhtml "html"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wslive"
)

func TestToolRowsPreferInputSummary(t *testing.T) {
	for _, key := range wslive.ToolSummaryKeys {
		t.Run(key, func(t *testing.T) {
			const summary = "Explain <safe> operation"
			input, _ := json.Marshal(map[string]any{key: summary, "query": "not-the-row-summary"})
			batch, _ := json.Marshal(map[string]any{
				"epoch": "summary1", "seq": 1, "toolNames": []string{"mcp_ws_runtime_read"},
				"callCount": 1, "startedAt": 100, "endedAt": 200,
				"calls": []map[string]any{{"name": "mcp_ws_runtime_read", "startedAt": 100, "endedAt": 200, "argsPreview": string(input)}},
			})
			st := wslive.NewState("example-session-key")
			if _, err := st.Apply(wslive.Event{Kind: wslive.KindAgent, Batches: []json.RawMessage{batch}}, 300); err != nil {
				t.Fatal(err)
			}
			lead := st.Snapshot(301).Lead
			lead.Phase = wslive.PhaseTool
			live := buildSessionView(pageInputs{Session: liveSession("self", "/proj", testNow), Snapshot: liveSnap(testNow, lead), Now: testNow})
			liveRegions, err := renderSessionRegions(live, newEntryCache())
			if err != nil {
				t.Fatal(err)
			}
			res, err := st.Apply(wslive.Event{Kind: wslive.KindReply, Final: true, Text: "done"}, 400)
			if err != nil || len(res.Ring) < 1 || res.Ring[0].Kind != wslive.RingTools {
				t.Fatalf("summary did not reach the ring: %+v, %v", res, err)
			}
			history := historyView([]wslive.RingRecord{ringRecord(1, res.Ring[0])}, "", PageContent{}, sessionLookup{})
			historyRegions, err := renderSessionRegions(history, newEntryCache())
			if err != nil {
				t.Fatal(err)
			}
			for source, html := range map[string]string{"live": string(liveRegions[regionConvLive].HTML), "history": string(historyRegions[regionEntries].HTML)} {
				escaped := stdhtml.EscapeString(summary)
				if !strings.Contains(html, `class="ba" title="`+escaped+`">`+escaped+`</span>`) || strings.Contains(html, "not-the-row-summary") || strings.Contains(html, "<safe>") {
					t.Fatalf("%s row must prefer and escape summary: %s", source, html)
				}
			}
		})
	}
}
