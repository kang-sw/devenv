package wswhiteboard

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
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

// addXML is a wb:add command in its topic's group: title, then the options
// when given.
func addXML(class, topic, title string, options ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<wb:topic name=%q><wb:add class=%q><wb:title>%s</wb:title>`, topic, class, title)
	for _, o := range options {
		fmt.Fprintf(&sb, `<wb:option>%s</wb:option>`, o)
	}
	sb.WriteString(`</wb:add></wb:topic>`)
	return sb.String()
}

// topicXML sets a topic's body.
func topicXML(name, body string) string {
	return `<wb:topic name="` + name + `"><wb:body>` + body + `</wb:body></wb:topic>`
}

func TestApplyBatchInOrderReturnsIDsInAddOrder(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `
<wb:topic name="t"><wb:add class="policy"><wb:title>first</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>b: safer</wb:recommend></wb:add></wb:topic>
<wb:topic name="t"><wb:add class="technical"><wb:title>second</wb:title><wb:why>because</wb:why></wb:add></wb:topic>
<wb:topic name="u"><wb:add class="announced"><wb:title>third</wb:title></wb:add></wb:topic>
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
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`+addXML(ClassTechnical, "t", "y"))
	if st.topicClosed("t") {
		t.Fatalf("close then add must leave the topic open: %v", st.ClosedTopics)
	}
	mustApply(t, st, addXML(ClassTechnical, "t", "z")+`<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	if !st.topicClosed("t") {
		t.Fatalf("add then close must leave the topic closed")
	}

	// A patch that reopens an item, then one that edits its options, in one batch.
	mustApply(t, st, `<wb:topic name="p"><wb:add class="policy"><wb:title>p</wb:title><wb:option>a1</wb:option><wb:option>b1</wb:option></wb:add></wb:topic>
<wb:answer id="4" value="a">q</wb:answer>
<wb:patch id="4" status="open"/>
<wb:patch id="4"><wb:option>only</wb:option></wb:patch>`)
	if got := item(t, st, 4); got.Status != StatusOpen || len(got.Options) != 1 || got.Answer != "" {
		t.Fatalf("item 4 = %+v", got)
	}
}

func TestApplyExplicitIDAdoptsAndRaisesCounter(t *testing.T) {
	st := &State{}
	out := mustApply(t, st, `<wb:topic name="q"><wb:add id="40" class="policy"><wb:title>adopted</wb:title></wb:add></wb:topic>`+addXML(ClassTechnical, "q", "minted"))
	if fmt.Sprint(out.IDs) != "[40 41]" {
		t.Fatalf("IDs = %v, want [40 41]", out.IDs)
	}
	// A lower explicit ID not yet in the store is adopted without lowering the counter.
	out = mustApply(t, st, `<wb:topic name="q"><wb:add id="7" class="policy"><wb:title>older</wb:title></wb:add></wb:topic>`+addXML(ClassPolicy, "q", "next"))
	if fmt.Sprint(out.IDs) != "[7 42]" {
		t.Fatalf("IDs = %v, want [7 42]", out.IDs)
	}
	problems := refused(t, st, `<wb:topic name="q"><wb:add id="40" class="policy"><wb:title>dup</wb:title></wb:add></wb:topic>`)
	wantProblems(t, problems, "id: (40) is already in the store")
	// Within one batch an adopted ID is taken for the commands after it.
	problems = refused(t, st, `<wb:topic name="q"><wb:add id="50" class="policy"><wb:title>a</wb:title></wb:add></wb:topic><wb:topic name="q"><wb:add id="50" class="policy"><wb:title>b</wb:title></wb:add></wb:topic>`)
	wantProblems(t, problems, "command 2 <wb:add> (line 1) id: (50) is already in the store")
	for _, bad := range []string{"(43)", "0", "-3", "1.5", ""} {
		problems = refused(t, st, fmt.Sprintf(`<wb:topic name="q"><wb:add id=%q class="policy"><wb:title>x</wb:title></wb:add></wb:topic>`, bad))
		wantProblems(t, problems, "id: must be a positive whole number")
	}
}

