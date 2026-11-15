package wswhiteboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

var stateNow = time.Date(2026, 11, 15, 1, 0, 0, 123456789, time.UTC)

func stateApply(t *testing.T, s *State, commands string, now time.Time) Outcome {
	t.Helper()
	out, problems := Apply(s, commands, Env{Now: now})
	if len(problems) != 0 {
		t.Fatalf("Apply: %v", problems)
	}
	return out
}

func stateCommand(value, attrs string) string {
	return `<wb:topic name="Work"><wb:state value="` + value + `" ` + attrs + `/></wb:topic>`
}

func stateBoard(t *testing.T) *State {
	t.Helper()
	s := &State{}
	stateApply(t, s, `<wb:topic name="Work"><wb:log>Latest human summary</wb:log></wb:topic>`, stateNow)
	return s
}

func TestTopicStateGrammarAtomicityAndBounds(t *testing.T) {
	for _, command := range []string{
		`<wb:state value="waiting"/>`,
		stateCommand("closed", ""), stateCommand("", ""),
		stateCommand("waiting", `owner="executor"`),
		stateCommand("discussing", `eta_minutes="1"`),
		stateCommand("implementing", `eta_minutes="1"`),
		stateCommand("implementing", `owner="   " eta_minutes="1"`),
		stateCommand("implementing", `owner="`+strings.Repeat("雪", 86)+`" eta_minutes="1"`),
		stateCommand("implementing", "owner=\"bad\x01owner\" eta_minutes=\"1\""),
		stateCommand("implementing", "owner=\"bad\xffowner\" eta_minutes=\"1\""),
		stateCommand("waiting", `started_at="2026-01-01T00:00:00Z"`),
		`<wb:topic name="Work"><wb:state value="waiting">body</wb:state></wb:topic>`,
		`<wb:topic name="Work"><wb:eta minutes="2" reason="later"/></wb:topic>`,
	} {
		t.Run(fmt.Sprintf("invalid-%q", command), func(t *testing.T) {
			s := stateBoard(t)
			before := s.clone()
			_, problems := Apply(s, `<wb:topic name="Work"><wb:log>must roll back</wb:log></wb:topic>`+command, Env{Now: stateNow})
			if len(problems) == 0 || !reflect.DeepEqual(s, before) {
				t.Fatalf("non-atomic refusal: %v", problems)
			}
		})
	}
	for _, eta := range []string{"", "0", "-1", "+1", "1.5", " 1", "1 ", "1e2", "١", "9223372036854775808", fmt.Sprint(maxETAMinutes + 1)} {
		t.Run("eta-"+eta, func(t *testing.T) {
			s := stateBoard(t)
			_, problems := Apply(s, stateCommand("implementing", `owner="executor" eta_minutes="`+eta+`"`), Env{Now: stateNow})
			if len(problems) == 0 {
				t.Fatal("accepted invalid ETA")
			}
		})
	}
	for _, eta := range []int64{1, maxETAMinutes} {
		s := stateBoard(t)
		stateApply(t, s, stateCommand("implementing", fmt.Sprintf(`owner="  %s  " eta_minutes="%d"`, strings.Repeat("x", 256), eta)), stateNow)
		run := s.TopicStates()["Work"].State.Run
		if len(run.Owner) != 256 || run.ETAMinutes != eta || run.DeadlineAt == "" {
			t.Fatalf("run: %+v", run)
		}
	}
	for _, now := range []time.Time{time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(-1, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 0, 0, time.UTC)} {
		s := stateBoard(t)
		_, problems := Apply(s, stateCommand("implementing", `owner="executor" eta_minutes="1"`), Env{Now: now})
		if len(problems) == 0 {
			t.Fatalf("accepted unrepresentable deadline at %s", now)
		}
	}
	// A representable final minute and non-UTC server time round-trip exactly.
	s := stateBoard(t)
	now := time.Date(9999, 12, 31, 23, 58, 0, 0, time.UTC).In(time.FixedZone("offset", 3600))
	stateApply(t, s, stateCommand("implementing", `owner="executor" eta_minutes="1"`), now)
	if s.TopicStates()["Work"].State.Run.DeadlineAt != "9999-12-31T23:59:00Z" {
		t.Fatal("deadline conversion")
	}
}

