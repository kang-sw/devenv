package wsdashboard

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wslive"
)

// SummaryView contains only validated provider summaries, never reply markdown.
// Key survives live revisions or retained paging so keyed details keep user state.
type SummaryView struct {
	Key                        string
	Live, Streaming, Truncated bool
	Requests                   []wslive.SummaryAttempt
}

func summaryHasText(requests []wslive.SummaryAttempt) bool {
	for _, a := range requests {
		for _, p := range a.Parts {
			if p.Text != "" {
				return true
			}
		}
	}
	return false
}

func liveSummary(s *wslive.ReasoningSummary) *SummaryView {
	if wslive.ValidateReasoningSummary(s) != nil || !summaryHasText(s.Requests) {
		return nil
	}
	streaming := false
	for _, a := range s.Requests {
		streaming = streaming || a.Status == wslive.SummaryStreaming
	}
	return &SummaryView{Key: fmt.Sprintf("summary-live-%s-%d", s.Producer, s.Turn), Live: true, Streaming: streaming, Truncated: s.Truncated, Requests: s.Requests}
}

// Summary belongs to this immutable natural turn, not an earlier cached reply.
// History has no aggregate clipping flag: the UI always qualifies retained text
// as observed prefixes and never infers that the entire summary was retained.
func (e ConvEntry) Summary() *SummaryView {
	if e.Kind != wslive.RingTurn || wslive.ValidateSummaryHistory(e.SummaryHistory) != nil || !summaryHasText(e.SummaryHistory.Requests) {
		return nil
	}
	return &SummaryView{Key: "summary-turn-" + e.Key, Requests: e.SummaryHistory.Requests}
}
