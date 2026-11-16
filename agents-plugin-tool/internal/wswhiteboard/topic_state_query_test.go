package wswhiteboard

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestQueryLatestSummary(t *testing.T) {
	const later = "2026-11-13T13:00:00Z"
	for _, tc := range []struct {
		name, agentID, panelID, agentAt, panelAt, wantID, wantText string
	}{
		{"panel only", "", "e2", "", queryEntryTime, "e2", "panel recap"},
		{"agent only", "e2", "", queryEntryTime, "", "e2", "agent body"},
		{"newer panel", "e10", "e2", queryEntryTime, later, "e2", "panel recap"},
		{"newer agent", "e2", "e10", later, queryEntryTime, "e2", "agent body"},
		{"equal panel wins", "e9", "e10", queryEntryTime, queryEntryTime, "e10", "panel recap"},
		{"equal agent wins", "e10", "e9", queryEntryTime, queryEntryTime, "e10", "agent body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := TopicDoc{Name: "summary"}
			if tc.agentID != "" {
				doc.Entries = append(doc.Entries, Entry{ID: tc.agentID, Kind: EntryAgent, At: tc.agentAt, Text: "agent body"})
			}
			if tc.panelID != "" {
				doc.Entries = append(doc.Entries, Entry{ID: tc.panelID, Kind: EntryPanel, PanelID: "p1", At: tc.panelAt, Recap: "panel recap", Text: "secret panel payload"})
			}
			doc.Entries = append(doc.Entries, Entry{ID: "e99", Kind: EntrySystem, At: later, Text: "system event"})
			st := &State{Topics: []TopicDoc{doc}}
			for _, raw := range []string{`{"topic":"summary"}`, `{"topic":"summary","depth":"topics"}`, `{"topic":"summary","body":true}`, `{"topic":"summary","entries":"log","kind":"system","last":1}`} {
				r := queryResult(t, st, raw)
				q := r.Topics[0]
				if q.LatestSummary == nil || q.LatestSummary.ID != tc.wantID {
					t.Fatalf("%s: summary = %+v", raw, q.LatestSummary)
				}
				if tc.agentID == "" && q.LatestAgent != nil || tc.agentID != "" && (q.LatestAgent == nil || q.LatestAgent.ID != tc.agentID) {
					t.Fatalf("latest agent changed: %+v", q.LatestAgent)
				}
				text := FormatQuery(r)
				if !strings.Contains(text, "current "+tc.wantID+" ") || !strings.Contains(text, ": "+tc.wantText+"\n") || strings.Contains(text, "secret panel payload") {
					t.Fatalf("%s: wrong current summary or leaked payload:\n%s", raw, text)
				}
				encoded, err := json.Marshal(q)
				if err != nil || strings.Contains(string(encoded), "secret panel payload") {
					t.Fatalf("query metadata leaked payload: %s; %v", encoded, err)
				}
			}
		})
	}
}

func topicStateQueryBoard() *State {
	st := queryEntryBoard()
	st.Topics[0].Tickets = []string{"261101-feat-example"}
	st.EntrySeq = 19
	st.Topics[0].Entries = append(st.Topics[0].Entries, Entry{
		ID: "e19", Kind: EntrySystem, At: queryEntryTime, Text: "Execution started",
		TopicState: json.RawMessage(`{"v":1,"op":"set","value":"implementing","run":{"id":"e19","started_at":"2026-11-13T12:00:00Z","owner":"query executor","initial_eta_minutes":40,"eta_minutes":40,"extension_count":0}}`),
	})
	return st
}

