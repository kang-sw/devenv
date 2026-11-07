package wswhiteboard

import (
	"encoding/json"
	"testing"
)

func parse(s string) ([]command, []string) {
	p := &problems{}
	cmds := parseCommands(p, s)
	return cmds, p.list
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
	cmds := mustParse(t, `<wb:topic name="화이트보드 재설계">`+"\n"+body+"\n"+`</wb:topic>`)
	if len(cmds) != 1 || cmds[0].tag != "wb:topic" || cmds[0].body != body {
		t.Fatalf("topic command = %+v", cmds)
	}
	if name, _ := cmds[0].attr("name"); name != "화이트보드 재설계" {
		t.Fatalf("name = %q", name)
	}

	// The same text through an item field and an answer quote, applied.
	st := &State{}
	mustApply(t, st, `<wb:add id="195" class="policy" topic="화이트보드 재설계">
<wb:title>화이트보드 입력을 XML 커맨드 목록으로 바꾼다</wb:title>
<wb:option>문자열 인자 하나에 XML 커맨드를 나열한다</wb:option>
<wb:option>연산별 도구로 쪼갠다</wb:option>
<wb:recommend>a</wb:recommend>
<wb:why>본문에 <details>나 <option>이 있어도 안전하다. A & B</wb:why>
<wb:link>261107-epic-ws-dashboard-messenger-round2</wb:link>
</wb:add>
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
	body := `quoting the format: <wb:add class="policy" topic="t"><wb:title>x</wb:title></wb:add> and </wb:why> and </wb:topicx>, a < b`
	cmds := mustParse(t, `<wb:topic name="t">`+body+`</wb:topic><wb:close topic="t"/>`)
	if len(cmds) != 2 || cmds[0].body != body || cmds[1].tag != "wb:close" {
		t.Fatalf("commands = %+v", cmds)
	}
}

func TestCDATAHoldsALiteralClosingTag(t *testing.T) {
	cmds := mustParse(t, `<wb:topic name="t">use <![CDATA[</wb:topic> and <b> & ]]> here</wb:topic>`)
	if want := "use </wb:topic> and <b> & " + " here"; cmds[0].body != want {
		t.Fatalf("body = %q, want %q", cmds[0].body, want)
	}
	// A field body, and the newline strip around a CDATA-only body.
	cmds = mustParse(t, "<wb:add class=\"technical\" topic=\"t\"><wb:why>\n<![CDATA[the </wb:why> tag]]>\n</wb:why><wb:title>x</wb:title></wb:add>")
	if got := cmds[0].fieldsOf("wb:why"); len(got) != 1 || got[0] != "the </wb:why> tag" {
		t.Fatalf("why = %q", got)
	}
	// Applied: the stored body holds the closing tag literally.
	st := &State{}
	mustApply(t, st, `<wb:topic name="t"><![CDATA[a </wb:topic> b]]></wb:topic>`)
	if st.TopicBody("t") != "a </wb:topic> b" {
		t.Fatalf("body = %q", st.TopicBody("t"))
	}
	_, problems := parse(`<wb:topic name="t"><![CDATA[never closed</wb:topic>`)
	wantProblems(t, problems, "line 1: <wb:topic>: a CDATA section has no closing ]]>")
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
		cmds := mustParse(t, `<wb:topic name="t">`+tc.in+`</wb:topic>`)
		if cmds[0].body != tc.want {
			t.Errorf("%s: body of %q = %q, want %q", name, tc.in, cmds[0].body, tc.want)
		}
	}
	// The same rule reads every body: a field, an answer quote, wb:old/wb:new.
	cmds := mustParse(t, "<wb:add class=\"technical\" topic=\"t\"><wb:title>\n  padded\n</wb:title></wb:add><wb:answer id=\"1\" value=\"ok\">\nq\n\n</wb:answer><wb:edit topic=\"t\"><wb:old>\nfind\n</wb:old><wb:new>\n\nrepl\n</wb:new></wb:edit>")
	if got := cmds[0].fieldsOf("wb:title")[0]; got != "  padded" {
		t.Errorf("title = %q", got)
	}
	if cmds[1].body != "q\n" {
		t.Errorf("answer body = %q", cmds[1].body)
	}
	if cmds[2].fieldsOf("wb:old")[0] != "find" || cmds[2].fieldsOf("wb:new")[0] != "\nrepl" {
		t.Errorf("edit fields = %+v", cmds[2].fields)
	}
}

func TestAttributeValuesAreQuotedAndNeverDecoded(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"double quotes":                   {`name="plain"`, "plain"},
		"single quotes":                   {`name='plain'`, "plain"},
		"single quote inside double":      {`name="it's"`, "it's"},
		"double quote inside single":      {`name='say "hi"'`, `say "hi"`},
		"entities stay as written":        {`name="a &amp; b &lt; c &quot;"`, `a &amp; b &lt; c &quot;`},
		"angle brackets inside":           {`name="a>b<c"`, "a>b<c"},
		"slash and gt do not end the tag": {`name="x/>y"`, "x/>y"},
		"spaces around =":                 {`name = "spaced"`, "spaced"},
		"newlines in the tag":             {"\n  name=\n\"multi\"\n", "multi"},
		"empty":                           {`name=""`, ""},
		"hangul":                          {`name="초기 범위"`, "초기 범위"},
	} {
		cmds, problems := parse("<wb:topic " + tc.in + ">x</wb:topic>")
		if len(problems) > 0 {
			t.Errorf("%s: problems %q", name, problems)
			continue
		}
		if got, _ := cmds[0].attr("name"); got != tc.want {
			t.Errorf("%s: name = %q, want %q", name, got, tc.want)
		}
	}
	// A topic name with a quote or entity is the same topic everywhere.
	st := &State{}
	mustApply(t, st, `<wb:add class="technical" topic='say "hi" &amp; <go>'><wb:title>x</wb:title></wb:add>`+
		`<wb:topic name='say "hi" &amp; <go>'>body</wb:topic><wb:close topic='say "hi" &amp; <go>'/>`)
	if st.Items[0].Topic != `say "hi" &amp; <go>` || st.TopicBody(`say "hi" &amp; <go>`) != "body" || !st.topicClosed(`say "hi" &amp; <go>`) {
		t.Fatalf("state = %+v", st)
	}
}

func TestSelfClosingAndEmptyForms(t *testing.T) {
	cmds := mustParse(t, `<wb:close topic="a"/><wb:close topic="b"></wb:close><wb:close topic="c" >  </wb:close><wb:floor id="3" /><wb:topic name="t"/>
<wb:add class="technical" topic="t"><wb:title>x</wb:title><wb:why/><wb:link></wb:link></wb:add>`)
	if len(cmds) != 6 {
		t.Fatalf("commands = %+v", cmds)
	}
	for i, tag := range []string{"wb:close", "wb:close", "wb:close", "wb:floor", "wb:topic", "wb:add"} {
		if cmds[i].tag != tag || cmds[i].pos != i+1 {
			t.Fatalf("command %d = %+v", i, cmds[i])
		}
	}
	if cmds[4].body != "" || len(cmds[5].fields) != 3 || cmds[5].fields[1].body != "" {
		t.Fatalf("empty forms = %+v / %+v", cmds[4], cmds[5].fields)
	}
	if cmds[5].line != 2 || cmds[5].fields[0].line != 2 {
		t.Fatalf("add line = %d", cmds[5].line)
	}
}

func TestCommandPositionsAndLinesInProblems(t *testing.T) {
	_, problems := parse("<wb:close topic=\"a\"/>\n\n<wb:floor id=\"1\"/>\n<wb:mystery>\nbody\n</wb:mystery>\n<wb:add class=\"x\" topic=\"t\"\n  color=\"red\"><wb:title>t</wb:title><wb:size>1</wb:size></wb:add>")
	wantProblems(t, problems,
		"command 3 <wb:mystery> (line 4): unknown command (commands: wb:add, wb:answer, wb:close, wb:edit, wb:embed-dir, wb:floor, wb:patch, wb:topic)",
		"command 4 <wb:add> (line 7): unknown attribute color (known: id, class, topic)",
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
		"unprefixed command":         {`<add class="policy" topic="t"/>`, "command 1 <add> (line 1): unknown command"},
		"unknown self-closing":       {`<wb:nope/>`, "unknown command"},
		"unknown attribute":          {`<wb:close topic="t" force="1"/>`, "unknown attribute force (known: topic)"},
		"attribute on a field":       {`<wb:add class="technical" topic="t"><wb:title lang="ko">x</wb:title></wb:add>`, "command 1 <wb:add> (line 1) field <wb:title>: unknown attribute lang (it takes none)"},
		"attribute given twice":      {`<wb:close topic="a" topic="b"/>`, "attribute topic given twice"},
		"unknown field":              {`<wb:add class="technical" topic="t"><wb:subtitle>x</wb:subtitle></wb:add>`, "unknown field <wb:subtitle> (fields: "},
		"field of another command":   {`<wb:edit topic="t"><wb:title>x</wb:title></wb:edit>`, "unknown field <wb:title> (fields: wb:old, wb:new)"},
		"stray text":                 {"hello <wb:close topic=\"a\"/>", `line 1: text outside a command: "hello"`},
		"stray text after":           {"<wb:close topic=\"a\"/>\ntrailing words", `line 2: text outside a command: "trailing words"`},
		"text in a container":        {`<wb:add class="technical" topic="t">loose<wb:title>x</wb:title></wb:add>`, `outside its field tags: "loose"`},
		"stray closing tag":          {`<wb:close topic="a"/></wb:add>`, "line 1: </wb:add> closes nothing open"},
		"stray closing in container": {`<wb:add class="technical" topic="t"></wb:why><wb:title>x</wb:title></wb:add>`, "</wb:why> closes nothing open"},
		"body in an empty command":   {`<wb:close topic="a">text</wb:close>`, "command 1 <wb:close> (line 1): takes no body; write it as <wb:close .../>"},
		"unterminated body":          {"<wb:topic name=\"t\">\nnever closed", "line 1: <wb:topic> has no closing </wb:topic>"},
		"unterminated container":     {"<wb:add class=\"technical\" topic=\"t\">\n<wb:title>x</wb:title>\n", "<wb:add> from line 1 has no closing </wb:add>"},
		"unterminated field":         {`<wb:add class="technical" topic="t"><wb:title>x</wb:add>`, "<wb:title> has no closing </wb:title>"},
		"unterminated unknown":       {`<wb:nope>x`, "<wb:nope> has no closing </wb:nope>"},
		"unterminated tag":           {`<wb:close topic="t"`, "<wb:close> is not closed with '>'"},
		"unterminated quote":         {`<wb:close topic="t/>`, "attribute topic has no closing \""},
		"unquoted attribute":         {`<wb:close topic=t/>`, `attribute topic takes a value quoted with " or '`},
		"attribute without value":    {`<wb:close topic/>`, `attribute topic needs ="value"`},
		"bare angle bracket":         {`< wb:close/>`, "expected a tag name after '<'"},
		"junk in the tag":            {`<wb:close "t"/>`, `unexpected '"' in the tag`},
		"half a self-close":          {`<wb:close topic="t"/ >`, "expected '/>'"},
		"unterminated closing":       {`<wb:close topic="a"/></wb:add`, "a closing tag is not closed with '>'"},
	} {
		_, problems := parse(tc.in)
		if !hasProblem(problems, tc.want) {
			t.Errorf("%s: parse(%q) problems %q lack %q", name, tc.in, problems, tc.want)
		}
	}
}

// Reading goes on past a problem it can step over, so one refusal lists
// every problem; past an unterminated body it cannot, but the commands
// before it are still validated.
func TestRefusalListsEveryProblemSoFar(t *testing.T) {
	st := &State{}
	problems := refused(t, st, "<wb:nope/>\nstray\n</wb:add>\n<wb:close topic=\"missing\"/>\n<wb:add class=\"bad\" topic=\"t\"><wb:title>x</wb:title></wb:add>")
	wantProblems(t, problems,
		"command 1 <wb:nope> (line 1): unknown command",
		`line 2: text outside a command: "stray"`,
		"line 3: </wb:add> closes nothing open",
		`command 2 <wb:close> (line 4) topic: no item or topic body has topic "missing"`,
		"command 3 <wb:add> (line 5) class: must be one of",
	)
	problems = refused(t, st, "<wb:close topic=\"missing\"/>\n<wb:topic name=\"t\">never closed")
	wantProblems(t, problems,
		`command 1 <wb:close> (line 1) topic: no item or topic body has topic "missing"`,
		"line 2: <wb:topic> has no closing </wb:topic>",
	)
	// A well-formed command ahead of the broken text is not applied.
	refused(t, st, addXML(ClassTechnical, "t", "x")+"\n<wb:topic name=\"t\">never closed")
	if len(st.Items) != 0 {
		t.Fatalf("items = %+v", st.Items)
	}
	// The retired JSON payload text is no command at all.
	wantProblems(t, refused(t, st, `{"add":[{"class":"policy"}]}`), "text outside a command")
}
