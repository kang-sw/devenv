package wsagent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wslive"
)

func hostedSummaryFixture() *wslive.ReasoningSummary {
	return &wslive.ReasoningSummary{V: 1, Producer: "22222222-2222-4222-8222-222222222222", Turn: 1, Revision: 1, ActiveRequest: &wslive.SummaryPair{Request: 1, Attempt: 1}, Requests: []wslive.SummaryAttempt{{Request: 1, Attempt: 1, Provider: "openai-codex", API: "openai-codex-responses", Model: "model", Status: wslive.SummaryStreaming, Parts: []wslive.SummaryPart{{Text: "received prefix", ObservedBytes: 15, RetainedBytes: 15, Status: wslive.SummaryStreaming}}}}}
}

func hostedSummaryLine(t *testing.T, s *wslive.ReasoningSummary) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"kind": "agent", "reasoningSummary": s, "batches": []any{}, "sessionKey": "must-not-be-authority", "caption": "not copied"})
	if err != nil {
		t.Fatal(err)
	}
	return string(piWidgetLine(piEventsWidgetKey, string(raw)))
}

func hostedCapabilitiesLine(t *testing.T, capabilities any) string {
	t.Helper()
	status := map[string]any{"kind": "status", "background": 0, "monitors": 0}
	if capabilities != nil {
		status["capabilities"] = capabilities
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	return string(piWidgetLine(piEventsWidgetKey, string(raw)))
}

func TestSummaryHostedWidgetIsProjectionNotLifecycleAuthority(t *testing.T) {
	m := newPiMapper(StartSpec{Lead: &LeadSpec{}})
	if evs := m.mapLine([]byte(hostedSummaryLine(t, hostedSummaryFixture())), nopReplies{}); len(evs) != 0 {
		t.Fatal("summary payload inferred bundle capability")
	}
	m.mapLine([]byte(hostedCapabilitiesLine(t, []string{wslive.ReasoningSummaryCapability})), nopReplies{})
	evs := m.mapLine([]byte(hostedSummaryLine(t, hostedSummaryFixture())), nopReplies{})
	if len(evs) != 1 || evs[0].ReasoningSummary == nil || evs[0].Live != nil || evs[0].TurnEnd || evs[0].TurnStart || evs[0].BackgroundTasks != nil {
		t.Fatalf("widget acquired unrelated authority: %+v", evs)
	}
	worker := newPiMapper(StartSpec{})
	if evs := worker.mapLine([]byte(hostedSummaryLine(t, hostedSummaryFixture())), nopReplies{}); len(evs) != 0 {
		t.Fatal("subagent accepted lead summary")
	}
	bad := hostedSummaryFixture()
	bad.ActiveRequest.Attempt = 2
	if evs := m.mapLine([]byte(hostedSummaryLine(t, bad)), nopReplies{}); len(evs) != 0 {
		t.Fatal("dangling active pair accepted")
	}
	// Generic thinking cannot establish summary provenance.
	if evs := m.mapLine([]byte(`{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"private"}}`), nopReplies{}); len(evs) != 0 {
		t.Fatal("generic thinking became summary")
	}
	line := hostedSummaryLine(t, hostedSummaryFixture())
	line = strings.TrimSuffix(line, "}") + `,"padding":"` + strings.Repeat("x", wslive.SummaryFrameBytes) + `"}`
	if evs := m.mapLine([]byte(line), nopReplies{}); len(evs) != 0 {
		t.Fatal("oversized actual widget JSONL accepted")
	}
}

func hostedSummaryRecords(t *testing.T, h *leadHarness, id string) []wslive.RingRecord {
	t.Helper()
	h.quiesce(id)
	rows, err := wslive.ReadRing(wslive.RingDir(wsdashboard.SessionDir(h.dir, id)))
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestSummaryHostedFinalPrecedesNativeEndAndQuietReplayNeverWrites(t *testing.T) {
	h := newLeadHarness(t).start()
	id := h.lead(BackendPi, LeadOptions{}).ID
	h.send(id, "one")
	holder := h.holder(id)
	h.holders.out(holder, `{"type":"agent_start"}`, hostedCapabilitiesLine(t, []string{wslive.ReasoningSummaryCapability}), hostedSummaryLine(t, hostedSummaryFixture()))
	eventually(t, "live summary", func() bool { return h.snapshot(id).Lead.ReasoningSummary != nil })
	for _, r := range h.ring(id) {
		if r.SummaryHistory != nil {
			t.Fatal("widget committed a turn before native settlement")
		}
	}
	final := hostedSummaryFixture()
	final.Revision = 2
	final.ActiveRequest = nil
	final.Requests[0].Status = wslive.SummaryFailed
	final.Requests[0].Parts[0].Status = wslive.SummaryFailed
	h.holders.out(holder, hostedSummaryLine(t, final), `{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"actual failure"}}`, `{"type":"agent_settled","aborted":false}`)
	h.idle(id)
	rows := hostedSummaryRecords(t, h, id)
	var name, commit string
	for _, r := range rows {
		if r.SummaryHistory != nil {
			name = r.Name
			commit = r.SummaryHistory.Commit
			if r.Kind != wslive.RingTurn || r.Reason != "error" || r.SummaryHistory.Requests[0].Status != wslive.SummaryFailed || r.SummaryHistory.Requests[0].Parts[0].Text != "received prefix" {
				t.Fatalf("retained final: %+v", r)
			}
		}
	}
	if name == "" || commit == "" {
		t.Fatal("native natural end lost exact failed prefix")
	}
	h.restart()
	h.holders.out(holder, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":7,"monitors":0}`)))
	eventually(t, "Pi stream after quiet replay", func() bool { return h.state(id).BackgroundTasks == 7 })
	h.holders.out(holder, string(piWidgetLine(piEventsWidgetKey, `{"kind":"status","background":0,"monitors":0}`)))
	eventually(t, "Pi counts restored", func() bool { return h.state(id).BackgroundTasks == 0 })
	count := 0
	for _, r := range hostedSummaryRecords(t, h, id) {
		if r.SummaryHistory != nil {
			count++
			if r.Name != name || r.SummaryHistory.Commit != commit {
				t.Fatal("quiet replay rewrote retained turn")
			}
		}
	}
	if count != 1 {
		t.Fatalf("quiet replay repeated summary: %d", count)
	}
}

func TestSummaryHostedNativeAbortReasonIsActualFact(t *testing.T) {
	m := newPiMapper(StartSpec{Lead: &LeadSpec{}})
	m.mapLine([]byte(hostedCapabilitiesLine(t, []string{wslive.ReasoningSummaryCapability})), nopReplies{})
	evs := m.mapLine([]byte(`{"type":"agent_settled","aborted":true}`), nopReplies{})
	if len(evs) == 0 || !evs[len(evs)-1].TurnEnd || evs[len(evs)-1].TurnReason != wslive.TurnAborted {
		t.Fatalf("native abort: %+v", evs)
	}
}

func TestSummaryHostedReasonsRequireExplicitBundleCapability(t *testing.T) {
	for _, bundle := range []struct {
		name         string
		capabilities any
		capable      bool
	}{
		{"old-omitted", nil, false},
		{"unsupported", []string{}, false},
		{"unknown", []string{"future-capability"}, false},
		{"malformed", "reasoning_summary_v1", false},
		{"new-capable", []string{wslive.ReasoningSummaryCapability}, true},
	} {
		for _, stop := range []struct {
			name, reason, legacyError, capableError string
			aborted                                 bool
		}{
			{"stop", "", "", "", false},
			{"aborted", "aborted", "", "", false},
			{"error", "error", "actual failure", "actual failure", false},
			{"length", "incomplete", "", "", false},
			{"stop", "aborted", "", "", true},
			{"error", "aborted", "actual failure", "", true},
		} {
			t.Run(bundle.name+"/"+stop.name, func(t *testing.T) {
				m := newPiMapper(StartSpec{Lead: &LeadSpec{}})
				m.mapLine([]byte(hostedCapabilitiesLine(t, bundle.capabilities)), nopReplies{})
				m.mapLine([]byte(`{"type":"agent_start"}`), nopReplies{})
				raw, _ := json.Marshal(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{}, "stopReason": stop.name, "errorMessage": "actual failure"}})
				m.mapLine(raw, nopReplies{})
				raw, _ = json.Marshal(map[string]any{"type": "agent_settled", "aborted": stop.aborted})
				evs := m.mapLine(raw, nopReplies{})
				end := evs[len(evs)-1]
				want, wantError := "", stop.legacyError
				if bundle.capable {
					want, wantError = stop.reason, stop.capableError
				}
				if !end.TurnEnd || end.TurnReason != want || end.TurnError != wantError {
					t.Fatalf("bundle capability changed legacy end: %+v; want reason %q, error %q", end, want, wantError)
				}
			})
		}
	}
	m := newPiMapper(StartSpec{Lead: &LeadSpec{}})
	m.mapLine([]byte(hostedCapabilitiesLine(t, []string{wslive.ReasoningSummaryCapability})), nopReplies{})
	m.mapLine([]byte(hostedCapabilitiesLine(t, []string{})), nopReplies{})
	if evs := m.mapLine([]byte(hostedSummaryLine(t, hostedSummaryFixture())), nopReplies{}); len(evs) != 0 {
		t.Fatal("explicit capability revocation retained optional admission")
	}
}

func TestSummaryHostedOldBundlePersistsExactLegacyReason(t *testing.T) {
	for _, nativeAbort := range []bool{false, true} {
		for _, stop := range []string{"aborted", "error", "length"} {
			name := stop
			if nativeAbort {
				name += "-native-abort"
			}
			t.Run(name, func(t *testing.T) {
				h := newLeadHarness(t).start()
				id := h.lead(BackendPi, LeadOptions{}).ID
				h.send(id, "legacy bundle")
				holder := h.holder(id)
				raw, _ := json.Marshal(map[string]any{"type": "message_end", "message": map[string]any{"role": "assistant", "content": []any{}, "stopReason": stop, "errorMessage": "actual failure"}})
				settled, _ := json.Marshal(map[string]any{"type": "agent_settled", "aborted": nativeAbort})
				h.holders.out(holder, `{"type":"agent_start"}`, hostedCapabilitiesLine(t, nil), string(raw), string(settled))
				h.idle(id)
				turns := 0
				for _, row := range hostedSummaryRecords(t, h, id) {
					if row.Kind != wslive.RingTurn {
						continue
					}
					turns++
					if row.Reason != "" || row.SummaryHistory != nil {
						t.Fatalf("old bundle changed natural turn fields: %+v", row)
					}
				}
				if turns != 1 {
					t.Fatalf("legacy turn count = %d", turns)
				}
				wantError := ""
				if stop == "error" {
					wantError = "actual failure"
				}
				if got := h.state(id).Error; got != wantError {
					t.Fatalf("old bundle diagnostic changed: got %q, want %q", got, wantError)
				}
			})
		}
	}
}
