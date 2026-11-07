package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)

func testEnv() Env {
	return Env{
		Now:   testNow,
		IsDir: func(string) bool { return false },
		IsAbs: filepath.IsAbs,
	}
}

func mustApply(t *testing.T, st *State, commands string) Outcome {
	t.Helper()
	out, problems := Apply(st, commands, testEnv())
	if len(problems) > 0 {
		t.Fatalf("Apply(%s) refused: %v", commands, problems)
	}
	return out
}

// refused applies a batch that must be refused and checks it left the state
// untouched; it returns every problem.
func refused(t *testing.T, st *State, commands string) []string {
	t.Helper()
	before, _ := json.Marshal(st)
	_, problems := Apply(st, commands, testEnv())
	if len(problems) == 0 {
		t.Fatalf("Apply(%s) must refuse", commands)
	}
	after, _ := json.Marshal(st)
	if string(before) != string(after) {
		t.Fatalf("a refused batch must leave the state untouched:\nbefore %s\nafter  %s", before, after)
	}
	return problems
}

// queryPayload decodes a JSON literal the way the MCP layer receives a
// whiteboard.query payload (the query keeps its JSON filter).
func queryPayload(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("bad test payload %s: %v", raw, err)
	}
	return v
}

func hasProblem(problems []string, substr string) bool {
	for _, p := range problems {
		if strings.Contains(p, substr) {
			return true
		}
	}
	return false
}

func wantProblems(t *testing.T, problems []string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !hasProblem(problems, want) {
			t.Fatalf("problems missing %q: %q", want, problems)
		}
	}
}

func item(t *testing.T, st *State, id int) Item {
	t.Helper()
	idx := st.index(id)
	if idx < 0 {
		t.Fatalf("item %d not in store", id)
	}
	return st.Items[idx]
}

// addXML is a wb:add command: title, then the options when given.
func addXML(class, topic, title string, options ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<wb:add class=%q topic=%q><wb:title>%s</wb:title>`, class, topic, title)
	for _, o := range options {
		fmt.Fprintf(&sb, `<wb:option>%s</wb:option>`, o)
	}
	sb.WriteString(`</wb:add>`)
	return sb.String()
}

func topicXML(name, body string) string {
	return `<wb:topic name="` + name + `">` + body + `</wb:topic>`
}

func TestApplyBatchInOrderReturnsIDsInAddOrder(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `
<wb:add class="policy" topic="t"><wb:title>first</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>b: safer</wb:recommend></wb:add>
<wb:add class="technical" topic="t"><wb:title>second</wb:title><wb:why>because</wb:why></wb:add>
<wb:add class="announced" topic="u"><wb:title>third</wb:title></wb:add>
<wb:answer id="2" value="ok">fine</wb:answer>`)
	if fmt.Sprint(out.IDs) != "[1 2 3]" {
		t.Fatalf("minted IDs = %v, want [1 2 3]", out.IDs)
	}
	// The answer applied after the add in the same batch.
	if got := item(t, st, 2); got.Status != StatusAnswered || got.AnswerQuote != "fine" || got.Rationale != "because" {
		t.Fatalf("item 2 = %+v, want answered with quote", got)
	}
	if got := item(t, st, 1); got.Recommendation != "b: safer" || len(got.Options) != 2 {
		t.Fatalf("item 1 = %+v", got)
	}
	if st.Seq != 1 {
		t.Fatalf("one update advances the render sequence once; Seq = %d", st.Seq)
	}
}

// Commands apply in the order written: the same two commands in the other
// order is a different batch.
func TestApplyCommandsRunInTheOrderWritten(t *testing.T) {
	st := &State{}
	// An answer ahead of the add it targets names an item that does not exist yet.
	problems := refused(t, st, `<wb:answer id="1" value="ok">q</wb:answer>`+addXML(ClassTechnical, "t", "x"))
	wantProblems(t, problems, "command 1 <wb:answer> (line 1) (01): not in the store")

	// Close then add: the add reopens the topic. Add then close: it stays closed.
	mustApply(t, st, addXML(ClassTechnical, "t", "x"))
	mustApply(t, st, `<wb:close topic="t"/>`+addXML(ClassTechnical, "t", "y"))
	if st.topicClosed("t") {
		t.Fatalf("close then add must leave the topic open: %v", st.ClosedTopics)
	}
	mustApply(t, st, addXML(ClassTechnical, "t", "z")+`<wb:close topic="t"/>`)
	if !st.topicClosed("t") {
		t.Fatalf("add then close must leave the topic closed")
	}

	// A patch that reopens an item, then one that edits its options, in one batch.
	mustApply(t, st, `<wb:add class="policy" topic="p"><wb:title>p</wb:title><wb:option>a1</wb:option><wb:option>b1</wb:option></wb:add>
<wb:answer id="4" value="a">q</wb:answer>
<wb:patch id="4" status="open"/>
<wb:patch id="4"><wb:option>only</wb:option></wb:patch>`)
	if got := item(t, st, 4); got.Status != StatusOpen || len(got.Options) != 1 || got.Answer != "" {
		t.Fatalf("item 4 = %+v", got)
	}
}

func TestApplyExplicitIDAdoptsAndRaisesCounter(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `<wb:add id="40" class="policy" topic="q"><wb:title>adopted</wb:title></wb:add>`+addXML(ClassTechnical, "q", "minted"))
	if fmt.Sprint(out.IDs) != "[40 41]" {
		t.Fatalf("IDs = %v, want [40 41]", out.IDs)
	}
	// A lower explicit ID not yet in the store is adopted without lowering the counter.
	out = mustApply(t, st, `<wb:add id="7" class="policy" topic="q"><wb:title>older</wb:title></wb:add>`+addXML(ClassPolicy, "q", "next"))
	if fmt.Sprint(out.IDs) != "[7 42]" {
		t.Fatalf("IDs = %v, want [7 42]", out.IDs)
	}
	problems := refused(t, st, `<wb:add id="40" class="policy" topic="q"><wb:title>dup</wb:title></wb:add>`)
	wantProblems(t, problems, "id: (40) is already in the store")
	// Within one batch an adopted ID is taken for the commands after it.
	problems = refused(t, st, `<wb:add id="50" class="policy" topic="q"><wb:title>a</wb:title></wb:add><wb:add id="50" class="policy" topic="q"><wb:title>b</wb:title></wb:add>`)
	wantProblems(t, problems, "command 2 <wb:add> (line 1) id: (50) is already in the store")
	for _, bad := range []string{"(43)", "0", "-3", "1.5", ""} {
		problems = refused(t, st, fmt.Sprintf(`<wb:add id=%q class="policy" topic="q"><wb:title>x</wb:title></wb:add>`, bad))
		wantProblems(t, problems, "id: must be a positive whole number")
	}
}

func TestApplyRefusedBatchAppliesNothingAndReportsEveryProblem(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "one"))
	problems := refused(t, st, `<wb:add class="wrong" topic="t"><wb:title>x</wb:title></wb:add>
<wb:add class="policy"><wb:title>no topic</wb:title></wb:add>
<wb:answer id="99" value="a">q</wb:answer>
<wb:patch id="1" status="answered"/>
<wb:bogus/>`)
	wantProblems(t, problems,
		`command 1 <wb:add> (line 1) class: must be one of policy, technical, announced`,
		`command 2 <wb:add> (line 2) topic: required`,
		`command 3 <wb:answer> (line 3) (99): not in the store`,
		`command 4 <wb:patch> (line 4) status: must be one of open, withdrawn, superseded`,
		`command 5 <wb:bogus> (line 5): unknown command`,
	)
	if st.Counter != 1 || len(st.Items) != 1 {
		t.Fatalf("refused batch changed the store: %+v", st)
	}
	// A valid command ahead of a bad one is not applied either.
	problems = refused(t, st, addXML(ClassTechnical, "t", "fine")+"\n"+`<wb:close topic="nowhere"/>`)
	wantProblems(t, problems, `command 2 <wb:close> (line 2) topic: no item or topic body has topic "nowhere"`)
}

func TestApplyAddFieldRules(t *testing.T) {
	st := &State{}
	for want, batch := range map[string]string{
		"<wb:title>: required":                   `<wb:add class="policy" topic="t"/>`,
		"<wb:title>: must be non-empty":          `<wb:add class="policy" topic="t"><wb:title> </wb:title></wb:add>`,
		"more than one <wb:title>":               `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:title>b</wb:title></wb:add>`,
		"more than one <wb:why>":                 `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:why>x</wb:why><wb:why>y</wb:why></wb:add>`,
		"more than one <wb:recommend>":           `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:option>x</wb:option><wb:recommend>a</wb:recommend><wb:recommend>a</wb:recommend></wb:add>`,
		"option b must be non-empty":             `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:option>x</wb:option><wb:option>  </wb:option></wb:add>`,
		"class: required":                        `<wb:add topic="t"><wb:title>a</wb:title></wb:add>`,
		"topic: must be non-empty":               `<wb:add class="policy" topic=""><wb:title>a</wb:title></wb:add>`,
		"<wb:recommend>: names an option letter": `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:recommend>a</wb:recommend></wb:add>`,
		"starts with its option letter (a-b)":    `<wb:add class="policy" topic="t"><wb:title>a</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>c: no such</wb:recommend></wb:add>`,
		"at most 26 options":                     `<wb:add class="policy" topic="t"><wb:title>a</wb:title>` + strings.Repeat(`<wb:option>o</wb:option>`, 27) + `</wb:add>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
	// Links are repeatable and keep their order and text.
	mustApply(t, st, `<wb:add class="technical" topic="t"><wb:title>l</wb:title><wb:link>one</wb:link><wb:link>two</wb:link></wb:add>`)
	if got := item(t, st, 1).Links; strings.Join(got, "|") != "one|two" {
		t.Fatalf("links = %q", got)
	}
}