func TestApplyRefusedBatchAppliesNothingAndReportsEveryProblem(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassPolicy, "t", "one"))
	problems := refused(t, st, `<wb:topic name="t"><wb:add class="wrong"><wb:title>x</wb:title></wb:add></wb:topic>
<wb:add class="policy"><wb:title>no topic</wb:title></wb:add>
<wb:answer id="99" value="a">q</wb:answer>
<wb:patch id="1" status="answered"/>
<wb:bogus/>`)
	wantProblems(t, problems,
		`command 1 <wb:add> (line 1) class: must be one of policy, technical, announced`,
		`command 2 <wb:add> (line 2): goes inside a <wb:topic name="..."> group`,
		`command 3 <wb:answer> (line 3) (99): not in the store`,
		`command 4 <wb:patch> (line 4) status: must be one of open, withdrawn, superseded`,
		`command 5 <wb:bogus> (line 5): unknown command`,
	)
	if st.Counter != 1 || len(st.Items) != 1 {
		t.Fatalf("refused batch changed the store: %+v", st)
	}
	// A valid command ahead of a bad one is not applied either.
	problems = refused(t, st, addXML(ClassTechnical, "t", "fine")+"\n"+`<wb:topic name="nowhere"><wb:close result="done"/></wb:topic>`)
	wantProblems(t, problems, `command 2 <wb:close> (line 2): no item or topic body has topic "nowhere"`)
}

func TestApplyAddFieldRules(t *testing.T) {
	st := &State{}
	for want, batch := range map[string]string{
		"<wb:title>: required":                        `<wb:topic name="t"><wb:add class="policy"/></wb:topic>`,
		"<wb:title>: must be non-empty":               `<wb:topic name="t"><wb:add class="policy"><wb:title> </wb:title></wb:add></wb:topic>`,
		"more than one <wb:title>":                    `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:title>b</wb:title></wb:add></wb:topic>`,
		"more than one <wb:why>":                      `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:why>x</wb:why><wb:why>y</wb:why></wb:add></wb:topic>`,
		"more than one <wb:recommend>":                `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:option>x</wb:option><wb:recommend>a</wb:recommend><wb:recommend>a</wb:recommend></wb:add></wb:topic>`,
		"option b must be non-empty":                  `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:option>x</wb:option><wb:option>  </wb:option></wb:add></wb:topic>`,
		"class: required":                             `<wb:topic name="t"><wb:add><wb:title>a</wb:title></wb:add></wb:topic>`,
		"<wb:topic> (line 1) name: must be non-empty": `<wb:topic name=""><wb:add class="policy"><wb:title>a</wb:title></wb:add></wb:topic>`,
		"<wb:recommend>: names an option letter":      `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:recommend>a</wb:recommend></wb:add></wb:topic>`,
		"starts with its option letter (a-b)":         `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>c: no such</wb:recommend></wb:add></wb:topic>`,
		"at most 26 options":                          `<wb:topic name="t"><wb:add class="policy"><wb:title>a</wb:title>` + strings.Repeat(`<wb:option>o</wb:option>`, 27) + `</wb:add></wb:topic>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
	// Links are repeatable and keep their order and text.
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>l</wb:title><wb:link>one</wb:link><wb:link>two</wb:link></wb:add></wb:topic>`)
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
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>p</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:recommend>a</wb:recommend><wb:why>old why</wb:why><wb:link>old</wb:link></wb:add></wb:topic>`)
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
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>p</wb:title><wb:option>x</wb:option><wb:option>y</wb:option><wb:link>a</wb:link><wb:link>b</wb:link></wb:add></wb:topic>`)
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
	// An assumed item takes a settle order, so the page rotates it with the
	// settled items.
	got := item(t, st, 1)
	if got.Status != StatusAssumed || got.Settled == 0 {
		t.Fatalf("assumed item = %+v", got)
	}
	assumedAt := got.Settled
	later := testEnv()
	later.Now = testNow.Add(time.Hour)
	if _, problems := Apply(st, `<wb:answer id="1" value="a">(01) ok</wb:answer>`, later); len(problems) > 0 {
		t.Fatalf("confirm refused: %v", problems)
	}
	got = item(t, st, 1)
	if got.Status != StatusAnswered || got.AnswerQuote != "(01) ok" || got.AnsweredAt != testNow.Add(time.Hour).Format(time.RFC3339) || got.Settled <= assumedAt {
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
	mustApply(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`)
	view = BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	for _, topic := range view.Topics {
		if topic.Name == "a" && (len(topic.Expanded) != 0 || len(topic.Collapsed) != 8 || !topic.Quiet || !topic.Closed) {
			t.Fatalf("closed topic a = %+v", topic)
		}
	}
	problems := refused(t, st, `<wb:topic name="missing"><wb:close result="done"/></wb:topic>`)
	wantProblems(t, problems, `no item or topic body has topic "missing"`)
	// Closing twice keeps one mark; a close sits in its topic's group.
	mustApply(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`)
	if len(st.ClosedTopics) != 1 {
		t.Fatalf("closed topics = %v", st.ClosedTopics)
	}
	wantProblems(t, refused(t, st, `<wb:close result="done"/>`), `command 1 <wb:close> (line 1): goes inside a <wb:topic name="..."> group`)
}

