package wswhiteboard

import (
	"strings"
	"testing"
)

// queryBoard holds, in topic order: a (open policy (01) with options,
// answered (02), assumed (03)), b (open (04) and a body), c (closed, its
// (05) withdrawn by the close), and d (a body only).
func queryBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	mustApply(t, st, `<wb:topic name="a"><wb:add class="policy"><wb:title>pick one</wb:title><wb:option>optx first</wb:option><wb:option>second</wb:option><wb:recommend>a: smaller</wb:recommend></wb:add>
<wb:add class="technical"><wb:title>settled q</wb:title><wb:why>because zeta</wb:why></wb:add>
<wb:add class="technical"><wb:title>assumed q</wb:title></wb:add></wb:topic>
<wb:topic name="b"><wb:add class="technical"><wb:title>open b</wb:title></wb:add><wb:body>notes on b</wb:body></wb:topic>
<wb:topic name="c"><wb:add class="technical"><wb:title>close me</wb:title></wb:add></wb:topic>
<wb:topic name="d"><wb:body>배경 notes</wb:body></wb:topic>`)
	mustApply(t, st, `<wb:answer id="2" value="ok">(02) ok</wb:answer>
<wb:answer id="3" value="ok" assumed="true">sure</wb:answer>
<wb:topic name="c"><wb:close result="done c"/></wb:topic>`)
	return st
}

// queryText runs a query that must be accepted and formats it.
func queryText(t *testing.T, st *State, raw string) string {
	t.Helper()
	var payload any
	if raw != "" {
		payload = queryPayload(t, raw)
	}
	r, problems := Query(st, payload)
	if len(problems) > 0 {
		t.Fatalf("Query(%s) refused: %v", raw, problems)
	}
	return FormatQuery(r)
}

func wantText(t *testing.T, label, text string, wants, unwanted []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("%s: missing %q in:\n%s", label, want, text)
		}
	}
	for _, bad := range unwanted {
		if strings.Contains(text, bad) {
			t.Fatalf("%s: unexpected %q in:\n%s", label, bad, text)
		}
	}
}

func lastLine(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return lines[len(lines)-1]
}

const changedAt = "changed: 2026-11-04T09:00:00Z"

// An unfiltered query prints every open topic with its open items at depth
// items, folds closed topics, and names both omissions on its last line.
func TestQueryUnfilteredDefault(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, "")
	want := `topic "a" [open] items: open 1, answered 1, assumed 1; ` + changedAt + `
  (01) [open] [policy] pick one
topic "b" [open] items: open 1; body: 10 chars; ` + changedAt + `
  (04) [open] [technical] open b
topic "d" [open] items: none; body: 8 chars; ` + changedAt + `
omitted: 2 items not open (answered 1, assumed 1), shown by status, topic, or ids; 1 closed topic with 1 item, listed by closed: true or topic
`
	if text != want {
		t.Fatalf("unfiltered query:\n%s\nwant:\n%s", text, want)
	}
	if got := queryText(t, st, `{}`); got != want {
		t.Fatalf("an empty payload must match no payload:\n%s", got)
	}
}