func TestTopicStateQueryResearching(t *testing.T) {
	st := topicStateQueryBoard()
	stateApply(t, st, `<wb:topic name="a"><wb:state value="researching" owner="query investigator" eta_minutes="7"/></wb:topic>`, stateNow)
	for _, raw := range []string{`{"topic":"a","body":true}`, `{"topic":"a"}`, `{"topic":"a","entries":"log","kind":"agent","last":1}`, `{"id":1,"depth":"full"}`} {
		r := queryResult(t, st, raw)
		q := r.Topics[0]
		if q.State == nil || q.State.Value != StateResearching || q.State.Run != nil || q.State.Owner != "query investigator" || q.State.ETAMinutes != 7 || q.StateDiagnostic != nil || q.LatestAgent.ID != "e15" {
			t.Fatalf("research query: %+v", q)
		}
		wantText(t, "informational research", FormatQuery(r), []string{"state: researching; entry: " + q.State.Entry, `owner: "query investigator"`, "eta_minutes: 7 (informational)"}, []string{"run:", "deadline_at:", "started_at:", "extension_count:"})
		data, err := json.Marshal(q.State)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := json.Unmarshal(data, &fields); err != nil || fields["owner"] != "query investigator" || fields["eta_minutes"] != float64(7) || fields["run"] != nil {
			t.Fatalf("research JSON: %s; %v", data, err)
		}
	}
	stateApply(t, st, `<wb:topic name="a"><wb:state value="researching"/></wb:topic>`, stateNow)
	data, _ := json.Marshal(queryResult(t, st, `{"topic":"a"}`).Topics[0].State)
	if strings.Contains(string(data), "owner") || strings.Contains(string(data), "eta_minutes") || strings.Contains(string(data), "run") {
		t.Fatalf("absent research metadata serialized: %s", data)
	}
}

func TestTopicStateQueryImplementing(t *testing.T) {
	st := topicStateQueryBoard()
	wantRun := TopicRun{ID: "e19", StartedAt: queryEntryTime, Owner: "query executor", InitialETAMinutes: 40, ETAMinutes: 40, DeadlineAt: "2026-11-13T12:40:00Z"}
	for _, raw := range []string{
		`{"topic":"a","body":true}`, // retained item/content route
		`{"topic":"a"}`,             // discovery
		`{"topic":"a","entries":"log","kind":"agent","last":1}`, // filtered stream
		`{"id":1,"status":"open","text":"title-marker","depth":"full"}`,
	} {
		t.Run(raw, func(t *testing.T) {
			r := queryResult(t, st, raw)
			if len(r.Topics) != 1 {
				t.Fatalf("topics = %+v", r.Topics)
			}
			q := r.Topics[0]
			if q.State == nil || q.State.Value != StateImplementing || q.State.Entry != "e19" || q.State.Run == nil || *q.State.Run != wantRun || q.StateDiagnostic != nil {
				t.Fatalf("derived state = %+v", q)
			}
			if q.LatestAgent == nil || q.LatestAgent.ID != "e15" || !reflect.DeepEqual(q.Tickets, st.Topics[0].Tickets) {
				t.Fatalf("latest agent or tickets changed: %+v", q)
			}
			text := FormatQuery(r)
			wantText(t, "state metadata", text, []string{"state: implementing; entry: e19", "run: e19", "started_at: " + queryEntryTime, `owner: "query executor"`, "initial_eta_minutes: 40", "eta_minutes: 40", "extension_count: 0", "deadline_at: " + wantRun.DeadlineAt, "current e15 ", "tickets: 261101-feat-example"}, []string{"current e19", "payload secret"})
			if strings.Contains(raw, `"body":true`) {
				if q.Body != st.Topics[0].Body || !strings.Contains(text, "topic body:\n<<<\n"+q.Body+"\n>>>\n") {
					t.Fatal("explicit body changed")
				}
			} else if q.Body != "" || strings.Contains(text, "CURRENT CONTENT") {
				t.Fatal("implicit body leaked")
			}
			if r.Mode == "stream" && !reflect.DeepEqual(queryEntryIDs(q.Entries), []string{"e15"}) {
				t.Fatalf("stream filter changed: %+v", q.Entries)
			}
			if strings.Contains(raw, `"id":1`) && (len(q.Items) != 1 || q.Items[0].ID != 1 || !reflect.DeepEqual(r.Matches[1], []string{"title"})) {
				t.Fatalf("item filters changed: %+v", r)
			}
			encoded, err := json.Marshal(q)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			var state TopicState
			if err := json.Unmarshal(fields["state"], &state); err != nil || !reflect.DeepEqual(&state, q.State) {
				t.Fatalf("state JSON = %s: %v", encoded, err)
			}
			if _, present := fields["state_diagnostic"]; present {
				t.Fatalf("unexpected diagnostic JSON: %s", encoded)
			}
		})
	}
	// State is topic metadata, not searchable text or a new selector.
	for _, raw := range []string{`{"topic":"a","text":"query executor"}`, `{"id":2,"topic":"a"}`} {
		if r := queryResult(t, st, raw); len(r.Topics) != 0 {
			t.Fatalf("filters broadened for %s: %+v", raw, r)
		}
	}
	if _, problems := Query(st, map[string]any{"state": "implementing"}); len(problems) == 0 {
		t.Fatal("state unexpectedly accepted as selector")
	}
	for _, raw := range []string{`{"entry":"e19"}`, `{"panel":"e4"}`} {
		r := queryResult(t, st, raw)
		if r.Direct == nil || len(r.Topics) != 0 || !strings.HasSuffix(FormatQuery(r), "\n"+r.Direct.Entry.Text) || strings.Contains(FormatQuery(r), "state:") {
			t.Fatalf("direct retrieval changed: %+v", r)
		}
	}
}

