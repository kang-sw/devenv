package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func etaCommand(minutes, reason string) string {
	return `<wb:topic name="Work"><wb:eta minutes="` + minutes + `" reason="` + reason + `"/></wb:topic>`
}

func etaBoard(t *testing.T) *State {
	t.Helper()
	s := stateBoard(t)
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="40"`), stateNow)
	return s
}

func TestTopicETAResearchIsUntimedAndImplementationStartsFresh(t *testing.T) {
	s := etaBoard(t)
	oldRun := *s.TopicStates()["Work"].State.Run
	stateApply(t, s, stateCommand(StateResearching, `owner="investigator" eta_minutes="1"`), stateNow.Add(10*time.Minute))
	e := s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
	r, code := parseTopicState(e)
	if code != "" || r.From != StateImplementing || r.Run == nil || r.Run.ID != oldRun.ID || r.ElapsedMS == nil || *r.ElapsedMS != 10*60*1000 || s.TopicStates()["Work"].State.Run != nil {
		t.Fatalf("implementing completion into research: %+v %s", r, code)
	}
	before := s.clone()
	if _, p := Apply(s, etaCommand("60", "research is not a run"), Env{Now: stateNow.Add(time.Hour)}); len(p) == 0 || !reflect.DeepEqual(before, s) {
		t.Fatal("research accepted timed extension")
	}
	start := stateNow.Add(2 * time.Hour)
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor2" eta_minutes="5"`), start)
	run := s.TopicStates()["Work"].State.Run
	if run.ID == oldRun.ID || run.StartedAt != start.Format(time.RFC3339Nano) || run.Owner != "executor2" || run.InitialETAMinutes != 5 || run.ETAMinutes != 5 || run.ExtensionCount != 0 || run.DeadlineAt != start.Add(5*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("research clock leaked into implementing: %+v", run)
	}
	if state := s.TopicStates()["Work"].State; state.Owner != "" || state.ETAMinutes != 0 {
		t.Fatal("research attributes leaked into implementing")
	}
	stateApply(t, s, etaCommand("8", "implementation extension"), start.Add(time.Minute))
	stateApply(t, s, `<wb:topic name="Work"><wb:close result="Done"/></wb:topic>`, start.Add(3*time.Minute))
	r, code = parseTopicState(s.Topics[0].Entries[len(s.Topics[0].Entries)-1])
	if code != "" || r.Run.ID != run.ID || r.Run.InitialETAMinutes != 5 || r.Run.ETAMinutes != 8 || r.Run.ExtensionCount != 1 || *r.ElapsedMS != 3*60*1000 {
		t.Fatalf("fresh implementation completion: %+v %s", r, code)
	}
}