func TestQueryDepths(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"depth":"topics"}`)
	wantText(t, "depth topics", text, []string{`topic "a" [open]`, `topic "d" [open]`}, []string{"(01)", "items not open"})
	if lastLine(text) != "omitted: 1 closed topic with 1 item, listed by closed: true or topic" {
		t.Fatalf("depth topics omission line: %q", lastLine(text))
	}
	text = queryText(t, st, `{"depth":"full"}`)
	wantText(t, "depth full", text, []string{"  (01) [open] [policy] pick one\n    a) optx first\n    b) second\n    recommendation: a: smaller\n"}, []string{"(02)"})
}

// fields picks the detail lines at depth full; answered_at is a field of
// its own, which answer no longer carries.
func TestQueryFields(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"ids":[1,2],"depth":"full","fields":["answer"]}`)
	wantText(t, "fields answer", text,
		[]string{"  (02) [answered] [technical] settled q\n    answer: ok\n"},
		[]string{"a) optx", "recommendation:", "rationale:", "answer_quote:", "answered_at"})
	text = queryText(t, st, `{"id":2,"depth":"full","fields":["answer","answered_at"]}`)
	wantText(t, "fields answer and time", text, []string{"    answer: ok\n    answered_at: 2026-11-04T09:00:00Z\n"}, nil)
	text = queryText(t, st, `{"id":2,"depth":"full","fields":["rationale","answer_quote"]}`)
	wantText(t, "fields rationale quote", text, []string{"    rationale: because zeta\n", `    answer_quote: "(02) ok"`}, []string{"answer: ok", "answered_at"})
	// fields without depth means depth full, whatever the target's default.
	text = queryText(t, st, `{"id":2,"fields":["rationale"]}`)
	wantText(t, "fields default depth ids", text, []string{"    rationale: because zeta\n"}, []string{"answer: ok"})
	text = queryText(t, st, `{"fields":"options"}`)
	wantText(t, "fields default depth none", text, []string{"  (01) [open] [policy] pick one\n    a) optx first\n    b) second\n"}, []string{"recommendation:"})
	// An explicit depth still wins: summary ignores fields.
	text = queryText(t, st, `{"id":2,"depth":"summary","fields":["rationale"]}`)
	wantText(t, "fields at explicit summary", text, []string{"    answer: ok\n"}, []string{"rationale:"})
}

// id and ids print their items at depth summary, every status, with no
// body, and union; an explicit depth full prints every detail.
func TestQueryIDs(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"id":2}`)
	want := `topic "a" [open] items: open 1, answered 1, assumed 1; ` + changedAt + `
  (02) [answered] [technical] settled q
    answer: ok
`
	if text != want {
		t.Fatalf("id query:\n%s\nwant:\n%s", text, want)
	}
	text = queryText(t, st, `{"id":2,"depth":"full"}`)
	want = `topic "a" [open] items: open 1, answered 1, assumed 1; ` + changedAt + `
  (02) [answered] [technical] settled q
    rationale: because zeta
    answer: ok
    answer_quote: "(02) ok"
    answered_at: 2026-11-04T09:00:00Z
`
	if text != want {
		t.Fatalf("id query at depth full:\n%s\nwant:\n%s", text, want)
	}
	text = queryText(t, st, `{"id":2,"ids":[4,5]}`)
	wantText(t, "id and ids", text, []string{"(02) [answered]", `topic "b" [open]`, "(04) [open]", `topic "c" [closed]`, "(05) [withdrawn]"}, []string{"(01)", "(03)", `topic "d"`, "topic body:", "omitted:"})
	if text := queryText(t, st, `{"ids":[99]}`); text != "no matching items\n" {
		t.Fatalf("an unknown id: %q", text)
	}
}

// topic prints the topic body after its topic line and every item at depth
// summary; a named closed topic is listed with its result.
func TestQueryTopic(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"topic":"b"}`)
	if !strings.HasPrefix(text, `topic "b" [open] items: open 1; body: 10 chars; `+changedAt+"\ntopic body:\n<<<\nnotes on b\n>>>\n  (04) [open] [technical] open b\n") {
		t.Fatalf("topic query:\n%s", text)
	}
	text = queryText(t, st, `{"topic":["a","c"]}`)
	wantText(t, "topic list", text,
		[]string{"(01) [open]", "    a) optx first", "(02) [answered]", "(03) [assumed]", `topic "c" [closed] items: withdrawn 1; ` + changedAt + "; result: done c\n  (05) [withdrawn] [technical] close me"},
		[]string{`topic "b"`, "omitted:"})
	if text := queryText(t, st, `{"topic":"d"}`); text != `topic "d" [open] items: none; body: 8 chars; `+changedAt+"\ntopic body:\n<<<\n배경 notes\n>>>\n" {
		t.Fatalf("body-only topic: %q", text)
	}
	if text := queryText(t, st, `{"topic":"nothing"}`); text != "no matching items\n" {
		t.Fatalf("unknown topic: %q", text)
	}
	if text := queryText(t, &State{}, `{"topic":"x"}`); text != "no matching items\n" {
		t.Fatalf("empty board: %q", text)
	}
	if r, problems := Query(nil, queryPayload(t, `{"topic":"x"}`)); len(problems) > 0 || FormatQuery(r) != "no matching items\n" {
		t.Fatalf("nil state: %v %q", problems, FormatQuery(r))
	}
}