func TestTopicStateLifecycleIdempotenceAndAdoption(t *testing.T) {
	s := &State{}
	if _, problems := Apply(s, stateCommand("waiting", ""), Env{Now: stateNow}); len(problems) == 0 || len(s.Topics) != 0 {
		t.Fatal("state created an empty topic")
	}
	s = stateBoard(t)
	command := stateCommand("implementing", `owner="executor" eta_minutes="40"`)
	stateApply(t, s, command, stateNow)
	start := s.TopicStates()["Work"].State
	seq := s.EntrySeq
	stateApply(t, s, command, stateNow.Add(time.Hour))
	if s.EntrySeq != seq || !reflect.DeepEqual(start, s.TopicStates()["Work"].State) {
		t.Fatal("identical declaration reset run")
	}
	for _, attrs := range []string{`owner="other" eta_minutes="40"`, `owner="executor" eta_minutes="41"`} {
		before := s.clone()
		if _, p := Apply(s, stateCommand("implementing", attrs), Env{Now: stateNow}); len(p) == 0 || !reflect.DeepEqual(before, s) {
			t.Fatal("changed declaration accepted")
		}
	}
	stateApply(t, s, `<wb:topic name="work" label="Work"/>`, stateNow)
	if !reflect.DeepEqual(start, s.TopicStates()["work"].State) {
		t.Fatal("adoption reset run or entry identity")
	}
	stateApply(t, s, `<wb:topic name="work"><wb:state value="awaiting-confirmation"/></wb:topic>`, stateNow.Add(2*time.Minute))
	e := s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
	r, code := parseTopicState(e)
	if code != "" || r.From != StateImplementing || *r.ElapsedMS != 120000 || r.Run.ID != start.Run.ID || !strings.Contains(e.Text, "actual 120000 ms") {
		t.Fatalf("completed run: %+v %s %s", r, code, e.Text)
	}
	if s.TopicStates()["work"].State.Run != nil {
		t.Fatal("active completed run")
	}
	stateApply(t, s, command, stateNow.Add(3*time.Minute)) // old label resolves adopted topic
	if s.TopicStates()["work"].State.Run.ID == start.Run.ID {
		t.Fatal("re-entry reused run")
	}
	stateApply(t, s, `<wb:topic name="work"><wb:add class="check"><wb:title>Verify</wb:title></wb:add><wb:close result="Done"/></wb:topic>`, stateNow.Add(4*time.Minute))
	if s.TopicStates()["work"].State != nil || s.Items[0].Status != StatusWithdrawn {
		t.Fatal("close retained activity")
	}
	last := s.Topics[0].Entries[len(s.Topics[0].Entries)-1]
	r, code = parseTopicState(last)
	if code != "" || r.Op != "reset" || r.Run == nil || *r.ElapsedMS != 60000 {
		t.Fatalf("close: %+v %s", r, code)
	}
	before := s.clone()
	if _, p := Apply(s, command, Env{Now: stateNow}); len(p) == 0 || !reflect.DeepEqual(before, s) {
		t.Fatal("state reopened closed topic")
	}
	stateApply(t, s, `<wb:topic name="work"><wb:log>Reopened</wb:log></wb:topic>`, stateNow)
	if s.TopicStates()["work"].State != nil || s.Items[0].Status != StatusWithdrawn {
		t.Fatal("reopen restored state/ask")
	}
	stateApply(t, s, stateCommand("discussing", ""), stateNow)
	seq = s.EntrySeq
	stateApply(t, s, stateCommand("discussing", ""), stateNow)
	if s.EntrySeq != seq {
		t.Fatal("non-implementing repeat emitted history")
	}
	if latest, _ := s.LatestAgent("work"); latest.Text != "Reopened" {
		t.Fatal("state event replaced LatestAgent")
	}
}

