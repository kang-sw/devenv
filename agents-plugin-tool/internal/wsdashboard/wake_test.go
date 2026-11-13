package wsdashboard

import (
	"github.com/kang-sw/devenv/internal/wslive"
	"strings"
	"testing"
)

func TestWakeEntryMarkup(t *testing.T) {
	ring := []wslive.RingRecord{
		ringRecord(1, wslive.RingEntry{Kind: wslive.RingWake, Cause: "background_exit", Source: "shell", Caption: "exited"}),
		ringRecord(2, wslive.RingEntry{Kind: wslive.RingWake, Cause: "subagent_report", Source: "reviewer", Caption: "found <issue>", Text: "full <script>not markup</script>"}),
		ringRecord(3, wslive.RingEntry{Kind: wslive.RingTurn, WakeTurn: true, Done: 4, DurationMs: 1200}),
	}
	set, err := renderSessionRegions(historyView(ring, "", PageContent{}, sessionLookup{}), newEntryCache())
	if err != nil {
		t.Fatal(err)
	}
	html := string(set[regionEntries].HTML)
	for _, want := range []string{`class="wake-divider"`, `class="wake-card"`, `class="wake-caption"`, `class="wake-body"`, `data-key="wake-`, `class="turnline wake-turn"`, `↳ lead turn · 4 tool calls`, `full &lt;script&gt;not markup&lt;/script&gt;`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s: %s", want, html)
		}
	}
}