// text matches title, options, rationale, and answer, ignoring case, across
// every status and closed topics, at depth items.
func TestQueryText(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"text":"ZETA"}`)
	if text != `topic "a" [open] items: open 1, answered 1, assumed 1; `+changedAt+"\n  (02) [answered] [technical] settled q [match: rationale]\n" {
		t.Fatalf("text over rationale:\n%s", text)
	}
	wantText(t, "text over options", queryText(t, st, `{"text":"OPTX"}`), []string{"(01)"}, []string{"(02)", "a) optx"})
	wantText(t, "text over title in a closed topic", queryText(t, st, `{"text":"close me"}`), []string{`topic "c" [closed]`, "(05) [withdrawn]"}, []string{`topic "a"`, "omitted:"})
	wantText(t, "text over answer", queryText(t, st, `{"text":"OK"}`), []string{"(02)", "(03)"}, []string{"(01)", "(04)"})
	// topic with text takes the topic defaults with text ANDed in.
	wantText(t, "topic and text", queryText(t, st, `{"topic":"a","text":"zeta"}`), []string{"(02) [answered] [technical] settled q [match: rationale]\n    answer: ok\n"}, []string{"(01)", "(03)"})
	if text := queryText(t, st, `{"topic":"b","text":"zeta"}`); text != "no matching items\n" {
		t.Fatalf("topic and text with no match: %q", text)
	}
}

// An explicit status replaces the unfiltered default's open-only status and
// keeps the rest of the unfiltered defaults; a single string still works.
func TestQueryStatus(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"status":["answered","withdrawn"]}`)
	wantText(t, "status list", text, []string{"  (02) [answered] [technical] settled q\n", `topic "b" [open]`, `topic "d" [open]`}, []string{"(01)", "(05)", "rationale:", "items not open"})
	if lastLine(text) != "omitted: 1 closed topic with 1 item, listed by closed: true or topic" {
		t.Fatalf("status list omission line: %q", lastLine(text))
	}
	wantText(t, "status string", queryText(t, st, `{"status":"assumed"}`), []string{"(03) [assumed]"}, []string{"(01)", "(02)"})
	wantText(t, "status with depth full", queryText(t, st, `{"status":"answered","depth":"full"}`), []string{"    rationale: because zeta"}, nil)
	// An explicit status narrows a topic query too.
	wantText(t, "topic and status", queryText(t, st, `{"topic":"a","status":"open"}`), []string{"(01) [open]"}, []string{"(02)", "(03)", "omitted:"})
}

