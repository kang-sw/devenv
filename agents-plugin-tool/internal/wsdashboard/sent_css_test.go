package wsdashboard

import (
	"strings"
	"testing"
)

// TestSentStateDimsAckAndCheckRows pins the sent and held dimming to the
// board's own row classes: ack rows (.arow) and check rows (.ckrow), whose
// sent-sent / sent-held classes applySent sets. .aline is the agents
// panel's row wrapper, never a board item, so a rule on it dims nothing.
func TestSentStateDimsAckAndCheckRows(t *testing.T) {
	css := mustAsset(t, "dashboard.css")
	for _, row := range []string{".arow", ".ckrow"} {
		for _, want := range []string{
			row + ".sent-sent .why, " + row + ".sent-sent input",
			row + ".sent-held .why, " + row + ".sent-held input",
			row + ".sent-sent .head .title",
			row + ".sent-held .head .title",
			row + ":is(.sent-sent, .sent-held):focus-within :is(.why, input, .head .title)",
		} {
			if !strings.Contains(css, want) {
				t.Errorf("dashboard.css lacks %q", want)
			}
		}
	}
	if strings.Contains(css, ".aline.sent-") || strings.Contains(css, ".aline:is(.sent-") {
		t.Error("dashboard.css dims the agents panel's .aline as if it were a board row")
	}
}