func stateRaw(s *State, topic, id, payload string) {
	s.topicDoc(topic).Entries = append(s.topicDoc(topic).Entries, Entry{ID: id, Kind: EntrySystem, At: stateNow.Format(time.RFC3339Nano), Text: "machine event", TopicState: json.RawMessage(payload)})
	s.EntrySeq = max(s.EntrySeq, folderNumber(id, "e"))
}

func TestTopicStateDiagnosticsAndRawRetention(t *testing.T) {
	for _, tc := range []struct{ raw, code string }{
		{`null`, "malformed"}, {`[]`, "malformed"}, {`{}`, "malformed"},
		{`{"v":1,"op":"set","value":"nonsense"}`, "malformed"},
		{`{"v":1,"op":"set","value":"implementing"}`, "malformed"},
		{`{"v":1,"op":"set","value":"waiting","run":{}}`, "malformed"},
		{`{"v":2,"op":"set","value":"waiting"}`, "unsupported"},
		{`{"v":1,"op":"future"}`, "unsupported"},
		{`{"v":1,"op":"extend"}`, "unsupported"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			s := stateBoard(t)
			stateApply(t, s, stateCommand("implementing", `owner="executor" eta_minutes="1"`), stateNow)
			stateRaw(s, "Work", "e3", tc.raw)
			view := s.TopicStates()["Work"]
			if view.State != nil || view.Diagnostic == nil || view.Diagnostic.Code != tc.code || view.Diagnostic.Entry != "e3" {
				t.Fatalf("view: %+v", view)
			}
			before := s.clone()
			_, p := Apply(s, stateCommand("waiting", ""), Env{Now: stateNow})
			if tc.code == "unsupported" {
				if len(p) == 0 || !reflect.DeepEqual(before, s) {
					t.Fatal("mutated unsupported state")
				}
				stateApply(t, s, `<wb:topic name="Work"><wb:log>old content remains accepted</wb:log><wb:close result="Closed"/><wb:log>Reopen</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow)
			} else if len(p) != 0 {
				t.Fatal("malformed v1 prevented explicit new baseline", p)
			}
			if s.TopicStates()["Work"].State.Value != StateWaiting {
				t.Fatal("new baseline failed")
			}
		})
	}
	raw := []byte(`{"id":"e1","kind":"system","at":"2026-11-15T01:00:00.123456789Z","topic_state": { "v":1, "op":"set", "value":"waiting", "future": [ 1, 2 ] }, "future_entry": { "opaque": true }}`)
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatal(err)
	}
	s := &State{EntrySeq: 1, Topics: []TopicDoc{{Name: "Work", Entries: []Entry{e}}}}
	clone := s.clone()
	if !bytes.Equal(e.TopicState, clone.Topics[0].Entries[0].TopicState) || !reflect.DeepEqual(e.unknown, clone.Topics[0].Entries[0].unknown) {
		t.Fatal("clone changed raw bytes")
	}
	clone.Topics[0].Entries[0].TopicState[0] = '['
	clone.Topics[0].Entries[0].unknown["future_entry"][0] = '['
	if e.TopicState[0] != '{' || e.unknown["future_entry"][0] != '{' {
		t.Fatal("clone aliases raw bytes")
	}
	if view := s.TopicStates()["Work"]; view.State == nil || view.State.Value != StateWaiting {
		t.Fatalf("unknown optional member rejected: %+v", view)
	}
}

func TestTopicStateNumericOrderAndBoundedOldClose(t *testing.T) {
	s := &State{}
	stateRaw(s, "Z-closed", "e9", `{"v":1,"op":"set","value":"waiting"}`)
	stateRaw(s, "A-open", "e2", `{"v":1,"op":"set","value":"discussing"}`)
	stateRaw(s, "Z-closed", "e10", `{"v":1,"op":"reset","from":"waiting"}`)
	s.ClosedTopics = []string{"Z-closed"}
	events := s.stateEvents()
	if len(events) != 3 || events[0].entry.ID != "e2" || events[1].entry.ID != "e9" || events[2].entry.ID != "e10" || s.TopicStates()["Z-closed"].State != nil {
		t.Fatal("numeric/all-history fold")
	}
	for _, tc := range []struct {
		kind     string
		imported bool
		item     int
		text     string
		reset    bool
	}{
		{EntrySystem, false, 0, "Topic closed: real old close", true},
		{EntryAgent, false, 0, "Topic closed: authored log", false},
		{EntryPanel, false, 0, "Topic closed: panel", false},
		{EntrySystem, true, 0, "Topic closed: imported recap", false},
		{EntrySystem, false, 1, "Topic closed: item", false},
		{EntrySystem, false, 0, "prefix Topic closed: not whole", false},
		{EntrySystem, false, 0, " Topic closed: leading space", false},
		{EntrySystem, false, 0, "Topic close result updated: later", false},
	} {
		t.Run(tc.text, func(t *testing.T) {
			s := stateBoard(t)
			stateApply(t, s, stateCommand("waiting", ""), stateNow)
			s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: "e3", Kind: tc.kind, Imported: tc.imported, ItemID: tc.item, Text: tc.text, At: stateNow.Format(time.RFC3339Nano)})
			if (s.TopicStates()["Work"].State == nil) != tc.reset {
				t.Fatal("close producer boundary")
			}
		})
	}
	// Real accepted authoring paths cannot supply raw system provenance.
	s = stateBoard(t)
	stateApply(t, s, stateCommand("waiting", ""), stateNow)
	stateApply(t, s, `<wb:topic name="Work"><wb:log>Topic closed: authored log</wb:log><wb:panel recap="Topic closed: recap">Topic closed: panel</wb:panel><wb:add class="check"><wb:title>Topic closed: item</wb:title></wb:add></wb:topic><wb:answer id="1" value="ok">Topic closed: answer</wb:answer>`, stateNow)
	if s.TopicStates()["Work"].State.Value != StateWaiting {
		t.Fatal("authored text reset state")
	}
	// Prefer present structured metadata, even malformed, over the text adapter.
	s.Topics[0].Entries = append(s.Topics[0].Entries, Entry{ID: "e99", Kind: EntrySystem, Text: "Topic closed: forged local file", At: stateNow.Format(time.RFC3339Nano), TopicState: json.RawMessage(`{"v":1}`)})
	if s.TopicStates()["Work"].Diagnostic == nil {
		t.Fatal("text overrode structured metadata")
	}
}

func TestTopicStateStructuralWarnings(t *testing.T) {
	s := &State{}
	out := stateApply(t, s, `<wb:topic name="z"><wb:log>z</wb:log><wb:state value="discussing"/></wb:topic><wb:topic name="a"><wb:log>a</wb:log><wb:state value="discussing"/></wb:topic><wb:topic name="wait"><wb:log>w</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow)
	if !strings.Contains(out.Info, `discussing_without_open_items: "a", "z"`) || !strings.Contains(out.Info, `waiting_without_implementing: "wait"`) {
		t.Fatalf("warnings: %s", out.Info)
	}
	before := len(s.Items)
	out = stateApply(t, s, `<wb:topic name="a"><wb:add class="check"><wb:title>Verify</wb:title></wb:add></wb:topic><wb:topic name="z"><wb:add class="technical"><wb:title>Technical ask</wb:title></wb:add></wb:topic><wb:topic name="run"><wb:log>r</wb:log><wb:state value="implementing" owner="executor" eta_minutes="1"/></wb:topic>`, stateNow)
	if strings.Contains(out.Info, "discussing_without") || strings.Contains(out.Info, "waiting_without") || len(s.Items) != before+2 {
		t.Fatalf("non-triggering counterpart: %s", out.Info)
	}
	out = stateApply(t, s, `<wb:topic name="run"><wb:close result="Done"/></wb:topic>`, stateNow)
	if !strings.Contains(out.Info, "waiting_without_implementing") {
		t.Fatal("closed implementation suppressed warning")
	}
	legacy := stateBoard(t)
	if out := stateApply(t, legacy, "", stateNow); strings.Contains(out.Info, "without_") {
		t.Fatal("absent legacy state warned")
	}
	if len(legacy.Items) != 0 {
		t.Fatal("warning synthesized asks")
	}
}