// closed: true lists closed topics, whose items follow the same status
// default; an explicit closed overrides the per-target default.
func TestQueryClosedAndOverrides(t *testing.T) {
	st := queryBoard(t)
	text := queryText(t, st, `{"closed":true}`)
	wantText(t, "closed true", text, []string{`topic "c" [closed] items: withdrawn 1; ` + changedAt + "; result: done c\n"}, []string{"(05)", "closed topic with"})
	if lastLine(text) != "omitted: 3 items not open (answered 1, assumed 1, withdrawn 1), shown by status, topic, or ids" {
		t.Fatalf("closed true omission line: %q", lastLine(text))
	}
	if text := queryText(t, st, `{"topic":"c","closed":false}`); text != "no matching items\nomitted: 1 closed topic with 1 item, listed by closed: true or topic\n" {
		t.Fatalf("topic with closed false: %q", text)
	}
	wantText(t, "topic without body", queryText(t, st, `{"topic":"b","body":false}`), []string{"(04)"}, []string{"topic body:"})
	wantText(t, "unfiltered with body", queryText(t, st, `{"body":true}`), []string{"topic body:\n<<<\nnotes on b\n>>>\n"}, nil)
	wantText(t, "id at depth items", queryText(t, st, `{"id":2,"depth":"items"}`), []string{"(02) [answered]"}, []string{"rationale:"})
	wantText(t, "topic at depth topics", queryText(t, st, `{"topic":"a","depth":"topics"}`), []string{`topic "a" [open]`}, []string{"(01)", "omitted:"})
}

// Settled items past the page's expanded window stay queryable by id.
func TestQueryReturnsArchivedItems(t *testing.T) {
	st := &State{}
	settleItems(t, st, "a", 12)
	wantText(t, "archived id", queryText(t, st, `{"id":1}`), []string{"  (01) [answered] [technical] item 0\n"}, nil)
}

func TestQueryRefusesBadPayloads(t *testing.T) {
	for raw, wants := range map[string][]string{
		`{"topic":5}`:                     {"payload.topic: must be a string or a list of strings; got number"},
		`{"topic":[1]}`:                   {"payload.topic[0]: must be a string; got number"},
		`{"topic":[]}`:                    {"payload.topic: must list at least one value"},
		`{"status":"done"}`:               {"payload.status: must be one of open, answered, assumed, withdrawn, superseded"},
		`{"status":{"a":1}}`:              {"payload.status: must be a string or a list of strings; got object"},
		`{"status":["open","done"]}`:      {"payload.status[1]: must be one of"},
		`{"depth":"deep"}`:                {"payload.depth: must be one of topics, items, summary, full"},
		`{"fields":["answer","bogus"]}`:   {"payload.fields[1]: must be one of options, recommendation, rationale, answer, answered_at, answer_quote, links"},
		`{"ids":3}`:                       {"payload.ids: must be a list of numbers; got number"},
		`{"ids":[1.5]}`:                   {"payload.ids[0]: must be a whole number"},
		`{"id":"2"}`:                      {"payload.id: must be a number; got string"},
		`{"text":""}`:                     {"payload.text: must be non-empty"},
		`{"body":"yes"}`:                  {"payload.body: must be a boolean; got string"},
		`{"closed":1}`:                    {"payload.closed: must be a boolean; got number"},
		`{"bogus":1,"depth":"x","q":"a"}`: {"payload.bogus: unknown field", "payload.q: unknown field", "payload.depth: must be one of"},
	} {
		r, problems := Query(queryBoard(t), queryPayload(t, raw))
		if len(problems) == 0 {
			t.Fatalf("Query(%s) must refuse; got %+v", raw, r)
		}
		wantProblems(t, problems, wants...)
	}
	if _, problems := Query(nil, `{}`); !hasProblem(problems, "payload: must be an object; got string") {
		t.Fatalf("string payload problems = %v", problems)
	}
}