// An assumed item stored before assumed items took a settle order keeps
// Settled 0; it rotates as the newest settled row, never the oldest, and
// two such items order by Changed.
func TestLegacyAssumedItemRotatesAsNewest(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "a", "first legacy")+addXML(ClassTechnical, "a", "second legacy"))
	mustApply(t, st, `<wb:answer id="1" value="guess" assumed="true">maybe</wb:answer>`)
	mustApply(t, st, `<wb:answer id="2" value="guess" assumed="true">maybe</wb:answer>`)
	for i := range st.Items {
		st.Items[i].Settled = 0 // as stored before this release
	}
	settleItems(t, st, "a", ExpandedSettled+2)

	view := BuildView(st, ViewInput{Layout: LayoutFull, ReplyID: "r"})
	var expanded []int
	for _, topic := range view.Topics {
		for _, iv := range topic.Expanded {
			expanded = append(expanded, iv.ID)
		}
	}
	if len(expanded) != ExpandedSettled || expanded[0] != 2 || expanded[1] != 1 {
		t.Fatalf("expanded = %v, want the legacy assumed items 2 then 1 first of %d", expanded, ExpandedSettled)
	}
}

func TestClosedTopicReopensWhenAnItemIsAdded(t *testing.T) {
	st := &State{}
	settleItems(t, st, "a", 2)
	mustApply(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`)
	if !st.topicClosed("a") {
		t.Fatal("topic a should be closed")
	}
	mustApply(t, st, addXML(ClassTechnical, "a", "new"))
	if st.topicClosed("a") {
		t.Fatalf("an added item reopens its topic: %v", st.ClosedTopics)
	}
	// A rejected add reopens nothing.
	mustApply(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic>`)
	refused(t, st, `<wb:topic name="a"><wb:add class="bad"><wb:title>x</wb:title></wb:add></wb:topic>`)
	if !st.topicClosed("a") {
		t.Fatalf("a refused add must not reopen the topic")
	}
}

// Closing a topic withdraws its open items, settled now as a patch to
// withdrawn would; assumed (already read as settled), answered, superseded,
// and other topics' items stay; a reopen does not restore them; a refused
// batch withdraws nothing.
func TestCloseWithdrawsTheTopicsOpenItems(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "a", "open")+addXML(ClassTechnical, "a", "assumed")+
		addXML(ClassTechnical, "a", "answered")+addXML(ClassTechnical, "a", "superseded")+addXML(ClassTechnical, "b", "elsewhere"))
	mustApply(t, st, `<wb:answer id="2" value="guess" assumed="true">maybe</wb:answer><wb:answer id="3" value="ok">ok</wb:answer><wb:patch id="4" status="superseded"/>`)

	// A refused batch carrying the close changes no item and closes nothing.
	refused(t, st, `<wb:topic name="a"><wb:close result="done"/></wb:topic><wb:patch id="99" status="open"/>`)

	prevSeq := st.SettleSeq
	assumed := item(t, st, 2)
	out := mustApply(t, st, `<wb:topic name="a"><wb:close result="Done."/></wb:topic>`)
	if !st.topicClosed("a") {
		t.Fatal("topic a should be closed")
	}
	if it := item(t, st, 1); it.Status != StatusWithdrawn || it.Changed != st.Seq || it.Settled <= prevSeq || it.ChangedAt == "" {
		t.Fatalf("item 1 after close = %+v, want withdrawn, changed and settled by this batch", it)
	}
	if it := item(t, st, 2); it.Status != StatusAssumed || it.Changed != assumed.Changed || it.Settled != assumed.Settled || it.Answer != "guess" {
		t.Fatalf("assumed item after close = %+v, want it untouched", it)
	}
	for id, want := range map[int]string{3: StatusAnswered, 4: StatusSuperseded, 5: StatusOpen} {
		if got := item(t, st, id).Status; got != want {
			t.Fatalf("item %d = %s, want %s", id, got, want)
		}
	}
	if !slices.Equal(out.Withdrawn, []int{1}) || out.Info != "wb:close withdrew (01) (still open in the closed topic)." {
		t.Fatalf("outcome withdrawn %v, info %q", out.Withdrawn, out.Info)
	}

	// A reopen by an add leaves the withdrawn items withdrawn.
	mustApply(t, st, addXML(ClassTechnical, "a", "again"))
	if st.topicClosed("a") {
		t.Fatal("the add should reopen topic a")
	}
	if got := item(t, st, 1).Status; got != StatusWithdrawn {
		t.Fatalf("item 1 after reopen = %s, want withdrawn", got)
	}

	// Closing with nothing open withdraws nothing and adds no info.
	mustApply(t, st, `<wb:patch id="6" status="withdrawn"/>`)
	if out := mustApply(t, st, `<wb:topic name="a"><wb:close result="Done."/></wb:topic>`); len(out.Withdrawn) != 0 || out.Info != "" {
		t.Fatalf("outcome withdrawn %v, info %q", out.Withdrawn, out.Info)
	}
}