func TestApplyPatchOptionsRefusedOnNonOpenItem(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "p", "x", "y")+`<wb:answer id="1" value="b">b please</wb:answer>`)
	problems := refused(t, st, `<wb:patch id="1"><wb:option>z</wb:option><wb:option>x</wb:option><wb:option>y</wb:option></wb:patch>`)
	wantProblems(t, problems, "options change only on an open item")
	// Re-opening first, as its own command, then changing options is accepted.
	mustApply(t, st, `<wb:patch id="1" status="open"/><wb:patch id="1"><wb:option>z</wb:option><wb:option>x</wb:option><wb:option>y</wb:option></wb:patch>`)
	if got := item(t, st, 1); got.Status != StatusOpen || len(got.Options) != 3 || got.Settled != 0 {
		t.Fatalf("item = %+v", got)
	}
	// Options are lettered a, b, c by position: a patch re-letters the list.
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for i, o := range view.Topics[0].Policy[0].Options {
		if o.Letter != OptionLetterFor(i) || o.Text != []string{"z", "x", "y"}[i] {
			t.Fatalf("option %d = %+v", i, o)
		}
	}
	// Reopening drops the earlier answer; withdrawing keeps it as history.
	if got := item(t, st, 1); got.Answer != "" || got.AnswerQuote != "" || got.AnsweredAt != "" {
		t.Fatalf("a reopened item must carry no answer: %+v", got)
	}
	mustApply(t, st, `<wb:answer id="1" value="a">a then</wb:answer>`)
	mustApply(t, st, `<wb:patch id="1" status="withdrawn"/>`)
	if got := item(t, st, 1); got.Status != StatusWithdrawn || got.Answer != "a" || got.AnswerQuote != "a then" {
		t.Fatalf("a withdrawn item keeps its answer: %+v", got)
	}
}

func TestApplyPatchFieldsAndStatuses(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:add class="policy" topic="t"><wb:title>p</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>a</wb:recommend><wb:why>old why</wb:why><wb:link>old</wb:link></wb:add>`)
	// A patch replaces only the fields it carries.
	mustApply(t, st, `<wb:patch id="1"><wb:title>reworded</wb:title><wb:link>n1</wb:link><wb:link>n2</wb:link></wb:patch>`)
	got := item(t, st, 1)
	if got.Title != "reworded" || got.Rationale != "old why" || got.Recommendation != "a" || strings.Join(got.Links, "|") != "n1|n2" || len(got.Options) != 2 {
		t.Fatalf("item = %+v", got)
	}
	// An empty recommend or why clears that field.
	mustApply(t, st, `<wb:patch id="1"><wb:recommend></wb:recommend><wb:why/></wb:patch>`)
	if got := item(t, st, 1); got.Recommendation != "" || got.Rationale != "" {
		t.Fatalf("empty fields must clear: %+v", got)
	}
	// Superseding settles the item, reopening clears its settle order.
	mustApply(t, st, `<wb:patch id="1" status="superseded"/>`)
	if got := item(t, st, 1); got.Status != StatusSuperseded || got.Settled == 0 {
		t.Fatalf("superseded item = %+v", got)
	}
	mustApply(t, st, `<wb:patch id="1" status="open"/>`)
	if got := item(t, st, 1); got.Status != StatusOpen || got.Settled != 0 {
		t.Fatalf("reopened item = %+v", got)
	}
	// An answer to a withdrawn or superseded item is refused until it is reopened.
	mustApply(t, st, `<wb:patch id="1" status="withdrawn"/>`)
	wantProblems(t, refused(t, st, `<wb:answer id="1" value="a">q</wb:answer>`), "is withdrawn; patch its status to open before answering it")
	for want, batch := range map[string]string{
		"id: required":                   `<wb:patch status="open"/>`,
		"(09) is not in the store":       `<wb:patch id="9" status="open"/>`,
		"<wb:title>: must be non-empty":  `<wb:patch id="1"><wb:title> </wb:title></wb:patch>`,
		"must be one of open, withdrawn": `<wb:patch id="1" status="answered"/>`,
		"unknown attribute value":        `<wb:patch id="1" value="x"/>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
}