// summaryBoard holds topic s: an answered policy item (01) with a note, an
// open policy item (02), an assumed technical item (03), a withdrawn item
// (04), an option-less policy item (05) answered in free text, and three
// technical items (06)-(08) answered together.
func summaryBoard(t *testing.T) *State {
	t.Helper()
	st := &State{}
	mustApply(t, st, `<wb:topic name="s"><wb:body>body <code>kept</code> <b>as is</b></wb:body>
<wb:add class="policy"><wb:title>Order <code>parse</code> or <b>emit</b>?</wb:title><wb:option>Emitter first</wb:option><wb:option>Parser <i>first</i></wb:option><wb:recommend>b</wb:recommend><wb:why>line one
line two</wb:why></wb:add>
<wb:add class="policy"><wb:title>open ask</wb:title><wb:option>yes</wb:option><wb:option>no</wb:option><wb:recommend>a: simpler</wb:recommend><wb:why>first
second</wb:why><wb:link>https://example.com</wb:link></wb:add>
<wb:add class="technical"><wb:title>assumed one</wb:title></wb:add>
<wb:add class="technical"><wb:title>dropped one</wb:title></wb:add>
<wb:add class="policy"><wb:title>free form</wb:title></wb:add>
<wb:add class="technical"><wb:title>g1</wb:title></wb:add>
<wb:add class="technical"><wb:title>g2</wb:title></wb:add>
<wb:add class="technical"><wb:title>g3</wb:title></wb:add></wb:topic>`)
	mustApply(t, st, `<wb:answer id="1" value="b, note text">b <code>please</code></wb:answer>
<wb:answer id="3" value="ok" assumed="true">fine</wb:answer>
<wb:patch id="4" status="withdrawn"/>
<wb:answer id="5" value="go ahead">go ahead</wb:answer>
<wb:answer ids="6 7 8" value="ok">all ok</wb:answer>`)
	return st
}

// Depth summary prints a settled item as one decoded answer line and an
// open item as depth full does.
func TestQuerySummary(t *testing.T) {
	st := summaryBoard(t)
	text := queryText(t, st, `{"topic":"s"}`)
	want := `topic "s" [open] items: open 1, answered 5, assumed 1, withdrawn 1; body: 35 chars; ` + changedAt + `
topic body:
<<<
body <code>kept</code> <b>as is</b>
>>>
  (01) [answered] [policy] Order ` + "`parse`" + ` or emit?
    answer: b: Parser first - note text
  (02) [open] [policy] open ask
    a) yes
    b) no
    recommendation: a: simpler
    rationale: first
    second
    links: https://example.com
  (03) [assumed] [technical] assumed one
    answer: ok (assumed)
  (04) [withdrawn] [technical] dropped one
  (05) [answered] [policy] free form
    answer: go ahead
  (06) [answered] [technical] g1
    answer: ok
  (07) [answered] [technical] g2
    answer: ok
  (08) [answered] [technical] g3
    answer: ok
`
	if text != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", text, want)
	}
	if got := queryText(t, st, `{"topic":"s","depth":"summary"}`); got != text {
		t.Fatalf("explicit summary differs from the topic default:\n%s", got)
	}
	// An open item at summary prints exactly as at full.
	if summary, full := queryText(t, st, `{"id":2}`), queryText(t, st, `{"id":2,"depth":"full"}`); summary != full {
		t.Fatalf("open item summary:\n%s\nfull:\n%s", summary, full)
	}
	wantText(t, "summary drops the settled ask", text, nil, []string{"a) Emitter", "rationale: line one", "answer_quote", "answered_at"})
}

// At depth full a grouped answer's quote prints once, a multi-line value
// indents every line, and markup renders as plain text outside the body and
// the answer quote.
func TestQueryFullOutputShape(t *testing.T) {
	st := summaryBoard(t)
	text := queryText(t, st, `{"topic":"s","depth":"full"}`)
	wantText(t, "full", text, []string{
		"  (01) [answered] [policy] Order `parse` or emit?\n    a) Emitter first\n    b) Parser first\n    recommendation: b\n    rationale: line one\n    line two\n    answer: b, note text\n    answer_quote: \"b <code>please</code>\"\n",
		"topic body:\n<<<\nbody <code>kept</code> <b>as is</b>\n>>>\n",
		"  (06) [answered] [technical] g1\n    answer: ok\n    answer_quote: \"all ok\"\n",
		"  (07) [answered] [technical] g2\n    answer: ok\n    answer_quote: same as (06)\n",
		"  (08) [answered] [technical] g3\n    answer: ok\n    answer_quote: same as (06)\n",
	}, []string{"<i>", "Order <code>"})
	// The quote prints in full again in a query that does not print (06).
	wantText(t, "quote without its first", queryText(t, st, `{"ids":[7,8],"depth":"full"}`),
		[]string{"(07) [answered] [technical] g2\n    answer: ok\n    answer_quote: \"all ok\"\n", "answer_quote: same as (07)"}, nil)
}

