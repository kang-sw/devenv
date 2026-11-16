package wswhiteboard

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func wantSummaries(t *testing.T, out Outcome, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(out.Summaries, want) {
		t.Fatalf("summaries: got %q, want %q", out.Summaries, want)
	}
}

func TestTopicUpdateDefaultCreationPathsAndExplicitOverrides(t *testing.T) {
	for _, tc := range []struct{ name, attrs, command string }{
		{"item", "", `<wb:add class="check"><wb:title>Check</wb:title></wb:add>`},
		{"body", "", `<wb:body>Content</wb:body>`},
		{"log", "", `<wb:log>Ready</wb:log>`},
		{"panel", "", `<wb:panel recap="Design">Payload</wb:panel>`},
		{"tickets", `tickets="261115-feat-example"`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, explicit := range []string{"", StateDiscussing, StateWaiting, StateResearching, StateImplementing, StateAwaitingConfirmation} {
				s := &State{}
				command := `<wb:topic name="fresh" label="Fresh" ` + tc.attrs + `>` + tc.command
				value, source := StateDiscussing, "default"
				if explicit != "" {
					attrs := ""
					if explicit == StateImplementing {
						attrs = ` owner="executor" eta_minutes="40"`
					}
					command += `<wb:state value="` + explicit + `"` + attrs + `/>`
					value, source = explicit, "explicit"
				}
				command += `</wb:topic>`
				out := stateApply(t, s, command, stateNow)
				wantSummaries(t, out, "topic added: fresh (state: "+value+", "+source+")")
				view := s.TopicStates()["fresh"]
				if view.Diagnostic != nil || view.State == nil || view.State.Value != value {
					t.Fatalf("state: %+v", view)
				}
				var records []topicStateRecord
				for _, e := range s.Topics[0].Entries {
					if len(e.TopicState) > 0 {
						r, code := parseTopicState(e)
						if code != "" {
							t.Fatal(code)
						}
						records = append(records, r)
					}
				}
				if len(records) != 1 || records[0].Value != value || records[0].From != "" {
					t.Fatalf("intermediate default history: %+v", records)
				}
				if explicit == StateImplementing && (view.State.Run == nil || view.State.Run.Owner != "executor" || view.State.Run.ETAMinutes != 40) {
					t.Fatal("lost explicit run")
				}
				wantSummaries(t, stateApply(t, s, "", stateNow))
			}
		})
	}
}