// wb:patch clears a list field with clear="options" or clear="links"; naming
// the same field with fields contradicts itself.
func TestApplyPatchClearsListFields(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:add class="policy" topic="t"><wb:title>p</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:link>a</wb:link><wb:link>b</wb:link></wb:add>`)
	mustApply(t, st, `<wb:patch id="1" clear="links"/>`)
	if got := item(t, st, 1); len(got.Links) != 0 || len(got.Options) != 2 {
		t.Fatalf("clear links: %+v", got)
	}
	mustApply(t, st, `<wb:patch id="1" clear="options"/>`)
	if got := item(t, st, 1); len(got.Options) != 0 {
		t.Fatalf("clear options: %+v", got)
	}
	mustApply(t, st, `<wb:patch id="1"><wb:option>p</wb:option></wb:patch><wb:patch id="1" clear="options links"/>`)
	if got := item(t, st, 1); len(got.Options) != 0 || len(got.Links) != 0 {
		t.Fatalf("clear both: %+v", got)
	}

	wantProblems(t, refused(t, st, `<wb:patch id="1" clear="options"><wb:option>z</wb:option></wb:patch>`),
		`command 1 <wb:patch> (line 1): clear="options" and <wb:option> fields contradict each other`)
	wantProblems(t, refused(t, st, `<wb:patch id="1" clear="links"><wb:link>z</wb:link></wb:patch>`),
		`clear="links" and <wb:link> fields contradict each other`)
	wantProblems(t, refused(t, st, `<wb:patch id="1" clear="title"/>`), `clear: must name options or links; got "title"`)

	// Options clear only on an open item, like any options change.
	mustApply(t, st, `<wb:patch id="1"><wb:option>p</wb:option></wb:patch><wb:answer id="1" value="a">q</wb:answer>`)
	wantProblems(t, refused(t, st, `<wb:patch id="1" clear="options"/>`), "options change only on an open item")
	// A recommendation that names an option letter needs the options it names.
	mustApply(t, st, `<wb:patch id="1" status="open"><wb:recommend>a</wb:recommend></wb:patch>`)
	wantProblems(t, refused(t, st, `<wb:patch id="1" clear="options"/>`), "names an option letter, so it needs options")
	mustApply(t, st, `<wb:patch id="1" clear="options"><wb:recommend></wb:recommend></wb:patch>`)
	if got := item(t, st, 1); len(got.Options) != 0 || got.Recommendation != "" {
		t.Fatalf("clear options with the recommendation: %+v", got)
	}
}

func TestApplyAnswerRequiresQuote(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x"))
	for _, batch := range []string{
		`<wb:answer id="1" value="ok"/>`,
		`<wb:answer id="1" value="ok"></wb:answer>`,
		"<wb:answer id=\"1\" value=\"ok\">  \n </wb:answer>",
	} {
		wantProblems(t, refused(t, st, batch), "command 1 <wb:answer> (line 1) body (the user's verbatim words): must be non-empty")
	}
	wantProblems(t, refused(t, st, `<wb:answer id="1">q</wb:answer>`), "value: required")
	wantProblems(t, refused(t, st, `<wb:answer id="1" value="ok" assumed="yes">q</wb:answer>`), `assumed: must be "true" or "false"`)
	wantProblems(t, refused(t, st, `<wb:answer value="ok">q</wb:answer>`), "id (or ids) required")
	// The quote is the body, kept as written apart from the one newline at each end.
	mustApply(t, st, "<wb:answer id=\"1\" value=\"ok\">\n  (01) ok, <b>fine</b> & done\n</wb:answer>")
	if got := item(t, st, 1).AnswerQuote; got != "  (01) ok, <b>fine</b> & done" {
		t.Fatalf("quote = %q", got)
	}
}

func TestApplyPolicyAnswerIsOptionLetter(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "p", "x", "y"))
	wantProblems(t, refused(t, st, `<wb:answer id="1" value="yes">yes</wb:answer>`), "starts with its option letter (a-b)")
	mustApply(t, st, `<wb:answer id="1" value="b, keep it small">b but small</wb:answer>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	iv := view.Topics[0].Expanded[0]
	if iv.AnswerLetter != "b" || iv.AnswerText != "keep it small" || !iv.Options[1].Picked {
		t.Fatalf("answer split = %+v", iv)
	}
}

func TestApplyAssumedThenConfirmedMovesToAnswered(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "p", "x", "y"))
	mustApply(t, st, `<wb:answer id="1" value="a" assumed="true">sounds right</wb:answer>`)
	got := item(t, st, 1)
	if got.Status != StatusAssumed || got.Settled != 0 {
		t.Fatalf("assumed item = %+v", got)
	}
	later := testEnv()
	later.Now = testNow.Add(time.Hour)
	if _, problems := Apply(st, `<wb:answer id="1" value="a">(01) ok</wb:answer>`, later); len(problems) > 0 {
		t.Fatalf("confirm refused: %v", problems)
	}
	got = item(t, st, 1)
	if got.Status != StatusAnswered || got.AnswerQuote != "(01) ok" || got.AnsweredAt != testNow.Add(time.Hour).Format(time.RFC3339) || got.Settled == 0 {
		t.Fatalf("confirmed item = %+v, want answered with the new quote and time", got)
	}
}