// An answer after the close that withdrew its item, in one batch, is refused
// naming that close; the same answer before the close is accepted.
func TestAnswerAfterCloseInOneBatchNamesTheClose(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "a", "open"))
	problems := refused(t, st, `<wb:topic name="a"><wb:close result="Done."/></wb:topic><wb:answer id="1" value="ok">ok</wb:answer>`)
	wantProblems(t, problems, `command 2 <wb:answer> (line 1) (01): command 1 <wb:close> (line 1) earlier in this batch withdrew it; put the answers before the close`)
	if hasProblem(problems, "patch its status to open") {
		t.Fatalf("problems = %q, want no reopen advice for an item this batch withdrew", problems)
	}
	out := mustApply(t, st, `<wb:answer id="1" value="ok">ok</wb:answer><wb:topic name="a"><wb:close result="Done."/></wb:topic>`)
	if got := item(t, st, 1).Status; got != StatusAnswered || len(out.Withdrawn) != 0 {
		t.Fatalf("item 1 = %s, withdrawn %v; want answered and nothing withdrawn", got, out.Withdrawn)
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
	mustApply(t, st, `<wb:topic name="t"><wb:add class="policy"><wb:title>new</wb:title><wb:link>supersedes {{01}}</wb:link><wb:link>261030-feat-x</wb:link></wb:add></wb:topic>
<wb:patch id="1" status="superseded"><wb:link>superseded by {{02}}</wb:link></wb:patch>`)
	// References are display only: an ID the store never held, prose
	// parentheses, and non-digit brace tokens are all accepted.
	mustApply(t, st, `<wb:topic name="t"><wb:add class="technical"><wb:title>see {{77}} after step (2)</wb:title><wb:why>step (3) then {{my-tag}}</wb:why><wb:link>see {{77}}</wb:link><wb:link>step (2)</wb:link></wb:add></wb:topic>`)
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
	// An assumed item reads as settled: its topic ranks with the settled one.
	if strings.Join(names, ",") != "policy,acks,settled,assumed" {
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
	// A second wb:body replaces the body whole.
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
	wantProblems(t, refused(t, st, `<wb:topic><wb:body>x</wb:body></wb:topic>`), "<wb:topic> (line 1) name: required")
	wantProblems(t, refused(t, st, `<wb:topic name=" "><wb:body>x</wb:body></wb:topic>`), "name: must be non-empty")
	wantProblems(t, refused(t, st, `<wb:topic name="n" mode="x"><wb:body>x</wb:body></wb:topic>`), "<wb:topic> (line 1): unknown attribute mode (known: name)")
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
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
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
		"empty":        `<wb:topic name="t"><wb:body></wb:body></wb:topic>`,
		"self-closing": `<wb:topic name="t"><wb:body/></wb:topic>`,
		"blank":        "<wb:topic name=\"t\"><wb:body>\n  \n</wb:body></wb:topic>",
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
		mustApply(t, st, `<wb:topic name="never"><wb:body/></wb:topic>`)
		if len(st.Topics) != 0 {
			t.Fatalf("%s: topics = %+v", name, st.Topics)
		}
	}
	// A topic with items stays closed when its body is cleared; a topic left
	// with neither drops its closed mark, and is unknown again.
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "has items", "x")+topicXML("has items", "b")+topicXML("only body", "b")+`<wb:topic name="has items"><wb:close result="done"/></wb:topic><wb:topic name="only body"><wb:close result="done"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="has items"><wb:body/></wb:topic><wb:topic name="only body"><wb:body/></wb:topic>`)
	if !st.topicClosed("has items") || st.topicClosed("only body") || len(st.ClosedTopics) != 1 {
		t.Fatalf("closed topics = %v", st.ClosedTopics)
	}
	if st.topicKnown("only body") {
		t.Fatal("a topic with neither items nor body must be gone")
	}
	wantProblems(t, refused(t, st, `<wb:topic name="only body"><wb:close result="done"/></wb:topic>`), `no item or topic body has topic "only body"`)
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
		wantProblems(t, problems, "command 1 <wb:body> (line 1): a topic body takes markdown with inline HTML, not a full document")
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
	mustApply(t, st, `<wb:topic name="t"><wb:edit>
<wb:old>## 배경</wb:old>
<wb:new>## 배경과 동기</wb:new>
</wb:edit></wb:topic>`)
	want := "## 배경과 동기\n본문은 닫는 태그까지 raw 텍스트다.\n\n## 결정\nXML로 간다."
	if got := topicEntry(t, st, "t"); got.Body != want || got.Changed != st.Seq || got.Changed == before {
		t.Fatalf("topic = %+v", got)
	}
	// Whole lines match as written on their own lines: the one newline at
	// each end of wb:old and wb:new is the tag layout, the rest is text.
	mustApply(t, st, "<wb:topic name=\"t\"><wb:edit><wb:old>\nXML로 간다.\n</wb:old><wb:new>\nXML로 간다.\n\n## 후속\n</wb:new></wb:edit></wb:topic>")
	if got := st.TopicBody("t"); !strings.HasSuffix(got, "XML로 간다.\n\n## 후속") {
		t.Fatalf("body = %q", got)
	}
	// Raw text in old and new, markup included.
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>## 후속</wb:old><wb:new><details><option>a & b</option></details></wb:new></wb:edit></wb:topic>`)
	if got := st.TopicBody("t"); !strings.HasSuffix(got, "<details><option>a & b</option></details>") {
		t.Fatalf("body = %q", got)
	}
}