func TestTopicStateQueryAbsentAndDiagnostic(t *testing.T) {
	for _, tc := range []struct {
		name, payload, code string
	}{
		{"legacy", "", ""},
		{"malformed", `{"v":1,"op":"set","value":"implementing"}`, "malformed"},
		{"unsupported", `{"v":2,"op":"set"}`, "unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := topicStateQueryBoard()
			if tc.payload == "" {
				st.Topics[0].Entries = st.Topics[0].Entries[:len(st.Topics[0].Entries)-1]
			} else {
				st.Topics[0].Entries = append(st.Topics[0].Entries, Entry{ID: "e20", Kind: EntrySystem, At: queryEntryTime, Text: "Unknown state", TopicState: json.RawMessage(tc.payload)})
			}
			for _, raw := range []string{`{"topic":"a","depth":"topics"}`, `{"topic":"a"}`, `{"topic":"a","entries":"log","kind":"agent","last":1}`} {
				q := queryResult(t, st, raw).Topics[0]
				if q.State != nil || q.LatestAgent == nil || q.LatestAgent.ID != "e15" {
					t.Fatalf("state fallback or agent replacement: %+v", q)
				}
				encoded, err := json.Marshal(q)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(encoded, &fields); err != nil {
					t.Fatal(err)
				}
				if _, present := fields["state"]; present {
					t.Fatalf("absent state serialized: %s", encoded)
				}
				text := FormatQuery(queryResult(t, st, raw))
				if tc.code == "" {
					if q.StateDiagnostic != nil || fields["state_diagnostic"] != nil || strings.Contains(text, "state:") || strings.Contains(text, "state_diagnostic:") {
						t.Fatalf("legacy absence changed: %s", encoded)
					}
				} else {
					want := StateDiagnostic{Entry: "e20", Code: tc.code}
					var diagnostic StateDiagnostic
					if q.StateDiagnostic == nil || *q.StateDiagnostic != want || json.Unmarshal(fields["state_diagnostic"], &diagnostic) != nil || diagnostic != want {
						t.Fatalf("diagnostic = %s", encoded)
					}
					wantText(t, "diagnostic metadata", text, []string{"state_diagnostic: " + tc.code + "; entry: e20"}, []string{"  state:", "deadline_at:"})
				}
			}
		})
	}
}