func TestTopicUpdateResearchingMetadataNetChanges(t *testing.T) {
	s := &State{}
	out := stateApply(t, s, `<wb:topic name="Work"><wb:log>Investigate</wb:log><wb:state value="researching" owner="first" eta_minutes="7"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: Work (state: researching, explicit)")
	for _, e := range s.Topics[0].Entries {
		if strings.Contains(string(e.TopicState), StateDiscussing) {
			t.Fatal("explicit research retained intermediate default")
		}
	}
	for _, tc := range []struct {
		attrs, owner string
		eta          int64
	}{
		{`owner="second" eta_minutes="7"`, "second", 7},
		{`owner="second" eta_minutes="9"`, "second", 9},
		{`eta_minutes="9"`, "", 9},
		{"", "", 0},
	} {
		out = stateApply(t, s, stateCommand(StateResearching, tc.attrs), stateNow)
		wantSummaries(t, out, "topic updated: Work (state: researching)")
		state := s.TopicStates()["Work"].State
		if state.Owner != tc.owner || state.ETAMinutes != tc.eta || state.Run != nil {
			t.Fatalf("summary lost metadata: %+v", state)
		}
		wantSummaries(t, stateApply(t, s, stateCommand(StateResearching, tc.attrs), stateNow))
	}
	out = stateApply(t, s, `<wb:topic name="Work"><wb:state value="researching" owner="interim" eta_minutes="1"/><wb:state value="researching"/></wb:topic>`, stateNow)
	wantSummaries(t, out)
	out = stateApply(t, s, `<wb:topic name="Work"><wb:state value="waiting"/><wb:state value="researching"/></wb:topic>`, stateNow)
	wantSummaries(t, out)
}

func TestTopicUpdateAtomicRefusalAndProvisionalGroups(t *testing.T) {
	for _, command := range []string{
		`<wb:topic name="fresh"><wb:state value="waiting"/><wb:log>Too late</wb:log></wb:topic>`,
		`<wb:topic name="fresh"><wb:log>Rollback</wb:log></wb:topic><wb:patch id="999"/>`,
		`<wb:topic name="fresh" label="Fresh"><wb:state value="waiting"/><wb:body>Too late</wb:body></wb:topic>`,
	} {
		s := &State{}
		accepted := logXML("fresh", "Too late")
		if strings.Contains(command, "Rollback") {
			accepted = logXML("fresh", "Rollback")
		}
		if strings.Contains(command, `label="Fresh"`) {
			accepted = `<wb:topic name="fresh" label="Fresh"><wb:body>Too late</wb:body></wb:topic>`
		}
		mixedAt(t, s, command, accepted, stateNow)
		if s.TopicStates()["fresh"].State.Value != StateDiscussing {
			t.Fatal("failed state declaration suppressed default")
		}
	}
	s := &State{}
	wantSummaries(t, stateApply(t, s, `<wb:topic name="unused" label="Unused"><wb:body/></wb:topic>`, stateNow))
	if len(s.Topics) != 0 || len(s.TopicStates()) != 0 || s.EntrySeq != 0 {
		t.Fatal("empty group defaulted")
	}
	out := stateApply(t, s, `<wb:topic name="fresh"><wb:body>First</wb:body></wb:topic><wb:topic name="fresh"><wb:log>Again</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: fresh (state: waiting, explicit)")
	var n int
	for _, e := range s.Topics[0].Entries {
		if len(e.TopicState) > 0 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("repeated group default history: %d", n)
	}
}

func TestTopicUpdateLegacyAdoptionRenameAndLifecycle(t *testing.T) {
	// A genuine old board, not a new topic authored through the new producer.
	s := &State{Topics: []TopicDoc{{Name: "Old title", Body: "Legacy content"}}}
	wantSummaries(t, stateApply(t, s, "", stateNow))
	out := stateApply(t, s, `<wb:topic name="old" label="Old title"/>`, stateNow)
	wantSummaries(t, out, "topic updated: old (renamed from Old title)")
	if s.TopicStates()["old"].State != nil || s.EntrySeq != 1 {
		t.Fatal("adoption backfilled")
	}
	out = stateApply(t, s, `<wb:topic name="old" label="New title"><wb:body>Updated</wb:body></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic updated: old (panels 1, body, renamed from Old title)")
	if s.TopicStates()["old"].State != nil {
		t.Fatal("update backfilled")
	}
	stateRaw(s, "old", "e9", `{"v":2,"op":"set","value":"future"}`)
	out = stateApply(t, s, `<wb:topic name="old"><wb:log>Still allowed</wb:log></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic updated: old (logged 1)")
	if s.TopicStates()["old"].Diagnostic == nil {
		t.Fatal("lost diagnostic")
	}

	s = &State{}
	out = stateApply(t, s, `<wb:topic name="fresh"><wb:log>Created</wb:log><wb:close result="Done"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: fresh (state: none, default, closed)")
	var records []topicStateRecord
	for _, e := range s.Topics[0].Entries {
		if len(e.TopicState) > 0 {
			r, _ := parseTopicState(e)
			records = append(records, r)
		}
	}
	if len(records) != 2 || records[0].Value != StateDiscussing || records[1].Op != "reset" || records[1].From != StateDiscussing {
		t.Fatalf("default/close history: %+v", records)
	}
	out = stateApply(t, s, `<wb:topic name="fresh"><wb:log>Reopen</wb:log></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic updated: fresh (logged 1, reopened)")
	if s.TopicStates()["fresh"].State != nil {
		t.Fatal("reopen defaulted")
	}
	stateApply(t, s, `<wb:topic name="fresh"><wb:state value="implementing" owner="executor" eta_minutes="10"/></wb:topic>`, stateNow)
	run := *s.TopicStates()["fresh"].State.Run
	out = stateApply(t, s, `<wb:topic name="fresh" label="Fresh"><wb:body>Content</wb:body></wb:topic>`, stateNow.Add(time.Minute))
	wantSummaries(t, out, "topic updated: fresh (panels 1, body, renamed from fresh)")
	if !reflect.DeepEqual(run, *s.TopicStates()["fresh"].State.Run) {
		t.Fatal("rename changed run")
	}
	out = stateApply(t, s, `<wb:topic name="fresh"><wb:eta minutes="20" reason="More work"/></wb:topic>`, stateNow.Add(time.Minute))
	wantSummaries(t, out, "topic updated: fresh (eta 20m)")
	if got := s.TopicStates()["fresh"].State.Run; got.ID != run.ID || got.StartedAt != run.StartedAt || got.Owner != run.Owner || got.InitialETAMinutes != run.InitialETAMinutes || got.ExtensionCount != 1 {
		t.Fatal("ETA replaced run")
	}
}

func TestTopicUpdateNetChangesFirstTouchAndRootAnswers(t *testing.T) {
	s := &State{}
	stateApply(t, s, `<wb:topic name="a" label="A"><wb:add class="check"><wb:title>One</wb:title></wb:add><wb:add class="check"><wb:title>Two</wb:title></wb:add><wb:body>Original</wb:body></wb:topic><wb:topic name="b"><wb:add class="check"><wb:title>Three</wb:title></wb:add></wb:topic>`, stateNow)
	out := stateApply(t, s, `<wb:topic name="b"><wb:body/></wb:topic><wb:answer ids="1 3 2" value="yes">Yes.</wb:answer><wb:topic name="a"><wb:close result="Done"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic updated: b (answered 1)", "topic updated: a (answered 2, closed)")
	out = stateApply(t, s, `<wb:answer ids="1 2 3" value="yes">Yes.</wb:answer>`, stateNow)
	wantSummaries(t, out)
	out = stateApply(t, s, `<wb:patch id="3"><wb:title>Changed</wb:title></wb:patch>`, stateNow)
	wantSummaries(t, out, "topic updated: b (patched 1)")
	out = stateApply(t, s, `<wb:topic name="b"><wb:body>Interim</wb:body><wb:body/></wb:topic><wb:topic name="a" label="Interim"/><wb:topic name="a" label="A"/><wb:patch id="3"><wb:title>Interim</wb:title></wb:patch><wb:patch id="3"><wb:title>Changed</wb:title></wb:patch>`, stateNow)
	wantSummaries(t, out, "topic updated: b (panels 2)")
	stateApply(t, s, `<wb:topic name="b"><wb:state value="waiting"/></wb:topic>`, stateNow)
	out = stateApply(t, s, `<wb:topic name="b"><wb:state value="discussing"/><wb:state value="waiting"/></wb:topic>`, stateNow)
	wantSummaries(t, out)
	out = stateApply(t, s, `<wb:topic name="fresh"><wb:add class="check"><wb:title>New</wb:title></wb:add></wb:topic><wb:answer id="4" value="yes">Yes.</wb:answer><wb:topic name="fresh"><wb:body>More</wb:body></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: fresh (state: discussing, default)")
}

func TestTopicUpdateAdoptionKeepsFirstTouchAndExplicitIdentity(t *testing.T) {
	s := &State{Counter: 1, Items: []Item{{ID: 1, Topic: "Old", Class: ClassCheck, Status: StatusOpen, Title: "Check"}}, Topics: []TopicDoc{{Name: "other", Body: "Other"}}}
	out := stateApply(t, s, `<wb:answer id="1" value="yes">Yes.</wb:answer><wb:topic name="other"><wb:body>Changed</wb:body></wb:topic><wb:topic name="new" label="Old"/>`, stateNow)
	wantSummaries(t, out, "topic updated: new (answered 1, renamed from Old)", "topic updated: other (panels 2, body)")
	if s.TopicStates()["new"].State != nil {
		t.Fatal("root answer/adoption backfilled")
	}
	out = stateApply(t, s, `<wb:topic name="other" label="other"/>`, stateNow)
	wantSummaries(t, out, "topic updated: other (adopted)")
	wantSummaries(t, stateApply(t, s, `<wb:topic name="other" label="other"/>`, stateNow))
	s = &State{}
	out = stateApply(t, s, `<wb:topic name="Old"><wb:log>Start</wb:log></wb:topic><wb:topic name="new" label="Old"><wb:state value="waiting"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: new (state: waiting, explicit)")
	for _, e := range s.Topics[0].Entries {
		if strings.Contains(string(e.TopicState), StateDiscussing) {
			t.Fatal("default survived adoption/explicit")
		}
	}
}

func TestTopicUpdateWaitingOnNetDeclarations(t *testing.T) {
	s := &State{}
	stateApply(t, s, `<wb:topic name="dep"><wb:log>Blocker</wb:log></wb:topic><wb:topic name="other"><wb:log>Other blocker</wb:log></wb:topic>`, stateNow)
	out := stateApply(t, s, `<wb:topic name="wait"><wb:log>Waiting</wb:log><wb:state value="waiting"/></wb:topic>`, stateNow)
	wantSummaries(t, out, "topic added: wait (state: waiting, explicit)")
	stateEntry := s.TopicStates()["wait"].State.Entry
	for _, tc := range []struct {
		commands string
		targets  []string
		changed  bool
	}{
		{`<wb:state value="waiting" on="topic:dep"/>`, []string{"topic:dep"}, true},
		{`<wb:state value="waiting" on="topic:other"/>`, []string{"topic:other"}, true},
		{`<wb:state value="waiting" on="topic:other topic:other"/>`, []string{"topic:other"}, false},
		{`<wb:state value="waiting" on=""/>`, nil, true},
		{`<wb:state value="waiting" on=""/>`, nil, false},
		{`<wb:state value="waiting" on="topic:dep"/>`, []string{"topic:dep"}, true},
		{`<wb:state value="waiting"/>`, nil, true},
		{`<wb:state value="waiting" on="topic:dep"/><wb:state value="waiting"/>`, nil, false},
	} {
		out = stateApply(t, s, `<wb:topic name="wait">`+tc.commands+`</wb:topic>`, stateNow)
		if tc.changed {
			wantSummaries(t, out, "topic updated: wait (waiting on)")
		} else {
			wantSummaries(t, out)
		}
		if !slices.Equal(s.waitingDeclarations()["wait"].Targets, tc.targets) || s.TopicStates()["wait"].State.Entry != stateEntry {
			t.Fatal("summary changed upstream dependency-only declaration semantics")
		}
	}
}

func TestTopicUpdateClosedAdoptionDeduplicatesConvergingTouches(t *testing.T) {
	s := &State{}
	stateApply(t, s, `<wb:topic name="Old"><wb:log>History</wb:log><wb:close result="Done"/></wb:topic>`, stateNow)
	before := s.clone()
	out := stateApply(t, s, `<wb:topic name="new"><wb:body/></wb:topic><wb:topic name="Old"><wb:body/></wb:topic><wb:topic name="new" label="Old"/>`, stateNow)
	wantSummaries(t, out, "topic updated: new (renamed from Old)")
	if !s.topicClosed("new") || s.CloseResults["new"] != "Done" || !reflect.DeepEqual(before.Topics[0].Entries, s.Topics[0].Entries) || s.TopicStates()["new"].State != nil {
		t.Fatal("closed adoption changed lifecycle/history")
	}
}

func TestTopicUpdateKindTagsUseNetFieldWriters(t *testing.T) {
	s := &State{}
	stateApply(t, s, `<wb:topic name="t"><wb:add class="check"><wb:title>Original</wb:title></wb:add></wb:topic>`, stateNow)
	// The no-op patch must not take credit for the answer's changed fields.
	out := stateApply(t, s, `<wb:answer id="1" value="yes">Yes.</wb:answer><wb:patch id="1"><wb:title>Original</wb:title></wb:patch>`, stateNow)
	wantSummaries(t, out, "topic updated: t (answered 1)")
	// Real intermediate patches cancel; the answer remains the only net kind.
	out = stateApply(t, s, `<wb:patch id="1"><wb:title>Interim</wb:title></wb:patch><wb:patch id="1"><wb:title>Original</wb:title></wb:patch><wb:answer id="1" value="no">No.</wb:answer>`, stateNow)
	wantSummaries(t, out, "topic updated: t (answered 1)")
	out = stateApply(t, s, `<wb:answer id="1" value="yes">Yes.</wb:answer><wb:patch id="1"><wb:title>Changed</wb:title></wb:patch>`, stateNow)
	wantSummaries(t, out, "topic updated: t (answered 1, patched 1)")
	// An intermediate answer cancels while a separate title patch remains.
	out = stateApply(t, s, `<wb:answer id="1" value="no">No.</wb:answer><wb:answer id="1" value="yes">Yes.</wb:answer><wb:patch id="1"><wb:title>Final</wb:title></wb:patch>`, stateNow)
	wantSummaries(t, out, "topic updated: t (patched 1)")
}

func TestTopicUpdateDefaultFolderReloadAndRecovery(t *testing.T) {
	f := FolderStore{Dir: filepath.Join(t.TempDir(), "board")}
	var out Outcome
	if err := f.Update(nil, func(s *State) error {
		var p []string
		out, p = Apply(s, `<wb:topic name="fresh" tickets="261115-feat-example"/>`, Env{Now: stateNow})
		if len(p) > 0 {
			return fmt.Errorf("%v", p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	wantSummaries(t, out, "topic added: fresh (state: discussing, default)")
	s := folderOpen(t, f.Dir, nil)
	before := s.clone()
	if view := s.TopicStates()["fresh"]; view.State == nil || view.State.Value != StateDiscussing || view.Diagnostic != nil {
		t.Fatalf("reload: %+v", view)
	}
	if err := f.Update(nil, func(s *State) error { out = stateApply(t, s, "", stateNow); return nil }); err != nil {
		t.Fatal(err)
	}
	wantSummaries(t, out)
	after := folderOpen(t, f.Dir, nil)
	if !reflect.DeepEqual(before.Topics, after.Topics) || before.EntrySeq != after.EntrySeq {
		t.Fatal("render-only duplicated default")
	}
}
