package wswhiteboard

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func parse(s string) ([]command, []string) {
	p := &problems{}
	cmds := parseCommands(p, s)
	return cmds, p.List
}

func mustParse(t *testing.T, s string) []command {
	t.Helper()
	cmds, problems := parse(s)
	if len(problems) > 0 {
		t.Fatalf("parse(%q) problems: %q", s, problems)
	}
	return cmds
}

// A body is raw text: Korean, HTML, mermaid, and & come out as written, and
// an item field, a topic body, and an answer quote all read the same way.
func TestBodiesAreRawText(t *testing.T) {
	body := "## 배경\n\n<details><summary>접기</summary>\n<option value=\"a\">옵션 & <b>굵게</b></option>\n</details>\n\n```mermaid\ngraph TD; A-->B\n```\n\n1 < 2 && 3 > 2, &amp; &lt;"
	cmds := mustParse(t, `<wb:topic name="화이트보드 재설계"><wb:body>`+"\n"+body+"\n"+`</wb:body></wb:topic>`)
	if len(cmds) != 1 || cmds[0].Tag != "wb:body" || cmds[0].Body != body {
		t.Fatalf("body command = %+v", cmds)
	}
	if name, _ := cmds[0].Scoped(); name != "화이트보드 재설계" {
		t.Fatalf("name = %q", name)
	}

	// The same text through an item field and an answer quote, applied.
	st := &State{}
	mustApply(t, st, `<wb:topic name="화이트보드 재설계"><wb:add id="195" class="policy">
<wb:title>화이트보드 입력을 XML 커맨드 목록으로 바꾼다</wb:title>
<wb:option>문자열 인자 하나에 XML 커맨드를 나열한다</wb:option>
<wb:option>연산별 도구로 쪼갠다</wb:option>
<wb:recommend>a</wb:recommend>
<wb:why>본문에 <details>나 <option>이 있어도 안전하다. A & B</wb:why>
<wb:link>261107-epic-ws-dashboard-messenger-round2</wb:link>
</wb:add></wb:topic>
<wb:answer id="195" value="a">(195) a, 한글 <b>그대로</b></wb:answer>
`+topicXML("화이트보드 재설계", "\n"+body+"\n"))
	got := item(t, st, 195)
	if got.Title != "화이트보드 입력을 XML 커맨드 목록으로 바꾼다" || got.Options[1] != "연산별 도구로 쪼갠다" ||
		got.Rationale != "본문에 <details>나 <option>이 있어도 안전하다. A & B" || got.AnswerQuote != "(195) a, 한글 <b>그대로</b>" {
		t.Fatalf("item = %+v", got)
	}
	if st.TopicBody("화이트보드 재설계") != body {
		t.Fatalf("body = %q", st.TopicBody("화이트보드 재설계"))
	}
	// Stored and loaded back, the body is the same raw text.
	var loaded State
	if err := json.Unmarshal(mustMarshal(t, st), &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.TopicBody("화이트보드 재설계") != body || loaded.Items[0].Rationale != got.Rationale {
		t.Fatalf("stored body = %q", loaded.TopicBody("화이트보드 재설계"))
	}
}

func TestBodyEndsAtItsOwnExactClosingTag(t *testing.T) {
	// Other closing tags, a lookalike, and the format's own tags are text.
	body := `quoting the format: <wb:topic name="t"><wb:add class="policy"><wb:title>x</wb:title></wb:add></wb:topic> and </wb:why> and </wb:bodyx>, a < b`
	cmds := mustParse(t, `<wb:topic name="t"><wb:body>`+body+`</wb:body><wb:close result="done"/></wb:topic>`)
	if len(cmds) != 2 || cmds[0].Body != body || cmds[1].Tag != "wb:close" {
		t.Fatalf("commands = %+v", cmds)
	}
}

func TestCDATAHoldsALiteralClosingTag(t *testing.T) {
	cmds := mustParse(t, `<wb:topic name="t"><wb:body>use <![CDATA[</wb:body> and <b> & ]]> here</wb:body></wb:topic>`)
	if want := "use </wb:body> and <b> & " + " here"; cmds[0].Body != want {
		t.Fatalf("body = %q, want %q", cmds[0].Body, want)
	}
	// A field body, and the newline strip around a CDATA-only body.
	cmds = mustParse(t, "<wb:topic name=\"t\"><wb:add class=\"technical\"><wb:why>\n<![CDATA[the </wb:why> tag]]>\n</wb:why><wb:title>x</wb:title></wb:add></wb:topic>")
	if got := cmds[0].FieldsOf("wb:why"); len(got) != 1 || got[0] != "the </wb:why> tag" {
		t.Fatalf("why = %q", got)
	}
	// Applied: the stored body holds the closing tag literally.
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><wb:body><![CDATA[a </wb:body></wb:topic> b]]></wb:body></wb:topic>`)
	if st.TopicBody("t") != "a </wb:body></wb:topic> b" {
		t.Fatalf("body = %q", st.TopicBody("t"))
	}
	_, problems := parse(`<wb:topic name="t"><wb:body><![CDATA[never closed</wb:body></wb:topic>`)
	wantProblems(t, problems, "line 1: <wb:body>: a CDATA section has no closing ]]>")
}

// A body loses one newline right after its opening tag and one right before
// its closing tag; nothing else is trimmed.
func TestBodyStripsOneNewlineAtEachEndOnly(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"same line":               {"text", "text"},
		"own lines":               {"\ntext\n", "text"},
		"crlf":                    {"\r\ntext\r\n", "text"},
		"leading only":            {"\ntext", "text"},
		"trailing only":           {"text\n", "text"},
		"two newlines keep one":   {"\n\ntext\n\n", "\ntext\n"},
		"spaces are kept":         {"  text  ", "  text  "},
		"space shields a newline": {" \ntext\n ", " \ntext\n "},
		"tabs are kept":           {"\n\ttext\t\n", "\ttext\t"},
		"interior is untouched":   {"\na\n\n  b\n", "a\n\n  b"},
		"one newline is empty":    {"\n", ""},
		"two newlines is one":     {"\n\n", ""},
		"empty":                   {"", ""},
	} {
		cmds := mustParse(t, `<wb:topic name="t"><wb:body>`+tc.in+`</wb:body></wb:topic>`)
		if cmds[0].Body != tc.want {
			t.Errorf("%s: body of %q = %q, want %q", name, tc.in, cmds[0].Body, tc.want)
		}
	}
	// The same rule reads every body: a field, an answer quote, wb:old/wb:new.
	cmds := mustParse(t, "<wb:topic name=\"t\"><wb:add class=\"technical\"><wb:title>\n  padded\n</wb:title></wb:add></wb:topic><wb:answer id=\"1\" value=\"ok\">\nq\n\n</wb:answer><wb:topic name=\"t\"><wb:edit><wb:old>\nfind\n</wb:old><wb:new>\n\nrepl\n</wb:new></wb:edit></wb:topic>")
	if got := cmds[0].FieldsOf("wb:title")[0]; got != "  padded" {
		t.Errorf("title = %q", got)
	}
	if cmds[1].Body != "q\n" {
		t.Errorf("answer body = %q", cmds[1].Body)
	}
	if cmds[2].FieldsOf("wb:old")[0] != "find" || cmds[2].FieldsOf("wb:new")[0] != "\nrepl" {
		t.Errorf("edit fields = %+v", cmds[2].Fields)
	}
}

func TestAttributeValuesAreQuotedAndDecoded(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"double quotes":                   {`path="plain"`, "plain"},
		"single quotes":                   {`path='plain'`, "plain"},
		"single quote inside double":      {`path="it's"`, "it's"},
		"double quote inside single":      {`path='say "hi"'`, `say "hi"`},
		"entities are decoded":            {`path="a &amp; b &lt; c &quot; &#39;"`, `a & b < c " '`},
		"a bare & stays":                  {`path="a && b &nbsp;"`, "a && b &nbsp;"},
		"angle brackets inside":           {`path="a>b<c"`, "a>b<c"},
		"slash and gt do not end the tag": {`path="x/>y"`, "x/>y"},
		"spaces around =":                 {`path = "spaced"`, "spaced"},
		"newlines in the tag":             {"\n  path=\n\"multi\"\n", "multi"},
		"empty":                           {`path=""`, ""},
		"hangul":                          {`path="초기 범위"`, "초기 범위"},
	} {
		cmds, problems := parse("<wb:embed-dir " + tc.in + "/>")
		if len(problems) > 0 {
			t.Errorf("%s: problems %q", name, problems)
			continue
		}
		if got, _ := cmds[0].Attr("path"); got != tc.want {
			t.Errorf("%s: path = %q, want %q", name, got, tc.want)
		}
	}
	// A topic name with a quote or entity is the same topic everywhere,
	// escaped or not.
	st := &State{}
	mustApply(t, st, `<wb:topic name='say "hi" &amp; <go>'><wb:add class="technical"><wb:title>x</wb:title></wb:add></wb:topic>`+
		`<wb:topic name='say "hi" & <go>'><wb:body>body</wb:body></wb:topic><wb:topic name="say &quot;hi&quot; &amp; &lt;go&gt;"><wb:close result="done"/></wb:topic>`)
	if st.Items[0].Topic != `say "hi" & <go>` || st.TopicBody(`say "hi" & <go>`) != "body" || !st.topicClosed(`say "hi" & <go>`) {
		t.Fatalf("state = %+v", st)
	}
}

// An escaped item field is decoded, since an item field is plain text; a
// topic body (raw HTML), a recap (written as a body is), wb:edit's operands
// (matched against a stored body), and an answer (the user's verbatim words)
// keep their entities.
func TestItemFieldsDecodeEntitiesOtherBodiesDoNot(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><wb:add id="1" class="policy">
<wb:title>Key &amp; value</wb:title>
<wb:option><code>&lt;key&gt;</code></wb:option>
<wb:option>a &#60;b&#x3E; <![CDATA[&lt;kept&gt;]]></wb:option>
<wb:recommend>a: &quot;short&quot;</wb:recommend>
<wb:why>x &lt; y</wb:why>
<wb:link>see &apos;notes&apos;</wb:link>
</wb:add></wb:topic>
<wb:topic name="t"><wb:body>raw &lt;b&gt; in HTML</wb:body><wb:recap>a &amp; b</wb:recap></wb:topic>`)
	if st.TopicRecap("t") != "a &amp; b" {
		t.Fatalf("recap = %q", st.TopicRecap("t"))
	}
	got := item(t, st, 1)
	if got.Title != "Key & value" || got.Options[0] != "<code><key></code>" || got.Options[1] != "a <b> &lt;kept&gt;" ||
		got.Recommendation != `a: "short"` || got.Rationale != "x < y" || got.Links[0] != "see 'notes'" {
		t.Fatalf("item = %+v", got)
	}
	mustApply(t, st, `<wb:patch id="1"><wb:why>p &gt; q</wb:why></wb:patch>`)
	if got := item(t, st, 1); got.Rationale != "p > q" {
		t.Fatalf("patched why = %q", got.Rationale)
	}
	if st.TopicBody("t") != "raw &lt;b&gt; in HTML" {
		t.Fatalf("topic body = %q", st.TopicBody("t"))
	}
	cmds := mustParse(t, `<wb:topic name="t"><wb:edit><wb:old>&lt;b&gt;</wb:old><wb:new>&lt;i&gt;</wb:new></wb:edit></wb:topic><wb:answer id="1" value="a &amp; b">(1) &lt;a&gt;</wb:answer>`)
	if cmds[0].FieldsOf("wb:old")[0] != "&lt;b&gt;" || cmds[0].FieldsOf("wb:new")[0] != "&lt;i&gt;" {
		t.Fatalf("edit fields = %+v", cmds[0].Fields)
	}
	if v, _ := cmds[1].Attr("value"); v != "a & b" || cmds[1].Body != "(1) &lt;a&gt;" {
		t.Fatalf("answer = %+v", cmds[1])
	}
	mustApply(t, st, `<wb:topic name="t"><wb:edit><wb:old>&lt;b&gt;</wb:old><wb:new>&lt;i&gt;</wb:new></wb:edit></wb:topic>`)
	if st.TopicBody("t") != "raw &lt;i&gt; in HTML" {
		t.Fatalf("edited topic body = %q", st.TopicBody("t"))
	}
}

func TestSelfClosingAndEmptyForms(t *testing.T) {
	cmds := mustParse(t, `<wb:topic name="a"><wb:close/></wb:topic><wb:topic name="b"><wb:close></wb:close></wb:topic><wb:topic name="c"><wb:close >  </wb:close></wb:topic><wb:floor id="3" /><wb:topic name="t"><wb:body/></wb:topic>
<wb:topic name="t"><wb:add class="technical"><wb:title>x</wb:title><wb:why/><wb:link></wb:link></wb:add></wb:topic>`)
	if len(cmds) != 6 {
		t.Fatalf("commands = %+v", cmds)
	}
	for i, tag := range []string{"wb:close", "wb:close", "wb:close", "wb:floor", "wb:body", "wb:add"} {
		if cmds[i].Tag != tag || cmds[i].Pos != i+1 {
			t.Fatalf("command %d = %+v", i, cmds[i])
		}
	}
	if cmds[4].Body != "" || len(cmds[5].Fields) != 3 || cmds[5].Fields[1].Body != "" {
		t.Fatalf("empty forms = %+v / %+v", cmds[4], cmds[5].Fields)
	}
	if cmds[5].Line != 2 || cmds[5].Fields[0].Line != 2 {
		t.Fatalf("add line = %d", cmds[5].Line)
	}
}

func TestCommandPositionsAndLinesInProblems(t *testing.T) {
	_, problems := parse("<wb:topic name=\"a\"><wb:close/></wb:topic>\n\n<wb:floor id=\"1\"/>\n<wb:mystery>\nbody\n</wb:mystery>\n<wb:topic name=\"t\"><wb:add class=\"x\"\n  color=\"red\"><wb:title>t</wb:title><wb:size>1</wb:size></wb:add></wb:topic>")
	wantProblems(t, problems,
		"command 3 <wb:mystery> (line 4): unknown command (commands: wb:add, wb:answer, wb:body, wb:close, wb:edit, wb:embed-dir, wb:floor, wb:patch, wb:recap, wb:topic)",
		"command 4 <wb:add> (line 7): unknown attribute color (known: id, class)",
		"command 4 <wb:add> (line 7): unknown field <wb:size> (fields: wb:title, wb:option, wb:recommend, wb:why, wb:link)",
	)
	if len(problems) != 3 {
		t.Fatalf("problems = %q", problems)
	}
}

func TestUnknownTagsAttributesAndStructureAreProblems(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		want string
	}{
		"unknown command":            {`<wb:nope x="1">body</wb:nope>`, "command 1 <wb:nope> (line 1): unknown command"},
		"unprefixed command":         {`<add class="policy"/>`, "command 1 <add> (line 1): unknown command"},
		"unknown self-closing":       {`<wb:nope/>`, "unknown command"},
		"unknown attribute":          {`<wb:topic name="t"><wb:close force="1"/></wb:topic>`, "unknown attribute force (known: result)"},
		"attribute on a field":       {`<wb:topic name="t"><wb:add class="technical"><wb:title lang="ko">x</wb:title></wb:add></wb:topic>`, "command 1 <wb:add> (line 1) field <wb:title>: unknown attribute lang (it takes none)"},
		"attribute given twice":      {`<wb:topic name="a"><wb:close result="a" result="b"/></wb:topic>`, "attribute result given twice"},
		"unknown field":              {`<wb:topic name="t"><wb:add class="technical"><wb:subtitle>x</wb:subtitle></wb:add></wb:topic>`, "unknown field <wb:subtitle> (fields: "},
		"field of another command":   {`<wb:topic name="t"><wb:edit><wb:title>x</wb:title></wb:edit></wb:topic>`, "unknown field <wb:title> (fields: wb:old, wb:new)"},
		"stray text":                 {"hello <wb:topic name=\"a\"><wb:close/></wb:topic>", `line 1: text outside a command: "hello"`},
		"stray text after":           {"<wb:topic name=\"a\"><wb:close/></wb:topic>\ntrailing words", `line 2: text outside a command: "trailing words"`},
		"text in a container":        {`<wb:topic name="t"><wb:add class="technical">loose<wb:title>x</wb:title></wb:add></wb:topic>`, `outside its field tags: "loose"`},
		"stray closing tag":          {`<wb:topic name="a"><wb:close/></wb:topic></wb:add>`, "line 1: </wb:add> closes nothing open"},
		"stray closing in container": {`<wb:topic name="t"><wb:add class="technical"></wb:why><wb:title>x</wb:title></wb:add></wb:topic>`, "</wb:why> closes nothing open"},
		"body in an empty command":   {`<wb:topic name="a"><wb:close>text</wb:close></wb:topic>`, "command 1 <wb:close> (line 1): takes no body; write it as <wb:close .../>"},
		"unterminated body":          {"<wb:topic name=\"t\"><wb:body>\nnever closed", "line 1: <wb:body> has no closing </wb:body>"},
		"unterminated container":     {"<wb:topic name=\"t\"><wb:add class=\"technical\">\n<wb:title>x</wb:title>\n", "<wb:add> from line 1 has no closing </wb:add>"},
		"unterminated group":         {"<wb:topic name=\"t\">\n<wb:body>x</wb:body>\n", "line 3: <wb:topic> from line 1 has no closing </wb:topic>"},
		"unterminated field":         {`<wb:topic name="t"><wb:add class="technical"><wb:title>x</wb:add></wb:topic>`, "<wb:title> has no closing </wb:title>"},
		"unterminated unknown":       {`<wb:nope>x`, "<wb:nope> has no closing </wb:nope>"},
		"unterminated tag":           {`<wb:floor id="1"`, "<wb:floor> is not closed with '>'"},
		"unterminated quote":         {`<wb:floor id="1/>`, "attribute id has no closing \""},
		"unquoted attribute":         {`<wb:floor id=1/>`, `attribute id takes a value quoted with " or '`},
		"attribute without value":    {`<wb:floor id/>`, `attribute id needs ="value"`},
		"bare angle bracket":         {`< wb:floor/>`, "expected a tag name after '<'"},
		"junk in the tag":            {`<wb:floor "1"/>`, `unexpected '"' in the tag`},
		"half a self-close":          {`<wb:floor id="1"/ >`, "expected '/>'"},
		"unterminated closing":       {`<wb:topic name="a"><wb:close/></wb:topic></wb:add`, "a closing tag is not closed with '>'"},
	} {
		_, problems := parse(tc.in)
		if !hasProblem(problems, tc.want) {
			t.Errorf("%s: parse(%q) problems %q lack %q", name, tc.in, problems, tc.want)
		}
	}
}