func TestApplyEditInsertsThroughAnAnchor(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "## A\n\n## C"))
	// Insert after an anchor, then before one: the anchor sits inside wb:old.
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>## A</wb:old><wb:new>## A

## B</wb:new></wb:edit></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>## C</wb:old><wb:new>## C2
## C</wb:new></wb:edit></wb:topic>`)
	if got := st.TopicBody("t"); got != "## A\n\n## B\n\n## C2\n## C" {
		t.Fatalf("body = %q", got)
	}
}

func TestApplyEditRefusals(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "alpha beta alpha"))
	before := st.TopicBody("t")
	for want, batch := range map[string]string{
		`command 1 <wb:edit> (line 1): <wb:old> matches nothing in topic "t"'s body`:               `<wb:topic name="t"><wb:edit><wb:old>gamma</wb:old><wb:new>x</wb:new></wb:edit></wb:topic>`,
		`<wb:old> matches 2 places in topic "t"'s body; widen it to one or set replace_all="true"`: `<wb:topic name="t"><wb:edit><wb:old>alpha</wb:old><wb:new>x</wb:new></wb:edit></wb:topic>`,
		`topic "other" has no body; set one with <wb:body>`:                                        `<wb:topic name="other"><wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`,
		`goes inside a <wb:topic name="..."> group`:                                                `<wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit>`,
		`<wb:old>: required`:                                `<wb:topic name="t"><wb:edit><wb:new>b</wb:new></wb:edit></wb:topic>`,
		`<wb:old>: must be non-empty`:                       `<wb:topic name="t"><wb:edit><wb:old></wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`,
		`<wb:new>: required (empty deletes the match)`:      `<wb:topic name="t"><wb:edit><wb:old>alpha</wb:old></wb:edit></wb:topic>`,
		`more than one <wb:old>`:                            `<wb:topic name="t"><wb:edit><wb:old>a</wb:old><wb:old>b</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`,
		`replace_all: must be "true" or "false"; got "yes"`: `<wb:topic name="t"><wb:edit replace_all="yes"><wb:old>alpha</wb:old><wb:new>x</wb:new></wb:edit></wb:topic>`,
		`the edited body would hold a full document`:        `<wb:topic name="t"><wb:edit><wb:old>beta</wb:old><wb:new><!DOCTYPE html></wb:new></wb:edit></wb:topic>`,
	} {
		wantProblems(t, refused(t, st, batch), want)
	}
	// An edit on a topic that has items but no body is refused the same way.
	mustApply(t, st, addXML(ClassTechnical, "items only", "x"))
	wantProblems(t, refused(t, st, `<wb:topic name="items only"><wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`), `topic "items only" has no body`)
	// A failed edit leaves the body as it was, whatever else the batch holds.
	if st.TopicBody("t") != before {
		t.Fatalf("body = %q", st.TopicBody("t"))
	}
	// An edit inside a batch sees the bodies the commands ahead of it wrote.
	wantProblems(t, refused(t, st, `<wb:topic name="new"><wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`+topicXML("new", "a")), `topic "new" has no body`)
	mustApply(t, st, topicXML("new", "a")+`<wb:topic name="new"><wb:edit><wb:old>a</wb:old><wb:new>b</wb:new></wb:edit></wb:topic>`)
	if st.TopicBody("new") != "b" {
		t.Fatalf("body = %q", st.TopicBody("new"))
	}
}