// ids= records one verbatim quote on several items.
func TestApplyGroupedAnswers(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "a")+addXML(ClassTechnical, "t", "b")+addXML(ClassTechnical, "t", "c")+addXML(ClassPolicy, "t", "d", "x", "y"))
	mustApply(t, st, "<wb:answer ids=\"3  1\n2\" value=\"ok\">(01), (02), (03) ok</wb:answer>")
	var orders []int
	for id := 1; id <= 3; id++ {
		got := item(t, st, id)
		if got.Status != StatusAnswered || got.Answer != "ok" || got.AnswerQuote != "(01), (02), (03) ok" || got.AnsweredAt == "" {
			t.Fatalf("item %d = %+v", id, got)
		}
		orders = append(orders, got.Settled)
	}
	// Items settle in the order the ids are written: 3, then 1, then 2.
	if !(orders[2] < orders[0] && orders[0] < orders[1]) {
		t.Fatalf("settle orders for ids 1,2,3 = %v, want 3 before 1 before 2", orders)
	}

	// assumed applies to every id of the group.
	mustApply(t, st, `<wb:patch id="1" status="open"/><wb:patch id="2" status="open"/><wb:answer ids="1 2" value="ok" assumed="true">q</wb:answer>`)
	if item(t, st, 1).Status != StatusAssumed || item(t, st, 2).Status != StatusAssumed {
		t.Fatalf("grouped assumed answers: %+v %+v", item(t, st, 1), item(t, st, 2))
	}

	// One failing id refuses the whole batch, naming that id.
	problems := refused(t, st, `<wb:answer ids="1 4" value="yes">q</wb:answer>`)
	wantProblems(t, problems, "command 1 <wb:answer> (line 1) (04): a policy answer value starts with its option letter (a-b)")
	wantProblems(t, refused(t, st, `<wb:answer ids="1 99 98" value="ok">q</wb:answer>`), "(99): not in the store", "(98): not in the store")

	for want, batch := range map[string]string{
		"takes id or ids, not both":      `<wb:answer id="1" ids="2 3" value="ok">q</wb:answer>`,
		"ids: (02) is listed twice":      `<wb:answer ids="2 3 2" value="ok">q</wb:answer>`,
		`ids: "x" is not a positive num`: `<wb:answer ids="1 x" value="ok">q</wb:answer>`,
		`ids: "0" is not a positive num`: `<wb:answer ids="0" value="ok">q</wb:answer>`,
		"ids: must list at least one id": `<wb:answer ids="  " value="ok">q</wb:answer>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
}

func TestApplyIDFloor(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "one"))
	out := mustApply(t, st, `<wb:floor id="30"/>`+addXML(ClassPolicy, "t", "after floor"))
	if fmt.Sprint(out.IDs) != "[31]" {
		t.Fatalf("IDs after floor 30 = %v, want [31]", out.IDs)
	}
	// A floor at or below the counter is ignored without error.
	out = mustApply(t, st, `<wb:floor id="5"/>`+addXML(ClassPolicy, "t", "low floor"))
	if fmt.Sprint(out.IDs) != "[32]" {
		t.Fatalf("IDs after a low floor = %v, want [32]", out.IDs)
	}
	mustApply(t, st, `<wb:floor id="32"/>`)
	if st.Counter != 32 {
		t.Fatalf("counter = %d, want 32", st.Counter)
	}
	// The floor applies in order: an add ahead of it mints below it.
	out = mustApply(t, st, addXML(ClassPolicy, "t", "before")+`<wb:floor id="60"/>`+addXML(ClassPolicy, "t", "after"))
	if fmt.Sprint(out.IDs) != "[33 61]" {
		t.Fatalf("IDs around a floor = %v, want [33 61]", out.IDs)
	}
	for _, batch := range []string{`<wb:floor/>`, `<wb:floor id="0"/>`, `<wb:floor id="x"/>`} {
		refused(t, st, batch)
	}
}

func settleItems(t *testing.T, st *State, topic string, n int) []int {
	t.Helper()
	var adds strings.Builder
	for i := 0; i < n; i++ {
		adds.WriteString(addXML(ClassTechnical, topic, fmt.Sprintf("item %d", i)) + "\n")
	}
	out := mustApply(t, st, adds.String())
	ids := make([]string, len(out.IDs))
	for i, id := range out.IDs {
		ids[i] = fmt.Sprint(id)
	}
	mustApply(t, st, `<wb:answer ids="`+strings.Join(ids, " ")+`" value="ok">ok</wb:answer>`)
	return out.IDs
}

func TestRotationKeepsTenMostRecentSettledBoardWide(t *testing.T) {
	st := &State{}
	older := settleItems(t, st, "a", 8)
	newer := settleItems(t, st, "b", 6)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	expanded := map[int]bool{}
	collapsed := 0
	for _, topic := range view.Topics {
		for _, iv := range topic.Expanded {
			expanded[iv.ID] = true
		}
		collapsed += len(topic.Collapsed)
	}
	if len(expanded) != ExpandedSettled || collapsed != 4 {
		t.Fatalf("expanded %d collapsed %d, want 10 and 4", len(expanded), collapsed)
	}
	for _, id := range newer {
		if !expanded[id] {
			t.Fatalf("newest item %d must be expanded", id)
		}
	}
	for _, id := range older[:4] {
		if expanded[id] {
			t.Fatalf("oldest item %d must be collapsed", id)
		}
	}
	// Closing a topic collapses it at once; the freed slots go to no one
	// else here because topic b already shows all of its items.
	mustApply(t, st, `<wb:close topic="a"/>`)
	view = BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for _, topic := range view.Topics {
		if topic.Name == "a" && (len(topic.Expanded) != 0 || len(topic.Collapsed) != 8 || !topic.Quiet || !topic.Closed) {
			t.Fatalf("closed topic a = %+v", topic)
		}
	}
	problems := refused(t, st, `<wb:close topic="missing"/>`)
	wantProblems(t, problems, `no item or topic body has topic "missing"`)
	// Closing twice keeps one mark; a topic attribute is required.
	mustApply(t, st, `<wb:close topic="a"/>`)
	if len(st.ClosedTopics) != 1 {
		t.Fatalf("closed topics = %v", st.ClosedTopics)
	}
	wantProblems(t, refused(t, st, `<wb:close/>`), "topic: required")
}

func TestClosedTopicReopensWhenAnItemIsAdded(t *testing.T) {
	st := &State{}
	settleItems(t, st, "a", 2)
	mustApply(t, st, `<wb:close topic="a"/>`)
	if !st.topicClosed("a") {
		t.Fatal("topic a should be closed")
	}
	mustApply(t, st, addXML(ClassTechnical, "a", "new"))
	if st.topicClosed("a") {
		t.Fatalf("an added item reopens its topic: %v", st.ClosedTopics)
	}
	// A rejected add reopens nothing.
	mustApply(t, st, `<wb:close topic="a"/>`)
	refused(t, st, `<wb:add class="bad" topic="a"><wb:title>x</wb:title></wb:add>`)
	if !st.topicClosed("a") {
		t.Fatalf("a refused add must not reopen the topic")
	}
}

func TestArchiveCapEvictsOldestSettled(t *testing.T) {
	st := &State{}
	first := settleItems(t, st, "a", 300)
	settleItems(t, st, "b", 210)
	mustApply(t, st, addXML(ClassPolicy, "c", "still open"))
	settled := 0
	for _, it := range st.Items {
		if IsSettled(it.Status) {
			settled++
		}
	}
	if settled != ArchiveCap {
		t.Fatalf("settled kept = %d, want %d", settled, ArchiveCap)
	}
	for _, id := range first[:10] {
		if st.index(id) >= 0 {
			t.Fatalf("oldest settled item %d must be evicted", id)
		}
	}
	if st.index(first[10]) < 0 {
		t.Fatalf("item %d must survive the cap", first[10])
	}
	// Eviction never lowers the counter, so a minted ID never repeats.
	out := mustApply(t, st, addXML(ClassPolicy, "c", "next"))
	if out.IDs[0] != 512 {
		t.Fatalf("next minted ID = %d, want 512", out.IDs[0])
	}
}

func TestQueryReturnsArchivedAndFilters(t *testing.T) {
	st := &State{}
	settleItems(t, st, "a", 12)
	mustApply(t, st, addXML(ClassPolicy, "b", "open one"))
	all, _, problems := Query(st, nil)
	if len(problems) > 0 || len(all) != 13 {
		t.Fatalf("query all = %d items, problems %v", len(all), problems)
	}
	// Item 1 is collapsed on the page but still returned.
	byID, _, _ := Query(st, queryPayload(t, `{"id":1}`))
	if len(byID) != 1 || byID[0].Status != StatusAnswered {
		t.Fatalf("query id 1 = %+v", byID)
	}
	open, _, _ := Query(st, queryPayload(t, `{"status":"open","topic":"b"}`))
	if len(open) != 1 || open[0].ID != 13 {
		t.Fatalf("query open b = %+v", open)
	}
	if _, _, problems := Query(st, queryPayload(t, `{"status":"done"}`)); !hasProblem(problems, "payload.status: must be one of") {
		t.Fatalf("bad status problems = %v", problems)
	}
	if _, _, problems := Query(st, queryPayload(t, `{"bogus":1}`)); !hasProblem(problems, "payload.bogus: unknown field") {
		t.Fatalf("unknown field problems = %v", problems)
	}
	if _, _, problems := Query(st, `{}`); !hasProblem(problems, "payload: must be an object; got string") {
		t.Fatalf("string query payload problems = %v", problems)
	}
	if !strings.Contains(FormatItems(byID, ""), "(01) [answered] [technical] a: item 0") {
		t.Fatalf("formatted = %s", FormatItems(byID, ""))
	}
	if got := FormatItems(nil, ""); got != "no matching items\n" {
		t.Fatalf("empty result = %q", got)
	}
}

// A query that names a topic also returns that topic's stored body, so a
// lead can quote it in a wb:edit.
func TestQueryWithTopicReturnsItsBody(t *testing.T) {
	st := &State{}
	body := "## 배경\n\n<details><summary>x</summary>본문 & more</details>"
	mustApply(t, st, addXML(ClassTechnical, "with body", "an item")+addXML(ClassTechnical, "other", "other item")+topicXML("with body", "\n"+body+"\n")+topicXML("only body", "just notes"))

	items, got, problems := Query(st, queryPayload(t, `{"topic":"with body"}`))
	if len(problems) > 0 || len(items) != 1 || got != body {
		t.Fatalf("query topic = %v, %q, %v", items, got, problems)
	}
	text := FormatItems(items, got)
	if !strings.HasPrefix(text, "topic body:\n<<<\n"+body+"\n>>>\n") || !strings.Contains(text, "an item") {
		t.Fatalf("formatted = %q", text)
	}
	// A body-only topic has no item to return but still returns its body.
	items, got, _ = Query(st, queryPayload(t, `{"topic":"only body"}`))
	if len(items) != 0 || got != "just notes" {
		t.Fatalf("query body-only topic = %v, %q", items, got)
	}
	if text := FormatItems(items, got); text != "topic body:\n<<<\njust notes\n>>>\nno matching items\n" {
		t.Fatalf("formatted = %q", text)
	}
	// A topic without a body, an unfiltered query, and a nil state return no body.
	if _, got, _ := Query(st, queryPayload(t, `{"topic":"other"}`)); got != "" {
		t.Fatalf("body of a topic without one = %q", got)
	}
	if _, got, _ := Query(st, queryPayload(t, `{"status":"open"}`)); got != "" {
		t.Fatalf("an unfiltered query returned a body: %q", got)
	}
	if items, got, problems := Query(nil, queryPayload(t, `{"topic":"x"}`)); len(items) != 0 || got != "" || len(problems) > 0 {
		t.Fatalf("nil state query = %v %q %v", items, got, problems)
	}
	if strings.Contains(FormatItems(items, ""), "topic body") {
		t.Fatal("no body, no body block")
	}
}

// A stored state written while the board had a Now slot still loads; the
// retired fields are dropped on the next write.
func TestStoredStateWithRetiredNowFieldsLoads(t *testing.T) {
	stored := `{"counter":3,"seq":4,"now":"<p>x</p>","now_seq":4,"update_seq":4,"items":[{"id":3,"class":"technical","topic":"t","title":"kept","status":"open","changed":4}]}`
	var st State
	if err := json.Unmarshal([]byte(stored), &st); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if st.Counter != 3 || len(st.Items) != 1 {
		t.Fatalf("state = %+v", st)
	}
	retired := func(s *State) []string {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var keys map[string]json.RawMessage
		if err := json.Unmarshal(data, &keys); err != nil {
			t.Fatal(err)
		}
		var found []string
		for _, k := range []string{"now", "now_seq", "update_seq"} {
			if _, ok := keys[k]; ok {
				found = append(found, k)
			}
		}
		return found
	}
	if got := retired(&st); len(got) != 0 {
		t.Fatalf("re-marshalled state still holds %v", got)
	}
	mustApply(t, &st, addXML(ClassTechnical, "t", "next"))
	if got := retired(&st); len(got) != 0 || st.Items[1].ID != 4 {
		t.Fatalf("after an update: retired %v, items %+v", got, st.Items)
	}
}

func TestApplyEmbedDir(t *testing.T) {
	st := &State{}
	// Absolute paths come from t.TempDir so the test holds on Windows, where
	// a slash-rooted path such as /host/scratch is not absolute.
	base := t.TempDir()
	scratch := filepath.Join(base, "scratch")
	other := filepath.Join(base, "other")
	missing := filepath.Join(base, "missing")
	embedDir := func(dir string) string { return `<wb:embed-dir path="` + dir + `"/>` }
	dirs := map[string]bool{scratch: true, other: true}
	env := testEnv()
	env.IsDir = func(path string) bool { return dirs[path] }
	apply := func(commands string) []string {
		_, problems := Apply(st, commands, env)
		return problems
	}
	if problems := apply(`<wb:embed-dir path="relative/dir"/>`); !hasProblem(problems, "must be an absolute path") {
		t.Fatalf("relative embed-dir problems = %v", problems)
	}
	if problems := apply(embedDir(missing)); !hasProblem(problems, "not an existing directory") {
		t.Fatalf("missing embed-dir problems = %v", problems)
	}
	// The path attribute is required; an empty one clears the setting.
	if problems := apply(`<wb:embed-dir/>`); !hasProblem(problems, `command 1 <wb:embed-dir> (line 1) path: required ("" clears it)`) {
		t.Fatalf("pathless embed-dir problems = %v", problems)
	}
	if problems := apply(embedDir(scratch)); len(problems) > 0 {
		t.Fatalf("embed-dir refused: %v", problems)
	}
	if st.EmbedDir != scratch {
		t.Fatalf("embed-dir must be remembered: %q", st.EmbedDir)
	}
	// Setting another existing folder replaces it.
	if problems := apply(embedDir(other)); len(problems) > 0 || st.EmbedDir != other {
		t.Fatalf("replacing embed-dir: problems %v dir %q", problems, st.EmbedDir)
	}
	// The stored folder is re-checked by every later update, refusing the
	// whole batch, even one that touches nothing else.
	delete(dirs, other)
	before, _ := json.Marshal(st)
	if problems := apply(addXML(ClassTechnical, "t", "x")); !hasProblem(problems, other+" (stored for this session) is no longer an existing directory") {
		t.Fatalf("vanished embed-dir problems = %v", problems)
	}
	if after, _ := json.Marshal(st); string(after) != string(before) {
		t.Fatalf("the refused batch changed the state:\n%s\n%s", before, after)
	}
	if problems := apply(``); !hasProblem(problems, "send <wb:embed-dir") {
		t.Fatalf("an empty batch must re-check the stored folder: %v", problems)
	}
	// Re-pointing or clearing it in the same update is accepted.
	if problems := apply(embedDir(scratch) + addXML(ClassTechnical, "t", "x")); len(problems) > 0 || st.EmbedDir != scratch || len(st.Items) != 1 {
		t.Fatalf("re-pointing embed-dir: problems %v dir %q", problems, st.EmbedDir)
	}
	delete(dirs, scratch)
	if problems := apply(`<wb:embed-dir path=""/>`); len(problems) > 0 || st.EmbedDir != "" {
		t.Fatalf("clearing embed-dir: problems %v dir %q", problems, st.EmbedDir)
	}
}

func TestApplyAcceptsAnyItemReference(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "old"))
	mustApply(t, st, `<wb:add class="policy" topic="t"><wb:title>new</wb:title><wb:link>supersedes {{01}}</wb:link><wb:link>261030-feat-x</wb:link></wb:add>
<wb:patch id="1" status="superseded"><wb:link>superseded by {{02}}</wb:link></wb:patch>`)
	// References are display only: an ID the store never held, prose
	// parentheses, and non-digit brace tokens are all accepted.
	mustApply(t, st, `<wb:add class="technical" topic="t"><wb:title>see {{77}} after step (2)</wb:title><wb:why>step (3) then {{my-tag}}</wb:why><wb:link>see {{77}}</wb:link><wb:link>step (2)</wb:link></wb:add>`)
	if got := st.Items[2].Links; len(got) != 2 || got[0] != "see {{77}}" || got[1] != "step (2)" {
		t.Fatalf("links stored as %q", got)
	}
	if got := item(t, st, 1).Links; len(got) != 1 || got[0] != "superseded by {{02}}" {
		t.Fatalf("patched links stored as %q", got)
	}
}

func TestApplyEmptyCommandsOnlyAdvanceRender(t *testing.T) {
	st := &State{}
	mustApply(t, st, ``)
	mustApply(t, st, " \n\t ")
	if st.Seq != 2 || len(st.Items) != 0 {
		t.Fatalf("state after empty updates = %+v", st)
	}
	// A refused batch advances nothing.
	refused(t, st, `<wb:nope/>`)
	if st.Seq != 2 {
		t.Fatalf("Seq after a refused batch = %d", st.Seq)
	}
}

func TestBuildViewTopicOrderAndEmbedFreshness(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "settled", "s")+
		addXML(ClassTechnical, "acks", "k")+
		addXML(ClassPolicy, "assumed", "m", "x", "y")+
		addXML(ClassPolicy, "policy", "p", "x", "y")+
		`<wb:answer id="1" value="ok">ok</wb:answer><wb:answer id="3" value="a" assumed="true">maybe</wb:answer>`)
	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	var names []string
	for _, topic := range view.Topics {
		names = append(names, topic.Name)
	}
	if strings.Join(names, ",") != "policy,assumed,acks,settled" {
		t.Fatalf("topic order = %v", names)
	}
	if !view.BulkAck || !view.Answerable || view.StampFile != "r.stamp.js" {
		t.Fatalf("view flags = %+v", view)
	}
	// Mark the first embed, then change one item: only it is new.
	st.LastEmbedSeq = st.Seq
	mustApply(t, st, `<wb:answer id="2" value="ok">ok</wb:answer>`)
	embed := BuildView(st, ViewInput{Layout: LayoutEmbed, ReplyID: "r"})
	fresh := 0
	for _, topic := range embed.Topics {
		fresh += topic.Fresh
		for _, iv := range topic.Expanded {
			if iv.ID != 2 || !iv.New {
				t.Fatalf("embed expanded only the new settled item: %+v", iv)
			}
		}
		if topic.Name == "settled" && (!topic.Quiet || len(topic.Collapsed) != 1) {
			t.Fatalf("unchanged settled topic must be quiet in the embed: %+v", topic)
		}
	}
	if fresh != 1 || embed.StampFile != "" {
		t.Fatalf("embed fresh = %d stamp %q", fresh, embed.StampFile)
	}
}

func mustMarshal(t *testing.T, st *State) []byte {
	t.Helper()
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func topicEntry(t *testing.T, st *State, name string) TopicDoc {
	t.Helper()
	i := st.topicDocIndex(name)
	if i < 0 {
		t.Fatalf("topic %q has no body", name)
	}
	return st.Topics[i]
}

func TestApplyTopicSetsReplacesAndClearsABody(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("notes", "\n## 첫 본문\n\n내용\n"))
	got := topicEntry(t, st, "notes")
	if got.Body != "## 첫 본문\n\n내용" || got.Changed != st.Seq {
		t.Fatalf("topic = %+v (Seq %d)", got, st.Seq)
	}
	// A second wb:topic replaces the body whole.
	mustApply(t, st, topicXML("notes", "replacement"))
	if got := topicEntry(t, st, "notes"); got.Body != "replacement" || got.Changed != st.Seq || len(st.Topics) != 1 {
		t.Fatalf("topic = %+v topics %d", got, len(st.Topics))
	}
	// Topics keep first-set order.
	mustApply(t, st, topicXML("second", "two")+topicXML("notes", "again"))
	if st.Topics[0].Name != "notes" || st.Topics[1].Name != "second" {
		t.Fatalf("topics = %+v", st.Topics)
	}
	// The name is required.
	wantProblems(t, refused(t, st, `<wb:topic>x</wb:topic>`), "command 1 <wb:topic> (line 1) name: required")
	wantProblems(t, refused(t, st, `<wb:topic name=" ">x</wb:topic>`), "name: must be non-empty")
	wantProblems(t, refused(t, st, `<wb:topic name="n" mode="x">x</wb:topic>`), "unknown attribute mode (known: name)")
}

func TestApplyTopicSameBodyIsNoChange(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "same"))
	first := topicEntry(t, st, "t").Changed
	mustApply(t, st, `<wb:floor id="1"/>`) // an update that touches nothing else
	mustApply(t, st, topicXML("t", "\nsame\n"))
	if got := topicEntry(t, st, "t"); got.Changed != first || st.Seq == first {
		t.Fatalf("the same body again bumped Changed to %d (was %d, Seq %d)", got.Changed, first, st.Seq)
	}
	// A no-op does not reopen a closed topic either; a real change does.
	mustApply(t, st, `<wb:close topic="t"/>`)
	mustApply(t, st, topicXML("t", "same"))
	if !st.topicClosed("t") {
		t.Fatal("setting the same body reopened the topic")
	}
	mustApply(t, st, topicXML("t", "different"))
	if st.topicClosed("t") || topicEntry(t, st, "t").Changed != st.Seq {
		t.Fatalf("a changed body must reopen the topic: closed %v", st.ClosedTopics)
	}
}

func TestApplyTopicEmptyBodyClears(t *testing.T) {
	for name, clear := range map[string]string{
		"empty":        `<wb:topic name="t"></wb:topic>`,
		"self-closing": `<wb:topic name="t"/>`,
		"blank":        "<wb:topic name=\"t\">\n  \n</wb:topic>",
	} {
		st := &State{}
		mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "body"))
		mustApply(t, st, clear)
		if st.TopicBody("t") != "" || len(st.Topics) != 0 {
			t.Fatalf("%s: topics = %+v", name, st.Topics)
		}
		if len(st.Items) != 1 {
			t.Fatalf("%s: clearing a body keeps the topic's items", name)
		}
		// Clearing again, and clearing a topic that never had a body, is no problem.
		mustApply(t, st, clear)
		mustApply(t, st, `<wb:topic name="never"/>`)
		if len(st.Topics) != 0 {
			t.Fatalf("%s: topics = %+v", name, st.Topics)
		}
	}
	// A topic with items stays closed when its body is cleared; a topic left
	// with neither drops its closed mark, and is unknown again.
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "has items", "x")+topicXML("has items", "b")+topicXML("only body", "b")+`<wb:close topic="has items"/><wb:close topic="only body"/>`)
	mustApply(t, st, `<wb:topic name="has items"/><wb:topic name="only body"/>`)
	if !st.topicClosed("has items") || st.topicClosed("only body") || len(st.ClosedTopics) != 1 {
		t.Fatalf("closed topics = %v", st.ClosedTopics)
	}
	if st.topicKnown("only body") {
		t.Fatal("a topic with neither items nor body must be gone")
	}
	wantProblems(t, refused(t, st, `<wb:close topic="only body"/>`), `no item or topic body has topic "only body"`)
	// A body set again afterwards starts open.
	mustApply(t, st, topicXML("only body", "back"))
	if st.topicClosed("only body") {
		t.Fatal("a recreated topic must not come back closed")
	}
}

func TestApplyTopicRefusesAFullDocument(t *testing.T) {
	st := &State{}
	for _, doc := range []string{`<!DOCTYPE html><p>x</p>`, `<html><body>x</body></html>`, `<HTML lang=en>`,
		"# Title\n\n<!DOCTYPE html>\n<p>x</p>", "<!doctype html>\n<html><body>x</body></html>", "Some text <html lang=en> inline."} {
		problems := refused(t, st, topicXML("t", "\n"+doc+"\n"))
		wantProblems(t, problems, "command 1 <wb:topic> (line 1): a topic body takes markdown with inline HTML, not a full document")
	}
	// A body that only mentions a document tag in code (or in text goldmark
	// escapes, such as a lowercase doctype alone) is markdown, not a full
	// document.
	for _, doc := range []string{"Wrap it in `<html>` and start with `<!DOCTYPE html>`.",
		"Example:\n\n```html\n<!DOCTYPE html>\n<html lang=en><body></body></html>\n```\n",
		"Indented:\n\n    <html>\n", "Text <!doctype html> escaped."} {
		other := &State{}
		mustApply(t, other, topicXML("t", "\n"+doc+"\n"))
		if other.TopicBody("t") != doc {
			t.Fatalf("code-only mention %q stored as %q", doc, other.TopicBody("t"))
		}
	}
}

func TestApplyEditReplacesOneUniqueMatch(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "\n## 배경\n본문은 닫는 태그까지 raw 텍스트다.\n\n## 결정\nXML로 간다.\n"))
	before := topicEntry(t, st, "t").Changed
	mustApply(t, st, `<wb:edit topic="t">
<wb:old>## 배경</wb:old>
<wb:new>## 배경과 동기</wb:new>
</wb:edit>`)
	want := "## 배경과 동기\n본문은 닫는 태그까지 raw 텍스트다.\n\n## 결정\nXML로 간다."
	if got := topicEntry(t, st, "t"); got.Body != want || got.Changed != st.Seq || got.Changed == before {
		t.Fatalf("topic = %+v", got)
	}
	// Whole lines match as written on their own lines: the one newline at
	// each end of wb:old and wb:new is the tag layout, the rest is text.
	mustApply(t, st, "<wb:edit topic=\"t\"><wb:old>\nXML로 간다.\n</wb:old><wb:new>\nXML로 간다.\n\n## 후속\n</wb:new></wb:edit>")
	if got := st.TopicBody("t"); !strings.HasSuffix(got, "XML로 간다.\n\n## 후속") {
		t.Fatalf("body = %q", got)
	}
	// Raw text in old and new, markup included.
	mustApply(t, st, `<wb:edit topic="t"><wb:old>## 후속</wb:old><wb:new><details><option>a & b</option></details></wb:new></wb:edit>`)
	if got := st.TopicBody("t"); !strings.HasSuffix(got, "<details><option>a & b</option></details>") {
		t.Fatalf("body = %q", got)
	}
}