// A wb:topic group names the topic of the commands inside it: one group may
// hold a body, an edit, items, and a close, and a batch may hold several.
func TestTopicGroupScopesItsCommands(t *testing.T) {
	st := &State{}
	mustApply(t, st, `<wb:topic name="Rollout and its long prose name">
<wb:body>alpha</wb:body>
<wb:edit><wb:old>alpha</wb:old><wb:new>beta</wb:new></wb:edit>
<wb:add class="technical"><wb:title>one</wb:title></wb:add>
<wb:add class="announced"><wb:title>two</wb:title></wb:add>
</wb:topic>
<wb:answer id="1" value="ok">ok</wb:answer>
<wb:topic name="Old scope">
<wb:add class="technical"><wb:title>three</wb:title></wb:add>
<wb:close result="Folded into Rollout."/>
</wb:topic>`)
	const long = "Rollout and its long prose name"
	if st.TopicBody(long) != "beta" || item(t, st, 1).Topic != long || item(t, st, 2).Topic != long ||
		item(t, st, 3).Topic != "Old scope" || !st.topicClosed("Old scope") || st.CloseResults["Old scope"] != "Folded into Rollout." {
		t.Fatalf("state = %+v", st)
	}
}

// The old forms are refused, and the refusal of a body written straight in
// wb:topic shows the form that replaced it.
func TestTopicGroupRefusesTheOldForms(t *testing.T) {
	newForm := `a topic body goes in <wb:body> inside the group, and an empty <wb:body/> clears it: <wb:topic name="..."><wb:body>...</wb:body></wb:topic>`
	for name, tc := range map[string]struct {
		in   string
		want []string
	}{
		"body in wb:topic": {"<wb:topic name=\"t\">\n## Context\n<div>raw</div>\n</wb:topic>", []string{
			`<wb:topic> (line 1): text before its first command: "## Context\n<div>raw</div>"; ` + newForm,
		}},
		"empty wb:topic":        {`<wb:topic name="t"></wb:topic>`, []string{`<wb:topic> (line 1): holds no commands; ` + newForm}},
		"self-closing wb:topic": {`<wb:topic name="t"/>`, []string{`<wb:topic> (line 1): holds no commands; ` + newForm}},
		"topic attribute outside a group": {`<wb:close topic="t" result="done"/>`, []string{
			`command 1 <wb:close> (line 1): goes inside a <wb:topic name="..."> group`,
			"command 1 <wb:close> (line 1): unknown attribute topic (known: result)",
		}},
		"topic attribute inside a group": {`<wb:topic name="t"><wb:add class="technical" topic="u"><wb:title>x</wb:title></wb:add></wb:topic>`, []string{
			"command 1 <wb:add> (line 1): unknown attribute topic (known: id, class)",
		}},
		"root-only command inside a group": {`<wb:topic name="t"><wb:body>x</wb:body><wb:floor id="3"/></wb:topic>`, []string{
			"command 2 <wb:floor> (line 1): goes at the root, outside <wb:topic> (line 1)",
		}},
		"nested group": {`<wb:topic name="t"><wb:topic name="u"><wb:body>x</wb:body></wb:topic><wb:body>y</wb:body></wb:topic>`, []string{
			"line 1: <wb:topic> inside <wb:topic> (line 1); groups do not nest",
		}},
	} {
		_, problems := parse(tc.in)
		if strings.Join(problems, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s: problems =\n%s\nwant\n%s", name, strings.Join(problems, "\n"), strings.Join(tc.want, "\n"))
		}
	}
}