// A text match names the fields it matched; matching reads stored text.
func TestQueryTextMatchFields(t *testing.T) {
	st := summaryBoard(t)
	wantText(t, "text over title and options", queryText(t, st, `{"text":"first"}`),
		[]string{"  (01) [answered] [policy] Order `parse` or emit? [match: options]\n", "  (02) [open] [policy] open ask [match: rationale]\n"}, []string{"(03)"})
	wantText(t, "text over markup", queryText(t, st, `{"text":"<code>parse"}`), []string{"(01) [answered] [policy] Order `parse` or emit? [match: title]"}, nil)
	wantText(t, "text over title and answer", queryText(t, st, `{"text":"g"}`), []string{"(05) [answered] [policy] free form [match: answer]", "(06) [answered] [technical] g1 [match: title]"}, nil)
}

func TestPlain(t *testing.T) {
	for in, want := range map[string]string{
		"<code>x</code> and <B>y</B>":                      "`x` and y",
		"a<br>b<br/>c<br />d":                              "a b c d",
		"<strong>s</strong><em>e</em><kbd>k</kbd><i>i</i>": "seki",
		`<b class="x">kept</b> <div>kept</div>`:            `<b class="x">kept <div>kept</div>`,
		"no tags":                                          "no tags",
	} {
		if got := Plain(in); got != want {
			t.Errorf("Plain(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestQueryPrintsItemFieldHTMLAsText pins whiteboard.query over the HTML
// the block fields keep: no tag is left, script and style content print
// nothing, and image alt, SVG, and table cell text stay.
func TestQueryPrintsItemFieldHTMLAsText(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="h"><wb:add class="policy"><wb:title>html <code>q</code></wb:title>`+
		`<wb:option>see <img src="a.png" alt="shot"><script>s()</script><style>.x{}</style></wb:option>`+
		`<wb:option><table><tr><td>c1</td><td>c2</td></tr></table></wb:option>`+
		`<wb:recommend>b: <svg viewBox="0 0 9 9"><text>svgtext</text><script>v()</script></svg></wb:recommend>`+
		`<wb:why><details><summary>sum</summary><b>bold</b> &amp; more</details></wb:why></wb:add></wb:topic>`)
	mustApply(t, st, `<wb:answer id="1" value="a">a</wb:answer>`)
	full := queryText(t, st, `{"status": "answered", "depth": "full"}`)
	wantText(t, "full", full,
		[]string{"a) see shot", "b) c1 c2", "recommendation: b: svgtext", "rationale: sum bold & more", "[policy] html `q`"},
		[]string{"<", "s()", ".x{}", "v()"})
	summary := queryText(t, st, `{"status": "answered", "depth": "summary"}`)
	wantText(t, "summary", summary, []string{"answer: a: see shot"}, []string{"<", "s()"})
}

func TestPlainHTML(t *testing.T) {
	for in, want := range map[string]string{
		"no tags, R&D":                                  "no tags, R&D",
		"<code>x</code> a<br>b":                         "`x` a b",
		`<img alt="pic" src="p.png"> tail`:              "pic tail",
		"<ul><li>one</li><li>two</li></ul>":             "one two",
		"a &lt;b&gt; &amp; c":                           "a <b> & c",
		"keep<!-- c -->s":                               "keeps",
		"<noscript>n</noscript><template>t</template>x": "x",
	} {
		if got := PlainHTML(in); got != want {
			t.Errorf("PlainHTML(%q) = %q, want %q", in, got, want)
		}
	}
}
