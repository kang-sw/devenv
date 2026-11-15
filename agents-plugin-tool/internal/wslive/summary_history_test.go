package wslive

import (
	"strings"
	"testing"
)

func TestSummaryHistoryValidationUsesRetainedIdentityAndBounds(t *testing.T) {
	history := summaryTurnFixture().SummaryHistory
	if err := ValidateSummaryHistory(history); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SummaryHistory){
		func(h *SummaryHistory) { h.V = 2 },
		func(h *SummaryHistory) { h.Commit = "not-a-commit" },
		func(h *SummaryHistory) { h.Commit = strings.Repeat("z", 64) },
		func(h *SummaryHistory) { h.Requests[0].Attempt = 0 },
		func(h *SummaryHistory) { h.Requests[0].Parts[0].RetainedBytes++ },
		func(h *SummaryHistory) { h.Requests = append(h.Requests, h.Requests[0]) },
	} {
		h := summaryTurnFixture().SummaryHistory
		mutate(h)
		if err := ValidateSummaryHistory(h); err == nil {
			t.Fatal("invalid retained summary accepted")
		}
	}
	if err := ValidateSummaryHistory(nil); err == nil {
		t.Fatal("nil history accepted")
	}
	history.Requests[0].Truncated = true
	history.Requests[0].Parts[0].Truncated = true
	history.Requests[0].Parts[0].ObservedBytes++
	if err := ValidateSummaryHistory(history); err != nil {
		t.Fatalf("received truncated prefix rejected: %v", err)
	}
}