func TestApplyEditInsertsThroughAnAnchor(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "## A\n\n## C"))
	// Insert after an anchor, then before one: the anchor sits inside wb:old.
	mustApply(t, st, `<wb:edit topic="t"><wb:old>## A</wb:old><wb:new>## A

## B</wb:new></wb:edit>`)
	mustApply(t, st, `<wb:edit topic="t"><wb:old>## C</wb:old><wb:new>## C2
## C</wb:new></wb:edit>`)
	if got := st.TopicBody("t"); got != "## A\n\n## B\n\n## C2\n## C" {
		t.Fatalf("body = %q", got)
	}
}

func TestApplyEditRefusals(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "alpha beta alpha"))
	before := st.TopicBody("t")
	for want, batch := range map[string]string{
		`command 1 <wb:edit> (line 1): <wb:old> matches nothing in topic "t"'s body`:               `<wb:edit topic="t"><wb:old>gamma</wb:old><wb:new>x</wb:new></wb:edit>`,
		`<wb:old> matches 2 places in topic "t"'s body; widen it to one or set replace_all="true"`: `<wb:edit topic="t"><wb:old>alpha</wb:old><wb:new>x</wb:new></wb:edit>`,
		`topic "other" has no body; set one with <wb:topic>`:                                       `<wb:edit topic="other"><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`,
		`topic: required`:                                   `<wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`,
		`<wb:old>: required`:                                `<wb:edit topic="t"><wb:new>b</wb:new></wb:edit>`,
		`<wb:old>: must be non-empty`:                       `<wb:edit topic="t"><wb:old></wb:old><wb:new>b</wb:new></wb:edit>`,
		`<wb:new>: required (empty deletes the match)`:      `<wb:edit topic="t"><wb:old>alpha</wb:old></wb:edit>`,
		`more than one <wb:old>`:                            `<wb:edit topic="t"><wb:old>a</wb:old><wb:old>b</wb:old><wb:new>b</wb:new></wb:edit>`,
		`replace_all: must be "true" or "false"; got "yes"`: `<wb:edit topic="t" replace_all="yes"><wb:old>alpha</wb:old><wb:new>x</wb:new></wb:edit>`,
		`the edited body would hold a full document`:        `<wb:edit topic="t"><wb:old>beta</wb:old><wb:new><!DOCTYPE html></wb:new></wb:edit>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
	// An edit on a topic that has items but no body is refused the same way.
	mustApply(t, st, addXML(ClassTechnical, "items only", "x"))
	wantProblems(t, refused(t, st, `<wb:edit topic="items only"><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`), `topic "items only" has no body`)
	// A failed edit leaves the body as it was, whatever else the batch holds.
	if st.TopicBody("t") != before {
		t.Fatalf("body = %q", st.TopicBody("t"))
	}
	// An edit inside a batch sees the bodies the commands ahead of it wrote.
	wantProblems(t, refused(t, st, `<wb:edit topic="new"><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`+topicXML("new", "a")), `topic "new" has no body`)
	mustApply(t, st, topicXML("new", "a")+`<wb:edit topic="new"><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`)
	if st.TopicBody("new") != "b" {
		t.Fatalf("body = %q", st.TopicBody("new"))
	}
}

func TestApplyEditReplaceAllAndDelete(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "alpha beta alpha gamma alpha"))
	mustApply(t, st, `<wb:edit topic="t" replace_all="true"><wb:old>alpha</wb:old><wb:new>A</wb:new></wb:edit>`)
	if got := st.TopicBody("t"); got != "A beta A gamma A" {
		t.Fatalf("body = %q", got)
	}
	// replace_all="false" is the default; one match is fine either way.
	mustApply(t, st, `<wb:edit topic="t" replace_all="false"><wb:old>gamma</wb:old><wb:new>G</wb:new></wb:edit>`)
	// An empty wb:new deletes the match, self-closing or not.
	mustApply(t, st, `<wb:edit topic="t"><wb:old> beta</wb:old><wb:new/></wb:edit>`)
	mustApply(t, st, `<wb:edit topic="t"><wb:old> G</wb:old><wb:new></wb:new></wb:edit>`)
	if got := st.TopicBody("t"); got != "A A A" {
		t.Fatalf("body = %q", got)
	}
	// An edit that leaves nothing clears the body like an empty wb:topic.
	mustApply(t, st, `<wb:edit topic="t" replace_all="true"><wb:old>A</wb:old><wb:new/></wb:edit>`)
	if len(st.Topics) != 0 {
		t.Fatalf("topics = %+v", st.Topics)
	}
}

func TestApplyEditAndTopicReopenAClosedTopic(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "alpha")+`<wb:close topic="t"/>`)
	mustApply(t, st, `<wb:edit topic="t"><wb:old>alpha</wb:old><wb:new>beta</wb:new></wb:edit>`)
	if st.topicClosed("t") {
		t.Fatal("an edit that changes the body reopens the topic")
	}
	mustApply(t, st, `<wb:close topic="t"/>`+topicXML("t", "gamma"))
	if st.topicClosed("t") {
		t.Fatal("a wb:topic that changes the body reopens the topic")
	}
	// Closing after the change in the same batch keeps it closed.
	mustApply(t, st, topicXML("t", "delta")+`<wb:close topic="t"/>`)
	if !st.topicClosed("t") {
		t.Fatal("close after a body change must stick")
	}
	// A body-only topic closes the same way.
	mustApply(t, st, topicXML("doc", "x")+`<wb:close topic="doc"/>`)
	mustApply(t, st, topicXML("doc", "y"))
	if st.topicClosed("doc") {
		t.Fatal("a body-only topic reopens on change")
	}
}

func TestApplyCloseAcceptsAContentOnlyTopic(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("notes", "only a body"))
	if len(st.Items) != 0 || !st.topicKnown("notes") {
		t.Fatalf("state = %+v", st)
	}
	mustApply(t, st, `<wb:close topic="notes"/>`)
	if !st.topicClosed("notes") {
		t.Fatal("wb:close must accept a topic that has a body and no items")
	}
}
