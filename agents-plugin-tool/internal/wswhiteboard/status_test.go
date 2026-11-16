package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
)

func statusEnv() Env {
	env := testEnv()
	env.StatusAuthor = "ticket-runner"
	return env
}

// statusBoard is a board whose lead opened topic "work" with a named panel,
// and closed topic "done".
func statusBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	mustApply(t, st, `<wb:topic name="work"><wb:panel name="plan" recap="Plan">steps</wb:panel></wb:topic>`+
		`<wb:topic name="done"><wb:log>finished</wb:log><wb:close result="ok"/></wb:topic>`)
	return st
}

func statusRefused(t *testing.T, st *State, commands, want string) {
	t.Helper()
	before, _ := json.Marshal(st)
	out, problems := Apply(st, commands, statusEnv())
	if out.Accepted != 0 || !hasProblem(problems, want) {
		t.Fatalf("Apply(%s) accepted %d, problems %v; want a refusal naming %q", commands, out.Accepted, problems, want)
	}
	after, _ := json.Marshal(st)
	if string(before) != string(after) {
		t.Fatalf("a refused status write must leave the board untouched")
	}
}

func TestStatusWriteAcceptsLogAndNewPanelWithAttribution(t *testing.T) {
	st := statusBoard(t)
	out, problems := Apply(st, `<wb:topic name="work"><wb:log>build green</wb:log><wb:panel name="report" recap="Phase result">all done</wb:panel><wb:panel recap="Unnamed">more</wb:panel></wb:topic>`, statusEnv())
	if len(problems) > 0 || out.Accepted != 3 {
		t.Fatalf("status write: accepted %d, problems %v", out.Accepted, problems)
	}
	doc := st.Topics[st.topicDocIndex("work")]
	var log, panels []string
	for _, e := range doc.Entries {
		switch e.Kind {
		case EntryAgent:
			log = append(log, e.Text)
		case EntryPanel:
			panels = append(panels, e.Recap+"|"+e.Text)
		}
	}
	if len(log) != 1 || log[0] != "[ticket-runner] build green" {
		t.Fatalf("log lines = %q, want the attributed line", log)
	}
	want := []string{"Plan|steps", "[ticket-runner] Phase result|all done", "[ticket-runner] Unnamed|more"}
	if strings.Join(panels, "\n") != strings.Join(want, "\n") {
		t.Fatalf("panels = %q, want %q", panels, want)
	}
}

func TestStatusWriteRefusals(t *testing.T) {
	for _, tc := range []struct{ name, commands, want string }{
		{"no such topic", `<wb:topic name="missing"><wb:log>x</wb:log></wb:topic>`, `no open topic "missing"`},
		{"closed topic", `<wb:topic name="done"><wb:log>x</wb:log></wb:topic>`, `no open topic "done"`},
		{"label group", `<wb:topic name="work" label="Work"><wb:log>x</wb:log></wb:topic>`, "cannot create, relabel, or relink a topic"},
		{"tickets group", `<wb:topic name="work" tickets="261116-feat-x"><wb:log>x</wb:log></wb:topic>`, "cannot create, relabel, or relink a topic"},
		{"ask", `<wb:topic name="work"><wb:add class="policy"><wb:title>t</wb:title></wb:add></wb:topic>`, "writes status only"},
		{"body", `<wb:topic name="work"><wb:body>b</wb:body></wb:topic>`, "writes status only"},
		{"close", `<wb:topic name="work"><wb:close result="r"/></wb:topic>`, "writes status only"},
		{"panel id", `<wb:topic name="work"><wb:panel id="p1" recap="r">x</wb:panel></wb:topic>`, "omit id"},
		{"existing panel name", `<wb:topic name="work"><wb:panel name="plan" recap="r">x</wb:panel></wb:topic>`, `panel "plan" already exists`},
		{"panel patch", `<wb:topic name="work"><wb:panel name="plan"><wb:change><wb:old>steps</wb:old><wb:new>s</wb:new></wb:change></wb:panel></wb:topic>`, `panel "plan" already exists`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statusRefused(t, statusBoard(t), tc.commands, tc.want)
		})
	}
}

// A panel name minted earlier in the same call is taken for later commands.
func TestStatusWriteRefusesPanelNameMintedEarlierInCall(t *testing.T) {
	st := statusBoard(t)
	out, problems := Apply(st, `<wb:topic name="work"><wb:panel name="fresh" recap="a">one</wb:panel><wb:panel name="fresh" recap="b">two</wb:panel></wb:topic>`, statusEnv())
	if out.Accepted != 1 || !hasProblem(problems, `panel "fresh" already exists`) {
		t.Fatalf("accepted %d, problems %v; want the first panel accepted and the second refused", out.Accepted, problems)
	}
	if !strings.Contains(out.Failed, `recap="b"`) || strings.Contains(out.Failed, "[ticket-runner]") {
		t.Fatalf("the failed retry payload must be the second panel as written: %q", out.Failed)
	}
}

// Without StatusAuthor the same commands keep the lead's full grammar.
func TestStatusAuthorEmptyKeepsLeadGrammar(t *testing.T) {
	st := statusBoard(t)
	mustApply(t, st, `<wb:topic name="work"><wb:panel name="plan" recap="Plan v2">steps 2</wb:panel><wb:log>lead line</wb:log></wb:topic>`)
	doc := st.Topics[st.topicDocIndex("work")]
	last := doc.Entries[len(doc.Entries)-1]
	if last.Text != "lead line" {
		t.Fatalf("a lead's log line carries no attribution: %q", last.Text)
	}
}