func TestTopicETACumulativeHistoryAndNoop(t *testing.T) {
	s := etaBoard(t)
	original := *s.TopicStates()["Work"].State.Run
	for i, step := range []struct{ minutes, reason string }{{"60", "  dependency expanded  "}, {"90", "integration needs validation"}} {
		stateApply(t, s, etaCommand(step.minutes, step.reason), stateNow.Add(time.Duration(i+1)*20*time.Minute))
		view := s.TopicStates()["Work"]
		run := view.State.Run
		wantMinutes := []int64{60, 90}[i]
		if run.ID != original.ID || run.StartedAt != original.StartedAt || run.Owner != original.Owner || run.InitialETAMinutes != 40 || run.ETAMinutes != wantMinutes || run.ExtensionCount != i+1 || run.DeadlineAt != stateNow.Add(time.Duration(wantMinutes)*time.Minute).Format(time.RFC3339Nano) || view.State.Entry == original.ID {
			t.Fatalf("extension lost original run: %+v", run)
		}
		e := s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
		r, code := parseTopicState(e)
		if code != "" || r.Op != "extend" || r.Reason != strings.TrimSpace(step.reason) || !strings.Contains(e.Text, r.Reason) || strings.Contains(string(e.TopicState), "deadline_at") {
			t.Fatalf("extension history: %+v, %s, %s", r, code, e.Text)
		}
	}
	before := s.clone()
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="90"`), stateNow.Add(2*time.Hour))
	before.Seq++ // Accepted batches advance render bookkeeping, even with no new event.
	if !reflect.DeepEqual(before, s) {
		t.Fatal("identical current declaration reset execution or history")
	}
	for _, minutes := range []string{"40", "60", "90"} {
		if _, p := Apply(s, etaCommand(minutes, "not an extension"), Env{Now: stateNow}); len(p) == 0 || !reflect.DeepEqual(before, s) {
			t.Fatal("same/smaller total was accepted or mutated state", minutes, p)
		}
	}
	stateApply(t, s, stateCommand(StateAwaitingConfirmation, ""), stateNow.Add(95*time.Minute))
	r, code := parseTopicState(s.Topics[0].Entries[len(s.Topics[0].Entries)-1])
	if code != "" || r.Run.InitialETAMinutes != 40 || r.Run.ETAMinutes != 90 || r.Run.ExtensionCount != 2 || r.ElapsedMS == nil || *r.ElapsedMS != 95*60*1000 {
		t.Fatalf("completion lost cumulative accounting: %+v %s", r, code)
	}
	var reasons []string
	for _, e := range s.Topics[0].Entries {
		if r, code := parseTopicState(e); code == "" && r.Op == "extend" {
			reasons = append(reasons, r.Reason)
		}
	}
	if !reflect.DeepEqual(reasons, []string{"dependency expanded", "integration needs validation"}) {
		t.Fatalf("individual reasons: %q", reasons)
	}
	if latest, _ := s.LatestAgent("Work"); latest.Text != "Latest human summary" {
		t.Fatal("extension replaced LatestAgent")
	}
}

func TestTopicETAGrammarBoundsAndAtomicity(t *testing.T) {
	invalid := []string{
		`<wb:eta minutes="60" reason="scope"/>`,
		`<wb:topic name="Work"><wb:eta reason="scope"/></wb:topic>`,
		`<wb:topic name="Work"><wb:eta minutes="60"/></wb:topic>`,
		`<wb:topic name="Work"><wb:eta minutes="60" reason="scope">body</wb:eta></wb:topic>`,
		`<wb:topic name="Work"><wb:eta minutes="60" reason="scope" owner="other"/></wb:topic>`,
		`<wb:topic name="Work"><wb:eta minutes="60" reason="scope" started_at="2026-01-01T00:00:00Z"/></wb:topic>`,
	}
	for _, minutes := range []string{"", "0", "-60", "+60", "60.5", " 60", "60 ", "6e1", "٦٠", "9223372036854775808", fmt.Sprint(maxETAMinutes + 1), "40", "39"} {
		invalid = append(invalid, etaCommand(minutes, "scope"))
	}
	for _, reason := range []string{"", "   ", strings.Repeat("x", 2049), strings.Repeat("雪", 683), "bad\x01reason", "bad\x7freason", "bad\xffreason"} {
		invalid = append(invalid, etaCommand("60", reason))
	}
	for _, command := range invalid {
		t.Run(fmt.Sprintf("invalid-%q", command), func(t *testing.T) {
			s := etaBoard(t)
			mixedAt(t, s, `<wb:topic name="Work"><wb:log>accepted</wb:log></wb:topic>`+command, `<wb:topic name="Work"><wb:log>accepted</wb:log></wb:topic>`, stateNow.Add(time.Minute))
		})
	}
	for _, minutes := range []int64{41, maxETAMinutes} {
		s := etaBoard(t)
		stateApply(t, s, etaCommand(fmt.Sprint(minutes), "  "+strings.Repeat("x", 2048)+"  "), stateNow)
		if s.TopicStates()["Work"].State.Run.ETAMinutes != minutes {
			t.Fatal("valid ETA boundary lost")
		}
	}
	for _, now := range []time.Time{time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC)} {
		s := etaBoard(t)
		before := s.clone()
		if _, p := Apply(s, etaCommand("60", "scope"), Env{Now: now}); len(p) == 0 || !reflect.DeepEqual(before, s) {
			t.Fatal("unrepresentable server time accepted")
		}
	}
	s := stateBoard(t)
	nearEnd := time.Date(9999, 12, 31, 23, 58, 0, 0, time.UTC)
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="1"`), nearEnd)
	before := s.clone()
	if _, p := Apply(s, etaCommand("2", "past representable timestamp"), Env{Now: nearEnd}); len(p) == 0 || !reflect.DeepEqual(before, s) {
		t.Fatal("unrepresentable extension deadline accepted")
	}
}

func TestTopicETAKnownOpenOrderedBatch(t *testing.T) {
	for _, setup := range []string{"", stateCommand(StateWaiting, ""), stateCommand(StateResearching, `eta_minutes="40"`), `<wb:topic name="Work"><wb:close result="done"/></wb:topic>`} {
		s := stateBoard(t)
		stateApply(t, s, setup, stateNow)
		before := s.clone()
		if _, p := Apply(s, etaCommand("60", "scope"), Env{Now: stateNow}); len(p) == 0 || !reflect.DeepEqual(before, s) {
			t.Fatal("ETA created/reopened/non-implementing topic")
		}
	}
	s := &State{}
	stateApply(t, s, `<wb:topic name="Work"><wb:log>create</wb:log><wb:state value="implementing" owner="executor" eta_minutes="40"/><wb:eta minutes="60" reason="same batch"/></wb:topic>`, stateNow)
	if s.TopicStates()["Work"].State.Run.ExtensionCount != 1 {
		t.Fatal("ordered working-batch state was ignored")
	}
	stateApply(t, s, `<wb:topic name="Work"><wb:close result="done"/><wb:log>reopen</wb:log><wb:state value="implementing" owner="replacement" eta_minutes="1"/><wb:eta minutes="2" reason="fresh run"/></wb:topic>`, stateNow.Add(time.Hour))
	if run := s.TopicStates()["Work"].State.Run; run.InitialETAMinutes != 1 || run.Owner != "replacement" || run.ExtensionCount != 1 {
		t.Fatalf("re-entry inherited old run: %+v", run)
	}
}