// Reading goes on past a problem it can step over, so one refusal lists
// every problem; past an unterminated body it cannot, but the commands
// before it are still validated.
func TestRefusalListsEveryProblemSoFar(t *testing.T) {
	st := &State{}
	problems := refused(t, st, "<wb:nope/>\nstray\n</wb:add>\n<wb:topic name=\"missing\"><wb:close result=\"done\"/></wb:topic>\n<wb:topic name=\"t\"><wb:add class=\"bad\"><wb:title>x</wb:title></wb:add></wb:topic>")
	wantProblems(t, problems,
		"command 1 <wb:nope> (line 1): unknown command",
		`line 2: text outside a command: "stray"`,
		"line 3: </wb:add> closes nothing open",
		`command 2 <wb:close> (line 4): no item, topic body, or recap has topic "missing"`,
		"command 3 <wb:add> (line 5) class: must be one of",
	)
	problems = refused(t, st, "<wb:topic name=\"missing\"><wb:close result=\"done\"/></wb:topic>\n<wb:topic name=\"t\"><wb:body>never closed")
	wantProblems(t, problems,
		`command 1 <wb:close> (line 1): no item, topic body, or recap has topic "missing"`,
		"line 2: <wb:body> has no closing </wb:body>",
	)
	// A well-formed command ahead of the broken text is not applied.
	refused(t, st, addXML(ClassTechnical, "t", "x")+"\n<wb:topic name=\"t\"><wb:body>never closed")
	if len(st.Items) != 0 {
		t.Fatalf("items = %+v", st.Items)
	}
	// The retired JSON payload text is no command at all.
	wantProblems(t, refused(t, st, `{"add":[{"class":"policy"}]}`), "text outside a command")
}

// A closing tag cut short inside a container once looped forever and grew
// the problem list; parsing must return and report the cut tag once.
func TestTruncatedClosingTagInContainerTerminates(t *testing.T) {
	for _, in := range []string{
		"<wb:topic name=\"T\"><wb:add class=\"policy\">\n<wb:title>x</wb:title>\n</wb:add",
		"<wb:topic name=\"x\"><wb:edit><wb:old>a</wb:old></wb:",
	} {
		done := make(chan []string, 1)
		go func() {
			_, problems := parse(in)
			done <- problems
		}()
		select {
		case problems := <-done:
			n := 0
			for _, p := range problems {
				if strings.Contains(p, "a closing tag is not closed with '>'") {
					n++
				}
			}
			if n != 1 {
				t.Errorf("parse(%q) problems %q: want the cut closing tag reported once", in, problems)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("parse(%q) did not return", in)
		}
	}
}