func TestApplyEditReplaceAllAndDelete(t *testing.T) {
	st := &State{}
	mustApply(t, st, topicXML("t", "alpha beta alpha gamma alpha"))
	mustApply(t, st, `<wb:topic name="t"><wb:edit replace_all="true"><wb:old>alpha</wb:old><wb:new>A</wb:new></wb:edit></wb:topic>`)
	if got := st.TopicBody("t"); got != "A beta A gamma A" {
		t.Fatalf("body = %q", got)
	}
	// replace_all="false" is the default; one match is fine either way.
	mustApply(t, st, `<wb:topic name="t"><wb:edit replace_all="false"><wb:old>gamma</wb:old><wb:new>G</wb:new></wb:edit></wb:topic>`)
	// An empty wb:new deletes the match, self-closing or not.
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old> beta</wb:old><wb:new/></wb:edit></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old> G</wb:old><wb:new></wb:new></wb:edit></wb:topic>`)
	if got := st.TopicBody("t"); got != "A A A" {
		t.Fatalf("body = %q", got)
	}
	// An edit that leaves nothing clears the body like an empty wb:body.
	mustApply(t, st, `<wb:topic name="t"><wb:edit replace_all="true"><wb:old>A</wb:old><wb:new/></wb:edit></wb:topic>`)
	if len(st.Topics) != 0 {
		t.Fatalf("topics = %+v", st.Topics)
	}
}

func TestApplyEditAndTopicReopenAClosedTopic(t *testing.T) {
	st := &State{}
	mustApply(t, st, addXML(ClassTechnical, "t", "x")+topicXML("t", "alpha")+`<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>alpha</wb:old><wb:new>beta</wb:new></wb:edit></wb:topic>`)
	if st.topicClosed("t") {
		t.Fatal("an edit that changes the body reopens the topic")
	}
	mustApply(t, st, `<wb:topic name="t"><wb:close result="done"/></wb:topic>`+topicXML("t", "gamma"))
	if st.topicClosed("t") {
		t.Fatal("a wb:body that changes the body reopens the topic")
	}
	// Closing after the change in the same batch keeps it closed.
	mustApply(t, st, topicXML("t", "delta")+`<wb:topic name="t"><wb:close result="done"/></wb:topic>`)
	if !st.topicClosed("t") {
		t.Fatal("close after a body change must stick")
	}
	// A body-only topic closes the same way.
	mustApply(t, st, topicXML("doc", "x")+`<wb:topic name="doc"><wb:close result="done"/></wb:topic>`)
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
	mustApply(t, st, `<wb:topic name="notes"><wb:close result="done"/></wb:topic>`)
	if !st.topicClosed("notes") {
		t.Fatal("wb:close must accept a topic that has a body and no items")
	}
}