func TestTopicETAExtensionSnapshotChainDiagnostics(t *testing.T) {
	base := etaBoard(t)
	stateApply(t, base, etaCommand("60", "first reason"), stateNow.Add(time.Minute))
	stateApply(t, base, etaCommand("90", "second reason"), stateNow.Add(2*time.Minute))
	last := len(base.Topics[0].Entries) - 1
	for name, tamper := range map[string]func(*topicStateRecord){
		"run-id":        func(r *topicStateRecord) { r.Run.ID = "e1" },
		"start":         func(r *topicStateRecord) { r.Run.StartedAt = stateNow.Add(time.Second).Format(time.RFC3339Nano) },
		"owner":         func(r *topicStateRecord) { r.Run.Owner = "replacement" },
		"initial":       func(r *topicStateRecord) { r.Run.InitialETAMinutes = 39 },
		"count-skip":    func(r *topicStateRecord) { r.Run.ExtensionCount = 3 },
		"count-repeat":  func(r *topicStateRecord) { r.Run.ExtensionCount = 1 },
		"same-total":    func(r *topicStateRecord) { r.Run.ETAMinutes = 60 },
		"smaller-total": func(r *topicStateRecord) { r.Run.ETAMinutes = 59 },
		"empty-reason":  func(r *topicStateRecord) { r.Reason = "" },
		"padded-reason": func(r *topicStateRecord) { r.Reason = " reason " },
	} {
		t.Run(name, func(t *testing.T) {
			s := base.clone()
			e := &s.Topics[0].Entries[last]
			r, code := parseTopicState(*e)
			if code != "" {
				t.Fatal(code)
			}
			tamper(&r)
			data, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			e.TopicState = data
			view := s.TopicStates()["Work"]
			if view.State != nil || view.Diagnostic == nil || view.Diagnostic.Code != "malformed" || view.Diagnostic.Entry != e.ID {
				t.Fatalf("tampered chain restored older authority: %+v", view)
			}
			before := s.clone()
			if _, p := Apply(s, etaCommand("100", "cannot extend unknown"), Env{Now: stateNow}); len(p) == 0 || !reflect.DeepEqual(before, s) {
				t.Fatal("extended unknown chain")
			}
			stateApply(t, s, stateCommand(StateWaiting, ""), stateNow)
			if s.TopicStates()["Work"].State.Value != StateWaiting {
				t.Fatal("valid baseline did not supersede diagnostic")
			}
		})
	}
	// A structurally valid extension still needs an implementing predecessor.
	s := &State{Topics: []TopicDoc{{Name: "Work", Entries: []Entry{base.Topics[0].Entries[last]}}}}
	if v := s.TopicStates()["Work"]; v.State != nil || v.Diagnostic == nil || v.Diagnostic.Code != "malformed" {
		t.Fatalf("orphan extension: %+v", v)
	}
}

func TestTopicETALongRunElapsedMilliseconds(t *testing.T) {
	s := stateBoard(t)
	start := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	stateApply(t, s, stateCommand(StateImplementing, `owner="executor" eta_minutes="40"`), start)
	stateApply(t, s, etaCommand("60", "long-running work"), end)
	stateApply(t, s, stateCommand(StateAwaitingConfirmation, ""), end)
	r, code := parseTopicState(s.Topics[0].Entries[len(s.Topics[0].Entries)-1])
	want := end.UnixMilli() - start.UnixMilli()
	if code != "" || r.ElapsedMS == nil || *r.ElapsedMS != want || want <= int64(1<<63-1)/int64(time.Millisecond) {
		t.Fatalf("long-run elapsed milliseconds: %+v %s; want %d", r, code, want)
	}
}

// Pending the lead-owned parser adjustment; excluded by the first focused run,
// not skipped in code, so the integration run must exercise this contract.
func TestTopicETAPendingOmittedValue(t *testing.T) {
	s := etaBoard(t)
	run := *s.TopicStates()["Work"].State.Run
	run.ETAMinutes, run.ExtensionCount, run.DeadlineAt = 60, 1, ""
	data, err := json.Marshal(topicStateRecord{V: 1, Op: "extend", Run: &run, Reason: "snapshot carries implementing run"})
	if err != nil {
		t.Fatal(err)
	}
	stateRaw(s, "Work", fmt.Sprintf("e%d", s.EntrySeq+1), string(data))
	v := s.TopicStates()["Work"]
	if v.Diagnostic != nil || v.State == nil || v.State.Value != StateImplementing || v.State.Run.ETAMinutes != 60 {
		t.Fatalf("omitted extend value must derive implementing: %+v", v)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	fields["value"] = json.RawMessage(`"waiting"`)
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	stateRaw(s, "Work", fmt.Sprintf("e%d", s.EntrySeq+1), string(data))
	if v := s.TopicStates()["Work"]; v.State != nil || v.Diagnostic == nil || v.Diagnostic.Code != "malformed" {
		t.Fatalf("conflicting explicit extend value accepted: %+v", v)
	}
}
