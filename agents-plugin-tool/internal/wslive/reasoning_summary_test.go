package wslive

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func summaryFixture() *ReasoningSummary {
	return &ReasoningSummary{V: 1, Producer: "11111111-1111-4111-8111-111111111111", Turn: 1, Revision: 1,
		ActiveRequest: &SummaryPair{Request: 1, Attempt: 1}, Requests: []SummaryAttempt{{Request: 1, Attempt: 1,
			Provider: "openai", API: "openai-responses", Model: "model", Status: SummaryStreaming,
			Parts: []SummaryPart{{Output: 0, Part: 0, Text: "é", ObservedBytes: 2, RetainedBytes: 2, Status: SummaryStreaming}}}}}
}

func TestReasoningSummaryValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ReasoningSummary)
	}{
		{"unsafe request", func(s *ReasoningSummary) { s.Requests[0].Request = 1 << 53 }},
		{"missing attempt", func(s *ReasoningSummary) { s.Requests[0].Attempt = 0 }},
		{"dangling active attempt", func(s *ReasoningSummary) { s.ActiveRequest.Attempt = 2 }},
		{"private provider", func(s *ReasoningSummary) { s.Requests[0].API = "openai-completions" }},
		{"false byte count", func(s *ReasoningSummary) { s.Requests[0].Parts[0].RetainedBytes = 1 }},
		{"false prefix", func(s *ReasoningSummary) { s.Requests[0].Parts[0].ObservedBytes = 3 }},
		{"unknown status", func(s *ReasoningSummary) { s.Requests[0].Status = "guessed" }},
		{"duplicate part", func(s *ReasoningSummary) { s.Requests[0].Parts = append(s.Requests[0].Parts, s.Requests[0].Parts[0]) }},
		{"aggregate attempts", func(s *ReasoningSummary) {
			s.ActiveRequest = nil
			for i := 2; i <= 17; i++ {
				a := s.Requests[0]
				a.Attempt = int64(i)
				s.Requests = append(s.Requests, a)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := summaryFixture()
			tc.change(s)
			if err := ValidateReasoningSummary(s); err == nil {
				t.Fatal("accepted malformed projection")
			}
		})
	}
	if err := ValidateReasoningSummary(summaryFixture()); err != nil {
		t.Fatal(err)
	}
}

func TestReasoningSummaryFreshnessAndRealEnd(t *testing.T) {
	st := NewState("key")
	s := summaryFixture()
	if _, err := st.Apply(Event{Kind: KindAgent, ReasoningSummary: s}, 100); err == nil {
		t.Fatal("projection without real turn accepted")
	}
	_, _ = st.Apply(Event{Kind: KindTurnStart, Origin: OriginTerminal}, 100)
	_, err := st.Apply(Event{Kind: KindAgent, ReasoningSummary: s}, 110)
	if err != nil {
		t.Fatal(err)
	}
	s.Requests[0].Parts[0].Text = "mutated caller"
	if got := st.Snapshot(111).Lead.ReasoningSummary.Requests[0].Parts[0].Text; got != "é" {
		t.Fatalf("not frozen: %q", got)
	}
	s = summaryFixture()
	if _, err = st.Apply(Event{Kind: KindAgent, ReasoningSummary: s}, 112); err != nil {
		t.Fatal("equal identical replay refused", err)
	}
	conflict := summaryFixture()
	conflict.Requests[0].Model = "other"
	if _, err = st.Apply(Event{Kind: KindAgent, ReasoningSummary: conflict}, 113); err == nil {
		t.Fatal("conflicting revision accepted")
	}
	final := summaryFixture()
	final.Revision = 2
	final.ActiveRequest = nil
	final.Requests[0].Status = SummaryFailed
	final.Requests[0].Parts[0].Status = SummaryFailed
	res, err := st.Apply(Event{Kind: KindTurnEnd, Reason: "error", ReasoningSummary: final}, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ring) != 1 || res.Ring[0].Kind != RingTurn || res.Ring[0].DurationMs != 100 || res.Ring[0].Reason != "error" || res.Ring[0].SummaryHistory == nil {
		t.Fatalf("real end: %+v", res)
	}
	h := res.Ring[0].SummaryHistory
	if h.Commit == "" || !reflect.DeepEqual(h.Requests, final.Requests) {
		t.Fatalf("history: %+v", h)
	}
	_, _ = st.Apply(Event{Kind: KindReset}, 201)
	_, _ = st.Apply(Event{Kind: KindTurnStart}, 202)
	if _, err = st.Apply(Event{Kind: KindAgent, ReasoningSummary: final}, 203); err == nil {
		t.Fatal("reset resurrected closed source turn")
	}
	if res, err = st.Apply(Event{Kind: KindTurnEnd, ReasoningSummary: final}, 204); err == nil || len(res.Ring) != 0 {
		t.Fatal("invalid final mutated real turn")
	}
	if st.Snapshot(205).Lead.TurnStartedAt != 202 {
		t.Fatal("invalid final ended turn")
	}
}

func TestReasoningSummaryLeadOnlyAndAttemptPreservation(t *testing.T) {
	st := NewState("key")
	_, _ = st.Apply(Event{Kind: KindTurnStart}, 1)
	s := summaryFixture()
	if _, err := st.Apply(Event{Kind: KindAgent, AgentID: "child", ReasoningSummary: s}, 2); err == nil {
		t.Fatal("subagent summary accepted")
	}
	failed := s.Requests[0]
	failed.Status = SummaryFailed
	failed.Parts = append([]SummaryPart(nil), failed.Parts...)
	failed.Parts[0].Status = SummaryFailed
	s.Requests[0] = failed
	next := failed
	next.Attempt = 2
	next.Status = SummaryComplete
	next.Parts = append([]SummaryPart(nil), next.Parts...)
	next.Parts[0].Text = "ok"
	next.Parts[0].Status = SummaryComplete
	s.Requests = append(s.Requests, next)
	s.ActiveRequest = nil
	res, err := st.Apply(Event{Kind: KindTurnEnd, ReasoningSummary: s}, 3)
	if err != nil {
		t.Fatal(err)
	}
	got := res.Ring[0].SummaryHistory.Requests
	if len(got) != 2 || got[0].Attempt != 1 || got[1].Attempt != 2 || got[0].Parts[0].Text != "é" || got[0].Status != SummaryFailed {
		t.Fatalf("attempts joined: %+v", got)
	}
	raw, _ := json.Marshal(res.Ring[0])
	if strings.Contains(string(raw), "response_id") {
		t.Fatal("internal response identity escaped")
	}
}
